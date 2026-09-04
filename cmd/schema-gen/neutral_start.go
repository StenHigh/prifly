package main

import (
	"slices"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func neutralStartConstraints(g *generator) {
	for name, version := range map[string]string{
		"runtime_Run": prifly.CoreNeutralStateVersion, "runtime_RunView": prifly.CoreNeutralReadVersion,
		"runtime_NextView": prifly.CoreNeutralNextVersion, "runtime_Preview": prifly.CoreNeutralPreviewVersion,
		"runtime_StepReadView": prifly.CoreNeutralStepReadVersion,
	} {
		g.property(name, "schema_version", map[string]any{"const": version})
	}
	run := g.defs["runtime_Run"].(map[string]any)
	run["required"] = slices.DeleteFunc(run["required"].([]string), func(name string) bool { return name == "brief_ref" })
}
