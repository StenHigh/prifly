package main

import (
	"encoding/json"
	"reflect"
	"slices"
	"sort"

	"github.com/stenhigh/prifly/internal/flow"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// Shared Go structs may grow while the delivered schemas stay immutable. Skip
// new fields before reflection visits their types, including unreachable defs.
func invocationField(t reflect.Type, name string) bool {
	switch t {
	case reflect.TypeFor[prifly.Run]():
		return name == "Invocations" || name == "WorkflowConfigurations"
	case reflect.TypeFor[prifly.Stop]():
		return name == "Scope" || name == "ScopeID"
	case reflect.TypeFor[prifly.NextView]():
		return name == "InvocationID" || name == "StageID"
	case reflect.TypeFor[prifly.Preview]():
		return name == "Workflows"
	case reflect.TypeFor[prifly.RegistryFile]():
		return name == "Aliases"
	case reflect.TypeFor[prifly.ProfileCapabilities]():
		return name == "StateVersions" || name == "ReadVersions"
	}
	return false
}

func invocationConstraints(g *generator) {
	for _, name := range []string{"Identifier", "ImmutableRef", "ArtifactRef", "PortName", "Timestamp"} {
		data, err := flow.ProtocolSchema(name)
		if err != nil {
			panic(err)
		}
		var source struct {
			Defs map[string]any `json:"$defs"`
		}
		if err := json.Unmarshal(data, &source); err != nil {
			panic(err)
		}
		for name, value := range source.Defs {
			g.defs[name] = value
		}
	}
	ref := func(name string) map[string]any { return map[string]any{"$ref": "#/$defs/" + name} }
	objectMap := func(name string, minimum, maximum int) map[string]any {
		return map[string]any{"type": "object", "additionalProperties": ref(name), "minProperties": minimum, "maxProperties": maximum}
	}
	portMap := func() map[string]any {
		shape := objectMap("ArtifactRef", 0, 256)
		shape["propertyNames"] = ref("PortName")
		return shape
	}
	require := func(name string, fields ...string) {
		shape := g.defs[name].(map[string]any)
		required := shape["required"].([]string)
		for _, field := range fields {
			if !slices.Contains(required, field) {
				required = append(required, field)
			}
		}
		sort.Strings(required)
		shape["required"] = required
	}
	for name, version := range map[string]string{
		"runtime_Run":                prifly.CoreInvocationStateVersion,
		"runtime_RunView":            prifly.CoreInvocationReadVersion,
		"runtime_NextView":           prifly.CoreInvocationNextVersion,
		"runtime_Preview":            prifly.CoreInvocationPreviewVersion,
		"runtime_RegistryFile":       "2",
		"runtime_CapabilityManifest": "capabilities/2",
	} {
		if _, exists := g.defs[name]; exists {
			g.property(name, "schema_version", map[string]any{"const": version})
		}
	}
	g.defs["flow_Ref"] = ref("ImmutableRef")
	g.defs["runtime_ArtifactRef"] = ref("ArtifactRef")
	// time.Time exposes no struct fields but marshals as an RFC 3339 string.
	// Correct the new contract only; delivered legacy bundle bytes stay frozen.
	// Native process observations use time.Time.MarshalJSON: RFC 3339 with
	// the recorded offset, unlike authority Observation.UTC's UTC-only contract.
	g.defs["time_Time"] = map[string]any{"type": "string", "format": "date-time", "maxLength": 40}
	g.property("runtime_Observation", "utc", ref("Timestamp"))
	g.property("runtime_Observation", "session", ref("Identifier"))
	g.property("runtime_Observation", "monotonic_ms", map[string]any{"type": "integer", "minimum": 0, "maximum": 9007199254740991})

	statuses := enum("ready", "running", "waiting", "stopping", "completed", "failed", "cancelled", "uncertain")
	outcomes := enum("succeeded", "rejected", "no_work", "partial")
	for _, name := range []string{"runtime_Run", "runtime_Invocation"} {
		g.property(name, "status", statuses)
		g.property(name, "outcome", nullable(outcomes))
		g.property(name, "settled", ref("runtime_Observation"))
		g.property(name, "control_transitions", map[string]any{"type": "integer", "minimum": 0, "maximum": 9007199254740991})
		shape := g.defs[name].(map[string]any)
		shape["if"] = map[string]any{"properties": map[string]any{"status": map[string]any{"const": "completed"}}}
		shape["then"] = map[string]any{"properties": map[string]any{"outcome": outcomes}}
		shape["else"] = map[string]any{"properties": map[string]any{"outcome": map[string]any{"type": "null"}}}
		terminal := map[string]any{"required": []string{"settled"}}
		if name == "runtime_Invocation" {
			terminal["properties"] = map[string]any{"ready_stages": map[string]any{"maxItems": 0}}
		}
		shape["allOf"] = []any{map[string]any{
			"if":   map[string]any{"properties": map[string]any{"status": enum("completed", "failed", "cancelled")}},
			"then": terminal,
			"else": map[string]any{"not": map[string]any{"required": []string{"settled"}}},
		}}
	}
	for _, field := range []string{"id", "run_id", "parent_invocation_id", "caller_stage_activation_id"} {
		g.property("runtime_Invocation", field, ref("Identifier"))
	}
	for _, field := range []string{"input_refs", "output_refs"} {
		g.property("runtime_Invocation", field, portMap())
	}
	g.property("runtime_Invocation", "ready_stages", map[string]any{"type": "array", "items": ref("Identifier"), "maxItems": 1, "uniqueItems": true})
	g.property("runtime_Invocation", "step_instances", map[string]any{"type": "integer", "minimum": 0, "maximum": 9007199254740991})
	g.defs["runtime_Invocation"].(map[string]any)["dependentRequired"] = map[string]any{
		"parent_invocation_id":       []string{"caller_stage_activation_id"},
		"caller_stage_activation_id": []string{"parent_invocation_id"},
	}
	invocations := objectMap("runtime_Invocation", 1, 1025)
	invocations["propertyNames"] = ref("Identifier")
	g.property("runtime_Run", "invocations", invocations)
	configurations := objectMap("runtime_EffectiveConfiguration", 1, 1024)
	configurations["propertyNames"] = map[string]any{"type": "string", "pattern": "^sha256:[a-f0-9]{64}$"}
	g.property("runtime_Run", "workflow_configurations", configurations)
	require("runtime_Run", "invocations", "workflow_configurations")
	for _, field := range []string{"input_artifacts", "output_artifacts"} {
		g.property("runtime_Run", field, portMap())
	}
	g.property("runtime_Stop", "scope", enum("run", "invocation"))
	g.property("runtime_Stop", "scope_id", ref("Identifier"))
	g.defs["runtime_Stop"].(map[string]any)["dependentRequired"] = map[string]any{
		"scope": []string{"scope_id"}, "scope_id": []string{"scope"},
	}
	g.property("runtime_NextView", "admission", map[string]any{"const": false})
	g.property("runtime_NextView", "read_only", map[string]any{"const": true})
	g.property("runtime_NextView", "action", enum("stage", "active", "cancel", "restricted", "resume_required", "blocked_child", "terminal", "uncertain", "idle"))
	for _, field := range []string{"workflow_invocation_id", "stage_id"} {
		// The optional fields are written by Next only for scoped work.
		g.property("runtime_NextView", field, ref("Identifier"))
	}
	g.defs["runtime_NextView"].(map[string]any)["allOf"] = []any{
		map[string]any{
			"if":   map[string]any{"properties": map[string]any{"action": map[string]any{"const": "stage"}}},
			"then": map[string]any{"required": []string{"workflow_invocation_id", "stage_id"}},
			"else": map[string]any{"not": map[string]any{"required": []string{"stage_id"}}},
		},
		map[string]any{
			"if":   map[string]any{"properties": map[string]any{"action": enum("stage", "active", "cancel", "restricted", "resume_required", "blocked_child")}},
			"then": map[string]any{"required": []string{"workflow_invocation_id"}, "properties": map[string]any{"work_id": ref("Identifier")}},
			"else": map[string]any{"not": map[string]any{"required": []string{"workflow_invocation_id"}}, "properties": map[string]any{"work_id": map[string]any{"const": ""}}},
		},
	}
	workflows := objectMap("runtime_WorkflowPreview", 1, 1024)
	workflows["propertyNames"] = map[string]any{"type": "string", "pattern": "^sha256:[a-f0-9]{64}$"}
	g.property("runtime_Preview", "workflows", workflows)
	require("runtime_Preview", "workflows")
	if _, exists := g.defs["runtime_RegistryFile"]; exists {
		g.property("runtime_RegistryFile", "entries", map[string]any{"type": "array", "items": ref("runtime_Definition"), "maxItems": 512})
		g.property("runtime_RegistryFile", "aliases", map[string]any{
			"type": "object", "maxProperties": 512,
			"propertyNames":        map[string]any{"type": "string", "minLength": 1, "maxLength": 128, "pattern": "^[^/\\\\\x00\r\n\t ]+$"},
			"additionalProperties": map[string]any{"type": "string", "minLength": 1, "maxLength": 4096},
		})
	}
	if _, exists := g.defs["runtime_ProfileCapabilities"]; exists {
		for _, field := range []string{"state_versions", "read_versions"} {
			g.property("runtime_ProfileCapabilities", field, map[string]any{"type": "array", "items": map[string]any{"type": "string", "minLength": 1}, "minItems": 1, "maxItems": 32, "uniqueItems": true})
		}
		require("runtime_ProfileCapabilities", "state_versions", "read_versions")
	}
}
