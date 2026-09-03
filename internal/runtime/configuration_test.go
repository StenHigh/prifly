package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

func configurationFixture(t *testing.T, scope string) (*Engine, StartOptions) {
	t.Helper()
	e, options := emptyRuntime(t)
	defs, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	e.Config.ConfigurationSchemaRef = builtinRef(defs, "core:schema/core-configuration")
	e.Config.Configuration.SchemaVersion = CoreConfigVersion
	e.Config.Configuration.SemanticsProfile = flow.CoreProfile
	ref, _ := artifactSchema(t, e, `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":["boolean","null"]}`)
	wb, err := os.ReadFile(filepath.Join(e.Root, options.WorkflowFile))
	if err != nil {
		t.Fatal(err)
	}
	var w flow.WorkflowRevision
	if err := json.Unmarshal(wb, &w); err != nil {
		t.Fatal(err)
	}
	w.SchemaVersion = "2"
	w.Inputs["value"] = flow.InputPort{Port: flow.Port{Format: "json", SchemaRef: &ref}, Required: true, Configuration: &flow.InputConfiguration{Scope: scope, Default: json.RawMessage(`false`)}}
	w.Outputs["value"] = flow.OutputPort{Port: w.Inputs["value"].Port, RequiredFor: []string{"no_work"}}
	w.Definition.Stages["done"].OutputBindings["value"] = flow.Binding{From: "workflow_input", Port: "value"}
	writeRuntimeJSON(t, filepath.Join(e.Root, options.WorkflowFile), w)
	writeRuntimeJSON(t, filepath.Join(e.Root, "prifly.json"), e.Config)
	return e, options
}

func TestBuiltinsReturnDetachedContracts(t *testing.T) {
	defs, registry, err := Builtins()
	if err != nil || len(defs) == 0 {
		t.Fatalf("builtins: %v", err)
	}
	ref, expectedDefinition := defs[0].Ref, bytes.Clone(defs[0].Bytes)
	expectedRegistry := bytes.Clone(registry[ref])
	defs[0].Bytes[0] ^= 1
	registry[ref][0] ^= 1
	delete(registry, ref)
	again, againRegistry, err := Builtins()
	if err != nil || !bytes.Equal(again[0].Bytes, expectedDefinition) || !bytes.Equal(againRegistry[ref], expectedRegistry) {
		t.Fatalf("builtin cache leaked caller mutation: %v", err)
	}
}

func TestCoreInputConfiguration(t *testing.T) {
	for _, tc := range []struct{ name, scope, project, run, source, value, refusal string }{
		{"default_false", "run", "", "", "package_default", "false", ""},
		{"project_null", "run", "null", "", "project", "null", ""},
		{"run_overrides", "run", "false", "true", "run", "true", ""},
		{"scope_blocks_run", "project", "false", "true", "", "", "configuration_scope"},
		{"project_type", "run", `"false"`, "", "", "", "schema"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, options := configurationFixture(t, tc.scope)
			if tc.project != "" {
				e.Config.Configuration.InputValues = map[string]map[string]json.RawMessage{"test:workflow/empty": {"value": json.RawMessage(tc.project)}}
			}
			if tc.run != "" {
				if err := os.WriteFile(filepath.Join(e.Root, "value.json"), []byte(tc.run), 0600); err != nil {
					t.Fatal(err)
				}
				options.Inputs["value"] = "value.json"
			}
			writeRuntimeJSON(t, filepath.Join(e.Root, "prifly.json"), e.Config)
			result, err := e.Start(context.Background(), options)
			if tc.refusal != "" {
				if err == nil || !strings.Contains(err.Error(), tc.refusal) {
					t.Fatalf("want %s: %v", tc.refusal, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			r := driverRun(t, e, result.Receipt.RunID)
			value := r.EffectiveConfiguration.Inputs["value"]
			if value.Source != tc.source || string(value.Value) != tc.value {
				t.Fatalf("configuration: %+v", value)
			}
			_, data, err := e.Artifact(r.Inputs["value"])
			if err != nil || string(data) != tc.value {
				t.Fatalf("materialized input: %s %v", data, err)
			}
			// Changing the project's default does not change an accepted Run.
			e.Config.Configuration.SemanticsProfile = flow.Profile
			e.Config.Configuration.InputValues = nil
			writeRuntimeJSON(t, filepath.Join(e.Root, "prifly.json"), e.Config)
			reopened, err := Open(e.Root, false)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			if err := reopened.Drive(context.Background(), r.ID); err != nil {
				t.Fatal(err)
			}
			view, err := reopened.View(context.Background(), r.ID)
			if err != nil || view.SchemaVersion != CoreReadVersion || view.Run.Status != "completed" {
				t.Fatalf("pinned profile not retained: %+v %v", view, err)
			}
			if view.Run.Outputs["value"] != r.Inputs["value"] {
				t.Fatal("configuration input was replaced during resume")
			}
			before, _ := canonical(r.EffectiveConfiguration)
			after, _ := canonical(view.Run.EffectiveConfiguration)
			if !bytes.Equal(before, after) {
				t.Fatal("effective configuration changed")
			}
		})
	}
	t.Run("unknown_project_parameter", func(t *testing.T) {
		e, options := configurationFixture(t, "run")
		e.Config.Configuration.InputValues = map[string]map[string]json.RawMessage{"test:workflow/empty": {"executable": json.RawMessage(`"/bin/sh"`)}}
		if _, err := e.Start(context.Background(), options); err == nil || !strings.Contains(err.Error(), "unknown_configuration_input") {
			t.Fatalf("unknown parameter: %v", err)
		}
	})
	t.Run("preview_is_read_only", func(t *testing.T) {
		e, options := configurationFixture(t, "run")
		_, before, err := e.Store.ReadAll(context.Background(), 100)
		if err != nil {
			t.Fatal(err)
		}
		preview, err := e.Preview(PreviewOptions{WorkflowFile: options.WorkflowFile, InputRefs: map[string]ArtifactRef{}})
		if err != nil || preview.EffectiveConfiguration.Inputs["value"].Source != "package_default" {
			t.Fatalf("preview: %+v %v", preview, err)
		}
		_, after, err := e.Store.ReadAll(context.Background(), 100)
		if err != nil || before != after {
			t.Fatalf("preview mutated history: %d %d %v", before, after, err)
		}
	})
}

func TestCoreProjectionSealsDistinctArtifact(t *testing.T) {
	for _, value := range []string{`{"value":null}`, `{"value":true}`, `{}`, `{"value":"invalid"}`} {
		t.Run(value, func(t *testing.T) {
			e, options := configurationFixture(t, "run")
			wb, err := os.ReadFile(filepath.Join(e.Root, options.WorkflowFile))
			if err != nil {
				t.Fatal(err)
			}
			var w flow.WorkflowRevision
			if err := json.Unmarshal(wb, &w); err != nil {
				t.Fatal(err)
			}
			projected := w.Outputs["value"].SchemaRef
			schema := []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"value":{"type":["boolean","null","string"]}},"additionalProperties":false}`)
			digest, _ := flow.Digest(schema)
			source := flow.Ref{ID: "test:schema/document", Version: "1.0.0", Digest: digest}
			if err := os.WriteFile(filepath.Join(e.Root, "schemas/document.json"), schema, 0600); err != nil {
				t.Fatal(err)
			}
			writeRuntimeJSON(t, filepath.Join(e.Root, "definitions.json"), RegistryFile{SchemaVersion: "1", Entries: []Definition{{Ref: *projected, Kind: "schema", Path: "schemas/artifact.json"}, {Ref: source, Kind: "schema", Path: "schemas/document.json"}}})
			w.Inputs = map[string]flow.InputPort{"document": {Port: flow.Port{Format: "json", SchemaRef: &source}, Required: true}}
			output := w.Outputs["value"]
			output.RequiredFor = []string{}
			w.Outputs["value"] = output
			pointer := "/value"
			w.Definition.Stages["done"].OutputBindings["value"] = flow.Binding{From: "workflow_input", Port: "document", Pointer: &pointer, ProjectedSchemaRef: projected}
			writeRuntimeJSON(t, filepath.Join(e.Root, options.WorkflowFile), w)
			if err := os.WriteFile(filepath.Join(e.Root, "document.json"), []byte(value), 0600); err != nil {
				t.Fatal(err)
			}
			options.Inputs = map[string]string{"document": "document.json"}
			result, err := e.Start(context.Background(), options)
			if err != nil {
				t.Fatal(err)
			}
			if err := e.Drive(context.Background(), result.Receipt.RunID); err != nil {
				t.Fatal(err)
			}
			r := driverRun(t, e, result.Receipt.RunID)
			if value == `{"value":"invalid"}` {
				activation := activationFor(&r, "done")
				if r.Status != "failed" || r.Settled == nil || r.Outcome != nil || len(r.Ready) != 0 || len(r.Outputs) != 0 || len(r.Steps) != 0 || len(r.Attempts) != 0 || activation == nil || activation.Status != "failed" || activation.Settled == nil {
					t.Fatalf("invalid finish projection did not settle without a worker: %+v", r)
				}
				if len(r.Diagnostics) != 1 || r.Diagnostics[0].ActivationID != activation.ID || r.Diagnostics[0].AttemptID != "" || r.Diagnostics[0].Code != "output_binding_failed" {
					t.Fatalf("invalid finish has no stage diagnostic: %+v", r.Diagnostics)
				}
				before, err := e.View(context.Background(), r.ID)
				if err != nil {
					t.Fatal(err)
				}
				if err := e.Close(); err != nil {
					t.Fatal(err)
				}
				reopened, err := Open(e.Root, false)
				if err != nil {
					t.Fatal(err)
				}
				defer reopened.Close()
				if err := reopened.Drive(context.Background(), r.ID); err != nil {
					t.Fatal(err)
				}
				after, err := reopened.View(context.Background(), r.ID)
				if err != nil || after.Run.Status != "failed" || after.RunVersion != before.RunVersion || after.EventSequence != before.EventSequence {
					t.Fatalf("failed finish was reevaluated after restart: %+v %v", after, err)
				}
				return
			}
			if r.Status != "completed" {
				t.Fatalf("valid finish did not complete: %+v", r)
			}
			outputRef, present := r.Outputs["value"]
			if value == `{}` {
				if present {
					t.Fatal("absent projection became a value")
				}
				return
			}
			if !present || outputRef == r.Inputs["document"] {
				t.Fatal("projection reused whole-object identity")
			}
			artifact, data, err := e.Artifact(outputRef)
			var document map[string]json.RawMessage
			if err := json.Unmarshal([]byte(value), &document); err != nil {
				t.Fatal(err)
			}
			if err != nil || !bytes.Equal(data, document["value"]) || artifact.SchemaRef == nil || *artifact.SchemaRef != *projected || len(artifact.Provenance) != 2 || artifact.Provenance[0] != r.Inputs["document"] {
				t.Fatalf("bad derived artifact: %+v %s %v", artifact, data, err)
			}
			_, manifestBytes, err := e.Artifact(artifact.Provenance[1])
			if err != nil {
				t.Fatal(err)
			}
			var manifest ProjectionManifest
			if err := decode(manifestBytes, &manifest); err != nil {
				t.Fatal(err)
			}
			if manifest.Source != r.Inputs["document"] || manifest.Pointer != "/value" || manifest.ProjectedSchemaRef != *projected || manifest.WorkflowRef != r.WorkflowRef {
				t.Fatalf("projection provenance: %+v", manifest)
			}
		})
	}
}
