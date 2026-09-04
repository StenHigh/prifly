package runtime

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// A waiver is a recorded refusal to require one named quality check, not a way
// to make it pass. CTRL-008 keeps the meaningfulness checks outside this reach
// entirely: identity, authorisation, input integrity, resource confinement and
// evidence consistency are core code paths that reject, never package checks a
// declaration could rename into something optional.
const (
	MaxRunWaivers = 64
	// ponytail: one fixed maximum waiver lifetime until a policy contract can
	// state its own; a shorter expiry is always accepted.
	maxWaiverLifetime = 24 * time.Hour
)

// protectedCheckClasses can never be waived. A package declares only content
// and result checks, so naming one of these is refused explicitly rather than
// silently failing to match, and the boundary is stated instead of implied.
var protectedCheckClasses = []string{"identity", "authorization", "input_integrity", "resource_confinement", "evidence_consistency"}

type Waiver struct {
	ID         string        `json:"id"`
	StepID     string        `json:"step_instance_id"`
	CheckRef   flow.Ref      `json:"check_ref"`
	Subjects   []ArtifactRef `json:"subject_refs"`
	Reason     string        `json:"reason"`
	ExpiresAt  string        `json:"expires_at"`
	ApproverID string        `json:"approver_id"`
	PolicyRef  flow.Ref      `json:"policy_ref"`
	Status     string        `json:"status"`
	Created    Observation   `json:"created"`
	AppliedTo  []string      `json:"applied_check_execution_ids"`
}

type WaiveRequest struct {
	CommandID string
	RunID     string
	StepID    string
	CheckRef  flow.Ref
	Subjects  []ArtifactRef
	Reason    string
}

// Waive records an explicit refusal to require one check instance. It creates
// no artifact, cancels no other check and does not turn a verdict into pass.
func (e *Engine) Waive(ctx context.Context, c WaiveRequest) (local.ApplyResult, error) {
	if e.ReadOnly {
		return local.ApplyResult{}, local.ErrReadOnly
	}
	if c.CommandID == "" || c.RunID == "" || c.StepID == "" || c.Reason == "" || len(c.Reason) > 4096 {
		return local.ApplyResult{}, errors.New("explicit command, run, step instance and reason required")
	}
	if slices.Contains(protectedCheckClasses, c.CheckRef.ID) {
		return local.ApplyResult{}, local.Reject("protected_check", "identity, authorisation, input integrity, resource confinement and evidence consistency are not waivable")
	}
	control, _, err := e.ensureControl(ctx)
	if err != nil {
		return local.ApplyResult{}, err
	}
	if !control.allows(e.owner, "project", e.Config.ID, ControlOperationApprove) {
		return local.ApplyResult{}, local.Reject("object_access_denied", "the session principal cannot waive checks for this project")
	}
	r, view, err := e.load(ctx, c.RunID)
	if err != nil {
		return local.ApplyResult{}, err
	}
	if !isWaiverState(r.SchemaVersion) {
		return local.ApplyResult{}, local.Reject("unsupported_state", "this Run predates recorded waivers and keeps its pinned semantics")
	}
	subjects := c.Subjects
	if subjects == nil {
		subjects = []ArtifactRef{}
	}
	command := map[string]any{
		"schema_version": "1", "command_id": c.CommandID, "run_id": c.RunID, "expected_run_version": view.Snapshot.Version,
		// No clock reading belongs in a command's identity: a retry with the
		// same command id must be the same command. The waiver's own window is
		// computed in the transform from the recorded observation.
		"payload": map[string]any{"step_instance_id": c.StepID, "check_ref": c.CheckRef, "subject_refs": subjects, "reason": c.Reason},
	}
	encoded, err := canonical(command)
	if err != nil {
		return local.ApplyResult{}, err
	}
	if err := flow.ValidateProtocol("WaiveCommand", encoded); err != nil {
		return local.ApplyResult{}, err
	}
	return e.apply(ctx, e.owner, c.CommandID, c.RunID, "diagnostic.recorded", command, &view.Snapshot.Version, local.CommandCAS, func(r *Run, _ local.Snapshot, obs Observation) (local.Change, error) {
		if r.terminal() {
			return local.Change{}, local.Reject("terminal_run", "a terminal run does not accept a new waiver")
		}
		if len(r.Waivers) >= MaxRunWaivers {
			return local.Change{}, local.Reject("waiver_capacity", "this run already holds its maximum recorded waivers")
		}
		step := r.Steps[c.StepID]
		if step == nil {
			return local.Change{}, local.Reject("not_found", "no step instance with this identity")
		}
		now, err := time.Parse(time.RFC3339Nano, obs.UTC)
		if err != nil {
			return local.Change{}, local.ErrIntegrity
		}
		waiver := Waiver{
			ID: derivedID("waiver", r.ID, c.CommandID), StepID: c.StepID, CheckRef: c.CheckRef, Subjects: subjects,
			Reason: c.Reason, ExpiresAt: now.Add(maxWaiverLifetime).Format(time.RFC3339Nano), ApproverID: e.owner,
			PolicyRef: rPolicy(*r), Status: "active", Created: obs, AppliedTo: []string{},
		}
		for _, existing := range r.Waivers {
			if existing.Status == "active" && existing.StepID == waiver.StepID && existing.CheckRef == waiver.CheckRef {
				return local.Change{}, local.Reject("waiver_present", "an active waiver already covers this check instance")
			}
		}
		r.Waivers = append(r.Waivers, waiver)
		if err := diagnostic(r, c.CommandID, "", "check_waived", "acceptance", "A named quality check was explicitly waived; it did not pass", obs); err != nil {
			return local.Change{}, err
		}
		return local.Change{}, nil
	})
}

// waiverFor finds the active waiver covering exactly this check instance. A
// waiver covers the check it names and nothing else: a neighbour's failure is
// still a failure.
func (r *Run) waiverFor(stepID string, ref flow.Ref, obs Observation) *Waiver {
	due, err := time.Parse(time.RFC3339Nano, obs.UTC)
	if err != nil {
		return nil
	}
	for i := range r.Waivers {
		waiver := &r.Waivers[i]
		if waiver.Status != "active" || waiver.StepID != stepID || waiver.CheckRef != ref {
			continue
		}
		expiry, err := time.Parse(time.RFC3339Nano, waiver.ExpiresAt)
		if err != nil || !due.Before(expiry) {
			continue
		}
		return waiver
	}
	return nil
}

// waiverAppliedWithin reports whether a waiver was applied inside this scope.
// A waiver names the step instance it covered, and that step belongs to one
// activation in one invocation, so the reduction is attributed to the scope
// that actually rested on it instead of to every scope in the Run.
func (r Run) waiverAppliedWithin(invocationID string) bool {
	if !isWaiverState(r.SchemaVersion) || !r.WaiverApplied {
		return false
	}
	for _, waiver := range r.Waivers {
		if waiver.Status != "applied" || len(waiver.AppliedTo) == 0 {
			continue
		}
		step := r.Steps[waiver.StepID]
		if step == nil {
			// The covered step is unknown, so the scope cannot be narrowed.
			// Attribute it to the whole Run rather than lose the reduction.
			return invocationID == r.RootInvocationID
		}
		a := r.Activations[step.ActivationID]
		if a == nil {
			return invocationID == r.RootInvocationID
		}
		if r.withinInvocation(a.InvocationID, invocationID) {
			return true
		}
	}
	return false
}

// outcomeWithWaivers keeps the quality reduction visible in the result of the
// scope that rested on it. A declared success is reported as such rather than
// becoming plain success when the report is built.
func outcomeWithWaivers(r *Run, invocationID, declared string) string {
	if declared == "succeeded" && r.waiverAppliedWithin(invocationID) {
		return "completed_with_waivers"
	}
	return declared
}

// reportedOutcome is the outcome a finishing scope may actually report.
// Reporting plain success would hide the reduction, and reporting an outcome
// the workflow never declared would put a value into the result that its own
// contract cannot express, so a scope that rested on a waived check without
// declaring the reduction is refused rather than quietly reported either way.
func reportedOutcome(r *Run, p *flow.Plan, invocationID, declared string) (string, error) {
	outcome := outcomeWithWaivers(r, invocationID, declared)
	if outcome != declared && !slices.Contains(p.Workflow.AllowedOutcomes, outcome) {
		return "", local.Reject("undeclared_waived_outcome", "this scope rested on a waived check but its workflow does not declare "+outcome)
	}
	return outcome, nil
}

// WaiverView is the reference CLI projection of recorded quality reductions.
func WaiverView(r Run) map[string]any {
	waivers := make([]Waiver, len(r.Waivers))
	copy(waivers, r.Waivers)
	return map[string]any{"schema_version": "foundation-waivers/1", "run_id": r.ID, "waiver_applied": r.WaiverApplied, "waivers": waivers}
}
