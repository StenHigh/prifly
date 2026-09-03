package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

// reviewFanOut builds the shape a real review fan-out has: one plan handed to
// two reviewers with different parameters, both required. The reviewers write
// nothing, so they hold no worktree and cannot disturb each other.
func reviewFanOut(t *testing.T, parallelism int) (*Engine, string) {
	t.Helper()
	return reviewFanOutWith(t, parallelism, 2, &flow.Join{Mode: "all", AcceptOutcomes: []string{"succeeded"}, Selection: "all", Remainder: "wait"})
}

func reviewFanOutWith(t *testing.T, parallelism, reviewers int, join *flow.Join) (*Engine, string) {
	t.Helper()
	e := contextRegistryRuntime(t)
	definitions, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	skill := "---\nname: aif-improve\n---\n\n# Propose refinements\n"
	if err := os.WriteFile(filepath.Join(e.Root, "resources/improve-skill.md"), []byte(skill), 0600); err != nil {
		t.Fatal(err)
	}
	skillRef := flow.Ref{ID: "aif:context/improve-skill", Version: "1.0.0", Digest: rawDigest([]byte(skill))}

	proposal := map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object", "required": []string{"verdict"}, "properties": map[string]any{"verdict": map[string]any{"type": "string", "minLength": 1}}, "additionalProperties": false}
	proposalBytes := writeRegistryDocument(t, e, "schemas/proposal.json", proposal)
	proposalRef := flow.Ref{ID: "aif:schema/proposal", Version: "1.0.0", Digest: rawDigest(proposalBytes)}
	variant := map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object", "required": []string{"name"}, "properties": map[string]any{"name": map[string]any{"type": "string", "minLength": 1}}, "additionalProperties": false}
	variantBytes := writeRegistryDocument(t, e, "schemas/variant.json", variant)
	variantRef := flow.Ref{ID: "aif:schema/variant", Version: "1.0.0", Digest: rawDigest(variantBytes)}

	step := flow.StepDefinition{
		SchemaVersion: "2", ID: "aif:step/improve", Version: "1.0.0", Title: "Propose refinements", Kind: "worker",
		Inputs:      map[string]flow.InputPort{"variant": {Port: flow.Port{Format: "json", SchemaRef: &variantRef}, Required: true}},
		Outputs:     map[string]flow.OutputPort{"proposal": {Port: flow.Port{Format: "json", SchemaRef: &proposalRef}, RequiredFor: []string{"pass"}}},
		ContextRefs: []flow.Ref{}, RequiredCapabilities: []string{}, ResultCheckRefs: []flow.Ref{}, ResultSchemaRef: builtinRef(definitions, "core:schema/step-result"),
	}
	step.Executor.AdapterRef, step.Executor.Operation = builtinRef(definitions, "core:adapter/assisted-session"), "session"
	step.InstructionsRef, step.Effects.Class, step.Effects.RetryClass = &skillRef, "none", "never"
	stepBytes := writeRegistryDocument(t, e, "steps/improve.json", step)
	stepRef := flow.Ref{ID: step.ID, Version: step.Version, Digest: rawDigest(stepBytes)}

	branch := flow.WorkflowRevision{
		SchemaVersion: "1", ID: "aif:workflow/review", Version: "1.0.0", Title: "One reviewer",
		Inputs:          map[string]flow.InputPort{"variant": {Port: flow.Port{Format: "json", SchemaRef: &variantRef}, Required: true}},
		Outputs:         map[string]flow.OutputPort{"proposal": {Port: flow.Port{Format: "json", SchemaRef: &proposalRef}, RequiredFor: []string{"succeeded"}}},
		AllowedOutcomes: []string{"succeeded"},
		Limits:          flow.Limits{MaxStepInstances: 2, MaxControlTransitions: 16, MaxParallelism: 1},
		PolicyRef:       builtinVersionRef(definitions, "core:policy/local", "2.0.0"),
	}
	branch.Definition.Entry = "improve"
	branch.Definition.Stages = map[string]flow.Stage{
		"improve": {Kind: "step", StepRef: stepRef, InputBindings: map[string]flow.Binding{"variant": {From: "workflow_input", Port: "variant"}}, On: map[string]string{"pass": "done"}},
		"done":    {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]flow.Binding{"proposal": {From: "stage_output", StageID: "improve", Port: "proposal"}}},
	}
	branchBytes := writeRegistryDocument(t, e, "workflows/review.json", branch)
	branchRef := flow.Ref{ID: branch.ID, Version: branch.Version, Digest: rawDigest(branchBytes)}

	names := []string{"opus", "sonnet", "haiku"}[:reviewers]
	branches := make([]flow.ParallelBranch, 0, reviewers)
	for _, name := range names {
		value, err := canonical(map[string]any{"name": name})
		if err != nil {
			t.Fatal(err)
		}
		branches = append(branches, flow.ParallelBranch{ID: name, WorkflowRef: branchRef,
			InputBindings: map[string]flow.Binding{"variant": {From: "literal", SchemaRef: &variantRef, Value: value}}})
	}
	parent := flow.WorkflowRevision{
		SchemaVersion: "1", ID: "aif:workflow/fanout", Version: "1.0.0", Title: "Two reviewers, both required",
		Inputs: map[string]flow.InputPort{}, Outputs: map[string]flow.OutputPort{}, AllowedOutcomes: []string{"succeeded", "rejected"},
		Limits:    flow.Limits{MaxStepInstances: 16, MaxControlTransitions: 128, MaxParallelism: parallelism, MaxChildDepth: 2},
		PolicyRef: builtinVersionRef(definitions, "core:policy/local", "3.0.0"),
	}
	parent.Definition.Entry = "refine"
	parent.Definition.Stages = map[string]flow.Stage{
		"refine": {Kind: "parallel", MaxParallelism: parallelism, ParallelBranches: branches,
			Join: join, On: map[string]string{"satisfied": "done", "unsatisfied": "refused"}},
		"done":    {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]flow.Binding{}},
		"refused": {Kind: "finish", Outcome: "rejected", OutputBindings: map[string]flow.Binding{}},
	}
	writeRuntimeJSON(t, filepath.Join(e.Root, "workflows/fanout.json"), parent)

	registry := RegistryFile{SchemaVersion: "3", Entries: []Definition{
		{Ref: skillRef, Kind: "resource", Path: "resources/improve-skill.md", ByteEncoding: "utf8_text", MediaType: "text/markdown; charset=utf-8"},
		{Ref: proposalRef, Kind: "schema", Path: "schemas/proposal.json"},
		{Ref: variantRef, Kind: "schema", Path: "schemas/variant.json"},
		{Ref: stepRef, Kind: "step", Path: "steps/improve.json"},
		{Ref: branchRef, Kind: "workflow", Path: "workflows/review.json"},
	}}
	writeRuntimeJSON(t, filepath.Join(e.Root, e.Config.Configuration.RegistryFile), registry)
	e.Config.Configuration.SchemaVersion = CoreContextConfigVersion
	e.Config.ConfigurationSchemaRef = builtinVersionRef(definitions, "core:schema/core-configuration", "2.0.0")
	e.Config.AdapterBindings["local_process"] = builtinVersionRef(definitions, "core:adapter/local-process", "2.0.0")
	e.Config.DefaultPolicyRef = builtinVersionRef(definitions, "core:policy/local", "2.0.0")
	writeRuntimeJSON(t, filepath.Join(e.Root, "prifly.json"), e.Config)
	brief := Brief{"1", "aif:brief/fanout", "Two reviews of one plan", "Both reviewers report", []string{"No writes"}, []string{"Network"}, []string{"Two proposals"}, []ArtifactRef{}, []string{}, "explicit"}
	writeRuntimeJSON(t, filepath.Join(e.Root, "brief.json"), brief)

	// The installation's capacity is a separate statement from the workflow's,
	// and the smaller governs: a Run may not run more at once than its
	// authority admits, whatever its definition declares.
	if parallelism > 1 {
		if _, err := e.SetAdmissionCapacity(context.Background(), CapacityRequest{CommandID: newID("command"), Capacity: int64(parallelism), Reason: "run the declared reviewers at once"}); err != nil {
			t.Fatal(err)
		}
	}
	result, err := e.Start(context.Background(), StartOptions{CommandID: newID("command"), WorkflowFile: "workflows/fanout.json", BriefFile: "brief.json", Inputs: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	return e, result.Receipt.RunID
}

// submitProposal acts as the host for one exact outstanding attempt.
func submitProposal(t *testing.T, e *Engine, runID, attemptID string) {
	t.Helper()
	ctx := context.Background()
	task, err := e.SessionTask(ctx, runID, attemptID)
	if err != nil {
		t.Fatal(err)
	}
	slot := task.Context.Outputs["proposal"]
	body := []byte(`{"verdict":"changes_suggested"}`)
	if err := os.WriteFile(filepath.Join(task.Workspace, slot.Path), body, 0600); err != nil {
		t.Fatal(err)
	}
	result, err := canonical(map[string]any{
		"schema_version": "1", "run_id": runID, "step_instance_id": task.StepInstanceID,
		"attempt_id": attemptID, "envelope_digest": task.EnvelopeDigest, "verdict": "pass",
		"summary": "proposal reported", "evidence_refs": []any{}, "effect_receipt_refs": []any{},
		"outputs": map[string]any{"proposal": map[string]any{
			"artifact_id": slot.ArtifactID, "revision": slot.Revision, "digest": rawDigest(body)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.SubmitSession(ctx, SessionSubmission{SchemaVersion: task.SchemaVersion,
		RunID: runID, AttemptID: attemptID, EnvelopeDigest: task.EnvelopeDigest, Result: result}); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatal(err)
	}
}

func awaitingReviewers(t *testing.T, r Run) []*Attempt {
	t.Helper()
	held := []*Attempt{}
	for _, a := range r.Attempts {
		if a.Session != nil && a.Session.HostState == SessionAwaiting && a.Settled == nil {
			held = append(held, a)
		}
	}
	return held
}

// Two reviewers asked at once must both be holding a task at the same time.
// Handing one out and waiting for it before asking the second is a fan-out in
// shape only: the second reviewer would start after the first had finished.
func TestBothReviewersHoldATaskAtOnce(t *testing.T) {
	e, runID := reviewFanOut(t, 2)
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	held := awaitingReviewers(t, r)
	if len(held) != 2 {
		t.Fatalf("reviewers holding a task at once: %d, want 2 (attempts: %d)", len(held), len(r.Attempts))
	}
	// Neither holds a worktree, so neither can disturb the other.
	for _, a := range held {
		if a.Session.ClaimID != "" {
			t.Fatalf("a proposing reviewer was handed a worktree: %+v", a.Session)
		}
	}
}

// A stage that declares one at a time still runs one at a time: the same
// definition under a different declaration must not quietly become concurrent.
func TestOneAtATimeStaysOneAtATime(t *testing.T) {
	e, runID := reviewFanOut(t, 1)
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	if held := awaitingReviewers(t, driverRun(t, e, runID)); len(held) != 1 {
		t.Fatalf("a sequential fan-out handed out %d tasks at once", len(held))
	}
}

// quorumFanOut asks three reviewers at once and accepts the first two. The
// third is still live when the quorum is reached, which is the case a sequential
// fan-out never produces: there the remainder was simply never entered.
func quorumFanOut(t *testing.T, required int) (*Engine, string) {
	t.Helper()
	return reviewFanOutWith(t, 3, 3, &flow.Join{Mode: "quorum", AcceptOutcomes: []string{"succeeded"},
		RequiredSuccesses: required, Selection: "first_observed", Remainder: "cancel"})
}

// AC-043: a reached quorum must not publish over a branch that is still
// running. The selected set is fixed, but the transition and the summary wait
// until the remainder is known to have stopped — a cancellation requested is
// not a cancellation confirmed. The order is read from the journal, because
// that is where "before" and "after" are recorded.
func TestReachedQuorumWaitsForTheLiveRemainder(t *testing.T) {
	e, runID := quorumFanOut(t, 2)
	ctx := context.Background()
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatal(err)
	}
	held := awaitingReviewers(t, driverRun(t, e, runID))
	if len(held) != 3 {
		t.Fatalf("three reviewers should hold a task at once, got %d", len(held))
	}
	for _, attempt := range held[:2] {
		submitProposal(t, e, runID, attempt.ID)
	}
	r := driverRun(t, e, runID)
	if r.Status != "completed" || r.Outcome == nil || *r.Outcome != "succeeded" {
		t.Fatalf("the quorum did not carry the run to its accepted finish: %s %+v", r.Status, r.Outcome)
	}

	decisions, _ := joinHistory(t, e, runID)
	cancelling, cancelled, terminal := -1, -1, -1
	// The decision that folds in the stopped branch is also the one that settles
	// the join, so these are independent marks rather than exclusive cases.
	for i, d := range decisions {
		if d.Route == "cancelling" {
			cancelling = i
		}
		if d.BranchStatus == "cancelled" {
			cancelled = i
		}
		if d.Route == "satisfied" || d.Route == "unsatisfied" {
			terminal = i
		}
	}
	if cancelling < 0 {
		t.Fatal("the quorum settled without ever asking the remainder to stop")
	}
	if cancelled < 0 {
		t.Fatal("the stopped branch was never folded into the join")
	}
	if terminal < 0 {
		t.Fatal("the join never reached a verdict")
	}
	if !(cancelling < cancelled && cancelled <= terminal) {
		t.Fatalf("the join published before the remainder was confirmed: cancelling=%d cancelled=%d terminal=%d",
			cancelling, cancelled, terminal)
	}
	// Every branch was entered and folded in, so nothing was left out of the
	// account and there is no remainder to describe. What happened to the third
	// is recorded where it belongs: in its own decision.
	final, stoppedDecision := decisions[terminal], decisions[cancelled]
	if final.ObservedCount != final.BranchCount || final.RemainderDisposition != "" {
		t.Fatalf("a fully accounted join still claimed a remainder: %+v", final)
	}
	if stoppedDecision.BranchStatus != "cancelled" || stoppedDecision.BranchOutcome != nil || stoppedDecision.Accepted {
		t.Fatalf("the stopped branch was recorded as something it was not: %+v", stoppedDecision)
	}
	// The summary names the stopped branch as stopped, not as absent.
	a := r.activationForInvocation(r.RootInvocationID, "refine")
	if a == nil || a.Parallel == nil || a.Parallel.ResultsRef == nil {
		t.Fatal("a settled join published no summary")
	}
	_, data, err := e.Artifact(*a.Parallel.ResultsRef)
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		SelectedBranchIDs []string `json:"selected_branch_ids"`
		Branches          []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"branches"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	stopped := 0
	for _, b := range manifest.Branches {
		if b.Status == "cancelled" {
			stopped++
		}
	}
	if stopped != 1 || len(manifest.Branches) != 3 {
		t.Fatalf("the summary does not account for all three branches: %+v", manifest.Branches)
	}
	if len(manifest.SelectedBranchIDs) != 2 {
		t.Fatalf("a quorum of two selected %d branches", len(manifest.SelectedBranchIDs))
	}
}

// A branch folded in while others still owe the join their own decisions is
// "recorded", and that stays true when none of them is still running: several
// branches entered at once can all settle before the first decision is taken.
// Judging by what is still running instead of by what has been decided left
// that case with no admissible route at all, and the stage failed asking for a
// route for "undecided". The map fixture reaches it in the driver; this pins
// the rule itself, without depending on the order the driver happens to take.
func TestRecordedRouteDependsOnDecisionsNotOnWhatIsStillRunning(t *testing.T) {
	settled := Observation{UTC: "2026-01-01T00:00:00Z"}
	outcome := "succeeded"
	entered := []*Invocation{
		{ID: "invocation:a", BranchID: "left", Status: "completed", Outcome: &outcome, Settled: &settled},
		{ID: "invocation:b", BranchID: "right", Status: "completed", Outcome: &outcome, Settled: &settled},
	}
	progress := ParallelProgress{BranchIDs: []string{"left", "right"}, EnteredCount: 2, CurrentBranchInvocationID: "invocation:b"}
	stage := flow.Stage{Kind: "parallel", MaxParallelism: 2, Join: &flow.Join{Mode: "all", AcceptOutcomes: []string{"succeeded"}, Selection: "all", Remainder: "wait"}}
	stage.ParallelBranches = []flow.ParallelBranch{{ID: "left"}, {ID: "right"}}
	decision := JoinDecision{Route: "recorded", Mode: "all", Selection: "all", Remainder: "wait", BranchID: "left", Position: 1, BranchCount: 2, BranchStatus: "completed", BranchOutcome: &outcome, Accepted: true, AcceptedCount: 1, ObservedCount: 1, Verdict: "undecided"}
	if err := checkJoinRoute(stage, progress, entered, decision); err != nil {
		t.Fatalf("the first of two settled branches could not be recorded: %v", err)
	}
	// The last decision is not "recorded": there is nothing left to wait for.
	decision.BranchID, decision.Position, decision.ObservedCount, decision.AcceptedCount, decision.Verdict = "right", 2, 2, 2, "satisfied"
	if err := checkJoinRoute(stage, progress, entered, decision); err == nil {
		t.Fatal("a join with every decision in hand was allowed to keep waiting")
	}
}
