package flow

import (
	"encoding/json"
	"strings"
	"testing"
)

// parallelFixture builds a parent whose one stage fans out to two copies of a
// child workflow, on the shared fixture registry so its policy and schemas are
// the real pinned ones. Each case bends exactly one property, so a refusal is
// attributable to that property alone.
func parallelFixture(t *testing.T, bend func(map[string]any)) (map[string]any, Registry) {
	t.Helper()
	child, registry := callWorkflow(t, "test:workflow/branch")
	child.AllowedOutcomes = []string{"succeeded", "rejected"}
	child.Definition.Stages = map[string]Stage{"done": {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]Binding{}}}
	child.Limits = Limits{MaxStepInstances: 1, MaxControlTransitions: 8, MaxParallelism: 1, MaxChildDepth: 0}
	ref := registerCallWorkflow(t, registry, child)

	parentRevision, _ := callWorkflow(t, "test:workflow/fanout")
	parentRevision.AllowedOutcomes = []string{"succeeded"}
	parentRevision.Limits = Limits{MaxStepInstances: 4, MaxControlTransitions: 32, MaxParallelism: 1, MaxChildDepth: 1}
	parentRevision.Definition.Entry = "fan"
	parentRevision.Definition.Stages = map[string]Stage{
		"done": {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]Binding{}},
	}
	encodedParent := encoded(t, parentRevision)
	var parent map[string]any
	if err := json.Unmarshal(encodedParent, &parent); err != nil {
		t.Fatal(err)
	}
	branch := func(id string) map[string]any {
		return map[string]any{"id": id, "workflow_ref": map[string]any{"id": ref.ID, "version": ref.Version, "digest": ref.Digest}, "input_bindings": map[string]any{}}
	}
	parent["definition"].(map[string]any)["stages"].(map[string]any)["fan"] = map[string]any{
		"kind": "parallel", "max_parallelism": 1,
		"branches": []any{branch("left"), branch("right")},
		"join":     map[string]any{"mode": "all", "accept_outcomes": []any{"succeeded"}, "selection": "all", "remainder": "wait"},
		"on":       map[string]any{"satisfied": "done", "unsatisfied": "done"},
	}
	if bend != nil {
		bend(parent)
	}
	return parent, registry
}

func compileParallel(t *testing.T, parent map[string]any, registry Registry) (*Plan, error) {
	t.Helper()
	data, err := json.Marshal(parent)
	if err != nil {
		t.Fatal(err)
	}
	return CompileProfile(data, "json", registry, CoreProfile)
}

func TestParallelCompilesItsBranchesAsOrdinaryChildren(t *testing.T) {
	parent, registry := parallelFixture(t, nil)
	plan, err := compileParallel(t, parent, registry)
	if err != nil {
		t.Fatal(err)
	}
	branches := plan.Branches["fan"]
	if len(branches) != 2 || branches["left"] == nil || branches["right"] == nil {
		t.Fatalf("branches were not compiled as child plans: %+v", branches)
	}
	if branches["left"].Workflow.ID != "test:workflow/branch" {
		t.Fatalf("a branch resolved to the wrong workflow: %s", branches["left"].Workflow.ID)
	}
	stage := plan.Workflow.Definition.Stages["fan"]
	if len(stage.ParallelBranches) != 2 || stage.Join == nil || stage.Join.Mode != "all" || stage.MaxParallelism != 1 {
		t.Fatalf("the join contract did not survive decoding: %+v", stage)
	}
	// The same name carries a different shape for choice; neither may leak.
	if stage.Branches != nil {
		t.Fatalf("parallel branches were decoded as choice branches: %+v", stage.Branches)
	}
}

func TestParallelBranchMayReadPrecedingStageOutput(t *testing.T) {
	root, child, registry := callBindingFixture(t)
	childRef := registerCallWorkflow(t, registry, child)
	root.Outputs = map[string]OutputPort{}
	root.Definition.Entry = "produce"
	root.Definition.Stages = map[string]Stage{
		"produce": {Kind: "call", WorkflowRef: childRef, InputBindings: map[string]Binding{"value": {From: "workflow_input", Port: "value"}}, On: map[string]string{"no_work": "fan"}},
		"fan": {
			Kind: "parallel", MaxParallelism: 1,
			ParallelBranches: []ParallelBranch{
				{ID: "left", WorkflowRef: childRef, InputBindings: map[string]Binding{"value": {From: "stage_output", StageID: "produce", Port: "report"}}},
				{ID: "right", WorkflowRef: childRef, InputBindings: map[string]Binding{"value": {From: "stage_output", StageID: "produce", Port: "report"}}},
			},
			Join: &Join{Mode: "all", AcceptOutcomes: []string{"no_work"}, Selection: "all", Remainder: "wait"},
			On:   map[string]string{"satisfied": "done", "unsatisfied": "done"},
		},
		"done": {Kind: "finish", Outcome: "no_work", OutputBindings: map[string]Binding{}},
	}
	if _, err := CompileProfile(encoded(t, root), "json", registry, CoreProfile); err != nil {
		t.Fatalf("parallel branches could not read a preceding output: %v", err)
	}
}

func TestParallelIsRefusedByTheFoundationProfile(t *testing.T) {
	parent, registry := parallelFixture(t, nil)
	data, err := json.Marshal(parent)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CompileProfile(data, "json", registry, Profile); err == nil {
		t.Fatal("the foundation profile compiled a parallel stage")
	}
}

func TestParallelJoinContractIsChecked(t *testing.T) {
	for _, bend := range []struct {
		name string
		fn   func(map[string]any)
	}{
		{"duplicate branch id", func(parent map[string]any) {
			stage := parent["definition"].(map[string]any)["stages"].(map[string]any)["fan"].(map[string]any)
			branches := stage["branches"].([]any)
			branches[1].(map[string]any)["id"] = "left"
		}},
		{"quorum larger than the branch count", func(parent map[string]any) {
			stage := parent["definition"].(map[string]any)["stages"].(map[string]any)["fan"].(map[string]any)
			stage["join"] = map[string]any{"mode": "quorum", "accept_outcomes": []any{"succeeded"}, "required_successes": 3, "selection": "first_observed", "remainder": "cancel"}
		}},
		{"accepted outcome no branch declares", func(parent map[string]any) {
			stage := parent["definition"].(map[string]any)["stages"].(map[string]any)["fan"].(map[string]any)
			stage["join"].(map[string]any)["accept_outcomes"] = []any{"partial"}
		}},
		{"route that is not a join result", func(parent map[string]any) {
			stage := parent["definition"].(map[string]any)["stages"].(map[string]any)["fan"].(map[string]any)
			stage["on"] = map[string]any{"succeeded": "done"}
		}},
	} {
		t.Run(bend.name, func(t *testing.T) {
			parent, registry := parallelFixture(t, bend.fn)
			if _, err := compileParallel(t, parent, registry); err == nil {
				t.Fatalf("a parallel stage with %s compiled", bend.name)
			}
		})
	}
}

// A parallel stage produces exactly one thing: the summary of its branches.
// A reference to any other port fails by name rather than resolving to nothing
// at run time.
func TestParallelStageProducesOnlyItsSummary(t *testing.T) {
	plan := &Plan{Profile: CoreProfile, Registry: Registry{Ref{ID: AggregateSchemaID, Version: "1.0.0", Digest: "sha256:0"}: []byte(`true`)}}
	plan.Workflow.Definition.Stages = map[string]Stage{
		"fan":  {Kind: "parallel"},
		"done": {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]Binding{"report": {From: "stage_output", StageID: "fan", Port: "report"}}},
	}
	err := plan.checkParallelOutputReferences()
	if err == nil {
		t.Fatal("a port the stage cannot produce was accepted")
	}
	if !strings.Contains(err.Error(), AggregateResultsPort) {
		t.Fatalf("the refusal did not name what the stage produces: %v", err)
	}
	// The summary itself is accepted.
	plan.Workflow.Definition.Stages["done"] = Stage{Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]Binding{"report": {From: "stage_output", StageID: "fan", Port: AggregateResultsPort}}}
	if err := plan.checkParallelOutputReferences(); err != nil {
		t.Fatalf("the stage's own summary was refused: %v", err)
	}
	// Without the shipped form the build cannot describe what it produces.
	plan.Registry = Registry{}
	if err := plan.checkParallelOutputReferences(); err == nil {
		t.Fatal("a summary was promised without the form that describes it")
	}
	// A reference to an ordinary stage is untouched by the rule.
	plan.Workflow.Definition.Stages["fan"] = Stage{Kind: "call"}
	if err := plan.checkParallelOutputReferences(); err != nil {
		t.Fatalf("the rule refused a reference to an ordinary stage: %v", err)
	}
}

// Simultaneity is bounded by what this build was qualified for, by what the
// workflow declared, and by the join being one that waits. Each bound is
// refused by its own reason rather than by one blanket refusal.
func TestParallelSimultaneityIsBoundedByItsOwnDeclarations(t *testing.T) {
	for _, c := range []struct {
		name   string
		reason string
		bend   func(map[string]any)
	}{
		{"beyond what this build is qualified for", "qualified for", func(parent map[string]any) {
			parent["limits"].(map[string]any)["max_parallelism"] = MaxQualifiedParallelism + 1
		}},
		{"a stage claiming more than its workflow", "exceed the workflow", func(parent map[string]any) {
			stage := parent["definition"].(map[string]any)["stages"].(map[string]any)["fan"].(map[string]any)
			stage["max_parallelism"] = 2
		}},
		{"a quorum larger than the branches can supply", "quorum cannot exceed", func(parent map[string]any) {
			parent["limits"].(map[string]any)["max_parallelism"] = 2
			stage := parent["definition"].(map[string]any)["stages"].(map[string]any)["fan"].(map[string]any)
			stage["max_parallelism"] = 2
			stage["join"] = map[string]any{"mode": "quorum", "accept_outcomes": []any{"succeeded"}, "required_successes": 5, "selection": "first_observed", "remainder": "cancel"}
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			parent, registry := parallelFixture(t, c.bend)
			_, err := compileParallel(t, parent, registry)
			if err == nil {
				t.Fatalf("%s compiled", c.name)
			}
			if !strings.Contains(err.Error(), c.reason) {
				t.Fatalf("the refusal did not name its reason: %v", err)
			}
		})
	}

	// A quorum that can decide before every branch settles is admitted now that
	// the runtime stops the remainder and waits for that to be confirmed.
	early, registry := parallelFixture(t, func(parent map[string]any) {
		parent["limits"].(map[string]any)["max_parallelism"] = 2
		stage := parent["definition"].(map[string]any)["stages"].(map[string]any)["fan"].(map[string]any)
		stage["max_parallelism"] = 2
		stage["join"] = map[string]any{"mode": "quorum", "accept_outcomes": []any{"succeeded"}, "required_successes": 1, "selection": "first_observed", "remainder": "cancel"}
	})
	if _, err := compileParallel(t, early, registry); err != nil {
		t.Fatalf("a quorum that decides early was refused: %v", err)
	}

	// A workflow and stage that stay within all three bounds compile.
	parent, wide := parallelFixture(t, func(parent map[string]any) {
		parent["limits"].(map[string]any)["max_parallelism"] = 2
		stage := parent["definition"].(map[string]any)["stages"].(map[string]any)["fan"].(map[string]any)
		stage["max_parallelism"] = 2
	})
	if _, err := compileParallel(t, parent, wide); err != nil {
		t.Fatalf("a fan-out within every declared bound was refused: %v", err)
	}
}

// A branch is an ordinary child, so the budget must cover every branch rather
// than the largest one: all of them may run.
func TestParallelChargesEveryBranch(t *testing.T) {
	parent, registry := parallelFixture(t, func(parent map[string]any) {
		parent["limits"] = map[string]any{"max_step_instances": 4, "max_control_transitions": 3, "max_parallelism": 1, "max_child_depth": 1}
	})
	if _, err := compileParallel(t, parent, registry); err == nil {
		t.Fatal("a fan-out compiled under a budget that covers only one branch")
	}
}
