package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// A step permitted only its declared output slot used to be trusted about it:
// permitted_effects named the boundary and nothing measured it. A dependent
// session, running verify on a battle Run, wrote a patch into the claimed
// checkout, noticed, reverted it by hand to stay honest, and paid with a
// second test run -- the engine would not have noticed had they left it. Now
// the handoff records how each workspace the Run holds stood, and a report
// that left one changed is refused with the paths named, keeping the handoff
// awaiting so the executor can put it back.
func effectsFixture(t *testing.T) (*Engine, string, WorktreeClaim) {
	t.Helper()
	e, runID, claim := assistedWorkspaceFixture(t, "checkout")
	// Rewrite the workflow: the write step hands over to a verify step that
	// declares no workspace effect, whose report must leave the checkout alone.
	definitions, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	var registry RegistryFile
	readRuntimeJSON(t, filepath.Join(e.Root, e.Config.Configuration.RegistryFile), &registry)
	var planStep flow.StepDefinition
	var planRef flow.Ref
	for _, entry := range registry.Entries {
		if entry.Kind == "step" {
			readRuntimeJSON(t, filepath.Join(e.Root, entry.Path), &planStep)
			planRef = entry.Ref
		}
	}
	verify := planStep
	verify.ID, verify.Title = "aif:step/verify", "Verify without touching the checkout"
	verify.Effects.Class = "none"
	verifyBytes := writeRegistryDocument(t, e, "steps/verify.json", verify)
	verifyRef := flow.Ref{ID: verify.ID, Version: verify.Version, Digest: rawDigest(verifyBytes)}
	registry.Entries = append(registry.Entries, Definition{Ref: verifyRef, Kind: "step", Path: "steps/verify.json"})
	writeRuntimeJSON(t, filepath.Join(e.Root, e.Config.Configuration.RegistryFile), registry)

	workflow := flow.WorkflowRevision{
		SchemaVersion: "1", ID: "aif:workflow/pilot-verify", Version: "1.0.0", Title: "Plan, then verify",
		Inputs: map[string]flow.InputPort{}, Outputs: map[string]flow.OutputPort{}, AllowedOutcomes: []string{"succeeded"},
		Limits: flow.Limits{MaxStepInstances: 4, MaxControlTransitions: 16, MaxParallelism: 1}, PolicyRef: builtinVersionRef(definitions, "core:policy/local", "2.0.0"),
	}
	workflow.Definition.Entry = "plan"
	workflow.Definition.Stages = map[string]flow.Stage{
		"plan":   {Kind: "step", StepRef: planRef, InputBindings: map[string]flow.Binding{}, On: map[string]string{"pass": "verify"}},
		"verify": {Kind: "step", StepRef: verifyRef, InputBindings: map[string]flow.Binding{}, On: map[string]string{"pass": "done"}},
		"done":   {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]flow.Binding{}},
	}
	writeRuntimeJSON(t, filepath.Join(e.Root, "workflows/pilot-verify.json"), workflow)
	_ = runID
	result, err := e.Start(context.Background(), StartOptions{CommandID: newID("command"), WorkflowFile: "workflows/pilot-verify.json", BriefFile: "brief.json", Inputs: map[string]string{}, WorkspaceMode: "checkout"})
	if err != nil {
		t.Fatal(err)
	}
	return e, result.Receipt.RunID, claim
}

func TestAReportThatChangedTheWorkspaceWithoutPermissionIsRefused(t *testing.T) {
	e, runID, claim := effectsFixture(t)
	ctx := context.Background()

	// The write step does its work and is accepted; it is never measured.
	planTask := handOver(t, e, runID)
	if _, err := e.SubmitSession(ctx, hostResult(t, e, planTask, "planned")); err != nil {
		t.Fatalf("the write step's report was refused: %v", err)
	}

	// The verify step is handed the checkout as it stands.
	verifyTask := handOver(t, e, runID)
	r := driverRun(t, e, runID)
	verifyAttempt := r.Attempts[verifyTask.AttemptID]
	if verifyAttempt.Session == nil || len(verifyAttempt.Session.WorkspaceMarks) != 1 {
		t.Fatalf("the verify handoff recorded no workspace mark: %+v", verifyAttempt.Session)
	}
	if _, recorded := verifyAttempt.Session.WorkspaceMarks[claim.ID]; !recorded {
		t.Fatalf("the mark is not keyed by the Run's claim: %v", verifyAttempt.Session.WorkspaceMarks)
	}
	planAttempt := r.Attempts[planTask.AttemptID]
	if len(planAttempt.Session.WorkspaceMarks) != 0 {
		t.Fatalf("a write step was measured as if it could not write: %v", planAttempt.Session.WorkspaceMarks)
	}

	// The executor writes a patch into the checkout, which verify may not do.
	patch := filepath.Join(claim.Path, "src", "patched.txt")
	if err := os.MkdirAll(filepath.Dir(patch), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(patch, []byte("a fix that belongs in the fix step\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := e.SubmitSession(ctx, hostResult(t, e, verifyTask, "verified"))
	var rejection *local.Rejection
	if err == nil || !asRejection(err, &rejection) || rejection.Code != "effect_not_permitted" {
		t.Fatalf("a report that changed the checkout was accepted or refused otherwise: %v", err)
	}
	if !strings.Contains(rejection.Message, "src/patched.txt") || !strings.Contains(rejection.Message, claim.Path) {
		t.Fatalf("the refusal does not name the path that moved or the workspace: %s", rejection.Message)
	}
	if !strings.Contains(rejection.Message, "workspace_write") {
		t.Fatalf("the refusal does not name the exit that declares the effect: %s", rejection.Message)
	}
	// The handoff is still awaiting: the executor can put the checkout back.
	if after := driverRun(t, e, runID).Attempts[verifyTask.AttemptID]; after.Session.HostState != SessionAwaiting {
		t.Fatalf("a refused report settled the handoff: %s", after.Session.HostState)
	}

	// Reverting the patch makes the same report acceptable.
	if err := os.Remove(patch); err != nil {
		t.Fatal(err)
	}
	if _, err := e.SubmitSession(ctx, hostResult(t, e, verifyTask, "verified")); err != nil {
		t.Fatalf("the report was refused after the checkout was put back: %v", err)
	}
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatal(err)
	}
	if final := driverRun(t, e, runID); final.Status != "completed" {
		t.Fatalf("the Run did not complete after an honest verify: %s", final.Status)
	}
}

func asRejection(err error, target **local.Rejection) bool {
	for err != nil {
		if r, ok := err.(*local.Rejection); ok {
			*target = r
			return true
		}
		unwrap, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = unwrap.Unwrap()
	}
	return false
}

func readRuntimeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, value); err != nil {
		t.Fatal(err)
	}
}
