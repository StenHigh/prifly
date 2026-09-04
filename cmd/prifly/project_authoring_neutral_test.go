package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func authoringReferenceJSON(t *testing.T, name string) []byte {
	t.Helper()
	source, err := os.ReadFile(filepath.Join("..", "..", "examples", "authoring", name+"-authoring-reference.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	value, err := flow.Parse(source, "yaml")
	if err != nil {
		t.Fatalf("reference must be one valid YAML document: %v", err)
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func validateAuthoringForTest(t *testing.T, name string, data []byte) error {
	t.Helper()
	schema, err := prifly.AuthoringSchema(name)
	if err != nil {
		t.Fatal(err)
	}
	schema, err = flow.Canonical(schema)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := flow.Digest(schema)
	if err != nil {
		t.Fatal(err)
	}
	ref := flow.Ref{ID: "test:schema/authoring", Version: "1.0.0", Digest: digest}
	return flow.ValidateSchema(flow.Registry{ref: schema}, ref, data)
}

func TestNeutralAuthoringReferencesMatchServedSchemas(t *testing.T) {
	for _, example := range []struct{ name, schema string }{
		{"check", "check-v1"},
		{"execution-bindings", "project-workflow-folder-v1"},
		{"project-profile", "project-profile-v3"},
	} {
		t.Run(example.name, func(t *testing.T) {
			data := authoringReferenceJSON(t, example.name)
			if err := validateAuthoringForTest(t, example.schema, data); err != nil {
				t.Fatalf("annotated reference rejected by served editor schema: %v", err)
			}
		})
	}
}

func TestCheckAuthoringSchemaMatchesFullDefinition(t *testing.T) {
	base := authoringReferenceJSON(t, "check")
	for _, test := range []struct {
		name  string
		valid bool
		edit  func(map[string]any)
	}{
		{"content", true, func(map[string]any) {}},
		{"result", true, func(v map[string]any) { v["kind"], v["claim"] = "result", "check_passed" }},
		{"semantic", true, func(v map[string]any) { v["kind"], v["claim"] = "result", "semantic_review" }},
		{"missing", false, func(v map[string]any) { delete(v, "title") }},
		{"null", false, func(v map[string]any) { v["executor"] = nil }},
		{"unknown", false, func(v map[string]any) { v["authoring"] = "prifly-check/1" }},
		{"claim", false, func(v map[string]any) { v["claim"] = "semantic_review" }},
		{"kind", false, func(v map[string]any) { v["kind"] = "step" }},
		{"empty-title", false, func(v map[string]any) { v["title"] = "" }},
		{"long-title", false, func(v map[string]any) { v["title"] = strings.Repeat("я", 257) }},
		{"identifier", false, func(v map[string]any) { v["id"] = "invalid id" }},
		{"version", false, func(v map[string]any) { v["version"] = "latest" }},
		{"unknown-executor-field", false, func(v map[string]any) { v["executor"].(map[string]any)["command"] = "sh" }},
		{"missing-adapter", false, func(v map[string]any) { delete(v["executor"].(map[string]any), "adapter_ref") }},
		{"adapter-digest", false, func(v map[string]any) {
			v["executor"].(map[string]any)["adapter_ref"].(map[string]any)["digest"] = "latest"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var value map[string]any
			if err := json.Unmarshal(base, &value); err != nil {
				t.Fatal(err)
			}
			test.edit(value)
			data, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateAuthoringForTest(t, "check-v1", data); (err == nil) != test.valid {
				t.Fatalf("editor: valid=%v err=%v", test.valid, err)
			}
			if _, err := flow.ParseCheckDefinition(data); (err == nil) != test.valid {
				t.Fatalf("full definition: valid=%v err=%v", test.valid, err)
			}
		})
	}
	// Only the source editor accepts a whole-scalar reference placeholder;
	// compilation must resolve it before the full definition is admitted.
	var source map[string]any
	if err := json.Unmarshal(base, &source); err != nil {
		t.Fatal(err)
	}
	source["executor"].(map[string]any)["adapter_ref"] = "{{check_adapter}}"
	data, _ := json.Marshal(source)
	if err := validateAuthoringForTest(t, "check-v1", data); err != nil {
		t.Fatalf("source placeholder: %v", err)
	}
	if _, err := flow.ParseCheckDefinition(data); err == nil {
		t.Fatal("unresolved source placeholder accepted as a sealed definition")
	}
}
