package main

import (
	"reflect"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// profileTranslationField names what only this bundle carries: what the
// project said a declared profile name means, sealed on the Run and delivered
// in the task. Bundles published before it describe both without it.
func profileTranslationField(t reflect.Type, name string) bool {
	if t == reflect.TypeFor[prifly.Run]() && name == "ModelProfileTranslations" {
		return true
	}
	return t == reflect.TypeFor[prifly.SessionTask]() && name == "ModelProfileTranslation"
}

func profileTranslationConstraints(g *generator) {
	for name, version := range map[string]string{
		"runtime_Run": prifly.CoreProfileTranslationStateVersion, "runtime_RunView": prifly.CoreProfileTranslationReadVersion,
		"runtime_NextView": prifly.CoreProfileTranslationNextVersion, "runtime_Preview": prifly.CoreProfileTranslationPreviewVersion,
		"runtime_StepReadView": prifly.CoreProfileTranslationStepReadVersion,
	} {
		g.property(name, "schema_version", map[string]any{"const": version})
	}
	g.describe("runtime_Run", "model_profile_translations", "What the project said each declared model profile name means for the host this Run was started with, sealed at its start so a machine-local edit made afterwards is visibly not part of it. The values are opaque to this authority: it carries them to the host and reads no meaning from them.")
	g.describe("runtime_SessionTask", "model_profile_translation", "What the project said this step's declared profile means for this host, and which source said so. Absent means nobody has said yet, which is not a refusal: the host answers honestly with what it has.")
}
