package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The boundary of a step that may not write is measured, not trusted. When the
// engine cannot read the tree it was going to measure, "I could not look" and
// "nothing moved" are the same answer today: the step runs unmeasured and its
// report is accepted. A claim is always a git worktree, so a mark that cannot
// be read there is a fault of this authority, not a legal shape.
func TestUnreadableWorkspaceMarkIsRefusedBeforeTheProgramStarts(t *testing.T) {
	broken := environmentSourceFile(t, "env", "PASSWORD=unused\n")
	_ = broken
	e, runID, claim := programAfterWriteFixture(t, "workspace-read", "none")
	ctx := context.Background()
	planTask := handOver(t, e, runID)
	if _, err := e.SubmitSession(ctx, hostResult(t, e, planTask, "planned")); err != nil {
		t.Fatal(err)
	}
	// The tree the read-only program would be measured against stops being
	// readable: its git data is gone.
	away := filepath.Join(filepath.Dir(claim.Path), "git-taken-away")
	if err := os.Rename(filepath.Join(claim.Path, ".git"), away); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Rename(away, filepath.Join(claim.Path, ".git")) })
	err := e.Drive(ctx, runID)
	r := driverRun(t, e, runID)
	if r.Status != "failed" {
		t.Fatalf("a step that could not be measured ran and was accepted: %s %+v", r.Status, r.Diagnostics)
	}
	named := false
	for _, d := range r.Diagnostics {
		if d.Code == "workspace_mark_unreadable" {
			named = true
		}
	}
	if !named {
		t.Fatalf("the refusal does not name what could not be read: %+v %v", r.Diagnostics, err)
	}
	for _, a := range r.Attempts {
		if a.ProcessOutcome != nil && a.ProcessOutcome.Started {
			t.Fatal("the program ran although the boundary could not be established")
		}
	}
}

// The same silence at the other end: a mark taken before the program and
// unreadable after it means the engine does not know whether the tree moved,
// and "does not know" is not "did not move".
func TestUnreadableWorkspaceMarkAfterTheProgramIsNotSilence(t *testing.T) {
	e, _, claim := programAfterWriteFixture(t, "workspace-read", "none")
	before := processWorkspaceBoundary{claimID: claim.ID, path: claim.Path, measured: true, mark: "sha256:whatever", status: ""}
	away := filepath.Join(filepath.Dir(claim.Path), "git-taken-away")
	if err := os.Rename(filepath.Join(claim.Path, ".git"), away); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Rename(away, filepath.Join(claim.Path, ".git")) })
	changed, err := before.changes(context.Background(), e)
	if err == nil {
		t.Fatalf("an unreadable tree answered %q instead of saying it could not be read", changed)
	}
	if !strings.Contains(err.Error(), claim.Path) {
		t.Fatalf("the error does not name the tree it could not read: %v", err)
	}
}
