package runtime

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// Drive owns one foreground authority lock. It has no background scheduler,
// retry loop or permission to dispatch an uncertain previous launch.
func (e *Engine) Drive(ctx context.Context, runID string) (retErr error) {
	if e.ReadOnly {
		return local.ErrReadOnly
	}
	lock, err := e.driverLock(runID)
	if err != nil {
		return err
	}
	defer lock.Close()
	defer func() {
		if ctx.Err() != nil {
			retErr = errors.Join(retErr, e.interruptDriver(runID))
		}
	}()
	var socket string
	var closeServer func()
	defer func() {
		if closeServer != nil {
			closeServer()
		}
	}()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		r, v, err := e.load(ctx, runID)
		if err != nil {
			return err
		}
		// A deadline that has passed is work nobody scheduled: the wait holds no
		// frontier, so the driver looks for it directly before asking what is
		// ready. Nothing fires here on its own - this build owns no timer, and
		// a deadline is observed when the authority next looks.
		if isWaitState(r.SchemaVersion) {
			if due := r.dueWait(e.clock.now().UTC); due != nil {
				p, err := r.planFor(due.InvocationID)
				if err != nil {
					return err
				}
				if err := e.resolveWaitWithTimeout(ctx, r, v, p, due); err != nil {
					return err
				}
				continue
			}
		}
		// Guards are read before anything is admitted. Nothing wakes up here
		// either: a guard reads facts this Run already holds, so its answer can
		// only change when the Run does, and it is re-read when the authority
		// next looks rather than the moment a fact lands.
		if progressed, err := e.advanceGuards(ctx, r, v); err != nil {
			return err
		} else if progressed {
			continue
		}
		kind, work := nextKind(r)
		switch kind {
		case "terminal":
			return nil
		case "uncertain":
			return recoveryError(r)
		case "restricted", "resume_required", "idle", "blocked_child", "guarded", "waiting_decision":
			return nil
		case "cancel":
			if err := e.finishCancellation(ctx, runID, work); err != nil {
				return err
			}
		case "check":
			check := r.CheckExecutions[work]
			if check.Dispatch == nil && r.admissionsBlockedFor(check.Request.InvocationID) && !r.cancelRequestedFor(check.Request.InvocationID) {
				return nil
			}
			if err := e.executePendingCheck(ctx, r, v, check); err != nil {
				return err
			}
		case "acceptance":
			if err := e.driveAcceptance(ctx, r, v); err != nil {
				return err
			}
		case "publication_checks":
			if err := e.driveArtifactPublicationChecks(ctx, r, v); err != nil {
				return err
			}
		case "active":
			a := r.Attempts[work]
			activation := r.Activations[a.ActivationID]
			if activation == nil {
				return local.ErrIntegrity
			}
			if a.Dispatch != nil {
				// The saved journal already proves the process group was empty,
				// so nothing of this attempt can still be running: it is closed
				// as a lost settlement instead of holding the slot uncertain.
				if a.ExecutorEnd != nil {
					if err := e.settleRecoveredExecutorEnd(ctx, runID, a.ID); err != nil {
						return err
					}
					continue
				}
				return e.recoverUncertain(ctx, r, "dispatch boundary exists without this driver's live ownership")
			}
			if r.cancelRequestedFor(activation.InvocationID) {
				if err := e.settleUnstarted(ctx, r.ID, a.ID, "", "cancelled"); err != nil {
					return err
				}
				continue
			}
			if r.admissionsBlockedFor(activation.InvocationID) {
				return nil
			}
			// An assisted attempt is held by a session this driver did not
			// start. There is nothing to spawn and nothing to wait on here.
			// A report that arrived without being judged is still closed, so a
			// crash between report and settlement does not lose the result.
			if a.Session != nil {
				if a.Session.HostState == SessionReported && a.Settled == nil {
					return e.settleAssisted(ctx, runID, a.ID)
				}
				return nil
			}
			if socket == "" {
				socket, closeServer, err = e.serveSteps()
				if err != nil {
					return err
				}
			}
			if err := e.executePending(ctx, r, v, a, socket); err != nil {
				return err
			}
		case "stage":
			invocationID, stageID := r.readyScope()
			if stageID != work || invocationID == "" {
				return local.ErrIntegrity
			}
			p, err := r.planFor(invocationID)
			if err != nil {
				return err
			}
			a := r.activationForInvocation(invocationID, work)
			if a == nil {
				activationID, stepID := newID("activation"), newID("step")
				// A wait may have been promised to a sender before it existed,
				// so its identity is derived rather than minted.
				if p.Workflow.Definition.Stages[work].Kind == "wait" {
					activationID = waitActivationID(runID, invocationID, work)
				}
				_, err = e.apply(ctx, e.owner, newID("command"), runID, "stage.activated", map[string]any{"stage_id": work}, &v.Snapshot.Version, local.CommandCAS, func(r *Run, s local.Snapshot, obs Observation) (local.Change, error) {
					if r.admissionsBlockedFor(invocationID) || r.cancelRequestedFor(invocationID) || r.HasUnresolvedEffects {
						return local.Change{}, local.Reject("active_stop", "activation blocked")
					}
					if err := activateFor(r, p, invocationID, work, activationID, stepID, s.EventSeq, obs); err != nil {
						return local.Change{}, err
					}
					if isInvocationState(r.SchemaVersion) {
						data, err := canonical(map[string]any{"stage_activation_id": activationID, "stage_id": work, "workflow_invocation_id": invocationID, "kind": p.Workflow.Definition.Stages[work].Kind, "observation": obs})
						return local.Change{RequireStorageBudget: true, Events: []local.EventInput{{Type: "stage.activated", Version: 1, Data: data}}}, err
					}
					return local.Change{}, nil
				})
				if err != nil {
					return err
				}
				continue
			}
			switch a.Kind {
			case "finish":
				if err := e.finish(ctx, r, v, p, a); err != nil {
					return err
				}
			case "choice":
				decision := e.evaluateChoiceFor(r, p, invocationID, work)
				if _, err := e.commitChoice(ctx, r, v, p, a, newID("command"), decision); err != nil {
					return err
				}
			case "step":
				if err := e.admit(ctx, r, v, p, a); err != nil {
					return err
				}
			case "call":
				if a.Status == "ready" {
					err = e.enterCall(ctx, r, v, p, a)
				} else {
					err = e.returnCall(ctx, r, v, p, a)
				}
				if err != nil {
					return err
				}
			case "repeat":
				if a.Status == "ready" {
					err = e.enterRepeat(ctx, r, v, p, a)
				} else {
					err = e.decideRepeat(ctx, r, v, p, a)
				}
				if err != nil {
					return err
				}
			case "parallel":
				if a.Status == "ready" {
					err = e.enterParallel(ctx, r, v, p, a)
				} else {
					err = e.decideJoin(ctx, r, v, p, a)
				}
				if err != nil {
					return err
				}
			case "wait":
				// Entering is work. Waiting is not: once entered, the driver
				// has nothing to do here until a deadline passes, and a
				// delivered event resolves the wait through its own command.
				if a.Status == "ready" {
					err = e.enterWait(ctx, r, v, p, a)
				} else {
					err = e.resolveWaitWithTimeout(ctx, r, v, p, a)
				}
				if err != nil {
					return err
				}
			case "map":
				// Only entry differs: a map seals its collection into the
				// branches a parallel stage declares. From the first decision
				// on, both are the same fan-out.
				if a.Status == "ready" {
					err = e.enterMap(ctx, r, v, p, a)
				} else {
					err = e.decideJoin(ctx, r, v, p, a)
				}
				if err != nil {
					return err
				}
			default:
				return errors.New("unsupported activation kind")
			}
		default:
			return errors.New("unhandled planner state")
		}
	}
}

func (e *Engine) finish(ctx context.Context, r Run, v local.ReadView, p *flow.Plan, activation *Activation) error {
	stageID, invocationID := activation.StageID, activation.InvocationID
	stage := p.Workflow.Definition.Stages[stageID]
	ports := map[string]flow.InputPort{}
	for name, out := range p.Workflow.Outputs {
		ports[name] = flow.InputPort{Port: out.Port, Required: slices.Contains(out.RequiredFor, stage.Outcome)}
	}
	commandID := newID("command")
	checked := pendingPassed(r, "workflow_output", activation.ID)
	var refs map[string]ArtifactRef
	var err error
	if checked {
		refs = maps.Clone(r.PendingAcceptance.Bindings)
		err = e.validateBoundArtifacts(p, ports, refs)
	} else {
		refs, err = e.prepareBindings(r, invocationID, stage.OutputBindings, ports, commandID)
	}
	if err != nil {
		return e.failPreparation(ctx, r, v, p, activation, err, "output_binding_failed")
	}
	if !checked {
		if prepared, err := e.prepareBoundaryAcceptance(ctx, r, v, p, "workflow_output", activation, refs, commandID); prepared || err != nil {
			return err
		}
	}
	event := "run.finished"
	if invocationID != r.RootInvocationID {
		event = "invocation.finished"
	}
	_, err = e.apply(ctx, e.owner, commandID, r.ID, event, map[string]any{"stage_id": stageID, "workflow_invocation_id": invocationID, "outcome": stage.Outcome, "outputs": refs}, &v.Snapshot.Version, local.CommandCAS, func(r *Run, s local.Snapshot, obs Observation) (local.Change, error) {
		// A finish concerns its own scope. Another branch still running is that
		// branch's business; what must be settled here is this scope's own work.
		// The Run cannot finish over a live branch regardless, because the
		// fan-out that owns them does not settle while any of them is live.
		if r.admissionsBlockedFor(invocationID) || r.cancelRequestedFor(invocationID) || r.activeIn(invocationID) != "" || r.ActiveCheckID != "" || r.HasUnresolvedEffects {
			return local.Change{}, local.Reject("unsettled", "terminal invariant or restriction prevents finish")
		}
		a := r.activationForInvocation(invocationID, stageID)
		if a == nil || a.ID != activation.ID || a.Kind != "finish" || a.Status != "ready" {
			return local.Change{}, local.Reject("stage_conflict", "finish activation is absent")
		}
		if r.PendingAcceptance != nil {
			if !pendingPassed(*r, "workflow_output", activation.ID) || !maps.Equal(refs, r.PendingAcceptance.Bindings) {
				return local.Change{}, local.Reject("acceptance_blocked", "workflow exports differ from the checked boundary")
			}
			r.PendingAcceptance = nil
		}
		a.Status = "completed"
		a.Settled = &obs
		if err := r.setReadyFor(invocationID, []string{}); err != nil {
			return local.Change{}, err
		}
		outcome, err := reportedOutcome(r, p, invocationID, stage.Outcome)
		if err != nil {
			return local.Change{}, err
		}
		if isInvocationState(r.SchemaVersion) {
			inv := r.Invocations[invocationID]
			inv.Outputs, inv.Outcome = refs, &outcome
		} else {
			r.Outputs, r.Outcome = refs, &outcome
		}
		if err := r.setInvocationStatus(invocationID, "completed", &obs); err != nil {
			return local.Change{}, err
		}
		if isInvocationState(r.SchemaVersion) {
			finished, err := invocationFinishedEvent(r.Invocations[invocationID], r.RootInvocationID, obs)
			return local.Change{Events: []local.EventInput{finished}}, err
		}
		return local.Change{}, nil
	})
	return err
}

func (e *Engine) admit(ctx context.Context, r Run, v local.ReadView, p *flow.Plan, a *Activation) error {
	stage := p.Workflow.Definition.Stages[a.StageID]
	step := p.Steps[a.StageID]
	if !isInvocationState(r.SchemaVersion) && len(r.Attempts) >= int(p.Workflow.Limits.MaxStepInstances) {
		return fault("budget_exhausted", "max step instances")
	}
	commandID, attemptID, admissionID, reservationID := newID("command"), newID("attempt"), newID("admission"), newID("reservation")
	checked := pendingPassed(r, "step_input", a.ID)
	var inputs map[string]ArtifactRef
	var err error
	if checked {
		inputs = maps.Clone(r.PendingAcceptance.Bindings)
		err = e.validateBoundArtifacts(p, step.Inputs, inputs)
	} else {
		inputs, err = e.prepareBindings(r, a.InvocationID, stage.InputBindings, step.Inputs, commandID)
	}
	if err != nil {
		return e.failPreparation(ctx, r, v, p, a, err, "input_binding_failed")
	}
	if !checked {
		if prepared, err := e.prepareBoundaryAcceptance(ctx, r, v, p, "step_input", a, inputs, commandID); prepared || err != nil {
			return err
		}
	}
	manifest := ContextManifest{SchemaVersion: "local-context/1", Inputs: map[string]LocalPort{}, Outputs: map[string]OutputSlot{}, Dependencies: []flow.Ref{}}
	for port, ref := range inputs {
		manifest.Inputs[port] = LocalPort{ref, "inputs/" + port}
	}
	for port := range step.Outputs {
		manifest.Outputs[port] = OutputSlot{ArtifactID: fmt.Sprintf("artifact:%x", sha256.Sum256([]byte(attemptID+"/"+port))), Revision: 1, Path: "outputs/" + port}
	}
	for _, d := range r.Definitions {
		manifest.Dependencies = append(manifest.Dependencies, d.Ref)
	}
	executor := r.Executors[executorKey(r, stage.StepRef, step.ID)]
	assisted := isAssistedExecutor(r.Definitions, flow.Executor{AdapterRef: step.Executor.AdapterRef, Operation: step.Executor.Operation})
	var full FullContextManifest
	var sources []ContextSource
	var contextArtifact Artifact
	if executor.ContextProfile != nil {
		full, sources, contextArtifact, err = e.prepareFullContext(r, step, attemptID, commandID, *executor.ContextProfile, inputs)
		manifest.SchemaVersion = "local-context/2"
		manifest.Manifest = &LocalPort{Ref: contextArtifact.Ref(), Path: "context/manifest.json"}
		for i, source := range sources {
			port := LocalPort{Ref: source.Artifact.Ref(), Path: ContextSourcePath(i)}
			manifest.Sources = append(manifest.Sources, port)
			if strings.HasPrefix(full.Entries[i].SourceID, "input:") {
				manifest.Inputs[strings.TrimPrefix(full.Entries[i].SourceID, "input:")] = port
			}
		}
		for _, resource := range r.ContextResources {
			manifest.Dependencies = append(manifest.Dependencies, resource.Ref)
		}
	} else {
		var data []byte
		data, err = canonical(manifest)
		if err == nil {
			schema := builtinRef(r.Definitions, "core:schema/local-context")
			contextArtifact, err = e.putArtifact(data, "json", &schema, "artifact:"+strings.TrimPrefix(attemptID, "attempt:")+"/context", map[string]any{"kind": "authority", "authority_id": r.AuthorityID, "command_id": commandID, "port": "context"}, nil, r.registry())
		}
	}
	if err != nil {
		return e.failPreparation(ctx, r, v, p, a, err, "context_preparation_failed")
	}
	created := e.clock.now()
	now, err := time.Parse(time.RFC3339Nano, created.UTC)
	if err != nil {
		return err
	}
	cfg := executor.Config
	if assisted {
		cfg.TimeoutMS, cfg.MaxOutputBytes = assistedAttemptTimeoutMS, MaxArtifactBytes
	}
	envelope := map[string]any{"schema_version": "1", "run_id": r.ID, "authority_id": r.AuthorityID, "workflow_invocation_id": a.InvocationID, "stage_activation_id": a.ID, "step_instance_id": a.StepID, "attempt_id": attemptID, "execution_admission_id": admissionID, "admitted_run_version": v.Snapshot.Version + 1, "control_epoch": r.ControlEpoch, "workflow_ref": planRef(p), "step_ref": stage.StepRef, "policy_ref": p.Workflow.PolicyRef, "package_lock_digest": r.LockRef.Digest, "input_artifacts": inputs, "context_manifest_ref": contextArtifact.Ref(), "grant_refs": []any{}, "claims": []any{}, "budget_reservation_id": reservationID, "dispatch_not_after": now.Add(30 * time.Second).Format(time.RFC3339Nano), "attempt_deadline": now.Add(time.Duration(cfg.TimeoutMS) * time.Millisecond).Format(time.RFC3339Nano), "output_contracts": step.Outputs}
	envelopeBytes, err := canonical(envelope)
	if err != nil {
		return err
	}
	if err := flow.ValidateProtocol("ExecutionEnvelope", envelopeBytes); err != nil {
		return err
	}
	if executor.ContextProfile != nil {
		rendered, err := RenderContext(full, envelopeBytes, sources)
		if err != nil {
			return e.failPreparation(ctx, r, v, p, a, err, "context_preparation_failed")
		}
		// Rendering may exceed the per-JSON-document compiler limit. It is an
		// immutable transport blob, not a claim of author JSON schema validity.
		artifact, err := e.putArtifact(rendered, "blob", nil, derivedID("artifact", attemptID, "rendering"), map[string]any{"kind": "authority", "authority_id": r.AuthorityID, "command_id": commandID, "port": "rendering"}, []ArtifactRef{contextArtifact.Ref()}, r.registry(), "application/json")
		if err != nil {
			return e.failPreparation(ctx, r, v, p, a, err, "context_preparation_failed")
		}
		manifest.Rendering = &LocalPort{Ref: artifact.Ref(), Path: "context/rendered.json"}
	}
	manifestBytes, err := canonical(manifest)
	if err != nil {
		return err
	}
	if executor.ContextProfile != nil {
		if err := flow.ValidateSchema(r.registry(), builtinVersionRef(r.Definitions, "core:schema/local-context", "2.0.0"), manifestBytes); err != nil {
			return e.failPreparation(ctx, r, v, p, a, err, "context_preparation_failed")
		}
	}
	workspaceName := strings.TrimPrefix(attemptID, "attempt:")
	workspace, err := e.prepareWorkspace(r, stage.StepRef, step, workspaceName, manifest, manifestBytes)
	if err != nil {
		return e.failPreparation(ctx, r, v, p, a, err, "workspace_preparation_failed")
	}
	treeRollback := func() {}
	treeAdmitted := false
	defer func() {
		if !treeAdmitted {
			treeRollback()
		}
	}()
	var handoff *SessionHandoff
	if assisted {
		skills := assistedSkillRefs(step)
		if _, err := e.materializeSkills(r, workspace, skills); err != nil {
			return e.failPreparation(ctx, r, v, p, a, err, "context_preparation_failed")
		}
		version := AssistedSessionVersion
		if isReportedCostState(r.SchemaVersion) {
			version = AssistedSessionCostVersion
		}
		if isWorkspaceState(r.SchemaVersion) {
			version = AssistedSessionWorkspaceVersion
		}
		if len(step.WorkspaceTrees) != 0 {
			version = AssistedSessionTreeVersion
		}
		if isDecisionState(r.SchemaVersion) {
			version = AssistedSessionDecisionVersion
		}
		handoff = &SessionHandoff{SchemaVersion: version, PrincipalID: e.owner, SkillRefs: skills, HostState: SessionAwaiting}
		if version == AssistedSessionDecisionVersion {
			handoff.DecisionContext = decisionSessionContext(r.DecisionCatalog, r.DecisionSheet)
			handoff.DeliveryGeneration = 1
		}
		// A worktree is claimed for a step that declared it will write one. A
		// proposal-only step is handed no worktree, so it cannot quietly share
		// one with another step that is running beside it.
		if step.Effects.Class == "workspace_write" {
			claim, err := e.activeClaim(ctx)
			if err != nil {
				return e.failPreparation(ctx, r, v, p, a, err, "claim_unavailable")
			}
			handoff.ClaimID, handoff.ClaimGeneration = claim.ID, claim.Generation
			if version == AssistedSessionWorkspaceVersion || version == AssistedSessionDecisionVersion {
				handoff.WorkspaceMode = claimMode(claim)
			}
			if version == AssistedSessionTreeVersion || (version == AssistedSessionDecisionVersion && len(step.WorkspaceTrees) != 0) {
				handoff.WorkspaceMode = claimMode(claim)
				trees, rollback, err := e.prepareWorkspaceTrees(r, step, inputs, claim)
				if err != nil {
					return e.failPreparation(ctx, r, v, p, a, err, "workspace_tree_preparation_failed")
				}
				treeRollback, handoff.WorkspaceTrees = rollback, trees
			}
		}
	}
	pin, blocked, err := e.admissionGate(ctx)
	if err != nil {
		return err
	}
	packagePin, packageBlocked, err := e.revokedPin(ctx, r)
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
	_, err = e.applyControlledWithPins(ctx, pin, pins, e.owner, commandID, r.ID, "attempt.admitted", map[string]any{"attempt_id": attemptID, "envelope_digest": rawDigest(envelopeBytes), "reservation_id": reservationID}, &v.Snapshot.Version, local.CommandCAS, func(r *Run, s local.Snapshot, obs Observation) (local.Change, error) {
		if blocked != nil {
			return local.Change{}, blocked
		}
		// Running work in another scope is that scope's business. What bounds
		// admission is this scope having work of its own and the simultaneity
		// the Run itself declared; the authority's slots are enforced by the
		// slot table in this same transaction.
		simultaneous, err := r.declaredParallelism()
		if err != nil {
			return local.Change{}, err
		}
		if r.admissionsBlockedFor(a.InvocationID) || r.cancelRequestedFor(a.InvocationID) || r.HasUnresolvedEffects || r.activeIn(a.InvocationID) != "" || r.ActiveCheckID != "" || int64(len(r.Active)) >= simultaneous {
			return local.Change{}, local.Reject("admission_blocked", "restriction or unsettled work prevents admission")
		}
		stepState := r.Steps[a.StepID]
		if stepState == nil || stepState.Status != "ready" {
			return local.Change{}, local.Reject("step_conflict", "step is not ready")
		}
		if r.PendingAcceptance != nil {
			if !pendingPassed(*r, "step_input", a.ID) || !maps.Equal(inputs, r.PendingAcceptance.Bindings) {
				return local.Change{}, local.Reject("acceptance_blocked", "step inputs differ from the checked boundary")
			}
			r.PendingAcceptance = nil
		}
		deadline, dispatchDeadline := created, created
		deadline.MonotonicMS += cfg.TimeoutMS
		dispatchDeadline.MonotonicMS += dispatchWindow.Milliseconds()
		deadline.UTC = now.Add(time.Duration(cfg.TimeoutMS) * time.Millisecond).Format(time.RFC3339Nano)
		dispatchDeadline.UTC = now.Add(dispatchWindow).Format(time.RFC3339Nano)
		if handoff != nil {
			handoff.Handed, handoff.DeadlineTrust = obs, obs.UTCTrust
		}
		r.Attempts[attemptID] = &Attempt{Session: handoff, ID: attemptID, StepID: a.StepID, ActivationID: a.ID, Status: "pending", AdmissionID: admissionID, ReservationID: reservationID, AdmittedVersion: s.Version + 1, ControlEpoch: r.ControlEpoch, Envelope: envelopeBytes, EnvelopeDigest: rawDigest(envelopeBytes), Workspace: workspace, Context: manifest, Admitted: obs, Deadline: deadline, DispatchDeadline: dispatchDeadline}
		stepState.AttemptIDs = append(stepState.AttemptIDs, attemptID)
		// Attempts accumulate: another scope's running work is not this one's,
		// and replacing the set would silently drop an admitted attempt.
		r.Active = append(append([]string{}, r.Active...), attemptID)
		slices.Sort(r.Active)
		if err := r.setReadyFor(a.InvocationID, []string{}); err != nil {
			return local.Change{}, err
		}
		if err := r.setInvocationStatus(a.InvocationID, "running", nil); err != nil {
			return local.Change{}, err
		}
		change := local.Change{AcquireSlot: attemptID}
		if isInvocationState(r.SchemaVersion) {
			data, err := canonical(map[string]any{"attempt_id": attemptID, "step_instance_id": a.StepID, "stage_activation_id": a.ID, "workflow_invocation_id": a.InvocationID, "envelope_digest": rawDigest(envelopeBytes), "observation": obs})
			if err != nil {
				return local.Change{}, err
			}
			change.Events = []local.EventInput{{Type: "attempt.admitted", Version: 1, Data: data}}
		}
		return change, nil
	})
	if err == nil {
		treeAdmitted = true
	}
	return err
}

// No Attempt or execution slot exists yet. A binding/materialization failure
// can take a declared error path, but admission, budget and stop rejections
// cannot be caught here to bypass their guards.
func (e *Engine) failPreparation(ctx context.Context, loaded Run, view local.ReadView, p *flow.Plan, activation *Activation, cause error, fallback string) error {
	if loaded.Profile != flow.CoreProfile {
		return cause
	}
	if activation == nil {
		return local.ErrIntegrity
	}
	commandID, code := newID("command"), driverFailureCode(cause, fallback)
	if isContextState(loaded.SchemaVersion) {
		var problem *flow.Problem
		if errors.As(cause, &problem) {
			code = problem.Code
		}
	}
	_, err := e.apply(ctx, e.owner, commandID, loaded.ID, "stage.failed", map[string]any{"stage_activation_id": activation.ID, "failure": code, "execution": "not_admitted"}, &view.Snapshot.Version, local.CommandCAS, func(r *Run, s local.Snapshot, obs Observation) (local.Change, error) {
		a := r.Activations[activation.ID]
		if a == nil || a.Status != "ready" || r.admissionsBlockedFor(a.InvocationID) || r.cancelRequestedFor(a.InvocationID) || r.HasUnresolvedEffects || r.activeIn(a.InvocationID) != "" {
			return local.Change{}, local.Reject("stage_conflict", "preparation no longer owns a ready stage")
		}
		if r.PendingAcceptance != nil {
			if r.PendingAcceptance.ActivationID != a.ID || r.PendingAcceptance.Status != "passed" {
				return local.Change{}, local.Reject("acceptance_blocked", "another boundary owns preparation")
			}
			r.PendingAcceptance = nil
		}
		category := "executor"
		if a.Kind == "step" {
			step := r.Steps[a.StepID]
			if step == nil || len(step.AttemptIDs) != 0 {
				return local.Change{}, local.Reject("stage_conflict", "preparation failure cannot settle an admitted attempt")
			}
			step.Status, step.Settled = "failed", &obs
		} else if (a.Kind == "finish" || a.Kind == "call" || a.Kind == "repeat" || a.Kind == "wait" || fanOut(a.Kind)) && a.StepID == "" {
			category = "workflow"
		} else {
			return local.Change{}, local.Reject("stage_conflict", "unsupported preparation activation")
		}
		a.Status, a.Settled = "failed", &obs
		if err := r.failInvocation(a.InvocationID, obs); err != nil {
			return local.Change{}, err
		}
		if err := recordDiagnostic(r, Diagnostic{ID: derivedID("diagnostic", r.ID, commandID, "preparation", code), RunID: r.ID, ActivationID: a.ID, Origin: "core", Severity: "error", Code: code, Category: category, Phase: "preparation", Message: "Stage binding or preparation failed without an execution admission", Observed: obs, CauseRefs: []string{}}); err != nil {
			return local.Change{}, err
		}
		event, handled, err := routeKnownError(r, p, a.ID, "", code, obs)
		if err != nil {
			return local.Change{}, err
		}
		data, err := canonical(map[string]any{"stage_activation_id": a.ID, "failure": code, "execution": "not_admitted", "observation": obs})
		change := local.Change{Events: []local.EventInput{{Type: "stage.failed", Version: local.EventVersion, Data: data}}}
		if handled {
			change.Events = append(change.Events, event)
		}
		return change, err
	})
	return err
}

func (e *Engine) prepareWorkspace(r Run, ref flow.Ref, step flow.StepDefinition, name string, manifest ContextManifest, manifestBytes []byte) (string, error) {
	return e.prepareExecutorWorkspace(r.Executors[executorKey(r, ref, step.ID)], name, manifest, manifestBytes, nil)
}

func (e *Engine) prepareExecutorWorkspace(executor PinnedExecutor, name string, manifest ContextManifest, manifestBytes []byte, prepared map[ArtifactRef]Artifact) (string, error) {
	project, err := os.OpenRoot(e.Root)
	if err != nil {
		return "", err
	}
	defer project.Close()
	workRoot, err := project.OpenRoot(e.Config.Configuration.WorkspaceRoot)
	if err != nil {
		return "", err
	}
	defer workRoot.Close()
	if err := workRoot.Mkdir(name, 0700); err != nil {
		return "", err
	}
	work, err := workRoot.OpenRoot(name)
	if err != nil {
		return "", err
	}
	defer work.Close()
	for _, dir := range []string{"inputs", "outputs", "tmp"} {
		if err := work.Mkdir(dir, 0700); err != nil {
			return "", err
		}
	}
	workspace := filepath.Join(e.Root, e.Config.Configuration.WorkspaceRoot, name)
	materialized := []LocalPort{}
	if manifest.SchemaVersion == "local-context/2" {
		if manifest.Manifest == nil || manifest.Rendering == nil {
			return "", local.ErrIntegrity
		}
		if err := work.MkdirAll("context/sources", 0700); err != nil {
			return "", err
		}
		materialized = append(materialized, *manifest.Manifest, *manifest.Rendering)
		materialized = append(materialized, manifest.Sources...)
	} else {
		for _, port := range manifest.Inputs {
			materialized = append(materialized, port)
		}
	}
	for _, port := range materialized {
		a, _, err := e.contextArtifact(port.Ref, prepared)
		if err != nil {
			return "", err
		}
		if err := e.Blobs.Export(local.BlobRef{Digest: a.Digest, Size: a.SizeBytes}, workspace, port.Path); err != nil {
			return "", err
		}
	}
	for target, blob := range executor.Files {
		dir := filepath.Dir(target)
		if dir != "." {
			if err := work.MkdirAll(dir, 0700); err != nil {
				return "", err
			}
		}
		if err := e.Blobs.Export(blob, workspace, target); err != nil {
			return "", err
		}
	}
	if err := writeExclusive(filepath.Join(workspace, "context.json"), manifestBytes); err != nil {
		return "", err
	}
	return workspace, nil
}

func driverFailureCode(err error, fallback string) string {
	if err == nil {
		return fallback
	}
	var rejection *local.Rejection
	if errors.As(err, &rejection) {
		return rejection.Code
	}
	var problem *flow.Problem
	if errors.As(err, &problem) {
		return problem.Code
	}
	// A refusal raised with a stable code and no message is the same refusal.
	// Recorded under the generic failure instead, a diagnostic keeps the phase
	// and loses what the driver already knew went wrong.
	if code, _, _ := strings.Cut(leafError(err).Error(), ":"); validProblemCode(code) {
		return code
	}
	return fallback
}

// remainingBudget never reconstructs a monotonic deadline from a wall date.
// The unqualified wall clock can only shorten a same-session allowance; an
// apparent rollback is refused. Across sessions only explicitly trusted UTC
// bounds can authorize more execution. Millisecond observations allow 2ms of
// sampling/rounding disagreement, not a time-reset or suspend allowance.
func remainingBudget(admitted, deadline, now Observation) (time.Duration, error) {
	expired := func() (time.Duration, error) {
		return 0, local.Reject("attempt_deadline_expired", "admitted deadline has expired")
	}
	unqualified := func() (time.Duration, error) {
		return 0, local.Reject("deadline_clock_unqualified", "deadline cannot be compared in this clock domain")
	}
	startUTC, err := time.Parse(time.RFC3339Nano, admitted.UTC)
	if err != nil {
		return unqualified()
	}
	dueUTC, err := time.Parse(time.RFC3339Nano, deadline.UTC)
	if err != nil {
		return unqualified()
	}
	nowUTC, err := time.Parse(time.RFC3339Nano, now.UTC)
	if err != nil {
		return unqualified()
	}
	if !nowUTC.Before(dueUTC) {
		return expired()
	}
	wall := dueUTC.Sub(nowUTC)
	if nowUTC.Before(startUTC.Add(-2*time.Millisecond)) || wall > time.Hour {
		return unqualified()
	}
	sameClock := func(a, b Observation) bool {
		return a.Session != "" && a.Session == b.Session && a.Source != "" && a.Source == b.Source &&
			slices.Contains([]string{"includes_suspend", "excludes_suspend", "excludes_suspend_on_darwin"}, a.SuspendBasis) && a.SuspendBasis == b.SuspendBasis
	}
	if !sameClock(admitted, deadline) || !sameClock(deadline, now) {
		if admitted.UTCTrust != "trusted" || deadline.UTCTrust != "trusted" || now.UTCTrust != "trusted" {
			return unqualified()
		}
		return wall, nil
	}
	if now.MonotonicMS < admitted.MonotonicMS || admitted.MonotonicMS < 0 {
		return unqualified()
	}
	if deadline.MonotonicMS <= now.MonotonicMS {
		return expired()
	}
	ms := deadline.MonotonicMS - now.MonotonicMS
	if ms > int64(time.Hour/time.Millisecond) {
		return unqualified()
	}
	monotonic := time.Duration(ms) * time.Millisecond
	if wall > monotonic+2*time.Millisecond {
		return 0, local.Reject("deadline_clock_rollback", "wall clock moved backwards relative to the admitted clock")
	}
	return min(monotonic, wall), nil
}

func verifyWorkspace(a *Attempt, executor PinnedExecutor) error {
	if rawDigest(a.Envelope) != a.EnvelopeDigest || flow.ValidateProtocol("ExecutionEnvelope", a.Envelope) != nil {
		return local.Reject("envelope_drift", "admitted envelope no longer validates")
	}
	// A subset decoder reads already schema-validated v1 fields; it adds no
	// fields or alternate worker wire contract.
	var envelope struct {
		Context          ArtifactRef            `json:"context_manifest_ref"`
		Deadline         string                 `json:"attempt_deadline"`
		DispatchDeadline string                 `json:"dispatch_not_after"`
		Inputs           map[string]ArtifactRef `json:"input_artifacts"`
	}
	if err := json.Unmarshal(a.Envelope, &envelope); err != nil || envelope.Deadline != a.Deadline.UTC || envelope.DispatchDeadline != a.DispatchDeadline.UTC {
		return local.Reject("envelope_drift", "admitted deadline fields differ")
	}
	manifest, err := canonical(a.Context)
	if err != nil {
		return local.Reject("context_manifest_drift", "admitted context cannot be encoded")
	}
	materialized, err := readLocal(a.Workspace, "context.json", MaxDefinitionBytes)
	if err != nil || !bytes.Equal(materialized, manifest) {
		return local.Reject("context_manifest_drift", "materialized context differs from the admitted manifest")
	}
	if a.Context.SchemaVersion == "local-context/2" {
		if err := verifyFullWorkspaceContext(a.Workspace, a.Context, envelope.Context, envelope.Inputs, executor.ContextProfile); err != nil {
			return err
		}
	} else if a.Context.SchemaVersion != "local-context/1" || executor.ContextProfile != nil || rawDigest(materialized) != envelope.Context.Digest {
		return local.Reject("context_manifest_drift", "local context transport differs from its pinned adapter contract")
	}
	for _, port := range a.Context.Inputs {
		data, err := readLocal(a.Workspace, port.Path, MaxArtifactBytes)
		if err != nil || rawDigest(data) != port.Ref.Digest {
			return local.Reject("workspace_input_drift", "materialized input differs from admitted bytes")
		}
	}
	for path, blob := range executor.Files {
		data, err := readLocal(a.Workspace, path, MaxArtifactBytes)
		if err != nil || rawDigest(data) != blob.Digest || int64(len(data)) != blob.Size {
			return local.Reject("workspace_script_drift", "materialized executor file differs from pinned bytes")
		}
	}
	return nil
}

func (e *Engine) executePending(ctx context.Context, r Run, v local.ReadView, a *Attempt, socket string) error {
	// Only this foreground owner can assert no spawn on these paths. Recovery
	// never calls this function after a persisted dispatch boundary.
	tokenHash := ""
	failBeforeStart := func(code string) error {
		if ctx.Err() != nil {
			if err := e.requestDriverCancellation(r.ID); err != nil {
				return err
			}
		}
		settleCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return e.settleUnstarted(settleCtx, r.ID, a.ID, tokenHash, code)
	}
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return failBeforeStart("launch_identity_unavailable")
	}
	token := hex.EncodeToString(nonce[:])
	tokenHash = rawDigest([]byte(token))
	activation := r.Activations[a.ActivationID]
	if activation == nil {
		return failBeforeStart("activation_missing")
	}
	p, err := r.planFor(activation.InvocationID)
	if err != nil {
		return failBeforeStart("pinned_plan_invalid")
	}
	step := p.Steps[activation.StageID]
	executor := r.Executors[executorKey(r, p.Workflow.Definition.Stages[activation.StageID].StepRef, step.ID)]
	// Verify all materialized bytes immediately before the durable dispatch boundary.
	if err := verifyWorkspace(a, executor); err != nil {
		return failBeforeStart(driverFailureCode(err, "workspace_validation_failed"))
	}
	var remaining time.Duration
	_, err = e.apply(ctx, e.owner, newID("command"), r.ID, "attempt.dispatching", map[string]any{"attempt_id": a.ID}, &v.Snapshot.Version, local.CommandCAS, func(r *Run, s local.Snapshot, obs Observation) (local.Change, error) {
		current := r.Attempts[a.ID]
		if current == nil || current.Dispatch != nil || current.Status != "pending" || r.admissionsBlockedFor(activation.InvocationID) || r.cancelRequestedFor(activation.InvocationID) {
			return local.Change{}, local.Reject("dispatch_blocked", "attempt changed or is restricted")
		}
		if _, err := remainingBudget(current.Admitted, current.DispatchDeadline, obs); err != nil {
			return local.Change{}, err
		}
		var err error
		remaining, err = remainingBudget(current.Admitted, current.Deadline, obs)
		if err != nil {
			return local.Change{}, err
		}
		current.Dispatch = &obs
		current.Status = "dispatching"
		current.TokenHash = tokenHash
		data, err := canonical(map[string]any{"observation": obs, "attempt_id": current.ID, "admission_control_epoch": current.ControlEpoch, "dispatch_control_epoch": r.ControlEpoch})
		return local.Change{Events: []local.EventInput{{Type: "attempt.dispatching", Version: 1, Data: data}}}, err
	})
	if err != nil {
		// A concurrent pause may leave an undispatched admission waiting. It
		// must not become a worker failure solely because run CAS advanced.
		current, _, readErr := e.load(context.Background(), r.ID)
		if readErr != nil {
			return errors.Join(err, readErr)
		}
		pending := current.Attempts[a.ID]
		if pending != nil && pending.Dispatch == nil && current.admissionsBlockedFor(activation.InvocationID) && !current.cancelRequestedFor(activation.InvocationID) && ctx.Err() == nil {
			return nil
		}
		return failBeforeStart(driverFailureCode(err, "dispatch_failed"))
	}
	remaining, err = remainingBudget(a.Admitted, a.Deadline, e.clock.now())
	if err != nil {
		return failBeforeStart(driverFailureCode(err, "deadline_unavailable"))
	}
	// This timer starts before runner hashing/setup. RunProcess's own runtime
	// limit is an additional bound, never a fresh full configured timeout.
	processCtx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	watchDone := make(chan struct{})
	watchExited := make(chan struct{})
	var watchErr error
	go func() {
		defer close(watchExited)
		deadline := time.Now().Add(remaining)
		timer := time.NewTimer(remaining)
		defer timer.Stop()
		ticker := time.NewTicker(watchInterval)
		defer ticker.Stop()
		version, sequence := int64(0), int64(0)
		for {
			select {
			case <-watchDone:
				return
			case <-timer.C:
				cancel(local.Reject("attempt_deadline_expired", "admitted attempt deadline expired"))
				return
			case <-ctx.Done():
				// Stop is durable before cancellation is reported as settled.
				// Signal promptly even if the control database is unavailable.
				cancel(ctx.Err())
				watchErr = e.requestDriverCancellation(r.ID)
				return
			case <-ticker.C:
				budget, err := remainingBudget(a.Admitted, a.Deadline, e.clock.now())
				if err != nil {
					cancel(err)
					return
				}
				// A wall-clock advance (including sleep on Darwin) may shorten
				// the bound. A later rollback never extends the tightened timer.
				if next := time.Now().Add(budget); next.Before(deadline) {
					deadline = next
					timer.Reset(time.Until(deadline))
				}
				// Ask what the stored state is before reading it: an idle wait
				// then costs one indexed row, not a full state decode.
				readCtx, readCancel := context.WithTimeout(context.Background(), time.Second)
				currentVersion, currentSequence, err := e.Store.Revision(readCtx, r.ID)
				if err == nil && (currentVersion != version || currentSequence != sequence) {
					version, sequence = currentVersion, currentSequence
					var current Run
					current, _, err = e.load(readCtx, r.ID)
					if err == nil && current.cancelRequestedFor(activation.InvocationID) {
						readCancel()
						cancel(context.Canceled)
						return
					}
				}
				readCancel()
				if err != nil {
					cancel(local.Reject("control_observation_failed", "cannot observe current cancellation state"))
					return
				}
			}
		}
	}()
	env := map[string]string{"PATH": "/usr/bin:/bin", "LANG": "C.UTF-8", "TMPDIR": filepath.Join(a.Workspace, "tmp"), "PRIFLY_SOCKET": socket, "PRIFLY_TOKEN": token, "PRIFLY_CONTEXT_FILE": filepath.Join(a.Workspace, "context.json"), "PRIFLY_RUN_ID": r.ID, "PRIFLY_STEP_ID": a.StepID, "PRIFLY_ATTEMPT_ID": a.ID}
	for name, value := range executor.Config.Environment {
		env[name] = value
	}
	outcome, runErr := local.RunProcess(processCtx, local.ProcessSpec{Executable: executor.Config.Executable, ExecutableDigest: executor.ExecutableDigest, Args: executor.Config.Args, Dir: a.Workspace, Env: env, Envelope: a.Envelope, MaxRuntime: remaining, GracePeriod: time.Duration(executor.Config.GraceMS) * time.Millisecond, KillWait: 2 * time.Second, MaxStdoutBytes: 64 << 10, MaxStderrBytes: 64 << 10, MaxResultBytes: 1 << 20, BeforeStart: func() error {
		readCtx, readCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer readCancel()
		current, _, err := e.load(readCtx, r.ID)
		if err != nil {
			return err
		}
		attempt := current.Attempts[a.ID]
		if attempt == nil || attempt.Dispatch == nil || attempt.Settled != nil || attempt.Started != nil || attempt.TokenHash != tokenHash || !slices.Contains(current.Active, a.ID) {
			return local.Reject("dispatch_ownership_lost", "attempt no longer belongs to this launch")
		}
		if current.cancelRequestedFor(activation.InvocationID) {
			return local.Reject("cancelled", "cancellation committed before process start")
		}
		// Pause/epoch changes after dispatch do not revoke this admitted work.
		now := e.clock.now()
		if _, err := remainingBudget(attempt.Admitted, attempt.DispatchDeadline, now); err != nil {
			return err
		}
		_, err = remainingBudget(attempt.Admitted, attempt.Deadline, now)
		return err
	}}, func(observation local.ProcessObservation) error {
		observeCtx, observeCancel := context.WithTimeout(context.Background(), observationDeadline)
		defer observeCancel()
		return e.observe(observeCtx, r.ID, a.ID, observation)
	})
	close(watchDone)
	<-watchExited
	if ctx.Err() != nil && watchErr == nil {
		watchErr = e.requestDriverCancellation(r.ID)
	}
	if cause := context.Cause(processCtx); cause != nil && (outcome.StopReason == "" || outcome.StopReason == "cancelled") {
		outcome.StopReason = driverFailureCode(cause, "cancelled")
	}
	if !outcome.Started {
		code := driverFailureCode(runErr, "executor_start_failed")
		if outcome.StopReason != "" {
			code = outcome.StopReason
		}
		return errors.Join(watchErr, failBeforeStart(code))
	}
	settleCtx, settleCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer settleCancel()
	return errors.Join(watchErr, e.settle(settleCtx, r.ID, a.ID, outcome, runErr))
}

// dispatchWindow bounds how long an admitted attempt may sit between its
// dispatch record and its actual launch. It is not the step's own timeout: it
// only says how long a launch may take to begin.
const dispatchWindow = 30 * time.Second

// observationDeadline bounds recording one process observation. It is longer
// than the store's busy timeout on purpose: a concurrent writer must be waited
// out and retried, because a dropped observation costs the worker its evidence.
const observationDeadline = 15 * time.Second

// retryOnBusy repeats a write that SQLite refused because another writer held
// the database. The observation it carries has nowhere else to go: dropping it
// turns a live worker into an unexplained failure, so it is retried for as long
// as the caller's own context allows.
func retryOnBusy(ctx context.Context, write func() error) error {
	const busyRetryInterval = 50 * time.Millisecond
	for {
		err := write()
		if !local.IsBusy(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(busyRetryInterval):
		}
	}
}

func (e *Engine) observe(ctx context.Context, runID, attemptID string, o local.ProcessObservation) error {
	return retryOnBusy(ctx, func() error { return e.observeOnce(ctx, runID, attemptID, o) })
}

func (e *Engine) observeOnce(ctx context.Context, runID, attemptID string, o local.ProcessObservation) error {
	if o.Kind == "result_candidate" {
		return e.observeResult(ctx, runID, attemptID, o)
	}
	eventType := "attempt.observed"
	if o.Kind == "start_returned" {
		eventType = "attempt.started"
	}
	_, err := e.apply(ctx, "runner:"+strings.TrimPrefix(attemptID, "attempt:"), newID("command"), runID, eventType, map[string]any{"attempt_id": attemptID, "kind": o.Kind}, nil, local.CommandGuarded, func(r *Run, s local.Snapshot, obs Observation) (local.Change, error) {
		a := r.Attempts[attemptID]
		if a == nil || a.Dispatch == nil || a.Settled != nil || r.HasUnresolvedEffects || !slices.Contains(r.Active, attemptID) {
			return local.Change{}, local.Reject("stale_attempt", "observation does not own active attempt")
		}
		switch o.Kind {
		case "start_returned":
			if a.Started != nil {
				return local.Change{}, local.Reject("duplicate_start", "attempt already has a started process")
			}
			a.Started = &obs
			a.Process = &o.Identity
			a.Status = "running"
			r.Steps[a.StepID].Status = "running"
			r.Activations[a.ActivationID].Status = "running"
		case "group_empty":
			a.ExecutorEnd = &obs
			a.Status = "verifying"
			r.Steps[a.StepID].Status = "verifying"
			r.Activations[a.ActivationID].Status = "verifying"
		case "term_sent", "kill_sent":
			a.Status = "stopping"
			r.Steps[a.StepID].Status = "stopping"
			r.Activations[a.ActivationID].Status = "stopping"
		}
		data, _ := canonical(map[string]any{"observation": obs, "process_observation": o, "attempt_id": attemptID})
		return local.Change{Events: []local.EventInput{{Type: eventType, Version: 1, Data: data}}}, nil
	})
	return err
}

// watchInterval is how often a running attempt or check asks whether its Run
// changed. It is a control latency, not a deadline: the deadline has its own
// timer, and asking more often only re-read state nobody had written.
const watchInterval = 250 * time.Millisecond

const maxLateResultsPerRun = 32
const maxLateResultsPerAttempt = 4
const maxResultEvidenceBytes = 1 << 20

func resultIntakeOwner(r *Run, attemptID string, result Result) (*Attempt, error) {
	a := r.Attempts[attemptID]
	if a == nil || a.Dispatch == nil {
		return nil, local.Reject("stale_attempt", "result has no dispatched attempt")
	}
	if result.RunID != r.ID || result.AttemptID != attemptID || result.StepInstanceID != a.StepID || result.EnvelopeDigest != a.EnvelopeDigest {
		return nil, local.Reject("result_identity_mismatch", "candidate differs from the admitted attempt")
	}
	return a, nil
}

func resultDisposition(r *Run, a *Attempt, candidate []byte) (string, bool) {
	active := a.Settled == nil && !r.terminal() && !r.HasUnresolvedEffects && slices.Contains(r.Active, a.ID)
	if len(a.Candidate) > 0 && !bytes.Equal(a.Candidate, candidate) {
		return "conflicting", active
	}
	if !active {
		return "late", false
	}
	return "candidate", true
}

func resultEvidenceQuota(r *Run, attemptID string) error {
	total, own := 0, 0
	for _, d := range r.Diagnostics {
		if d.Phase == "result_intake" && (d.Code == "late_result" || d.Code == "conflicting_result") {
			total++
			if d.AttemptID == attemptID {
				own++
			}
		}
	}
	if total >= maxLateResultsPerRun || own >= maxLateResultsPerAttempt {
		return local.Reject("result_evidence_limit", "late/conflicting result evidence allowance is exhausted")
	}
	return nil
}

// A result receipt acknowledges intake, never semantic acceptance. Its identity
// is independent of delivery time so an exact retry remains a retry after the
// attempt settles. Raw bodies are restricted artifacts, not ordinary read DTOs.
func (e *Engine) observeResult(ctx context.Context, runID, attemptID string, o local.ProcessObservation) error {
	candidate, err := flow.Canonical(o.Result)
	if err != nil {
		return err
	}
	if len(candidate) > maxResultEvidenceBytes {
		return local.Reject("result_payload_limit", "result evidence exceeds the local byte allowance")
	}
	if err := flow.ValidateProtocol("StepResult", candidate); err != nil {
		return err
	}
	var parsed Result
	if err := decode(candidate, &parsed); err != nil {
		return err
	}
	r, _, err := e.load(ctx, runID)
	if err != nil {
		return err
	}
	a, err := resultIntakeOwner(&r, attemptID, parsed)
	if err != nil {
		return err
	}
	digest := rawDigest(candidate)
	commandID := derivedID("command", "result-intake", runID, attemptID, digest)
	actor := "runner:" + strings.TrimPrefix(attemptID, "attempt:")
	payload := map[string]any{"attempt_id": attemptID, "kind": "result_candidate", "candidate_digest": digest}
	var evidence ArtifactRef
	// Current ownership is checked above even for retained receipts. A retry
	// neither reseals bytes nor needs the mutable source or an intact old workspace.
	_, lookupErr := e.Store.LookupReceipt(ctx, actor, commandID)
	if errors.Is(lookupErr, local.ErrNotFound) {
		if disposition, _ := resultDisposition(&r, a, candidate); disposition != "candidate" {
			if err := resultEvidenceQuota(&r, attemptID); err != nil {
				return err
			}
		}
		resultSchema := builtinRef(r.Definitions, "core:schema/step-result")
		artifact, err := e.putArtifact(candidate, "json", &resultSchema, derivedID("artifact", "result-intake", runID, attemptID, digest), map[string]any{"kind": "authority", "authority_id": r.AuthorityID, "command_id": commandID, "port": "result_intake"}, nil, r.registry())
		if err != nil {
			return err
		}
		evidence = artifact.Ref()
	} else if lookupErr != nil {
		return lookupErr
	}
	_, err = e.apply(ctx, actor, commandID, runID, "attempt.result_candidate", payload, nil, local.CommandGuarded, func(r *Run, _ local.Snapshot, obs Observation) (local.Change, error) {
		a, err := resultIntakeOwner(r, attemptID, parsed)
		if err != nil {
			return local.Change{}, err
		}
		if evidence.ArtifactID == "" {
			return local.Change{}, local.ErrIntegrity // A retained receipt cannot disappear in F1.
		}
		disposition, active := resultDisposition(r, a, candidate)
		diagnosticIDs := []string{}
		if disposition != "candidate" {
			if err := resultEvidenceQuota(r, attemptID); err != nil {
				return local.Change{}, err
			}
			code := "late_result"
			if disposition == "conflicting" {
				code = "conflicting_result"
			}
			diagnosticID := derivedID("diagnostic", commandID)
			if err := recordDiagnostic(r, Diagnostic{ID: diagnosticID, RunID: r.ID, AttemptID: attemptID, Origin: "core", Severity: "warn", Code: code, Category: "executor", Phase: "result_intake", Message: "Result evidence was retained without replacing an accepted result", Observed: obs, CauseRefs: []string{evidence.ArtifactID}}); err != nil {
				return local.Change{}, err
			}
			diagnosticIDs = append(diagnosticIDs, diagnosticID)
		}
		if active {
			if disposition == "conflicting" {
				a.CandidateConflict = true
			} else if len(a.Candidate) == 0 {
				a.Candidate, a.CandidateAt = candidate, &obs
			}
		}
		data, err := canonical(map[string]any{"observation": obs, "process_observation": o, "attempt_id": attemptID, "candidate_digest": digest, "evidence_ref": evidence, "disposition": disposition})
		if err != nil {
			return local.Change{}, err
		}
		ack, err := canonical(map[string]any{"run_id": r.ID, "attempt_id": attemptID, "candidate_digest": digest, "evidence_ref": evidence, "disposition": disposition, "diagnostic_ids": diagnosticIDs})
		if err != nil {
			return local.Change{}, err
		}
		return local.Change{Events: []local.EventInput{{Type: "attempt.result_candidate", Version: 1, Data: data}}, Result: ack, RequireStorageBudget: disposition != "candidate"}, nil
	})
	return err
}

func (e *Engine) acceptedOutputs(r Run, a *Attempt, step flow.StepDefinition, p *flow.Plan, result Result) error {
	_, err := e.resultOutputs(r, a, step, p, result, false)
	return err
}

// outputProblem names the exact port an intake refusal is about, so a host
// corrects the report it sent instead of guessing which port was wrong.
func outputProblem(code, port, message string) error {
	return &flow.Problem{Code: code, Path: "/result/outputs/" + port, Message: message}
}

// readResultOutputs is the side-effect-free half of output intake: it checks
// the reported ports against the step and the admitted slots and returns the
// slot bytes, sealing nothing. Assisted intake runs it before a candidate is
// recorded, so a report that acceptance would reject is refused while its
// handoff is still awaiting instead of burning the attempt.
func (e *Engine) readResultOutputs(r Run, a *Attempt, step flow.StepDefinition, p *flow.Plan, result Result) (map[string][]byte, error) {
	if len(result.EvidenceRefs) > 0 || len(result.EffectReceiptRefs) > 0 {
		return nil, fault("unsupported_evidence", "local output checks do not trust worker-supplied external receipts")
	}
	for port, definition := range step.Outputs {
		if slices.Contains(definition.RequiredFor, result.Verdict) {
			if _, ok := result.Outputs[port]; !ok {
				return nil, outputProblem("output_required_missing", port, "this output port is required for the reported verdict and was not reported")
			}
		}
	}
	activation := r.Activations[a.ActivationID]
	if activation == nil {
		return nil, local.ErrIntegrity
	}
	contents := map[string][]byte{}
	for port, ref := range result.Outputs {
		definition, ok := step.Outputs[port]
		if !ok {
			return nil, outputProblem("output_port_undeclared", port, "this step declares no such output port")
		}
		slot := a.Context.Outputs[port]
		if ref.ArtifactID != slot.ArtifactID || ref.Revision != slot.Revision {
			return nil, outputProblem("output_identity_mismatch", port, "the reported artifact identity differs from the admitted output slot")
		}
		limit := r.Executors[executorKey(r, p.Workflow.Definition.Stages[activation.StageID].StepRef, step.ID)].Config.MaxOutputBytes
		if limit == 0 && a.Session != nil {
			limit = MaxArtifactBytes
		}
		content, err := readLocal(a.Workspace, slot.Path, limit)
		if errors.Is(err, os.ErrNotExist) {
			return nil, outputProblem("output_slot_empty", port, "the admitted output slot holds no file")
		}
		if err != nil {
			return nil, err
		}
		if rawDigest(content) != ref.Digest {
			return nil, outputProblem("output_digest_mismatch", port, "the bytes in the admitted output slot do not match the reported digest")
		}
		if definition.Format == "json" {
			if err := p.ValidateJSON(*definition.SchemaRef, content); err != nil {
				return nil, err
			}
		}
		contents[port] = content
	}
	return contents, nil
}

func (e *Engine) resultOutputs(r Run, a *Attempt, step flow.StepDefinition, p *flow.Plan, result Result, deferred bool) (map[string]Artifact, error) {
	contents, err := e.readResultOutputs(r, a, step, p, result)
	if err != nil {
		return nil, err
	}
	activation := r.Activations[a.ActivationID]
	if activation == nil {
		return nil, local.ErrIntegrity
	}
	outputs := map[string]Artifact{}
	for port, ref := range result.Outputs {
		definition := step.Outputs[port]
		slot := a.Context.Outputs[port]
		data := contents[port]
		if workspaceTreeBinding(step, port) != nil {
			if deferred {
				return nil, fault("workspace_tree_deferred_acceptance_unsupported", "")
			}
			artifact, err := e.sealWorkspaceTreeOutput(r, a, step, definition, port, ref, data)
			if err != nil {
				return nil, err
			}
			if err := e.validatePortArtifact(p, definition.Port, artifact, data); err != nil {
				return nil, err
			}
			outputs[port] = artifact
			continue
		}
		producer := map[string]any{"kind": "step", "run_id": r.ID, "workflow_invocation_id": activation.InvocationID, "stage_activation_id": a.ActivationID, "step_instance_id": a.StepID, "attempt_id": a.ID, "port": port}
		var artifact Artifact
		if deferred {
			artifact, err = e.prepareArtifact(data, definition.Format, definition.SchemaRef, slot.ArtifactID, producer, nil, r.registry(), portMedia(definition.Port))
		} else {
			artifact, err = e.putArtifact(data, definition.Format, definition.SchemaRef, slot.ArtifactID, producer, nil, r.registry(), portMedia(definition.Port))
		}
		if err != nil {
			return nil, err
		}
		if artifact.Ref() != ref {
			return nil, outputProblem("output_seal_mismatch", port, "the sealed artifact identity differs from the reported one")
		}
		if err := e.validatePortArtifact(p, definition.Port, artifact, data); err != nil {
			return nil, err
		}
		outputs[port] = artifact
	}
	return outputs, nil
}

// settlementEvidence names what a settlement rests on. A managed attempt rests
// on observed process facts; an assisted attempt rests on the host's own
// report. Nothing may substitute one for the other: a fabricated ProcessOutcome
// would record an observation of a process that never existed.
type settlementEvidence struct {
	Kind      string
	Outcome   *local.ProcessOutcome
	Actor     string
	CommandID string
	// Failure names a settlement whose verdict is already known without
	// reading the attempt: recovery closes what it can prove, and does not
	// judge a saved result candidate it never owned.
	Failure string
}

func (e *Engine) settle(ctx context.Context, runID, attemptID string, outcome local.ProcessOutcome, runErr error) error {
	actor := "runner:" + strings.TrimPrefix(attemptID, "attempt:")
	return e.settleWith(ctx, runID, attemptID, settlementEvidence{Kind: "process", Outcome: &outcome, Actor: actor, CommandID: newID("command")}, runErr)
}

func (e *Engine) settleWith(ctx context.Context, runID, attemptID string, evidence settlementEvidence, runErr error) error {
	r, _, err := e.load(ctx, runID)
	if err != nil {
		return err
	}
	a := r.Attempts[attemptID]
	if a == nil {
		return errors.New("attempt missing")
	}
	activation := r.Activations[a.ActivationID]
	if activation == nil {
		return errors.New("attempt activation missing")
	}
	p, planErr := r.planFor(activation.InvocationID)
	var accepted *Result
	var pending *PendingAcceptance
	failure := evidence.Failure
	detail := ""
	outcome := evidence.Outcome
	uncertain := outcome != nil && (outcome.Uncertain || (outcome.Started && (!outcome.WaitReturned || !outcome.GroupEmpty)))
	if failure != "" {
		// The caller already read the evidence and named the failure.
	} else if uncertain {
		failure = "executor_settlement_unknown"
	} else if r.cancelRequestedFor(activation.InvocationID) {
		failure = "cancelled"
	} else if outcome != nil && !outcome.Started {
		failure = "executor_start_failed"
	} else if persistenceFailure(runErr) {
		// The authority could not record what it observed. That is this
		// authority failing, not a verdict about the worker, so no declared
		// error route answers it.
		failure = "authority_persistence_failed"
	} else if runErr != nil {
		failure = "executor_observation_failed"
	} else if outcome != nil && outcome.StopReason != "" {
		failure = outcome.StopReason
	} else if outcome != nil && (outcome.ExitCode == nil || *outcome.ExitCode != 0) {
		failure = "nonzero_exit"
	} else if outcome != nil && outcome.ResultError != "" || a.CandidateConflict {
		failure = "invalid_result"
	} else if len(a.Candidate) == 0 {
		failure = "missing_result"
	} else if planErr != nil {
		failure = "pinned_plan_invalid"
	} else {
		step := p.Steps[activation.StageID]
		var value Result
		if err := decode(a.Candidate, &value); err != nil {
			failure, detail = "invalid_result", err.Error()
		} else if err := p.ValidateJSON(step.ResultSchemaRef, a.Candidate); err != nil {
			failure, detail = "result_schema_invalid", err.Error()
		} else {
			deferred := isContextState(r.SchemaVersion) && hasStepAcceptanceChecks(step)
			outputs, err := e.resultOutputs(r, a, step, p, value, deferred)
			if err != nil {
				failure, detail = sealingClass(err, "invalid_output"), err.Error()
			} else if deferred {
				digest := rawDigest(a.Candidate)
				candidate := ArtifactRef{ArtifactID: derivedID("artifact", "result-intake", r.ID, a.ID, digest), Revision: 1, Digest: digest}
				if _, _, err = e.Artifact(candidate); err == nil {
					pending, err = newPendingAcceptance(r, p, "step_result", activation.InvocationID, activation.ID, a.ID, value.Outputs, outputs, &candidate, e.clock.now())
				}
				if err != nil {
					failure, detail = sealingClass(err, "result_evidence_invalid"), err.Error()
				} else if pending == nil {
					// All checked output ports were optional and absent. No check
					// is invented, but any other prepared outputs still publish.
					for _, output := range outputs {
						if _, err = e.publishPreparedArtifact(output, r.registry()); err != nil {
							failure, detail = sealingClass(err, "invalid_output"), err.Error()
							break
						}
					}
					if failure == "" {
						accepted = &value
					}
				}
			} else {
				accepted = &value
			}
		}
	}
	// The transform checks the current owning attempt. Concurrent stops and
	// publications are new facts, not stale run-CAS failures that kill a worker.
	commandID := evidence.CommandID
	payload := map[string]any{"attempt_id": attemptID, "evidence": evidence.Kind, "candidate_digest": rawDigest(a.Candidate), "accepted": accepted != nil, "failure": failure}
	if outcome != nil {
		payload["outcome"] = *outcome
	}
	_, err = e.apply(ctx, evidence.Actor, commandID, runID, "attempt.settled", payload, nil, local.CommandGuarded, func(r *Run, s local.Snapshot, obs Observation) (local.Change, error) {
		current := r.Attempts[attemptID]
		if current == nil || current.Settled != nil || !slices.Contains(r.Active, attemptID) {
			return local.Change{}, local.Reject("stale_attempt", "result belongs to another or settled attempt")
		}
		// An assisted attempt records no process facts and no executor end: the
		// host reported, which is not proof that its session stopped working.
		if outcome != nil {
			current.ProcessOutcome = outcome
			if outcome.Started && outcome.GroupEmpty && current.ExecutorEnd == nil {
				current.ExecutorEnd = &obs
			}
		}
		if uncertain {
			current.Status = "uncertain"
			r.Steps[current.StepID].Status = "uncertain"
			r.Activations[current.ActivationID].Status = "uncertain"
			if err := r.setInvocationStatus(activation.InvocationID, "uncertain", nil); err != nil {
				return local.Change{}, err
			}
			r.HasUnresolvedEffects = true
			if err := diagnostic(r, commandID, attemptID, failure, "settlement", "Executor settlement is not proven", obs); err != nil {
				return local.Change{}, err
			}
			return local.Change{}, nil
		}
		if (accepted != nil || pending != nil) && (current.CandidateConflict || !bytes.Equal(current.Candidate, a.Candidate)) {
			accepted = nil
			pending = nil
			failure = "result_candidate_changed"
		}
		current.Settled = &obs
		removeActive(r, attemptID)
		stepState := r.Steps[current.StepID]
		stageState := r.Activations[current.ActivationID]
		if pending != nil && !r.cancelRequestedFor(activation.InvocationID) {
			current.Status = "completed" // Proven process success, not result acceptance.
			if err := r.holdAcceptance(pending, obs); err != nil {
				return local.Change{}, err
			}
			// Proven process settlement consumes the reserved storage allowance.
			// A soft quota may reject the next check, never discard this proof.
			return local.Change{ReleaseSlot: attemptID}, nil
		}
		stepState.Settled = &obs
		stageState.Settled = &obs
		change := local.Change{ReleaseSlot: attemptID}
		if r.cancelRequestedFor(activation.InvocationID) {
			current.Status = "cancelled"
			stepState.Status = "cancelled"
			stageState.Status = "cancelled"
			if isInvocationState(r.SchemaVersion) {
				if err := r.setInvocationStatus(activation.InvocationID, "stopping", nil); err != nil {
					return local.Change{}, err
				}
			} else {
				r.Status, r.Settled = "cancelled", &obs
			}
		} else if accepted != nil {
			current.Accepted = accepted
			current.Status = "completed"
			stepState.Status = "completed"
			stageState.Status = "completed"
			stepState.Verdict = accepted.Verdict
			stepState.Outputs = accepted.Outputs
			next, routeErr := p.Next(activation.StageID, accepted.Verdict)
			if routeErr != nil {
				if err := r.failInvocation(activation.InvocationID, obs); err != nil {
					return local.Change{}, err
				}
				if err := diagnostic(r, commandID, attemptID, "unhandled_verdict", "routing", "Accepted verdict has no declared handler", obs); err != nil {
					return local.Change{}, err
				}
			} else {
				if err := r.advanceInvocation(activation.InvocationID, next); err != nil {
					return local.Change{}, err
				}
			}
		} else {
			current.Status = "failed"
			stepState.Status = "failed"
			stageState.Status = "failed"
			if err := r.failInvocation(activation.InvocationID, obs); err != nil {
				return local.Change{}, err
			}
			if err := diagnosticDetail(r, commandID, attemptID, failure, "settlement", "Executor or result validation failed; inspect recorded evidence", detail, obs); err != nil {
				return local.Change{}, err
			}
			if planErr == nil {
				event, handled, err := routeKnownError(r, p, current.ActivationID, attemptID, failure, obs)
				if err != nil {
					return local.Change{}, err
				}
				if handled {
					data, _ := canonical(map[string]any{"observation": obs, "attempt_id": attemptID, "status": current.Status, "failure": failure})
					change.Events = []local.EventInput{{Type: "attempt.settled", Version: local.EventVersion, Data: data}, event}
				}
			}
		}
		return change, nil
	})
	return err
}

// settleRecoveredExecutorEnd closes an attempt whose process group was already
// observed empty and journalled before this driver lost the run. That end is
// saved evidence, not a guess, so the attempt fails and its slot is released
// instead of being held for an owner resolution. The saved result candidate is
// not accepted here: judging it belongs to the driver that owned the dispatch.
func (e *Engine) settleRecoveredExecutorEnd(ctx context.Context, runID, attemptID string) error {
	evidence := settlementEvidence{
		Kind:      "recovered_executor_end",
		Actor:     e.owner,
		CommandID: derivedID("command", attemptID, "settlement-lost"),
		Failure:   "executor_settlement_lost",
	}
	return e.settleWith(ctx, runID, attemptID, evidence, nil)
}

// settleUnstarted is only for a launch this foreground owner knows never
// reached cmd.Start. A recovered dispatch cannot supply that knowledge merely
// by reading the persisted token hash or finding its old PID absent.
func (e *Engine) settleUnstarted(ctx context.Context, runID, attemptID, tokenHash, code string) error {
	loaded, _, err := e.load(ctx, runID)
	if err != nil {
		return err
	}
	prior := loaded.Attempts[attemptID]
	if prior == nil || loaded.Activations[prior.ActivationID] == nil {
		return local.ErrIntegrity
	}
	invocationID := loaded.Activations[prior.ActivationID].InvocationID
	p, planErr := loaded.planFor(invocationID)
	commandID := newID("command")
	_, err = e.apply(ctx, e.owner, commandID, runID, "attempt.settled", map[string]any{"attempt_id": attemptID, "spawn_observation": "known_not_started", "failure": code}, nil, local.CommandGuarded, func(r *Run, s local.Snapshot, obs Observation) (local.Change, error) {
		a := r.Attempts[attemptID]
		if a == nil || a.Settled != nil || a.Started != nil || a.Process != nil || r.HasUnresolvedEffects || !slices.Contains(r.Active, attemptID) {
			return local.Change{}, local.Reject("unstarted_conflict", "attempt is not a known unstarted admission")
		}
		if a.Dispatch != nil && (tokenHash == "" || a.TokenHash != tokenHash) {
			return local.Change{}, local.Reject("dispatch_conflict", "no local no-spawn evidence for this dispatch")
		}
		status := "failed"
		if r.cancelRequestedFor(invocationID) {
			status = "cancelled"
		}
		a.Status, a.Settled = status, &obs
		a.ProcessOutcome = &local.ProcessOutcome{Started: false, StopReason: code}
		step, stage := r.Steps[a.StepID], r.Activations[a.ActivationID]
		step.Status, step.Settled = status, &obs
		stage.Status, stage.Settled = status, &obs
		removeActive(r, attemptID)
		if err := r.setReadyFor(invocationID, []string{}); err != nil {
			return local.Change{}, err
		}
		if status == "cancelled" && isInvocationState(r.SchemaVersion) {
			if err := r.setInvocationStatus(invocationID, "stopping", nil); err != nil {
				return local.Change{}, err
			}
		} else if err := r.setInvocationStatus(invocationID, status, &obs); err != nil {
			return local.Change{}, err
		}
		change := local.Change{ReleaseSlot: attemptID}
		if status == "failed" {
			if err := diagnostic(r, commandID, attemptID, code, "dispatch", "Process was not started; the admission and slot are settled", obs); err != nil {
				return local.Change{}, err
			}
			if planErr == nil {
				event, handled, err := routeKnownError(r, p, a.ActivationID, attemptID, code, obs)
				if err != nil {
					return local.Change{}, err
				}
				if handled {
					data, _ := canonical(map[string]any{"observation": obs, "attempt_id": attemptID, "spawn_observation": "known_not_started", "failure": code})
					change.Events = []local.EventInput{{Type: "attempt.settled", Version: local.EventVersion, Data: data}, event}
				}
			}
		}
		return change, nil
	})
	return err
}

// Only a proven settled failure can consume on_error. The failed StepInstance
// stays failed; the durable transition names its activation and next stage.
// Unknown effects, cancellation and an invalid pinned plan never enter here.
// authorityFailures are failures of this authority itself. A declared on_error
// branch answers what the worker did; it must never be entered because the
// engine could not persist or seal the evidence of what the worker did.
var authorityFailures = map[string]bool{"authority_persistence_failed": true, "authority_output_sealing_failed": true}

// sealingClass separates this authority's own storage failure from a worker's
// invalid output. Only the second is a verdict about the work, and only the
// second may be answered by a declared error route.
func sealingClass(err error, workerCode string) string {
	if persistenceFailure(err) || errors.Is(err, local.ErrBlobLimit) || errors.Is(err, local.ErrSampleLimit) || errors.Is(err, local.ErrReadOnly) {
		return "authority_output_sealing_failed"
	}
	return workerCode
}

func routeKnownError(r *Run, p *flow.Plan, activationID, attemptID, code string, obs Observation) (local.EventInput, bool, error) {
	if authorityFailures[code] {
		return local.EventInput{}, false, nil
	}
	if r.Profile != flow.CoreProfile || p.Profile != r.Profile || r.HasUnresolvedEffects || len(r.Active) != 0 || r.ActiveCheckID != "" || r.PendingAcceptance != nil {
		return local.EventInput{}, false, nil
	}
	a := r.Activations[activationID]
	if a == nil || a.Status != "failed" || a.Settled == nil {
		return local.EventInput{}, false, errors.New("error transition requires a settled failed activation")
	}
	if r.cancelRequestedFor(a.InvocationID) {
		return local.EventInput{}, false, nil
	}
	next, err := p.NextError(a.StageID)
	if err != nil {
		return local.EventInput{}, false, nil // An absent handler leaves the failure terminal.
	}
	if err := r.advanceInvocation(a.InvocationID, next); err != nil {
		return local.EventInput{}, false, err
	}
	data, err := canonical(map[string]any{"stage_activation_id": activationID, "stage_id": a.StageID, "attempt_id": attemptID, "failure": code, "next_stage_id": next, "observation": obs})
	return local.EventInput{Type: "stage.error_handled", Version: local.EventVersion, Data: data}, true, err
}

func (e *Engine) finishCancellation(ctx context.Context, runID, invocationID string) error {
	_, err := e.apply(ctx, e.owner, newID("command"), runID, "run.finished", map[string]any{"kind": "cancel"}, nil, local.CommandGuarded, func(r *Run, s local.Snapshot, obs Observation) (local.Change, error) {
		if !r.cancelRequestedFor(invocationID) || len(r.Active) > 0 || r.ActiveCheckID != "" || r.HasUnresolvedEffects {
			return local.Change{}, local.Reject("unsettled", "active obligations prevent cancellation")
		}
		if pending := r.PendingAcceptance; pending != nil && r.withinInvocation(pending.InvocationID, invocationID) {
			r.PendingAcceptance = nil
		}
		if isInvocationState(r.SchemaVersion) {
			for _, a := range r.Activations {
				if !r.withinInvocation(a.InvocationID, invocationID) || a.Settled != nil {
					continue
				}
				a.Status, a.Settled = "cancelled", &obs
				if a.StepID != "" {
					step := r.Steps[a.StepID]
					if step == nil {
						return local.Change{}, local.ErrIntegrity
					}
					step.Status, step.Settled = "cancelled", &obs
				}
			}
			ids := make([]string, 0, len(r.Invocations))
			for id := range r.Invocations {
				ids = append(ids, id)
			}
			slices.Sort(ids)
			change := local.Change{}
			for _, id := range ids {
				inv := r.Invocations[id]
				if r.withinInvocation(inv.ID, invocationID) && !invocationTerminal(inv.Status) {
					inv.Ready = []string{}
					inv.Status, inv.Settled = "cancelled", &obs
					event, err := invocationFinishedEvent(inv, r.RootInvocationID, obs)
					if err != nil {
						return local.Change{}, err
					}
					change.Events = append(change.Events, event)
				}
			}
			return change, nil
		}
		// A choice has no worker to settle it later. Cancellation closes its
		// own activation and timing without inventing a branch decision.
		for _, a := range r.Activations {
			if a.Kind != "choice" || a.Settled != nil {
				continue
			}
			if r.Profile != flow.CoreProfile || a.Status != "ready" || a.StepID != "" || !slices.Contains(r.Ready, a.StageID) {
				return local.Change{}, local.Reject("stage_conflict", "cancellation cannot settle an invalid choice activation")
			}
			a.Status, a.Settled = "cancelled", &obs
		}
		r.Status, r.Settled = "cancelled", &obs
		r.Ready = []string{}
		return local.Change{}, nil
	})
	return err
}

func (e *Engine) requestDriverCancellation(runID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	r, _, err := e.load(ctx, runID)
	if err != nil || r.terminal() || r.CancelRequested {
		return err
	}
	_, err = e.Restrict(ctx, RestrictCommand{SchemaVersion: "1", CommandID: newID("command"), Scope: "run", ScopeID: runID, Kind: "cancel", Reason: "foreground driver interrupted"})
	return err
}

func (e *Engine) interruptDriver(runID string) error {
	if err := e.requestDriverCancellation(runID); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r, _, err := e.load(ctx, runID)
	if err != nil || r.terminal() || r.HasUnresolvedEffects {
		return err
	}
	if r.ActiveCheckID != "" {
		check := r.CheckExecutions[r.ActiveCheckID]
		if check.Dispatch == nil {
			return e.settleCheckUnstarted(ctx, runID, check.ID, "", "cancelled")
		}
		return nil
	}
	if len(r.Active) == 0 {
		return e.finishCancellation(ctx, runID, r.RootInvocationID)
	}
	a := r.Attempts[r.Active[0]]
	if a.Dispatch == nil {
		return e.settleUnstarted(ctx, runID, a.ID, "", "cancelled")
	}
	return nil // No signal or retry authority can be inferred from persisted state.
}

func (e *Engine) recoverUncertain(ctx context.Context, r Run, reason string) error {
	commandID := newID("command")
	_, err := e.apply(ctx, e.owner, commandID, r.ID, "run.recovered", map[string]any{"reason": reason}, nil, local.CommandGuarded, func(r *Run, s local.Snapshot, obs Observation) (local.Change, error) {
		if len(r.Active) == 0 || r.terminal() {
			return local.Change{}, local.Reject("recovery_conflict", "there is no active dispatched attempt to recover")
		}
		previous := r.LastObserved
		r.Status = "uncertain"
		r.HasUnresolvedEffects = true
		r.Gaps = append(r.Gaps, TimingGap{previous, obs, "driver_ownership_lost"})
		for _, id := range r.Active {
			a := r.Attempts[id]
			a.Status = "uncertain"
			r.Steps[a.StepID].Status = "uncertain"
			r.Activations[a.ActivationID].Status = "uncertain"
			if err := r.setInvocationStatus(r.Activations[a.ActivationID].InvocationID, "uncertain", nil); err != nil {
				return local.Change{}, err
			}
		}
		if err := diagnostic(r, commandID, "", "executor_ownership_lost", "recovery", "Dispatch may have started; no automatic re-execution is allowed", obs); err != nil {
			return local.Change{}, err
		}
		return local.Change{}, nil
	})
	if err != nil {
		return err
	}
	return &DiagnosticError{ID: derivedID("diagnostic", r.ID, commandID, "recovery", "executor_ownership_lost"), Err: fault("recovery_required", "uncertain attempt retained; no process was launched")}
}

func recoveryError(r Run) error {
	err := fault("recovery_required", "an unresolved execution retains the slot; no blind retry")
	for i := len(r.Diagnostics) - 1; i >= 0; i-- {
		if r.Diagnostics[i].Origin == "core" {
			return &DiagnosticError{ID: r.Diagnostics[i].ID, Err: err}
		}
	}
	return err
}
