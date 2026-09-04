package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// Keep this lifecycle in one authority: separate fixture authorities concealed
// the collision between two profiles with the same author version.
// This exercises Git/host/brief launch with scripted effects:none handoffs,
// two explicitly admitted slots, and release of unused launch claims. It does
// not qualify the later no-Git/no-host launch or an actual model's behavior.
func TestCLIProjectCompiledVariantsLifecycle(t *testing.T) {
	repository, authority := newProjectFixture(t)
	const folder = ".prifly/workflows/sample/"
	const initialContext = "# Inspect\n\nInspect the supplied data.\n"
	const initialExtend = "profile: a\nextensions: []\n"
	writeFixtureFile(t, repository, ".prifly/project.yaml", "schema_version: prifly-project-profile/3\n"+projectHostsYAML+`packages:
  sample: {source: .prifly/workflows/sample}
launches:
  sample:
    title: Inspect data
    description: Neutral compilation lifecycle fixture.
    kind: workflow
    workflow: .prifly/workflows/sample/workflow.yaml
`)
	writeFixtureFile(t, repository, ".codex/skills/inspect/SKILL.md", initialContext)
	writeFixtureFile(t, repository, folder+"contexts/inspect.yaml", `id: test:context/inspect
version: 1.0.0
media_type: text/markdown; charset=utf-8
source: {root: host_skills, path: inspect/SKILL.md}
`)
	for _, name := range []string{"inspect", "quality", "extra"} {
		writeFixtureFile(t, repository, folder+"steps/"+name+".yaml", fmt.Sprintf(`authoring: prifly-step/1
id: test:step/%s
version: 1.0.0
title: "{{inspection_title}}"
kind: worker
executor: {adapter_ref: "{{assisted_adapter}}", operation: session}
instructions_ref: "{{context_inspect}}"
effects: {class: none, retry_class: never}
result_schema_ref: "{{step_result_schema}}"
`, name))
	}
	writeFixtureFile(t, repository, folder+"schemas/flag.yaml", "id: test:schema/flag\nversion: 1.0.0\ntype: boolean\n")
	writeFixtureFile(t, repository, folder+"schemas/limit.yaml", "id: test:schema/limit\nversion: 1.0.0\ntype: integer\nminimum: 1\nmaximum: 3\n")
	writeFixtureFile(t, repository, folder+"workflow.yaml", `authoring: prifly-project-workflow/1
package:
  id: test:package/sample
  version: 1.0.0
  description: Neutral compilation lifecycle fixture.
  requires_core_protocol: "1"
  profiles:
    default: a
    values:
      a: {inspection_title: Inspect data once}
      b: {inspection_title: Inspect data carefully}
  references:
    assisted_adapter: core:adapter/assisted-session@1.0.0
    local_policy: core:policy/local@3.0.0
    step_result_schema: core:schema/step-result@1.0.0
id: test:workflow/sample
version: 1.0.0
refs:
  inspect: "{{step_inspect}}"
  quality: "{{step_quality}}"
  flag: "{{schema_flag}}"
  limit: "{{schema_limit}}"
  local_policy: "{{local_policy}}"
inputs:
  quality_enabled:
    schema_ref: flag
    required: false
    configuration: {scope: project, default: true}
  batch_limit:
    schema_ref: limit
    required: false
    configuration: {scope: project, default: 3}
features: {quality: {input: quality_enabled}}
entry: choose-quality
limits: {max_step_instances: 3, max_control_transitions: 8}
policy_ref: local_policy
stages:
  choose-quality:
    kind: choice
    selection: exclusive
    branches:
      - id: enabled
        predicate: {op: eq, left: $inputs.quality_enabled, right: true}
        next: quality
    default: inspect
  quality: {kind: step, step_ref: quality, on: {pass: inspect}}
  inspect: {kind: step, step_ref: inspect, on: {pass: done}}
  done: {kind: finish, outcome: succeeded}
`)
	writeFixtureFile(t, repository, folder+"extend.yaml", initialExtend)
	gitFixture(t, repository, "add", ".")
	gitFixture(t, repository, "commit", "-qm", "declare neutral variant fixture")
	brief := filepath.Join(t.TempDir(), "brief.json")
	writeFixtureFile(t, filepath.Dir(brief), filepath.Base(brief), `{"schema_version":"1","id":"test:brief/variants","subject":"Inspect data","desired_outcome":"Reach the inspection","in_scope":["inspection"],"out_of_scope":[],"completion_criteria":["inspection handoff"],"source_refs":[],"assumptions":[],"confirmation":"explicit"}`)
	if code, _, errout := runCLI(t, "--project", authority, "capacity", "set", "--capacity", "2", "--reason", "Keep the original Run active while checking another variant"); code != 0 {
		t.Fatalf("set fixture capacity: %d %s", code, errout)
	}

	compile := func(profile string, selectedHost ...string) (projectCompileResult, string) {
		t.Helper()
		host := "codex-cli"
		if len(selectedHost) != 0 {
			host = selectedHost[0]
		}
		directory := filepath.Join(t.TempDir(), "package")
		code, out, errout := runCLI(t, "--project", authority, "project", "compile", "--repository", repository, "--package", "sample", "--package-profile", profile, "--host", host, "--output", directory)
		if code != 0 {
			t.Fatalf("compile profile %s: %d %s", profile, code, errout)
		}
		var compiled projectCompileResult
		if err := json.Unmarshal([]byte(out), &compiled); err != nil {
			t.Fatal(err)
		}
		if compiled.SchemaVersion != "project-compile/2" || compiled.AuthorPackage == nil || *compiled.AuthorPackage != (projectBuildIdentity{ID: "test:package/sample", Version: "1.0.0"}) || !strings.HasPrefix(compiled.BuildKey, "sha256:") {
			t.Fatalf("compile omitted author version or exact build key: %+v", compiled)
		}
		assertVariantRef(t, compiled.Package)
		for _, component := range compiled.Components {
			assertVariantRef(t, component.Ref)
		}
		return compiled, directory
	}
	importPackage := func(directory, wantRefusal string) {
		t.Helper()
		code, out, errout := runCLI(t, "--project", authority, "package", "import", "--dir", directory, "--reason", "Explicit fixture trust")
		var refusal string
		if code != 0 {
			var problem flow.Problem
			if err := json.Unmarshal([]byte(errout), &problem); err != nil || problem.Code == "" {
				t.Fatalf("invalid import problem: %d %s", code, errout)
			}
			refusal = problem.Code
		} else {
			var response struct {
				Receipt struct {
					Rejection *struct{ Code string }
				}
			}
			if err := json.Unmarshal([]byte(out), &response); err != nil {
				t.Fatal(err)
			}
			if response.Receipt.Rejection != nil {
				refusal = response.Receipt.Rejection.Code
			}
		}
		if refusal != wantRefusal {
			t.Fatalf("import wanted %q, got %q: %d %s %s", wantRefusal, refusal, code, out, errout)
		}
	}
	start := func(profile string, compiled projectCompileResult) projectStartResult {
		t.Helper()
		code, out, errout := runCLI(t, "--project", authority, "project", "start", "--repository", repository, "--launch", "sample", "--package-profile", profile, "--host", "codex-cli", "--brief", brief)
		if code != 0 {
			t.Fatalf("start profile %s: %d %s", profile, code, errout)
		}
		var started projectStartResult
		if err := json.Unmarshal([]byte(out), &started); err != nil {
			t.Fatal(err)
		}
		if started.Package != compiled.Package {
			t.Fatalf("compile/start disagree: %+v != %+v", compiled.Package, started.Package)
		}
		if started.SchemaVersion != "project-start/3" || !reflect.DeepEqual(started.AuthorPackage, compiled.AuthorPackage) || started.BuildKey != compiled.BuildKey {
			t.Fatalf("compile/start author version or build key disagree: %+v", started)
		}
		for _, component := range compiled.Components {
			if component.Ref.ID == "test:workflow/sample" && started.Run.Run.WorkflowRef != component.Ref {
				t.Fatalf("start selected the wrong compiled root: %+v != %+v", started.Run.Run.WorkflowRef, component.Ref)
			}
		}
		code, out, errout = runCLI(t, "--project", authority, "session", "task", "--run", started.Run.Run.ID)
		var task prifly.SessionTask
		if code != 0 || json.Unmarshal([]byte(out), &task) != nil || task.ClaimID != "" {
			t.Fatalf("effects:none fixture must have a real handoff without a repository claim: %d %s %s", code, out, errout)
		}
		if started.Workspace != nil {
			t.Fatal("effects:none launch must not create a workspace claim")
		}
		return started
	}
	readRun := func(id string) (prifly.Run, prifly.SessionTask) {
		t.Helper()
		// No engine survives a CLI invocation or this read. Reopening must recover
		// the pinned closure from durable state, not a compiler's memory.
		engine, err := prifly.Open(authority, true)
		if err != nil {
			t.Fatal(err)
		}
		defer engine.Close()
		stored, err := engine.Store.Read(context.Background(), id, 0, 1)
		if err != nil {
			t.Fatal(err)
		}
		// Public views deliberately redact raw definitions and context bytes.
		// Pinning assertions inspect the durable snapshot, never weaken that view.
		var run prifly.Run
		if err := json.Unmarshal(stored.Snapshot.Data, &run); err != nil {
			t.Fatal(err)
		}
		task, err := engine.SessionTask(context.Background(), id, "")
		if err != nil {
			t.Fatal(err)
		}
		return run, task
	}
	finish := func(started projectStartResult) {
		t.Helper()
		for count := 0; count < 3; count++ {
			_, task := readRun(started.Run.Run.ID)
			result, err := json.Marshal(map[string]any{
				"schema_version": "1", "run_id": task.RunID, "step_instance_id": task.StepInstanceID,
				"attempt_id": task.AttemptID, "envelope_digest": task.EnvelopeDigest, "verdict": "pass",
				"outputs": map[string]any{}, "evidence_refs": []any{}, "effect_receipt_refs": []any{}, "summary": "Fixture inspection completed.",
			})
			if err != nil {
				t.Fatal(err)
			}
			submission, err := json.Marshal(prifly.SessionSubmission{SchemaVersion: task.SchemaVersion, RunID: task.RunID, AttemptID: task.AttemptID, EnvelopeDigest: task.EnvelopeDigest, Result: result})
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "submission.json")
			writeFixtureFile(t, filepath.Dir(path), filepath.Base(path), string(submission))
			if code, _, errout := runCLI(t, "--project", authority, "session", "submit", "--file", path); code != 0 {
				t.Fatalf("submit fixture inspection: %d %s", code, errout)
			}
			code, out, errout := runCLI(t, "--project", authority, "run", "drive", task.RunID)
			var view prifly.RunView
			if code != 0 || json.Unmarshal([]byte(out), &view) != nil {
				t.Fatalf("continue fixture inspection: %d %s", code, errout)
			}
			if view.Run.Status == "completed" && view.Run.Outcome != nil && *view.Run.Outcome == "succeeded" {
				return
			}
		}
		t.Fatal("fixture variant did not complete within its three declared steps")
	}
	assertSameBuild := func(a, b projectCompileResult) {
		t.Helper()
		if a.Package != b.Package || a.BuildKey != b.BuildKey || len(a.Components) != len(b.Components) {
			t.Fatalf("identical authoring did not reproduce its build: %+v %+v", a.Package, b.Package)
		}
		for i := range a.Components {
			if a.Components[i].Ref != b.Components[i].Ref {
				t.Fatalf("owned component changed on identical recompilation: %+v %+v", a.Components[i], b.Components[i])
			}
		}
	}

	a, directoryA := compile("a")
	if a.Package.ID != "test:package/sample" || !strings.HasPrefix(a.Package.Version, "0.0.0-b1.") {
		t.Fatalf("author identity was not compiled to b1: %+v", a.Package)
	}
	importPackage(directoryA, "")
	startedA := start("a", a)
	oldRun, oldTask := readRun(startedA.Run.Run.ID)
	if len(oldRun.Active) == 0 || len(oldRun.Definitions) == 0 || len(oldRun.ContextResources) == 0 {
		t.Fatal("the original Run must really be active with pinned definitions and context")
	}
	contextPinned := false
	for _, resource := range oldRun.ContextResources {
		if resource.Ref.ID == "test:context/inspect" {
			contextPinned = bytes.Equal(resource.Bytes, []byte(initialContext))
		}
	}
	if !contextPinned {
		t.Fatal("the initial source's exact bytes were not pinned")
	}
	b, directoryB := compile("b")
	if a.Package == b.Package || a.Package.Version == b.Package.Version {
		t.Fatal("different profiles share one compiled identity")
	}
	for _, left := range a.Components {
		for _, right := range b.Components {
			if left.Ref.ID == right.Ref.ID && left.Ref.Version == right.Ref.Version {
				t.Fatalf("owned %s still collides across profiles", left.Ref.ID)
			}
		}
	}
	refFile := filepath.Join(t.TempDir(), "package-ref.json")
	refBytes, _ := json.Marshal(b.Package)
	writeFixtureFile(t, filepath.Dir(refFile), filepath.Base(refFile), string(refBytes))
	if code, _, errout := runCLI(t, "--project", authority, "package", "inspect", "--ref", refFile); code == 0 {
		t.Fatalf("a newly compiled variant inherited trust before import: %s", errout)
	}
	importPackage(directoryB, "")
	finish(start("b", b))
	againA, repeatedDirectory := compile("a")
	assertSameBuild(a, againA)
	importPackage(repeatedDirectory, "package_present")
	finish(start("a", againA))
	if got, err := os.ReadFile(filepath.Join(repository, folder, "extend.yaml")); err != nil || string(got) != initialExtend {
		t.Fatalf("per-Run selection changed the tracked default: %v %q", err, got)
	}

	for _, change := range []struct{ name, extend string }{
		{"setting", "profile: a\nsettings: {sample: {batch_limit: 2}}\nextensions: []\n"},
		{"exclude", "profile: a\nexclude: [quality]\nextensions: []\n"},
		{"insert", "profile: a\nextensions:\n  - id: extra-inspection\n    workflow: sample\n    between: {from: inspect, to: done}\n    step: extra\n    on: {pass: done}\n"},
	} {
		t.Logf("same-authority variation: %s", change.name)
		writeFixtureFile(t, repository, folder+"extend.yaml", change.extend)
		changed, directory := compile("a")
		if changed.Package.Version == a.Package.Version {
			t.Fatalf("%s did not change the compiled identity", change.name)
		}
		importPackage(directory, "")
		finish(start("a", changed))
	}
	writeFixtureFile(t, repository, folder+"extend.yaml", initialExtend)
	writeFixtureFile(t, repository, ".codex/skills/inspect/SKILL.md", "# Inspect\n\nUse the changed inspection instructions.\n")
	changedContext, contextDirectory := compile("a")
	if changedContext.Package.Version == a.Package.Version {
		t.Fatal("changed context bytes reused the old compiled identity")
	}
	importPackage(contextDirectory, "")
	finish(start("a", changedContext))
	writeFixtureFile(t, repository, ".codex/skills/inspect/SKILL.md", initialContext)
	restored, _ := compile("a")
	assertSameBuild(a, restored)
	rootPath := filepath.Join(repository, folder, "workflow.yaml")
	rootBytes, err := os.ReadFile(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	leaf := filepath.Join(repository, folder, "steps", "inspect.yaml")
	for _, relocated := range []string{"steps/zz-renamed.yaml", "steps/inspection/deep/renamed.yaml"} {
		destination := filepath.Join(repository, folder, filepath.FromSlash(relocated))
		if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(leaf, destination); err != nil {
			t.Fatal(err)
		}
		moved, _ := compile("a")
		assertSameBuild(a, moved)
		if err := os.Rename(destination, leaf); err != nil {
			t.Fatal(err)
		}
	}
	writeFixtureFile(t, repository, ".agents/skills/inspect/SKILL.md", initialContext)
	otherHost, _ := compile("a", "codex-app")
	assertSameBuild(a, otherHost)
	restoredLayout, _ := compile("a")
	assertSameBuild(a, restoredLayout)
	if got, err := os.ReadFile(rootPath); err != nil || !bytes.Equal(got, rootBytes) {
		t.Fatalf("layout/host changes rewrote the workflow graph: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(repository, folder, "extend.yaml")); err != nil || string(got) != initialExtend {
		t.Fatalf("layout/host changes rewrote tracked defaults: %v %q", err, got)
	}
	describedRoot := strings.Replace(string(rootBytes), "  description: Neutral compilation lifecycle fixture.", "  description: Changed description without changing the workflow.", 1)
	if describedRoot == string(rootBytes) {
		t.Fatal("description-only fixture did not change the package description")
	}
	writeFixtureFile(t, repository, folder+"workflow.yaml", describedRoot)
	described, descriptionDirectory := compile("a")
	if described.Package.Version == a.Package.Version {
		t.Fatal("package description alone did not change compilation identity")
	}
	importPackage(descriptionDirectory, "")
	writeFixtureFile(t, repository, folder+"workflow.yaml", string(rootBytes))
	withoutDescriptionChange, _ := compile("a")
	assertSameBuild(a, withoutDescriptionChange)

	// The two imports below have identical authored workflow bytes. Only the
	// human-readable question changes, so hashing graph components alone fails.
	catalogRoot := string(rootBytes) + "\ndecision_catalog: [.prifly/workflows/sample/decisions/inspection.yaml]\n"
	writeFixtureFile(t, repository, folder+"workflow.yaml", catalogRoot)
	previousCatalog := a.Package
	for _, title := range []string{"Choose inspection detail", "How much inspection detail is needed"} {
		writeFixtureFile(t, repository, folder+"decisions/inspection.yaml", fmt.Sprintf(`authoring: prifly-run-decision/1
id: inspection_detail
title: %s
phase: preflight
choices: [{id: concise, title: Concise, value: concise}]
destination: {kind: session_context, name: inspection_detail}
`, title))
		withCatalog, catalogDirectory := compile("a")
		if withCatalog.Package.Version == previousCatalog.Version || withCatalog.Package.Version == a.Package.Version {
			t.Fatal("question title did not produce its own compiled identity")
		}
		importPackage(catalogDirectory, "")
		previousCatalog = withCatalog.Package
		if got, err := os.ReadFile(rootPath); err != nil || string(got) != catalogRoot {
			t.Fatalf("question-only compilation changed authored workflow bytes: %v", err)
		}
	}
	writeFixtureFile(t, repository, folder+"workflow.yaml", string(rootBytes))
	restoredMetadata, _ := compile("a")
	assertSameBuild(a, restoredMetadata)
	currentRun, currentTask := readRun(oldRun.ID)
	if oldRun.WorkflowRef != currentRun.WorkflowRef || oldRun.LockRef != currentRun.LockRef || !bytes.Equal(oldRun.Workflow, currentRun.Workflow) || !reflect.DeepEqual(oldRun.Definitions, currentRun.Definitions) || !reflect.DeepEqual(oldRun.ContextResources, currentRun.ContextResources) || !reflect.DeepEqual(oldTask, currentTask) {
		t.Fatal("later variants or authority reopen changed the original Run's pinned bytes/handoff")
	}

	_, tamperedDirectory := compile("a")
	component := a.Components[0]
	writeFixtureFile(t, tamperedDirectory, component.Path, "tampered sealed payload\n")
	importPackage(tamperedDirectory, "digest_mismatch")
	_, collisionDirectory := compile("a")
	manifestPath := filepath.Join(collisionDirectory, prifly.PackageManifestFile)
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest["description"] = "Different bytes claiming the same compiled package identity."
	manifestBytes, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, collisionDirectory, prifly.PackageManifestFile, string(manifestBytes))
	importPackage(collisionDirectory, "package_identity_conflict")
	if code, _, errout := runCLI(t, "--project", authority, "package", "revoke", "--id", a.Package.ID, "--version", a.Package.Version, "--reason", "Fixture revocation"); code != 0 {
		t.Fatalf("revoke variant: %d %s", code, errout)
	}
	recompiled, revokedDirectory := compile("a")
	assertSameBuild(a, recompiled)
	importPackage(revokedDirectory, "package_revoked")
}

// Compiled refs must remain valid public ImmutableRefs, not merely strings the
// CLI accepts internally. Version's length limit protects persisted contracts.
func assertVariantRef(t *testing.T, ref flow.Ref) {
	t.Helper()
	data, err := json.Marshal(ref)
	if err != nil {
		t.Fatal(err)
	}
	if err := flow.ValidateProtocol("ImmutableRef", data); err != nil {
		t.Fatalf("invalid compiled ref %+v: %v", ref, err)
	}
}
