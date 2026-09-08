package runtime

import (
	"context"
	"os"
	"path/filepath"
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
