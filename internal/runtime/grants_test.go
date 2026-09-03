package runtime

import (
	"context"
	"testing"
)

func issuedGrant(t *testing.T, e *Engine, ctx context.Context, operations int64) ControlGrant {
	t.Helper()
	result, err := e.IssueControlGrant(ctx, ControlGrantRequest{
		CommandID: "command:grant", SubjectID: e.owner, Capabilities: []string{"stop.release"},
		MaxOperations: operations, LifetimeMS: 60000, Reason: "delegate bounded releases for the pilot",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Receipt.Rejection != nil {
		t.Fatalf("grant refused: %+v", result.Receipt.Rejection)
	}
	control, _, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(control.Grants) != 1 {
		t.Fatalf("expected exactly one grant: %+v", control.Grants)
	}
	return control.Grants[0]
}

func TestGrantSpendsOneOperationPerAdmissionAndExhausts(t *testing.T) {
	e, _ := emptyRuntime(t)
	ctx := context.Background()
	grant := issuedGrant(t, e, ctx, 1)
	if grant.Grant.MaxLogicalOperations != 1 || grant.UsedCount != 0 || grant.Grant.Status != "active" {
		t.Fatalf("unexpected grant: %+v", grant)
	}
	stop := openStop(t, e, ctx)
	control, _, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	released, err := e.ReleaseControl(ctx, ControlReleaseRequest{CommandID: "command:release", Scope: "project", ExpectedControlEpoch: control.ControlEpoch, Stops: []StopGeneration{{ID: stop.ID, Generation: stop.Generation}}, GrantID: grant.Grant.ID, Reason: "release under the grant"})
	if err != nil {
		t.Fatal(err)
	}
	if released.Receipt.Rejection != nil {
		t.Fatalf("the grant did not admit its own operation: %+v", released.Receipt.Rejection)
	}
	spent, _, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	used := spent.Grants[0]
	if used.UsedCount != 1 || len(used.Uses) != 1 || used.Grant.Status != "exhausted" {
		t.Fatalf("the bound did not move with the admission: %+v", used)
	}
	if used.Uses[0].AdmissionID == "" || used.Uses[0].Operation != "stop.release" {
		t.Fatalf("the use was not recorded against its admission: %+v", used.Uses[0])
	}

	// A second release cannot draw on an exhausted bound.
	second := openStopWith(t, e, ctx, "command:stop-2")
	current, _, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	again, err := e.ReleaseControl(ctx, ControlReleaseRequest{CommandID: "command:release-2", Scope: "project", ExpectedControlEpoch: current.ControlEpoch, Stops: []StopGeneration{{ID: second.ID, Generation: second.Generation}}, GrantID: grant.Grant.ID, Reason: "spend an exhausted grant"})
	if err != nil {
		t.Fatal(err)
	}
	if again.Receipt.Rejection == nil || again.Receipt.Rejection.Code != "grant_not_admissible" {
		t.Fatalf("an exhausted grant admitted another operation: %+v", again.Receipt)
	}
}

func TestGrantCannotDeliverRightsItsSubjectLacks(t *testing.T) {
	e, _ := emptyRuntime(t)
	ctx := context.Background()
	if _, _, err := e.ensureControl(ctx); err != nil {
		t.Fatal(err)
	}
	_, err := e.IssueControlGrant(ctx, ControlGrantRequest{
		CommandID: "command:grant", SubjectID: "local:uid:999999", Capabilities: []string{"stop.release"},
		MaxOperations: 1, LifetimeMS: 60000, Reason: "delegate to a principal that holds nothing",
	})
	rejectionCode(t, err, "object_access_denied")
	control, _, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(control.Grants) != 0 {
		t.Fatalf("a refused grant was recorded: %+v", control.Grants)
	}
}

func TestGrantIssuedToAnotherSubjectIsNotUsableHere(t *testing.T) {
	e, _ := emptyRuntime(t)
	ctx := context.Background()
	grant := issuedGrant(t, e, ctx, 2)
	// Reassign the subject exactly as a grant for someone else would look.
	control, version, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	control.Grants[0].Grant.SubjectID = "local:uid:999999"
	if err := overwriteControl(t, e, ctx, control, version); err != nil {
		t.Fatal(err)
	}
	stop := openStop(t, e, ctx)
	current, _, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err := e.ReleaseControl(ctx, ControlReleaseRequest{CommandID: "command:release", Scope: "project", ExpectedControlEpoch: current.ControlEpoch, Stops: []StopGeneration{{ID: stop.ID, Generation: stop.Generation}}, GrantID: grant.Grant.ID, Reason: "use another subject's grant"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Receipt.Rejection == nil || result.Receipt.Rejection.Code != "grant_subject_conflict" {
		t.Fatalf("a grant issued to another subject was spent: %+v", result.Receipt)
	}
}

func TestRevokedGrantStopsNewUseButKeepsRecordedOnes(t *testing.T) {
	e, _ := emptyRuntime(t)
	ctx := context.Background()
	grant := issuedGrant(t, e, ctx, 5)
	stop := openStop(t, e, ctx)
	control, _, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.ReleaseControl(ctx, ControlReleaseRequest{CommandID: "command:release", Scope: "project", ExpectedControlEpoch: control.ControlEpoch, Stops: []StopGeneration{{ID: stop.ID, Generation: stop.Generation}}, GrantID: grant.Grant.ID, Reason: "first release"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.RevokeControlGrant(ctx, ControlGrantRevoke{CommandID: "command:revoke", GrantID: grant.Grant.ID, Reason: "withdrawn"}); err != nil {
		t.Fatal(err)
	}
	revoked, _, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if revoked.Grants[0].Grant.Status != "revoked" || len(revoked.Grants[0].Uses) != 1 {
		t.Fatalf("revocation rewrote what already happened: %+v", revoked.Grants[0])
	}
	second := openStopWith(t, e, ctx, "command:stop-2")
	current, _, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err := e.ReleaseControl(ctx, ControlReleaseRequest{CommandID: "command:release-2", Scope: "project", ExpectedControlEpoch: current.ControlEpoch, Stops: []StopGeneration{{ID: second.ID, Generation: second.Generation}}, GrantID: grant.Grant.ID, Reason: "use a revoked grant"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Receipt.Rejection == nil || result.Receipt.Rejection.Code != "grant_not_admissible" {
		t.Fatalf("a revoked grant admitted an operation: %+v", result.Receipt)
	}
}

func TestExpiredGrantIsNotAdmissible(t *testing.T) {
	e, _ := emptyRuntime(t)
	ctx := context.Background()
	grant := issuedGrant(t, e, ctx, 5)
	control, version, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	control.Grants[0].Grant.ExpiresAt = "2000-01-01T00:00:00Z"
	if err := overwriteControl(t, e, ctx, control, version); err != nil {
		t.Fatal(err)
	}
	stop := openStop(t, e, ctx)
	current, _, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err := e.ReleaseControl(ctx, ControlReleaseRequest{CommandID: "command:release", Scope: "project", ExpectedControlEpoch: current.ControlEpoch, Stops: []StopGeneration{{ID: stop.ID, Generation: stop.Generation}}, GrantID: grant.Grant.ID, Reason: "use a lapsed grant"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Receipt.Rejection == nil || result.Receipt.Rejection.Code != "grant_not_admissible" {
		t.Fatalf("a lapsed grant admitted an operation: %+v", result.Receipt)
	}
	view := e.ControlGrantView(current)["grants"].([]map[string]any)
	if view[0]["effective_status"] != "expired" {
		t.Fatalf("the lapsed grant is not visible as expired: %+v", view[0])
	}
}

// CTRL-007: a grant bounds when a decision is made; it is not the way to skip
// one. Issuing it is therefore gated exactly like the operation it delegates.
func TestGrantIssuanceIsItselfGatedByTheApprovalPolicy(t *testing.T) {
	e, _ := emptyRuntime(t)
	ctx := context.Background()
	if _, err := e.SetControlApprovalPolicy(ctx, ControlApprovalPolicyRequest{
		CommandID: "command:policy", Operations: []string{"grant.issue"}, Quorum: 1,
		Independence: "none", Reason: "delegation needs a recorded decision",
	}); err != nil {
		t.Fatal(err)
	}
	result, err := e.IssueControlGrant(ctx, ControlGrantRequest{
		CommandID: "command:grant", SubjectID: e.owner, Capabilities: []string{"stop.release"},
		MaxOperations: 1, LifetimeMS: 60000, Reason: "issue without a decision",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Receipt.Rejection == nil || result.Receipt.Rejection.Code != "approval_required" {
		t.Fatalf("a gated grant was issued with no decision: %+v", result.Receipt)
	}
	control, _, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(control.Grants) != 0 {
		t.Fatalf("a refused issuance recorded a grant: %+v", control.Grants)
	}
}

func TestReleaseTakesEitherApprovalsOrOneGrant(t *testing.T) {
	e, _ := emptyRuntime(t)
	ctx := context.Background()
	grant := issuedGrant(t, e, ctx, 2)
	stop := openStop(t, e, ctx)
	control, _, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err := e.ReleaseControl(ctx, ControlReleaseRequest{CommandID: "command:release", Scope: "project", ExpectedControlEpoch: control.ControlEpoch, Stops: []StopGeneration{{ID: stop.ID, Generation: stop.Generation}}, GrantID: grant.Grant.ID, Approvals: []string{"approval:none"}, Reason: "present both"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Receipt.Rejection == nil || result.Receipt.Rejection.Code != "decision_conflict" {
		t.Fatalf("a release presented two kinds of decision: %+v", result.Receipt)
	}
}

// Permission filtering precedes selection: a principal without read access must
// not learn a count, a cursor page or a receipt.
func TestReadsAreRefusedWithoutCurrentReadAccess(t *testing.T) {
	e, _ := emptyRuntime(t)
	ctx := context.Background()
	if _, _, err := e.ensureControl(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Telemetry(ctx, TelemetryQuery{SchemaVersion: TelemetryQueryVersion, Mode: "catalog"}); err != nil {
		t.Fatalf("the owner could not read its own records: %v", err)
	}
	control, version, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for i := range control.Access {
		operations := []string{}
		for _, operation := range control.Access[i].Operations {
			if operation != ControlOperationRead {
				operations = append(operations, operation)
			}
		}
		control.Access[i].Operations = operations
	}
	if err := overwriteControl(t, e, ctx, control, version); err != nil {
		t.Fatal(err)
	}
	// Read paths never enrol or reconcile, so the refusal is observed against
	// the state exactly as written.
	if _, err := e.readAccess(ctx); err == nil {
		t.Fatal("a principal without read access was admitted to read")
	} else {
		rejectionCode(t, err, "object_access_denied")
	}
	if _, err := e.Telemetry(ctx, TelemetryQuery{SchemaVersion: TelemetryQueryVersion, Mode: "catalog"}); err == nil {
		t.Fatal("telemetry was produced for a principal without read access")
	}
	if _, err := e.Receipt(ctx, "command:any"); err == nil {
		t.Fatal("a receipt was read without current access")
	}
}
