package runtime

import (
	"slices"
	"sort"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

func sourceMatchesPublicationType(source flow.PublicationSourceDefinition, publication ArtifactPublication) bool {
	port := source.ArtifactPort()
	return publication.Format == port.Format && (port.Format != "blob" || slices.Contains(port.MediaTypes, publication.MediaType))
}

// publicationWait matches one compiled once-source to the exact sibling
// producer instance named by the durable wait registration.
func publicationWait(r *Run, root *flow.Plan, registration *WaitRegistration, publication ArtifactPublication) (*Activation, flow.Stage, bool, error) {
	plan, err := r.planForCompiled(root, registration.InvocationID)
	if err != nil {
		return nil, flow.Stage{}, false, err
	}
	source, subscribed := plan.PublicationSource(registration.SourceRef)
	if !subscribed || source.Mode != "once" || publication.Hook != source.Hook || publication.ItemKey != source.ItemKey || publication.SchemaRef != source.HookSchemaRef || publication.Consumption != "early" || !sourceMatchesPublicationType(source, publication) {
		return nil, flow.Stage{}, false, nil
	}
	if source.Initial == "new_only" {
		if registration.PublicationStartSequence == nil || publication.AcceptedSequence <= *registration.PublicationStartSequence {
			return nil, flow.Stage{}, false, nil
		}
	} else if registration.PublicationStartSequence != nil {
		return nil, flow.Stage{}, false, local.ErrIntegrity
	}
	consumer := r.Invocations[registration.InvocationID]
	if consumer == nil || consumer.BranchID == "" || consumer.CallerActivationID == "" {
		return nil, flow.Stage{}, false, local.ErrIntegrity
	}
	caller := r.Activations[consumer.CallerActivationID]
	if caller == nil || caller.Kind != "parallel" || caller.InvocationID != consumer.ParentInvocationID {
		return nil, flow.Stage{}, false, local.ErrIntegrity
	}
	producer := r.Invocations[branchInvocationID(r.ID, caller.ID, source.ProducerBranchID)]
	if producer == nil || producer.CallerActivationID != caller.ID || producer.BranchID != source.ProducerBranchID {
		return nil, flow.Stage{}, false, nil
	}
	producerStep := r.Steps[publication.StepID]
	if producerStep == nil {
		return nil, flow.Stage{}, false, local.ErrIntegrity
	}
	producerActivation := r.Activations[producerStep.ActivationID]
	if producerActivation == nil || producerActivation.StepID != publication.StepID {
		return nil, flow.Stage{}, false, local.ErrIntegrity
	}
	if producerActivation.InvocationID != producer.ID || producerActivation.StageID != source.ProducerStageID {
		return nil, flow.Stage{}, false, nil
	}
	wait := r.Activations[registration.ActivationID]
	stage, exists := plan.Workflow.Definition.Stages[registration.TargetStageID]
	if wait == nil || wait.ID != registration.ActivationID || wait.InvocationID != registration.InvocationID || wait.StageID != registration.TargetStageID || !exists || stage.Kind != "wait" || stage.SourceRef != registration.SourceRef || stage.EventSchemaRef != registration.EventSchemaRef {
		return nil, flow.Stage{}, false, local.ErrIntegrity
	}
	return wait, stage, true, nil
}

func publicationDeliveryEvent(registration *WaitRegistration, publication ArtifactPublication, obs Observation, disposition string) InboxEvent {
	eventID := derivedID("event", publication.ID, registration.ID)
	return InboxEvent{Envelope: EventEnvelope{
		SchemaVersion: EventEnvelopeVersion, SourceRef: registration.SourceRef,
		EventID: eventID, EventType: "artifact.published", PayloadSchemaRef: registration.EventSchemaRef,
		PayloadRef: publication.Artifact, RunID: registration.RunID, ActivationID: registration.ActivationID,
		Generation: registration.Generation, Nonce: registration.Nonce, ReceivedAt: obs.UTC,
	}, RegistrationID: registration.ID, Disposition: disposition, Observed: obs}
}

// assignPublicationToWait gives each matching registration its own delivery
// identity. An active wait is resolved in this same transaction; a reserved
// wait retains the immutable item until the graph reaches it.
func assignPublicationToWait(r *Run, root *flow.Plan, registration *WaitRegistration, publication ArtifactPublication, obs Observation) ([]local.EventInput, bool, error) {
	wait, stage, matches, err := publicationWait(r, root, registration, publication)
	if err != nil || !matches || registration.Status != "reserved" && registration.Status != "active" {
		return nil, false, err
	}
	eventID := derivedID("event", publication.ID, registration.ID)
	existingIndex := -1
	for i, existing := range r.Inbox {
		if existing.Envelope.EventID == eventID {
			if existing.RegistrationID != registration.ID || existing.Envelope.PayloadRef != publication.Artifact {
				return nil, false, local.ErrIntegrity
			}
			existingIndex = i
			break
		}
	}
	if existingIndex >= 0 && (registration.Status != "active" || r.Inbox[existingIndex].Disposition != "held") {
		return nil, false, nil
	}
	if existingIndex < 0 && len(r.Inbox) >= MaxInboxEvents {
		return nil, false, local.Reject("inbox_full", "publication delivery would exceed the run inbox limit")
	}
	disposition := "held"
	if registration.Status == "active" {
		disposition = "consumed"
	}
	events := []local.EventInput{}
	if existingIndex < 0 {
		delivery := publicationDeliveryEvent(registration, publication, obs, disposition)
		r.Inbox = append(r.Inbox, delivery)
		received, err := canonical(map[string]any{
			"run_id": r.ID, "wait_registration_id": registration.ID, "event_id": eventID,
			"disposition": disposition, "reason": "", "envelope": delivery.Envelope, "observation": obs,
		})
		if err != nil {
			return nil, false, err
		}
		events = append(events, local.EventInput{Type: "wait.event_received", Version: local.EventVersion, Data: received})
	}
	if registration.Status == "reserved" {
		return events, true, nil
	}
	if wait.Status != "waiting" || wait.Wait == nil || wait.Wait.Resolution != "" || r.Invocations[wait.InvocationID].Status != "waiting" {
		return nil, false, local.ErrIntegrity
	}
	if err := r.chargeInvocation(wait.InvocationID, 1, 0); err != nil {
		return nil, false, err
	}
	registration.Status = "consumed"
	if existingIndex >= 0 {
		r.Inbox[existingIndex].Disposition = "consumed"
	}
	ref := publication.Artifact
	wait.Wait.Resolution, wait.Wait.ResolvedAt, wait.Wait.EventRef = "event", obs.UTC, &ref
	wait.Status, wait.Settled = "completed", &obs
	if err := r.advanceInvocation(wait.InvocationID, stage.OnEvent); err != nil {
		return nil, false, err
	}
	resolved, err := canonical(map[string]any{
		"run_id": r.ID, "stage_activation_id": wait.ID, "stage_id": wait.StageID,
		"workflow_invocation_id": wait.InvocationID, "wait_registration_id": registration.ID,
		"wait_generation": registration.Generation, "resolution": "event", "event_id": eventID,
		"payload_ref": ref, "received_at": obs.UTC, "next_stage_id": stage.OnEvent, "observation": obs,
	})
	if err != nil {
		return nil, false, err
	}
	return append(events, local.EventInput{Type: "stage.wait_resolved", Version: local.EventVersion, Data: resolved}), true, nil
}

func assignPublicationToWaits(r *Run, root *flow.Plan, publication ArtifactPublication, obs Observation) ([]local.EventInput, error) {
	ids := make([]string, 0, len(r.Waits))
	for id := range r.Waits {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	events := []local.EventInput{}
	for _, id := range ids {
		assigned, _, err := assignPublicationToWait(r, root, r.Waits[id], publication, obs)
		if err != nil {
			return nil, err
		}
		events = append(events, assigned...)
	}
	return events, nil
}

func assignRetainedPublication(r *Run, root *flow.Plan, registration *WaitRegistration, obs Observation) ([]local.EventInput, bool, error) {
	plan, err := r.planForCompiled(root, registration.InvocationID)
	if err != nil {
		return nil, false, err
	}
	if source, subscribed := plan.PublicationSource(registration.SourceRef); subscribed && source.Initial == "new_only" {
		return nil, false, nil
	}
	for _, publication := range r.ArtifactPublications {
		events, assigned, err := assignPublicationToWait(r, root, registration, publication, obs)
		if err != nil || assigned {
			return events, assigned, err
		}
	}
	return nil, false, nil
}
