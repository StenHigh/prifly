package runtime

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/stenhigh/prifly/internal/flow"
)

// QuestionReportVersion is the stored record, not the session contract.
const QuestionReportVersion = "question-report/1"

// QuestionReportRequired is the value a task carries when its report must
// list the questions the step answered.
const QuestionReportRequired = "required"

const (
	MaxAnsweredQuestions  = 32
	maxQuestionRunes      = 2000
	maxAnswerRunes        = 4000
	maxAskedByRunes       = 200
	maxQuestionOptions    = 16
	maxQuestionOptionRune = 200
)

// QuestionBases are what an answer may rest on, in the order an operator
// checks them: something the Run sealed, something the step was handed, what
// its instructions say, a person in the session, the executor's own judgement.
var QuestionBases = []string{"decision", "input", "instructions", "person", "judgement"}

// AnsweredQuestion is one question the step met during an attempt and the
// answer it went on with. It is the host's statement: a native question of a
// skill never reaches this authority, so nothing here is observed, only told.
type AnsweredQuestion struct {
	Question string   `json:"question"`
	AskedBy  string   `json:"asked_by,omitempty"`
	Options  []string `json:"options,omitempty"`
	Answer   string   `json:"answer"`
	Basis    string   `json:"basis"`
	// DecisionID names the Run's declared decision an answer came from.
	// Required with basis decision and refused otherwise.
	DecisionID string `json:"decision_id,omitempty"`
	// Port names the step input an answer came from. Required with basis
	// input and refused otherwise.
	Port string `json:"port,omitempty"`
}

// QuestionReport is the list as the host gave it with the report. It is not a
// DecisionRecord: the decision journal holds what this authority delivered and
// accepted, and a question the host answered for itself is neither.
type QuestionReport struct {
	SchemaVersion string             `json:"schema_version"`
	Questions     []AnsweredQuestion `json:"questions"`
	Reported      Observation        `json:"reported"`
}

func questionProblem(code, path, message string) error {
	return &flow.Problem{Code: code, Path: path, Message: message}
}

const answeredQuestionsShape = "answered_questions is a list, empty when the step met no question; each entry has question, answer and basis (one of decision, input, instructions, person, judgement), decision_id with basis decision, port with basis input, and optionally asked_by and options"

// checkAnsweredQuestions holds a report to what this authority can check: the
// list exists where the Run requires it, each entry has a shape an operator
// can read, and a reference names a decision or input the step really had.
// Whether the host listed every question it met is beyond it.
func checkAnsweredQuestions(r Run, step flow.StepDefinition, questions *[]AnsweredQuestion) error {
	if !isQuestionState(r.SchemaVersion) {
		if questions != nil {
			return questionProblem("answered_questions_unsupported", "/answered_questions", "this Run was sealed before reports listed their questions; send the report without answered_questions")
		}
		return nil
	}
	if questions == nil {
		return questionProblem("answered_questions_missing", "/answered_questions", "this Run records the questions each step answered, so the report must carry them: "+answeredQuestionsShape+". A host whose prifly-run instructions do not mention it is out of date: project runners update --repository .")
	}
	if len(*questions) > MaxAnsweredQuestions {
		return questionProblem("answered_questions_invalid", "/answered_questions", fmt.Sprintf("a report lists at most %d questions", MaxAnsweredQuestions))
	}
	for index, q := range *questions {
		path := fmt.Sprintf("/answered_questions/%d", index)
		if err := boundedText(path+"/question", q.Question, maxQuestionRunes, true); err != nil {
			return err
		}
		if err := boundedText(path+"/answer", q.Answer, maxAnswerRunes, true); err != nil {
			return err
		}
		if err := boundedText(path+"/asked_by", q.AskedBy, maxAskedByRunes, false); err != nil {
			return err
		}
		if len(q.Options) > maxQuestionOptions {
			return questionProblem("answered_questions_invalid", path+"/options", fmt.Sprintf("a question lists at most %d options", maxQuestionOptions))
		}
		for option, text := range q.Options {
			if err := boundedText(fmt.Sprintf("%s/options/%d", path, option), text, maxQuestionOptionRune, true); err != nil {
				return err
			}
		}
		if !slices.Contains(QuestionBases, q.Basis) {
			return questionProblem("answered_questions_invalid", path+"/basis", "basis is one of "+strings.Join(QuestionBases, ", "))
		}
		if q.Basis == "decision" {
			if _, exists := decisionDefinition(r.DecisionCatalog, q.DecisionID); !exists {
				return questionProblem("answered_questions_invalid", path+"/decision_id", "basis decision names a decision this Run declares; "+declaredDecisionIDs(r.DecisionCatalog))
			}
		} else if q.DecisionID != "" {
			return questionProblem("answered_questions_invalid", path+"/decision_id", "decision_id belongs to basis decision")
		}
		if q.Basis == "input" {
			if _, exists := step.Inputs[q.Port]; !exists {
				return questionProblem("answered_questions_invalid", path+"/port", "basis input names an input of this step; "+stepInputNames(step))
			}
		} else if q.Port != "" {
			return questionProblem("answered_questions_invalid", path+"/port", "port belongs to basis input")
		}
	}
	return nil
}

func boundedText(path, text string, limit int, required bool) error {
	if required && strings.TrimSpace(text) == "" {
		return questionProblem("answered_questions_invalid", path, "this field is required and cannot be blank")
	}
	if !utf8.ValidString(text) || utf8.RuneCountInString(text) > limit {
		return questionProblem("answered_questions_invalid", path, fmt.Sprintf("this field is UTF-8 text of at most %d characters", limit))
	}
	return nil
}

func declaredDecisionIDs(catalog *DecisionCatalog) string {
	if catalog == nil || len(catalog.Decisions) == 0 {
		return "this Run declares none, so no answer rests on one"
	}
	ids := make([]string, 0, len(catalog.Decisions))
	for _, definition := range catalog.Decisions {
		ids = append(ids, definition.ID)
	}
	return "it declares " + strings.Join(ids, ", ")
}

func stepInputNames(step flow.StepDefinition) string {
	if len(step.Inputs) == 0 {
		return "this step has none, so no answer rests on one"
	}
	names := make([]string, 0, len(step.Inputs))
	for name := range step.Inputs {
		names = append(names, name)
	}
	slices.Sort(names)
	return "its inputs are " + strings.Join(names, ", ")
}
