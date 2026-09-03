package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

func acceptanceProject(t *testing.T, boundaries []string, failureBoundary, mode string, onError bool) (*Engine, StartOptions) {
	t.Helper()
	e, options := contextDriverProject(t, nil)
	definitions, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	readJSON := func(path string, target any) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := decode(data, target); err != nil {
			t.Fatal(err)
		}
	}
	var registry RegistryFile
	readJSON(filepath.Join(e.Root, "definitions.json"), &registry)
	var step flow.StepDefinition
	readJSON(filepath.Join(e.Root, "steps/context.json"), &step)
	var workflow flow.WorkflowRevision
	readJSON(filepath.Join(e.Root, options.WorkflowFile), &workflow)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	refs := map[string]flow.Ref{}
	for _, kind := range []string{"content", "result"} {
		claim := "content_valid"
		if kind == "result" {
			claim = "check_passed"
		}
		definition := flow.CheckDefinition{SchemaVersion: flow.CheckDefinitionVersion, ID: "test:check/" + kind, Version: "1.0.0", Title: "Boundary checker", Kind: kind, Claim: claim, Executor: flow.Executor{AdapterRef: builtinVersionRef(definitions, "core:adapter/local-process", "2.0.0"), Operation: "check"}}
		data, err := canonical(definition)
		if err != nil {
			t.Fatal(err)
		}
		ref := flow.Ref{ID: definition.ID, Version: definition.Version, Digest: rawDigest(data)}
		path := "steps/check-" + kind + ".json"
		if err := os.WriteFile(filepath.Join(e.Root, path), data, 0600); err != nil {
			t.Fatal(err)
		}
		registry.Entries = append(registry.Entries, Definition{Ref: ref, Kind: "check", Path: path})
		refs[kind] = ref
		e.Config.Configuration.Executors[definition.ID] = ExecutorConfig{Executable: executable, Args: []string{"-test.run=^TestAcceptanceCheckerHelper$"}, Files: map[string]string{}, Environment: map[string]string{"ACCEPTANCE_CHECKER": "1", "ACCEPTANCE_FAILURE_BOUNDARY": failureBoundary, "ACCEPTANCE_MODE": mode, "GORACE": "atexit_sleep_ms=0"}, TimeoutMS: 20000, GraceMS: 30, MaxOutputBytes: MaxCheckWireBytes}
	}
	if slices.Contains(boundaries, "workflow_input") {
		port := workflow.Inputs["source"]
		port.ContentCheckRefs = []flow.Ref{refs["content"]}
		workflow.Inputs["source"] = port
	}
	if slices.Contains(boundaries, "step_input") {
		port := step.Inputs["source"]
		port.ContentCheckRefs = []flow.Ref{refs["content"]}
		step.Inputs["source"] = port
	}
	if slices.Contains(boundaries, "step_output") {
		port := step.Outputs["report"]
		port.ContentCheckRefs = []flow.Ref{refs["content"]}
		step.Outputs["report"] = port
	}
	if slices.Contains(boundaries, "workflow_output") {
		port := workflow.Outputs["report"]
		port.ContentCheckRefs = []flow.Ref{refs["content"]}
		workflow.Outputs["report"] = port
	}
	if slices.Contains(boundaries, "step_result") {
		step.ResultCheckRefs = []flow.Ref{refs["result"]}
	}
	stepData, err := canonical(step)
	if err != nil {
		t.Fatal(err)
	}
	stepRef := flow.Ref{ID: step.ID, Version: step.Version, Digest: rawDigest(stepData)}
	if err := os.WriteFile(filepath.Join(e.Root, "steps/context.json"), stepData, 0600); err != nil {
		t.Fatal(err)
	}
	for i := range registry.Entries {
		if registry.Entries[i].Kind == "step" && registry.Entries[i].Ref.ID == step.ID {
			registry.Entries[i].Ref = stepRef
		}
	}
	stage := workflow.Definition.Stages["work"]
	stage.StepRef = stepRef
	if onError {
		stage.OnError = "rejected"
		workflow.Definition.Stages["rejected"] = flow.Stage{Kind: "finish", Outcome: "rejected", OutputBindings: map[string]flow.Binding{}}
		workflow.AllowedOutcomes = append(workflow.AllowedOutcomes, "rejected")
	}
	workflow.Definition.Stages["work"] = stage
	workflow.Limits.MaxControlTransitions = 32
	writeRuntimeJSON(t, filepath.Join(e.Root, options.WorkflowFile), workflow)
	writeRuntimeJSON(t, filepath.Join(e.Root, "definitions.json"), registry)
	writeRuntimeJSON(t, filepath.Join(e.Root, "prifly.json"), e.Config)
	return e, options
}

func TestAcceptanceAllBoundariesUseRealChecks(t *testing.T) {
	e, options := acceptanceProject(t, []string{"workflow_input", "step_input", "step_output", "step_result", "workflow_output"}, "", "pass", false)
	started, err := e.Start(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	runID := started.Receipt.RunID
	initial := driverRun(t, e, runID)
	if initial.PendingAcceptance == nil || initial.PendingAcceptance.Kind != "workflow_input" || len(initial.Steps) != 0 || len(initial.Attempts) != 0 {
		t.Fatal("workflow input checking fabricated or admitted a producer")
	}
	assertPendingAcceptanceBindings(t, initial)
	next, err := e.Next(context.Background(), runID)
	if err != nil || next.Action != "acceptance" || next.Admission {
		t.Fatal("next did not expose the pending boundary", err, next)
	}
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	r, view, err := e.load(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "completed" || r.PendingAcceptance != nil || r.ActiveCheckID != "" || len(r.Active) != 0 || len(r.CheckExecutions) != 5 || len(r.Attempts) != 1 || len(r.Steps) != 1 || r.ControlTransitions != 7 {
		t.Fatalf("boundary lifecycle/counts mismatch: status=%s checks=%d attempts=%d steps=%d controls=%d diagnostics=%+v pending=%+v", r.Status, len(r.CheckExecutions), len(r.Attempts), len(r.Steps), r.ControlTransitions, r.Diagnostics, r.PendingAcceptance)
	}
	seen := map[string]bool{}
	for _, check := range r.CheckExecutions {
		if check.Status != "completed" || check.Report == nil || check.Report.Status != "pass" || check.Settled == nil {
			t.Fatalf("required check did not settle: %+v", check)
		}
		seen[check.Request.Boundary] = true
		launches, err := os.ReadFile(filepath.Join(check.Workspace, "launches"))
		if err != nil || string(launches) != "launch\n" {
			t.Fatal("check was not an exactly-once foreground launch", err)
		}
		evidence, err := e.checkEvidence(r, check)
		if err != nil {
			t.Fatal(err)
		}
		_, evidenceBytes, err := e.Artifact(ArtifactRef{ArtifactID: evidence.ID, Revision: 1, Digest: evidence.Digest})
		if err != nil || flow.ValidateProtocol("Evidence", evidenceBytes) != nil {
			t.Fatal("evidence is not a retained exact record", err)
		}
	}
	if len(seen) != 5 {
		t.Fatal("a declared boundary was skipped", seen)
	}
	output, _, err := e.Artifact(r.Outputs["report"])
	if err != nil || len(output.ContentCheckEvidence) != 1 {
		t.Fatal("output has no exact content check evidence", err)
	}
	input, _, err := e.Artifact(r.Inputs["source"])
	if err != nil || len(input.ContentCheckEvidence) != 0 {
		t.Fatal("checking an existing input mutated its immutable metadata", err)
	}
	for _, attempt := range r.Attempts {
		if attempt.Accepted == nil || attempt.Accepted.Verdict != "pass" || attempt.Settled == nil || r.Steps[attempt.StepID].Settled == nil {
			t.Fatal("producer result did not pass its separate acceptance")
		}
	}
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	_, after, err := e.load(context.Background(), runID)
	if err != nil || after.Snapshot.Version != view.Snapshot.Version {
		t.Fatal("settled checks were re-executed by another Drive", err)
	}
}

func TestAcceptanceProducerOutputsStayUnpublishedUntilChecks(t *testing.T) {
	e, options := acceptanceProject(t, []string{"step_output", "step_result"}, "", "pass", false)
	started, err := e.Start(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	runID := started.Receipt.RunID
	attempt := driverExecuteFirst(t, e, runID)
	r := driverRun(t, e, runID)
	if attempt.Status != "completed" || attempt.Settled == nil || attempt.Accepted != nil || r.PendingAcceptance == nil || len(r.Active) != 0 || r.Steps[attempt.StepID].Status != "verifying" || len(r.Steps[attempt.StepID].Outputs) != 0 {
		t.Fatal("process success was confused with result acceptance")
	}
	assertPendingAcceptanceBindings(t, r)
	ref := r.PendingAcceptance.Bindings["report"]
	if _, _, err := e.Artifact(ref); !os.IsNotExist(err) {
		t.Fatal("producer output was publicly accepted before required checks", err)
	}
	if err := os.WriteFile(filepath.Join(attempt.Workspace, "outputs/report"), []byte("changed after the producer settled"), 0600); err != nil {
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
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	r = driverRun(t, e, runID)
	_, data, err := e.Artifact(ref)
	if err != nil || string(data) != "accepted output\n" || r.Status != "completed" || len(r.Attempts) != 1 || len(r.CheckExecutions) != 2 {
		t.Fatal("acceptance recovery reread mutable files or reran the producer", err, r.Status, r.Diagnostics)
	}
}

func assertPendingAcceptanceBindings(t *testing.T, r Run) {
	t.Helper()
	if err := acceptanceInvariant(r); err != nil {
		t.Fatal("actual pending acceptance failed its invariant", err)
	}
	for _, mutation := range []string{"foreign_boundary", "foreign_subject", "missing_subject", "unknown_port", "missing_check_ref"} {
		copyRun := r
		pending := *r.PendingAcceptance
		pending.Checks = slices.Clone(pending.Checks)
		required := &pending.Checks[0]
		required.Subjects = slices.Clone(required.Subjects)
		switch mutation {
		case "foreign_boundary":
			required.Boundary = "unknown"
		case "foreign_subject":
			required.Subjects[0].ArtifactID = "test:artifact/foreign"
		case "missing_subject":
			required.Subjects = []ArtifactRef{}
		case "unknown_port":
			required.Port = "unknown"
		case "missing_check_ref":
			required.Ref = flow.Ref{}
		}
		copyRun.PendingAcceptance = &pending
		if acceptanceInvariant(copyRun) == nil {
			t.Fatal("pending acceptance accepted", mutation)
		}
	}
}

func TestAcceptanceFailureNeverRewritesProducerVerdict(t *testing.T) {
	for _, test := range []struct {
		boundary, report string
		onError          bool
		attempts, steps  int
	}{
		{"workflow_input", "fail", false, 0, 0},
		{"step_input", "fail", true, 0, 1},
		{"step_output", "fail", true, 1, 1},
		{"step_result", "inconclusive", true, 1, 1},
		{"workflow_output", "fail", false, 1, 1},
	} {
		t.Run(test.boundary, func(t *testing.T) {
			e, options := acceptanceProject(t, []string{test.boundary}, test.boundary, test.report, test.onError)
			started, err := e.Start(context.Background(), options)
			if err != nil {
				t.Fatal(err)
			}
			if err := e.Drive(context.Background(), started.Receipt.RunID); err != nil {
				t.Fatal(err)
			}
			r := driverRun(t, e, started.Receipt.RunID)
			if r.PendingAcceptance != nil || r.ActiveCheckID != "" || len(r.Active) != 0 || len(r.CheckExecutions) != 1 || len(r.Attempts) != test.attempts || len(r.Steps) != test.steps || len(r.Outputs) != 0 {
				t.Fatalf("failed check passed a boundary: status=%s diagnostics=%+v", r.Status, r.Diagnostics)
			}
			if test.onError {
				if r.Status != "completed" || r.Outcome == nil || *r.Outcome != "rejected" {
					t.Fatal("explicit on_error was not used", r.Status, r.Diagnostics)
				}
			} else if r.Status != "failed" {
				t.Fatal("unhandled check failure was not terminal", r.Status)
			}
			for _, check := range r.CheckExecutions {
				if check.Status != "completed" || check.Report == nil || check.Report.Status != test.report {
					t.Fatal("negative report was confused with process failure")
				}
				if _, err := e.checkEvidence(r, check); err != nil {
					t.Fatal("negative check evidence was lost", err)
				}
			}
			for _, attempt := range r.Attempts {
				var candidate Result
				if decode(attempt.Candidate, &candidate) != nil || candidate.Verdict != "pass" || attempt.Status != "completed" || attempt.Settled == nil {
					t.Fatal("check failure rewrote producer process outcome or verdict")
				}
				if test.boundary == "workflow_output" {
					if attempt.Accepted == nil {
						t.Fatal("export failure erased the earlier step acceptance")
					}
					continue
				}
				if attempt.Accepted != nil || r.Steps[attempt.StepID].Verdict != "" || len(r.Steps[attempt.StepID].Outputs) != 0 {
					t.Fatal("producer result was accepted despite a required check failure")
				}
				for _, ref := range candidate.Outputs {
					if _, _, err := e.Artifact(ref); !os.IsNotExist(err) {
						t.Fatal("failed content escaped as an accepted output", err)
					}
				}
			}
		})
	}
}

func TestAcceptanceCheckerHelper(t *testing.T) {
	if os.Getenv("ACCEPTANCE_CHECKER") != "1" {
		return
	}
	raw, err := io.ReadAll(io.LimitReader(os.Stdin, MaxCheckWireBytes+1))
	if err != nil {
		os.Exit(50)
	}
	request, err := ParseCheckRequest(raw)
	if err != nil || os.Getenv("PRIFLY_STEP_SOCKET") != "" || os.Getenv("PRIFLY_STEP_TOKEN") != "" {
		os.Exit(51)
	}
	contextBytes, err := os.ReadFile(os.Getenv("PRIFLY_CONTEXT_FILE"))
	var transport ContextManifest
	if err != nil || json.Unmarshal(contextBytes, &transport) != nil {
		os.Exit(52)
	}
	for i, ref := range request.Subjects {
		port := transport.Inputs[fmt.Sprintf("subject_%03d", i)]
		data, err := os.ReadFile(port.Path)
		if err != nil || port.Ref != ref || rawDigest(data) != ref.Digest {
			os.Exit(53)
		}
	}
	if request.CandidateRef != nil {
		port := transport.Inputs["candidate"]
		data, err := os.ReadFile(port.Path)
		if err != nil || port.Ref != *request.CandidateRef || rawDigest(data) != request.CandidateRef.Digest || flow.ValidateProtocol("StepResult", data) != nil {
			os.Exit(54)
		}
	}
	rendered, err := os.ReadFile(transport.Rendering.Path)
	var rendering struct {
		Request json.RawMessage `json:"check_request"`
		Digest  string          `json:"check_request_digest"`
	}
	if err != nil || json.Unmarshal(rendered, &rendering) != nil || rendering.Digest != rawDigest(raw) || !bytes.Equal(rendering.Request, raw) {
		os.Exit(55)
	}
	file, err := os.OpenFile("launches", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		os.Exit(56)
	}
	_, _ = file.WriteString("launch\n")
	_ = file.Close()
	result := CheckResult{SchemaVersion: CheckResultVersion, CheckID: request.CheckID, RunID: request.RunID, RequestDigest: rawDigest(raw), Status: "pass", Summary: "Verified exact sealed boundary bytes", Limitations: []string{"Local test method; no independent semantic or fresh isolation qualification"}}
	if request.Boundary == os.Getenv("ACCEPTANCE_FAILURE_BOUNDARY") {
		result.Status = os.Getenv("ACCEPTANCE_MODE")
	}
	data, err := json.Marshal(result)
	if err != nil {
		os.Exit(57)
	}
	_, _ = os.Stdout.Write(data)
	os.Exit(0)
}
