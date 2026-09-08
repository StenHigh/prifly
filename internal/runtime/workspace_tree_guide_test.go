package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
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
	guide, declared := workspaceTreeGuide(step)
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
	guide, _ := workspaceTreeGuide(step)
	for _, expected := range []string{"lists no output slot", "workspace_tree_output_host_supplied", "workspace_tree_capture_conflict"} {
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
	if _, declared := workspaceTreeGuide(flow.StepDefinition{}); declared {
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

// Three documents describe one step's outputs, and until this they disagreed:
// the guide and the submission template left a captured port out, while
// context.json offered it a slot like any other. Slots are the natural place an
// executor starts — they are what "where do I write the result" means — so
// starting there earned workspace_tree_output_host_supplied for doing the
// obvious. Reading order was the only thing that saved anyone.
func TestCapturedPortGetsNoSlotWhileOrdinaryPortsKeepTheirs(t *testing.T) {
	policy := flow.WorkspaceTreeCapturePolicy{Kind: "exact_file", Path: ".ai-factory/PLAN.md"}
	e, runID := treeSessionFixture(t, policy)
	task := handOver(t, e, runID)
	if _, offered := task.Context.Outputs["plan"]; offered {
		t.Fatalf("the captured port is still offered a slot to write to: %+v", task.Context.Outputs)
	}
	// This step's only output is the captured one, so outputs is an empty map —
	// not a missing key and not a refusal. That shape is legal in the model and
	// nobody had it in production, so it is asserted here rather than guessed.
	if task.Context.Outputs == nil || len(task.Context.Outputs) != 0 {
		t.Fatalf("a step whose only output is captured did not produce an empty map: %+v", task.Context.Outputs)
	}
	// A step with no capture keeps every slot and every address: this is the
	// change that could lose a Run's output rather than refuse it, so it is
	// asserted on its own fixture.
	plain, plainRun, _ := assistedFixture(t)
	ordinary := handOver(t, plain, plainRun)
	slot, kept := ordinary.Context.Outputs["plan"]
	if !kept || slot.Path != "outputs/plan" {
		t.Fatalf("an ordinary port lost its slot: %+v", ordinary.Context.Outputs)
	}
	if slot.ArtifactID != outputArtifactID(ordinary.AttemptID, "plan") || slot.Revision != 1 {
		t.Fatalf("an ordinary port's identity moved: %+v", slot)
	}
	// The submission template already left the port out; the guide still names
	// it. All three now say the same, so reading order stops mattering.
	skeleton, err := task.SubmissionTemplate()
	if err != nil {
		t.Fatal(err)
	}
	var reported Result
	if err := json.Unmarshal(skeleton.Result, &reported); err != nil {
		t.Fatal(err)
	}
	if _, offered := reported.Outputs["plan"]; offered {
		t.Fatalf("the template offers a captured port: %+v", reported.Outputs)
	}
	guide, declared := workspaceTreeGuide(flow.StepDefinition{WorkspaceTrees: []flow.WorkspaceTreeBinding{{OutputPort: "plan", Capture: policy}}})
	if !declared || len(guide.Ports) != 1 || guide.Ports[0].OutputPort != "plan" {
		t.Fatalf("the guide stopped naming the port the other two omit: %+v", guide)
	}
}

// A refusal that names an obstacle without naming what lifts it costs the
// reader the same search every time. Both of these were found by a session that
// spent three attempts taking the expected version from the wrong level of one
// document, and one more discovering that a stop is released by name.
func TestStopAndVersionRefusalsPointAtWhereTheAnswerIsRead(t *testing.T) {
	for code, want := range map[string]string{"active_stop": "run.release", "version_conflict": "run.status", "recovery_required": "run.resolve"} {
		problem, _ := ProblemFor(&flow.Problem{Code: code, Message: "x"})
		if !slices.Contains(problem.SafeNextActions, want) {
			t.Fatalf("%s does not offer %s: %+v", code, want, problem.SafeNextActions)
		}
	}
}
