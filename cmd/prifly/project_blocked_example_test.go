package main

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// blockedExample is the shipped example installed in a fresh Git-less project
// with Node allowed and the condition file path set, but the file absent: the
// stand controls the condition by creating or removing it.
type blockedExample struct {
	root, authority, condition string
	command                    func(...string) string
}

func newBlockedExample(t *testing.T) blockedExample {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("the runnable blocked example requires Node")
	}
	resolveContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	resolved, err := exec.CommandContext(resolveContext, node, "-p", "process.execPath").Output()
	cancel()
	if err != nil || !filepath.IsAbs(strings.TrimSpace(string(resolved))) {
		t.Fatalf("resolve actual Node executable: %v %q", err, resolved)
	}
	node = strings.TrimSpace(string(resolved))
	t.Setenv("PATH", t.TempDir())
	root, authority := t.TempDir(), filepath.Join(t.TempDir(), "authority")
	condition := filepath.Join(t.TempDir(), "condition")
	command := func(args ...string) string {
		t.Helper()
		code, out, stderr := runCLI(t, args...)
		if code != 0 {
			t.Fatalf("%v: exit=%d %s", args, code, stderr)
		}
		return out
	}
	command("project", "init", "--repository", root, "--state-root", authority)
	for from, to := range map[string]string{"source": "blocked-condition", "continue": "blocked-condition-continue"} {
		source, err := filepath.Abs(filepath.Join("../../examples/workflows/blocked-condition", from))
		if err != nil {
			t.Fatal(err)
		}
		folder := filepath.Join(root, ".prifly", "workflows", to)
		if err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			relative, err := filepath.Rel(source, path)
			if err != nil {
				return err
			}
			data, err := os.ReadFile(path)
			if err == nil {
				writeFixtureFile(t, folder, relative, string(data))
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	writeFixtureFile(t, root, ".prifly/project.yaml", `schema_version: prifly-project-profile/3
packages:
  blocked-condition: {source: .prifly/workflows/blocked-condition}
  blocked-condition-continue: {source: .prifly/workflows/blocked-condition-continue}
launches:
  work:
    title: Work on a condition
    description: Retries at most three times while the condition is absent.
    kind: workflow
    workflow: .prifly/workflows/blocked-condition/workflow.yaml
  continue:
    title: Continue work on a condition
    description: Continues a Run that stopped partial.
    kind: workflow
    workflow: .prifly/workflows/blocked-condition-continue/workflow.yaml
`)
	writeFixtureFile(t, root, "request.json", `{"subject":"the report"}`+"\n")
	command("project", "local", "set", "--repository", root, "--allow-executable", "node="+node, "--env", "CONDITION_FILE="+condition)
	return blockedExample{root: root, authority: authority, condition: condition, command: command}
}

func (f blockedExample) start(t *testing.T, launch string, extra ...string) projectStartResult {
	t.Helper()
	verb := "start"
	if slices.Contains(extra, "--source-run") {
		verb = "continue"
	}
	return f.launch(t, verb, launch, extra...)
}

func (f blockedExample) launch(t *testing.T, verb, launch string, extra ...string) projectStartResult {
	t.Helper()
	args := append([]string{"--repository", f.root, "--launch", launch, "--allow-execution"}, extra...)
	prepare := append([]string{"project", "questionnaire", "--prepare"}, args...)
	if verb == "continue" {
		prepare = append([]string{"project", "continue", "--prepare"}, args...)
	}
	var review projectLaunchSummary
	if err := json.Unmarshal([]byte(f.command(prepare...)), &review); err != nil {
		t.Fatal(err)
	}
	var started projectStartResult
	if err := json.Unmarshal([]byte(f.command(append(append([]string{"project", verb}, args...), "--expected-launch-digest", review.ReviewDigest)...)), &started); err != nil {
		t.Fatal(err)
	}
	return started
}

func (f blockedExample) status(t *testing.T, runID string) prifly.RunView {
	t.Helper()
	var view prifly.RunView
	if err := json.Unmarshal([]byte(f.command("--project", f.authority, "run", "status", runID)), &view); err != nil {
		t.Fatal(err)
	}
	return view
}

func (f blockedExample) next(t *testing.T, runID string) prifly.NextView {
	t.Helper()
	var next prifly.NextView
	if err := json.Unmarshal([]byte(f.command("--project", f.authority, "run", "next", runID)), &next); err != nil {
		t.Fatal(err)
	}
	return next
}

// verdicts lists the accepted verdict of every step Attempt the Run settled,
// by stage, in no particular order.
func verdicts(view prifly.RunView) map[string][]string {
	found := map[string][]string{}
	for _, step := range view.Run.Steps {
		if activation := view.Run.Activations[step.ActivationID]; activation != nil {
			found[activation.StageID] = append(found[activation.StageID], step.Verdict)
		}
	}
	return found
}

// The customer's acceptance, point by point, on real Runs of the shipped
// example: a program returns blocked with the obstacle it promised, a remedy
// step with its own inputs and no permission to change anything records it,
// a bounded repeat retries and stops on its declared limit, a fresh reader
// learns from run next why the Run stopped, and a declared continuation goes
// on once the condition holds -- without repeating anything already accepted.
func TestCLIBlockedExampleAcceptance(t *testing.T) {
	f := newBlockedExample(t)
	input := []string{"--input", "request=" + filepath.Join(f.root, "request.json")}

	// 2, 3, 5: the condition is absent. Each attempt's check returns blocked
	// with its obstacle, the remedy records it and the body ends partial; the
	// repeat stops on its limit of three, not on an exhausted budget.
	blocked := f.start(t, "work", input...)
	view := f.status(t, blocked.Run.Run.ID)
	if view.Run.Outcome == nil || *view.Run.Outcome != "partial" {
		t.Fatalf("the Run did not stop partial on its limit: %v %+v", view.Run.Outcome, view.Run.Diagnostics)
	}
	byStage := verdicts(view)
	if len(byStage["check"]) != 3 || slices.ContainsFunc(byStage["check"], func(v string) bool { return v != "blocked" }) {
		t.Fatalf("the check did not return blocked on each of three attempts: %v", byStage)
	}
	if len(byStage["remedy"]) != 3 || slices.ContainsFunc(byStage["remedy"], func(v string) bool { return v != "pass" }) {
		t.Fatalf("the remedy did not run once per blocked attempt: %v", byStage)
	}
	if !slices.Equal(byStage["prepare"], []string{"pass"}) {
		t.Fatalf("the work was not prepared once: %v", byStage)
	}
	for _, diagnostic := range view.Run.Diagnostics {
		if diagnostic.Code == "budget_exhausted" {
			t.Fatalf("the loop ended on the budget, not its declared limit: %+v", diagnostic)
		}
	}
	obstacle := view.Run.Outputs["obstacle"]
	if obstacle.ArtifactID == "" {
		t.Fatalf("the obstacle is not the Run's output: %+v", view.Run.Outputs)
	}

	// 6: a reader with none of this history learns from run next where the
	// Run stopped, by which route and with which obstacle.
	next := f.next(t, blocked.Run.Run.ID)
	if next.ArrivedFrom == nil || next.ArrivedFrom.StageID != "attempt" || next.ArrivedFrom.Verdict != "on_limit" || next.ArrivedFrom.Outputs["obstacle"] != obstacle {
		t.Fatalf("run next does not say the Run stopped on its limit with the obstacle: %+v", next.ArrivedFrom)
	}

	// 1: with the condition present, the work passes on its first attempt
	// with no waiting and no remedy.
	if err := os.WriteFile(f.condition, []byte("present\n"), 0600); err != nil {
		t.Fatal(err)
	}
	f.command("--project", f.authority, "capacity", "set", "--capacity", "3", "--reason", "several example Runs")
	passed := f.start(t, "work", input...)
	passedView := f.status(t, passed.Run.Run.ID)
	if passedView.Run.Outcome == nil || *passedView.Run.Outcome != "succeeded" || len(verdicts(passedView)["check"]) != 1 || len(verdicts(passedView)["remedy"]) != 0 {
		t.Fatalf("with the condition present the work did not simply pass: %v %v", passedView.Run.Outcome, verdicts(passedView))
	}

	// 4, 7: the stopped Run is continued. The continuation takes the request
	// and the last obstacle as declared, checks the condition again as its
	// first stage -- it does not trust the carried obstacle -- and passes; none
	// of the source's accepted stages run again.
	continued := f.start(t, "continue", "--source-run", blocked.Run.Run.ID)
	continuedView := f.status(t, continued.Run.Run.ID)
	if continuedView.Run.Outcome == nil || *continuedView.Run.Outcome != "succeeded" {
		t.Fatalf("the continuation did not pass once the condition held: %v %+v", continuedView.Run.Outcome, continuedView.Run.Diagnostics)
	}
	if got := verdicts(continuedView); len(got) != 1 || len(got["check"]) != 1 {
		t.Fatalf("the continuation repeated accepted stages: %v", got)
	}
	if continuedView.Run.Fork == nil || continuedView.Run.Fork.SourceRunID != blocked.Run.Run.ID {
		t.Fatalf("the continuation lost its source: %+v", continuedView.Run.Fork)
	}
	if source := f.status(t, blocked.Run.Run.ID); source.Run.Outcome == nil || *source.Run.Outcome != "partial" {
		t.Fatalf("continuing changed the source Run: %v", source.Run.Outcome)
	}
	// Now that its package is installed, run next on the source names it --
	// and the Run's own workflow, which declares it resumes such a Run.
	if next := f.next(t, blocked.Run.Run.ID); next.Continuations == nil || len(*next.Continuations) != 2 || (*next.Continuations)[0].ID != "example:workflow/blocked-condition" || (*next.Continuations)[1].ID != "example:workflow/blocked-condition-continue" || !slices.Contains(next.SafeNextActions, "project.continue") {
		t.Fatalf("run next does not name what continues the stopped Run: %+v", next.Continuations)
	}

	// The same stopped Run resumed by its own launch: the accepted prepare is
	// carried and not run again, the attempt it stopped at runs again, and
	// the Run's own inputs are taken as they were. Resuming from the entry
	// would carry nothing, which is a new start, not a resume.
	if code, _, stderr := runCLI(t, "project", "continue", "--prepare", "--repository", f.root, "--launch", "work", "--allow-execution", "--source-run", blocked.Run.Run.ID, "--from-stage", "prepare"); code == 0 || !strings.Contains(stderr, "resume_prefix_empty") {
		t.Fatalf("a resume carrying nothing was prepared: exit=%d %s", code, stderr)
	}
	if code, _, stderr := runCLI(t, "project", "continue", "--prepare", "--repository", f.root, "--launch", "work", "--allow-execution", "--source-run", blocked.Run.Run.ID, "--input", "request="+filepath.Join(f.root, "request.json")); code == 0 || !strings.Contains(stderr, "resume_input_override") {
		t.Fatalf("a resume took another input: exit=%d %s", code, stderr)
	}
	resumed := f.launch(t, "continue", "work", "--source-run", blocked.Run.Run.ID)
	resumedView := f.status(t, resumed.Run.Run.ID)
	if resumedView.Run.Outcome == nil || *resumedView.Run.Outcome != "succeeded" {
		t.Fatalf("the resumed Run did not pass once the condition held: %v %+v", resumedView.Run.Outcome, resumedView.Run.Diagnostics)
	}
	if got := verdicts(resumedView); len(got) != 1 || !slices.Equal(got["check"], []string{"pass"}) {
		t.Fatalf("the resume ran again what was accepted, or not the stage it stopped at: %v", got)
	}
	if resumedView.Run.Fork == nil || resumedView.Run.Fork.Reason != prifly.ResumeReason || resumedView.Run.Fork.SourceRunID != blocked.Run.Run.ID {
		t.Fatalf("the resume lost its source: %+v", resumedView.Run.Fork)
	}
	if recovery := resumedView.Run.Recovery; recovery == nil || recovery.FrontierStageID != "attempt" || len(recovery.Reused) != 1 || recovery.Reused[0].StageID != "prepare" {
		t.Fatalf("the resume did not carry prepare and start at attempt: %+v", recovery)
	}
	if source := f.status(t, blocked.Run.Run.ID); source.Run.Outcome == nil || *source.Run.Outcome != "partial" {
		t.Fatalf("resuming changed the source Run: %v", source.Run.Outcome)
	}

	// 9: a judgement that the work is wrong is not blocked. Without the
	// condition configured at all the check fails, and the Run ends rejected,
	// not partial.
	f.command("project", "local", "set", "--repository", f.root, "--env", "CONDITION_FILE=")
	failed := f.start(t, "work", input...)
	failedView := f.status(t, failed.Run.Run.ID)
	if failedView.Run.Outcome == nil || *failedView.Run.Outcome != "rejected" || !slices.Equal(verdicts(failedView)["check"], []string{"fail"}) {
		t.Fatalf("fail was not kept apart from blocked: %v %v", failedView.Run.Outcome, verdicts(failedView))
	}
	// The workflow resumes only what it declared: a rejected Run is not.
	if code, _, stderr := runCLI(t, "project", "continue", "--prepare", "--repository", f.root, "--launch", "work", "--allow-execution", "--source-run", failed.Run.Run.ID); code == 0 || !strings.Contains(stderr, "resume_undeclared") {
		t.Fatalf("a Run of an undeclared outcome was resumed: exit=%d %s", code, stderr)
	}
}

// A resume is decided as its source was: with nothing answered on the command
// line it takes the source Run's answers, and an answer that differs is
// refused rather than sealed into a Run whose carried stages were decided
// otherwise.
func TestCLIResumeTakesTheSourceRunsAnswers(t *testing.T) {
	f := newBlockedExample(t)
	folder := ".prifly/workflows/blocked-condition/"
	data, err := os.ReadFile(filepath.Join(f.root, folder+"workflow.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, f.root, folder+"workflow.yaml", strings.Replace(string(data), "entry: prepare\n", "decision_catalog: ["+folder+"decisions/subject.yaml]\nentry: prepare\n", 1))
	writeFixtureFile(t, f.root, folder+"decisions/subject.yaml", `authoring: prifly-run-decision/1
id: subject
title: What to work on
phase: preflight
choices:
  - {id: report, title: The report, value: {subject: the report}}
  - {id: summary, title: The summary, value: {subject: the summary}}
destination: {kind: launch_input, name: request}
`)
	stopped := f.launch(t, "start", "work", "--preflight-answer", `subject={"subject":"the report"}`)
	if view := f.status(t, stopped.Run.Run.ID); view.Run.Outcome == nil || *view.Run.Outcome != "partial" || view.Run.DecisionSheet == nil {
		t.Fatalf("the source did not stop partial with its answers sealed: %v", view.Run.Outcome)
	}
	if err := os.WriteFile(f.condition, []byte("present\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runCLI(t, "project", "continue", "--prepare", "--repository", f.root, "--launch", "work", "--allow-execution", "--source-run", stopped.Run.Run.ID, "--preflight-answer", `subject={"subject":"the summary"}`); code == 0 || !strings.Contains(stderr, "recover_context_changed") {
		t.Fatalf("a resume decided otherwise was prepared: exit=%d %s", code, stderr)
	}
	resumed := f.launch(t, "continue", "work", "--source-run", stopped.Run.Run.ID)
	view := f.status(t, resumed.Run.Run.ID)
	if view.Run.Outcome == nil || *view.Run.Outcome != "succeeded" || view.Run.Fork == nil || view.Run.Fork.Reason != prifly.ResumeReason {
		t.Fatalf("the resume without answers did not take the source's: %v %+v", view.Run.Outcome, view.Run.Fork)
	}
}
