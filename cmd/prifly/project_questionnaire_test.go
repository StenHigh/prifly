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

// The identifier of a decision must be named `id` in all three lists of one
// response. Decoded Go values would agree even while the emitted JSON called
// it `decision_id` in `decision_states`, so this reads the published bytes:
// a reader splitting on `.id` got nine nulls instead of a refusal.
func questionnaireIdentifiersAgree(t *testing.T, out string) {
	t.Helper()
	type published struct {
		ID            string `json:"id"`
		Applicability string `json:"applicability"`
	}
	var response struct {
		Preflight []published `json:"preflight"`
		Runtime   []published `json:"runtime"`
		States    []published `json:"decision_states"`
	}
	if err := json.Unmarshal([]byte(out), &response); err != nil {
		t.Fatal(err)
	}
	applicable := map[string]bool{}
	for _, state := range response.States {
		if state.ID == "" {
			t.Fatalf("a decision_states entry has no `id`; preflight and runtime name it differently: %s", out)
		}
		if state.Applicability != "inactive" {
			applicable[state.ID] = true
		}
	}
	for _, list := range [][]published{response.Preflight, response.Runtime} {
		for _, listed := range list {
			if !applicable[listed.ID] {
				t.Fatalf("listed decision %q is not an applicable decision_states entry", listed.ID)
			}
			delete(applicable, listed.ID)
		}
	}
	if len(applicable) != 0 {
		t.Fatalf("applicable decisions missing from preflight and runtime: %v", applicable)
	}
}

func questionnaireDecisionState(t *testing.T, result projectQuestionnaire, id, applicability, wait string, answered bool) {
	t.Helper()
	for _, state := range result.DecisionStates {
		if state.ID == id {
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
			if code != 0 || json.Unmarshal([]byte(out), &result) != nil || result.SchemaVersion != "project-questionnaire/4" || !result.KnownQuestionsOnly || result.CatalogDigest == "" {
				t.Fatalf("questionnaire selection: %d %s %s", code, out, stderr)
			}
			if len(result.DecisionStates) != 7 {
				t.Fatalf("state inventory lost inactive definitions: %+v", result.DecisionStates)
			}
			questionnaireIdentifiersAgree(t, out)
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

// The refusal half cannot be reached through a real authority in a review test:
// this build is qualified for one to four attempts, so emptying the slots needs
// a live Run, and the review fixture exists to prove nothing is created. The
// decision is therefore its own function, and both branches are checked here.
// Until 0.13.10 there was no way to ask this at all -- capacity_conflict creates
// and queues a Run, so the question changed its own answer.
func TestAdmissionPreviewNamesTheRefusalOnlyWhenNoSlotIsFree(t *testing.T) {
	for _, test := range []struct {
		name          string
		capacity      int64
		held, waiting int
		available     int64
		refusal       string
	}{
		{"idle authority", 4, 0, 0, 4, ""},
		{"one slot left", 4, 3, 0, 1, ""},
		{"every slot taken", 4, 4, 2, 0, "capacity_conflict"},
		{"more held than the lowered capacity allows", 1, 3, 0, -2, "capacity_conflict"},
	} {
		t.Run(test.name, func(t *testing.T) {
			preview := projectAdmissionState(test.capacity, test.held, test.waiting)
			if preview.Capacity != test.capacity || preview.Held != int64(test.held) || preview.Waiting != int64(test.waiting) {
				t.Fatalf("the reading was not reported as read: %+v", preview)
			}
			if preview.Available != test.available || preview.WouldRefuse != test.refusal {
				t.Fatalf("got available=%d refusal=%q, want %d %q", preview.Available, preview.WouldRefuse, test.available, test.refusal)
			}
		})
	}
}

// A decision the owner settled for the project is not a choice to make at
// every launch. Until 0.13.23 five such answers lived in the operator's memory
// and a launcher's fixed flags: extend.yaml had a slot for the package profile
// and none for the policy or the answers. The standing answers are read where
// the profile is, validated where a flag is, recorded as project_default, and
// a flag overrides each one -- the questionnaire shows the same resolution
// Start seals.
func TestProjectStandingAnswersInExtendYAML(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	root, authority := projectQuestionnaireFixture(t)
	profile, err := readProjectProfile(root)
	if err != nil {
		t.Fatal(err)
	}
	const extend = ".prifly/workflows/questions/extend.yaml"
	writeFixtureFile(t, root, extend, "extensions: []\nanswers:\n  decision_policy: autonomous\n  preflight:\n    gate: false\n  runtime:\n    plain: true\n")
	record := func(sheet prifly.DecisionSheet, id string) (string, string) {
		for _, record := range sheet.Records {
			if record.DefinitionID == id {
				return string(record.Value), record.Source
			}
		}
		return "", ""
	}
	standing, err := projectStartPreflight(root, profile, "questions", "", "", nil, nil)
	if err != nil {
		t.Fatalf("standing answers were not accepted: %v", err)
	}
	if standing.Sheet.DecisionPolicy != "autonomous" {
		t.Fatalf("the standing policy was not read: %+v", standing.Sheet)
	}
	if value, source := record(standing.Sheet, "gate"); value != "false" || source != "project_default" {
		t.Fatalf("the standing preflight answer was not sealed as the project's: %q %q", value, source)
	}
	if value, source := record(standing.Sheet, "plain"); value != "true" || source != "project_default" {
		t.Fatalf("the standing runtime answer was not sealed as the project's: %q %q", value, source)
	}
	// A flag wins, and only over the answer it names.
	flagged, err := projectStartPreflight(root, profile, "questions", "", "attended", []string{"gate=true"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if flagged.Sheet.DecisionPolicy != "attended" {
		t.Fatalf("the flag did not override the standing policy: %+v", flagged.Sheet)
	}
	if value, source := record(flagged.Sheet, "gate"); value != "true" || source != "actor" {
		t.Fatalf("the flag did not override the standing answer: %q %q", value, source)
	}
	if value, source := record(flagged.Sheet, "plain"); value != "true" || source != "project_default" {
		t.Fatalf("an unnamed standing answer was lost beside a flag: %q %q", value, source)
	}
	// The read-only questionnaire resolves the same sheet Start seals.
	code, out, stderr := runCLI(t, "--project", authority, "project", "questionnaire", "--repository", root, "--launch", "questions")
	var result projectQuestionnaire
	if code != 0 || json.Unmarshal([]byte(out), &result) != nil {
		t.Fatalf("questionnaire with standing answers: %d %s %s", code, out, stderr)
	}
	if !reflect.DeepEqual(result.DecisionSheet, standing.Sheet) {
		t.Fatalf("questionnaire and Start resolved different standing answers:\n%+v\n%+v", result.DecisionSheet, standing.Sheet)
	}
	// A standing answer is validated like a flag, and the refusal names the
	// file to edit, not a flag to retype.
	for _, test := range []struct{ name, block, want string }{
		{"unknown", "  preflight:\n    nobody: true\n", "project_start_unknown_decision: nobody (from extend.yaml answers.preflight) is not declared"},
		{"wrong-phase", "  preflight:\n    plain: true\n", "plain is a runtime decision; pass it with extend.yaml answers.runtime, not extend.yaml answers.preflight"},
		{"invalid-value", "  runtime:\n    retry: 9\n  preflight:\n    gate: true\n", "project_start_invalid_decision_answer: retry (from extend.yaml answers.runtime): "},
		{"bad-policy", "  decision_policy: sometimes\n", "answers.decision_policy must be attended or autonomous"},
		{"unknown-field", "  defaults: {}\n", "answers has unknown field defaults"},
	} {
		writeFixtureFile(t, root, extend, "extensions: []\nanswers:\n"+test.block)
		_, err := projectStartPreflight(root, profile, "questions", "", "", nil, nil)
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("%s: standing answer refusal does not name its place: %v", test.name, err)
		}
	}
}
