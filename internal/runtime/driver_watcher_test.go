package runtime

import (
	"context"
	"encoding/json"
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
	e, runID, claim := programAfterWriteFixture(t, "workspace-read", true)
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
