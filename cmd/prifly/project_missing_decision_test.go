package main

import (
	"encoding/json"
	"strings"
	"testing"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// A dependent session met "project_start_missing_decision: plan_tests" with an
// attended policy. The refusal was correct and useless: plan_tests is declared
// automatic with a recommendation, so --decision-policy autonomous would have
// answered it, and the text said neither that nor that five more decisions were
// waiting behind it. They found the flag in their own notes, not in the refusal.
//
// The two exits are not interchangeable, which is why the text is built from the
// decisions rather than fixed: a sensitive decision is never answered by policy,
// and naming the policy there would send the reader in a circle.
func TestMissingDecisionRefusalNamesEveryDecisionAndTheExitThatFitsIt(t *testing.T) {
	automatic := func(id string) prifly.DecisionDefinition {
		return prifly.DecisionDefinition{ID: id, Phase: "preflight", Required: true, Automatic: true, Sensitivity: "ordinary", Recommendation: json.RawMessage(`true`)}
	}
	catalog := []prifly.DecisionDefinition{
		automatic("plan_tests"),
		automatic("gate_checks"),
		{ID: "release_scope", Phase: "preflight", Required: true, Automatic: false, Sensitivity: "ordinary", Recommendation: json.RawMessage(`true`)},
		{ID: "publish_now", Phase: "preflight", Required: true, Automatic: true, Sensitivity: "sensitive", Recommendation: json.RawMessage(`true`)},
		{ID: "no_advice", Phase: "preflight", Required: true, Automatic: true, Sensitivity: "ordinary"},
		{ID: "optional_one", Phase: "preflight", Required: false, Automatic: true, Sensitivity: "ordinary", Recommendation: json.RawMessage(`true`)},
		{ID: "later", Phase: "runtime", Required: true, Automatic: true, Sensitivity: "ordinary", Recommendation: json.RawMessage(`true`)},
	}
	answers := map[string]json.RawMessage{}

	attended := projectMissingDecisionRefusal(catalog, "", answers, "attended")
	if !strings.Contains(attended, "5 declared decisions are unanswered") {
		t.Fatalf("the count is wrong or missing -- optional and runtime decisions must not be counted: %s", attended)
	}
	for _, id := range []string{"plan_tests", "gate_checks", "release_scope", "publish_now", "no_advice"} {
		if !strings.Contains(attended, id) {
			t.Fatalf("%s is unanswered and unnamed; a caller would learn it one refusal at a time: %s", id, attended)
		}
	}
	for _, id := range []string{"optional_one", "later"} {
		if strings.Contains(attended, id) {
			t.Fatalf("%s does not block this start and was named anyway: %s", id, attended)
		}
	}
	policyPart, handPart, split := strings.Cut(attended, "must be answered with --preflight-answer")
	if !split {
		t.Fatalf("the refusal never names the explicit exit: %s", attended)
	}
	if !strings.Contains(policyPart, "--decision-policy autonomous") {
		t.Fatalf("two decisions are answerable by policy and the policy is not named: %s", attended)
	}
	for _, id := range []string{"release_scope", "publish_now", "no_advice"} {
		if !strings.Contains(policyPart[strings.Index(policyPart, "unanswered."):], id) && !strings.Contains(handPart, id) && !strings.Contains(policyPart, id) {
			t.Fatalf("%s is not answerable by policy and must be listed with the explicit exit: %s", id, attended)
		}
	}
	// Under an autonomous policy the same call is refused only by the three that
	// no policy answers, and offering the policy again would be a circle.
	autonomous := projectMissingDecisionRefusal(catalog, "", answers, "autonomous")
	if strings.Contains(autonomous, "--decision-policy autonomous") {
		t.Fatalf("the refusal advises the policy that is already in force: %s", autonomous)
	}
	if !strings.Contains(autonomous, "release_scope") || !strings.Contains(autonomous, "--preflight-answer") {
		t.Fatalf("the autonomous refusal does not name what only a person can answer: %s", autonomous)
	}
	// Answering one of them removes it from the list rather than from the count.
	answers["plan_tests"] = json.RawMessage(`true`)
	after := projectMissingDecisionRefusal(catalog, "", answers, "attended")
	if !strings.Contains(after, "4 declared decisions are unanswered") || strings.Contains(after, "plan_tests") {
		t.Fatalf("an answered decision still blocks or is still named: %s", after)
	}
	if !strings.Contains(after, "project questionnaire") {
		t.Fatalf("the refusal does not name the command that lists every question: %s", after)
	}
}
