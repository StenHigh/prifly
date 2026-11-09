package main

import (
	"encoding/json"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func projectSchemaMigrationSchema() ([]byte, error) {
	raw, err := prifly.PublicSchema("ProjectLaunchSummaryV8")
	if err != nil {
		return nil, err
	}
	var bundle map[string]any
	if err := json.Unmarshal(raw, &bundle); err != nil {
		return nil, err
	}
	defs := bundle["$defs"].(map[string]any)
	summary := defs["projectwire_LaunchSummary"].(map[string]any)
	summary["properties"].(map[string]any)["schema_version"] = map[string]any{"const": "project-launch-summary/9"}
	summary["properties"].(map[string]any)["schema_migration"] = map[string]any{"type": "string", "minLength": 1}
	removeSchemaRequired(summary, "worktree_naming")
	removeSchemaRequired(summary, "review_digest")
	delete(summary, "anyOf")
	summary["required"] = append(summary["required"].([]any), "schema_migration", "review_digest")
	start := defs["projectwire_StartResult"].(map[string]any)
	start["properties"].(map[string]any)["schema_version"] = map[string]any{"const": "project-start/9"}
	start["properties"].(map[string]any)["schema_migration"] = map[string]any{"type": "string", "minLength": 1}
	start["properties"].(map[string]any)["launch_summary"] = map[string]any{"$ref": "#/$defs/ProjectLaunchSummaryV9"}
	for _, field := range []string{"schema_migration", "launch_summary", "run_id"} {
		removeSchemaRequired(start, field)
	}
	start["required"] = append(start["required"].([]any), "schema_migration", "launch_summary", "run_id")
	// A migration Run has its own closed state/read boundary.
	run, err := prifly.PublicSchema("CoreRunViewV45")
	if err != nil {
		return nil, err
	}
	var migration map[string]any
	if err := json.Unmarshal(run, &migration); err != nil {
		return nil, err
	}
	start["properties"].(map[string]any)["run"] = importNamingSchema(defs, migration, "migrationRun")
	defs["ProjectLaunchSummaryV9"] = map[string]any{"$ref": "#/$defs/projectwire_LaunchSummary"}
	defs["ProjectStartV9"] = map[string]any{"$ref": "#/$defs/projectwire_StartResult"}
	delete(bundle, "$ref")
	bundle["$id"] = "urn:prifly:project-schema-migration:9"
	bundle["title"] = "Pri-Fly explicitly selected Project schema migration"
	bundle["x-prifly-contracts"] = []string{"ProjectLaunchSummaryV9", "ProjectStartV9"}
	pruneMigrationDefinitions(bundle)
	return schemaBytes(bundle)
}
