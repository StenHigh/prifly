package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

func canonicalOf(t *testing.T, value any) []byte {
	t.Helper()
	data, err := canonical(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// deepBranchChild builds a branch whose own definition uses choice and repeat
// over a leaf workflow. A branch is an ordinary workflow, so the operators
// inside it must work exactly as they do anywhere else.
func deepBranchChild(t *testing.T, e *Engine, base map[string]any, name string) flow.Ref {
	t.Helper()
	leaf := callClone(t, base)
	leaf["id"], leaf["title"] = "test:workflow/leaf-"+name, "leaf "+name
	leaf["allowed_outcomes"] = []string{"succeeded"}
	leaf["outputs"] = map[string]any{}
	leaf["limits"] = map[string]any{"max_step_instances": 1, "max_control_transitions": 4, "max_parallelism": 1, "max_child_depth": 0}
	leaf["definition"] = map[string]any{"entry": "done", "stages": map[string]any{"done": choiceFinish("succeeded")}}
	leafRef := callRegister(t, e, leaf, "workflows/leaf-"+name+".json")

	bindings := map[string]any{}
	for port := range base["inputs"].(map[string]any) {
		bindings[port] = map[string]any{"from": "workflow_input", "port": port}
	}
	child := callClone(t, base)
	child["id"], child["title"] = "test:workflow/deep-"+name, "deep "+name
	child["allowed_outcomes"] = []string{"succeeded"}
	child["outputs"] = map[string]any{}
	child["limits"] = map[string]any{"max_step_instances": 8, "max_control_transitions": 64, "max_parallelism": 1, "max_child_depth": 2}
	child["definition"] = map[string]any{"entry": "pick", "stages": map[string]any{
		"pick": choiceStage("exclusive", choiceBranch("yes", choiceFieldEqual("/flag", true), "loop"), choiceBranch("no", choiceFieldEqual("/flag", false), "done")),
		"loop": map[string]any{
			"kind": "repeat", "body_workflow_ref": leafRef, "initial_bindings": bindings,
			"next_bindings": callClone(t, bindings), "continue_on": []string{"succeeded"},
			"until": repeatLiteral(true), "max_iterations": int64(2),
			"on_complete": map[string]any{"succeeded": "done"}, "on_limit": "done",
		},
		"done": choiceFinish("succeeded"),
	}}
	return callRegister(t, e, child, "workflows/deep-"+name+".json")
}

// A branch is an ordinary child, so choice, repeat and their nested bodies keep
// working inside one. Their invocations belong to the branch, not to the
// fan-out's own scope.
func TestParallelComposesWithChoiceAndRepeatInsideABranch(t *testing.T) {
	e, workflow, options := parallelFixture(t, joinAll("succeeded"), branchSpec{"left", "succeeded"})
	deep := deepBranchChild(t, e, workflow, "left")
	stage := workflow["definition"].(map[string]any)["stages"].(map[string]any)["fan"].(map[string]any)
	branches := stage["branches"].([]any)
	branches[0].(map[string]any)["workflow_ref"] = deep
	workflow["limits"].(map[string]any)["max_child_depth"] = 4

	runID := choiceStart(t, e, workflow, options)
	r := driveParallel(t, e, runID)
	if r.Status != "completed" || r.Outcome == nil || *r.Outcome != "succeeded" {
		t.Fatalf("a branch containing choice and repeat did not complete: %s %+v", r.Status, r.Outcome)
	}
	a := fanActivation(t, r)
	entered, err := r.parallelBranches(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	branch := entered[0]
	if branch.WorkflowRef != deep {
		t.Fatalf("the branch did not run its declared workflow: %+v", branch.WorkflowRef)
	}
	// The repeat bodies inside the branch belong to the branch, not the parent.
	bodies := 0
	for _, inv := range r.Invocations {
		if inv.Iteration == nil {
			continue
		}
		bodies++
		if inv.ParentInvocationID != branch.ID {
			t.Fatalf("an iteration inside a branch was attributed elsewhere: %+v", inv)
		}
	}
	if bodies == 0 {
		t.Fatal("the repeat inside the branch produced no body invocation")
	}
	// The whole fan-out is charged to the root, not only the branch.
	root := r.Invocations[r.RootInvocationID]
	if root.ControlTransitions <= branch.ControlTransitions {
		t.Fatalf("the root budget did not cover the branch: root %d, branch %d", root.ControlTransitions, branch.ControlTransitions)
	}
}

// The state version is chosen from the whole compiled closure. A repeat that
// exists only inside a branch still requires the state that records it.
func TestParallelClosureSelectsStateForOperatorsInsideBranches(t *testing.T) {
	e, workflow, options := parallelFixture(t, joinAll("succeeded"), branchSpec{"left", "succeeded"})
	deep := deepBranchChild(t, e, workflow, "left")
	stage := workflow["definition"].(map[string]any)["stages"].(map[string]any)["fan"].(map[string]any)
	stage["branches"].([]any)[0].(map[string]any)["workflow_ref"] = deep
	workflow["limits"].(map[string]any)["max_child_depth"] = 4
	runID := choiceStart(t, e, workflow, options)
	r := driverRun(t, e, runID)
	if r.SchemaVersion != CoreParallelStateVersion {
		t.Fatalf("the closure did not select the fan-out state: %s", r.SchemaVersion)
	}
	// Every definition in the closure, branch bodies included, is pinned.
	for _, inv := range r.Invocations {
		if r.WorkflowConfigurations[inv.WorkflowRef.Digest] == nil {
			t.Fatalf("an invocation has no pinned configuration: %+v", inv.WorkflowRef)
		}
	}
	if len(r.WorkflowConfigurations) < 3 {
		t.Fatalf("the closure did not pin the branch and its nested bodies: %d", len(r.WorkflowConfigurations))
	}
}

// Branch identity is derived from the run, the activation and the declared
// branch, so re-entering the same branch cannot produce a second invocation.
func TestParallelBranchEntryIsIdempotentUnderRetry(t *testing.T) {
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
	a := r.activationForInvocation(r.RootInvocationID, "fan")
	if err := e.enterParallel(context.Background(), r, view, p, a); err != nil {
		t.Fatal(err)
	}
	before := driverRun(t, e, runID)
	// The same entry replayed against the stale view must not create a branch.
	if err := e.enterParallel(context.Background(), r, view, p, a); err == nil {
		t.Fatal("a replayed entry created a second branch")
	}
	after := driverRun(t, e, runID)
	if len(after.Invocations) != len(before.Invocations) {
		t.Fatalf("a refused entry still changed the invocation tree: %d then %d", len(before.Invocations), len(after.Invocations))
	}
	if fanActivation(t, after).Parallel.EnteredCount != 1 {
		t.Fatal("a refused entry advanced the branch counter")
	}
}

// Cancellation covers the whole active work set: a live branch is an ordinary
// invocation inside the fan-out's scope and is cancelled with it.
func TestCancellationReachesALiveBranch(t *testing.T) {
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
	live := fanActivation(t, driverRun(t, e, runID)).Parallel.CurrentBranchInvocationID
	if _, err := e.Restrict(context.Background(), RestrictCommand{SchemaVersion: "1", CommandID: newID("command"), Scope: "run", ScopeID: runID, Kind: "cancel", Reason: "operator stopped the fan-out"}); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	r = driverRun(t, e, runID)
	if r.Status != "cancelled" {
		t.Fatalf("the Run did not reach cancellation: %s", r.Status)
	}
	branch := r.Invocations[live]
	if branch == nil || branch.Status != "cancelled" || branch.Settled == nil {
		t.Fatalf("the live branch was left outside the cancelled work set: %+v", branch)
	}
	a := fanActivation(t, r)
	if a.Status != "cancelled" || a.Settled == nil {
		t.Fatalf("the fan-out activation was not settled by cancellation: %+v", a)
	}
	if a.Parallel.LastDecision != nil {
		t.Fatal("cancellation invented a join decision")
	}
}

// settleCurrentBranch settles the branch the fan-out is waiting on, taking the
// same path the driver takes when a child returns, so the fan-out is left
// exactly mid-flight rather than hand-placed there.
func settleCurrentBranch(t *testing.T, r Run, a *Activation, outcome string) {
	t.Helper()
	branch := r.Invocations[a.Parallel.CurrentBranchInvocationID]
	if branch == nil {
		t.Fatal("the fan-out has no current branch")
	}
	settled := branch.Created
	branch.Status, branch.Ready, branch.Outcome, branch.Settled = "completed", []string{}, &outcome, &settled
	if err := r.syncInvocationState(); err != nil {
		t.Fatal(err)
	}
}

// joinHistory returns the durable decisions in order, read from the journal
// rather than from the projection they were folded into.
func joinHistory(t *testing.T, e *Engine, runID string) ([]JoinDecision, []string) {
	t.Helper()
	view, _ := choiceHistory(t, e, runID)
	decisions := []JoinDecision{}
	branches := []string{}
	for _, event := range view.Events {
		switch event.Type {
		case "stage.join_decided":
			var d JoinDecision
			if err := json.Unmarshal(event.Data, &d); err != nil {
				t.Fatal(err)
			}
			decisions = append(decisions, d)
		case "invocation.created":
			var created struct {
				BranchID string `json:"branch_id"`
			}
			if err := json.Unmarshal(event.Data, &created); err != nil {
				t.Fatal(err)
			}
			if created.BranchID != "" {
				branches = append(branches, created.BranchID)
			}
		}
	}
	return decisions, branches
}

// The journal is the record of what happened. Each branch is created once and
// each settled branch is decided once, in order; nothing is repeated and the
// projection agrees with the journal it was folded from.
func TestJoinRecordsEachBranchAndDecisionExactlyOnce(t *testing.T) {
	e, workflow, options := parallelFixture(t, joinAll("succeeded"), branchSpec{"left", "succeeded"}, branchSpec{"right", "succeeded"})
	runID := choiceStart(t, e, workflow, options)
	r := driveParallel(t, e, runID)
	decisions, branches := joinHistory(t, e, runID)
	if len(branches) != 2 || branches[0] != "left" || branches[1] != "right" {
		t.Fatalf("branches were not created once each in declared order: %v", branches)
	}
	if len(decisions) != 2 {
		t.Fatalf("expected one decision per settled branch, got %d", len(decisions))
	}
	if decisions[0].Route != "next_branch" || decisions[0].Position != 1 || decisions[0].NextBranchID != "right" {
		t.Fatalf("the first decision did not continue to the second branch: %+v", decisions[0])
	}
	if decisions[1].Route != "satisfied" || decisions[1].Position != 2 || decisions[1].NextBranchID != "" {
		t.Fatalf("the second decision is not the terminal one: %+v", decisions[1])
	}
	// Only the latest decision is projected; the earlier one stays in the journal.
	last := fanActivation(t, r).Parallel.LastDecision
	if last == nil || !bytes.Equal(canonicalOf(t, *last), canonicalOf(t, decisions[1])) {
		t.Fatalf("the projection disagrees with the journal: %+v", last)
	}
}

// A decision is evidence about a settled branch. While a branch is still live
// no decision may be taken, and the refusal changes nothing.
func TestJoinRefusesToDecideALiveBranch(t *testing.T) {
	e, workflow, options := parallelFixture(t, joinAll("succeeded"), branchSpec{"left", "succeeded"}, branchSpec{"right", "succeeded"})
	runID := choiceStart(t, e, workflow, options)
	ctx := context.Background()
	r, view, err := e.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	p, err := r.planFor(r.RootInvocationID)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.enterParallel(ctx, r, view, p, r.activationForInvocation(r.RootInvocationID, "fan")); err != nil {
		t.Fatal(err)
	}
	live, liveView, err := e.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	before := len(live.Invocations)
	if err := e.decideJoin(ctx, live, liveView, p, fanActivation(t, live)); err == nil {
		t.Fatal("a join was decided while its branch was still live")
	}
	after := driverRun(t, e, runID)
	if len(after.Invocations) != before {
		t.Fatalf("a refused decision changed the invocation tree: %d then %d", before, len(after.Invocations))
	}
	if a := fanActivation(t, after); a.Parallel.LastDecision != nil || a.Parallel.EnteredCount != 1 {
		t.Fatalf("a refused decision advanced the fan-out: %+v", a.Parallel)
	}
	// The fan-out still finishes normally from exactly this point.
	final := driveParallel(t, e, runID)
	if final.Outcome == nil || *final.Outcome != "succeeded" {
		t.Fatalf("the fan-out did not finish after the refusal: %s %+v", final.Status, final.Outcome)
	}
}

// A reached join is not permission to finish while an effect is unresolved.
// The fan-out cannot settle its verdict until the uncertainty is settled.
func TestJoinDoesNotSettleOverAnUnresolvedEffect(t *testing.T) {
	e, workflow, options := parallelFixture(t, joinQuorum(1, "cancel", "succeeded"), branchSpec{"left", "succeeded"}, branchSpec{"right", "succeeded"})
	runID := choiceStart(t, e, workflow, options)
	ctx := context.Background()
	r, view, err := e.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	p, err := r.planFor(r.RootInvocationID)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.enterParallel(ctx, r, view, p, r.activationForInvocation(r.RootInvocationID, "fan")); err != nil {
		t.Fatal(err)
	}
	r = driverRun(t, e, runID)
	a := fanActivation(t, r)
	branch := r.Invocations[a.Parallel.CurrentBranchInvocationID]
	branch.Status, branch.Ready = "completed", []string{}
	outcome := "succeeded"
	settled := branch.Created
	branch.Outcome, branch.Settled = &outcome, &settled
	r.HasUnresolvedEffects = true
	if err := r.syncInvocationState(); err != nil {
		t.Fatal(err)
	}
	if _, err := e.evaluateJoin(r, p, r.Activations[a.ID]); err == nil {
		t.Fatal("a join was decided while an effect was unresolved")
	}
	if kind, _ := nextKind(r); kind != "uncertain" {
		t.Fatalf("an unresolved effect did not hold the Run: %s", kind)
	}
}

// Parent and branch intervals are different quantities. The fan-out's own
// elapsed time covers its branches; it is not the sum of their leaf work.
func TestFanOutTimingSeparatesParentFromBranches(t *testing.T) {
	e, workflow, options := parallelFixture(t, joinAll("succeeded"), branchSpec{"left", "succeeded"}, branchSpec{"right", "succeeded"})
	runID := choiceStart(t, e, workflow, options)
	r := driveParallel(t, e, runID)
	a := fanActivation(t, r)
	report := Timing(r, r.LastObserved, false)
	node := timingFind(t, report.Root, a.ID)
	if node.StageKind != "parallel" {
		t.Fatalf("the fan-out node did not carry its stage kind: %+v", node.StageKind)
	}
	// A control stage runs no executor of its own, whatever its branches did.
	if duration := node.Metrics["executor_time"]; duration.Quality != "not_applicable" || duration.ValueMS != nil {
		t.Fatalf("a control stage reported executor time: %+v", duration)
	}
	entered, err := r.parallelBranches(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, inv := range entered {
		branch := timingFind(t, report.Root, inv.ID)
		if branch.ID != inv.ID {
			t.Fatalf("branch %s is missing from the timing tree", inv.ID)
		}
	}
}

// waiverAwareFixture declares branches that may report their success with the
// reduction still visible. The reduction is derived from an applied waiver, so
// no finish stage authors it; the workflow only declares that it can occur.
func waiverAwareFixture(t *testing.T, accept ...string) (*Engine, string, *flow.Plan, *Activation) {
	t.Helper()
	e, workflow, options := parallelFixture(t, joinAll(accept...), branchSpec{"left", "succeeded"}, branchSpec{"right", "succeeded"})
	runID := choiceStart(t, e, workflow, options)
	ctx := context.Background()
	r, view, err := e.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	p, err := r.planFor(r.RootInvocationID)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.enterParallel(ctx, r, view, p, r.activationForInvocation(r.RootInvocationID, "fan")); err != nil {
		t.Fatal(err)
	}
	return e, runID, p, nil
}

// WF-AC-11: a join counts a reduced result only because the contract names it.
// Resemblance to success is not acceptance, and the branch keeps its own
// recorded outcome either way.
func TestJoinAcceptsAReducedOutcomeOnlyWhenDeclared(t *testing.T) {
	for _, c := range []struct {
		name     string
		accept   []string
		accepted bool
	}{
		{"reduction not named by the join", []string{"succeeded"}, false},
		{"reduction named by the join", []string{"succeeded", "completed_with_waivers"}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			e, runID, p, _ := waiverAwareFixture(t, c.accept...)
			r := driverRun(t, e, runID)
			a := fanActivation(t, r)
			settleCurrentBranch(t, r, a, "completed_with_waivers")
			d, err := e.evaluateJoin(r, p, r.Activations[a.ID])
			if err != nil {
				t.Fatal(err)
			}
			if d.Accepted != c.accepted {
				t.Fatalf("acceptance did not follow the declared list: %+v", d)
			}
			if d.BranchOutcome == nil || *d.BranchOutcome != "completed_with_waivers" {
				t.Fatalf("the branch's reduced outcome was rewritten: %+v", d.BranchOutcome)
			}
			if !c.accepted && d.Verdict != "unsatisfied" {
				t.Fatalf("an unnamed reduction still satisfied the join: %+v", d)
			}
		})
	}
}

// The budget is one shared quantity, not a fresh allowance per scope. Work
// inside a branch, and inside a loop inside that branch, is spent from the
// root's budget too, so a generous local limit cannot buy more than the Run has.
func TestNestedWorkSpendsTheSharedBudgetOnce(t *testing.T) {
	build := func(t *testing.T, rootTransitions int64) (*Engine, string) {
		t.Helper()
		e, workflow, options := parallelFixture(t, joinAll("succeeded"), branchSpec{"left", "succeeded"})
		deep := deepBranchChild(t, e, workflow, "left")
		stage := workflow["definition"].(map[string]any)["stages"].(map[string]any)["fan"].(map[string]any)
		stage["branches"].([]any)[0].(map[string]any)["workflow_ref"] = deep
		workflow["limits"] = map[string]any{"max_step_instances": 8, "max_control_transitions": rootTransitions, "max_parallelism": 1, "max_child_depth": 4}
		return e, choiceStart(t, e, workflow, options)
	}

	e, runID := build(t, 64)
	r := driveParallel(t, e, runID)
	if r.Status != "completed" {
		t.Fatalf("the fan-out did not complete under a generous budget: %s", r.Status)
	}
	root := r.Invocations[r.RootInvocationID]
	spent := root.ControlTransitions
	if spent != r.ControlTransitions {
		t.Fatalf("the Run and its root disagree on what was spent: %d and %d", r.ControlTransitions, spent)
	}
	// Every scope spent from the same quantity: no descendant spent more than
	// the root, which is what "shared" means here.
	for _, inv := range r.Invocations {
		if inv.ControlTransitions > spent {
			t.Fatalf("a nested scope spent more than the whole Run: %+v", inv)
		}
	}
	deepest := int64(0)
	for _, inv := range r.Invocations {
		if inv.Iteration != nil && inv.ControlTransitions > deepest {
			deepest = inv.ControlTransitions
		}
	}
	if deepest == 0 {
		t.Fatal("the loop inside the branch spent nothing")
	}

	// The same workflow under a root budget one short of what it costs never
	// starts: the shared budget must cover every permitted nested path, and a
	// generous limit on a nested definition cannot buy what the Run lacks.
	// Refusing here is stronger than refusing midway, because no work begins.
	short, workflow, options := parallelFixture(t, joinAll("succeeded"), branchSpec{"left", "succeeded"})
	deep := deepBranchChild(t, short, workflow, "left")
	stage := workflow["definition"].(map[string]any)["stages"].(map[string]any)["fan"].(map[string]any)
	stage["branches"].([]any)[0].(map[string]any)["workflow_ref"] = deep
	workflow["limits"] = map[string]any{"max_step_instances": 8, "max_control_transitions": spent - 1, "max_parallelism": 1, "max_child_depth": 4}
	writeRuntimeJSON(t, filepath.Join(short.Root, options.WorkflowFile), workflow)
	writeRuntimeJSON(t, filepath.Join(short.Root, "prifly.json"), short.Config)
	_, err := short.Start(context.Background(), options)
	if err == nil {
		t.Fatal("nested scopes bought more work than the Run's budget allowed")
	}
	var problem *flow.Problem
	if !errors.As(err, &problem) || problem.Code != "limit_exceeded" {
		t.Fatalf("the refusal did not name the shared budget: %v", err)
	}
}

// The point of the summary is that the next stage can read what the branches
// did. It reports the entered branches with their own outcomes, and it is
// sealed evidence like any other artifact, not a value assembled at read time.
func TestFanOutSummaryIsReadableByTheNextStage(t *testing.T) {
	e, workflow, options := parallelFixture(t, joinAll("succeeded"), branchSpec{"left", "succeeded"}, branchSpec{"right", "succeeded"})
	defs, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	schema := builtinRef(defs, flow.AggregateSchemaID)
	// The workflow declares an output of the shipped summary form and binds it
	// to what the fan-out produced.
	workflow["outputs"] = map[string]any{"report": map[string]any{"format": "json", "schema_ref": schema, "required_for": []string{"succeeded"}}}
	stages := workflow["definition"].(map[string]any)["stages"].(map[string]any)
	stages["accepted"].(map[string]any)["output_bindings"] = map[string]any{"report": map[string]any{"from": "stage_output", "stage_id": "fan", "port": flow.AggregateResultsPort}}
	runID := choiceStart(t, e, workflow, options)
	r := driveParallel(t, e, runID)
	if r.Status != "completed" || r.Outcome == nil || *r.Outcome != "succeeded" {
		t.Fatalf("the fan-out did not complete: %s %+v", r.Status, r.Outcome)
	}
	ref, exported := r.Outputs["report"]
	if !exported {
		t.Fatalf("the summary was not exported: %+v", r.Outputs)
	}
	a := fanActivation(t, r)
	if a.Parallel.ResultsRef == nil || *a.Parallel.ResultsRef != ref {
		t.Fatalf("the exported summary is not the one the join sealed: %+v", a.Parallel.ResultsRef)
	}
	_, data, err := e.Artifact(ref)
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		SchemaVersion     string   `json:"schema_version"`
		StageActivationID string   `json:"stage_activation_id"`
		JoinResult        string   `json:"join_result"`
		SelectedBranchIDs []string `json:"selected_branch_ids"`
		Branches          []struct {
			ID           string  `json:"id"`
			RunID        string  `json:"run_id"`
			InvocationID string  `json:"workflow_invocation_id"`
			Status       string  `json:"status"`
			Outcome      *string `json:"outcome"`
			HasWaivers   bool    `json:"has_waivers"`
		} `json:"branches"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != "1" || manifest.StageActivationID != a.ID || manifest.JoinResult != "satisfied" {
		t.Fatalf("the summary does not describe this join: %+v", manifest)
	}
	if len(manifest.Branches) != 2 {
		t.Fatalf("the summary does not report both branches: %+v", manifest.Branches)
	}
	for i, branch := range manifest.Branches {
		if branch.ID != a.Parallel.BranchIDs[i] || branch.RunID != r.ID || branch.Status != "completed" || branch.Outcome == nil || branch.HasWaivers {
			t.Fatalf("branch %d is not reported as it settled: %+v", i, branch)
		}
		if r.Invocations[branch.InvocationID] == nil {
			t.Fatalf("branch %d names an invocation this Run does not have: %+v", i, branch)
		}
	}
	if len(manifest.SelectedBranchIDs) != 2 {
		t.Fatalf("a satisfied join of two accepted branches selected %d", len(manifest.SelectedBranchIDs))
	}
}
