package flow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"slices"
	"strings"

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
	// The reference demonstrates every field a step may declare, so it lowers
	// to the newest contract that carries them all. Pinning an older one here
	// would quietly stop checking whatever was added since.
	if err := ValidateProtocol("StepDefinitionV10", data); err != nil {
		t.Fatal(err)
	}
	var step StepDefinition
	if err := json.Unmarshal(data, &step); err != nil {
		t.Fatal(err)
	}
	if step.SchemaVersion != "10" || step.SessionLimits == nil || step.SessionLimits.ActiveTimeoutMS != nil || step.SessionLimits.DecisionWaitTimeoutMS != nil {
		t.Fatalf("full reference lost the declared absence of both deadlines: %+v", step.SessionLimits)
	}
	if err := (&Plan{}).checkWorkspaceTrees(step, "/step"); err != nil {
		t.Fatalf("step authoring reference violates workspace-tree constraints: %v", err)
	}
	// The reference teaches every field, so every field must survive lowering.
	if step.ModelProfile == nil || step.ModelProfile.Requested == "" || step.ModelProfile.Reason == "" {
		t.Fatalf("full reference lost its declared model profile: %+v", step.ModelProfile)
	}
	if !slices.Contains(step.Outputs["report"].RequiredFor, "blocked") {
		t.Fatalf("full reference lost the output promised for the blocked verdict: %+v", step.Outputs["report"].RequiredFor)
	}
}

// The contract accepting a verdict and the authoring path accepting it are two
// statements. The first shipped in 0.13.51 and the second did not: a source
// naming it lowered to the contract its other fields needed, and that contract's
// port refused the word. The check goes through the lowering because the one
// that went straight to the contract is what let it ship.
func TestAuthoringDerivesTheContractThatCarriesTheVerdict(t *testing.T) {
	for _, test := range []struct {
		name, marker, want string
		verdicts           []any
	}{
		// The assisted form is covered by the reference above, which is written
		// at prifly-step/2 and now lowers to 10 through the same path. This
		// covers the program form, which a gate is just as likely to be.
		{"program source promising it", StepAuthoringVersion, "10", []any{"pass", "blocked"}},
		{"program source without it", StepAuthoringVersion, "2", []any{"pass"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			digest := "sha256:" + strings.Repeat("0", 64)
			ref := map[string]any{"id": "test:schema/out", "version": "1.0.0", "digest": digest}
			executor := map[string]any{"adapter_ref": "adapter", "operation": "process"}
			source := map[string]any{
				"authoring": test.marker,
				"id":        "test:step/derive", "version": "1.0.0", "title": "Derive",
				"refs":              map[string]any{"out": ref, "adapter": map[string]any{"id": "test:adapter/x", "version": "1.0.0", "digest": digest}, "result": ref},
				"kind":              "command",
				"inputs":            map[string]any{},
				"outputs":           map[string]any{"gate": map[string]any{"schema_ref": "out", "required_for": test.verdicts}},
				"executor":          executor,
				"effects":           map[string]any{"class": "none", "retry_class": "never"},
				"result_schema_ref": "result",
			}
			encoded, err := json.Marshal(source)
			if err != nil {
				t.Fatal(err)
			}
			var asYAML map[string]any
			if err := json.Unmarshal(encoded, &asYAML); err != nil {
				t.Fatal(err)
			}
			lowered, err := lowerStepAuthoring(asYAML)
			if err != nil {
				t.Fatalf("the authoring path refused a source the contract accepts: %v", err)
			}
			if lowered["schema_version"] != test.want {
				t.Fatalf("lowered to %v, expected %s", lowered["schema_version"], test.want)
			}
		})
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
	// The reference documents every authoring field, and only v4 admits
	// impossible_verdicts, so that is the contract it lowers to.
	if err := ValidateProtocol("WorkflowRevisionV4", data); err != nil {
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

// A program step written against prifly-step/2 was refused on the adapter id
// with the assisted adapter named as the declared value -- a typo to fix, as
// far as the author could tell; the pilot's first hour with a program step
// went there. The refusal now names the authoring version a program uses.
func TestAProgramStepUnderTheSessionAuthoringNamesTheVersionToUse(t *testing.T) {
	_, err := StepJSONBytes([]byte(`authoring: prifly-step/2
id: test:step/tests
version: 1.0.0
kind: worker
executor: {adapter_ref: {id: core:adapter/local-process, version: 2.0.0, digest: "sha256:0000000000000000000000000000000000000000000000000000000000000000"}, operation: process}
`), "yaml")
	p := expectProblem(t, err, "schema_invalid")
	if p.Path != "/executor/operation" || !strings.Contains(p.Message, "a program step (operation: process) is written as authoring: prifly-step/1") {
		t.Fatalf("the refusal does not name the authoring version a program uses: %s %s", p.Path, p.Message)
	}
}

// A read-only step reads a captured tree through a binding with no output
// port. Only v8 carries it, only on effects none, and never beside a captured
// binding; the older contracts keep refusing the absent port.
func TestMaterializeOnlyWorkspaceTreeAuthoringAndValidation(t *testing.T) {
	digest := "sha256:" + string(bytes.Repeat([]byte{'0'}, 64))
	author := func(effects, version string) []byte {
		return []byte(fmt.Sprintf(`authoring: prifly-step/1
id: test:step/verify
version: 1.0.0
%srefs:
  manifest: {id: core:schema/workspace-tree-manifest, version: 1.0.0, digest: %s}
  adapter: {id: core:adapter/assisted-session, version: 1.0.0, digest: %s}
  instructions: {id: test:context/instructions, version: 1.0.0, digest: %s}
  result: {id: test:schema/step-result, version: 1.0.0, digest: %s}
kind: worker
inputs: {plan: manifest}
outputs: {}
executor: {adapter_ref: adapter, operation: session}
instructions_ref: instructions
effects: %s
result_schema_ref: result
workspace_trees:
  - input_port: plan
    capture: {kind: exact_file, path: .ai-factory/PLAN.md}
`, version, digest, digest, digest, digest, effects))
	}
	data, err := StepJSONBytes(author("{class: none, retry_class: never}", ""), "yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateProtocol("StepDefinitionV8", data); err != nil {
		t.Fatalf("materialize-only binding rejected by v8: %v", err)
	}
	if err := ValidateProtocol("StepDefinitionV7", data); err == nil {
		t.Fatal("v7 accepted a binding without output_port")
	}
	var step StepDefinition
	if err := json.Unmarshal(data, &step); err != nil || step.SchemaVersion != "8" || len(step.WorkspaceTrees) != 1 || !step.WorkspaceTrees[0].MaterializeOnly() {
		t.Fatalf("authoring did not lower a materialize-only binding to v8: %+v %v", step, err)
	}
	if err := (&Plan{}).checkWorkspaceTrees(step, "/step"); err != nil {
		t.Fatalf("materialize-only binding on a read-only step was refused: %v", err)
	}
	if _, err := StepJSONBytes(author("{class: none, retry_class: never}", "schema_version: \"7\"\n"), "yaml"); err == nil {
		t.Fatal("an author pinning v7 sealed a binding without output_port")
	}
	writing := step
	writing.Effects.Class = "workspace_write"
	if err := (&Plan{}).checkWorkspaceTrees(writing, "/step"); err == nil {
		t.Fatal("materialize-only binding on a writing step was accepted")
	}
	pinned := step
	pinned.SchemaVersion = "7"
	if err := (&Plan{}).checkWorkspaceTrees(pinned, "/step"); err == nil {
		t.Fatal("materialize-only binding under v7 was accepted by the compiler")
	}
	mixed := step
	mixed.Outputs = map[string]OutputPort{"plan_out": {Port: step.Inputs["plan"].Port, RequiredFor: []string{"pass"}}}
	mixed.WorkspaceTrees = append(append([]WorkspaceTreeBinding{}, step.WorkspaceTrees...), WorkspaceTreeBinding{OutputPort: "plan_out", Capture: WorkspaceTreeCapturePolicy{Kind: "direct_child_file", Path: ".ai-factory/plans"}})
	if err := (&Plan{}).checkWorkspaceTrees(mixed, "/step"); err == nil {
		t.Fatal("a step mixing materialize-only and captured trees was accepted")
	}
}

// A step that judges someone else's work and a step that does it want
// different things from a model, and until now that knowledge lived in the
// host's head. The declaration is a property of the step, so it is sealed in
// the plan; what the host did with it is a fact of one execution and belongs
// in the report, not here.
func TestStepDeclaresTheModelProfileItWants(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	author := func(extra string) []byte {
		return []byte(fmt.Sprintf(`authoring: prifly-step/2
id: test:step/review
version: 1.0.0
refs:
  adapter: {id: core:adapter/assisted-session, version: 1.0.0, digest: %s}
  instructions: {id: test:context/instructions, version: 1.0.0, digest: %s}
  result: {id: test:schema/step-result, version: 1.0.0, digest: %s}
kind: worker
inputs: {}
outputs: {}
executor: {adapter_ref: adapter, operation: session}
instructions_ref: instructions
effects: {class: none, retry_class: never}
result_schema_ref: result
%s`, digest, digest, digest, extra))
	}
	data, err := StepJSONBytes(author("model_profile:\n  requested: deep-reasoning\n  reason: this step judges work it did not do\n"), "yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateProtocol("StepDefinitionV9", data); err != nil {
		t.Fatalf("a declared model profile was rejected by v9: %v", err)
	}
	if err := ValidateProtocol("StepDefinitionV8", data); err == nil {
		t.Fatal("v8 accepted a field it never declared")
	}
	var step StepDefinition
	if err := json.Unmarshal(data, &step); err != nil {
		t.Fatal(err)
	}
	if step.SchemaVersion != "9" || step.ModelProfile == nil || step.ModelProfile.Requested != "deep-reasoning" {
		t.Fatalf("the declaration did not reach the plan: %+v", step.ModelProfile)
	}
	if step.ModelProfile.Reason == "" {
		t.Fatal("a declaration without its reason tells the next author nothing")
	}
	// Saying nothing new must still seal the bytes it sealed before v9 existed.
	plain, err := StepJSONBytes(author("session_limits: {active_timeout_ms: null}\n"), "yaml")
	if err != nil {
		t.Fatal(err)
	}
	var untouched StepDefinition
	if err := json.Unmarshal(plain, &untouched); err != nil {
		t.Fatal(err)
	}
	if untouched.SchemaVersion != "7" || untouched.ModelProfile != nil {
		t.Fatalf("a step that declares no profile moved to %s: %+v", untouched.SchemaVersion, untouched.ModelProfile)
	}
}

// Found on 0.13.41 by the first package that declared a model profile on every
// step: a read-only gate handed a captured tree and asking for a careful model
// could not compile. The binding check listed the one version that introduced
// the form instead of asking whether the step is at least that version, so v9
// -- which is v8 plus a field -- was refused by a rule v8 passes.
func TestAMaterializeOnlyTreeSurvivesALaterStepContract(t *testing.T) {
	manifest := Ref{ID: WorkspaceTreeManifestSchemaID, Version: "1.0.0", Digest: "sha256:" + strings.Repeat("a", 64)}
	step := StepDefinition{
		SchemaVersion: "9", ID: "test:step/review", Version: "1.0.0", Kind: "worker",
		Inputs:         map[string]InputPort{"plan": {Port: Port{Format: "json", SchemaRef: &manifest}}},
		Outputs:        map[string]OutputPort{},
		WorkspaceTrees: []WorkspaceTreeBinding{{InputPort: "plan", Capture: WorkspaceTreeCapturePolicy{Kind: "exact_file", Path: ".ai-factory/PLAN.md"}}},
		ModelProfile:   &ModelProfile{Requested: "careful-review", Reason: "judges work it did not do"},
	}
	step.Effects.Class = "none"
	if err := (&Plan{}).checkWorkspaceTrees(step, "/step"); err != nil {
		t.Fatalf("a materialize-only tree on a v9 step was refused: %v", err)
	}
	// The form still needs the contract that introduced it.
	earlier := step
	earlier.SchemaVersion = "7"
	if err := (&Plan{}).checkWorkspaceTrees(earlier, "/step"); err == nil {
		t.Fatal("v7 accepted a binding without output_port")
	}
	// A capturing binding on the later contract is unaffected.
	writing := step
	writing.Effects.Class = "workspace_write"
	writing.Outputs = map[string]OutputPort{"plan": {Port: Port{Format: "json", SchemaRef: &manifest}, RequiredFor: []string{"pass"}}}
	writing.WorkspaceTrees = []WorkspaceTreeBinding{{InputPort: "plan", OutputPort: "plan", Capture: WorkspaceTreeCapturePolicy{Kind: "exact_file", Path: ".ai-factory/PLAN.md"}}}
	if err := (&Plan{}).checkWorkspaceTrees(writing, "/step"); err != nil {
		t.Fatalf("a capturing tree on a v9 step was refused: %v", err)
	}
}
