package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

// guardFixture builds a control-only workflow whose first stage calls a child
// that republishes one fact, and whose second stage is the one a guard may
// hold. That shape exists because a guard in this build reads only facts the
// Run already holds: the single way such a fact can appear part-way through a
// Run is for an earlier stage to produce it.
func guardFixture(t *testing.T, controlJSON string) (*Engine, map[string]any, StartOptions) {
	t.Helper()
	e, workflow, options := choiceFixture(t, controlJSON, "")
	defs, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	child := callClone(t, workflow)
	child["id"], child["title"] = "test:workflow/guard-child", "Guard child"
	child["policy_ref"] = builtinVersionRef(defs, "core:policy/local", "2.0.0")
	child["allowed_outcomes"] = []string{"succeeded"}
	child["limits"] = map[string]any{"max_step_instances": 1, "max_control_transitions": 8, "max_parallelism": 1, "max_child_depth": 0}
	port := child["inputs"].(map[string]any)["control"].(map[string]any)
	child["outputs"] = map[string]any{"verdict": map[string]any{"format": port["format"], "schema_ref": port["schema_ref"], "required_for": []string{"succeeded"}}}
	finish := choiceFinish("succeeded")
	finish["output_bindings"] = map[string]any{"verdict": map[string]any{"from": "workflow_input", "port": "control"}}
	child["definition"] = map[string]any{"entry": "done", "stages": map[string]any{"done": finish}}
	ref := callRegister(t, e, child, "workflows/guard-child.json")

	workflow["id"], workflow["title"] = "test:workflow/guard-parent", "Guard parent"
	workflow["policy_ref"] = builtinVersionRef(defs, "core:policy/local", "2.0.0")
	workflow["allowed_outcomes"] = []string{"succeeded"}
	workflow["outputs"] = map[string]any{}
	workflow["limits"] = map[string]any{"max_step_instances": 4, "max_control_transitions": 64, "max_parallelism": 1, "max_child_depth": 4}
	workflow["definition"] = map[string]any{"entry": "probe", "stages": map[string]any{
		"probe": callStage(ref, workflow["inputs"].(map[string]any), "succeeded", "gate"),
		"gate":  choiceStage("exclusive", choiceBranch("always", choiceFieldEqual("/flag", true), "done"), choiceBranch("never", choiceFieldEqual("/flag", false), "done")),
		"done":  choiceFinish("succeeded"),
	}}
	options.WorkflowFile = "workflows/guard-parent.json"
	return e, workflow, options
}

// guardPredicate compares one pointer inside a named source against a literal.
func guardPredicate(ref flow.FieldRef, value any) flow.Predicate {
	literal, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return flow.Predicate{Op: "eq",
		Left:  &flow.Operand{Kind: "field", Ref: &ref},
		Right: &flow.Operand{Kind: "literal", Value: literal}}
}

func guardVerdict(pointer string) flow.FieldRef {
	return flow.FieldRef{From: "stage_output", StageID: "probe", Port: "verdict", Pointer: pointer}
}

func guardControl(pointer string) flow.FieldRef {
	return flow.FieldRef{From: "workflow_input", Port: "control", Pointer: pointer}
}

func guardOnly(t *testing.T, r Run) *GuardRegistration {
	t.Helper()
	if len(r.Guards) != 1 {
		t.Fatalf("the run holds %d guards, not one", len(r.Guards))
	}
	for _, g := range r.Guards {
		return g
	}
	return nil
}

func guardTruths(g *GuardRegistration) []string {
	truths := make([]string, 0, len(g.Observations))
	for _, observation := range g.Observations {
		truths = append(truths, observation.Truth)
	}
	return truths
}

// The fact the guard reads does not exist when the Run starts and exists once
// the first stage has produced it. The scope waits in between, with the reason
// written down, and the activation it was holding is opened exactly once.
func TestStartGuardWaitsForAFactAndThenOpensOneActivation(t *testing.T) {
	e, workflow, options := guardFixture(t, `{"flag":true}`)
	options.Guards = []GuardDeclaration{{Kind: "start", TargetStageID: "gate", Predicate: guardPredicate(guardVerdict("/flag"), true)}}
	runID := choiceStart(t, e, workflow, options)
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	if r.SchemaVersion != CoreGuardStateVersion {
		t.Fatalf("a declared guard did not select the guard state: %s", r.SchemaVersion)
	}
	if r.Status != "completed" {
		t.Fatalf("the guarded run did not finish once its fact was true: %s", r.Status)
	}
	g := guardOnly(t, r)
	truths := guardTruths(g)
	if len(truths) < 2 || truths[0] != string(flow.TruthUnknown) || truths[len(truths)-1] != string(flow.TruthTrue) {
		t.Fatalf("the guard did not record waiting and then admitting: %v", truths)
	}
	if g.Observations[0].Reason != "facts_absent" || len(g.Observations[0].Facts) != 1 || g.Observations[0].Facts[0].Availability != "absent" {
		t.Fatalf("the wait recorded no reason of its own: %+v", g.Observations[0])
	}
	// The evidence names the exact artifact revision the true was read from,
	// so the decision can be explained afterwards instead of re-derived.
	last := g.Observations[len(g.Observations)-1]
	if last.Facts[0].SourceRef == nil || last.Facts[0].Availability != "present" || last.Cut == 0 {
		t.Fatalf("the admitting observation kept no fact reference or cut: %+v", last)
	}
	opened := 0
	for _, a := range r.Activations {
		if a.StageID == "gate" {
			opened++
		}
	}
	if opened != 1 {
		t.Fatalf("the guarded stage opened %d activations", opened)
	}
	if g.Status != "satisfied" {
		t.Fatalf("a start guard whose activation exists is still consulted: %s", g.Status)
	}
}

// A false start guard holds its stage without an activation, without an
// attempt and without a worker slot, and says which fact it read.
func TestStartGuardHoldsItsScopeWithoutHoldingAWorker(t *testing.T) {
	e, workflow, options := guardFixture(t, `{"flag":false}`)
	options.Guards = []GuardDeclaration{{Kind: "start", TargetStageID: "gate", Predicate: guardPredicate(guardVerdict("/flag"), true)}}
	runID := choiceStart(t, e, workflow, options)
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	if r.terminal() {
		t.Fatalf("a held stage finished the run: %s", r.Status)
	}
	if len(r.Attempts) != 0 || len(r.Active) != 0 {
		t.Fatalf("a held stage admitted work: %d attempts, %d active", len(r.Attempts), len(r.Active))
	}
	if r.activationForInvocation(r.RootInvocationID, "gate") != nil {
		t.Fatal("a false start guard opened its activation anyway")
	}
	g := guardOnly(t, r)
	if settled := g.settled(); settled == nil || settled.Truth != string(flow.TruthFalse) || settled.Reason != "facts_false" {
		t.Fatalf("the hold recorded no false: %+v", settled)
	}
	next, err := e.Next(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if next.Action != "guarded" || next.InvocationID != r.RootInvocationID {
		t.Fatalf("the held scope is not reported as guarded: %s %s", next.Action, next.InvocationID)
	}
	if guard, reason := r.guardBlock(r.RootInvocationID, "gate"); guard != g.ID || reason != "start_condition_false" {
		t.Fatalf("the refusal names no guard and no reason: %s %s", guard, reason)
	}
}

// A start guard is a gate on work that has not begun. Once the activation
// exists the work has started, and the guard going false does not take it
// back: withdrawing started work is what a stop guard is for.
func TestStartGuardFalseAfterOpeningDoesNotWithdrawTheWork(t *testing.T) {
	g := &GuardRegistration{SchemaVersion: GuardRegistrationVersion, ID: "guard:one", RunID: "run:one", Kind: "start",
		InvocationID: "invocation:root", TargetStageID: "gate", Actor: "actor", Status: "observing", Cursor: 1,
		Observations: []GuardObservation{{Sequence: 0, Truth: string(flow.TruthFalse), Reason: "facts_false", Processed: true}}}
	r := Run{SchemaVersion: CoreGuardStateVersion, ID: "run:one", Activations: map[string]*Activation{},
		Guards: map[string]*GuardRegistration{g.ID: g}}
	if guard, _ := r.guardBlock("invocation:root", "gate"); guard != g.ID {
		t.Fatal("a false start guard did not hold an unopened stage")
	}
	r.Activations["activation:gate"] = &Activation{ID: "activation:gate", StageID: "gate", InvocationID: "invocation:root", Kind: "choice", Status: "ready"}
	if guard, reason := r.guardBlock("invocation:root", "gate"); guard != "" {
		t.Fatalf("a false start guard withdrew work that had already started: %s %s", guard, reason)
	}
}

// Firing creates the ordinary durable stop, scoped to the invocation and
// carrying the control epoch, not a second stop mechanism of its own.
func TestStopGuardFiresAnOrdinaryScopedRestrict(t *testing.T) {
	e, workflow, options := guardFixture(t, `{"flag":true}`)
	options.Guards = []GuardDeclaration{{Kind: "stop", Predicate: guardPredicate(guardControl("/flag"), true), Action: "pause_scope", OnUnknown: "cancel_scope"}}
	runID := choiceStart(t, e, workflow, options)
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	// A stop guard already true when the scope was registered prevents the
	// start: nothing was activated before it was asked.
	if len(r.Activations) != 0 {
		t.Fatalf("a stop guard true at registration let %d activations open", len(r.Activations))
	}
	g := guardOnly(t, r)
	if g.Status != "fired" || !g.Latched || g.StopID == "" {
		t.Fatalf("the stop guard did not fire: %+v", g)
	}
	if len(r.Stops) != 1 {
		t.Fatalf("firing created %d stops", len(r.Stops))
	}
	stop := r.Stops[0]
	if stop.ID != g.StopID || stop.Kind != "pause" || stop.Status != "active" || stop.Scope != "invocation" || stop.ScopeID != r.RootInvocationID {
		t.Fatalf("the stop is not an ordinary scoped restriction: %+v", stop)
	}
	if stop.Epoch != r.ControlEpoch || stop.Epoch == 0 || stop.Actor != e.owner {
		t.Fatalf("the stop carries no control epoch or actor: %+v", stop)
	}
	if !r.restrictedFor(r.RootInvocationID) {
		t.Fatal("the fired stop does not restrict its own scope")
	}
}

// on_unknown is mandatory and is itself a restriction. A fact this Run does
// not hold produces the declared reaction, never a permissive default.
func TestStopGuardActsOnUnknownRatherThanFailingOpen(t *testing.T) {
	e, workflow, options := guardFixture(t, `{"flag":true}`)
	options.Guards = []GuardDeclaration{{Kind: "stop", Predicate: guardPredicate(guardControl("/absent"), true), Action: "pause_scope", OnUnknown: "cancel_scope"}}
	runID := choiceStart(t, e, workflow, options)
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	g := guardOnly(t, r)
	if settled := g.settled(); settled == nil || settled.Truth != string(flow.TruthUnknown) || settled.Reason != "facts_absent" {
		t.Fatalf("a missing fact did not read as unknown: %+v", settled)
	}
	if len(r.Stops) != 1 || r.Stops[0].Kind != "cancel" || !r.Invocations[r.RootInvocationID].CancelRequested {
		t.Fatalf("on_unknown did not take its declared action: %+v", r.Stops)
	}
	// A stop guard is never lifted by its own facts recovering. Even if the
	// condition later read false, the stop record stays until it is released,
	// and cancel is not releasable at all.
	if !r.restrictedFor(r.RootInvocationID) {
		t.Fatal("the unknown reaction restricted nothing")
	}
}

// A declaration that omits either reaction is refused. There is no default
// that could be chosen here without choosing it on the author's behalf.
func TestStopGuardWithoutADeclaredReactionIsRefused(t *testing.T) {
	for _, declaration := range []GuardDeclaration{
		{Kind: "stop", Predicate: guardPredicate(guardControl("/flag"), true), Action: "pause_scope"},
		{Kind: "stop", Predicate: guardPredicate(guardControl("/flag"), true), OnUnknown: "pause_scope"},
		{Kind: "stop", Predicate: guardPredicate(guardControl("/flag"), true), Action: "carry_on", OnUnknown: "pause_scope"},
		{Kind: "start", Predicate: guardPredicate(guardControl("/flag"), true), OnUnknown: "pause_scope"},
	} {
		e, workflow, options := guardFixture(t, `{"flag":true}`)
		options.Guards = []GuardDeclaration{declaration}
		writeRuntimeJSON(t, e.Root+"/workflows/guard-parent.json", workflow)
		writeRuntimeJSON(t, e.Root+"/prifly.json", e.Config)
		if _, err := e.Start(context.Background(), options); err == nil {
			t.Fatalf("a guard with no declared reaction was accepted: %+v", declaration)
		}
	}
}

// REA-008: a true observed while the cursor is behind it must survive a false
// that arrives after it. All three observations are read, not just the last.
func TestGuardLatchKeepsATrueThatALaterFalseFollowed(t *testing.T) {
	g := &GuardRegistration{SchemaVersion: GuardRegistrationVersion, ID: "guard:one", RunID: "run:one", Kind: "stop",
		InvocationID: "invocation:root", Action: "pause_scope", OnUnknown: "pause_scope", Actor: "actor", Status: "observing",
		Observations: []GuardObservation{
			{Sequence: 0, Truth: string(flow.TruthFalse), Reason: "facts_false"},
			{Sequence: 1, Truth: string(flow.TruthTrue), Reason: "facts_true", Cut: 7},
			{Sequence: 2, Truth: string(flow.TruthFalse), Reason: "facts_false"},
		}}
	r := &Run{SchemaVersion: CoreGuardStateVersion, ID: "run:one", RootInvocationID: "invocation:root",
		Invocations: map[string]*Invocation{"invocation:root": {ID: "invocation:root", RunID: "run:one", Status: "ready"}},
		Activations: map[string]*Activation{}, Guards: map[string]*GuardRegistration{g.ID: g}, Stops: []Stop{}}
	// While the observations are unprocessed, nothing ordinary is admitted:
	// one of them may be the observation that stops the scope.
	if guard, reason := r.guardBlock("invocation:root", "gate"); guard != g.ID || reason != "guard_observations_pending" {
		t.Fatalf("unprocessed observations did not refuse admission: %s %s", guard, reason)
	}
	if _, err := processGuard(r, g, Observation{}); err != nil {
		t.Fatal(err)
	}
	if !g.Latched || g.StopID == "" || g.Status != "fired" || g.Cursor != 3 {
		t.Fatalf("the true between two falses was lost: %+v", g)
	}
	if len(r.Stops) != 1 || r.Stops[0].Kind != "pause" || r.Stops[0].ScopeID != "invocation:root" {
		t.Fatalf("the latch created no ordinary stop: %+v", r.Stops)
	}
	if err := r.guardInvariant(); err != nil {
		t.Fatal(err)
	}
	// The stop names the observation it rests on, so the cause can be read
	// back rather than recomputed against facts that have since moved.
	if want := "observation 1"; !strings.Contains(r.Stops[0].Reason, want) {
		t.Fatalf("the stop does not name its cause: %s", r.Stops[0].Reason)
	}
}

// Recovered facts do not lift a stop, and neither does releasing it while the
// condition still reads true: release lifts the record, not the condition.
func TestFiredStopGuardKeepsRefusingWhileItsConditionHolds(t *testing.T) {
	g := &GuardRegistration{SchemaVersion: GuardRegistrationVersion, ID: "guard:one", RunID: "run:one", Kind: "stop",
		InvocationID: "invocation:root", Action: "pause_scope", OnUnknown: "pause_scope", Actor: "actor",
		Status: "fired", Latched: true, StopID: "stop:one", Cursor: 1,
		Observations: []GuardObservation{{Sequence: 0, Truth: string(flow.TruthTrue), Reason: "facts_true", Processed: true}}}
	r := Run{SchemaVersion: CoreGuardStateVersion, ID: "run:one", Activations: map[string]*Activation{},
		Guards: map[string]*GuardRegistration{g.ID: g},
		// The stop itself is already released, which is the ordinary explicit
		// action. The guard still refuses, because the facts still say so.
		Stops: []Stop{{ID: "stop:one", Kind: "pause", Status: "released"}}}
	if guard, reason := r.guardBlock("invocation:root", "gate"); guard != g.ID || reason != "stop_condition_true" {
		t.Fatalf("a released stop reopened a scope its condition still refuses: %s %s", guard, reason)
	}
	g.Observations = append(g.Observations, GuardObservation{Sequence: 1, Truth: string(flow.TruthFalse), Reason: "facts_false", Processed: true})
	g.Cursor = 2
	if guard, _ := r.guardBlock("invocation:root", "gate"); guard != "" {
		t.Fatal("the guard kept refusing after both the release and a false")
	}
}

// A type error is an error. It is never downgraded to unknown, because unknown
// has a declared reaction and lending it to a fault would let a broken guard
// take whichever of the two happened to be the permissive one.
func TestGuardTypeMismatchIsAnErrorAndKeepsRefusing(t *testing.T) {
	e, workflow, options := guardFixture(t, `{"flag":"yes"}`)
	options.Guards = []GuardDeclaration{{Kind: "start", TargetStageID: "gate", Predicate: guardPredicate(guardControl("/flag"), true)}}
	runID := choiceStart(t, e, workflow, options)
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	g := guardOnly(t, r)
	if len(g.Observations) == 0 || g.Observations[0].Truth != "error" || g.Observations[0].Reason != "condition_type_mismatch" {
		t.Fatalf("a comparison between different types did not record an error: %+v", g.Observations)
	}
	if g.Status != "failed" {
		t.Fatalf("a guard that cannot be evaluated did not fail: %s", g.Status)
	}
	if guard, reason := r.guardBlock(r.RootInvocationID, "gate"); guard != g.ID || reason != "guard_evaluation_failed" {
		t.Fatalf("a failed guard admitted work: %s %s", guard, reason)
	}
	if r.activationForInvocation(r.RootInvocationID, "gate") != nil {
		t.Fatal("a failed guard opened its activation")
	}
}

// Two literals of different types are an authoring mistake fully visible in
// the declaration, so the registration is refused before the Run exists.
func TestGuardLiteralTypeMismatchIsRefusedAtRegistration(t *testing.T) {
	e, workflow, options := guardFixture(t, `{"flag":true}`)
	options.Guards = []GuardDeclaration{{Kind: "start", TargetStageID: "gate", Predicate: flow.Predicate{Op: "eq",
		Left:  &flow.Operand{Kind: "literal", Value: json.RawMessage(`"one"`)},
		Right: &flow.Operand{Kind: "literal", Value: json.RawMessage(`1`)}}}}
	writeRuntimeJSON(t, e.Root+"/workflows/guard-parent.json", workflow)
	writeRuntimeJSON(t, e.Root+"/prifly.json", e.Config)
	_, err := e.Start(context.Background(), options)
	var problem *flow.Problem
	if err == nil || !errors.As(err, &problem) || problem.Code != "condition_type_mismatch" {
		t.Fatalf("an incomparable literal comparison was accepted: %v", err)
	}
}

// A reference to a port the guarded workflow does not declare, and one this
// build cannot resolve for a guard at all, are both refused at registration.
func TestGuardFieldReferenceIsCheckedAgainstThePinnedPlan(t *testing.T) {
	for _, ref := range []flow.FieldRef{
		{From: "workflow_input", Port: "absent"},
		{From: "stage_output", StageID: "probe", Port: "absent"},
		{From: "iteration_output", Port: "verdict"},
	} {
		e, workflow, options := guardFixture(t, `{"flag":true}`)
		options.Guards = []GuardDeclaration{{Kind: "start", TargetStageID: "gate", Predicate: guardPredicate(ref, true)}}
		writeRuntimeJSON(t, e.Root+"/workflows/guard-parent.json", workflow)
		writeRuntimeJSON(t, e.Root+"/prifly.json", e.Config)
		if _, err := e.Start(context.Background(), options); err == nil {
			t.Fatalf("an unresolvable guard reference was accepted: %+v", ref)
		}
	}
}
