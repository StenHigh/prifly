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

// A program step is handed the Run's claimed workspace the way a host is --
// PRIFLY_REPOSITORY_WORKSPACE beside the socket and the context file -- and a
// program permitted no workspace effect is held to the same mark: a tests
// program has to run the project's suite in the tree the write steps produced,
// and until 0.13.25 it had no path but a guess from claim naming, and nothing
// measured what it did there.
func TestAProgramStepIsHandedTheWorkspaceAndHeldToItsEffects(t *testing.T) {
	for _, test := range []struct {
		mode, failure string
	}{
		{"workspace-read", ""},
		{"workspace-write", "effect_not_permitted"},
	} {
		t.Run(test.mode, func(t *testing.T) {
			e, runID, claim := programAfterWriteFixture(t, test.mode)
			ctx := context.Background()
			planTask := handOver(t, e, runID)
			if _, err := e.SubmitSession(ctx, hostResult(t, e, planTask, "planned")); err != nil {
				t.Fatalf("the write step's report was refused: %v", err)
			}
			if err := e.Drive(ctx, runID); err != nil {
				t.Fatalf("drive: %v", err)
			}
			r := driverRun(t, e, runID)
			// The program's attempt is the one with a process outcome; the
			// write step's was a session.
			var attempt *Attempt
			for _, candidate := range r.Attempts {
				if candidate.ProcessOutcome != nil {
					attempt = candidate
				}
			}
			if attempt == nil || attempt.Settled == nil {
				t.Fatalf("the program step did not settle: run=%s", r.Status)
			}
			if test.failure == "" {
				if attempt.Status != "completed" || r.Status != "completed" {
					t.Fatalf("a program that only read the workspace failed: %s %s", attempt.Status, r.Status)
				}
				if attempt.Accepted == nil || attempt.Accepted.Summary != "workspace="+claim.Path {
					t.Fatalf("the program was not handed the claimed workspace path: %+v", attempt.Accepted)
				}
				return
			}
			if attempt.Status != "failed" {
				t.Fatalf("a program that changed the workspace settled as %q, not failed", attempt.Status)
			}
			named := false
			for _, diagnostic := range r.Diagnostics {
				if diagnostic.Code == test.failure && strings.Contains(diagnostic.Message, "program-wrote.txt") && strings.Contains(diagnostic.Message, claim.Path) {
					named = true
				}
			}
			if !named {
				t.Fatalf("the failure does not name the path the program changed: %+v", r.Diagnostics)
			}
		})
	}
}

// programAfterWriteFixture rewrites the assisted checkout fixture: the write
// step hands over to a program step (the test binary in the given helper
// mode) that declares no workspace effect.
func programAfterWriteFixture(t *testing.T, mode string) (*Engine, string, WorktreeClaim) {
	t.Helper()
	e, _, claim := assistedWorkspaceFixture(t, "checkout")
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
	check := planStep
	check.ID, check.Title = "aif:step/check", "Check the tree with a program"
	check.Effects.Class = "none"
	check.InstructionsRef = nil
	check.Outputs = map[string]flow.OutputPort{}
	check.Executor.AdapterRef = builtinVersionRef(definitions, "core:adapter/local-process", "2.0.0")
	check.Executor.Operation = "process"
	checkBytes := writeRegistryDocument(t, e, "steps/check.json", check)
	checkRef := flow.Ref{ID: check.ID, Version: check.Version, Digest: rawDigest(checkBytes)}
	registry.Entries = append(registry.Entries, Definition{Ref: checkRef, Kind: "step", Path: "steps/check.json"})
	writeRuntimeJSON(t, filepath.Join(e.Root, e.Config.Configuration.RegistryFile), registry)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	e.Config.Configuration.Executors[check.ID] = ExecutorConfig{Executable: executable, Args: []string{"-test.run=^TestDriverWorkerHelper$", "--", mode}, Files: map[string]string{}, Environment: map[string]string{"DRIVER_TEST_HELPER": "1", "GORACE": "atexit_sleep_ms=0"}, TimeoutMS: 20000, GraceMS: 30, MaxOutputBytes: 1 << 20}
	writeRuntimeJSON(t, filepath.Join(e.Root, "prifly.json"), e.Config)

	workflow := flow.WorkflowRevision{
		SchemaVersion: "1", ID: "aif:workflow/pilot-program", Version: "1.0.0", Title: "Plan, then check with a program",
		Inputs: map[string]flow.InputPort{}, Outputs: map[string]flow.OutputPort{}, AllowedOutcomes: []string{"succeeded"},
		Limits: flow.Limits{MaxStepInstances: 4, MaxControlTransitions: 16, MaxParallelism: 1}, PolicyRef: builtinVersionRef(definitions, "core:policy/local", "2.0.0"),
	}
	workflow.Definition.Entry = "plan"
	workflow.Definition.Stages = map[string]flow.Stage{
		"plan":     {Kind: "step", StepRef: planRef, InputBindings: map[string]flow.Binding{}, On: map[string]string{"pass": "check"}},
		"check":    {Kind: "step", StepRef: checkRef, InputBindings: map[string]flow.Binding{}, On: map[string]string{"pass": "done"}, OnError: "rejected"},
		"done":     {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]flow.Binding{}},
		"rejected": {Kind: "finish", Outcome: "rejected", OutputBindings: map[string]flow.Binding{}},
	}
	workflow.AllowedOutcomes = []string{"succeeded", "rejected"}
	writeRuntimeJSON(t, filepath.Join(e.Root, "workflows/pilot-program.json"), workflow)
	result, err := e.Start(context.Background(), StartOptions{CommandID: newID("command"), WorkflowFile: "workflows/pilot-program.json", BriefFile: "brief.json", Inputs: map[string]string{}, WorkspaceMode: "checkout"})
	if err != nil {
		t.Fatal(err)
	}
	return e, result.Receipt.RunID, claim
}
