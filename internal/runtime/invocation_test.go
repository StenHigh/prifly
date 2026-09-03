package runtime

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

// An in-memory fixture for identity and scope predicates, not worker or journal
// evidence. Real call execution/recovery tests use the runtime admission path.
func invocationScopesFixture() Run {
	r := Run{SchemaVersion: CoreInvocationStateVersion, Profile: flow.CoreProfile, ID: "run:scopes", RootInvocationID: "invocation:root", Invocations: map[string]*Invocation{}, Activations: map[string]*Activation{}}
	for _, name := range []string{"root", "first", "second", "nested"} {
		id := "invocation:" + name
		ref := ArtifactRef{ArtifactID: "artifact:" + name, Revision: 1, Digest: rawDigest([]byte(name))}
		inv := &Invocation{ID: id, RunID: r.ID, WorkflowRef: flow.Ref{ID: "test:workflow/" + name, Version: "1.0.0", Digest: rawDigest([]byte(name))}, Status: "ready", Inputs: map[string]ArtifactRef{"value": ref}, Outputs: map[string]ArtifactRef{"value": ref}, Ready: []string{"done"}, Created: timingObservation(0)}
		r.Invocations[id] = inv
		r.Activations["activation:"+name] = &Activation{ID: "activation:" + name, InvocationID: id, StageID: "done", Kind: "finish", Status: "ready"}
		if name != "root" {
			inv.ParentInvocationID = r.RootInvocationID
			if name == "nested" {
				inv.ParentInvocationID = "invocation:first"
			}
			inv.CallerActivationID = "activation:call-" + name
			r.Activations[inv.CallerActivationID] = &Activation{ID: inv.CallerActivationID, InvocationID: inv.ParentInvocationID, StageID: "call-" + name, Kind: "call", Status: "waiting"}
		}
	}
	return r
}

func TestInvocationScopesQualifyStageAndPorts(t *testing.T) {
	r := invocationScopesFixture()
	for _, name := range []string{"root", "first", "second", "nested"} {
		id := "invocation:" + name
		a := r.activationForInvocation(id, "done")
		if a == nil || a.ID != "activation:"+name {
			t.Fatalf("same stage ID escaped invocation %s: %+v", id, a)
		}
		if r.inputsFor(id)["value"].ArtifactID != "artifact:"+name || r.outputsFor(id)["value"].ArtifactID != "artifact:"+name {
			t.Fatal("same port name escaped its invocation", name)
		}
	}
	if r.inputsFor("invocation:missing") != nil || r.readyFor("invocation:missing") != nil || r.activationForInvocation("invocation:missing", "done") != nil {
		t.Fatal("unknown invocation fell back to the root")
	}
	ready := []string{"next"}
	if err := r.setReadyFor("invocation:first", ready); err != nil {
		t.Fatal(err)
	}
	ready[0] = "changed-by-caller"
	if !reflect.DeepEqual(r.readyFor("invocation:first"), []string{"next"}) || !reflect.DeepEqual(r.readyFor("invocation:second"), []string{"done"}) || r.Ready != nil {
		t.Fatal("frontiers alias caller memory, another invocation, or legacy Run.Ready")
	}
	if err := r.setReadyFor("invocation:missing", []string{"next"}); err == nil {
		t.Fatal("unknown frontier owner was repaired")
	}
	duplicate := *r.Activations["activation:first"]
	duplicate.ID = "activation:duplicate"
	r.Activations[duplicate.ID] = &duplicate
	if r.activationForInvocation("invocation:first", "done") != nil {
		t.Fatal("ambiguous activation was selected by map iteration")
	}
}

func TestInvocationCallerUniquenessIsSpecificToCall(t *testing.T) {
	r := invocationScopesFixture()
	if got := r.childForCall("activation:call-first"); got == nil || got.ID != "invocation:first" {
		t.Fatal("call did not resolve its own child")
	}
	r.Activations["activation:call-first"].Kind = "repeat"
	if r.childForCall("activation:call-first") != nil {
		t.Fatal("call-specific lookup was applied to another operator")
	}
	r.Activations["activation:call-first"].Kind = "call"
	duplicate := *r.Invocations["invocation:first"]
	duplicate.ID = "invocation:duplicate"
	r.Invocations[duplicate.ID] = &duplicate
	if r.childForCall("activation:call-first") != nil {
		t.Fatal("ambiguous child was selected by map iteration")
	}
}

func TestInvocationScopedRestrictionsAndResumeLatches(t *testing.T) {
	r := invocationScopesFixture()
	r.Stops = []Stop{{ID: "stop:first", Kind: "pause", Status: "active", Scope: "invocation", ScopeID: "invocation:first"}}
	for _, id := range []string{"invocation:first", "invocation:nested"} {
		if !r.restrictedFor(id) || !r.admissionsBlockedFor(id) {
			t.Fatal("parent restriction did not cover its descendant", id)
		}
	}
	for _, id := range []string{"invocation:root", "invocation:second"} {
		if r.restrictedFor(id) || r.admissionsBlockedFor(id) {
			t.Fatal("child restriction spread to an ancestor or sibling", id)
		}
	}
	r.Stops[0].Status = "released"
	r.Invocations["invocation:first"].ResumeRequired = true
	if r.restrictedFor("invocation:nested") || !r.admissionsBlockedFor("invocation:nested") || r.admissionsBlockedFor("invocation:second") {
		t.Fatal("stop release implicitly cleared the parent's explicit resume latch")
	}
	r.ResumeRequired = true
	if !r.admissionsBlockedFor("invocation:second") {
		t.Fatal("child bypassed the Run resume latch")
	}
	r.ResumeRequired = false
	r.Stops = []Stop{{ID: "stop:run", Status: "active"}}
	if !r.restrictedFor("invocation:second") {
		t.Fatal("legacy empty scope stopped applying to the whole Run")
	}
	r.Stops[0].Scope = "unrecognized"
	if !r.restrictedFor("invocation:second") {
		t.Fatal("unknown stop scope granted admission")
	}
}

func TestInvocationAncestryDoesNotRepairMissingOrCyclicScopes(t *testing.T) {
	r := invocationScopesFixture()
	if !r.withinInvocation("invocation:nested", "invocation:first") || !r.withinInvocation("invocation:nested", r.RootInvocationID) || r.withinInvocation("invocation:second", "invocation:first") {
		t.Fatal("invocation ancestry does not follow parent identity")
	}
	if r.withinInvocation("invocation:missing", r.RootInvocationID) || !r.admissionsBlockedFor("invocation:missing") {
		t.Fatal("unknown invocation was treated as a valid root scope")
	}
	r.Invocations["invocation:first"].ParentInvocationID = "invocation:nested"
	if r.withinInvocation("invocation:nested", "invocation:first") || !r.admissionsBlockedFor("invocation:nested") {
		t.Fatal("cyclic ancestry was used as a valid scope")
	}
	if _, err := r.planFor("invocation:nested"); err == nil {
		t.Fatal("cyclic invocation linkage was compiled as a root workflow")
	}
}

func TestInvocationWirePreservesLegacyRunAndRegistry(t *testing.T) {
	type withoutMarshal Run
	for _, state := range []string{StateVersion, CoreStateVersion} {
		for _, ready := range [][]string{nil, {}, {"done"}} {
			r := Run{SchemaVersion: state, RootInvocationID: "invocation:root", Ready: ready}
			before, err := json.Marshal(withoutMarshal(r))
			if err != nil {
				t.Fatal(err)
			}
			after, err := json.Marshal(r)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("legacy Run marshal changed bytes", err)
			}
			if bytes.Contains(after, []byte(`"invocations"`)) || bytes.Contains(after, []byte(`"workflow_configurations"`)) {
				t.Fatal("legacy state gained invocation fields")
			}
		}
	}
	registry, err := json.Marshal(RegistryFile{SchemaVersion: "1", Entries: []Definition{}})
	if err != nil || string(registry) != `{"schema_version":"1","entries":[]}` {
		t.Fatal("legacy registry gained alias metadata", string(registry), err)
	}
	r := invocationScopesFixture()
	encoded, err := json.Marshal(r)
	var wire map[string]json.RawMessage
	if err != nil || json.Unmarshal(encoded, &wire) != nil {
		t.Fatal(err)
	}
	if _, exists := wire["ready_stages"]; exists {
		t.Fatal("invocation state has two authoritative frontiers")
	}
	var decoded Run
	if err := decodeState(encoded, &decoded); err != nil || decoded.Ready != nil || !reflect.DeepEqual(decoded.Invocations, r.Invocations) {
		t.Fatal("invocation state failed a typed round trip", err)
	}
	for _, value := range []json.RawMessage{json.RawMessage(`null`), json.RawMessage(`[]`)} {
		wire["ready_stages"] = value
		data, err := json.Marshal(wire)
		if err != nil || decodeState(data, &Run{}) == nil {
			t.Fatal("state v2 accepted a second authoritative frontier", err)
		}
	}
	for _, state := range []string{StateVersion, CoreStateVersion} {
		for _, field := range []string{"invocations", "workflow_configurations", "stops"} {
			for _, value := range []json.RawMessage{json.RawMessage(`null`), json.RawMessage(`{}`)} {
				legacy := map[string]json.RawMessage{"schema_version": json.RawMessage(`"` + state + `"`), field: value}
				if field == "stops" {
					legacy[field] = json.RawMessage(`[{"scope":null,"scope_id":""}]`)
				}
				data, err := json.Marshal(legacy)
				if err != nil || decodeState(data, &Run{}) == nil {
					t.Fatal("legacy state silently accepted scoped fields", field, err)
				}
			}
		}
	}
	r.Ready = []string{"legacy-stage"}
	if _, err := json.Marshal(r); err == nil {
		t.Fatal("serialization silently discarded a live legacy frontier")
	}
}

func TestInvocationBudgetArithmeticDoesNotOverflow(t *testing.T) {
	for _, test := range []struct {
		used, additional, limit int64
		fits                    bool
	}{
		{0, 1, 1, true}, {1, 0, 1, true}, {1, 1, 1, false},
		{math.MaxInt64 - 1, 1, math.MaxInt64, true}, {math.MaxInt64, 1, math.MaxInt64, false},
		{-1, 0, 1, false}, {0, -1, 1, false}, {0, 0, -1, false},
	} {
		if invocationBudgetFits(test.used, test.additional, test.limit) != test.fits {
			t.Fatalf("incorrect bounded charge: %+v", test)
		}
	}
}

// Only immutable workflow definitions are compiled here. This fixture does not
// insert a snapshot, manufacture an accepted Result or launch a worker.
func invocationPlanFixture(t *testing.T) Run {
	t.Helper()
	definitions, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	registry := flow.Registry{}
	for _, definition := range definitions {
		registry[definition.Ref] = definition.Bytes
	}
	child := flow.WorkflowRevision{SchemaVersion: "1", ID: "test:workflow/child", Version: "1.0.0", Title: "Child scope unit fixture", Inputs: map[string]flow.InputPort{}, Outputs: map[string]flow.OutputPort{}, AllowedOutcomes: []string{"no_work"}, Limits: flow.Limits{MaxStepInstances: 1, MaxControlTransitions: 3, MaxParallelism: 1}, PolicyRef: builtinRef(definitions, "core:policy/local")}
	child.Definition.Entry = "done"
	child.Definition.Stages = map[string]flow.Stage{"done": {Kind: "finish", Outcome: "no_work", OutputBindings: map[string]flow.Binding{}}}
	childBytes, err := canonical(child)
	if err != nil {
		t.Fatal(err)
	}
	childDigest, err := flow.Digest(childBytes)
	if err != nil {
		t.Fatal(err)
	}
	childRef := flow.Ref{ID: child.ID, Version: child.Version, Digest: childDigest}
	registry[childRef] = childBytes
	root := child
	root.ID, root.Title = "test:workflow/root", "Parent scope unit fixture"
	root.PolicyRef = builtinVersionRef(definitions, "core:policy/local", "2.0.0")
	root.Limits = flow.Limits{MaxStepInstances: 2, MaxControlTransitions: 6, MaxParallelism: 1, MaxChildDepth: 1}
	root.Definition.Entry = "enter"
	root.Definition.Stages = map[string]flow.Stage{
		"enter": {Kind: "call", WorkflowRef: childRef, InputBindings: map[string]flow.Binding{}, On: map[string]string{"no_work": "done"}},
		"done":  {Kind: "finish", Outcome: "no_work", OutputBindings: map[string]flow.Binding{}},
	}
	rootBytes, err := canonical(root)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := flow.CompileProfile(rootBytes, "json", registry, flow.CoreProfile)
	if err != nil {
		t.Fatal(err)
	}
	rootRef := flow.Ref{ID: root.ID, Version: root.Version, Digest: plan.Digest}
	rootConfig := &EffectiveConfiguration{SchemaVersion: "effective-configuration/1", WorkflowRef: rootRef, Inputs: map[string]ConfigurationValue{}}
	childConfig := &EffectiveConfiguration{SchemaVersion: "effective-configuration/1", WorkflowRef: childRef, Inputs: map[string]ConfigurationValue{}}
	r := Run{SchemaVersion: CoreInvocationStateVersion, Profile: flow.CoreProfile, ID: "run:plan", RootInvocationID: "invocation:root", WorkflowRef: rootRef, Workflow: plan.Canonical, EffectiveConfiguration: rootConfig, WorkflowConfigurations: map[string]*EffectiveConfiguration{rootRef.Digest: rootConfig, childRef.Digest: childConfig}, Inputs: map[string]ArtifactRef{}, Outputs: map[string]ArtifactRef{}, ControlTransitions: 2, Steps: map[string]*Step{}, Invocations: map[string]*Invocation{}, Activations: map[string]*Activation{}}
	for ref, data := range plan.Registry {
		r.Definitions = append(r.Definitions, PinnedDefinition{Ref: ref, Bytes: data, RawDigest: rawDigest(data)})
	}
	r.Invocations[r.RootInvocationID] = &Invocation{ID: r.RootInvocationID, RunID: r.ID, WorkflowRef: rootRef, Status: "running", Inputs: r.Inputs, Outputs: r.Outputs, Ready: []string{}, Created: timingObservation(0), ControlTransitions: 2}
	r.Invocations["invocation:child"] = &Invocation{ID: "invocation:child", RunID: r.ID, ParentInvocationID: r.RootInvocationID, CallerActivationID: "activation:enter", WorkflowRef: childRef, Status: "ready", Inputs: map[string]ArtifactRef{}, Outputs: map[string]ArtifactRef{}, Ready: []string{"done"}, Created: timingObservation(1), ControlTransitions: 1}
	r.Activations["activation:enter"] = &Activation{ID: "activation:enter", InvocationID: r.RootInvocationID, StageID: "enter", Kind: "call", Status: "waiting", Created: timingObservation(0)}
	r.Activations["activation:done"] = &Activation{ID: "activation:done", InvocationID: "invocation:child", StageID: "done", Kind: "finish", Status: "ready", Created: timingObservation(1)}
	return r
}

func TestInvocationPlanFollowsPinnedCallerPath(t *testing.T) {
	r := invocationPlanFixture(t)
	child, err := r.planFor("invocation:child")
	if err != nil || child.Workflow.ID != "test:workflow/child" {
		t.Fatal("child plan resolved through root identity", err)
	}
	r.Invocations["invocation:child"].WorkflowRef = r.WorkflowRef
	if _, err := r.planFor("invocation:child"); err == nil {
		t.Fatal("caller link silently accepted a different exact workflow")
	}
}

func TestInvocationBudgetChargesAncestorsAtomically(t *testing.T) {
	t.Run("successful_charge", func(t *testing.T) {
		r := invocationPlanFixture(t)
		if err := r.chargeInvocation("invocation:child", 1, 1); err != nil {
			t.Fatal(err)
		}
		root, child := r.Invocations[r.RootInvocationID], r.Invocations["invocation:child"]
		if r.ControlTransitions != 3 || root.ControlTransitions != 3 || child.ControlTransitions != 2 || root.StepInstances != 1 || child.StepInstances != 1 {
			t.Fatal("charge did not reach every ancestor exactly once")
		}
	})
	for _, test := range []struct {
		name            string
		change          func(*Run)
		controls, steps int64
	}{
		{"child_control_limit", func(r *Run) {
			r.Invocations["invocation:child"].ControlTransitions = 3
			r.ControlTransitions = 4
			r.Invocations[r.RootInvocationID].ControlTransitions = 4
		}, 1, 0},
		{"root_control_limit", func(r *Run) { r.ControlTransitions = 6; r.Invocations[r.RootInvocationID].ControlTransitions = 6 }, 1, 0},
		{"child_step_limit", func(r *Run) {
			r.Invocations["invocation:child"].StepInstances = 1
			r.Invocations[r.RootInvocationID].StepInstances = 1
		}, 0, 1},
		{"root_step_limit", func(r *Run) { r.Invocations[r.RootInvocationID].StepInstances = 2 }, 0, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := invocationPlanFixture(t)
			test.change(&r)
			before, err := canonicalState(r)
			if err != nil {
				t.Fatal(err)
			}
			rejectionCode(t, r.chargeInvocation("invocation:child", test.controls, test.steps), "budget_exhausted")
			after, err := canonicalState(r)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("rejected charge partially changed the Run", err)
			}
		})
	}
	t.Run("counter_mismatch_is_not_repaired", func(t *testing.T) {
		r := invocationPlanFixture(t)
		r.ControlTransitions++
		before, _ := canonicalState(r)
		if err := r.chargeInvocation("invocation:child", 1, 0); err == nil || !strings.Contains(err.Error(), "counters disagree") {
			t.Fatal("inconsistent counters were accepted", err)
		}
		after, _ := canonicalState(r)
		if !bytes.Equal(before, after) {
			t.Fatal("inconsistent counter was repaired")
		}
	})
}

func TestInvocationBudgetRejectsChangedPinnedPath(t *testing.T) {
	for name, change := range map[string]func(*testing.T, *Run){
		"changed_limits_same_reference": func(t *testing.T, r *Run) {
			ref := r.Invocations["invocation:child"].WorkflowRef
			for i := range r.Definitions {
				if r.Definitions[i].Ref != ref {
					continue
				}
				var workflow flow.WorkflowRevision
				if err := json.Unmarshal(r.Definitions[i].Bytes, &workflow); err != nil {
					t.Fatal(err)
				}
				workflow.Limits.MaxControlTransitions = 100
				data, err := canonical(workflow)
				if err != nil {
					t.Fatal(err)
				}
				r.Definitions[i].Bytes = data
				return
			}
			t.Fatal("fixture has no pinned child definition")
		},
		"different_workflow_reference": func(_ *testing.T, r *Run) { r.Invocations["invocation:child"].WorkflowRef = r.WorkflowRef },
		"different_caller_stage":       func(_ *testing.T, r *Run) { r.Activations["activation:enter"].StageID = "done" },
		"different_caller_owner":       func(_ *testing.T, r *Run) { r.Activations["activation:enter"].InvocationID = "invocation:child" },
		"missing_child_definition": func(_ *testing.T, r *Run) {
			ref := r.Invocations["invocation:child"].WorkflowRef
			for i, definition := range r.Definitions {
				if definition.Ref == ref {
					r.Definitions = append(r.Definitions[:i], r.Definitions[i+1:]...)
					return
				}
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := invocationPlanFixture(t)
			change(t, &r)
			before, err := canonicalState(r)
			if err != nil {
				t.Fatal(err)
			}
			if err := r.chargeInvocation("invocation:child", 1, 0); err == nil {
				t.Fatal("untrusted budget path granted a charge")
			}
			after, err := canonicalState(r)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("failed identity check changed counters", err)
			}
		})
	}
	r := invocationPlanFixture(t)
	for _, field := range []string{"id", "version"} {
		ref := r.WorkflowRef
		if field == "id" {
			ref.ID = "test:workflow/other"
		} else {
			ref.Version = "2.0.0"
		}
		if _, err := pinnedBudgetWorkflow(r.Workflow, ref); err == nil {
			t.Fatal("definition bytes were accepted under another identity", field)
		}
	}
}

func TestInvocationBudgetKeepsLegacyAccountingShape(t *testing.T) {
	r := invocationPlanFixture(t)
	child := r.Invocations["invocation:child"]
	data := r.registry()[child.WorkflowRef]
	r.SchemaVersion, r.Profile = StateVersion, flow.Profile
	r.Workflow, r.WorkflowRef = data, child.WorkflowRef
	r.Invocations, r.WorkflowConfigurations, r.EffectiveConfiguration = nil, nil, nil
	r.ControlTransitions, r.Ready = 1, []string{"done"}
	if err := r.chargeInvocation(r.RootInvocationID, 1, 1); err != nil {
		t.Fatal(err)
	}
	if r.ControlTransitions != 2 || r.Invocations != nil || r.WorkflowConfigurations != nil || len(r.Steps) != 0 || !reflect.DeepEqual(r.Ready, []string{"done"}) {
		t.Fatal("legacy budget charge introduced a tree, frontier change or fictional Step")
	}
	before, _ := canonicalState(r)
	if err := r.chargeInvocation("invocation:other", 1, 0); err == nil {
		t.Fatal("legacy child received an independent budget")
	}
	after, _ := canonicalState(r)
	if !bytes.Equal(before, after) {
		t.Fatal("unknown legacy budget scope changed state")
	}
}

func TestInvocationPublicSchemaIsSeparate(t *testing.T) {
	const releasedChoice = "sha256:7fcacd3aa4719606b3f7ec0d1395b20feabdd393f22f013e48178a13f38f9cd8"
	const releasedInvocation = "sha256:ff73ea6801148b60e077b20093b904b465ca298c3b233c106375ae2194654864"
	if rawDigest(choiceContracts) != releasedChoice {
		t.Fatal("call changed the delivered choice-decision/1 schema")
	}
	if rawDigest(invocationPublicContracts) != releasedInvocation {
		t.Fatal("repeat changed the delivered invocation version 2 schema")
	}
	distributed, err := os.ReadFile("../../schemas/core/invocations.schema.json")
	if err != nil || !bytes.Equal(distributed, invocationPublicContracts) {
		t.Fatal("invocation embedded/distributed schemas differ", err)
	}
	r := invocationScopesFixture()
	for _, id := range []string{r.RootInvocationID, "invocation:first"} {
		if err := validatePublic(t, "CoreWorkflowInvocation", r.Invocations[id]); err != nil {
			t.Fatal(err)
		}
	}
	inv := r.Invocations["invocation:first"]
	partial := "partial"
	inv.Status, inv.Outcome = "completed", &partial
	inv.Ready, inv.Settled = []string{}, &inv.Created
	if err := validatePublic(t, "CoreWorkflowInvocation", inv); err != nil {
		t.Fatal("declared child partial outcome is not representable", err)
	}
	encoded, _ := json.Marshal(inv)
	for name, change := range map[string]func(map[string]any){
		"unknown_field":               func(v map[string]any) { v["override"] = true },
		"unpaired_parent":             func(v map[string]any) { delete(v, "caller_stage_activation_id") },
		"unpaired_caller":             func(v map[string]any) { delete(v, "parent_invocation_id") },
		"null_inputs":                 func(v map[string]any) { v["input_refs"] = nil },
		"negative_counter":            func(v map[string]any) { v["step_instances"] = -1 },
		"negative_control_counter":    func(v map[string]any) { v["control_transitions"] = -1 },
		"unqualified_ref":             func(v map[string]any) { delete(v["workflow_ref"].(map[string]any), "digest") },
		"invented_outcome":            func(v map[string]any) { v["outcome"] = "pass" },
		"waiver_not_implemented":      func(v map[string]any) { v["outcome"] = "completed_with_waivers" },
		"completed_without_outcome":   func(v map[string]any) { v["outcome"] = nil },
		"failed_with_outcome":         func(v map[string]any) { v["status"] = "failed" },
		"empty_parent_identity":       func(v map[string]any) { v["parent_invocation_id"] = "" },
		"null_frontier":               func(v map[string]any) { v["ready_stages"] = nil },
		"terminal_frontier":           func(v map[string]any) { v["ready_stages"] = []any{"done"} },
		"terminal_without_settlement": func(v map[string]any) { delete(v, "settled") },
		"terminal_null_settlement":    func(v map[string]any) { v["settled"] = nil },
	} {
		t.Run(name, func(t *testing.T) {
			var object map[string]any
			if err := json.Unmarshal(encoded, &object); err != nil {
				t.Fatal(err)
			}
			change(object)
			if validatePublic(t, "CoreWorkflowInvocation", object) == nil {
				t.Fatal("invalid invocation accepted")
			}
		})
	}
	schema, err := PublicSchema("CoreRunStateV2")
	if err != nil || !strings.Contains(string(schema), CoreInvocationStateVersion) {
		t.Fatal("version 2 state contract is not explicitly selectable", err)
	}
}

func TestInvocationNextSchemaRequiresQualifiedWork(t *testing.T) {
	next := NextView{SchemaVersion: CoreInvocationNextVersion, RunID: "run:next", Action: "stage", WorkID: "done", InvocationID: "invocation:child", StageID: "done", ReadOnly: true, SafeNextActions: []string{"run.drive"}}
	if err := validatePublic(t, "CoreNextViewV2", next); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(next)
	for name, change := range map[string]func(map[string]any){
		"missing_scope":              func(v map[string]any) { delete(v, "workflow_invocation_id") },
		"missing_local_stage":        func(v map[string]any) { delete(v, "stage_id") },
		"missing_work":               func(v map[string]any) { v["work_id"] = "" },
		"read_admits_work":           func(v map[string]any) { v["admission"] = true },
		"active_with_stage_frontier": func(v map[string]any) { v["action"] = "active" },
		"terminal_with_work":         func(v map[string]any) { v["action"] = "terminal" },
	} {
		t.Run(name, func(t *testing.T) {
			var value map[string]any
			if err := json.Unmarshal(encoded, &value); err != nil {
				t.Fatal(err)
			}
			change(value)
			if validatePublic(t, "CoreNextViewV2", value) == nil {
				t.Fatal("unqualified or admitting read accepted")
			}
		})
	}
	for _, action := range []string{"active", "cancel", "restricted", "resume_required", "blocked_child", "terminal", "uncertain", "idle"} {
		candidate := next
		candidate.Action, candidate.StageID = action, ""
		if action == "terminal" || action == "uncertain" || action == "idle" {
			candidate.InvocationID, candidate.WorkID = "", ""
		}
		if err := validatePublic(t, "CoreNextViewV2", candidate); err != nil {
			t.Fatalf("%s: %v", action, err)
		}
	}
}

func TestInvocationLocalRegistrySchemaIsExplicitVersionTwo(t *testing.T) {
	registry := RegistryFile{SchemaVersion: "2", Entries: []Definition{}, Aliases: map[string]string{"child": "workflows/child.json"}}
	if err := validatePublic(t, "LocalRegistryV2", registry); err != nil {
		t.Fatal(err)
	}
	registry.SchemaVersion = "1"
	if validatePublic(t, "LocalRegistryV2", registry) == nil {
		t.Fatal("aliases were admitted under registry version 1")
	}
	registry.SchemaVersion = "2"
	for _, name := range []string{"", "a b", "a/b", "a\\b", "a\x00b", "a\nb", strings.Repeat("x", 129)} {
		registry.Aliases = map[string]string{name: "workflows/child.json"}
		if validatePublic(t, "LocalRegistryV2", registry) == nil {
			t.Fatalf("invalid local alias %q accepted", name)
		}
	}
}
