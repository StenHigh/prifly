package main

import (
	"reflect"
	"slices"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// awaitingHostField names what only next 42 carries: the command that takes a
// task waiting for its host, and what each step declared it may change.
// Bundles published before it describe the answer without them.
func awaitingHostField(t reflect.Type, name string) bool {
	return t == reflect.TypeFor[prifly.NextView]() && (name == "NextCommand" || name == "Effects" || name == "StepEffects")
}

func awaitingHostConstraints(g *generator) {
	g.property("runtime_NextView", "schema_version", map[string]any{"const": prifly.CoreAwaitingHostNextVersion})
	actions := g.defs["runtime_NextView"].(map[string]any)["properties"].(map[string]any)["action"].(map[string]any)["enum"].([]string)
	g.property("runtime_NextView", "action", enum(append(actions, "awaiting_host")...))
	// awaiting_host names its attempt in work_id and its invocation, as the
	// actions that carry work do; the rule that says which ones do is
	// widened, not written a second time.
	for _, rule := range g.defs["runtime_NextView"].(map[string]any)["allOf"].([]any) {
		condition := rule.(map[string]any)["if"].(map[string]any)["properties"].(map[string]any)["action"].(map[string]any)
		if carrying, ok := condition["enum"].([]string); ok && slices.Contains(carrying, "blocked_child") {
			condition["enum"] = append(slices.Clone(carrying), "awaiting_host")
		}
	}
	g.describe("runtime_NextView", "action", "What the Run has to do now. awaiting_host: the driver has nothing to do and a task handed to a host waits for it -- next_command takes it. idle: nobody waits on anybody; before 42 a waiting task also answered idle, which readers took for nothing to do.")
	g.describe("runtime_NextView", "next_command", "For awaiting_host, the prifly command that takes the waiting task, without the global flags the reader already uses. Absent for every other action.")
	g.describe("runtime_NextView", "effects", "What the step of the current action declares it may change, from the sealed plan: a ready stage's before it runs, a held attempt's while it runs. A declaration, not an observation of what the step did.")
	g.describe("runtime_NextView", "step_effects", "Every step instance of the Run with its stage and what its sealed definition declared it may change: effects class, retry_class and, for external_write, the system, operations and target. A declaration, not an observation.")
}
