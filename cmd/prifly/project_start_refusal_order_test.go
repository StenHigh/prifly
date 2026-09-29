package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// The pilot's start met dependency_limit only while installing its edition,
// after the launch summary had been written: the refusal came as the second
// document on the stream, the exit was 2, and a reader taking the first
// document read a summary of a launch that never started. A refusal that is
// already certain now comes before the summary, alone.
// singleLaunchFixture is a project with one launch whose workflow is one
// assisted step, committed. Changing the step's context text and committing
// makes a new edition of the package.
func singleLaunchFixture(t *testing.T) (root, authority string) {
	t.Helper()
	root, authority = newProjectFixture(t)
	writeFixtureFile(t, root, ".prifly/project.yaml", `schema_version: prifly-project-profile/3
`+projectHostsYAML+`packages:
  single: {source: .prifly/workflows/single}
launches:
  single:
    title: Single
    description: One assisted step.
    kind: workflow
    workflow: .prifly/workflows/single/workflow.yaml
`)
	folder := ".prifly/workflows/single/"
	writeFixtureFile(t, root, folder+"extend.yaml", "extensions: []\n")
	writeFixtureFile(t, root, folder+"schemas/object.yaml", "id: example-single:schema/object\nversion: 1.0.0\ntype: object\n")
	writeFixtureFile(t, root, "task.json", "{}\n")
	writeFixtureFile(t, root, folder+"contexts/work.yaml", "id: example-single:context/work\nversion: 1.0.0\nmedia_type: text/markdown; charset=utf-8\ntext: Fixture work.\n")
	writeFixtureFile(t, root, folder+"steps/work.yaml", `authoring: prifly-step/1
id: example:step/work
version: 1.0.0
kind: worker
executor: {adapter_ref: "{{assisted_adapter}}", operation: session}
instructions_ref: "{{context_work}}"
effects: {class: none, retry_class: never}
result_schema_ref: "{{step_result_schema}}"
`)
	writeFixtureFile(t, root, folder+"workflow.yaml", `authoring: prifly-project-workflow/1
package:
  id: example-single:package/single
  version: 1.0.0
  description: One assisted step.
  requires_core_protocol: "1"
  references:
    assisted_adapter: core:adapter/assisted-session@1.0.0
    local_policy: core:policy/local@3.0.0
    step_result_schema: core:schema/step-result@1.0.0
id: example:workflow/single
version: 1.0.0
refs:
  work: "{{step_work}}"
  local_policy: "{{local_policy}}"
  object: "{{schema_object}}"
inputs: {task: {schema_ref: object}}
entry: work
limits: {max_step_instances: 1, max_control_transitions: 2}
policy_ref: local_policy
stages:
  work: {kind: step, step_ref: work, on: {pass: done}, impossible_verdicts: [fail, needs_revision, no_work]}
  done: {kind: finish, outcome: succeeded}
`)
	gitFixture(t, root, "add", ".")
	gitFixture(t, root, "commit", "-qm", "base")
	return root, authority
}

// fillRegistry fills the authority's local registry to entries.
func fillRegistry(t *testing.T, authority string, entries int) {
	t.Helper()
	registryPath := filepath.Join(authority, "definitions.json")
	var registry prifly.RegistryFile
	data, err := os.ReadFile(registryPath)
	if err != nil || json.Unmarshal(data, &registry) != nil {
		t.Fatalf("read registry: %v", err)
	}
	filler := []byte(`{"type":"object"}`)
	fillerDigest, err := flow.Digest(filler)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(authority, "schemas"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authority, "schemas/filler.json"), filler, 0600); err != nil {
		t.Fatal(err)
	}
	for index := len(registry.Entries); index < entries; index++ {
		registry.Entries = append(registry.Entries, prifly.Definition{Ref: flow.Ref{ID: "example:schema/filler-" + fmt.Sprint(index), Version: "1.0.0", Digest: fillerDigest}, Kind: "schema", Path: "schemas/filler.json"})
	}
	filled, err := json.Marshal(registry)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(registryPath, filled, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestProjectStartRefusesAFullRegistryBeforeItsSummary(t *testing.T) {
	root, authority := singleLaunchFixture(t)
	// Fill the authority's own registry to the bound: this launch's edition
	// is what tips it over.
	fillRegistry(t, authority, prifly.MaxLocalRegistryEntries)
	code, out, stderr := runCLI(t, "--project", authority, "project", "start", "--repository", root, "--launch", "single", "--host", "codex-cli", "--input", "task="+filepath.Join(root, "task.json"))
	if code == 0 {
		t.Fatalf("a launch past the registry bound started: %s", out)
	}
	if out != "" {
		t.Fatalf("a refused start wrote a result: %s", out)
	}
	documents := 0
	decoder := json.NewDecoder(strings.NewReader(stderr))
	var first map[string]any
	for decoder.More() {
		var document map[string]any
		if err := decoder.Decode(&document); err != nil {
			t.Fatalf("stderr is not JSON documents: %v\n%s", err, stderr)
		}
		if documents == 0 {
			first = document
		}
		documents++
	}
	if documents != 1 || first["code"] != "dependency_limit" {
		t.Fatalf("the refusal is not the only document on stderr (%d documents):\n%s", documents, stderr)
	}
	if message, _ := first["message"].(string); !strings.Contains(message, "package restore --id ID --version VERSION --reason TEXT") {
		t.Fatalf("the refusal does not say package remove is undone by package restore: %s", message)
	}
}

// newSingleEdition changes the package so the next launch installs a new edition.
func newSingleEdition(t *testing.T, root, text string) {
	t.Helper()
	writeFixtureFile(t, root, ".prifly/workflows/single/contexts/work.yaml", "id: example-single:context/work\nversion: 1.0.0\nmedia_type: text/markdown; charset=utf-8\ntext: "+text+"\n")
	gitFixture(t, root, "commit", "-qam", text)
}

// startSingle starts the single launch and answers with its Run id.
func startSingle(t *testing.T, root, authority string) string {
	t.Helper()
	code, out, stderr := runCLI(t, "--project", authority, "project", "start", "--repository", root, "--launch", "single", "--host", "codex-cli", "--input", "task="+filepath.Join(root, "task.json"))
	if code != 0 {
		t.Fatalf("start: exit=%d %s", code, stderr)
	}
	var started projectStartResult
	if err := json.Unmarshal([]byte(out), &started); err != nil {
		t.Fatal(err)
	}
	// The id and the next move lead the document; a weak reader could not
	// find either in the Run view below them.
	if started.RunID != started.Run.Run.ID || started.NextCommand != "session task --run "+started.RunID+" --all" || !strings.HasPrefix(out, "{\"schema_version\":\"project-start/3\",\"run_id\":") {
		t.Fatalf("the start does not lead with the Run id and the next command: %s %s", started.RunID, started.NextCommand)
	}
	return started.Run.Run.ID
}

// finishSingle reports pass for the Run's one step and drives it to its end.
func finishSingle(t *testing.T, authority, runID string) {
	t.Helper()
	code, out, stderr := runCLI(t, "--project", authority, "session", "task", "--run", runID)
	if code != 0 {
		t.Fatalf("session task: %s", stderr)
	}
	var task prifly.SessionTask
	if err := json.Unmarshal([]byte(out), &task); err != nil {
		t.Fatal(err)
	}
	result, err := json.Marshal(map[string]any{"schema_version": "1", "run_id": task.RunID, "step_instance_id": task.StepInstanceID, "attempt_id": task.AttemptID, "envelope_digest": task.EnvelopeDigest, "verdict": "pass", "outputs": map[string]any{}, "evidence_refs": []any{}, "effect_receipt_refs": []any{}, "summary": "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	submission, err := json.Marshal(prifly.SessionSubmission{SchemaVersion: task.SchemaVersion, RunID: task.RunID, AttemptID: task.AttemptID, EnvelopeDigest: task.EnvelopeDigest, Result: result})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "submission.json")
	if err := os.WriteFile(path, submission, 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"session", "submit", "--file", path}, {"run", "drive", runID}} {
		if code, _, stderr := runCLI(t, append([]string{"--project", authority}, args...)...); code != 0 {
			t.Fatalf("%v: %s", args, stderr)
		}
	}
}

func singleEditions(t *testing.T, authority string) map[string]string {
	t.Helper()
	code, out, stderr := runCLI(t, "--project", authority, "package", "list")
	if code != 0 {
		t.Fatalf("package list: %s", stderr)
	}
	var listing struct {
		Packages []struct {
			Ref    flow.Ref `json:"ref"`
			Status string   `json:"status"`
		} `json:"packages"`
	}
	if err := json.Unmarshal([]byte(out), &listing); err != nil {
		t.Fatal(err)
	}
	editions := map[string]string{}
	for _, pkg := range listing.Packages {
		if pkg.Ref.ID == "example-single:package/single" {
			status := pkg.Status
			if status == "" {
				status = prifly.PackageTrusted
			}
			editions[pkg.Ref.Version] = status
		}
	}
	return editions
}

// A project that follows its package installs a new edition on every change,
// and the registry filled every few launches until a person withdrew old
// editions by hand. The launch now withdraws them itself: never one a Run still
// in progress holds, never the newest earlier one, shown in the review before
// anything changes, and only in the transaction that trusts the new edition.
func TestProjectStartWithdrawsOldEditionsToFitTheRegistry(t *testing.T) {
	root, authority := singleLaunchFixture(t)
	if code, _, stderr := runCLI(t, "--project", authority, "capacity", "set", "--capacity", "4", "--reason", "Runs side by side"); code != 0 {
		t.Fatalf("capacity: %s", stderr)
	}
	held := startSingle(t, root, authority)
	editions := singleEditions(t, authority)
	if len(editions) != 1 {
		t.Fatalf("the first launch did not install one edition: %v", editions)
	}
	var first string
	for version := range editions {
		first = version
	}
	engine, err := prifly.Open(authority, true)
	if err != nil {
		t.Fatal(err)
	}
	budget, err := engine.RegistryBudget()
	engine.Close()
	if err != nil {
		t.Fatal(err)
	}
	size := budget.Entries
	// Room for exactly one more edition of the same size.
	var registry prifly.RegistryFile
	if data, err := os.ReadFile(filepath.Join(authority, "definitions.json")); err != nil || json.Unmarshal(data, &registry) != nil {
		t.Fatalf("read registry: %v", err)
	}
	fillRegistry(t, authority, len(registry.Entries)+prifly.MaxLocalRegistryEntries-size-(size-len(registry.Entries)))
	newSingleEdition(t, root, "Second work.")
	second := startSingle(t, root, authority)
	finishSingle(t, authority, second)
	newSingleEdition(t, root, "Third work.")
	// The first edition is held by a Run in progress, the second is the newest
	// earlier one: nothing may go, and the refusal says why, alone.
	code, out, stderr := runCLI(t, "--project", authority, "project", "start", "--repository", root, "--launch", "single", "--host", "codex-cli", "--input", "task="+filepath.Join(root, "task.json"))
	if code == 0 || out != "" || strings.Count(stderr, `"schema_version"`) != 1 || !strings.Contains(stderr, "dependency_limit") || !strings.Contains(stderr, "is still in progress") || !strings.Contains(stderr, "newest earlier edition is kept") {
		t.Fatalf("a launch that cannot make room did not refuse alone, naming what stays: %d %s", code, stderr)
	}
	finishSingle(t, authority, held)
	// The review shows the withdrawal and changes nothing.
	args := []string{"--repository", root, "--launch", "single", "--host", "codex-cli", "--input", "task=" + filepath.Join(root, "task.json")}
	code, out, stderr = runCLI(t, append([]string{"--project", authority, "project", "questionnaire", "--prepare"}, args...)...)
	if code != 0 {
		t.Fatalf("prepare: %s", stderr)
	}
	var reviewed projectLaunchSummary
	if err := json.Unmarshal([]byte(out), &reviewed); err != nil {
		t.Fatal(err)
	}
	if reviewed.RegistryRetirement == nil || len(reviewed.RegistryRetirement.Retire) != 1 || reviewed.RegistryRetirement.Retire[0].Ref.Version != first {
		t.Fatalf("the review does not show the edition the launch withdraws: %+v", reviewed.RegistryRetirement)
	}
	if reviewed.RegistryBudget == nil || reviewed.RegistryBudget.WouldRefuse != "" || reviewed.RegistryBudget.Entries > prifly.MaxLocalRegistryEntries {
		t.Fatalf("the review's budget refuses a launch that starts: %+v", reviewed.RegistryBudget)
	}
	if status := singleEditions(t, authority)[first]; status != prifly.PackageTrusted {
		t.Fatalf("the review withdrew an edition: %s", status)
	}
	code, out, stderr = runCLI(t, append(append([]string{"--project", authority, "project", "start"}, args...), "--expected-launch-digest", reviewed.ReviewDigest)...)
	if code != 0 {
		t.Fatalf("the reviewed launch did not start: %s", stderr)
	}
	var started projectStartResult
	if err := json.Unmarshal([]byte(out), &started); err != nil || len(started.RetiredEditions) != 1 || started.RetiredEditions[0].Ref.Version != first {
		t.Fatalf("the start does not say which edition it withdrew: %+v %v", started.RetiredEditions, err)
	}
	after := singleEditions(t, authority)
	if len(after) != 3 || after[first] != prifly.PackageRemoved {
		t.Fatalf("the launch did not withdraw exactly the oldest edition: %v", after)
	}
	for version, status := range after {
		if version != first && status != prifly.PackageTrusted {
			t.Fatalf("edition %s was withdrawn too: %v", version, after)
		}
	}
	// The Run that ran on the withdrawn edition still reads.
	if code, _, stderr := runCLI(t, "--project", authority, "run", "status", held); code != 0 {
		t.Fatalf("a finished Run on a withdrawn edition no longer reads: %s", stderr)
	}
}
