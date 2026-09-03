package flow

import (
	"slices"
	"strconv"
)

// RepeatResult is a pure post-body routing calculation, not an execution
// admission. UntilTruth is empty if until was skipped or failed; callers retain
// UntilEvaluated to distinguish these cases without inventing an unknown value.
type RepeatResult struct {
	Route          string
	NextStageID    string
	UntilEvaluated bool
	UntilTruth     Truth
}

// SelectRepeat receives the one-based index and accepted outcome of the current
// completed body. The runtime binds iteration_output to that exact invocation,
// records the decision, and atomically admits any next body. Failed, cancelled
// or uncertain work must not be presented here as a completed outcome.
func (p *Plan) SelectRepeat(stageID string, iteration int64, outcome string, resolve func(FieldRef) (any, bool, error)) (RepeatResult, error) {
	stage, exists := p.Workflow.Definition.Stages[stageID]
	if !exists {
		return RepeatResult{}, problem("invalid_stage", "/definition/stages/"+escapePointer(stageID), "expected a compiled repeat stage")
	}
	return p.SelectRepeatWithLimit(stageID, iteration, outcome, stage.MaxIterations, resolve)
}

// SelectRepeatWithLimit applies a Run-pinned limit that was already checked to
// be within the author's declared bound. The compiler always costs the bound,
// never this narrower value, so a project setting cannot enlarge admission.
func (p *Plan) SelectRepeatWithLimit(stageID string, iteration int64, outcome string, limit int64, resolve func(FieldRef) (any, bool, error)) (RepeatResult, error) {
	result := RepeatResult{}
	path := "/definition/stages/" + escapePointer(stageID)
	if p.Profile != CoreProfile {
		return result, problem("unsupported", path, "repeat requires "+CoreProfile)
	}
	stage, exists := p.Workflow.Definition.Stages[stageID]
	body := p.BodyPlan(stageID)
	if !exists || stage.Kind != "repeat" || body == nil {
		return result, problem("invalid_stage", path, "expected a compiled repeat stage")
	}
	if limit < 1 || limit > stage.MaxIterations {
		return result, problem("invalid_configuration", path+"/limit_configuration", "repeat configuration must be a positive integer within max_iterations")
	}
	if iteration < 1 || iteration > limit {
		return result, problem("invalid_iteration", path+"/max_iterations", "body iteration is outside the pinned bound")
	}
	if !slices.Contains(body.Workflow.AllowedOutcomes, outcome) {
		return result, problem("invalid_outcome", path+"/on_complete", "outcome is not declared by the body workflow")
	}
	complete := func() (RepeatResult, error) {
		next, exists := stage.OnComplete[outcome]
		if !exists || next == "" {
			return result, problem("unhandled_outcome", path+"/on_complete", "completed body has no declared outcome route")
		}
		result.Route, result.NextStageID = "on_complete", next
		return result, nil
	}
	if !slices.Contains(stage.ContinueOn, outcome) {
		return complete()
	}
	result.UntilEvaluated = true
	nodes := 0
	truth, err := evaluatePredicate(stage.Until, resolve, path+"/until", 1, &nodes)
	if err != nil {
		return result, err
	}
	result.UntilTruth = truth
	if truth == TruthTrue {
		return complete()
	}
	if truth == TruthUnknown {
		if stage.OnUnknown == "" {
			return result, problem("condition_unknown", path+"/on_unknown", "unknown condition has no declared route")
		}
		result.Route, result.NextStageID = "on_unknown", stage.OnUnknown
		return result, nil
	}
	if iteration == limit {
		if stage.OnLimit == "" {
			return result, problem("no_transition", path+"/on_limit", "iteration limit has no declared route")
		}
		result.Route, result.NextStageID = "on_limit", stage.OnLimit
		return result, nil
	}
	result.Route = "continue"
	return result, nil
}

func requiredForAll(port OutputPort, outcomes []string) bool {
	if len(outcomes) == 0 {
		return false
	}
	for _, outcome := range outcomes {
		if !slices.Contains(port.RequiredFor, outcome) {
			return false
		}
	}
	return true
}

func (p *Plan) checkRepeatInitialBinding(binding Binding, target Port, required bool, body *Plan, stageID string, available map[outputKey]bool, path string) error {
	if binding.From == "subscription" {
		return p.checkSubscriptionBinding(binding, target, body, stageID, true, path)
	}
	if binding.From == "publication" {
		return problem("invalid_publication_source", path+"/from", "an assigned publication can feed only a call")
	}
	return p.checkAvailableBinding(binding, target, required, available, path)
}

func (p *Plan) checkRepeatBinding(binding Binding, target Port, required bool, body *Plan, stageID string, outcomes []string, available map[outputKey]bool, path string) error {
	if binding.From == "subscription" {
		return p.checkSubscriptionBinding(binding, target, body, stageID, false, path)
	}
	if binding.From == "publication" {
		return problem("invalid_publication_source", path+"/from", "an assigned publication can feed only a call")
	}
	if binding.From != "iteration_output" {
		return p.checkAvailableBinding(binding, target, required, available, path)
	}
	output, exists := body.Workflow.Outputs[binding.Port]
	if !exists {
		return problem("unknown_port", path+"/port", "iteration output is not declared by the body workflow")
	}
	return p.checkBoundPort(binding, output.Port, target, required, requiredForAll(output, outcomes), path)
}

func (p *Plan) checkSubscriptionBinding(binding Binding, target Port, body *Plan, stageID string, initial bool, path string) error {
	if p.Workflow.SchemaVersion != "3" || p.Workflow.Definition.Stages[stageID].Kind != "repeat" {
		return problem("unsupported", path+"/from", "subscription bindings require WorkflowRevision v3 repeat")
	}
	if binding.SourceRef == nil {
		return problem("invalid_publication_source", path+"/source_ref", "subscription binding names its source")
	}
	source, exists := body.PublicationSource(*binding.SourceRef)
	if !exists || source.Mode != "each_publication" {
		return problem("invalid_publication_source", path+"/source_ref", "subscription binding must name the repeat body's each_publication source")
	}
	if source.HandleSchemaRef == nil || source.CursorSchemaRef == nil {
		return problem("invalid_publication_source", path+"/source_ref", "subscription source lacks transport schemas")
	}
	want := *source.HandleSchemaRef
	if binding.Port == "cursor" {
		if !initial {
			return problem("invalid_publication_source", path+"/port", "next cursor must come from iteration_output")
		}
		want = *source.CursorSchemaRef
	} else if binding.Port != "handle" {
		return problem("unknown_port", path+"/port", "subscription exposes only handle and cursor")
	}
	if target.Format != "json" || target.SchemaRef == nil || *target.SchemaRef != want {
		return problem("port_type_mismatch", path, "subscription transport requires its exact schema")
	}
	return nil
}

// checkStreamRepeat ties the author-visible lowering together. Without this
// check independently valid bindings could name two different subscriptions.
func (p *Plan) checkStreamRepeat(stage Stage, stageID string, body *Plan, path string) error {
	var wait Stage
	var waitID string
	var source PublicationSourceDefinition
	count := 0
	for _, candidateID := range keys(body.Workflow.Definition.Stages) {
		candidate := body.Workflow.Definition.Stages[candidateID]
		definition, ok := body.PublicationSource(candidate.SourceRef)
		if candidate.Kind == "wait" && ok && definition.Mode == "each_publication" {
			wait, waitID, source, count = candidate, candidateID, definition, count+1
		}
	}
	if count == 0 {
		return nil
	}
	if count != 1 || wait.CorrelationInput == nil || wait.CursorInput == nil || wait.CorrelationInput.From != "workflow_input" || wait.CursorInput.From != "workflow_input" {
		return problem("invalid_publication_source", path+"/body_workflow_ref", "each_publication repeat body requires one wait over its handle and cursor inputs")
	}
	handleName, cursorName := wait.CorrelationInput.Port, wait.CursorInput.Port
	initialHandle, initialCursor := stage.InitialBindings[handleName], stage.InitialBindings[cursorName]
	nextHandle, nextCursor := stage.NextBindings[handleName], stage.NextBindings[cursorName]
	if initialHandle.From != "subscription" || initialHandle.SourceRef == nil || *initialHandle.SourceRef != wait.SourceRef || initialHandle.Port != "handle" || initialCursor.From != "subscription" || initialCursor.SourceRef == nil || *initialCursor.SourceRef != wait.SourceRef || initialCursor.Port != "cursor" {
		return problem("invalid_publication_source", path+"/initial_bindings", "first iteration receives the exact subscription handle and cursor")
	}
	if stage.MaxIterations > 1 && (nextHandle.From != "subscription" || nextHandle.SourceRef == nil || *nextHandle.SourceRef != wait.SourceRef || nextHandle.Port != "handle" || nextCursor.From != "iteration_output") {
		return problem("invalid_publication_source", path+"/next_bindings", "next iteration reuses the handle and reads cursor from iteration_output")
	}
	if output, ok := body.Workflow.Outputs[nextCursor.Port]; stage.MaxIterations > 1 && (!ok || output.Format != "json" || output.SchemaRef == nil || source.CursorSchemaRef == nil || *output.SchemaRef != *source.CursorSchemaRef) {
		return problem("port_type_mismatch", path+"/next_bindings/"+escapePointer(cursorName), "next cursor output requires the exact cursor schema")
	}
	choiceID := wait.OnEvent
	choice, exists := body.Workflow.Definition.Stages[choiceID]
	if choiceID == "" || wait.OnTimeout != choiceID || !exists || choice.Kind != "choice" {
		return problem("invalid_publication_source", path+"/body_workflow_ref", "stream event and timeout must enter one tagged delivery choice")
	}
	targets := map[string]string{}
	for i, branch := range choice.Branches {
		kind := ""
		for _, candidate := range []string{"Item", "Closed", "Interrupted"} {
			if body.deliveryPredicateStage(branch.Predicate, candidate) == waitID {
				kind = candidate
				break
			}
		}
		if kind == "" || targets[kind] != "" {
			return problem("invalid_publication_source", path+"/body_workflow_ref@"+body.Workflow.ID+"/definition/stages/"+escapePointer(choiceID)+"/branches/"+strconv.Itoa(i)+"/predicate", "stream choice branches match each delivery tag at most once")
		}
		targets[kind] = branch.Next
	}
	if targets["Item"] == "" || targets["Closed"] == "" || targets["Interrupted"] == "" && choice.Default == "" {
		return problem("invalid_publication_source", path+"/body_workflow_ref", "stream choice handles Item, Closed and Interrupted separately")
	}
	interruptedTarget := targets["Interrupted"]
	if interruptedTarget == "" {
		interruptedTarget = choice.Default
	}
	if interruptedTarget == targets["Closed"] {
		return problem("invalid_publication_source", path+"/body_workflow_ref", "Interrupted must not use the Closed route")
	}
	publicationBindings := 0
	for candidateID, candidate := range body.Workflow.Definition.Stages {
		for name, binding := range candidate.InputBindings {
			if binding.From != "publication" {
				continue
			}
			publicationBindings++
			if candidateID != targets["Item"] || binding.StageID != waitID {
				return problem("invalid_publication_source", path+"/body_workflow_ref@"+body.Workflow.ID+"/definition/stages/"+escapePointer(candidateID)+"/input_bindings/"+escapePointer(name), "the assigned publication has exactly one consumer call")
			}
		}
	}
	consumer := body.Workflow.Definition.Stages[targets["Item"]]
	if consumer.Kind != "call" || publicationBindings != 1 {
		return problem("invalid_publication_source", path+"/body_workflow_ref", "the Item branch calls one consumer with the exact assigned publication")
	}
	closed := body.Workflow.Definition.Stages[targets["Closed"]]
	if closed.Kind != "finish" || slices.Contains(stage.ContinueOn, closed.Outcome) {
		return problem("invalid_publication_source", path+"/body_workflow_ref", "the Closed branch finishes without a consumer or another iteration")
	}
	if stage.MaxIterations > 1 {
		for finishID, finish := range body.Workflow.Definition.Stages {
			if finish.Kind != "finish" || !slices.Contains(stage.ContinueOn, finish.Outcome) {
				continue
			}
			binding := finish.OutputBindings[nextCursor.Port]
			if binding.From != "stage_output" || binding.StageID != waitID || binding.Port != WaitEventPort || binding.Pointer == nil || *binding.Pointer != "/next_cursor" || binding.ProjectedSchemaRef == nil || source.CursorSchemaRef == nil || *binding.ProjectedSchemaRef != *source.CursorSchemaRef {
				return problem("invalid_publication_source", path+"/body_workflow_ref@"+body.Workflow.ID+"/definition/stages/"+escapePointer(finishID)+"/output_bindings/"+escapePointer(nextCursor.Port), "a continuing delivery returns the authority next_cursor")
			}
		}
	}
	return nil
}

func (p *Plan) checkRepeatFieldRef(ref FieldRef, body *Plan, available map[outputKey]bool, path string) error {
	if ref.From != "iteration_output" {
		return p.checkFieldRef(ref, available, path)
	}
	output, exists := body.Workflow.Outputs[ref.Port]
	if !exists {
		return problem("unknown_port", path+"/port", "condition iteration output is not declared by the body workflow")
	}
	if output.Format != "json" || output.SchemaRef == nil {
		return problem("condition_type_mismatch", path, "condition fields require a JSON port")
	}
	if _, err := pointerParts(ref.Pointer); err != nil {
		return problem("invalid_pointer", path+"/pointer", err.Error())
	}
	return nil
}

// repeatCost excludes the controller's initial activation (charged by the
// caller). Each prior body and its post-body decision is counted once, followed
// by the selected last body and decision. No compiler work scales with n.
func (p *Plan) repeatCost(stage Stage, body *Plan, route, outcome string) (executionCost, error) {
	continuing := executionCost{}
	for _, outcome := range stage.ContinueOn {
		cost, exists := body.bounds.outcomes[outcome]
		if !exists {
			cost = body.bounds.prefix // Keep declared but unused outcomes conservative.
		}
		continuing = maxCost(continuing, cost)
	}
	priorBody, err := p.addCost(continuing, executionCost{transitions: 1})
	if err != nil {
		return executionCost{}, err
	}
	prior := executionCost{
		steps:       saturatedProduct(priorBody.steps, stage.MaxIterations-1, p.Workflow.Limits.MaxStepInstances),
		transitions: saturatedProduct(priorBody.transitions, stage.MaxIterations-1, p.Workflow.Limits.MaxControlTransitions),
	}
	last := body.bounds.prefix
	switch route {
	case "on_complete":
		if cost, exists := body.bounds.outcomes[outcome]; exists {
			last = cost
		}
	case "on_limit", "on_unknown":
		last = continuing
	}
	last, err = p.addCost(last, executionCost{transitions: 1})
	if err != nil {
		return executionCost{}, err
	}
	return p.addCost(prior, last)
}

func saturatedProduct(value, count, limit int64) int64 {
	if count == 0 || value == 0 {
		return 0
	}
	if value > limit/count {
		return limit + 1
	}
	return value * count
}
