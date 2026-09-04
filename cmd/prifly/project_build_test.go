package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func TestProjectBuildDeterminismAndDataBoundaries(t *testing.T) {
	source := projectPackageSource{ID: "test:package/build", Version: "1.0.0", Description: "Inspect", RequiresCoreProtocol: "1"}
	options := projectWorkflowOptions{Settings: map[string]map[string]any{}, Exclude: []string{}}
	values := map[string]any{"label": "Inspect"}
	external := flow.Ref{ID: "core:adapter/assisted-session", Version: "1.0.0", Digest: "sha256:" + strings.Repeat("e", 64)}
	makeComponent := func(kind, id string, object map[string]any) projectCompileComponent {
		t.Helper()
		object["id"], object["version"] = id, "1.0.0"
		data, err := projectCanonicalJSON(object)
		if err != nil {
			t.Fatal(err)
		}
		digest, err := flow.Digest(data)
		if err != nil {
			t.Fatal(err)
		}
		return projectCompileComponent{Kind: kind, Ref: flow.Ref{ID: id, Version: "1.0.0", Digest: digest}, Bytes: data, Path: "old/path.json"}
	}
	contextBytes := []byte("Exact instructions.\r\n")
	contextRef := flow.Ref{ID: "test:context/inspect", Version: "1.0.0", Digest: fmt.Sprintf("sha256:%x", sha256.Sum256(contextBytes))}
	context := projectCompileComponent{Kind: "context", Ref: contextRef, Bytes: contextBytes, Resource: &flow.ContextResource{ByteEncoding: "utf8_text", MediaType: "text/plain", Bytes: contextBytes}}
	// Ref-shaped JSON instance data must not be mistaken for a definition edge.
	literal := projectRefValue(contextRef)
	schema := makeComponent("schema", "test:schema/result", map[string]any{"const": literal, "default": literal})
	step := makeComponent("step", "test:step/inspect", map[string]any{
		"instructions_ref": literal, "result_schema_ref": projectRefValue(schema.Ref),
		"executor": map[string]any{"adapter_ref": projectRefValue(external), "operation": "session"},
	})
	root := makeComponent("workflow", "test:workflow/root", map[string]any{
		"inputs": map[string]any{"plan": map[string]any{"format": "json", "required": false, "schema_ref": projectRefValue(schema.Ref), "configuration": map[string]any{"scope": "project", "default": literal}}},
		"stages": map[string]any{"default": map[string]any{"kind": "step", "step_ref": projectRefValue(step.Ref), "input_bindings": map[string]any{"value": map[string]any{"from": "literal", "value": literal}}}},
	})
	root.Root = true
	original := []projectCompileComponent{root, context, step, schema}
	build := func(source projectPackageSource, components []projectCompileComponent, profile string, values map[string]any, options projectWorkflowOptions) ([]projectCompileComponent, *projectBuildProvenance) {
		t.Helper()
		result, provenance, err := projectBuildVariant(source, components, profile, values, options)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := projectCanonicalJSON(provenance)
		if err != nil {
			t.Fatal(err)
		}
		if err := validateProjectBuild(encoded, result); err != nil {
			t.Fatal(err)
		}
		return result, provenance
	}
	compiled, baseline := build(source, original, "default", values, options)
	if baseline.BuildKey != "sha256:858eca4b122e4bd822811bbfa78d0d5ffc1fad6ae3d8af8f5267df8d10108779" {
		t.Fatalf("b1 baseline changed: %s", baseline.BuildKey)
	}
	// This full encoding vector also guards the 64-character Version ceiling.
	if got := projectBuildVersionFor(sha256.Sum256([]byte("abc"))); got != "0.0.0-b1.xj4bnp4pahh6uqkbidpf3lrceoyagyndsylxvhfucd7wd4qacwwq" {
		t.Fatalf("b1 encoding changed: %s", got)
	}
	shuffled := slices.Clone(original)
	slices.Reverse(shuffled)
	for i := range shuffled {
		shuffled[i].Path = "/another/computer/different-name.yaml"
	}
	shuffledResult, shuffledBuild := build(source, shuffled, "default", values, options)
	if !reflect.DeepEqual(compiled, shuffledResult) || !reflect.DeepEqual(baseline, shuffledBuild) {
		t.Fatal("inventory order or source location changed a sealed build")
	}
	refs := map[string]flow.Ref{}
	documents := map[string]map[string]any{}
	for _, component := range compiled {
		refs[component.Kind] = component.Ref
		if component.Kind == "context" {
			if !bytes.Equal(component.Bytes, contextBytes) || !bytes.Equal(component.Resource.Bytes, contextBytes) {
				t.Fatal("raw context bytes changed")
			}
			continue
		}
		var object map[string]any
		if err := json.Unmarshal(component.Bytes, &object); err != nil {
			t.Fatal(err)
		}
		documents[component.Kind] = object
	}
	assertEqual := func(want, got any) {
		t.Helper()
		if !reflect.DeepEqual(want, got) {
			t.Fatalf("wanted %#v; got %#v", want, got)
		}
	}
	assertEqual(projectRefValue(refs["context"]), documents["step"]["instructions_ref"])
	assertEqual(projectRefValue(refs["schema"]), documents["step"]["result_schema_ref"])
	assertEqual(projectRefValue(external), documents["step"]["executor"].(map[string]any)["adapter_ref"])
	stage := documents["workflow"]["stages"].(map[string]any)["default"].(map[string]any)
	assertEqual(projectRefValue(refs["step"]), stage["step_ref"])
	assertEqual(literal, stage["input_bindings"].(map[string]any)["value"].(map[string]any)["value"])
	assertEqual(literal, documents["workflow"]["inputs"].(map[string]any)["plan"].(map[string]any)["configuration"].(map[string]any)["default"])
	assertEqual(literal, documents["schema"]["const"])
	assertEqual(literal, documents["schema"]["default"])
	assertEqual("1.0.0", original[0].Ref.Version)
	for _, test := range []struct{ name, key string }{
		{"profile", "3eb6ba9148a679317b43ddb0e404d81a1d8c91483fcfc2dba0d81d1be218d0c0"},
		{"value", "e19b274360d14af1359e3e3f4aadf5a6831d7a6d1f30136e02e9fdfee0bca949"},
		{"description", "ef43204f34630be6448aeae569c63c03b874ccf0aa878842611ea61b48db4921"},
		{"license", "be980963276a0c8b6b90a756d429a9108f585006a5a1343c51fb53de9d6dd91e"},
		{"protocol", "5007e19d06f0d3adcacdbb226fda32f8212a9c0ef477cc615cc6847bbfe3f065"},
		{"capabilities", "86d92baf8fd3022b6e34d43d8373d4c41898d7b2a63cb6146a6c1fa9274d7bbd"},
		{"dependency", "27ca480da16972df2fdda91fb0d813d9fbd77d9aadb2e0c4835609a044b308ca"},
		{"question", "891462dfb32d9f7662365af762d16cf8037d7281eaae06614b31c36f4d5d9a6d"},
		{"context", "e682daa045eac110969d1df2976aabde211001558a9ba7e6cb966765cdc4bffd"},
		{"settings", "87d3ff7a97303931641e921c4c8fe867079621302d3e64f6b7030836177e2f5d"},
		{"exclude", "8081a32db673f3767ed69b59a59571dc5d8aaa2b60921792ab36fa36387360d3"},
		{"insert", "4b28c299f3db97f486dfb3a5000a3989daf8a0b031065a43a84072239e82b3f0"},
	} {
		t.Run(test.name, func(t *testing.T) {
			changedSource, changedComponents, profile, changedValues, changedOptions := source, slices.Clone(original), "default", values, options
			switch test.name {
			case "profile":
				profile = "careful"
			case "value":
				changedValues = map[string]any{"label": "Review"}
			case "description":
				changedSource.Description = "Different description, identical graph"
			case "license":
				changedSource.License = "MIT"
			case "protocol":
				changedSource.RequiresCoreProtocol = "2"
			case "capabilities":
				changedSource.RequestedCapabilities = []string{"network"}
			case "dependency":
				changedSource.ResolvedDependencies = []flow.Ref{external}
			case "question":
				changedSource.DecisionCatalog = []prifly.DecisionDefinition{{ID: "confirm", Title: "Continue?"}}
			case "context":
				changedComponents[1].Bytes = []byte("Different instructions\n")
				changedComponents[1].Ref.Digest = fmt.Sprintf("sha256:%x", sha256.Sum256(changedComponents[1].Bytes))
				resource := *context.Resource
				resource.Bytes = changedComponents[1].Bytes
				changedComponents[1].Resource = &resource
			case "settings":
				changedOptions.Settings = map[string]map[string]any{"test:workflow/root": {"limit": 2}}
			case "exclude":
				changedOptions.Exclude = []string{"optional"}
			case "insert":
				changedOptions.Extensions = []projectWorkflowExtension{{ID: "quality", From: "inspect", To: "done", Step: "quality"}}
			}
			_, changedBuild := build(changedSource, changedComponents, profile, changedValues, changedOptions)
			if changedBuild.BuildKey != "sha256:"+test.key {
				t.Fatalf("b1 vector changed: %s", changedBuild.BuildKey)
			}
		})
	}
}
