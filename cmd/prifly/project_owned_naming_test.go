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

func TestProjectOwnedNamingClosedWorkflowBeforeEffects(t *testing.T) {
	t.Parallel()
	root, authority, args := writeNamedProjectFixture(t)
	profilePath := filepath.Join(root, ".prifly/project.yaml")
	data, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	profile := strings.Replace(strings.Split(string(data), "worktree_naming:")[0], "prifly-project-profile/4", "prifly-project-profile/5", 1)
	profile += `worktree_naming:
  template: "demo-{id}"
  inputs:
    identity: {schema: .prifly/naming/identity.schema.json}
  bindings:
    id: {naming_input: identity, pointer: /id}
`
	writeFixtureFile(t, root, ".prifly/project.yaml", profile)
	writeFixtureFile(t, root, ".prifly/naming/identity.schema.json", `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","required":["id"],"properties":{"id":{"type":"string"}},"additionalProperties":false}`)
	// The unchanged workflow has only its closed value input. Naming metadata must never enter it.
	workflowPath := filepath.Join(root, ".prifly/workflows/inspect/workflow.yaml")
	upstream, _ := os.ReadFile(workflowPath)
	code, _, stderr := runCLI(t, append([]string{"project", "questionnaire", "--prepare"}, args...)...)
	if code == 0 || !strings.Contains(stderr, "project_worktree_input_missing") {
		t.Fatalf("missing input: %d %s", code, stderr)
	}
	file := filepath.Join(t.TempDir(), "identity.json")
	if err := os.WriteFile(file, []byte(`{"id":"issue-19"}`), 0600); err != nil {
		t.Fatal(err)
	}
	out := queueCLICommand(t, append(append([]string{"project", "questionnaire", "--prepare"}, args...), "--naming-input", "identity="+file)...)
	var review projectLaunchSummary
	if err := json.Unmarshal([]byte(out), &review); err != nil {
		t.Fatal(err)
	}
	if review.SchemaVersion != "project-launch-summary/8" || review.WorktreeNaming.Base != "demo-issue-19" {
		t.Fatal(out)
	}
	assertUnifiedLaunchHasNoEffects(t, root, authority)
	current, _ := os.ReadFile(workflowPath)
	if string(current) != string(upstream) {
		t.Fatal("workflow changed")
	}
	engine, err := prifly.Open(authority, true)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	claims, err := engine.Claims(context.Background())
	if err != nil || len(claims.Claims) != 0 {
		t.Fatal("prepare mutated claims", err)
	}
}

func ownedNamingFixture(t *testing.T) (string, string, []string, string) {
	t.Helper()
	root, authority, args := writeNamedProjectFixture(t)
	configureOwnedNaming(t, root)
	file := filepath.Join(root, "naming.json")
	writeFixtureFile(t, root, "naming.json", `{"id":"issue-19","note":"original"}`)
	return root, authority, append(args, "--naming-input", "identity="+file), file
}

func configureOwnedNaming(t *testing.T, root string) {
	t.Helper()
	path := filepath.Join(root, ".prifly/project.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	profile := strings.Split(string(data), "worktree_naming:")[0]
	profile = strings.ReplaceAll(strings.ReplaceAll(profile, "prifly-project-profile/3", "prifly-project-profile/5"), "prifly-project-profile/4", "prifly-project-profile/5")
	writeFixtureFile(t, root, ".prifly/project.yaml", profile+`worktree_naming:
  template: "demo-{id}"
  inputs:
    identity: {schema: .prifly/naming/identity.schema.json}
  bindings:
    id: {naming_input: identity, pointer: /id}
`)
	writeFixtureFile(t, root, ".prifly/naming/identity.schema.json", `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","required":["id"],"properties":{"id":{"type":"string"},"note":{"type":"string"}},"additionalProperties":false}`)
}

func TestProjectOwnedNamingReviewFreshnessAndPersistedStart(t *testing.T) {
	t.Parallel()
	root, authority, args, file := ownedNamingFixture(t)
	prepare := func() projectLaunchSummary {
		raw := queueCLICommand(t, append([]string{"project", "questionnaire", "--prepare"}, args...)...)
		validateQueueCLI(t, "ProjectLaunchSummaryV8", raw)
		var review projectLaunchSummary
		if err := json.Unmarshal([]byte(raw), &review); err != nil {
			t.Fatal(err)
		}
		return review
	}
	original := prepare()
	// Whitespace does not affect either snapshot or review.
	writeFixtureFile(t, root, "naming.json", "{\n  \"note\": \"original\", \"id\": \"issue-19\"\n}\n")
	if prepare().ReviewDigest != original.ReviewDigest {
		t.Fatal("canonical equivalent changed review")
	}
	for _, change := range []string{"unused-value", "schema", "declaration"} {
		t.Run(change, func(t *testing.T) {
			switch change {
			case "unused-value":
				writeFixtureFile(t, root, "naming.json", `{"id":"issue-19","note":"changed"}`)
			case "schema":
				writeFixtureFile(t, root, ".prifly/naming/identity.schema.json", `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"}`)
			case "declaration":
				p := filepath.Join(root, ".prifly/project.yaml")
				b, _ := os.ReadFile(p)
				writeFixtureFile(t, root, ".prifly/project.yaml", strings.Replace(string(b), "demo-{id}", "changed-{id}", 1))
			}
			current := prepare()
			if current.ReviewDigest == original.ReviewDigest {
				t.Fatal("full snapshot change not reviewed")
			}
			if current.Workflow != original.Workflow || current.Package != original.Package {
				t.Fatal("naming changed compiled workflow identity")
			}
			code, _, stderr := runCLI(t, append(append([]string{"project", "start"}, args...), "--expected-launch-digest", original.ReviewDigest)...)
			if code == 0 || !strings.Contains(stderr, `"code":"project_start_stale_launch"`) {
				t.Fatalf("stale: %d %s", code, stderr)
			}
			configureOwnedNaming(t, root)
			writeFixtureFile(t, root, "naming.json", `{"id":"issue-19","note":"original"}`)
		})
	}
	gitFixture(t, root, "add", ".")
	gitFixture(t, root, "commit", "-qm", "project owned naming")
	review := prepare()
	start := append(append([]string{"project", "start"}, args...), "--command-id", "command:owned-start", "--expected-launch-digest", review.ReviewDigest)
	raw := queueCLICommand(t, start...)
	validateQueueCLI(t, "ProjectStartV8", raw)
	var result projectStartResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatal(err)
	}
	if result.Workspace.NamingSnapshot == nil || result.Workspace.Path != ".prifly/work/claims/demo-issue-19" {
		t.Fatal(raw)
	}
	if _, ok := result.Run.Run.Inputs["identity"]; ok {
		t.Fatal("naming leaked into workflow input")
	}
	if queueCLICommand(t, start...) != raw {
		t.Fatal("duplicate changed result")
	}
	engine, err := prifly.Open(authority, true)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	record, err := engine.Claims(context.Background())
	if err != nil || record.SchemaVersion != prifly.SnapshotAuthorityClaimsVersion || !prifly.EqualWorktreeNamingSnapshot(record.Claims[0].NamingSnapshot, review.WorktreeNaming.Snapshot) {
		t.Fatal("snapshot lost after reopen", err)
	}
	view, _ := json.Marshal(prifly.ClaimView(record))
	validateQueueCLI(t, "WorktreeClaimsV4", string(view))
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
}

func TestProjectOwnedNamingRefusalsAndPathGuards(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, value, schema, code string }{
		{"null", `{"id":null}`, "", "project_worktree_input_invalid"},
		{"number", `{"id":19}`, "", "project_worktree_input_invalid"},
		{"prose", `{"note":"issue 19"}`, "", "project_worktree_input_invalid"},
		{"invalid-name", `{"id":"../bad"}`, "", "project_worktree_value_invalid"},
		{"remote", `{"id":"ok"}`, `{"$schema":"https://json-schema.org/draft/2020-12/schema","$ref":"https://example.invalid/schema"}`, "project_worktree_schema_invalid"},
		{"fragment", `{"id":"ok"}`, `{"$schema":"https://json-schema.org/draft/2020-12/schema","$defs":{"id":{"type":"string"}},"type":"object","properties":{"id":{"$ref":"#/$defs/id"}}}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root, authority, args, _ := ownedNamingFixture(t)
			writeFixtureFile(t, root, "naming.json", tc.value)
			if tc.schema != "" {
				writeFixtureFile(t, root, ".prifly/naming/identity.schema.json", tc.schema)
			}
			code, _, stderr := runCLI(t, append([]string{"project", "questionnaire", "--prepare"}, args...)...)
			if (code == 0) != (tc.code == "") || tc.code != "" && !strings.Contains(stderr, `"code":"`+tc.code+`"`) {
				t.Fatalf("%d %s", code, stderr)
			}
			engine, err := prifly.Open(authority, true)
			if err != nil {
				t.Fatal(err)
			}
			defer engine.Close()
			record, err := engine.Claims(context.Background())
			if err != nil || len(record.Claims) != 0 {
				t.Fatal("refusal created claim", err)
			}
		})
	}
}

func TestProjectOwnedNamingChangedAfterSummaryBeforePreflight(t *testing.T) {
	t.Parallel()
	for _, queue := range []bool{false, true} {
		t.Run(fmt.Sprint(queue), func(t *testing.T) { t.Parallel(); checkOwnedNamingBeforePreflight(t, queue) })
	}
}

func checkOwnedNamingBeforePreflight(t *testing.T, queue bool) {
	t.Helper()
	root, authority, args, _ := ownedNamingFixture(t)
	path := filepath.Join(root, ".prifly/project.yaml")
	profile, _ := os.ReadFile(path)
	writeFixtureFile(t, root, ".prifly/project.yaml", strings.Replace(string(profile), "    workflow: .prifly/workflows/inspect/workflow.yaml\n", "    workflow: .prifly/workflows/inspect/workflow.yaml\n    preflight: {executable: shell, args: [preflight.sh], timeout_ms: 5000}\n", 1))
	writeFixtureFile(t, root, "preflight.sh", "#!/bin/sh\necho effect > preflight-effect\n")
	queueCLICommand(t, "project", "local", "set", "--repository", root, "--allow-executable", "shell=/bin/sh")
	if queue {
		input := queueCLIRequest(t, root, "naming freshness queue")
		queueCLICommand(t, "project", "queue", "add", "--repository", root, "--input", input, "--command-id", "command:naming-task")
		claimed := queueCLISummary(t, queueCLICommand(t, "project", "queue", "claim-next", "--repository", root, "--owner", "host:naming", "--command-id", "command:naming-claim"))
		args = append(args, queueSelectionArgs(claimed)...)
		args = append(args, "--command-id", "command:naming-start")
	}
	queueBefore := queueCLICommand(t, "project", "queue", "list", "--repository", root)
	raw := queueCLICommand(t, append([]string{"project", "questionnaire", "--prepare"}, args...)...)
	var review projectLaunchSummary
	json.Unmarshal([]byte(raw), &review)
	var out, stderr bytes.Buffer
	writer := projectSummaryWriter(func(data []byte) (int, error) {
		if bytes.Contains(data, []byte(`"schema_version":"project-launch-summary/8"`)) {
			writeFixtureFile(t, root, "naming.json", `{"id":"issue-20"}`)
		}
		return stderr.Write(data)
	})
	code := execute(context.Background(), append(append(append([]string{"project", "start"}, args...), "--expected-launch-digest", review.ReviewDigest), "--json"), &out, writer)
	if code == 0 || !strings.Contains(stderr.String(), `"code":"project_start_stale_launch"`) {
		t.Fatal(code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, "preflight-effect")); !os.IsNotExist(err) {
		t.Fatal("stale naming ran preflight")
	}
	if queueCLICommand(t, "project", "queue", "list", "--repository", root) != queueBefore {
		t.Fatal("stale naming mutated queue")
	}
	engine, err := prifly.Open(authority, true)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	claims, err := engine.Claims(context.Background())
	if err != nil || len(claims.Claims) != 0 {
		t.Fatal("stale naming created claim", err)
	}
}

func TestProjectOwnedNamingSourceSnapshotAfterPolicyRemoval(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                     string
		resume, queue, newCommit bool
	}{
		{"resume", true, false, false}, {"queued-resume", true, true, false},
		{"continue-handover", false, false, false}, {"continue-new-commit", false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			setup := func(root string) {
				configureOwnedNaming(t, root)
				writeFixtureFile(t, root, "naming.json", `{"id":"issue-19"}`)
			}
			f := continuationCLIFixtureWithOptions(t, "  from_outcomes: [partial]\n", tc.resume, tc.queue, []string{"--naming-input", "identity=naming.json"}, setup)
			f.submit(map[string]string{"handoff": "{}\n"}, nil)
			writeFixtureFile(t, f.source.WorkspacePath, "plans/plan.md", "# Plan\n")
			f.submit(map[string]string{"draft": "{}\n"}, []prifly.WorkspaceTreeLocation{{OutputPort: "plan", Path: "plans/plan.md"}})
			f.submit(map[string]string{}, nil)
			p := filepath.Join(f.root, ".prifly/project.yaml")
			data, _ := os.ReadFile(p)
			writeFixtureFile(t, f.root, ".prifly/project.yaml", strings.Split(string(data), "worktree_naming:")[0])
			for _, name := range []string{"naming.json", ".prifly/naming/identity.schema.json"} {
				if err := os.Remove(filepath.Join(f.root, name)); err != nil {
					t.Fatal(err)
				}
			}
			launch := "tail"
			if tc.resume {
				launch = "source"
			}
			args := []string{"--repository", f.root, "--launch", launch, "--host", "codex-cli", "--source-run", f.source.RunID}
			if tc.newCommit {
				args = append(args, "--workspace-commit", f.base)
			}
			if tc.queue {
				binding := f.source.QueueTask.Selection
				args = append(args, "--task-id", binding.TaskID, "--task-owner", binding.Owner, "--task-revision", fmt.Sprint(binding.TaskRevision), "--task-claim-generation", fmt.Sprint(binding.ClaimGeneration))
			}
			commandName := "continue"
			f.refuse("project_worktree_input_override", append(append([]string{"project", commandName, "--prepare"}, args...), "--naming-input", "identity=other.json")...)
			raw := f.command(append([]string{"project", commandName, "--prepare"}, args...)...)
			validateQueueCLI(t, "ProjectLaunchSummaryV8", raw)
			var review projectLaunchSummary
			json.Unmarshal([]byte(raw), &review)
			raw = f.command(append(append([]string{"project", commandName}, args...), "--command-id", "command:owned-source", "--expected-launch-digest", review.ReviewDigest)...)
			validateQueueCLI(t, "ProjectStartV8", raw)
			var result projectStartResult
			json.Unmarshal([]byte(raw), &result)
			if !prifly.EqualWorktreeNamingSnapshot(result.Workspace.NamingSnapshot, f.source.Workspace.NamingSnapshot) {
				t.Fatal("source snapshot changed")
			}
			if tc.newCommit {
				if result.Workspace.Path != f.source.Workspace.Path+"-02" || result.Workspace.BaseCommit != f.base {
					t.Fatal("new claim lost source base", raw)
				}
			} else if result.Workspace.Path != f.source.Workspace.Path || result.Workspace.Branch != f.source.Workspace.Branch || result.Workspace.Generation <= f.source.Workspace.Generation {
				t.Fatal("handover changed name or did not fence generation", raw)
			}
		})
	}
}

func TestProjectOwnedNamingDeclarationClosure(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, block string
		valid       bool
	}{
		{"new", `{"template":"{id}","inputs":{"identity":{"schema":".prifly/naming/schema.json"}},"bindings":{"id":{"naming_input":"identity","pointer":"/id"}}}`, true},
		{"mixed", `{"template":"{id}","inputs":{"identity":{"schema":"s"}},"bindings":{"id":{"value":"demo","naming_input":"identity","pointer":"/id"}}}`, false},
		{"mixed-null", `{"template":"{id}","bindings":{"id":{"value":"demo","naming_input":null}}}`, false},
		{"empty", `{"template":"{id}","inputs":{},"bindings":{"id":{"value":"demo"}}}`, false},
		{"unused", `{"template":"{id}","inputs":{"identity":{"schema":"s"}},"bindings":{"id":{"value":"demo"}}}`, false},
		{"unknown", `{"template":"{id}","inputs":{"identity":{"schema":"s","other":true}},"bindings":{"id":{"naming_input":"identity","pointer":"/id"}}}`, false},
		{"undeclared", `{"template":"{id}","bindings":{"id":{"naming_input":"identity","pointer":"/id"}}}`, false},
		{"bad-pointer", `{"template":"{id}","inputs":{"identity":{"schema":"s"}},"bindings":{"id":{"naming_input":"identity","pointer":"/bad~2"}}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var value any
			if err := json.Unmarshal([]byte(tc.block), &value); err != nil {
				t.Fatal(err)
			}
			_, err := projectParseWorktreeNaming(value)
			if (err == nil) != tc.valid {
				t.Fatal(tc.valid, err)
			}
		})
	}
	for _, count := range []int{16, 17} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			t.Parallel()
			inputs := map[string]any{}
			bindings := map[string]any{}
			parts := []string{}
			for i := 0; i < count; i++ {
				name := fmt.Sprintf("a%d", i)
				inputs[name] = map[string]any{"schema": "s"}
				bindings[name] = map[string]any{"naming_input": name, "pointer": "/id"}
				parts = append(parts, "{"+name+"}")
			}
			_, err := projectParseWorktreeNaming(map[string]any{"template": strings.Join(parts, "-"), "inputs": inputs, "bindings": bindings})
			if (err == nil) != (count == 16) {
				t.Fatal(count, err)
			}
		})
	}
}

func TestProjectOwnedNamingReaderRefusesTraversalAndLinks(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"schema-traversal", "schema-link", "directory-link", "value-link", "value-directory-link", "upstream-schema"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			root, _, args, file := ownedNamingFixture(t)
			schema := filepath.Join(root, ".prifly/naming/identity.schema.json")
			code := "project_worktree_schema_invalid"
			switch kind {
			case "schema-traversal", "upstream-schema":
				path := filepath.Join(root, ".prifly/project.yaml")
				data, _ := os.ReadFile(path)
				target := "../schema.json"
				if kind == "upstream-schema" {
					target = ".prifly/workflows/inspect/schemas/value.yaml"
				}
				writeFixtureFile(t, root, ".prifly/project.yaml", strings.Replace(string(data), ".prifly/naming/identity.schema.json", target, 1))
			case "schema-link":
				if err := os.Rename(schema, schema+".real"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(schema+".real", schema); err != nil {
					t.Fatal(err)
				}
			case "directory-link":
				dir := filepath.Dir(schema)
				if err := os.Rename(dir, dir+"-real"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(dir+"-real", dir); err != nil {
					t.Fatal(err)
				}
			case "value-link":
				code = "project_worktree_input_invalid"
				if err := os.Rename(file, file+".real"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(file+".real", file); err != nil {
					t.Fatal(err)
				}
			case "value-directory-link":
				code = "project_worktree_input_invalid"
				if err := os.Symlink(root, filepath.Join(root, "linked")); err != nil {
					t.Fatal(err)
				}
				args[len(args)-1] = "identity=" + filepath.Join(root, "linked/naming.json")
			}
			exit, _, stderr := runCLI(t, append([]string{"project", "questionnaire", "--prepare"}, args...)...)
			if exit == 0 || !strings.Contains(stderr, `"code":"`+code+`"`) {
				t.Fatal(exit, stderr)
			}
		})
	}
}

func TestProjectOwnedNamingNamespaceAndFlagRefusals(t *testing.T) {
	t.Parallel()
	root, _, args, file := ownedNamingFixture(t)
	path := filepath.Join(root, ".prifly/project.yaml")
	data, _ := os.ReadFile(path)
	writeFixtureFile(t, root, ".prifly/project.yaml", strings.ReplaceAll(string(data), "identity", "value"))
	// Keep the declaration path after changing the input name.
	schemaData, _ := os.ReadFile(filepath.Join(root, ".prifly/naming/identity.schema.json"))
	writeFixtureFile(t, root, ".prifly/naming/value.schema.json", string(schemaData))
	baseArgs := args[:len(args)-2]
	for _, tc := range []struct{ flag, code string }{{"unknown=" + file, "project_worktree_input_unknown"}, {"value=" + file, ""}} {
		raw := append(append([]string{"project", "questionnaire", "--prepare"}, baseArgs...), "--naming-input", tc.flag)
		exit, out, stderr := runCLI(t, raw...)
		if tc.code != "" {
			if exit == 0 || !strings.Contains(stderr, `"code":"`+tc.code+`"`) {
				t.Fatal(exit, stderr)
			}
			continue
		}
		var review projectLaunchSummary
		if exit != 0 || json.Unmarshal([]byte(out), &review) != nil || review.WorktreeNaming.Base != "demo-issue-19" || review.InputDigests["value"] == review.WorktreeNaming.Snapshot.Inputs["value"].ValueDigest {
			t.Fatal("namespaces overlap", out, stderr)
		}
	}
	exit, _, stderr := runCLI(t, append(append([]string{"project", "questionnaire", "--prepare"}, baseArgs...), "--naming-input", "value="+file, "--naming-input", "value="+file)...)
	if exit == 0 || !strings.Contains(stderr, `"code":"project_worktree_input_duplicate"`) {
		t.Fatal(exit, stderr)
	}
}

func TestProjectOwnedNamingUnifiedCompositionAndQueue(t *testing.T) {
	t.Parallel()
	root, _, args := writeUnifiedNamedLaunchFixture(t)
	unifiedRewriteFixture(t, filepath.Join(root, ".prifly/project.yaml"), func(profile map[string]any) {
		profile["schema_version"] = "prifly-project-profile/5"
		profile["worktree_naming"] = map[string]any{"template": "demo-{id}", "inputs": map[string]any{"identity": map[string]any{"schema": ".prifly/naming/identity.schema.json"}}, "bindings": map[string]any{"id": map[string]any{"naming_input": "identity", "pointer": "/id"}}}
	})
	writeFixtureFile(t, root, ".prifly/naming/identity.schema.json", `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"}`)
	writeFixtureFile(t, root, "identity.json", `{"id":"composition"}`)
	// Exercise the full snapshot budget through queue + composition carriers.
	profile, err := readProjectProfile(root)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := projectReadNamingSnapshot(root, profile.WorktreeNaming, projectNamingFiles{"identity": "identity.json"})
	if err != nil {
		t.Fatal(err)
	}
	base, _ := json.Marshal(snapshot)
	// Adding the padding field contributes thirteen JSON bytes plus its contents.
	padding := prifly.MaxWorktreeNamingSnapshotBytes - len(base) - 13
	writeFixtureFile(t, root, "identity.json", `{"id":"composition","padding":"`+strings.Repeat("x", padding)+`"}`)
	snapshot, err = projectReadNamingSnapshot(root, profile.WorktreeNaming, projectNamingFiles{"identity": "identity.json"})
	if err != nil {
		t.Fatal(err)
	}
	full, _ := json.Marshal(snapshot)
	if len(full) != prifly.MaxWorktreeNamingSnapshotBytes {
		t.Fatal("not exact snapshot boundary", len(full))
	}
	args = append(args, "--naming-input", "identity=identity.json")
	input := queueCLIRequest(t, root, "unified naming queue")
	queueCLICommand(t, "project", "queue", "add", "--repository", root, "--input", input, "--command-id", "command:unified-task")
	task := queueCLISummary(t, queueCLICommand(t, "project", "queue", "claim-next", "--repository", root, "--owner", "host:naming", "--command-id", "command:unified-task-claim"))
	args = append(args, queueSelectionArgs(task)...)
	raw := queueCLICommand(t, append([]string{"project", "questionnaire", "--prepare"}, args...)...)
	validateQueueCLI(t, "ProjectLaunchSummaryV8", raw)
	var summary projectLaunchSummary
	json.Unmarshal([]byte(raw), &summary)
	if summary.Composition == nil || summary.QueueTask == nil || summary.WorktreeNaming.Snapshot == nil {
		t.Fatal("missing coexistence fields", raw)
	}
	raw = queueCLICommand(t, append(append([]string{"project", "start"}, args...), "--command-id", "command:composed-owned", "--expected-launch-digest", summary.ReviewDigest)...)
	validateQueueCLI(t, "ProjectStartV8", raw)
	var result projectStartResult
	json.Unmarshal([]byte(raw), &result)
	if result.Workspace.NamingSnapshot == nil || result.QueueTask == nil {
		t.Fatal("missing claim or queue", raw)
	}
	if queueCLICommand(t, append(append([]string{"project", "start"}, args...), "--command-id", "command:composed-owned", "--expected-launch-digest", summary.ReviewDigest)...) != raw {
		t.Fatal("queue replay changed /8 result")
	}
}

func TestProjectOwnedNamingRecoveryFromTechnicalFailure(t *testing.T) {
	t.Parallel()
	setup := func(root string) {
		configureOwnedNaming(t, root)
		writeFixtureFile(t, root, "naming.json", `{"id":"recover-19"}`)
		path := filepath.Join(root, ".prifly/workflows/source/steps/polish.yaml")
		data, _ := os.ReadFile(path)
		writeFixtureFile(t, root, ".prifly/workflows/source/steps/polish.yaml", string(data)+`inputs: {plan: {schema_ref: "{{workspace_tree_manifest}}"}}
workspace_trees:
  - input_port: plan
    capture: {kind: exact_file, path: plans/plan.md}
`)
		path = filepath.Join(root, ".prifly/workflows/source/workflow.yaml")
		data, _ = os.ReadFile(path)
		writeFixtureFile(t, root, ".prifly/workflows/source/workflow.yaml", strings.Replace(string(data), "polish: {kind: step, step_ref: polish, on:", "polish: {kind: step, step_ref: polish, input_bindings: {plan: $stages.draft.plan}, on:", 1))
	}
	f := continuationCLIFixtureWithOptions(t, "  from_outcomes: [partial]\n", true, false, []string{"--naming-input", "identity=naming.json"}, setup)
	f.submit(map[string]string{"handoff": "{}\n"}, nil)
	writeFixtureFile(t, f.source.WorkspacePath, "plans/plan.md", "# Plan\n")
	f.submitOnly(map[string]string{"draft": "{}\n"}, []prifly.WorkspaceTreeLocation{{OutputPort: "plan", Path: "plans/plan.md"}})
	writeFixtureFile(t, f.source.WorkspacePath, "plans/plan.md", "# Changed after accepted capture\n")
	f.command("--project", f.authority, "run", "drive", f.source.RunID)
	var view prifly.RunView
	json.Unmarshal([]byte(f.command("--project", f.authority, "run", "status", f.source.RunID)), &view)
	if view.Run.Status != "failed" {
		t.Fatal("fixture did not reach technical failure", view.Run.Diagnostics)
	}
	writeFixtureFile(t, f.source.WorkspacePath, "plans/plan.md", "# Plan\n")
	path := filepath.Join(f.root, ".prifly/project.yaml")
	data, _ := os.ReadFile(path)
	writeFixtureFile(t, f.root, ".prifly/project.yaml", strings.Split(string(data), "worktree_naming:")[0])
	for _, name := range []string{"naming.json", ".prifly/naming/identity.schema.json"} {
		if err := os.Remove(filepath.Join(f.root, name)); err != nil {
			t.Fatal(err)
		}
	}
	args := []string{"--repository", f.root, "--launch", "source", "--host", "codex-cli", "--source-run", f.source.RunID}
	raw := f.command(append([]string{"project", "recover", "--prepare"}, args...)...)
	validateQueueCLI(t, "ProjectLaunchSummaryV8", raw)
	var review projectLaunchSummary
	json.Unmarshal([]byte(raw), &review)
	raw = f.command(append(append([]string{"project", "recover"}, args...), "--expected-launch-digest", review.ReviewDigest, "--command-id", "command:recover-snapshot")...)
	validateQueueCLI(t, "ProjectStartV8", raw)
	var result projectStartResult
	json.Unmarshal([]byte(raw), &result)
	if result.Workspace.ID != f.source.Workspace.ID || result.Workspace.Generation <= f.source.Workspace.Generation || !prifly.EqualWorktreeNamingSnapshot(result.Workspace.NamingSnapshot, f.source.Workspace.NamingSnapshot) {
		t.Fatal("recover changed snapshot or claim", raw)
	}
}

func TestProjectOwnedNamingOnlyAppliesToNewWorktrees(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"checkout", "no-workspace"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			root, _, args, _ := ownedNamingFixture(t)
			args = args[:len(args)-2]
			if mode == "checkout" {
				args[len(args)-1] = "checkout"
			} else {
				args = args[:len(args)-2]
				// Locate the fixture's only step rather than assume its component name.
				files, err := filepath.Glob(filepath.Join(root, ".prifly/workflows/inspect/steps/*.yaml"))
				if err != nil || len(files) != 1 {
					t.Fatal(files, err)
				}
				path := files[0]
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(strings.ReplaceAll(string(data), "workspace_write", "none")), 0600); err != nil {
					t.Fatal(err)
				}
			}
			gitFixture(t, root, "add", ".")
			gitFixture(t, root, "commit", "-qm", "conditional naming fixture")
			raw := queueCLICommand(t, append([]string{"project", "questionnaire", "--prepare"}, args...)...)
			var review projectLaunchSummary
			json.Unmarshal([]byte(raw), &review)
			if review.WorktreeNaming != nil {
				t.Fatal("unused naming read")
			}
			exit, _, stderr := runCLI(t, append(append([]string{"project", "questionnaire", "--prepare"}, args...), "--naming-input", "identity=naming.json")...)
			if exit == 0 || !strings.Contains(stderr, `"code":"project_worktree_input_inapplicable"`) {
				t.Fatal(exit, stderr)
			}
		})
	}
}

func TestProjectOwnedNamingSnapshotOverflowBeforeStartEffects(t *testing.T) {
	t.Parallel()
	root, authority, args, _ := ownedNamingFixture(t)
	writeFixtureFile(t, root, "naming.json", `{"id":"issue-19","note":"`+strings.Repeat("x", prifly.MaxWorktreeNamingSnapshotBytes)+`"}`)
	exit, _, stderr := runCLI(t, append([]string{"project", "start"}, args...)...)
	if exit == 0 || !strings.Contains(stderr, `"code":"project_worktree_snapshot_too_large"`) {
		t.Fatal(exit, stderr)
	}
	assertUnifiedLaunchHasNoEffects(t, root, authority)
}

func TestProjectOwnedNamingRepeatIgnoresFlagOrder(t *testing.T) {
	t.Parallel()
	root, _, args, file := ownedNamingFixture(t)
	path := filepath.Join(root, ".prifly/project.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	profile := strings.Replace(string(data), "template: \"demo-{id}\"", "template: \"demo-{id}-{source}\"", 1)
	profile = strings.Replace(profile, "    identity: {schema: .prifly/naming/identity.schema.json}", "    identity: {schema: .prifly/naming/identity.schema.json}\n    origin: {schema: .prifly/naming/identity.schema.json}", 1)
	profile += "    source: {naming_input: origin, pointer: /id}\n"
	writeFixtureFile(t, root, ".prifly/project.yaml", profile)
	writeFixtureFile(t, root, "origin.json", `{"id":"tracker"}`)
	base := append([]string{}, args[:len(args)-2]...)
	args = append(args, "--naming-input", "origin=origin.json")
	raw := queueCLICommand(t, append([]string{"project", "questionnaire", "--prepare"}, args...)...)
	var review projectLaunchSummary
	if err := json.Unmarshal([]byte(raw), &review); err != nil {
		t.Fatal(err)
	}
	raw = queueCLICommand(t, append(append([]string{"project", "start"}, args...), "--command-id", "command:reorder-naming", "--expected-launch-digest", review.ReviewDigest)...)
	reordered := append(base, "--naming-input", "origin=origin.json", "--naming-input", "identity="+file)
	replay := queueCLICommand(t, append(append([]string{"project", "start"}, reordered...), "--command-id", "command:reorder-naming", "--expected-launch-digest", review.ReviewDigest)...)
	if replay != raw {
		t.Fatal("flag order changed semantic retry")
	}
}

func TestProjectOwnedNamingSourceSnapshotAfterHandover(t *testing.T) {
	t.Parallel()
	for _, released := range []bool{false, true} {
		t.Run(fmt.Sprintf("released-%t", released), func(t *testing.T) {
			t.Parallel()
			f := continuationCLIFixtureWithOptions(t, "  from_outcomes: [partial]\n", false, false, []string{"--naming-input", "identity=naming.json"}, func(root string) {
				configureOwnedNaming(t, root)
				writeFixtureFile(t, root, "naming.json", `{"id":"issue-19"}`)
			})
			f.submit(map[string]string{"handoff": "{}\n"}, nil)
			writeFixtureFile(t, f.source.WorkspacePath, "plans/plan.md", "# Plan\n")
			f.submit(map[string]string{"draft": "{}\n"}, []prifly.WorkspaceTreeLocation{{OutputPort: "plan", Path: "plans/plan.md"}})
			f.submit(map[string]string{}, nil)
			args := []string{"--repository", f.root, "--launch", "tail", "--host", "codex-cli", "--source-run", f.source.RunID}
			var review projectLaunchSummary
			json.Unmarshal([]byte(f.command(append([]string{"project", "continue", "--prepare"}, args...)...)), &review)
			var handed projectStartResult
			json.Unmarshal([]byte(f.command(append(append([]string{"project", "continue"}, args...), "--command-id", "command:owned-handover", "--expected-launch-digest", review.ReviewDigest)...)), &handed)
			submitResumeChain(t, f, handed.RunID, "pass")
			if released {
				f.command("--project", f.authority, "claim", "release", "--id", handed.Workspace.ID, "--generation", fmt.Sprint(handed.Workspace.Generation))
			}
			profile, err := os.ReadFile(filepath.Join(f.root, ".prifly/project.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			writeFixtureFile(t, f.root, ".prifly/project.yaml", strings.Split(string(profile), "worktree_naming:")[0])
			for _, path := range []string{"naming.json", ".prifly/naming/identity.schema.json"} {
				if err := os.Remove(filepath.Join(f.root, path)); err != nil {
					t.Fatal(err)
				}
			}
			args = append(args, "--workspace-commit", f.base)
			json.Unmarshal([]byte(f.command(append([]string{"project", "continue", "--prepare"}, args...)...)), &review)
			raw := f.command(append(append([]string{"project", "continue"}, args...), "--command-id", "command:owned-after-handover", "--expected-launch-digest", review.ReviewDigest)...)
			var result projectStartResult
			json.Unmarshal([]byte(raw), &result)
			if result.Workspace.ID == handed.Workspace.ID || result.Workspace.BaseCommit != f.base || !prifly.EqualWorktreeNamingSnapshot(result.Workspace.NamingSnapshot, f.source.Workspace.NamingSnapshot) || result.Workspace.Naming == nil || result.Workspace.Naming.Base != f.source.Workspace.Naming.Base {
				t.Fatal("new claim lost source naming after handover", raw)
			}
			var claims prifly.ClaimRecord
			json.Unmarshal([]byte(f.command("--project", f.authority, "claim", "list")), &claims)
			for _, claim := range claims.Claims {
				if claim.ID == handed.Workspace.ID && (claim.RunID != handed.RunID || claim.Generation != handed.Workspace.Generation || claim.Path != handed.Workspace.Path || claim.Branch != handed.Workspace.Branch) {
					t.Fatal("historical claim was taken over", claim)
				}
			}
		})
	}
}

func TestProjectOwnedNamingLegacySourceIgnoresNewPolicy(t *testing.T) {
	t.Parallel()
	for _, named := range []bool{false, true} {
		t.Run(fmt.Sprintf("named-%t", named), func(t *testing.T) {
			t.Parallel()
			f := continuationCLIFixtureWithOptions(t, "  from_outcomes: [partial]\n", false, false, nil, func(root string) {
				if named {
					data, err := os.ReadFile(filepath.Join(root, ".prifly/project.yaml"))
					if err != nil {
						t.Fatal(err)
					}
					writeFixtureFile(t, root, ".prifly/project.yaml", strings.Replace(string(data), "prifly-project-profile/3", "prifly-project-profile/4", 1)+"worktree_naming:\n  template: '{project}'\n  bindings: {project: {value: legacy-demo}}\n")
				}
			})
			f.submit(map[string]string{"handoff": "{}\n"}, nil)
			writeFixtureFile(t, f.source.WorkspacePath, "plans/plan.md", "# Plan\n")
			f.submit(map[string]string{"draft": "{}\n"}, []prifly.WorkspaceTreeLocation{{OutputPort: "plan", Path: "plans/plan.md"}})
			f.submit(map[string]string{}, nil)
			configureOwnedNaming(t, f.root)
			if err := os.Remove(filepath.Join(f.root, ".prifly/naming/identity.schema.json")); err != nil {
				t.Fatal(err)
			}
			args := []string{"--repository", f.root, "--launch", "tail", "--host", "codex-cli", "--source-run", f.source.RunID, "--workspace-commit", f.base}
			var review projectLaunchSummary
			json.Unmarshal([]byte(f.command(append([]string{"project", "continue", "--prepare"}, args...)...)), &review)
			raw := f.command(append(append([]string{"project", "continue"}, args...), "--command-id", "command:legacy-new-policy", "--expected-launch-digest", review.ReviewDigest)...)
			var result projectStartResult
			json.Unmarshal([]byte(raw), &result)
			if result.Workspace.ID == f.source.Workspace.ID || result.Workspace.BaseCommit != f.base || result.Workspace.NamingSnapshot != nil || result.Workspace.Naming != nil {
				t.Fatal("legacy source acquired new naming metadata", raw)
			}
		})
	}
}
