package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func TestCLIProjectNeutralInitAndExplicitHosts(t *testing.T) {
	// Any accidental Git subprocess fails: this path must work without Git.
	t.Setenv("PATH", t.TempDir())
	root, authority := t.TempDir(), filepath.Join(t.TempDir(), "authority")
	if code, _, stderr := runCLI(t, "project", "init", "--repository", root, "--state-root", authority); code != 0 {
		t.Fatalf("neutral init: %d %s", code, stderr)
	}
	profilePath := filepath.Join(root, ".prifly", "project.yaml")
	before, err := os.ReadFile(profilePath)
	if err != nil || string(before) != projectProfileSource {
		t.Fatalf("default profile: %v %q", err, before)
	}
	for _, path := range []string{".git", ".agents", ".codex", ".claude", ".prifly/state"} {
		if _, err := os.Lstat(filepath.Join(root, path)); !os.IsNotExist(err) {
			t.Fatalf("neutral init created %s: %v", path, err)
		}
	}
	engine, err := prifly.Open(authority, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkProjectAuthority(engine); err != nil {
		t.Fatal(err)
	}
	_ = engine.Close()
	nested := filepath.Join(root, "src", "nested")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	if code, out, stderr := runCLI(t, "project", "workflows", "--repository", nested); code != 0 || !strings.Contains(out, `"launches":[]`) {
		t.Fatalf("neutral discovery/list: %d %s %s", code, out, stderr)
	}
	writeFixtureWorkflowFolder(t, root, ".prifly/workflows/report", "report")
	writeFixtureFile(t, root, ".prifly/project.yaml", strings.Replace(projectProfileSource, "packages: {}", "packages:\n  report: {source: .prifly/workflows/report}", 1))
	if code, out, stderr := runCLI(t, "project", "questionnaire", "--repository", nested, "--package", "report"); code != 0 || !strings.Contains(out, `"preflight":[]`) {
		t.Fatalf("neutral questionnaire: %d %s %s", code, out, stderr)
	}
	if code, out, stderr := runCLI(t, "project", "runners", "update", "--repository", root); code != 0 || !strings.Contains(out, `"updated_hosts":[]`) {
		t.Fatalf("no declared runners: %d %s %s", code, out, stderr)
	}
	hostless, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, ".prifly", "local.yaml")); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runCLI(t, "project", "init", "--repository", nested, "--state-root", filepath.Join(t.TempDir(), "hostless-copy")); code != 0 {
		t.Fatalf("hostless copy bootstrap: %d %s", code, stderr)
	}
	afterBootstrap, err := os.ReadFile(profilePath)
	if err != nil || !bytes.Equal(hostless, afterBootstrap) {
		t.Fatalf("hostless bootstrap rewrote shared profile: %v", err)
	}
	if code, _, stderr := runCLI(t, "project", "runners", "add", "--repository", root, "--host", "claude-code"); code != 0 {
		t.Fatalf("explicit runner attach: %d %s", code, stderr)
	}
	profile, err := readProjectProfile(root)
	if err != nil || len(profile.HostSkillsRoots) != 1 || profile.HostSkillsRoots["claude-code"] != ".claude/skills" {
		t.Fatalf("selected hosts: %v %+v", err, profile.HostSkillsRoots)
	}
	for _, directory := range []string{".codex", ".agents"} {
		if _, err := os.Lstat(filepath.Join(root, directory)); !os.IsNotExist(err) {
			t.Fatalf("unselected host created: %s %v", directory, err)
		}
	}
	claude := projectHosts[2]
	frozen := projectPreviousRunnerSkill(claude)
	writeFixtureFile(t, root, ".claude/skills/prifly-run/SKILL.md", frozen)
	shared, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, ".prifly", "local.yaml")); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runCLI(t, "project", "init", "--repository", nested, "--state-root", filepath.Join(t.TempDir(), "copied-authority")); code != 0 {
		t.Fatalf("copy bootstrap: %d %s", code, stderr)
	}
	if code, out, stderr := runCLI(t, "project", "runners", "add", "--repository", root, "--host", "claude-code"); code != 0 || !strings.Contains(out, `"added_hosts":[]`) {
		t.Fatalf("reattach changed frozen runner: %d %s %s", code, out, stderr)
	}
	after, err := os.ReadFile(profilePath)
	if err != nil || !bytes.Equal(shared, after) {
		t.Fatalf("bootstrap or reattach rewrote shared profile: %v", err)
	}
	runner, err := os.ReadFile(filepath.Join(projectRunnerPath(root, claude), "SKILL.md"))
	if err != nil || string(runner) != frozen {
		t.Fatalf("bootstrap or reattach rewrote frozen runner: %v", err)
	}
	writeFixtureFile(t, root, ".agents/skills/prifly-run/SKILL.md", "custom instructions\n")
	if code, _, stderr := runCLI(t, "project", "runners", "add", "--repository", root, "--host", "codex-cli", "--host", "codex-app"); code == 0 || !strings.Contains(stderr, "project_runner_conflict") {
		t.Fatalf("attach ignored existing runner: %d %s", code, stderr)
	}
	if _, err := os.Lstat(filepath.Join(root, ".codex")); !os.IsNotExist(err) {
		t.Fatalf("conflict partially created earlier runner: %v", err)
	}
	after, err = os.ReadFile(profilePath)
	if err != nil || !bytes.Equal(shared, after) {
		t.Fatalf("rejected attach rewrote profile: %v", err)
	}
}

func TestCLIProjectNeutralInitRejectsUnsafeSelection(t *testing.T) {
	for _, failure := range []string{"unknown", "duplicate", "symlink", "file"} {
		t.Run(failure, func(t *testing.T) {
			root, authority := t.TempDir(), filepath.Join(t.TempDir(), "authority")
			args := []string{"project", "init", "--repository", root, "--state-root", authority, "--host", "codex-cli"}
			want := "project_runner_conflict"
			switch failure {
			case "unknown":
				args = append(args, "--host", "unknown")
				want = "project_runner_invalid_host"
			case "duplicate":
				args = append(args, "--host", "codex-cli")
				want = "project_runner_invalid_host"
			case "symlink":
				if err := os.Symlink(t.TempDir(), filepath.Join(root, ".codex")); err != nil {
					t.Fatal(err)
				}
			case "file":
				writeFixtureFile(t, root, ".codex", "not a directory")
			}
			if code, _, stderr := runCLI(t, args...); code == 0 || !strings.Contains(stderr, want) {
				t.Fatalf("unsafe selection: %d %s", code, stderr)
			}
			for _, path := range []string{filepath.Join(root, ".prifly"), authority} {
				if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Fatalf("failed init mutated %s: %v", path, err)
				}
			}
		})
	}
}

func TestProjectNeutralProfileSchemasAndLegacyGitRequirement(t *testing.T) {
	root := t.TempDir()
	for _, test := range []struct {
		name, profile string
		valid         bool
	}{
		{"neutral", projectProfileSource, true},
		{"selected", projectProfileSource + "hosts: {claude-code: .claude/skills}\n", true},
		{"unknown", projectProfileSource + "hosts: {unknown: .unknown/skills}\n", false},
		{"wrong-root", projectProfileSource + "hosts: {claude-code: .codex/skills}\n", false},
		{"legacy-missing-hosts", strings.Replace(projectProfileSource, "/3", "/2", 1), false},
		{"legacy", "schema_version: prifly-project-profile/2\n" + projectHostsYAML + "packages: {}\nlaunches: {}\n", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			writeFixtureFile(t, root, ".prifly/project.yaml", test.profile)
			_, err := readProjectProfile(root)
			if (err == nil) != test.valid {
				t.Fatalf("profile parser: valid=%v err=%v", test.valid, err)
			}
			value, err := flow.Parse([]byte(test.profile), "yaml")
			if err != nil {
				t.Fatal(err)
			}
			data, _ := json.Marshal(value)
			for _, schemaName := range []string{"project-profile.schema.json", "project-profile-v3.schema.json", "project-profile-v2.schema.json"} {
				schema, err := os.ReadFile(filepath.Join("..", "..", "schemas", "authoring", schemaName))
				if err != nil {
					t.Fatal(err)
				}
				schema, err = flow.Canonical(schema)
				if err != nil {
					t.Fatal(err)
				}
				ref := flow.Ref{ID: "test:schema/profile", Version: "1.0.0", Digest: fmt.Sprintf("sha256:%x", sha256.Sum256(schema))}
				err = flow.ValidateSchema(flow.Registry{ref: schema}, ref, data)
				valid := test.valid && (schemaName == "project-profile.schema.json" || (schemaName == "project-profile-v2.schema.json") == strings.Contains(test.profile, "profile/2"))
				if (err == nil) != valid {
					t.Fatalf("%s: valid=%v err=%v", schemaName, valid, err)
				}
			}
		})
	}
	// The last case is valid /2, but not a Git repository. New discovery must
	// not silently reinterpret that published profile as the neutral contract.
	t.Setenv("PATH", t.TempDir())
	if _, err := projectRoot(context.Background(), root); err == nil || !strings.Contains(err.Error(), "repository_required") {
		t.Fatalf("legacy /2 lost its Git requirement: %v", err)
	}
}
