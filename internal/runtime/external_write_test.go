package runtime

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

// externalWriteFixture is one assisted step that changes something outside this
// authority, declaring what it may change. The names here are placeholders on
// purpose: which system, which operations and which target are the workflow
// author's to declare, and this engine knows none of them.
func externalWriteFixture(t *testing.T, shape func(*flow.StepDefinition)) (*Engine, string, error) {
	t.Helper()
	e, _, _ := assistedWorkspaceFixture(t, "checkout")
	definitions, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	var registry RegistryFile
	readRuntimeJSON(t, filepath.Join(e.Root, e.Config.Configuration.RegistryFile), &registry)
	var planStep flow.StepDefinition
	for _, entry := range registry.Entries {
		if entry.Kind == "step" {
			readRuntimeJSON(t, filepath.Join(e.Root, entry.Path), &planStep)
		}
	}
	step := planStep
	step.ID, step.SchemaVersion = "test:step/external", "11"
	step.Effects.Class, step.Effects.RetryClass = "external_write", "reconcile_required"
	step.ExternalWrite = &flow.ExternalWriteBoundary{
		System: "example-system", Operations: []string{"record.create"}, Target: "example/target",
	}
	step.WorkspaceTrees = nil
	shape(&step)
	bytes := writeRegistryDocument(t, e, "steps/external.json", step)
	ref := flow.Ref{ID: step.ID, Version: step.Version, Digest: rawDigest(bytes)}
	entries := append(append([]Definition{}, registry.Entries...), Definition{Ref: ref, Kind: "step", Path: "steps/external.json"})
	writeRuntimeJSON(t, filepath.Join(e.Root, e.Config.Configuration.RegistryFile), RegistryFile{SchemaVersion: registry.SchemaVersion, Entries: entries})

	workflow := flow.WorkflowRevision{
		SchemaVersion: "1", ID: "test:workflow/publishing", Version: "1.0.0", Title: "Publish inside the Run",
		Inputs: map[string]flow.InputPort{}, Outputs: map[string]flow.OutputPort{}, AllowedOutcomes: []string{"succeeded"},
		Limits: flow.Limits{MaxStepInstances: 2, MaxControlTransitions: 8, MaxParallelism: 1},
		// The edition that admits the class at all. Earlier ones do not, and the
		// policy is what gives the permission.
		PolicyRef: builtinVersionRef(definitions, "core:policy/local", "4.0.0"),
	}
	workflow.Definition.Entry = "publish"
	workflow.Definition.Stages = map[string]flow.Stage{
		"publish": {Kind: "step", StepRef: ref, InputBindings: map[string]flow.Binding{}, On: map[string]string{"pass": "done"}},
		"done":    {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]flow.Binding{}},
	}
	writeRuntimeJSON(t, filepath.Join(e.Root, "workflows/publishing.json"), workflow)
	result, err := e.Start(context.Background(), StartOptions{CommandID: newID("command"), WorkflowFile: "workflows/publishing.json", BriefFile: "brief.json", Inputs: map[string]string{}, WorkspaceMode: "checkout"})
	if err != nil {
		return e, "", err
	}
	return e, result.Receipt.RunID, nil
}

// The step compiles, starts, and its handoff carries the permission with the
// bounds the author declared. Without the bounds the permission would be
// "changes something outside", which no host can act under and no owner can
// review.
func TestAnAssistedStepPublishesUnderADeclaredBoundary(t *testing.T) {
	e, runID, err := externalWriteFixture(t, func(*flow.StepDefinition) {})
	if err != nil {
		t.Fatalf("a step declaring a bounded external write did not start: %v", err)
	}
	r := driverRun(t, e, runID)
	if r.SchemaVersion != CoreExternalWriteStateVersion {
		t.Fatalf("sealed at %s, not the state that records the boundary", r.SchemaVersion)
	}
	task := handOver(t, e, runID)
	if !slices.Contains(task.PermittedEffects, "change_declared_external_target") {
		t.Fatalf("the handoff does not permit the external change: %v", task.PermittedEffects)
	}
	if task.ExternalWrite == nil {
		t.Fatal("the handoff permits the change and does not say what may change")
	}
	if task.ExternalWrite.System != "example-system" || task.ExternalWrite.Target != "example/target" {
		t.Fatalf("the boundary reached the host altered: %+v", task.ExternalWrite)
	}
	if !slices.Contains(task.ExternalWrite.Operations, "record.create") {
		t.Fatalf("the permitted changes did not reach the host: %v", task.ExternalWrite.Operations)
	}
	// The step may not touch the claimed worktree: external_write is not a
	// workspace right, and git push does not need one.
	if slices.Contains(task.PermittedEffects, "write_inside_claimed_workspace") || slices.Contains(task.PermittedEffects, "write_inside_claimed_worktree") {
		t.Fatalf("an external write was given workspace rights: %v", task.PermittedEffects)
	}
}

// Three refusals, each for a reason an author can act on.
func TestADeclaredExternalWriteIsRefusedWithoutItsBounds(t *testing.T) {
	for _, test := range []struct {
		name, contains string
		shape          func(*flow.StepDefinition)
	}{
		{"no boundary at all", "system", func(s *flow.StepDefinition) { s.ExternalWrite = nil }},
		{"a retry that would write twice", "twice", func(s *flow.StepDefinition) { s.Effects.RetryClass = "idempotent" }},
		{"a boundary without the class", "external_write", func(s *flow.StepDefinition) {
			s.Effects.Class, s.Effects.RetryClass = "none", "never"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := externalWriteFixture(t, test.shape)
			if err == nil {
				t.Fatal("the declaration was admitted")
			}
			if !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("the refusal does not say what to change: %v", err)
			}
		})
	}
}

// The permission is the policy's to give. A Run pinned to an edition that does
// not admit the class does not get it, whatever the step declares -- the field
// named those classes from the first edition and bound nothing until now.
func TestAnEarlierPolicyEditionDoesNotAdmitAnExternalWrite(t *testing.T) {
	e, _, err := externalWriteFixture(t, func(s *flow.StepDefinition) {})
	if err != nil {
		t.Fatalf("the admitting edition refused: %v", err)
	}
	definitions, _, buildErr := Builtins()
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	var workflow flow.WorkflowRevision
	readRuntimeJSON(t, filepath.Join(e.Root, "workflows/publishing.json"), &workflow)
	workflow.PolicyRef = builtinVersionRef(definitions, "core:policy/local", "3.0.0")
	workflow.ID = "test:workflow/publishing-old-policy"
	writeRuntimeJSON(t, filepath.Join(e.Root, "workflows/publishing-old.json"), workflow)
	_, err = e.Start(context.Background(), StartOptions{CommandID: newID("command"), WorkflowFile: "workflows/publishing-old.json", BriefFile: "brief.json", Inputs: map[string]string{}, WorkspaceMode: "checkout"})
	if err == nil {
		t.Fatal("an edition that admits none and workspace_write admitted an external write")
	}
	if !strings.Contains(err.Error(), "admits") {
		t.Fatalf("the refusal does not name what the pinned edition allows: %v", err)
	}
}
