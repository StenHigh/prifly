package flow

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"
)

func callWorkflow(t *testing.T, id string) (WorkflowRevision, Registry) {
	t.Helper()
	value, registry := fixture(t)
	var w WorkflowRevision
	if err := json.Unmarshal(encoded(t, value), &w); err != nil {
		t.Fatal(err)
	}
	w.ID, w.Title = id, id
	w.Inputs, w.Outputs = map[string]InputPort{}, map[string]OutputPort{}
	w.AllowedOutcomes = []string{"no_work"}
	w.Limits = Limits{MaxStepInstances: 100, MaxControlTransitions: 1000, MaxParallelism: 1, MaxChildDepth: 8}
	w.Definition.Entry = "done"
	w.Definition.Stages = map[string]Stage{"done": {Kind: "finish", Outcome: "no_work", OutputBindings: map[string]Binding{}}}
	return w, registry
}

func registerCallWorkflow(t *testing.T, registry Registry, w WorkflowRevision) Ref {
	t.Helper()
	data := encoded(t, w)
	digest, err := Digest(data)
	if err != nil {
		t.Fatal(err)
	}
	ref := Ref{ID: w.ID, Version: w.Version, Digest: digest}
	registry[ref] = data
	return ref
}

func callStage(ref Ref, next string) Stage {
	return Stage{Kind: "call", WorkflowRef: ref, InputBindings: map[string]Binding{}, On: map[string]string{"no_work": next}}
}

func callStep(t *testing.T, registry Registry, inputs map[string]InputPort) Ref {
	t.Helper()
	w, _ := fixture(t)
	var ref Ref
	if err := json.Unmarshal(encoded(t, stages(w)["check_first"].(map[string]any)["step_ref"]), &ref); err != nil {
		t.Fatal(err)
	}
	var step StepDefinition
	if err := json.Unmarshal(registry[ref], &step); err != nil {
		t.Fatal(err)
	}
	step.ID, step.Inputs, step.Outputs = "test:step/call", inputs, map[string]OutputPort{}
	data := encoded(t, step)
	digest, err := Digest(data)
	if err != nil {
		t.Fatal(err)
	}
	ref = Ref{ID: step.ID, Version: step.Version, Digest: digest}
	registry[ref] = data
	return ref
}

func callBindingFixture(t *testing.T) (WorkflowRevision, WorkflowRevision, Registry) {
	t.Helper()
	root, registry := callWorkflow(t, "test:workflow/caller")
	child, _ := callWorkflow(t, "test:workflow/child")
	schema := []byte(`true`)
	digest, _ := Digest(schema)
	ref := Ref{ID: "test:schema/call", Version: "1.0.0", Digest: digest}
	registry[ref] = schema
	port := Port{Format: "json", SchemaRef: &ref}
	child.Inputs = map[string]InputPort{"value": {Port: port, Required: true}}
	child.Outputs = map[string]OutputPort{"report": {Port: port, RequiredFor: []string{"no_work"}}}
	child.Limits.MaxChildDepth = 0
	child.Definition.Stages["done"] = Stage{Kind: "finish", Outcome: "no_work", OutputBindings: map[string]Binding{"report": {From: "workflow_input", Port: "value"}}}
	root.Inputs = map[string]InputPort{"value": child.Inputs["value"]}
	root.Outputs = map[string]OutputPort{"report": child.Outputs["report"]}
	root.Definition.Entry = "call"
	stage := callStage(Ref{}, "done")
	stage.InputBindings["value"] = Binding{From: "workflow_input", Port: "value"}
	root.Definition.Stages["call"] = stage
	root.Definition.Stages["done"] = Stage{Kind: "finish", Outcome: "no_work", OutputBindings: map[string]Binding{"report": {From: "stage_output", StageID: "call", Port: "report"}}}
	return root, child, registry
}

func compileCallFixture(t *testing.T, root, child WorkflowRevision, registry Registry) (*Plan, error) {
	t.Helper()
	stage := root.Definition.Stages["call"]
	stage.WorkflowRef = registerCallWorkflow(t, registry, child)
	root.Definition.Stages["call"] = stage
	return CompileProfile(encoded(t, root), "json", registry, CoreProfile)
}

func TestCoreCallsReuseLocalPlansAndExports(t *testing.T) {
	root, child, registry := callBindingFixture(t)
	child.AllowedOutcomes = []string{"no_work", "rejected"}
	ref := registerCallWorkflow(t, registry, child)
	first := root.Definition.Stages["call"]
	first.WorkflowRef, first.On["no_work"] = ref, "second"
	root.Definition.Stages["call"] = first
	second := callStage(ref, "done")
	second.InputBindings["value"] = Binding{From: "stage_output", StageID: "call", Port: "report"}
	root.Definition.Stages["second"] = second
	finish := root.Definition.Stages["done"]
	finish.OutputBindings["report"] = Binding{From: "stage_output", StageID: "second", Port: "report"}
	root.Definition.Stages["done"] = finish
	root.Limits.MaxControlTransitions, root.Limits.MaxChildDepth = 7, 1
	p, err := CompileProfile(encoded(t, root), "json", registry, CoreProfile)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Calls) != 2 || p.Calls["call"] != p.Calls["second"] || len(p.Steps) != 0 || !slices.Equal(p.Sequence, []string{"call", "second", "done"}) || !slices.Equal(p.Calls["call"].Sequence, []string{"done"}) {
		t.Fatal("calls flattened private stages or did not reuse a validated definition")
	}
	if p.StageOutputs("call")["report"].SchemaRef == nil || p.StageOutputs("done") != nil {
		t.Fatal("call public exports were lost or a finish gained producer ports")
	}
	if next, err := p.NextOutcome("call", "no_work"); err != nil || next != "second" {
		t.Fatalf("wrong child outcome route: %q %v", next, err)
	}
	_, err = p.NextOutcome("call", "rejected")
	expectProblem(t, err, "unhandled_outcome")
	_, err = p.NextOutcome("call", "pass")
	expectProblem(t, err, "invalid_outcome")
	_, err = p.Next("call", "pass")
	expectProblem(t, err, "invalid_stage")
	_, err = p.NextError("call")
	expectProblem(t, err, "unhandled_error")
	if err := ValidateProtocol("WorkflowRevision", encoded(t, p.Workflow)); err != nil {
		t.Fatal("call changed the baseline wire contract", err)
	}
	_, err = Compile(encoded(t, root), "json", registry)
	expectProblem(t, err, "unsupported")
	registry[ref] = []byte(`null`)
	if p.Calls["call"].Workflow.ID != child.ID || p.Calls["call"].Digest != ref.Digest || string(p.Registry[ref]) == "null" {
		t.Fatal("mutable inventory replaced the pinned child")
	}
}

func TestCoreCallBindingsAndOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		edit       func(*WorkflowRevision, *WorkflowRevision, Registry)
	}{
		{"missing required binding", "missing_binding", func(r, _ *WorkflowRevision, _ Registry) { delete(r.Definition.Stages["call"].InputBindings, "value") }},
		{"unknown input", "unknown_port", func(r, _ *WorkflowRevision, _ Registry) {
			r.Definition.Stages["call"].InputBindings["other"] = Binding{From: "workflow_input", Port: "value"}
		}},
		{"optional source cannot satisfy required child", "unavailable_output", func(r, _ *WorkflowRevision, _ Registry) {
			p := r.Inputs["value"]
			p.Required = false
			r.Inputs["value"] = p
		}},
		{"private child stage cannot be bound", "unknown_port", func(r, _ *WorkflowRevision, _ Registry) {
			r.Definition.Stages["call"].InputBindings["value"] = Binding{From: "stage_output", StageID: "child/done", Port: "report"}
		}},
		{"undeclared child outcome", "invalid_outcome", func(r, _ *WorkflowRevision, _ Registry) { r.Definition.Stages["call"].On["partial"] = "done" }},
		{"missing child export", "missing_binding", func(_, c *WorkflowRevision, _ Registry) { delete(c.Definition.Stages["done"].OutputBindings, "report") }},
		{"missing export on same outcome finish", "missing_binding", func(_, c *WorkflowRevision, _ Registry) {
			c.Definition.Entry = "pick"
			c.Definition.Stages["pick"] = Stage{Kind: "choice", Selection: "first_match", Branches: []ChoiceBranch{{ID: "one", Predicate: conditionFixturePredicate("true"), Next: "done"}}, Default: "other"}
			c.Definition.Stages["other"] = Stage{Kind: "finish", Outcome: "no_work", OutputBindings: map[string]Binding{}}
		}},
		{"error return guarantees no export", "unavailable_output", func(r, _ *WorkflowRevision, _ Registry) {
			s := r.Definition.Stages["call"]
			s.OnError = "done"
			r.Definition.Stages["call"] = s
		}},
		{"project configuration cannot be overridden", "configuration_scope", func(_, c *WorkflowRevision, _ Registry) {
			c.SchemaVersion = "2"
			p := c.Inputs["value"]
			p.Configuration = &InputConfiguration{Scope: "project"}
			c.Inputs["value"] = p
		}},
		{"declared required configuration may be pinned later", "", func(r, c *WorkflowRevision, _ Registry) {
			c.SchemaVersion = "2"
			p := c.Inputs["value"]
			p.Configuration = &InputConfiguration{Scope: "project"}
			c.Inputs["value"] = p
			delete(r.Definition.Stages["call"].InputBindings, "value")
		}},
		{"null child configuration default is a value", "", func(r, c *WorkflowRevision, _ Registry) {
			c.SchemaVersion = "2"
			p := c.Inputs["value"]
			p.Configuration = &InputConfiguration{Scope: "run", Default: json.RawMessage(`null`)}
			c.Inputs["value"] = p
			delete(r.Definition.Stages["call"].InputBindings, "value")
		}},
		{"partial exports are not renamed pass", "", func(r, c *WorkflowRevision, _ Registry) {
			r.AllowedOutcomes, c.AllowedOutcomes = []string{"partial"}, []string{"partial"}
			p := c.Outputs["report"]
			p.RequiredFor = []string{"partial"}
			c.Outputs["report"], r.Outputs["report"] = p, p
			for _, w := range []*WorkflowRevision{r, c} {
				s := w.Definition.Stages["done"]
				s.Outcome = "partial"
				w.Definition.Stages["done"] = s
			}
			s := r.Definition.Stages["call"]
			s.On = map[string]string{"partial": "done"}
			r.Definition.Stages["call"] = s
		}},
		// A child that may rest on a waived check declares the reduced outcome
		// and the caller routes it explicitly; the reduction is never a route
		// the caller may claim for a child that cannot produce it.
		{"declared reduction is routable", "", func(r, c *WorkflowRevision, _ Registry) {
			c.AllowedOutcomes = append(c.AllowedOutcomes, "completed_with_waivers")
			// The reduction is a path like any other, so a required output
			// must be guaranteed on it too.
			out := c.Outputs["report"]
			out.RequiredFor = append(out.RequiredFor, "completed_with_waivers")
			c.Outputs["report"] = out
			s := r.Definition.Stages["call"]
			s.On = map[string]string{"no_work": "done", "completed_with_waivers": "done"}
			r.Definition.Stages["call"] = s
		}},
		{"reduction routed for a child that cannot produce it", "invalid_outcome", func(r, _ *WorkflowRevision, _ Registry) {
			s := r.Definition.Stages["call"]
			s.On = map[string]string{"no_work": "done", "completed_with_waivers": "done"}
			r.Definition.Stages["call"] = s
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, child, registry := callBindingFixture(t)
			tc.edit(&root, &child, registry)
			p, err := compileCallFixture(t, root, child, registry)
			if tc.code != "" {
				expectProblem(t, err, tc.code)
				return
			}
			if err != nil || p.Calls["call"] == nil {
				t.Fatal("valid child contract rejected", err)
			}
		})
	}
}

func TestCoreCallOutcomeAvailability(t *testing.T) {
	for _, optional := range []bool{false, true} {
		t.Run(fmt.Sprintf("optional_%v", optional), func(t *testing.T) {
			root, child, registry := callBindingFixture(t)
			child.AllowedOutcomes = []string{"no_work", "rejected"}
			child.Definition.Entry = "pick"
			child.Definition.Stages["pick"] = Stage{Kind: "choice", Selection: "first_match", Branches: []ChoiceBranch{{ID: "current_fixture", Predicate: conditionFixturePredicate("true"), Next: "done"}}, Default: "rejected"}
			child.Definition.Stages["rejected"] = Stage{Kind: "finish", Outcome: "rejected", OutputBindings: map[string]Binding{}}
			port := child.Outputs["report"].Port
			consumer := callStep(t, registry, map[string]InputPort{"value": {Port: port, Required: !optional}})
			root.Outputs = map[string]OutputPort{}
			root.Definition.Stages["done"] = Stage{Kind: "finish", Outcome: "no_work", OutputBindings: map[string]Binding{}}
			stage := root.Definition.Stages["call"]
			stage.On = map[string]string{"no_work": "consume", "rejected": "consume"}
			root.Definition.Stages["call"] = stage
			root.Definition.Stages["consume"] = Stage{Kind: "step", StepRef: consumer, InputBindings: map[string]Binding{"value": {From: "stage_output", StageID: "call", Port: "report"}}, On: map[string]string{"pass": "done"}}
			_, err := compileCallFixture(t, root, child, registry)
			if optional {
				if err != nil {
					t.Fatal(err)
				}
			} else {
				expectProblem(t, err, "unavailable_output")
			}
		})
	}
}

func TestCoreCallSharedBudgets(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		calls, workers, depth int
		steps, controls       int64
		code                  string
	}{
		{"empty exact", 1, 0, 1, 1, 4, ""}, {"empty return is charged", 1, 0, 1, 1, 3, "limit_exceeded"},
		{"two empty calls", 2, 0, 1, 1, 7, ""}, {"reuse is not free", 2, 0, 1, 1, 6, "limit_exceeded"},
		{"depth is enforced", 1, 0, 0, 1, 4, "limit_exceeded"},
		{"two actual instances", 2, 1, 1, 2, 9, ""}, {"no renewed step budget", 2, 1, 1, 1, 9, "limit_exceeded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, registry := callWorkflow(t, "test:workflow/budget")
			child, _ := callWorkflow(t, "test:workflow/budget-child")
			child.Limits = Limits{MaxStepInstances: 1, MaxControlTransitions: 2, MaxParallelism: 1}
			if tc.workers != 0 {
				child.Definition.Entry = "work"
				child.Definition.Stages["work"] = Stage{Kind: "step", StepRef: callStep(t, registry, map[string]InputPort{}), InputBindings: map[string]Binding{}, On: map[string]string{"pass": "done"}}
			}
			ref := registerCallWorkflow(t, registry, child)
			root.Definition.Entry = "call0"
			for i := range tc.calls {
				next := "done"
				if i+1 < tc.calls {
					next = fmt.Sprintf("call%d", i+1)
				}
				root.Definition.Stages[fmt.Sprintf("call%d", i)] = callStage(ref, next)
			}
			root.Limits = Limits{MaxStepInstances: tc.steps, MaxControlTransitions: tc.controls, MaxParallelism: 1, MaxChildDepth: tc.depth}
			p, err := CompileProfile(encoded(t, root), "json", registry, CoreProfile)
			if tc.code != "" {
				expectProblem(t, err, tc.code)
				return
			}
			if err != nil || p.bounds.outcomes["no_work"].transitions != tc.controls {
				t.Fatalf("wrong composed cost: %+v %v", p, err)
			}
		})
	}
}

func TestCoreCallOutcomeBudgetAndErrorPrefix(t *testing.T) {
	for _, handledError := range []bool{false, true} {
		t.Run(fmt.Sprintf("error_route_%v", handledError), func(t *testing.T) {
			root, registry := callWorkflow(t, "test:workflow/cost-root")
			child, _ := callWorkflow(t, "test:workflow/cost-child")
			worker := callStep(t, registry, map[string]InputPort{})
			chain := func(w *WorkflowRevision, outcome string) {
				for i := 0; i < 5; i++ {
					next := "done"
					if i < 4 {
						next = fmt.Sprintf("work%d", i+1)
					}
					w.Definition.Stages[fmt.Sprintf("work%d", i)] = Stage{Kind: "step", StepRef: worker, InputBindings: map[string]Binding{}, On: map[string]string{"pass": next}}
				}
				w.Definition.Stages["done"] = Stage{Kind: "finish", Outcome: outcome, OutputBindings: map[string]Binding{}}
			}
			child.AllowedOutcomes = []string{"rejected", "no_work"}
			chain(&child, "rejected")
			child.Definition.Entry = "choose"
			child.Definition.Stages["choose"] = Stage{Kind: "choice", Selection: "first_match", Branches: []ChoiceBranch{{ID: "slow", Predicate: conditionFixturePredicate("true"), Next: "work0"}}, Default: "fast"}
			child.Definition.Stages["fast"] = Stage{Kind: "finish", Outcome: "no_work", OutputBindings: map[string]Binding{}}
			ref := registerCallWorkflow(t, registry, child)
			chain(&root, "no_work")
			root.Definition.Entry = "call"
			stage := callStage(ref, "work0")
			stage.On["rejected"] = "done"
			root.Limits.MaxStepInstances, root.Limits.MaxControlTransitions = 5, 10
			if handledError {
				stage.OnError = "work0"
				root.Limits.MaxStepInstances, root.Limits.MaxControlTransitions = 9, 15
			}
			root.Definition.Stages["call"] = stage
			_, err := CompileProfile(encoded(t, root), "json", registry, CoreProfile)
			if handledError {
				expectProblem(t, err, "limit_exceeded")
				root.Limits.MaxStepInstances = 10
				_, err = CompileProfile(encoded(t, root), "json", registry, CoreProfile)
			}
			if err != nil {
				t.Fatal("outcome costs combined incompatible child paths or lost error prefix", err)
			}
		})
	}
}

func TestCoreChildPreflightBeforeCanonicalization(t *testing.T) {
	for _, raw := range []string{"1.0000000000000000001", "1e-999999999"} {
		t.Run(raw, func(t *testing.T) {
			root, registry := callWorkflow(t, "test:workflow/raw-root")
			child, _ := callWorkflow(t, "test:workflow/raw-child")
			child.Definition.Entry = "choose"
			child.Definition.Stages["choose"] = Stage{Kind: "choice", Selection: "first_match", Branches: []ChoiceBranch{{ID: "one", Predicate: Predicate{Op: "eq", Left: conditionLiteral(raw), Right: conditionLiteral("1")}, Next: "done"}}}
			ref := registerCallWorkflow(t, registry, child)
			root.Definition.Entry, root.Definition.Stages["call"] = "call", callStage(ref, "done")
			_, err := CompileProfile(encoded(t, root), "json", registry, CoreProfile)
			expectProblem(t, err, "condition_type_mismatch")
			expectProblem(t, ValidateWorkflowConditions(registry[ref], "json"), "condition_type_mismatch")
		})
	}
}

func TestCoreCallReferenceValidation(t *testing.T) {
	// nested_unsupported compiles under the foundation profile on purpose: an
	// operator a profile does not implement must be refused wherever it sits,
	// including inside a child nobody looked at. Every core operator is now
	// implemented, so the foundation profile is where that property still lives.
	for _, tc := range []struct{ name, code string }{
		{"missing", "missing_ref"}, {"digest", "digest_mismatch"}, {"identity", "ref_identity_mismatch"},
		{"not_workflow", "schema_invalid"}, {"root_identity_conflict", "ref_identity_conflict"}, {"nested_unsupported", "unsupported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, registry := callWorkflow(t, "test:workflow/ref-root")
			child, _ := callWorkflow(t, "test:workflow/ref-child")
			if tc.name == "root_identity_conflict" {
				child.ID = root.ID
			}
			ref := registerCallWorkflow(t, registry, child)
			switch tc.name {
			case "missing":
				delete(registry, ref)
			case "digest":
				child.Title = "Changed after reference was selected"
				registry[ref] = encoded(t, child)
			case "identity":
				other := ref
				other.ID = "test:workflow/forged"
				registry[other], ref = registry[ref], other
			case "not_workflow":
				ref = callStep(t, registry, map[string]InputPort{})
			case "nested_unsupported":
				var future any
				for _, data := range registry {
					var candidate map[string]any
					if json.Unmarshal(data, &candidate) != nil {
						continue
					}
					definition, _ := candidate["definition"].(map[string]any)
					graph, _ := definition["stages"].(map[string]any)
					for _, node := range graph {
						stage, _ := node.(map[string]any)
						if stage["kind"] == "wait" {
							future = stage
						}
					}
					if future != nil {
						break
					}
				}
				if future == nil {
					t.Fatal("fixture has no schema-valid unsupported wait")
				}
				var value map[string]any
				_ = json.Unmarshal(encoded(t, child), &value)
				stages(value)["future"] = future
				data := encoded(t, value)
				digest, err := Digest(data)
				if err != nil {
					t.Fatal(err)
				}
				ref.Digest, registry[Ref{ID: ref.ID, Version: ref.Version, Digest: digest}] = digest, data
			}
			root.Definition.Entry, root.Definition.Stages["call"] = "call", callStage(ref, "done")
			profile := CoreProfile
			if tc.name == "nested_unsupported" {
				profile = Profile
			}
			_, err := CompileProfile(encoded(t, root), "json", registry, profile)
			expectProblem(t, err, tc.code)
		})
	}
}

func TestCoreCallNestedDepthAndSharedValidationBudget(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		rootDepth, childDepth int
		code                  string
	}{
		{"nested depth exact", 2, 1, ""}, {"root depth", 1, 1, "limit_exceeded"}, {"child narrows depth", 2, 0, "limit_exceeded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, registry := callWorkflow(t, "test:workflow/depth-root")
			child, _ := callWorkflow(t, "test:workflow/depth-child")
			leaf, _ := callWorkflow(t, "test:workflow/depth-leaf")
			child.Definition.Entry, child.Definition.Stages["call"] = "call", callStage(registerCallWorkflow(t, registry, leaf), "done")
			root.Limits.MaxChildDepth, child.Limits.MaxChildDepth = tc.rootDepth, tc.childDepth
			root.Definition.Entry, root.Definition.Stages["call"] = "call", callStage(registerCallWorkflow(t, registry, child), "done")
			p, err := CompileProfile(encoded(t, root), "json", registry, CoreProfile)
			if tc.code != "" {
				expectProblem(t, err, tc.code)
				return
			}
			if err != nil || p.bounds.depth != 2 || p.bounds.outcomes["no_work"].transitions != 7 {
				t.Fatal("nested depth/cost mismatch", err)
			}
		})
	}
	// Exact remaining-work boundaries exercise one shared compiler counter;
	// this is not a claim that a million-node workflow has been load tested.
	for _, remaining := range []int{4, 3} {
		root, registry := callWorkflow(t, "test:workflow/work-root")
		child, _ := callWorkflow(t, "test:workflow/work-child")
		root.Definition.Entry, root.Definition.Stages["call"] = "call", callStage(registerCallWorkflow(t, registry, child), "done")
		shared := newCompilation()
		shared.graphWork = 1_000_000 - remaining
		_, err := compileWorkflow(encoded(t, root), "json", registry, CoreProfile, shared)
		if remaining == 3 {
			expectProblem(t, err, "graph_validation_limit")
		} else if err != nil || shared.graphWork != 1_000_000 {
			t.Fatal("child reset or miscounted shared validation work", err)
		}
	}
}

func TestCoreCallChoiceUsesOnlyDeclaredExports(t *testing.T) {
	root, child, registry := callBindingFixture(t)
	root.Outputs = map[string]OutputPort{}
	root.Definition.Stages["done"] = Stage{Kind: "finish", Outcome: "no_work", OutputBindings: map[string]Binding{}}
	stage := root.Definition.Stages["call"]
	stage.On["no_work"], stage.OnError = "choose", "choose"
	root.Definition.Stages["call"] = stage
	root.Definition.Stages["choose"] = Stage{Kind: "choice", Selection: "first_match", Branches: []ChoiceBranch{{ID: "present", Predicate: Predicate{Op: "exists", Ref: &FieldRef{From: "stage_output", StageID: "call", Port: "report"}}, Next: "done"}}, Default: "done"}
	p, err := compileCallFixture(t, root, child, registry)
	if err != nil {
		t.Fatal("optional call export cannot be inspected", err)
	}
	if next, err := p.NextError("call"); err != nil || next != "choose" {
		t.Fatalf("call lost explicit technical error route: %s %v", next, err)
	}
	stage = root.Definition.Stages["choose"]
	stage.Branches[0].Predicate.Ref.StageID = "call/done"
	root.Definition.Stages["choose"] = stage
	_, err = compileCallFixture(t, root, child, registry)
	expectProblem(t, err, "missing_stage")
}
