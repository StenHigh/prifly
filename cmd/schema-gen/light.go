package main

import (
	"encoding/json"
	"reflect"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func lightSchema() ([]byte, error) {
	g := generator{defs: map[string]any{}}
	ref := func(name string) map[string]any { return map[string]any{"$ref": "#/$defs/" + name} }
	types := []struct {
		Name    string
		Type    reflect.Type
		Version string
	}{
		{"LightBoard", reflect.TypeFor[prifly.LightBoardPage](), "prifly-light-board/1"},
		{"LightTask", reflect.TypeFor[prifly.LightTaskPage](), "prifly-light-task/1"},
		{"LightReviews", reflect.TypeFor[prifly.LightReviewsPage](), "prifly-light-reviews/1"},
		{"LightReport", reflect.TypeFor[prifly.LightReportView](), "prifly-light-report/1"},
		{"LightProjection", reflect.TypeFor[prifly.LightProjection](), prifly.LightProjectionVersion},
	}
	names := []string{}
	for _, item := range types {
		g.defs[item.Name] = g.schema(item.Type)
		g.property("runtime_"+item.Type.Name(), "schema_version", map[string]any{"const": item.Version})
		names = append(names, item.Name)
	}
	g.property("flow_Ref", "id", map[string]any{"type": "string", "minLength": 1, "maxLength": 128})
	g.property("flow_Ref", "version", map[string]any{"type": "string", "minLength": 1, "maxLength": 64})
	g.property("flow_Ref", "digest", map[string]any{"type": "string", "pattern": "^sha256:[a-f0-9]{64}$"})
	g.property("runtime_LightProjection", "workflows", map[string]any{"type": "array", "maxItems": 64, "items": ref("runtime_LightWorkflowMapping")})
	g.property("runtime_LightWorkflowMapping", "phases", map[string]any{"type": "array", "maxItems": 128, "items": ref("runtime_LightPhaseMapping")})
	g.property("runtime_LightWorkflowMapping", "reviews", map[string]any{"type": "array", "maxItems": 16, "items": ref("runtime_LightReviewMapping")})
	g.property("runtime_LightPhaseMapping", "key", enum("plan", "implementation", "review", "work"))
	g.property("runtime_LightReviewMapping", "gate_scope", enum("lens", "aggregate"))
	g.property("runtime_LightReviewMapping", "disabled_state", enum("disabled", "not_applicable"))
	g.property("runtime_LightReviewMapping", "gate_ancestor", map[string]any{"type": "integer", "minimum": 0, "maximum": 8})
	for _, field := range []string{"round_pointer", "candidate_pointer", "request_pointer", "checks_pointer", "gate_status_pointer", "gate_blocking_pointer", "gate_owner_pointer", "findings_pointer"} {
		g.property("runtime_LightReviewMapping", field, map[string]any{"type": "string", "maxLength": 2048, "pattern": "^(?:/(?:[^~/]|~[01])*)*$"})
	}
	g.property("runtime_LightReview", "state", enum("passed", "defect", "unavailable", "waiting", "disabled", "not_applicable", "unknown", "warnings"))
	g.property("runtime_LightReviewChip", "state", enum("passed", "defect", "unavailable", "waiting", "disabled", "not_applicable", "unknown", "warnings"))
	g.property("runtime_LightSourceInfo", "freshness", enum("fresh", "updating", "stale", "unavailable"))
	g.property("runtime_LightSourceInfo", "coverage", enum("complete", "indexing", "unsupported"))
	bundle := map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "$id": "urn:prifly:light:1", "$ref": "#/$defs/LightBoard", "x-prifly-contracts": names, "$defs": g.defs}
	return json.MarshalIndent(bundle, "", "  ")
}
