package runtime

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

const ContinuationReason = "project continuation"

// ContinuationSource is what a continuation takes from the Run it continues,
// exactly as the continuing workflow declared it. The authority finds each
// declared place and reads nothing else: no stage, port or content is known
// to it beforehand.
type ContinuationSource struct {
	RunID      string `json:"source_run_id"`
	RunVersion int64  `json:"source_run_version"`
	WorkflowID string `json:"source_workflow_id"`
	// Outcome is how the source Run ended: its outcome, or cancelled.
	Outcome    string                       `json:"source_outcome"`
	Inputs     map[string]ContinuationInput `json:"inputs"`
	Checkpoint *CheckpointRef               `json:"checkpoint,omitempty"`
	// Claim is the source Run's tree while it is still held; the continuation
	// takes it over as it was left. Absent, the continuation claims anew.
	Claim *ContinuationClaim `json:"claim,omitempty"`
}

type ContinuationInput struct {
	Ref    ArtifactRef             `json:"ref"`
	Source flow.ContinuationSource `json:"source"`
}

// CheckpointRef is a Run's last accepted checkpoint and the stage that
// reported it. What it holds is the workflow author's; it is never read here.
type CheckpointRef struct {
	Ref          ArtifactRef `json:"ref"`
	StageID      string      `json:"stage_id"`
	InvocationID string      `json:"workflow_invocation_id"`
}

type ContinuationClaim struct {
	ID         string `json:"id"`
	Generation int64  `json:"generation"`
	Mode       string `json:"mode"`
	Path       string `json:"path"`
}

func continuationRefs(r Run, version int64, target *flow.Plan) (ContinuationSource, error) {
	declared := target.Workflow.Continuation
	if declared == nil {
		return ContinuationSource{}, local.Reject("project_continue_undeclared", "workflow "+target.Workflow.ID+" declares no continuation")
	}
	// A Run is continued either for the outcome it reached or, where the
	// workflow says so, because it was cancelled: it has no outcome then, and
	// its accepted steps and its tree are what it leaves behind.
	ended, admitted := r.Status, declared.FromCancelled && r.Status == "cancelled"
	if r.Status == "completed" && r.Outcome != nil {
		ended, admitted = *r.Outcome, slices.Contains(declared.FromOutcomes, *r.Outcome)
	}
	if !admitted {
		accepted := slices.Clone(declared.FromOutcomes)
		if declared.FromCancelled {
			accepted = append(accepted, "cancelled")
		}
		return ContinuationSource{}, local.Reject("continuation_source_ineligible", "source Run ended "+ended+"; "+target.Workflow.ID+" continues only Runs that ended "+strings.Join(accepted, ", "))
	}
	// A cancellation that left an effect nobody resolved has not stopped yet:
	// what happened there is not known, and continuing would act on a guess.
	if r.HasUnresolvedEffects || len(r.Active) != 0 {
		return ContinuationSource{}, local.Reject("continuation_source_unsettled", "source Run still holds an active or unresolved execution; resolve it before continuing")
	}
	if !slices.Contains(declared.FromWorkflows, r.WorkflowRef.ID) {
		return ContinuationSource{}, local.Reject("continuation_source_ineligible", "source Run's workflow "+r.WorkflowRef.ID+" is not one "+target.Workflow.ID+" continues: "+strings.Join(declared.FromWorkflows, ", "))
	}
	result := ContinuationSource{RunID: r.ID, RunVersion: version, WorkflowID: r.WorkflowRef.ID, Outcome: ended, Inputs: map[string]ContinuationInput{}}
	// The checkpoint is read only when a declared input takes it: finding it
	// compiles the Run's sealed plans, and nothing else here needs them.
	for _, source := range declared.Inputs {
		if source.Checkpoint && result.Checkpoint == nil {
			checkpoint, err := lastCheckpoint(r)
			if err != nil {
				return ContinuationSource{}, err
			}
			if checkpoint == nil {
				return ContinuationSource{}, local.Reject("continuation_source_incomplete", "source Run accepted no checkpoint, and "+target.Workflow.ID+" continues from one")
			}
			result.Checkpoint = checkpoint
		}
	}
	for _, name := range slices.Sorted(maps.Keys(declared.Inputs)) {
		source := declared.Inputs[name]
		var ref ArtifactRef
		var found bool
		switch {
		case source.SourceInput != "":
			ref, found = r.Inputs[source.SourceInput]
			if !found {
				return ContinuationSource{}, local.Reject("continuation_source_incomplete", "source Run has no input "+source.SourceInput+" for "+name)
			}
		case source.Checkpoint:
			ref = result.Checkpoint.Ref
		default:
			var err error
			if ref, err = acceptedStageOutput(r, source); err != nil {
				return ContinuationSource{}, err
			}
		}
		result.Inputs[name] = ContinuationInput{Ref: ref, Source: source}
	}
	return result, nil
}

// acceptedStageOutput finds the declared output of a stage of the source Run's
// root invocation, accepted with the declared verdict or outcome. A stage run
// more than once there answers with its last settlement; two settlements the
// record cannot order are refused rather than guessed.
func acceptedStageOutput(r Run, source flow.ContinuationSource) (ArtifactRef, error) {
	var ref ArtifactRef
	var last time.Time
	found := false
	for _, activation := range r.Activations {
		if activation == nil || activation.InvocationID != r.RootInvocationID || activation.StageID != source.Stage || activation.Status != "completed" {
			continue
		}
		var outputs map[string]ArtifactRef
		var settled *Observation
		if source.Verdict != "" {
			step := r.Steps[activation.StepID]
			if step == nil || step.Status != "completed" || step.Verdict != source.Verdict {
				continue
			}
			outputs, settled = step.Outputs, step.Settled
		} else {
			for _, invocation := range r.Invocations {
				if invocation != nil && invocation.CallerActivationID == activation.ID && invocation.Status == "completed" && invocation.Outcome != nil && *invocation.Outcome == source.Outcome {
					outputs, settled = invocation.Outputs, invocation.Settled
				}
			}
		}
		candidate, exists := outputs[source.Output]
		if !exists {
			continue
		}
		at, err := settlementTime(settled)
		if err != nil {
			return ArtifactRef{}, err
		}
		if found && at.Equal(last) {
			return ArtifactRef{}, local.Reject("continuation_source_ambiguous", "stage "+source.Stage+" settled twice at the same moment; which output is the last cannot be told")
		}
		if !found || at.After(last) {
			ref, last, found = candidate, at, true
		}
	}
	if !found {
		accepted := source.Verdict
		if accepted == "" {
			accepted = source.Outcome
		}
		return ArtifactRef{}, local.Reject("continuation_source_incomplete", "source Run has no output "+source.Output+" of stage "+source.Stage+" accepted "+accepted)
	}
	return ref, nil
}

// lastCheckpoint is the Run's last accepted checkpoint, across every
// invocation, by settlement. It is derived from what the Run already records
// -- accepted results, their outputs and the sealed plans naming which output
// is the checkpoint -- so no Run records it twice. A result that was not
// accepted reports nothing.
func lastCheckpoint(r Run) (*CheckpointRef, error) {
	root, err := r.plan()
	if err != nil {
		return nil, err
	}
	plans := map[string]*flow.Plan{}
	var result *CheckpointRef
	var last time.Time
	for _, id := range slices.Sorted(maps.Keys(r.Steps)) {
		step := r.Steps[id]
		if step == nil || step.Status != "completed" || step.Verdict == "" {
			continue
		}
		activation := r.Activations[step.ActivationID]
		if activation == nil {
			continue
		}
		plan, cached := plans[activation.InvocationID]
		if !cached {
			if activation.InvocationID == r.RootInvocationID || !isInvocationState(r.SchemaVersion) {
				plan = root
			} else if plan, err = r.planForCompiled(root, activation.InvocationID); err != nil {
				return nil, err
			}
			plans[activation.InvocationID] = plan
		}
		port := plan.Workflow.Definition.Stages[activation.StageID].Checkpoint
		ref, reported := step.Outputs[port]
		if port == "" || !reported {
			continue
		}
		at, err := settlementTime(step.Settled)
		if err != nil {
			return nil, err
		}
		if result != nil && at.Equal(last) {
			return nil, local.Reject("continuation_source_ambiguous", "two checkpoints settled at the same moment; which is the last cannot be told")
		}
		if result == nil || at.After(last) {
			result, last = &CheckpointRef{Ref: ref, StageID: activation.StageID, InvocationID: activation.InvocationID}, at
		}
	}
	return result, nil
}

func settlementTime(settled *Observation) (time.Time, error) {
	if settled == nil {
		return time.Time{}, local.Reject("continuation_source_incomplete", "an accepted result has no settled observation")
	}
	at, err := time.Parse(time.RFC3339Nano, settled.UTC)
	if err != nil {
		return time.Time{}, local.ErrIntegrity
	}
	return at, nil
}

// continuationClaim is the source Run's tree if it still holds one.
func (e *Engine) continuationClaim(ctx context.Context, runID string) (*ContinuationClaim, error) {
	record, _, err := e.readClaims(ctx)
	if err != nil {
		return nil, err
	}
	for _, claim := range record.Claims {
		if claim.RunID == runID && claim.Status == "active" {
			return &ContinuationClaim{ID: claim.ID, Generation: claim.Generation, Mode: claimMode(claim), Path: claim.Path}, nil
		}
	}
	return nil, nil
}

// continuationChainLimit bounds how far a refusal walks fork provenance to name
// the Run a continuation is admissible from. The chain is short in practice;
// reading must not depend on that being true.
const continuationChainLimit = 16

// continuationOriginOf follows fork provenance to the Run a chain of
// continuations started from. It answers "" when the chain does not lead
// anywhere this build can read -- an origin it cannot reach is not a guess it
// may print -- and when the Run it starts at is not a continuation at all.
// The loader is a parameter because the walk is exactly that: the bound is a
// property of the walk, and a cycle is something no stored Run can be made to
// demonstrate.
func continuationOriginOf(r Run, load func(string) (Run, bool)) string {
	for hop := 0; hop < continuationChainLimit; hop++ {
		if r.Fork == nil || r.Fork.Reason != ContinuationReason || r.Fork.SourceRunID == "" {
			if hop == 0 {
				return ""
			}
			return r.ID
		}
		source, ok := load(r.Fork.SourceRunID)
		if !ok {
			return ""
		}
		r = source
	}
	return ""
}

func (e *Engine) continuationOrigin(ctx context.Context, r Run) string {
	return continuationOriginOf(r, func(id string) (Run, bool) {
		source, _, err := e.load(ctx, id)
		return source, err == nil
	})
}

// ContinuationSource reads what target declares it continues from in the
// Run runID. Every carried artifact must still be readable and must satisfy
// the schema of the target input it fills.
func (e *Engine) ContinuationSource(ctx context.Context, runID string, target *flow.Plan) (ContinuationSource, error) {
	r, read, err := e.load(ctx, runID)
	if err != nil {
		return ContinuationSource{}, err
	}
	refs, err := continuationRefs(r, read.Snapshot.Version, target)
	if err != nil {
		// A Run that is itself a continuation is never an admissible source,
		// and the Run that is one is recorded right here in its provenance.
		// Refusing without naming it sent a host to reconstruct the chain by
		// hand from a fact this answer was already holding.
		var rejection *local.Rejection
		if errors.As(err, &rejection) {
			if origin := e.continuationOrigin(ctx, r); origin != "" {
				return refs, local.Reject(rejection.Code, rejection.Message+"; this Run is itself a continuation, and the chain it belongs to started from "+origin+", which is the source a continuation is admissible from")
			}
		}
		return refs, err
	}
	for _, name := range slices.Sorted(maps.Keys(refs.Inputs)) {
		// The bytes are sealed again under the new workflow's own input, so it
		// is their content that must satisfy that input, not the identity of
		// the schema the source sealed them under.
		_, data, err := e.Artifact(refs.Inputs[name].Ref)
		if err != nil {
			return ContinuationSource{}, err
		}
		if port := target.Workflow.Inputs[name].Port; port.Format == "json" && port.SchemaRef != nil {
			if err := target.ValidateJSON(*port.SchemaRef, data); err != nil {
				return ContinuationSource{}, local.Reject("continuation_source_incompatible", "the artifact carried into "+name+" does not satisfy that input: "+err.Error())
			}
		}
	}
	if refs.Claim, err = e.continuationClaim(ctx, runID); err != nil {
		return ContinuationSource{}, err
	}
	return refs, nil
}
