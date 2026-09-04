package runtime

import (
	"context"
	"encoding/json"
	"testing"
)

func TestValidateDecisionDefinition(t *testing.T) {
	valid := DecisionDefinition{
		SchemaVersion:  DecisionDefinitionVersion,
		ID:             "plan_profile",
		Title:          "Plan profile",
		Phase:          "preflight",
		Required:       true,
		Choices:        []DecisionChoice{{ID: "fast", Title: "Fast", Value: json.RawMessage(`"fast"`)}},
		Recommendation: json.RawMessage(`"fast"`),
		Automatic:      true,
		Sensitivity:    "ordinary",
		Destination:    DecisionDestination{Kind: "package_profile"},
	}
	if err := ValidateDecisionDefinition(valid); err != nil {
		t.Fatal(err)
	}
	catalog := DecisionCatalog{SchemaVersion: DecisionCatalogVersion, Decisions: []DecisionDefinition{valid}}
	digest, err := DecisionCatalogDigest(catalog)
	if err != nil {
		t.Fatal(err)
	}
	definitionDigest, err := DecisionDefinitionDigest(valid)
	if err != nil {
		t.Fatal(err)
	}
	sheet := DecisionSheet{SchemaVersion: DecisionSheetVersion, CatalogDigest: digest, PackageProfile: "fast", ProfileSource: "actor", Records: []DecisionRecord{{SchemaVersion: DecisionRecordVersion, DefinitionID: valid.ID, DefinitionDigest: definitionDigest, Status: "answered", Source: "actor", Value: json.RawMessage(`"fast"`)}}}
	if err := ValidateDecisionSheet(catalog, sheet); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*DecisionDefinition){
		"unknown phase":       func(value *DecisionDefinition) { value.Phase = "later" },
		"duplicate choice":    func(value *DecisionDefinition) { value.Choices = append(value.Choices, value.Choices[0]) },
		"automatic scope":     func(value *DecisionDefinition) { value.Sensitivity = "scope-changing" },
		"unknown destination": func(value *DecisionDefinition) { value.Destination.Kind = "route" },
		"unnamed input":       func(value *DecisionDefinition) { value.Destination = DecisionDestination{Kind: "launch_input"} },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			candidate.Choices = append([]DecisionChoice(nil), valid.Choices...)
			mutate(&candidate)
			if err := ValidateDecisionDefinition(candidate); err == nil {
				t.Fatal("invalid decision definition was accepted")
			}
		})
	}
}

func TestDecisionStateKeepsLegacyRunsUnchanged(t *testing.T) {
	e, runID, _ := assistedWorkspaceFixture(t, "")
	before, err := e.View(context.Background(), runID)
	if err != nil || before.Run.SchemaVersion == CoreDecisionStateVersion || before.Run.DecisionCatalog != nil || before.Run.DecisionSheet != nil || len(before.Run.DecisionLedger) != 0 || before.Run.PendingDecision != nil {
		t.Fatalf("legacy run unexpectedly carries decision state: %+v %v", before.Run, err)
	}
	root := e.Root
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	e, err = Open(root, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close() })
	after, err := e.View(context.Background(), runID)
	if err != nil || after.Run.SchemaVersion != before.Run.SchemaVersion || after.Run.DecisionCatalog != nil || after.Run.DecisionSheet != nil || len(after.Run.DecisionLedger) != 0 || after.Run.PendingDecision != nil {
		t.Fatalf("legacy run was reinterpreted after reopen: %+v %v", after.Run, err)
	}
	for _, field := range []string{"decision_catalog", "decision_sheet", "decision_ledger", "pending_decision"} {
		data := []byte(`{"schema_version":"` + before.Run.SchemaVersion + `","` + field + `":null}`)
		if err := decodeState(data, &Run{}); err == nil {
			t.Fatalf("legacy state accepted explicit %s", field)
		}
	}
}

func TestDecisionConditionRequiresSealedPredecessorAnswer(t *testing.T) {
	definition := DecisionDefinition{When: &DecisionCondition{Answers: map[string]json.RawMessage{"roadmap_linkage": json.RawMessage(`"link"`)}}}
	linked := []DecisionRecord{{DefinitionID: "roadmap_linkage", Value: json.RawMessage(`"link"`)}}
	if !decisionApplies(definition, "full", linked) {
		t.Fatal("matching preflight answer did not reveal the dependent decision")
	}
	if decisionApplies(definition, "full", []DecisionRecord{{DefinitionID: "roadmap_linkage", Value: json.RawMessage(`"skip"`)}}) || decisionApplies(definition, "full", nil) {
		t.Fatal("missing or different preflight answer revealed the dependent decision")
	}
}

func TestDecisionBridgeResumesSameAssistedAttempt(t *testing.T) {
	preflight := DecisionDefinition{SchemaVersion: DecisionDefinitionVersion, ID: "logging", Title: "Logging", Phase: "preflight", Required: true, Choices: []DecisionChoice{{ID: "concise", Title: "Concise", Value: json.RawMessage(`"concise"`)}}, Sensitivity: "ordinary", Destination: DecisionDestination{Kind: "session_context", Name: "logging"}}
	runtime := DecisionDefinition{SchemaVersion: DecisionDefinitionVersion, ID: "continue", Title: "Continue", Phase: "runtime", Choices: []DecisionChoice{{ID: "yes", Title: "Yes", Value: json.RawMessage(`true`)}, {ID: "no", Title: "No", Value: json.RawMessage(`false`)}}, Sensitivity: "ordinary", Destination: DecisionDestination{Kind: "session_context", Name: "continue"}}
	catalog := DecisionCatalog{SchemaVersion: DecisionCatalogVersion, Decisions: []DecisionDefinition{preflight, runtime}}
	catalogDigest, err := DecisionCatalogDigest(catalog)
	if err != nil {
		t.Fatal(err)
	}
	preflightDigest, err := DecisionDefinitionDigest(preflight)
	if err != nil {
		t.Fatal(err)
	}
	sheet := DecisionSheet{SchemaVersion: DecisionSheetVersion, CatalogDigest: catalogDigest, ProfileSource: "none", Records: []DecisionRecord{{SchemaVersion: DecisionRecordVersion, DefinitionID: preflight.ID, DefinitionDigest: preflightDigest, Status: "answered", Source: "actor", Value: json.RawMessage(`"concise"`)}}}
	e, runID, _ := assistedWorkspaceFixtureWithDecisions(t, "", &catalog, &sheet)
	task := handOver(t, e, runID)
	if task.SchemaVersion != AssistedSessionDecisionVersion || !task.DecisionBridge || string(task.DecisionContext["logging"]) != `"concise"` {
		t.Fatalf("decision bridge handoff is incomplete: %+v", task)
	}
	runtimeDigest, err := DecisionDefinitionDigest(runtime)
	if err != nil {
		t.Fatal(err)
	}
	request := DecisionRequest{SchemaVersion: DecisionRequestVersion, RunID: runID, AttemptID: task.AttemptID, EnvelopeDigest: task.EnvelopeDigest, DecisionID: runtime.ID, DefinitionDigest: runtimeDigest, ExpectedRunVersion: task.RunVersion}
	if _, err := e.SubmitSession(context.Background(), SessionSubmission{SchemaVersion: task.SchemaVersion, RunID: runID, AttemptID: task.AttemptID, EnvelopeDigest: task.EnvelopeDigest, DecisionRequest: &request}); err != nil {
		t.Fatal(err)
	}
	root := e.Root
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	e, err = Open(root, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close() })
	if _, err := e.SessionTask(context.Background(), runID, task.AttemptID); refusalCode(err) != "no_active_handoff" {
		t.Fatalf("paused delivery remained available: %v", err)
	}
	next, err := e.Next(context.Background(), runID)
	if err != nil || next.SchemaVersion != CoreDecisionNextVersion || next.Action != "waiting_decision" {
		t.Fatalf("pending bridge state is not visible: %+v %v", next, err)
	}
	requestDigest, err := DecisionRequestDigest(request)
	if err != nil {
		t.Fatal(err)
	}
	answer := DecisionAnswer{SchemaVersion: DecisionAnswerVersion, RunID: runID, DecisionID: runtime.ID, DefinitionDigest: runtimeDigest, RequestDigest: requestDigest, ExpectedRunVersion: next.RunVersion, Value: json.RawMessage(`true`)}
	stale := answer
	stale.ExpectedRunVersion--
	if _, err := e.AnswerDecision(context.Background(), stale); err == nil {
		t.Fatal("stale decision answer was accepted")
	} else {
		rejectionCode(t, err, "decision_conflict")
	}
	if _, err := e.AnswerDecision(context.Background(), answer); err != nil {
		t.Fatal(err)
	}
	if _, err := e.AnswerDecision(context.Background(), answer); err == nil {
		t.Fatal("second answer replaced the first one")
	}
	resumed, err := e.SessionTask(context.Background(), runID, task.AttemptID)
	if err != nil || resumed.AttemptID != task.AttemptID || resumed.EnvelopeDigest == task.EnvelopeDigest || string(resumed.DecisionContext["continue"]) != "true" {
		t.Fatalf("answer did not create a new delivery for the same attempt: %+v %v", resumed, err)
	}
	if _, err := e.SubmitSession(context.Background(), hostResult(t, e, resumed, "resumed after answer")); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	view, err := e.View(context.Background(), runID)
	if err != nil || view.Run.PendingDecision != nil || len(view.Run.DecisionLedger) != 2 || view.Run.DecisionLedger[1].Status != "answered" || view.Run.DecisionLedger[1].Source != "actor" {
		t.Fatalf("decision ledger did not retain the answer: %+v %v", view.Run.DecisionLedger, err)
	}
}

func TestDecisionBridgeAutonomousPolicyUsesDeclaredRecommendation(t *testing.T) {
	runtime := DecisionDefinition{SchemaVersion: DecisionDefinitionVersion, ID: "continue", Title: "Continue", Phase: "runtime", Choices: []DecisionChoice{{ID: "yes", Title: "Yes", Value: json.RawMessage(`true`)}}, Recommendation: json.RawMessage(`true`), Automatic: true, Sensitivity: "ordinary", Destination: DecisionDestination{Kind: "session_context", Name: "continue"}}
	catalog := DecisionCatalog{SchemaVersion: DecisionCatalogVersion, Decisions: []DecisionDefinition{runtime}}
	digest, err := DecisionCatalogDigest(catalog)
	if err != nil {
		t.Fatal(err)
	}
	sheet := DecisionSheet{SchemaVersion: DecisionSheetVersion, CatalogDigest: digest, ProfileSource: "none", DecisionPolicy: "autonomous", Records: []DecisionRecord{}}
	e, runID, _ := assistedWorkspaceFixtureWithDecisions(t, "", &catalog, &sheet)
	task := handOver(t, e, runID)
	definitionDigest, err := DecisionDefinitionDigest(runtime)
	if err != nil {
		t.Fatal(err)
	}
	request := DecisionRequest{SchemaVersion: DecisionRequestVersion, RunID: runID, AttemptID: task.AttemptID, EnvelopeDigest: task.EnvelopeDigest, DecisionID: runtime.ID, DefinitionDigest: definitionDigest, ExpectedRunVersion: task.RunVersion}
	if _, err := e.SubmitSession(context.Background(), SessionSubmission{SchemaVersion: task.SchemaVersion, RunID: runID, AttemptID: task.AttemptID, EnvelopeDigest: task.EnvelopeDigest, DecisionRequest: &request}); err != nil {
		t.Fatal(err)
	}
	resumed, err := e.SessionTask(context.Background(), runID, task.AttemptID)
	if err != nil || string(resumed.DecisionContext["continue"]) != "true" {
		t.Fatalf("automatic answer did not resume the same delivery: %+v %v", resumed, err)
	}
	view, err := e.View(context.Background(), runID)
	if err != nil || view.Run.PendingDecision != nil || len(view.Run.DecisionLedger) != 1 || view.Run.DecisionLedger[0].Status != "defaulted" || view.Run.DecisionLedger[0].Source != "autonomous_policy" {
		t.Fatalf("automatic decision was not recorded as policy: %+v %v", view.Run.DecisionLedger, err)
	}
}

func TestDecisionBridgeKeepsRestrictedChoiceForHumanAndRefusesUnknown(t *testing.T) {
	restricted := DecisionDefinition{SchemaVersion: DecisionDefinitionVersion, ID: "publish_scope", Title: "Publish scope", Phase: "runtime", Choices: []DecisionChoice{{ID: "none", Title: "No publication", Value: json.RawMessage(`false`)}}, Recommendation: json.RawMessage(`false`), Sensitivity: "scope-changing", Destination: DecisionDestination{Kind: "session_context", Name: "publish_scope"}}
	catalog := DecisionCatalog{SchemaVersion: DecisionCatalogVersion, Decisions: []DecisionDefinition{restricted}}
	digest, err := DecisionCatalogDigest(catalog)
	if err != nil {
		t.Fatal(err)
	}
	sheet := DecisionSheet{SchemaVersion: DecisionSheetVersion, CatalogDigest: digest, ProfileSource: "none", DecisionPolicy: "autonomous", Records: []DecisionRecord{}}
	e, runID, _ := assistedWorkspaceFixtureWithDecisions(t, "", &catalog, &sheet)
	task := handOver(t, e, runID)
	definitionDigest, err := DecisionDefinitionDigest(restricted)
	if err != nil {
		t.Fatal(err)
	}
	request := DecisionRequest{SchemaVersion: DecisionRequestVersion, RunID: runID, AttemptID: task.AttemptID, EnvelopeDigest: task.EnvelopeDigest, DecisionID: restricted.ID, DefinitionDigest: definitionDigest, ExpectedRunVersion: task.RunVersion}
	unknown := request
	unknown.DecisionID, unknown.DefinitionDigest = "unknown", "sha256:unknown"
	if _, err := e.SubmitSession(context.Background(), SessionSubmission{SchemaVersion: task.SchemaVersion, RunID: runID, AttemptID: task.AttemptID, EnvelopeDigest: task.EnvelopeDigest, DecisionRequest: &unknown}); err == nil {
		t.Fatal("undeclared runtime decision was accepted")
	} else {
		rejectionCode(t, err, "unknown_decision")
	}
	stillAwaiting, err := e.SessionTask(context.Background(), runID, task.AttemptID)
	if err != nil || stillAwaiting.EnvelopeDigest != task.EnvelopeDigest {
		t.Fatalf("unknown decision changed or redispatched the original delivery: %+v %v", stillAwaiting, err)
	}
	if _, err := e.SubmitSession(context.Background(), SessionSubmission{SchemaVersion: task.SchemaVersion, RunID: runID, AttemptID: task.AttemptID, EnvelopeDigest: task.EnvelopeDigest, DecisionRequest: &request}); err != nil {
		t.Fatal(err)
	}
	next, err := e.Next(context.Background(), runID)
	if err != nil || next.Action != "waiting_decision" {
		t.Fatalf("autonomous policy answered a restricted choice: %+v %v", next, err)
	}
}

func TestDecisionBridgeRejectsUnbridgedSession(t *testing.T) {
	e, runID, _ := assistedWorkspaceFixture(t, "")
	task := handOver(t, e, runID)
	request := DecisionRequest{SchemaVersion: DecisionRequestVersion, RunID: runID, AttemptID: task.AttemptID, EnvelopeDigest: task.EnvelopeDigest, DecisionID: "continue", DefinitionDigest: "sha256:undeclared", ExpectedRunVersion: task.RunVersion}
	if _, err := e.SubmitSession(context.Background(), SessionSubmission{SchemaVersion: task.SchemaVersion, RunID: runID, AttemptID: task.AttemptID, EnvelopeDigest: task.EnvelopeDigest, DecisionRequest: &request}); err == nil {
		t.Fatal("pre-decision assisted session accepted a decision request")
	}
	stillAwaiting, err := e.SessionTask(context.Background(), runID, task.AttemptID)
	if err != nil || stillAwaiting.EnvelopeDigest != task.EnvelopeDigest {
		t.Fatalf("unbridged rejection changed the original delivery: %+v %v", stillAwaiting, err)
	}
}

// Two packages with unrelated catalogs drive the same bridge through the same
// DTOs; the bridge itself never learns which package is asking.
func TestDecisionBridgeServesTwoPackagesWithOneProtocol(t *testing.T) {
	packages := []struct {
		name       string
		definition DecisionDefinition
		answer     string
		invalid    string
	}{
		{"continue", DecisionDefinition{SchemaVersion: DecisionDefinitionVersion, ID: "continue", Title: "Continue", Phase: "runtime", Choices: []DecisionChoice{{ID: "yes", Title: "Yes", Value: json.RawMessage(`true`)}, {ID: "no", Title: "No", Value: json.RawMessage(`false`)}}, Sensitivity: "ordinary", Destination: DecisionDestination{Kind: "session_context", Name: "continue"}}, `true`, `"maybe"`},
		{"reviewers", DecisionDefinition{SchemaVersion: DecisionDefinitionVersion, ID: "reviewers", Title: "Reviewer count", Phase: "runtime", Choices: []DecisionChoice{{ID: "one", Title: "One", Value: json.RawMessage(`1`)}, {ID: "two", Title: "Two", Value: json.RawMessage(`2`)}}, Sensitivity: "ordinary", Destination: DecisionDestination{Kind: "session_context", Name: "reviewers"}}, `2`, `3`},
	}
	for _, pkg := range packages {
		t.Run(pkg.name, func(t *testing.T) {
			catalog := DecisionCatalog{SchemaVersion: DecisionCatalogVersion, Decisions: []DecisionDefinition{pkg.definition}}
			catalogDigest, err := DecisionCatalogDigest(catalog)
			if err != nil {
				t.Fatal(err)
			}
			sheet := DecisionSheet{SchemaVersion: DecisionSheetVersion, CatalogDigest: catalogDigest, ProfileSource: "none", Records: []DecisionRecord{}}
			e, runID, _ := assistedWorkspaceFixtureWithDecisions(t, "", &catalog, &sheet)
			task := handOver(t, e, runID)
			if task.SchemaVersion != AssistedSessionDecisionVersion || !task.DecisionBridge {
				t.Fatalf("handoff does not declare the bridge: %+v", task)
			}
			definitionDigest, err := DecisionDefinitionDigest(pkg.definition)
			if err != nil {
				t.Fatal(err)
			}
			request := DecisionRequest{SchemaVersion: DecisionRequestVersion, RunID: runID, AttemptID: task.AttemptID, EnvelopeDigest: task.EnvelopeDigest, DecisionID: pkg.definition.ID, DefinitionDigest: definitionDigest, ExpectedRunVersion: task.RunVersion}
			submit := func(request DecisionRequest) error {
				_, err := e.SubmitSession(context.Background(), SessionSubmission{SchemaVersion: task.SchemaVersion, RunID: runID, AttemptID: task.AttemptID, EnvelopeDigest: task.EnvelopeDigest, DecisionRequest: &request})
				return err
			}
			if err := submit(request); err != nil {
				t.Fatal(err)
			}
			waiting, err := e.Next(context.Background(), runID)
			if err != nil || waiting.Action != "waiting_decision" {
				t.Fatalf("request did not park the Run: %+v %v", waiting, err)
			}
			// A duplicate request is refused whether it is stale or current, and
			// neither attempt at it changes what is pending.
			if err := submit(request); err == nil {
				t.Fatal("stale duplicate request was accepted")
			} else {
				rejectionCode(t, err, "decision_conflict")
			}
			current := request
			current.ExpectedRunVersion = waiting.RunVersion
			if err := submit(current); err == nil {
				t.Fatal("duplicate request was accepted while one is pending")
			} else {
				rejectionCode(t, err, "decision_pending")
			}
			// Driving a parked Run must not settle the attempt or open a successor.
			if err := e.Drive(context.Background(), runID); err != nil {
				t.Fatal(err)
			}
			parked, err := e.View(context.Background(), runID)
			if err != nil || parked.Run.PendingDecision == nil || len(parked.Run.Attempts) != 1 || len(parked.Run.Steps) != 1 {
				t.Fatalf("drive dispatched hidden work while waiting: attempts=%d steps=%d pending=%v err=%v", len(parked.Run.Attempts), len(parked.Run.Steps), parked.Run.PendingDecision != nil, err)
			}
			if again, err := e.Next(context.Background(), runID); err != nil || again.Action != "waiting_decision" || again.RunVersion != waiting.RunVersion {
				t.Fatalf("drive changed the parked Run: %+v %v", again, err)
			}
			requestDigest, err := DecisionRequestDigest(request)
			if err != nil {
				t.Fatal(err)
			}
			answer := DecisionAnswer{SchemaVersion: DecisionAnswerVersion, RunID: runID, DecisionID: pkg.definition.ID, DefinitionDigest: definitionDigest, RequestDigest: requestDigest, ExpectedRunVersion: waiting.RunVersion, Value: json.RawMessage(pkg.invalid)}
			if _, err := e.AnswerDecision(context.Background(), answer); err == nil {
				t.Fatal("answer outside the declared choices was accepted")
			} else {
				rejectionCode(t, err, "invalid_decision_answer")
			}
			answer.Value = json.RawMessage(pkg.answer)
			if _, err := e.AnswerDecision(context.Background(), answer); err != nil {
				t.Fatal(err)
			}
			resumed, err := e.SessionTask(context.Background(), runID, task.AttemptID)
			if err != nil || resumed.AttemptID != task.AttemptID || string(resumed.DecisionContext[pkg.definition.Destination.Name]) != pkg.answer {
				t.Fatalf("answer did not redeliver the same attempt with the declared value: %+v %v", resumed, err)
			}
			if _, err := e.SubmitSession(context.Background(), hostResult(t, e, resumed, "resumed after "+pkg.name)); err != nil {
				t.Fatal(err)
			}
			if err := e.Drive(context.Background(), runID); err != nil {
				t.Fatal(err)
			}
			settled, err := e.View(context.Background(), runID)
			if err != nil || settled.Run.PendingDecision != nil || len(settled.Run.DecisionLedger) != 1 || settled.Run.DecisionLedger[0].Status != "answered" || settled.Run.DecisionLedger[0].SchemaVersion != DecisionRecordVersion {
				t.Fatalf("ledger did not settle through the shared record contract: %+v %v", settled.Run.DecisionLedger, err)
			}
			// A late answer names the current version but no pending request.
			late := answer
			late.ExpectedRunVersion = settled.RunVersion
			if _, err := e.AnswerDecision(context.Background(), late); err == nil {
				t.Fatal("late answer was accepted after the decision settled")
			} else {
				rejectionCode(t, err, "decision_not_pending")
			}
			if after, err := e.View(context.Background(), runID); err != nil || after.RunVersion != settled.RunVersion || len(after.Run.DecisionLedger) != 1 {
				t.Fatalf("late answer changed the Run: %+v %v", after.Run.DecisionLedger, err)
			}
		})
	}
}

// The catalog declares obligation only where it is enforced. A runtime decision
// marked required reads as a gate in the questionnaire, while the authority
// accepts a report whose executor never raised the request.
func TestRuntimeDecisionCannotBeRequired(t *testing.T) {
	definition := DecisionDefinition{
		SchemaVersion: DecisionDefinitionVersion, ID: "commit_grouping", Title: "Commit grouping",
		Phase: "runtime", Choices: []DecisionChoice{{ID: "follow", Title: "Follow", Value: json.RawMessage(`"follow"`)}},
		Sensitivity: "ordinary", Destination: DecisionDestination{Kind: "session_context", Name: "commit_grouping"},
	}
	if err := ValidateDecisionDefinition(definition); err != nil {
		t.Fatalf("an ordinary runtime decision was refused: %v", err)
	}
	definition.Required = true
	if err := ValidateDecisionDefinition(definition); refusalCode(err) != "decision_required_unenforceable" {
		t.Fatalf("a runtime decision was allowed to promise a gate: %v", err)
	}
	preflight := definition
	preflight.Phase, preflight.Required = "preflight", true
	preflight.Recommendation = json.RawMessage(`"follow"`)
	if err := ValidateDecisionDefinition(preflight); err != nil {
		t.Fatalf("a required preflight decision was refused: %v", err)
	}
}
