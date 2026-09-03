package runtime

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stenhigh/prifly/internal/local"
)

func TestControlPlaneEnrolsTheSessionPrincipal(t *testing.T) {
	e, _ := emptyRuntime(t)
	ctx := context.Background()
	if control, version, err := e.Control(ctx); err != nil || version != 0 || control.SchemaVersion != "" {
		t.Fatalf("a fresh installation reported a control plane: %v %d", err, version)
	}
	control, version, err := e.ensureControl(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if version != 1 || len(control.Principals) != 1 {
		t.Fatalf("enrolment did not commit exactly one principal: %d %+v", version, control.Principals)
	}
	principal := control.Principals[0]
	if principal.OwnerUID != os.Geteuid() || principal.ID != e.owner || principal.HumanID != "local:human:owner" {
		t.Fatalf("principal was not derived from the authenticated OS owner: %+v", principal)
	}
	if control.PolicyRef != e.Config.DefaultPolicyRef || control.ControlEpoch != 0 || len(control.Stops) != 0 {
		t.Fatalf("unexpected enrolled control state: %+v", control)
	}
	if !control.allows(e.owner, "project", e.Config.ID, ControlOperationAdmit) || control.allows("local:uid:999999", "project", e.Config.ID, ControlOperationAdmit) {
		t.Fatal("object access is not bound to the enrolled principal")
	}
	// Re-entry is idempotent: a second session does not enrol a second owner.
	again, againVersion, err := e.ensureControl(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if againVersion != version || len(again.Principals) != 1 {
		t.Fatalf("re-entry rewrote the control plane: %d %+v", againVersion, again.Principals)
	}
}

func TestProjectStopForbidsNewAdmissionAndReleaseRestoresIt(t *testing.T) {
	e, options := emptyRuntime(t)
	ctx := context.Background()
	stop, err := e.RestrictControl(ctx, ControlRestrictRequest{CommandID: "command:control-stop", Scope: "project", Reason: "pilot boundary check"})
	if err != nil {
		t.Fatal(err)
	}
	if stop.Receipt.Rejection != nil {
		t.Fatalf("stop rejected: %+v", stop.Receipt.Rejection)
	}
	control, _, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(control.Stops) != 1 || control.Stops[0].Status != "active" || control.ControlEpoch != 1 {
		t.Fatalf("stop was not recorded monotonically: %+v", control)
	}
	// A control stop is an authority object, never a Run.
	runs, _, err := e.Store.ReadAll(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("a global stop invented %d runs", len(runs))
	}
	if _, err := e.Start(ctx, options); err == nil {
		t.Fatal("a project stop admitted a new run")
	} else {
		rejectionCode(t, err, "control_stop_active")
	}
	recorded := control.Stops[0]
	stale, err := e.ReleaseControl(ctx, ControlReleaseRequest{CommandID: "command:control-stale", Scope: "project", ExpectedControlEpoch: 0, Stops: []StopGeneration{{ID: recorded.ID, Generation: recorded.Generation}}, Reason: "stale epoch"})
	if err != nil {
		t.Fatal(err)
	}
	if stale.Receipt.Rejection == nil || stale.Receipt.Rejection.Code != "control_epoch_conflict" {
		t.Fatalf("a stale release removed a later restriction: %+v", stale.Receipt)
	}
	released, err := e.ReleaseControl(ctx, ControlReleaseRequest{CommandID: "command:control-release", Scope: "project", ExpectedControlEpoch: 1, Stops: []StopGeneration{{ID: recorded.ID, Generation: recorded.Generation}}, Reason: "pilot boundary check done"})
	if err != nil {
		t.Fatal(err)
	}
	if released.Receipt.Rejection != nil {
		t.Fatalf("release rejected: %+v", released.Receipt.Rejection)
	}
	var admission struct {
		SchemaVersion string   `json:"schema_version"`
		ID            string   `json:"id"`
		Scope         string   `json:"scope"`
		ScopeID       string   `json:"scope_id"`
		CommandID     string   `json:"command_id"`
		IntentDigest  string   `json:"intent_digest"`
		Approvals     []string `json:"approval_refs"`
		Epoch         int64    `json:"control_epoch"`
		AdmittedAt    string   `json:"admitted_at"`
	}
	if err := decode(released.Receipt.Result, &admission); err != nil {
		t.Fatal(err)
	}
	if admission.Scope != "project" || admission.CommandID != "command:control-release" || admission.IntentDigest == "" || admission.Epoch != 2 {
		t.Fatalf("release did not commit a bound control admission: %+v", admission)
	}
	after, _, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.Stops[0].Status != "released" || after.Stops[0].Released == nil {
		t.Fatalf("stop was not released: %+v", after.Stops[0])
	}
	repeat, err := e.ReleaseControl(ctx, ControlReleaseRequest{CommandID: "command:control-repeat", Scope: "project", ExpectedControlEpoch: 2, Stops: []StopGeneration{{ID: recorded.ID, Generation: recorded.Generation}}, Reason: "already released"})
	if err != nil {
		t.Fatal(err)
	}
	if repeat.Receipt.Rejection == nil || repeat.Receipt.Rejection.Code != "stop_generation_conflict" {
		t.Fatalf("an already released stop was released twice: %+v", repeat.Receipt)
	}
	// The control pin is part of the request digest, so re-using the identity of
	// the stopped attempt is a conflict, not a late success. The original
	// decision stays readable through its receipt.
	if _, err := e.Start(ctx, options); !errors.Is(err, local.ErrCommandConflict) {
		t.Fatalf("a stopped command identity was reused for a new admission: %v", err)
	}
	if receipt, err := e.Receipt(ctx, options.CommandID); err != nil || receipt.Rejection == nil || receipt.Rejection.Code != "control_stop_active" {
		t.Fatalf("the original stop decision is no longer readable: %v %+v", err, receipt)
	}
	options.CommandID = "command:empty-after-release"
	if _, err := e.Start(ctx, options); err != nil {
		t.Fatalf("release did not restore admission: %v", err)
	}
}

func TestInstallationStopCoversTheProject(t *testing.T) {
	e, options := emptyRuntime(t)
	ctx := context.Background()
	if _, err := e.RestrictControl(ctx, ControlRestrictRequest{CommandID: "command:installation-stop", Scope: "installation", Reason: "whole installation halted"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Start(ctx, options); err == nil {
		t.Fatal("an installation stop admitted a project run")
	} else {
		rejectionCode(t, err, "control_stop_active")
	}
	// A project release cannot remove an installation stop.
	control, _, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	recorded := control.Stops[0]
	wrongScope, err := e.ReleaseControl(ctx, ControlReleaseRequest{CommandID: "command:wrong-scope", Scope: "project", ExpectedControlEpoch: 1, Stops: []StopGeneration{{ID: recorded.ID, Generation: recorded.Generation}}, Reason: "wrong scope"})
	if err != nil {
		t.Fatal(err)
	}
	if wrongScope.Receipt.Rejection == nil || wrongScope.Receipt.Rejection.Code != "stop_generation_conflict" {
		t.Fatalf("a project release removed an installation stop: %+v", wrongScope.Receipt)
	}
	if _, err := e.RestrictControl(ctx, ControlRestrictRequest{CommandID: "command:bad-scope", Scope: "run", Reason: "run scope"}); err == nil {
		t.Fatal("authority control accepted a run scope")
	}
}

// A stop that commits after an admission was evaluated must not be overtaken by
// it. The pin makes that ordering a rejection instead of a silent admission.
func TestControlPinRejectsAnAdmissionPreparedBeforeAStop(t *testing.T) {
	e, options := emptyRuntime(t)
	ctx := context.Background()
	pin, blocked, err := e.admissionGate(ctx)
	if err != nil || blocked != nil {
		t.Fatalf("gate refused a clean installation: %v %v", err, blocked)
	}
	if _, err := e.RestrictControl(ctx, ControlRestrictRequest{CommandID: "command:late-stop", Scope: "project", Reason: "committed after the gate"}); err != nil {
		t.Fatal(err)
	}
	zero := int64(0)
	payload, err := canonical(map[string]any{"command_id": options.CommandID})
	if err != nil {
		t.Fatal(err)
	}
	result, err := e.Store.Apply(ctx, local.Command{ID: "command:stale-admission", Actor: e.owner, RunID: "run:stale", Payload: payload, ExpectedVersion: &zero, Mode: local.CommandCAS, Control: pin}, func(local.Snapshot) (local.Change, error) {
		t.Fatal("a stale pin reached the transform")
		return local.Change{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Receipt.Rejection == nil || result.Receipt.Rejection.Code != "control_conflict" {
		t.Fatalf("a stale control pin was admitted: %+v", result.Receipt)
	}
}
