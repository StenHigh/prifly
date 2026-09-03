package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	return treeFixture(t, policy, nil, nil)
}

func treeDecisionSessionFixture(t *testing.T, policy flow.WorkspaceTreeCapturePolicy) (*Engine, string) {
	t.Helper()
	catalog, sheet := treeDecisionInputs(t)
	return treeFixture(t, policy, catalog, sheet)
}

func treeFixture(t *testing.T, policy flow.WorkspaceTreeCapturePolicy, catalog *DecisionCatalog, sheet *DecisionSheet) (*Engine, string) {
	t.Helper()
	e := contextRegistryRuntime(t)
	claim, err := e.ClaimWorktree(context.Background(), ClaimRequest{CommandID: "command:tree-claim", Repository: gitRepository(t), OwnerID: "session:pilot", WorkspaceMode: "worktree"})
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
	workflow := flow.WorkflowRevision{SchemaVersion: "1", ID: "test:workflow/tree", Version: "1.0.0", Title: "Native tree", Inputs: map[string]flow.InputPort{}, Outputs: map[string]flow.OutputPort{}, AllowedOutcomes: []string{"succeeded"}, Limits: flow.Limits{MaxStepInstances: 4, MaxControlTransitions: 32, MaxParallelism: 1}, PolicyRef: builtinVersionRef(definitions, "core:policy/local", "2.0.0")}
	workflow.Definition.Entry = "plan"
	workflow.Definition.Stages = map[string]flow.Stage{
		"plan":      {Kind: "step", StepRef: planRef, InputBindings: map[string]flow.Binding{}, On: map[string]string{"pass": "improve"}},
		"improve":   {Kind: "step", StepRef: improveRef, InputBindings: map[string]flow.Binding{"plan": {From: "stage_output", StageID: "plan", Port: "plan"}}, On: map[string]string{"pass": "implement"}},
		"implement": {Kind: "step", StepRef: implementRef, InputBindings: map[string]flow.Binding{"plan": {From: "stage_output", StageID: "improve", Port: "improved"}}, On: map[string]string{"pass": "done"}},
		"done":      {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]flow.Binding{}},
	}
	writeRuntimeJSON(t, filepath.Join(e.Root, "workflows/tree.json"), workflow)
	registry := RegistryFile{SchemaVersion: "3", Entries: []Definition{{Ref: skillRef, Kind: "resource", Path: "resources/tree-skill.md", ByteEncoding: "utf8_text", MediaType: "text/markdown; charset=utf-8"}, {Ref: planRef, Kind: "step", Path: "steps/tree-plan.json"}, {Ref: improveRef, Kind: "step", Path: "steps/tree-improve.json"}, {Ref: implementRef, Kind: "step", Path: "steps/tree-implement.json"}}}
	writeRuntimeJSON(t, filepath.Join(e.Root, e.Config.Configuration.RegistryFile), registry)
	e.Config.Configuration.SchemaVersion, e.Config.ConfigurationSchemaRef = CoreContextConfigVersion, builtinVersionRef(definitions, "core:schema/core-configuration", "2.0.0")
	e.Config.AdapterBindings["local_process"], e.Config.DefaultPolicyRef = builtinVersionRef(definitions, "core:adapter/local-process", "2.0.0"), builtinVersionRef(definitions, "core:policy/local", "2.0.0")
	writeRuntimeJSON(t, filepath.Join(e.Root, "prifly.json"), e.Config)
	writeRuntimeJSON(t, filepath.Join(e.Root, "brief.json"), Brief{"1", "test:brief/tree", "tree", "native plan", []string{"Native plan"}, []string{}, []string{"Seal and improve the native plan"}, []ArtifactRef{}, []string{}, "explicit"})
	started, err := e.Start(context.Background(), StartOptions{CommandID: newID("command"), WorkflowFile: "workflows/tree.json", BriefFile: "brief.json", Inputs: map[string]string{}, DecisionCatalog: catalog, DecisionSheet: sheet, WorkspaceMode: "worktree"})
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
			if first.SchemaVersion != AssistedSessionTreeVersion || len(first.WorkspaceTrees) != 1 || first.WorkspaceTrees[0].InputManifest != nil {
				t.Fatalf("first tree handoff is not output-only v4: %+v", first)
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
			if second.SchemaVersion != AssistedSessionTreeVersion || len(second.WorkspaceTrees) != 1 || second.WorkspaceTrees[0].InputManifest == nil {
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
			if third.SchemaVersion != AssistedSessionTreeVersion || len(third.WorkspaceTrees) != 1 || third.WorkspaceTrees[0].InputManifest == nil {
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
			if r.SchemaVersion != CoreWorkspaceTreeStateVersion || r.Status != "completed" {
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
func TestWorkspaceTreeCaptureFollowsDeclaredBindingsAtDecisionSessionVersion(t *testing.T) {
	policy := flow.WorkspaceTreeCapturePolicy{Kind: "exact_file", Path: ".ai-factory/PLAN.md"}
	e, runID := treeDecisionSessionFixture(t, policy)
	first := handOver(t, e, runID)
	if first.SchemaVersion != AssistedSessionDecisionVersion || len(first.WorkspaceTrees) != 1 || first.WorkspaceTrees[0].InputManifest != nil {
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
	if r.Status != "failed" || len(r.Diagnostics) != 1 || r.Diagnostics[0].Code != "workspace_tree_output_exists" {
		t.Fatalf("an existing output file was not refused by name: status=%s diagnostics=%+v", r.Status, r.Diagnostics)
	}
	actual, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(policy.Path)))
	if err != nil || string(actual) != "# Left by an earlier run\n" {
		t.Fatalf("preparation touched the existing file: %q %v", actual, err)
	}
}
