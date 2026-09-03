package runtime

import (
	"context"
	"errors"
	"slices"
	"sort"
	"strconv"
	"time"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

type streamDeliveryCandidate struct {
	Delivery      PublicationDelivery
	PublicationID string
	ClosureID     string
	Item          *ArtifactRef
}

func subscriptionForRepeat(r Run, activationID string, sourceRef flow.Ref) *PublicationSubscription {
	return r.PublicationSubscriptions[publicationSubscriptionID(r.ID, activationID, sourceRef)]
}

func streamSubscriptionForWaitState(r Run, p *flow.Plan, activation *Activation) (*PublicationSubscription, flow.PublicationSourceDefinition, error) {
	if activation == nil || activation.Kind != "wait" || activation.Wait == nil {
		return nil, flow.PublicationSourceDefinition{}, local.ErrIntegrity
	}
	stage := p.Workflow.Definition.Stages[activation.StageID]
	source, ok := p.PublicationSource(stage.SourceRef)
	if !ok || source.Mode != "each_publication" {
		return nil, source, errors.New("wait is not an each_publication source")
	}
	body := r.Invocations[activation.InvocationID]
	if body == nil || body.CallerActivationID == "" || body.Iteration == nil {
		return nil, source, local.ErrIntegrity
	}
	repeat := r.Activations[body.CallerActivationID]
	if repeat == nil || repeat.Kind != "repeat" || repeat.Repeat == nil || repeat.Repeat.CurrentBodyInvocationID != body.ID {
		return nil, source, local.ErrIntegrity
	}
	subscription := subscriptionForRepeat(r, repeat.ID, stage.SourceRef)
	newOnly := source.Initial == "new_only" && subscription != nil && subscription.SchemaVersion == PublicationNewOnlySubscriptionVersion && subscription.PublicationStartSequence != nil
	retained := source.Initial == "retained" && subscription != nil && subscription.SchemaVersion == PublicationSubscriptionVersion && subscription.PublicationStartSequence == nil
	if subscription == nil || !newOnly && !retained || subscription.InvocationID != repeat.InvocationID || subscription.Status != "open" || subscription.PendingAssignmentID != "" || *body.Iteration != subscription.Cursor+1 {
		return nil, source, local.ErrIntegrity
	}
	return subscription, source, nil
}

func (e *Engine) validateStreamWaitInputs(r Run, p *flow.Plan, activation *Activation) (*PublicationSubscription, flow.PublicationSourceDefinition, error) {
	subscription, source, err := streamSubscriptionForWaitState(r, p, activation)
	if err != nil {
		return nil, source, err
	}
	stage := p.Workflow.Definition.Stages[activation.StageID]
	if stage.CorrelationInput == nil || stage.CursorInput == nil {
		return nil, source, local.ErrIntegrity
	}
	inputs := r.inputsFor(activation.InvocationID)
	read := func(name string, schema *flow.Ref, target any) error {
		ref, ok := inputs[name]
		if !ok || schema == nil {
			return local.ErrIntegrity
		}
		artifact, data, err := e.Artifact(ref)
		if err != nil || artifact.SchemaRef == nil || *artifact.SchemaRef != *schema {
			return local.ErrIntegrity
		}
		return decode(data, target)
	}
	var handle PublicationSubscriptionHandle
	if err := read(stage.CorrelationInput.Port, source.HandleSchemaRef, &handle); err != nil || handle.SchemaVersion != PublicationHandleVersion || handle.SubscriptionID != subscription.ID || handle.RunID != r.ID || handle.Generation != subscription.Generation || handle.SourceRef != subscription.SourceRef {
		return nil, source, local.ErrIntegrity
	}
	var cursor PublicationCursor
	if err := read(stage.CursorInput.Port, source.CursorSchemaRef, &cursor); err != nil || cursor.SchemaVersion != PublicationCursorVersion || cursor.SubscriptionID != subscription.ID || cursor.Generation != subscription.Generation || cursor.Position != subscription.Cursor {
		return nil, source, local.ErrIntegrity
	}
	return subscription, source, nil
}

func streamProducerInvocation(r Run, subscription *PublicationSubscription, source flow.PublicationSourceDefinition) (*Invocation, error) {
	consumer := r.Invocations[subscription.InvocationID]
	return publicationProducerInvocation(r, consumer, source)
}

func publicationProducerInvocation(r Run, consumer *Invocation, source flow.PublicationSourceDefinition) (*Invocation, error) {
	if consumer == nil || consumer.BranchID == "" || consumer.CallerActivationID == "" {
		return nil, local.ErrIntegrity
	}
	caller := r.Activations[consumer.CallerActivationID]
	if caller == nil || caller.Kind != "parallel" || caller.InvocationID != consumer.ParentInvocationID {
		return nil, local.ErrIntegrity
	}
	producer := r.Invocations[branchInvocationID(r.ID, caller.ID, source.ProducerBranchID)]
	if producer == nil || producer.CallerActivationID != caller.ID || producer.BranchID != source.ProducerBranchID {
		return nil, local.ErrIntegrity
	}
	return producer, nil
}

func streamPublicationMatches(r Run, subscription *PublicationSubscription, source flow.PublicationSourceDefinition, stepID, hook string) (bool, error) {
	if hook != source.Hook {
		return false, nil
	}
	producer, err := streamProducerInvocation(r, subscription, source)
	if err != nil {
		return false, err
	}
	step := r.Steps[stepID]
	if step == nil {
		return false, local.ErrIntegrity
	}
	activation := r.Activations[step.ActivationID]
	if activation == nil || activation.StepID != step.ID {
		return false, local.ErrIntegrity
	}
	return activation.InvocationID == producer.ID && activation.StageID == source.ProducerStageID, nil
}

func nextStreamDelivery(r Run, subscription *PublicationSubscription, source flow.PublicationSourceDefinition) (streamDeliveryCandidate, bool, error) {
	if subscription.Status != "open" || subscription.PendingAssignmentID != "" {
		return streamDeliveryCandidate{}, false, nil
	}
	publications := []ArtifactPublication{}
	for _, publication := range r.ArtifactPublications {
		matches, err := streamPublicationMatches(r, subscription, source, publication.StepID, publication.Hook)
		if err != nil {
			return streamDeliveryCandidate{}, false, err
		}
		if matches && sourceMatchesPublicationType(source, publication) && (subscription.PublicationStartSequence == nil || publication.AcceptedSequence > *subscription.PublicationStartSequence) {
			publications = append(publications, publication)
		}
	}
	position := subscription.Cursor
	base := PublicationDelivery{
		SchemaVersion: PublicationDeliveryVersion, SubscriptionID: subscription.ID, Generation: subscription.Generation,
		Cursor: publicationCursor(subscription, position), NextCursor: publicationCursor(subscription, position+1),
	}
	if position < int64(len(publications)) {
		publication := publications[position]
		item := publication.Artifact
		base.Kind = "Item"
		base.Publication = &PublicationDeliveryItem{publication.ID, publication.ItemKey, publication.Artifact, publication.SchemaRef}
		return streamDeliveryCandidate{Delivery: base, PublicationID: publication.ID, Item: &item}, true, nil
	}
	if position > int64(len(publications)) {
		return streamDeliveryCandidate{}, false, local.ErrIntegrity
	}
	for _, closure := range r.ArtifactClosures {
		matches, err := streamPublicationMatches(r, subscription, source, closure.StepID, closure.Hook)
		if err != nil {
			return streamDeliveryCandidate{}, false, err
		}
		if matches && (subscription.PublicationStartSequence == nil || closure.AcceptedSequence > *subscription.PublicationStartSequence) {
			base.Kind = "Closed"
			base.Closure = &PublicationDeliveryClosure{closure.ID, closure.Manifest, closure.ItemCount, closure.CutSequence}
			return streamDeliveryCandidate{Delivery: base, ClosureID: closure.ID}, true, nil
		}
	}
	return streamDeliveryCandidate{}, false, nil
}

func interruptedStreamDelivery(subscription *PublicationSubscription, reason string) streamDeliveryCandidate {
	position := subscription.Cursor
	return streamDeliveryCandidate{Delivery: PublicationDelivery{
		SchemaVersion: PublicationDeliveryVersion, Kind: "Interrupted", SubscriptionID: subscription.ID, Generation: subscription.Generation,
		Cursor: publicationCursor(subscription, position), NextCursor: publicationCursor(subscription, position+1), Reason: reason,
	}}
}

func (e *Engine) createStreamAssignment(r *Run, _ *flow.Plan, wait *Activation, subscription *PublicationSubscription, source flow.PublicationSourceDefinition, candidate streamDeliveryCandidate, obs Observation, route, registrationStatus, resolution string) ([]local.EventInput, error) {
	if subscription.PendingAssignmentID != "" || wait.Status != "waiting" || wait.Wait == nil || wait.Wait.Resolution != "" {
		return nil, local.ErrIntegrity
	}
	registration := r.Waits[wait.Wait.RegistrationID]
	if registration == nil || registration.Status != "active" {
		return nil, local.ErrIntegrity
	}
	id := publicationAssignmentID(subscription.ID, subscription.Generation, subscription.Cursor)
	data, err := canonical(candidate.Delivery)
	if err != nil || source.DeliverySchemaRef == nil {
		return nil, local.ErrIntegrity
	}
	artifact, err := e.putArtifact(data, "json", source.DeliverySchemaRef, derivedID("artifact", id, "delivery"),
		map[string]any{"kind": "authority", "authority_id": r.AuthorityID, "command_id": derivedID("command", id), "port": flow.WaitEventPort}, nil, r.registry())
	if err != nil {
		return nil, err
	}
	if err := r.chargeInvocation(wait.InvocationID, 1, 0); err != nil {
		return nil, err
	}
	assignment := PublicationAssignment{
		SchemaVersion: PublicationAssignmentVersion, ID: id, SubscriptionID: subscription.ID, Generation: subscription.Generation,
		Cursor: subscription.Cursor, NextCursor: subscription.Cursor + 1, Kind: candidate.Delivery.Kind,
		PublicationID: candidate.PublicationID, ClosureID: candidate.ClosureID, Item: candidate.Item, Delivery: artifact.Ref(),
		WaitActivationID: wait.ID, BodyInvocationID: wait.InvocationID, Status: "assigned", Assigned: obs,
	}
	r.PublicationAssignments = append(r.PublicationAssignments, assignment)
	subscription.PendingAssignmentID = assignment.ID
	registration.Status = registrationStatus
	wait.Wait.Resolution, wait.Wait.ResolvedAt, wait.Wait.EventRef, wait.Wait.PublicationAssignmentID = resolution, obs.UTC, ptrArtifact(artifact.Ref()), assignment.ID
	wait.Status, wait.Settled = "completed", &obs
	if err := r.advanceInvocation(wait.InvocationID, route); err != nil {
		return nil, err
	}
	event, err := canonical(map[string]any{
		"run_id": r.ID, "stage_activation_id": wait.ID, "stage_id": wait.StageID, "workflow_invocation_id": wait.InvocationID,
		"wait_registration_id": registration.ID, "wait_generation": registration.Generation, "resolution": resolution,
		"publication_assignment_id": assignment.ID, "delivery_kind": assignment.Kind, "delivery_ref": assignment.Delivery,
		"cursor": assignment.Cursor, "next_cursor": assignment.NextCursor, "next_stage_id": route, "observation": obs,
	})
	if err != nil {
		return nil, err
	}
	return []local.EventInput{{Type: "stage.wait_resolved", Version: local.EventVersion, Data: event}}, nil
}

func ptrArtifact(ref ArtifactRef) *ArtifactRef { return &ref }

func (e *Engine) enterPublicationStreamWait(ctx context.Context, loaded Run, view local.ReadView, p *flow.Plan, activation *Activation) error {
	if _, _, err := e.validateStreamWaitInputs(loaded, p, activation); err != nil {
		return err
	}
	stage := p.Workflow.Definition.Stages[activation.StageID]
	commandID := newID("command")
	_, err := e.apply(ctx, e.owner, commandID, loaded.ID, "stage.wait_entered", map[string]any{"stage_activation_id": activation.ID, "registration_id": activation.Wait.RegistrationID}, &view.Snapshot.Version, local.CommandCAS, func(r *Run, _ local.Snapshot, obs Observation) (local.Change, error) {
		wait := r.Activations[activation.ID]
		if err := checkWaitActivation(*r, p, wait, "ready"); err != nil {
			return local.Change{}, err
		}
		subscription, source, err := streamSubscriptionForWaitState(*r, p, wait)
		if err != nil {
			return local.Change{}, err
		}
		registration := r.Waits[wait.Wait.RegistrationID]
		if registration.Status != "reserved" {
			return local.Change{}, local.ErrIntegrity
		}
		if err := r.chargeInvocation(wait.InvocationID, 1, 0); err != nil {
			return local.Change{}, err
		}
		registration.Status = "active"
		if stage.TimeoutSeconds != nil {
			now, err := time.Parse(time.RFC3339Nano, obs.UTC)
			if err != nil {
				return local.Change{}, err
			}
			registration.ExpiresAt = now.Add(time.Duration(*stage.TimeoutSeconds) * time.Second).UTC().Format(time.RFC3339Nano)
		}
		if err := r.setReadyFor(wait.InvocationID, []string{}); err != nil {
			return local.Change{}, err
		}
		if err := r.setInvocationStatus(wait.InvocationID, "waiting", nil); err != nil {
			return local.Change{}, err
		}
		wait.Status = "waiting"
		entered, err := canonical(map[string]any{"run_id": r.ID, "stage_activation_id": wait.ID, "stage_id": wait.StageID, "workflow_invocation_id": wait.InvocationID,
			"wait_registration_id": registration.ID, "wait_generation": registration.Generation, "expires_at": registration.ExpiresAt,
			"event_type": stage.EventType, "source_ref": stage.SourceRef, "subscription_id": subscription.ID, "cursor": subscription.Cursor, "observation": obs})
		if err != nil {
			return local.Change{}, err
		}
		events := []local.EventInput{{Type: "stage.wait_entered", Version: local.EventVersion, Data: entered}}
		candidate, available, err := nextStreamDelivery(*r, subscription, source)
		if err != nil {
			return local.Change{}, err
		}
		if available {
			resolved, err := e.createStreamAssignment(r, p, wait, subscription, source, candidate, obs, stage.OnEvent, "consumed", "event")
			if err != nil {
				return local.Change{}, err
			}
			events = append(events, resolved...)
		}
		return local.Change{RequireStorageBudget: true, Events: events}, nil
	})
	return err
}

// assignPublicationStreams runs inside the publication/close authority commit.
// Every active subscriber gets its own assignment; a pending one is untouched.
func (e *Engine) assignPublicationStreams(r *Run, root *flow.Plan, obs Observation) ([]local.EventInput, error) {
	ids := make([]string, 0, len(r.Activations))
	for id := range r.Activations {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	events := []local.EventInput{}
	for _, id := range ids {
		wait := r.Activations[id]
		if wait == nil || wait.Kind != "wait" || wait.Status != "waiting" {
			continue
		}
		p, err := r.planForCompiled(root, wait.InvocationID)
		if err != nil {
			return nil, err
		}
		stage := p.Workflow.Definition.Stages[wait.StageID]
		declared, ok := p.PublicationSource(stage.SourceRef)
		if !ok || declared.Mode != "each_publication" {
			continue
		}
		subscription, source, err := streamSubscriptionForWaitState(*r, p, wait)
		if err != nil {
			return nil, err
		}
		candidate, available, err := nextStreamDelivery(*r, subscription, source)
		if err != nil {
			return nil, err
		}
		if !available {
			continue
		}
		resolved, err := e.createStreamAssignment(r, p, wait, subscription, source, candidate, obs, stage.OnEvent, "consumed", "event")
		if err != nil {
			return nil, err
		}
		events = append(events, resolved...)
	}
	return events, nil
}

func (e *Engine) resolvePublicationStreamTimeout(ctx context.Context, loaded Run, view local.ReadView, p *flow.Plan, activation *Activation) error {
	subscription, source, err := e.validateStreamWaitInputs(loaded, p, activation)
	if err != nil {
		return err
	}
	stage := p.Workflow.Definition.Stages[activation.StageID]
	commandID := newID("command")
	_, err = e.apply(ctx, e.owner, commandID, loaded.ID, "stage.wait_resolved", map[string]any{"stage_activation_id": activation.ID, "registration_id": activation.Wait.RegistrationID, "next_stage_id": stage.OnTimeout}, &view.Snapshot.Version, local.CommandCAS, func(r *Run, _ local.Snapshot, obs Observation) (local.Change, error) {
		wait := r.Activations[activation.ID]
		if err := checkWaitActivation(*r, p, wait, "waiting"); err != nil {
			return local.Change{}, err
		}
		current, currentSource, err := streamSubscriptionForWaitState(*r, p, wait)
		if err != nil || current.ID != subscription.ID || currentSource.SchemaVersion != source.SchemaVersion {
			return local.Change{}, local.ErrIntegrity
		}
		registration := r.Waits[wait.Wait.RegistrationID]
		if !waitDue(registration, obs.UTC) {
			return local.Change{}, local.Reject("wait_not_due", "the deadline has not passed by the authority's clock")
		}
		events, err := e.createStreamAssignment(r, p, wait, current, currentSource, interruptedStreamDelivery(current, "deadline_elapsed"), obs, stage.OnTimeout, "expired", "interrupted")
		return local.Change{RequireStorageBudget: true, Events: events}, err
	})
	return err
}

func streamSubscriptionForActivation(r Run, activationID string) *PublicationSubscription {
	var found *PublicationSubscription
	for _, subscription := range r.PublicationSubscriptions {
		if subscription != nil && subscription.RepeatActivationID == activationID {
			if found != nil {
				return nil
			}
			found = subscription
		}
	}
	return found
}

func (e *Engine) validateRepeatPublicationCursor(r Run, p *flow.Plan, activation *Activation, inputs map[string]ArtifactRef) error {
	subscription := streamSubscriptionForActivation(r, activation.ID)
	if subscription == nil || subscription.PendingAssignmentID == "" {
		return nil
	}
	var assignment *PublicationAssignment
	for i := range r.PublicationAssignments {
		if r.PublicationAssignments[i].ID == subscription.PendingAssignmentID {
			assignment = &r.PublicationAssignments[i]
		}
	}
	if assignment == nil || assignment.Kind != "Item" {
		return local.ErrIntegrity
	}
	stage := p.Workflow.Definition.Stages[activation.StageID]
	body := p.Repeats[activation.StageID]
	for name, binding := range stage.NextBindings {
		if binding.From != "iteration_output" {
			continue
		}
		output := body.Workflow.Outputs[binding.Port]
		source, ok := body.PublicationSource(subscription.SourceRef)
		if !ok || output.SchemaRef == nil || source.CursorSchemaRef == nil || *output.SchemaRef != *source.CursorSchemaRef {
			continue
		}
		artifact, data, err := e.Artifact(inputs[name])
		if err != nil || artifact.SchemaRef == nil || *artifact.SchemaRef != *source.CursorSchemaRef {
			return errors.New("next subscription cursor artifact is invalid")
		}
		var cursor PublicationCursor
		if decode(data, &cursor) != nil || cursor.SchemaVersion != PublicationCursorVersion || cursor.SubscriptionID != subscription.ID || cursor.Generation != subscription.Generation || cursor.Position != assignment.NextCursor {
			return errors.New("next subscription cursor does not advance the pending assignment")
		}
		return nil
	}
	return errors.New("next subscription cursor binding is absent")
}

func settleRepeatPublicationAssignment(r *Run, p *flow.Plan, activation *Activation, body *Invocation, decision RepeatDecision, obs Observation) error {
	subscription := streamSubscriptionForActivation(*r, activation.ID)
	if subscription == nil || subscription.PendingAssignmentID == "" {
		return nil
	}
	var assignment *PublicationAssignment
	for i := range r.PublicationAssignments {
		if r.PublicationAssignments[i].ID == subscription.PendingAssignmentID {
			assignment = &r.PublicationAssignments[i]
		}
	}
	if assignment == nil || assignment.Status != "assigned" || assignment.BodyInvocationID != body.ID || assignment.Cursor != subscription.Cursor || body.Outcome == nil {
		return local.ErrIntegrity
	}
	processed := false
	switch assignment.Kind {
	case "Item":
		processed = slices.Contains(p.Workflow.Definition.Stages[activation.StageID].ContinueOn, *body.Outcome)
	case "Closed":
		processed = decision.Route != "continue"
		if processed {
			subscription.Status = "closed"
		}
	case "Interrupted":
		processed = decision.Route != "continue"
		if processed {
			subscription.Status = "interrupted"
		}
	}
	if processed {
		assignment.Status, assignment.Processed = "processed", &obs
		subscription.Cursor, subscription.PendingAssignmentID = assignment.NextCursor, ""
	}
	return nil
}

// prepareRepeatBodyInputs is the only materializer for subscription bindings.
// Ordinary body bindings retain the shared path; the two authority values are
// supplied explicitly so no other operator learns a hidden binding source.
func (e *Engine) prepareRepeatBodyInputs(r Run, activation *Activation, bodyID string, body *flow.Plan, bindings map[string]flow.Binding, commandID string) (map[string]ArtifactRef, error) {
	if activation == nil {
		return nil, local.ErrIntegrity
	}
	supplied := []string{}
	for name, binding := range bindings {
		if binding.From == "subscription" {
			supplied = append(supplied, name)
		}
	}
	inputs, err := e.prepareBodyInputs(r, activation.InvocationID, bodyID, body, bindings, commandID, "", supplied...)
	if err != nil {
		return nil, err
	}
	for name, binding := range bindings {
		if binding.From != "subscription" {
			continue
		}
		if binding.SourceRef == nil {
			return nil, errors.New("subscription binding has no source")
		}
		subscription := subscriptionForRepeat(r, activation.ID, *binding.SourceRef)
		source, ok := body.PublicationSource(*binding.SourceRef)
		if subscription == nil || !ok || source.Mode != "each_publication" || subscription.Status != "open" {
			return nil, errors.New("subscription is not ready for a repeat body")
		}
		var value any
		var schema *flow.Ref
		switch binding.Port {
		case "handle":
			value = PublicationSubscriptionHandle{PublicationHandleVersion, subscription.ID, r.ID, subscription.Generation, subscription.SourceRef}
			schema = source.HandleSchemaRef
		case "cursor":
			value = publicationCursor(subscription, subscription.Cursor)
			schema = source.CursorSchemaRef
		default:
			return nil, errors.New("subscription binding port is unsupported")
		}
		if schema == nil {
			return nil, local.ErrIntegrity
		}
		data, err := canonical(value)
		if err != nil {
			return nil, err
		}
		stableCommand := derivedID("command", subscription.ID, binding.Port)
		identity := binding.Port
		if binding.Port == "cursor" {
			identity += "/" + strconv.FormatInt(subscription.Cursor, 10)
		}
		artifact, err := e.putArtifact(data, "json", schema,
			derivedID("artifact", subscription.ID, identity),
			map[string]any{"kind": "authority", "authority_id": r.AuthorityID, "command_id": stableCommand, "port": binding.Port}, nil, body.Registry)
		if err != nil {
			return nil, err
		}
		port, declared := body.Workflow.Inputs[name]
		if !declared {
			return nil, local.ErrIntegrity
		}
		if err := e.validatePortArtifact(body, port.Port, artifact, data); err != nil {
			return nil, err
		}
		inputs[name] = artifact.Ref()
	}
	return inputs, nil
}
