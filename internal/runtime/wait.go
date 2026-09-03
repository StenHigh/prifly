package runtime

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// waitNonce is derived, not random, so that replaying the journal reproduces
// the same registration. That is safe precisely because a nonce is not a
// credential: it correlates a signal to one exact wait and says nothing about
// who is speaking. Deciding whether a sender may speak at all stays with the
// source adapter, where it can actually be answered.
// waitActivationID is derived rather than minted, and only for a wait. A
// promise that one exact signal may resolve one exact wait has to name that
// wait before it exists, which is impossible with a random identity. Within one
// invocation a stage activates at most once - the graph is acyclic - so run,
// invocation and stage already name it uniquely.
func waitActivationID(runID, invocationID, stageID string) string {
	return derivedID("activation", runID, invocationID, stageID)
}

func waitNonce(runID, activationID string, generation int64) string {
	return derivedID("nonce", runID, activationID, strconv.FormatInt(generation, 10))
}

// checkWaitActivation guards both halves of a wait's life, which differ in one
// way. Entering is ordinary scheduled work and must own the ready scope.
// Resolving is not: an entered wait deliberately holds no frontier, because a
// scope that is ready with nothing to do would be handed to the driver forever.
// So a resolution asks a different question - is this still the wait its
// invocation is stopped on - rather than pretending the frontier is there.
func checkWaitActivation(r Run, p *flow.Plan, a *Activation, status string) error {
	if a == nil || !isWaitState(r.SchemaVersion) || r.Profile != flow.CoreProfile || p.Profile != r.Profile ||
		a.ID == "" || a.Kind != "wait" || a.Status != status || a.Settled != nil || a.StepID != "" {
		return local.Reject("wait_blocked", "the wait is not in the state this decision assumes")
	}
	parent, stage := r.Invocations[a.InvocationID], p.Workflow.Definition.Stages[a.StageID]
	if parent == nil || parent.WorkflowRef != planRef(p) || stage.Kind != "wait" {
		return local.Reject("stage_conflict", "wait activation or pinned definition differs")
	}
	if status == "ready" {
		kind, stageID := nextKind(r)
		parentID, _ := r.readyScope()
		if kind != "stage" || a.InvocationID != parentID || a.StageID != stageID {
			return local.Reject("wait_blocked", "the wait no longer owns its ready scope")
		}
	} else if parent.Status != "waiting" || len(r.readyFor(a.InvocationID)) != 0 {
		return local.Reject("wait_blocked", "the invocation is no longer stopped on this wait")
	}
	if invocationTerminal(parent.Status) || r.terminal() || r.restrictedFor(a.InvocationID) {
		return local.Reject("wait_blocked", "the scope holding this wait cannot advance")
	}
	return r.waitProgressInvariant(a)
}

// dueWait finds one activation whose finite deadline has passed by the
// authority's clock. There is no timer owner in this build, so a deadline is
// observed when the authority next looks rather than fired at the second: the
// waiting status says so, and nothing here pretends otherwise.
func (r Run) dueWait(now string) *Activation {
	ids := make([]string, 0, len(r.Activations))
	for id := range r.Activations {
		ids = append(ids, id)
	}
	// A stable order so that two due deadlines are always taken in the same
	// sequence, whatever the map iteration happens to produce.
	sort.Strings(ids)
	for _, id := range ids {
		a := r.Activations[id]
		if a == nil || a.Kind != "wait" || a.Status != "waiting" || a.Wait == nil || a.Wait.Resolution != "" {
			continue
		}
		if waitDue(r.Waits[a.Wait.RegistrationID], now) {
			return a
		}
	}
	return nil
}

// enterWait turns the reservation this activation was created with into an
// active one and pins its deadline. The deadline is the authority's: a sender
// does not get to say when its own signal stops being welcome.
func (e *Engine) enterWait(ctx context.Context, loaded Run, view local.ReadView, p *flow.Plan, activation *Activation) error {
	if err := checkWaitActivation(loaded, p, activation, "ready"); err != nil {
		return err
	}
	stage := p.Workflow.Definition.Stages[activation.StageID]
	if source, publication := p.PublicationSource(stage.SourceRef); publication && source.Mode == "each_publication" {
		return e.enterPublicationStreamWait(ctx, loaded, view, p, activation)
	}
	var rootPlan *flow.Plan
	if _, publication := p.PublicationSource(stage.SourceRef); publication {
		var err error
		rootPlan, err = loaded.plan()
		if err != nil {
			return err
		}
	}
	commandID := newID("command")
	// A held event that arrived before the wait was entered is applied here,
	// exactly once, and only now: an early callback must not have routed
	// anything while the graph was still somewhere else.
	held := loaded.heldEventFor(activation.Wait.RegistrationID)
	payload := map[string]any{"stage_activation_id": activation.ID, "registration_id": activation.Wait.RegistrationID, "held_event_id": ""}
	if held != nil {
		payload["held_event_id"] = held.Envelope.EventID
	}
	_, err := e.apply(ctx, e.owner, commandID, loaded.ID, "stage.wait_entered", payload, &view.Snapshot.Version, local.CommandCAS, func(r *Run, s local.Snapshot, obs Observation) (local.Change, error) {
		a := r.Activations[activation.ID]
		if err := checkWaitActivation(*r, p, a, "ready"); err != nil {
			return local.Change{}, err
		}
		registration := r.Waits[a.Wait.RegistrationID]
		if registration.Status != "reserved" {
			return local.Change{}, local.Reject("stage_conflict", "wait already left its reservation")
		}
		if err := r.chargeInvocation(a.InvocationID, 1, 0); err != nil {
			return local.Change{}, err
		}
		registration.Status = "active"
		// An indefinite wait is recorded as having no deadline at all rather
		// than a very distant one: "we will wait" and "we will wait until
		// Tuesday" are different promises and must read differently.
		if stage.TimeoutSeconds != nil {
			deadline, err := time.Parse(time.RFC3339Nano, obs.UTC)
			if err != nil {
				return local.Change{}, err
			}
			registration.ExpiresAt = deadline.Add(time.Duration(*stage.TimeoutSeconds) * time.Second).UTC().Format(time.RFC3339Nano)
		}
		if err := r.setReadyFor(a.InvocationID, []string{}); err != nil {
			return local.Change{}, err
		}
		if err := r.setInvocationStatus(a.InvocationID, "waiting", nil); err != nil {
			return local.Change{}, err
		}
		a.Status = "waiting"
		data, err := canonical(map[string]any{"run_id": r.ID, "stage_activation_id": a.ID, "stage_id": a.StageID, "workflow_invocation_id": a.InvocationID,
			"wait_registration_id": registration.ID, "wait_generation": registration.Generation, "expires_at": registration.ExpiresAt,
			"event_type": stage.EventType, "source_ref": stage.SourceRef, "observation": obs})
		if err != nil {
			return local.Change{}, err
		}
		events := []local.EventInput{{Type: "stage.wait_entered", Version: 1, Data: data}}
		if rootPlan != nil {
			retained, _, err := assignRetainedPublication(r, rootPlan, registration, obs)
			if err != nil {
				return local.Change{}, err
			}
			events = append(events, retained...)
		}
		return local.Change{RequireStorageBudget: true, Events: events}, nil
	})
	if err != nil || held == nil {
		return err
	}
	// The wait is active and an event was already in hand. Applying it is a
	// separate decision with its own record, so the journal shows both.
	current, next, err := e.load(ctx, loaded.ID)
	if err != nil {
		return err
	}
	currentActivation := current.Activations[activation.ID]
	if currentActivation == nil {
		return local.ErrIntegrity
	}
	if currentActivation.Status != "waiting" {
		return nil // A managed retained publication resolved it at entry.
	}
	return e.resolveWaitWithEvent(ctx, current, next, p, currentActivation, *held)
}

// heldEventFor finds the one early event matched to this reservation. An event
// is applied once: a consumed one is never returned again.
func (r Run) heldEventFor(registrationID string) *InboxEvent {
	for i := range r.Inbox {
		if r.Inbox[i].RegistrationID == registrationID && r.Inbox[i].Disposition == "held" {
			return &r.Inbox[i]
		}
	}
	return nil
}

// waitDue reports whether a finite deadline has passed by the authority's own
// clock. An indefinite wait is never due, which is what makes it indefinite.
func waitDue(registration *WaitRegistration, now string) bool {
	if registration == nil || registration.Status != "active" || registration.ExpiresAt == "" {
		return false
	}
	deadline, err := time.Parse(time.RFC3339Nano, registration.ExpiresAt)
	if err != nil {
		return false
	}
	observed, err := time.Parse(time.RFC3339Nano, now)
	return err == nil && !observed.Before(deadline)
}

// resolveWaitWithTimeout takes the expiry route. It creates no event: expiry is
// the absence of a signal, and manufacturing one would be an answer nobody gave.
func (e *Engine) resolveWaitWithTimeout(ctx context.Context, loaded Run, view local.ReadView, p *flow.Plan, activation *Activation) error {
	if err := checkWaitActivation(loaded, p, activation, "waiting"); err != nil {
		return err
	}
	stage := p.Workflow.Definition.Stages[activation.StageID]
	if source, publication := p.PublicationSource(stage.SourceRef); publication && source.Mode == "each_publication" {
		return e.resolvePublicationStreamTimeout(ctx, loaded, view, p, activation)
	}
	if stage.OnTimeout == "" {
		return local.Reject("stage_conflict", "an indefinite wait has no expiry route")
	}
	commandID := newID("command")
	payload := map[string]any{"stage_activation_id": activation.ID, "registration_id": activation.Wait.RegistrationID, "next_stage_id": stage.OnTimeout}
	_, err := e.apply(ctx, e.owner, commandID, loaded.ID, "stage.wait_resolved", payload, &view.Snapshot.Version, local.CommandCAS, func(r *Run, s local.Snapshot, obs Observation) (local.Change, error) {
		a := r.Activations[activation.ID]
		if err := checkWaitActivation(*r, p, a, "waiting"); err != nil {
			return local.Change{}, err
		}
		registration := r.Waits[a.Wait.RegistrationID]
		if a.Wait.Resolution != "" || registration.Status != "active" {
			return local.Change{}, local.Reject("wait_resolved", "this wait already has its one resolution")
		}
		if !waitDue(registration, obs.UTC) {
			return local.Change{}, local.Reject("wait_not_due", "the deadline has not passed by the authority's clock")
		}
		if err := r.chargeInvocation(a.InvocationID, 1, 0); err != nil {
			return local.Change{}, err
		}
		registration.Status = "expired"
		a.Wait.Resolution, a.Wait.ResolvedAt = "timeout", obs.UTC
		a.Status, a.Settled = "completed", &obs
		if err := r.advanceInvocation(a.InvocationID, stage.OnTimeout); err != nil {
			return local.Change{}, err
		}
		data, err := canonical(map[string]any{"run_id": r.ID, "stage_activation_id": a.ID, "stage_id": a.StageID, "workflow_invocation_id": a.InvocationID,
			"wait_registration_id": registration.ID, "wait_generation": registration.Generation, "resolution": "timeout",
			"expires_at": registration.ExpiresAt, "next_stage_id": stage.OnTimeout, "observation": obs})
		if err != nil {
			return local.Change{}, err
		}
		return local.Change{RequireStorageBudget: true, Events: []local.EventInput{{Type: "stage.wait_resolved", Version: 1, Data: data}}}, nil
	})
	return err
}

// resolveWaitWithEvent takes the accepted route and exports the payload the
// sender supplied. The envelope was already checked against this registration;
// what is decided here is that this Run applies it exactly once.
func (e *Engine) resolveWaitWithEvent(ctx context.Context, loaded Run, view local.ReadView, p *flow.Plan, activation *Activation, held InboxEvent) error {
	if err := checkWaitActivation(loaded, p, activation, "waiting"); err != nil {
		return err
	}
	stage := p.Workflow.Definition.Stages[activation.StageID]
	commandID := newID("command")
	payload := map[string]any{"stage_activation_id": activation.ID, "event_id": held.Envelope.EventID, "next_stage_id": stage.OnEvent}
	_, err := e.apply(ctx, e.owner, commandID, loaded.ID, "stage.wait_resolved", payload, &view.Snapshot.Version, local.CommandCAS, func(r *Run, s local.Snapshot, obs Observation) (local.Change, error) {
		a := r.Activations[activation.ID]
		if err := checkWaitActivation(*r, p, a, "waiting"); err != nil {
			return local.Change{}, err
		}
		registration := r.Waits[a.Wait.RegistrationID]
		if a.Wait.Resolution != "" || registration.Status != "active" {
			return local.Change{}, local.Reject("wait_resolved", "this wait already has its one resolution")
		}
		index := -1
		for i := range r.Inbox {
			if r.Inbox[i].Envelope.EventID == held.Envelope.EventID && r.Inbox[i].Disposition == "held" {
				index = i
			}
		}
		if index < 0 {
			return local.Change{}, local.Reject("wait_event_absent", "the event this resolution names is not held for it")
		}
		event := r.Inbox[index]
		if event.RegistrationID != registration.ID || event.Envelope.Generation != registration.Generation || event.Envelope.Nonce != registration.Nonce {
			return local.Change{}, local.Reject("stage_conflict", "the held event belongs to another wait")
		}
		if err := r.chargeInvocation(a.InvocationID, 1, 0); err != nil {
			return local.Change{}, err
		}
		r.Inbox[index].Disposition = "consumed"
		registration.Status = "consumed"
		ref := event.Envelope.PayloadRef
		a.Wait.Resolution, a.Wait.ResolvedAt, a.Wait.EventRef = "event", obs.UTC, &ref
		a.Status, a.Settled = "completed", &obs
		if err := r.advanceInvocation(a.InvocationID, stage.OnEvent); err != nil {
			return local.Change{}, err
		}
		data, err := canonical(map[string]any{"run_id": r.ID, "stage_activation_id": a.ID, "stage_id": a.StageID, "workflow_invocation_id": a.InvocationID,
			"wait_registration_id": registration.ID, "wait_generation": registration.Generation, "resolution": "event",
			"event_id": event.Envelope.EventID, "payload_ref": ref, "received_at": event.Envelope.ReceivedAt,
			"next_stage_id": stage.OnEvent, "observation": obs})
		if err != nil {
			return local.Change{}, err
		}
		return local.Change{RequireStorageBudget: true, Events: []local.EventInput{{Type: "stage.wait_resolved", Version: 1, Data: data}}}, nil
	})
	return err
}

// DeliverEventRequest is one signal offered to a Run. The sender supplies the
// payload and the correlation it was given; everything else about the delivery
// is the authority's to decide, including when it was received.
type DeliverEventRequest struct {
	RunID          string
	CommandID      string
	RegistrationID string
	EventID        string
	EventType      string
	Nonce          string
	Generation     int64
	Payload        []byte
}

// DeliverEvent accepts a signal for one exact registration. It never routes
// anything by itself: an event for an entered wait resolves it, an event that
// arrives first is held for the moment that wait is entered, and a delivery
// that matches nothing is recorded with its reason. "We never got it" and "we
// got it and would not use it" must not look the same afterwards.
func (e *Engine) DeliverEvent(ctx context.Context, request DeliverEventRequest) (InboxEvent, error) {
	loaded, view, err := e.load(ctx, request.RunID)
	if err != nil {
		return InboxEvent{}, err
	}
	if !isWaitState(loaded.SchemaVersion) {
		return InboxEvent{}, local.Reject("incompatible_run", "this run declares no waits")
	}
	registration := loaded.Waits[request.RegistrationID]
	if registration == nil {
		return InboxEvent{}, local.Reject("wait_registration_absent", "no registration of that identity exists in this run")
	}
	commandID := request.CommandID
	if commandID == "" {
		commandID = newID("command")
	}
	p, err := loaded.planFor(registration.InvocationID)
	if err != nil {
		return InboxEvent{}, err
	}
	stage := p.Workflow.Definition.Stages[registration.TargetStageID]
	if _, publication := p.PublicationSource(registration.SourceRef); publication {
		return InboxEvent{}, local.Reject("managed_source", "artifact publication sources are resolved only by authority-accepted publications")
	}
	// The payload is sealed as an artifact of the declared schema before any
	// decision is taken about it. An event that cannot be read is not an event.
	artifact, err := e.putArtifact(request.Payload, "json", &registration.EventSchemaRef,
		derivedID("artifact", commandID, registration.ID, request.EventID),
		// An event is brought in from outside by a named principal through a
		// named source. That is what an import is, so it is recorded as one
		// rather than as something this authority produced by itself.
		map[string]any{"kind": "import", "import_id": derivedID("import", commandID, request.EventID), "source_ref": registration.SourceRef, "principal_id": e.owner},
		nil, p.Registry)
	if err != nil {
		return InboxEvent{}, err
	}
	var stored InboxEvent
	_, err = e.apply(ctx, e.owner, commandID, loaded.ID, "wait.event_received", map[string]any{
		"wait_registration_id": request.RegistrationID, "event_id": request.EventID, "event_type": request.EventType,
		"wait_generation": request.Generation, "payload_ref": artifact.Ref(),
	}, &view.Snapshot.Version, local.CommandCAS, func(r *Run, s local.Snapshot, obs Observation) (local.Change, error) {
		held := r.Waits[request.RegistrationID]
		if held == nil {
			return local.Change{}, local.Reject("wait_registration_absent", "the registration disappeared under this delivery")
		}
		if len(r.Inbox) >= MaxInboxEvents {
			// The cap is announced rather than enforced by silence: a sender
			// that is being dropped has to be able to find that out.
			return local.Change{}, local.Reject("inbox_full", fmt.Sprintf("this run's inbox holds its limit of %d events", MaxInboxEvents))
		}
		for _, existing := range r.Inbox {
			if existing.Envelope.EventID == request.EventID {
				return local.Change{}, local.Reject("duplicate_event", "an event of that identity was already delivered")
			}
		}
		envelope := EventEnvelope{SchemaVersion: EventEnvelopeVersion, SourceRef: held.SourceRef, EventID: request.EventID,
			EventType: request.EventType, PayloadSchemaRef: held.EventSchemaRef, PayloadRef: artifact.Ref(),
			RunID: r.ID, ActivationID: held.ActivationID, Generation: request.Generation, Nonce: request.Nonce,
			// Received is the authority's timestamp. When the sender says it
			// acted is the sender's claim and is not what deadlines are judged by.
			ReceivedAt: obs.UTC}
		event := InboxEvent{Envelope: envelope, RegistrationID: held.ID, Disposition: "held", Observed: obs}
		switch {
		case request.EventType != stage.EventType:
			event.Disposition, event.Reason, event.RegistrationID = "refused", "event_type_mismatch", ""
		case request.Generation != held.Generation:
			// An old generation is a late signal for a wait that has moved on.
			// It is kept as evidence and opens nothing.
			event.Disposition, event.Reason, event.RegistrationID = "refused", "stale_generation", ""
		case request.Nonce != held.Nonce:
			event.Disposition, event.Reason, event.RegistrationID = "refused", "correlation_mismatch", ""
		case held.Status == "consumed" || held.Status == "expired" || held.Status == "cancelled":
			event.Disposition, event.Reason, event.RegistrationID = "refused", "wait_"+held.Status, ""
		}
		r.Inbox = append(r.Inbox, event)
		stored = event
		data, err := canonical(map[string]any{"run_id": r.ID, "wait_registration_id": request.RegistrationID,
			"event_id": request.EventID, "disposition": event.Disposition, "reason": event.Reason,
			"envelope": envelope, "observation": obs})
		if err != nil {
			return local.Change{}, err
		}
		return local.Change{RequireStorageBudget: true, Events: []local.EventInput{{Type: "wait.event_received", Version: 1, Data: data}}}, nil
	})
	if err != nil || stored.Disposition != "held" {
		return stored, err
	}
	// A held event whose wait is already entered is applied now. One that
	// arrived early stays held until that wait is actually entered.
	current, next, err := e.load(ctx, request.RunID)
	if err != nil {
		return stored, err
	}
	activation := current.Activations[registration.ActivationID]
	if activation == nil || activation.Status != "waiting" {
		return stored, nil
	}
	plan, err := current.planFor(registration.InvocationID)
	if err != nil {
		return stored, err
	}
	return stored, e.resolveWaitWithEvent(ctx, current, next, plan, activation, stored)
}

// ReserveWaitRequest asks for a durable promise about a wait the graph has not
// reached yet. It exists for one situation: a step is about to start something
// outside that may answer immediately, and the answer must have somewhere to
// land even if it beats the graph to the wait.
type ReserveWaitRequest struct {
	RunID     string
	CommandID string
	// InvocationID and TargetStageID name the exact wait being promised. The
	// source and the event schema are not parameters: they are taken from that
	// stage's pinned definition, so a reservation cannot widen what the
	// definition said it would accept.
	InvocationID  string
	TargetStageID string
	// RequestedExpiresAt is what the caller would like. The authority clamps
	// it: a promise this build cannot hold is not made longer by asking.
	RequestedExpiresAt string
}

// ReserveWait creates a reserved registration for a wait that has not been
// entered. Reserving is not activating: the promise exists, an early event has
// somewhere to land, and nothing about the route changes until the graph
// actually reaches that wait.
func (e *Engine) ReserveWait(ctx context.Context, request ReserveWaitRequest) (WaitRegistration, error) {
	loaded, view, err := e.load(ctx, request.RunID)
	if err != nil {
		return WaitRegistration{}, err
	}
	if !isWaitState(loaded.SchemaVersion) {
		return WaitRegistration{}, local.Reject("incompatible_run", "this run declares no waits")
	}
	p, err := loaded.planFor(request.InvocationID)
	if err != nil {
		return WaitRegistration{}, err
	}
	stage, declared := p.Workflow.Definition.Stages[request.TargetStageID]
	if !declared || stage.Kind != "wait" {
		return WaitRegistration{}, local.Reject("invalid_target", "the reserved target is not a wait stage of this invocation")
	}
	requested, err := time.Parse(time.RFC3339Nano, request.RequestedExpiresAt)
	if err != nil {
		return WaitRegistration{}, local.Reject("invalid_deadline", "a reservation states when it stops being valid")
	}
	commandID := request.CommandID
	if commandID == "" {
		commandID = newID("command")
	}
	const firstGeneration = 1
	activationID := waitActivationID(loaded.ID, request.InvocationID, request.TargetStageID)
	var reserved WaitRegistration
	_, err = e.apply(ctx, e.owner, commandID, loaded.ID, "wait.reserved", map[string]any{
		"workflow_invocation_id": request.InvocationID, "target_stage_id": request.TargetStageID,
		"stage_activation_id": activationID, "requested_expires_at": request.RequestedExpiresAt,
	}, &view.Snapshot.Version, local.CommandCAS, func(r *Run, s local.Snapshot, obs Observation) (local.Change, error) {
		invocation := r.Invocations[request.InvocationID]
		if invocation == nil || invocationTerminal(invocation.Status) || r.terminal() {
			return local.Change{}, local.Reject("invalid_target", "the scope that would hold this wait is not live")
		}
		if r.Activations[activationID] != nil {
			return local.Change{}, local.Reject("wait_already_entered", "that wait already exists, so it cannot be promised in advance")
		}
		if len(r.Waits) >= MaxWaitRegistrations {
			return local.Change{}, local.Reject("wait_registrations_exhausted", "this run already holds the most registrations it may")
		}
		id := waitRegistrationID(r.ID, activationID, firstGeneration)
		if r.Waits[id] != nil {
			return local.Change{}, local.Reject("wait_already_reserved", "that wait is already promised")
		}
		now, err := time.Parse(time.RFC3339Nano, obs.UTC)
		if err != nil {
			return local.Change{}, err
		}
		// The ceiling is the authority's, not the caller's. A reservation that
		// outlived what this build is prepared to hold would be a promise made
		// by nobody.
		ceiling := now.Add(time.Duration(flow.MaxWaitSeconds) * time.Second)
		expires := requested
		if expires.After(ceiling) {
			expires = ceiling
		}
		if !expires.After(now) {
			return local.Change{}, local.Reject("invalid_deadline", "a reservation that has already lapsed promises nothing")
		}
		if err := r.chargeInvocation(request.InvocationID, 1, 0); err != nil {
			return local.Change{}, err
		}
		registration := &WaitRegistration{SchemaVersion: WaitRegistrationVersion, ID: id, RunID: r.ID,
			InvocationID: request.InvocationID, TargetStageID: request.TargetStageID, ActivationID: activationID,
			SourceRef: stage.SourceRef, EventSchemaRef: stage.EventSchemaRef, Generation: firstGeneration,
			Nonce: waitNonce(r.ID, activationID, firstGeneration), Status: "reserved",
			ExpiresAt: expires.UTC().Format(time.RFC3339Nano)}
		if r.Waits == nil {
			r.Waits = map[string]*WaitRegistration{}
		}
		r.Waits[id] = registration
		reserved = *registration
		data, err := canonical(map[string]any{"run_id": r.ID, "wait_registration_id": id, "workflow_invocation_id": request.InvocationID,
			"target_stage_id": request.TargetStageID, "stage_activation_id": activationID, "wait_generation": firstGeneration,
			"requested_expires_at": request.RequestedExpiresAt, "expires_at": registration.ExpiresAt, "observation": obs})
		if err != nil {
			return local.Change{}, err
		}
		return local.Change{RequireStorageBudget: true, Events: []local.EventInput{{Type: "wait.reserved", Version: 1, Data: data}}}, nil
	})
	return reserved, err
}

// cancelUnreachedReservations retires the promises a finished scope will never
// keep. A branch that was not taken owes its senders an answer too: their
// events must not sit waiting for a wait that will never be entered.
func (r *Run) cancelUnreachedReservations(invocationID string) {
	for _, registration := range r.Waits {
		if registration.InvocationID != invocationID || registration.Status != "reserved" {
			continue
		}
		if r.Activations[registration.ActivationID] != nil {
			continue
		}
		registration.Status = "cancelled"
		for i := range r.Inbox {
			if r.Inbox[i].RegistrationID == registration.ID && r.Inbox[i].Disposition == "held" {
				r.Inbox[i].Disposition, r.Inbox[i].Reason, r.Inbox[i].RegistrationID = "refused", "wait_cancelled", ""
			}
		}
	}
}
