package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The inserted step used to receive an empty input map no matter what it
// declared, so a project whose step needed the artifact the preceding step had
// already sealed retyped that artifact into every executor prompt by hand.
const extensionInputWorkflow = `authoring: prifly-project-workflow/1
package:
  id: test:package/cycle
  version: 1.0.0
  description: One inserted step reading the preceding step's artifact.
  requires_core_protocol: "1"
  references:
    assisted: core:adapter/assisted-session@1.0.0
    policy: core:policy/local@3.0.0
    result: core:schema/step-result@1.0.0
id: test:workflow/cycle
version: 1.0.0
refs:
  step_build: "{{step_build}}"
  step_commit: "{{step_commit}}"
  policy: "{{policy}}"
inputs: {}
outputs: {}
entry: build
limits: {max_step_instances: 3, max_control_transitions: 8}
policy_ref: policy
stages:
  build:
    kind: step
    step_ref: step_build
    on: {pass: commit, fail: rejected, needs_revision: rejected, no_work: rejected}
    on_error: rejected
  commit:
    kind: step
    step_ref: step_commit
    on: {pass: done, fail: rejected, needs_revision: rejected, no_work: rejected}
    on_error: rejected
  done: {kind: finish, outcome: succeeded}
  rejected: {kind: finish, outcome: rejected}
`

func writeExtensionInputFixture(t *testing.T, root, extend string) {
	t.Helper()
	writeFixtureFile(t, root, ".prifly/project.yaml", "schema_version: prifly-project-profile/3\npackages:\n  cycle: {source: .prifly/workflows/cycle}\nlaunches: {}\n")
	writeFixtureFile(t, root, ".prifly/workflows/cycle/schemas/handoff.yaml", `id: test:schema/handoff
version: 1.0.0
$schema: https://json-schema.org/draft/2020-12/schema
type: object
`)
	step := func(name, title, ports string) {
		t.Helper()
		writeFixtureFile(t, root, ".prifly/workflows/cycle/steps/"+name+".yaml", `authoring: prifly-step/1
id: test:step/`+name+`
version: 1.0.0
title: `+title+`
kind: worker
executor: {adapter_ref: "{{assisted}}", operation: session}
effects: {class: none, retry_class: never}
result_schema_ref: "{{result}}"
`+ports)
	}
	// Both producers declare the same output so a binding to the one that has
	// not run yet is refused for that reason, not for a missing port.
	handoff := "outputs:\n  handoff:\n    schema_ref: \"{{schema_handoff}}\"\n    required_for: [pass]\n"
	step("build", "Build", handoff)
	step("commit", "Commit", handoff)
	step("tests", "Tests", "inputs:\n  handoff:\n    schema_ref: \"{{schema_handoff}}\"\n    required: true\n")
	writeFixtureFile(t, root, ".prifly/workflows/cycle/extend.yaml", extend)
	writeFixtureFile(t, root, ".prifly/workflows/cycle/workflow.yaml", extensionInputWorkflow)
}

const extensionInputExtend = `extensions:
  - id: tests-after-build
    workflow: cycle
    between: {from: build, to: commit}
    step: tests
    input_bindings: {handoff: $stages.build.handoff}
    on: {pass: commit, fail: rejected, needs_revision: rejected, no_work: rejected}
`

// The sealed graph must carry the binding, not an empty map: without it the
// artifact never reaches the inserted step's envelope.
func TestProjectCompileSealsTheInsertedStepsDeclaredInput(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	root, authority := t.TempDir(), filepath.Join(t.TempDir(), "authority")
	if code, _, stderr := runCLI(t, "project", "init", "--repository", root, "--state-root", authority); code != 0 {
		t.Fatalf("neutral init: %d %s", code, stderr)
	}
	writeExtensionInputFixture(t, root, extensionInputExtend)
	output := filepath.Join(t.TempDir(), "compiled")
	code, out, stderr := runCLI(t, "--project", authority, "project", "compile", "--repository", root, "--package", "cycle", "--output", output)
	if code != 0 {
		t.Fatalf("compile with a bound insertion: %d %s", code, stderr)
	}
	var compiled projectCompileResult
	if err := json.Unmarshal([]byte(out), &compiled); err != nil {
		t.Fatal(err)
	}
	var workflowPath string
	for _, component := range compiled.Components {
		if component.Kind == "workflow" {
			workflowPath = component.Path
		}
	}
	if workflowPath == "" {
		t.Fatalf("no workflow component was sealed: %+v", compiled.Components)
	}
	data, err := os.ReadFile(filepath.Join(output, filepath.FromSlash(workflowPath)))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	stage, ok := document["definition"].(map[string]any)["stages"].(map[string]any)["tests"].(map[string]any)
	if !ok {
		t.Fatalf("the insertion is missing from the sealed graph: %s", data)
	}
	binding, ok := stage["input_bindings"].(map[string]any)["handoff"].(map[string]any)
	if !ok {
		t.Fatalf("the inserted step received no input: %#v", stage["input_bindings"])
	}
	if binding["from"] != "stage_output" || binding["stage_id"] != "build" || binding["port"] != "handoff" {
		t.Fatalf("the binding does not read the preceding step's artifact: %#v", binding)
	}
}

// Nothing here re-implements the compiler's binding rules; this proves they run
// over an inserted stage exactly as they run over an authored one.
func TestProjectCompileRefusesAnInsertedBindingThatCannotBeSatisfied(t *testing.T) {
	for _, test := range []struct {
		name, bindings, refusal string
	}{
		{"the producer has not run on this path", "{handoff: $stages.commit.handoff}", "unavailable_output"},
		{"the producer has no such output", "{handoff: $stages.build.missing}", "unknown_port"},
		{"the step declares no such input", "{missing: $stages.build.handoff}", "project_extension_unknown_input"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("PATH", t.TempDir())
			root, authority := t.TempDir(), filepath.Join(t.TempDir(), "authority")
			if code, _, stderr := runCLI(t, "project", "init", "--repository", root, "--state-root", authority); code != 0 {
				t.Fatalf("neutral init: %d %s", code, stderr)
			}
			writeExtensionInputFixture(t, root, strings.Replace(extensionInputExtend, "{handoff: $stages.build.handoff}", test.bindings, 1))
			output := filepath.Join(t.TempDir(), "compiled")
			code, _, stderr := runCLI(t, "--project", authority, "project", "compile", "--repository", root, "--package", "cycle", "--output", output)
			if code == 0 || !strings.Contains(stderr, test.refusal) {
				t.Fatalf("an unsatisfiable insertion compiled: %d %s", code, stderr)
			}
		})
	}
}

// An inserted step that leaves a second required input unbound is the one case
// extend.yaml genuinely cannot express, and it stays refused.
func TestProjectCompileRefusesAnInsertedStepWithASecondRequiredInput(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	root, authority := t.TempDir(), filepath.Join(t.TempDir(), "authority")
	if code, _, stderr := runCLI(t, "project", "init", "--repository", root, "--state-root", authority); code != 0 {
		t.Fatalf("neutral init: %d %s", code, stderr)
	}
	writeExtensionInputFixture(t, root, extensionInputExtend)
	writeFixtureFile(t, root, ".prifly/workflows/cycle/steps/tests.yaml", `authoring: prifly-step/1
id: test:step/tests
version: 1.0.0
title: Tests
kind: worker
executor: {adapter_ref: "{{assisted}}", operation: session}
effects: {class: none, retry_class: never}
result_schema_ref: "{{result}}"
inputs:
  handoff: {schema_ref: "{{schema_handoff}}", required: true}
  budget: {schema_ref: "{{schema_handoff}}", required: true}
`)
	output := filepath.Join(t.TempDir(), "compiled")
	code, _, stderr := runCLI(t, "--project", authority, "project", "compile", "--repository", root, "--package", "cycle", "--output", output)
	if code == 0 || !strings.Contains(stderr, "project_extension_requires_full_yaml") || !strings.Contains(stderr, "budget") {
		t.Fatalf("a second required input was accepted: %d %s", code, stderr)
	}
}

// `project extend` never compiles, so the binding it writes is read back here.
func TestCLIProjectExtendWritesTheDeclaredBinding(t *testing.T) {
	dir := t.TempDir()
	workflowPath := filepath.Join(dir, "cycle.yaml")
	writeFixtureFile(t, dir, "cycle.yaml", `id: test:workflow/cycle
definition:
  stages:
    build: {kind: step, on: {pass: commit}}
    commit: {kind: finish, outcome: succeeded}
`)
	writeFixtureFile(t, dir, "extensions.yaml", `extensions:
  - id: tests-after-build
    workflow: cycle
    between: {from: build, to: commit}
    step: tests
    input_bindings: {handoff: $stages.build.handoff}
    on: {pass: commit}
`)
	writeFixtureFile(t, dir, "tests.yaml", "inputs:\n  handoff: {required: true, format: json}\n  notes: {required: false, format: json}\n")
	stepRef := `{"id":"test:step/tests","version":"1.0.0","digest":"sha256:` + strings.Repeat("a", 64) + `"}`
	outputPath := filepath.Join(dir, "compiled.json")
	code, _, stderr := runCLI(t, "project", "extend", "--workflow", workflowPath, "--workflow-id", "test:workflow/cycle",
		"--extensions", filepath.Join(dir, "extensions.yaml"), "--output", outputPath,
		"--step-ref", "tests="+stepRef, "--step-source", "tests="+filepath.Join(dir, "tests.yaml"))
	if code != 0 {
		t.Fatalf("project extend with a bound insertion: %d %s", code, stderr)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	stage := document["definition"].(map[string]any)["stages"].(map[string]any)["tests"].(map[string]any)
	binding, ok := stage["input_bindings"].(map[string]any)["handoff"].(map[string]any)
	if !ok || binding["from"] != "stage_output" || binding["stage_id"] != "build" || binding["port"] != "handoff" {
		t.Fatalf("the inserted step did not receive its declared input: %#v", stage["input_bindings"])
	}
}

// The step source is the only thing `project extend` can read the step's ports
// from, so it carries the two refusals compilation would otherwise raise.
func TestCLIProjectExtendReadsTheStepSourceForItsPorts(t *testing.T) {
	for _, test := range []struct {
		name, bindings, step, refusal string
	}{
		{"an unbound required input", "input_bindings: {handoff: $stages.build.handoff}\n", "inputs:\n  handoff: {required: true, format: json}\n  budget: {required: true, format: json}\n", "project_extension_requires_full_yaml"},
		{"a port the step never declares", "input_bindings: {handoff: $stages.build.handoff}\n", "inputs: {}\n", "project_extension_unknown_input"},
		{"a producer outside the graph", "input_bindings: {handoff: $stages.absent.handoff}\n", "inputs:\n  handoff: {required: true, format: json}\n", "project_extension_unknown_stage"},
		{"two bindings at once", "input_bindings: {handoff: $stages.build.handoff, notes: $stages.build.notes}\n", "inputs: {}\n", "must declare exactly one input"},
		{"a source that is not a stage output", "input_bindings: {handoff: $inputs.handoff}\n", "inputs: {}\n", "must read a stage output"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFixtureFile(t, dir, "cycle.yaml", `id: test:workflow/cycle
definition:
  stages:
    build: {kind: step, on: {pass: commit}}
    commit: {kind: finish, outcome: succeeded}
`)
			writeFixtureFile(t, dir, "extensions.yaml", `extensions:
  - id: tests-after-build
    workflow: cycle
    between: {from: build, to: commit}
    step: tests
    `+test.bindings+`    on: {pass: commit}
`)
			writeFixtureFile(t, dir, "tests.yaml", test.step)
			stepRef := `{"id":"test:step/tests","version":"1.0.0","digest":"sha256:` + strings.Repeat("a", 64) + `"}`
			code, _, stderr := runCLI(t, "project", "extend", "--workflow", filepath.Join(dir, "cycle.yaml"), "--workflow-id", "test:workflow/cycle",
				"--extensions", filepath.Join(dir, "extensions.yaml"), "--output", filepath.Join(dir, "compiled.json"),
				"--step-ref", "tests="+stepRef, "--step-source", "tests="+filepath.Join(dir, "tests.yaml"))
			if code == 0 || !strings.Contains(stderr, test.refusal) {
				t.Fatalf("the insertion was accepted: %d %s", code, stderr)
			}
		})
	}
}

// A dependent session edited a step under .prifly/extensions/… while the
// package's declared source was .prifly/workflows/…; the copies were
// byte-identical, so a diff said nothing, the digest did not move and the
// compile stayed green. They spent two attempts concluding the rule they were
// testing had been removed, and nearly reported it as a defect here. The result
// now names what it read, and the twin that was not read must stay out of it.
func TestProjectCompileNamesTheFilesItRead(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	root, authority := t.TempDir(), filepath.Join(t.TempDir(), "authority")
	if code, _, stderr := runCLI(t, "project", "init", "--repository", root, "--state-root", authority); code != 0 {
		t.Fatalf("neutral init: %d %s", code, stderr)
	}
	writeExtensionInputFixture(t, root, extensionInputExtend)
	// The same file name under a tree nobody declared as the package source.
	writeFixtureFile(t, root, ".prifly/extensions/cycle/steps/tests.yaml", `authoring: prifly-step/1
id: test:step/tests
version: 1.0.0
title: Tests
kind: worker
executor: {adapter_ref: "{{assisted}}", operation: session}
effects: {class: none, retry_class: never}
result_schema_ref: "{{result}}"
inputs:
  handoff: {schema_ref: "{{schema_handoff}}", required: true}
`)
	output := filepath.Join(t.TempDir(), "compiled")
	code, out, stderr := runCLI(t, "--project", authority, "project", "compile", "--repository", root, "--package", "cycle", "--output", output)
	if code != 0 {
		t.Fatalf("compile: %d %s", code, stderr)
	}
	var compiled projectCompileResult
	if err := json.Unmarshal([]byte(out), &compiled); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(compiled.Sources, ".prifly/workflows/cycle/steps/tests.yaml") {
		t.Fatalf("the step that was read is missing from sources: %v", compiled.Sources)
	}
	if slices.Contains(compiled.Sources, ".prifly/extensions/cycle/steps/tests.yaml") {
		t.Fatalf("a file this compile never read is named as read: %v", compiled.Sources)
	}
	for _, expected := range []string{".prifly/project.yaml", ".prifly/workflows/cycle/workflow.yaml", ".prifly/workflows/cycle/extend.yaml", ".prifly/workflows/cycle/schemas/handoff.yaml"} {
		if !slices.Contains(compiled.Sources, expected) {
			t.Fatalf("%s was read and is not named: %v", expected, compiled.Sources)
		}
	}
	if !slices.IsSorted(compiled.Sources) {
		t.Fatalf("sources is not sorted, so two compiles of one tree can differ by order: %v", compiled.Sources)
	}
}

// A project's inserted step could be assisted and nothing else: execution
// bindings were read from the package's root workflow.yaml alone, so a tests
// program the project owns needed an AI session to run it. extend.yaml now
// binds the steps its own extensions insert -- the same form, the same allowed
// executable and --allow-execution -- and the package's programs stay the
// package's: a binding for a step no extension inserts is refused, and a step
// bound on both sides is refused rather than overridden.
func TestProjectCompileSealsTheProjectsBindingForAnInsertedStep(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	root, authority := t.TempDir(), filepath.Join(t.TempDir(), "authority")
	if code, _, stderr := runCLI(t, "project", "init", "--repository", root, "--state-root", authority); code != 0 {
		t.Fatalf("neutral init: %d %s", code, stderr)
	}
	extend := extensionInputExtend + `execution_bindings:
  steps:
    tests:
      executable: shell
      args: [tests.sh]
      files: {tests.sh: workers/tests.sh}
      timeout_ms: 30000
      grace_ms: 100
      max_output_bytes: 65536
`
	writeExtensionInputFixture(t, root, strings.Replace(extend, "files: {tests.sh: workers/tests.sh}", "files: {tests.sh: project/workers/tests.sh}", 1))
	writeFixtureFile(t, root, ".prifly/workflows/cycle/project/workers/tests.sh", "#!/bin/sh\nexit 0\n")
	// The inserted step is a program, not a session, and it lives with its
	// program under the team's project/ subtree, not among upstream's steps.
	tests := filepath.Join(root, ".prifly/workflows/cycle/steps/tests.yaml")
	data, err := os.ReadFile(tests)
	if err != nil {
		t.Fatal(err)
	}
	program := strings.Replace(string(data), `executor: {adapter_ref: "{{assisted}}", operation: session}`, `executor: {adapter_ref: "{{process}}", operation: process}`, 1)
	if program == string(data) {
		t.Fatal("the fixture step no longer declares the assisted executor this test replaces")
	}
	if err := os.Remove(tests); err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, root, ".prifly/workflows/cycle/project/steps/tests.yaml", program)
	workflow := strings.Replace(extensionInputWorkflow, "    assisted: core:adapter/assisted-session@1.0.0\n", "    assisted: core:adapter/assisted-session@1.0.0\n    process: core:adapter/local-process@2.0.0\n", 1)
	writeFixtureFile(t, root, ".prifly/workflows/cycle/workflow.yaml", workflow)
	output := filepath.Join(t.TempDir(), "compiled")
	code, _, stderr := runCLI(t, "--project", authority, "project", "compile", "--repository", root, "--package", "cycle", "--output", output)
	if code != 0 {
		t.Fatalf("compile with a project-bound insertion: %d %s", code, stderr)
	}
	sealed, err := os.ReadFile(filepath.Join(output, projectExecutionFile))
	if err != nil {
		t.Fatalf("no execution bindings were sealed beside the package: %v", err)
	}
	var bindings struct {
		Bindings []struct {
			DefinitionRef struct {
				ID string `json:"id"`
			} `json:"definition_ref"`
			Config struct {
				Executable string   `json:"executable"`
				Args       []string `json:"args"`
			} `json:"config"`
		} `json:"bindings"`
	}
	if err := json.Unmarshal(sealed, &bindings); err != nil {
		t.Fatal(err)
	}
	if len(bindings.Bindings) != 1 || bindings.Bindings[0].DefinitionRef.ID != "test:step/tests" || bindings.Bindings[0].Config.Executable != "shell" || !slices.Equal(bindings.Bindings[0].Config.Args, []string{"tests.sh"}) {
		t.Fatalf("the project's binding for the inserted step was not sealed: %s", sealed)
	}
	// A step no extension inserts is the package's; a binding for it is refused
	// before any program is read.
	writeFixtureFile(t, root, ".prifly/workflows/cycle/extend.yaml", strings.Replace(extend, "    tests:\n", "    build:\n", 1))
	if code, _, stderr := runCLI(t, "--project", authority, "project", "compile", "--repository", root, "--package", "cycle", "--output", filepath.Join(t.TempDir(), "refused")); code == 0 || !strings.Contains(stderr, "execution_bindings.steps.build names a step no extension in this file inserts") {
		t.Fatalf("a binding for the package's own step was accepted: %d %s", code, stderr)
	}
}
