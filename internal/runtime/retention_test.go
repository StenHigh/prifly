package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanupRefusesUnfinishedAndUnresolved(t *testing.T) {
	for _, status := range []string{"running", "waiting", "pending", "uncertain", "stopping"} {
		if cleanupRefusal(Run{Status: status, Settled: &Observation{}}) == "" {
			t.Fatal("accepted", status)
		}
	}
	for _, run := range []Run{
		{Status: "completed"},
		{Status: "completed", Settled: &Observation{}, HasUnresolvedEffects: true},
		{Status: "cancelled", Settled: &Observation{}, Attempts: map[string]*Attempt{"a": {}}},
		{Status: "failed", Settled: &Observation{}, ResumeRequired: true},
	} {
		if cleanupRefusal(run) == "" {
			t.Fatal("accepted incomplete terminal Run")
		}
	}
	for _, status := range []string{"completed", "failed", "cancelled"} {
		if reason := cleanupRefusal(Run{Status: status, Settled: &Observation{}}); reason != "" {
			t.Fatal(status, reason)
		}
	}
}

func TestCleanupRemovesOnlyOwnedWorkspace(t *testing.T) {
	e, id := driverProject(t, "pass", 10000)
	ctx := context.Background()
	if err := e.Drive(ctx, id); err != nil {
		t.Fatal(err)
	}
	run := driverRun(t, e, id)
	if reason := cleanupRefusal(run); reason != "" {
		t.Fatal(reason, run.Status)
	}
	var workspace string
	for _, a := range run.Attempts {
		workspace = a.Workspace
	}
	outside := filepath.Join(t.TempDir(), "keep.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "external-link")); err != nil {
		t.Fatal(err)
	}
	root := e.Root
	e.Close()
	m, err := OpenMonitorMaintenance(root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	// A validation probe in metadata blocks even a single-Run cleanup. Name
	// the probe, keep the Run, and allow retry once probes leave metadata.
	probe := filepath.Join(root, ".prifly", "fp")
	if err := os.Mkdir(probe, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(probe, "symlinked.json")); err != nil {
		t.Fatal(err)
	}
	for _, selected := range []string{id, ""} {
		_, err := m.PreviewCleanup(ctx, selected)
		problem, _ := ProblemFor(err)
		if err == nil || problem.Code != "unsafe_path" || !strings.Contains(problem.Message, ".prifly/fp/symlinked.json") {
			t.Fatalf("cleanup must name the blocking metadata link: %+v (%v)", problem, err)
		}
	}
	if _, err := m.Cleanup(ctx, id, "unconfirmed"); err != nil {
		problem, _ := ProblemFor(err)
		if problem.Code != "unsafe_path" || !strings.Contains(problem.Message, ".prifly/fp/symlinked.json") {
			t.Fatalf("cleanup must recheck metadata before the digest: %+v", problem)
		}
	} else {
		t.Fatal("cleanup ignored the metadata link")
	}
	if kept := driverRun(t, m, id); kept.ID != id {
		t.Fatal("refused cleanup removed the Run")
	}
	if err := os.Rename(probe, filepath.Join(root, "fp")); err != nil {
		t.Fatal(err)
	}
	plan, err := m.PreviewCleanup(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(workspace, 0500); err != nil {
		t.Fatal(err)
	}
	result, err := m.Cleanup(ctx, id, plan.Digest)
	if err != nil || result.DeletedRuns != 1 || len(result.Warnings) == 0 {
		t.Fatal("partial cleanup was not reported", result, err)
	}
	if err = os.Chmod(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	plan, err = m.PreviewCleanup(ctx, "")
	if err != nil {
		t.Fatal("cannot recover orphan workspace", err)
	}
	result, err = m.Cleanup(ctx, "", plan.Digest)
	if err != nil || len(result.Warnings) > 0 {
		t.Fatal(result, err)
	}
	if _, err = os.Stat(workspace); !os.IsNotExist(err) {
		t.Fatal("workspace survived", err)
	}
	if data, err := os.ReadFile(outside); err != nil || string(data) != "outside" {
		t.Fatal("symlink target changed", err)
	}
	if err = m.Store.Verify(ctx); err != nil {
		t.Fatal(err)
	}
}
