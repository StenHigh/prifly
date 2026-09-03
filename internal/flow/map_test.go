package flow

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// mapFixture builds a parent whose one stage fans out over a collection into
// copies of one body workflow, on the shared fixture registry so its policy and
// schemas are the real pinned ones. Each case bends exactly one property, so a
// refusal is attributable to that property alone.
func mapFixture(t *testing.T, bend func(map[string]any)) (map[string]any, Registry) {
	t.Helper()
	body, registry := callWorkflow(t, "test:workflow/item-body")
	schema := []byte(`true`)
	digest, _ := Digest(schema)
	schemaRef := Ref{ID: "test:schema/item", Version: "1.0.0", Digest: digest}
	registry[schemaRef] = schema
	registry[Ref{ID: AggregateSchemaID, Version: "1.0.0", Digest: digest}] = schema
	port := Port{Format: "json", SchemaRef: &schemaRef}
	body.AllowedOutcomes = []string{"succeeded", "rejected"}
	body.Inputs = map[string]InputPort{"item": {Port: port, Required: true}}
	body.Definition.Stages = map[string]Stage{"done": {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]Binding{}}}
	body.Limits = Limits{MaxStepInstances: 1, MaxControlTransitions: 8, MaxParallelism: 1, MaxChildDepth: 0}
	ref := registerCallWorkflow(t, registry, body)

	parentRevision, _ := callWorkflow(t, "test:workflow/over-collection")
	parentRevision.AllowedOutcomes = []string{"succeeded"}
	parentRevision.Inputs = map[string]InputPort{"items": {Port: port, Required: true}}
	parentRevision.Limits = Limits{MaxStepInstances: 8, MaxControlTransitions: 200, MaxParallelism: 2, MaxChildDepth: 1}
	parentRevision.Definition.Entry = "over"
	parentRevision.Definition.Stages = map[string]Stage{
		"done": {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]Binding{}},
	}
	var parent map[string]any
	if err := json.Unmarshal(encoded(t, parentRevision), &parent); err != nil {
		t.Fatal(err)
	}
	parent["definition"].(map[string]any)["stages"].(map[string]any)["over"] = map[string]any{
		"kind": "map", "max_parallelism": 2, "max_items": 4,
		"items":             map[string]any{"from": "workflow_input", "port": "items"},
		"body_workflow_ref": map[string]any{"id": ref.ID, "version": ref.Version, "digest": ref.Digest},
		"item_input":        "item",
		"item_key_pointer":  "/id",
		"input_bindings":    map[string]any{},
		"join":              map[string]any{"mode": "all", "accept_outcomes": []any{"succeeded"}, "selection": "all", "remainder": "wait"},
		"on":                map[string]any{"satisfied": "done", "unsatisfied": "done", "empty": "done"},
	}
	if bend != nil {
		bend(parent)
	}
	return parent, registry
}

func mapStage(parent map[string]any) map[string]any {
	return parent["definition"].(map[string]any)["stages"].(map[string]any)["over"].(map[string]any)
}

func TestMapCompilesOneBodyForEveryItem(t *testing.T) {
	parent, registry := mapFixture(t, nil)
	plan, err := compileParallel(t, parent, registry)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Maps["over"] == nil {
		t.Fatal("map body was not pinned")
	}
	// Every item runs the same definition, so the key selects the item, not a
	// definition of its own.
	if plan.BranchPlan("over", "a") != plan.Maps["over"] || plan.BranchPlan("over", "b") != plan.Maps["over"] {
		t.Fatal("item keys resolved to different definitions")
	}
	// The summary form is pinned by the project, not by the stage: a build
	// whose definitions omit it cannot say what the stage produces, and the
	// reference is looked up rather than assumed.
	if plan.aggregateSchemaRef() == nil && plan.StageOutputs("over") != nil {
		t.Fatal("a stage described a summary it cannot name")
	}
}

// A map's declared cap is what the definition may spend. It is refused above
// the qualified fan-out before any collection is ever read, because the
// collection is not what makes the work affordable.
func TestMapRefusesWhatThisBuildCannotRun(t *testing.T) {
	for _, c := range []struct {
		name, code string
		bend       func(map[string]any)
	}{
		{"more items than the qualified profile admits", "unsupported", func(p map[string]any) {
			mapStage(p)["max_items"] = MaxMapItems + 1
		}},
		{"more simultaneity than the workflow declares", "unsupported", func(p map[string]any) {
			mapStage(p)["max_parallelism"] = 3
		}},
		{"an item port the body does not declare", "unknown_port", func(p map[string]any) {
			mapStage(p)["item_input"] = "absent"
		}},
		{"the item port bound twice", "duplicate_binding", func(p map[string]any) {
			mapStage(p)["input_bindings"] = map[string]any{"item": map[string]any{"from": "workflow_input", "port": "items"}}
		}},
		{"the whole item as its own key", "invalid_pointer", func(p map[string]any) {
			mapStage(p)["item_key_pointer"] = ""
		}},
		{"a key pointer the contract itself refuses", "schema_invalid", func(p map[string]any) {
			mapStage(p)["item_key_pointer"] = "id"
		}},
		{"a quorum no collection could reach", "invalid_join", func(p map[string]any) {
			mapStage(p)["join"] = map[string]any{"mode": "quorum", "required_successes": 5, "accept_outcomes": []any{"succeeded"}, "selection": "first_observed", "remainder": "cancel"}
		}},
		{"an outcome the body never produces", "invalid_join", func(p map[string]any) {
			mapStage(p)["join"] = map[string]any{"mode": "all", "accept_outcomes": []any{"partial"}, "selection": "all", "remainder": "wait"}
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			parent, registry := mapFixture(t, c.bend)
			_, err := compileParallel(t, parent, registry)
			if err == nil {
				t.Fatalf("%s compiled", c.name)
			}
			var problem *Problem
			if !errors.As(err, &problem) || problem.Code != c.code {
				t.Fatalf("expected %s, got %v", c.code, err)
			}
		})
	}
}

// An empty collection has no branch results to summarise. Its route must be
// declared, and the summary must not be treated as present on it.
func TestMapEmptyCollectionHasItsOwnRoute(t *testing.T) {
	parent, registry := mapFixture(t, func(p map[string]any) {
		mapStage(p)["on"] = map[string]any{"satisfied": "done", "unsatisfied": "done"}
	})
	// The published contract itself requires the route, so the refusal lands
	// before the compiler's own check ever runs.
	_, err := compileParallel(t, parent, registry)
	var problem *Problem
	if !errors.As(err, &problem) || problem.Code != "schema_invalid" {
		t.Fatalf("a map without an empty route compiled: %v", err)
	}

	// Reading the summary after the empty route is a reference to a value that
	// route cannot produce, and must fail by name rather than at run time.
	parent, registry = mapFixture(t, func(p map[string]any) {
		stages := p["definition"].(map[string]any)["stages"].(map[string]any)
		stages["nothing"] = map[string]any{"kind": "finish", "outcome": "succeeded",
			"output_bindings": map[string]any{"summary": map[string]any{"from": "stage_output", "stage_id": "over", "port": AggregateResultsPort}}}
		mapStage(p)["on"].(map[string]any)["empty"] = "nothing"
	})
	if _, err := compileParallel(t, parent, registry); err == nil {
		t.Fatal("the empty route read a summary that route never produces")
	} else if !strings.Contains(err.Error(), "summary") && !strings.Contains(err.Error(), "output") && !strings.Contains(err.Error(), "port") {
		t.Fatalf("refused for an unrelated reason: %v", err)
	}
}

// The stage produces exactly one port. Naming another must fail by name rather
// than resolve to nothing when the stage settles.
func TestMapProducesOnlyItsSummary(t *testing.T) {
	parent, registry := mapFixture(t, func(p map[string]any) {
		stages := p["definition"].(map[string]any)["stages"].(map[string]any)
		stages["done"] = map[string]any{"kind": "finish", "outcome": "succeeded",
			"output_bindings": map[string]any{"summary": map[string]any{"from": "stage_output", "stage_id": "over", "port": "items"}}}
	})
	_, err := compileParallel(t, parent, registry)
	var problem *Problem
	if !errors.As(err, &problem) || problem.Code != "unknown_port" {
		t.Fatalf("expected unknown_port, got %v", err)
	}
}
