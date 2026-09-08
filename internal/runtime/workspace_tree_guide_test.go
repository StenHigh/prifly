package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

// The executor learns the shape of the result before producing it. A guide
// carrying one path would repeat the manifest's mistake in a new place, so the
// declared policy is copied whole: a tree with an entrypoint is not a file.
func TestGuideCopiesTheDeclaredPolicyWhole(t *testing.T) {
	policies := []flow.WorkspaceTreeCapturePolicy{
		{Kind: "exact_file", Path: ".ai-factory/PLAN.md"},
		{Kind: "direct_child_file", Path: ".ai-factory/plans"},
		{Kind: "direct_child_tree", Path: ".ai-factory/bundles", Entrypoint: "index.md"},
	}
	step := flow.StepDefinition{}
	manifest := ContextManifest{Outputs: map[string]OutputSlot{}}
	for index, policy := range policies {
		port := []string{"plan", "full", "ultra"}[index]
		step.WorkspaceTrees = append(step.WorkspaceTrees, flow.WorkspaceTreeBinding{InputPort: port + "_in", OutputPort: port, Capture: policy})
		manifest.Outputs[port] = OutputSlot{ArtifactID: "artifact:" + port, Revision: 1, Path: "outputs/" + port}
	}
	guide, declared := workspaceTreeGuide(step, manifest)
	if !declared || len(guide.Ports) != len(policies) {
		t.Fatalf("the guide did not describe every declared port: %+v", guide)
	}
	for index, shown := range guide.Ports {
		if shown.Capture != policies[index] {
			t.Fatalf("port %s shows a summary, not the declaration: %+v", shown.OutputPort, shown.Capture)
		}
		if shown.InputPort != shown.OutputPort+"_in" {
			t.Fatalf("port %s lost the input it is bound to: %+v", shown.OutputPort, shown)
		}
		if shown.EngineSlotPath != "outputs/"+shown.OutputPort {
			t.Fatalf("port %s does not name the address the manifest prints: %q", shown.OutputPort, shown.EngineSlotPath)
		}
	}
	// The entrypoint is the field an executor cannot infer, and the one a
	// path-only guide would have dropped.
	if guide.Ports[2].Capture.Entrypoint != "index.md" {
		t.Fatalf("the tree policy lost its entrypoint: %+v", guide.Ports[2].Capture)
	}
}

// context.json shows a captured port an output slot like any other, and that
// slot is real: capture requires it and the engine writes the sealed manifest
// there. So the guide cannot say the slot does not exist — it has to say whose
// it is, and name both refusals an executor earns by treating it as its own.
func TestGuideSaysWhoTheSlotBelongsTo(t *testing.T) {
	step := flow.StepDefinition{WorkspaceTrees: []flow.WorkspaceTreeBinding{{OutputPort: "plan", Capture: flow.WorkspaceTreeCapturePolicy{Kind: "exact_file", Path: ".ai-factory/PLAN.md"}}}}
	manifest := ContextManifest{Outputs: map[string]OutputSlot{"plan": {Path: "outputs/plan"}}}
	guide, _ := workspaceTreeGuide(step, manifest)
	for _, expected := range []string{"belongs to the engine", "not yours to fill", "workspace_tree_output_host_supplied", "workspace_tree_capture_conflict"} {
		if !strings.Contains(guide.Note, expected) {
			t.Fatalf("the guide does not name %q, so a reader still earns the refusal by following the manifest: %q", expected, guide.Note)
		}
	}
	data, err := json.Marshal(guide)
	if err != nil || !json.Valid(data) {
		t.Fatalf("the guide is not readable as JSON: %v", err)
	}
}

// A step that declares no capture gets no second document. An empty guide is
// one more file to open that answers nothing.
func TestStepWithoutCaptureGetsNoGuide(t *testing.T) {
	if _, declared := workspaceTreeGuide(flow.StepDefinition{}, ContextManifest{}); declared {
		t.Fatal("a step with no declared capture was handed a guide anyway")
	}
}

// The guide is only useful beside the document it corrects. An assisted handoff
// names two directories — the claimed repository worktree the executor works in,
// and the directory holding the step manifest — and the guide belongs with the
// manifest, because the reader it argues with is reading that file.
func TestGuideSitsBesideTheManifestItCorrects(t *testing.T) {
	policy := flow.WorkspaceTreeCapturePolicy{Kind: "direct_child_tree", Path: ".ai-factory/plans", Entrypoint: "index.md"}
	e, runID := treeSessionFixture(t, policy)
	task := handOver(t, e, runID)
	if task.Workspace == "" {
		t.Fatal("the handoff names no manifest workspace")
	}
	if _, err := os.Stat(filepath.Join(task.Workspace, "context.json")); err != nil {
		t.Fatalf("the manifest is not where this test expects it: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(task.Workspace, WorkspaceTreeGuideFile))
	if err != nil {
		t.Fatalf("no guide beside the manifest the executor reads: %v", err)
	}
	var guide WorkspaceTreeGuide
	if err := json.Unmarshal(data, &guide); err != nil {
		t.Fatalf("the guide is not readable: %v", err)
	}
	if len(guide.Ports) != 1 || guide.Ports[0].Capture != policy {
		t.Fatalf("the guide does not describe this step: %+v", guide)
	}
	if guide.Ports[0].Capture.Entrypoint != "index.md" {
		t.Fatalf("the entrypoint an executor cannot infer was dropped: %+v", guide.Ports[0])
	}
}
