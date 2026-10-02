package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func submitResumeChain(t *testing.T, f continuationCLI, run, verdict string) prifly.SessionTask {
	t.Helper()
	var task prifly.SessionTask
	if err := json.Unmarshal([]byte(f.command("--project", f.authority, "session", "task", "--run", run)), &task); err != nil {
		t.Fatal(err)
	}
	result, err := json.Marshal(map[string]any{"schema_version": "1", "run_id": run, "step_instance_id": task.StepInstanceID, "attempt_id": task.AttemptID, "envelope_digest": task.EnvelopeDigest, "verdict": verdict, "outputs": map[string]any{}, "evidence_refs": []any{}, "effect_receipt_refs": []any{}, "summary": "chain gate"})
	if err != nil {
		t.Fatal(err)
	}
	report, err := json.Marshal(prifly.SessionSubmission{SchemaVersion: task.SchemaVersion, RunID: run, AttemptID: task.AttemptID, EnvelopeDigest: task.EnvelopeDigest, Result: result, AnsweredQuestions: noQuestions(task)})
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "report.json")
	if err := os.WriteFile(file, report, 0600); err != nil {
		t.Fatal(err)
	}
	f.command("--project", f.authority, "session", "submit", "--file", file)
	f.command("--project", f.authority, "run", "drive", run)
	return task
}

func TestCLIResumeChainNestedDirtyClaim(t *testing.T) {
	t.Parallel()
	f := newContinuationCLIResuming(t, "  from_outcomes: [partial]\n", true, func(root string) {
		file := filepath.Join(root, ".prifly/workflows/source/workflow.yaml")
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		text := strings.Replace(string(data), "max_step_instances: 3, max_control_transitions: 4", "max_step_instances: 8, max_control_transitions: 20, max_child_depth: 1", 1)
		text = strings.Replace(text, "  polish: \"{{step_polish}}\"", "  polish: \"{{step_polish}}\"\n  gates: \"{{workflow_gates}}\"", 1)
		text = strings.Replace(text, "  polish: {kind: step, step_ref: polish, on: {pass: partial}, impossible_verdicts: [fail, needs_revision, no_work, blocked]}", "  polish: {kind: call, workflow_ref: gates, on: {partial: partial, succeeded: tail}}\n  tail: {kind: step, step_ref: polish, on: {pass: partial}, impossible_verdicts: [fail, needs_revision, no_work, blocked]}", 1)
		writeFixtureFile(t, root, ".prifly/workflows/source/workflow.yaml", text)
		writeFixtureFile(t, root, ".prifly/workflows/source/workflows/gates.yaml", `authoring: prifly-workflow/1
schema_version: "6"
id: example:workflow/gates
version: 1.0.0
policy_ref: "{{local_policy}}"
entry: verify
limits: {max_step_instances: 3, max_control_transitions: 5}
stages:
  verify: {kind: step, step_ref: "{{step_polish}}", on: {pass: fix}, impossible_verdicts: [fail, needs_revision, no_work, blocked]}
  fix: {kind: step, step_ref: "{{step_fix}}", on: {pass: verify-again}, impossible_verdicts: [fail, needs_revision, no_work, blocked]}
  verify-again: {kind: step, step_ref: "{{step_polish}}", on: {pass: partial, no_work: done}, impossible_verdicts: [fail, needs_revision, blocked]}
  partial: {kind: finish, outcome: partial}
  done: {kind: finish, outcome: succeeded}
`)
		data, err = os.ReadFile(filepath.Join(root, ".prifly/workflows/source/steps/polish.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		writeFixtureFile(t, root, ".prifly/workflows/source/steps/fix.yaml", strings.Replace(strings.Replace(string(data), "example:step/polish", "example:step/fix", 1), "class: none", "class: workspace_write", 1))
		writeFixtureFile(t, root, "feature.txt", "base\n")
	})
	f.submit(map[string]string{"handoff": "{}\n"}, nil)
	writeFixtureFile(t, f.source.WorkspacePath, "plans/plan.md", "# Plan\n")
	f.submit(map[string]string{"draft": "{}\n"}, []prifly.WorkspaceTreeLocation{{OutputPort: "plan", Path: "plans/plan.md"}})
	a := f.source.Run.Run.ID
	for i := 0; i < 3; i++ {
		submitResumeChain(t, f, a, "pass")
	}
	writeFixtureFile(t, f.source.WorkspacePath, "feature.txt", "dirty tracked\n")
	writeFixtureFile(t, f.source.WorkspacePath, "unsaved.txt", "dirty untracked\n")
	baseline := gitFixture(t, f.source.WorkspacePath, "diff", "--binary")
	head := gitFixture(t, f.source.WorkspacePath, "rev-parse", "HEAD")
	status := func(run string) prifly.RunView {
		t.Helper()
		var v prifly.RunView
		if err := json.Unmarshal([]byte(f.command("--project", f.authority, "run", "status", run)), &v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	resume := func(source, from string) projectStartResult {
		t.Helper()
		before := status(source)
		args := []string{"--repository", f.root, "--launch", "source", "--host", "codex-cli", "--source-run", source}
		if from != "" {
			args = append(args, "--from-stage", from)
		}
		var review, again projectLaunchSummary
		if err := json.Unmarshal([]byte(f.command(append([]string{"project", "continue", "--prepare"}, args...)...)), &review); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(f.command(append([]string{"project", "continue", "--prepare"}, args...)...)), &again); err != nil {
			t.Fatal(err)
		}
		if review.ReviewDigest != again.ReviewDigest || status(source).RunVersion != before.RunVersion {
			t.Fatal("prepare changed source or digest")
		}
		if source != a && from == "" && len(review.Recovery.Reused) != 7 {
			t.Fatalf("lost early or nested prefix: %+v", review.Recovery.Reused)
		}
		if from == "polish" && len(review.Recovery.Reused) != 2 {
			t.Fatalf("from-stage retained caller or descendants: %+v", review.Recovery.Reused)
		}
		f.refuse("project_start_stale_launch", append(append([]string{"project", "continue"}, args...), "--expected-launch-digest", "sha256:stale")...)
		var child projectStartResult
		command := append(append([]string{"project", "continue"}, args...), "--expected-launch-digest", review.ReviewDigest, "--command-id", "chain-"+source)
		if err := json.Unmarshal([]byte(f.command(command...)), &child); err != nil {
			t.Fatal(err)
		}
		// Project rechecks the source before runtime receipt dedup: after handover
		// its active child is the safe answer, never an independently created Run.
		f.refuse("project_continue_active_child", command...)
		if child.Workspace.ID != f.source.Workspace.ID || child.Workspace.RunID != child.RunID {
			t.Fatal("transferred another claim")
		}
		if gitFixture(t, child.WorkspacePath, "diff", "--binary") != baseline || gitFixture(t, child.WorkspacePath, "rev-parse", "HEAD") != head {
			t.Fatal("dirty tracked work or HEAD changed before write Attempt")
		}
		if data, err := os.ReadFile(filepath.Join(child.WorkspacePath, "unsaved.txt")); err != nil || string(data) != "dirty untracked\n" {
			t.Fatal("untracked bytes lost")
		}
		if !reflect.DeepEqual(before.Run.Recovery, status(source).Run.Recovery) || status(source).EventSequence != before.EventSequence {
			t.Fatal("source evidence changed")
		}
		f.refuse("project_continue_active_child", append([]string{"project", "continue", "--prepare"}, args...)...)
		return child
	}
	b := resume(a, "")
	submitResumeChain(t, f, b.RunID, "pass")
	submitResumeChain(t, f, b.RunID, "pass")
	submitResumeChain(t, f, b.RunID, "no_work")
	submitResumeChain(t, f, b.RunID, "pass")
	c := resume(b.RunID, "")
	if task := submitResumeChain(t, f, c.RunID, "pass"); len(task.Context.Inputs) != 0 {
		t.Fatal("tail got undeclared inputs")
	}
	f.refuse("resume_from_stage_invalid", "project", "continue", "--prepare", "--repository", f.root, "--launch", "source", "--host", "codex-cli", "--source-run", c.RunID, "--from-stage", "fix")
	f.refuse("resume_prefix_empty", "project", "continue", "--prepare", "--repository", f.root, "--launch", "source", "--host", "codex-cli", "--source-run", c.RunID, "--from-stage", "prepare")
	// Two reviewed starts compete for the same current claim. Exactly one
	// creates a Run; the losing pin cannot transfer it or publish an orphan.
	args := []string{"--repository", f.root, "--launch", "source", "--host", "codex-cli", "--source-run", c.RunID, "--from-stage", "polish"}
	var review projectLaunchSummary
	if err := json.Unmarshal([]byte(f.command(append([]string{"project", "continue", "--prepare"}, args...)...)), &review); err != nil {
		t.Fatal(err)
	}
	if len(review.Recovery.Reused) != 2 {
		t.Fatal("from-stage retained nested descendants")
	}
	type startAnswer struct {
		code        int
		out, stderr string
	}
	answers := make(chan startAnswer, 2)
	ready := make(chan struct{})
	for _, id := range []string{"concurrent-resume-one", "concurrent-resume-two"} {
		go func(id string) {
			<-ready
			code, out, stderr := runCLI(t, append(append([]string{"project", "continue"}, args...), "--expected-launch-digest", review.ReviewDigest, "--command-id", id)...)
			answers <- startAnswer{code, out, stderr}
		}(id)
	}
	close(ready)
	var d projectStartResult
	created := 0
	for range 2 {
		answer := <-answers
		if answer.code == 0 {
			created++
			if err := json.Unmarshal([]byte(answer.out), &d); err != nil {
				t.Fatal(err)
			}
		} else {
			guarded := false
			for _, code := range []string{"project_continue_active_child", "control_conflict", "claim_run_conflict", "recover_workspace_missing", "project_continue_stale_workspace", "project_start_stale_launch", "recover_plan_stale", "claim_generation_conflict"} {
				guarded = guarded || strings.Contains(answer.stderr, code)
			}
			if !guarded {
				t.Fatalf("untyped or unexpected losing start: %d %s", answer.code, answer.stderr)
			}
		}
	}
	if created != 1 || d.Workspace.ID != f.source.Workspace.ID || d.Workspace.RunID != d.RunID {
		t.Fatal("concurrent start did not have exactly one claim owner")
	}
	if gitFixture(t, d.WorkspacePath, "diff", "--binary") != baseline || gitFixture(t, d.WorkspacePath, "rev-parse", "HEAD") != head {
		t.Fatal("concurrent transfer changed tracked work")
	}
	if data, err := os.ReadFile(filepath.Join(d.WorkspacePath, "unsaved.txt")); err != nil || string(data) != "dirty untracked\n" {
		t.Fatal("concurrent transfer lost untracked bytes")
	}

	if d.Workspace.Generation != f.source.Workspace.Generation+3 {
		t.Fatal("claim did not transfer from each immediate source")
	}
	e, err := prifly.Open(f.authority, true)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	runs, err := e.Runs(context.Background())
	if err != nil || len(runs) != 4 {
		t.Fatal("prepare or refusals created extra Runs")
	}
}
