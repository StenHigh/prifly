package runtime

import (
	"context"
	"errors"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

var errArtifactCandidateRequired = errors.New("artifact candidate bytes are required")

func artifactItemKey(hook flow.Hook, supplied string) (string, error) {
	switch hook.Artifact.Cardinality {
	case "one":
		if supplied != "" {
			return "", local.Reject("invalid_publication", "one artifact hook uses its fixed item key")
		}
		return "item", nil
	case "keyed_many":
		if supplied == "" {
			return "", local.Reject("invalid_publication", "keyed_many artifact hook requires item_key")
		}
		return supplied, nil
	default:
		return "", local.Reject("invalid_publication", "artifact hook cardinality is invalid")
	}
}

func artifactPublicationRetry(r *Run, c PublishCommand, itemKey string) (local.Change, bool, error) {
	for _, previous := range r.ArtifactPublications {
		if previous.StepID != c.StepID || previous.Hook != c.Hook || previous.ItemKey != itemKey {
			continue
		}
		if previous.Artifact.Digest != c.ExpectedDigest || previous.SizeBytes != *c.ExpectedSizeBytes {
			return local.Change{}, true, local.Reject("artifact_key_conflict", "logical artifact key already identifies different bytes")
		}
		result, err := canonical(map[string]any{"publication": previous})
		return local.Change{ReceiptOnly: true, Result: result}, true, err
	}
	return local.Change{}, false, nil
}

func pendingArtifactPublicationRetry(r *Run, c PublishCommand, itemKey string) (local.Change, bool, error) {
	pending := r.PendingArtifactPublication
	if pending == nil || pending.StepID != c.StepID || pending.Hook != c.Hook || pending.ItemKey != itemKey {
		return local.Change{}, false, nil
	}
	if pending.Artifact.Digest != c.ExpectedDigest || pending.SizeBytes != *c.ExpectedSizeBytes {
		return local.Change{}, true, local.Reject("artifact_key_conflict", "logical artifact key already identifies different bytes")
	}
	return local.Change{}, true, local.Reject("publication_check_pending", "artifact bytes are sealed and awaiting their declared checks")
}

func (e *Engine) commitArtifactPublication(r *Run, snapshot local.Snapshot, obs Observation, rootPlan *flow.Plan, c PublishCommand, itemKey string, artifact Artifact, hook flow.Hook, actor string, evidence []EvidenceRef) (local.Change, error) {
	var count, recent int64
	for _, previous := range r.ArtifactPublications {
		if previous.StepID == c.StepID && previous.Hook == c.Hook {
			count++
		}
		if previous.AttemptID != c.AttemptID || previous.Hook != c.Hook {
			continue
		}
		if previous.Accepted.Session != obs.Session || previous.Accepted.MonotonicMS > obs.MonotonicMS {
			return local.Change{}, local.Reject("publisher_clock_unknown", "receipt order cannot be established for this publisher generation")
		}
		if obs.MonotonicMS-previous.Accepted.MonotonicMS < 60000 {
			recent++
		}
	}
	if len(r.Publications)+len(r.ArtifactPublications)+len(r.ArtifactClosures) >= MaxRunPublications || count >= hook.MaxCount {
		return local.Change{}, local.Reject("publication_count_exhausted", "declared publication count limit reached")
	}
	if recent >= hook.MaxPerMinute {
		return local.Change{}, local.Reject("publication_rate_exhausted", "declared publication rate limit reached")
	}
	publication := ArtifactPublication{
		SchemaVersion: ArtifactPublicationVersion,
		ID:            derivedID("artifact-publication", r.ID, c.StepID, c.Hook, itemKey),
		AttemptID:     c.AttemptID, StepID: c.StepID, Hook: c.Hook, ItemKey: itemKey,
		Artifact: artifact.Ref(), Format: artifact.Format, SchemaRef: hook.SchemaRef,
		MediaType: artifact.MediaType, SizeBytes: artifact.SizeBytes,
		ContentCheckEvidence: evidence, Classification: hook.Classification,
		Consumption: artifactConsumption(hook), AcceptedSequence: snapshot.EventSeq + 1,
		Accepted: obs, Actor: actor,
	}
	deliveryEvents, err := assignPublicationToWaits(r, rootPlan, publication, obs)
	if err != nil {
		return local.Change{}, err
	}
	r.ArtifactPublications = append(r.ArtifactPublications, publication)
	streamEvents, err := e.assignPublicationStreams(r, rootPlan, obs)
	if err != nil {
		return local.Change{}, err
	}
	deliveryEvents = append(deliveryEvents, streamEvents...)
	optional, err := canonicalState(map[string]any{"publications": r.Publications, "artifact_publications": r.ArtifactPublications, "diagnostics": r.Diagnostics})
	if err != nil {
		return local.Change{}, err
	}
	if len(optional) > maxPublicationBytes {
		return local.Change{}, local.Reject("publication_budget_exhausted", "optional publication budget is exhausted; control reserve remains available")
	}
	event, err := canonical(map[string]any{
		"publication_id": publication.ID, "step_instance_id": c.StepID,
		"attempt_id": c.AttemptID, "hook": c.Hook, "kind": "artifact",
		"item_key": itemKey, "artifact_ref": publication.Artifact,
		"observation": obs, "origin": "worker-reported", "trust": "worker-reported",
	})
	if err != nil {
		return local.Change{}, err
	}
	response, err := canonical(map[string]any{"publication": publication})
	events := append([]local.EventInput{{Type: "step.publication", Version: local.EventVersion, Data: event}}, deliveryEvents...)
	return local.Change{Events: events, Result: response, RequireStorageBudget: len(deliveryEvents) != 0, AdvanceRunVersion: len(deliveryEvents) != 0}, err
}

type publicationPublisherCurrent func(*Run, Observation) (*Attempt, *Activation, error)
type publicationPublisherReceipt func(local.ApplyResult, error) (local.ApplyResult, error)

func (e *Engine) publishArtifact(ctx context.Context, token string, c PublishCommand) (local.ApplyResult, error) {
	r, _, err := e.load(ctx, c.RunID)
	if err != nil {
		return local.ApplyResult{}, local.Reject("publisher_forbidden", "publisher has no current access to this namespace")
	}
	attempt, activation, err := publisherAttempt(r, token, c)
	if err != nil {
		return local.ApplyResult{}, err
	}
	definitionRef := r.Steps[attempt.StepID].Ref
	current := func(r *Run, obs Observation) (*Attempt, *Activation, error) {
		return currentPublisher(*r, token, c, definitionRef, activation.InvocationID, activation.StageID, obs)
	}
	receipt := func(result local.ApplyResult, err error) (local.ApplyResult, error) {
		return e.publisherReceipt(ctx, token, c, result, err)
	}
	return e.publishArtifactAs(ctx, c, r, attempt, activation, current, receipt)
}

func (e *Engine) publishArtifactAs(ctx context.Context, c PublishCommand, r Run, attempt *Attempt, activation *Activation, current publicationPublisherCurrent, receipt publicationPublisherReceipt) (local.ApplyResult, error) {
	if !isArtifactPublicationState(r.SchemaVersion) {
		return local.ApplyResult{}, local.Reject("unsupported_publication", "this Run does not carry the artifact publication contract")
	}
	rootPlan, err := r.plan()
	if err != nil {
		return local.ApplyResult{}, err
	}
	plan, err := r.planForCompiled(rootPlan, activation.InvocationID)
	if err != nil {
		return local.ApplyResult{}, err
	}
	hook, err := plan.ValidateArtifactPublication(activation.StageID, c.Hook)
	if err != nil {
		var problem *flow.Problem
		if errors.As(err, &problem) {
			return local.ApplyResult{}, local.Reject(problem.Code, problem.Message)
		}
		return local.ApplyResult{}, err
	}
	itemKey, err := artifactItemKey(hook, c.ItemKey)
	if err != nil {
		return local.ApplyResult{}, err
	}
	if *c.ExpectedSizeBytes > hook.MaxPayloadBytes {
		return local.ApplyResult{}, local.Reject("payload_too_large", "artifact exceeds its declared hook byte limit")
	}
	mediaType, err := artifactMediaType(hook.Artifact.Format, hook.Artifact.MediaTypes)
	if err != nil {
		return local.ApplyResult{}, local.Reject("invalid_publication", err.Error())
	}
	actor := "publisher:" + attempt.ID

	// First ask the command store. Exact retries and already accepted logical
	// keys return here without touching the mutable candidate path.
	preflight, preflightErr := e.apply(ctx, actor, c.CommandID, c.RunID, "step.publication", c, nil, local.CommandPublication, func(currentRun *Run, _ local.Snapshot, obs Observation) (local.Change, error) {
		publisher, currentActivation, err := current(currentRun, obs)
		if err != nil {
			return local.Change{}, err
		}
		if currentRun.restrictedFor(currentActivation.InvocationID) || currentRun.CancelRequested || publisher.Status == "stopping" {
			return local.Change{}, local.Reject("publication_restricted", "artifact production is forbidden during a stop")
		}
		if artifactHookClosed(currentRun, c.StepID, c.Hook) {
			return local.Change{}, local.Reject("artifact_hook_closed", "this artifact hook has already accepted its final manifest")
		}
		if change, found, err := artifactPublicationRetry(currentRun, c, itemKey); found || err != nil {
			return change, err
		}
		if change, found, err := pendingArtifactPublicationRetry(currentRun, c, itemKey); found || err != nil {
			return change, err
		}
		return local.Change{}, errArtifactCandidateRequired
	})
	if !errors.Is(preflightErr, errArtifactCandidateRequired) {
		return receipt(preflight, preflightErr)
	}

	if attempt.Workspace == "" {
		return local.ApplyResult{}, local.Reject("artifact_candidate_invalid", "publisher has no owned workspace")
	}
	data, err := readLocal(attempt.Workspace, c.CandidatePath, hook.MaxPayloadBytes)
	if err != nil {
		return local.ApplyResult{}, local.Reject("artifact_candidate_invalid", "candidate is unavailable, unsafe or exceeds its declared limit")
	}
	if int64(len(data)) != *c.ExpectedSizeBytes {
		return local.ApplyResult{}, local.Reject("artifact_size_mismatch", "candidate size differs from the declared identity")
	}
	if rawDigest(data) != c.ExpectedDigest {
		return local.ApplyResult{}, local.Reject("artifact_digest_mismatch", "candidate bytes differ from the declared identity")
	}
	producer := map[string]any{
		"kind": "step", "run_id": r.ID, "workflow_invocation_id": activation.InvocationID,
		"stage_activation_id": activation.ID, "step_instance_id": attempt.StepID,
		"attempt_id": attempt.ID, "port": c.Hook,
	}
	artifactID := derivedID("artifact", r.ID, c.StepID, c.Hook, itemKey, c.CommandID)
	prepared, err := e.prepareArtifactWithClassification(data, hook.Artifact.Format, &hook.SchemaRef, artifactID, producer, nil, r.registry(), hook.Classification, mediaType)
	if err != nil {
		return local.ApplyResult{}, err
	}
	artifact, err := e.publishPreparedArtifact(prepared, r.registry())
	if err != nil {
		return local.ApplyResult{}, err
	}
	if len(hook.Artifact.ContentCheckRefs) != 0 {
		return e.prepareArtifactPublicationChecks(ctx, c, r, attempt, activation, hook, itemKey, artifact, current, receipt)
	}

	result, err := e.apply(ctx, actor, c.CommandID, c.RunID, "step.publication", c, nil, local.CommandPublication, func(currentRun *Run, snapshot local.Snapshot, obs Observation) (local.Change, error) {
		publisher, currentActivation, err := current(currentRun, obs)
		if err != nil {
			return local.Change{}, err
		}
		if currentRun.restrictedFor(currentActivation.InvocationID) || currentRun.CancelRequested || publisher.Status == "stopping" {
			return local.Change{}, local.Reject("publication_restricted", "artifact production is forbidden during a stop")
		}
		if artifactHookClosed(currentRun, c.StepID, c.Hook) {
			return local.Change{}, local.Reject("artifact_hook_closed", "this artifact hook has already accepted its final manifest")
		}
		if change, found, err := artifactPublicationRetry(currentRun, c, itemKey); found || err != nil {
			return change, err
		}
		return e.commitArtifactPublication(currentRun, snapshot, obs, rootPlan, c, itemKey, artifact, hook, actor, []EvidenceRef{})
	})
	return receipt(result, err)
}
