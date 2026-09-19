package main

import (
	"reflect"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// modelProfileField names what only the model-profile bundle carries: what a
// step's author asked of the model that executes it. Bundles published before
// it describe the task without it, byte for byte.
func modelProfileField(t reflect.Type, name string) bool {
	return t == reflect.TypeFor[prifly.SessionTask]() && name == "ModelProfile"
}

func modelProfileConstraints(g *generator) {
	g.property("runtime_SessionTask", "schema_version", map[string]any{"const": prifly.AssistedSessionModelProfileVersion})
	g.describe("runtime_SessionTask", "model_profile", "What this step's author asked of the model that executes it, read from the sealed plan. A profile of work, not a provider or product. Absent means the step asked nothing, never that the host may decide silently. This authority selects no model: an assisted session exists before the Run and it holds no channel to it.")
}
