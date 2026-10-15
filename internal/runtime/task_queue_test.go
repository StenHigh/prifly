package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

func queueTaskInput(t *testing.T, title string) []byte {
	t.Helper()
	input := TaskInput{SchemaVersion: TaskInputVersion, ID: "task:original", Title: title, RawText: "Original request\n  keep this spacing and meaning.\n", DesiredOutcome: "Preserve the local request", InScope: []string{"Local fixture"}, OutOfScope: []string{}, CompletionCriteria: []string{"Explicit recorded result"}, Source: TaskSource{Type: "local", ExternalID: "fixture:public", FetchedAt: "2026-10-15T00:00:00Z"}, SourceRefs: []ArtifactRef{}, Assumptions: []string{}, Confirmation: "explicit"}
	data, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(data, '\n')
}

func queueSummaryResult(t *testing.T, result local.AuthorityApplyResult, err error) QueueTaskSummary {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	if result.Receipt.Rejection != nil {
		t.Fatalf("queue mutation refused: %+v", result.Receipt.Rejection)
	}
	var task QueueTaskSummary
	if err := json.Unmarshal(result.Receipt.Result, &task); err != nil {
		t.Fatal(err)
	}
	return task
}

func TestTaskQueueRejectsMalformedRequestsWithoutCreatingState(t *testing.T) {
	t.Parallel()
	e, _ := emptyRuntime(t)
	ctx := context.Background()
	for name, request := range map[string]QueueAddRequest{
		"invalid input":               {CommandID: "command:bad-input", RequestBytes: []byte(`{}`)},
		"invalid utf8":                {CommandID: "command:bad-utf8", RequestBytes: []byte{0xff}},
		"priority outside JSON range": {CommandID: "command:bad-priority", RequestBytes: queueTaskInput(t, "task"), Priority: 1 << 53},
		"invalid command":             {CommandID: "a command with spaces", RequestBytes: queueTaskInput(t, "task")},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := e.QueueAdd(ctx, request); err == nil {
				t.Fatal("invalid queue request was accepted")
			}
			view, err := e.Queue(ctx)
			if err != nil || view.Initialized || len(view.Tasks) != 0 {
				t.Fatalf("invalid request initialized queue: %+v %v", view, err)
			}
		})
	}
}

func TestTaskQueueReadsDoNotEnrollLegacyAuthority(t *testing.T) {
	t.Parallel()
	e, _ := emptyRuntime(t)
	ctx := context.Background()
	for range 2 {
		view, err := e.Queue(ctx)
		if err != nil || view.Initialized || view.Version != 0 || len(view.Tasks) != 0 {
			t.Fatalf("legacy queue read: %+v %v", view, err)
		}
	}
	if _, err := e.Store.ReadAuthority(ctx, TaskQueueKey); !errors.Is(err, local.ErrNotFound) {
		t.Fatalf("read created authority queue: %v", err)
	}
	if _, err := e.QueueTask(ctx, "queue-task:missing"); refusalCode(err) != "queue_task_missing" {
		t.Fatalf("missing task: %v", err)
	}
	if _, err := e.Store.ReadAuthority(ctx, TaskQueueKey); !errors.Is(err, local.ErrNotFound) {
		t.Fatalf("missing task lookup created queue: %v", err)
	}
}

func TestTaskQueueKeepsExactRequestsAndRejectsStaleOwnership(t *testing.T) {
	t.Parallel()
	e, _ := emptyRuntime(t)
	ctx := context.Background()
	original := queueTaskInput(t, "same title")
	result, err := e.QueueAdd(ctx, QueueAddRequest{CommandID: "command:add-original", RequestBytes: original, Priority: 4})
	added := queueSummaryResult(t, result, err)
	result, err = e.QueueClaimNext(ctx, QueueClaimRequest{CommandID: "command:claim-original", Owner: "host:first"})
	claimed := queueSummaryResult(t, result, err)
	if claimed.ID != added.ID || claimed.Revision != 1 || claimed.Claim == nil || claimed.Claim.Generation != 1 {
		t.Fatalf("claim changed content or selected another task: %+v", claimed)
	}
	for _, change := range []QueueUpdateRequest{
		{CommandID: "command:stale-revision", TaskID: added.ID, ExpectedRevision: 2, Owner: "host:first", ClaimGeneration: 1, RequestBytes: original},
		{CommandID: "command:stale-owner", TaskID: added.ID, ExpectedRevision: 1, Owner: "host:second", ClaimGeneration: 1, RequestBytes: original},
		{CommandID: "command:stale-generation", TaskID: added.ID, ExpectedRevision: 1, Owner: "host:first", ClaimGeneration: 2, RequestBytes: original},
	} {
		before, _ := e.Queue(ctx)
		result, err := e.QueueUpdate(ctx, change)
		if err != nil || result.Receipt.Rejection == nil {
			t.Fatalf("stale update: %+v %v", result, err)
		}
		after, err := e.Queue(ctx)
		if err != nil || after.Version != before.Version {
			t.Fatalf("refusal changed queue revision: %+v %+v %v", before, after, err)
		}
	}
	changed := queueTaskInput(t, "new title")
	result, err = e.QueueUpdate(ctx, QueueUpdateRequest{CommandID: "command:update-original", TaskID: added.ID, ExpectedRevision: 1, Owner: "host:first", ClaimGeneration: 1, RequestBytes: changed, Priority: 7})
	updated := queueSummaryResult(t, result, err)
	if updated.Revision != 2 || updated.Claim.Generation != 1 {
		t.Fatalf("content edit changed ownership: %+v", updated)
	}
	view, err := e.QueueTask(ctx, added.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Task.Revisions) != 2 || !bytes.Equal(view.Task.Revisions[0].RequestBytes, original) || !bytes.Equal(view.Task.Revisions[1].RequestBytes, changed) {
		t.Fatalf("original TaskInput bytes were rewritten: %+v", view.Task.Revisions)
	}
	other, _ := emptyRuntime(t)
	if _, err := other.QueueTask(ctx, added.ID); refusalCode(err) != "queue_task_missing" {
		t.Fatalf("task crossed authorities: %v", err)
	}
}

func TestTaskQueueConcurrentClaimsAreDistinctAndReplayExact(t *testing.T) {
	t.Parallel()
	e, _ := emptyRuntime(t)
	ctx := context.Background()
	ids := map[string]bool{}
	for i, priority := range []int64{0, 10, 10} {
		result, err := e.QueueAdd(ctx, QueueAddRequest{CommandID: fmt.Sprintf("command:add-%d", i), RequestBytes: queueTaskInput(t, "equal title"), Priority: priority})
		task := queueSummaryResult(t, result, err)
		if i > 0 {
			ids[task.ID] = true
		}
	}
	type outcome struct {
		result local.AuthorityApplyResult
		err    error
	}
	results := make(chan outcome, 2)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for i := range 2 {
		workers.Go(func() {
			<-start
			result, err := e.QueueClaimNext(ctx, QueueClaimRequest{CommandID: fmt.Sprintf("command:parallel-claim-%d", i), Owner: fmt.Sprintf("host:%d", i)})
			results <- outcome{result, err}
		})
	}
	close(start)
	workers.Wait()
	close(results)
	seen := map[string]bool{}
	for result := range results {
		task := queueSummaryResult(t, result.result, result.err)
		if !ids[task.ID] || seen[task.ID] {
			t.Fatalf("concurrent claims selected same or lower-priority task: %+v", task)
		}
		seen[task.ID] = true
	}
	first, err := e.QueueClaimNext(ctx, QueueClaimRequest{CommandID: "command:parallel-claim-0", Owner: "host:0"})
	task := queueSummaryResult(t, first, err)
	if !first.Duplicate || !seen[task.ID] {
		t.Fatalf("retry did not return original claim: %+v", first)
	}
	before, err := e.Queue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.QueueClaimNext(ctx, QueueClaimRequest{CommandID: "command:parallel-claim-0", Owner: "host:changed"}); !errors.Is(err, local.ErrCommandConflict) {
		t.Fatalf("same identity/different payload: %v", err)
	}
	after, err := e.Queue(ctx)
	if err != nil || after.Version != before.Version {
		t.Fatalf("command conflict changed queue: %+v %+v %v", before, after, err)
	}
	remaining, err := e.QueueClaimNext(ctx, QueueClaimRequest{CommandID: "command:last-task", Owner: "host:last"})
	last := queueSummaryResult(t, remaining, err)
	if last.Priority != 0 {
		t.Fatalf("priority ordering lost: %+v", last)
	}
}

func TestTaskQueueClosedDocumentsAndAuthorityBinding(t *testing.T) {
	t.Parallel()
	e, _ := emptyRuntime(t)
	ctx := context.Background()
	result, err := e.QueueAdd(ctx, QueueAddRequest{CommandID: "command:closed-queue", RequestBytes: queueTaskInput(t, "closed task")})
	added := queueSummaryResult(t, result, err)
	snapshot, err := e.Store.ReadAuthority(ctx, TaskQueueKey)
	if err != nil {
		t.Fatal(err)
	}
	view, err := e.Queue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	task, err := e.QueueTask(ctx, added.ID)
	if err != nil {
		t.Fatal(err)
	}
	for name, document := range map[string]any{"ProjectTaskQueueRead": view, "ProjectQueueTaskRead": task} {
		data, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		if err := flow.ValidateProtocol(name, data); err != nil {
			t.Fatalf("actual %s output: %v", name, err)
		}
	}
	if err := flow.ValidateProtocol("ProjectTaskQueue", snapshot.Data); err != nil {
		t.Fatal(err)
	}
	var damaged map[string]any
	if err := json.Unmarshal(snapshot.Data, &damaged); err != nil {
		t.Fatal(err)
	}
	damaged["undocumented_field"] = true
	bad, err := json.Marshal(damaged)
	if err != nil {
		t.Fatal(err)
	}
	if flow.ValidateProtocol("ProjectTaskQueue", bad) == nil {
		t.Fatal("queue schema accepted undocumented field")
	}
	if _, err := e.queueSnapshot(local.AuthoritySnapshot{Version: 1, Data: bad}); refusalCode(err) != "queue_incompatible" {
		t.Fatalf("malformed stored queue was readable: %v", err)
	}
	other, _ := emptyRuntime(t)
	if _, err := other.queueSnapshot(snapshot); refusalCode(err) != "queue_incompatible" {
		t.Fatalf("foreign authority queue was accepted: %v", err)
	}
}

func TestTaskQueueMalformedOwnerCannotAliasAnAcceptedReceipt(t *testing.T) {
	t.Parallel()
	e, _ := emptyRuntime(t)
	ctx := context.Background()
	added, err := e.QueueAdd(ctx, QueueAddRequest{CommandID: "command:add-unicode-owner", RequestBytes: queueTaskInput(t, "unicode owner")})
	queueSummaryResult(t, added, err)
	request := QueueClaimRequest{CommandID: "command:claim-unicode-owner", Owner: "host:\ufffd"}
	first, err := e.QueueClaimNext(ctx, request)
	queueSummaryResult(t, first, err)
	request.Owner = "host:" + string([]byte{0xff})
	replay, err := e.QueueClaimNext(ctx, request)
	if err == nil || replay.Duplicate {
		t.Fatalf("malformed UTF-8 aliased a different accepted owner: %+v %v", replay, err)
	}
}
