package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"time"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// SessionTiming counts report availability outside declared human waits, not
// CPU time in a host process this authority does not control.
type SessionTiming struct {
	Limits       flow.SessionLimits `json:"limits"`
	RemainingMS  int64              `json:"remaining_ms"`
	Observed     Observation        `json:"observed"`
	WaitDeadline *Observation       `json:"wait_deadline,omitempty"`
	SlotHeld     bool               `json:"slot_held"`
}

// A delivery binds the immutable original instruction to the current answer
// and allowance. The initial ExecutionEnvelope is never rewritten on resume.
type SessionDelivery struct {
	BaseEnvelopeDigest string                     `json:"base_envelope_digest"`
	Generation         int64                      `json:"generation"`
	DecisionContext    map[string]json.RawMessage `json:"decision_context,omitempty"`
	Timing             SessionTiming              `json:"timing"`
	Deadline           Observation                `json:"deadline"`
}

func timedSession(a *Attempt) bool {
	return a != nil && a.Session != nil && a.Session.SchemaVersion == AssistedSessionTimingVersion && a.Session.Timing != nil
}

func sessionDelivery(a *Attempt) SessionDelivery {
	return SessionDelivery{rawDigest(a.Envelope), a.Session.DeliveryGeneration, cloneDecisionContext(a.Session.DecisionContext), *a.Session.Timing, a.Deadline}
}

func recordTimedDelivery(a *Attempt, observed Observation) error {
	if !timedSession(a) || a.Session.Timing.RemainingMS < 1 {
		return local.ErrIntegrity
	}
	timing := a.Session.Timing
	deadline, err := sessionDeadline(observed, timing.RemainingMS)
	if err != nil {
		return err
	}
	timing.Observed, timing.WaitDeadline, timing.SlotHeld = observed, nil, true
	a.Deadline = deadline
	a.Session.DeliveryGeneration++
	a.Session.HostState, a.Session.Handed = SessionAwaiting, observed
	data, err := canonical(sessionDelivery(a))
	if err == nil {
		a.EnvelopeDigest = rawDigest(data)
	}
	return err
}

func sessionDeadline(observed Observation, milliseconds int64) (Observation, error) {
	if milliseconds < 1 || milliseconds > flow.MaxSessionTimeoutMS {
		return Observation{}, local.ErrIntegrity
	}
	now, err := time.Parse(time.RFC3339Nano, observed.UTC)
	if err != nil {
		return Observation{}, local.ErrIntegrity
	}
	due := observed
	due.UTC = now.Add(time.Duration(milliseconds) * time.Millisecond).Format(time.RFC3339Nano)
	due.MonotonicMS += milliseconds
	return due, nil
}

func timingObservationAfter(now Observation, previous ...Observation) error {
	current, err := time.Parse(time.RFC3339Nano, now.UTC)
	if err != nil {
		return local.ErrIntegrity
	}
	for _, observation := range previous {
		before, err := time.Parse(time.RFC3339Nano, observation.UTC)
		if err != nil {
			return local.ErrIntegrity
		}
		if current.Before(before) {
			return local.Reject("deadline_clock_unqualified", "the local clock moved behind a saved timing boundary")
		}
	}
	return nil
}

func sessionReportAdmissible(r Run, a *Attempt, now Observation) error {
	if !timedSession(a) {
		return assistedReportAdmissible(a.Admitted, a.Deadline, now)
	}
	if err := timingObservationAfter(now, r.LastObserved, a.Session.Timing.Observed); err != nil {
		return err
	}
	return assistedReportAdmissible(a.Session.Timing.Observed, a.Deadline, now)
}

func consumeSessionTime(r Run, a *Attempt, now Observation) error {
	if err := sessionReportAdmissible(r, a, now); err != nil {
		return err
	}
	timing := a.Session.Timing
	start, _ := time.Parse(time.RFC3339Nano, timing.Observed.UTC)
	end, _ := time.Parse(time.RFC3339Nano, now.UTC)
	elapsed := end.Sub(start)
	spent := elapsed.Milliseconds()
	if elapsed%time.Millisecond != 0 {
		spent++ // Rounding must never refill an allowance through small questions.
	}
	if spent >= timing.RemainingMS {
		return local.Reject("attempt_deadline_expired", "no active allowance remains for another delivery")
	}
	timing.RemainingMS -= spent
	timing.Observed = now
	return nil
}

func decisionWaitAdmissible(r Run, a *Attempt, now Observation) error {
	if err := timingObservationAfter(now, r.LastObserved, a.Session.Timing.Observed); err != nil {
		return err
	}
	if due := a.Session.Timing.WaitDeadline; due != nil {
		current, _ := time.Parse(time.RFC3339Nano, now.UTC)
		deadline, err := time.Parse(time.RFC3339Nano, due.UTC)
		if err != nil {
			return local.ErrIntegrity
		}
		if !current.Before(deadline) {
			return local.Reject("decision_wait_expired", "the declared time to answer this question has expired; inspect recovery")
		}
	}
	return nil
}

func (r Run) executingAttempts() int64 {
	var count int64
	for _, id := range r.Active {
		a := r.Attempts[id]
		if !timedSession(a) || a.Session.Timing.SlotHeld {
			count++
		}
	}
	return count
}

func sessionTimingInvariant(r Run) error {
	for _, a := range r.Attempts {
		if a == nil || a.Session == nil {
			continue
		}
		s := a.Session
		if s.Timing == nil && s.SchemaVersion != AssistedSessionTimingVersion {
			continue
		}
		if !isTimingState(r.SchemaVersion) || !timedSession(a) {
			return errors.New("session timing requires the timed state and session editions")
		}
		timing := s.Timing
		step := r.Steps[a.StepID]
		matched := false
		if step != nil {
			for _, pin := range r.Definitions {
				if pin.Ref == step.Ref {
					var definition flow.StepDefinition
					matched = json.Unmarshal(pin.Bytes, &definition) == nil && reflect.DeepEqual(definition.SessionLimits, &timing.Limits)
					break
				}
			}
		}
		if !matched {
			return errors.New("session allowance differs from its pinned definition")
		}
		if timing.Limits.ActiveTimeoutMS < 1 || timing.Limits.ActiveTimeoutMS > flow.MaxSessionTimeoutMS || timing.RemainingMS < 1 || timing.RemainingMS > timing.Limits.ActiveTimeoutMS || s.DeliveryGeneration < 1 {
			return errors.New("invalid saved session allowance")
		}
		if wait := timing.Limits.DecisionWaitTimeoutMS; wait != nil && (*wait < 1 || *wait > flow.MaxSessionTimeoutMS) {
			return errors.New("invalid saved decision wait allowance")
		}
		if err := timingObservationAfter(timing.Observed, a.Admitted); err != nil {
			return err
		}
		switch s.HostState {
		case SessionAwaiting:
			data, err := canonical(sessionDelivery(a))
			if err != nil || rawDigest(data) != a.EnvelopeDigest || !timing.SlotHeld || timing.WaitDeadline != nil {
				return errors.New("timed delivery does not match its saved allowance")
			}
		case sessionWaitingDecision, SessionWaitingAdmission:
			if timing.SlotHeld || a.Settled != nil {
				return errors.New("parked session cannot occupy execution capacity or be settled")
			}
		case SessionReported, SessionDisconnected:
		default:
			return errors.New("unknown timed session state")
		}
	}
	return nil
}

// Expiry is observed on a command/read, not by a background timer. Reads do
// not settle work; they stop advertising an expired delivery as executable.
func sessionTimingIssue(r Run, now Observation) (*Attempt, string) {
	for _, id := range r.Active {
		a := r.Attempts[id]
		if !timedSession(a) || a.Settled != nil || a.Session.HostState == SessionDisconnected || a.Session.HostState == SessionReported {
			continue
		}
		var err error
		if a.Session.HostState == SessionAwaiting {
			err = sessionReportAdmissible(r, a, now)
		} else {
			err = decisionWaitAdmissible(r, a, now)
		}
		if err != nil {
			problem, _ := ProblemFor(err)
			return a, problem.Code
		}
	}
	return nil, ""
}

func (e *Engine) expireSession(ctx context.Context, r Run, view local.ReadView) (bool, error) {
	if r.terminal() || r.HasUnresolvedEffects {
		return false, nil
	}
	a, code := sessionTimingIssue(r, e.clock.now())
	if a == nil {
		return false, nil
	}
	if r.cancelRequestedFor(r.Activations[a.ActivationID].InvocationID) {
		return false, nil // Explicit cancellation remains usable after expiry/rollback.
	}
	if code == "deadline_clock_unqualified" {
		return false, local.Reject(code, "the clock contradicts saved timing; inspect recovery before continuing")
	}
	return true, e.closeSession(ctx, r, view, a, code)
}

func (e *Engine) resumeTimedSession(ctx context.Context, loaded Run, view local.ReadView, attempt *Attempt) error {
	if !timedSession(attempt) {
		return local.ErrIntegrity
	}
	pin, blocked, err := e.admissionGate(ctx)
	if err != nil {
		return err
	}
	packagePin, packageBlocked, err := e.revokedPin(ctx, loaded)
	if err != nil {
		return err
	}
	if blocked == nil {
		blocked = packageBlocked
	}
	pins := []local.ControlPin{}
	if packagePin != nil {
		pins = append(pins, *packagePin)
	}
	var claimMutation func(local.AuthoritySnapshot, Observation) (json.RawMessage, error)
	if attempt.Session.ClaimID != "" {
		binding, err := e.prepareClaimRunBinding(ctx, loaded.ID, attempt.Session.ClaimID, attempt.Session.ClaimGeneration)
		if err != nil {
			return err
		}
		if pin != nil {
			pins = append(pins, *pin)
		}
		pin, claimMutation = &binding.Pin, binding.mutate
	}
	// Like ordinary admission, another drive rechecks changing capacity. Reusing
	// a rejected receipt would pin yesterday's busy slot forever.
	id := newID("command")
	_, err = e.applyControlledWithControlMutation(ctx, pin, pins, claimMutation, e.owner, id, loaded.ID, "attempt.observed", map[string]any{"attempt_id": attempt.ID, "session_transition": "resumed"}, &view.Snapshot.Version, local.CommandCAS, func(r *Run, _ local.Snapshot, obs Observation) (local.Change, error) {
		if blocked != nil {
			return local.Change{}, blocked
		}
		a := r.Attempts[attempt.ID]
		if !timedSession(a) || a.Session.HostState != SessionWaitingAdmission || a.Session.PrincipalID != e.owner || a.Settled != nil || !slices.Contains(r.Active, a.ID) {
			return local.Change{}, local.Reject("session_state_conflict", "the answered delivery is not awaiting admission for this principal")
		}
		invocation := r.Activations[a.ActivationID].InvocationID
		limit, err := r.declaredParallelism()
		if err != nil {
			return local.Change{}, err
		}
		if r.terminal() || r.admissionsBlockedFor(invocation) || r.cancelRequestedFor(invocation) || r.HasUnresolvedEffects || r.ActiveCheckID != "" || r.executingAttempts() >= limit {
			return local.Change{}, local.Reject("admission_blocked", "saved answer is retained; current restrictions or execution capacity prevent delivery")
		}
		if err := decisionWaitAdmissible(*r, a, obs); err != nil {
			return local.Change{}, err
		}
		if err := recordTimedDelivery(a, obs); err != nil {
			return local.Change{}, err
		}
		a.ControlEpoch = r.ControlEpoch
		data, err := canonical(map[string]any{"attempt_id": a.ID, "session_transition": "resumed", "envelope_digest": a.EnvelopeDigest, "delivery": sessionDelivery(a), "observation": obs})
		return local.Change{AcquireSlot: a.ID, RequireStorageBudget: true, Events: []local.EventInput{{Type: "attempt.observed", Version: 1, Data: data}}}, err
	})
	return err
}

// A host handoff is not evidence that an OS process never started. Close only
// effect-free work; keep potentially changed workspace obligations for recovery.
func (e *Engine) closeSession(ctx context.Context, loaded Run, view local.ReadView, attempt *Attempt, reason string) error {
	id := derivedID("command", attempt.ID, "session-close", reason, fmt.Sprint(view.Snapshot.Version))
	_, err := e.apply(ctx, e.owner, id, loaded.ID, "attempt.observed", map[string]any{"attempt_id": attempt.ID, "session_transition": "closed", "reason": reason}, &view.Snapshot.Version, local.CommandCAS, func(r *Run, _ local.Snapshot, obs Observation) (local.Change, error) {
		a := r.Attempts[attempt.ID]
		if a == nil || a.Session == nil || a.Settled != nil || !slices.Contains(r.Active, a.ID) || a.Session.HostState == SessionDisconnected || a.Session.HostState == SessionReported {
			return local.Change{}, local.Reject("session_state_conflict", "this session no longer has an open delivery")
		}
		activation := r.Activations[a.ActivationID]
		if activation == nil {
			return local.Change{}, local.ErrIntegrity
		}
		if reason == "run_cancelled" {
			if !r.cancelRequestedFor(activation.InvocationID) {
				return local.Change{}, local.Reject("cancel_conflict", "this session has not been cancelled")
			}
		} else {
			_, current := sessionTimingIssue(*r, obs)
			if current != reason {
				return local.Change{}, local.Reject("deadline_not_reached", "no matching expired session is current")
			}
		}
		if pending := r.PendingDecision; pending != nil && pending.AttemptID == a.ID {
			for index := range r.DecisionLedger {
				record := &r.DecisionLedger[index]
				if record.AttemptID == a.ID && record.Status == "pending" {
					record.Status, record.Observed = "rejected", &obs
					if timedSession(a) {
						record.Status, record.ClosureReason = "expired", reason
						if reason == "run_cancelled" {
							record.Status = "cancelled"
						}
					}
				}
			}
			r.PendingDecision = nil
		}
		plan, err := r.planFor(activation.InvocationID)
		if err != nil {
			return local.Change{}, err
		}
		a.Session.HostState = SessionDisconnected
		change := local.Change{}
		if plan.Steps[activation.StageID].Effects.Class != "none" || r.HasUnresolvedEffects {
			r.HasUnresolvedEffects = true
			a.Status, r.Steps[a.StepID].Status, activation.Status = "uncertain", "uncertain", "uncertain"
			if err := r.setInvocationStatus(activation.InvocationID, "uncertain", nil); err != nil {
				return local.Change{}, err
			}
		} else {
			status := "failed"
			if reason == "run_cancelled" {
				status = "cancelled"
			}
			a.Status, a.Settled = status, &obs
			step := r.Steps[a.StepID]
			step.Status, step.Settled, activation.Status, activation.Settled = status, &obs, status, &obs
			if !timedSession(a) || a.Session.Timing.SlotHeld {
				change.ReleaseSlot = a.ID
			}
			if timedSession(a) {
				a.Session.Timing.SlotHeld = false
			}
			removeActive(r, a.ID)
			if err := r.setReadyFor(activation.InvocationID, []string{}); err != nil {
				return local.Change{}, err
			}
			var settled = &obs
			if status == "cancelled" && isInvocationState(r.SchemaVersion) {
				status, settled = "stopping", nil
			}
			if err := r.setInvocationStatus(activation.InvocationID, status, settled); err != nil {
				return local.Change{}, err
			}
		}
		if err := diagnostic(r, id, a.ID, reason, "dispatch", "the host delivery was closed without an OS-process outcome", obs); err != nil {
			return local.Change{}, err
		}
		return change, nil
	})
	return err
}
