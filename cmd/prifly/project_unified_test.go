package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func TestProjectUnifiedLocalPointsKeepCarrierContract(t *testing.T) {
	t.Parallel()
	root, authority := writePointFixture(t)
	changePointFixture(t, root, "extend.yaml", "prifly-extension/4", "prifly-extension/5")
	c, out, p := compilePointFixture(t, root, authority)
	if p != nil {
		t.Fatalf("unified: %+v", p)
	}
	if c.SchemaVersion != "project-compile/4" || !strings.HasPrefix(c.Package.Version, "0.0.0-b2.") {
		t.Fatalf("boundary: %+v", c)
	}
	w := pointFixtureWorkflow(t, c, out, "round")
	if w.Definition.Entry != "measure-before-read" {
		t.Fatalf("entry: %s", w.Definition.Entry)
	}
	b, e := os.ReadFile(filepath.Join(out, projectBuildFile))
	if e != nil {
		t.Fatal(e)
	}
	var build map[string]any
	if e = json.Unmarshal(b, &build); e != nil {
		t.Fatal(e)
	}
	if build["algorithm"] != "b2" || build["schema_version"] != "prifly-build-provenance/3" {
		t.Fatalf("build: %s", b)
	}
}

func TestProjectUnifiedOptInDoesNotChangeLegacyPoints(t *testing.T) {
	t.Parallel()
	root, authority := writePointFixture(t)
	a, _, p := compilePointFixture(t, root, authority)
	if p != nil {
		t.Fatal(p)
	}
	b, _, p := compilePointFixture(t, root, authority)
	if p != nil {
		t.Fatal(p)
	}
	if a.Package != b.Package || a.BuildKey != b.BuildKey || a.SchemaVersion != "project-compile/2" {
		t.Fatalf("legacy changed: %+v %+v", a, b)
	}
}

func TestProjectUnifiedPublishedFourBytes(t *testing.T) {
	t.Parallel()
	var baseline struct {
		Cases map[string]struct {
			SchemaVersion string            `json:"schema_version"`
			Package       json.RawMessage   `json:"package"`
			BuildKey      string            `json:"build_key"`
			Files         map[string]string `json:"files"`
		} `json:"cases"`
	}
	data, err := os.ReadFile("../../test/fixtures/unified-composition/published-four-baseline.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &baseline); err != nil {
		t.Fatal(err)
	}
	for name, want := range baseline.Cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root, authority := writePointFixture(t)
			if name == "absent" {
				writeFixtureFile(t, root, ".prifly/workflows/transport/extend.yaml", "authoring: prifly-extension/4\nextensions: []\n")
			}
			if name == "disabled" {
				path := filepath.Join(root, ".prifly/workflows/transport/extend.yaml")
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(path, []byte(strings.ReplaceAll(string(data), "    point:", "    enabled: false\n    point:")), 0644); err != nil {
					t.Fatal(err)
				}
			}
			c, out, p := compilePointFixture(t, root, authority)
			if p != nil {
				t.Fatal(p)
			}
			raw, _ := json.Marshal(c.Package)
			if !projectCompositionEqual(json.RawMessage(raw), want.Package) || c.BuildKey != want.BuildKey || c.SchemaVersion != want.SchemaVersion {
				t.Fatalf("published identity changed: %+v", c)
			}
			files := map[string]string{}
			err := filepath.WalkDir(out, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() {
					return nil
				}
				data, e := os.ReadFile(path)
				if e != nil {
					return e
				}
				relative, _ := filepath.Rel(out, path)
				files[filepath.ToSlash(relative)] = fmt.Sprintf("sha256:%x", sha256.Sum256(data))
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if !projectCompositionEqual(files, want.Files) {
				t.Fatalf("published package bytes changed: got %v want %v", files, want.Files)
			}
		})
	}
}

func unifiedRewriteFixture(t *testing.T, path string, update func(map[string]any)) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	v, err := flow.Parse(data, "yaml")
	if err != nil {
		t.Fatal(err)
	}
	update(v.(map[string]any))
	data, err = json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}
func writeUnifiedImportFixture(t *testing.T) (string, string) {
	t.Helper()
	root, authority := writePointFixture(t)
	consumer := filepath.Join(root, ".prifly/workflows/transport")
	provider := filepath.Join(root, ".prifly/workflows/provider")
	if err := os.CopyFS(provider, os.DirFS(consumer)); err != nil {
		t.Fatal(err)
	}
	for _, folder := range []string{consumer, provider} {
		err := filepath.WalkDir(folder, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".yaml") {
				return nil
			}
			data, e := os.ReadFile(path)
			if e != nil {
				return e
			}
			return os.WriteFile(path, []byte(strings.ReplaceAll(string(data), "{{schema_carrier}}", "{{any_carrier}}")), 0644)
		})
		if err != nil {
			t.Fatal(err)
		}
		unifiedRewriteFixture(t, filepath.Join(folder, "workflow.yaml"), func(v map[string]any) {
			v["authoring"] = projectUnifiedFolderVersion
			v["package"].(map[string]any)["references"].(map[string]any)["any_carrier"] = "core:schema/context-json@1.0.0"
		})
	}
	unifiedRewriteFixture(t, filepath.Join(provider, "workflow.yaml"), func(v map[string]any) {
		v["exports"] = map[string]any{"workflows": map[string]any{"review": "{{workflow_round}}", "other": "{{workflow_round}}"}, "points": map[string]any{"before-read": map[string]any{"workflow": "review", "point": "before-read"}, "other-hook": map[string]any{"workflow": "other", "point": "before-read"}}}
	})
	unifiedRewriteFixture(t, filepath.Join(consumer, "workflow.yaml"), func(v map[string]any) {
		v["refs"] = map[string]any{"first": "a.review", "second": "b.review"}
		v["inputs"] = map[string]any{"carrier": map[string]any{"schema_ref": "{{any_carrier}}", "required": false, "configuration": map[string]any{"scope": "project", "default": map[string]any{"candidate": "original", "reports": []any{}}}}}
		v["outputs"] = map[string]any{}
		v["entry"] = "first"
		delete(v, "resumable")
		call := func(ref, to string) any {
			return map[string]any{"kind": "call", "workflow_ref": ref, "input_bindings": map[string]any{"carrier": "$inputs.carrier"}, "on": map[string]any{"succeeded": to, "partial": to, "rejected": to, "no_work": to}}
		}
		v["stages"] = map[string]any{"first": call("first", "second"), "second": call("second", "done"), "done": map[string]any{"kind": "finish", "outcome": "succeeded"}}
	})
	writeFixtureFile(t, root, ".prifly/project.yaml", "schema_version: prifly-project-profile/3\npackages:\n  transport: {source: .prifly/workflows/transport}\n  provider: {source: .prifly/workflows/provider}\nlaunches: {}\n")
	writeFixtureFile(t, root, ".prifly/workflows/transport/extend.yaml", `authoring: prifly-extension/5
imports: {a: provider, b: provider}
extensions:
  - id: measure-before-read
    point: {import: a, export: before-read}
    step: measure
    output_bindings: {reader: carrier, repair: carrier, clean: carrier}
`)
	return root, authority
}
func TestProjectUnifiedImportedPointVariantsAreIsolated(t *testing.T) {
	t.Parallel()
	root, authority := writeUnifiedImportFixture(t)
	c, out, p := compilePointFixture(t, root, authority)
	if p != nil {
		t.Fatalf("unified imports: %+v", p)
	}
	entries := map[string]int{}
	refs := map[string]bool{}
	for _, component := range c.Components {
		if component.Ref.ID == "report-probe:workflow/round" {
			data, err := os.ReadFile(filepath.Join(out, component.Path))
			if err != nil {
				t.Fatal(err)
			}
			var w flow.WorkflowRevision
			if err = json.Unmarshal(data, &w); err != nil {
				t.Fatal(err)
			}
			entries[w.Definition.Entry]++
			refs[component.Ref.String()] = true
		}
	}
	if entries["read"] != 1 || entries["measure-before-read"] != 1 || len(refs) != 2 {
		t.Fatalf("variants: %v %v", entries, refs)
	}
	again, _, p := compilePointFixture(t, root, authority)
	if p != nil || again.Package != c.Package {
		t.Fatalf("nondeterministic: %+v", p)
	}
}
func TestProjectUnifiedImportScopesDeduplicateEqualPolicy(t *testing.T) {
	t.Parallel()
	root, authority := writeUnifiedImportFixture(t)
	writeFixtureFile(t, root, ".prifly/workflows/transport/extend.yaml", "authoring: prifly-extension/5\nimports: {a: provider, b: provider}\nextensions: []\n")
	c, _, p := compilePointFixture(t, root, authority)
	if p != nil {
		t.Fatal(p)
	}
	count := 0
	for _, component := range c.Components {
		if component.Ref.ID == "report-probe:workflow/round" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("identical provider policy emitted %d variants", count)
	}
}
func TestProjectUnifiedPointExportDoesNotOpenPrivatePoints(t *testing.T) {
	t.Parallel()
	root, authority := writeUnifiedImportFixture(t)
	changePointFixture(t, root, "extend.yaml", "export: before-read", "export: after-repair")
	_, _, p := compilePointFixture(t, root, authority)
	if p == nil || p.Code != "project_composition_point_not_exported" {
		t.Fatalf("private point: %+v", p)
	}
}

func TestProjectUnifiedProtectedContracts(t *testing.T) {
	t.Parallel()
	var manifest struct {
		Files []struct {
			Path   string `json:"path"`
			SHA256 string `json:"sha256"`
			Mode   string `json:"mode"`
		} `json:"files"`
	}
	data, err := os.ReadFile("../../test/fixtures/unified-composition/protected-files.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, file := range manifest.Files {
		info, err := os.Stat(filepath.Join("../..", file.Path))
		if err != nil {
			t.Fatal(err)
		}
		mode := "100644"
		if info.Mode().Perm()&0111 != 0 {
			mode = "100755"
		}
		if mode != file.Mode {
			t.Errorf("protected mode changed: %s: %s != %s", file.Path, mode, file.Mode)
		}
		data, err := os.ReadFile(filepath.Join("../..", file.Path))
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != file.SHA256 {
			t.Errorf("published/history bytes changed: %s", file.Path)
		}
	}
}

func TestProjectUnifiedInheritedFourKeepsImplementationOwner(t *testing.T) {
	t.Parallel()
	root, authority := writePointFixture(t)
	writeFixtureFile(t, root, ".prifly/workflows/derived/workflow.yaml", `authoring: prifly-project-workflow/3
package: {id: child:package/derived, version: 1.0.0, description: Derived point contract, requires_core_protocol: "1", references: {}}
id: child:workflow/derived
version: 1.0.0
extends: transport
`)
	writeFixtureFile(t, root, ".prifly/workflows/derived/extend.yaml", "authoring: prifly-extension/5\nextensions: []\n")
	writeFixtureFile(t, root, ".prifly/project.yaml", "schema_version: prifly-project-profile/3\npackages:\n  transport: {source: .prifly/workflows/transport}\n  derived: {source: .prifly/workflows/derived}\nlaunches: {}\n")
	out := filepath.Join(t.TempDir(), "compiled")
	code, stdout, stderr := runCLI(t, "--project", authority, "project", "compile", "--repository", root, "--package", "derived", "--output", out)
	if code != 0 {
		t.Fatal(stderr)
	}
	var c projectCompileResult
	if err := json.Unmarshal([]byte(stdout), &c); err != nil {
		t.Fatal(err)
	}
	w := pointFixtureWorkflow(t, c, out, "round")
	if w.Definition.Entry != "measure-before-read" {
		t.Fatalf("lost parent point: %+v", w.Definition)
	}
	data, err := os.ReadFile(filepath.Join(out, projectBuildFile))
	if err != nil {
		t.Fatal(err)
	}
	var build projectBuildProvenance
	if err = json.Unmarshal(data, &build); err != nil {
		t.Fatal(err)
	}
	for _, m := range build.Components {
		if m.AuthorRef.ID == "report-probe:step/measure" && m.SourceOwner != ".prifly/workflows/transport" {
			t.Fatalf("implementation owner changed: %+v", m)
		}
	}
}

func TestProjectUnifiedPointGroupsUseNativeOperators(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"sequence", "parallel", "choice"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			root, authority := writePointFixture(t)
			unifiedRewriteFixture(t, filepath.Join(root, ".prifly/workflows/transport/extension-points.yaml"), func(v map[string]any) {
				points := v["points"].(map[string]any)
				point := points["before-read"].(map[string]any)
				point["consumers"] = map[string]any{}
				point["routes"].(map[string]any)["step"].(map[string]any)["on_error"] = "stopped"
			})
			for _, name := range []string{"workflow.yaml", "workflows/round.yaml"} {
				unifiedRewriteFixture(t, filepath.Join(root, ".prifly/workflows/transport", name), func(v map[string]any) {
					v["limits"] = map[string]any{"max_step_instances": 64, "max_control_transitions": 256, "max_child_depth": 8, "max_parallelism": 4}
				})
			}
			group := mode + ": [transport.first, transport.second]"
			if mode == "choice" {
				group = "choice:\n      selection: first_match\n      branches: [{predicate: true, use: transport.first}]\n      default: transport.second\n      on_unknown: rejected"
			}
			writeFixtureFile(t, root, ".prifly/workflows/transport/extend.yaml", `authoring: prifly-extension/5
extensions:
  - {id: first, point: before-read, step: measure, output_bindings: {}}
  - {id: second, point: before-read, step: measure, output_bindings: {}}
arrange:
  - id: combined
    point: before-read
    `+group+"\n")
			c, out, p := compilePointFixture(t, root, authority)
			if p != nil {
				t.Fatalf("group %s: %+v", mode, p)
			}
			w := pointFixtureWorkflow(t, c, out, "round")
			stage := w.Definition.Stages["combined"]
			if stage.Kind != "call" || w.Definition.Entry != "combined" || stage.OnError != "stopped" {
				t.Fatalf("point routes: %+v", w.Definition)
			}
			found := mode == "sequence"
			for _, component := range c.Components {
				if component.Kind == "workflow" {
					data, err := os.ReadFile(filepath.Join(out, component.Path))
					if err != nil {
						t.Fatal(err)
					}
					var w flow.WorkflowRevision
					if err = json.Unmarshal(data, &w); err != nil {
						t.Fatal(err)
					}
					for _, s := range w.Definition.Stages {
						if s.Kind == mode {
							found = true
						}
					}
				}
			}
			if !found {
				t.Fatalf("native %s absent", mode)
			}
		})
	}
}
func TestProjectUnifiedPointGroupRejectsConsumerTransport(t *testing.T) {
	t.Parallel()
	root, authority := writePointFixture(t)
	writeFixtureFile(t, root, ".prifly/workflows/transport/extend.yaml", `authoring: prifly-extension/5
extensions:
  - {id: first, point: before-read, step: measure, output_bindings: {reader: carrier, repair: carrier, clean: carrier}}
arrange:
  - {id: combined, point: before-read, sequence: [transport.first]}
`)
	_, _, p := compilePointFixture(t, root, authority)
	if p == nil || p.Code != "project_composition_point_group_outputs_unsupported" {
		t.Fatalf("consumer group: %+v", p)
	}
}

func TestProjectUnifiedProgramUsesSelectedOwnerAndPinsFiles(t *testing.T) {
	t.Parallel()
	root, authority := writeUnifiedImportFixture(t)
	folder := filepath.Join(root, ".prifly/workflows/provider")
	unifiedRewriteFixture(t, filepath.Join(folder, "workflow.yaml"), func(v map[string]any) {
		v["package"].(map[string]any)["references"].(map[string]any)["process"] = "core:adapter/local-process@2.0.0"
		v["execution_bindings"] = map[string]any{"steps": map[string]any{"report-probe:step/read": map[string]any{"executable": "shell", "args": []any{"worker.sh"}, "files": map[string]any{"worker.sh": "worker.sh"}, "timeout_ms": 30000, "grace_ms": 100, "max_output_bytes": 65536}}}
	})
	unifiedRewriteFixture(t, filepath.Join(folder, "steps/read.yaml"), func(v map[string]any) {
		v["authoring"] = "prifly-step/1"
		delete(v, "repository_workspace")
		v["executor"] = map[string]any{"adapter_ref": "{{process}}", "operation": "process"}
	})
	writeFixtureFile(t, root, ".prifly/workflows/provider/worker.sh", "exit 0\n")
	first, out, p := compilePointFixture(t, root, authority)
	if p != nil {
		t.Fatalf("program: %+v", p)
	}
	data, err := os.ReadFile(filepath.Join(out, projectExecutionFile))
	if err != nil {
		t.Fatal(err)
	}
	payload, err := projectDecodeExecution(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload.Bindings) != 1 || string(payload.Bindings[0].Files["worker.sh"]) != "exit 0\n" {
		t.Fatalf("program variants: %+v", payload)
	}
	matches := 0
	for _, c := range first.Components {
		if c.Ref == payload.Bindings[0].DefinitionRef {
			matches++
		}
	}
	if matches != 1 {
		t.Fatal("program does not name an exact selected variant")
	}
	writeFixtureFile(t, root, ".prifly/workflows/provider/worker.sh", "exit 1\n")
	second, _, p := compilePointFixture(t, root, authority)
	if p != nil || second.BuildKey == first.BuildKey {
		t.Fatalf("supporting bytes not pinned: %+v", p)
	}
}

func TestProjectUnifiedBuildIdentityIgnoresMachineRoot(t *testing.T) {
	t.Parallel()
	a, aa := writeUnifiedImportFixture(t)
	b, ba := writeUnifiedImportFixture(t)
	first, _, p := compilePointFixture(t, a, aa)
	if p != nil {
		t.Fatal(p)
	}
	second, _, p := compilePointFixture(t, b, ba)
	if p != nil {
		t.Fatal(p)
	}
	if first.Package != second.Package || first.BuildKey != second.BuildKey {
		t.Fatalf("machine root changed identities: %s %s", first.Package, second.Package)
	}
}

func TestProjectUnifiedExportsCanSharePrivateChild(t *testing.T) {
	t.Parallel()
	root, authority := writeUnifiedImportFixture(t)
	folder := filepath.Join(root, ".prifly/workflows/provider")
	for _, name := range []string{"public-one", "public-two"} {
		writeFixtureFile(t, root, ".prifly/workflows/provider/workflows/"+name+".yaml", `authoring: prifly-workflow/1
schema_version: "8"
id: report-probe:workflow/`+name+`
version: 1.0.0
title: Public wrapper
inputs: {carrier: {schema_ref: "{{any_carrier}}"}}
outputs: {}
allowed_outcomes: [succeeded]
entry: run
policy_ref: "{{policy}}"
limits: {max_step_instances: 32, max_control_transitions: 128, max_child_depth: 8}
stages:
  run:
    kind: call
    workflow_ref: "{{workflow_round}}"
    input_bindings: {carrier: $inputs.carrier}
    on: {succeeded: done, partial: done, rejected: done, no_work: done}
  done: {kind: finish, outcome: succeeded}
`)
	}
	unifiedRewriteFixture(t, filepath.Join(folder, "workflow.yaml"), func(v map[string]any) {
		v["exports"].(map[string]any)["workflows"] = map[string]any{"review": "{{workflow_public-one}}", "other": "{{workflow_public-two}}"}
	})
	unifiedRewriteFixture(t, filepath.Join(root, ".prifly/workflows/transport/workflow.yaml"), func(v map[string]any) {
		v["refs"].(map[string]any)["second"] = "a.other"
		v["limits"] = map[string]any{"max_step_instances": 64, "max_control_transitions": 256, "max_child_depth": 8}
		for _, name := range []string{"first", "second"} {
			stage := v["stages"].(map[string]any)[name].(map[string]any)
			on := stage["on"].(map[string]any)
			delete(on, "partial")
			delete(on, "rejected")
			delete(on, "no_work")
		}
	})
	c, out, p := compilePointFixture(t, root, authority)
	if p != nil {
		t.Fatal(p)
	}
	entries := map[string]int{}
	for _, component := range c.Components {
		if component.Ref.ID == "report-probe:workflow/round" {
			data, err := os.ReadFile(filepath.Join(out, component.Path))
			if err != nil {
				t.Fatal(err)
			}
			var w flow.WorkflowRevision
			if err = json.Unmarshal(data, &w); err != nil {
				t.Fatal(err)
			}
			entries[w.Definition.Entry]++
		}
	}
	if entries["read"] != 1 || entries["measure-before-read"] != 1 {
		t.Fatalf("shared child policies leaked: %v", entries)
	}
}

func validateUnifiedView(t *testing.T, name string, data []byte) {
	t.Helper()
	schema, err := prifly.PublicSchema(name)
	if err != nil {
		t.Fatal(err)
	}
	schema, err = flow.Canonical(schema)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := flow.Digest(schema)
	if err != nil {
		t.Fatal(err)
	}
	ref := flow.Ref{ID: "test:schema/unified-view", Version: "1.0.0", Digest: digest}
	if err = flow.ValidateSchema(flow.Registry{ref: schema}, ref, data); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
}
func TestProjectUnifiedEmitsClosedCompileAndLaunchViews(t *testing.T) {
	t.Parallel()
	root, authority := writeUnifiedImportFixture(t)
	unifiedRewriteFixture(t, filepath.Join(root, ".prifly/project.yaml"), func(v map[string]any) {
		v["hosts"] = map[string]any{"codex-app": ".agents/skills"}
		v["launches"] = map[string]any{"transport": map[string]any{"title": "Unified transport", "description": "Closed view fixture", "kind": "workflow", "workflow": ".prifly/workflows/transport/workflow.yaml"}}
	})
	writeFixtureFile(t, root, ".agents/skills/.keep", "")
	writeFixtureFile(t, root, "metadata.txt", "original\n")
	gitFixture(t, root, "init", "-q")
	gitFixture(t, root, "add", ".")
	gitFixture(t, root, "commit", "-qm", "Unified fixture")
	out := filepath.Join(t.TempDir(), "compiled")
	code, stdout, stderr := runCLI(t, "--project", authority, "project", "compile", "--repository", root, "--package", "transport", "--output", out, "--explain")
	if code != 0 {
		t.Fatal(stderr)
	}
	validateUnifiedView(t, "ProjectCompileV4", []byte(stdout))
	data, err := os.ReadFile(filepath.Join(out, projectBuildFile))
	if err != nil {
		t.Fatal(err)
	}
	validateUnifiedView(t, "BuildProvenanceV3", data)
	args := []string{"--project", authority, "project", "questionnaire", "--repository", root, "--launch", "transport", "--host", "codex-app", "--workspace", "worktree", "--prepare"}
	code, stdout, stderr = runCLI(t, args...)
	if code != 0 {
		t.Fatal(stderr)
	}
	validateUnifiedView(t, "ProjectLaunchSummaryV7", []byte(stdout))
	var summary projectLaunchSummary
	if err = json.Unmarshal([]byte(stdout), &summary); err != nil {
		t.Fatal(err)
	}
	args = []string{"--project", authority, "project", "start", "--repository", root, "--launch", "transport", "--host", "codex-app", "--workspace", "worktree", "--expected-launch-digest", summary.ReviewDigest}
	code, stdout, stderr = runCLI(t, args...)
	if code != 0 {
		t.Fatal(stderr)
	}
	validateUnifiedView(t, "ProjectStartV7", []byte(stdout))
	var start projectStartResult
	if err = json.Unmarshal([]byte(stdout), &start); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runCLI(t, "--project", authority, "run", "cancel", start.Run.Run.ID, "--reason", "Close emitted-schema fixture"); code != 0 {
		t.Fatal(stderr)
	}
}

func TestProjectUnifiedImportedSettingsStayOnExport(t *testing.T) {
	t.Parallel()
	root, authority := writeUnifiedImportFixture(t)
	unifiedRewriteFixture(t, filepath.Join(root, ".prifly/workflows/provider/workflows/round.yaml"), func(v map[string]any) {
		v["inputs"].(map[string]any)["mode"] = map[string]any{"schema_ref": "{{any_carrier}}", "required": false, "configuration": map[string]any{"scope": "project", "default": "original"}}
	})
	writeFixtureFile(t, root, ".prifly/workflows/transport/extend.yaml", "authoring: prifly-extension/5\nimports: {a: provider, b: provider}\nsettings: {a.review: {mode: configured}}\nextensions: []\n")
	c, out, p := compilePointFixture(t, root, authority)
	if p != nil {
		t.Fatal(p)
	}
	defaults := map[string]int{}
	for _, component := range c.Components {
		if component.Ref.ID == "report-probe:workflow/round" {
			data, err := os.ReadFile(filepath.Join(out, component.Path))
			if err != nil {
				t.Fatal(err)
			}
			var value map[string]any
			if err = json.Unmarshal(data, &value); err != nil {
				t.Fatal(err)
			}
			port := value["inputs"].(map[string]any)["mode"].(map[string]any)
			configuration := port["configuration"].(map[string]any)
			defaults[configuration["default"].(string)]++
		}
	}
	if defaults["original"] != 1 || defaults["configured"] != 1 {
		t.Fatalf("settings scopes: %v", defaults)
	}
}

func TestProjectUnifiedProvenanceRejectsSwappedOwnerAndVariant(t *testing.T) {
	t.Parallel()
	root, authority := writeUnifiedImportFixture(t)
	profile, err := readProjectProfile(root)
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "compiled")
	c, err := (&cli{project: authority}).compileDeclaredProjectPackage(context.Background(), root, profile, "transport", "", "", output)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(output, projectBuildFile))
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"source_owner", "variant_key", "compiled_ref", "build_inputs"} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			var build map[string]any
			if err := json.Unmarshal(raw, &build); err != nil {
				t.Fatal(err)
			}
			entries := build["components"].([]any)
			first := entries[0].(map[string]any)
			switch field {
			case "source_owner":
				first[field] = ".prifly/workflows/another-owner"
			case "variant_key":
				first[field] = "sha256:" + strings.Repeat("0", 64)
			case "compiled_ref":
				first[field] = entries[1].(map[string]any)[field]
			case "build_inputs":
				build[field].(map[string]any)["variants"] = []any{}
			}
			data, err := json.Marshal(build)
			if err != nil {
				t.Fatal(err)
			}
			if err = validateProjectBuild(data, c.Components); err == nil {
				t.Fatal("tampered provenance accepted")
			}
		})
	}
}

func TestProjectUnifiedVariantBudgetIsGlobal(t *testing.T) {
	t.Parallel()
	for _, children := range []int{255, 256} {
		t.Run(fmt.Sprint(children), func(t *testing.T) {
			t.Parallel()
			root, authority := writeUnifiedImportFixture(t)
			provider := filepath.Join(root, ".prifly/workflows/provider")
			if err := os.Remove(filepath.Join(provider, projectExtensionPointsFile)); err != nil {
				t.Fatal(err)
			}
			unifiedRewriteFixture(t, filepath.Join(provider, "workflow.yaml"), func(v map[string]any) { delete(v["exports"].(map[string]any), "points") })
			unifiedRewriteFixture(t, filepath.Join(provider, "workflows/round.yaml"), func(v map[string]any) {
				v["inputs"].(map[string]any)["mode"] = map[string]any{"schema_ref": "{{any_carrier}}", "required": false, "configuration": map[string]any{"scope": "project", "default": "default"}}
				v["outputs"] = map[string]any{}
				v["entry"] = "done"
				v["stages"] = map[string]any{"done": map[string]any{"kind": "finish", "outcome": "succeeded"}}
				v["allowed_outcomes"] = []any{"succeeded"}
			})
			imports, settings, refs, stages := map[string]any{}, map[string]any{}, map[string]any{}, map[string]any{}
			for i := 0; i < children; i++ {
				name := fmt.Sprintf("a%03d", i)
				imports[name] = "provider"
				settings[name+".review"] = map[string]any{"mode": name}
				refs[name] = name + ".review"
				next := "done"
				if i+1 < children {
					next = fmt.Sprintf("a%03d", i+1)
				}
				stages[name] = map[string]any{"kind": "call", "workflow_ref": name, "input_bindings": map[string]any{"carrier": "$inputs.carrier"}, "on": map[string]any{"succeeded": next}}
			}
			stages["done"] = map[string]any{"kind": "finish", "outcome": "succeeded"}
			unifiedRewriteFixture(t, filepath.Join(root, ".prifly/workflows/transport/workflow.yaml"), func(v map[string]any) {
				v["refs"] = refs
				v["entry"] = "a000"
				v["stages"] = stages
				v["limits"] = map[string]any{"max_step_instances": 64, "max_control_transitions": 2048, "max_child_depth": 8}
			})
			ext, _ := json.Marshal(map[string]any{"authoring": projectUnifiedExtensionVersion, "imports": imports, "settings": settings, "extensions": []any{}})
			writeFixtureFile(t, root, ".prifly/workflows/transport/extend.yaml", string(ext))
			c, _, p := compilePointFixture(t, root, authority)
			if children == 256 {
				if p == nil || p.Code != "project_composition_variant_limit" {
					t.Fatalf("257 variants: %+v", p)
				}
				return
			}
			if p != nil {
				t.Fatalf("256 variants: %+v", p)
			}
			count := 0
			for _, component := range c.Components {
				if component.Kind == "workflow" {
					count++
				}
			}
			if count != 256 {
				t.Fatalf("variant count %d", count)
			}
		})
	}
}

func TestProjectUnifiedLegacyFourRefusalBaseline(t *testing.T) {
	t.Parallel()
	var baseline struct {
		Refusals map[string]struct {
			Old        string `json:"old"`
			New        string `json:"new"`
			ExitCode   int    `json:"exit_code"`
			Code       string `json:"code"`
			Violations any    `json:"violations"`
		} `json:"refusals"`
	}
	data, err := os.ReadFile("../../test/fixtures/unified-composition/published-four-baseline.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &baseline); err != nil {
		t.Fatal(err)
	}
	for name, want := range baseline.Refusals {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root, authority := writePointFixture(t)
			changePointFixture(t, root, "extend.yaml", want.Old, want.New)
			_, _, p := compilePointFixture(t, root, authority)
			if p == nil || p.Code != want.Code || !projectCompositionEqual(p.Violations, want.Violations) {
				t.Fatalf("published refusal changed: %+v want %+v", p, want)
			}
		})
	}
}

func TestProjectUnifiedChoicePredicateLift(t *testing.T) {
	t.Parallel()
	ref := flow.Ref{ID: "test:step/produce", Version: "1.0.0", Digest: "sha256:" + strings.Repeat("1", 64)}
	step := flow.StepDefinition{Outputs: map[string]flow.OutputPort{"carrier": {Port: flow.Port{Format: "json"}}}}
	bytes, err := json.Marshal(step)
	if err != nil {
		t.Fatal(err)
	}
	components := []projectCompileComponent{{Kind: "step", Ref: ref, Bytes: bytes}}
	root := flow.WorkflowRevision{ID: "test:workflow/root"}
	root.Definition.Stages = map[string]flow.Stage{"produce": {Kind: "step", StepRef: ref}}
	lowerer := projectCompositionLowerer{root: root, components: &components, external: map[string]flow.Binding{}, ports: map[string]flow.InputPort{}}
	predicate, err := lowerer.predicate(map[string]any{"op": "eq", "left": "$stages.produce.carrier#/missing", "right": true})
	if err != nil {
		t.Fatal(err)
	}
	if predicate.Left.Kind != "field" || predicate.Left.Ref.From != "workflow_input" || predicate.Left.Ref.Pointer != "/missing" || predicate.Left.Ref.StageID != "" {
		t.Fatalf("stage field was not fully lifted: %+v", predicate.Left.Ref)
	}
}

func TestProjectUnifiedB2FormulaVectors(t *testing.T) {
	t.Parallel()
	var fixture struct {
		Vectors []struct {
			Name             string
			VariantInputs    map[string]any `json:"variant_inputs"`
			VariantKey       string         `json:"variant_key"`
			BuildInputs      map[string]any `json:"build_inputs"`
			BuildKey         string         `json:"build_key"`
			PackageVersion   string         `json:"package_version"`
			ComponentVersion string         `json:"component_version"`
		}
	}
	data, err := os.ReadFile("../../test/fixtures/unified-composition/b2-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, vector := range fixture.Vectors {
		t.Run(vector.Name, func(t *testing.T) {
			t.Parallel()
			if key := projectUnifiedHash(vector.VariantInputs); key != vector.VariantKey {
				t.Fatalf("variant %s != %s", key, vector.VariantKey)
			}
			if key := projectUnifiedHash(vector.BuildInputs); key != vector.BuildKey {
				t.Fatalf("build %s != %s", key, vector.BuildKey)
			}
			version, err := projectUnifiedPackageVersion(vector.BuildKey)
			if err != nil || version != vector.PackageVersion {
				t.Fatalf("package %s: %v", version, err)
			}
			author := flow.Ref{ID: vector.VariantInputs["author_id"].(string), Version: vector.VariantInputs["author_version"].(string)}
			version, err = projectUnifiedComponentVersion(vector.BuildKey, "workflow", author, vector.VariantKey)
			if err != nil || version != vector.ComponentVersion {
				t.Fatalf("component %s: %v", version, err)
			}
			reordered := map[string]any{}
			for key, value := range vector.VariantInputs {
				reordered[key] = value
			}
			if projectUnifiedHash(reordered) != vector.VariantKey {
				t.Fatal("map traversal changed identity")
			}
		})
	}
}

func TestProjectUnifiedSelectorsAndExportsRefuseForeignAuthority(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, old, new, code string }{
		{"unknown-import", "import: a", "import: missing", "project_composition_unknown_import"},
		{"unknown-parent", "{import: a, export: before-read}", "{parent: missing, name: before-read}", "project_composition_unknown_parent"},
		{"private-implementation", "step: measure", "step: read", "project_extension_point_component_not_permitted"},
		{"wrong-kind", "step: measure", "step: a.review", "project_composition_wrong_kind"},
		{"qualified-four", "authoring: prifly-extension/5", "authoring: prifly-extension/4", "project_extension_point_invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root, authority := writeUnifiedImportFixture(t)
			changePointFixture(t, root, "extend.yaml", tc.old, tc.new)
			_, _, p := compilePointFixture(t, root, authority)
			if p == nil || p.Code != tc.code {
				t.Fatalf("got %+v; want %s", p, tc.code)
			}
		})
	}
}

func TestProjectUnifiedAuthoringBoundariesAreClosed(t *testing.T) {
	t.Parallel()
	point := map[string]any{"id": "hook", "point": map[string]any{"import": "ui", "export": "before-read"}, "call": "ui.review", "output_bindings": map[string]any{}}
	doc := map[string]any{"authoring": projectUnifiedExtensionVersion, "imports": map[string]any{"ui": "ui"}, "extensions": []any{point}}
	if err := projectValidatePointShape("extension-v5", doc); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"input_bindings", "on", "consumers", "workflow", "extra"} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			value := projectCompositionClone(doc).(map[string]any)
			value["extensions"].([]any)[0].(map[string]any)[field] = map[string]any{}
			if err := projectValidatePointShape("extension-v5", value); err == nil {
				t.Fatal("client overrode point contract", field)
			}
		})
	}
	legacy := projectCompositionClone(doc).(map[string]any)
	legacy["authoring"] = projectPointExtensionVersion
	if err := projectValidatePointShape("extension-v4", legacy); err == nil {
		t.Fatal("/4 accepted /5 selectors")
	}
}

func TestProjectUnifiedRejectsDefinitionCycleBeforeCompile(t *testing.T) {
	t.Parallel()
	root, authority := writeUnifiedImportFixture(t)
	path := filepath.Join(root, ".prifly/workflows/provider/workflows/round.yaml")
	unifiedRewriteFixture(t, path, func(v map[string]any) {
		v["entry"] = "recursive"
		v["stages"].(map[string]any)["recursive"] = map[string]any{"kind": "call", "workflow_ref": "{{workflow_round}}", "input_bindings": map[string]any{"carrier": "$inputs.carrier"}, "on": map[string]any{"succeeded": "clean", "partial": "clean", "rejected": "clean", "no_work": "clean"}}
	})
	unifiedRewriteFixture(t, filepath.Join(root, ".prifly/workflows/provider/extension-points.yaml"), func(v map[string]any) {
		v["points"].(map[string]any)["before-read"].(map[string]any)["entry"] = "recursive"
	})
	_, _, p := compilePointFixture(t, root, authority)
	if p == nil || p.Code != "project_compile_dependency_cycle" {
		t.Fatalf("cycle: %+v", p)
	}
}

func TestProjectUnifiedLegacyCompositionDiamond(t *testing.T) {
	t.Parallel()
	root, authority := writeCompositionFixture(t)
	if err := os.Remove(filepath.Join(root, ".prifly/workflows/cycle/extend.yaml")); err != nil {
		t.Fatal(err)
	}
	registerCompositionDerived(t, root, "classic-ui", "cycle", strings.Replace(compositionImportExtension, "interface-review", "interface-left", 1))
	registerCompositionDerived(t, root, "classic-audit", "cycle", strings.Replace(compositionImportExtension, "interface-review", "interface-right", 1))
	registerCompositionDerived(t, root, "delivery", "[classic-ui, classic-audit]", `authoring: prifly-extension/5
arrange:
  - id: extra-checks
    between: {from: build, to: commit}
    parallel: [classic-ui.interface-left, classic-audit.interface-right]
    on: {succeeded: commit, rejected: rejected}
`)
	// Independently owned schemas have separate b2 identities; the port contract
	// deliberately pins a shared external schema instead.
	err := filepath.WalkDir(filepath.Join(root, ".prifly/workflows"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err = os.WriteFile(path, []byte(strings.ReplaceAll(string(data), "{{schema_handoff}}", "{{any_handoff}}")), 0644); err != nil {
			return err
		}
		if filepath.Base(path) == "workflow.yaml" {
			unifiedRewriteFixture(t, path, func(v map[string]any) {
				pkg := v["package"].(map[string]any)
				refs, _ := pkg["references"].(map[string]any)
				if refs == nil {
					refs = map[string]any{}
					pkg["references"] = refs
				}
				refs["any_handoff"] = "core:schema/context-json@1.0.0"
			})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	compiled := compileCompositionName(t, root, authority, "delivery")
	if compiled.SchemaVersion != "project-compile/4" {
		t.Fatal(compiled.SchemaVersion)
	}
	counts := map[string]int{}
	for _, c := range compiled.Components {
		counts[c.Ref.ID]++
	}
	if counts["test:step/build"] != 1 || counts["test:step/commit"] != 1 || counts["test:workflow/delivery"] != 1 {
		t.Fatal(counts)
	}
}

func TestProjectUnifiedPointExportsStayWithinOwnedClosure(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"unreachable", "root", "foreign-target"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root, authority := writeUnifiedImportFixture(t)
			provider := filepath.Join(root, ".prifly/workflows/provider")
			switch name {
			case "unreachable":
				data, err := os.ReadFile(filepath.Join(provider, "workflows/round.yaml"))
				if err != nil {
					t.Fatal(err)
				}
				value, err := flow.Parse(data, "yaml")
				if err != nil {
					t.Fatal(err)
				}
				unrelated := value.(map[string]any)
				unrelated["id"] = "report-probe:workflow/unrelated"
				unrelated["entry"] = "done"
				unrelated["outputs"] = map[string]any{}
				unrelated["allowed_outcomes"] = []any{"succeeded"}
				unrelated["stages"] = map[string]any{"done": map[string]any{"kind": "finish", "outcome": "succeeded"}}
				encoded, err := json.Marshal(unrelated)
				if err != nil {
					t.Fatal(err)
				}
				writeFixtureFile(t, root, ".prifly/workflows/provider/workflows/unrelated.yaml", string(encoded))
				unifiedRewriteFixture(t, filepath.Join(provider, "workflow.yaml"), func(v map[string]any) {
					v["exports"].(map[string]any)["workflows"].(map[string]any)["review"] = "{{workflow_unrelated}}"
				})
			case "root":
				unifiedRewriteFixture(t, filepath.Join(provider, "workflow.yaml"), func(v map[string]any) {
					v["exports"].(map[string]any)["workflows"].(map[string]any)["review"] = "{{workflow_transport}}"
				})
			case "foreign-target":
				unifiedRewriteFixture(t, filepath.Join(provider, "extension-points.yaml"), func(v map[string]any) {
					v["points"].(map[string]any)["before-read"].(map[string]any)["workflow"] = "foreign:workflow/round"
				})
			}
			_, _, p := compilePointFixture(t, root, authority)
			if p == nil || (p.Code != "project_composition_invalid_export" && p.Code != "project_extension_point_invalid" && p.Code != "project_extension_point_unknown_workflow") {
				t.Fatalf("invalid export %s: %+v", name, p)
			}
		})
	}
}

func TestProjectUnifiedDisabledAndUnconfiguredAliasesHaveValidMappings(t *testing.T) {
	t.Parallel()
	root, authority := writeUnifiedImportFixture(t)
	changePointFixture(t, root, "extend.yaml", "step: measure", "enabled: false\n    step: measure")
	c, _, p := compilePointFixture(t, root, authority)
	if p != nil {
		t.Fatalf("disabled/unconfigured variants: %+v", p)
	}
	refs := map[flow.Ref]bool{}
	for _, component := range c.Components {
		if refs[component.Ref] {
			t.Fatal("duplicate compiled ref", component.Ref)
		}
		refs[component.Ref] = true
	}
	if c.Composition == nil || c.Composition.Unified == nil {
		t.Fatal("missing unified review")
	}
	found := false
	for _, point := range c.Composition.Unified.Points {
		if !point.Enabled {
			found = true
		}
	}
	if !found {
		t.Fatal("disabled choice disappeared from review")
	}
}

func TestProjectUnifiedReviewNamesTheSelectedPointExport(t *testing.T) {
	t.Parallel()
	root, authority := writeUnifiedImportFixture(t)
	unifiedRewriteFixture(t, filepath.Join(root, ".prifly/workflows/provider/workflow.yaml"), func(v map[string]any) {
		points := v["exports"].(map[string]any)["points"].(map[string]any)
		points["public-hook"] = points["before-read"]
		delete(points, "before-read")
	})
	changePointFixture(t, root, "extend.yaml", "export: before-read", "export: public-hook")
	c, _, p := compilePointFixture(t, root, authority)
	if p != nil {
		t.Fatal(p)
	}
	data, err := json.Marshal(c.Composition)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err = json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	for _, raw := range value["points"].([]any) {
		point := raw.(map[string]any)
		if point["name"] == "before-read" {
			if point["export"] != "public-hook" {
				t.Fatalf("public export not reviewed: %v", point)
			}
			return
		}
	}
	t.Fatal("selected point absent from review")
}

func addUnifiedFixtureFeature(t *testing.T, root string) {
	t.Helper()
	folder := filepath.Join(root, ".prifly/workflows/provider")
	unifiedRewriteFixture(t, filepath.Join(folder, "workflows/round.yaml"), func(v map[string]any) {
		v["inputs"].(map[string]any)["feature_enabled"] = map[string]any{"schema_ref": "{{any_carrier}}", "required": false, "configuration": map[string]any{"scope": "project", "default": true}}
		v["features"] = map[string]any{"demo": map[string]any{"input": "feature_enabled"}}
		v["entry"] = "feature-choice"
		v["stages"].(map[string]any)["feature-choice"] = map[string]any{"kind": "choice", "selection": "first_match", "branches": []any{map[string]any{"id": "enabled", "predicate": map[string]any{"op": "eq", "left": "$inputs.feature_enabled", "right": true}, "next": "read"}}, "default": "clean", "on_unknown": "clean"}
	})
	unifiedRewriteFixture(t, filepath.Join(folder, "extension-points.yaml"), func(v map[string]any) {
		v["points"].(map[string]any)["before-read"].(map[string]any)["entry"] = "feature-choice"
	})
}

func TestProjectUnifiedFeatureDeclarationFollowsItsVariants(t *testing.T) {
	t.Parallel()
	root, authority := writeUnifiedImportFixture(t)
	addUnifiedFixtureFeature(t, root)
	writeFixtureFile(t, root, ".prifly/workflows/transport/extend.yaml", "authoring: prifly-extension/5\nimports: {a: provider, b: provider}\nsettings: {a.review: {feature_enabled: false}, b.review: {feature_enabled: true}}\nextensions: []\n")
	c, out, p := compilePointFixture(t, root, authority)
	if p != nil {
		t.Fatal(p)
	}
	defaults := map[bool]int{}
	for _, component := range c.Components {
		if component.Ref.ID != "report-probe:workflow/round" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(out, component.Path))
		if err != nil {
			t.Fatal(err)
		}
		var w flow.WorkflowRevision
		if err = json.Unmarshal(data, &w); err != nil {
			t.Fatal(err)
		}
		var flag bool
		if err = json.Unmarshal(w.Inputs["feature_enabled"].Configuration.Default, &flag); err != nil {
			t.Fatal(err)
		}
		defaults[flag]++
	}
	if defaults[true] != 1 || defaults[false] != 1 {
		t.Fatal(defaults)
	}
}

func TestProjectUnifiedFeatureExcludeRetainsSettingConflicts(t *testing.T) {
	t.Parallel()
	for _, setting := range []string{"true", "false", "absent"} {
		t.Run(setting, func(t *testing.T) {
			t.Parallel()
			root, authority := writeUnifiedImportFixture(t)
			addUnifiedFixtureFeature(t, root)
			unifiedRewriteFixture(t, filepath.Join(root, ".prifly/workflows/transport/workflow.yaml"), func(v map[string]any) { v["refs"].(map[string]any)["second"] = "a.review" })
			settings := ""
			if setting != "absent" {
				settings = "settings: {a.review: {feature_enabled: " + setting + "}}\n"
			}
			writeFixtureFile(t, root, ".prifly/workflows/transport/extend.yaml", "authoring: prifly-extension/5\nimports: {a: provider, b: provider}\n"+settings+"exclude: [demo]\nextensions: []\n")
			c, out, p := compilePointFixture(t, root, authority)
			if setting == "true" {
				if p == nil || p.Code != "project_option_conflict" {
					t.Fatalf("conflicting exclude: %+v", p)
				}
				return
			}
			if p != nil {
				t.Fatal(p)
			}
			for _, component := range c.Components {
				if component.Ref.ID != "report-probe:workflow/round" {
					continue
				}
				data, err := os.ReadFile(filepath.Join(out, component.Path))
				if err != nil {
					t.Fatal(err)
				}
				var w flow.WorkflowRevision
				if err = json.Unmarshal(data, &w); err != nil {
					t.Fatal(err)
				}
				if string(w.Inputs["feature_enabled"].Configuration.Default) != "false" {
					t.Fatal("exclude not applied", string(w.Inputs["feature_enabled"].Configuration.Default))
				}
				return
			}
			t.Fatal("feature workflow absent")
		})
	}
}

func TestProjectUnifiedExternalRootBindingUsesNormalResolver(t *testing.T) {
	t.Parallel()
	root, authority := writeUnifiedImportFixture(t)
	unifiedRewriteFixture(t, filepath.Join(root, ".prifly/workflows/transport/workflow.yaml"), func(v map[string]any) {
		v["stages"].(map[string]any)["first"].(map[string]any)["on"] = map[string]any{"succeeded": "second", "partial": "done", "rejected": "done", "no_work": "done"}
	})
	writeFixtureFile(t, root, ".prifly/workflows/transport/extend.yaml", `authoring: prifly-extension/5
imports: {a: provider, b: provider}
extensions:
  - id: literal-review
    workflow: root
    between: {from: first, to: second}
    call: a.review
    input_bindings:
      carrier: {from: literal, value: {candidate: literal, reports: []}, schema_ref: "{{any_carrier}}"}
    on: {succeeded: second, partial: done, rejected: done, no_work: done}
    on_error: done
`)
	c, out, p := compilePointFixture(t, root, authority)
	if p != nil {
		t.Fatal(p)
	}
	w := pointFixtureWorkflow(t, c, out, "transport")
	binding := w.Definition.Stages["literal-review"].InputBindings["carrier"]
	if binding.From != "literal" || binding.SchemaRef == nil || binding.SchemaRef.ID != "core:schema/context-json" || !strings.Contains(string(binding.Value), "literal") {
		t.Fatalf("external binding: %+v", binding)
	}
}

func TestProjectUnifiedFeaturesFromDifferentSourcesStillConflict(t *testing.T) {
	t.Parallel()
	root, authority := writeUnifiedImportFixture(t)
	addUnifiedFixtureFeature(t, root)
	unifiedRewriteFixture(t, filepath.Join(root, ".prifly/workflows/transport/workflow.yaml"), func(v map[string]any) {
		v["inputs"].(map[string]any)["feature_enabled"] = map[string]any{"schema_ref": "{{any_carrier}}", "required": false, "configuration": map[string]any{"scope": "project", "default": true}}
		v["features"] = map[string]any{"demo": map[string]any{"input": "feature_enabled"}}
		v["entry"] = "feature-choice"
		v["stages"].(map[string]any)["feature-choice"] = map[string]any{"kind": "choice", "selection": "first_match", "branches": []any{map[string]any{"id": "enabled", "predicate": map[string]any{"op": "eq", "left": "$inputs.feature_enabled", "right": true}, "next": "first"}}, "default": "done", "on_unknown": "done"}
	})
	writeFixtureFile(t, root, ".prifly/workflows/transport/extend.yaml", "authoring: prifly-extension/5\nimports: {a: provider, b: provider}\nextensions: []\n")
	_, _, p := compilePointFixture(t, root, authority)
	if p == nil || p.Code != "project_feature_duplicate" {
		t.Fatalf("independent feature declarations: %+v", p)
	}
}

func TestProjectUnifiedRootBindingKeepsLateBoundProfileValue(t *testing.T) {
	t.Parallel()
	root, authority := writeUnifiedImportFixture(t)
	unifiedRewriteFixture(t, filepath.Join(root, ".prifly/workflows/transport/workflow.yaml"), func(v map[string]any) {
		v["stages"].(map[string]any)["first"].(map[string]any)["on"] = map[string]any{"succeeded": "second", "partial": "done", "rejected": "done", "no_work": "done"}
		v["package"].(map[string]any)["profiles"] = map[string]any{"default": "base", "values": map[string]any{"base": map[string]any{"configured_carrier": "from-profile"}}}
	})
	writeFixtureFile(t, root, ".prifly/workflows/transport/extend.yaml", `authoring: prifly-extension/5
imports: {a: provider, b: provider}
extensions:
  - id: profile-review
    workflow: root
    between: {from: first, to: second}
    call: a.review
    input_bindings:
      carrier: {from: literal, value: "{{configured_carrier}}", schema_ref: "{{any_carrier}}"}
    on: {succeeded: second, partial: done, rejected: done, no_work: done}
    on_error: done
`)
	c, out, p := compilePointFixture(t, root, authority)
	if p != nil {
		t.Fatal(p)
	}
	w := pointFixtureWorkflow(t, c, out, "transport")
	binding := w.Definition.Stages["profile-review"].InputBindings["carrier"]
	if binding.From != "literal" || !strings.Contains(string(binding.Value), "from-profile") || strings.Contains(string(binding.Value), "{{") {
		t.Fatalf("profile binding: %+v", binding)
	}
}
