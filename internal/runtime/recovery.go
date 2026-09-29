package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

type RecoveryRequest struct {
	SourceRunID      string `json:"source_run_id"`
	SourceRunVersion int64  `json:"source_run_version"`
	ReviewDigest     string `json:"review_digest"`
	// FromStage names an accepted stage of the source's root invocation, run
	// before the point the source stopped at, to start again from instead.
	FromStage string `json:"from_stage,omitempty"`
}

// RecoveryReason and ResumeReason are the fork reasons of a Run made from a
// recovery plan: a technical failure run again, or a stopped Run resumed by
// its own workflow's declaration.
const (
	RecoveryReason = "recover_failed_stage"
	ResumeReason   = "resume_stopped_run"
)

// CheckRecoveryContext refuses to silently change the decisions or executor
// profile that made the accepted prefix meaningful.
func (e *Engine) CheckRecoveryContext(ctx context.Context, request RecoveryRequest, catalog *DecisionCatalog, sheet *DecisionSheet, profiles map[string]ModelProfileTranslation) error {
	source, view, err := e.load(ctx, request.SourceRunID)
	if err != nil {
		return err
	}
	if view.Snapshot.Version != request.SourceRunVersion {
		return local.Reject("recover_source_changed", "source Run changed after prepare")
	}
	if !reflect.DeepEqual(source.DecisionCatalog, catalog) || !recoverySameChoices(source.DecisionSheet, sheet) || !reflect.DeepEqual(source.ModelProfileTranslations, profiles) {
		return local.Reject("recover_context_changed", "source decisions or executor profile differ from the recovery launch")
	}
	return nil
}

func recoverySameChoices(a, b *DecisionSheet) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if len(a.Records) != len(b.Records) {
		return false
	}
	for i := range a.Records {
		x, y := a.Records[i], b.Records[i]
		if x.DefinitionID != y.DefinitionID || x.DefinitionDigest != y.DefinitionDigest || x.Status != y.Status || !bytes.Equal(x.Value, y.Value) {
			return false
		}
	}
	return true
}

type RecoveryPlan struct {
	SchemaVersion     string                            `json:"schema_version"`
	SourceRunID       string                            `json:"source_run_id"`
	SourceRunVersion  int64                             `json:"source_run_version"`
	TargetWorkflowRef flow.Ref                          `json:"target_workflow_ref"`
	Checkpoint        *CheckpointRef                    `json:"checkpoint,omitempty"`
	Claim             *ContinuationClaim                `json:"claim,omitempty"`
	FrontierStageID   string                            `json:"frontier_stage_id"`
	FrontierAttemptID string                            `json:"source_frontier_attempt_id,omitempty"`
	FrontierAction    string                            `json:"frontier_action"`
	FrontierReason    string                            `json:"frontier_reason"`
	NextStageID       string                            `json:"next_stage_id,omitempty"`
	CandidateRef      *ArtifactRef                      `json:"candidate_ref,omitempty"`
	Reused            []RecoveryReuse                   `json:"reused"`
	RootOutputs       map[string]map[string]ArtifactRef `json:"root_output_refs"`
	ReviewDigest      string                            `json:"review_digest"`
}

type RecoveryProvenance struct {
	SchemaVersion    string `json:"schema_version"`
	SourceRunID      string `json:"source_run_id"`
	SourceRunVersion int64  `json:"source_run_version"`
	ReviewDigest     string `json:"review_digest"`
	// SubjectCommit is kept by recovery/1 only, which chose its tree by a
	// commit read out of an artifact. recovery/2 takes the source Run's own
	// tree over and chooses nothing, so it records the field empty.
	SubjectCommit   string                            `json:"subject_commit,omitempty"`
	FrontierStageID string                            `json:"frontier_stage_id"`
	FrontierAction  string                            `json:"frontier_action,omitempty"`
	CandidateRef    *ArtifactRef                      `json:"source_candidate_ref,omitempty"`
	Reused          []RecoveryReuse                   `json:"reused"`
	RootOutputs     map[string]map[string]ArtifactRef `json:"root_output_refs"`
}

func recoveryInvariant(r Run) error {
	if r.Recovery == nil {
		return nil
	}
	p := r.Recovery
	if !isRecoveryState(r.SchemaVersion) || !(p.SchemaVersion == "recovery/1" && recoveryCommit(p.SubjectCommit) || p.SchemaVersion == "recovery/2" && p.SubjectCommit == "" && isContinuationState(r.SchemaVersion)) || r.Fork == nil || r.Fork.SourceRunID != p.SourceRunID || r.Fork.SourceRunVersion != p.SourceRunVersion || p.ReviewDigest == "" || p.FrontierStageID == "" || p.FrontierAction != "" && p.FrontierAction != "execute" && p.FrontierAction != "revalidate" || p.FrontierAction == "revalidate" && p.CandidateRef == nil || len(p.Reused) == 0 || p.RootOutputs == nil {
		return local.ErrIntegrity
	}
	for stageID, outputs := range p.RootOutputs {
		if stageID == "" || len(outputs) == 0 {
			return local.ErrIntegrity
		}
		for port, ref := range outputs {
			if port == "" || ref.ArtifactID == "" || ref.Revision < 1 || ref.Digest == "" {
				return local.ErrIntegrity
			}
		}
	}
	return nil
}

// PlanRecovery reads only immutable source evidence and compiled target bytes.
// The caller may compile a not-yet-imported package, then review this digest
// before any claim, import or new Run exists.
func (e *Engine) PlanRecovery(ctx context.Context, request RecoveryRequest, target *flow.Plan, definitions []PinnedDefinition, resources []PinnedResource) (RecoveryPlan, error) {
	var result RecoveryPlan
	source, view, err := e.load(ctx, request.SourceRunID)
	if err != nil {
		return result, err
	}
	if view.Snapshot.Version != request.SourceRunVersion || source.Profile != flow.CoreProfile {
		return result, local.Reject("recover_source_changed", "source Run version or profile differs from recovery request")
	}
	oldPlan, err := source.plan()
	if err != nil {
		return result, err
	}
	events := []local.Event{}
	for after := int64(0); ; {
		page, err := e.Events(ctx, source.ID, after, 1000)
		if err != nil {
			return result, err
		}
		events = append(events, page.Events...)
		if len(page.Events) == 0 || int64(len(events)) >= view.Snapshot.EventSeq {
			break
		}
		after = page.Events[len(page.Events)-1].Seq
	}
	sequences, err := activationSequences(events)
	if err != nil {
		return result, err
	}
	point, err := recoveryPointOf(source, oldPlan, target, sequences, request.FromStage)
	if err != nil {
		return result, err
	}
	reused, rootOutputs, err := recoveryTrace(source, oldPlan, target, definitions, resources, sequences, point)
	if err != nil {
		return result, err
	}
	for _, entry := range reused {
		for _, ref := range entry.Outputs {
			if _, _, err := e.Artifact(ref); err != nil {
				return result, local.Reject("recover_evidence_unavailable", "accepted source output is unavailable: "+entry.StageID)
			}
		}
	}
	// The failed stage runs again in the tree it failed in, so the source's
	// claim is handed over as it was left. A released tree is gone; nothing
	// the authority holds could stand in for it.
	claim, released, err := e.recoveryClaim(ctx, source.ID)
	if err != nil {
		return result, err
	}
	if released {
		return result, local.Reject("recover_workspace_released", "the source Run's tree was released, so the stage has no tree to run in again")
	}
	checkpoint, err := lastCheckpoint(source)
	if err != nil {
		return result, err
	}
	result = RecoveryPlan{SchemaVersion: "recovery-plan/1", SourceRunID: source.ID, SourceRunVersion: view.Snapshot.Version, TargetWorkflowRef: flow.Ref{ID: target.Workflow.ID, Version: target.Workflow.Version, Digest: target.Digest}, Checkpoint: checkpoint, Claim: claim, FrontierStageID: point.StageID, FrontierAction: "execute", FrontierReason: point.Reason, Reused: reused, RootOutputs: rootOutputs}
	// Only a failed Attempt can have left a result that passes without
	// another process; a stage resumed or chosen runs again.
	attempt := point.Attempt
	if attempt == nil {
		return digestRecoveryPlan(result)
	}
	result.FrontierAttemptID = attempt.ID
	frontier := point.Activation
	candidate, err := recoveryCandidateRef(events, attempt.ID)
	if err != nil {
		return RecoveryPlan{}, err
	}
	if candidate != (ArtifactRef{}) {
		result.CandidateRef = &candidate
		_, data, err := e.Artifact(candidate)
		if err != nil {
			return RecoveryPlan{}, local.Reject("recover_candidate_invalid", "candidate evidence cannot be read")
		}
		var reported Result
		if json.Unmarshal(data, &reported) != nil || reported.AttemptID != attempt.ID || reported.RunID != source.ID || reported.StepInstanceID != attempt.StepID || reported.EnvelopeDigest != attempt.EnvelopeDigest {
			return RecoveryPlan{}, local.Reject("recover_candidate_invalid", "candidate evidence does not belong to the failed Attempt")
		}
		sealed := reported.Outputs != nil && attempt.ProcessOutcome != nil && attempt.ProcessOutcome.ExitCode != nil && *attempt.ProcessOutcome.ExitCode == 0 && !attempt.ProcessOutcome.Uncertain && attempt.ProcessOutcome.StopReason == ""
		for _, ref := range reported.Outputs {
			if _, _, err := e.Artifact(ref); err != nil {
				sealed = false
			}
		}
		if sealed {
			if err := e.recoveryValidateCandidate(target, frontier.StageID, reported, data); err != nil {
				return RecoveryPlan{}, err
			}
			next := target.Workflow.Definition.Stages[frontier.StageID].On[reported.Verdict]
			if next == "" || target.Workflow.Definition.Stages[next].Kind == "" {
				return RecoveryPlan{}, local.Reject("recover_candidate_route_invalid", "sealed candidate verdict has no declared target route")
			}
			result.FrontierAction, result.FrontierReason, result.NextStageID = "revalidate", "sealed candidate bytes pass the target contract without another process", next
			if len(reported.Outputs) != 0 {
				result.RootOutputs[frontier.StageID] = reported.Outputs
			}
		}
	}
	return digestRecoveryPlan(result)
}

func digestRecoveryPlan(result RecoveryPlan) (RecoveryPlan, error) {
	toDigest := result
	toDigest.ReviewDigest = ""
	encoded, err := json.Marshal(toDigest)
	if err != nil {
		return RecoveryPlan{}, err
	}
	result.ReviewDigest, err = flow.Digest(encoded)
	return result, err
}

func (e *Engine) recoveryValidateCandidate(target *flow.Plan, stageID string, reported Result, data []byte) error {
	step, exists := target.Steps[stageID]
	if !exists || step.Effects.Class != "none" || len(step.WorkspaceTrees) != 0 || len(step.ResultCheckRefs) != 0 || len(reported.EvidenceRefs) != 0 || len(reported.EffectReceiptRefs) != 0 || target.ValidateJSON(step.ResultSchemaRef, data) != nil {
		return local.Reject("recover_candidate_invalid", "sealed result candidate fails the target result contract")
	}
	for name, output := range step.Outputs {
		if slices.Contains(output.RequiredFor, reported.Verdict) && reported.Outputs[name] == (ArtifactRef{}) {
			return local.Reject("recover_candidate_invalid", "sealed candidate is missing a required output: "+name)
		}
	}
	for name, ref := range reported.Outputs {
		output, ok := step.Outputs[name]
		if !ok || len(output.ContentCheckRefs) != 0 {
			return local.Reject("recover_candidate_invalid", "sealed candidate has an undeclared output: "+name)
		}
		_, bytes, err := e.Artifact(ref)
		if err != nil || output.Format == "json" && target.ValidateJSON(*output.SchemaRef, bytes) != nil {
			return local.Reject("recover_candidate_invalid", "sealed candidate output fails the target contract: "+name)
		}
	}
	return nil
}

func recoveryCommit(value string) bool {
	if len(value) != 40 {
		return false
	}
	return strings.Trim(value, "0123456789abcdef") == ""
}

// recoveryPoint is where a new Run starts again from a stopped one: the stage,
// its activation in the source when it had one, the failed Attempt of a
// technical failure, and the journal sequence before which accepted stages
// are carried. A stage never activated before the source stopped has no
// activation, and everything the source accepted is carried.
type recoveryPoint struct {
	StageID    string
	Activation *Activation
	Attempt    *Attempt
	Cutoff     int64
	Reason     string
}

// recoveryFrontier identifies one settled technical failure. It does not
// authorize reuse: that decision also needs pinned bytes, route and subject.
func recoveryFrontier(source Run) (*Activation, *Attempt, error) {
	if source.Status != "failed" || source.Outcome != nil || source.Settled == nil {
		return nil, nil, local.Reject("recover_source_ineligible", "recovery requires a technically failed Run without an outcome")
	}
	if source.CancelRequested || source.restricted() || recoverySourceUnsettled(source) {
		return nil, nil, local.Reject("recover_source_unsettled", "source Run holds a stop, active work or unresolved obligation")
	}
	frontier := source.brokenStage()
	if frontier == nil || frontier.Kind != "step" {
		return nil, nil, local.Reject("recover_frontier_unsupported", "source Run has no single failed step frontier")
	}
	step := source.Steps[frontier.StepID]
	if step == nil || step.Status != "failed" || len(step.AttemptIDs) == 0 {
		return nil, nil, local.Reject("recover_frontier_unsupported", "failed step has no recorded Attempt")
	}
	attempt := source.Attempts[step.AttemptIDs[len(step.AttemptIDs)-1]]
	if attempt == nil || attempt.StepID != step.ID || attempt.ActivationID != frontier.ID || attempt.Settled == nil || attempt.ProcessOutcome != nil && attempt.ProcessOutcome.Uncertain {
		return nil, nil, local.Reject("recover_source_unsettled", "failed Attempt is missing settlement or has an uncertain process")
	}
	if frontier.InvocationID != source.RootInvocationID {
		return nil, nil, local.Reject("recover_frontier_unsupported", "recovery runs again a failed step of the root workflow")
	}
	return frontier, attempt, nil
}

func recoverySourceUnsettled(source Run) bool {
	return source.HasUnresolvedEffects || len(source.Active) != 0 || source.ActiveCheckID != "" || source.PendingAcceptance != nil || source.PendingArtifactPublication != nil || source.PendingDecision != nil
}

// recoveryPointOf finds where the source stopped. A technical failure stops at
// its failed step and needs no declaration. A Run that ended with an outcome,
// or was cancelled, is resumed only when the target -- a revision of its own
// workflow -- declares resumable for it: it stopped at the stage whose
// accepted result led to finish, or at the stage it was cancelled at. An
// operator may name an accepted root stage run before that point instead.
func recoveryPointOf(source Run, oldPlan, target *flow.Plan, sequences map[string]int64, fromStage string) (recoveryPoint, error) {
	var point recoveryPoint
	if oldPlan == nil || target == nil || oldPlan.Workflow.ID != target.Workflow.ID {
		return point, local.Reject("recover_prefix_changed", "root workflow changed")
	}
	switch {
	case source.Status == "uncertain":
		// Not ineligible: it becomes eligible the moment its owner says what
		// the unresolved execution did, and that is the one step to name.
		return point, local.Reject("recover_source_unsettled", "source Run holds an execution whose outcome nobody observed; attest it with run resolve --outcome applied|not_applied, then run it again")
	case source.Status == "failed" && source.Outcome == nil:
		frontier, attempt, err := recoveryFrontier(source)
		if err != nil {
			return point, err
		}
		point = recoveryPoint{StageID: frontier.StageID, Activation: frontier, Attempt: attempt, Cutoff: sequences[frontier.ID], Reason: "failed Attempt outputs were not all sealed"}
	case source.Status == "completed" && source.Outcome != nil, source.Status == "cancelled":
		declared := target.Workflow.Resumable
		if source.Status == "cancelled" && (declared == nil || !declared.FromCancelled) {
			return point, local.Reject("resume_undeclared", "workflow "+target.Workflow.ID+" does not declare resumable from_cancelled, so its cancelled Run is not resumed")
		}
		if source.Status == "completed" && (declared == nil || !slices.Contains(declared.FromOutcomes, *source.Outcome)) {
			return point, local.Reject("resume_undeclared", "workflow "+target.Workflow.ID+" does not declare resumable from_outcomes "+*source.Outcome+", so its Run is not resumed")
		}
		if source.Settled == nil || recoverySourceUnsettled(source) {
			return point, local.Reject("recover_source_unsettled", "source Run holds active work or an unresolved obligation")
		}
		stopped, err := resumeStop(source, oldPlan, sequences)
		if err != nil {
			return point, err
		}
		point = stopped
	default:
		return point, local.Reject("recover_source_ineligible", "only a technically failed, ended or cancelled Run is run again")
	}
	if fromStage != "" {
		chosen := source.activationForInvocation(source.RootInvocationID, fromStage)
		if chosen == nil || chosen.Status != "completed" || chosen.Settled == nil || sequences[chosen.ID] == 0 || sequences[chosen.ID] >= point.Cutoff {
			return recoveryPoint{}, local.Reject("resume_from_stage_invalid", "stage "+fromStage+" is not an accepted stage of the source's root workflow run before "+point.StageID)
		}
		point = recoveryPoint{StageID: fromStage, Activation: chosen, Cutoff: sequences[chosen.ID], Reason: "the operator chose to start again from this accepted stage"}
	}
	switch target.Workflow.Definition.Stages[point.StageID].Kind {
	case "step", "call", "repeat":
	default:
		return recoveryPoint{}, local.Reject("resume_frontier_unsupported", "the source stopped at "+point.StageID+", which is not a step, call or repeat; name an accepted stage before it with --from-stage")
	}
	return point, nil
}

// resumeStop is where an ended or cancelled Run stopped, read with the same
// routing the driver used. Two candidates are never guessed between.
func resumeStop(source Run, p *flow.Plan, sequences map[string]int64) (recoveryPoint, error) {
	root := source.RootInvocationID
	ambiguous := local.Reject("resume_frontier_ambiguous", "the stage the source stopped at cannot be named from its record; name an accepted stage with --from-stage")
	if source.Status == "completed" {
		for _, a := range source.Activations {
			if a.InvocationID != root || a.Kind != "finish" || a.Status != "completed" {
				continue
			}
			arrival := arrivalAt(source, p, root, a.StageID)
			if arrival == nil {
				return recoveryPoint{}, ambiguous
			}
			stopped := source.activationForInvocation(root, arrival.StageID)
			if stopped == nil {
				return recoveryPoint{}, ambiguous
			}
			return recoveryPoint{StageID: stopped.StageID, Activation: stopped, Cutoff: sequences[stopped.ID], Reason: "the Run ended " + *source.Outcome + " on the result of this stage"}, nil
		}
		return recoveryPoint{}, ambiguous
	}
	var cancelled, last *Activation
	for _, a := range source.Activations {
		if a.InvocationID != root {
			continue
		}
		if a.Status == "cancelled" {
			if cancelled != nil {
				return recoveryPoint{}, ambiguous
			}
			cancelled = a
		}
		if a.Status == "completed" && (last == nil || sequences[a.ID] > sequences[last.ID]) {
			last = a
		}
	}
	if cancelled != nil {
		return recoveryPoint{StageID: cancelled.StageID, Activation: cancelled, Cutoff: sequences[cancelled.ID], Reason: "the Run was cancelled at this stage"}, nil
	}
	// Cancelled between stages: the next stage was never activated, and it is
	// where the last accepted one routed.
	if last == nil {
		return recoveryPoint{}, ambiguous
	}
	_, next, _, ok := routeTaken(source, p, last)
	if !ok {
		return recoveryPoint{}, ambiguous
	}
	return recoveryPoint{StageID: next, Cutoff: math.MaxInt64, Reason: "the Run was cancelled before this stage started"}, nil
}

// recoveryCandidateRef reads the immutable result-intake record rather than
// the failed Attempt's cleared in-memory candidate or its mutable output slot.
func recoveryCandidateRef(events []local.Event, attemptID string) (ArtifactRef, error) {
	var selected ArtifactRef
	for _, event := range events {
		if event.Type != "attempt.result_candidate" || event.Version != 1 {
			continue
		}
		var record struct {
			AttemptID       string      `json:"attempt_id"`
			CandidateDigest string      `json:"candidate_digest"`
			Disposition     string      `json:"disposition"`
			EvidenceRef     ArtifactRef `json:"evidence_ref"`
		}
		if err := json.Unmarshal(event.Data, &record); err != nil {
			return ArtifactRef{}, local.Reject("recover_candidate_invalid", "candidate event cannot be decoded")
		}
		if record.AttemptID != attemptID || record.Disposition != "candidate" {
			continue
		}
		if record.EvidenceRef.ArtifactID == "" || record.EvidenceRef.Revision < 1 || record.EvidenceRef.Digest == "" || record.EvidenceRef.Digest != record.CandidateDigest {
			return ArtifactRef{}, local.Reject("recover_candidate_invalid", "candidate event has no matching sealed evidence reference")
		}
		if selected != (ArtifactRef{}) && selected != record.EvidenceRef {
			return ArtifactRef{}, local.Reject("recover_candidate_ambiguous", "failed Attempt has conflicting candidate evidence")
		}
		selected = record.EvidenceRef
	}
	return selected, nil
}

// recoveryEffectiveBytes resolves exact references before comparing a reused
// definition. Project profile /3 changes compiled versions throughout the
// closure when one unrelated component changes; comparing those outer refs
// would always miss a genuinely unchanged gate.
func recoveryEffectiveBytes(ref flow.Ref, definitions []PinnedDefinition, resources []PinnedResource) ([]byte, error) {
	values := make(map[flow.Ref][]byte, len(definitions)+len(resources))
	for _, definition := range definitions {
		values[definition.Ref] = definition.Bytes
	}
	for _, resource := range resources {
		values[resource.Ref] = resource.Bytes
	}
	active := map[flow.Ref]bool{}
	var resolve func(flow.Ref) (any, error)
	var normalize func(any) (any, error)
	normalize = func(value any) (any, error) {
		switch current := value.(type) {
		case map[string]any:
			if len(current) == 3 {
				id, idOK := current["id"].(string)
				version, versionOK := current["version"].(string)
				digest, digestOK := current["digest"].(string)
				if idOK && versionOK && digestOK {
					if id == "" && version == "" && digest == "" {
						return nil, nil
					}
					child := flow.Ref{ID: id, Version: version, Digest: digest}
					resolved, err := resolve(child)
					if err != nil {
						return nil, err
					}
					return map[string]any{"id": id, "effective": resolved}, nil
				}
			}
			result := make(map[string]any, len(current))
			for key, item := range current {
				resolved, err := normalize(item)
				if err != nil {
					return nil, err
				}
				result[key] = resolved
			}
			return result, nil
		case []any:
			result := make([]any, len(current))
			for index, item := range current {
				resolved, err := normalize(item)
				if err != nil {
					return nil, err
				}
				result[index] = resolved
			}
			return result, nil
		default:
			return value, nil
		}
	}
	resolve = func(current flow.Ref) (any, error) {
		data, exists := values[current]
		if !exists || len(data) == 0 || active[current] {
			return nil, local.Reject("recover_definition_unavailable", "effective definition is missing or recursive: "+current.ID)
		}
		active[current] = true
		defer delete(active, current)
		var value any
		if err := json.Unmarshal(data, &value); err != nil {
			return map[string]any{"text": string(data)}, nil
		}
		object, ok := value.(map[string]any)
		if !ok {
			return normalize(value)
		}
		// Only the definition's own compiled version is identity noise. A
		// nested schema/version field remains meaningful unless it is an exact
		// reference resolved by the branch above.
		delete(object, "version")
		return normalize(object)
	}
	value, err := resolve(ref)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode effective definition: %w", err)
	}
	return flow.Canonical(bytes.TrimSpace(encoded))
}

func recoveryEffectiveStage(stage flow.Stage, definitions []PinnedDefinition, resources []PinnedResource) ([]byte, error) {
	data, err := json.Marshal(stage)
	if err != nil {
		return nil, err
	}
	digest, err := flow.Digest(data)
	if err != nil {
		return nil, err
	}
	ref := flow.Ref{ID: "recovery:stage/effective", Version: "1.0.0", Digest: digest}
	return recoveryEffectiveBytes(ref, append(definitions, PinnedDefinition{Ref: ref, Kind: "stage", Bytes: data}), resources)
}

// RecoveryReuse is evidence from the source Run, not an execution in the new
// Run. The source identities remain visible even when its package changes.
type RecoveryReuse struct {
	InvocationID string                 `json:"source_workflow_invocation_id"`
	ActivationID string                 `json:"source_stage_activation_id"`
	StageID      string                 `json:"stage_id"`
	Kind         string                 `json:"kind"`
	StepID       string                 `json:"source_step_instance_id,omitempty"`
	AttemptID    string                 `json:"source_attempt_id,omitempty"`
	Outputs      map[string]ArtifactRef `json:"output_refs,omitempty"`
	Sequence     int64                  `json:"source_event_sequence"`
}

// activationSequences reads when each activation was created from the Run's
// journal: the order the stages ran in, which the state alone does not keep.
func activationSequences(events []local.Event) (map[string]int64, error) {
	sequences := map[string]int64{}
	for _, event := range events {
		switch event.Type {
		case "state.changed":
			var record struct {
				Transitions []struct {
					ID, Kind, From, To string
				} `json:"transitions"`
			}
			if json.Unmarshal(event.Data, &record) != nil {
				return nil, local.Reject("recover_trace_invalid", "state transition journal is invalid")
			}
			for _, transition := range record.Transitions {
				if transition.Kind == "activation" && transition.From == "" && transition.To == "ready" && sequences[transition.ID] == 0 {
					sequences[transition.ID] = event.Seq
				}
			}
		case "stage.activated":
			var record struct {
				ActivationID string `json:"stage_activation_id"`
			}
			if err := json.Unmarshal(event.Data, &record); err != nil || record.ActivationID == "" || sequences[record.ActivationID] != 0 {
				return nil, local.Reject("recover_trace_invalid", "stage activation journal is ambiguous")
			}
			sequences[record.ActivationID] = event.Seq
		}
	}
	return sequences, nil
}

// recoveryTrace checks the actual settled path, including nested calls and
// repeats. It only supports a sequential quality tail; unsupported control
// shapes fail before any new Run or claim is created.
func recoveryTrace(source Run, oldPlan, newPlan *flow.Plan, newDefinitions []PinnedDefinition, newResources []PinnedResource, sequences map[string]int64, point recoveryPoint) ([]RecoveryReuse, map[string]map[string]ArtifactRef, error) {
	if oldPlan == nil || newPlan == nil || oldPlan.Workflow.ID != newPlan.Workflow.ID || oldPlan.Workflow.Definition.Entry != newPlan.Workflow.Definition.Entry {
		return nil, nil, local.Reject("recover_prefix_changed", "root workflow or entry changed")
	}
	oldPlans, newPlans := map[string]*flow.Plan{source.RootInvocationID: oldPlan}, map[string]*flow.Plan{source.RootInvocationID: newPlan}
	resolving := map[string]bool{}
	var resolveInvocation func(string) error
	resolveInvocation = func(id string) error {
		if newPlans[id] != nil {
			return nil
		}
		if resolving[id] {
			return local.Reject("recover_trace_invalid", "nested invocation ancestry is cyclic")
		}
		resolving[id] = true
		defer delete(resolving, id)
		inv := source.Invocations[id]
		if inv == nil || inv.ParentInvocationID == "" || inv.CallerActivationID == "" || inv.Status != "completed" {
			return local.Reject("recover_trace_invalid", "nested invocation is missing or unsettled")
		}
		if err := resolveInvocation(inv.ParentInvocationID); err != nil {
			return err
		}
		caller := source.Activations[inv.CallerActivationID]
		if caller == nil || caller.InvocationID != inv.ParentInvocationID || caller.Kind != "call" && caller.Kind != "repeat" {
			return local.Reject("recover_topology_unsupported", "nested invocation has no supported caller")
		}
		oldChild := oldPlans[inv.ParentInvocationID].BodyPlan(caller.StageID)
		newChild := newPlans[inv.ParentInvocationID].BodyPlan(caller.StageID)
		if oldChild == nil || newChild == nil || oldChild.Workflow.ID != newChild.Workflow.ID || oldChild.Workflow.Definition.Entry != newChild.Workflow.Definition.Entry {
			return local.Reject("recover_prefix_changed", "nested workflow or entry changed")
		}
		oldPlans[id], newPlans[id] = oldChild, newChild
		return nil
	}
	reused := make([]RecoveryReuse, 0, len(source.Activations))
	rootOutputs := map[string]map[string]ArtifactRef{}
	for _, activation := range source.Activations {
		if activation == nil {
			continue
		}
		// Only what the source accepted before the point is carried; the
		// point itself, anything it called and everything after run again.
		sequence := sequences[activation.ID]
		if sequence == 0 {
			return nil, nil, local.Reject("recover_trace_invalid", "accepted stage is missing from the prefix journal")
		}
		if sequence >= point.Cutoff {
			continue
		}
		if activation.Status != "completed" || activation.Settled == nil {
			return nil, nil, local.Reject("recover_trace_invalid", "source prefix contains an unsettled stage")
		}
		if err := resolveInvocation(activation.InvocationID); err != nil {
			return nil, nil, err
		}
		switch activation.Kind {
		case "step", "call", "repeat", "choice", "finish":
		default:
			return nil, nil, local.Reject("recover_topology_unsupported", "source prefix contains unsupported stage kind "+activation.Kind)
		}
		oldStage, oldOK := oldPlans[activation.InvocationID].Workflow.Definition.Stages[activation.StageID]
		newStage, newOK := newPlans[activation.InvocationID].Workflow.Definition.Stages[activation.StageID]
		if !oldOK || !newOK || oldStage.Kind != activation.Kind || newStage.Kind != activation.Kind {
			return nil, nil, local.Reject("recover_prefix_changed", "stage is missing or changed: "+activation.StageID)
		}
		oldEffective, err := recoveryEffectiveStage(oldStage, source.Definitions, source.ContextResources)
		if err != nil {
			return nil, nil, err
		}
		newEffective, err := recoveryEffectiveStage(newStage, newDefinitions, newResources)
		if err != nil {
			return nil, nil, err
		}
		if !bytes.Equal(oldEffective, newEffective) {
			return nil, nil, local.Reject("recover_prefix_changed", "effective contract changed at stage "+activation.StageID)
		}
		entry := RecoveryReuse{InvocationID: activation.InvocationID, ActivationID: activation.ID, StageID: activation.StageID, Kind: activation.Kind, Sequence: sequence}
		if activation.Kind == "step" {
			step := source.Steps[activation.StepID]
			if step == nil || step.Status != "completed" || step.Ref != oldStage.StepRef || len(step.AttemptIDs) == 0 {
				return nil, nil, local.Reject("recover_trace_invalid", "accepted step evidence is missing")
			}
			attemptID := step.AttemptIDs[len(step.AttemptIDs)-1]
			attempt := source.Attempts[attemptID]
			if attempt == nil || attempt.Settled == nil || attempt.Accepted == nil || attempt.Accepted.Verdict != step.Verdict || !reflect.DeepEqual(attempt.Accepted.Outputs, step.Outputs) {
				return nil, nil, local.Reject("recover_trace_invalid", "accepted Attempt evidence is missing")
			}
			entry.StepID, entry.AttemptID, entry.Outputs = step.ID, attemptID, step.Outputs
		} else if activation.Kind == "call" || activation.Kind == "repeat" {
			child := source.currentBody(activation)
			if child == nil || child.Status != "completed" {
				return nil, nil, local.Reject("recover_trace_invalid", "nested workflow has no accepted exports")
			}
			entry.Outputs = child.Outputs
		}
		if activation.InvocationID == source.RootInvocationID && len(entry.Outputs) != 0 {
			rootOutputs[activation.StageID] = entry.Outputs
		}
		reused = append(reused, entry)
	}
	if len(reused) == 0 {
		return nil, nil, local.Reject("resume_prefix_empty", "nothing was accepted before "+point.StageID+", so there is nothing to carry; start the workflow anew")
	}
	oldFrontier, oldOK := oldPlan.Workflow.Definition.Stages[point.StageID]
	newFrontier, newOK := newPlan.Workflow.Definition.Stages[point.StageID]
	if !oldOK || !newOK || oldFrontier.Kind != newFrontier.Kind {
		return nil, nil, local.Reject("recover_frontier_unsupported", "stage "+point.StageID+" is absent from the target workflow or changed its kind")
	}
	// The stage run again may change what it runs -- its step or the workflow
	// it calls; its incoming bindings and route cannot. Otherwise the carried
	// prefix would feed a different action.
	oldFrontier.StepRef, newFrontier.StepRef = flow.Ref{}, flow.Ref{}
	oldFrontier.WorkflowRef, newFrontier.WorkflowRef = flow.Ref{}, flow.Ref{}
	oldFrontier.BodyWorkflowRef, newFrontier.BodyWorkflowRef = flow.Ref{}, flow.Ref{}
	oldShape, err := recoveryEffectiveStage(oldFrontier, source.Definitions, source.ContextResources)
	if err != nil {
		return nil, nil, err
	}
	newShape, err := recoveryEffectiveStage(newFrontier, newDefinitions, newResources)
	if err != nil {
		return nil, nil, err
	}
	if !bytes.Equal(oldShape, newShape) {
		return nil, nil, local.Reject("recover_frontier_changed", "stage "+point.StageID+" changed its bindings or route")
	}
	sort.Slice(reused, func(i, j int) bool {
		if reused[i].Sequence != reused[j].Sequence {
			return reused[i].Sequence < reused[j].Sequence
		}
		return reused[i].ActivationID < reused[j].ActivationID
	})
	return reused, rootOutputs, nil
}

// recoveryClaim is the source Run's tree: the claim still bound to it, or the
// fact that one was bound and has since been released.
func (e *Engine) recoveryClaim(ctx context.Context, runID string) (*ContinuationClaim, bool, error) {
	record, _, err := e.readClaims(ctx)
	if err != nil {
		return nil, false, err
	}
	released := false
	for _, claim := range record.Claims {
		if claim.RunID != runID {
			continue
		}
		if claim.Status == "active" {
			return &ContinuationClaim{ID: claim.ID, Generation: claim.Generation, Mode: claimMode(claim), Path: claim.Path}, false, nil
		}
		released = true
	}
	return nil, released, nil
}
