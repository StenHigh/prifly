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

// A view names the cut it was taken at. Until 0.13.38 it then read the Run's
// recorded state changes without any upper bound, so a transition committed
// after that cut appeared inside a report that claimed to describe the Run
// before it — and both numbers in the report looked plausible.
func TestAViewAtACutDoesNotSeeTransitionsCommittedAfterIt(t *testing.T) {
	e, runID := driverProject(t, "pass", 5000)
	ctx := context.Background()
	before, err := e.View(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Run.Transitions) == 0 {
		t.Fatal("the fixture recorded no transitions to read")
	}
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatal(err)
	}
	after, err := e.View(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Run.Transitions) <= len(before.Run.Transitions) {
		t.Fatalf("driving the Run recorded no further transitions: %d then %d", len(before.Run.Transitions), len(after.Run.Transitions))
	}
	// The same Run, read again at the earlier cut, must answer what it
	// answered then.
	r, read, err := e.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	r.Transitions = nil
	if err := e.hydrateTransitions(ctx, &r, before.EventSequence); err != nil {
		t.Fatal(err)
	}
	if len(r.Transitions) != len(before.Run.Transitions) {
		t.Fatalf("a read bounded by the earlier cut saw %d transitions, the view at that cut saw %d", len(r.Transitions), len(before.Run.Transitions))
	}
	if read.Snapshot.EventSeq <= before.EventSequence {
		t.Fatal("the fixture did not advance the journal, so the bound was never exercised")
	}
}

// A read that stops at its own bound has not learned that the Run's history
// ended. Reporting "state_history_not_recorded" for an entity whose changes
// were simply never reached states as a fact about the Run what is a fact
// about the read.
func TestATruncatedHistoryIsNamedInsteadOfReportedAsAbsent(t *testing.T) {
	e, runID := driverProject(t, "pass", 5000)
	ctx := context.Background()
	full, err := e.View(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(full.Run.Transitions) < 2 {
		t.Fatalf("the fixture recorded %d transitions, too few to truncate", len(full.Run.Transitions))
	}
	restore := maxRecordedTransitions
	maxRecordedTransitions = 1
	t.Cleanup(func() { maxRecordedTransitions = restore })
	partial, err := e.View(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if !partial.Run.TransitionsPartial {
		t.Fatal("a read that stopped at its bound did not say so")
	}
	if len(partial.Run.Transitions) >= len(full.Run.Transitions) {
		t.Fatalf("the bound read %d of %d transitions", len(partial.Run.Transitions), len(full.Run.Transitions))
	}
	if reason := timingReasons(partial.Timing); !reason["state_history_partially_read"] {
		t.Fatalf("a truncated read reported %v", reason)
	} else if reason["state_history_not_recorded"] {
		t.Fatal("a truncated read still claims the Run recorded no history")
	}
}

// timingReasons collects every reason the tree names, at any depth.
func timingReasons(tree TimingTree) map[string]bool {
	found := map[string]bool{}
	var walk func(TimingNode)
	walk = func(node TimingNode) {
		for _, reason := range node.Reasons {
			found[reason] = true
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(tree.Root)
	return found
}
