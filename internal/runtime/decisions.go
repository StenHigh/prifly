package runtime

import (
	"encoding/json"
	"errors"
	"regexp"
	"sync/atomic"

	"github.com/stenhigh/prifly/internal/flow"
)

const (
	DecisionDefinitionVersion = "run-decision/1"
	DecisionCatalogVersion    = "run-decision-catalog/1"
	DecisionSheetVersion      = "decision-sheet/1"
	DecisionRequestVersion    = "decision-request/1"
	DecisionAnswerVersion     = "decision-answer/1"
	DecisionRecordVersion     = "decision-record/1"
)

var decisionID = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,127}$`)

// DecisionChoice is one finite answer a package author exposes to a person.
// Value stays typed JSON: display text is never used as a control value.
type DecisionChoice struct {
	ID    string          `json:"id"`
	Title string          `json:"title"`
	Value json.RawMessage `json:"value"`
}

// DecisionDestination names the declared value a decision can supply. It is
// data routing only: it never selects a stage, grants authority or changes a
// Run's scope.
type DecisionDestination struct {
	Kind string `json:"kind"`
	Name string `json:"name,omitempty"`
}

// DecisionCondition restricts a definition to selected values. It is only a
// questionnaire predicate, never a graph predicate.
type DecisionCondition struct {
	Profiles []string                   `json:"profiles"`
	Answers  map[string]json.RawMessage `json:"answers,omitempty"`
}

// DecisionDefinition is the sealed contract for one package decision. It is
// data for the universal Decision Bridge, not an Approval, Grant or route.
type DecisionDefinition struct {
	SchemaVersion  string              `json:"schema_version"`
	ID             string              `json:"id"`
	Title          string              `json:"title"`
	Description    string              `json:"description,omitempty"`
	Phase          string              `json:"phase"`
	Required       bool                `json:"required"`
	Choices        []DecisionChoice    `json:"choices,omitempty"`
	ValueSchema    json.RawMessage     `json:"value_schema,omitempty"`
	Recommendation json.RawMessage     `json:"recommendation,omitempty"`
	Automatic      bool                `json:"automatic"`
	Sensitivity    string              `json:"sensitivity"`
	Destination    DecisionDestination `json:"destination"`
	When           *DecisionCondition  `json:"when,omitempty"`
}

// DecisionCatalog is the canonical data file sealed in a package.
type DecisionCatalog struct {
	SchemaVersion string               `json:"schema_version"`
	Decisions     []DecisionDefinition `json:"decisions"`
}

// DecisionRecord explains what happened to one definition in one Run.
type DecisionRecord struct {
	SchemaVersion    string          `json:"schema_version"`
	DefinitionID     string          `json:"definition_id"`
	DefinitionDigest string          `json:"definition_digest"`
	AttemptID        string          `json:"attempt_id,omitempty"`
	Status           string          `json:"status"`
	Source           string          `json:"source"`
	Value            json.RawMessage `json:"value,omitempty"`
	Observed         *Observation    `json:"observed,omitempty"`
}

// DecisionSheet is the immutable set of preflight choices delivered to a Run.
type DecisionSheet struct {
	SchemaVersion  string           `json:"schema_version"`
	CatalogDigest  string           `json:"catalog_digest"`
	PackageProfile string           `json:"package_profile"`
	ProfileSource  string           `json:"profile_source"`
	DecisionPolicy string           `json:"decision_policy,omitempty"`
	Records        []DecisionRecord `json:"records"`
}

// DecisionRequest is emitted by an executor that supports the universal
// Decision Bridge. It names exactly one declared runtime decision.
type DecisionRequest struct {
	SchemaVersion      string `json:"schema_version"`
	RunID              string `json:"run_id"`
	AttemptID          string `json:"attempt_id"`
	EnvelopeDigest     string `json:"envelope_digest"`
	DecisionID         string `json:"decision_id"`
	DefinitionDigest   string `json:"definition_digest"`
	ExpectedRunVersion int64  `json:"expected_run_version"`
}

// DecisionAnswer is the typed answer to one current DecisionRequest.
type DecisionAnswer struct {
	SchemaVersion      string          `json:"schema_version"`
	RunID              string          `json:"run_id"`
	DecisionID         string          `json:"decision_id"`
	DefinitionDigest   string          `json:"definition_digest"`
	RequestDigest      string          `json:"request_digest"`
	ExpectedRunVersion int64           `json:"expected_run_version"`
	Value              json.RawMessage `json:"value"`
}

func ValidateDecisionDefinition(definition DecisionDefinition) error {
	if definition.SchemaVersion != DecisionDefinitionVersion || !decisionID.MatchString(definition.ID) || definition.Title == "" {
		return errors.New("decision definition requires schema version, id and title")
	}
	if definition.Phase != "preflight" && definition.Phase != "runtime" {
		return errors.New("decision definition phase must be preflight or runtime")
	}
	// A preflight answer is demanded before a Run starts. Nothing can demand a
	// runtime answer: the authority cannot make an executor raise a request and
	// accepts a report that never carried one, so the word would promise a gate
	// that does not exist.
	if definition.Phase == "runtime" && definition.Required {
		return fault("decision_required_unenforceable", "only a preflight decision can be required; a runtime decision is answered when its executor raises a request")
	}
	if definition.Sensitivity != "ordinary" && definition.Sensitivity != "scope-changing" && definition.Sensitivity != "approval-like" {
		return errors.New("decision definition sensitivity is invalid")
	}
	if definition.Destination.Kind != "package_profile" && definition.Destination.Kind != "launch_input" && definition.Destination.Kind != "session_context" {
		return errors.New("decision definition destination is invalid")
	}
	if definition.Destination.Kind == "package_profile" && definition.Destination.Name != "" {
		return errors.New("package profile destination must not name a value")
	}
	if definition.Destination.Kind != "package_profile" && !decisionID.MatchString(definition.Destination.Name) {
		return errors.New("decision definition destination requires a valid value name")
	}
	if len(definition.Choices) == 0 && len(definition.ValueSchema) == 0 {
		return errors.New("decision definition requires choices or value schema")
	}
	seen := map[string]bool{}
	for _, choice := range definition.Choices {
		if !decisionID.MatchString(choice.ID) || choice.Title == "" || len(choice.Value) == 0 || !json.Valid(choice.Value) || seen[choice.ID] {
			return errors.New("decision choices require unique id, title and JSON value")
		}
		seen[choice.ID] = true
	}
	if len(definition.ValueSchema) != 0 && !json.Valid(definition.ValueSchema) {
		return errors.New("decision value schema must be JSON")
	}
	if len(definition.Recommendation) != 0 && !json.Valid(definition.Recommendation) {
		return errors.New("decision recommendation must be JSON")
	}
	if definition.Automatic && (definition.Sensitivity != "ordinary" || len(definition.Recommendation) == 0) {
		return errors.New("automatic decision requires ordinary sensitivity and recommendation")
	}
	if definition.When != nil {
		seen := map[string]bool{}
		for _, profile := range definition.When.Profiles {
			if !decisionID.MatchString(profile) || seen[profile] {
				return errors.New("decision condition requires unique profile names")
			}
			seen[profile] = true
		}
		for id, value := range definition.When.Answers {
			if !decisionID.MatchString(id) || len(value) == 0 || !json.Valid(value) {
				return errors.New("decision condition requires valid predecessor answers")
			}
		}
		if len(seen) == 0 && len(definition.When.Answers) == 0 {
			return errors.New("decision condition requires a profile or predecessor answer")
		}
	}
	return nil
}

func DecisionDefinitionDigest(definition DecisionDefinition) (string, error) {
	data, err := json.Marshal(definition)
	if err != nil {
		return "", err
	}
	return flow.Digest(data)
}

func DecisionCatalogDigest(catalog DecisionCatalog) (string, error) {
	if err := ValidateDecisionCatalog(catalog); err != nil {
		return "", err
	}
	data, err := json.Marshal(catalog)
	if err != nil {
		return "", err
	}
	return flow.Digest(data)
}

func DecisionRequestDigest(request DecisionRequest) (string, error) {
	data, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	return flow.Digest(data)
}

func ValidateDecisionCatalog(catalog DecisionCatalog) error {
	if catalog.SchemaVersion != DecisionCatalogVersion {
		return errors.New("decision catalog schema version is invalid")
	}
	seen := map[string]bool{}
	for _, definition := range catalog.Decisions {
		if err := ValidateDecisionDefinition(definition); err != nil {
			return err
		}
		if seen[definition.ID] {
			return errors.New("decision catalog has duplicate definition ID")
		}
		seen[definition.ID] = true
	}
	return nil
}

func ValidateDecisionValue(definition DecisionDefinition, value json.RawMessage) error {
	decisionValueChecks.Add(1)
	canonical, err := flow.Canonical(value)
	if err != nil {
		return err
	}
	if len(definition.Choices) != 0 {
		for _, choice := range definition.Choices {
			candidate, err := flow.Canonical(choice.Value)
			if err != nil {
				return err
			}
			if string(candidate) == string(canonical) {
				goto schema
			}
		}
		return errors.New("answer is not one of the declared choices")
	}

schema:
	if len(definition.ValueSchema) == 0 {
		return nil
	}
	schemaBytes, err := flow.Canonical(definition.ValueSchema)
	if err != nil {
		return err
	}
	digest, err := flow.Digest(schemaBytes)
	if err != nil {
		return err
	}
	ref := flow.Ref{ID: "prifly:decision/" + definition.ID, Version: "1.0.0", Digest: digest}
	return flow.ValidateSchema(flow.Registry{ref: schemaBytes}, ref, canonical)
}

// decisionValueChecks counts full value validations. Checking a recorded answer
// canonicalizes it and runs its schema; a test reads this to prove that reading
// a Run does not repeat the work its writer already did.
var decisionValueChecks atomic.Int64

// ValidateDecisionSheet checks a sheet completely, including the recorded
// answers. It belongs to the paths that accept an answer - Start, Request and
// Answer - which is where a value first enters the Run.
func ValidateDecisionSheet(catalog DecisionCatalog, sheet DecisionSheet) error {
	return validateDecisionSheet(catalog, sheet, true)
}

// decisionSheetStructure checks everything except the answers themselves. It is
// what reading a Run needs: a recorded answer was validated when it was
// written, and revalidating every answer on every load and every command was
// the single most expensive part of holding decisions in state.
func decisionSheetStructure(catalog DecisionCatalog, sheet DecisionSheet) error {
	return validateDecisionSheet(catalog, sheet, false)
}

func validateDecisionSheet(catalog DecisionCatalog, sheet DecisionSheet, values bool) error {
	if err := ValidateDecisionCatalog(catalog); err != nil {
		return err
	}
	profileValid := sheet.PackageProfile == "" && sheet.ProfileSource == "none" || sheet.PackageProfile != "" && (sheet.ProfileSource == "actor" || sheet.ProfileSource == "project_default" || sheet.ProfileSource == "package_default" || sheet.ProfileSource == "autonomous_policy")
	if sheet.SchemaVersion != DecisionSheetVersion || !profileValid || sheet.DecisionPolicy != "" && sheet.DecisionPolicy != "attended" && sheet.DecisionPolicy != "autonomous" {
		return errors.New("decision sheet has invalid profile selection")
	}
	digest, err := DecisionCatalogDigest(catalog)
	if err != nil || sheet.CatalogDigest != digest {
		return errors.New("decision sheet catalog digest does not match")
	}
	definitions := map[string]DecisionDefinition{}
	for _, definition := range catalog.Decisions {
		definitions[definition.ID] = definition
	}
	seen := map[string]bool{}
	for _, record := range sheet.Records {
		definition, exists := definitions[record.DefinitionID]
		if !exists || record.SchemaVersion != DecisionRecordVersion || seen[record.DefinitionID] {
			return errors.New("decision sheet has invalid record")
		}
		expected, err := DecisionDefinitionDigest(definition)
		if err != nil || record.DefinitionDigest != expected {
			return errors.New("decision sheet record definition digest does not match")
		}
		if record.Status != "answered" && record.Status != "defaulted" && record.Status != "rejected" && record.Status != "presented" && record.Status != "pending" {
			return errors.New("decision sheet record status is invalid")
		}
		if record.Source != "actor" && record.Source != "project_default" && record.Source != "package_default" && record.Source != "autonomous_policy" && record.Source != "unanswered" {
			return errors.New("decision sheet record source is invalid")
		}
		if record.Status == "answered" || record.Status == "defaulted" {
			if record.Source == "unanswered" || len(record.Value) == 0 {
				return errors.New("decision sheet record value is invalid")
			}
			if values && ValidateDecisionValue(definition, record.Value) != nil {
				return errors.New("decision sheet record value is invalid")
			}
		} else if len(record.Value) != 0 || record.Source != "unanswered" {
			return errors.New("unanswered decision sheet record must not carry a value or source")
		}
		seen[record.DefinitionID] = true
	}
	return nil
}
