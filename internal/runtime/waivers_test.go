package runtime

import (
	"context"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

// CTRL-008: the meaningfulness checks are outside a waiver's reach entirely.
// Naming one is refused explicitly, so the boundary is stated rather than left
// to a lookup that happens not to match.
func TestProtectedCheckClassesAreNeverWaivable(t *testing.T) {
	e, _ := emptyRuntime(t)
	ctx := context.Background()
	for _, class := range protectedCheckClasses {
		_, err := e.Waive(ctx, WaiveRequest{
			CommandID: "command:waive-" + class, RunID: "run:any", StepID: "step:any",
			CheckRef: flow.Ref{ID: class, Version: "1.0.0", Digest: rawDigest([]byte(class))},
			Reason:   "try to waive a meaningfulness check",
		})
		if err == nil {
			t.Fatalf("%s was waivable", class)
		}
		rejectionCode(t, err, "protected_check")
	}
}

func TestWaiverIsRefusedForARunThatPredatesThem(t *testing.T) {
	e, options := emptyRuntime(t)
	ctx := context.Background()
	result, err := e.Start(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	runID := result.Receipt.RunID
	r, _, err := e.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if isWaiverState(r.SchemaVersion) {
		t.Fatalf("the foundation fixture unexpectedly carries waiver state: %s", r.SchemaVersion)
	}
	_, err = e.Waive(ctx, WaiveRequest{
		CommandID: "command:waive", RunID: runID, StepID: "step:any",
		CheckRef: flow.Ref{ID: "demo:check/quality", Version: "1.0.0", Digest: rawDigest([]byte("q"))},
		Reason:   "waive on a pinned older run",
	})
	rejectionCode(t, err, "unsupported_state")
}

// A recorded reduction must stay visible: a declared success that rested on a
// waived check is reported as completed_with_waivers, not as plain success.
func TestWaivedOutcomeStaysVisible(t *testing.T) {
	root := "invocation:root"
	run := &Run{SchemaVersion: CoreWaiverStateVersion, RootInvocationID: root}
	if got := outcomeWithWaivers(run, root, "succeeded"); got != "succeeded" {
		t.Fatalf("an untouched run reported a reduction: %s", got)
	}
	// The flag alone does not name a scope. Without a locatable covered step
	// the reduction is attributed to the whole Run rather than lost.
	run.WaiverApplied = true
	run.Waivers = []Waiver{{ID: "waiver:1", StepID: "step:gone", Status: "applied", AppliedTo: []string{"check:1"}}}
	if got := outcomeWithWaivers(run, root, "succeeded"); got != "completed_with_waivers" {
		t.Fatalf("the reduction disappeared from the outcome: %s", got)
	}
	// A waiver never upgrades a different outcome into success.
	for _, declared := range []string{"rejected", "no_work", "partial"} {
		if got := outcomeWithWaivers(run, root, declared); got != declared {
			t.Fatalf("%s was rewritten to %s", declared, got)
		}
	}
}

// A waiver names the step it covered. The reduction belongs to the scope that
// rested on it and to its ancestors, not to an unrelated sibling scope.
func TestWaivedOutcomeIsAttributedToItsOwnScope(t *testing.T) {
	root, left, right := "invocation:root", "invocation:left", "invocation:right"
	run := &Run{
		SchemaVersion: CoreWaiverStateVersion, RootInvocationID: root, WaiverApplied: true,
		Invocations: map[string]*Invocation{
			root:  {ID: root, Ready: []string{}, Inputs: map[string]ArtifactRef{}, Outputs: map[string]ArtifactRef{}},
			left:  {ID: left, ParentInvocationID: root, CallerActivationID: "activation:fan", BranchID: "left", Ready: []string{}, Inputs: map[string]ArtifactRef{}, Outputs: map[string]ArtifactRef{}},
			right: {ID: right, ParentInvocationID: root, CallerActivationID: "activation:fan", BranchID: "right", Ready: []string{}, Inputs: map[string]ArtifactRef{}, Outputs: map[string]ArtifactRef{}},
		},
		Activations: map[string]*Activation{"activation:work": {ID: "activation:work", InvocationID: left, StageID: "work", Kind: "step", StepID: "step:1"}},
		Steps:       map[string]*Step{"step:1": {ID: "step:1", ActivationID: "activation:work"}},
		Waivers:     []Waiver{{ID: "waiver:1", StepID: "step:1", Status: "applied", AppliedTo: []string{"check:1"}}},
	}
	if got := outcomeWithWaivers(run, left, "succeeded"); got != "completed_with_waivers" {
		t.Fatalf("the scope that rested on the waiver hid it: %s", got)
	}
	if got := outcomeWithWaivers(run, root, "succeeded"); got != "completed_with_waivers" {
		t.Fatalf("the reduction did not reach the enclosing scope: %s", got)
	}
	if got := outcomeWithWaivers(run, right, "succeeded"); got != "succeeded" {
		t.Fatalf("an unrelated scope inherited another scope's reduction: %s", got)
	}
	// A recorded but unapplied waiver reduces nothing.
	run.Waivers[0].Status, run.Waivers[0].AppliedTo = "active", nil
	if got := outcomeWithWaivers(run, left, "succeeded"); got != "succeeded" {
		t.Fatalf("an unapplied waiver reduced an outcome: %s", got)
	}
}

// A waiver covers exactly the check instance it names. Its neighbours, other
// steps and expired decisions are not covered.
func TestWaiverCoversOnlyItsOwnCheckInstance(t *testing.T) {
	ref := flow.Ref{ID: "demo:check/quality", Version: "1.0.0", Digest: rawDigest([]byte("q"))}
	other := flow.Ref{ID: "demo:check/neighbour", Version: "1.0.0", Digest: rawDigest([]byte("n"))}
	now := Observation{UTC: "2026-08-29T12:00:00Z"}
	run := &Run{SchemaVersion: CoreWaiverStateVersion, Waivers: []Waiver{{
		ID: "waiver:1", StepID: "step:1", CheckRef: ref, Status: "active",
		ExpiresAt: "2026-08-29T13:00:00Z",
	}}}
	if run.waiverFor("step:1", ref, now) == nil {
		t.Fatal("the waiver did not cover its own check")
	}
	if run.waiverFor("step:1", other, now) != nil {
		t.Fatal("a waiver covered a neighbouring check")
	}
	if run.waiverFor("step:2", ref, now) != nil {
		t.Fatal("a waiver covered the same check in another step")
	}
	// A boundary with no step instance cannot be covered by accident.
	if run.waiverFor("", ref, now) != nil {
		t.Fatal("a waiver covered a boundary with no step instance")
	}
	lapsed := Observation{UTC: "2026-08-29T14:00:00Z"}
	if run.waiverFor("step:1", ref, lapsed) != nil {
		t.Fatal("a lapsed waiver still covered its check")
	}
	run.Waivers[0].Status = "applied"
	if run.waiverFor("step:1", ref, now) != nil {
		t.Fatal("an already applied waiver was reused")
	}
}

func TestWaiverStateIsNotCarriedByOlderContracts(t *testing.T) {
	for _, version := range []string{StateVersion, CoreStateVersion, CoreInvocationStateVersion, CoreRepeatStateVersion, CoreContextStateVersion, CoreSessionStateVersion} {
		run := Run{SchemaVersion: version, Profile: flow.CoreProfile, Waivers: []Waiver{{ID: "waiver:1"}}}
		if supportedRun(run) {
			t.Fatalf("%s accepted a recorded waiver", version)
		}
	}
	if !isWaiverState(CoreWaiverStateVersion) || !isSessionState(CoreWaiverStateVersion) || !isContextState(CoreWaiverStateVersion) {
		t.Fatal("waiver state lost a capability its predecessors carried")
	}
}

// A scope that rested on a waived check must be able to say so. Reporting
// plain success would hide the reduction; reporting an outcome the workflow
// never declared would put an inexpressible value into its own result.
func TestFinishRefusesToReportAnUndeclaredReduction(t *testing.T) {
	root := "invocation:root"
	run := &Run{
		SchemaVersion: CoreWaiverStateVersion, RootInvocationID: root, WaiverApplied: true,
		Waivers: []Waiver{{ID: "waiver:1", StepID: "step:gone", Status: "applied", AppliedTo: []string{"check:1"}}},
	}
	plan := &flow.Plan{}
	plan.Workflow.AllowedOutcomes = []string{"succeeded", "rejected"}
	if _, err := reportedOutcome(run, plan, root, "succeeded"); err == nil {
		t.Fatal("a reduction was reported by a workflow that cannot express it")
	} else {
		rejectionCode(t, err, "undeclared_waived_outcome")
	}
	// An outcome the waiver never touches is unaffected by the rule.
	if outcome, err := reportedOutcome(run, plan, root, "rejected"); err != nil || outcome != "rejected" {
		t.Fatalf("an untouched outcome was refused: %s %v", outcome, err)
	}
	// Declaring the reduction lets the scope report it.
	plan.Workflow.AllowedOutcomes = append(plan.Workflow.AllowedOutcomes, "completed_with_waivers")
	if outcome, err := reportedOutcome(run, plan, root, "succeeded"); err != nil || outcome != "completed_with_waivers" {
		t.Fatalf("a declared reduction was not reported: %s %v", outcome, err)
	}
	// A scope that never rested on a waiver keeps reporting plain success.
	run.WaiverApplied = false
	if outcome, err := reportedOutcome(run, plan, root, "succeeded"); err != nil || outcome != "succeeded" {
		t.Fatalf("an untouched scope reported a reduction: %s %v", outcome, err)
	}
}
