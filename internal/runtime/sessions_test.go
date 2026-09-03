package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// The fixture starts a real assisted Run through the ordinary Start path: a
// pinned markdown skill in the project registry, a step whose definition
// selects the assisted adapter, and one active worktree claim. No process is
// configured for it anywhere, so any settlement it reaches rests on the host
// report alone.
func assistedFixture(t *testing.T) (*Engine, string, WorktreeClaim) {
	return assistedWorkspaceFixture(t, "")
}

func assistedWorkspaceFixture(t *testing.T, workspace string) (*Engine, string, WorktreeClaim) {
	return assistedWorkspaceFixtureWithDecisions(t, workspace, nil, nil)
}

func assistedWorkspaceFixtureWithDecisions(t *testing.T, workspace string, catalog *DecisionCatalog, sheet *DecisionSheet) (*Engine, string, WorktreeClaim) {
	t.Helper()
	e := contextRegistryRuntime(t)
	repository := gitRepository(t)
	claim, err := e.ClaimWorktree(context.Background(), ClaimRequest{CommandID: "command:claim", Repository: repository, OwnerID: "session:pilot", WorkspaceMode: workspace})
	if err != nil {
		t.Fatal(err)
	}
	definitions, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	skill := "---\nname: aif-plan\n---\n\n# Plan the change\n"
	if err := os.WriteFile(filepath.Join(e.Root, "resources/plan-skill.md"), []byte(skill), 0600); err != nil {
		t.Fatal(err)
	}
	skillRef := flow.Ref{ID: "aif:context/plan-skill", Version: "1.0.0", Digest: rawDigest([]byte(skill))}

	planSchema := map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object", "required": []string{"summary"}, "properties": map[string]any{"summary": map[string]any{"type": "string", "minLength": 1}}, "additionalProperties": false}
	planSchemaBytes := writeRegistryDocument(t, e, "schemas/plan.json", planSchema)
	planSchemaRef := flow.Ref{ID: "aif:schema/plan", Version: "1.0.0", Digest: rawDigest(planSchemaBytes)}
	tool := flow.ToolDescriptor{SchemaVersion: flow.ToolDescriptorVersion, ID: "aif:tool/commit", Version: "1.0.0", AdapterRef: builtinRef(definitions, "core:adapter/assisted-session"), Operation: "commit", ArgumentsSchemaRef: planSchemaRef, ResultSchemaRef: planSchemaRef, EffectClass: "workspace_write", RetryClass: "never", RequiredCapabilities: []string{}}
	toolBytes := writeRegistryDocument(t, e, "tools/commit.json", tool)
	toolRef := flow.Ref{ID: tool.ID, Version: tool.Version, Digest: rawDigest(toolBytes)}

	step := flow.StepDefinition{
		SchemaVersion: "2", ID: "aif:step/plan", Version: "1.0.0", Title: "Plan through an assisted session", Kind: "worker",
		Inputs: map[string]flow.InputPort{}, Outputs: map[string]flow.OutputPort{"plan": {Port: flow.Port{Format: "json", SchemaRef: &planSchemaRef}, RequiredFor: []string{"pass"}}},
		ContextRefs: []flow.Ref{}, RequiredCapabilities: []string{}, ResultCheckRefs: []flow.Ref{}, ResultSchemaRef: builtinRef(definitions, "core:schema/step-result"),
	}
	step.Executor.AdapterRef = builtinRef(definitions, "core:adapter/assisted-session")
	step.Executor.Operation = "session"
	step.InstructionsRef = &skillRef
	step.Effects.Class = "workspace_write"
	step.Effects.RetryClass = "never"
	stepBytes := writeRegistryDocument(t, e, "steps/plan.json", step)
	stepRef := flow.Ref{ID: step.ID, Version: step.Version, Digest: rawDigest(stepBytes)}

	workflow := flow.WorkflowRevision{
		SchemaVersion: "1", ID: "aif:workflow/pilot", Version: "1.0.0", Title: "Assisted pilot",
		Inputs: map[string]flow.InputPort{}, Outputs: map[string]flow.OutputPort{}, AllowedOutcomes: []string{"succeeded"},
		Limits: flow.Limits{MaxStepInstances: 2, MaxControlTransitions: 16, MaxParallelism: 1}, PolicyRef: builtinVersionRef(definitions, "core:policy/local", "2.0.0"),
	}
	workflow.Definition.Entry = "plan"
	workflow.Definition.Stages = map[string]flow.Stage{
		"plan": {Kind: "step", StepRef: stepRef, InputBindings: map[string]flow.Binding{}, On: map[string]string{"pass": "done"}},
		"done": {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]flow.Binding{}},
	}
	writeRuntimeJSON(t, filepath.Join(e.Root, "workflows/pilot.json"), workflow)

	registry := RegistryFile{SchemaVersion: "3", Entries: []Definition{
		{Ref: skillRef, Kind: "resource", Path: "resources/plan-skill.md", ByteEncoding: "utf8_text", MediaType: "text/markdown; charset=utf-8"},
		{Ref: planSchemaRef, Kind: "schema", Path: "schemas/plan.json"},
		{Ref: toolRef, Kind: "tool", Path: "tools/commit.json"},
		{Ref: stepRef, Kind: "step", Path: "steps/plan.json"},
	}}
	writeRuntimeJSON(t, filepath.Join(e.Root, e.Config.Configuration.RegistryFile), registry)

	// Full context and pinned resources require the core configuration contract.
	e.Config.Configuration.SchemaVersion = CoreContextConfigVersion
	e.Config.ConfigurationSchemaRef = builtinVersionRef(definitions, "core:schema/core-configuration", "2.0.0")
	e.Config.AdapterBindings["local_process"] = builtinVersionRef(definitions, "core:adapter/local-process", "2.0.0")
	e.Config.DefaultPolicyRef = builtinVersionRef(definitions, "core:policy/local", "2.0.0")
	writeRuntimeJSON(t, filepath.Join(e.Root, "prifly.json"), e.Config)

	brief := Brief{"1", "aif:brief/pilot", "Assisted pilot", "Plan a small change in the claimed worktree", []string{"Local worktree only"}, []string{"Network and credentials"}, []string{"A sealed plan"}, []ArtifactRef{}, []string{}, "explicit"}
	writeRuntimeJSON(t, filepath.Join(e.Root, "brief.json"), brief)

	result, err := e.Start(context.Background(), StartOptions{CommandID: newID("command"), WorkflowFile: "workflows/pilot.json", BriefFile: "brief.json", Inputs: map[string]string{}, DecisionCatalog: catalog, DecisionSheet: sheet, WorkspaceMode: workspace})
	if err != nil {
		t.Fatal(err)
	}
	return e, result.Receipt.RunID, claim
}

func TestWorkspaceCheckoutHandoffKeepsScratchOutsideRepository(t *testing.T) {
	e, runID, claim := assistedWorkspaceFixture(t, "checkout")
	task := handOver(t, e, runID)
	r := driverRun(t, e, runID)
	if r.SchemaVersion != CoreWorkspaceStateVersion || task.SchemaVersion != AssistedSessionWorkspaceVersion || task.WorkspaceMode != "checkout" || task.RepositoryWorkspace != claim.Repository.Toplevel || task.Workspace == task.RepositoryWorkspace {
		t.Fatalf("checkout handoff did not keep workspace identities separate: run=%s task=%+v claim=%+v", r.SchemaVersion, task, claim)
	}
	if _, err := os.Stat(filepath.Join(task.RepositoryWorkspace, "context")); !os.IsNotExist(err) {
		t.Fatalf("authority context leaked into the checkout: %v", err)
	}
	view, err := e.View(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]any{"CoreRunStateV23": r, "CoreRunViewV23": view, "SessionTaskV3": task} {
		if err := validatePublic(t, name, value); err != nil {
			t.Fatalf("%s rejected checkout state: %v", name, err)
		}
	}
}

// writeRegistryDocument writes a canonical registry document and returns the
// exact bytes its reference must digest.
func writeRegistryDocument(t *testing.T, e *Engine, path string, value any) []byte {
	t.Helper()
	data, err := canonical(value)
	if err != nil {
		t.Fatal(err)
	}
	full := filepath.Join(e.Root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, data, 0600); err != nil {
		t.Fatal(err)
	}
	return data
}

// handOver drives the run until the attempt is held by a host and returns the
// task the host was given.
func handOver(t *testing.T, e *Engine, runID string) SessionTask {
	t.Helper()
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatalf("drive to handoff: %v", err)
	}
	task, err := e.SessionTask(context.Background(), runID, "")
	if err != nil {
		t.Fatalf("no handoff was recorded: %v", err)
	}
	return task
}

func hostResult(t *testing.T, e *Engine, task SessionTask, summary string) SessionSubmission {
	t.Helper()
	r, _, err := e.load(context.Background(), task.RunID)
	if err != nil {
		t.Fatal(err)
	}
	attempt := r.Attempts[task.AttemptID]
	slot := attempt.Context.Outputs["plan"]
	body, err := canonical(map[string]any{"summary": summary})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(attempt.Workspace, filepath.FromSlash(slot.Path)), body, 0600); err != nil {
		t.Fatal(err)
	}
	result := map[string]any{
		"schema_version": "1", "run_id": task.RunID, "step_instance_id": attempt.StepID, "attempt_id": attempt.ID,
		"envelope_digest": attempt.EnvelopeDigest, "verdict": "pass",
		"outputs":       map[string]any{"plan": map[string]any{"artifact_id": slot.ArtifactID, "revision": slot.Revision, "digest": rawDigest(body)}},
		"evidence_refs": []any{}, "effect_receipt_refs": []any{}, "summary": summary,
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	return SessionSubmission{SchemaVersion: task.SchemaVersion, RunID: task.RunID, AttemptID: task.AttemptID, EnvelopeDigest: task.EnvelopeDigest, Result: encoded}
}

func TestAssistedReportRecordsEachNamedCostOnTheAttempt(t *testing.T) {
	e, runID, _ := assistedFixture(t)
	task := handOver(t, e, runID)
	if task.SchemaVersion != AssistedSessionCostVersion {
		t.Fatalf("new assisted handoff uses %s", task.SchemaVersion)
	}
	submission := hostResult(t, e, task, "planned with two reports")
	submission.ReportedCosts = []ReportedCost{
		{SchemaVersion: ReportedCostVersion, Source: "claude-code", Amount: "0.012500", Currency: "USD"},
		{SchemaVersion: ReportedCostVersion, Source: "litellm", Amount: "0.0131", Currency: "USD"},
	}
	if _, err := e.SubmitSession(context.Background(), submission); err != nil {
		t.Fatal(err)
	}
	view, err := e.View(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	attempt := view.Run.Attempts[task.AttemptID]
	if !reflect.DeepEqual(attempt.ReportedCosts, submission.ReportedCosts) {
		t.Fatalf("reported amounts were reconciled or changed: got %+v want %+v", attempt.ReportedCosts, submission.ReportedCosts)
	}
	if view.Run.SchemaVersion != CoreActionDeliveryStateVersion || view.SchemaVersion != CoreActionDeliveryReadVersion {
		t.Fatalf("reported cost used old state/read contracts: %s %s", view.Run.SchemaVersion, view.SchemaVersion)
	}
	next, err := e.Next(context.Background(), runID)
	if err != nil || next.SchemaVersion != CoreActionDeliveryNextVersion {
		t.Fatalf("reported cost used old next contract: %+v %v", next, err)
	}
	for name, value := range map[string]any{
		"CoreRunStateV21": view.Run, "CoreRunViewV21": view, "CoreNextViewV21": next,
		"SessionTaskV2": task, "SessionSubmissionV2": submission, "ReportedCost": submission.ReportedCosts[0],
	} {
		if err := validatePublic(t, name, value); err != nil {
			t.Fatalf("%s rejected an actual value: %v", name, err)
		}
	}
	events, err := e.Events(context.Background(), runID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range events.Events {
		if event.Type == "attempt.cost_reported" {
			found = bytes.Contains(event.Data, []byte(`"source":"claude-code"`)) && bytes.Contains(event.Data, []byte(`"amount":"0.012500"`))
		}
	}
	if !found {
		t.Fatal("journal does not retain the exact named cost report")
	}
}

func TestAssistedReportedCostRejectsAmbiguousOrInexactClaims(t *testing.T) {
	valid := ReportedCost{SchemaVersion: ReportedCostVersion, Source: "claude-code", Amount: "0", Currency: "USD"}
	for name, costs := range map[string][]ReportedCost{
		"missing version":  {{Source: valid.Source, Amount: valid.Amount, Currency: valid.Currency}},
		"unnamed source":   {{SchemaVersion: ReportedCostVersion, Amount: valid.Amount, Currency: valid.Currency}},
		"negative":         {{SchemaVersion: ReportedCostVersion, Source: valid.Source, Amount: "-1", Currency: valid.Currency}},
		"leading zero":     {{SchemaVersion: ReportedCostVersion, Source: valid.Source, Amount: "01", Currency: valid.Currency}},
		"exponent":         {{SchemaVersion: ReportedCostVersion, Source: valid.Source, Amount: "1e-2", Currency: valid.Currency}},
		"lower currency":   {{SchemaVersion: ReportedCostVersion, Source: valid.Source, Amount: valid.Amount, Currency: "usd"}},
		"duplicate source": {valid, valid},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateReportedCosts(costs); err == nil {
				t.Fatal("invalid cost claim was accepted")
			}
		})
	}
	if err := validateReportedCosts([]ReportedCost{valid}); err != nil {
		t.Fatalf("explicitly reported zero was rejected: %v", err)
	}

	e, runID, _ := assistedFixture(t)
	task := handOver(t, e, runID)
	legacy := hostResult(t, e, task, "legacy report")
	legacy.SchemaVersion = AssistedSessionVersion
	legacy.ReportedCosts = []ReportedCost{valid}
	if _, err := e.SubmitSession(context.Background(), legacy); err == nil {
		t.Fatal("legacy assisted-session contract accepted a reported cost")
	}
}

func TestAssistedHandoffCarriesPinnedSkillClaimAndDeadline(t *testing.T) {
	e, runID, claim := assistedFixture(t)
	task := handOver(t, e, runID)
	if task.PrincipalID != e.owner || task.ClaimID != claim.ID || task.ClaimGeneration != claim.Generation {
		t.Fatalf("the handoff does not name its principal and claim: %+v", task)
	}
	if len(task.SkillRefs) != 1 || task.SkillRefs[0].ID != "aif:context/plan-skill" {
		t.Fatalf("the handoff does not carry the pinned skill: %+v", task.SkillRefs)
	}
	if task.ClaimPath == "" || task.Deadline == "" || task.EnvelopeDigest == "" {
		t.Fatalf("the handoff omits its boundary: %+v", task)
	}
	skill, err := os.ReadFile(filepath.Join(e.Root, e.Config.Configuration.WorkspaceRoot, strings.TrimPrefix(task.AttemptID, "attempt:"), "context/skills/00"))
	if err != nil || rawDigest(skill) != task.SkillRefs[0].Digest {
		t.Fatalf("the skill bytes handed over are not the pinned ones: %v", err)
	}
	// Nothing was started: an assisted attempt has no process facts at all.
	r, _, err := e.load(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	attempt := r.Attempts[task.AttemptID]
	if attempt.Process != nil || attempt.ProcessOutcome != nil || attempt.Dispatch != nil {
		t.Fatalf("the assisted attempt recorded process facts: %+v", attempt)
	}
	if attempt.Session.HostState != SessionAwaiting {
		t.Fatalf("unexpected host state: %s", attempt.Session.HostState)
	}
}

func TestAssistedReportForAnotherAttemptOrEnvelopeIsRefused(t *testing.T) {
	e, runID, _ := assistedFixture(t)
	task := handOver(t, e, runID)
	ctx := context.Background()

	wrongEnvelope := hostResult(t, e, task, "planned")
	wrongEnvelope.EnvelopeDigest = rawDigest([]byte("another envelope"))
	if _, err := e.SubmitSession(ctx, wrongEnvelope); err == nil {
		t.Fatal("a report carrying another envelope digest was accepted")
	} else {
		rejectionCode(t, err, "envelope_conflict")
	}

	wrongAttempt := hostResult(t, e, task, "planned")
	wrongAttempt.AttemptID = "attempt:0000"
	if _, err := e.SubmitSession(ctx, wrongAttempt); err == nil {
		t.Fatal("a report about another attempt was accepted")
	} else {
		rejectionCode(t, err, "not_found")
	}

	// The handoff names its principal; a report for a different one is refused
	// even though this process is the enrolled owner.
	foreign, view, err := e.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	foreign.Attempts[task.AttemptID].Session.PrincipalID = "local:uid:999999"
	if _, err := e.apply(ctx, e.owner, newID("command"), runID, "diagnostic.recorded", map[string]any{"reassign": true}, &view.Snapshot.Version, local.CommandCAS, func(r *Run, _ local.Snapshot, obs Observation) (local.Change, error) {
		r.Attempts[task.AttemptID].Session.PrincipalID = "local:uid:999999"
		return local.Change{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.SubmitSession(ctx, hostResult(t, e, task, "planned")); err == nil {
		t.Fatal("a report was accepted for a handoff given to another principal")
	} else {
		rejectionCode(t, err, "session_identity_conflict")
	}
}

func TestAssistedReportIsRefusedUnderAStopAndAfterSettlement(t *testing.T) {
	e, runID, _ := assistedFixture(t)
	task := handOver(t, e, runID)
	ctx := context.Background()
	if _, err := e.Restrict(ctx, RestrictCommand{SchemaVersion: "1", CommandID: "command:stop", Scope: "run", ScopeID: runID, Kind: "pause", Reason: "held"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.SubmitSession(ctx, hostResult(t, e, task, "planned")); err == nil {
		t.Fatal("a report was accepted while the run was restricted")
	} else {
		rejectionCode(t, err, "dispatch_blocked")
	}
}

func TestAssistedReportClaimingAbsentOutputsIsRefusedByName(t *testing.T) {
	e, runID, _ := assistedFixture(t)
	task := handOver(t, e, runID)
	ctx := context.Background()
	submission := hostResult(t, e, task, "planned")
	r, _, err := e.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	slot := r.Attempts[task.AttemptID].Context.Outputs["plan"]
	if err := os.Remove(filepath.Join(r.Attempts[task.AttemptID].Workspace, filepath.FromSlash(slot.Path))); err != nil {
		t.Fatal(err)
	}
	_, err = e.SubmitSession(ctx, submission)
	problem, exit := ProblemFor(err)
	if problem.Code != "output_slot_empty" || exit != 2 {
		t.Fatalf("a report claiming an output it never produced was not refused by name: %+v %v", problem, err)
	}
	if len(problem.Violations) != 1 || problem.Violations[0].Pointer != "/result/outputs/plan" {
		t.Fatalf("the refusal did not name the port: %+v", problem.Violations)
	}
	after, _, err := e.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	attempt := after.Attempts[task.AttemptID]
	if attempt.Accepted != nil || attempt.Settled != nil || attempt.Session.HostState != SessionAwaiting {
		t.Fatalf("a report claiming outputs it never produced changed the attempt: %+v", attempt)
	}
	if attempt.ProcessOutcome != nil {
		t.Fatalf("the refused assisted report invented process facts: %+v", attempt.ProcessOutcome)
	}
}

func TestAssistedDisconnectStaysUnknownAndRefusesALateReport(t *testing.T) {
	e, runID, _ := assistedFixture(t)
	task := handOver(t, e, runID)
	ctx := context.Background()
	if _, err := e.MarkSessionDisconnected(ctx, runID, task.AttemptID); err == nil {
		t.Fatal("a live handoff was declared disconnected before its deadline")
	} else {
		rejectionCode(t, err, "deadline_not_reached")
	}
	// Move the deadline into the past exactly as an expired handoff would be.
	_, view, err := e.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.apply(ctx, e.owner, newID("command"), runID, "diagnostic.recorded", map[string]any{"expire": true}, &view.Snapshot.Version, local.CommandCAS, func(r *Run, _ local.Snapshot, obs Observation) (local.Change, error) {
		attempt := r.Attempts[task.AttemptID]
		attempt.Deadline = attempt.Admitted
		return local.Change{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.MarkSessionDisconnected(ctx, runID, task.AttemptID); err != nil {
		t.Fatal(err)
	}
	after, _, err := e.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	attempt := after.Attempts[task.AttemptID]
	if attempt.Session.HostState != SessionDisconnected || after.Status != "uncertain" || !after.HasUnresolvedEffects {
		t.Fatalf("a vanished host did not leave an honest unknown: %s %s", attempt.Session.HostState, after.Status)
	}
	if attempt.Settled != nil || attempt.Accepted != nil {
		t.Fatalf("a vanished host settled its attempt: %+v", attempt)
	}
	if _, err := e.SubmitSession(ctx, hostResult(t, e, task, "late")); err == nil {
		t.Fatal("a late report reopened a disconnected handoff")
	} else {
		rejectionCode(t, err, "session_state_conflict")
	}
}

func TestAssistedReportSettlesTheStepOnItsOwnEvidence(t *testing.T) {
	e, runID, _ := assistedFixture(t)
	task := handOver(t, e, runID)
	ctx := context.Background()
	submission := hostResult(t, e, task, "rename the helper")
	if _, err := e.SubmitSession(ctx, submission); err != nil {
		t.Fatal(err)
	}
	r, _, err := e.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	attempt := r.Attempts[task.AttemptID]
	if attempt.Settled == nil || attempt.Status != "completed" || attempt.Accepted == nil || attempt.Accepted.Verdict != "pass" {
		t.Fatalf("the host report did not settle its step: %+v", attempt)
	}
	if attempt.ProcessOutcome != nil || attempt.Process != nil || attempt.ExecutorEnd != nil {
		t.Fatalf("assisted settlement recorded facts about a process that never ran: %+v", attempt)
	}
	if attempt.Session.HostState != SessionReported || attempt.Session.Reported == nil {
		t.Fatalf("the handoff does not record the report it settled on: %+v", attempt.Session)
	}
	sealed, _, err := e.Artifact(attempt.Accepted.Outputs["plan"])
	if err != nil {
		t.Fatalf("the declared output was not sealed: %v", err)
	}
	if sealed.Format != "json" {
		t.Fatalf("unexpected sealed output: %+v", sealed)
	}
	// A second report for a settled attempt is not a second settlement.
	if _, err := e.SubmitSession(ctx, submission); err == nil {
		t.Fatal("a settled attempt accepted another report")
	}
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatalf("the run did not continue past the assisted step: %v", err)
	}
	final, _, err := e.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != "completed" || final.Outcome == nil || *final.Outcome != "succeeded" {
		t.Fatalf("the assisted run did not reach its declared outcome: %s %+v", final.Status, final.Outcome)
	}
}

// A step that only produces a proposal declares no writes, so it is handed no
// worktree at all. Two such steps cannot be kept apart by convention alone,
// because neither is permitted to touch a shared one.
func TestProposalOnlyAssistedStepClaimsNoWorktree(t *testing.T) {
	for _, c := range []struct {
		name       string
		effect     string
		wantClaim  bool
		wantEffect string
	}{
		{"writing step keeps its worktree", "workspace_write", true, "write_inside_claimed_worktree"},
		{"proposal-only step is handed none", "none", false, "write_inside_declared_output_slot"},
	} {
		t.Run(c.name, func(t *testing.T) {
			plan := &flow.Plan{}
			plan.Resources = flow.ContextResources{}
			step := flow.StepDefinition{InstructionsRef: &flow.Ref{ID: "test:context/skill", Version: "1.0.0", Digest: "sha256:0"}}
			step.Effects.Class = c.effect
			if err := validateAssistedStep(plan, step); err != nil {
				t.Fatalf("a declared %s assisted step was refused: %v", c.effect, err)
			}
		})
	}
	// Any other effect class is still refused by name.
	plan := &flow.Plan{}
	plan.Resources = flow.ContextResources{}
	step := flow.StepDefinition{InstructionsRef: &flow.Ref{ID: "test:context/skill", Version: "1.0.0", Digest: "sha256:0"}}
	step.Effects.Class = "network_write"
	if err := validateAssistedStep(plan, step); err == nil {
		t.Fatal("an assisted step declared an effect class the contract does not admit")
	}
}

// Acceptance settles the attempt, so a report it rejects burns a step that
// never retries. Intake reads the same ports first: a malformed report is a
// refusal that names its port and leaves the handoff awaiting a corrected one.
func TestAssistedIntakeRefusesAMalformedReportWithoutBurningTheHandoff(t *testing.T) {
	e, runID, _ := assistedFixture(t)
	task := handOver(t, e, runID)
	submission := hostResult(t, e, task, "planned without its output")
	var body map[string]any
	if err := json.Unmarshal(submission.Result, &body); err != nil {
		t.Fatal(err)
	}
	body["outputs"] = map[string]any{}
	stripped, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	submission.Result = stripped
	_, err = e.SubmitSession(context.Background(), submission)
	problem, exit := ProblemFor(err)
	if problem.Code != "output_required_missing" || exit != 2 {
		t.Fatalf("a report missing a required output was not refused by name: %+v %v", problem, err)
	}
	if len(problem.Violations) != 1 || problem.Violations[0].Pointer != "/result/outputs/plan" {
		t.Fatalf("the refusal did not name the port: %+v", problem.Violations)
	}
	r := driverRun(t, e, runID)
	attempt := r.Attempts[task.AttemptID]
	if r.Status == "failed" || attempt.Settled != nil || attempt.Session.HostState != SessionAwaiting || len(attempt.Candidate) != 0 {
		t.Fatalf("a refused report was recorded against the attempt: status=%s attempt=%+v", r.Status, attempt)
	}
	if _, err := e.SubmitSession(context.Background(), hostResult(t, e, task, "planned")); err != nil {
		t.Fatalf("the corrected report was refused under the same envelope: %v", err)
	}
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	if settled := driverRun(t, e, runID); settled.Status == "failed" {
		t.Fatalf("the corrected report did not settle the run: %+v", settled.Diagnostics)
	}
}
