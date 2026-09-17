package main

import (
	"reflect"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// materializedSessionField names what only state/read 30 carries: the entries
// a materialize-only handoff placed, and the failure summary of a stopped Run.
// Every earlier bundle describes a handoff and a view without them, byte for
// byte.
func materializedSessionField(t reflect.Type, name string) bool {
	return t == reflect.TypeFor[prifly.WorkspaceTreeHandoff]() && name == "MaterializedEntries" || t == reflect.TypeFor[prifly.RunView]() && name == "Failure"
}

// materializedSessionRequired names the field that became omitempty only so a
// materialize-only handoff may leave it out. Every earlier bundle still
// requires it: a handoff always captured through an output port.
func materializedSessionRequired(t reflect.Type, name string) bool {
	return t == reflect.TypeFor[prifly.WorkspaceTreeHandoff]() && name == "OutputPort"
}

func materializedSessionConstraints(g *generator) {
	for name, version := range map[string]string{
		"runtime_Run": prifly.CoreMaterializedStateVersion, "runtime_RunView": prifly.CoreMaterializedReadVersion,
		"runtime_NextView": prifly.CoreMaterializedNextVersion, "runtime_Preview": prifly.CoreMaterializedPreviewVersion,
		"runtime_StepReadView": prifly.CoreMaterializedStepReadVersion,
	} {
		g.property(name, "schema_version", map[string]any{"const": version})
	}
	g.describe("runtime_WorkspaceTreeHandoff", "materialized_entries", "Workspace-relative files the engine placed for a binding that names input_port and no output_port; settlement takes back exactly these, and only while their bytes still equal the pinned entry.")
	g.describe("runtime_RunView", "failure", "For a failed or cancelled Run, the diagnostic that stopped it: its code, id, and the attempt and step it belongs to. Absent for a completed or unfinished Run; derived from diagnostics at read time.")
}
