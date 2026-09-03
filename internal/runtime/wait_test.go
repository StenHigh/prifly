package runtime

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stenhigh/prifly/internal/flow"
)

// waitRuntimeFixture builds a workflow whose entry stage waits for one event.
// The source is a pinned definition of the project, so what may resolve the
// wait is named before anything runs.
// waitBeforeFixture puts a choice ahead of the wait, so the wait exists in the
// definition but has not been reached. That is the only situation a reservation
// is for: a promise about a wait the graph has not arrived at yet.
func waitBeforeFixture(t *testing.T, reach bool) (*Engine, map[string]any, StartOptions) {
	t.Helper()
	e, workflow, options := waitRuntimeFixture(t, int64(3600))
	controlRef := registerRuntimeSchema(t, e, "wait-control", []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"}`))
	workflow["inputs"].(map[string]any)["control"] = map[string]any{"format": "json", "schema_ref": controlRef, "required": true}
	stages := choiceStages(workflow)
	stages["pick"] = choiceStage("exclusive",
		choiceBranch("wait", choiceFieldEqual("/reach", true), "hold"),
		choiceBranch("skip", choiceFieldEqual("/reach", false), "accepted"))
	workflow["definition"].(map[string]any)["entry"] = "pick"
	writeRuntimeJSON(t, e.Root+"/control.json", map[string]any{"reach": reach})
	options.Inputs["control"] = "control.json"
	return e, workflow, options
}

func waitRuntimeFixture(t *testing.T, timeout any) (*Engine, map[string]any, StartOptions) {
	t.Helper()
	e, workflow, options := choiceFixture(t, `{"flag":true}`, "")
	eventRef := registerRuntimeSchema(t, e, "wait-event", []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"decision":{"type":"string"}},"required":["decision"]}`))
	keyRef := registerRuntimeSchema(t, e, "wait-key", []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"}`))
	sourceRef := registerRuntimeSchema(t, e, "wait-source", []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"}`))

	defs, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	workflow["id"], workflow["title"] = "test:workflow/waiting", "Waiting fixture"
	workflow["policy_ref"] = builtinVersionRef(defs, "core:policy/local", "2.0.0")
	workflow["allowed_outcomes"] = []string{"succeeded", "rejected"}
	workflow["inputs"] = map[string]any{"ticket": map[string]any{"format": "json", "schema_ref": keyRef, "required": true}}
	workflow["outputs"] = map[string]any{}
	workflow["limits"] = map[string]any{"max_step_instances": 2, "max_control_transitions": 32, "max_parallelism": 1, "max_child_depth": 0}
	stage := map[string]any{
		"kind": "wait", "source_ref": sourceRef, "event_type": "approval.granted", "event_schema_ref": eventRef,
		"correlation_input": map[string]any{"from": "workflow_input", "port": "ticket"},
		"timeout_seconds":   timeout, "on_event": "accepted",
	}
	stages := map[string]any{
		"hold":     stage,
		"accepted": choiceFinish("succeeded"),
	}
	if timeout != nil {
		stage["on_timeout"] = "expired"
		stages["expired"] = choiceFinish("rejected")
	} else {
		workflow["allowed_outcomes"] = []string{"succeeded"}
	}
	workflow["definition"] = map[string]any{"entry": "hold", "stages": stages}
	writeRuntimeJSON(t, e.Root+"/ticket.json", map[string]any{"id": "T-1"})
	options.Inputs = map[string]string{"ticket": "ticket.json"}
	options.WorkflowFile = "workflows/waiting.json"
	return e, workflow, options
}

func waitActivation(t *testing.T, r Run) *Activation {
	t.Helper()
	for _, a := range r.Activations {
		if a.Kind == "wait" {
			return a
		}
	}
	t.Fatal("the run has no wait activation")
	return nil
}

// A wait holds no worker and no attempt. Its whole point is that nothing
// executes while it waits, so the run stops with the registration in hand.
func TestWaitHoldsNoWorkerWhileItWaits(t *testing.T) {
	e, workflow, options := waitRuntimeFixture(t, int64(3600))
	runID := choiceStart(t, e, workflow, options)
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	if r.SchemaVersion != CoreWaitStateVersion {
		t.Fatalf("the closure did not select the registration state: %s", r.SchemaVersion)
	}
	if r.Status == "completed" {
		t.Fatal("the run finished without waiting for anything")
	}
	if len(r.Attempts) != 0 || len(r.Active) != 0 {
		t.Fatalf("waiting admitted work: %d attempts, %d active", len(r.Attempts), len(r.Active))
	}
	a := waitActivation(t, r)
	registration := r.Waits[a.Wait.RegistrationID]
	if a.Status != "waiting" || registration == nil || registration.Status != "active" {
		t.Fatalf("the wait is not holding an active registration: %s %+v", a.Status, registration)
	}
	if registration.ExpiresAt == "" {
		t.Fatal("a finite wait pinned no deadline")
	}
}

// An event delivered for the entered wait resolves it exactly once, and its
// payload is exported as an artifact of the declared schema.
func TestWaitResolvesOnceOnItsEvent(t *testing.T) {
	e, workflow, options := waitRuntimeFixture(t, int64(3600))
	runID := choiceStart(t, e, workflow, options)
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	registration := waitActivation(t, driverRun(t, e, runID)).Wait.RegistrationID
	held := driverRun(t, e, runID).Waits[registration]

	delivered, err := e.DeliverEvent(context.Background(), DeliverEventRequest{RunID: runID, RegistrationID: registration,
		EventID: "event:one", EventType: "approval.granted", Nonce: held.Nonce, Generation: held.Generation,
		Payload: []byte(`{"decision":"granted"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if delivered.Disposition != "held" {
		t.Fatalf("a correct event was refused: %+v", delivered)
	}
	r := driverRun(t, e, runID)
	a := waitActivation(t, r)
	if a.Wait.Resolution != "event" || a.Wait.EventRef == nil {
		t.Fatalf("the wait did not resolve on its event: %+v", a.Wait)
	}
	if r.Waits[registration].Status != "consumed" {
		t.Fatalf("the registration was not consumed: %s", r.Waits[registration].Status)
	}
	_, payload, err := e.Artifact(*a.Wait.EventRef)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if json.Unmarshal(payload, &value) != nil || value["decision"] != "granted" {
		t.Fatalf("the exported event is not what was delivered: %s", payload)
	}
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	if done := driverRun(t, e, runID); done.Status != "completed" || done.Outcome == nil || *done.Outcome != "succeeded" {
		t.Fatalf("the accepted route did not finish: %s %+v", done.Status, done.Outcome)
	}

	// A second delivery of the same event, and a fresh one for a wait that is
	// already resolved, change nothing and are kept with their reasons.
	if _, err := e.DeliverEvent(context.Background(), DeliverEventRequest{RunID: runID, RegistrationID: registration,
		EventID: "event:one", EventType: "approval.granted", Nonce: held.Nonce, Generation: held.Generation,
		Payload: []byte(`{"decision":"granted"}`)}); err == nil {
		t.Fatal("a duplicate event was accepted a second time")
	}
	late, err := e.DeliverEvent(context.Background(), DeliverEventRequest{RunID: runID, RegistrationID: registration,
		EventID: "event:two", EventType: "approval.granted", Nonce: held.Nonce, Generation: held.Generation,
		Payload: []byte(`{"decision":"granted"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if late.Disposition != "refused" || late.Reason != "wait_consumed" {
		t.Fatalf("a late event was not kept with its reason: %+v", late)
	}
}

// A delivery that does not match the registration opens nothing. Each refusal
// keeps its own reason, because "wrong signal" and "no signal" differ.
func TestWaitRefusesSignalsThatAreNotItsOwn(t *testing.T) {
	for _, c := range []struct{ name, eventType, nonce, reason string }{
		{"an event of another type", "approval.denied", "", "event_type_mismatch"},
		{"a correlation it never issued", "approval.granted", "not-the-nonce", "correlation_mismatch"},
	} {
		t.Run(c.name, func(t *testing.T) {
			e, workflow, options := waitRuntimeFixture(t, int64(3600))
			runID := choiceStart(t, e, workflow, options)
			if err := e.Drive(context.Background(), runID); err != nil {
				t.Fatal(err)
			}
			a := waitActivation(t, driverRun(t, e, runID))
			held := driverRun(t, e, runID).Waits[a.Wait.RegistrationID]
			nonce := held.Nonce
			if c.nonce != "" {
				nonce = c.nonce
			}
			stored, err := e.DeliverEvent(context.Background(), DeliverEventRequest{RunID: runID, RegistrationID: held.ID,
				EventID: "event:wrong", EventType: c.eventType, Nonce: nonce, Generation: held.Generation,
				Payload: []byte(`{"decision":"granted"}`)})
			if err != nil {
				t.Fatal(err)
			}
			if stored.Disposition != "refused" || stored.Reason != c.reason {
				t.Fatalf("expected %s, got %+v", c.reason, stored)
			}
			after := driverRun(t, e, runID)
			if waitActivation(t, after).Wait.Resolution != "" {
				t.Fatal("a refused event resolved the wait")
			}
			if after.Waits[held.ID].Status != "active" {
				t.Fatalf("a refused event moved the registration: %s", after.Waits[held.ID].Status)
			}
		})
	}
}

// A payload that does not satisfy the declared event schema is not an event.
func TestWaitRefusesAPayloadItCannotRead(t *testing.T) {
	e, workflow, options := waitRuntimeFixture(t, int64(3600))
	runID := choiceStart(t, e, workflow, options)
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	held := driverRun(t, e, runID).Waits[waitActivation(t, driverRun(t, e, runID)).Wait.RegistrationID]
	if _, err := e.DeliverEvent(context.Background(), DeliverEventRequest{RunID: runID, RegistrationID: held.ID,
		EventID: "event:bad", EventType: "approval.granted", Nonce: held.Nonce, Generation: held.Generation,
		Payload: []byte(`{"verdict":"granted"}`)}); err == nil {
		t.Fatal("a payload that fails its own schema was accepted")
	}
}

// An indefinite wait declares that it will not expire, and it does not: no
// deadline is pinned and nothing in the driver moves it on.
func TestIndefiniteWaitPinsNoDeadline(t *testing.T) {
	e, workflow, options := waitRuntimeFixture(t, nil)
	runID := choiceStart(t, e, workflow, options)
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	a := waitActivation(t, r)
	registration := r.Waits[a.Wait.RegistrationID]
	if registration.ExpiresAt != "" {
		t.Fatalf("an indefinite wait pinned a deadline: %s", registration.ExpiresAt)
	}
	if waitDue(registration, "2999-01-01T00:00:00Z") {
		t.Fatal("an indefinite wait came due")
	}
	// Driving again changes nothing: there is nothing to observe.
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	if again := driverRun(t, e, runID); waitActivation(t, again).Wait.Resolution != "" {
		t.Fatal("an indefinite wait resolved itself")
	}
}

// A deadline that has passed takes the expiry route and creates no event.
// Nothing fires here on its own: this build owns no timer, so the deadline is
// observed the next time the authority looks, which is what "waiting" means.
func TestWaitExpiresWithoutInventingAnEvent(t *testing.T) {
	e, workflow, options := waitRuntimeFixture(t, int64(1))
	runID := choiceStart(t, e, workflow, options)
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	a := waitActivation(t, driverRun(t, e, runID))
	if a.Wait.Resolution != "" {
		t.Fatal("the wait expired before its deadline")
	}
	// The deadline is a wall-clock fact, so the test waits for it rather than
	// pretending time passed.
	time.Sleep(1100 * time.Millisecond)
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	a = waitActivation(t, r)
	if a.Wait.Resolution != "timeout" || a.Wait.EventRef != nil {
		t.Fatalf("expiry did not take its own route: %+v", a.Wait)
	}
	if r.Waits[a.Wait.RegistrationID].Status != "expired" {
		t.Fatalf("the registration was not expired: %s", r.Waits[a.Wait.RegistrationID].Status)
	}
	if r.Status != "completed" || r.Outcome == nil || *r.Outcome != "rejected" {
		t.Fatalf("the expiry route did not finish: %s %+v", r.Status, r.Outcome)
	}
	// An event arriving after expiry is kept with its reason and opens nothing.
	held := r.Waits[a.Wait.RegistrationID]
	late, err := e.DeliverEvent(context.Background(), DeliverEventRequest{RunID: runID, RegistrationID: held.ID,
		EventID: "event:late", EventType: "approval.granted", Nonce: held.Nonce, Generation: held.Generation,
		Payload: []byte(`{"decision":"granted"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if late.Disposition != "refused" || late.Reason != "wait_expired" {
		t.Fatalf("an event after expiry was not kept with its reason: %+v", late)
	}
	if again := driverRun(t, e, runID); waitActivation(t, again).Wait.Resolution != "timeout" {
		t.Fatal("a late event changed a settled resolution")
	}
}

var _ = flow.CoreProfile

// A callback can beat the graph to its own wait. The promise is made before the
// external job is started, the answer lands in the inbox, and it is applied
// exactly once at the moment the wait is actually entered - not before.
func TestReservedWaitHoldsAnAnswerThatArrivesFirst(t *testing.T) {
	e, workflow, options := waitBeforeFixture(t, true)
	runID := choiceStart(t, e, workflow, options)
	root := driverRun(t, e, runID).RootInvocationID

	reserved, err := e.ReserveWait(context.Background(), ReserveWaitRequest{RunID: runID, InvocationID: root,
		TargetStageID: "hold", RequestedExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)})
	if err != nil {
		t.Fatal(err)
	}
	if reserved.Status != "reserved" {
		t.Fatalf("a promise was made as something other than a reservation: %+v", reserved)
	}
	// Nothing about the route has changed: reserving is not activating.
	before := driverRun(t, e, runID)
	for _, a := range before.Activations {
		if a.Kind == "wait" {
			t.Fatalf("reserving activated the wait: %+v", a)
		}
	}

	delivered, err := e.DeliverEvent(context.Background(), DeliverEventRequest{RunID: runID, RegistrationID: reserved.ID,
		EventID: "event:fast", EventType: "approval.granted", Nonce: reserved.Nonce, Generation: reserved.Generation,
		Payload: []byte(`{"decision":"granted"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if delivered.Disposition != "held" {
		t.Fatalf("an early answer was not held: %+v", delivered)
	}
	for _, a := range driverRun(t, e, runID).Activations {
		if a.Kind == "wait" {
			t.Fatal("an early answer created the wait before the graph reached it")
		}
	}

	// Now the graph reaches the wait. The held answer applies exactly once.
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	a := waitActivation(t, r)
	if a.Wait.RegistrationID != reserved.ID {
		t.Fatalf("the wait made a second promise instead of keeping the first: %s", a.Wait.RegistrationID)
	}
	if a.Wait.Resolution != "event" || a.Wait.EventRef == nil {
		t.Fatalf("the held answer was not applied at entry: %+v", a.Wait)
	}
	consumed := 0
	for _, event := range r.Inbox {
		if event.Disposition == "consumed" {
			consumed++
		}
	}
	if consumed != 1 {
		t.Fatalf("the held answer was applied %d times", consumed)
	}
	if r.Status != "completed" || r.Outcome == nil || *r.Outcome != "succeeded" {
		t.Fatalf("the run did not finish on its held answer: %s %+v", r.Status, r.Outcome)
	}
}

// A promise the graph never reaches is retired with the scope that made it, and
// anything held for it is refused. A branch nobody took owes its senders an
// answer too: their events must not wait on a route that will never be taken.
func TestUnreachedReservationIsRetiredWithItsScope(t *testing.T) {
	// The choice will route past the wait, so the promise is never kept.
	e, workflow, options := waitBeforeFixture(t, false)
	runID := choiceStart(t, e, workflow, options)
	root := driverRun(t, e, runID).RootInvocationID
	reserved, err := e.ReserveWait(context.Background(), ReserveWaitRequest{RunID: runID, InvocationID: root,
		TargetStageID: "hold", RequestedExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.DeliverEvent(context.Background(), DeliverEventRequest{RunID: runID, RegistrationID: reserved.ID,
		EventID: "event:orphan", EventType: "approval.granted", Nonce: reserved.Nonce, Generation: reserved.Generation,
		Payload: []byte(`{"decision":"granted"}`)}); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	if r.Waits[reserved.ID].Status != "cancelled" {
		t.Fatalf("an unreached promise was left open: %s", r.Waits[reserved.ID].Status)
	}
	for _, event := range r.Inbox {
		if event.Disposition == "consumed" {
			t.Fatalf("an event for an unreached wait started work: %+v", event)
		}
		if event.Envelope.EventID == "event:orphan" && event.Reason != "wait_cancelled" {
			t.Fatalf("the orphaned event was not kept with its reason: %+v", event)
		}
	}
}

// A reservation cannot outlive what this build is prepared to hold, and one
// that has already lapsed promises nothing.
func TestReservationDeadlineIsTheAuthoritysToSet(t *testing.T) {
	e, workflow, options := waitBeforeFixture(t, true)
	runID := choiceStart(t, e, workflow, options)
	root := driverRun(t, e, runID).RootInvocationID

	far := time.Now().Add(20 * 365 * 24 * time.Hour).UTC().Format(time.RFC3339Nano)
	reserved, err := e.ReserveWait(context.Background(), ReserveWaitRequest{RunID: runID, InvocationID: root,
		TargetStageID: "hold", RequestedExpiresAt: far})
	if err != nil {
		t.Fatal(err)
	}
	if reserved.ExpiresAt >= far {
		t.Fatalf("the caller's deadline was taken as given: %s", reserved.ExpiresAt)
	}

	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)
	if _, err := e.ReserveWait(context.Background(), ReserveWaitRequest{RunID: runID, InvocationID: root,
		TargetStageID: "accepted", RequestedExpiresAt: past}); err == nil {
		t.Fatal("a finish stage was reserved as a wait")
	}
}
