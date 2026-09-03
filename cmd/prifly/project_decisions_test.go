package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func TestProjectWorkflowDecisionCatalog(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, ".prifly", "workflows", "sample")
	write := func(name, text string) {
		t.Helper()
		path := filepath.Join(folder, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0644); err != nil {
			t.Fatal(err)
		}
	}
	workflow := func(catalog string) {
		write("workflow.yaml", `authoring: prifly-project-workflow/1
package:
  id: test:package/sample
  version: 1.0.0
  description: Decision catalog fixture.
  requires_core_protocol: "1"
id: test:workflow/sample
version: 1.0.0
inputs: {verbosity: {}}
decision_catalog: `+catalog+`
entry: done
stages: {done: {kind: finish, outcome: succeeded}}
limits: {max_step_instances: 1, max_control_transitions: 1}
policy_ref: {id: test:policy/local, version: 1.0.0, digest: sha256:0000000000000000000000000000000000000000000000000000000000000000}
`)
	}
	decision := func(id, destination string) string {
		return `authoring: prifly-run-decision/1
id: ` + id + `
title: Verbosity
phase: preflight
choices: [{id: concise, title: Concise, value: concise}]
destination: ` + destination + `
`
	}
	write("decisions/nested/verbosity.yaml", decision("verbosity", "{kind: session_context, name: verbosity}"))
	workflow("[.prifly/workflows/sample/decisions/nested/verbosity.yaml]")
	source, err := readProjectWorkflowFolder(root, folder)
	if err != nil || len(source.DecisionCatalog) != 1 || source.DecisionCatalog[0].ID != "verbosity" {
		t.Fatalf("valid nested decision catalog: %v %#v", err, source.DecisionCatalog)
	}
	output := t.TempDir()
	component := projectCompileComponent{Kind: "workflow", Ref: flow.Ref{ID: "test:workflow/sample", Version: "1.0.0", Digest: "sha256:" + strings.Repeat("0", 64)}, Path: "workflows/000.json", Bytes: []byte(`{}`)}
	if err := os.MkdirAll(filepath.Join(output, "workflows"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(output, component.Path), component.Bytes, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := writeProjectPackageManifest(output, source, []projectCompileComponent{component}, prifly.PackageRecord{}); err != nil {
		t.Fatalf("seal decision catalog: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(output, projectDecisionCatalogFile)); err != nil || !strings.Contains(string(data), `"id":"verbosity"`) {
		t.Fatalf("sealed catalog missing exact decision: %v %s", err, data)
	}

	write("decisions/roadmap-linkage.yaml", strings.ReplaceAll(decision("roadmap_linkage", "{kind: session_context, name: roadmap_linkage}"), "choices: [{id: concise, title: Concise, value: concise}]", "choices: [{id: link, title: Link, value: link}]"))
	write("decisions/roadmap-milestone.yaml", strings.ReplaceAll(decision("roadmap_milestone", "{kind: session_context, name: roadmap_milestone}"), "choices: [{id: concise, title: Concise, value: concise}]", "choices: [{id: current, title: Current, value: current}]\nwhen: {answers: {roadmap_linkage: link}}"))
	workflow("[.prifly/workflows/sample/decisions/roadmap-linkage.yaml, .prifly/workflows/sample/decisions/roadmap-milestone.yaml]")
	if source, err := readProjectWorkflowFolder(root, folder); err != nil || len(source.DecisionCatalog) != 2 {
		t.Fatalf("conditional decision catalog: %v %#v", err, source.DecisionCatalog)
	}
	workflow("[.prifly/workflows/sample/decisions/roadmap-milestone.yaml, .prifly/workflows/sample/decisions/roadmap-linkage.yaml]")
	if _, err := readProjectWorkflowFolder(root, folder); err == nil || !strings.Contains(err.Error(), "unknown or forward preflight predecessor roadmap_linkage") {
		t.Fatalf("forward decision condition was accepted: %v", err)
	}

	write("decisions/duplicate.yaml", decision("verbosity", "{kind: session_context, name: other}"))
	workflow("[.prifly/workflows/sample/decisions/nested/verbosity.yaml, .prifly/workflows/sample/decisions/duplicate.yaml]")
	if _, err := readProjectWorkflowFolder(root, folder); err == nil || !strings.Contains(err.Error(), "duplicate decision ID") {
		t.Fatalf("duplicate decision ID was accepted: %v", err)
	}

	write("decisions/unknown-input.yaml", decision("input_choice", "{kind: launch_input, name: missing}"))
	workflow("[.prifly/workflows/sample/decisions/unknown-input.yaml]")
	if _, err := readProjectWorkflowFolder(root, folder); err == nil || !strings.Contains(err.Error(), "unknown launch input") {
		t.Fatalf("unknown destination was accepted: %v", err)
	}

	outside := filepath.Join(root, ".prifly", "outside.yaml")
	if err := os.WriteFile(outside, []byte(decision("outside", "{kind: session_context, name: outside}")), 0644); err != nil {
		t.Fatal(err)
	}
	workflow("[.prifly/outside.yaml]")
	if _, err := readProjectWorkflowFolder(root, folder); err == nil || !strings.Contains(err.Error(), "stay inside its workflow folder") {
		t.Fatalf("traversal source was accepted: %v", err)
	}

	write("decisions/graph.yaml", decision("graph", "{kind: session_context, name: graph}")+"stages: {surprise: {kind: finish}}\n")
	workflow("[.prifly/workflows/sample/decisions/graph.yaml]")
	if _, err := readProjectWorkflowFolder(root, folder); err == nil || !strings.Contains(err.Error(), "unknown field stages") {
		t.Fatalf("decision was allowed to add workflow graph data: %v", err)
	}
}

func TestCLIProjectQuestionnaireIsReadOnly(t *testing.T) {
	repository := filepath.Join(t.TempDir(), "repository")
	if output, err := exec.Command("git", "init", "-q", repository).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	write := func(name, text string) {
		t.Helper()
		path := filepath.Join(repository, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write(".prifly/project.yaml", `schema_version: prifly-project-profile/2
`+projectHostsYAML+`packages: {sample: {source: .prifly/workflows/sample}}
launches:
  sample:
    title: Sample
    description: Questionnaire fixture.
    kind: workflow
    workflow: .prifly/workflows/sample/workflow.yaml
`)
	write(".prifly/workflows/sample/workflow.yaml", `authoring: prifly-project-workflow/1
package:
  id: test:package/sample
  version: 1.0.0
  description: Questionnaire fixture.
  requires_core_protocol: "1"
  profiles:
    default: fast
    values: {fast: {plan_capture: one}, full: {plan_capture: two}}
id: test:workflow/sample
version: 1.0.0
decision_catalog: [.prifly/workflows/sample/decisions/roadmap.yaml]
entry: done
stages: {done: {kind: finish, outcome: succeeded}}
limits: {max_step_instances: 1, max_control_transitions: 1}
policy_ref: {id: test:policy/local, version: 1.0.0, digest: sha256:0000000000000000000000000000000000000000000000000000000000000000}
`)
	write(".prifly/workflows/sample/decisions/roadmap.yaml", `authoring: prifly-run-decision/1
id: update_roadmap
title: Update roadmap
phase: preflight
choices: [{id: "yes", title: "Yes", value: true}, {id: "no", title: "No", value: false}]
recommendation: false
sensitivity: ordinary
destination: {kind: session_context, name: update_roadmap}
when: {profiles: [full]}
`)
	authority := filepath.Join(t.TempDir(), "never-opened-authority")
	var out, errout bytes.Buffer
	if code := execute(context.Background(), []string{"--project", authority, "project", "questionnaire", "--repository", repository, "--package", "sample", "--json"}, &out, &errout); code != 0 {
		t.Fatalf("questionnaire %d: %s", code, errout.String())
	}
	var result projectQuestionnaire
	if err := json.Unmarshal(out.Bytes(), &result); err != nil || result.SchemaVersion != "project-questionnaire/2" || result.Package.ID != "test:package/sample" || len(result.Profiles) != 2 || len(result.Preflight) != 1 || result.Preflight[0].When == nil || result.CatalogDigest == "" {
		t.Fatalf("questionnaire result: %v %#v", err, result)
	}
	if _, err := os.Stat(authority); !os.IsNotExist(err) {
		t.Fatalf("read-only questionnaire created authority state: %v", err)
	}
	out.Reset()
	errout.Reset()
	if code := execute(context.Background(), []string{"--project", authority, "project", "questionnaire", "--repository", repository, "--launch", "sample", "--json"}, &out, &errout); code != 0 {
		t.Fatalf("launch questionnaire %d: %s", code, errout.String())
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil || result.Package.ID != "test:package/sample" {
		t.Fatalf("launch questionnaire result: %v %#v", err, result)
	}
	if _, err := os.Stat(authority); !os.IsNotExist(err) {
		t.Fatalf("launch questionnaire created authority state: %v", err)
	}
}
