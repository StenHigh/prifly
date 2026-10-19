package runtime

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

func resolvedWriterRecoveryFixture(t *testing.T, kind, scope, outcome string) (*Engine, Run, []local.Event, string) {
	t.Helper()
	policy := flow.WorkspaceTreeCapturePolicy{Kind: "exact_file", Path: ".ai-factory/PLAN.md"}
	e, id := treeFixture(t, policy, nil, nil, "worktree", false, func(e *Engine, root *flow.WorkflowRevision, registry *RegistryFile) {
		root.SchemaVersion = "8"
		root.Resumable = &flow.Resumable{FromCancelled: true}
		for id, stage := range root.Definition.Stages {
			if stage.Kind == "step" {
				stage.ImpossibleVerdicts = []string{"fail", "needs_revision", "no_work", "blocked"}
				root.Definition.Stages[id] = stage
			}
		}
		if kind == "step" {
			return
		}
		defs, _, err := Builtins()
		if err != nil {
			t.Fatal(err)
		}
		manifest := builtinRef(defs, flow.WorkspaceTreeManifestSchemaID)
		input := flow.InputPort{Port: flow.Port{Format: "json", SchemaRef: &manifest}, Required: true}
		leaf := root.Definition.Stages["improve"]
		leaf.InputBindings = map[string]flow.Binding{"plan": {From: "workflow_input", Port: "plan"}}
		leaf.On = map[string]string{"pass": "done"}
		child := *root
		child.ID = "test:workflow/resolved-writer"
		child.Inputs = map[string]flow.InputPort{"plan": input}
		child.Outputs = map[string]flow.OutputPort{"improved": {Port: input.Port, RequiredFor: []string{"succeeded"}}}
		child.Definition.Entry = "improve"
		child.Definition.Stages = map[string]flow.Stage{"improve": leaf, "done": {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]flow.Binding{"improved": {From: "stage_output", StageID: "improve", Port: "improved"}}}}
		data := writeRegistryDocument(t, e, "workflows/resolved-writer.json", child)
		ref := flow.Ref{ID: child.ID, Version: child.Version, Digest: rawDigest(data)}
		registry.Entries = append(registry.Entries, Definition{Ref: ref, Kind: "workflow", Path: "workflows/resolved-writer.json"})
		bindings := root.Definition.Stages["improve"].InputBindings
		stage := flow.Stage{Kind: "call", WorkflowRef: ref, InputBindings: bindings, On: map[string]string{"succeeded": "implement"}}
		if kind == "repeat" {
			stage = flow.Stage{Kind: "repeat", BodyWorkflowRef: ref, InitialBindings: bindings, NextBindings: map[string]flow.Binding{"plan": {From: "iteration_output", Port: "improved"}}, ContinueOn: []string{"succeeded"}, MaxIterations: 2, OnComplete: map[string]string{"succeeded": "implement"}, OnLimit: "implement", Until: flow.Predicate{Op: "eq", Left: &flow.Operand{Kind: "literal", Value: json.RawMessage("true")}, Right: &flow.Operand{Kind: "literal", Value: json.RawMessage("true")}}}
		}
		root.Definition.Stages["improve"] = stage
		root.Limits = flow.Limits{MaxStepInstances: 8, MaxControlTransitions: 32, MaxParallelism: 1, MaxChildDepth: 2}
	})
	ctx := context.Background()
	producer := handOver(t, e, id)
	writeWorkspaceTreeFile(t, producer.RepositoryWorkspace, policy.Path, "# Accepted\n")
	if _, err := e.SubmitSession(ctx, treeSubmission(t, producer, "accepted plan", []WorkspaceTreeLocation{{OutputPort: "plan", Path: policy.Path}})); err != nil {
		t.Fatal(err)
	}
	writer := handOver(t, e, id)
	before := driverRun(t, e, id)
	scopeID := id
	if scope == "invocation" {
		scopeID = before.Activations[before.Attempts[writer.AttemptID].ActivationID].InvocationID
	}
	if _, err := e.Restrict(ctx, RestrictCommand{SchemaVersion: "1", CommandID: newID("command"), Scope: scope, ScopeID: scopeID, Kind: "cancel", Reason: "stop fixture actor"}); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(ctx, id); refusalCode(err) != "recovery_required" {
		t.Fatalf("unresolved writer: %v", err)
	}
	v, err := e.View(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	unsettled := driverRun(t, e, id)
	p, err := unsettled.plan()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.PlanRecovery(ctx, RecoveryRequest{SourceRunID: id, SourceRunVersion: v.RunVersion}, p, unsettled.Definitions, unsettled.ContextResources); refusalCode(err) != "recover_source_unsettled" {
		t.Fatalf("unknown effects admitted: %v", err)
	}
	if _, err := e.ResolveObligation(ctx, id, newID("command"), writer.AttemptID, "", outcome, "actor stopped; inspected effects", v.RunVersion); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(ctx, id); err != nil {
		t.Fatal(err)
	}
	source := driverRun(t, e, id)
	page, err := e.Events(ctx, id, 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	return e, source, page.Events, writer.AttemptID
}

func TestRecoverResolvedCancellationNestedWriter(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"step", "call", "repeat"} {
		for _, scope := range []string{"run", "invocation"} {
			for _, outcome := range []string{ResolveOutcomeApplied, ResolveOutcomeNotApplied} {
				t.Run(kind+"/"+scope+"/"+outcome, func(t *testing.T) {
					e, source, _, attemptID := resolvedWriterRecoveryFixture(t, kind, scope, outcome)
					p, err := source.plan()
					if err != nil {
						t.Fatal(err)
					}
					v, err := e.View(context.Background(), source.ID)
					if err != nil {
						t.Fatal(err)
					}
					request := RecoveryRequest{SourceRunID: source.ID, SourceRunVersion: v.RunVersion}
					review, err := e.PlanRecovery(context.Background(), request, p, source.Definitions, source.ContextResources)
					if err != nil {
						t.Fatalf("resolved %s: %s %+v: %v", kind, source.Status, source.Diagnostics, err)
					}
					if review.FrontierStageID != "improve" || review.FrontierAction != "execute" || review.CandidateRef != nil || source.Attempts[attemptID].Accepted != nil {
						t.Fatalf("resolution became a result: %+v", review)
					}
					if kind != "step" && review.FrontierAttemptID != "" {
						t.Fatal("nested writer was promoted to root")
					}
					record, err := e.Claims(context.Background())
					if err != nil {
						t.Fatal(err)
					}
					var claim *WorktreeClaim
					for _, c := range record.Claims {
						if c.RunID == source.ID && c.Status == "active" {
							copy := c
							claim = &copy
						}
					}
					request.ReviewDigest = review.ReviewDigest
					started, err := e.Start(context.Background(), StartOptions{SchemaVersion: "2", CommandID: newID("command"), WorkflowFile: "workflows/tree.json", Inputs: map[string]string{}, WorkspaceMode: "worktree", WorkspaceClaim: claim, Recovery: &request})
					if err != nil {
						t.Fatal(err)
					}
					next := handOver(t, e, started.Receipt.RunID)
					wantPath, err := e.claimWorkspacePath(*claim)
					if err != nil {
						t.Fatal(err)
					}
					if next.AttemptID == attemptID || next.RepositoryWorkspace != wantPath {
						t.Fatal("writer not repeated in same tree")
					}
					if _, err := e.SubmitSession(context.Background(), treeSubmission(t, next, "fresh writer", nil)); err != nil {
						t.Fatal(err)
					}
					tail := handOver(t, e, started.Receipt.RunID)
					if _, err := e.SubmitSession(context.Background(), treeSubmission(t, tail, "fresh tail", nil)); err != nil {
						t.Fatal(err)
					}
					if err := e.Drive(context.Background(), started.Receipt.RunID); err != nil {
						t.Fatal(err)
					}
					if got := driverRun(t, e, started.Receipt.RunID); got.Status != "completed" {
						t.Fatalf("replay failed: %+v", got.Diagnostics)
					}
				})
			}
		}
	}
}

func TestRecoverCancelledWriterRequiresCorrelatedResolution(t *testing.T) {
	t.Parallel()
	_, original, journal, attemptID := resolvedWriterRecoveryFixture(t, "call", "invocation", ResolveOutcomeNotApplied)
	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*Run, *[]local.Event){
		"no journal": func(_ *Run, events *[]local.Event) { *events = nil },
		"wrong resolution command": func(_ *Run, events *[]local.Event) {
			for i := range *events {
				if (*events)[i].Type == "attempt.resolved" {
					(*events)[i].CommandID = "command:other"
				}
			}
		},
		"wrong cancel command": func(_ *Run, events *[]local.Event) {
			for i := range *events {
				if (*events)[i].Type == "run.restricted" {
					(*events)[i].CommandID = "command:other"
				}
			}
		},
		"later cancellation": func(_ *Run, events *[]local.Event) {
			for i := range *events {
				if (*events)[i].Type == "run.restricted" {
					(*events)[i].Seq = 1 << 50
				}
			}
		},
		"missing diagnostic": func(r *Run, _ *[]local.Event) { r.Diagnostics = nil },
		"unresolved effects": func(r *Run, _ *[]local.Event) { r.HasUnresolvedEffects = true },
		"active attempt":     func(r *Run, _ *[]local.Event) { r.Active = []string{attemptID} },
		"pause":              func(r *Run, _ *[]local.Event) { r.Stops = append(r.Stops, Stop{Kind: "pause", Status: "active"}) },
		"live descendant": func(r *Run, _ *[]local.Event) {
			for _, inv := range r.Invocations {
				if inv.ParentInvocationID != "" {
					inv.Status = "running"
					inv.Settled = nil
				}
			}
		},
		"foreign diagnostic": func(r *Run, _ *[]local.Event) {
			for i := range r.Diagnostics {
				if r.Diagnostics[i].Phase == "resolution" {
					r.Diagnostics[i].AttemptID = "attempt:other"
				}
			}
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var source Run
			if err := json.Unmarshal(encoded, &source); err != nil {
				t.Fatal(err)
			}
			events := append([]local.Event{}, journal...)
			change(&source, &events)
			if _, _, err := recoveryFrontier(source, events); err == nil {
				t.Fatal("unproven cancellation recovery admitted")
			}
		})
	}
	// An unresolved process observation remains historical evidence after owner
	// attestation; the attestation permits execute, never candidate revalidation.
	original.Attempts[attemptID].ProcessOutcome = &local.ProcessOutcome{Uncertain: true}
	if _, _, err := recoveryFrontier(original, journal); err != nil {
		t.Fatalf("resolved uncertain process still blocked: %v", err)
	}
}
