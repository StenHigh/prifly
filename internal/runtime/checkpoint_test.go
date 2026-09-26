package runtime

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

// checkpointFixture is one assisted step whose plan output the workflow
// declares as its checkpoint. What a checkpoint holds is the author's; here it
// is simply the plan the step reports.
func checkpointFixture(t *testing.T) (*Engine, string) {
	t.Helper()
	e, _, _ := assistedWorkspaceFixture(t, "checkout")
	definitions, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	var registry RegistryFile
	readRuntimeJSON(t, filepath.Join(e.Root, e.Config.Configuration.RegistryFile), &registry)
	var planStep flow.StepDefinition
	var planRef flow.Ref
	for _, entry := range registry.Entries {
		if entry.Kind == "step" {
			readRuntimeJSON(t, filepath.Join(e.Root, entry.Path), &planStep)
			planRef = entry.Ref
		}
	}
	workflow := flow.WorkflowRevision{
		SchemaVersion: flow.WorkflowRevisionContinuationVersion, ID: "test:workflow/checkpointed", Version: "1.0.0", Title: "Keep a checkpoint",
		Inputs: map[string]flow.InputPort{}, Outputs: map[string]flow.OutputPort{}, AllowedOutcomes: []string{"succeeded"},
		Limits:     flow.Limits{MaxStepInstances: 2, MaxControlTransitions: 8, MaxParallelism: 1},
		PolicyRef:  builtinVersionRef(definitions, "core:policy/local", "2.0.0"),
		Checkpoint: &flow.CheckpointDeclaration{SchemaRef: *planStep.Outputs["plan"].SchemaRef},
	}
	workflow.Definition.Entry = "plan"
	workflow.Definition.Stages = map[string]flow.Stage{
		"plan": {Kind: "step", StepRef: planRef, InputBindings: map[string]flow.Binding{}, On: map[string]string{"pass": "done"}, ImpossibleVerdicts: []string{"fail", "needs_revision", "no_work", "blocked"}, Checkpoint: "plan"},
		"done": {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]flow.Binding{}},
	}
	writeRuntimeJSON(t, filepath.Join(e.Root, "workflows/checkpointed.json"), workflow)
	result, err := e.Start(context.Background(), StartOptions{CommandID: newID("command"), WorkflowFile: "workflows/checkpointed.json", BriefFile: "brief.json", Inputs: map[string]string{}, WorkspaceMode: "checkout"})
	if err != nil {
		t.Fatal(err)
	}
	return e, result.Receipt.RunID
}

// The state is read, not the declaration: a workflow keeping a checkpoint is
// sealed at the state whose read names it, and the read names the output the
// step reported once it is accepted -- not before.
func TestAcceptedCheckpointIsNamedOnTheRunRead(t *testing.T) {
	e, runID := checkpointFixture(t)
	ctx := context.Background()
	if r := driverRun(t, e, runID); r.SchemaVersion != CoreContinuationStateVersion {
		t.Fatalf("sealed at %s, not the state whose read names a checkpoint", r.SchemaVersion)
	}
	task := handOver(t, e, runID)
	before, err := e.View(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if before.Checkpoint != nil {
		t.Fatalf("a checkpoint was named before any was accepted: %+v", before.Checkpoint)
	}
	if unsettled, _, err := e.load(ctx, runID); err != nil {
		t.Fatal(err)
	} else if checkpoint, err := lastCheckpoint(unsettled); err != nil || checkpoint != nil {
		t.Fatalf("a Run that accepted nothing reported a checkpoint: %+v %v", checkpoint, err)
	}
	if _, err := e.SubmitSession(ctx, hostResult(t, e, task, "planned")); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	view, err := e.View(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	var reported ArtifactRef
	for _, step := range r.Steps {
		reported = step.Outputs["plan"]
	}
	if view.SchemaVersion != CoreContinuationReadVersion || view.Checkpoint == nil || view.Checkpoint.Ref != reported || view.Checkpoint.StageID != "plan" || view.Checkpoint.InvocationID != r.RootInvocationID {
		t.Fatalf("the read does not name the accepted checkpoint %v: %+v", reported, view.Checkpoint)
	}
	// A continuation declaring the checkpoint as an input takes exactly it.
	target := &flow.Plan{}
	target.Workflow.ID = "test:workflow/continue"
	target.Workflow.Continuation = &flow.Continuation{FromWorkflows: []string{"test:workflow/checkpointed"}, FromOutcomes: []string{"succeeded"}, Inputs: map[string]flow.ContinuationSource{"resume": {Checkpoint: true}}}
	source, _, err := e.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	carried, err := continuationRefs(source, 1, target)
	if err != nil || carried.Inputs["resume"].Ref != reported || carried.Checkpoint == nil || carried.Checkpoint.StageID != "plan" {
		t.Fatalf("the continuation did not take the accepted checkpoint: %+v %v", carried, err)
	}
	for name, value := range map[string]any{"CoreRunStateV40": r, "CoreRunViewV40": view} {
		if err := validatePublic(t, name, value); err != nil {
			t.Fatalf("%s rejected the Run: %v", name, err)
		}
	}
}

// A Run needing several features is sealed at the highest of them. The last
// feature checked used to win instead: a project title, checked after an
// external write, sealed the Run at 37, whose contract has no place for the
// boundary 39 records.
func TestAProjectTitleDoesNotLowerTheSealedState(t *testing.T) {
	e, _, err := externalWriteFixture(t, func(*flow.StepDefinition) {})
	if err != nil {
		t.Fatal(err)
	}
	result, err := e.Start(context.Background(), StartOptions{CommandID: newID("command"), WorkflowFile: "workflows/publishing.json", BriefFile: "brief.json", Inputs: map[string]string{}, WorkspaceMode: "checkout", ProjectTitle: "A project"})
	if err != nil {
		t.Fatal(err)
	}
	if r := driverRun(t, e, result.Receipt.RunID); r.SchemaVersion != CoreExternalWriteStateVersion || r.ProjectTitle != "A project" {
		t.Fatalf("sealed at %s with title %q", r.SchemaVersion, r.ProjectTitle)
	}
}
