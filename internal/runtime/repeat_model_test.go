package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

// This is an in-memory model fixture, not execution or crash evidence. Its
// definitions are compiled normally; real repeat tests use runtime admission.
func repeatModelFixture(t *testing.T, count int) Run {
	t.Helper()
	r := invocationPlanFixture(t)
	var workflow flow.WorkflowRevision
	if err := json.Unmarshal(r.Workflow, &workflow); err != nil {
		t.Fatal(err)
	}
	bodyRef := r.Invocations["invocation:child"].WorkflowRef
	workflow.Limits.MaxControlTransitions = 12
	workflow.Definition.Stages["enter"] = flow.Stage{
		Kind: "repeat", BodyWorkflowRef: bodyRef, InitialBindings: map[string]flow.Binding{}, NextBindings: map[string]flow.Binding{},
		ContinueOn: []string{"no_work"}, MaxIterations: 3, OnComplete: map[string]string{"no_work": "done"}, OnLimit: "done",
		Until: flow.Predicate{Op: "eq", Left: &flow.Operand{Kind: "literal", Value: json.RawMessage("true")}, Right: &flow.Operand{Kind: "literal", Value: json.RawMessage("false")}},
	}
	data, err := canonical(workflow)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := flow.CompileProfile(data, "json", r.registry(), flow.CoreProfile)
	if err != nil {
		t.Fatal(err)
	}
	r.SchemaVersion, r.Status = CoreRepeatStateVersion, "ready"
	r.Workflow, r.WorkflowRef = plan.Canonical, planRef(plan)
	r.AuthorityID, r.ProjectID, r.CoreBuild = "authority:model", "project:model", Version
	r.TrustProfile, r.InteractionMode, r.ExecutionMode, r.CapacityProfile = "core-local/cooperative", "with_human", "managed", "foundation:one-slot"
	r.Brief = ArtifactRef{ArtifactID: "artifact:model-brief", Revision: 1, Digest: rawDigest([]byte("model brief"))}
	r.LockRef = flow.Ref{ID: "test:lock/model", Version: "1.0.0", Digest: rawDigest([]byte("model lock"))}
	r.Created, r.LastObserved = timingObservation(0), timingObservation(10)
	r.EffectiveConfiguration = &EffectiveConfiguration{SchemaVersion: "effective-configuration/1", WorkflowRef: r.WorkflowRef, Inputs: map[string]ConfigurationValue{}}
	r.WorkflowConfigurations = map[string]*EffectiveConfiguration{
		r.WorkflowRef.Digest: r.EffectiveConfiguration,
		bodyRef.Digest:       {SchemaVersion: "effective-configuration/1", WorkflowRef: bodyRef, Inputs: map[string]ConfigurationValue{}},
	}
	r.Definitions = nil
	for ref, value := range plan.Registry {
		r.Definitions = append(r.Definitions, PinnedDefinition{Ref: ref, Bytes: value, RawDigest: rawDigest(value)})
	}
	r.ControlTransitions = 1
	if count > 0 {
		r.ControlTransitions = int64(count * 2)
	}
	r.Invocations = map[string]*Invocation{r.RootInvocationID: {
		ID: r.RootInvocationID, RunID: r.ID, WorkflowRef: r.WorkflowRef, Status: "waiting", Inputs: r.Inputs, Outputs: r.Outputs,
		Ready: []string{}, Created: r.Created, ControlTransitions: r.ControlTransitions,
	}}
	a := &Activation{ID: "activation:enter", InvocationID: r.RootInvocationID, StageID: "enter", Kind: "repeat", Status: "waiting", Created: r.Created, Repeat: &RepeatProgress{IterationCount: int64(count)}}
	r.Activations = map[string]*Activation{a.ID: a}
	if count == 0 {
		a.Status = "ready"
		r.Invocations[r.RootInvocationID].Status, r.Invocations[r.RootInvocationID].Ready = "ready", []string{"enter"}
	}
	for i := 1; i <= count; i++ {
		iteration, id := int64(i), fmt.Sprintf("invocation:body-%d", i)
		inv := &Invocation{ID: id, RunID: r.ID, ParentInvocationID: r.RootInvocationID, CallerActivationID: a.ID, Iteration: &iteration,
			WorkflowRef: bodyRef, Status: "ready", Inputs: map[string]ArtifactRef{}, Outputs: map[string]ArtifactRef{}, Ready: []string{"done"}, Created: timingObservation(int64(i)), ControlTransitions: 1}
		if i < count {
			outcome := "no_work"
			inv.Status, inv.Outcome, inv.Settled, inv.Ready = "completed", &outcome, &inv.Created, []string{}
		}
		r.Invocations[id] = inv
		stage := &Activation{ID: fmt.Sprintf("activation:body-%d", i), InvocationID: id, StageID: "done", Kind: "finish", Status: inv.Status, Created: inv.Created, Settled: inv.Settled}
		r.Activations[stage.ID] = stage
		a.Repeat.CurrentBodyInvocationID = id
	}
	if count > 1 {
		a.Repeat.LastDecision = repeatModelDecision(r, int64(count-1), "continue", "false")
		a.Repeat.LastDecision.NextBodyInvocationID = a.Repeat.CurrentBodyInvocationID
	}
	return r
}

func repeatModelDecision(r Run, iteration int64, route, until string) *RepeatDecision {
	a := r.Activations["activation:enter"]
	body := r.Invocations[fmt.Sprintf("invocation:body-%d", iteration)]
	return &RepeatDecision{SchemaVersion: RepeatDecisionVersion, ID: derivedID("decision", r.ID, a.ID, body.ID), RunID: r.ID, InvocationID: a.InvocationID,
		ActivationID: a.ID, StageID: a.StageID, WorkflowRef: r.WorkflowRef, BodyInvocationID: body.ID, Iteration: iteration, BodyStatus: body.Status,
		BodyOutcome: body.Outcome, UntilResult: until, Inputs: []ChoiceInput{}, Route: route, Observed: timingObservation(10)}
}

func finishRepeatModel(r *Run, route, until string) {
	a := r.Activations["activation:enter"]
	body := r.currentBodyForRepeat(a.ID)
	outcome, observed := "no_work", timingObservation(10)
	body.Status, body.Outcome, body.Settled, body.Ready = "completed", &outcome, &observed, []string{}
	a.Status, a.Settled = "completed", &observed
	a.Repeat.LastDecision = repeatModelDecision(*r, a.Repeat.IterationCount, route, until)
	a.Repeat.LastDecision.NextStageID = "done"
}

func TestRepeatModelScopeAndHistoricalMembership(t *testing.T) {
	r := repeatModelFixture(t, 2)
	a := r.Activations["activation:enter"]
	if err := r.repeatProgressInvariant(a); err != nil {
		t.Fatal(err)
	}
	if r.currentBodyForRepeat(a.ID) != r.Invocations["invocation:body-2"] || r.childForCall(a.ID) != nil {
		t.Fatal("repeat was treated as a one-child call or chose a historical body")
	}
	for _, id := range []string{"invocation:body-1", "invocation:body-2"} {
		inv := r.Invocations[id]
		if !r.childMatchesCaller(inv, a) || !r.withinInvocation(id, r.RootInvocationID) {
			t.Fatal("historical or current body lost its owner", id)
		}
		plan, err := r.planFor(id)
		if err != nil || planRef(plan) != inv.WorkflowRef || r.activationForInvocation(id, "done").InvocationID != id {
			t.Fatal("body plan or same-named stage escaped its invocation", id, err)
		}
	}
	r.Invocations["invocation:body-1"].Outputs["value"] = ArtifactRef{ArtifactID: "artifact:old", Revision: 1, Digest: rawDigest([]byte("old"))}
	if _, exists := r.outputsFor(r.currentBodyForRepeat(a.ID).ID)["value"]; exists {
		t.Fatal("absent current output fell back to a previous iteration")
	}
	r.Stops = []Stop{{ID: "stop:old", Kind: "pause", Status: "active", Scope: "invocation", ScopeID: "invocation:body-1"}}
	if r.admissionsBlockedFor("invocation:body-2") || !r.admissionsBlockedFor("invocation:body-1") {
		t.Fatal("a historical body's stop escaped its scope")
	}
}

func TestRepeatModelRejectsCounterAndOwnerDrift(t *testing.T) {
	for name, change := range map[string]func(*Run){
		"reset_counter":       func(r *Run) { r.Activations["activation:enter"].Repeat.IterationCount = 1 },
		"missing_body":        func(r *Run) { delete(r.Invocations, "invocation:body-1") },
		"duplicate_iteration": func(r *Run) { *r.Invocations["invocation:body-2"].Iteration = 1 },
		"old_current_body":    func(r *Run) { r.Activations["activation:enter"].Repeat.CurrentBodyInvocationID = "invocation:body-1" },
		"missing_decision":    func(r *Run) { r.Activations["activation:enter"].Repeat.LastDecision = nil },
		"wrong_next_body": func(r *Run) {
			r.Activations["activation:enter"].Repeat.LastDecision.NextBodyInvocationID = "invocation:body-1"
		},
		"wrong_decision_parent":   func(r *Run) { r.Activations["activation:enter"].Repeat.LastDecision.InvocationID = "invocation:body-1" },
		"future_decision":         func(r *Run) { r.Activations["activation:enter"].Repeat.LastDecision.Iteration = 3 },
		"unobserved_decision":     func(r *Run) { r.Activations["activation:enter"].Repeat.LastDecision.Observed = Observation{} },
		"wrong_run":               func(r *Run) { r.Invocations["invocation:body-2"].RunID = "run:other" },
		"wrong_parent":            func(r *Run) { r.Invocations["invocation:body-2"].ParentInvocationID = "invocation:body-1" },
		"wrong_exact_body":        func(r *Run) { r.Invocations["invocation:body-2"].WorkflowRef = r.WorkflowRef },
		"cancelled_previous_body": func(r *Run) { r.Invocations["invocation:body-1"].Status = "cancelled" },
		"fictional_step":          func(r *Run) { r.Activations["activation:enter"].StepID = "step:fictional" },
	} {
		t.Run(name, func(t *testing.T) {
			r := repeatModelFixture(t, 2)
			change(&r)
			before, err := canonicalState(r)
			if err != nil {
				t.Fatal(err)
			}
			if err := r.repeatProgressInvariant(r.Activations["activation:enter"]); err == nil {
				t.Fatal("invalid repeat projection accepted")
			}
			after, err := canonicalState(r)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("validation repaired or rewrote saved progress", err)
			}
		})
	}
}

func TestRepeatModelBudgetUsesPinnedBodyAndAncestors(t *testing.T) {
	r := repeatModelFixture(t, 2)
	beforeRoot, beforeCurrent := r.ControlTransitions, r.Invocations["invocation:body-2"].ControlTransitions
	if err := r.chargeInvocation("invocation:body-2", 1, 0); err != nil {
		t.Fatal(err)
	}
	if r.ControlTransitions != beforeRoot+1 || r.Invocations[r.RootInvocationID].ControlTransitions != r.ControlTransitions || r.Invocations["invocation:body-2"].ControlTransitions != beforeCurrent+1 || r.Invocations["invocation:body-1"].ControlTransitions != 1 {
		t.Fatal("budget reset or charge escaped its ancestor chain")
	}
	for name, change := range map[string]func(*Run){
		"root_limit":    func(r *Run) { r.ControlTransitions = 12; r.Invocations[r.RootInvocationID].ControlTransitions = 12 },
		"child_limit":   func(r *Run) { r.Invocations["invocation:body-2"].ControlTransitions = 3 },
		"different_ref": func(r *Run) { r.Invocations["invocation:body-2"].WorkflowRef = r.WorkflowRef },
		"changed_pinned_limits": func(r *Run) {
			r.Workflow = bytes.Replace(r.Workflow, []byte(`"max_iterations":3`), []byte(`"max_iterations":1`), 1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := repeatModelFixture(t, 2)
			change(&r)
			before, _ := canonicalState(r)
			if err := r.chargeInvocation("invocation:body-2", 1, 0); err == nil {
				t.Fatal("invalid or exhausted pinned budget accepted")
			}
			after, err := canonicalState(r)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("failed budget check partially charged progress", err)
			}
		})
	}
}

func TestRepeatModelVersionedWire(t *testing.T) {
	for _, version := range []string{StateVersion, CoreStateVersion, "core-state/26", ""} {
		if isInvocationState(version) {
			t.Fatal("unknown or flat state entered the invocation whitelist", version)
		}
	}
	if !isInvocationState(CoreInvocationStateVersion) || !isInvocationState(CoreRepeatStateVersion) || !isInvocationState(CoreContextStateVersion) || !isInvocationState(CoreSessionStateVersion) || !isInvocationState(CoreWaiverStateVersion) || !isInvocationState(CoreParallelStateVersion) || !isInvocationState(CoreMapStateVersion) || !isInvocationState(CoreWaitStateVersion) || !isInvocationState(CoreGuardStateVersion) || !isInvocationState(CoreReportedCostStateVersion) || !isInvocationState(CoreArtifactPublicationStateVersion) || !isInvocationState(CoreArtifactClosureStateVersion) || !isInvocationState(CorePublicationSubscriptionStateVersion) || !isInvocationState(CorePublicationChecksStateVersion) || !isInvocationState(CorePublicationNewOnlyStateVersion) || !isInvocationState(CorePublicationFailureStateVersion) || !isInvocationState(CoreActionIntentStateVersion) || !isInvocationState(CoreActionAdmissionStateVersion) || !isInvocationState(CoreActionGrantAdmissionStateVersion) || !isInvocationState(CoreActionDeliveryStateVersion) || !isInvocationState(CoreForkStateVersion) || !isInvocationState(CoreWorkspaceStateVersion) || !isInvocationState(CoreWorkspaceTreeStateVersion) || !isInvocationState(CoreDecisionStateVersion) {
		t.Fatal("supported invocation state disappeared")
	}
	r := repeatModelFixture(t, 1)
	data, err := canonicalState(r)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatal(err)
	}
	if _, exists := wire["ready_stages"]; exists {
		t.Fatal("state3 serialized a second Run frontier")
	}
	for _, value := range []json.RawMessage{json.RawMessage(`null`), json.RawMessage(`[]`)} {
		wire["ready_stages"] = value
		encoded, err := json.Marshal(wire)
		if err != nil || decodeState(encoded, &Run{}) == nil {
			t.Fatal("state3 accepted a second Run frontier", err)
		}
	}
	var roundtrip Run
	if err := decodeState(data, &roundtrip); err != nil || roundtrip.Activations["activation:enter"].Repeat.IterationCount != 1 || *roundtrip.Invocations["invocation:body-1"].Iteration != 1 {
		t.Fatal("state3 lost its repeat frontier", err)
	}
	for _, field := range []string{"repeat", "iteration"} {
		old := invocationPlanFixture(t)
		encoded, _ := canonicalState(old)
		var value map[string]any
		if err := json.Unmarshal(encoded, &value); err != nil {
			t.Fatal(err)
		}
		collection, id := "activations", "activation:enter"
		if field == "iteration" {
			collection, id = "invocations", "invocation:child"
		}
		value[collection].(map[string]any)[id].(map[string]any)[field] = nil
		encoded, _ = json.Marshal(value)
		if decodeState(encoded, &Run{}) == nil {
			t.Fatal("state2 accepted an explicit null repeat extension", field)
		}
	}
	r.SchemaVersion = CoreInvocationStateVersion
	if _, err := canonicalState(r); err == nil {
		t.Fatal("repeat state was silently written as invocation version 2")
	}
	old := repeatModelFixture(t, 0)
	old.SchemaVersion, old.Activations["activation:enter"].Repeat = CoreInvocationStateVersion, nil
	if !supportedRun(old) {
		t.Fatal("fixture must reach the pinned plan version guard")
	}
	if _, err := old.plan(); err == nil || !strings.Contains(err.Error(), "repeat closure requires core-state/3") {
		t.Fatal("a pinned repeat closure was reinterpreted under the old state version", err)
	}
}

func TestRepeatPublicSchemaAndDecisionRoutes(t *testing.T) {
	distributed, err := os.ReadFile("../../schemas/core/repeats.schema.json")
	if err != nil || !bytes.Equal(distributed, repeatPublicContracts) {
		t.Fatal("repeat embedded/distributed schemas differ", err)
	}
	for _, count := range []int{0, 1, 2} {
		r := repeatModelFixture(t, count)
		if err := r.repeatProgressInvariant(r.Activations["activation:enter"]); err != nil {
			t.Fatal(count, err)
		}
		if err := validatePublic(t, "CoreRunStateV3", r); err != nil {
			t.Fatal("state3 shape rejected", count, err)
		}
		if validatePublic(t, "CoreRunStateV2", r) == nil {
			t.Fatal("state3 accepted by the frozen version 2 schema")
		}
	}
	for route, until := range map[string]string{"on_complete": "true", "on_limit": "false", "on_unknown": "unknown"} {
		r := repeatModelFixture(t, 1)
		finishRepeatModel(&r, route, until)
		a := r.Activations["activation:enter"]
		if err := r.repeatProgressInvariant(a); err != nil {
			t.Fatal(route, err)
		}
		if err := validatePublic(t, "RepeatDecision", a.Repeat.LastDecision); err != nil {
			t.Fatal(route, err)
		}
	}
	for _, test := range []struct {
		name, bodyStatus, route, until, failure string
	}{
		{"body_error_handled", "failed", "on_error", "not_evaluated", "child_failed"},
		{"body_error_unhandled", "failed", "failed", "not_evaluated", "child_failed"},
		{"predicate_error_handled", "completed", "on_error", "error", "condition_type_mismatch"},
		{"predicate_error_unhandled", "completed", "failed", "error", "condition_type_mismatch"},
		{"next_binding_error_handled", "completed", "on_error", "false", "input_binding_failed"},
		{"unknown_unhandled", "completed", "failed", "unknown", "condition_unknown"},
		{"outcome_unhandled", "completed", "failed", "not_evaluated", "unhandled_outcome"},
		{"limit_unhandled", "completed", "failed", "false", "no_transition"},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := repeatModelFixture(t, 1)
			finishRepeatModel(&r, "on_complete", "true")
			a := r.Activations["activation:enter"]
			body := r.currentBodyForRepeat(a.ID)
			body.Status, a.Status = test.bodyStatus, "failed"
			if body.Status == "failed" {
				body.Outcome = nil
			}
			d := repeatModelDecision(r, 1, test.route, test.until)
			d.Failure = test.failure
			if d.Route == "on_error" {
				d.NextStageID = "done"
			}
			a.Repeat.LastDecision = d
			if err := r.repeatProgressInvariant(a); err != nil {
				t.Fatal("valid failure route rejected", err)
			}
			if err := validatePublic(t, "RepeatDecision", d); err != nil {
				t.Fatal("valid failure decision shape rejected", err)
			}
			if test.failure == "condition_unknown" || test.failure == "unhandled_outcome" || test.failure == "no_transition" {
				d.Route, d.NextStageID = "on_error", "done"
				if r.repeatProgressInvariant(a) == nil || validatePublic(t, "RepeatDecision", d) == nil {
					t.Fatal("unknown condition or unhandled outcome became a catchable error")
				}
			}
		})
	}
	r := repeatModelFixture(t, 2)
	d := r.Activations["activation:enter"].Repeat.LastDecision
	d.Inputs = []ChoiceInput{{FieldRef: flow.FieldRef{From: "iteration_output", Port: "value"}, Availability: "present", SourceRef: &ArtifactRef{ArtifactID: "artifact:value", Revision: 1, Digest: rawDigest([]byte("false"))}}}
	if err := validatePublic(t, "RepeatDecision", d); err != nil {
		t.Fatal("repeat input trace rejected its iteration scope", err)
	}
	encoded, _ := json.Marshal(d)
	for name, change := range map[string]func(map[string]any){
		"unknown_field":       func(v map[string]any) { v["reset_count"] = true },
		"old_version":         func(v map[string]any) { v["schema_version"] = "choice-decision/1" },
		"zero_iteration":      func(v map[string]any) { v["iteration"] = 0 },
		"unbounded_iteration": func(v map[string]any) { v["iteration"] = 101 },
		"missing_next":        func(v map[string]any) { delete(v, "next_body_workflow_invocation_id") },
		"two_next_routes":     func(v map[string]any) { v["next_stage_id"] = "done" },
		"continue_on_true":    func(v map[string]any) { v["until_result"] = "true" },
		"continue_on_failure": func(v map[string]any) { v["body_status"] = "failed"; v["body_outcome"] = nil },
		"unobserved":          func(v map[string]any) { v["observation"].(map[string]any)["session"] = "" },
		"unread_trace":        func(v map[string]any) { v["until_result"] = "not_evaluated" },
	} {
		t.Run(name, func(t *testing.T) {
			var object map[string]any
			if err := json.Unmarshal(encoded, &object); err != nil {
				t.Fatal(err)
			}
			change(object)
			if validatePublic(t, "RepeatDecision", object) == nil {
				t.Fatal("invalid repeat decision shape accepted")
			}
		})
	}
}
