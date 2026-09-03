package runtime

import (
	"context"
	"errors"
	"slices"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

const ChoiceDecisionVersion = "choice-decision/1"

// ChoiceDecision is a core control fact, not a worker result or a new input.
// Its event and the selected transition commit together. Recovery reads that
// state; it never reevaluates a settled choice or reads an author's source file.
type ChoiceDecision struct {
	SchemaVersion string             `json:"schema_version"`
	ID            string             `json:"id"`
	RunID         string             `json:"run_id"`
	InvocationID  string             `json:"workflow_invocation_id"`
	ActivationID  string             `json:"stage_activation_id"`
	StageID       string             `json:"stage_id"`
	WorkflowRef   flow.Ref           `json:"workflow_ref"`
	Selection     string             `json:"selection"`
	Evaluations   []ChoiceEvaluation `json:"evaluations"`
	Inputs        []ChoiceInput      `json:"inputs"`
	Route         string             `json:"route"`
	BranchID      string             `json:"branch_id,omitempty"`
	NextStageID   string             `json:"next_stage_id,omitempty"`
	Failure       string             `json:"failure,omitempty"`
	FailurePath   string             `json:"failure_path,omitempty"`
	Observed      Observation        `json:"observation"`
}

// The trace includes every branch in pinned order. Error/not_evaluated are
// trace states, not additional values in the predicate's three-valued logic.
type ChoiceEvaluation struct {
	BranchID string `json:"branch_id"`
	Result   string `json:"result"`
}

type ChoiceInput struct {
	FieldRef             flow.FieldRef `json:"field_ref"`
	SourceRef            *ArtifactRef  `json:"source_ref,omitempty"`
	ProducerActivationID string        `json:"producer_activation_id,omitempty"`
	Availability         string        `json:"availability"`
}

type choiceSourceValue struct {
	artifact Artifact
	value    any
}

func (e *Engine) evaluateChoice(r Run, p *flow.Plan, stageID string) ChoiceDecision {
	return e.evaluateChoiceFor(r, p, r.RootInvocationID, stageID)
}

func (e *Engine) evaluateChoiceFor(r Run, p *flow.Plan, invocationID, stageID string) ChoiceDecision {
	stage := p.Workflow.Definition.Stages[stageID]
	d := ChoiceDecision{SchemaVersion: ChoiceDecisionVersion, RunID: r.ID, InvocationID: invocationID, StageID: stageID, WorkflowRef: planRef(p), Selection: stage.Selection, Evaluations: []ChoiceEvaluation{}, Inputs: []ChoiceInput{}}
	if a := r.activationForInvocation(invocationID, stageID); a != nil {
		d.ActivationID = a.ID
	}
	d.ID = derivedID("decision", r.ID, d.ActivationID)
	sources := map[ArtifactRef]choiceSourceValue{}
	seen := map[flow.FieldRef]bool{}
	var readBytes int64
	resolve := func(ref flow.FieldRef) (any, bool, error) {
		input, value, present, err := e.choiceSource(r, p, invocationID, ref, &readBytes, sources)
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
	result, err := p.SelectChoice(stageID, resolve)
	for i, branch := range stage.Branches {
		trace := ChoiceEvaluation{BranchID: branch.ID, Result: "not_evaluated"}
		if i < len(result.Evaluations) {
			trace.Result = string(result.Evaluations[i].Result)
		} else if branch.ID == result.ErrorBranchID {
			trace.Result = "error"
		}
		d.Evaluations = append(d.Evaluations, trace)
	}
	d.Route, d.BranchID, d.NextStageID = result.Route, result.BranchID, result.NextStageID
	if err != nil {
		d.Route, d.Failure = "failed", "condition_input_invalid"
		var problem *flow.Problem
		if errors.As(err, &problem) {
			d.Failure, d.FailurePath = problem.Code, problem.Path
		}
		// Unknown is not a failed predicate and on_error is not an implicit
		// default. These two missing routes must remain explicit failures.
		if d.Failure != "condition_unknown" && d.Failure != "no_transition" {
			if next, routeErr := p.NextError(stageID); routeErr == nil {
				d.Route, d.NextStageID = "on_error", next
			}
		}
	}
	return d
}

// choiceSource reads accepted immutable content against the pinned schema,
// once per source. The aggregate byte bound also bounds the retained JSON cache.
func (e *Engine) choiceSource(r Run, p *flow.Plan, invocationID string, ref flow.FieldRef, readBytes *int64, sources map[ArtifactRef]choiceSourceValue) (ChoiceInput, any, bool, error) {
	return e.conditionSource(r, p, invocationID, "", ref, readBytes, sources)
}

func (e *Engine) conditionSource(r Run, p *flow.Plan, invocationID, bodyID string, ref flow.FieldRef, readBytes *int64, sources map[ArtifactRef]choiceSourceValue) (ChoiceInput, any, bool, error) {
	input := ChoiceInput{FieldRef: ref}
	invalid := func(code string) (ChoiceInput, any, bool, error) {
		return input, nil, false, &flow.Problem{Code: code, Message: "Condition input cannot be read against its pinned contract"}
	}
	var port flow.Port
	var required bool
	switch ref.From {
	case "workflow_input":
		declared, ok := p.Workflow.Inputs[ref.Port]
		if !ok || ref.StageID != "" {
			return invalid("condition_input_invalid")
		}
		port, required = declared.Port, declared.Required
	case "stage_output":
		declared, ok := p.StageOutputs(ref.StageID)[ref.Port]
		if !ok {
			return invalid("condition_input_invalid")
		}
		port = declared.Port
		if a := r.activationForInvocation(invocationID, ref.StageID); a != nil {
			input.ProducerActivationID = a.ID
			if a.Kind == "call" || a.Kind == "repeat" {
				child := r.currentBody(a)
				if child == nil {
					if a.Status != "failed" {
						return invalid("condition_input_invalid")
					}
				} else {
					required = a.Status == "completed" && child.Status == "completed" && child.Outcome != nil && slices.Contains(declared.RequiredFor, *child.Outcome)
				}
			} else if a.Kind == "wait" {
				if a.InvocationID != invocationID || a.Wait == nil || a.Status != "completed" || a.Wait.EventRef == nil {
					return invalid("condition_input_invalid")
				}
				route := "on_event"
				if a.Wait.Resolution == "interrupted" {
					route = "on_timeout"
				}
				required = slices.Contains(declared.RequiredFor, route)
			} else {
				s := r.Steps[a.StepID]
				if a.Kind != "step" || a.InvocationID != invocationID || s == nil || s.ActivationID != a.ID {
					return invalid("condition_input_invalid")
				}
				required = s.Status == "completed" && slices.Contains(declared.RequiredFor, s.Verdict)
			}
		}
	case "iteration_output":
		body, err := r.iterationBody(invocationID, bodyID)
		if err != nil || ref.StageID != "" {
			return invalid("condition_input_invalid")
		}
		bodyPlan := p.BodyPlan(r.Activations[body.CallerActivationID].StageID)
		if bodyPlan == nil || planRef(bodyPlan) != body.WorkflowRef {
			return invalid("condition_input_invalid")
		}
		declared, ok := bodyPlan.Workflow.Outputs[ref.Port]
		if !ok {
			return invalid("condition_input_invalid")
		}
		port, required = declared.Port, slices.Contains(declared.RequiredFor, *body.Outcome)
	default:
		return invalid("condition_input_invalid")
	}
	if port.Format != "json" || port.SchemaRef == nil {
		return invalid("condition_type_mismatch")
	}
	refs, err := bindingRefsForBody(r, invocationID, bodyID, map[string]flow.Binding{"source": {From: ref.From, StageID: ref.StageID, Port: ref.Port}})
	if err != nil {
		return invalid("condition_input_invalid")
	}
	artifactRef, present := refs["source"]
	if !present {
		if required {
			return invalid("condition_input_invalid")
		}
		return input, nil, false, nil
	}
	input.SourceRef = &artifactRef
	if cached, ok := sources[artifactRef]; ok {
		if cached.artifact.Format != port.Format || !sameRef(cached.artifact.SchemaRef, port.SchemaRef) {
			return invalid("condition_type_mismatch")
		}
		return input, cached.value, true, nil
	}
	artifact, data, err := e.Artifact(artifactRef)
	if err != nil {
		return invalid("condition_input_unavailable")
	}
	*readBytes += int64(len(data))
	if *readBytes > MaxArtifactBytes {
		return invalid("condition_input_limit")
	}
	if err := e.validatePortArtifact(p, port, artifact, data); err != nil {
		var problem *flow.Problem
		if errors.As(err, &problem) {
			return input, nil, false, err
		}
		return invalid("condition_type_mismatch")
	}
	value, err := flow.Parse(data, "json")
	if err == nil {
		sources[artifactRef] = choiceSourceValue{artifact, value}
	}
	return input, value, err == nil, err
}

func (e *Engine) commitChoice(ctx context.Context, loaded Run, view local.ReadView, p *flow.Plan, activation *Activation, commandID string, decision ChoiceDecision) (local.ApplyResult, error) {
	return e.apply(ctx, e.owner, commandID, loaded.ID, "stage.choice_decided", decision, &view.Snapshot.Version, local.CommandCAS, func(r *Run, s local.Snapshot, obs Observation) (local.Change, error) {
		kind, stageID := nextKind(*r)
		invocationID, _ := r.readyScope()
		if activation == nil || kind != "stage" || stageID != decision.StageID || invocationID != activation.InvocationID {
			return local.Change{}, local.Reject("stage_conflict", "choice no longer owns a ready stage")
		}
		a := r.Activations[activation.ID]
		stage := p.Workflow.Definition.Stages[stageID]
		workflowRef := r.WorkflowRef
		if isInvocationState(r.SchemaVersion) {
			workflowRef = r.Invocations[invocationID].WorkflowRef
		}
		if r.Profile != flow.CoreProfile || p.Profile != r.Profile || planRef(p) != workflowRef || a == nil || a.Kind != "choice" || a.Status != "ready" || a.Settled != nil || a.StepID != "" || a.StageID != stageID || a.InvocationID != invocationID || stage.Kind != "choice" {
			return local.Change{}, local.Reject("stage_conflict", "choice activation or pinned plan differs")
		}
		if decision.SchemaVersion != ChoiceDecisionVersion || decision.RunID != r.ID || decision.WorkflowRef != workflowRef || decision.ActivationID != a.ID || decision.InvocationID != a.InvocationID || decision.ID != derivedID("decision", r.ID, a.ID) || decision.Selection != stage.Selection || decision.Observed != (Observation{}) {
			return local.Change{}, local.Reject("stage_conflict", "decision does not belong to this activation")
		}
		if err := checkChoiceRoute(stage, decision); err != nil {
			return local.Change{}, err
		}
		decision.Observed = obs
		a.Status, a.Settled = "completed", &obs
		var errorEvent local.EventInput
		if decision.Failure != "" {
			a.Status = "failed"
			if err := r.failInvocation(invocationID, obs); err != nil {
				return local.Change{}, err
			}
			if err := recordDiagnostic(r, Diagnostic{ID: derivedID("diagnostic", decision.ID, decision.Failure), RunID: r.ID, ActivationID: a.ID, Origin: "core", Severity: "error", Code: decision.Failure, Category: "workflow", Phase: "choice", Message: "Choice failed; its decision records the ordered branch trace and input references", Observed: obs, CauseRefs: []string{decision.ID}}); err != nil {
				return local.Change{}, err
			}
			if decision.Route == "on_error" {
				var handled bool
				var err error
				errorEvent, handled, err = routeKnownError(r, p, a.ID, "", decision.Failure, obs)
				if err != nil {
					return local.Change{}, err
				}
				ready := r.readyFor(invocationID)
				if !handled || len(ready) != 1 || ready[0] != decision.NextStageID {
					return local.Change{}, local.Reject("stage_conflict", "decision error route differs from pinned plan")
				}
			}
		} else if err := r.advanceInvocation(invocationID, decision.NextStageID); err != nil {
			return local.Change{}, err
		}
		data, err := canonical(decision)
		if err != nil {
			return local.Change{}, err
		}
		change := local.Change{RequireStorageBudget: decision.NextStageID != "", Events: []local.EventInput{{Type: "stage.choice_decided", Version: local.EventVersion, Data: data}}}
		if decision.Route == "on_error" {
			change.Events = append(change.Events, errorEvent)
		}
		return change, nil
	})
}

func checkChoiceRoute(stage flow.Stage, d ChoiceDecision) error {
	valid := false
	switch d.Route {
	case "branch":
		for _, branch := range stage.Branches {
			valid = valid || d.BranchID == branch.ID && d.NextStageID == branch.Next && d.Failure == ""
		}
	case "default":
		valid = stage.Default != "" && d.NextStageID == stage.Default && d.BranchID == "" && d.Failure == ""
	case "on_unknown":
		valid = stage.OnUnknown != "" && d.NextStageID == stage.OnUnknown && d.BranchID == "" && d.Failure == ""
	case "on_error":
		valid = stage.OnError != "" && d.NextStageID == stage.OnError && d.BranchID == "" && d.Failure != "" && d.Failure != "condition_unknown" && d.Failure != "no_transition"
	case "failed":
		valid = d.NextStageID == "" && d.BranchID == "" && d.Failure != ""
	}
	if !valid {
		return local.Reject("stage_conflict", "decision route is not declared by this choice")
	}
	return nil
}
