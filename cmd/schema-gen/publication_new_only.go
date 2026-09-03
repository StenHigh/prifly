package main

import (
	"reflect"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func publicationNewOnlyField(t reflect.Type, name string) bool {
	return t == reflect.TypeFor[prifly.WaitRegistration]() && name == "PublicationStartSequence" ||
		t == reflect.TypeFor[prifly.PublicationSubscription]() && name == "PublicationStartSequence"
}

func publicationNewOnlyConstraints(g *generator) {
	for name, version := range map[string]string{
		"runtime_Run":          prifly.CorePublicationNewOnlyStateVersion,
		"runtime_RunView":      prifly.CorePublicationNewOnlyReadVersion,
		"runtime_NextView":     prifly.CorePublicationNewOnlyNextVersion,
		"runtime_Preview":      prifly.CorePublicationNewOnlyPreviewVersion,
		"runtime_StepReadView": prifly.CorePublicationNewOnlyStepReadVersion,
	} {
		g.property(name, "schema_version", map[string]any{"const": version})
	}
	sequence := map[string]any{"type": "integer", "minimum": 0, "maximum": int64(9007199254740991)}
	g.property("runtime_WaitRegistration", "publication_start_sequence", sequence)
	g.property("runtime_PublicationSubscription", "publication_start_sequence", sequence)
	g.property("runtime_PublicationSubscription", "schema_version", enum(prifly.PublicationSubscriptionVersion, prifly.PublicationNewOnlySubscriptionVersion))
	g.defs["runtime_PublicationSubscription"].(map[string]any)["allOf"] = []any{
		map[string]any{
			"if":   map[string]any{"properties": map[string]any{"schema_version": map[string]any{"const": prifly.PublicationNewOnlySubscriptionVersion}}},
			"then": map[string]any{"required": []string{"publication_start_sequence"}},
			"else": map[string]any{"not": map[string]any{"required": []string{"publication_start_sequence"}}},
		},
	}
}
