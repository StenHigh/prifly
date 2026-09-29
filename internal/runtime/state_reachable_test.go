package runtime

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

// Two state versions were declared for facts a Run records, published as
// bundles and listed in the capability document -- and nothing ever sealed a
// Run with either. The ladder in start stopped at 32, so a Run carrying a
// sealed translation was stamped 31, and bundle 31 rejected that same Run.
//
// Three checks stood over those two releases and all three read the
// declaration: the ladder has the row, the bundle exists, the capability is
// listed. None asked what a Run was actually stamped with. This one reads the
// Run.
//
// Since 41 every project Run with an assisted step is sealed there, and a model
// profile is only ever declared by an assisted step: 34 and 35 are now what
// Runs sealed before 41 carry. What stays checked is that the Run carrying
// either fact is sealed at a version whose bundle has a place for it.
func TestAStateVersionDeclaredForARecordedFactIsReached(t *testing.T) {
	e, _, _ := assistedWorkspaceFixture(t, "checkout")
	ctx := context.Background()
	var registry RegistryFile
	readRuntimeJSON(t, filepath.Join(e.Root, e.Config.Configuration.RegistryFile), &registry)
	definitions, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	var planStep flow.StepDefinition
	var planEntry Definition
	for _, entry := range registry.Entries {
		if entry.Kind == "step" {
			readRuntimeJSON(t, filepath.Join(e.Root, entry.Path), &planStep)
			planEntry = entry
		}
	}
	workflow := flow.WorkflowRevision{
		SchemaVersion: "1", ID: "aif:workflow/state-reach", Version: "1.0.0", Title: "One assisted step",
		Inputs: map[string]flow.InputPort{}, Outputs: map[string]flow.OutputPort{}, AllowedOutcomes: []string{"succeeded"},
		Limits: flow.Limits{MaxStepInstances: 2, MaxControlTransitions: 8, MaxParallelism: 1}, PolicyRef: builtinVersionRef(definitions, "core:policy/local", "2.0.0"),
	}
	workflow.Definition.Entry = "plan"
	workflow.Definition.Stages = map[string]flow.Stage{
		"plan": {Kind: "step", StepRef: planEntry.Ref, InputBindings: map[string]flow.Binding{}, On: map[string]string{"pass": "done"}},
		"done": {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]flow.Binding{}},
	}
	start := func(t *testing.T, profiles map[string]ModelProfileTranslation) Run {
		t.Helper()
		writeRuntimeJSON(t, filepath.Join(e.Root, "workflows/state-reach.json"), workflow)
		result, err := e.Start(ctx, StartOptions{CommandID: newID("command"), WorkflowFile: "workflows/state-reach.json", BriefFile: "brief.json", Inputs: map[string]string{}, WorkspaceMode: "checkout", ModelProfiles: profiles})
		if err != nil {
			t.Fatal(err)
		}
		return driverRun(t, e, result.Receipt.RunID)
	}

	t.Run("a declared profile reaches its own state", func(t *testing.T) {
		declared := planStep
		declared.ID, declared.SchemaVersion = "aif:step/plan-profiled", "9"
		declared.ModelProfile = &flow.ModelProfile{Requested: "careful-review", Reason: "this step reads more than it writes"}
		declaredBytes := writeRegistryDocument(t, e, "steps/plan-profiled.json", declared)
		ref := flow.Ref{ID: declared.ID, Version: declared.Version, Digest: rawDigest(declaredBytes)}
		entries := append([]Definition{}, registry.Entries...)
		entries = append(entries, Definition{Ref: ref, Kind: "step", Path: "steps/plan-profiled.json"})
		writeRuntimeJSON(t, filepath.Join(e.Root, e.Config.Configuration.RegistryFile), RegistryFile{SchemaVersion: registry.SchemaVersion, Entries: entries})
		t.Cleanup(func() {
			writeRuntimeJSON(t, filepath.Join(e.Root, e.Config.Configuration.RegistryFile), registry)
		})
		profiled := workflow
		profiled.ID = "aif:workflow/state-reach-profiled"
		profiled.Definition.Stages = map[string]flow.Stage{
			"plan": {Kind: "step", StepRef: ref, InputBindings: map[string]flow.Binding{}, On: map[string]string{"pass": "done"}},
			"done": workflow.Definition.Stages["done"],
		}
		writeRuntimeJSON(t, filepath.Join(e.Root, "workflows/state-reach-profiled.json"), profiled)
		result, err := e.Start(ctx, StartOptions{CommandID: newID("command"), WorkflowFile: "workflows/state-reach-profiled.json", BriefFile: "brief.json", Inputs: map[string]string{}, WorkspaceMode: "checkout"})
		if err != nil {
			t.Fatal(err)
		}
		r := driverRun(t, e, result.Receipt.RunID)
		if r.SchemaVersion != CoreQuestionStateVersion {
			t.Fatalf("a Run whose step declares a model profile is sealed at %s, not %s", r.SchemaVersion, CoreQuestionStateVersion)
		}
		if err := validateInBundle(t, questionPublicContracts, "CoreRunStateV41", r); err != nil {
			t.Fatalf("the bundle of the version this Run names rejects it: %v", err)
		}
	})

	t.Run("a sealed translation reaches its own state", func(t *testing.T) {
		r := start(t, map[string]ModelProfileTranslation{"careful-review": {Values: map[string]string{"model": "opus"}, Source: "package"}})
		if r.SchemaVersion != CoreQuestionStateVersion {
			t.Fatalf("a Run sealing a profile translation is sealed at %s, not %s", r.SchemaVersion, CoreQuestionStateVersion)
		}
		if len(r.ModelProfileTranslations) == 0 {
			t.Fatal("the fixture sealed no translation, so this proves nothing about the version that carries one")
		}
		if err := validateInBundle(t, questionPublicContracts, "CoreRunStateV41", r); err != nil {
			t.Fatalf("the bundle of the version this Run names rejects it: %v", err)
		}
	})

	t.Run("an assisted project Run records its questions", func(t *testing.T) {
		r := start(t, nil)
		if r.SchemaVersion != CoreQuestionStateVersion {
			t.Fatalf("a project Run with an assisted step is sealed at %s, not %s", r.SchemaVersion, CoreQuestionStateVersion)
		}
		if err := validateInBundle(t, questionPublicContracts, "CoreRunStateV41", r); err != nil {
			t.Fatalf("the bundle of the version this Run names rejects it: %v", err)
		}
	})
}
