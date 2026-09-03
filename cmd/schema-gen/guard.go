package main

import (
	"reflect"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// P2-12 guard fields must be excluded before traversal of every earlier bundle,
// so the delivered contracts keep their exact published bytes.
func guardField(t reflect.Type, name string) bool {
	return t == reflect.TypeFor[prifly.Run]() && name == "Guards"
}

func guardConstraints(g *generator) {
	// The preview keeps the version 9 constant on purpose: a preview describes
	// a workflow, and a guard is declared when a Run is created rather than by
	// the workflow, so no preview carries a version-10 shape.
	for name, version := range map[string]string{
		"runtime_Run": prifly.CoreGuardStateVersion, "runtime_RunView": prifly.CoreGuardReadVersion,
		"runtime_NextView": prifly.CoreGuardNextVersion,
	} {
		g.property(name, "schema_version", map[string]any{"const": version})
	}
	g.property("runtime_GuardRegistration", "schema_version", map[string]any{"const": prifly.GuardRegistrationVersion})
	// A guard is one of two rules, not one rule read in two directions.
	g.property("runtime_GuardRegistration", "kind", enum("start", "stop"))
	// Both reactions refuse new ordinary work in the scope. There is no third
	// value, and in particular no value that means "carry on regardless".
	for _, field := range []string{"action", "on_unknown"} {
		g.property("runtime_GuardRegistration", field, enum("pause_scope", "cancel_scope"))
	}
	// A failed guard is one whose predicate could not be evaluated at all. It
	// keeps refusing admissions rather than resolving to a convenient answer.
	g.property("runtime_GuardRegistration", "status", enum("observing", "satisfied", "fired", "failed"))
	// error is not a fourth logical value: it says the predicate could not be
	// evaluated, and it is separate so it can never be read as unknown.
	g.property("runtime_GuardObservation", "truth", enum("true", "false", "unknown", "error"))
	g.property("runtime_GuardRegistration", "observations", map[string]any{"type": "array", "minItems": 0, "maxItems": prifly.MaxGuardObservations, "items": map[string]any{"$ref": "#/$defs/runtime_GuardObservation"}})
	// The closed predicate AST, including the one operator only a guard may
	// use: the published choice and repeat contracts do not list `not`, and
	// this bundle is where it becomes legal. Depth and node budgets stay in the
	// compiler, which can count them; field references already reuse the
	// protocol contract the repeat bundle installed, along with the three-way
	// availability that keeps present, absent and unreadable apart.
	g.property("flow_Predicate", "op", enum("eq", "ne", "exists", "all", "any", "not"))
	g.property("flow_Operand", "kind", enum("literal", "field"))
	g.property("runtime_Run", "guards", map[string]any{"type": "object", "maxProperties": prifly.MaxGuards, "additionalProperties": map[string]any{"$ref": "#/$defs/runtime_GuardRegistration"}})
}
