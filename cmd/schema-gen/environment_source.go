package main

import (
	"reflect"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// environmentSourceField names what only state/read 32 carries: the place a
// pinned executor reads a value from instead of carrying it. Every earlier
// bundle describes a sealed config without it, byte for byte.
func environmentSourceField(t reflect.Type, name string) bool {
	return t == reflect.TypeFor[prifly.ExecutorConfig]() && name == "EnvironmentFrom"
}

func environmentSourceConstraints(g *generator) {
	// Only the state and the read that show a sealed executor config change.
	// The next action, the preview and the step read never carried one, so
	// they keep the versions published with 31.
	for name, version := range map[string]string{
		"runtime_Run": prifly.CoreEnvironmentSourceStateVersion, "runtime_RunView": prifly.CoreEnvironmentSourceReadVersion,
	} {
		g.property(name, "schema_version", map[string]any{"const": version})
	}
	g.describe("runtime_ExecutorConfig", "environment_from", "For each name, the one place the engine reads that value from when the program starts: an environment variable of the caller, a whole file, or one key of a NAME=value file. The value itself is never sealed, never recorded and never shown; a source that is absent or empty refuses the start by name.")
}
