package main

import (
	"reflect"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// Closure fields are removed before traversal for every earlier bundle.
func artifactClosureField(t reflect.Type, name string) bool {
	return t == reflect.TypeFor[prifly.Run]() && name == "ArtifactClosures" ||
		t == reflect.TypeFor[prifly.PublishCommand]() && name == "ItemKeys" ||
		t == reflect.TypeFor[prifly.HookReadView]() && name == "LatestClosure"
}

func artifactClosureConstraints(g *generator) {
	ref := func(name string) map[string]any { return map[string]any{"$ref": "#/$defs/" + name} }
	for name, version := range map[string]string{
		"runtime_Run":          prifly.CoreArtifactClosureStateVersion,
		"runtime_RunView":      prifly.CoreArtifactClosureReadVersion,
		"runtime_NextView":     prifly.CoreArtifactClosureNextVersion,
		"runtime_Preview":      prifly.CoreArtifactClosurePreviewVersion,
		"runtime_StepReadView": prifly.CoreArtifactClosureStepReadVersion,
	} {
		g.property(name, "schema_version", map[string]any{"const": version})
	}
	// Earlier additive bundles retained the repeat-era enum even after the
	// runtime added these activation kinds. V13 describes the state it emits;
	// published earlier bundles remain byte-stable.
	g.property("runtime_Activation", "kind", enum("step", "choice", "call", "repeat", "parallel", "map", "wait", "finish"))

	g.property("runtime_ArtifactManifest", "schema_version", map[string]any{"const": prifly.ArtifactManifestVersion})
	for _, name := range []string{"run_id", "step_instance_id"} {
		g.property("runtime_ArtifactManifest", name, ref("Identifier"))
	}
	g.property("runtime_ArtifactManifest", "hook", ref("PortName"))
	g.property("runtime_ArtifactManifest", "item_count", map[string]any{"type": "integer", "minimum": 0, "maximum": prifly.MaxRunPublications})
	g.property("runtime_ArtifactManifest", "cut_sequence", map[string]any{"type": "integer", "minimum": 0, "maximum": 9007199254740991})
	g.property("runtime_ArtifactManifest", "items", map[string]any{"type": "array", "maxItems": prifly.MaxRunPublications, "items": ref("runtime_ArtifactPublication")})

	g.property("runtime_ArtifactClosure", "schema_version", map[string]any{"const": prifly.ArtifactClosureVersion})
	for _, name := range []string{"id", "attempt_id", "step_instance_id", "actor_id"} {
		g.property("runtime_ArtifactClosure", name, ref("Identifier"))
	}
	g.property("runtime_ArtifactClosure", "hook", ref("PortName"))
	g.property("runtime_ArtifactClosure", "item_keys", map[string]any{"type": "array", "maxItems": prifly.MaxRunPublications, "uniqueItems": true, "items": ref("Identifier")})
	g.property("runtime_ArtifactClosure", "manifest_ref", ref("ArtifactRef"))
	g.property("runtime_ArtifactClosure", "item_count", map[string]any{"type": "integer", "minimum": 0, "maximum": prifly.MaxRunPublications})
	g.property("runtime_ArtifactClosure", "cut_sequence", map[string]any{"type": "integer", "minimum": 0, "maximum": 9007199254740991})
	g.property("runtime_ArtifactClosure", "accepted_sequence", map[string]any{"type": "integer", "minimum": 1, "maximum": 9007199254740991})
	g.property("runtime_Run", "artifact_closures", map[string]any{"type": "array", "maxItems": prifly.MaxRunPublications, "items": ref("runtime_ArtifactClosure")})
	g.property("runtime_HookReadView", "latest_closure", ref("runtime_ArtifactClosure"))

	g.property("runtime_PublishCommand", "schema_version", map[string]any{"const": "3"})
	g.property("runtime_PublishCommand", "kind", enum("state", "event", "artifact", "close"))
	g.property("runtime_PublishCommand", "item_keys", map[string]any{"type": "array", "maxItems": prifly.MaxRunPublications, "uniqueItems": true, "items": ref("Identifier")})
	required := func(names ...string) []any {
		out := make([]any, 0, len(names))
		for _, name := range names {
			out = append(out, map[string]any{"required": []string{name}})
		}
		return out
	}
	stateFields := required("expected_state_version")
	eventFields := required("event_key")
	valueFields := required("value")
	artifactFields := required("item_key", "candidate_path", "expected_digest", "expected_size_bytes")
	closeFields := required("item_keys")
	g.defs["runtime_PublishCommand"].(map[string]any)["oneOf"] = []any{
		map[string]any{"properties": map[string]any{"kind": map[string]any{"const": "state"}}, "required": []string{"expected_state_version", "value"}, "not": map[string]any{"anyOf": append(append(eventFields, artifactFields...), closeFields...)}},
		map[string]any{"properties": map[string]any{"kind": map[string]any{"const": "event"}, "event_key": ref("Identifier")}, "required": []string{"event_key", "value"}, "not": map[string]any{"anyOf": append(append(stateFields, artifactFields...), closeFields...)}},
		map[string]any{"properties": map[string]any{"kind": map[string]any{"const": "artifact"}}, "required": []string{"candidate_path", "expected_digest", "expected_size_bytes"}, "not": map[string]any{"anyOf": append(append(stateFields, eventFields...), append(valueFields, closeFields...)...)}},
		map[string]any{"properties": map[string]any{"kind": map[string]any{"const": "close"}}, "required": []string{"item_keys"}, "not": map[string]any{"anyOf": append(append(append(stateFields, eventFields...), valueFields...), artifactFields...)}},
	}
}
