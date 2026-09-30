package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

func questionsFixture(t *testing.T) (*Engine, string, SessionTask) {
	t.Helper()
	apply := DecisionDefinition{SchemaVersion: DecisionDefinitionVersion, ID: "improve_apply", Title: "Apply the proposed improvements", Phase: "preflight", Required: true, Choices: []DecisionChoice{{ID: "all", Title: "All", Value: json.RawMessage(`"all"`)}}, Sensitivity: "ordinary", Destination: DecisionDestination{Kind: "session_context", Name: "improve_apply"}}
	catalog := DecisionCatalog{SchemaVersion: DecisionCatalogVersion, Decisions: []DecisionDefinition{apply}}
	catalogDigest, err := DecisionCatalogDigest(catalog)
	if err != nil {
		t.Fatal(err)
	}
	applyDigest, err := DecisionDefinitionDigest(apply)
	if err != nil {
		t.Fatal(err)
	}
	sheet := DecisionSheet{SchemaVersion: DecisionSheetVersion, CatalogDigest: catalogDigest, PackageProfile: "fast", ProfileSource: "actor", Records: []DecisionRecord{
		{SchemaVersion: DecisionRecordVersion, DefinitionID: apply.ID, DefinitionDigest: applyDigest, Status: "answered", Source: "project_default", Value: json.RawMessage(`"all"`)},
	}}
	e, runID, _ := assistedWorkspaceFixtureWithDecisions(t, "", &catalog, &sheet)
	return e, runID, handOver(t, e, runID)
}

// A native question of a skill never reaches this authority. The host says
// which questions its step met and what the work went on with, and the Run
// keeps that statement on the attempt -- apart from the journal of what the
// authority itself delivered and accepted.
func TestTheReportCarriesTheQuestionsTheStepAnswered(t *testing.T) {
	t.Parallel()
	e, runID, task := questionsFixture(t)
	ctx := context.Background()
	if task.QuestionReport != QuestionReportRequired {
		t.Fatalf("the task does not tell the host the list is owed: %q", task.QuestionReport)
	}
	journal := len(driverRun(t, e, runID).DecisionLedger)
	refused := []struct {
		name      string
		questions *[]AnsweredQuestion
		code      string
		path      string
	}{
		{"absent", nil, "answered_questions_missing", "/answered_questions"},
		{"blank question", &[]AnsweredQuestion{{Question: " ", Answer: "all", Basis: "judgement"}}, "answered_questions_invalid", "/answered_questions/0/question"},
		{"unknown basis", &[]AnsweredQuestion{{Question: "Apply these improvements?", Answer: "all", Basis: "guess"}}, "answered_questions_invalid", "/answered_questions/0/basis"},
		{"decision the Run does not declare", &[]AnsweredQuestion{{Question: "Apply these improvements?", Answer: "all", Basis: "decision", DecisionID: "gate_warnings"}}, "answered_questions_invalid", "/answered_questions/0/decision_id"},
		{"decision without its id", &[]AnsweredQuestion{{Question: "Apply these improvements?", Answer: "all", Basis: "decision"}}, "answered_questions_invalid", "/answered_questions/0/decision_id"},
		{"input the step does not have", &[]AnsweredQuestion{{Question: "Where does work start?", Answer: "app/", Basis: "input", Port: "handoff"}}, "answered_questions_invalid", "/answered_questions/0/port"},
		{"a reference beside another basis", &[]AnsweredQuestion{{Question: "Which one?", Answer: "the first", Basis: "judgement", Port: "handoff"}}, "answered_questions_invalid", "/answered_questions/0/port"},
	}
	for _, c := range refused {
		submission := hostResult(t, e, task, "planned")
		submission.AnsweredQuestions = c.questions
		_, err := e.SubmitSession(ctx, submission)
		problem, ok := err.(*flow.Problem)
		if !ok || problem.Code != c.code || problem.Path != c.path {
			t.Fatalf("%s: refused as %v, want %s at %s", c.name, err, c.code, c.path)
		}
	}
	missing := hostResult(t, e, task, "planned")
	missing.AnsweredQuestions = nil
	if _, err := e.SubmitSession(ctx, missing); err == nil || !strings.Contains(err.Error(), "project runners update") {
		t.Fatalf("the refusal does not say how a host with old instructions gets new ones: %v", err)
	}

	answered := []AnsweredQuestion{
		{Question: "Apply these improvements?", AskedBy: "aif-improve step 5", Options: []string{"all", "select", "none"}, Answer: "all", Basis: "decision", DecisionID: "improve_apply"},
		{Question: "Which test runner does the project use?", Answer: "phpunit, as the gate script names", Basis: "instructions"},
		{Question: "Split the migration into two?", Answer: "no, one change is smaller to review", Basis: "judgement"},
	}
	submission := hostResult(t, e, task, "planned")
	submission.AnsweredQuestions = &answered
	if _, err := e.SubmitSession(ctx, submission); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	report := r.Attempts[task.AttemptID].QuestionReport
	if report == nil || report.SchemaVersion != QuestionReportVersion || len(report.Questions) != 3 || report.Questions[0].DecisionID != "improve_apply" || report.Questions[2].Basis != "judgement" || report.Reported.UTC == "" {
		t.Fatalf("the attempt does not keep what the host said: %+v", report)
	}
	if len(r.DecisionLedger) != journal {
		t.Fatalf("a question the host answered became a decision record: %d -> %d", journal, len(r.DecisionLedger))
	}
	if err := validateInBundle(t, questionPublicContracts, "CoreRunStateV41", r); err != nil {
		t.Fatalf("the published state rejects the kept report: %v", err)
	}
	if err := validateInBundle(t, questionPublicContracts, "SessionSubmissionV7", submission); err != nil {
		t.Fatalf("the published submission rejects the report: %v", err)
	}
}

// An empty list is a statement, and it is kept as one: a reader must be able to
// tell "the host said there were none" from "nobody said anything".
func TestAnEmptyListIsKeptAsNoQuestions(t *testing.T) {
	t.Parallel()
	e, runID, task := questionsFixture(t)
	submission := hostResult(t, e, task, "planned")
	submission.AnsweredQuestions = &[]AnsweredQuestion{}
	if _, err := e.SubmitSession(context.Background(), submission); err != nil {
		t.Fatal(err)
	}
	report := driverRun(t, e, runID).Attempts[task.AttemptID].QuestionReport
	if report == nil || report.Questions == nil || len(report.Questions) != 0 {
		t.Fatalf("an empty list was not kept as the host's statement: %+v", report)
	}
	encoded, err := json.Marshal(report)
	if err != nil || !strings.Contains(string(encoded), `"questions":[]`) {
		t.Fatalf("the stored statement reads as absent: %s %v", encoded, err)
	}
}

// A decision request is not the end of an attempt: its questions come with the
// report after the answer is redelivered.
func TestADecisionRequestCarriesNoQuestions(t *testing.T) {
	t.Parallel()
	e, runID, task := questionsFixture(t)
	request := DecisionRequest{SchemaVersion: DecisionRequestVersion, RunID: runID, AttemptID: task.AttemptID, EnvelopeDigest: task.EnvelopeDigest, DecisionID: "improve_apply"}
	_, err := e.SubmitSession(context.Background(), SessionSubmission{SchemaVersion: task.SchemaVersion, RunID: runID, AttemptID: task.AttemptID, EnvelopeDigest: task.EnvelopeDigest, DecisionRequest: &request, AnsweredQuestions: &[]AnsweredQuestion{}})
	if problem, ok := err.(*flow.Problem); !ok || problem.Path != "/decision_request" {
		t.Fatalf("a decision request carrying questions was not refused: %v", err)
	}
}

// A Run sealed before 41 keeps the report it always had: the list is neither
// owed nor accepted there.
func TestARunSealedBeforeQuestionsNeitherOwesNorTakesThem(t *testing.T) {
	t.Parallel()
	step := flow.StepDefinition{Inputs: map[string]flow.InputPort{"handoff": {}}}
	earlier := Run{SchemaVersion: CoreContinuationStateVersion}
	if err := checkAnsweredQuestions(earlier, step, nil); err != nil {
		t.Fatalf("an earlier Run was held to the list: %v", err)
	}
	if err := checkAnsweredQuestions(earlier, step, &[]AnsweredQuestion{}); refusalCode(err) != "answered_questions_unsupported" {
		t.Fatalf("an earlier Run took a list it has no place for: %v", err)
	}
	current := Run{SchemaVersion: CoreQuestionStateVersion}
	if err := checkAnsweredQuestions(current, step, &[]AnsweredQuestion{{Question: "Where does work start?", Answer: "app/Services", Basis: "input", Port: "handoff"}}); err != nil {
		t.Fatalf("an answer resting on an input the step has was refused: %v", err)
	}
	many := make([]AnsweredQuestion, MaxAnsweredQuestions+1)
	for index := range many {
		many[index] = AnsweredQuestion{Question: "q", Answer: "a", Basis: "judgement"}
	}
	if err := checkAnsweredQuestions(current, step, &many); refusalCode(err) != "answered_questions_invalid" {
		t.Fatalf("a list over the bound was taken: %v", err)
	}
}

// Reading a task writes nothing, so taking it is its own command. The task
// names it until it has run, a second take changes nothing, and a reported task
// has nothing left to take.
func TestTakingATaskIsRecordedOnce(t *testing.T) {
	t.Parallel()
	e, runID, task := questionsFixture(t)
	ctx := context.Background()
	if task.TakeCommand != "session take --run "+runID+" --attempt "+task.AttemptID {
		t.Fatalf("the task does not name the command that takes it: %q", task.TakeCommand)
	}
	if _, err := e.TakeSession(ctx, runID, task.AttemptID); err != nil {
		t.Fatal(err)
	}
	first := *driverRun(t, e, runID).Attempts[task.AttemptID].Session.Taken
	if _, err := e.TakeSession(ctx, runID, task.AttemptID); err != nil {
		t.Fatalf("a second take was refused: %v", err)
	}
	if again := *driverRun(t, e, runID).Attempts[task.AttemptID].Session.Taken; again != first {
		t.Fatalf("a second take moved the first: %+v -> %+v", first, again)
	}
	if retaken := handOver(t, e, runID); retaken.TakeCommand != "" {
		t.Fatalf("a taken task still asks to be taken: %q", retaken.TakeCommand)
	}
	if _, err := e.SubmitSession(ctx, hostResult(t, e, task, "planned")); err != nil {
		t.Fatal(err)
	}
	if result, err := e.TakeSession(ctx, runID, task.AttemptID); err != nil || !result.Duplicate {
		t.Fatalf("a repeated take after the report was not the first one's receipt: %+v %v", result, err)
	}
	view, err := e.View(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if pickup := timingFind(t, view.Timing.Root, task.AttemptID).Metrics["host_pickup"]; pickup.Quality == "unavailable" {
		t.Fatalf("the recorded take gave no pickup time: %+v", pickup)
	}
}

func TestAReportedTaskThatWasNeverTakenCannotBeTakenLater(t *testing.T) {
	t.Parallel()
	e, runID, task := questionsFixture(t)
	ctx := context.Background()
	if _, err := e.SubmitSession(ctx, hostResult(t, e, task, "planned")); err != nil {
		t.Fatal(err)
	}
	if _, err := e.TakeSession(ctx, runID, task.AttemptID); refusalCode(err) != "session_state_conflict" {
		t.Fatalf("a take after the report was recorded: %v", err)
	}
}

// A field of the report put inside result was refused as a bare schema_invalid
// with no pointer; the refusal names the field and where it belongs.
func TestAReportFieldInsideResultIsNamed(t *testing.T) {
	t.Parallel()
	e, _, task := questionsFixture(t)
	for _, name := range reportLevelFields {
		submission := hostResult(t, e, task, "planned")
		var result map[string]any
		if err := json.Unmarshal(submission.Result, &result); err != nil {
			t.Fatal(err)
		}
		result[name] = []any{}
		body, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		submission.Result = body
		_, err = e.SubmitSession(context.Background(), submission)
		problem, ok := err.(*flow.Problem)
		if !ok || problem.Path != "/result/"+name || !strings.Contains(problem.Message, "top level") {
			t.Fatalf("%s inside result was refused as %v", name, err)
		}
	}
}
