package main

import (
	"reflect"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// externalWriteField names what only this bundle carries: the boundary a step
// declared for the one effect this authority cannot observe. Bundles published
// before it describe the handoff without it, byte for byte.
func externalWriteField(t reflect.Type, name string) bool {
	return t == reflect.TypeFor[prifly.SessionTask]() && name == "ExternalWrite"
}

func externalWriteConstraints(g *generator) {
	for name, version := range map[string]string{
		"runtime_Run": prifly.CoreExternalWriteStateVersion, "runtime_RunView": prifly.CoreExternalWriteReadVersion,
	} {
		g.property(name, "schema_version", map[string]any{"const": version})
	}
	g.describe("runtime_SessionTask", "external_write", "For a step declaring effects.class external_write, the boundary its author declared: the system, the changing operations and the exact target. The values are opaque to this authority -- it carries them to the host as the permission and its bounds, reaches nothing itself and reads no meaning from them. Operations name changes only: reading the same system is not a change, needs no permission from here, and its absence from the list forbids nothing. Nothing in this authority observes the change or receipts it; what the host did is what the host reports.")
}
