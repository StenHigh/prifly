package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every refusal of the project surface used to reach the wire as code
// "invalid_usage" with its real code inside the sentence, so a reader matching
// on `code` saw one refusal where there are a hundred and five, and a reader
// wanting the actual one had to split prose. refusal-check forbade exactly this
// shape for errors.New and fmt.Errorf and never looked at usageError, which is
// how the rule held everywhere except the surface carrying the most codes.
func TestProjectRefusalsCarryTheirCodeInTheEnvelope(t *testing.T) {
	root, authority := t.TempDir(), filepath.Join(t.TempDir(), "authority")
	if code, _, stderr := runCLI(t, "project", "init", "--repository", root, "--state-root", authority, "--host", "codex-cli"); code != 0 {
		t.Fatalf("init: %d %s", code, stderr)
	}
	if err := os.WriteFile(filepath.Join(root, ".prifly/project.yaml"), []byte("schema_version: prifly-project-profile/3\nhosts: {codex-cli: .codex/skills}\npackages: {}\nlaunches: {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, code string
		args       []string
		exit       int
	}{
		{"unknown launch", "project_start_unknown_launch", []string{"--project", authority, "project", "questionnaire", "--repository", root, "--launch", "no-such-launch"}, 2},
		{"missing repository", "project_root_invalid", []string{"project", "workflows", "--repository", filepath.Join(root, "no-such-directory")}, 2},
		{"unallowed program", "project_local_invalid_executable", []string{"project", "local", "set", "--repository", root, "--allow-executable", "shell=relative/path"}, 2},
	} {
		var out, errout bytes.Buffer
		exit := execute(context.Background(), append(c.args, "--json"), &out, &errout)
		if exit == 0 {
			t.Fatalf("%s: was accepted", c.name)
		}
		var problem struct {
			Code            string   `json:"code"`
			Message         string   `json:"message"`
			SafeNextActions []string `json:"safe_next_actions"`
		}
		if err := json.Unmarshal([]byte(lastLine(errout.String())), &problem); err != nil {
			t.Fatalf("%s: %v: %s", c.name, err, errout.String())
		}
		if problem.Code != c.code {
			t.Errorf("%s: code is %s, expected %s (%s)", c.name, problem.Code, c.code, problem.Message)
		}
		// The code moved out of the sentence, so it must not still be in it.
		if strings.HasPrefix(problem.Message, problem.Code+":") {
			t.Errorf("%s: the code is still inside its own message: %s", c.name, problem.Message)
		}
		if problem.Message == "" {
			t.Errorf("%s: the refusal says nothing beyond its code", c.name)
		}
		// A mistyped declaration is not a state conflict, and the project
		// surface answers "help" unless a code names a better move.
		if exit != c.exit {
			t.Errorf("%s: exited %d, expected %d", c.name, exit, c.exit)
		}
		if len(problem.SafeNextActions) == 0 {
			t.Errorf("%s: no safe next action was offered", c.name)
		}
	}
}
