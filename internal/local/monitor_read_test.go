//go:build cgo

package local

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func TestMonitorReadUsesOneCutWithoutChangingAdmission(t *testing.T) {
	t.Parallel()
	s, _ := testStore(t)
	change := storeChange(`{"phase":"held"}`)
	change.AcquireSlot = "slot:reader"
	change.AcquireResources = []ResourceClaim{{Name: "heavy", Limit: 1}}
	applyChange(t, s, storeCommand("command:holder", "run:holder", 0), change)
	applyChange(t, s, storeCommand("command:waiting", "run:waiting", 0), storeChange(`{"phase":"ready"}`))
	before, cut, err := s.ReadReceiptsAt(context.Background(), -1, 100)
	if err != nil {
		t.Fatal(err)
	}
	view, err := s.ReadMonitor(context.Background(), "run:waiting")
	if err != nil {
		t.Fatal(err)
	}
	if view.Cut != cut || view.Snapshot.Version != 1 || view.Resources["heavy"] != 1 || view.Waiting {
		t.Fatalf("mixed monitor cut: %+v", view)
	}
	after, afterCut, err := s.ReadReceiptsAt(context.Background(), -1, 100)
	if err != nil || afterCut != cut || !reflect.DeepEqual(before, after) {
		t.Fatalf("monitor mutated ledger: %d %d %v", cut, afterCut, err)
	}
}

func TestMonitorReadDoesNotMixSnapshotAndResourceOccupancy(t *testing.T) {
	t.Parallel()
	s, _ := testStore(t)
	ctx := context.Background()
	applyChange(t, s, storeCommand("command:start", "run:changing", 0), storeChange(`{"held":false}`))
	done := make(chan error, 1)
	go func() {
		for n := 0; n < 32; n++ {
			held := n%2 == 0
			change := storeChange(fmt.Sprintf(`{"held":%t}`, held))
			if held {
				change.AcquireSlot = "slot:changing"
				change.AcquireResources = []ResourceClaim{{Name: "heavy", Limit: 1}}
			} else {
				change.ReleaseSlot = "slot:changing"
			}
			result, err := s.Apply(ctx, storeCommand(fmt.Sprintf("command:change-%d", n), "run:changing", int64(n+1)), func(Snapshot) (Change, error) { return change, nil })
			if err != nil {
				done <- err
				return
			}
			if result.Receipt.Rejection != nil {
				done <- fmt.Errorf("unexpected refusal: %+v", result.Receipt.Rejection)
				return
			}
		}
		done <- nil
	}()
	for n := 0; n < 64; n++ {
		view, err := s.ReadMonitor(ctx, "run:changing")
		if err != nil {
			t.Fatal(err)
		}
		var state struct {
			Held bool `json:"held"`
		}
		if err = json.Unmarshal(view.Snapshot.Data, &state); err != nil {
			t.Fatal(err)
		}
		if state.Held != (view.Resources["heavy"] == 1) {
			t.Fatalf("mixed snapshot/resources at cut %d: %s %+v", view.Cut, view.Snapshot.Data, view.Resources)
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestMonitorReadReportsAdmissionWithoutAdvancingTheRun(t *testing.T) {
	t.Parallel()
	s, _ := testStore(t)
	ctx := context.Background()
	held := storeChange(`{}`)
	held.AcquireSlot = "slot:holder"
	applyChange(t, s, storeCommand("command:holder", "run:holder", 0), held)
	applyChange(t, s, storeCommand("command:create", "run:waiter", 0), storeChange(`{}`))
	admission := storeChange(`{"must_not_commit":true}`)
	admission.AcquireSlot = "slot:waiter"
	result := applyChange(t, s, storeCommand("command:wait", "run:waiter", 1), admission)
	if result.Receipt.Rejection == nil {
		t.Fatal("fixture admitted beyond capacity")
	}
	view, err := s.ReadMonitor(ctx, "run:waiter")
	if err != nil || !view.Waiting || view.Snapshot.Version != 1 || string(view.Snapshot.Data) != "{}" {
		t.Fatalf("waiter or original revision lost: %+v %v", view, err)
	}
}

func TestMonitorReadLeavesLegacyStorageUnmigrated(t *testing.T) {
	t.Parallel()
	s, dir := testStore(t)
	applyChange(t, s, storeCommand("command:legacy", "run:legacy", 0), storeChange(`{"legacy":true}`))
	if _, err := s.db.Exec("DROP TABLE attempt_output; DROP TABLE attempt_progress; DROP TABLE slot_resources; DROP TABLE pinned_bytes; DROP TABLE slot_waiters; ALTER TABLE runs DROP COLUMN snapshot_packed; ALTER TABLE events DROP COLUMN state_packed; ALTER TABLE authority DROP COLUMN admission_seq; ALTER TABLE authority DROP COLUMN verified_cut; PRAGMA user_version=3"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	options := storeTestOptions
	options.ReadOnly = true
	legacy, err := OpenStore(dir, options)
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.Close()
	view, err := legacy.ReadMonitor(context.Background(), "run:legacy")
	if err != nil || view.Resources != nil || view.Waiting || string(view.Snapshot.Data) != `{"legacy":true}` {
		t.Fatalf("legacy monitor: %+v %v", view, err)
	}
	var version int
	if err = legacy.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 3 {
		t.Fatalf("read migrated legacy storage: %d %v", version, err)
	}
}
