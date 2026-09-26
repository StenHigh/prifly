package main

import (
	"reflect"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// continuationField names what only this bundle carries: the last accepted
// checkpoint a Run reports on its read. Bundles published before it describe
// the read without it, byte for byte.
func continuationField(t reflect.Type, name string) bool {
	return t == reflect.TypeFor[prifly.RunView]() && name == "Checkpoint"
}

// continuationRequired names a field that became omitempty only so recovery/2
// may leave it out. Every earlier bundle still requires it: recovery/1 always
// recorded the commit it chose its tree by.
func continuationRequired(t reflect.Type, name string) bool {
	return t == reflect.TypeFor[prifly.RecoveryProvenance]() && name == "SubjectCommit"
}

func continuationConstraints(g *generator) {
	g.property("runtime_Run", "schema_version", map[string]any{"const": prifly.CoreContinuationStateVersion})
	g.property("runtime_RunView", "schema_version", map[string]any{"const": prifly.CoreContinuationReadVersion})
	// recovery/1 chose its tree by a commit read out of an artifact and keeps
	// recording it; recovery/2 takes the source Run's own tree over and chooses
	// nothing, so it has no commit to record.
	g.defs["runtime_RecoveryProvenance"].(map[string]any)["oneOf"] = []any{
		map[string]any{"properties": map[string]any{"schema_version": map[string]any{"const": "recovery/1"}}, "required": []string{"subject_commit"}},
		map[string]any{"properties": map[string]any{"schema_version": map[string]any{"const": "recovery/2"}}, "not": map[string]any{"required": []string{"subject_commit"}}},
	}
	g.describe("runtime_RunView", "checkpoint", "The last checkpoint this Run accepted: the output a stage declared as the workflow's checkpoint, from the accepted result settled last across every invocation. Its content is the workflow author's -- a commit, a document version, a row id -- and this authority never reads it. Absent when the workflow declares no checkpoint, when no stage reporting one has been accepted yet, or when two were settled at the same moment and which is last cannot be told.")
}
