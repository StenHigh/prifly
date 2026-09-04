package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/release"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

const projectHostsYAML = `hosts:
  codex-cli: .codex/skills
  codex-app: .agents/skills
  claude-code: .claude/skills
`

func TestMain(m *testing.M) {
	if handled, code := flow.SchemaWorker(os.Args[1:], os.Stdin, os.Stdout); handled {
		os.Exit(code)
	}
	os.Exit(m.Run())
}

func emptyCLIWorkflow(t *testing.T) flow.WorkflowRevision {
	t.Helper()
	defs, _, err := prifly.Builtins()
	if err != nil {
		t.Fatal(err)
	}
	var policy flow.Ref
	for _, definition := range defs {
		if definition.Ref.ID == "core:policy/local" && definition.Ref.Version == "1.0.0" {
			policy = definition.Ref
		}
	}
	w := flow.WorkflowRevision{SchemaVersion: "1", ID: "test:workflow/cli", Version: "1.0.0", Title: "Local CLI contract fixture", Inputs: map[string]flow.InputPort{}, Outputs: map[string]flow.OutputPort{}, AllowedOutcomes: []string{"no_work"}, Limits: flow.Limits{MaxStepInstances: 1, MaxControlTransitions: 4, MaxParallelism: 1}, PolicyRef: policy}
	w.Definition.Entry = "done"
	w.Definition.Stages = map[string]flow.Stage{"done": {Kind: "finish", Outcome: "no_work", OutputBindings: map[string]flow.Binding{}}}
	return w
}

func TestCLIEmptyInstallAndStableProblems(t *testing.T) {
	project := filepath.Join(t.TempDir(), "plain-install")
	var out, errout bytes.Buffer
	if code := execute(context.Background(), []string{"init", project, "--json"}, &out, &errout); code != 0 {
		t.Fatalf("init %d: %s", code, errout.String())
	}
	for _, operation := range []string{"version", "doctor", "inventory"} {
		out.Reset()
		errout.Reset()
		if code := execute(context.Background(), []string{"--project", project, operation, "--json"}, &out, &errout); code != 0 || !json.Valid(out.Bytes()) {
			t.Fatalf("%s: %d %s", operation, code, errout.String())
		}
	}
	out.Reset()
	errout.Reset()
	code := execute(context.Background(), []string{"--project", project, "run", "retry", "run:unknown", "--json"}, &out, &errout)
	if code != 2 {
		t.Fatalf("unsupported operation: %d %s", code, errout.String())
	}
	if out.Len() != 0 {
		t.Fatal("problem mixed with stdout response")
	}
	if err := flow.ValidateProtocol("Problem", errout.Bytes()); err != nil {
		t.Fatalf("closed Problem: %v (%s)", err, errout.String())
	}
	if _, err := os.Stat(filepath.Join(project, ".git")); !os.IsNotExist(err) {
		t.Fatal("empty install acquired Git dependency")
	}
}

func TestCLIUpdateHasStructuredResultAndRejectsArgumentsBeforeNetwork(t *testing.T) {
	prior := updateBinary
	t.Cleanup(func() { updateBinary = prior })
	called := 0
	updateBinary = func(context.Context, string) (release.Result, error) {
		called++
		return release.Result{SchemaVersion: "prifly-update/1", PreviousVersion: "1.0.0", Version: "1.1.0", Updated: true}, nil
	}
	var out, errout bytes.Buffer
	if code := execute(context.Background(), []string{"update", "--json"}, &out, &errout); code != 0 || errout.Len() != 0 {
		t.Fatalf("update: %d %s", code, errout.String())
	}
	var result release.Result
	if err := json.Unmarshal(out.Bytes(), &result); err != nil || !result.Updated || result.Version != "1.1.0" || called != 1 {
		t.Fatalf("unexpected update result: %+v calls=%d err=%v", result, called, err)
	}
	out.Reset()
	errout.Reset()
	if code := execute(context.Background(), []string{"update", "unexpected", "--json"}, &out, &errout); code == 0 || called != 1 || !strings.Contains(errout.String(), "invalid_usage") {
		t.Fatalf("invalid update arguments started an update: %d calls=%d %s", code, called, errout.String())
	}
}

func TestCLIProjectInitCreatesTrackedProfileAndSeparateAuthority(t *testing.T) {
	repository := filepath.Join(t.TempDir(), "repository")
	if err := os.MkdirAll(repository, 0755); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("git", "init", "-q", repository).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	stateRoot := filepath.Join(t.TempDir(), "authority")
	canonicalRepository, err := canonicalProjectPath(repository)
	if err != nil {
		t.Fatal(err)
	}
	canonicalState, err := canonicalProjectPath(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	canonicalExecutable, err := projectExecutable()
	if err != nil {
		t.Fatal(err)
	}
	var out, errout bytes.Buffer
	args := []string{"project", "init", "--repository", repository, "--state-root", stateRoot, "--json"}
	if code := execute(context.Background(), args, &out, &errout); code != 0 {
		t.Fatalf("project init %d: %s", code, errout.String())
	}
	var result struct {
		SchemaVersion string `json:"schema_version"`
		Repository    string `json:"repository"`
		Profile       string `json:"profile"`
		AuthorityRoot string `json:"authority_root"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.SchemaVersion != "project-profile-init/1" || result.Repository != canonicalRepository || result.Profile != filepath.Join(canonicalRepository, ".prifly") || result.AuthorityRoot != canonicalState {
		t.Fatalf("unexpected project init result: %+v", result)
	}
	profile, err := os.ReadFile(filepath.Join(repository, ".prifly", "project.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(profile), "schema_version: prifly-project-profile/2") || !strings.Contains(string(profile), projectHostsYAML) || !strings.Contains(string(profile), "packages: {}") || !strings.Contains(string(profile), "launches: {}") {
		t.Fatalf("project profile omitted shared workflow settings: %s", profile)
	}
	for _, legacyRoot := range []string{"workflows_root:", "steps_root:", "schemas_root:", "locks_root:"} {
		if strings.Contains(string(profile), legacyRoot) {
			t.Fatalf("project profile contains obsolete root %q: %s", legacyRoot, profile)
		}
	}
	ignored, err := os.ReadFile(filepath.Join(repository, ".prifly", ".gitignore"))
	if err != nil || string(ignored) != "local.yaml\n" {
		t.Fatalf("local override is not isolated from Git: %v %q", err, ignored)
	}
	local, err := os.ReadFile(filepath.Join(repository, ".prifly", "local.yaml"))
	if err != nil || !strings.Contains(string(local), fmt.Sprintf("authority_root: %q", canonicalState)) || !strings.Contains(string(local), fmt.Sprintf("prifly_executable: %q", canonicalExecutable)) {
		t.Fatalf("local override does not point at the created authority: %v %q", err, local)
	}
	if _, err := os.Stat(filepath.Join(repository, ".prifly", "aif-cycle.py")); !os.IsNotExist(err) {
		t.Fatalf("generic project profile unexpectedly installed an AI Factory recipe: %v", err)
	}
	for _, unused := range []string{"steps", "schemas", "locks"} {
		if _, err := os.Stat(filepath.Join(repository, ".prifly", unused)); !os.IsNotExist(err) {
			t.Fatalf("project profile created an unused root directory %q: %v", unused, err)
		}
	}
	for _, host := range projectHosts {
		runner, err := os.ReadFile(filepath.Join(projectRunnerPath(repository, host), "SKILL.md"))
		questionTool := "request_user_input"
		if host.ID == "claude-code" {
			questionTool = "AskUserQuestion"
		}
		if err != nil || string(runner) != projectRunnerSkill(host) || !strings.Contains(string(runner), "project workflows --repository") || strings.Contains(string(runner), "task_recipe") || !strings.Contains(string(runner), "prifly_executable") || !strings.Contains(string(runner), "--host "+host.ID) || !strings.Contains(string(runner), questionTool) || !strings.Contains(string(runner), "deterministic\npages") || !strings.Contains(string(runner), "wait without mutation") || !strings.Contains(string(runner), "RunBrief text") || !strings.Contains(string(runner), "project questionnaire") || !strings.Contains(string(runner), "attended or autonomous policy") || !strings.Contains(string(runner), "waiting_decision") || !strings.Contains(string(runner), "--request-digest") || !strings.Contains(string(runner), "--envelope-digest") || !strings.Contains(string(runner), "project start") || !strings.Contains(string(runner), "project workflows search") || !strings.Contains(string(runner), "project workflows add NAME") {
			t.Fatalf("project profile omitted the %s runner skill: %v %q", host.ID, err, runner)
		}
	}
	authority, err := prifly.Open(canonicalState, true)
	if err != nil {
		t.Fatalf("authority was not initialized outside repository: %v", err)
	}
	defer authority.Close()
	if authority.Config.Configuration.SchemaVersion != prifly.CoreContextConfigVersion {
		t.Fatalf("project authority cannot seal pinned context: %+v", authority.Config.Configuration)
	}
	if _, err := os.Stat(filepath.Join(repository, ".prifly", "state")); !os.IsNotExist(err) {
		t.Fatalf("repository profile received authority state: %v", err)
	}
	out.Reset()
	errout.Reset()
	if code := execute(context.Background(), []string{"project", "workflows", "--repository", repository, "--json"}, &out, &errout); code != 0 || !strings.Contains(out.String(), `"launches":[]`) {
		t.Fatalf("empty neutral profile is not readable: %d %s %s", code, out.String(), errout.String())
	}
	out.Reset()
	errout.Reset()
	if code := execute(context.Background(), args, &out, &errout); code == 0 || !strings.Contains(errout.String(), "project_local_conflict") {
		t.Fatalf("second project init overwrote an existing local configuration: %d %s", code, errout.String())
	}
	profileBefore := append([]byte(nil), profile...)
	runnersBefore := make(map[string][]byte, len(projectHosts))
	for _, host := range projectHosts {
		data, err := os.ReadFile(filepath.Join(projectRunnerPath(repository, host), "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		legacy := []byte(projectPreviousRunnerSkill(host))
		if err := os.WriteFile(filepath.Join(projectRunnerPath(repository, host), "SKILL.md"), legacy, 0644); err != nil {
			t.Fatal(err)
		}
		runnersBefore[host.ID] = legacy
		if string(data) != projectRunnerSkill(host) {
			t.Fatalf("fresh %s runner did not use the current template", host.ID)
		}
	}
	if err := os.Remove(filepath.Join(repository, ".prifly", "local.yaml")); err != nil {
		t.Fatal(err)
	}
	cloneState := filepath.Join(t.TempDir(), "clone-authority")
	canonicalCloneState, err := canonicalProjectPath(cloneState)
	if err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errout.Reset()
	if code := execute(context.Background(), []string{"project", "init", "--repository", repository, "--state-root", cloneState, "--json"}, &out, &errout); code != 0 {
		t.Fatalf("project init did not bootstrap a cloned profile: %d %s", code, errout.String())
	}
	profileAfter, err := os.ReadFile(filepath.Join(repository, ".prifly", "project.yaml"))
	if err != nil || string(profileAfter) != string(profileBefore) {
		t.Fatalf("clone bootstrap overwrote project profile: %v", err)
	}
	for _, host := range projectHosts {
		data, err := os.ReadFile(filepath.Join(projectRunnerPath(repository, host), "SKILL.md"))
		if err != nil || string(data) != string(runnersBefore[host.ID]) {
			t.Fatalf("clone bootstrap overwrote %s runner: %v", host.ID, err)
		}
	}
	cloneLocal, err := os.ReadFile(filepath.Join(repository, ".prifly", "local.yaml"))
	if err != nil || !strings.Contains(string(cloneLocal), fmt.Sprintf("authority_root: %q", canonicalCloneState)) {
		t.Fatalf("clone bootstrap did not create only its local authority setting: %v %q", err, cloneLocal)
	}
	if err := os.Remove(filepath.Join(repository, ".prifly", "local.yaml")); err != nil {
		t.Fatal(err)
	}
	unknown := []byte("unknown runner\n")
	path := filepath.Join(projectRunnerPath(repository, projectHosts[0]), "SKILL.md")
	if err := os.WriteFile(path, unknown, 0644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errout.Reset()
	if code := execute(context.Background(), []string{"project", "init", "--repository", repository, "--state-root", filepath.Join(t.TempDir(), "rejected-authority"), "--json"}, &out, &errout); code == 0 || !strings.Contains(errout.String(), "project_runner_conflict") {
		t.Fatalf("clone bootstrap accepted an unknown runner: %d %s", code, errout.String())
	}
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, unknown) {
		t.Fatalf("clone bootstrap overwrote an unknown runner: %v %q", err, data)
	}
}

func TestCLIProjectRunnersUpdateOnlyKnownTemplates(t *testing.T) {
	repository := filepath.Join(t.TempDir(), "repository")
	if err := os.MkdirAll(repository, 0755); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("git", "init", "-q", repository).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	var out, errout bytes.Buffer
	if code := execute(context.Background(), []string{"project", "init", "--repository", repository, "--state-root", filepath.Join(t.TempDir(), "authority"), "--json"}, &out, &errout); code != 0 {
		t.Fatalf("project init %d: %s", code, errout.String())
	}
	for index, host := range projectHosts {
		previous := projectRunnerSkillBeforeDecisionBridge(host)
		if index == 0 {
			previous = projectRunnerSkillBeforeCatalog(host)
		}
		if err := os.WriteFile(filepath.Join(projectRunnerPath(repository, host), "SKILL.md"), []byte(previous), 0644); err != nil {
			t.Fatal(err)
		}
	}
	out.Reset()
	errout.Reset()
	if code := execute(context.Background(), []string{"project", "runners", "update", "--repository", repository, "--json"}, &out, &errout); code != 0 {
		t.Fatalf("runner update %d: %s", code, errout.String())
	}
	var result struct {
		UpdatedHosts []string `json:"updated_hosts"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil || len(result.UpdatedHosts) != len(projectHosts) {
		t.Fatalf("runner update result: %v %#v", err, result)
	}
	for _, host := range projectHosts {
		data, err := os.ReadFile(filepath.Join(projectRunnerPath(repository, host), "SKILL.md"))
		if err != nil || string(data) != projectRunnerSkill(host) {
			t.Fatalf("runner %s was not updated: %v", host.ID, err)
		}
	}
	modified := projectHosts[0]
	stale := projectHosts[1]
	stalePath := filepath.Join(projectRunnerPath(repository, stale), "SKILL.md")
	if err := os.WriteFile(filepath.Join(projectRunnerPath(repository, modified), "SKILL.md"), []byte("local customization\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stalePath, []byte(projectRunnerSkillBeforeDecisionBridge(stale)), 0644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errout.Reset()
	if code := execute(context.Background(), []string{"project", "runners", "update", "--repository", repository, "--json"}, &out, &errout); code == 0 || !strings.Contains(errout.String(), "project_runner_conflict") {
		t.Fatalf("runner update overwrote a customization: %d %s", code, errout.String())
	}
	data, err := os.ReadFile(stalePath)
	if err != nil || string(data) != projectRunnerSkillBeforeDecisionBridge(stale) {
		t.Fatalf("runner update partially replaced a stale file: %v", err)
	}
}

func TestCLIProjectWorkflowsRejectsLegacyAuthoring(t *testing.T) {
	repository := filepath.Join(t.TempDir(), "repository")
	if err := os.MkdirAll(repository, 0755); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("git", "init", "-q", repository).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	var out, errout bytes.Buffer
	if code := execute(context.Background(), []string{"project", "init", "--repository", repository, "--state-root", filepath.Join(t.TempDir(), "authority"), "--json"}, &out, &errout); code != 0 {
		t.Fatalf("project init %d: %s", code, errout.String())
	}
	profile := `schema_version: prifly-project-profile/1
skills_root: .codex/skills
launches: {}
`
	if err := os.WriteFile(filepath.Join(repository, ".prifly", "project.yaml"), []byte(profile), 0644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errout.Reset()
	if code := execute(context.Background(), []string{"project", "workflows", "--repository", repository, "--json"}, &out, &errout); code == 0 || !strings.Contains(errout.String(), "schema_version must be prifly-project-profile/2") {
		t.Fatalf("v1 profile was accepted: %d %s", code, errout.String())
	}
	profile = "schema_version: prifly-project-profile/2\n" + projectHostsYAML + `packages: {}
launches:
  legacy:
    title: Legacy recipe
    description: Must be rejected before execution.
    kind: task_recipe
    workflow: .prifly/workflows/legacy/workflow.yaml
`
	if err := os.WriteFile(filepath.Join(repository, ".prifly", "project.yaml"), []byte(profile), 0644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errout.Reset()
	if code := execute(context.Background(), []string{"project", "workflows", "--repository", repository, "--json"}, &out, &errout); code == 0 || !strings.Contains(errout.String(), "kind must be workflow") {
		t.Fatalf("task recipe was accepted: %d %s", code, errout.String())
	}
	if err := os.MkdirAll(filepath.Join(repository, ".prifly", "workflows", "legacy"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, ".prifly", "workflows", "legacy", "workflow.yaml"), []byte("id: test:workflow/legacy\n"), 0644); err != nil {
		t.Fatal(err)
	}
	profile = strings.Replace(profile, "kind: task_recipe", "kind: workflow", 1)
	if err := os.WriteFile(filepath.Join(repository, ".prifly", "project.yaml"), []byte(profile), 0644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errout.Reset()
	if code := execute(context.Background(), []string{"project", "workflows", "--repository", repository, "--json"}, &out, &errout); code == 0 || !strings.Contains(errout.String(), "must be a prifly-project-workflow/1 folder root") {
		t.Fatalf("direct workflow was accepted: %d %s", code, errout.String())
	}
}

func TestCLIProjectExtendInsertsOneDeclaredStep(t *testing.T) {
	dir := t.TempDir()
	workflowPath := filepath.Join(dir, "cycle.yaml")
	workflow := `id: test:workflow/cycle
definition:
  stages:
    build: {kind: repeat, on_complete: {succeeded: commit}}
    commit: {kind: finish, outcome: succeeded}
`
	if err := os.WriteFile(workflowPath, []byte(workflow), 0600); err != nil {
		t.Fatal(err)
	}
	extensionsPath := filepath.Join(dir, "extensions.yaml")
	extensions := `extensions:
  - id: qa-after-build
    workflow: cycle
    between: {from: build, to: commit}
    step: qa
    on: {pass: commit}
`
	if err := os.WriteFile(extensionsPath, []byte(extensions), 0600); err != nil {
		t.Fatal(err)
	}
	stepPath := filepath.Join(dir, "qa.yaml")
	if err := os.WriteFile(stepPath, []byte("inputs: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	stepRef := `{"id":"test:step/qa","version":"1.0.0","digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`
	outputPath := filepath.Join(dir, "compiled.json")
	var out, errout bytes.Buffer
	args := []string{"project", "extend", "--workflow", workflowPath, "--workflow-id", "test:workflow/cycle", "--extensions", extensionsPath,
		"--output", outputPath, "--step-ref", "qa=" + stepRef, "--step-source", "qa=" + stepPath, "--json"}
	if code := execute(context.Background(), args, &out, &errout); code != 0 {
		t.Fatalf("project extend %d: %s", code, errout.String())
	}
	var compiled map[string]any
	data, err := os.ReadFile(outputPath)
	if err != nil || json.Unmarshal(data, &compiled) != nil {
		t.Fatalf("compiled extension output: %v %s", err, data)
	}
	stages := compiled["definition"].(map[string]any)["stages"].(map[string]any)
	if got := stages["build"].(map[string]any)["on_complete"].(map[string]any)["succeeded"]; got != "qa" {
		t.Fatalf("build route did not lead to the inserted step: %#v", got)
	}
	qa := stages["qa"].(map[string]any)
	if qa["kind"] != "step" || qa["on"].(map[string]any)["pass"] != "commit" {
		t.Fatalf("inserted step has the wrong contract: %#v", qa)
	}
}

func TestProjectWorkflowFeatureRequiresChoiceBypass(t *testing.T) {
	err := projectApplyWorkflowOptions([]projectPendingDocument{{
		document: projectPackageDocument{Kind: "workflow"},
		value: map[string]any{
			"id": "test:workflow/sample",
			"inputs": map[string]any{
				"enabled": map[string]any{"configuration": map[string]any{"scope": "project", "default": true}},
			},
			"features": map[string]any{"optional": map[string]any{"input": "enabled"}},
			"stages":   map[string]any{"done": map[string]any{"kind": "finish"}},
		},
	}}, projectWorkflowOptions{})
	if err == nil || !strings.Contains(err.Error(), "project_feature_invalid: optional requires a Choice bypass for enabled") {
		t.Fatalf("feature without an explicit bypass was accepted: %v", err)
	}
}

func TestCLIProjectCompileRejectsFlatPackageSource(t *testing.T) {
	{
		repository := filepath.Join(t.TempDir(), "repository")
		if err := os.MkdirAll(filepath.Join(repository, ".prifly"), 0755); err != nil {
			t.Fatal(err)
		}
		if output, err := exec.Command("git", "init", "-q", repository).CombinedOutput(); err != nil {
			t.Fatalf("git init: %v: %s", err, output)
		}
		if err := os.WriteFile(filepath.Join(repository, ".prifly", "package.yaml"), []byte("schema_version: prifly-package-source/1\n"), 0644); err != nil {
			t.Fatal(err)
		}
		profile := "schema_version: prifly-project-profile/2\n" + projectHostsYAML + `packages:
  legacy:
    source: .prifly/package.yaml
launches: {}
`
		if err := os.WriteFile(filepath.Join(repository, ".prifly", "project.yaml"), []byte(profile), 0644); err != nil {
			t.Fatal(err)
		}
		output := filepath.Join(t.TempDir(), "package")
		var out, errout bytes.Buffer
		if code := execute(context.Background(), []string{"project", "compile", "--repository", repository, "--package", "legacy", "--host", "codex-cli", "--output", output, "--json"}, &out, &errout); code == 0 || !strings.Contains(errout.String(), "package source must be a workflow folder") {
			t.Fatalf("flat package source was accepted: %d %s", code, errout.String())
		}
		if _, err := os.Stat(output); !os.IsNotExist(err) {
			t.Fatalf("rejected flat source left output: %v", err)
		}
	}
}

func TestCLIProjectCompileSealsWorkflowFolder(t *testing.T) {
	repository := filepath.Join(t.TempDir(), "repository")
	if err := os.MkdirAll(filepath.Join(repository, ".prifly", "workflows", "cycle", "steps", "quality"), 0755); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("git", "init", "-q", repository).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	authority := filepath.Join(t.TempDir(), "authority")
	if err := prifly.InitProfile(authority, flow.CoreProfile); err != nil {
		t.Fatal(err)
	}
	write := func(name, value string) {
		t.Helper()
		path := filepath.Join(repository, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(value), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write(".prifly/project.yaml", "schema_version: prifly-project-profile/2\n"+projectHostsYAML+`packages:
  cycle:
    source: .prifly/workflows/cycle
launches:
  cycle:
    title: Compile cycle
    description: List the YAML-folder workflow.
    kind: workflow
    workflow: .prifly/workflows/cycle/workflow.yaml
`)
	write(".codex/skills/quality/SKILL.md", "# Quality\n\nRun the Codex check.\n")
	write(".claude/skills/quality/SKILL.md", "# Quality\n\nRun the Claude check.\n")
	write(".prifly/workflows/cycle/contexts/quality.yaml", `id: test:context/quality
version: 1.0.0
media_type: text/markdown; charset=utf-8
source: {root: host_skills, path: quality/SKILL.md}
`)
	step := func(id, title string) string {
		return fmt.Sprintf(`authoring: prifly-step/1
id: test:step/%s
version: 1.0.0
title: %s
kind: worker
executor: {adapter_ref: "{{assisted_adapter}}", operation: session}
instructions_ref: "{{context_quality}}"
effects: {class: none, retry_class: never}
result_schema_ref: "{{step_result_schema}}"
`, id, title)
	}
	write(".prifly/workflows/cycle/steps/build.yaml", step("build", "Build"))
	write(".prifly/workflows/cycle/steps/commit.yaml", step("commit", "Commit"))
	write(".prifly/workflows/cycle/steps/quality/qa.yaml", step("qa", "Run QA"))
	write(".prifly/workflows/cycle/schemas/flag.yaml", `id: test:schema/flag
version: 1.0.0
$schema: https://json-schema.org/draft/2020-12/schema
type: boolean
`)
	write(".prifly/workflows/cycle/schemas/limit.yaml", `id: test:schema/limit
version: 1.0.0
$schema: https://json-schema.org/draft/2020-12/schema
type: integer
minimum: 1
maximum: 3
`)
	write(".prifly/workflows/cycle/extend.yaml", `settings:
  cycle: {quality_enabled: false, batch_limit: 2}
exclude: [quality]
extensions:
  - id: qa-after-build
    workflow: cycle
    between: {from: build, to: commit}
    step: qa
    on: {pass: commit}
`)
	write(".prifly/workflows/cycle/workflow.yaml", `authoring: prifly-project-workflow/1
package:
  id: test:package/cycle
  version: 1.0.0
  description: A workflow folder fixture.
  requires_core_protocol: "1"
  references:
    assisted_adapter: core:adapter/assisted-session@1.0.0
    local_policy: core:policy/local@3.0.0
    step_result_schema: core:schema/step-result@1.0.0
id: test:workflow/cycle
version: 1.0.0
refs:
  step_build: "{{step_build}}"
  step_commit: "{{step_commit}}"
  schema_flag: "{{schema_flag}}"
  schema_limit: "{{schema_limit}}"
  local_policy: "{{local_policy}}"
inputs:
  quality_enabled:
    schema_ref: schema_flag
    required: false
    configuration: {scope: project, default: true}
  batch_limit:
    schema_ref: schema_limit
    required: false
    configuration: {scope: project, default: 3}
outputs: {}
features: {quality: {input: quality_enabled}}
entry: choose-quality
limits: {max_step_instances: 3, max_control_transitions: 8}
policy_ref: local_policy
stages:
  choose-quality:
    kind: choice
    selection: exclusive
    branches:
      - id: enabled
        predicate: {op: eq, left: $inputs.quality_enabled, right: true}
        next: build
    default: commit
  build: {kind: step, step_ref: step_build, on: {pass: commit}}
  commit: {kind: step, step_ref: step_commit, on: {pass: done}}
  done: {kind: finish, outcome: succeeded}
`)
	var out, errout bytes.Buffer
	if code := execute(context.Background(), []string{"project", "workflows", "--repository", repository, "--json"}, &out, &errout); code != 0 {
		t.Fatalf("workflow folder discovery %d: %s", code, errout.String())
	}
	var launches projectWorkflowList
	if err := json.Unmarshal(out.Bytes(), &launches); err != nil || len(launches.Launches) != 1 || launches.Launches[0].ID != "cycle" || len(launches.Launches[0].Inputs) != 2 {
		t.Fatalf("workflow folder launch was not listed: %v %+v", err, launches)
	}
	output := filepath.Join(t.TempDir(), "package")
	out.Reset()
	errout.Reset()
	args := []string{"--project", authority, "project", "compile", "--repository", repository, "--package", "cycle", "--host", "codex-cli", "--output", output, "--json"}
	if code := execute(context.Background(), args, &out, &errout); code != 0 {
		t.Fatalf("workflow folder compile %d: %s", code, errout.String())
	}
	var result projectCompileResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Package.ID != "test:package/cycle" || len(result.Components) != 7 {
		t.Fatalf("unexpected workflow folder result: %+v", result)
	}
	out.Reset()
	errout.Reset()
	if code := execute(context.Background(), []string{"--project", authority, "project", "compile", "--repository", repository, "--package", "cycle", "--output", filepath.Join(t.TempDir(), "missing-host"), "--json"}, &out, &errout); code == 0 || !strings.Contains(errout.String(), "requires --package, --host and --output") {
		t.Fatalf("workflow folder inferred a host: %d %s", code, errout.String())
	}
	claudeOutput := filepath.Join(t.TempDir(), "claude-package")
	out.Reset()
	errout.Reset()
	if code := execute(context.Background(), []string{"--project", authority, "project", "compile", "--repository", repository, "--package", "cycle", "--host", "claude-code", "--output", claudeOutput, "--json"}, &out, &errout); code != 0 {
		t.Fatalf("Claude workflow folder compile %d: %s", code, errout.String())
	}
	var claude projectCompileResult
	if err := json.Unmarshal(out.Bytes(), &claude); err != nil {
		t.Fatal(err)
	}
	contextDigest := func(components []projectCompileComponent) string {
		for _, component := range components {
			if component.Kind == "context" {
				return component.Ref.Digest
			}
		}
		t.Fatal("compiled workflow has no context")
		return ""
	}
	if contextDigest(result.Components) == contextDigest(claude.Components) {
		t.Fatalf("host-specific context was not sealed independently: codex=%+v claude=%+v", result.Components, claude.Components)
	}
	out.Reset()
	errout.Reset()
	if code := execute(context.Background(), []string{"--project", authority, "project", "compile", "--repository", repository, "--package", "cycle", "--host", "unknown", "--output", filepath.Join(t.TempDir(), "unknown-host"), "--json"}, &out, &errout); code == 0 || !strings.Contains(errout.String(), "project_compile_unknown_host") {
		t.Fatalf("workflow folder accepted an unknown host: %d %s", code, errout.String())
	}
	write(".prifly/workflows/cycle/contexts/quality.yaml", `id: test:context/quality
version: 1.0.0
media_type: text/markdown; charset=utf-8
source: {root: host_skills, path: ../quality/SKILL.md}
`)
	out.Reset()
	errout.Reset()
	if code := execute(context.Background(), []string{"--project", authority, "project", "compile", "--repository", repository, "--package", "cycle", "--host", "codex-cli", "--output", filepath.Join(t.TempDir(), "escaped-context"), "--json"}, &out, &errout); code == 0 || !strings.Contains(errout.String(), "host context source must stay inside") {
		t.Fatalf("workflow folder accepted an escaped host context: %d %s", code, errout.String())
	}
	write(".prifly/workflows/cycle/contexts/quality.yaml", `id: test:context/quality
version: 1.0.0
media_type: text/markdown; charset=utf-8
source: {root: host_skills, path: quality/SKILL.md}
`)
	for _, component := range result.Components {
		if component.Kind != "workflow" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(output, filepath.FromSlash(component.Path)))
		if err != nil {
			t.Fatal(err)
		}
		var workflow map[string]any
		if err := json.Unmarshal(data, &workflow); err != nil {
			t.Fatal(err)
		}
		stages := workflow["definition"].(map[string]any)["stages"].(map[string]any)
		build := stages["build"].(map[string]any)
		if build["on"].(map[string]any)["pass"] != "qa" || stages["qa"] == nil {
			t.Fatalf("folder extension did not insert QA: %#v", stages)
		}
		if _, exists := workflow["features"]; exists {
			t.Fatalf("project feature metadata leaked into sealed workflow: %#v", workflow)
		}
		inputs := workflow["inputs"].(map[string]any)
		if inputs["quality_enabled"].(map[string]any)["configuration"].(map[string]any)["default"] != false || inputs["batch_limit"].(map[string]any)["configuration"].(map[string]any)["default"] != float64(2) {
			t.Fatalf("settings and exclude were not sealed as declared defaults: %#v", inputs)
		}
	}
	if _, err := os.Stat(filepath.Join(output, prifly.PackageManifestFile)); err != nil {
		t.Fatalf("missing package manifest: %v", err)
	}
	out.Reset()
	errout.Reset()
	if code := execute(context.Background(), []string{"--project", authority, "package", "import", "--dir", output, "--reason", "exercise workflow folder compile", "--json"}, &out, &errout); code != 0 {
		t.Fatalf("compiled workflow folder import %d: %s", code, errout.String())
	}
	write(".prifly/workflows/cycle/steps/quality/qa.yaml", step("qa", "Run QA")+"---\n"+step("qa-second", "Run second QA"))
	out.Reset()
	errout.Reset()
	if code := execute(context.Background(), []string{"--project", authority, "project", "compile", "--repository", repository, "--package", "cycle", "--host", "codex-cli", "--output", filepath.Join(t.TempDir(), "rejected"), "--json"}, &out, &errout); code == 0 || !strings.Contains(errout.String(), "project_workflow_folder_invalid: .prifly/workflows/cycle/steps/quality/qa.yaml requires exactly one YAML document") {
		t.Fatalf("workflow folder accepted a multi-document YAML source: %d: %s", code, errout.String())
	}
	write(".prifly/workflows/cycle/steps/quality/qa.yaml", step("qa", "Run QA"))
	write(".prifly/workflows/cycle/extend.yaml", "settings: {cycle: {missing: 1}}\n")
	out.Reset()
	errout.Reset()
	if code := execute(context.Background(), []string{"--project", authority, "project", "compile", "--repository", repository, "--package", "cycle", "--host", "codex-cli", "--output", filepath.Join(t.TempDir(), "unknown-setting"), "--json"}, &out, &errout); code == 0 || !strings.Contains(errout.String(), "project_option_unknown_input: cycle.missing") {
		t.Fatalf("workflow folder accepted unknown setting: %d: %s", code, errout.String())
	}
	write(".prifly/workflows/cycle/extend.yaml", "settings: {cycle: {batch_limit: 4}}\n")
	out.Reset()
	errout.Reset()
	if code := execute(context.Background(), []string{"--project", authority, "project", "compile", "--repository", repository, "--package", "cycle", "--host", "codex-cli", "--output", filepath.Join(t.TempDir(), "invalid-setting"), "--json"}, &out, &errout); code == 0 || !strings.Contains(errout.String(), "project_compile_invalid_workflow: invalid_default") {
		t.Fatalf("workflow folder accepted setting outside its declared schema: %d: %s", code, errout.String())
	}
	write(".prifly/workflows/cycle/extend.yaml", "exclude: [missing]\n")
	out.Reset()
	errout.Reset()
	if code := execute(context.Background(), []string{"--project", authority, "project", "compile", "--repository", repository, "--package", "cycle", "--host", "codex-cli", "--output", filepath.Join(t.TempDir(), "unknown-feature"), "--json"}, &out, &errout); code == 0 || !strings.Contains(errout.String(), "project_option_unknown_feature: missing") {
		t.Fatalf("workflow folder accepted unknown feature: %d: %s", code, errout.String())
	}
	write(".prifly/workflows/cycle/extend.yaml", "settings: {cycle: {quality_enabled: true}}\nexclude: [quality]\n")
	out.Reset()
	errout.Reset()
	if code := execute(context.Background(), []string{"--project", authority, "project", "compile", "--repository", repository, "--package", "cycle", "--host", "codex-cli", "--output", filepath.Join(t.TempDir(), "conflicting-option"), "--json"}, &out, &errout); code == 0 || !strings.Contains(errout.String(), "project_option_conflict: exclude quality conflicts with setting cycle.quality_enabled") {
		t.Fatalf("workflow folder accepted contradictory feature setting: %d: %s", code, errout.String())
	}
	write(".prifly/workflows/cycle/extend.yaml", "extensions: []\n")
	outsideSkill := filepath.Join(t.TempDir(), "outside-skill.md")
	if err := os.WriteFile(outsideSkill, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(repository, ".codex", "skills", "quality", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideSkill, filepath.Join(repository, ".codex", "skills", "quality", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errout.Reset()
	if code := execute(context.Background(), []string{"--project", authority, "project", "compile", "--repository", repository, "--package", "cycle", "--host", "codex-cli", "--output", filepath.Join(t.TempDir(), "symlink-context"), "--json"}, &out, &errout); code == 0 || !strings.Contains(errout.String(), "host context source must stay inside") {
		t.Fatalf("workflow folder accepted a symlinked host context: %d %s", code, errout.String())
	}
}

func TestCLIProjectStartClaimsDeclaredWorkspaceAndStopsAtHostHandoff(t *testing.T) {
	start := func(t *testing.T, workspace, policy, runtimeAnswer string) projectStartResult {
		t.Helper()
		repository := filepath.Join(t.TempDir(), "repository")
		for _, directory := range []string{
			filepath.Join(repository, ".prifly", "workflows", "pilot", "steps"),
			filepath.Join(repository, ".prifly", "workflows", "pilot", "contexts"),
			filepath.Join(repository, ".prifly", "workflows", "pilot", "decisions"),
			filepath.Join(repository, ".codex", "skills", "work"),
		} {
			if err := os.MkdirAll(directory, 0755); err != nil {
				t.Fatal(err)
			}
		}
		git := func(args ...string) {
			t.Helper()
			command := exec.Command("git", args...)
			command.Dir = repository
			command.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid")
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
			}
		}
		git("init", "--initial-branch=main")
		write := func(path, value string) {
			t.Helper()
			if err := os.WriteFile(filepath.Join(repository, path), []byte(value), 0644); err != nil {
				t.Fatal(err)
			}
		}
		write("README.md", "# pilot\n")
		git("-c", "commit.gpgsign=false", "add", "README.md")
		git("-c", "commit.gpgsign=false", "commit", "-m", "initial")
		write(".codex/skills/work/SKILL.md", "# Work\n")
		write(".prifly/project.yaml", "schema_version: prifly-project-profile/2\n"+projectHostsYAML+`packages:
  pilot:
    source: .prifly/workflows/pilot
launches:
  pilot:
    title: Pilot
    description: Workspace launch fixture.
    kind: workflow
    workflow: .prifly/workflows/pilot/workflow.yaml
`)
		write(".prifly/workflows/pilot/contexts/work.yaml", `id: test:context/work
version: 1.0.0
media_type: text/markdown; charset=utf-8
source: {root: host_skills, path: work/SKILL.md}
`)
		write(".prifly/workflows/pilot/steps/work.yaml", `authoring: prifly-step/1
id: test:step/work
version: 1.0.0
title: Work
kind: worker
executor: {adapter_ref: "{{assisted_adapter}}", operation: session}
instructions_ref: "{{context_work}}"
effects: {class: workspace_write, retry_class: never}
result_schema_ref: "{{step_result_schema}}"
`)
		write(".prifly/workflows/pilot/decisions/checkpoint.yaml", `authoring: prifly-run-decision/1
id: checkpoint
title: Create checkpoint
phase: preflight
choices: [{id: proceed, title: Proceed, value: true}]
destination: {kind: session_context, name: checkpoint}
`)
		write(".prifly/workflows/pilot/decisions/roadmap-linkage.yaml", `authoring: prifly-run-decision/1
id: roadmap_linkage
title: Link a roadmap milestone
phase: preflight
choices: [{id: link, title: Link, value: link}, {id: skip, title: Skip, value: skip}]
destination: {kind: session_context, name: roadmap_linkage}
`)
		write(".prifly/workflows/pilot/decisions/roadmap-milestone.yaml", `authoring: prifly-run-decision/1
id: roadmap_milestone
title: Roadmap milestone
phase: preflight
choices: [{id: current, title: Current, value: current}]
destination: {kind: session_context, name: roadmap_milestone}
when: {answers: {roadmap_linkage: link}}
`)
		write(".prifly/workflows/pilot/decisions/continue.yaml", `authoring: prifly-run-decision/1
id: continue
title: Continue work
phase: runtime
choices: [{id: proceed, title: Proceed, value: true}]
destination: {kind: session_context, name: continue}
`)
		write(".prifly/workflows/pilot/workflow.yaml", `authoring: prifly-project-workflow/1
package:
  id: test:package/pilot
  version: 1.0.0
  description: Project launch fixture.
  requires_core_protocol: "1"
  profiles:
    default: fast
    values: {fast: {plan_capture: fast}, full: {plan_capture: full}}
  references:
    assisted_adapter: core:adapter/assisted-session@1.0.0
    local_policy: core:policy/local@3.0.0
    step_result_schema: core:schema/step-result@1.0.0
id: test:workflow/pilot
version: 1.0.0
refs:
  step_work: "{{step_work}}"
  local_policy: "{{local_policy}}"
inputs: {}
outputs: {}
decision_catalog: [.prifly/workflows/pilot/decisions/checkpoint.yaml, .prifly/workflows/pilot/decisions/roadmap-linkage.yaml, .prifly/workflows/pilot/decisions/roadmap-milestone.yaml, .prifly/workflows/pilot/decisions/continue.yaml]
entry: work
limits: {max_step_instances: 1, max_control_transitions: 2}
policy_ref: local_policy
stages:
  work: {kind: step, step_ref: step_work, input_bindings: {}, on: {pass: done}}
  done: {kind: finish, outcome: succeeded, output_bindings: {}}
`)
		write(".prifly/.gitignore", "local.yaml\n")
		if err := writeProjectRunners(repository); err != nil {
			t.Fatal(err)
		}
		git("-c", "commit.gpgsign=false", "add", ".")
		git("-c", "commit.gpgsign=false", "commit", "-m", "add project workflow")
		authority := filepath.Join(t.TempDir(), "authority")
		var initOut, initErr bytes.Buffer
		if code := execute(context.Background(), []string{"project", "init", "--repository", repository, "--state-root", authority, "--json"}, &initOut, &initErr); code != 0 {
			t.Fatalf("project init for cloned profile: %d %s", code, initErr.String())
		}
		brief := filepath.Join(t.TempDir(), "brief.json")
		briefData := []byte(`{"schema_version":"1","id":"test:brief/pilot","subject":"Pilot","desired_outcome":"Test the declared launch","in_scope":["handoff"],"out_of_scope":[],"completion_criteria":["handoff"],"source_refs":[],"assumptions":[],"confirmation":"explicit"}`)
		if err := os.WriteFile(brief, briefData, 0600); err != nil {
			t.Fatal(err)
		}
		var questionnaireOut, questionnaireErr bytes.Buffer
		if code := execute(context.Background(), []string{"--project", authority, "project", "questionnaire", "--repository", repository, "--launch", "pilot", "--json"}, &questionnaireOut, &questionnaireErr); code != 0 {
			t.Fatalf("project questionnaire %d: %s", code, questionnaireErr.String())
		}
		var questionnaire projectQuestionnaire
		if err := json.Unmarshal(questionnaireOut.Bytes(), &questionnaire); err != nil || questionnaire.SchemaVersion != "project-questionnaire/2" || questionnaire.CatalogDigest == "" {
			t.Fatalf("project questionnaire did not return a stable catalog: %v %#v", err, questionnaire)
		}
		if workspace == "" {
			assertPreflight := func(want string, arguments ...string) {
				t.Helper()
				var invalidOut, invalidErr bytes.Buffer
				if code := execute(context.Background(), arguments, &invalidOut, &invalidErr); code == 0 || !strings.Contains(invalidErr.String(), want) {
					t.Fatalf("invalid project start mutated instead of refusing: code=%d want=%q stderr=%s", code, want, invalidErr.String())
				}
				check, err := prifly.Open(authority, true)
				if err != nil {
					t.Fatal(err)
				}
				defer check.Close()
				claims, err := check.Claims(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				packages, err := check.Packages(context.Background())
				if err != nil || len(claims.Claims) != 0 || len(packages.Packages) != 0 {
					t.Fatalf("invalid project start created authority state: claims=%+v packages=%+v err=%v", claims.Claims, packages.Packages, err)
				}
			}
			base := []string{"--project", authority, "project", "start", "--repository", repository, "--launch", "pilot", "--host", "codex-cli", "--brief", brief, "--json"}
			assertPreflight("project_start_unknown_launch", append(append([]string{}, base...), "--launch", "missing")...)
			assertPreflight("project_compile_unknown_host", append(append([]string{}, base...), "--host", "missing-host")...)
			assertPreflight("project_start_unknown_input", append(append([]string{}, base...), "--input", "missing=unused.json")...)
			assertPreflight("project_start_invalid_workspace", append(append([]string{}, base...), "--workspace", "surprise")...)
			assertPreflight("project_start_missing_decision", base...)
			assertPreflight("project_start_missing_decision: roadmap_milestone", append(append([]string{}, base...), "--preflight-answer", "checkpoint=true", "--preflight-answer", `roadmap_linkage="link"`)...)
			assertPreflight("project_start_stale_decision_catalog", append(append([]string{}, base...), "--package-profile", "full", "--preflight-answer", "checkpoint=true", "--preflight-answer", `roadmap_linkage="skip"`, "--expected-decision-catalog-digest", "sha256:"+strings.Repeat("0", 64))...)
			assertPreflight("project_start_unknown_decision", append(append([]string{}, base...), "--preflight-answer", "unknown=true")...)
			assertPreflight("project_start_invalid_decision_answer", append(append([]string{}, base...), "--preflight-answer", "checkpoint=false")...)
			legacyAuthority := filepath.Join(t.TempDir(), "legacy-authority")
			if err := prifly.InitProfile(legacyAuthority, flow.CoreProfile); err != nil {
				t.Fatal(err)
			}
			legacyArgs := append([]string{"--project", legacyAuthority}, append(base[2:], "--preflight-answer", "checkpoint=true", "--preflight-answer", `roadmap_linkage="skip"`)...)
			var legacyOut, legacyErr bytes.Buffer
			if code := execute(context.Background(), legacyArgs, &legacyOut, &legacyErr); code == 0 || !strings.Contains(legacyErr.String(), "authority_configuration_incompatible") {
				t.Fatalf("legacy authority reached project launch: code=%d stderr=%s", code, legacyErr.String())
			}
			legacy, err := prifly.Open(legacyAuthority, true)
			if err != nil {
				t.Fatal(err)
			}
			defer legacy.Close()
			claims, claimErr := legacy.Claims(context.Background())
			packages, packageErr := legacy.Packages(context.Background())
			if claimErr != nil || packageErr != nil || len(claims.Claims) != 0 || len(packages.Packages) != 0 {
				t.Fatalf("legacy authority rejection created state: claims=%+v packages=%+v claim_err=%v package_err=%v", claims.Claims, packages.Packages, claimErr, packageErr)
			}
		}
		args := []string{"--project", authority, "project", "start", "--repository", repository, "--launch", "pilot", "--host", "codex-cli", "--brief", brief, "--package-profile", "full", "--preflight-answer", "checkpoint=true", "--preflight-answer", `roadmap_linkage="skip"`, "--expected-decision-catalog-digest", questionnaire.CatalogDigest, "--json"}
		if policy != "" {
			args = append(args, "--decision-policy", policy)
		}
		if runtimeAnswer != "" {
			args = append(args, "--runtime-answer", runtimeAnswer)
		}
		if workspace != "" {
			args = append(args, "--workspace", workspace)
		}
		var out, errout bytes.Buffer
		if code := execute(context.Background(), args, &out, &errout); code != 0 {
			t.Fatalf("project start %d: %s", code, errout.String())
		}
		var result projectStartResult
		if err := json.Unmarshal(out.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.PackageProfile != "full" {
			t.Fatalf("project start did not seal its explicit package profile: %#v", result)
		}
		sealedRecords := 2
		if runtimeAnswer != "" {
			sealedRecords++
		}
		if result.SchemaVersion != "project-start/2" || result.DecisionSheet == nil || len(result.DecisionSheet.Records) != sealedRecords || result.Run.Run.DecisionCatalog == nil || result.Run.Run.DecisionSheet == nil || result.Run.Run.DecisionSheet.CatalogDigest != result.DecisionSheet.CatalogDigest {
			t.Fatalf("project start did not retain its reviewed decision sheet: %#v", result)
		}
		// An autonomous launch says which declared runtime decisions its policy
		// will not take, before the first dispatch. Attended does not: waiting
		// for the owner is that policy's normal path, not a warning.
		switch {
		case policy == "autonomous" && runtimeAnswer != "":
			// Answered before the Run started, so nothing about it will stop.
			if result.AutonomyUnanswered == nil || len(*result.AutonomyUnanswered) != 0 {
				t.Fatalf("a sealed runtime answer was still reported as unanswerable: %#v", result.AutonomyUnanswered)
			}
			sealed := false
			for _, record := range result.DecisionSheet.Records {
				sealed = sealed || (record.DefinitionID == "continue" && record.Status == "answered" && record.Source == "actor" && string(record.Value) == "true")
			}
			if !sealed {
				t.Fatalf("the runtime answer was not sealed into the decision sheet: %#v", result.DecisionSheet.Records)
			}
		case policy == "autonomous":
			if result.AutonomyUnanswered == nil || len(*result.AutonomyUnanswered) != 1 || (*result.AutonomyUnanswered)[0].DecisionID != "continue" || (*result.AutonomyUnanswered)[0].Reason != "automatic_selection_not_allowed" {
				t.Fatalf("autonomous launch did not name the decision its policy cannot take: %#v", result.AutonomyUnanswered)
			}
		default:
			if result.AutonomyUnanswered != nil {
				t.Fatalf("attended launch reported an autonomy list: %#v", result.AutonomyUnanswered)
			}
		}
		e, err := prifly.Open(authority, false)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = e.Close() })
		task, err := e.SessionTask(context.Background(), result.Run.Run.ID, "")
		wantRepositoryWorkspace := result.Workspace.Repository.Toplevel
		if result.Workspace.Mode == "worktree" {
			wantRepositoryWorkspace = filepath.Join(e.Root, result.Workspace.Path)
		}
		if err != nil || task.SchemaVersion != prifly.AssistedSessionDecisionVersion || task.RepositoryWorkspace != wantRepositoryWorkspace || task.Workspace == task.RepositoryWorkspace {
			t.Fatalf("project start did not stop at the workspace handoff: want_repository_workspace=%q got=%q task_schema=%q err=%v", wantRepositoryWorkspace, task.RepositoryWorkspace, task.SchemaVersion, err)
		}
		if _, err := os.Stat(filepath.Join(task.RepositoryWorkspace, "context")); !os.IsNotExist(err) {
			t.Fatalf("scratch context entered the repository: %v", err)
		}
		var runtimeDecision prifly.DecisionDefinition
		for _, definition := range result.Run.Run.DecisionCatalog.Decisions {
			if definition.ID == "continue" {
				runtimeDecision = definition
			}
		}
		out.Reset()
		errout.Reset()
		requestArgs := []string{"--project", authority, "run", "decision", result.Run.Run.ID, "request", "--attempt", task.AttemptID, "--envelope-digest", task.EnvelopeDigest, "--decision", runtimeDecision.ID, "--expected-run-version", strconv.FormatInt(task.RunVersion, 10), "--json"}
		if code := execute(context.Background(), requestArgs, &out, &errout); code != 0 {
			t.Fatalf("request declared decision: %d %s", code, errout.String())
		}
		var decisions struct {
			RunVersion    int64                   `json:"run_version"`
			Pending       *prifly.DecisionRequest `json:"pending"`
			PendingDigest string                  `json:"pending_request_digest"`
			Records       []prifly.DecisionRecord `json:"records"`
		}
		out.Reset()
		errout.Reset()
		if code := execute(context.Background(), []string{"--project", authority, "run", "decisions", result.Run.Run.ID, "--json"}, &out, &errout); code != 0 || json.Unmarshal(out.Bytes(), &decisions) != nil {
			t.Fatalf("read decisions: code=%d stdout=%s stderr=%s", code, out.String(), errout.String())
		}
		// A sealed answer is applied to the request instead of parking it, so
		// there is no question left for anyone to answer.
		if runtimeAnswer != "" {
			if decisions.Pending != nil {
				t.Fatalf("a sealed runtime answer still parked the Run: %+v", decisions.Pending)
			}
			answered := false
			for _, record := range decisions.Records {
				answered = answered || (record.DefinitionID == "continue" && record.AttemptID == task.AttemptID && record.Status == "answered" && record.Source == "actor")
			}
			if !answered {
				t.Fatalf("the request was not recorded as answered by the owner: %+v", decisions.Records)
			}
			return result
		}
		if decisions.Pending == nil {
			t.Fatalf("no pending decision to answer: stdout=%s", out.String())
		}
		out.Reset()
		errout.Reset()
		if code := execute(context.Background(), []string{"--project", authority, "run", "decision", result.Run.Run.ID, "answer", "--value", "true", "--json"}, &out, &errout); code == 0 || !strings.Contains(errout.String(), "invalid_usage") {
			t.Fatalf("decision answer without identity was accepted: %d %s", code, errout.String())
		}
		// The answer is built only from what the read printed. Recomputing the
		// digest with the Go API proved the engine agreed with itself while the
		// CLI user had no way to obtain that value at all.
		expected, err := prifly.DecisionRequestDigest(*decisions.Pending)
		if err != nil {
			t.Fatal(err)
		}
		if decisions.PendingDigest != expected {
			t.Fatalf("run decisions did not report the digest its answer requires: %q", decisions.PendingDigest)
		}
		version := strconv.FormatInt(decisions.RunVersion, 10)
		answer := func(decision, digest, runVersion string) (int, string) {
			out.Reset()
			errout.Reset()
			code := execute(context.Background(), []string{"--project", authority, "run", "decision", result.Run.Run.ID, "answer", "--decision", decision, "--request-digest", digest, "--expected-run-version", runVersion, "--value", "true", "--json"}, &out, &errout)
			return code, errout.String()
		}
		// Each identity that can be wrong refuses by naming what is current, so
		// four refusals in a row cannot say the same thing.
		for _, mismatch := range []struct{ name, decision, digest, version string }{
			{"decision", "unknown", decisions.PendingDigest, version},
			{"version", decisions.Pending.DecisionID, decisions.PendingDigest, strconv.FormatInt(decisions.RunVersion+1, 10)},
			{"digest", decisions.Pending.DecisionID, "sha256:" + strings.Repeat("0", 64), version},
		} {
			code, message := answer(mismatch.decision, mismatch.digest, mismatch.version)
			if code == 0 || !strings.Contains(message, "invalid_usage") || !strings.Contains(message, "received") {
				t.Fatalf("a wrong %s was not refused by naming the current one: %d %s", mismatch.name, code, message)
			}
		}
		if code, message := answer(decisions.Pending.DecisionID, decisions.PendingDigest, version); code != 0 {
			t.Fatalf("answer pending decision: %d %s", code, message)
		}
		return result
	}

	if result := start(t, "", "", ""); result.Workspace.Mode != "worktree" {
		t.Fatalf("direct CLI did not default to worktree: %+v", result.Workspace)
	}
	if result := start(t, "checkout", "", ""); result.Workspace.Mode != "checkout" {
		t.Fatalf("project start did not retain checkout mode: %+v", result.Workspace)
	}
	start(t, "", "autonomous", "")
	start(t, "", "autonomous", "continue=true")
}

func TestCLIProjectCompileSelectsDeclaredPackageProfile(t *testing.T) {
	repository, authority := newProjectFixture(t)
	writeFixtureFile(t, repository, ".codex/skills/work/SKILL.md", "# Work\n")
	writeFixtureFile(t, repository, ".prifly/workflows/sample/contexts/work.yaml", `id: test:context/work
version: 1.0.0
media_type: text/markdown; charset=utf-8
source: {root: host_skills, path: work/SKILL.md}
`)
	writeFixtureFile(t, repository, ".prifly/workflows/sample/steps/work.yaml", `authoring: prifly-step/1
id: test:step/work
version: 1.0.0
title: Work
kind: worker
outputs:
  plan: {schema_ref: "{{workspace_tree_manifest}}", required_for: [pass]}
executor: {adapter_ref: "{{assisted_adapter}}", operation: session}
instructions_ref: "{{context_work}}"
effects: {class: workspace_write, retry_class: never}
result_schema_ref: "{{step_result_schema}}"
workspace_trees:
  - output_port: plan
    capture: "{{plan_capture}}"
`)
	writeFixtureFile(t, repository, ".prifly/workflows/sample/workflow.yaml", `authoring: prifly-project-workflow/1
package:
  id: test:package/sample
  version: 1.0.0
  description: Package profile fixture.
  requires_core_protocol: "1"
  profiles:
    default: file
    values:
      file: {plan_capture: {kind: exact_file, path: plans/PLAN.md}}
      tree: {plan_capture: {kind: direct_child_tree, path: plans, entrypoint: index.md}}
  references:
    assisted_adapter: core:adapter/assisted-session@1.0.0
    local_policy: core:policy/local@3.0.0
    step_result_schema: core:schema/step-result@1.0.0
    workspace_tree_manifest: core:schema/workspace-tree-manifest@1.0.0
id: test:workflow/sample
version: 1.0.0
refs:
  step_work: "{{step_work}}"
  local_policy: "{{local_policy}}"
inputs: {}
outputs: {}
entry: work
limits: {max_step_instances: 1, max_control_transitions: 2}
policy_ref: local_policy
stages:
  work: {kind: step, step_ref: step_work, input_bindings: {}, on: {pass: done}}
  done: {kind: finish, outcome: succeeded, output_bindings: {}}
`)
	extendPath := filepath.Join(repository, ".prifly", "workflows", "sample", "extend.yaml")
	writeFixtureFile(t, repository, ".prifly/workflows/sample/extend.yaml", "profile: file\nextensions: []\n")
	writeFixtureFile(t, repository, ".prifly/project.yaml", "schema_version: prifly-project-profile/2\n"+projectHostsYAML+`packages:
  sample:
    source: .prifly/workflows/sample
launches:
  sample:
    title: Sample
    description: Package profile fixture.
    kind: workflow
    workflow: .prifly/workflows/sample/workflow.yaml
`)
	capture := func(profile string) map[string]any {
		t.Helper()
		output := filepath.Join(t.TempDir(), "package")
		args := []string{"--project", authority, "project", "compile", "--repository", repository, "--package", "sample", "--host", "codex-cli", "--output", output}
		if profile != "" {
			args = append(args, "--package-profile", profile)
		}
		code, out, errout := runCLI(t, args...)
		if code != 0 {
			t.Fatalf("compile with profile %q: %d %s", profile, code, errout)
		}
		var result projectCompileResult
		if err := json.Unmarshal([]byte(out), &result); err != nil {
			t.Fatal(err)
		}
		for _, component := range result.Components {
			if component.Kind != "step" {
				continue
			}
			data, err := os.ReadFile(filepath.Join(output, filepath.FromSlash(component.Path)))
			if err != nil {
				t.Fatal(err)
			}
			var step map[string]any
			if err := json.Unmarshal(data, &step); err != nil {
				t.Fatal(err)
			}
			return step["workspace_trees"].([]any)[0].(map[string]any)["capture"].(map[string]any)
		}
		t.Fatal("compiled package has no step")
		return nil
	}
	if reviewed := capture(""); reviewed["kind"] != "exact_file" || reviewed["path"] != "plans/PLAN.md" {
		t.Fatalf("reviewed default profile was not sealed: %#v", reviewed)
	}
	if selected := capture("tree"); selected["kind"] != "direct_child_tree" || selected["path"] != "plans" || selected["entrypoint"] != "index.md" {
		t.Fatalf("per-Run profile was not sealed: %#v", selected)
	}
	refused := filepath.Join(t.TempDir(), "refused")
	if code, _, errout := runCLI(t, "--project", authority, "project", "compile", "--repository", repository, "--package", "sample", "--host", "codex-cli", "--package-profile", "nondefault", "--output", refused); code == 0 || !strings.Contains(errout, "project_compile_unknown_profile") {
		t.Fatalf("undeclared profile was accepted: %d %s", code, errout)
	}
	if _, err := os.Stat(refused); !os.IsNotExist(err) {
		t.Fatalf("refused compile left an output: %v", err)
	}
	if data, err := os.ReadFile(extendPath); err != nil || string(data) != "profile: file\nextensions: []\n" {
		t.Fatalf("per-Run profile rewrote the reviewed extend.yaml: %v %q", err, data)
	}
}

func TestCLIProjectInitDoesNotOverwriteProjectRunner(t *testing.T) {
	repository := filepath.Join(t.TempDir(), "repository")
	if err := os.MkdirAll(filepath.Join(repository, ".codex", "skills", "prifly-run"), 0755); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("git", "init", "-q", repository).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	var out, errout bytes.Buffer
	args := []string{"project", "init", "--repository", repository, "--state-root", filepath.Join(t.TempDir(), "authority"), "--json"}
	if code := execute(context.Background(), args, &out, &errout); code == 0 || !strings.Contains(errout.String(), "project_runner_conflict") {
		t.Fatalf("project init overwrote the existing runner: %d %s", code, errout.String())
	}
	if _, err := os.Stat(filepath.Join(repository, ".prifly")); !os.IsNotExist(err) {
		t.Fatalf("rejected project init wrote a profile: %v", err)
	}
}

func TestCLIProjectInitRefusesAuthorityInsideRepository(t *testing.T) {
	repository := filepath.Join(t.TempDir(), "repository")
	if err := os.MkdirAll(repository, 0755); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("git", "init", "-q", repository).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	var out, errout bytes.Buffer
	args := []string{"project", "init", "--repository", repository, "--state-root", filepath.Join(repository, ".prifly-local"), "--json"}
	if code := execute(context.Background(), args, &out, &errout); code == 0 || !strings.Contains(errout.String(), "unsafe_authority_root") {
		t.Fatalf("project init accepted authority inside repository: %d %s", code, errout.String())
	}
	if _, err := os.Stat(filepath.Join(repository, ".prifly")); !os.IsNotExist(err) {
		t.Fatalf("rejected project init wrote a profile: %v", err)
	}
}

func TestCLIProjectInitRequiresCoreAuthority(t *testing.T) {
	repository := filepath.Join(t.TempDir(), "repository")
	if err := os.MkdirAll(repository, 0755); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("git", "init", "-q", repository).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	stateRoot := filepath.Join(t.TempDir(), "foundation-authority")
	if err := prifly.Init(stateRoot); err != nil {
		t.Fatal(err)
	}
	var out, errout bytes.Buffer
	args := []string{"project", "init", "--repository", repository, "--state-root", stateRoot, "--json"}
	if code := execute(context.Background(), args, &out, &errout); code == 0 || !strings.Contains(errout.String(), "authority_profile_incompatible") {
		t.Fatalf("project init accepted a foundation authority: %d %s", code, errout.String())
	}
	if _, err := os.Stat(filepath.Join(repository, ".prifly")); !os.IsNotExist(err) {
		t.Fatalf("incompatible authority wrote a profile: %v", err)
	}
}

func TestCLIPackageInspectReadsOneExactSealedPackage(t *testing.T) {
	project := t.TempDir()
	source := t.TempDir()
	body := []byte("# inspected package\n")
	if err := os.MkdirAll(filepath.Join(source, "skills", "inspect"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "skills", "inspect", "SKILL.md"), body, 0600); err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256(body))
	manifest := map[string]any{
		"schema_version": "1", "id": "test:package/inspect", "version": "1.0.0", "description": "Package visible through the CLI", "requires_core_protocol": "1",
		"dependencies": []any{}, "components": []any{map[string]any{"kind": "context", "ref": map[string]any{"id": "test:context/inspect", "version": "1.0.0", "digest": digest}, "path": "skills/inspect/SKILL.md"}},
		"files":                  []any{map[string]any{"path": "skills/inspect/SKILL.md", "digest": digest, "size_bytes": len(body), "media_type": "text/markdown; charset=utf-8", "role": "data"}},
		"requested_capabilities": []any{"network"}, "license": "MIT",
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "prifly.package.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	var out, errout bytes.Buffer
	if code := execute(context.Background(), []string{"init", "--profile", flow.CoreProfile, project, "--json"}, &out, &errout); code != 0 {
		t.Fatalf("init %d: %s", code, errout.String())
	}
	out.Reset()
	errout.Reset()
	if code := execute(context.Background(), []string{"--project", project, "package", "import", "--dir", source, "--reason", "reviewed", "--command-id", "command:inspect-import", "--json"}, &out, &errout); code != 0 {
		t.Fatalf("package import %d: %s", code, errout.String())
	}
	e, err := prifly.Open(project, true)
	if err != nil {
		t.Fatal(err)
	}
	record, err := e.Packages(context.Background())
	e.Close()
	if err != nil || len(record.Packages) != 1 {
		t.Fatalf("imported package record: %v %+v", err, record)
	}
	refFile := filepath.Join(project, "package-ref.json")
	refBytes, _ := json.Marshal(record.Packages[0].Ref)
	if err := os.WriteFile(refFile, refBytes, 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errout.Reset()
	if code := execute(context.Background(), []string{"--project", project, "package", "inspect", "--ref", refFile, "--json"}, &out, &errout); code != 0 {
		t.Fatalf("package inspect %d: %s", code, errout.String())
	}
	var inspection prifly.PackageInspection
	if err := json.Unmarshal(out.Bytes(), &inspection); err != nil {
		t.Fatal(err)
	}
	if inspection.Ref != record.Packages[0].Ref || inspection.Manifest.Description != "Package visible through the CLI" || len(inspection.Manifest.RequestedCapabilities) != 1 || inspection.Manifest.RequestedCapabilities[0] != "network" {
		t.Fatalf("CLI inspection omitted sealed metadata: %+v", inspection)
	}
}

func TestCLIHelpDoesNotDenyImplementedCoreOperators(t *testing.T) {
	var out, errout bytes.Buffer
	if code := execute(context.Background(), []string{"help"}, &out, &errout); code != 0 || errout.Len() != 0 {
		t.Fatalf("help: %d %s", code, errout.String())
	}
	help := out.String()
	if strings.Contains(help, "parallel/map/wait") || strings.Contains(help, "subscriptions/early artifacts/live guards") {
		t.Fatalf("help denies implemented capabilities: %s", help)
	}
	if strings.Contains(help, "ActionIntent/Admission/Delivery") || !strings.Contains(help, "external effect delivery/execution (ActionDelivery)") {
		t.Fatalf("help does not state the actual external-action boundary: %s", help)
	}
	if !strings.Contains(help, "action propose --file COMMAND.json") || !strings.Contains(help, "action admit --file COMMAND.json") || !strings.Contains(help, "this never executes it") {
		t.Fatalf("help does not describe the implemented durable action commands: %s", help)
	}
	if !strings.Contains(help, "run fork --file REQUEST.json") {
		t.Fatalf("help does not describe the linked Run command: %s", help)
	}
	if !strings.Contains(help, "project init [--repository DIR] [--state-root DIR]") {
		t.Fatalf("help does not describe safe project profile setup: %s", help)
	}
	if !strings.Contains(help, "project workflows [--repository DIR]") {
		t.Fatalf("help does not describe project launch discovery: %s", help)
	}
	if !strings.Contains(help, "project compile --repository DIR --package NAME --host codex-cli|codex-app|claude-code --output DIR") {
		t.Fatalf("help does not describe YAML package compilation: %s", help)
	}
	if !strings.Contains(help, "project start --repository DIR --launch ID --host codex-cli|codex-app|claude-code --brief FILE") || !strings.Contains(help, "[--workspace worktree|checkout]") {
		t.Fatalf("help does not describe the declared project launch: %s", help)
	}
}

func TestCLIRefCanonicalYAMLAndJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "schema.json"), []byte(`{"type":"object","properties":{"n":{"type":"integer"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "schema.yaml"), []byte("type: object\nproperties:\n  n:\n    type: integer\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var refs []flow.Ref
	for _, path := range []string{"schema.json", "schema.yaml"} {
		var out, errout bytes.Buffer
		if code := execute(context.Background(), []string{"--project", dir, "ref", path, "--id", "test:schema", "--version", "1.0.0", "--json"}, &out, &errout); code != 0 {
			t.Fatalf("ref %s: %s", path, errout.String())
		}
		var ref flow.Ref
		if err := json.Unmarshal(out.Bytes(), &ref); err != nil {
			t.Fatal(err)
		}
		refs = append(refs, ref)
	}
	if refs[0] != refs[1] {
		t.Fatal("JSON/YAML definitions have different canonical identity")
	}
}

func TestCLIUsesConciseWorkflowYAMLForRefAndPreview(t *testing.T) {
	project := t.TempDir()
	workflow := emptyCLIWorkflow(t)
	source := fmt.Sprintf(`authoring: prifly-workflow/1
id: %s
version: %s
title: %s
entry: done
limits:
  max_step_instances: 1
  max_control_transitions: 4
policy_ref:
  id: %s
  version: %s
  digest: %s
stages:
  done:
    kind: finish
    outcome: no_work
`, workflow.ID, workflow.Version, workflow.Title, workflow.PolicyRef.ID, workflow.PolicyRef.Version, workflow.PolicyRef.Digest)
	if err := os.WriteFile(filepath.Join(project, "workflow.yaml"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	var out, errout bytes.Buffer
	if code := execute(context.Background(), []string{"init", project, "--json"}, &out, &errout); code != 0 {
		t.Fatalf("init: %s", errout.String())
	}
	out.Reset()
	errout.Reset()
	if code := execute(context.Background(), []string{"--project", project, "ref", "workflow.yaml", "--id", workflow.ID, "--version", workflow.Version, "--json"}, &out, &errout); code != 0 {
		t.Fatalf("ref: %s", errout.String())
	}
	var ref flow.Ref
	if err := json.Unmarshal(out.Bytes(), &ref); err != nil {
		t.Fatal(err)
	}
	want, err := flow.Digest(encodedWorkflow(t, workflow))
	if err != nil {
		t.Fatal(err)
	}
	if ref.Digest != want {
		t.Fatalf("ref hashed author source instead of lowered revision: %s != %s", ref.Digest, want)
	}
	out.Reset()
	errout.Reset()
	if code := execute(context.Background(), []string{"--project", project, "preview", "--workflow", "workflow.yaml", "--json"}, &out, &errout); code != 0 {
		t.Fatalf("preview: %s", errout.String())
	}
	var preview prifly.Preview
	if err := json.Unmarshal(out.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.WorkflowRef.Digest != want || !preview.Validation.ShapeValid || !preview.Validation.GraphValid {
		t.Fatalf("preview did not use lowered workflow: %+v", preview)
	}
}

func encodedWorkflow(t *testing.T, workflow flow.WorkflowRevision) []byte {
	t.Helper()
	data, err := json.Marshal(workflow)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestCLIRefRawTextUsesExactUTF8Bytes(t *testing.T) {
	for name, data := range map[string][]byte{
		"empty":    {},
		"markdown": []byte("\ufeff# Правила\r\n ${HOME} $(touch not-executed)\r\ne\u0301 / é\n "),
		"json":     []byte(" { \"text\": \"значение\" }\r\n"),
		"limit":    bytes.Repeat([]byte("x"), prifly.MaxDefinitionBytes),
	} {
		t.Run(name, func(t *testing.T) {
			project := t.TempDir()
			path := "resource.yaml"
			if err := os.WriteFile(filepath.Join(project, path), data, 0600); err != nil {
				t.Fatal(err)
			}
			var out, errout bytes.Buffer
			args := []string{"--project", project, "ref", path, "--id", "test:context/cli", "--version", "1.0.0", "--raw-text", "--json"}
			if code := execute(context.Background(), args, &out, &errout); code != 0 || errout.Len() != 0 {
				t.Fatalf("raw ref: exit %d %s", code, errout.String())
			}
			var ref flow.Ref
			if err := json.Unmarshal(out.Bytes(), &ref); err != nil {
				t.Fatal(err)
			}
			if err := flow.ValidateProtocol("ImmutableRef", out.Bytes()); err != nil || ref.ID != "test:context/cli" || ref.Version != "1.0.0" || ref.Digest != fmt.Sprintf("sha256:%x", sha256.Sum256(data)) {
				t.Fatal("raw text mode normalized bytes or changed the reference contract", err)
			}
			if name == "json" {
				canonicalDigest, err := flow.Digest(data)
				if err != nil || canonicalDigest == ref.Digest {
					t.Fatal("JSON-looking raw text silently became canonical JSON", err)
				}
			}
			if _, err := os.Stat(filepath.Join(project, ".prifly")); !os.IsNotExist(err) {
				t.Fatal("computing a raw reference imported data or created an authority")
			}
		})
	}
}

func TestCLIRefRawTextRejectsInvalidBytes(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		data       []byte
	}{
		{"utf8", "invalid_unicode", append([]byte("RAW-CONTEXT-CANARY"), 0xff)},
		{"size", "quota_exceeded", bytes.Repeat([]byte("x"), prifly.MaxDefinitionBytes+1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			project := t.TempDir()
			if err := os.WriteFile(filepath.Join(project, "resource"), tc.data, 0600); err != nil {
				t.Fatal(err)
			}
			var out, errout bytes.Buffer
			args := []string{"--project", project, "ref", "resource", "--id", "test:context/cli", "--version", "1.0.0", "--raw-text", "--json"}
			if code := execute(context.Background(), args, &out, &errout); code == 0 || out.Len() != 0 {
				t.Fatal("invalid raw text received an exact reference")
			}
			if err := flow.ValidateProtocol("Problem", errout.Bytes()); err != nil || strings.Contains(errout.String(), "RAW-CONTEXT-CANARY") {
				t.Fatal("raw ref rejection exposed input bytes or lost its public contract", err)
			}
			var rejection prifly.Problem
			if err := json.Unmarshal(errout.Bytes(), &rejection); err != nil || rejection.Code != tc.code {
				t.Fatalf("raw ref rejection = %s, want %s (%v)", rejection.Code, tc.code, err)
			}
		})
	}
}

func TestCLIOutputDoesNotExecuteControlText(t *testing.T) {
	var out bytes.Buffer
	data := map[string]any{"=EXEC()": "\x1b[31mnot a command", "quoted": "<script>alert(1)</script>"}
	c := cli{format: "json", out: &out}
	if err := c.emit(data); err != nil {
		t.Fatal(err)
	}
	if bytes.ContainsRune(out.Bytes(), '\x1b') || strings.Contains(out.String(), "<script>") {
		t.Fatal("terminal/HTML control text was not escaped")
	}
	out.Reset()
	if err := writeCSV(&out, data); err != nil {
		t.Fatal(err)
	}
	records, err := csv.NewReader(&out).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 || records[1][0] != "'=EXEC()" {
		t.Fatalf("CSV formula cell not neutralized: %v", records)
	}
	for _, value := range []string{"=1", " +1", "-1", "@cell", "\t=2", "\r=3"} {
		if !strings.HasPrefix(csvSafe(value), "'") {
			t.Fatalf("unsafe CSV: %q", value)
		}
	}

	// Exercise the real preview/read/error path with untrusted prose, not just
	// a renderer in isolation. The local endpoint observes any unwanted fetch.
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	project := t.TempDir()
	if err := prifly.Init(project); err != nil {
		t.Fatal(err)
	}
	workflow := emptyCLIWorkflow(t)
	opaque := `<img src="` + server.URL + `/remote-image?token=PREVIEW-CANARY"><a href="javascript:alert(1)">data</a>` + "\x1b]8;;" + server.URL + "\x1b\\"
	brief := prifly.Brief{SchemaVersion: "1", ID: "test:brief/preview-safety", Subject: opaque, DesiredOutcome: "No execution", InScope: []string{"Preview only"}, OutOfScope: []string{}, CompletionCriteria: []string{"Display author text safely"}, SourceRefs: []prifly.ArtifactRef{}, Assumptions: []string{}, Confirmation: "explicit"}
	for path, value := range map[string]any{"workflows/preview.json": workflow, "brief.json": brief} {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(project, path), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, format := range []string{"text", "json"} {
		out.Reset()
		var errout bytes.Buffer
		if code := execute(context.Background(), []string{"--project", project, "preview", "--workflow", "workflows/preview.json", "--brief", "brief.json", "--format", format}, &out, &errout); code != 0 {
			t.Fatalf("preview %s: %d %s", format, code, errout.String())
		}
		var preview prifly.Preview
		if err := json.Unmarshal(out.Bytes(), &preview); err != nil || preview.Brief == nil || preview.Brief.Subject != opaque || preview.Admission {
			t.Fatal("safe preview changed the author data or admitted work", err)
		}
		if bytes.ContainsRune(out.Bytes(), '\x1b') || strings.Contains(out.String(), "<img") || strings.Contains(out.String(), "<a ") {
			t.Fatal("real preview emitted active control markup")
		}
	}
	invalid, err := json.Marshal(map[string]string{"schema_version": "1", "unknown_field": opaque})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "brief.json"), invalid, 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	var errout bytes.Buffer
	if code := execute(context.Background(), []string{"--project", project, "preview", "--workflow", "workflows/preview.json", "--brief", "brief.json", "--json"}, &out, &errout); code == 0 || out.Len() != 0 {
		t.Fatal("invalid brief was accepted or mixed with a success response")
	}
	if err := flow.ValidateProtocol("Problem", errout.Bytes()); err != nil || strings.Contains(errout.String(), "PREVIEW-CANARY") || strings.Contains(errout.String(), server.URL) || bytes.ContainsRune(errout.Bytes(), '\x1b') {
		t.Fatal("unsafe author payload escaped through Problem", err)
	}
	if requests.Load() != 0 {
		t.Fatal("preview/error handling fetched a remote image or link")
	}
}

func TestCLIClosedJSONAndUnknownFlags(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "request.json")
	if err := os.WriteFile(path, []byte(`{"id":"a","id":"b"}`), 0600); err != nil {
		t.Fatal(err)
	}
	var ref flow.Ref
	if err := readJSON(path, &ref); err == nil {
		t.Fatal("duplicate keys accepted")
	}
	if err := os.WriteFile(path, []byte(`{"id":"a","magic":"secret"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := readJSON(path, &ref); err == nil {
		t.Fatal("unknown field accepted")
	}
	var out, errout bytes.Buffer
	if code := execute(context.Background(), []string{"ref", path, "--execute", "rm", "--json"}, &out, &errout); code != 2 {
		t.Fatalf("unknown flag: %d", code)
	}
	if strings.Contains(errout.String(), "secret") {
		t.Fatal("rejected JSON value leaked into a Problem")
	}

	project := t.TempDir()
	if err := prifly.Init(project); err != nil {
		t.Fatal(err)
	}
	reader, err := prifly.Open(project, true)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	_, cut, err := reader.Store.ReadAll(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	valid, err := json.Marshal(emptyCLIWorkflow(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "workflows/valid.json"), valid, 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errout.Reset()
	if code := execute(context.Background(), []string{"--project", project, "validate", "--workflow", "workflows/valid.json", "--json"}, &out, &errout); code != 0 {
		t.Fatal("valid framing baseline failed", errout.String())
	}
	future := bytes.Replace(valid, []byte(`"schema_version":"1"`), []byte(`"schema_version":"2"`), 1)
	capacity := bytes.Replace(valid, []byte(`"max_step_instances":1`), []byte(`"max_step_instances":257`), 1)
	safety := append(bytes.Clone(valid[:len(valid)-1]), []byte(`,"ignore_authority":"SECRET-FRAMING-CANARY"}`)...)
	for name, input := range map[string]struct{ body, code string }{
		"duplicate":      {`{"schema_version":"1","schema_version":"1"}`, "duplicate_key"},
		"invalid_utf8":   {string([]byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'}), "invalid_unicode"},
		"future_version": {string(future), "schema_invalid"},
		"policy_limit":   {string(capacity), "resource_limit"},
		"bytes":          {strings.Repeat(" ", flow.MaxDocumentBytes+1), "unsafe_path"},
		"depth":          {strings.Repeat("[", flow.MaxDepth+1) + "0" + strings.Repeat("]", flow.MaxDepth+1), "document_limit"},
		"nodes":          {"[" + strings.Repeat("0,", flow.MaxNodes) + "0]", "document_limit"},
		"safety_field":   {string(safety), "schema_invalid"},
		"defs_only":      {`{"$defs":{"WorkflowRevision":{"type":"object"}}}`, "schema_invalid"},
		"trailing":       {`{"schema_version":"1"} {}`, "invalid_json"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(project, "workflows/invalid.json"), []byte(input.body), 0600); err != nil {
				t.Fatal(err)
			}
			for _, operation := range [][]string{
				{"validate", "--workflow", "workflows/invalid.json"},
				{"run", "start", "--workflow", "workflows/invalid.json", "--brief", "absent.json", "--command-id", "command:invalid-" + name, "--drive"},
			} {
				out.Reset()
				errout.Reset()
				arguments := append([]string{"--project", project, "--json"}, operation...)
				if code := execute(context.Background(), arguments, &out, &errout); code == 0 || out.Len() != 0 {
					t.Fatal("malformed public input was accepted or silently truncated")
				}
				if err := flow.ValidateProtocol("Problem", errout.Bytes()); err != nil || strings.Contains(errout.String(), "SECRET-FRAMING-CANARY") {
					t.Fatal("malformed input did not produce a safe concrete Problem", err)
				}
				var rejection prifly.Problem
				if err := json.Unmarshal(errout.Bytes(), &rejection); err != nil || rejection.Code != input.code {
					t.Fatalf("wrong rejection boundary: got %s, want %s (%v)", rejection.Code, input.code, err)
				}
				runs, after, err := reader.Store.ReadAll(context.Background(), 10)
				if err != nil || len(runs) != 0 || after != cut {
					t.Fatal("framing rejection changed committed authority state", err)
				}
				if slot, _, err := reader.Store.Slot(context.Background()); err != nil || slot != "" {
					t.Fatal("framing rejection admitted execution", err)
				}
			}
		})
	}
}

func TestCLISourceImportReturnsImmutableSnapshot(t *testing.T) {
	for _, format := range []string{"json", "blob"} {
		t.Run(format, func(t *testing.T) {
			project := t.TempDir()
			command := func(args ...string) []byte {
				t.Helper()
				var out, errout bytes.Buffer
				args = append([]string{"--project", project, "--json"}, args...)
				if code := execute(context.Background(), args, &out, &errout); code != 0 || errout.Len() != 0 {
					t.Fatalf("%v: exit %d %s", args, code, errout.String())
				}
				return bytes.Clone(out.Bytes())
			}
			write := func(path string, data []byte) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(project, path), data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			command("init", "--profile", flow.CoreProfile)
			data := []byte("\ufeff# Источник\r\n<system>reference data</system>\r\n")
			media := "text/markdown; charset=utf-8"
			var schema *flow.Ref
			args := []string{"source", "import", "--file", "source", "--type", format}
			if format == "json" {
				data, media = []byte("{ \"value\": 7 }\r\n"), "application/json"
				write("schemas/source.json", []byte(`{"type":"object","required":["value"],"properties":{"value":{"type":"integer"}},"additionalProperties":false}`))
				refBytes := command("ref", "schemas/source.json", "--id", "test:schema/source", "--version", "1.0.0")
				write("schema-ref.json", refBytes)
				schema = &flow.Ref{}
				if err := json.Unmarshal(refBytes, schema); err != nil {
					t.Fatal(err)
				}
				registry, err := json.Marshal(prifly.RegistryFile{SchemaVersion: "1", Entries: []prifly.Definition{{Ref: *schema, Kind: "schema", Path: "schemas/source.json"}}})
				if err != nil {
					t.Fatal(err)
				}
				write("definitions.json", registry)
				args = append(args, "--schema-ref", "schema-ref.json")
			}
			write("source", data)
			identity, version, scope := "tracker:opaque/item", "revision-7", "../declared-only/$(touch not-executed)"
			args = append(args, "--media-type", media, "--external-identity", identity, "--external-version", version, "--external-scope", scope)
			reader, err := prifly.Open(project, true)
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			_, before, err := reader.Store.ReadAll(context.Background(), 10)
			if err != nil {
				t.Fatal(err)
			}
			var response struct {
				SchemaVersion string             `json:"schema_version"`
				Ref           prifly.ArtifactRef `json:"ref"`
				Artifact      prifly.Artifact    `json:"artifact"`
			}
			if err := json.Unmarshal(command(args...), &response); err != nil {
				t.Fatal(err)
			}
			artifactBytes, err := json.Marshal(response.Artifact)
			if err != nil {
				t.Fatal(err)
			}
			if err := flow.ValidateProtocol("ArtifactRevision", artifactBytes); err != nil || response.SchemaVersion != "foundation-artifact/1" || response.Ref != response.Artifact.Ref() {
				t.Fatal("source import did not return the public artifact descriptor", err)
			}
			snapshot, err := reader.SourceSnapshot(response.Ref)
			if err != nil {
				t.Fatal(err)
			}
			if snapshot.Scope != (prifly.SourceScope{Root: reader.Root, Path: "source"}) || snapshot.ExternalIdentity != identity || snapshot.ExternalVersion != version || snapshot.ExternalScope != scope || snapshot.Observed.UTC == "" {
				t.Fatal("CLI changed acquisition scope, declared metadata or observation", snapshot)
			}
			content, retained, err := reader.Artifact(snapshot.ContentRef)
			if err != nil || !bytes.Equal(retained, data) || content.Format != format || content.MediaType != media || content.Ref() == response.Ref {
				t.Fatal("CLI returned live/canonicalized bytes or confused the descriptor with content", err)
			}
			if schema != nil && (content.SchemaRef == nil || *content.SchemaRef != *schema) {
				t.Fatal("CLI did not pass the exact content schema")
			}
			if response.Artifact.Producer["kind"] != "import" || content.Producer["kind"] != "import" || response.Artifact.Producer["import_id"] != content.Producer["import_id"] || len(response.Artifact.Provenance) != 1 || response.Artifact.Provenance[0] != content.Ref() {
				t.Fatal("source import lost its acquisition/content provenance")
			}
			// The ordinary artifact commands can inspect/export a snapshot and
			// its content without another source read or a new source command.
			write("source", []byte("changed after import"))
			for name, ref := range map[string]prifly.ArtifactRef{"snapshot": response.Ref, "content": snapshot.ContentRef} {
				refBytes, err := json.Marshal(ref)
				if err != nil {
					t.Fatal(err)
				}
				write(name+"-ref.json", refBytes)
				command("artifact", "inspect", "--ref", name+"-ref.json")
				command("artifact", "export", "--ref", name+"-ref.json", "--output", name+"-export")
			}
			exported, err := os.ReadFile(filepath.Join(project, "content-export"))
			if err != nil || !bytes.Equal(exported, data) {
				t.Fatal("artifact export followed the changed source", err)
			}
			descriptor, err := os.ReadFile(filepath.Join(project, "snapshot-export"))
			if err != nil {
				t.Fatal(err)
			}
			if exportedSnapshot, err := prifly.ParseSourceSnapshot(descriptor); err != nil || exportedSnapshot != snapshot {
				t.Fatal("exported snapshot differs from the recorded acquisition", err)
			}
			runs, after, err := reader.Store.ReadAll(context.Background(), 10)
			if err != nil || len(runs) != 0 || after != before {
				t.Fatal("source CLI created a Run or command journal entries", err)
			}
			if slot, _, err := reader.Store.Slot(context.Background()); err != nil || slot != "" {
				t.Fatal("source CLI admitted a worker", err)
			}
		})
	}
}

func TestCLITaskPrepareSealsProviderNeutralInput(t *testing.T) {
	project := t.TempDir()
	command := func(args ...string) []byte {
		t.Helper()
		var out, errout bytes.Buffer
		args = append([]string{"--project", project, "--json"}, args...)
		if code := execute(context.Background(), args, &out, &errout); code != 0 || errout.Len() != 0 {
			t.Fatalf("%v: exit %d %s", args, code, errout.String())
		}
		return bytes.Clone(out.Bytes())
	}
	command("init", "--profile", flow.CoreProfile)
	taskInput := []byte(`{
  "schema_version": "task-input/1",
  "id": "task:tracker-17",
  "title": "Add the animated Pri-Fly page",
  "raw_text": "Write an HTML page that animates Pri-Fly with GSAP.",
  "desired_outcome": "A committed page demonstrates the requested animation.",
  "in_scope": ["The selected repository"],
  "out_of_scope": ["Publishing the page"],
  "completion_criteria": ["The page renders Pri-Fly", "The animation runs in a browser"],
  "source": {"type": "gitlab", "external_id": "group/project#17", "url": "https://gitlab.example.test/group/project/-/issues/17", "fetched_at": "2026-08-31T12:00:00Z", "version": "updated-17"},
  "source_refs": [],
  "assumptions": ["GSAP is available in the project"],
  "confirmation": "explicit"
}`)
	if err := flow.ValidateProtocol("TaskInput", taskInput); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "task.json"), taskInput, 0600); err != nil {
		t.Fatal(err)
	}
	type prepared struct {
		SchemaVersion  string             `json:"schema_version"`
		TaskID         string             `json:"task_id"`
		BriefPath      string             `json:"brief_path"`
		Brief          prifly.Brief       `json:"brief"`
		SourceSnapshot prifly.ArtifactRef `json:"source_snapshot"`
	}
	decode := func(data []byte) prepared {
		t.Helper()
		var value prepared
		if err := json.Unmarshal(data, &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	first := decode(command("task", "prepare", "--input", "task.json"))
	authorityRoot, err := canonicalProjectPath(project)
	if err != nil {
		t.Fatal(err)
	}
	wantBriefPath := taskBriefPath(authorityRoot, taskInput)
	if first.SchemaVersion != "task-prepared/1" || first.TaskID != "task:tracker-17" || first.BriefPath != wantBriefPath {
		t.Fatalf("unexpected task preparation: got=%+v want_brief_path=%s", first, wantBriefPath)
	}
	briefBytes, err := os.ReadFile(first.BriefPath)
	if err != nil || flow.ValidateProtocol("RunBrief", briefBytes) != nil {
		t.Fatalf("prepared brief is not a usable RunBrief: %v", err)
	}
	if first.Brief.Subject != "Add the animated Pri-Fly page" || len(first.Brief.SourceRefs) != 1 || first.Brief.SourceRefs[0] != first.SourceSnapshot || first.Brief.Confirmation != "explicit" {
		t.Fatalf("task projection changed owner input: %+v", first.Brief)
	}
	reader, err := prifly.Open(project, true)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := reader.SourceSnapshot(first.SourceSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	_, sealed, err := reader.Artifact(snapshot.ContentRef)
	if err != nil || !bytes.Equal(sealed, taskInput) || snapshot.ExternalIdentity != "group/project#17" || snapshot.ExternalVersion != "updated-17" || snapshot.ExternalScope != "https://gitlab.example.test/group/project/-/issues/17" {
		t.Fatalf("task input was not sealed with declared provenance: %+v %v", snapshot, err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(project, ".prifly", "artifact-refs"))
	if err != nil || len(entries) != 2 {
		t.Fatalf("task intake artifacts = %d, want content and snapshot (%v)", len(entries), err)
	}
	second := decode(command("task", "prepare", "--input", "task.json"))
	if second.BriefPath != first.BriefPath || second.SourceSnapshot != first.SourceSnapshot {
		t.Fatalf("exact task retry created a second intake: first=%+v second=%+v", first, second)
	}
	entries, err = os.ReadDir(filepath.Join(project, ".prifly", "artifact-refs"))
	if err != nil || len(entries) != 2 {
		t.Fatalf("exact task retry created artifacts: %d %v", len(entries), err)
	}
	if err := os.WriteFile(filepath.Join(project, "invalid-task.json"), []byte(`{"schema_version":"task-input/1","unknown":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	var out, errout bytes.Buffer
	if code := execute(context.Background(), []string{"--project", project, "--json", "task", "prepare", "--input", "invalid-task.json"}, &out, &errout); code == 0 || out.Len() != 0 {
		t.Fatal("invalid TaskInput created a task preparation")
	}
	entries, err = os.ReadDir(filepath.Join(project, ".prifly", "artifact-refs"))
	if err != nil || len(entries) != 2 {
		t.Fatalf("invalid TaskInput published artifacts: %d %v", len(entries), err)
	}
}

func TestCLISourceImportRejectsBeforePublication(t *testing.T) {
	project := t.TempDir()
	if err := prifly.InitProfile(project, flow.CoreProfile); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{
		"source":   []byte(`{"value":7}`),
		"bad-ref":  []byte(`{"unknown":"SOURCE-SECRET-CANARY"}`),
		"bad-text": {0xff},
	} {
		if err := os.WriteFile(filepath.Join(project, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	reader, err := prifly.Open(project, true)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	_, before, err := reader.Store.ReadAll(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, code string
		args       []string
	}{
		{"missing_operation", "invalid_usage", []string{}},
		{"unknown_operation", "invalid_usage", []string{"inspect"}},
		{"missing_file", "invalid_usage", []string{"import", "--type", "blob"}},
		{"missing_type", "invalid_usage", []string{"import", "--file", "source"}},
		{"unknown_type", "invalid_usage", []string{"import", "--file", "source", "--type", "yaml"}},
		{"unknown_flag", "invalid_usage", []string{"import", "--file", "source", "--type", "blob", "--execute", "SOURCE-SECRET-CANARY"}},
		{"no_command_retry", "invalid_usage", []string{"import", "--file", "source", "--type", "blob", "--command-id", "command:unsupported"}},
		{"extra_argument", "invalid_usage", []string{"import", "--file", "source", "--type", "blob", "extra"}},
		{"invalid_reference", "invalid_usage", []string{"import", "--file", "source", "--type", "json", "--schema-ref", "bad-ref"}},
		{"missing_schema", "source_content_invalid", []string{"import", "--file", "source", "--type", "json"}},
		{"invalid_text", "source_content_invalid", []string{"import", "--file", "bad-text", "--type", "blob", "--media-type", "text/plain"}},
		{"invalid_scope", "source_metadata_invalid", []string{"import", "--file", "source", "--type", "blob", "--external-scope", strings.Repeat("s", 1025)}},
		{"unsafe_path", "unsafe_path", []string{"import", "--file", "../source", "--type", "blob"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, errout bytes.Buffer
			args := append([]string{"--project", project, "--json", "source"}, tc.args...)
			if code := execute(context.Background(), args, &out, &errout); code == 0 || out.Len() != 0 {
				t.Fatal("invalid source command succeeded or mixed a result with the Problem")
			}
			if err := flow.ValidateProtocol("Problem", errout.Bytes()); err != nil || strings.Contains(errout.String(), "SOURCE-SECRET-CANARY") {
				t.Fatal("source rejection exposed invalid payload or lost the public Problem", err)
			}
			var rejection prifly.Problem
			if err := json.Unmarshal(errout.Bytes(), &rejection); err != nil || rejection.Code != tc.code {
				t.Fatalf("source rejection = %s, want %s (%v)", rejection.Code, tc.code, err)
			}
			if entries, err := os.ReadDir(filepath.Join(project, ".prifly", "artifact-refs")); err != nil || len(entries) != 0 {
				t.Fatal("rejected source import published an artifact", err)
			}
			runs, after, err := reader.Store.ReadAll(context.Background(), 10)
			if err != nil || len(runs) != 0 || after != before {
				t.Fatal("rejected source command changed Run state", err)
			}
			if slot, _, err := reader.Store.Slot(context.Background()); err != nil || slot != "" {
				t.Fatal("rejected source command admitted a worker", err)
			}
		})
	}
}

// The selected package profile must reach the handoff as sealed bytes: the
// tree capture a host receives is the one for the profile of this Run, and a
// launch that names no profile falls back to the reviewed default rather than
// to whatever the previous Run chose.
func TestCLIProjectStartSealsSelectedProfileIntoHandoff(t *testing.T) {
	launch := func(t *testing.T, profile string) (string, projectStartResult, prifly.SessionTask) {
		t.Helper()
		repository, authority := newProjectFixture(t)
		writeFixtureFile(t, repository, ".codex/skills/work/SKILL.md", "# Work\n")
		writeFixtureFile(t, repository, ".prifly/workflows/sample/contexts/work.yaml", `id: test:context/work
version: 1.0.0
media_type: text/markdown; charset=utf-8
source: {root: host_skills, path: work/SKILL.md}
`)
		writeFixtureFile(t, repository, ".prifly/workflows/sample/steps/work.yaml", `authoring: prifly-step/1
id: test:step/work
version: 1.0.0
title: Work
kind: worker
outputs:
  plan: {schema_ref: "{{workspace_tree_manifest}}", required_for: [pass]}
executor: {adapter_ref: "{{assisted_adapter}}", operation: session}
instructions_ref: "{{context_work}}"
effects: {class: workspace_write, retry_class: never}
result_schema_ref: "{{step_result_schema}}"
workspace_trees:
  - output_port: plan
    capture: "{{plan_capture}}"
`)
		writeFixtureFile(t, repository, ".prifly/workflows/sample/workflow.yaml", `authoring: prifly-project-workflow/1
package:
  id: test:package/sample
  version: 1.0.0
  description: Package profile handoff fixture.
  requires_core_protocol: "1"
  profiles:
    default: file
    values:
      file: {plan_capture: {kind: exact_file, path: plans/PLAN.md}}
      tree: {plan_capture: {kind: direct_child_tree, path: plans, entrypoint: index.md}}
  references:
    assisted_adapter: core:adapter/assisted-session@1.0.0
    local_policy: core:policy/local@3.0.0
    step_result_schema: core:schema/step-result@1.0.0
    workspace_tree_manifest: core:schema/workspace-tree-manifest@1.0.0
id: test:workflow/sample
version: 1.0.0
refs:
  step_work: "{{step_work}}"
  local_policy: "{{local_policy}}"
inputs: {}
outputs: {}
entry: work
limits: {max_step_instances: 1, max_control_transitions: 2}
policy_ref: local_policy
stages:
  work: {kind: step, step_ref: step_work, input_bindings: {}, on: {pass: done}}
  done: {kind: finish, outcome: succeeded, output_bindings: {}}
`)
		writeFixtureFile(t, repository, ".prifly/workflows/sample/extend.yaml", "profile: file\nextensions: []\n")
		writeFixtureFile(t, repository, ".prifly/project.yaml", "schema_version: prifly-project-profile/2\n"+projectHostsYAML+`packages:
  sample:
    source: .prifly/workflows/sample
launches:
  sample:
    title: Sample
    description: Package profile handoff fixture.
    kind: workflow
    workflow: .prifly/workflows/sample/workflow.yaml
`)
		gitFixture(t, repository, "add", ".")
		gitFixture(t, repository, "commit", "-qm", "declare sample")
		brief := filepath.Join(t.TempDir(), "brief.json")
		if err := os.WriteFile(brief, []byte(`{"schema_version":"1","id":"test:brief/sample","subject":"Sample","desired_outcome":"Reach the handoff","in_scope":["handoff"],"out_of_scope":[],"completion_criteria":["handoff"],"source_refs":[],"assumptions":[],"confirmation":"explicit"}`), 0600); err != nil {
			t.Fatal(err)
		}
		args := []string{"--project", authority, "project", "start", "--repository", repository, "--launch", "sample", "--host", "codex-cli", "--brief", brief}
		if profile != "" {
			args = append(args, "--package-profile", profile)
		}
		code, out, errout := runCLI(t, args...)
		if code != 0 {
			t.Fatalf("project start with profile %q: %d %s", profile, code, errout)
		}
		var result projectStartResult
		if err := json.Unmarshal([]byte(out), &result); err != nil {
			t.Fatal(err)
		}
		e, err := prifly.Open(authority, true)
		if err != nil {
			t.Fatal(err)
		}
		defer e.Close()
		task, err := e.SessionTask(context.Background(), result.Run.Run.ID, "")
		if err != nil {
			t.Fatalf("no handoff after project start: %v", err)
		}
		return repository, result, task
	}
	repository, explicit, task := launch(t, "tree")
	if explicit.PackageProfile != "tree" || explicit.DecisionSheet == nil || explicit.DecisionSheet.PackageProfile != "tree" {
		t.Fatalf("explicit profile was not sealed into the launch: %+v", explicit)
	}
	if task.DecisionSheet == nil || task.DecisionSheet.PackageProfile != "tree" || len(task.WorkspaceTrees) != 1 {
		t.Fatalf("handoff does not carry the selected profile: %+v", task)
	}
	if capture := task.WorkspaceTrees[0].Capture; capture.Kind != "direct_child_tree" || capture.Path != "plans" || capture.Entrypoint != "index.md" {
		t.Fatalf("handoff capture does not match the selected profile: %+v", capture)
	}
	if data, err := os.ReadFile(filepath.Join(repository, ".prifly", "workflows", "sample", "extend.yaml")); err != nil || string(data) != "profile: file\nextensions: []\n" {
		t.Fatalf("per-Run profile rewrote the reviewed default: %v %q", err, data)
	}
	_, reviewed, task := launch(t, "")
	if reviewed.PackageProfile != "file" || task.DecisionSheet == nil || task.DecisionSheet.PackageProfile != "file" || len(task.WorkspaceTrees) != 1 {
		t.Fatalf("launch without a profile did not use the reviewed default: %+v %+v", reviewed, task)
	}
	if capture := task.WorkspaceTrees[0].Capture; capture.Kind != "exact_file" || capture.Path != "plans/PLAN.md" || capture.Entrypoint != "" {
		t.Fatalf("reviewed default did not seal its own capture: %+v", capture)
	}
}

// Reading the form of a command is not an operation on a project: a help or
// version request answers itself instead of opening an authority and reporting
// that some object was not found.
func TestHelpAndVersionAnswerWithoutAnAuthority(t *testing.T) {
	for _, test := range []struct {
		name     string
		args     []string
		contains string
	}{
		{"subcommand help", []string{"--project", "/nonexistent-authority", "session", "submit", "--help"}, "session submit --file SUBMISSION.json"},
		{"short flag", []string{"run", "status", "-h"}, "run status|next|explain|events|timing"},
		{"alternation token", []string{"run", "explain", "--help"}, "run status|next|explain"},
		{"nested command", []string{"project", "workflows", "add", "--help"}, "project workflows add SOURCE"},
		{"topic", []string{"help", "session"}, "session submit --file SUBMISSION.json"},
		{"version flag", []string{"--version"}, "\"version\""},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out, errout bytes.Buffer
			if code := execute(context.Background(), test.args, &out, &errout); code != 0 {
				t.Fatalf("a read of the command form was refused: %d %s", code, errout.String())
			}
			if !strings.Contains(out.String(), test.contains) {
				t.Fatalf("output does not document the asked command: %s", out.String())
			}
		})
	}
	var out, errout bytes.Buffer
	if code := execute(context.Background(), []string{"help", "session"}, &out, &errout); code != 0 {
		t.Fatal(errout.String())
	}
	var full bytes.Buffer
	if code := execute(context.Background(), []string{"help"}, &full, &errout); code != 0 {
		t.Fatal(errout.String())
	}
	if out.Len() >= full.Len() || strings.Contains(out.String(), "run drive") {
		t.Fatalf("a topic printed the whole help: %d of %d bytes", out.Len(), full.Len())
	}
	out.Reset()
	errout.Reset()
	if code := execute(context.Background(), []string{"help", "nonexistent"}, &out, &errout); code == 0 || !strings.Contains(errout.String(), "invalid_usage") {
		t.Fatalf("an unknown topic was accepted: %d %s", code, errout.String())
	}
}

// A usage refusal repeats what it received: a path the caller's shell truncated
// looks exactly like a tool defect until the value is visible.
func TestUsageRefusalRepeatsTheReceivedValue(t *testing.T) {
	var out, errout bytes.Buffer
	if code := execute(context.Background(), []string{"--format", "yaml", "version"}, &out, &errout); code == 0 {
		t.Fatal("an unsupported format was accepted")
	}
	if !strings.Contains(errout.String(), "yaml") {
		t.Fatalf("the refusal hid the received value: %s", errout.String())
	}
}

// A caller that must name an exact contract has to be able to read the set of
// names, and to use the reference form its own task already carries.
func TestSchemaListsContractsAndAcceptsDeclaredReferences(t *testing.T) {
	var out, errout bytes.Buffer
	if code := execute(context.Background(), []string{"--json", "schema"}, &out, &errout); code != 0 {
		t.Fatalf("listing the contracts was refused: %d %s", code, errout.String())
	}
	var index struct {
		SchemaVersion string   `json:"schema_version"`
		Contracts     []string `json:"contracts"`
	}
	if err := json.Unmarshal(out.Bytes(), &index); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"StepResult", "SessionSubmissionV5", "WorkspaceTreeLocation"} {
		if !slices.Contains(index.Contracts, name) {
			t.Fatalf("the index omits %s", name)
		}
	}
	for _, name := range index.Contracts {
		if _, err := contractSchema(name); err != nil {
			t.Fatalf("a listed contract does not resolve: %s %v", name, err)
		}
	}
	direct, err := contractSchema("StepResult")
	if err != nil {
		t.Fatal(err)
	}
	declared, err := contractSchema("core:schema/step-result")
	if err != nil {
		t.Fatalf("the reference form of a handed contract was refused: %v", err)
	}
	if !bytes.Equal(direct, declared) {
		t.Fatal("the two forms of one contract name returned different bytes")
	}
	if _, err := contractSchema("core:schema/not-a-contract"); err == nil {
		t.Fatal("an unknown reference resolved")
	}
}

// Changing the binary a project runs is a declared command: the machine-only
// file is otherwise edited by hand, and the authority it names must not move.
func TestProjectLocalSetReplacesOnlyTheExecutable(t *testing.T) {
	repository := t.TempDir()
	if output, err := exec.Command("git", "init", "-q", repository).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	authority := t.TempDir()
	var out, errout bytes.Buffer
	if code := execute(context.Background(), []string{"project", "init", "--repository", repository, "--state-root", authority}, &out, &errout); code != 0 {
		t.Fatalf("project init: %d %s", code, errout.String())
	}
	path := filepath.Join(repository, ".prifly", "local.yaml")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "prifly")
	out.Reset()
	errout.Reset()
	if code := execute(context.Background(), []string{"project", "local", "set", "--repository", repository, "--executable", target}, &out, &errout); code != 0 {
		t.Fatalf("project local set: %d %s", code, errout.String())
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), strconv.Quote(target)) {
		t.Fatalf("the executable was not replaced: %s", after)
	}
	for _, line := range strings.Split(string(before), "\n") {
		if strings.HasPrefix(line, "prifly_executable:") || line == "" {
			continue
		}
		if !strings.Contains(string(after), line) {
			t.Fatalf("an unrelated line changed: %q", line)
		}
	}
	out.Reset()
	errout.Reset()
	if code := execute(context.Background(), []string{"project", "local", "set", "--repository", repository, "--executable", "relative/prifly"}, &out, &errout); code == 0 || !strings.Contains(errout.String(), "project_local_executable_relative") {
		t.Fatalf("a relative path was accepted: %d %s", code, errout.String())
	}
	empty := t.TempDir()
	if output, err := exec.Command("git", "init", "-q", empty).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	out.Reset()
	errout.Reset()
	if code := execute(context.Background(), []string{"project", "local", "set", "--repository", empty, "--executable", target}, &out, &errout); code == 0 || !strings.Contains(errout.String(), "project_local_missing") {
		t.Fatalf("a project without local.yaml was edited: %d %s", code, errout.String())
	}
}

// A mistyped --project and a missing Run are different problems. Reported as
// one code they send the reader looking for an object that was never at fault.
func TestMissingAuthorityIsDistinctFromMissingObject(t *testing.T) {
	authority := t.TempDir()
	var out, errout bytes.Buffer
	if code := execute(context.Background(), []string{"init", authority}, &out, &errout); code != 0 {
		t.Fatalf("init: %d %s", code, errout.String())
	}
	for _, test := range []struct{ name, project, code string }{
		{"nonexistent path", filepath.Join(t.TempDir(), "absent"), "authority_not_found"},
		{"directory without an authority", t.TempDir(), "authority_not_found"},
		{"authority without that run", authority, "not_found"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out, errout bytes.Buffer
			code := execute(context.Background(), []string{"--json", "--project", test.project, "run", "status", "run:absent"}, &out, &errout)
			if code == 0 {
				t.Fatal("the read was accepted")
			}
			var problem prifly.Problem
			if err := json.Unmarshal(errout.Bytes(), &problem); err != nil {
				t.Fatal(err)
			}
			if problem.Code != test.code {
				t.Fatalf("got %s, want %s: %s", problem.Code, test.code, problem.Message)
			}
			if test.code == "authority_not_found" && !slices.Contains(problem.SafeNextActions, "init") {
				t.Fatalf("the refusal does not point at creating one: %+v", problem.SafeNextActions)
			}
		})
	}
}

// An author writes YAML, not wire messages, and looks for its form in the same
// place. The binary carries the authoring schemas because an installed tool has
// no repository to read them from.
func TestAuthoringDocumentsAreServedAndMatchTheDistributedFiles(t *testing.T) {
	var out, errout bytes.Buffer
	if code := execute(context.Background(), []string{"--json", "schema"}, &out, &errout); code != 0 {
		t.Fatalf("listing was refused: %s", errout.String())
	}
	var index struct {
		Contracts []string `json:"contracts"`
	}
	if err := json.Unmarshal(out.Bytes(), &index); err != nil {
		t.Fatal(err)
	}
	documents, err := prifly.AuthoringSchemaNames()
	if err != nil {
		t.Fatal(err)
	}
	if len(documents) == 0 || !slices.Contains(documents, "extension-v1") {
		t.Fatalf("the authoring set is empty or lost its extension document: %v", documents)
	}
	for _, document := range documents {
		if !slices.Contains(index.Contracts, document) {
			t.Fatalf("the index omits the authoring document %s", document)
		}
		served, err := contractSchema(document)
		if err != nil {
			t.Fatalf("a listed authoring document does not resolve: %s %v", document, err)
		}
		distributed, err := os.ReadFile(filepath.Join("..", "..", "schemas", "authoring", document+".schema.json"))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(served, distributed) {
			t.Fatalf("the embedded %s differs from the distributed file", document)
		}
	}
}

// A refusal that only says the path is unsafe leaves an author driving whole
// Runs to check a folder. It names the command that checks one instead.
func TestPathOutsideTheAuthorityNamesTheAuthoringCheck(t *testing.T) {
	authority := t.TempDir()
	var out, errout bytes.Buffer
	if code := execute(context.Background(), []string{"init", authority}, &out, &errout); code != 0 {
		t.Fatalf("init: %s", errout.String())
	}
	outside := filepath.Join(t.TempDir(), "workflow.yaml")
	if err := os.WriteFile(outside, []byte("authoring: prifly-workflow/1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errout.Reset()
	if code := execute(context.Background(), []string{"--json", "--project", authority, "validate", "--workflow", outside}, &out, &errout); code == 0 {
		t.Fatal("a path outside the authority was read")
	}
	if !strings.Contains(errout.String(), "project compile") {
		t.Fatalf("the refusal does not name the authoring check: %s", errout.String())
	}
}

// The help points at the worked authoring references, which is where the form
// of an extend.yaml actually lives.
func TestHelpNamesTheAuthoringReferences(t *testing.T) {
	var out, errout bytes.Buffer
	if code := execute(context.Background(), []string{"help", "schema"}, &out, &errout); code != 0 {
		t.Fatalf("help: %s", errout.String())
	}
	for _, expected := range []string{"examples/authoring/", "extension-authoring-reference.yaml"} {
		if !strings.Contains(out.String(), expected) {
			t.Fatalf("the help does not name %s: %s", expected, out.String())
		}
	}
}

// An extension names components by their short folder name, while every
// component file carries a full id inside it. Taking that id is the natural
// guess, so the refusal lists the names that would have worked.
func TestExtensionRefusalsListTheKnownNames(t *testing.T) {
	for _, expected := range []string{
		"project_extension_unknown_workflow",
		"project_extension_unknown_step",
		"(known: ",
		"belongs in a workflow graph you write yourself",
	} {
		found := false
		for _, source := range []string{"project.go", "project_compile.go"} {
			body, err := os.ReadFile(filepath.Join(source))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(body), expected) {
				found = true
			}
		}
		if !found {
			t.Fatalf("no extension refusal carries %q", expected)
		}
	}
}

// The declared questions can be answered up front, and the flag that does it
// was absent from the help: seven plausible names were tried before asking.
func TestHelpNamesTheQuestionnaireFlags(t *testing.T) {
	var out, errout bytes.Buffer
	if code := execute(context.Background(), []string{"help", "project", "start"}, &out, &errout); code != 0 {
		t.Fatalf("help: %s", errout.String())
	}
	for _, flag := range []string{"--preflight-answer", "--package-profile", "--decision-policy", "--expected-decision-catalog-digest"} {
		if !strings.Contains(out.String(), flag) {
			t.Fatalf("the help does not name %s: %s", flag, out.String())
		}
	}
}

// The launch path resolves the package it has just sealed. Reported as a bare
// not_found, a package that is absent, untrusted or byte-different reads as a
// missing file and sends the reader looking anywhere but at the package.
func TestSealedPackageLookupNamesThePackage(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("project_start.go"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	for _, expected := range []string{
		"project_start_package_not_installed",
		"was not found among trusted packages",
		"read package list",
	} {
		if !strings.Contains(source, expected) {
			t.Fatalf("the sealed-package lookup does not carry %q", expected)
		}
	}
	if strings.Contains(source, "return \"\", local.ErrNotFound") {
		t.Fatal("the sealed-package lookup still refuses without naming its subject")
	}
}

// A declared package whose folder is missing is already named. This pins that,
// because the same run reported both refusals as one nameless not_found.
func TestMissingPackageFolderIsNamed(t *testing.T) {
	repository := t.TempDir()
	if output, err := exec.Command("git", "init", "-q", repository).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	authority := t.TempDir()
	var out, errout bytes.Buffer
	if code := execute(context.Background(), []string{"project", "init", "--repository", repository, "--state-root", authority}, &out, &errout); code != 0 {
		t.Fatalf("project init: %s", errout.String())
	}
	profile := filepath.Join(repository, ".prifly", "project.yaml")
	body, err := os.ReadFile(profile)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(body), "packages: {}\n", "packages:\n  absent:\n    source: .prifly/workflows/absent\n", 1)
	if updated == string(body) {
		t.Fatal("the generated profile no longer declares an empty package map")
	}
	if err := os.WriteFile(profile, []byte(updated), 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errout.Reset()
	if code := execute(context.Background(), []string{"--json", "--project", authority, "project", "questionnaire", "--repository", repository, "--package", "absent"}, &out, &errout); code == 0 {
		t.Fatal("a package with no folder was read")
	}
	if !strings.Contains(errout.String(), "package source does not exist") {
		t.Fatalf("the refusal does not name the missing folder: %s", errout.String())
	}
}

// The text summary is what a reader gets by default, and it withheld the one
// fact a worker comes for: whether its result was accepted.
func TestRunSummaryPrintsStepVerdicts(t *testing.T) {
	var out bytes.Buffer
	view := prifly.RunView{}
	view.Run.ID = "run:example"
	view.Run.Status = "running"
	view.Run.Diagnostics = []prifly.Diagnostic{}
	view.Run.Outputs = map[string]prifly.ArtifactRef{}
	view.Run.Steps = map[string]*prifly.Step{
		"step:two":   {ID: "step:two", Status: "completed", Verdict: "needs_revision", Outputs: map[string]prifly.ArtifactRef{}},
		"step:one":   {ID: "step:one", Status: "completed", Verdict: "pass", Outputs: map[string]prifly.ArtifactRef{"plan": {}}},
		"step:three": {ID: "step:three", Status: "running"},
	}
	if err := renderRun(&out, view); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if !strings.Contains(text, `step "step:one" status=completed verdict=pass outputs=1`) {
		t.Fatalf("the summary hides an accepted verdict: %s", text)
	}
	if !strings.Contains(text, `verdict=needs_revision`) {
		t.Fatalf("the summary hides a non-passing verdict: %s", text)
	}
	if strings.Contains(text, "step:three") {
		t.Fatalf("the summary invented a verdict for a running step: %s", text)
	}
	if strings.Index(text, "step:one") > strings.Index(text, "step:two") {
		t.Fatalf("the summary printed steps in map order: %s", text)
	}
}

// A run that failed printed only how many diagnostics it had. The reader had to
// go to the JSON to learn what went wrong, so the summary now names the cause.
func TestRunSummaryPrintsDiagnosticCauses(t *testing.T) {
	var out bytes.Buffer
	view := prifly.RunView{}
	view.Run.ID = "run:example"
	view.Run.Status = "failed"
	view.Run.Outputs = map[string]prifly.ArtifactRef{}
	view.Run.Diagnostics = []prifly.Diagnostic{
		{Code: "authority_output_sealing_failed", Severity: "error", Phase: "settlement", Message: "Executor or result validation failed; inspect recorded evidence: mkdir artifacts/ab: permission denied"},
		{Code: "context_pinned", Severity: "info", Phase: "admission", Message: "not an error"},
	}
	if err := renderRun(&out, view); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if !strings.Contains(text, `diagnostic "authority_output_sealing_failed" phase=settlement`) {
		t.Fatalf("the summary hides the failure: %s", text)
	}
	if !strings.Contains(text, "mkdir artifacts/ab: permission denied") {
		t.Fatalf("the summary hides the recorded cause: %s", text)
	}
	if strings.Contains(text, "not an error") {
		t.Fatalf("the summary printed a non-error diagnostic: %s", text)
	}
}

// Reading one form cost the whole bundle: for the assisted session contracts
// that is hundreds of kilobytes of context spent on a single field.
func TestSchemaSelectorReturnsOneDefinitionWithItsClosure(t *testing.T) {
	var full, selected, errout bytes.Buffer
	if code := execute(context.Background(), []string{"--json", "schema", "SessionSubmissionV5"}, &full, &errout); code != 0 {
		t.Fatal(errout.String())
	}
	if code := execute(context.Background(), []string{"--json", "schema", "SessionSubmissionV5", "--def", "runtime_SessionSubmission"}, &selected, &errout); code != 0 {
		t.Fatalf("the selector was refused: %s", errout.String())
	}
	if selected.Len() >= full.Len()/4 {
		t.Fatalf("the selector saved nothing: %d of %d bytes", selected.Len(), full.Len())
	}
	var answer struct {
		Ref  string                     `json:"$ref"`
		Defs map[string]json.RawMessage `json:"$defs"`
	}
	if err := json.Unmarshal(selected.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if answer.Ref != "#/$defs/runtime_SessionSubmission" {
		t.Fatalf("the selector answered for %s", answer.Ref)
	}
	// Every reference the answer makes must resolve inside the answer.
	for name, raw := range answer.Defs {
		for _, match := range contractReference.FindAllStringSubmatch(string(raw), -1) {
			if _, ok := answer.Defs[match[1]]; !ok {
				t.Fatalf("%s references %s, which the answer does not carry", name, match[1])
			}
		}
	}
	errout.Reset()
	selected.Reset()
	if code := execute(context.Background(), []string{"--json", "schema", "SessionSubmissionV5", "--def", "Absent"}, &selected, &errout); code == 0 || !strings.Contains(errout.String(), "declares no definition named Absent") {
		t.Fatalf("an unknown definition was accepted: %d %s", code, errout.String())
	}
}

// Which commands may write is a list of facts, and a command that opens the
// authority in the wrong mode either cannot do its work or holds a writer for
// a read. This pins both directions for the whole surface.
func TestEveryMutatingCommandOpensForWriting(t *testing.T) {
	for _, c := range []struct {
		args   []string
		writes bool
	}{
		{[]string{"run", "start"}, true},
		{[]string{"run", "drive"}, true},
		{[]string{"run", "waive"}, true},
		{[]string{"run", "resolve"}, true},
		{[]string{"run", "status"}, false},
		{[]string{"run", "explain"}, false},
		{[]string{"run", "decision", "run:1", "request"}, true},
		{[]string{"run", "decision", "run:1", "answer"}, true},
		{[]string{"run", "decision", "run:1", "show"}, false},
		{[]string{"control", "stop"}, true},
		{[]string{"control", "show"}, false},
		{[]string{"package", "import"}, true},
		{[]string{"package", "list"}, false},
		{[]string{"claim", "create"}, true},
		{[]string{"claim", "list"}, false},
		{[]string{"session", "submit"}, true},
		{[]string{"session", "task"}, false},
		{[]string{"action", "propose"}, true},
		{[]string{"action", "list"}, false},
		{[]string{"approval", "request"}, true},
		{[]string{"approval", "list"}, false},
		{[]string{"grant", "issue"}, true},
		{[]string{"grant", "list"}, false},
		{[]string{"capacity", "set"}, true},
		{[]string{"capacity", "show"}, false},
		{[]string{"artifact", "import"}, true},
		{[]string{"artifact", "show"}, false},
		{[]string{"source", "import"}, true},
		{[]string{"task", "prepare"}, true},
		{[]string{"task", "list"}, false},
		{[]string{"doctor"}, false},
		{[]string{"telemetry"}, false},
	} {
		if got := mutates(c.args); got != c.writes {
			t.Fatalf("%v opens for writing: %t, want %t", c.args, got, c.writes)
		}
	}
}
