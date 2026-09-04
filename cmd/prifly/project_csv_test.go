package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// This exercises the shipped files through the public CLI. Node is a declared
// native worker, not an AI host; no Git executable or repository is available.
func TestCLIProjectCSVReportNoGitNoAI(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("the runnable CSV example requires Node")
	}
	// PATH may name a version-manager shell shim. Pin the actual Node binary,
	// which does not depend on a login shell or the parent's PATH at dispatch.
	resolveContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	resolved, err := exec.CommandContext(resolveContext, node, "-p", "process.execPath").Output()
	cancel()
	if err != nil || !filepath.IsAbs(strings.TrimSpace(string(resolved))) {
		t.Fatalf("resolve actual Node executable: %v %q", err, resolved)
	}
	node = strings.TrimSpace(string(resolved))
	source, err := filepath.Abs("../../examples/workflows/csv-report")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	root, state := t.TempDir(), filepath.Join(t.TempDir(), "authority")
	command := func(args ...string) string {
		t.Helper()
		code, out, stderr := runCLI(t, args...)
		if code != 0 {
			t.Fatalf("%v: exit=%d %s", args, code, stderr)
		}
		return out
	}
	command("project", "init", "--repository", root, "--state-root", state)
	folder := filepath.Join(root, ".prifly", "workflows", "csv-report")
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
	writeFixtureFile(t, root, ".prifly/project.yaml", `schema_version: prifly-project-profile/3
packages:
  csv-report: {source: .prifly/workflows/csv-report}
launches:
  csv-report:
    title: CSV report
    description: Parse, validate and summarize a CSV file.
    kind: workflow
    workflow: .prifly/workflows/csv-report/workflow.yaml
`)
	listed := command("project", "workflows", "--repository", root)
	if !strings.Contains(listed, `"name":"csv"`) || !strings.Contains(listed, `"required":true`) {
		t.Fatalf("list lost typed CSV input: %s", listed)
	}
	questionnaire := command("project", "questionnaire", "--repository", root, "--launch", "csv-report")
	if !strings.Contains(questionnaire, `"preflight":[]`) {
		t.Fatalf("ordinary workflow manufactured a questionnaire: %s", questionnaire)
	}
	command("project", "compile", "--repository", root, "--package", "csv-report", "--output", filepath.Join(t.TempDir(), "package"))
	csv := filepath.Join(folder, "sample.csv")
	start := []string{"project", "start", "--repository", root, "--launch", "csv-report"}
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"--input", "csv=" + csv}, "project_execution_approval_required"},
		{[]string{"--input", "csv=" + csv, "--allow-execution"}, "project_execution_not_allowed"},
	} {
		args := append(append([]string{}, start...), test.args...)
		if code, _, stderr := runCLI(t, args...); code == 0 || !strings.Contains(stderr, test.want) {
			t.Fatalf("expected %s before import: %v %s", test.want, args, stderr)
		}
	}
	command("project", "local", "set", "--repository", root, "--allow-executable", "node="+node)
	if code, _, stderr := runCLI(t, append(append([]string{}, start...), "--allow-execution")...); code == 0 || !strings.Contains(stderr, `"code":"missing_input"`) {
		t.Fatalf("missing typed CSV was not rejected before import: exit=%d %s", code, stderr)
	}
	engine, err := prifly.Open(state, true)
	if err != nil {
		t.Fatal(err)
	}
	packages, err := engine.Packages(context.Background())
	if err != nil || len(packages.Packages) != 0 {
		t.Fatalf("rejected preflight imported packages: %+v %v", packages, err)
	}
	_ = engine.Close()
	unchanged := map[string][]byte{}
	for _, path := range []string{filepath.Join(root, ".prifly/project.yaml"), filepath.Join(root, ".prifly/local.yaml"), filepath.Join(state, "prifly.json")} {
		unchanged[path], err = os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
	}
	var started projectStartResult
	out := command(append(start, "--input", "csv="+csv, "--allow-execution")...)
	if err := json.Unmarshal([]byte(out), &started); err != nil {
		t.Fatal(err)
	}
	view := started.Run
	for i := 0; view.Run.Status != "completed" && i < 4; i++ {
		out = command("--project", state, "run", "drive", view.Run.ID)
		if err := json.Unmarshal([]byte(out), &view); err != nil {
			t.Fatal(err)
		}
	}
	if view.Run.Status != "completed" || view.Run.Outcome == nil || *view.Run.Outcome != "succeeded" || len(view.Run.Attempts) != 3 {
		for _, attempt := range view.Run.Attempts {
			t.Logf("attempt %s: status=%s result=%+v", attempt.ID, attempt.Status, attempt.Accepted)
		}
		t.Fatalf("CSV pipeline did not succeed in three real workers: status=%s outcome=%v diagnostics=%+v", view.Run.Status, view.Run.Outcome, view.Run.Diagnostics)
	}
	if started.Workspace != nil || view.Run.Brief != (prifly.ArtifactRef{}) || view.Run.DecisionCatalog != nil || view.Run.DecisionSheet != nil {
		t.Fatal("ordinary workflow acquired an AI/task/workspace contract")
	}
	if bytes.Contains([]byte(out), []byte(`"brief_ref"`)) {
		t.Fatal("absent RunBrief serialized as a fake reference")
	}
	// Every CLI call opens a fresh owner. Explicitly read again and export the
	// sealed bytes instead of trusting a worker's summary or an in-memory result.
	var reopened prifly.RunView
	if err := json.Unmarshal([]byte(command("--project", state, "run", "status", view.Run.ID)), &reopened); err != nil {
		t.Fatal(err)
	}
	ref, err := json.Marshal(reopened.Run.Outputs["report"])
	if err != nil {
		t.Fatal(err)
	}
	refFile := filepath.Join(t.TempDir(), "report-ref.json")
	writeFixtureFile(t, filepath.Dir(refFile), filepath.Base(refFile), string(ref))
	report := filepath.Join(t.TempDir(), "report.txt")
	command("--project", state, "artifact", "export", "--ref", refFile, "--output", report)
	data, err := os.ReadFile(report)
	if err != nil || string(data) != "Pri-Fly CSV report\nRows: 3\nTotal: 24\n" {
		t.Fatalf("wrong actual report bytes: %q %v", data, err)
	}
	engine, err = prifly.Open(state, true)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	claims, err := engine.Claims(context.Background())
	if err != nil || len(claims.Claims) != 0 {
		t.Fatalf("CSV workflow claimed a repository: %+v %v", claims, err)
	}
	for path, before := range unchanged {
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatalf("launch changed shared/local/global configuration %s: %v", path, err)
		}
	}
	for _, name := range []string{".git", ".agents", ".codex", ".claude", ".prifly/state"} {
		if _, err := os.Lstat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatalf("neutral execution manufactured %s: %v", name, err)
		}
	}
}
