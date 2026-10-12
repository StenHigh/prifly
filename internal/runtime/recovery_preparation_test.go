package runtime

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// Fail the actual workspace-tree materialization after accepting a native plan.
// Every caller failure is produced by Drive; no Attempt result is invented.
func recoveryPreparationFixture(t *testing.T, kinds ...string) (*Engine, Run, []local.Event) {
	t.Helper()
	policy := flow.WorkspaceTreeCapturePolicy{Kind: "direct_child_file", Path: ".ai-factory/plans"}
	e, runID := treeFixture(t, policy, nil, nil, "worktree", false, func(e *Engine, root *flow.WorkflowRevision, registry *RegistryFile) {
		defs, _, err := Builtins()
		if err != nil {
			t.Fatal(err)
		}
		warmup := flow.StepDefinition{SchemaVersion: "1", ID: "test:step/recover-warmup", Version: "1.0.0", Title: "warmup", Kind: "worker", Inputs: map[string]flow.InputPort{}, Outputs: map[string]flow.OutputPort{}, ContextRefs: []flow.Ref{}, RequiredCapabilities: []string{}, ResultCheckRefs: []flow.Ref{}, ResultSchemaRef: builtinRef(defs, "core:schema/step-result")}
		warmup.InstructionsRef = &registry.Entries[0].Ref
		warmup.Executor.AdapterRef, warmup.Executor.Operation = builtinRef(defs, "core:adapter/assisted-session"), "session"
		warmup.Effects.Class, warmup.Effects.RetryClass = "none", "never"
		data := writeRegistryDocument(t, e, "steps/recover-warmup.json", warmup)
		registry.Entries = append(registry.Entries, Definition{Ref: flow.Ref{ID: warmup.ID, Version: warmup.Version, Digest: rawDigest(data)}, Kind: "step", Path: "steps/recover-warmup.json"})
		root.Definition.Entry = "warmup"
		root.Definition.Stages["warmup"] = flow.Stage{Kind: "step", StepRef: registry.Entries[len(registry.Entries)-1].Ref, InputBindings: map[string]flow.Binding{}, On: map[string]string{"pass": "plan"}}
		root.Limits = flow.Limits{MaxStepInstances: 32, MaxControlTransitions: 128, MaxParallelism: 1, MaxChildDepth: 8}
		if len(kinds) == 0 {
			return
		}
		manifest := builtinRef(defs, flow.WorkspaceTreeManifestSchemaID)
		input := flow.InputPort{Port: flow.Port{Format: "json", SchemaRef: &manifest}, Required: true}
		output := flow.OutputPort{Port: input.Port, RequiredFor: []string{"succeeded"}}
		child := *root
		child.ID, child.Title = "test:workflow/recovery-leaf", "Recovery leaf"
		child.Inputs, child.Outputs = map[string]flow.InputPort{"plan": input}, map[string]flow.OutputPort{"improved": output}
		leaf := root.Definition.Stages["improve"]
		if len(kinds) == 1 && kinds[0] == "handled-call" {
			leaf.OnError = "implement"
		}
		leaf.InputBindings = map[string]flow.Binding{"plan": {From: "workflow_input", Port: "plan"}}
		child.Definition.Entry, child.Definition.Stages = "improve", map[string]flow.Stage{"improve": leaf, "implement": root.Definition.Stages["implement"], "done": {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]flow.Binding{"improved": {From: "stage_output", StageID: "improve", Port: "improved"}}}}
		register := func(w flow.WorkflowRevision, name string) flow.Ref {
			path := "workflows/" + name + ".json"
			data := writeRegistryDocument(t, e, path, w)
			ref := flow.Ref{ID: w.ID, Version: w.Version, Digest: rawDigest(data)}
			registry.Entries = append(registry.Entries, Definition{Ref: ref, Kind: "workflow", Path: path})
			return ref
		}
		if len(kinds) == 1 && kinds[0] == "handled-call" {
			following := child.Definition.Stages["implement"]
			following.InputBindings = map[string]flow.Binding{"plan": {From: "workflow_input", Port: "plan"}}
			child.Definition.Stages["implement"] = following
			done := child.Definition.Stages["done"]
			done.OutputBindings = map[string]flow.Binding{"improved": {From: "stage_output", StageID: "implement", Port: "final"}}
			child.Definition.Stages["done"] = done
		}
		ref := register(child, "recovery-leaf")
		for i := len(kinds) - 1; i >= 0; i-- {
			bindings := map[string]flow.Binding{"plan": {From: "workflow_input", Port: "plan"}}
			stage := flow.Stage{Kind: "call", WorkflowRef: ref, InputBindings: bindings, On: map[string]string{"succeeded": "done"}}
			if kinds[i] == "repeat" || kinds[i] == "repeat-after-accepted" {
				stage = flow.Stage{Kind: "repeat", BodyWorkflowRef: ref, InitialBindings: bindings, NextBindings: bindings, ContinueOn: []string{"succeeded"}, MaxIterations: 2, OnComplete: map[string]string{"succeeded": "done"}, OnLimit: "done", Until: flow.Predicate{Op: "eq", Left: &flow.Operand{Kind: "literal", Value: json.RawMessage("true")}, Right: &flow.Operand{Kind: "literal", Value: json.RawMessage("true")}}}
				if kinds[i] == "repeat-after-accepted" {
					stage.Until.Left.Value = json.RawMessage("false")
				}
			}
			if i == 0 {
				if stage.Kind == "call" {
					stage.InputBindings = root.Definition.Stages["improve"].InputBindings
				} else {
					stage.InitialBindings = root.Definition.Stages["improve"].InputBindings
					stage.NextBindings = map[string]flow.Binding{"plan": {From: "iteration_output", Port: "improved"}}
				}
				stage.On, stage.OnComplete, stage.OnLimit = map[string]string{"succeeded": "implement"}, stage.OnComplete, stage.OnLimit
				if stage.Kind == "repeat" {
					stage.On = nil
					stage.OnComplete = map[string]string{"succeeded": "implement"}
					stage.OnLimit = "implement"
				}
				root.Definition.Stages["improve"] = stage
				break
			}
			child.ID = "test:workflow/recovery-wrapper-" + kinds[i]
			child.Definition.Entry, child.Definition.Stages = "improve", map[string]flow.Stage{"improve": stage, "done": {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]flow.Binding{"improved": {From: "stage_output", StageID: "improve", Port: "improved"}}}}
			ref = register(child, filepath.Base(child.ID))
		}
	})
	warmup := handOver(t, e, runID)
	if _, err := e.SubmitSession(context.Background(), treeSubmission(t, warmup, "warmup accepted", nil)); err != nil {
		t.Fatal(err)
	}
	plan := handOver(t, e, runID)
	path := ".ai-factory/plans/source.md"
	writeWorkspaceTreeFile(t, plan.RepositoryWorkspace, path, "# Accepted plan\n")
	if _, err := e.SubmitSession(context.Background(), treeSubmission(t, plan, "plan accepted", []WorkspaceTreeLocation{{OutputPort: "plan", Path: path}})); err != nil {
		t.Fatal(err)
	}
	wantAttempts := 2
	if len(kinds) == 1 && kinds[0] == "repeat-after-accepted" {
		improve := handOver(t, e, runID)
		if _, err := e.SubmitSession(context.Background(), treeSubmission(t, improve, "accepted first body improve", nil)); err != nil {
			t.Fatal(err)
		}
		implement := handOver(t, e, runID)
		if _, err := e.SubmitSession(context.Background(), treeSubmission(t, implement, "accepted first body implement", nil)); err != nil {
			t.Fatal(err)
		}
		wantAttempts = 4
	}
	writeWorkspaceTreeFile(t, plan.RepositoryWorkspace, path, "# Preserved later edit\n")
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	if r.Status != "failed" || len(r.Attempts) != wantAttempts {
		t.Fatalf("not a pre-admission failure: %s attempts=%d", r.Status, len(r.Attempts))
	}
	page, err := e.Events(context.Background(), runID, 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	return e, r, page.Events
}

func TestRecoverPreparationRootFrontier(t *testing.T) {
	t.Parallel()
	for _, kinds := range [][]string{nil, {"call"}, {"repeat"}, {"call", "repeat", "call"}, {"handled-call"}, {"repeat-after-accepted"}} {
		t.Run("root-"+filepath.Join(append([]string{"step"}, kinds...)...), func(t *testing.T) {
			e, source, events := recoveryPreparationFixture(t, kinds...)
			plan, err := source.plan()
			if err != nil {
				t.Fatal(err)
			}
			view, err := e.View(context.Background(), source.ID)
			if err != nil {
				t.Fatal(err)
			}
			for _, from := range []string{"", "plan"} {
				recovered, err := e.PlanRecovery(context.Background(), RecoveryRequest{SourceRunID: source.ID, SourceRunVersion: view.RunVersion, FromStage: from}, plan, source.Definitions, source.ContextResources)
				if err != nil {
					t.Fatalf("prepare from %q: %v", from, err)
				}
				want := "improve"
				if from != "" {
					want = from
				}
				if recovered.FrontierStageID != want || recovered.FrontierAction != "execute" || recovered.FrontierAttemptID != "" || recovered.CandidateRef != nil {
					t.Fatalf("invented root result: %+v", recovered)
				}
				for _, reused := range recovered.Reused {
					if reused.StageID == "improve" {
						t.Fatal("caller subtree crossed root cutoff")
					}
				}
			}
			for from, code := range map[string]string{"improve": "resume_from_stage_invalid", "implement": "resume_from_stage_invalid", "nested-only": "resume_from_stage_invalid", "warmup": "resume_prefix_empty"} {
				_, err := e.PlanRecovery(context.Background(), RecoveryRequest{SourceRunID: source.ID, SourceRunVersion: view.RunVersion, FromStage: from}, plan, source.Definitions, source.ContextResources)
				if refusalCode(err) != code {
					t.Fatalf("from %s: want %s, got %v", from, code, err)
				}
			}
			if _, err := e.PlanRecovery(context.Background(), RecoveryRequest{SourceRunID: source.ID, SourceRunVersion: view.RunVersion + 1}, plan, source.Definitions, source.ContextResources); refusalCode(err) != "recover_source_changed" {
				t.Fatalf("stale source accepted: %v", err)
			}
			found := false
			for _, event := range events {
				if event.Type == "stage.failed" {
					var fact map[string]any
					if json.Unmarshal(event.Data, &fact) == nil && fact["execution"] == "not_admitted" {
						found = true
					}
				}
			}
			if !found {
				t.Fatal("fixture omitted recorded preparation evidence")
			}
		})
	}
}

func TestRecoverNestedFrontierDoesNotPromoteLeafAttempt(t *testing.T) {
	t.Parallel()
	e, workflow, _, options := callFixture(t, "nonzero", "succeeded", false)
	runID := choiceStart(t, e, workflow, options)
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	source := driverRun(t, e, runID)
	frontier, attempt, err := recoveryFrontier(source)
	if err != nil || frontier.Kind != "call" || frontier.InvocationID != source.RootInvocationID || attempt != nil {
		t.Fatalf("nested result promoted: %+v %+v %v", frontier, attempt, err)
	}
	for _, leaf := range source.Attempts {
		leaf.ProcessOutcome.Uncertain = true
		if _, _, err := recoveryFrontier(source); refusalCode(err) != "recover_source_unsettled" {
			t.Fatalf("uncertain leaf accepted: %v", err)
		}
	}
}

func TestRecoverNestedFrontierRejectsUnprovenOrIndependentFailures(t *testing.T) {
	t.Parallel()
	_, original, journal := recoveryPreparationFixture(t, "call", "repeat", "call")
	root := original.activationForInvocation(original.RootInvocationID, "improve")
	var leaf *Activation
	for _, a := range original.Activations {
		if a.Status == "failed" && a.Kind == "step" {
			leaf = a
		}
	}
	if leaf == nil {
		t.Fatal("fixture lacks a failed leaf")
	}
	cases := map[string]func(*Run, *[]local.Event){
		"missing caller":     func(r *Run, _ *[]local.Event) { delete(r.Activations, root.ID) },
		"missing invocation": func(r *Run, _ *[]local.Event) { delete(r.Invocations, leaf.InvocationID) },
		"cycle": func(r *Run, _ *[]local.Event) {
			r.Invocations[leaf.InvocationID].ParentInvocationID = leaf.InvocationID
		},
		"wrong cause": func(r *Run, _ *[]local.Event) {
			for i := range r.Diagnostics {
				if r.Diagnostics[i].ActivationID == root.ID {
					r.Diagnostics[i].CauseRefs = []string{"invocation:unrelated"}
				}
			}
		},
		"wrong repeat decision": func(r *Run, _ *[]local.Event) {
			for _, a := range r.Activations {
				if a.Kind == "repeat" {
					a.Repeat.LastDecision.BodyInvocationID = "invocation:unrelated"
				}
			}
		},
		"independent leaf in same caller": func(r *Run, _ *[]local.Event) {
			a := *r.Activations[leaf.ID]
			a.ID = "activation:independent"
			r.Activations[a.ID] = &a
		},
		"second root cause": func(r *Run, _ *[]local.Event) {
			a := *r.Activations[root.ID]
			a.ID = "activation:independent"
			r.Activations[a.ID] = &a
		},
		"unsettled child": func(r *Run, _ *[]local.Event) { r.Invocations[leaf.InvocationID].Settled = nil },
		"missing preparation diagnostic": func(r *Run, _ *[]local.Event) {
			for i := range r.Diagnostics {
				if r.Diagnostics[i].ActivationID == leaf.ID {
					r.Diagnostics[i].Phase = "other"
				}
			}
		},
		"missing not-admitted journal": func(_ *Run, events *[]local.Event) { *events = nil },
		"unlisted admitted Attempt": func(r *Run, _ *[]local.Event) {
			r.Attempts["attempt:unlisted"] = &Attempt{ID: "attempt:unlisted", ActivationID: leaf.ID, StepID: leaf.StepID, Settled: leaf.Settled}
		},
		"active execution":  func(r *Run, _ *[]local.Event) { r.Active = []string{"attempt:active"} },
		"uncertain effects": func(r *Run, _ *[]local.Event) { r.HasUnresolvedEffects = true },
		"stop":              func(r *Run, _ *[]local.Event) { r.Stops = []Stop{{Kind: "pause", Status: "active"}} },
	}
	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			var source Run
			if err := json.Unmarshal(encoded, &source); err != nil {
				t.Fatal(err)
			}
			events := append([]local.Event{}, journal...)
			mutate(&source, &events)
			if _, _, err := recoveryFrontier(source, events); err == nil {
				t.Fatal("unproven failure was accepted")
			}
			plan, err := original.plan()
			if err != nil {
				t.Fatal(err)
			}
			sequences, err := activationSequences(journal)
			if err != nil {
				t.Fatal(err)
			}
			// Earlier accepted stages do not bypass natural eligibility.
			if _, err := recoveryPointOf(source, plan, plan, sequences, "plan", events); err == nil {
				t.Fatal("earlier stage bypassed refusal")
			}
		})
	}
}
