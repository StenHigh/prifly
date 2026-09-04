package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

type mutation func(*Run, local.Snapshot, Observation) (local.Change, error)

func (e *Engine) load(ctx context.Context, id string) (Run, local.ReadView, error) {
	v, err := e.Store.Read(ctx, id, 0, 1)
	if err != nil {
		return Run{}, v, err
	}
	var r Run
	if err := decodeState(v.Snapshot.Data, &r); err != nil {
		return r, v, err
	}
	if !supportedRun(r) || r.AuthorityID != e.Installation.ID || r.ProjectID != e.Config.ID {
		return r, v, errors.New("unsupported or foreign run manifest")
	}
	if err := invocationInvariant(r); err != nil {
		return r, v, err
	}
	if err := contextPinnedInvariant(r); err != nil {
		return r, v, err
	}
	if err := acceptanceInvariant(r); err != nil {
		return r, v, err
	}
	return r, v, nil
}

func invariant(r Run) error {
	if r.ID == "" || !supportedRun(r) {
		return errors.New("run identity and profile are required")
	}
	if r.terminal() && (len(r.Active) != 0 || r.ActiveCheckID != "" || r.PendingAcceptance != nil || r.HasUnresolvedEffects) {
		return errors.New("terminal invariant: active or unresolved obligations")
	}
	if r.Status == "completed" && r.Outcome == nil {
		return errors.New("completed run lacks outcome")
	}
	if r.Status != "completed" && r.Outcome != nil {
		return errors.New("non-completed run has outcome")
	}
	for _, id := range r.Active {
		a, ok := r.Attempts[id]
		if !ok || a.Settled != nil {
			return errors.New("invalid active attempt set")
		}
	}
	// A Run runs no more at once than it declared. The foundation profile
	// declares one; Core may declare more, and the number is read from the
	// pinned workflow rather than assumed.
	simultaneous, err := r.declaredParallelism()
	if err != nil {
		return err
	}
	if int64(len(r.Active)) > simultaneous {
		return errors.New("declared simultaneity exceeded")
	}
	if err := contextPinnedInvariant(r); err != nil {
		return err
	}
	if err := acceptanceInvariant(r); err != nil {
		return err
	}
	if err := artifactPublicationInvariant(r); err != nil {
		return err
	}
	if err := artifactClosureInvariant(r); err != nil {
		return err
	}
	if err := actionIntentInvariant(r); err != nil {
		return err
	}
	if err := forkProvenanceInvariant(r); err != nil {
		return err
	}
	return invocationInvariant(r)
}

func (e *Engine) apply(ctx context.Context, actor, id, runID, eventType string, payload any, expected *int64, mode local.CommandMode, fn mutation) (local.ApplyResult, error) {
	return e.applyControlled(ctx, nil, actor, id, runID, eventType, payload, expected, mode, fn)
}

// applyControlled pins a command that grants new work to the authority control
// version its caller checked. Only admissions carry a pin: a stop committed in
// the meantime rejects them instead of being overtaken.
func (e *Engine) applyControlled(ctx context.Context, control *local.ControlPin, actor, id, runID, eventType string, payload any, expected *int64, mode local.CommandMode, fn mutation) (local.ApplyResult, error) {
	return e.applyControlledWithPins(ctx, control, nil, actor, id, runID, eventType, payload, expected, mode, fn)
}

func (e *Engine) applyControlledWithPins(ctx context.Context, control *local.ControlPin, pins []local.ControlPin, actor, id, runID, eventType string, payload any, expected *int64, mode local.CommandMode, fn mutation) (local.ApplyResult, error) {
	return e.applyControlledWithControlMutation(ctx, control, pins, nil, actor, id, runID, eventType, payload, expected, mode, fn)
}

// applyControlledWithControlMutation is the narrow coupled path for an
// admission that changes both its Run ledger and the pinned authority control.
// The authority reducer is internal code; its result is committed only after
// the Run reducer accepted in the same SQLite transaction.
func (e *Engine) applyControlledWithControlMutation(ctx context.Context, control *local.ControlPin, pins []local.ControlPin, controlMutation func(local.AuthoritySnapshot, Observation) (json.RawMessage, error), actor, id, runID, eventType string, payload any, expected *int64, mode local.CommandMode, fn mutation) (local.ApplyResult, error) {
	if e.ReadOnly {
		return local.ApplyResult{}, local.ErrReadOnly
	}
	if id == "" {
		return local.ApplyResult{}, errors.New("command_id required")
	}
	commandBytes, err := canonical(map[string]any{"operation": eventType, "payload": payload})
	if err != nil {
		return local.ApplyResult{}, err
	}
	started := time.Now()
	var observed Observation
	command := local.Command{ID: id, Actor: actor, RunID: runID, Payload: commandBytes, ExpectedVersion: expected, Mode: mode, Control: control, Pins: pins, Samples: e.commandTelemetry(id, runID, started)}
	if controlMutation != nil {
		command.ControlMutation = func(snapshot local.AuthoritySnapshot) (json.RawMessage, error) {
			return controlMutation(snapshot, observed)
		}
	}
	result, err := e.Store.Apply(ctx, command, func(s local.Snapshot) (local.Change, error) {
		observed = e.clock.now()
		var r Run
		if len(s.Data) > 0 {
			if err := decodeState(s.Data, &r); err != nil {
				return local.Change{}, err
			}
			if !supportedRun(r) || r.AuthorityID != e.Installation.ID || r.ProjectID != e.Config.ID {
				return local.Change{}, local.Reject("incompatible_run", "unsupported or foreign run")
			}
			if err := invocationInvariant(r); err != nil {
				return local.Change{}, err
			}
			if err := contextPinnedInvariant(r); err != nil {
				return local.Change{}, err
			}
			if err := acceptanceInvariant(r); err != nil {
				return local.Change{}, err
			}
		} else if eventType != "run.created" {
			return local.Change{}, local.Reject("not_found", "run not found")
		}
		before := states(r)
		previousAcceptance := ""
		if r.PendingAcceptance != nil {
			previousAcceptance = r.PendingAcceptance.ID
		}
		profile, stateVersion := r.Profile, r.SchemaVersion
		diagnosticStart := len(r.Diagnostics)
		change, err := fn(&r, s, observed)
		if err != nil {
			return local.Change{}, err
		}
		if len(s.Data) != 0 && (r.Profile != profile || r.SchemaVersion != stateVersion) {
			return local.Change{}, local.Reject("incompatible_run", "pinned profile and state contract cannot change")
		}
		change.RequireStorageBudget = change.RequireStorageBudget || eventType == "step.publication" || eventType == "run.created" || eventType == "attempt.admitted"
		if change.ReceiptOnly {
			return change, nil
		}
		if err := r.syncInvocationState(); err != nil {
			return local.Change{}, err
		}
		failureEvents, err := e.interruptTerminalPublicationFailures(&r, observed)
		if err != nil {
			return local.Change{}, err
		}
		if len(failureEvents) != 0 {
			change.RequireStorageBudget = true
			change.Events = append(change.Events, failureEvents...)
			if err := r.syncInvocationState(); err != nil {
				return local.Change{}, err
			}
		}
		after := states(r)
		keys := make([]string, 0, len(after))
		for key := range after {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		// A Run's state changes are journal facts, not state that a Run carries
		// forward: keeping them in the snapshot meant every version, and every
		// event that records one, repeated the whole history so far. They are
		// recorded once here and read back from the journal.
		transitions := []StateChange{}
		for _, key := range keys {
			if before[key] != after[key] {
				parts := strings.SplitN(key, "|", 2)
				transitions = append(transitions, StateChange{parts[0], parts[1], before[key], after[key], observed})
			}
		}
		r.LastObserved = observed
		if err := invariant(r); err != nil {
			return local.Change{}, err
		}
		change.Data, err = canonicalState(r)
		if err != nil {
			return local.Change{}, err
		}
		if change.RequireStorageBudget && len(change.Data) > 8<<20 {
			return local.Change{}, local.Reject("state_budget_exhausted", "control and settlement reserve is protected")
		}
		if len(change.Events) == 0 {
			data, _ := canonical(map[string]any{"observation": observed, "status": r.Status})
			change.Events = []local.EventInput{{Type: eventType, Version: 1, Data: data}}
		}
		if len(transitions) != 0 {
			data, err := canonical(map[string]any{"transitions": transitions, "observation": observed})
			if err != nil {
				return local.Change{}, err
			}
			change.Events = append(change.Events, local.EventInput{Type: "state.changed", Version: local.EventVersion, Data: data})
		}
		if r.PendingAcceptance != nil && r.PendingAcceptance.ID != previousAcceptance {
			event, err := acceptancePreparedEvent(r.PendingAcceptance)
			if err != nil {
				return local.Change{}, err
			}
			if eventType == "acceptance.prepared" {
				change.Events = []local.EventInput{event}
			} else {
				change.Events = append(change.Events, event)
			}
		}
		if isInvocationState(r.SchemaVersion) {
			// Every terminal invocation has one explicit lifecycle fact, including
			// failures settled by a worker/choice rather than a finish stage.
			ids := make([]string, 0, len(r.Invocations))
			for id := range r.Invocations {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			for _, id := range ids {
				inv := r.Invocations[id]
				if !invocationTerminal(inv.Status) || before["invocation|"+id] == inv.Status {
					continue
				}
				present := false
				for _, event := range change.Events {
					if event.Type != "invocation.finished" && event.Type != "run.finished" {
						continue
					}
					var fact struct {
						InvocationID string `json:"workflow_invocation_id"`
					}
					if err := json.Unmarshal(event.Data, &fact); err != nil {
						return local.Change{}, err
					}
					present = present || fact.InvocationID == id
				}
				if !present {
					event, err := invocationFinishedEvent(inv, r.RootInvocationID, observed)
					if err != nil {
						return local.Change{}, err
					}
					change.Events = append(change.Events, event)
				}
			}
		}
		diagnosticIDs := []string{}
		for _, d := range r.Diagnostics[diagnosticStart:] {
			diagnosticIDs = append(diagnosticIDs, d.ID)
			data, err := canonical(map[string]any{"diagnostic_id": d.ID, "attempt_id": d.AttemptID, "code": d.Code, "origin": d.Origin, "severity": d.Severity, "cause_refs": d.CauseRefs, "observation": d.Observed})
			if err != nil {
				return local.Change{}, err
			}
			change.Events = append(change.Events, local.EventInput{Type: "diagnostic.recorded", Version: 1, Data: data})
		}
		if change.Result == nil {
			change.Result, _ = canonical(map[string]any{"run_id": r.ID, "status": r.Status, "diagnostic_ids": diagnosticIDs})
		}
		return change, nil
	})
	e.recordCommand(id, runID, started, result, err)
	if err != nil {
		return result, err
	}
	if result.Receipt.Rejection != nil {
		return result, result.Receipt.Rejection
	}
	return result, nil
}

// maxRecordedTransitions bounds how much recorded history one read restores.
// A Run with more changes than this is read up to the bound; timing already
// reports what it could not measure rather than inventing the rest.
const maxRecordedTransitions = 1 << 16

// hydrateTransitions restores a Run's recorded state changes from the journal.
// A Run written before this build keeps them in its snapshot, a Run written now
// keeps them as events, and one that spans both is the two in order.
func (e *Engine) hydrateTransitions(ctx context.Context, r *Run) error {
	after := int64(0)
	for len(r.Transitions) < maxRecordedTransitions {
		events, more, err := e.Store.ReadEventsOfType(ctx, r.ID, "state.changed", after, 500)
		if err != nil {
			return err
		}
		for _, event := range events {
			var recorded struct {
				Transitions []StateChange `json:"transitions"`
				Observation Observation   `json:"observation"`
			}
			if err := decode(event.Data, &recorded); err != nil {
				return err
			}
			r.Transitions = append(r.Transitions, recorded.Transitions...)
			after = event.Seq
		}
		if !more {
			return nil
		}
	}
	return nil
}

func (e *Engine) View(ctx context.Context, id string) (RunView, error) {
	r, read, err := e.load(ctx, id)
	if err != nil {
		return RunView{}, err
	}
	if err := e.hydrateTransitions(ctx, &r); err != nil {
		return RunView{}, err
	}
	asOf, live := e.clock.now(), e.driverLiveFor(id)
	timing := Timing(r, asOf, live)
	// Views never dump executable arguments, environment, raw definitions or
	// publication credentials. They do carry what a worker reported about its
	// own work: withholding an accepted summary left the owner reading an empty
	// field where the engine held the text, which reads as work that produced
	// nothing. A view is therefore the owner's to read, not a document to
	// forward wholesale: the reported text is worker-authored and unvetted.
	r.Definitions = nil
	r.ContextResources = nil
	r.Executors = nil
	r.Workflow = nil
	for _, a := range r.Attempts {
		a.TokenHash = ""
		a.Envelope = nil
		a.Candidate = nil
	}
	for _, check := range r.CheckExecutions {
		check.TokenHash = ""
	}
	version := ReadVersion
	if r.Profile == flow.CoreProfile {
		version = CoreReadVersion
	}
	if isInvocationState(r.SchemaVersion) {
		version = CoreInvocationReadVersion
	}
	if r.SchemaVersion == CoreRepeatStateVersion {
		version = CoreRepeatReadVersion
	}
	if isContextState(r.SchemaVersion) {
		version = CoreContextReadVersion
	}
	if isSessionState(r.SchemaVersion) {
		version = CoreSessionReadVersion
	}
	if isWaiverState(r.SchemaVersion) {
		version = CoreWaiverReadVersion
	}
	if isParallelState(r.SchemaVersion) {
		version = CoreParallelReadVersion
	}
	// The same chain stopped at parallel here. A Run must report the read
	// contract its own state version belongs to, not an earlier one that does
	// not describe the fields it carries.
	if isMapState(r.SchemaVersion) {
		version = CoreMapReadVersion
	}
	if isWaitState(r.SchemaVersion) {
		version = CoreWaitReadVersion
	}
	if isGuardState(r.SchemaVersion) {
		version = CoreGuardReadVersion
	}
	if isReportedCostState(r.SchemaVersion) {
		version = CoreReportedCostReadVersion
	}
	if isArtifactPublicationState(r.SchemaVersion) {
		version = CoreArtifactPublicationReadVersion
	}
	if isArtifactClosureState(r.SchemaVersion) {
		version = CoreArtifactClosureReadVersion
	}
	if isPublicationSubscriptionState(r.SchemaVersion) {
		version = CorePublicationSubscriptionReadVersion
	}
	if isPublicationChecksState(r.SchemaVersion) {
		version = CorePublicationChecksReadVersion
	}
	if isPublicationNewOnlyState(r.SchemaVersion) {
		version = CorePublicationNewOnlyReadVersion
	}
	if isPublicationFailureState(r.SchemaVersion) {
		version = CorePublicationFailureReadVersion
	}
	if isDecisionState(r.SchemaVersion) {
		version = CoreDecisionReadVersion
	} else if isWorkspaceTreeState(r.SchemaVersion) {
		version = CoreWorkspaceTreeReadVersion
	} else if isWorkspaceState(r.SchemaVersion) {
		version = CoreWorkspaceReadVersion
	} else if isForkState(r.SchemaVersion) {
		version = CoreForkReadVersion
	} else if isActionDeliveryState(r.SchemaVersion) {
		version = CoreActionDeliveryReadVersion
	} else if isActionGrantAdmissionState(r.SchemaVersion) {
		version = CoreActionGrantAdmissionReadVersion
	} else if isActionAdmissionState(r.SchemaVersion) {
		version = CoreActionAdmissionReadVersion
	} else if isActionIntentState(r.SchemaVersion) {
		version = CoreActionIntentReadVersion
	}
	return RunView{version, read.Snapshot.Version, read.Snapshot.EventSeq, read.Cut, asOf, live, r, timing}, nil
}
func (e *Engine) Events(ctx context.Context, id string, after int64, limit int) (local.ReadView, error) {
	if _, err := e.readAccess(ctx); err != nil {
		return local.ReadView{}, err
	}
	v, err := e.Store.Read(ctx, id, after, limit)
	if err != nil {
		return v, err
	}
	// StateAfter is replay material, not an ordinary diagnostic payload.
	v.Snapshot.Data = nil
	for i := range v.Events {
		v.Events[i].StateAfter = nil
	}
	return v, nil
}
func (e *Engine) driverLive() bool {
	return e.driverLiveFor("")
}
func (e *Engine) driverLiveFor(runID string) bool {
	f, err := os.OpenFile(filepath.Join(e.Root, e.Config.Configuration.StateRoot, "driver.lock"), os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return false
	}
	defer f.Close()
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_SH|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		if runID == "" {
			return true
		}
		b := make([]byte, 256)
		n, err := f.ReadAt(b, 0)
		return (err == nil || n > 0) && string(b[:n]) == runID
	}
	if err == nil {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	}
	return false
}
func (e *Engine) driverLock(runID string) (*os.File, error) {
	path := filepath.Join(e.Root, e.Config.Configuration.StateRoot, "driver.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("driver_already_active: %w", err)
	}
	if err := f.Truncate(0); err != nil {
		f.Close()
		return nil, err
	}
	if _, err := f.WriteAt([]byte(runID), 0); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// nextKind is pure. A paused run may accept facts, but cannot create another
// activation/admission or finish past its restriction boundary.
func nextKind(r Run) (string, string) {
	if r.terminal() {
		return "terminal", ""
	}
	if r.HasUnresolvedEffects || r.Status == "uncertain" {
		return "uncertain", ""
	}
	if r.ActiveCheckID != "" {
		if check := r.CheckExecutions[r.ActiveCheckID]; check != nil && check.Request.Boundary == "artifact_publication" {
			return "check", r.ActiveCheckID
		}
	}
	if r.PendingArtifactPublication != nil {
		return "publication_checks", r.PendingArtifactPublication.ID
	}
	if r.PendingDecision != nil {
		return "waiting_decision", r.PendingDecision.DecisionID
	}
	// An assisted attempt awaiting its host is not work this driver can do:
	// reporting it would stop the driver from handing out the next branch.
	for _, id := range r.Active {
		attempt := r.Attempts[id]
		if attempt == nil {
			return "active", id
		}
		if attempt.Session != nil && attempt.Session.HostState == SessionAwaiting && attempt.Settled == nil {
			// Awaiting a host is not work this driver can do — unless the scope
			// was asked to stop, in which case closing the handoff is exactly
			// the work, and skipping it would leave the stop unfinished.
			activation := r.Activations[attempt.ActivationID]
			if activation == nil || !r.cancelRequestedFor(activation.InvocationID) {
				continue
			}
		}
		return "active", id
	}
	if r.ActiveCheckID != "" {
		return "check", r.ActiveCheckID
	}
	if scope := r.pendingCancellation(); scope != "" {
		return "cancel", scope
	}
	if pending := r.PendingAcceptance; pending != nil {
		if r.restrictedFor(pending.InvocationID) {
			return "restricted", pending.InvocationID
		}
		if r.admissionsBlockedFor(pending.InvocationID) {
			return "resume_required", pending.InvocationID
		}
		if pending.Status == "pending" || pending.Kind == "step_result" {
			return "acceptance", pending.ID
		}
	}
	if isInvocationState(r.SchemaVersion) {
		invID, stageID := r.readyScope()
		if invID != "" {
			if r.restrictedFor(invID) {
				return "restricted", invID
			}
			if r.admissionsBlockedFor(invID) {
				return "resume_required", invID
			}
			// A live guard is the last thing asked before ordinary work is
			// admitted into its scope, so an observation that has not been
			// acted on cannot be overtaken by the admission it might forbid.
			if guard, _ := r.guardBlock(invID, stageID); guard != "" {
				return "guarded", invID
			}
			return "stage", stageID
		}
		if child := r.blockedChild(); child != "" {
			return "blocked_child", child
		}
		return "idle", ""
	}
	if r.restricted() {
		return "restricted", ""
	}
	if r.ResumeRequired {
		return "resume_required", ""
	}
	if len(r.Ready) == 1 {
		return "stage", r.Ready[0]
	}
	return "idle", ""
}

type NextView struct {
	SchemaVersion   string   `json:"schema_version"`
	RunID           string   `json:"run_id"`
	RunVersion      int64    `json:"run_version"`
	Cut             int64    `json:"cut"`
	Action          string   `json:"action"`
	WorkID          string   `json:"work_id"`
	ReadOnly        bool     `json:"read_only"`
	Admission       bool     `json:"admission"`
	DriverLive      bool     `json:"driver_live"`
	ControlEpoch    int64    `json:"control_epoch"`
	ResumeRequired  bool     `json:"resume_required"`
	SafeNextActions []string `json:"safe_next_actions"`
	InvocationID    string   `json:"workflow_invocation_id,omitempty"`
	StageID         string   `json:"stage_id,omitempty"`
}

func (e *Engine) Next(ctx context.Context, id string) (NextView, error) {
	r, v, err := e.load(ctx, id)
	if err != nil {
		return NextView{}, err
	}
	kind, work := nextKind(r)
	actions := []string{"run.status", "run.events"}
	switch kind {
	case "stage", "acceptance", "publication_checks":
		actions = append(actions, "run.drive", "run.pause", "run.cancel")
	case "restricted":
		actions = append(actions, "run.release", "run.cancel")
	case "guarded":
		// A guarded scope holds no slot and nothing fires by itself. Driving it
		// again is the action that re-reads the facts; cancelling is the other.
		actions = append(actions, "run.drive", "run.cancel")
	case "resume_required":
		actions = append(actions, "run.resume", "run.cancel")
	case "active", "check", "cancel":
		actions = append(actions, "run.cancel")
	case "waiting_decision":
		actions = append(actions, "run.decision.answer", "run.cancel")
	case "blocked_child":
		actions = append(actions, "run.cancel")
	case "uncertain":
		actions = append(actions, "doctor")
	}
	// An attempt awaiting its host is work this driver cannot do, so the action
	// stays what the driver sees. Reading the handoff is still the move, and
	// without it the view looks like a Run with nothing left in it.
	for _, attemptID := range r.Active {
		attempt := r.Attempts[attemptID]
		if attempt != nil && attempt.Session != nil && attempt.Session.HostState == SessionAwaiting && attempt.Settled == nil {
			actions = append(actions, "session.task")
			break
		}
	}
	next := NextView{SchemaVersion: "foundation-next/1", RunID: id, RunVersion: v.Snapshot.Version, Cut: v.Cut, Action: kind, WorkID: work, ReadOnly: true, Admission: false, DriverLive: e.driverLiveFor(id), ControlEpoch: r.ControlEpoch, ResumeRequired: r.ResumeRequired, SafeNextActions: actions}
	if isInvocationState(r.SchemaVersion) {
		next.SchemaVersion = CoreInvocationNextVersion
		if r.SchemaVersion == CoreRepeatStateVersion {
			next.SchemaVersion = CoreRepeatNextVersion
		}
		if isContextState(r.SchemaVersion) {
			next.SchemaVersion = CoreContextNextVersion
		}
		if isSessionState(r.SchemaVersion) {
			next.SchemaVersion = CoreSessionNextVersion
		}
		if isWaiverState(r.SchemaVersion) {
			next.SchemaVersion = CoreWaiverNextVersion
		}
		if isParallelState(r.SchemaVersion) {
			next.SchemaVersion = CoreParallelNextVersion
		}
		if kind == "stage" {
			next.InvocationID, next.StageID = r.readyScope()
		}
		if kind == "active" {
			next.InvocationID = r.Activations[r.Attempts[work].ActivationID].InvocationID
		}
		if kind == "check" {
			next.InvocationID = r.CheckExecutions[work].Request.InvocationID
		}
		if kind == "acceptance" {
			next.InvocationID = r.PendingAcceptance.InvocationID
			if activation := r.Activations[r.PendingAcceptance.ActivationID]; activation != nil {
				next.StageID = activation.StageID
			}
		}
		if kind == "publication_checks" {
			next.InvocationID = r.Activations[r.PendingArtifactPublication.ActivationID].InvocationID
		}
		// The chain stopped at parallel, so a map or wait Run reported a next
		// version its own published bundle pins to a different constant. That
		// is corrected here rather than left for the guard bundle to inherit.
		if isMapState(r.SchemaVersion) {
			next.SchemaVersion = CoreMapNextVersion
		}
		if isWaitState(r.SchemaVersion) {
			next.SchemaVersion = CoreWaitNextVersion
		}
		if isGuardState(r.SchemaVersion) {
			next.SchemaVersion = CoreGuardNextVersion
		}
		if isReportedCostState(r.SchemaVersion) {
			next.SchemaVersion = CoreReportedCostNextVersion
		}
		if isArtifactPublicationState(r.SchemaVersion) {
			next.SchemaVersion = CoreArtifactPublicationNextVersion
		}
		if isArtifactClosureState(r.SchemaVersion) {
			next.SchemaVersion = CoreArtifactClosureNextVersion
		}
		if isPublicationSubscriptionState(r.SchemaVersion) {
			next.SchemaVersion = CorePublicationSubscriptionNextVersion
		}
		if isPublicationChecksState(r.SchemaVersion) {
			next.SchemaVersion = CorePublicationChecksNextVersion
		}
		if isPublicationNewOnlyState(r.SchemaVersion) {
			next.SchemaVersion = CorePublicationNewOnlyNextVersion
		}
		if isPublicationFailureState(r.SchemaVersion) {
			next.SchemaVersion = CorePublicationFailureNextVersion
		}
		if isDecisionState(r.SchemaVersion) {
			next.SchemaVersion = CoreDecisionNextVersion
		} else if isWorkspaceTreeState(r.SchemaVersion) {
			next.SchemaVersion = CoreWorkspaceTreeNextVersion
		} else if isWorkspaceState(r.SchemaVersion) {
			next.SchemaVersion = CoreWorkspaceNextVersion
		} else if isForkState(r.SchemaVersion) {
			next.SchemaVersion = CoreForkNextVersion
		} else if isActionDeliveryState(r.SchemaVersion) {
			next.SchemaVersion = CoreActionDeliveryNextVersion
		} else if isActionGrantAdmissionState(r.SchemaVersion) {
			next.SchemaVersion = CoreActionGrantAdmissionNextVersion
		} else if isActionAdmissionState(r.SchemaVersion) {
			next.SchemaVersion = CoreActionAdmissionNextVersion
		} else if isActionIntentState(r.SchemaVersion) {
			next.SchemaVersion = CoreActionIntentNextVersion
		}
		if slices.Contains([]string{"cancel", "restricted", "resume_required", "blocked_child", "guarded"}, kind) {
			next.InvocationID = work
		}
		if next.InvocationID != "" {
			next.ResumeRequired = r.admissionsBlockedFor(next.InvocationID) && !r.restrictedFor(next.InvocationID)
		}
	}
	return next, nil
}

// A receipt is read under current access, including on an exact retry: holding
// a command identity is not proof of the right to read its decision.
func (e *Engine) Receipt(ctx context.Context, commandID string) (local.Receipt, error) {
	if _, err := e.readAccess(ctx); err != nil {
		return local.Receipt{}, err
	}
	return e.Store.LookupReceipt(ctx, e.owner, commandID)
}

type RestrictCommand struct {
	SchemaVersion      string `json:"schema_version"`
	CommandID          string `json:"command_id"`
	Scope              string `json:"scope"`
	ScopeID            string `json:"scope_id"`
	Kind               string `json:"kind"`
	Reason             string `json:"reason"`
	ObservedRunVersion *int64 `json:"observed_run_version,omitempty"`
}

func (e *Engine) Restrict(ctx context.Context, c RestrictCommand) (local.ApplyResult, error) {
	b, err := canonical(c)
	if err != nil {
		return local.ApplyResult{}, err
	}
	if err := flow.ValidateProtocol("RestrictCommand", b); err != nil {
		return local.ApplyResult{}, err
	}
	if c.Scope != "run" && c.Scope != "invocation" {
		return local.ApplyResult{}, errors.New("unsupported_scope: F1 control targets one explicitly selected run")
	}
	runID := c.ScopeID
	if c.Scope == "invocation" {
		runID, err = e.invocationRun(ctx, c.ScopeID)
		if err != nil {
			return local.ApplyResult{}, err
		}
	}
	return e.apply(ctx, e.owner, c.CommandID, runID, "run.restricted", c, nil, local.CommandMonotonic, func(r *Run, s local.Snapshot, obs Observation) (local.Change, error) {
		if r.ID == "" {
			return local.Change{}, local.Reject("not_found", "run not found")
		}
		if r.terminal() {
			return local.Change{}, local.Reject("terminal_run", "terminal run cannot be restricted")
		}
		var inv *Invocation
		if c.Scope == "invocation" {
			inv = r.Invocations[c.ScopeID]
			if !isInvocationState(r.SchemaVersion) || inv == nil {
				return local.Change{}, local.Reject("scope_not_found", "invocation does not belong to this Run")
			}
			if invocationTerminal(inv.Status) {
				return local.Change{}, local.Reject("terminal_invocation", "terminal invocation cannot be restricted")
			}
		}
		r.ControlEpoch++
		stop := Stop{ID: derivedID("stop", e.owner, c.CommandID), Generation: 1, Epoch: r.ControlEpoch, Kind: c.Kind, Reason: c.Reason, Actor: e.owner, Status: "active", Created: obs}
		if isInvocationState(r.SchemaVersion) {
			stop.Scope, stop.ScopeID = c.Scope, c.ScopeID
		}
		r.Stops = append(r.Stops, stop)
		if inv != nil {
			if c.Kind == "cancel" {
				inv.CancelRequested, inv.Status = true, "stopping"
			} else {
				inv.ResumeRequired = true
				if len(r.Active) == 0 {
					inv.Status = "waiting"
				}
			}
			return local.Change{}, nil
		}
		if c.Kind == "cancel" {
			r.CancelRequested = true
			r.Status = "stopping"
		} else {
			r.ResumeRequired = true
			if len(r.Active) == 0 {
				r.Status = "waiting"
			}
		}
		return local.Change{}, nil
	})
}

type StopGeneration struct {
	ID         string `json:"id"`
	Generation int64  `json:"generation"`
}
type ReleaseRequest struct {
	CommandID            string           `json:"command_id"`
	RunID                string           `json:"run_id"`
	ExpectedControlEpoch int64            `json:"expected_control_epoch"`
	Stops                []StopGeneration `json:"stop_generations"`
	Reason               string           `json:"reason"`
}

func (e *Engine) Release(ctx context.Context, c ReleaseRequest) (local.ApplyResult, error) {
	if c.CommandID == "" || c.Reason == "" || len(c.Reason) > 4096 || len(c.Stops) == 0 || len(c.Stops) > 128 {
		return local.ApplyResult{}, errors.New("explicit command, reason and exact stop generations required")
	}
	r, _, err := e.load(ctx, c.RunID)
	if err != nil {
		return local.ApplyResult{}, err
	}
	intentPayload, err := canonical(map[string]any{"run_id": c.RunID, "expected_control_epoch": c.ExpectedControlEpoch, "stop_generations": c.Stops, "reason": c.Reason})
	if err != nil {
		return local.ApplyResult{}, err
	}
	artifact, ib, expiry, err := e.controlIntentFor("stop.release", "run", c.RunID, rPolicy(r), r.registry(), c.CommandID, intentPayload)
	if err != nil {
		return local.ApplyResult{}, err
	}
	command := map[string]any{"schema_version": "1", "command_id": c.CommandID, "scope": "run", "scope_id": c.RunID, "expected_control_epoch": c.ExpectedControlEpoch, "stop_generations": c.Stops, "control_intent_ref": artifact.Ref(), "approval_refs": []any{}, "reason": c.Reason}
	cb, _ := canonical(command)
	if err := flow.ValidateProtocol("ReleaseStopCommand", cb); err != nil {
		return local.ApplyResult{}, err
	}
	return e.apply(ctx, e.owner, c.CommandID, c.RunID, "stop.released", command, nil, local.CommandGuarded, func(r *Run, s local.Snapshot, obs Observation) (local.Change, error) {
		now, err := time.Parse(time.RFC3339Nano, obs.UTC)
		if err != nil || !now.Before(expiry) {
			return local.Change{}, local.Reject("intent_expired", "prepare a new explicit release command against current stops")
		}
		if r.ControlEpoch != c.ExpectedControlEpoch {
			return local.Change{}, local.Reject("control_epoch_conflict", "a newer restriction exists")
		}
		if r.CancelRequested || r.terminal() {
			return local.Change{}, local.Reject("cancel_not_reversible", "cancel or terminal run cannot be released into execution")
		}
		seen := map[string]bool{}
		for _, requested := range c.Stops {
			if seen[requested.ID] {
				return local.Change{}, local.Reject("duplicate_stop", "duplicate stop reference")
			}
			seen[requested.ID] = true
			found := false
			for i := range r.Stops {
				stop := &r.Stops[i]
				if stop.ID == requested.ID && stop.Generation == requested.Generation && stop.Status == "active" {
					if stop.Kind == "cancel" {
						return local.Change{}, local.Reject("cancel_not_reversible", "invocation cancellation cannot be released")
					}
					stop.Status = "released"
					stop.Released = &obs
					found = true
					break
				}
			}
			if !found {
				return local.Change{}, local.Reject("stop_generation_conflict", "stop is absent, changed or already released")
			}
		}
		r.ControlEpoch++
		admission := map[string]any{"schema_version": "1", "id": derivedID("control-admission", e.owner, c.CommandID), "scope": "run", "scope_id": c.RunID, "command_id": c.CommandID, "intent_digest": rawDigest(ib), "approval_refs": []any{}, "control_epoch": r.ControlEpoch, "admitted_at": obs.UTC}
		ab, _ := canonical(admission)
		if err := flow.ValidateProtocol("ControlAdmission", ab); err != nil {
			return local.Change{}, err
		}
		data, _ := canonical(map[string]any{"observation": obs, "control_intent": artifact.Ref(), "control_admission": admission, "stop_generations": c.Stops})
		return local.Change{Events: []local.EventInput{{Type: "stop.released", Version: 1, Data: data}}}, nil
	})
}

type controlIntent struct {
	SchemaVersion          string   `json:"schema_version"`
	ID                     string   `json:"id"`
	Scope                  string   `json:"scope"`
	ScopeID                string   `json:"scope_id"`
	Operation              string   `json:"operation"`
	ProtectedPayloadDigest string   `json:"protected_payload_digest"`
	PolicyRef              flow.Ref `json:"policy_ref"`
	ProposedBy             string   `json:"proposed_by"`
	ExpiresAt              string   `json:"expires_at"`
}

// A retry uses the original immutable five-minute intent. It cannot extend the
// decision's lifetime, replace its payload, or manufacture an approval.
func (e *Engine) controlIntentFor(operation, scope, scopeID string, policy flow.Ref, reg flow.Registry, commandID string, payload []byte) (Artifact, []byte, time.Time, error) {
	intentID := derivedID("intent", e.owner, commandID)
	artifactID := derivedID("artifact", intentID)
	readExisting := func() (Artifact, []byte, time.Time, error) {
		metadata, err := readLocal(e.Root, artifactMetadataPath(artifactID), MaxDefinitionBytes)
		if err != nil {
			return Artifact{}, nil, time.Time{}, err
		}
		var artifact Artifact
		if err := decode(metadata, &artifact); err != nil {
			return Artifact{}, nil, time.Time{}, err
		}
		artifact, data, err := e.Artifact(artifact.Ref())
		if err != nil {
			return Artifact{}, nil, time.Time{}, err
		}
		if err := flow.ValidateProtocol("ControlIntent", data); err != nil {
			return Artifact{}, nil, time.Time{}, err
		}
		var intent controlIntent
		if err := decode(data, &intent); err != nil {
			return Artifact{}, nil, time.Time{}, err
		}
		if intent.ID != intentID || intent.Scope != scope || intent.ScopeID != scopeID || intent.Operation != operation || intent.ProposedBy != e.owner || intent.PolicyRef != policy || intent.ProtectedPayloadDigest != rawDigest(payload) {
			return Artifact{}, nil, time.Time{}, local.ErrCommandConflict
		}
		expiry, err := time.Parse(time.RFC3339Nano, intent.ExpiresAt)
		return artifact, data, expiry, err
	}
	if a, b, expiry, err := readExisting(); err == nil {
		return a, b, expiry, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return Artifact{}, nil, time.Time{}, err
	}
	expiry := time.Now().UTC().Add(5 * time.Minute)
	intent := controlIntent{"1", intentID, scope, scopeID, operation, rawDigest(payload), policy, e.owner, expiry.Format(time.RFC3339Nano)}
	data, err := canonical(intent)
	if err != nil {
		return Artifact{}, nil, time.Time{}, err
	}
	if err := flow.ValidateProtocol("ControlIntent", data); err != nil {
		return Artifact{}, nil, time.Time{}, err
	}
	artifact, err := e.putArtifact(data, "blob", nil, artifactID, map[string]any{"kind": "authority", "authority_id": e.Installation.ID, "command_id": commandID, "port": "control_intent"}, nil, reg)
	if errors.Is(err, ErrArtifactIdentity) {
		return readExisting()
	}
	return artifact, data, expiry, err
}

// A control decision is admitted only inside its immutable intent lifetime; a
// retry reuses that lifetime rather than extending it.
func controlIntentCurrent(intent Artifact, expiry time.Time, obs Observation) error {
	now, err := time.Parse(time.RFC3339Nano, obs.UTC)
	if err != nil || !now.Before(expiry) {
		return local.Reject("intent_expired", "prepare a new explicit control command against current state")
	}
	if intent.ID == "" {
		return local.ErrIntegrity
	}
	return nil
}

func rPolicy(r Run) flow.Ref {
	var w flow.WorkflowRevision
	_ = json.Unmarshal(r.Workflow, &w)
	return w.PolicyRef
}

func (e *Engine) Resume(ctx context.Context, runID, commandID, reason string, expected int64) (local.ApplyResult, error) {
	command := map[string]any{"schema_version": "1", "command_id": commandID, "run_id": runID, "expected_run_version": expected, "payload": map[string]any{"reason": reason}}
	b, _ := canonical(command)
	if err := flow.ValidateProtocol("ResumeCommand", b); err != nil {
		return local.ApplyResult{}, err
	}
	return e.apply(ctx, e.owner, commandID, runID, "run.resumed", command, &expected, local.CommandCAS, func(r *Run, s local.Snapshot, obs Observation) (local.Change, error) {
		if r.terminal() || r.CancelRequested {
			return local.Change{}, local.Reject("terminal_run", "resume cannot reopen cancellation or a terminal run")
		}
		if r.restricted() {
			return local.Change{}, local.Reject("active_stop", "release exact stops separately before resume")
		}
		if r.HasUnresolvedEffects || r.Status == "uncertain" {
			return local.Change{}, local.Reject("recovery_required", "unsettled attempt cannot be retried by resume")
		}
		for _, id := range r.Active {
			if r.Attempts[id].Dispatch != nil {
				return local.Change{}, local.Reject("recovery_required", "resume cannot recover a dispatched attempt")
			}
		}
		r.ResumeRequired = false
		if isInvocationState(r.SchemaVersion) {
			for _, inv := range r.Invocations {
				if !invocationTerminal(inv.Status) {
					inv.ResumeRequired = false
				}
			}
		}
		if len(r.Active) > 0 {
			r.Status = "running"
		} else {
			r.Status = "ready"
		}
		return local.Change{}, nil
	})
}

func diagnostic(r *Run, occurrence, attemptID, code, phase, message string, obs Observation) error {
	return diagnosticDetail(r, occurrence, attemptID, code, phase, message, "", obs)
}

// maxDiagnosticDetailBytes bounds the recorded cause. A stored error text is
// evidence, not a log: it must not grow the Run state without limit.
const maxDiagnosticDetailBytes = 512

func diagnosticDetail(r *Run, occurrence, attemptID, code, phase, message, detail string, obs Observation) error {
	if len(detail) > maxDiagnosticDetailBytes {
		detail = strings.ToValidUTF8(detail[:maxDiagnosticDetailBytes], "")
	}
	// The cause joins the engine-authored sentence instead of taking a field of
	// its own: the published v1 diagnostic contract is pinned by digest.
	if detail != "" {
		message += ": " + detail
	}
	id := derivedID("diagnostic", r.ID, occurrence, phase, code)
	return recordDiagnostic(r, Diagnostic{ID: id, RunID: r.ID, AttemptID: attemptID, Origin: "core", Severity: "error", Code: code, Category: "executor", Phase: phase, Message: message, Observed: obs, CauseRefs: []string{}})
}

func recordDiagnostic(r *Run, d Diagnostic) error {
	if d.ID == "" || d.RunID != r.ID {
		return local.Reject("diagnostic_identity_mismatch", "diagnostic belongs to another run")
	}
	if r.Profile == flow.CoreProfile {
		attempt := r.Attempts[d.AttemptID]
		if d.AttemptID != "" {
			if attempt == nil || attempt.ActivationID == "" || d.ActivationID != "" && d.ActivationID != attempt.ActivationID {
				return local.Reject("diagnostic_identity_mismatch", "diagnostic does not match its attempt activation")
			}
			d.ActivationID = attempt.ActivationID
		}
		if d.ActivationID != "" {
			activation := r.Activations[d.ActivationID]
			if activation == nil || activation.ID != d.ActivationID {
				return local.Reject("diagnostic_identity_mismatch", "diagnostic activation is absent from the run")
			}
			if activation.Kind == "step" {
				step := r.Steps[activation.StepID]
				if step == nil || step.ID != activation.StepID || step.ActivationID != activation.ID || attempt != nil && attempt.StepID != step.ID {
					return local.Reject("diagnostic_identity_mismatch", "diagnostic does not match its step instance")
				}
			} else if activation.StepID != "" || d.AttemptID != "" {
				return local.Reject("diagnostic_identity_mismatch", "control-stage diagnostic cannot refer to a step instance or attempt")
			}
		}
	}
	for _, previous := range r.Diagnostics {
		if previous.ID == d.ID {
			old, err := canonical(previous)
			if err != nil {
				return err
			}
			next, err := canonical(d)
			if err != nil {
				return err
			}
			if !bytes.Equal(old, next) {
				return local.Reject("diagnostic_conflict", "an occurrence cannot be rewritten")
			}
			return nil
		}
	}
	r.Diagnostics = append(r.Diagnostics, d)
	return nil
}
func removeActive(r *Run, id string) {
	r.Active = slices.DeleteFunc(r.Active, func(v string) bool { return v == id })
}
func states(r Run) map[string]string {
	m := map[string]string{}
	if r.ID != "" {
		m["run|"+r.ID] = r.Status
		if isInvocationState(r.SchemaVersion) {
			for id, inv := range r.Invocations {
				m["invocation|"+id] = inv.Status
			}
		} else {
			m["invocation|"+r.RootInvocationID] = r.Status
		}
	}
	for _, a := range r.Activations {
		m["activation|"+a.ID] = a.Status
	}
	for _, s := range r.Steps {
		m["step|"+s.ID] = s.Status
	}
	for _, a := range r.Attempts {
		m["attempt|"+a.ID] = a.Status
	}
	if isContextState(r.SchemaVersion) {
		for id, check := range r.CheckExecutions {
			if check != nil {
				m["check_execution|"+id] = check.Status
			}
		}
	}
	return m
}
