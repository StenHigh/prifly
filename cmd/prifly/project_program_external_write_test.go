package main

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

// The pilot's reproduction: a project's program step inserted into a package
// declares that it changes an external system. On 0.13.64 every way of writing
// it was refused, and the step had to say class none. It now compiles to step
// contract 14 carrying the boundary.
func TestProjectCompileSealsAProgramStepsExternalWrite(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	root, authority := t.TempDir(), filepath.Join(t.TempDir(), "authority")
	if code, _, stderr := runCLI(t, "project", "init", "--repository", root, "--state-root", authority); code != 0 {
		t.Fatalf("neutral init: %d %s", code, stderr)
	}
	extend := strings.Replace(extensionInputExtend+`execution_bindings:
  steps:
    tests:
      executable: shell
      args: [tests.sh]
      files: {tests.sh: workers/tests.sh}
      timeout_ms: 30000
      grace_ms: 100
      max_output_bytes: 65536
`, "files: {tests.sh: workers/tests.sh}", "files: {tests.sh: project/workers/tests.sh}", 1)
	writeExtensionInputFixture(t, root, extend)
	writeFixtureFile(t, root, ".prifly/workflows/cycle/project/workers/tests.sh", "#!/bin/sh\nexit 0\n")
	tests := filepath.Join(root, ".prifly/workflows/cycle/steps/tests.yaml")
	data, err := os.ReadFile(tests)
	if err != nil {
		t.Fatal(err)
	}
	program := strings.Replace(string(data), `executor: {adapter_ref: "{{assisted}}", operation: session}`, `executor: {adapter_ref: "{{process}}", operation: process}`, 1)
	program = strings.Replace(program, "effects: {class: none, retry_class: never}\n", "effects: {class: external_write, retry_class: idempotent}\nexternal_write: {system: example-system, operations: [environment.ensure, environment.remove], target: example-environment}\n", 1)
	if !strings.Contains(program, "operation: process") || !strings.Contains(program, "external_write:") {
		t.Fatal("the fixture step no longer has the lines this test replaces")
	}
	if err := os.Remove(tests); err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, root, ".prifly/workflows/cycle/project/steps/tests.yaml", program)
	writeFixtureFile(t, root, ".prifly/workflows/cycle/extend.yaml", extend+"references:\n  process: core:adapter/local-process@2.0.0\n")
	output := filepath.Join(t.TempDir(), "compiled")
	if code, _, stderr := runCLI(t, "--project", authority, "project", "compile", "--repository", root, "--package", "cycle", "--output", output); code != 0 {
		t.Fatalf("a program step declaring an external write did not compile: %d %s", code, stderr)
	}
	var sealed *flow.StepDefinition
	if err := filepath.WalkDir(output, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".json") {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var step flow.StepDefinition
		if json.Unmarshal(content, &step) == nil && step.ID == "test:step/tests" {
			sealed = &step
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if sealed == nil {
		t.Fatal("the compiled package has no tests step")
	}
	if sealed.SchemaVersion != "14" || sealed.Effects.Class != "external_write" || sealed.Effects.RetryClass != "idempotent" || sealed.ExternalWrite == nil || sealed.ExternalWrite.Target != "example-environment" {
		t.Fatalf("the compiled step lost the declaration: %+v", sealed)
	}
}
