package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// PendingAcceptance retains the exact boundary inputs while required checks
// own the Run's one execution slot. It is not a second scheduler or retry queue.
// PreparedArtifacts are sealed, unpublished producer outputs; Bindings never
// change between checking and consumption, including projected input identities.
type PendingAcceptance struct {
	ID                string                 `json:"id"`
	Kind              string                 `json:"kind"`
	InvocationID      string                 `json:"workflow_invocation_id"`
	ActivationID      string                 `json:"stage_activation_id,omitempty"`
	ProducerAttemptID string                 `json:"producer_attempt_id,omitempty"`
	Bindings          map[string]ArtifactRef `json:"bindings"`
	PreparedArtifacts map[string]Artifact    `json:"prepared_artifacts,omitempty"`
	CandidateRef      *ArtifactRef           `json:"candidate_ref,omitempty"`
	Checks            []PendingCheck         `json:"checks"`
	Status            string                 `json:"status"`
	Created           Observation            `json:"created"`
	Checked           *Observation           `json:"checked,omitempty"`
}

type PendingCheck struct {
	ID       string        `json:"check_execution_id"`
	Ref      flow.Ref      `json:"check_ref"`
	Boundary string        `json:"boundary"`
	Port     string        `json:"port,omitempty"`
	Subjects []ArtifactRef `json:"subjects"`
}

type EvidenceRef struct {
	ID     string `json:"id"`
	Digest string `json:"digest"`
}

// stepInstanceFor names the step this boundary belongs to. A workflow-level
// boundary has no step, so a waiver cannot silently cover one.
func (p *PendingAcceptance) stepInstanceFor(r Run) string {
	if p.ActivationID == "" {
		return ""
	}
	if activation := r.Activations[p.ActivationID]; activation != nil {
		return activation.StepID
	}
	return ""
}

func (p *PendingAcceptance) checkNamed(id string) *PendingCheck {
	for i := range p.Checks {
		if p.Checks[i].ID == id {
			return &p.Checks[i]
		}
	}
	return nil
}

// No caller supplies the required-check list: it is derived from the pinned
// owner definition. Optional absent ports have no subject and cause no check.
func newPendingAcceptance(r Run, p *flow.Plan, kind, invocationID, activationID, attemptID string, bindings map[string]ArtifactRef, prepared map[string]Artifact, candidate *ArtifactRef, obs Observation) (*PendingAcceptance, error) {
	if !isContextState(r.SchemaVersion) {
		return nil, nil
	}
	pending := &PendingAcceptance{ID: derivedID("acceptance", r.ID, invocationID, activationID, kind), Kind: kind, InvocationID: invocationID, ActivationID: activationID, ProducerAttemptID: attemptID, Bindings: maps.Clone(bindings), PreparedArtifacts: prepared, CandidateRef: candidate, Checks: []PendingCheck{}, Status: "pending", Created: obs}
	ports := map[string]flow.Port{}
	var resultRefs []flow.Ref
	switch kind {
	case "workflow_input":
		for name, input := range p.Workflow.Inputs {
			ports[name] = input.Port
		}
	case "workflow_output":
		for name, output := range p.Workflow.Outputs {
			ports[name] = output.Port
		}
	case "step_input", "step_result":
		activation := r.Activations[activationID]
		if activation == nil || activation.InvocationID != invocationID || activation.Kind != "step" {
			return nil, local.ErrIntegrity
		}
		step, ok := p.Steps[activation.StageID]
		if !ok {
			return nil, local.ErrIntegrity
		}
		if kind == "step_input" {
			for name, input := range step.Inputs {
				ports[name] = input.Port
			}
		} else {
			for name, output := range step.Outputs {
				ports[name] = output.Port
			}
			resultRefs = step.ResultCheckRefs
		}
	default:
		return nil, local.ErrIntegrity
	}
	add := func(ref flow.Ref, boundary, port string, subjects []ArtifactRef) {
		pending.Checks = append(pending.Checks, PendingCheck{ID: derivedID("check", pending.ID, strconv.Itoa(len(pending.Checks))), Ref: ref, Boundary: boundary, Port: port, Subjects: subjects})
	}
	names := slices.Sorted(maps.Keys(bindings))
	subjects := []ArtifactRef{}
	for _, name := range names {
		port, ok := ports[name]
		if !ok {
			return nil, local.ErrIntegrity
		}
		ref := bindings[name]
		if !slices.Contains(subjects, ref) {
			subjects = append(subjects, ref)
		}
		boundary := kind
		if kind == "step_result" {
			boundary = "step_output"
		}
		for _, check := range port.ContentCheckRefs {
			add(check, boundary, name, []ArtifactRef{ref})
		}
	}
	for _, ref := range resultRefs {
		add(ref, "step_result", "", slices.Clone(subjects))
	}
	if len(pending.Checks) == 0 {
		return nil, nil
	}
	return pending, nil
}

func (r *Run) beginWorkflowInputAcceptance(p *flow.Plan, invocationID string, obs Observation) error {
	pending, err := newPendingAcceptance(*r, p, "workflow_input", invocationID, "", "", r.inputsFor(invocationID), nil, nil, obs)
	if err != nil || pending == nil {
		return err
	}
	return r.holdAcceptance(pending, obs)
}

func (r *Run) holdAcceptance(pending *PendingAcceptance, obs Observation) error {
	if r.PendingAcceptance != nil || r.ActiveCheckID != "" || r.executingAttempts() != 0 || r.HasUnresolvedEffects || r.terminal() || r.cancelRequestedFor(pending.InvocationID) {
		return local.Reject("acceptance_blocked", "another obligation or restriction prevents boundary checks")
	}
	pending.Created = obs
	r.PendingAcceptance = pending
	if pending.ActivationID != "" {
		activation := r.Activations[pending.ActivationID]
		if activation == nil || activation.Settled != nil {
			return local.ErrIntegrity
		}
		activation.Status = "verifying"
		if activation.StepID != "" {
			r.Steps[activation.StepID].Status = "verifying"
		}
	}
	if err := r.setReadyFor(pending.InvocationID, []string{}); err != nil {
		return err
	}
	return r.setInvocationStatus(pending.InvocationID, "verifying", nil)
}

// Check admission cannot borrow a declared method for another candidate,
// projection, or output. The pending owner, not the checker, selects subjects.
func checkPendingBinding(r Run, request CheckRequest) error {
	pending := r.PendingAcceptance
	if pending == nil || pending.Status != "pending" || request.InvocationID != pending.InvocationID || request.ActivationID != pending.ActivationID || request.ProducerAttemptID != pending.ProducerAttemptID {
		return local.Reject("check_subject_mismatch", "check has no matching pending acceptance")
	}
	for _, required := range pending.Checks {
		if required.ID != request.CheckID {
			continue
		}
		if required.Ref != request.CheckRef || required.Boundary != request.Boundary || required.Port != request.Port || !slices.Equal(required.Subjects, request.Subjects) {
			return local.Reject("check_subject_mismatch", "request differs from the pinned boundary subjects")
		}
		if required.Boundary == "step_result" {
			if pending.CandidateRef == nil || request.CandidateRef == nil || *pending.CandidateRef != *request.CandidateRef {
				return local.Reject("check_subject_mismatch", "result check does not identify the pending candidate")
			}
		} else if request.CandidateRef != nil {
			return local.Reject("check_subject_mismatch", "content check cannot substitute a result candidate")
		}
		return nil
	}
	return local.Reject("check_not_declared", "check identity is not owned by pending acceptance")
}

func pendingPassed(r Run, kind, activationID string) bool {
	p := r.PendingAcceptance
	return p != nil && p.Status == "passed" && p.Kind == kind && p.ActivationID == activationID
}

func (e *Engine) validateBoundArtifacts(p *flow.Plan, ports map[string]flow.InputPort, refs map[string]ArtifactRef) error {
	for name, port := range ports {
		ref, exists := refs[name]
		if !exists {
			if port.Required {
				return local.ErrIntegrity
			}
			continue
		}
		artifact, data, err := e.Artifact(ref)
		if err != nil {
			return err
		}
		if err := e.validatePortArtifact(p, port.Port, artifact, data); err != nil {
			return err
		}
	}
	for name := range refs {
		if _, exists := ports[name]; !exists {
			return local.ErrIntegrity
		}
	}
	return nil
}

func (e *Engine) prepareBoundaryAcceptance(ctx context.Context, r Run, view local.ReadView, p *flow.Plan, kind string, activation *Activation, bindings map[string]ArtifactRef, commandID string) (bool, error) {
	pending, err := newPendingAcceptance(r, p, kind, activation.InvocationID, activation.ID, "", bindings, nil, nil, e.clock.now())
	if err != nil || pending == nil {
		return false, err
	}
	_, err = e.apply(ctx, e.owner, commandID, r.ID, "acceptance.prepared", map[string]any{"pending_acceptance_id": pending.ID, "bindings": pending.Bindings}, &view.Snapshot.Version, local.CommandCAS, func(r *Run, _ local.Snapshot, obs Observation) (local.Change, error) {
		current := r.Activations[activation.ID]
		if current == nil || current.Status != "ready" || current.Settled != nil {
			return local.Change{}, local.Reject("stage_conflict", "boundary no longer owns its ready stage")
		}
		return local.Change{RequireStorageBudget: true}, r.holdAcceptance(pending, obs)
	})
	return true, err
}

// driveAcceptance makes one durable advance. A settled check is never invoked
// again; a recorded dispatch without current ownership is handled by Drive's
// active-check branch and retains the global uncertainty barrier.
func (e *Engine) driveAcceptance(ctx context.Context, r Run, view local.ReadView) error {
	pending := r.PendingAcceptance
	if pending == nil || r.ActiveCheckID != "" || r.executingAttempts() != 0 {
		return local.ErrIntegrity
	}
	if pending.Status == "passed" && pending.Kind == "step_result" {
		return e.acceptCheckedResult(ctx, r, view)
	}
	if pending.Status != "pending" {
		return local.ErrIntegrity
	}
	waived := []string{}
	for _, required := range pending.Checks {
		check := r.CheckExecutions[required.ID]
		if check == nil {
			prepared, err := e.prepareCheckAdmission(r, view, pending, required)
			if err != nil {
				return e.rejectAcceptance(ctx, r, view, checkFailureCode(err, "check_preparation_failed"), required.ID)
			}
			_, err = e.admitCheck(ctx, r, view, newID("command"), prepared)
			return err
		}
		if check.Settled == nil {
			return local.ErrIntegrity
		}
		if check.Status == "completed" {
			if _, err := e.checkEvidence(r, check); err != nil {
				return e.rejectAcceptance(ctx, r, view, checkFailureCode(err, "check_evidence_invalid"), check.ID)
			}
		}
		if check.Status != "completed" || check.Report == nil || check.Report.Status != "pass" {
			failure := check.Failure
			if failure == "" {
				failure = "check_failed"
				if check.Report != nil && check.Report.Status == "inconclusive" {
					failure = "check_inconclusive"
				}
			}
			// A waiver covers exactly the check it names. The check stays
			// unpassed: only the requirement to pass it was refused, and the
			// reduction travels to the outcome.
			if waiver := r.waiverFor(pending.stepInstanceFor(r), required.Ref, e.clock.now()); waiver != nil {
				waived = append(waived, required.ID)
				continue
			}
			return e.rejectAcceptance(ctx, r, view, failure, check.ID)
		}
	}
	_, err := e.apply(ctx, e.owner, newID("command"), r.ID, "acceptance.passed", map[string]any{"pending_acceptance_id": pending.ID}, &view.Snapshot.Version, local.CommandCAS, func(r *Run, _ local.Snapshot, obs Observation) (local.Change, error) {
		current := r.PendingAcceptance
		if current == nil || current.ID != pending.ID || current.Status != "pending" || r.ActiveCheckID != "" || r.executingAttempts() != 0 || r.HasUnresolvedEffects || r.admissionsBlockedFor(current.InvocationID) || r.cancelRequestedFor(current.InvocationID) {
			return local.Change{}, local.Reject("acceptance_blocked", "pending acceptance or control state changed")
		}
		// The waivers are rechecked here: a decision that lapsed or was
		// withdrawn between the scan and the commit must not admit a boundary.
		for _, id := range waived {
			required := current.checkNamed(id)
			if required == nil {
				return local.Change{}, local.Reject("acceptance_blocked", "the waived check is no longer part of this boundary")
			}
			waiver := r.waiverFor(current.stepInstanceFor(*r), required.Ref, obs)
			if waiver == nil {
				return local.Change{}, local.Reject("waiver_absent", "no current waiver covers this check")
			}
			waiver.Status, waiver.AppliedTo = "applied", append(waiver.AppliedTo, id)
			r.WaiverApplied = true
		}
		current.Status, current.Checked = "passed", &obs
		data, err := canonical(map[string]any{"pending_acceptance_id": current.ID, "workflow_invocation_id": current.InvocationID, "stage_activation_id": current.ActivationID, "producer_attempt_id": current.ProducerAttemptID, "checks": current.Checks, "observation": obs})
		change := local.Change{Events: []local.EventInput{{Type: "acceptance.passed", Version: 1, Data: data}}}
		if err != nil {
			return local.Change{}, err
		}
		if current.Kind == "workflow_input" {
			plan, err := r.planFor(current.InvocationID)
			if err != nil {
				return local.Change{}, err
			}
			r.PendingAcceptance = nil
			return change, r.advanceInvocation(current.InvocationID, plan.Workflow.Definition.Entry)
		}
		if current.Kind != "step_result" {
			activation := r.Activations[current.ActivationID]
			activation.Status = "ready"
			if activation.StepID != "" {
				r.Steps[activation.StepID].Status = "ready"
			}
			return change, r.advanceInvocation(current.InvocationID, activation.StageID)
		}
		return change, nil
	})
	return err
}

func (e *Engine) rejectAcceptance(ctx context.Context, loaded Run, view local.ReadView, code, checkID string) error {
	pending := loaded.PendingAcceptance
	if pending == nil {
		return local.ErrIntegrity
	}
	plan, err := loaded.planFor(pending.InvocationID)
	if err != nil {
		return err
	}
	commandID := newID("command")
	_, err = e.apply(ctx, e.owner, commandID, loaded.ID, "acceptance.failed", map[string]any{"pending_acceptance_id": pending.ID, "check_execution_id": checkID, "failure": code}, &view.Snapshot.Version, local.CommandCAS, func(r *Run, _ local.Snapshot, obs Observation) (local.Change, error) {
		current := r.PendingAcceptance
		if current == nil || current.ID != pending.ID || r.ActiveCheckID != "" || r.executingAttempts() != 0 || r.HasUnresolvedEffects || r.cancelRequestedFor(current.InvocationID) {
			return local.Change{}, local.Reject("acceptance_blocked", "unsettled or changed acceptance cannot fail normally")
		}
		r.PendingAcceptance = nil
		if current.ActivationID != "" {
			activation := r.Activations[current.ActivationID]
			activation.Status, activation.Settled = "failed", &obs
			if activation.StepID != "" {
				step := r.Steps[activation.StepID]
				step.Status, step.Settled = "failed", &obs
			}
		}
		if err := r.failInvocation(current.InvocationID, obs); err != nil {
			return local.Change{}, err
		}
		causes := []string{current.ID}
		if checkID != "" {
			causes = append(causes, checkID)
		}
		if err := recordDiagnostic(r, Diagnostic{ID: derivedID("diagnostic", commandID, code), RunID: r.ID, AttemptID: current.ProducerAttemptID, ActivationID: current.ActivationID, Origin: "core", Severity: "error", Code: code, Category: "validation", Phase: "acceptance", Message: "Required check did not authorize this boundary; the producer result was not accepted", Observed: obs, CauseRefs: causes}); err != nil {
			return local.Change{}, err
		}
		data, err := canonical(map[string]any{"pending_acceptance_id": current.ID, "workflow_invocation_id": current.InvocationID, "stage_activation_id": current.ActivationID, "check_execution_id": checkID, "failure": code, "observation": obs})
		change := local.Change{Events: []local.EventInput{{Type: "acceptance.failed", Version: 1, Data: data}}}
		if err == nil && current.ActivationID != "" {
			event, handled, routeErr := routeKnownError(r, plan, current.ActivationID, current.ProducerAttemptID, code, obs)
			if routeErr != nil {
				return local.Change{}, routeErr
			}
			if handled {
				change.Events = append(change.Events, event)
			}
		}
		return change, err
	})
	return err
}

// Evidence records describe this check operation and its exact report. They do
// not turn the report into independent truth or allow cross-Run trusted reuse.
func (e *Engine) checkEvidence(r Run, check *CheckExecution) (EvidenceRef, error) {
	if check.Status != "completed" || check.Settled == nil || check.Report == nil || check.ReportBytes == nil {
		return EvidenceRef{}, local.ErrIntegrity
	}
	raw, err := e.Blobs.Read(*check.ReportBytes)
	if err != nil {
		return EvidenceRef{}, err
	}
	request, err := e.Blobs.Read(check.RequestBytes)
	if err != nil {
		return EvidenceRef{}, err
	}
	report, err := ParseCheckResult(raw, request)
	if err != nil {
		return EvidenceRef{}, err
	}
	left, _ := canonical(report)
	right, _ := canonical(check.Report)
	if !bytes.Equal(left, right) {
		return EvidenceRef{}, local.ErrIntegrity
	}
	plan, err := r.planFor(check.Request.InvocationID)
	if err != nil {
		return EvidenceRef{}, err
	}
	definition, ok := plan.Checks[check.Request.CheckRef]
	if !ok {
		return EvidenceRef{}, local.ErrIntegrity
	}
	producer := map[string]any{"kind": "authority", "authority_id": r.AuthorityID, "command_id": derivedID("command", check.ID, "evidence"), "port": "check_report"}
	artifact, err := e.putArtifact(raw, "blob", nil, derivedID("artifact", check.ID, "report"), producer, []ArtifactRef{check.Request.ContextManifestRef}, r.registry(), "application/json")
	if err != nil {
		return EvidenceRef{}, err
	}
	limitations := slices.Clone(report.Limitations)
	if limitations == nil {
		limitations = []string{}
	}
	// The bounded protocol allows at most 128 limitations. Always retain the
	// method's statements; the local qualification boundary is in its pinned
	// adapter/profile rather than an extra statement that could overflow them.
	evidenceID := derivedID("evidence", check.ID)
	subjects := check.Request.Subjects
	if check.Request.CandidateRef != nil {
		// The result claim is about this exact StepResult; its output subjects
		// remain individually bound by CheckRequest and the full context.
		subjects = []ArtifactRef{*check.Request.CandidateRef}
	}
	value := map[string]any{"schema_version": "1", "id": evidenceID, "claim": definition.Claim, "producer_ref": definition.Executor.AdapterRef, "subject_refs": subjects, "operation_id": check.ID, "method_ref": check.Request.CheckRef, "result_ref": artifact.Ref(), "observed_at": check.Settled.UTC, "limitations": limitations}
	data, err := canonical(value)
	if err != nil {
		return EvidenceRef{}, err
	}
	if err := flow.ValidateProtocol("Evidence", data); err != nil {
		return EvidenceRef{}, err
	}
	schema := builtinRef(r.Definitions, "core:schema/evidence")
	evidence, err := e.putArtifact(data, "json", &schema, evidenceID, producer, []ArtifactRef{artifact.Ref()}, r.registry())
	return EvidenceRef{ID: evidence.ID, Digest: evidence.Digest}, err
}

func (e *Engine) prepareCheckAdmission(r Run, view local.ReadView, pending *PendingAcceptance, required PendingCheck) (CheckAdmission, error) {
	plan, err := r.planFor(pending.InvocationID)
	if err != nil {
		return CheckAdmission{}, err
	}
	definition := plan.Checks[required.Ref]
	executor := r.Executors[executorKey(r, required.Ref, definition.ID)]
	if executor.ContextProfile == nil {
		return CheckAdmission{}, local.ErrIntegrity
	}
	inputs := map[string]ArtifactRef{}
	prepared := map[ArtifactRef]Artifact{}
	for _, artifact := range pending.PreparedArtifacts {
		prepared[artifact.Ref()] = artifact
	}
	for i, ref := range required.Subjects {
		inputs[fmt.Sprintf("subject_%03d", i)] = ref
	}
	var candidate *ArtifactRef
	if required.Boundary == "step_result" {
		candidate = pending.CandidateRef
		if candidate == nil {
			return CheckAdmission{}, local.ErrIntegrity
		}
		inputs["candidate"] = *candidate
	}
	commandID := derivedID("command", required.ID, "prepare")
	manifest, sources, contextArtifact, err := e.prepareContext(r, nil, nil, required.ID, commandID, *executor.ContextProfile, inputs, prepared)
	if err != nil {
		return CheckAdmission{}, err
	}
	created := e.clock.now()
	now, err := time.Parse(time.RFC3339Nano, created.UTC)
	if err != nil {
		return CheckAdmission{}, err
	}
	runtimeLimit := time.Duration(executor.Config.TimeoutMS) * time.Millisecond
	request := CheckRequest{SchemaVersion: CheckRequestVersion, CheckID: required.ID, RunID: r.ID, InvocationID: pending.InvocationID, ActivationID: pending.ActivationID, ProducerAttemptID: pending.ProducerAttemptID, Boundary: required.Boundary, Port: required.Port, CheckRef: required.Ref, WorkflowRef: planRef(plan), PolicyRef: plan.Workflow.PolicyRef, AdmissionID: derivedID("admission", required.ID), AdmittedVersion: view.Snapshot.Version + 1, ControlEpoch: r.ControlEpoch, PackageLockDigest: r.LockRef.Digest, Subjects: required.Subjects, CandidateRef: candidate, ContextManifestRef: contextArtifact.Ref(), DispatchNotAfter: now.Add(min(30*time.Second, runtimeLimit)).Format(time.RFC3339Nano), CheckDeadline: now.Add(runtimeLimit).Format(time.RFC3339Nano)}
	requestBytes, err := canonical(request)
	if err != nil {
		return CheckAdmission{}, err
	}
	if err := checkBinding(r, request); err != nil {
		return CheckAdmission{}, err
	}
	rendered, err := RenderCheckContext(manifest, requestBytes, sources)
	if err != nil {
		return CheckAdmission{}, err
	}
	// A preparation interrupted before admission may be repeated. The request
	// dates/version are new, so the rendering receives its own exact identity.
	rendering, err := e.putArtifact(rendered, "blob", nil, derivedID("artifact", required.ID, "rendering", rawDigest(rendered)), map[string]any{"kind": "authority", "authority_id": r.AuthorityID, "command_id": commandID, "port": "rendering"}, []ArtifactRef{contextArtifact.Ref()}, r.registry(), "application/json")
	if err != nil {
		return CheckAdmission{}, err
	}
	transport := ContextManifest{SchemaVersion: "local-context/2", Inputs: map[string]LocalPort{}, Outputs: map[string]OutputSlot{}, Dependencies: []flow.Ref{}, Manifest: &LocalPort{Ref: contextArtifact.Ref(), Path: "context/manifest.json"}, Rendering: &LocalPort{Ref: rendering.Ref(), Path: "context/rendered.json"}, Sources: []LocalPort{}}
	for _, definition := range r.Definitions {
		transport.Dependencies = append(transport.Dependencies, definition.Ref)
	}
	for i, source := range sources {
		port := LocalPort{Ref: source.Artifact.Ref(), Path: ContextSourcePath(i)}
		transport.Sources = append(transport.Sources, port)
		if name, ok := strings.CutPrefix(manifest.Entries[i].SourceID, "input:"); ok {
			transport.Inputs[name] = port
		}
	}
	transportBytes, err := canonical(transport)
	if err != nil {
		return CheckAdmission{}, err
	}
	if err := flow.ValidateSchema(r.registry(), builtinVersionRef(r.Definitions, "core:schema/local-context", "2.0.0"), transportBytes); err != nil {
		return CheckAdmission{}, err
	}
	workspace, err := e.prepareExecutorWorkspace(executor, strings.TrimPrefix(newID("workspace"), "workspace:"), transport, transportBytes, prepared)
	return CheckAdmission{Request: requestBytes, Workspace: workspace, Context: transport, Prepared: created}, err
}

func acceptancePreparedEvent(pending *PendingAcceptance) (local.EventInput, error) {
	data, err := canonical(map[string]any{"pending_acceptance_id": pending.ID, "kind": pending.Kind, "workflow_invocation_id": pending.InvocationID, "stage_activation_id": pending.ActivationID, "producer_attempt_id": pending.ProducerAttemptID, "bindings": pending.Bindings, "candidate_ref": pending.CandidateRef, "checks": pending.Checks, "observation": pending.Created})
	return local.EventInput{Type: "acceptance.prepared", Version: 1, Data: data}, err
}

func hasStepAcceptanceChecks(step flow.StepDefinition) bool {
	if len(step.ResultCheckRefs) != 0 {
		return true
	}
	for _, output := range step.Outputs {
		if len(output.ContentCheckRefs) != 0 {
			return true
		}
	}
	return false
}

func (e *Engine) acceptCheckedResult(ctx context.Context, loaded Run, view local.ReadView) error {
	pending := loaded.PendingAcceptance
	if pending == nil || pending.Kind != "step_result" || pending.Status != "passed" || pending.CandidateRef == nil {
		return local.ErrIntegrity
	}
	plan, err := loaded.planFor(pending.InvocationID)
	if err != nil {
		return err
	}
	_, data, err := e.Artifact(*pending.CandidateRef)
	if err != nil {
		return e.rejectAcceptance(ctx, loaded, view, "result_evidence_invalid", "")
	}
	var result Result
	if err := json.Unmarshal(data, &result); err != nil {
		return e.rejectAcceptance(ctx, loaded, view, "result_evidence_invalid", "")
	}
	producer := loaded.Attempts[pending.ProducerAttemptID]
	if producer == nil || producer.Accepted != nil || producer.Settled == nil || !bytes.Equal(data, producer.Candidate) || result.AttemptID != producer.ID || result.StepInstanceID != producer.StepID || !maps.Equal(result.Outputs, pending.Bindings) {
		return e.rejectAcceptance(ctx, loaded, view, "result_candidate_changed", "")
	}
	outputs := maps.Clone(pending.PreparedArtifacts)
	evidenceRefs := []EvidenceRef{}
	for _, required := range pending.Checks {
		check := loaded.CheckExecutions[required.ID]
		if check == nil || check.Status != "completed" || check.Report == nil || check.Report.Status != "pass" {
			return local.ErrIntegrity
		}
		evidence, err := e.checkEvidence(loaded, check)
		if err != nil {
			return e.rejectAcceptance(ctx, loaded, view, "check_evidence_invalid", required.ID)
		}
		evidenceRefs = append(evidenceRefs, evidence)
		if required.Boundary == "step_output" {
			artifact := outputs[required.Port]
			artifact.ContentCheckEvidence = append(slices.Clone(artifact.ContentCheckEvidence), evidence)
			outputs[required.Port] = artifact
		}
	}
	// Validate all siblings before publishing any. Recovery uses these sealed
	// bytes and original metadata; it never reads the producer's working files.
	for _, output := range outputs {
		data, err := e.Blobs.Read(local.BlobRef{Digest: output.Digest, Size: output.SizeBytes})
		if err == nil {
			_, _, err = e.validatePreparedArtifact(data, output, loaded.registry())
		}
		if err != nil {
			return e.rejectAcceptance(ctx, loaded, view, "checked_output_unavailable", "")
		}
	}
	// Publication is storage work, not a decision, so it happens here and the
	// transform only records what was published. Bytes that a refused
	// acceptance never references leave no accepted result behind, exactly as
	// a refused settlement does.
	for _, output := range outputs {
		if _, err := e.publishPreparedArtifact(output, loaded.registry()); err != nil {
			return err
		}
	}
	commandID := newID("command")
	_, err = e.apply(ctx, e.owner, commandID, loaded.ID, "attempt.accepted", map[string]any{"pending_acceptance_id": pending.ID, "attempt_id": producer.ID, "candidate_ref": pending.CandidateRef, "evidence_refs": evidenceRefs}, &view.Snapshot.Version, local.CommandCAS, func(r *Run, _ local.Snapshot, obs Observation) (local.Change, error) {
		current := r.PendingAcceptance
		if current == nil || current.ID != pending.ID || current.Status != "passed" || r.ActiveCheckID != "" || r.executingAttempts() != 0 || r.HasUnresolvedEffects || r.admissionsBlockedFor(current.InvocationID) || r.cancelRequestedFor(current.InvocationID) {
			return local.Change{}, local.Reject("acceptance_blocked", "checked result no longer owns unrestricted acceptance")
		}
		attempt := r.Attempts[current.ProducerAttemptID]
		if attempt == nil || attempt.Accepted != nil || attempt.CandidateConflict || !bytes.Equal(attempt.Candidate, data) {
			return local.Change{}, local.Reject("result_candidate_changed", "checked result candidate changed")
		}
		attempt.Accepted = &result
		step, activation := r.Steps[attempt.StepID], r.Activations[attempt.ActivationID]
		step.Status, step.Settled, step.Verdict, step.Outputs = "completed", &obs, result.Verdict, result.Outputs
		activation.Status, activation.Settled = "completed", &obs
		r.PendingAcceptance = nil
		next, routeErr := plan.Next(activation.StageID, result.Verdict)
		if routeErr != nil {
			if err := r.failInvocation(current.InvocationID, obs); err != nil {
				return local.Change{}, err
			}
			if err := diagnostic(r, commandID, attempt.ID, "unhandled_verdict", "routing", "Accepted verdict has no declared handler", obs); err != nil {
				return local.Change{}, err
			}
		} else if err := r.advanceInvocation(current.InvocationID, next); err != nil {
			return local.Change{}, err
		}
		fact, err := canonical(map[string]any{"attempt_id": attempt.ID, "stage_activation_id": activation.ID, "workflow_invocation_id": current.InvocationID, "pending_acceptance_id": current.ID, "candidate_ref": current.CandidateRef, "evidence_refs": evidenceRefs, "observation": obs})
		return local.Change{RequireStorageBudget: true, Events: []local.EventInput{{Type: "attempt.accepted", Version: 1, Data: fact}}}, err
	})
	return err
}

// Structural invariants are checked on every persisted read and mutation. OS
// byte proofs remain at admission/dispatch/acceptance, never inferred here.
func acceptanceInvariant(r Run) error {
	if !isContextState(r.SchemaVersion) {
		return nil
	}
	invalid := func() error { return errors.New("acceptance invariant: invalid check ownership or pending boundary") }
	if r.terminal() && (r.PendingAcceptance != nil || r.PendingArtifactPublication != nil || r.ActiveCheckID != "") || len(r.CheckExecutions) > 1024 || r.executingAttempts() != 0 && r.ActiveCheckID != "" && !activeCheckMayOverlapPublisher(r) {
		return invalid()
	}
	unsettled := 0
	for id, check := range r.CheckExecutions {
		if check == nil || check.ID != id || check.Request.CheckID != id || check.Request.RunID != r.ID || r.Invocations[check.Request.InvocationID] == nil || check.Request.PackageLockDigest != r.LockRef.Digest {
			return invalid()
		}
		terminal := check.Status == "completed" || check.Status == "failed" || check.Status == "cancelled"
		if terminal != (check.Settled != nil) || !slices.Contains([]string{"pending", "dispatching", "running", "stopping", "verifying", "completed", "failed", "cancelled", "uncertain"}, check.Status) {
			return invalid()
		}
		if !terminal {
			unsettled++
			if r.ActiveCheckID != id || invocationTerminal(r.Invocations[check.Request.InvocationID].Status) || checkBinding(r, check.Request) != nil || check.Request.Boundary != "artifact_publication" && r.PendingAcceptance == nil {
				return invalid()
			}
		}
		if check.Status == "completed" {
			if check.Report == nil || check.ReportBytes == nil || check.ProcessOutcome == nil || !check.ProcessOutcome.Started || !check.ProcessOutcome.WaitReturned || !check.ProcessOutcome.GroupEmpty || check.ProcessOutcome.Uncertain || check.Report.CheckID != id || check.Report.RunID != r.ID || check.Report.RequestDigest != check.RequestBytes.Digest {
				return invalid()
			}
		} else if check.Report != nil {
			return invalid()
		}
		if check.Dispatch != nil && check.TokenHash == "" || check.Started != nil && (check.Dispatch == nil || check.Process == nil) {
			return invalid()
		}
	}
	if (r.ActiveCheckID != "") != (unsettled == 1) || unsettled > 1 {
		return invalid()
	}
	pending := r.PendingAcceptance
	if pending == nil {
		return nil
	}
	if pending.ID != derivedID("acceptance", r.ID, pending.InvocationID, pending.ActivationID, pending.Kind) || len(pending.Checks) < 1 || len(pending.Checks) > 1024 || pending.Bindings == nil || r.Invocations[pending.InvocationID] == nil || invocationTerminal(r.Invocations[pending.InvocationID].Status) || r.executingAttempts() != 0 || (pending.Status != "pending" && pending.Status != "passed") || (pending.Status == "passed") != (pending.Checked != nil) {
		return invalid()
	}
	if pending.Kind == "workflow_input" {
		if pending.ActivationID != "" || pending.ProducerAttemptID != "" || pending.CandidateRef != nil || pending.PreparedArtifacts != nil || !maps.Equal(pending.Bindings, r.inputsFor(pending.InvocationID)) {
			return invalid()
		}
	} else {
		activation := r.Activations[pending.ActivationID]
		if activation == nil || activation.InvocationID != pending.InvocationID || activation.Settled != nil {
			return invalid()
		}
		if pending.Kind == "step_result" {
			attempt := r.Attempts[pending.ProducerAttemptID]
			if activation.Kind != "step" || attempt == nil || attempt.ActivationID != activation.ID || attempt.Settled == nil || attempt.Accepted != nil || pending.CandidateRef == nil || pending.CandidateRef.Digest != rawDigest(attempt.Candidate) || len(pending.PreparedArtifacts) != len(pending.Bindings) {
				return invalid()
			}
			for port, artifact := range pending.PreparedArtifacts {
				if artifact.Ref() != pending.Bindings[port] || artifact.Producer["attempt_id"] != attempt.ID || artifact.Producer["port"] != port || len(artifact.ContentCheckEvidence) != 0 {
					return invalid()
				}
			}
		} else if (pending.Kind != "step_input" && pending.Kind != "workflow_output") || pending.CandidateRef != nil || pending.ProducerAttemptID != "" || pending.PreparedArtifacts != nil {
			return invalid()
		}
		if pending.Kind == "step_input" && activation.Kind != "step" || pending.Kind == "workflow_output" && activation.Kind != "finish" {
			return invalid()
		}
	}
	subjects := []ArtifactRef{}
	for _, name := range slices.Sorted(maps.Keys(pending.Bindings)) {
		ref := pending.Bindings[name]
		if !slices.Contains(subjects, ref) {
			subjects = append(subjects, ref)
		}
	}
	copyPending := *pending
	copyPending.Status = "pending"
	copyRun := r
	copyRun.PendingAcceptance = &copyPending
	for i, required := range pending.Checks {
		if required.ID != derivedID("check", pending.ID, strconv.Itoa(i)) || required.Subjects == nil || len(required.Subjects) > MaxCheckSubjects || required.Ref.ID == "" || required.Ref.Version == "" || required.Ref.Digest == "" {
			return invalid()
		}
		if pending.Kind == "step_result" {
			if required.Boundary != "step_output" && required.Boundary != "step_result" {
				return invalid()
			}
		} else if required.Boundary != pending.Kind {
			return invalid()
		}
		if required.Boundary == "step_result" {
			if required.Port != "" || !slices.Equal(required.Subjects, subjects) {
				return invalid()
			}
		} else if ref, ok := pending.Bindings[required.Port]; !ok || len(required.Subjects) != 1 || required.Subjects[0] != ref {
			return invalid()
		}
		if check := r.CheckExecutions[required.ID]; check != nil {
			if checkPendingBinding(copyRun, check.Request) != nil {
				return invalid()
			}
		}
		if pending.Status == "passed" {
			check := r.CheckExecutions[required.ID]
			if check == nil || check.Status != "completed" || check.Report == nil || check.Report.Status != "pass" {
				return invalid()
			}
		}
	}
	return nil
}
