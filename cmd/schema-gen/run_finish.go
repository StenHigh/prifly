package main

import (
	"reflect"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// runFinishField names what only next 36 carries: where a Run that reached an
// outcome stopped, and the declared edge that reached it. Bundles published
// before it describe the next-action answer without it, byte for byte.
func runFinishField(t reflect.Type, name string) bool {
	return t == reflect.TypeFor[prifly.NextView]() && name == "Finish"
}

func runFinishConstraints(g *generator) {
	// Only the answer moves. Nothing new is recorded, so the state, the read,
	// the preview and the step read keep exactly what 35 published — which is
	// also why a Run started before this build answers under 36.
	g.property("runtime_NextView", "schema_version", map[string]any{"const": prifly.CoreRunFinishNextVersion})
	g.describe("runtime_NextView", "finish", "For a Run that reached an outcome, where its graph stopped: the invocation, the finish stage it ended at, and the outcome the Run reports — which a recorded waiver may have reduced from the one that stage declares. Absent for every other action, and for a Run that stopped without reaching a finish stage — a failed or cancelled Run names what stopped it in the read view instead.")
	g.describe("runtime_RunFinish", "from_stage_id", "The stage the declared edge into this finish came from. Present only when the sealed plan's own routing names exactly one such edge among the stages this Run settled; absent means this build cannot name one, never that there was none.")
	g.describe("runtime_RunFinish", "verdict", "The verdict that took the declared edge into this finish. Present and absent together with from_stage_id.")
}
