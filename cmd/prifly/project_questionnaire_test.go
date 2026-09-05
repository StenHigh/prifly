package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func projectQuestionnaireFixture(t *testing.T) (string, string) {
	t.Helper()
	root, err := canonicalProjectPath(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	authority := filepath.Join(t.TempDir(), "never-created-authority")
	const folder = ".prifly/workflows/questions/"
	writeFixtureFile(t, root, ".prifly/project.yaml", `schema_version: prifly-project-profile/3
packages: {questions: {source: .prifly/workflows/questions}}
launches:
  questions:
    title: Declared questions
    description: Read-only typed questionnaire fixture.
    kind: workflow
    workflow: .prifly/workflows/questions/workflow.yaml
`)
	workflow := strings.Replace(fixtureWorkflowYAML("questions"), "  references:", "  profiles:\n    default: fast\n    values: {fast: {detail: short}, full: {detail: long}}\n  references:", 1)
	var paths []string
	for _, decision := range []struct{ id, source string }{
		{"profile", `title: Package profile
phase: preflight
choices: [{id: fast, title: Fast, value: fast}, {id: full, title: Full, value: full}]
destination: {kind: package_profile}
`},
		{"gate", `title: Inspect optional details
phase: preflight
choices: [{id: "yes", title: "Yes", value: true}, {id: "no", title: "No", value: false}]
automatic: true
recommendation: true
destination: {kind: session_context, name: gate}
`},
		{"detail", `title: Select extra detail
phase: preflight
required: false
choices: [{id: "yes", title: "Yes", value: true}, {id: "no", title: "No", value: false}]
destination: {kind: session_context, name: detail}
`},
		{"plain", `title: Accept the inspection
phase: runtime
choices: [{id: "yes", title: "Yes", value: true}, {id: "no", title: "No", value: false}]
sensitivity: scope-changing
destination: {kind: session_context, name: plain}
`},
		{"retry", `title: Retry count
phase: runtime
value_schema: {type: integer, minimum: 1, maximum: 3}
destination: {kind: session_context, name: retry}
when: {answers: {gate: true}}
`},
		{"both", `title: Conditional follow-up
phase: runtime
choices: [{id: "yes", title: "Yes", value: true}, {id: "no", title: "No", value: false}]
destination: {kind: session_context, name: both}
when: {answers: {gate: true, detail: true}}
`},
		{"full", `title: Full profile follow-up
phase: runtime
choices: [{id: "yes", title: "Yes", value: true}, {id: "no", title: "No", value: false}]
destination: {kind: session_context, name: full}
when: {answers: {profile: full}}
`},
	} {
		path := folder + "decisions/" + decision.id + ".yaml"
		paths = append(paths, path)
		writeFixtureFile(t, root, path, "authoring: prifly-run-decision/1\nid: "+decision.id+"\n"+decision.source)
	}
	workflow = strings.Replace(workflow, "entry: done", "decision_catalog: ["+strings.Join(paths, ", ")+"]\nentry: done", 1)
	writeFixtureFile(t, root, folder+"workflow.yaml", workflow)
	writeFixtureFile(t, root, folder+"extend.yaml", "extensions: []\n")
	return root, authority
}

func questionnaireDecisionState(t *testing.T, result projectQuestionnaire, id, applicability, wait string, answered bool) {
	t.Helper()
	for _, state := range result.DecisionStates {
		if state.DecisionID == id {
			if state.Applicability != applicability || state.WaitReason != wait || state.Answered != answered {
				t.Fatalf("decision %s: %+v", id, state)
			}
			return
		}
	}
	t.Fatalf("decision %s missing from state inventory", id)
}

func TestCLIProjectQuestionnaireTypedSelections(t *testing.T) {
	// A questionnaire needs neither an authority nor a Git executable.
	t.Setenv("PATH", t.TempDir())
	root, authority := projectQuestionnaireFixture(t)
	for _, test := range []struct {
		name  string
		args  []string
		check func(*testing.T, projectQuestionnaire)
	}{
		{"optional-runtime", []string{"--preflight-answer", "gate=false"}, func(t *testing.T, result projectQuestionnaire) {
			questionnaireDecisionState(t, result, "plain", "applicable", "owner_answer_if_requested", false)
			questionnaireDecisionState(t, result, "retry", "inactive", "", false)
			questionnaireDecisionState(t, result, "both", "inactive", "", false)
			if len(result.Runtime) != 1 || result.Runtime[0].ID != "plain" || result.Runtime[0].Required {
				t.Fatalf("inactive questions escaped filtering or runtime became required: %+v", result.Runtime)
			}
		}},
		{"typed-false-preanswer", []string{"--preflight-answer", "gate=false", "--runtime-answer", "plain=false"}, func(t *testing.T, result projectQuestionnaire) {
			questionnaireDecisionState(t, result, "plain", "applicable", "", true)
			for _, record := range result.DecisionSheet.Records {
				if record.DefinitionID == "plain" && string(record.Value) == "false" && record.Source == "actor" {
					return
				}
			}
			t.Fatal("false preanswer was lost or attributed to the automatic policy")
		}},
		{"unknown-predecessor", nil, func(t *testing.T, result projectQuestionnaire) {
			questionnaireDecisionState(t, result, "gate", "applicable", "required_before_start", false)
			questionnaireDecisionState(t, result, "retry", "conditional", "owner_answer_if_requested", false)
			questionnaireDecisionState(t, result, "both", "conditional", "owner_answer_if_requested", false)
		}},
		{"known-true-and-missing", []string{"--preflight-answer", "gate=true"}, func(t *testing.T, result projectQuestionnaire) {
			questionnaireDecisionState(t, result, "retry", "applicable", "owner_answer_if_requested", false)
			questionnaireDecisionState(t, result, "both", "conditional", "owner_answer_if_requested", false)
		}},
		{"autonomous-default-dependent", []string{"--decision-policy", "autonomous", "--runtime-answer", "retry=2"}, func(t *testing.T, result projectQuestionnaire) {
			questionnaireDecisionState(t, result, "gate", "applicable", "", true)
			questionnaireDecisionState(t, result, "retry", "applicable", "", true)
			questionnaireDecisionState(t, result, "plain", "applicable", "automatic_selection_not_allowed", false)
			for _, record := range result.DecisionSheet.Records {
				if record.DefinitionID == "gate" && string(record.Value) == "true" && record.Source == "autonomous_policy" {
					return
				}
			}
			t.Fatal("required policy default was not included before its dependent preanswer")
		}},
		{"profile-predecessor", []string{"--package-profile", "full", "--preflight-answer", "gate=false", "--runtime-answer", "full=true"}, func(t *testing.T, result projectQuestionnaire) {
			questionnaireDecisionState(t, result, "profile", "applicable", "", true)
			questionnaireDecisionState(t, result, "full", "applicable", "", true)
			if result.DecisionSheet.PackageProfile != "full" || result.DecisionSheet.ProfileSource != "actor" {
				t.Fatalf("selected package profile was not pinned: %+v", result.DecisionSheet)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := append([]string{"--project", authority, "project", "questionnaire", "--repository", root, "--launch", "questions"}, test.args...)
			code, out, stderr := runCLI(t, args...)
			var result projectQuestionnaire
			if code != 0 || json.Unmarshal([]byte(out), &result) != nil || result.SchemaVersion != "project-questionnaire/3" || !result.KnownQuestionsOnly || result.CatalogDigest == "" {
				t.Fatalf("questionnaire selection: %d %s %s", code, out, stderr)
			}
			if len(result.DecisionStates) != 7 {
				t.Fatalf("state inventory lost inactive definitions: %+v", result.DecisionStates)
			}
			profile, err := readProjectProfile(root)
			if err != nil {
				t.Fatal(err)
			}
			source, err := readProjectWorkflowFolder(root, filepath.Join(root, ".prifly/workflows/questions"))
			if err != nil {
				t.Fatal(err)
			}
			catalog := prifly.DecisionCatalog{SchemaVersion: prifly.DecisionCatalogVersion, Decisions: source.DecisionCatalog}
			if err := prifly.ValidateDecisionSheet(catalog, result.DecisionSheet); err != nil {
				t.Fatalf("questionnaire bypassed Start's typed validation: %v", err)
			}
			if test.name == "typed-false-preanswer" {
				start, err := projectStartPreflight(root, profile, "questions", "", "attended", []string{"gate=false"}, []string{"plain=false"})
				if err != nil || !reflect.DeepEqual(start.Sheet, result.DecisionSheet) {
					t.Fatalf("questionnaire and Start resolved different answers: %v", err)
				}
			}
			test.check(t, result)
			if _, err := os.Lstat(authority); !os.IsNotExist(err) {
				t.Fatalf("read-only questionnaire created authority state: %v", err)
			}
		})
	}
}

func TestCLIProjectQuestionnaireRejectsInvalidSelections(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	root, authority := projectQuestionnaireFixture(t)
	for _, test := range []struct {
		name, refusal string
		args          []string
	}{
		{"wrong-type", "project_start_invalid_decision_answer", []string{"--runtime-answer", `plain="false"`}},
		{"unknown-runtime", "project_start_unknown_decision", []string{"--runtime-answer", "unknown=true"}},
		{"inactive-condition", "project_start_unknown_decision", []string{"--preflight-answer", "gate=false", "--runtime-answer", "retry=2"}},
		{"duplicate-runtime", "project_start_invalid_decision_answer", []string{"--runtime-answer", "plain=true", "--runtime-answer", "plain=false"}},
		{"stale-catalog", "project_start_stale_decision_catalog", []string{"--expected-decision-catalog-digest", "sha256:" + strings.Repeat("0", 64)}},
		{"profile-answer-route", "project_start_profile_is_selected_with_package_profile", []string{"--preflight-answer", `profile="full"`}},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := append([]string{"--project", authority, "project", "questionnaire", "--repository", root, "--launch", "questions"}, test.args...)
			code, out, stderr := runCLI(t, args...)
			if code == 0 || !strings.Contains(stderr, test.refusal) {
				t.Fatalf("wanted %s: %d %s %s", test.refusal, code, out, stderr)
			}
			if _, err := os.Lstat(authority); !os.IsNotExist(err) {
				t.Fatalf("invalid questionnaire created authority state: %v", err)
			}
		})
	}
}
