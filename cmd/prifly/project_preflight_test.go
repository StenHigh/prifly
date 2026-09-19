package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A preflight is the last thing that runs before anything is taken, and its
// timeout is the promise that it cannot hold a launch open. exec.CommandContext
// kills the program it started and nothing else: a script that leaves a child
// behind leaves that child holding the output pipe, and Run waits for the pipe,
// not for the program. The refusal then arrives when the grandchild feels like
// it — here thirty seconds after a five-hundred-millisecond deadline — and the
// grandchild is still running when it does.
func TestPreflightTimeoutEndsTheProgramGroupAndDoesNotWaitOnIt(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	root, authority := t.TempDir(), filepath.Join(t.TempDir(), "authority")
	if code, _, stderr := runCLI(t, "project", "init", "--repository", root, "--state-root", authority, "--host", "codex-cli"); code != 0 {
		t.Fatalf("init: %d %s", code, stderr)
	}
	if code, _, stderr := runCLI(t, "project", "local", "set", "--repository", root, "--allow-executable", "shell=/bin/sh"); code != 0 {
		t.Fatalf("allow: %d %s", code, stderr)
	}
	pidFile := filepath.Join(root, "grandchild.pid")
	script := "#!/bin/sh\n/bin/sleep 30 &\necho $! > '" + pidFile + "'\n/bin/sleep 30\n"
	if err := os.WriteFile(filepath.Join(root, "preflight.sh"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	preflight := &projectLaunchPreflight{Executable: "shell", Args: []string{"preflight.sh"}, TimeoutMS: 500}
	started := time.Now()
	err := runProjectLaunchPreflight(context.Background(), root, "inspect", preflight)
	elapsed := time.Since(started)
	if err == nil || !strings.Contains(err.Error(), "project_start_preflight_timeout") {
		t.Fatalf("a preflight past its deadline was not refused as a timeout: %v", err)
	}
	// Generous next to the thirty seconds the grandchild sleeps, tight next to
	// the half second the launch declared.
	if elapsed > 5*time.Second {
		t.Fatalf("the refusal waited %s on a program whose deadline was 500ms", elapsed)
	}
	raw, readErr := os.ReadFile(pidFile)
	if readErr != nil {
		t.Fatalf("the fixture recorded no grandchild: %v", readErr)
	}
	pid, convErr := strconv.Atoi(strings.TrimSpace(string(raw)))
	if convErr != nil || pid <= 0 {
		t.Fatalf("the fixture recorded %q as a pid: %v", raw, convErr)
	}
	// Signal 0 asks only whether the process is still there.
	if alive := syscall.Kill(pid, 0); alive == nil {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		_ = syscall.Kill(pid, syscall.SIGKILL)
		t.Fatalf("the preflight's grandchild %d outlived the launch that started it", pid)
	}
}
