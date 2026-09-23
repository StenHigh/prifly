package main

import (
	"context"
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
	if got, err := projectContinuationHead(context.Background(), root, ""); err != nil || got != base {
		t.Fatalf("default head = %s, %v; want %s", got, err, base)
	}
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
