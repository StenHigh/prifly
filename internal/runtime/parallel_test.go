package runtime

import (
	"context"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

// parallelChild registers a control-only workflow that finishes with one exact
// outcome, so a branch result is decided by the definition rather than by a
// worker's timing.
func parallelChild(t *testing.T, e *Engine, base map[string]any, name, outcome string) flow.Ref {
	t.Helper()
	child := callClone(t, base)
	id := "test:workflow/branch-" + name
	child["id"], child["title"] = id, id
	child["allowed_outcomes"] = []string{outcome}
	if outcome == "succeeded" {
		// A branch that may rest on a waived check declares that its success
		// can be reported with the reduction still visible.
		child["allowed_outcomes"] = []string{outcome, "completed_with_waivers"}
	}
	child["outputs"] = map[string]any{}
	child["limits"] = map[string]any{"max_step_instances": 1, "max_control_transitions": 4, "max_parallelism": 1, "max_child_depth": 0}
	child["definition"] = map[string]any{"entry": "done", "stages": map[string]any{"done": choiceFinish(outcome)}}
	return callRegister(t, e, child, "workflows/branch-"+name+".json")
}

type branchSpec struct{ id, outcome string }

// parallelFixture builds a parent whose entry stage fans out to the declared
// branches. Each case bends exactly one property of the join, so a refusal or a
// verdict is attributable to that property alone.
func parallelFixture(t *testing.T, join map[string]any, branches ...branchSpec) (*Engine, map[string]any, StartOptions) {
	t.Helper()
	e, workflow, options := choiceFixture(t, `{"flag":true}`, "")
	inputs := workflow["inputs"].(map[string]any)
	declared := make([]any, 0, len(branches))
	for i, spec := range branches {
		ref := parallelChild(t, e, workflow, spec.id, spec.outcome)
		bindings := map[string]any{}
		for name := range inputs {
			bindings[name] = map[string]any{"from": "workflow_input", "port": name}
		}
		declared = append(declared, map[string]any{"id": spec.id, "workflow_ref": ref, "input_bindings": bindings})
		_ = i
	}
	defs, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	workflow["id"], workflow["title"] = "test:workflow/fanout", "Fan-out parent"
	workflow["policy_ref"] = builtinVersionRef(defs, "core:policy/local", "2.0.0")
	workflow["allowed_outcomes"] = []string{"succeeded", "rejected"}
	workflow["limits"] = map[string]any{"max_step_instances": 4, "max_control_transitions": 64, "max_parallelism": 1, "max_child_depth": 4}
	workflow["outputs"] = map[string]any{}
	workflow["definition"] = map[string]any{"entry": "fan", "stages": map[string]any{
		"fan": map[string]any{
			"kind": "parallel", "max_parallelism": 1, "branches": declared, "join": join,
			"on": map[string]any{"satisfied": "accepted", "unsatisfied": "refused"},
		},
		"accepted": choiceFinish("succeeded"),
		"refused":  choiceFinish("rejected"),
	}}
	options.WorkflowFile = "workflows/fanout.json"
	return e, workflow, options
}

func joinAll(outcomes ...string) map[string]any {
	accept := make([]any, 0, len(outcomes))
	for _, outcome := range outcomes {
		accept = append(accept, outcome)
	}
	return map[string]any{"mode": "all", "accept_outcomes": accept, "selection": "all", "remainder": "wait"}
}

func joinQuorum(k int, remainder string, outcomes ...string) map[string]any {
	accept := make([]any, 0, len(outcomes))
	for _, outcome := range outcomes {
		accept = append(accept, outcome)
	}
	return map[string]any{"mode": "quorum", "accept_outcomes": accept, "required_successes": k, "selection": "first_observed", "remainder": remainder}
}

func driveParallel(t *testing.T, e *Engine, runID string) Run {
	t.Helper()
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	return driverRun(t, e, runID)
}

func fanActivation(t *testing.T, r Run) *Activation {
	t.Helper()
	a := r.activationForInvocation(r.RootInvocationID, "fan")
	if a == nil || a.Parallel == nil {
		t.Fatal("the parallel stage has no activation with progress")
	}
	return a
}

// A fan-out is real work: each branch is an ordinary invocation with its own
// pinned workflow, inputs and settled result, not a lightweight marker.
func TestParallelEntersEveryBranchAsAnOrdinaryInvocation(t *testing.T) {
	e, workflow, options := parallelFixture(t, joinAll("succeeded"), branchSpec{"left", "succeeded"}, branchSpec{"right", "succeeded"})
	runID := choiceStart(t, e, workflow, options)
	before := driverRun(t, e, runID)
	if before.SchemaVersion != CoreParallelStateVersion {
		t.Fatalf("a fan-out did not pin its own state version: %s", before.SchemaVersion)
	}
	r := driveParallel(t, e, runID)
	if r.Status != "completed" || r.Outcome == nil || *r.Outcome != "succeeded" {
		t.Fatalf("a satisfied join did not route to its accepted finish: %s %+v", r.Status, r.Outcome)
	}
	a := fanActivation(t, r)
	if a.Parallel.EnteredCount != 2 || len(a.Parallel.BranchIDs) != 2 {
		t.Fatalf("the fan-out did not enter both declared branches: %+v", a.Parallel)
	}
	entered, err := r.parallelBranches(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	for i, inv := range entered {
		if inv.BranchID != a.Parallel.BranchIDs[i] || inv.Status != "completed" || inv.Settled == nil || inv.Outcome == nil {
			t.Fatalf("branch %d is not a settled invocation of its declared identity: %+v", i, inv)
		}
		if inv.WorkflowRef.Digest == "" || inv.ParentInvocationID != r.RootInvocationID {
			t.Fatalf("branch %d is not an ordinary child of the fan-out scope: %+v", i, inv)
		}
	}
	// Each branch has its own decision; the last one carries the verdict.
	d := a.Parallel.LastDecision
	if d == nil || d.Verdict != "satisfied" || d.Route != "satisfied" || d.ObservedCount != 2 || d.AcceptedCount != 2 {
		t.Fatalf("the join decision does not account for both branches: %+v", d)
	}
	if d.RemainderDisposition != "" {
		t.Fatalf("every branch was observed, so nothing was left out: %+v", d)
	}
}

// A join verdict is a statement about the declared contract, not about success.
// An unsatisfied join routes; it does not fail the Run.
func TestUnsatisfiedJoinRoutesInsteadOfFailing(t *testing.T) {
	e, workflow, options := parallelFixture(t, joinAll("succeeded"), branchSpec{"left", "succeeded"}, branchSpec{"right", "rejected"})
	runID := choiceStart(t, e, workflow, options)
	r := driveParallel(t, e, runID)
	if r.Status != "completed" || r.Outcome == nil || *r.Outcome != "rejected" {
		t.Fatalf("an unsatisfied join did not route to its declared stage: %s %+v", r.Status, r.Outcome)
	}
	d := fanActivation(t, r).Parallel.LastDecision
	if d == nil || d.Verdict != "unsatisfied" || d.Route != "unsatisfied" || d.AcceptedCount != 1 || d.ObservedCount != 2 {
		t.Fatalf("the decision did not record what it observed: %+v", d)
	}
	// remainder=wait means every branch is entered even once the verdict is known.
	if d.RemainderDisposition != "" {
		t.Fatalf("remainder=wait left a branch unentered: %+v", d)
	}
}

// remainder=cancel stops entering further branches once the quorum decides.
// Nothing is abandoned mid-flight, so the honest record is "never entered".
func TestQuorumWithCancelledRemainderNeverEntersTheRest(t *testing.T) {
	e, workflow, options := parallelFixture(t, joinQuorum(1, "cancel", "succeeded"), branchSpec{"left", "succeeded"}, branchSpec{"right", "succeeded"})
	runID := choiceStart(t, e, workflow, options)
	r := driveParallel(t, e, runID)
	if r.Outcome == nil || *r.Outcome != "succeeded" {
		t.Fatalf("a reached quorum did not route as satisfied: %+v", r.Outcome)
	}
	a := fanActivation(t, r)
	if a.Parallel.EnteredCount != 1 {
		t.Fatalf("a cancelled remainder still entered a branch: %+v", a.Parallel)
	}
	d := a.Parallel.LastDecision
	if d == nil || d.Verdict != "satisfied" || d.RemainderDisposition != "not_entered" || d.ObservedCount != 1 || d.BranchCount != 2 {
		t.Fatalf("the decision did not record the fate of the remaining branch: %+v", d)
	}
	if r.Invocations[branchInvocationID(r.ID, a.ID, "right")] != nil {
		t.Fatal("a branch that was never entered still produced an invocation")
	}
}

// remainder=wait keeps entering branches after the quorum is reached, and the
// later results do not overturn the verdict the counts already produced.
func TestQuorumWithWaitingRemainderStillEntersEveryBranch(t *testing.T) {
	e, workflow, options := parallelFixture(t, joinQuorum(1, "wait", "succeeded"), branchSpec{"left", "succeeded"}, branchSpec{"right", "rejected"})
	runID := choiceStart(t, e, workflow, options)
	r := driveParallel(t, e, runID)
	if r.Outcome == nil || *r.Outcome != "succeeded" {
		t.Fatalf("a later unaccepted branch overturned a reached quorum: %+v", r.Outcome)
	}
	a := fanActivation(t, r)
	if a.Parallel.EnteredCount != 2 {
		t.Fatalf("remainder=wait did not enter every branch: %+v", a.Parallel)
	}
	d := a.Parallel.LastDecision
	if d == nil || d.AcceptedCount != 1 || d.ObservedCount != 2 || d.Verdict != "satisfied" || d.RemainderDisposition != "" {
		t.Fatalf("the final decision did not account for every branch: %+v", d)
	}
}

// An unreachable quorum is decided as soon as the counts make it unreachable,
// rather than after pointlessly entering the rest.
func TestUnreachableQuorumIsDecidedFromTheCountsAlone(t *testing.T) {
	e, workflow, options := parallelFixture(t, joinQuorum(2, "cancel", "succeeded"), branchSpec{"a", "rejected"}, branchSpec{"b", "succeeded"}, branchSpec{"c", "succeeded"})
	runID := choiceStart(t, e, workflow, options)
	r := driveParallel(t, e, runID)
	a := fanActivation(t, r)
	// One rejection still leaves two branches, so the quorum stays reachable.
	if a.Parallel.EnteredCount != 3 {
		t.Fatalf("a reachable quorum stopped early: %+v", a.Parallel)
	}
	if r.Outcome == nil || *r.Outcome != "succeeded" {
		t.Fatalf("two accepted branches did not reach a quorum of two: %+v", r.Outcome)
	}
	if d := a.Parallel.LastDecision; d == nil || d.AcceptedCount != 2 || d.Verdict != "satisfied" {
		t.Fatalf("the decision counted the wrong branches: %+v", d)
	}
}

// joinVerdict is the whole rule; it must be a function of the counts so that a
// verdict reached early cannot drift as later branches settle.
func TestJoinVerdictDependsOnCountsOnly(t *testing.T) {
	all := flow.Join{Mode: "all", AcceptOutcomes: []string{"succeeded"}}
	quorum := flow.Join{Mode: "quorum", AcceptOutcomes: []string{"succeeded"}, RequiredSuccesses: 2}
	for _, c := range []struct {
		name                      string
		join                      flow.Join
		accepted, observed, total int64
		want                      string
	}{
		{"all pending", all, 1, 1, 2, "undecided"},
		{"all complete", all, 2, 2, 2, "satisfied"},
		{"all broken by one", all, 1, 2, 3, "unsatisfied"},
		{"quorum reached", quorum, 2, 2, 3, "satisfied"},
		{"quorum still reachable", quorum, 1, 2, 3, "undecided"},
		{"quorum unreachable", quorum, 0, 2, 3, "unsatisfied"},
		{"quorum reached on the last branch", quorum, 2, 3, 3, "satisfied"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := joinVerdict(c.join, c.accepted, c.observed, c.total); got != c.want {
				t.Fatalf("verdict %s, want %s", got, c.want)
			}
		})
	}
}

// A branch that failed produced no outcome at all. That is a technical failure
// of a child, not an unsatisfied join, and it must not be routed as one.
func TestFailedBranchIsNotAnUnsatisfiedJoin(t *testing.T) {
	e, workflow, options := parallelFixture(t, joinAll("succeeded"), branchSpec{"left", "succeeded"}, branchSpec{"right", "succeeded"})
	runID := choiceStart(t, e, workflow, options)
	r, view, err := e.load(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	p, err := r.planFor(r.RootInvocationID)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.enterParallel(context.Background(), r, view, p, r.activationForInvocation(r.RootInvocationID, "fan")); err != nil {
		t.Fatal(err)
	}
	r = driverRun(t, e, runID)
	a := fanActivation(t, r)
	branch := r.Invocations[a.Parallel.CurrentBranchInvocationID]
	branch.Status, branch.Outcome, branch.Ready = "failed", nil, []string{}
	settled := branch.Created
	branch.Settled = &settled
	// A settled child returns the frontier to its caller; take the same path
	// the driver takes rather than hand-placing the scope.
	if err := r.syncInvocationState(); err != nil {
		t.Fatal(err)
	}
	d, err := e.evaluateJoin(r, p, a)
	if err != nil {
		t.Fatal(err)
	}
	if d.Verdict != "undecided" || d.Failure != "branch_failed" || d.Route == "unsatisfied" || d.Route == "satisfied" {
		t.Fatalf("a failed branch was folded into a join verdict: %+v", d)
	}
	if d.Accepted {
		t.Fatalf("a branch without an outcome was counted as accepted: %+v", d)
	}
}

// A join verdict the definition never routed is a gap in the contract, not a
// technical error. on_error catches technical failures; it must not quietly
// become the route the author forgot to declare.
func TestUnroutedJoinVerdictIsNotCaughtByOnError(t *testing.T) {
	e, workflow, options := parallelFixture(t, joinAll("succeeded"), branchSpec{"left", "rejected"}, branchSpec{"right", "succeeded"})
	stages := workflow["definition"].(map[string]any)["stages"].(map[string]any)
	fan := stages["fan"].(map[string]any)
	fan["on"] = map[string]any{"satisfied": "accepted"}
	fan["on_error"] = "accepted"
	delete(stages, "refused")
	runID := choiceStart(t, e, workflow, options)
	r := driveParallel(t, e, runID)
	d := fanActivation(t, r).Parallel.LastDecision
	if d == nil {
		t.Fatal("the fan-out produced no decision")
	}
	if d.Route == "on_error" || d.NextStageID != "" {
		t.Fatalf("an unrouted verdict was caught by on_error: %+v", d)
	}
	if d.Route != "failed" || d.Failure == "" {
		t.Fatalf("an unrouted verdict did not fail with a named reason: %+v", d)
	}
	if r.Status != "failed" {
		t.Fatalf("a contract gap did not stop the Run: %s", r.Status)
	}
}

// checkJoinRoute re-derives the verdict at commit time, so a decision that
// claims a route its own counts do not produce is refused rather than trusted.
func TestJoinRouteIsRecheckedAgainstItsOwnCounts(t *testing.T) {
	e, workflow, options := parallelFixture(t, joinAll("succeeded"), branchSpec{"left", "succeeded"}, branchSpec{"right", "succeeded"})
	runID := choiceStart(t, e, workflow, options)
	r := driveParallel(t, e, runID)
	a := fanActivation(t, r)
	p, err := r.planFor(r.RootInvocationID)
	if err != nil {
		t.Fatal(err)
	}
	stage := p.Workflow.Definition.Stages["fan"]
	entered, err := r.parallelBranches(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	// The decision under test is the one the engine committed, so the progress
	// re-checked against it is the progress as it stood before that decision.
	progress := *a.Parallel
	progress.DecidedBranchIDs = progress.DecidedBranchIDs[:len(progress.DecidedBranchIDs)-1]
	sound := *a.Parallel.LastDecision
	if err := checkJoinRoute(stage, progress, entered, sound); err != nil {
		t.Fatalf("the decision the engine committed was refused on re-check: %v", err)
	}
	for _, bend := range []struct {
		name string
		fn   func(*JoinDecision)
	}{
		{"verdict its counts do not produce", func(d *JoinDecision) { d.Verdict, d.Route = "unsatisfied", "unsatisfied" }},
		{"count of accepted branches raised", func(d *JoinDecision) { d.AcceptedCount = 5 }},
		{"route to a stage the definition does not declare", func(d *JoinDecision) { d.NextStageID = "refused" }},
		{"join contract other than the pinned one", func(d *JoinDecision) { d.Mode = "quorum" }},
		{"remainder claimed without an unobserved branch", func(d *JoinDecision) { d.RemainderDisposition = "not_entered" }},
	} {
		t.Run(bend.name, func(t *testing.T) {
			bent := sound
			bend.fn(&bent)
			if err := checkJoinRoute(stage, progress, entered, bent); err == nil {
				t.Fatalf("a decision with a %s was accepted", bend.name)
			}
		})
	}
}

// The order is sealed at entry from the pinned plan. A branch invocation that
// does not belong to that sealed order is not silently adopted.
func TestSealedBranchOrderOwnsItsInvocations(t *testing.T) {
	e, workflow, options := parallelFixture(t, joinAll("succeeded"), branchSpec{"left", "succeeded"}, branchSpec{"right", "succeeded"})
	runID := choiceStart(t, e, workflow, options)
	r := driveParallel(t, e, runID)
	a := fanActivation(t, r)
	entered, err := r.parallelBranches(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	entered[0].BranchID = "renamed"
	if _, err := r.parallelBranches(a.ID); err == nil {
		t.Fatal("a renamed branch stayed a member of the sealed order")
	}
	entered[0].BranchID = a.Parallel.BranchIDs[0]
	a.Parallel.EnteredCount = 1
	if _, err := r.parallelBranches(a.ID); err == nil {
		t.Fatal("a lowered counter hid an existing branch invocation")
	}
}
