//go:build cgo

package local

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

func TestLightReadConsistentCutAndChangedSnapshots(t *testing.T) {
	t.Parallel()
	s, _ := testStore(t)
	ctx := context.Background()
	applyChange(t, s, storeCommand("command:light-create", "run:light", 0), storeChange(`{"held":false}`))
	done := make(chan error, 1)
	go func() {
		for i := 0; i < 24; i++ {
			held := i%2 == 0
			change := storeChange(fmt.Sprintf(`{"held":%t}`, held))
			if held {
				change.AcquireSlot = "slot:light"
				change.AcquireResources = []ResourceClaim{{Name: "heavy", Limit: 1}}
			} else {
				change.ReleaseSlot = "slot:light"
			}
			result, err := s.Apply(ctx, storeCommand(fmt.Sprintf("command:light-%d", i), "run:light", int64(i+1)), func(Snapshot) (Change, error) { return change, nil })
			if err != nil {
				done <- err
				return
			}
			if result.Receipt.Rejection != nil {
				done <- fmt.Errorf("rejected: %+v", result.Receipt)
				return
			}
		}
		done <- nil
	}()
	for range 32 {
		view, err := s.ReadLight(ctx, "task_queue", nil)
		if err != nil {
			t.Fatal(err)
		}
		var body struct {
			Held bool `json:"held"`
		}
		if err := json.Unmarshal(view.Changed["run:light"].Data, &body); err != nil {
			t.Fatal(err)
		}
		if body.Held != (view.Resources["heavy"] == 1) {
			t.Fatalf("mixed cut %d: %+v", view.Cut, view)
		}
		if view.Queue.Version != 0 {
			t.Fatal("read initialized queue")
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	first, err := s.ReadLight(ctx, "task_queue", nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.ReadLight(ctx, "task_queue", map[string]int64{"run:light": first.Revisions[0].Version})
	if err != nil || len(second.Changed) != 0 || second.Cut != first.Cut {
		t.Fatalf("unchanged read: %+v %v", second, err)
	}
}
func TestLightReadMetadataPagesHaveNoPopulationLimit(t *testing.T) {
	t.Parallel()
	s, _ := testStore(t)
	for i := 0; i < 251; i++ {
		applyChange(t, s, storeCommand(fmt.Sprintf("command:light-%d", i), fmt.Sprintf("run:light-%03d", i), 0), storeChange(`{}`))
	}
	view, err := s.ReadLight(context.Background(), "task_queue", nil)
	if err != nil || len(view.Revisions) != 251 || len(view.Changed) != 251 {
		t.Fatalf("truncated metadata: %d %v", len(view.Revisions), err)
	}
}
