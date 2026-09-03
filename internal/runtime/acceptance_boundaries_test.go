package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

func TestAcceptancePendingProducerPauseAndCancel(t *testing.T) {
	for _, continuation := range []string{"resume", "cancel"} {
		t.Run(continuation, func(t *testing.T) {
			e, options := acceptanceProject(t, []string{"step_output", "step_result"}, "", "pass", true)
			ctx := context.Background()
			started, err := e.Start(ctx, options)
			if err != nil {
				t.Fatal(err)
			}
			runID := started.Receipt.RunID
			producer := driverExecuteFirst(t, e, runID)
			if _, err := e.Restrict(ctx, RestrictCommand{SchemaVersion: "1", CommandID: newID("command"), Scope: "run", ScopeID: runID, Kind: "pause", Reason: "pause before required checks"}); err != nil {
				t.Fatal(err)
			}
			paused, before, err := e.load(ctx, runID)
			if err != nil {
				t.Fatal(err)
			}
			if err := e.Drive(ctx, runID); err != nil {
				t.Fatal(err)
			}
			r, after, err := e.load(ctx, runID)
			if err != nil || !bytes.Equal(before.Snapshot.Data, after.Snapshot.Data) || after.Snapshot.EventSeq != before.Snapshot.EventSeq {
				t.Fatal("paused acceptance advanced", err)
			}
			if r.PendingAcceptance == nil || r.PendingAcceptance.Status != "pending" || r.Steps[producer.StepID].Status != "verifying" || r.Attempts[producer.ID].Accepted != nil || len(r.CheckExecutions) != 0 || r.ActiveCheckID != "" {
				t.Fatal("pause accepted the producer or admitted a checker")
			}
			ref := r.PendingAcceptance.Bindings["report"]
			if _, _, err := e.Artifact(ref); !os.IsNotExist(err) {
				t.Fatal("paused unchecked output became accepted", err)
			}
			if continuation == "resume" {
				stop := paused.Stops[0]
				if _, err := e.Release(ctx, ReleaseRequest{CommandID: newID("command"), RunID: runID, ExpectedControlEpoch: paused.ControlEpoch, Stops: []StopGeneration{{ID: stop.ID, Generation: stop.Generation}}, Reason: "release exact pause"}); err != nil {
					t.Fatal(err)
				}
				next, err := e.Next(ctx, runID)
				if err != nil || next.Action != "resume_required" {
					t.Fatal("release implicitly resumed pending checks", err)
				}
				if err := e.Drive(ctx, runID); err != nil {
					t.Fatal(err)
				}
				r = driverRun(t, e, runID)
				if r.PendingAcceptance == nil || r.PendingAcceptance.ID != paused.PendingAcceptance.ID || r.Attempts[producer.ID].Accepted != nil || len(r.CheckExecutions) != 0 {
					t.Fatal("release without resume consumed the pending boundary")
				}
				if _, err := e.Resume(ctx, runID, newID("command"), "resume required checks", next.RunVersion); err != nil {
					t.Fatal(err)
				}
			} else if _, err := e.Restrict(ctx, RestrictCommand{SchemaVersion: "1", CommandID: newID("command"), Scope: "run", ScopeID: runID, Kind: "cancel", Reason: "cancel pending checks"}); err != nil {
				t.Fatal(err)
			}
			if err := e.Drive(ctx, runID); err != nil {
				t.Fatal(err)
			}
			r = driverRun(t, e, runID)
			if r.PendingAcceptance != nil || len(r.Attempts) != 1 || driverObservedStarts(t, e) != 1 {
				t.Fatal("resolving the pending boundary repeated its producer")
			}
			if continuation == "resume" {
				if r.Status != "completed" || r.Outcome == nil || *r.Outcome != "succeeded" || len(r.CheckExecutions) != 2 || r.Attempts[producer.ID].Accepted == nil || r.Outputs["report"] != ref {
					t.Fatal("resume did not check and accept the original producer result")
				}
			} else {
				if r.Status != "cancelled" || r.Outcome != nil || len(r.CheckExecutions) != 0 || r.Attempts[producer.ID].Accepted != nil || len(r.Outputs) != 0 || r.Steps[producer.StepID].Status != "cancelled" {
					t.Fatal("cancel ran checks, accepted a result, or took an error route")
				}
				if _, _, err := e.Artifact(ref); !os.IsNotExist(err) {
					t.Fatal("cancel published the unchecked output", err)
				}
			}
			history, err := e.Store.Read(ctx, runID, 0, 1000)
			if err != nil {
				t.Fatal(err)
			}
			for _, event := range history.Events {
				if event.Type == "stage.error_handled" {
					t.Fatal("pause/cancel was routed through on_error")
				}
			}
			if slot, owner, err := e.Store.Slot(ctx); err != nil || slot != "" || owner != "" {
				t.Fatal("pending-boundary control left an execution slot", err)
			}
		})
	}
}

func TestAcceptanceProjectedInputKeepsCheckedIdentityAcrossReopen(t *testing.T) {
	e, options := acceptanceProject(t, []string{"step_input"}, "", "pass", false)
	var workflow flow.WorkflowRevision
	var step flow.StepDefinition
	acceptanceReadJSON(t, e.Root, options.WorkflowFile, &workflow)
	acceptanceReadJSON(t, e.Root, "steps/context.json", &step)
	source := contextRegistryEntry(t, e, "schemas/check-source.json", "test:schema/check-source", "schema", "", "", []byte(`{"type":"object","required":["selected"],"properties":{"selected":{"type":"integer"}},"additionalProperties":false}`))
	selected := contextRegistryEntry(t, e, "schemas/check-selected.json", "test:schema/check-selected", "schema", "", "", []byte(`{"type":"integer"}`))
	workflow.Inputs["source"] = flow.InputPort{Port: flow.Port{Format: "json", SchemaRef: &source.Ref}, Required: true}
	input := step.Inputs["source"]
	input.Format, input.SchemaRef, input.MediaTypes = "json", &selected.Ref, nil
	step.Inputs["source"] = input
	pointer := "/selected"
	workflow.Definition.Stages["work"].InputBindings["source"] = flow.Binding{From: "workflow_input", Port: "source", Pointer: &pointer, ProjectedSchemaRef: &selected.Ref}
	acceptanceWriteStepFixture(t, e, options.WorkflowFile, workflow, step, source, selected)
	if err := os.WriteFile(filepath.Join(e.Root, "source.txt"), []byte(`{"selected":7}`), 0600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	started, err := e.Start(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	runID := started.Receipt.RunID
	r, view, err := e.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := r.plan()
	if err != nil {
		t.Fatal(err)
	}
	// The normal admission method first retains the projected input boundary;
	// it cannot create a producer Attempt before its required input check.
	if err := e.admit(ctx, r, view, plan, activationFor(&r, "work")); err != nil {
		t.Fatal(err)
	}
	r = driverRun(t, e, runID)
	if r.PendingAcceptance == nil || r.PendingAcceptance.Kind != "step_input" || len(r.Attempts) != 0 {
		t.Fatal("projected input bypassed its acceptance boundary")
	}
	ref := r.PendingAcceptance.Bindings["source"]
	artifact, data, err := e.Artifact(ref)
	if err != nil || string(data) != "7" || ref == r.Inputs["source"] || len(artifact.Provenance) != 2 || artifact.Provenance[0] != r.Inputs["source"] {
		t.Fatal("pending boundary did not retain the exact projected artifact", err)
	}
	metadata, err := os.ReadFile(filepath.Join(e.Root, artifactMetadataPath(ref.ArtifactID)))
	if err != nil {
		t.Fatal(err)
	}
	projections := acceptanceProjectionRefs(t, e)
	if len(projections) != 1 || !projections[artifact.Provenance[1]] {
		t.Fatal("fixture did not materialize exactly one projection manifest")
	}
	if err := os.WriteFile(filepath.Join(e.Root, "source.txt"), []byte(`{"selected":99}`), 0600); err != nil {
		t.Fatal(err)
	}
	e = acceptanceReopen(t, e)
	func() {
		lock, err := e.driverLock(runID)
		if err != nil {
			t.Fatal(err)
		}
		defer lock.Close()
		for i := 0; i < 3; i++ {
			r, view, err := e.load(ctx, runID)
			if err != nil {
				t.Fatal(err)
			}
			if r.ActiveCheckID != "" {
				err = e.executePendingCheck(ctx, r, view, r.CheckExecutions[r.ActiveCheckID])
			} else {
				err = e.driveAcceptance(ctx, r, view)
			}
			if err != nil {
				t.Fatal(err)
			}
		}
	}()
	r = driverRun(t, e, runID)
	if r.PendingAcceptance == nil || r.PendingAcceptance.Status != "passed" || r.PendingAcceptance.Bindings["source"] != ref || len(r.Attempts) != 0 || len(r.CheckExecutions) != 1 {
		t.Fatal("input check did not preserve the pending projection identity")
	}
	e = acceptanceReopen(t, e)
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatal(err)
	}
	r = driverRun(t, e, runID)
	if r.Status != "completed" || len(r.Attempts) != 1 || len(r.CheckExecutions) != 1 || r.ControlTransitions != 3 || !maps.Equal(projections, acceptanceProjectionRefs(t, e)) {
		t.Fatal("checked-input consumption repeated projection, checker, or producer")
	}
	for _, check := range r.CheckExecutions {
		if check.Request.Boundary != "step_input" || len(check.Request.Subjects) != 1 || check.Request.Subjects[0] != ref {
			t.Fatal("checker evaluated another projected input")
		}
	}
	for _, attempt := range r.Attempts {
		var envelope struct {
			Inputs map[string]ArtifactRef `json:"input_artifacts"`
		}
		if err := json.Unmarshal(attempt.Envelope, &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Inputs["source"] != ref || attempt.Context.Inputs["source"].Ref != ref {
			t.Fatal("producer consumed a different artifact than the checker")
		}
		inputBytes, err := readLocal(attempt.Workspace, attempt.Context.Inputs["source"].Path, MaxArtifactBytes)
		if err != nil || string(inputBytes) != "7" {
			t.Fatal("producer reread the mutable source or consumed different bytes", err)
		}
	}
	if retained, err := os.ReadFile(filepath.Join(e.Root, artifactMetadataPath(ref.ArtifactID))); err != nil || !bytes.Equal(retained, metadata) {
		t.Fatal("use-time input checking changed immutable projection metadata", err)
	}
}

func TestAcceptanceOptionalAbsentPortsNeverCreateChecks(t *testing.T) {
	e, options := acceptanceProject(t, nil, "", "pass", false)
	var workflow flow.WorkflowRevision
	var step flow.StepDefinition
	var registry RegistryFile
	acceptanceReadJSON(t, e.Root, options.WorkflowFile, &workflow)
	acceptanceReadJSON(t, e.Root, "steps/context.json", &step)
	acceptanceReadJSON(t, e.Root, "definitions.json", &registry)
	var check flow.Ref
	for _, definition := range registry.Entries {
		if definition.Ref.ID == "test:check/content" {
			check = definition.Ref
		}
	}
	port := flow.Port{Format: "blob", MediaTypes: []string{"text/plain"}, ContentCheckRefs: []flow.Ref{check}}
	workflow.Inputs["optional"] = flow.InputPort{Port: port, Required: false}
	step.Inputs["optional"] = flow.InputPort{Port: port, Required: false}
	step.Outputs["optional"] = flow.OutputPort{Port: port, RequiredFor: []string{}}
	workflow.Outputs["optional"] = flow.OutputPort{Port: port, RequiredFor: []string{}}
	workflow.Definition.Stages["work"].InputBindings["optional"] = flow.Binding{From: "workflow_input", Port: "optional"}
	workflow.Definition.Stages["done"].OutputBindings["optional"] = flow.Binding{From: "stage_output", StageID: "work", Port: "optional"}
	acceptanceWriteStepFixture(t, e, options.WorkflowFile, workflow, step)
	ctx := context.Background()
	started, err := e.Start(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	runID := started.Receipt.RunID
	if r := driverRun(t, e, runID); r.PendingAcceptance != nil || len(r.CheckExecutions) != 0 {
		t.Fatal("absent workflow input invented a check subject")
	}
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	if r.Status != "completed" || r.PendingAcceptance != nil || len(r.CheckExecutions) != 0 || len(r.Attempts) != 1 || r.ControlTransitions != 2 {
		t.Fatal("absent checked ports created checks or blocked the actual result")
	}
	if _, present := r.Inputs["optional"]; present {
		t.Fatal("absent optional workflow input became an artifact")
	}
	if _, present := r.Outputs["optional"]; present {
		t.Fatal("absent optional workflow export became an artifact")
	}
	for _, attempt := range r.Attempts {
		if attempt.Accepted == nil || len(attempt.Accepted.Outputs) != 1 || len(attempt.Context.Inputs) != 1 {
			t.Fatal("optional absence changed the admitted inputs or real result outputs")
		}
	}
	if _, data, err := e.Artifact(r.Outputs["report"]); err != nil || string(data) != "accepted output\n" {
		t.Fatal("optional absence suppressed another real output", err)
	}
	history, err := e.Store.Read(ctx, runID, 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range history.Events {
		if event.Type == "acceptance.prepared" || event.Type == "check.admitted" {
			t.Fatal("optional absence generated a fictitious acceptance obligation")
		}
	}
}

func TestAcceptanceNestedWorkflowChecksKeepInvocationOwnership(t *testing.T) {
	for _, kind := range []string{"call", "repeat"} {
		t.Run(kind, func(t *testing.T) {
			e, options := acceptanceProject(t, []string{"workflow_input", "workflow_output"}, "", "pass", false)
			var child flow.WorkflowRevision
			acceptanceReadJSON(t, e.Root, options.WorkflowFile, &child)
			child.ID = "test:workflow/checked-child"
			ref := callRegister(t, e, contextContractObject(t, child), "workflows/checked-child.json")
			parent := child
			parent.ID = "test:workflow/checked-" + kind
			parent.Inputs, parent.Outputs = maps.Clone(child.Inputs), maps.Clone(child.Outputs)
			for name, input := range parent.Inputs {
				input.ContentCheckRefs = nil
				parent.Inputs[name] = input
			}
			for name, output := range parent.Outputs {
				output.ContentCheckRefs = nil
				parent.Outputs[name] = output
			}
			definitions, _, err := Builtins()
			if err != nil {
				t.Fatal(err)
			}
			parent.PolicyRef = builtinVersionRef(definitions, "core:policy/local", "2.0.0")
			parent.Limits = flow.Limits{MaxStepInstances: 2, MaxControlTransitions: 32, MaxParallelism: 1, MaxChildDepth: 1}
			bindings := map[string]flow.Binding{"source": {From: "workflow_input", Port: "source"}}
			stage := flow.Stage{Kind: "call", WorkflowRef: ref, InputBindings: bindings, On: map[string]string{"succeeded": "done"}}
			bodies, controls := 1, int64(7)
			if kind == "repeat" {
				stage = flow.Stage{Kind: "repeat", BodyWorkflowRef: ref, InitialBindings: bindings, NextBindings: maps.Clone(bindings), ContinueOn: []string{"succeeded"}, Until: flow.Predicate{Op: "eq", Left: &flow.Operand{Kind: "literal", Value: json.RawMessage("false")}, Right: &flow.Operand{Kind: "literal", Value: json.RawMessage("true")}}, MaxIterations: 2, OnComplete: map[string]string{"succeeded": "done"}, OnLimit: "done"}
				bodies, controls = 2, 12
			}
			parent.Definition.Stages = map[string]flow.Stage{"work": stage, "done": {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]flow.Binding{"report": {From: "stage_output", StageID: "work", Port: "report"}}}}
			options.WorkflowFile = "workflows/checked-parent.json"
			writeRuntimeJSON(t, filepath.Join(e.Root, options.WorkflowFile), parent)
			ctx := context.Background()
			started, err := e.Start(ctx, options)
			if err != nil {
				t.Fatal(err)
			}
			runID := started.Receipt.RunID
			if err := e.Drive(ctx, runID); err != nil {
				t.Fatal(err)
			}
			r := driverRun(t, e, runID)
			if r.Status != "completed" || r.PendingAcceptance != nil || len(r.Invocations) != bodies+1 || len(r.Attempts) != bodies || len(r.Steps) != bodies || len(r.CheckExecutions) != 2*bodies || r.ControlTransitions != controls || r.Invocations[r.RootInvocationID].ControlTransitions != controls {
				t.Fatalf("nested checks lost scope or accounting: status=%s invocations=%d attempts=%d checks=%d controls=%d diagnostics=%+v", r.Status, len(r.Invocations), len(r.Attempts), len(r.CheckExecutions), r.ControlTransitions, r.Diagnostics)
			}
			owned := map[string]int{}
			for _, check := range r.CheckExecutions {
				invocation := r.Invocations[check.Request.InvocationID]
				if invocation == nil || invocation.ID == r.RootInvocationID || invocation.WorkflowRef != ref || check.Request.WorkflowRef != ref || check.Request.PolicyRef != child.PolicyRef || check.Request.ProducerAttemptID != "" || check.Status != "completed" || check.Report == nil || check.Report.Status != "pass" || len(check.Request.Subjects) != 1 {
					t.Fatalf("nested check borrowed another invocation or producer identity: check=%s boundary=%s status=%s invocation=%v producer=%q subjects=%d report=%+v", check.ID, check.Request.Boundary, check.Status, check.Request.InvocationID, check.Request.ProducerAttemptID, len(check.Request.Subjects), check.Report)
				}
				owned[invocation.ID]++
				switch check.Request.Boundary {
				case "workflow_input":
					if check.Request.ActivationID != "" || check.Request.Subjects[0] != invocation.Inputs["source"] {
						t.Fatal("child input check acquired a stage or another invocation's subject")
					}
				case "workflow_output":
					finish := r.activationForInvocation(invocation.ID, "done")
					if finish == nil || check.Request.ActivationID != finish.ID || check.Request.Subjects[0] != invocation.Outputs["report"] {
						t.Fatal("child export check acquired the parent's same-name finish activation")
					}
				default:
					t.Fatal("unexpected nested check boundary", check.Request.Boundary)
				}
			}
			for id, count := range owned {
				invocation := r.Invocations[id]
				if count != 2 || invocation.ControlTransitions != 4 || invocation.StepInstances != 1 || invocation.ParentInvocationID != r.RootInvocationID {
					t.Fatalf("checks did not charge their real child and ancestors exactly once: invocation=%s checks=%d controls=%d steps=%d parent=%s", id, count, invocation.ControlTransitions, invocation.StepInstances, invocation.ParentInvocationID)
				}
			}
			// The start count is read from worker files rather than from the
			// journal, so a mismatch names both numbers: they answer different
			// questions and only their disagreement is a defect.
			starts := driverObservedStarts(t, e)
			if len(owned) != bodies || r.Invocations[r.RootInvocationID].StepInstances != int64(bodies) || starts != bodies {
				t.Fatalf("nested body or checker was omitted or replayed: owned=%d bodies=%d root_steps=%d observed_starts=%d", len(owned), bodies, r.Invocations[r.RootInvocationID].StepInstances, starts)
			}
			caller := r.activationForInvocation(r.RootInvocationID, "work")
			if kind == "repeat" {
				iterations, err := r.repeatBodies(caller.ID)
				if err != nil || len(iterations) != 2 || r.Outputs["report"] != iterations[1].Outputs["report"] || iterations[0].Outputs["report"] == iterations[1].Outputs["report"] {
					t.Fatal("checked repeat exported another iteration's output", err)
				}
			} else if nested := r.childForCall(caller.ID); nested == nil || r.Outputs["report"] != nested.Outputs["report"] {
				t.Fatal("checked call lost its exact child export")
			}
		})
	}
}

func acceptanceReadJSON(t *testing.T, root, path string, value any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatal(err)
	}
	if err := decode(data, value); err != nil {
		t.Fatal(err)
	}
}

func acceptanceWriteStepFixture(t *testing.T, e *Engine, workflowPath string, workflow flow.WorkflowRevision, step flow.StepDefinition, extra ...Definition) {
	t.Helper()
	data, err := canonical(step)
	if err != nil {
		t.Fatal(err)
	}
	ref := flow.Ref{ID: step.ID, Version: step.Version, Digest: rawDigest(data)}
	if err := os.WriteFile(filepath.Join(e.Root, "steps/context.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	var registry RegistryFile
	acceptanceReadJSON(t, e.Root, "definitions.json", &registry)
	for i := range registry.Entries {
		if registry.Entries[i].Ref.ID == step.ID && registry.Entries[i].Kind == "step" {
			registry.Entries[i].Ref = ref
		}
	}
	registry.Entries = append(registry.Entries, extra...)
	writeRuntimeJSON(t, filepath.Join(e.Root, "definitions.json"), registry)
	stage := workflow.Definition.Stages["work"]
	stage.StepRef = ref
	workflow.Definition.Stages["work"] = stage
	writeRuntimeJSON(t, filepath.Join(e.Root, workflowPath), workflow)
}

func acceptanceReopen(t *testing.T, e *Engine) *Engine {
	t.Helper()
	root := e.Root
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(root, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	return reopened
}

func acceptanceProjectionRefs(t *testing.T, e *Engine) map[ArtifactRef]bool {
	t.Helper()
	refs := map[ArtifactRef]bool{}
	path := filepath.Join(e.Root, ".prifly", "artifact-refs")
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		var artifact Artifact
		acceptanceReadJSON(t, path, entry.Name(), &artifact)
		if artifact.SchemaRef != nil && artifact.SchemaRef.ID == "core:schema/json-projection" {
			refs[artifact.Ref()] = true
		}
	}
	return refs
}
