package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Two releases in a row published a contract that accepts an output promised
// for the blocked verdict and a path that refuses it. The second time the
// mapping from a step's schema_version to its contract had been written out a
// second time in this package; it stopped at 9, so a step lowered to 10 fell
// through to the base contract, whose output port refuses the very verdict the
// tenth exists for.
//
// This enters where the package session enters: project compile, over a source
// carrying {{...}} placeholders, because that was their other hypothesis and it
// has to be ruled out by the same run rather than by argument.
func TestProjectCompileAcceptsAnOutputPromisedForTheBlockedVerdict(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	root, authority := t.TempDir(), filepath.Join(t.TempDir(), "authority")
	if code, _, stderr := runCLI(t, "project", "init", "--repository", root, "--state-root", authority); code != 0 {
		t.Fatalf("neutral init: %d %s", code, stderr)
	}
	writeExtensionInputFixture(t, root, extensionInputExtend)
	// The same fixture, with one producer promising its output for the verdict
	// that judged nothing -- which is what a gate handing over its findings on
	// an unreachable dependency declares.
	writeFixtureFile(t, root, ".prifly/workflows/cycle/steps/build.yaml", `authoring: prifly-step/1
id: test:step/build
version: 1.0.0
title: Build
kind: worker
executor: {adapter_ref: "{{assisted}}", operation: session}
effects: {class: none, retry_class: never}
result_schema_ref: "{{result}}"
outputs:
  handoff:
    schema_ref: "{{schema_handoff}}"
    required_for: [pass, blocked]
`)
	output := filepath.Join(t.TempDir(), "compiled")
	code, out, stderr := runCLI(t, "--project", authority, "project", "compile", "--repository", root, "--package", "cycle", "--output", output)
	if code != 0 {
		t.Fatalf("compile refused an output promised for the verdict: %d %s", code, stderr)
	}
	var compiled projectCompileResult
	if err := json.Unmarshal([]byte(out), &compiled); err != nil {
		t.Fatal(err)
	}
	var stepPath string
	for _, component := range compiled.Components {
		if component.Kind == "step" && strings.HasPrefix(component.Ref.ID, "test:step/build") {
			stepPath = component.Path
		}
	}
	if stepPath == "" {
		t.Fatalf("the step was not sealed: %+v", compiled.Components)
	}
	data, err := os.ReadFile(filepath.Join(output, filepath.FromSlash(stepPath)))
	if err != nil {
		t.Fatal(err)
	}
	var sealed map[string]any
	if err := json.Unmarshal(data, &sealed); err != nil {
		t.Fatal(err)
	}
	// Sealed at the contract that carries the promise, not at whichever one the
	// rest of the source needed: the bytes a Run reads have to be the ones the
	// promise is valid under.
	if sealed["schema_version"] != "10" {
		t.Fatalf("sealed at contract %v, expected the one that carries the promise", sealed["schema_version"])
	}
	verdicts := sealed["outputs"].(map[string]any)["handoff"].(map[string]any)["required_for"].([]any)
	if len(verdicts) != 2 || verdicts[1] != "blocked" {
		t.Fatalf("the sealed step lost the promise: %v", verdicts)
	}
}
