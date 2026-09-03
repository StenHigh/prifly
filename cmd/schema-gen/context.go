package main

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/stenhigh/prifly/internal/flow"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// P2-05 fields must be excluded before traversal of every published bundle.
func contextField(t reflect.Type, name string) bool {
	return t == reflect.TypeFor[prifly.Run]() && (name == "ContextResources" || name == "CheckExecutions" || name == "ActiveCheckID" || name == "PendingAcceptance") ||
		t == reflect.TypeFor[prifly.Preview]() && name == "CheckExecutors" ||
		t == reflect.TypeFor[prifly.Definition]() && (name == "ByteEncoding" || name == "MediaType") ||
		t == reflect.TypeFor[prifly.ExecutorConfig]() && name == "ContextProfileRef" ||
		t == reflect.TypeFor[prifly.PinnedExecutor]() && name == "ContextProfile" ||
		t == reflect.TypeFor[prifly.ContextManifest]() && (name == "Manifest" || name == "Rendering" || name == "Sources")
}

func contextConstraints(g *generator) {
	ref := func(name string) map[string]any { return map[string]any{"$ref": "#/$defs/" + name} }
	for name, version := range map[string]string{
		"runtime_Run": prifly.CoreContextStateVersion, "runtime_RunView": prifly.CoreContextReadVersion,
		"runtime_NextView": prifly.CoreContextNextVersion, "runtime_Preview": prifly.CoreContextPreviewVersion,
		"runtime_RegistryFile": "3",
	} {
		g.property(name, "schema_version", map[string]any{"const": version})
	}
	// Use the same immutable contracts as admission. Source schemas have their
	// own $id and local $defs, so their references retain a separate namespace.
	definitions, _, err := prifly.Builtins()
	if err != nil {
		panic(err)
	}
	for _, selected := range []struct{ name, id, version string }{
		{"runtime_Configuration", "core:schema/core-configuration", "2.0.0"},
		{"runtime_ContextManifest", "core:schema/local-context", "2.0.0"},
		{"runtime_ContextProfile", "core:schema/context-profile", "1.0.0"},
		{"runtime_SourceSnapshot", "core:schema/source-snapshot", "1.0.0"},
		{"runtime_ContextRequest", "core:schema/context-request", "1.0.0"},
	} {
		found := false
		for _, definition := range definitions {
			if definition.Ref.ID == selected.id && definition.Ref.Version == selected.version {
				var schema map[string]any
				if err := json.Unmarshal(definition.Bytes, &schema); err != nil {
					panic(err)
				}
				g.defs[selected.name], found = schema, true
				break
			}
		}
		if !found {
			panic(fmt.Sprintf("missing exact context schema %s@%s", selected.id, selected.version))
		}
	}
	// Present-null is not an omitted optional profile declaration.
	g.property("runtime_ExecutorConfig", "context_profile_ref", ref("ImmutableRef"))
	g.property("runtime_PinnedExecutor", "context_profile", ref("runtime_ContextProfile"))
	g.property("runtime_Run", "context_resources", map[string]any{"type": "array", "maxItems": 512, "items": ref("runtime_PinnedResource")})
	g.property("runtime_PinnedResource", "raw_digest", ref("Digest"))
	g.property("runtime_PinnedResource", "bytes", map[string]any{
		"type": "string", "contentEncoding": "base64", "maxLength": 4 * ((prifly.MaxDefinitionBytes + 2) / 3),
		"pattern": "^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$",
	})
	for _, name := range []string{"runtime_PinnedResource", "runtime_Definition"} {
		g.property(name, "byte_encoding", enum("json", "utf8_text"))
		g.property(name, "media_type", map[string]any{"type": "string", "minLength": 1, "maxLength": 128, "pattern": "^[^\r\n\x00]+$"})
		g.defs[name].(map[string]any)["allOf"] = []any{
			map[string]any{
				"if":   map[string]any{"required": []string{"byte_encoding"}, "properties": map[string]any{"byte_encoding": map[string]any{"const": "json"}}},
				"then": map[string]any{"properties": map[string]any{"media_type": map[string]any{"const": "application/json"}}},
			},
			map[string]any{
				"if":   map[string]any{"required": []string{"byte_encoding"}, "properties": map[string]any{"byte_encoding": map[string]any{"const": "utf8_text"}}},
				"then": map[string]any{"properties": map[string]any{"media_type": map[string]any{"pattern": "^[Tt][Ee][Xx][Tt]/[^*\r\n\x00]+$"}}},
			},
		}
	}
	g.property("runtime_Definition", "kind", enum("step", "schema", "workflow", "policy", "adapter", "resource", "check"))
	g.property("runtime_Definition", "path", map[string]any{"type": "string", "minLength": 1, "maxLength": 4096})
	definition := g.defs["runtime_Definition"].(map[string]any)
	definition["dependentRequired"] = map[string]any{"byte_encoding": []string{"media_type"}, "media_type": []string{"byte_encoding"}}
	definition["allOf"] = append(definition["allOf"].([]any), map[string]any{
		"if":   map[string]any{"required": []string{"byte_encoding"}},
		"then": map[string]any{"properties": map[string]any{"kind": map[string]any{"const": "resource"}}},
	})
	checkContractConstraints(g)
	acceptanceConstraints(g)
}

func acceptanceConstraints(g *generator) {
	data, err := flow.ProtocolSchema("ArtifactRevision")
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
	ref := func(name string) map[string]any { return map[string]any{"$ref": "#/$defs/" + name} }
	g.property("runtime_Run", "pending_acceptance", ref("runtime_PendingAcceptance"))
	for _, name := range []string{"runtime_Run", "runtime_Invocation"} {
		g.property(name, "status", enum("ready", "running", "waiting", "stopping", "verifying", "completed", "failed", "cancelled", "uncertain"))
	}
	g.property("runtime_Preview", "check_executors", map[string]any{"type": "object", "maxProperties": 512, "propertyNames": map[string]any{"type": "string", "minLength": 1, "maxLength": 512}, "additionalProperties": ref("runtime_ExecutorPreview")})
	for _, name := range []string{"id", "workflow_invocation_id", "stage_activation_id", "producer_attempt_id"} {
		g.property("runtime_PendingAcceptance", name, ref("Identifier"))
	}
	g.property("runtime_PendingAcceptance", "kind", enum("workflow_input", "step_input", "step_result", "workflow_output"))
	g.property("runtime_PendingAcceptance", "status", enum("pending", "passed"))
	g.property("runtime_PendingAcceptance", "candidate_ref", ref("ArtifactRef"))
	g.property("runtime_PendingAcceptance", "checked", ref("runtime_Observation"))
	g.property("runtime_PendingAcceptance", "bindings", map[string]any{"type": "object", "propertyNames": ref("PortName"), "maxProperties": 256, "additionalProperties": ref("ArtifactRef")})
	g.property("runtime_PendingAcceptance", "prepared_artifacts", map[string]any{"type": "object", "propertyNames": ref("PortName"), "maxProperties": 256, "additionalProperties": ref("ArtifactRevision")})
	g.property("runtime_PendingAcceptance", "checks", map[string]any{"type": "array", "minItems": 1, "maxItems": 1024, "items": ref("runtime_PendingCheck")})
	g.property("runtime_PendingCheck", "check_execution_id", ref("Identifier"))
	g.property("runtime_PendingCheck", "check_ref", ref("ImmutableRef"))
	g.property("runtime_PendingCheck", "boundary", enum("workflow_input", "step_input", "step_output", "step_result", "workflow_output"))
	g.property("runtime_PendingCheck", "port", ref("PortName"))
	g.property("runtime_PendingCheck", "subjects", map[string]any{"type": "array", "maxItems": prifly.MaxCheckSubjects, "uniqueItems": true, "items": ref("ArtifactRef")})
	g.defs["runtime_PendingCheck"].(map[string]any)["allOf"] = []any{map[string]any{
		"if":   map[string]any{"properties": map[string]any{"boundary": map[string]any{"const": "step_result"}}},
		"then": map[string]any{"not": map[string]any{"required": []string{"port"}}},
		"else": map[string]any{"required": []string{"port"}, "properties": map[string]any{"subjects": map[string]any{"minItems": 1, "maxItems": 1}}},
	}}
	g.defs["runtime_PendingAcceptance"].(map[string]any)["allOf"] = []any{
		map[string]any{
			"if":   map[string]any{"properties": map[string]any{"kind": map[string]any{"const": "workflow_input"}}},
			"then": map[string]any{"not": map[string]any{"required": []string{"stage_activation_id"}}},
			"else": map[string]any{"required": []string{"stage_activation_id"}},
		},
		map[string]any{
			"if":   map[string]any{"properties": map[string]any{"kind": map[string]any{"const": "step_result"}}},
			"then": map[string]any{"required": []string{"producer_attempt_id", "candidate_ref"}},
			"else": map[string]any{"not": map[string]any{"anyOf": []any{
				map[string]any{"required": []string{"producer_attempt_id"}}, map[string]any{"required": []string{"candidate_ref"}}, map[string]any{"required": []string{"prepared_artifacts"}},
			}}},
		},
		map[string]any{
			"if":   map[string]any{"properties": map[string]any{"status": map[string]any{"const": "passed"}}},
			"then": map[string]any{"required": []string{"checked"}},
			"else": map[string]any{"not": map[string]any{"required": []string{"checked"}}},
		},
	}
	for _, kind := range []string{"workflow_input", "step_input", "step_result", "workflow_output"} {
		boundaries := enum(kind)
		if kind == "step_result" {
			boundaries = enum("step_output", "step_result")
		}
		shape := g.defs["runtime_PendingAcceptance"].(map[string]any)
		shape["allOf"] = append(shape["allOf"].([]any), map[string]any{
			"if": map[string]any{"properties": map[string]any{"kind": map[string]any{"const": kind}}},
			"then": map[string]any{"properties": map[string]any{"checks": map[string]any{
				"items": map[string]any{"properties": map[string]any{"boundary": boundaries}},
			}}},
		})
	}
	g.property("runtime_NextView", "action", enum("stage", "active", "check", "acceptance", "cancel", "restricted", "resume_required", "blocked_child", "terminal", "uncertain", "idle"))
	g.defs["runtime_NextView"].(map[string]any)["allOf"] = []any{
		map[string]any{
			"if":   map[string]any{"properties": map[string]any{"action": map[string]any{"const": "stage"}}},
			"then": map[string]any{"required": []string{"workflow_invocation_id", "stage_id"}},
		},
		map[string]any{
			"if":   map[string]any{"properties": map[string]any{"action": map[string]any{"not": enum("stage", "acceptance")}}},
			"then": map[string]any{"not": map[string]any{"required": []string{"stage_id"}}},
		},
		map[string]any{
			"if":   map[string]any{"properties": map[string]any{"action": enum("stage", "active", "check", "acceptance", "cancel", "restricted", "resume_required", "blocked_child")}},
			"then": map[string]any{"required": []string{"workflow_invocation_id"}, "properties": map[string]any{"work_id": ref("Identifier")}},
			"else": map[string]any{"not": map[string]any{"required": []string{"workflow_invocation_id"}}, "properties": map[string]any{"work_id": map[string]any{"const": ""}}},
		},
	}
}

// These constraints mirror the closed check parsers. Exact request identity,
// subject identity uniqueness, deadline ordering and authority cannot be proved
// by a standalone JSON shape and remain mandatory runtime checks.
func checkContractConstraints(g *generator) {
	ref := func(name string) map[string]any { return map[string]any{"$ref": "#/$defs/" + name} }
	integer := func(minimum int) map[string]any {
		return map[string]any{"type": "integer", "minimum": minimum, "maximum": 9007199254740991}
	}
	for name, version := range map[string]string{
		"flow_CheckDefinition": flow.CheckDefinitionVersion,
		"runtime_CheckRequest": prifly.CheckRequestVersion,
		"runtime_CheckResult":  prifly.CheckResultVersion,
	} {
		g.property(name, "schema_version", map[string]any{"const": version})
	}
	g.property("flow_CheckDefinition", "id", ref("Identifier"))
	g.property("flow_CheckDefinition", "version", ref("Version"))
	g.property("flow_CheckDefinition", "title", map[string]any{"type": "string", "minLength": 1, "maxLength": 256})
	g.property("flow_CheckDefinition", "kind", enum("content", "result"))
	g.property("flow_CheckDefinition", "claim", enum("content_valid", "check_passed", "semantic_review"))
	g.property("flow_Executor", "operation", ref("Identifier"))
	definition := g.defs["flow_CheckDefinition"].(map[string]any)
	definition["if"] = map[string]any{"properties": map[string]any{"kind": map[string]any{"const": "content"}}}
	definition["then"] = map[string]any{"properties": map[string]any{"claim": map[string]any{"const": "content_valid"}}}
	definition["else"] = map[string]any{"properties": map[string]any{"claim": enum("check_passed", "semantic_review")}}
	for _, name := range []string{"check_execution_id", "run_id", "workflow_invocation_id", "stage_activation_id", "producer_attempt_id", "admission_id"} {
		g.property("runtime_CheckRequest", name, ref("Identifier"))
	}
	g.property("runtime_CheckRequest", "boundary", enum("workflow_input", "step_input", "step_output", "workflow_output", "step_result"))
	g.property("runtime_CheckRequest", "port", ref("PortName"))
	g.property("runtime_CheckRequest", "candidate_ref", ref("ArtifactRef"))
	g.property("runtime_CheckRequest", "context_manifest_ref", ref("ArtifactRef"))
	g.property("runtime_CheckRequest", "package_lock_digest", ref("Digest"))
	g.property("runtime_CheckRequest", "admitted_run_version", integer(1))
	g.property("runtime_CheckRequest", "control_epoch", integer(0))
	g.property("runtime_CheckRequest", "subjects", map[string]any{"type": "array", "items": ref("ArtifactRef"), "maxItems": prifly.MaxCheckSubjects, "uniqueItems": true})
	for _, name := range []string{"dispatch_not_after", "check_deadline"} {
		g.property("runtime_CheckRequest", name, ref("Timestamp"))
	}
	request := g.defs["runtime_CheckRequest"].(map[string]any)
	request["allOf"] = []any{
		map[string]any{
			"if":   map[string]any{"properties": map[string]any{"boundary": map[string]any{"const": "step_result"}}},
			"then": map[string]any{"required": []string{"candidate_ref"}, "not": map[string]any{"required": []string{"port"}}},
			"else": map[string]any{"required": []string{"port"}, "properties": map[string]any{"subjects": map[string]any{"minItems": 1, "maxItems": 1}}, "not": map[string]any{"required": []string{"candidate_ref"}}},
		},
		map[string]any{
			"if":   map[string]any{"properties": map[string]any{"boundary": map[string]any{"not": map[string]any{"const": "workflow_input"}}}},
			"then": map[string]any{"required": []string{"stage_activation_id"}},
			"else": map[string]any{"not": map[string]any{"required": []string{"stage_activation_id"}}},
		},
		map[string]any{
			"if":   map[string]any{"properties": map[string]any{"boundary": enum("step_output", "step_result")}},
			"then": map[string]any{"required": []string{"producer_attempt_id"}},
			"else": map[string]any{"not": map[string]any{"required": []string{"producer_attempt_id"}}},
		},
	}
	for _, name := range []string{"check_execution_id", "run_id"} {
		g.property("runtime_CheckResult", name, ref("Identifier"))
	}
	g.property("runtime_CheckResult", "request_digest", ref("Digest"))
	g.property("runtime_CheckResult", "status", enum("pass", "fail", "inconclusive"))
	g.property("runtime_CheckResult", "summary", map[string]any{"type": "string", "maxLength": 16000})
	g.property("runtime_CheckResult", "limitations", map[string]any{"type": "array", "maxItems": 128, "items": map[string]any{"type": "string", "minLength": 1, "maxLength": 4096}})

	g.property("runtime_Run", "check_executions", map[string]any{"type": "object", "propertyNames": ref("Identifier"), "maxProperties": 1024, "additionalProperties": ref("runtime_CheckExecution")})
	g.property("runtime_Run", "active_check_execution_id", ref("Identifier"))
	g.property("runtime_CheckExecution", "id", ref("Identifier"))
	g.property("runtime_CheckExecution", "workspace", map[string]any{"type": "string", "minLength": 1, "maxLength": 4096})
	g.property("runtime_CheckExecution", "status", enum("pending", "dispatching", "running", "stopping", "verifying", "completed", "failed", "cancelled", "uncertain"))
	g.property("local_BlobRef", "digest", ref("Digest"))
	g.property("local_BlobRef", "size", map[string]any{"type": "integer", "minimum": 0, "maximum": prifly.MaxArtifactBytes})
	g.property("runtime_CheckExecution", "request_bytes", map[string]any{"allOf": []any{
		ref("local_BlobRef"), map[string]any{"properties": map[string]any{"size": map[string]any{"minimum": 1, "maximum": prifly.MaxCheckWireBytes}}},
	}})
	for _, name := range []string{"dispatch", "started", "executor_end", "settled"} {
		g.property("runtime_CheckExecution", name, ref("runtime_Observation"))
	}
	for field, name := range map[string]string{
		"process": "local_ProcessIdentity", "process_outcome": "local_ProcessOutcome", "report": "runtime_CheckResult", "report_bytes": "local_BlobRef",
	} {
		g.property("runtime_CheckExecution", field, ref(name))
	}
	// Metadata reads omit the token hash. Persisted dispatch additionally
	// requires it; a public read shape must not demand a redacted secret.
	g.property("runtime_CheckExecution", "token_hash", ref("Digest"))
	g.property("runtime_CheckExecution", "failure", ref("Identifier"))
	execution := g.defs["runtime_CheckExecution"].(map[string]any)
	execution["dependentRequired"] = map[string]any{"started": []string{"process"}}
	execution["allOf"] = []any{
		map[string]any{
			"if":   map[string]any{"properties": map[string]any{"status": enum("completed", "failed", "cancelled")}},
			"then": map[string]any{"required": []string{"settled"}},
			"else": map[string]any{"not": map[string]any{"required": []string{"settled"}}},
		},
		map[string]any{
			"if":   map[string]any{"properties": map[string]any{"status": map[string]any{"const": "completed"}}},
			"then": map[string]any{"required": []string{"report", "report_bytes", "process_outcome"}},
			"else": map[string]any{"not": map[string]any{"required": []string{"report"}}},
		},
	}
}
