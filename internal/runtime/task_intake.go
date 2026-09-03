package runtime

import (
	"encoding/json"

	"github.com/stenhigh/prifly/internal/flow"
)

const TaskInputVersion = "task-input/1"

// TaskSource records where the owner or a read-only host obtained TaskInput.
// It is provenance data, not a credential, capability or external authority.
type TaskSource struct {
	Type       string `json:"type"`
	ExternalID string `json:"external_id,omitempty"`
	URL        string `json:"url,omitempty"`
	FetchedAt  string `json:"fetched_at"`
	Version    string `json:"version,omitempty"`
}

// TaskInput is the provider-neutral, pre-Run description of one selected
// task. Its raw text and declared source remain sealed evidence; RunBrief is
// the smaller execution-facing projection.
type TaskInput struct {
	SchemaVersion      string        `json:"schema_version"`
	ID                 string        `json:"id"`
	Title              string        `json:"title"`
	RawText            string        `json:"raw_text"`
	DesiredOutcome     string        `json:"desired_outcome"`
	InScope            []string      `json:"in_scope"`
	OutOfScope         []string      `json:"out_of_scope"`
	CompletionCriteria []string      `json:"completion_criteria"`
	Source             TaskSource    `json:"source"`
	SourceRefs         []ArtifactRef `json:"source_refs"`
	Assumptions        []string      `json:"assumptions"`
	Confirmation       string        `json:"confirmation"`
}

// ParseTaskInput accepts only the closed TaskInput/1 wire form. It does not
// contact the declared source or grant any external permission.
func ParseTaskInput(data []byte) (TaskInput, error) {
	if err := flow.ValidateProtocol("TaskInput", data); err != nil {
		return TaskInput{}, err
	}
	var input TaskInput
	if err := json.Unmarshal(data, &input); err != nil {
		return TaskInput{}, err
	}
	return input, nil
}

// RunBrief projects the selected task without inventing scope, criteria or
// confirmation. Callers validate supplied SourceSnapshot refs separately.
func (input TaskInput) RunBrief(sourceRefs []ArtifactRef) Brief {
	return Brief{
		SchemaVersion: "1", ID: input.ID, Subject: input.Title,
		DesiredOutcome: input.DesiredOutcome, InScope: append([]string{}, input.InScope...),
		OutOfScope:         append([]string{}, input.OutOfScope...),
		CompletionCriteria: append([]string{}, input.CompletionCriteria...),
		SourceRefs:         append([]ArtifactRef{}, sourceRefs...),
		Assumptions:        append([]string{}, input.Assumptions...), Confirmation: input.Confirmation,
	}
}
