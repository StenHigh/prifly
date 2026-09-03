package main

import (
	"encoding/json"
	"reflect"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// Subscription fields are removed before traversal for every earlier bundle.
func publicationSubscriptionField(t reflect.Type, name string) bool {
	return t == reflect.TypeFor[prifly.Run]() && (name == "PublicationSubscriptions" || name == "PublicationAssignments") ||
		t == reflect.TypeFor[prifly.WaitProgress]() && name == "PublicationAssignmentID"
}

func publicationSubscriptionConstraints(g *generator) {
	ref := func(name string) map[string]any { return map[string]any{"$ref": "#/$defs/" + name} }
	for name, version := range map[string]string{
		"runtime_Run":          prifly.CorePublicationSubscriptionStateVersion,
		"runtime_RunView":      prifly.CorePublicationSubscriptionReadVersion,
		"runtime_NextView":     prifly.CorePublicationSubscriptionNextVersion,
		"runtime_Preview":      prifly.CorePublicationSubscriptionPreviewVersion,
		"runtime_StepReadView": prifly.CorePublicationSubscriptionStepReadVersion,
	} {
		g.property(name, "schema_version", map[string]any{"const": version})
	}

	for name, version := range map[string]string{
		"runtime_PublicationSubscription": prifly.PublicationSubscriptionVersion,
		"runtime_PublicationAssignment":   prifly.PublicationAssignmentVersion,
	} {
		g.property(name, "schema_version", map[string]any{"const": version})
	}
	for _, field := range []string{"id", "run_id", "workflow_invocation_id", "repeat_activation_id"} {
		g.property("runtime_PublicationSubscription", field, ref("Identifier"))
	}
	g.property("runtime_PublicationSubscription", "source_ref", ref("ImmutableRef"))
	g.property("runtime_PublicationSubscription", "generation", map[string]any{"type": "integer", "minimum": 1, "maximum": 9007199254740991})
	g.property("runtime_PublicationSubscription", "cursor", map[string]any{"type": "integer", "minimum": 0, "maximum": 9007199254740991})
	g.property("runtime_PublicationSubscription", "pending_assignment_id", ref("Identifier"))
	g.property("runtime_PublicationSubscription", "status", enum("open", "closed", "interrupted"))

	for _, field := range []string{"id", "subscription_id", "publication_id", "closure_id", "wait_activation_id", "body_workflow_invocation_id"} {
		g.property("runtime_PublicationAssignment", field, ref("Identifier"))
	}
	g.property("runtime_PublicationAssignment", "generation", map[string]any{"type": "integer", "minimum": 1, "maximum": 9007199254740991})
	for _, field := range []string{"cursor", "next_cursor"} {
		g.property("runtime_PublicationAssignment", field, map[string]any{"type": "integer", "minimum": 0, "maximum": 9007199254740991})
	}
	g.property("runtime_PublicationAssignment", "kind", enum("Item", "Closed", "Interrupted"))
	g.property("runtime_PublicationAssignment", "item_ref", ref("ArtifactRef"))
	g.property("runtime_PublicationAssignment", "delivery_ref", ref("ArtifactRef"))
	g.property("runtime_PublicationAssignment", "status", enum("assigned", "processed"))
	assignment := g.defs["runtime_PublicationAssignment"].(map[string]any)
	assignment["allOf"] = []any{
		map[string]any{
			"if":   map[string]any{"properties": map[string]any{"status": map[string]any{"const": "processed"}}},
			"then": map[string]any{"required": []string{"processed"}},
			"else": map[string]any{"not": map[string]any{"required": []string{"processed"}}},
		},
	}
	assignment["oneOf"] = []any{
		map[string]any{"properties": map[string]any{"kind": map[string]any{"const": "Item"}}, "required": []string{"publication_id", "item_ref"}, "not": map[string]any{"required": []string{"closure_id"}}},
		map[string]any{"properties": map[string]any{"kind": map[string]any{"const": "Closed"}}, "required": []string{"closure_id"}, "not": map[string]any{"anyOf": []any{map[string]any{"required": []string{"publication_id"}}, map[string]any{"required": []string{"item_ref"}}}}},
		map[string]any{"properties": map[string]any{"kind": map[string]any{"const": "Interrupted"}}, "not": map[string]any{"anyOf": []any{map[string]any{"required": []string{"publication_id"}}, map[string]any{"required": []string{"closure_id"}}, map[string]any{"required": []string{"item_ref"}}}}},
	}

	subscriptions := map[string]any{"type": "object", "maxProperties": prifly.MaxPublicationSubscriptions, "propertyNames": ref("Identifier"), "additionalProperties": ref("runtime_PublicationSubscription")}
	g.property("runtime_Run", "publication_subscriptions", subscriptions)
	g.property("runtime_Run", "publication_assignments", map[string]any{"type": "array", "maxItems": prifly.MaxPublicationAssignments, "items": ref("runtime_PublicationAssignment")})
	g.property("runtime_WaitProgress", "resolution", enum("event", "timeout", "cancelled", "interrupted"))
	g.property("runtime_WaitProgress", "publication_assignment_id", ref("Identifier"))

	// These transport schemas are already the authority's built-in validation
	// contracts. Reuse their exact shapes instead of maintaining looser copies.
	builtins, _, err := prifly.Builtins()
	if err != nil {
		panic(err)
	}
	for name, id := range map[string]string{
		"runtime_PublicationSubscriptionHandle": "core:schema/publication-subscription-handle",
		"runtime_PublicationCursor":             "core:schema/publication-cursor",
		"runtime_PublicationDelivery":           "core:schema/publication-delivery",
	} {
		var schema map[string]any
		for _, definition := range builtins {
			if definition.Ref.ID == id {
				if err := json.Unmarshal(definition.Bytes, &schema); err != nil {
					panic(err)
				}
				break
			}
		}
		if schema == nil {
			panic("builtin schema is missing: " + id)
		}
		g.defs[name] = schema
	}
}
