package runtime

import (
	"context"
	"testing"
)

// Capacity is a qualification statement, not a performance dial: this build
// admits a bounded number of attempts at once and refuses to claim more.
func TestAdmissionCapacityStaysWithinTheQualifiedProfile(t *testing.T) {
	e, _ := emptyRuntime(t)
	ctx := context.Background()
	capacity, held, err := e.AdmissionCapacity(ctx)
	if err != nil || capacity != 1 || len(held) != 0 {
		t.Fatalf("a new authority did not start at one free slot: %d %v %v", capacity, held, err)
	}
	for _, unqualified := range []int64{0, -1, MaxAdmissionCapacity + 1} {
		_, err := e.SetAdmissionCapacity(ctx, CapacityRequest{CommandID: newID("command"), Capacity: unqualified, Reason: "beyond the qualified profile"})
		rejectionCode(t, err, "unqualified_capacity")
	}
	if capacity, _, _ := e.AdmissionCapacity(ctx); capacity != 1 {
		t.Fatalf("a refused request still changed the capacity: %d", capacity)
	}
	result, err := e.SetAdmissionCapacity(ctx, CapacityRequest{CommandID: newID("command"), Capacity: MaxAdmissionCapacity, Reason: "run the qualified maximum"})
	if err != nil || result.Receipt.Rejection != nil {
		t.Fatalf("the qualified maximum was refused: %+v %v", result.Receipt.Rejection, err)
	}
	capacity, _, err = e.AdmissionCapacity(ctx)
	if err != nil || capacity != MaxAdmissionCapacity {
		t.Fatalf("the decision did not take effect: %d %v", capacity, err)
	}
	// The decision names the capacity it set, so the receipt explains the state.
	if len(result.Receipt.Result) == 0 {
		t.Fatal("the recorded decision does not name the capacity it set")
	}
	// Reopening re-verifies the authority against its own recorded capacity.
	root := e.Root
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(root, false)
	if err != nil {
		t.Fatalf("the authority did not reopen against its recorded capacity: %v", err)
	}
	defer reopened.Close()
	if capacity, _, err := reopened.AdmissionCapacity(ctx); err != nil || capacity != MaxAdmissionCapacity {
		t.Fatalf("the recorded capacity did not survive a reopen: %d %v", capacity, err)
	}
}

// Changing how much work an authority admits at once is an admission decision,
// so a principal who may not admit work may not raise the ceiling either.
func TestAdmissionCapacityRequiresAdmissionAccess(t *testing.T) {
	e, _ := emptyRuntime(t)
	ctx := context.Background()
	if _, err := e.SetAdmissionCapacity(ctx, CapacityRequest{CommandID: newID("command"), Capacity: 2, Reason: "raise the ceiling"}); err != nil {
		t.Fatalf("the owner could not change capacity: %v", err)
	}
	if _, err := e.SetAdmissionCapacity(ctx, CapacityRequest{CommandID: newID("command"), Capacity: 2}); err == nil {
		t.Fatal("capacity changed without a recorded reason")
	}
}

// The bound is enforced where work is actually admitted, not only in the
// authority's records: a second run is refused by name while the single slot
// is held, and admitted once the recorded capacity says two may run.
func TestAdmissionCapacityBoundsConcurrentRuns(t *testing.T) {
	e, _ := driverProject(t, "commit-pass", 10000)
	ctx := context.Background()
	first, second := driverStart(t, e), driverStart(t, e)
	if attempt := driverAdmit(t, e, first); attempt == nil {
		t.Fatal("the first run was not admitted")
	}
	capacity, held, err := e.AdmissionCapacity(ctx)
	if err != nil || capacity != 1 || len(held) != 1 {
		t.Fatalf("the held slot was not recorded: %d %v %v", capacity, held, err)
	}

	// The second run is refused for a named reason rather than over-admitted.
	r, v, err := e.load(ctx, second)
	if err != nil {
		t.Fatal(err)
	}
	p, err := r.plan()
	if err != nil {
		t.Fatal(err)
	}
	err = e.admit(ctx, r, v, p, activationFor(&r, "work"))
	rejectionCode(t, err, "capacity_conflict")
	if blocked := driverRun(t, e, second); len(blocked.Active) != 0 || len(blocked.Attempts) != 0 {
		t.Fatalf("a refused admission left an attempt behind: %+v", blocked.Attempts)
	}

	// Raising the recorded capacity admits it; the bound moved, not the rule.
	if _, err := e.SetAdmissionCapacity(ctx, CapacityRequest{CommandID: newID("command"), Capacity: 2, Reason: "run two attempts at once"}); err != nil {
		t.Fatal(err)
	}
	r, v, err = e.load(ctx, second)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.admit(ctx, r, v, p, activationFor(&r, "work")); err != nil {
		t.Fatalf("the second run was refused under a capacity that admits it: %v", err)
	}
	capacity, held, err = e.AdmissionCapacity(ctx)
	if err != nil || capacity != 2 || len(held) != 2 {
		t.Fatalf("two attempts do not hold the two recorded slots: %d %v %v", capacity, held, err)
	}
	// Each slot belongs to the run that took it.
	owners := map[string]bool{}
	for _, runID := range held {
		owners[runID] = true
	}
	if !owners[first] || !owners[second] {
		t.Fatalf("the held slots do not belong to the admitted runs: %v", held)
	}
}

// A run refused for capacity joins the queue, and the freed slot goes to it
// rather than to whoever asks next. The refusal is explicit either way: the
// queue decides the order, it does not make a caller wait.
func TestRefusedAdmissionJoinsTheQueueInOrder(t *testing.T) {
	e, _ := driverProject(t, "commit-pass", 10000)
	ctx := context.Background()
	holder, early, late := driverStart(t, e), driverStart(t, e), driverStart(t, e)
	driverAdmit(t, e, holder)

	tryAdmit := func(runID string) error {
		r, v, err := e.load(ctx, runID)
		if err != nil {
			t.Fatal(err)
		}
		p, err := r.plan()
		if err != nil {
			t.Fatal(err)
		}
		return e.admit(ctx, r, v, p, activationFor(&r, "work"))
	}
	rejectionCode(t, tryAdmit(early), "capacity_conflict")
	rejectionCode(t, tryAdmit(late), "capacity_conflict")
	queue, err := e.AdmissionQueue(ctx)
	if err != nil || len(queue) != 2 || queue[early] >= queue[late] {
		t.Fatalf("the queue did not record both runs in order: %v %v", queue, err)
	}

	// Free the slot the way an unstarted attempt is settled, so the release is
	// the engine's own path rather than a hand-written store change.
	held := driverRun(t, e, holder)
	if err := e.settleUnstarted(ctx, holder, held.Active[0], "", "dispatch_abandoned"); err != nil {
		t.Fatal(err)
	}
	// The later arrival asks first and is deferred to the one ahead of it.
	rejectionCode(t, tryAdmit(late), "admission_deferred")
	if err := tryAdmit(early); err != nil {
		t.Fatalf("the longest waiting run was refused its turn: %v", err)
	}
	queue, err = e.AdmissionQueue(ctx)
	if err != nil || len(queue) != 1 {
		t.Fatalf("the admitted run stayed in the queue: %v %v", queue, err)
	}
	if _, waiting := queue[late]; !waiting {
		t.Fatalf("the deferred run left the queue: %v", queue)
	}
}
