package runtime

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stenhigh/prifly/internal/local"
)

func TestAcceptanceProducerSettlementKeepsStorageReserve(t *testing.T) {
	e, options := acceptanceProject(t, []string{"step_output", "step_result"}, "", "pass", false)
	ctx := context.Background()
	started, err := e.Start(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	runID := started.Receipt.RunID
	var attemptID, workspace string
	func() {
		lock, err := e.driverLock(runID)
		if err != nil {
			t.Fatal(err)
		}
		defer lock.Close()
		socket, closeServer, err := e.serveSteps()
		if err != nil {
			t.Fatal(err)
		}
		defer closeServer()
		attempt := driverAdmit(t, e, runID)
		attemptID, workspace = attempt.ID, attempt.Workspace
		usage, err := e.Store.StorageUsage(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := e.Store.Close(); err != nil {
			t.Fatal(err)
		}
		// Change only the real store's logical allocation limit after producer
		// admission. Its process and settlement still use the native driver.
		e.Store, err = local.OpenStore(filepath.Join(e.Root, e.Config.Configuration.StateRoot), local.StoreOptions{EventTypes: EventTypes, SoftLimitBytes: max(64<<10, usage.AllocatedBytes)})
		if err != nil {
			t.Fatal(err)
		}
		r, view, err := e.load(ctx, runID)
		if err != nil {
			t.Fatal(err)
		}
		if err := e.executePending(ctx, r, view, attempt, socket); err != nil {
			t.Fatal("storage quota discarded proven producer settlement", err)
		}
	}()
	r, settled, err := e.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	producer := r.Attempts[attemptID]
	if producer == nil || producer.Status != "completed" || producer.Settled == nil || producer.Accepted != nil || producer.ProcessOutcome == nil || !producer.ProcessOutcome.Started || !producer.ProcessOutcome.WaitReturned || !producer.ProcessOutcome.GroupEmpty || producer.ProcessOutcome.Uncertain {
		t.Fatalf("producer process settlement was not retained: %+v", producer)
	}
	if len(r.Active) != 0 || r.ActiveCheckID != "" || len(r.CheckExecutions) != 0 || r.HasUnresolvedEffects || r.PendingAcceptance == nil || r.PendingAcceptance.Kind != "step_result" || r.PendingAcceptance.ProducerAttemptID != attemptID || r.Steps[producer.StepID].Status != "verifying" {
		t.Fatal("settlement failed to retain the boundary without admitting a checker")
	}
	ref := r.PendingAcceptance.Bindings["report"]
	if _, _, err := e.Artifact(ref); !os.IsNotExist(err) {
		t.Fatal("quota implicitly accepted unchecked producer output", err)
	}
	if slot, owner, err := e.Store.Slot(ctx); err != nil || slot != "" || owner != "" {
		t.Fatal("proven producer settlement retained the execution slot", slot, owner, err)
	}

	// The reserve pays for settlement, not another check admission. Drive
	// must preserve this pending boundary and must not rerun the producer.
	if err := e.Drive(ctx, runID); driverFailureCode(err, "") != "storage_budget_exhausted" {
		t.Fatal("new checker admission crossed the storage budget", err)
	}
	after, current, err := e.load(ctx, runID)
	if err != nil || !bytes.Equal(current.Snapshot.Data, settled.Snapshot.Data) || current.Snapshot.EventSeq != settled.Snapshot.EventSeq || current.Snapshot.Version != settled.Snapshot.Version || len(after.CheckExecutions) != 0 {
		t.Fatal("quota refusal changed the pending boundary or admitted a checker", err)
	}
	if starts, err := os.ReadFile(filepath.Join(workspace, "worker-starts")); err != nil || string(starts) != "start\n" {
		t.Fatal("producer was not executed exactly once", err)
	}
	if _, err := e.Restrict(ctx, RestrictCommand{SchemaVersion: "1", CommandID: newID("command"), Scope: "run", ScopeID: runID, Kind: "cancel", Reason: "cancel pending checks above the admission quota"}); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatal(err)
	}
	after = driverRun(t, e, runID)
	if after.Status != "cancelled" || after.PendingAcceptance != nil || len(after.CheckExecutions) != 0 || after.Attempts[attemptID].Accepted != nil {
		t.Fatal("storage quota prevented cancellation or promoted the producer result")
	}
	if _, _, err := e.Artifact(ref); !os.IsNotExist(err) {
		t.Fatal("cancelling pending acceptance published the output", err)
	}
	if slot, owner, err := e.Store.Slot(ctx); err != nil || slot != "" || owner != "" {
		t.Fatal("cancellation retained a slot", slot, owner, err)
	}
	if err := e.Store.Verify(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestAcceptancePassedRecoveryAfterStorageRefusal(t *testing.T) {
	for _, continuation := range []string{"resume", "cancel"} {
		t.Run(continuation, func(t *testing.T) {
			e, options := acceptanceProject(t, []string{"step_output", "step_result"}, "", "pass", false)
			ctx := context.Background()
			started, err := e.Start(ctx, options)
			if err != nil {
				t.Fatal(err)
			}
			runID := started.Receipt.RunID
			producer := driverExecuteFirst(t, e, runID)
			passed := acceptanceRunChecksThroughPassed(t, e, runID)
			ref := passed.PendingAcceptance.Bindings["report"]
			if _, _, err := e.Artifact(ref); !os.IsNotExist(err) {
				t.Fatal("passed boundary consumed its producer result before acceptance", err)
			}
			usage, err := e.Store.StorageUsage(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if err := e.Store.Close(); err != nil {
				t.Fatal(err)
			}
			e.Store, err = local.OpenStore(filepath.Join(e.Root, e.Config.Configuration.StateRoot), local.StoreOptions{EventTypes: EventTypes, SoftLimitBytes: max(64<<10, usage.AllocatedBytes)})
			if err != nil {
				t.Fatal(err)
			}
			r, before, err := e.load(ctx, runID)
			if err != nil {
				t.Fatal(err)
			}
			if err := e.Drive(ctx, runID); driverFailureCode(err, "") != "storage_budget_exhausted" {
				t.Fatal("acceptance unexpectedly crossed the protected storage budget", err)
			}
			after, refused, err := e.load(ctx, runID)
			if err != nil || !bytes.Equal(refused.Snapshot.Data, before.Snapshot.Data) || refused.Snapshot.Version != before.Snapshot.Version || refused.Snapshot.EventSeq != before.Snapshot.EventSeq {
				t.Fatal("refused acceptance committed a partial Run transition", err)
			}
			if after.PendingAcceptance == nil || after.PendingAcceptance.Status != "passed" || after.Attempts[producer.ID].Accepted != nil || len(after.Steps[producer.StepID].Outputs) != 0 || len(after.Outputs) != 0 {
				t.Fatal("checked bytes were confused with accepted StepResult or workflow exports")
			}
			// All checks passed durably before this attempt. Filesystem metadata
			// can survive a rejected SQLite transition; it is not a flow export.
			artifact, data, err := e.Artifact(ref)
			if err != nil || string(data) != "accepted output\n" || len(artifact.ContentCheckEvidence) != 1 {
				t.Fatal("fixture did not cross the checked-metadata publication boundary", err)
			}
			metadataPath := filepath.Join(e.Root, artifactMetadataPath(ref.ArtifactID))
			metadata, err := os.ReadFile(metadataPath)
			if err != nil {
				t.Fatal(err)
			}
			root := e.Root
			if err := e.Close(); err != nil {
				t.Fatal(err)
			}
			// Restore normal quota only by reopening the actual store. The Run,
			// candidates, reports, and accepted metadata are left untouched.
			e, err = Open(root, false)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = e.Close() })
			if continuation == "cancel" {
				if _, err := e.Restrict(ctx, RestrictCommand{SchemaVersion: "1", CommandID: newID("command"), Scope: "run", ScopeID: runID, Kind: "cancel", Reason: "cancel checked but unconsumed producer result"}); err != nil {
					t.Fatal(err)
				}
			}
			if err := e.Drive(ctx, runID); err != nil {
				t.Fatal(err)
			}
			after = driverRun(t, e, runID)
			if after.PendingAcceptance != nil || len(after.CheckExecutions) != len(r.CheckExecutions) || len(after.Attempts) != 1 || len(after.Steps) != 1 || !bytes.Equal(after.Attempts[producer.ID].Candidate, producer.Candidate) {
				t.Fatal("recovery changed candidate identity or invented another execution")
			}
			if continuation == "resume" {
				if after.Status != "completed" || after.Attempts[producer.ID].Accepted == nil || after.Outputs["report"] != ref {
					t.Fatal("recovery did not consume the already checked candidate")
				}
			} else if after.Status != "cancelled" || after.Attempts[producer.ID].Accepted != nil || len(after.Steps[producer.StepID].Outputs) != 0 || len(after.Outputs) != 0 {
				t.Fatal("cancellation accepted the previously checked candidate")
			}
			for id, check := range r.CheckExecutions {
				want, wantErr := canonical(check)
				got, gotErr := canonical(after.CheckExecutions[id])
				if wantErr != nil || gotErr != nil || !bytes.Equal(want, got) {
					t.Fatal("recovery changed a settled CheckExecution", wantErr, gotErr)
				}
				if starts, err := os.ReadFile(filepath.Join(check.Workspace, "launches")); err != nil || string(starts) != "launch\n" {
					t.Fatal("recovery repeated a checker process", err)
				}
			}
			if starts, err := os.ReadFile(filepath.Join(producer.Workspace, "worker-starts")); err != nil || string(starts) != "start\n" {
				t.Fatal("recovery repeated the producer process", err)
			}
			if retained, err := os.ReadFile(metadataPath); err != nil || !bytes.Equal(retained, metadata) {
				t.Fatal("recovery rewrote immutable accepted metadata", err)
			}
			if slot, owner, err := e.Store.Slot(ctx); err != nil || slot != "" || owner != "" {
				t.Fatal("recovery retained an execution slot", slot, owner, err)
			}
			if err := e.Store.Verify(ctx); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Run real checks, stopping at the persisted passed boundary before its final
// producer-result acceptance. No completed process or report is fabricated.
func acceptanceRunChecksThroughPassed(t *testing.T, e *Engine, runID string) Run {
	t.Helper()
	lock, err := e.driverLock(runID)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	initial := driverRun(t, e, runID)
	if initial.PendingAcceptance == nil || initial.PendingAcceptance.Kind != "step_result" {
		t.Fatal("fixture has no producer result boundary")
	}
	for range 2*len(initial.PendingAcceptance.Checks) + 2 {
		r, view, err := e.load(context.Background(), runID)
		if err != nil {
			t.Fatal(err)
		}
		if r.PendingAcceptance == nil {
			t.Fatal("check processing consumed the boundary too soon")
		}
		if r.PendingAcceptance.Status == "passed" {
			return r
		}
		if r.ActiveCheckID != "" {
			err = e.executePendingCheck(context.Background(), r, view, r.CheckExecutions[r.ActiveCheckID])
		} else {
			err = e.driveAcceptance(context.Background(), r, view)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Fatal("checks did not reach their passed boundary")
	return Run{}
}
