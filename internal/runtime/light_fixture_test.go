package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Opt-in only: produces real native records for browser/performance replay.
func TestLightExportPerformanceFixtures(t *testing.T) {
	t.Parallel()
	target := os.Getenv("PRIFLY_LIGHT_PERF_FIXTURE_DIR")
	if target == "" {
		t.Skip("used by test/e2e/verify-light.py")
	}
	e, options := emptyRuntime(t)
	ctx := context.Background()
	ids := []string{}
	for i := 0; i < 1000; i++ {
		options.CommandID = fmt.Sprintf("command:light-run-%04d", i)
		started, err := e.Start(ctx, options)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, started.Receipt.RunID)
	}
	// One test-only bootstrap uses the same native queue validator and pure
	// authority transaction. Measuring repeated queue writers is outside Light;
	// normal QueueAdd/claim/continue paths have separate integration tests.
	tasks := map[string]QueueTask{}
	for i := 0; i < 1000; i++ {
		id := fmt.Sprintf("queue-task:light-%04d", i)
		title := fmt.Sprintf("Задача %04d", i)
		body := queueTaskInput(t, title)
		priority := int64(i % 4)
		tasks[id] = QueueTask{ID: id, Revision: 1, Title: title, Priority: priority, Ordinal: int64(i + 1), State: "ready", Revisions: []QueueTaskRevision{{Revision: 1, RequestBytes: body, RequestDigest: rawDigest(body), Priority: priority}}, Intents: map[string]QueueLaunchIntent{}, Associations: []QueueRunAssociation{}}
	}
	seeded, err := e.queueApply(ctx, "command:light-fixture-seed", "fixture.seed", map[string]int{"tasks": 1000}, func(queue *TaskQueue) (any, error) {
		queue.Tasks = tasks
		queue.NextOrdinal = 1001
		return map[string]int{"tasks": 1000}, nil
	})
	if err != nil || seeded.Receipt.Rejection != nil {
		t.Fatalf("native queue bootstrap: %+v %v", seeded, err)
	}
	cache := NewLightCache()
	started := time.Now()
	first, err := e.Light(ctx, cache, nil)
	if err != nil {
		t.Fatal(err)
	}
	cold := time.Since(started)
	if len(first.Cards) != 2000 || first.Decodes != 1000 {
		t.Fatalf("cold population: %d / %d", len(first.Cards), first.Decodes)
	}
	started = time.Now()
	second, err := e.Light(ctx, cache, nil)
	if err != nil {
		t.Fatal(err)
	}
	warm := time.Since(started)
	if second.Decodes != 0 || second.ArtifactReads != 0 || second.QueueDecodes != 0 {
		t.Fatalf("unchanged work: %+v", second)
	}
	if err := e.Drive(ctx, ids[0]); err != nil {
		t.Fatal(err)
	}
	changed, err := e.Light(ctx, cache, nil)
	if err != nil || changed.Decodes != 1 {
		t.Fatalf("one changed Run: %d %v", changed.Decodes, err)
	}
	if _, err := e.QueueAdd(ctx, QueueAddRequest{CommandID: "command:light-queue-only", RequestBytes: queueTaskInput(t, "Добавленная задача")}); err != nil {
		t.Fatal(err)
	}
	queueOnly, err := e.Light(ctx, cache, nil)
	if err != nil || queueOnly.Decodes != 0 || queueOnly.QueueDecodes != 1 {
		t.Fatalf("queue-only work: %+v %v", queueOnly, err)
	}
	if _, err := e.SetAdmissionCapacity(ctx, CapacityRequest{CommandID: "command:light-admission-only", Capacity: 2, Reason: "Light fixture"}); err != nil {
		t.Fatal(err)
	}
	admission, err := e.Light(ctx, cache, nil)
	if err != nil || admission.Decodes != 0 || admission.Cut <= queueOnly.Cut {
		t.Fatalf("admission-only invalidation: %+v %v", admission, err)
	}
	metrics := map[string]any{"tasks": 1001, "runs": 1000, "cards": len(admission.Cards), "cold_index_ms": float64(cold.Microseconds()) / 1000, "warm_index_ms": float64(warm.Microseconds()) / 1000, "cold_decodes": first.Decodes, "unchanged_decodes": second.Decodes, "unchanged_artifact_reads": second.ArtifactReads, "one_changed_decodes": changed.Decodes, "queue_only_decodes": queueOnly.Decodes, "queue_only_queue_decodes": queueOnly.QueueDecodes, "admission_only_decodes": admission.Decodes, "run_id": ids[0]}
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := monitorCopyAuthority(filepath.Join(target, "performance"), e.Root); err != nil {
		t.Fatal(err)
	}
	body, _ := json.MarshalIndent(metrics, "", "  ")
	if err := os.WriteFile(filepath.Join(target, "performance.json"), body, 0600); err != nil {
		t.Fatal(err)
	}
	if cold > 5*time.Second {
		t.Fatalf("cold index budget exceeded: %v", cold)
	}
}

func TestLightExportLongHistoryFixture(t *testing.T) {
	t.Parallel()
	target := os.Getenv("PRIFLY_LIGHT_PERF_FIXTURE_DIR")
	if target == "" {
		t.Skip("used by local Light replay")
	}
	e, workflow, options := choiceFixture(t, `{"flag":true}`, "")
	stages := map[string]any{"done": choiceFinish("succeeded")}
	for i := 0; i < 350; i++ {
		name := fmt.Sprintf("choice:%04d", i)
		next := "done"
		if i < 349 {
			next = fmt.Sprintf("choice:%04d", i+1)
		}
		stages[name] = choiceStage("first_match", choiceBranch("yes", choiceFieldEqual("/flag", true), next))
	}
	workflow["definition"] = map[string]any{"entry": "choice:0000", "stages": stages}
	workflow["limits"] = map[string]any{"max_step_instances": 1, "max_control_transitions": 1024, "max_parallelism": 1, "max_child_depth": 0}
	id := choiceStart(t, e, workflow, options)
	if err := e.Drive(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	revision, err := e.Store.ReadMonitor(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if revision.Snapshot.EventSeq < 1000 {
		t.Fatalf("history fixture too short: %d", revision.Snapshot.EventSeq)
	}
	view, err := e.Light(context.Background(), NewLightCache(), nil)
	if err != nil || len(view.Cards) != 1 {
		t.Fatalf("long history projection: %+v %v", view, err)
	}
	metrics := map[string]any{"run_id": id, "events": revision.Snapshot.EventSeq, "light_snapshot_decodes": view.Decodes, "light_artifact_reads": view.ArtifactReads}
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	if err := monitorCopyAuthority(filepath.Join(target, "long-history"), e.Root); err != nil {
		t.Fatal(err)
	}
	body, _ := json.MarshalIndent(metrics, "", "  ")
	if err := os.WriteFile(filepath.Join(target, "long-history.json"), body, 0600); err != nil {
		t.Fatal(err)
	}
}
