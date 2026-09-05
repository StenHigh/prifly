package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

type projectSummaryWriter func([]byte) (int, error)

func (write projectSummaryWriter) Write(data []byte) (int, error) { return write(data) }

// This proves optimistic launch review, not human consent or a native host UI.
// All execution is by the shipped native workers with Git absent from PATH.
func TestCLIProjectLaunchSummaryBeforeDispatch(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("the native CSV example requires Node")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	resolved, err := exec.CommandContext(ctx, node, "-p", "process.execPath").Output()
	cancel()
	if err != nil || !filepath.IsAbs(strings.TrimSpace(string(resolved))) {
		t.Fatalf("resolve native Node binary: %q %v", resolved, err)
	}
	node = strings.TrimSpace(string(resolved))
	t.Setenv("PATH", t.TempDir())
	root, authority := t.TempDir(), filepath.Join(t.TempDir(), "authority")
	command := func(args ...string) string {
		t.Helper()
		code, out, stderr := runCLI(t, args...)
		if code != 0 {
			t.Fatalf("%v: exit=%d %s", args, code, stderr)
		}
		return out
	}
	decode := func(data string, target any) {
		t.Helper()
		if err := json.Unmarshal([]byte(data), target); err != nil {
			t.Fatal(err)
		}
	}
	command("project", "init", "--repository", root, "--state-root", authority)
	const relative = ".prifly/workflows/csv-report"
	folder := filepath.Join(root, relative)
	for _, name := range []string{"workflow.yaml", "steps/parse.yaml", "steps/validate.yaml", "steps/report.yaml", "schemas/rows.yaml", "files/worker.mjs", "sample.csv"} {
		data, err := os.ReadFile(filepath.Join("../../examples/workflows/csv-report", name))
		if err != nil {
			t.Fatal(err)
		}
		if name == "workflow.yaml" {
			data = []byte(strings.Replace(string(data), "entry: parse", "decision_catalog: ["+relative+"/decisions/confirmed.yaml]\nentry: parse", 1))
		}
		if name == "files/worker.mjs" {
			data = []byte(strings.Replace(string(data), "const envelope =", "writeFileSync('launch-summary-worker-marker', process.argv[2]);\nconst envelope =", 1))
		}
		writeFixtureFile(t, folder, name, string(data))
	}
	writeFixtureFile(t, folder, "decisions/confirmed.yaml", `authoring: prifly-run-decision/1
id: confirmed
title: Confirm the declared output format
phase: runtime
choices: [{id: "yes", title: "Yes", value: true}, {id: "no", title: "No", value: false}]
destination: {kind: session_context, name: confirmed}
`)
	writeFixtureFile(t, root, ".prifly/project.yaml", `schema_version: prifly-project-profile/3
packages: {csv-report: {source: .prifly/workflows/csv-report}}
launches:
  csv-report:
    title: CSV report
    description: Native workers for exact launch review.
    kind: workflow
    workflow: .prifly/workflows/csv-report/workflow.yaml
`)
	// Use an owned link so the last-moment replacement cannot touch the actual
	// installation. The prepared binding must pin the resolved native binary.
	link := filepath.Join(t.TempDir(), "node")
	if err := os.Symlink(node, link); err != nil {
		t.Fatal(err)
	}
	command("project", "local", "set", "--repository", root, "--allow-executable", "node="+link)
	csv := filepath.Join(folder, "sample.csv")
	args := []string{"--repository", root, "--launch", "csv-report", "--input", "csv=" + csv, "--allow-execution", "--runtime-answer", "confirmed=false"}
	prepare := func() projectLaunchSummary {
		t.Helper()
		var summary projectLaunchSummary
		decode(command(append([]string{"project", "questionnaire", "--prepare"}, args...)...), &summary)
		return summary
	}
	assertNoEffects := func(t *testing.T) {
		t.Helper()
		engine, err := prifly.Open(authority, true)
		if err != nil {
			t.Fatal(err)
		}
		defer engine.Close()
		packages, err := engine.Packages(context.Background())
		if err != nil || len(packages.Packages) != 0 {
			t.Fatalf("review imported packages: %+v %v", packages, err)
		}
		claims, err := engine.Claims(context.Background())
		if err != nil || len(claims.Claims) != 0 {
			t.Fatalf("review created claims: %+v %v", claims, err)
		}
		runs, err := engine.Runs(context.Background())
		if err != nil || len(runs) != 0 {
			t.Fatalf("review created Runs: %+v %v", runs, err)
		}
		workspaces := engine.Config.Configuration.WorkspaceRoot
		if !filepath.IsAbs(workspaces) {
			workspaces = filepath.Join(authority, workspaces)
		}
		entries, err := os.ReadDir(workspaces)
		if err != nil && !os.IsNotExist(err) || len(entries) != 0 {
			t.Fatalf("review materialized worker workspaces: %v %v", entries, err)
		}
	}
	reviewed := prepare()
	assertNoEffects(t)
	if reviewed.SchemaVersion != "project-launch-summary/2" || !reviewed.KnownQuestionsOnly || reviewed.ReviewDigest == "" || reviewed.BuildKey == "" || len(reviewed.Execution) != 3 || len(reviewed.InputDigests) != 1 || reviewed.BriefDigest != "" {
		t.Fatalf("incomplete neutral launch review: %+v", reviewed)
	}
	if len(reviewed.DecisionSheet.Records) != 1 || string(reviewed.DecisionSheet.Records[0].Value) != "false" || !reviewed.DecisionStates[0].Answered {
		t.Fatal("typed false preanswer is absent from review")
	}
	start := append(append([]string{"project", "start"}, args...), "--expected-launch-digest", reviewed.ReviewDigest, "--command-id", "reviewed-start")
	for _, test := range []struct{ name, path, before, after string }{
		{"workflow_source", filepath.Join(folder, "workflow.yaml"), "title: CSV report", "title: Reviewed CSV report"},
		{"supporting_file", filepath.Join(folder, "files/worker.mjs"), "const result =", "// Updated supporting implementation.\nconst result ="},
		{"input_bytes", csv, "Alpha,12", "Alpha,13"},
		{"local_executable_mapping", filepath.Join(root, ".prifly/local.yaml"), link, "/bin/echo"},
	} {
		t.Run(test.name, func(t *testing.T) {
			original, err := os.ReadFile(test.path)
			if err != nil || !bytes.Contains(original, []byte(test.before)) {
				t.Fatalf("fixture mutation has no target: %v", err)
			}
			writeFixtureFile(t, filepath.Dir(test.path), filepath.Base(test.path), strings.Replace(string(original), test.before, test.after, 1))
			defer writeFixtureFile(t, filepath.Dir(test.path), filepath.Base(test.path), string(original))
			if code, _, stderr := runCLI(t, start...); code == 0 || !strings.Contains(stderr, "project_start_stale_launch") {
				t.Fatalf("stale review was not rejected: %d %s", code, stderr)
			}
			assertNoEffects(t)
		})
	}
	t.Run("runtime_answer", func(t *testing.T) {
		changed := append([]string{}, start...)
		for i, arg := range changed {
			if arg == "confirmed=false" {
				changed[i] = "confirmed=true"
			}
		}
		if code, _, stderr := runCLI(t, changed...); code == 0 || !strings.Contains(stderr, "project_start_stale_launch") {
			t.Fatalf("changed typed preanswer reused the review: %d %s", code, stderr)
		}
		assertNoEffects(t)
	})
	t.Run("summary_write_failure", func(t *testing.T) {
		writes := 0
		writer := projectSummaryWriter(func(data []byte) (int, error) {
			if bytes.Contains(data, []byte(`"schema_version":"project-launch-summary/2"`)) {
				writes++
				assertNoEffects(t)
			}
			return 0, errors.New("review output unavailable")
		})
		var out bytes.Buffer
		if code := execute(context.Background(), append(start, "--json"), &out, writer); code == 0 || writes != 1 {
			t.Fatalf("summary publication failure did not stop: exit=%d writes=%d", code, writes)
		}
		assertNoEffects(t)
	})
	t.Run("executable_changed_after_summary", func(t *testing.T) {
		writes := 0
		var out, stderr bytes.Buffer
		writer := projectSummaryWriter(func(data []byte) (int, error) {
			if bytes.Contains(data, []byte(`"schema_version":"project-launch-summary/2"`)) {
				writes++
				assertNoEffects(t)
				if err := os.Remove(link); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("/bin/echo", link); err != nil {
					t.Fatal(err)
				}
			}
			return stderr.Write(data)
		})
		defer func() {
			if err := os.Remove(link); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(node, link); err != nil {
				t.Fatal(err)
			}
		}()
		if code := execute(context.Background(), append(start, "--json"), &out, writer); code == 0 || writes != 1 || !strings.Contains(stderr.String(), "project_start_stale_launch") {
			t.Fatalf("last-moment executable replacement was not rejected: exit=%d writes=%d %s", code, writes, stderr.String())
		}
		assertNoEffects(t)
	})
	if current := prepare(); !reflect.DeepEqual(reviewed, current) {
		t.Fatal("restored request changed its review (including generated command identity)")
	}
	var out, stderr bytes.Buffer
	writes := 0
	writer := projectSummaryWriter(func(data []byte) (int, error) {
		if bytes.Contains(data, []byte(`"schema_version":"project-launch-summary/2"`)) {
			writes++
			assertNoEffects(t)
			var actual projectLaunchSummary
			decode(string(data), &actual)
			if !reflect.DeepEqual(reviewed, actual) {
				t.Fatal("published summary differs from the reviewed request")
			}
		}
		return stderr.Write(data)
	})
	if code := execute(context.Background(), append(start, "--json"), &out, writer); code != 0 || writes != 1 {
		t.Fatalf("reviewed start: exit=%d summaries=%d %s", code, writes, stderr.String())
	}
	var started projectStartResult
	decode(out.String(), &started)
	if started.LaunchSummary == nil || !reflect.DeepEqual(reviewed, *started.LaunchSummary) || started.Workspace != nil {
		t.Fatal("start lost the exact summary or acquired a repository claim")
	}
	view := started.Run
	for i := 0; view.Run.Status != "completed" && i < 4; i++ {
		decode(command("--project", authority, "run", "drive", view.Run.ID), &view)
	}
	if view.Run.Status != "completed" || view.Run.Outcome == nil || *view.Run.Outcome != "succeeded" || len(view.Run.Attempts) != 3 {
		t.Fatalf("reviewed native workers failed: %s %+v", view.Run.Status, view.Run.Diagnostics)
	}
	decode(command("--project", authority, "run", "status", view.Run.ID), &view)
	if view.Run.Executors != nil {
		t.Fatal("public status disclosed pinned executable configuration")
	}
	if len(view.Run.DecisionLedger) != 1 || view.Run.DecisionLedger[0].Status != "answered" || view.Run.DecisionLedger[0].Source != "actor" || string(view.Run.DecisionLedger[0].Value) != "false" {
		t.Fatal("reopened terminal ledger lost the reviewed typed preanswer")
	}
	ref, err := json.Marshal(view.Run.Outputs["report"])
	if err != nil {
		t.Fatal(err)
	}
	refFile, report := filepath.Join(t.TempDir(), "ref.json"), filepath.Join(t.TempDir(), "report.txt")
	writeFixtureFile(t, filepath.Dir(refFile), filepath.Base(refFile), string(ref))
	command("--project", authority, "artifact", "export", "--ref", refFile, "--output", report)
	data, err := os.ReadFile(report)
	if err != nil || string(data) != "Pri-Fly CSV report\nRows: 3\nTotal: 24\n" {
		t.Fatalf("reviewed workers produced wrong bytes: %q %v", data, err)
	}
	engine, err := prifly.Open(authority, true)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := engine.Store.Read(context.Background(), view.Run.ID, 0, 1)
	_ = engine.Close()
	if err != nil {
		t.Fatal(err)
	}
	var run prifly.Run
	decode(string(snapshot.Snapshot.Data), &run)
	for _, attempt := range run.Attempts {
		workspace := attempt.Workspace
		if !filepath.IsAbs(workspace) {
			workspace = filepath.Join(authority, workspace)
		}
		marker, err := os.ReadFile(filepath.Join(workspace, "launch-summary-worker-marker"))
		if err != nil || len(marker) == 0 {
			t.Fatalf("native worker was not observed in its real Attempt workspace: %s %v", attempt.ID, err)
		}
	}
	// Installing this exact package and creating a Run do not invalidate an
	// otherwise equal request. Neither belongs to the owner's reviewed inputs.
	if current := prepare(); !reflect.DeepEqual(reviewed, current) {
		t.Fatal("package installation or Run history changed the review digest")
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); !os.IsNotExist(err) {
		t.Fatal("neutral launch created Git state", err)
	}
}

func TestCLIProjectLaunchPrepareRequiresProfile3(t *testing.T) {
	// The published /2 route still requires Git; it must not silently acquire
	// /3's new review or launch semantics merely because a flag was supplied.
	root, authority := newProjectFixture(t)
	for _, args := range [][]string{
		{"project", "questionnaire", "--repository", root, "--launch", "unused", "--prepare"},
		{"project", "start", "--repository", root, "--launch", "unused", "--expected-launch-digest", "unused"},
	} {
		if code, _, stderr := runCLI(t, args...); code == 0 || !strings.Contains(stderr, "project_questionnaire_prepare_requires_profile_3") {
			t.Fatalf("legacy profile silently adopted reviewed launch semantics: %d %s", code, stderr)
		}
	}
	engine, err := prifly.Open(authority, true)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	if runs, err := engine.Runs(context.Background()); err != nil || len(runs) != 0 {
		t.Fatalf("legacy rejection created a Run: %+v %v", runs, err)
	}
}
