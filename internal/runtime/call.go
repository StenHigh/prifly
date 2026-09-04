package runtime

import (
	"context"
	"errors"
	"maps"
	"slices"
	"sort"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// The qualified local profile has one advancing frontier and one execution
// slot for the whole Run. Scope is explicit even when stage names repeat.
func (r Run) readyScope() (string, string) {
	if !isInvocationState(r.SchemaVersion) {
		if len(r.Ready) == 1 {
			return r.RootInvocationID, r.Ready[0]
		}
		return "", ""
	}
	// Several scopes may be ready at once. The next one is chosen by identity
	// so the order never depends on map iteration or on which branch settled
	// first; a scope with running work of its own holds no frontier.
	var owner, stage string
	for id, inv := range r.Invocations {
		if inv == nil || len(inv.Ready) > 1 {
			return "", ""
		}
		if len(inv.Ready) == 1 && (owner == "" || id < owner) {
			owner, stage = id, inv.Ready[0]
		}
	}
	return owner, stage
}

func (r Run) cancelRequestedFor(invID string) bool {
	if r.CancelRequested {
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
		if inv.CancelRequested {
			return true
		}
	}
	return false
}

func invocationTerminal(status string) bool {
	return status == "completed" || status == "failed" || status == "cancelled"
}

// Scheduling and exports use only the current body. Lineage and telemetry use
// childMatchesCaller so completed iterations remain part of their own scope.
func (r Run) currentBody(a *Activation) *Invocation {
	if a == nil {
		return nil
	}
	switch a.Kind {
	case "repeat":
		return r.currentBodyForRepeat(a.ID)
	case "parallel", "map":
		return r.currentBranchFor(a.ID)
	}
	return r.childForCall(a.ID)
}

func (r *Run) setInvocationStatus(id, status string, settled *Observation) error {
	if !isInvocationState(r.SchemaVersion) {
		if id != r.RootInvocationID {
			return local.ErrIntegrity
		}
		r.Status, r.Settled = status, settled
		return nil
	}
	inv := r.Invocations[id]
	if inv == nil {
		return local.ErrIntegrity
	}
	inv.Status, inv.Settled = status, settled
	// A scope that has finished will never reach a wait it promised. Leaving
	// the promise open would leave its senders waiting on a route nobody will
	// take, so it is retired here with the scope that made it.
	if isWaitState(r.SchemaVersion) && invocationTerminal(status) {
		r.cancelUnreachedReservations(id)
	}
	return nil
}

func (r *Run) advanceInvocation(id, stage string) error {
	if err := r.setReadyFor(id, []string{stage}); err != nil {
		return err
	}
	status := "ready"
	if r.admissionsBlockedFor(id) {
		status = "waiting"
	}
	return r.setInvocationStatus(id, status, nil)
}

func (r *Run) failInvocation(id string, obs Observation) error {
	if err := r.setReadyFor(id, []string{}); err != nil {
		return err
	}
	return r.setInvocationStatus(id, "failed", &obs)
}

// Sync only the versioned tree's Run summary and already-settled child return
// frontier. Returning the call is a separate durable transition, never replayed
// by reconstructing a result from a process or mutable workflow source.
func (r *Run) syncInvocationState() error {
	if !isInvocationState(r.SchemaVersion) {
		return nil
	}
	root := r.Invocations[r.RootInvocationID]
	if root == nil {
		return local.ErrIntegrity
	}
	for _, inv := range r.Invocations {
		if inv.ParentInvocationID == "" || !invocationTerminal(inv.Status) {
			continue
		}
		parent, caller := r.Invocations[inv.ParentInvocationID], r.Activations[inv.CallerActivationID]
		if parent == nil || caller == nil {
			return local.ErrIntegrity
		}
		if !r.childMatchesCaller(inv, caller) {
			return local.ErrIntegrity
		}
		if fanOut(caller.Kind) {
			// Any settled branch waiting for its decision returns the frontier
			// to the fan-out. With several branches live there is no single
			// current one for that to follow from.
			entered, err := r.parallelBranches(caller.ID)
			if err != nil {
				return err
			}
			pending, _ := settledUndecidedBranch(caller, entered)
			if pending == nil {
				continue
			}
			// A failed branch cannot settle its fan-out while a sibling still
			// owns live work. Let the sibling reach its declared failure route
			// first; otherwise the caller would become terminal above a live child.
			if pending.Status == "failed" {
				live := false
				for _, sibling := range entered {
					live = live || sibling.ID != pending.ID && !invocationTerminal(sibling.Status)
				}
				if live {
					continue
				}
			}
		} else if r.currentBody(caller) != inv {
			continue
		}
		if invocationTerminal(parent.Status) || caller.Settled != nil {
			continue
		}
		if caller.Status != "waiting" {
			return local.ErrIntegrity
		}
		if inv.Status == "cancelled" {
			// Cancellation is neither a Workflow outcome nor a catchable error.
			// Keep this caller waiting; do not cancel independent scopes.
			parent.Status = "waiting"
			continue
		}
		if len(parent.Ready) != 0 && (len(parent.Ready) != 1 || parent.Ready[0] != caller.StageID) {
			return local.ErrIntegrity
		}
		if err := r.advanceInvocation(parent.ID, caller.StageID); err != nil {
			return err
		}
	}
	r.Inputs, r.Outputs, r.Outcome, r.Settled = root.Inputs, root.Outputs, root.Outcome, root.Settled
	if invocationTerminal(root.Status) {
		r.Status = root.Status
		return nil
	}
	switch {
	case r.HasUnresolvedEffects:
		r.Status = "uncertain"
	case r.CancelRequested:
		r.Status = "stopping"
	case len(r.Active) != 0 || r.ActiveCheckID != "":
		r.Status = "running"
	default:
		id, _ := r.readyScope()
		r.Status = "waiting"
		if id != "" && !r.admissionsBlockedFor(id) && !r.cancelRequestedFor(id) {
			r.Status = "ready"
		}
	}
	return nil
}

func invocationInvariant(r Run) error {
	if !isInvocationState(r.SchemaVersion) {
		return nil
	}
	root := r.Invocations[r.RootInvocationID]
	if root == nil || root.Iteration != nil || root.WorkflowRef != r.WorkflowRef || root.ControlTransitions != r.ControlTransitions || !maps.Equal(root.Inputs, r.Inputs) || !maps.Equal(root.Outputs, r.Outputs) || root.StepInstances != int64(len(r.Steps)) {
		return errors.New("invocation invariant: root summary or budget differs")
	}
	if len(r.Invocations) > 1025 {
		return errors.New("invocation invariant: tree exceeds control capacity")
	}
	frontiers := 0
	for id, inv := range r.Invocations {
		lineage, err := r.invocationLineage(id)
		if err != nil {
			return err
		}
		if len(lineage) > 9 || inv.Inputs == nil || inv.Outputs == nil || inv.Ready == nil || inv.ControlTransitions < 0 || inv.StepInstances < 0 {
			return errors.New("invocation invariant: invalid scope or counters")
		}
		if (inv.Status == "completed") != (inv.Outcome != nil) || invocationTerminal(inv.Status) != (inv.Settled != nil) || invocationTerminal(inv.Status) && len(inv.Ready) != 0 {
			return errors.New("invocation invariant: terminal state differs from outcome or settlement")
		}
		if len(inv.Ready) > 1 {
			return errors.New("invocation invariant: multiple local frontiers")
		}
		frontiers += len(inv.Ready)
		if len(inv.Ready) != 0 && r.activeIn(inv.ID) != "" {
			return errors.New("invocation invariant: a scope with running work also holds a frontier")
		}
		if inv.ParentInvocationID != "" {
			caller := r.Activations[inv.CallerActivationID]
			if !r.childMatchesCaller(inv, caller) {
				return errors.New("invocation invariant: invalid caller linkage")
			}
			if caller.Settled != nil && !invocationTerminal(inv.Status) {
				return errors.New("invocation invariant: settled caller has live child")
			}
		}
		config := r.WorkflowConfigurations[inv.WorkflowRef.Digest]
		if config == nil || config.WorkflowRef != inv.WorkflowRef || config.SchemaVersion != "effective-configuration/1" || config.Inputs == nil {
			return errors.New("invocation invariant: configuration snapshot missing")
		}
	}
	// Several scopes may be ready at once: readiness is pending work, not
	// running work. What is bounded is how much runs simultaneously, and a
	// check occupies the same capacity as an attempt.
	running := len(r.Active)
	if r.ActiveCheckID != "" {
		running++
	}
	if frontiers > flow.MaxParallelBranches || running > flow.MaxQualifiedParallelism {
		return errors.New("invocation invariant: qualified local capacity exceeded")
	}
	seen := map[[2]string]bool{}
	for id, a := range r.Activations {
		if a == nil {
			return errors.New("invocation invariant: missing activation")
		}
		key := [2]string{a.InvocationID, a.StageID}
		if a.ID != id || r.Invocations[a.InvocationID] == nil || seen[key] {
			return errors.New("invocation invariant: duplicate or missing activation identity")
		}
		seen[key] = true
		if err := r.repeatProgressInvariant(a); err != nil {
			return err
		}
		if err := r.parallelProgressInvariant(a); err != nil {
			return err
		}
	}
	for _, id := range r.Active {
		a := r.Attempts[id]
		if a == nil || r.Activations[a.ActivationID] == nil || invocationTerminal(r.Invocations[r.Activations[a.ActivationID].InvocationID].Status) {
			return errors.New("invocation invariant: active work in terminal scope")
		}
	}
	for _, stop := range r.Stops {
		if stop.Scope != "run" && stop.Scope != "invocation" || stop.Scope == "run" && stop.ScopeID != r.ID || stop.Scope == "invocation" && r.Invocations[stop.ScopeID] == nil {
			return errors.New("invocation invariant: invalid stop scope")
		}
	}
	return nil
}

func (e *Engine) enterCall(ctx context.Context, r Run, view local.ReadView, p *flow.Plan, activation *Activation) error {
	child := p.Calls[activation.StageID]
	if child == nil || !isInvocationState(r.SchemaVersion) {
		return local.ErrIntegrity
	}
	stage := p.Workflow.Definition.Stages[activation.StageID]
	commandID, childID := newID("command"), derivedID("invocation", r.ID, activation.ID)
	inputs, err := e.prepareBodyInputs(r, activation.InvocationID, "", child, stage.InputBindings, commandID, "")
	if err != nil {
		return e.failPreparation(ctx, r, view, p, activation, err, "call_input_binding_failed")
	}
	payload := map[string]any{"workflow_invocation_id": childID, "parent_invocation_id": activation.InvocationID, "caller_stage_activation_id": activation.ID, "workflow_ref": planRef(child), "input_refs": inputs}
	_, err = e.apply(ctx, e.owner, commandID, r.ID, "invocation.created", payload, &view.Snapshot.Version, local.CommandCAS, func(r *Run, s local.Snapshot, obs Observation) (local.Change, error) {
		a := r.Activations[activation.ID]
		if a == nil || a.Kind != "call" || a.Status != "ready" || a.Settled != nil || r.childForCall(a.ID) != nil || r.admissionsBlockedFor(a.InvocationID) || r.cancelRequestedFor(a.InvocationID) || r.HasUnresolvedEffects || r.activeIn(a.InvocationID) != "" {
			return local.Change{}, local.Reject("call_blocked", "call no longer owns its ready scope")
		}
		if err := r.setReadyFor(a.InvocationID, []string{}); err != nil {
			return local.Change{}, err
		}
		if err := r.setInvocationStatus(a.InvocationID, "waiting", nil); err != nil {
			return local.Change{}, err
		}
		a.Status = "waiting"
		r.Invocations[childID] = &Invocation{ID: childID, RunID: r.ID, ParentInvocationID: a.InvocationID, CallerActivationID: a.ID, WorkflowRef: planRef(child), Status: "ready", Inputs: inputs, Outputs: map[string]ArtifactRef{}, Ready: []string{child.Workflow.Definition.Entry}, Created: obs}
		if err := r.beginWorkflowInputAcceptance(child, childID, obs); err != nil {
			return local.Change{}, err
		}
		fact := maps.Clone(payload)
		fact["observation"] = obs
		data, err := canonical(fact)
		return local.Change{RequireStorageBudget: true, Events: []local.EventInput{{Type: "invocation.created", Version: 1, Data: data}}}, err
	})
	return err
}

func invocationFinishedEvent(inv *Invocation, rootID string, obs Observation) (local.EventInput, error) {
	kind := "invocation.finished"
	if inv.ID == rootID {
		kind = "run.finished"
	}
	fact := map[string]any{"run_id": inv.RunID, "workflow_invocation_id": inv.ID, "parent_invocation_id": inv.ParentInvocationID, "caller_stage_activation_id": inv.CallerActivationID, "workflow_ref": inv.WorkflowRef, "status": inv.Status, "outcome": inv.Outcome, "output_refs": inv.Outputs, "observation": obs}
	if inv.Iteration != nil {
		fact["iteration"] = *inv.Iteration
	}
	data, err := canonical(fact)
	return local.EventInput{Type: kind, Version: 1, Data: data}, err
}

func (e *Engine) returnCall(ctx context.Context, r Run, view local.ReadView, p *flow.Plan, activation *Activation) error {
	_, err := e.apply(ctx, e.owner, newID("command"), r.ID, "stage.call_returned", map[string]any{"stage_activation_id": activation.ID}, &view.Snapshot.Version, local.CommandCAS, func(r *Run, s local.Snapshot, obs Observation) (local.Change, error) {
		a, child := r.Activations[activation.ID], r.childForCall(activation.ID)
		if a == nil || a.Kind != "call" || a.Status != "waiting" || a.Settled != nil || child == nil || (child.Status != "completed" && child.Status != "failed") || r.admissionsBlockedFor(a.InvocationID) || r.cancelRequestedFor(a.InvocationID) || r.HasUnresolvedEffects || r.activeIn(a.InvocationID) != "" {
			return local.Change{}, local.Reject("call_blocked", "call return is not a settled unrestricted child")
		}
		if err := r.chargeInvocation(a.InvocationID, 1, 0); err != nil {
			return local.Change{}, err
		}
		a.Status, a.Settled = "completed", &obs
		failure, next := "", ""
		if child.Status == "completed" {
			var err error
			next, err = p.NextOutcome(a.StageID, *child.Outcome)
			if err != nil {
				failure = "unhandled_child_outcome"
			}
		} else {
			failure = "child_failed"
		}
		var errorEvent local.EventInput
		if failure != "" {
			a.Status = "failed"
			if err := r.failInvocation(a.InvocationID, obs); err != nil {
				return local.Change{}, err
			}
			if err := recordDiagnostic(r, Diagnostic{ID: derivedID("diagnostic", a.ID, failure), RunID: r.ID, ActivationID: a.ID, Origin: "core", Severity: "error", Code: failure, Category: "workflow", Phase: "call", Message: "Child workflow did not return a handled outcome", Observed: obs, CauseRefs: []string{child.ID}}); err != nil {
				return local.Change{}, err
			}
			if failure == "child_failed" {
				var handled bool
				var err error
				errorEvent, handled, err = routeKnownError(r, p, a.ID, "", failure, obs)
				if err != nil {
					return local.Change{}, err
				}
				if handled {
					next = r.readyFor(a.InvocationID)[0]
				}
			}
		} else if err := r.advanceInvocation(a.InvocationID, next); err != nil {
			return local.Change{}, err
		}
		data, err := canonical(map[string]any{"stage_activation_id": a.ID, "workflow_invocation_id": child.ID, "parent_invocation_id": a.InvocationID, "status": child.Status, "outcome": child.Outcome, "output_refs": child.Outputs, "next_stage_id": next, "failure": failure, "observation": obs})
		change := local.Change{RequireStorageBudget: next != "", Events: []local.EventInput{{Type: "stage.call_returned", Version: 1, Data: data}}}
		if errorEvent.Type != "" {
			change.Events = append(change.Events, errorEvent)
		}
		return change, err
	})
	return err
}

func (r Run) pendingCancellation() string {
	if r.CancelRequested {
		return r.RootInvocationID
	}
	ids := make([]string, 0, len(r.Invocations))
	for id, inv := range r.Invocations {
		if inv.CancelRequested && !invocationTerminal(inv.Status) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	if len(ids) != 0 {
		return ids[0]
	}
	return ""
}

func (r Run) blockedChild() string {
	ids := []string{}
	for id, inv := range r.Invocations {
		if inv.Status == "cancelled" && inv.CallerActivationID != "" {
			caller := r.Activations[inv.CallerActivationID]
			if caller != nil && caller.Settled == nil && r.currentBody(caller) == inv {
				ids = append(ids, id)
			}
		}
	}
	slices.Sort(ids)
	if len(ids) != 0 {
		return ids[0]
	}
	return ""
}

func (e *Engine) invocationRun(ctx context.Context, id string) (string, error) {
	// ponytail: bounded local-owner scan (1000 Runs / 64 MiB); add an ownership
	// index when the qualified installation capacity grows, not a second journal.
	snapshots, _, err := e.Store.ReadAll(ctx, TelemetryMaxRuns)
	if err != nil {
		return "", err
	}
	var owner string
	bytes := 0
	for _, snapshot := range snapshots {
		bytes += len(snapshot.Data)
		if bytes > TelemetryMaxBytes {
			return "", fault("scope_lookup_limit", "local snapshot budget exceeded")
		}
		var r Run
		if err := decodeState(snapshot.Data, &r); err != nil {
			return "", err
		}
		if !supportedRun(r) || r.AuthorityID != e.Installation.ID || r.ProjectID != e.Config.ID {
			return "", local.ErrIntegrity
		}
		if r.Invocations[id] != nil {
			if owner != "" {
				return "", local.ErrIntegrity
			}
			owner = r.ID
		}
	}
	if owner == "" {
		return "", local.ErrNotFound
	}
	return owner, nil
}
