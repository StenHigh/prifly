package main

import (
	"reflect"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// effectsSessionField names what only the effects handoff carries. Bundles
// published before it described a handoff that recorded no workspace marks,
// and must keep describing exactly that handoff.
func effectsSessionField(t reflect.Type, name string) bool {
	return t == reflect.TypeFor[prifly.SessionHandoff]() && (name == "WorkspaceMarks" || name == "WorkspaceStatus")
}

func effectsSessionConstraints(g *generator) {
	for name, version := range map[string]string{
		"runtime_Run": prifly.CoreEffectsStateVersion, "runtime_RunView": prifly.CoreEffectsReadVersion,
		"runtime_NextView": prifly.CoreEffectsNextVersion, "runtime_Preview": prifly.CoreEffectsPreviewVersion,
		"runtime_StepReadView": prifly.CoreEffectsStepReadVersion,
	} {
		g.property(name, "schema_version", map[string]any{"const": version})
	}
	// The host contract is unchanged: session 7 tasks and submissions are what
	// an effects-state Run hands and accepts. What the state adds lives on the
	// handoff record the engine keeps for itself.
	marks := map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}}
	g.property("runtime_SessionHandoff", "workspace_marks", marks)
	g.property("runtime_SessionHandoff", "workspace_status", marks)
	g.describe("runtime_SessionHandoff", "workspace_marks", "By claim id, a digest of how each workspace the Run holds stood when this step was handed over: sha256 over `git rev-parse HEAD` and `git status --porcelain=v1 --untracked-files=all --no-renames`. Recorded only for a step permitted no workspace effect; its report is refused with effect_not_permitted if any mark differs at submission.")
	g.describe("runtime_SessionHandoff", "workspace_status", "By claim id, the porcelain listing behind workspace_marks, kept so a refusal names the paths that moved rather than only that some did.")
}
