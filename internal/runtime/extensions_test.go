package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// Reuse the real F1 driver fixture, retaining its ready Run unchanged. The
// extension is a new workflow identity selected through a versioned config.
func coreDriverFixture(t *testing.T, mode string) (*Engine, flow.WorkflowRevision) {
	t.Helper()
	e, legacyID := driverProject(t, mode, 10000)
	legacy := driverRun(t, e, legacyID)
	var workflow flow.WorkflowRevision
	if err := json.Unmarshal(legacy.Workflow, &workflow); err != nil {
		t.Fatal(err)
	}
	workflow.ID = "test:workflow/core-driver"
	workflow.AllowedOutcomes = []string{"succeeded", "rejected"}
	workflow.Limits.MaxControlTransitions = 8
	work := workflow.Definition.Stages["work"]
	work.OnError = "recovered"
	workflow.Definition.Stages["work"] = work
	workflow.Definition.Stages["recovered"] = flow.Stage{Kind: "finish", Outcome: "rejected", OutputBindings: map[string]flow.Binding{}}
	defs, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	e.Config.Configuration.SemanticsProfile = flow.CoreProfile
	e.Config.Configuration.SchemaVersion = CoreConfigVersion
	e.Config.ConfigurationSchemaRef = builtinRef(defs, "core:schema/core-configuration")
	return e, workflow
}

func coreDriverStart(t *testing.T, e *Engine, workflow flow.WorkflowRevision) string {
	t.Helper()
	writeRuntimeJSON(t, filepath.Join(e.Root, "workflows/driver.json"), workflow)
	writeRuntimeJSON(t, filepath.Join(e.Root, "prifly.json"), e.Config)
	id := driverStart(t, e)
	r := driverRun(t, e, id)
	if r.Profile != flow.CoreProfile || r.SchemaVersion != CoreStateVersion || r.EffectiveConfiguration == nil {
		t.Fatal("fixture did not pin the extended profile and its configuration")
	}
	return id
}

func coreErrorEvents(t *testing.T, e *Engine, runID string) []local.Event {
	t.Helper()
	view, err := e.Store.Read(context.Background(), runID, 0, 1000)
	if err != nil || view.More {
		t.Fatalf("complete error transition history: %v", err)
	}
	var result []local.Event
	for _, event := range view.Events {
		if event.Type == "stage.error_handled" {
			result = append(result, event)
		}
	}
	return result
}

func TestCoreKnownFailureUsesErrorTransition(t *testing.T) {
	for _, recoveryStep := range []bool{false, true} {
		name := "finish"
		if recoveryStep {
			name = "recovery-step"
		}
		t.Run(name, func(t *testing.T) {
			e, workflow := coreDriverFixture(t, "nonzero")
			next := "recovered"
			if recoveryStep {
				data, err := os.ReadFile(filepath.Join(e.Root, "steps/driver.json"))
				var step flow.StepDefinition
				if err != nil || json.Unmarshal(data, &step) != nil {
					t.Fatal("read fixture step", err)
				}
				step.ID = "test:step/recovery"
				data, err = canonical(step)
				if err != nil {
					t.Fatal(err)
				}
				digest, err := flow.Digest(data)
				if err != nil {
					t.Fatal(err)
				}
				ref := flow.Ref{ID: step.ID, Version: step.Version, Digest: digest}
				registryBytes, err := os.ReadFile(filepath.Join(e.Root, "definitions.json"))
				var registry RegistryFile
				if err != nil || json.Unmarshal(registryBytes, &registry) != nil {
					t.Fatal("read fixture registry", err)
				}
				registry.Entries = append(registry.Entries, Definition{Ref: ref, Kind: "step", Path: "steps/recovery.json"})
				writeRuntimeJSON(t, filepath.Join(e.Root, "steps/recovery.json"), step)
				writeRuntimeJSON(t, filepath.Join(e.Root, "definitions.json"), registry)
				executor := e.Config.Configuration.Executors["test:step/driver"]
				executor.Args = []string{"-test.run=^TestDriverWorkerHelper$", "--", "pass"}
				e.Config.Configuration.Executors[step.ID] = executor
				next = "recover"
				work := workflow.Definition.Stages["work"]
				work.OnError = next
				workflow.Definition.Stages["work"] = work
				workflow.Definition.Stages[next] = flow.Stage{Kind: "step", StepRef: ref, InputBindings: work.InputBindings, On: map[string]string{"pass": "recovered"}}
				workflow.Limits.MaxStepInstances = 2
			}
			runID := coreDriverStart(t, e, workflow)
			a := driverExecuteFirst(t, e, runID)
			r := driverRun(t, e, runID)
			if r.Status != "ready" || r.Settled != nil || len(r.Active) != 0 || r.HasUnresolvedEffects || len(r.Ready) != 1 || r.Ready[0] != next || a.Status != "failed" || a.Accepted != nil || r.Steps[a.StepID].Status != "failed" {
				t.Fatalf("known failure did not preserve failed step and select handler: %+v", r)
			}
			if a.ProcessOutcome == nil || a.ProcessOutcome.ExitCode == nil || *a.ProcessOutcome.ExitCode != 9 || !a.ProcessOutcome.WaitReturned || !a.ProcessOutcome.GroupEmpty {
				t.Fatalf("failure did not come from a settled real process: %+v", a)
			}
			events := coreErrorEvents(t, e, runID)
			var event struct {
				ActivationID string `json:"stage_activation_id"`
				AttemptID    string `json:"attempt_id"`
				Failure      string `json:"failure"`
				Next         string `json:"next_stage_id"`
			}
			if len(events) != 1 || json.Unmarshal(events[0].Data, &event) != nil || event.ActivationID != a.ActivationID || event.AttemptID != a.ID || event.Failure != "nonzero_exit" || event.Next != next {
				t.Fatalf("error route lacks its durable owning transition: %+v", events)
			}
			if slot, _, err := e.Store.Slot(context.Background()); err != nil || slot != "" {
				t.Fatalf("settled failure retained its execution slot: %q %v", slot, err)
			}
			if err := e.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := Open(e.Root, false)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = reopened.Close() })
			if err := reopened.Drive(context.Background(), runID); err != nil {
				t.Fatal(err)
			}
			r, view, err := reopened.load(context.Background(), runID)
			wantAttempts := 1
			if recoveryStep {
				wantAttempts++
			}
			if err != nil || r.Status != "completed" || r.Outcome == nil || *r.Outcome != "rejected" || len(r.Attempts) != wantAttempts || r.Attempts[a.ID].Status != "failed" || len(coreErrorEvents(t, reopened, runID)) != 1 {
				t.Fatalf("reopen lost handler or repeated failed work: %+v %v", r, err)
			}
			if recoveryStep {
				activation := activationFor(&r, next)
				if activation == nil || r.Steps[activation.StepID].Status != "completed" || r.Steps[activation.StepID].Verdict != "pass" {
					t.Fatal("recovery step did not complete through the real worker")
				}
			}
			if err := reopened.Drive(context.Background(), runID); err != nil {
				t.Fatal(err)
			}
			_, repeated, err := reopened.load(context.Background(), runID)
			if err != nil || repeated.Cut != view.Cut || repeated.Snapshot.EventSeq != view.Snapshot.EventSeq {
				t.Fatal("terminal drive added error routes or dispatches", err)
			}
		})
	}
}

func TestCoreUnstartedFailureUsesErrorTransition(t *testing.T) {
	e, workflow := coreDriverFixture(t, "pass")
	runID := coreDriverStart(t, e, workflow)
	a := driverAdmit(t, e, runID)
	if err := os.WriteFile(filepath.Join(a.Workspace, "worker.data"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	a = r.Attempts[a.ID]
	if r.Status != "completed" || r.Outcome == nil || *r.Outcome != "rejected" || a.Status != "failed" || a.Dispatch != nil || a.Started != nil || a.Settled == nil || a.ProcessOutcome == nil || a.ProcessOutcome.Started || len(r.Attempts) != 1 || len(coreErrorEvents(t, e, runID)) != 1 {
		t.Fatalf("known unstarted failure was not handled: %+v", r)
	}
	if _, err := os.Stat(filepath.Join(a.Workspace, "worker-ready")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a refused worker actually started")
	}
}

func TestCorePreparationFailureDoesNotInventAttempt(t *testing.T) {
	e, workflow := coreDriverFixture(t, "pass")
	runID := coreDriverStart(t, e, workflow)
	r := driverRun(t, e, runID)
	ref := r.Inputs["source"]
	artifact, contents, err := e.Artifact(ref)
	if err != nil || string(contents) != "pinned input" {
		t.Fatalf("input was not actually sealed before the fault: %q %v", contents, err)
	}
	metadataPath := filepath.Join(e.Root, artifactMetadataPath(ref.ArtifactID))
	metadata, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	blob := filepath.Join(e.Root, e.Config.Configuration.ArtifactRoot, strings.TrimPrefix(artifact.Digest, "sha256:"))
	if err := os.Chmod(blob, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blob, []byte("corrupt sealed input"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	r = driverRun(t, e, runID)
	activation := activationFor(&r, "work")
	if r.Status != "completed" || r.Outcome == nil || *r.Outcome != "rejected" || len(r.Attempts) != 0 || len(r.Active) != 0 || activation == nil || activation.Status != "failed" || r.Steps[activation.StepID].Status != "failed" || len(r.Steps[activation.StepID].AttemptIDs) != 0 {
		t.Fatalf("preparation failure invented an Attempt or lost handler: %+v", r)
	}
	events := coreErrorEvents(t, e, runID)
	var handled struct {
		ActivationID string `json:"stage_activation_id"`
		AttemptID    string `json:"attempt_id"`
	}
	if len(events) != 1 || json.Unmarshal(events[0].Data, &handled) != nil || handled.ActivationID != activation.ID || handled.AttemptID != "" {
		t.Fatalf("preparation error route has an invented execution identity: %+v", events)
	}
	view, err := e.Store.Read(context.Background(), runID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	failedStages := 0
	for _, event := range view.Events {
		if event.Type == "stage.failed" {
			failedStages++
		}
		if event.Type == "attempt.admitted" || event.Type == "attempt.dispatching" {
			t.Fatal("preparation error emitted fake execution facts")
		}
	}
	if failedStages != 1 {
		t.Fatal("preparation failure did not retain its durable stage fact")
	}
	if slot, _, err := e.Store.Slot(context.Background()); err != nil || slot != "" {
		t.Fatalf("preparation failure reserved a slot: %q %v", slot, err)
	}
	if entries, err := os.ReadDir(filepath.Join(e.Root, e.Config.Configuration.WorkspaceRoot)); err != nil || len(entries) != 0 {
		t.Fatalf("invalid input materialized a worker workspace: %+v %v", entries, err)
	}
	after, err := os.ReadFile(metadataPath)
	if err != nil || !bytes.Equal(metadata, after) {
		t.Fatal("handling preparation failure rewrote accepted metadata", err)
	}
	if data, err := os.ReadFile(blob); err != nil || string(data) != "corrupt sealed input" {
		t.Fatal("error handler silently repaired a corrupt accepted input", err)
	}
}

func TestCorePauseDoesNotRunErrorHandler(t *testing.T) {
	for _, invalidWorkspace := range []bool{false, true} {
		name := "pending-dispatch"
		if invalidWorkspace {
			name = "known-unstarted-failure"
		}
		t.Run(name, func(t *testing.T) {
			e, workflow := coreDriverFixture(t, "pass")
			runID := coreDriverStart(t, e, workflow)
			a := driverAdmit(t, e, runID)
			stale, view, err := e.load(context.Background(), runID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := e.Restrict(context.Background(), RestrictCommand{SchemaVersion: "1", CommandID: newID("command"), Scope: "run", ScopeID: runID, Kind: "pause", Reason: "hold error handler admission"}); err != nil {
				t.Fatal(err)
			}
			if invalidWorkspace {
				if err := os.WriteFile(filepath.Join(a.Workspace, "worker.data"), []byte("changed"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			// Exercise the already admitted caller with its genuinely stale read.
			// A known pre-spawn validation failure may settle, but not advance.
			if err := e.executePending(context.Background(), stale, view, a, ""); err != nil {
				t.Fatal(err)
			}
			if err := e.Drive(context.Background(), runID); err != nil {
				t.Fatal(err)
			}
			r := driverRun(t, e, runID)
			a = r.Attempts[a.ID]
			if !r.ResumeRequired || len(r.Attempts) != 1 || len(r.Activations) != 1 || a.Dispatch != nil || a.Started != nil || a.Accepted != nil {
				t.Fatalf("pause admitted error handler or worker: %+v", r)
			}
			if invalidWorkspace {
				if r.Status != "waiting" || a.Status != "failed" || a.Settled == nil || len(r.Active) != 0 || len(coreErrorEvents(t, e, runID)) != 1 {
					t.Fatal("known failure did not remain waiting behind the pause")
				}
			} else if a.Status != "pending" || a.Settled != nil || len(coreErrorEvents(t, e, runID)) != 0 {
				t.Fatal("a pause was misclassified as an executor failure")
			}
			if _, err := os.Stat(filepath.Join(a.Workspace, "worker-ready")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("paused worker actually started")
			}
		})
	}
}

func TestCoreCancellationDoesNotConsumeErrorHandler(t *testing.T) {
	e, workflow := coreDriverFixture(t, "wait")
	runID := coreDriverStart(t, e, workflow)
	_, finished := driverAsync(t, e, runID)
	r := driverWait(t, e, runID, func(r Run) bool {
		for _, a := range r.Attempts {
			if a.Started != nil {
				if _, err := os.Stat(filepath.Join(a.Workspace, "worker-ready")); err == nil {
					return true
				}
			}
		}
		return false
	})
	a := r.Attempts[r.Active[0]]
	if _, err := e.Restrict(context.Background(), RestrictCommand{SchemaVersion: "1", CommandID: newID("command"), Scope: "run", ScopeID: runID, Kind: "cancel", Reason: "cancel is not an error route"}); err != nil {
		t.Fatal(err)
	}
	driverDone(t, finished, false)
	r = driverRun(t, e, runID)
	a = r.Attempts[a.ID]
	if r.Status != "cancelled" || len(r.Attempts) != 1 || len(r.Active) != 0 || len(r.Activations) != 1 || len(coreErrorEvents(t, e, runID)) != 0 || a.ProcessOutcome == nil || !a.ProcessOutcome.WaitReturned || !a.ProcessOutcome.GroupEmpty {
		t.Fatalf("cancellation entered a handler or did not settle: %+v", r)
	}
	if probe := local.ProbeProcess(a.ProcessOutcome.Identity); probe.State != "absent" {
		t.Fatalf("cancelled fixture still owns a process: %+v", probe)
	}
}

func TestCoreMissingVerdictPreservesAcceptedResult(t *testing.T) {
	e, workflow := coreDriverFixture(t, "pass")
	work := workflow.Definition.Stages["work"]
	work.On = map[string]string{"fail": "done"}
	workflow.Definition.Stages["work"] = work
	runID := coreDriverStart(t, e, workflow)
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	if r.Status != "failed" || r.Outcome != nil || len(r.Attempts) != 1 || len(r.Activations) != 1 || len(coreErrorEvents(t, e, runID)) != 0 {
		t.Fatalf("unhandled verdict became a technical error route: %+v", r)
	}
	for _, a := range r.Attempts {
		if a.Accepted == nil || a.Accepted.Verdict != "pass" || a.Status != "completed" || r.Steps[a.StepID].Status != "completed" {
			t.Fatalf("accepted StepResult was erased by a missing handler: %+v", a)
		}
	}
	if len(r.Diagnostics) != 1 || r.Diagnostics[0].Code != "unhandled_verdict" {
		t.Fatalf("missing handler has no precise diagnostic: %+v", r.Diagnostics)
	}
}

func TestCoreLostDriverDoesNotConsumeErrorHandler(t *testing.T) {
	e, workflow := coreDriverFixture(t, "crash-short")
	runID := coreDriverStart(t, e, workflow)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(executable, "-test.run=^TestDriverCrashHelper$", "--", e.Root, runID)
	child.Env = []string{"DRIVER_CRASH_HELPER=1", "GORACE=atexit_sleep_ms=0"}
	var stderr bytes.Buffer
	child.Stderr = &stderr
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	t.Cleanup(func() {
		if !waited {
			_ = child.Process.Kill()
			_ = child.Wait()
		}
	})
	r := driverWait(t, e, runID, func(r Run) bool {
		for _, a := range r.Attempts {
			if a.Started != nil && a.Process != nil {
				if _, err := os.Stat(filepath.Join(a.Workspace, "worker-ready")); err == nil {
					return true
				}
			}
		}
		return false
	})
	a := r.Attempts[r.Active[0]]
	identity := *a.Process
	if identity.OwnerPID != child.Process.Pid || identity.PID == child.Process.Pid {
		t.Fatal("fixture did not start a real driver-owned process")
	}
	if probe := local.ProbeProcess(identity); probe.State != "present" {
		t.Fatalf("worker was not live before loss of ownership: %+v", probe)
	}
	socketBytes, err := os.ReadFile(filepath.Join(a.Workspace, "driver-socket"))
	if err != nil {
		t.Fatal(err)
	}
	socket := string(socketBytes)
	if filepath.Base(socket) != "api.sock" || !strings.HasPrefix(filepath.Base(filepath.Dir(socket)), "prifly-step-") || filepath.Dir(filepath.Dir(socket)) != "/tmp" {
		t.Fatal("unexpected helper socket")
	}
	t.Cleanup(func() { _ = os.Remove(socket); _ = os.Remove(filepath.Dir(socket)) })
	// Only kill the Cmd.Start child still owned by this test. Its bounded worker
	// exits normally; neither the test nor recovery signals a reconstructed PID.
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	err = child.Wait()
	waited = true
	var exited *exec.ExitError
	if !errors.As(err, &exited) {
		t.Fatalf("driver did not crash: %v %s", err, stderr.String())
	}
	status, ok := exited.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() || status.Signal() != syscall.SIGKILL {
		t.Fatalf("driver crash was not SIGKILL: %v", err)
	}
	reopened, err := Open(e.Root, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if err := reopened.Drive(context.Background(), runID); err == nil || !strings.Contains(err.Error(), "recovery_required") {
		t.Fatalf("unknown execution entered its error handler: %v", err)
	}
	r = driverRun(t, reopened, runID)
	if r.Status != "uncertain" || !r.HasUnresolvedEffects || len(r.Attempts) != 1 || len(r.Active) != 1 || r.Attempts[a.ID].Settled != nil || len(coreErrorEvents(t, reopened, runID)) != 0 {
		t.Fatalf("unknown effect was treated as a handled failure: %+v", r)
	}
	if slot, owner, err := reopened.Store.Slot(context.Background()); err != nil || slot != a.ID || owner != runID {
		t.Fatalf("unknown execution lost its actual slot: %q %q %v", slot, owner, err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, finishErr := os.Stat(filepath.Join(a.Workspace, "worker-finished"))
		if finishErr == nil && local.ProbeProcess(identity).State == "absent" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("bounded orphan did not exit normally")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := reopened.Drive(context.Background(), runID); err == nil || !strings.Contains(err.Error(), "recovery_required") {
		t.Fatal("later PID absence authorized the error handler")
	}
	if driverObservedStarts(t, reopened) != 1 || len(coreErrorEvents(t, reopened, runID)) != 0 || len(driverRun(t, reopened, runID).Attempts) != 1 {
		t.Fatal("recovery retried the uncertain worker or started an error handler")
	}
}
