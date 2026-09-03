package main

import (
	"reflect"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// P2-12 wait fields must be excluded before traversal of every earlier bundle,
// so the delivered contracts keep their exact published bytes.
func waitField(t reflect.Type, name string) bool {
	return t == reflect.TypeFor[prifly.Activation]() && name == "Wait" ||
		t == reflect.TypeFor[prifly.Run]() && (name == "Waits" || name == "Inbox")
}

func waitConstraints(g *generator) {
	for name, version := range map[string]string{
		"runtime_Run": prifly.CoreWaitStateVersion, "runtime_RunView": prifly.CoreWaitReadVersion,
		"runtime_NextView": prifly.CoreWaitNextVersion, "runtime_Preview": prifly.CoreWaitPreviewVersion,
	} {
		g.property(name, "schema_version", map[string]any{"const": version})
	}
	g.property("runtime_WaitRegistration", "schema_version", map[string]any{"const": prifly.WaitRegistrationVersion})
	g.property("runtime_EventEnvelope", "schema_version", map[string]any{"const": prifly.EventEnvelopeVersion})
	// The whole lifecycle, with exactly one terminal word. A wait that was
	// reserved and never taken is cancelled, not quietly forgotten.
	g.property("runtime_WaitRegistration", "status", enum("reserved", "active", "consumed", "cancelled", "expired"))
	// What became of a delivery. A refusal is a recorded outcome, because
	// "never arrived" and "arrived and was not used" are different answers.
	g.property("runtime_InboxEvent", "disposition", enum("held", "consumed", "refused"))
	g.property("runtime_WaitProgress", "resolution", enum("event", "timeout", "cancelled"))
	generation := map[string]any{"type": "integer", "minimum": 1, "maximum": 9007199254740991}
	g.property("runtime_WaitRegistration", "wait_generation", generation)
	g.property("runtime_EventEnvelope", "wait_generation", generation)
	g.property("runtime_WaitProgress", "wait_generation", generation)
	g.property("runtime_WaitRegistration", "nonce", map[string]any{"type": "string", "minLength": 1, "maxLength": 256})
	g.property("runtime_EventEnvelope", "nonce", map[string]any{"type": "string", "minLength": 1, "maxLength": 256})
	g.property("runtime_Run", "inbox", map[string]any{"type": "array", "minItems": 0, "maxItems": prifly.MaxInboxEvents, "items": map[string]any{"$ref": "#/$defs/runtime_InboxEvent"}})
}
