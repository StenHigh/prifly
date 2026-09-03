package flow

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func checkDefinitionFixture(kind, claim string) CheckDefinition {
	return CheckDefinition{
		SchemaVersion: CheckDefinitionVersion,
		ID:            "example:check/pdf",
		Version:       "1.2.3",
		Title:         "PDF content check",
		Kind:          kind,
		Claim:         claim,
		Executor: Executor{
			AdapterRef: Ref{ID: "core:adapter/local-process", Version: "2.0.0", Digest: "sha256:" + strings.Repeat("a", 64)},
			Operation:  "process",
		},
	}
}

func checkDefinitionProblem(t *testing.T, err error, code string) *Problem {
	t.Helper()
	var problem *Problem
	if !errors.As(err, &problem) || problem.Code != code {
		t.Fatalf("expected %s problem, got %v", code, err)
	}
	return problem
}

func TestCheckDefinitionClosedContract(t *testing.T) {
	for name, definition := range map[string]CheckDefinition{
		"content":         checkDefinitionFixture("content", "content_valid"),
		"result":          checkDefinitionFixture("result", "check_passed"),
		"semantic_result": checkDefinitionFixture("result", "semantic_review"),
	} {
		t.Run(name, func(t *testing.T) {
			data, err := json.Marshal(definition)
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := ParseCheckDefinition(data)
			if err != nil || parsed != definition || ValidateCheckDefinition(definition) != nil {
				t.Fatalf("definition did not round trip: %+v %v", parsed, err)
			}
		})
	}

	base, err := json.Marshal(checkDefinitionFixture("content", "content_valid"))
	if err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(map[string]any){
		"unknown_field":      func(v map[string]any) { v["result_check_refs"] = []any{} },
		"missing_field":      func(v map[string]any) { delete(v, "claim") },
		"null_field":         func(v map[string]any) { v["executor"] = nil },
		"wrong_version":      func(v map[string]any) { v["schema_version"] = "check-definition/2" },
		"unknown_kind":       func(v map[string]any) { v["kind"] = "plugin" },
		"elevated_claim":     func(v map[string]any) { v["claim"] = "semantic_review" },
		"wrong_result_claim": func(v map[string]any) { v["kind"], v["claim"] = "result", "content_valid" },
		"empty_title":        func(v map[string]any) { v["title"] = "" },
		"invalid_id":         func(v map[string]any) { v["id"] = "not an id" },
		"live_adapter": func(v map[string]any) {
			v["executor"].(map[string]any)["adapter_ref"].(map[string]any)["latest"] = true
		},
		"hidden_operation": func(v map[string]any) { v["executor"].(map[string]any)["operation"] = "" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			var value map[string]any
			if err := json.Unmarshal(base, &value); err != nil {
				t.Fatal(err)
			}
			mutate(value)
			data, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ParseCheckDefinition(data); err == nil {
				t.Fatal("invalid definition was accepted")
			}
		})
	}
}

func TestCheckDefinitionStrictInputBounds(t *testing.T) {
	for name, data := range map[string][]byte{
		"duplicate":    []byte(`{"schema_version":"check-definition/1","schema_version":"check-definition/1"}`),
		"array":        []byte(`[]`),
		"trailing":     []byte(`{} {}`),
		"invalid_utf8": {0xff},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseCheckDefinition(data); err == nil {
				t.Fatal("invalid input was accepted")
			}
		})
	}
	large := checkDefinitionFixture("content", "content_valid")
	large.Title = strings.Repeat("x", 257)
	checkDefinitionProblem(t, ValidateCheckDefinition(large), "invalid_check_definition")
	large.Title = string([]byte{0xff})
	checkDefinitionProblem(t, ValidateCheckDefinition(large), "invalid_check_definition")

	data := append([]byte(`{"schema_version":"check-definition/1","padding":"`), make([]byte, MaxDocumentBytes)...)
	data = append(data, []byte(`"}`)...)
	if _, err := ParseCheckDefinition(data); err == nil {
		t.Fatal("oversized definition was accepted")
	}
}

func TestCheckDefinitionSafeIntegerPreflight(t *testing.T) {
	for raw, want := range map[string]int64{"3": 3, "3.0": 3, "3e0": 3, "0.003e3": 3, "-0": 0, "9007199254740991": 9007199254740991, "-9007199254740991": -9007199254740991} {
		if got, ok := ParseSafeInteger(raw); !ok || got != want {
			t.Fatalf("exact integer %s = %d, %t; want %d", raw, got, ok, want)
		}
	}
	for _, raw := range []string{"3.0000000000000000001", "9007199254740990.9", "9007199254740992", "1e999999999999999999", "01", "NaN", `"3"`} {
		if value, ok := ParseSafeInteger(raw); ok {
			t.Fatalf("invalid exact integer %s accepted as %d", raw, value)
		}
	}
}

func checkComponent(t *testing.T, registry Registry, id, version string, value any) Ref {
	t.Helper()
	data := encoded(t, value)
	digest, err := Digest(data)
	if err != nil {
		t.Fatal(err)
	}
	ref := Ref{ID: id, Version: version, Digest: digest}
	registry[ref] = data
	return ref
}

func changeCheckedStep(t *testing.T, workflow *WorkflowRevision, registry Registry, mutate func(*StepDefinition)) {
	t.Helper()
	stage := workflow.Definition.Stages["work"]
	var step StepDefinition
	if err := json.Unmarshal(registry[stage.StepRef], &step); err != nil {
		t.Fatal(err)
	}
	mutate(&step)
	stage.StepRef = checkComponent(t, registry, step.ID, step.Version, step)
	workflow.Definition.Stages["work"] = stage
}

func checkCompileFixture(t *testing.T) (WorkflowRevision, Registry, Ref, Ref) {
	t.Helper()
	w, registry := contextWorkflow(t, nil, nil)
	var step StepDefinition
	if err := json.Unmarshal(registry[w.Definition.Stages["work"].StepRef], &step); err != nil {
		t.Fatal(err)
	}
	content := checkDefinitionFixture("content", "content_valid")
	content.ID, content.Executor.AdapterRef, content.Executor.Operation = "test:check/content", step.Executor.AdapterRef, "check"
	contentRef := checkComponent(t, registry, content.ID, content.Version, content)
	result := checkDefinitionFixture("result", "check_passed")
	result.ID, result.Executor = "test:check/result", content.Executor
	resultRef := checkComponent(t, registry, result.ID, result.Version, result)
	schema := checkComponent(t, registry, "test:schema/check", "1.0.0", true)
	port := Port{Format: "json", SchemaRef: &schema, ContentCheckRefs: []Ref{contentRef}}
	w.Inputs = map[string]InputPort{"value": {Port: port, Required: true}}
	w.Outputs = map[string]OutputPort{"report": {Port: port, RequiredFor: []string{"no_work"}}}
	changeCheckedStep(t, &w, registry, func(step *StepDefinition) {
		step.Inputs = map[string]InputPort{"data": {Port: port, Required: true}}
		step.Outputs = map[string]OutputPort{"report": {Port: port, RequiredFor: []string{"pass"}}}
		step.ResultCheckRefs = []Ref{resultRef}
	})
	stage := w.Definition.Stages["work"]
	stage.InputBindings = map[string]Binding{"data": {From: "workflow_input", Port: "value"}}
	w.Definition.Stages["work"] = stage
	w.Definition.Stages["done"] = Stage{Kind: "finish", Outcome: "no_work", OutputBindings: map[string]Binding{"report": {From: "stage_output", StageID: "work", Port: "report"}}}
	return w, registry, contentRef, resultRef
}

func TestCoreCheckClosureIsOptInAndChargesEveryBoundary(t *testing.T) {
	w, registry, contentRef, resultRef := checkCompileFixture(t)
	w.Limits.MaxControlTransitions, w.Limits.MaxStepInstances = 7, 1
	p, err := CompileCore(encoded(t, w), "json", registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.Resources == nil || p.Checks == nil || len(p.Checks) != 2 || p.Checks[contentRef].Kind != "content" || p.Checks[resultRef].Kind != "result" {
		t.Fatal("exact check closure or explicit Core opt-in marker missing")
	}
	if p.bounds.outcomes["no_work"] != (executionCost{steps: 1, transitions: 7}) || len(p.Steps) != 1 || len(p.Sequence) != 2 {
		t.Fatal("checks were deduplicated across uses or became fake steps/stages", p.bounds)
	}
	if _, exists := p.Registry[p.Checks[contentRef].Executor.AdapterRef]; !exists {
		t.Fatal("checker adapter was not pinned")
	}
	w.Limits.MaxControlTransitions--
	_, err = CompileCore(encoded(t, w), "json", registry, nil)
	expectProblem(t, err, "limit_exceeded")
	w.Limits.MaxControlTransitions++
	for _, profile := range []string{Profile, CoreProfile} {
		_, err := CompileProfile(encoded(t, w), "json", registry, profile)
		expectProblem(t, err, "unsupported")
	}
	_, err = Compile(encoded(t, w), "json", registry)
	expectProblem(t, err, "unsupported")

	want := bytes.Clone(p.Registry[contentRef])
	registry[contentRef][0] = 'X'
	if !bytes.Equal(p.Registry[contentRef], want) || p.Checks[contentRef].Kind != "content" {
		t.Fatal("parsed check closure aliases mutable registry bytes")
	}

	plain, registry := contextWorkflow(t, nil, nil)
	legacy, err := CompileProfile(encoded(t, plain), "json", registry, CoreProfile)
	if err != nil || legacy.Resources != nil || legacy.Checks != nil {
		t.Fatal("legacy compiler gained opt-in markers", err)
	}
}

func TestCoreCheckKindsMatchEveryDeclaringPosition(t *testing.T) {
	for _, position := range []string{"workflow_input", "workflow_output", "step_input", "step_output", "step_result"} {
		t.Run(position, func(t *testing.T) {
			w, registry, contentRef, resultRef := checkCompileFixture(t)
			switch position {
			case "workflow_input":
				port := w.Inputs["value"]
				port.ContentCheckRefs = []Ref{resultRef}
				w.Inputs["value"] = port
			case "workflow_output":
				port := w.Outputs["report"]
				port.ContentCheckRefs = []Ref{resultRef}
				w.Outputs["report"] = port
			default:
				changeCheckedStep(t, &w, registry, func(step *StepDefinition) {
					switch position {
					case "step_input":
						port := step.Inputs["data"]
						port.ContentCheckRefs = []Ref{resultRef}
						step.Inputs["data"] = port
					case "step_output":
						port := step.Outputs["report"]
						port.ContentCheckRefs = []Ref{resultRef}
						step.Outputs["report"] = port
					case "step_result":
						step.ResultCheckRefs = []Ref{contentRef}
					}
				})
			}
			_, err := CompileCore(encoded(t, w), "json", registry, nil)
			failure := expectProblem(t, err, "check_kind_mismatch")
			if !strings.HasSuffix(failure.Path, "_check_refs/0") {
				t.Fatal("kind diagnostic lost its declaring position", failure)
			}
		})
	}
}

func TestCoreCheckDefinitionsRejectRecursiveAndUnpinnedDependencies(t *testing.T) {
	for _, mode := range []string{"recursive", "missing_adapter", "digest", "ordinary_step", "context_resource"} {
		t.Run(mode, func(t *testing.T) {
			w, registry, contentRef, _ := checkCompileFixture(t)
			var definition CheckDefinition
			if err := json.Unmarshal(registry[contentRef], &definition); err != nil {
				t.Fatal(err)
			}
			definition.ID = "test:check/invalid"
			ref := checkComponent(t, registry, definition.ID, definition.Version, definition)
			resources := ContextResources{}
			code := "invalid_check_definition"
			switch mode {
			case "recursive":
				var value map[string]any
				if err := json.Unmarshal(registry[ref], &value); err != nil {
					t.Fatal(err)
				}
				value["result_check_refs"] = []Ref{{ID: "test:missing", Version: "1.0.0", Digest: "sha256:" + strings.Repeat("f", 64)}}
				ref = checkComponent(t, registry, definition.ID, definition.Version, value)
			case "missing_adapter":
				definition.Executor.AdapterRef = Ref{ID: "test:adapter/missing", Version: "1.0.0", Digest: "sha256:" + strings.Repeat("f", 64)}
				ref = checkComponent(t, registry, definition.ID, definition.Version, definition)
				code = "missing_ref"
			case "digest":
				registry[ref] = []byte(`{}`)
				code = "digest_mismatch"
			case "ordinary_step":
				ref = w.Definition.Stages["work"].StepRef
			case "context_resource":
				resources[ref] = ContextResource{ByteEncoding: "json", MediaType: "application/json", Bytes: registry[ref]}
				code = "resource_type_mismatch"
			}
			port := w.Inputs["value"]
			port.ContentCheckRefs = []Ref{ref}
			w.Inputs["value"] = port
			_, err := CompileCore(encoded(t, w), "json", registry, resources)
			expectProblem(t, err, code)
		})
	}
}

func TestCoreCheckNestedCostsCountEachInvocation(t *testing.T) {
	for _, kind := range []string{"call", "repeat", "repeat_call"} {
		t.Run(kind, func(t *testing.T) {
			child, registry, contentRef, resultRef := checkCompileFixture(t)
			childRef := registerCallWorkflow(t, registry, child)
			root, _ := callWorkflow(t, "test:workflow/check-parent")
			input := child.Inputs["value"]
			input.ContentCheckRefs = nil
			root.Inputs["value"] = input
			binding := Binding{From: "workflow_input", Port: "value"}
			root.Definition.Entry = "first"
			want := executionCost{steps: 2, transitions: 19}
			if kind == "call" {
				first, second := callStage(childRef, "second"), callStage(childRef, "done")
				first.InputBindings["value"], second.InputBindings["value"] = binding, binding
				root.Definition.Stages["first"], root.Definition.Stages["second"] = first, second
			} else {
				want = executionCost{steps: 3, transitions: 26}
				if kind == "repeat_call" {
					wrapper, _ := callWorkflow(t, "test:workflow/check-wrapper")
					wrapper.Inputs["value"] = input
					wrapper.Definition.Entry = "call"
					stage := callStage(childRef, "done")
					stage.InputBindings["value"] = binding
					wrapper.Definition.Stages["call"] = stage
					childRef = registerCallWorkflow(t, registry, wrapper)
					want.transitions = 35
				}
				stage := repeatStage(childRef, "done", 3)
				stage.InitialBindings["value"], stage.NextBindings["value"] = binding, binding
				root.Definition.Stages["first"] = stage
			}
			root.Limits.MaxControlTransitions, root.Limits.MaxStepInstances = want.transitions, want.steps
			plan, err := CompileCore(encoded(t, root), "json", registry, nil)
			if err != nil {
				t.Fatal(err)
			}
			if plan.bounds.outcomes["no_work"] != want || len(plan.Steps) != 0 || len(plan.Checks) != 2 {
				t.Fatal("nested check costs or local stage identities were lost", plan.bounds)
			}
			body := plan.BodyPlan("first")
			if body.Checks[contentRef] != plan.Checks[contentRef] || body.Checks[resultRef] != plan.Checks[resultRef] {
				t.Fatal("nested check closure differs from root")
			}
			if kind == "call" && plan.Calls["first"] != plan.Calls["second"] {
				t.Fatal("reused exact definition was expanded into per-call plans")
			}
			root.Limits.MaxControlTransitions--
			_, err = CompileCore(encoded(t, root), "json", registry, nil)
			expectProblem(t, err, "limit_exceeded")
			root.Limits.MaxControlTransitions++
			root.Limits.MaxStepInstances--
			_, err = CompileCore(encoded(t, root), "json", registry, nil)
			expectProblem(t, err, "limit_exceeded")
		})
	}
}

func TestCoreCheckFailedPrefixesKeepCostsButNoOutputs(t *testing.T) {
	for _, kind := range []string{"step", "call", "repeat"} {
		t.Run(kind, func(t *testing.T) {
			checked, registry, _, resultRef := checkCompileFixture(t)
			root, controls := checked, int64(8)
			if kind != "step" {
				childRef := registerCallWorkflow(t, registry, checked)
				root, _ = callWorkflow(t, "test:workflow/check-failure")
				input := checked.Inputs["value"]
				input.ContentCheckRefs = nil
				root.Inputs["value"] = input
				root.Definition.Entry = "work"
				binding := Binding{From: "workflow_input", Port: "value"}
				stage := callStage(childRef, "done")
				stage.InputBindings["value"] = binding
				controls = 12
				if kind == "repeat" {
					stage = repeatStage(childRef, "done", 3)
					stage.InitialBindings["value"], stage.NextBindings["value"] = binding, binding
					controls = 28
				}
				root.Definition.Stages["work"] = stage
			}
			root.AllowedOutcomes = []string{"no_work", "rejected"}
			stage := root.Definition.Stages["work"]
			stage.OnError = "repair"
			root.Definition.Stages["work"] = stage
			repairRef := callStep(t, registry, map[string]InputPort{})
			var repair StepDefinition
			if err := json.Unmarshal(registry[repairRef], &repair); err != nil {
				t.Fatal(err)
			}
			repair.ResultCheckRefs = []Ref{resultRef}
			repairRef = checkComponent(t, registry, repair.ID, repair.Version, repair)
			root.Definition.Stages["repair"] = Stage{Kind: "step", StepRef: repairRef, InputBindings: map[string]Binding{}, On: map[string]string{"pass": "rejected"}}
			root.Definition.Stages["rejected"] = Stage{Kind: "finish", Outcome: "rejected", OutputBindings: map[string]Binding{}}
			root.Limits.MaxControlTransitions = controls
			plan, err := CompileCore(encoded(t, root), "json", registry, nil)
			if err != nil {
				t.Fatal(err)
			}
			if plan.bounds.outcomes["rejected"].transitions != controls {
				t.Fatal("on_error lost the potentially failed check prefix", plan.bounds)
			}
			root.Limits.MaxControlTransitions--
			_, err = CompileCore(encoded(t, root), "json", registry, nil)
			expectProblem(t, err, "limit_exceeded")
			root.Limits.MaxControlTransitions++
			// A content/result check can fail after the producer wrote files.
			// Those candidates must not become guaranteed stage outputs.
			port := checked.Inputs["value"]
			port.ContentCheckRefs = nil
			repair.Inputs = map[string]InputPort{"candidate": port}
			repairStage := root.Definition.Stages["repair"]
			repairStage.StepRef = checkComponent(t, registry, repair.ID, repair.Version, repair)
			repairStage.InputBindings["candidate"] = Binding{From: "stage_output", StageID: "work", Port: "report"}
			root.Definition.Stages["repair"] = repairStage
			_, err = CompileCore(encoded(t, root), "json", registry, nil)
			expectProblem(t, err, "unavailable_output")
		})
	}
}

func TestCoreCheckFinishCostsFollowExportsAndOptionalBounds(t *testing.T) {
	w, registry, _, _ := checkCompileFixture(t)
	w.AllowedOutcomes = []string{"no_work", "rejected"}
	stage := w.Definition.Stages["work"]
	stage.On["fail"] = "rejected"
	w.Definition.Stages["work"] = stage
	w.Definition.Stages["rejected"] = Stage{Kind: "finish", Outcome: "rejected", OutputBindings: map[string]Binding{}}
	optional := w.Outputs["report"]
	optional.RequiredFor = []string{}
	w.Outputs["unbound"] = optional
	plan, err := CompileCore(encoded(t, w), "json", registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.bounds.outcomes["no_work"].transitions != 7 || plan.bounds.outcomes["rejected"].transitions != 6 {
		t.Fatal("unexported workflow ports were charged or optional step outputs were omitted", plan.bounds)
	}

	// A workflow input check does not create an executor stage or StepInstance.
	w.Definition.Entry = "done"
	w.Definition.Stages = map[string]Stage{"done": {Kind: "finish", Outcome: "no_work", OutputBindings: map[string]Binding{}}}
	w.Outputs = map[string]OutputPort{}
	w.Limits.MaxStepInstances, w.Limits.MaxControlTransitions = 1, 2
	plan, err = CompileCore(encoded(t, w), "json", registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.bounds.outcomes["no_work"] != (executionCost{transitions: 2}) || len(plan.Steps) != 0 {
		t.Fatal("workflow input check became a fake StepInstance", plan.bounds)
	}
}

func TestCoreCheckRefShapedInstanceDataIsNotExecutable(t *testing.T) {
	w, registry, _, _ := checkCompileFixture(t)
	stage := w.Definition.Stages["work"]
	stage.InputBindings["data"] = Binding{From: "literal", SchemaRef: w.Inputs["value"].SchemaRef, Value: json.RawMessage(`{"format":"json","content_check_refs":[{"id":"test:missing","version":"1.0.0","digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]}`)}
	w.Definition.Stages["work"] = stage
	plan, err := CompileCore(encoded(t, w), "json", registry, nil)
	if err != nil {
		t.Fatal("literal payload was followed as check configuration", err)
	}
	if len(plan.Checks) != 2 || plan.bounds.outcomes["no_work"].transitions != 7 {
		t.Fatal("instance data created checks or changed their charge")
	}
}
