package runtime

import (
	"context"
	"fmt"
	"slices"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

func (e *Engine) prepareArtifactPublicationChecks(ctx context.Context, command PublishCommand, loaded Run, attempt *Attempt, activation *Activation, hook flow.Hook, itemKey string, artifact Artifact, current publicationPublisherCurrent, receipt publicationPublisherReceipt) (local.ApplyResult, error) {
	if !isPublicationChecksState(loaded.SchemaVersion) {
		return local.ApplyResult{}, local.Reject("unsupported_artifact_checks", "this Run does not carry the publication check contract")
	}
	_, err := e.apply(ctx, "publisher:"+attempt.ID, derivedID("command", command.CommandID, "publication-check"), command.RunID, "artifact.publication_prepared", map[string]any{"command_id": command.CommandID, "artifact_ref": artifact.Ref()}, nil, local.CommandGuarded, func(run *Run, _ local.Snapshot, observed Observation) (local.Change, error) {
		publisher, currentActivation, err := current(run, observed)
		if err != nil {
			return local.Change{}, err
		}
		if run.PendingArtifactPublication != nil {
			pending := run.PendingArtifactPublication
			if pending.CommandID == command.CommandID && pending.AttemptID == command.AttemptID && pending.StepID == command.StepID && pending.Hook == command.Hook && pending.ItemKey == itemKey && pending.Artifact == artifact.Ref() {
				return local.Change{ReceiptOnly: true, Result: []byte(`{"status":"pending"}`)}, nil
			}
			return local.Change{}, local.Reject("publication_check_pending", "another artifact publication is awaiting its declared checks")
		}
		if run.restrictedFor(currentActivation.InvocationID) || run.CancelRequested || publisher.Status == "stopping" {
			return local.Change{}, local.Reject("publication_restricted", "artifact production is forbidden during a stop")
		}
		pending, err := newPendingArtifactPublication(run.ID, command.CommandID, publisher, currentActivation, command.Hook, itemKey, hook, artifact, observed)
		if err != nil {
			return local.Change{}, err
		}
		run.PendingArtifactPublication = pending
		return local.Change{RequireStorageBudget: true}, nil
	})
	if err != nil {
		return receipt(local.ApplyResult{}, err)
	}
	return local.ApplyResult{}, local.Reject("publication_check_pending", "artifact bytes are sealed and awaiting their declared checks")
}

func newPendingArtifactPublication(runID, commandID string, attempt *Attempt, activation *Activation, hookName, itemKey string, hook flow.Hook, artifact Artifact, observed Observation) (*PendingArtifactPublication, error) {
	if hook.Artifact == nil || len(hook.Artifact.ContentCheckRefs) == 0 {
		return nil, local.ErrIntegrity
	}
	pending := &PendingArtifactPublication{
		ID:           derivedID("pending-artifact-publication", runID, attempt.StepID, hookName, itemKey, commandID),
		CommandID:    commandID,
		AttemptID:    attempt.ID,
		StepID:       attempt.StepID,
		ActivationID: activation.ID,
		Hook:         hookName,
		ItemKey:      itemKey,
		Artifact:     artifact.Ref(),
		Format:       hook.Artifact.Format,
		SchemaRef:    hook.SchemaRef,
		MediaType:    artifact.MediaType,
		SizeBytes:    artifact.SizeBytes,
		Created:      observed,
	}
	for _, ref := range hook.Artifact.ContentCheckRefs {
		pending.CheckIDs = append(pending.CheckIDs, derivedID("publication-check", pending.ID, ref.ID, ref.Version, ref.Digest))
	}
	return pending, nil
}

// checkPublicationBinding proves that a checker is bound to one sealed
// candidate which has not yet become a visible ArtifactPublication.
func checkPublicationBinding(r Run, request CheckRequest) error {
	pending := r.PendingArtifactPublication
	if pending == nil || request.Boundary != "artifact_publication" || request.InvocationID == "" || request.ActivationID != pending.ActivationID || request.ProducerAttemptID != pending.AttemptID || request.Port != pending.Hook || len(request.Subjects) != 1 || request.Subjects[0] != pending.Artifact || !slices.Contains(pending.CheckIDs, request.CheckID) {
		return local.Reject("check_subject_mismatch", "check has no matching pending artifact publication")
	}
	return nil
}

// checkMayOverlapPublisher is deliberately narrower than the ordinary check
// admission rule: the sealed artifact is the only subject and its producer is
// still the exact live Attempt that published it.
func checkMayOverlapPublisher(r Run, request CheckRequest) bool {
	return request.Boundary == "artifact_publication" && slices.Contains(r.Active, request.ProducerAttemptID) && checkPublicationBinding(r, request) == nil
}

func activeCheckMayOverlapPublisher(r Run) bool {
	check := r.CheckExecutions[r.ActiveCheckID]
	return check != nil && checkMayOverlapPublisher(r, check.Request)
}

func (e *Engine) publicationPendingAcceptance(r Run) (*PendingAcceptance, error) {
	pending := r.PendingArtifactPublication
	if pending == nil {
		return nil, local.ErrIntegrity
	}
	activation := r.Activations[pending.ActivationID]
	attempt := r.Attempts[pending.AttemptID]
	if activation == nil || attempt == nil || activation.Kind != "step" || activation.StepID != pending.StepID || attempt.ActivationID != activation.ID || !slices.Contains(r.Active, attempt.ID) {
		return nil, local.ErrIntegrity
	}
	plan, err := r.planFor(activation.InvocationID)
	if err != nil {
		return nil, err
	}
	step := plan.Steps[activation.StageID]
	hook, ok := step.Hooks[pending.Hook]
	if !ok || hook.Kind != "artifact" || hook.Artifact == nil || len(hook.Artifact.ContentCheckRefs) != len(pending.CheckIDs) {
		return nil, local.ErrIntegrity
	}
	artifact, _, err := e.Artifact(pending.Artifact)
	if err != nil || artifact.Format != pending.Format || artifact.SchemaRef == nil || *artifact.SchemaRef != pending.SchemaRef || artifact.MediaType != pending.MediaType || artifact.SizeBytes != pending.SizeBytes {
		return nil, fmt.Errorf("pending publication artifact drift: %w", local.ErrIntegrity)
	}
	checks := make([]PendingCheck, len(hook.Artifact.ContentCheckRefs))
	for i, ref := range hook.Artifact.ContentCheckRefs {
		id := derivedID("publication-check", pending.ID, ref.ID, ref.Version, ref.Digest)
		if pending.CheckIDs[i] != id {
			return nil, local.ErrIntegrity
		}
		checks[i] = PendingCheck{ID: id, Ref: ref, Boundary: "artifact_publication", Port: pending.Hook, Subjects: []ArtifactRef{pending.Artifact}}
	}
	return &PendingAcceptance{ID: pending.ID, Kind: "artifact_publication", InvocationID: activation.InvocationID, ActivationID: activation.ID, ProducerAttemptID: attempt.ID, Bindings: map[string]ArtifactRef{pending.Hook: pending.Artifact}, PreparedArtifacts: map[string]Artifact{pending.Hook: artifact}, Checks: checks, Status: "pending", Created: pending.Created}, nil
}

func (e *Engine) driveArtifactPublicationChecks(ctx context.Context, r Run, view local.ReadView) error {
	pending, err := e.publicationPendingAcceptance(r)
	if err != nil {
		return err
	}
	for _, required := range pending.Checks {
		check := r.CheckExecutions[required.ID]
		if check == nil {
			prepared, err := e.prepareCheckAdmission(r, view, pending, required)
			if err != nil {
				return e.failArtifactPublicationChecks(ctx, r, view, checkFailureCode(err, "check_preparation_failed"))
			}
			_, err = e.admitCheck(ctx, r, view, newID("command"), prepared)
			return err
		}
		if check.Settled == nil {
			return local.ErrIntegrity
		}
		if check.Status != "completed" || check.Report == nil || check.Report.Status != "pass" {
			code := check.Failure
			if code == "" {
				code = "check_failed"
				if check.Report != nil && check.Report.Status == "inconclusive" {
					code = "check_inconclusive"
				}
			}
			return e.failArtifactPublicationChecks(ctx, r, view, code)
		}
		if _, err := e.checkEvidence(r, check); err != nil {
			return e.failArtifactPublicationChecks(ctx, r, view, checkFailureCode(err, "check_evidence_invalid"))
		}
	}
	return e.acceptArtifactPublicationChecks(ctx, r, view)
}

func (e *Engine) failArtifactPublicationChecks(ctx context.Context, loaded Run, view local.ReadView, failure string) error {
	pending := loaded.PendingArtifactPublication
	if pending == nil {
		return local.ErrIntegrity
	}
	_, err := e.apply(ctx, e.owner, newID("command"), loaded.ID, "artifact.publication_checks_failed", map[string]any{"pending_artifact_publication_id": pending.ID, "failure": failure}, &view.Snapshot.Version, local.CommandCAS, func(r *Run, _ local.Snapshot, obs Observation) (local.Change, error) {
		current := r.PendingArtifactPublication
		if current == nil || current.ID != pending.ID || r.ActiveCheckID != "" || r.HasUnresolvedEffects {
			return local.Change{}, local.Reject("publication_check_changed", "pending artifact publication changed before check failure")
		}
		r.PendingArtifactPublication = nil
		return local.Change{}, nil
	})
	return err
}

func (e *Engine) acceptArtifactPublicationChecks(ctx context.Context, loaded Run, view local.ReadView) error {
	pending, err := e.publicationPendingAcceptance(loaded)
	if err != nil {
		return err
	}
	evidence := make([]EvidenceRef, 0, len(pending.Checks))
	for _, required := range pending.Checks {
		check := loaded.CheckExecutions[required.ID]
		if check == nil || check.Status != "completed" || check.Report == nil || check.Report.Status != "pass" {
			return local.ErrIntegrity
		}
		ref, err := e.checkEvidence(loaded, check)
		if err != nil {
			return err
		}
		evidence = append(evidence, ref)
	}
	rootPlan, err := loaded.plan()
	if err != nil {
		return err
	}
	_, err = e.apply(ctx, e.owner, newID("command"), loaded.ID, "artifact.publication_checked", map[string]any{"pending_artifact_publication_id": pending.ID, "evidence": evidence}, &view.Snapshot.Version, local.CommandCAS, func(r *Run, snapshot local.Snapshot, obs Observation) (local.Change, error) {
		current := r.PendingArtifactPublication
		if current == nil || current.ID != pending.ID || r.ActiveCheckID != "" || r.HasUnresolvedEffects {
			return local.Change{}, local.Reject("publication_check_changed", "pending artifact publication changed before acceptance")
		}
		activation := r.Activations[current.ActivationID]
		attempt := r.Attempts[current.AttemptID]
		if activation == nil || attempt == nil || activation.Kind != "step" || activation.StepID != current.StepID || attempt.ActivationID != activation.ID || !slices.Contains(r.Active, attempt.ID) || r.restrictedFor(activation.InvocationID) || r.CancelRequested || attempt.Status == "stopping" {
			return local.Change{}, local.Reject("publication_restricted", "checked artifact no longer has a live publisher")
		}
		plan, err := r.planFor(activation.InvocationID)
		if err != nil {
			return local.Change{}, err
		}
		hook, ok := plan.Steps[activation.StageID].Hooks[current.Hook]
		if !ok || hook.Kind != "artifact" || hook.Artifact == nil || artifactHookClosed(r, current.StepID, current.Hook) {
			return local.Change{}, local.Reject("artifact_hook_closed", "artifact hook cannot accept this checked item")
		}
		for _, id := range current.CheckIDs {
			check := r.CheckExecutions[id]
			if check == nil || check.Status != "completed" || check.Report == nil || check.Report.Status != "pass" || checkPublicationBinding(*r, check.Request) != nil {
				return local.Change{}, local.Reject("publication_check_changed", "declared publication check is no longer a passing exact binding")
			}
		}
		artifact, _, err := e.Artifact(current.Artifact)
		if err != nil || artifact.Format != current.Format || artifact.SchemaRef == nil || *artifact.SchemaRef != current.SchemaRef || artifact.MediaType != current.MediaType || artifact.SizeBytes != current.SizeBytes {
			return local.Change{}, fmt.Errorf("pending publication artifact drift: %w", local.ErrIntegrity)
		}
		command := PublishCommand{CommandID: current.CommandID, RunID: r.ID, AttemptID: current.AttemptID, StepID: current.StepID, Hook: current.Hook, ItemKey: current.ItemKey, ExpectedDigest: current.Artifact.Digest, ExpectedSizeBytes: &current.SizeBytes}
		change, err := e.commitArtifactPublication(r, snapshot, obs, rootPlan, command, current.ItemKey, artifact, hook, "publisher:"+attempt.ID, evidence)
		if err != nil {
			return local.Change{}, err
		}
		r.PendingArtifactPublication = nil
		return change, nil
	})
	return err
}

func checkBinding(r Run, request CheckRequest) error {
	if request.Boundary == "artifact_publication" {
		return checkPublicationBinding(r, request)
	}
	return checkPendingBinding(r, request)
}
