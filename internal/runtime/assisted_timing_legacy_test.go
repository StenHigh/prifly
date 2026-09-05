package runtime

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"testing/synctest"
	"time"
)

// This is deliberately a legacy regression, not the new pause-aware promise:
// the old bridge accepts a late answer but keeps the original report deadline.
func TestLegacyAssistedDecisionAcceptsLateAnswerButRejectsResult(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		definition := DecisionDefinition{
			SchemaVersion: DecisionDefinitionVersion, ID: "continue", Title: "Continue", Phase: "runtime",
			Choices:     []DecisionChoice{{ID: "yes", Title: "Yes", Value: json.RawMessage(`true`)}},
			Sensitivity: "ordinary", Destination: DecisionDestination{Kind: "session_context", Name: "continue"},
		}
		catalog := DecisionCatalog{SchemaVersion: DecisionCatalogVersion, Decisions: []DecisionDefinition{definition}}
		catalogDigest, err := DecisionCatalogDigest(catalog)
		if err != nil {
			t.Fatal(err)
		}
		sheet := DecisionSheet{SchemaVersion: DecisionSheetVersion, CatalogDigest: catalogDigest, ProfileSource: "none", Records: []DecisionRecord{}}
		e, runID, _ := assistedWorkspaceFixtureWithDecisions(t, "", &catalog, &sheet)
		task := handOver(t, e, runID)
		before := driverRun(t, e, runID)
		attempt := before.Attempts[task.AttemptID]
		originalDeadline := attempt.Deadline
		originalEnvelope := string(attempt.Envelope)
		if before.SchemaVersion != "core-state/25" || task.SchemaVersion != "assisted-session/5" {
			t.Fatalf("fixture did not select the legacy contract: state=%s session=%s", before.SchemaVersion, task.SchemaVersion)
		}
		admitted, err := time.Parse(time.RFC3339Nano, attempt.Admitted.UTC)
		if err != nil {
			t.Fatal(err)
		}
		deadline, err := time.Parse(time.RFC3339Nano, originalDeadline.UTC)
		if err != nil || deadline.Sub(admitted) != time.Hour {
			t.Fatalf("legacy admission did not grant its original hour: deadline=%+v admitted=%+v err=%v", originalDeadline, attempt.Admitted, err)
		}

		// The standard-library bubble advances the clock, never persisted state
		// or the machine clock. No real ten-minute or two-week wait takes place.
		time.Sleep(10 * time.Minute)
		definitionDigest, err := DecisionDefinitionDigest(definition)
		if err != nil {
			t.Fatal(err)
		}
		request := DecisionRequest{SchemaVersion: DecisionRequestVersion, RunID: runID, AttemptID: task.AttemptID, EnvelopeDigest: task.EnvelopeDigest, DecisionID: definition.ID, DefinitionDigest: definitionDigest, ExpectedRunVersion: task.RunVersion}
		if _, err := e.SubmitSession(ctx, SessionSubmission{SchemaVersion: task.SchemaVersion, RunID: runID, AttemptID: task.AttemptID, EnvelopeDigest: task.EnvelopeDigest, DecisionRequest: &request}); err != nil {
			t.Fatal(err)
		}
		if _, err := e.SessionTask(ctx, runID, task.AttemptID); refusalCode(err) != "no_active_handoff" {
			t.Fatalf("pending question still exposed an executable delivery: %v", err)
		}
		root := e.Root
		if err := e.Close(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(14 * 24 * time.Hour)
		e, err = Open(root, false)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = e.Close() })
		next, err := e.Next(ctx, runID)
		if err != nil || next.Action != "waiting_decision" {
			t.Fatalf("legacy pending question was not preserved after restart: %+v %v", next, err)
		}
		requestDigest, err := DecisionRequestDigest(request)
		if err != nil {
			t.Fatal(err)
		}
		answer := DecisionAnswer{SchemaVersion: DecisionAnswerVersion, RunID: runID, DecisionID: definition.ID, DefinitionDigest: definitionDigest, RequestDigest: requestDigest, ExpectedRunVersion: next.RunVersion, Value: json.RawMessage(`true`)}
		if _, err := e.AnswerDecision(ctx, answer); err != nil {
			t.Fatalf("legacy bridge no longer reproduces acceptance of the late answer: %v", err)
		}
		resumed, err := e.SessionTask(ctx, runID, task.AttemptID)
		if err != nil || resumed.AttemptID != task.AttemptID || resumed.EnvelopeDigest == task.EnvelopeDigest || string(resumed.DecisionContext["continue"]) != "true" {
			t.Fatalf("legacy answer did not expose its new delivery: %+v %v", resumed, err)
		}
		answered := driverRun(t, e, runID)
		attempt = answered.Attempts[task.AttemptID]
		if answered.SchemaVersion != before.SchemaVersion || attempt.Deadline != originalDeadline || string(attempt.Envelope) != originalEnvelope {
			t.Fatal("restart or answer changed the legacy contract, original envelope, or deadline")
		}
		if answered.PendingDecision != nil || len(answered.DecisionLedger) != 1 || answered.DecisionLedger[0].Status != "answered" || answered.DecisionLedger[0].Source != "actor" || string(answered.DecisionLedger[0].Value) != "true" {
			t.Fatalf("late answer was not retained as the actor's decision: %+v", answered.DecisionLedger)
		}
		if !time.Now().After(deadline) {
			t.Fatal("test did not actually advance beyond the pinned deadline")
		}
		_, err = e.SubmitSession(ctx, hostResult(t, e, resumed, "late result"))
		rejectionCode(t, err, "attempt_deadline_expired")
		after := driverRun(t, e, runID)
		attempt = after.Attempts[task.AttemptID]
		if len(after.Attempts) != 1 || attempt.Accepted != nil || attempt.Settled != nil || attempt.ProcessOutcome != nil || after.Outcome != nil {
			t.Fatalf("late result settled, retried, or invented process facts: %+v", attempt)
		}
	})
}

// This records a pre-existing isolation gap, not permission for new timed
// sessions to hand another Run the checkout retained by a parked Attempt.
func TestLegacyAssistedRunsShareInstallationClaim(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	ctx := context.Background()
	e, firstID, claim := assistedWorkspaceFixture(t, "checkout")
	if _, err := e.SetAdmissionCapacity(ctx, CapacityRequest{CommandID: newID("command"), Capacity: 2, Reason: "reproduce legacy claim sharing across Runs"}); err != nil {
		t.Fatal(err)
	}
	first := handOver(t, e, firstID)
	started, err := e.Start(ctx, StartOptions{CommandID: newID("command"), WorkflowFile: "workflows/pilot.json", BriefFile: "brief.json", Inputs: map[string]string{}, WorkspaceMode: "checkout"})
	if err != nil {
		t.Fatal(err)
	}
	second := handOver(t, e, started.Receipt.RunID)
	if first.RunID == second.RunID || first.AttemptID == second.AttemptID {
		t.Fatal("fixture did not admit distinct Runs and Attempts")
	}
	if first.ClaimID != claim.ID || second.ClaimID != claim.ID || first.ClaimGeneration != claim.Generation || second.ClaimGeneration != claim.Generation || first.RepositoryWorkspace != claim.Repository.Toplevel || second.RepositoryWorkspace != first.RepositoryWorkspace {
		t.Fatalf("legacy Runs no longer reproduce sharing one claimed checkout: first=%+v second=%+v claim=%+v", first, second, claim)
	}
	capacity, held, err := e.AdmissionCapacity(ctx)
	if err != nil || capacity != 2 || len(held) != 2 || held[first.AttemptID] != first.RunID || held[second.AttemptID] != second.RunID {
		t.Fatalf("both deliveries were not admitted concurrently: capacity=%d held=%v err=%v", capacity, held, err)
	}
	for _, task := range []SessionTask{first, second} {
		r := driverRun(t, e, task.RunID)
		attempt := r.Attempts[task.AttemptID]
		if r.SchemaVersion != "core-state/23" || task.SchemaVersion != "assisted-session/3" || attempt.Settled != nil || attempt.Accepted != nil || attempt.Session.HostState != SessionAwaiting {
			t.Fatalf("the shared checkout was not held by an unsettled legacy delivery: %+v", attempt)
		}
	}
}
