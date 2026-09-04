package runtime

import (
	"bytes"
	"context"
	"errors"
	"slices"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

var errArtifactManifestRequired = errors.New("artifact manifest bytes are required")

func artifactHookClosed(r *Run, stepID, hook string) bool {
	for _, closure := range r.ArtifactClosures {
		if closure.StepID == stepID && closure.Hook == hook {
			return true
		}
	}
	return false
}

func artifactClosureRetry(r *Run, command PublishCommand) (local.Change, bool, error) {
	for _, closure := range r.ArtifactClosures {
		if closure.StepID != command.StepID || closure.Hook != command.Hook {
			continue
		}
		if !slices.Equal(closure.ItemKeys, command.ItemKeys) {
			return local.Change{}, true, local.Reject("artifact_close_conflict", "artifact hook already closed with different membership")
		}
		result, err := canonical(map[string]any{"closure": closure})
		return local.Change{ReceiptOnly: true, Result: result}, true, err
	}
	return local.Change{}, false, nil
}

func artifactManifestFor(r Run, command PublishCommand) (ArtifactManifest, error) {
	items := []ArtifactPublication{}
	keys := []string{}
	cut := int64(0)
	for _, publication := range r.ArtifactPublications {
		if publication.StepID != command.StepID || publication.Hook != command.Hook {
			continue
		}
		items = append(items, publication)
		keys = append(keys, publication.ItemKey)
		cut = publication.AcceptedSequence
	}
	if !slices.Equal(keys, command.ItemKeys) {
		return ArtifactManifest{}, local.Reject("artifact_manifest_conflict", "close item_keys differ from accepted publication order")
	}
	return ArtifactManifest{
		SchemaVersion: ArtifactManifestVersion, RunID: r.ID, StepID: command.StepID,
		Hook: command.Hook, ItemCount: int64(len(items)), CutSequence: cut, Items: items,
	}, nil
}

func (e *Engine) publishArtifactClosure(ctx context.Context, token string, command PublishCommand) (local.ApplyResult, error) {
	r, _, err := e.load(ctx, command.RunID)
	if err != nil {
		return local.ApplyResult{}, local.Reject("publisher_forbidden", "publisher has no current access to this namespace")
	}
	attempt, activation, err := publisherAttempt(r, token, command)
	if err != nil {
		return local.ApplyResult{}, err
	}
	definitionRef := r.Steps[attempt.StepID].Ref
	current := func(r *Run, obs Observation) (*Attempt, *Activation, error) {
		return currentPublisher(*r, token, command, definitionRef, activation.InvocationID, activation.StageID, obs)
	}
	receipt := func(result local.ApplyResult, err error) (local.ApplyResult, error) {
		return e.publisherReceipt(ctx, token, command, result, err)
	}
	return e.publishArtifactClosureAs(ctx, command, r, attempt, activation, current, receipt)
}

func (e *Engine) publishArtifactClosureAs(ctx context.Context, command PublishCommand, r Run, attempt *Attempt, activation *Activation, current publicationPublisherCurrent, receipt publicationPublisherReceipt) (local.ApplyResult, error) {
	if !isArtifactClosureState(r.SchemaVersion) {
		return local.ApplyResult{}, local.Reject("unsupported_publication", "this Run does not carry the artifact closure contract")
	}
	rootPlan, err := r.plan()
	if err != nil {
		return local.ApplyResult{}, err
	}
	plan, err := r.planForCompiled(rootPlan, activation.InvocationID)
	if err != nil {
		return local.ApplyResult{}, err
	}
	hook, err := plan.ValidateArtifactPublication(activation.StageID, command.Hook)
	if err != nil {
		var problem *flow.Problem
		if errors.As(err, &problem) {
			return local.ApplyResult{}, local.Reject(problem.Code, problem.Message)
		}
		return local.ApplyResult{}, err
	}
	if hook.Artifact.Cardinality != "keyed_many" {
		return local.ApplyResult{}, local.Reject("unsupported_publication", "only a keyed_many artifact hook has a collection to close")
	}
	actor := "publisher:" + attempt.ID
	preflight, preflightErr := e.apply(ctx, actor, command.CommandID, command.RunID, "step.publication", command, nil, local.CommandPublication, func(currentRun *Run, _ local.Snapshot, obs Observation) (local.Change, error) {
		if change, found, err := artifactClosureRetry(currentRun, command); found || err != nil {
			return change, err
		}
		publisher, currentActivation, err := current(currentRun, obs)
		if err != nil {
			return local.Change{}, err
		}
		if currentRun.restrictedFor(currentActivation.InvocationID) || currentRun.CancelRequested || publisher.Status == "stopping" {
			return local.Change{}, local.Reject("publication_restricted", "artifact closure is forbidden during a stop")
		}
		if _, err := artifactManifestFor(*currentRun, command); err != nil {
			return local.Change{}, err
		}
		return local.Change{}, errArtifactManifestRequired
	})
	if !errors.Is(preflightErr, errArtifactManifestRequired) {
		return receipt(preflight, preflightErr)
	}

	fresh, _, err := e.load(ctx, command.RunID)
	if err != nil {
		return local.ApplyResult{}, err
	}
	manifest, err := artifactManifestFor(fresh, command)
	if err != nil {
		return local.ApplyResult{}, err
	}
	manifestBytes, err := canonical(manifest)
	if err != nil {
		return local.ApplyResult{}, err
	}
	manifestSchema := builtinRef(fresh.Definitions, "core:schema/artifact-manifest")
	if manifestSchema.ID == "" || flow.ValidateSchema(fresh.registry(), manifestSchema, manifestBytes) != nil {
		return local.ApplyResult{}, local.ErrIntegrity
	}
	producer := map[string]any{"kind": "authority", "authority_id": fresh.AuthorityID, "command_id": command.CommandID, "port": command.Hook}
	prepared, err := e.prepareArtifactWithClassification(manifestBytes, "json", &manifestSchema, derivedID("artifact", command.RunID, command.StepID, command.Hook, "manifest", command.CommandID), producer, nil, fresh.registry(), hook.Classification, "application/json")
	if err != nil {
		return local.ApplyResult{}, err
	}
	manifestArtifact, err := e.publishPreparedArtifact(prepared, fresh.registry())
	if err != nil {
		return local.ApplyResult{}, err
	}

	result, err := e.apply(ctx, actor, command.CommandID, command.RunID, "step.publication", command, nil, local.CommandPublication, func(currentRun *Run, snapshot local.Snapshot, obs Observation) (local.Change, error) {
		if change, found, err := artifactClosureRetry(currentRun, command); found || err != nil {
			return change, err
		}
		publisher, currentActivation, err := current(currentRun, obs)
		if err != nil {
			return local.Change{}, err
		}
		if currentRun.restrictedFor(currentActivation.InvocationID) || currentRun.CancelRequested || publisher.Status == "stopping" {
			return local.Change{}, local.Reject("publication_restricted", "artifact closure is forbidden during a stop")
		}
		currentManifest, err := artifactManifestFor(*currentRun, command)
		if err != nil {
			return local.Change{}, err
		}
		currentBytes, err := canonical(currentManifest)
		if err != nil || !bytes.Equal(currentBytes, manifestBytes) {
			return local.Change{}, local.Reject("artifact_manifest_conflict", "accepted publications changed while the manifest was sealed")
		}
		if len(currentRun.Publications)+len(currentRun.ArtifactPublications)+len(currentRun.ArtifactClosures) >= MaxRunPublications {
			return local.Change{}, local.Reject("publication_count_exhausted", "global publication count limit reached before close")
		}
		closure := ArtifactClosure{
			SchemaVersion: ArtifactClosureVersion, ID: derivedID("artifact-closure", currentRun.ID, command.StepID, command.Hook),
			AttemptID: command.AttemptID, StepID: command.StepID, Hook: command.Hook,
			ItemKeys: append([]string(nil), command.ItemKeys...), Manifest: manifestArtifact.Ref(),
			ItemCount: manifest.ItemCount, CutSequence: manifest.CutSequence,
			AcceptedSequence: snapshot.EventSeq + 1, Accepted: obs, Actor: actor,
		}
		currentRun.ArtifactClosures = append(currentRun.ArtifactClosures, closure)
		deliveryEvents, err := e.assignPublicationStreams(currentRun, rootPlan, obs)
		if err != nil {
			return local.Change{}, err
		}
		exhausted, err := optionalPublicationBudgetExhausted(currentRun, map[string]any{"publications": currentRun.Publications, "artifact_publications": currentRun.ArtifactPublications, "artifact_closures": currentRun.ArtifactClosures, "diagnostics": currentRun.Diagnostics})
		if err != nil {
			return local.Change{}, err
		}
		if exhausted {
			return local.Change{}, local.Reject("publication_budget_exhausted", "optional publication budget is exhausted; control reserve remains available")
		}
		event, err := canonical(map[string]any{
			"closure_id": closure.ID, "step_instance_id": command.StepID, "attempt_id": command.AttemptID,
			"hook": command.Hook, "kind": "close", "manifest_ref": closure.Manifest,
			"item_count": closure.ItemCount, "cut_sequence": closure.CutSequence,
			"observation": obs, "origin": "worker-reported", "trust": "worker-reported",
		})
		if err != nil {
			return local.Change{}, err
		}
		response, err := canonical(map[string]any{"closure": closure})
		events := append([]local.EventInput{{Type: "step.publication", Version: local.EventVersion, Data: event}}, deliveryEvents...)
		return local.Change{Events: events, Result: response, RequireStorageBudget: len(deliveryEvents) != 0, AdvanceRunVersion: len(deliveryEvents) != 0}, err
	})
	return receipt(result, err)
}
