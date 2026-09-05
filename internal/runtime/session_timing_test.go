package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stenhigh/prifly/internal/flow"
)

// Change registry definitions before Start, never a pinned Run or deadline.
func timedFixture(t *testing.T, limits flow.SessionLimits, effect string) (*Engine, string, StartOptions) {
	t.Helper()
	definition := DecisionDefinition{SchemaVersion: DecisionDefinitionVersion, ID: "continue", Title: "Continue", Phase: "runtime", Choices: []DecisionChoice{{ID: "yes", Title: "Yes", Value: json.RawMessage(`true`)}}, Sensitivity: "ordinary", Destination: DecisionDestination{Kind: "session_context", Name: "continue"}}
	catalog := DecisionCatalog{SchemaVersion: DecisionCatalogVersion, Decisions: []DecisionDefinition{definition}}
	digest, err := DecisionCatalogDigest(catalog)
	if err != nil {
		t.Fatal(err)
	}
	sheet := DecisionSheet{SchemaVersion: DecisionSheetVersion, CatalogDigest: digest, ProfileSource: "none", Records: []DecisionRecord{}}
	e, _, _ := assistedWorkspaceFixtureWithDecisions(t, "checkout", &catalog, &sheet)
	var step flow.StepDefinition
	data, err := os.ReadFile(filepath.Join(e.Root, "steps/plan.json"))
	if err != nil || json.Unmarshal(data, &step) != nil {
		t.Fatal(err)
	}
	step.SchemaVersion, step.SessionLimits, step.Effects.Class = "6", &limits, effect
	step.Version = "2.0.0"
	data = writeRegistryDocument(t, e, "steps/plan.json", step)
	ref := flow.Ref{ID: step.ID, Version: step.Version, Digest: rawDigest(data)}
	var registry RegistryFile
	data, err = os.ReadFile(filepath.Join(e.Root, e.Config.Configuration.RegistryFile))
	if err != nil || json.Unmarshal(data, &registry) != nil {
		t.Fatal(err)
	}
	for index := range registry.Entries {
		if registry.Entries[index].Kind == "step" {
			registry.Entries[index].Ref = ref
		}
	}
	writeRuntimeJSON(t, filepath.Join(e.Root, e.Config.Configuration.RegistryFile), registry)
	var workflow flow.WorkflowRevision
	data, err = os.ReadFile(filepath.Join(e.Root, "workflows/pilot.json"))
	if err != nil || json.Unmarshal(data, &workflow) != nil {
		t.Fatal(err)
	}
	stage := workflow.Definition.Stages["plan"]
	workflow.Version = "2.0.0"
	stage.StepRef = ref
	workflow.Definition.Stages["plan"] = stage
	writeRuntimeJSON(t, filepath.Join(e.Root, "workflows/pilot.json"), workflow)
	options := StartOptions{SchemaVersion: "2", CommandID: newID("command"), WorkflowFile: "workflows/pilot.json", DecisionCatalog: &catalog, DecisionSheet: &sheet}
	if effect == "workspace_write" {
		options.WorkspaceMode = "checkout"
	}
	started, err := e.Start(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	return e, started.Receipt.RunID, options
}

func parkTimed(t *testing.T, e *Engine, task SessionTask) DecisionRequest {
	t.Helper()
	run, view, err := e.load(context.Background(), task.RunID)
	if err != nil {
		t.Fatal(err)
	}
	definition := run.DecisionCatalog.Decisions[0]
	digest, err := DecisionDefinitionDigest(definition)
	if err != nil {
		t.Fatal(err)
	}
	request := DecisionRequest{SchemaVersion: DecisionRequestTimingVersion, RunID: task.RunID, AttemptID: task.AttemptID, EnvelopeDigest: task.EnvelopeDigest, DecisionID: definition.ID, DefinitionDigest: digest, ExpectedRunVersion: view.Snapshot.Version, YieldExecution: true}
	if _, err := e.RequestDecision(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	return request
}

func timedAnswer(t *testing.T, e *Engine, request DecisionRequest) DecisionAnswer {
	t.Helper()
	_, view, err := e.load(context.Background(), request.RunID)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := DecisionRequestDigest(request)
	if err != nil {
		t.Fatal(err)
	}
	return DecisionAnswer{SchemaVersion: DecisionAnswerVersion, RunID: request.RunID, DecisionID: request.DecisionID, DefinitionDigest: request.DefinitionDigest, RequestDigest: digest, ExpectedRunVersion: view.Snapshot.Version, Value: json.RawMessage(`true`)}
}

func TestTimedSessionWaitReopenAndCapacityKeepRemainingAllowance(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		e, runID, options := timedFixture(t, flow.SessionLimits{ActiveTimeoutMS: time.Hour.Milliseconds()}, "none")
		task := handOver(t, e, runID)
		before := driverRun(t, e, runID)
		envelope := bytes.Clone(before.Attempts[task.AttemptID].Envelope)
		if task.Delivery == nil || task.Delivery.Generation != 1 || task.SchemaVersion != AssistedSessionTimingVersion {
			t.Fatalf("initial timed delivery: %+v", task)
		}
		time.Sleep(10 * time.Minute)
		request := parkTimed(t, e, task)
		parked := driverRun(t, e, runID)
		if parked.Attempts[task.AttemptID].Session.Timing.RemainingMS != (50*time.Minute).Milliseconds() || parked.executingAttempts() != 0 {
			t.Fatal("parking lost or refilled allowance")
		}
		if _, err := e.SessionTask(ctx, runID, task.AttemptID); refusalCode(err) != "no_active_handoff" {
			t.Fatal(err)
		}
		if _, err := e.SubmitSession(ctx, hostResult(t, e, task, "stale parked result")); refusalCode(err) != "session_state_conflict" {
			t.Fatalf("parked result: %v", err)
		}
		options.CommandID = newID("command")
		second, err := e.Start(ctx, options)
		if err != nil {
			t.Fatal(err)
		}
		other := handOver(t, e, second.Receipt.RunID)
		// B owns capacity while A's answer is accepted, but cannot grant A a delivery.
		answer := timedAnswer(t, e, request)
		if _, err := e.AnswerDecision(ctx, answer); err != nil {
			t.Fatal(err)
		}
		if err := e.Drive(ctx, runID); refusalCode(err) != "capacity_exhausted" && refusalCode(err) != "capacity_conflict" && refusalCode(err) != "admission_not_turn" {
			t.Fatalf("occupied slot: %v", err)
		}
		if _, err := e.SubmitSession(ctx, hostResult(t, e, other, "other Run completed")); err != nil {
			t.Fatal(err)
		}
		if err := e.Drive(ctx, other.RunID); err != nil {
			t.Fatal(err)
		}
		root := e.Root
		if err := e.Close(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(14 * 24 * time.Hour)
		e, err = Open(root, false)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = e.Close() })
		waiting := driverRun(t, e, runID)
		if waiting.Attempts[task.AttemptID].Session.Timing.RemainingMS != (50*time.Minute).Milliseconds() || waiting.PendingDecision != nil {
			t.Fatal("waiting admission lost answer or time")
		}
		if err := e.Drive(ctx, runID); err != nil {
			t.Fatal(err)
		}
		resumed, err := e.SessionTask(ctx, runID, task.AttemptID)
		if err != nil {
			t.Fatal(err)
		}
		if resumed.AttemptID != task.AttemptID || resumed.EnvelopeDigest == task.EnvelopeDigest || resumed.Delivery.Generation != 2 || resumed.Delivery.Timing.RemainingMS != (50*time.Minute).Milliseconds() {
			t.Fatalf("resumed delivery: %+v", resumed)
		}
		if string(resumed.DecisionContext["continue"]) != "true" {
			t.Fatal("answer missing")
		}
		current := driverRun(t, e, runID)
		if !bytes.Equal(envelope, current.Attempts[task.AttemptID].Envelope) || len(current.Attempts) != 1 {
			t.Fatal("resume replaced pinned work")
		}
		for name, value := range map[string]any{"CoreRunStateV27": current, "SessionTaskV6": resumed} {
			if err := validatePublic(t, name, value); err != nil {
				t.Fatal(name, err)
			}
		}
		time.Sleep(49 * time.Minute)
		if _, err := e.SubmitSession(ctx, hostResult(t, e, resumed, "same work completed after two weeks")); err != nil {
			t.Fatal(err)
		}
		if err := e.Drive(ctx, runID); err != nil {
			t.Fatal(err)
		}
		final := driverRun(t, e, runID)
		if final.Status != "completed" || final.Attempts[task.AttemptID].ProcessOutcome != nil {
			t.Fatalf("final: %s", final.Status)
		}
	})
}

func TestTimedSessionTwoWeekHumanWaitPreservesExactOutputAndClaimBoundary(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, effect := range []string{"none", "workspace_write"} {
		t.Run(effect, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx := context.Background()
				e, runID, _ := timedFixture(t, flow.SessionLimits{ActiveTimeoutMS: 3600000}, effect)
				task := handOver(t, e, runID)
				original := driverRun(t, e, runID)
				envelope := bytes.Clone(original.Attempts[task.AttemptID].Envelope)
				time.Sleep(10 * time.Minute)
				request := parkTimed(t, e, task)
				root := e.Root
				if err := e.Close(); err != nil {
					t.Fatal(err)
				}
				time.Sleep(14 * 24 * time.Hour)
				var err error
				e, err = Open(root, false)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = e.Close() })
				next, err := e.Next(ctx, runID)
				if err != nil || next.Action != "waiting_decision" {
					t.Fatalf("two-week question: %+v %v", next, err)
				}
				if _, err := e.AnswerDecision(ctx, timedAnswer(t, e, request)); err != nil {
					t.Fatal(err)
				}
				err = e.Drive(ctx, runID)
				if effect == "workspace_write" {
					if refusalCode(err) != "claim_owner_unproven" {
						t.Fatalf("expired claim renewed through an answer: %v", err)
					}
					current := driverRun(t, e, runID)
					claim, err := e.claim(ctx, task.ClaimID)
					if err != nil || claim.RunID != runID || current.PendingDecision != nil || current.Attempts[task.AttemptID].Session.HostState != SessionWaitingAdmission {
						t.Fatal("claim or accepted answer was lost", err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				resumed, err := e.SessionTask(ctx, runID, task.AttemptID)
				if err != nil {
					t.Fatal(err)
				}
				if resumed.Delivery.Timing.RemainingMS != 3000000 || resumed.Delivery.Generation != 2 || resumed.AttemptID != task.AttemptID {
					t.Fatal("human wait changed the remaining fifty minutes")
				}
				deadline, err := time.Parse(time.RFC3339Nano, resumed.Deadline)
				if err != nil || deadline.Sub(time.Now()) != 50*time.Minute {
					t.Fatal("actual resumed deadline differs from allowance", err)
				}
				time.Sleep(49 * time.Minute)
				if _, err := e.SubmitSession(ctx, hostResult(t, e, resumed, "two-week output")); err != nil {
					t.Fatal(err)
				}
				if err := e.Drive(ctx, runID); err != nil {
					t.Fatal(err)
				}
				final := driverRun(t, e, runID)
				a := final.Attempts[task.AttemptID]
				if final.Status != "completed" || len(final.Attempts) != 1 || !bytes.Equal(a.Envelope, envelope) || a.ProcessOutcome != nil {
					t.Fatal("original work or honest outcome was lost")
				}
				ref := final.Steps[a.StepID].Outputs["plan"]
				_, body, err := e.Artifact(ref)
				if err != nil || string(body) != `{"summary":"two-week output"}` {
					t.Fatalf("exact output: %s %v", body, err)
				}
				if err := e.Store.Verify(ctx); err != nil {
					t.Fatal(err)
				}
			})
		})
	}
}

func TestTimedSessionExpiryAndCancellation(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, name := range []string{"active expiry", "wait expiry", "cancel waiting", "cancel active", "unknown workspace"} {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx := context.Background()
				wait := time.Minute.Milliseconds()
				limits := flow.SessionLimits{ActiveTimeoutMS: time.Hour.Milliseconds(), DecisionWaitTimeoutMS: &wait}
				effect := "none"
				if name == "unknown workspace" {
					effect = "workspace_write"
				}
				e, runID, _ := timedFixture(t, limits, effect)
				task := handOver(t, e, runID)
				var request *DecisionRequest
				if name != "active expiry" && name != "cancel active" {
					value := parkTimed(t, e, task)
					request = &value
				}
				cancel := name == "cancel waiting" || name == "cancel active"
				if cancel {
					if _, err := e.Restrict(ctx, RestrictCommand{SchemaVersion: "1", CommandID: newID("command"), Scope: "run", ScopeID: runID, Kind: "cancel", Reason: "owner cancelled"}); err != nil {
						t.Fatal(err)
					}
				} else {
					if name == "active expiry" {
						time.Sleep(time.Hour)
					} else {
						time.Sleep(time.Minute)
					}
					next, err := e.Next(ctx, runID)
					if err != nil || next.Action != "session_expired" || next.ReasonCode == "" || slices.Contains(next.SafeNextActions, "session.task") {
						t.Fatalf("expiry not visible: %+v %v", next, err)
					}
				}
				if request != nil {
					if _, err := e.AnswerDecision(ctx, timedAnswer(t, e, *request)); err == nil {
						t.Fatal("late answer accepted")
					}
				}
				err := e.Drive(ctx, runID)
				if effect == "none" && err != nil {
					t.Fatal(err)
				}
				final := driverRun(t, e, runID)
				a := final.Attempts[task.AttemptID]
				if a.ProcessOutcome != nil || final.PendingDecision != nil {
					t.Fatal("closure fabricated process outcome or retained question")
				}
				if cancel && final.Status != "cancelled" {
					t.Fatalf("cancellation stalled: %s", final.Status)
				}
				if effect != "none" && (final.Status != "uncertain" || !final.HasUnresolvedEffects || a.Settled != nil) {
					t.Fatal("unknown workspace effect was lost")
				}
				if _, err := e.SubmitSession(ctx, hostResult(t, e, task, "late")); err == nil {
					t.Fatal("closed delivery accepted result")
				}
				if err := validatePublic(t, "CoreRunStateV27", final); err != nil {
					t.Fatal(err)
				}
			})
		})
	}
}

func TestSessionTimingRejectsClockRollbackWithoutRefill(t *testing.T) {
	before := Observation{UTC: "2026-09-06T12:00:00Z"}
	a := &Attempt{Admitted: before, Deadline: Observation{UTC: "2026-09-06T13:00:00Z"}, Session: &SessionHandoff{SchemaVersion: AssistedSessionTimingVersion, Timing: &SessionTiming{Limits: flow.SessionLimits{ActiveTimeoutMS: 3600000}, RemainingMS: 3000000, Observed: before}}}
	r := Run{LastObserved: before}
	if err := consumeSessionTime(r, a, Observation{UTC: "2026-09-06T11:59:00Z"}); refusalCode(err) != "deadline_clock_unqualified" {
		t.Fatal(err)
	}
	if err := decisionWaitAdmissible(r, a, Observation{UTC: "2026-09-06T11:59:00Z"}); refusalCode(err) != "deadline_clock_unqualified" {
		t.Fatal(err)
	}
	if a.Session.Timing.RemainingMS != 3000000 {
		t.Fatal("rollback refilled allowance")
	}
}

func TestTimedQuestionDoesNotHideSiblingOrPrematurelyFinishJoin(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	ctx := context.Background()
	e, _, options := timedFixture(t, flow.SessionLimits{ActiveTimeoutMS: 3600000}, "none")
	data, err := os.ReadFile(filepath.Join(e.Root, options.WorkflowFile))
	var child map[string]any
	if err != nil || json.Unmarshal(data, &child) != nil {
		t.Fatal(err)
	}
	child["id"], child["version"] = "test:workflow/timed-child", "1.0.0"
	ref := callRegister(t, e, child, "workflows/timed-child.json")
	parent := callClone(t, child)
	parent["id"] = "test:workflow/timed-parent"
	definitions, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	parent["policy_ref"] = builtinVersionRef(definitions, "core:policy/local", "3.0.0")
	parent["limits"] = map[string]any{"max_step_instances": 4, "max_control_transitions": 64, "max_parallelism": 2, "max_child_depth": 4}
	parent["definition"] = map[string]any{"entry": "fan", "stages": map[string]any{
		"fan": map[string]any{"kind": "parallel", "max_parallelism": 2, "branches": []any{
			map[string]any{"id": "one", "workflow_ref": ref, "input_bindings": map[string]any{}},
			map[string]any{"id": "two", "workflow_ref": ref, "input_bindings": map[string]any{}},
		}, "join": joinAll("succeeded"), "on": map[string]any{"satisfied": "done", "unsatisfied": "done"}},
		"done": choiceFinish("succeeded"),
	}}
	options.WorkflowFile, options.CommandID = "workflows/timed-parent.json", newID("command")
	writeRuntimeJSON(t, filepath.Join(e.Root, options.WorkflowFile), parent)
	started, err := e.Start(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	runID := started.Receipt.RunID
	if err := e.Drive(ctx, runID); refusalCode(err) != "capacity_conflict" {
		t.Fatalf("second branch did not meet the one-slot boundary: %v", err)
	}
	first, err := e.SessionTask(ctx, runID, "")
	if err != nil {
		t.Fatal(err)
	}
	request := parkTimed(t, e, first)
	second := handOver(t, e, runID)
	if first.AttemptID == second.AttemptID {
		t.Fatal("parked delivery exposed as sibling")
	}
	if _, err := e.SubmitSession(ctx, hostResult(t, e, second, "sibling completed")); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatal(err)
	}
	before := driverRun(t, e, runID)
	if before.terminal() || before.PendingDecision == nil || before.Attempts[second.AttemptID].Settled == nil {
		t.Fatal("sibling result hidden or join finished over waiting branch")
	}
	if _, err := e.AnswerDecision(ctx, timedAnswer(t, e, request)); err != nil {
		t.Fatal(err)
	}
	resumed := handOver(t, e, runID)
	if resumed.AttemptID != first.AttemptID {
		t.Fatal("new attempt created for answer")
	}
	if _, err := e.SubmitSession(ctx, hostResult(t, e, resumed, "first completed")); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatal(err)
	}
	final := driverRun(t, e, runID)
	if final.Status != "completed" || len(final.Attempts) != 2 {
		t.Fatalf("join failed: %s", final.Status)
	}
}
