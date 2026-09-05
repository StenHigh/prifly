package main

import (
	"encoding/json"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// Decision destinations bind ordinary launch data. The same input validation
// still owns schemas and configuration scope; a decision grants no new rights.
func projectDecisionInputs(preflight projectPreflight, inputs map[string]json.RawMessage, refs map[string]prifly.ArtifactRef, neutral bool) error {
	answers := map[string]json.RawMessage{}
	for _, record := range preflight.Sheet.Records {
		if record.Status == "answered" || record.Status == "defaulted" {
			answers[record.DefinitionID] = record.Value
		}
	}
	selected := map[string]json.RawMessage{}
	for _, definition := range preflight.Catalog.Decisions {
		if definition.Destination.Kind != "launch_input" {
			continue
		}
		if definition.Phase != "preflight" {
			return usageError("project_start_unsupported_decision_input: launch_input requires a preflight decision")
		}
		value, answered := answers[definition.ID]
		if !answered || !projectDecisionApplies(definition, preflight.PackageProfile, answers) {
			continue
		}
		if !neutral {
			return usageError("project_start_unsupported_decision_input: launch_input answers require Project profile /3")
		}
		port := definition.Destination.Name
		_, explicitValue := inputs[port]
		_, explicitRef := refs[port]
		_, anotherDecision := selected[port]
		if explicitValue || explicitRef || anotherDecision {
			return usageError("project_start_duplicate_input: decision and another source both bind " + port)
		}
		selected[port] = append(json.RawMessage(nil), value...)
	}
	// Do not partially merge answers when a later destination conflicts.
	for port, value := range selected {
		inputs[port] = value
	}
	return nil
}
