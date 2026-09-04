package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

const sessionWaitingDecision = "waiting_decision"

func decisionDefinition(catalog *DecisionCatalog, id string) (DecisionDefinition, bool) {
	if catalog == nil {
		return DecisionDefinition{}, false
	}
	for _, definition := range catalog.Decisions {
		if definition.ID == id {
			return definition, true
		}
	}
	return DecisionDefinition{}, false
}

func decisionApplies(definition DecisionDefinition, profile string, records []DecisionRecord) bool {
	if definition.When == nil {
		return true
	}
	if len(definition.When.Profiles) != 0 {
		matchesProfile := false
		for _, candidate := range definition.When.Profiles {
			matchesProfile = matchesProfile || candidate == profile
		}
		if !matchesProfile {
			return false
		}
	}
	answers := map[string]json.RawMessage{}
	for _, record := range records {
		if len(record.Value) != 0 {
			answers[record.DefinitionID] = record.Value
		}
	}
	for id, expected := range definition.When.Answers {
		actual, exists := answers[id]
		if !exists {
			return false
		}
		canonicalActual, actualErr := flow.Canonical(actual)
		canonicalExpected, expectedErr := flow.Canonical(expected)
		if actualErr != nil || expectedErr != nil || string(canonicalActual) != string(canonicalExpected) {
			return false
		}
	}
	return true
}

func decisionSessionContext(catalog *DecisionCatalog, sheet *DecisionSheet) map[string]json.RawMessage {
	if catalog == nil || sheet == nil {
		return nil
	}
	values := map[string]json.RawMessage{}
	for _, record := range sheet.Records {
		definition, exists := decisionDefinition(catalog, record.DefinitionID)
		if !exists || definition.Destination.Kind != "session_context" || !decisionApplies(definition, sheet.PackageProfile, sheet.Records) || len(record.Value) == 0 {
			continue
		}
		values[definition.Destination.Name] = append(json.RawMessage(nil), record.Value...)
	}
	if len(values) == 0 {
		return nil
	}
	return values
}

// sealedDecisionAnswer returns the answer the owner gave this decision before
// the Run started, if they gave one. It is the same person the bridge would
// otherwise wait for, so the wait has already been satisfied.
func sealedDecisionAnswer(sheet *DecisionSheet, id string) (json.RawMessage, bool) {
	if sheet == nil {
		return nil, false
	}
	for _, record := range sheet.Records {
		if record.DefinitionID == id && record.Status == "answered" && record.Source == "actor" && len(record.Value) != 0 {
			return record.Value, true
		}
	}
	return nil, false
}

// autonomousBlock reports why an autonomous policy cannot take this decision,
// or the empty string when it can. The condition exists once: a second copy of
// it would drift from the bridge, and a launch would then describe a Run that
// behaves differently.
func autonomousBlock(definition DecisionDefinition) string {
	switch {
	case !definition.Automatic:
		return "automatic_selection_not_allowed"
	case definition.Sensitivity != "ordinary":
		return "sensitivity_above_ordinary"
	case len(definition.Recommendation) == 0:
		return "no_declared_recommendation"
	}
	return ""
}

// UnansweredDecision names one declared runtime decision an autonomous policy
// will not answer, and the declared reason it will not.
type UnansweredDecision struct {
	DecisionID string `json:"decision_id"`
	Reason     string `json:"reason"`
}

// DecisionsAutonomyCannotTake lists, in catalog order, the runtime decisions
// this Run's policy will leave waiting. Under an autonomous policy there is
// nobody to answer them, so a Run that reaches one stops for good; the launch
// says which ones those are before the first dispatch rather than parking
// hours into the work. It reports, it does not refuse: a listed decision may
// never be requested.
func DecisionsAutonomyCannotTake(catalog *DecisionCatalog, sheet *DecisionSheet) []UnansweredDecision {
	blocked := []UnansweredDecision{}
	if catalog == nil || sheet == nil || sheet.DecisionPolicy != "autonomous" {
		return blocked
	}
	for _, definition := range catalog.Decisions {
		if definition.Phase != "runtime" || definition.Destination.Kind != "session_context" || !decisionApplies(definition, sheet.PackageProfile, sheet.Records) {
			continue
		}
		if _, sealed := sealedDecisionAnswer(sheet, definition.ID); sealed {
			continue
		}
		if reason := autonomousBlock(definition); reason != "" {
			blocked = append(blocked, UnansweredDecision{DecisionID: definition.ID, Reason: reason})
		}
	}
	return blocked
}

func decisionRuntimeAvailable(catalog *DecisionCatalog, sheet *DecisionSheet) bool {
	if catalog == nil || sheet == nil {
		return false
	}
	for _, definition := range catalog.Decisions {
		if definition.Phase == "runtime" && decisionApplies(definition, sheet.PackageProfile, sheet.Records) {
			return true
		}
	}
	return false
}

func cloneDecisionContext(values map[string]json.RawMessage) map[string]json.RawMessage {
	if len(values) == 0 {
		return nil
	}
	copy := make(map[string]json.RawMessage, len(values))
	for name, value := range values {
		copy[name] = append(json.RawMessage(nil), value...)
	}
	return copy
}

func decisionInitialLedger(sheet *DecisionSheet, observed Observation) []DecisionRecord {
	if sheet == nil {
		return nil
	}
	ledger := make([]DecisionRecord, len(sheet.Records))
	for index, record := range sheet.Records {
		ledger[index] = record
		ledger[index].Value = append(json.RawMessage(nil), record.Value...)
		ledger[index].Observed = &observed
	}
	return ledger
}

func advanceDecisionDelivery(attempt *Attempt, name string, value json.RawMessage, observed Observation) error {
	if attempt.Session == nil {
		return local.ErrIntegrity
	}
	if attempt.Session.DecisionContext == nil {
		attempt.Session.DecisionContext = map[string]json.RawMessage{}
	}
	attempt.Session.DecisionContext[name] = append(json.RawMessage(nil), value...)
	delivery, err := canonical(map[string]any{"envelope_digest": attempt.EnvelopeDigest, "decision_context": attempt.Session.DecisionContext})
	if err != nil {
		return err
	}
	attempt.EnvelopeDigest = rawDigest(delivery)
	attempt.Session.DeliveryGeneration++
	attempt.Session.HostState, attempt.Session.Handed = SessionAwaiting, observed
	return nil
}

func (e *Engine) RequestDecision(ctx context.Context, request DecisionRequest) (local.ApplyResult, error) {
	if e.ReadOnly {
		return local.ApplyResult{}, local.ErrReadOnly
	}
	if request.SchemaVersion != DecisionRequestVersion || request.RunID == "" || request.AttemptID == "" || request.EnvelopeDigest == "" || !decisionID.MatchString(request.DecisionID) || request.DefinitionDigest == "" {
		return local.ApplyResult{}, errors.New("decision request has invalid identity")
	}
	control, _, err := e.ensureControl(ctx)
	if err != nil {
		return local.ApplyResult{}, err
	}
	if !control.allows(e.owner, "project", e.Config.ID, ControlOperationAdmit) {
		return local.ApplyResult{}, local.Reject("object_access_denied", "the session principal cannot request a decision for this project")
	}
	_, view, err := e.load(ctx, request.RunID)
	if err != nil {
		return local.ApplyResult{}, err
	}
	if request.ExpectedRunVersion != view.Snapshot.Version {
		return local.ApplyResult{}, local.Reject("decision_conflict", "the request does not name the current Run version")
	}
	digest, err := DecisionRequestDigest(request)
	if err != nil {
		return local.ApplyResult{}, err
	}
	commandID := derivedID("command", request.RunID, "decision-request", digest)
	return e.apply(ctx, e.owner, commandID, request.RunID, "decision.requested", request, &view.Snapshot.Version, local.CommandCAS, func(r *Run, _ local.Snapshot, observed Observation) (local.Change, error) {
		if r.PendingDecision != nil {
			return local.Change{}, local.Reject("decision_pending", "this Run already awaits one declared decision")
		}
		definition, exists := decisionDefinition(r.DecisionCatalog, request.DecisionID)
		if !exists || definition.Phase != "runtime" || definition.Destination.Kind != "session_context" || r.DecisionSheet == nil || !decisionApplies(definition, r.DecisionSheet.PackageProfile, r.DecisionLedger) {
			return local.Change{}, local.Reject("unknown_decision", "the request does not name an active runtime session-context decision")
		}
		definitionDigest, err := DecisionDefinitionDigest(definition)
		if err != nil || definitionDigest != request.DefinitionDigest {
			return local.Change{}, local.Reject("decision_conflict", "the request definition differs from the sealed catalog")
		}
		attempt := r.Attempts[request.AttemptID]
		if attempt == nil || attempt.Session == nil || attempt.Session.SchemaVersion != AssistedSessionDecisionVersion || attempt.Session.PrincipalID != e.owner || attempt.Session.HostState != SessionAwaiting || attempt.EnvelopeDigest != request.EnvelopeDigest {
			return local.Change{}, local.Reject("decision_request_unsupported", "this attempt is not an awaiting decision-bridge session delivery")
		}
		// The owner's own answer, given before the Run started, outranks any
		// policy default: waiting for them is what the wait was for.
		if sealed, exists := sealedDecisionAnswer(r.DecisionSheet, definition.ID); exists {
			value, err := flow.Canonical(sealed)
			if err != nil || ValidateDecisionValue(definition, value) != nil {
				return local.Change{}, local.Reject("invalid_decision_default", "the sealed owner answer is not a declared value of this decision")
			}
			if err := advanceDecisionDelivery(attempt, definition.Destination.Name, value, observed); err != nil {
				return local.Change{}, err
			}
			r.DecisionLedger = append(r.DecisionLedger, DecisionRecord{SchemaVersion: DecisionRecordVersion, DefinitionID: definition.ID, DefinitionDigest: definitionDigest, AttemptID: attempt.ID, Status: "answered", Source: "actor", Value: value, Observed: &observed})
			data, err := canonical(map[string]any{"request": request, "request_digest": digest, "observation": observed, "source": "actor"})
			return local.Change{Events: []local.EventInput{{Type: "decision.answered", Version: 1, Data: data}}}, err
		}
		if r.DecisionSheet.DecisionPolicy == "autonomous" && autonomousBlock(definition) == "" {
			value, err := flow.Canonical(definition.Recommendation)
			if err != nil || ValidateDecisionValue(definition, value) != nil {
				return local.Change{}, local.Reject("invalid_decision_default", "the declared automatic recommendation is invalid")
			}
			if err := advanceDecisionDelivery(attempt, definition.Destination.Name, value, observed); err != nil {
				return local.Change{}, err
			}
			r.DecisionLedger = append(r.DecisionLedger, DecisionRecord{SchemaVersion: DecisionRecordVersion, DefinitionID: definition.ID, DefinitionDigest: definitionDigest, AttemptID: attempt.ID, Status: "defaulted", Source: "autonomous_policy", Value: value, Observed: &observed})
			data, err := canonical(map[string]any{"request": request, "request_digest": digest, "observation": observed, "source": "autonomous_policy"})
			return local.Change{Events: []local.EventInput{{Type: "decision.defaulted", Version: 1, Data: data}}}, err
		}
		r.PendingDecision = &request
		attempt.Session.HostState = sessionWaitingDecision
		r.DecisionLedger = append(r.DecisionLedger, DecisionRecord{SchemaVersion: DecisionRecordVersion, DefinitionID: definition.ID, DefinitionDigest: definitionDigest, AttemptID: attempt.ID, Status: "pending", Source: "unanswered", Observed: &observed})
		data, err := canonical(map[string]any{"request": request, "request_digest": digest, "observation": observed})
		return local.Change{Events: []local.EventInput{{Type: "decision.requested", Version: 1, Data: data}}}, err
	})
}

func (e *Engine) AnswerDecision(ctx context.Context, answer DecisionAnswer) (local.ApplyResult, error) {
	if e.ReadOnly {
		return local.ApplyResult{}, local.ErrReadOnly
	}
	if answer.SchemaVersion != DecisionAnswerVersion || answer.RunID == "" || !decisionID.MatchString(answer.DecisionID) || answer.DefinitionDigest == "" || answer.RequestDigest == "" || len(answer.Value) == 0 {
		return local.ApplyResult{}, errors.New("decision answer has invalid identity")
	}
	canonicalValue, err := flow.Canonical(answer.Value)
	if err != nil {
		return local.ApplyResult{}, local.Reject("invalid_decision_answer", "the answer is not canonicalizable JSON")
	}
	control, _, err := e.ensureControl(ctx)
	if err != nil {
		return local.ApplyResult{}, err
	}
	if !control.allows(e.owner, "project", e.Config.ID, ControlOperationAdmit) {
		return local.ApplyResult{}, local.Reject("object_access_denied", "the session principal cannot answer a decision for this project")
	}
	_, view, err := e.load(ctx, answer.RunID)
	if err != nil {
		return local.ApplyResult{}, err
	}
	if answer.ExpectedRunVersion != view.Snapshot.Version {
		return local.ApplyResult{}, local.Reject("decision_conflict", "the answer does not name the current Run version")
	}
	// Version and value are part of the command identity: a retry of the same
	// answer replays, a corrected value after a schema refusal is a new command
	// judged by the pending request, and a late answer for a settled request is
	// reported as not pending instead of colliding with the earlier command.
	commandID := derivedID("command", answer.RunID, "decision-answer", answer.RequestDigest, strconv.FormatInt(answer.ExpectedRunVersion, 10), rawDigest(canonicalValue))
	return e.apply(ctx, e.owner, commandID, answer.RunID, "decision.answered", answer, &view.Snapshot.Version, local.CommandCAS, func(r *Run, _ local.Snapshot, observed Observation) (local.Change, error) {
		request := r.PendingDecision
		if request == nil {
			return local.Change{}, local.Reject("decision_not_pending", "this Run has no pending decision")
		}
		requestDigest, err := DecisionRequestDigest(*request)
		if err != nil || requestDigest != answer.RequestDigest || request.RunID != answer.RunID || request.DecisionID != answer.DecisionID || request.DefinitionDigest != answer.DefinitionDigest {
			return local.Change{}, local.Reject("decision_conflict", "the answer does not name the pending request")
		}
		definition, exists := decisionDefinition(r.DecisionCatalog, answer.DecisionID)
		if !exists || definition.Phase != "runtime" || definition.Destination.Kind != "session_context" {
			return local.Change{}, local.Reject("unknown_decision", "the answer does not name a runtime session-context decision")
		}
		if err := ValidateDecisionValue(definition, canonicalValue); err != nil {
			return local.Change{}, local.Reject("invalid_decision_answer", err.Error())
		}
		attempt := r.Attempts[request.AttemptID]
		if attempt == nil || attempt.Session == nil || attempt.Session.SchemaVersion != AssistedSessionDecisionVersion || attempt.Session.HostState != sessionWaitingDecision || attempt.EnvelopeDigest != request.EnvelopeDigest {
			return local.Change{}, local.Reject("decision_conflict", "the pending request no longer owns this session delivery")
		}
		found := false
		for index := range r.DecisionLedger {
			record := &r.DecisionLedger[index]
			if record.DefinitionID == answer.DecisionID && record.AttemptID == attempt.ID && record.Status == "pending" {
				record.Status, record.Source, record.Value, record.Observed = "answered", "actor", append(json.RawMessage(nil), canonicalValue...), &observed
				found = true
				break
			}
		}
		if !found {
			return local.Change{}, local.ErrIntegrity
		}
		if err := advanceDecisionDelivery(attempt, definition.Destination.Name, canonicalValue, observed); err != nil {
			return local.Change{}, err
		}
		r.PendingDecision = nil
		data, err := canonical(map[string]any{"answer": answer, "request_digest": requestDigest, "observation": observed})
		return local.Change{Events: []local.EventInput{{Type: "decision.answered", Version: 1, Data: data}}}, err
	})
}
