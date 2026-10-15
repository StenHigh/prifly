package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// The browser replay opts in explicitly. Fixtures are actual saved authorities
// produced through the CLI and runtime APIs, rather than mocked HTTP payloads.
func TestMonitorBoardExportFixtures(t *testing.T) {
	t.Parallel()
	target := os.Getenv("PRIFLY_MONITOR_FIXTURE_DIR")
	if target == "" {
		t.Skip("used by test/e2e/verify-monitor-board.py")
	}
	f := newContinuationCLIResuming(t, "  from_outcomes: [partial]\n  from_cancelled: true\n", false, func(root string) {
		folder := filepath.Join(root, ".prifly/workflows/tail")
		next := filepath.Join(root, ".prifly/workflows/next")
		if err := monitorCopyAuthority(next, folder); err != nil {
			t.Fatal(err)
		}
		if err := filepath.WalkDir(next, func(path string, d os.DirEntry, err error) error {
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
		}); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, ".prifly/project.yaml")
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text := strings.Replace(string(body), "  tail: {source: .prifly/workflows/tail}", "  tail: {source: .prifly/workflows/tail}\n  next: {source: .prifly/workflows/next}", 1)
		text += "  next:\n    title: Next continuation\n    description: Continue a cancelled tail.\n    kind: workflow\n    workflow: .prifly/workflows/next/workflow.yaml\n    workspace: worktree\n"
		if err = os.WriteFile(path, []byte(text), 0644); err != nil {
			t.Fatal(err)
		}
	})
	f.submit(map[string]string{"handoff": "{}\n"}, nil)
	writeFixtureFile(t, f.source.WorkspacePath, "plans/plan.md", "# Browser evidence\n")
	gitFixture(t, f.source.WorkspacePath, "add", ".")
	gitFixture(t, f.source.WorkspacePath, "commit", "-qm", "fixture draft")
	commit := gitFixture(t, f.source.WorkspacePath, "rev-parse", "HEAD")
	f.submit(map[string]string{"draft": "{}\n"}, []prifly.WorkspaceTreeLocation{{OutputPort: "plan", Path: "plans/plan.md"}})
	f.submit(map[string]string{}, nil)
	continueRun := func(launch, source string, extra ...string) projectStartResult {
		args := append([]string{"--repository", f.root, "--launch", launch, "--host", "codex-cli", "--source-run", source}, extra...)
		var review projectLaunchSummary
		if err := json.Unmarshal([]byte(f.command(append([]string{"project", "continue", "--prepare"}, args...)...)), &review); err != nil {
			t.Fatal(err)
		}
		var child projectStartResult
		if err := json.Unmarshal([]byte(f.command(append(append([]string{"project", "continue"}, args...), "--expected-launch-digest", review.ReviewDigest)...)), &child); err != nil {
			t.Fatal(err)
		}
		return child
	}
	first := continueRun("tail", f.source.RunID)
	f.command("--project", f.authority, "run", "cancel", first.RunID, "--reason", "browser fixture cancellation")
	f.command("--project", f.authority, "run", "drive", first.RunID)
	next := continueRun("next", first.RunID)
	f.command("--project", f.authority, "capacity", "set", "--capacity", "4", "--reason", "independent continuations")
	repeated := continueRun("tail", f.source.RunID, "--allow-duplicate-continuation", "--workspace-commit", commit, "--workspace", "worktree")
	if err := monitorCopyAuthority(filepath.Join(target, "continuations"), f.authority); err != nil {
		t.Fatal(err)
	}
	// A generic legacy workflow has no title input, Git, or provenance. More
	// than one metadata page is materialized; equal titles never form identity.
	generic := filepath.Join(t.TempDir(), "generic")
	legacy := monitorFixture(t, generic)
	e, err := prifly.Open(generic, false)
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 251; n++ {
		started, err := e.Start(context.Background(), prifly.StartOptions{CommandID: fmt.Sprintf("command:board-history-%03d", n), WorkflowFile: "workflows/monitor.json", BriefFile: "brief.json", Inputs: map[string]string{}})
		if err != nil {
			t.Fatal(err)
		}
		if err = e.Drive(context.Background(), started.Receipt.RunID); err != nil {
			t.Fatal(err)
		}
	}
	if err = e.Close(); err != nil {
		t.Fatal(err)
	}
	if err = monitorCopyAuthority(filepath.Join(target, "generic"), generic); err != nil {
		t.Fatal(err)
	}
	report := map[string]any{"partial": f.source.RunID, "cancelled": first.RunID, "next": next.RunID, "repeated": repeated.RunID, "legacy": legacy, "generic_count": 252, "boundary": "Saved CLI/runtime fixtures; no external host execution or release qualification claim."}
	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(target, "cli-fixtures.json"), body, 0600); err != nil {
		t.Fatal(err)
	}
}

// os.CopyFS intentionally adds write permissions. Authority databases must
// retain their private source mode when exported for read-only replay.
func monitorCopyAuthority(destination, source string) error {
	if err := os.CopyFS(destination, os.DirFS(source)); err != nil {
		return err
	}
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return os.Chmod(filepath.Join(destination, relative), info.Mode().Perm())
	})
}
