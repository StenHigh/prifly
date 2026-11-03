package main

import (
	"encoding/json"
	"reflect"

	"github.com/stenhigh/prifly/internal/projectwire"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func projectOwnedNamingSchema() ([]byte, error) {
	data, err := prifly.PublicSchema("ProjectLaunchSummaryV7")
	if err != nil {
		return nil, err
	}
	var bundle map[string]any
	if err := json.Unmarshal(data, &bundle); err != nil {
		return nil, err
	}
	defs := bundle["$defs"].(map[string]any)
	g := generator{defs: defs}
	snapshot := g.schema(reflect.TypeFor[prifly.WorktreeNamingSnapshot]())
	authoring, err := prifly.AuthoringSchema("project-profile-v5")
	if err != nil {
		return nil, err
	}
	var profile map[string]any
	if err := json.Unmarshal(authoring, &profile); err != nil {
		return nil, err
	}
	g.property("runtime_WorktreeNamingSnapshot", "declaration", profile["properties"].(map[string]any)["worktree_naming"])
	g.property("runtime_WorktreeNamingSnapshot", "schema_version", map[string]any{"const": "project-worktree-naming-snapshot/1"})
	g.property("runtime_WorktreeNamingSnapshot", "inputs", map[string]any{"type": "object", "minProperties": 1, "maxProperties": 16, "propertyNames": map[string]any{"pattern": "^[a-z][a-z0-9_]{0,31}$"}, "additionalProperties": g.schema(reflect.TypeFor[prifly.WorktreeNamingInput]())})
	for _, field := range []string{"schema_digest", "value_digest"} {
		g.property("runtime_WorktreeNamingInput", field, map[string]any{"type": "string", "pattern": "^sha256:[0-9a-f]{64}$"})
	}
	for _, field := range []string{"schema_path", "value_source"} {
		g.property("runtime_WorktreeNamingInput", field, map[string]any{"type": "string", "minLength": 1})
	}
	claim := defs["runtime_WorktreeClaim"].(map[string]any)
	claim["properties"].(map[string]any)["naming_snapshot"] = snapshot
	review := defs["projectwire_WorktreeNamingReview"].(map[string]any)
	review["properties"].(map[string]any)["snapshot"] = snapshot
	bindings := review["properties"].(map[string]any)["bindings"].(map[string]any)["additionalProperties"].(map[string]any)
	bindings["oneOf"] = append(bindings["oneOf"].([]any), map[string]any{"type": "object", "additionalProperties": false, "required": []string{"naming_input", "pointer"}, "properties": map[string]any{"naming_input": map[string]any{"type": "string", "minLength": 1}, "pointer": map[string]any{"type": "string"}}})
	// /8 admits both existing composition contracts without changing either.
	raw, err := prifly.PublicSchema("WorkflowCompositionV1")
	if err != nil {
		return nil, err
	}
	var oldComposition map[string]any
	if err := json.Unmarshal(raw, &oldComposition); err != nil {
		return nil, err
	}
	compositionV1 := importNamingSchema(defs, oldComposition, "namingCompositionV1")
	summary := defs["projectwire_LaunchSummary"].(map[string]any)
	props := summary["properties"].(map[string]any)
	props["schema_version"] = map[string]any{"const": "project-launch-summary/8"}
	props["composition"] = map[string]any{"oneOf": []any{props["composition"], compositionV1}}
	removeSchemaRequired(summary, "composition")
	summary["anyOf"] = []any{map[string]any{"required": []string{"worktree_naming"}, "properties": map[string]any{"worktree_naming": map[string]any{"required": []string{"snapshot"}}}}, map[string]any{"required": []string{"worktree_claim"}, "properties": map[string]any{"worktree_claim": map[string]any{"required": []string{"naming_snapshot"}}}}}
	defs["ProjectLaunchSummaryV8"] = map[string]any{"$ref": "#/$defs/projectwire_LaunchSummary"}
	start := defs["projectwire_StartResult"].(map[string]any)
	start["properties"].(map[string]any)["schema_version"] = map[string]any{"const": "project-start/8"}
	start["properties"].(map[string]any)["launch_summary"] = map[string]any{"$ref": "#/$defs/ProjectLaunchSummaryV8"}
	defs["ProjectStartV8"] = map[string]any{"$ref": "#/$defs/projectwire_StartResult"}
	defs["WorktreeNamingSnapshotV1"] = snapshot
	for _, tc := range []struct {
		name, version string
		authority     bool
	}{{"AuthorityClaimsV5", prifly.SnapshotAuthorityClaimsVersion, true}, {"WorktreeClaimsV4", "foundation-claims/4", false}} {
		fields := map[string]any{"schema_version": map[string]any{"const": tc.version}, "claims": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/$defs/runtime_WorktreeClaim"}}}
		required := []string{"schema_version", "claims"}
		if tc.authority {
			fields["authority_id"] = map[string]any{"type": "string"}
			required = append(required, "authority_id")
		}
		defs[tc.name] = map[string]any{"type": "object", "additionalProperties": false, "required": required, "properties": fields}
	}
	defs["ProjectQuestionnaireV5"] = g.schema(reflect.TypeFor[projectwire.Questionnaire]())
	g.property("projectwire_Questionnaire", "schema_version", map[string]any{"const": "project-questionnaire/5"})
	contracts := []string{"AuthorityClaimsV5", "WorktreeClaimsV4", "WorktreeNamingSnapshotV1", "ProjectLaunchSummaryV8", "ProjectStartV8", "ProjectQuestionnaireV5"}
	// Prune unrelated roots and their historical discriminators.
	selected := map[string]any{}
	var visit func(any)
	visit = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			if ref, ok := v["$ref"].(string); ok && len(ref) > 8 && ref[:8] == "#/$defs/" {
				name := ref[8:]
				if _, ok := selected[name]; !ok {
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
	return json.MarshalIndent(map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "$id": "urn:prifly:project-owned-naming:1", "x-prifly-contracts": contracts, "$defs": selected}, "", "  ")
}
