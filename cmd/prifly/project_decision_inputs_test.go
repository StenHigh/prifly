package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func TestProjectDecisionInputs(t *testing.T) {
	for _, test := range []struct{ name, refusal string }{
		{"typed-value", ""},
		{"typed-null", ""},
		{"inactive", ""},
		{"unanswered", ""},
		{"explicit-value", "project_start_duplicate_input"},
		{"explicit-ref", "project_start_duplicate_input"},
		{"duplicate-target", "project_start_duplicate_input"},
		{"legacy", "project_start_unsupported_decision_input"},
		{"runtime", "project_start_unsupported_decision_input"},
	} {
		t.Run(test.name, func(t *testing.T) {
			selection := projectPreflight{
				Catalog: prifly.DecisionCatalog{Decisions: []prifly.DecisionDefinition{
					{ID: "value", Phase: "preflight", Destination: prifly.DecisionDestination{Kind: "launch_input", Name: "value"}, When: &prifly.DecisionCondition{Answers: map[string]json.RawMessage{"gate": json.RawMessage("true")}}},
				}},
				Sheet: prifly.DecisionSheet{Records: []prifly.DecisionRecord{
					{DefinitionID: "gate", Status: "answered", Value: json.RawMessage("true")},
					{DefinitionID: "value", Status: "answered", Value: json.RawMessage("2")},
				}},
			}
			inputs := map[string]json.RawMessage{}
			refs := map[string]prifly.ArtifactRef{}
			neutral := true
			want := "2"
			switch test.name {
			case "typed-null":
				selection.Sheet.Records[1].Value = json.RawMessage("null")
				want = "null"
			case "inactive":
				selection.Sheet.Records[0].Value = json.RawMessage("false")
				want = ""
			case "unanswered":
				selection.Sheet.Records = selection.Sheet.Records[:1]
				want = ""
			case "explicit-value":
				inputs["value"] = json.RawMessage("2")
			case "explicit-ref":
				refs["value"] = prifly.ArtifactRef{ArtifactID: "artifact:existing", Revision: 1}
			case "duplicate-target":
				other := selection.Catalog.Decisions[0]
				other.ID = "other"
				selection.Catalog.Decisions = append(selection.Catalog.Decisions, other)
				selection.Sheet.Records = append(selection.Sheet.Records, prifly.DecisionRecord{DefinitionID: "other", Status: "answered", Value: json.RawMessage("3")})
			case "legacy":
				neutral = false
			case "runtime":
				selection.Catalog.Decisions[0].Phase = "runtime"
			}
			before := map[string]json.RawMessage{}
			for port, value := range inputs {
				before[port] = append(json.RawMessage(nil), value...)
			}
			err := projectDecisionInputs(selection, inputs, refs, neutral)
			if test.refusal != "" {
				if err == nil || !strings.Contains(err.Error(), test.refusal) || !reflect.DeepEqual(before, inputs) {
					t.Fatalf("wanted atomic %s refusal: %v inputs=%v", test.refusal, err, inputs)
				}
				return
			}
			if err != nil || string(inputs["value"]) != want {
				t.Fatalf("decision input: %v value=%s want=%s", err, inputs["value"], want)
			}
			if want != "" {
				selection.Sheet.Records[1].Value[0] = 'x'
				if string(inputs["value"]) != want {
					t.Fatal("mapped value aliases mutable decision bytes")
				}
			}
		})
	}
}

func TestCLIProjectDecisionInputReachesRun(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	root, authority := projectQuestionnaireFixture(t)
	const folder = ".prifly/workflows/questions/"
	path := filepath.Join(root, folder+"workflow.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	workflow := strings.Replace(string(data), "inputs: {}", `inputs:
  value:
    schema_ref: "{{schema_value}}"
    required: true
    configuration: {scope: run, default: 3}`, 1)
	workflow = strings.Replace(workflow, "decision_catalog: [", "decision_catalog: ["+folder+"decisions/input.yaml, ", 1)
	writeFixtureFile(t, root, folder+"workflow.yaml", workflow)
	writeFixtureFile(t, root, folder+"schemas/value.yaml", "id: test:schema/value\nversion: 1.0.0\ntype: integer\nminimum: 1\nmaximum: 3\n")
	decision := `authoring: prifly-run-decision/1
id: input
title: Input value
phase: preflight
choices: [{id: two, title: Two, value: 2}]
destination: {kind: launch_input, name: value}
`
	writeFixtureFile(t, root, folder+"decisions/input.yaml", decision)
	writeFixtureFile(t, root, ".prifly/.gitignore", "local.yaml\n")
	if code, _, stderr := runCLI(t, "project", "init", "--repository", root, "--state-root", authority); code != 0 {
		t.Fatalf("init: %d %s", code, stderr)
	}
	selection := []string{"--repository", root, "--launch", "questions", "--preflight-answer", "gate=false", "--preflight-answer", "input=2"}
	prepare := append([]string{"project", "questionnaire", "--prepare"}, selection...)
	code, out, stderr := runCLI(t, prepare...)
	var summary projectLaunchSummary
	if code != 0 || json.Unmarshal([]byte(out), &summary) != nil || summary.InputDigests["value"] != projectBytesDigest([]byte("2")) {
		t.Fatalf("prepare lost the selected input: %d %s %s", code, out, stderr)
	}
	start := append([]string{"project", "start", "--expected-launch-digest", summary.ReviewDigest}, selection...)
	code, out, stderr = runCLI(t, start...)
	var started projectStartResult
	if code != 0 || json.Unmarshal([]byte(out), &started) != nil || started.Run.Run.Status != "completed" {
		t.Fatalf("start with only a decision-bound input: %d %s %s", code, out, stderr)
	}
	engine, err := prifly.Open(authority, true)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	_, input, err := engine.Artifact(started.Run.Run.Inputs["value"])
	if err != nil || string(input) != "2" {
		t.Fatalf("Run received default instead of selected decision: %q %v", input, err)
	}
	configuration := started.Run.Run.EffectiveConfiguration
	if configuration == nil || string(configuration.Inputs["value"].Value) != "2" || configuration.Inputs["value"].Source != "run" {
		t.Fatalf("ordinary configuration resolution was bypassed: %+v", configuration)
	}
	claims, err := engine.Claims(context.Background())
	if err != nil || len(claims.Claims) != 0 {
		t.Fatalf("decision input acquired a workspace: %+v %v", claims, err)
	}

	// A runtime decision cannot rewrite the already sealed launch inputs.
	writeFixtureFile(t, root, folder+"decisions/input.yaml", strings.Replace(decision, "phase: preflight", "phase: runtime", 1))
	if code, _, stderr := runCLI(t, "project", "questionnaire", "--repository", root, "--launch", "questions"); code == 0 || !strings.Contains(stderr, "launch input decision requires preflight phase") {
		t.Fatalf("runtime launch-input destination accepted: %d %s", code, stderr)
	}
}
