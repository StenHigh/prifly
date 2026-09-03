package runtime

import (
	"context"
	"testing"

	"github.com/stenhigh/prifly/internal/local"
)

// gatedRuntime selects a control policy that gates stop.release, so releasing a
// stop stops being a single-command action.
func gatedRuntime(t *testing.T, quorum int, independence string) (*Engine, context.Context) {
	t.Helper()
	e, _ := emptyRuntime(t)
	ctx := context.Background()
	result, err := e.SetControlApprovalPolicy(ctx, ControlApprovalPolicyRequest{
		CommandID: "command:policy", Operations: []string{"stop.release"}, Quorum: quorum,
		Independence: independence, Reason: "control releases need a recorded decision",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Receipt.Rejection != nil {
		t.Fatalf("policy rejected: %+v", result.Receipt.Rejection)
	}
	return e, ctx
}

func openStop(t *testing.T, e *Engine, ctx context.Context) AuthorityStop {
	t.Helper()
	if _, err := e.RestrictControl(ctx, ControlRestrictRequest{CommandID: "command:stop", Scope: "project", Reason: "held for the decision"}); err != nil {
		t.Fatal(err)
	}
	control, _, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return control.Stops[len(control.Stops)-1]
}

func TestGatedReleaseRefusesWithoutAnApprovedDecision(t *testing.T) {
	e, ctx := gatedRuntime(t, 1, "none")
	stop := openStop(t, e, ctx)
	control, _, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err := e.ReleaseControl(ctx, ControlReleaseRequest{CommandID: "command:bare", Scope: "project", ExpectedControlEpoch: control.ControlEpoch, Stops: []StopGeneration{{ID: stop.ID, Generation: stop.Generation}}, Reason: "no decision"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Receipt.Rejection == nil || result.Receipt.Rejection.Code != "approval_required" {
		t.Fatalf("a gated release was admitted with no decision: %+v", result.Receipt)
	}
	after, _, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.Stops[0].Status != "active" {
		t.Fatal("the refused release still freed its stop")
	}
}

// CTRL-004: one owner is one human. An independent quorum of two cannot be
// formed by the same person, and the installation refuses instead of counting
// a second technical account as a second approver.
func TestSingleOwnerCannotFormAnIndependentQuorum(t *testing.T) {
	e, ctx := gatedRuntime(t, 2, "pairwise_distinct")
	stop := openStop(t, e, ctx)
	intent := stopReleaseIntentDigest(t, e, ctx, "command:release", stop)
	requested, err := e.RequestControlApproval(ctx, ApprovalRequest{CommandID: "command:request", Operation: "stop.release", IntentDigest: intent, Reason: "release the pilot stop"})
	if err != nil {
		t.Fatal(err)
	}
	if requested.Receipt.Rejection != nil {
		t.Fatalf("request rejected: %+v", requested.Receipt.Rejection)
	}
	control, _, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	approval := control.Approvals[0]
	if approval.RequiredApprovals != 2 {
		t.Fatalf("the frozen quorum is wrong: %+v", approval)
	}
	// The requester is this owner, so the first vote is already refused.
	vote, err := e.DecideControlApproval(ctx, ApprovalDecision{CommandID: "command:vote", ApprovalID: approval.ID, Decision: "approve", Reason: "I approve my own request"})
	if err != nil {
		t.Fatal(err)
	}
	if vote.Receipt.Rejection == nil || vote.Receipt.Rejection.Code != "independence_violated" {
		t.Fatalf("the requester approved their own decision: %+v", vote.Receipt)
	}

	after, _, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.Approvals[0].Status != "pending" || len(after.Approvals[0].VoteRefs) != 0 {
		t.Fatalf("a refused vote was recorded: %+v", after.Approvals[0])
	}
}

func TestApprovalCountsHumansNotAccounts(t *testing.T) {
	e, ctx := gatedRuntime(t, 2, "none")
	stop := openStop(t, e, ctx)
	intent := stopReleaseIntentDigest(t, e, ctx, "command:release", stop)
	if _, err := e.RequestControlApproval(ctx, ApprovalRequest{CommandID: "command:request", Operation: "stop.release", IntentDigest: intent, Reason: "release"}); err != nil {
		t.Fatal(err)
	}
	control, _, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	approval := control.Approvals[0]
	if _, err := e.DecideControlApproval(ctx, ApprovalDecision{CommandID: "command:vote-1", ApprovalID: approval.ID, Decision: "approve", Reason: "first"}); err != nil {
		t.Fatal(err)
	}
	// A second vote from the same human is not a second approver, even under a
	// policy that permits the requester to vote.
	second, err := e.DecideControlApproval(ctx, ApprovalDecision{CommandID: "command:vote-2", ApprovalID: approval.ID, Decision: "approve", Reason: "again"})
	if err != nil {
		t.Fatal(err)
	}
	if second.Receipt.Rejection == nil || second.Receipt.Rejection.Code != "duplicate_vote" {
		t.Fatalf("one human voted twice: %+v", second.Receipt)
	}

	after, _, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.Approvals[0].Status != "pending" || len(after.Approvals[0].VoteRefs) != 1 {
		t.Fatalf("quorum was reached by one human twice: %+v", after.Approvals[0])
	}
}

func TestApprovalIsConsumedOnceAndBoundToItsExactIntent(t *testing.T) {
	e, ctx := gatedRuntime(t, 1, "none")
	// Both stops exist before the decision, so the protected payload of the
	// release this approval covers does not move under it.
	stop := openStop(t, e, ctx)
	otherStop := openStopWith(t, e, ctx, "command:stop-2")
	intent := stopReleaseIntentDigest(t, e, ctx, "command:release", stop)
	otherIntent := stopReleaseIntentDigest(t, e, ctx, "command:release-other", otherStop)
	if otherIntent == intent {
		t.Fatal("fixture did not produce two distinct protected payloads")
	}
	if _, err := e.RequestControlApproval(ctx, ApprovalRequest{CommandID: "command:request", Operation: "stop.release", IntentDigest: intent, Reason: "release"}); err != nil {
		t.Fatal(err)
	}
	control, _, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	approval := control.Approvals[0]
	if _, err := e.DecideControlApproval(ctx, ApprovalDecision{CommandID: "command:vote", ApprovalID: approval.ID, Decision: "approve", Reason: "reviewed"}); err != nil {
		t.Fatal(err)
	}
	approved, _, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if approved.Approvals[0].Status != "approved" {
		t.Fatalf("quorum did not approve: %+v", approved.Approvals[0])
	}

	// A decision covers one protected payload and cannot be spent on another.
	wrong, err := e.ReleaseControl(ctx, ControlReleaseRequest{CommandID: "command:release-other", Scope: "project", ExpectedControlEpoch: approved.ControlEpoch, Stops: []StopGeneration{{ID: otherStop.ID, Generation: otherStop.Generation}}, Approvals: []string{approval.ID}, Reason: "reuse the decision"})
	if err != nil {
		t.Fatal(err)
	}
	if wrong.Receipt.Rejection == nil || wrong.Receipt.Rejection.Code != "approval_intent_conflict" {
		t.Fatalf("a decision was spent on another payload: %+v", wrong.Receipt)
	}

	released, err := e.ReleaseControl(ctx, ControlReleaseRequest{CommandID: "command:release", Scope: "project", ExpectedControlEpoch: approved.ControlEpoch, Stops: []StopGeneration{{ID: stop.ID, Generation: stop.Generation}}, Approvals: []string{approval.ID}, Reason: "release the held stop"})
	if err != nil {
		t.Fatal(err)
	}
	if released.Receipt.Rejection != nil {
		t.Fatalf("the approved release was refused: %+v", released.Receipt.Rejection)
	}
	consumed, _, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if consumed.Approvals[0].Status != "consumed" || consumed.Approvals[0].ConsumedByAdmissionID == "" {
		t.Fatalf("the decision was not consumed by its admission: %+v", consumed.Approvals[0])
	}
	late, err := e.RevokeControlApproval(ctx, ApprovalRevoke{CommandID: "command:revoke", ApprovalID: approval.ID, Reason: "too late"})
	if err != nil {
		t.Fatal(err)
	}
	if late.Receipt.Rejection == nil || late.Receipt.Rejection.Code != "approval_consumed" {
		t.Fatalf("a consumed decision was revoked after its admission: %+v", late.Receipt)
	}
}

func TestRevokedDecisionCannotBeConsumed(t *testing.T) {
	e, ctx := gatedRuntime(t, 1, "none")
	stop := openStop(t, e, ctx)
	intent := stopReleaseIntentDigest(t, e, ctx, "command:release", stop)
	if _, err := e.RequestControlApproval(ctx, ApprovalRequest{CommandID: "command:request", Operation: "stop.release", IntentDigest: intent, Reason: "release"}); err != nil {
		t.Fatal(err)
	}
	control, _, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	approval := control.Approvals[0]
	if _, err := e.DecideControlApproval(ctx, ApprovalDecision{CommandID: "command:vote", ApprovalID: approval.ID, Decision: "approve", Reason: "reviewed"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.RevokeControlApproval(ctx, ApprovalRevoke{CommandID: "command:revoke", ApprovalID: approval.ID, Reason: "withdrawn"}); err != nil {
		t.Fatal(err)
	}
	current, _, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err := e.ReleaseControl(ctx, ControlReleaseRequest{CommandID: "command:release", Scope: "project", ExpectedControlEpoch: current.ControlEpoch, Stops: []StopGeneration{{ID: stop.ID, Generation: stop.Generation}}, Approvals: []string{approval.ID}, Reason: "spend a revoked decision"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Receipt.Rejection == nil || result.Receipt.Rejection.Code != "approval_not_admissible" {
		t.Fatalf("a revoked decision was consumed: %+v", result.Receipt)
	}
	lateVote, err := e.DecideControlApproval(ctx, ApprovalDecision{CommandID: "command:late-vote", ApprovalID: approval.ID, Decision: "approve", Reason: "late"})
	if err != nil {
		t.Fatal(err)
	}
	if lateVote.Receipt.Rejection == nil || lateVote.Receipt.Rejection.Code != "approval_terminal" {
		t.Fatalf("a revoked decision accepted another vote: %+v", lateVote.Receipt)
	}
}

func TestRejectedDecisionIsTerminalAndAbsenceIsNotApproval(t *testing.T) {
	e, ctx := gatedRuntime(t, 1, "none")
	stop := openStop(t, e, ctx)
	intent := stopReleaseIntentDigest(t, e, ctx, "command:release", stop)
	if _, err := e.RequestControlApproval(ctx, ApprovalRequest{CommandID: "command:request", Operation: "stop.release", IntentDigest: intent, Reason: "release"}); err != nil {
		t.Fatal(err)
	}
	control, _, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	approval := control.Approvals[0]
	if _, err := e.DecideControlApproval(ctx, ApprovalDecision{CommandID: "command:vote", ApprovalID: approval.ID, Decision: "reject", Reason: "not now"}); err != nil {
		t.Fatal(err)
	}
	current, _, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if current.Approvals[0].Status != "rejected" {
		t.Fatalf("a reject vote did not resolve the decision: %+v", current.Approvals[0])
	}
	result, err := e.ReleaseControl(ctx, ControlReleaseRequest{CommandID: "command:release", Scope: "project", ExpectedControlEpoch: current.ControlEpoch, Stops: []StopGeneration{{ID: stop.ID, Generation: stop.Generation}}, Approvals: []string{approval.ID}, Reason: "ignore the refusal"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Receipt.Rejection == nil || result.Receipt.Rejection.Code != "approval_not_admissible" {
		t.Fatalf("a rejected decision admitted a release: %+v", result.Receipt)
	}
}

func TestExpiredDecisionIsRecordedAndNotConsumable(t *testing.T) {
	e, ctx := gatedRuntime(t, 1, "none")
	stop := openStop(t, e, ctx)
	intent := stopReleaseIntentDigest(t, e, ctx, "command:release", stop)
	if _, err := e.RequestControlApproval(ctx, ApprovalRequest{CommandID: "command:request", Operation: "stop.release", IntentDigest: intent, Reason: "release"}); err != nil {
		t.Fatal(err)
	}
	control, version, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	approval := control.Approvals[0]
	if _, err := e.DecideControlApproval(ctx, ApprovalDecision{CommandID: "command:vote", ApprovalID: approval.ID, Decision: "approve", Reason: "reviewed"}); err != nil {
		t.Fatal(err)
	}
	// Move consume_before into the past exactly as a lapsed decision would be.
	control, version, err = e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	control.Approvals[0].ConsumeBefore = "2000-01-01T00:00:00Z"
	payload, err := canonical(map[string]any{"operation": "test.expire"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Store.ApplyAuthority(ctx, local.AuthorityCommand{ID: "command:expire", Actor: e.owner, Key: AuthorityControlKey, Payload: payload, ExpectedVersion: &version}, func(local.AuthoritySnapshot) (local.AuthorityChange, error) {
		data, err := canonicalState(control)
		return local.AuthorityChange{Data: data}, err
	}); err != nil {
		t.Fatal(err)
	}
	current, _, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err := e.ReleaseControl(ctx, ControlReleaseRequest{CommandID: "command:release", Scope: "project", ExpectedControlEpoch: current.ControlEpoch, Stops: []StopGeneration{{ID: stop.ID, Generation: stop.Generation}}, Approvals: []string{approval.ID}, Reason: "spend a lapsed decision"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Receipt.Rejection == nil || result.Receipt.Rejection.Code != "approval_expired" {
		t.Fatalf("a lapsed decision admitted a release: %+v", result.Receipt)
	}
	after, _, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	view := e.ControlApprovalView(after)["approvals"].([]map[string]any)
	if view[0]["effective_status"] != "expired" {
		t.Fatalf("the closed window is not visible: %+v", view[0])
	}
}

// overwriteControl writes a control row directly, so a test can present the
// state a later condition would have produced without waiting for it.
func overwriteControl(t *testing.T, e *Engine, ctx context.Context, control AuthorityControl, version int64) error {
	t.Helper()
	payload, err := canonical(map[string]any{"operation": "test.overwrite", "version": version})
	if err != nil {
		return err
	}
	_, err = e.Store.ApplyAuthority(ctx, local.AuthorityCommand{ID: newID("command"), Actor: e.owner, Key: AuthorityControlKey, Payload: payload, ExpectedVersion: &version}, func(local.AuthoritySnapshot) (local.AuthorityChange, error) {
		data, err := canonicalState(control)
		return local.AuthorityChange{Data: data}, err
	})
	return err
}

func openStopWith(t *testing.T, e *Engine, ctx context.Context, command string) AuthorityStop {
	t.Helper()
	if _, err := e.RestrictControl(ctx, ControlRestrictRequest{CommandID: command, Scope: "project", Reason: "second hold"}); err != nil {
		t.Fatal(err)
	}
	control, _, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return control.Stops[len(control.Stops)-1]
}

// stopReleaseIntentDigest builds the exact protected payload a release of this
// stop will carry, so the approval is bound to that payload and nothing else.
func stopReleaseIntentDigest(t *testing.T, e *Engine, ctx context.Context, command string, stop AuthorityStop) string {
	t.Helper()
	control, _, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := canonical(map[string]any{
		"scope": "project", "scope_id": e.Config.ID, "expected_control_epoch": control.ControlEpoch,
		"stop_generations": []StopGeneration{{ID: stop.ID, Generation: stop.Generation}},
		"reason":           releaseReasonFor(command),
	})
	if err != nil {
		t.Fatal(err)
	}
	return rawDigest(payload)
}

func releaseReasonFor(command string) string {
	switch command {
	case "command:release":
		return "release the held stop"
	case "command:release-other":
		return "reuse the decision"
	}
	return "release"
}

// The stage acceptance asks for one explainable order under concurrency: two
// racing consumers of one decision must not both win, and the loser must say
// why rather than silently doing nothing.
func TestConcurrentConsumeOfOneDecisionHasOneExplainableOrder(t *testing.T) {
	e, ctx := gatedRuntime(t, 1, "none")
	first := openStop(t, e, ctx)
	second := openStopWith(t, e, ctx, "command:stop-2")
	intent := stopReleaseIntentDigest(t, e, ctx, "command:release", first)
	if _, err := e.RequestControlApproval(ctx, ApprovalRequest{CommandID: "command:request", Operation: "stop.release", IntentDigest: intent, Reason: "release"}); err != nil {
		t.Fatal(err)
	}
	control, _, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	approval := control.Approvals[0]
	if _, err := e.DecideControlApproval(ctx, ApprovalDecision{CommandID: "command:vote", ApprovalID: approval.ID, Decision: "approve", Reason: "reviewed"}); err != nil {
		t.Fatal(err)
	}
	approved, _, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Both releases name the same decision; only one protected payload matches.
	requests := []ControlReleaseRequest{
		{CommandID: "command:release", Scope: "project", ExpectedControlEpoch: approved.ControlEpoch, Stops: []StopGeneration{{ID: first.ID, Generation: first.Generation}}, Approvals: []string{approval.ID}, Reason: "release the held stop"},
		{CommandID: "command:release-other", Scope: "project", ExpectedControlEpoch: approved.ControlEpoch, Stops: []StopGeneration{{ID: second.ID, Generation: second.Generation}}, Approvals: []string{approval.ID}, Reason: "reuse the decision"},
	}
	type outcome struct {
		rejection string
		err       error
	}
	results := make(chan outcome, len(requests))
	for _, request := range requests {
		go func(request ControlReleaseRequest) {
			result, err := e.ReleaseControl(context.Background(), request)
			code := ""
			if result.Receipt.Rejection != nil {
				code = result.Receipt.Rejection.Code
			}
			results <- outcome{code, err}
		}(request)
	}
	admitted, refused := 0, 0
	for range requests {
		got := <-results
		if got.err != nil {
			t.Fatalf("a concurrent release failed instead of deciding: %v", got.err)
		}
		if got.rejection == "" {
			admitted++
			continue
		}
		refused++
		if got.rejection != "approval_intent_conflict" && got.rejection != "control_epoch_conflict" && got.rejection != "approval_not_admissible" {
			t.Fatalf("a concurrent release was refused without an explainable reason: %s", got.rejection)
		}
	}
	if admitted != 1 || refused != 1 {
		t.Fatalf("concurrent consume did not produce one order: admitted=%d refused=%d", admitted, refused)
	}
	consumed, _, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if consumed.Approvals[0].Status != "consumed" {
		t.Fatalf("the decision was not spent exactly once: %+v", consumed.Approvals[0])
	}
}
