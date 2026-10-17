package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

func TestLightQueueBeforeRunAndNoReadMutation(t *testing.T) {
	t.Parallel()
	e, _ := emptyRuntime(t)
	ctx := context.Background()
	cache := NewLightCache()
	before, err := e.Light(ctx, cache, nil)
	if err != nil || len(before.Cards) != 0 {
		t.Fatalf("legacy read: %+v %v", before, err)
	}
	for i := 0; i < 3; i++ {
		_, err := e.QueueAdd(ctx, QueueAddRequest{CommandID: fmt.Sprintf("command:light-task-%d", i), RequestBytes: queueTaskInput(t, "same title"), Priority: int64(i)})
		if err != nil {
			t.Fatal(err)
		}
	}
	view, err := e.Light(ctx, cache, nil)
	if err != nil || len(view.Cards) != 3 {
		t.Fatalf("queue projection: %+v %v", view, err)
	}
	for _, c := range view.Cards {
		if c.Kind != "queue_task" || c.TaskID == nil || c.Lane != "queue" || c.HistoryCount != 0 || c.Round != nil || len(c.Reviews) != 0 {
			t.Fatalf("invented execution: %+v", c)
		}
	}
	unchanged, err := e.Light(ctx, cache, nil)
	if err != nil || unchanged.Decodes != 0 || unchanged.QueueDecodes != 0 || unchanged.ArtifactReads != 0 || unchanged.Cut != view.Cut {
		t.Fatalf("unchanged projection: %+v %v", unchanged, err)
	}
}
func TestLightStandaloneRunRemainsIndependent(t *testing.T) {
	t.Parallel()
	e, options := emptyRuntime(t)
	started, startErr := e.Start(context.Background(), options)
	if startErr != nil {
		t.Fatal(startErr)
	}
	id := started.Receipt.RunID
	cache := NewLightCache()
	ctx := context.Background()
	view, err := e.Light(ctx, cache, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Cards) != 1 || view.Cards[0].Kind != "run" || view.Cards[0].TaskID != nil || view.Cards[0].CurrentRuns[0].ID != id {
		t.Fatalf("fallback: %+v", view)
	}
	second, err := e.Light(ctx, cache, nil)
	if err != nil || second.Decodes != 0 || second.ArtifactReads != 0 {
		t.Fatalf("uncached Run: %+v %v", second, err)
	}
}
func TestLightTaskFrontierDoesNotSelectLatestTimestamp(t *testing.T) {
	t.Parallel()
	succeeded, partial := "succeeded", "partial"
	state := "claimed"
	card := LightCard{Kind: "queue_task", QueueState: &state, Reasons: []MonitorReason{}}
	runs := []LightRun{{ID: "run:source", Status: "completed", Outcome: &partial}, {ID: "run:a", Parent: "run:source", Status: "completed", Outcome: &succeeded}, {ID: "run:b", Parent: "run:source", Status: "cancelled", LastObserved: "2099-01-01", Board: &MonitorBoard{Category: "terminal"}}}
	lightSummarize(&card, runs)
	if card.CurrentCount != 2 || card.HistoryCount != 3 || card.Status != "mixed" || card.Outcome != nil || card.Lane != "attention" {
		t.Fatalf("lost branch: %+v", card)
	}
	card = LightCard{Kind: "queue_task", QueueState: &state}
	lightSummarize(&card, runs[1:2])
	if card.Lane != "result" {
		t.Fatalf("Run success became task done: %+v", card)
	}
}
func TestLightMappingRejectsUnknownAndAmbiguousDeclarations(t *testing.T) {
	t.Parallel()
	for _, s := range []string{`{"schema_version":"wrong","workflows":[]}`, `{"schema_version":"prifly-light-projection/1","workflows":[],"script":"bad"}`, `{"schema_version":"prifly-light-projection/1","workflows":[]} {}`} {
		if _, err := ParseLightProjection([]byte(s)); err == nil {
			t.Fatalf("accepted invalid mapping: %s", s)
		}
	}
	p, err := ParseLightProjection([]byte(`{"schema_version":"prifly-light-projection/1","workflows":[]}`))
	if err != nil || p.Digest == "" {
		t.Fatal(err)
	}
	ref := flow.Ref{ID: "test:workflow/plain", Version: "1.0.0", Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	p.Workflows = []LightWorkflowMapping{{WorkflowRef: ref}, {WorkflowRef: ref}}
	body, _ := json.Marshal(p)
	if _, err := ParseLightProjection(body); err == nil {
		t.Fatal("duplicate mapping accepted")
	}
}
