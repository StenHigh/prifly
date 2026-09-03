package flow

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestMain(m *testing.M) {
	if handled, code := SchemaWorker(os.Args[1:], os.Stdin, os.Stdout); handled {
		os.Exit(code)
	}
	os.Exit(m.Run())
}

func fixture(t *testing.T) (map[string]any, Registry) {
	t.Helper()
	workflowBytes, err := os.ReadFile("../../test/fixtures/foundation/two-checks.workflow.json")
	if err != nil {
		t.Fatal(err)
	}
	registryBytes, err := os.ReadFile("../../test/fixtures/contracts/workflow-examples.json")
	if err != nil {
		t.Fatal(err)
	}
	var workflow map[string]any
	if err := json.Unmarshal(workflowBytes, &workflow); err != nil {
		t.Fatal(err)
	}
	var bundle map[string]json.RawMessage
	if err := json.Unmarshal(registryBytes, &bundle); err != nil {
		t.Fatal(err)
	}
	registry := make(Registry)
	for _, group := range []string{"components", "resources", "external_dependencies"} {
		var entries []struct {
			Ref   Ref             `json:"ref"`
			Value json.RawMessage `json:"value"`
		}
		if err := json.Unmarshal(bundle[group], &entries); err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			registry[entry.Ref] = entry.Value
		}
	}
	return workflow, registry
}

func encoded(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func stages(workflow map[string]any) map[string]any {
	return workflow["definition"].(map[string]any)["stages"].(map[string]any)
}

func expectProblem(t *testing.T, err error, code string) *Problem {
	t.Helper()
	var p *Problem
	if !errors.As(err, &p) || (code != "" && p.Code != code) {
		t.Fatalf("expected problem %q, got %v", code, err)
	}
	return p
}

func TestSchemaProblemsNeverExposeSchemaURLsOrValues(t *testing.T) {
	const secret = "CANARY-SECRET-SCHEMA-CONTENT"
	url := "https://example.invalid/schema?token=" + secret
	for _, test := range []struct {
		name   string
		schema any
		value  any
		code   string
	}{
		{"external loader", map[string]any{"$ref": url}, true, "invalid_schema"},
		{"invalid schema with id", map[string]any{"$id": url, "type": secret}, true, "invalid_schema"},
		{"invalid pattern", map[string]any{"pattern": "(" + secret}, secret, "invalid_schema"},
		{"validation source URL", map[string]any{"$id": url, "type": "integer"}, secret, "schema_invalid"},
		{"bounded instance pointer", map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "integer"}}, map[string]any{strings.Repeat("long", 2048): secret}, "schema_invalid"},
	} {
		t.Run(test.name, func(t *testing.T) {
			schema := encoded(t, test.schema)
			digest, err := Digest(schema)
			if err != nil {
				t.Fatal(err)
			}
			ref := Ref{ID: "test:schema/redaction", Version: "1.0.0", Digest: digest}
			err = ValidateSchema(Registry{ref: schema}, ref, encoded(t, test.value))
			p := expectProblem(t, err, test.code)
			if strings.Contains(p.Error(), secret) || strings.Contains(p.Error(), "example.invalid") || len(p.Path) > 1024 {
				t.Fatalf("schema diagnostic leaked an input value/URL or unbounded pointer: %+v", p)
			}
		})
	}
	_, err := ProtocolSchema(url)
	p := expectProblem(t, err, "unsupported_contract")
	if strings.Contains(p.Error(), secret) {
		t.Fatal("unknown contract name was copied into a public error")
	}
	// Workflow-level wrapping must preserve the safe message, not reconstruct
	// the rejected resource URL from dependency compiler errors.
	w, registry := fixture(t)
	schema := encoded(t, map[string]any{"$ref": url})
	digest, _ := Digest(schema)
	ref := Ref{ID: "test:schema/redaction", Version: "1.0.0", Digest: digest}
	registry[ref] = schema
	w["inputs"].(map[string]any)["first"].(map[string]any)["schema_ref"] = ref
	_, err = Compile(encoded(t, w), "json", registry)
	p = expectProblem(t, err, "invalid_schema_ref")
	if strings.Contains(p.Error(), secret) || strings.Contains(p.Error(), "example.invalid") || p.Path != "/inputs/first/schema_ref" {
		t.Fatalf("workflow wrapper leaked schema source: %+v", p)
	}
}

func TestFoundationFixture(t *testing.T) {
	workflow, registry := fixture(t)
	plan, err := Compile(encoded(t, workflow), "json", registry)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(plan.Sequence, []string{"check_first", "check_second", "done"}) || len(plan.Steps) != 2 {
		t.Fatalf("unexpected plan: %v", plan.Sequence)
	}
	if plan.Steps["check_first"].ID != plan.Steps["check_second"].ID {
		t.Fatal("two activations should use the same definition")
	}
	if err := ValidateProtocol("WorkflowRevision", encoded(t, plan.Workflow)); err != nil {
		t.Fatalf("typed workflow no longer encodes its closed stage variants: %v", err)
	}
	for _, test := range []struct{ stage, verdict, target string }{
		{"check_first", "pass", "check_second"},
		{"check_first", "fail", "rejected_first"},
		{"check_second", "fail", "rejected_second"},
		{"check_second", "pass", "done"},
	} {
		target, err := plan.Next(test.stage, test.verdict)
		if err != nil || target != test.target {
			t.Fatalf("%s/%s -> %s, %v", test.stage, test.verdict, target, err)
		}
	}
	_, err = plan.Next("check_first", "needs_revision")
	expectProblem(t, err, "unhandled_verdict")
	_, err = plan.Next("done", "pass")
	expectProblem(t, err, "invalid_stage")
	_, err = plan.Next("check_first", "skipped")
	expectProblem(t, err, "invalid_verdict")
	inputSchema := *plan.Workflow.Inputs["first"].SchemaRef
	if err := plan.ValidateJSON(inputSchema, []byte(`{"key":"one","text":"valid"}`)); err != nil {
		t.Fatal(err)
	}
	expectProblem(t, plan.ValidateJSON(inputSchema, []byte(`{"key":"one","text":5}`)), "schema_invalid")
	// Compiled bytes are detached from the caller's inventory.
	for ref := range registry {
		registry[ref] = []byte(`{"replaced":true}`)
	}
	if err := plan.ValidateJSON(inputSchema, []byte(`{"key":"one","text":"still valid"}`)); err != nil {
		t.Fatal(err)
	}
}

func TestFoundationValidationFailures(t *testing.T) {
	tests := []struct {
		name, code string
		edit       func(map[string]any)
	}{
		{"unknown field", "schema_invalid", func(w map[string]any) { w["magic"] = true }},
		{"cycle", "cycle", func(w map[string]any) {
			stages(w)["check_second"].(map[string]any)["on"].(map[string]any)["pass"] = "check_first"
		}},
		{"missing target", "missing_stage", func(w map[string]any) {
			stages(w)["check_second"].(map[string]any)["on"].(map[string]any)["pass"] = "missing"
		}},
		{"missing entry", "missing_stage", func(w map[string]any) { w["definition"].(map[string]any)["entry"] = "missing" }},
		{"unreachable finish", "unreachable_stage", func(w map[string]any) { stages(w)["unreachable"] = stages(w)["done"] }},
		{"unreachable step", "unreachable_stage", func(w map[string]any) { stages(w)["unreachable"] = stages(w)["check_first"] }},
		{"missing pass", "missing_handler", func(w map[string]any) {
			delete(stages(w)["check_first"].(map[string]any)["on"].(map[string]any), "pass")
		}},
		{"negative nonterminal", "unsupported", func(w map[string]any) {
			stages(w)["check_first"].(map[string]any)["on"].(map[string]any)["fail"] = "check_second"
		}},
		{"negative success", "invalid_outcome", func(w map[string]any) {
			stages(w)["check_first"].(map[string]any)["on"].(map[string]any)["fail"] = "done"
		}},
		{"bad negative outcome", "invalid_outcome", func(w map[string]any) { stages(w)["rejected_first"].(map[string]any)["outcome"] = "no_work" }},
		{"undeclared outcome", "invalid_outcome", func(w map[string]any) { w["allowed_outcomes"] = []string{"succeeded"} }},
		{"unsupported outcome", "unsupported", func(w map[string]any) { w["allowed_outcomes"] = []string{"succeeded", "partial"} }},
		{"missing required export", "missing_binding", func(w map[string]any) {
			delete(stages(w)["done"].(map[string]any)["output_bindings"].(map[string]any), "report_second")
		}},
		{"output from future path", "unavailable_output", func(w map[string]any) {
			stages(w)["rejected_first"].(map[string]any)["output_bindings"].(map[string]any)["report_second"] = map[string]any{"from": "stage_output", "stage_id": "check_second", "port": "report"}
		}},
		{"unknown export", "unknown_port", func(w map[string]any) {
			b := stages(w)["done"].(map[string]any)["output_bindings"].(map[string]any)
			b["unexpected"] = b["report_first"]
		}},
		{"missing required input", "missing_binding", func(w map[string]any) { stages(w)["check_first"].(map[string]any)["input_bindings"] = map[string]any{} }},
		{"optional source required target", "unavailable_output", func(w map[string]any) { w["inputs"].(map[string]any)["first"].(map[string]any)["required"] = false }},
		{"unknown input port", "unknown_port", func(w map[string]any) {
			stages(w)["check_first"].(map[string]any)["input_bindings"].(map[string]any)["document"].(map[string]any)["port"] = "missing"
		}},
		{"source schema mismatch", "port_type_mismatch", func(w map[string]any) {
			w["inputs"].(map[string]any)["first"].(map[string]any)["schema_ref"] = w["outputs"].(map[string]any)["report_first"].(map[string]any)["schema_ref"]
		}},
		{"insufficient steps", "limit_exceeded", func(w map[string]any) { w["limits"].(map[string]any)["max_step_instances"] = 1 }},
		{"insufficient transitions", "limit_exceeded", func(w map[string]any) { w["limits"].(map[string]any)["max_control_transitions"] = 2 }},
		{"parallel capacity", "unsupported", func(w map[string]any) { w["limits"].(map[string]any)["max_parallelism"] = 2 }},
		{"child capacity", "unsupported", func(w map[string]any) { w["limits"].(map[string]any)["max_child_depth"] = 1 }},
		{"on_error", "unsupported", func(w map[string]any) { stages(w)["check_first"].(map[string]any)["on_error"] = "rejected_first" }},
		{"projection", "unsupported", func(w map[string]any) {
			b := stages(w)["check_first"].(map[string]any)["input_bindings"].(map[string]any)["document"].(map[string]any)
			b["pointer"] = ""
			b["projected_schema_ref"] = w["inputs"].(map[string]any)["first"].(map[string]any)["schema_ref"]
		}},
		{"iteration binding", "unsupported", func(w map[string]any) {
			stages(w)["check_first"].(map[string]any)["input_bindings"].(map[string]any)["document"] = map[string]any{"from": "iteration_output", "port": "document"}
		}},
		{"skip route", "unsupported", func(w map[string]any) {
			stages(w)["check_first"].(map[string]any)["on"].(map[string]any)["skipped"] = "done"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workflow, registry := fixture(t)
			test.edit(workflow)
			_, err := Compile(encoded(t, workflow), "json", registry)
			expectProblem(t, err, test.code)
		})
	}
}

func TestUnsupportedUnreachableOperators(t *testing.T) {
	workflow, registry := fixture(t)
	seen := make(map[string]bool)
	for _, bytes := range registry {
		var candidate map[string]any
		if err := json.Unmarshal(bytes, &candidate); err != nil {
			continue // The registry also contains literal JSON context strings.
		}
		definition, _ := candidate["definition"].(map[string]any)
		candidateStages, _ := definition["stages"].(map[string]any)
		for _, value := range candidateStages {
			stage := value.(map[string]any)
			kind := stage["kind"].(string)
			if kind == "step" || kind == "finish" || seen[kind] {
				continue
			}
			seen[kind] = true
			t.Run(kind, func(t *testing.T) {
				stages(workflow)["unreachable_future"] = stage
				for _, profile := range []string{Profile, CoreProfile} {
					if profile == CoreProfile && (kind == "choice" || kind == "call" || kind == "repeat" || kind == "parallel" || kind == "map" || kind == "wait") {
						continue // Supported Core operators have independent graph cases.
					}
					_, err := CompileProfile(encoded(t, workflow), "json", registry, profile)
					p := expectProblem(t, err, "unsupported")
					if p.Path != "/definition/stages/unreachable_future/kind" {
						t.Fatalf("wrong pointer for %s: %s", profile, p.Path)
					}
				}
			})
		}
	}
	for _, kind := range []string{"choice", "call", "repeat", "parallel", "map", "wait"} {
		if !seen[kind] {
			t.Errorf("fixture has no %s operator", kind)
		}
	}
}

func TestCoreGraphAvailability(t *testing.T) {
	for _, test := range []struct {
		name, code string
		edit       func(map[string]any)
	}{
		{"foundation graph", "", func(map[string]any) {}},
		{"negative recovery", "", func(w map[string]any) {
			stages(w)["check_first"].(map[string]any)["on"].(map[string]any)["fail"] = "check_second"
			delete(stages(w), "rejected_first")
		}},
		{"branch budget is longest path not total steps", "", func(w map[string]any) {
			first := stages(w)["check_first"].(map[string]any)
			first["on"].(map[string]any)["fail"] = "recover"
			stages(w)["recover"] = map[string]any{"kind": "step", "step_ref": first["step_ref"], "input_bindings": first["input_bindings"], "on": map[string]any{"pass": "rejected_first"}}
		}},
		{"error recovery lacks producer output", "unavailable_output", func(w map[string]any) {
			stages(w)["check_first"].(map[string]any)["on_error"] = "check_second"
		}},
		{"error recovery with separate inputs", "", func(w map[string]any) {
			stages(w)["check_first"].(map[string]any)["on_error"] = "check_second"
			w["outputs"] = map[string]any{}
			for _, stage := range stages(w) {
				if stage.(map[string]any)["kind"] == "finish" {
					stage.(map[string]any)["output_bindings"] = map[string]any{}
				}
			}
		}},
		{"merge bypasses required producer", "unavailable_output", func(w map[string]any) {
			stages(w)["check_first"].(map[string]any)["on"].(map[string]any)["fail"] = "done"
			delete(stages(w), "rejected_first")
		}},
		{"optional producer absent on one path", "", func(w map[string]any) {
			stages(w)["check_first"].(map[string]any)["on"].(map[string]any)["fail"] = "done"
			delete(stages(w), "rejected_first")
			w["outputs"].(map[string]any)["report_second"].(map[string]any)["required_for"] = []any{}
		}},
		{"optional future producer", "unavailable_output", func(w map[string]any) {
			stages(w)["rejected_first"].(map[string]any)["output_bindings"].(map[string]any)["report_second"] = map[string]any{"from": "stage_output", "stage_id": "check_second", "port": "report"}
		}},
		{"same outcome still requires every export", "missing_binding", func(w map[string]any) {
			w["outputs"].(map[string]any)["report_second"].(map[string]any)["required_for"] = []string{"succeeded", "rejected"}
		}},
		{"missing pass is unresolved at runtime", "", func(w map[string]any) {
			delete(stages(w)["check_first"].(map[string]any)["on"].(map[string]any), "pass")
			for _, id := range []string{"check_second", "done", "rejected_second"} {
				delete(stages(w), id)
			}
		}},
		{"missing error target", "missing_stage", func(w map[string]any) {
			stages(w)["check_first"].(map[string]any)["on_error"] = "missing"
		}},
		{"error cycle", "cycle", func(w map[string]any) {
			stages(w)["check_first"].(map[string]any)["on_error"] = "check_first"
		}},
		{"unreachable", "unreachable_stage", func(w map[string]any) { stages(w)["orphan"] = stages(w)["done"] }},
		{"step budget", "limit_exceeded", func(w map[string]any) { w["limits"].(map[string]any)["max_step_instances"] = 1 }},
		{"transition budget", "limit_exceeded", func(w map[string]any) { w["limits"].(map[string]any)["max_control_transitions"] = 2 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			workflow, registry := fixture(t)
			test.edit(workflow)
			plan, err := CompileProfile(encoded(t, workflow), "json", registry, CoreProfile)
			if test.code != "" {
				expectProblem(t, err, test.code)
				return
			}
			if err != nil || plan.Profile != CoreProfile || len(plan.Sequence) != len(stages(workflow)) {
				t.Fatalf("core graph not compiled: %+v %v", plan, err)
			}
			if err := ValidateProtocol("WorkflowRevision", encoded(t, plan.Workflow)); err != nil {
				t.Fatal("typed definition lost closed fields", err)
			}
			stage := plan.Workflow.Definition.Stages["check_first"]
			next, err := plan.NextError("check_first")
			if stage.OnError == "" {
				expectProblem(t, err, "unhandled_error")
			} else if err != nil || next != stage.OnError {
				t.Fatalf("wrong error transition: %s %v", next, err)
			}
			if _, exists := stage.On["pass"]; !exists {
				_, err := plan.Next("check_first", "pass")
				expectProblem(t, err, "unhandled_verdict")
			}
		})
	}
	workflow, registry := fixture(t)
	_, err := CompileProfile(encoded(t, workflow), "json", registry, "future/1")
	expectProblem(t, err, "unsupported_profile")
	plan, err := Compile(encoded(t, workflow), "json", registry)
	if err != nil || plan.Profile != Profile {
		t.Fatalf("default profile changed: %v", err)
	}
	_, err = plan.NextError("check_first")
	expectProblem(t, err, "unsupported")

	t.Run("bounded dataflow proof", func(t *testing.T) {
		p := &Plan{Profile: CoreProfile, Steps: map[string]StepDefinition{}}
		p.Workflow.Inputs, p.Workflow.Outputs = map[string]InputPort{}, map[string]OutputPort{}
		p.Workflow.AllowedOutcomes = []string{"succeeded"}
		p.Workflow.Limits = Limits{MaxStepInstances: 2000, MaxControlTransitions: 2001, MaxParallelism: 1}
		p.Workflow.Definition.Entry = "s0"
		p.Workflow.Definition.Stages = map[string]Stage{"done": {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]Binding{}}}
		port := Port{Format: "blob", MediaTypes: []string{"text/plain"}}
		for i := range 2000 {
			id, next := fmt.Sprintf("s%d", i), fmt.Sprintf("s%d", i+1)
			if i == 1999 {
				next = "done"
			}
			step := StepDefinition{Inputs: map[string]InputPort{}, Outputs: map[string]OutputPort{"value": {Port: port, RequiredFor: []string{"pass"}}}}
			stage := Stage{Kind: "step", InputBindings: map[string]Binding{}, On: map[string]string{"pass": next}}
			if i > 0 {
				step.Inputs["value"] = InputPort{Port: port, Required: true}
				stage.InputBindings["value"] = Binding{From: "stage_output", StageID: fmt.Sprintf("s%d", i-1), Port: "value"}
			}
			p.Steps[id], p.Workflow.Definition.Stages[id] = step, stage
		}
		expectProblem(t, p.checkCoreGraph(), "graph_validation_limit")
	})
}

func TestCoreChoiceGraph(t *testing.T) {
	optionalProducer := func(w map[string]any, choice *Stage) {
		first := stages(w)["check_first"].(map[string]any)["on"].(map[string]any)
		first["pass"], first["fail"] = "check_second", "choose"
		stages(w)["check_second"].(map[string]any)["on"].(map[string]any)["pass"] = "choose"
		choice.Branches[0].Next, choice.Default = "done", "rejected_first"
		choice.Branches[0].Predicate.Ref.StageID = "check_second"
	}
	for _, tc := range []struct {
		name, code string
		edit       func(map[string]any, *Stage)
	}{
		{"closed choice", "", func(map[string]any, *Stage) {}},
		{"missing branch target", "missing_stage", func(_ map[string]any, s *Stage) { s.Branches[0].Next = "missing" }},
		{"missing default target", "missing_stage", func(_ map[string]any, s *Stage) { s.Default = "missing" }},
		{"missing unknown target", "missing_stage", func(_ map[string]any, s *Stage) { s.OnUnknown = "missing" }},
		{"missing error target", "missing_stage", func(_ map[string]any, s *Stage) { s.OnError = "missing" }},
		{"no implicit default required", "", func(_ map[string]any, s *Stage) { s.Default = "" }},
		{"no implicit unknown handler required", "", func(_ map[string]any, s *Stage) { s.OnUnknown = "" }},
		{"no implicit error handler required", "", func(_ map[string]any, s *Stage) { s.OnError = "" }},
		{"duplicate branch identity", "duplicate_branch_id", func(_ map[string]any, s *Stage) { s.Branches = append(s.Branches, s.Branches[0]) }},
		{"branch cycle", "cycle", func(_ map[string]any, s *Stage) {
			s.Branches = append(s.Branches, ChoiceBranch{ID: "cycle", Predicate: conditionFixturePredicate("false"), Next: "check_first"})
		}},
		{"default cycle", "cycle", func(_ map[string]any, s *Stage) { s.Default = "choose" }},
		{"unknown cycle", "cycle", func(_ map[string]any, s *Stage) { s.OnUnknown = "choose" }},
		{"error cycle", "cycle", func(_ map[string]any, s *Stage) { s.OnError = "choose" }},
		{"missing producer", "missing_stage", func(_ map[string]any, s *Stage) { s.Branches[0].Predicate.Ref.StageID = "missing" }},
		{"future producer", "unavailable_output", func(_ map[string]any, s *Stage) { s.Branches[0].Predicate.Ref.StageID = "check_second" }},
		{"control stage is not output producer", "unknown_port", func(_ map[string]any, s *Stage) { s.Branches[0].Predicate.Ref.StageID = "choose" }},
		{"missing producer port", "unknown_port", func(_ map[string]any, s *Stage) { s.Branches[0].Predicate.Ref.Port = "missing" }},
		{"missing workflow input", "unknown_port", func(_ map[string]any, s *Stage) {
			s.Branches[0].Predicate.Ref = &FieldRef{From: "workflow_input", Port: "missing"}
		}},
		{"escaped optional pointer", "", func(_ map[string]any, s *Stage) {
			s.Branches[0].Predicate.Ref = &FieldRef{From: "workflow_input", Port: "first", Pointer: "/a~1b/~0key/0"}
		}},
		{"malformed pointer", "schema_invalid", func(_ map[string]any, s *Stage) { s.Branches[0].Predicate.Ref.Pointer = "/bad~2" }},
		{"iteration source remains unsupported", "unsupported", func(_ map[string]any, s *Stage) {
			s.Branches[0].Predicate.Ref = &FieldRef{From: "iteration_output", Port: "report"}
		}},
		{"blob field", "condition_type_mismatch", func(w map[string]any, s *Stage) {
			w["inputs"].(map[string]any)["blob"] = InputPort{Port: Port{Format: "blob", MediaTypes: []string{"text/plain"}}}
			s.Branches[0].Predicate.Ref = &FieldRef{From: "workflow_input", Port: "blob"}
		}},
		{"producer optional on one predecessor", "", optionalProducer},
		{"producer only on disjoint branch", "unavailable_output", func(w map[string]any, s *Stage) {
			delete(stages(w), "check_first")
			w["definition"].(map[string]any)["entry"] = "gate"
			stages(w)["gate"] = Stage{Kind: "choice", Selection: "first_match", Branches: []ChoiceBranch{{ID: "produce", Predicate: conditionFixturePredicate("true"), Next: "check_second"}}, Default: "choose"}
			s.Branches[0].Next = "done"
			s.Branches[0].Predicate.Ref.StageID = "check_second"
		}},
		{"required export still needs every predecessor", "unavailable_output", func(w map[string]any, s *Stage) {
			optionalProducer(w, s)
			w["outputs"].(map[string]any)["report_second"].(map[string]any)["required_for"] = []string{"succeeded"}
			stages(w)["done"].(map[string]any)["output_bindings"] = map[string]Binding{"report_second": {From: "stage_output", StageID: "check_second", Port: "report"}}
		}},
		{"each same-outcome finish needs exports", "missing_binding", func(w map[string]any, s *Stage) {
			w["outputs"].(map[string]any)["report_first"].(map[string]any)["required_for"] = []string{"succeeded"}
			stages(w)["done"].(map[string]any)["output_bindings"] = map[string]Binding{"report_first": {From: "stage_output", StageID: "check_first", Port: "report"}}
			stages(w)["other_done"] = Stage{Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]Binding{}}
			s.Default = "other_done"
		}},
		{"unselected branch is validated", "unavailable_output", func(w map[string]any, s *Stage) {
			s.Branches[0].Predicate = conditionFixturePredicate("false")
			stages(w)["check_second"].(map[string]any)["input_bindings"] = map[string]Binding{"document": {From: "stage_output", StageID: "check_second", Port: "report"}}
		}},
		{"control transition budget includes choice", "limit_exceeded", func(w map[string]any, _ *Stage) { w["limits"].(map[string]any)["max_control_transitions"] = 3 }},
		{"choice is not step and longest path is not graph size", "", func(w map[string]any, _ *Stage) {
			w["limits"].(map[string]any)["max_step_instances"] = 2
			w["limits"].(map[string]any)["max_control_transitions"] = 4
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, registry := fixture(t)
			for _, output := range w["outputs"].(map[string]any) {
				output.(map[string]any)["required_for"] = []string{}
			}
			for _, stage := range stages(w) {
				if stage.(map[string]any)["kind"] == "finish" {
					stage.(map[string]any)["output_bindings"] = map[string]Binding{}
				}
			}
			stages(w)["check_first"].(map[string]any)["on"].(map[string]any)["pass"] = "choose"
			choice := Stage{Kind: "choice", Selection: "exclusive", Branches: []ChoiceBranch{{ID: "has_report", Predicate: Predicate{Op: "exists", Ref: &FieldRef{From: "stage_output", StageID: "check_first", Port: "report"}}, Next: "check_second"}}, Default: "rejected_second", OnUnknown: "rejected_first", OnError: "rejected_first"}
			tc.edit(w, &choice)
			stages(w)["choose"] = choice
			p, err := CompileProfile(encoded(t, w), "json", registry, CoreProfile)
			if tc.code != "" {
				expectProblem(t, err, tc.code)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, invented := p.Steps["choose"]; invented || len(p.Sequence) != len(stages(w)) {
				t.Fatalf("choice created a fake step or lost declared paths: %+v", p.Sequence)
			}
			if err := ValidateProtocol("WorkflowRevision", encoded(t, p.Workflow)); err != nil {
				t.Fatal("typed choice changed the existing wire shape", err)
			}
			next, err := p.NextError("choose")
			if choice.OnError == "" {
				expectProblem(t, err, "unhandled_error")
			} else if err != nil || next != choice.OnError {
				t.Fatalf("choice lost its explicit error route: %q %v", next, err)
			}
			_, err = Compile(encoded(t, w), "json", registry)
			expectProblem(t, err, "unsupported")
		})
	}
}

func TestCoreProjection(t *testing.T) {
	for _, test := range []struct {
		name, source, projection, pointer, value, want, code string
		optional, absent                                     bool
	}{
		{name: "present null", source: `{"type":"object","properties":{"report":{"type":["string","null"]}},"required":["report"]}`, projection: `{"type":["string","null"]}`, pointer: "/report", value: `{"report":null}`, want: `null`},
		{name: "empty pointer", source: `true`, projection: `{"type":"null"}`, pointer: "", value: `null`, want: `null`},
		{name: "escaped keys", source: `{"type":"object","properties":{"a/b":{"type":"object","required":["~key"],"properties":{"~key":{"type":"integer"}}}},"required":["a/b"]}`, projection: `{"type":"integer"}`, pointer: "/a~1b/~0key", value: `{"a/b":{"~key":7}}`, want: `7`},
		{name: "optional missing", source: `{"type":"object","properties":{"report":{"type":"string"}}}`, projection: `{"type":"string"}`, pointer: "/report", value: `{}`, optional: true, absent: true},
		{name: "required missing", source: `{"type":"object","properties":{"report":{"type":"string"}}}`, projection: `{"type":"string"}`, pointer: "/report", code: "unavailable_output"},
		{name: "array item", source: `{"type":"array","minItems":1,"items":{"type":"object","required":["name"],"properties":{"name":{"type":"string"}}}}`, projection: `{"type":"string"}`, pointer: "/0/name", value: `[{"name":"value"}]`, want: `"value"`},
		{name: "tuple item", source: `{"type":"array","minItems":1,"prefixItems":[{"type":"object","required":["name"],"properties":{"name":{"type":"string"}}}],"items":false}`, projection: `{"type":"string"}`, pointer: "/0/name", value: `[{"name":"tuple"}]`, want: `"tuple"`},
		{name: "array may be empty", source: `{"type":"array","items":{"type":"string"}}`, projection: `{"type":"string"}`, pointer: "/0", code: "unavailable_output"},
		{name: "array leading zero", source: `{"type":"array","minItems":2,"items":{"type":"string"}}`, projection: `{"type":"string"}`, pointer: "/01", code: "unavailable_output"},
		{name: "local ref", source: `{"type":"object","properties":{"report":{"$ref":"#/$defs/report"}},"required":["report"],"$defs":{"report":{"type":"object","required":["value"],"properties":{"value":{"type":"string"}}}}}`, projection: `{"type":"string"}`, pointer: "/report/value", value: `{"report":{"value":"yes"}}`, want: `"yes"`},
		{name: "composed presence not proved", source: `{"allOf":[{"type":"object","required":["report"],"properties":{"report":{"type":"string"}}}]}`, projection: `{"type":"string"}`, pointer: "/report", code: "unsupported_projection"},
		{name: "nested resource changes fragment base", source: `{"type":"object","properties":{"report":{"$id":"urn:test:nested","type":"object","required":["value"],"properties":{"value":{"type":"string"}}}},"required":["report"]}`, projection: `{"type":"string"}`, pointer: "/report/value", code: "unsupported_projection"},
	} {
		t.Run(test.name, func(t *testing.T) {
			workflow, registry := fixture(t)
			addSchema := func(id, schema string) Ref {
				digest, err := Digest([]byte(schema))
				if err != nil {
					t.Fatal(err)
				}
				ref := Ref{ID: "test:schema/" + id, Version: "1.0.0", Digest: digest}
				registry[ref] = []byte(schema)
				return ref
			}
			source, projected := addSchema("source", test.source), addSchema("projected", test.projection)
			requiredFor := []string{"succeeded"}
			if test.optional {
				requiredFor = []string{}
			}
			workflow["inputs"] = map[string]any{"source": InputPort{Port: Port{Format: "json", SchemaRef: &source}, Required: true}}
			workflow["outputs"] = map[string]any{"projected": OutputPort{Port: Port{Format: "json", SchemaRef: &projected}, RequiredFor: requiredFor}}
			binding := Binding{From: "workflow_input", Port: "source", Pointer: &test.pointer, ProjectedSchemaRef: &projected}
			workflow["definition"] = map[string]any{"entry": "done", "stages": map[string]Stage{"done": {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]Binding{"projected": binding}}}}
			plan, err := CompileProfile(encoded(t, workflow), "json", registry, CoreProfile)
			if test.code != "" {
				expectProblem(t, err, test.code)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			data, present, err := plan.ProjectJSON(binding, []byte(test.value))
			if err != nil || present == test.absent || string(data) != test.want {
				t.Fatalf("projection %q: data=%s present=%v error=%v", test.pointer, data, present, err)
			}
			_, err = Compile(encoded(t, workflow), "json", registry)
			expectProblem(t, err, "unsupported")
			if test.name == "present null" {
				_, _, err := plan.ProjectJSON(binding, []byte(`{"report":5}`))
				expectProblem(t, err, "projection_schema_invalid")
				invalid := "/invalid~2escape"
				badBinding := binding
				badBinding.Pointer = &invalid
				_, _, err = plan.ProjectJSON(badBinding, []byte(test.value))
				expectProblem(t, err, "invalid_pointer")
				workflow["outputs"] = map[string]any{"projected": OutputPort{Port: Port{Format: "json", SchemaRef: &source}, RequiredFor: requiredFor}}
				_, err = CompileProfile(encoded(t, workflow), "json", registry, CoreProfile)
				expectProblem(t, err, "port_type_mismatch")
				workflow["outputs"] = map[string]any{"projected": OutputPort{Port: Port{Format: "json", SchemaRef: &projected}, RequiredFor: requiredFor}}
				workflow["inputs"] = map[string]any{"source": InputPort{Port: Port{Format: "blob", MediaTypes: []string{"text/plain"}}, Required: true}}
				_, err = CompileProfile(encoded(t, workflow), "json", registry, CoreProfile)
				expectProblem(t, err, "port_type_mismatch")
			}
		})
	}
}

func TestWorkflowConfigurationDeclarations(t *testing.T) {
	for _, test := range []struct {
		name, schema, defaultValue, scope, version, profile, code string
	}{
		{"null default", `{"type":["object","null"]}`, "null", "run", "2", CoreProfile, ""},
		{"absent default", `{"type":"object"}`, "", "project", "2", CoreProfile, ""},
		{"default reference is data", `{"type":"object"}`, `{"id":"test:data/missing","version":"1.0.0","digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`, "project", "2", CoreProfile, ""},
		{"invalid default", `{"type":"object"}`, "null", "run", "2", CoreProfile, "invalid_default"},
		{"unknown scope", `{"type":"object"}`, "", "environment", "2", CoreProfile, "schema_invalid"},
		{"v1 remains closed", `{"type":"object"}`, "", "run", "1", CoreProfile, "schema_invalid"},
		{"F1 rejects v2 at its original schema boundary", `{"type":"object"}`, "", "run", "2", Profile, "schema_invalid"},
	} {
		t.Run(test.name, func(t *testing.T) {
			workflow, registry := fixture(t)
			digest, _ := Digest([]byte(test.schema))
			ref := Ref{ID: "test:schema/configuration", Version: "1.0.0", Digest: digest}
			registry[ref] = []byte(test.schema)
			workflow["schema_version"] = test.version
			workflow["inputs"] = map[string]InputPort{"configuration": {Port: Port{Format: "json", SchemaRef: &ref}, Required: false, Configuration: &InputConfiguration{Scope: test.scope, Default: json.RawMessage(test.defaultValue)}}}
			workflow["outputs"] = map[string]any{}
			workflow["definition"] = map[string]any{"entry": "done", "stages": map[string]Stage{"done": {Kind: "finish", Outcome: "no_work", OutputBindings: map[string]Binding{}}}}
			workflow["allowed_outcomes"] = []string{"no_work"}
			plan, err := CompileProfile(encoded(t, workflow), "json", registry, test.profile)
			if test.code != "" {
				expectProblem(t, err, test.code)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			configuration := plan.Workflow.Inputs["configuration"].Configuration
			want := []byte(test.defaultValue)
			if len(want) != 0 {
				want, err = Canonical(want)
				if err != nil {
					t.Fatal(err)
				}
			}
			if configuration == nil || !bytes.Equal(configuration.Default, want) {
				t.Fatalf("default absence/null changed: %+v", configuration)
			}
			if err := ValidateProtocol("WorkflowRevisionV2", encoded(t, plan.Workflow)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestEmptyLiteralAndSharedFinish(t *testing.T) {
	workflow, registry := fixture(t)
	workflow["definition"] = map[string]any{"entry": "done", "stages": map[string]any{"done": map[string]any{"kind": "finish", "outcome": "no_work", "output_bindings": map[string]any{}}}}
	workflow["allowed_outcomes"] = []string{"no_work"}
	workflow["outputs"] = map[string]any{}
	plan, err := Compile(encoded(t, workflow), "json", registry)
	if err != nil || len(plan.Steps) != 0 || !slices.Equal(plan.Sequence, []string{"done"}) {
		t.Fatalf("empty plan: %v, %v", plan, err)
	}
	if err := ValidateProtocol("WorkflowRevision", encoded(t, plan.Workflow)); err != nil {
		t.Fatalf("required empty finish bindings were lost: %v", err)
	}
	workflow, registry = fixture(t)
	first := stages(workflow)["check_first"].(map[string]any)
	first["input_bindings"] = map[string]any{"document": map[string]any{"from": "literal", "value": map[string]any{"key": "one", "text": "typed"}, "schema_ref": workflow["inputs"].(map[string]any)["first"].(map[string]any)["schema_ref"]}}
	if _, err := Compile(encoded(t, workflow), "json", registry); err != nil {
		t.Fatal(err)
	}
	first["input_bindings"].(map[string]any)["document"].(map[string]any)["value"] = nil
	_, err = Compile(encoded(t, workflow), "json", registry)
	expectProblem(t, err, "invalid_literal")
	workflow, registry = fixture(t)
	// Sharing the same rejected finish between negative paths is valid when
	// every required export is present on both paths.
	stages(workflow)["check_second"].(map[string]any)["on"].(map[string]any)["fail"] = "rejected_first"
	delete(stages(workflow), "rejected_second")
	if _, err := Compile(encoded(t, workflow), "json", registry); err != nil {
		t.Fatal(err)
	}
}

func TestExactDependencyAndSchemaClosure(t *testing.T) {
	for _, test := range []struct{ name, code string }{{"missing", "missing_ref"}, {"changed", "digest_mismatch"}, {"identity", "ref_identity_mismatch"}} {
		t.Run(test.name, func(t *testing.T) {
			workflow, registry := fixture(t)
			var ref Ref
			if err := decodeValue(stages(workflow)["check_first"].(map[string]any)["step_ref"], &ref); err != nil {
				t.Fatal(err)
			}
			switch test.name {
			case "missing":
				delete(registry, ref)
			case "changed":
				registry[ref] = []byte(`{"changed":true}`)
			case "identity":
				var step map[string]any
				json.Unmarshal(registry[ref], &step)
				step["id"] = "wrong:identity"
				data := encoded(t, step)
				ref.Digest, _ = Digest(data)
				registry[ref] = data
				stages(workflow)["check_first"].(map[string]any)["step_ref"] = ref
			}
			_, err := Compile(encoded(t, workflow), "json", registry)
			expectProblem(t, err, test.code)
		})
	}
	// No fetcher exists: a schema reference to a host file or URL fails closed.
	for _, external := range []string{"file:///etc/passwd", "https://example.invalid/schema.json"} {
		workflow, registry := fixture(t)
		data := encoded(t, map[string]any{"$ref": external})
		digest, _ := Digest(data)
		ref := Ref{ID: "example:schema/external", Version: "1.0.0", Digest: digest}
		registry[ref] = data
		workflow["inputs"].(map[string]any)["first"].(map[string]any)["schema_ref"] = ref
		_, err := Compile(encoded(t, workflow), "json", registry)
		expectProblem(t, err, "invalid_schema_ref")
	}
}

func TestDefinitionNamesAndJSONInstanceReferences(t *testing.T) {
	dataRef := Ref{ID: "test:data/not-a-dependency", Version: "1.0.0", Digest: "sha256:" + strings.Repeat("a", 64)}
	schema := encoded(t, map[string]any{
		"type": "object", "properties": map[string]any{"value": map[string]any{"const": dataRef}, "default": map[string]any{"type": "string"}},
		"required": []string{"value"}, "default": map[string]any{"value": dataRef}, "examples": []any{map[string]any{"value": dataRef}},
	})
	digest, _ := Digest(schema)
	ref := Ref{ID: "test:schema/instance-values", Version: "1.0.0", Digest: digest}
	for _, profile := range []string{Profile, CoreProfile} {
		for _, name := range []string{"value", "default", "const", "enum", "examples", "schema_ref"} {
			t.Run(profile+"/"+name, func(t *testing.T) {
				workflow, registry := fixture(t)
				registry[ref] = schema
				workflow["inputs"] = map[string]InputPort{name: {Port: Port{Format: "json", SchemaRef: &ref}, Required: true}}
				workflow["outputs"] = map[string]OutputPort{name: {Port: Port{Format: "json", SchemaRef: &ref}, RequiredFor: []string{"succeeded"}}}
				workflow["definition"] = map[string]any{"entry": name, "stages": map[string]Stage{name: {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]Binding{name: {From: "workflow_input", Port: name}}}}}
				plan, err := CompileProfile(encoded(t, workflow), "json", registry, profile)
				if err != nil {
					t.Fatal(err)
				}
				instance := encoded(t, map[string]any{"value": dataRef})
				if err := plan.ValidateJSON(ref, instance); err != nil {
					t.Fatal("schema property named value was skipped", err)
				}
				if _, found := plan.Registry[dataRef]; found {
					t.Fatal("JSON Schema instance data became a dependency")
				}
				workflow["definition"] = map[string]any{"entry": name, "stages": map[string]Stage{name: {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]Binding{name: {From: "literal", SchemaRef: &ref, Value: instance}}}}}
				if _, err := CompileProfile(encoded(t, workflow), "json", registry, profile); err != nil {
					t.Fatal("literal instance data became a dependency", err)
				}
			})
		}
	}
	if err := ValidateSchema(Registry{ref: schema}, ref, encoded(t, map[string]any{"value": dataRef})); err != nil {
		t.Fatal("standalone schema interpreted instance data as refs", err)
	}
}

func TestStrictJSON(t *testing.T) {
	for _, test := range []struct{ name, data, code string }{
		{"duplicate", `{"a":1,"a":2}`, "duplicate_key"},
		{"escaped duplicate", `{"a":1,"\u0061":2}`, "duplicate_key"},
		{"nested duplicate", `{"a":[{"b":1,"b":2}]}`, "duplicate_key"},
		{"trailing", `{} {}`, "invalid_json"},
		{"empty", ``, "invalid_json"},
		{"unpaired high", `"\ud800"`, "invalid_unicode"},
		{"unpaired low", `"\udfff"`, "invalid_unicode"},
		{"wrong pair", `"\ud800\u0061"`, "invalid_unicode"},
		{"unsafe integer", `9007199254740992`, "unsafe_integer"},
		{"negative unsafe", `-9007199254740992`, "unsafe_integer"},
		{"overflow", `1e400`, "invalid_number"},
		{"NaN", `NaN`, "invalid_json"},
		{"invalid UTF8", string([]byte{'"', 0xff, '"'}), "invalid_unicode"},
		{"depth", strings.Repeat("[", MaxDepth+2) + "0" + strings.Repeat("]", MaxDepth+2), "document_limit"},
		{"nodes", "[" + strings.Repeat("0,", MaxNodes) + "0]", "document_limit"},
		{"size", strings.Repeat(" ", MaxDocumentBytes+1), "document_too_large"},
	} {
		t.Run(test.name, func(t *testing.T) { _, err := Parse([]byte(test.data), "json"); expectProblem(t, err, test.code) })
	}
	for _, data := range []string{`null`, `"\ud83d\ude00"`, `9007199254740991`, `-9007199254740991`, `{"n":1.5,"f":false,"t":true,"literal":"\\ud800"}`} {
		if _, err := Parse([]byte(data), "json"); err != nil {
			t.Fatalf("valid JSON %s: %v", data, err)
		}
	}
}

func TestExplicitConverterRequiredForDifferentSchemaRevision(t *testing.T) {
	original, registry := fixture(t)
	base, err := Compile(encoded(t, original), "json", registry)
	if err != nil {
		t.Fatal(err)
	}
	a := *base.Workflow.Inputs["first"].SchemaRef
	var secondSchema map[string]any
	if err := json.Unmarshal(registry[a], &secondSchema); err != nil {
		t.Fatal(err)
	}
	secondSchema["properties"].(map[string]any)["text"] = map[string]any{"type": "integer"}
	schemaBytes := encoded(t, secondSchema)
	hash, err := Digest(schemaBytes)
	if err != nil {
		t.Fatal(err)
	}
	b := Ref{ID: "test:schema/document", Version: "2.0.0", Digest: hash}
	registry[b] = schemaBytes
	converter := base.Steps["check_first"]
	converter.ID, converter.Kind = "test:step/converter", "command"
	converter.Inputs = map[string]InputPort{"source": {Port: Port{Format: "json", SchemaRef: &a}, Required: true}}
	converter.Outputs = map[string]OutputPort{"converted": {Port: Port{Format: "json", SchemaRef: &b}, RequiredFor: []string{"pass"}}}
	consumer := base.Steps["check_first"]
	consumer.ID = "test:step/consumer-v2"
	consumer.Inputs = map[string]InputPort{"document": {Port: Port{Format: "json", SchemaRef: &b}, Required: true}}
	consumer.Outputs = map[string]OutputPort{}
	register := func(step StepDefinition) Ref {
		data := encoded(t, step)
		digest, err := Digest(data)
		if err != nil {
			t.Fatal(err)
		}
		ref := Ref{ID: step.ID, Version: step.Version, Digest: digest}
		registry[ref] = data
		return ref
	}
	converterRef, consumerRef := register(converter), register(consumer)
	w := base.Workflow
	w.Inputs = map[string]InputPort{"source": converter.Inputs["source"]}
	w.Outputs, w.AllowedOutcomes = map[string]OutputPort{}, []string{"succeeded"}
	w.Limits = Limits{MaxStepInstances: 2, MaxControlTransitions: 3, MaxParallelism: 1}
	w.Definition.Entry = "convert"
	w.Definition.Stages = map[string]Stage{
		"convert": {Kind: "step", StepRef: converterRef, InputBindings: map[string]Binding{"source": {From: "workflow_input", Port: "source"}}, On: map[string]string{"pass": "consume"}},
		"consume": {Kind: "step", StepRef: consumerRef, InputBindings: map[string]Binding{"document": {From: "stage_output", StageID: "convert", Port: "converted"}}, On: map[string]string{"pass": "done"}},
		"done":    {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]Binding{}},
	}
	plan, err := Compile(encoded(t, w), "json", registry)
	if err != nil || !slices.Equal(plan.Sequence, []string{"convert", "consume", "done"}) {
		t.Fatalf("explicit compatible converter refused: %v", err)
	}
	stage := w.Definition.Stages["consume"]
	stage.InputBindings = map[string]Binding{"document": {From: "workflow_input", Port: "source"}}
	w.Definition.Stages["consume"] = stage
	_, err = Compile(encoded(t, w), "json", registry)
	expectProblem(t, err, "port_type_mismatch")
	stage.InputBindings = map[string]Binding{"document": {From: "stage_output", StageID: "convert", Port: "converted"}}
	w.Definition.Stages["consume"] = stage
	converter.Outputs["converted"] = OutputPort{Port: Port{Format: "json", SchemaRef: &a}, RequiredFor: []string{"pass"}}
	stage = w.Definition.Stages["convert"]
	stage.StepRef = register(converter)
	w.Definition.Stages["convert"] = stage
	_, err = Compile(encoded(t, w), "json", registry)
	expectProblem(t, err, "port_type_mismatch")
}

func TestRestrictedYAMLAndEquivalence(t *testing.T) {
	workflow, registry := fixture(t)
	yamlBytes, err := yaml.Marshal(workflow)
	if err != nil {
		t.Fatal(err)
	}
	fromJSON, err := Compile(encoded(t, workflow), "json", registry)
	if err != nil {
		t.Fatal(err)
	}
	fromYAML, err := Compile(yamlBytes, "yaml", registry)
	if err != nil {
		t.Fatal(err)
	}
	if fromJSON.Digest != fromYAML.Digest || !bytes.Equal(fromJSON.Canonical, fromYAML.Canonical) {
		t.Fatal("YAML and JSON compiled to different definitions")
	}
	for _, data := range []string{
		"a: 1\na: 2\n", "a: &anchor 1\nb: *anchor\n", "a: !!str x\n", "a: !custom x\n", "a: {<<: {x: 1}}\n",
		"a: yes\n", "a: True\n", "a: ~\n", "a:\n", "a: 0x10\n", "a: 01\n", "a: 1_000\n", "a: +1\n", "a: .inf\n",
		"a: 2026-08-28\n", "? [a, b]\n: 1\n", "a: 1\n---\nb: 2\n", "a: 9007199254740992\n",
	} {
		t.Run(data, func(t *testing.T) {
			if _, err := Parse([]byte(data), "yaml"); err == nil {
				t.Fatal("unsafe/ambiguous YAML accepted")
			}
		})
	}
	value, err := Parse([]byte("a: \"yes\"\nb: false\nc: null\nd: 1.5\nenv: '${SECRET}'\n"), "yaml")
	if err != nil || value.(map[string]any)["env"] != "${SECRET}" {
		t.Fatalf("literal environment text altered: %v, %v", value, err)
	}
}

func TestCanonicalAndEmbeddedSchemas(t *testing.T) {
	// RFC 8785 UTF-16 property sorting differs from UTF-8 sorting for these keys.
	a, err := Canonical([]byte("{\"\ufb33\":2,\"😀\":1,\"a\":1.0,\"z\":-0}"))
	if err != nil {
		t.Fatal(err)
	}
	expected := []byte("{\"a\":1,\"z\":0,\"😀\":1,\"\ufb33\":2}")
	if !bytes.Equal(a, expected) {
		t.Fatalf("JCS result %s != %s", a, expected)
	}
	d1, _ := Digest([]byte(`{"a":1,"b":2}`))
	d2, _ := Digest([]byte("{ \"b\":2, \"a\":1.0 }"))
	if d1 != d2 {
		t.Fatal("equivalent definitions hash differently")
	}
	for _, pair := range []struct {
		path     string
		embedded []byte
	}{
		{"protocol.schema.json", protocolSchema},
		{"../../schemas/foundation/step-definition-v2.schema.json", stepDefinitionV2Schema},
	} {
		data, err := os.ReadFile(pair.path)
		if err != nil || !bytes.Equal(data, pair.embedded) {
			t.Fatalf("embedded contract drift: %s, %v", pair.path, err)
		}
	}
	stepV3, err := ProtocolSchema("StepDefinitionV3")
	if err != nil {
		t.Fatal(err)
	}
	distributedV3, err := os.ReadFile("../../schemas/core/step-definition-v3.schema.json")
	if err != nil || !bytes.Equal(append(stepV3, '\n'), distributedV3) {
		t.Fatalf("generated StepDefinition v3 drift: %v", err)
	}
	stepV4, err := ProtocolSchema("StepDefinitionV4")
	if err != nil {
		t.Fatal(err)
	}
	distributedV4, err := os.ReadFile("../../schemas/core/step-definition-v4.schema.json")
	if err != nil || !bytes.Equal(append(stepV4, '\n'), distributedV4) {
		t.Fatalf("generated StepDefinition v4 drift: %v", err)
	}
	expectProblem(t, ValidateProtocol("../unexpected", []byte(`{}`)), "unsupported_contract")
	_, registry := fixture(t)
	for ref := range registry {
		if ref.ID == "prifly:core/schema/StepResult" {
			data, err := ProtocolSchema("StepResult")
			if err != nil {
				t.Fatal(err)
			}
			digest, err := Digest(data)
			if err != nil || digest != ref.Digest {
				t.Fatalf("historical StepResult schema changed: %s, %v", digest, err)
			}
			if err := ValidateSchema(Registry{ref: data}, ref, []byte(`{}`)); err == nil {
				t.Fatal("invalid result accepted by standalone schema validator")
			}
		}
	}
}

func TestCanonicalScalarDocumentsAndBooleanSchemas(t *testing.T) {
	for _, test := range []struct{ source, expected string }{
		{`"command:one"`, `"command:one"`},
		{`"\u0061"`, `"a"`},
		{`1.0`, `1`},
		{`-0`, `0`},
		{`1e-7`, `1e-7`},
		{" \ntrue\t", `true`},
		{`false`, `false`},
		{`null`, `null`},
	} {
		t.Run(test.source, func(t *testing.T) {
			actual, err := Canonical([]byte(test.source))
			if err != nil || string(actual) != test.expected {
				t.Fatalf("canonical scalar %s: got %s, %v", test.source, actual, err)
			}
			first, err := Digest([]byte(test.source))
			if err != nil {
				t.Fatal(err)
			}
			second, err := Digest([]byte(test.expected))
			if err != nil || first != second {
				t.Fatalf("canonical scalar digests differ: %s %s %v", first, second, err)
			}
		})
	}
	for _, source := range []string{`true`, `false`} {
		digest, err := Digest([]byte(source))
		if err != nil {
			t.Fatal(err)
		}
		ref := Ref{ID: "test:schema/boolean", Version: "1.0.0", Digest: digest}
		registry := Registry{ref: []byte(source)}
		err = ValidateSchema(registry, ref, []byte(`"scalar instance"`))
		if source == "true" && err != nil {
			t.Fatalf("true schema must accept a scalar instance: %v", err)
		}
		if source == "false" && err == nil {
			t.Fatal("false schema accepted a scalar instance")
		}
	}
}

func hookFixture(t *testing.T) (map[string]any, Registry, map[string]any, func()) {
	t.Helper()
	workflow, registry := fixture(t)
	var ref Ref
	if err := decodeValue(stages(workflow)["check_first"].(map[string]any)["step_ref"], &ref); err != nil {
		t.Fatal(err)
	}
	var step map[string]any
	if err := json.Unmarshal(registry[ref], &step); err != nil {
		t.Fatal(err)
	}
	schema := []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"completed":{"type":"integer","minimum":0},"phase":{"type":"string","enum":["starting","working","finished"]}},"required":["phase"],"additionalProperties":false}`)
	digest, err := Digest(schema)
	if err != nil {
		t.Fatal(err)
	}
	schemaRef := Ref{ID: "example:schema/progress", Version: "1.0.0", Digest: digest}
	registry[schemaRef] = schema
	step["schema_version"] = "2"
	step["version"] = "2.0.0"
	step["hooks"] = map[string]any{
		"progress_changed": map[string]any{"kind": "state", "schema_ref": schemaRef, "description": "Completed work and current phase", "classification": "internal", "read_policy": "owner", "max_payload_bytes": 256, "max_count": 100, "max_per_minute": 60, "allow_during_stop": true, "freshness_ms": 5000},
		"warning_raised":   map[string]any{"kind": "event", "schema_ref": schemaRef, "description": "Structured warning occurrence", "classification": "internal", "read_policy": "owner", "max_payload_bytes": 256, "max_count": 100, "max_per_minute": 60, "allow_during_stop": false},
	}
	step["telemetry"] = []any{
		map[string]any{"name": "processed_total", "revision": "1.0.0", "description": "Cumulative accepted checkpoints", "hook": "progress_changed", "kind": "counter", "field": "/completed", "unit": "1", "aggregation": "delta", "reset": "attempt", "minimum": 0, "maximum": 1000, "dimensions": map[string]any{"phase": "/phase"}},
		map[string]any{"name": "warnings", "revision": "1.0.0", "description": "Declared warning occurrences", "hook": "warning_raised", "kind": "diagnostic", "aggregation": "occurrences", "reset": "none", "severity": "warn", "code": "quality_warning", "message": "A quality warning was reported", "dimensions": map[string]any{"phase": "/phase"}},
	}
	update := func() {
		data := encoded(t, step)
		ref.Version = "2.0.0"
		ref.Digest, err = Digest(data)
		if err != nil {
			t.Fatal(err)
		}
		registry[ref] = data
		for _, id := range []string{"check_first", "check_second"} {
			stages(workflow)[id].(map[string]any)["step_ref"] = ref
		}
	}
	update()
	return workflow, registry, step, update
}

func TestDeclaredHooksAndMapping(t *testing.T) {
	workflow, registry, step, _ := hookFixture(t)
	if err := ValidateProtocol("StepDefinition", encoded(t, step)); err == nil {
		t.Fatal("baseline v1 must not silently gain hooks")
	}
	if err := ValidateProtocol("StepDefinitionV2", encoded(t, step)); err != nil {
		t.Fatal(err)
	}
	plan, err := Compile(encoded(t, workflow), "json", registry)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps["check_first"].Hooks) != 2 || len(plan.Steps["check_first"].Telemetry) != 2 {
		t.Fatal("hook declarations were lost")
	}
	for _, test := range []struct{ hook, kind, payload, code string }{
		{"progress_changed", "state", `{"phase":"working","completed":10}`, ""},
		{"progress_changed", "state", `{"phase":"working"}`, ""},
		{"warning_raised", "event", `{"phase":"working"}`, ""},
		{"undeclared", "state", `{"phase":"working"}`, "unknown_hook"},
		{"progress_changed", "event", `{"phase":"working"}`, "hook_kind_mismatch"},
		{"progress_changed", "state", `{"phase":"working","completed":"10"}`, "schema_invalid"},
		{"progress_changed", "state", `{"phase":"working","completed":1001}`, "measurement_out_of_bounds"},
		{"progress_changed", "state", `{"phase":"working","trust":"os"}`, "schema_invalid"},
		{"progress_changed", "state", `{"phase":"working","completed":-1}`, "schema_invalid"},
		{"progress_changed", "state", strings.Repeat(" ", 257), "payload_too_large"},
	} {
		t.Run(test.hook+"/"+test.payload, func(t *testing.T) {
			_, err := plan.ValidatePublication("check_first", test.hook, test.kind, []byte(test.payload))
			if test.code != "" {
				expectProblem(t, err, test.code)
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
	if next, err := plan.Next("check_first", "pass"); err != nil || next != "check_second" {
		t.Fatal("publications must not change the sequential route")
	}
}

func TestArtifactHookDefinitionV3IsCoreOnlyAndClosed(t *testing.T) {
	workflow, registry, step, update := hookFixture(t)
	hooks := step["hooks"].(map[string]any)
	schemaRef := hooks["progress_changed"].(map[string]any)["schema_ref"].(Ref)
	hooks["document_created"] = map[string]any{
		"kind": "artifact", "schema_ref": schemaRef, "description": "One sealed document",
		"classification": "internal", "read_policy": "owner", "max_payload_bytes": 4096,
		"max_count": 1, "max_per_minute": 10, "allow_during_stop": false,
		"artifact": map[string]any{"format": "json", "cardinality": "one", "content_check_refs": []any{}, "early_consumption": true},
	}
	step["schema_version"] = "3"
	update()
	if err := ValidateProtocol("StepDefinitionV2", encoded(t, step)); err == nil {
		t.Fatal("StepDefinition v2 silently accepted an artifact hook")
	}
	if err := ValidateProtocol("StepDefinitionV3", encoded(t, step)); err != nil {
		t.Fatal(err)
	}
	if _, err := Compile(encoded(t, workflow), "json", registry); err == nil {
		t.Fatal("foundation profile accepted StepDefinition v3")
	} else {
		expectProblem(t, err, "unsupported")
	}
	plan, err := CompileProfile(encoded(t, workflow), "json", registry, CoreProfile)
	if err != nil {
		t.Fatal(err)
	}
	hook := plan.Steps["check_first"].Hooks["document_created"]
	if hook.Artifact == nil || hook.Artifact.Cardinality != "one" || !hook.Artifact.EarlyConsumption {
		t.Fatal("artifact hook contract was lost")
	}

	step["schema_version"] = "4"
	artifact := hooks["document_created"].(map[string]any)
	artifact["read_policy"] = "declared_subscribers"
	update()
	if err := ValidateProtocol("StepDefinitionV3", encoded(t, step)); err == nil {
		t.Fatal("StepDefinition v3 silently gained subscriber access")
	}
	if err := ValidateProtocol("StepDefinitionV4", encoded(t, step)); err != nil {
		t.Fatal(err)
	}
	hooks["progress_changed"].(map[string]any)["read_policy"] = "declared_subscribers"
	update()
	if err := ValidateProtocol("StepDefinitionV4", encoded(t, step)); err == nil {
		t.Fatal("StepDefinition v4 widened state-hook readers")
	}
	hooks["progress_changed"].(map[string]any)["read_policy"] = "owner"
	update()

	artifact["artifact"].(map[string]any)["content_check_refs"] = []any{schemaRef}
	update()
	_, err = CompileProfile(encoded(t, workflow), "json", registry, CoreProfile)
	expectProblem(t, err, "unsupported")
	artifact["artifact"].(map[string]any)["content_check_refs"] = []any{}
	artifact["max_count"] = 2
	update()
	_, err = CompileProfile(encoded(t, workflow), "json", registry, CoreProfile)
	expectProblem(t, err, "invalid_artifact_hook")
}

func TestInvalidHookDeclarations(t *testing.T) {
	for _, test := range []struct {
		name, code string
		edit       func(map[string]any)
	}{
		{"unknown hook field", "schema_invalid", func(s map[string]any) {
			s["hooks"].(map[string]any)["progress_changed"].(map[string]any)["callback"] = "run shell"
		}},
		{"missing state freshness", "schema_invalid", func(s map[string]any) {
			delete(s["hooks"].(map[string]any)["progress_changed"].(map[string]any), "freshness_ms")
		}},
		{"event freshness", "schema_invalid", func(s map[string]any) {
			s["hooks"].(map[string]any)["warning_raised"].(map[string]any)["freshness_ms"] = 1000
		}},
		{"zero publication limit", "schema_invalid", func(s map[string]any) {
			s["hooks"].(map[string]any)["progress_changed"].(map[string]any)["max_count"] = 0
		}},
		{"unqualified readers", "schema_invalid", func(s map[string]any) {
			s["hooks"].(map[string]any)["progress_changed"].(map[string]any)["read_policy"] = "everyone"
		}},
		{"undeclared mapping hook", "unknown_hook", func(s map[string]any) { s["telemetry"].([]any)[0].(map[string]any)["hook"] = "missing" }},
		{"wrong counter reset", "invalid_mapping", func(s map[string]any) { s["telemetry"].([]any)[0].(map[string]any)["reset"] = "none" }},
		{"wrong counter aggregation", "invalid_mapping", func(s map[string]any) { s["telemetry"].([]any)[0].(map[string]any)["aggregation"] = "last" }},
		{"unknown aggregation", "schema_invalid", func(s map[string]any) { s["telemetry"].([]any)[0].(map[string]any)["aggregation"] = "sum_every_poll" }},
		{"unknown unit syntax", "schema_invalid", func(s map[string]any) { s["telemetry"].([]any)[0].(map[string]any)["unit"] = "byte per $$$" }},
		{"nonnumeric field", "invalid_mapping", func(s map[string]any) { s["telemetry"].([]any)[0].(map[string]any)["field"] = "/phase" }},
		{"missing field", "invalid_mapping", func(s map[string]any) { s["telemetry"].([]any)[0].(map[string]any)["field"] = "/missing" }},
		{"unbounded dimension", "invalid_mapping", func(s map[string]any) {
			s["telemetry"].([]any)[0].(map[string]any)["dimensions"] = map[string]any{"completed": "/completed"}
		}},
		{"core namespace", "reserved_namespace", func(s map[string]any) { s["telemetry"].([]any)[0].(map[string]any)["name"] = "core_failure_count" }},
		{"duplicate descriptor", "ambiguous_mapping", func(s map[string]any) { s["telemetry"] = append(s["telemetry"].([]any), s["telemetry"].([]any)[0]) }},
		{"state diagnostic", "invalid_mapping", func(s map[string]any) { s["telemetry"].([]any)[1].(map[string]any)["hook"] = "progress_changed" }},
		{"negative counter bound", "invalid_mapping", func(s map[string]any) { s["telemetry"].([]any)[0].(map[string]any)["minimum"] = -1 }},
		{"inverted bounds", "invalid_mapping", func(s map[string]any) { s["telemetry"].([]any)[0].(map[string]any)["maximum"] = -1 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			workflow, registry, step, update := hookFixture(t)
			test.edit(step)
			update()
			_, err := Compile(encoded(t, workflow), "json", registry)
			expectProblem(t, err, test.code)
		})
	}
}

func TestJSONPointerAbsentAndNull(t *testing.T) {
	value := map[string]any{"a/b": map[string]any{"~key": []any{nil, "yes"}}}
	for _, test := range []struct {
		pointer string
		value   any
		exists  bool
	}{
		{"/a~1b/~0key/0", nil, true}, {"/a~1b/~0key/1", "yes", true}, {"/a~1b/~0key/2", nil, false},
		{"/a~1b/~0key/01", nil, false}, {"/missing", nil, false}, {"/a~2b", nil, false},
	} {
		actual, exists := JSONPointer(value, test.pointer)
		if exists != test.exists || !reflect.DeepEqual(actual, test.value) {
			t.Fatalf("%s = %v,%v", test.pointer, actual, exists)
		}
	}
}

func TestArtifactRevisionVersionReference(t *testing.T) {
	ref := Ref{ID: "demo:schema/value", Version: "1.0.0", Digest: "sha256:" + strings.Repeat("a", 64)}
	artifact := map[string]any{
		"schema_version": "1", "id": "artifact:one", "revision": 1, "digest": "sha256:" + strings.Repeat("b", 64),
		"producer": map[string]any{"kind": "import", "import_id": "import:one", "source_ref": ref, "principal_id": "local:owner"},
		"format":   "json", "schema_ref": ref, "media_type": "application/json", "size_bytes": 2, "classification": "internal",
		"content_check_evidence": []any{}, "provenance": []any{}, "created_at": "2026-08-28T00:00:00Z",
	}
	if err := ValidateProtocol("ArtifactRevision", encoded(t, artifact)); err != nil {
		t.Fatal(err)
	}
	ref.Version = `1\x0\x0`
	artifact["schema_ref"] = ref
	expectProblem(t, ValidateProtocol("ArtifactRevision", encoded(t, artifact)), "schema_invalid")
}
