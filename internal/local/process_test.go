//go:build darwin || linux

package local

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func processTestSpec(t *testing.T, mode string) ProcessSpec {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	digest, err := ProcessExecutableDigest(executable)
	if err != nil {
		t.Fatal(err)
	}
	return ProcessSpec{
		Executable: executable, ExecutableDigest: digest, Args: []string{"-test.run=^TestProcessHelper$"}, Dir: t.TempDir(),
		Env:        map[string]string{"PRIFLY_PROCESS_TEST": mode, "GORACE": "atexit_sleep_ms=0"},
		Envelope:   []byte(`{"schema_version":"1","run_id":"run_test","step_instance_id":"step_test","attempt_id":"attempt_test"}`),
		MaxRuntime: 5 * time.Second, GracePeriod: 100 * time.Millisecond, KillWait: 2 * time.Second,
		MaxStdoutBytes: 4096, MaxStderrBytes: 4096, MaxResultBytes: 4096,
	}
}

func requireSettledProcess(t *testing.T, out ProcessOutcome, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("RunProcess: %v; outcome: %+v", err, out)
	}
	if !out.Started || !out.WaitReturned || !out.GroupEmpty || out.Uncertain {
		t.Fatalf("unsettled process: %+v", out)
	}
	if out.CPU == nil || out.CPU.UserNS < 0 || out.CPU.SystemNS < 0 || out.CPU.Coverage != "may_include_reaped_children_not_complete_group" {
		t.Fatalf("dishonest/missing resource evidence: %+v", out.CPU)
	}
	if out.Identity.StartID == "" || out.Identity.BootID == "" || out.Identity.OwnerStart == "" || out.Identity.LaunchID == "" {
		t.Fatalf("incomplete process identity: %+v", out.Identity)
	}
	if out.LeaderExitObservedAt.IsZero() || out.WaitReturnedAt.Before(out.LeaderExitObservedAt) || out.GroupEmptyAt.Before(out.WaitReturnedAt) {
		t.Fatalf("invalid observed boundary ordering: %+v", out)
	}
}

func TestProcessNativeIdentity(t *testing.T) {
	identity, err := readProcess(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if identity.PID != os.Getpid() || identity.PPID != os.Getppid() || identity.UID != os.Getuid() || identity.StartID == "" || identity.Zombie {
		t.Fatalf("bad native identity: %+v", identity)
	}
	// A container runner may place its own test process in group 1. Only
	// RunProcess-launched children need a dedicated, signal-safe group.
	if identity.PGID <= 1 {
		return
	}
	group, err := readProcessGroup(identity.PGID)
	if err != nil {
		t.Fatal(err)
	}
	for _, member := range group {
		if member.PID == identity.PID {
			return
		}
	}
	t.Fatal("native group snapshot omitted current process")
}

func TestProcessWireEnvironmentAndArgv(t *testing.T) {
	t.Setenv("PRIFLY_PARENT_SECRET", "do-not-inherit-this")
	spec := processTestSpec(t, "success")
	literal := "$(touch forbidden); `echo also-forbidden` 'quoted'"
	spec.Args = append(spec.Args, "--", literal)
	spec.Env["PRIFLY_EXPLICIT_VALUE"] = literal
	var events []ProcessObservation
	out, err := RunProcess(context.Background(), spec, func(o ProcessObservation) error {
		events = append(events, o)
		if o.Kind == "start_returned" {
			probe := ProbeProcess(o.Identity)
			if probe.State != "present" {
				return fmt.Errorf("live process probe: %+v", probe)
			}
		}
		return nil
	})
	requireSettledProcess(t, out, err)
	if out.ExitCode == nil || *out.ExitCode != 0 || len(out.ResultCandidates) != 1 || out.ResultError != "" {
		t.Fatalf("wire result lost: %+v", out)
	}
	if !bytes.Equal(out.Stdout.Raw, append(bytes.Clone(spec.Envelope), []byte("\n"+literal+"\n"+literal)...)) || string(out.Stderr.Raw) != "plain diagnostic, ERROR is not a verdict" {
		t.Fatalf("stdio or argv changed: stdout=%q stderr=%q", out.Stdout.Raw, out.Stderr.Raw)
	}
	if !out.Stdout.Complete || !out.Stderr.Complete || out.Stdout.Truncated || out.Stderr.Truncated {
		t.Fatalf("diagnostic coverage: %+v %+v", out.Stdout, out.Stderr)
	}
	encoded, err := json.Marshal(out)
	if err != nil || bytes.Contains(encoded, []byte(literal)) || bytes.Contains(encoded, []byte("plain diagnostic")) {
		t.Fatal("raw diagnostic bytes leaked into default persistence encoding")
	}
	if len(events) < 5 || events[0].Kind != "start_returned" {
		t.Fatalf("missing observations: %+v", events)
	}
	probe := ProbeProcess(out.Identity)
	if probe.State != "absent" && probe.State != "mismatch" {
		t.Fatalf("completed process probe: %+v", probe)
	}
}

func TestProcessEarlyResultDoesNotReleaseProcess(t *testing.T) {
	spec := processTestSpec(t, "early")
	var resultAt time.Time
	out, err := RunProcess(context.Background(), spec, func(o ProcessObservation) error {
		if o.Kind == "result_candidate" {
			resultAt = time.Now()
			if probe := ProbeProcess(o.Identity); probe.State != "present" || probe.GroupAlive == nil || !*probe.GroupAlive {
				return fmt.Errorf("early result falsely settled live process: %+v", probe)
			}
		}
		return nil
	})
	requireSettledProcess(t, out, err)
	if resultAt.IsZero() || out.WaitReturnedAt.Sub(resultAt) < 200*time.Millisecond {
		t.Fatalf("result did not arrive while process was live: result=%v wait=%v", resultAt, out.WaitReturnedAt)
	}
}

func TestProcessWaitsForChildAfterParentExitAndClosedPipes(t *testing.T) {
	spec := processTestSpec(t, "orphan")
	out, err := RunProcess(context.Background(), spec, nil)
	requireSettledProcess(t, out, err)
	if out.WaitReturnedAt.Sub(out.LeaderExitObservedAt) < 150*time.Millisecond {
		t.Fatalf("driver released slot on parent exit: %+v", out)
	}
	if _, err := os.Stat(filepath.Join(spec.Dir, "child-finished")); err != nil {
		t.Fatal("live child was not awaited", err)
	}
}

func TestProcessCancellationEscalatesWholeGroup(t *testing.T) {
	for _, mode := range []string{"term", "ignore-term-tree"} {
		t.Run(mode, func(t *testing.T) {
			spec := processTestSpec(t, mode)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var events []string
			out, err := RunProcess(ctx, spec, func(o ProcessObservation) error {
				events = append(events, o.Kind)
				if o.Kind == "result_candidate" {
					cancel() // the helper installs its signal handler before this candidate
				}
				return nil
			})
			requireSettledProcess(t, out, err)
			joined := strings.Join(events, " ")
			if out.StopReason != "cancelled" || !strings.Contains(joined, "term_sent") {
				t.Fatalf("cancellation was not observed: %+v %s", out, joined)
			}
			if mode == "ignore-term-tree" {
				if !strings.Contains(joined, "kill_sent") || out.Signal != "killed" {
					t.Fatalf("SIGKILL escalation missing: %+v %s", out, joined)
				}
			} else if strings.Contains(joined, "kill_sent") {
				t.Fatal("graceful TERM unnecessarily escalated")
			}
		})
	}
}

func TestProcessRuntimeAndOutputLimits(t *testing.T) {
	for _, mode := range []string{"runtime", "stdout", "stderr", "result-overflow", "trailing", "malformed"} {
		t.Run(mode, func(t *testing.T) {
			spec := processTestSpec(t, mode)
			if mode == "runtime" {
				spec.MaxRuntime = 200 * time.Millisecond
			}
			out, err := RunProcess(context.Background(), spec, nil)
			requireSettledProcess(t, out, err)
			if out.StopReason == "" && out.ResultError == "" {
				t.Fatalf("limit or malformed channel silently accepted: %+v", out)
			}
			if len(out.Stdout.Raw) > int(spec.MaxStdoutBytes) || len(out.Stderr.Raw) > int(spec.MaxStderrBytes) || out.ResultBytes > spec.MaxResultBytes+1 {
				t.Fatal("bounded capture exceeded configured memory limit")
			}
			if mode == "runtime" && out.StopReason != "runtime_limit" {
				t.Fatalf("runtime limit missing: %+v", out)
			}
			if mode == "result-overflow" && out.ResultError != "result_limit" {
				t.Fatalf("result limit missing: %+v", out)
			}
		})
	}
}

func TestProcessExitIsEvidenceNotVerdict(t *testing.T) {
	spec := processTestSpec(t, "exit-error")
	out, err := RunProcess(context.Background(), spec, nil)
	requireSettledProcess(t, out, err)
	if out.ExitCode == nil || *out.ExitCode != 9 || len(out.ResultCandidates) != 0 {
		t.Fatalf("unexpected terminal evidence: %+v", out)
	}
	duplicate := processTestSpec(t, "duplicate")
	out, err = RunProcess(context.Background(), duplicate, nil)
	requireSettledProcess(t, out, err)
	if len(out.ResultCandidates) != 2 || !bytes.Equal(out.ResultCandidates[0], out.ResultCandidates[1]) {
		t.Fatal("identical candidate observations were lost")
	}
}

func TestProcessRefusesBeforeSpawnAndCleansObservationFailure(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "not-an-executable")
	if err := syscall.Mkfifo(fifo, 0700); err != nil {
		t.Fatal(err)
	}
	digestDone := make(chan error, 1)
	go func() { _, err := ProcessExecutableDigest(fifo); digestDone <- err }()
	select {
	case err := <-digestDone:
		if err == nil {
			t.Fatal("FIFO accepted as an executable")
		}
	case <-time.After(time.Second):
		t.Fatal("invalid executable blocked before regular-file validation")
	}
	for _, mode := range []string{"digest", "relative", "environment", "cancelled", "limit", "envelope", "before-start", "cancel-during-preflight"} {
		t.Run(mode, func(t *testing.T) {
			spec := processTestSpec(t, "success")
			ctx := context.Background()
			switch mode {
			case "digest":
				spec.ExecutableDigest = "sha256:" + strings.Repeat("0", 64)
			case "relative":
				spec.Executable = "./test"
			case "environment":
				spec.Env["PRIFLY_RESULT_FD"] = "1"
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "limit":
				spec.MaxStdoutBytes = 0
			case "envelope":
				spec.Envelope = []byte("null")
			case "before-start":
				spec.BeforeStart = func() error { return errors.New("admitted deadline expired during preparation") }
			case "cancel-during-preflight":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				spec.BeforeStart = func() error { cancel(); return nil }
			}
			out, err := RunProcess(ctx, spec, nil)
			if err == nil || out.Started {
				t.Fatalf("invalid launch was admitted: %+v %v", out, err)
			}
		})
	}
	spec := processTestSpec(t, "runtime")
	out, err := RunProcess(context.Background(), spec, func(o ProcessObservation) error { return errors.New("durable observation failed") })
	if err == nil || !out.WaitReturned || !out.GroupEmpty || out.StopReason != "observation_failure" {
		t.Fatalf("observation failure abandoned active process: %+v %v", out, err)
	}
}

func TestProcessProbeNeverAuthorizesRecoveredIdentity(t *testing.T) {
	if got := ProbeProcess(ProcessIdentity{PID: os.Getpid()}); got.State != "unknown" {
		t.Fatalf("PID alone accepted: %+v", got)
	}
	spec := processTestSpec(t, "early")
	out, err := RunProcess(context.Background(), spec, func(o ProcessObservation) error {
		if o.Kind == "result_candidate" {
			wrong := o.Identity
			wrong.StartID += "-wrong"
			if got := ProbeProcess(wrong); got.State != "mismatch" {
				return fmt.Errorf("wrong birth accepted: %+v", got)
			}
			wrong = o.Identity
			wrong.BootID += "-wrong"
			if got := ProbeProcess(wrong); got.State != "mismatch" {
				return fmt.Errorf("wrong boot accepted: %+v", got)
			}
		}
		return nil
	})
	requireSettledProcess(t, out, err)
}

// TestProcessHelper is a real child process, not a fake runner. Its deliberately
// small envelope omits core-only fields: full protocol/schema validation belongs
// to the runtime tests, while this package verifies byte transport and OS facts.
func TestProcessHelper(t *testing.T) {
	mode := os.Getenv("PRIFLY_PROCESS_TEST")
	if mode == "" {
		return
	}
	if mode == "linger" {
		time.Sleep(350 * time.Millisecond)
		_ = os.WriteFile("child-finished", []byte("done"), 0600)
		os.Exit(0)
	}
	if mode == "ignore-term-child" {
		signal.Ignore(syscall.SIGTERM)
		_ = os.WriteFile("child-ready", []byte(strconv.Itoa(os.Getpid())), 0600)
		for {
			time.Sleep(10 * time.Second)
		}
	}
	envelope, err := io.ReadAll(os.Stdin)
	if err != nil || os.Getenv("PRIFLY_PARENT_SECRET") != "" || os.Getenv("PRIFLY_RESULT_FD") != "3" {
		os.Exit(91)
	}
	fd := os.NewFile(3, "result")
	result := []byte(`{"schema_version":"1","run_id":"run_test","step_instance_id":"step_test","attempt_id":"attempt_test","envelope_digest":"` + os.Getenv("PRIFLY_ENVELOPE_DIGEST") + `","verdict":"pass","outputs":{},"evidence_refs":[],"effect_receipt_refs":[],"summary":""}`)
	writeResult := func() { _, _ = fd.Write(append(bytes.Clone(result), '\n')) }
	switch mode {
	case "success":
		_, _ = os.Stdout.Write(envelope)
		_, _ = fmt.Fprint(os.Stdout, "\n"+os.Getenv("PRIFLY_EXPLICIT_VALUE")+"\n"+os.Args[len(os.Args)-1])
		_, _ = fmt.Fprint(os.Stderr, "plain diagnostic, ERROR is not a verdict")
		writeResult()
	case "early":
		writeResult()
		time.Sleep(350 * time.Millisecond)
	case "orphan", "ignore-term-tree":
		childMode := "linger"
		if mode == "ignore-term-tree" {
			childMode = "ignore-term-child"
			signal.Ignore(syscall.SIGTERM)
		}
		child := exec.Command(os.Args[0], "-test.run=^TestProcessHelper$")
		child.Env = []string{"PRIFLY_PROCESS_TEST=" + childMode, "GORACE=atexit_sleep_ms=0"}
		if err := child.Start(); err != nil {
			os.Exit(92)
		}
		if mode == "ignore-term-tree" {
			deadline := time.Now().Add(2 * time.Second)
			for time.Now().Before(deadline) {
				if _, err := os.Stat("child-ready"); err == nil {
					break
				}
				time.Sleep(time.Millisecond)
			}
		}
		writeResult()
		if mode == "ignore-term-tree" {
			_ = child.Wait()
		}
	case "term":
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGTERM)
		writeResult()
		<-signals
	case "runtime":
		time.Sleep(10 * time.Second)
	case "stdout", "stderr":
		writer := os.Stdout
		if mode == "stderr" {
			writer = os.Stderr
		}
		for {
			_, _ = writer.Write(bytes.Repeat([]byte("x"), 16384))
		}
	case "result-overflow":
		_, _ = fmt.Fprint(fd, `{"summary":"`+strings.Repeat("x", 16384)+`"}`)
	case "trailing":
		writeResult()
		_, _ = fmt.Fprint(fd, "not-json")
	case "malformed":
		_, _ = fmt.Fprint(fd, `{"verdict":`)
	case "exit-error":
		os.Exit(9)
	case "duplicate":
		writeResult()
		writeResult()
	default:
		os.Exit(93)
	}
	_ = fd.Close()
	os.Exit(0)
}

// Hashing an executable is remembered for the life of the process, and a file
// that changed is a different file: it is hashed again and reports its new
// digest, so a pinned digest still refuses a swapped binary.
func TestExecutableDigestIsRememberedUntilTheFileChanges(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "worker")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	first, err := ProcessExecutableDigest(path)
	if err != nil {
		t.Fatal(err)
	}
	again, err := ProcessExecutableDigest(path)
	if err != nil || again != first {
		t.Fatalf("an unchanged executable reported a different digest: %q %q %v", first, again, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if identity, ok := executableIdentity(info); !ok || identity == "" {
		t.Fatal("the executable has no addressable identity")
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	// A same-size replacement within one filesystem timestamp tick is exactly
	// the case a naive cache would miss, so the file is given a new timestamp.
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	changed, err := ProcessExecutableDigest(path)
	if err != nil {
		t.Fatal(err)
	}
	if changed == first {
		t.Fatal("a replaced executable kept the digest of the file it replaced")
	}
}
