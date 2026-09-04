package runtime

import (
	"fmt"
	"slices"

	"github.com/stenhigh/prifly/internal/flow"
)

const RepeatDecisionVersion = "repeat-decision/1"

// RepeatProgress is the materialized frontier of one repeat activation. Its
// counter includes every durably created body, including an unfinished one.
// Only the latest decision is retained here; earlier decisions and every body
// remain in the journal and invocation tree. There is no mutable output cache.
type RepeatProgress struct {
	IterationCount          int64           `json:"iteration_count"`
	CurrentBodyInvocationID string          `json:"current_body_workflow_invocation_id,omitempty"`
	LastDecision            *RepeatDecision `json:"last_decision,omitempty"`
}

// RepeatDecision records a decision about one exact body invocation, not an
// instruction to re-run that body. A continue decision and its next body commit
// together before any worker admission. Observation is assigned in that commit.
type RepeatDecision struct {
	SchemaVersion        string        `json:"schema_version"`
	ID                   string        `json:"id"`
	RunID                string        `json:"run_id"`
	InvocationID         string        `json:"workflow_invocation_id"`
	ActivationID         string        `json:"stage_activation_id"`
	StageID              string        `json:"stage_id"`
	WorkflowRef          flow.Ref      `json:"workflow_ref"`
	BodyInvocationID     string        `json:"body_workflow_invocation_id"`
	Iteration            int64         `json:"iteration"`
	BodyStatus           string        `json:"body_status"`
	BodyOutcome          *string       `json:"body_outcome"`
	UntilResult          string        `json:"until_result"`
	Inputs               []ChoiceInput `json:"inputs"`
	Route                string        `json:"route"`
	NextBodyInvocationID string        `json:"next_body_workflow_invocation_id,omitempty"`
	NextStageID          string        `json:"next_stage_id,omitempty"`
	Failure              string        `json:"failure,omitempty"`
	FailurePath          string        `json:"failure_path,omitempty"`
	Observed             Observation   `json:"observation"`
}

// isInvocationState is a closed version whitelist. An unknown future version
// must never inherit either tree semantics or the legacy root-only fallback.
func isInvocationState(version string) bool { return atLeast(version, CoreInvocationStateVersion) }

func isRepeatState(version string) bool { return atLeast(version, CoreRepeatStateVersion) }

// Context state carries pinned resources and checks; session state adds the
// assisted host facts on top of it. Older Runs keep their delivered version.
func isContextState(version string) bool { return atLeast(version, CoreContextStateVersion) }

func isSessionState(version string) bool { return atLeast(version, CoreSessionStateVersion) }

// Waiver state carries everything the earlier versions carried and adds the
// recorded quality reductions; parallel state adds branch fan-out on top of it.
func isWaiverState(version string) bool { return atLeast(version, CoreWaiverStateVersion) }

// repeatBodies validates the saved count and exact identities without compiling
// definitions or reading artifacts. Historical bodies remain valid lineage
// members; only the final member is the current body for scheduling and exports.
func (r Run) repeatBodies(activationID string) ([]*Invocation, error) {
	a := r.Activations[activationID]
	if !isRepeatState(r.SchemaVersion) || a == nil || a.ID != activationID || a.Kind != "repeat" || a.StepID != "" || a.Repeat == nil {
		return nil, fmt.Errorf("repeat invariant: missing repeat activation")
	}
	progress := a.Repeat
	if progress.IterationCount < 0 || progress.IterationCount > 100 || progress.IterationCount > int64(len(r.Invocations)) {
		return nil, fmt.Errorf("repeat invariant: invalid iteration count")
	}
	bodies := make([]*Invocation, int(progress.IterationCount))
	for id, inv := range r.Invocations {
		if inv == nil || inv.CallerActivationID != a.ID {
			continue
		}
		if id != inv.ID || inv.RunID != r.ID || inv.ParentInvocationID != a.InvocationID || inv.Iteration == nil || *inv.Iteration < 1 || *inv.Iteration > progress.IterationCount {
			return nil, fmt.Errorf("repeat invariant: body owner or iteration changed")
		}
		index := int(*inv.Iteration - 1)
		if bodies[index] != nil {
			return nil, fmt.Errorf("repeat invariant: duplicate iteration identity")
		}
		bodies[index] = inv
	}
	for i, inv := range bodies {
		if inv == nil {
			return nil, fmt.Errorf("repeat invariant: iteration history is not contiguous")
		}
		if inv.WorkflowRef != bodies[0].WorkflowRef || i+1 < len(bodies) && (inv.Status != "completed" || inv.Outcome == nil || inv.Settled == nil) {
			return nil, fmt.Errorf("repeat invariant: earlier body was not a completed exact workflow")
		}
	}
	if len(bodies) == 0 {
		if progress.CurrentBodyInvocationID != "" || progress.LastDecision != nil {
			return nil, fmt.Errorf("repeat invariant: empty repeat has body or decision")
		}
	} else if progress.CurrentBodyInvocationID != bodies[len(bodies)-1].ID {
		return nil, fmt.Errorf("repeat invariant: current body is not the last created iteration")
	}
	return bodies, nil
}

func (r Run) currentBodyForRepeat(activationID string) *Invocation {
	bodies, err := r.repeatBodies(activationID)
	if err != nil || len(bodies) == 0 {
		return nil
	}
	return bodies[len(bodies)-1]
}

// childMatchesCaller checks membership, not scheduling eligibility. A previous
// iteration is still the owner of its old stages, budgets and observations.
func (r Run) childMatchesCaller(inv *Invocation, caller *Activation) bool {
	if inv == nil || caller == nil || caller.ID != inv.CallerActivationID || caller.InvocationID != inv.ParentInvocationID || inv.RunID != r.ID || r.Invocations[inv.ID] != inv {
		return false
	}
	switch caller.Kind {
	case "call":
		return inv.Iteration == nil && caller.Repeat == nil && r.childForCall(caller.ID) == inv
	case "repeat":
		bodies, err := r.repeatBodies(caller.ID)
		return err == nil && inv.Iteration != nil && *inv.Iteration > 0 && *inv.Iteration <= int64(len(bodies)) && bodies[*inv.Iteration-1] == inv
	case "parallel", "map":
		entered, err := r.parallelBranches(caller.ID)
		return err == nil && inv.Iteration == nil && inv.BranchID != "" && slices.Contains(entered, inv)
	default:
		return false
	}
}

// repeatProgressInvariant checks the bounded projection against existing
// invocations. It never repairs a counter, picks an arbitrary last output or
// recovers an earlier decision by evaluating the predicate again.
func (r Run) repeatProgressInvariant(a *Activation) error {
	if a == nil {
		return fmt.Errorf("repeat invariant: missing activation")
	}
	if a.Kind != "repeat" {
		if a.Repeat != nil {
			return fmt.Errorf("repeat invariant: progress on another stage kind")
		}
		return nil
	}
	bodies, err := r.repeatBodies(a.ID)
	if err != nil {
		return err
	}
	d := a.Repeat.LastDecision
	if d == nil {
		if len(bodies) > 1 {
			return fmt.Errorf("repeat invariant: next body lacks its decision")
		}
		return nil
	}
	parent := r.Invocations[a.InvocationID]
	if parent == nil || d.SchemaVersion != RepeatDecisionVersion || d.ID == "" || d.RunID != r.ID || d.InvocationID != a.InvocationID || d.ActivationID != a.ID || d.StageID != a.StageID || d.WorkflowRef != parent.WorkflowRef || d.Iteration < 1 || d.Iteration > int64(len(bodies)) || d.Inputs == nil {
		return fmt.Errorf("repeat invariant: decision identity changed")
	}
	body := bodies[d.Iteration-1]
	if d.BodyInvocationID != body.ID || d.BodyStatus != body.Status || (d.BodyOutcome == nil) != (body.Outcome == nil) || d.BodyOutcome != nil && *d.BodyOutcome != *body.Outcome {
		return fmt.Errorf("repeat invariant: decision body differs from retained result")
	}
	switch d.UntilResult {
	case "not_evaluated", "true", "false", "unknown", "error":
	default:
		return fmt.Errorf("repeat invariant: unknown until result")
	}
	if body.Settled == nil || d.BodyStatus != "completed" && d.BodyStatus != "failed" || (d.BodyStatus == "completed") != (d.BodyOutcome != nil) || d.BodyStatus == "failed" && d.UntilResult != "not_evaluated" {
		return fmt.Errorf("repeat invariant: decision requires a settled body result")
	}
	if d.UntilResult == "not_evaluated" && len(d.Inputs) != 0 {
		return fmt.Errorf("repeat invariant: skipped predicate contains reads")
	}
	if d.Observed.UTC == "" || d.Observed.Session == "" || d.Observed.MonotonicMS < 0 {
		return fmt.Errorf("repeat invariant: decision lacks a committed observation")
	}
	if d.Route == "continue" {
		if d.Iteration != int64(len(bodies))-1 || d.NextBodyInvocationID != bodies[len(bodies)-1].ID || d.NextStageID != "" || d.BodyStatus != "completed" || d.UntilResult != "false" || d.Failure != "" || d.FailurePath != "" {
			return fmt.Errorf("repeat invariant: continue does not identify the next body")
		}
		return nil
	}
	if d.Iteration != int64(len(bodies)) || d.NextBodyInvocationID != "" {
		return fmt.Errorf("repeat invariant: terminal decision is not for the current body")
	}
	switch d.Route {
	case "on_complete", "on_limit", "on_unknown":
		if d.NextStageID == "" || d.BodyStatus != "completed" || d.Failure != "" || d.FailurePath != "" || a.Status != "completed" || a.Settled == nil {
			return fmt.Errorf("repeat invariant: completed decision has no settled route")
		}
		if d.Route == "on_complete" && d.UntilResult != "not_evaluated" && d.UntilResult != "true" || d.Route == "on_limit" && d.UntilResult != "false" || d.Route == "on_unknown" && d.UntilResult != "unknown" {
			return fmt.Errorf("repeat invariant: outcome route differs from until result")
		}
	case "on_error", "failed":
		if d.Failure == "" || a.Status != "failed" || a.Settled == nil || (d.Route == "on_error") != (d.NextStageID != "") {
			return fmt.Errorf("repeat invariant: failed decision lacks failure or settlement")
		}
		if d.Route == "on_error" && (d.Failure == "condition_unknown" || d.Failure == "unhandled_outcome" || d.Failure == "no_transition" || d.UntilResult != "not_evaluated" && d.UntilResult != "false" && d.UntilResult != "error") {
			return fmt.Errorf("repeat invariant: noncatchable result entered on_error")
		}
	default:
		return fmt.Errorf("repeat invariant: unknown decision route")
	}
	return nil
}
