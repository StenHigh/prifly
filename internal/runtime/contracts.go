package runtime

import (
	_ "embed"
	"encoding/json"
	"slices"

	"github.com/stenhigh/prifly/internal/flow"
)

//go:embed public.schema.json
var publicContracts []byte

//go:embed core-public.schema.json
var corePublicContracts []byte

//go:embed choice-decision.schema.json
var choiceContracts []byte

//go:embed invocations.schema.json
var invocationPublicContracts []byte

//go:embed repeats.schema.json
var repeatPublicContracts []byte

//go:embed contexts.schema.json
var contextPublicContracts []byte

//go:embed sessions.schema.json
var sessionPublicContracts []byte

//go:embed waivers.schema.json
var waiverPublicContracts []byte

//go:embed parallel.schema.json
var parallelPublicContracts []byte

// Everything below this line ships the same way the bundles above do. A bundle
// that is generated and drift-checked but never embedded is a contract the tool
// announces and then cannot produce; three had accumulated that way before a
// test compared the files on disk against what this function can reach.
//
//go:embed map.schema.json
var mapPublicContracts []byte

//go:embed wait.schema.json
var waitPublicContracts []byte

//go:embed guards.schema.json
var guardPublicContracts []byte

//go:embed reported-cost.schema.json
var reportedCostPublicContracts []byte

//go:embed artifact-publication.schema.json
var artifactPublicationPublicContracts []byte

//go:embed artifact-closure.schema.json
var artifactClosurePublicContracts []byte

//go:embed publication-subscription.schema.json
var publicationSubscriptionPublicContracts []byte

//go:embed publication-checks.schema.json
var publicationChecksPublicContracts []byte

//go:embed publication-new-only.schema.json
var publicationNewOnlyPublicContracts []byte

//go:embed publication-failure.schema.json
var publicationFailurePublicContracts []byte

//go:embed action-intent.schema.json
var actionIntentPublicContracts []byte

//go:embed action-admission.schema.json
var actionAdmissionPublicContracts []byte

//go:embed action-grant-admission.schema.json
var actionGrantAdmissionPublicContracts []byte

//go:embed action-delivery.schema.json
var actionDeliveryPublicContracts []byte

//go:embed fork.schema.json
var forkPublicContracts []byte

//go:embed workspace.schema.json
var workspacePublicContracts []byte

//go:embed workspace-tree.schema.json
var workspaceTreePublicContracts []byte

//go:embed decision-state.schema.json
var decisionStatePublicContracts []byte

//go:embed run-decisions.schema.json
var runDecisionPublicContracts []byte

// PublicSchema selects a named contract from its versioned public bundle.
// Baseline contracts (including RunSnapshot v1) remain in flow.ProtocolSchema.
func PublicSchema(name string) ([]byte, error) {
	for _, content := range [][]byte{publicContracts, corePublicContracts, choiceContracts, invocationPublicContracts, repeatPublicContracts, contextPublicContracts, sessionPublicContracts, waiverPublicContracts, parallelPublicContracts, mapPublicContracts, waitPublicContracts, guardPublicContracts, reportedCostPublicContracts, artifactPublicationPublicContracts, artifactClosurePublicContracts, publicationSubscriptionPublicContracts, publicationChecksPublicContracts, publicationNewOnlyPublicContracts, publicationFailurePublicContracts, actionIntentPublicContracts, actionAdmissionPublicContracts, actionGrantAdmissionPublicContracts, actionDeliveryPublicContracts, forkPublicContracts, workspacePublicContracts, workspaceTreePublicContracts, decisionStatePublicContracts, runDecisionPublicContracts} {
		var bundle map[string]json.RawMessage
		if err := json.Unmarshal(content, &bundle); err != nil {
			return nil, err
		}
		var names []string
		if err := json.Unmarshal(bundle["x-prifly-contracts"], &names); err != nil {
			return nil, err
		}
		if !slices.Contains(names, name) {
			continue
		}
		ref, _ := json.Marshal("#/$defs/" + name)
		bundle["$ref"] = ref
		return json.Marshal(bundle)
	}
	return nil, &flow.Problem{Code: "unsupported_contract", Message: "Unknown explicitly selected public contract"}
}
