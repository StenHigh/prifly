package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// readingFixture is an authority with count finished Runs of an empty
// workflow, started in order.
func readingFixture(t *testing.T, count int) (project string, runs []string, cli func(...string) string) {
	t.Helper()
	project = t.TempDir()
	if err := prifly.Init(project); err != nil {
		t.Fatal(err)
	}
	brief := prifly.Brief{SchemaVersion: "1", ID: "test:brief/reading", Subject: "Reading a Run", DesiredOutcome: "Finish with no_work", InScope: []string{"Local state"}, OutOfScope: []string{"Network"}, CompletionCriteria: []string{"no_work"}, SourceRefs: []prifly.ArtifactRef{}, Assumptions: []string{}, Confirmation: "explicit"}
	for path, value := range map[string]any{"workflows/reading.json": emptyCLIWorkflow(t), "brief.json": brief} {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(project, path), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	cli = func(arguments ...string) string {
		t.Helper()
		var out, errout bytes.Buffer
		if code := execute(context.Background(), append([]string{"--project", project}, arguments...), &out, &errout); code != 0 {
			t.Fatalf("%v: exit=%d %s", arguments, code, errout.String())
		}
		return out.String()
	}
	for index := range count {
		started := cli("--json", "run", "start", "--workflow", "workflows/reading.json", "--brief", "brief.json", "--command-id", "command:reading-"+string(rune('a'+index)), "--drive")
		var view prifly.RunView
		if err := json.Unmarshal([]byte(started), &view); err != nil || view.Run.ID == "" {
			t.Fatalf("no run id in the start result: %v", err)
		}
		runs = append(runs, view.Run.ID)
	}
	return project, runs, cli
}

// The text form of run events was the JSON envelope indented: a reader asking
// for events met snapshot and cut first, and the pilot's first JSON read of
// .events returned nothing because they sit in .view.events. The text form is
// one line per event and says how to read on.
func TestRunEventsTextIsOneLinePerEventAndSaysHowToContinue(t *testing.T) {
	_, runs, cli := readingFixture(t, 1)
	text := cli("run", "events", runs[0], "--limit", "1")
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "1 run.created ") {
		t.Fatalf("the text form is not one line per event:\n%s", text)
	}
	if lines[1] != "more: run events "+runs[0]+" --after 1" {
		t.Fatalf("the text form does not say how to read on: %q", lines[1])
	}
	var answer struct {
		View struct {
			Events []json.RawMessage `json:"events"`
		} `json:"view"`
		NextAfter int64 `json:"next_after"`
	}
	if err := json.Unmarshal([]byte(cli("--json", "run", "events", runs[0], "--limit", "1")), &answer); err != nil || len(answer.View.Events) != 1 || answer.NextAfter != 1 {
		t.Fatalf("the published JSON form changed: %+v %v", answer, err)
	}
}

// A pilot needed the id of its own Run and found it through capacity show:
// run list was "not a run operation". It names the authority's Runs, newest
// first, and reads only.
func TestRunListNamesTheAuthoritysRunsNewestFirst(t *testing.T) {
	project, runs, cli := readingFixture(t, 3)
	engine, err := prifly.Open(project, true)
	if err != nil {
		t.Fatal(err)
	}
	before, _, err := engine.MonitorRevisions(context.Background(), "")
	engine.Close()
	if err != nil {
		t.Fatal(err)
	}
	var answer struct {
		SchemaVersion string `json:"schema_version"`
		Total         int    `json:"total"`
		Runs          []struct {
			RunID        string  `json:"run_id"`
			WorkflowID   string  `json:"workflow_id"`
			Status       string  `json:"status"`
			Outcome      *string `json:"outcome"`
			AwaitingHost bool    `json:"awaiting_host"`
		} `json:"runs"`
	}
	if err := json.Unmarshal([]byte(cli("--json", "run", "list")), &answer); err != nil {
		t.Fatal(err)
	}
	if answer.SchemaVersion != "run-list/1" || answer.Total != 3 || len(answer.Runs) != 3 {
		t.Fatalf("run list does not name the three Runs: %+v", answer)
	}
	for index, entry := range answer.Runs {
		if entry.RunID != runs[len(runs)-1-index] {
			t.Fatalf("run list is not newest first: %+v, started %v", answer.Runs, runs)
		}
		if entry.WorkflowID != "test:workflow/cli" || entry.Status != "completed" || entry.Outcome == nil || *entry.Outcome != "no_work" || entry.AwaitingHost {
			t.Fatalf("a listed Run misreports itself: %+v", entry)
		}
	}
	text := cli("run", "list", "--limit", "2")
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[0], runs[2]+" status=completed outcome=no_work workflow=test:workflow/cli") || lines[2] != "2 of 3 shown; run list --limit 3 shows all" {
		t.Fatalf("the text list does not say what it left out:\n%s", text)
	}
	engine, err = prifly.Open(project, true)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	after, _, err := engine.MonitorRevisions(context.Background(), "")
	if err != nil || len(after) != len(before) {
		t.Fatalf("listing changed the authority: %v", err)
	}
	for index := range before {
		if before[index].Version != after[index].Version || before[index].EventSeq != after[index].EventSeq {
			t.Fatalf("listing moved a Run: %+v -> %+v", before[index], after[index])
		}
	}
	// The refusal of an unknown operation names list among the real ones.
	var out, errout bytes.Buffer
	if code := execute(context.Background(), []string{"--project", project, "run", "bogus"}, &out, &errout); code == 0 || !strings.Contains(errout.String(), "list") {
		t.Fatalf("the refusal of an unknown run operation does not name list: %s", errout.String())
	}
}

// The questions a host said its step answered were reachable only as a field
// of the stored attempt. The operator reading the summary sees each one with
// its answer and basis, and an empty report reads as the host's "none" rather
// than as nothing recorded.
func TestRunStatusPrintsTheQuestionsEachStepAnswered(t *testing.T) {
	view := prifly.RunView{Run: prifly.Run{
		ID: "run:questions",
		Activations: map[string]*prifly.Activation{
			"activation:plan":    {StageID: "plan"},
			"activation:improve": {StageID: "improve"},
		},
		Attempts: map[string]*prifly.Attempt{
			"attempt:plan": {ID: "attempt:plan", ActivationID: "activation:plan", Admitted: prifly.Observation{UTC: "2026-09-30T10:00:00Z"}, QuestionReport: &prifly.QuestionReport{SchemaVersion: prifly.QuestionReportVersion, Questions: []prifly.AnsweredQuestion{}}},
			"attempt:improve": {ID: "attempt:improve", ActivationID: "activation:improve", Admitted: prifly.Observation{UTC: "2026-09-30T10:05:00Z"}, QuestionReport: &prifly.QuestionReport{SchemaVersion: prifly.QuestionReportVersion, Questions: []prifly.AnsweredQuestion{
				{Question: "Apply these improvements?", Answer: "all", Basis: "decision", DecisionID: "improve_apply"},
				{Question: "Where does work start?", Answer: "app/Services", Basis: "input", Port: "handoff"},
			}}},
			"attempt:program": {ID: "attempt:program", ActivationID: "activation:plan", Admitted: prifly.Observation{UTC: "2026-09-30T09:00:00Z"}},
		},
	}}
	var out bytes.Buffer
	if err := renderQuestions(&out, view.Run); err != nil {
		t.Fatal(err)
	}
	want := `questions stage=plan attempt="attempt:plan" none (the host said the step met no question)
question stage=improve attempt="attempt:improve" basis=decision decision=improve_apply question="Apply these improvements?" answer="all"
question stage=improve attempt="attempt:improve" basis=input input=handoff question="Where does work start?" answer="app/Services"
`
	if out.String() != want {
		t.Fatalf("the questions read\n%s\nwant\n%s", out.String(), want)
	}
}
