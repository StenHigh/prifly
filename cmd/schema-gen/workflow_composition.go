package main

import (
	"encoding/json"
	"reflect"
	"strings"

	"github.com/stenhigh/prifly/internal/projectwire"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func workflowCompositionSchema() ([]byte, error) {
	data, err := prifly.PublicSchema("ProjectLaunchSummaryV5")
	if err != nil {
		return nil, err
	}
	var bundle map[string]any
	if err := json.Unmarshal(data, &bundle); err != nil {
		return nil, err
	}
	defs := bundle["$defs"].(map[string]any)
	g := generator{defs: defs}
	composition := g.schema(reflect.TypeFor[projectwire.CompositionReview]())
	g.property("projectwire_CompositionReview", "schema_version", map[string]any{"const": "workflow-composition/1"})
	g.property("projectwire_CompositionGroupReview", "mode", enum("sequence", "parallel", "choice"))
	summary := defs["projectwire_LaunchSummary"].(map[string]any)
	delete(summary, "oneOf")
	summary["properties"].(map[string]any)["schema_version"] = map[string]any{"const": "project-launch-summary/6"}
	summary["properties"].(map[string]any)["composition"] = composition
	summary["required"] = append(summary["required"].([]any), "composition")
	defs["ProjectLaunchSummaryV6"] = map[string]any{"$ref": "#/$defs/projectwire_LaunchSummary"}
	start := defs["projectwire_StartResult"].(map[string]any)
	delete(start, "oneOf")
	start["properties"].(map[string]any)["schema_version"] = map[string]any{"const": "project-start/6"}
	start["properties"].(map[string]any)["launch_summary"] = map[string]any{"$ref": "#/$defs/ProjectLaunchSummaryV6"}
	defs["ProjectStartV6"] = map[string]any{"$ref": "#/$defs/projectwire_StartResult"}
	var provenance map[string]any
	if err := json.Unmarshal(projectwire.BuildProvenanceV2Schema, &provenance); err != nil {
		return nil, err
	}
	provenanceRoot := map[string]any{}
	for key, value := range provenance {
		if key != "$defs" && key != "$schema" && key != "$id" {
			provenanceRoot[key] = value
		}
	}
	provenance["$defs"].(map[string]any)["root"] = provenanceRoot
	provenance["$ref"] = "#/$defs/root"
	defs["BuildProvenanceV2"] = importNamingSchema(defs, provenance, "compositionProvenance")
	ref := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"id", "version", "digest"}, "properties": map[string]any{"id": map[string]any{"type": "string"}, "version": map[string]any{"type": "string"}, "digest": map[string]any{"type": "string", "pattern": "^sha256:[0-9a-f]{64}$"}}}
	stringsArray := map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
	defs["ProjectCompileV3"] = map[string]any{"type": "object", "additionalProperties": false, "required": []string{"schema_version", "repository", "package", "output", "components", "author_package", "build_key", "sources", "composition"}, "properties": map[string]any{
		"schema_version": map[string]any{"const": "project-compile/3"}, "repository": map[string]any{"type": "string"}, "output": map[string]any{"type": "string"}, "package": ref, "build_key": map[string]any{"type": "string", "pattern": "^sha256:[0-9a-f]{64}$"}, "author_package": g.schema(reflect.TypeFor[projectwire.BuildIdentity]()), "sources": stringsArray, "composition": composition, "explanation": stringsArray, "package_verdicts_unclosed": stringsArray,
		"components": map[string]any{"type": "array", "minItems": 1, "maxItems": 1024, "items": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"kind", "ref", "path"}, "properties": map[string]any{"kind": enum("workflow", "step", "check", "schema", "context"), "ref": ref, "path": map[string]any{"type": "string"}}}},
	}}
	contracts := []string{"BuildProvenanceV2", "ProjectCompileV3", "ProjectLaunchSummaryV6", "ProjectStartV6", "WorkflowCompositionV1"}
	defs["WorkflowCompositionV1"] = composition
	selected := map[string]any{}
	var visit func(any)
	visit = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			if ref, ok := v["$ref"].(string); ok && strings.HasPrefix(ref, "#/$defs/") {
				name := strings.TrimPrefix(ref, "#/$defs/")
				if _, seen := selected[name]; !seen {
					selected[name] = defs[name]
					visit(defs[name])
				}
			}
			for _, child := range v {
				visit(child)
			}
		case []any:
			for _, child := range v {
				visit(child)
			}
		}
	}
	for _, name := range contracts {
		visit(map[string]any{"$ref": "#/$defs/" + name})
	}
	return json.MarshalIndent(map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "$id": "urn:prifly:workflow-composition:1", "x-prifly-contracts": contracts, "$defs": selected}, "", "  ")
}
