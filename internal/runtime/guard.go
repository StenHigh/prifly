package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// GuardDeclaration is one live guard as a caller declares it when the Run is
// created. Guards are declared there rather than installed later for the reason
// REA-004 gives: a registration has to exist before the first admission it
// protects, and one installed afterwards would have already let something
// through. The declared scope is this Run's root invocation.
type GuardDeclaration struct {
	// Kind is start or stop.
	Kind string
	// TargetStageID narrows a start guard to one stage of the root workflow.
	// Empty gates the invocation itself, which is the first activation in it.
	TargetStageID string
	Predicate     flow.Predicate
	// Action and OnUnknown are required of a stop guard and forbidden of a
	// start guard. Both are restrictions; there is no permissive default.
	Action    string
	OnUnknown string
}

// installGuards turns validated declarations into durable registrations. It
// runs inside the transaction that creates the Run, so a guard is never absent
// for a moment during which something could have been admitted under it.
func installGuards(r *Run, declarations []GuardDeclaration, actor string, obs Observation) error {
	if len(declarations) == 0 {
		return nil
	}
	if !isGuardState(r.SchemaVersion) {
		return local.ErrIntegrity
	}
	if len(declarations) > MaxGuards {
		return local.Reject("guard_limit", fmt.Sprintf("a Run may declare at most %d guards", MaxGuards))
	}
	r.Guards = make(map[string]*GuardRegistration, len(declarations))
	for i, d := range declarations {
		id := guardID(r.ID, d.Kind, d.TargetStageID, i)
		if r.Guards[id] != nil {
			return local.Reject("guard_conflict", "two guards of the same kind and target occupy one identity")
		}
		r.Guards[id] = &GuardRegistration{SchemaVersion: GuardRegistrationVersion, ID: id, RunID: r.ID,
			Kind: d.Kind, InvocationID: r.RootInvocationID, TargetStageID: d.TargetStageID,
			Predicate: d.Predicate, Action: d.Action, OnUnknown: d.OnUnknown, Actor: actor,
			Status: "observing", Observations: []GuardObservation{}, Created: obs}
	}
	return r.guardInvariant()
}

// evaluateGuard reads the facts this Run already holds and records what the
// predicate makes of them. It reads nothing outside the Run: a guard in this
// build sees workflow inputs and completed stage outputs, exactly the fact
// space a choice sees, and no live source at all.
func (e *Engine) evaluateGuard(r Run, p *flow.Plan, g *GuardRegistration, cut int64, obs Observation) GuardObservation {
	observation := GuardObservation{Sequence: int64(len(g.Observations)), Facts: []ChoiceInput{}, Cut: cut, Observed: obs}
	sources := map[ArtifactRef]choiceSourceValue{}
	seen := map[flow.FieldRef]bool{}
	var readBytes int64
	resolve := func(ref flow.FieldRef) (any, bool, error) {
		input, value, present, err := ChoiceInput{FieldRef: ref}, any(nil), false, error(nil)
		if r.guardFactSettled(g.InvocationID, ref) {
			input, value, present, err = e.conditionSource(r, p, g.InvocationID, "", ref, &readBytes, sources)
		}
		if err == nil && present {
			value, present = flow.JSONPointer(value, ref.Pointer)
		}
		if !seen[ref] {
			switch {
			case err != nil:
				input.Availability = "unavailable"
			case present:
				input.Availability = "present"
			default:
				input.Availability = "absent"
			}
			observation.Facts = append(observation.Facts, input)
			seen[ref] = true
		}
		return value, present, err
	}
	truth, err := flow.EvaluateGuardPredicate(g.Predicate, resolve)
	if err != nil {
		// A fact that cannot be read, and a comparison between values of
		// different types, are errors about the declaration or the store. They
		// are recorded as errors and never folded into unknown: unknown has a
		// declared reaction, and lending it to a fault would let a broken guard
		// take whichever of the two actions happened to be the permissive one.
		observation.Truth, observation.Reason = "error", "guard_evaluation_failed"
		var problem *flow.Problem
		if errors.As(err, &problem) {
			observation.Reason = problem.Code
		}
		return observation
	}
	observation.Truth = string(truth)
	switch truth {
	case flow.TruthTrue:
		observation.Reason = "facts_true"
	case flow.TruthFalse:
		observation.Reason = "facts_false"
	default:
		// An unknown here means a fact this Run does not hold yet. A fact it
		// could not read at all is an error above, so "missing" and
		// "unreadable" never arrive at the same word.
		observation.Reason = "facts_absent"
	}
	return observation
}

// sameGuardAnswer reports whether a fresh evaluation says exactly what the last
// recorded one already said. Only a changed answer is written down: recording
// an identical one on every pass would grow the ledger with the passage of time
// rather than with anything that happened, and would never terminate.
func sameGuardAnswer(previous *GuardObservation, current GuardObservation) bool {
	if previous == nil || previous.Truth != current.Truth || previous.Reason != current.Reason || len(previous.Facts) != len(current.Facts) {
		return false
	}
	for i, fact := range previous.Facts {
		other := current.Facts[i]
		if fact.FieldRef != other.FieldRef || fact.Availability != other.Availability || fact.ProducerActivationID != other.ProducerActivationID {
			return false
		}
		if (fact.SourceRef == nil) != (other.SourceRef == nil) || fact.SourceRef != nil && *fact.SourceRef != *other.SourceRef {
			return false
		}
	}
	return true
}

// advanceGuards runs one guard cycle and reports whether it changed anything.
// Processing comes before observing: an observation nobody has acted on is what
// forbids new admissions, so clearing it is the more urgent of the two.
func (e *Engine) advanceGuards(ctx context.Context, loaded Run, view local.ReadView) (bool, error) {
	if !isGuardState(loaded.SchemaVersion) || len(loaded.Guards) == 0 || loaded.terminal() {
		return false, nil
	}
	for _, id := range loaded.guardIDs() {
		if loaded.Guards[id].pending() {
			return true, e.processGuards(ctx, loaded, view)
		}
	}
	return e.observeGuards(ctx, loaded, view)
}

// observeGuards records a new observation for every guard whose answer has
// changed, in one transaction and at one cut. Several facts changed by one
// transaction are therefore read together: a multi-fact predicate is never a
// mixture of one fact from an old cut and another from a newer one.
func (e *Engine) observeGuards(ctx context.Context, loaded Run, view local.ReadView) (bool, error) {
	fresh := map[string]GuardObservation{}
	obs := e.clock.now()
	for _, id := range loaded.guardIDs() {
		g := loaded.Guards[id]
		if g.Status == "satisfied" || g.Status == "failed" {
			continue
		}
		if g.Kind == "start" && loaded.guardTargetOpen(g) {
			// The gated activation exists, so this guard has done its work. It
			// is not consulted again: a start guard that went on answering
			// would be a cancellation mechanism nobody declared.
			fresh[id] = GuardObservation{}
			continue
		}
		p, err := loaded.planFor(g.InvocationID)
		if err != nil {
			return false, err
		}
		observation := e.evaluateGuard(loaded, p, g, view.Snapshot.Version, obs)
		if !sameGuardAnswer(g.settled(), observation) || g.pending() {
			fresh[id] = observation
		}
	}
	if len(fresh) == 0 {
		return false, nil
	}
	_, err := e.apply(ctx, e.owner, newID("command"), loaded.ID, "guard.observed", map[string]any{"guard_ids": loaded.guardIDs(), "cut": view.Snapshot.Version}, &view.Snapshot.Version, local.CommandCAS, func(r *Run, s local.Snapshot, obs Observation) (local.Change, error) {
		if !isGuardState(r.SchemaVersion) || r.terminal() {
			return local.Change{}, local.Reject("guard_conflict", "this Run no longer holds live guards")
		}
		events, moved := []local.EventInput{}, false
		for _, id := range r.guardIDs() {
			observation, changed := fresh[id]
			g := r.Guards[id]
			if !changed || g == nil {
				continue
			}
			if observation.Truth == "" {
				// The gated activation exists, so this guard is retired. That
				// is a change without an observation: there is no new answer to
				// record, only a rule that has finished asking the question.
				g.Status, moved = "satisfied", true
				continue
			}
			if len(g.Observations) >= MaxGuardObservations {
				// The ledger is full, so this answer cannot be written down.
				// Dropping the oldest entry to make room would be able to drop
				// an observed true, which is what the latch exists to prevent,
				// so the guard fails and keeps refusing instead.
				g.Status, moved = "failed", true
				continue
			}
			observation.Observed, observation.Sequence = obs, int64(len(g.Observations))
			g.Observations = append(g.Observations, observation)
			data, err := canonical(map[string]any{"run_id": r.ID, "guard_id": g.ID, "guard_kind": g.Kind,
				"workflow_invocation_id": g.InvocationID, "target_stage_id": g.TargetStageID,
				"sequence": observation.Sequence, "truth": observation.Truth, "reason": observation.Reason,
				"facts": observation.Facts, "cut": observation.Cut, "observation": obs})
			if err != nil {
				return local.Change{}, err
			}
			events, moved = append(events, local.EventInput{Type: "guard.observed", Version: 1, Data: data}), true
		}
		if err := r.guardInvariant(); err != nil {
			return local.Change{}, err
		}
		if !moved {
			return local.Change{}, local.Reject("guard_conflict", "the guards changed under this observation")
		}
		return local.Change{RequireStorageBudget: len(events) != 0, Events: events}, nil
	})
	return err == nil, err
}

// processGuards acts on every observation the cursor has not passed and then
// advances it. Latching and firing happen in the same transaction as the cursor
// move: a latch that moved without its stop, or a cursor that moved without its
// latch, would each lose exactly the true the latch exists to keep.
func (e *Engine) processGuards(ctx context.Context, loaded Run, view local.ReadView) error {
	_, err := e.apply(ctx, e.owner, newID("command"), loaded.ID, "guard.processed", map[string]any{"guard_ids": loaded.guardIDs()}, &view.Snapshot.Version, local.CommandCAS, func(r *Run, s local.Snapshot, obs Observation) (local.Change, error) {
		if !isGuardState(r.SchemaVersion) || r.terminal() {
			return local.Change{}, local.Reject("guard_conflict", "this Run no longer holds live guards")
		}
		events := []local.EventInput{}
		for _, id := range r.guardIDs() {
			g := r.Guards[id]
			if !g.pending() {
				continue
			}
			event, err := processGuard(r, g, obs)
			if err != nil {
				return local.Change{}, err
			}
			data, err := canonical(event)
			if err != nil {
				return local.Change{}, err
			}
			events = append(events, local.EventInput{Type: "guard.processed", Version: 1, Data: data})
		}
		if err := r.guardInvariant(); err != nil {
			return local.Change{}, err
		}
		if len(events) == 0 {
			return local.Change{}, local.Reject("guard_conflict", "another process already processed these observations")
		}
		return local.Change{RequireStorageBudget: true, Events: events}, nil
	})
	return err
}

// processGuard consumes one guard's unprocessed observations. Every one of them
// is read, not just the last: a false arriving after a true must not be able to
// hide the true, so a true anywhere in the window latches.
func processGuard(r *Run, g *GuardRegistration, obs Observation) (map[string]any, error) {
	action, failed, deciding := "", false, int64(-1)
	for i := g.Cursor; i < int64(len(g.Observations)); i++ {
		observation := &g.Observations[i]
		observation.Processed = true
		if observation.Truth == "error" {
			failed = true
			continue
		}
		if g.Kind != "stop" {
			continue
		}
		declared := ""
		switch observation.Truth {
		case string(flow.TruthTrue):
			declared = g.Action
		case string(flow.TruthUnknown):
			declared = g.OnUnknown
		}
		if declared != "" {
			g.Latched = true
			// Two causes in one window are not averaged. Cancelling is the
			// stronger request, so it is the one that survives: choosing the
			// weaker would quietly drop an accepted instruction to cancel. The
			// recorded cause moves with the action, so the stop always names
			// the observation whose reaction it is actually taking.
			if action != "cancel_scope" && (action == "" || declared == "cancel_scope") {
				action, deciding = declared, i
			}
		}
	}
	cursor := g.Cursor
	g.Cursor = int64(len(g.Observations))
	event := map[string]any{"run_id": r.ID, "guard_id": g.ID, "guard_kind": g.Kind,
		"workflow_invocation_id": g.InvocationID, "cursor": g.Cursor, "latched": g.Latched,
		"stop_id": "", "action": "", "observation": obs}
	// A cause is needed to fire, not merely a latch: a guard that latched while
	// its scope was already finished has nothing left to restrict, and firing
	// on a window that carried no cause would name an observation nobody made.
	if g.Latched && g.StopID == "" && deciding >= 0 && !invocationTerminal(guardScopeStatus(*r, g)) {
		stop, err := fireGuardStop(r, g, action, cursor, deciding, obs)
		if err != nil {
			return nil, err
		}
		event["stop_id"], event["action"] = stop, action
	}
	if failed {
		g.Status = "failed"
	} else if g.StopID != "" {
		g.Status = "fired"
	}
	return event, nil
}

func guardScopeStatus(r Run, g *GuardRegistration) string {
	if inv := r.Invocations[g.InvocationID]; inv != nil {
		return inv.Status
	}
	return "failed"
}

// fireGuardStop creates the ordinary durable stop. It is the same record a
// person's restrict command creates, carried by the same control epoch and
// released only by the same explicit release: an automatic restriction that
// could be lifted by a second automatic decision would not be a stop at all.
func fireGuardStop(r *Run, g *GuardRegistration, action string, cursor, deciding int64, obs Observation) (string, error) {
	inv := r.Invocations[g.InvocationID]
	if inv == nil {
		return "", local.ErrIntegrity
	}
	r.ControlEpoch++
	kind := guardStopKind(action)
	cause := g.Observations[deciding]
	stop := Stop{ID: guardStopID(g.ID, cursor), Generation: 1, Epoch: r.ControlEpoch, Kind: kind,
		Reason: fmt.Sprintf("stop_when guard %s observed %s at cut %d; observation %d records the exact facts it read",
			g.ID, cause.Truth, cause.Cut, cause.Sequence),
		Actor: g.Actor, Status: "active", Created: obs, Scope: "invocation", ScopeID: g.InvocationID}
	r.Stops = append(r.Stops, stop)
	g.StopID = stop.ID
	if kind == "cancel" {
		inv.CancelRequested, inv.Status = true, "stopping"
		return stop.ID, nil
	}
	inv.ResumeRequired = true
	if len(r.Active) == 0 {
		inv.Status = "waiting"
	}
	return stop.ID, nil
}
