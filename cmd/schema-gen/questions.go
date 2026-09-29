package main

import (
	"reflect"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// questionField names what only state/read 41 carries: the report of the
// questions a step answered, the task's demand for it, and when a runtime
// decision was requested. Bundles published before it describe none of them.
func questionField(t reflect.Type, name string) bool {
	switch {
	case t == reflect.TypeFor[prifly.Attempt]() && name == "QuestionReport":
		return true
	case t == reflect.TypeFor[prifly.SessionSubmission]() && name == "AnsweredQuestions":
		return true
	case t == reflect.TypeFor[prifly.SessionTask]() && (name == "QuestionReport" || name == "TakeCommand"):
		return true
	case t == reflect.TypeFor[prifly.SessionHandoff]() && name == "Taken":
		return true
	}
	return t == reflect.TypeFor[prifly.DecisionRecord]() && name == "Requested"
}

func questionConstraints(g *generator) {
	g.property("runtime_Run", "schema_version", map[string]any{"const": prifly.CoreQuestionStateVersion})
	g.property("runtime_RunView", "schema_version", map[string]any{"const": prifly.CoreQuestionReadVersion})
	text := func(limit int) map[string]any {
		return map[string]any{"type": "string", "minLength": 1, "maxLength": limit}
	}
	g.property("runtime_AnsweredQuestion", "question", text(2000))
	g.property("runtime_AnsweredQuestion", "answer", text(4000))
	g.property("runtime_AnsweredQuestion", "asked_by", text(200))
	g.property("runtime_AnsweredQuestion", "options", map[string]any{"type": "array", "maxItems": 16, "items": text(200)})
	g.property("runtime_AnsweredQuestion", "basis", enum(prifly.QuestionBases...))
	g.defs["runtime_AnsweredQuestion"].(map[string]any)["allOf"] = []any{
		map[string]any{"if": map[string]any{"properties": map[string]any{"basis": map[string]any{"const": "decision"}}}, "then": map[string]any{"required": []string{"decision_id"}}, "else": map[string]any{"not": map[string]any{"required": []string{"decision_id"}}}},
		map[string]any{"if": map[string]any{"properties": map[string]any{"basis": map[string]any{"const": "input"}}}, "then": map[string]any{"required": []string{"port"}}, "else": map[string]any{"not": map[string]any{"required": []string{"port"}}}},
	}
	g.property("runtime_QuestionReport", "schema_version", map[string]any{"const": prifly.QuestionReportVersion})
	g.property("runtime_QuestionReport", "questions", map[string]any{"type": "array", "maxItems": prifly.MaxAnsweredQuestions, "items": map[string]any{"$ref": "#/$defs/runtime_AnsweredQuestion"}})
	g.property("runtime_SessionTask", "question_report", map[string]any{"const": prifly.QuestionReportRequired})
	g.describe("runtime_SessionTask", "question_report", "required: the report must carry answered_questions -- every question the step met during this attempt with its answer and basis, or an empty list when there was none. Absent in a Run sealed before 41.")
	g.describe("runtime_SessionSubmission", "answered_questions", "The questions the step met during this attempt, native questions of a skill included, each with the answer the work went on with and its basis: decision (decision_id of a decision this Run declares), input (port of this step), instructions, person, judgement. An empty list says there were none; absent is refused where the task requires the list.")
	g.describe("runtime_Attempt", "question_report", "What the host said about the questions its step answered during this attempt, as given with the report. The host's statement, kept apart from the decision journal: not a DecisionRecord, not an Approval, not evidence of who answered.")
	g.describe("runtime_SessionTask", "take_command", "The command that records the host taking this task -- run it once when you start the work. Until it runs, the time the task waited for a host cannot be told from the host's own time. Absent once taken and in a Run sealed before 41.")
	g.describe("runtime_SessionHandoff", "taken", "When the host said it took this task, from session take. Kept once: a redelivery after an answer does not move it.")
	g.describe("runtime_DecisionRecord", "requested", "When an attempt asked this runtime decision. Recorded from state 41 so the wait for the answer can be measured; absent on answers given before the Run started.")
}
