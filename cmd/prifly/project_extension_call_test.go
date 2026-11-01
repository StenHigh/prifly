package main

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

const callExtensionSource = `authoring: prifly-extension/2
extensions:
  - id: review-after-build
    workflow: cycle
    between: {from: build, to: commit}
    call: review
    input_bindings: {handoff: $stages.build.handoff}
    on: {succeeded: commit, partial: rejected, rejected: rejected}
    on_error: rejected
`

const extensionReviewWorkflow = `authoring: prifly-workflow/1
id: test:workflow/review
version: 1.0.0
refs:
  tests: "{{step_tests}}"
  handoff: "{{schema_handoff}}"
  policy: "{{policy}}"
inputs:
  handoff: {schema_ref: handoff, required: true}
outputs: {}
allowed_outcomes: [succeeded, partial, rejected]
entry: assess
limits: {max_step_instances: 1, max_control_transitions: 4}
policy_ref: policy
stages:
  assess:
    kind: step
    step_ref: tests
    input_bindings: {handoff: $inputs.handoff}
    on: {pass: done, fail: failed, needs_revision: partial, no_work: failed}
    on_error: failed
  done: {kind: finish, outcome: succeeded}
  partial: {kind: finish, outcome: partial}
  failed: {kind: finish, outcome: rejected}
`

func writeCallExtensionFixture(t *testing.T, extend string) (string, string) {
	t.Helper()
	root, authority := t.TempDir(), filepath.Join(t.TempDir(), "authority")
	if code, _, errout := runCLI(t, "project", "init", "--repository", root, "--state-root", authority); code != 0 {
		t.Fatalf("init: %d %s", code, errout)
	}
	writeExtensionInputFixture(t, root, extend)
	workflow := strings.Replace(extensionInputWorkflow, "max_step_instances: 3, max_control_transitions: 8", "max_step_instances: 8, max_control_transitions: 32, max_child_depth: 4", 1)
	writeFixtureFile(t, root, ".prifly/workflows/cycle/workflow.yaml", workflow)
	writeFixtureFile(t, root, ".prifly/workflows/cycle/project/workflows/z-review.yaml", extensionReviewWorkflow)
	return root, authority
}

func compileCallExtensionFixture(t *testing.T, root, authority string) (projectCompileResult, string, *prifly.Problem) {
	t.Helper()
	output := filepath.Join(t.TempDir(), "compiled")
	args := []string{"--project", authority, "project", "compile", "--repository", root, "--package", "cycle", "--output", output}
	profile, _ := os.ReadFile(filepath.Join(root, ".prifly/project.yaml"))
	if strings.Contains(string(profile), "prifly-project-profile/2") {
		args = append(args, "--host", "codex-app")
	}
	code, out, errout := runCLI(t, args...)
	if code != 0 {
		var p prifly.Problem
		if err := json.Unmarshal([]byte(errout), &p); err != nil {
			t.Fatalf("Problem: %s", errout)
		}
		if _, err := os.Stat(filepath.Join(output, "prifly.package.json")); err == nil {
			t.Fatal("refusal left a valid partial package")
		}
		return projectCompileResult{}, output, &p
	}
	var c projectCompileResult
	if err := json.Unmarshal([]byte(out), &c); err != nil {
		t.Fatal(err)
	}
	return c, output, nil
}

func TestProjectCallExtensionSealsNativeCallee(t *testing.T) {
	t.Parallel()
	root, authority := writeCallExtensionFixture(t, callExtensionSource)
	compiled, output, p := compileCallExtensionFixture(t, root, authority)
	if p != nil {
		t.Fatalf("compile: %+v", p)
	}
	var child flow.Ref
	var parent flow.WorkflowRevision
	for _, c := range compiled.Components {
		if c.Kind != "workflow" {
			continue
		}
		if strings.HasSuffix(c.Ref.ID, "/review") {
			child = c.Ref
		}
		if strings.HasSuffix(c.Ref.ID, "/cycle") {
			data, err := os.ReadFile(filepath.Join(output, c.Path))
			if err != nil {
				t.Fatal(err)
			}
			if err = json.Unmarshal(data, &parent); err != nil {
				t.Fatal(err)
			}
		}
	}
	stage := parent.Definition.Stages["review-after-build"]
	if stage.Kind != "call" || stage.WorkflowRef != child {
		t.Fatalf("native call/ref: %+v child=%+v", stage, child)
	}
	if parent.Definition.Stages["build"].On["pass"] != "review-after-build" {
		t.Fatal("root route not spliced")
	}
}

func TestProjectCallExtensionParserDiscriminatesBranches(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, source, code string }{
		{"call", callExtensionSource, ""},
		{"empty bindings", strings.Replace(callExtensionSource, "{handoff: $stages.build.handoff}", "{}", 1), ""},
		{"many bindings", strings.Replace(callExtensionSource, "{handoff: $stages.build.handoff}", "{handoff: $stages.build.handoff, other: $stages.build.handoff}", 1), ""},
		{"unmarked", strings.TrimPrefix(callExtensionSource, "authoring: prifly-extension/2\n"), "project_extension_unsupported_authoring"},
		{"unknown marker", strings.Replace(callExtensionSource, "prifly-extension/2", "prifly-extension/999", 1), "project_extension_unsupported_authoring"},
		{"both", strings.Replace(callExtensionSource, "call: review", "call: review\n    step: tests", 1), "project_extension_invalid"},
		{"neither", strings.Replace(callExtensionSource, "    call: review\n", "", 1), "project_extension_invalid"},
		{"impossible verdict", callExtensionSource + "    impossible_verdicts: []\n", "project_extension_invalid"},
		{"deep expression", strings.Replace(callExtensionSource, "$stages.build.handoff", "$stages.build.handoff.deep", 1), "project_extension_invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseProjectWorkflowOptions([]byte(tc.source))
			if tc.code == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			var f *prifly.Fault
			if !errors.As(err, &f) || f.Code != tc.code {
				t.Fatalf("want typed %s, got %v", tc.code, err)
			}
		})
	}
}

func TestProjectCallExtensionRefusesUnresolvedTargetAndOutcomes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, target, source, code string }{
		{"unknown", "absent", callExtensionSource, "project_extension_unknown_call"},
		{"wrong kind", "tests", callExtensionSource, "project_extension_call_wrong_kind"},
		{"self", "cycle", callExtensionSource, "call_cycle"},
		{"missing outcome", "review", strings.Replace(callExtensionSource, ", partial: rejected", "", 1), "project_extension_call_outcome_unmapped"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root, authority := writeCallExtensionFixture(t, strings.Replace(tc.source, "call: review", "call: "+tc.target, 1))
			_, _, p := compileCallExtensionFixture(t, root, authority)
			if p == nil || p.Code != tc.code {
				t.Fatalf("want %s, got %+v", tc.code, p)
			}
		})
	}
}

func TestProjectCallExtensionNativeValidation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, from, to, code string }{
		{"unknown producer", "$stages.build.handoff", "$stages.absent.handoff", "project_extension_unknown_stage"},
		{"late producer", "$stages.build.handoff", "$stages.commit.handoff", "unavailable_output"},
		{"unknown input", "{handoff: $stages.build.handoff}", "{handoff: $stages.build.handoff, absent: $stages.build.handoff}", "unknown_port"},
		{"unknown output", "$stages.build.handoff", "$stages.build.absent", "unknown_port"},
		{"missing required", "{handoff: $stages.build.handoff}", "{}", "missing_binding"},
		{"extra outcome", "succeeded: commit", "no_work: commit, succeeded: commit", "invalid_outcome"},
		{"bad error target", "on_error: rejected", "on_error: absent", "project_extension_unknown_stage"},
		{"bad workflow scope", "workflow: cycle", "workflow: review", "project_extension_unknown_workflow"},
		{"stage collision", "id: review-after-build", "id: build", "project_extension_stage_conflict"},
		{"missing edge", "from: build, to: commit", "from: commit, to: build", "project_extension_route_missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root, authority := writeCallExtensionFixture(t, strings.Replace(callExtensionSource, tc.from, tc.to, 1))
			_, _, p := compileCallExtensionFixture(t, root, authority)
			if p == nil || p.Code != tc.code {
				t.Fatalf("want %s, got %+v", tc.code, p)
			}
			if tc.code == "unavailable_output" || tc.code == "unknown_port" {
				if len(p.Violations) == 0 {
					t.Fatalf("native path lost: %+v", p)
				}
			}
		})
	}
}

func TestProjectCallExtensionInventoryAndCycles(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, code string }{
		{"ambiguous", "project_extension_call_ambiguous"}, {"cycle", "call_cycle"}, {"missing dependency", "project_compile_unresolved_values"}, {"profile /2", "project_compile_profile_required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root, authority := writeCallExtensionFixture(t, callExtensionSource)
			switch tc.name {
			case "ambiguous":
				writeFixtureFile(t, root, ".prifly/workflows/cycle/workflows/a-review.yaml", strings.Replace(extensionReviewWorkflow, "test:workflow/review", "other:workflow/review", 1))
			case "cycle":
				child := strings.Replace(extensionReviewWorkflow, "refs:", "refs:\n  root: \"{{workflow_cycle}}\"", 1)
				begin := strings.Index(child, "  assess:\n")
				end := strings.Index(child, "  done:")
				child = child[:begin] + "  assess: {kind: repeat, body_workflow_ref: root, max_iterations: 1, initial_bindings: {}, next_bindings: {}, continue_on: [succeeded], on_complete: {succeeded: done, rejected: failed, partial: partial, no_work: partial}, on_limit: partial}\n" + child[end:]
				writeFixtureFile(t, root, ".prifly/workflows/cycle/project/workflows/z-review.yaml", child)
			case "missing dependency":
				writeFixtureFile(t, root, ".prifly/workflows/cycle/project/workflows/z-review.yaml", strings.Replace(extensionReviewWorkflow, "{{step_tests}}", "{{step_missing}}", 1))
			case "profile /2":
				gitFixture(t, root, "init", "-q")
				writeFixtureFile(t, root, ".prifly/project.yaml", "schema_version: prifly-project-profile/2\n"+projectHostsYAML+"packages:\n  cycle: {source: .prifly/workflows/cycle}\nlaunches: {}\n")
			}
			_, _, p := compileCallExtensionFixture(t, root, authority)
			if p == nil || p.Code != tc.code {
				t.Fatalf("want %s, got %+v", tc.code, p)
			}
		})
	}
}

func TestProjectCallExtensionTwoInstancesAndSourceOrder(t *testing.T) {
	t.Parallel()
	source := callExtensionSource + `  - id: second-review
    workflow: cycle
    between: {from: review-after-build, to: commit}
    call: review
    input_bindings: {handoff: $stages.build.handoff}
    on: {succeeded: commit, partial: rejected, rejected: rejected}
`
	root, authority := writeCallExtensionFixture(t, source)
	a, _, p := compileCallExtensionFixture(t, root, authority)
	if p != nil {
		t.Fatal(p)
	}
	old := filepath.Join(root, ".prifly/workflows/cycle/project/workflows/z-review.yaml")
	next := filepath.Join(root, ".prifly/workflows/cycle/project/workflows/a-review.yaml")
	if err := os.Rename(old, next); err != nil {
		t.Fatal(err)
	}
	b, output, p := compileCallExtensionFixture(t, root, authority)
	if p != nil {
		t.Fatal(p)
	}
	if a.BuildKey != b.BuildKey || a.Package != b.Package {
		t.Fatalf("filename selected build: %s %s", a.BuildKey, b.BuildKey)
	}
	for _, c := range b.Components {
		if c.Kind != "workflow" || !strings.HasSuffix(c.Ref.ID, "/cycle") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(output, c.Path))
		if err != nil {
			t.Fatal(err)
		}
		var w flow.WorkflowRevision
		if err = json.Unmarshal(data, &w); err != nil {
			t.Fatal(err)
		}
		first, second := w.Definition.Stages["review-after-build"], w.Definition.Stages["second-review"]
		if first.Kind != "call" || second.Kind != "call" || first.WorkflowRef != second.WorkflowRef || first.On["succeeded"] != "second-review" {
			t.Fatalf("two calls not independent: %+v %+v", first, second)
		}
	}
}

func TestProjectCallExtensionAuthoringEditions(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, source string
		valid        bool
	}{
		{"extension", callExtensionSource, true}, {"extension-v2", callExtensionSource, true}, {"extension-v1", callExtensionSource, false},
		{"extension", extensionInputExtend, true}, {"extension-v1", extensionInputExtend, true}, {"extension-v2", extensionInputExtend, false},
		{"extension", strings.Replace(callExtensionSource, "call: review", "call: review\n    step: tests", 1), false},
	} {
		t.Run(tc.name+tc.source[:8], func(t *testing.T) {
			t.Parallel()
			value, err := flow.Parse([]byte(tc.source), "yaml")
			if err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			err = validateAuthoringForTest(t, tc.name, data)
			if (err == nil) != tc.valid {
				t.Fatalf("accepted=%v expected=%v: %v", err == nil, tc.valid, err)
			}
		})
	}
}

func TestProjectCallExtensionExactSchemaAndUnavailableExports(t *testing.T) {
	t.Parallel()
	t.Run("exact schema", func(t *testing.T) {
		t.Parallel()
		root, authority := writeCallExtensionFixture(t, callExtensionSource)
		writeFixtureFile(t, root, ".prifly/workflows/cycle/schemas/other.yaml", "id: test:schema/other\nversion: 1.0.0\ntype: object\n")
		child := strings.Replace(extensionReviewWorkflow, "{{schema_handoff}}", "{{schema_other}}", 1)
		stepPath := filepath.Join(root, ".prifly/workflows/cycle/steps/tests.yaml")
		stepBytes, err := os.ReadFile(stepPath)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(stepPath, []byte(strings.ReplaceAll(string(stepBytes), "{{schema_handoff}}", "{{schema_other}}")), 0600); err != nil {
			t.Fatal(err)
		}
		writeFixtureFile(t, root, ".prifly/workflows/cycle/project/workflows/z-review.yaml", child)
		_, _, p := compileCallExtensionFixture(t, root, authority)
		if p == nil || p.Code != "port_type_mismatch" {
			t.Fatalf("exact schemas not enforced: %+v", p)
		}
	})
	for _, route := range []string{"partial", "on_error"} {
		t.Run(route, func(t *testing.T) {
			t.Parallel()
			first := strings.Replace(callExtensionSource, "partial: rejected", "partial: partial", 1)
			if route == "on_error" {
				first = strings.Replace(first, "on_error: rejected", "on_error: error", 1)
			}
			target := "partial"
			if route == "on_error" {
				target = "error"
			}
			source := first + `  - id: read-export
    workflow: cycle
    between: {from: review-after-build, to: ` + target + `}
    step: tests
    input_bindings: {handoff: $stages.review-after-build.handoff}
    on: {pass: ` + target + `, fail: ` + target + `, needs_revision: ` + target + `, no_work: ` + target + `}
`
			root, authority := writeCallExtensionFixture(t, source)
			path := filepath.Join(root, ".prifly/workflows/cycle/workflow.yaml")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			text := string(data) + "  partial: {kind: finish, outcome: partial}\n"
			if route == "on_error" {
				text += "  error: {kind: finish, outcome: rejected}\n"
			}
			writeFixtureFile(t, root, ".prifly/workflows/cycle/workflow.yaml", text)
			child := strings.Replace(extensionReviewWorkflow, "outputs: {}", "outputs:\n  handoff: {schema_ref: handoff, required_for: [succeeded]}", 1)
			child = strings.Replace(child, "done: {kind: finish, outcome: succeeded}", "done: {kind: finish, outcome: succeeded, output_bindings: {handoff: $inputs.handoff}}", 1)
			writeFixtureFile(t, root, ".prifly/workflows/cycle/project/workflows/z-review.yaml", child)
			_, _, p := compileCallExtensionFixture(t, root, authority)
			if p == nil || p.Code != "unavailable_output" {
				t.Fatalf("unguaranteed %s export accepted: %+v", route, p)
			}
		})
	}
}

func TestProjectCallExtensionProgramBindingsStayInClosure(t *testing.T) {
	t.Parallel()
	const config = `{executable: shell, args: [worker.sh], files: {worker.sh: project/worker.sh}, timeout_ms: 30000, grace_ms: 100, max_output_bytes: 65536}`
	for _, tc := range []struct{ name, target, code string }{
		{"child program", "tests", ""}, {"outside child", "build", "project_extension_invalid"}, {"callee is not a step", "review", "project_extension_invalid"}, {"package override", "tests", "project_execution_invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			source := callExtensionSource + "references:\n  process: core:adapter/local-process@2.0.0\nexecution_bindings:\n  steps:\n    " + tc.target + ": " + config + "\n"
			root, authority := writeCallExtensionFixture(t, source)
			path := filepath.Join(root, ".prifly/workflows/cycle/steps/tests.yaml")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			writeFixtureFile(t, root, ".prifly/workflows/cycle/steps/tests.yaml", strings.Replace(string(data), `executor: {adapter_ref: "{{assisted}}", operation: session}`, `executor: {adapter_ref: "{{process}}", operation: process}`, 1))
			writeFixtureFile(t, root, ".prifly/workflows/cycle/project/worker.sh", "exit 0\n")
			if tc.name == "package override" {
				path = filepath.Join(root, ".prifly/workflows/cycle/workflow.yaml")
				data, err = os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				writeFixtureFile(t, root, ".prifly/workflows/cycle/workflow.yaml", string(data)+"execution_bindings:\n  steps:\n    test:step/tests: "+config+"\n")
			}
			c, output, p := compileCallExtensionFixture(t, root, authority)
			if tc.code != "" {
				if p == nil || p.Code != tc.code {
					t.Fatalf("want %s got %+v", tc.code, p)
				}
				return
			}
			if p != nil {
				t.Fatal(p)
			}
			data, err = os.ReadFile(filepath.Join(output, projectExecutionFile))
			if err != nil {
				t.Fatal(err)
			}
			c.ExecutionBindings, err = projectDecodeExecution(data)
			if err != nil {
				t.Fatal(err)
			}
			if c.ExecutionBindings == nil || len(c.ExecutionBindings.Bindings) != 1 {
				t.Fatalf("no child program binding: %+v", c.ExecutionBindings)
			}
			bound := c.ExecutionBindings.Bindings[0]
			found := false
			for _, component := range c.Components {
				if component.Ref == bound.DefinitionRef {
					found = true
				}
			}
			if !found || string(bound.Files["project/worker.sh"]) != "exit 0\n" {
				t.Fatalf("binding not sealed to compiled child: %+v", bound)
			}
		})
	}
}

func TestProjectCallExtensionStandaloneRequiresPackage(t *testing.T) {
	t.Parallel()
	root, authority := writeCallExtensionFixture(t, callExtensionSource)
	compiled, output, p := compileCallExtensionFixture(t, root, authority)
	if p != nil {
		t.Fatal(p)
	}
	var source string
	for _, component := range compiled.Components {
		if component.Kind == "workflow" && strings.HasSuffix(component.Ref.ID, "/cycle") {
			source = filepath.Join(output, component.Path)
		}
	}
	target := filepath.Join(t.TempDir(), "refused.json")
	code, _, errout := runCLI(t, "project", "extend", "--workflow", source, "--workflow-id", "test:workflow/cycle", "--extensions", filepath.Join(root, ".prifly/workflows/cycle/extend.yaml"), "--output", target)
	var problem prifly.Problem
	if err := json.Unmarshal([]byte(errout), &problem); err != nil {
		t.Fatal(err)
	}
	if code == 0 || problem.Code != "project_extension_call_requires_package" {
		t.Fatal(code, errout)
	}
	if _, err := os.Stat(target); err == nil {
		t.Fatal("standalone refusal wrote a valid definition")
	}
}

func TestProjectCallExtensionStorageCeilingPreventsChildAdmission(t *testing.T) {
	t.Parallel()
	root, authority := newProjectFixture(t, 2<<20)
	fixture := filepath.Join("..", "..", "test", "fixtures", "project-call-extensions", "repository")
	if err := filepath.WalkDir(fixture, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(fixture, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		writeFixtureFile(t, root, relative, string(data))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	gitFixture(t, root, "add", ".")
	gitFixture(t, root, "commit", "-qm", "native call storage fixture")
	command := func(args ...string) string {
		t.Helper()
		code, out, errout := runCLI(t, append([]string{"--project", authority}, args...)...)
		if code != 0 {
			t.Fatalf("%v: %d %s", args, code, errout)
		}
		return out
	}
	args := []string{"--repository", root, "--launch", "quality", "--host", "codex-app"}
	var review projectLaunchSummary
	if err := json.Unmarshal([]byte(command(append([]string{"project", "questionnaire", "--prepare"}, args...)...)), &review); err != nil {
		t.Fatal(err)
	}
	var started projectStartResult
	if err := json.Unmarshal([]byte(command(append(append([]string{"project", "start"}, args...), "--expected-launch-digest", review.ReviewDigest)...)), &started); err != nil {
		t.Fatal(err)
	}
	run := started.RunID
	var task prifly.SessionTask
	if err := json.Unmarshal([]byte(command("session", "task", "--run", run)), &task); err != nil {
		t.Fatal(err)
	}
	command("session", "take", "--run", run, "--attempt", task.AttemptID)
	refs := map[string]prifly.ArtifactRef{}
	for name, slot := range task.Context.Outputs {
		writeFixtureFile(t, task.Workspace, slot.Path, "{}\n")
		refs[name] = prifly.ArtifactRef{ArtifactID: slot.ArtifactID, Revision: slot.Revision, Digest: projectBytesDigest([]byte("{}\n"))}
	}
	result, err := json.Marshal(map[string]any{"schema_version": "1", "run_id": run, "step_instance_id": task.StepInstanceID, "attempt_id": task.AttemptID, "envelope_digest": task.EnvelopeDigest, "verdict": "pass", "outputs": refs, "evidence_refs": []any{}, "effect_receipt_refs": []any{}, "summary": "accepted producer"})
	if err != nil {
		t.Fatal(err)
	}
	submission, err := json.Marshal(prifly.SessionSubmission{SchemaVersion: task.SchemaVersion, RunID: run, AttemptID: task.AttemptID, EnvelopeDigest: task.EnvelopeDigest, Result: result, AnsweredQuestions: noQuestions(task)})
	if err != nil {
		t.Fatal(err)
	}
	report := filepath.Join(t.TempDir(), "report.json")
	if err = os.WriteFile(report, submission, 0600); err != nil {
		t.Fatal(err)
	}
	command("session", "submit", "--file", report)
	engine, err := prifly.Open(authority, false)
	if err != nil {
		t.Fatal(err)
	}
	padding, _ := json.Marshal(map[string]string{"fixture_padding": strings.Repeat("p", (2<<20)+(64<<10))})
	mutation, err := engine.Store.ApplyAuthority(context.Background(), local.AuthorityCommand{ID: "command:fixture-call-saturation", Actor: "fixture:owner", Key: "fixture:call_padding", Payload: json.RawMessage(`{}`)}, func(local.AuthoritySnapshot) (local.AuthorityChange, error) {
		return local.AuthorityChange{Data: padding}, nil
	})
	if err != nil || mutation.Receipt.Rejection != nil {
		t.Fatal(err, mutation.Receipt.Rejection)
	}
	engine.Close()
	code, _, errout := runCLI(t, "--project", authority, "run", "drive", run)
	var problem prifly.Problem
	if err = json.Unmarshal([]byte(errout), &problem); err != nil {
		t.Fatal(errout)
	}
	if code != 5 || problem.Code != "storage_budget_exhausted" {
		t.Fatal(code, problem)
	}
	var view prifly.RunView
	if err = json.Unmarshal([]byte(command("run", "status", run)), &view); err != nil {
		t.Fatal(err)
	}
	if len(view.Run.Attempts) != 1 || view.Run.Attempts[task.AttemptID].Accepted == nil {
		t.Fatal("child dispatch bypassed storage ceiling or producer evidence changed")
	}
	engine, err = prifly.Open(authority, true)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := engine.Claims(context.Background())
	engine.Close()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, claim := range claims.Claims {
		if claim.ID == started.Workspace.ID {
			found = true
			if claim.Generation != started.Workspace.Generation {
				t.Fatal("storage refusal changed claim generation")
			}
		}
	}
	if !found {
		t.Fatal("storage refusal lost claim")
	}
}

func TestProjectCallExtensionDoesNotDiscoverInstalledCallee(t *testing.T) {
	t.Parallel()
	root, authority := writeCallExtensionFixture(t, callExtensionSource)
	_, output, p := compileCallExtensionFixture(t, root, authority)
	if p != nil {
		t.Fatal(p)
	}
	if code, _, errout := runCLI(t, "--project", authority, "package", "import", "--dir", output, "--reason", "explicit fixture trust"); code != 0 {
		t.Fatal(code, errout)
	}
	if err := os.Remove(filepath.Join(root, ".prifly/workflows/cycle/project/workflows/z-review.yaml")); err != nil {
		t.Fatal(err)
	}
	_, _, p = compileCallExtensionFixture(t, root, authority)
	if p == nil || p.Code != "project_extension_unknown_call" {
		t.Fatalf("installed but undeclared child was discovered: %+v", p)
	}
}
