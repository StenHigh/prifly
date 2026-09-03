package flow

import (
	"encoding/json"
	"errors"
	"testing"
)

// waitFixture builds a workflow whose entry stage waits for one event. Each
// case bends exactly one property, so a refusal is attributable to it alone.
func waitFixture(t *testing.T, bend func(map[string]any)) (map[string]any, Registry) {
	t.Helper()
	base, registry := callWorkflow(t, "test:workflow/waiting")
	schema := []byte(`true`)
	digest, _ := Digest(schema)
	eventRef := Ref{ID: "test:schema/approval", Version: "1.0.0", Digest: digest}
	registry[eventRef] = schema
	keyRef := Ref{ID: "test:schema/key", Version: "1.0.0", Digest: digest}
	registry[keyRef] = schema
	sourceRef := Ref{ID: "test:source/inbox", Version: "1.0.0", Digest: digest}
	registry[sourceRef] = schema

	base.AllowedOutcomes = []string{"succeeded", "rejected"}
	base.Inputs = map[string]InputPort{"ticket": {Port: Port{Format: "json", SchemaRef: &keyRef}, Required: true}}
	base.Outputs = map[string]OutputPort{}
	base.Limits = Limits{MaxStepInstances: 2, MaxControlTransitions: 32, MaxParallelism: 1, MaxChildDepth: 0}
	base.Definition.Entry = "hold"
	base.Definition.Stages = map[string]Stage{
		"accepted": {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]Binding{}},
		"expired":  {Kind: "finish", Outcome: "rejected", OutputBindings: map[string]Binding{}},
	}
	var workflow map[string]any
	if err := json.Unmarshal(encoded(t, base), &workflow); err != nil {
		t.Fatal(err)
	}
	workflow["definition"].(map[string]any)["stages"].(map[string]any)["hold"] = map[string]any{
		"kind":              "wait",
		"source_ref":        map[string]any{"id": sourceRef.ID, "version": sourceRef.Version, "digest": sourceRef.Digest},
		"event_type":        "approval.granted",
		"event_schema_ref":  map[string]any{"id": eventRef.ID, "version": eventRef.Version, "digest": eventRef.Digest},
		"correlation_input": map[string]any{"from": "workflow_input", "port": "ticket"},
		"timeout_seconds":   int64(3600),
		"on_event":          "accepted",
		"on_timeout":        "expired",
	}
	if bend != nil {
		bend(workflow)
	}
	return workflow, registry
}

func waitStage(workflow map[string]any) map[string]any {
	return workflow["definition"].(map[string]any)["stages"].(map[string]any)["hold"].(map[string]any)
}

func TestWaitCompilesWithItsSourceAndEventPinned(t *testing.T) {
	workflow, registry := waitFixture(t, nil)
	plan, err := compileParallel(t, workflow, registry)
	if err != nil {
		t.Fatal(err)
	}
	// The stage produces the accepted event and nothing else.
	outputs := plan.StageOutputs("hold")
	if outputs[WaitEventPort].SchemaRef == nil || outputs[WaitEventPort].SchemaRef.ID != "test:schema/approval" {
		t.Fatalf("a wait stage does not export its event: %+v", outputs)
	}
	if len(outputs) != 1 {
		t.Fatalf("a wait stage exports more than the event: %+v", outputs)
	}
}

// An indefinite wait is a deliberate declaration, not an omission. It cannot
// expire, so it has no expiry route, and a build refuses one that claims both.
func TestWaitDeclaresEitherADeadlineOrNone(t *testing.T) {
	indefinite, registry := waitFixture(t, func(w map[string]any) {
		stage := waitStage(w)
		stage["timeout_seconds"] = nil
		delete(stage, "on_timeout")
		// The expiry route is now unreachable, so it must go too.
		delete(w["definition"].(map[string]any)["stages"].(map[string]any), "expired")
		w["allowed_outcomes"] = []string{"succeeded"}
	})
	if _, err := compileParallel(t, indefinite, registry); err != nil {
		t.Fatalf("an explicitly indefinite wait was refused: %v", err)
	}

	for _, c := range []struct {
		name, code string
		bend       func(map[string]any)
	}{
		{"an indefinite wait with an expiry route", "schema_invalid", func(w map[string]any) {
			waitStage(w)["timeout_seconds"] = nil
		}},
		{"a deadline with nowhere to go", "schema_invalid", func(w map[string]any) {
			delete(waitStage(w), "on_timeout")
		}},
		{"a deadline longer than this build will hold", "unsupported", func(w map[string]any) {
			waitStage(w)["timeout_seconds"] = MaxWaitSeconds + 1
		}},
		{"an event schema that is not pinned", "missing_ref", func(w map[string]any) {
			waitStage(w)["event_schema_ref"] = map[string]any{"id": "test:schema/absent", "version": "1.0.0", "digest": "sha256:" + "0000000000000000000000000000000000000000000000000000000000000000"}
		}},
		{"a source that is not pinned", "missing_ref", func(w map[string]any) {
			waitStage(w)["source_ref"] = map[string]any{"id": "test:source/absent", "version": "1.0.0", "digest": "sha256:" + "0000000000000000000000000000000000000000000000000000000000000000"}
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			workflow, registry := waitFixture(t, c.bend)
			_, err := compileParallel(t, workflow, registry)
			var problem *Problem
			if !errors.As(err, &problem) || problem.Code != c.code {
				t.Fatalf("expected %s, got %v", c.code, err)
			}
		})
	}
}

// Expiry produces no event. Reading one after the expiry route is a reference
// to a value that route cannot produce, and must fail by name.
func TestWaitEventIsAbsentOnTheExpiryRoute(t *testing.T) {
	workflow, registry := waitFixture(t, func(w map[string]any) {
		stages := w["definition"].(map[string]any)["stages"].(map[string]any)
		stage := waitStage(w)
		w["outputs"] = map[string]any{"seen": map[string]any{"format": "json", "schema_ref": stage["event_schema_ref"], "required_for": []any{"succeeded", "rejected"}}}
		stages["expired"] = map[string]any{"kind": "finish", "outcome": "rejected",
			"output_bindings": map[string]any{"seen": map[string]any{"from": "stage_output", "stage_id": "hold", "port": WaitEventPort}}}
		stages["accepted"] = map[string]any{"kind": "finish", "outcome": "succeeded",
			"output_bindings": map[string]any{"seen": map[string]any{"from": "stage_output", "stage_id": "hold", "port": WaitEventPort}}}
	})
	if _, err := compileParallel(t, workflow, registry); err == nil {
		t.Fatal("the expiry route read an event that never arrived")
	}

	// On the accepted route the same reference is fine.
	workflow, registry = waitFixture(t, func(w map[string]any) {
		stage := waitStage(w)
		w["outputs"] = map[string]any{"seen": map[string]any{"format": "json", "schema_ref": stage["event_schema_ref"], "required_for": []any{"succeeded"}}}
		stages := w["definition"].(map[string]any)["stages"].(map[string]any)
		stages["accepted"] = map[string]any{"kind": "finish", "outcome": "succeeded",
			"output_bindings": map[string]any{"seen": map[string]any{"from": "stage_output", "stage_id": "hold", "port": WaitEventPort}}}
		stages["expired"] = map[string]any{"kind": "finish", "outcome": "rejected", "output_bindings": map[string]any{}}
	})
	if _, err := compileParallel(t, workflow, registry); err != nil {
		t.Fatalf("the accepted route could not read its own event: %v", err)
	}
}
