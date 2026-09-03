package runtime

import (
	"embed"
	"encoding/json"
	"slices"

	"github.com/stenhigh/prifly/internal/flow"
)

// The authoring schemas describe the YAML an author writes, not the wire this
// engine speaks. They are published as files for editors, and embedded here so
// an installed binary can answer for them too: an author who has only the
// binary has no repository to read.
//
//go:embed authoring/*.json
var authoringSchemas embed.FS

type authoringDocument struct {
	Document string   `json:"document"`
	ID       string   `json:"id"`
	Path     string   `json:"path"`
	Patterns []string `json:"patterns"`
}

func authoringManifest() ([]authoringDocument, error) {
	data, err := authoringSchemas.ReadFile("authoring/manifest.json")
	if err != nil {
		return nil, err
	}
	var manifest struct {
		Schemas []authoringDocument `json:"schemas"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	return manifest.Schemas, nil
}

// AuthoringSchemaNames lists the authoring documents AuthoringSchema answers
// for, under the same names the editor manifest uses.
func AuthoringSchemaNames() ([]string, error) {
	documents, err := authoringManifest()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(documents))
	for _, document := range documents {
		names = append(names, document.Document)
	}
	slices.Sort(names)
	return names, nil
}

// AuthoringSchema returns the schema of one authoring document by its name.
func AuthoringSchema(name string) ([]byte, error) {
	documents, err := authoringManifest()
	if err != nil {
		return nil, err
	}
	for _, document := range documents {
		if document.Document != name {
			continue
		}
		return authoringSchemas.ReadFile("authoring/" + document.Path)
	}
	return nil, &flow.Problem{Code: "unsupported_contract", Message: "unknown authoring document"}
}
