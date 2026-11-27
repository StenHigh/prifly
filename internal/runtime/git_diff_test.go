package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

func TestGitDiffSurvivesClaimRelease(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	configure := func(e *Engine, workflow *flow.WorkflowRevision, registry *RegistryFile) {
		definitions, _, err := Builtins()
		if err != nil {
			t.Fatal(err)
		}
		diff := builtinRef(definitions, gitDiffSchemaID)
		implementation := flow.Ref{ID: "test:schema/implementation", Version: "1.0.0"}
		schema := writeRegistryDocument(t, e, "implementation.json", map[string]any{"type": "object", "required": []string{"base_commit", "head_commit"}})
		implementation.Digest = rawDigest(schema)
		registry.Entries = append(registry.Entries, Definition{Ref: implementation, Kind: "schema", Path: "implementation.json"})
		pairPort := flow.Port{Format: "json", SchemaRef: &implementation}
		diffPort := flow.Port{Format: "json", SchemaRef: &diff}
		original := workflow.Definition.Stages["plan"]
		var step flow.StepDefinition
		data, err := os.ReadFile(filepath.Join(e.Root, "steps/tree-plan.json"))
		if err != nil || json.Unmarshal(data, &step) != nil {
			t.Fatalf("read fixture step: %v", err)
		}
		step.SchemaVersion = "2"
		step.WorkspaceTrees = nil
		stage := func(name, next, predecessor string) flow.Stage {
			definition := step
			definition.ID = "test:step/" + name
			definition.Outputs = map[string]flow.OutputPort{
				"implementation": {Port: pairPort, RequiredFor: []string{"pass"}},
				"diff":           {Port: diffPort, RequiredFor: []string{"pass"}},
			}
			result := original
			result.On = map[string]string{"pass": next}
			if predecessor != "" {
				definition.Inputs = map[string]flow.InputPort{"implementation": {Port: pairPort, Required: true}}
				definition.Outputs["step_diff"] = flow.OutputPort{Port: diffPort, RequiredFor: []string{}}
				result.InputBindings = map[string]flow.Binding{"implementation": {From: "stage_output", StageID: predecessor, Port: "implementation"}}
			}
			path := "steps/" + name + ".json"
			payload := writeRegistryDocument(t, e, path, definition)
			result.StepRef = flow.Ref{ID: definition.ID, Version: definition.Version, Digest: rawDigest(payload)}
			registry.Entries = append(registry.Entries, Definition{Ref: result.StepRef, Kind: "step", Path: path})
			return result
		}
		workflow.Definition.Entry = "implement"
		workflow.Definition.Stages = map[string]flow.Stage{
			"implement": stage("implement", "fix", ""),
			"fix":       stage("fix", "commit", "implement"),
			"commit":    stage("commit", "done", "fix"),
			"done":      workflow.Definition.Stages["done"],
		}
	}
	e, runID := treeFixture(t, flow.WorkspaceTreeCapturePolicy{Kind: "exact_file", Path: "plan.md"}, nil, nil, "worktree", false, configure)
	first := handOver(t, e, runID)
	git := func(args ...string) string {
		t.Helper()
		out, err := e.git(ctx, first.RepositoryWorkspace, args...)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	base := git("rev-parse", "HEAD")
	commit := func() string {
		t.Helper()
		git("add", ".")
		git("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "change")
		return git("rev-parse", "HEAD")
	}
	report := func(task SessionTask, head string, supplied bool) SessionSubmission {
		t.Helper()
		if _, ok := task.Context.Outputs["diff"]; ok {
			t.Fatal("archive exposed as a host-owned slot")
		}
		slot := task.Context.Outputs["implementation"]
		data, err := json.Marshal(gitCommitRange{Base: base, Head: head})
		if err != nil {
			t.Fatal(err)
		}
		writeWorkspaceTreeFile(t, task.Workspace, slot.Path, string(data))
		submission := treeSubmission(t, task, "implementation", nil)
		var result Result
		if err := json.Unmarshal(submission.Result, &result); err != nil {
			t.Fatal(err)
		}
		result.Outputs["implementation"] = ArtifactRef{ArtifactID: slot.ArtifactID, Revision: slot.Revision, Digest: rawDigest(data)}
		if supplied {
			result.Outputs["diff"] = result.Outputs["implementation"]
		}
		submission.Result, err = json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		return submission
	}
	accept := func(submission SessionSubmission) {
		t.Helper()
		if _, err := e.SubmitSession(ctx, submission); err != nil {
			t.Fatal(err)
		}
		if err := e.Drive(ctx, runID); err != nil {
			t.Fatal(err)
		}
	}
	writeWorkspaceTreeFile(t, first.RepositoryWorkspace, "README.md", "# implemented\n")
	writeWorkspaceTreeFile(t, first.RepositoryWorkspace, "binary.bin", "new\x00binary")
	implemented := commit()
	if _, err := e.SubmitSession(ctx, report(first, implemented, true)); err == nil || !strings.Contains(err.Error(), "authority captures") {
		t.Fatalf("host-supplied diff was not refused: %v", err)
	}
	if _, err := e.SubmitSession(ctx, report(first, base, false)); err == nil || !strings.Contains(err.Error(), "head must match") {
		t.Fatalf("stale implementation was not refused: %v", err)
	}
	accept(report(first, implemented, false))
	second := handOver(t, e, runID)
	writeWorkspaceTreeFile(t, first.RepositoryWorkspace, "README.md", "# repaired\n")
	repaired := commit()
	accept(report(second, repaired, false))
	third := handOver(t, e, runID)
	accept(report(third, repaired, false))
	run, _, err := e.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := e.claim(ctx, first.ClaimID)
	if err != nil {
		t.Fatal(err)
	}
	missing := run.Attempts[second.AttemptID].Accepted.Outputs["step_diff"]
	blob := filepath.Join(e.Root, e.Config.Configuration.ArtifactRoot, strings.TrimPrefix(missing.Digest, "sha256:"))
	backup := filepath.Join(t.TempDir(), "archive")
	if err := os.Rename(blob, backup); err != nil {
		t.Fatal(err)
	}
	release := ClaimReleaseRequest{CommandID: "command:release-git-diff", ClaimID: claim.ID, Generation: claim.Generation}
	if _, err := e.ReleaseWorktree(ctx, release); err == nil || !strings.Contains(err.Error(), "git_diff_archive_missing") {
		t.Fatalf("claim release ignored the missing archive: %v", err)
	}
	if _, err := os.Stat(first.RepositoryWorkspace); err != nil {
		t.Fatalf("rejected release removed the worktree: %v", err)
	}
	if err := os.Rename(backup, blob); err != nil {
		t.Fatal(err)
	}
	read := func(task SessionTask, port, from, to string) savedGitDiff {
		t.Helper()
		ref := run.Attempts[task.AttemptID].Accepted.Outputs[port]
		_, data, err := e.Artifact(ref)
		var value savedGitDiff
		if err != nil || json.Unmarshal(data, &value) != nil || !value.Complete || value.Base != from || value.Head != to {
			t.Fatalf("archive %s does not describe %s..%s: %s (%v)", port, from, to, data, err)
		}
		return value
	}
	if _, err := e.ReleaseWorktree(ctx, release); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(first.RepositoryWorkspace); !os.IsNotExist(err) {
		t.Fatalf("expected removed worktree, got %v", err)
	}
	if err := os.Rename(claim.Repository.Toplevel, filepath.Join(t.TempDir(), "offline-repository")); err != nil {
		t.Fatal(err)
	}
	overall := read(first, "diff", base, implemented)
	if len(overall.Files) != 2 || overall.Files[1].Kind != "binary" || !strings.Contains(overall.Files[0].Patch, "+# implemented") {
		t.Fatalf("wrong text / binary inventory: %+v", overall.Files)
	}
	fix := read(second, "step_diff", implemented, repaired)
	if len(fix.Files) != 1 || !strings.Contains(fix.Files[0].Patch, "-# implemented\n+# repaired") {
		t.Fatalf("wrong step patch: %+v", fix.Files)
	}
	read(third, "diff", base, repaired)
	fixRef := run.Attempts[second.AttemptID].Accepted.Outputs["diff"]
	commitRef := run.Attempts[third.AttemptID].Accepted.Outputs["diff"]
	if fixRef.Digest != commitRef.Digest || fixRef.ArtifactID == commitRef.ArtifactID {
		t.Fatal("equal comparisons should share bytes and keep distinct attempt identities")
	}
	if _, exists := run.Attempts[third.AttemptID].Accepted.Outputs["step_diff"]; exists {
		t.Fatal("equal heads should not require a second artifact")
	}
}
