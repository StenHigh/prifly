package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/stenhigh/prifly/internal/projectwire"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// Skip before recursion: historical bundles must not acquire even unreachable
// definitions merely because a current Go type gained opt-in metadata.
func worktreeNamingField(t reflect.Type, field string) bool {
	return t == reflect.TypeFor[prifly.WorktreeClaim]() && field == "Naming" || t == reflect.TypeFor[projectwire.LaunchSummary]() && (field == "WorktreeNaming" || field == "WorktreeNamePreview" || field == "WorktreeClaim")
}

func worktreeNamingSchema() ([]byte, error) {
	data, err := prifly.PublicSchema("ProjectLaunchSummaryV4")
	if err != nil {
		return nil, err
	}
	var old map[string]any
	if err = json.Unmarshal(data, &old); err != nil {
		return nil, err
	}
	defs := old["$defs"].(map[string]any)
	g := generator{defs: defs}
	g.schema(reflect.TypeFor[prifly.WorktreeNameAllocation]())
	g.schema(reflect.TypeFor[prifly.WorktreeNamePreview]())
	g.schema(reflect.TypeFor[projectwire.WorktreeNamingReview]())
	claim := defs["runtime_WorktreeClaim"].(map[string]any)
	claim["properties"].(map[string]any)["naming"] = map[string]any{"$ref": "#/$defs/runtime_WorktreeNameAllocation"}
	allocation := defs["runtime_WorktreeNameAllocation"].(map[string]any)["properties"].(map[string]any)
	allocation["base"] = map[string]any{"type": "string", "minLength": 1, "maxLength": 96, "pattern": "^[a-z0-9]+(?:-[a-z0-9]+)*$"}
	allocation["policy"] = map[string]any{"const": prifly.WorktreeNamingPolicy}
	allocation["ordinal"] = map[string]any{"type": "integer", "minimum": 1, "maximum": prifly.MaxWorktreeNames}
	allocation["phase"] = enum("reserved", "directory", "creating_branch", "branch", "tree")
	review := defs["projectwire_WorktreeNamingReview"].(map[string]any)["properties"].(map[string]any)
	review["policy"] = map[string]any{"const": prifly.WorktreeNamingPolicy}
	review["root"] = map[string]any{"const": prifly.ClaimRoot}
	review["branch_prefix"] = map[string]any{"const": "prifly/"}
	review["max_candidates"] = map[string]any{"const": prifly.MaxWorktreeNames}
	review["bindings"] = map[string]any{"type": "object", "additionalProperties": map[string]any{"oneOf": []any{
		map[string]any{"type": "object", "additionalProperties": false, "required": []string{"value"}, "properties": map[string]any{"value": map[string]any{"type": "string"}}},
		map[string]any{"type": "object", "additionalProperties": false, "required": []string{"input", "pointer"}, "properties": map[string]any{"input": map[string]any{"type": "string", "minLength": 1}, "pointer": map[string]any{"type": "string"}}},
	}}}
	preview := defs["runtime_WorktreeNamePreview"].(map[string]any)
	preview["properties"].(map[string]any)["reserved"] = map[string]any{"const": false}
	summary := defs["projectwire_LaunchSummary"].(map[string]any)
	properties := summary["properties"].(map[string]any)
	properties["schema_version"] = map[string]any{"const": "project-launch-summary/5"}
	properties["worktree_naming"] = map[string]any{"$ref": "#/$defs/projectwire_WorktreeNamingReview"}
	properties["worktree_name_preview"] = map[string]any{"$ref": "#/$defs/runtime_WorktreeNamePreview"}
	properties["worktree_claim"] = map[string]any{"$ref": "#/$defs/runtime_WorktreeClaim"}
	removeSchemaRequired(summary, "queue_task")
	summary["oneOf"] = []any{map[string]any{"required": []string{"worktree_naming"}}, map[string]any{"required": []string{"worktree_claim"}}}
	start := defs["projectwire_StartResult"].(map[string]any)
	startProperties := start["properties"].(map[string]any)
	startProperties["schema_version"] = map[string]any{"const": "project-start/5"}
	startProperties["launch_summary"] = map[string]any{"$ref": "#/$defs/ProjectLaunchSummaryV5"}
	removeSchemaRequired(start, "queue_task")
	// Import each existing read contract exactly. Content-addressed definitions
	// share identical subgraphs across versions without broadening saved Run
	// shapes or copying a new field into an older contract.
	names, err := prifly.PublicSchemaNames()
	if err != nil {
		return nil, err
	}
	reads := []any{}
	for _, name := range names {
		if !strings.HasPrefix(name, "CoreRunView") {
			continue
		}
		raw, err := prifly.PublicSchema(name)
		if err != nil {
			return nil, err
		}
		var schema map[string]any
		if err = json.Unmarshal(raw, &schema); err != nil {
			return nil, err
		}
		reads = append(reads, importNamingSchema(defs, schema, name))
	}
	startProperties["run"] = map[string]any{"oneOf": reads}
	defs["ProjectLaunchSummaryV5"] = map[string]any{"$ref": "#/$defs/projectwire_LaunchSummary"}
	defs["ProjectStartV5"] = map[string]any{"$ref": "#/$defs/projectwire_StartResult"}
	defs["AuthorityClaimsV4"] = map[string]any{"type": "object", "additionalProperties": false, "required": []string{"schema_version", "authority_id", "claims"}, "properties": map[string]any{"schema_version": map[string]any{"const": prifly.NamedAuthorityClaimsVersion}, "authority_id": map[string]any{"type": "string"}, "claims": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/$defs/runtime_WorktreeClaim"}}}}
	defs["WorktreeClaimsV3"] = map[string]any{"type": "object", "additionalProperties": false, "required": []string{"schema_version", "claims"}, "properties": map[string]any{"schema_version": map[string]any{"const": "foundation-claims/3"}, "claims": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/$defs/runtime_WorktreeClaim"}}}}
	contracts := []string{"AuthorityClaimsV4", "ProjectLaunchSummaryV5", "ProjectStartV5", "WorktreeClaimsV3"}
	// Discard the unrelated roots from the source bundle.
	selected := map[string]any{}
	var visit func(any)
	visit = func(value any) {
		switch value := value.(type) {
		case map[string]any:
			if ref, ok := value["$ref"].(string); ok && strings.HasPrefix(ref, "#/$defs/") {
				name := strings.TrimPrefix(ref, "#/$defs/")
				if _, seen := selected[name]; !seen {
					selected[name] = defs[name]
					visit(defs[name])
				}
			}
			for _, child := range value {
				visit(child)
			}
		case []any:
			for _, child := range value {
				visit(child)
			}
		}
	}
	for _, name := range contracts {
		visit(map[string]any{"$ref": "#/$defs/" + name})
	}
	return json.MarshalIndent(map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "$id": "urn:prifly:worktree-naming:1", "x-prifly-contracts": contracts, "$defs": selected}, "", "  ")
}

func removeSchemaRequired(def map[string]any, name string) {
	result := []any{}
	for _, field := range def["required"].([]any) {
		if field != name {
			result = append(result, field)
		}
	}
	def["required"] = result
}

func importNamingSchema(target map[string]any, source map[string]any, prefix string) any {
	defs := source["$defs"].(map[string]any)
	memo := map[string]string{}
	var rewrite func(any) any
	definition := func(name string) string {
		if existing, ok := memo[name]; ok {
			return existing
		}
		alias := prefix + "_" + name
		memo[name] = alias
		value := rewrite(defs[name])
		data, _ := json.Marshal(value)
		key := fmt.Sprintf("read_%x", sha256.Sum256(data))
		target[key] = value
		target[alias] = map[string]any{"$ref": "#/$defs/" + key}
		memo[name] = key
		return key
	}
	rewrite = func(value any) any {
		switch value := value.(type) {
		case map[string]any:
			result := map[string]any{}
			keys := make([]string, 0, len(value))
			for key := range value {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				child := value[key]
				if key == "$ref" {
					if ref, ok := child.(string); ok && strings.HasPrefix(ref, "#/$defs/") {
						result[key] = "#/$defs/" + definition(strings.TrimPrefix(ref, "#/$defs/"))
						continue
					}
				}
				result[key] = rewrite(child)
			}
			return result
		case []any:
			result := make([]any, len(value))
			for i, child := range value {
				result[i] = rewrite(child)
			}
			return result
		default:
			return value
		}
	}
	return rewrite(map[string]any{"$ref": source["$ref"]})
}
