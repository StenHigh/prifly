package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

func TestExecutionReviewChecksPinnedBytesWithoutDisclosure(t *testing.T) {
	e, options := acceptanceProject(t, []string{"workflow_input", "step_result"}, "", "pass", false)
	options.SchemaVersion = "2"
	started, err := e.Start(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	id := started.Receipt.RunID
	run, before, err := e.load(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]string{}
	for ref, executor := range run.Executors {
		expected[ref] = executor.ExecutableDigest
	}
	if len(expected) == 0 {
		t.Fatal("fixture must pin actual executables")
	}
	if err := e.CheckPinnedExecutables(context.Background(), id, expected); err != nil {
		t.Fatal(err)
	}
	wrong := maps.Clone(expected)
	for ref := range wrong {
		wrong[ref] = "sha256:" + strings.Repeat("0", 64)
		break
	}
	for _, candidate := range []map[string]string{nil, wrong} {
		err := e.CheckPinnedExecutables(context.Background(), id, candidate)
		var refused *Fault
		if !errors.As(err, &refused) || refused.Code != "execution_review_mismatch" {
			t.Fatalf("changed review accepted: %v", err)
		}
	}
	view, err := e.View(context.Background(), id)
	if err != nil || view.Run.Executors != nil {
		t.Fatal("review check exposed private executor configuration", err)
	}
	_, after, err := e.load(context.Background(), id)
	if err != nil || !bytes.Equal(before.Snapshot.Data, after.Snapshot.Data) || before.Snapshot.Version != after.Snapshot.Version {
		t.Fatal("read-only review changed the Run", err)
	}
}

func TestExecutionBindingsExactVersionsChecksAndRestart(t *testing.T) {
	e, firstOptions := acceptanceProject(t, []string{"workflow_input", "step_result"}, "", "pass", false)
	read := func(path string, target any) {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(e.Root, path))
		if err != nil || json.Unmarshal(data, target) != nil {
			t.Fatalf("read %s: %v", path, err)
		}
	}
	var step flow.StepDefinition
	var workflow flow.WorkflowRevision
	var registry RegistryFile
	read("steps/context.json", &step)
	read(firstOptions.WorkflowFile, &workflow)
	read("definitions.json", &registry)
	firstRef := workflow.Definition.Stages["work"].StepRef
	step.Version = "2.0.0"
	stepBytes, _ := canonical(step)
	secondRef := flow.Ref{ID: step.ID, Version: step.Version, Digest: rawDigest(stepBytes)}
	writeRuntimeJSON(t, filepath.Join(e.Root, "steps/context-two.json"), step)
	registry.Entries = append(registry.Entries, Definition{Ref: secondRef, Kind: "step", Path: "steps/context-two.json"})
	writeRuntimeJSON(t, filepath.Join(e.Root, "definitions.json"), registry)
	workflow.ID = "test:workflow/context-two"
	stage := workflow.Definition.Stages["work"]
	stage.StepRef = secondRef
	workflow.Definition.Stages["work"] = stage
	writeRuntimeJSON(t, filepath.Join(e.Root, "workflows/context-two.json"), workflow)

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	programBytes, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	secondProgram := filepath.Join(t.TempDir(), "worker-two")
	if err := os.WriteFile(secondProgram, programBytes, 0700); err != nil {
		t.Fatal(err)
	}
	secondProgram, err = filepath.EvalSymlinks(secondProgram)
	if err != nil {
		t.Fatal(err)
	}
	secondOptions := firstOptions
	secondOptions.CommandID, secondOptions.WorkflowFile = newID("command"), "workflows/context-two.json"
	options := []*StartOptions{&firstOptions, &secondOptions}
	var plans []*flow.Plan
	var definitions []PinnedDefinition
	for i, option := range options {
		plan, defs, _, _, err := e.compileFile(option.WorkflowFile)
		if err != nil {
			t.Fatal(err)
		}
		plans, definitions = append(plans, plan), defs
		payload := &ExecutionBindings{SchemaVersion: ExecutionBindingsVersion, Bindings: []ExecutionBinding{}}
		for ref := range executorDefinitions(plan) {
			config := e.Config.Configuration.Executors[ref.ID]
			files := map[string][]byte{}
			if ref.ID == step.ID {
				config.Files = map[string]string{"worker.data": "support/worker.data"}
				files["support/worker.data"] = []byte("pinned first")
				if i == 1 {
					config.Executable = secondProgram
					config.Args = []string{"-test.run=^TestDriverWorkerHelper$", "--", "mixed-pass"}
					files["support/worker.data"] = []byte("pinned second")
				}
			}
			payload.Bindings = append(payload.Bindings, ExecutionBinding{DefinitionRef: ref, Config: config, Files: files})
		}
		slices.SortFunc(payload.Bindings, func(a, b ExecutionBinding) int {
			return strings.Compare(a.DefinitionRef.String(), b.DefinitionRef.String())
		})
		option.SchemaVersion, option.BriefFile, option.ExecutionBindings = "2", "", payload
	}
	// No authority-level executor can accidentally satisfy a new binding.
	e.Config.Configuration.Executors = map[string]ExecutorConfig{}
	writeRuntimeJSON(t, filepath.Join(e.Root, "prifly.json"), e.Config)
	configBefore, err := os.ReadFile(filepath.Join(e.Root, "prifly.json"))
	if err != nil {
		t.Fatal(err)
	}
	_, executionRegistry, err := e.Inventory()
	if err != nil {
		t.Fatal(err)
	}
	for i, option := range options {
		if err := e.ValidateExecutionBindings(plans[i], definitions, executionRegistry, option.ExecutionBindings); err != nil {
			t.Fatal(err)
		}
		preview, err := e.Preview(PreviewOptions{SchemaVersion: "2", WorkflowFile: option.WorkflowFile, ExecutionBindings: option.ExecutionBindings})
		if err != nil || len(preview.CheckExecutors) != 2 || preview.Executors["work"].Executable == "" {
			t.Fatalf("preview lost exact step/check bindings: %+v %v", preview.CheckExecutors, err)
		}
	}
	clone := func() *ExecutionBindings {
		data, _ := canonical(firstOptions.ExecutionBindings)
		var payload ExecutionBindings
		if err := json.Unmarshal(data, &payload); err != nil {
			t.Fatal(err)
		}
		return &payload
	}
	for _, test := range []struct {
		name, code string
		edit       func(*ExecutionBindings)
	}{
		{"foreign", "execution_binding_outside_closure", func(p *ExecutionBindings) { p.Bindings[0].DefinitionRef.Version = "9.0.0" }},
		{"duplicate", "execution_bindings_invalid", func(p *ExecutionBindings) { p.Bindings = append(p.Bindings, p.Bindings[0]) }},
		{"missing-check", "missing_executor", func(p *ExecutionBindings) { p.Bindings = p.Bindings[1:] }},
		{"unsafe-source", "execution_bindings_invalid", func(p *ExecutionBindings) {
			p.Bindings[0].Config.Files = map[string]string{"data": "../escape"}
			p.Bindings[0].Files = map[string][]byte{"../escape": []byte("x")}
		}},
		{"extra-bytes", "execution_bindings_invalid", func(p *ExecutionBindings) { p.Bindings[0].Files["unselected"] = []byte("x") }},
		{"missing-bytes", "execution_bindings_invalid", func(p *ExecutionBindings) { p.Bindings[0].Config.Files = map[string]string{"data": "missing"} }},
		{"reserved-environment", "execution_bindings_invalid", func(p *ExecutionBindings) { p.Bindings[0].Config.Environment["PRIFLY_SOCKET"] = "/tmp/other" }},
		{"nul-argument", "execution_bindings_invalid", func(p *ExecutionBindings) { p.Bindings[0].Config.Args = []string{"unsafe\x00argument"} }},
		{"missing-context", "missing_context_profile", func(p *ExecutionBindings) {
			ref := firstRef
			ref.Version = "9.0.0"
			p.Bindings[0].Config.ContextProfileRef = &ref
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			payload := clone()
			test.edit(payload)
			err := e.ValidateExecutionBindings(plans[0], definitions, executionRegistry, payload)
			problem, _ := ProblemFor(err)
			if err == nil || problem.Code != test.code {
				t.Fatalf("wanted %s, got %v", test.code, err)
			}
		})
	}
	t.Run("assisted-executor", func(t *testing.T) {
		plan := *plans[0]
		plan.Steps = maps.Clone(plan.Steps)
		step := plan.Steps["work"]
		step.Executor.AdapterRef, step.Executor.Operation = assistedAdapter(definitions), "session"
		plan.Steps["work"] = step
		_, err := e.resolveExecutionBindings(&plan, definitions, clone())
		problem, _ := ProblemFor(err)
		if err == nil || problem.Code != "execution_binding_unsupported" {
			t.Fatalf("assisted binding accepted or misclassified: %v", err)
		}
	})
	legacy := firstOptions
	legacy.SchemaVersion, legacy.BriefFile = "", "brief.json"
	if _, err := e.Start(context.Background(), legacy); err == nil {
		t.Fatal("old Start accepted new execution payload")
	}
	started, err := e.Start(context.Background(), firstOptions)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := e.Start(context.Background(), firstOptions)
	if err != nil || !duplicate.Duplicate {
		t.Fatal("same binding changed dedup", err)
	}
	changed := firstOptions
	changed.ExecutionBindings = clone()
	changed.ExecutionBindings.Bindings[0].Config.Args = append(changed.ExecutionBindings.Bindings[0].Config.Args, "changed")
	if _, err := e.Start(context.Background(), changed); !errors.Is(err, local.ErrCommandConflict) {
		t.Fatalf("binding change lost command conflict: %v", err)
	}
	changed.ExecutionBindings = clone()
	for i := range changed.ExecutionBindings.Bindings {
		if len(changed.ExecutionBindings.Bindings[i].Files) > 0 {
			changed.ExecutionBindings.Bindings[i].Files["support/worker.data"] = []byte("changed source bytes")
		}
	}
	if _, err := e.Start(context.Background(), changed); !errors.Is(err, local.ErrCommandConflict) {
		t.Fatalf("supporting bytes escaped command dedup: %v", err)
	}
	second, err := e.Start(context.Background(), secondOptions)
	if err != nil {
		t.Fatal(err)
	}
	before := driverRun(t, e, started.Receipt.RunID)
	beforeConfig, _ := canonical(before.Executors)
	for _, option := range options {
		for i := range option.ExecutionBindings.Bindings {
			if len(option.ExecutionBindings.Bindings[i].Files) > 0 {
				option.ExecutionBindings.Bindings[i].Files["support/worker.data"][0] = 'X'
			}
		}
	}
	root := e.Root
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	e, err = Open(root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	for i, runID := range []string{started.Receipt.RunID, second.Receipt.RunID} {
		if err := e.Drive(context.Background(), runID); err != nil {
			t.Fatal(err)
		}
		r := driverRun(t, e, runID)
		if r.Status != "completed" || r.Outcome == nil || *r.Outcome != "succeeded" || len(r.CheckExecutions) != 2 || len(r.Executors) != 3 {
			t.Fatalf("run/checks did not complete: status=%s checks=%d diagnostics=%+v", r.Status, len(r.CheckExecutions), r.Diagnostics)
		}
		ref, expectedFile, expectedOutput := firstRef, "pinned first", "accepted output\n"
		if i == 1 {
			ref, expectedFile, expectedOutput = secondRef, "pinned second", "pinned input\n"+step.ID
		}
		pinned, exists := r.Executors[ref.String()]
		if !exists || pinned.ContextProfile == nil {
			t.Fatal("exact version or context pin disappeared")
		}
		for _, attempt := range r.Attempts {
			data, err := os.ReadFile(filepath.Join(attempt.Workspace, "worker.data"))
			if err != nil || string(data) != expectedFile {
				t.Fatalf("supporting bytes were reread after restart: %q %v", data, err)
			}
		}
		_, output, err := e.Artifact(r.Outputs["report"])
		if err != nil || string(output) != expectedOutput {
			t.Fatalf("wrong program output %q: %v", output, err)
		}
		if i == 0 {
			afterConfig, _ := canonical(r.Executors)
			if !bytes.Equal(beforeConfig, afterConfig) {
				t.Fatal("second Run or restart changed first executors")
			}
		} else if pinned.Config.Executable != secondProgram {
			t.Fatal("second Run used first executable")
		}
	}
	configAfter, err := os.ReadFile(filepath.Join(root, "prifly.json"))
	if err != nil || !bytes.Equal(configBefore, configAfter) || len(e.Config.Configuration.Executors) != 0 {
		t.Fatal("launch rewrote authority configuration", err)
	}
}

func TestExecutionBindingsClosedPayload(t *testing.T) {
	valid := []byte(`{"schema_version":"execution-bindings/1","bindings":[]}`)
	if err := ValidateExecutionBindingsPayload(valid); err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{
		[]byte(`{"schema_version":"execution-bindings/1","bindings":[],"effects":"external_write"}`),
		[]byte(`{"schema_version":"execution-bindings/2","bindings":[]}`),
		[]byte(`{"schema_version":"execution-bindings/1","bindings":null}`),
	} {
		if err := ValidateExecutionBindingsPayload(data); err == nil {
			t.Fatalf("accepted unknown payload: %s", data)
		}
	}
	if _, err := PublicSchema("ExecutionBindings"); err != nil {
		t.Fatal(err)
	}
}
