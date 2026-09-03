package runtime

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// WF-AC-09: the second body has completed, but its post-body decision has not
// committed. Recovery must use that retained result and the existing counter.
func TestRepeatAcceptanceCrashAfterSecondCompletedBody(t *testing.T) {
	e, workflow, body, options := repeatFixture(t, "commit-pass", "succeeded", 3)
	body["id"] = "test:workflow/repeat-second-outcome"
	body["allowed_outcomes"] = []string{"succeeded", "rejected"}
	body["outputs"].(map[string]any)["report"].(map[string]any)["required_for"] = []string{"succeeded", "rejected"}
	stages := choiceStages(body)
	stages["work"].(map[string]any)["on"] = map[string]any{"pass": "pick"}
	stages["pick"] = choiceStage("exclusive", choiceBranch("first", choiceFieldEqual("/flag", true), "done"), choiceBranch("second", choiceFieldEqual("/flag", false), "rejected"))
	stages["rejected"] = callClone(t, stages["done"].(map[string]any))
	stages["rejected"].(map[string]any)["outcome"] = "rejected"
	bodyRef := callRegister(t, e, body, "workflows/repeat-second-outcome.json")
	workflow["allowed_outcomes"] = []string{"succeeded", "rejected"}
	workflow["outputs"].(map[string]any)["report"].(map[string]any)["required_for"] = []string{"succeeded", "rejected"}
	stage := choiceStages(workflow)["work"].(map[string]any)
	stage["body_workflow_ref"] = bodyRef
	stage["on_complete"] = map[string]any{"succeeded": "done", "rejected": "rejected"}
	control := body["inputs"].(map[string]any)["control"].(map[string]any)
	stage["next_bindings"].(map[string]any)["control"] = map[string]any{"from": "literal", "schema_ref": control["schema_ref"], "value": map[string]any{"flag": false}}
	choiceStages(workflow)["rejected"] = callClone(t, choiceStages(workflow)["done"].(map[string]any))
	choiceStages(workflow)["rejected"].(map[string]any)["outcome"] = "rejected"
	runID := choiceStart(t, e, workflow, options)
	_, crash := choiceCrashDriver(t, "TestRepeatAcceptanceSecondBodyHelper", "REPEAT_SECOND_BODY_HELPER", e.Root, runID)

	// Two actual workers plus their choices must settle before the marker.
	// Keep the wait bounded without assuming native and race runtimes match.
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(filepath.Join(e.Root, "repeat-second-completed-ready")); err == nil {
			break
		}
		select {
		case <-deadline.C:
			t.Fatal("owned helper did not reach the second completed body")
		case <-ticker.C:
		}
	}
	before := driverRun(t, e, runID)
	a := activationFor(&before, "work")
	bodies, err := before.repeatBodies(a.ID)
	if err != nil || len(bodies) != 2 || a.Repeat.IterationCount != 2 || len(before.Invocations) != 3 || len(before.Attempts) != 2 || len(before.Active) != 0 || before.ControlTransitions != 8 || a.Status != "waiting" || a.Settled != nil {
		t.Fatal("helper did not stop before the second post-body decision", err)
	}
	if root := before.Invocations[before.RootInvocationID]; root == nil || root.StepInstances != 2 || root.ControlTransitions != 8 {
		t.Fatal("root subtree budgets did not include both completed bodies")
	}
	for index, body := range bodies {
		wantOutcome := []string{"succeeded", "rejected"}[index]
		if body.Status != "completed" || body.Settled == nil || body.Outcome == nil || *body.Outcome != wantOutcome || body.StepInstances != 1 || body.ControlTransitions != 3 {
			t.Fatalf("body %d is not an accepted, fully settled outcome: %+v", index+1, body)
		}
		_, input, err := e.Artifact(body.Inputs["control"])
		if err != nil || string(input) != []string{`{"flag":true}`, `{"flag":false}`}[index] {
			t.Fatal("body did not receive its distinct initial/next input", err)
		}
	}
	for _, attempt := range before.Attempts {
		if attempt.Accepted == nil || attempt.Settled == nil || attempt.Started == nil || attempt.ProcessOutcome == nil || !attempt.ProcessOutcome.WaitReturned || !attempt.ProcessOutcome.GroupEmpty {
			t.Fatal("crash boundary lacks two actual accepted and settled workers")
		}
	}
	old, oldDecisions := repeatHistory(t, e, runID)
	if len(oldDecisions) != 1 || oldDecisions[0].Iteration != 1 || oldDecisions[0].Route != "continue" || a.Repeat.LastDecision == nil || a.Repeat.LastDecision.Iteration != 1 || driverObservedStarts(t, e) != 2 {
		t.Fatal("second decision was committed before the crash boundary")
	}
	crash() // Asserted SIGKILL of this Cmd.Start-owned helper, never a saved PID.
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(e.Root, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if err := reopened.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	after := driverRun(t, reopened, runID)
	read, decisions := repeatHistory(t, reopened, runID)
	if after.Status != "completed" || after.Outcome == nil || *after.Outcome != "rejected" || len(after.Invocations) != 3 || len(after.Steps) != 2 || len(after.Attempts) != 2 || after.ControlTransitions != 10 || after.Activations[a.ID].Repeat.IterationCount != 2 || driverObservedStarts(t, reopened) != 2 {
		t.Fatal("recovery reset the repeat counter, repeated work or admitted body three")
	}
	if root := after.Invocations[after.RootInvocationID]; root == nil || root.StepInstances != 2 || root.ControlTransitions != 10 {
		t.Fatal("recovery reset a shared root subtree budget")
	}
	if len(decisions) != 2 {
		t.Fatal("recovery did not commit exactly the second decision")
	}
	last := decisions[1]
	if last.Iteration != 2 || last.BodyInvocationID != bodies[1].ID || last.BodyOutcome == nil || *last.BodyOutcome != "rejected" || last.Route != "on_complete" || last.NextStageID != "rejected" || last.NextBodyInvocationID != "" || last.UntilResult != "not_evaluated" || len(last.Inputs) != 0 || after.Outputs["report"] != bodies[1].Outputs["report"] {
		t.Fatal("recovery ignored the retained non-continuing outcome", last)
	}
	if !reflect.DeepEqual(before.Steps, after.Steps) || !reflect.DeepEqual(before.Attempts, after.Attempts) {
		t.Fatal("recovery reset a completed StepInstance or Attempt")
	}
	for _, body := range bodies {
		if !reflect.DeepEqual(body, after.Invocations[body.ID]) {
			t.Fatal("recovery rewrote completed body state", body.ID)
		}
	}
	if len(read.Events) < len(old.Events) || !reflect.DeepEqual(old.Events, read.Events[:len(old.Events)]) || !reflect.DeepEqual(oldDecisions, decisions[:len(oldDecisions)]) {
		t.Fatal("recovery rewrote the accepted event or decision prefix")
	}
	if err := reopened.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	again, _ := repeatHistory(t, reopened, runID)
	repeatSameProjection(t, read, again)
	if driverObservedStarts(t, reopened) != 2 {
		t.Fatal("terminal retry started another worker")
	}
	if err := reopened.Store.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRepeatAcceptanceSecondBodyHelper(t *testing.T) {
	if os.Getenv("REPEAT_SECOND_BODY_HELPER") != "1" {
		return
	}
	root, runID := os.Args[len(os.Args)-2], os.Args[len(os.Args)-1]
	e, err := Open(root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	repeatEnter(t, e, runID)
	r, view, p, a := repeatFinishBody(t, e, runID)
	commandID := newID("command")
	decision, inputs := repeatContinueInputs(t, e, r, p, a, commandID)
	if _, err := e.commitRepeat(context.Background(), r, view, p, a, commandID, decision, inputs); err != nil {
		t.Fatal(err)
	}
	repeatFinishBody(t, e, runID)
	lock, err := e.driverLock(runID)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := os.WriteFile(filepath.Join(root, "repeat-second-completed-ready"), []byte("body two completed; decision pending"), 0600); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Second)
	t.Fatal("parent did not kill the owned helper at the second completed body")
}
