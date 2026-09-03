package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

func repeatLiteral(value bool) map[string]any {
	return map[string]any{"op": "eq", "left": map[string]any{"kind": "literal", "value": value}, "right": map[string]any{"kind": "literal", "value": true}}
}

func repeatFixture(t *testing.T, mode, outcome string, iterations int64) (*Engine, map[string]any, map[string]any, StartOptions) {
	t.Helper()
	e, root, body, options := callFixture(t, mode, outcome, false)
	root["id"], root["title"] = "test:workflow/repeat-parent", "Repeat parent"
	root["limits"] = map[string]any{"max_step_instances": 256, "max_control_transitions": 1024, "max_parallelism": 1, "max_child_depth": 4}
	call := choiceStages(root)["work"].(map[string]any)
	initial := call["input_bindings"].(map[string]any)
	choiceStages(root)["work"] = map[string]any{"kind": "repeat", "body_workflow_ref": call["workflow_ref"], "initial_bindings": initial, "next_bindings": callClone(t, initial), "continue_on": []string{outcome}, "until": repeatLiteral(false), "max_iterations": iterations, "on_complete": map[string]any{outcome: "done"}, "on_limit": "done"}
	options.WorkflowFile = "workflows/repeat-parent.json"
	return e, root, body, options
}

func repeatEnter(t *testing.T, e *Engine, runID string) Run {
	t.Helper()
	r, view, p, a := callActivateReady(t, e, runID)
	if err := e.enterRepeat(context.Background(), r, view, p, a); err != nil {
		t.Fatal(err)
	}
	return driverRun(t, e, runID)
}

func repeatHistory(t *testing.T, e *Engine, runID string) (local.ReadView, []RepeatDecision) {
	t.Helper()
	read, _ := choiceHistory(t, e, runID)
	decisions := []RepeatDecision{}
	for _, event := range read.Events {
		if event.Type == "stage.repeat_decided" {
			var d RepeatDecision
			if err := json.Unmarshal(event.Data, &d); err != nil {
				t.Fatal(err)
			}
			if err := validatePublic(t, "RepeatDecision", d); err != nil {
				t.Fatal(err)
			}
			decisions = append(decisions, d)
		}
	}
	return read, decisions
}

func TestRepeatNativeExportsScopesAndHistory(t *testing.T) {
	for _, outcome := range []string{"succeeded", "partial", "rejected"} {
		t.Run(outcome, func(t *testing.T) {
			e, workflow, _, options := repeatFixture(t, "commit-pass", outcome, 3)
			runID := choiceStart(t, e, workflow, options)
			initial := driverRun(t, e, runID)
			if initial.SchemaVersion != CoreRepeatStateVersion || len(initial.Invocations) != 1 || activationFor(&initial, "work").Repeat.IterationCount != 0 || initial.ControlTransitions != 1 {
				t.Fatal("repeat activation changed initial admission accounting")
			}
			if err := e.Drive(context.Background(), runID); err != nil {
				t.Fatal(err)
			}
			r := driverRun(t, e, runID)
			if r.Status != "completed" || r.Outcome == nil || *r.Outcome != outcome || len(r.Invocations) != 4 || len(r.Steps) != 3 || len(r.Attempts) != 3 || r.ControlTransitions != 11 {
				t.Fatalf("repeat status=%s outcome=%v invocations=%d steps=%d attempts=%d controls=%d", r.Status, r.Outcome, len(r.Invocations), len(r.Steps), len(r.Attempts), r.ControlTransitions)
			}
			a := activationFor(&r, "work")
			bodies, err := r.repeatBodies(a.ID)
			if err != nil || len(bodies) != 3 || a.Repeat.LastDecision.Route != "on_limit" || r.Outputs["report"] != bodies[2].Outputs["report"] {
				t.Fatal("repeat lost exact last-body exports", err)
			}
			for i, body := range bodies {
				if *body.Iteration != int64(i+1) || body.ParentInvocationID != r.RootInvocationID || body.ControlTransitions != 2 || body.StepInstances != 1 || r.activationForInvocation(body.ID, "work") == nil {
					t.Fatal("iteration counters or local stage identities escaped their body", i)
				}
			}
			view, err := e.View(context.Background(), runID)
			if err != nil || view.SchemaVersion != CoreRepeatReadVersion || validatePublic(t, "CoreRunViewV3", view) != nil {
				t.Fatal("repeat read contract", err)
			}
			if view.Timing.CalculatorRevision != "core-timing/1" || view.Timing.Root.AttemptCount != 3 || len(timingFind(t, view.Timing.Root, a.ID).Children) != 3 {
				t.Fatal("repeat timing omitted historical bodies or double-counted attempts")
			}
			report, err := e.Telemetry(context.Background(), TelemetryQuery{SchemaVersion: TelemetryQueryVersion, Mode: "records", RunIDs: []string{runID}, Limit: 200})
			if err != nil || report.CalculatorRevision != "core-telemetry/2" || report.Population.Invocations != 4 || report.Population.Attempts != 3 {
				t.Fatal("repeat telemetry population", err)
			}
			read, decisions := repeatHistory(t, e, runID)
			if len(decisions) != 3 {
				t.Fatal("missing durable decisions", len(decisions))
			}
			entered, created := 0, 0
			for _, event := range read.Events {
				if event.Type == "stage.repeat_entered" {
					entered++
				}
				if event.Type == "invocation.created" {
					created++
				}
			}
			if entered != 1 || created != 3 {
				t.Fatal("body creation facts missing or duplicated", entered, created)
			}
			for i, d := range decisions {
				if d.BodyInvocationID != bodies[i].ID || d.Iteration != int64(i+1) || d.UntilResult != "false" || d.Route != map[bool]string{true: "continue", false: "on_limit"}[i < 2] {
					t.Fatal("post-body decision lost its exact iteration", d)
				}
				if i < 2 && (d.NextBodyInvocationID != bodies[i+1].ID || d.Observed != bodies[i+1].Created) {
					t.Fatal("continuation and next body were not admitted together")
				}
			}
			if err := e.Drive(context.Background(), runID); err != nil {
				t.Fatal(err)
			}
			again, _ := repeatHistory(t, e, runID)
			if read.Cut != again.Cut || driverObservedStarts(t, e) != 3 {
				t.Fatal("terminal repeat was run again")
			}
		})
	}
}

func TestRepeatPostBodyRoutesAndUnknownAreExplicit(t *testing.T) {
	for _, tc := range []struct {
		name, input, route, failure, outcome string
		max, bodies                          int64
		unknown, handler, missingComplete    bool
	}{
		{"complete", `{"flag":true}`, "on_complete", "", "succeeded", 3, 1, false, false, false},
		{"limit", `{"flag":false}`, "on_limit", "", "succeeded", 3, 3, false, false, false},
		{"true_at_limit", `{"flag":true}`, "on_complete", "", "succeeded", 1, 1, false, false, false},
		{"unknown_at_limit", `{}`, "on_unknown", "", "rejected", 1, 1, true, true, false},
		{"unhandled_unknown", `{}`, "failed", "condition_unknown", "", 1, 1, false, true, false},
		{"type_error", `{"flag":null}`, "on_error", "condition_type_mismatch", "rejected", 3, 1, false, true, false},
		{"unhandled_type_error", `{"flag":null}`, "failed", "condition_type_mismatch", "", 3, 1, false, false, false},
		{"unhandled_outcome", `{"flag":true}`, "failed", "unhandled_outcome", "", 3, 1, false, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, workflow, body, options := repeatFixture(t, "", "succeeded", tc.max)
			stage := choiceStages(workflow)["work"].(map[string]any)
			stage["until"] = choiceFieldEqual("/flag", true)
			if tc.handler || tc.unknown {
				workflow["allowed_outcomes"] = []string{"succeeded", "rejected"}
				choiceStages(workflow)["rejected"] = choiceFinish("rejected")
			}
			if tc.handler {
				stage["on_error"] = "rejected"
			}
			if tc.unknown {
				stage["on_unknown"] = "rejected"
			}
			if tc.missingComplete {
				body["id"], body["allowed_outcomes"] = "test:workflow/unhandled-repeat", []string{"succeeded", "rejected"}
				stage["body_workflow_ref"] = callRegister(t, e, body, "workflows/unhandled-repeat.json")
				stage["on_complete"] = map[string]any{"rejected": "done"}
			}
			if err := os.WriteFile(filepath.Join(e.Root, "control.json"), []byte(tc.input), 0600); err != nil {
				t.Fatal(err)
			}
			runID := choiceStart(t, e, workflow, options)
			if err := e.Drive(context.Background(), runID); err != nil {
				t.Fatal(err)
			}
			r := driverRun(t, e, runID)
			a := activationFor(&r, "work")
			if a.Repeat.IterationCount != tc.bodies || a.Repeat.LastDecision.Route != tc.route || a.Repeat.LastDecision.Failure != tc.failure || len(r.Attempts) != 0 || len(r.Steps) != 0 {
				t.Fatalf("repeat route: %+v status=%s", a.Repeat, r.Status)
			}
			if tc.outcome == "" {
				if r.Status != "failed" || r.Outcome != nil {
					t.Fatal("unhandled condition invented an outcome")
				}
			} else if r.Status != "completed" || r.Outcome == nil || *r.Outcome != tc.outcome {
				t.Fatal("wrong completion route", r.Status, r.Outcome)
			}
			_, decisions := repeatHistory(t, e, runID)
			if int64(len(decisions)) != tc.bodies || decisions[len(decisions)-1].BodyOutcome == nil {
				t.Fatal("repeat did not retain every completed body decision")
			}
		})
	}
}

func TestRepeatIterationBindingsAndProjectionProvenance(t *testing.T) {
	e, workflow, body, options := repeatFixture(t, "", "succeeded", 3)
	body["id"] = "test:workflow/repeat-values"
	control := body["inputs"].(map[string]any)["control"].(map[string]any)
	body["outputs"] = map[string]any{"state": map[string]any{"format": "json", "schema_ref": control["schema_ref"], "required_for": []string{"succeeded"}}}
	choiceStages(body)["done"].(map[string]any)["output_bindings"] = map[string]any{"state": map[string]any{"from": "workflow_input", "port": "control"}}
	ref := callRegister(t, e, body, "workflows/repeat-values.json")
	stage := choiceStages(workflow)["work"].(map[string]any)
	stage["body_workflow_ref"] = ref
	stage["until"] = map[string]any{"op": "eq", "left": map[string]any{"kind": "field", "ref": map[string]any{"from": "iteration_output", "port": "state", "pointer": "/flag"}}, "right": map[string]any{"kind": "literal", "value": false}}
	stage["next_bindings"].(map[string]any)["control"] = map[string]any{"from": "iteration_output", "port": "state", "pointer": "", "projected_schema_ref": control["schema_ref"]}
	workflow["outputs"] = callClone(t, body["outputs"].(map[string]any))
	choiceStages(workflow)["done"].(map[string]any)["output_bindings"] = map[string]any{"state": map[string]any{"from": "stage_output", "stage_id": "work", "port": "state"}}
	runID := choiceStart(t, e, workflow, options)
	if err := os.WriteFile(filepath.Join(e.Root, "control.json"), []byte(`{"flag":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	a := activationFor(&r, "work")
	bodies, err := r.repeatBodies(a.ID)
	if err != nil || len(bodies) != 3 || r.ControlTransitions != 8 || r.Outputs["state"] != bodies[2].Outputs["state"] {
		t.Fatal("iteration binding execution", err)
	}
	_, decisions := repeatHistory(t, e, runID)
	for i, body := range bodies {
		_, value, err := e.Artifact(body.Inputs["control"])
		if err != nil || string(value) != `{"flag":true}` {
			t.Fatal("body reread mutable input or lost its binding", err)
		}
		if len(decisions[i].Inputs) != 1 || decisions[i].Inputs[0].SourceRef == nil || *decisions[i].Inputs[0].SourceRef != body.Outputs["state"] {
			t.Fatal("until did not read exact current body output")
		}
		if i == 0 {
			continue
		}
		artifact, _, err := e.Artifact(body.Inputs["control"])
		if err != nil || !slices.Contains(artifact.Provenance, bodies[i-1].Outputs["state"]) {
			t.Fatal("iteration projection lost its source", err)
		}
		manifestFound := false
		for _, source := range artifact.Provenance {
			_, data, err := e.Artifact(source)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(data, []byte(`"schema_version":"json-projection/1"`)) {
				var m ProjectionManifest
				if json.Unmarshal(data, &m) != nil || m.Source != bodies[i-1].Outputs["state"] || m.WorkflowRef != r.WorkflowRef || m.Pointer != "" {
					t.Fatal("wrong repeat projection provenance")
				}
				manifestFound = true
			}
		}
		if !manifestFound {
			t.Fatal("iteration projection has no sealed manifest")
		}
	}
}

func TestRepeatQualifiedCapacityAndCapabilityVersions(t *testing.T) {
	manifest := Capabilities()
	core := manifest.Profiles[1]
	if !slices.Contains(core.Capabilities, "repeat") || slices.Contains(manifest.Profiles[0].Capabilities, "repeat") || slices.Contains(manifest.Unsupported, "repeat") || !slices.Contains(core.StateVersions, CoreRepeatStateVersion) || !slices.Contains(core.ReadVersions, CoreRepeatReadVersion) {
		t.Fatal("capabilities did not separate supported contracts and legacy profiles")
	}
	for _, count := range []int64{1, 100, 101} {
		e, workflow, _, options := repeatFixture(t, "", "succeeded", count)
		writeRuntimeJSON(t, filepath.Join(e.Root, options.WorkflowFile), workflow)
		preview, err := e.Preview(PreviewOptions{WorkflowFile: options.WorkflowFile})
		if count > 100 {
			if err == nil || !strings.Contains(err.Error(), "resource_limit") {
				t.Fatal("unqualified repeat capacity passed preview", err)
			}
			if _, err := e.Start(context.Background(), options); err == nil || !strings.Contains(err.Error(), "resource_limit") {
				t.Fatal("unqualified repeat capacity was admitted", err)
			}
			states, _, err := e.Store.ReadAll(context.Background(), 10)
			if err != nil || len(states) != 0 || driverObservedStarts(t, e) != 0 {
				t.Fatal("capacity refusal created execution", err)
			}
			continue
		}
		if err != nil || preview.SchemaVersion != CoreRepeatPreviewVersion {
			t.Fatal("repeat preview", err)
		}
		if err := validatePublic(t, "CorePreviewV3", preview); err != nil {
			t.Fatal(err)
		}
		runID := choiceStart(t, e, workflow, options)
		next, err := e.Next(context.Background(), runID)
		if err != nil || next.SchemaVersion != CoreRepeatNextVersion || next.Action != "stage" || next.InvocationID == "" || next.StageID != "work" {
			t.Fatal("qualified repeat next", err)
		}
		if err := validatePublic(t, "CoreNextViewV3", next); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRepeatProjectLimitIsPinnedAndOnlyNarrows(t *testing.T) {
	for _, tc := range []struct {
		name, value, refusal string
		bodies               int
	}{
		{"narrowed", "2", "", 2},
		{"zero", "0", "invalid_repeat_limit_configuration", 0},
		{"over_declared", "4", "invalid_repeat_limit_configuration", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, workflow, _, options := repeatFixture(t, "", "succeeded", 3)
			// The configuration schema is deliberately broader than the repeat's
			// contract: Start must still reject zero or a value above the declared
			// bound before it creates a Run.
			schema := []byte(`true`)
			digest, err := flow.Digest(schema)
			if err != nil {
				t.Fatal(err)
			}
			limitSchema := flow.Ref{ID: "test:schema/repeat-limit", Version: "1.0.0", Digest: digest}
			if err := os.WriteFile(filepath.Join(e.Root, "schemas/repeat-limit.json"), schema, 0600); err != nil {
				t.Fatal(err)
			}
			registryData, err := os.ReadFile(filepath.Join(e.Root, "definitions.json"))
			var registry RegistryFile
			if err != nil || json.Unmarshal(registryData, &registry) != nil {
				t.Fatal("read registry", err)
			}
			registry.Entries = append(registry.Entries, Definition{Ref: limitSchema, Kind: "schema", Path: "schemas/repeat-limit.json"})
			writeRuntimeJSON(t, filepath.Join(e.Root, "definitions.json"), registry)
			workflow["schema_version"] = "2"
			workflow["inputs"].(map[string]any)["round_limit"] = map[string]any{
				"format": "json", "schema_ref": limitSchema, "required": false,
				"configuration": map[string]any{"scope": "project", "default": 3},
			}
			stage := choiceStages(workflow)["work"].(map[string]any)
			stage["limit_configuration"] = "round_limit"
			e.Config.Configuration.InputValues = map[string]map[string]json.RawMessage{
				workflow["id"].(string): {"round_limit": json.RawMessage(tc.value)},
			}
			writeRuntimeJSON(t, filepath.Join(e.Root, options.WorkflowFile), workflow)
			writeRuntimeJSON(t, filepath.Join(e.Root, "prifly.json"), e.Config)
			result, err := e.Start(context.Background(), options)
			if tc.refusal != "" {
				if err == nil || !strings.Contains(err.Error(), tc.refusal) {
					t.Fatalf("want %s: %v", tc.refusal, err)
				}
				states, _, readErr := e.Store.ReadAll(context.Background(), 10)
				if readErr != nil || len(states) != 0 {
					t.Fatalf("refusal created a Run: %d %v", len(states), readErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := e.Drive(context.Background(), result.Receipt.RunID); err != nil {
				t.Fatal(err)
			}
			r := driverRun(t, e, result.Receipt.RunID)
			a := activationFor(&r, "work")
			if a.Repeat.IterationCount != int64(tc.bodies) || a.Repeat.LastDecision.Route != "on_limit" || r.WorkflowConfigurations[r.WorkflowRef.Digest].Inputs["round_limit"].Source != "project" {
				t.Fatalf("project limit was not pinned and applied: %+v", a.Repeat)
			}
		})
	}
}

func TestRepeatNextInputsAreIndependentAndConfigurationIsPinned(t *testing.T) {
	for _, mode := range []string{"omitted", "absent", "literal"} {
		t.Run(mode, func(t *testing.T) {
			e, workflow, body, options := repeatFixture(t, "", "succeeded", 2)
			body["id"], body["schema_version"] = "test:workflow/repeat-configured", "2"
			port := body["inputs"].(map[string]any)["control"].(map[string]any)
			port["required"] = false
			port["configuration"] = map[string]any{"scope": "run", "default": map[string]any{"flag": false}}
			stage := choiceStages(workflow)["work"].(map[string]any)
			stage["body_workflow_ref"] = callRegister(t, e, body, "workflows/repeat-configured.json")
			next := stage["next_bindings"].(map[string]any)
			switch mode {
			case "omitted":
				delete(next, "control")
			case "absent":
				missing := callClone(t, workflow["inputs"].(map[string]any)["control"].(map[string]any))
				missing["required"] = false
				workflow["inputs"].(map[string]any)["missing"] = missing
				next["control"] = map[string]any{"from": "workflow_input", "port": "missing"}
			case "literal":
				next["control"] = map[string]any{"from": "literal", "schema_ref": port["schema_ref"], "value": map[string]any{"flag": "next"}}
			}
			runID := choiceStart(t, e, workflow, options)
			e.Config.Configuration.InputValues = map[string]map[string]json.RawMessage{body["id"].(string): {"control": json.RawMessage(`{"flag":"drift"}`)}}
			if err := e.Drive(context.Background(), runID); err != nil {
				t.Fatal(err)
			}
			r := driverRun(t, e, runID)
			bodies, err := r.repeatBodies(activationFor(&r, "work").ID)
			if err != nil || len(bodies) != 2 || r.Status != "completed" {
				t.Fatal("configured repeat", err)
			}
			_, first, err := e.Artifact(bodies[0].Inputs["control"])
			if err != nil || string(first) != `{"flag":true}` {
				t.Fatal("initial binding changed", err)
			}
			ref, present := bodies[1].Inputs["control"]
			if mode == "absent" {
				if present {
					t.Fatal("explicit absence inherited prior inputs or used a default")
				}
				return
			}
			_, value, err := e.Artifact(ref)
			want := `{"flag":false}`
			if mode == "literal" {
				want = `{"flag":"next"}`
			}
			if !present || err != nil || string(value) != want {
				t.Fatalf("next inputs %s want %s: %v", value, want, err)
			}
		})
	}
}

func TestRepeatBindingFailurePublishesNoLastBodyExports(t *testing.T) {
	for _, phase := range []string{"initial", "next"} {
		t.Run(phase, func(t *testing.T) {
			e, workflow, body, options := repeatFixture(t, "", "succeeded", 2)
			body["id"] = "test:workflow/repeat-optional-state"
			control := body["inputs"].(map[string]any)["control"].(map[string]any)
			control["required"] = false
			body["outputs"] = map[string]any{"state": map[string]any{"format": "json", "schema_ref": control["schema_ref"], "required_for": []string{}}}
			choiceStages(body)["done"].(map[string]any)["output_bindings"] = map[string]any{"state": map[string]any{"from": "workflow_input", "port": "control"}}
			stage := choiceStages(workflow)["work"].(map[string]any)
			stage["body_workflow_ref"] = callRegister(t, e, body, "workflows/repeat-optional-state.json")
			stage["on_error"] = "rejected"
			if phase == "initial" {
				stage["initial_bindings"].(map[string]any)["control"] = map[string]any{"from": "workflow_input", "port": "control", "pointer": "/bad", "projected_schema_ref": control["schema_ref"]}
			} else {
				stage["next_bindings"].(map[string]any)["control"] = map[string]any{"from": "iteration_output", "port": "state", "pointer": "/bad", "projected_schema_ref": control["schema_ref"]}
			}
			workflow["allowed_outcomes"] = []string{"succeeded", "rejected"}
			workflow["outputs"] = callClone(t, body["outputs"].(map[string]any))
			choiceStages(workflow)["rejected"] = choiceFinish("rejected")
			choiceStages(workflow)["rejected"].(map[string]any)["output_bindings"] = map[string]any{"state": map[string]any{"from": "stage_output", "stage_id": "work", "port": "state"}}
			if err := os.WriteFile(filepath.Join(e.Root, "control.json"), []byte(`{"flag":true,"bad":"not an object"}`), 0600); err != nil {
				t.Fatal(err)
			}
			runID := choiceStart(t, e, workflow, options)
			if err := e.Drive(context.Background(), runID); err != nil {
				t.Fatal(err)
			}
			r := driverRun(t, e, runID)
			a := activationFor(&r, "work")
			if r.Status != "completed" || r.Outcome == nil || *r.Outcome != "rejected" || a.Status != "failed" || len(r.Outputs) != 0 || len(r.Attempts) != 0 {
				t.Fatal("failed repeat exported a successful result")
			}
			if phase == "initial" {
				if len(r.Invocations) != 1 || a.Repeat.IterationCount != 0 || a.Repeat.LastDecision != nil {
					t.Fatal("failed initial binding created a body")
				}
				return
			}
			last := r.currentBody(a)
			if len(r.Invocations) != 2 || last == nil || last.Status != "completed" || last.Outputs["state"].ArtifactID == "" || a.Repeat.LastDecision.Route != "on_error" || a.Repeat.LastDecision.UntilResult != "false" {
				t.Fatal("next binding failure lost its accepted body or created another")
			}
			repeatHistory(t, e, runID)
		})
	}
}

func TestRepeatNonContinuingOutcomeSkipsUntil(t *testing.T) {
	e, workflow, body, options := repeatFixture(t, "", "succeeded", 3)
	body["id"], body["allowed_outcomes"] = "test:workflow/repeat-outcomes", []string{"succeeded", "rejected"}
	stage := choiceStages(workflow)["work"].(map[string]any)
	stage["body_workflow_ref"] = callRegister(t, e, body, "workflows/repeat-outcomes.json")
	stage["continue_on"] = []string{"rejected"}
	stage["until"] = choiceFieldEqual("/missing", true)
	runID := choiceStart(t, e, workflow, options)
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	d := activationFor(&r, "work").Repeat.LastDecision
	if r.Status != "completed" || d.Iteration != 1 || d.Route != "on_complete" || d.UntilResult != "not_evaluated" || len(d.Inputs) != 0 || len(r.Invocations) != 2 {
		t.Fatal("noncontinuing outcome read until or ran another body")
	}
}
