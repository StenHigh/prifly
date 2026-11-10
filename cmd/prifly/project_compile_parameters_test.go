package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	prifly "github.com/stenhigh/prifly/internal/runtime"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const compileParameterFolder = ".prifly/workflows/limits/"
const compileParameterDeclaration = `authoring: prifly-compile-parameters/1
parameters:
  task_description_limit:
    type: integer
    minimum: 1
    maximum: 1000000
    default: 12000
    bindings:
      - {schema: schema_task, pointer: /properties/description/maxLength}
`

func compileParameterFixture(t *testing.T) (string, string) {
	t.Helper()
	root, authority := t.TempDir(), filepath.Join(t.TempDir(), "authority")
	if code, _, errout := runCLI(t, "project", "init", "--repository", root, "--state-root", authority, "--host", "codex-cli"); code != 0 {
		t.Fatal(errout)
	}
	writeFixtureFile(t, root, ".prifly/project.yaml", `schema_version: prifly-project-profile/3
hosts: {codex-cli: .codex/skills}
packages: {limits: {source: .prifly/workflows/limits}}
launches:
  limits:
    title: Task limit
    description: Synthetic issue 22 fixture.
    kind: workflow
    workflow: .prifly/workflows/limits/workflow.yaml
`)
	writeFixtureFile(t, root, compileParameterFolder+"workflow.yaml", `authoring: prifly-project-workflow/1
package:
  id: test:package/limits
  version: 1.0.0
  description: Task limit fixture.
  requires_core_protocol: "1"
  references:
    policy: core:policy/local@3.0.0
    assisted: core:adapter/assisted-session@1.0.0
    result: core:schema/step-result@1.0.0
id: test:workflow/limits
version: 1.0.0
policy_ref: "{{policy}}"
inputs: {task: {schema_ref: "{{schema_task}}", required: true}}
entry: child
limits: {max_step_instances: 2, max_control_transitions: 8, max_child_depth: 1}
stages:
  child:
    kind: call
    workflow_ref: "{{workflow_child}}"
    input_bindings: {task: $inputs.task}
    on: {succeeded: done}
  done: {kind: finish, outcome: succeeded}
`)
	writeFixtureFile(t, root, compileParameterFolder+"workflows/child.yaml", `authoring: prifly-workflow/1
id: test:workflow/child
version: 1.0.0
policy_ref: "{{policy}}"
inputs: {task: {schema_ref: "{{schema_task}}", required: true}}
entry: worker
limits: {max_step_instances: 1, max_control_transitions: 3}
stages:
  worker:
    kind: step
    step_ref: "{{step_worker}}"
    input_bindings: {task: $inputs.task}
    on: {pass: done}
    impossible_verdicts: [fail, needs_revision, no_work]
  done: {kind: finish, outcome: succeeded}
`)
	writeFixtureFile(t, root, compileParameterFolder+"project/steps/worker.yaml", `authoring: prifly-step/1
id: test:step/worker
version: 1.0.0
kind: worker
executor: {adapter_ref: "{{assisted}}", operation: session}
instructions_ref: "{{context_worker}}"
inputs: {task: {schema_ref: "{{schema_task}}", required: true}}
effects: {class: none, retry_class: never}
result_schema_ref: "{{result}}"
`)
	writeFixtureFile(t, root, compileParameterFolder+"schemas/task.yaml", `id: test:schema/task
version: 1.0.0
type: object
required: [description]
additionalProperties: false
properties:
  description: {type: string, maxLength: "{{task_description_limit}}"}
`)
	writeFixtureFile(t, root, compileParameterFolder+"contexts/worker.yaml", "id: test:context/worker\nversion: 1.0.0\nmedia_type: text/markdown; charset=utf-8\ntext: Preserve the complete synthetic task.\n")
	writeFixtureFile(t, root, compileParameterFolder+"compile-parameters.yaml", compileParameterDeclaration)
	return root, authority
}

func setCompileParameter(t *testing.T, root, value string) {
	t.Helper()
	writeFixtureFile(t, root, compileParameterFolder+"project/compile-values.yaml", "authoring: prifly-compile-values/1\nvalues: {task_description_limit: "+value+"}\n")
}

func TestProjectCompileParameterRefusals(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, value, replace, replacement, code string }{
		{name: "string", value: `"32000"`, code: "project_compile_parameter_invalid"},
		{name: "fraction", value: "1.5", code: "project_compile_parameter_invalid"},
		{name: "zero", value: "0", code: "project_compile_parameter_invalid"},
		{name: "negative", value: "-1", code: "project_compile_parameter_invalid"},
		{name: "overflow", value: "1000001", code: "project_compile_parameter_invalid"},
		{name: "null", value: "null", code: "project_compile_parameter_invalid"},
		{name: "boolean", value: "true", code: "project_compile_parameter_invalid"},
		{name: "object", value: "{}", code: "project_compile_parameter_invalid"},
		{name: "array", value: "[]", code: "project_compile_parameter_invalid"},
		{name: "unsafe-integer", value: "9007199254740992", code: "project_compile_parameter_invalid"},
		{name: "unknown", replace: "task_description_limit:", replacement: "other:", value: "32000", code: "project_compile_parameter_unknown"},
		{name: "foreign-schema", replace: "schema_task", replacement: "schema_foreign", code: "project_compile_parameter_binding"},
		{name: "missing-pointer", replace: "/properties/description/maxLength", replacement: "/properties/missing/maxLength", code: "project_compile_parameter_binding"},
		{name: "other-keyword", replace: "/properties/description/maxLength", replacement: "/properties/description/type", code: "project_compile_parameter_binding"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root, authority := compileParameterFixture(t)
			if tc.value != "" {
				setCompileParameter(t, root, tc.value)
			}
			if tc.replace != "" {
				writeFixtureFile(t, root, compileParameterFolder+"compile-parameters.yaml", strings.ReplaceAll(compileParameterDeclaration, tc.replace, tc.replacement))
			}
			output := filepath.Join(t.TempDir(), "compiled")
			code, _, errout := runCLI(t, "--project", authority, "project", "compile", "--repository", root, "--package", "limits", "--output", output)
			if code == 0 || !strings.Contains(errout, tc.code) {
				t.Fatalf("expected %s, got exit %d: %s", tc.code, code, errout)
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatalf("failed compile left output: %v", err)
			}
		})
	}
}

func TestProjectCompileParameterDefaultAndOwner(t *testing.T) {
	t.Parallel()
	root, authority := compileParameterFixture(t)
	for _, limit := range []int{12000, 32000, 40000} {
		if limit != 12000 {
			setCompileParameter(t, root, fmt.Sprint(limit))
		}
		output := filepath.Join(t.TempDir(), "compiled")
		code, out, errout := runCLI(t, "--project", authority, "project", "compile", "--repository", root, "--package", "limits", "--output", output)
		if code != 0 {
			t.Fatalf("compile %d: %s", limit, errout)
		}
		var compiled projectCompileResult
		if err := json.Unmarshal([]byte(out), &compiled); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, component := range compiled.Components {
			if component.Kind != "schema" {
				continue
			}
			data, err := os.ReadFile(filepath.Join(output, component.Path))
			if err != nil {
				t.Fatal(err)
			}
			var schema struct {
				Properties map[string]struct {
					MaxLength int `json:"maxLength"`
				} `json:"properties"`
			}
			if err := json.Unmarshal(data, &schema); err != nil {
				t.Fatal(err)
			}
			if schema.Properties["description"].MaxLength != limit {
				t.Fatalf("effective limit differs: %s", data)
			}
			found = true
		}
		if !found {
			t.Fatal("task schema absent")
		}
	}
}

func compileParameterCommand(t *testing.T, args ...string) string {
	t.Helper()
	code, out, errout := runCLI(t, args...)
	if code != 0 {
		t.Fatalf("%v: exit %d: %s", args, code, errout)
	}
	return out
}

func compileParameterReview(t *testing.T, root, task string) projectLaunchSummary {
	t.Helper()
	out := compileParameterCommand(t, "project", "questionnaire", "--prepare", "--repository", root, "--launch", "limits", "--host", "codex-cli", "--input", "task="+task)
	var summary projectLaunchSummary
	if err := json.Unmarshal([]byte(out), &summary); err != nil {
		t.Fatal(err)
	}
	return summary
}

func compileParameterTaskFile(t *testing.T, length int) string {
	t.Helper()
	// Each rune is one JSON Schema character. Emoji use multiple UTF-8 bytes.
	description := strings.Repeat("я😀", length/2) + strings.Repeat("я", length%2)
	data, err := json.Marshal(map[string]string{"description": description})
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "task.json")
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestProjectCompileParameterPrepareStartUnicode(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		limit, length int
		valid         bool
	}{
		{12000, 12000, true}, {12000, 12001, false}, {32000, 16557, true}, {32000, 32000, true}, {32000, 32001, false},
	} {
		t.Run(fmt.Sprintf("%d-%d", tc.limit, tc.length), func(t *testing.T) {
			t.Parallel()
			root, authority := compileParameterFixture(t)
			if tc.limit != 12000 {
				setCompileParameter(t, root, fmt.Sprint(tc.limit))
			}
			taskFile := compileParameterTaskFile(t, tc.length)
			args := []string{"--repository", root, "--launch", "limits", "--host", "codex-cli", "--input", "task=" + taskFile}
			code, out, errout := runCLI(t, append([]string{"project", "questionnaire", "--prepare"}, args...)...)
			if !tc.valid {
				assert := func(code int, errout string) {
					t.Helper()
					for _, want := range []string{"schema_invalid", "/description", fmt.Sprint(tc.limit), fmt.Sprint(tc.length)} {
						if code == 0 || !strings.Contains(errout, want) {
							t.Fatalf("missing %s: exit %d %s", want, code, errout)
						}
					}
				}
				assert(code, errout)
				code, _, errout = runCLI(t, append([]string{"project", "start"}, args...)...)
				assert(code, errout)
				engine, err := prifly.Open(authority, true)
				if err != nil {
					t.Fatal(err)
				}
				defer engine.Close()
				runs, err := engine.Runs(context.Background())
				if err != nil || len(runs) != 0 {
					t.Fatalf("refusal created Run: %v %v", runs, err)
				}
				return
			}
			if code != 0 {
				t.Fatal(errout)
			}
			var summary projectLaunchSummary
			if err := json.Unmarshal([]byte(out), &summary); err != nil {
				t.Fatal(err)
			}
			engine, err := prifly.Open(authority, true)
			if err != nil {
				t.Fatal(err)
			}
			packages, err := engine.Packages(context.Background())
			if err != nil || len(packages.Packages) != 0 {
				t.Fatalf("prepare imported: %v %v", packages, err)
			}
			runs, err := engine.Runs(context.Background())
			if err != nil || len(runs) != 0 {
				t.Fatalf("prepare created Run: %v %v", runs, err)
			}
			engine.Close()
			startedBytes := compileParameterCommand(t, append(append([]string{"project", "start"}, args...), "--expected-launch-digest", summary.ReviewDigest)...)
			var started projectStartResult
			if err := json.Unmarshal([]byte(startedBytes), &started); err != nil {
				t.Fatal(err)
			}
			run := started.Run.Run.ID
			compileParameterCommand(t, "--project", authority, "run", "drive", run)
			var task prifly.SessionTask
			if err := json.Unmarshal([]byte(compileParameterCommand(t, "--project", authority, "session", "task", "--run", run)), &task); err != nil {
				t.Fatal(err)
			}
			input, ok := task.Context.Inputs["task"]
			if !ok {
				t.Fatal("project-owned worker lost task input")
			}
			delivered, err := os.ReadFile(filepath.Join(task.Workspace, input.Path))
			if err != nil {
				t.Fatal(err)
			}
			original, err := os.ReadFile(taskFile)
			if err != nil {
				t.Fatal(err)
			}
			var a, b any
			if json.Unmarshal(original, &a) != nil || json.Unmarshal(delivered, &b) != nil {
				t.Fatal("invalid task JSON")
			}
			canonicalA, _ := json.Marshal(a)
			canonicalB, _ := json.Marshal(b)
			if !bytes.Equal(canonicalA, canonicalB) {
				t.Fatal("root/child/worker truncated task")
			}
			// A real accepted worker report proves the long value survives its call.
			result, _ := json.Marshal(map[string]any{"schema_version": "1", "run_id": run, "step_instance_id": task.StepInstanceID, "attempt_id": task.AttemptID, "envelope_digest": task.EnvelopeDigest, "verdict": "pass", "outputs": map[string]any{}, "evidence_refs": []any{}, "effect_receipt_refs": []any{}, "summary": "Synthetic Unicode task preserved"})
			submission, _ := json.Marshal(prifly.SessionSubmission{SchemaVersion: task.SchemaVersion, RunID: run, AttemptID: task.AttemptID, EnvelopeDigest: task.EnvelopeDigest, Result: result, AnsweredQuestions: noQuestions(task)})
			report := filepath.Join(t.TempDir(), "report.json")
			if err := os.WriteFile(report, submission, 0600); err != nil {
				t.Fatal(err)
			}
			compileParameterCommand(t, "--project", authority, "session", "submit", "--file", report)
			compileParameterCommand(t, "--project", authority, "run", "drive", run)
			engine, err = prifly.Open(authority, true)
			if err != nil {
				t.Fatal(err)
			}
			defer engine.Close()
			view, err := engine.View(context.Background(), run)
			if err != nil || view.Run.Status != "completed" || (view.Run.Outcome == nil || *view.Run.Outcome != "succeeded") {
				t.Fatalf("long task Run did not succeed: %v %v", view.Run.Status, err)
			}
		})
	}
}

func TestProjectCompileParameterStaleReview(t *testing.T) {
	t.Parallel()
	root, authority := compileParameterFixture(t)
	setCompileParameter(t, root, "32000")
	task := compileParameterTaskFile(t, 16557)
	before := compileParameterReview(t, root, task)
	setCompileParameter(t, root, "40000")
	after := compileParameterReview(t, root, task)
	if before.BuildKey == after.BuildKey || before.ReviewDigest == after.ReviewDigest {
		t.Fatal("owner value absent from build/review identity")
	}
	code, _, errout := runCLI(t, "project", "start", "--repository", root, "--launch", "limits", "--host", "codex-cli", "--input", "task="+task, "--expected-launch-digest", before.ReviewDigest)
	if code == 0 || !strings.Contains(errout, "project_start_stale_launch") {
		t.Fatalf("stale review accepted: %d %s", code, errout)
	}
	engine, err := prifly.Open(authority, true)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	runs, err := engine.Runs(context.Background())
	if err != nil || len(runs) != 0 {
		t.Fatalf("stale review created Run: %v %v", runs, err)
	}
}

func compileParameterStart(t *testing.T, root, authority, task string) projectStartResult {
	t.Helper()
	args := []string{"--project", authority, "project", "questionnaire", "--prepare", "--repository", root, "--launch", "limits", "--host", "codex-cli", "--input", "task=" + task}
	var summary projectLaunchSummary
	if err := json.Unmarshal([]byte(compileParameterCommand(t, args...)), &summary); err != nil {
		t.Fatal(err)
	}
	args[3] = "start"
	args = append(args[:4], args[5:]...)
	args = append(args, "--expected-launch-digest", summary.ReviewDigest)
	var started projectStartResult
	if err := json.Unmarshal([]byte(compileParameterCommand(t, args...)), &started); err != nil {
		t.Fatal(err)
	}
	return started
}

func TestProjectCompileParameterSharedAuthorityAndPinnedRun(t *testing.T) {
	t.Parallel()
	root, authority := compileParameterFixture(t)
	second, _ := compileParameterFixture(t)
	for _, r := range []string{root, second} {
		file := filepath.Join(r, compileParameterFolder, "workflow.yaml")
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		writeFixtureFile(t, r, compileParameterFolder+"workflow.yaml", strings.Replace(string(data), "entry: child", "continuation:\n  from_workflows: [test:workflow/limits]\n  from_cancelled: true\n  inputs: {task: {source_input: task}}\nentry: child", 1))
	}
	setCompileParameter(t, root, "32000")
	setCompileParameter(t, second, "40000")
	compileParameterCommand(t, "--project", authority, "capacity", "set", "--capacity", "2", "--reason", "Issue 22 synthetic independent projects")
	task := compileParameterTaskFile(t, 16557)
	firstRun := compileParameterStart(t, root, authority, task)
	engine, err := prifly.Open(authority, true)
	if err != nil {
		t.Fatal(err)
	}
	before, err := engine.View(context.Background(), firstRun.RunID)
	if err != nil {
		t.Fatal(err)
	}
	_, registry, err := engine.Inventory()
	if err != nil {
		t.Fatal(err)
	}
	oldDefinition := append([]byte(nil), registry[before.Run.WorkflowRef]...)
	if len(oldDefinition) == 0 {
		t.Fatal("source workflow definition is absent from the registry")
	}
	_, oldTask, err := engine.Artifact(before.Run.Inputs["task"])
	if err != nil {
		t.Fatal(err)
	}
	engine.Close()
	secondRun := compileParameterStart(t, second, authority, task)
	if firstRun.Package == secondRun.Package || firstRun.BuildKey == secondRun.BuildKey {
		t.Fatal("different limits reused one definition identity")
	}
	setCompileParameter(t, root, "12000")
	engine, err = prifly.Open(authority, true)
	if err != nil {
		t.Fatal(err)
	}
	after, err := engine.View(context.Background(), firstRun.RunID)
	if err != nil {
		t.Fatal(err)
	}
	_, registry, err = engine.Inventory()
	if err != nil {
		t.Fatal(err)
	}
	_, savedTask, err := engine.Artifact(after.Run.Inputs["task"])
	if err != nil {
		t.Fatal(err)
	}
	if before.Run.WorkflowRef != after.Run.WorkflowRef || before.Run.LockRef != after.Run.LockRef || !bytes.Equal(oldDefinition, registry[after.Run.WorkflowRef]) || !bytes.Equal(oldTask, savedTask) {
		t.Fatal("new project build changed pinned Run definitions or task")
	}
	engine.Close()
	// Cancellation leaves a declared source-input continuation. Its new, narrower
	// contract must refuse the carried task before another Run or worker effect.
	compileParameterCommand(t, "--project", authority, "run", "cancel", firstRun.RunID, "--reason", "synthetic continuation check")
	code, _, errout := runCLI(t, "--project", authority, "project", "continue", "--prepare", "--repository", root, "--launch", "limits", "--host", "codex-cli", "--source-run", firstRun.RunID)
	if code == 0 || !strings.Contains(errout, "schema_invalid") || !strings.Contains(errout, "12000") || !strings.Contains(errout, "16557") || !strings.Contains(errout, "/description") {
		t.Fatalf("narrowed continuation did not refuse precisely: %d %s", code, errout)
	}
	engine, err = prifly.Open(authority, true)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	runs, err := engine.Runs(context.Background())
	if err != nil || len(runs) != 2 {
		t.Fatalf("continue refusal created Run: %v %v", runs, err)
	}
	compileParameterCommand(t, "--project", authority, "run", "cancel", secondRun.RunID, "--reason", "synthetic test cleanup")
}

func TestProjectCompileParameterUpdateKeepsOwnerValues(t *testing.T) {
	t.Parallel()
	fixture, _ := compileParameterFixture(t)
	source := t.TempDir()
	if err := os.CopyFS(filepath.Join(source, "flows/limits"), os.DirFS(filepath.Join(fixture, compileParameterFolder))); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(source, "flows/limits/steps"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(source, "flows/limits/project/steps/worker.yaml"), filepath.Join(source, "flows/limits/steps/worker.yaml")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(source, "flows/limits/project")); err != nil {
		t.Fatal(err)
	}
	gitFixture(t, source, "init", "-q", "-b", "main")
	gitFixture(t, source, "add", ".")
	gitFixture(t, source, "commit", "-qm", "declare compile parameter fixture")
	root, authority := compileParameterFixture(t)
	if err := os.RemoveAll(filepath.Join(root, compileParameterFolder)); err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, root, ".prifly/project.yaml", "schema_version: prifly-project-profile/3\nhosts: {codex-cli: .codex/skills}\npackages: {}\nlaunches: {}\n")
	compileParameterCommand(t, "project", "workflows", "add", source, "--path", "flows/limits", "--name", "limits", "--repository", root)
	setCompileParameter(t, root, "32000")
	ownerFile := filepath.Join(root, compileParameterFolder, projectCompileValuesFile)
	original, err := os.ReadFile(ownerFile)
	if err != nil {
		t.Fatal(err)
	}
	compileParameterCommand(t, "--project", authority, "project", "compile", "--repository", root, "--package", "limits", "--output", filepath.Join(t.TempDir(), "before"))
	writeFixtureFile(t, source, "flows/limits/compile-parameters.yaml", strings.Replace(compileParameterDeclaration, "maximum: 1000000", "maximum: 20000", 1))
	gitFixture(t, source, "add", ".")
	gitFixture(t, source, "commit", "-qm", "narrow compile parameter range")
	compileParameterCommand(t, "project", "workflows", "update", "limits", "--repository", root)
	kept, err := os.ReadFile(ownerFile)
	if err != nil || !bytes.Equal(original, kept) {
		t.Fatalf("update rewrote owner value: %q %v", kept, err)
	}
	code, _, errout := runCLI(t, "--project", authority, "project", "compile", "--repository", root, "--package", "limits", "--output", filepath.Join(t.TempDir(), "after"))
	if code == 0 || !strings.Contains(errout, "project_compile_parameter_invalid") || !strings.Contains(errout, "20000") {
		t.Fatalf("updated declaration silently ignored incompatible owner value: %d %s", code, errout)
	}
}

func TestProjectCompileParameterConsumerBindingMismatch(t *testing.T) {
	t.Parallel()
	root, authority := compileParameterFixture(t)
	setCompileParameter(t, root, "32000")
	writeFixtureFile(t, root, compileParameterFolder+"project/schemas/other.yaml", "id: test:schema/other\nversion: 1.0.0\ntype: object\n")
	file := filepath.Join(root, compileParameterFolder, "project/steps/worker.yaml")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, root, compileParameterFolder+"project/steps/worker.yaml", strings.ReplaceAll(string(data), "{{schema_task}}", "{{schema_other}}"))
	code, _, errout := runCLI(t, "--project", authority, "project", "compile", "--repository", root, "--package", "limits", "--output", filepath.Join(t.TempDir(), "compiled"))
	if code == 0 || !strings.Contains(errout, "binding") {
		t.Fatalf("incompatible project-owned consumer accepted: %d %s", code, errout)
	}
}

func TestProjectCompileParameterCompositionScope(t *testing.T) {
	t.Parallel()
	for _, marker := range []string{"prifly-project-workflow/2", "prifly-project-workflow/3"} {
		t.Run(marker, func(t *testing.T) {
			t.Parallel()
			root, authority := compileParameterFixture(t)
			library := filepath.Join(root, ".prifly/workflows/library")
			if err := os.CopyFS(library, os.DirFS(filepath.Join(root, compileParameterFolder))); err != nil {
				t.Fatal(err)
			}
			if err := filepath.WalkDir(library, func(path string, entry os.DirEntry, err error) error {
				if err != nil || entry.IsDir() {
					return err
				}
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				return os.WriteFile(path, []byte(strings.ReplaceAll(string(data), "test:", "library:")), 0600)
			}); err != nil {
				t.Fatal(err)
			}
			profile, err := os.ReadFile(filepath.Join(root, ".prifly/project.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			writeFixtureFile(t, root, ".prifly/project.yaml", strings.Replace(string(profile), "packages: {limits: {source: .prifly/workflows/limits}}", "packages:\n  limits: {source: .prifly/workflows/limits}\n  library: {source: .prifly/workflows/library}", 1))
			mainFile := filepath.Join(root, compileParameterFolder, "workflow.yaml")
			data, err := os.ReadFile(mainFile)
			if err != nil {
				t.Fatal(err)
			}
			main := strings.Replace(string(data), "prifly-project-workflow/1", marker, 1)
			main = strings.Replace(main, "entry: child", "imports: {library: library}\nrefs: {imported_child: library.child}\nentry: child", 1)
			main = strings.Replace(main, `workflow_ref: "{{workflow_child}}"`, `workflow_ref: imported_child`, 1)
			main = strings.Replace(main, "input_bindings: {task: $inputs.task}", "input_bindings: {}", 1)
			writeFixtureFile(t, root, compileParameterFolder+"workflow.yaml", main)
			data, err = os.ReadFile(filepath.Join(library, "workflow.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			provider := strings.Replace(string(data), "prifly-project-workflow/1", "prifly-project-workflow/2", 1) + "exports:\n  workflows: {child: \"{{workflow_child}}\"}\n"
			writeFixtureFile(t, library, "workflow.yaml", provider)
			data, err = os.ReadFile(filepath.Join(library, "workflows/child.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			writeFixtureFile(t, library, "workflows/child.yaml", strings.Replace(string(data), `schema_ref: "{{schema_task}}", required: true`, `schema_ref: "{{schema_task}}", required: true, configuration: {scope: project, default: {description: synthetic}}`, 1))
			setCompileParameter(t, root, "32000")
			writeFixtureFile(t, library, projectCompileValuesFile, "authoring: prifly-compile-values/1\nvalues: {task_description_limit: 40000}\n")
			output := filepath.Join(t.TempDir(), "compiled")
			out := compileParameterCommand(t, "--project", authority, "project", "compile", "--repository", root, "--package", "limits", "--output", output)
			var compiled projectCompileResult
			if err := json.Unmarshal([]byte(out), &compiled); err != nil {
				t.Fatal(err)
			}
			found := map[int]bool{}
			for _, component := range compiled.Components {
				if component.Kind != "schema" {
					continue
				}
				data, err := os.ReadFile(filepath.Join(output, component.Path))
				if err != nil {
					t.Fatal(err)
				}
				var schema struct {
					Properties map[string]struct {
						MaxLength int `json:"maxLength"`
					} `json:"properties"`
				}
				if err := json.Unmarshal(data, &schema); err != nil {
					t.Fatal(err)
				}
				found[schema.Properties["description"].MaxLength] = true
			}
			if len(found) != 2 || !found[32000] || !found[40000] {
				t.Fatalf("source-owner parameter names mixed: %v", found)
			}
			for _, path := range []string{compileParameterFolder + "compile-parameters.yaml", ".prifly/workflows/library/project/compile-values.yaml"} {
				present := false
				for _, source := range compiled.Sources {
					if source == path {
						present = true
					}
				}
				if !present {
					t.Fatalf("sidecar missing from source inventory: %s", path)
				}
			}
		})
	}
}

func TestProjectCompileParameterProtectsInstanceDataAndLegacy(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"instance-data", "unbound-placeholder", "profile-two", "flag-conflict", "symlink"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			root, authority := compileParameterFixture(t)
			want := "project_compile_parameter_binding"
			extra := []string{}
			switch scenario {
			case "instance-data":
				writeFixtureFile(t, root, compileParameterFolder+"compile-parameters.yaml", strings.Replace(compileParameterDeclaration, "/properties/description/maxLength", "/default/maxLength", 1))
				file := filepath.Join(root, compileParameterFolder, "schemas/task.yaml")
				data, err := os.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}
				writeFixtureFile(t, root, compileParameterFolder+"schemas/task.yaml", string(data)+"default: {type: string, maxLength: \"{{task_description_limit}}\"}\n")
			case "unbound-placeholder":
				writeFixtureFile(t, root, compileParameterFolder+"schemas/other.yaml", "id: test:schema/other\nversion: 1.0.0\ntype: string\nmaxLength: \"{{task_description_limit}}\"\n")
			case "profile-two":
				gitFixture(t, root, "init", "-q", "-b", "main")
				writeFixtureFile(t, root, ".prifly/project.yaml", "schema_version: prifly-project-profile/2\n"+projectHostsYAML+"packages: {limits: {source: .prifly/workflows/limits}}\nlaunches: {}\n")
				extra = []string{"--host", "codex-cli"}
				want = "project_compile_profile_required"
			case "flag-conflict":
				extra = []string{"--value", "task_description_limit=40000"}
				want = "project_compile_parameter_invalid"
			case "symlink":
				target := filepath.Join(t.TempDir(), "values.yaml")
				if err := os.WriteFile(target, []byte("authoring: prifly-compile-values/1\nvalues: {task_description_limit: 32000}\n"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, filepath.Join(root, compileParameterFolder, projectCompileValuesFile)); err != nil {
					t.Fatal(err)
				}
				want = "project_compile_parameter_invalid"
			}
			args := append([]string{"--project", authority, "project", "compile", "--repository", root, "--package", "limits", "--output", filepath.Join(t.TempDir(), "compiled")}, extra...)
			code, _, errout := runCLI(t, args...)
			if code == 0 || !strings.Contains(errout, want) {
				t.Fatalf("%s: expected %s: %d %s", scenario, want, code, errout)
			}
		})
	}
}
