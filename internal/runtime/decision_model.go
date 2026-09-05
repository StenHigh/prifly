package runtime

import "errors"

func isDecisionState(version string) bool { return atLeast(version, CoreDecisionStateVersion) }

func hasDecisionStateFields(r Run) bool {
	return r.DecisionCatalog != nil || r.DecisionSheet != nil || len(r.DecisionLedger) != 0 || r.PendingDecision != nil
}

func decisionInvariant(r Run) error {
	if isNeutralState(r.SchemaVersion) && !hasDecisionStateFields(r) {
		return nil
	}
	if !isDecisionState(r.SchemaVersion) {
		if hasDecisionStateFields(r) {
			return errors.New("decision invariant: older state carries decision fields")
		}
		return nil
	}
	if r.DecisionCatalog == nil || r.DecisionSheet == nil {
		return errors.New("decision invariant: decision state requires catalog and sheet")
	}
	// Reading a Run checks the shape of what is recorded, not the answers: a
	// recorded answer was validated by the command that accepted it.
	if err := decisionSheetStructure(*r.DecisionCatalog, *r.DecisionSheet); err != nil {
		return err
	}
	pending := 0
	for _, record := range r.DecisionLedger {
		definition, exists := decisionDefinition(r.DecisionCatalog, record.DefinitionID)
		timedRecord := record.SchemaVersion == DecisionRecordTimingVersion
		if !exists || (record.SchemaVersion != DecisionRecordVersion && !timedRecord) || record.DefinitionDigest == "" || record.Status == "" || record.Source == "" {
			return errors.New("decision invariant: ledger record is invalid")
		}
		if timedRecord != timedSession(r.Attempts[record.AttemptID]) || timedRecord && !isTimingState(r.SchemaVersion) {
			return errors.New("decision invariant: record edition differs from its session owner")
		}
		closed := record.Status == "cancelled" || record.Status == "expired"
		if record.ClosureReason != "" && (!timedRecord || !closed) {
			return errors.New("decision invariant: closure reason requires a closed timed record")
		}
		digest, err := DecisionDefinitionDigest(definition)
		if err != nil || digest != record.DefinitionDigest {
			return errors.New("decision invariant: ledger definition differs from catalog")
		}
		if record.Status == "pending" {
			pending++
			if record.Source != "unanswered" || len(record.Value) != 0 {
				return errors.New("decision invariant: pending record carries an answer")
			}
		} else if record.Status == "answered" || record.Status == "defaulted" {
			if len(record.Value) == 0 {
				return errors.New("decision invariant: recorded answer is invalid")
			}
		} else if record.Status == "rejected" || closed {
			if len(record.Value) != 0 || record.Source != "unanswered" || record.Observed == nil || closed && (!timedRecord || record.ClosureReason == "") {
				return errors.New("decision invariant: a closed question cannot invent an answer")
			}
		} else {
			return errors.New("decision invariant: unsupported ledger status")
		}
	}
	if r.PendingDecision == nil {
		if pending != 0 {
			return errors.New("decision invariant: ledger has an unowned pending record")
		}
		return nil
	}
	request := r.PendingDecision
	timedRequest := request.SchemaVersion == DecisionRequestTimingVersion
	if pending != 1 || (request.SchemaVersion != DecisionRequestVersion && !timedRequest) || timedRequest != request.YieldExecution {
		return errors.New("decision invariant: pending request does not match ledger")
	}
	if timedRequest != timedSession(r.Attempts[request.AttemptID]) || timedRequest && !isTimingState(r.SchemaVersion) {
		return errors.New("decision invariant: request edition differs from its session owner")
	}
	return nil
}
