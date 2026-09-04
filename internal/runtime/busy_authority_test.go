package runtime

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stenhigh/prifly/internal/local"
)

// TestAuthorityWriteLockHelper is a second process that really holds the
// authority database's write lock. Nothing is injected into the engine: the
// contention is the one SQLite itself reports to the next writer.
func TestAuthorityWriteLockHelper(t *testing.T) {
	if os.Getenv("AUTHORITY_LOCK_HELPER") != "1" {
		return
	}
	if len(os.Args) < 3 {
		os.Exit(80)
	}
	database, marker := os.Args[len(os.Args)-2], os.Args[len(os.Args)-1]
	db, err := sql.Open("sqlite3", "file:"+database+"?mode=rw&_txlock=immediate&_busy_timeout=0")
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(81)
	}
	defer db.Close()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(82)
	}
	defer func() { _ = tx.Rollback() }()
	if err := os.WriteFile(marker, []byte("held"), 0600); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(83)
	}
	// The parent releases this lock by killing this process when its own
	// observation has run. The bound is only a leak guard.
	time.Sleep(60 * time.Second)
}

// holdAuthorityWriteLock starts that second process and returns once the lock
// is actually held, together with the release the caller decides to call.
func holdAuthorityWriteLock(t *testing.T, e *Engine) (release func()) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root := e.Root
	database := filepath.Join(root, e.Config.Configuration.StateRoot, "state.sqlite3")
	marker := filepath.Join(root, "authority-lock-held")
	_ = os.Remove(marker)
	child := exec.Command(executable, "-test.run=^TestAuthorityWriteLockHelper$", "--", database, marker)
	child.Env = []string{"AUTHORITY_LOCK_HELPER=1", "GORACE=atexit_sleep_ms=0"}
	child.Stderr = os.Stderr
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	stopped := false
	release = func() {
		if stopped {
			return
		}
		stopped = true
		_ = child.Process.Kill()
		_ = child.Wait()
	}
	t.Cleanup(release)
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			return release
		}
		if time.Now().After(deadline) {
			release()
			t.Fatal("second writer never took the authority write lock")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// A busy authority is contention, not a verdict about the worker. The
// observation is retried until the other writer lets go; if it cannot be
// recorded at all, the step fails as an authority failure that no declared
// error handler consumes.
func TestBusyAuthorityRetriesAndNeverRoutesOnError(t *testing.T) {
	for _, name := range []string{"retried", "unrecorded"} {
		t.Run(name, func(t *testing.T) {
			e, workflow := coreDriverFixture(t, "pass")
			runID := coreDriverStart(t, e, workflow)
			a := driverAdmit(t, e, runID)
			driverDispatchFixture(t, e, runID, a.ID)
			ctx := context.Background()
			identity := local.ProcessIdentity{PID: os.Getpid(), OwnerPID: os.Getpid()}
			if err := e.observe(ctx, runID, a.ID, local.ProcessObservation{Kind: "start_returned", Identity: identity}); err != nil {
				t.Fatal(err)
			}
			release := holdAuthorityWriteLock(t, e)
			// Longer than the store's busy timeout, so SQLite really reports
			// busy to the engine at least once before the lock is released.
			// Both bounds sit just above that timeout: the test is about the
			// retry happening at all, not about waiting.
			observeCtx, cancel := context.WithTimeout(ctx, 3500*time.Millisecond)
			defer cancel()
			if name == "retried" {
				time.AfterFunc(3200*time.Millisecond, release)
			}
			started := time.Now()
			err := e.observe(observeCtx, runID, a.ID, local.ProcessObservation{Kind: "group_empty", Identity: identity})
			elapsed := time.Since(started)
			release()
			if elapsed < 2900*time.Millisecond {
				t.Fatalf("the engine did not wait out the concurrent writer: %v %v", elapsed, err)
			}
			if name == "retried" {
				if err != nil {
					t.Fatalf("a released lock did not let the observation through: %v", err)
				}
				r := driverRun(t, e, runID)
				if r.Attempts[a.ID].ExecutorEnd == nil || r.Attempts[a.ID].Status != "verifying" || len(r.Diagnostics) != 0 {
					t.Fatalf("the retried observation was not recorded cleanly: %+v", r.Attempts[a.ID])
				}
				return
			}
			if !local.IsBusy(err) {
				t.Fatalf("a held write lock was not reported by the store: %v", err)
			}
			exit := 0
			outcome := local.ProcessOutcome{Started: true, WaitReturned: true, GroupEmpty: true, ExitCode: &exit, Identity: identity}
			if err := e.settle(ctx, runID, a.ID, outcome, err); err != nil {
				t.Fatal(err)
			}
			r := driverRun(t, e, runID)
			settled := r.Attempts[a.ID]
			if settled.Status != "failed" || settled.Settled == nil {
				t.Fatalf("an unrecordable observation left the attempt open: %+v", settled)
			}
			for _, d := range r.Diagnostics {
				if d.Code == "executor_observation_failed" {
					t.Fatalf("a busy authority was blamed on the worker: %+v", d)
				}
			}
			named := false
			for _, d := range r.Diagnostics {
				named = named || d.Code == "authority_persistence_failed"
			}
			if !named {
				t.Fatalf("the authority failure was not named: %+v", r.Diagnostics)
			}
			if events := coreErrorEvents(t, e, runID); len(events) != 0 {
				t.Fatalf("an authority failure entered a declared on_error branch: %+v", events)
			}
			if slices.Contains(r.Ready, "recovered") {
				t.Fatalf("the run was routed to its error handler: %+v", r.Ready)
			}
		})
	}
}

// Sealing a result is the authority's own work. When its blob store cannot
// accept the bytes, the step must not be failed as if the worker produced an
// invalid output, and no declared error route may consume it. The recorded
// cause is what a reader needs to fix the storage.
func TestUnavailableBlobStoreAtSettlementIsAnAuthorityFailure(t *testing.T) {
	e, workflow := coreDriverFixture(t, "commit-wait")
	runID := coreDriverStart(t, e, workflow)
	ctx := context.Background()
	artifacts := filepath.Join(e.Root, e.Config.Configuration.ArtifactRoot)
	t.Cleanup(func() { _ = os.Chmod(artifacts, 0700) })
	driven := make(chan error, 1)
	go func() { driven <- e.Drive(ctx, runID) }()
	// The worker holds until its marker appears, so the store is made
	// unavailable exactly between a reported result and its settlement.
	r := driverWait(t, e, runID, func(r Run) bool {
		for _, a := range r.Attempts {
			if len(a.Candidate) > 0 {
				return true
			}
		}
		return false
	})
	var a *Attempt
	for _, current := range r.Attempts {
		a = current
	}
	if err := os.Chmod(artifacts, 0500); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a.Workspace, "finish"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := <-driven; err != nil {
		t.Fatalf("the driver did not finish its settlement: %v", err)
	}
	r = driverRun(t, e, runID)
	settled := r.Attempts[a.ID]
	if settled.Status != "failed" || settled.Settled == nil || settled.Accepted != nil {
		t.Fatalf("an unsealable result was accepted or left open: %+v", settled)
	}
	var recorded *Diagnostic
	for i, d := range r.Diagnostics {
		if d.Code == "invalid_output" {
			t.Fatalf("a storage failure was blamed on the worker's output: %+v", d)
		}
		if d.Code == "authority_output_sealing_failed" {
			recorded = &r.Diagnostics[i]
		}
	}
	if recorded == nil {
		t.Fatalf("the sealing failure was not named: %+v", r.Diagnostics)
	}
	const sentence = "Executor or result validation failed; inspect recorded evidence: "
	cause := strings.TrimPrefix(recorded.Message, sentence)
	if cause == recorded.Message || cause == "" || len(cause) > maxDiagnosticDetailBytes {
		t.Fatalf("the recorded cause is missing or unbounded: %q", recorded.Message)
	}
	if events := coreErrorEvents(t, e, runID); len(events) != 0 {
		t.Fatalf("an authority failure entered a declared on_error branch: %+v", events)
	}
	if slices.Contains(r.Ready, "recovered") {
		t.Fatalf("the run was routed to its error handler: %+v", r.Ready)
	}
}
