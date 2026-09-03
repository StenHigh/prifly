package main

import (
	"reflect"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func actionIntentField(t reflect.Type, name string) bool {
	return t == reflect.TypeFor[prifly.Run]() && name == "ActionIntents"
}

func actionIntentConstraints(g *generator) {
	for name, version := range map[string]string{
		"runtime_Run":          prifly.CoreActionIntentStateVersion,
		"runtime_RunView":      prifly.CoreActionIntentReadVersion,
		"runtime_NextView":     prifly.CoreActionIntentNextVersion,
		"runtime_Preview":      prifly.CoreActionIntentPreviewVersion,
		"runtime_StepReadView": prifly.CoreActionIntentStepReadVersion,
	} {
		g.property(name, "schema_version", map[string]any{"const": version})
	}
}
