package runtime

import (
	"bytes"
	"context"
	"testing"
)

// A completed reader's join decision is history. Cancelling the other reader
// must settle the Run without changing that decision or inventing a result.
func TestParallelCancellationAfterEitherReaderCompletes(t *testing.T) {
	t.Parallel()
	for _, branchID := range []string{"opus", "sonnet"} {
		t.Run(branchID, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			e, runID := reviewFanOut(t, 2)
			if err := e.Drive(ctx, runID); err != nil {
				t.Fatal(err)
			}
			r := driverRun(t, e, runID)
			held := awaitingReviewers(t, r)
			if len(held) != 2 {
				t.Fatalf("expected two active readers, got %d", len(held))
			}
			for _, attempt := range held {
				if r.Invocations[r.Activations[attempt.ActivationID].InvocationID].BranchID == branchID {
					submitProposal(t, e, runID, attempt.ID)
				}
			}
			r = driverRun(t, e, runID)
			a := r.activationForInvocation(r.RootInvocationID, "refine")
			if len(awaitingReviewers(t, r)) != 1 || a.Parallel.LastDecision == nil || a.Parallel.LastDecision.Route != "recorded" {
				t.Fatalf("expected one completed and one waiting reader: %+v", a.Parallel)
			}
			progress := canonicalOf(t, a.Parallel)
			history, _ := joinHistory(t, e, runID)
			if _, err := e.Restrict(ctx, RestrictCommand{SchemaVersion: "1", CommandID: newID("command"), Scope: "run", ScopeID: runID, Kind: "cancel", Reason: "Stop the remaining read-only reviewer"}); err != nil {
				t.Fatal(err)
			}
			// A cancel request alone cannot close an admitted reader.
			rejectionCode(t, e.finishCancellation(ctx, runID, r.RootInvocationID), "unsettled")
			if err := e.Drive(ctx, runID); err != nil {
				t.Fatal(err)
			}
			r, view, err := e.load(ctx, runID)
			if err != nil {
				t.Fatal(err)
			}
			if r.Status != "cancelled" || r.Outcome != nil || len(r.Active) != 0 || r.HasUnresolvedEffects {
				t.Fatalf("settled read-only cancellation did not finish: %s %+v", r.Status, r.Outcome)
			}
			for _, attempt := range r.Attempts {
				if attempt.Settled == nil {
					t.Fatalf("cancelled Run has an unsettled attempt: %s", attempt.ID)
				}
			}
			a = r.Activations[a.ID]
			if a.Status != "cancelled" || a.Settled == nil || a.Parallel.ResultsRef != nil {
				t.Fatalf("cancellation invented a join result or left it running: %+v", a)
			}
			afterHistory, _ := joinHistory(t, e, runID)
			if !bytes.Equal(progress, canonicalOf(t, a.Parallel)) || !bytes.Equal(canonicalOf(t, history), canonicalOf(t, afterHistory)) {
				t.Fatal("cancellation rewrote parallel progress or added a join decision")
			}
			if err := e.Drive(ctx, runID); err != nil {
				t.Fatal(err)
			}
			_, repeated, err := e.load(ctx, runID)
			if err != nil || repeated.Snapshot.Version != view.Snapshot.Version {
				t.Fatalf("repeated drive changed terminal cancellation: %v", err)
			}
		})
	}
}

func TestCancelledParallelStillValidatesItsHistory(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e, runID := reviewFanOut(t, 2)
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatal(err)
	}
	held := awaitingReviewers(t, driverRun(t, e, runID))
	if len(held) != 2 {
		t.Fatalf("expected two readers, got %d", len(held))
	}
	submitProposal(t, e, runID, held[0].ID)
	if _, err := e.Restrict(ctx, RestrictCommand{SchemaVersion: "1", CommandID: newID("command"), Scope: "run", ScopeID: runID, Kind: "cancel", Reason: "Cancel after recording one branch"}); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatal(err)
	}
	snapshot := canonicalOf(t, driverRun(t, e, runID))
	mutations := []struct {
		name   string
		mutate func(*Run, *Activation)
	}{
		{"without cancel request", func(r *Run, _ *Activation) { r.CancelRequested = false }},
		{"only branch cancellation", func(r *Run, a *Activation) {
			r.CancelRequested = false
			for _, inv := range r.Invocations {
				if inv.CallerActivationID == a.ID {
					inv.CancelRequested = true
				}
			}
		}},
		{"completed instead of cancelled", func(_ *Run, a *Activation) { a.Status = "completed" }},
		{"live branch", func(r *Run, a *Activation) {
			for _, inv := range r.Invocations {
				if inv.CallerActivationID == a.ID && inv.Status == "cancelled" {
					inv.Status, inv.Settled = "running", nil
				}
			}
		}},
		{"invented summary", func(_ *Run, a *Activation) { a.Parallel.ResultsRef = &ArtifactRef{} }},
		{"changed decision counts", func(_ *Run, a *Activation) { a.Parallel.LastDecision.ObservedCount++ }},
		{"changed branch result", func(_ *Run, a *Activation) { a.Parallel.LastDecision.BranchStatus = "cancelled" }},
		{"invented next branch", func(_ *Run, a *Activation) { a.Parallel.LastDecision.NextBranchID = "extra" }},
	}
	for _, route := range []string{"recorded", "cancelling"} {
		t.Run(route, func(t *testing.T) {
			t.Parallel()
			var r Run
			if err := decodeState(snapshot, &r); err != nil {
				t.Fatal(err)
			}
			a := r.activationForInvocation(r.RootInvocationID, "refine")
			if route == "cancelling" {
				// A quorum may already have fixed its verdict before a separate
				// owner cancellation settles the whole fan-out.
				d := a.Parallel.LastDecision
				d.Route, d.Mode, d.Selection = route, "quorum", "first_observed"
				d.Verdict, d.Remainder, d.RemainderDisposition = "satisfied", "cancel", "cancel_requested"
			}
			if err := r.parallelProgressInvariant(a); err != nil {
				t.Fatalf("valid cancelled history was refused: %v", err)
			}
			baseline := canonicalOf(t, r)
			for _, tc := range mutations {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()
					var changed Run
					if err := decodeState(baseline, &changed); err != nil {
						t.Fatal(err)
					}
					activation := changed.Activations[a.ID]
					tc.mutate(&changed, activation)
					if err := changed.parallelProgressInvariant(activation); err == nil {
						t.Fatal("cancelled activation accepted invalid parallel history")
					}
				})
			}
		})
	}
}
