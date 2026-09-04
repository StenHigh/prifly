package runtime

import (
	"fmt"
	"slices"
	"strconv"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

const (
	GuardRegistrationVersion = "guard-registration/1"
	// MaxGuards is how many guards one Run may hold. A guard is re-evaluated
	// on every state change of the Run, so their number is stated rather than
	// discovered when a Run stops making progress; thirty-two is far past any
	// declaration a person writes by hand.
	MaxGuards = 32
	// MaxGuardObservations bounds one guard's recorded evidence. An
	// observation is only written when it says something new, so this is a
	// ceiling on how many times a guard's answer may change, not on how often
	// it is computed. Reaching it fails the guard rather than dropping the
	// oldest entry: losing an observed true is exactly what REA-008 forbids.
	MaxGuardObservations = 256
)

// GuardRegistration is the durable rule that one exact scope of this Run may
// not start, or may not continue, while the facts say so. It is registered
// before the first admission it protects and it outlives any process: nothing
// about a client's connection is an input to whether it holds.
//
// A guard in this build reads only facts the Run already holds - its workflow
// inputs and the outputs of stages that have already completed - which is the
// same fact space a choice reads. It cannot watch anything outside the Run,
// and no part of it polls, subscribes or waits on an external source.
type GuardRegistration struct {
	SchemaVersion string `json:"schema_version"`
	ID            string `json:"id"`
	RunID         string `json:"run_id"`
	// Kind is start or stop. They are different rules, not one rule read in
	// two directions: a start guard gates work that has not begun, and a stop
	// guard restricts a scope that may already be running.
	Kind         string `json:"kind"`
	InvocationID string `json:"workflow_invocation_id"`
	// TargetStageID narrows a guard to one stage of that invocation. Empty
	// means the whole invocation, which is how the invocation itself is gated.
	TargetStageID string         `json:"target_stage_id,omitempty"`
	Predicate     flow.Predicate `json:"predicate"`
	// Action and OnUnknown belong to a stop guard alone. Both are mandatory
	// there and both are restrictions: there is no implicit fail-open, because
	// a guard that stopped meaning anything when the facts went missing would
	// be at its most permissive exactly when least is known.
	Action    string `json:"action,omitempty"`
	OnUnknown string `json:"on_unknown,omitempty"`
	// Actor is the principal the registration was installed by. An automatic
	// restriction is still executed by someone, and this says by whom.
	Actor string `json:"actor_id"`
	// Status is observing, satisfied, fired or failed. A failed guard is one
	// whose predicate could not be evaluated at all; it keeps refusing
	// admissions rather than guessing an answer.
	Status string `json:"status"`
	// Latched records that a true was observed while the cursor was behind it.
	// It never clears: a false arriving after an accepted true must not be able
	// to erase the true, which is the whole of REA-008's latch.
	Latched bool `json:"latched"`
	// Cursor is how far the recorded observations have been acted on. While it
	// is behind, ordinary admissions into the guarded scope are refused: an
	// observation nobody has processed may still be the one that stops it.
	Cursor int64 `json:"cursor"`
	// StopID names the one durable stop this guard created, if it fired. A
	// guard creates at most one: repeating a cause it already acted on would
	// grow an unbounded history of stops that all say the same thing.
	StopID       string             `json:"stop_id,omitempty"`
	Observations []GuardObservation `json:"observations"`
	Created      Observation        `json:"created"`
}

// GuardObservation is one recorded evaluation: what the guard concluded, which
// facts it read, and the state cut it read them at. It exists so that a
// decision can be explained afterwards from what was written down, rather than
// re-derived later against facts that have moved on.
type GuardObservation struct {
	Sequence int64 `json:"sequence"`
	// Truth is true, false, unknown or error. Error is not a fourth logical
	// value: it says the predicate could not be evaluated, and it is kept
	// distinct precisely so that it is never quietly read as unknown.
	Truth  string `json:"truth"`
	Reason string `json:"reason"`
	// Facts are the exact field references this evaluation read, each with the
	// artifact revision it was read from and whether it was present, absent or
	// unavailable. Missing, null and false are three different answers here.
	Facts []ChoiceInput `json:"facts"`
	// Cut is the Run version the facts were read at. Two guards evaluated in
	// one cycle share it, so a multi-fact predicate is never a mixture of one
	// fact from yesterday and another from today.
	Cut       int64       `json:"cut"`
	Processed bool        `json:"processed"`
	Observed  Observation `json:"observation"`
}

// isGuardState is the version from which a Run can hold guard registrations.
func isGuardState(version string) bool { return atLeast(version, CoreGuardStateVersion) }

func guardID(runID, kind, stageID string, index int) string {
	return derivedID("guard", runID, kind, stageID, strconv.Itoa(index))
}

// guardStopID is derived from the guard and the cursor position that caused it,
// so an interrupted firing presents the same stop identity when it is retried.
func guardStopID(guard string, cursor int64) string {
	return derivedID("stop", "guard", guard, strconv.FormatInt(cursor, 10))
}

// guardActions are the only two things a stop guard may do. Both refuse new
// ordinary work in the scope; they differ in what they ask of work already
// running, and that difference is declared rather than inferred from a word.
var guardActions = []string{"pause_scope", "cancel_scope"}

// guardStopKind maps a declared action onto the ordinary stop vocabulary. A
// stop guard creates a standard durable stop, not a second stop mechanism, so
// release, epochs and scope checks apply to it exactly as they do to a human's.
func guardStopKind(action string) string {
	if action == "cancel_scope" {
		return "cancel"
	}
	return "pause"
}

// covers reports whether this guard governs an admission into that scope. A
// guard without a target stage governs the whole invocation.
func (g *GuardRegistration) covers(invocationID, stageID string) bool {
	return g.InvocationID == invocationID && (g.TargetStageID == "" || g.TargetStageID == stageID)
}

func (g *GuardRegistration) pending() bool { return g.Cursor < int64(len(g.Observations)) }

// settled returns the last observation the cursor has already passed. That is
// the only one a decision may rest on: an observation still ahead of the cursor
// has not been acted on, and acting on it is what the cursor records.
func (g *GuardRegistration) settled() *GuardObservation {
	if g.Cursor == 0 {
		return nil
	}
	return &g.Observations[g.Cursor-1]
}

// guardBlock reports why this scope may not admit ordinary work now, naming the
// guard and a machine-readable reason. The order matters: an unprocessed
// observation outranks everything, because until it is processed nobody knows
// whether it was the one that stops the scope.
//
// A start guard is consulted only while its activation does not exist yet. Once
// the graph has opened the activation, the work has begun, and a start guard
// turning false afterwards does not take it back - that is what a stop guard is
// for, and conflating the two would make every start guard a silent canceller.
func (r Run) guardBlock(invocationID, stageID string) (string, string) {
	if !isGuardState(r.SchemaVersion) {
		return "", ""
	}
	for _, id := range r.guardIDs() {
		g := r.Guards[id]
		if !g.covers(invocationID, stageID) {
			continue
		}
		if g.pending() {
			return g.ID, "guard_observations_pending"
		}
		if g.Status == "failed" {
			return g.ID, "guard_evaluation_failed"
		}
		observed := g.settled()
		if observed == nil {
			// Nothing has been decided about this guard yet, so nothing may be
			// admitted under it. A guard that permitted work until its first
			// evaluation would be at its weakest before anyone looked.
			return g.ID, "guard_not_yet_observed"
		}
		if g.Kind == "start" && !r.guardTargetOpen(g) && observed.Truth != string(flow.TruthTrue) {
			return g.ID, "start_condition_" + observed.Truth
		}
		// A stop guard keeps refusing while its condition still reads true or
		// unknown, including after its stop was explicitly released. Release
		// lifts the stop record; it does not make the condition false.
		if g.Kind == "stop" && observed.Truth != string(flow.TruthFalse) {
			return g.ID, "stop_condition_" + observed.Truth
		}
	}
	return "", ""
}

// guardFactSettled reports whether the stage a guard reads has finished. A
// guard is evaluated ahead of the graph, so it routinely asks about a stage
// that has not run; a choice never does, and asking that stage's unfinished
// activation raises an error about the read instead of answering the question
// the guard actually asked, which is whether the fact is there yet. Absent and
// unreadable must not arrive at the same word, so the two are separated here.
func (r Run) guardFactSettled(invocationID string, ref flow.FieldRef) bool {
	if ref.From != "stage_output" {
		return true
	}
	a := r.activationForInvocation(invocationID, ref.StageID)
	return a != nil && a.Status == "completed" && a.Settled != nil
}

// guardTargetOpen reports whether the thing this guard gates has already been
// opened. Opening is recorded once, by the ordinary frontier, so a repeated
// true, a redelivered notification or a restart finds the activation already
// there and creates no second one.
func (r Run) guardTargetOpen(g *GuardRegistration) bool {
	if g.TargetStageID != "" {
		return r.activationForInvocation(g.InvocationID, g.TargetStageID) != nil
	}
	for _, a := range r.Activations {
		if a != nil && a.InvocationID == g.InvocationID {
			return true
		}
	}
	return false
}

// guardIDs is a stable order over the registrations. Two guards that both have
// something to say must be reported in the same sequence in every process.
func (r Run) guardIDs() []string {
	ids := make([]string, 0, len(r.Guards))
	for id := range r.Guards {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// guardInvariant checks the saved registrations without reading artifacts. It
// never repairs a status, advances a cursor or invents an observation.
func (r Run) guardInvariant() error {
	if len(r.Guards) == 0 {
		return nil
	}
	if !isGuardState(r.SchemaVersion) {
		return fmt.Errorf("guard invariant: guards require %s", CoreGuardStateVersion)
	}
	if len(r.Guards) > MaxGuards {
		return fmt.Errorf("guard invariant: more registrations than %d", MaxGuards)
	}
	for id, g := range r.Guards {
		if g == nil || g.ID != id || g.RunID != r.ID || g.SchemaVersion != GuardRegistrationVersion || g.Actor == "" {
			return fmt.Errorf("guard invariant: registration %q does not belong to this Run", id)
		}
		if g.Cursor < 0 || g.Cursor > int64(len(g.Observations)) || len(g.Observations) > MaxGuardObservations {
			return fmt.Errorf("guard invariant: cursor %d does not address %d observations", g.Cursor, len(g.Observations))
		}
		for i := range g.Observations {
			if g.Observations[i].Sequence != int64(i) || g.Observations[i].Processed != (int64(i) < g.Cursor) {
				return fmt.Errorf("guard invariant: observation %d disagrees with the cursor", i)
			}
		}
		switch g.Kind {
		case "start":
			if g.Action != "" || g.OnUnknown != "" || g.StopID != "" || g.Latched {
				return fmt.Errorf("guard invariant: start guard %q carries stop fields", id)
			}
		case "stop":
			if !slices.Contains(guardActions, g.Action) || !slices.Contains(guardActions, g.OnUnknown) {
				return fmt.Errorf("guard invariant: stop guard %q has no declared action", id)
			}
			if g.StopID != "" && !g.Latched {
				return fmt.Errorf("guard invariant: stop guard %q fired without a latched cause", id)
			}
		default:
			return fmt.Errorf("guard invariant: unknown guard kind %q", g.Kind)
		}
		if !slices.Contains([]string{"observing", "satisfied", "fired", "failed"}, g.Status) {
			return fmt.Errorf("guard invariant: unknown guard status %q", g.Status)
		}
	}
	return nil
}

// validateGuardDeclaration checks one declared guard against the pinned plan
// before the Run exists. What can be refused here is refused here: an
// undeclared port, a reference this build cannot resolve for a guard, or two
// literals of different types are authoring mistakes, and turning any of them
// into unknown at run time would hide them behind a permissive default.
func validateGuardDeclaration(d GuardDeclaration, p *flow.Plan) error {
	refuse := func(code, message string) error { return local.Reject(code, message) }
	switch d.Kind {
	case "start":
		if d.Action != "" || d.OnUnknown != "" {
			return refuse("invalid_guard", "a start guard declares no action: it gates work, it does not restrict it")
		}
	case "stop":
		if !slices.Contains(guardActions, d.Action) || !slices.Contains(guardActions, d.OnUnknown) {
			return refuse("invalid_guard", "a stop guard declares both action and on_unknown as pause_scope or cancel_scope")
		}
	default:
		return refuse("invalid_guard", "a guard is declared as start or stop")
	}
	if d.TargetStageID != "" {
		if _, declared := p.Workflow.Definition.Stages[d.TargetStageID]; !declared {
			return refuse("invalid_guard", "the guarded stage is not declared by this workflow")
		}
	}
	if err := flow.ValidateGuardPredicate(d.Predicate); err != nil {
		return err
	}
	return flow.WalkPredicateFields(d.Predicate, func(ref flow.FieldRef) error {
		var port flow.Port
		switch ref.From {
		case "workflow_input":
			declared, ok := p.Workflow.Inputs[ref.Port]
			if !ok || ref.StageID != "" {
				return refuse("condition_input_invalid", "the guarded workflow declares no such input")
			}
			port = declared.Port
		case "stage_output":
			declared, ok := p.StageOutputs(ref.StageID)[ref.Port]
			if !ok {
				return refuse("condition_input_invalid", "the guarded workflow declares no such stage output")
			}
			port = declared.Port
		default:
			// An iteration output is addressed relative to one loop body, and a
			// guard is registered against an invocation rather than an
			// iteration. Accepting the reference would mean choosing a body on
			// the guard's behalf, which nobody declared.
			return refuse("condition_input_invalid", "a guard reads workflow inputs and stage outputs only")
		}
		if port.Format != "json" || port.SchemaRef == nil {
			return refuse("condition_type_mismatch", "a guard reads explicitly typed JSON ports only")
		}
		return nil
	})
}
