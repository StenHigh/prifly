package main

import (
	"reflect"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func workspaceField(t reflect.Type, name string) bool {
	return t == reflect.TypeFor[prifly.SessionHandoff]() && name == "WorkspaceMode" ||
		t == reflect.TypeFor[prifly.SessionTask]() && (name == "WorkspaceMode" || name == "RepositoryWorkspace")
}

func workspaceConstraints(g *generator) {
	for name, version := range map[string]string{
		"runtime_Run": prifly.CoreWorkspaceStateVersion, "runtime_RunView": prifly.CoreWorkspaceReadVersion,
		"runtime_NextView": prifly.CoreWorkspaceNextVersion, "runtime_Preview": prifly.CoreWorkspacePreviewVersion,
		"runtime_StepReadView": prifly.CoreWorkspaceStepReadVersion,
	} {
		g.property(name, "schema_version", map[string]any{"const": version})
	}
	for _, name := range []string{"runtime_SessionHandoff", "runtime_SessionTask", "runtime_SessionSubmission"} {
		g.property(name, "schema_version", map[string]any{"const": prifly.AssistedSessionWorkspaceVersion})
	}
	for _, name := range []string{"runtime_SessionHandoff", "runtime_SessionTask"} {
		g.property(name, "workspace_mode", enum("worktree", "checkout"))
	}
	g.property("runtime_SessionTask", "repository_workspace", map[string]any{"type": "string", "minLength": 1})
	g.property("runtime_SessionTask", "permitted_effects", map[string]any{
		"type": "array", "maxItems": 8, "uniqueItems": true,
		"items": enum("write_inside_claimed_workspace", "local_git_commit_on_claimed_workspace", "write_inside_declared_output_slot"),
	})
}
