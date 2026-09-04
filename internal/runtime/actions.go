package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// ActionIntent is the sealed proposal for one logical operation. Parsing it
// does not admit, deliver, or authorize the operation.
type ActionIntent struct {
	SchemaVersion      string                          `json:"schema_version"`
	ID                 string                          `json:"id"`
	RunID              string                          `json:"run_id"`
	StepInstanceID     string                          `json:"step_instance_id"`
	OriginatingAttempt string                          `json:"originating_attempt_id"`
	OperationID        string                          `json:"operation_id"`
	ToolRef            flow.Ref                        `json:"tool_ref"`
	Operation          string                          `json:"operation"`
	ArgumentsSchemaRef flow.Ref                        `json:"arguments_schema_ref"`
	Arguments          json.RawMessage                 `json:"arguments"`
	Targets            []ResourceIdentity              `json:"targets"`
	InputArtifacts     []ArtifactRef                   `json:"input_artifacts"`
	ExpectedOutputs    map[string]ActionExpectedOutput `json:"expected_outputs"`
	EffectClass        string                          `json:"effect_class"`
	RetryClass         string                          `json:"retry_class"`
	Preconditions      []ActionPrecondition            `json:"preconditions"`
	DispatchNotAfter   string                          `json:"dispatch_not_after"`
}

type ActionExpectedOutput struct {
	Contract    json.RawMessage   `json:"contract"`
	Destination *ResourceIdentity `json:"destination,omitempty"`
}

type ActionPrecondition struct {
	Resource        ResourceIdentity `json:"resource"`
	ExpectedVersion string           `json:"expected_version"`
}

// ActionIntentRecord is the authority-sealed proposal that later Admission
// must name. Proposal records have no delivery status because this transition
// never contacts an adapter or target.
type ActionIntentRecord struct {
	Intent   ActionIntent     `json:"intent"`
	Tool     PinnedDefinition `json:"tool_definition"`
	Digest   string           `json:"digest"`
	Proposed Observation      `json:"proposed"`
	Actor    string           `json:"actor_id"`
}

// ActionAdmission is the durable decision for one sealed ActionIntent. It
// records authority approval consumption only; ActionDelivery remains a later
// boundary before any adapter or target can be contacted.
type ActionAdmission struct {
	SchemaVersion string      `json:"schema_version"`
	ID            string      `json:"id"`
	IntentID      string      `json:"intent_id"`
	IntentDigest  string      `json:"intent_digest"`
	ApprovalRefs  []string    `json:"approval_refs"`
	GrantRefs     []string    `json:"grant_refs"`
	Admitted      Observation `json:"admitted"`
	Actor         string      `json:"actor_id"`
}

// ActionDelivery is the authority record written before an adapter may contact
// a target. This slice creates only prepared deliveries, so it cannot claim an
// external effect or make recovery resend possible.
type ActionDelivery struct {
	SchemaVersion    string       `json:"schema_version"`
	ID               string       `json:"id"`
	OperationID      string       `json:"operation_id"`
	OwningAttemptID  string       `json:"owning_attempt_id"`
	AdmissionID      string       `json:"admission_id"`
	Ordinal          int64        `json:"ordinal"`
	ExecutorRef      flow.Ref     `json:"executor_ref"`
	DeliveryStatus   string       `json:"delivery_status"`
	EffectStatus     *string      `json:"effect_status"`
	DispatchNotAfter string       `json:"dispatch_not_after"`
	ReceiptRef       *EvidenceRef `json:"receipt_ref,omitempty"`
}

type ProposeActionCommand struct {
	SchemaVersion      string       `json:"schema_version"`
	CommandID          string       `json:"command_id"`
	RunID              string       `json:"run_id"`
	ExpectedRunVersion int64        `json:"expected_run_version"`
	Payload            ActionIntent `json:"payload"`
}

type AdmitActionPayload struct {
	IntentID     string   `json:"intent_id"`
	IntentDigest string   `json:"intent_digest"`
	ApprovalRefs []string `json:"approval_refs"`
	GrantRefs    []string `json:"grant_refs"`
}

type AdmitActionCommand struct {
	SchemaVersion      string             `json:"schema_version"`
	CommandID          string             `json:"command_id"`
	RunID              string             `json:"run_id"`
	ExpectedRunVersion int64              `json:"expected_run_version"`
	Payload            AdmitActionPayload `json:"payload"`
}

// ParseActionIntent accepts only the closed published contract. Runtime
// ownership, current controls and tool qualification are checked at admission.
func ParseActionIntent(data []byte) (ActionIntent, error) {
	if err := flow.ValidateProtocol("ActionIntent", data); err != nil {
		return ActionIntent{}, err
	}
	var intent ActionIntent
	if err := decode(data, &intent); err != nil {
		return ActionIntent{}, err
	}
	return intent, nil
}

func ParseProposeActionCommand(data []byte) (ProposeActionCommand, error) {
	if err := flow.ValidateProtocol("ProposeActionCommand", data); err != nil {
		return ProposeActionCommand{}, err
	}
	var command ProposeActionCommand
	if err := decode(data, &command); err != nil {
		return ProposeActionCommand{}, err
	}
	return command, nil
}

func ParseAdmitActionCommand(data []byte) (AdmitActionCommand, error) {
	if err := flow.ValidateProtocol("AdmitActionCommand", data); err != nil {
		return AdmitActionCommand{}, err
	}
	var command AdmitActionCommand
	if err := decode(data, &command); err != nil {
		return AdmitActionCommand{}, err
	}
	return command, nil
}

func validateProposeActionCommand(command ProposeActionCommand) (ActionIntent, []byte, error) {
	data, err := canonical(command)
	if err != nil {
		return ActionIntent{}, nil, err
	}
	parsed, err := ParseProposeActionCommand(data)
	if err != nil || parsed.SchemaVersion != command.SchemaVersion || parsed.CommandID != command.CommandID || parsed.RunID != command.RunID || parsed.ExpectedRunVersion != command.ExpectedRunVersion {
		return ActionIntent{}, nil, local.Reject("invalid_action_intent", "proposal must satisfy the closed ActionIntent command contract")
	}
	intentData, err := canonical(parsed.Payload)
	if err != nil {
		return ActionIntent{}, nil, err
	}
	intent, err := ParseActionIntent(intentData)
	if err != nil {
		return ActionIntent{}, nil, local.Reject("invalid_action_intent", "proposal must satisfy the closed ActionIntent contract")
	}
	return intent, intentData, nil
}

func validateAdmitActionCommand(command AdmitActionCommand) error {
	data, err := canonical(command)
	if err != nil {
		return err
	}
	parsed, err := ParseAdmitActionCommand(data)
	if err != nil {
		return local.Reject("invalid_action_admission", "admission must satisfy the closed ActionAdmission command contract")
	}
	reencoded, err := canonical(parsed)
	if err != nil || string(reencoded) != string(data) {
		return local.Reject("invalid_action_admission", "admission must satisfy the closed ActionAdmission command contract")
	}
	return nil
}

func (e *Engine) actionTool(ref flow.Ref) (PinnedDefinition, flow.ToolDescriptor, flow.Registry, error) {
	definitions, registry, _, err := e.inventoryResources()
	if err != nil {
		return PinnedDefinition{}, flow.ToolDescriptor{}, nil, err
	}
	for _, definition := range definitions {
		if definition.Ref != ref || definition.Kind != "tool" || rawDigest(definition.Bytes) != ref.Digest {
			continue
		}
		descriptor, err := flow.ParseToolDescriptor(definition.Bytes)
		if err == nil && descriptor.ID == ref.ID && descriptor.Version == ref.Version {
			return definition, descriptor, registry, nil
		}
	}
	return PinnedDefinition{}, flow.ToolDescriptor{}, nil, local.Reject("tool_not_found", "action proposal must name an installed sealed tool descriptor")
}

// proposalContract is the pinned step contract a proposal was checked against.
// It is read before the command is applied, so the transform compares instead
// of compiling the workflow inside the transaction.
type proposalContract struct {
	ActivationID string
	StageID      string
	EffectClass  string
	RetryClass   string
}

func (e *Engine) proposalStepContract(r Run, intent ActionIntent) (proposalContract, error) {
	attempt := r.Attempts[intent.OriginatingAttempt]
	if attempt == nil {
		return proposalContract{}, local.Reject("action_forbidden", "proposal does not belong to an active Step Attempt")
	}
	activation := r.Activations[attempt.ActivationID]
	if activation == nil {
		return proposalContract{}, local.Reject("action_forbidden", "proposal does not belong to an active Step Attempt")
	}
	plan, err := r.planFor(activation.InvocationID)
	if err != nil {
		return proposalContract{}, err
	}
	step := plan.Steps[activation.StageID]
	return proposalContract{ActivationID: activation.ID, StageID: activation.StageID, EffectClass: step.Effects.Class, RetryClass: step.Effects.RetryClass}, nil
}

func (r Run) actionProposalAttempt(principal string, intent ActionIntent, descriptor flow.ToolDescriptor, contract proposalContract, observation Observation) (*Attempt, error) {
	if !isActionIntentState(r.SchemaVersion) {
		return nil, local.Reject("unsupported_action_intent", "this Run was created before durable action proposals")
	}
	attempt := r.Attempts[intent.OriginatingAttempt]
	if attempt == nil || intent.RunID != r.ID || attempt.StepID != intent.StepInstanceID || !slices.Contains(r.Active, attempt.ID) {
		return nil, local.Reject("action_forbidden", "proposal does not belong to an active Step Attempt")
	}
	activation := r.Activations[attempt.ActivationID]
	if activation == nil || attempt.ActivationID != activation.ID {
		return nil, local.Reject("action_forbidden", "proposal does not belong to an active Step Attempt")
	}
	if attempt.Session == nil || attempt.Session.PrincipalID != principal || attempt.Session.HostState != SessionAwaiting {
		return nil, local.Reject("action_forbidden", "only the current assisted host may propose an action")
	}
	if err := assistedReportAdmissible(attempt.Admitted, attempt.Deadline, observation); err != nil {
		return nil, err
	}
	if r.HasUnresolvedEffects || r.admissionsBlockedFor(activation.InvocationID) || r.cancelRequestedFor(activation.InvocationID) {
		return nil, local.Reject("dispatch_blocked", "current controls or an unresolved effect forbid a new action proposal")
	}
	if activation.ID != contract.ActivationID || activation.StageID != contract.StageID {
		return nil, local.Reject("action_forbidden", "proposal no longer belongs to the step it was prepared for")
	}
	if descriptor.AdapterRef != assistedAdapter(r.Definitions) || descriptor.Operation != intent.Operation || descriptor.ArgumentsSchemaRef != intent.ArgumentsSchemaRef || descriptor.EffectClass != intent.EffectClass || descriptor.RetryClass != intent.RetryClass || contract.EffectClass != intent.EffectClass || contract.RetryClass != intent.RetryClass {
		return nil, local.Reject("action_contract_conflict", "proposal differs from its pinned tool or owning step contract")
	}
	return attempt, nil
}

func actionIntentInvariant(r Run) error {
	if !isActionIntentState(r.SchemaVersion) {
		if len(r.ActionIntents) != 0 || len(r.ActionAdmissions) != 0 || len(r.ActionDeliveries) != 0 {
			return errors.New("action intent invariant: older state contains action proposals")
		}
		return nil
	}
	operations := map[string]bool{}
	for id, record := range r.ActionIntents {
		data, err := canonical(record.Intent)
		if err != nil || record.Intent.ID != id || record.Tool.Kind != "tool" || record.Tool.Ref != record.Intent.ToolRef || rawDigest(record.Tool.Bytes) != record.Tool.Ref.Digest || record.Digest != rawDigest(data) || record.Actor == "" {
			return errors.New("action intent invariant: invalid sealed proposal")
		}
		descriptor, descriptorErr := flow.ParseToolDescriptor(record.Tool.Bytes)
		if _, err := ParseActionIntent(data); err != nil || descriptorErr != nil || descriptor.ID != record.Tool.Ref.ID || descriptor.Version != record.Tool.Ref.Version || descriptor.Operation != record.Intent.Operation || descriptor.ArgumentsSchemaRef != record.Intent.ArgumentsSchemaRef || descriptor.EffectClass != record.Intent.EffectClass || descriptor.RetryClass != record.Intent.RetryClass || operations[record.Intent.OperationID] {
			return errors.New("action intent invariant: invalid or duplicate logical operation")
		}
		operations[record.Intent.OperationID] = true
	}
	if !isActionAdmissionState(r.SchemaVersion) && len(r.ActionAdmissions) != 0 {
		return errors.New("action admission invariant: older state contains admissions")
	}
	if !isActionDeliveryState(r.SchemaVersion) && len(r.ActionDeliveries) != 0 {
		return errors.New("action delivery invariant: older state contains deliveries")
	}
	for intentID, admission := range r.ActionAdmissions {
		intent, exists := r.ActionIntents[intentID]
		if !exists || admission.SchemaVersion != "1" || admission.ID == "" || admission.IntentID != intentID || admission.IntentDigest != intent.Digest || admission.Actor == "" || hasDuplicate(admission.ApprovalRefs) || hasDuplicate(admission.GrantRefs) || (!isActionGrantAdmissionState(r.SchemaVersion) && len(admission.GrantRefs) != 0) || len(admission.GrantRefs) > 1 {
			return errors.New("action admission invariant: invalid admission")
		}
		if isActionDeliveryState(r.SchemaVersion) {
			descriptor, err := flow.ParseToolDescriptor(intent.Tool.Bytes)
			if err != nil {
				return errors.New("action delivery invariant: invalid tool descriptor")
			}
			delivery, present := r.ActionDeliveries[admission.ID]
			if !present || delivery.SchemaVersion != "1" || delivery.ID == "" || delivery.OperationID != intent.Intent.OperationID || delivery.OwningAttemptID != intent.Intent.OriginatingAttempt || delivery.AdmissionID != admission.ID || delivery.Ordinal != 1 || delivery.ExecutorRef != descriptor.AdapterRef || delivery.DeliveryStatus != "prepared" || delivery.EffectStatus == nil || *delivery.EffectStatus != "not_started" || delivery.DispatchNotAfter != intent.Intent.DispatchNotAfter || delivery.ReceiptRef != nil {
				return errors.New("action delivery invariant: invalid prepared delivery")
			}
		}
	}
	return nil
}

func hasDuplicate(values []string) bool {
	seen := map[string]bool{}
	for _, value := range values {
		if value == "" || seen[value] {
			return true
		}
		seen[value] = true
	}
	return false
}

// ProposeSessionAction records one exact operation selected by the current
// assisted host. It deliberately does not admit, dispatch or execute it.
func (e *Engine) ProposeSessionAction(ctx context.Context, command ProposeActionCommand) (local.ApplyResult, error) {
	if e.ReadOnly {
		return local.ApplyResult{}, local.ErrReadOnly
	}
	intent, intentData, err := validateProposeActionCommand(command)
	if err != nil {
		return local.ApplyResult{}, err
	}
	control, _, err := e.ensureControl(ctx)
	if err != nil {
		return local.ApplyResult{}, err
	}
	if !control.allows(e.owner, "project", e.Config.ID, ControlOperationAdmit) {
		return local.ApplyResult{}, local.Reject("object_access_denied", "the session principal cannot propose an action for this project")
	}
	// Store owns exact-command idempotency. Do not reread a mutable registry
	// merely to return a receipt whose decision it has already retained.
	if _, err := e.Store.LookupReceipt(ctx, e.owner, command.CommandID); err == nil {
		return e.apply(ctx, e.owner, command.CommandID, command.RunID, "action.intent_proposed", command, &command.ExpectedRunVersion, local.CommandCAS, func(*Run, local.Snapshot, Observation) (local.Change, error) {
			return local.Change{}, local.ErrIntegrity
		})
	} else if !errors.Is(err, local.ErrNotFound) {
		return local.ApplyResult{}, err
	}
	tool, descriptor, registry, err := e.actionTool(intent.ToolRef)
	if err != nil {
		return local.ApplyResult{}, err
	}
	if err := flow.ValidateSchema(registry, descriptor.ArgumentsSchemaRef, intent.Arguments); err != nil {
		return local.ApplyResult{}, local.Reject("action_argument_invalid", "action arguments do not satisfy the sealed tool schema")
	}
	packagePin, packageBlocked, err := e.packageAdmissionGate(ctx, []flow.Ref{intent.ToolRef}, true)
	if err != nil {
		return local.ApplyResult{}, err
	}
	for _, ref := range intent.InputArtifacts {
		if _, _, err := e.Artifact(ref); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return local.ApplyResult{}, local.Reject("action_input_unavailable", "action input artifacts must name sealed available bytes")
			}
			return local.ApplyResult{}, err
		}
	}
	// The owning step's contract is read here; the transform then only checks
	// that the proposal still belongs to that exact activation and stage.
	proposed, _, err := e.load(ctx, command.RunID)
	if err != nil {
		return local.ApplyResult{}, err
	}
	contract, contractErr := e.proposalStepContract(proposed, intent)
	digest := rawDigest(intentData)
	pins := []local.ControlPin(nil)
	if packagePin != nil {
		pins = append(pins, *packagePin)
	}
	return e.applyControlledWithPins(ctx, nil, pins, e.owner, command.CommandID, command.RunID, "action.intent_proposed", command, &command.ExpectedRunVersion, local.CommandCAS, func(r *Run, _ local.Snapshot, observation Observation) (local.Change, error) {
		if packageBlocked != nil {
			return local.Change{}, packageBlocked
		}
		if contractErr != nil {
			return local.Change{}, contractErr
		}
		if _, err := r.actionProposalAttempt(e.owner, intent, descriptor, contract, observation); err != nil {
			return local.Change{}, err
		}
		if prior, exists := r.ActionIntents[intent.ID]; exists {
			if prior.Digest != digest {
				return local.Change{}, local.Reject("action_intent_conflict", "intent ID already names different operation bytes")
			}
			data, err := canonical(map[string]any{"action_intent": prior})
			return local.Change{ReceiptOnly: true, Result: data}, err
		}
		for _, prior := range r.ActionIntents {
			if prior.Intent.OperationID == intent.OperationID {
				return local.Change{}, local.Reject("operation_conflict", "logical operation ID already belongs to another action intent")
			}
		}
		if r.ActionIntents == nil {
			r.ActionIntents = map[string]ActionIntentRecord{}
		}
		record := ActionIntentRecord{Intent: intent, Tool: tool, Digest: digest, Proposed: observation, Actor: e.owner}
		r.ActionIntents[intent.ID] = record
		event, err := canonical(map[string]any{"action_intent": record})
		if err != nil {
			return local.Change{}, fmt.Errorf("action intent event: %w", err)
		}
		result, err := canonical(map[string]any{"action_intent": record})
		return local.Change{Result: result, Events: []local.EventInput{{Type: "action.intent_proposed", Version: 1, Data: event}}}, err
	})
}

// AdmitSessionAction consumes approvals and records the durable admission for
// one previously sealed proposal. It deliberately does not dispatch, deliver
// or execute the tool.
func (e *Engine) AdmitSessionAction(ctx context.Context, command AdmitActionCommand) (local.ApplyResult, error) {
	if e.ReadOnly {
		return local.ApplyResult{}, local.ErrReadOnly
	}
	if err := validateAdmitActionCommand(command); err != nil {
		return local.ApplyResult{}, err
	}
	if len(command.Payload.GrantRefs) > 1 {
		return local.ApplyResult{}, local.Reject("unsupported_action_grant_composition", "action admission consumes at most one exact resource-scoped grant")
	}
	pin, blocked, err := e.admissionGate(ctx)
	if err != nil {
		return local.ApplyResult{}, err
	}
	var packagePin *local.ControlPin
	admitted := proposalContract{}
	var contractErr error
	if run, _, loadErr := e.load(ctx, command.RunID); loadErr == nil {
		if intent, exists := run.ActionIntents[command.Payload.IntentID]; exists {
			admitted, contractErr = e.proposalStepContract(run, intent.Intent)
			var packageBlocked error
			packagePin, packageBlocked, err = e.packageAdmissionGate(ctx, []flow.Ref{intent.Tool.Ref}, false)
			if err != nil {
				return local.ApplyResult{}, err
			}
			if blocked == nil {
				blocked = packageBlocked
			}
		}
	} else if !errors.Is(loadErr, local.ErrNotFound) {
		return local.ApplyResult{}, loadErr
	}
	pins := []local.ControlPin(nil)
	if packagePin != nil {
		pins = append(pins, *packagePin)
	}
	admissionID := derivedID("action-admission", e.owner, command.CommandID)
	var targets []ResourceIdentity
	return e.applyControlledWithControlMutation(ctx, pin, pins, func(snapshot local.AuthoritySnapshot, observation Observation) (json.RawMessage, error) {
		control, err := decodeControl(snapshot.Data)
		if err != nil {
			return nil, err
		}
		if err := e.controlCompatible(control); err != nil {
			return nil, err
		}
		if _, err := consumeApprovals(&control, "action.admit", command.Payload.IntentDigest, admissionID, command.Payload.ApprovalRefs, observation); err != nil {
			return nil, err
		}
		if len(command.Payload.GrantRefs) == 1 {
			if err := consumeGrant(&control, e.owner, "action.admit", command.Payload.IntentDigest, admissionID, command.Payload.GrantRefs[0], targets, observation); err != nil {
				return nil, err
			}
		}
		return canonicalState(control)
	}, e.owner, command.CommandID, command.RunID, "action.admitted", command, &command.ExpectedRunVersion, local.CommandCAS, func(r *Run, _ local.Snapshot, observation Observation) (local.Change, error) {
		if blocked != nil {
			return local.Change{}, blocked
		}
		if !isActionAdmissionState(r.SchemaVersion) {
			return local.Change{}, local.Reject("unsupported_action_admission", "this Run was created before durable action admissions")
		}
		if len(command.Payload.GrantRefs) != 0 && !isActionGrantAdmissionState(r.SchemaVersion) {
			return local.Change{}, local.Reject("unsupported_action_grant", "this Run was created before action grant admissions")
		}
		intent, exists := r.ActionIntents[command.Payload.IntentID]
		if !exists || intent.Digest != command.Payload.IntentDigest {
			return local.Change{}, local.Reject("action_intent_not_found", "admission must name one retained ActionIntent and its exact digest")
		}
		descriptor, err := flow.ParseToolDescriptor(intent.Tool.Bytes)
		if err != nil {
			return local.Change{}, local.ErrIntegrity
		}
		if contractErr != nil {
			return local.Change{}, contractErr
		}
		if _, err := r.actionProposalAttempt(e.owner, intent.Intent, descriptor, admitted, observation); err != nil {
			return local.Change{}, err
		}
		targets = append([]ResourceIdentity{}, intent.Intent.Targets...)
		if prior, exists := r.ActionAdmissions[command.Payload.IntentID]; exists {
			if prior.IntentDigest != command.Payload.IntentDigest || !slices.Equal(prior.ApprovalRefs, command.Payload.ApprovalRefs) || !slices.Equal(prior.GrantRefs, command.Payload.GrantRefs) {
				return local.Change{}, local.Reject("action_admission_conflict", "the ActionIntent already has a different admission")
			}
			data, err := canonical(map[string]any{"action_admission": prior})
			return local.Change{ReceiptOnly: true, Result: data}, err
		}
		if r.ActionAdmissions == nil {
			r.ActionAdmissions = map[string]ActionAdmission{}
		}
		admission := ActionAdmission{SchemaVersion: "1", ID: admissionID, IntentID: command.Payload.IntentID, IntentDigest: command.Payload.IntentDigest, ApprovalRefs: append([]string{}, command.Payload.ApprovalRefs...), GrantRefs: append([]string{}, command.Payload.GrantRefs...), Admitted: observation, Actor: e.owner}
		r.ActionAdmissions[command.Payload.IntentID] = admission
		if isActionDeliveryState(r.SchemaVersion) {
			if r.ActionDeliveries == nil {
				r.ActionDeliveries = map[string]ActionDelivery{}
			}
			notStarted := "not_started"
			r.ActionDeliveries[admission.ID] = ActionDelivery{SchemaVersion: "1", ID: derivedID("action-delivery", e.owner, command.CommandID), OperationID: intent.Intent.OperationID, OwningAttemptID: intent.Intent.OriginatingAttempt, AdmissionID: admission.ID, Ordinal: 1, ExecutorRef: descriptor.AdapterRef, DeliveryStatus: "prepared", EffectStatus: &notStarted, DispatchNotAfter: intent.Intent.DispatchNotAfter}
		}
		event, err := canonical(map[string]any{"action_admission": admission})
		if err != nil {
			return local.Change{}, fmt.Errorf("action admission event: %w", err)
		}
		result, err := canonical(map[string]any{"action_admission": admission})
		return local.Change{Result: result, Events: []local.EventInput{{Type: "action.admitted", Version: 1, Data: event}}}, err
	})
}
