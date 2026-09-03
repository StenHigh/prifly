package main

import (
	"reflect"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func actionAdmissionField(t reflect.Type, name string) bool {
	return t == reflect.TypeFor[prifly.Run]() && name == "ActionAdmissions"
}

func actionDeliveryField(t reflect.Type, name string) bool {
	return t == reflect.TypeFor[prifly.Run]() && name == "ActionDeliveries"
}

func forkField(t reflect.Type, name string) bool {
	return t == reflect.TypeFor[prifly.Run]() && name == "Fork"
}

func actionAdmissionConstraints(g *generator) {
	for name, version := range map[string]string{
		"runtime_Run":          prifly.CoreActionAdmissionStateVersion,
		"runtime_RunView":      prifly.CoreActionAdmissionReadVersion,
		"runtime_NextView":     prifly.CoreActionAdmissionNextVersion,
		"runtime_Preview":      prifly.CoreActionAdmissionPreviewVersion,
		"runtime_StepReadView": prifly.CoreActionAdmissionStepReadVersion,
	} {
		g.property(name, "schema_version", map[string]any{"const": version})
	}
}

func actionGrantAdmissionConstraints(g *generator) {
	for name, version := range map[string]string{
		"runtime_Run":          prifly.CoreActionGrantAdmissionStateVersion,
		"runtime_RunView":      prifly.CoreActionGrantAdmissionReadVersion,
		"runtime_NextView":     prifly.CoreActionGrantAdmissionNextVersion,
		"runtime_Preview":      prifly.CoreActionGrantAdmissionPreviewVersion,
		"runtime_StepReadView": prifly.CoreActionGrantAdmissionStepReadVersion,
	} {
		g.property(name, "schema_version", map[string]any{"const": version})
	}
	for _, name := range []string{"runtime_ActionAdmission", "runtime_AdmitActionPayload"} {
		g.property(name, "grant_refs", map[string]any{"anyOf": []any{map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "maxItems": 1}, map[string]any{"type": "null"}}})
	}
}

func actionDeliveryConstraints(g *generator) {
	for name, version := range map[string]string{
		"runtime_Run": prifly.CoreActionDeliveryStateVersion, "runtime_RunView": prifly.CoreActionDeliveryReadVersion,
		"runtime_NextView": prifly.CoreActionDeliveryNextVersion, "runtime_Preview": prifly.CoreActionDeliveryPreviewVersion,
		"runtime_StepReadView": prifly.CoreActionDeliveryStepReadVersion,
	} {
		g.property(name, "schema_version", map[string]any{"const": version})
	}
	for _, name := range []string{"runtime_ActionAdmission", "runtime_AdmitActionPayload"} {
		g.property(name, "grant_refs", map[string]any{"anyOf": []any{map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "maxItems": 1}, map[string]any{"type": "null"}}})
	}
}

func forkConstraints(g *generator) {
	for name, version := range map[string]string{
		"runtime_Run": prifly.CoreForkStateVersion, "runtime_RunView": prifly.CoreForkReadVersion,
		"runtime_NextView": prifly.CoreForkNextVersion, "runtime_Preview": prifly.CoreForkPreviewVersion,
		"runtime_StepReadView": prifly.CoreForkStepReadVersion,
	} {
		g.property(name, "schema_version", map[string]any{"const": version})
	}
}
