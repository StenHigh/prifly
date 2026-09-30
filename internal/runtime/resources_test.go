package runtime

import (
	"context"
	"strings"
	"testing"
)

// Two Runs of one project may run side by side, and still not both run what
// the project said may not overlap: an attempt of a stage holding a full
// resource is refused naming a holder, runs nothing, and is admitted on the
// next drive once the holder settles.
func TestAResourceKeepsTwoRunsFromOverlappingOnAStage(t *testing.T) {
	t.Parallel()
	e, first, claim := assistedWorkspaceFixture(t, "worktree")
	ctx := context.Background()
	if _, err := e.SetAdmissionCapacity(ctx, CapacityRequest{CommandID: newID("command"), Capacity: 4, Reason: "Runs side by side"}); err != nil {
		t.Fatal(err)
	}
	start := func() string {
		t.Helper()
		own, err := e.ClaimWorktree(ctx, ClaimRequest{CommandID: newID("command"), Repository: claim.Repository.Toplevel, OwnerID: newID("session"), WorkspaceMode: "worktree"})
		if err != nil {
			t.Fatal(err)
		}
		result, err := e.Start(ctx, StartOptions{CommandID: newID("command"), WorkflowFile: "workflows/pilot.json", BriefFile: "brief.json", Inputs: map[string]string{}, WorkspaceMode: "worktree", WorkspaceClaim: &own, StageResources: map[string][]string{"plan": {"heavy"}}, ResourceLimits: map[string]int64{"heavy": 1}})
		if err != nil {
			t.Fatal(err)
		}
		return result.Receipt.RunID
	}
	a, b := start(), start()
	if r := driverRun(t, e, a); r.SchemaVersion != CoreResourceStateVersion || r.ResourceLimits["heavy"] != 1 {
		t.Fatalf("the Run did not seal its resources: %s %+v", r.SchemaVersion, r.ResourceLimits)
	}
	_ = first
	if err := e.Drive(ctx, a); err != nil {
		t.Fatal(err)
	}
	held := handOver(t, e, a)
	err := e.Drive(ctx, b)
	if refusalCode(err) != "resource_busy" || !strings.Contains(err.Error(), a) {
		t.Fatalf("a second holder of a resource limited to one was admitted or not told who holds it: %v", err)
	}
	if r := driverRun(t, e, b); len(r.Attempts) != 0 {
		t.Fatalf("a refused attempt left a trace: %+v", r.Attempts)
	}
	if _, err := e.SubmitSession(ctx, hostResult(t, e, held, "planned")); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(ctx, b); err != nil {
		t.Fatalf("the freed resource was not taken on the next drive: %v", err)
	}
	if r := driverRun(t, e, b); len(r.Attempts) != 1 {
		t.Fatalf("the waiting Run was not admitted: %+v", r.Attempts)
	}
	if err := validateInBundle(t, resourcePublicContracts, "CoreRunStateV42", driverRun(t, e, b)); err != nil {
		t.Fatalf("the published state rejects the sealed resources: %v", err)
	}
}

func TestAResourceDeclarationIsCheckedAtStart(t *testing.T) {
	t.Parallel()
	e, _, _ := assistedWorkspaceFixture(t, "")
	for _, c := range []struct {
		stages map[string][]string
		limits map[string]int64
		code   string
	}{
		{map[string][]string{"nowhere": {"heavy"}}, map[string]int64{"heavy": 1}, "unknown_resource_stage"},
		{map[string][]string{"plan": {"Heavy CPU"}}, map[string]int64{"Heavy CPU": 1}, "invalid_resource"},
		{map[string][]string{"plan": {"heavy"}}, map[string]int64{}, "invalid_resource"},
		{map[string][]string{"plan": {"heavy"}}, map[string]int64{"heavy": 0}, "invalid_resource"},
	} {
		_, err := e.Start(context.Background(), StartOptions{CommandID: newID("command"), WorkflowFile: "workflows/pilot.json", BriefFile: "brief.json", Inputs: map[string]string{}, StageResources: c.stages, ResourceLimits: c.limits})
		if refusalCode(err) != c.code {
			t.Fatalf("%v %v: refused as %v, want %s", c.stages, c.limits, err, c.code)
		}
	}
}

// A call's resource holds every attempt inside the workflow it calls: verify
// in aif-classic is a call, and the tests run in its step.
func TestACallsResourceHoldsTheAttemptsInsideIt(t *testing.T) {
	t.Parallel()
	r := Run{
		SchemaVersion: CoreResourceStateVersion, RootInvocationID: "root",
		StageResources: map[string][]string{"verify": {"heavy", "heavy"}}, ResourceLimits: map[string]int64{"heavy": 1},
		Invocations: map[string]*Invocation{
			"root":  {ID: "root"},
			"child": {ID: "child", ParentInvocationID: "root", CallerActivationID: "verify-call"},
			"deep":  {ID: "deep", ParentInvocationID: "child", CallerActivationID: "inner-call"},
		},
		Activations: map[string]*Activation{
			"verify-call": {ID: "verify-call", StageID: "verify", InvocationID: "root"},
			"inner-call":  {ID: "inner-call", StageID: "once", InvocationID: "child"},
			"gate":        {ID: "gate", StageID: "gate", InvocationID: "deep"},
			"plan":        {ID: "plan", StageID: "plan", InvocationID: "root"},
		},
	}
	if claims := r.resourcesFor("gate"); len(claims) != 1 || claims[0].Name != "heavy" || claims[0].Limit != 1 {
		t.Fatalf("an attempt two calls under verify does not hold its resource: %+v", claims)
	}
	if claims := r.resourcesFor("plan"); len(claims) != 0 {
		t.Fatalf("a stage naming no resource holds one: %+v", claims)
	}
	r.SchemaVersion = CoreQuestionStateVersion
	if claims := r.resourcesFor("gate"); len(claims) != 0 {
		t.Fatalf("a Run sealed before resources holds one: %+v", claims)
	}
}
