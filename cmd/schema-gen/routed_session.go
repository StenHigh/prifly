package main

import (
	"reflect"
	"slices"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// routedSessionField names what only the routed handoff carries. Bundles
// published before it never mentioned the verdicts a node routes, and must
// keep describing exactly the task they described.
func routedSessionField(t reflect.Type, name string) bool {
	return t == reflect.TypeFor[prifly.SessionTask]() && name == "RoutedVerdicts"
}

// routedSessionRequired names a field that became omitempty only so the routed
// task may omit it. Every earlier bundle still requires it, byte for byte: a
// legacy task always carried a deadline string, even when it was empty.
func routedSessionRequired(t reflect.Type, name string) bool {
	return t == reflect.TypeFor[prifly.SessionTask]() && name == "Deadline"
}

func routedSessionConstraints(g *generator) {
	for name, version := range map[string]string{
		"runtime_Run": prifly.CoreRoutedStateVersion, "runtime_RunView": prifly.CoreRoutedReadVersion,
		"runtime_NextView": prifly.CoreRoutedNextVersion, "runtime_Preview": prifly.CoreRoutedPreviewVersion,
		"runtime_StepReadView": prifly.CoreRoutedStepReadVersion,
	} {
		g.property(name, "schema_version", map[string]any{"const": version})
	}
	for _, name := range []string{"runtime_SessionHandoff", "runtime_SessionTask", "runtime_SessionSubmission"} {
		g.property(name, "schema_version", map[string]any{"const": prifly.AssistedSessionRoutedVersion})
	}
	// State 28 hands every assisted step the same contract, so an attempt no
	// longer chooses between a legacy and a timed handoff shape. What separates
	// a declared allowance from an inherited absolute deadline is the timing a
	// handoff carries, which is therefore no longer required of every one.
	g.property("runtime_Attempt", "session", nullable(map[string]any{"$ref": "#/$defs/runtime_SessionHandoff"}))
	delete(g.defs, "runtime_LegacySessionHandoff")
	for _, optional := range []struct{ def, field string }{{"runtime_SessionHandoff", "timing"}, {"runtime_SessionTask", "delivery"}} {
		definition := g.defs[optional.def].(map[string]any)
		definition["required"] = slices.DeleteFunc(definition["required"].([]string), func(name string) bool { return name == optional.field })
	}
	task := g.defs["runtime_SessionTask"].(map[string]any)
	task["required"] = append(task["required"].([]string), "routed_verdicts")
	g.property("runtime_SessionTask", "routed_verdicts", map[string]any{
		"type": "array", "items": enum("pass", "fail", "needs_revision", "no_work"),
		"minItems": 1, "maxItems": 4, "uniqueItems": true,
	})
	g.describe("runtime_SessionTask", "routed_verdicts", "The StepResult verdicts this node declares a route for, in StepResult order. Every other verdict is a legal StepResult that this node cannot receive: such a report is accepted and the invocation then fails routing, losing the work already accepted. Where a route leads is not disclosed.")
	g.describe("runtime_SessionTask", "deadline", "The deadline actually in force for this delivery, whether the step declared session limits or inherited the runtime default. Absence means no deadline exists; it is never an empty string.")
}
