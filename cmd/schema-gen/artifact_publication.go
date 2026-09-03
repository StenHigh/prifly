package main

import (
	"reflect"

	"github.com/stenhigh/prifly/internal/flow"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// Artifact publication fields are removed before traversal for every earlier
// bundle. New Go fields therefore cannot silently widen delivered contracts.
func artifactPublicationField(t reflect.Type, name string) bool {
	return t == reflect.TypeFor[prifly.Run]() && name == "ArtifactPublications" ||
		t == reflect.TypeFor[prifly.PublishCommand]() && (name == "ItemKey" || name == "CandidatePath" || name == "ExpectedDigest" || name == "ExpectedSizeBytes") ||
		t == reflect.TypeFor[flow.Hook]() && name == "Artifact" ||
		t == reflect.TypeFor[prifly.HookReadView]() && name == "LatestArtifact"
}

func artifactPublicationConstraints(g *generator) {
	ref := func(name string) map[string]any { return map[string]any{"$ref": "#/$defs/" + name} }
	for name, version := range map[string]string{
		"runtime_Run":          prifly.CoreArtifactPublicationStateVersion,
		"runtime_RunView":      prifly.CoreArtifactPublicationReadVersion,
		"runtime_NextView":     prifly.CoreArtifactPublicationNextVersion,
		"runtime_Preview":      prifly.CoreArtifactPublicationPreviewVersion,
		"runtime_StepReadView": prifly.CoreArtifactPublicationStepReadVersion,
	} {
		g.property(name, "schema_version", map[string]any{"const": version})
	}
	g.property("runtime_ArtifactPublication", "schema_version", map[string]any{"const": prifly.ArtifactPublicationVersion})
	for _, name := range []string{"id", "attempt_id", "step_instance_id", "item_key", "actor_id"} {
		g.property("runtime_ArtifactPublication", name, ref("Identifier"))
	}
	g.property("runtime_ArtifactPublication", "hook", ref("PortName"))
	g.property("runtime_ArtifactPublication", "artifact_ref", ref("ArtifactRef"))
	g.property("runtime_ArtifactPublication", "schema_ref", ref("ImmutableRef"))
	g.property("runtime_ArtifactPublication", "format", enum("json", "blob"))
	g.property("runtime_ArtifactPublication", "media_type", map[string]any{"type": "string", "minLength": 1, "maxLength": 128})
	g.property("runtime_ArtifactPublication", "size_bytes", map[string]any{"type": "integer", "minimum": 0, "maximum": prifly.MaxArtifactBytes})
	g.property("runtime_ArtifactPublication", "classification", enum("public", "internal", "confidential"))
	g.property("runtime_ArtifactPublication", "consumption", enum("early", "after_producer_success"))
	g.property("runtime_ArtifactPublication", "accepted_sequence", map[string]any{"type": "integer", "minimum": 1, "maximum": 9007199254740991})
	g.property("runtime_ArtifactPublication", "content_check_evidence", map[string]any{"type": "array", "maxItems": 128, "items": ref("EvidenceRef")})
	g.property("runtime_Run", "artifact_publications", map[string]any{"type": "array", "maxItems": prifly.MaxRunPublications, "items": ref("runtime_ArtifactPublication")})

	g.property("flow_Hook", "kind", enum("state", "event", "artifact"))
	g.property("flow_Hook", "artifact", ref("flow_ArtifactHook"))
	g.defs["flow_Hook"].(map[string]any)["oneOf"] = []any{
		map[string]any{"properties": map[string]any{"kind": map[string]any{"enum": []string{"state", "event"}}}, "not": map[string]any{"required": []string{"artifact"}}},
		map[string]any{"properties": map[string]any{"kind": map[string]any{"const": "artifact"}, "artifact": ref("flow_ArtifactHook"), "allow_during_stop": map[string]any{"const": false}}, "required": []string{"artifact"}, "not": map[string]any{"required": []string{"freshness_ms"}}},
	}
	g.property("flow_ArtifactHook", "format", enum("json", "blob"))
	g.property("flow_ArtifactHook", "cardinality", enum("one", "keyed_many"))
	g.property("flow_ArtifactHook", "media_types", map[string]any{"type": "array", "minItems": 1, "maxItems": 1, "uniqueItems": true, "items": map[string]any{"type": "string", "minLength": 1, "maxLength": 128}})
	g.property("flow_ArtifactHook", "content_check_refs", map[string]any{"type": "array", "maxItems": 32, "uniqueItems": true, "items": ref("ImmutableRef")})
	g.defs["flow_ArtifactHook"].(map[string]any)["allOf"] = []any{
		map[string]any{"if": map[string]any{"properties": map[string]any{"format": map[string]any{"const": "json"}}}, "then": map[string]any{"not": map[string]any{"required": []string{"media_types"}}}},
		map[string]any{"if": map[string]any{"properties": map[string]any{"format": map[string]any{"const": "blob"}}}, "then": map[string]any{"required": []string{"media_types"}}},
	}
	g.property("runtime_HookReadView", "latest_artifact", ref("runtime_ArtifactPublication"))

	g.property("runtime_PublishCommand", "schema_version", map[string]any{"const": "2"})
	g.property("runtime_PublishCommand", "kind", enum("state", "event", "artifact"))
	g.property("runtime_PublishCommand", "expected_state_version", map[string]any{"type": "integer", "minimum": 0, "maximum": 9007199254740991})
	g.property("runtime_PublishCommand", "item_key", ref("Identifier"))
	g.property("runtime_PublishCommand", "candidate_path", map[string]any{"type": "string", "minLength": 1, "maxLength": 4096})
	g.property("runtime_PublishCommand", "expected_digest", ref("Digest"))
	g.property("runtime_PublishCommand", "expected_size_bytes", map[string]any{"type": "integer", "minimum": 0, "maximum": prifly.MaxArtifactBytes})
	artifactFields := []any{
		map[string]any{"required": []string{"item_key"}},
		map[string]any{"required": []string{"candidate_path"}},
		map[string]any{"required": []string{"expected_digest"}},
		map[string]any{"required": []string{"expected_size_bytes"}},
	}
	g.defs["runtime_PublishCommand"].(map[string]any)["oneOf"] = []any{
		map[string]any{"properties": map[string]any{"kind": map[string]any{"const": "state"}}, "required": []string{"expected_state_version", "value"}, "not": map[string]any{"anyOf": append([]any{map[string]any{"required": []string{"event_key"}}}, artifactFields...)}},
		map[string]any{"properties": map[string]any{"kind": map[string]any{"const": "event"}, "event_key": ref("Identifier")}, "required": []string{"event_key", "value"}, "not": map[string]any{"anyOf": append([]any{map[string]any{"required": []string{"expected_state_version"}}}, artifactFields...)}},
		map[string]any{"properties": map[string]any{"kind": map[string]any{"const": "artifact"}}, "required": []string{"candidate_path", "expected_digest", "expected_size_bytes"}, "not": map[string]any{"anyOf": []any{
			map[string]any{"required": []string{"expected_state_version"}}, map[string]any{"required": []string{"event_key"}}, map[string]any{"required": []string{"value"}},
		}}},
	}
}
