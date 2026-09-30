package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// The path a project takes: a package opens one program's output in its
// execution_bindings, the review shows it, the Run is sealed at 43, and the
// output is read beside the Run -- only for that program.
func TestProjectLiveOutputFromBindingToRead(t *testing.T) {
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
	command := func(args ...string) string {
		t.Helper()
		code, out, stderr := runCLI(t, args...)
		if code != 0 {
			t.Fatalf("%v: exit=%d %s", args, code, stderr)
		}
		return out
	}
	command("project", "init", "--repository", root, "--state-root", authority)
	const relative = ".prifly/workflows/csv-report"
	folder := filepath.Join(root, relative)
	for _, name := range []string{"workflow.yaml", "steps/parse.yaml", "steps/validate.yaml", "steps/report.yaml", "schemas/rows.yaml", "files/worker.mjs", "sample.csv"} {
		data, err := os.ReadFile(filepath.Join("../../examples/workflows/csv-report", name))
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		switch name {
		case "workflow.yaml":
			opened := strings.Replace(text, "      args: [worker.mjs, parse]\n      files: {worker.mjs: files/worker.mjs}\n      timeout_ms: 10000\n      grace_ms: 100\n      max_output_bytes: 1048576\n", "      args: [worker.mjs, parse]\n      files: {worker.mjs: files/worker.mjs}\n      timeout_ms: 10000\n      grace_ms: 100\n      max_output_bytes: 1048576\n      live_output: true\n      output_limit_bytes: 131072\n", 1)
			if opened == text {
				t.Fatal("the example's parse binding no longer has the lines this test extends")
			}
			text = opened
		case "files/worker.mjs":
			text = strings.Replace(text, "const envelope =", "console.log('step ' + process.argv[2] + ' says hello');\nconsole.error('verdict: fail');\nconst envelope =", 1)
		}
		writeFixtureFile(t, folder, name, text)
	}
	writeFixtureFile(t, root, ".prifly/project.yaml", `schema_version: prifly-project-profile/3
packages: {csv-report: {source: .prifly/workflows/csv-report}}
launches:
  csv-report:
    title: CSV report
    description: One program opens its output.
    kind: workflow
    workflow: .prifly/workflows/csv-report/workflow.yaml
`)
	command("project", "local", "set", "--repository", root, "--allow-executable", "node="+node)
	args := []string{"--repository", root, "--launch", "csv-report", "--input", "csv=" + filepath.Join(folder, "sample.csv"), "--allow-execution"}
	var reviewed projectLaunchSummary
	if err := json.Unmarshal([]byte(command(append([]string{"project", "questionnaire", "--prepare"}, args...)...)), &reviewed); err != nil {
		t.Fatal(err)
	}
	opened := 0
	for _, program := range reviewed.Execution {
		if program.LiveOutput {
			opened++
			if program.OutputLimitBytes != 131072 || !strings.Contains(program.DefinitionRef.ID, "parse") {
				t.Fatalf("the review misnamed the opened program: %+v", program)
			}
		}
	}
	if opened != 1 {
		t.Fatalf("the review shows %d programs with opened output, want 1", opened)
	}
	command(append(append([]string{"project", "start"}, args...), "--expected-launch-digest", reviewed.ReviewDigest, "--command-id", "live-output-start")...)
	engine, err := prifly.Open(authority, true)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := engine.Runs(context.Background())
	engine.Close()
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs: %+v %v", runs, err)
	}
	runID := runs[0].ID
	var status struct {
		Run prifly.Run `json:"run"`
	}
	if err := json.Unmarshal([]byte(command("--project", authority, "--json", "run", "status", runID)), &status); err != nil {
		t.Fatal(err)
	}
	if status.Run.SchemaVersion != prifly.CoreLiveOutputStateVersion || status.Run.Status != "completed" {
		t.Fatalf("the Run was not sealed at 43 or did not complete: %s %s", status.Run.SchemaVersion, status.Run.Status)
	}
	shown := 0
	for id, attempt := range status.Run.Attempts {
		var view prifly.ProgramOutputView
		if err := json.Unmarshal([]byte(command("--project", authority, "--json", "run", "output", runID, "--attempt", id)), &view); err != nil {
			t.Fatal(err)
		}
		if !view.Disclosed {
			if len(view.Streams) != 0 || view.OutputLimitBytes != prifly.DefaultProgramOutputBytes {
				t.Fatalf("a program that did not open its output was read: %+v", view)
			}
			continue
		}
		shown++
		if attempt.Accepted == nil || attempt.Accepted.Verdict != "pass" {
			t.Fatalf("printed text changed the verdict: %+v", attempt.Accepted)
		}
		text := map[string]string{}
		for _, stream := range view.Streams {
			text[stream.Stream] = stream.Text
		}
		if !strings.Contains(text["stdout"], "step parse says hello") || !strings.Contains(text["stderr"], "verdict: fail") {
			t.Fatalf("the opened output was not read back: %+v", view)
		}
	}
	if shown != 1 {
		t.Fatalf("%d attempts disclosed output, want exactly the parse step", shown)
	}
}
