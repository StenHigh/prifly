//go:build darwin || linux

package local

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ProcessSpec is a trusted local execution, not a sandbox. The caller commits
// the dispatch boundary and owns the authority's execution slot before RunProcess.
// Every argument/script dependency and the ExecutionEnvelope must already be pinned.
// Children must stay in the new process group; detached descendants are forbidden.
type ProcessSpec struct {
	Executable       string
	ExecutableDigest string // sha256:<lowercase hex>, checked immediately before exec
	Args             []string
	Dir              string
	Env              map[string]string // an explicit allowlist, never os.Environ()
	Envelope         []byte            // exact, validated ExecutionEnvelope v1 bytes
	MaxRuntime       time.Duration
	GracePeriod      time.Duration
	KillWait         time.Duration
	MaxStdoutBytes   int64
	MaxStderrBytes   int64
	MaxResultBytes   int64 // aggregate bytes, including duplicate results/whitespace
	// BeforeStart rechecks the caller's admitted deadline/cancellation after
	// executable hashing and pipe setup. It must be bounded and cannot spawn.
	// It does not make the durable dispatch and OS start boundary atomic.
	BeforeStart func() error `json:"-"`
}

// ProcessIdentity binds an observation to an actual launch, not merely a PID.
// It is evidence for ProbeProcess, not a capability to signal a recovered process.
type ProcessIdentity struct {
	PID              int    `json:"pid"`
	PGID             int    `json:"pgid"`
	UID              int    `json:"uid"`
	OwnerPID         int    `json:"owner_pid"`
	OwnerStart       string `json:"owner_start"`
	Hostname         string `json:"hostname"`
	BootID           string `json:"boot_id"`
	StartID          string `json:"start_id"`
	LaunchID         string `json:"launch_id"`
	Executable       string `json:"executable"`
	ExecutableDigest string `json:"executable_digest"`
}

// ProcessObservation names the actual observed boundary. wait_returned is NOT
// an exact process exit timestamp. Result is an untrusted candidate; the callback
// must validate its closed StepResult schema and identity before durable acceptance.
// Callbacks are serialized and must be bounded; failure requests process shutdown.
type ProcessObservation struct {
	Kind     string          `json:"kind"`
	At       time.Time       `json:"at"`
	Elapsed  time.Duration   `json:"elapsed_ns"`
	Quality  string          `json:"quality"`
	Identity ProcessIdentity `json:"identity"`
	Reason   string          `json:"reason,omitempty"`
	Result   json.RawMessage `json:"-"`
	ExitCode *int            `json:"exit_code,omitempty"`
	Signal   string          `json:"signal,omitempty"`
	CPU      *ProcessCPU     `json:"cpu,omitempty"`
}

// ProcessCPU is the OS accounting attached to the waited-for child. Depending
// on the OS it may include descendants that child reaped, but never claims complete
// process-group coverage. Missing accounting is nil, not an invented zero.
type ProcessCPU struct {
	UserNS   int64  `json:"user_ns"`
	SystemNS int64  `json:"system_ns"`
	Method   string `json:"method"`
	Scope    string `json:"scope"`
	Coverage string `json:"coverage"`
}

// DiagnosticStream retains only bounded raw bytes in memory. Raw is deliberately
// absent from JSON: callers must apply their disclosure policy before persistence.
type DiagnosticStream struct {
	Raw       []byte `json:"-"`
	BytesRead int64  `json:"bytes_read"`
	Truncated bool   `json:"truncated"`
	Complete  bool   `json:"complete"`
}

type ProcessOutcome struct {
	Started              bool              `json:"started"`
	Identity             ProcessIdentity   `json:"identity"`
	StartedAt            time.Time         `json:"started_at"`
	LeaderExitObservedAt time.Time         `json:"leader_exit_observed_at"`
	WaitReturnedAt       time.Time         `json:"wait_returned_at"`
	GroupEmptyAt         time.Time         `json:"group_empty_at"`
	FinishedAt           time.Time         `json:"finished_at"`
	Elapsed              time.Duration     `json:"elapsed_ns"`
	WaitReturned         bool              `json:"wait_returned"`
	GroupEmpty           bool              `json:"group_empty"`
	Uncertain            bool              `json:"uncertain"`
	ExitCode             *int              `json:"exit_code"`
	Signal               string            `json:"signal,omitempty"`
	SignalErrors         []string          `json:"signal_errors,omitempty"`
	CPU                  *ProcessCPU       `json:"cpu"`
	StopReason           string            `json:"stop_reason,omitempty"`
	ResultError          string            `json:"result_error,omitempty"`
	ResultCandidates     []json.RawMessage `json:"-"`
	Stdout               DiagnosticStream  `json:"stdout"`
	Stderr               DiagnosticStream  `json:"stderr"`
	ResultBytes          int64             `json:"result_bytes"`
}

type ProcessProbe struct {
	State      string `json:"state"` // present, absent, mismatch, unknown
	Reason     string `json:"reason"`
	GroupAlive *bool  `json:"group_alive"` // nil when the group could not be observed
}

// ProbeProcess never signals or authorizes retry. Even an absent process cannot
// prove that its effects did not happen, or that all descendants disappeared.
func ProbeProcess(id ProcessIdentity) ProcessProbe {
	if id.PID <= 1 || id.PGID != id.PID || id.StartID == "" || id.BootID == "" || id.LaunchID == "" {
		return ProcessProbe{State: "unknown", Reason: "incomplete_process_identity"}
	}
	boot, err := processBootID()
	if err != nil {
		return ProcessProbe{State: "unknown", Reason: "boot_identity_unavailable"}
	}
	host, err := os.Hostname()
	if err != nil || boot != id.BootID || host != id.Hostname {
		return ProcessProbe{State: "mismatch", Reason: "host_or_boot_changed"}
	}
	p, err := readProcess(id.PID)
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ESRCH) {
		return ProcessProbe{State: "absent", Reason: "absence_does_not_resolve_effects"}
	}
	if err != nil {
		return ProcessProbe{State: "unknown", Reason: "process_identity_unavailable"}
	}
	if p.StartID != id.StartID || p.PGID != id.PGID || p.UID != id.UID {
		return ProcessProbe{State: "mismatch", Reason: "process_identity_changed"}
	}
	group, err := readProcessGroup(id.PGID)
	if err != nil {
		return ProcessProbe{State: "unknown", Reason: "group_observation_unavailable"}
	}
	alive := false
	for _, member := range group {
		alive = alive || !member.Zombie
	}
	return ProcessProbe{State: "present", Reason: "read_only_observation_not_signal_authority", GroupAlive: &alive}
}

// ProcessExecutableDigest hashes a regular executable. A live external binary is
// checked again at launch, but this cooperative profile does not defeat malicious
// mutation between hashing and exec or pin an interpreter's dynamic libraries.
// executableDigests remembers a hashed executable for the life of this process.
// One project commonly points many step definitions at the same binary, and
// hashing a 26 MiB executable once per definition was the largest single cost
// of starting a Run. The key is the file's identity and both timestamps, so a
// replaced or touched file is a different key and is hashed again.
var executableDigests sync.Map

func ProcessExecutableDigest(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", errors.New("executable must be absolute")
	}
	// O_NONBLOCK avoids hanging on an invalid FIFO/device before f.Stat can
	// reject it. It does not change reads from regular executable files.
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return "", err
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !before.Mode().IsRegular() || before.Mode().Perm()&0111 == 0 {
		return "", errors.New("executable must be a regular executable file")
	}
	identity, addressable := executableIdentity(before)
	if addressable {
		if cached, found := executableDigests.Load(identity); found {
			return cached.(string), nil
		}
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	after, err := f.Stat()
	if err != nil {
		return "", err
	}
	if before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return "", errors.New("executable changed while hashing")
	}
	digest := "sha256:" + hex.EncodeToString(h.Sum(nil))
	if identity, stillAddressable := executableIdentity(after); stillAddressable && addressable && identity != "" {
		executableDigests.Store(identity, digest)
	}
	return digest, nil
}

func validateProcessSpec(spec ProcessSpec) error {
	if !filepath.IsAbs(spec.Executable) || !filepath.IsAbs(spec.Dir) {
		return errors.New("executable and cwd must be absolute")
	}
	dir, err := os.Stat(spec.Dir)
	if err != nil || !dir.IsDir() {
		return errors.New("cwd must be an existing directory")
	}
	if spec.MaxRuntime <= 0 || spec.GracePeriod <= 0 || spec.KillWait <= 0 {
		return errors.New("runtime, TERM grace and KILL observation limits must be positive")
	}
	for _, limit := range []int64{spec.MaxStdoutBytes, spec.MaxStderrBytes, spec.MaxResultBytes} {
		if limit < 1 || limit > 16<<20 {
			return errors.New("each stream limit must be between 1 byte and 16 MiB")
		}
	}
	if len(spec.Envelope) == 0 || len(spec.Envelope) > 4<<20 || !json.Valid(spec.Envelope) || bytes.TrimSpace(spec.Envelope)[0] != '{' {
		return errors.New("envelope must be a JSON object no larger than 4 MiB")
	}
	for key, value := range spec.Env {
		if key == "" || strings.ContainsAny(key, "=\x00") || strings.ContainsRune(value, '\x00') {
			return errors.New("invalid environment entry")
		}
		if key == "PRIFLY_RESULT_FD" || key == "PRIFLY_ENVELOPE_DIGEST" || key == "PRIFLY_LAUNCH_ID" {
			return fmt.Errorf("reserved environment key %s", key)
		}
	}
	for _, arg := range spec.Args {
		if strings.ContainsRune(arg, '\x00') {
			return errors.New("argument contains NUL")
		}
	}
	return nil
}

type processRecord struct {
	PID, PPID, PGID, UID int
	StartID              string
	Zombie               bool
}

type processCapture struct {
	mu       sync.Mutex
	data     []byte
	count    int64
	limit    int64
	complete bool
	name     string
	overflow chan<- string
}

func (c *processCapture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	previous := c.count
	c.count += int64(len(p))
	if remaining := c.limit - int64(len(c.data)); remaining > 0 {
		c.data = append(c.data, p[:min(int64(len(p)), remaining)]...)
	}
	if previous <= c.limit && c.count > c.limit {
		select {
		case c.overflow <- c.name + "_limit":
		default:
		}
	}
	return len(p), nil
}

func (c *processCapture) snapshot() DiagnosticStream {
	c.mu.Lock()
	defer c.mu.Unlock()
	return DiagnosticStream{Raw: bytes.Clone(c.data), BytesRead: c.count, Truncated: c.count > c.limit, Complete: c.complete}
}

func (c *processCapture) read(r *os.File, done chan<- struct{}) {
	_, err := io.Copy(c, r)
	c.mu.Lock()
	c.complete = err == nil
	c.mu.Unlock()
	done <- struct{}{}
}

type processResultMessage struct {
	raw json.RawMessage
	err string
	at  time.Time
}

// RunProcess implements the fd protocol without adding fields to wire DTOs:
// stdin = exact ExecutionEnvelope; fd 3 = one or identical duplicate StepResult
// JSON objects; stdout/stderr = diagnostics. Root validates candidate equality.
// Return error describes driver/observation failure; nonzero worker exits are
// evidence in Outcome and do not manufacture a StepResult or verdict.
// The process group is polled closely while a step is young, then at a calmer
// interval. Termination is still bounded by its own timers, not by this rate.
const (
	processPollInterval   = 20 * time.Millisecond
	processPollCloseWatch = 200 * time.Millisecond
	processPollSettled    = 200 * time.Millisecond
)

func RunProcess(ctx context.Context, spec ProcessSpec, observe func(ProcessObservation) error) (out ProcessOutcome, retErr error) {
	if err := validateProcessSpec(spec); err != nil {
		return out, err
	}
	executable, err := filepath.EvalSymlinks(spec.Executable)
	if err != nil {
		return out, err
	}
	digest, err := ProcessExecutableDigest(executable)
	if err != nil {
		return out, err
	}
	if digest != spec.ExecutableDigest {
		return out, errors.New("pinned executable digest mismatch")
	}
	boot, err := processBootID()
	if err != nil {
		return out, fmt.Errorf("process boot identity: %w", err)
	}
	owner, err := readProcess(os.Getpid())
	if err != nil {
		return out, fmt.Errorf("process owner identity: %w", err)
	}
	host, err := os.Hostname()
	if err != nil {
		return out, err
	}
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return out, err
	}
	launch := hex.EncodeToString(token[:])
	var files []*os.File
	defer func() {
		for _, f := range files {
			_ = f.Close()
		}
	}()
	pipe := func() (*os.File, *os.File, error) {
		r, w, err := os.Pipe()
		if err == nil {
			files = append(files, r, w)
		}
		return r, w, err
	}
	stdinR, stdinW, err := pipe()
	if err != nil {
		return out, err
	}
	stdoutR, stdoutW, err := pipe()
	if err != nil {
		return out, err
	}
	stderrR, stderrW, err := pipe()
	if err != nil {
		return out, err
	}
	resultR, resultW, err := pipe()
	if err != nil {
		return out, err
	}
	cmd := exec.Command(executable, spec.Args...)
	cmd.Dir = spec.Dir
	cmd.Env = make([]string, 0, len(spec.Env)+3)
	for key, value := range spec.Env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	sort.Strings(cmd.Env)
	envelopeDigest := sha256.Sum256(spec.Envelope)
	cmd.Env = append(cmd.Env, "PRIFLY_RESULT_FD=3", "PRIFLY_ENVELOPE_DIGEST=sha256:"+hex.EncodeToString(envelopeDigest[:]), "PRIFLY_LAUNCH_ID="+launch)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdinR, stdoutW, stderrW
	cmd.ExtraFiles = []*os.File{resultW}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	if spec.BeforeStart != nil {
		if err := spec.BeforeStart(); err != nil {
			return out, err
		}
		if err := ctx.Err(); err != nil {
			return out, err
		}
	}
	startCall := time.Now()
	if err := cmd.Start(); err != nil {
		return out, err
	}
	out.Started = true
	out.StartedAt = time.Now()
	out.Identity = ProcessIdentity{PID: cmd.Process.Pid, PGID: cmd.Process.Pid, UID: os.Getuid(), OwnerPID: os.Getpid(), OwnerStart: owner.StartID, Hostname: host, BootID: boot, LaunchID: launch, Executable: executable, ExecutableDigest: digest}
	_ = stdinR.Close()
	_ = stdoutW.Close()
	_ = stderrW.Close()
	_ = resultW.Close()
	go func() {
		_, _ = io.Copy(stdinW, bytes.NewReader(spec.Envelope))
		_ = stdinW.Close()
	}()
	overflow := make(chan string, 3)
	streamDone := make(chan struct{}, 2)
	stdout := &processCapture{limit: spec.MaxStdoutBytes, name: "stdout", overflow: overflow}
	stderr := &processCapture{limit: spec.MaxStderrBytes, name: "stderr", overflow: overflow}
	go stdout.read(stdoutR, streamDone)
	go stderr.read(stderrR, streamDone)
	resultMessages := make(chan processResultMessage, 1)
	resultDone := make(chan int64, 1)
	resultStop := make(chan struct{})
	defer close(resultStop)
	resultReader := &io.LimitedReader{R: resultR, N: spec.MaxResultBytes + 1}
	go func() {
		defer func() { resultDone <- spec.MaxResultBytes + 1 - resultReader.N }()
		decoder := json.NewDecoder(resultReader)
		for count := 0; ; count++ {
			var raw json.RawMessage
			err := decoder.Decode(&raw)
			if err == io.EOF {
				return
			}
			message := processResultMessage{raw: raw, at: time.Now()}
			if resultReader.N == 0 {
				message.err = "result_limit"
			} else if err != nil || len(raw) == 0 || raw[0] != '{' {
				message.err = "invalid_result_json"
			} else if count >= 32 {
				message.err = "result_count_limit"
			}
			select {
			case resultMessages <- message:
			case <-resultStop:
				return
			}
			if message.err != "" {
				return
			}
		}
	}()
	// The leader remains unreaped while descendants may live. Its PID cannot be
	// reused as an unrelated process-group id during TERM/KILL escalation.
	leader, identityErr := readProcess(cmd.Process.Pid)
	if identityErr == nil && leader.PGID == out.Identity.PGID && leader.PPID == os.Getpid() {
		out.Identity.StartID = leader.StartID
		out.Identity.UID = leader.UID
	} else {
		out.Uncertain = true
		retErr = errors.New("spawned process identity could not be verified")
	}
	var observationErr error
	emit := func(kind, reason, quality string, at time.Time, raw json.RawMessage) {
		if observe == nil || observationErr != nil {
			return
		}
		err := observe(ProcessObservation{Kind: kind, At: at.UTC(), Elapsed: at.Sub(startCall), Quality: quality, Identity: out.Identity, Reason: reason, Result: bytes.Clone(raw), ExitCode: out.ExitCode, Signal: out.Signal, CPU: out.CPU})
		if err != nil {
			observationErr = err
			retErr = errors.Join(retErr, fmt.Errorf("persist process observation: %w", err))
		}
	}
	emit("start_returned", "", "observed", out.StartedAt, nil)
	runtimeLimit := time.NewTimer(spec.MaxRuntime - time.Since(startCall))
	defer runtimeLimit.Stop()
	// A step that finishes quickly is worth watching closely; one that runs for
	// minutes is not, and polling it every 20ms burned a measurable share of a
	// core reading /proc for a process nobody was waiting on that closely.
	ticker := time.NewTicker(processPollInterval)
	defer ticker.Stop()
	slowPoll := time.AfterFunc(processPollCloseWatch, func() { ticker.Reset(processPollSettled) })
	defer slowPoll.Stop()
	var stopAt time.Time
	var killAt time.Time
	signalGroup := func(sig syscall.Signal, phase, reason string, at time.Time) {
		err := syscall.Kill(-cmd.Process.Pid, sig)
		if err == nil {
			emit(phase+"_sent", reason, "observed_signal_request", at, nil)
		} else if errors.Is(err, syscall.ESRCH) {
			emit(phase+"_absent", reason, "observed", at, nil)
		} else {
			// macOS may return EPERM for a group containing only zombies. A failed
			// signal is recorded, not called successful; subsequent Wait and group
			// absence can still establish an unambiguous process termination.
			out.SignalErrors = append(out.SignalErrors, phase+": "+err.Error())
			emit(phase+"_failed", err.Error(), "observed", at, nil)
		}
	}
	startStop := func(reason string) {
		if !stopAt.IsZero() {
			return
		}
		stopAt = time.Now()
		out.StopReason = reason
		// Stopping is the one phase where the poll rate is a bound on behaviour:
		// the escalation from term to kill and the observation that the group is
		// gone both happen on this tick. The timer that slows the poll down has
		// to be stopped first: a step cancelled inside its first close-watch
		// window would otherwise be slowed back down right after this reset,
		// and a group that did exit on TERM would be escalated to KILL because
		// nothing looked in time.
		slowPoll.Stop()
		ticker.Reset(processPollInterval)
		// The unreaped Cmd is the authority here. Serialized identities recovered
		// after a crash never reach this signaling path.
		signalGroup(syscall.SIGTERM, "term", reason, stopAt)
	}
	if retErr != nil || observationErr != nil {
		startStop("observation_failure")
	}
	ctxDone := ctx.Done()
	resultsComplete := false
	streamsComplete := 0
	emptyScans := 0
	for {
		select {
		case <-ctxDone:
			ctxDone = nil
			startStop("cancelled")
		case <-runtimeLimit.C:
			startStop("runtime_limit")
		case reason := <-overflow:
			startStop(reason)
		case message := <-resultMessages:
			if message.err != "" {
				out.ResultError = message.err
				startStop(message.err)
			} else {
				out.ResultCandidates = append(out.ResultCandidates, bytes.Clone(message.raw))
				emit("result_candidate", "", "observed", message.at, message.raw)
				if observationErr != nil {
					startStop("observation_failure")
				}
			}
		case count := <-resultDone:
			out.ResultBytes = count
			resultsComplete = true
			if count > spec.MaxResultBytes {
				out.ResultError = "result_limit"
				startStop("result_limit")
			}
		case <-streamDone:
			streamsComplete++
		case <-ticker.C:
			group, groupErr := readProcessGroup(out.Identity.PGID)
			leaderNow, leaderErr := readProcess(out.Identity.PID)
			if leaderErr == nil && out.Identity.StartID != "" && (leaderNow.StartID != out.Identity.StartID || leaderNow.PPID != os.Getpid() || leaderNow.PGID != out.Identity.PGID) {
				// A cooperative child must not call setsid/setpgid or be reaped by
				// another owner. Never send signals to a now-unbound group.
				out.Uncertain = true
				retErr = errors.Join(retErr, errors.New("process containment identity changed"))
				goto uncertain
			}
			if leaderErr == nil && leaderNow.Zombie && out.LeaderExitObservedAt.IsZero() {
				out.LeaderExitObservedAt = time.Now()
				emit("leader_exit_observed", "", "sampled_upper_bound", out.LeaderExitObservedAt, nil)
			}
			if groupErr != nil || leaderErr != nil {
				out.Uncertain = true
				startStop("process_observation_unavailable")
				emptyScans = 0
			} else {
				live := !leaderNow.Zombie
				for _, member := range group {
					live = live || !member.Zombie
				}
				if live {
					emptyScans = 0
				} else {
					emptyScans++
				}
				if emptyScans >= 2 {
					goto reap
				}
			}
			if !stopAt.IsZero() && killAt.IsZero() && time.Since(stopAt) >= spec.GracePeriod {
				killAt = time.Now()
				signalGroup(syscall.SIGKILL, "kill", out.StopReason, killAt)
			}
			if !killAt.IsZero() && time.Since(killAt) >= spec.KillWait {
				out.Uncertain = true
				retErr = errors.Join(retErr, errors.New("process group did not reach observed quiescence"))
				goto uncertain
			}
		}
	}

reap:
	{
		waitErr := cmd.Wait()
		out.WaitReturned = true
		out.WaitReturnedAt = time.Now()
		if cmd.ProcessState == nil {
			out.Uncertain = true
			retErr = errors.Join(retErr, waitErr)
		} else {
			exit := cmd.ProcessState.ExitCode()
			out.ExitCode = &exit
			if status, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok && status.Signaled() {
				out.Signal = status.Signal().String()
			}
			out.CPU = &ProcessCPU{UserNS: cmd.ProcessState.UserTime().Nanoseconds(), SystemNS: cmd.ProcessState.SystemTime().Nanoseconds(), Method: "os_process_wait_rusage", Scope: "waited_child_os_accounting", Coverage: "may_include_reaped_children_not_complete_group"}
		}
		emit("wait_returned", "", "observed_after_group_quiescence", out.WaitReturnedAt, nil)
		// No signals are permitted after reaping the group leader. Confirm physical
		// group absence separately; a zombie or a PID reuse only delays/refuses success.
		deadline := time.NewTimer(spec.KillWait)
		defer deadline.Stop()
		for {
			if err := syscall.Kill(-out.Identity.PGID, 0); errors.Is(err, syscall.ESRCH) {
				out.GroupEmpty = true
				out.GroupEmptyAt = time.Now()
				emit("group_empty", "", "observed", out.GroupEmptyAt, nil)
				break
			}
			select {
			case <-deadline.C:
				out.Uncertain = true
				retErr = errors.Join(retErr, errors.New("process group absence not confirmed after reap"))
				goto drain
			case <-ticker.C:
			}
		}
		goto drain
	}

uncertain:
	// Reap eventually, without blocking recovery forever on an unkillable child.
	// No new ordinary work may follow Uncertain, even if this background wait ends.
	go func() { _ = cmd.Wait() }()

drain:
	{
		// Reading is bounded even if a prohibited detached descendant kept a pipe.
		timer := time.NewTimer(spec.KillWait)
		defer timer.Stop()
		for !resultsComplete || streamsComplete < 2 || len(resultMessages) > 0 {
			select {
			case message := <-resultMessages:
				if message.err != "" {
					out.ResultError = message.err
				} else {
					out.ResultCandidates = append(out.ResultCandidates, bytes.Clone(message.raw))
					emit("result_candidate", "", "observed", message.at, message.raw)
				}
			case count := <-resultDone:
				out.ResultBytes = count
				resultsComplete = true
				if count > spec.MaxResultBytes {
					out.ResultError = "result_limit"
				}
			case <-streamDone:
				streamsComplete++
			case <-timer.C:
				out.Uncertain = true
				retErr = errors.Join(retErr, errors.New("inherited process streams did not close"))
				goto finish
			}
		}
	}

finish:
	_ = stdoutR.Close()
	_ = stderrR.Close()
	_ = resultR.Close()
	_ = stdinW.Close()
	out.Stdout, out.Stderr = stdout.snapshot(), stderr.snapshot()
	if out.Stdout.Truncated && out.StopReason == "" {
		out.StopReason = "stdout_limit"
	}
	if out.Stderr.Truncated && out.StopReason == "" {
		out.StopReason = "stderr_limit"
	}
	out.FinishedAt = time.Now()
	out.Elapsed = out.FinishedAt.Sub(startCall)
	return out, retErr
}
