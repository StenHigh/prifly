package main

import prifly "github.com/stenhigh/prifly/internal/runtime"

func publicationFailureConstraints(g *generator) {
	for name, version := range map[string]string{
		"runtime_Run":          prifly.CorePublicationFailureStateVersion,
		"runtime_RunView":      prifly.CorePublicationFailureReadVersion,
		"runtime_NextView":     prifly.CorePublicationFailureNextVersion,
		"runtime_Preview":      prifly.CorePublicationFailurePreviewVersion,
		"runtime_StepReadView": prifly.CorePublicationFailureStepReadVersion,
	} {
		g.property(name, "schema_version", map[string]any{"const": version})
	}
	g.property("runtime_WaitRegistration", "status", enum("reserved", "active", "consumed", "cancelled", "expired", "interrupted"))
	g.property("runtime_WaitProgress", "resolution", enum("event", "timeout", "cancelled", "interrupted", "producer_failed"))
}
