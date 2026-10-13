package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func TestCLIRecoverNestedPreparationPreservesDirtyClaimAndHistory(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"call", "repeat"} {
		t.Run(kind, func(t *testing.T) {
			f := newContinuationCLIResuming(t, "  from_outcomes: [partial]\n", true, func(root string) {
				file := filepath.Join(root, ".prifly/workflows/source/workflow.yaml")
				data, err := os.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}
				text := strings.Replace(string(data), "max_step_instances: 3, max_control_transitions: 4", "max_step_instances: 8, max_control_transitions: 20, max_child_depth: 1", 1)
				text = strings.Replace(text, "  polish: \"{{step_polish}}\"", "  polish: \"{{step_polish}}\"\n  gates: \"{{workflow_gates}}\"", 1)
				stage := "  polish: {kind: call, workflow_ref: gates, input_bindings: {plan: $stages.draft.plan}, on: {partial: partial}}"
				if kind == "repeat" {
					stage = "  polish: {kind: repeat, body_workflow_ref: gates, initial_bindings: {plan: $stages.draft.plan}, next_bindings: {plan: $stages.draft.plan}, continue_on: [partial], until: {op: eq, left: true, right: true}, max_iterations: 2, on_complete: {partial: partial}, on_limit: partial}"
				}
				text = strings.Replace(text, "  polish: {kind: step, step_ref: polish, on: {pass: partial}, impossible_verdicts: [fail, needs_revision, no_work, blocked]}", stage, 1)
				writeFixtureFile(t, root, ".prifly/workflows/source/workflow.yaml", text)
				writeFixtureFile(t, root, ".prifly/workflows/source/workflows/gates.yaml", `authoring: prifly-workflow/1
schema_version: "6"
id: example:workflow/gates
version: 1.0.0
inputs: {plan: {schema_ref: "{{workspace_tree_manifest}}"}}
policy_ref: "{{local_policy}}"
entry: verify
limits: {max_step_instances: 1, max_control_transitions: 2}
stages:
  verify: {kind: step, step_ref: "{{step_polish}}", input_bindings: {plan: $inputs.plan}, on: {pass: partial}, impossible_verdicts: [fail, needs_revision, no_work, blocked]}
  partial: {kind: finish, outcome: partial}
`)
				file = filepath.Join(root, ".prifly/workflows/source/steps/polish.yaml")
				data, err = os.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}
				writeFixtureFile(t, root, ".prifly/workflows/source/steps/polish.yaml", string(data)+`inputs: {plan: {schema_ref: "{{workspace_tree_manifest}}"}}
workspace_trees:
  - input_port: plan
    capture: {kind: direct_child_file, path: plans}
`)
				file = filepath.Join(root, ".prifly/workflows/source/steps/draft.yaml")
				data, err = os.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}
				writeFixtureFile(t, root, ".prifly/workflows/source/steps/draft.yaml", strings.Replace(string(data), "kind: exact_file, path: plans/plan.md", "kind: direct_child_file, path: plans", 1))
				writeFixtureFile(t, root, "feature.txt", "base\n")
			})
			f.submit(map[string]string{"handoff": "{}\n"}, nil)
			writeFixtureFile(t, f.source.WorkspacePath, "plans/plan.md", "# Accepted plan\n")
			f.submitOnly(map[string]string{"draft": "{}\n"}, []prifly.WorkspaceTreeLocation{{OutputPort: "plan", Path: "plans/plan.md"}})
			writeFixtureFile(t, f.source.WorkspacePath, "plans/plan.md", "# Preserved later edit\n")
			writeFixtureFile(t, f.source.WorkspacePath, "feature.txt", "dirty tracked\n")
			writeFixtureFile(t, f.source.WorkspacePath, "unsaved.txt", "dirty untracked\n")
			sourceID := f.source.Run.Run.ID
			f.command("--project", f.authority, "run", "drive", sourceID)
			status := func(id string) prifly.RunView {
				var view prifly.RunView
				if err := json.Unmarshal([]byte(f.command("--project", f.authority, "run", "status", id)), &view); err != nil {
					t.Fatal(err)
				}
				return view
			}
			before := status(sourceID)
			if before.Run.Status != "failed" || len(before.Run.Attempts) != 2 {
				t.Fatalf("not pre-admission: %+v", before.Run.Diagnostics)
			}
			head, diff := gitFixture(t, f.source.WorkspacePath, "rev-parse", "HEAD"), gitFixture(t, f.source.WorkspacePath, "diff", "--binary")
			prepare := func(id, from string) projectLaunchSummary {
				args := []string{"--project", f.authority, "project", "recover", "--prepare", "--repository", f.root, "--launch", "source", "--host", "codex-cli", "--source-run", id}
				if from != "" {
					args = append(args, "--from-stage", from)
				}
				var reviewed projectLaunchSummary
				if err := json.Unmarshal([]byte(f.command(args...)), &reviewed); err != nil {
					t.Fatal(err)
				}
				return reviewed
			}
			natural, earlier := prepare(sourceID, ""), prepare(sourceID, "draft")
			if natural.Recovery.FrontierStageID != "polish" || len(natural.Recovery.Reused) != 2 || earlier.Recovery.FrontierStageID != "draft" || len(earlier.Recovery.Reused) != 1 {
				t.Fatalf("incorrect root prefix: %+v %+v", natural.Recovery, earlier.Recovery)
			}
			if natural.Recovery.CandidateRef != nil || natural.Recovery.FrontierAttemptID != "" || natural.Recovery.FrontierAction != "execute" {
				t.Fatal("invented Attempt result")
			}
			if after := status(sourceID); after.RunVersion != before.RunVersion || after.EventSequence != before.EventSequence || !reflect.DeepEqual(before.Run, after.Run) {
				t.Fatal("prepare changed source")
			}
			args := []string{"--project", f.authority, "project", "recover", "--repository", f.root, "--launch", "source", "--host", "codex-cli", "--source-run", sourceID}
			f.refuse("project_start_stale_launch", append(args, "--expected-launch-digest", "sha256:stale")...)
			var child projectStartResult
			if err := json.Unmarshal([]byte(f.command(append(args, "--expected-launch-digest", natural.ReviewDigest)...)), &child); err != nil {
				t.Fatal(err)
			}
			// The unchanged manifest still rejects the unrelated edit after takeover.
			failedChild := status(child.RunID)
			if failedChild.Run.Status != "failed" || len(failedChild.Run.Attempts) != 0 || failedChild.Run.Diagnostics[0].Code != "workspace_tree_input_drift" {
				t.Fatal("recovery bypassed drift guard")
			}
			if child.Workspace.ID != f.source.Workspace.ID || child.Workspace.Generation != f.source.Workspace.Generation+1 || child.WorkspacePath != f.source.WorkspacePath || gitFixture(t, child.WorkspacePath, "rev-parse", "HEAD") != head || gitFixture(t, child.WorkspacePath, "diff", "--binary") != diff {
				t.Fatal("takeover changed claim or tracked bytes")
			}
			for path, want := range map[string]string{"plans/plan.md": "# Preserved later edit\n", "unsaved.txt": "dirty untracked\n"} {
				data, err := os.ReadFile(filepath.Join(child.WorkspacePath, path))
				if err != nil || string(data) != want {
					t.Fatal("untracked bytes lost", path, err)
				}
			}
			if before.Run.Inputs["task"].Digest != failedChild.Run.Inputs["task"].Digest || !reflect.DeepEqual(before.Run.DecisionSheet, failedChild.Run.DecisionSheet) || !reflect.DeepEqual(before.Run.DecisionCatalog, failedChild.Run.DecisionCatalog) {
				t.Fatal("recovery changed task or answers")
			}
			if status(sourceID).EventSequence != before.EventSequence || status(sourceID).RunVersion != before.RunVersion {
				t.Fatal("start changed source journal")
			}
			inherited := prepare(child.RunID, "draft")
			if inherited.Recovery.FrontierStageID != "draft" || len(inherited.Recovery.Reused) != 1 {
				t.Fatal("inherited earlier root stage lost")
			}
			var earlierChild projectStartResult
			args = []string{"--project", f.authority, "project", "recover", "--repository", f.root, "--launch", "source", "--host", "codex-cli", "--source-run", child.RunID, "--from-stage", "draft", "--expected-launch-digest", inherited.ReviewDigest}
			if err := json.Unmarshal([]byte(f.command(args...)), &earlierChild); err != nil {
				t.Fatal(err)
			}
			var task prifly.SessionTask
			if err := json.Unmarshal([]byte(f.command("--project", f.authority, "session", "task", "--run", earlierChild.RunID)), &task); err != nil {
				t.Fatal(err)
			}
			issued := status(earlierChild.RunID).Run
			if stage := issued.Activations[issued.Steps[task.StepInstanceID].ActivationID].StageID; stage != "draft" {
				t.Fatalf("earlier stage not issued: %s", stage)
			}
			if task.Context.Inputs["handoff"].Ref.Digest != inherited.Recovery.RootOutputs["prepare"]["handoff"].Digest {
				t.Fatal("earlier stage lost accepted artifact bytes")
			}
			if task.WorkspaceTrees[0].InputManifest != nil {
				t.Fatal("new output inherited an unattributed old manifest")
			}
			writeFixtureFile(t, task.RepositoryWorkspace, "plans/recovered.md", "# New reviewed plan\n")
			slot := task.Context.Outputs["draft"]
			writeFixtureFile(t, task.Workspace, slot.Path, "{}\n")
			var submission prifly.SessionSubmission
			if err := json.Unmarshal([]byte(f.command("--project", f.authority, "session", "submit", "--template", "--run", earlierChild.RunID, "--attempt", task.AttemptID)), &submission); err != nil {
				t.Fatal(err)
			}
			result, err := json.Marshal(map[string]any{"schema_version": "1", "run_id": task.RunID, "step_instance_id": task.StepInstanceID, "attempt_id": task.AttemptID, "envelope_digest": task.EnvelopeDigest, "verdict": "pass", "outputs": map[string]prifly.ArtifactRef{"draft": {ArtifactID: slot.ArtifactID, Revision: slot.Revision, Digest: projectBytesDigest([]byte("{}\n"))}}, "evidence_refs": []any{}, "effect_receipt_refs": []any{}, "summary": "new captured plan"})
			if err != nil {
				t.Fatal(err)
			}
			submission.Result, submission.AnsweredQuestions = result, noQuestions(task)
			submission.WorkspaceTrees = []prifly.WorkspaceTreeLocation{{OutputPort: "plan", Path: "plans/recovered.md"}}
			report, err := json.Marshal(submission)
			if err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(t.TempDir(), "recovered-report.json")
			if err := os.WriteFile(file, report, 0600); err != nil {
				t.Fatal(err)
			}
			f.command("--project", f.authority, "session", "submit", "--file", file)
			f.command("--project", f.authority, "run", "drive", earlierChild.RunID)
			if status(earlierChild.RunID).Run.Status == "failed" {
				t.Fatal("new allowed plan did not reach nested worker")
			}
			// Output capture uses a new allowed location; old dirty bytes survive.
			if data, err := os.ReadFile(filepath.Join(task.RepositoryWorkspace, "plans/plan.md")); err != nil || string(data) != "# Preserved later edit\n" {
				t.Fatal("earlier preparation overwrote old plan")
			}
		})
	}
}
