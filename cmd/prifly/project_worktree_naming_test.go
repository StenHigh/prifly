package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func TestProjectNamingProfileIsExplicitAndClosed(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, version, block string
		valid                bool
	}{
		{"v4", "4", "worktree_naming:\n  template: '{project}'\n  bindings:\n    project: {value: demo}\n", true},
		{"v4-without-naming", "4", "", true},
		{"v3", "3", "worktree_naming: {}\n", false},
		{"unknown-field", "4", "worktree_naming:\n  template: '{project}'\n  bindings: {project: {value: demo}}\n  root: /tmp\n", false},
		{"mixed-binding", "4", "worktree_naming:\n  template: '{project}'\n  bindings: {project: {value: demo, input: task, pointer: /id}}\n", false},
		{"unknown-placeholder", "4", "worktree_naming:\n  template: '{task}'\n  bindings: {project: {value: demo}}\n", false},
		{"repeated-placeholder", "4", "worktree_naming:\n  template: '{project}-{project}'\n  bindings: {project: {value: demo}}\n", true},
		{"extra-binding", "4", "worktree_naming:\n  template: '{project}'\n  bindings: {project: {value: demo}, unused: {value: unused}}\n", false},
		{"unclosed", "4", "worktree_naming:\n  template: '{project'\n  bindings: {project: {value: demo}}\n", false},
		{"shell-literal", "4", "worktree_naming:\n  template: '$(cmd)'\n  bindings: {}\n", false},
		{"null-binding", "4", "worktree_naming:\n  template: '{project}'\n  bindings: {project: {value: null}}\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeFixtureFile(t, root, ".prifly/project.yaml", "schema_version: prifly-project-profile/"+tc.version+"\npackages: {}\nlaunches: {}\n"+tc.block)
			_, err := readProjectProfile(root)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%t: %v", tc.valid, err)
			}
		})
	}
}

func TestProjectNamingValueAndPointerRefusalsBeforeMutation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, pointer, value, port, code string }{
		{"nested", "/a/id", `{"a":{"id":"safe"}}`, "value", ""},
		{"escaped", "/a~1b/~0id", `{"a/b":{"~id":"safe"}}`, "value", ""},
		{"empty", "", `"safe"`, "value", ""},
		{"absent", "/id", `{}`, "value", "project_worktree_value_missing"},
		{"null", "/id", `{"id":null}`, "value", "project_worktree_value_invalid"},
		{"number", "/id", `{"id":3}`, "value", "project_worktree_value_invalid"},
		{"traversal", "/id", `{"id":"../escape"}`, "value", "project_worktree_value_invalid"},
		{"uppercase", "/id", `{"id":"TASK"}`, "value", "project_worktree_value_invalid"},
		{"git-syntax", "/id", `{"id":"a@{b"}`, "value", "project_worktree_value_invalid"},
		{"unknown-port", "/id", `{"id":"safe"}`, "missing", "project_worktree_binding_invalid"},
		{"bad-pointer", "/id~2", `{"id":"safe"}`, "value", "project_worktree_binding_invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root, authority, args := writeNamedProjectFixture(t)
			writeFixtureFile(t, root, ".prifly/workflows/inspect/schemas/value.yaml", "id: test:schema/value\nversion: 1.0.0\n")
			profile, err := os.ReadFile(filepath.Join(root, ".prifly/project.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			block := fmt.Sprintf("worktree_naming:\n  template: '{id}-{id}'\n  bindings:\n    id: {input: %s, pointer: %q}\n", tc.port, tc.pointer)
			writeFixtureFile(t, root, ".prifly/project.yaml", strings.Split(string(profile), "worktree_naming:")[0]+block)
			input := filepath.Join(t.TempDir(), "input.json")
			if err := os.WriteFile(input, []byte(tc.value), 0600); err != nil {
				t.Fatal(err)
			}
			code, raw, stderr := runCLI(t, append(append([]string{"project", "questionnaire", "--prepare"}, args...), "--input", "value="+input)...)
			if tc.code != "" {
				if code == 0 || !strings.Contains(stderr, tc.code) {
					t.Fatalf("wanted %s: %d %s", tc.code, code, stderr)
				}
			} else {
				var result projectLaunchSummary
				if code != 0 || json.Unmarshal([]byte(raw), &result) != nil || result.WorktreeNaming.Base != "safe-safe" {
					t.Fatalf("resolved: %d %s %s", code, raw, stderr)
				}
			}
			e, err := prifly.Open(authority, true)
			if err != nil {
				t.Fatal(err)
			}
			defer e.Close()
			claims, err := e.Claims(context.Background())
			if err != nil || len(claims.Claims) != 0 {
				t.Fatal("naming refusal mutated claims", err)
			}
		})
	}
}

func TestNamedWorktreeCLIProcessHelper(t *testing.T) {
	t.Parallel()
	if os.Getenv("PRIFLY_NAMED_PROCESS_HELPER") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Exit(execute(context.Background(), os.Args[i+1:], os.Stdout, os.Stderr))
		}
	}
	t.Fatal("missing process arguments")
}

func TestNamedWorktreeConcurrentCLIProcessesFromPublishedExample(t *testing.T) {
	t.Parallel()
	root, authority := newProjectFixture(t)
	queueCLICommand(t, "--project", authority, "capacity", "set", "--capacity", "4", "--reason", "admit three independent naming fixture Runs")
	folder := filepath.Join(root, ".prifly/workflows/named")
	for _, name := range []string{"workflow.yaml", "schemas/task.yaml", "contexts/work.yaml", "steps/work.yaml", "task.json"} {
		data, err := os.ReadFile(filepath.Join("../../examples/workflows/named-worktree", name))
		if err != nil {
			t.Fatal(err)
		}
		writeFixtureFile(t, folder, name, string(data))
	}
	profile, err := os.ReadFile("../../examples/workflows/named-worktree/project.yaml")
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, root, ".prifly/project.yaml", string(profile))
	gitFixture(t, root, "add", ".")
	gitFixture(t, root, "commit", "-qm", "published named example")
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	call := func(args ...string) (string, error) {
		command := exec.Command(binary, append([]string{"-test.run=^TestNamedWorktreeCLIProcessHelper$", "--"}, append(args, "--json")...)...)
		command.Env = append(os.Environ(), "PRIFLY_NAMED_PROCESS_HELPER=1")
		var out, stderr bytes.Buffer
		command.Stdout, command.Stderr = &out, &stderr
		if err := command.Run(); err != nil {
			return "", fmt.Errorf("%v: %w: %s", args, err, stderr.String())
		}
		return out.String(), nil
	}
	args := []string{"--repository", root, "--launch", "named", "--host", "codex-cli", "--input", "task=" + filepath.Join(folder, "task.json")}
	raw, err := call(append([]string{"project", "questionnaire", "--prepare"}, args...)...)
	if err != nil {
		t.Fatal(err)
	}
	var review projectLaunchSummary
	if err := json.Unmarshal([]byte(raw), &review); err != nil {
		t.Fatal(err)
	}
	before := gitFixture(t, root, "status", "--porcelain")
	type response struct {
		raw string
		err error
	}
	results := make(chan response, 2)
	barrier := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-barrier
			raw, err := call(append(append([]string{"project", "start"}, args...), "--command-id", fmt.Sprintf("command:process-%d", i), "--expected-launch-digest", review.ReviewDigest)...)
			results <- response{raw, err}
		}(i)
	}
	close(barrier)
	wg.Wait()
	close(results)
	seen := map[string]bool{}
	for response := range results {
		if response.err != nil {
			t.Fatal(response.err)
		}
		validateQueueCLI(t, "ProjectStartV5", response.raw)
		var result projectStartResult
		json.Unmarshal([]byte(response.raw), &result)
		if result.Workspace == nil || seen[result.Workspace.Path] {
			t.Fatal("shared or missing worktree")
		}
		seen[result.Workspace.Path] = true
	}
	if len(seen) != 2 || gitFixture(t, root, "status", "--porcelain") != before {
		t.Fatal("concurrent start changed source checkout")
	}
	duplicateResults := make(chan response, 2)
	duplicateBarrier := make(chan struct{})
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-duplicateBarrier
			raw, err := call(append(append([]string{"project", "start"}, args...), "--command-id", "command:parallel-duplicate", "--expected-launch-digest", review.ReviewDigest)...)
			duplicateResults <- response{raw, err}
		}()
	}
	close(duplicateBarrier)
	wg.Wait()
	close(duplicateResults)
	original := ""
	for result := range duplicateResults {
		if result.err != nil {
			t.Fatal(result.err)
		}
		if original != "" && result.raw != original {
			t.Fatal("parallel duplicate returned a different result")
		}
		original = result.raw
	}
	engine, err := prifly.Open(authority, true)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	record, err := engine.Claims(context.Background())
	if err != nil || len(record.Claims) != 3 {
		t.Fatalf("claims: %+v %v", record, err)
	}
}

func writeNamedProjectFixture(t *testing.T) (string, string, []string) {
	t.Helper()
	root, authority := newProjectFixture(t)
	writeProjectLaunchRequirementsFixture(t, root, "4", "workspace_write")
	file := filepath.Join(root, ".prifly/project.yaml")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, root, ".prifly/project.yaml", string(data)+`worktree_naming:
  template: "{project_name}-{task_source}-{task_id}"
  bindings:
    project_name: {value: smsplace}
    task_source: {input: value, pointer: /source}
    task_id: {input: value, pointer: /id}
`)
	writeFixtureFile(t, root, ".prifly/workflows/inspect/schemas/value.yaml", "id: test:schema/value\nversion: 1.0.0\ntype: object\nadditionalProperties: false\nrequired: [source, id]\nproperties:\n  source: {type: string}\n  id: {type: string}\n")
	workflow := filepath.Join(root, ".prifly/workflows/inspect/workflow.yaml")
	data, err = os.ReadFile(workflow)
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, root, ".prifly/workflows/inspect/workflow.yaml", strings.Replace(string(data), "default: 3", "default: {source: trucker, id: default-123}", 1))
	gitFixture(t, root, "add", ".")
	gitFixture(t, root, "commit", "-qm", "declare named worktree workflow")
	return root, authority, []string{"--repository", root, "--launch", "inspect", "--host", "codex-cli", "--workspace", "worktree"}
}

func TestCLIProjectNamedWorktreePrepareStartAndFreshness(t *testing.T) {
	t.Parallel()
	root, authority, args := writeNamedProjectFixture(t)
	prepare := func() projectLaunchSummary {
		t.Helper()
		out := queueCLICommand(t, append([]string{"project", "questionnaire", "--prepare"}, args...)...)
		validateQueueCLI(t, "ProjectLaunchSummaryV5", out)
		var result projectLaunchSummary
		if err := json.Unmarshal([]byte(out), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	first := prepare()
	second := prepare()
	if first.ReviewDigest != second.ReviewDigest || first.WorktreeNamePreview == nil || first.WorktreeNamePreview.Reserved || first.WorktreeNaming.Base != "smsplace-trucker-default-123" {
		t.Fatalf("review: %+v", first)
	}
	engine, err := prifly.Open(authority, true)
	if err != nil {
		t.Fatal(err)
	}
	record, err := engine.Claims(context.Background())
	engine.Close()
	if err != nil || len(record.Claims) != 0 {
		t.Fatalf("prepare reserved claim: %+v %v", record, err)
	}
	start := func(id string) projectStartResult {
		t.Helper()
		out := queueCLICommand(t, append(append([]string{"project", "start"}, args...), "--command-id", id, "--expected-launch-digest", first.ReviewDigest)...)
		validateQueueCLI(t, "ProjectStartV5", out)
		var result projectStartResult
		if err := json.Unmarshal([]byte(out), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	started := start("command:named-project")
	if started.Workspace == nil || !strings.HasSuffix(started.Workspace.Path, "smsplace-trucker-default-123") || started.Workspace.Naming == nil {
		t.Fatalf("start: %+v", started.Workspace)
	}
	refFile := filepath.Join(t.TempDir(), "value-ref.json")
	refBytes, err := json.Marshal(started.Run.Run.Inputs["value"])
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(refFile, refBytes, 0600); err != nil {
		t.Fatal(err)
	}
	refReviewRaw := queueCLICommand(t, append(append([]string{"project", "questionnaire", "--prepare"}, args...), "--input-ref", "value="+refFile)...)
	var refReview projectLaunchSummary
	if err := json.Unmarshal([]byte(refReviewRaw), &refReview); err != nil || refReview.WorktreeNaming.Base != first.WorktreeNaming.Base {
		t.Fatalf("ArtifactRef resolution: %+v %v", refReview.WorktreeNaming, err)
	}
	repeated := start("command:named-project")
	if repeated.RunID != started.RunID || repeated.Workspace.ID != started.Workspace.ID || repeated.Workspace.Path != started.Workspace.Path {
		t.Fatalf("duplicate start: %+v", repeated.Workspace)
	}
	// The availability reading changes; the policy and its digest do not.
	third := prepare()
	if third.ReviewDigest != first.ReviewDigest || !strings.HasSuffix(third.WorktreeNamePreview.Path, "-02") {
		t.Fatalf("availability changed review: %+v", third.WorktreeNamePreview)
	}
	other := start("command:named-project-other")
	if !strings.HasSuffix(other.Workspace.Path, "-02") || other.Workspace.ID == started.Workspace.ID {
		t.Fatalf("independent start: %+v", other.Workspace)
	}
	profilePath := filepath.Join(root, ".prifly/project.yaml")
	data, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, root, ".prifly/project.yaml", strings.Replace(string(data), "value: smsplace", "value: changed", 1))
	code, _, stderr := runCLI(t, append(append([]string{"project", "start"}, args...), "--command-id", "command:stale-name", "--expected-launch-digest", first.ReviewDigest)...)
	if code == 0 || !strings.Contains(stderr, "project_start_stale_launch") {
		t.Fatalf("stale: %d %s", code, stderr)
	}
}

func TestCLINamedClaimResumeKeepsNamesAfterPolicyRemoval(t *testing.T) {
	t.Parallel()
	for _, queue := range []bool{false, true} {
		t.Run(fmt.Sprintf("queue-%t", queue), func(t *testing.T) {
			t.Parallel()
			configure := func(root string) {
				path := filepath.Join(root, ".prifly/project.yaml")
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				writeFixtureFile(t, root, ".prifly/project.yaml", strings.Replace(string(data), "prifly-project-profile/3", "prifly-project-profile/4", 1)+"worktree_naming:\n  template: '{project}'\n  bindings: {project: {value: resume-demo}}\n")
			}
			var f continuationCLI
			if queue {
				f = continuationCLIFixture(t, "  from_outcomes: [partial]\n", true, true, configure)
			} else {
				f = newContinuationCLIResuming(t, "  from_outcomes: [partial]\n", true, configure)
			}
			f.submit(map[string]string{"handoff": "{}\n"}, nil)
			writeFixtureFile(t, f.source.WorkspacePath, "plans/plan.md", "# Plan\n")
			f.submit(map[string]string{"draft": "{}\n"}, []prifly.WorkspaceTreeLocation{{OutputPort: "plan", Path: "plans/plan.md"}})
			f.submit(map[string]string{}, nil)
			path := filepath.Join(f.root, ".prifly/project.yaml")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			writeFixtureFile(t, f.root, ".prifly/project.yaml", strings.Split(string(data), "worktree_naming:")[0])
			args := []string{"--repository", f.root, "--launch", "source", "--host", "codex-cli", "--source-run", f.source.RunID}
			if queue {
				binding := f.source.QueueTask.Selection
				args = append(args, "--task-id", binding.TaskID, "--task-owner", binding.Owner, "--task-revision", fmt.Sprint(binding.TaskRevision), "--task-claim-generation", fmt.Sprint(binding.ClaimGeneration))
			}
			raw := f.command(append([]string{"project", "continue", "--prepare"}, args...)...)
			validateQueueCLI(t, "ProjectLaunchSummaryV5", raw)
			var review projectLaunchSummary
			json.Unmarshal([]byte(raw), &review)
			if review.WorktreeClaim == nil || review.WorktreeNaming != nil || review.WorktreeClaim.ID != f.source.Workspace.ID {
				t.Fatal("handover re-rendered naming")
			}
			startArgs := append(append([]string{"project", "continue"}, args...), "--command-id", "command:named-resume", "--expected-launch-digest", review.ReviewDigest)
			resultRaw := f.command(startArgs...)
			validateQueueCLI(t, "ProjectStartV5", resultRaw)
			var result projectStartResult
			json.Unmarshal([]byte(resultRaw), &result)
			if result.Workspace.Path != f.source.Workspace.Path || result.Workspace.Branch != f.source.Workspace.Branch || result.Workspace.Generation <= f.source.Workspace.Generation {
				t.Fatalf("handover: %+v", result.Workspace)
			}
			if f.command(startArgs...) != resultRaw {
				t.Fatal("duplicate continuation did not return original result")
			}
		})
	}
}
