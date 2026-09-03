package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

func callClone(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var clone map[string]any
	if err := json.Unmarshal(data, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}

func callRegister(t *testing.T, e *Engine, workflow map[string]any, path string) flow.Ref {
	t.Helper()
	data, err := canonical(workflow)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := flow.Digest(data)
	if err != nil {
		t.Fatal(err)
	}
	ref := flow.Ref{ID: workflow["id"].(string), Version: workflow["version"].(string), Digest: digest}
	if err := os.WriteFile(filepath.Join(e.Root, path), data, 0600); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(e.Root, "definitions.json"))
	var registry RegistryFile
	if err != nil || json.Unmarshal(raw, &registry) != nil {
		t.Fatal("read registry", err)
	}
	registry.Entries = append(registry.Entries, Definition{Ref: ref, Kind: "workflow", Path: path})
	writeRuntimeJSON(t, filepath.Join(e.Root, "definitions.json"), registry)
	return ref
}

func callStage(ref flow.Ref, inputs map[string]any, outcome, next string) map[string]any {
	bindings := map[string]any{}
	for name := range inputs {
		bindings[name] = map[string]any{"from": "workflow_input", "port": name}
	}
	return map[string]any{"kind": "call", "workflow_ref": ref, "input_bindings": bindings, "on": map[string]any{outcome: next}}
}

// Reuse the native process fixture. Child/parent deliberately share work/done
// IDs; the worker is the real test executable under the local process runner.
func callFixture(t *testing.T, mode, outcome string, twice bool) (*Engine, map[string]any, map[string]any, StartOptions) {
	t.Helper()
	e, workflow, options := choiceFixture(t, `{"flag":true}`, mode)
	child := callClone(t, workflow)
	child["id"], child["title"] = "test:workflow/call-child", "Call child"
	child["allowed_outcomes"] = []string{outcome}
	child["limits"] = map[string]any{"max_step_instances": 1, "max_control_transitions": 4, "max_parallelism": 1, "max_child_depth": 0}
	stages := map[string]any{"done": choiceFinish(outcome)}
	entry := "done"
	if mode != "" {
		stages["work"] = choiceStages(child)["work"]
		stages["work"].(map[string]any)["on"] = map[string]any{"pass": "done"}
		entry = "work"
	}
	child["outputs"] = map[string]any{}
	if mode == "commit-pass" {
		port := child["inputs"].(map[string]any)["source"].(map[string]any)
		child["outputs"] = map[string]any{"report": map[string]any{"format": port["format"], "media_types": port["media_types"], "required_for": []string{outcome}}}
		stages["done"].(map[string]any)["output_bindings"] = map[string]any{"report": map[string]any{"from": "stage_output", "stage_id": "work", "port": "report"}}
	}
	child["definition"] = map[string]any{"entry": entry, "stages": stages}
	ref := callRegister(t, e, child, "workflows/call-child.json")
	workflow["id"], workflow["title"] = "test:workflow/call-parent", "Call parent"
	defs, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	workflow["policy_ref"] = builtinVersionRef(defs, "core:policy/local", "2.0.0")
	workflow["allowed_outcomes"] = []string{outcome}
	workflow["limits"] = map[string]any{"max_step_instances": 4, "max_control_transitions": 32, "max_parallelism": 1, "max_child_depth": 4}
	workflow["outputs"] = callClone(t, child["outputs"].(map[string]any))
	parent := map[string]any{"work": callStage(ref, workflow["inputs"].(map[string]any), outcome, "done"), "done": choiceFinish(outcome)}
	producer := "work"
	if twice {
		parent["work"].(map[string]any)["on"] = map[string]any{outcome: "work_again"}
		parent["work_again"] = callStage(ref, workflow["inputs"].(map[string]any), outcome, "done")
		producer = "work_again"
	}
	if mode == "commit-pass" {
		parent["done"].(map[string]any)["output_bindings"] = map[string]any{"report": map[string]any{"from": "stage_output", "stage_id": producer, "port": "report"}}
	}
	workflow["definition"] = map[string]any{"entry": "work", "stages": parent}
	options.WorkflowFile = "workflows/call-parent.json"
	return e, workflow, child, options
}

func callEnter(t *testing.T, e *Engine, runID string) Run {
	t.Helper()
	r, view, err := e.load(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	id, stage := r.readyScope()
	p, err := r.planFor(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.enterCall(context.Background(), r, view, p, r.activationForInvocation(id, stage)); err != nil {
		t.Fatal(err)
	}
	return driverRun(t, e, runID)
}

func TestCallNativeExportsOutcomesAndRepeatedScope(t *testing.T) {
	for _, outcome := range []string{"succeeded", "partial", "rejected"} {
		t.Run(outcome, func(t *testing.T) {
			e, workflow, _, options := callFixture(t, "commit-pass", outcome, true)
			runID := choiceStart(t, e, workflow, options)
			before := driverRun(t, e, runID)
			if before.SchemaVersion != CoreInvocationStateVersion || len(before.Invocations) != 1 || before.Ready != nil {
				t.Fatal("call Run did not pin versioned root state")
			}
			if err := e.Drive(context.Background(), runID); err != nil {
				t.Fatal(err)
			}
			r := driverRun(t, e, runID)
			if r.Status != "completed" || r.Outcome == nil || *r.Outcome != outcome || len(r.Invocations) != 3 || len(r.Steps) != 2 || len(r.Attempts) != 2 || r.ControlTransitions != 9 {
				t.Fatalf("call execution: status=%s outcome=%v inv=%d steps=%d attempts=%d controls=%d", r.Status, r.Outcome, len(r.Invocations), len(r.Steps), len(r.Attempts), r.ControlTransitions)
			}
			caller := r.activationForInvocation(r.RootInvocationID, "work_again")
			child := r.childForCall(caller.ID)
			if child == nil || child.Outcome == nil || *child.Outcome != outcome || child.StepInstances != 1 || child.ControlTransitions != 2 || child.Outputs["report"] != r.Outputs["report"] {
				t.Fatal("child exports or shared counters differ")
			}
			for _, attempt := range r.Attempts {
				a := r.Activations[attempt.ActivationID]
				var envelope struct {
					InvocationID string   `json:"workflow_invocation_id"`
					WorkflowRef  flow.Ref `json:"workflow_ref"`
				}
				if err := json.Unmarshal(attempt.Envelope, &envelope); err != nil || envelope.InvocationID != a.InvocationID || envelope.WorkflowRef != r.Invocations[a.InvocationID].WorkflowRef || a.InvocationID == r.RootInvocationID {
					t.Fatal("worker envelope escaped child scope", err)
				}
				artifact, _, err := e.Artifact(attempt.Accepted.Outputs["report"])
				if err != nil {
					t.Fatal(err)
				}
				producer, _ := canonical(artifact.Producer)
				if !bytes.Contains(producer, []byte(a.InvocationID)) {
					t.Fatal("artifact lost child provenance")
				}
			}
			view, err := e.View(context.Background(), runID)
			if err != nil || view.SchemaVersion != CoreInvocationReadVersion {
				t.Fatal("call read contract", err)
			}
			if err := validatePublic(t, "CoreRunViewV2", view); err != nil {
				t.Fatal(err)
			}
			// Native time.Time retains its RFC 3339 offset;
			// this contract check does not rewrite the stored Run.
			for _, attempt := range view.Run.Attempts {
				attempt.ProcessOutcome.FinishedAt = attempt.ProcessOutcome.FinishedAt.In(time.FixedZone("test-offset", 3*60*60))
			}
			if err := validatePublic(t, "CoreRunViewV2", view); err != nil {
				t.Fatal("native timestamp offset refused", err)
			}
			if view.Timing.CalculatorRevision != "core-timing/1" || view.Timing.Root.AttemptCount != 2 {
				t.Fatal("native nested timing double-counted Attempts or used legacy semantics")
			}
			callTiming := timingFind(t, view.Timing.Root, caller.ID)
			if len(callTiming.Children) != 1 || callTiming.Children[0].ID != child.ID || callTiming.Children[0].AttemptCount != 1 {
				t.Fatal("native child timing lost its caller path")
			}
			report, err := e.Telemetry(context.Background(), TelemetryQuery{SchemaVersion: TelemetryQueryVersion, Mode: "records", RunIDs: []string{runID}, Limit: 200})
			if err != nil || report.CalculatorRevision != "core-telemetry/2" || report.Population.Invocations != 3 || report.Population.Attempts != 2 {
				t.Fatal("native telemetry did not retain actual invocation population", err)
			}
			read, _ := choiceHistory(t, e, runID)
			if err := e.Drive(context.Background(), runID); err != nil {
				t.Fatal(err)
			}
			again, _ := choiceHistory(t, e, runID)
			if read.Cut != again.Cut || driverObservedStarts(t, e) != 2 {
				t.Fatal("completed call was dispatched twice")
			}
		})
	}
}

func TestCallScopedPauseReleaseAndCancel(t *testing.T) {
	for _, kind := range []string{"pause", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			e, workflow, _, options := callFixture(t, "commit-pass", "succeeded", true)
			runID := choiceStart(t, e, workflow, options)
			r := callEnter(t, e, runID)
			childID, _ := r.readyScope()
			result, err := e.Restrict(context.Background(), RestrictCommand{SchemaVersion: "1", CommandID: newID("command"), Scope: "invocation", ScopeID: childID, Kind: kind, Reason: "Scoped call test"})
			if err != nil {
				t.Fatal(err)
			}
			if result.Receipt.RunID != runID {
				t.Fatal("scoped stop escaped Run")
			}
			if err := e.Drive(context.Background(), runID); err != nil {
				t.Fatal(err)
			}
			r = driverRun(t, e, runID)
			if len(r.Attempts) != 0 || len(r.Invocations) != 2 || r.Invocations[r.RootInvocationID].CancelRequested || r.CancelRequested {
				t.Fatal("scoped stop affected independent parent/future call")
			}
			stop := r.Stops[0]
			if stop.Scope != "invocation" || stop.ScopeID != childID {
				t.Fatal("stop scope was not persisted")
			}
			_, err = e.Release(context.Background(), ReleaseRequest{CommandID: newID("command"), RunID: runID, ExpectedControlEpoch: r.ControlEpoch, Stops: []StopGeneration{{ID: stop.ID, Generation: stop.Generation}}, Reason: "Exact scope release"})
			if kind == "cancel" {
				if err == nil || r.Status != "waiting" || r.Invocations[childID].Status != "cancelled" || r.Outcome != nil {
					t.Fatal("child cancellation became reversible or completed the Run", err)
				}
				next, err := e.Next(context.Background(), runID)
				if err != nil || next.Action != "blocked_child" || next.InvocationID != childID {
					t.Fatal("cancelled child did not explain waiting caller", err)
				}
				if _, err := e.Restrict(context.Background(), RestrictCommand{SchemaVersion: "1", CommandID: newID("command"), Scope: "run", ScopeID: runID, Kind: "cancel", Reason: "Finish test"}); err != nil {
					t.Fatal(err)
				}
				if err := e.Drive(context.Background(), runID); err != nil {
					t.Fatal(err)
				}
				if driverRun(t, e, runID).Status != "cancelled" {
					t.Fatal("Run cancellation did not close waiting tree")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			next, err := e.Next(context.Background(), runID)
			if err != nil || next.Action != "resume_required" {
				t.Fatal("release implicitly resumed child", err)
			}
			if _, err := e.Resume(context.Background(), runID, newID("command"), "Resume released call", next.RunVersion); err != nil {
				t.Fatal(err)
			}
			if err := e.Drive(context.Background(), runID); err != nil {
				t.Fatal(err)
			}
			if r = driverRun(t, e, runID); r.Status != "completed" || len(r.Attempts) != 2 {
				t.Fatal("released scope did not finish both distinct call sites")
			}
		})
	}
}

func TestCallConfigurationDoesNotReplaceExplicitAbsence(t *testing.T) {
	for _, mode := range []string{"omitted", "absent", "value"} {
		t.Run(mode, func(t *testing.T) {
			e, workflow, child, options := callFixture(t, "", "succeeded", false)
			workflow["inputs"].(map[string]any)["control"].(map[string]any)["required"] = false
			if mode != "value" {
				delete(options.Inputs, "control")
			}
			port := child["inputs"].(map[string]any)["control"].(map[string]any)
			port["required"] = mode == "omitted"
			port["configuration"] = map[string]any{"scope": "run", "default": map[string]any{"flag": false}}
			// A new identity keeps the earlier registered fixture immutable.
			child["id"] = "test:workflow/configured-child"
			child["schema_version"] = "2"
			ref := callRegister(t, e, child, "workflows/configured-child.json")
			stage := choiceStages(workflow)["work"].(map[string]any)
			stage["workflow_ref"] = ref
			if mode == "omitted" {
				delete(stage["input_bindings"].(map[string]any), "control")
			}
			runID := choiceStart(t, e, workflow, options)
			// Accepted child configuration must not be reread at call entry.
			e.Config.Configuration.InputValues = map[string]map[string]json.RawMessage{child["id"].(string): {"control": json.RawMessage(`{"flag":"changed"}`)}}
			if err := e.Drive(context.Background(), runID); err != nil {
				t.Fatal(err)
			}
			r := driverRun(t, e, runID)
			if r.Status != "completed" || len(r.Attempts) != 0 {
				t.Fatal("configured call did not complete without a worker")
			}
			inv := r.childForCall(activationFor(&r, "work").ID)
			input, present := inv.Inputs["control"]
			if mode == "absent" {
				if present {
					t.Fatal("explicit optional absence was replaced by configuration")
				}
				return
			}
			_, value, err := e.Artifact(input)
			want := `{"flag":false}`
			if mode == "value" {
				want = `{"flag":true}`
			}
			if !present || err != nil || string(value) != want {
				t.Fatalf("configured call input: %s, want %s: %v", value, want, err)
			}
		})
	}
}

func TestCallLocalAliasCycleAndPinnedResolution(t *testing.T) {
	e, workflow, child, options := callFixture(t, "", "succeeded", false)
	var registry RegistryFile
	data, _ := os.ReadFile(filepath.Join(e.Root, "definitions.json"))
	if err := json.Unmarshal(data, &registry); err != nil {
		t.Fatal(err)
	}
	registry.SchemaVersion = "2"
	registry.Entries = registry.Entries[:len(registry.Entries)-1]
	registry.Aliases = map[string]string{"child": "workflows/alias-child.json", "parent": options.WorkflowFile}
	choiceStages(workflow)["work"].(map[string]any)["workflow_ref"] = map[string]any{"alias": "child"}
	child["limits"].(map[string]any)["max_child_depth"] = 4
	child["policy_ref"] = workflow["policy_ref"]
	child["definition"] = map[string]any{"entry": "work", "stages": map[string]any{"work": map[string]any{"kind": "call", "workflow_ref": map[string]any{"alias": "parent"}, "input_bindings": map[string]any{}, "on": map[string]any{"succeeded": "done"}}, "done": choiceFinish("succeeded")}}
	writeRuntimeJSON(t, filepath.Join(e.Root, "definitions.json"), registry)
	writeRuntimeJSON(t, filepath.Join(e.Root, options.WorkflowFile), workflow)
	writeRuntimeJSON(t, filepath.Join(e.Root, "workflows/alias-child.json"), child)
	for _, op := range []string{"preview", "start"} {
		var err error
		if op == "preview" {
			_, err = e.Preview(PreviewOptions{WorkflowFile: options.WorkflowFile})
		} else {
			_, err = e.Start(context.Background(), options)
		}
		var problem *flow.Problem
		if !errors.As(err, &problem) || problem.Code != "alias_cycle" {
			t.Fatalf("%s: real A→B→A did not fail as alias_cycle: %v", op, err)
		}
	}
	snapshots, _, err := e.Store.ReadAll(context.Background(), 10)
	if err != nil || len(snapshots) != 0 || driverObservedStarts(t, e) != 0 {
		t.Fatal("alias recursion was admitted", err)
	}
	child["definition"] = map[string]any{"entry": "done", "stages": map[string]any{"done": choiceFinish("succeeded")}}
	writeRuntimeJSON(t, filepath.Join(e.Root, "workflows/alias-child.json"), child)
	runID := choiceStart(t, e, workflow, options)
	r := driverRun(t, e, runID)
	for _, def := range r.Definitions {
		if bytes.Contains(def.Bytes, []byte(`"alias":`)) {
			t.Fatal("author alias leaked into PackageLock")
		}
	}
	if err := os.WriteFile(filepath.Join(e.Root, "workflows/alias-child.json"), []byte(`{"changed":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	if driverRun(t, e, runID).Status != "completed" {
		t.Fatal("pinned call re-read a changed author alias")
	}
}

func TestCallKnownFailureAndGlobalUnknownBarrier(t *testing.T) {
	t.Run("known", func(t *testing.T) {
		e, workflow, _, options := callFixture(t, "nonzero", "succeeded", false)
		workflow["allowed_outcomes"] = []string{"succeeded", "rejected"}
		choiceStages(workflow)["work"].(map[string]any)["on_error"] = "rejected"
		choiceStages(workflow)["rejected"] = choiceFinish("rejected")
		runID := choiceStart(t, e, workflow, options)
		if err := e.Drive(context.Background(), runID); err != nil {
			t.Fatal(err)
		}
		r := driverRun(t, e, runID)
		if r.Status != "completed" || r.Outcome == nil || *r.Outcome != "rejected" || len(r.Attempts) != 1 {
			t.Fatal("known child failure did not take explicit caller error handler")
		}
		child := r.childForCall(activationFor(&r, "work").ID)
		if child == nil || child.Status != "failed" || child.Outcome != nil || len(r.Diagnostics) < 2 {
			t.Fatal("child failure evidence was lost")
		}
		read, _ := choiceHistory(t, e, runID)
		finished := 0
		for _, event := range read.Events {
			if event.Type != "invocation.finished" {
				continue
			}
			var fact struct {
				InvocationID string  `json:"workflow_invocation_id"`
				Status       string  `json:"status"`
				Outcome      *string `json:"outcome"`
			}
			if err := json.Unmarshal(event.Data, &fact); err != nil {
				t.Fatal(err)
			}
			if fact.InvocationID == child.ID {
				if fact.Status != "failed" || fact.Outcome != nil {
					t.Fatal("failure lifecycle fact invented an outcome")
				}
				finished++
			}
		}
		if finished != 1 {
			t.Fatal("failed invocation must have exactly one terminal fact", finished)
		}
	})
	t.Run("unknown", func(t *testing.T) {
		e, workflow, _, options := callFixture(t, "crash-short", "succeeded", true)
		runID := choiceStart(t, e, workflow, options)
		_, crash := choiceCrashDriver(t, "TestDriverCrashHelper", "DRIVER_CRASH_HELPER", e.Root, runID)
		before := driverWait(t, e, runID, func(r Run) bool {
			for _, a := range r.Attempts {
				if a.Started != nil {
					_, err := os.Stat(filepath.Join(a.Workspace, "worker-ready"))
					if err == nil {
						return true
					}
				}
			}
			return false
		})
		crash()
		// The short owned worker exits on its own; absence still cannot prove
		// settlement after losing the authority's process ownership.
		deadline := time.Now().Add(3 * time.Second)
		for local.ProbeProcess(*before.Attempts[before.Active[0]].Process).State == "present" && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		if err := e.Drive(context.Background(), runID); err == nil || !strings.Contains(err.Error(), "recovery_required") {
			t.Fatal("unknown child was retried", err)
		}
		r := driverRun(t, e, runID)
		if !r.HasUnresolvedEffects || r.Status != "uncertain" || len(r.Active) != 1 || len(r.Attempts) != 1 || len(r.Invocations) != 2 || activationFor(&r, "work_again") != nil {
			t.Fatal("unknown child failed to block ordinary admissions for whole Run")
		}
	})
}

func TestCallCapabilitiesDeclareBothStateContracts(t *testing.T) {
	manifest := Capabilities()
	if manifest.SchemaVersion != "capabilities/2" || len(manifest.Profiles) != 2 {
		t.Fatal("missing versioned capability discovery")
	}
	core := manifest.Profiles[1]
	if !slices.Contains(core.Capabilities, "call") || !slices.Contains(core.StateVersions, CoreStateVersion) || !slices.Contains(core.StateVersions, CoreInvocationStateVersion) || slices.Contains(manifest.Profiles[0].Capabilities, "call") {
		t.Fatal("capabilities confused retained profiles and new calls")
	}
	if err := validatePublic(t, "CoreCapabilitiesV2", manifest); err != nil {
		t.Fatal(err)
	}
}

func callActivateReady(t *testing.T, e *Engine, runID string) (Run, local.ReadView, *flow.Plan, *Activation) {
	t.Helper()
	r, view, err := e.load(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	id, stage := r.readyScope()
	p, err := r.planFor(id)
	if err != nil {
		t.Fatal(err)
	}
	if r.activationForInvocation(id, stage) == nil {
		_, err := e.apply(context.Background(), e.owner, newID("command"), runID, "stage.activated", map[string]any{"stage_id": stage}, &view.Snapshot.Version, local.CommandCAS, func(r *Run, _ local.Snapshot, obs Observation) (local.Change, error) {
			return local.Change{}, activateFor(r, p, id, stage, newID("activation"), newID("step"), 0, obs)
		})
		if err != nil {
			t.Fatal(err)
		}
		r, view, err = e.load(context.Background(), runID)
		if err != nil {
			t.Fatal(err)
		}
	}
	return r, view, p, r.activationForInvocation(id, stage)
}

func TestCallCrashAtDurableBoundaries(t *testing.T) {
	for _, boundary := range []string{"entered", "child_finished", "returned"} {
		t.Run(boundary, func(t *testing.T) {
			e, workflow, _, options := callFixture(t, "commit-pass", "succeeded", false)
			runID := choiceStart(t, e, workflow, options)
			if err := os.WriteFile(filepath.Join(e.Root, "call-boundary-kind"), []byte(boundary), 0600); err != nil {
				t.Fatal(err)
			}
			_, crash := choiceCrashDriver(t, "TestCallBoundaryHelper", "CALL_BOUNDARY_HELPER", e.Root, runID)
			before := driverWait(t, e, runID, func(r Run) bool {
				_, err := os.Stat(filepath.Join(e.Root, "call-boundary-ready"))
				caller := activationFor(&r, "work")
				if err != nil || caller == nil {
					return false
				}
				child := r.childForCall(caller.ID)
				if child == nil || len(r.Invocations) != 2 {
					return false
				}
				return boundary == "entered" || child.Status == "completed" && ((boundary == "returned") == (caller.Settled != nil))
			})
			caller := activationFor(&before, "work")
			child := before.childForCall(caller.ID)
			if child == nil || len(before.Invocations) != 2 {
				t.Fatal("boundary did not persist exactly one child")
			}
			if boundary == "entered" && len(before.Attempts) != 0 || boundary != "entered" && (len(before.Attempts) != 1 || child.Status != "completed") || (boundary == "returned") != (caller.Settled != nil) {
				t.Fatal("helper crossed requested durable boundary")
			}
			old, _ := choiceHistory(t, e, runID)
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
			after := driverRun(t, reopened, runID)
			if after.Status != "completed" || len(after.Invocations) != 2 || len(after.Attempts) != 1 || after.ControlTransitions != 5 || driverObservedStarts(t, reopened) != 1 || after.childForCall(activationFor(&after, "work").ID).ID != child.ID {
				t.Fatal("recovery duplicated the child, call return or worker")
			}
			read, _ := choiceHistory(t, reopened, runID)
			for i, event := range old.Events {
				if !bytes.Equal(event.Data, read.Events[i].Data) || !bytes.Equal(event.StateAfter, read.Events[i].StateAfter) {
					t.Fatal("recovery rewrote accepted history")
				}
			}
		})
	}
}

func TestCallBoundaryHelper(t *testing.T) {
	if os.Getenv("CALL_BOUNDARY_HELPER") != "1" {
		return
	}
	root, runID := os.Args[len(os.Args)-2], os.Args[len(os.Args)-1]
	e, err := Open(root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	lock, err := e.driverLock(runID)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	socket, closeServer, err := e.serveSteps()
	if err != nil {
		t.Fatal(err)
	}
	defer closeServer()
	boundary, err := os.ReadFile(filepath.Join(root, "call-boundary-kind"))
	if err != nil {
		t.Fatal(err)
	}
	callEnter(t, e, runID)
	if string(boundary) != "entered" {
		r, view, p, activation := callActivateReady(t, e, runID)
		if err := e.admit(context.Background(), r, view, p, activation); err != nil {
			t.Fatal(err)
		}
		r, view, err = e.load(context.Background(), runID)
		if err != nil {
			t.Fatal(err)
		}
		if err := e.executePending(context.Background(), r, view, r.Attempts[r.Active[0]], socket); err != nil {
			t.Fatal(err)
		}
		r, view, p, activation = callActivateReady(t, e, runID)
		if activation.Kind != "finish" {
			t.Fatal("child did not reach finish")
		}
		if err := e.finish(context.Background(), r, view, p, activation); err != nil {
			t.Fatal(err)
		}
		if string(boundary) == "returned" {
			r, view, p, activation = callActivateReady(t, e, runID)
			if err := e.returnCall(context.Background(), r, view, p, activation); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := os.WriteFile(filepath.Join(root, "call-boundary-ready"), boundary, 0600); err != nil {
		t.Fatal(err)
	}
	// Only the parent test signals this owned helper; production has no test
	// pause point or alternate worker path. The wait is bounded by this test.
	time.Sleep(10 * time.Second)
	t.Fatal("parent did not stop the owned helper at its boundary")
}

func TestCallNestedScopeStopsDescendantsOnly(t *testing.T) {
	e, workflow, _, options := callFixture(t, "commit-pass", "succeeded", false)
	middle := callClone(t, workflow)
	middle["id"] = "test:workflow/call-middle"
	middle["limits"].(map[string]any)["max_child_depth"] = 1
	ref := callRegister(t, e, middle, "workflows/call-middle.json")
	choiceStages(workflow)["work"].(map[string]any)["workflow_ref"] = ref
	runID := choiceStart(t, e, workflow, options)
	r := callEnter(t, e, runID)
	middleID, _ := r.readyScope()
	callActivateReady(t, e, runID)
	r = callEnter(t, e, runID)
	leafID, _ := r.readyScope()
	if len(r.Invocations) != 3 || !r.withinInvocation(leafID, middleID) {
		t.Fatal("nested call did not preserve ancestry")
	}
	if _, err := e.Restrict(context.Background(), RestrictCommand{SchemaVersion: "1", CommandID: newID("command"), Scope: "invocation", ScopeID: middleID, Kind: "cancel", Reason: "Cancel nested subtree"}); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	r = driverRun(t, e, runID)
	if r.Invocations[middleID].Status != "cancelled" || r.Invocations[leafID].Status != "cancelled" || r.Invocations[r.RootInvocationID].Status == "cancelled" || len(r.Attempts) != 0 || r.Status != "waiting" {
		t.Fatal("invocation cancel escaped or omitted descendants")
	}
}
