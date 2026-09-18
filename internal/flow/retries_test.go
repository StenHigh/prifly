package flow

import (
	"strings"
	"testing"
)

// A retry budget is the graph author's number, but whether a step may be taken
// again at all is the step author's declaration. A stage that asks to repeat a
// step declared never repeatable is refused at compile rather than accepted
// and quietly ignored, which would read as a budget that exists.
func TestTechnicalRetriesNeedTheStepAuthorsPermission(t *testing.T) {
	workflow, registry := fixture(t)
	var w WorkflowRevision
	if err := decodeValue(workflow, &w); err != nil {
		t.Fatal(err)
	}
	stageID := ""
	for id, stage := range w.Definition.Stages {
		if stage.Kind == "step" {
			stageID = id
		}
	}
	if stageID == "" {
		t.Fatal("the fixture has no step stage to repeat")
	}
	var step StepDefinition
	if err := decodeValue(mustValue(t, registry[w.Definition.Stages[stageID].StepRef]), &step); err != nil {
		t.Fatal(err)
	}
	register := func(step StepDefinition) Ref {
		t.Helper()
		data := encoded(t, step)
		digest, err := Digest(data)
		if err != nil {
			t.Fatal(err)
		}
		ref := Ref{ID: step.ID, Version: step.Version, Digest: digest}
		registry[ref] = data
		return ref
	}
	budgeted := func(class string) (*Plan, error) {
		repeated := step
		repeated.Effects.RetryClass = class
		// A second content under one identity is a conflict of its own, and
		// the fixture binds this step twice.
		repeated.ID = step.ID + "-" + strings.ReplaceAll(class, "_", "-")
		// Raising the document to v5 applies v4's completeness to every step
		// stage, so each one answers for the verdicts it does not route.
		for id, stage := range w.Definition.Stages {
			if stage.Kind != "step" {
				continue
			}
			routed := map[string]string{}
			for verdict, next := range stage.On {
				routed[verdict] = next
			}
			impossible := []string{}
			for _, verdict := range StepVerdicts {
				if _, exists := routed[verdict]; !exists {
					impossible = append(impossible, verdict)
				}
			}
			stage.ImpossibleVerdicts = impossible
			if id == stageID {
				stage.StepRef = register(repeated)
				stage.TechnicalRetries = 2
			}
			w.Definition.Stages[id] = stage
		}
		w.SchemaVersion = WorkflowRevisionRetryVersion
		return CompileProfile(encoded(t, w), "json", registry, CoreProfile)
	}
	for _, class := range []string{"never", "deduplicated", "reconcile_required"} {
		_, err := budgeted(class)
		problem := expectProblem(t, err, "unsupported_retries")
		if !strings.Contains(problem.Message, class) {
			t.Fatalf("the refusal does not name what the step declared: %s", problem.Message)
		}
	}
	for _, class := range []string{"pure", "idempotent"} {
		if _, err := budgeted(class); err != nil {
			t.Fatalf("a step whose author allows a repeat was refused a budget (%s): %v", class, err)
		}
	}
}

func mustValue(t *testing.T, data []byte) any {
	t.Helper()
	value, err := Parse(data, "json")
	if err != nil {
		t.Fatal(err)
	}
	return value
}
