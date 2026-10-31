package main

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func writePointFixture(t *testing.T) (string, string) {
	t.Helper()
	root, authority := t.TempDir(), filepath.Join(t.TempDir(), "authority")
	if code, _, errout := runCLI(t, "project", "init", "--repository", root, "--state-root", authority); code != 0 {
		t.Fatalf("init: %s", errout)
	}
	source := filepath.Join("..", "..", "examples", "nested-extension-points")
	if err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		writeFixtureFile(t, root, filepath.Join(".prifly/workflows/transport", relative), string(data))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, root, ".prifly/project.yaml", "schema_version: prifly-project-profile/3\npackages:\n  transport: {source: .prifly/workflows/transport}\nlaunches: {}\n")
	return root, authority
}
func compilePointFixture(t *testing.T, root, authority string) (projectCompileResult, string, *prifly.Problem) {
	t.Helper()
	output := filepath.Join(t.TempDir(), "compiled")
	code, out, errout := runCLI(t, "--project", authority, "project", "compile", "--repository", root, "--package", "transport", "--output", output)
	if code != 0 {
		var p prifly.Problem
		if err := json.Unmarshal([]byte(errout), &p); err != nil {
			t.Fatalf("Problem: %s", errout)
		}
		if _, err := os.Stat(filepath.Join(output, "prifly.package.json")); err == nil {
			t.Fatal("refusal published partial package")
		}
		return projectCompileResult{}, output, &p
	}
	var compiled projectCompileResult
	if err := json.Unmarshal([]byte(out), &compiled); err != nil {
		t.Fatal(err)
	}
	return compiled, output, nil
}
func pointFixtureWorkflow(t *testing.T, c projectCompileResult, output, name string) flow.WorkflowRevision {
	t.Helper()
	for _, component := range c.Components {
		if component.Kind == "workflow" && strings.HasSuffix(component.Ref.ID, "/"+name) {
			data, err := os.ReadFile(filepath.Join(output, component.Path))
			if err != nil {
				t.Fatal(err)
			}
			var w flow.WorkflowRevision
			if err := json.Unmarshal(data, &w); err != nil {
				t.Fatal(err)
			}
			return w
		}
	}
	t.Fatalf("missing workflow %s", name)
	return flow.WorkflowRevision{}
}
func changePointFixture(t *testing.T, root, file, old, new string) {
	t.Helper()
	p := filepath.Join(root, ".prifly/workflows/transport", file)
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), old) {
		t.Fatalf("fixture missing %q", old)
	}
	if err := os.WriteFile(p, []byte(strings.Replace(string(data), old, new, 1)), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestProjectExtensionPointsSealNestedEntryAndRepeatExport(t *testing.T) {
	t.Parallel()
	root, authority := writePointFixture(t)
	compiled, output, p := compilePointFixture(t, root, authority)
	if p != nil {
		t.Fatalf("compile: %+v", p)
	}
	round := pointFixtureWorkflow(t, compiled, output, "round")
	parent := pointFixtureWorkflow(t, compiled, output, "transport")
	if round.Definition.Entry != "measure-before-read" {
		t.Fatalf("entry: %s", round.Definition.Entry)
	}
	if round.Definition.Stages["repair"].On["pass"] != "measure-after-repair" {
		t.Fatal("repair edge not spliced")
	}
	if got := round.Definition.Stages["read"].InputBindings["carrier"]; got.From != "stage_output" || got.StageID != "measure-before-read" || got.Port != "carrier" {
		t.Fatalf("reader got stale carrier: %+v", got)
	}
	if got := round.Definition.Stages["fixed"].OutputBindings["carrier"]; got.StageID != "measure-after-repair" {
		t.Fatalf("next iteration got stale carrier: %+v", got)
	}
	if parent.Definition.Stages["cycle"].BodyWorkflowRef.Digest != roundRef(compiled, "round").Digest {
		t.Fatal("parent sealed the old child ref")
	}
	if round.Definition.Stages["measure-after-repair"].Kind != "call" {
		t.Fatal("project call was not native")
	}
	found := false
	for _, source := range compiled.Sources {
		if strings.HasSuffix(source, "/extension-points.yaml") {
			found = true
		}
	}
	if !found {
		t.Fatal("point manifest missing from Sources")
	}
}
func roundRef(c projectCompileResult, name string) flow.Ref {
	for _, component := range c.Components {
		if component.Kind == "workflow" && strings.HasSuffix(component.Ref.ID, "/"+name) {
			return component.Ref
		}
	}
	return flow.Ref{}
}

func TestProjectExtensionPointsAbsentDisabledAndBuildIdentity(t *testing.T) {
	t.Parallel()
	root, authority := writePointFixture(t)
	writeFixtureFile(t, root, ".prifly/workflows/transport/extend.yaml", "authoring: prifly-extension/4\nextensions: []\n")
	absent, out, p := compilePointFixture(t, root, authority)
	if p != nil {
		t.Fatal(p)
	}
	w := pointFixtureWorkflow(t, absent, out, "round")
	if w.Definition.Entry != "read" || w.Definition.Stages["repair"].On["pass"] != "fixed" || w.Definition.Stages["read"].InputBindings["carrier"].From != "workflow_input" {
		t.Fatal("absent extension changed passthrough")
	}
	writeFixtureFile(t, root, ".prifly/workflows/transport/extend.yaml", "authoring: prifly-extension/4\nextensions:\n  - id: disabled\n    point: before-read\n    step: absent-project-component\n    enabled: false\n    output_bindings: {reader: carrier, repair: carrier, clean: carrier}\n")
	disabled, out, p := compilePointFixture(t, root, authority)
	if p != nil {
		t.Fatal(p)
	}
	w = pointFixtureWorkflow(t, disabled, out, "round")
	if w.Definition.Entry != "read" || w.Definition.Stages["repair"].On["pass"] != "fixed" {
		t.Fatal("disabled changed graph")
	}
	if absent.BuildKey == disabled.BuildKey {
		t.Fatal("disabled choice omitted from identity")
	}
	changePointFixture(t, root, "extension-points.yaml", "clean: {stage: clean, output: carrier}", "clean: {stage: clean, output: carrier}\n      blocked: {stage: blocked, output: carrier}")
	changePointFixture(t, root, "extend.yaml", "clean: carrier}", "clean: carrier, blocked: carrier}")
	changed, _, p := compilePointFixture(t, root, authority)
	if p != nil {
		t.Fatal(p)
	}
	if changed.BuildKey == disabled.BuildKey {
		t.Fatal("manifest omitted from identity")
	}
}

func TestProjectExtensionPointsRefuseUnsafeAuthoring(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, file, old, new, code string }{
		{"unknown point", "extend.yaml", "point: before-read", "point: unknown", "project_extension_point_unknown"},
		{"undeclared consumer", "extend.yaml", "reader: carrier", "other: carrier", "project_extension_point_binding_not_permitted"},
		{"missing consumer", "extend.yaml", "reader: carrier, repair: carrier, clean: carrier", "reader: carrier", "project_extension_point_binding_not_permitted"},
		{"arbitrary workflow", "extend.yaml", "point: before-read", "point: before-read\n    workflow: round", "project_extension_point_invalid"},
		{"package implementation", "extend.yaml", "step: measure", "step: read", "project_extension_point_component_not_permitted"},
		{"unknown component", "extend.yaml", "step: measure", "step: absent", "project_extension_point_unknown_component"},
		{"entry anchor", "extension-points.yaml", "entry: read", "entry: repair", "project_extension_point_unknown_anchor"},
		{"missing edge", "extension-points.yaml", "from: repair, to: fixed", "from: read, to: fixed", "project_extension_point_unknown_anchor"},
		{"ambiguous edge", "workflows/round.yaml", "needs_revision: blocked, blocked: blocked, fail: stopped", "needs_revision: fixed, blocked: blocked, fail: stopped", "project_extension_point_ambiguous"},
		{"unknown workflow", "extension-points.yaml", "report-probe:workflow/round", "report-probe:workflow/absent", "project_extension_point_unknown_workflow"},
		{"consumer not bound", "extension-points.yaml", "stage: clean, output: carrier", "stage: stopped, output: carrier", "project_extension_point_unknown_consumer"},
		{"unknown output", "extend.yaml", "reader: carrier", "reader: missing", "unknown_port"},
		{"unavailable producer", "extension-points.yaml", "$inputs.carrier", "$stages.repair.carrier", "unavailable_output"},
		{"missing required input", "extension-points.yaml", "input_bindings: {carrier: $inputs.carrier}", "input_bindings: {}", "missing_binding"},
		{"schema mismatch", "project/steps/measure.yaml", "carrier: {schema_ref: \"{{schema_carrier}}\", required_for: [pass]}", "carrier: {schema_ref: \"{{schema_report}}\", required_for: [pass]}", "port_type_mismatch"},
		{"control cycle", "extension-points.yaml", "succeeded: fixed, partial: stopped", "succeeded: repair, partial: fixed", "cycle"},
		{"call cycle", "project/workflows/measure-child.yaml", "step_ref: \"{{step_measure}}\"", "step_ref: \"{{workflow_round}}\"", "call_cycle"},
		{"unguaranteed call output", "extension-points.yaml", "succeeded: fixed, partial: stopped", "succeeded: fixed, partial: fixed", "unavailable_output"},
		{"unmapped child outcome", "extension-points.yaml", "succeeded: fixed, partial: stopped, rejected: stopped", "succeeded: fixed, rejected: stopped", "project_extension_call_outcome_unmapped"},
	} {
		if tc.code == "" {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root, authority := writePointFixture(t)
			changePointFixture(t, root, tc.file, tc.old, tc.new)
			_, _, p := compilePointFixture(t, root, authority)
			if p == nil {
				t.Fatalf("wanted %s", tc.code)
			}
			if p.Code != tc.code {
				found := false
				for _, v := range p.Violations {
					if strings.Contains(v.Reason, tc.code) {
						found = true
					}
				}
				if !found {
					t.Fatalf("want %s got %+v", tc.code, p)
				}
			}
		})
	}
}

func TestProjectExtensionPointManifestRejectsUnknownFields(t *testing.T) {
	t.Parallel()
	root, authority := writePointFixture(t)
	changePointFixture(t, root, "extension-points.yaml", "entry: read", "entry: read\n    arbitrary: true")
	_, _, p := compilePointFixture(t, root, authority)
	if p == nil || p.Code != "project_extension_point_invalid" {
		t.Fatalf("closed manifest: %+v", p)
	}
}

func TestProjectExtensionPointsSupportBothImplementationKinds(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"all steps", "all calls"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			root, authority := writePointFixture(t)
			if mode == "all steps" {
				changePointFixture(t, root, "extend.yaml", "call: measure-child", "step: measure")
			} else {
				changePointFixture(t, root, "extend.yaml", "step: measure", "call: measure-child")
			}
			c, out, p := compilePointFixture(t, root, authority)
			if p != nil {
				t.Fatal(p)
			}
			w := pointFixtureWorkflow(t, c, out, "round")
			wanted := "step"
			if mode == "all calls" {
				wanted = "call"
			}
			for _, id := range []string{"measure-before-read", "measure-after-repair"} {
				if w.Definition.Stages[id].Kind != wanted {
					t.Fatalf("%s: %s", id, w.Definition.Stages[id].Kind)
				}
			}
		})
	}
}

func TestProjectExtensionPointsRefuseConflictingAndAmbiguousDeclarations(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"workflow identity", "anchor", "consumer", "implementation identity"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			root, authority := writePointFixture(t)
			code := "project_extension_point_conflict"
			switch mode {
			case "workflow identity":
				p := filepath.Join(root, ".prifly/workflows/transport/workflows/round.yaml")
				data, err := os.ReadFile(p)
				if err != nil {
					t.Fatal(err)
				}
				writeFixtureFile(t, root, ".prifly/workflows/transport/workflows/duplicate.yaml", string(data))
				code = "project_extension_point_ambiguous"
			case "implementation identity":
				p := filepath.Join(root, ".prifly/workflows/transport/project/steps/measure.yaml")
				data, err := os.ReadFile(p)
				if err != nil {
					t.Fatal(err)
				}
				writeFixtureFile(t, root, ".prifly/workflows/transport/project/steps/duplicate.yaml", strings.Replace(string(data), "report-probe:step/measure", "other:step/measure", 1))
				code = "project_extension_point_ambiguous"
			case "anchor":
				changePointFixture(t, root, "extend.yaml", "point: after-repair\n    call: measure-child\n    output_bindings: {repeated: carrier}", "point: before-read\n    call: measure-child\n    output_bindings: {reader: carrier, repair: carrier, clean: carrier}")
			case "consumer":
				changePointFixture(t, root, "extension-points.yaml", "repeated: {stage: fixed, output: carrier}", "repeated: {stage: fixed, output: carrier}\n      duplicate: {stage: read, input: carrier}")
				changePointFixture(t, root, "extend.yaml", "repeated: carrier}", "repeated: carrier, duplicate: carrier}")
			}
			_, _, p := compilePointFixture(t, root, authority)
			if p == nil || p.Code != code {
				t.Fatalf("want %s got %+v", code, p)
			}
		})
	}
}

func TestProjectExtensionPointsProgramFilesAndResourceReviewAreSealed(t *testing.T) {
	t.Parallel()
	root, authority := writePointFixture(t)
	writeFixtureFile(t, root, ".prifly/workflows/transport/project/steps/measure.yaml", `authoring: prifly-step/1
id: report-probe:step/measure
version: 1.0.0
title: A declared project program
kind: worker
inputs: {carrier: {schema_ref: "{{schema_carrier}}"}}
outputs: {carrier: {schema_ref: "{{schema_carrier}}", required_for: [pass]}}
executor: {adapter_ref: "{{process}}", operation: process}
effects: {class: none, retry_class: pure}
result_schema_ref: "{{result}}"
`)
	p := filepath.Join(root, ".prifly/workflows/transport/extend.yaml")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, root, ".prifly/workflows/transport/extend.yaml", string(data)+`references:
  process: core:adapter/local-process@2.0.0
execution_bindings:
  steps:
    measure: {executable: shell, args: [worker.sh], files: {worker.sh: project/worker.sh}, timeout_ms: 30000, grace_ms: 100, max_output_bytes: 65536}
`)
	writeFixtureFile(t, root, ".prifly/workflows/transport/project/worker.sh", "exit 0\n")
	profile, err := readProjectProfile(root)
	if err != nil {
		t.Fatal(err)
	}
	first, err := (&cli{project: authority}).compileDeclaredProjectPackage(context.Background(), root, profile, "transport", "", "", filepath.Join(t.TempDir(), "compiled"))
	if err != nil {
		t.Fatal(err)
	}
	if !first.CallExtensions || first.ExecutionBindings == nil || len(first.ExecutionBindings.Bindings) != 1 {
		t.Fatal("point omitted reviewed execution contract")
	}
	found := false
	for _, source := range first.Sources {
		if strings.HasSuffix(source, "/project/worker.sh") {
			found = true
		}
	}
	if !found {
		t.Fatal("program source omitted from Sources")
	}
	writeFixtureFile(t, root, ".prifly/workflows/transport/project/worker.sh", "exit 1\n")
	second, _, problem := compilePointFixture(t, root, authority)
	if problem != nil {
		t.Fatal(problem)
	}
	if first.BuildKey == second.BuildKey {
		t.Fatal("program bytes omitted from build identity")
	}
	summary := projectLaunchSummary{}
	review1, err := projectCallExtensionReview(summary, first.CallExtensions, map[string][]string{"cycle": {"measurement"}}, map[string]int64{"measurement": 1})
	if err != nil {
		t.Fatal(err)
	}
	review2, err := projectCallExtensionReview(summary, first.CallExtensions, map[string][]string{"cycle": {"measurement"}}, map[string]int64{"measurement": 2})
	if err != nil {
		t.Fatal(err)
	}
	if review1 == review2 {
		t.Fatal("point resource limits omitted from review digest")
	}
}

func TestProjectExtensionPointsEditorContracts(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, file string
		valid      bool
	}{
		{"extension", "extend.yaml", true}, {"extension-v4", "extend.yaml", true}, {"extension-v2", "extend.yaml", false}, {"extension-v1", "extend.yaml", false}, {"extension-points-v1", "extension-points.yaml", true},
	} {
		t.Run(tc.name+tc.file, func(t *testing.T) {
			t.Parallel()
			source, err := os.ReadFile(filepath.Join("..", "..", "examples", "nested-extension-points", tc.file))
			if err != nil {
				t.Fatal(err)
			}
			value, err := flow.Parse(source, "yaml")
			if err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			err = validateAuthoringForTest(t, tc.name, data)
			if (err == nil) != tc.valid {
				t.Fatalf("accepted=%v want=%v: %v", err == nil, tc.valid, err)
			}
		})
	}
}

func TestProjectExtensionPointsOwnershipIsRelativeToPackageFolder(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"valid project directory", "package step remains forbidden"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			root, authority := writePointFixture(t)
			old := filepath.Join(root, ".prifly/workflows/transport")
			folder := filepath.Join(root, ".prifly/workflows/project")
			if err := os.Rename(old, folder); err != nil {
				t.Fatal(err)
			}
			writeFixtureFile(t, root, ".prifly/project.yaml", "schema_version: prifly-project-profile/3\npackages:\n  transport: {source: .prifly/workflows/project}\nlaunches: {}\n")
			if mode == "package step remains forbidden" {
				p := filepath.Join(folder, "extend.yaml")
				data, err := os.ReadFile(p)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(strings.Replace(string(data), "step: measure", "step: read", 1)), 0644); err != nil {
					t.Fatal(err)
				}
			}
			_, _, problem := compilePointFixture(t, root, authority)
			if mode == "valid project directory" {
				if problem != nil {
					t.Fatalf("valid package folder named project refused: %+v", problem)
				}
			} else if problem == nil || problem.Code != "project_extension_point_component_not_permitted" {
				t.Fatalf("package step was not protected: %+v", problem)
			}
		})
	}
}

func TestProjectExtensionPointsCycleIncludesLegacyRootCall(t *testing.T) {
	t.Parallel()
	root, authority := writePointFixture(t)
	rootPath := filepath.Join(root, ".prifly/workflows/transport/workflow.yaml")
	data, err := os.ReadFile(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	text = text[:strings.Index(text, "stages:")] + `stages:
  produce:
    kind: step
    step_ref: "{{step_produce}}"
    input_bindings: {}
    on: {pass: done, needs_revision: abandoned, blocked: abandoned, fail: abandoned, no_work: abandoned}
  done: {kind: finish, outcome: succeeded, output_bindings: {carrier: $stages.produce.carrier}}
  abandoned: {kind: finish, outcome: rejected}
`
	writeFixtureFile(t, root, ".prifly/workflows/transport/workflow.yaml", text)
	for _, name := range []string{"legacy-child", "measure-child"} {
		callee := "{{workflow_round}}"
		if name == "measure-child" {
			callee = "{{workflow_transport}}"
		}
		writeFixtureFile(t, root, ".prifly/workflows/transport/project/workflows/"+name+".yaml", `authoring: prifly-workflow/1
schema_version: "6"
id: report-probe:workflow/`+name+`
version: 1.0.0
inputs: {carrier: {schema_ref: "{{schema_carrier}}"}}
outputs: {carrier: {schema_ref: "{{schema_carrier}}", required_for: [succeeded]}}
allowed_outcomes: [succeeded, partial, rejected]
entry: call
policy_ref: "{{policy}}"
limits: {max_step_instances: 12, max_control_transitions: 32, max_child_depth: 8}
stages:
  call:
    kind: call
    workflow_ref: "`+callee+`"
    input_bindings: `+map[bool]string{true: "{}", false: "{carrier: $inputs.carrier}"}[name == "measure-child"]+`
    on: {succeeded: done, partial: failed, rejected: failed, no_work: failed}
  done: {kind: finish, outcome: succeeded, output_bindings: {carrier: $stages.call.carrier}}
  failed: {kind: finish, outcome: rejected}
`)
	}
	p := filepath.Join(root, ".prifly/workflows/transport/extend.yaml")
	data, err = os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, root, ".prifly/workflows/transport/extend.yaml", string(data)+`  - id: legacy-root
    workflow: transport
    call: legacy-child
    between: {from: produce, to: done}
    input_bindings: {carrier: $stages.produce.carrier}
    on: {succeeded: done, partial: abandoned, rejected: abandoned}
`)
	_, _, problem := compilePointFixture(t, root, authority)
	if problem == nil || problem.Code != "call_cycle" {
		t.Fatalf("compound legacy/point cycle: %+v", problem)
	}
}

func TestProjectExtensionPointsRejectNullAndDoubleDiscriminators(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, file, old, new string }{
		{"enabled null", "extend.yaml", "step: measure", "step: measure\n    enabled: null"},
		{"step null and call", "extend.yaml", "step: measure", "step: null\n    call: measure-child"},
		{"entry null and between", "extension-points.yaml", "between: {from: repair, to: fixed}", "entry: null\n    between: {from: repair, to: fixed}"},
		{"consumer null and output", "extension-points.yaml", "stage: clean, output: carrier", "stage: clean, input: null, output: carrier"},
		{"null on_error", "extension-points.yaml", "on_error: stopped", "on_error: null"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root, authority := writePointFixture(t)
			changePointFixture(t, root, tc.file, tc.old, tc.new)
			_, _, problem := compilePointFixture(t, root, authority)
			if problem == nil || problem.Code != "project_extension_point_invalid" {
				t.Fatalf("closed shape accepted null/double selector: %+v", problem)
			}
		})
	}
}
