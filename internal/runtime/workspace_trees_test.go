package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

func writeWorkspaceTreeFile(t *testing.T, root, path, value string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(value), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestCaptureWorkspaceTreePreservesFastFullAndUltraBytes(t *testing.T) {
	root := t.TempDir()
	for _, test := range []struct {
		name, location, entrypoint string
		policy                     flow.WorkspaceTreeCapturePolicy
		files                      map[string]string
	}{
		{"fast", ".ai-factory/PLAN.md", "PLAN.md", flow.WorkspaceTreeCapturePolicy{Kind: "exact_file", Path: ".ai-factory/PLAN.md"}, map[string]string{".ai-factory/PLAN.md": "# Fast\n"}},
		{"full", ".ai-factory/plans/feature.md", "feature.md", flow.WorkspaceTreeCapturePolicy{Kind: "direct_child_file", Path: ".ai-factory/plans"}, map[string]string{".ai-factory/plans/feature.md": "# Full\n"}},
		{"ultra", ".ai-factory/plans/feature", "index.md", flow.WorkspaceTreeCapturePolicy{Kind: "direct_child_tree", Path: ".ai-factory/plans", Entrypoint: "index.md"}, map[string]string{".ai-factory/plans/feature/index.md": "# Ultra\n", ".ai-factory/plans/feature/phase-1.md": "# Phase\n"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			for path, value := range test.files {
				writeWorkspaceTreeFile(t, root, path, value)
			}
			manifest, files, err := captureWorkspaceTree(root, WorkspaceTreeHandoff{Capture: test.policy}, test.location)
			if err != nil || manifest.Root == "" || manifest.Entrypoint != test.entrypoint || len(manifest.Files) != len(test.files) {
				t.Fatalf("capture is not the declared native form: %+v %v", manifest, err)
			}
			for path, value := range test.files {
				name := filepath.Base(path)
				if !bytes.Equal(files[name], []byte(value)) {
					t.Fatalf("%s bytes changed: %q", name, files[name])
				}
			}
		})
	}
}

func TestCaptureWorkspaceTreeRefusesUnsafeOrMissingUltraEntries(t *testing.T) {
	root := t.TempDir()
	policy := flow.WorkspaceTreeCapturePolicy{Kind: "direct_child_tree", Path: ".ai-factory/plans", Entrypoint: "index.md"}
	writeWorkspaceTreeFile(t, root, ".ai-factory/plans/feature/phase.md", "phase")
	if _, _, err := captureWorkspaceTree(root, WorkspaceTreeHandoff{Capture: policy}, ".ai-factory/plans/feature"); err == nil {
		t.Fatal("bundle without entrypoint was captured")
	}
	writeWorkspaceTreeFile(t, root, ".ai-factory/plans/feature/index.md", "index")
	if err := os.Symlink("index.md", filepath.Join(root, ".ai-factory/plans/feature/escape.md")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := captureWorkspaceTree(root, WorkspaceTreeHandoff{Capture: policy}, ".ai-factory/plans/feature"); !errors.Is(err, local.ErrUnsafePath) {
		t.Fatalf("symlink capture was not refused: %v", err)
	}
}

// treeDecisionInputs is the smallest sealed decision catalog and sheet that
// puts a Run on the decision-state contract, so its handoffs are
// assisted-session/5 while still declaring workspace trees.
func treeDecisionInputs(t *testing.T) (*DecisionCatalog, *DecisionSheet) {
	t.Helper()
	definition := DecisionDefinition{SchemaVersion: DecisionDefinitionVersion, ID: "plan_profile", Title: "Plan profile", Phase: "preflight", Required: true, Choices: []DecisionChoice{{ID: "fast", Title: "Fast", Value: json.RawMessage(`"fast"`)}}, Recommendation: json.RawMessage(`"fast"`), Automatic: true, Sensitivity: "ordinary", Destination: DecisionDestination{Kind: "package_profile"}}
	catalog := DecisionCatalog{SchemaVersion: DecisionCatalogVersion, Decisions: []DecisionDefinition{definition}}
	catalogDigest, err := DecisionCatalogDigest(catalog)
	if err != nil {
		t.Fatal(err)
	}
	definitionDigest, err := DecisionDefinitionDigest(definition)
	if err != nil {
		t.Fatal(err)
	}
	sheet := DecisionSheet{SchemaVersion: DecisionSheetVersion, CatalogDigest: catalogDigest, PackageProfile: "fast", ProfileSource: "actor", Records: []DecisionRecord{{SchemaVersion: DecisionRecordVersion, DefinitionID: definition.ID, DefinitionDigest: definitionDigest, Status: "answered", Source: "actor", Value: json.RawMessage(`"fast"`)}}}
	if err := ValidateDecisionSheet(catalog, sheet); err != nil {
		t.Fatal(err)
	}
	return &catalog, &sheet
}

func treeSessionFixture(t *testing.T, policy flow.WorkspaceTreeCapturePolicy) (*Engine, string) {
	t.Helper()
	return treeFixture(t, policy, nil, nil, "worktree", false)
}

// treeVerifyFixture appends a read-only verify step that is handed the
// implemented plan through a materialize-only binding (StepDefinition v8).
func treeVerifyFixture(t *testing.T, policy flow.WorkspaceTreeCapturePolicy) (*Engine, string) {
	t.Helper()
	return treeFixture(t, policy, nil, nil, "worktree", true)
}

func treeDecisionSessionFixture(t *testing.T, policy flow.WorkspaceTreeCapturePolicy) (*Engine, string) {
	t.Helper()
	catalog, sheet := treeDecisionInputs(t)
	return treeFixture(t, policy, catalog, sheet, "worktree", false)
}

func treeFixture(t *testing.T, policy flow.WorkspaceTreeCapturePolicy, catalog *DecisionCatalog, sheet *DecisionSheet, mode string, verify bool) (*Engine, string) {
	t.Helper()
	e := contextRegistryRuntime(t)
	claim, err := e.ClaimWorktree(context.Background(), ClaimRequest{CommandID: "command:tree-claim", Repository: gitRepository(t), OwnerID: "session:pilot", WorkspaceMode: mode})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.claimWorkspacePath(claim); err != nil {
		t.Fatal(err)
	}
	definitions, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	skill := []byte("# native plan skill\n")
	if err := os.MkdirAll(filepath.Join(e.Root, "resources"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.Root, "resources", "tree-skill.md"), skill, 0600); err != nil {
		t.Fatal(err)
	}
	skillRef := flow.Ref{ID: "test:context/tree-skill", Version: "1.0.0", Digest: rawDigest(skill)}
	manifestRef := builtinRef(definitions, flow.WorkspaceTreeManifestSchemaID)
	step := func(id string, input, output string) (flow.StepDefinition, flow.Ref) {
		definition := flow.StepDefinition{SchemaVersion: "5", ID: id, Version: "1.0.0", Title: id, Kind: "worker", Inputs: map[string]flow.InputPort{}, Outputs: map[string]flow.OutputPort{output: {Port: flow.Port{Format: "json", SchemaRef: &manifestRef}, RequiredFor: []string{"pass"}}}, InstructionsRef: &skillRef, ContextRefs: []flow.Ref{}, RequiredCapabilities: []string{}, ResultCheckRefs: []flow.Ref{}, ResultSchemaRef: builtinRef(definitions, "core:schema/step-result"), WorkspaceTrees: []flow.WorkspaceTreeBinding{{InputPort: input, OutputPort: output, Capture: policy}}}
		if input != "" {
			definition.Inputs[input] = flow.InputPort{Port: flow.Port{Format: "json", SchemaRef: &manifestRef}, Required: true}
		}
		definition.Executor.AdapterRef, definition.Executor.Operation = builtinRef(definitions, "core:adapter/assisted-session"), "session"
		definition.Effects.Class, definition.Effects.RetryClass = "workspace_write", "never"
		data := writeRegistryDocument(t, e, "steps/"+filepath.Base(id)+".json", definition)
		return definition, flow.Ref{ID: definition.ID, Version: definition.Version, Digest: rawDigest(data)}
	}
	_, planRef := step("test:step/tree-plan", "", "plan")
	_, improveRef := step("test:step/tree-improve", "plan", "improved")
	_, implementRef := step("test:step/tree-implement", "plan", "final")
	readOnly := flow.StepDefinition{SchemaVersion: "8", ID: "test:step/tree-verify", Version: "1.0.0", Title: "verify", Kind: "worker", Inputs: map[string]flow.InputPort{"plan": {Port: flow.Port{Format: "json", SchemaRef: &manifestRef}, Required: true}}, Outputs: map[string]flow.OutputPort{}, WorkspaceTrees: []flow.WorkspaceTreeBinding{{InputPort: "plan", Capture: policy}}}
	readOnly.Executor.AdapterRef, readOnly.Executor.Operation = builtinRef(definitions, "core:adapter/assisted-session"), "session"
	readOnly.Effects.Class, readOnly.Effects.RetryClass = "none", "never"
	readOnly.InstructionsRef, readOnly.ContextRefs, readOnly.RequiredCapabilities, readOnly.ResultCheckRefs, readOnly.ResultSchemaRef = &skillRef, []flow.Ref{}, []string{}, []flow.Ref{}, builtinRef(definitions, "core:schema/step-result")
	verifyData := writeRegistryDocument(t, e, "steps/tree-verify.json", readOnly)
	verifyRef := flow.Ref{ID: readOnly.ID, Version: readOnly.Version, Digest: rawDigest(verifyData)}
	workflow := flow.WorkflowRevision{SchemaVersion: "1", ID: "test:workflow/tree", Version: "1.0.0", Title: "Native tree", Inputs: map[string]flow.InputPort{}, Outputs: map[string]flow.OutputPort{}, AllowedOutcomes: []string{"succeeded"}, Limits: flow.Limits{MaxStepInstances: 4, MaxControlTransitions: 32, MaxParallelism: 1}, PolicyRef: builtinVersionRef(definitions, "core:policy/local", "2.0.0")}
	workflow.Definition.Entry = "plan"
	workflow.Definition.Stages = map[string]flow.Stage{
		"plan":      {Kind: "step", StepRef: planRef, InputBindings: map[string]flow.Binding{}, On: map[string]string{"pass": "improve"}},
		"improve":   {Kind: "step", StepRef: improveRef, InputBindings: map[string]flow.Binding{"plan": {From: "stage_output", StageID: "plan", Port: "plan"}}, On: map[string]string{"pass": "implement"}},
		"implement": {Kind: "step", StepRef: implementRef, InputBindings: map[string]flow.Binding{"plan": {From: "stage_output", StageID: "improve", Port: "improved"}}, On: map[string]string{"pass": "done"}},
		"done":      {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]flow.Binding{}},
	}
	if verify {
		workflow.Definition.Stages["implement"] = flow.Stage{Kind: "step", StepRef: implementRef, InputBindings: map[string]flow.Binding{"plan": {From: "stage_output", StageID: "improve", Port: "improved"}}, On: map[string]string{"pass": "verify"}}
		workflow.Definition.Stages["verify"] = flow.Stage{Kind: "step", StepRef: verifyRef, InputBindings: map[string]flow.Binding{"plan": {From: "stage_output", StageID: "implement", Port: "final"}}, On: map[string]string{"pass": "done"}}
	}
	writeRuntimeJSON(t, filepath.Join(e.Root, "workflows/tree.json"), workflow)
	registry := RegistryFile{SchemaVersion: "3", Entries: []Definition{{Ref: skillRef, Kind: "resource", Path: "resources/tree-skill.md", ByteEncoding: "utf8_text", MediaType: "text/markdown; charset=utf-8"}, {Ref: planRef, Kind: "step", Path: "steps/tree-plan.json"}, {Ref: improveRef, Kind: "step", Path: "steps/tree-improve.json"}, {Ref: implementRef, Kind: "step", Path: "steps/tree-implement.json"}}}
	if verify {
		registry.Entries = append(registry.Entries, Definition{Ref: verifyRef, Kind: "step", Path: "steps/tree-verify.json"})
	}
	writeRuntimeJSON(t, filepath.Join(e.Root, e.Config.Configuration.RegistryFile), registry)
	e.Config.Configuration.SchemaVersion, e.Config.ConfigurationSchemaRef = CoreContextConfigVersion, builtinVersionRef(definitions, "core:schema/core-configuration", "2.0.0")
	e.Config.AdapterBindings["local_process"], e.Config.DefaultPolicyRef = builtinVersionRef(definitions, "core:adapter/local-process", "2.0.0"), builtinVersionRef(definitions, "core:policy/local", "2.0.0")
	writeRuntimeJSON(t, filepath.Join(e.Root, "prifly.json"), e.Config)
	writeRuntimeJSON(t, filepath.Join(e.Root, "brief.json"), Brief{"1", "test:brief/tree", "tree", "native plan", []string{"Native plan"}, []string{}, []string{"Seal and improve the native plan"}, []ArtifactRef{}, []string{}, "explicit"})
	started, err := e.Start(context.Background(), StartOptions{CommandID: newID("command"), WorkflowFile: "workflows/tree.json", BriefFile: "brief.json", Inputs: map[string]string{}, DecisionCatalog: catalog, DecisionSheet: sheet, WorkspaceMode: mode})
	if err != nil {
		t.Fatal(err)
	}
	return e, started.Receipt.RunID
}

func treeSubmission(t *testing.T, task SessionTask, summary string, locations []WorkspaceTreeLocation) SessionSubmission {
	t.Helper()
	result, err := json.Marshal(Result{SchemaVersion: "1", RunID: task.RunID, StepInstanceID: task.StepInstanceID, AttemptID: task.AttemptID, EnvelopeDigest: task.EnvelopeDigest, Verdict: "pass", Outputs: map[string]ArtifactRef{}, EvidenceRefs: []any{}, EffectReceiptRefs: []any{}, Summary: summary})
	if err != nil {
		t.Fatal(err)
	}
	return SessionSubmission{SchemaVersion: task.SchemaVersion, RunID: task.RunID, AttemptID: task.AttemptID, EnvelopeDigest: task.EnvelopeDigest, Result: result, WorkspaceTrees: locations}
}

func TestWorkspaceTreeSessionPassesExactNativePlanToImproveAndImplement(t *testing.T) {
	tests := []struct {
		name, location string
		policy         flow.WorkspaceTreeCapturePolicy
		original       map[string]string
		improved       map[string]string
		final          map[string]string
	}{
		{"fast", ".ai-factory/PLAN.md", flow.WorkspaceTreeCapturePolicy{Kind: "exact_file", Path: ".ai-factory/PLAN.md"}, map[string]string{".ai-factory/PLAN.md": "# Original\n"}, map[string]string{".ai-factory/PLAN.md": "# Improved\n"}, map[string]string{".ai-factory/PLAN.md": "# Checked\n"}},
		{"full", ".ai-factory/plans/feature.md", flow.WorkspaceTreeCapturePolicy{Kind: "direct_child_file", Path: ".ai-factory/plans"}, map[string]string{".ai-factory/plans/feature.md": "# Original\n"}, map[string]string{".ai-factory/plans/feature.md": "# Improved\n"}, map[string]string{".ai-factory/plans/feature.md": "# Checked\n"}},
		{"ultra", ".ai-factory/plans/feature", flow.WorkspaceTreeCapturePolicy{Kind: "direct_child_tree", Path: ".ai-factory/plans", Entrypoint: "index.md"}, map[string]string{".ai-factory/plans/feature/index.md": "# Original\n", ".ai-factory/plans/feature/phase.md": "# Phase original\n"}, map[string]string{".ai-factory/plans/feature/index.md": "# Improved\n", ".ai-factory/plans/feature/phase.md": "# Phase improved\n"}, map[string]string{".ai-factory/plans/feature/index.md": "# Checked\n", ".ai-factory/plans/feature/phase.md": "# Phase checked\n"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			e, runID := treeSessionFixture(t, test.policy)
			assertWorkspace := func(workspace string, expected map[string]string) {
				for path, value := range expected {
					actual, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(path)))
					if err != nil || !bytes.Equal(actual, []byte(value)) {
						t.Fatalf("native workspace bytes differ for %s: %q %v", path, actual, err)
					}
				}
			}
			writeWorkspace := func(workspace string, files map[string]string) {
				for path, value := range files {
					writeWorkspaceTreeFile(t, workspace, path, value)
				}
			}
			first := handOver(t, e, runID)
			if first.SchemaVersion != AssistedSessionRoutedVersion || len(first.WorkspaceTrees) != 1 || first.WorkspaceTrees[0].InputManifest != nil {
				t.Fatalf("first tree handoff is not output-only: %+v", first)
			}
			writeWorkspace(first.RepositoryWorkspace, test.original)
			if _, err := e.SubmitSession(context.Background(), treeSubmission(t, first, "plan", []WorkspaceTreeLocation{{OutputPort: "plan", Path: test.location}})); err != nil {
				t.Fatal(err)
			}
			if err := e.Drive(context.Background(), runID); err != nil {
				t.Fatal(err)
			}
			second, err := e.SessionTask(context.Background(), runID, "")
			if err != nil {
				t.Fatal(err)
			}
			if second.SchemaVersion != AssistedSessionRoutedVersion || len(second.WorkspaceTrees) != 1 || second.WorkspaceTrees[0].InputManifest == nil {
				t.Fatalf("improve handoff lost the captured input: %+v", second)
			}
			assertWorkspace(second.RepositoryWorkspace, test.original)
			writeWorkspace(second.RepositoryWorkspace, test.improved)
			if _, err := e.SubmitSession(context.Background(), treeSubmission(t, second, "improved", nil)); err != nil {
				t.Fatal(err)
			}
			if err := e.Drive(context.Background(), runID); err != nil {
				t.Fatal(err)
			}
			third, err := e.SessionTask(context.Background(), runID, "")
			if err != nil {
				t.Fatal(err)
			}
			if third.SchemaVersion != AssistedSessionRoutedVersion || len(third.WorkspaceTrees) != 1 || third.WorkspaceTrees[0].InputManifest == nil {
				t.Fatalf("implement handoff lost the improved plan: %+v", third)
			}
			assertWorkspace(third.RepositoryWorkspace, test.improved)
			writeWorkspace(third.RepositoryWorkspace, test.final)
			if _, err := e.SubmitSession(context.Background(), treeSubmission(t, third, "implement", nil)); err != nil {
				t.Fatal(err)
			}
			if err := e.Drive(context.Background(), runID); err != nil {
				t.Fatal(err)
			}
			r := driverRun(t, e, runID)
			if r.SchemaVersion != CoreStageWorkStateVersion || r.Status != "completed" {
				t.Fatalf("tree run did not use and settle the v24 contract: %+v", r)
			}
			ref := r.Attempts[third.AttemptID].Accepted.Outputs["final"]
			artifact, data, err := e.Artifact(ref)
			if err != nil || !slices.Contains(artifact.Provenance, *third.WorkspaceTrees[0].InputManifest) {
				t.Fatalf("checked manifest lost improved-plan provenance: %+v %v", artifact, err)
			}
			var manifest WorkspaceTreeManifest
			if err := decode(data, &manifest); err != nil || len(manifest.Files) != len(test.final) {
				t.Fatalf("checked plan is not a complete manifest: %+v %v", manifest, err)
			}
			for _, entry := range manifest.Files {
				_, captured, err := e.Artifact(entry.Ref)
				if err != nil || !bytes.Equal(captured, []byte(test.final[filepath.ToSlash(filepath.Join(manifest.Root, entry.Path))])) {
					t.Fatalf("final native plan bytes were not sealed for %s: %q %v", entry.Path, captured, err)
				}
			}
		})
	}
}

func TestWorkspaceTreeRefusesPreHandoffDriftAndPolicyEscape(t *testing.T) {
	policy := flow.WorkspaceTreeCapturePolicy{Kind: "exact_file", Path: ".ai-factory/PLAN.md"}
	t.Run("output location", func(t *testing.T) {
		e, runID := treeSessionFixture(t, policy)
		first := handOver(t, e, runID)
		_, err := e.SubmitSession(context.Background(), treeSubmission(t, first, "plan", []WorkspaceTreeLocation{{OutputPort: "plan", Path: ".ai-factory/other.md"}}))
		if err == nil || !strings.Contains(err.Error(), "workspace_tree_policy_escape") {
			t.Fatalf("output outside the declared policy was accepted: %v", err)
		}
	})
	t.Run("input bytes", func(t *testing.T) {
		e, runID := treeSessionFixture(t, policy)
		first := handOver(t, e, runID)
		path := ".ai-factory/PLAN.md"
		writeWorkspaceTreeFile(t, first.RepositoryWorkspace, path, "# Pinned\n")
		if _, err := e.SubmitSession(context.Background(), treeSubmission(t, first, "plan", []WorkspaceTreeLocation{{OutputPort: "plan", Path: path}})); err != nil {
			t.Fatal(err)
		}
		writeWorkspaceTreeFile(t, first.RepositoryWorkspace, path, "# Unrelated edit\n")
		if err := e.Drive(context.Background(), runID); err != nil {
			t.Fatal(err)
		}
		if _, err := e.SessionTask(context.Background(), runID, ""); refusalCode(err) != "no_active_handoff" {
			t.Fatalf("a changed plan reached the next host: %v", err)
		}
		r := driverRun(t, e, runID)
		// The diagnostic names what preparation found, not merely that it ran:
		// the generic code left a reader with the phase and nothing else.
		if r.Status != "failed" || len(r.Diagnostics) != 1 || r.Diagnostics[0].Code != "workspace_tree_input_drift" {
			t.Fatalf("pre-handoff drift was not refused: status=%s diagnostics=%+v", r.Status, r.Diagnostics)
		}
		actual, err := os.ReadFile(filepath.Join(first.RepositoryWorkspace, filepath.FromSlash(path)))
		if err != nil || string(actual) != "# Unrelated edit\n" {
			t.Fatalf("runtime overwrote the unrelated file: %q %v", actual, err)
		}
	})
}

// A step that declares a workspace tree is captured by the runtime whatever the
// session version and whatever the host names. At assisted-session/5 an absent
// workspace_trees once skipped capture entirely, so the port the runtime owns
// was then reported missing and the host had no accepted submission form at all.
func TestWorkspaceTreeCaptureFollowsDeclaredBindingsAtRoutedSessionVersion(t *testing.T) {
	policy := flow.WorkspaceTreeCapturePolicy{Kind: "exact_file", Path: ".ai-factory/PLAN.md"}
	e, runID := treeDecisionSessionFixture(t, policy)
	first := handOver(t, e, runID)
	if first.SchemaVersion != AssistedSessionRoutedVersion || len(first.WorkspaceTrees) != 1 || first.WorkspaceTrees[0].InputManifest != nil {
		t.Fatalf("first handoff is not an output-only v5 tree binding: %+v", first)
	}
	writeWorkspaceTreeFile(t, first.RepositoryWorkspace, policy.Path, "# Original\n")
	// An exact-file policy admits one path, so the host names nothing.
	if _, err := e.SubmitSession(context.Background(), treeSubmission(t, first, "plan", nil)); err != nil {
		t.Fatalf("a v5 report without locations was refused: %v", err)
	}
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	second, err := e.SessionTask(context.Background(), runID, "")
	if err != nil {
		t.Fatalf("the captured plan never reached the next host: %v", err)
	}
	if second.WorkspaceTrees[0].InputManifest == nil || second.WorkspaceTrees[0].InputLocation != policy.Path {
		t.Fatalf("improve handoff lost the captured input: %+v", second.WorkspaceTrees)
	}
	// The same form serves an input+output binding, and repeating the location
	// the handoff named is equally accepted.
	writeWorkspaceTreeFile(t, second.RepositoryWorkspace, policy.Path, "# Improved\n")
	if _, err := e.SubmitSession(context.Background(), treeSubmission(t, second, "improved", []WorkspaceTreeLocation{{OutputPort: "improved", Path: policy.Path}})); err != nil {
		t.Fatalf("a repeated input location was refused: %v", err)
	}
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	third, err := e.SessionTask(context.Background(), runID, "")
	if err != nil {
		t.Fatal(err)
	}
	ref := driverRun(t, e, runID).Attempts[second.AttemptID].Accepted.Outputs["improved"]
	artifact, data, err := e.Artifact(ref)
	if err != nil || !slices.Contains(artifact.Provenance, *second.WorkspaceTrees[0].InputManifest) {
		t.Fatalf("the improved manifest lost its input provenance: %+v %v", artifact, err)
	}
	var manifest WorkspaceTreeManifest
	if err := decode(data, &manifest); err != nil || len(manifest.Files) != 1 {
		t.Fatalf("the improved plan is not a complete manifest: %+v %v", manifest, err)
	}
	_, captured, err := e.Artifact(manifest.Files[0].Ref)
	if err != nil || string(captured) != "# Improved\n" {
		t.Fatalf("the improved bytes were not the ones captured: %q %v", captured, err)
	}
	if third.WorkspaceTrees[0].InputManifest == nil {
		t.Fatalf("implement handoff lost the improved plan: %+v", third.WorkspaceTrees)
	}
}

// A location is refused only where it says something the runtime did not: a
// path other than the one the handoff named, or a missing name where the host
// genuinely chooses one. Each refusal names the entry it is about.
func TestWorkspaceTreeLocationRefusalsNameTheReportedEntry(t *testing.T) {
	exact := flow.WorkspaceTreeCapturePolicy{Kind: "exact_file", Path: ".ai-factory/PLAN.md"}
	t.Run("input location mismatch", func(t *testing.T) {
		e, runID := treeSessionFixture(t, exact)
		first := handOver(t, e, runID)
		writeWorkspaceTreeFile(t, first.RepositoryWorkspace, exact.Path, "# Original\n")
		if _, err := e.SubmitSession(context.Background(), treeSubmission(t, first, "plan", nil)); err != nil {
			t.Fatal(err)
		}
		if err := e.Drive(context.Background(), runID); err != nil {
			t.Fatal(err)
		}
		second, err := e.SessionTask(context.Background(), runID, "")
		if err != nil {
			t.Fatal(err)
		}
		_, err = e.SubmitSession(context.Background(), treeSubmission(t, second, "improved", []WorkspaceTreeLocation{{OutputPort: "improved", Path: ".ai-factory/OTHER.md"}}))
		problem, exit := ProblemFor(err)
		if problem.Code != "workspace_tree_input_location_mismatch" || exit != 2 {
			t.Fatalf("a different input location was not refused by name: %+v %v", problem, err)
		}
		if len(problem.Violations) != 1 || problem.Violations[0].Pointer != "/workspace_trees/0/path" {
			t.Fatalf("the refusal did not name the reported entry: %+v", problem.Violations)
		}
		if task, err := e.SessionTask(context.Background(), runID, ""); err != nil || task.AttemptID != second.AttemptID {
			t.Fatalf("a refused report closed the handoff: %+v %v", task, err)
		}
	})
	t.Run("chosen child name missing", func(t *testing.T) {
		e, runID := treeSessionFixture(t, flow.WorkspaceTreeCapturePolicy{Kind: "direct_child_file", Path: ".ai-factory/plans"})
		first := handOver(t, e, runID)
		writeWorkspaceTreeFile(t, first.RepositoryWorkspace, ".ai-factory/plans/feature.md", "# Original\n")
		_, err := e.SubmitSession(context.Background(), treeSubmission(t, first, "plan", nil))
		problem, _ := ProblemFor(err)
		if problem.Code != "workspace_tree_location_missing" {
			t.Fatalf("a policy the host chooses within accepted no name: %+v %v", problem, err)
		}
	})
}

// A plan left in the workspace by an earlier Run is not this step's output, so
// preparation refuses to claim it. The diagnostic says which refusal that was.
func TestExistingOutputFileIsRefusedByName(t *testing.T) {
	policy := flow.WorkspaceTreeCapturePolicy{Kind: "exact_file", Path: ".ai-factory/PLAN.md"}
	e, runID := treeSessionFixture(t, policy)
	claims, err := e.Claims(context.Background())
	if err != nil || len(claims.Claims) == 0 {
		t.Fatalf("no claimed workspace: %+v %v", claims, err)
	}
	workspace, err := e.claimWorkspacePath(claims.Claims[0])
	if err != nil {
		t.Fatal(err)
	}
	writeWorkspaceTreeFile(t, workspace, policy.Path, "# Left by an earlier run\n")
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	// The reader gets the path from the refusal itself: the pilot lost a Run to
	// this and could not tell from the envelope that a file was in the way.
	if len(r.Diagnostics) == 1 && !strings.Contains(r.Diagnostics[0].Message, "already exists in the workspace") {
		t.Fatalf("the refusal does not name the file that blocked the step: %q", r.Diagnostics[0].Message)
	}
	if r.Status != "failed" || len(r.Diagnostics) != 1 || r.Diagnostics[0].Code != "workspace_tree_output_exists" {
		t.Fatalf("an existing output file was not refused by name: status=%s diagnostics=%+v", r.Status, r.Diagnostics)
	}
	actual, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(policy.Path)))
	if err != nil || string(actual) != "# Left by an earlier run\n" {
		t.Fatalf("preparation touched the existing file: %q %v", actual, err)
	}
}

func treeWorkspace(t *testing.T, e *Engine) string {
	t.Helper()
	claims, err := e.Claims(context.Background())
	if err != nil || len(claims.Claims) != 1 {
		t.Fatalf("no single claimed workspace: %+v %v", claims, err)
	}
	workspace, err := e.claimWorkspacePath(claims.Claims[0])
	if err != nil {
		t.Fatal(err)
	}
	return workspace
}

// An exact-file policy names one fixed path and preparation refuses to start
// while anything occupies it, so a sealed capture left there makes the step
// unrepeatable in the workspace it was handed: the pilot deleted the same file
// by hand before every launch and warned a later commit step not to sweep it
// into history. Checkout mode is where that bites -- a worktree claim is a fresh
// directory, while a borrowed checkout carries the leftover into the next Run.
func TestSealedExactFileCaptureLeavesItsPathFreeForTheNextRun(t *testing.T) {
	policy := flow.WorkspaceTreeCapturePolicy{Kind: "exact_file", Path: ".ai-factory/PLAN.md"}
	e, runID := treeFixture(t, policy, nil, nil, "checkout", false)
	ctx := context.Background()
	workspace := treeWorkspace(t, e)
	last := ""
	for _, summary := range []string{"plan", "improve", "implement"} {
		task := handOver(t, e, runID)
		last = task.AttemptID
		writeWorkspaceTreeFile(t, workspace, policy.Path, "# "+summary+"\n")
		if _, err := e.SubmitSession(ctx, treeSubmission(t, task, summary, nil)); err != nil {
			t.Fatalf("%s report was refused: %v", summary, err)
		}
	}
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatal(err)
	}
	completed := driverRun(t, e, runID)
	if completed.Status != "completed" {
		t.Fatalf("the tree run did not settle: %s %+v", completed.Status, completed.Diagnostics)
	}
	if _, err := os.Lstat(filepath.Join(workspace, filepath.FromSlash(policy.Path))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the sealed capture still occupies its declared path: %v", err)
	}
	// Removing the file loses nothing: the manifest this step reported still
	// resolves to the exact bytes the host wrote.
	var manifest WorkspaceTreeManifest
	_, data, err := e.Artifact(completed.Attempts[last].Accepted.Outputs["final"])
	if err != nil || decode(data, &manifest) != nil || len(manifest.Files) != 1 {
		t.Fatalf("the final manifest was not sealed: %+v %v", manifest, err)
	}
	if _, sealed, err := e.Artifact(manifest.Files[0].Ref); err != nil || string(sealed) != "# implement\n" {
		t.Fatalf("the captured bytes did not survive the cleanup: %q %v", sealed, err)
	}
	// The whole point: the next launch over the same checkout runs unattended.
	claim, err := e.ClaimWorktree(ctx, ClaimRequest{CommandID: newID("command"), Repository: workspace, OwnerID: "session:pilot", WorkspaceMode: "checkout"})
	if err != nil {
		t.Fatalf("the settled Run left the checkout unclaimable: %v", err)
	}
	if claim.Status != "active" {
		t.Fatalf("the next claim is not active: %+v", claim)
	}
	started, err := e.Start(ctx, StartOptions{CommandID: newID("command"), WorkflowFile: "workflows/tree.json", BriefFile: "brief.json", Inputs: map[string]string{}, WorkspaceMode: "checkout"})
	if err != nil {
		t.Fatal(err)
	}
	first := handOver(t, e, started.Receipt.RunID)
	if len(first.WorkspaceTrees) != 1 || first.WorkspaceTrees[0].InputManifest != nil {
		t.Fatalf("the next Run did not reach its output-only tree step: %+v", first)
	}
}

// A tracked file at the declared path is the repository's own document. The
// runtime seals its bytes and leaves it exactly where git has it: cleaning up
// after a capture must never turn into deleting from a borrowed checkout.
func TestSealedExactFileCaptureKeepsATrackedDocument(t *testing.T) {
	policy := flow.WorkspaceTreeCapturePolicy{Kind: "exact_file", Path: ".ai-factory/PLAN.md"}
	e, runID := treeSessionFixture(t, policy)
	ctx := context.Background()
	workspace := treeWorkspace(t, e)
	task := handOver(t, e, runID)
	writeWorkspaceTreeFile(t, workspace, policy.Path, "# Tracked\n")
	if _, err := e.git(ctx, workspace, "add", "--", ":(literal)"+policy.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := e.SubmitSession(ctx, treeSubmission(t, task, "plan", nil)); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(policy.Path)))
	if err != nil || string(actual) != "# Tracked\n" {
		t.Fatalf("the runtime removed a tracked document: %q %v", actual, err)
	}
}

// A read-only verify step is handed the plan implement captured, through a
// binding with no output port: materialized into the Run's claim before its
// mark is taken, readable at the declared location, never captured, and taken
// back at settlement only where the engine placed it.
func TestMaterializeOnlyTreeHandsAReadOnlyStepTheCapturedPlan(t *testing.T) {
	policies := map[string]struct {
		policy flow.WorkspaceTreeCapturePolicy
		files  map[string]string
	}{
		"fast":  {flow.WorkspaceTreeCapturePolicy{Kind: "exact_file", Path: ".ai-factory/PLAN.md"}, map[string]string{".ai-factory/PLAN.md": "# Final\n"}},
		"ultra": {flow.WorkspaceTreeCapturePolicy{Kind: "direct_child_tree", Path: ".ai-factory/plans", Entrypoint: "index.md"}, map[string]string{".ai-factory/plans/feature/index.md": "# Final\n", ".ai-factory/plans/feature/phase.md": "# Phase\n"}},
	}
	for name, test := range policies {
		for _, kept := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/kept=%t", name, kept), func(t *testing.T) {
				e, runID := treeVerifyFixture(t, test.policy)
				location := test.policy.Path
				if test.policy.Kind == "direct_child_tree" {
					location = ".ai-factory/plans/feature"
				}
				first := handOver(t, e, runID)
				for path, value := range test.files {
					writeWorkspaceTreeFile(t, first.RepositoryWorkspace, path, value)
				}
				if _, err := e.SubmitSession(context.Background(), treeSubmission(t, first, "plan", []WorkspaceTreeLocation{{OutputPort: "plan", Path: location}})); err != nil {
					t.Fatal(err)
				}
				for _, summary := range []string{"improved", "implement"} {
					if err := e.Drive(context.Background(), runID); err != nil {
						t.Fatal(err)
					}
					task, err := e.SessionTask(context.Background(), runID, "")
					if err != nil {
						t.Fatal(err)
					}
					if _, err := e.SubmitSession(context.Background(), treeSubmission(t, task, summary, nil)); err != nil {
						t.Fatal(err)
					}
				}
				if !kept {
					// The tree implement captured is gone from the workspace: the
					// engine has to put it back for the read-only step.
					for path := range test.files {
						if err := os.RemoveAll(filepath.Join(first.RepositoryWorkspace, filepath.FromSlash(path))); err != nil {
							t.Fatal(err)
						}
					}
				}
				if err := e.Drive(context.Background(), runID); err != nil {
					t.Fatal(err)
				}
				verify, err := e.SessionTask(context.Background(), runID, "")
				if err != nil {
					t.Fatal(err)
				}
				if verify.RepositoryWorkspace != first.RepositoryWorkspace || len(verify.WorkspaceTrees) != 1 || verify.WorkspaceTrees[0].OutputPort != "" || verify.WorkspaceTrees[0].InputManifest == nil || verify.WorkspaceTrees[0].InputLocation != location {
					t.Fatalf("the read-only step was not handed the materialized tree in the Run's claim: %+v", verify)
				}
				// An exact file is freed from the tree once captured (see
				// freeSealedExactFile), so the engine places it again whether or
				// not the test removed it; a bundle stays and is placed only when gone.
				expectPlaced := !kept || test.policy.Kind == "exact_file"
				if placed := len(verify.WorkspaceTrees[0].MaterializedEntries); expectPlaced && placed != len(test.files) || !expectPlaced && placed != 0 {
					t.Fatalf("materialized entries do not say what the engine placed (kept=%t): %v", kept, verify.WorkspaceTrees[0].MaterializedEntries)
				}
				for path, value := range test.files {
					actual, err := os.ReadFile(filepath.Join(verify.RepositoryWorkspace, filepath.FromSlash(path)))
					if err != nil || string(actual) != value {
						t.Fatalf("materialized bytes differ for %s: %q %v", path, actual, err)
					}
				}
				var guide WorkspaceTreeGuide
				if data, err := os.ReadFile(filepath.Join(verify.Workspace, WorkspaceTreeGuideFile)); err != nil || json.Unmarshal(data, &guide) != nil || guide.SchemaVersion != WorkspaceTreeGuideVersion || len(guide.Ports) != 1 || guide.Ports[0].OutputPort != "" || guide.Ports[0].InputPort != "plan" || !strings.Contains(string(data), "reading only") {
					t.Fatalf("the guide does not describe a materialize-only port: %s %v", data, err)
				}
				if _, err := e.SubmitSession(context.Background(), treeSubmission(t, verify, "verified", nil)); err != nil {
					t.Fatalf("an untouched materialized tree was refused: %v", err)
				}
				if err := e.Drive(context.Background(), runID); err != nil {
					t.Fatal(err)
				}
				r := driverRun(t, e, runID)
				if r.SchemaVersion != CoreStageWorkStateVersion || r.Status != "completed" || len(r.Attempts[verify.AttemptID].Accepted.Outputs) != 0 {
					t.Fatalf("the read-only step did not settle without an output: %+v", r)
				}
				for path := range test.files {
					_, err := os.Lstat(filepath.Join(verify.RepositoryWorkspace, filepath.FromSlash(path)))
					if !expectPlaced && err != nil || expectPlaced && !os.IsNotExist(err) {
						t.Fatalf("after settlement %s should be present=%t: %v", path, !expectPlaced, err)
					}
				}
				if expectPlaced {
					if _, err := os.Lstat(filepath.Join(verify.RepositoryWorkspace, ".ai-factory")); !os.IsNotExist(err) {
						t.Fatalf("the parent left empty by the removal was left behind: %v", err)
					}
				}
			})
		}
	}
}

// The mark of a read-only step is taken after the engine materialized its
// tree, so a host that edits the materialized entry is refused as one that
// changed the workspace, and putting the bytes back lets it report again.
func TestMaterializeOnlyTreeRefusesAHostThatEditsIt(t *testing.T) {
	policy := flow.WorkspaceTreeCapturePolicy{Kind: "exact_file", Path: ".ai-factory/PLAN.md"}
	e, runID := treeVerifyFixture(t, policy)
	first := handOver(t, e, runID)
	writeWorkspaceTreeFile(t, first.RepositoryWorkspace, policy.Path, "# Final\n")
	if _, err := e.SubmitSession(context.Background(), treeSubmission(t, first, "plan", []WorkspaceTreeLocation{{OutputPort: "plan", Path: policy.Path}})); err != nil {
		t.Fatal(err)
	}
	for _, summary := range []string{"improved", "implement"} {
		if err := e.Drive(context.Background(), runID); err != nil {
			t.Fatal(err)
		}
		task, err := e.SessionTask(context.Background(), runID, "")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.SubmitSession(context.Background(), treeSubmission(t, task, summary, nil)); err != nil {
			t.Fatal(err)
		}
	}
	// The captured exact file was freed from the tree, so verify's copy is
	// the engine's own materialization.
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	verify, err := e.SessionTask(context.Background(), runID, "")
	if err != nil {
		t.Fatal(err)
	}
	writeWorkspaceTreeFile(t, verify.RepositoryWorkspace, policy.Path, "# Edited by the host\n")
	var rejection *local.Rejection
	if _, err := e.SubmitSession(context.Background(), treeSubmission(t, verify, "verified", nil)); err == nil || !asRejection(err, &rejection) || rejection.Code != "effect_not_permitted" || !strings.Contains(rejection.Message, policy.Path) {
		t.Fatalf("an edited materialized entry was not refused by name: %v", err)
	}
	writeWorkspaceTreeFile(t, verify.RepositoryWorkspace, policy.Path, "# Final\n")
	if _, err := e.SubmitSession(context.Background(), treeSubmission(t, verify, "verified", nil)); err != nil {
		t.Fatalf("the restored tree was still refused: %v", err)
	}
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	if r := driverRun(t, e, runID); r.Status != "completed" {
		t.Fatalf("the Run did not complete after the restored report: %+v", r)
	}
	if _, err := os.Lstat(filepath.Join(verify.RepositoryWorkspace, filepath.FromSlash(policy.Path))); !os.IsNotExist(err) {
		t.Fatalf("the materialized entry was not taken back: %v", err)
	}
}

// The read view of a stopped Run names what stopped it, and a completed Run
// names nothing: the reason used to live only somewhere in diagnostics[].
func TestRunViewNamesTheFailureOfAStoppedRun(t *testing.T) {
	policy := flow.WorkspaceTreeCapturePolicy{Kind: "exact_file", Path: ".ai-factory/PLAN.md"}
	e, runID := treeSessionFixture(t, policy)
	task := handOver(t, e, runID)
	if _, err := e.Restrict(context.Background(), RestrictCommand{SchemaVersion: "1", CommandID: newID("command"), Scope: "run", ScopeID: runID, Kind: "cancel", Reason: "the owner stops the probe"}); err != nil {
		t.Fatal(err)
	}
	// A cancel that meets an outstanding handoff leaves the Run uncertain:
	// the handed attempt is an obligation the owner closes, and only then
	// does the Run settle. The first drive reports that it needs recovery.
	_ = e.Drive(context.Background(), runID)
	stopped, err := e.View(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if stopped.Run.Status == "uncertain" {
		if _, err := e.ResolveObligation(context.Background(), runID, newID("command"), task.AttemptID, "", "not_applied", "the host never worked the handoff", stopped.RunVersion); err != nil {
			t.Fatal(err)
		}
		if err := e.Drive(context.Background(), runID); err != nil {
			t.Fatal(err)
		}
	}
	view, err := e.View(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	// A resolution of not_applied ends the Run failed rather than cancelled
	// (see examples/troubleshooting.md); either way the view names the stop.
	if view.SchemaVersion != CoreStageWorkReadVersion || view.Run.Status != "failed" && view.Run.Status != "cancelled" || view.Failure == nil || view.Failure.DiagnosticID == "" {
		t.Fatalf("a stopped Run does not name its failure: %+v %+v", view.Run.Status, view.Failure)
	}
	last := view.Run.Diagnostics[len(view.Run.Diagnostics)-1]
	if view.Failure.Code != last.Code || view.Failure.DiagnosticID != last.ID || view.Failure.AttemptID != last.AttemptID {
		t.Fatalf("failure does not name the diagnostic that stopped the Run: %+v vs %+v", view.Failure, last)
	}
	if view.Failure.AttemptID != "" && view.Failure.StepInstanceID != view.Run.Attempts[view.Failure.AttemptID].StepID {
		t.Fatalf("failure names the wrong step: %+v", view.Failure)
	}
	data, err := json.Marshal(view)
	if err != nil || !strings.Contains(string(data), `"failure":{`) {
		t.Fatalf("failure is not on the wire: %v", err)
	}
}

// The answer about the next action names what the driver would do there, so a
// host learns that a program is next without starting it: a dependent session
// chained run drive --next after a submit and ran a seventeen-minute test
// program inside its own harness.
func TestNextNamesTheWorkAReadyStageHolds(t *testing.T) {
	e, runID := treeVerifyFixture(t, flow.WorkspaceTreeCapturePolicy{Kind: "exact_file", Path: ".ai-factory/PLAN.md"})
	next, err := e.Next(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if next.SchemaVersion != CoreProgramEnvironmentNextVersion || next.Action != "stage" || next.StageWork != StageWorkAssistedSession {
		t.Fatalf("a ready assisted stage is not named as one: %+v", next)
	}
	// Reading names the work; it does not do it.
	r := driverRun(t, e, runID)
	if len(r.Attempts) != 0 {
		t.Fatalf("reading the next action admitted work: %+v", r.Attempts)
	}
	task := handOver(t, e, runID)
	handed, err := e.Next(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if handed.Action != "idle" || handed.StageWork != "" || !slices.Contains(handed.SafeNextActions, "session.task") {
		t.Fatalf("a handed-out step reports a stage work or hides its handoff: %+v", handed)
	}
	writeWorkspaceTreeFile(t, task.RepositoryWorkspace, ".ai-factory/PLAN.md", "# Plan\n")
	if _, err := e.SubmitSession(context.Background(), treeSubmission(t, task, "plan", []WorkspaceTreeLocation{{OutputPort: "plan", Path: ".ai-factory/PLAN.md"}})); err != nil {
		t.Fatal(err)
	}
	after, err := e.Next(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Action != "stage" || after.StageWork != StageWorkAssistedSession {
		t.Fatalf("the next assisted stage lost its name: %+v", after)
	}
}

// A control stage is work the driver does alone, and a step this build cannot
// classify is named nothing rather than guessed at.
func TestStageWorkNamesControlAndStaysSilentWhenItCannotTell(t *testing.T) {
	e, runID := treeSessionFixture(t, flow.WorkspaceTreeCapturePolicy{Kind: "exact_file", Path: ".ai-factory/PLAN.md"})
	r := driverRun(t, e, runID)
	plan, err := r.planFor(r.RootInvocationID)
	if err != nil {
		t.Fatal(err)
	}
	if work := e.stageWork(r, r.RootInvocationID, "done"); work != StageWorkControl {
		t.Fatalf("a finish stage is not control work: %q", work)
	}
	if work := e.stageWork(r, r.RootInvocationID, "no-such-stage"); work != "" {
		t.Fatalf("an unknown stage was named: %q", work)
	}
	if work := e.stageWork(r, "no-such-invocation", "plan"); work != "" {
		t.Fatalf("an unreadable plan was named: %q", work)
	}
	step := plan.Steps["plan"]
	if step.Executor.Operation != "session" {
		t.Fatalf("the fixture stopped using an assisted plan step: %+v", step.Executor)
	}
}
