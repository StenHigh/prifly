package main

import (
	"reflect"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// stageWorkField names what only read 31 carries: the kind of work a ready
// stage holds. Bundles published before it describe an answer without it, byte
// for byte.
func stageWorkField(t reflect.Type, name string) bool {
	return t == reflect.TypeFor[prifly.NextView]() && name == "StageWork"
}

func stageWorkConstraints(g *generator) {
	for name, version := range map[string]string{
		"runtime_Run": prifly.CoreStageWorkStateVersion, "runtime_RunView": prifly.CoreStageWorkReadVersion,
		"runtime_NextView": prifly.CoreStageWorkNextVersion, "runtime_Preview": prifly.CoreStageWorkPreviewVersion,
		"runtime_StepReadView": prifly.CoreStageWorkStepReadVersion,
	} {
		g.property(name, "schema_version", map[string]any{"const": version})
	}
	g.property("runtime_NextView", "stage_work", map[string]any{"enum": []any{prifly.StageWorkAssistedSession, prifly.StageWorkProgram, prifly.StageWorkControl}})
	g.describe("runtime_NextView", "stage_work", "For a ready stage, the work the driver would do: an assisted step it hands to a host, a program it runs to completion inside the call, or a control stage that only moves the graph. Absent when this build cannot tell, and for every other action.")
}
