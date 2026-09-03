package main

import (
	"encoding/json"
	"reflect"

	"github.com/stenhigh/prifly/internal/flow"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// Skip before reflection traverses new pointer targets. The v2 bundle remains
// byte-identical even though its Go model is shared with the v3 implementation.
func repeatField(t reflect.Type, name string) bool {
	return t == reflect.TypeFor[prifly.Activation]() && name == "Repeat" || t == reflect.TypeFor[prifly.Invocation]() && name == "Iteration"
}

func repeatConstraints(g *generator) {
	data, err := flow.ProtocolSchema("FieldRef")
	if err != nil {
		panic(err)
	}
	var source struct {
		Defs map[string]any `json:"$defs"`
	}
	if err := json.Unmarshal(data, &source); err != nil {
		panic(err)
	}
	for name, value := range source.Defs {
		g.defs[name] = value
	}
	ref := func(name string) map[string]any { return map[string]any{"$ref": "#/$defs/" + name} }
	for name, version := range map[string]string{
		"runtime_Run":            prifly.CoreRepeatStateVersion,
		"runtime_RunView":        prifly.CoreRepeatReadVersion,
		"runtime_NextView":       prifly.CoreRepeatNextVersion,
		"runtime_Preview":        prifly.CoreRepeatPreviewVersion,
		"runtime_RepeatDecision": prifly.RepeatDecisionVersion,
	} {
		g.property(name, "schema_version", map[string]any{"const": version})
	}
	g.property("runtime_Invocation", "iteration", map[string]any{"type": "integer", "minimum": 1, "maximum": 100})
	invocation := g.defs["runtime_Invocation"].(map[string]any)
	invocation["allOf"] = append(invocation["allOf"].([]any), map[string]any{
		"if":   map[string]any{"not": map[string]any{"required": []string{"parent_invocation_id"}}},
		"then": map[string]any{"not": map[string]any{"required": []string{"iteration"}}},
	})
	g.property("runtime_Activation", "kind", enum("step", "choice", "call", "repeat", "finish"))
	g.property("runtime_Activation", "repeat", ref("runtime_RepeatProgress"))
	activation := g.defs["runtime_Activation"].(map[string]any)
	activation["if"] = map[string]any{"properties": map[string]any{"kind": map[string]any{"const": "repeat"}}}
	activation["then"] = map[string]any{"required": []string{"repeat"}, "not": map[string]any{"required": []string{"step_instance_id"}}}
	activation["else"] = map[string]any{"not": map[string]any{"required": []string{"repeat"}}}

	g.property("runtime_RepeatProgress", "iteration_count", map[string]any{"type": "integer", "minimum": 0, "maximum": 100})
	g.property("runtime_RepeatProgress", "current_body_workflow_invocation_id", ref("Identifier"))
	g.property("runtime_RepeatProgress", "last_decision", ref("runtime_RepeatDecision"))
	progress := g.defs["runtime_RepeatProgress"].(map[string]any)
	progress["allOf"] = []any{
		map[string]any{
			"if": map[string]any{"properties": map[string]any{"iteration_count": map[string]any{"const": 0}}},
			"then": map[string]any{"not": map[string]any{"anyOf": []any{
				map[string]any{"required": []string{"current_body_workflow_invocation_id"}},
				map[string]any{"required": []string{"last_decision"}},
			}}},
			"else": map[string]any{"required": []string{"current_body_workflow_invocation_id"}},
		},
		map[string]any{
			"if":   map[string]any{"properties": map[string]any{"iteration_count": map[string]any{"minimum": 2}}},
			"then": map[string]any{"required": []string{"last_decision"}},
		},
	}

	for _, name := range []string{"id", "run_id", "workflow_invocation_id", "stage_activation_id", "stage_id", "body_workflow_invocation_id", "next_body_workflow_invocation_id", "next_stage_id", "failure"} {
		g.property("runtime_RepeatDecision", name, ref("Identifier"))
	}
	g.property("runtime_RepeatDecision", "failure_path", ref("Pointer"))
	g.property("runtime_RepeatDecision", "iteration", map[string]any{"type": "integer", "minimum": 1, "maximum": 100})
	g.property("runtime_RepeatDecision", "body_status", enum("completed", "failed"))
	outcomes := enum("succeeded", "rejected", "no_work", "partial")
	g.property("runtime_RepeatDecision", "body_outcome", nullable(outcomes))
	g.property("runtime_RepeatDecision", "until_result", enum("not_evaluated", "true", "false", "unknown", "error"))
	g.property("runtime_RepeatDecision", "route", enum("continue", "on_complete", "on_limit", "on_unknown", "on_error", "failed"))
	g.property("runtime_RepeatDecision", "inputs", map[string]any{"type": "array", "items": ref("runtime_ChoiceInput"), "maxItems": 512})
	g.defs["flow_FieldRef"] = ref("FieldRef")
	g.property("runtime_ChoiceInput", "source_ref", ref("ArtifactRef"))
	g.property("runtime_ChoiceInput", "producer_activation_id", ref("Identifier"))
	g.property("runtime_ChoiceInput", "availability", enum("present", "absent", "unavailable"))
	g.defs["runtime_ChoiceInput"].(map[string]any)["if"] = map[string]any{"properties": map[string]any{"availability": map[string]any{"const": "present"}}}
	g.defs["runtime_ChoiceInput"].(map[string]any)["then"] = map[string]any{"required": []string{"source_ref"}}
	decision := g.defs["runtime_RepeatDecision"].(map[string]any)
	decision["allOf"] = []any{
		map[string]any{
			"if":   map[string]any{"properties": map[string]any{"body_status": map[string]any{"const": "completed"}}},
			"then": map[string]any{"properties": map[string]any{"body_outcome": outcomes}},
			"else": map[string]any{"properties": map[string]any{"body_outcome": map[string]any{"type": "null"}, "until_result": map[string]any{"const": "not_evaluated"}}},
		},
		map[string]any{
			"if":   map[string]any{"properties": map[string]any{"until_result": map[string]any{"const": "not_evaluated"}}},
			"then": map[string]any{"properties": map[string]any{"inputs": map[string]any{"maxItems": 0}}},
		},
	}
	variants := []any{}
	for _, route := range []string{"continue", "on_complete", "on_limit", "on_unknown", "on_error", "failed"} {
		properties := map[string]any{"route": map[string]any{"const": route}}
		required, forbidden := []string{}, []any{}
		for _, name := range []string{"next_body_workflow_invocation_id", "next_stage_id", "failure", "failure_path"} {
			allowed := name == "next_body_workflow_invocation_id" && route == "continue" || name == "next_stage_id" && route != "continue" && route != "failed" || (name == "failure" || name == "failure_path") && (route == "on_error" || route == "failed")
			if allowed && name != "failure_path" {
				required = append(required, name)
			} else if !allowed {
				forbidden = append(forbidden, map[string]any{"required": []string{name}})
			}
		}
		switch route {
		case "continue", "on_limit":
			properties["until_result"] = map[string]any{"const": "false"}
		case "on_complete":
			properties["until_result"] = enum("not_evaluated", "true")
		case "on_unknown":
			properties["until_result"] = map[string]any{"const": "unknown"}
		case "on_error":
			properties["failure"] = map[string]any{"not": enum("condition_unknown", "unhandled_outcome", "no_transition")}
			properties["until_result"] = enum("not_evaluated", "false", "error")
		}
		if route != "on_error" && route != "failed" {
			properties["body_status"] = map[string]any{"const": "completed"}
		}
		variants = append(variants, map[string]any{"properties": properties, "required": required, "not": map[string]any{"anyOf": forbidden}})
	}
	decision["oneOf"] = variants
}
