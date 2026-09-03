package runtime

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// Advance only the already-created body through the normal admission, owned
// process runner and finish paths. No accepted Result or projection is seeded.
func repeatFinishBody(t *testing.T, e *Engine, runID string) (Run, local.ReadView, *flow.Plan, *Activation) {
	t.Helper()
	lock, err := e.driverLock(runID)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	r, view, p, a := callActivateReady(t, e, runID)
	bodyID := a.InvocationID
	if a.Kind == "step" {
		socket, closeServer, err := e.serveSteps()
		if err != nil {
			t.Fatal(err)
		}
		defer closeServer()
		if err := e.admit(context.Background(), r, view, p, a); err != nil {
			t.Fatal(err)
		}
		r, view, err = e.load(context.Background(), runID)
		if err != nil || len(r.Active) != 1 {
			t.Fatal("body did not admit its one owned process", err)
		}
		if err := e.executePending(context.Background(), r, view, r.Attempts[r.Active[0]], socket); err != nil {
			t.Fatal(err)
		}
		r, view, p, a = callActivateReady(t, e, runID)
	}
	if a.Kind == "choice" && a.InvocationID == bodyID {
		decision := e.evaluateChoiceFor(r, p, bodyID, a.StageID)
		if _, err := e.commitChoice(context.Background(), r, view, p, a, newID("command"), decision); err != nil {
			t.Fatal(err)
		}
		r, view, p, a = callActivateReady(t, e, runID)
	}
	if a.Kind != "finish" || a.InvocationID != bodyID {
		t.Fatal("fixture did not reach the current body's finish")
	}
	if err := e.finish(context.Background(), r, view, p, a); err != nil {
		t.Fatal(err)
	}
	r, view, p, a = callActivateReady(t, e, runID)
	if a.Kind != "repeat" || a.Status != "waiting" || r.currentBodyForRepeat(a.ID).ID != bodyID || r.Invocations[bodyID].Status != "completed" {
		t.Fatal("completed body did not return to its own repeat decision")
	}
	return r, view, p, a
}

func repeatContinueInputs(t *testing.T, e *Engine, r Run, p *flow.Plan, a *Activation, commandID string) (RepeatDecision, map[string]ArtifactRef) {
	t.Helper()
	d, err := e.evaluateRepeat(r, p, a)
	if err != nil || d.Route != "continue" || d.Observed != (Observation{}) {
		t.Fatal("fixture did not produce an uncommitted continue decision", err)
	}
	inputs, err := e.prepareBodyInputs(r, a.InvocationID, d.BodyInvocationID, p.Repeats[a.StageID], p.Workflow.Definition.Stages[a.StageID].NextBindings, commandID, "")
	if err != nil {
		t.Fatal(err)
	}
	return d, inputs
}

func repeatSameProjection(t *testing.T, before, after local.ReadView) {
	t.Helper()
	// Attempted commands may add request samples or rejection receipts, but
	// they cannot change accepted state or its event sequence.
	if after.Snapshot.Version != before.Snapshot.Version || after.Snapshot.EventSeq != before.Snapshot.EventSeq || !bytes.Equal(after.Snapshot.Data, before.Snapshot.Data) {
		t.Fatal("refused or duplicate operation changed the accepted projection")
	}
}

func TestRepeatControlPauseAtBodyBoundary(t *testing.T) {
	for _, boundary := range []string{"body_finished", "continued"} {
		t.Run(boundary, func(t *testing.T) {
			e, workflow, _, options := repeatFixture(t, "commit-pass", "succeeded", 2)
			runID := choiceStart(t, e, workflow, options)
			repeatEnter(t, e, runID)
			r, view, p, a := repeatFinishBody(t, e, runID)
			commandID := newID("command")
			d, inputs := repeatContinueInputs(t, e, r, p, a, commandID)
			if boundary == "continued" {
				if _, err := e.commitRepeat(context.Background(), r, view, p, a, commandID, d, inputs); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := e.Restrict(context.Background(), RestrictCommand{SchemaVersion: "1", CommandID: newID("command"), Scope: "run", ScopeID: runID, Kind: "pause", Reason: "Pause at a repeat boundary"}); err != nil {
				t.Fatal(err)
			}
			before, decisions := repeatHistory(t, e, runID)
			if boundary == "body_finished" {
				if _, err := e.commitRepeat(context.Background(), r, view, p, a, commandID, d, inputs); err == nil {
					t.Fatal("stale continue crossed a stop")
				}
				current, currentView, err := e.load(context.Background(), runID)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := e.commitRepeat(context.Background(), current, currentView, p, current.Activations[a.ID], newID("command"), d, inputs); err == nil {
					t.Fatal("fresh CAS version bypassed the effective pause")
				}
			}
			if err := e.Drive(context.Background(), runID); err != nil {
				t.Fatal(err)
			}
			after, afterDecisions := repeatHistory(t, e, runID)
			repeatSameProjection(t, before, after)
			r = driverRun(t, e, runID)
			wantBodies := int64(1)
			if boundary == "continued" {
				wantBodies = 2
			}
			if len(afterDecisions) != len(decisions) || r.Activations[a.ID].Repeat.IterationCount != wantBodies || len(r.Invocations) != int(wantBodies)+1 || len(r.Attempts) != 1 || len(r.Steps) != 1 || driverObservedStarts(t, e) != 1 {
				t.Fatal("paused repeat admitted a decision, body or worker")
			}
			stop := r.Stops[0]
			if _, err := e.Release(context.Background(), ReleaseRequest{CommandID: newID("command"), RunID: runID, ExpectedControlEpoch: r.ControlEpoch, Stops: []StopGeneration{{ID: stop.ID, Generation: stop.Generation}}, Reason: "Release the exact pause"}); err != nil {
				t.Fatal(err)
			}
			next, err := e.Next(context.Background(), runID)
			if err != nil || next.Action != "resume_required" {
				t.Fatal("release implicitly resumed the loop", err)
			}
			if _, err := e.Resume(context.Background(), runID, newID("command"), "Resume the repeat", next.RunVersion); err != nil {
				t.Fatal(err)
			}
			if err := e.Drive(context.Background(), runID); err != nil {
				t.Fatal(err)
			}
			r = driverRun(t, e, runID)
			if r.Status != "completed" || len(r.Invocations) != 3 || len(r.Attempts) != 2 || r.ControlTransitions != 8 || driverObservedStarts(t, e) != 2 {
				t.Fatal("resume duplicated a body, worker or control charge")
			}
		})
	}
}

func TestRepeatControlCancelledCurrentChildStaysBlocked(t *testing.T) {
	e, workflow, _, options := repeatFixture(t, "commit-pass", "succeeded", 2)
	workflow["allowed_outcomes"] = []string{"succeeded", "rejected"}
	choiceStages(workflow)["work"].(map[string]any)["on_error"] = "rejected"
	choiceStages(workflow)["rejected"] = choiceFinish("rejected")
	runID := choiceStart(t, e, workflow, options)
	r := repeatEnter(t, e, runID)
	a := activationFor(&r, "work")
	bodyID := r.currentBodyForRepeat(a.ID).ID
	if _, err := e.Restrict(context.Background(), RestrictCommand{SchemaVersion: "1", CommandID: newID("command"), Scope: "invocation", ScopeID: bodyID, Kind: "cancel", Reason: "Cancel only the current iteration"}); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	r = driverRun(t, e, runID)
	before, decisions := repeatHistory(t, e, runID)
	if r.Status != "waiting" || r.Outcome != nil || r.CancelRequested || r.Invocations[r.RootInvocationID].CancelRequested || r.Invocations[bodyID].Status != "cancelled" || r.Activations[a.ID].Repeat.IterationCount != 1 || len(r.Invocations) != 2 || len(r.Attempts) != 0 || len(decisions) != 0 {
		t.Fatal("cancelled body became an outcome, error handler or next iteration")
	}
	next, err := e.Next(context.Background(), runID)
	if err != nil || next.Action != "blocked_child" || next.InvocationID != bodyID {
		t.Fatal("cancelled current body was not exposed as blocked_child", err)
	}
	stop := r.Stops[0]
	if _, err := e.Release(context.Background(), ReleaseRequest{CommandID: newID("command"), RunID: runID, ExpectedControlEpoch: r.ControlEpoch, Stops: []StopGeneration{{ID: stop.ID, Generation: stop.Generation}}, Reason: "Cancellation is not reversible"}); err == nil {
		t.Fatal("cancelled iteration was released for retry")
	}
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	after, _ := repeatHistory(t, e, runID)
	repeatSameProjection(t, before, after)
	if _, err := e.Restrict(context.Background(), RestrictCommand{SchemaVersion: "1", CommandID: newID("command"), Scope: "run", ScopeID: runID, Kind: "cancel", Reason: "Close the blocked Run"}); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	r = driverRun(t, e, runID)
	a = r.Activations[a.ID]
	if r.Status != "cancelled" || a.Status != "cancelled" || a.Settled == nil || a.Repeat.IterationCount != 1 || a.Repeat.LastDecision != nil || len(r.Attempts) != 0 || driverObservedStarts(t, e) != 0 {
		t.Fatal("Run cancellation left open repeat control or invented execution")
	}
	if timingFind(t, Timing(r, e.clock.now(), false).Root, a.ID).Metrics["elapsed"].IsOpen {
		t.Fatal("cancelled repeat kept accumulating elapsed time")
	}
}

func TestRepeatControlCommitReceiptAndCAS(t *testing.T) {
	e, workflow, _, options := repeatFixture(t, "", "succeeded", 2)
	runID := choiceStart(t, e, workflow, options)
	repeatEnter(t, e, runID)
	r, view, p, a := repeatFinishBody(t, e, runID)
	commandID := newID("command")
	d, inputs := repeatContinueInputs(t, e, r, p, a, commandID)
	first, err := e.commitRepeat(context.Background(), r, view, p, a, commandID, d, inputs)
	if err != nil || first.Receipt.Rejection != nil {
		t.Fatal("initial continue commit", err)
	}
	before, decisions := repeatHistory(t, e, runID)
	if len(decisions) != 1 || decisions[0].NextBodyInvocationID != d.NextBodyInvocationID {
		t.Fatal("continue did not create one durable decision and exact next body")
	}
	duplicate, err := e.commitRepeat(context.Background(), r, view, p, a, commandID, d, inputs)
	if err != nil || !duplicate.Duplicate || !reflect.DeepEqual(first.Receipt, duplicate.Receipt) {
		t.Fatal("exact old-read retry changed its original receipt/cut", err)
	}
	stale, err := e.commitRepeat(context.Background(), r, view, p, a, newID("command"), d, inputs)
	if err == nil || stale.Receipt.Rejection == nil || stale.Receipt.Rejection.Code != "version_conflict" {
		t.Fatal("stale writer created another iteration", err)
	}
	after, decisions := repeatHistory(t, e, runID)
	repeatSameProjection(t, before, after)
	current := driverRun(t, e, runID)
	if len(decisions) != 1 || len(current.Invocations) != 3 || current.ControlTransitions != 3 || current.Activations[a.ID].Repeat.IterationCount != 2 || len(current.Steps) != 0 || len(current.Attempts) != 0 {
		t.Fatal("duplicate continue charged budget or fabricated work")
	}
}

func TestRepeatControlConcurrentWritersCreateOneBody(t *testing.T) {
	e, workflow, _, options := repeatFixture(t, "", "succeeded", 2)
	runID := choiceStart(t, e, workflow, options)
	repeatEnter(t, e, runID)
	r, view, p, a := repeatFinishBody(t, e, runID)
	d, inputs := repeatContinueInputs(t, e, r, p, a, newID("command"))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	type result struct {
		change local.ApplyResult
		err    error
	}
	start, done := make(chan struct{}), make(chan result, 2)
	for i := 0; i < 2; i++ {
		commandID := newID("command")
		go func() {
			<-start
			change, err := e.commitRepeat(ctx, r, view, p, a, commandID, d, inputs)
			done <- result{change, err}
		}()
	}
	close(start)
	accepted, rejected := 0, 0
	for i := 0; i < 2; i++ {
		got := <-done
		if got.err == nil && got.change.Receipt.Rejection == nil {
			accepted++
		} else if got.change.Receipt.Rejection != nil && got.change.Receipt.Rejection.Code == "version_conflict" {
			rejected++
		} else {
			t.Fatalf("unexpected concurrent repeat result: %+v %v", got.change, got.err)
		}
	}
	current := driverRun(t, e, runID)
	_, decisions := repeatHistory(t, e, runID)
	if accepted != 1 || rejected != 1 || len(decisions) != 1 || len(current.Invocations) != 3 || current.Activations[a.ID].Repeat.IterationCount != 2 || current.ControlTransitions != 3 {
		t.Fatal("concurrent decision writers created or charged more than one body")
	}
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	current = driverRun(t, e, runID)
	_, decisions = repeatHistory(t, e, runID)
	if current.Status != "completed" || len(decisions) != 2 || current.ControlTransitions != 6 || len(current.Attempts) != 0 || len(current.Steps) != 0 {
		t.Fatal("continuation reset the accounting of existing control-only bodies")
	}
}

func TestRepeatControlStorageBudgetGuardsEveryBodyCreation(t *testing.T) {
	for _, boundary := range []string{"entry", "continue"} {
		t.Run(boundary, func(t *testing.T) {
			e, workflow, _, options := repeatFixture(t, "commit-pass", "succeeded", 2)
			runID := choiceStart(t, e, workflow, options)
			if boundary == "continue" {
				repeatEnter(t, e, runID)
				repeatFinishBody(t, e, runID)
			}
			usage, err := e.Store.StorageUsage(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if err := e.Store.Close(); err != nil {
				t.Fatal(err)
			}
			// Exercise the real logical quota without filling the disk or
			// injecting a synthetic body, accepted result or storage failure.
			e.Store, err = local.OpenStore(filepath.Join(e.Root, e.Config.Configuration.StateRoot), local.StoreOptions{EventTypes: EventTypes, SoftLimitBytes: max(64<<10, usage.AllocatedBytes)})
			if err != nil {
				t.Fatal(err)
			}
			r, before, p, a := callActivateReady(t, e, runID)
			if boundary == "entry" {
				err = e.enterRepeat(context.Background(), r, before, p, a)
			} else {
				commandID := newID("command")
				d, inputs := repeatContinueInputs(t, e, r, p, a, commandID)
				_, err = e.commitRepeat(context.Background(), r, before, p, a, commandID, d, inputs)
			}
			rejectionCode(t, err, "storage_budget_exhausted")
			after, decisions := repeatHistory(t, e, runID)
			repeatSameProjection(t, before, after)
			current := driverRun(t, e, runID)
			if len(decisions) != 0 || current.Activations[a.ID].Repeat.IterationCount != a.Repeat.IterationCount || len(current.Invocations) != len(r.Invocations) || len(current.Attempts) != len(r.Attempts) || len(current.Active) != 0 {
				t.Fatal("quota refusal left an iteration or pending attempt behind")
			}
			if slot, owner, err := e.Store.Slot(context.Background()); err != nil || slot != "" || owner != "" {
				t.Fatal("quota-refused body acquired execution capacity", err)
			}
			if _, err := e.Restrict(context.Background(), RestrictCommand{SchemaVersion: "1", CommandID: newID("command"), Scope: "run", ScopeID: runID, Kind: "cancel", Reason: "Control must remain possible above the admission quota"}); err != nil {
				t.Fatal(err)
			}
			if err := e.Drive(context.Background(), runID); err != nil {
				t.Fatal(err)
			}
			current = driverRun(t, e, runID)
			if current.Status != "cancelled" || len(current.Attempts) != len(r.Attempts) || driverObservedStarts(t, e) != len(r.Attempts) {
				t.Fatal("mandatory cancellation was blocked by the work quota")
			}
			if err := e.Store.Verify(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRepeatControlCrashAtDurableBoundaries(t *testing.T) {
	for _, boundary := range []string{"entered", "body_finished", "continued"} {
		t.Run(boundary, func(t *testing.T) {
			e, workflow, _, options := repeatFixture(t, "commit-pass", "succeeded", 2)
			runID := choiceStart(t, e, workflow, options)
			if err := os.WriteFile(filepath.Join(e.Root, "repeat-boundary-kind"), []byte(boundary), 0600); err != nil {
				t.Fatal(err)
			}
			_, crash := choiceCrashDriver(t, "TestRepeatControlBoundaryHelper", "REPEAT_BOUNDARY_HELPER", e.Root, runID)
			driverWait(t, e, runID, func(r Run) bool {
				_, err := os.Stat(filepath.Join(e.Root, "repeat-boundary-ready"))
				return err == nil
			})
			// The helper can commit between driverWait's read and the marker check.
			before := driverRun(t, e, runID)
			a := activationFor(&before, "work")
			bodies, err := before.repeatBodies(a.ID)
			if err != nil || len(bodies) == 0 {
				t.Fatal("helper did not reach a durable body boundary", err)
			}
			firstID := bodies[0].ID
			wantBodies, wantAttempts := 1, 1
			if boundary == "entered" {
				wantAttempts = 0
			} else if boundary == "continued" {
				wantBodies = 2
			}
			if len(bodies) != wantBodies || len(before.Attempts) != wantAttempts || len(before.Active) != 0 || boundary != "entered" && (bodies[0].Status != "completed" || bodies[0].Settled == nil) || (a.Repeat.LastDecision != nil) != (boundary == "continued") {
				t.Fatal("helper crossed the requested boundary or faked settlement")
			}
			for _, attempt := range before.Attempts {
				if attempt.Accepted == nil || attempt.ProcessOutcome == nil || !attempt.ProcessOutcome.WaitReturned || !attempt.ProcessOutcome.GroupEmpty || attempt.Settled == nil {
					t.Fatal("boundary has no actual accepted and settled owned worker")
				}
			}
			old, oldDecisions := repeatHistory(t, e, runID)
			crash()
			if err := e.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := Open(e.Root, false)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = reopened.Close() })
			if err := reopened.Drive(context.Background(), runID); err != nil {
				t.Fatal(err)
			}
			after := driverRun(t, reopened, runID)
			read, decisions := repeatHistory(t, reopened, runID)
			if after.Status != "completed" || len(after.Invocations) != 3 || len(after.Attempts) != 2 || after.ControlTransitions != 8 || driverObservedStarts(t, reopened) != 2 || len(decisions) != 2 || decisions[0].BodyInvocationID != firstID {
				t.Fatal("recovery duplicated a body, decision, worker or budget charge")
			}
			if len(read.Events) < len(old.Events) || !reflect.DeepEqual(old.Events, read.Events[:len(old.Events)]) || !reflect.DeepEqual(oldDecisions, decisions[:len(oldDecisions)]) {
				t.Fatal("recovery rewrote the old event or decision prefix")
			}
			if err := reopened.Drive(context.Background(), runID); err != nil {
				t.Fatal(err)
			}
			again, _ := repeatHistory(t, reopened, runID)
			repeatSameProjection(t, read, again)
			if driverObservedStarts(t, reopened) != 2 {
				t.Fatal("terminal reopen dispatched an iteration again")
			}
		})
	}
}

func TestRepeatControlBoundaryHelper(t *testing.T) {
	if os.Getenv("REPEAT_BOUNDARY_HELPER") != "1" {
		return
	}
	root, runID := os.Args[len(os.Args)-2], os.Args[len(os.Args)-1]
	e, err := Open(root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	boundary, err := os.ReadFile(filepath.Join(root, "repeat-boundary-kind"))
	if err != nil {
		t.Fatal(err)
	}
	repeatEnter(t, e, runID)
	if string(boundary) != "entered" {
		r, view, p, a := repeatFinishBody(t, e, runID)
		if string(boundary) == "continued" {
			commandID := newID("command")
			d, inputs := repeatContinueInputs(t, e, r, p, a, commandID)
			if _, err := e.commitRepeat(context.Background(), r, view, p, a, commandID, d, inputs); err != nil {
				t.Fatal(err)
			}
		}
	}
	lock, err := e.driverLock(runID)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := os.WriteFile(filepath.Join(root, "repeat-boundary-ready"), boundary, 0600); err != nil {
		t.Fatal(err)
	}
	// Parent signals only this Cmd.Start-owned helper. No production pause
	// hook or alternate worker path is installed, and orphan lifetime is bound.
	time.Sleep(10 * time.Second)
	t.Fatal("parent did not stop the owned helper at the repeat boundary")
}

func TestRepeatControlUnknownWorkerBlocksNextIteration(t *testing.T) {
	e, workflow, _, options := repeatFixture(t, "crash-short", "succeeded", 2)
	workflow["allowed_outcomes"] = []string{"succeeded", "rejected"}
	choiceStages(workflow)["work"].(map[string]any)["on_error"] = "rejected"
	choiceStages(workflow)["rejected"] = choiceFinish("rejected")
	runID := choiceStart(t, e, workflow, options)
	_, crash := choiceCrashDriver(t, "TestDriverCrashHelper", "DRIVER_CRASH_HELPER", e.Root, runID)
	before := driverWait(t, e, runID, func(r Run) bool {
		for _, a := range r.Attempts {
			if a.Started != nil {
				if _, err := os.Stat(filepath.Join(a.Workspace, "worker-ready")); err == nil {
					return true
				}
			}
		}
		return false
	})
	crash()
	// The bounded worker exits itself. Process absence cannot establish a
	// successful settlement after the authority loses ownership of that child.
	deadline := time.Now().Add(3 * time.Second)
	for local.ProbeProcess(*before.Attempts[before.Active[0]].Process).State == "present" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if err := e.Drive(context.Background(), runID); err == nil || !strings.Contains(err.Error(), "recovery_required") {
		t.Fatal("uncertain iteration was treated as failed or retried", err)
	}
	r := driverRun(t, e, runID)
	a := activationFor(&r, "work")
	_, decisions := repeatHistory(t, e, runID)
	if r.Status != "uncertain" || !r.HasUnresolvedEffects || r.Outcome != nil || len(r.Active) != 1 || len(r.Attempts) != 1 || len(r.Invocations) != 2 || a.Repeat.IterationCount != 1 || a.Repeat.LastDecision != nil || len(decisions) != 0 || driverObservedStarts(t, e) != 1 {
		t.Fatal("unknown child allowed a decision, error handler or later iteration")
	}
	next, err := e.Next(context.Background(), runID)
	if err != nil || next.Action != "uncertain" {
		t.Fatal("global unknown barrier disappeared from Next", err)
	}
}
