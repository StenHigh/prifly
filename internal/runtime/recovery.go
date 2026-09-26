package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
}

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
	FrontierAttemptID string                            `json:"source_frontier_attempt_id"`
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
	frontier, attempt, err := recoveryFrontier(source)
	if err != nil {
		return result, err
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
	reused, rootOutputs, err := recoveryTrace(source, oldPlan, target, definitions, resources, events)
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
		return result, local.Reject("recover_workspace_released", "the source Run's tree was released, so the failed stage has no tree to run in again")
	}
	checkpoint, err := lastCheckpoint(source)
	if err != nil {
		return result, err
	}
	if frontier.InvocationID != source.RootInvocationID {
		return result, local.Reject("recover_frontier_unsupported", "the first recovery edition supports a root failed step")
	}
	result = RecoveryPlan{SchemaVersion: "recovery-plan/1", SourceRunID: source.ID, SourceRunVersion: view.Snapshot.Version, TargetWorkflowRef: flow.Ref{ID: target.Workflow.ID, Version: target.Workflow.Version, Digest: target.Digest}, Checkpoint: checkpoint, Claim: claim, FrontierStageID: frontier.StageID, FrontierAttemptID: attempt.ID, FrontierAction: "execute", FrontierReason: "failed Attempt outputs were not all sealed", Reused: reused, RootOutputs: rootOutputs}
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

// recoveryFrontier identifies one settled technical failure. It does not
// authorize reuse: that decision also needs pinned bytes, route and subject.
func recoveryFrontier(source Run) (*Activation, *Attempt, error) {
	if source.Status != "failed" || source.Outcome != nil || source.Settled == nil {
		return nil, nil, local.Reject("recover_source_ineligible", "recovery requires a technically failed Run without an outcome")
	}
	if source.CancelRequested || source.restricted() || source.HasUnresolvedEffects || len(source.Active) != 0 || source.ActiveCheckID != "" || source.PendingAcceptance != nil || source.PendingArtifactPublication != nil || source.PendingDecision != nil {
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
	return frontier, attempt, nil
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

// recoveryTrace checks the actual settled path, including nested calls and
// repeats. It only supports a sequential quality tail; unsupported control
// shapes fail before any new Run or claim is created.
func recoveryTrace(source Run, oldPlan, newPlan *flow.Plan, newDefinitions []PinnedDefinition, newResources []PinnedResource, events []local.Event) ([]RecoveryReuse, map[string]map[string]ArtifactRef, error) {
	frontier, _, err := recoveryFrontier(source)
	if err != nil {
		return nil, nil, err
	}
	if oldPlan == nil || newPlan == nil || oldPlan.Workflow.ID != newPlan.Workflow.ID || oldPlan.Workflow.Definition.Entry != newPlan.Workflow.Definition.Entry {
		return nil, nil, local.Reject("recover_prefix_changed", "root workflow or entry changed")
	}
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
				return nil, nil, local.Reject("recover_trace_invalid", "state transition journal is invalid")
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
				return nil, nil, local.Reject("recover_trace_invalid", "stage activation journal is ambiguous")
			}
			sequences[record.ActivationID] = event.Seq
		}
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
		if activation == nil || activation.ID == frontier.ID {
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
		entry := RecoveryReuse{InvocationID: activation.InvocationID, ActivationID: activation.ID, StageID: activation.StageID, Kind: activation.Kind, Sequence: sequences[activation.ID]}
		if entry.Sequence == 0 || entry.Sequence >= sequences[frontier.ID] {
			return nil, nil, local.Reject("recover_trace_invalid", "accepted stage is missing from the prefix journal")
		}
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
			if attempt.Session == nil {
				return nil, nil, local.Reject("recover_topology_unsupported", "reused quality gate must be an accepted assisted session")
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
	if err := resolveInvocation(frontier.InvocationID); err != nil {
		return nil, nil, err
	}
	oldFrontier, oldOK := oldPlans[frontier.InvocationID].Workflow.Definition.Stages[frontier.StageID]
	newFrontier, newOK := newPlans[frontier.InvocationID].Workflow.Definition.Stages[frontier.StageID]
	if !oldOK || !newOK || oldFrontier.Kind != "step" || newFrontier.Kind != "step" {
		return nil, nil, local.Reject("recover_frontier_unsupported", "failed step is absent from target workflow")
	}
	// The failed stage may change its result contract; its incoming bindings
	// and route cannot. Otherwise the old prefix would feed a different action.
	oldFrontier.StepRef, newFrontier.StepRef = flow.Ref{}, flow.Ref{}
	oldShape, err := recoveryEffectiveStage(oldFrontier, source.Definitions, source.ContextResources)
	if err != nil {
		return nil, nil, err
	}
	newShape, err := recoveryEffectiveStage(newFrontier, newDefinitions, newResources)
	if err != nil {
		return nil, nil, err
	}
	if !bytes.Equal(oldShape, newShape) {
		return nil, nil, local.Reject("recover_frontier_changed", "failed stage bindings or route changed")
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
