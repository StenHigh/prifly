package runtime

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
	"github.com/stenhigh/prifly/internal/purity"
)

// CheckExecution owns one automatic-check admission and process. The report's
// pass/fail/inconclusive outcome is separate from this operation's settlement.
// RequestBytes preserves exact stdin bytes; JSON state normalization must not
// silently replace the request identity with a canonicalized approximation.
type CheckExecution struct {
	ID               string                 `json:"id"`
	Request          CheckRequest           `json:"request"`
	RequestBytes     local.BlobRef          `json:"request_bytes"`
	Workspace        string                 `json:"workspace"`
	Context          ContextManifest        `json:"context"`
	Status           string                 `json:"status"`
	Admitted         Observation            `json:"admitted"`
	Deadline         Observation            `json:"deadline"`
	DispatchDeadline Observation            `json:"dispatch_deadline"`
	Dispatch         *Observation           `json:"dispatch,omitempty"`
	Started          *Observation           `json:"started,omitempty"`
	ExecutorEnd      *Observation           `json:"executor_end,omitempty"`
	Settled          *Observation           `json:"settled,omitempty"`
	TokenHash        string                 `json:"token_hash,omitempty"`
	Process          *local.ProcessIdentity `json:"process,omitempty"`
	ProcessOutcome   *local.ProcessOutcome  `json:"process_outcome,omitempty"`
	Report           *CheckResult           `json:"report,omitempty"`
	ReportBytes      *local.BlobRef         `json:"report_bytes,omitempty"`
	Failure          string                 `json:"failure,omitempty"`
}

// CheckAdmission is prepared by the boundary owner before acquiring a slot.
// Context is transport local-context/2; Request has its own bootstrap protocol,
// not a surrogate ExecutionEnvelope or a manufactured producer Attempt.
type CheckAdmission struct {
	Request   []byte
	Workspace string
	Context   ContextManifest
	Prepared  Observation
}

func (e *Engine) admitCheck(ctx context.Context, loaded Run, view local.ReadView, commandID string, prepared CheckAdmission) (local.ApplyResult, error) {
	if e.ReadOnly {
		return local.ApplyResult{}, local.ErrReadOnly
	}
	request, err := ParseCheckRequest(prepared.Request)
	if err != nil {
		return local.ApplyResult{}, err
	}
	if err := checkBinding(loaded, request); err != nil {
		return local.ApplyResult{}, err
	}
	executor, err := checkExecutor(loaded, request)
	if err != nil {
		return local.ApplyResult{}, err
	}
	deadline, err := checkDeadline(prepared.Prepared, request.CheckDeadline, time.Duration(executor.Config.TimeoutMS)*time.Millisecond)
	if err != nil {
		return local.ApplyResult{}, err
	}
	dispatchDeadline, err := checkDeadline(prepared.Prepared, request.DispatchNotAfter, 30*time.Second)
	if err != nil {
		return local.ApplyResult{}, err
	}
	requestBytes, err := e.Blobs.Put(bytes.NewReader(prepared.Request), MaxCheckWireBytes)
	if err != nil {
		return local.ApplyResult{}, err
	}
	check := CheckExecution{ID: request.CheckID, Request: request, RequestBytes: requestBytes, Workspace: prepared.Workspace, Context: prepared.Context, Status: "pending", Deadline: deadline, DispatchDeadline: dispatchDeadline}
	if _, err := e.verifyCheckWorkspace(&check, executor); err != nil {
		return local.ApplyResult{}, err
	}
	pin, blocked, err := e.admissionGate(ctx)
	if err != nil {
		return local.ApplyResult{}, err
	}
	packagePin, packageBlocked, err := e.revokedPin(ctx, loaded)
	if err != nil {
		return local.ApplyResult{}, err
	}
	// A revoked checker stops being a source of new trusted acceptance.
	if blocked == nil {
		blocked = packageBlocked
	}
	pins := []local.ControlPin{}
	if packagePin != nil {
		pins = append(pins, *packagePin)
	}
	return e.applyControlledWithPins(ctx, pin, pins, e.owner, commandID, loaded.ID, "check.admitted", map[string]any{"check_execution_id": check.ID, "request_bytes": requestBytes, "workspace": check.Workspace, "context": check.Context, "prepared": prepared.Prepared}, &view.Snapshot.Version, local.CommandCAS, func(r *Run, snapshot local.Snapshot, obs Observation) (local.Change, error) {
		if blocked != nil {
			return local.Change{}, blocked
		}
		if r.CheckExecutions[check.ID] != nil || r.ActiveCheckID != "" || len(r.Active) != 0 && !checkMayOverlapPublisher(*r, request) || r.HasUnresolvedEffects || r.terminal() || r.admissionsBlockedFor(request.InvocationID) || r.cancelRequestedFor(request.InvocationID) {
			return local.Change{}, local.Reject("check_admission_blocked", "restriction or unsettled work prevents check admission")
		}
		if request.AdmittedVersion != snapshot.Version+1 || request.ControlEpoch != r.ControlEpoch {
			return local.Change{}, local.Reject("check_admission_changed", "request was prepared for another run version or control epoch")
		}
		if err := checkBinding(*r, request); err != nil {
			return local.Change{}, err
		}
		if _, err := checkExecutor(*r, request); err != nil {
			return local.Change{}, err
		}
		for _, due := range []Observation{deadline, dispatchDeadline} {
			if _, err := remainingBudget(prepared.Prepared, due, obs); err != nil {
				return local.Change{}, err
			}
		}
		if err := r.chargeInvocation(request.InvocationID, 1, 0); err != nil {
			return local.Change{}, err
		}
		if r.CheckExecutions == nil {
			r.CheckExecutions = make(map[string]*CheckExecution)
		}
		check.Admitted = obs
		r.CheckExecutions[check.ID], r.ActiveCheckID = &check, check.ID
		data, err := canonical(map[string]any{"check_execution_id": check.ID, "workflow_invocation_id": request.InvocationID, "stage_activation_id": request.ActivationID, "check_ref": request.CheckRef, "request_bytes": requestBytes, "observation": obs})
		return local.Change{AcquireSlot: check.ID, RequireStorageBudget: true, Events: []local.EventInput{{Type: "check.admitted", Version: local.EventVersion, Data: data}}}, err
	})
}

// The boundary owner proves which candidate is being checked. This independent
// guard prevents it from borrowing another scope, method, or live producer.
func checkExecutor(r Run, request CheckRequest) (PinnedExecutor, error) {
	if !isContextState(r.SchemaVersion) || r.Profile != flow.CoreProfile || request.RunID != r.ID || request.PackageLockDigest != r.LockRef.Digest {
		return PinnedExecutor{}, local.Reject("check_scope_mismatch", "check requires its pinned context-state Run")
	}
	invocation := r.Invocations[request.InvocationID]
	if invocation == nil || invocationTerminal(invocation.Status) {
		return PinnedExecutor{}, local.Reject("check_scope_mismatch", "check requires a live owning workflow invocation")
	}
	plan, err := r.planFor(request.InvocationID)
	if err != nil {
		return PinnedExecutor{}, err
	}
	if request.WorkflowRef != planRef(plan) || request.PolicyRef != plan.Workflow.PolicyRef {
		return PinnedExecutor{}, local.Reject("check_scope_mismatch", "request workflow or policy differs from its exact owner")
	}
	definition, exists := plan.Checks[request.CheckRef]
	if !exists {
		return PinnedExecutor{}, local.Reject("check_not_declared", "check definition is not in the pinned closure")
	}
	var refs []flow.Ref
	if request.Boundary == "workflow_input" {
		input, exists := plan.Workflow.Inputs[request.Port]
		if !exists || request.ActivationID != "" || request.ProducerAttemptID != "" || len(request.Subjects) != 1 || r.inputsFor(request.InvocationID)[request.Port] != request.Subjects[0] {
			return PinnedExecutor{}, local.Reject("check_scope_mismatch", "workflow input subject or ownership differs")
		}
		refs = input.ContentCheckRefs
	} else {
		activation := r.Activations[request.ActivationID]
		if activation == nil || activation.InvocationID != request.InvocationID || activation.Settled != nil {
			return PinnedExecutor{}, local.Reject("check_scope_mismatch", "check activation is absent, settled, or foreign")
		}
		if request.Boundary == "workflow_output" {
			if activation.Kind != "finish" || request.ProducerAttemptID != "" {
				return PinnedExecutor{}, local.Reject("check_scope_mismatch", "workflow output check requires its finish activation")
			}
			refs = plan.Workflow.Outputs[request.Port].ContentCheckRefs
		} else {
			if activation.Kind != "step" || r.Steps[activation.StepID] == nil {
				return PinnedExecutor{}, local.Reject("check_scope_mismatch", "step boundary has no real StepInstance")
			}
			step := plan.Steps[activation.StageID]
			switch request.Boundary {
			case "step_input":
				if request.ProducerAttemptID != "" {
					return PinnedExecutor{}, local.Reject("check_scope_mismatch", "input boundary has no producer attempt")
				}
				refs = step.Inputs[request.Port].ContentCheckRefs
			case "step_output", "step_result":
				producer := r.Attempts[request.ProducerAttemptID]
				if producer == nil || producer.ActivationID != activation.ID || producer.ProcessOutcome == nil || !producer.ProcessOutcome.Started || !producer.ProcessOutcome.WaitReturned || !producer.ProcessOutcome.GroupEmpty || producer.ProcessOutcome.Uncertain {
					return PinnedExecutor{}, local.Reject("check_producer_unsettled", "producer process settlement is not proven")
				}
				refs = step.ResultCheckRefs
				if request.Boundary == "step_output" {
					refs = step.Outputs[request.Port].ContentCheckRefs
				}
			case "artifact_publication":
				producer := r.Attempts[request.ProducerAttemptID]
				if producer == nil || producer.ActivationID != activation.ID || producer.Settled != nil {
					return PinnedExecutor{}, local.Reject("check_producer_unsettled", "publication check requires its live producer")
				}
				hook := step.Hooks[request.Port]
				if hook.Kind != "artifact" || hook.Artifact == nil {
					return PinnedExecutor{}, local.Reject("check_not_declared", "publication check hook is not declared")
				}
				refs = hook.Artifact.ContentCheckRefs
			default:
				return PinnedExecutor{}, local.Reject("check_scope_mismatch", "unsupported check boundary")
			}
		}
	}
	if !slices.Contains(refs, request.CheckRef) || (request.Boundary == "step_result") != (definition.Kind == "result") {
		return PinnedExecutor{}, local.Reject("check_not_declared", "check is not declared at this boundary")
	}
	if definition.Executor.AdapterRef != builtinVersionRef(r.Definitions, "core:adapter/local-process", "2.0.0") || definition.Executor.Operation != "check" {
		return PinnedExecutor{}, local.Reject("unsupported_check_executor", "automatic check requires the exact local check operation")
	}
	executor, exists := r.Executors[executorKey(r, request.CheckRef, definition.ID)]
	if !exists || executor.ContextProfile == nil || executor.Config.TimeoutMS <= 0 || executor.Config.TimeoutMS > 3600000 || executor.Config.GraceMS <= 0 || executor.Config.MaxOutputBytes <= 0 {
		return PinnedExecutor{}, local.Reject("unsupported_check_executor", "qualified pinned check executor is missing")
	}
	return executor, nil
}

func checkDeadline(prepared Observation, utc string, maximum time.Duration) (Observation, error) {
	start, err := time.Parse(time.RFC3339Nano, prepared.UTC)
	if err != nil {
		return Observation{}, local.Reject("deadline_clock_unqualified", "check preparation observation is invalid")
	}
	due, err := time.Parse(time.RFC3339Nano, utc)
	if err != nil || due.Sub(start) < time.Millisecond || due.Sub(start) > maximum {
		return Observation{}, local.Reject("check_deadline_invalid", "check deadline exceeds its original preparation allowance")
	}
	deadline := prepared
	deadline.UTC, deadline.MonotonicMS = utc, prepared.MonotonicMS+due.Sub(start).Milliseconds()
	return deadline, nil
}

func (e *Engine) verifyCheckWorkspace(check *CheckExecution, executor PinnedExecutor) ([]byte, error) {
	if check.RequestBytes.Size < 1 || check.RequestBytes.Size > MaxCheckWireBytes {
		return nil, local.Reject("check_request_drift", "request byte length is outside the check wire limit")
	}
	requestBytes, err := e.Blobs.Read(check.RequestBytes)
	if err != nil {
		return nil, local.Reject("check_request_drift", "sealed request bytes are unavailable")
	}
	request, err := ParseCheckRequest(requestBytes)
	if err != nil {
		return nil, err
	}
	left, leftErr := canonical(request)
	right, rightErr := canonical(check.Request)
	if leftErr != nil || rightErr != nil || !bytes.Equal(left, right) || request.CheckID != check.ID || request.CheckDeadline != check.Deadline.UTC || request.DispatchNotAfter != check.DispatchDeadline.UTC {
		return nil, local.Reject("check_request_drift", "stored request fields differ from exact admitted bytes")
	}
	relative, err := filepath.Rel(filepath.Join(e.Root, e.Config.Configuration.WorkspaceRoot), check.Workspace)
	if err != nil || !safeRelative(relative) || !filepath.IsAbs(check.Workspace) {
		return nil, local.ErrUnsafePath
	}
	purity.Guard("os.open")
	root, err := os.OpenRoot(filepath.Join(e.Root, e.Config.Configuration.WorkspaceRoot))
	if err != nil {
		return nil, err
	}
	defer root.Close()
	current := ""
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := root.Lstat(current)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, local.ErrUnsafePath
		}
	}
	transport := check.Context
	if transport.SchemaVersion != "local-context/2" || transport.Manifest == nil || transport.Rendering == nil || len(transport.Outputs) != 0 || transport.Manifest.Ref != request.ContextManifestRef {
		return nil, local.Reject("check_context_drift", "check requires its exact full manifest and no worker output slots")
	}
	transportBytes, err := canonical(transport)
	if err != nil {
		return nil, err
	}
	fileBytes, err := readLocal(check.Workspace, "context.json", MaxDefinitionBytes)
	if err != nil || !bytes.Equal(transportBytes, fileBytes) {
		return nil, local.Reject("check_context_drift", "materialized check transport differs")
	}
	inputs := make(map[string]ArtifactRef, len(transport.Inputs))
	for name, input := range transport.Inputs {
		inputs[name] = input.Ref
	}
	if err := verifyFullWorkspaceContext(check.Workspace, transport, request.ContextManifestRef, inputs, executor.ContextProfile); err != nil {
		return nil, err
	}
	// The source copies and full manifest were validated above. A valid hash
	// alone must not let an older rendering describe another admitted request.
	var rendering struct {
		SchemaVersion string          `json:"schema_version"`
		Request       json.RawMessage `json:"check_request"`
		RequestDigest string          `json:"check_request_digest"`
		Envelope      json.RawMessage `json:"envelope"`
	}
	rendered, err := readLocal(check.Workspace, transport.Rendering.Path, MaxArtifactBytes)
	if err != nil || json.Unmarshal(rendered, &rendering) != nil || rendering.SchemaVersion != "context-rendering/1" || len(rendering.Envelope) != 0 || rendering.RequestDigest != check.RequestBytes.Digest || !bytes.Equal(bytes.TrimSpace(rendering.Request), bytes.TrimSpace(requestBytes)) {
		return nil, local.Reject("check_context_drift", "rendering does not describe the exact admitted check request")
	}
	available := make(map[ArtifactRef]bool, len(transport.Sources))
	for _, source := range transport.Sources {
		available[source.Ref] = true
	}
	for _, subject := range request.Subjects {
		if !available[subject] {
			return nil, local.Reject("check_context_drift", "check subject is missing from the manifest")
		}
	}
	if request.CandidateRef != nil && !available[*request.CandidateRef] {
		return nil, local.Reject("check_context_drift", "result candidate is missing from the manifest")
	}
	for path, blob := range executor.Files {
		data, err := readLocal(check.Workspace, path, MaxArtifactBytes)
		if err != nil || rawDigest(data) != blob.Digest || int64(len(data)) != blob.Size {
			return nil, local.Reject("workspace_script_drift", "check executor dependency differs from its pinned bytes")
		}
	}
	return requestBytes, nil
}

// The foreground driver must hold driverLock. A saved dispatch boundary never
// grants this invocation permission to launch a replacement process.
func (e *Engine) executePendingCheck(ctx context.Context, loaded Run, _ local.ReadView, admitted *CheckExecution) error {
	if e.ReadOnly {
		return local.ErrReadOnly
	}
	if admitted == nil || admitted.Request.RunID != loaded.ID {
		return local.ErrIntegrity
	}
	r, view, err := e.load(ctx, loaded.ID)
	if err != nil {
		return err
	}
	check := r.CheckExecutions[admitted.ID]
	if check == nil {
		return local.ErrIntegrity
	}
	if check.Settled != nil {
		return nil
	}
	if r.HasUnresolvedEffects || check.Status == "uncertain" {
		return recoveryError(r)
	}
	if check.Dispatch != nil {
		// A journalled empty process group leaves nothing to wait on, so the
		// check is closed as a lost settlement rather than held uncertain.
		if check.ExecutorEnd != nil {
			return e.settleCheckLost(ctx, r.ID, check.ID)
		}
		return e.recoverCheckUncertain(ctx, r, check.ID, "saved dispatch has no live foreground owner")
	}
	tokenHash := ""
	failBeforeStart := func(code string) error {
		var cancellationErr error
		if ctx.Err() != nil {
			cancellationErr = e.requestDriverCancellation(r.ID)
		}
		settleCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return errors.Join(cancellationErr, e.settleCheckUnstarted(settleCtx, r.ID, check.ID, tokenHash, code))
	}
	if r.cancelRequestedFor(check.Request.InvocationID) {
		return failBeforeStart("cancelled")
	}
	if r.admissionsBlockedFor(check.Request.InvocationID) {
		return nil
	}
	executor, err := checkExecutor(r, check.Request)
	if err != nil {
		return failBeforeStart(checkFailureCode(err, "unsupported_check_executor"))
	}
	requestBytes, err := e.verifyCheckWorkspace(check, executor)
	if err != nil {
		return failBeforeStart(checkFailureCode(err, "check_workspace_invalid"))
	}
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		return failBeforeStart("launch_identity_unavailable")
	}
	tokenHash = rawDigest([]byte(hex.EncodeToString(token[:])))
	_, err = e.apply(ctx, e.owner, newID("command"), r.ID, "check.dispatching", map[string]any{"check_execution_id": check.ID}, &view.Snapshot.Version, local.CommandCAS, func(r *Run, _ local.Snapshot, obs Observation) (local.Change, error) {
		current, err := activeCheck(r, check.ID, "")
		if err != nil {
			return local.Change{}, err
		}
		if current.Dispatch != nil || current.Status != "pending" || r.admissionsBlockedFor(current.Request.InvocationID) || r.cancelRequestedFor(current.Request.InvocationID) {
			return local.Change{}, local.Reject("check_dispatch_blocked", "pending check changed or is restricted")
		}
		if err := checkBinding(*r, current.Request); err != nil {
			return local.Change{}, err
		}
		for _, due := range []Observation{current.DispatchDeadline, current.Deadline} {
			if _, err := remainingBudget(current.Admitted, due, obs); err != nil {
				return local.Change{}, err
			}
		}
		current.Dispatch, current.Status, current.TokenHash = &obs, "dispatching", tokenHash
		data, err := canonical(map[string]any{"check_execution_id": check.ID, "observation": obs, "admission_control_epoch": current.Request.ControlEpoch, "dispatch_control_epoch": r.ControlEpoch})
		return local.Change{Events: []local.EventInput{{Type: "check.dispatching", Version: local.EventVersion, Data: data}}}, err
	})
	if err != nil {
		current, _, readErr := e.load(context.Background(), r.ID)
		if readErr != nil {
			return errors.Join(err, readErr)
		}
		pending := current.CheckExecutions[check.ID]
		if pending != nil && pending.Dispatch == nil && current.admissionsBlockedFor(pending.Request.InvocationID) && !current.cancelRequestedFor(pending.Request.InvocationID) && ctx.Err() == nil {
			return nil
		}
		return failBeforeStart(checkFailureCode(err, "check_dispatch_failed"))
	}
	remaining, err := remainingBudget(check.Admitted, check.Deadline, e.clock.now())
	if err != nil {
		return failBeforeStart(checkFailureCode(err, "deadline_clock_unqualified"))
	}
	environment := map[string]string{"PATH": "/usr/bin:/bin", "LANG": "C.UTF-8", "TMPDIR": filepath.Join(check.Workspace, "tmp"), "PRIFLY_CHECK_EXECUTION_ID": check.ID, "PRIFLY_REQUEST_DIGEST": check.RequestBytes.Digest, "PRIFLY_RUN_ID": r.ID, "PRIFLY_CONTEXT_FILE": filepath.Join(check.Workspace, "context.json")}
	for name, value := range executor.Config.Environment {
		if strings.HasPrefix(name, "PRIFLY_") {
			return failBeforeStart("check_environment_invalid")
		}
		environment[name] = value
	}
	processCtx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	watchDone, watchExited := make(chan struct{}), make(chan struct{})
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
				cancel(local.Reject("check_deadline_expired", "admitted check deadline expired"))
				return
			case <-ctx.Done():
				cancel(ctx.Err())
				watchErr = e.requestDriverCancellation(r.ID)
				return
			case <-ticker.C:
				budget, err := remainingBudget(check.Admitted, check.Deadline, e.clock.now())
				if err != nil {
					cancel(err)
					return
				}
				if next := time.Now().Add(budget); next.Before(deadline) {
					deadline = next
					timer.Reset(time.Until(deadline))
				}
				// The Run is read only when it changed; an idle check costs one
				// indexed row per tick instead of a full state decode.
				readCtx, readCancel := context.WithTimeout(context.Background(), time.Second)
				currentVersion, currentSequence, err := e.Store.Revision(readCtx, r.ID)
				if err == nil && (currentVersion != version || currentSequence != sequence) {
					version, sequence = currentVersion, currentSequence
					var current Run
					current, _, err = e.load(readCtx, r.ID)
					if err == nil {
						if current.cancelRequestedFor(check.Request.InvocationID) {
							readCancel()
							cancel(context.Canceled)
							return
						}
						if current.ActiveCheckID != check.ID || current.HasUnresolvedEffects {
							readCancel()
							cancel(local.Reject("check_ownership_lost", "check no longer owns the execution obligation"))
							return
						}
					}
				}
				readCancel()
				if err != nil {
					cancel(local.Reject("control_observation_failed", "cannot observe check cancellation state"))
					return
				}
			}
		}
	}()
	// RunProcess's historical Envelope field transports opaque validated JSON
	// bytes. No ExecutionEnvelope or producer Attempt is fabricated here. FD3
	// is not part of the check protocol; any output there is rejected below.
	outcome, runErr := local.RunProcess(processCtx, local.ProcessSpec{
		Executable: executor.Config.Executable, ExecutableDigest: executor.ExecutableDigest, Args: executor.Config.Args, Dir: check.Workspace, Env: environment, Envelope: requestBytes,
		MaxRuntime: remaining, GracePeriod: time.Duration(executor.Config.GraceMS) * time.Millisecond, KillWait: 2 * time.Second,
		MaxStdoutBytes: min(executor.Config.MaxOutputBytes, MaxCheckWireBytes), MaxStderrBytes: 64 << 10, MaxResultBytes: 1,
		BeforeStart: func() error {
			readCtx, readCancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer readCancel()
			current, _, err := e.load(readCtx, r.ID)
			if err != nil {
				return err
			}
			active, err := activeCheck(&current, check.ID, tokenHash)
			if err != nil || active.Dispatch == nil || active.Started != nil {
				return local.Reject("check_dispatch_ownership_lost", "check no longer belongs to this launch")
			}
			if current.cancelRequestedFor(active.Request.InvocationID) {
				return local.Reject("cancelled", "cancellation committed before check start")
			}
			if err := checkBinding(current, active.Request); err != nil {
				return err
			}
			slots, err := e.Store.Slots(readCtx)
			if err != nil || slots[check.ID] != r.ID {
				return local.Reject("check_dispatch_ownership_lost", "check does not own the authority slot")
			}
			// A pause after dispatch does not revoke already admitted work.
			now := e.clock.now()
			for _, due := range []Observation{active.DispatchDeadline, active.Deadline} {
				if _, err := remainingBudget(active.Admitted, due, now); err != nil {
					return err
				}
			}
			return nil
		},
	}, func(observation local.ProcessObservation) error {
		observeCtx, observeCancel := context.WithTimeout(context.Background(), observationDeadline)
		defer observeCancel()
		return e.observeCheck(observeCtx, r.ID, check.ID, tokenHash, observation)
	})
	close(watchDone)
	<-watchExited
	if ctx.Err() != nil && watchErr == nil {
		watchErr = e.requestDriverCancellation(r.ID)
	}
	if cause := context.Cause(processCtx); cause != nil && (outcome.StopReason == "" || outcome.StopReason == "cancelled") {
		outcome.StopReason = checkFailureCode(cause, "cancelled")
	}
	if !outcome.Started {
		code := checkFailureCode(runErr, "check_start_failed")
		if outcome.StopReason != "" {
			code = outcome.StopReason
		}
		return errors.Join(watchErr, failBeforeStart(code))
	}
	settleCtx, settleCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer settleCancel()
	return errors.Join(watchErr, e.settleCheck(settleCtx, r.ID, check.ID, tokenHash, outcome, runErr))
}

func activeCheck(r *Run, checkID, tokenHash string) (*CheckExecution, error) {
	check := r.CheckExecutions[checkID]
	if check == nil || check.ID != checkID || check.Request.RunID != r.ID || check.Settled != nil || r.ActiveCheckID != checkID || len(r.Active) != 0 && !checkMayOverlapPublisher(*r, check.Request) || r.HasUnresolvedEffects || tokenHash != "" && check.TokenHash != tokenHash {
		return nil, local.Reject("stale_check", "operation does not own the active check")
	}
	return check, nil
}

func (e *Engine) observeCheck(ctx context.Context, runID, checkID, tokenHash string, observed local.ProcessObservation) error {
	return retryOnBusy(ctx, func() error { return e.observeCheckOnce(ctx, runID, checkID, tokenHash, observed) })
}

func (e *Engine) observeCheckOnce(ctx context.Context, runID, checkID, tokenHash string, observed local.ProcessObservation) error {
	if observed.Kind == "result_candidate" {
		return local.Reject("check_protocol_violation", "check result must use stdout, not the StepResult channel")
	}
	event := "check.observed"
	if observed.Kind == "start_returned" {
		event = "check.started"
	}
	_, err := e.apply(ctx, "runner:"+strings.TrimPrefix(checkID, "check:"), newID("command"), runID, event, map[string]any{"check_execution_id": checkID, "kind": observed.Kind}, nil, local.CommandGuarded, func(r *Run, _ local.Snapshot, obs Observation) (local.Change, error) {
		check, err := activeCheck(r, checkID, tokenHash)
		if err != nil || check.Dispatch == nil {
			return local.Change{}, local.Reject("stale_check", "process observation has no owned dispatch")
		}
		if observed.Kind == "start_returned" {
			if check.Started != nil || check.Process != nil {
				return local.Change{}, local.Reject("duplicate_check_start", "check already has an observed process")
			}
			check.Started, check.Process, check.Status = &obs, &observed.Identity, "running"
		} else {
			if check.Process == nil || *check.Process != observed.Identity {
				return local.Change{}, local.Reject("check_process_mismatch", "observation belongs to another process")
			}
			switch observed.Kind {
			case "group_empty":
				check.ExecutorEnd, check.Status = &obs, "verifying"
			case "term_sent", "kill_sent":
				check.Status = "stopping"
			}
		}
		data, err := canonical(map[string]any{"check_execution_id": checkID, "observation": obs, "process_observation": observed})
		return local.Change{Events: []local.EventInput{{Type: event, Version: local.EventVersion, Data: data}}}, err
	})
	return err
}

func checkFailureCode(err error, fallback string) string {
	var problem *flow.Problem
	if errors.As(err, &problem) {
		return problem.Code
	}
	return driverFailureCode(err, fallback)
}

func (e *Engine) settleCheck(ctx context.Context, runID, checkID, tokenHash string, outcome local.ProcessOutcome, runErr error) error {
	loaded, _, err := e.load(ctx, runID)
	if err != nil {
		return err
	}
	prior := loaded.CheckExecutions[checkID]
	if prior == nil {
		return local.ErrIntegrity
	}
	if prior.Settled != nil {
		return nil
	}
	uncertain := outcome.Uncertain || !outcome.Started || !outcome.WaitReturned || !outcome.GroupEmpty
	var report *CheckResult
	var reportBytes *local.BlobRef
	failure := ""
	if !uncertain && len(outcome.Stdout.Raw) > 0 {
		blob, err := e.Blobs.Put(bytes.NewReader(outcome.Stdout.Raw), MaxCheckWireBytes)
		if err != nil {
			failure = "check_report_storage_failed"
		} else {
			reportBytes = &blob
		}
	}
	switch {
	case uncertain:
		failure = "check_settlement_unknown"
	case loaded.cancelRequestedFor(prior.Request.InvocationID):
		failure = "cancelled"
	case runErr != nil:
		failure = checkFailureCode(runErr, "check_observation_failed")
	case outcome.StopReason != "":
		failure = outcome.StopReason
	case outcome.ExitCode == nil || *outcome.ExitCode != 0:
		failure = "nonzero_exit"
	case outcome.ResultBytes != 0 || outcome.ResultError != "" || len(outcome.ResultCandidates) != 0:
		failure = "check_protocol_violation"
	case outcome.Stdout.Truncated || !outcome.Stdout.Complete:
		failure = "invalid_check_result"
	case failure != "":
		// Report storage failure cannot prevent recording a proven settlement.
	case reportBytes == nil:
		failure = "missing_check_result"
	default:
		requestBytes, err := e.Blobs.Read(prior.RequestBytes)
		if err != nil {
			failure = "check_request_drift"
		} else if parsed, err := ParseCheckResult(outcome.Stdout.Raw, requestBytes); err != nil {
			failure = checkFailureCode(err, "invalid_check_result")
		} else {
			report = &parsed
		}
	}
	event := "check.settled"
	if uncertain {
		event = "check.observed"
	}
	_, err = e.apply(ctx, "runner:"+strings.TrimPrefix(checkID, "check:"), newID("command"), runID, event, map[string]any{"check_execution_id": checkID, "process_outcome": outcome, "report_bytes": reportBytes, "failure": failure}, nil, local.CommandGuarded, func(r *Run, _ local.Snapshot, obs Observation) (local.Change, error) {
		check, err := activeCheck(r, checkID, tokenHash)
		if err != nil || check.Dispatch == nil || check.RequestBytes != prior.RequestBytes {
			return local.Change{}, local.Reject("stale_check", "settlement does not own the admitted check")
		}
		if check.Process != nil && *check.Process != outcome.Identity {
			return local.Change{}, local.Reject("check_process_mismatch", "settlement belongs to another process")
		}
		check.ProcessOutcome, check.ReportBytes, check.Failure = &outcome, reportBytes, failure
		if outcome.GroupEmpty && check.ExecutorEnd == nil {
			check.ExecutorEnd = &obs
		}
		change := local.Change{}
		if uncertain {
			check.Status, check.Report = "uncertain", nil
			r.HasUnresolvedEffects = true
			if err := r.setInvocationStatus(check.Request.InvocationID, "uncertain", nil); err != nil {
				return local.Change{}, err
			}
		} else {
			check.Settled = &obs
			r.ActiveCheckID = ""
			change.ReleaseSlot = checkID
			switch {
			case r.cancelRequestedFor(check.Request.InvocationID):
				check.Status, check.Failure, check.Report = "cancelled", "cancelled", nil
			case report != nil:
				check.Status, check.Report = "completed", report
			default:
				check.Status, check.Report = "failed", nil
			}
		}
		data, err := canonical(map[string]any{"check_execution_id": checkID, "status": check.Status, "failure": check.Failure, "report_bytes": reportBytes, "process_outcome": outcome, "observation": obs})
		change.Events = []local.EventInput{{Type: event, Version: local.EventVersion, Data: data}}
		return change, err
	})
	return err
}

// settleCheckLost closes a check whose process group was already observed empty
// and journalled before this driver lost the run: its slot is released instead
// of being held for a resolution. No report is read - the executor output that
// carries it was never seen by this driver, so the check settles failed.
func (e *Engine) settleCheckLost(ctx context.Context, runID, checkID string) error {
	_, err := e.apply(ctx, e.owner, derivedID("command", checkID, "settlement-lost"), runID, "check.settled", map[string]any{"check_execution_id": checkID, "failure": "check_settlement_lost"}, nil, local.CommandGuarded, func(r *Run, _ local.Snapshot, obs Observation) (local.Change, error) {
		check, err := activeCheck(r, checkID, "")
		if err != nil {
			return local.Change{}, err
		}
		if check.Dispatch == nil || check.ExecutorEnd == nil {
			return local.Change{}, local.Reject("check_recovery_conflict", "no dispatched check with a saved executor end")
		}
		check.Status, check.Failure, check.Settled, check.Report = "failed", "check_settlement_lost", &obs, nil
		if r.cancelRequestedFor(check.Request.InvocationID) {
			check.Status, check.Failure = "cancelled", "cancelled"
		}
		r.ActiveCheckID = ""
		data, err := canonical(map[string]any{"check_execution_id": checkID, "status": check.Status, "failure": check.Failure, "observation": obs})
		return local.Change{ReleaseSlot: checkID, Events: []local.EventInput{{Type: "check.settled", Version: local.EventVersion, Data: data}}}, err
	})
	return err
}

// Only this foreground owner can supply the token for a dispatched launch
// known not to have reached Start. Recovery never calls this after dispatch.
func (e *Engine) settleCheckUnstarted(ctx context.Context, runID, checkID, tokenHash, code string) error {
	_, err := e.apply(ctx, e.owner, newID("command"), runID, "check.settled", map[string]any{"check_execution_id": checkID, "spawn_observation": "known_not_started", "failure": code}, nil, local.CommandGuarded, func(r *Run, _ local.Snapshot, obs Observation) (local.Change, error) {
		check, err := activeCheck(r, checkID, "")
		if err != nil || check.Started != nil || check.Process != nil || check.Dispatch != nil && (tokenHash == "" || check.TokenHash != tokenHash) {
			return local.Change{}, local.Reject("check_unstarted_conflict", "no local no-spawn proof for this check")
		}
		check.Status, check.Settled, check.Failure = "failed", &obs, code
		if r.cancelRequestedFor(check.Request.InvocationID) {
			check.Status, check.Failure = "cancelled", "cancelled"
		}
		check.ProcessOutcome = &local.ProcessOutcome{Started: false, StopReason: code}
		r.ActiveCheckID = ""
		data, err := canonical(map[string]any{"check_execution_id": checkID, "status": check.Status, "failure": check.Failure, "spawn_observation": "known_not_started", "observation": obs})
		return local.Change{ReleaseSlot: checkID, Events: []local.EventInput{{Type: "check.settled", Version: local.EventVersion, Data: data}}}, err
	})
	return err
}

func (e *Engine) recoverCheckUncertain(ctx context.Context, loaded Run, checkID, reason string) error {
	_, err := e.apply(ctx, e.owner, newID("command"), loaded.ID, "check.recovered", map[string]any{"check_execution_id": checkID, "reason": reason}, nil, local.CommandGuarded, func(r *Run, _ local.Snapshot, obs Observation) (local.Change, error) {
		check := r.CheckExecutions[checkID]
		if check == nil || check.Settled != nil || check.Dispatch == nil || r.ActiveCheckID != checkID || r.terminal() {
			return local.Change{}, local.Reject("check_recovery_conflict", "no active dispatched check to recover")
		}
		check.Status, check.Failure = "uncertain", "check_ownership_lost"
		r.HasUnresolvedEffects = true
		r.Gaps = append(r.Gaps, TimingGap{r.LastObserved, obs, "check_ownership_lost"})
		if err := r.setInvocationStatus(check.Request.InvocationID, "uncertain", nil); err != nil {
			return local.Change{}, err
		}
		data, err := canonical(map[string]any{"check_execution_id": checkID, "failure": check.Failure, "observation": obs})
		return local.Change{Events: []local.EventInput{{Type: "check.recovered", Version: local.EventVersion, Data: data}}}, err
	})
	if err != nil {
		return err
	}
	return fault("recovery_required", "uncertain check retained; no process was launched")
}
