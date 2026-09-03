package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

func TestForkReuseRequiresCompletedExplicitOutput(t *testing.T) {
	ref := ArtifactRef{Digest: "sha256:output"}
	source := Run{Status: "completed", Outputs: map[string]ArtifactRef{"result": ref}}
	payload := ForkPayload{ReuseRefs: []ArtifactRef{ref}}
	if err := validateForkReuse(source, payload, map[string]ArtifactRef{"input": ref}); err != nil {
		t.Fatal(err)
	}
	if err := validateForkReuse(source, payload, map[string]ArtifactRef{}); err == nil {
		t.Fatal("hidden source output was accepted")
	}
	source.Status = "running"
	if err := validateForkReuse(source, payload, map[string]ArtifactRef{"input": ref}); err == nil {
		t.Fatal("output of unfinished source was accepted")
	}
}

func TestForkCreatesSeparateCoreRunWithProvenance(t *testing.T) {
	e, options := emptyRuntime(t)
	defs, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	e.Config.Configuration.SemanticsProfile = flow.CoreProfile
	e.Config.Configuration.SchemaVersion = CoreConfigVersion
	e.Config.ConfigurationSchemaRef = builtinRef(defs, "core:schema/core-configuration")
	workflow, err := flow.WorkflowJSONBytes(mustReadRuntime(t, filepath.Join(e.Root, options.WorkflowFile)), "json")
	if err != nil {
		t.Fatal(err)
	}
	digest, err := flow.Digest(workflow)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := e.localRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registry.Entries = append(registry.Entries, Definition{Ref: flow.Ref{ID: "test:workflow/empty", Version: "1.0.0", Digest: digest}, Kind: "workflow", Path: options.WorkflowFile})
	writeRuntimeJSON(t, filepath.Join(e.Root, e.Config.Configuration.RegistryFile), registry)
	writeRuntimeJSON(t, filepath.Join(e.Root, "prifly.json"), e.Config)
	if _, err := e.Start(context.Background(), options); err != nil {
		t.Fatal(err)
	}
	sourceID := startRunID(e.owner, options.CommandID)
	if err := e.Drive(context.Background(), sourceID); err != nil {
		t.Fatal(err)
	}
	source, view, err := e.load(context.Background(), sourceID)
	if err != nil || !source.terminal() {
		t.Fatalf("source did not complete: %+v %v", source, err)
	}
	command := ForkCommand{SchemaVersion: "1", CommandID: "command:fork", RunID: sourceID, ExpectedRunVersion: view.Snapshot.Version, Payload: ForkPayload{WorkflowRef: source.WorkflowRef, BriefRef: source.Brief, Inputs: map[string]ArtifactRef{}, ReuseRefs: []ArtifactRef{}, Reason: "owner changed the work scope"}}
	result, err := e.Fork(context.Background(), command)
	if err != nil || result.Receipt.Rejection != nil {
		t.Fatalf("fork: %+v %v", result, err)
	}
	createdID := startRunID(e.owner, command.CommandID)
	created, createdView, err := e.load(context.Background(), createdID)
	if err != nil || createdView.Snapshot.Version != 1 || created.SchemaVersion != CoreForkStateVersion || created.Fork == nil || created.Fork.SourceRunID != sourceID || created.Fork.SourceRunVersion != view.Snapshot.Version {
		t.Fatalf("forked run: %+v %+v %v", created, createdView, err)
	}
	stillSource, stillView, err := e.load(context.Background(), sourceID)
	if err != nil || stillView.Snapshot.Version != view.Snapshot.Version || stillSource.Fork != nil {
		t.Fatalf("fork rewrote source: %+v %+v %v", stillSource, stillView, err)
	}
}

func mustReadRuntime(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
