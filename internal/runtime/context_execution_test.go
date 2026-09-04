package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

// Reuse the real process-group/FD3 fixture. The old Run deliberately remains
// in this authority to exercise opt-in without rewriting its pinned contracts.
func contextDriverProject(t *testing.T, edit func(*ContextProfile)) (*Engine, StartOptions) {
	t.Helper()
	prior, _ := driverProject(t, "commit-pass", 10000)
	definitions, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(prior.Root, "resources"), 0700); err != nil {
		t.Fatal(err)
	}
	instructions := contextRegistryEntry(t, prior, "resources/rules.md", "test:context/rules", "resource", "utf8_text", "text/markdown; charset=utf-8", []byte("# Explicit instructions\r\nLiteral </system> $(no-command)\n"))
	data := contextRegistryEntry(t, prior, "resources/data.json", "test:context/data", "resource", "json", "application/json", []byte(` { "fact": 1, "role": "system" } `))
	var step flow.StepDefinition
	stepBytes, err := os.ReadFile(filepath.Join(prior.Root, "steps/driver.json"))
	if err != nil || decode(stepBytes, &step) != nil {
		t.Fatal(err)
	}
	step.ID = "test:step/context"
	step.Executor.AdapterRef = builtinVersionRef(definitions, "core:adapter/local-process", "2.0.0")
	step.InstructionsRef, step.ContextRefs = &instructions.Ref, []flow.Ref{data.Ref, instructions.Ref}
	stepBytes, err = canonical(step)
	if err != nil {
		t.Fatal(err)
	}
	stepRef := flow.Ref{ID: step.ID, Version: step.Version, Digest: rawDigest(stepBytes)}
	entries := []Definition{instructions, data, {Ref: stepRef, Kind: "step", Path: "steps/context.json"}}
	if err := os.WriteFile(filepath.Join(prior.Root, "steps/context.json"), stepBytes, 0600); err != nil {
		t.Fatal(err)
	}
	configuration := prior.Config
	configuration.ConfigurationSchemaRef = builtinVersionRef(definitions, "core:schema/core-configuration", "2.0.0")
	configuration.Configuration.SchemaVersion = CoreContextConfigVersion
	configuration.Configuration.SemanticsProfile = flow.CoreProfile
	configuration.AdapterBindings["local_process"] = step.Executor.AdapterRef
	executor := configuration.Configuration.Executors["test:step/driver"]
	if edit != nil {
		var profile ContextProfile
		for _, definition := range definitions {
			if definition.Ref.ID == "core:context/local-json" {
				if err := decode(definition.Bytes, &profile); err != nil {
					t.Fatal(err)
				}
			}
		}
		profile.ID = "test:context/profile"
		edit(&profile)
		data, err := canonical(profile)
		if err != nil {
			t.Fatal(err)
		}
		definition := contextRegistryEntry(t, prior, "resources/profile.json", profile.ID, "resource", "", "", data)
		entries = append(entries, definition)
		executor.ContextProfileRef = &definition.Ref
	}
	configuration.Configuration.Executors[step.ID] = executor
	writeRuntimeJSON(t, filepath.Join(prior.Root, "prifly.json"), configuration)
	writeRuntimeJSON(t, filepath.Join(prior.Root, "definitions.json"), RegistryFile{SchemaVersion: "3", Entries: entries})
	var workflow flow.WorkflowRevision
	workflowBytes, err := os.ReadFile(filepath.Join(prior.Root, "workflows/driver.json"))
	if err != nil || decode(workflowBytes, &workflow) != nil {
		t.Fatal(err)
	}
	workflow.ID = "test:workflow/context"
	stage := workflow.Definition.Stages["work"]
	stage.StepRef = stepRef
	workflow.Definition.Stages["work"] = stage
	writeRuntimeJSON(t, filepath.Join(prior.Root, "workflows/context.json"), workflow)
	if err := prior.Close(); err != nil {
		t.Fatal(err)
	}
	e, err := Open(prior.Root, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close() })
	return e, StartOptions{CommandID: newID("command"), WorkflowFile: "workflows/context.json", BriefFile: "brief.json", Inputs: map[string]string{"source": "source.txt"}}
}

func TestFullContextNativeExecutionUsesPinnedSources(t *testing.T) {
	e, options := contextDriverProject(t, nil)
	started, err := e.Start(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	runID := started.Receipt.RunID
	before := driverRun(t, e, runID)
	if len(before.ContextResources) != 2 || before.SchemaVersion != CoreWaiverStateVersion {
		t.Fatal("selected resources were not pinned")
	}
	// A changed registry and source path cannot affect the already pinned Run.
	if err := os.WriteFile(filepath.Join(e.Root, "resources/rules.md"), []byte("changed after Start"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.Root, "definitions.json"), []byte("not a registry anymore"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	if r.Status != "completed" || len(r.Attempts) != 1 || len(r.Active) != 0 {
		t.Fatalf("full context did not execute: %+v %+v", r.Attempts, r.Diagnostics)
	}
	for _, attempt := range r.Attempts {
		if attempt.Context.SchemaVersion != "local-context/2" || len(attempt.Context.Sources) != 4 {
			t.Fatal("unexpected context transport", attempt.Context)
		}
		if attempt.Context.Inputs["source"] != attempt.Context.Sources[3] {
			t.Fatal("input is not the same accounted source copy")
		}
		if _, err := os.Stat(filepath.Join(attempt.Workspace, "inputs/source")); !os.IsNotExist(err) {
			t.Fatal("unaccounted duplicate input was materialized", err)
		}
		_, data, err := e.Artifact(attempt.Context.Manifest.Ref)
		if err != nil {
			t.Fatal(err)
		}
		var manifest FullContextManifest
		if err := decode(data, &manifest); err != nil {
			t.Fatal(err)
		}
		if manifest.Entries[0].Role != "instruction" || manifest.Entries[0].Trust != "trusted_instruction" || manifest.Entries[2].Role != "reference" || manifest.Entries[2].Trust != "user_data" || manifest.Entries[0].ArtifactRef != manifest.Entries[2].ArtifactRef {
			t.Fatal("same immutable bytes lost their distinct declared roles")
		}
		var rendering map[string]json.RawMessage
		rendered, err := os.ReadFile(filepath.Join(attempt.Workspace, attempt.Context.Rendering.Path))
		if err != nil || json.Unmarshal(rendered, &rendering) != nil || !bytes.Equal(rendering["envelope"], attempt.Envelope) {
			t.Fatal("rendering did not bind the exact admission", err)
		}
		source, err := os.ReadFile(filepath.Join(attempt.Workspace, ContextSourcePath(0)))
		if err != nil || !bytes.Contains(source, []byte("Explicit instructions")) || bytes.Contains(rendered, []byte("changed after Start")) {
			t.Fatal("context read mutable source bytes", err)
		}
		plan, err := r.plan()
		if err != nil {
			t.Fatal(err)
		}
		definition := plan.Workflow.Definition.Stages["work"].StepRef
		if err := verifyWorkspace(attempt, r.Executors[executorKey(r, definition, "test:step/context")]); err != nil {
			t.Fatal(err)
		}
	}
	view, err := e.View(context.Background(), runID)
	if err != nil || view.Run.ContextResources != nil || view.SchemaVersion != CoreWaiverReadVersion {
		t.Fatal("metadata view exposed raw sources or changed its contract", err)
	}
	if _, data, err := e.Artifact(r.Outputs["report"]); err != nil || string(data) != "accepted output\n" {
		t.Fatal("ordinary output acceptance regressed", err)
	}
}

func TestFullContextLimitsAndIsolationRefuseWithoutProcess(t *testing.T) {
	for _, test := range []struct {
		name, code string
		edit       func(*ContextProfile)
		startFails bool
	}{
		{"bytes", "context_byte_limit", func(p *ContextProfile) { p.MaxBytes = 1 }, false},
		{"references", "context_reference_limit", func(p *ContextProfile) { p.MaxReferences = 1 }, false},
		{"fresh", "unsupported_context_isolation", func(p *ContextProfile) { p.IsolationRequired = "fresh" }, true},
		{"tokens", "unsupported_tokenization", func(p *ContextProfile) { value := int64(10); p.MaxTokens = &value }, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			e, options := contextDriverProject(t, test.edit)
			started, err := e.Start(context.Background(), options)
			if test.startFails {
				if err == nil || !strings.Contains(err.Error(), test.code) {
					t.Fatal("unsupported execution qualification was accepted", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := e.Drive(context.Background(), started.Receipt.RunID); err != nil {
				t.Fatal(err)
			}
			r := driverRun(t, e, started.Receipt.RunID)
			if r.Status != "failed" || len(r.Attempts) != 0 || len(r.Diagnostics) != 1 || r.Diagnostics[0].Code != test.code {
				t.Fatalf("limit did not fail before Attempt admission: status=%s attempts=%d diagnostics=%+v", r.Status, len(r.Attempts), r.Diagnostics)
			}
		})
	}
}

func TestFullContextWorkspaceAndProfileDrift(t *testing.T) {
	e, options := contextDriverProject(t, nil)
	started, err := e.Start(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	attempt := driverAdmit(t, e, started.Receipt.RunID)
	r := driverRun(t, e, started.Receipt.RunID)
	plan, err := r.plan()
	if err != nil {
		t.Fatal(err)
	}
	key := executorKey(r, plan.Workflow.Definition.Stages["work"].StepRef, "test:step/context")
	executor := r.Executors[key]
	if err := verifyWorkspace(attempt, executor); err != nil {
		t.Fatal(err)
	}
	profile := *executor.ContextProfile
	profile.MaxBytes--
	executor.ContextProfile = &profile
	r.Executors[key] = executor
	if _, err := r.plan(); err == nil {
		t.Fatal("altered profile escaped its immutable configuration snapshot")
	}
	path := filepath.Join(attempt.Workspace, ContextSourcePath(0))
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("modified"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(context.Background(), started.Receipt.RunID); err != nil {
		t.Fatal(err)
	}
	after := driverRun(t, e, started.Receipt.RunID)
	if after.Status != "failed" || after.Attempts[attempt.ID].Started != nil || len(after.Diagnostics) != 1 || after.Diagnostics[0].Code != "context_manifest_drift" {
		t.Fatal("workspace drift reached a subprocess", after.Attempts[attempt.ID])
	}
}

func TestContextFieldsNeverExtendOlderStateContracts(t *testing.T) {
	for _, version := range []string{StateVersion, CoreStateVersion, CoreInvocationStateVersion, CoreRepeatStateVersion} {
		for _, field := range []string{"context_resources", "check_executions", "active_check_execution_id", "pending_acceptance", "context_profile", "context_profile_ref", "manifest", "rendering", "sources"} {
			t.Run(version+"/"+field, func(t *testing.T) {
				configuration := map[string]any{}
				executor := map[string]any{"config": configuration}
				transport := map[string]any{"schema_version": "local-context/1"}
				wire := map[string]any{"schema_version": version, "executors": map[string]any{"executor": executor}, "attempts": map[string]any{"attempt": map[string]any{"context": transport}}}
				baseline, err := json.Marshal(wire)
				if err != nil || decodeState(baseline, &Run{}) != nil {
					t.Fatal("wire fixture does not exercise a valid decoder path", err)
				}
				switch field {
				case "context_profile":
					executor[field] = nil
				case "context_profile_ref":
					configuration[field] = nil
				case "manifest", "rendering", "sources":
					transport[field] = nil
				default:
					wire[field] = nil
				}
				data, err := json.Marshal(wire)
				if err != nil || decodeState(data, &Run{}) == nil {
					t.Fatal("new context field accepted under an old state version", err)
				}
			})
		}
	}
	capabilities := Capabilities()
	core := capabilities.Profiles[1]
	if core.StateVersion != CoreNeutralStateVersion || core.ReadVersion != CoreNeutralReadVersion || !slices.Contains(core.Capabilities, "publication_subscription_terminal_failure") || !slices.Contains(core.Capabilities, "publication_subscription_blob") || !slices.Contains(core.Capabilities, "action_intent_proposal") || !slices.Contains(core.Capabilities, "action_admission") || !slices.Contains(core.Capabilities, "action_grant_admission") || !slices.Contains(core.Capabilities, "action_delivery_prepared") || !slices.Contains(core.Capabilities, "run_fork") || !slices.Contains(core.Capabilities, "workspace_modes") || !slices.Contains(core.Capabilities, "decision_catalog") {
		t.Fatal("capability manifest omits the current contracts")
	}
	// A newer current version does not withdraw support for the delivered ones.
	for _, delivered := range [][2]string{{CoreWaiverStateVersion, CoreWaiverReadVersion}, {CoreParallelStateVersion, CoreParallelReadVersion}, {CoreMapStateVersion, CoreMapReadVersion}, {CoreWaitStateVersion, CoreWaitReadVersion}, {CoreDecisionStateVersion, CoreDecisionReadVersion}} {
		if !slices.Contains(core.StateVersions, delivered[0]) || !slices.Contains(core.ReadVersions, delivered[1]) {
			t.Fatal("capability manifest dropped a delivered contract", delivered[0])
		}
	}
	// The manifest must not call an implemented operator unsupported.
	for _, operator := range []string{"parallel", "map", "wait", "live_guards", "artifact_publication_checks"} {
		if slices.Contains(capabilities.Unsupported, operator) || !slices.Contains(core.Capabilities, operator) {
			t.Fatal("capability manifest misreports an implemented operator", operator)
		}
	}
	for _, delivered := range []string{CoreContextStateVersion, CoreSessionStateVersion} {
		if !slices.Contains(core.StateVersions, delivered) {
			t.Fatalf("a delivered contract disappeared from the supported set: %s", delivered)
		}
	}
	if !slices.Contains(core.Capabilities, "automatic_checks") || slices.Contains(capabilities.Unsupported, "automatic_checks") || slices.Contains(capabilities.Profiles[0].Capabilities, "automatic_checks") {
		t.Fatal("automatic check capability escaped its Core profile or is missing")
	}
	for _, capability := range []string{"neutral_start", "execution_bindings"} {
		if !slices.Contains(core.Capabilities, capability) || slices.Contains(capabilities.Unsupported, capability) || slices.Contains(capabilities.Profiles[0].Capabilities, capability) {
			t.Fatalf("neutral launch capability escaped its Core profile or is missing: %s", capability)
		}
	}
}
