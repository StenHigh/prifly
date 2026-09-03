package runtime

import (
	"context"
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
