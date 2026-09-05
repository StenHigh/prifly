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
	"github.com/stenhigh/prifly/internal/local"
)

func TestCancelledTimedSessionSettlesSavedReport(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	ctx := context.Background()
	e, runID, _ := timedFixture(t, flow.SessionLimits{ActiveTimeoutMS: time.Hour.Milliseconds()}, "workspace_write")
	task := handOver(t, e, runID)
	report := hostResult(t, e, task, "report persisted before cancellation")
	_, view, err := e.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	// Exercise the durable report/settlement cut without a production crash
	// hook: commit the reported candidate, then leave settlement to recovery.
	_, err = e.apply(ctx, e.owner, newID("command"), runID, "attempt.result_candidate", map[string]string{"attempt_id": task.AttemptID}, &view.Snapshot.Version, local.CommandCAS, func(r *Run, _ local.Snapshot, obs Observation) (local.Change, error) {
		a := r.Attempts[task.AttemptID]
		a.Session.HostState, a.Session.Reported = SessionReported, &obs
		a.Candidate, a.CandidateAt = bytes.Clone(report.Result), &obs
		return local.Change{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Restrict(ctx, RestrictCommand{SchemaVersion: "1", CommandID: newID("command"), Scope: "run", ScopeID: runID, Kind: "cancel", Reason: "owner cancelled after the report was saved"}); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatal(err)
	}
	run := driverRun(t, e, runID)
	a := run.Attempts[task.AttemptID]
	if run.Status != "cancelled" || run.HasUnresolvedEffects || len(run.Active) != 0 || a.Settled == nil || a.Session.HostState != SessionReported || a.Session.Timing.SlotHeld || a.ProcessOutcome != nil || !bytes.Equal(a.Candidate, report.Result) {
		t.Fatal("cancellation lost the saved report or stranded its settlement")
	}
	_, held, err := e.AdmissionCapacity(ctx)
	if err != nil || len(held) != 0 {
		t.Fatalf("saved report retained capacity after cancellation: %+v %v", held, err)
	}
	if err := e.Store.Verify(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestMixedTimedRunKeepsLegacyDecisionVisible(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	ctx := context.Background()
	e, _, options := timedFixture(t, flow.SessionLimits{ActiveTimeoutMS: time.Hour.Milliseconds()}, "none")
	data, err := os.ReadFile(filepath.Join(e.Root, "steps/plan.json"))
	var legacy flow.StepDefinition
	if err != nil || json.Unmarshal(data, &legacy) != nil {
		t.Fatal("read step fixture", err)
	}
	legacy.ID, legacy.SchemaVersion, legacy.SessionLimits = "test:step/legacy-plan", "2", nil
	data = writeRegistryDocument(t, e, "steps/legacy-plan.json", legacy)
	ref := flow.Ref{ID: legacy.ID, Version: legacy.Version, Digest: rawDigest(data)}
	var registry RegistryFile
	data, err = os.ReadFile(filepath.Join(e.Root, e.Config.Configuration.RegistryFile))
	if err != nil || json.Unmarshal(data, &registry) != nil {
		t.Fatal("read registry fixture", err)
	}
	registry.Entries = append(registry.Entries, Definition{Ref: ref, Kind: "step", Path: "steps/legacy-plan.json"})
	writeRuntimeJSON(t, filepath.Join(e.Root, e.Config.Configuration.RegistryFile), registry)
	var workflow flow.WorkflowRevision
	data, err = os.ReadFile(filepath.Join(e.Root, options.WorkflowFile))
	if err != nil || json.Unmarshal(data, &workflow) != nil {
		t.Fatal("read workflow fixture", err)
	}
	workflow.ID, workflow.Definition.Entry = "test:workflow/mixed-decision", "legacy"
	workflow.Definition.Stages["legacy"] = flow.Stage{Kind: "step", StepRef: ref, InputBindings: map[string]flow.Binding{}, On: map[string]string{"pass": "plan"}}
	options.WorkflowFile, options.CommandID = "workflows/mixed-decision.json", newID("command")
	writeRuntimeJSON(t, filepath.Join(e.Root, options.WorkflowFile), workflow)
	started, err := e.Start(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	runID := started.Receipt.RunID
	task := handOver(t, e, runID)
	before := driverRun(t, e, runID)
	if before.SchemaVersion != CoreTimingStateVersion || task.SchemaVersion != AssistedSessionDecisionVersion || task.Delivery != nil {
		t.Fatal("mixed fixture did not retain the legacy session edition")
	}
	definition := before.DecisionCatalog.Decisions[0]
	digest, err := DecisionDefinitionDigest(definition)
	if err != nil {
		t.Fatal(err)
	}
	request := DecisionRequest{SchemaVersion: DecisionRequestVersion, RunID: runID, AttemptID: task.AttemptID, EnvelopeDigest: task.EnvelopeDigest, DecisionID: definition.ID, DefinitionDigest: digest, ExpectedRunVersion: task.RunVersion}
	if _, err := e.RequestDecision(ctx, request); err != nil {
		t.Fatal(err)
	}
	next, err := e.Next(ctx, runID)
	if err != nil || next.Action != "waiting_decision" || !slices.Contains(next.SafeNextActions, "run.decision.answer") {
		t.Fatalf("mixed Run hid its legacy question: %+v %v", next, err)
	}
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.AnswerDecision(ctx, timedAnswer(t, e, request)); err != nil {
		t.Fatal(err)
	}
	resumed, err := e.SessionTask(ctx, runID, task.AttemptID)
	if err != nil || resumed.SchemaVersion != AssistedSessionDecisionVersion || resumed.Deadline != task.Deadline || resumed.Delivery != nil || resumed.EnvelopeDigest == task.EnvelopeDigest {
		t.Fatalf("answer changed the legacy timing contract: %+v %v", resumed, err)
	}
	if _, err := e.SubmitSession(ctx, hostResult(t, e, resumed, "legacy work finished")); err != nil {
		t.Fatal(err)
	}
	timed := handOver(t, e, runID)
	if timed.SchemaVersion != AssistedSessionTimingVersion || timed.Delivery == nil {
		t.Fatal("mixed Run lost the later timed step")
	}
}

func TestResolveParkedTimedSessionDoesNotReleaseSlotTwice(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		wait := time.Minute.Milliseconds()
		e, runID, _ := timedFixture(t, flow.SessionLimits{ActiveTimeoutMS: time.Hour.Milliseconds(), DecisionWaitTimeoutMS: &wait}, "workspace_write")
		task := handOver(t, e, runID)
		parkTimed(t, e, task)
		time.Sleep(time.Minute)
		_ = e.Drive(ctx, runID)
		before, view, err := e.load(ctx, runID)
		if err != nil || before.Status != "uncertain" || !before.HasUnresolvedEffects || before.Attempts[task.AttemptID].Session.Timing.SlotHeld {
			t.Fatalf("parked workspace did not preserve a slot-free obligation: %s %v", before.Status, err)
		}
		if slot, _, err := e.Store.Slot(ctx); err != nil || slot != "" {
			t.Fatalf("parking retained a slot: %q %v", slot, err)
		}
		if _, err := e.ResolveObligation(ctx, runID, newID("command"), task.AttemptID, "", ResolveOutcomeNotApplied, "owner inspected the workspace and confirmed no change", view.Snapshot.Version); err != nil {
			t.Fatal(err)
		}
		after := driverRun(t, e, runID)
		attempt := after.Attempts[task.AttemptID]
		if attempt.Settled == nil || after.HasUnresolvedEffects || len(after.Active) != 0 || attempt.Session.Timing.SlotHeld || attempt.ProcessOutcome != nil {
			t.Fatal("resolution lost the attestation-only, slot-free settlement")
		}
		if slot, _, err := e.Store.Slot(ctx); err != nil || slot != "" {
			t.Fatalf("resolution changed capacity: %q %v", slot, err)
		}
	})
}

func TestTimedActionProposalUsesDeliveryClockAndHostPhase(t *testing.T) {
	definitions, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	boundary := Observation{UTC: "2026-09-20T12:00:00Z"}
	attempt := &Attempt{ID: "attempt:test", StepID: "step:test", ActivationID: "activation:test", Admitted: Observation{UTC: "2026-09-06T12:00:00Z"}, Deadline: Observation{UTC: "2026-09-20T12:50:00Z"}, Session: &SessionHandoff{SchemaVersion: AssistedSessionTimingVersion, PrincipalID: "principal:test", HostState: SessionAwaiting, Timing: &SessionTiming{Observed: boundary}}}
	run := Run{SchemaVersion: CoreTimingStateVersion, ID: "run:test", RootInvocationID: "invocation:test", Definitions: definitions, LastObserved: boundary, Active: []string{attempt.ID}, Attempts: map[string]*Attempt{attempt.ID: attempt}, Activations: map[string]*Activation{attempt.ActivationID: {ID: attempt.ActivationID, InvocationID: "invocation:test", StageID: "inspect"}}, Invocations: map[string]*Invocation{"invocation:test": {ID: "invocation:test", RunID: "run:test"}}}
	descriptor := flow.ToolDescriptor{AdapterRef: assistedAdapter(definitions), Operation: "inspect", EffectClass: "none", RetryClass: "never"}
	intent := ActionIntent{RunID: run.ID, StepInstanceID: attempt.StepID, OriginatingAttempt: attempt.ID, Operation: descriptor.Operation, EffectClass: descriptor.EffectClass, RetryClass: descriptor.RetryClass}
	contract := proposalContract{ActivationID: attempt.ActivationID, StageID: "inspect", EffectClass: descriptor.EffectClass, RetryClass: descriptor.RetryClass}
	for _, test := range []struct {
		name, host, utc, code string
	}{
		{"current delivery", SessionAwaiting, "2026-09-20T12:01:00Z", ""},
		{"clock rollback", SessionAwaiting, "2026-09-20T11:59:00Z", "deadline_clock_unqualified"},
		{"waiting for answer", sessionWaitingDecision, "2026-09-20T12:01:00Z", "action_forbidden"},
		{"waiting for capacity", SessionWaitingAdmission, "2026-09-20T12:01:00Z", "action_forbidden"},
	} {
		t.Run(test.name, func(t *testing.T) {
			attempt.Session.HostState = test.host
			_, err := run.actionProposalAttempt("principal:test", intent, descriptor, contract, Observation{UTC: test.utc})
			code := ""
			if err != nil {
				code = refusalCode(err)
			}
			if code != test.code {
				t.Fatalf("wanted %q, got %v", test.code, err)
			}
		})
	}
}
