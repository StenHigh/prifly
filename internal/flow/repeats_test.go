package flow

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
)

func repeatStage(ref Ref, next string, iterations int64) Stage {
	return Stage{Kind: "repeat", BodyWorkflowRef: ref, InitialBindings: map[string]Binding{}, NextBindings: map[string]Binding{}, ContinueOn: []string{"no_work"}, Until: conditionFixturePredicate("false"), MaxIterations: iterations, OnComplete: map[string]string{"no_work": next}, OnLimit: next}
}

func repeatBindingFixture(t *testing.T) (WorkflowRevision, WorkflowRevision, Registry) {
	t.Helper()
	root, child, registry := callBindingFixture(t)
	root.Definition.Entry = "repeat"
	delete(root.Definition.Stages, "call")
	stage := repeatStage(Ref{}, "done", 3)
	stage.InitialBindings["value"] = Binding{From: "workflow_input", Port: "value"}
	stage.NextBindings["value"] = Binding{From: "iteration_output", Port: "report"}
	root.Definition.Stages["repeat"] = stage
	root.Definition.Stages["done"].OutputBindings["report"] = Binding{From: "stage_output", StageID: "repeat", Port: "report"}
	return root, child, registry
}

func compileRepeatFixture(t *testing.T, root, body WorkflowRevision, registry Registry) (*Plan, error) {
	t.Helper()
	stage := root.Definition.Stages["repeat"]
	stage.BodyWorkflowRef = registerCallWorkflow(t, registry, body)
	root.Definition.Stages["repeat"] = stage
	return CompileProfile(encoded(t, root), "json", registry, CoreProfile)
}

func TestCoreRepeatExactBodyAndBaselineWire(t *testing.T) {
	root, body, registry := repeatBindingFixture(t)
	p, err := compileRepeatFixture(t, root, body, registry)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Calls) != 0 || len(p.Repeats) != 1 || p.BodyPlan("repeat") != p.Repeats["repeat"] || p.BodyPlan("done") != nil || len(p.Steps) != 0 {
		t.Fatal("repeat flattened body or confused its definition with an invocation")
	}
	if !slices.Equal(p.Sequence, []string{"repeat", "done"}) || p.StageOutputs("repeat")["report"].SchemaRef == nil || p.bounds.outcomes["no_work"].transitions != 8 {
		t.Fatal("repeat lost its public ports or n+1 control accounting")
	}
	if err := ValidateProtocol("WorkflowRevision", encoded(t, p.Workflow)); err != nil {
		t.Fatal("repeat changed the baseline authoring contract", err)
	}
	if err := ValidateProtocol("RepeatStage", encoded(t, p.Workflow.Definition.Stages["repeat"])); err != nil {
		t.Fatal(err)
	}
	_, err = Compile(encoded(t, p.Workflow), "json", registry)
	expectProblem(t, err, "unsupported")
	ref := p.Workflow.Definition.Stages["repeat"].BodyWorkflowRef
	registry[ref] = []byte(`null`)
	if string(p.Registry[ref]) == "null" || p.BodyPlan("repeat").Digest != ref.Digest {
		t.Fatal("mutable inventory changed an accepted body plan")
	}
	_, err = p.NextOutcome("repeat", "no_work")
	expectProblem(t, err, "invalid_stage")
}

func TestWorkflowRevisionV3OwnsPublicationBindings(t *testing.T) {
	root, _, _ := repeatBindingFixture(t)
	source := Ref{ID: "test:source/documents", Version: "1.0.0", Digest: "sha256:0000000000000000000000000000000000000000000000000000000000000000"}
	body := Ref{ID: "test:workflow/body", Version: "1.0.0", Digest: "sha256:1111111111111111111111111111111111111111111111111111111111111111"}
	stage := root.Definition.Stages["repeat"]
	stage.BodyWorkflowRef = body
	stage.InitialBindings["value"] = Binding{From: "subscription", SourceRef: &source, Port: "handle"}
	stage.NextBindings["value"] = Binding{From: "publication", StageID: "await_document"}
	root.Definition.Stages["repeat"] = stage
	root.SchemaVersion = "3"
	if err := ValidateProtocol("WorkflowRevisionV3", encoded(t, root)); err != nil {
		t.Fatal(err)
	}
	root.SchemaVersion = "2"
	if err := ValidateProtocol("WorkflowRevisionV2", encoded(t, root)); err == nil {
		t.Fatal("WorkflowRevision v2 silently acquired publication bindings")
	}
}

func TestPublicationBindingOnlyFeedsCall(t *testing.T) {
	stage := map[string]any{"kind": "step", "input_bindings": map[string]any{"document": map[string]any{"from": "publication", "stage_id": "await_document"}}}
	workflow := map[string]any{
		"schema_version": "3", "definition": map[string]any{"stages": map[string]any{"consume": stage}},
		"limits": map[string]any{"max_parallelism": json.Number("1"), "max_child_depth": json.Number("0")}, "allowed_outcomes": []any{"succeeded"},
	}
	p := expectProblem(t, supportedWorkflowProfile(workflow, CoreProfile), "unsupported")
	if p.Path != "/definition/stages/consume/input_bindings/document/from" {
		t.Fatalf("publication refusal points elsewhere: %s", p.Path)
	}
	stage["kind"] = "call"
	if err := supportedWorkflowProfile(workflow, CoreProfile); err != nil {
		t.Fatalf("call could not receive an assigned publication: %v", err)
	}
}

func streamRepeatCompilerFixture(t *testing.T) (*Plan, *Plan, Stage) {
	t.Helper()
	digest := "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	ref := func(id string) Ref { return Ref{ID: id, Version: "1.0.0", Digest: digest} }
	itemRef, handleRef, cursorRef, deliveryRef := ref("test:schema/document"), ref("test:schema/subscription-handle"), ref("test:schema/publication-cursor"), ref("test:schema/publication-delivery")
	sourceRef := ref("test:source/documents")
	source := PublicationSourceDefinition{
		SchemaVersion: PublicationStreamSourceVersion, ID: sourceRef.ID, Version: sourceRef.Version, Mode: "each_publication",
		ProducerBranchID: "producer", ProducerStageID: "produce", Hook: "document_created", HookSchemaRef: itemRef,
		HandleSchemaRef: &handleRef, CursorSchemaRef: &cursorRef, DeliverySchemaRef: &deliveryRef,
		Initial: "retained", ProducerFailure: "wait_until_timeout",
	}
	predicate := func(kind string) Predicate {
		return Predicate{Op: "eq",
			Left:  &Operand{Kind: "field", Ref: &FieldRef{From: "stage_output", StageID: "await_document", Port: WaitEventPort, Pointer: "/kind"}},
			Right: &Operand{Kind: "literal", Value: json.RawMessage(`"` + kind + `"`)},
		}
	}
	nextCursor := "/next_cursor"
	body := &Plan{Registry: Registry{sourceRef: encoded(t, source)}, publicationSources: map[Ref]PublicationSourceDefinition{sourceRef: source}}
	body.Workflow = WorkflowRevision{
		SchemaVersion: "3", ID: "test:workflow/publication-body",
		Outputs: map[string]OutputPort{"next_cursor": {Port: Port{Format: "json", SchemaRef: &cursorRef}, RequiredFor: []string{"succeeded"}}},
	}
	body.Workflow.Definition.Stages = map[string]Stage{
		"await_document": {
			Kind: "wait", SourceRef: sourceRef, CorrelationInput: &Binding{From: "workflow_input", Port: "subscription"}, CursorInput: &Binding{From: "workflow_input", Port: "cursor"},
			OnEvent: "delivery", OnTimeout: "delivery",
		},
		"delivery": {
			Kind: "choice", Branches: []ChoiceBranch{{ID: "item", Predicate: predicate("Item"), Next: "consume"}, {ID: "closed", Predicate: predicate("Closed"), Next: "closed"}}, Default: "interrupted",
		},
		"consume": {Kind: "call", InputBindings: map[string]Binding{"document": {From: "publication", StageID: "await_document"}}},
		"item_done": {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]Binding{"next_cursor": {
			From: "stage_output", StageID: "await_document", Port: WaitEventPort, Pointer: &nextCursor, ProjectedSchemaRef: &cursorRef,
		}}},
		"closed":      {Kind: "finish", Outcome: "no_work"},
		"interrupted": {Kind: "finish", Outcome: "rejected"},
	}
	stage := Stage{
		Kind: "repeat", MaxIterations: 3, ContinueOn: []string{"succeeded"},
		InitialBindings: map[string]Binding{
			"subscription": {From: "subscription", SourceRef: &sourceRef, Port: "handle"},
			"cursor":       {From: "subscription", SourceRef: &sourceRef, Port: "cursor"},
		},
		NextBindings: map[string]Binding{
			"subscription": {From: "subscription", SourceRef: &sourceRef, Port: "handle"},
			"cursor":       {From: "iteration_output", Port: "next_cursor"},
		},
	}
	return &Plan{}, body, stage
}

func TestEachPublicationRepeatRequiresExactLowering(t *testing.T) {
	root, body, stage := streamRepeatCompilerFixture(t)
	if err := root.checkStreamRepeat(stage, "publications", body, "/definition/stages/publications"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		edit func(*Plan)
	}{
		{"item_must_call", func(body *Plan) {
			consumer := body.Workflow.Definition.Stages["consume"]
			consumer.Kind = "step"
			body.Workflow.Definition.Stages["consume"] = consumer
		}},
		{"one_consumer", func(body *Plan) {
			body.Workflow.Definition.Stages["consume_again"] = Stage{Kind: "call", InputBindings: map[string]Binding{"document": {From: "publication", StageID: "await_document"}}}
		}},
		{"interrupted_is_not_eof", func(body *Plan) {
			choice := body.Workflow.Definition.Stages["delivery"]
			choice.Default = "closed"
			body.Workflow.Definition.Stages["delivery"] = choice
		}},
		{"explicit_interrupted_is_not_eof", func(body *Plan) {
			choice := body.Workflow.Definition.Stages["delivery"]
			predicate := choice.Branches[0].Predicate
			right := *predicate.Right
			right.Value = json.RawMessage(`"Interrupted"`)
			predicate.Right = &right
			choice.Branches = append(choice.Branches, ChoiceBranch{ID: "interrupted", Predicate: predicate, Next: "closed"})
			choice.Default = ""
			body.Workflow.Definition.Stages["delivery"] = choice
		}},
		{"cursor_comes_from_delivery", func(body *Plan) {
			finish := body.Workflow.Definition.Stages["item_done"]
			binding := finish.OutputBindings["next_cursor"]
			wrong := "/cursor"
			binding.Pointer = &wrong
			finish.OutputBindings["next_cursor"] = binding
			body.Workflow.Definition.Stages["item_done"] = finish
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, body, stage := streamRepeatCompilerFixture(t)
			tc.edit(body)
			expectProblem(t, root.checkStreamRepeat(stage, "publications", body, "/definition/stages/publications"), "invalid_publication_source")
		})
	}
}

func TestCoreRepeatPostBodyDecisionOrder(t *testing.T) {
	root, body, registry := repeatBindingFixture(t)
	body.AllowedOutcomes = []string{"no_work", "rejected"}
	stage := root.Definition.Stages["repeat"]
	stage.Until = Predicate{Op: "eq", Left: &Operand{Kind: "field", Ref: &FieldRef{From: "iteration_output", Port: "report"}}, Right: conditionLiteral("true")}
	stage.OnComplete["rejected"], stage.OnUnknown, stage.OnError = "done", "done", "done"
	// The error route carries no public exports.
	root.Outputs = map[string]OutputPort{}
	root.Definition.Stages["done"] = Stage{Kind: "finish", Outcome: "no_work", OutputBindings: map[string]Binding{}}
	root.Definition.Stages["repeat"] = stage
	p, err := compileRepeatFixture(t, root, body, registry)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, outcome, route, code string
		iteration                  int64
		value                      any
		present, evaluated         bool
		truth                      Truth
	}{
		{"continue", "no_work", "continue", "", 1, false, true, true, TruthFalse},
		{"complete", "no_work", "on_complete", "", 1, true, true, true, TruthTrue},
		{"true_wins_at_limit", "no_work", "on_complete", "", 3, true, true, true, TruthTrue},
		{"false_at_limit", "no_work", "on_limit", "", 3, false, true, true, TruthFalse},
		{"unknown_wins_at_limit", "no_work", "on_unknown", "", 3, nil, false, true, TruthUnknown},
		{"noncontinue_skips_until", "rejected", "on_complete", "", 3, []any{}, true, false, ""},
		{"null_is_not_absence", "no_work", "", "condition_type_mismatch", 1, nil, true, true, ""},
		{"invalid_type", "no_work", "", "condition_type_mismatch", 1, []any{}, true, true, ""},
		{"zero_index", "no_work", "", "invalid_iteration", 0, nil, false, false, ""},
		{"over_limit", "no_work", "", "invalid_iteration", 4, nil, false, false, ""},
		{"not_body_outcome", "pass", "", "invalid_outcome", 1, nil, false, false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reads := 0
			result, err := p.SelectRepeat("repeat", tc.iteration, tc.outcome, func(ref FieldRef) (any, bool, error) {
				reads++
				if ref.From != "iteration_output" || ref.Port != "report" {
					t.Fatal("wrong scoped field", ref)
				}
				return tc.value, tc.present, nil
			})
			if tc.code != "" {
				expectProblem(t, err, tc.code)
			} else if err != nil {
				t.Fatal(err)
			}
			if result.Route != tc.route || result.UntilEvaluated != tc.evaluated || result.UntilTruth != tc.truth || reads != map[bool]int{false: 0, true: 1}[tc.evaluated] {
				t.Fatalf("wrong post-body decision: %+v reads=%d", result, reads)
			}
			if tc.route == "continue" && result.NextStageID != "" || tc.route != "" && tc.route != "continue" && result.NextStageID != "done" {
				t.Fatal("repeat invented or lost an outgoing route", result)
			}
		})
	}
	configured := p.Workflow.Definition.Stages["repeat"]
	configured.OnUnknown = ""
	delete(configured.OnComplete, "rejected")
	p.Workflow.Definition.Stages["repeat"] = configured
	result, err := p.SelectRepeat("repeat", 1, "rejected", func(FieldRef) (any, bool, error) { t.Fatal("noncontinue read until"); return nil, false, nil })
	expectProblem(t, err, "unhandled_outcome")
	if result.Route != "" || result.UntilEvaluated {
		t.Fatal("missing completion route invented a fallback")
	}
	result, err = p.SelectRepeat("repeat", 3, "no_work", func(FieldRef) (any, bool, error) { return nil, false, nil })
	expectProblem(t, err, "condition_unknown")
	if result.Route != "" || result.UntilTruth != TruthUnknown {
		t.Fatal("unknown fell through to limit or error handler")
	}
	sentinel := errors.New("unavailable pinned evidence")
	result, err = p.SelectRepeat("repeat", 1, "no_work", func(FieldRef) (any, bool, error) { return nil, false, sentinel })
	if !errors.Is(err, sentinel) || !result.UntilEvaluated || result.UntilTruth != "" || result.Route != "" {
		t.Fatal("resolver error became a boolean result", result, err)
	}
}

func TestCoreRepeatConfiguredLimitOnlyNarrowsDeclaredBound(t *testing.T) {
	root, body, registry := repeatBindingFixture(t)
	root.SchemaVersion = "2"
	input := root.Inputs["value"]
	input.Required = false
	input.Configuration = &InputConfiguration{Scope: "project"}
	root.Inputs["round_limit"] = input
	stage := root.Definition.Stages["repeat"]
	stage.LimitConfiguration = "round_limit"
	root.Definition.Stages["repeat"] = stage
	p, err := compileRepeatFixture(t, root, body, registry)
	if err != nil {
		t.Fatal(err)
	}
	result, err := p.SelectRepeatWithLimit("repeat", 2, "no_work", 2, func(FieldRef) (any, bool, error) { return false, true, nil })
	if err != nil || result.Route != "on_limit" {
		t.Fatalf("configured limit did not select its own limit route: %+v %v", result, err)
	}
	_, err = p.SelectRepeatWithLimit("repeat", 1, "no_work", 4, func(FieldRef) (any, bool, error) { return false, true, nil })
	expectProblem(t, err, "invalid_configuration")
	for name, edit := range map[string]func(*WorkflowRevision, *Stage){
		"unknown": func(_ *WorkflowRevision, s *Stage) { s.LimitConfiguration = "missing" },
		"run_scope": func(w *WorkflowRevision, _ *Stage) {
			input := w.Inputs["round_limit"]
			input.Configuration = &InputConfiguration{Scope: "run", Default: input.Configuration.Default}
			w.Inputs["round_limit"] = input
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := root
			candidate.Inputs = maps.Clone(root.Inputs)
			candidate.Definition.Stages = maps.Clone(root.Definition.Stages)
			s := candidate.Definition.Stages["repeat"]
			edit(&candidate, &s)
			candidate.Definition.Stages["repeat"] = s
			_, err := compileRepeatFixture(t, candidate, body, registry)
			if err == nil {
				t.Fatal("invalid limit configuration compiled")
			}
		})
	}
}

func TestCoreRepeatBindingsScopesAndConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		edit       func(*WorkflowRevision, *WorkflowRevision, *Stage)
	}{
		{"missing_initial", "missing_binding", func(_, _ *WorkflowRevision, s *Stage) { delete(s.InitialBindings, "value") }},
		{"missing_next", "missing_binding", func(_, _ *WorkflowRevision, s *Stage) { delete(s.NextBindings, "value") }},
		{"one_body_needs_no_next", "", func(_, _ *WorkflowRevision, s *Stage) { s.MaxIterations = 1; delete(s.NextBindings, "value") }},
		{"one_body_still_checks_next_port", "unknown_port", func(_, _ *WorkflowRevision, s *Stage) {
			s.MaxIterations = 1
			s.NextBindings["value"] = Binding{From: "iteration_output", Port: "missing"}
		}},
		{"iteration_not_initial", "unsupported", func(_, _ *WorkflowRevision, s *Stage) {
			s.InitialBindings["value"] = Binding{From: "iteration_output", Port: "report"}
		}},
		{"iteration_not_public_finish", "unsupported", func(r, _ *WorkflowRevision, _ *Stage) {
			r.Definition.Stages["done"].OutputBindings["report"] = Binding{From: "iteration_output", Port: "report"}
		}},
		{"iteration_not_body_choice", "unsupported", func(_, b *WorkflowRevision, _ *Stage) {
			b.Definition.Entry = "pick"
			b.Definition.Stages["pick"] = Stage{Kind: "choice", Selection: "first_match", Branches: []ChoiceBranch{{ID: "one", Predicate: Predicate{Op: "exists", Ref: &FieldRef{From: "iteration_output", Port: "report"}}, Next: "done"}}, Default: "done"}
		}},
		{"unknown_until_port", "unknown_port", func(_, _ *WorkflowRevision, s *Stage) {
			s.Until = Predicate{Op: "exists", Ref: &FieldRef{From: "iteration_output", Port: "missing"}}
		}},
		{"cannot_read_self_as_stage_output", "unavailable_output", func(_, _ *WorkflowRevision, s *Stage) {
			s.NextBindings["value"] = Binding{From: "stage_output", StageID: "repeat", Port: "report"}
		}},
		{"no_private_body_stage", "missing_stage", func(_, _ *WorkflowRevision, s *Stage) {
			s.Until = Predicate{Op: "exists", Ref: &FieldRef{From: "stage_output", StageID: "repeat/done", Port: "report"}}
		}},
		{"undeclared_continue_outcome", "invalid_outcome", func(_, _ *WorkflowRevision, s *Stage) { s.ContinueOn = []string{"partial"} }},
		{"undeclared_complete_outcome", "invalid_outcome", func(_, _ *WorkflowRevision, s *Stage) { s.OnComplete["partial"] = "done" }},
		{"missing_limit_target", "missing_stage", func(_, _ *WorkflowRevision, s *Stage) { s.OnLimit = "missing" }},
		{"missing_unknown_target", "missing_stage", func(_, _ *WorkflowRevision, s *Stage) { s.OnUnknown = "missing" }},
		{"repeat_error_exports_absent", "unavailable_output", func(_, _ *WorkflowRevision, s *Stage) { s.OnError = "done" }},
		{"project_initial_override", "configuration_scope", func(_, b *WorkflowRevision, _ *Stage) {
			b.SchemaVersion = "2"
			input := b.Inputs["value"]
			input.Configuration = &InputConfiguration{Scope: "project"}
			b.Inputs["value"] = input
		}},
		{"project_next_override", "configuration_scope", func(_, b *WorkflowRevision, s *Stage) {
			b.SchemaVersion = "2"
			input := b.Inputs["value"]
			input.Configuration = &InputConfiguration{Scope: "project"}
			b.Inputs["value"] = input
			delete(s.InitialBindings, "value")
		}},
		{"one_body_project_next_still_refused", "configuration_scope", func(_, b *WorkflowRevision, s *Stage) {
			b.SchemaVersion = "2"
			input := b.Inputs["value"]
			input.Configuration = &InputConfiguration{Scope: "project"}
			b.Inputs["value"] = input
			s.MaxIterations = 1
			delete(s.InitialBindings, "value")
		}},
		{"pinned_configuration_can_supply_both", "", func(_, b *WorkflowRevision, s *Stage) {
			b.SchemaVersion = "2"
			input := b.Inputs["value"]
			input.Configuration = &InputConfiguration{Scope: "run", Default: json.RawMessage(`null`)}
			b.Inputs["value"] = input
			delete(s.InitialBindings, "value")
			delete(s.NextBindings, "value")
		}},
		{"repeat_does_not_make_outer_cycle_safe", "cycle", func(_, _ *WorkflowRevision, s *Stage) { s.OnLimit = "repeat" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, body, registry := repeatBindingFixture(t)
			stage := root.Definition.Stages["repeat"]
			tc.edit(&root, &body, &stage)
			root.Definition.Stages["repeat"] = stage
			_, err := compileRepeatFixture(t, root, body, registry)
			if tc.code != "" {
				expectProblem(t, err, tc.code)
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCoreRepeatSharedBoundsWithoutUnrolling(t *testing.T) {
	for _, tc := range []struct {
		name                        string
		iterations, steps, controls int64
		worker                      bool
		depth                       int
		code                        string
	}{
		{"one_empty", 1, 1, 4, false, 1, ""}, {"one_empty_short", 1, 1, 3, false, 1, "limit_exceeded"},
		{"two_empty", 2, 1, 6, false, 1, ""}, {"two_empty_short", 2, 1, 5, false, 1, "limit_exceeded"},
		{"two_workers", 2, 2, 8, true, 1, ""}, {"steps_not_reset", 2, 1, 8, true, 1, "limit_exceeded"},
		{"depth_not_zero", 2, 2, 8, true, 0, "limit_exceeded"},
		{"general_schema_max_not_local_100", 1_000_000, 1, 2_000_002, false, 1, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, registry := callWorkflow(t, "test:workflow/repeat-budget")
			body, _ := callWorkflow(t, "test:workflow/repeat-budget-body")
			body.Limits = Limits{MaxStepInstances: 1, MaxControlTransitions: 2, MaxParallelism: 1}
			if tc.worker {
				body.Definition.Entry = "work"
				body.Definition.Stages["work"] = Stage{Kind: "step", StepRef: callStep(t, registry, map[string]InputPort{}), InputBindings: map[string]Binding{}, On: map[string]string{"pass": "done"}}
			}
			root.Definition.Entry, root.Definition.Stages["repeat"] = "repeat", repeatStage(registerCallWorkflow(t, registry, body), "done", tc.iterations)
			root.Limits = Limits{MaxStepInstances: tc.steps, MaxControlTransitions: tc.controls, MaxParallelism: 1, MaxChildDepth: tc.depth}
			p, err := CompileProfile(encoded(t, root), "json", registry, CoreProfile)
			if tc.code != "" {
				expectProblem(t, err, tc.code)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if p.bounds.outcomes["no_work"].transitions != tc.controls || len(p.compilation.plans) != 2 || len(p.Sequence) != 2 || p.compilation.graphWork > 8 {
				t.Fatal("repeat bounds flattened the body or miscounted controls")
			}
		})
	}
	root, registry := callWorkflow(t, "test:workflow/repeat-overflow")
	body, _ := callWorkflow(t, "test:workflow/repeat-overflow-leaf")
	for i := range 3 {
		parent := root
		parent.ID = fmt.Sprintf("test:workflow/repeat-overflow-%d", i)
		parent.Limits.MaxStepInstances, parent.Limits.MaxControlTransitions = 100_000_000, 100_000_000
		parent.Definition.Stages = map[string]Stage{"done": {Kind: "finish", Outcome: "no_work", OutputBindings: map[string]Binding{}}, "repeat": repeatStage(registerCallWorkflow(t, registry, body), "done", 1_000_000)}
		parent.Definition.Entry = "repeat"
		body = parent
	}
	_, err := CompileProfile(encoded(t, body), "json", registry, CoreProfile)
	expectProblem(t, err, "limit_exceeded")
}

func TestCoreRepeatOutputAvailabilityPerExit(t *testing.T) {
	for _, route := range []string{"on_complete", "on_limit", "on_unknown", "on_error", "next_bindings"} {
		for _, optional := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/optional_%v", route, optional), func(t *testing.T) {
				root, body, registry := repeatBindingFixture(t)
				body.AllowedOutcomes = []string{"no_work", "rejected"}
				body.Definition.Entry = "pick"
				body.Definition.Stages["pick"] = Stage{Kind: "choice", Selection: "first_match", Branches: []ChoiceBranch{{ID: "one", Predicate: conditionFixturePredicate("true"), Next: "done"}}, Default: "negative"}
				body.Definition.Stages["negative"] = Stage{Kind: "finish", Outcome: "rejected", OutputBindings: map[string]Binding{}}
				root.Outputs = map[string]OutputPort{}
				root.Definition.Stages["done"] = Stage{Kind: "finish", Outcome: "no_work", OutputBindings: map[string]Binding{}}
				stage := root.Definition.Stages["repeat"]
				stage.ContinueOn = []string{"no_work", "rejected"}
				stage.OnComplete["rejected"] = "done"
				stage.NextBindings["value"] = Binding{From: "workflow_input", Port: "value"}
				if route == "next_bindings" {
					// Keep the required body value independently bound; this extra
					// input tests only the conditional report carried to iteration2.
					body.Inputs["extra"] = InputPort{Port: body.Inputs["value"].Port, Required: !optional}
					stage.InitialBindings["extra"] = Binding{From: "workflow_input", Port: "value"}
					stage.NextBindings["extra"] = Binding{From: "iteration_output", Port: "report"}
				} else {
					consumer := callStep(t, registry, map[string]InputPort{"value": {Port: body.Outputs["report"].Port, Required: !optional}})
					root.Definition.Stages["consume"] = Stage{Kind: "step", StepRef: consumer, InputBindings: map[string]Binding{"value": {From: "stage_output", StageID: "repeat", Port: "report"}}, On: map[string]string{"pass": "done"}}
					switch route {
					case "on_complete":
						stage.OnComplete = map[string]string{"no_work": "consume", "rejected": "consume"}
					case "on_limit":
						stage.OnLimit = "consume"
					case "on_unknown":
						stage.OnUnknown = "consume"
					case "on_error":
						stage.OnError = "consume"
					}
				}
				root.Definition.Stages["repeat"] = stage
				_, err := compileRepeatFixture(t, root, body, registry)
				if optional {
					if err != nil {
						t.Fatal(err)
					}
				} else {
					expectProblem(t, err, "unavailable_output")
				}
				if route == "on_limit" || route == "on_unknown" {
					// A port required for every continuing outcome is still
					// available if a predicate field is unknown or the limit wins.
					stage.ContinueOn = []string{"no_work"}
					root.Definition.Stages["repeat"] = stage
					if _, err := compileRepeatFixture(t, root, body, registry); err != nil {
						t.Fatal("valid exports lost on controller exit", err)
					}
				}
			})
		}
	}
}

func TestCoreRepeatParentFactsAndBodyScope(t *testing.T) {
	root, body, registry := repeatBindingFixture(t)
	ref := registerCallWorkflow(t, registry, body)
	before := callStage(ref, "repeat")
	before.InputBindings["value"] = Binding{From: "workflow_input", Port: "value"}
	root.Definition.Stages["before"] = before
	stage := root.Definition.Stages["repeat"]
	stage.Until = Predicate{Op: "exists", Ref: &FieldRef{From: "stage_output", StageID: "before", Port: "report"}}
	stage.NextBindings["value"] = Binding{From: "stage_output", StageID: "before", Port: "report"}
	root.Definition.Entry, root.Definition.Stages["repeat"] = "before", stage
	p, err := compileRepeatFixture(t, root, body, registry)
	if err != nil || p.Calls["before"] != p.Repeats["repeat"] || p.bounds.outcomes["no_work"].transitions != 11 {
		t.Fatal("parent facts or shared call/repeat definition cache lost", err)
	}
	// An optional predecessor can be inspected as unknown, but cannot provide
	// an unconditionally required next input on the bypass path.
	root.Definition.Entry = "choose"
	root.Definition.Stages["choose"] = Stage{Kind: "choice", Selection: "first_match", Branches: []ChoiceBranch{{ID: "one", Predicate: conditionFixturePredicate("true"), Next: "before"}}, Default: "repeat"}
	_, err = compileRepeatFixture(t, root, body, registry)
	expectProblem(t, err, "unavailable_output")
	stage.NextBindings["value"] = Binding{From: "workflow_input", Port: "value"}
	root.Definition.Stages["repeat"] = stage
	if _, err := compileRepeatFixture(t, root, body, registry); err != nil {
		t.Fatal("optional parent predicate reference rejected", err)
	}

	// Unlike a parent predecessor, the controller's future consumer cannot be
	// read as if it were the just-completed body's similarly named private step.
	stage.Until.Ref.StageID = "after"
	stage.OnComplete["no_work"], stage.OnLimit = "after", "after"
	root.Definition.Stages["repeat"] = stage
	root.Definition.Stages["after"] = before
	after := root.Definition.Stages["after"]
	after.On = map[string]string{"no_work": "done"}
	root.Definition.Stages["after"] = after
	_, err = compileRepeatFixture(t, root, body, registry)
	expectProblem(t, err, "unavailable_output")
}

func TestCoreRepeatOutcomeCostsAndFailedPrefix(t *testing.T) {
	for _, handled := range []bool{false, true} {
		t.Run(fmt.Sprintf("handled_%v", handled), func(t *testing.T) {
			root, registry := callWorkflow(t, "test:workflow/repeat-cost")
			body, _ := callWorkflow(t, "test:workflow/repeat-cost-body")
			worker := callStep(t, registry, map[string]InputPort{})
			chain := func(w *WorkflowRevision, outcome string) {
				for i := range 5 {
					next := "done"
					if i < 4 {
						next = fmt.Sprintf("work%d", i+1)
					}
					w.Definition.Stages[fmt.Sprintf("work%d", i)] = Stage{Kind: "step", StepRef: worker, InputBindings: map[string]Binding{}, On: map[string]string{"pass": next}}
				}
				w.Definition.Stages["done"] = Stage{Kind: "finish", Outcome: outcome, OutputBindings: map[string]Binding{}}
			}
			body.AllowedOutcomes = []string{"rejected", "no_work"}
			chain(&body, "rejected")
			body.Definition.Entry = "choose"
			body.Definition.Stages["choose"] = Stage{Kind: "choice", Selection: "first_match", Branches: []ChoiceBranch{{ID: "slow", Predicate: conditionFixturePredicate("true"), Next: "work0"}}, Default: "fast"}
			body.Definition.Stages["fast"] = Stage{Kind: "finish", Outcome: "no_work", OutputBindings: map[string]Binding{}}
			chain(&root, "no_work")
			stage := repeatStage(registerCallWorkflow(t, registry, body), "work0", 3)
			stage.OnComplete["rejected"] = "done"
			root.Limits.MaxStepInstances, root.Limits.MaxControlTransitions = 5, 16
			if handled {
				stage.OnError = "work0"
				root.Limits.MaxStepInstances, root.Limits.MaxControlTransitions = 9, 21
			}
			root.Definition.Entry, root.Definition.Stages["repeat"] = "repeat", stage
			_, err := CompileProfile(encoded(t, root), "json", registry, CoreProfile)
			if handled {
				expectProblem(t, err, "limit_exceeded")
				root.Limits.MaxStepInstances = 10
				_, err = CompileProfile(encoded(t, root), "json", registry, CoreProfile)
			}
			if err != nil {
				t.Fatal("non-continuing expensive path was multiplied or failed prefix was omitted", err)
			}
		})
	}
}

func TestCoreRepeatAliasesAndMixedCycles(t *testing.T) {
	root, registry := callWorkflow(t, "test:workflow/repeat-alias")
	body, _ := callWorkflow(t, "test:workflow/repeat-alias-body")
	leaf, _ := callWorkflow(t, "test:workflow/repeat-alias-leaf")
	root.Definition.Entry, root.Definition.Stages["repeat"] = "repeat", repeatStage(Ref{}, "after", 2)
	root.Definition.Stages["after"] = callStage(Ref{}, "done")
	body.Definition.Entry, body.Definition.Stages["call"] = "call", callStage(Ref{}, "done")
	aliases := map[string][]byte{"body": aliasAuthor(t, body, map[string]string{"call": "leaf"}), "leaf": encoded(t, leaf)}
	raw := aliasAuthor(t, root, map[string]string{"repeat": "body", "after": "leaf"})
	resolved, pinned, err := ResolveWorkflowAliases(raw, "json", registry, aliases)
	if err != nil {
		t.Fatal(err)
	}
	p, err := CompileProfile(resolved, "json", pinned, CoreProfile)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(resolved, []byte(`"alias"`)) || p.Repeats["repeat"].Calls["call"] != p.Calls["after"] || p.bounds.outcomes["no_work"].transitions != 15 || p.bounds.depth != 2 {
		t.Fatal("alias resolution lost exact nested reuse or call/repeat accounting")
	}
	for _, firstRepeat := range []bool{false, true} {
		t.Run(fmt.Sprintf("root_repeat_%v", firstRepeat), func(t *testing.T) {
			a, _ := callWorkflow(t, "test:workflow/repeat-cycle-a")
			b, _ := callWorkflow(t, "test:workflow/repeat-cycle-b")
			a.Definition.Entry, b.Definition.Entry = "nested", "nested"
			a.Definition.Stages["nested"], b.Definition.Stages["nested"] = callStage(Ref{}, "done"), repeatStage(Ref{}, "done", 1)
			if firstRepeat {
				a.Definition.Stages["nested"], b.Definition.Stages["nested"] = b.Definition.Stages["nested"], a.Definition.Stages["nested"]
			}
			aRaw, bRaw := aliasAuthor(t, a, map[string]string{"nested": "b"}), aliasAuthor(t, b, map[string]string{"nested": "a"})
			_, _, err := ResolveWorkflowAliases(aRaw, "json", registry, map[string][]byte{"a": aRaw, "b": bRaw})
			expectProblem(t, err, "alias_cycle")
		})
	}
	// A finite repeat count does not authorize recursion into an active
	// workflow identity/version, even if a conflicting digest is supplied.
	a, reg := callWorkflow(t, "test:workflow/repeat-identity")
	b, _ := callWorkflow(t, a.ID)
	b.Title = "Conflicting body with the active root identity"
	a.Definition.Entry, a.Definition.Stages["repeat"] = "repeat", repeatStage(registerCallWorkflow(t, reg, b), "done", 1)
	_, err = CompileProfile(encoded(t, a), "json", reg, CoreProfile)
	expectProblem(t, err, "ref_identity_conflict")
}

func TestCoreRepeatPreflightAndLiteralSeparation(t *testing.T) {
	for _, raw := range []string{"1.0000000000000000001", "1e-999999999"} {
		root, registry := callWorkflow(t, "test:workflow/repeat-raw")
		body, _ := callWorkflow(t, "test:workflow/repeat-raw-body")
		body.Definition.Entry = "choose"
		body.Definition.Stages["choose"] = Stage{Kind: "choice", Selection: "first_match", Branches: []ChoiceBranch{{ID: "one", Predicate: Predicate{Op: "eq", Left: conditionLiteral(raw), Right: conditionLiteral("1")}, Next: "done"}}}
		root.Definition.Entry, root.Definition.Stages["repeat"] = "repeat", repeatStage(registerCallWorkflow(t, registry, body), "done", 2)
		_, err := CompileProfile(encoded(t, root), "json", registry, CoreProfile)
		expectProblem(t, err, "condition_type_mismatch")
		_, _, err = ResolveWorkflowAliases(aliasAuthor(t, root, map[string]string{"repeat": "body"}), "json", registry, map[string][]byte{"body": encoded(t, body)})
		expectProblem(t, err, "condition_type_mismatch")
	}
	for _, count := range []int{16, 17, 256, 257} {
		t.Run(fmt.Sprintf("bound_%d", count), func(t *testing.T) {
			root, registry := callWorkflow(t, "test:workflow/repeat-ast")
			body, _ := callWorkflow(t, "test:workflow/repeat-ast-body")
			stage := repeatStage(registerCallWorkflow(t, registry, body), "done", 1)
			if count < 20 {
				stage.Until = conditionNested(count)
			} else {
				stage.Until = Predicate{Op: "all"}
				for _, branch := range conditionSizedChoice(count - 1).Branches {
					stage.Until.Args = append(stage.Until.Args, branch.Predicate)
				}
			}
			root.Definition.Entry, root.Definition.Stages["repeat"] = "repeat", stage
			_, err := CompileProfile(encoded(t, root), "json", registry, CoreProfile)
			if count == 17 || count == 257 {
				expectProblem(t, err, "predicate_limit")
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
	root, body, registry := repeatBindingFixture(t)
	stage := root.Definition.Stages["repeat"]
	fake := `{"id":"data:not-a-dependency","version":"1.0.0","digest":"sha256:` + strings.Repeat("1", 64) + `"}`
	for _, bindings := range []map[string]Binding{stage.InitialBindings, stage.NextBindings} {
		bindings["value"] = Binding{From: "literal", SchemaRef: body.Inputs["value"].SchemaRef, Value: json.RawMessage(fake)}
	}
	root.Definition.Stages["repeat"] = stage
	if _, err := compileRepeatFixture(t, root, body, registry); err != nil {
		t.Fatal("repeat literal JSON became a dependency", err)
	}
}

func TestCoreRepeatIterationProjectionAndPredicateTypes(t *testing.T) {
	root, body, registry := repeatBindingFixture(t)
	register := func(id, raw string) Ref {
		t.Helper()
		digest, err := Digest([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		ref := Ref{ID: id, Version: "1.0.0", Digest: digest}
		registry[ref] = []byte(raw)
		return ref
	}
	valueRef := register("test:schema/repeat-value", `{"type":["integer","null"]}`)
	envelopeRef := register("test:schema/repeat-envelope", `{"type":"object","properties":{"inner":{"type":["integer","null"]}},"required":["inner"]}`)
	body.Inputs["value"] = InputPort{Port: Port{Format: "json", SchemaRef: &valueRef}, Required: true}
	body.Outputs["report"] = OutputPort{Port: Port{Format: "json", SchemaRef: &envelopeRef}, RequiredFor: []string{"no_work"}}
	body.Definition.Stages["done"].OutputBindings["report"] = Binding{From: "literal", SchemaRef: &envelopeRef, Value: json.RawMessage(`{"inner":null}`)}
	root.Inputs["value"], root.Outputs["report"] = body.Inputs["value"], body.Outputs["report"]
	stage := root.Definition.Stages["repeat"]
	pointer := "/inner"
	stage.NextBindings["value"] = Binding{From: "iteration_output", Port: "report", Pointer: &pointer, ProjectedSchemaRef: &valueRef}
	stage.Until = Predicate{Op: "eq", Left: &Operand{Kind: "field", Ref: &FieldRef{From: "iteration_output", Port: "report", Pointer: pointer}}, Right: conditionLiteral("null")}
	root.Definition.Stages["repeat"] = stage
	p, err := compileRepeatFixture(t, root, body, registry)
	if err != nil {
		t.Fatal(err)
	}
	projected, present, err := p.ProjectJSON(stage.NextBindings["value"], []byte(`{"inner":null}`))
	if err != nil || !present || string(projected) != "null" {
		t.Fatal("iteration projection lost present null", string(projected), present, err)
	}
	decision, err := p.SelectRepeat("repeat", 1, "no_work", func(ref FieldRef) (any, bool, error) {
		if ref.Pointer != pointer || ref.From != "iteration_output" {
			t.Fatal("wrong iteration projection", ref)
		}
		return nil, true, nil
	})
	if err != nil || decision.Route != "on_complete" || decision.UntilTruth != TruthTrue {
		t.Fatal("present null did not compare as null", decision, err)
	}
	decision, err = p.SelectRepeat("repeat", 1, "no_work", func(FieldRef) (any, bool, error) { return nil, false, nil })
	expectProblem(t, err, "condition_unknown")
	if decision.UntilTruth != TruthUnknown {
		t.Fatal("missing became present null")
	}

	// The same projection cannot assert availability once the body contract
	// stops guaranteeing the pointed field, even if fixture bytes contain it.
	optionalEnvelope := register("test:schema/repeat-optional-envelope", `{"type":"object","properties":{"inner":{"type":["integer","null"]}}}`)
	output := body.Outputs["report"]
	output.SchemaRef = &optionalEnvelope
	body.Outputs["report"], root.Outputs["report"] = output, output
	binding := body.Definition.Stages["done"].OutputBindings["report"]
	binding.SchemaRef = &optionalEnvelope
	body.Definition.Stages["done"].OutputBindings["report"] = binding
	_, err = compileRepeatFixture(t, root, body, registry)
	expectProblem(t, err, "unavailable_output")

	for _, inspect := range []bool{false, true} {
		t.Run(fmt.Sprintf("blob_predicate_%v", inspect), func(t *testing.T) {
			r, b, registry := repeatBindingFixture(t)
			port := Port{Format: "blob", MediaTypes: []string{"text/plain"}}
			b.Inputs["value"] = InputPort{Port: port, Required: true}
			b.Outputs["report"] = OutputPort{Port: port, RequiredFor: []string{"no_work"}}
			r.Inputs["value"], r.Outputs["report"] = b.Inputs["value"], b.Outputs["report"]
			if inspect {
				s := r.Definition.Stages["repeat"]
				s.Until = Predicate{Op: "exists", Ref: &FieldRef{From: "iteration_output", Port: "report"}}
				r.Definition.Stages["repeat"] = s
			}
			_, err := compileRepeatFixture(t, r, b, registry)
			if inspect {
				expectProblem(t, err, "condition_type_mismatch")
			} else if err != nil {
				t.Fatal("whole blob cannot be carried to the next body", err)
			}
		})
	}
}

func TestCoreRepeatFieldEvidenceAndNestedBounds(t *testing.T) {
	ref := FieldRef{From: "iteration_output", Port: "report", Pointer: "/" + strings.Repeat("𐀀", 2047)}
	allowed := MaxPredicateFieldBytes / len(encoded(t, ref))
	for _, count := range []int{allowed, allowed + 1} {
		root, body, registry := repeatBindingFixture(t)
		stage := root.Definition.Stages["repeat"]
		stage.Until = Predicate{Op: "all"}
		for range count {
			stage.Until.Args = append(stage.Until.Args, Predicate{Op: "exists", Ref: &ref})
		}
		root.Definition.Stages["repeat"] = stage
		_, err := compileRepeatFixture(t, root, body, registry)
		if count > allowed {
			expectProblem(t, err, "predicate_limit")
		} else if err != nil {
			t.Fatal("bounded UTF-8 evidence rejected", err)
		}
	}
	for _, tc := range []struct {
		rootDepth, bodyDepth int
		fail                 bool
	}{{2, 1, false}, {1, 1, true}, {2, 0, true}} {
		root, registry := callWorkflow(t, "test:workflow/repeat-nested")
		body, _ := callWorkflow(t, "test:workflow/repeat-nested-body")
		leaf, _ := callWorkflow(t, "test:workflow/repeat-nested-leaf")
		body.Definition.Entry, body.Definition.Stages["repeat"] = "repeat", repeatStage(registerCallWorkflow(t, registry, leaf), "done", 2)
		body.Limits.MaxControlTransitions, body.Limits.MaxChildDepth = 6, tc.bodyDepth
		root.Definition.Entry, root.Definition.Stages["repeat"] = "repeat", repeatStage(registerCallWorkflow(t, registry, body), "done", 3)
		root.Limits.MaxControlTransitions, root.Limits.MaxChildDepth = 23, tc.rootDepth
		p, err := CompileProfile(encoded(t, root), "json", registry, CoreProfile)
		if tc.fail {
			expectProblem(t, err, "limit_exceeded")
		} else if err != nil || p.bounds.outcomes["no_work"].transitions != 23 || p.bounds.depth != 2 {
			t.Fatal("nested repeat renewed aggregate budget or used iteration count as depth", err)
		}
	}
	// The multiplication guard runs before a possible int64 overflow. Actual
	// protocol ceilings are smaller, but nested costs must never wrap positive.
	if got := saturatedProduct(1<<62, 4, 1<<63-2); got != 1<<63-1 {
		t.Fatal("cost multiplication wrapped", got)
	}
}
