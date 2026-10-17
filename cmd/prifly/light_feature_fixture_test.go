package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// This optional replay compiles an immutable consumer checkout and admits its
// original graph. It never drives a step, invokes a model or edits the package.
func TestLightShippedFeaturePhaseFixture(t *testing.T) {
	t.Parallel()
	source := os.Getenv("PRIFLY_LIGHT_FEATURE_SOURCE")
	target := os.Getenv("PRIFLY_LIGHT_PERF_FIXTURE_DIR")
	if source == "" || target == "" {
		t.Skip("explicit immutable consumer source required")
	}
	root, authority := newProjectFixture(t)
	folder := filepath.Join(root, ".prifly/workflows/aif-feature")
	if err := os.CopyFS(folder, os.DirFS(source)); err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, root, ".prifly/project.yaml", "schema_version: prifly-project-profile/3\ntitle: Shipped feature fixture\nhosts: {codex-cli: .codex/skills}\npackages: {feature: {source: .prifly/workflows/aif-feature}}\nlaunches: {}\n")
	files, err := filepath.Glob(filepath.Join(folder, "contexts/*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		body, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		value, err := flow.Parse(body, "yaml")
		if err != nil {
			t.Fatal(err)
		}
		object := value.(map[string]any)
		if src, ok := object["source"].(map[string]any); ok && src["root"] == "host_skills" {
			writeFixtureFile(t, root, ".codex/skills/"+src["path"].(string), "# Deterministic fixture host instructions\n")
		}
	}
	output := filepath.Join(t.TempDir(), "compiled")
	raw := queueCLICommand(t, "project", "compile", "--repository", root, "--package", "feature", "--host", "codex-cli", "--output", output)
	var compiled projectCompileResult
	if err := json.Unmarshal([]byte(raw), &compiled); err != nil {
		t.Fatal(err)
	}
	python, err := filepath.EvalSymlinks("/usr/bin/python3")
	if err != nil {
		t.Fatal(err)
	}
	queueCLICommand(t, "project", "local", "set", "--repository", root, "--allow-executable", "python="+python)
	gitFixture(t, root, "add", ".")
	gitFixture(t, root, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "fixture inputs")
	e, err := prifly.Open(authority, false)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	ctx := context.Background()
	imported, err := e.ImportPackage(ctx, prifly.PackageImportRequest{CommandID: "command:feature-import", Directory: output, Reason: "immutable Light feature fixture"})
	if err != nil || imported.Receipt.Rejection != nil {
		t.Fatalf("import: %+v %v", imported, err)
	}
	closure := map[flow.Ref]bool{}
	workflowPath := ""
	for _, component := range compiled.Components {
		closure[component.Ref] = true
		if component.Path == "workflow.json" || component.Ref.ID == "aif-feature:workflow/feature" {
			workflowPath = component.Path
		}
	}
	executionBytes, err := os.ReadFile(filepath.Join(output, projectExecutionFile))
	if err != nil {
		t.Fatal(err)
	}
	compiled.ExecutionBindings, err = projectDecodeExecution(executionBytes)
	if err != nil {
		t.Fatal(err)
	}
	execution, err := projectExecutionPayload(root, compiled, closure, true)
	if err != nil {
		t.Fatal(err)
	}
	workflowPath, err = projectInstalledWorkflowPath(ctx, e, compiled.Package, workflowPath)
	if err != nil {
		t.Fatal(err)
	}
	check := map[string]any{"id": "fixture", "description": "Recorded read-only fixture", "method": "code", "required": true, "enabled": true, "applicable": true}
	task := map[string]any{"title": "Выпущенный aif-feature: ожидает подготовку", "description": "Verify declared Light phase without running a reader", "scope": []string{"Read projection"}, "assumptions": []string{}, "checks": []any{check}, "security": []any{check}, "review": []any{check}, "optional_policy": "ignore", "require_separate_session": false, "product": nil}
	body, _ := json.Marshal(task)
	claim, err := e.ClaimWorktree(ctx, prifly.ClaimRequest{CommandID: "command:feature-claim", Repository: root, OwnerID: "host:light-fixture", WorkspaceMode: "checkout"})
	if err != nil {
		t.Fatal(err)
	}
	brief, _ := json.Marshal(prifly.Brief{SchemaVersion: "1", ID: "brief:feature", Subject: "Выпущенный aif-feature: ожидает подготовку", DesiredOutcome: "Read the admitted graph only", InScope: []string{"Read projection"}, OutOfScope: []string{"LLM execution"}, CompletionCriteria: []string{"Mapped phase"}, SourceRefs: []prifly.ArtifactRef{}, Assumptions: []string{}, Confirmation: "explicit"})
	started, err := e.Start(ctx, prifly.StartOptions{SchemaVersion: "2", CommandID: "command:feature-start", ProjectTitle: "Shipped feature fixture", WorkflowFile: workflowPath, Brief: brief, InputValues: map[string]json.RawMessage{"task": body}, WorkspaceMode: "checkout", WorkspaceClaim: &claim, ExecutionBindings: execution})
	if err != nil {
		t.Fatal(err)
	}
	view, err := e.MonitorView(ctx, started.Receipt.RunID)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ := json.Marshal(view)
	runPath := filepath.Join(t.TempDir(), "run-view.json")
	if err := os.WriteFile(runPath, snapshot, 0600); err != nil {
		t.Fatal(err)
	}
	// Resolve exactly the source->compiled mappings recorded by the compiler.
	provenancePath := filepath.Join(output, projectBuildFile)
	mappingPath := filepath.Join(target, "feature-mapping.json")
	script, err := filepath.Abs("../../scripts/light-projection.py")
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(python, script, "--run-view", runPath, "--build-provenance", provenancePath, "--output", mappingPath)
	if result, err := command.CombinedOutput(); err != nil {
		t.Fatalf("exact mapping resolver: %s %v", result, err)
	}
	mapping, err := os.ReadFile(mappingPath)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := prifly.ParseLightProjection(mapping)
	if err != nil {
		t.Fatal(err)
	}
	light, err := e.Light(ctx, prifly.NewLightCache(), projection)
	if err != nil || len(light.Cards) != 1 || light.Cards[0].Lane != "plan" || !light.Cards[0].Phase.Mapped {
		t.Fatalf("shipped feature phase: %+v %v", light, err)
	}
	if !strings.Contains(light.Cards[0].Title, "aif-feature") {
		t.Fatal("lost task title")
	}
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	if err := monitorCopyAuthority(filepath.Join(target, "shipped-feature"), authority); err != nil {
		t.Fatal(err)
	}
}
