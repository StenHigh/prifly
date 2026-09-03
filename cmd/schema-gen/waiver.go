package main

import (
	"reflect"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// P2-06 waiver fields must be excluded before traversal of every earlier
// bundle, so the delivered contracts keep their exact published bytes.
func waiverField(t reflect.Type, name string) bool {
	return t == reflect.TypeFor[prifly.Run]() && (name == "Waivers" || name == "WaiverApplied")
}

func waiverConstraints(g *generator) {
	ref := func(name string) map[string]any { return map[string]any{"$ref": "#/$defs/" + name} }
	for name, version := range map[string]string{
		"runtime_Run": prifly.CoreWaiverStateVersion, "runtime_RunView": prifly.CoreWaiverReadVersion,
		"runtime_NextView": prifly.CoreWaiverNextVersion, "runtime_Preview": prifly.CoreWaiverPreviewVersion,
	} {
		g.property(name, "schema_version", map[string]any{"const": version})
	}
	// A recorded quality reduction must stay visible in the outcome, so this
	// version admits completed_with_waivers where earlier ones could not.
	outcomes := enum("succeeded", "completed_with_waivers", "rejected", "no_work", "partial")
	for _, name := range []string{"runtime_Run", "runtime_Invocation"} {
		g.property(name, "outcome", nullable(outcomes))
		shape := g.defs[name].(map[string]any)
		shape["then"] = map[string]any{"properties": map[string]any{"outcome": outcomes}}
	}
	g.property("runtime_Waiver", "status", enum("active", "applied", "expired"))
	g.property("runtime_Waiver", "approver_id", ref("Identifier"))
	g.property("runtime_Waiver", "step_instance_id", ref("Identifier"))
	g.property("runtime_Waiver", "expires_at", ref("Timestamp"))
	g.property("runtime_Waiver", "check_ref", ref("ImmutableRef"))
	g.property("runtime_Waiver", "policy_ref", ref("ImmutableRef"))
	g.property("runtime_Run", "waivers", map[string]any{"type": "array", "maxItems": prifly.MaxRunWaivers, "items": ref("runtime_Waiver")})
}
