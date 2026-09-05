package flow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestConciseWorkflowYAMLCompilesToTheSameRevision(t *testing.T) {
	machine, registry := fixture(t)
	inputs := machine["inputs"].(map[string]any)
	outputs := machine["outputs"].(map[string]any)
	stageValues := stages(machine)
	refs := map[string]any{
		"document": inputs["first"].(map[string]any)["schema_ref"],
		"report":   outputs["report_first"].(map[string]any)["schema_ref"],
		"validate": stageValues["check_first"].(map[string]any)["step_ref"],
		"policy":   machine["policy_ref"],
	}
	source := map[string]any{
		"authoring": WorkflowAuthoringVersion,
		"id":        machine["id"], "version": machine["version"], "title": machine["title"],
		"refs":   refs,
		"inputs": map[string]any{"first": "document", "second": "document"},
		"outputs": map[string]any{
			"report_first":  map[string]any{"schema_ref": "report", "required_for": []any{"succeeded", "rejected"}},
			"report_second": map[string]any{"schema_ref": "report", "required_for": []any{"succeeded"}},
		},
		"allowed_outcomes": machine["allowed_outcomes"],
		"entry":            "check_first",
		"limits": map[string]any{
			"max_step_instances":      machine["limits"].(map[string]any)["max_step_instances"],
			"max_control_transitions": machine["limits"].(map[string]any)["max_control_transitions"],
		},
		"policy_ref": "policy",
		"stages": map[string]any{
			"check_first":  map[string]any{"kind": "step", "step_ref": "validate", "input_bindings": map[string]any{"document": "$inputs.first"}, "on": map[string]any{"pass": "check_second", "fail": "rejected_first"}},
			"check_second": map[string]any{"kind": "step", "step_ref": "validate", "input_bindings": map[string]any{"document": "$inputs.second"}, "on": map[string]any{"pass": "done", "fail": "rejected_second"}},
			"done": map[string]any{"kind": "finish", "outcome": "succeeded", "output_bindings": map[string]any{
				"report_first": "$stages.check_first.report", "report_second": "$stages.check_second.report"}},
			"rejected_first": map[string]any{"kind": "finish", "outcome": "rejected", "output_bindings": map[string]any{"report_first": "$stages.check_first.report"}},
			"rejected_second": map[string]any{"kind": "finish", "outcome": "rejected", "output_bindings": map[string]any{
				"report_first": "$stages.check_first.report", "report_second": "$stages.check_second.report"}},
		},
	}
	yamlBytes, err := yaml.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := Compile(yamlBytes, "yaml", registry)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := Compile(encoded(t, machine), "json", registry)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Digest != expected.Digest || !bytes.Equal(plan.Canonical, expected.Canonical) {
		t.Fatal("concise YAML did not lower to the exact machine WorkflowRevision")
	}
	if plan.Workflow.SchemaVersion != "1" || plan.Workflow.Limits.MaxParallelism != 1 || plan.Workflow.Limits.MaxChildDepth != 0 {
		t.Fatal("safe authoring defaults were not applied")
	}
	resolved, _, err := ResolveWorkflowAliases(yamlBytes, "yaml", registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Compile(resolved, "json", registry); err != nil || bytes.Contains(resolved, []byte("authoring")) {
		t.Fatal("local workflow loading did not lower authoring YAML to machine JSON", err)
	}
}

func TestConciseStepYAMLLowersSafeDefaults(t *testing.T) {
	digest := "sha256:" + string(bytes.Repeat([]byte{'0'}, 64))
	source := fmt.Sprintf(`authoring: prifly-step/1
id: test:step/concise
version: 1.0.0
refs:
  plan: {id: test:schema/plan, version: 1.0.0, digest: %s}
  report: {id: test:schema/report, version: 1.0.0, digest: %s}
  adapter: {id: test:adapter/session, version: 1.0.0, digest: %s}
  instructions: {id: test:context/instructions, version: 1.0.0, digest: %s}
  result: {id: test:schema/step-result, version: 1.0.0, digest: %s}
kind: worker
inputs: {plan: plan}
outputs: {report: report}
executor: {adapter_ref: adapter, operation: session}
instructions_ref: instructions
effects: {class: none, retry_class: never}
result_schema_ref: result
`, digest, digest, digest, digest, digest)
	data, err := StepJSONBytes([]byte(source), "yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateProtocol("StepDefinitionV2", data); err != nil {
		t.Fatal(err)
	}
	var step map[string]any
	if err := json.Unmarshal(data, &step); err != nil {
		t.Fatal(err)
	}
	input := step["inputs"].(map[string]any)["plan"].(map[string]any)
	output := step["outputs"].(map[string]any)["report"].(map[string]any)
	if step["schema_version"] != "2" || step["title"] != "test:step/concise" || input["format"] != "json" || input["required"] != true || output["format"] != "json" || len(output["required_for"].([]any)) != 0 || len(step["context_refs"].([]any)) != 0 || len(step["required_capabilities"].([]any)) != 0 || len(step["result_check_refs"].([]any)) != 0 {
		t.Fatalf("concise step did not lower its safe defaults: %#v", step)
	}
	if _, exists := step["authoring"]; exists {
		t.Fatalf("sealed StepDefinition retained authoring marker: %#v", step)
	}
}

func TestStepAuthoringRejectsUnsafeSurface(t *testing.T) {
	_, err := StepJSONBytes([]byte(`authoring: prifly-step/1
id: test:step/a
version: 1.0.0
kind: worker
magic: true
`), "yaml")
	p := expectProblem(t, err, "schema_invalid")
	if p.Path != "/magic" {
		t.Fatalf("unknown authoring field points to %q", p.Path)
	}
	_, err = StepJSONBytes([]byte(`{"authoring":"prifly-step/1"}`), "json")
	expectProblem(t, err, "unsupported_authoring")
	_, err = StepJSONBytes([]byte(`authoring: prifly-step/1
schema_version: "3"
`), "yaml")
	expectProblem(t, err, "schema_invalid")
	data, err := StepJSONBytes([]byte(`authoring: prifly-step/1
id: test:step/unclassified
version: 1.0.0
kind: worker
`), "yaml")
	if err != nil {
		t.Fatal(err)
	}
	var step map[string]any
	if err := json.Unmarshal(data, &step); err != nil {
		t.Fatal(err)
	}
	if _, exists := step["effects"]; exists {
		t.Fatalf("authoring silently defaulted effects: %#v", step)
	}
}

func TestStepAuthoringReferenceIsAValidStepDefinition(t *testing.T) {
	source, err := os.ReadFile("../../examples/authoring/step-authoring-reference.yaml")
	if err != nil {
		t.Fatal(err)
	}
	data, err := StepJSONBytes(source, "yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateProtocol("StepDefinitionV6", data); err != nil {
		t.Fatal(err)
	}
	var step StepDefinition
	if err := json.Unmarshal(data, &step); err != nil {
		t.Fatal(err)
	}
	if step.SessionLimits == nil || step.SessionLimits.ActiveTimeoutMS != DefaultSessionActiveTimeoutMS || step.SessionLimits.DecisionWaitTimeoutMS != nil {
		t.Fatalf("full reference lost its documented work/wait defaults: %+v", step.SessionLimits)
	}
	if err := (&Plan{}).checkWorkspaceTrees(step, "/step"); err != nil {
		t.Fatalf("step authoring reference violates workspace-tree constraints: %v", err)
	}
}

func TestWorkspaceTreeAuthoringAndValidation(t *testing.T) {
	digest := "sha256:" + string(bytes.Repeat([]byte{'0'}, 64))
	author := func(capture, effects string) []byte {
		return []byte(fmt.Sprintf(`authoring: prifly-step/1
id: test:step/tree
version: 1.0.0
refs:
  manifest: {id: core:schema/workspace-tree-manifest, version: 1.0.0, digest: %s}
  adapter: {id: test:adapter/session, version: 1.0.0, digest: %s}
  instructions: {id: test:context/instructions, version: 1.0.0, digest: %s}
  result: {id: test:schema/step-result, version: 1.0.0, digest: %s}
kind: worker
inputs: {plan: manifest}
outputs: {plan: {schema_ref: manifest, required_for: [pass]}}
executor: {adapter_ref: adapter, operation: session}
instructions_ref: instructions
effects: %s
result_schema_ref: result
workspace_trees:
  - input_port: plan
    output_port: plan
    capture: %s
`, digest, digest, digest, digest, effects, capture))
	}
	for _, capture := range []string{
		"{kind: exact_file, path: .ai-factory/PLAN.md}",
		"{kind: direct_child_file, path: .ai-factory/plans}",
		"{kind: direct_child_tree, path: .ai-factory/plans, entrypoint: index.md}",
	} {
		data, err := StepJSONBytes(author(capture, "{class: workspace_write, retry_class: never}"), "yaml")
		if err != nil || ValidateProtocol("StepDefinitionV5", data) != nil {
			t.Fatalf("valid workspace-tree capture %s was rejected: %v", capture, err)
		}
	}
	data, err := StepJSONBytes(author("{kind: direct_child_tree, path: .ai-factory/plans, entrypoint: index.md}", "{class: workspace_write, retry_class: never}"), "yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateProtocol("StepDefinitionV5", data); err != nil {
		t.Fatal(err)
	}
	var step StepDefinition
	if err := json.Unmarshal(data, &step); err != nil || len(step.WorkspaceTrees) != 1 || step.SchemaVersion != "5" {
		t.Fatalf("workspace tree authoring did not lower: %+v %v", step, err)
	}
	bad := append([]WorkspaceTreeBinding{}, step.WorkspaceTrees...)
	bad = append(bad, bad[0])
	step.WorkspaceTrees = bad
	if err := (&Plan{}).checkWorkspaceTrees(step, "/step"); err == nil {
		t.Fatal("duplicate workspace tree binding was accepted")
	}
	step.WorkspaceTrees = step.WorkspaceTrees[:1]
	step.Effects.Class = "none"
	if err := (&Plan{}).checkWorkspaceTrees(step, "/step"); err == nil {
		t.Fatal("workspace tree without workspace_write was accepted")
	}
	for _, capture := range []string{
		"{kind: direct_child_tree, path: .ai-factory/plans, entrypoint: PLAN.md}",
		"{kind: exact_file, path: ../PLAN.md}",
		"{kind: recursive_tree, path: .ai-factory}",
	} {
		data, err := StepJSONBytes(author(capture, "{class: workspace_write, retry_class: never}"), "yaml")
		if err != nil || ValidateProtocol("StepDefinitionV5", data) == nil {
			t.Fatalf("invalid workspace-tree capture %s was accepted: %v", capture, err)
		}
	}
	if _, err := StepJSONBytes([]byte(`{"authoring":"prifly-step/1"}`), "json"); err == nil {
		t.Fatal("JSON concise authoring was accepted")
	}
}

func TestWorkflowAuthoringReferenceIsAValidWorkflowRevision(t *testing.T) {
	source, err := os.ReadFile("../../examples/authoring/workflow-authoring-reference.yaml")
	if err != nil {
		t.Fatal(err)
	}
	data, err := WorkflowJSONBytes(source, "yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateProtocol("WorkflowRevisionV3", data); err != nil {
		t.Fatal(err)
	}
}

func TestWorkflowAuthoringPreservesEveryStageKind(t *testing.T) {
	paths, err := filepath.Glob("../../test/fixtures/contracts/workflows/*.workflow.json")
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var machine map[string]any
		if err := json.Unmarshal(raw, &machine); err != nil {
			t.Fatal(err)
		}
		for _, stage := range stages(machine) {
			kind, _ := stage.(map[string]any)["kind"].(string)
			kinds[kind] = true
		}
		assertAuthoringRoundTrip(t, machine)
	}
	root, child, registry := callBindingFixture(t)
	stage := root.Definition.Stages["call"]
	stage.WorkflowRef = registerCallWorkflow(t, registry, child)
	root.Definition.Stages["call"] = stage
	var callMachine map[string]any
	if err := json.Unmarshal(encoded(t, root), &callMachine); err != nil {
		t.Fatal(err)
	}
	kinds["call"] = true
	assertAuthoringRoundTrip(t, callMachine)
	for _, kind := range []string{"step", "finish", "choice", "call", "repeat", "parallel", "map", "wait"} {
		if !kinds[kind] {
			t.Fatalf("authoring round trip did not cover %s", kind)
		}
	}
}

func TestWorkflowAuthoringDerivesV3AndNormalizesPublicationBindings(t *testing.T) {
	digest := "sha256:" + string(bytes.Repeat([]byte{'0'}, 64))
	ref := map[string]any{"id": "test:source/documents", "version": "1.0.0", "digest": digest}
	source := []byte(`authoring: prifly-workflow/1
id: test:workflow/stream
version: 1.0.0
refs:
  source:
    id: test:source/documents
    version: 1.0.0
    digest: ` + digest + `
  policy:
    id: test:policy/local
    version: 1.0.0
    digest: ` + digest + `
entry: stream
limits:
  max_step_instances: 3
  max_control_transitions: 20
policy_ref: policy
stages:
  stream:
    kind: repeat
    body_workflow_ref:
      alias: body
    initial_bindings:
      subscription: $subscription.source.handle
      cursor: $subscription.source.cursor
    next_bindings:
      subscription: $subscription.source.handle
      cursor: $iteration.next_cursor
    continue_on: [succeeded]
    until: false
    max_iterations: 3
    on_complete: {no_work: done}
    on_limit: done
  consume:
    kind: call
    workflow_ref: {alias: body}
    input_bindings:
      document: $publication.await_document
    on: {succeeded: done}
  done:
    kind: finish
    outcome: no_work
`)
	data, err := WorkflowJSONBytes(source, "yaml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow map[string]any
	if err := json.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	if workflow["schema_version"] != "3" {
		t.Fatalf("publication bindings derived schema v%v", workflow["schema_version"])
	}
	repeat := stages(workflow)["stream"].(map[string]any)
	handle := repeat["initial_bindings"].(map[string]any)["subscription"].(map[string]any)
	if handle["from"] != "subscription" || handle["port"] != "handle" || !mapsEqual(handle["source_ref"], ref) {
		t.Fatalf("subscription shorthand was not normalized: %#v", handle)
	}
	if repeat["until"].(map[string]any)["op"] != "eq" {
		t.Fatal("constant predicate shorthand was not normalized")
	}
	publication := stages(workflow)["consume"].(map[string]any)["input_bindings"].(map[string]any)["document"].(map[string]any)
	if publication["from"] != "publication" || publication["stage_id"] != "await_document" || publication["port"] != nil {
		t.Fatalf("publication shorthand was not normalized: %#v", publication)
	}
}

func TestWorkflowAuthoringRejectsUnknownSurface(t *testing.T) {
	_, err := WorkflowJSONBytes([]byte(`{"authoring":"prifly-workflow/1"}`), "json")
	expectProblem(t, err, "unsupported_authoring")

	_, err = WorkflowJSONBytes([]byte("authoring: prifly-workflow/1\nid: test:workflow/a\nversion: 1.0.0\nmagic: true\n"), "yaml")
	p := expectProblem(t, err, "schema_invalid")
	if p.Path != "/magic" {
		t.Fatalf("unknown authoring field points to %q", p.Path)
	}
	_, err = WorkflowJSONBytes([]byte(`authoring: prifly-workflow/1
id: test:workflow/a
version: 1.0.0
entry: done
stages:
  done: {kind: finish, outcome: no_work}
limits: {max_step_instances: 1, max_control_transitions: 2}
policy_ref: missing
`), "yaml")
	expectProblem(t, err, "unknown_ref")
}

func TestWorkflowAuthoringSourcesKeepDottedIdentifiers(t *testing.T) {
	digest := "sha256:" + string(bytes.Repeat([]byte{'0'}, 64))
	ref := map[string]any{"id": "test:source/documents", "version": "1.0.0", "digest": digest}
	stage, err := authorSource("$stages.review.one.report", nil, "/binding")
	if err != nil {
		t.Fatal(err)
	}
	if stage["stage_id"] != "review.one" || stage["port"] != "report" {
		t.Fatalf("dotted stage source was split incorrectly: %#v", stage)
	}
	subscription, err := authorSource("$subscription.source.main.cursor", map[string]any{"source.main": ref}, "/binding")
	if err != nil {
		t.Fatal(err)
	}
	if subscription["port"] != "cursor" || !mapsEqual(subscription["source_ref"], ref) {
		t.Fatalf("dotted reference alias was split incorrectly: %#v", subscription)
	}
}

func TestWorkflowAuthoringNormalizesCompensation(t *testing.T) {
	digest := "sha256:" + string(bytes.Repeat([]byte{'0'}, 64))
	data, err := WorkflowJSONBytes([]byte(`authoring: prifly-workflow/1
id: test:workflow/compensated
version: 1.0.0
refs:
  exact: {id: test:definition/exact, version: 1.0.0, digest: `+digest+`}
  source: {id: test:source/artifacts, version: 1.0.0, digest: `+digest+`}
entry: work
limits: {max_step_instances: 2, max_control_transitions: 8}
policy_ref: exact
stages:
  work:
    kind: step
    step_ref: exact
    on: {pass: done}
    compensation:
      workflow_ref: exact
      input_bindings:
        reason: $compensation#/reason
        subscription: $subscription.source.handle
  done: {kind: finish, outcome: succeeded}
`), "yaml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow map[string]any
	if err := json.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	compensation := stages(workflow)["work"].(map[string]any)["compensation"].(map[string]any)
	binding := compensation["input_bindings"].(map[string]any)["reason"].(map[string]any)
	subscription := compensation["input_bindings"].(map[string]any)["subscription"].(map[string]any)
	if workflow["schema_version"] != "3" || compensation["workflow_ref"].(map[string]any)["id"] != "test:definition/exact" || binding["from"] != "compensation_context" || binding["pointer"] != "/reason" || subscription["from"] != "subscription" {
		t.Fatalf("compensation shorthand was not normalized: %#v", compensation)
	}
}

func assertAuthoringRoundTrip(t *testing.T, machine map[string]any) {
	t.Helper()
	definition := machine["definition"].(map[string]any)
	source := map[string]any{
		"authoring":        WorkflowAuthoringVersion,
		"schema_version":   machine["schema_version"],
		"id":               machine["id"],
		"version":          machine["version"],
		"title":            machine["title"],
		"inputs":           machine["inputs"],
		"outputs":          machine["outputs"],
		"allowed_outcomes": machine["allowed_outcomes"],
		"entry":            definition["entry"],
		"stages":           definition["stages"],
		"limits":           machine["limits"],
		"policy_ref":       machine["policy_ref"],
	}
	yamlBytes, err := yaml.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	lowered, err := WorkflowJSONBytes(yamlBytes, "yaml")
	if err != nil {
		t.Fatal(err)
	}
	want, err := Canonical(encoded(t, machine))
	if err != nil {
		t.Fatal(err)
	}
	got, err := Canonical(lowered)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("authoring facade changed a machine WorkflowRevision field")
	}
}

func mapsEqual(left, right any) bool {
	a, _ := json.Marshal(left)
	b, _ := json.Marshal(right)
	return bytes.Equal(a, b)
}
