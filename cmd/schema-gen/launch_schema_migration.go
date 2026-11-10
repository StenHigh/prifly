package main

import (
	"encoding/json"
	"reflect"
	"slices"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func launchSchemaMigrationSchema() ([]byte, error) {
	raw, err := prifly.PublicSchema("ProjectLaunchSummaryV9")
	if err != nil {
		return nil, err
	}
	var bundle map[string]any
	if err = json.Unmarshal(raw, &bundle); err != nil {
		return nil, err
	}
	defs := bundle["$defs"].(map[string]any)
	g := generator{defs: defs}
	roots := map[string]reflect.Type{
		"LaunchSchemaMigrationStateV1":      reflect.TypeFor[prifly.LaunchSchemaMigrationState](),
		"LaunchSchemaMigrationInspectionV1": reflect.TypeFor[prifly.LaunchSchemaMigrationInspection](),
		"LaunchSchemaMigrationResultV1":     reflect.TypeFor[prifly.LaunchSchemaMigrationResult](),
		"LaunchSchemaMigrationReviewV1":     reflect.TypeFor[prifly.LaunchSchemaMigrationReview](),
	}
	names := []string{"ProjectLaunchSummaryV10", "ProjectStartV10"}
	for name, t := range roots {
		defs[name] = g.schema(t)
		names = append(names, name)
	}
	for name, version := range map[string]string{"LaunchSchemaMigrationState": prifly.LaunchSchemaMigrationStateVersion, "LaunchSchemaMigrationInspection": "launch-schema-migration-inspection/1", "LaunchSchemaMigrationResult": "launch-schema-migration-result/1"} {
		g.property("runtime_"+name, "schema_version", map[string]any{"const": version})
	}
	for _, name := range []string{"runtime_LaunchSchemaMigrationState", "runtime_LaunchSchemaMigrationReview"} {
		g.property(name, "generation", map[string]any{"type": "integer", "minimum": 0, "maximum": 9007199254740991})
	}
	g.property("runtime_LaunchSchemaMigrationReview", "source", map[string]any{"enum": []string{"explicit", "saved"}})
	defs["runtime_LaunchSchemaMigrationReview"].(map[string]any)["allOf"] = []any{map[string]any{
		"if":   map[string]any{"properties": map[string]any{"source": map[string]any{"const": "saved"}}},
		"then": map[string]any{"required": []string{"generation"}, "properties": map[string]any{"generation": map[string]any{"minimum": 1}}},
		"else": map[string]any{"not": map[string]any{"required": []string{"generation"}}},
	}}
	for _, field := range []string{"build_key", "review_digest"} {
		g.property("runtime_LaunchSchemaMigrationChoice", field, map[string]any{"type": "string", "pattern": "^sha256:[0-9a-f]{64}$"})
	}
	g.property("runtime_LaunchSchemaMigrationChoice", "decision_id", map[string]any{"type": "string", "minLength": 1, "maxLength": 128})
	// The embedded build is flattened by the reflection generator.
	for _, field := range []string{"authority_id", "project_id", "repository", "launch"} {
		g.property("runtime_LaunchSchemaMigrationScope", field, map[string]any{"type": "string", "minLength": 1, "maxLength": 4096})
	}
	reasons := defs["runtime_LaunchSchemaMigrationInspection"].(map[string]any)["properties"].(map[string]any)["reasons"].(map[string]any)
	if choices, ok := reasons["anyOf"].([]any); ok {
		g.property("runtime_LaunchSchemaMigrationInspection", "reasons", choices[0].(map[string]any))
	}
	for _, item := range []struct{ name, version string }{{"projectwire_LaunchSummary", "project-launch-summary/10"}, {"projectwire_StartResult", "project-start/10"}} {
		g.property(item.name, "schema_version", map[string]any{"const": item.version})
		g.property(item.name, "schema_migration_choice", map[string]any{"$ref": "#/$defs/LaunchSchemaMigrationReviewV1"})
		def := defs[item.name].(map[string]any)
		def["required"] = append(def["required"].([]any), "schema_migration_choice")
	}
	g.property("projectwire_StartResult", "launch_summary", map[string]any{"$ref": "#/$defs/ProjectLaunchSummaryV10"})
	defs["ProjectLaunchSummaryV10"] = map[string]any{"$ref": "#/$defs/projectwire_LaunchSummary"}
	defs["ProjectStartV10"] = map[string]any{"$ref": "#/$defs/projectwire_StartResult"}
	bundle["$id"] = "urn:prifly:launch-schema-migration:1"
	bundle["title"] = "Pri-Fly exact launch schema migration choices"
	slices.Sort(names)
	bundle["x-prifly-contracts"] = names
	pruneMigrationDefinitions(bundle)
	return schemaBytes(bundle)
}
