package main

import (
	"encoding/json"
	"reflect"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// Reported cost is an additive field on Attempt and the assisted submission.
// Excluding both before traversal keeps every delivered bundle byte-stable.
func reportedCostField(t reflect.Type, name string) bool {
	return t == reflect.TypeFor[prifly.Attempt]() && name == "ReportedCosts" ||
		t == reflect.TypeFor[prifly.SessionSubmission]() && name == "ReportedCosts"
}

func reportedCostConstraints(g *generator) {
	ref := func(name string) map[string]any { return map[string]any{"$ref": "#/$defs/" + name} }
	definitions, _, err := prifly.Builtins()
	if err != nil {
		panic(err)
	}
	found := false
	for _, definition := range definitions {
		if definition.Ref.ID == "core:schema/local-context" && definition.Ref.Version == "1.0.0" {
			var schema map[string]any
			if err := json.Unmarshal(definition.Bytes, &schema); err != nil {
				panic(err)
			}
			g.defs["runtime_AssistedContextManifestV1"], found = schema, true
			break
		}
	}
	if !found {
		panic("missing exact assisted context schema")
	}
	for name, version := range map[string]string{
		"runtime_Run": prifly.CoreReportedCostStateVersion, "runtime_RunView": prifly.CoreReportedCostReadVersion,
		"runtime_NextView": prifly.CoreReportedCostNextVersion,
	} {
		g.property(name, "schema_version", map[string]any{"const": version})
	}
	for _, name := range []string{"runtime_SessionHandoff", "runtime_SessionTask", "runtime_SessionSubmission"} {
		g.property(name, "schema_version", map[string]any{"const": prifly.AssistedSessionCostVersion})
	}
	g.property("runtime_ReportedCost", "schema_version", map[string]any{"const": prifly.ReportedCostVersion})
	g.property("runtime_ReportedCost", "source", ref("Identifier"))
	g.property("runtime_ReportedCost", "amount", map[string]any{"type": "string", "pattern": "^(?:0|[1-9][0-9]{0,29})(?:\\.[0-9]{1,18})?$"})
	g.property("runtime_ReportedCost", "currency", map[string]any{"type": "string", "pattern": "^[A-Z]{3}$"})
	// Assisted execution currently transports local-context/1 even though the
	// Run also pins the full context resources. Managed context Attempts use v2;
	// the state contract must describe both actual forms rather than claim every
	// SessionTask carries fields the host is never handed.
	context := map[string]any{"oneOf": []any{ref("runtime_AssistedContextManifestV1"), ref("runtime_ContextManifest")}}
	g.property("runtime_Attempt", "context", context)
	g.property("runtime_SessionTask", "context", context)
	g.property("runtime_SessionTask", "permitted_effects", map[string]any{
		"type": "array", "maxItems": 8, "uniqueItems": true,
		"items": enum("write_inside_claimed_worktree", "local_git_commit_on_claimed_base", "write_inside_declared_output_slot"),
	})
	reports := map[string]any{
		"anyOf": []any{
			map[string]any{"type": "array", "maxItems": prifly.MaxReportedCosts, "items": ref("runtime_ReportedCost")},
			map[string]any{"type": "null"},
		},
	}
	g.property("runtime_Attempt", "reported_costs", reports)
	g.property("runtime_SessionSubmission", "reported_costs", reports)
}
