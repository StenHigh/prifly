package runtime

import (
	"context"
	"fmt"
	"testing"
)

// The listing is a projection of recorded Runs, so what it reports must be what
// the Run holds: a monitor that counted differently would tell its own story.
func TestRunsListReportsWhatEachRunHolds(t *testing.T) {
	e, runID := reviewFanOut(t, 2)
	ctx := context.Background()
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatal(err)
	}
	runs, err := e.Runs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected exactly the started run, got %d", len(runs))
	}
	view, err := e.MonitorView(ctx, runID)
	if err != nil || len(view.Run.Workflow) == 0 || len(view.Run.Definitions) == 0 {
		t.Fatal("monitor lost pinned plan", err)
	}
	for _, a := range view.Run.Attempts {
		if len(a.Envelope) == 0 || a.TokenHash != "" {
			t.Fatal("missing task or exposed credential")
		}
	}
	public, err := e.View(ctx, runID)
	if err != nil || len(public.Run.Workflow) != 0 {
		t.Fatal("public view changed", err)
	}
	summary, r := runs[0], driverRun(t, e, runID)
	if summary.ID != r.ID || summary.Status != r.Status || summary.SchemaVersion != r.SchemaVersion {
		t.Fatalf("the summary does not match the run: %+v", summary)
	}
	if summary.Attempts != len(r.Attempts) || summary.Invocations != len(r.Invocations) || summary.Active != len(r.Active) {
		t.Fatalf("the summary counted something other than what the run holds: %+v", summary)
	}
	if summary.AwaitingHosts != len(awaitingReviewers(t, r)) {
		t.Fatalf("outstanding handoffs were miscounted: %d", summary.AwaitingHosts)
	}
	// A completed run stays in the listing: history is what a monitor is for.
	if summary.WorkflowID != r.WorkflowRef.ID {
		t.Fatalf("the summary named another workflow: %s", summary.WorkflowID)
	}
}

func TestMonitorHistoryAndCreationCallback(t *testing.T) {
	e, options := emptyRuntime(t)
	ctx := context.Background()
	called := 0
	e.AfterRunCreated = func() {
		called++
		// Reading here also proves that the creation write transaction has ended.
		row, err := e.MonitorSummary(ctx, startRunID(e.owner, options.CommandID))
		if err != nil || row.Version != 1 || row.Subject != "An explicit empty control path" {
			t.Fatalf("creation callback before commit: %+v %v", row, err)
		}
	}
	for i := 0; i < 205; i++ {
		options.CommandID = fmt.Sprintf("command:monitor-%03d", i)
		if _, err := e.Start(ctx, options); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := e.Runs(ctx)
	if err != nil || len(rows) != 205 || called != 205 {
		t.Fatalf("history truncated: %d callbacks %d, %v", len(rows), called, err)
	}
	options.WorkflowFile = "missing.json"
	if _, err := e.Start(ctx, options); err == nil {
		t.Fatal("invalid start accepted")
	}
	if called != 205 {
		t.Fatal("callback on failed creation")
	}
	e.owner = "local:uid:999999"
	if _, _, err := e.MonitorRevisions(ctx, ""); err == nil {
		t.Fatal("foreign reader enumerated runs")
	}
}
