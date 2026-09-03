package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// Author both optional ports before Start. The same exact workflow is used
// with no snapshot, a descriptor alone, and explicitly bound content. Nothing
// in this fixture requires a repository, task system, or model adapter.
func sourceRuntimeProject(t *testing.T) (*Engine, StartOptions) {
	t.Helper()
	e, options := contextDriverProject(t, nil)
	definitions, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	schema := builtinRef(definitions, sourceSnapshotSchemaID)
	var step flow.StepDefinition
	data, err := os.ReadFile(filepath.Join(e.Root, "steps/context.json"))
	if err != nil || json.Unmarshal(data, &step) != nil {
		t.Fatal("read authored step", err)
	}
	step.Inputs = map[string]flow.InputPort{
		"snapshot": {Port: flow.Port{Format: "json", SchemaRef: &schema}},
		"content":  {Port: flow.Port{Format: "blob", MediaTypes: []string{"text/plain"}}},
	}
	data, err = canonical(step)
	if err != nil {
		t.Fatal(err)
	}
	stepRef := flow.Ref{ID: step.ID, Version: step.Version, Digest: rawDigest(data)}
	sourceTestFile(t, e, "steps/context.json", data)
	var registry RegistryFile
	data, err = os.ReadFile(filepath.Join(e.Root, "definitions.json"))
	if err != nil || json.Unmarshal(data, &registry) != nil {
		t.Fatal("read authored registry", err)
	}
	for i := range registry.Entries {
		if registry.Entries[i].Ref.ID == step.ID {
			registry.Entries[i].Ref = stepRef
		}
	}
	writeRuntimeJSON(t, filepath.Join(e.Root, "definitions.json"), registry)
	var workflow flow.WorkflowRevision
	data, err = os.ReadFile(filepath.Join(e.Root, options.WorkflowFile))
	if err != nil || json.Unmarshal(data, &workflow) != nil {
		t.Fatal("read authored workflow", err)
	}
	workflow.Inputs = step.Inputs
	stage := workflow.Definition.Stages["work"]
	stage.StepRef = stepRef
	stage.InputBindings = map[string]flow.Binding{
		"snapshot": {From: "workflow_input", Port: "snapshot"},
		"content":  {From: "workflow_input", Port: "content"},
	}
	workflow.Definition.Stages["work"] = stage
	writeRuntimeJSON(t, filepath.Join(e.Root, options.WorkflowFile), workflow)
	executor := e.Config.Configuration.Executors[step.ID]
	executor.Args = []string{"-test.run=^TestSourceRuntimeWorkerHelper$"}
	executor.Environment = map[string]string{"SOURCE_RUNTIME_HELPER": "1", "GORACE": "atexit_sleep_ms=0"}
	executor.Files = map[string]string{}
	e.Config.Configuration.Executors[step.ID] = executor
	writeRuntimeJSON(t, filepath.Join(e.Root, "prifly.json"), e.Config)
	root := e.Root
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	e, err = Open(root, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close() })
	options.Inputs, options.InputRefs = nil, map[string]ArtifactRef{}
	return e, options
}

// This is the actual subprocess selected by the authored executor. Its result
// echoes only files named by the supplied input ports; []byte JSON values keep
// CRLF, UTF-8 and all other input bytes observable without normalization.
func TestSourceRuntimeWorkerHelper(t *testing.T) {
	if os.Getenv("SOURCE_RUNTIME_HELPER") != "1" {
		return
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		RunID     string `json:"run_id"`
		StepID    string `json:"step_instance_id"`
		AttemptID string `json:"attempt_id"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(os.Getenv("PRIFLY_CONTEXT_FILE"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest ContextManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	inputs := map[string][]byte{}
	for name, slot := range manifest.Inputs {
		data, err := os.ReadFile(slot.Path)
		if err != nil || rawDigest(data) != slot.Ref.Digest {
			t.Fatal("worker did not receive exact input", name, err)
		}
		inputs[name] = data
	}
	data, err = json.Marshal(inputs)
	if err != nil {
		t.Fatal(err)
	}
	slot := manifest.Outputs["report"]
	if err := os.WriteFile(slot.Path, data, 0600); err != nil {
		t.Fatal(err)
	}
	result := Result{
		SchemaVersion: "1", RunID: envelope.RunID, StepInstanceID: envelope.StepID,
		AttemptID: envelope.AttemptID, EnvelopeDigest: os.Getenv("PRIFLY_ENVELOPE_DIGEST"), Verdict: "pass",
		Outputs:      map[string]ArtifactRef{"report": {ArtifactID: slot.ArtifactID, Revision: slot.Revision, Digest: rawDigest(data)}},
		EvidenceRefs: []any{}, EffectReceiptRefs: []any{}, Summary: "Explicit source input consumption",
	}
	fd := os.NewFile(3, "result")
	if err := json.NewEncoder(fd).Encode(result); err != nil {
		t.Fatal(err)
	}
	if err := fd.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestSourceRuntimeOptionalSnapshotDoesNotExpandContext(t *testing.T) {
	e, options := sourceRuntimeProject(t)
	content := []byte("sealed-source-only: Пример \"literal\"\r\n")
	sourceTestFile(t, e, "selected-source.txt", content)
	artifact, snapshot := sourceTestImport(t, e, SourceImportOptions{Path: "selected-source.txt", Format: "blob", MediaType: "text/plain"})
	_, descriptor, err := e.Artifact(artifact.Ref())
	if err != nil {
		t.Fatal(err)
	}
	var workflowRef flow.Ref
	for _, tc := range []struct {
		name   string
		refs   map[string]ArtifactRef
		inputs map[string][]byte
	}{
		{"absent", map[string]ArtifactRef{}, map[string][]byte{}},
		{"descriptor_only", map[string]ArtifactRef{"snapshot": artifact.Ref()}, map[string][]byte{"snapshot": descriptor}},
		{"explicit_content", map[string]ArtifactRef{"snapshot": artifact.Ref(), "content": snapshot.ContentRef}, map[string][]byte{"snapshot": descriptor, "content": content}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			options.CommandID, options.InputRefs = newID("command"), tc.refs
			preview, err := e.Preview(PreviewOptions{WorkflowFile: options.WorkflowFile, BriefFile: options.BriefFile, InputRefs: tc.refs})
			if err != nil || preview.Validation.Inputs != "sealed_refs_verified" {
				t.Fatal("explicit optional inputs were not previewable", err)
			}
			started, err := e.Start(context.Background(), options)
			if err != nil {
				t.Fatal(err)
			}
			// Each edit happens after Start. Neither the descriptor nor the
			// separately requested content may reread this acquisition path.
			if tc.name == "descriptor_only" {
				sourceTestFile(t, e, "selected-source.txt", []byte("changed live source"))
			}
			if tc.name == "explicit_content" {
				if err := os.Remove(filepath.Join(e.Root, "selected-source.txt")); err != nil {
					t.Fatal(err)
				}
			}
			if err := e.Drive(context.Background(), started.Receipt.RunID); err != nil {
				t.Fatal(err)
			}
			r := driverRun(t, e, started.Receipt.RunID)
			if r.Status != "completed" || len(r.Attempts) != 1 || !reflect.DeepEqual(r.Inputs, tc.refs) {
				t.Fatalf("optional source changed execution: status=%s attempts=%d diagnostics=%+v", r.Status, len(r.Attempts), r.Diagnostics)
			}
			if workflowRef == (flow.Ref{}) {
				workflowRef = r.WorkflowRef
			}
			if r.WorkflowRef != workflowRef || preview.WorkflowRef != workflowRef {
				t.Fatal("optional source required a different workflow revision")
			}
			_, report, err := e.Artifact(r.Outputs["report"])
			var consumed map[string][]byte
			if err != nil || json.Unmarshal(report, &consumed) != nil || !reflect.DeepEqual(consumed, tc.inputs) {
				t.Fatalf("native worker consumed different inputs: %s, error=%v", report, err)
			}
			for _, attempt := range r.Attempts {
				if attempt.Started == nil || attempt.Process == nil || attempt.Accepted == nil || attempt.ProcessOutcome == nil || !attempt.ProcessOutcome.WaitReturned || !attempt.ProcessOutcome.GroupEmpty {
					t.Fatal("input consumption did not come from a settled native worker")
				}
				if len(attempt.Context.Sources) != 3+len(tc.refs) || len(attempt.Context.Inputs) != len(tc.refs) {
					t.Fatal("descriptor added undeclared context copies")
				}
				if _, bound := tc.refs["content"]; !bound {
					for _, source := range attempt.Context.Sources {
						if source.Ref == snapshot.ContentRef {
							t.Fatal("descriptor implicitly disclosed source content")
						}
					}
					rendered, err := os.ReadFile(filepath.Join(attempt.Workspace, attempt.Context.Rendering.Path))
					if err != nil || bytes.Contains(rendered, []byte("sealed-source-only")) {
						t.Fatal("rendering implicitly disclosed unbound source bytes", err)
					}
				}
			}
			retained, err := e.SourceSnapshot(artifact.Ref())
			if err != nil || retained != snapshot {
				t.Fatal("reuse changed acquisition provenance or consulted live bytes", err)
			}
		})
	}
}

func TestSourceRuntimeRejectsShapeOnlyDescriptors(t *testing.T) {
	e, options := sourceRuntimeProject(t)
	sourceTestFile(t, e, "selected-source.txt", []byte("only an adapter import can assert this acquisition"))
	artifact, _ := sourceTestImport(t, e, SourceImportOptions{Path: "selected-source.txt", Format: "blob", MediaType: "text/plain"})
	_, descriptor, err := e.Artifact(artifact.Ref())
	if err != nil {
		t.Fatal(err)
	}
	sourceTestFile(t, e, "shape-only.json", descriptor)
	ordinary, err := e.ImportArtifact("shape-only.json", "json", artifact.SchemaRef)
	if err != nil {
		t.Fatal("the control descriptor must be shape-valid JSON", err)
	}
	if _, err := ParseSourceSnapshot(descriptor); err != nil {
		t.Fatal(err)
	}
	_, err = e.SourceSnapshot(ordinary.Ref())
	contextErrorCode(t, err, "source_snapshot_invalid")
	before, cut, err := e.Store.ReadAll(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	unchanged := func(t *testing.T) {
		t.Helper()
		after, afterCut, err := e.Store.ReadAll(context.Background(), 100)
		if err != nil || cut != afterCut || !reflect.DeepEqual(before, after) {
			t.Fatal("shape-only descriptor admitted or changed a Run", err)
		}
		entries, err := os.ReadDir(filepath.Join(e.Root, e.Config.Configuration.WorkspaceRoot))
		if err != nil || len(entries) != 0 {
			t.Fatal("shape-only descriptor reached a worker workspace", err)
		}
	}
	t.Run("preview_reference", func(t *testing.T) {
		_, err := e.Preview(PreviewOptions{WorkflowFile: options.WorkflowFile, BriefFile: options.BriefFile, InputRefs: map[string]ArtifactRef{"snapshot": ordinary.Ref()}})
		contextErrorCode(t, err, "source_snapshot_invalid")
		unchanged(t)
	})
	t.Run("start_reference", func(t *testing.T) {
		options.CommandID, options.InputRefs = newID("command"), map[string]ArtifactRef{"snapshot": ordinary.Ref()}
		_, err := e.Start(context.Background(), options)
		contextErrorCode(t, err, "source_snapshot_invalid")
		unchanged(t)
	})
	t.Run("start_file", func(t *testing.T) {
		options.CommandID, options.InputRefs, options.Inputs = newID("command"), nil, map[string]string{"snapshot": "shape-only.json"}
		_, err := e.Start(context.Background(), options)
		contextErrorCode(t, err, "source_snapshot_invalid")
		unchanged(t)
	})
	t.Run("start_default", func(t *testing.T) {
		var workflow flow.WorkflowRevision
		data, err := os.ReadFile(filepath.Join(e.Root, options.WorkflowFile))
		if err != nil || json.Unmarshal(data, &workflow) != nil {
			t.Fatal("read authored workflow", err)
		}
		workflow.SchemaVersion = "2"
		port := workflow.Inputs["snapshot"]
		port.Configuration = &flow.InputConfiguration{Scope: "run", Default: descriptor}
		workflow.Inputs["snapshot"] = port
		writeRuntimeJSON(t, filepath.Join(e.Root, options.WorkflowFile), workflow)
		options.CommandID, options.InputRefs, options.Inputs = newID("command"), nil, nil
		metadataBefore, err := os.ReadDir(filepath.Join(e.Root, ".prifly/artifact-refs"))
		if err != nil {
			t.Fatal(err)
		}
		_, err = e.Preview(PreviewOptions{WorkflowFile: options.WorkflowFile, BriefFile: options.BriefFile, InputRefs: map[string]ArtifactRef{}})
		var rejection *local.Rejection
		if !errors.As(err, &rejection) || rejection.Code != "source_snapshot_invalid" {
			t.Fatal("preview treated a default as an acquired source", err)
		}
		unchanged(t)
		_, err = e.Start(context.Background(), options)
		if !errors.As(err, &rejection) || rejection.Code != "source_snapshot_invalid" {
			t.Fatal("start treated a default as an acquired source", err)
		}
		unchanged(t)
		metadataAfter, err := os.ReadDir(filepath.Join(e.Root, ".prifly/artifact-refs"))
		if err != nil || !reflect.DeepEqual(metadataBefore, metadataAfter) {
			t.Fatal("default was sealed as a new purported acquisition before refusal", err)
		}
	})
}

func TestSourceRuntimeProjectionCannotReassertImport(t *testing.T) {
	e, options := sourceRuntimeProject(t)
	sourceTestFile(t, e, "selected-source.txt", []byte("sealed content"))
	artifact, expected := sourceTestImport(t, e, SourceImportOptions{Path: "selected-source.txt", Format: "blob", MediaType: "text/plain"})
	var workflow flow.WorkflowRevision
	data, err := os.ReadFile(filepath.Join(e.Root, options.WorkflowFile))
	if err != nil || json.Unmarshal(data, &workflow) != nil {
		t.Fatal("read authored workflow", err)
	}
	// An identity projection is shape-valid and retains the content_ref, but
	// its authority-produced artifact is not an acquisition by the adapter.
	pointer := ""
	workflow.Definition.Stages["work"].InputBindings["snapshot"] = flow.Binding{
		From: "workflow_input", Port: "snapshot", Pointer: &pointer, ProjectedSchemaRef: artifact.SchemaRef,
	}
	writeRuntimeJSON(t, filepath.Join(e.Root, options.WorkflowFile), workflow)
	options.InputRefs = map[string]ArtifactRef{"snapshot": artifact.Ref()}
	started, err := e.Start(context.Background(), options)
	if err != nil {
		t.Fatal("the genuine workflow input should be accepted", err)
	}
	if err := e.Drive(context.Background(), started.Receipt.RunID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, started.Receipt.RunID)
	if r.Status != "failed" || len(r.Attempts) != 0 || len(r.Diagnostics) != 1 || r.Diagnostics[0].Code != "source_snapshot_invalid" || r.Diagnostics[0].Phase != "preparation" {
		t.Fatalf("projection asserted a false import: status=%s attempts=%d diagnostics=%+v", r.Status, len(r.Attempts), r.Diagnostics)
	}
	if retained, err := e.SourceSnapshot(artifact.Ref()); err != nil || retained != expected {
		t.Fatal("projection failure damaged the genuine acquisition", err)
	}
	entries, err := os.ReadDir(filepath.Join(e.Root, e.Config.Configuration.WorkspaceRoot))
	if err != nil || len(entries) != 0 {
		t.Fatal("invalid projected source reached a worker workspace", err)
	}
}

func TestSourceRuntimeLostSealedContentFailsBeforeWorker(t *testing.T) {
	for _, failure := range []string{"missing", "corrupt"} {
		t.Run(failure, func(t *testing.T) {
			e, options := sourceRuntimeProject(t)
			live := []byte("live source still contains the original bytes")
			sourceTestFile(t, e, "selected-source.txt", live)
			artifact, snapshot := sourceTestImport(t, e, SourceImportOptions{Path: "selected-source.txt", Format: "blob", MediaType: "text/plain"})
			options.InputRefs = map[string]ArtifactRef{"snapshot": artifact.Ref(), "content": snapshot.ContentRef}
			started, err := e.Start(context.Background(), options)
			if err != nil {
				t.Fatal(err)
			}
			blob := filepath.Join(e.Root, e.Config.Configuration.ArtifactRoot, strings.TrimPrefix(snapshot.ContentRef.Digest, "sha256:"))
			if failure == "missing" {
				if err := os.Remove(blob); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Chmod(blob, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(blob, []byte("corrupt"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := e.SourceSnapshot(artifact.Ref()); !errors.Is(err, local.ErrIntegrity) {
				t.Fatal("lost accepted source was not an integrity refusal", err)
			}
			if err := e.Drive(context.Background(), started.Receipt.RunID); err != nil {
				t.Fatal(err)
			}
			r := driverRun(t, e, started.Receipt.RunID)
			if r.Status != "failed" || len(r.Attempts) != 0 || len(r.Outputs) != 0 || len(r.Diagnostics) != 1 || r.Diagnostics[0].Phase != "preparation" {
				t.Fatalf("lost source reached execution: status=%s attempts=%d diagnostics=%+v", r.Status, len(r.Attempts), r.Diagnostics)
			}
			entries, err := os.ReadDir(filepath.Join(e.Root, e.Config.Configuration.WorkspaceRoot))
			if err != nil || len(entries) != 0 {
				t.Fatal("lost content was materialized for a worker", err)
			}
			if failure == "missing" {
				if _, err := os.Stat(blob); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("dispatch repaired missing content from its live path", err)
				}
			} else if data, err := os.ReadFile(blob); err != nil || string(data) != "corrupt" {
				t.Fatal("dispatch repaired corrupt content", err)
			}
			if data, err := os.ReadFile(filepath.Join(e.Root, "selected-source.txt")); err != nil || !bytes.Equal(data, live) {
				t.Fatal("fixture no longer offers a live source to detect repair", err)
			}
		})
	}
}

func TestSourceRuntimeReadOnlyPreservesAcquisitionAndAdmission(t *testing.T) {
	e, options := sourceRuntimeProject(t)
	sourceTestFile(t, e, "selected-source.txt", []byte("retained source"))
	artifact, snapshot := sourceTestImport(t, e, SourceImportOptions{Path: "selected-source.txt", Format: "blob", MediaType: "text/plain"})
	options.InputRefs = map[string]ArtifactRef{"snapshot": artifact.Ref(), "content": snapshot.ContentRef}
	started, err := e.Start(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(e.Root, "selected-source.txt")); err != nil {
		t.Fatal(err)
	}
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	reader, err := Open(e.Root, true)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	before, cut, err := reader.Store.ReadAll(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	metadataBefore, err := os.ReadDir(filepath.Join(reader.Root, ".prifly/artifact-refs"))
	if err != nil {
		t.Fatal(err)
	}
	if retained, err := reader.SourceSnapshot(artifact.Ref()); err != nil || retained != snapshot {
		t.Fatal("read-only reuse changed the acquisition", err)
	}
	if preview, err := reader.Preview(PreviewOptions{WorkflowFile: options.WorkflowFile, BriefFile: options.BriefFile, InputRefs: options.InputRefs}); err != nil || preview.Validation.Inputs != "sealed_refs_verified" {
		t.Fatal("read-only preview requires a new acquisition", err)
	}
	if _, err := reader.ImportSource(SourceImportOptions{Path: "selected-source.txt", Format: "blob", MediaType: "text/plain"}); !errors.Is(err, local.ErrReadOnly) {
		t.Fatal("source import did not refuse before live file lookup", err)
	}
	options.CommandID = newID("command")
	if _, err := reader.Start(context.Background(), options); !errors.Is(err, local.ErrReadOnly) {
		t.Fatal("read-only source reuse admitted a Run", err)
	}
	if err := reader.Drive(context.Background(), started.Receipt.RunID); !errors.Is(err, local.ErrReadOnly) {
		t.Fatal("read-only source reuse dispatched a worker", err)
	}
	after, afterCut, err := reader.Store.ReadAll(context.Background(), 100)
	if err != nil || cut != afterCut || !reflect.DeepEqual(before, after) {
		t.Fatal("read-only source operations changed saved execution", err)
	}
	metadataAfter, err := os.ReadDir(filepath.Join(reader.Root, ".prifly/artifact-refs"))
	if err != nil || !reflect.DeepEqual(metadataBefore, metadataAfter) {
		t.Fatal("read-only preview or reuse published new artifact metadata", err)
	}
	r := driverRun(t, reader, started.Receipt.RunID)
	if len(r.Attempts) != 0 {
		t.Fatal("read-only operations created an Attempt")
	}
}
