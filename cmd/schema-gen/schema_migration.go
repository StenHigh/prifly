package main

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func schemaMigrationSchema() ([]byte, error) {
	raw, err := prifly.PublicSchema("CoreRunStateV44")
	if err != nil {
		return nil, err
	}
	var bundle map[string]any
	if err := json.Unmarshal(raw, &bundle); err != nil {
		return nil, err
	}
	defs := bundle["$defs"].(map[string]any)
	g := generator{defs: defs}
	contracts := map[string]reflect.Type{
		"SchemaMigrationRequestV1":    reflect.TypeFor[prifly.SchemaMigrationRequest](),
		"SchemaMigrationPlanV1":       reflect.TypeFor[prifly.SchemaMigrationPlan](),
		"SchemaMigrationDecisionV1":   reflect.TypeFor[prifly.SchemaMigrationDecision](),
		"SchemaMigrationBindingV1":    reflect.TypeFor[prifly.SchemaMigrationBinding](),
		"SchemaMigrationResultV1":     reflect.TypeFor[prifly.SchemaMigrationResult](),
		"SchemaMigrationInspectionV1": reflect.TypeFor[prifly.SchemaMigrationInspection](),
		"AuthorityPackagesV2":         reflect.TypeFor[prifly.PackageRecord](),
	}
	names := []string{"CoreRunStateV45", "CoreRunViewV45"}
	for name, t := range contracts {
		defs[name] = g.schema(t)
		names = append(names, name)
	}
	versions := map[string]string{"SchemaMigrationRequest": "schema-migration-request/1", "SchemaMigrationPlan": "schema-migration-plan/1", "SchemaMigrationDecision": "schema-migration-decision/1", "SchemaMigrationResult": "schema-migration-result/1", "SchemaMigrationInspection": "schema-migration-inspection/1", "PackageRecord": prifly.AuthorityPackagesMigrationVersion}
	for name, version := range versions {
		g.property("runtime_"+name, "schema_version", map[string]any{"const": version})
	}
	g.property("runtime_PackageEntry", "lifecycle_generation", map[string]any{"type": "integer", "minimum": 1})
	for name, fields := range map[string][]string{
		"SchemaMigrationRequest": {"selections"}, "SchemaMigrationBinding": {"selections"},
		"SchemaMigrationPlan":   {"packages", "schemas", "runs", "reasons"},
		"SchemaMigrationSchema": {"candidates", "revocations"}, "SchemaMigrationRun": {"schema_refs"},
		"SchemaMigrationInspection": {"reasons"}, "SchemaMigrationResult": {"safe_next_commands"},
		"PackageRecord": {"packages", "schema_migrations"},
	} {
		for _, field := range fields {
			property := defs["runtime_"+name].(map[string]any)["properties"].(map[string]any)[field].(map[string]any)
			if choices, ok := property["anyOf"].([]any); ok {
				defs["runtime_"+name].(map[string]any)["properties"].(map[string]any)[field] = choices[0]
			}
		}
	}
	for _, name := range []string{"SchemaMigrationPlan", "SchemaMigrationBinding"} {
		g.property("runtime_"+name, "review_digest", map[string]any{"type": "string", "pattern": "^sha256:[0-9a-f]{64}$"})
	}
	g.property("runtime_SchemaMigrationPackageFact", "generation", map[string]any{"type": "integer", "minimum": 1})
	g.property("runtime_SchemaMigrationDecision", "commit_cut", map[string]any{"type": "integer", "minimum": 1})
	for name, fields := range map[string][]string{"SchemaMigrationDecision": {"id", "actor", "reason", "command_id"}, "SchemaMigrationBinding": {"decision_id"}, "SchemaMigrationRun": {"run_id"}} {
		for _, field := range fields {
			g.property("runtime_"+name, field, map[string]any{"type": "string", "minLength": 1})
		}
	}

	// Every v2 entry carries its lifecycle generation, including the legacy seed.
	entry := defs["runtime_PackageEntry"].(map[string]any)
	entry["required"] = append(entry["required"].([]string), "lifecycle_generation")
	record := defs["runtime_PackageRecord"].(map[string]any)
	record["required"] = append(record["required"].([]string), "schema_migrations")
	state := defs["runtime_Run"].(map[string]any)
	removeSchemaRequired(state, "queue_task")
	removeSchemaRequired(state, "queue_request_bytes")
	g.property("runtime_Run", "schema_version", map[string]any{"const": prifly.CoreSchemaMigrationStateVersion})
	g.property("runtime_Run", "schema_migration", map[string]any{"$ref": "#/$defs/SchemaMigrationBindingV1"})
	state["required"] = append(state["required"].([]any), "schema_migration")
	state["dependentRequired"] = map[string]any{"queue_task": []string{"queue_request_bytes"}, "queue_request_bytes": []string{"queue_task"}}
	g.property("runtime_RunView", "schema_version", map[string]any{"const": prifly.CoreSchemaMigrationReadVersion})
	defs["CoreRunStateV45"] = map[string]any{"$ref": "#/$defs/runtime_Run"}
	defs["CoreRunViewV45"] = map[string]any{"$ref": "#/$defs/runtime_RunView"}
	delete(bundle, "$ref")
	bundle["$id"] = "urn:prifly:schema-migration:1"
	bundle["title"] = "Pri-Fly explicit shared schema migration contracts"
	bundle["description"] = "An owner decision covers exact schemas and suppliers after a mistaken revoke. Only explicitly selected new Runs carry the binding. Revoked packages and old Runs retain their lifecycle. State/read 45 admits migration with or without a queue; queue-only 44 stays unchanged."
	slices.Sort(names)
	bundle["x-prifly-contracts"] = names
	pruneMigrationDefinitions(bundle)
	return schemaBytes(bundle)
}

func schemaBytes(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// Keep only new public roots and their reachable definitions. Inheriting a
// bundle must not expose old root names with the new root's discriminator.
func pruneMigrationDefinitions(bundle map[string]any) {
	definitions := bundle["$defs"].(map[string]any)
	selected := map[string]any{}
	var visit func(any)
	visit = func(value any) {
		switch value := value.(type) {
		case map[string]any:
			if ref, ok := value["$ref"].(string); ok && strings.HasPrefix(ref, "#/$defs/") {
				name := strings.TrimPrefix(ref, "#/$defs/")
				if _, seen := selected[name]; !seen {
					selected[name] = definitions[name]
					visit(definitions[name])
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
	for _, name := range bundle["x-prifly-contracts"].([]string) {
		visit(map[string]any{"$ref": "#/$defs/" + name})
	}
	bundle["$defs"] = selected
}
