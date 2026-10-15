package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/projectwire"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func queueCLIRequest(t *testing.T, root, title string) string {
	t.Helper()
	data, err := json.MarshalIndent(prifly.TaskInput{SchemaVersion: prifly.TaskInputVersion, ID: "task:local-request", Title: title, RawText: "Original request\n  preserve spaces and text.\n", DesiredOutcome: "Inspect the chosen value", InScope: []string{"local only"}, OutOfScope: []string{}, CompletionCriteria: []string{"record an explicit local decision"}, Source: prifly.TaskSource{Type: "local", ExternalID: "fixture:queue", FetchedAt: "2026-10-15T00:00:00Z"}, SourceRefs: []prifly.ArtifactRef{}, Assumptions: []string{}, Confirmation: "explicit"}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "queue-request.json")
	if err := os.WriteFile(path, append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
func queueCLICommand(t *testing.T, args ...string) string {
	t.Helper()
	code, out, stderr := runCLI(t, args...)
	if code != 0 {
		t.Fatalf("%v: %d %s", args, code, stderr)
	}
	return out
}
func queueCLISummary(t *testing.T, raw string) prifly.QueueTaskSummary {
	t.Helper()
	var command projectwire.QueueCommand
	if err := json.Unmarshal([]byte(raw), &command); err != nil {
		t.Fatal(err)
	}
	validateQueueCLI(t, "ProjectQueueCommand", raw)
	var task prifly.QueueTaskSummary
	if err := json.Unmarshal(command.Result, &task); err != nil {
		t.Fatal(err)
	}
	return task
}
func validateQueueCLI(t *testing.T, name, raw string) {
	t.Helper()
	schema, err := prifly.PublicSchema(name)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := flow.Digest(schema)
	if err != nil {
		t.Fatal(err)
	}
	ref := flow.Ref{ID: "test:schema/" + name, Version: "1.0.0", Digest: digest}
	if err := flow.ValidateSchema(flow.Registry{ref: schema}, ref, []byte(raw)); err != nil {
		t.Fatalf("actual %s response violates its schema: %v", name, err)
	}
}
func queueSelectionArgs(task prifly.QueueTaskSummary) []string {
	return []string{"--task-id", task.ID, "--task-owner", task.Claim.Owner, "--task-revision", "1", "--task-claim-generation", "1"}
}

func TestCLIProjectQueueReviewRejectsStaleTaskBeforePreflightAndReplaysResult(t *testing.T) {
	t.Parallel()
	root, authority := t.TempDir(), filepath.Join(t.TempDir(), "authority")
	queueCLICommand(t, "project", "init", "--repository", root, "--state-root", authority, "--host", "codex-cli")
	writeProjectLaunchRequirementsFixture(t, root, "3", "none")
	profilePath := filepath.Join(root, ".prifly/project.yaml")
	profile, _ := os.ReadFile(profilePath)
	profile = []byte(strings.Replace(string(profile), "    workflow: .prifly/workflows/inspect/workflow.yaml\n", "    workflow: .prifly/workflows/inspect/workflow.yaml\n    preflight: {executable: shell, args: [preflight.sh], timeout_ms: 5000}\n", 1))
	if err := os.WriteFile(profilePath, profile, 0600); err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, root, "preflight.sh", "#!/bin/sh\necho dispatched >> preflight-count\n")
	queueCLICommand(t, "project", "local", "set", "--repository", root, "--allow-executable", "shell=/bin/sh")
	prefix := []string{"project", "queue"}
	repo := []string{"--repository", root}
	list := queueCLICommand(t, append(append(prefix, "list"), repo...)...)
	var initial prifly.TaskQueueView
	json.Unmarshal([]byte(list), &initial)
	if initial.Initialized || len(initial.Tasks) != 0 {
		t.Fatal("readonly list enrolled queue")
	}
	input := queueCLIRequest(t, root, "first request")
	original, _ := os.ReadFile(input)
	queueCLISummary(t, queueCLICommand(t, "project", "queue", "add", "--repository", root, "--input", input, "--command-id", "queue:add"))
	task := queueCLISummary(t, queueCLICommand(t, "project", "queue", "claim-next", "--repository", root, "--owner", "host:one", "--command-id", "queue:claim"))
	args := append([]string{"--repository", root, "--launch", "inspect", "--host", "codex-cli"}, queueSelectionArgs(task)...)
	plain := queueCLICommand(t, append([]string{"project", "questionnaire"}, args...)...)
	var questionnaire projectQuestionnaire
	json.Unmarshal([]byte(plain), &questionnaire)
	if questionnaire.SchemaVersion != "project-questionnaire/4" {
		t.Fatal("plain questionnaire contract changed")
	}
	rawReview := queueCLICommand(t, append([]string{"project", "questionnaire", "--prepare"}, args...)...)
	validateQueueCLI(t, "ProjectLaunchSummaryV4", rawReview)
	var old projectLaunchSummary
	json.Unmarshal([]byte(rawReview), &old)
	noID := append(append([]string{"project", "start"}, args...), "--expected-launch-digest", old.ReviewDigest)
	missingCode, _, missingErr := runCLI(t, noID...)
	if missingCode == 0 || !strings.Contains(missingErr, "explicit stable --command-id") {
		t.Fatalf("queue start manufactured caller identity: %d %s", missingCode, missingErr)
	}

	if old.QueueTask == nil || old.QueueTask.TaskID != task.ID {
		t.Fatal("review lost queue selection")
	}
	reader, err := prifly.Open(authority, true)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := reader.Runs(context.Background())
	reader.Close()
	if err != nil || len(runs) != 0 {
		t.Fatal("claim/questionnaire/prepare created a Run")
	}
	input = queueCLIRequest(t, root, "updated request")
	update := append([]string{"project", "queue", "update", "--repository", root, "--input", input, "--command-id", "queue:update"}, queueSelectionArgs(task)...)
	queueCLISummary(t, queueCLICommand(t, update...))
	stale := append(append([]string{"project", "start"}, args...), "--command-id", "queue:start-stale", "--expected-launch-digest", old.ReviewDigest)
	code, _, stderr := runCLI(t, stale...)
	if code == 0 || !strings.Contains(stderr, `"code":"queue_task_revision_conflict"`) {
		t.Fatalf("stale review started: %d %s", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(root, "preflight-count")); !os.IsNotExist(err) {
		t.Fatal("stale selection dispatched preflight")
	}
	for i := range args {
		if args[i] == "--task-revision" {
			args[i+1] = "2"
		}
	}
	rawReview = queueCLICommand(t, append([]string{"project", "questionnaire", "--prepare"}, args...)...)
	validateQueueCLI(t, "ProjectLaunchSummaryV4", rawReview)
	var review projectLaunchSummary
	json.Unmarshal([]byte(rawReview), &review)
	startArgs := append(append([]string{"project", "start"}, args...), "--command-id", "queue:start", "--expected-launch-digest", review.ReviewDigest)
	first := queueCLICommand(t, startArgs...)
	validateQueueCLI(t, "ProjectStartV4", first)
	var started projectStartResult
	if err := json.Unmarshal([]byte(first), &started); err != nil {
		t.Fatal(err)
	}
	if started.Run.Run.SchemaVersion != prifly.CoreQueueStateVersion || started.QueueTask == nil || started.Run.Run.QueueTask == nil || *started.QueueTask != *started.Run.Run.QueueTask {
		t.Fatal("actual start did not seal queue binding")
	}
	updated, _ := os.ReadFile(input)
	if !bytes.Equal(started.Run.Run.QueueRequestBytes, updated) || bytes.Equal(started.Run.Run.QueueRequestBytes, original) {
		t.Fatal("Run did not keep the selected exact request revision")
	}
	reader, err = prifly.Open(authority, true)
	if err != nil {
		t.Fatal(err)
	}
	_, typed, err := reader.Artifact(started.Run.Run.Inputs["value"])
	reader.Close()
	if err != nil || string(typed) != "3" {
		t.Fatalf("queue prose replaced typed/default input: %s %v", typed, err)
	}
	// Exact retries precede mutable source/program reads and never preflight again.
	if err := os.Remove(filepath.Join(root, "preflight.sh")); err != nil {
		t.Fatal(err)
	}
	second := queueCLICommand(t, startArgs...)
	var replay projectStartResult
	json.Unmarshal([]byte(second), &replay)
	if !reflect.DeepEqual(started, replay) {
		t.Fatal("retry changed original start result")
	}
	count, _ := os.ReadFile(filepath.Join(root, "preflight-count"))
	if string(count) != "dispatched\n" {
		t.Fatalf("preflight dispatch count: %q", count)
	}
	changed := append(append([]string{}, startArgs...), "--allow-execution")
	code, _, stderr = runCLI(t, changed...)
	if code == 0 || !strings.Contains(stderr, `"code":"command_conflict"`) {
		t.Fatalf("changed payload replay: %d %s", code, stderr)
	}
	omitted := []string{}
	for i := 0; i < len(startArgs); i++ {
		if strings.HasPrefix(startArgs[i], "--task-") {
			i++
			continue
		}
		omitted = append(omitted, startArgs[i])
	}
	code, _, stderr = runCLI(t, omitted...)
	if code == 0 || !strings.Contains(stderr, `"code":"command_conflict"`) {
		t.Fatalf("omitted queue selection escaped replay guard: %d %s", code, stderr)
	}

	show := queueCLICommand(t, "project", "queue", "show", "--repository", root, "--task-id", task.ID)
	var view prifly.QueueTaskView
	json.Unmarshal([]byte(show), &view)
	if len(view.Task.Associations) != 1 || view.Task.Associations[0].RunID != started.RunID || !bytes.Equal(view.Task.Revisions[0].RequestBytes, original) {
		t.Fatal("queue lost history or duplicated Run")
	}
}

func queueNextContinuationFixture(t *testing.T, root string) {
	t.Helper()
	folder := filepath.Join(root, ".prifly/workflows/tail")
	next := filepath.Join(root, ".prifly/workflows/next")
	if err := monitorCopyAuthority(next, folder); err != nil {
		t.Fatal(err)
	}
	err := filepath.WalkDir(next, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := strings.ReplaceAll(string(body), "example-tail:", "example-next:")
		text = strings.ReplaceAll(text, "example:step/verify", "example-next:step/verify")
		text = strings.ReplaceAll(text, "id: example:workflow/tail", "id: example:workflow/next")
		text = strings.ReplaceAll(text, "from_workflows: [example:workflow/source]", "from_workflows: [example:workflow/tail]")
		text = strings.ReplaceAll(text, "handoff: {stage: prepare, output: handoff, verdict: pass}", "handoff: {source_input: handoff}")
		text = strings.ReplaceAll(text, "plan: {stage: draft, output: plan, verdict: pass}", "plan: {source_input: plan}")
		text = strings.ReplaceAll(text, "draft: {stage: draft, output: draft, verdict: pass}", "draft: {source_input: draft}")
		return os.WriteFile(path, []byte(text), 0644)
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".prifly/project.yaml")
	body, _ := os.ReadFile(path)
	text := strings.Replace(string(body), "  tail: {source: .prifly/workflows/tail}", "  tail: {source: .prifly/workflows/tail}\n  next: {source: .prifly/workflows/next}", 1)
	text += "  next:\n    title: Next continuation\n    description: Continue a cancelled tail.\n    kind: workflow\n    workflow: .prifly/workflows/next/workflow.yaml\n    workspace: worktree\n"
	if err := os.WriteFile(path, []byte(text), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestCLIProjectQueuePartialCancelContinueKeepsTaskAndRunHistory(t *testing.T) {
	t.Parallel()
	f := newQueueContinuationCLI(t, "  from_outcomes: [partial]\n  from_cancelled: true\n", func(root string) { queueNextContinuationFixture(t, root) })
	original := f.source.Run.Run
	if original.QueueTask == nil {
		t.Fatal("source was not queue-bound")
	}
	f.submit(map[string]string{"handoff": "{}\n"}, nil)
	writeFixtureFile(t, f.source.WorkspacePath, "plans/plan.md", "# Queue plan\n")
	f.submit(map[string]string{"draft": "{}\n"}, []prifly.WorkspaceTreeLocation{{OutputPort: "plan", Path: "plans/plan.md"}})
	f.submit(map[string]string{}, nil)
	var source prifly.RunView
	json.Unmarshal([]byte(f.command("--project", f.authority, "run", "status", f.source.RunID)), &source)
	if source.Run.Outcome == nil || *source.Run.Outcome != "partial" {
		t.Fatal("source did not reach partial")
	}
	id := original.QueueTask.Selection.TaskID
	flags := []string{"--task-id", id, "--task-owner", "host:source", "--task-revision", "1", "--task-claim-generation", "1"}
	request := queueCLIRequest(t, f.root, "current request for the next Run")
	queueCLISummary(t, f.command(append([]string{"project", "queue", "update", "--repository", f.root, "--input", request, "--command-id", "command:next-revision"}, flags...)...))
	flags[5] = "2"
	launch := func(target, from, command string) projectStartResult {
		t.Helper()
		args := append([]string{"--repository", f.root, "--launch", target, "--host", "codex-cli", "--source-run", from}, flags...)
		var review projectLaunchSummary
		raw := f.command(append([]string{"project", "continue", "--prepare"}, args...)...)
		json.Unmarshal([]byte(raw), &review)
		validateQueueCLI(t, "ProjectLaunchSummaryV4", raw)
		var result projectStartResult
		raw = f.command(append(append([]string{"project", "continue"}, args...), "--command-id", command, "--expected-launch-digest", review.ReviewDigest)...)
		json.Unmarshal([]byte(raw), &result)
		validateQueueCLI(t, "ProjectStartV4", raw)
		return result
	}
	f.refuse("queue_source_identity_conflict", "project", "continue", "--prepare", "--repository", f.root, "--launch", "tail", "--host", "codex-cli", "--source-run", source.Run.ID)
	first := launch("tail", source.Run.ID, "command:queue-tail")
	f.command("--project", f.authority, "run", "cancel", first.RunID, "--reason", "fixture cancellation")
	f.command("--project", f.authority, "run", "drive", first.RunID)
	second := launch("next", first.RunID, "command:queue-next")
	if source.Run.ID == first.RunID || first.RunID == second.RunID || source.Run.ID == second.RunID {
		t.Fatal("continuations reused Run identity")
	}
	for _, child := range []projectStartResult{first, second} {
		if child.QueueTask == nil || child.QueueTask.Selection.TaskID != id || child.QueueTask.Selection.TaskRevision != 2 {
			t.Fatal("continuation lost task/current revision")
		}
	}
	var unchanged prifly.RunView
	json.Unmarshal([]byte(f.command("--project", f.authority, "run", "status", source.Run.ID)), &unchanged)
	if !reflect.DeepEqual(source, unchanged) {
		// Read clocks can advance; history and sealed data cannot.
		if source.RunVersion != unchanged.RunVersion || !reflect.DeepEqual(source.Run, unchanged.Run) {
			t.Fatal("continuation edited the source Run")
		}
	}
	if !bytes.Equal(unchanged.Run.QueueRequestBytes, original.QueueRequestBytes) {
		t.Fatal("task edit rewrote previous Run request")
	}
	var task prifly.QueueTaskView
	json.Unmarshal([]byte(f.command("project", "queue", "show", "--repository", f.root, "--task-id", id)), &task)
	if len(task.Task.Associations) != 3 {
		t.Fatalf("association history: %+v", task.Task.Associations)
	}
}
