package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stenhigh/prifly/internal/flow"
)

func TestMonitorBoardExportFixtures(t *testing.T) {
	t.Parallel()
	target := os.Getenv("PRIFLY_MONITOR_FIXTURE_DIR")
	if target == "" {
		t.Skip("used by test/e2e/verify-monitor-board.py")
	}
	ctx := context.Background()
	report := map[string]any{}
	save := func(name string, e *Engine, ids map[string]string) {
		t.Helper()
		for label, id := range ids {
			view, err := e.MonitorSummary(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			report[label] = view
		}
		if err := e.Close(); err != nil {
			t.Fatal(err)
		}
		if err := monitorCopyAuthority(filepath.Join(target, name), e.Root); err != nil {
			t.Fatal(err)
		}
	}
	e, _, options := timedFixture(t, flow.SessionLimits{}, "none")
	body, err := os.ReadFile(filepath.Join(e.Root, options.WorkflowFile))
	var child map[string]any
	if err != nil || json.Unmarshal(body, &child) != nil {
		t.Fatal(err)
	}
	child["id"], child["version"] = "test:workflow/board-child", "1.0.0"
	ref := callRegister(t, e, child, "workflows/board-child.json")
	parent := callClone(t, child)
	parent["id"] = "test:workflow/board-parallel"
	definitions, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	parent["policy_ref"] = builtinVersionRef(definitions, "core:policy/local", "3.0.0")
	parent["limits"] = map[string]any{"max_step_instances": 4, "max_control_transitions": 64, "max_parallelism": 2, "max_child_depth": 4}
	parent["definition"] = map[string]any{"entry": "fan", "stages": map[string]any{
		"fan": map[string]any{"kind": "parallel", "max_parallelism": 2, "branches": []any{
			map[string]any{"id": "reader-left", "workflow_ref": ref, "input_bindings": map[string]any{}},
			map[string]any{"id": "reader-right", "workflow_ref": ref, "input_bindings": map[string]any{}},
		}, "join": joinAll("succeeded"), "on": map[string]any{"satisfied": "done", "unsatisfied": "done"}}, "done": choiceFinish("succeeded"),
	}}
	options.WorkflowFile, options.CommandID = "workflows/board-parallel.json", newID("command")
	writeRuntimeJSON(t, filepath.Join(e.Root, options.WorkflowFile), parent)
	if _, err = e.SetAdmissionCapacity(ctx, CapacityRequest{CommandID: newID("command"), Capacity: 2, Reason: "two fixture branches"}); err != nil {
		t.Fatal(err)
	}
	started, err := e.Start(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Drive(ctx, started.Receipt.RunID); err != nil {
		t.Fatal(err)
	}
	tasks, err := e.SessionTasks(ctx, started.Receipt.RunID)
	if err != nil || len(tasks) != 2 {
		t.Fatalf("two real branches: %v %v", tasks, err)
	}
	parkTimed(t, e, tasks[0])
	save("parallel", e, map[string]string{"parallel": started.Receipt.RunID})

	e, _, claim := assistedWorkspaceFixture(t, "worktree")
	if _, err = e.SetAdmissionCapacity(ctx, CapacityRequest{CommandID: newID("command"), Capacity: 4, Reason: "resource fixture"}); err != nil {
		t.Fatal(err)
	}
	start := func() string {
		t.Helper()
		own, err := e.ClaimWorktree(ctx, ClaimRequest{CommandID: newID("command"), Repository: claim.Repository.Toplevel, OwnerID: newID("session"), WorkspaceMode: "worktree"})
		if err != nil {
			t.Fatal(err)
		}
		result, err := e.Start(ctx, StartOptions{CommandID: newID("command"), WorkflowFile: "workflows/pilot.json", BriefFile: "brief.json", Inputs: map[string]string{}, WorkspaceMode: "worktree", WorkspaceClaim: &own, StageResources: map[string][]string{"plan": {"heavy"}}, ResourceLimits: map[string]int64{"heavy": 1}})
		if err != nil {
			t.Fatal(err)
		}
		return result.Receipt.RunID
	}
	holder, waiting := start(), start()
	if err = e.Drive(ctx, holder); err != nil {
		t.Fatal(err)
	}
	if err = e.Drive(ctx, waiting); refusalCode(err) != "resource_busy" {
		t.Fatalf("resource fixture was not refused: %v", err)
	}
	save("resource", e, map[string]string{"holder": holder, "resource": waiting})

	for _, effect := range []string{"none", "workspace_write"} {
		e, id, _ := timedFixture(t, flow.SessionLimits{ActiveTimeoutMS: activeMS(200)}, effect)
		handOver(t, e, id)
		time.Sleep(220 * time.Millisecond)
		_ = e.Drive(ctx, id)
		name := "failed"
		if effect != "none" {
			name = "uncertain"
		}
		saved, err := e.MonitorSummary(ctx, id)
		if err != nil || saved.Status != name {
			t.Fatalf("expiry fixture %s: %+v %v", name, saved, err)
		}
		save(name, e, map[string]string{name: id})
	}
	body, err = json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(target, "runtime-fixtures.json"), body, 0600); err != nil {
		t.Fatal(err)
	}
}

// os.CopyFS intentionally adds write permissions. Authority databases must
// retain their private source mode when exported for read-only replay.
func monitorCopyAuthority(destination, source string) error {
	if err := os.CopyFS(destination, os.DirFS(source)); err != nil {
		return err
	}
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return os.Chmod(filepath.Join(destination, relative), info.Mode().Perm())
	})
}
