package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func TestContinuationSelectsUnmergedImplementationCommit(t *testing.T) {
	root, authority := newProjectFixture(t)
	writeFixtureFile(t, root, "feature.txt", "base\n")
	gitFixture(t, root, "add", ".")
	gitFixture(t, root, "commit", "-q", "-m", "base")
	base := gitFixture(t, root, "rev-parse", "HEAD")
	branch := filepath.Join(t.TempDir(), "implementation")
	gitFixture(t, root, "worktree", "add", "-q", "-b", "implementation", branch)
	if err := os.WriteFile(filepath.Join(branch, "feature.txt"), []byte("implemented\n"), 0644); err != nil {
		t.Fatal(err)
	}
	gitFixture(t, branch, "add", ".")
	gitFixture(t, branch, "commit", "-q", "-m", "implementation")
	implementation := gitFixture(t, branch, "rev-parse", "HEAD")
	if got, err := projectContinuationHead(context.Background(), root, implementation); err != nil || got != implementation {
		t.Fatalf("unmerged implementation = %s, %v; want %s", got, err, implementation)
	}
	if _, err := projectContinuationHead(context.Background(), root, "542493c"); err == nil || !strings.Contains(err.Error(), "project_continue_invalid_head") {
		t.Fatalf("abbreviated commit accepted: %v", err)
	}
	if _, err := projectContinuationHead(context.Background(), root, strings.Repeat("f", 40)); err == nil || !strings.Contains(err.Error(), "project_continue_invalid_head") {
		t.Fatalf("missing commit accepted: %v", err)
	}
	gitFixture(t, branch, "tag", "-a", "implementation-tag", "-m", "tag object")
	tagObject := gitFixture(t, branch, "rev-parse", "implementation-tag^{tag}")
	if _, err := projectContinuationHead(context.Background(), root, tagObject); err == nil || !strings.Contains(err.Error(), "project_continue_invalid_head") {
		t.Fatalf("tag object accepted as an exact commit: %v", err)
	}
	engine, err := prifly.Open(authority, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = engine.Close() }()
	claim, err := engine.ClaimWorktree(context.Background(), prifly.ClaimRequest{CommandID: "command:continuation", Repository: root, BaseRef: implementation, OwnerID: "session:continuation", WorkspaceMode: "worktree"})
	if err != nil {
		t.Fatal(err)
	}
	if got := gitFixture(t, filepath.Join(authority, claim.Path), "rev-parse", "HEAD"); got != implementation {
		t.Fatalf("claimed worktree HEAD = %s; want %s", got, implementation)
	}
	if got := gitFixture(t, root, "rev-parse", "HEAD"); got != base {
		t.Fatalf("base checkout moved to %s; want %s", got, base)
	}
}

func TestActiveContinuationSelectsOnlyUnfinishedChildOfSource(t *testing.T) {
	runs := []prifly.RunSummary{
		{ID: "run:other-source", ForkSourceRunID: "run:other", ForkReason: prifly.ContinuationReason, Status: "running"},
		{ID: "run:ordinary-fork", ForkSourceRunID: "run:source", ForkReason: "manual fork", Status: "running"},
		{ID: "run:completed", ForkSourceRunID: "run:source", ForkReason: prifly.ContinuationReason, Status: "completed"},
		{ID: "run:failed", ForkSourceRunID: "run:source", ForkReason: prifly.ContinuationReason, Status: "failed"},
		{ID: "run:cancelled", ForkSourceRunID: "run:source", ForkReason: prifly.ContinuationReason, Status: "cancelled"},
		{ID: "run:active", ForkSourceRunID: "run:source", ForkReason: prifly.ContinuationReason, Status: "running"},
	}
	if got := projectActiveContinuation(runs, "run:source"); got == nil || got.ID != "run:active" || got.Status != "running" {
		t.Fatalf("active child = %+v", got)
	}
	if got := projectActiveContinuation(runs, "run:other"); got == nil || got.ID != "run:other-source" {
		t.Fatalf("other source child = %+v", got)
	}
	if got := projectActiveContinuation(runs[:5], "run:source"); got != nil {
		t.Fatalf("terminal or unrelated child blocked source: %+v", got)
	}
}

func TestDuplicateContinuationChoiceChangesReviewDigest(t *testing.T) {
	summary := projectLaunchSummary{SchemaVersion: "project-launch-summary/3", Launch: "aif-classic-continuation"}
	without, err := projectReviewDigest(summary)
	if err != nil {
		t.Fatal(err)
	}
	summary.AllowDuplicateContinuation = true
	with, err := projectReviewDigest(summary)
	if err != nil {
		t.Fatal(err)
	}
	if without == with {
		t.Fatal("explicit duplicate choice did not change the reviewed launch")
	}
}

// Nothing in this fixture belongs to a package the engine knows: the tail
// workflow declares what it continues and where each input comes from, and
// the continuation takes over the source Run's tree as it was left.
func TestCLIContinuationTakesOverTheSourceTree(t *testing.T) {
	root, authority := newProjectFixture(t)
	writeFixtureFile(t, root, ".prifly/project.yaml", `schema_version: prifly-project-profile/3
`+projectHostsYAML+`packages:
  source: {source: .prifly/workflows/source}
  tail: {source: .prifly/workflows/tail}
launches:
  source:
    title: Source
    description: Partial source Run.
    kind: workflow
    workflow: .prifly/workflows/source/workflow.yaml
    workspace: worktree
  tail:
    title: Continuation
    description: Quality tail from saved work.
    kind: workflow
    workflow: .prifly/workflows/tail/workflow.yaml
    workspace: worktree
`)
	for _, name := range []string{"source", "tail"} {
		folder := ".prifly/workflows/" + name + "/"
		writeFixtureFile(t, root, folder+"schemas/object.yaml", "id: example-"+name+":schema/object\nversion: 1.0.0\ntype: object\n")
		writeFixtureFile(t, root, folder+"contexts/work.yaml", "id: example-"+name+":context/work\nversion: 1.0.0\nmedia_type: text/markdown; charset=utf-8\ntext: Fixture work.\n")
		writeFixtureFile(t, root, folder+"extend.yaml", "extensions: []\n")
	}
	writeFixtureFile(t, root, ".prifly/workflows/source/steps/prepare.yaml", `authoring: prifly-step/1
id: example:step/prepare
version: 1.0.0
kind: worker
outputs: {handoff: {schema_ref: "{{schema_object}}", required_for: [pass]}}
executor: {adapter_ref: "{{assisted_adapter}}", operation: session}
instructions_ref: "{{context_work}}"
effects: {class: none, retry_class: never}
result_schema_ref: "{{step_result_schema}}"
`)
	writeFixtureFile(t, root, ".prifly/workflows/source/steps/draft.yaml", `authoring: prifly-step/1
id: example:step/draft
version: 1.0.0
kind: worker
inputs: {handoff: {schema_ref: "{{schema_object}}"}}
outputs:
  plan: {schema_ref: "{{workspace_tree_manifest}}", required_for: [pass]}
  draft: {schema_ref: "{{schema_object}}", required_for: [pass]}
executor: {adapter_ref: "{{assisted_adapter}}", operation: session}
instructions_ref: "{{context_work}}"
effects: {class: workspace_write, retry_class: never}
result_schema_ref: "{{step_result_schema}}"
workspace_trees:
  - output_port: plan
    capture: {kind: exact_file, path: plans/plan.md}
`)
	writeFixtureFile(t, root, ".prifly/workflows/source/workflow.yaml", `authoring: prifly-project-workflow/1
package:
  id: example-source:package/source
  version: 1.0.0
  description: Minimal partial source fixture.
  requires_core_protocol: "1"
  references:
    assisted_adapter: core:adapter/assisted-session@1.0.0
    local_policy: core:policy/local@3.0.0
    step_result_schema: core:schema/step-result@1.0.0
    workspace_tree_manifest: core:schema/workspace-tree-manifest@1.0.0
id: example:workflow/source
version: 1.0.0
refs:
  object: "{{schema_object}}"
  prepare: "{{step_prepare}}"
  draft: "{{step_draft}}"
  local_policy: "{{local_policy}}"
inputs: {task: {schema_ref: object}}
entry: prepare
limits: {max_step_instances: 2, max_control_transitions: 3}
policy_ref: local_policy
stages:
  prepare: {kind: step, step_ref: prepare, on: {pass: draft}, impossible_verdicts: [fail, needs_revision, no_work]}
  draft:
    kind: step
    step_ref: draft
    input_bindings: {handoff: $stages.prepare.handoff}
    on: {pass: partial}
    impossible_verdicts: [fail, needs_revision, no_work]
  partial: {kind: finish, outcome: partial}
`)
	writeFixtureFile(t, root, ".prifly/workflows/tail/steps/verify.yaml", `authoring: prifly-step/1
id: example:step/verify
version: 1.0.0
kind: worker
inputs:
  draft: {schema_ref: "{{schema_object}}"}
  plan: {schema_ref: "{{workspace_tree_manifest}}"}
executor: {adapter_ref: "{{assisted_adapter}}", operation: session}
instructions_ref: "{{context_work}}"
effects: {class: none, retry_class: never}
result_schema_ref: "{{step_result_schema}}"
workspace_trees:
  - input_port: plan
    capture: {kind: exact_file, path: plans/plan.md}
`)
	tail := `authoring: prifly-project-workflow/1
package:
  id: example-tail:package/tail
  version: 1.0.0
  description: Minimal continuation fixture.
  requires_core_protocol: "1"
  references:
    assisted_adapter: core:adapter/assisted-session@1.0.0
    local_policy: core:policy/local@3.0.0
    step_result_schema: core:schema/step-result@1.0.0
    workspace_tree_manifest: core:schema/workspace-tree-manifest@1.0.0
id: example:workflow/tail
version: 1.0.0
refs:
  object: "{{schema_object}}"
  plan: "{{workspace_tree_manifest}}"
  verify: "{{step_verify}}"
  local_policy: "{{local_policy}}"
inputs:
  task: {schema_ref: object}
  handoff: {schema_ref: object}
  plan: {schema_ref: plan}
  draft: {schema_ref: object}
continuation:
  from_workflows: [example:workflow/source]
  from_outcomes: [partial]
  inputs:
    task: {source_input: task}
    handoff: {stage: prepare, output: handoff, verdict: pass}
    plan: {stage: draft, output: plan, verdict: pass}
    draft: {stage: draft, output: draft, verdict: pass}
entry: verify
limits: {max_step_instances: 1, max_control_transitions: 2}
policy_ref: local_policy
stages:
  verify:
    kind: step
    step_ref: verify
    input_bindings: {draft: $inputs.draft, plan: $inputs.plan}
    on: {pass: done}
    impossible_verdicts: [fail, needs_revision, no_work, blocked]
  done: {kind: finish, outcome: succeeded}
`
	writeFixtureFile(t, root, ".prifly/workflows/tail/workflow.yaml", tail)
	writeFixtureFile(t, root, "task.json", "{}\n")
	gitFixture(t, root, "add", ".")
	gitFixture(t, root, "commit", "-qm", "base")
	base := gitFixture(t, root, "rev-parse", "HEAD")
	command := func(args ...string) string {
		t.Helper()
		code, out, stderr := runCLI(t, args...)
		if code != 0 {
			t.Fatalf("%v: exit=%d %s", args, code, stderr)
		}
		return out
	}
	refuse := func(code string, args ...string) string {
		t.Helper()
		exit, _, stderr := runCLI(t, args...)
		if exit == 0 || !strings.Contains(stderr, `"code":"`+code+`"`) {
			t.Fatalf("%v: wanted %s, exit=%d %s", args, code, exit, stderr)
		}
		return stderr
	}
	startArgs := []string{"--repository", root, "--launch", "source", "--host", "codex-cli", "--workspace", "worktree", "--input", "task=" + filepath.Join(root, "task.json")}
	var sourceReview projectLaunchSummary
	if err := json.Unmarshal([]byte(command(append([]string{"project", "questionnaire", "--prepare"}, startArgs...)...)), &sourceReview); err != nil {
		t.Fatal(err)
	}
	var source projectStartResult
	if err := json.Unmarshal([]byte(command(append(append([]string{"project", "start"}, startArgs...), "--expected-launch-digest", sourceReview.ReviewDigest)...)), &source); err != nil {
		t.Fatal(err)
	}
	if source.WorkspacePath == "" || source.Workspace == nil {
		t.Fatal("source Run has no claim worktree")
	}
	submit := func(outputs map[string]string, trees []prifly.WorkspaceTreeLocation) {
		t.Helper()
		var task prifly.SessionTask
		if err := json.Unmarshal([]byte(command("--project", authority, "session", "task", "--run", source.Run.Run.ID)), &task); err != nil {
			t.Fatal(err)
		}
		refs := map[string]prifly.ArtifactRef{}
		for port, value := range outputs {
			slot := task.Context.Outputs[port]
			if slot.ArtifactID == "" {
				t.Fatalf("no output slot for %s", port)
			}
			writeFixtureFile(t, task.Workspace, slot.Path, value)
			refs[port] = prifly.ArtifactRef{ArtifactID: slot.ArtifactID, Revision: slot.Revision, Digest: projectBytesDigest([]byte(value))}
		}
		result, err := json.Marshal(map[string]any{"schema_version": "1", "run_id": task.RunID, "step_instance_id": task.StepInstanceID, "attempt_id": task.AttemptID, "envelope_digest": task.EnvelopeDigest, "verdict": "pass", "outputs": refs, "evidence_refs": []any{}, "effect_receipt_refs": []any{}, "summary": "fixture output"})
		if err != nil {
			t.Fatal(err)
		}
		submission, err := json.Marshal(prifly.SessionSubmission{SchemaVersion: task.SchemaVersion, RunID: task.RunID, AttemptID: task.AttemptID, EnvelopeDigest: task.EnvelopeDigest, Result: result, WorkspaceTrees: trees})
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "submission.json")
		if err := os.WriteFile(path, submission, 0644); err != nil {
			t.Fatal(err)
		}
		command("--project", authority, "session", "submit", "--file", path)
		command("--project", authority, "run", "drive", source.Run.Run.ID)
	}
	submit(map[string]string{"handoff": "{}\n"}, nil)
	writeFixtureFile(t, source.WorkspacePath, "feature.txt", "drafted\n")
	writeFixtureFile(t, source.WorkspacePath, "plans/plan.md", "# Plan\n")
	gitFixture(t, source.WorkspacePath, "add", ".")
	gitFixture(t, source.WorkspacePath, "commit", "-qm", "draft")
	drafted := gitFixture(t, source.WorkspacePath, "rev-parse", "HEAD")
	submit(map[string]string{"draft": "{}\n"}, []prifly.WorkspaceTreeLocation{{OutputPort: "plan", Path: "plans/plan.md"}})
	// Left behind uncommitted, as a step stopped mid-way leaves its work.
	writeFixtureFile(t, source.WorkspacePath, "unsaved.txt", "work in progress\n")

	continueArgs := []string{"--repository", root, "--launch", "tail", "--host", "codex-cli", "--source-run", source.Run.Run.ID}
	refuse("project_continue_undeclared", "project", "continue", "--prepare", "--repository", root, "--launch", "source", "--host", "codex-cli", "--source-run", source.Run.Run.ID)
	refuse("project_continue_invalid_head", append(append([]string{"project", "continue", "--prepare"}, continueArgs...), "--workspace-commit", "542493c", "--workspace", "worktree")...)
	refuse("project_continue_invalid_workspace", append(append([]string{"project", "continue", "--prepare"}, continueArgs...), "--workspace-commit", drafted, "--workspace", "checkout")...)
	refuse("project_continue_input_override", append(append([]string{"project", "continue", "--prepare"}, continueArgs...), "--input", "task="+filepath.Join(root, "task.json"))...)
	var reviewed projectLaunchSummary
	if err := json.Unmarshal([]byte(command(append([]string{"project", "continue", "--prepare"}, continueArgs...)...)), &reviewed); err != nil {
		t.Fatal(err)
	}
	if reviewed.Continuation == nil || reviewed.Continuation.Source.Claim == nil || reviewed.Continuation.Source.Claim.ID != source.Workspace.ID {
		t.Fatalf("review does not hand over the source tree: %+v", reviewed.Continuation)
	}
	if got := reviewed.Continuation.Source.Inputs["handoff"].Source; got.Stage != "prepare" || got.Output != "handoff" || got.Verdict != "pass" {
		t.Fatalf("review does not name where handoff comes from: %+v", reviewed.Continuation.Source.Inputs)
	}
	refuse("project_start_stale_launch", append(append([]string{"project", "continue"}, continueArgs...), "--expected-launch-digest", "sha256:000")...)
	var child projectStartResult
	if err := json.Unmarshal([]byte(command(append(append([]string{"project", "continue"}, continueArgs...), "--expected-launch-digest", reviewed.ReviewDigest)...)), &child); err != nil {
		t.Fatal(err)
	}
	if child.Workspace == nil || child.Workspace.ID != source.Workspace.ID || child.Workspace.RunID != child.Run.Run.ID || child.Workspace.Generation != source.Workspace.Generation+1 {
		t.Fatalf("the continuation did not take over the source tree: %+v", child.Workspace)
	}
	if data, err := os.ReadFile(filepath.Join(child.WorkspacePath, "unsaved.txt")); err != nil || string(data) != "work in progress\n" {
		t.Fatalf("uncommitted work did not reach the continuation: %q %v", data, err)
	}
	if gitFixture(t, child.WorkspacePath, "rev-parse", "HEAD") != drafted || gitFixture(t, root, "rev-parse", "HEAD") != base {
		t.Fatal("the handover moved a checkout")
	}
	if child.Run.Run.Fork == nil || child.Run.Run.Fork.SourceRunID != source.Run.Run.ID || len(child.Run.Run.Fork.ReuseRefs) != 4 {
		t.Fatalf("new Run lost source provenance: %+v", child.Run.Run.Fork)
	}
	if problem := refuse("project_continue_active_child", append([]string{"project", "continue", "--prepare"}, continueArgs...)...); !strings.Contains(problem, child.Run.Run.ID) {
		t.Fatalf("active child refusal omitted Run ID: %s", problem)
	}
	// A second, independent continuation at a named commit claims a new tree:
	// the source tree now belongs to the first one.
	atCommit := append(append([]string{}, continueArgs...), "--allow-duplicate-continuation", "--workspace-commit", drafted, "--workspace", "worktree")
	var independent projectLaunchSummary
	if err := json.Unmarshal([]byte(command(append([]string{"project", "continue", "--prepare"}, atCommit...)...)), &independent); err != nil || !independent.AllowDuplicateContinuation || independent.ReviewDigest == reviewed.ReviewDigest || independent.Continuation.WorkspaceCommit != drafted {
		t.Fatalf("explicit duplicate review is not distinct: %+v %v", independent, err)
	}
	command("--project", authority, "capacity", "set", "--capacity", "2", "--reason", "two continuations of one source")
	var second projectStartResult
	if err := json.Unmarshal([]byte(command(append(append([]string{"project", "continue"}, atCommit...), "--expected-launch-digest", independent.ReviewDigest)...)), &second); err != nil {
		t.Fatal(err)
	}
	if second.Workspace == nil || second.Workspace.ID == source.Workspace.ID || gitFixture(t, second.WorkspacePath, "rev-parse", "HEAD") != drafted {
		t.Fatalf("a named commit did not get its own tree: %+v", second.Workspace)
	}
	engine, err := prifly.Open(authority, true)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	view, err := engine.View(context.Background(), source.Run.Run.ID)
	if err != nil || view.RunVersion != reviewed.Continuation.Source.RunVersion || view.Run.Outcome == nil || *view.Run.Outcome != "partial" {
		t.Fatalf("source changed during continuation: %+v %v", view.Run.Outcome, err)
	}
	runs, err := engine.Runs(context.Background())
	if err != nil || len(runs) != 3 {
		t.Fatalf("prepare/refusal created another Run: %d %v", len(runs), err)
	}
	var first prifly.SessionTask
	if err := json.Unmarshal([]byte(command("--project", authority, "session", "task", "--run", child.Run.Run.ID)), &first); err != nil {
		t.Fatal(err)
	}
	if first.ClaimID != source.Workspace.ID || first.ClaimPath != child.Workspace.Path {
		t.Fatalf("read-only first gate is not bound to the handed-over claim: %+v", first)
	}
}
