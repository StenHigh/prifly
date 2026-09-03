package runtime

import (
	"sort"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// interruptTerminalPublicationFailures runs in the same authority transition
// that made a producer terminal. A source must opt in: old contracts continue
// to wait until their declared deadline, rather than changing history's route.
func (e *Engine) interruptTerminalPublicationFailures(r *Run, obs Observation) ([]local.EventInput, error) {
	if !isPublicationFailureState(r.SchemaVersion) {
		return nil, nil
	}
	root, err := r.plan()
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(r.Activations))
	for id := range r.Activations {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	events := []local.EventInput{}
	for _, id := range ids {
		wait := r.Activations[id]
		if wait == nil || wait.Kind != "wait" || wait.Status != "waiting" || wait.Wait == nil {
			continue
		}
		plan, err := r.planForCompiled(root, wait.InvocationID)
		if err != nil {
			return nil, err
		}
		stage := plan.Workflow.Definition.Stages[wait.StageID]
		source, published := plan.PublicationSource(stage.SourceRef)
		if !published || source.ProducerFailure != "interrupt_on_terminal_failure" {
			continue
		}
		if stage.OnTimeout == "" {
			return nil, local.ErrIntegrity
		}
		if source.Mode == "once" {
			producer, err := publicationProducerInvocation(*r, r.Invocations[wait.InvocationID], source)
			if err != nil {
				return nil, err
			}
			if producer.Status != "failed" {
				continue
			}
			resolved, err := interruptOncePublicationWait(r, plan, wait, stage, producer, obs)
			if err != nil {
				return nil, err
			}
			events = append(events, resolved)
			continue
		}
		if source.Mode != "each_publication" {
			return nil, local.ErrIntegrity
		}
		subscription, current, err := streamSubscriptionForWaitState(*r, plan, wait)
		if err != nil || current.SchemaVersion != source.SchemaVersion {
			return nil, local.ErrIntegrity
		}
		producer, err := streamProducerInvocation(*r, subscription, current)
		if err != nil {
			return nil, err
		}
		if producer.Status != "failed" {
			continue
		}
		resolved, err := e.createStreamAssignment(r, plan, wait, subscription, current, interruptedStreamDelivery(subscription, "producer_terminal_failed"), obs, stage.OnTimeout, "interrupted", "interrupted")
		if err != nil {
			return nil, err
		}
		events = append(events, resolved...)
	}
	return events, nil
}

func interruptOncePublicationWait(r *Run, plan *flow.Plan, wait *Activation, stage flow.Stage, producer *Invocation, obs Observation) (local.EventInput, error) {
	if err := checkWaitActivation(*r, plan, wait, "waiting"); err != nil {
		return local.EventInput{}, err
	}
	registration := r.Waits[wait.Wait.RegistrationID]
	if registration == nil || registration.Status != "active" || wait.Wait.Resolution != "" || producer == nil || producer.Status != "failed" {
		return local.EventInput{}, local.ErrIntegrity
	}
	if err := r.chargeInvocation(wait.InvocationID, 1, 0); err != nil {
		return local.EventInput{}, err
	}
	registration.Status = "interrupted"
	wait.Wait.Resolution, wait.Wait.ResolvedAt = "producer_failed", obs.UTC
	wait.Status, wait.Settled = "completed", &obs
	if err := r.advanceInvocation(wait.InvocationID, stage.OnTimeout); err != nil {
		return local.EventInput{}, err
	}
	data, err := canonical(map[string]any{
		"run_id": r.ID, "stage_activation_id": wait.ID, "stage_id": wait.StageID,
		"workflow_invocation_id": wait.InvocationID, "wait_registration_id": registration.ID,
		"wait_generation": registration.Generation, "resolution": "producer_failed",
		"producer_workflow_invocation_id": producer.ID, "next_stage_id": stage.OnTimeout, "observation": obs,
	})
	if err != nil {
		return local.EventInput{}, err
	}
	return local.EventInput{Type: "stage.wait_resolved", Version: local.EventVersion, Data: data}, nil
}
