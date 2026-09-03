package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

// registerRuntimeSchema pins one schema in the project the way a real project
// does, so a fixture's ports refer to a definition the engine can resolve.
func registerRuntimeSchema(t *testing.T, e *Engine, name string, schema []byte) flow.Ref {
	t.Helper()
	digest, err := flow.Digest(schema)
	if err != nil {
		t.Fatal(err)
	}
	path := "schemas/" + name + ".json"
	if err := os.WriteFile(filepath.Join(e.Root, path), schema, 0600); err != nil {
		t.Fatal(err)
	}
	ref := flow.Ref{ID: "test:schema/" + name, Version: "1.0.0", Digest: digest}
	registryBytes, err := os.ReadFile(filepath.Join(e.Root, e.Config.Configuration.RegistryFile))
	var registry RegistryFile
	if err != nil || json.Unmarshal(registryBytes, &registry) != nil {
		t.Fatal("read registry", err)
	}
	registry.Entries = append(registry.Entries, Definition{Ref: ref, Kind: "schema", Path: path})
	writeRuntimeJSON(t, filepath.Join(e.Root, e.Config.Configuration.RegistryFile), registry)
	return ref
}

// mapFixture builds a parent whose entry stage fans out over a collection into
// copies of one body. The collection is a real start input, so what the stage
// seals is exactly what a project would hand it.
func mapFixture(t *testing.T, collection string, bend func(map[string]any)) (*Engine, map[string]any, StartOptions) {
	t.Helper()
	e, workflow, options := choiceFixture(t, `{"flag":true}`, "")
	itemRef := registerRuntimeSchema(t, e, "map-item", []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"id":{}},"required":["id"]}`))
	listRef := registerRuntimeSchema(t, e, "map-list", []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"array"}`))

	body := callClone(t, workflow)
	body["id"], body["title"] = "test:workflow/item-body", "item body"
	body["allowed_outcomes"] = []string{"succeeded"}
	body["inputs"] = map[string]any{"item": map[string]any{"format": "json", "schema_ref": itemRef, "required": true}}
	body["outputs"] = map[string]any{}
	body["limits"] = map[string]any{"max_step_instances": 1, "max_control_transitions": 4, "max_parallelism": 1, "max_child_depth": 0}
	body["definition"] = map[string]any{"entry": "done", "stages": map[string]any{"done": choiceFinish("succeeded")}}
	bodyRef := callRegister(t, e, body, "workflows/item-body.json")

	defs, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	workflow["id"], workflow["title"] = "test:workflow/over-collection", "Map parent"
	workflow["policy_ref"] = builtinVersionRef(defs, "core:policy/local", "3.0.0")
	workflow["allowed_outcomes"] = []string{"succeeded", "rejected", "no_work"}
	workflow["inputs"] = map[string]any{"items": map[string]any{"format": "json", "schema_ref": listRef, "required": true}}
	workflow["outputs"] = map[string]any{}
	workflow["limits"] = map[string]any{"max_step_instances": 8, "max_control_transitions": 400, "max_parallelism": 2, "max_child_depth": 2}
	workflow["definition"] = map[string]any{"entry": "over", "stages": map[string]any{
		"over": map[string]any{
			"kind": "map", "max_parallelism": 2, "max_items": 4,
			"items":             map[string]any{"from": "workflow_input", "port": "items"},
			"body_workflow_ref": bodyRef,
			"item_input":        "item",
			"item_key_pointer":  "/id",
			"input_bindings":    map[string]any{},
			"join":              map[string]any{"mode": "all", "accept_outcomes": []any{"succeeded"}, "selection": "all", "remainder": "wait"},
			"on":                map[string]any{"satisfied": "accepted", "unsatisfied": "refused", "empty": "nothing"},
		},
		"accepted": choiceFinish("succeeded"),
		"refused":  choiceFinish("rejected"),
		"nothing":  choiceFinish("no_work"),
	}}
	if err := os.WriteFile(filepath.Join(e.Root, "items.json"), []byte(collection), 0600); err != nil {
		t.Fatal(err)
	}
	options.Inputs = map[string]string{"items": "items.json"}
	options.WorkflowFile = "workflows/over-collection.json"
	if bend != nil {
		bend(workflow)
	}
	return e, workflow, options
}

func mapActivation(t *testing.T, r Run) *Activation {
	t.Helper()
	for _, a := range r.Activations {
		if a.Kind == "map" {
			return a
		}
	}
	t.Fatal("the run has no map activation")
	return nil
}

// The whole collection is checked before anything runs. A collection that fails
// anywhere leaves no half-processed expansion behind: no branch was ever
// created, so there is no "already processed half" to reason about.
func TestMapChecksTheWholeCollectionBeforeAnyChild(t *testing.T) {
	for _, c := range []struct {
		name, collection, code string
	}{
		{"two items sharing one identity", `[{"id":"a"},{"id":"b"},{"id":"a"}]`, "duplicate_item_key"},
		{"more items than the stage admits", `[{"id":"a"},{"id":"b"},{"id":"c"},{"id":"d"},{"id":"e"}]`, "map_items_exceeded"},
		{"an item with no identity at all", `[{"id":"a"},{"name":"b"}]`, "map_item_key_invalid"},
		{"an identity of an unusable type", `[{"id":"a"},{"id":{"nested":true}}]`, "map_item_key_invalid"},
		{"a number that is not a safe integer", `[{"id":1.5}]`, "map_item_key_invalid"},
	} {
		t.Run(c.name, func(t *testing.T) {
			e, workflow, options := mapFixture(t, c.collection, nil)
			runID := choiceStart(t, e, workflow, options)
			// The refusal happens while driving, so the run must be driven.
			_ = e.Drive(context.Background(), runID)
			r := driverRun(t, e, runID)
			a := mapActivation(t, r)
			if a.Status != "failed" {
				t.Fatalf("%s was admitted: activation is %s", c.name, a.Status)
			}
			// Nothing was created. This is the whole point of sealing first.
			for _, inv := range r.Invocations {
				if inv.BranchID != "" {
					t.Fatalf("%s created the child %s", c.name, inv.BranchID)
				}
			}
			if len(a.Parallel.Sealed) != 0 || len(a.Parallel.BranchIDs) != 0 {
				t.Fatalf("%s left a partial seal: %+v", c.name, a.Parallel)
			}
			found := false
			for _, d := range r.Diagnostics {
				found = found || strings.Contains(d.Code, c.code) || strings.Contains(d.Message, c.code)
			}
			if !found {
				t.Fatalf("no diagnostic named %s: %+v", c.code, r.Diagnostics)
			}
		})
	}
}

// A collection that is not a list is refused where every input is: at Start,
// against the port's own schema. The stage is never activated, so "before the
// first child" is not even the question here.
func TestMapCollectionMustBeAList(t *testing.T) {
	e, workflow, options := mapFixture(t, `{"id":"a"}`, nil)
	writeRuntimeJSON(t, filepath.Join(e.Root, options.WorkflowFile), workflow)
	writeRuntimeJSON(t, filepath.Join(e.Root, "prifly.json"), e.Config)
	if _, err := e.Start(context.Background(), options); err == nil {
		t.Fatal("a map accepted a collection that is not a list")
	} else if !strings.Contains(err.Error(), "schema_invalid") {
		t.Fatalf("refused for an unrelated reason: %v", err)
	}
}

// Identity is typed. The number 1 and the string "1" are different items, so a
// collection holding both is a legitimate two-item expansion, not a duplicate.
func TestMapItemIdentityCarriesItsType(t *testing.T) {
	e, workflow, options := mapFixture(t, `[{"id":1},{"id":"1"}]`, nil)
	runID := choiceStart(t, e, workflow, options)
	r := driveParallel(t, e, runID)
	if r.Status != "completed" || r.Outcome == nil || *r.Outcome != "succeeded" {
		t.Fatalf("a typed pair did not complete: %s %+v", r.Status, r.Outcome)
	}
	a := mapActivation(t, r)
	if len(a.Parallel.Sealed) != 2 || a.Parallel.Sealed[0].Key == a.Parallel.Sealed[1].Key {
		t.Fatalf("typed keys collided: %+v", a.Parallel.Sealed)
	}
	if a.Parallel.Sealed[0].Key != "integer:1" || a.Parallel.Sealed[1].Key != "string:1" {
		t.Fatalf("keys do not carry their type: %+v", a.Parallel.Sealed)
	}
	// The array position is evidence about the collection, never an identity.
	for _, item := range a.Parallel.Sealed {
		if strings.Contains(item.Key, "position") {
			t.Fatalf("a position leaked into an identity: %s", item.Key)
		}
	}
}

// An empty collection takes its own declared route and produces no summary.
// Calling it a satisfied join would be a claim about branches that never ran.
func TestMapEmptyCollectionTakesItsDeclaredRoute(t *testing.T) {
	e, workflow, options := mapFixture(t, `[]`, nil)
	runID := choiceStart(t, e, workflow, options)
	r := driveParallel(t, e, runID)
	if r.Status != "completed" || r.Outcome == nil || *r.Outcome != "no_work" {
		t.Fatalf("an empty map did not take its empty route: %s %+v", r.Status, r.Outcome)
	}
	a := mapActivation(t, r)
	if a.Parallel.ResultsRef != nil {
		t.Fatalf("an empty map produced a summary: %+v", a.Parallel.ResultsRef)
	}
	if a.Parallel.LastDecision != nil {
		t.Fatalf("an empty map decided a join about nothing: %+v", a.Parallel.LastDecision)
	}
	for _, inv := range r.Invocations {
		if inv.BranchID != "" {
			t.Fatalf("an empty map created a child: %+v", inv)
		}
	}
}

// Each item is bound to the body's item port from the seal, with provenance
// back to the exact collection it was cut from.
func TestMapItemsCarryTheirProvenance(t *testing.T) {
	e, workflow, options := mapFixture(t, `[{"id":"a"},{"id":"b"},{"id":"c"}]`, nil)
	runID := choiceStart(t, e, workflow, options)
	r := driveParallel(t, e, runID)
	if r.Status != "completed" {
		t.Fatalf("the map did not complete: %s", r.Status)
	}
	a := mapActivation(t, r)
	if len(a.Parallel.Sealed) != 3 {
		t.Fatalf("the seal does not hold the collection: %+v", a.Parallel.Sealed)
	}
	collection := r.Invocations[r.RootInvocationID].Inputs["items"]
	for _, item := range a.Parallel.Sealed {
		artifact, _, err := e.Artifact(item.Ref)
		if err != nil {
			t.Fatal(err)
		}
		named := false
		for _, source := range artifact.Provenance {
			named = named || source == collection
		}
		if !named {
			t.Fatalf("item %s does not name the collection it came from: %+v", item.Key, artifact.Provenance)
		}
	}
	// Every item became its own branch, bound to the body's item port.
	entered, err := r.parallelBranches(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(entered) != 3 {
		t.Fatalf("the fan-out entered %d of 3 items", len(entered))
	}
	for i, inv := range entered {
		if inv.Inputs["item"] != a.Parallel.Sealed[i].Ref {
			t.Fatalf("branch %s did not receive its sealed item", inv.BranchID)
		}
	}
}

// A run whose closure holds a map selects the state that can record a seal.
func TestMapClosureSelectsTheSealedState(t *testing.T) {
	e, workflow, options := mapFixture(t, `[{"id":"a"}]`, nil)
	runID := choiceStart(t, e, workflow, options)
	r := driverRun(t, e, runID)
	if r.SchemaVersion != CoreMapStateVersion {
		t.Fatalf("the closure did not select the sealed-collection state: %s", r.SchemaVersion)
	}
	if r.WorkflowConfigurations[r.Invocations[r.RootInvocationID].WorkflowRef.Digest] == nil {
		t.Fatal("the root definition was not pinned")
	}
}

// The expansion rests on sealed bytes, not on the live source. After the
// collection file is reordered and grown, the artifact the items were cut from
// still holds what was sealed, every item still names it, and the fan-out owns
// the same three children under the same identities.
//
// This is the durable half of the requirement. The other half - that a source
// changing mid-expansion cannot widen it - is held by the code rather than by
// this test: the collection binding is resolved once, at entry, and every later
// branch takes its item from the seal through sealedItem. A test could only
// observe that with a body that waits for a host, which this fixture has not.
func TestMapExpansionRestsOnSealedBytesNotOnTheSource(t *testing.T) {
	original := `[{"id":"a"},{"id":"b"},{"id":"c"}]`
	e, workflow, options := mapFixture(t, original, nil)
	runID := choiceStart(t, e, workflow, options)
	r := driveParallel(t, e, runID)
	if r.Status != "completed" {
		t.Fatalf("the map did not complete: %s", r.Status)
	}
	sealed := append([]SealedItem{}, mapActivation(t, r).Parallel.Sealed...)
	if err := os.WriteFile(filepath.Join(e.Root, "items.json"), []byte(`[{"id":"c"},{"id":"b"},{"id":"a"},{"id":"d"}]`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	r = driverRun(t, e, runID)
	a := mapActivation(t, r)
	collection := r.Invocations[r.RootInvocationID].Inputs["items"]
	_, data, err := e.Artifact(collection)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Fatalf("the sealed collection followed the file: %s", data)
	}
	if len(a.Parallel.Sealed) != len(sealed) {
		t.Fatalf("the changed source resized the expansion: %d then %d", len(sealed), len(a.Parallel.Sealed))
	}
	for i, item := range a.Parallel.Sealed {
		if item != sealed[i] {
			t.Fatalf("item %d changed after the seal: %+v then %+v", i, sealed[i], item)
		}
	}
	keys := map[string]int{}
	for _, inv := range r.Invocations {
		if inv.BranchID != "" {
			keys[inv.BranchID]++
		}
	}
	if len(keys) != 3 {
		t.Fatalf("the fan-out owns %d children after the source changed: %v", len(keys), keys)
	}
	for _, item := range sealed {
		if keys[item.Key] != 1 {
			t.Fatalf("item %s has %d children", item.Key, keys[item.Key])
		}
	}
}

// A map spends the Run's own budget. Its items are child invocations of the
// same Run, counted in the same journal, and its simultaneity is the Run's, not
// a new allowance multiplied by the number of items.
func TestMapItemsSpendTheRunsOwnBudget(t *testing.T) {
	e, workflow, options := mapFixture(t, `[{"id":"a"},{"id":"b"},{"id":"c"}]`, nil)
	runID := choiceStart(t, e, workflow, options)
	r := driveParallel(t, e, runID)
	if r.Status != "completed" {
		t.Fatalf("the map did not complete: %s", r.Status)
	}
	root := r.Invocations[r.RootInvocationID]
	a := mapActivation(t, r)
	entered, err := r.parallelBranches(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range entered {
		if item.RunID != r.ID || item.ParentInvocationID != root.ID {
			t.Fatalf("an item belongs to another Run or parent: %+v", item)
		}
		if root.ControlTransitions <= item.ControlTransitions {
			t.Fatalf("the root budget did not cover an item: root %d, item %d", root.ControlTransitions, item.ControlTransitions)
		}
	}
	// Three items ran under a stage that declared two at a time. The declared
	// simultaneity is a cap on the fan-out, not a promise to run every item.
	if len(entered) != 3 {
		t.Fatalf("the fan-out entered %d of 3 items", len(entered))
	}
}
