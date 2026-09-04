package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

func TestNeutralStartWithoutBriefExecutesAfterRestart(t *testing.T) {
	e, options := contextDriverProject(t, func(profile *ContextProfile) { profile.IncludeBrief = true })
	ctx := context.Background()
	legacy, err := e.Start(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	legacyState, err := e.Store.Read(ctx, legacy.Receipt.RunID, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	options.SchemaVersion, options.BriefFile, options.CommandID = "2", "", newID("command")
	started, err := e.Start(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, started.Receipt.RunID)
	if r.SchemaVersion != CoreNeutralStateVersion || r.Brief != (ArtifactRef{}) || hasDecisionStateFields(r) {
		t.Fatalf("neutral Run manufactured task or decision data: %s %+v", r.SchemaVersion, r.Brief)
	}
	state := contextContractObject(t, r)
	if _, exists := state["brief_ref"]; exists {
		t.Fatal("absence must be omitted, not a zero or null ArtifactRef")
	}
	briefID := derivedID("artifact", e.owner, options.CommandID, "brief")
	if _, err := os.Stat(filepath.Join(e.Root, artifactMetadataPath(briefID))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("no-brief start wrote task artifact: %v", err)
	}
	if err := validatePublic(t, "CoreRunStateV26", r); err != nil {
		t.Fatal(err)
	}
	if validatePublic(t, "CoreRunStateV25", r) == nil {
		t.Fatal("previous contract accepted a version it cannot read")
	}
	preview, err := e.Preview(PreviewOptions{SchemaVersion: "2", WorkflowFile: options.WorkflowFile})
	if err != nil || preview.SchemaVersion != CoreNeutralPreviewVersion || preview.Brief != nil {
		t.Fatalf("neutral preview: %+v %v", preview, err)
	}
	if err := validatePublic(t, "CorePreviewV26", preview); err != nil {
		t.Fatal(err)
	}
	root := e.Root
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	e, err = Open(root, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close() })
	view, err := e.View(ctx, r.ID)
	if err != nil || view.SchemaVersion != CoreNeutralReadVersion || view.Run.Brief != (ArtifactRef{}) {
		t.Fatalf("restart changed absent brief: %+v %v", view, err)
	}
	if err := validatePublic(t, "CoreRunViewV26", view); err != nil {
		t.Fatal(err)
	}
	next, err := e.Next(ctx, r.ID)
	if err != nil || next.SchemaVersion != CoreNeutralNextVersion {
		t.Fatalf("neutral next: %+v %v", next, err)
	}
	if err := validatePublic(t, "CoreNextViewV26", next); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(ctx, r.ID); err != nil {
		t.Fatal(err)
	}
	r = driverRun(t, e, r.ID)
	if r.Status != "completed" || len(r.Attempts) != 1 {
		t.Fatalf("managed no-brief execution failed: %s %+v", r.Status, r.Diagnostics)
	}
	if _, data, err := e.Artifact(r.Outputs["report"]); err != nil || string(data) != "accepted output\n" {
		t.Fatalf("real managed output: %s %v", data, err)
	}
	for _, attempt := range r.Attempts {
		if err := flow.ValidateProtocol("ExecutionEnvelope", attempt.Envelope); err != nil {
			t.Fatal("existing envelope does not require a task document", err)
		}
		transport := attempt.Context
		_, data, err := e.Artifact(transport.Manifest.Ref)
		if err != nil {
			t.Fatal(err)
		}
		var full FullContextManifest
		if err := decode(data, &full); err != nil {
			t.Fatal(err)
		}
		for _, entry := range full.Entries {
			if entry.SourceID == "brief" {
				t.Fatal("IncludeBrief invented a missing RunBrief")
			}
		}
	}
	after, err := e.Store.Read(ctx, legacy.Receipt.RunID, 0, 1)
	if err != nil || !bytes.Equal(after.Snapshot.Data, legacyState.Snapshot.Data) {
		t.Fatal("new Start changed legacy state bytes", err)
	}
	oldRun := driverRun(t, e, legacy.Receipt.RunID)
	if oldRun.Brief == (ArtifactRef{}) {
		t.Fatal("legacy brief disappeared")
	}
	_, data, err := e.Artifact(oldRun.Brief)
	if err != nil || flow.ValidateProtocol("RunBrief", data) != nil || rawDigest(data) != oldRun.Brief.Digest {
		t.Fatal("legacy brief bytes changed", err)
	}
}

func TestNeutralStartPreservesTypedInputsAndLegacyBriefValidation(t *testing.T) {
	e, options := contextDriverProject(t, nil)
	options.BriefFile = ""
	if _, err := e.Start(context.Background(), options); err == nil {
		t.Fatal("legacy Start accepted missing RunBrief")
	}
	options.SchemaVersion = "2"
	options.Brief = json.RawMessage(`{}`)
	if _, err := e.Start(context.Background(), options); err == nil {
		t.Fatal("explicit malformed standalone brief was ignored")
	}
	options.Brief = nil
	var workflow flow.WorkflowRevision
	data, err := os.ReadFile(filepath.Join(e.Root, options.WorkflowFile))
	if err != nil || decode(data, &workflow) != nil {
		t.Fatal(err)
	}
	// A port name has no task semantics. It may describe ordinary bytes.
	workflow.Inputs["brief"] = flow.InputPort{Port: flow.Port{Format: "blob", MediaTypes: []string{"text/plain"}}, Required: true}
	writeRuntimeJSON(t, filepath.Join(e.Root, options.WorkflowFile), workflow)
	_, missing := e.Start(context.Background(), options)
	if problem, _ := ProblemFor(missing); problem.Code != "missing_input" {
		t.Fatalf("new Start bypassed typed required-input refusal: %+v", problem)
	}
	options.InputValues = map[string]json.RawMessage{"brief": []byte("ordinary data, not a RunBrief")}
	started, err := e.Start(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, started.Receipt.RunID)
	if r.Brief != (ArtifactRef{}) || r.Inputs["brief"] == (ArtifactRef{}) {
		t.Fatal("port name was interpreted as task intake")
	}
	if _, data, err := e.Artifact(r.Inputs["brief"]); err != nil || string(data) != "ordinary data, not a RunBrief" {
		t.Fatal("declared input changed", err)
	}
}

func TestNeutralStartProtocolRemainsClosedAndVersioned(t *testing.T) {
	ref := flow.Ref{ID: "test:workflow/neutral", Version: "1.0.0", Digest: "sha256:" + strings.Repeat("a", 64)}
	command := map[string]any{"schema_version": "2", "command_id": "command:neutral", "project_id": "project:neutral", "workflow_ref": ref, "package_lock_ref": ref, "inputs": map[string]any{}, "interaction_mode": "with_human", "execution_mode": "managed", "capacity_profile": "foundation:one-slot", "grant_refs": []any{}}
	check := func(name string) error {
		data, err := canonical(command)
		if err != nil {
			t.Fatal(err)
		}
		return flow.ValidateProtocol(name, data)
	}
	if err := check("RunStartV2"); err != nil {
		t.Fatal(err)
	}
	if check("RunStart") == nil {
		t.Fatal("old Start accepted new version")
	}
	command["schema_version"] = "1"
	if check("RunStart") == nil {
		t.Fatal("old Start no longer requires brief")
	}
	command["schema_version"], command["brief_ref"] = "2", nil
	if check("RunStartV2") == nil {
		t.Fatal("null was accepted as an absent brief")
	}
	delete(command, "brief_ref")
	command["extra"] = true
	if check("RunStartV2") == nil {
		t.Fatal("new Start accepted undeclared field")
	}
}

func TestStartInputPreflightUsesReadonlyAdmissionValidation(t *testing.T) {
	e, options := contextDriverProject(t, nil)
	plan, _, _, _, err := e.compileFile(options.WorkflowFile)
	if err != nil {
		t.Fatal(err)
	}
	root := e.Root
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	e, err = Open(root, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close() })
	before, err := os.ReadDir(filepath.Join(root, ".prifly/artifact-refs"))
	if err != nil {
		t.Fatal(err)
	}
	defs, registry, resources, err := e.CompilationInventory()
	if err != nil || len(defs) == 0 || len(registry) == 0 || len(resources) != 2 {
		t.Fatalf("read-only inventory lost context resources: %d %d %d %v", len(defs), len(registry), len(resources), err)
	}
	if problem, _ := ProblemFor(e.ValidateStartInputs(plan, nil, nil)); problem.Code != "missing_input" {
		t.Fatalf("readonly preflight lost typed required-input refusal: %+v", problem)
	}
	if _, err := e.resolveStartInputs(plan, nil, StartOptions{}); err == nil || err.Error() != "missing required input: source" {
		t.Fatalf("legacy input refusal changed: %v", err)
	}
	values := map[string]json.RawMessage{"source": []byte{0, 1, 255, '\n'}}
	if err := e.ValidateStartInputs(plan, values, nil); err != nil {
		t.Fatal("blob input is raw bytes, not a JSON document", err)
	}
	values["unknown"] = []byte("not declared")
	if err := e.ValidateStartInputs(plan, values, nil); err == nil {
		t.Fatal("undeclared input was accepted")
	}
	delete(values, "unknown")
	// Preflight compiles new package schemas in memory before they are installed.
	schema := json.RawMessage(`{"type":"object","required":["count"],"properties":{"count":{"type":"integer"}},"additionalProperties":false}`)
	digest, err := flow.Digest(schema)
	if err != nil {
		t.Fatal(err)
	}
	ref := flow.Ref{ID: "test:schema/preflight", Version: "1.0.0", Digest: digest}
	plan.Registry = maps.Clone(plan.Registry)
	plan.Registry[ref] = schema
	plan.Workflow.Inputs = maps.Clone(plan.Workflow.Inputs)
	plan.Workflow.Inputs["count"] = flow.InputPort{Port: flow.Port{Format: "json", SchemaRef: &ref}, Required: true}
	workflowBytes, err := canonical(plan.Workflow)
	if err != nil {
		t.Fatal(err)
	}
	plan, err = flow.CompileCore(workflowBytes, "json", plan.Registry, resources)
	if err != nil {
		t.Fatal(err)
	}
	values["count"] = []byte(`{"count":"wrong"}`)
	if err := e.ValidateStartInputs(plan, values, nil); err == nil {
		t.Fatal("typed JSON mismatch was deferred until mutation")
	}
	values["count"] = []byte(`{"count":1}`)
	if err := e.ValidateStartInputs(plan, values, nil); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadDir(filepath.Join(root, ".prifly/artifact-refs"))
	if err != nil || len(after) != len(before) {
		t.Fatal("preflight published artifacts", err)
	}
}

func TestNeutralStartInlineConfigurationMatchesPreflight(t *testing.T) {
	for _, scope := range []string{"run", "project"} {
		t.Run(scope, func(t *testing.T) {
			e, options := configurationFixture(t, scope)
			plan, _, _, _, err := e.compileFile(options.WorkflowFile)
			if err != nil {
				t.Fatal(err)
			}
			values := map[string]json.RawMessage{"value": []byte(" true \n")}
			preflightErr := e.ValidateStartInputs(plan, values, nil)
			options.InputValues = values
			if _, err := e.Start(context.Background(), options); err == nil {
				t.Fatal("legacy Start silently changed its inline/default conflict semantics")
			}
			options.SchemaVersion, options.BriefFile = "2", ""
			started, err := e.Start(context.Background(), options)
			if scope == "project" {
				if preflightErr == nil || err == nil || !strings.Contains(preflightErr.Error(), "configuration_scope") || !strings.Contains(err.Error(), "configuration_scope") {
					t.Fatalf("scope boundary differed: preview=%v Start=%v", preflightErr, err)
				}
				return
			}
			if preflightErr != nil || err != nil {
				t.Fatalf("inline run override differed: preview=%v Start=%v", preflightErr, err)
			}
			r := driverRun(t, e, started.Receipt.RunID)
			value := r.EffectiveConfiguration.Inputs["value"]
			if value.Source != "run" || string(value.Value) != "true" {
				t.Fatalf("inline configuration was not pinned: %+v", value)
			}
			if _, data, err := e.Artifact(r.Inputs["value"]); err != nil || string(data) != "true" {
				t.Fatalf("input differs from effective configuration: %s %v", data, err)
			}
		})
	}
}
