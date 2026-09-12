package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// projectLaunchPreflight is a program the project runs before a launch takes
// anything: no package is imported, no workspace claimed, no Run created
// until it exits 0. It is the project's own declaration, reviewed in
// project.yaml, and its binary is the one the owner allowed in local.yaml, so
// it needs no --allow-execution; the program runs with the operator's
// environment, as the launcher it replaces did. project questionnaire
// --prepare stays read-only and does not run it.
type projectLaunchPreflight struct {
	Executable string   `json:"executable"`
	Args       []string `json:"args"`
	TimeoutMS  int64    `json:"timeout_ms"`
}

const (
	projectPreflightMaxTimeoutMS = 3600000
	// projectPreflightOutputTail is how much of the program's output a refusal
	// carries: enough to read the reason, not the whole log.
	projectPreflightOutputTail = 2000
	projectPreflightMaxOutput  = 1 << 20
)

func readProjectLaunchPreflight(id string, raw any) (*projectLaunchPreflight, error) {
	object, ok := raw.(map[string]any)
	if !ok || len(object) == 0 {
		return nil, usageError("project_profile_invalid: launch " + id + " preflight must be an object with executable and timeout_ms")
	}
	for key := range object {
		switch key {
		case "executable", "args", "timeout_ms":
		default:
			return nil, usageError("project_profile_invalid: unknown field in launch " + id + " preflight: " + key)
		}
	}
	result := &projectLaunchPreflight{Args: []string{}}
	executable, ok := object["executable"].(string)
	if !ok || !projectLaunchID(executable) {
		return nil, usageError("project_profile_invalid: launch " + id + " preflight executable must be a logical name allowed with project local set --allow-executable")
	}
	result.Executable = executable
	if raw, exists := object["args"]; exists {
		items, ok := raw.([]any)
		if !ok {
			return nil, usageError("project_profile_invalid: launch " + id + " preflight args must be a list of strings")
		}
		for _, item := range items {
			text, ok := item.(string)
			if !ok {
				return nil, usageError("project_profile_invalid: launch " + id + " preflight args must be a list of strings")
			}
			result.Args = append(result.Args, text)
		}
	}
	timeout, ok := object["timeout_ms"]
	if !ok {
		return nil, usageError("project_profile_invalid: launch " + id + " preflight requires timeout_ms")
	}
	// Parsed YAML carries numbers as json.Number; an integer written as a
	// float is refused rather than rounded.
	number, ok := timeout.(json.Number)
	if !ok {
		return nil, usageError("project_profile_invalid: launch " + id + " preflight timeout_ms must be an integer 1.." + strconv.Itoa(projectPreflightMaxTimeoutMS))
	}
	ms, err := number.Int64()
	if err != nil || ms < 1 || ms > projectPreflightMaxTimeoutMS {
		return nil, usageError("project_profile_invalid: launch " + id + " preflight timeout_ms must be an integer 1.." + strconv.Itoa(projectPreflightMaxTimeoutMS))
	}
	result.TimeoutMS = ms
	return result, nil
}

// runProjectLaunchPreflight resolves the program through the owner's allow
// list and runs it in the repository root. A non-zero exit or a timeout is a
// refusal carrying the tail of what the program printed.
func runProjectLaunchPreflight(ctx context.Context, root, launchID string, preflight *projectLaunchPreflight) error {
	if preflight == nil {
		return nil
	}
	_, executables, err := projectLocalExecution(root)
	if err != nil {
		return err
	}
	path, allowed := executables[preflight.Executable]
	if !allowed {
		return usageError("project_execution_not_allowed: use project local set --allow-executable " + preflight.Executable + "=/absolute/path")
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return usageError("project_execution_unavailable: selected executable is unavailable: " + preflight.Executable)
	}
	timeout := time.Duration(preflight.TimeoutMS) * time.Millisecond
	deadline, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := exec.CommandContext(deadline, path, preflight.Args...)
	command.Dir = root
	command.Env = os.Environ()
	output := &boundedBuffer{limit: projectPreflightMaxOutput}
	command.Stdout, command.Stderr = output, output
	err = command.Run()
	if err == nil {
		return nil
	}
	tail := output.tail(projectPreflightOutputTail)
	if errors.Is(deadline.Err(), context.DeadlineExceeded) {
		return usageError("project_start_preflight_timeout: launch " + launchID + " preflight " + preflight.Executable + " did not finish within " + timeout.String() + "; nothing was started" + tail)
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return usageError("project_start_preflight_failed: launch " + launchID + " preflight " + preflight.Executable + " exited " + strconv.Itoa(exit.ExitCode()) + "; nothing was started" + tail)
	}
	return usageError("project_start_preflight_failed: launch " + launchID + " preflight " + preflight.Executable + ": " + err.Error() + "; nothing was started" + tail)
}

// boundedBuffer keeps at most limit bytes of what a program printed and
// remembers how much it dropped, so a runaway log cannot exhaust memory and
// the refusal still says the output was cut.
type boundedBuffer struct {
	data    []byte
	limit   int
	dropped int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	written := len(p)
	room := b.limit - len(b.data)
	if room <= 0 {
		b.dropped += written
		return written, nil
	}
	if len(p) > room {
		b.dropped += len(p) - room
		p = p[:room]
	}
	b.data = append(b.data, p...)
	return written, nil
}

func (b *boundedBuffer) tail(limit int) string {
	text := strings.TrimSpace(string(b.data))
	if text == "" {
		return ""
	}
	if len(text) > limit {
		text = "…" + text[len(text)-limit:]
	}
	if b.dropped > 0 {
		text += fmt.Sprintf(" [%d bytes of output not kept]", b.dropped)
	}
	return "; output: " + text
}
