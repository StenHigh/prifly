package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// The case the recovery was made for, on real programs: a late stage is
// refused on its output contract after the stages before it were accepted,
// the package is corrected, and project recover carries the accepted stages
// and settles the failed one -- without running the accepted programs again.
// Every program run appends its operation to a marker, so the count of real
// executions is read, not assumed.
func TestCLIProjectRecoverCarriesAcceptedStagesOntoACorrectedPackage(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("the native CSV example requires Node")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	resolved, err := exec.CommandContext(ctx, node, "-p", "process.execPath").Output()
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	node = strings.TrimSpace(string(resolved))
	t.Setenv("PATH", t.TempDir())
	root, authority := t.TempDir(), filepath.Join(t.TempDir(), "authority")
	marker := filepath.Join(t.TempDir(), "executions")
	command := func(args ...string) string {
		t.Helper()
		code, out, stderr := runCLI(t, args...)
		if code != 0 {
			t.Fatalf("%v: exit=%d %s", args, code, stderr)
		}
		return out
	}
	executions := func() []string {
		data, err := os.ReadFile(marker)
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		return strings.Fields(string(data))
	}
	command("project", "init", "--repository", root, "--state-root", authority)
	folder := filepath.Join(root, ".prifly/workflows/csv-report")
	example := map[string]string{}
	for _, name := range []string{"workflow.yaml", "steps/parse.yaml", "steps/validate.yaml", "steps/report.yaml", "schemas/rows.yaml", "files/worker.mjs", "sample.csv"} {
		data, err := os.ReadFile(filepath.Join("../../examples/workflows/csv-report", name))
		if err != nil {
			t.Fatal(err)
		}
		example[name] = string(data)
	}
	replace := func(text, old, new string) string {
		t.Helper()
		if !strings.Contains(text, old) {
			t.Fatalf("the example no longer has %q", old)
		}
		return strings.Replace(text, old, new, 1)
	}
	// A technical failure must end the Run failed, not route to rejected.
	example["workflow.yaml"] = replace(example["workflow.yaml"], "    on: {pass: done, fail: rejected, needs_revision: rejected, no_work: rejected}\n    on_error: rejected\n", "    on: {pass: done, fail: rejected, needs_revision: rejected, no_work: rejected}\n")
	example["workflow.yaml"] = replace(example["workflow.yaml"], "entry: parse", "resumable: {from_outcomes: [succeeded]}\nentry: parse")
	example["workflow.yaml"] = strings.ReplaceAll(example["workflow.yaml"], "needs_revision: rejected, no_work: rejected}", "needs_revision: rejected, no_work: rejected, blocked: rejected}")
	example["files/worker.mjs"] = replace(example["files/worker.mjs"], "const envelope =", "appendFileSync(process.env.EXECUTIONS, process.argv[2] + '\\n');\nconst envelope =")
	example["files/worker.mjs"] = replace(example["files/worker.mjs"], "import { readFileSync, writeFileSync } from 'node:fs';", "import { appendFileSync, readFileSync, writeFileSync } from 'node:fs';")
	corrected := example["steps/report.yaml"]
	// The first package's contract is wrong: it requires a second output the
	// program never writes -- the program exits 0 and its result is refused.
	example["steps/report.yaml"] = replace(corrected, "  report: {format: blob, media_types: [text/plain], required_for: [pass]}", "  report: {format: blob, media_types: [text/plain], required_for: [pass]}\n  summary: {format: blob, media_types: [text/plain], required_for: [pass]}")
	for name, text := range example {
		writeFixtureFile(t, folder, name, text)
	}
	writeFixtureFile(t, root, ".prifly/project.yaml", `schema_version: prifly-project-profile/3
packages: {csv-report: {source: .prifly/workflows/csv-report}}
launches:
  csv-report:
    title: CSV report
    description: Parse, validate and report.
    kind: workflow
    workflow: .prifly/workflows/csv-report/workflow.yaml
`)
	command("project", "local", "set", "--repository", root, "--allow-executable", "node="+node, "--env", "EXECUTIONS="+marker)
	args := []string{"--repository", root, "--launch", "csv-report", "--allow-execution"}
	var reviewed projectLaunchSummary
	if err := json.Unmarshal([]byte(command(append(append([]string{"project", "questionnaire", "--prepare"}, args...), "--input", "csv="+filepath.Join(folder, "sample.csv"))...)), &reviewed); err != nil {
		t.Fatal(err)
	}
	code, out, stderr := runCLI(t, append(append([]string{"project", "start"}, args...), "--input", "csv="+filepath.Join(folder, "sample.csv"), "--expected-launch-digest", reviewed.ReviewDigest)...)
	var started projectStartResult
	if json.Unmarshal([]byte(out), &started) != nil || started.RunID == "" {
		t.Fatalf("the first Run did not start: %d %s %s", code, out, stderr)
	}
	source := started.RunID
	status := func(runID string) prifly.RunView {
		t.Helper()
		var view prifly.RunView
		if err := json.Unmarshal([]byte(command("--project", authority, "run", "status", runID)), &view); err != nil {
			t.Fatal(err)
		}
		return view
	}
	failed := status(source)
	if failed.Run.Status != "failed" || failed.Run.Outcome != nil || !slices.Equal(executions(), []string{"parse", "validate", "report"}) {
		t.Fatalf("the source did not fail technically at report: %s %v %v", failed.Run.Status, failed.Run.Outcome, executions())
	}
	sourceVersion := failed.RunVersion
	recover := append([]string{"--repository", root, "--launch", "csv-report", "--source-run", source}, "--allow-execution")

	// On the unchanged package the saved result still fails its contract, so
	// the plan runs the failed stage again rather than refusing the recovery.
	var same projectLaunchSummary
	if err := json.Unmarshal([]byte(command(append([]string{"project", "recover", "--prepare"}, recover...)...)), &same); err != nil {
		t.Fatal(err)
	}
	if same.Recovery == nil || same.Recovery.FrontierAction != "execute" || !strings.Contains(same.Recovery.FrontierReason, "fails the target contract") {
		t.Fatalf("an unchanged package did not plan to run the failed stage again: %+v", same.Recovery)
	}

	// Repeated technical recovery must retain the original parse/validate
	// prefix even when the first recovery fails at report again.
	chainSource := source
	for i := 0; i < 2; i++ {
		chainArgs := []string{"--repository", root, "--launch", "csv-report", "--source-run", chainSource, "--allow-execution"}
		var review projectLaunchSummary
		if err := json.Unmarshal([]byte(command(append([]string{"project", "recover", "--prepare"}, chainArgs...)...)), &review); err != nil {
			t.Fatal(err)
		}
		if len(review.Recovery.Reused) != 2 {
			t.Fatalf("recovery chain lost prefix: %+v", review.Recovery)
		}
		code, out, stderr := runCLI(t, append(append([]string{"project", "recover"}, chainArgs...), "--expected-launch-digest", review.ReviewDigest)...)
		var child projectStartResult
		if json.Unmarshal([]byte(out), &child) != nil || child.RunID == "" {
			t.Fatalf("recovery chain failed before creation: %d %s %s", code, out, stderr)
		}
		if view := status(child.RunID); view.Run.Status != "failed" || len(view.Run.Steps) != 1 {
			t.Fatalf("recovery did not fail only at report: %s %+v", view.Run.Status, view.Run.Steps)
		}
		chainSource = child.RunID
	}
	if !slices.Equal(executions(), []string{"parse", "validate", "report", "report", "report"}) {
		t.Fatalf("recovery re-executed prefix: %v", executions())
	}

	// The corrected package.
	writeFixtureFile(t, folder, "steps/report.yaml", corrected)
	var plan projectLaunchSummary
	if err := json.Unmarshal([]byte(command(append([]string{"project", "recover", "--prepare"}, recover...)...)), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Recovery == nil || plan.Recovery.FrontierStageID != "report" || len(plan.Recovery.Reused) != 2 || plan.Recovery.FrontierAction != "revalidate" {
		t.Fatalf("the plan does not carry parse and validate onto report: %+v", plan.Recovery)
	}
	if !slices.Equal(executions(), []string{"parse", "validate", "report", "report", "report"}) {
		t.Fatalf("a recovery review ran a program: %v", executions())
	}
	// A package changed after the review is not the reviewed one.
	writeFixtureFile(t, folder, "files/worker.mjs", example["files/worker.mjs"]+"\n// changed after review\n")
	if code, _, stderr := runCLI(t, append(append([]string{"project", "recover"}, recover...), "--expected-launch-digest", plan.ReviewDigest)...); code == 0 || !strings.Contains(stderr, "stale") {
		t.Fatalf("a stale recovery review started: %d %s", code, stderr)
	}
	writeFixtureFile(t, folder, "files/worker.mjs", example["files/worker.mjs"])
	// Without --allow-execution a recovery that runs a program is not reviewed.
	withoutApproval := slices.DeleteFunc(slices.Clone(recover), func(arg string) bool { return arg == "--allow-execution" })
	if code, _, stderr := runCLI(t, append(append([]string{"project", "recover"}, withoutApproval...), "--expected-launch-digest", plan.ReviewDigest)...); code == 0 || !strings.Contains(stderr, "allow-execution") {
		t.Fatalf("a recovery started without execution approval: %d %s", code, stderr)
	}

	var recovered projectStartResult
	if err := json.Unmarshal([]byte(command(append(append([]string{"project", "recover"}, recover...), "--expected-launch-digest", plan.ReviewDigest, "--command-id", "recover-once")...)), &recovered); err != nil || recovered.RunID == "" {
		t.Fatalf("recovery did not start: %v", err)
	}
	// The same command again is answered with the Run it made: no second one.
	code, out, stderr = runCLI(t, append(append([]string{"project", "recover"}, recover...), "--expected-launch-digest", plan.ReviewDigest, "--command-id", "recover-once")...)
	var repeated projectStartResult
	if json.Unmarshal([]byte(out), &repeated) != nil || code != 0 || repeated.RunID != recovered.RunID {
		t.Fatalf("a repeated recovery made another Run or failed: %d %s %s", code, repeated.RunID, stderr)
	}
	view := status(recovered.RunID)
	if view.Run.Status != "completed" || view.Run.Outcome == nil || *view.Run.Outcome != "succeeded" {
		t.Fatalf("the recovered Run did not finish: %s %v %+v", view.Run.Status, view.Run.Outcome, view.Run.Diagnostics)
	}
	provenance := view.Run.Recovery
	if provenance == nil || provenance.SourceRunID != source || provenance.FrontierStageID != "report" || len(provenance.Reused) != 2 {
		t.Fatalf("the recovered Run lost its provenance: %+v", provenance)
	}
	// The corrected contract accepts the bytes the failed report left, so
	// nothing runs again: not parse, not validate, not even report.
	if ran := executions()[5:]; provenance.FrontierAction != "revalidate" || len(ran) != 0 {
		t.Fatalf("recovery ran programs the evidence already settled: %s %v", provenance.FrontierAction, ran)
	}
	// The Run's output is the report the failed attempt wrote, carried whole.
	engine, err := prifly.Open(authority, true)
	if err != nil {
		t.Fatal(err)
	}
	_, report, err := engine.Artifact(view.Run.Outputs["report"])
	engine.Close()
	if err != nil || !strings.HasPrefix(string(report), "Pri-Fly CSV report\nRows: ") {
		t.Fatalf("the recovered Run's report is not the one the failed attempt wrote: %q %v", report, err)
	}
	after := status(source)
	if after.RunVersion != sourceVersion || after.Run.Status != "failed" {
		t.Fatalf("recovery changed the source Run: v%d→v%d %s", sourceVersion, after.RunVersion, after.Run.Status)
	}
	// A completed Run has no technical failure to recover from.
	if code, _, stderr := runCLI(t, "project", "recover", "--prepare", "--repository", root, "--launch", "csv-report", "--source-run", recovered.RunID, "--allow-execution"); code == 0 || !strings.Contains(stderr, "recover_source_ineligible") {
		t.Fatalf("a completed Run was offered for recovery: %d %s", code, stderr)
	}
	// A corrected program now emits the extra required output. The candidate
	// still lacks it, so recovery executes report and a subsequent resume does
	// so again while both retain the original parse/validate bindings.
	writeFixtureFile(t, folder, "steps/report.yaml", example["steps/report.yaml"])
	writeFixtureFile(t, folder, "files/worker.mjs", replace(example["files/worker.mjs"], "result.summary = 'Report produced';", "output('summary', 'Produced\\n');\n      result.summary = 'Report produced';"))
	chainArgs := []string{"--repository", root, "--launch", "csv-report", "--source-run", chainSource, "--allow-execution"}
	var chainReview projectLaunchSummary
	if err := json.Unmarshal([]byte(command(append([]string{"project", "recover", "--prepare"}, chainArgs...)...)), &chainReview); err != nil {
		t.Fatal(err)
	}
	if chainReview.Recovery.FrontierAction != "execute" {
		t.Fatal("incomplete candidate was revalidated")
	}
	var executed projectStartResult
	if err := json.Unmarshal([]byte(command(append(append([]string{"project", "recover"}, chainArgs...), "--expected-launch-digest", chainReview.ReviewDigest)...)), &executed); err != nil {
		t.Fatal(err)
	}
	chainArgs[5] = executed.RunID
	var resumeReview projectLaunchSummary
	if err := json.Unmarshal([]byte(command(append([]string{"project", "continue", "--prepare"}, chainArgs...)...)), &resumeReview); err != nil {
		t.Fatal(err)
	}
	var resumed projectStartResult
	if err := json.Unmarshal([]byte(command(append(append([]string{"project", "continue"}, chainArgs...), "--expected-launch-digest", resumeReview.ReviewDigest)...)), &resumed); err != nil {
		t.Fatal(err)
	}
	if view := status(resumed.RunID); view.Run.Outcome == nil || *view.Run.Outcome != "succeeded" || len(view.Run.Recovery.Reused) != 2 || len(view.Run.Steps) != 1 {
		t.Fatalf("recover/resume chain: %+v", view.Run.Diagnostics)
	}
	if !slices.Equal(executions(), []string{"parse", "validate", "report", "report", "report", "report", "report"}) {
		t.Fatalf("mixed chain repeated accepted programs: %v", executions())
	}

}
