package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// No host_skills source is needed here: admission must recognize an assisted
// executor itself, not rely on a missing skill file to reject a hostless launch.
func writeProjectLaunchRequirementsFixture(t *testing.T, root, version, effects string) {
	t.Helper()
	const folder = ".prifly/workflows/inspect/"
	writeFixtureFile(t, root, ".prifly/project.yaml", "schema_version: prifly-project-profile/"+version+"\n"+projectHostsYAML+`packages:
  inspect: {source: .prifly/workflows/inspect}
launches:
  inspect:
    title: Inspect a configured value
    description: Project launch admission fixture.
    kind: workflow
    workflow: .prifly/workflows/inspect/workflow.yaml
`)
	writeFixtureFile(t, root, folder+"schemas/value.yaml", "id: test:schema/value\nversion: 1.0.0\ntype: integer\nminimum: 1\nmaximum: 3\n")
	writeFixtureFile(t, root, folder+"contexts/inspect.yaml", "id: test:context/inspect\nversion: 1.0.0\nmedia_type: text/markdown; charset=utf-8\ntext: Inspect the supplied value.\n")
	writeFixtureFile(t, root, folder+"steps/inspect.yaml", `authoring: prifly-step/1
id: test:step/inspect
version: 1.0.0
kind: worker
refs: {value: "{{schema_value}}"}
inputs:
  value: {schema_ref: value, required: true}
executor: {adapter_ref: "{{assisted_adapter}}", operation: session}
instructions_ref: "{{context_inspect}}"
effects: {class: `+effects+`, retry_class: never}
result_schema_ref: "{{step_result_schema}}"
`)
	writeFixtureFile(t, root, folder+"workflow.yaml", `authoring: prifly-project-workflow/1
package:
  id: test:package/inspect
  version: 1.0.0
  description: Project launch admission fixture.
  requires_core_protocol: "1"
  references:
    assisted_adapter: core:adapter/assisted-session@1.0.0
    local_policy: core:policy/local@3.0.0
    step_result_schema: core:schema/step-result@1.0.0
id: test:workflow/inspect
version: 1.0.0
refs:
  inspect: "{{step_inspect}}"
  value: "{{schema_value}}"
  local_policy: "{{local_policy}}"
inputs:
  value:
    schema_ref: value
    required: true
    configuration: {scope: run, default: 3}
entry: inspect
limits: {max_step_instances: 1, max_control_transitions: 2}
policy_ref: local_policy
stages:
  inspect:
    kind: step
    step_ref: inspect
    input_bindings: {value: $inputs.value}
    on: {pass: done}
  done: {kind: finish, outcome: succeeded}
`)
	writeFixtureFile(t, root, folder+"extend.yaml", "extensions: []\n")
}

func TestCLIProjectConfigurableInputLaunch(t *testing.T) {
	for _, version := range []string{"2", "3"} {
		t.Run("profile-"+version, func(t *testing.T) {
			var root, authority string
			if version == "2" {
				root, authority = newProjectFixture(t)
			} else {
				root, authority = t.TempDir(), filepath.Join(t.TempDir(), "authority")
				if code, _, stderr := runCLI(t, "project", "init", "--repository", root, "--state-root", authority, "--host", "codex-cli"); code != 0 {
					t.Fatalf("init: %d %s", code, stderr)
				}
			}
			writeProjectLaunchRequirementsFixture(t, root, version, "none")
			args := []string{"--project", authority, "project", "start", "--repository", root, "--launch", "inspect", "--host", "codex-cli"}
			wantValue, wantSource := "3", "package_default"
			if version == "2" {
				gitFixture(t, root, "add", ".")
				gitFixture(t, root, "commit", "-qm", "declare legacy input fixture")
				requestDir := t.TempDir()
				writeFixtureFile(t, requestDir, "value.json", "2\n")
				writeFixtureFile(t, requestDir, "brief.json", `{"schema_version":"1","id":"test:brief/configuration","subject":"Inspect input","desired_outcome":"Read the chosen input","in_scope":["inspection"],"out_of_scope":[],"completion_criteria":["inspection handoff"],"source_refs":[],"assumptions":[],"confirmation":"explicit"}`)
				args = append(args, "--input", "value="+filepath.Join(requestDir, "value.json"), "--brief", filepath.Join(requestDir, "brief.json"), "--workspace", "checkout")
				wantValue, wantSource = "2", "run"
			}
			code, out, stderr := runCLI(t, args...)
			var started projectStartResult
			if code != 0 || json.Unmarshal([]byte(out), &started) != nil || started.Run.Run.ID == "" {
				t.Fatalf("configured launch: %d %s %s", code, out, stderr)
			}
			configuration := started.Run.Run.EffectiveConfiguration
			if configuration == nil || configuration.Inputs["value"].Source != wantSource || strings.TrimSpace(string(configuration.Inputs["value"].Value)) != wantValue {
				t.Fatalf("chosen configuration was lost: %+v", configuration)
			}
			code, out, stderr = runCLI(t, "--project", authority, "session", "task", "--run", started.Run.Run.ID)
			var task prifly.SessionTask
			if code != 0 || json.Unmarshal([]byte(out), &task) != nil || task.AttemptID == "" {
				t.Fatalf("configured input did not reach a real handoff: %d %s %s", code, out, stderr)
			}
			input, exists := task.Context.Inputs["value"]
			if !exists {
				t.Fatal("handoff omitted the configured input")
			}
			data, err := os.ReadFile(filepath.Join(task.Workspace, input.Path))
			if err != nil || strings.TrimSpace(string(data)) != wantValue {
				t.Fatalf("handoff received the wrong value: %q %v", data, err)
			}
			if version == "3" && (started.Workspace != nil || task.ClaimID != "") {
				t.Fatal("effects:none handoff claimed a Git workspace")
			}
		})
	}
}

func TestCLIProjectAssistedLaunchRequirementsBeforeMutation(t *testing.T) {
	for _, test := range []struct {
		name, effects, refusal string
		args                   []string
	}{
		{"host-required", "none", "project_start_host_required", nil},
		{"workspace-required", "workspace_write", "project_start_workspace_required", []string{"--host", "codex-cli"}},
		{"git-required", "workspace_write", "repository_required", []string{"--host", "codex-cli", "--workspace", "checkout"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			// No Git installation or repository: only a selected write workspace
			// may ask for Git, after host and workspace consent are present.
			t.Setenv("PATH", t.TempDir())
			root, authority := t.TempDir(), filepath.Join(t.TempDir(), "authority")
			if code, _, stderr := runCLI(t, "project", "init", "--repository", root, "--state-root", authority, "--host", "codex-cli"); code != 0 {
				t.Fatalf("init: %d %s", code, stderr)
			}
			writeProjectLaunchRequirementsFixture(t, root, "3", test.effects)
			args := append([]string{"--project", authority, "project", "start", "--repository", root, "--launch", "inspect"}, test.args...)
			code, out, stderr := runCLI(t, args...)
			if code == 0 || !strings.Contains(stderr, test.refusal) {
				t.Fatalf("expected %s: %d %s %s", test.refusal, code, out, stderr)
			}
			engine, err := prifly.Open(authority, true)
			if err != nil {
				t.Fatal(err)
			}
			defer engine.Close()
			packages, err := engine.Packages(context.Background())
			if err != nil || len(packages.Packages) != 0 {
				t.Fatalf("rejected launch registered packages: %+v %v", packages, err)
			}
			claims, err := engine.Claims(context.Background())
			if err != nil || len(claims.Claims) != 0 {
				t.Fatalf("rejected launch recorded a workspace claim: %+v %v", claims, err)
			}
			runs, err := engine.Runs(context.Background())
			if err != nil || len(runs) != 0 {
				t.Fatalf("rejected launch created Runs: %+v %v", runs, err)
			}
			if _, err := os.Lstat(filepath.Join(root, ".git")); !os.IsNotExist(err) {
				t.Fatalf("rejected launch changed the project into a repository: %v", err)
			}
		})
	}
}
