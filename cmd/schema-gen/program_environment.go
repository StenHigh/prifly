package main

import (
	"reflect"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// programEnvironmentField names what only read 33 carries: the composition of
// the environment a ready program stage would hand its program. Bundles
// published before it describe the answer without it, byte for byte.
func programEnvironmentField(t reflect.Type, name string) bool {
	return t == reflect.TypeFor[prifly.NextView]() && name == "ProgramEnvironment"
}

func programEnvironmentConstraints(g *generator) {
	// Only the answer moves. Nothing new is recorded, so the state, the read,
	// the preview and the step read keep exactly what 32 published.
	g.property("runtime_NextView", "schema_version", map[string]any{"const": prifly.CoreProgramEnvironmentNextVersion})
	g.describe("runtime_NextView", "program_environment", "For a ready stage whose work is a program, what that program would be given: every variable by name, and for a value read at dispatch the place it comes from. Never a value. Absent for every other action, and for a Run this build did not stamp with it.")
}
