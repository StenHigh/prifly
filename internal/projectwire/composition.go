package projectwire

import _ "embed"

//go:embed build-v2.schema.json
var BuildProvenanceV2Schema []byte

// CompositionReview describes source preparation, never a runtime lookup.
// Files pins all source bytes, including inherited and imported dependencies.
type CompositionReview struct {
	SchemaVersion string                   `json:"schema_version"`
	Parents       []string                 `json:"parents"`
	Sources       []string                 `json:"sources"`
	Files         map[string]string        `json:"files"`
	Imports       map[string]string        `json:"imports"`
	Layers        []string                 `json:"layers"`
	Contributions []string                 `json:"contributions"`
	Groups        []CompositionGroupReview `json:"groups"`
}

type CompositionGroupReview struct {
	ID      string   `json:"id"`
	Mode    string   `json:"mode"`
	Members []string `json:"members"`
}
