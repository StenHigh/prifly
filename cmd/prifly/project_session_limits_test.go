package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

func projectSessionLimitsFixture(t *testing.T) (string, string, string) {
	t.Helper()
	root, authority := t.TempDir(), filepath.Join(t.TempDir(), "authority")
	if code, _, stderr := runCLI(t, "project", "init", "--repository", root, "--state-root", authority, "--host", "codex-app"); code != 0 {
		t.Fatalf("init neutral project: %d %s", code, stderr)
	}
	const folder = ".prifly/workflows/limits/"
	writeFixtureFile(t, root, ".prifly/project.yaml", `schema_version: prifly-project-profile/3
hosts: {codex-app: .agents/skills}
packages: {limits: {source: .prifly/workflows/limits}}
launches:
  limits:
    title: Inspect a report
    description: A neutral compilation fixture with an assisted inspection.
    kind: workflow
    workflow: .prifly/workflows/limits/workflow.yaml
`)
	workflow := fixtureWorkflowYAML("limits")
	workflow = strings.Replace(workflow, "  references:\n", "  references:\n    assisted: core:adapter/assisted-session@1.0.0\n    result: core:schema/step-result@1.0.0\n", 1)
	workflow = strings.Replace(workflow, "entry: done", "entry: inspect", 1)
	workflow = strings.Replace(workflow, "max_control_transitions: 1", "max_control_transitions: 3", 1)
	workflow = strings.Replace(workflow, "stages:\n", "stages:\n  inspect: {kind: step, step_ref: \"{{step_inspect}}\", on: {pass: done}}\n", 1)
	writeFixtureFile(t, root, folder+"workflow.yaml", workflow)
	writeFixtureFile(t, root, folder+"contexts/inspect.yaml", `id: test:context/inspect
version: 1.0.0
media_type: text/plain
text: Inspect the supplied report and return the declared result.
`)
	const legacyStep = `authoring: prifly-step/1
id: test:step/inspect
version: 1.0.0
kind: worker
executor: {adapter_ref: "{{assisted}}", operation: session}
instructions_ref: "{{context_inspect}}"
effects: {class: none, retry_class: never}
result_schema_ref: "{{result}}"
`
	return root, authority, legacyStep
}

// Compile only: this test proves the authoring/sealing boundary, not that a
// host has consumed the timed session protocol or that a Run has resumed.
func TestCLIProjectSessionLimitsSealDistinctRevisions(t *testing.T) {
	root, _, legacyStep := projectSessionLimitsFixture(t)
	const folder = ".prifly/workflows/limits/"
	type sealed struct {
		result projectCompileResult
		files  map[string][]byte
		dir    string
		step   flow.StepDefinition
	}
	compile := func(source string) sealed {
		t.Helper()
		writeFixtureFile(t, root, folder+"steps/inspect.yaml", source)
		directory := filepath.Join(t.TempDir(), "package")
		code, out, stderr := runCLI(t, "project", "compile", "--repository", root, "--package", "limits", "--output", directory)
		if code != 0 {
			t.Fatalf("compile limits: %d %s", code, stderr)
		}
		value := sealed{dir: directory, files: map[string][]byte{}}
		if err := json.Unmarshal([]byte(out), &value.result); err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(value.result.Package.Version, "0.0.0-b1.") || value.result.BuildKey == "" {
			t.Fatalf("compile did not select the explicit /3 identity contract: %+v", value.result)
		}
		for _, component := range value.result.Components {
			data, err := os.ReadFile(filepath.Join(directory, component.Path))
			if err != nil {
				t.Fatal(err)
			}
			value.files[component.Path] = data
			if component.Ref.ID == "test:step/inspect" {
				if err := json.Unmarshal(data, &value.step); err != nil {
					t.Fatal(err)
				}
			}
		}
		if value.step.ID == "" {
			t.Fatal("compiled package omitted the inspected step")
		}
		return value
	}
	assertSame := func(a, b sealed) {
		t.Helper()
		if a.result.Package != b.result.Package || a.result.BuildKey != b.result.BuildKey || !reflect.DeepEqual(a.files, b.files) {
			t.Fatal("identical effective authoring did not reproduce the same sealed package")
		}
	}
	legacy := compile(legacyStep)
	if legacy.step.SchemaVersion != "2" || legacy.step.SessionLimits != nil {
		t.Fatal("legacy source acquired a timed contract")
	}
	timedSource := strings.Replace(legacyStep, "prifly-step/1", "prifly-step/2", 1)
	timed := compile(timedSource)
	if timed.step.SchemaVersion != "6" || timed.step.SessionLimits == nil || timed.step.SessionLimits.ActiveTimeoutMS != 3600000 || timed.step.SessionLimits.DecisionWaitTimeoutMS != nil {
		t.Fatalf("new marker did not seal its defaults: %+v", timed.step)
	}
	assertSame(timed, compile(timedSource+"session_limits: {active_timeout_ms: 3600000, decision_wait_timeout_ms: null}\n"))
	active := compile(timedSource + "session_limits: {active_timeout_ms: 7200000}\n")
	wait := compile(timedSource + "session_limits: {decision_wait_timeout_ms: 1209600000}\n")
	if active.step.SessionLimits.ActiveTimeoutMS != 7200000 || active.step.SessionLimits.DecisionWaitTimeoutMS != nil || wait.step.SessionLimits.ActiveTimeoutMS != 3600000 || wait.step.SessionLimits.DecisionWaitTimeoutMS == nil || *wait.step.SessionLimits.DecisionWaitTimeoutMS != 1209600000 {
		t.Fatal("independent active/wait edits did not reach the sealed definition")
	}
	seen := map[flow.Ref]bool{}
	for _, variant := range []sealed{legacy, timed, active, wait} {
		for _, component := range variant.result.Components {
			if seen[component.Ref] {
				t.Fatalf("different effective limits reused an owned compiled ref: %+v", component.Ref)
			}
			seen[component.Ref] = true
		}
		if seen[variant.result.Package] {
			t.Fatal("different effective limits reused the package revision")
		}
		seen[variant.result.Package] = true
		for path, before := range variant.files {
			after, err := os.ReadFile(filepath.Join(variant.dir, path))
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("later compilation rewrote an earlier sealed package: %s %v", path, err)
			}
		}
	}
	assertSame(legacy, compile(legacyStep))
}
