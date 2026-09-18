package runtime

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stenhigh/prifly/internal/local"
)

// A drive call that fails before starting the program must take its attempt
// watcher with it. The watcher's own job is to cancel the Run when the caller
// is interrupted; left behind by an early return, it does that later, to a Run
// that has already moved on, naming a driver that finished long ago.
func TestDriveTakesItsWatcherWithItOnAnEarlyFailure(t *testing.T) {
	e, runID, claim := programAfterWriteFixture(t, "workspace-read", "repair")
	ctx := context.Background()
	planTask := handOver(t, e, runID)
	if _, err := e.SubmitSession(ctx, hostResult(t, e, planTask, "planned")); err != nil {
		t.Fatalf("the write step's report was refused: %v", err)
	}
	// The boundary read fails for a reason of the authority's own, with the
	// caller's context alive: the recorded claim no longer names the tree it
	// was taken on.
	_, err := e.Store.ApplyAuthority(ctx, local.AuthorityCommand{ID: newID("command"), Actor: e.owner, Key: AuthorityClaimsKey, Payload: json.RawMessage(`{"unreadable_claim_path_fixture":true}`)}, func(s local.AuthoritySnapshot) (local.AuthorityChange, error) {
		record, err := e.decodeClaims(s)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		planted := false
		for index := range record.Claims {
			if record.Claims[index].ID == claim.ID {
				record.Claims[index].Path = claim.Path + "-moved-elsewhere"
				planted = true
			}
		}
		if !planted {
			return local.AuthorityChange{}, local.ErrIntegrity
		}
		data, err := canonicalState(record)
		return local.AuthorityChange{Data: data}, err
	})
	if err != nil {
		t.Fatal(err)
	}
	driveCtx, endDriveCall := context.WithCancel(context.Background())
	// The same unreadable claim stops the drive loop again on the stage the
	// error route leads to; what this test is about is the attempt that was
	// refused before its program started.
	if err := e.Drive(driveCtx, runID); err == nil {
		t.Fatal("a drive that could not read the claim reported success")
	}
	r := driverRun(t, e, runID)
	var attempt *Attempt
	for _, candidate := range r.Attempts {
		if candidate.ProcessOutcome != nil {
			attempt = candidate
		}
	}
	if attempt == nil || attempt.Settled == nil || attempt.Started != nil || attempt.ProcessOutcome.Started {
		t.Fatalf("the program's attempt did not settle unstarted: %+v", attempt)
	}
	if r.Diagnostics[len(r.Diagnostics)-1].Code != "workspace_validation_failed" {
		t.Fatalf("the refusal is not the one this test arranges: %+v", r.Diagnostics)
	}
	if attempt.Dispatch == nil {
		t.Fatal("the refusal this test arranges happens after the dispatch record; it stopped earlier")
	}
	if r.CancelRequested || r.terminal() {
		t.Fatalf("the Run did not outlive the failed attempt: cancel=%v status=%s", r.CancelRequested, r.Status)
	}
	// The host's drive call is over. Nothing that belonged to it may still act
	// on this Run.
	endDriveCall()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		current := driverRun(t, e, runID)
		if current.CancelRequested {
			reasons := []string{}
			for _, stop := range current.Stops {
				reasons = append(reasons, stop.Reason)
			}
			t.Fatalf("a finished drive call cancelled the Run afterwards: %s", strings.Join(reasons, "; "))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A Run that broke on a program stage runs that stage again without repeating
// the stages that already finished: a cold start lost six completed steps and
// three commits to a missing password, and repeating them would have paid
// twice for work already sealed.
func TestReopenRunsTheBrokenStageAgainAndKeepsWhatWasDone(t *testing.T) {
	broken := environmentSourceFile(t, "env", "OTHER=ignored\n")
	e, runID, _ := programAfterWriteFixture(t, "workspace-read", "none", map[string]EnvironmentSource{"DRIVER_TEST_SOURCED": {DotEnv: broken, Key: "PASSWORD"}})
	ctx := context.Background()
	planTask := handOver(t, e, runID)
	if _, err := e.SubmitSession(ctx, hostResult(t, e, planTask, "planned")); err != nil {
		t.Fatalf("the write step's report was refused: %v", err)
	}
	// The password its program was told to read is not in the file: the same
	// shape a cold start met, and the same refusal before the program starts.
	if err := e.Drive(ctx, runID); err == nil {
		t.Fatal("the program started without the value it was told to read")
	}
	r := driverRun(t, e, runID)
	if r.Status != "failed" || r.Outcome != nil {
		t.Fatalf("the Run did not break technically: %s %+v", r.Status, r.Outcome)
	}
	completed := 0
	for _, step := range r.Steps {
		if step.Status == "completed" {
			completed++
		}
	}
	if completed == 0 {
		t.Fatal("the fixture finished no stage before it broke")
	}
	// The Run's own configuration is what a reopened stage reads, so the
	// sealed binding still names the source; the file it names is fixed here.
	if err := os.WriteFile(broken, []byte("PASSWORD="+environmentSourceValue+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, view, err := e.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Reopen(ctx, runID, newID("command"), "the declared source was empty", view.Snapshot.Version); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	after := driverRun(t, e, runID)
	if after.Status != "ready" || after.Settled != nil {
		t.Fatalf("the reopened Run is not ready to be driven: %s", after.Status)
	}
	kept := 0
	for _, step := range after.Steps {
		if step.Status == "completed" {
			kept++
		}
	}
	if kept != completed {
		t.Fatalf("reopen re-ran or lost finished work: %d completed before, %d after", completed, kept)
	}
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatalf("drive after reopen: %v", err)
	}
	final := driverRun(t, e, runID)
	if final.Status != "completed" {
		t.Fatalf("the reopened stage did not finish the Run: %s %+v", final.Status, final.Diagnostics)
	}
}

// An accepted verdict is an answer, not a breakage, and reopen refuses it.
func TestReopenRefusesARunThatReachedAnOutcome(t *testing.T) {
	e, runID := driverProject(t, "pass", 5000)
	ctx := context.Background()
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatal(err)
	}
	r, view, err := e.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "completed" || r.Outcome == nil {
		t.Fatalf("the fixture did not reach an outcome: %s", r.Status)
	}
	_, err = e.Reopen(ctx, runID, newID("command"), "try again", view.Snapshot.Version)
	if err == nil || !strings.Contains(err.Error(), "not_a_broken_run") {
		t.Fatalf("a Run that answered its question was reopened: %v", err)
	}
}

// A Run is reopened once: the second call has nothing broken to reopen, and a
// stale expected version is refused before anything moves. The stop guard of
// this command cannot be reached from here — a terminal Run cannot be
// restricted at all — so it stands for a Run that failed while a stop was
// already in force.
func TestReopenTakesASettledRunOnceAndOnItsCurrentVersion(t *testing.T) {
	broken := environmentSourceFile(t, "env", "OTHER=ignored\n")
	e, runID, _ := programAfterWriteFixture(t, "workspace-read", "none", map[string]EnvironmentSource{"DRIVER_TEST_SOURCED": {DotEnv: broken, Key: "PASSWORD"}})
	ctx := context.Background()
	planTask := handOver(t, e, runID)
	if _, err := e.SubmitSession(ctx, hostResult(t, e, planTask, "planned")); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(ctx, runID); err == nil {
		t.Fatal("the program started without the value it was told to read")
	}
	_, view, err := e.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Reopen(ctx, runID, newID("command"), "stale version", view.Snapshot.Version-1); err == nil {
		t.Fatal("a stale expected version reopened the Run")
	}
	if _, err := e.Reopen(ctx, runID, newID("command"), "the declared source was empty", view.Snapshot.Version); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	_, view, err = e.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.Reopen(ctx, runID, newID("command"), "again", view.Snapshot.Version)
	if err == nil || !strings.Contains(err.Error(), "not_a_broken_run") {
		t.Fatalf("a Run that is already running was reopened again: %v", err)
	}
}

// A stage whose author declared a retry budget takes its own second attempt:
// the Run does not die on a program that fails once, and the failed attempt
// stays in the record with its diagnostic.
func TestDeclaredRetryTakesTheStageAgainWithoutTheOwner(t *testing.T) {
	e, runID, _ := programAfterWriteFixture(t, "workspace-read", "retry")
	ctx := context.Background()
	planTask := handOver(t, e, runID)
	if _, err := e.SubmitSession(ctx, hostResult(t, e, planTask, "planned")); err != nil {
		t.Fatal(err)
	}
	// One drive settles the failed attempt and returns the stage to ready; the
	// next one runs it again. A host that keeps driving sees one Run finish.
	for range 3 {
		if driverRun(t, e, runID).terminal() {
			break
		}
		if err := e.Drive(ctx, runID); err != nil && !strings.Contains(err.Error(), "nonzero_exit") {
			t.Fatalf("drive: %v", err)
		}
	}
	r := driverRun(t, e, runID)
	if r.Status != "completed" {
		t.Fatalf("the declared retry did not carry the Run: %s %+v", r.Status, r.Diagnostics)
	}
	attempts := 0
	for _, a := range r.Attempts {
		if a.ProcessOutcome != nil {
			attempts++
		}
	}
	if attempts != 2 {
		t.Fatalf("the program ran %d times, not twice", attempts)
	}
	failed := false
	for _, d := range r.Diagnostics {
		if d.Code == "nonzero_exit" && strings.Contains(d.Message, "declared retry") {
			failed = true
		}
	}
	if !failed {
		t.Fatalf("the failed attempt left no record of why it was taken again: %+v", r.Diagnostics)
	}
}

// A budget is a number, not a promise: when it is spent the Run fails exactly
// as it did before the budget existed.
func TestDeclaredRetryIsSpentAndThenTheRunFails(t *testing.T) {
	e, runID, _ := programAfterWriteFixture(t, "nonzero", "retry")
	ctx := context.Background()
	planTask := handOver(t, e, runID)
	if _, err := e.SubmitSession(ctx, hostResult(t, e, planTask, "planned")); err != nil {
		t.Fatal(err)
	}
	for range 4 {
		if driverRun(t, e, runID).terminal() {
			break
		}
		if err := e.Drive(ctx, runID); err != nil && !strings.Contains(err.Error(), "nonzero_exit") {
			t.Fatalf("drive: %v", err)
		}
	}
	r := driverRun(t, e, runID)
	if r.Status != "failed" || r.Outcome != nil {
		t.Fatalf("a program that always fails did not end the Run: %s", r.Status)
	}
	attempts := 0
	for _, a := range r.Attempts {
		if a.ProcessOutcome != nil {
			attempts++
		}
	}
	if attempts != 2 {
		t.Fatalf("a budget of one repeat produced %d attempts", attempts)
	}
}
