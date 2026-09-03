package main

import (
	"reflect"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func workspaceTreeField(t reflect.Type, name string) bool {
	return t == reflect.TypeFor[prifly.SessionHandoff]() && name == "WorkspaceTrees" ||
		t == reflect.TypeFor[prifly.SessionTask]() && name == "WorkspaceTrees" ||
		t == reflect.TypeFor[prifly.SessionSubmission]() && name == "WorkspaceTrees"
}

func workspaceTreeConstraints(g *generator) {
	for name, version := range map[string]string{
		"runtime_Run": prifly.CoreWorkspaceTreeStateVersion, "runtime_RunView": prifly.CoreWorkspaceTreeReadVersion,
		"runtime_NextView": prifly.CoreWorkspaceTreeNextVersion, "runtime_Preview": prifly.CoreWorkspaceTreePreviewVersion,
		"runtime_StepReadView": prifly.CoreWorkspaceTreeStepReadVersion,
	} {
		g.property(name, "schema_version", map[string]any{"const": version})
	}
	for _, name := range []string{"runtime_SessionHandoff", "runtime_SessionTask", "runtime_SessionSubmission"} {
		g.property(name, "schema_version", map[string]any{"const": prifly.AssistedSessionTreeVersion})
	}
}
