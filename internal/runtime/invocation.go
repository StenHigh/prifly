package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"sync"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// Invocation is one WorkflowInvocation in a Run, not another Run or an
// execution slot. A child belongs to the caller activation in its parent.
// Inputs and Outputs contain only the workflow's public ports. Counters cover
// the entire invocation subtree so entering a child cannot reset a budget.
type Invocation struct {
	ID                 string `json:"id"`
	RunID              string `json:"run_id"`
	ParentInvocationID string `json:"parent_invocation_id,omitempty"`
	CallerActivationID string `json:"caller_stage_activation_id,omitempty"`
	// Iteration is the one-based business iteration of a direct repeat body.
	// Root invocations and ordinary calls omit it; technical retries do not
	// create or change an iteration number.
	Iteration *int64 `json:"iteration,omitempty"`
	// BranchID is the declared identity of a parallel branch member. It is a
	// name from the definition, never a position, so reordering the definition
	// produces a different branch rather than silently renumbering this one.
	BranchID           string                 `json:"branch_id,omitempty"`
	WorkflowRef        flow.Ref               `json:"workflow_ref"`
	Status             string                 `json:"status"`
	Outcome            *string                `json:"outcome"`
	Inputs             map[string]ArtifactRef `json:"input_refs"`
	Outputs            map[string]ArtifactRef `json:"output_refs"`
	Ready              []string               `json:"ready_stages"`
	Created            Observation            `json:"created"`
	Settled            *Observation           `json:"settled,omitempty"`
	ControlTransitions int64                  `json:"control_transitions"`
	StepInstances      int64                  `json:"step_instances"`
	ResumeRequired     bool                   `json:"resume_required"`
	CancelRequested    bool                   `json:"cancel_requested"`
}

// MarshalJSON preserves the legacy Run encoding, including empty/null ready
// arrays. Invocation state has only scoped frontiers: it never emits a second
// ready_stages field on the Run or silently discards a nonempty legacy frontier.
func (r Run) MarshalJSON() ([]byte, error) {
	type wireRun Run
	if !isForkState(r.SchemaVersion) && r.Fork != nil {
		return nil, fmt.Errorf("invalid_fork: older state contains fork provenance")
	}
	if !isContextState(r.SchemaVersion) && hasContextStateFields(r) {
		return nil, fmt.Errorf("invalid_context: older state contains context contract fields")
	}
	if !isGuardState(r.SchemaVersion) && len(r.Guards) != 0 {
		return nil, fmt.Errorf("invalid_guard: older state contains live guard registrations")
	}
	if !isReportedCostState(r.SchemaVersion) && hasReportedCostStateFields(r) {
		return nil, fmt.Errorf("invalid_reported_cost: older state contains reported costs")
	}
	if !isArtifactPublicationState(r.SchemaVersion) && len(r.ArtifactPublications) != 0 {
		return nil, fmt.Errorf("invalid_artifact_publication: older state contains artifact publications")
	}
	if !isArtifactClosureState(r.SchemaVersion) && len(r.ArtifactClosures) != 0 {
		return nil, fmt.Errorf("invalid_artifact_closure: older state contains artifact closures")
	}
	if !isActionIntentState(r.SchemaVersion) && (len(r.ActionIntents) != 0 || len(r.ActionAdmissions) != 0) {
		return nil, fmt.Errorf("invalid_action_intent: older state contains action proposals")
	}
	if !isActionAdmissionState(r.SchemaVersion) && len(r.ActionAdmissions) != 0 {
		return nil, fmt.Errorf("invalid_action_admission: older state contains admissions")
	}
	if !isActionDeliveryState(r.SchemaVersion) && len(r.ActionDeliveries) != 0 {
		return nil, fmt.Errorf("invalid_action_delivery: older state contains deliveries")
	}
	if !isRepeatState(r.SchemaVersion) {
		for _, a := range r.Activations {
			if a != nil && a.Repeat != nil {
				return nil, fmt.Errorf("invalid_repeat: older state contains repeat progress")
			}
		}
		for _, inv := range r.Invocations {
			if inv != nil && inv.Iteration != nil {
				return nil, fmt.Errorf("invalid_repeat: older state contains an iteration")
			}
		}
	}
	if !isInvocationState(r.SchemaVersion) {
		return json.Marshal(wireRun(r))
	}
	if len(r.Ready) != 0 {
		return nil, fmt.Errorf("invalid_invocation: invocation state has a legacy Run frontier")
	}
	return json.Marshal(struct {
		*wireRun
		Ready *[]string `json:"ready_stages,omitempty"`
	}{wireRun: (*wireRun)(&r)})
}

// invocationLineage returns the selected invocation followed by its ancestors.
// Missing identities and cycles are errors, never a reason to use the root's
// inputs, plan or budget. Complete persisted-state validation belongs to load.
func (r Run) invocationLineage(id string) ([]*Invocation, error) {
	if !isInvocationState(r.SchemaVersion) {
		return nil, fmt.Errorf("invalid_invocation: scoped state is unavailable")
	}
	lineage := make([]*Invocation, 0, 4)
	for len(lineage) < len(r.Invocations) {
		inv := r.Invocations[id]
		if inv == nil || inv.ID != id || inv.RunID != r.ID {
			return nil, fmt.Errorf("invalid_invocation: unknown invocation identity %q", id)
		}
		lineage = append(lineage, inv)
		if inv.ParentInvocationID == "" {
			if inv.ID != r.RootInvocationID || inv.CallerActivationID != "" {
				return nil, fmt.Errorf("invalid_invocation: disconnected invocation root")
			}
			return lineage, nil
		}
		if inv.ID == r.RootInvocationID || inv.CallerActivationID == "" {
			return nil, fmt.Errorf("invalid_invocation: invalid parent/caller identity")
		}
		id = inv.ParentInvocationID
	}
	return nil, fmt.Errorf("invalid_invocation: invocation ancestry is cyclic or empty")
}

// invocationPlans compiles the pinned closure once and follows only exact
// caller links. The returned plans use the same leaf-to-root order as lineage.
func (r Run) invocationPlans(id string) ([]*Invocation, []*flow.Plan, error) {
	root, err := r.plan()
	if err != nil {
		return nil, nil, err
	}
	return r.invocationPlansFromRoot(id, root)
}

// invocationPlansFromRoot follows current invocation identities through one
// already compiled immutable closure. Mutations that must inspect several
// scopes can do so without recompiling schemas while holding the writer.
func (r Run) invocationPlansFromRoot(id string, root *flow.Plan) ([]*Invocation, []*flow.Plan, error) {
	lineage, err := r.invocationLineage(id)
	if err != nil {
		return nil, nil, err
	}
	last := len(lineage) - 1
	if root == nil || lineage[last].WorkflowRef != r.WorkflowRef || planRef(root) != r.WorkflowRef {
		return nil, nil, fmt.Errorf("invalid_invocation: root workflow reference changed")
	}
	plans := make([]*flow.Plan, len(lineage))
	plans[last] = root
	for i := last - 1; i >= 0; i-- {
		inv, parent := lineage[i], lineage[i+1]
		caller := r.Activations[inv.CallerActivationID]
		if caller == nil || caller.ID != inv.CallerActivationID || caller.InvocationID != parent.ID || !r.childMatchesCaller(inv, caller) {
			return nil, nil, fmt.Errorf("invalid_invocation: caller does not belong to parent")
		}
		child := plans[i+1].BodyPlan(caller.StageID)
		if fanOut(caller.Kind) {
			child = plans[i+1].BranchPlan(caller.StageID, inv.BranchID)
		}
		if child == nil || inv.WorkflowRef != (flow.Ref{ID: child.Workflow.ID, Version: child.Workflow.Version, Digest: child.Digest}) {
			return nil, nil, fmt.Errorf("invalid_invocation: child workflow is not the pinned body target")
		}
		plans[i] = child
	}
	return lineage, plans, nil
}

func (r Run) planForCompiled(root *flow.Plan, invID string) (*flow.Plan, error) {
	_, plans, err := r.invocationPlansFromRoot(invID, root)
	if err != nil {
		return nil, err
	}
	return plans[0], nil
}

func (r Run) planFor(invID string) (*flow.Plan, error) {
	if !isInvocationState(r.SchemaVersion) {
		if invID != r.RootInvocationID {
			return nil, fmt.Errorf("invalid_invocation: legacy Run has only its root invocation")
		}
		return r.plan()
	}
	_, plans, err := r.invocationPlans(invID)
	if err != nil {
		return nil, err
	}
	return plans[0], nil
}

func (r Run) inputsFor(invID string) map[string]ArtifactRef {
	if isInvocationState(r.SchemaVersion) {
		if inv := r.Invocations[invID]; inv != nil {
			return inv.Inputs
		}
		return nil
	}
	if invID == r.RootInvocationID {
		return r.Inputs
	}
	return nil
}

func (r Run) outputsFor(invID string) map[string]ArtifactRef {
	if isInvocationState(r.SchemaVersion) {
		if inv := r.Invocations[invID]; inv != nil {
			return inv.Outputs
		}
		return nil
	}
	if invID == r.RootInvocationID {
		return r.Outputs
	}
	return nil
}

func (r Run) readyFor(invID string) []string {
	if isInvocationState(r.SchemaVersion) {
		if inv := r.Invocations[invID]; inv != nil {
			return inv.Ready
		}
		return nil
	}
	if invID == r.RootInvocationID {
		return r.Ready
	}
	return nil
}

func (r *Run) setReadyFor(invID string, ready []string) error {
	if isInvocationState(r.SchemaVersion) {
		inv := r.Invocations[invID]
		if inv == nil || inv.ID != invID || inv.RunID != r.ID {
			return fmt.Errorf("invalid_invocation: unknown frontier owner")
		}
		inv.Ready = append([]string{}, ready...)
		return nil
	}
	if invID != r.RootInvocationID {
		return fmt.Errorf("invalid_invocation: legacy Run has only its root frontier")
	}
	r.Ready = slices.Clone(ready)
	return nil
}

// declaredParallelism is how much this Run said may run at once, read from the
// workflow it pinned rather than from a nested definition: a branch declaring
// one at a time describes itself, not the Run it belongs to.
func (r Run) declaredParallelism() (int64, error) {
	workflow, err := pinnedBudgetWorkflow(r.Workflow, r.WorkflowRef)
	if err != nil {
		return 0, err
	}
	if workflow.Limits.MaxParallelism < 1 {
		return 1, nil
	}
	return int64(workflow.Limits.MaxParallelism), nil
}

// activeIn names an attempt running in exactly this scope, not in a descendant.
// Running work in this scope is what forbids a frontier here; a descendant's
// work is that descendant's business.
func (r Run) activeIn(invocationID string) string {
	for _, id := range r.Active {
		attempt := r.Attempts[id]
		if attempt == nil {
			continue
		}
		if a := r.Activations[attempt.ActivationID]; a != nil && a.InvocationID == invocationID {
			return id
		}
	}
	return ""
}

func (r Run) withinInvocation(candidate, ancestor string) bool {
	if !isInvocationState(r.SchemaVersion) {
		return candidate == r.RootInvocationID && ancestor == r.RootInvocationID
	}
	lineage, err := r.invocationLineage(candidate)
	if err != nil {
		return false
	}
	for _, inv := range lineage {
		if inv.ID == ancestor {
			return true
		}
	}
	return false
}

func (r Run) activationForInvocation(invID, stageID string) *Activation {
	var found *Activation
	for _, activation := range r.Activations {
		if activation != nil && activation.InvocationID == invID && activation.StageID == stageID {
			if found != nil {
				return nil // Never choose an ambiguous identity by map iteration order.
			}
			found = activation
		}
	}
	return found
}

// childForCall applies the one-child-per-call rule only to a call activation.
// Other control operators may eventually own several invocations; this lookup
// must not become a global uniqueness rule for their caller linkage.
func (r Run) childForCall(activationID string) *Invocation {
	activation := r.Activations[activationID]
	if activation == nil || activation.Kind != "call" {
		return nil
	}
	var found *Invocation
	for _, inv := range r.Invocations {
		if inv != nil && inv.CallerActivationID == activationID {
			if found != nil || inv.ParentInvocationID != activation.InvocationID {
				return nil
			}
			found = inv
		}
	}
	return found
}

func (r Run) restrictedFor(invID string) bool {
	if !isInvocationState(r.SchemaVersion) {
		return invID != r.RootInvocationID || r.restricted()
	}
	if _, err := r.invocationLineage(invID); err != nil {
		return true
	}
	for _, stop := range r.Stops {
		if stop.Status != "active" {
			continue
		}
		switch stop.Scope {
		case "", "run":
			return true
		case "invocation":
			if r.Invocations[stop.ScopeID] == nil || r.withinInvocation(invID, stop.ScopeID) {
				return true
			}
		default:
			return true // An unrecognized active restriction cannot grant admission.
		}
	}
	return false
}

// admissionsBlockedFor includes unreleased stops and the explicit resume latch
// of every ancestor. Releasing one child's stop cannot clear a parent's latch.
// The caller separately enforces the Run-wide unknown-effect and cancel barriers.
func (r Run) admissionsBlockedFor(invID string) bool {
	if r.restrictedFor(invID) || r.ResumeRequired {
		return true
	}
	if !isInvocationState(r.SchemaVersion) {
		return false
	}
	lineage, err := r.invocationLineage(invID)
	if err != nil {
		return true
	}
	for _, inv := range lineage {
		if inv.ResumeRequired {
			return true
		}
	}
	return false
}

// chargeInvocation checks every ancestor before changing any counter. Step
// charges count materialized StepInstances, not Attempt admissions; control
// charges include call entry/return even when no worker is created. The Run's
// state and profile were already established when it was read; a budget charge
// re-establishing them on every call bought nothing.
func (r *Run) chargeInvocation(invID string, controls, steps int64) error {
	if controls < 0 || steps < 0 {
		return fmt.Errorf("invalid_invocation: negative budget charge")
	}
	if !isInvocationState(r.SchemaVersion) {
		if invID != r.RootInvocationID {
			return fmt.Errorf("invalid_invocation: legacy Run has only its root budget")
		}
		workflow, err := pinnedBudgetWorkflow(r.Workflow, r.WorkflowRef)
		if err != nil {
			return err
		}
		if !invocationBudgetFits(r.ControlTransitions, controls, workflow.Limits.MaxControlTransitions) || !invocationBudgetFits(int64(len(r.Steps)), steps, workflow.Limits.MaxStepInstances) {
			return local.Reject("budget_exhausted", "Run control transition or step instance limit reached")
		}
		r.ControlTransitions += controls
		return nil
	}
	lineage, limits, err := r.invocationLimits(invID)
	if err != nil {
		return err
	}
	if r.ControlTransitions != lineage[len(lineage)-1].ControlTransitions {
		return fmt.Errorf("invalid_invocation: root and Run control counters disagree")
	}
	for i, inv := range lineage {
		if !invocationBudgetFits(inv.ControlTransitions, controls, limits[i].MaxControlTransitions) || !invocationBudgetFits(inv.StepInstances, steps, limits[i].MaxStepInstances) {
			return local.Reject("budget_exhausted", "invocation or ancestor control transition or step instance limit reached")
		}
	}
	for _, inv := range lineage {
		inv.ControlTransitions += controls
		inv.StepInstances += steps
	}
	r.ControlTransitions += controls
	return nil
}

// Budget accounting runs inside the authoritative write transaction. It reads
// only already-pinned definition bytes and must not compile schemas or launch
// the schema helper under that lock. This identity/bounds reader is not a
// replacement for the executable admission checks in Start and the driver.
// budgetWorkflowCache keeps workflow revisions parsed for budget accounting.
// Verifying a pinned definition canonicalizes and decodes the whole workflow,
// which several charges per command repeated on identical bytes. The key is a
// digest of the exact bytes with their reference, so a hit means these bytes
// were already verified against this reference.
var budgetWorkflowCache = struct {
	sync.Mutex
	entries map[string]flow.WorkflowRevision
}{entries: map[string]flow.WorkflowRevision{}}

// maxCachedBudgetWorkflows bounds the cache; eviction resets it, as for plans.
const maxCachedBudgetWorkflows = 64

func pinnedBudgetWorkflow(data []byte, ref flow.Ref) (flow.WorkflowRevision, error) {
	key := rawDigest(data) + "|" + ref.ID + "@" + ref.Version + ":" + ref.Digest
	budgetWorkflowCache.Lock()
	cached, found := budgetWorkflowCache.entries[key]
	budgetWorkflowCache.Unlock()
	if found {
		return cached, nil
	}
	workflow, err := parsePinnedBudgetWorkflow(data, ref)
	if err != nil {
		return workflow, err
	}
	budgetWorkflowCache.Lock()
	if len(budgetWorkflowCache.entries) >= maxCachedBudgetWorkflows {
		clear(budgetWorkflowCache.entries)
	}
	budgetWorkflowCache.entries[key] = workflow
	budgetWorkflowCache.Unlock()
	return workflow, nil
}

func parsePinnedBudgetWorkflow(data []byte, ref flow.Ref) (flow.WorkflowRevision, error) {
	var workflow flow.WorkflowRevision
	canonical, err := flow.Canonical(data)
	if err != nil {
		return workflow, err
	}
	if rawDigest(canonical) != ref.Digest {
		return workflow, fmt.Errorf("invalid_invocation: pinned budget definition digest changed")
	}
	decoder := json.NewDecoder(bytes.NewReader(canonical))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&workflow); err != nil {
		return workflow, fmt.Errorf("invalid_invocation: invalid pinned budget definition: %w", err)
	}
	if workflow.ID != ref.ID || workflow.Version != ref.Version || workflow.SchemaVersion != "1" && workflow.SchemaVersion != "2" && workflow.SchemaVersion != "3" {
		return workflow, fmt.Errorf("invalid_invocation: pinned budget definition identity changed")
	}
	return workflow, nil
}

func (r Run) invocationLimits(invID string) ([]*Invocation, []flow.Limits, error) {
	lineage, err := r.invocationLineage(invID)
	if err != nil {
		return nil, nil, err
	}
	workflow, err := pinnedBudgetWorkflow(r.Workflow, r.WorkflowRef)
	if err != nil {
		return nil, nil, err
	}
	last := len(lineage) - 1
	if lineage[last].WorkflowRef != r.WorkflowRef {
		return nil, nil, fmt.Errorf("invalid_invocation: root budget workflow changed")
	}
	limits := make([]flow.Limits, len(lineage))
	limits[last] = workflow.Limits
	registry := r.registry()
	for i := last - 1; i >= 0; i-- {
		inv, parent := lineage[i], lineage[i+1]
		caller := r.Activations[inv.CallerActivationID]
		if caller == nil || caller.ID != inv.CallerActivationID || caller.InvocationID != parent.ID || !r.childMatchesCaller(inv, caller) {
			return nil, nil, fmt.Errorf("invalid_invocation: budget caller does not belong to parent")
		}
		stage, exists := workflow.Definition.Stages[caller.StageID]
		if !exists || stage.Kind != caller.Kind {
			return nil, nil, fmt.Errorf("invalid_invocation: budget caller is not the pinned stage")
		}
		ref := stage.WorkflowRef
		if stage.Kind == "map" {
			// Every item runs one pinned body, so membership is the sealed
			// identity rather than a position among declared branches.
			if caller.Parallel == nil || !slices.Contains(caller.Parallel.BranchIDs, inv.BranchID) {
				return nil, nil, fmt.Errorf("invalid_invocation: item is not a member of its sealed collection")
			}
			ref = stage.BodyWorkflowRef
		}
		if stage.Kind == "parallel" {
			index := slices.IndexFunc(stage.ParallelBranches, func(branch flow.ParallelBranch) bool { return branch.ID == inv.BranchID })
			if index < 0 || caller.Parallel == nil || int64(len(caller.Parallel.BranchIDs)) != int64(len(stage.ParallelBranches)) {
				return nil, nil, fmt.Errorf("invalid_invocation: branch is not a member of its pinned stage")
			}
			ref = stage.ParallelBranches[index].WorkflowRef
		}
		if stage.Kind == "repeat" {
			ref = stage.BodyWorkflowRef
			if inv.Iteration == nil || *inv.Iteration > stage.MaxIterations || caller.Repeat.IterationCount > stage.MaxIterations {
				return nil, nil, fmt.Errorf("invalid_invocation: iteration exceeds its pinned repeat limit")
			}
		}
		if ref != inv.WorkflowRef {
			return nil, nil, fmt.Errorf("invalid_invocation: budget workflow is not the pinned body target")
		}
		data, exists := registry[inv.WorkflowRef]
		if !exists {
			return nil, nil, fmt.Errorf("invalid_invocation: child budget definition is missing")
		}
		workflow, err = pinnedBudgetWorkflow(data, inv.WorkflowRef)
		if err != nil {
			return nil, nil, err
		}
		limits[i] = workflow.Limits
	}
	return lineage, limits, nil
}

func invocationBudgetFits(used, additional, limit int64) bool {
	return used >= 0 && additional >= 0 && used <= limit && additional <= limit-used
}
