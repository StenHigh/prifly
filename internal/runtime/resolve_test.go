package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/local"
)

// uncertainAssistedRun leaves one attempt in the state recovery is designed to
// preserve: dispatched, unproven, holding the authority's admission slot.
func uncertainAssistedRun(t *testing.T) (*Engine, string, string) {
	t.Helper()
	e, runID, _ := assistedFixture(t)
	ctx := context.Background()
	task := handOver(t, e, runID)
	_, view, err := e.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.apply(ctx, e.owner, newID("command"), runID, "diagnostic.recorded", map[string]any{"expire": true}, &view.Snapshot.Version, local.CommandCAS, func(r *Run, _ local.Snapshot, obs Observation) (local.Change, error) {
		attempt := r.Attempts[task.AttemptID]
		attempt.Deadline = attempt.Admitted
		return local.Change{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.MarkSessionDisconnected(ctx, runID, task.AttemptID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	if r.Status != "uncertain" || !r.HasUnresolvedEffects || r.Attempts[task.AttemptID].Status != "uncertain" {
		t.Fatalf("the obligation did not carry the uncertainty: %+v", r.Attempts[task.AttemptID])
	}
	return e, runID, task.AttemptID
}

// An unproven obligation holds the slot until a person says what happened.
// Resolution is that statement: it closes the obligation, frees the slot and
// leaves the outcome recorded as attested rather than observed.
func TestResolveReleasesSlotAndNeverRoutesOnError(t *testing.T) {
	e, runID, attemptID := uncertainAssistedRun(t)
	ctx := context.Background()
	if slot, _, err := e.Store.Slot(ctx); err != nil || slot == "" {
		t.Fatalf("the uncertain obligation released its slot early: %q %v", slot, err)
	}
	view, err := e.View(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.ResolveObligation(ctx, runID, newID("command"), attemptID, "", ResolveOutcomeNotApplied, "the host was stopped before it wrote anything", view.RunVersion); err != nil {
		t.Fatalf("the owner could not resolve an unproven obligation: %v", err)
	}
	r := driverRun(t, e, runID)
	attempt := r.Attempts[attemptID]
	if attempt.Status != "failed" || attempt.Settled == nil || r.HasUnresolvedEffects || r.Status == "uncertain" {
		t.Fatalf("resolution left the run unresolved: status=%s attempt=%+v", r.Status, attempt)
	}
	if len(r.Active) != 0 {
		t.Fatalf("the resolved attempt is still active: %v", r.Active)
	}
	// An attested unknown is not a known technical failure, so no declared
	// handler consumes it.
	for _, d := range r.Diagnostics {
		if d.Code == "resolved_not_applied" && d.Phase == "resolution" {
			goto recorded
		}
	}
	t.Fatalf("the attestation was not recorded as a diagnostic: %+v", r.Diagnostics)
recorded:
	for _, event := range runEvents(t, e, runID) {
		if event == "stage.error_handled" {
			t.Fatal("an attested unknown was routed through a declared error handler")
		}
	}
	if slot, _, err := e.Store.Slot(ctx); err != nil || slot != "" {
		t.Fatalf("the slot stayed held after resolution: %q %v", slot, err)
	}
}

// Resolution is refused while a driver still owns the Run: an obligation its
// owner still holds may yet settle by itself.
func TestResolveRefusesLiveDriver(t *testing.T) {
	e, runID, attemptID := uncertainAssistedRun(t)
	ctx := context.Background()
	lock, err := e.driverLock(runID)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	view, err := e.View(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.ResolveObligation(ctx, runID, newID("command"), attemptID, "", ResolveOutcomeNotApplied, "owner attests nothing ran", view.RunVersion)
	if refusalCode(err) != "driver_active" {
		t.Fatalf("resolution overrode a live driver: %v", err)
	}
	if r := driverRun(t, e, runID); r.Attempts[attemptID].Status != "uncertain" || !r.HasUnresolvedEffects {
		t.Fatal("a refused resolution changed the obligation")
	}
}

// The command states what it needs and refuses guesses.
func TestResolveRequiresAnExactAttestation(t *testing.T) {
	e, runID, attemptID := uncertainAssistedRun(t)
	ctx := context.Background()
	view, err := e.View(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ name, attempt, check, outcome, reason string }{
		{"no target", "", "", ResolveOutcomeNotApplied, "why"},
		{"both targets", attemptID, "check:one", ResolveOutcomeNotApplied, "why"},
		{"unknown outcome", attemptID, "", "succeeded", "why"},
		{"no reason", attemptID, "", ResolveOutcomeNotApplied, ""},
	} {
		_, err := e.ResolveObligation(ctx, runID, newID("command"), test.attempt, test.check, test.outcome, test.reason, view.RunVersion)
		if refusalCode(err) != "invalid_resolution" {
			t.Fatalf("%s was accepted: %v", test.name, err)
		}
	}
	if _, err := e.ResolveObligation(ctx, runID, newID("command"), "attempt:absent", "", ResolveOutcomeNotApplied, "why", view.RunVersion); refusalCode(err) != "not_found" {
		t.Fatalf("an unknown attempt was resolved: %v", err)
	}
}

func runEvents(t *testing.T, e *Engine, runID string) []string {
	t.Helper()
	history, err := e.Store.Read(context.Background(), runID, 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	types := make([]string, 0, len(history.Events))
	for _, event := range history.Events {
		types = append(types, event.Type)
	}
	if strings.Join(types, "") == "" {
		t.Fatal("the run recorded no events")
	}
	return types
}
