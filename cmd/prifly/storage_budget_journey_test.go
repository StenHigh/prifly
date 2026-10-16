package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func storageBudgetCLI(t *testing.T, authority string, args ...string) (int, string, string) {
	t.Helper()
	binary := os.Getenv("PRIFLY_STORAGE_FIXTURE_BINARY")
	args = append([]string{"--project", authority}, args...)
	if binary == "" {
		return runCLI(t, args...)
	}
	var out, stderr bytes.Buffer
	command := exec.Command(binary, append(args, "--json")...)
	command.Stdout = &out
	command.Stderr = &stderr
	err := command.Run()
	if err == nil {
		return 0, out.String(), stderr.String()
	}
	if exit, ok := err.(*exec.ExitError); ok {
		return exit.ExitCode(), out.String(), stderr.String()
	}
	t.Fatal(err)
	return 0, "", ""
}

func budgetAuthorityHashes(t *testing.T, root string) map[string][32]byte {
	t.Helper()
	result := map[string][32]byte{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.Type().IsRegular() || strings.HasSuffix(path, "-shm") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		// SQLite may create an empty WAL sidecar when a read-only connection
		// first opens a clean WAL database. It contains no committed bytes.
		if len(data) == 0 && strings.HasSuffix(path, "state.sqlite3-wal") {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		result[relative] = sha256.Sum256(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestCLIStorageBudgetLegacyPrepareIsReadOnly(t *testing.T) {
	t.Parallel()
	old := os.Getenv("PRIFLY_STORAGE_BASELINE_BINARY")
	if old == "" {
		t.Skip("set PRIFLY_STORAGE_BASELINE_BINARY to a storage-9 binary")
	}
	authority := filepath.Join(t.TempDir(), "authority")
	if output, err := exec.Command(old, "init", authority, "--json").CombinedOutput(); err != nil {
		t.Fatal(err, string(output))
	}
	before := budgetAuthorityHashes(t, authority)
	for _, args := range [][]string{{"storage", "budget", "show"}, {"doctor", "--storage"}, {"storage", "budget", "set", "--bytes", "1073741824", "--reason", "keep accepted work", "--prepare"}} {
		code, out, stderr := storageBudgetCLI(t, authority, args...)
		if code != 0 {
			t.Fatal(args, code, stderr)
		}
		if !strings.Contains(out, `"storage_version":9`) && !strings.Contains(out, `"storage_version": 9`) {
			t.Fatal("legacy read silently upgraded storage", out)
		}
	}
	if after := budgetAuthorityHashes(t, authority); !reflect.DeepEqual(before, after) {
		t.Fatal("show/prepare changed persisted authority files", budgetHashChanges(before, after))
	}
	_, out, stderr := storageBudgetCLI(t, authority, "storage", "budget", "set", "--bytes", "1073741824", "--reason", "keep accepted work", "--prepare")
	var review prifly.StorageBudgetReview
	if err := json.Unmarshal([]byte(out), &review); err != nil {
		t.Fatal(err, stderr)
	}
	if review.StorageFromVersion != 9 || review.StorageToVersion != 10 || review.Status.Control.Source != "legacy_default" {
		t.Fatal(review)
	}
	// A stale protected payload refuses before a writer can migrate legacy data.
	code, _, stderr := storageBudgetCLI(t, authority, "storage", "budget", "set", "--bytes", "1073741824", "--reason", "changed", "--expected-review-digest", review.ReviewDigest, "--command", "command:legacy")
	if code == 0 || !strings.Contains(stderr, "storage_budget_review_conflict") {
		t.Fatal(code, stderr)
	}
	if after := budgetAuthorityHashes(t, authority); !reflect.DeepEqual(before, after) {
		t.Fatal("unreviewed legacy set migrated storage")
	}
	code, _, stderr = storageBudgetCLI(t, authority, "storage", "budget", "set", "--bytes", "1073741824", "--reason", review.Reason, "--expected-review-digest", review.ReviewDigest, "--command", "command:legacy")
	if code != 0 {
		t.Fatal(code, stderr)
	}
	if output, err := exec.Command(old, "--project", authority, "doctor", "--json").CombinedOutput(); err == nil || !strings.Contains(string(output), "unsupported_storage_version") {
		t.Fatal("old binary accepted migrated storage", string(output), err)
	}
}

func TestCLIStorageBudgetRecoveryPreservesSameRunAndWorkspace(t *testing.T) {
	t.Parallel()
	root, authority := singleLaunchFixture(t, 2<<20)
	folder := filepath.Join(root, ".prifly/workflows/single")
	stepPath := filepath.Join(folder, "steps/work.yaml")
	step, err := os.ReadFile(stepPath)
	if err != nil {
		t.Fatal(err)
	}
	step = bytes.ReplaceAll(step, []byte("effects: {class: none"), []byte("outputs: {report: {schema_ref: \"{{schema_object}}\", required_for: [pass]}}\neffects: {class: workspace_write"))
	if err := os.WriteFile(stepPath, step, 0600); err != nil {
		t.Fatal(err)
	}
	workflowPath := filepath.Join(folder, "workflow.yaml")
	workflow, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatal(err)
	}
	workflow = bytes.ReplaceAll(workflow, []byte("max_control_transitions: 2"), []byte("max_control_transitions: 4"))
	workflow = bytes.ReplaceAll(workflow, []byte("on: {pass: done}"), []byte("on: {pass: route}"))
	workflow = bytes.ReplaceAll(workflow, []byte("  done: {kind: finish"), []byte("  route:\n    kind: choice\n    selection: first_match\n    branches: [{id: continue, predicate: {op: eq, left: true, right: true}, next: done}]\n  done: {kind: finish"))
	if err := os.WriteFile(workflowPath, workflow, 0600); err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, root, "preserved.txt", "base\n")
	gitFixture(t, root, "add", ".")
	gitFixture(t, root, "commit", "-qm", "recovery fixture")
	baseArgs := []string{"--repository", root, "--launch", "single", "--host", "codex-cli", "--workspace", "worktree", "--input", "task=" + filepath.Join(root, "task.json")}
	// Setup uses the real CLI implementation in-process; the optional executable
	// handles storage review/set/drive without creating an unrelated monitor.
	code, out, stderr := runCLI(t, append([]string{"--project", authority, "project", "questionnaire"}, append(baseArgs, "--prepare")...)...)
	if code != 0 {
		t.Fatal(code, stderr)
	}
	var summary projectLaunchSummary
	if err := json.Unmarshal([]byte(out), &summary); err != nil {
		t.Fatal(err)
	}
	code, out, stderr = runCLI(t, append([]string{"--project", authority, "project", "start"}, append(baseArgs, "--expected-launch-digest", summary.ReviewDigest, "--allow-execution", "--command-id", "command:recovery-fixture")...)...)
	if code != 0 {
		t.Fatal(code, stderr)
	}
	var started projectStartResult
	if err := json.Unmarshal([]byte(out), &started); err != nil {
		t.Fatal(err)
	}
	runID := started.RunID
	code, out, stderr = runCLI(t, "--project", authority, "session", "task", "--run", runID)
	if code != 0 {
		t.Fatal(stderr)
	}
	var task prifly.SessionTask
	if err := json.Unmarshal([]byte(out), &task); err != nil {
		t.Fatal(err)
	}
	if task.RepositoryWorkspace == "" || task.ClaimID == "" {
		t.Fatal("fixture lacks real claim", task.RepositoryWorkspace, task.ClaimID)
	}
	dirty := filepath.Join(task.RepositoryWorkspace, "preserved.txt")
	untracked := filepath.Join(task.RepositoryWorkspace, "untracked.txt")
	for path, value := range map[string]string{dirty: "accepted dirty work\n", untracked: "untracked evidence\n"} {
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	report := []byte(`{"accepted":true}`)
	slot := task.Context.Outputs["report"]
	if err := os.WriteFile(filepath.Join(task.Workspace, slot.Path), report, 0600); err != nil {
		t.Fatal(err)
	}
	digest, err := flow.Digest(report)
	if err != nil {
		t.Fatal(err)
	}
	ref := prifly.ArtifactRef{ArtifactID: slot.ArtifactID, Revision: slot.Revision, Digest: digest}
	result := prifly.Result{SchemaVersion: "1", RunID: runID, StepInstanceID: task.StepInstanceID, AttemptID: task.AttemptID, EnvelopeDigest: task.EnvelopeDigest, Verdict: "pass", Outputs: map[string]prifly.ArtifactRef{"report": ref}, EvidenceRefs: []any{}, EffectReceiptRefs: []any{}, Summary: "accepted fixture producer"}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	submission, err := json.Marshal(prifly.SessionSubmission{SchemaVersion: task.SchemaVersion, RunID: runID, AttemptID: task.AttemptID, EnvelopeDigest: task.EnvelopeDigest, Result: encoded, AnsweredQuestions: noQuestions(task)})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "result.json")
	if err := os.WriteFile(path, submission, 0600); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runCLI(t, "--project", authority, "session", "submit", "--file", path); code != 0 {
		t.Fatal(code, stderr)
	}
	engine, err := prifly.Open(authority, false)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	before, err := engine.Store.Read(ctx, runID, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	var accepted prifly.Run
	if err := json.Unmarshal(before.Snapshot.Data, &accepted); err != nil {
		t.Fatal(err)
	}
	if accepted.Attempts[task.AttemptID].Accepted == nil {
		t.Fatal("producer is not accepted")
	}
	artifact, artifactBytes, err := engine.Artifact(ref)
	if err != nil {
		t.Fatal(err)
	}
	artifactRecord, _ := json.Marshal(artifact)
	claims, err := engine.Claims(ctx)
	if err != nil {
		t.Fatal(err)
	}
	padding, _ := json.Marshal(map[string]string{"fixture_padding": strings.Repeat("p", (2<<20)+(64<<10))})
	mutation, err := engine.Store.ApplyAuthority(ctx, local.AuthorityCommand{ID: "command:fixture-saturation", Actor: "fixture:owner", Key: "fixture:budget_padding", Payload: json.RawMessage(`{}`)}, func(local.AuthoritySnapshot) (local.AuthorityChange, error) {
		return local.AuthorityChange{Data: padding}, nil
	})
	if err != nil || mutation.Receipt.Rejection != nil {
		t.Fatal(err, mutation.Receipt.Rejection)
	}
	engine.Close()
	code, _, stderr = storageBudgetCLI(t, authority, "run", "drive", runID)
	if code != 5 || !strings.Contains(stderr, "storage_budget_exhausted") || !strings.Contains(stderr, "allocated_bytes=") {
		t.Fatal("frontier did not refuse quota with actual limits", code, stderr)
	}
	readonlyBefore := budgetAuthorityHashes(t, authority)
	code, out, stderr = storageBudgetCLI(t, authority, "storage", "budget", "set", "--bytes", "4194304", "--reason", "retain accepted work", "--prepare")
	if code != 0 {
		t.Fatal(code, stderr)
	}
	var review prifly.StorageBudgetReview
	if err := json.Unmarshal([]byte(out), &review); err != nil {
		t.Fatal(err)
	}
	if after := budgetAuthorityHashes(t, authority); !reflect.DeepEqual(readonlyBefore, after) {
		t.Fatal("read-only review wrote authority", budgetHashChanges(readonlyBefore, after))
	}
	args := []string{"storage", "budget", "set", "--bytes", "4194304", "--reason", review.Reason, "--expected-review-digest", review.ReviewDigest, "--command", "command:fixture-raise"}
	code, first, stderr := storageBudgetCLI(t, authority, args...)
	if code != 0 {
		t.Fatal(code, stderr)
	}
	code, retry, stderr := storageBudgetCLI(t, authority, args...)
	if code != 0 || retry != first {
		t.Fatal("exact retry did not return recorded result", code, stderr)
	}
	engine, err = prifly.Open(authority, false)
	if err != nil {
		t.Fatal(err)
	}
	afterSet, err := engine.Store.Read(ctx, runID, 0, 1)
	if err != nil || !bytes.Equal(before.Snapshot.Data, afterSet.Snapshot.Data) {
		t.Fatal("set changed Run frontier", err)
	}
	engine.Close()
	code, _, stderr = storageBudgetCLI(t, authority, "run", "drive", runID)
	if code != 0 {
		t.Fatal("same Run did not resume", code, stderr)
	}
	engine, err = prifly.Open(authority, true)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	final, err := engine.Store.Read(ctx, runID, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	var completed prifly.Run
	if err := json.Unmarshal(final.Snapshot.Data, &completed); err != nil {
		t.Fatal(err)
	}
	if completed.ID != runID || completed.Status != "completed" || len(completed.Attempts) != 1 || len(completed.Steps) != 1 {
		t.Fatal("recovery replayed producer or replaced Run", completed.Status, len(completed.Attempts))
	}
	priorAttempt, _ := json.Marshal(accepted.Attempts[task.AttemptID])
	finalAttempt, _ := json.Marshal(completed.Attempts[task.AttemptID])
	if !bytes.Equal(priorAttempt, finalAttempt) {
		t.Fatal("accepted attempt/provenance changed")
	}
	finalClaims, err := engine.Claims(ctx)
	if err != nil || !reflect.DeepEqual(claims, finalClaims) {
		t.Fatal("claim generation/identity changed", err)
	}
	finalArtifact, finalBytes, err := engine.Artifact(ref)
	finalRecord, _ := json.Marshal(finalArtifact)
	if err != nil || !bytes.Equal(artifactBytes, finalBytes) || !bytes.Equal(artifactRecord, finalRecord) {
		t.Fatal("accepted artifact evidence changed", err)
	}
	for path, value := range map[string]string{dirty: "accepted dirty work\n", untracked: "untracked evidence\n"} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != value {
			t.Fatal("Workspace evidence lost", path, err)
		}
	}
	t.Logf("same Run %s recovered; one producer; claim %s generation %d; accepted artifact %s; dirty/untracked and provenance preserved", runID, task.ClaimID, task.ClaimGeneration, ref.Digest)
}

func budgetHashChanges(before, after map[string][32]byte) []string {
	out := []string{}
	for k, v := range before {
		if after[k] != v {
			out = append(out, "changed/removed "+k)
		}
	}
	for k := range after {
		if _, ok := before[k]; !ok {
			out = append(out, "added "+k)
		}
	}
	return out
}
