package runtime

import (
	"context"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// ResolveOutcomes are the only two things an owner may attest about an
// obligation whose result the authority never observed: the external effect
// was applied, or it was not. Neither is a verdict on the work, and neither
// re-runs anything.
const (
	ResolveOutcomeNotApplied = "not_applied"
	ResolveOutcomeApplied    = "applied"
)

// ResolveObligation closes one uncertain attempt or check by the owner's
// attestation. Recovery deliberately keeps an unproven obligation open and
// keeps the admission slot it holds, because a lease expiring is not proof its
// owner stopped. That leaves exactly one way out, and it is this: a person
// says what happened. The authority records that statement, closes the
// obligation as failed and frees the slot; it never infers the outcome, never
// retries, and never routes the closure through a declared error handler,
// because an attested unknown is not a known technical failure.
func (e *Engine) ResolveObligation(ctx context.Context, runID, commandID, attemptID, checkID, outcome, reason string, expected int64) (local.ApplyResult, error) {
	if attemptID == "" && checkID == "" || attemptID != "" && checkID != "" {
		return local.ApplyResult{}, &flow.Problem{Code: "invalid_resolution", Message: "resolve names exactly one obligation: an attempt or a check"}
	}
	if outcome != ResolveOutcomeNotApplied && outcome != ResolveOutcomeApplied {
		return local.ApplyResult{}, &flow.Problem{Code: "invalid_resolution", Path: "/outcome", Message: "resolve attests " + ResolveOutcomeNotApplied + " or " + ResolveOutcomeApplied}
	}
	if reason == "" {
		return local.ApplyResult{}, &flow.Problem{Code: "invalid_resolution", Path: "/reason", Message: "resolve records why the owner is certain; state it"}
	}
	// A live driver still owns this Run, and an obligation it holds may still
	// settle by itself. Attesting over a working owner would invent the very
	// uncertainty this command exists to close.
	if e.driverLiveFor(runID) {
		return local.ApplyResult{}, local.Reject("driver_active", "a live driver owns this run; stop it before resolving its obligations")
	}
	event, target := "attempt.resolved", attemptID
	if checkID != "" {
		event, target = "check.resolved", checkID
	}
	payload := map[string]any{"run_id": runID, "target": target, "outcome": outcome, "reason": reason}
	return e.apply(ctx, e.owner, commandID, runID, event, payload, &expected, local.CommandCAS, func(r *Run, s local.Snapshot, obs Observation) (local.Change, error) {
		resolution := ObligationResolution{Outcome: outcome, Reason: reason, Actor: e.owner, Observed: obs}
		if attemptID != "" {
			return resolveAttempt(r, commandID, attemptID, resolution, obs)
		}
		return resolveCheck(r, commandID, checkID, resolution, obs)
	})
}

func resolutionFailure(outcome string) string {
	if outcome == ResolveOutcomeApplied {
		return "resolved_applied"
	}
	return "resolved_not_applied"
}

// unresolvedRemains recomputes the flag from what is still open, so resolving
// one obligation never releases a Run that still holds another.
func unresolvedRemains(r *Run) bool {
	for _, id := range r.Active {
		if attempt := r.Attempts[id]; attempt != nil && attempt.Status == "uncertain" {
			return true
		}
	}
	for _, check := range r.CheckExecutions {
		if check != nil && check.Status == "uncertain" {
			return true
		}
	}
	return false
}

func resolveAttempt(r *Run, commandID, attemptID string, resolution ObligationResolution, obs Observation) (local.Change, error) {
	a := r.Attempts[attemptID]
	if a == nil {
		return local.Change{}, local.Reject("not_found", "this run holds no attempt with that identity")
	}
	if a.Status != "uncertain" {
		return local.Change{}, local.Reject("resolution_conflict", "only an uncertain obligation is resolved by attestation")
	}
	activation := r.Activations[a.ActivationID]
	if activation == nil {
		return local.Change{}, local.ErrIntegrity
	}
	code := resolutionFailure(resolution.Outcome)
	a.Status, a.Settled = "failed", &obs
	if step := r.Steps[a.StepID]; step != nil {
		step.Status, step.Settled = "failed", &obs
	}
	activation.Status, activation.Settled = "failed", &obs
	removeActive(r, attemptID)
	if err := r.setReadyFor(activation.InvocationID, []string{}); err != nil {
		return local.Change{}, err
	}
	if err := r.setInvocationStatus(activation.InvocationID, "failed", &obs); err != nil {
		return local.Change{}, err
	}
	if err := diagnostic(r, commandID, attemptID, code, "resolution", "The owner attested this obligation's outcome; no execution was repeated", obs); err != nil {
		return local.Change{}, err
	}
	r.HasUnresolvedEffects = unresolvedRemains(r)
	if !r.HasUnresolvedEffects && r.Status == "uncertain" {
		r.Status = "failed"
	}
	return local.Change{ReleaseSlot: attemptID}, nil
}

func resolveCheck(r *Run, commandID, checkID string, resolution ObligationResolution, obs Observation) (local.Change, error) {
	check := r.CheckExecutions[checkID]
	if check == nil {
		return local.Change{}, local.Reject("not_found", "this run holds no check with that identity")
	}
	if check.Status != "uncertain" {
		return local.Change{}, local.Reject("resolution_conflict", "only an uncertain obligation is resolved by attestation")
	}
	code := resolutionFailure(resolution.Outcome)
	check.Status, check.Settled = "failed", &obs
	if r.ActiveCheckID == checkID {
		r.ActiveCheckID = ""
	}
	if err := diagnostic(r, commandID, "", code, "resolution", "The owner attested this check's outcome; no execution was repeated", obs); err != nil {
		return local.Change{}, err
	}
	r.HasUnresolvedEffects = unresolvedRemains(r)
	if !r.HasUnresolvedEffects && r.Status == "uncertain" {
		r.Status = "failed"
	}
	return local.Change{ReleaseSlot: checkID}, nil
}
