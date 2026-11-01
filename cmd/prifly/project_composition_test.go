package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const compositionImportExtension = `authoring: prifly-extension/3
imports: {ui: review-library}
extensions:
  - id: interface-review
    workflow: root
    between: {from: build, to: commit}
    call: ui.review-for-change
    input_bindings: {handoff: $stages.build.handoff}
    on: {succeeded: commit, partial: rejected, rejected: rejected}
    on_error: rejected
`

func registerCompositionDerived(t *testing.T, root, name, parents, extension string) {
	t.Helper()
	profile, err := os.ReadFile(filepath.Join(root, ".prifly/project.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, root, ".prifly/project.yaml", strings.Replace(string(profile), "launches:", "  "+name+": {source: .prifly/workflows/"+name+"}\nlaunches:", 1))
	writeFixtureFile(t, root, ".prifly/workflows/"+name+"/workflow.yaml", `authoring: prifly-project-workflow/2
package:
  id: test:package/`+name+`
  version: 1.0.0
  description: Derived composition fixture.
  requires_core_protocol: "1"
id: test:workflow/`+name+`
version: 1.0.0
title: Derived fixture
extends: `+parents+`
limits: {max_step_instances: 16, max_control_transitions: 128, max_child_depth: 8, max_parallelism: 4}
`)
	if extension != "" {
		writeFixtureFile(t, root, ".prifly/workflows/"+name+"/extend.yaml", extension)
	}
}

func compileCompositionName(t *testing.T, root, authority, name string) projectCompileResult {
	t.Helper()
	output := filepath.Join(t.TempDir(), "compiled")
	code, out, stderr := runCLI(t, "--project", authority, "project", "compile", "--repository", root, "--package", name, "--output", output)
	if code != 0 {
		t.Fatalf("compile %s: %s", name, stderr)
	}
	var result projectCompileResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestProjectCompositionSingleParentAndDiamond(t *testing.T) {
	t.Parallel()
	for _, operator := range []string{"sequence", "parallel"} {
		t.Run(operator, func(t *testing.T) {
			t.Parallel()
			root, authority := writeCompositionFixture(t)
			if err := os.Remove(filepath.Join(root, ".prifly/workflows/cycle/extend.yaml")); err != nil {
				t.Fatal(err)
			}
			left := strings.Replace(compositionImportExtension, "interface-review", "interface-left", 1)
			right := strings.Replace(compositionImportExtension, "interface-review", "interface-right", 1)
			registerCompositionDerived(t, root, "classic-ui", "cycle", left)
			registerCompositionDerived(t, root, "classic-audit", "cycle", right)
			compileCompositionName(t, root, authority, "classic-ui")
			extend := `authoring: prifly-extension/3
arrange:
  - id: extra-checks
    between: {from: build, to: commit}
    ` + operator + `: [classic-ui.interface-left, classic-audit.interface-right]
    on: {succeeded: commit, rejected: rejected}
`
			registerCompositionDerived(t, root, "delivery", "[classic-ui, classic-audit]", extend)
			compiled := compileCompositionName(t, root, authority, "delivery")
			counts := map[string]int{}
			for _, component := range compiled.Components {
				counts[component.Ref.ID]++
			}
			if counts["test:step/build"] != 1 || counts["test:step/commit"] != 1 || counts["test:workflow/cycle"] != 0 || counts["test:workflow/delivery"] != 1 {
				t.Fatalf("diamond duplicated common graph: %v", counts)
			}
		})
	}
}

func writeCompositionFixture(t *testing.T) (string, string) {
	t.Helper()
	root, authority := writeCallExtensionFixture(t, compositionImportExtension)
	data, err := os.ReadFile(filepath.Join(root, ".prifly/project.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, root, ".prifly/project.yaml", strings.Replace(string(data), "launches:", "  review-library: {source: .prifly/workflows/review-library}\nlaunches:", 1))
	provider := strings.Replace(extensionInputWorkflow, "prifly-project-workflow/1", "prifly-project-workflow/2", 1)
	provider = strings.ReplaceAll(provider, "test:package/cycle", "test:package/review-library")
	provider = strings.ReplaceAll(provider, "test:workflow/cycle", "test:workflow/library-root")
	provider += "exports:\n  workflows:\n    review-for-change: \"{{workflow_review}}\"\n"
	writeFixtureFile(t, root, ".prifly/workflows/review-library/workflow.yaml", provider)
	for _, relative := range []string{"schemas/handoff.yaml", "steps/tests.yaml", "project/workflows/z-review.yaml"} {
		data, err := os.ReadFile(filepath.Join(root, ".prifly/workflows/cycle", relative))
		if err != nil {
			t.Fatal(err)
		}
		writeFixtureFile(t, root, ".prifly/workflows/review-library/"+relative, string(data))
	}
	// Only the provider owns these components. Its standalone graph is not
	// executable in the consumer, and its standalone overrides are not read.
	for _, relative := range []string{"steps/tests.yaml", "project/workflows/z-review.yaml"} {
		if err := os.Remove(filepath.Join(root, ".prifly/workflows/cycle", relative)); err != nil {
			t.Fatal(err)
		}
	}
	writeFixtureFile(t, root, ".prifly/workflows/review-library/extend.yaml", "this is deliberately not a standalone extension\n")
	return root, authority
}

func TestProjectCompositionImportsSourceExport(t *testing.T) {
	t.Parallel()
	root, authority := writeCompositionFixture(t)
	compiled, output, problem := compileCallExtensionFixture(t, root, authority)
	if problem != nil {
		t.Fatalf("compile: %+v", problem)
	}
	counts := map[string]int{}
	for _, component := range compiled.Components {
		counts[component.Ref.ID]++
	}
	if counts["test:workflow/review"] != 1 || counts["test:workflow/library-root"] != 0 || counts["test:step/tests"] != 1 {
		t.Fatalf("wrong import closure: %v", counts)
	}
	data, err := os.ReadFile(filepath.Join(output, projectBuildFile))
	if err != nil {
		t.Fatal(err)
	}
	var provenance map[string]any
	if err := json.Unmarshal(data, &provenance); err != nil {
		t.Fatal(err)
	}
}

func TestProjectCompositionRejectsPrivateAndWrongKind(t *testing.T) {
	t.Parallel()
	for _, target := range []string{"ui.review", "missing.review-for-change"} {
		t.Run(target, func(t *testing.T) {
			t.Parallel()
			root, authority := writeCompositionFixture(t)
			writeFixtureFile(t, root, ".prifly/workflows/cycle/extend.yaml", strings.Replace(compositionImportExtension, "ui.review-for-change", target, 1))
			_, _, problem := compileCallExtensionFixture(t, root, authority)
			if problem == nil {
				t.Fatal("private or undeclared import was accepted")
			}
		})
	}
}
