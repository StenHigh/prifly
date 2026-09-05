package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stenhigh/prifly/internal/flow"
)

// Unlike parkTimed, build the request without submitting it so refusals can be
// exercised through the same public API as a host, without rewriting Run state.
func timedDecisionRequest(t *testing.T, e *Engine, task SessionTask) DecisionRequest {
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
	return DecisionRequest{SchemaVersion: DecisionRequestTimingVersion, RunID: task.RunID, AttemptID: task.AttemptID, EnvelopeDigest: task.EnvelopeDigest, DecisionID: definition.ID, DefinitionDigest: digest, ExpectedRunVersion: view.Snapshot.Version, YieldExecution: true}
}

func TestTimedDecisionRefusalsPreserveDelivery(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		e, runID, _ := timedFixture(t, flow.SessionLimits{ActiveTimeoutMS: time.Hour.Milliseconds()}, "none")
		task := handOver(t, e, runID)
		request := timedDecisionRequest(t, e, task)
		before, err := canonical(driverRun(t, e, runID))
		if err != nil {
			t.Fatal(err)
		}
		for _, test := range []struct {
			name string
			code string
			edit func(*DecisionRequest)
		}{
			{"yield required", "decision_request_unsupported", func(r *DecisionRequest) { r.YieldExecution = false }},
			{"legacy request", "decision_request_unsupported", func(r *DecisionRequest) { r.SchemaVersion, r.YieldExecution = DecisionRequestVersion, false }},
			{"stale version", "decision_conflict", func(r *DecisionRequest) { r.ExpectedRunVersion-- }},
			{"unknown decision", "unknown_decision", func(r *DecisionRequest) { r.DecisionID = "unknown" }},
			{"stale delivery", "decision_request_unsupported", func(r *DecisionRequest) { r.EnvelopeDigest = rawDigest([]byte("stale")) }},
		} {
			invalid := request
			test.edit(&invalid)
			if _, err := e.RequestDecision(ctx, invalid); refusalCode(err) != test.code {
				t.Fatalf("%s: request refusal: %v", test.name, err)
			}
			after, err := canonical(driverRun(t, e, runID))
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("%s: refused request changed Run: %v", test.name, err)
			}
		}
		time.Sleep(time.Hour)
		if _, err := e.RequestDecision(ctx, request); refusalCode(err) != "attempt_deadline_expired" {
			t.Fatalf("expired work cannot begin human waiting: %v", err)
		}
		after, err := canonical(driverRun(t, e, runID))
		if err != nil || !bytes.Equal(before, after) {
			t.Fatalf("expiry refusal changed delivery: %v", err)
		}
		_, held, err := e.AdmissionCapacity(ctx)
		if err != nil || len(held) != 1 || held[task.AttemptID] != task.RunID {
			t.Fatalf("refusal silently freed the slot: %+v %v", held, err)
		}
	})
}

func TestTimedDecisionInvalidAnswerDoesNotResume(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	ctx := context.Background()
	e, runID, _ := timedFixture(t, flow.SessionLimits{ActiveTimeoutMS: time.Hour.Milliseconds()}, "none")
	task := handOver(t, e, runID)
	request := parkTimed(t, e, task)
	answer := timedAnswer(t, e, request)
	answer.Value = json.RawMessage(`false`)
	if _, err := e.AnswerDecision(ctx, answer); refusalCode(err) != "invalid_decision_answer" {
		t.Fatalf("undeclared answer: %v", err)
	}
	run := driverRun(t, e, runID)
	a := run.Attempts[task.AttemptID]
	if run.PendingDecision == nil || a.Session.HostState != sessionWaitingDecision || a.Session.Timing.SlotHeld || a.Session.DeliveryGeneration != 1 || a.EnvelopeDigest != task.EnvelopeDigest || len(a.Session.DecisionContext) != 0 {
		t.Fatal("invalid answer changed the waiting delivery")
	}
	if _, err := e.SessionTask(ctx, runID, task.AttemptID); refusalCode(err) != "no_active_handoff" {
		t.Fatalf("invalid answer dispatched: %v", err)
	}
	answer.Value = json.RawMessage(`true`)
	if _, err := e.AnswerDecision(ctx, answer); err != nil {
		t.Fatal(err)
	}
	run = driverRun(t, e, runID)
	a = run.Attempts[task.AttemptID]
	if run.PendingDecision != nil || a.Session.HostState != SessionWaitingAdmission || a.Session.Timing.SlotHeld || a.Session.DeliveryGeneration != 1 || a.EnvelopeDigest != task.EnvelopeDigest || string(a.Session.DecisionContext["continue"]) != "true" {
		t.Fatal("valid answer renewed delivery before readmission")
	}
	if _, err := e.AnswerDecision(ctx, answer); err == nil {
		t.Fatal("duplicate answer replaced the recorded answer")
	}
	_, held, err := e.AdmissionCapacity(ctx)
	if err != nil || len(held) != 0 {
		t.Fatalf("answer reacquired capacity: %+v %v", held, err)
	}
}

func TestTimedAutomaticDecisionPreservesBudgetAndHonorsStop(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, mode := range []string{"preanswered", "autonomous"} {
		for _, stop := range []string{"none", "run", "project"} {
			t.Run(mode+"/"+stop, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					ctx := context.Background()
					e, _, options := timedFixture(t, flow.SessionLimits{ActiveTimeoutMS: time.Hour.Milliseconds()}, "none")
					catalog := *options.DecisionCatalog
					catalog.Decisions = append([]DecisionDefinition(nil), catalog.Decisions...)
					sheet := *options.DecisionSheet
					if mode == "autonomous" {
						catalog.Decisions[0].Automatic = true
						catalog.Decisions[0].Recommendation = json.RawMessage(`true`)
						sheet.DecisionPolicy = "autonomous"
					} else {
						digest, err := DecisionDefinitionDigest(catalog.Decisions[0])
						if err != nil {
							t.Fatal(err)
						}
						sheet.Records = []DecisionRecord{{SchemaVersion: DecisionRecordVersion, DefinitionID: "continue", DefinitionDigest: digest, Status: "answered", Source: "actor", Value: json.RawMessage(`true`)}}
					}
					digest, err := DecisionCatalogDigest(catalog)
					if err != nil {
						t.Fatal(err)
					}
					sheet.CatalogDigest = digest
					options.CommandID, options.DecisionCatalog, options.DecisionSheet = newID("command"), &catalog, &sheet
					started, err := e.Start(ctx, options)
					if err != nil {
						t.Fatal(err)
					}
					runID := started.Receipt.RunID
					task := handOver(t, e, runID)
					time.Sleep(10 * time.Minute)
					if stop != "none" {
						restrictTimedDecision(t, e, runID, stop, "pause")
					}
					request := timedDecisionRequest(t, e, task)
					before, err := canonical(driverRun(t, e, runID))
					if err != nil {
						t.Fatal(err)
					}
					_, err = e.RequestDecision(ctx, request)
					if stop != "none" {
						if refusalCode(err) != "dispatch_blocked" {
							t.Fatalf("automatic delivery bypassed stop: %v", err)
						}
						after, err := canonical(driverRun(t, e, runID))
						if err != nil || !bytes.Equal(before, after) {
							t.Fatalf("refusal consumed or refilled saved allowance: %v", err)
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					run := driverRun(t, e, runID)
					a := run.Attempts[task.AttemptID]
					if run.PendingDecision != nil || a.Session.HostState != SessionAwaiting || !a.Session.Timing.SlotHeld || a.Session.DeliveryGeneration != 2 || a.EnvelopeDigest == task.EnvelopeDigest || a.Session.Timing.RemainingMS != (50*time.Minute).Milliseconds() {
						t.Fatal("automatic answer parked, refilled allowance or failed to renew")
					}
					record := run.DecisionLedger[len(run.DecisionLedger)-1]
					source, status := "actor", "answered"
					if mode == "autonomous" {
						source, status = "autonomous_policy", "defaulted"
					}
					if record.SchemaVersion != DecisionRecordTimingVersion || record.AttemptID != task.AttemptID || record.Source != source || record.Status != status || string(record.Value) != "true" {
						t.Fatalf("automatic decision provenance: %+v", record)
					}
				})
			})
		}
	}
}

func restrictTimedDecision(t *testing.T, e *Engine, runID, scope, kind string) {
	t.Helper()
	ctx := context.Background()
	if scope == "project" {
		if _, err := e.RestrictControl(ctx, ControlRestrictRequest{CommandID: newID("command"), Scope: scope, Reason: "owner paused project"}); err != nil {
			t.Fatal(err)
		}
		return
	}
	if _, err := e.Restrict(ctx, RestrictCommand{SchemaVersion: "1", CommandID: newID("command"), Scope: scope, ScopeID: runID, Kind: kind, Reason: "owner restricted Run"}); err != nil {
		t.Fatal(err)
	}
}

func TestTimedDecisionAnswerDoesNotBypassCurrentControls(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, scope := range []string{"run", "project"} {
		t.Run(scope, func(t *testing.T) {
			ctx := context.Background()
			e, runID, _ := timedFixture(t, flow.SessionLimits{ActiveTimeoutMS: time.Hour.Milliseconds()}, "none")
			task := handOver(t, e, runID)
			request := parkTimed(t, e, task)
			restrictTimedDecision(t, e, runID, scope, "pause")
			if _, err := e.AnswerDecision(ctx, timedAnswer(t, e, request)); err != nil {
				t.Fatalf("stop should retain the answer without dispatch: %v", err)
			}
			// A stopped driver may report its block or return at the waiting state;
			// neither outcome may grant a new execution delivery.
			if err := e.Drive(ctx, runID); err != nil && refusalCode(err) != "dispatch_blocked" && refusalCode(err) != "control_stop_active" {
				t.Fatal(err)
			}
			run := driverRun(t, e, runID)
			a := run.Attempts[task.AttemptID]
			if run.PendingDecision != nil || a.Session.HostState != SessionWaitingAdmission || a.Session.Timing.SlotHeld || a.Session.DeliveryGeneration != 1 || a.EnvelopeDigest != task.EnvelopeDigest || string(a.Session.DecisionContext["continue"]) != "true" {
				t.Fatal("saved answer bypassed current controls")
			}
			if _, err := e.SessionTask(ctx, runID, task.AttemptID); refusalCode(err) != "no_active_handoff" {
				t.Fatalf("stopped Run produced a handoff: %v", err)
			}
			_, held, err := e.AdmissionCapacity(ctx)
			if err != nil || len(held) != 0 {
				t.Fatalf("stopped answer reacquired capacity: %+v %v", held, err)
			}
		})
	}
}

func TestTimedDecisionCancelAnswerSerialization(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, order := range []string{"cancel first", "answer first", "concurrent"} {
		t.Run(order, func(t *testing.T) {
			ctx := context.Background()
			e, runID, _ := timedFixture(t, flow.SessionLimits{ActiveTimeoutMS: time.Hour.Milliseconds()}, "none")
			task := handOver(t, e, runID)
			request := parkTimed(t, e, task)
			answer := timedAnswer(t, e, request)
			var answerErr, cancelErr error
			answerCall := func() { _, answerErr = e.AnswerDecision(ctx, answer) }
			cancelCall := func() {
				_, cancelErr = e.Restrict(ctx, RestrictCommand{SchemaVersion: "1", CommandID: newID("command"), Scope: "run", ScopeID: runID, Kind: "cancel", Reason: "owner cancelled while answering"})
			}
			switch order {
			case "cancel first":
				cancelCall()
				answerCall()
			case "answer first":
				answerCall()
				cancelCall()
			case "concurrent":
				start := make(chan struct{})
				var done sync.WaitGroup
				done.Add(2)
				go func() { defer done.Done(); <-start; answerCall() }()
				go func() { defer done.Done(); <-start; cancelCall() }()
				close(start)
				done.Wait()
			}
			if cancelErr != nil {
				t.Fatal(cancelErr)
			}
			if order == "answer first" && answerErr != nil || order == "cancel first" && answerErr == nil {
				t.Fatalf("unexpected serialized answer: %v", answerErr)
			}
			if answerErr != nil && refusalCode(answerErr) != "decision_conflict" && refusalCode(answerErr) != "version_conflict" {
				t.Fatalf("unexpected racing answer error: %v", answerErr)
			}
			if err := e.Drive(ctx, runID); err != nil {
				t.Fatal(err)
			}
			run := driverRun(t, e, runID)
			a := run.Attempts[task.AttemptID]
			if run.Status != "cancelled" || run.PendingDecision != nil || a.Session.DeliveryGeneration != 1 || a.ProcessOutcome != nil || a.Settled == nil {
				t.Fatal("cancel/answer race dispatched work or lost terminal closure")
			}
			record := run.DecisionLedger[0]
			if answerErr == nil {
				if record.Status != "answered" || record.Source != "actor" || string(record.Value) != "true" {
					t.Fatal("cancellation discarded an answer committed before it")
				}
			} else if record.Status != "cancelled" || record.Source != "unanswered" || len(record.Value) != 0 || record.ClosureReason == "" {
				t.Fatal("cancelled question invented an answer or lost its reason")
			}
			_, held, err := e.AdmissionCapacity(ctx)
			if err != nil || len(held) != 0 {
				t.Fatalf("cancelled Run retained capacity: %+v %v", held, err)
			}
		})
	}
}

// Candidate-state unit tests, not proof of action execution: today's public
// action delivery stops at prepared/not_started. The pause guard must still
// reject unresolved or incomplete knowledge instead of asserting quiescence.
func TestTimedDecisionYieldRejectsUnresolvedCandidateEffects(t *testing.T) {
	notStarted, unknown := "not_started", "unknown"
	a := &Attempt{ID: "attempt:waiting"}
	for _, test := range []struct {
		name       string
		unresolved bool
		delivery   *ActionDelivery
		allowed    bool
	}{
		{name: "no action", allowed: true},
		{name: "known unresolved effects", unresolved: true},
		{name: "prepared not started", delivery: &ActionDelivery{OwningAttemptID: a.ID, DeliveryStatus: "prepared", EffectStatus: &notStarted}, allowed: true},
		{name: "in flight", delivery: &ActionDelivery{OwningAttemptID: a.ID, DeliveryStatus: "started", EffectStatus: &notStarted}},
		{name: "unknown effect", delivery: &ActionDelivery{OwningAttemptID: a.ID, DeliveryStatus: "prepared", EffectStatus: &unknown}},
		{name: "missing effect knowledge", delivery: &ActionDelivery{OwningAttemptID: a.ID, DeliveryStatus: "prepared"}},
		{name: "sibling prepared", delivery: &ActionDelivery{OwningAttemptID: "attempt:sibling", DeliveryStatus: "prepared", EffectStatus: &notStarted}, allowed: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := Run{HasUnresolvedEffects: test.unresolved, ActionDeliveries: map[string]ActionDelivery{}}
			if test.delivery != nil {
				r.ActionDeliveries["delivery:test"] = *test.delivery
			}
			err := decisionYieldAdmissible(r, a)
			if test.allowed && err != nil || !test.allowed && refusalCode(err) != "decision_yield_blocked" {
				t.Fatalf("yield guard: %v", err)
			}
		})
	}
}

func TestTimedDecisionInvariantRequiresExactOwnerEditions(t *testing.T) {
	definition := DecisionDefinition{SchemaVersion: DecisionDefinitionVersion, ID: "continue", Title: "Continue", Phase: "runtime", Choices: []DecisionChoice{{ID: "yes", Title: "Yes", Value: json.RawMessage(`true`)}}, Sensitivity: "ordinary", Destination: DecisionDestination{Kind: "session_context", Name: "continue"}}
	catalog := DecisionCatalog{SchemaVersion: DecisionCatalogVersion, Decisions: []DecisionDefinition{definition}}
	catalogDigest, err := DecisionCatalogDigest(catalog)
	if err != nil {
		t.Fatal(err)
	}
	definitionDigest, err := DecisionDefinitionDigest(definition)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name           string
		timedOwner     bool
		recordVersion  string
		requestVersion string
		preflight      bool
		allowed        bool
	}{
		{"timed owner", true, DecisionRecordTimingVersion, DecisionRequestTimingVersion, false, true},
		{"legacy record on timed owner", true, DecisionRecordVersion, DecisionRequestTimingVersion, false, false},
		{"legacy request on timed owner", true, DecisionRecordTimingVersion, DecisionRequestVersion, false, false},
		{"legacy owner", false, DecisionRecordVersion, DecisionRequestVersion, false, true},
		{"timed record on legacy owner", false, DecisionRecordTimingVersion, DecisionRequestVersion, false, false},
		{"timed request on legacy owner", false, DecisionRecordVersion, DecisionRequestTimingVersion, false, false},
		{"unchanged preflight edition", true, DecisionRecordVersion, "", true, true},
		{"timed preflight record without owner", true, DecisionRecordTimingVersion, "", true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			session := &SessionHandoff{SchemaVersion: AssistedSessionDecisionVersion}
			if test.timedOwner {
				session.SchemaVersion, session.Timing = AssistedSessionTimingVersion, &SessionTiming{}
			}
			r := Run{SchemaVersion: CoreTimingStateVersion, DecisionCatalog: &catalog, DecisionSheet: &DecisionSheet{SchemaVersion: DecisionSheetVersion, CatalogDigest: catalogDigest, ProfileSource: "none", Records: []DecisionRecord{}}, Attempts: map[string]*Attempt{"attempt:test": {Session: session}}}
			record := DecisionRecord{SchemaVersion: test.recordVersion, DefinitionID: definition.ID, DefinitionDigest: definitionDigest, AttemptID: "attempt:test", Status: "pending", Source: "unanswered"}
			if test.preflight {
				record.AttemptID, record.Status, record.Source, record.Value = "", "answered", "actor", json.RawMessage(`true`)
			} else {
				r.PendingDecision = &DecisionRequest{SchemaVersion: test.requestVersion, AttemptID: "attempt:test", YieldExecution: test.requestVersion == DecisionRequestTimingVersion}
			}
			r.DecisionLedger = []DecisionRecord{record}
			if err := decisionInvariant(r); test.allowed && err != nil || !test.allowed && err == nil {
				t.Fatalf("decision contract edition: %v", err)
			}
		})
	}
}

func TestTimedDecisionResumeRechecksRevokedPackage(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		e, _, options := timedFixture(t, flow.SessionLimits{ActiveTimeoutMS: time.Hour.Milliseconds()}, "none")
		body := "# Inspect\nAsk the declared continue question before finishing.\n"
		// Use a distinct context identity: the base fixture's ordinary local
		// instructions must not masquerade as this imported package component.
		skillRef := flow.Ref{ID: "test:context/revocable-skill", Version: "1.0.0", Digest: rawDigest([]byte(body))}
		source := packageSource(t, map[string]string{"skills/inspect.md": body}, []map[string]any{{"kind": "context", "ref": skillRef, "path": "skills/inspect.md"}}, nil)
		if _, err := e.ImportPackage(ctx, PackageImportRequest{CommandID: newID("command"), Directory: source, Reason: "install the exact instructions for this step"}); err != nil {
			t.Fatal(err)
		}
		var step flow.StepDefinition
		data, err := os.ReadFile(filepath.Join(e.Root, "steps/plan.json"))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &step); err != nil {
			t.Fatal(err)
		}
		step.Version, step.InstructionsRef = "3.0.0", &skillRef
		data = writeRegistryDocument(t, e, "steps/plan.json", step)
		stepRef := flow.Ref{ID: step.ID, Version: step.Version, Digest: rawDigest(data)}
		registry, err := e.localRegistry()
		if err != nil {
			t.Fatal(err)
		}
		for index := range registry.Entries {
			if registry.Entries[index].Kind == "step" {
				registry.Entries[index].Ref = stepRef
			}
		}
		writeRuntimeJSON(t, filepath.Join(e.Root, e.Config.Configuration.RegistryFile), registry)
		var workflow flow.WorkflowRevision
		data, err = os.ReadFile(filepath.Join(e.Root, options.WorkflowFile))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &workflow); err != nil {
			t.Fatal(err)
		}
		stage := workflow.Definition.Stages["plan"]
		workflow.Version, stage.StepRef = "3.0.0", stepRef
		workflow.Definition.Stages["plan"] = stage
		writeRuntimeJSON(t, filepath.Join(e.Root, options.WorkflowFile), workflow)
		opened, err := Open(e.Root, false)
		if err != nil {
			t.Fatal(err)
		}
		defer opened.Close()
		e = opened
		options.CommandID = newID("command")
		started, err := e.Start(ctx, options)
		if err != nil {
			t.Fatal(err)
		}
		runID := started.Receipt.RunID
		task := handOver(t, e, runID)
		if len(task.SkillRefs) != 1 || task.SkillRefs[0] != skillRef {
			t.Fatalf("delivery did not use the imported context: %+v", task.SkillRefs)
		}
		time.Sleep(10 * time.Minute)
		request := parkTimed(t, e, task)
		if _, err := e.SetPackageStatus(ctx, PackageLifecycleRequest{CommandID: newID("command"), ID: "aif:package/pilot", Version: "1.0.0", Status: PackageRevoked, Reason: "instructions withdrawn while the host waits"}); err != nil {
			t.Fatal(err)
		}
		if _, err := e.AnswerDecision(ctx, timedAnswer(t, e, request)); err != nil {
			t.Fatalf("revocation must not discard the human answer: %v", err)
		}
		before, err := canonical(driverRun(t, e, runID))
		if err != nil {
			t.Fatal(err)
		}
		if err := e.Drive(ctx, runID); refusalCode(err) != "package_revoked" {
			t.Fatalf("revoked instructions were readmitted: %v", err)
		}
		run := driverRun(t, e, runID)
		after, err := canonical(run)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatalf("refused readmission changed Run: %v", err)
		}
		a := run.Attempts[task.AttemptID]
		if run.PendingDecision != nil || a.Session.HostState != SessionWaitingAdmission || a.Session.Timing.SlotHeld || a.Session.DeliveryGeneration != 1 || a.EnvelopeDigest != task.EnvelopeDigest || a.Session.Timing.RemainingMS != (50*time.Minute).Milliseconds() || string(a.Session.DecisionContext["continue"]) != "true" {
			t.Fatal("revocation lost the answer, refilled time or granted a new delivery")
		}
		if _, err := e.SessionTask(ctx, runID, task.AttemptID); refusalCode(err) != "no_active_handoff" {
			t.Fatalf("revoked package produced a handoff: %v", err)
		}
		_, held, err := e.AdmissionCapacity(ctx)
		if err != nil || len(held) != 0 {
			t.Fatalf("revoked package acquired a slot: %+v %v", held, err)
		}
	})
}
