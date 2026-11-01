package projectwire

import (
	_ "embed"
	"encoding/json"

	"github.com/stenhigh/prifly/internal/flow"
)

//go:embed build-v3.schema.json
var BuildProvenanceV3Schema []byte

type CompositionPointsDetails struct {
	Owners   []CompositionSourceOwner   `json:"owners"`
	Points   []CompositionPointReview   `json:"points"`
	Variants []CompositionVariantReview `json:"variants"`
}
type CompositionSourceOwner struct {
	Source string `json:"source"`
	Digest string `json:"digest"`
}
type CompositionPointReview struct {
	Export          string            `json:"export"`
	Owner           string            `json:"owner"`
	Name            string            `json:"name"`
	Source          string            `json:"source"`
	Scope           string            `json:"scope"`
	Enabled         bool              `json:"enabled"`
	Target          flow.Ref          `json:"target"`
	Implementations []flow.Ref        `json:"implementations"`
	Consumers       map[string]string `json:"consumers"`
}
type CompositionVariantReview struct {
	Settings    map[string]any `json:"settings"`
	Owner       string         `json:"owner"`
	Source      string         `json:"source"`
	AuthorRef   flow.Ref       `json:"author_ref"`
	VariantKey  string         `json:"variant_key"`
	CompiledRef flow.Ref       `json:"compiled_ref"`
	Scopes      []string       `json:"scopes"`
}

// The optional details are invisible to the frozen /1 reflected wire shape.
// Only an explicit /2 review serializes the additional, separately closed fields.
func (v CompositionReview) MarshalJSON() ([]byte, error) {
	type legacy CompositionReview
	if v.Unified == nil {
		return json.Marshal(legacy(v))
	}
	return json.Marshal(struct {
		legacy
		*CompositionPointsDetails
	}{legacy(v), v.Unified})
}

func (v *CompositionReview) UnmarshalJSON(data []byte) error {
	type legacy CompositionReview
	var wire struct {
		legacy
		CompositionPointsDetails
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	*v = CompositionReview(wire.legacy)
	if v.SchemaVersion == "workflow-composition/2" {
		v.Unified = &wire.CompositionPointsDetails
	}
	return nil
}
