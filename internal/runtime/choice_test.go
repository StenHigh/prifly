package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

func choiceFieldEqual(pointer string, value any) map[string]any {
	return map[string]any{"op": "eq", "left": map[string]any{"kind": "field", "ref": map[string]any{"from": "workflow_input", "port": "control", "pointer": pointer}}, "right": map[string]any{"kind": "literal", "value": value}}
}

func choiceBranch(id string, predicate map[string]any, next string) map[string]any {
	return map[string]any{"id": id, "predicate": predicate, "next": next}
}

func choiceStage(selection string, branches ...map[string]any) map[string]any {
	return map[string]any{"kind": "choice", "selection": selection, "branches": branches}
}

func choiceFinish(outcome string) map[string]any {
	return map[string]any{"kind": "finish", "outcome": outcome, "output_bindings": map[string]any{}}
}

func choiceStages(workflow map[string]any) map[string]any {
	return workflow["definition"].(map[string]any)["stages"].(map[string]any)
}

// The control-only variant has no executor. The process variant reuses the
// real driver worker, preserving its original F1 Run and authored definition.
func choiceFixture(t *testing.T, controlJSON, workerMode string) (*Engine, map[string]any, StartOptions) {
	t.Helper()
	var e *Engine
	var options StartOptions
	if workerMode == "" {
		e, options = emptyRuntime(t)
	} else {
		e, _ = driverProject(t, workerMode, 10000)
		options = StartOptions{WorkflowFile: "workflows/driver.json", BriefFile: "brief.json", Inputs: map[string]string{"source": "source.txt"}}
	}
	data, err := os.ReadFile(filepath.Join(e.Root, options.WorkflowFile))
	var workflow map[string]any
	if err != nil || json.Unmarshal(data, &workflow) != nil {
		t.Fatal("read base workflow", err)
	}
	defs, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	e.Config.Configuration.SemanticsProfile = flow.CoreProfile
	e.Config.Configuration.SchemaVersion = CoreConfigVersion
	e.Config.ConfigurationSchemaRef = builtinRef(defs, "core:schema/core-configuration")
	controlSchema := []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"}`)
	digest, err := flow.Digest(controlSchema)
	if err != nil {
		t.Fatal(err)
	}
	ref := flow.Ref{ID: "test:schema/choice-control", Version: "1.0.0", Digest: digest}
	if err := os.WriteFile(filepath.Join(e.Root, "schemas/choice-control.json"), controlSchema, 0600); err != nil {
		t.Fatal(err)
	}
	registryBytes, err := os.ReadFile(filepath.Join(e.Root, e.Config.Configuration.RegistryFile))
	var registry RegistryFile
	if err != nil || json.Unmarshal(registryBytes, &registry) != nil {
		t.Fatal("read registry", err)
	}
	registry.Entries = append(registry.Entries, Definition{Ref: ref, Kind: "schema", Path: "schemas/choice-control.json"})
	writeRuntimeJSON(t, filepath.Join(e.Root, e.Config.Configuration.RegistryFile), registry)
	workflow["id"], workflow["title"] = "test:workflow/choice", "Deterministic choice fixture"
	workflow["inputs"].(map[string]any)["control"] = map[string]any{"format": "json", "schema_ref": ref, "required": true}
	workflow["outputs"] = map[string]any{}
	workflow["allowed_outcomes"] = []string{"succeeded", "rejected", "no_work"}
	workflow["limits"] = map[string]any{"max_step_instances": 1, "max_control_transitions": 8, "max_parallelism": 1, "max_child_depth": 0}
	stages := map[string]any{"done": choiceFinish("succeeded")}
	yes, no := "done", "done"
	if workerMode != "" {
		work := choiceStages(workflow)["work"].(map[string]any)
		stages["work"] = work
		other := make(map[string]any, len(work))
		for key, value := range work {
			other[key] = value
		}
		other["on"] = map[string]any{"pass": "done"}
		stages["other"] = other
		yes, no = "work", "other"
	}
	stages["pick"] = choiceStage("exclusive", choiceBranch("yes", choiceFieldEqual("/flag", true), yes), choiceBranch("no", choiceFieldEqual("/flag", false), no))
	workflow["definition"] = map[string]any{"entry": "pick", "stages": stages}
	options.CommandID, options.WorkflowFile = newID("command"), "workflows/choice.json"
	options.Inputs["control"] = "control.json"
	if err := os.WriteFile(filepath.Join(e.Root, "control.json"), []byte(controlJSON), 0600); err != nil {
		t.Fatal(err)
	}
	return e, workflow, options
}

func choiceStart(t *testing.T, e *Engine, workflow map[string]any, options StartOptions) string {
	t.Helper()
	writeRuntimeJSON(t, filepath.Join(e.Root, options.WorkflowFile), workflow)
	writeRuntimeJSON(t, filepath.Join(e.Root, "prifly.json"), e.Config)
	result, err := e.Start(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	return result.Receipt.RunID
}

func choiceHistory(t *testing.T, e *Engine, runID string) (local.ReadView, []local.Event) {
	t.Helper()
	view, err := e.Store.Read(context.Background(), runID, 0, 1000)
	if err != nil || view.More {
		t.Fatalf("read complete choice history: %v", err)
	}
	var decisions []local.Event
	for _, event := range view.Events {
		if event.Type == "stage.choice_decided" {
			decisions = append(decisions, event)
		}
	}
	return view, decisions
}

func TestChoiceRunsOnlySelectedWorker(t *testing.T) {
	e, workflow, options := choiceFixture(t, `{"flag":true}`, "commit-pass")
	runID := choiceStart(t, e, workflow, options)
	// The caller's mutable source changes after Start; routing uses sealed input.
	if err := os.WriteFile(filepath.Join(e.Root, "control.json"), []byte(`{"flag":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	if r.Status != "completed" || r.Outcome == nil || *r.Outcome != "succeeded" || len(r.Steps) != 1 || len(r.Attempts) != 1 {
		t.Fatalf("choice did not select exactly one worker: %+v", r)
	}
	choice, selected := activationFor(&r, "pick"), activationFor(&r, "work")
	if choice == nil || choice.Kind != "choice" || choice.Status != "completed" || choice.StepID != "" || selected == nil || activationFor(&r, "other") != nil {
		t.Fatal("choice invented a worker or selected the mutable/unselected path")
	}
	attempt := r.Attempts[r.Steps[selected.StepID].AttemptIDs[0]]
	if attempt.Accepted == nil || attempt.ProcessOutcome == nil || !attempt.ProcessOutcome.Started || !attempt.ProcessOutcome.WaitReturned || !attempt.ProcessOutcome.GroupEmpty || attempt.ProcessOutcome.ExitCode == nil || *attempt.ProcessOutcome.ExitCode != 0 {
		t.Fatal("selected worker lacks a real settled successful process")
	}
	starts, err := os.ReadFile(filepath.Join(attempt.Workspace, "worker-starts"))
	if err != nil || string(starts) != "start\n" {
		t.Fatalf("selected worker start evidence: %q %v", starts, err)
	}
	before, decisions := choiceHistory(t, e, runID)
	if len(decisions) != 1 {
		t.Fatalf("expected one durable choice decision, got %d", len(decisions))
	}
	var decision ChoiceDecision
	if err := json.Unmarshal(decisions[0].Data, &decision); err != nil {
		t.Fatal(err)
	}
	if decision.RunID != runID || decision.InvocationID != r.RootInvocationID || decision.ActivationID != choice.ID || decision.StageID != "pick" || decision.WorkflowRef != r.WorkflowRef || decision.Route != "branch" || decision.BranchID != "yes" || decision.NextStageID != "work" {
		t.Fatalf("decision is not tied to its pinned Run and chosen branch: %+v", decision)
	}
	if len(decision.Evaluations) != 2 || decision.Evaluations[0].BranchID != "yes" || decision.Evaluations[0].Result != "true" || decision.Evaluations[1].BranchID != "no" || decision.Evaluations[1].Result != "false" {
		t.Fatalf("ordered exclusive trace is incomplete: %+v", decision.Evaluations)
	}
	if len(decision.Inputs) != 1 || decision.Inputs[0].Availability != "present" || decision.Inputs[0].FieldRef.Pointer != "/flag" || decision.Inputs[0].SourceRef == nil || *decision.Inputs[0].SourceRef != r.Inputs["control"] {
		t.Fatalf("repeated pointer was not deduplicated with its exact source: %+v", decision.Inputs)
	}
	if err := validatePublic(t, "ChoiceDecision", decision); err != nil {
		t.Fatal("committed ChoiceDecision violates its public schema", err)
	}
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(e.Root, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if err := reopened.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	after, decisions := choiceHistory(t, reopened, runID)
	if len(decisions) != 1 || after.Snapshot.Version != before.Snapshot.Version || after.Snapshot.EventSeq != before.Snapshot.EventSeq || after.Cut != before.Cut || !bytes.Equal(after.Snapshot.Data, before.Snapshot.Data) {
		t.Fatal("reopen/terminal drive recomputed a committed choice")
	}
	starts, err = os.ReadFile(filepath.Join(attempt.Workspace, "worker-starts"))
	if err != nil || string(starts) != "start\n" {
		t.Fatal("reopen/terminal drive repeated the selected worker")
	}
}

func TestChoiceDurableSelection(t *testing.T) {
	exists := func(pointer string) map[string]any {
		return map[string]any{"op": "exists", "ref": map[string]any{"from": "workflow_input", "port": "control", "pointer": pointer}}
	}
	for _, tc := range []struct {
		name, input, selection, outcome string
		branches                        []map[string]any
		handlers                        map[string]string
		results                         []string
		route                           string
	}{
		{"first_match_order", `{"flag":true}`, "first_match", "succeeded", []map[string]any{choiceBranch("first", choiceFieldEqual("/flag", true), "done"), choiceBranch("second", choiceFieldEqual("/flag", true), "rejected")}, nil, []string{"true", "not_evaluated"}, "branch"},
		{"unknown_before_true", `{"flag":true}`, "first_match", "rejected", []map[string]any{choiceBranch("missing", choiceFieldEqual("/missing", true), "done"), choiceBranch("known", choiceFieldEqual("/flag", true), "done")}, map[string]string{"on_unknown": "rejected", "default": "no_work"}, []string{"unknown", "not_evaluated"}, "on_unknown"},
		{"unknown_after_true", `{"flag":true}`, "first_match", "succeeded", []map[string]any{choiceBranch("known", choiceFieldEqual("/flag", true), "done"), choiceBranch("missing", choiceFieldEqual("/missing", true), "done")}, map[string]string{"on_unknown": "rejected", "default": "no_work"}, []string{"true", "not_evaluated"}, "branch"},
		{"invalid_value_after_true", `{"flag":true,"invalid":[]}`, "first_match", "succeeded", []map[string]any{choiceBranch("known", choiceFieldEqual("/flag", true), "done"), choiceBranch("invalid", choiceFieldEqual("/invalid", true), "done")}, map[string]string{"on_error": "rejected"}, []string{"true", "not_evaluated"}, "branch"},
		{"exclusive_unknown_blocks_unique_true", `{"flag":true}`, "exclusive", "rejected", []map[string]any{choiceBranch("known", choiceFieldEqual("/flag", true), "done"), choiceBranch("missing", choiceFieldEqual("/missing", true), "done")}, map[string]string{"on_unknown": "rejected"}, []string{"true", "unknown"}, "on_unknown"},
		{"null_is_present", `{"maybe":null}`, "exclusive", "succeeded", []map[string]any{choiceBranch("present", exists("/maybe"), "done")}, map[string]string{"default": "rejected"}, []string{"true"}, "branch"},
		{"absence_is_not_null", `{}`, "exclusive", "rejected", []map[string]any{choiceBranch("present", exists("/maybe"), "done")}, map[string]string{"default": "rejected"}, []string{"false"}, "default"},
		{"null_equality", `{"maybe":null}`, "exclusive", "succeeded", []map[string]any{choiceBranch("null", choiceFieldEqual("/maybe", nil), "done")}, nil, []string{"true"}, "branch"},
		{"all_false_uses_default", `{"flag":false}`, "exclusive", "no_work", []map[string]any{choiceBranch("flagged", choiceFieldEqual("/flag", true), "done")}, map[string]string{"default": "no_work"}, []string{"false"}, "default"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, workflow, options := choiceFixture(t, tc.input, "")
			stages := choiceStages(workflow)
			stage := choiceStage(tc.selection, tc.branches...)
			stages["pick"] = stage
			for _, branch := range tc.branches {
				if branch["next"] == "rejected" {
					stages["rejected"] = choiceFinish("rejected")
				}
			}
			for field, outcome := range tc.handlers {
				next := "handler_" + field
				stage[field] = next
				stages[next] = choiceFinish(outcome)
			}
			runID := choiceStart(t, e, workflow, options)
			if err := e.Drive(context.Background(), runID); err != nil {
				t.Fatal(err)
			}
			r := driverRun(t, e, runID)
			if r.Status != "completed" || r.Outcome == nil || *r.Outcome != tc.outcome || len(r.Steps) != 0 || len(r.Attempts) != 0 {
				t.Fatalf("wrong control-only choice result: %+v", r)
			}
			a := activationFor(&r, "pick")
			if a == nil || a.StepID != "" || a.Status != "completed" || a.Settled == nil {
				t.Fatalf("choice did not settle as a control activation: %+v", a)
			}
			_, events := choiceHistory(t, e, runID)
			if len(events) != 1 {
				t.Fatalf("choice result has %d decisions", len(events))
			}
			var decision ChoiceDecision
			if json.Unmarshal(events[0].Data, &decision) != nil || decision.SchemaVersion != "choice-decision/1" {
				t.Fatalf("choice decision lacks a versioned payload: %s", events[0].Data)
			}
			if err := validatePublic(t, "ChoiceDecision", decision); err != nil {
				t.Fatal("committed selection violates its public schema", err)
			}
			if decision.Selection != tc.selection || decision.Route != tc.route || len(decision.Evaluations) != len(tc.results) {
				t.Fatalf("wrong recorded choice rule/result: %+v", decision)
			}
			for i, result := range tc.results {
				if decision.Evaluations[i].BranchID != tc.branches[i]["id"] || decision.Evaluations[i].Result != result {
					t.Fatalf("branch %d was not recorded in declared/evaluated order: %+v", i, decision.Evaluations)
				}
			}
			if tc.name == "exclusive_unknown_blocks_unique_true" {
				if len(decision.Inputs) != 2 || decision.Inputs[0].SourceRef == nil || decision.Inputs[1].SourceRef == nil || *decision.Inputs[0].SourceRef != r.Inputs["control"] || *decision.Inputs[1].SourceRef != r.Inputs["control"] || decision.Inputs[0].Availability != "present" || decision.Inputs[1].Availability != "absent" {
					t.Fatalf("different pointers lost their common source or absence: %+v", decision.Inputs)
				}
			}
			if files, err := os.ReadDir(filepath.Join(e.Root, e.Config.Configuration.WorkspaceRoot)); err != nil || len(files) != 0 {
				t.Fatalf("control selection allocated a worker workspace: %v %v", files, err)
			}
		})
	}
}

func TestChoiceErrorsAndMissingHandlers(t *testing.T) {
	for _, tc := range []struct {
		name, input, code string
		branches          []map[string]any
		hasErrorHandler   bool
		wantHandled       bool
		results           []string
	}{
		{"exclusive_ambiguity", `{"flag":true}`, "ambiguous_branch", []map[string]any{choiceBranch("one", choiceFieldEqual("/flag", true), "done"), choiceBranch("two", choiceFieldEqual("/flag", true), "done")}, false, false, []string{"true", "true"}},
		{"handled_ambiguity", `{"flag":true}`, "ambiguous_branch", []map[string]any{choiceBranch("one", choiceFieldEqual("/flag", true), "done"), choiceBranch("two", choiceFieldEqual("/flag", true), "done")}, true, true, []string{"true", "true"}},
		{"unknown_is_not_null_or_error", `{}`, "condition_unknown", []map[string]any{choiceBranch("optional", choiceFieldEqual("/maybe", nil), "done")}, true, false, []string{"unknown"}},
		{"no_default_is_not_error_route", `{"flag":false}`, "no_transition", []map[string]any{choiceBranch("flagged", choiceFieldEqual("/flag", true), "done")}, true, false, []string{"false"}},
		{"array_is_not_control_scalar", `{"flag":[]}`, "condition_type_mismatch", []map[string]any{choiceBranch("flagged", choiceFieldEqual("/flag", true), "done")}, true, true, []string{"error"}},
		{"fraction_is_not_control_scalar", `{"flag":1.5}`, "condition_type_mismatch", []map[string]any{choiceBranch("flagged", choiceFieldEqual("/flag", true), "done")}, false, false, []string{"error"}},
		{"string_is_not_boolean", `{"flag":"false"}`, "condition_type_mismatch", []map[string]any{choiceBranch("flagged", choiceFieldEqual("/flag", false), "done")}, false, false, []string{"error"}},
		{"exclusive_error_keeps_prefix", `{"flag":[],"known":false}`, "condition_type_mismatch", []map[string]any{choiceBranch("known", choiceFieldEqual("/known", true), "done"), choiceBranch("invalid", choiceFieldEqual("/flag", true), "done"), choiceBranch("unread", choiceFieldEqual("/unread", true), "done")}, true, true, []string{"false", "error", "not_evaluated"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, workflow, options := choiceFixture(t, tc.input, "")
			stages := choiceStages(workflow)
			stage := choiceStage("exclusive", tc.branches...)
			if tc.hasErrorHandler {
				stage["on_error"] = "error_handler"
				stages["error_handler"] = choiceFinish("rejected")
			}
			stages["pick"] = stage
			runID := choiceStart(t, e, workflow, options)
			if err := e.Drive(context.Background(), runID); err != nil {
				t.Fatal(err)
			}
			r := driverRun(t, e, runID)
			if tc.wantHandled {
				if r.Status != "completed" || r.Outcome == nil || *r.Outcome != "rejected" || activationFor(&r, "error_handler") == nil {
					t.Fatalf("declared error path did not run: %+v", r)
				}
			} else if r.Status != "failed" || r.Outcome != nil || len(r.Ready) != 0 || activationFor(&r, "error_handler") != nil {
				t.Fatalf("unhandled decision did not remain failed: %+v", r)
			}
			a := activationFor(&r, "pick")
			if a == nil || a.Status != "failed" || a.Settled == nil || a.StepID != "" || len(r.Attempts) != 0 || len(r.Steps) != 0 || len(r.Outputs) != 0 {
				t.Fatal("choice failure lost its activation or invented an execution")
			}
			if len(r.Diagnostics) != 1 || r.Diagnostics[0].Severity != "error" || r.Diagnostics[0].ActivationID != a.ID || r.Diagnostics[0].AttemptID != "" {
				t.Fatalf("choice failure lacks an owning diagnostic: %+v", r.Diagnostics)
			}
			if r.Diagnostics[0].Code != tc.code {
				t.Fatalf("expected %s, got %+v", tc.code, r.Diagnostics)
			}
			before, decisions := choiceHistory(t, e, runID)
			if len(decisions) != 1 {
				t.Fatalf("choice error lacks its single durable decision: %d", len(decisions))
			}
			var decision ChoiceDecision
			if err := json.Unmarshal(decisions[0].Data, &decision); err != nil {
				t.Fatal(err)
			}
			if err := validatePublic(t, "ChoiceDecision", decision); err != nil {
				t.Fatal("committed failure violates its public schema", err)
			}
			route, next := "failed", ""
			if tc.wantHandled {
				route, next = "on_error", "error_handler"
			}
			if decision.Route != route || decision.NextStageID != next || decision.Failure != tc.code || decision.BranchID != "" || len(decision.Evaluations) != len(tc.results) {
				t.Fatalf("failure trace disagrees with the committed transition: %+v", decision)
			}
			for i, result := range tc.results {
				if decision.Evaluations[i].BranchID != tc.branches[i]["id"] || decision.Evaluations[i].Result != result {
					t.Fatalf("error trace lost its evaluated prefix: %+v", decision.Evaluations)
				}
			}
			query := telemetryQuery("records", "core.diagnostics")
			query.RunIDs, query.Filters.Scope = []string{runID}, []string{"stage_activation"}
			report := telemetryReport(t, e, query)
			if len(report.Records) != 1 {
				t.Fatalf("choice diagnostic disappeared from stage telemetry: %+v", report)
			}
			record := report.Records[0]
			if record.Subject != (TelemetrySubject{Kind: "stage_activation", ID: a.ID, RunID: runID}) || record.Dimensions["code"] != tc.code || record.Dimensions["stage_id"] != "pick" || record.Dimensions["step_id"] != "" || record.Subject.AttemptID != "" || record.Integer == nil || *record.Integer != 1 || len(record.Evidence) != 1 || record.Evidence[0] != r.Diagnostics[0].ID {
				t.Fatalf("choice diagnostic acquired a worker or lost its cause: %+v", record)
			}
			if err := e.Drive(context.Background(), runID); err != nil {
				t.Fatal(err)
			}
			after, repeated := choiceHistory(t, e, runID)
			if len(repeated) != 1 || after.Snapshot.EventSeq != before.Snapshot.EventSeq || after.Cut != before.Cut || !bytes.Equal(after.Snapshot.Data, before.Snapshot.Data) {
				t.Fatal("terminal failure was evaluated again")
			}
		})
	}
}

func TestChoiceUnavailableArtifactIsNotUnknown(t *testing.T) {
	for _, fault := range []string{"missing", "corrupt"} {
		t.Run(fault, func(t *testing.T) {
			e, workflow, options := choiceFixture(t, `{"flag":true}`, "")
			stages := choiceStages(workflow)
			stage := stages["pick"].(map[string]any)
			stage["on_error"], stage["on_unknown"] = "error_handler", "unknown_handler"
			stages["error_handler"], stages["unknown_handler"] = choiceFinish("rejected"), choiceFinish("no_work")
			runID := choiceStart(t, e, workflow, options)
			r := driverRun(t, e, runID)
			ref := r.Inputs["control"]
			artifact, data, err := e.Artifact(ref)
			if err != nil || string(data) != `{"flag":true}` {
				t.Fatalf("input was not sealed before the fault: %q %v", data, err)
			}
			metadataPath := filepath.Join(e.Root, artifactMetadataPath(ref.ArtifactID))
			metadata, err := os.ReadFile(metadataPath)
			if err != nil {
				t.Fatal(err)
			}
			blob := filepath.Join(e.Root, e.Config.Configuration.ArtifactRoot, strings.TrimPrefix(artifact.Digest, "sha256:"))
			if fault == "missing" {
				err = os.Remove(blob)
			} else {
				if err := os.Chmod(blob, 0600); err != nil {
					t.Fatal(err)
				}
				err = os.WriteFile(blob, []byte(`{"flag":false}`), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := e.Drive(context.Background(), runID); err != nil {
				t.Fatal(err)
			}
			r = driverRun(t, e, runID)
			if r.Status != "completed" || r.Outcome == nil || *r.Outcome != "rejected" || activationFor(&r, "unknown_handler") != nil || len(r.Steps) != 0 || len(r.Attempts) != 0 {
				t.Fatal("unreadable accepted input became unknown or spawned a worker")
			}
			_, events := choiceHistory(t, e, runID)
			var decision ChoiceDecision
			if len(events) != 1 || json.Unmarshal(events[0].Data, &decision) != nil {
				t.Fatal("missing durable failed choice")
			}
			if decision.Route != "on_error" || decision.Failure != "condition_input_unavailable" || len(decision.Evaluations) != 2 || decision.Evaluations[0].Result != "error" || decision.Evaluations[1].Result != "not_evaluated" || len(decision.Inputs) != 1 || decision.Inputs[0].Availability != "unavailable" || decision.Inputs[0].SourceRef == nil || *decision.Inputs[0].SourceRef != ref {
				t.Fatalf("lost artifact identity/error trace: %+v", decision)
			}
			if err := validatePublic(t, "ChoiceDecision", decision); err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(metadataPath)
			if err != nil || !bytes.Equal(metadata, after) {
				t.Fatal("error routing rewrote accepted artifact metadata", err)
			}
		})
	}
}

func TestChoiceReadsAcceptedProducerOutputOrItsAbsence(t *testing.T) {
	for _, input := range []string{`{"flag":true}`, `{"flag":false}`} {
		t.Run(input, func(t *testing.T) {
			e, workflow, options := choiceFixture(t, input, "pass")
			data, err := os.ReadFile(filepath.Join(e.Root, "steps/driver.json"))
			var step flow.StepDefinition
			if err != nil || json.Unmarshal(data, &step) != nil {
				t.Fatal("read producer base definition", err)
			}
			ref := workflow["inputs"].(map[string]any)["control"].(map[string]any)["schema_ref"].(flow.Ref)
			step.ID = "test:step/choice-producer"
			step.Outputs["report"] = flow.OutputPort{Port: flow.Port{Format: "json", SchemaRef: &ref}, RequiredFor: []string{}}
			data, err = canonical(step)
			if err != nil {
				t.Fatal(err)
			}
			digest, err := flow.Digest(data)
			if err != nil {
				t.Fatal(err)
			}
			stepRef := flow.Ref{ID: step.ID, Version: step.Version, Digest: digest}
			stepPath := "steps/choice-producer.json"
			if err := os.WriteFile(filepath.Join(e.Root, stepPath), data, 0600); err != nil {
				t.Fatal(err)
			}
			registryBytes, err := os.ReadFile(filepath.Join(e.Root, e.Config.Configuration.RegistryFile))
			var registry RegistryFile
			if err != nil || json.Unmarshal(registryBytes, &registry) != nil {
				t.Fatal("read registry", err)
			}
			registry.Entries = append(registry.Entries, Definition{Ref: stepRef, Kind: "step", Path: stepPath})
			writeRuntimeJSON(t, filepath.Join(e.Root, e.Config.Configuration.RegistryFile), registry)
			executor := e.Config.Configuration.Executors["test:step/driver"]
			executor.Args = []string{"-test.run=^TestChoiceProducerHelper$"}
			executor.Environment = map[string]string{"CHOICE_PRODUCER_HELPER": "1", "GORACE": "atexit_sleep_ms=0"}
			e.Config.Configuration.Executors[step.ID] = executor
			predicate := choiceFieldEqual("/flag", true)
			predicate["left"].(map[string]any)["ref"] = map[string]any{"from": "stage_output", "stage_id": "producer", "port": "report", "pointer": "/flag"}
			pick := choiceStage("exclusive", choiceBranch("produced", predicate, "done"))
			pick["on_unknown"] = "absent"
			workflow["definition"] = map[string]any{"entry": "entry", "stages": map[string]any{
				"entry":    choiceStage("exclusive", choiceBranch("produce", choiceFieldEqual("/flag", true), "producer"), choiceBranch("skip", choiceFieldEqual("/flag", false), "pick")),
				"producer": map[string]any{"kind": "step", "step_ref": stepRef, "input_bindings": map[string]any{"source": map[string]any{"from": "workflow_input", "port": "source"}}, "on": map[string]string{"pass": "pick"}},
				"pick":     pick, "done": choiceFinish("succeeded"), "absent": choiceFinish("rejected"),
			}}
			runID := choiceStart(t, e, workflow, options)
			if err := e.Drive(context.Background(), runID); err != nil {
				t.Fatal(err)
			}
			r := driverRun(t, e, runID)
			_, events := choiceHistory(t, e, runID)
			var decision ChoiceDecision
			if len(events) != 2 || json.Unmarshal(events[1].Data, &decision) != nil || decision.StageID != "pick" || len(decision.Inputs) != 1 {
				t.Fatal("consumer choice did not persist its actual producer reference")
			}
			if err := validatePublic(t, "ChoiceDecision", decision); err != nil {
				t.Fatal(err)
			}
			field := decision.Inputs[0]
			if field.FieldRef.From != "stage_output" || field.FieldRef.StageID != "producer" || field.FieldRef.Port != "report" || field.FieldRef.Pointer != "/flag" {
				t.Fatalf("producer FieldRef was not preserved: %+v", field)
			}
			if input == `{"flag":true}` {
				producer := activationFor(&r, "producer")
				if r.Status != "completed" || r.Outcome == nil || *r.Outcome != "succeeded" || len(r.Steps) != 1 || len(r.Attempts) != 1 || producer == nil || producer.Status != "completed" {
					t.Fatal("real JSON producer did not settle")
				}
				s := r.Steps[producer.StepID]
				a := r.Attempts[s.AttemptIDs[0]]
				if a.Accepted == nil || a.ProcessOutcome == nil || !a.ProcessOutcome.Started || !a.ProcessOutcome.WaitReturned || !a.ProcessOutcome.GroupEmpty || field.Availability != "present" || field.ProducerActivationID != producer.ID || field.SourceRef == nil || *field.SourceRef != s.Outputs["report"] || *field.SourceRef != a.Accepted.Outputs["report"] {
					t.Fatalf("choice read an unaccepted or unrelated producer value: %+v", field)
				}
			} else if r.Status != "completed" || r.Outcome == nil || *r.Outcome != "rejected" || len(r.Steps) != 0 || len(r.Attempts) != 0 || activationFor(&r, "producer") != nil || decision.Route != "on_unknown" || field.Availability != "absent" || field.SourceRef != nil || field.ProducerActivationID != "" {
				t.Fatalf("skipped producer gained a fabricated artifact or execution: %+v", r)
			}
		})
	}
}

func TestChoiceProducerHelper(t *testing.T) {
	if os.Getenv("CHOICE_PRODUCER_HELPER") != "1" {
		return
	}
	var envelope struct {
		RunID     string `json:"run_id"`
		StepID    string `json:"step_instance_id"`
		AttemptID string `json:"attempt_id"`
	}
	if err := json.NewDecoder(os.Stdin).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(os.Getenv("PRIFLY_CONTEXT_FILE"))
	var manifest ContextManifest
	if err != nil || json.Unmarshal(data, &manifest) != nil {
		t.Fatal("read producer context", err)
	}
	slot := manifest.Outputs["report"]
	data = []byte(`{"flag":true,"producer":"actual"}`)
	if err := os.WriteFile(slot.Path, data, 0600); err != nil {
		t.Fatal(err)
	}
	result := Result{SchemaVersion: "1", RunID: envelope.RunID, StepInstanceID: envelope.StepID, AttemptID: envelope.AttemptID, EnvelopeDigest: os.Getenv("PRIFLY_ENVELOPE_DIGEST"), Verdict: "pass", Outputs: map[string]ArtifactRef{"report": {ArtifactID: slot.ArtifactID, Revision: slot.Revision, Digest: rawDigest(data)}}, EvidenceRefs: []any{}, EffectReceiptRefs: []any{}, Summary: "Real JSON producer"}
	channel := os.NewFile(3, "result")
	if err := json.NewEncoder(channel).Encode(result); err != nil {
		t.Fatal(err)
	}
	if err := channel.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestChoiceCommitReceiptAndCAS(t *testing.T) {
	e, workflow, options := choiceFixture(t, `{"flag":true}`, "")
	runID := choiceStart(t, e, workflow, options)
	ctx := context.Background()
	r, view, err := e.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	p, err := r.plan()
	if err != nil {
		t.Fatal(err)
	}
	a := activationFor(&r, "pick")
	decision := e.evaluateChoice(r, p, "pick")
	if decision.Observed.UTC != "" || decision.Observed.Session != "" {
		t.Fatal("a pure calculation claimed a commit observation")
	}
	commandID := newID("command")
	first, err := e.commitChoice(ctx, r, view, p, a, commandID, decision)
	if err != nil || first.Receipt.Rejection != nil {
		t.Fatalf("initial choice commit: %+v %v", first, err)
	}
	before, events := choiceHistory(t, e, runID)
	if len(events) != 1 {
		t.Fatal("initial choice did not commit exactly one decision")
	}
	duplicate, err := e.commitChoice(ctx, r, view, p, a, commandID, decision)
	if err != nil || !duplicate.Duplicate {
		t.Fatalf("exact old-read retry did not return its receipt: %+v %v", duplicate, err)
	}
	want, _ := canonical(first.Receipt)
	got, _ := canonical(duplicate.Receipt)
	if !bytes.Equal(want, got) {
		t.Fatal("exact choice retry changed the original receipt/cut")
	}
	conflict, err := e.commitChoice(ctx, r, view, p, a, newID("command"), decision)
	if err == nil || conflict.Receipt.Rejection == nil || conflict.Receipt.Rejection.Code != "version_conflict" {
		t.Fatalf("a second writer consumed a stale choice: %+v %v", conflict, err)
	}
	after, events := choiceHistory(t, e, runID)
	// New call samples/rejection receipts can advance the global cut, but not
	// the accepted choice, RunVersion, EventSequence or original receipt cut.
	if len(events) != 1 || after.Snapshot.Version != before.Snapshot.Version || after.Snapshot.EventSeq != before.Snapshot.EventSeq || !bytes.Equal(after.Snapshot.Data, before.Snapshot.Data) {
		t.Fatal("retry or stale writer changed the accepted choice")
	}
}

func TestChoiceConcurrentWritersCommitOnce(t *testing.T) {
	e, workflow, options := choiceFixture(t, `{"flag":true}`, "")
	runID := choiceStart(t, e, workflow, options)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r, view, err := e.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	p, err := r.plan()
	if err != nil {
		t.Fatal(err)
	}
	a := activationFor(&r, "pick")
	decision := e.evaluateChoice(r, p, "pick")
	type outcome struct {
		result local.ApplyResult
		err    error
	}
	start := make(chan struct{})
	done := make(chan outcome, 2)
	for i := 0; i < 2; i++ {
		commandID := newID("command")
		go func() {
			<-start
			result, err := e.commitChoice(ctx, r, view, p, a, commandID, decision)
			done <- outcome{result, err}
		}()
	}
	close(start)
	accepted, rejected := 0, 0
	for i := 0; i < 2; i++ {
		result := <-done
		if result.err == nil && result.result.Receipt.Rejection == nil {
			accepted++
		} else if result.result.Receipt.Rejection != nil && result.result.Receipt.Rejection.Code == "version_conflict" {
			rejected++
		} else {
			t.Fatalf("unexpected concurrent result: %+v %v", result.result, result.err)
		}
	}
	_, events := choiceHistory(t, e, runID)
	r = driverRun(t, e, runID)
	if accepted != 1 || rejected != 1 || len(events) != 1 || len(r.Ready) != 1 || r.Ready[0] != "done" || len(r.Steps) != 0 || len(r.Attempts) != 0 {
		t.Fatal("two writers selected more than one transition")
	}
}

func TestChoiceCommitHonorsPauseAndCancel(t *testing.T) {
	for _, kind := range []string{"pause", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			e, workflow, options := choiceFixture(t, `{"flag":true}`, "commit-pass")
			runID := choiceStart(t, e, workflow, options)
			ctx := context.Background()
			r, view, err := e.load(ctx, runID)
			if err != nil {
				t.Fatal(err)
			}
			p, err := r.plan()
			if err != nil {
				t.Fatal(err)
			}
			decision := e.evaluateChoice(r, p, "pick")
			if _, err := e.Restrict(ctx, RestrictCommand{SchemaVersion: "1", CommandID: newID("command"), Scope: "run", ScopeID: runID, Kind: kind, Reason: "stop between calculation and commit"}); err != nil {
				t.Fatal(err)
			}
			before, _ := choiceHistory(t, e, runID)
			if _, err := e.commitChoice(ctx, r, view, p, activationFor(&r, "pick"), newID("command"), decision); err == nil {
				t.Fatal("stale choice crossed a control restriction")
			}
			// A fresh CAS version must not turn the restriction into permission.
			current, currentView, err := e.load(ctx, runID)
			if err != nil {
				t.Fatal(err)
			}
			fresh := e.evaluateChoice(current, p, "pick")
			if _, err := e.commitChoice(ctx, current, currentView, p, activationFor(&current, "pick"), newID("command"), fresh); err == nil {
				t.Fatal("fresh-version choice crossed an effective restriction")
			}
			after, events := choiceHistory(t, e, runID)
			if len(events) != 0 || after.Snapshot.Version != before.Snapshot.Version || after.Snapshot.EventSeq != before.Snapshot.EventSeq || !bytes.Equal(after.Snapshot.Data, before.Snapshot.Data) {
				t.Fatal("refused choice changed control state")
			}
			if err := e.Drive(ctx, runID); err != nil {
				t.Fatal(err)
			}
			r = driverRun(t, e, runID)
			_, events = choiceHistory(t, e, runID)
			if len(events) != 0 || len(r.Steps) != 0 || len(r.Attempts) != 0 {
				t.Fatal("restricted driver chose a branch or started a worker")
			}
			if kind == "cancel" && r.Status != "cancelled" || kind == "pause" && (!r.ResumeRequired || r.Status != "waiting") {
				t.Fatalf("driver lost its restriction: %+v", r)
			}
			if kind == "cancel" {
				a := activationFor(&r, "pick")
				if a == nil || a.Status != "cancelled" || a.Settled == nil || *a.Settled != *r.Settled {
					t.Fatal("cancelled run left an unexecuted choice open")
				}
				node := timingFind(t, Timing(r, e.clock.now(), false).Root, a.ID)
				if node.Metrics["elapsed"].IsOpen || node.AttemptCount != 0 || len(node.Children) != 0 {
					t.Fatal("cancelled choice kept accumulating time or invented a worker")
				}
			}
			if files, err := os.ReadDir(filepath.Join(e.Root, e.Config.Configuration.WorkspaceRoot)); err != nil || len(files) != 0 {
				t.Fatal("choice behind a restriction materialized a workspace")
			}
		})
	}
}

// Only the child returned by Cmd.Start is ever signaled. A worker orphaned by
// a driver crash exits on its own bound; stored PIDs never authorize a signal.
func choiceCrashDriver(t *testing.T, helper, environment, root, runID string) (*exec.Cmd, func()) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(executable, "-test.run=^"+helper+"$", "--", root, runID)
	child.Env = []string{environment + "=1", "GORACE=atexit_sleep_ms=0"}
	var stderr bytes.Buffer
	child.Stderr = &stderr
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	t.Cleanup(func() {
		if !waited {
			_ = child.Process.Kill()
			_ = child.Wait()
		}
	})
	return child, func() {
		t.Helper()
		if err := child.Process.Kill(); err != nil {
			t.Fatal(err)
		}
		err := child.Wait()
		waited = true
		var exited *exec.ExitError
		if !errors.As(err, &exited) {
			t.Fatalf("owned helper did not crash: %v %s", err, stderr.String())
		}
		status, ok := exited.Sys().(syscall.WaitStatus)
		if !ok || !status.Signaled() || status.Signal() != syscall.SIGKILL {
			t.Fatalf("owned helper was not killed with SIGKILL: %v %s", err, stderr.String())
		}
	}
}

func TestChoiceCrashAfterCommitBeforeWorkerAdmission(t *testing.T) {
	e, workflow, options := choiceFixture(t, `{"flag":true}`, "commit-pass")
	runID := choiceStart(t, e, workflow, options)
	_, crash := choiceCrashDriver(t, "TestChoiceCommitHelper", "CHOICE_COMMIT_HELPER", e.Root, runID)
	before := driverWait(t, e, runID, func(r Run) bool {
		_, err := os.Stat(filepath.Join(e.Root, "choice-committed"))
		return err == nil && len(r.Ready) == 1 && r.Ready[0] == "work"
	})
	if len(before.Steps) != 0 || len(before.Attempts) != 0 || activationFor(&before, "work") != nil {
		t.Fatal("helper crossed the choice commit/worker admission boundary")
	}
	if slot, _, err := e.Store.Slot(context.Background()); err != nil || slot != "" {
		t.Fatal("a choice decision reserved the execution slot")
	}
	_, events := choiceHistory(t, e, runID)
	if len(events) != 1 {
		t.Fatal("helper did not persist a real choice before its marker")
	}
	saved := bytes.Clone(events[0].Data)
	crash()
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(e.Root, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if err := reopened.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, reopened, runID)
	_, events = choiceHistory(t, reopened, runID)
	if r.Status != "completed" || r.Outcome == nil || *r.Outcome != "succeeded" || len(r.Attempts) != 1 || len(r.Steps) != 1 || len(events) != 1 || !bytes.Equal(saved, events[0].Data) || activationFor(&r, "other") != nil {
		t.Fatal("recovery re-evaluated the choice or changed its selected worker")
	}
	if driverObservedStarts(t, reopened) != 1 {
		t.Fatal("recovery did not start exactly one selected process")
	}
	beforeRead, _ := choiceHistory(t, reopened, runID)
	if err := reopened.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	afterRead, _ := choiceHistory(t, reopened, runID)
	if afterRead.Cut != beforeRead.Cut || afterRead.Snapshot.EventSeq != beforeRead.Snapshot.EventSeq || driverObservedStarts(t, reopened) != 1 {
		t.Fatal("a repeated recovered drive started another decision or worker")
	}
}

func TestChoiceCommitHelper(t *testing.T) {
	if os.Getenv("CHOICE_COMMIT_HELPER") != "1" {
		return
	}
	if len(os.Args) < 3 {
		t.Fatal("helper needs its owned project and Run")
	}
	e, err := Open(os.Args[len(os.Args)-2], false)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	runID := os.Args[len(os.Args)-1]
	lock, err := e.driverLock(runID)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	r, view, err := e.load(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	p, err := r.plan()
	if err != nil {
		t.Fatal(err)
	}
	decision := e.evaluateChoice(r, p, "pick")
	if _, err := e.commitChoice(context.Background(), r, view, p, activationFor(&r, "pick"), newID("command"), decision); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.Root, "choice-committed"), []byte(runID), 0600); err != nil {
		t.Fatal(err)
	}
	// The parent SIGKILLs this owned helper at the durable boundary. No test
	// flag or substitute execution path is added to production Drive.
	time.Sleep(10 * time.Second)
	t.Fatal("parent did not crash the helper within its bound")
}

func TestChoiceLostDriverDoesNotAdvancePastUncertainWorker(t *testing.T) {
	e, workflow, options := choiceFixture(t, `{"flag":true}`, "crash-short")
	stages := choiceStages(workflow)
	for _, name := range []string{"work", "other"} {
		stages[name].(map[string]any)["on"] = map[string]any{"pass": "after"}
	}
	stages["after"] = choiceStage("exclusive", choiceBranch("still_flagged", choiceFieldEqual("/flag", true), "done"))
	runID := choiceStart(t, e, workflow, options)
	child, crash := choiceCrashDriver(t, "TestDriverCrashHelper", "DRIVER_CRASH_HELPER", e.Root, runID)
	r := driverWait(t, e, runID, func(r Run) bool {
		for _, a := range r.Attempts {
			if a.Started != nil && a.Process != nil {
				if _, err := os.Stat(filepath.Join(a.Workspace, "worker-ready")); err == nil {
					return true
				}
			}
		}
		return false
	})
	a := r.Attempts[r.Active[0]]
	identity := *a.Process
	if identity.OwnerPID != child.Process.Pid || local.ProbeProcess(identity).State != "present" {
		t.Fatal("fixture has no live process owned by the child driver")
	}
	socketBytes, err := os.ReadFile(filepath.Join(a.Workspace, "driver-socket"))
	if err != nil {
		t.Fatal(err)
	}
	socket := string(socketBytes)
	if filepath.Base(socket) != "api.sock" || !strings.HasPrefix(filepath.Base(filepath.Dir(socket)), "prifly-step-") || filepath.Dir(filepath.Dir(socket)) != "/tmp" {
		t.Fatal("unexpected owned helper socket")
	}
	t.Cleanup(func() { _ = os.Remove(socket); _ = os.Remove(filepath.Dir(socket)) })
	crash()
	reopened, err := Open(e.Root, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if err := reopened.Drive(context.Background(), runID); err == nil || !strings.Contains(err.Error(), "recovery_required") {
		t.Fatalf("lost driver did not preserve uncertainty: %v", err)
	}
	r = driverRun(t, reopened, runID)
	_, events := choiceHistory(t, reopened, runID)
	if r.Status != "uncertain" || !r.HasUnresolvedEffects || len(r.Attempts) != 1 || len(r.Active) != 1 || len(events) != 1 || activationFor(&r, "after") != nil {
		t.Fatal("uncertain worker authorized another choice or process")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, err := os.Stat(filepath.Join(a.Workspace, "worker-finished"))
		if err == nil && local.ProbeProcess(identity).State == "absent" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("bounded orphan did not exit normally")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := reopened.Drive(context.Background(), runID); err == nil || !strings.Contains(err.Error(), "recovery_required") {
		t.Fatal("later PID absence authorized the pending choice")
	}
	_, events = choiceHistory(t, reopened, runID)
	if len(events) != 1 || driverObservedStarts(t, reopened) != 1 || len(driverRun(t, reopened, runID).Attempts) != 1 {
		t.Fatal("recovery repeated a decision or dispatched an extra worker")
	}
}
