package runtime

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

// rejectedRunFixture is one assisted step whose accepted verdict routes into a
// step whose accepted verdict routes into a finish that declares an outcome
// other than success. The Run completes -- the graph did what it was told --
// and the reader is left asking where it turned.
func rejectedRunFixture(t *testing.T) (*Engine, string) {
	t.Helper()
	e, _, _ := assistedWorkspaceFixture(t, "checkout")
	definitions, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	var registry RegistryFile
	readRuntimeJSON(t, filepath.Join(e.Root, e.Config.Configuration.RegistryFile), &registry)
	var planRef flow.Ref
	for _, entry := range registry.Entries {
		if entry.Kind == "step" {
			planRef = entry.Ref
		}
	}
	workflow := flow.WorkflowRevision{
		SchemaVersion: "1", ID: "test:workflow/rejected", Version: "1.0.0", Title: "Plan, then refuse",
		Inputs: map[string]flow.InputPort{}, Outputs: map[string]flow.OutputPort{}, AllowedOutcomes: []string{"rejected"},
		Limits: flow.Limits{MaxStepInstances: 4, MaxControlTransitions: 16, MaxParallelism: 1}, PolicyRef: builtinVersionRef(definitions, "core:policy/local", "2.0.0"),
	}
	workflow.Definition.Entry = "gate"
	// Two stages declare a route into the same finish. Only one of them ever
	// settles here, which is the ordinary case and the one the edge is named
	// from; the second exists so a test can settle it too.
	workflow.Definition.Stages = map[string]flow.Stage{
		"gate":        {Kind: "step", StepRef: planRef, InputBindings: map[string]flow.Binding{}, On: map[string]string{"pass": "abandoned", "fail": "second-gate"}},
		"second-gate": {Kind: "step", StepRef: planRef, InputBindings: map[string]flow.Binding{}, On: map[string]string{"fail": "abandoned"}},
		"abandoned":   {Kind: "finish", Outcome: "rejected", OutputBindings: map[string]flow.Binding{}},
	}
	writeRuntimeJSON(t, filepath.Join(e.Root, "workflows/rejected.json"), workflow)
	result, err := e.Start(context.Background(), StartOptions{CommandID: newID("command"), WorkflowFile: "workflows/rejected.json", BriefFile: "brief.json", Inputs: map[string]string{}, WorkspaceMode: "checkout"})
	if err != nil {
		t.Fatal(err)
	}
	return e, result.Receipt.RunID
}

// A Run that reached an outcome now says where its graph stopped. Both halves
// were held already and neither was said: the host that read a rejected Run
// ordered activations by hand to find the finish stage, then opened the
// workflow source to learn which edge led there.
func TestTerminalRunNamesItsFinishAndTheEdgeThatReachedIt(t *testing.T) {
	e, runID := rejectedRunFixture(t)
	ctx := context.Background()
	task := handOver(t, e, runID)
	if _, err := e.SubmitSession(ctx, hostResult(t, e, task, "planned")); err != nil {
		t.Fatalf("the step's report was refused: %v", err)
	}
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	if r.Status != "completed" || r.Outcome == nil || *r.Outcome != "rejected" {
		t.Fatalf("the fixture did not reach its declared outcome: %s %+v", r.Status, r.Outcome)
	}
	next, err := e.Next(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if next.Action != "terminal" || next.SchemaVersion != CoreRunFinishNextVersion {
		t.Fatalf("a finished Run answered under the wrong contract: %s %s", next.Action, next.SchemaVersion)
	}
	if next.Finish == nil {
		t.Fatal("the answer did not say where the Run stopped")
	}
	finish := *next.Finish
	if finish.InvocationID != r.RootInvocationID || finish.StageID != "abandoned" || finish.Outcome != "rejected" {
		t.Fatalf("the finish is not the stage this Run ended at: %+v", finish)
	}
	if finish.FromStageID != "gate" || finish.Verdict != "pass" {
		t.Fatalf("the declared edge into the finish was not named: %+v", finish)
	}
}

// Two settled stages can declare a route into the same finish -- one stage
// that ran twice on different verdicts is enough. Nothing in the state says
// which pass took the exit, so naming either would be a guess, and map
// iteration would make it a different guess on each read.
func TestFinishEdgeIsNotNamedWhenTwoSettledStagesDeclareIt(t *testing.T) {
	e, runID := rejectedRunFixture(t)
	ctx := context.Background()
	task := handOver(t, e, runID)
	if _, err := e.SubmitSession(ctx, hostResult(t, e, task, "planned")); err != nil {
		t.Fatalf("the step's report was refused: %v", err)
	}
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	if finish := runFinish(r); finish == nil || finish.FromStageID != "gate" || finish.Verdict != "pass" {
		t.Fatalf("one settled stage did not name its edge: %+v", finish)
	}
	// A second settled pass of the same stage. The graph routes only pass, so
	// this one is given the same verdict from a separate activation: two
	// candidates for one exit, which is what a loop leaves behind.
	second := &Activation{ID: newID("activation"), StageID: "gate", InvocationID: r.RootInvocationID, Kind: "step", Status: "completed", StepID: newID("step")}
	r.Activations[second.ID] = second
	r.Steps[second.StepID] = &Step{ID: second.StepID, ActivationID: second.ID, Status: "completed", Verdict: "pass"}
	if finish := runFinish(r); finish == nil || finish.FromStageID != "gate" || finish.Verdict != "pass" {
		t.Fatalf("two passes of one stage name one edge, not none: %+v", finish)
	}
	// A different stage, settled on a verdict this graph also routes into the
	// same finish. Now the two candidates disagree and neither is the answer.
	other := &Activation{ID: newID("activation"), StageID: "second-gate", InvocationID: r.RootInvocationID, Kind: "step", Status: "completed", StepID: newID("step")}
	r.Activations[other.ID] = other
	r.Steps[other.StepID] = &Step{ID: other.StepID, ActivationID: other.ID, Status: "completed", Verdict: "fail"}
	finish := runFinish(r)
	if finish == nil || finish.StageID != "abandoned" || finish.Outcome != "rejected" {
		t.Fatalf("an unnameable edge also lost the place the Run stopped: %+v", finish)
	}
	if finish.FromStageID != "" || finish.Verdict != "" {
		t.Fatalf("one of two candidate edges was passed off as the one taken: %+v", finish)
	}
}
