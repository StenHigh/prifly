package main

import (
	"reflect"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// AIF-04 fields must be excluded before traversal of every earlier bundle, so
// the delivered context contracts keep their exact published bytes.
func sessionField(t reflect.Type, name string) bool {
	return t == reflect.TypeFor[prifly.Attempt]() && name == "Session"
}

func sessionConstraints(g *generator) {
	ref := func(name string) map[string]any { return map[string]any{"$ref": "#/$defs/" + name} }
	for name, version := range map[string]string{
		"runtime_Run": prifly.CoreSessionStateVersion, "runtime_RunView": prifly.CoreSessionReadVersion,
		"runtime_NextView": prifly.CoreSessionNextVersion, "runtime_Preview": prifly.CoreSessionPreviewVersion,
	} {
		g.property(name, "schema_version", map[string]any{"const": version})
	}
	for _, name := range []string{"runtime_SessionHandoff", "runtime_SessionTask", "runtime_SessionSubmission"} {
		g.property(name, "schema_version", map[string]any{"const": prifly.AssistedSessionVersion})
	}
	// A handoff records who holds the work and whether they ever answered. The
	// missing report is its own state, never an implied outcome.
	g.property("runtime_SessionHandoff", "host_state", enum(prifly.SessionAwaiting, prifly.SessionReported, prifly.SessionDisconnected))
	g.property("runtime_SessionHandoff", "principal_id", ref("Identifier"))
	g.property("runtime_SessionHandoff", "claim_id", ref("Identifier"))
	g.property("runtime_SessionTask", "principal_id", ref("Identifier"))
	g.property("runtime_SessionTask", "claim_id", ref("Identifier"))
	g.property("runtime_SessionSubmission", "attempt_id", ref("Identifier"))
	g.property("runtime_SessionSubmission", "envelope_digest", ref("Digest"))
	g.property("runtime_SessionTask", "envelope_digest", ref("Digest"))
	g.property("runtime_SessionTask", "permitted_effects", map[string]any{
		"type": "array", "maxItems": 8, "uniqueItems": true,
		"items": enum("write_inside_claimed_worktree", "local_git_commit_on_claimed_base"),
	})
}
