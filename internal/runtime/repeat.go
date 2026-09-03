package runtime

import (
	"context"
	"errors"
	"slices"
	"strconv"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

func repeatBodyID(runID, activationID string, iteration int64) string {
	return derivedID("invocation", runID, activationID, strconv.FormatInt(iteration, 10))
}

func repeatDecisionID(runID, activationID string, iteration int64) string {
	return derivedID("decision", runID, activationID, strconv.FormatInt(iteration, 10))
}

// iteration_output is only available while deciding the exact current body's
// accepted result. It is not a search through older iterations or other Runs.
func (r Run) iterationBody(parentID, bodyID string) (*Invocation, error) {
	body := r.Invocations[bodyID]
	if !isRepeatState(r.SchemaVersion) || body == nil || body.ParentInvocationID != parentID || body.Status != "completed" || body.Outcome == nil || body.Settled == nil {
		return nil, local.ErrIntegrity
	}
	caller := r.Activations[body.CallerActivationID]
	if caller == nil || caller.Kind != "repeat" || caller.Status != "waiting" || caller.Settled != nil || r.currentBody(caller) != body || !r.childMatchesCaller(body, caller) {
		return nil, local.ErrIntegrity
	}
	return body, nil
}

func checkRepeatActivation(r Run, p *flow.Plan, a *Activation, status string) error {
	kind, stageID := nextKind(r)
	parentID, _ := r.readyScope()
	if a == nil || !isRepeatState(r.SchemaVersion) || r.Profile != flow.CoreProfile || p.Profile != r.Profile || kind != "stage" || a.ID == "" || a.InvocationID != parentID || a.StageID != stageID || a.Kind != "repeat" || a.Status != status || a.Settled != nil || a.StepID != "" || a.Repeat == nil {
		return local.Reject("repeat_blocked", "repeat no longer owns its ready scope")
	}
	parent := r.Invocations[parentID]
	if parent == nil || parent.WorkflowRef != planRef(p) || p.Workflow.Definition.Stages[stageID].Kind != "repeat" || p.Repeats[stageID] == nil {
		return local.Reject("stage_conflict", "repeat activation or pinned body differs")
	}
	return nil
}

// Creation is part of entry or a continue decision. The activation already
// paid entry; each post-body decision pays once, not again for the new body.
func (r *Run) createRepeatBody(a *Activation, body *flow.Plan, inputs map[string]ArtifactRef, obs Observation) (local.EventInput, error) {
	iteration := a.Repeat.IterationCount + 1
	id := repeatBodyID(r.ID, a.ID, iteration)
	if inputs == nil || r.Invocations[id] != nil || iteration > 100 {
		return local.EventInput{}, local.ErrIntegrity
	}
	if err := r.setReadyFor(a.InvocationID, []string{}); err != nil {
		return local.EventInput{}, err
	}
	if err := r.setInvocationStatus(a.InvocationID, "waiting", nil); err != nil {
		return local.EventInput{}, err
	}
	a.Status = "waiting"
	r.Invocations[id] = &Invocation{ID: id, RunID: r.ID, ParentInvocationID: a.InvocationID, CallerActivationID: a.ID, Iteration: &iteration, WorkflowRef: planRef(body), Status: "ready", Inputs: inputs, Outputs: map[string]ArtifactRef{}, Ready: []string{body.Workflow.Definition.Entry}, Created: obs}
	a.Repeat.IterationCount, a.Repeat.CurrentBodyInvocationID = iteration, id
	if err := r.beginWorkflowInputAcceptance(body, id, obs); err != nil {
		return local.EventInput{}, err
	}
	data, err := canonical(map[string]any{"workflow_invocation_id": id, "parent_invocation_id": a.InvocationID, "caller_stage_activation_id": a.ID, "workflow_ref": planRef(body), "input_refs": inputs, "iteration": iteration, "observation": obs})
	return local.EventInput{Type: "invocation.created", Version: 1, Data: data}, err
}

func (e *Engine) enterRepeat(ctx context.Context, loaded Run, view local.ReadView, p *flow.Plan, activation *Activation) error {
	if err := checkRepeatActivation(loaded, p, activation, "ready"); err != nil {
		return err
	}
	body := p.Repeats[activation.StageID]
	commandID := newID("command")
	inputs, err := e.prepareRepeatBodyInputs(loaded, activation, "", body, p.Workflow.Definition.Stages[activation.StageID].InitialBindings, commandID)
	if err != nil {
		return e.failPreparation(ctx, loaded, view, p, activation, err, "repeat_input_binding_failed")
	}
	payload := map[string]any{"stage_activation_id": activation.ID, "input_refs": inputs}
	_, err = e.apply(ctx, e.owner, commandID, loaded.ID, "stage.repeat_entered", payload, &view.Snapshot.Version, local.CommandCAS, func(r *Run, s local.Snapshot, obs Observation) (local.Change, error) {
		a := r.Activations[activation.ID]
		if err := checkRepeatActivation(*r, p, a, "ready"); err != nil {
			return local.Change{}, err
		}
		if a.Repeat.IterationCount != 0 || a.Repeat.CurrentBodyInvocationID != "" || a.Repeat.LastDecision != nil {
			return local.Change{}, local.Reject("stage_conflict", "repeat already entered its first body")
		}
		created, err := r.createRepeatBody(a, body, inputs, obs)
		if err != nil {
			return local.Change{}, err
		}
		data, err := canonical(map[string]any{"run_id": r.ID, "stage_activation_id": a.ID, "stage_id": a.StageID, "workflow_invocation_id": a.InvocationID, "body_workflow_invocation_id": a.Repeat.CurrentBodyInvocationID, "body_workflow_ref": planRef(body), "iteration": int64(1), "observation": obs})
		return local.Change{RequireStorageBudget: true, Events: []local.EventInput{{Type: "stage.repeat_entered", Version: 1, Data: data}, created}}, err
	})
	return err
}

func repeatFailure(d *RepeatDecision, p *flow.Plan, cause error, fallback string) {
	d.Route, d.NextStageID, d.NextBodyInvocationID, d.Failure = "failed", "", "", driverFailureCode(cause, fallback)
	var problem *flow.Problem
	if errors.As(cause, &problem) {
		d.Failure, d.FailurePath = problem.Code, problem.Path
	}
	// Missing outcome/unknown routes are not implicit technical error handlers.
	if d.Failure != "condition_unknown" && d.Failure != "unhandled_outcome" && d.Failure != "no_transition" {
		if next, err := p.NextError(d.StageID); err == nil {
			d.Route, d.NextStageID = "on_error", next
		}
	}
}

func (e *Engine) evaluateRepeat(r Run, p *flow.Plan, a *Activation) (RepeatDecision, error) {
	if err := checkRepeatActivation(r, p, a, "waiting"); err != nil {
		return RepeatDecision{}, err
	}
	body := r.currentBody(a)
	if body == nil || body.Settled == nil || body.Status != "completed" && body.Status != "failed" || body.Iteration == nil || *body.Iteration > p.Workflow.Definition.Stages[a.StageID].MaxIterations {
		return RepeatDecision{}, local.Reject("repeat_blocked", "repeat body has no settled result")
	}
	limit, err := repeatLimit(p, r.WorkflowConfigurations[p.Digest], a.StageID)
	if err != nil {
		return RepeatDecision{}, err
	}
	d := RepeatDecision{SchemaVersion: RepeatDecisionVersion, ID: repeatDecisionID(r.ID, a.ID, *body.Iteration), RunID: r.ID, InvocationID: a.InvocationID, ActivationID: a.ID, StageID: a.StageID, WorkflowRef: planRef(p), BodyInvocationID: body.ID, Iteration: *body.Iteration, BodyStatus: body.Status, BodyOutcome: body.Outcome, UntilResult: "not_evaluated", Inputs: []ChoiceInput{}}
	if body.Status == "failed" {
		repeatFailure(&d, p, nil, "child_failed")
		return d, nil
	}
	if body.Outcome == nil || !slices.Contains(p.Repeats[a.StageID].Workflow.AllowedOutcomes, *body.Outcome) {
		return RepeatDecision{}, local.ErrIntegrity
	}
	sources := map[ArtifactRef]choiceSourceValue{}
	seen := map[flow.FieldRef]bool{}
	var readBytes int64
	resolve := func(ref flow.FieldRef) (any, bool, error) {
		input, value, present, err := e.conditionSource(r, p, a.InvocationID, body.ID, ref, &readBytes, sources)
		if err == nil && present {
			value, present = flow.JSONPointer(value, ref.Pointer)
		}
		if !seen[ref] {
			switch {
			case err != nil:
				input.Availability = "unavailable"
			case present:
				input.Availability = "present"
			default:
				input.Availability = "absent"
			}
			d.Inputs = append(d.Inputs, input)
			seen[ref] = true
		}
		return value, present, err
	}
	result, err := p.SelectRepeatWithLimit(a.StageID, *body.Iteration, *body.Outcome, limit, resolve)
	d.Route, d.NextStageID = result.Route, result.NextStageID
	if result.UntilEvaluated {
		d.UntilResult = string(result.UntilTruth)
		if result.UntilTruth == "" {
			d.UntilResult = "error"
		}
	}
	if err != nil {
		repeatFailure(&d, p, err, "condition_input_invalid")
	} else if d.Route == "continue" {
		d.NextBodyInvocationID = repeatBodyID(r.ID, a.ID, d.Iteration+1)
	}
	return d, nil
}

func (e *Engine) decideRepeat(ctx context.Context, r Run, view local.ReadView, p *flow.Plan, a *Activation) error {
	d, err := e.evaluateRepeat(r, p, a)
	if err != nil {
		return err
	}
	commandID := newID("command")
	var inputs map[string]ArtifactRef
	if d.Route == "continue" {
		inputs, err = e.prepareRepeatBodyInputs(r, a, d.BodyInvocationID, p.Repeats[a.StageID], p.Workflow.Definition.Stages[a.StageID].NextBindings, commandID)
		if err == nil {
			err = e.validateRepeatPublicationCursor(r, p, a, inputs)
		}
		if err != nil {
			repeatFailure(&d, p, err, "repeat_input_binding_failed")
			inputs = nil
		}
	}
	_, err = e.commitRepeat(ctx, r, view, p, a, commandID, d, inputs)
	return err
}

func (e *Engine) commitRepeat(ctx context.Context, loaded Run, view local.ReadView, p *flow.Plan, activation *Activation, commandID string, decision RepeatDecision, nextInputs map[string]ArtifactRef) (local.ApplyResult, error) {
	payload := map[string]any{"decision": decision, "next_input_refs": nextInputs}
	return e.apply(ctx, e.owner, commandID, loaded.ID, "stage.repeat_decided", payload, &view.Snapshot.Version, local.CommandCAS, func(r *Run, s local.Snapshot, obs Observation) (local.Change, error) {
		if activation == nil {
			return local.Change{}, local.ErrIntegrity
		}
		a := r.Activations[activation.ID]
		if err := checkRepeatActivation(*r, p, a, "waiting"); err != nil {
			return local.Change{}, err
		}
		body := r.currentBody(a)
		if body == nil || body.Settled == nil || body.Iteration == nil || body.Status != "completed" && body.Status != "failed" || body.WorkflowRef != planRef(p.Repeats[a.StageID]) {
			return local.Change{}, local.Reject("repeat_blocked", "repeat body is not a settled pinned workflow")
		}
		if decision.SchemaVersion != RepeatDecisionVersion || decision.ID != repeatDecisionID(r.ID, a.ID, *body.Iteration) || decision.RunID != r.ID || decision.InvocationID != a.InvocationID || decision.ActivationID != a.ID || decision.StageID != a.StageID || decision.WorkflowRef != planRef(p) || decision.BodyInvocationID != body.ID || decision.Iteration != *body.Iteration || decision.BodyStatus != body.Status || (decision.BodyOutcome == nil) != (body.Outcome == nil) || decision.BodyOutcome != nil && *decision.BodyOutcome != *body.Outcome || decision.Inputs == nil || decision.Observed != (Observation{}) {
			return local.Change{}, local.Reject("stage_conflict", "repeat decision does not belong to this body")
		}
		limit, err := repeatLimit(p, r.WorkflowConfigurations[p.Digest], a.StageID)
		if err != nil {
			return local.Change{}, err
		}
		if err := checkRepeatRoute(p.Workflow.Definition.Stages[a.StageID], limit, decision); err != nil {
			return local.Change{}, err
		}
		if decision.Route == "continue" {
			if decision.NextBodyInvocationID != repeatBodyID(r.ID, a.ID, decision.Iteration+1) || nextInputs == nil {
				return local.Change{}, local.Reject("stage_conflict", "repeat continuation has no exact next body and inputs")
			}
		} else if decision.NextBodyInvocationID != "" || nextInputs != nil {
			return local.Change{}, local.Reject("stage_conflict", "terminal repeat decision creates no body")
		}
		if err := r.chargeInvocation(a.InvocationID, 1, 0); err != nil {
			return local.Change{}, err
		}
		decision.Observed = obs
		if err := settleRepeatPublicationAssignment(r, p, a, body, decision, obs); err != nil {
			return local.Change{}, err
		}
		var created, errorEvent local.EventInput
		if decision.Route == "continue" {
			var err error
			created, err = r.createRepeatBody(a, p.Repeats[a.StageID], nextInputs, obs)
			if err != nil {
				return local.Change{}, err
			}
		} else {
			a.Status, a.Settled = "completed", &obs
			if decision.Failure != "" {
				a.Status = "failed"
				if err := r.failInvocation(a.InvocationID, obs); err != nil {
					return local.Change{}, err
				}
				if err := recordDiagnostic(r, Diagnostic{ID: derivedID("diagnostic", decision.ID, decision.Failure), RunID: r.ID, ActivationID: a.ID, Origin: "core", Severity: "error", Code: decision.Failure, Category: "workflow", Phase: "repeat", Message: "Repeat failed; its decision identifies the completed body and condition inputs", Observed: obs, CauseRefs: []string{decision.ID, body.ID}}); err != nil {
					return local.Change{}, err
				}
				if decision.Route == "on_error" {
					var handled bool
					var err error
					errorEvent, handled, err = routeKnownError(r, p, a.ID, "", decision.Failure, obs)
					if err != nil {
						return local.Change{}, err
					}
					ready := r.readyFor(a.InvocationID)
					if !handled || len(ready) != 1 || ready[0] != decision.NextStageID {
						return local.Change{}, local.Reject("stage_conflict", "repeat error route differs from pinned plan")
					}
				}
			} else if err := r.advanceInvocation(a.InvocationID, decision.NextStageID); err != nil {
				return local.Change{}, err
			}
		}
		a.Repeat.LastDecision = &decision
		data, err := canonical(decision)
		change := local.Change{RequireStorageBudget: decision.Route == "continue" || decision.NextStageID != "", Events: []local.EventInput{{Type: "stage.repeat_decided", Version: 1, Data: data}}}
		if created.Type != "" {
			change.Events = append(change.Events, created)
		}
		if errorEvent.Type != "" {
			change.Events = append(change.Events, errorEvent)
		}
		return change, err
	})
}

func checkRepeatRoute(stage flow.Stage, limit int64, d RepeatDecision) error {
	valid := limit >= 1 && limit <= stage.MaxIterations && d.Iteration >= 1 && d.Iteration <= limit
	continuing := d.BodyOutcome != nil && slices.Contains(stage.ContinueOn, *d.BodyOutcome)
	if d.BodyStatus == "failed" {
		valid = valid && d.BodyOutcome == nil && d.UntilResult == "not_evaluated" && len(d.Inputs) == 0 && d.Failure == "child_failed" && (d.Route == "failed" || d.Route == "on_error")
	} else {
		valid = valid && d.BodyStatus == "completed" && d.BodyOutcome != nil && ((continuing && slices.Contains([]string{"true", "false", "unknown", "error"}, d.UntilResult)) || (!continuing && d.UntilResult == "not_evaluated"))
	}
	switch d.Route {
	case "continue":
		valid = valid && continuing && d.UntilResult == "false" && d.Iteration < limit && d.NextStageID == "" && d.Failure == ""
	case "on_complete":
		valid = valid && d.BodyOutcome != nil && d.NextStageID != "" && d.NextStageID == stage.OnComplete[*d.BodyOutcome] && (d.UntilResult == "true" || !continuing && d.UntilResult == "not_evaluated") && d.Failure == ""
	case "on_limit":
		valid = valid && continuing && d.UntilResult == "false" && d.Iteration == limit && d.NextStageID != "" && d.NextStageID == stage.OnLimit && d.Failure == ""
	case "on_unknown":
		valid = valid && continuing && d.UntilResult == "unknown" && d.NextStageID != "" && d.NextStageID == stage.OnUnknown && d.Failure == ""
	case "on_error":
		valid = valid && d.NextStageID != "" && d.NextStageID == stage.OnError && d.Failure != "" && d.Failure != "condition_unknown" && d.Failure != "unhandled_outcome" && d.Failure != "no_transition"
	case "failed":
		valid = valid && d.NextStageID == "" && d.Failure != ""
	default:
		valid = false
	}
	if !valid || d.Failure == "" && d.FailurePath != "" {
		return local.Reject("stage_conflict", "repeat decision route differs from pinned plan")
	}
	return nil
}
