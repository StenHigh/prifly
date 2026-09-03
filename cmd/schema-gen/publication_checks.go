package main

import (
	"reflect"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// Pending candidates exist only in the checked-publication wire contract.
func publicationChecksField(t reflect.Type, name string) bool {
	return t == reflect.TypeFor[prifly.Run]() && name == "PendingArtifactPublication"
}

func publicationChecksConstraints(g *generator) {
	ref := func(name string) map[string]any { return map[string]any{"$ref": "#/$defs/" + name} }
	for name, version := range map[string]string{
		"runtime_Run":          prifly.CorePublicationChecksStateVersion,
		"runtime_RunView":      prifly.CorePublicationChecksReadVersion,
		"runtime_NextView":     prifly.CorePublicationChecksNextVersion,
		"runtime_Preview":      prifly.CorePublicationChecksPreviewVersion,
		"runtime_StepReadView": prifly.CorePublicationChecksStepReadVersion,
	} {
		g.property(name, "schema_version", map[string]any{"const": version})
	}
	for _, name := range []string{"id", "command_id", "attempt_id", "step_instance_id", "stage_activation_id", "item_key"} {
		g.property("runtime_PendingArtifactPublication", name, ref("Identifier"))
	}
	g.property("runtime_PendingArtifactPublication", "hook", ref("PortName"))
	g.property("runtime_PendingArtifactPublication", "artifact_ref", ref("ArtifactRef"))
	g.property("runtime_PendingArtifactPublication", "schema_ref", ref("ImmutableRef"))
	g.property("runtime_PendingArtifactPublication", "format", enum("json", "blob"))
	g.property("runtime_PendingArtifactPublication", "media_type", map[string]any{"type": "string", "minLength": 1, "maxLength": 128})
	g.property("runtime_PendingArtifactPublication", "size_bytes", map[string]any{"type": "integer", "minimum": 0, "maximum": prifly.MaxArtifactBytes})
	g.property("runtime_PendingArtifactPublication", "check_execution_ids", map[string]any{"type": "array", "minItems": 1, "maxItems": 128, "uniqueItems": true, "items": ref("Identifier")})
	g.property("runtime_Run", "pending_artifact_publication", ref("runtime_PendingArtifactPublication"))
	g.property("runtime_CheckRequest", "boundary", enum("workflow_input", "step_input", "step_output", "workflow_output", "step_result", "artifact_publication"))
	g.defs["runtime_CheckRequest"].(map[string]any)["allOf"] = []any{
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
			"if":   map[string]any{"properties": map[string]any{"boundary": enum("step_output", "step_result", "artifact_publication")}},
			"then": map[string]any{"required": []string{"producer_attempt_id"}},
			"else": map[string]any{"not": map[string]any{"required": []string{"producer_attempt_id"}}},
		},
	}
	g.property("runtime_NextView", "action", enum("stage", "active", "check", "acceptance", "publication_checks", "cancel", "restricted", "resume_required", "blocked_child", "terminal", "uncertain", "idle"))
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
			"if":   map[string]any{"properties": map[string]any{"action": enum("stage", "active", "check", "acceptance", "publication_checks", "cancel", "restricted", "resume_required", "blocked_child")}},
			"then": map[string]any{"required": []string{"workflow_invocation_id"}, "properties": map[string]any{"work_id": ref("Identifier")}},
			"else": map[string]any{"not": map[string]any{"required": []string{"workflow_invocation_id"}}, "properties": map[string]any{"work_id": map[string]any{"const": ""}}},
		},
	}
}
