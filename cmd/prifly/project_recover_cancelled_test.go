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

func TestCLIReviewedCancelledWriterRecovery(t *testing.T) {
	t.Parallel()
	for _, writer := range []string{"producer", "consumer"} {
		for _, outcome := range []string{"applied", "not_applied"} {
			t.Run(writer+"/"+outcome, func(t *testing.T) {
				f := newContinuationCLIResuming(t, "  from_outcomes: [partial]\n", true, func(root string) {
					path := filepath.Join(root, ".prifly/workflows/source/steps/polish.yaml")
					data, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					writeFixtureFile(t, root, ".prifly/workflows/source/steps/quality.yaml", strings.Replace(string(data), "id: example:step/polish", "id: example:step/quality", 1))
					text := strings.Replace(string(data), "class: none", "class: workspace_write", 1)
					text += "outputs: {final: {schema_ref: \"{{workspace_tree_manifest}}\", required_for: [pass]}}\ninputs: {plan: {schema_ref: \"{{workspace_tree_manifest}}\"}}\nworkspace_trees:\n  - input_port: plan\n    output_port: final\n    capture: {kind: exact_file, path: plans/plan.md}\n"
					writeFixtureFile(t, root, ".prifly/workflows/source/steps/polish.yaml", text)
					path = filepath.Join(root, ".prifly/workflows/source/workflow.yaml")
					data, err = os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					text = strings.Replace(string(data), "max_step_instances: 3, max_control_transitions: 4", "max_step_instances: 4, max_control_transitions: 5", 1)
					text = strings.Replace(text, "  polish: \"{{step_polish}}\"", "  polish: \"{{step_polish}}\"\n  quality: \"{{step_quality}}\"", 1)
					text = strings.Replace(text, "resumable: {from_outcomes: [partial]}", "resumable: {from_outcomes: [partial], from_cancelled: true}", 1)
					text = strings.Replace(text, "polish: {kind: step, step_ref: polish, on: {pass: partial}", "polish: {kind: step, step_ref: polish, input_bindings: {plan: $stages.draft.plan}, on: {pass: quality}", 1)
					text = strings.Replace(text, "  partial: {kind: finish", "  quality: {kind: step, step_ref: quality, on: {pass: partial}, impossible_verdicts: [fail, needs_revision, no_work, blocked]}\n  partial: {kind: finish", 1)
					writeFixtureFile(t, root, ".prifly/workflows/source/workflow.yaml", text)
					writeFixtureFile(t, root, "feature.txt", "base\n")
				})
				status := func(id string) prifly.RunView {
					t.Helper()
					var v prifly.RunView
					if err := json.Unmarshal([]byte(f.command("--project", f.authority, "run", "status", id)), &v); err != nil {
						t.Fatal(err)
					}
					return v
				}
				task := func(id string) prifly.SessionTask {
					t.Helper()
					var v prifly.SessionTask
					if err := json.Unmarshal([]byte(f.command("--project", f.authority, "session", "task", "--run", id)), &v); err != nil {
						t.Fatal(err)
					}
					return v
				}
				prepareArgs := func(id string) []string {
					return []string{"--project", f.authority, "project", "continue", "--repository", f.root, "--launch", "source", "--host", "codex-cli", "--source-run", id}
				}
				prepare := func(id string) projectLaunchSummary {
					t.Helper()
					var v projectLaunchSummary
					if err := json.Unmarshal([]byte(f.command(append(prepareArgs(id), "--prepare")...)), &v); err != nil {
						t.Fatal(err)
					}
					return v
				}
				start := func(id string, review projectLaunchSummary) projectStartResult {
					t.Helper()
					var v projectStartResult
					if err := json.Unmarshal([]byte(f.command(append(prepareArgs(id), "--expected-launch-digest", review.ReviewDigest)...)), &v); err != nil {
						t.Fatal(err)
					}
					return v
				}
				source := f.source.RunID
				f.submit(map[string]string{"handoff": "{}\n"}, nil)
				planBytes := "# Partial plan\n"
				writeFixtureFile(t, f.source.WorkspacePath, "plans/plan.md", planBytes)
				if writer == "consumer" {
					planBytes = "# Accepted plan\n"
					writeFixtureFile(t, f.source.WorkspacePath, "plans/plan.md", planBytes)
					f.submit(map[string]string{"draft": "{}\n"}, []prifly.WorkspaceTreeLocation{{OutputPort: "plan", Path: "plans/plan.md"}})
				}
				active := task(source)
				writeFixtureFile(t, f.source.WorkspacePath, "unsaved.txt", "owner bytes\n")
				writeFixtureFile(t, f.source.WorkspacePath, "feature.txt", "owner edit\n")
				head, diff := gitFixture(t, f.source.WorkspacePath, "rev-parse", "HEAD"), gitFixture(t, f.source.WorkspacePath, "diff", "--binary")
				cancelResolve := func(id string, a prifly.SessionTask) {
					t.Helper()
					f.command("--project", f.authority, "run", "cancel", id, "--reason", "stop synchronous fixture writer")
					f.refuse("recovery_required", "--project", f.authority, "run", "drive", id)
					f.refuse("recover_source_unsettled", append(prepareArgs(id), "--prepare")...)
					f.command("--project", f.authority, "run", "resolve", id, "--attempt", a.AttemptID, "--outcome", outcome, "--reason", "actor returned; inspected workspace effects")
					f.command("--project", f.authority, "run", "drive", id)
				}
				cancelResolve(source, active)
				before := status(source)
				if before.Run.Status != "failed" || !before.Run.CancelRequested || before.Run.HasUnresolvedEffects || before.Run.Attempts[active.AttemptID].Accepted != nil {
					t.Fatal("resolution invented success or lost cancellation")
				}
				// A changed accepted contract cannot be carried merely because the writer was resolved.
				prefixFile := filepath.Join(f.root, ".prifly/workflows/source/steps/prepare.yaml")
				prefixBytes, err := os.ReadFile(prefixFile)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(prefixFile, append(append([]byte{}, prefixBytes...), []byte("title: Changed accepted prefix\n")...), 0600); err != nil {
					t.Fatal(err)
				}
				f.refuse("recover_prefix_changed", append(prepareArgs(source), "--prepare")...)
				if err := os.WriteFile(prefixFile, prefixBytes, 0600); err != nil {
					t.Fatal(err)
				}
				review := prepare(source)
				if review.Recovery == nil || review.Recovery.FrontierAction != "execute" || review.Recovery.Claim.ID != f.source.Workspace.ID {
					t.Fatalf("not reviewed execute: %+v", review.Recovery)
				}
				if after := status(source); after.RunVersion != before.RunVersion || after.EventSequence != before.EventSequence || !reflect.DeepEqual(before.Run, after.Run) {
					t.Fatal("prepare changed source")
				}
				f.refuse("project_start_stale_launch", append(prepareArgs(source), "--expected-launch-digest", "sha256:stale")...)
				child := start(source, review)
				if child.Workspace.ID != f.source.Workspace.ID || child.Workspace.Generation != f.source.Workspace.Generation+1 || child.WorkspacePath != f.source.WorkspacePath {
					t.Fatal("recovery replaced claim")
				}
				if head != gitFixture(t, child.WorkspacePath, "rev-parse", "HEAD") || diff != gitFixture(t, child.WorkspacePath, "diff", "--binary") {
					t.Fatal("recovery changed HEAD or tracked bytes")
				}
				assertBytes := func() {
					t.Helper()
					for path, want := range map[string]string{"plans/plan.md": planBytes, "unsaved.txt": "owner bytes\n"} {
						data, err := os.ReadFile(filepath.Join(child.WorkspacePath, path))
						if err != nil || string(data) != want {
							t.Fatalf("lost %s: %q %v", path, data, err)
						}
					}
				}
				assertBytes()
				childView := status(child.RunID)
				if childView.Run.Inputs["task"].Digest != before.Run.Inputs["task"].Digest || !reflect.DeepEqual(childView.Run.DecisionSheet, before.Run.DecisionSheet) || !reflect.DeepEqual(childView.Run.DecisionCatalog, before.Run.DecisionCatalog) {
					t.Fatal("recovery changed source task/decisions")
				}
				if writer == "producer" {
					if childView.Run.Status != "failed" || len(childView.Run.Attempts) != 0 || len(childView.Run.Diagnostics) != 1 || childView.Run.Diagnostics[0].Code != "workspace_tree_output_exists" || !strings.Contains(childView.Run.Diagnostics[0].Message, "plans/plan.md") {
						t.Fatalf("occupied producer output admitted: %+v", childView.Run.Diagnostics)
					}
					if err := os.Rename(filepath.Join(child.WorkspacePath, "plans/plan.md"), filepath.Join(child.WorkspacePath, "partial-plan.saved")); err != nil {
						t.Fatal(err)
					}
					child = start(child.RunID, prepare(child.RunID))
				} else {
					// A second cancellation must prove its own resolution and carry the inherited prefix.
					cancelResolve(child.RunID, task(child.RunID))
					again := prepare(child.RunID)
					if len(again.Recovery.Reused) != len(review.Recovery.Reused) {
						t.Fatal("chained recovery lost prefix")
					}
					child = start(child.RunID, again)
				}
				// Submit fresh results for the repeated writer and every stage after it.
				for attempts := 0; ; attempts++ {
					if attempts > 4 {
						t.Fatal("recovery did not finish its bounded tail")
					}
					v := status(child.RunID)
					if v.Run.Status == "completed" {
						break
					}
					a := task(child.RunID)
					refs := map[string]prifly.ArtifactRef{}
					for port, slot := range a.Context.Outputs {
						bytes := "{}\n"
						writeFixtureFile(t, a.Workspace, slot.Path, bytes)
						refs[port] = prifly.ArtifactRef{ArtifactID: slot.ArtifactID, Revision: slot.Revision, Digest: projectBytesDigest([]byte(bytes))}
					}
					trees := []prifly.WorkspaceTreeLocation{}
					for _, binding := range a.WorkspaceTrees {
						if binding.InputPort == "" && binding.OutputPort != "" {
							writeFixtureFile(t, child.WorkspacePath, "plans/plan.md", "# Completed plan\n")
							trees = append(trees, prifly.WorkspaceTreeLocation{OutputPort: binding.OutputPort, Path: "plans/plan.md"})
						}
					}
					result, err := json.Marshal(prifly.Result{SchemaVersion: "1", RunID: a.RunID, StepInstanceID: a.StepInstanceID, AttemptID: a.AttemptID, EnvelopeDigest: a.EnvelopeDigest, Verdict: "pass", Outputs: refs, EvidenceRefs: []any{}, EffectReceiptRefs: []any{}, Summary: "fresh execution"})
					if err != nil {
						t.Fatal(err)
					}
					bytes, err := json.Marshal(prifly.SessionSubmission{SchemaVersion: a.SchemaVersion, RunID: a.RunID, AttemptID: a.AttemptID, EnvelopeDigest: a.EnvelopeDigest, Result: result, WorkspaceTrees: trees, AnsweredQuestions: noQuestions(a)})
					if err != nil {
						t.Fatal(err)
					}
					file := filepath.Join(t.TempDir(), "submission.json")
					if err := os.WriteFile(file, bytes, 0600); err != nil {
						t.Fatal(err)
					}
					f.command("--project", f.authority, "session", "submit", "--file", file)
					f.command("--project", f.authority, "run", "drive", child.RunID)
				}
				if writer == "producer" {
					saved, err := os.ReadFile(filepath.Join(child.WorkspacePath, "partial-plan.saved"))
					if err != nil || string(saved) != planBytes {
						t.Fatal("owner's saved partial was lost")
					}
				}
				final := status(child.RunID)
				if final.Run.CancelRequested || final.Run.Outcome == nil || *final.Run.Outcome != "partial" || final.Run.Recovery.FrontierAction != "execute" {
					t.Fatal("bad recovered settlement")
				}
				stages := map[string]bool{}
				for _, a := range final.Run.Activations {
					stages[a.StageID] = true
				}
				if !stages["polish"] || !stages["quality"] {
					t.Fatal("writer or quality was not executed again")
				}
				if after := status(source); after.RunVersion != before.RunVersion || after.EventSequence != before.EventSequence || !reflect.DeepEqual(before.Run, after.Run) {
					t.Fatal("recovery changed original source")
				}
			})
		}
	}
}
