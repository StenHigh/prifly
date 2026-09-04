package runtime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	goruntime "runtime"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// These integration fixtures launch this actual Go test executable through
// stdin/fd3 and the OS process-group runner. No fake executor substitutes for it.
func driverProject(t *testing.T, mode string, timeoutMS int64) (*Engine, string) {
	t.Helper()
	e := artifactEngine(t)
	defs, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	step := flow.StepDefinition{SchemaVersion: "1", ID: "test:step/driver", Version: "1.0.0", Title: "Driver fixture", Kind: "command",
		Inputs: map[string]flow.InputPort{"source": {Port: flow.Port{Format: "blob", MediaTypes: []string{"text/plain"}}, Required: true}}, Outputs: map[string]flow.OutputPort{},
		ContextRefs: []flow.Ref{}, RequiredCapabilities: []string{}, ResultCheckRefs: []flow.Ref{}, ResultSchemaRef: builtinRef(defs, "core:schema/step-result")}
	step.Executor.AdapterRef, step.Executor.Operation = builtinRef(defs, "core:adapter/local-process"), "process"
	step.Effects.Class, step.Effects.RetryClass = "workspace_write", "pure"
	files := map[string][]byte{"source.txt": []byte("pinned input"), "worker.data": []byte("pinned dependency")}
	entries := []Definition{}
	artifactCheck := strings.HasPrefix(mode, "artifact-publish-check")
	checkRefs := []flow.Ref{}
	if artifactCheck {
		check := flow.CheckDefinition{SchemaVersion: flow.CheckDefinitionVersion, ID: "test:check/early-document", Version: "1.0.0", Title: "Early document check", Kind: "content", Claim: "content_valid", Executor: flow.Executor{AdapterRef: builtinVersionRef(defs, "core:adapter/local-process", "2.0.0"), Operation: "check"}}
		data, err := canonical(check)
		if err != nil {
			t.Fatal(err)
		}
		ref := flow.Ref{ID: check.ID, Version: check.Version, Digest: rawDigest(data)}
		entries = append(entries, Definition{Ref: ref, Kind: "check", Path: "early-document-check.json"})
		files["early-document-check.json"] = data
		checkRefs = append(checkRefs, ref)
	}
	if mode == "artifact-publish" || artifactCheck {
		schema := []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","required":["value"],"properties":{"value":{"type":"integer"}},"additionalProperties":false}`)
		digest, err := flow.Digest(schema)
		if err != nil {
			t.Fatal(err)
		}
		ref := flow.Ref{ID: "test:schema/early-document", Version: "1.0.0", Digest: digest}
		entries = append(entries, Definition{Ref: ref, Kind: "schema", Path: "schemas/early-document.json"})
		files["schemas/early-document.json"] = schema
		step.SchemaVersion = "3"
		step.Hooks = map[string]flow.Hook{"document_created": {
			Kind: "artifact", SchemaRef: ref, Description: "One early immutable document",
			Classification: "internal", ReadPolicy: "owner", MaxPayloadBytes: 4096,
			MaxCount: 1, MaxPerMinute: 10, AllowDuringStop: false,
			Artifact: &flow.ArtifactHook{Format: "json", Cardinality: "one", ContentCheckRefs: checkRefs, EarlyConsumption: true},
		}}
		e.Config.Configuration.SemanticsProfile = flow.CoreProfile
		e.Config.Configuration.SchemaVersion = CoreConfigVersion
		e.Config.ConfigurationSchemaRef = builtinRef(defs, "core:schema/core-configuration")
		if artifactCheck {
			step.Executor.AdapterRef = builtinVersionRef(defs, "core:adapter/local-process", "2.0.0")
			e.Config.Configuration.SchemaVersion = CoreContextConfigVersion
			e.Config.ConfigurationSchemaRef = builtinVersionRef(defs, "core:schema/core-configuration", "2.0.0")
		}
	}
	withReport := slices.Contains([]string{"commit-pass", "commit-wait", "commit-consumer", "bad-digest", "bad-json", "mixed-pass", "mixed-check-fail"}, mode)
	if mode == "bad-json" {
		schema := []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","required":["value"],"properties":{"value":{"type":"integer"}},"additionalProperties":false}`)
		digest, err := flow.Digest(schema)
		if err != nil {
			t.Fatal(err)
		}
		ref := flow.Ref{ID: "test:schema/driver-output", Version: "1.0.0", Digest: digest}
		entries = append(entries, Definition{Ref: ref, Kind: "schema", Path: "schemas/driver-output.json"})
		files["schemas/driver-output.json"] = schema
		files["source.txt"] = []byte(`{"value":1}`)
		step.Inputs["source"] = flow.InputPort{Port: flow.Port{Format: "json", SchemaRef: &ref}, Required: true}
	}
	if withReport {
		step.Outputs["report"] = flow.OutputPort{Port: step.Inputs["source"].Port, RequiredFor: []string{"pass"}}
	}
	stepBytes, err := canonical(step)
	if err != nil {
		t.Fatal(err)
	}
	digest, _ := flow.Digest(stepBytes)
	stepRef := flow.Ref{ID: step.ID, Version: step.Version, Digest: digest}
	entries = append(entries, Definition{Ref: stepRef, Kind: "step", Path: "steps/driver.json"})
	files["steps/driver.json"] = stepBytes
	w := flow.WorkflowRevision{SchemaVersion: "1", ID: "test:workflow/driver", Version: "1.0.0", Title: "Driver fixture",
		Inputs: step.Inputs, Outputs: map[string]flow.OutputPort{}, AllowedOutcomes: []string{"succeeded"}, Limits: flow.Limits{MaxStepInstances: 1, MaxControlTransitions: 4, MaxParallelism: 1}, PolicyRef: builtinRef(defs, "core:policy/local")}
	w.Definition.Entry = "work"
	w.Definition.Stages = map[string]flow.Stage{
		"work": {Kind: "step", StepRef: stepRef, InputBindings: map[string]flow.Binding{"source": {From: "workflow_input", Port: "source"}}, On: map[string]string{"pass": "done"}},
		"done": {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]flow.Binding{}},
	}
	if withReport {
		w.Outputs["report"] = flow.OutputPort{Port: step.Outputs["report"].Port, RequiredFor: []string{"succeeded"}}
		w.Definition.Stages["done"].OutputBindings["report"] = flow.Binding{From: "stage_output", StageID: "work", Port: "report"}
	}
	if slices.Contains([]string{"commit-consumer", "bad-digest", "bad-json"}, mode) {
		w.Definition.Stages["work"].On["pass"] = "consume"
		w.Definition.Stages["consume"] = flow.Stage{Kind: "step", StepRef: stepRef, InputBindings: map[string]flow.Binding{"source": {From: "stage_output", StageID: "work", Port: "report"}}, On: map[string]string{"pass": "done"}}
		w.Definition.Stages["done"].OutputBindings["report"] = flow.Binding{From: "stage_output", StageID: "consume", Port: "report"}
		w.Limits.MaxStepInstances, w.Limits.MaxControlTransitions = 2, 6
	}
	if strings.HasPrefix(mode, "mixed-") {
		refs := map[string]flow.Ref{"command": stepRef}
		for _, kind := range []string{"worker", "check"} {
			definition := step
			definition.ID, definition.Kind = "test:step/"+kind, kind
			data, err := canonical(definition)
			if err != nil {
				t.Fatal(err)
			}
			digest, _ := flow.Digest(data)
			ref := flow.Ref{ID: definition.ID, Version: definition.Version, Digest: digest}
			refs[kind] = ref
			path := "steps/" + kind + ".json"
			files[path] = data
			entries = append(entries, Definition{Ref: ref, Kind: "step", Path: path})
		}
		sequence := []struct{ stage, kind string }{{"work", "command"}, {"worker", "worker"}, {"worker_again", "worker"}, {"check", "check"}, {"after_check", "worker"}}
		for i, item := range sequence {
			binding := flow.Binding{From: "workflow_input", Port: "source"}
			if i > 0 {
				binding = flow.Binding{From: "stage_output", StageID: sequence[i-1].stage, Port: "report"}
			}
			next := "done"
			if i+1 < len(sequence) {
				next = sequence[i+1].stage
			}
			w.Definition.Stages[item.stage] = flow.Stage{Kind: "step", StepRef: refs[item.kind], InputBindings: map[string]flow.Binding{"source": binding}, On: map[string]string{"pass": next}}
		}
		w.Definition.Stages["check"].On["fail"] = "rejected"
		w.Definition.Stages["rejected"] = flow.Stage{Kind: "finish", Outcome: "rejected", OutputBindings: map[string]flow.Binding{}}
		w.Definition.Stages["done"].OutputBindings["report"] = flow.Binding{From: "stage_output", StageID: "after_check", Port: "report"}
		w.AllowedOutcomes = append(w.AllowedOutcomes, "rejected")
		w.Limits.MaxStepInstances, w.Limits.MaxControlTransitions = 5, 12
	}
	workflow, err := canonical(w)
	if err != nil {
		t.Fatal(err)
	}
	registryVersion := "1"
	if artifactCheck {
		registryVersion = "3"
	}
	registry, _ := canonical(RegistryFile{SchemaVersion: registryVersion, Entries: entries})
	brief, _ := canonical(Brief{SchemaVersion: "1", ID: "test:brief/driver", Subject: "Local driver safety", DesiredOutcome: "Complete the declared process", InScope: []string{"local scratch files"}, OutOfScope: []string{"network"}, CompletionCriteria: []string{"validated result"}, SourceRefs: []ArtifactRef{}, Assumptions: []string{}, Confirmation: "explicit"})
	files["workflows/driver.json"], files["definitions.json"], files["brief.json"] = workflow, registry, brief
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(e.Root, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range entries {
		if definition.Kind == "step" {
			e.Config.Configuration.Executors[definition.Ref.ID] = ExecutorConfig{Executable: executable, Args: []string{"-test.run=^TestDriverWorkerHelper$", "--", mode}, Files: map[string]string{"worker.data": "worker.data"}, Environment: map[string]string{"DRIVER_TEST_HELPER": "1", "GORACE": "atexit_sleep_ms=0"}, TimeoutMS: timeoutMS, GraceMS: 30, MaxOutputBytes: 1 << 20}
		}
	}
	if artifactCheck {
		checkMode := "publication-pass"
		if mode == "artifact-publish-check-fail" {
			checkMode = "fail"
		}
		e.Config.Configuration.Executors["test:check/early-document"] = ExecutorConfig{Executable: executable, Args: []string{"-test.run=^TestCheckExecutionWorkerHelper$"}, Files: map[string]string{}, Environment: map[string]string{"CHECK_EXECUTION_HELPER": "1", "CHECK_EXECUTION_MODE": checkMode, "GORACE": "atexit_sleep_ms=0"}, TimeoutMS: timeoutMS, GraceMS: 30, MaxOutputBytes: MaxCheckWireBytes}
	}
	configuration, _ := canonical(e.Config)
	if err := os.WriteFile(filepath.Join(e.Root, "prifly.json"), configuration, 0600); err != nil {
		t.Fatal(err)
	}
	return e, driverStart(t, e)
}

func driverStart(t *testing.T, e *Engine) string {
	t.Helper()
	result, err := e.Start(context.Background(), StartOptions{CommandID: newID("command"), WorkflowFile: "workflows/driver.json", BriefFile: "brief.json", Inputs: map[string]string{"source": "source.txt"}})
	if err != nil {
		t.Fatal(err)
	}
	return result.Receipt.RunID
}

func driverRun(t *testing.T, e *Engine, runID string) Run {
	t.Helper()
	r, _, err := e.load(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func driverAdmit(t *testing.T, e *Engine, runID string) *Attempt {
	t.Helper()
	r, v, err := e.load(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	p, err := r.plan()
	if err != nil {
		t.Fatal(err)
	}
	if err := e.admit(context.Background(), r, v, p, activationFor(&r, "work")); err != nil {
		t.Fatal(err)
	}
	r = driverRun(t, e, runID)
	return r.Attempts[r.Active[0]]
}

func driverWait(t *testing.T, e *Engine, runID string, ready func(Run) bool) Run {
	t.Helper()
	// Helpers are separate race-instrumented test binaries. Loading the expanded
	// schema set can take longer than the old five-second polling window.
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		r := driverRun(t, e, runID)
		if ready(r) {
			return r
		}
		select {
		case <-deadline.C:
			t.Fatalf("run did not reach expected boundary: status=%s attempts=%+v diagnostics=%+v", r.Status, r.Attempts, r.Diagnostics)
		case <-ticker.C:
		}
	}
}

func driverAsync(t *testing.T, e *Engine, runID string) (context.CancelFunc, <-chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan error, 1)
	exited := make(chan struct{})
	go func() { defer close(exited); finished <- e.Drive(ctx, runID) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-exited:
		case <-time.After(10 * time.Second):
			t.Error("driver did not finish cleanup before closing its store")
		}
	})
	return cancel, finished
}

func driverDone(t *testing.T, finished <-chan error, interrupted bool) {
	t.Helper()
	select {
	case err := <-finished:
		if interrupted {
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("interruption lost: %v", err)
			}
		} else if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("foreground driver did not settle")
	}
}

func TestDriverRemainingBudget(t *testing.T) {
	base := Observation{UTC: "2026-08-28T00:00:00Z", Session: "clock:one", Source: "go.time.monotonic", SuspendBasis: "excludes_suspend_on_darwin", UTCTrust: "local_wall_unqualified"}
	due := base
	due.UTC, due.MonotonicMS = "2026-08-28T00:00:10Z", 10000
	now := base
	now.UTC, now.MonotonicMS = "2026-08-28T00:00:04Z", 4000
	for _, test := range []struct {
		name string
		edit func(*Observation, *Observation, *Observation)
		want time.Duration
		code string
	}{
		{"remaining", func(_, _, _ *Observation) {}, 6 * time.Second, ""},
		{"exact deadline", func(_, _, n *Observation) { n.UTC, n.MonotonicMS = due.UTC, due.MonotonicMS }, 0, "attempt_deadline_expired"},
		{"wall forward including suspend", func(_, _, n *Observation) { n.UTC = "2026-08-28T00:00:09Z" }, time.Second, ""},
		{"wall jumped past deadline", func(_, _, n *Observation) { n.UTC = "2026-08-28T00:00:11Z" }, 0, "attempt_deadline_expired"},
		{"rollback", func(_, _, n *Observation) { n.UTC = "2026-08-28T00:00:03Z" }, 0, "deadline_clock_rollback"},
		{"monotonic expired after rollback", func(_, _, n *Observation) { n.MonotonicMS = 11000 }, 0, "attempt_deadline_expired"},
		{"new session not trusted", func(_, _, n *Observation) { n.Session, n.MonotonicMS = "clock:two", 0 }, 0, "deadline_clock_unqualified"},
		{"new session explicitly trusted bounds", func(a, d, n *Observation) {
			a.UTCTrust, d.UTCTrust, n.UTCTrust = "trusted", "trusted", "trusted"
			n.Session, n.MonotonicMS = "clock:two", 0
		}, 6 * time.Second, ""},
		{"missing clock", func(_, _, n *Observation) { n.Source = "" }, 0, "deadline_clock_unqualified"},
		{"changed suspend basis", func(_, _, n *Observation) { n.SuspendBasis = "includes_suspend" }, 0, "deadline_clock_unqualified"},
	} {
		t.Run(test.name, func(t *testing.T) {
			a, d, n := base, due, now
			test.edit(&a, &d, &n)
			got, err := remainingBudget(a, d, n)
			if got != test.want || driverFailureCode(err, "") != test.code || ((err == nil) != (test.code == "")) {
				t.Fatalf("remaining=%s error=%v; expected %s/%s", got, err, test.want, test.code)
			}
		})
	}
}

func TestDriverMaterializedDriftSettlesUnstartedAndReleasesSlot(t *testing.T) {
	for _, test := range []struct{ path, code string }{{"context.json", "context_manifest_drift"}, {"inputs/source", "workspace_input_drift"}, {"worker.data", "workspace_script_drift"}} {
		t.Run(test.path, func(t *testing.T) {
			e, runID := driverProject(t, "pass", 5000)
			a := driverAdmit(t, e, runID)
			if err := os.WriteFile(filepath.Join(a.Workspace, test.path), []byte("changed"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := e.Drive(context.Background(), runID); err != nil {
				t.Fatal(err)
			}
			r := driverRun(t, e, runID)
			a = r.Attempts[a.ID]
			if r.Status != "failed" || len(r.Active) != 0 || a.Dispatch != nil || a.Started != nil || a.ExecutorEnd != nil || a.Settled == nil || a.ProcessOutcome == nil || a.ProcessOutcome.Started || r.Diagnostics[0].Code != test.code {
				t.Fatalf("drift did not settle known no-spawn: %+v %+v", r, a)
			}
			// The actual authority slot must be available to another admission.
			nextID := driverStart(t, e)
			next := driverAdmit(t, e, nextID)
			if err := e.settleUnstarted(context.Background(), nextID, next.ID, "", "test_cleanup"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDriverPendingClockRecoveryAndDeadlineNeverBlindRetry(t *testing.T) {
	for _, mode := range []string{"deadline", "new-clock", "dispatched"} {
		t.Run(mode, func(t *testing.T) {
			e, runID := driverProject(t, "pass", 5000)
			a := driverAdmit(t, e, runID)
			if mode == "new-clock" {
				e.clock = newClock()
			} else {
				_, err := e.apply(context.Background(), e.owner, newID("command"), runID, "attempt.observed", map[string]string{"test": mode}, nil, local.CommandGuarded, func(r *Run, _ local.Snapshot, obs Observation) (local.Change, error) {
					current := r.Attempts[a.ID]
					if mode == "dispatched" {
						current.Dispatch, current.Status, current.TokenHash = &obs, "dispatching", rawDigest([]byte("lost-token"))
					} else {
						current.Deadline = obs
						var envelope map[string]any
						if err := json.Unmarshal(current.Envelope, &envelope); err != nil {
							return local.Change{}, err
						}
						envelope["attempt_deadline"] = obs.UTC
						current.Envelope, _ = canonical(envelope)
						current.EnvelopeDigest = rawDigest(current.Envelope)
					}
					return local.Change{}, nil
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			err := e.Drive(context.Background(), runID)
			r := driverRun(t, e, runID)
			a = r.Attempts[a.ID]
			if mode == "dispatched" {
				if err == nil || !strings.Contains(err.Error(), "recovery_required") || r.Status != "uncertain" || len(r.Active) != 1 || a.Settled != nil || len(r.Gaps) != 1 {
					t.Fatalf("ambiguous dispatch retried or released: %+v %v", r, err)
				}
				nextID := driverStart(t, e)
				next, view, err := e.load(context.Background(), nextID)
				if err != nil {
					t.Fatal(err)
				}
				plan, err := next.plan()
				if err != nil {
					t.Fatal(err)
				}
				err = e.admit(context.Background(), next, view, plan, activationFor(&next, "work"))
				if driverFailureCode(err, "") != "capacity_conflict" {
					t.Fatalf("recovered uncertainty released the actual authority slot: %v", err)
				}
			} else if err != nil || r.Status != "failed" || a.Started != nil || a.Settled == nil || len(r.Active) != 0 {
				t.Fatalf("pending deadline retained or restarted: %+v %v", r, err)
			}
			if _, err := os.Stat(filepath.Join(a.Workspace, "worker-ready")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("refused/recovered attempt launched an OS process")
			}
		})
	}
}

func TestDriverStopWinsBeforeAdmissionAndDispatch(t *testing.T) {
	for _, phase := range []string{"before_admission", "before_dispatch"} {
		for _, kind := range []string{"pause", "cancel"} {
			t.Run(phase+"/"+kind, func(t *testing.T) {
				e, runID := driverProject(t, "commit-pass", 10000)
				var attempt *Attempt
				if phase == "before_dispatch" {
					attempt = driverAdmit(t, e, runID)
				}
				stale, view, err := e.load(context.Background(), runID)
				if err != nil {
					t.Fatal(err)
				}
				controller, err := Open(e.Root, false)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = controller.Close() })
				if _, err := controller.Restrict(context.Background(), RestrictCommand{SchemaVersion: "1", CommandID: newID("command"), Scope: "run", ScopeID: runID, Kind: kind, Reason: "win the pre-dispatch boundary"}); err != nil {
					t.Fatal(err)
				}
				if phase == "before_admission" {
					plan, err := stale.plan()
					if err != nil {
						t.Fatal(err)
					}
					err = e.admit(context.Background(), stale, view, plan, activationFor(&stale, "work"))
					if driverFailureCode(err, "") != "version_conflict" {
						t.Fatalf("stale admission crossed a committed stop: %v", err)
					}
				} else if err := e.executePending(context.Background(), stale, view, attempt, ""); err != nil {
					t.Fatal(err)
				}
				if err := e.Drive(context.Background(), runID); err != nil {
					t.Fatal(err)
				}
				r := driverRun(t, e, runID)
				for _, a := range r.Attempts {
					if a.Dispatch != nil || a.Started != nil || a.Accepted != nil {
						t.Fatalf("stop did not fence process dispatch: %+v", a)
					}
					if _, err := os.Stat(filepath.Join(a.Workspace, "worker-ready")); !errors.Is(err, os.ErrNotExist) {
						t.Fatal("stopped worker actually started")
					}
				}
				if phase == "before_admission" && len(r.Attempts) != 0 {
					t.Fatal("rejected admission created an attempt")
				}
				if kind == "cancel" {
					if r.Status != "cancelled" || len(r.Active) != 0 {
						t.Fatalf("known unstarted cancellation did not settle: %+v", r)
					}
					if slot, _, err := e.Store.Slot(context.Background()); err != nil || slot != "" {
						t.Fatalf("unstarted cancel retained slot: %q %v", slot, err)
					}
					return
				}
				if !r.ResumeRequired || r.Status != "waiting" && len(r.Active) == 0 {
					t.Fatal("pause forgot the separate resume gate")
				}
				stop := r.Stops[0]
				if _, err := e.Release(context.Background(), ReleaseRequest{CommandID: newID("command"), RunID: runID, ExpectedControlEpoch: r.ControlEpoch, Stops: []StopGeneration{{ID: stop.ID, Generation: stop.Generation}}, Reason: "release is not resume"}); err != nil {
					t.Fatal(err)
				}
				if err := e.Drive(context.Background(), runID); err != nil {
					t.Fatal(err)
				}
				r, current, err := e.load(context.Background(), runID)
				if err != nil || !r.ResumeRequired {
					t.Fatalf("release opened admission: %v", err)
				}
				for _, a := range r.Attempts {
					if a.Dispatch != nil {
						t.Fatal("release alone dispatched the pending worker")
					}
				}
				if _, err := e.Resume(context.Background(), runID, newID("command"), "explicit continuation", current.Snapshot.Version); err != nil {
					t.Fatal(err)
				}
				if err := e.Drive(context.Background(), runID); err != nil {
					t.Fatal(err)
				}
				r = driverRun(t, e, runID)
				if r.Status != "completed" || len(r.Attempts) != 1 {
					t.Fatalf("explicit resume did not execute exactly one step: %+v", r)
				}
				for _, a := range r.Attempts {
					starts, err := os.ReadFile(filepath.Join(a.Workspace, "worker-starts"))
					if err != nil || string(starts) != "start\n" || a.Accepted == nil {
						t.Fatalf("permitted worker was not observed exactly once: %q %v", starts, err)
					}
				}
			})
		}
	}
}

// This fixture names the deliberate test boundary without adding a production
// fault hook. The real authority transaction commits dispatch, but this test
// harness never invokes OS spawn until a different, explicit test path does so.
func driverDispatchFixture(t *testing.T, e *Engine, runID, attemptID string) {
	t.Helper()
	r, view, err := e.load(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.apply(context.Background(), e.owner, newID("command"), runID, "attempt.dispatching", map[string]string{"attempt_id": attemptID}, &view.Snapshot.Version, local.CommandCAS, func(current *Run, _ local.Snapshot, obs Observation) (local.Change, error) {
		a := current.Attempts[attemptID]
		if a == nil || a.Dispatch != nil || a.Status != "pending" || current.admissionsBlocked() || current.CancelRequested || r.ID != current.ID {
			return local.Change{}, local.Reject("dispatch_blocked", "fixture has no pending admission")
		}
		a.Dispatch, a.Status, a.TokenHash = &obs, "dispatching", rawDigest([]byte("test-owned-boundary-token"))
		return local.Change{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// A driver that died after the process group was already observed empty left
// saved proof that nothing of that attempt is still running. Recovery closes it
// as a lost settlement and releases the slot, instead of holding the authority
// for an owner statement about an outcome the journal already answers.
func TestDriverCrashAfterGroupEmptySettlesWithoutUncertainty(t *testing.T) {
	e, runID := driverProject(t, "pass", 10000)
	a := driverAdmit(t, e, runID)
	driverDispatchFixture(t, e, runID, a.ID)
	ctx := context.Background()
	// Injected runner observations exercise core handling only: no process is
	// spawned, probed or signalled by this fixture.
	identity := local.ProcessIdentity{PID: os.Getpid(), OwnerPID: os.Getpid()}
	for _, kind := range []string{"start_returned", "group_empty"} {
		if err := e.observe(ctx, runID, a.ID, local.ProcessObservation{Kind: kind, Identity: identity}); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatalf("saved executor end was not recovered: %v", err)
	}
	r := driverRun(t, e, runID)
	settled := r.Attempts[a.ID]
	if settled.Settled == nil || settled.Status != "failed" || r.Status == "uncertain" || r.HasUnresolvedEffects || len(r.Active) != 0 {
		t.Fatalf("proven executor end was kept uncertain: %+v", r)
	}
	if settled.ProcessOutcome != nil || settled.Accepted != nil {
		t.Fatalf("recovery invented process facts or accepted an unjudged candidate: %+v", settled)
	}
	if !slices.ContainsFunc(r.Diagnostics, func(d Diagnostic) bool { return d.Code == "executor_settlement_lost" }) {
		t.Fatalf("lost settlement was not named: %+v", r.Diagnostics)
	}
	if slot, owner, err := e.Store.Slot(ctx); err != nil || slot != "" || owner != "" {
		t.Fatalf("closed attempt kept the authority slot: %q %q %v", slot, owner, err)
	}
}

func TestDriverUncertainSettlementRetainsObligationsAndWorkspace(t *testing.T) {
	for _, kind := range []string{"group_not_empty", "uncertainty_flag"} {
		t.Run(kind, func(t *testing.T) {
			e, runID := driverProject(t, "pass", 10000)
			a := driverAdmit(t, e, runID)
			driverDispatchFixture(t, e, runID, a.ID)
			// An injected runner observation exercises core handling only. There is
			// no unkillable process, fabricated CPU evidence or recovered-PID signal.
			outcome := local.ProcessOutcome{Started: true, WaitReturned: true, GroupEmpty: kind == "uncertainty_flag", Uncertain: kind == "uncertainty_flag"}
			if err := e.settle(context.Background(), runID, a.ID, outcome, nil); err != nil {
				t.Fatal(err)
			}
			r := driverRun(t, e, runID)
			if r.Status != "uncertain" || r.terminal() || !r.HasUnresolvedEffects || len(r.Active) != 1 || r.Active[0] != a.ID || r.Attempts[a.ID].Settled != nil || r.Attempts[a.ID].Accepted != nil {
				t.Fatalf("ambiguous observation released obligations: %+v", r)
			}
			if slot, owner, err := e.Store.Slot(context.Background()); err != nil || slot != a.ID || owner != runID {
				t.Fatalf("uncertain attempt lost its slot: %q %q %v", slot, owner, err)
			}
			if data, err := os.ReadFile(filepath.Join(a.Workspace, "inputs/source")); err != nil || string(data) != "pinned input" {
				t.Fatalf("uncertain workspace was cleaned or replaced: %q %v", data, err)
			}
			if err := e.Drive(context.Background(), runID); err == nil || !strings.Contains(err.Error(), "recovery_required") {
				t.Fatalf("uncertain settlement admitted new execution: %v", err)
			}
		})
	}
}

// Execute exactly one ordinary foreground iteration so a test can corrupt an
// accepted dependency before the next admission/finish. The child still runs
// through the real process runner, result validator and settlement transaction.
func driverExecuteFirst(t *testing.T, e *Engine, runID string) *Attempt {
	t.Helper()
	lock, err := e.driverLock(runID)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	socket, closeServer, err := e.serveSteps()
	if err != nil {
		t.Fatal(err)
	}
	defer closeServer()
	a := driverAdmit(t, e, runID)
	r, view, err := e.load(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.executePending(context.Background(), r, view, a, socket); err != nil {
		t.Fatal(err)
	}
	return driverRun(t, e, runID).Attempts[a.ID]
}

func driverObservedStarts(t *testing.T, e *Engine) int {
	t.Helper()
	root := filepath.Join(e.Root, e.Config.Configuration.WorkspaceRoot)
	directories, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, directory := range directories {
		data, err := os.ReadFile(filepath.Join(root, directory.Name(), "worker-starts"))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		count += strings.Count(string(data), "start\n")
	}
	return count
}

func TestDriverAcceptedArtifactLossBlocksFinishAndDependent(t *testing.T) {
	for _, mode := range []string{"commit-pass", "commit-consumer"} {
		for _, fault := range []string{"missing", "corrupt"} {
			t.Run(mode+"/"+fault, func(t *testing.T) {
				e, runID := driverProject(t, mode, 10000)
				a := driverExecuteFirst(t, e, runID)
				if a.Accepted == nil || a.Settled == nil || a.ProcessOutcome == nil || !a.ProcessOutcome.WaitReturned || !a.ProcessOutcome.GroupEmpty {
					t.Fatalf("producer was not actually accepted and settled: %+v", a)
				}
				accepted, _ := canonical(a.Accepted)
				ref := a.Accepted.Outputs["report"]
				artifact, contents, err := e.Artifact(ref)
				if err != nil || string(contents) != "accepted output\n" {
					t.Fatalf("producer bytes were not durable before the fault: %q %v", contents, err)
				}
				metadataPath := filepath.Join(e.Root, artifactMetadataPath(ref.ArtifactID))
				metadata, err := os.ReadFile(metadataPath)
				if err != nil {
					t.Fatal(err)
				}
				blob := filepath.Join(e.Root, e.Config.Configuration.ArtifactRoot, strings.TrimPrefix(artifact.Digest, "sha256:"))
				if fault == "missing" {
					err = os.Remove(blob)
				} else {
					if err := os.Chmod(blob, 0600); err != nil {
						t.Fatal(err)
					}
					err = os.WriteFile(blob, []byte("corrupted accepted bytes"), 0600)
				}
				if err != nil {
					t.Fatal(err)
				}
				for retry := 0; retry < 2; retry++ {
					err := e.Drive(context.Background(), runID)
					if !errors.Is(err, local.ErrIntegrity) {
						t.Fatalf("accepted dependency loss was not an integrity refusal: %v", err)
					}
					if problem, exit := ProblemFor(err); problem.Code != "integrity_failure" || exit != 6 {
						t.Fatalf("integrity incident was hidden from the caller: %+v exit=%d", problem, exit)
					}
				}
				r := driverRun(t, e, runID)
				if r.terminal() || r.Settled != nil || r.Outcome != nil || len(r.Attempts) != 1 || len(r.Active) != 0 || len(r.Outputs) != 0 {
					t.Fatalf("dependency loss closed the run or admitted a consumer: %+v", r)
				}
				current, _ := canonical(r.Attempts[a.ID].Accepted)
				if !bytes.Equal(current, accepted) || r.Steps[a.StepID].Outputs["report"] != ref {
					t.Fatal("dependency failure rewrote the already accepted producer")
				}
				p, err := r.plan()
				if err != nil {
					t.Fatal(err)
				}
				// The original workspace still contains correct bytes. Retrying
				// the seal is not permission to repair accepted authority data.
				if err := e.acceptedOutputs(r, r.Attempts[a.ID], p.Steps["work"], p, *a.Accepted); !errors.Is(err, local.ErrIntegrity) {
					t.Fatalf("producer workspace implicitly healed accepted data: %v", err)
				}
				currentMetadata, err := os.ReadFile(metadataPath)
				if err != nil || !bytes.Equal(currentMetadata, metadata) {
					t.Fatalf("accepted metadata was replaced after the fault: %v", err)
				}
				currentBytes, err := os.ReadFile(blob)
				if fault == "missing" {
					if !errors.Is(err, os.ErrNotExist) {
						t.Fatalf("missing blob was silently created: %q %v", currentBytes, err)
					}
				} else if err != nil || string(currentBytes) != "corrupted accepted bytes" {
					t.Fatalf("corrupt evidence was silently replaced: %q %v", currentBytes, err)
				}
				if starts := driverObservedStarts(t, e); starts != 1 {
					t.Fatalf("a dependent or duplicate worker actually started: %d", starts)
				}
			})
		}
	}
}

func TestDriverInvalidOutputRefusesRealSettlementAndDownstream(t *testing.T) {
	for _, mode := range []string{"bad-digest", "bad-json"} {
		t.Run(mode, func(t *testing.T) {
			e, runID := driverProject(t, mode, 10000)
			if err := e.Drive(context.Background(), runID); err != nil {
				t.Fatal(err) // successful command execution can record failed work
			}
			r := driverRun(t, e, runID)
			if r.Status != "failed" || len(r.Attempts) != 1 || len(r.Active) != 0 || len(r.Outputs) != 0 {
				t.Fatalf("invalid output advanced the workflow: %+v", r)
			}
			var a *Attempt
			for _, attempt := range r.Attempts {
				a = attempt
			}
			if a.Accepted != nil || a.Settled == nil || a.ProcessOutcome == nil || !a.ProcessOutcome.Started || !a.ProcessOutcome.WaitReturned || !a.ProcessOutcome.GroupEmpty || a.ProcessOutcome.Uncertain || a.ProcessOutcome.ExitCode == nil || *a.ProcessOutcome.ExitCode != 0 {
				t.Fatalf("fixture did not reach real exit-0 result rejection: %+v", a)
			}
			if !slices.ContainsFunc(r.Diagnostics, func(d Diagnostic) bool { return d.Code == "invalid_output" }) {
				t.Fatalf("refusal was not output validation: %+v", r.Diagnostics)
			}
			var candidate Result
			if err := decode(a.Candidate, &candidate); err != nil {
				t.Fatal(err)
			}
			if err := flow.ValidateProtocol("StepResult", a.Candidate); err != nil {
				t.Fatalf("malformed envelope prevented reaching the artifact check: %v", err)
			}
			data, err := os.ReadFile(filepath.Join(a.Workspace, a.Context.Outputs["report"].Path))
			if err != nil {
				t.Fatal(err)
			}
			if mode == "bad-digest" && candidate.Outputs["report"].Digest == rawDigest(data) {
				t.Fatal("fixture did not produce a digest mismatch")
			}
			if mode == "bad-json" && (json.Valid(data) || candidate.Outputs["report"].Digest != rawDigest(data)) {
				t.Fatal("fixture did not reach malformed content with a correct digest")
			}
			if _, err := os.Stat(filepath.Join(e.Root, artifactMetadataPath(candidate.Outputs["report"].ArtifactID))); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("invalid output published accepted metadata: %v", err)
			}
			if consumer := activationFor(&r, "consume"); consumer != nil {
				t.Fatalf("invalid output activated a dependent stage: %+v", consumer)
			}
			if starts := driverObservedStarts(t, e); starts != 1 {
				t.Fatalf("downstream or retry actually ran: %d", starts)
			}
		})
	}
}

func TestDriverCommandWorkerCheckAndRepeatedDefinition(t *testing.T) {
	for _, mode := range []string{"mixed-pass", "mixed-check-fail"} {
		t.Run(mode, func(t *testing.T) {
			e, runID := driverProject(t, mode, 10000)
			if err := e.Drive(context.Background(), runID); err != nil {
				t.Fatal(err)
			}
			r := driverRun(t, e, runID)
			p, err := r.plan()
			if err != nil {
				t.Fatal(err)
			}
			stages := []string{"work", "worker", "worker_again", "check"}
			outcome := "rejected"
			if mode == "mixed-pass" {
				stages = append(stages, "after_check")
				outcome = "succeeded"
			}
			if r.Status != "completed" || r.Outcome == nil || *r.Outcome != outcome || len(r.Attempts) != len(stages) || len(r.Steps) != len(stages) || len(r.Active) != 0 {
				t.Fatalf("mixed workflow routed incorrectly: %+v", r)
			}
			input := []byte("pinned input")
			previous := r.Inputs["source"]
			for _, stageID := range stages {
				activation := activationFor(&r, stageID)
				if activation == nil || activation.Status != "completed" {
					t.Fatalf("missing completed stage %s", stageID)
				}
				step := r.Steps[activation.StepID]
				if len(step.AttemptIDs) != 1 {
					t.Fatalf("stage %s reused another step's attempt: %+v", stageID, step)
				}
				a := r.Attempts[step.AttemptIDs[0]]
				if a.Accepted == nil || a.ProcessOutcome == nil || a.ProcessOutcome.ExitCode == nil || *a.ProcessOutcome.ExitCode != 0 || !a.ProcessOutcome.WaitReturned || !a.ProcessOutcome.GroupEmpty || a.Context.Inputs["source"].Ref != previous {
					t.Fatalf("stage %s did not consume the pinned predecessor through a real process: %+v", stageID, a)
				}
				materialized, err := os.ReadFile(filepath.Join(a.Workspace, a.Context.Inputs["source"].Path))
				if err != nil || !bytes.Equal(materialized, input) {
					t.Fatalf("stage %s received incorrect predecessor bytes: %q %v", stageID, materialized, err)
				}
				input = append(append(input, '\n'), []byte(p.Steps[stageID].ID)...)
				previous = a.Accepted.Outputs["report"]
				_, actual, err := e.Artifact(previous)
				if err != nil || !bytes.Equal(actual, input) {
					t.Fatalf("stage %s lost dataflow ordering: %q %v", stageID, actual, err)
				}
				verdict := "pass"
				if stageID == "check" && mode == "mixed-check-fail" {
					verdict = "fail"
				}
				if a.Accepted.Verdict != verdict || step.Verdict != verdict {
					t.Fatalf("exit 0 replaced the declared verdict at %s: %+v", stageID, step)
				}
			}
			worker := r.Steps[activationFor(&r, "worker").StepID]
			repeated := r.Steps[activationFor(&r, "worker_again").StepID]
			if p.Steps["work"].Kind != "command" || p.Steps["worker"].Kind != "worker" || p.Steps["check"].Kind != "check" || worker.Ref != repeated.Ref || worker.ID == repeated.ID || worker.AttemptIDs[0] == repeated.AttemptIDs[0] {
				t.Fatal("mixed kinds or repeated exact StepDefinition usage were not realized")
			}
			if mode == "mixed-check-fail" {
				if activationFor(&r, "after_check") != nil || len(r.Outputs) != 0 {
					t.Fatal("negative check unexpectedly ran the pass successor or exported success outputs")
				}
			} else if r.Outputs["report"] != previous {
				t.Fatal("finish did not preserve the final exact artifact reference")
			}
			if starts := driverObservedStarts(t, e); starts != len(stages) {
				t.Fatalf("observed worker starts differ from selected route: %d want %d", starts, len(stages))
			}
		})
	}
}

func TestDriverActualCPUAndUnsupportedRSSCoverage(t *testing.T) {
	e, runID := driverProject(t, "cpu", 10000)
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	var a *Attempt
	for _, attempt := range r.Attempts {
		a = attempt
	}
	if r.Status != "completed" || a == nil || a.ProcessOutcome == nil || a.ProcessOutcome.CPU == nil {
		t.Fatalf("actual CPU workload did not complete with OS accounting: %+v", r)
	}
	cpu := a.ProcessOutcome.CPU
	if cpu.UserNS+cpu.SystemNS <= 0 || cpu.Method != "os_process_wait_rusage" || cpu.Scope != "waited_child_os_accounting" || cpu.Coverage != "may_include_reaped_children_not_complete_group" {
		t.Fatalf("CPU accounting was absent or claimed complete-tree coverage: %+v", cpu)
	}
	if data, err := os.ReadFile(filepath.Join(a.Workspace, "cpu-work")); err != nil || len(data) != sha256.Size {
		t.Fatalf("CPU worker did not perform its actual hash loop: %x %v", data, err)
	}
	if a.ProcessOutcome.Stdout.BytesRead == 0 || a.ProcessOutcome.Stderr.BytesRead == 0 || len(r.Diagnostics) != 0 || len(r.Publications) != 0 {
		t.Fatalf("opaque diagnostic text was absent or promoted to core/hook facts: %+v", a.ProcessOutcome)
	}
	q := TelemetryQuery{SchemaVersion: "telemetry-query/1", Mode: "records", RunIDs: []string{runID}, Metrics: []string{"os.cpu_total", "os.rss", "provider.tokens", "provider.requests", "core.diagnostics"}, Filters: TelemetryFilters{Scope: []string{"attempt"}}}
	report, err := e.Telemetry(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, record := range report.Records {
		if record.Subject.ID != a.ID {
			t.Fatal("resource evidence belongs to another attempt")
		}
		found[record.Metric] = true
		switch record.Metric {
		case "os.cpu_total":
			if record.Value == nil || *record.Value != float64(cpu.UserNS+cpu.SystemNS)/1e9 || record.Quality != "measured" || record.Coverage != "scope_only" || record.Unit != "s" || record.Dimensions["resource_scope"] != cpu.Scope || !slices.Contains(record.Reasons, cpu.Coverage) {
				t.Fatalf("CPU projection invented a scope or value: %+v", record)
			}
		case "os.rss", "provider.tokens", "provider.requests":
			if record.Value != nil || record.Quality != "unavailable" || !slices.Contains(record.Reasons, "unsupported_meter") {
				t.Fatalf("unsupported resource/usage was invented from diagnostic text: %+v", record)
			}
		default:
			t.Fatalf("opaque ERROR/WARNING text created a diagnostic or usage record: %+v", record)
		}
	}
	if !found["os.cpu_total"] || !found["os.rss"] || !found["provider.tokens"] || !found["provider.requests"] {
		t.Fatal("requested measured/missing resource evidence was omitted")
	}
	warnings := report.Population.Ratios["core.warning_run_fraction"]
	if report.Population.UnknownWarningCoverage != 1 || report.Population.FullWarningCoverage != 0 || report.Population.WarnedIncomplete != 0 || warnings.Value != nil || warnings.Denominator != 0 {
		t.Fatalf("opaque streams claimed warning coverage or an invented zero: %+v", report.Population)
	}
}

func TestDriverCrashBeforeSpawnDoesNotRetry(t *testing.T) {
	driverCrashAtDispatchBoundary(t, false)
}

func TestDriverCrashAfterActualStartRetainsUncertaintyAndSlot(t *testing.T) {
	driverCrashAtDispatchBoundary(t, true)
}

func driverCrashAtDispatchBoundary(t *testing.T, started bool) {
	t.Helper()
	e, runID := driverProject(t, "crash-short", 10000)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// This is the only process the test signals: a child returned by Cmd.Start,
	// still owned and unreaped by this test. No recovered PID is ever signalled.
	child := exec.Command(executable, "-test.run=^TestDriverCrashHelper$", "--", e.Root, runID)
	child.Env = []string{"DRIVER_CRASH_HELPER=1", "GORACE=atexit_sleep_ms=0"}
	if !started {
		child.Env = append(child.Env, "DRIVER_CRASH_BEFORE_SPAWN=1")
	}
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
			if !started && a.Dispatch != nil && a.Started == nil {
				_, err := os.Stat(filepath.Join(e.Root, "dispatch-commit"))
				return err == nil
			}
			if a.Started != nil && a.Process != nil {
				if _, err := os.Stat(filepath.Join(a.Workspace, "worker-ready")); err == nil {
					return true
				}
			}
		}
		return false
	})
	a := r.Attempts[r.Active[0]]
	workspace := a.Workspace
	var identity local.ProcessIdentity
	if a.Dispatch == nil || a.ProcessOutcome != nil {
		t.Fatalf("helper did not commit a pending dispatch: %+v", a)
	}
	if started {
		identity = *a.Process
		if identity.PID == child.Process.Pid || identity.OwnerPID != child.Process.Pid {
			t.Fatalf("not an actual driver-owned live worker: %+v", a)
		}
		if probe := local.ProbeProcess(identity); probe.State != "present" || probe.GroupAlive == nil || !*probe.GroupAlive {
			t.Fatalf("worker was not live before driver crash: %+v", probe)
		}
		socket, err := os.ReadFile(filepath.Join(workspace, "driver-socket"))
		if err != nil {
			t.Fatal(err)
		}
		// SIGKILL prevents the driver's deferred server cleanup. Remove only this
		// helper's exact socket and empty directory, not a glob of other servers.
		socketPath := string(socket)
		if filepath.Base(socketPath) != "api.sock" || !strings.HasPrefix(filepath.Base(filepath.Dir(socketPath)), "prifly-step-") || filepath.Dir(filepath.Dir(socketPath)) != "/tmp" {
			t.Fatalf("unexpected test-owned socket path: %q", socketPath)
		}
		t.Cleanup(func() { _ = os.Remove(socketPath); _ = os.Remove(filepath.Dir(socketPath)) })
	} else if a.Process != nil || a.Started != nil {
		t.Fatal("before-spawn fixture unexpectedly observed a worker")
	}
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	err = child.Wait()
	waited = true
	var exited *exec.ExitError
	if !errors.As(err, &exited) {
		t.Fatalf("driver did not exit by the injected crash: %v %s", err, stderr.String())
	}
	status, ok := exited.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() || status.Signal() != syscall.SIGKILL {
		t.Fatalf("driver exit was not SIGKILL: %v %s", err, stderr.String())
	}
	// Independently open the stored authority with a new clock/session.
	recovered, err := Open(e.Root, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = recovered.Close() })
	if recovered.clock.session == a.Dispatch.Session {
		t.Fatal("recovery reused the lost driver's clock session")
	}
	if err := recovered.Drive(context.Background(), runID); err == nil || !strings.Contains(err.Error(), "recovery_required") {
		t.Fatalf("lost driver was automatically resumed: %v", err)
	}
	r = driverRun(t, recovered, runID)
	if r.Status != "uncertain" || !r.HasUnresolvedEffects || len(r.Active) != 1 || len(r.Attempts) != 1 || r.Attempts[a.ID].Settled != nil {
		t.Fatalf("lost ownership was treated as a settled attempt: %+v", r)
	}
	inspection, err := recovered.View(context.Background(), runID)
	if err != nil || inspection.DriverLive || inspection.Timing.DriverLive || len(inspection.Run.Gaps) == 0 {
		t.Fatalf("recovered read lost the clock gap or invented a live driver: %+v %v", inspection, err)
	}
	executorTime := timingFind(t, inspection.Timing.Root, a.ID).Metrics["executor_time"]
	if executorTime.ValueMS != nil || executorTime.EstimateMS != nil || !slices.Contains([]string{"partial", "unavailable"}, executorTime.Quality) {
		t.Fatalf("recovery invented a complete executor duration: %+v", executorTime)
	}
	if started && (!executorTime.IsOpen || !slices.Contains(executorTime.Reasons, "executor_continuity_not_confirmed")) {
		t.Fatalf("actual ownership loss was not visible in timing coverage: %+v", executorTime)
	}
	beforeExecutor, _ := canonical(executorTime)
	nextID := driverStart(t, recovered)
	next, view, err := recovered.load(context.Background(), nextID)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := next.plan()
	if err != nil {
		t.Fatal(err)
	}
	if err := recovered.admit(context.Background(), next, view, plan, activationFor(&next, "work")); driverFailureCode(err, "") != "capacity_conflict" {
		t.Fatalf("another run obtained the unresolved authority slot: %v", err)
	}
	if next := driverRun(t, recovered, nextID); len(next.Attempts) != 0 || len(next.Active) != 0 {
		t.Fatal("rejected admission leaked a second active attempt")
	}
	// The orphaned worker has its own short bound; wait for normal self-exit.
	// There is deliberately no cancellation through its recovered ProcessIdentity.
	deadline := time.Now().Add(5 * time.Second)
	for started {
		_, finishErr := os.Stat(filepath.Join(workspace, "worker-finished"))
		probe := local.ProbeProcess(identity)
		if finishErr == nil && probe.State == "absent" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("worker did not self-terminate after driver crash: marker=%v probe=%+v", finishErr, probe)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := recovered.Drive(context.Background(), runID); err == nil || !strings.Contains(err.Error(), "recovery_required") {
		t.Fatal("later process absence incorrectly authorized a retry")
	}
	inspection, err = recovered.View(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	afterExecutor, _ := canonical(timingFind(t, inspection.Timing.Root, a.ID).Metrics["executor_time"])
	if !bytes.Equal(beforeExecutor, afterExecutor) {
		t.Fatal("later read extrapolated missing execution after the driver crash")
	}
	entries, err := os.ReadDir(filepath.Join(e.Root, e.Config.Configuration.WorkspaceRoot))
	if err != nil {
		t.Fatal(err)
	}
	launches := 0
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(e.Root, e.Config.Configuration.WorkspaceRoot, entry.Name(), "worker-starts"))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		launches += bytes.Count(data, []byte("start\n"))
	}
	read, err := recovered.Store.Read(context.Background(), runID, 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	dispatches := 0
	for _, event := range read.Events {
		if event.Type == "attempt.dispatching" {
			dispatches++
		}
	}
	wantLaunches := 0
	if started {
		wantLaunches = 1
	}
	if launches != wantLaunches || dispatches != 1 || driverRun(t, recovered, runID).Status != "uncertain" {
		t.Fatalf("recovery duplicated execution or forgot uncertainty: launches=%d dispatches=%d", launches, dispatches)
	}
}

func TestDriverCrashAfterAcceptedCommitDoesNotRepeatExecution(t *testing.T) {
	e, runID := driverProject(t, "commit-pass", 10000)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(executable, "-test.run=^TestDriverCrashHelper$", "--", e.Root, runID)
	child.Env = []string{"DRIVER_CRASH_HELPER=1", "DRIVER_CRASH_AFTER_COMMIT=1", "GORACE=atexit_sleep_ms=0"}
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
		_, err := os.Stat(filepath.Join(e.Root, "accepted-commit"))
		return err == nil && r.Status == "ready" && len(r.Active) == 0
	})
	var before *Attempt
	for _, a := range r.Attempts {
		before = a
	}
	if len(r.Attempts) != 1 || before.Accepted == nil || before.Settled == nil || before.ProcessOutcome == nil || !before.ProcessOutcome.WaitReturned || !before.ProcessOutcome.GroupEmpty || before.ProcessOutcome.Uncertain {
		t.Fatalf("helper did not reach a durable accepted and settled result: %+v", before)
	}
	accepted, err := canonical(before.Accepted)
	if err != nil {
		t.Fatal(err)
	}
	ref := before.Accepted.Outputs["report"]
	if _, data, err := e.Artifact(ref); err != nil || string(data) != "accepted output\n" {
		t.Fatalf("accepted output was not sealed before crash: %q %v", data, err)
	}
	if slot, owner, err := e.Store.Slot(context.Background()); err != nil || slot != "" || owner != "" {
		t.Fatalf("accepted attempt retained capacity: slot=%q owner=%q err=%v", slot, owner, err)
	}
	socket, err := os.ReadFile(filepath.Join(before.Workspace, "driver-socket"))
	if err != nil {
		t.Fatal(err)
	}
	socketPath := string(socket)
	if filepath.Base(socketPath) != "api.sock" || !strings.HasPrefix(filepath.Base(filepath.Dir(socketPath)), "prifly-step-") || filepath.Dir(filepath.Dir(socketPath)) != "/tmp" {
		t.Fatalf("unexpected test-owned socket path: %q", socketPath)
	}
	t.Cleanup(func() { _ = os.Remove(socketPath); _ = os.Remove(filepath.Dir(socketPath)) })
	// Only our live, unreaped helper is signalled. The worker already exited and
	// its OS group settled; the test never signals a persisted worker identity.
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	err = child.Wait()
	waited = true
	var exited *exec.ExitError
	if !errors.As(err, &exited) {
		t.Fatalf("helper did not crash: %v %s", err, stderr.String())
	}
	status, ok := exited.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() || status.Signal() != syscall.SIGKILL {
		t.Fatalf("helper exit was not SIGKILL: %v %s", err, stderr.String())
	}
	recovered, err := Open(e.Root, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = recovered.Close() })
	if err := recovered.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	after := driverRun(t, recovered, runID)
	if after.Status != "completed" || after.Outcome == nil || *after.Outcome != "succeeded" || after.HasUnresolvedEffects || len(after.Active) != 0 || len(after.Attempts) != 1 || after.Outputs["report"] != ref {
		t.Fatalf("accepted result was not continued through its finish: %+v", after)
	}
	again, err := canonical(after.Attempts[before.ID].Accepted)
	if err != nil || !bytes.Equal(accepted, again) {
		t.Fatalf("recovery replaced the committed result: %v", err)
	}
	starts, err := os.ReadFile(filepath.Join(before.Workspace, "worker-starts"))
	if err != nil || string(starts) != "start\n" {
		t.Fatalf("worker was repeated: %q %v", starts, err)
	}
	read, err := recovered.Store.Read(context.Background(), runID, 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	dispatches := 0
	for _, event := range read.Events {
		if event.Type == "attempt.dispatching" {
			dispatches++
		}
	}
	if dispatches != 1 {
		t.Fatalf("committed result was dispatched again: %d", dispatches)
	}
	if err := recovered.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	final, err := recovered.Store.Read(context.Background(), runID, 0, 1)
	if err != nil || final.Cut != read.Cut || final.Snapshot.EventSeq != read.Snapshot.EventSeq {
		t.Fatalf("terminal drive changed recorded history: %v", err)
	}
}

func TestDriverCrashHelper(t *testing.T) {
	if os.Getenv("DRIVER_CRASH_HELPER") != "1" {
		return
	}
	if len(os.Args) < 3 {
		os.Exit(80)
	}
	e, err := Open(os.Args[len(os.Args)-2], false)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(81)
	}
	defer e.Close()
	if os.Getenv("DRIVER_CRASH_BEFORE_SPAWN") == "1" {
		// Controlled boundary fixture: a real child driver commits a real
		// dispatch transaction, then dies without ever invoking RunProcess.
		runID := os.Args[len(os.Args)-1]
		lock, err := e.driverLock(runID)
		if err != nil {
			t.Fatal(err)
		}
		defer lock.Close()
		a := driverAdmit(t, e, runID)
		driverDispatchFixture(t, e, runID, a.ID)
		if err := os.WriteFile(filepath.Join(e.Root, "dispatch-commit"), []byte(a.ID), 0600); err != nil {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Second)
		os.Exit(84)
	}
	if os.Getenv("DRIVER_CRASH_AFTER_COMMIT") == "1" {
		// Stop the test harness between production driver iterations, precisely
		// after executePending's real process/validation/settlement commit. No
		// production failure flag or synthetic accepted snapshot is involved.
		runID := os.Args[len(os.Args)-1]
		lock, err := e.driverLock(runID)
		if err != nil {
			t.Fatal(err)
		}
		defer lock.Close()
		socket, closeServer, err := e.serveSteps()
		if err != nil {
			t.Fatal(err)
		}
		defer closeServer()
		a := driverAdmit(t, e, runID)
		r, view, err := e.load(context.Background(), runID)
		if err != nil {
			t.Fatal(err)
		}
		if err := e.executePending(context.Background(), r, view, a, socket); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(e.Root, "accepted-commit"), []byte(a.ID), 0600); err != nil {
			t.Fatal(err)
		}
		// The parent must SIGKILL this owned helper, not release it normally.
		// A short bound also prevents a stranded helper if the parent disappears.
		time.Sleep(10 * time.Second)
		os.Exit(83)
	}
	if err := e.Drive(context.Background(), os.Args[len(os.Args)-1]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(82)
	}
}

func TestDriverExecutableDriftAfterDispatchSettlesKnownNoSpawn(t *testing.T) {
	e, _ := driverProject(t, "pass", 5000)
	config := e.Config.Configuration.Executors["test:step/driver"]
	executable, err := os.ReadFile(config.Executable)
	if err != nil {
		t.Fatal(err)
	}
	config.Executable = filepath.Join(e.Root, "copied-worker")
	if err := os.WriteFile(config.Executable, executable, 0700); err != nil {
		t.Fatal(err)
	}
	e.Config.Configuration.Executors["test:step/driver"] = config
	runID := driverStart(t, e)
	a := driverAdmit(t, e, runID)
	// Change only our scratch copy, never the running test binary.
	if err := os.WriteFile(config.Executable, []byte("changed executable bytes"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	a = r.Attempts[a.ID]
	if r.Status != "failed" || len(r.Active) != 0 || a.Dispatch == nil || a.Started != nil || a.ExecutorEnd != nil || a.Settled == nil || a.ProcessOutcome == nil || a.ProcessOutcome.Started {
		t.Fatalf("known pre-start failure became a lost dispatch or fake exit: %+v", r)
	}
}

func TestDriverPreparationConsumesOriginalDeadline(t *testing.T) {
	e, runID := driverProject(t, "wait", 2000)
	a := driverAdmit(t, e, runID)
	deadline := a.Deadline
	time.Sleep(1300 * time.Millisecond)
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	a = r.Attempts[a.ID]
	if r.Status != "failed" || a.ProcessOutcome == nil || a.Settled == nil || len(r.Active) != 0 || a.Accepted != nil || a.Deadline != deadline || !slices.Contains([]string{"attempt_deadline_expired", "runtime_limit"}, a.ProcessOutcome.StopReason) {
		t.Fatalf("deadline reset after preparation: %+v diagnostics=%+v", a, r.Diagnostics)
	}
	if !a.ProcessOutcome.Started {
		if a.Started != nil || driverObservedStarts(t, e) != 0 {
			t.Fatal("expired preparation invented a process start")
		}
		return // Under instrumentation, preparation may consume the entire budget.
	}
	if !a.ProcessOutcome.WaitReturned || !a.ProcessOutcome.GroupEmpty {
		t.Fatal("deadline did not settle the owned worker")
	}
	view, err := e.Store.Read(context.Background(), runID, 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range view.Events {
		if event.Type != "attempt.observed" {
			continue
		}
		var data struct {
			Process local.ProcessObservation `json:"process_observation"`
		}
		if err := json.Unmarshal(event.Data, &data); err != nil {
			t.Fatal(err)
		}
		if data.Process.Kind == "term_sent" {
			// ProcessOutcome.Elapsed also includes evidence writes and cleanup.
			if data.Process.Elapsed > 1100*time.Millisecond {
				t.Fatalf("preparation did not shorten the stop deadline: %s", data.Process.Elapsed)
			}
			return
		}
	}
	t.Fatal("deadline expiration has no recorded termination request")
}

func TestDriverPauseKeepsEarlyResultPendingUntilProcessSettlement(t *testing.T) {
	e, runID := driverProject(t, "early", 10000)
	_, finished := driverAsync(t, e, runID)
	r := driverWait(t, e, runID, func(r Run) bool {
		for _, a := range r.Attempts {
			if a.CandidateAt != nil {
				return true
			}
		}
		return false
	})
	a := r.Attempts[r.Active[0]]
	if a.Accepted != nil || a.ExecutorEnd != nil || a.Settled != nil || !e.driverLiveFor(runID) || e.driverLiveFor("run:another") {
		t.Fatal("early result or another driver's lock falsely completed the attempt")
	}
	inspection, err := e.View(context.Background(), runID)
	if err != nil || !inspection.DriverLive || !inspection.Timing.DriverLive {
		t.Fatalf("live early-result view lost foreground ownership: %v", err)
	}
	liveTiming := timingFind(t, inspection.Timing.Root, a.ID)
	executor := liveTiming.Metrics["executor_time"]
	acceptance := liveTiming.Metrics["result_to_acceptance"]
	settlement := liveTiming.Metrics["post_execution_settlement"]
	if !executor.IsOpen || executor.ValueMS == nil || executor.Quality != "measured" || !acceptance.IsOpen || acceptance.KnownMS == nil || settlement.IsOpen || settlement.Quality != "unavailable" {
		t.Fatalf("early result closed execution or prematurely began settlement: %+v", liveTiming.Metrics)
	}
	if !slices.ContainsFunc(liveTiming.Intervals, func(i TimingInterval) bool {
		return i.Metric == "result_to_acceptance" && i.FromRef.Field == "candidate_at"
	}) || slices.ContainsFunc(liveTiming.Intervals, func(i TimingInterval) bool { return i.Metric == "post_execution_settlement" }) {
		t.Fatal("early result timing conflated candidate waiting with post-execution settlement")
	}
	_, err = e.Restrict(context.Background(), RestrictCommand{SchemaVersion: "1", CommandID: newID("command"), Scope: "run", ScopeID: runID, Kind: "pause", Reason: "test pause during live process"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a.Workspace, "finish"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	driverDone(t, finished, false)
	r = driverRun(t, e, runID)
	a = r.Attempts[a.ID]
	if r.Status != "waiting" || !r.ResumeRequired || len(r.Active) != 0 || a.Accepted == nil || a.ProcessOutcome == nil || a.ProcessOutcome.StopReason != "" || !a.ProcessOutcome.GroupEmpty || len(r.Activations) != 1 {
		t.Fatalf("pause cancelled worker or advanced the next stage: %+v", r)
	}
	inspection, err = e.View(context.Background(), runID)
	if err != nil || inspection.DriverLive {
		t.Fatalf("settled worker retained a live foreground view: %v", err)
	}
	closedTiming := timingFind(t, inspection.Timing.Root, a.ID)
	for _, metric := range []string{"executor_time", "result_to_acceptance", "post_execution_settlement"} {
		if duration := closedTiming.Metrics[metric]; duration.IsOpen || duration.KnownMS == nil || duration.Quality == "unavailable" {
			t.Fatalf("settled %s interval remained open or unknown: %+v", metric, duration)
		}
	}
	if !slices.ContainsFunc(closedTiming.Intervals, func(i TimingInterval) bool {
		return i.Metric == "post_execution_settlement" && i.FromRef.Field == "executor_end" && i.ToRef.Field == "settled"
	}) {
		t.Fatal("post-execution settlement lost its own boundaries")
	}
}

func TestDriverCancellationSettlesOwnedGroupAndPersistsInterrupt(t *testing.T) {
	for _, interrupted := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel-command", true: "context-interrupt"}[interrupted], func(t *testing.T) {
			e, runID := driverProject(t, "wait", 10000)
			cancel, finished := driverAsync(t, e, runID)
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
			if interrupted {
				cancel()
			} else {
				_, err := e.Restrict(context.Background(), RestrictCommand{SchemaVersion: "1", CommandID: newID("command"), Scope: "run", ScopeID: runID, Kind: "cancel", Reason: "test owned process cancellation"})
				if err != nil {
					t.Fatal(err)
				}
			}
			driverDone(t, finished, interrupted)
			r = driverRun(t, e, runID)
			a = r.Attempts[a.ID]
			if r.Status != "cancelled" || !r.CancelRequested || len(r.Stops) == 0 || len(r.Active) != 0 || a.Accepted != nil || a.ProcessOutcome == nil || !a.ProcessOutcome.WaitReturned || !a.ProcessOutcome.GroupEmpty || a.ExecutorEnd == nil || a.Settled == nil {
				t.Fatalf("cancellation accepted before owned process settled: %+v", r)
			}
			if probe := local.ProbeProcess(a.ProcessOutcome.Identity); probe.State != "absent" {
				t.Fatalf("settled process is still present: %+v", probe)
			}
		})
	}
}

func TestDriverResultCandidatesAndExitMustAgree(t *testing.T) {
	for _, mode := range []string{"pass", "duplicate", "conflict", "nonzero"} {
		t.Run(mode, func(t *testing.T) {
			e, runID := driverProject(t, mode, 5000)
			if err := e.Drive(context.Background(), runID); err != nil {
				t.Fatal(err)
			}
			r := driverRun(t, e, runID)
			want := "completed"
			if mode == "conflict" || mode == "nonzero" {
				want = "failed"
			}
			if r.Status != want || len(r.Active) != 0 {
				t.Fatalf("result/exit mismatch: %+v", r)
			}
			for _, a := range r.Attempts {
				if (a.Accepted != nil) != (want == "completed") || a.ProcessOutcome == nil || !a.ProcessOutcome.GroupEmpty {
					t.Fatalf("incorrect acceptance: %+v", a)
				}
			}
		})
	}
}

// This opt-in measurement is deliberately not a correctness performance gate.
// It compares two different public call boundaries, not telemetry on vs off.
// Run without -race; the regular safety suite separately exercises race builds.
func TestDriverOutputMediaMustMatchDeclaredPort(t *testing.T) {
	for _, test := range []struct {
		name  string
		media []string
		valid bool
	}{
		{"single_declared_label", []string{"text/plain"}, true},
		{"default_label_outside_declared_set", []string{"text/plain", "text/html"}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			e, runID := driverProject(t, "pass", 3000)
			a := driverAdmit(t, e, runID)
			r := driverRun(t, e, runID)
			p, err := r.plan()
			if err != nil {
				t.Fatal(err)
			}
			step := p.Steps["work"]
			step.Outputs = map[string]flow.OutputPort{"text": {Port: flow.Port{Format: "blob", MediaTypes: test.media}, RequiredFor: []string{"pass"}}}
			p.Steps["work"] = step
			capabilityErr := e.checkCapabilities(p)
			if test.valid && capabilityErr != nil || !test.valid && (capabilityErr == nil || !strings.Contains(capabilityErr.Error(), "unsupported_output_media")) {
				t.Fatalf("output media capability gate: %v", capabilityErr)
			}
			slot := OutputSlot{ArtifactID: "artifact:media-check", Revision: 1, Path: "outputs/text"}
			a.Context.Outputs["text"] = slot
			data := []byte("plain text\n")
			if err := os.WriteFile(filepath.Join(a.Workspace, slot.Path), data, 0600); err != nil {
				t.Fatal(err)
			}
			ref := ArtifactRef{ArtifactID: slot.ArtifactID, Revision: slot.Revision, Digest: rawDigest(data)}
			result := Result{Verdict: "pass", Outputs: map[string]ArtifactRef{"text": ref}}
			err = e.acceptedOutputs(r, a, step, p, result)
			if !test.valid {
				if err == nil || !strings.Contains(err.Error(), "binding_media_mismatch") {
					t.Fatalf("output outside declared media set was accepted: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			artifact, sealed, err := e.Artifact(ref)
			if err != nil || artifact.MediaType != "text/plain" || !bytes.Equal(sealed, data) {
				t.Fatalf("declared media or exact output bytes lost: artifact=%+v err=%v", artifact, err)
			}
		})
	}
}

func TestDriverCaptureCostSamples(t *testing.T) {
	if os.Getenv("PRIFLY_CAPTURE_BENCHMARK") != "1" {
		t.Skip("set PRIFLY_CAPTURE_BENCHMARK=1 for the 15-pair local measurement")
	}
	type sample struct {
		Index                int    `json:"index"`
		Order                string `json:"order"`
		RunnerCallNS         int64  `json:"runner_call_ns"`
		RuntimeDriveNS       int64  `json:"runtime_drive_ns"`
		RunnerObservedNS     int64  `json:"runner_observed_ns"`
		RuntimeObservedNS    int64  `json:"runtime_observed_ns"`
		CommandSampleNS      int64  `json:"command_sample_ns"`
		CommandSampleRecords int    `json:"command_sample_records"`
		CommandSampleBytes   int    `json:"command_sample_payload_bytes"`
	}
	// An untimed full run supplies exact v1 envelope bytes for the transport-only
	// baseline. Reusing these bytes outside the authority grants no admission or
	// publication capability; this helper only reads stdin and emits a result.
	template, templateID := driverProject(t, "pass", 5000)
	if err := template.Drive(context.Background(), templateID); err != nil {
		t.Fatal(err)
	}
	completed := driverRun(t, template, templateID)
	if completed.Status != "completed" {
		t.Fatalf("warmup did not complete: %+v", completed.Diagnostics)
	}
	var admitted *Attempt
	for _, a := range completed.Attempts {
		admitted = a
	}
	if admitted == nil || admitted.ProcessOutcome == nil {
		t.Fatal("warmup produced no settled process")
	}
	executor := completed.Executors["test:step/driver"]
	worker, err := os.Stat(executor.Config.Executable)
	if err != nil {
		t.Fatal(err)
	}
	prepareBaseline := func() local.ProcessSpec {
		dir := t.TempDir()
		for _, name := range []string{"inputs", "outputs", "tmp"} {
			if err := os.Mkdir(filepath.Join(dir, name), 0700); err != nil {
				t.Fatal(err)
			}
		}
		for _, name := range []string{"context.json", "inputs/source", "worker.data"} {
			data, err := os.ReadFile(filepath.Join(admitted.Workspace, name))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
				t.Fatal(err)
			}
		}
		return local.ProcessSpec{Executable: executor.Config.Executable, ExecutableDigest: executor.ExecutableDigest, Args: executor.Config.Args, Dir: dir,
			Env: map[string]string{"DRIVER_TEST_HELPER": "1", "GORACE": "atexit_sleep_ms=0", "PATH": "/usr/bin:/bin", "LANG": "C.UTF-8", "TMPDIR": filepath.Join(dir, "tmp"), "PRIFLY_CONTEXT_FILE": filepath.Join(dir, "context.json")}, Envelope: admitted.Envelope,
			MaxRuntime: 5 * time.Second, GracePeriod: 30 * time.Millisecond, KillWait: 2 * time.Second, MaxStdoutBytes: 64 << 10, MaxStderrBytes: 64 << 10, MaxResultBytes: 1 << 20}
	}
	runBaseline := func(spec local.ProcessSpec) (int64, int64) {
		start := time.Now()
		out, err := local.RunProcess(context.Background(), spec, nil)
		elapsed := time.Since(start).Nanoseconds()
		if err != nil || !out.Started || !out.WaitReturned || !out.GroupEmpty || out.Uncertain || out.ExitCode == nil || *out.ExitCode != 0 || out.StopReason != "" || out.ResultError != "" || len(out.ResultCandidates) != 1 {
			t.Fatalf("baseline did not settle successfully: %+v %v", out, err)
		}
		if err := flow.ValidateProtocol("StepResult", out.ResultCandidates[0]); err != nil {
			t.Fatal(err)
		}
		return elapsed, out.Elapsed.Nanoseconds()
	}
	runRuntime := func(e *Engine, runID string) (int64, int64) {
		start := time.Now()
		err := e.Drive(context.Background(), runID)
		elapsed := time.Since(start).Nanoseconds()
		if err != nil {
			t.Fatal(err)
		}
		r := driverRun(t, e, runID) // This verification read is outside the timer.
		if r.Status != "completed" || len(r.Attempts) != 1 || len(r.Active) != 0 {
			t.Fatalf("runtime sample failed: %+v", r.Diagnostics)
		}
		for _, a := range r.Attempts {
			if a.Accepted == nil || a.ProcessOutcome == nil || !a.ProcessOutcome.WaitReturned || !a.ProcessOutcome.GroupEmpty {
				t.Fatal("runtime sample has no accepted, settled worker")
			}
			return elapsed, a.ProcessOutcome.Elapsed.Nanoseconds()
		}
		panic("completed sample has no attempt")
	}
	// Measure one standard command including the telemetry it records in its
	// own transaction. There is no separate collector call to time any more:
	// a command is the only writer of its own samples.
	runCommandSample := func(e *Engine) (int64, int, int) {
		ctx := context.Background()
		before, err := e.Store.ReadSamples(ctx, -1, 0, 1000)
		if err != nil || before.More {
			t.Fatalf("sampling fixture exceeded its bounded read: %v", err)
		}
		var afterSeq int64
		if len(before.Records) > 0 {
			afterSeq = before.Records[len(before.Records)-1].Seq
		}
		commandID := newID("command")
		started := time.Now()
		result, err := e.Start(ctx, StartOptions{CommandID: commandID, WorkflowFile: "workflows/driver.json", BriefFile: "brief.json", Inputs: map[string]string{"source": "source.txt"}})
		elapsed := time.Since(started).Nanoseconds()
		if err != nil || result.Receipt.Version == 0 || result.TransactionDuration <= 0 {
			t.Fatalf("sampling input is not a real committed command: %+v %v", result, err)
		}
		if !result.SamplesRecorded {
			t.Fatal("the command did not record its own telemetry")
		}
		runID := result.Receipt.RunID
		after, err := e.Store.ReadSamples(ctx, -1, afterSeq, 100)
		if err != nil || after.More || len(after.Records) != 5 {
			t.Fatalf("sample loss/incomplete batch; do not report successful capture latency: %+v %v", after, err)
		}
		metrics := map[string]bool{}
		payloadBytes := 0
		for _, row := range after.Records {
			var value TelemetrySampleData
			if err := decode(row.Data, &value); err != nil {
				t.Fatal(err)
			}
			metrics[value.Metric] = true
			payloadBytes += len(row.Data)
			if row.Cut != result.Receipt.Cut {
				t.Fatalf("the samples did not ride the command's own transaction: cut %d, command cut %d", row.Cut, result.Receipt.Cut)
			}
			if value.Metric != "core.storage_bytes" && (row.RunID != runID || value.CommandID != commandID) {
				t.Fatal("sampling call did not retain its real command/run identity")
			}
		}
		for _, metric := range []string{"core.command_requests", "core.command_duration", "core.lock_wait", "core.transaction_duration", "core.storage_bytes"} {
			if !metrics[metric] {
				t.Fatalf("standard sample batch lost %s", metric)
			}
		}
		return elapsed, len(after.Records), payloadBytes
	}
	_, _ = runBaseline(prepareBaseline()) // One warmup per path, excluded below.
	_, _, _ = runCommandSample(template)
	samples := make([]sample, 0, 15)
	for index := 1; index <= 15; index++ {
		// Fresh project, ready run, executable pinning and baseline materialization
		// are all outside the measured call boundaries in every pair.
		e, runID := driverProject(t, "pass", 5000)
		spec := prepareBaseline()
		s := sample{Index: index}
		if index%2 == 1 {
			s.Order = "runner_then_runtime"
			s.RunnerCallNS, s.RunnerObservedNS = runBaseline(spec)
			s.RuntimeDriveNS, s.RuntimeObservedNS = runRuntime(e, runID)
		} else {
			s.Order = "runtime_then_runner"
			s.RuntimeDriveNS, s.RuntimeObservedNS = runRuntime(e, runID)
			s.RunnerCallNS, s.RunnerObservedNS = runBaseline(spec)
		}
		// Collector measurement is after both process paths, never in their
		// timer boundaries and never between the two members of a pair.
		s.CommandSampleNS, s.CommandSampleRecords, s.CommandSampleBytes = runCommandSample(e)
		samples = append(samples, s)
	}
	medians := map[string]int64{}
	for _, field := range []string{"runner_call_ns", "runtime_drive_ns", "runner_observed_ns", "runtime_observed_ns", "command_sample_ns"} {
		values := make([]int64, 0, len(samples))
		for _, s := range samples {
			switch field {
			case "runner_call_ns":
				values = append(values, s.RunnerCallNS)
			case "runtime_drive_ns":
				values = append(values, s.RuntimeDriveNS)
			case "runner_observed_ns":
				values = append(values, s.RunnerObservedNS)
			case "runtime_observed_ns":
				values = append(values, s.RuntimeObservedNS)
			case "command_sample_ns":
				values = append(values, s.CommandSampleNS)
			}
		}
		slices.Sort(values)
		medians[field] = values[len(values)/2]
	}
	info := template.Store.Info()
	report := map[string]any{"format": "process-capture-samples/2", "collected_at": time.Now().UTC().Format(time.RFC3339Nano), "go_version": goruntime.Version(), "goos": goruntime.GOOS, "goarch": goruntime.GOARCH, "logical_cpus": goruntime.NumCPU(), "gomaxprocs": goruntime.GOMAXPROCS(0), "worker_executable_digest": executor.ExecutableDigest, "worker_executable_bytes": worker.Size(), "baseline_envelope_bytes": len(admitted.Envelope), "sqlite_version": info.SQLiteVersion, "sqlite_journal": info.JournalMode, "sqlite_synchronous": info.Synchronous, "pairs": len(samples), "warmup_pairs_excluded": 1, "command_sample_method": "actual_committed_start_recording_its_own_samples_in_one_transaction", "command_sample_context_budget_ms": 30, "command_sample_warmups_excluded": 1, "samples": samples, "median_ns": medians}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("PROCESS_CAPTURE_SAMPLES %s", encoded)
}

func TestDriverWorkerHelper(t *testing.T) {
	if os.Getenv("DRIVER_TEST_HELPER") != "1" {
		return
	}
	mode := os.Args[len(os.Args)-1]
	if mode == "wait" {
		signal.Ignore(syscall.SIGTERM)
	}
	envelopeBytes, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(90)
	}
	var envelope struct {
		RunID     string   `json:"run_id"`
		StepID    string   `json:"step_instance_id"`
		AttemptID string   `json:"attempt_id"`
		StepRef   flow.Ref `json:"step_ref"`
	}
	if json.Unmarshal(envelopeBytes, &envelope) != nil {
		os.Exit(91)
	}
	withReport := slices.Contains([]string{"commit-pass", "commit-wait", "commit-consumer", "bad-digest", "bad-json", "mixed-pass", "mixed-check-fail"}, mode)
	if mode == "crash-short" || withReport {
		starts, err := os.OpenFile("worker-starts", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
		if err != nil {
			os.Exit(96)
		}
		if _, err := starts.WriteString("start\n"); err != nil {
			os.Exit(97)
		}
		if err := starts.Sync(); err != nil {
			os.Exit(98)
		}
		_ = starts.Close()
		if err := os.WriteFile("driver-socket", []byte(os.Getenv("PRIFLY_SOCKET")), 0600); err != nil {
			os.Exit(99)
		}
		if err := os.WriteFile("worker-ready", nil, 0600); err != nil {
			os.Exit(100)
		}
	}
	if mode == "crash-short" {
		// Do not keep any pipe open or write to a dead driver's descriptors.
		_ = os.NewFile(3, "result").Close()
		_ = os.Stdout.Close()
		_ = os.Stderr.Close()
		time.Sleep(2 * time.Second)
		if err := os.WriteFile("worker-finished", []byte("normal self exit"), 0600); err != nil {
			os.Exit(101)
		}
		os.Exit(0)
	}
	if err := os.WriteFile("worker-ready", nil, 0600); err != nil {
		os.Exit(92)
	}
	if mode == "cpu" {
		// These deliberately look like application failures/provider usage.
		// They are opaque diagnostics, not declared hooks or measured meters.
		_, _ = fmt.Fprintln(os.Stdout, "ERROR tokens=12345 calls=27 cost=42.00")
		_, _ = fmt.Fprintln(os.Stderr, "WARNING ERROR tokens=12345 calls=27 cost=42.00")
		digest := sha256.Sum256(envelopeBytes)
		until := time.Now().Add(150 * time.Millisecond)
		for time.Now().Before(until) {
			for i := 0; i < 1024; i++ {
				digest = sha256.Sum256(digest[:])
			}
		}
		if err := os.WriteFile("cpu-work", digest[:], 0600); err != nil {
			os.Exit(104)
		}
	}
	if mode == "artifact-publish" {
		fail := func(code, status int, body []byte) {
			message := []byte(fmt.Sprintf("exit=%d status=%d body=%s", code, status, body))
			_ = os.WriteFile("artifact-helper-error", message, 0600)
			os.Exit(code)
		}
		data := []byte(`{"value":1}`)
		if err := os.WriteFile("early.json", data, 0600); err != nil {
			fail(106, 0, []byte(err.Error()))
		}
		size := int64(len(data))
		command := PublishCommand{SchemaVersion: "2", CommandID: "command:artifact-publication", RunID: envelope.RunID, StepID: envelope.StepID, AttemptID: envelope.AttemptID, EnvelopeDigest: os.Getenv("PRIFLY_ENVELOPE_DIGEST"), Hook: "document_created", Kind: "artifact", CandidatePath: "early.json", ExpectedDigest: rawDigest(data), ExpectedSizeBytes: &size}
		publish := func(command PublishCommand) (int, []byte) {
			body, err := canonical(command)
			if err != nil {
				return 0, []byte(err.Error())
			}
			transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "unix", os.Getenv("PRIFLY_SOCKET"))
			}}
			defer transport.CloseIdleConnections()
			request, err := http.NewRequest(http.MethodPost, "http://prifly/publish", bytes.NewReader(body))
			if err != nil {
				return 0, []byte(err.Error())
			}
			request.Header.Set("Authorization", "Bearer "+os.Getenv("PRIFLY_TOKEN"))
			response, err := (&http.Client{Transport: transport, Timeout: 3 * time.Second}).Do(request)
			if err != nil {
				return 0, []byte(err.Error())
			}
			defer response.Body.Close()
			body, err = io.ReadAll(response.Body)
			if err != nil {
				return 0, []byte(err.Error())
			}
			return response.StatusCode, body
		}
		mismatch := command
		mismatch.CommandID, mismatch.ExpectedDigest = "command:artifact-mismatch", rawDigest([]byte("different bytes"))
		status, body := publish(mismatch)
		if status != http.StatusConflict || !bytes.Contains(body, []byte(`"code":"artifact_digest_mismatch"`)) {
			fail(107, status, body)
		}
		status, accepted := publish(command)
		if status != http.StatusOK {
			fail(108, status, accepted)
		}
		if err := os.WriteFile("early.json", []byte(`{"value":2}`), 0600); err != nil {
			os.Exit(109)
		}
		status, duplicate := publish(command)
		if status != http.StatusOK || !bytes.Equal(accepted, duplicate) {
			fail(110, status, duplicate)
		}
		if err := os.Remove("early.json"); err != nil {
			os.Exit(111)
		}
		status, duplicate = publish(command)
		if status != http.StatusOK || !bytes.Equal(accepted, duplicate) {
			fail(112, status, duplicate)
		}
		logicalRetry := command
		logicalRetry.CommandID = "command:artifact-logical-retry"
		status, duplicate = publish(logicalRetry)
		if status != http.StatusOK || !bytes.Equal(accepted, duplicate) {
			fail(114, status, duplicate)
		}
		logicalConflict := command
		logicalConflict.CommandID = "command:artifact-logical-conflict"
		logicalConflict.ExpectedDigest = rawDigest([]byte(`{"value":2}`))
		status, body = publish(logicalConflict)
		if status != http.StatusConflict || !bytes.Contains(body, []byte(`"code":"artifact_key_conflict"`)) {
			fail(115, status, body)
		}
		if err := os.WriteFile("artifact-published", accepted, 0600); err != nil {
			os.Exit(113)
		}
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat("finish"); err == nil {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	result := Result{SchemaVersion: "1", RunID: envelope.RunID, StepInstanceID: envelope.StepID, AttemptID: envelope.AttemptID, EnvelopeDigest: os.Getenv("PRIFLY_ENVELOPE_DIGEST"), Verdict: "pass", Outputs: map[string]ArtifactRef{}, EvidenceRefs: []any{}, EffectReceiptRefs: []any{}, Summary: "driver test"}
	if withReport {
		manifestBytes, err := os.ReadFile(os.Getenv("PRIFLY_CONTEXT_FILE"))
		var manifest ContextManifest
		if err != nil || json.Unmarshal(manifestBytes, &manifest) != nil {
			os.Exit(102)
		}
		slot := manifest.Outputs["report"]
		data := []byte("accepted output\n")
		if mode == "bad-json" {
			data = []byte(`{"value":`)
		}
		if strings.HasPrefix(mode, "mixed-") {
			input, err := os.ReadFile(manifest.Inputs["source"].Path)
			if err != nil {
				os.Exit(105)
			}
			data = append(append(input, '\n'), []byte(envelope.StepRef.ID)...)
			if mode == "mixed-check-fail" && envelope.StepRef.ID == "test:step/check" {
				result.Verdict = "fail"
			}
		}
		if err := os.WriteFile(slot.Path, data, 0600); err != nil {
			os.Exit(103)
		}
		result.Outputs["report"] = ArtifactRef{ArtifactID: slot.ArtifactID, Revision: slot.Revision, Digest: rawDigest(data)}
		if mode == "bad-digest" {
			ref := result.Outputs["report"]
			ref.Digest = rawDigest([]byte("not the produced bytes"))
			result.Outputs["report"] = ref
		}
	}
	fd := os.NewFile(3, "result")
	if mode != "wait" {
		if err := json.NewEncoder(fd).Encode(result); err != nil {
			os.Exit(93)
		}
		if mode == "duplicate" || mode == "conflict" {
			if mode == "conflict" {
				result.Summary = "conflicting second result"
			}
			if err := json.NewEncoder(fd).Encode(result); err != nil {
				os.Exit(94)
			}
		}
		if err := fd.Close(); err != nil {
			os.Exit(95)
		}
	}
	if mode == "early" || mode == "wait" || mode == "commit-wait" {
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat("finish"); err == nil {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	if mode == "nonzero" {
		os.Exit(9)
	}
	os.Exit(0)
}

func resultReceipt(t *testing.T, e *Engine, runID, attemptID string, body []byte) local.Receipt {
	t.Helper()
	b, err := flow.Canonical(body)
	if err != nil {
		t.Fatal(err)
	}
	id := derivedID("command", "result-intake", runID, attemptID, rawDigest(b))
	r, err := e.Store.LookupReceipt(context.Background(), "runner:"+strings.TrimPrefix(attemptID, "attempt:"), id)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// Late intake may add a diagnostic and observation, but no execution fact.
func resultControlBytes(t *testing.T, r Run) []byte {
	t.Helper()
	r.Diagnostics, r.LastObserved = nil, Observation{}
	b, err := canonicalState(r)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestDriverResultRetryAndLateConflictPreserveAcceptedResult(t *testing.T) {
	ctx := context.Background()
	e, runID := driverProject(t, "pass", 5000)
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatal(err)
	}
	r, before, err := e.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	var a *Attempt
	for _, current := range r.Attempts {
		a = current
	}
	if r.Status != "completed" || a == nil || a.Accepted == nil || a.Settled == nil {
		t.Fatalf("actual worker did not commit an accepted result: %+v", r.Diagnostics)
	}
	original := resultReceipt(t, e, runID, a.ID, a.Candidate)
	control := resultControlBytes(t, r)
	// Exact delivery after settlement retains the intake receipt. It does not
	// invent a second accepted result or refresh the run's observation time.
	if err := e.observe(ctx, runID, a.ID, local.ProcessObservation{Kind: "result_candidate", Result: a.Candidate}); err != nil {
		t.Fatal(err)
	}
	_, after, err := e.load(ctx, runID)
	if err != nil || before.Snapshot.EventSeq != after.Snapshot.EventSeq || !bytes.Equal(before.Snapshot.Data, after.Snapshot.Data) {
		t.Fatalf("exact delivery mutated the settled run: %v", err)
	}
	got := resultReceipt(t, e, runID, a.ID, a.Candidate)
	firstBytes, _ := canonical(original)
	retryBytes, _ := canonical(got)
	if !bytes.Equal(firstBytes, retryBytes) || !bytes.Contains(got.Result, []byte(`"disposition":"candidate"`)) {
		t.Fatal("retry did not retain the original intake-only receipt")
	}
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	e, err = Open(e.Root, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close() })
	if err := e.observe(ctx, runID, a.ID, local.ProcessObservation{Kind: "result_candidate", Result: a.Candidate}); err != nil {
		t.Fatalf("reopened authority lost retained receipt: %v", err)
	}
	got = resultReceipt(t, e, runID, a.ID, a.Candidate)
	retryBytes, _ = canonical(got)
	if !bytes.Equal(firstBytes, retryBytes) {
		t.Fatal("reopen changed the receipt")
	}
	late := *a.Accepted
	late.Summary = "SECRET-LATE-RESULT-RAW-BODY"
	lateBytes, err := canonical(late)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.observe(ctx, runID, a.ID, local.ProcessObservation{Kind: "result_candidate", Result: lateBytes}); err != nil {
		t.Fatal(err)
	}
	updated, retained, err := e.load(ctx, runID)
	if err != nil || !bytes.Equal(control, resultControlBytes(t, updated)) {
		t.Fatalf("late conflict changed accepted/lifecycle/control state: %v", err)
	}
	if len(updated.Diagnostics) != len(r.Diagnostics)+1 || updated.Diagnostics[len(r.Diagnostics)].Code != "conflicting_result" {
		t.Fatalf("late conflict has no unique diagnostic: %+v", updated.Diagnostics)
	}
	ack := resultReceipt(t, e, runID, a.ID, lateBytes)
	var intake struct {
		Disposition   string      `json:"disposition"`
		Evidence      ArtifactRef `json:"evidence_ref"`
		DiagnosticIDs []string    `json:"diagnostic_ids"`
	}
	if err := json.Unmarshal(ack.Result, &intake); err != nil || intake.Disposition != "conflicting" || len(intake.DiagnosticIDs) != 1 || intake.DiagnosticIDs[0] != updated.Diagnostics[len(r.Diagnostics)].ID {
		t.Fatalf("intake acknowledgement has wrong disposition/correlation: %s / %v", ack.Result, err)
	}
	artifact, stored, err := e.Artifact(intake.Evidence)
	if err != nil || !bytes.Equal(stored, lateBytes) || artifact.Classification != "restricted" || artifact.Producer["port"] != "result_intake" {
		t.Fatalf("late immutable evidence is unavailable or public: %+v / %v", artifact, err)
	}
	view, err := e.View(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	events, err := e.Events(ctx, runID, 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	publicBytes, _ := json.Marshal([]any{view, events, ack})
	if bytes.Contains(publicBytes, []byte(late.Summary)) {
		t.Fatal("ordinary view/events/receipt exposed raw result body")
	}
	// Equivalent JSON formatting is the same result; delivery timestamps and
	// whitespace do not create a second evidence occurrence.
	var formatted bytes.Buffer
	if err := json.Indent(&formatted, lateBytes, "", "  "); err != nil {
		t.Fatal(err)
	}
	if err := e.observe(ctx, runID, a.ID, local.ProcessObservation{Kind: "result_candidate", Result: formatted.Bytes()}); err != nil {
		t.Fatal(err)
	}
	_, duplicate, err := e.load(ctx, runID)
	if err != nil || retained.Snapshot.EventSeq != duplicate.Snapshot.EventSeq || !bytes.Equal(retained.Snapshot.Data, duplicate.Snapshot.Data) {
		t.Fatalf("late retry created another observation/evidence: %v", err)
	}
	if slot, owner, err := e.Store.Slot(ctx); err != nil || slot != "" || owner != "" {
		t.Fatalf("late result reacquired a settled slot: %q %q %v", slot, owner, err)
	}
}

func TestDriverResultIdentityAndVersionAreFenced(t *testing.T) {
	ctx := context.Background()
	e, runID := driverProject(t, "early", 5000)
	_, finished := driverAsync(t, e, runID)
	r := driverWait(t, e, runID, func(r Run) bool {
		return len(r.Active) == 1 && len(r.Attempts[r.Active[0]].Candidate) > 0
	})
	a := r.Attempts[r.Active[0]]
	var base Result
	if err := json.Unmarshal(a.Candidate, &base); err != nil {
		t.Fatal(err)
	}
	_, initial, err := e.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		edit func(*Result)
		arg  string
		code string
	}{
		{"wrong run", func(r *Result) { r.RunID = "run:other" }, a.ID, "result_identity_mismatch"},
		{"wrong step", func(r *Result) { r.StepInstanceID = "step:other" }, a.ID, "result_identity_mismatch"},
		{"wrong attempt", func(r *Result) { r.AttemptID = "attempt:other" }, a.ID, "result_identity_mismatch"},
		{"wrong envelope", func(r *Result) { r.EnvelopeDigest = "sha256:" + strings.Repeat("0", 64) }, a.ID, "result_identity_mismatch"},
		{"unknown attempt", func(r *Result) { r.AttemptID = "attempt:other" }, "attempt:other", "stale_attempt"},
		{"unsupported result version", func(r *Result) { r.SchemaVersion = "2" }, a.ID, "schema_invalid"},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := base
			test.edit(&candidate)
			b, err := canonical(candidate)
			if err != nil {
				t.Fatal(err)
			}
			err = e.observe(ctx, runID, test.arg, local.ProcessObservation{Kind: "result_candidate", Result: b})
			if test.code == "schema_invalid" {
				var p *flow.Problem
				if !errors.As(err, &p) || p.Code != test.code {
					t.Fatalf("unexpected protocol result: %v", err)
				}
			} else if driverFailureCode(err, "") != test.code {
				t.Fatalf("identity was not fenced: %v", err)
			}
			current, view, err := e.load(ctx, runID)
			if err != nil || !bytes.Equal(initial.Snapshot.Data, view.Snapshot.Data) || initial.Snapshot.EventSeq != view.Snapshot.EventSeq || current.Attempts[a.ID].Accepted != nil || len(current.Ready) != 0 {
				t.Fatalf("foreign result mutated candidate/lifecycle/continuation: %v", err)
			}
		})
	}
	if err := os.WriteFile(filepath.Join(a.Workspace, "finish"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	driverDone(t, finished, false)
	if got := driverRun(t, e, runID); got.Status != "completed" {
		t.Fatalf("valid original result no longer completes: %+v", got.Diagnostics)
	}
}

func TestDriverLateResultAfterCancellationIsBoundedEvidence(t *testing.T) {
	ctx := context.Background()
	e, runID := driverProject(t, "wait", 5000)
	_, finished := driverAsync(t, e, runID)
	r := driverWait(t, e, runID, func(r Run) bool {
		return len(r.Active) == 1 && r.Attempts[r.Active[0]].Started != nil
	})
	a := r.Attempts[r.Active[0]]
	if _, err := e.Restrict(ctx, RestrictCommand{SchemaVersion: "1", CommandID: newID("command"), Scope: "run", ScopeID: runID, Kind: "cancel", Reason: "test cancellation"}); err != nil {
		t.Fatal(err)
	}
	driverDone(t, finished, false)
	r = driverRun(t, e, runID)
	if r.Status != "cancelled" || r.Attempts[a.ID].Accepted != nil || r.Attempts[a.ID].Settled == nil {
		t.Fatalf("actual process was not cancelled and settled: %+v", r)
	}
	control := resultControlBytes(t, r)
	late := Result{SchemaVersion: "1", RunID: runID, StepInstanceID: a.StepID, AttemptID: a.ID, EnvelopeDigest: a.EnvelopeDigest, Verdict: "pass", Outputs: map[string]ArtifactRef{}, EvidenceRefs: []any{}, EffectReceiptRefs: []any{}}
	var last []byte
	for i := 0; i <= maxLateResultsPerAttempt; i++ {
		late.Summary = fmt.Sprintf("late occurrence %d", i)
		b, err := canonical(late)
		if err != nil {
			t.Fatal(err)
		}
		err = e.observe(ctx, runID, a.ID, local.ProcessObservation{Kind: "result_candidate", Result: b})
		if i == maxLateResultsPerAttempt {
			if driverFailureCode(err, "") != "result_evidence_limit" {
				t.Fatalf("unbounded late intake: %v", err)
			}
		} else {
			if err != nil {
				t.Fatal(err)
			}
			if ack := resultReceipt(t, e, runID, a.ID, b); !bytes.Contains(ack.Result, []byte(`"disposition":"late"`)) {
				t.Fatalf("late receipt implies acceptance: %s", ack.Result)
			}
			last = b
		}
	}
	current, before, err := e.load(ctx, runID)
	if err != nil || !bytes.Equal(control, resultControlBytes(t, current)) || len(current.Diagnostics)-len(r.Diagnostics) != maxLateResultsPerAttempt {
		t.Fatalf("late intake changed cancellation or violated diagnostic bound: %v", err)
	}
	if err := e.observe(ctx, runID, a.ID, local.ProcessObservation{Kind: "result_candidate", Result: last}); err != nil {
		t.Fatalf("quota hid a retained acknowledgement: %v", err)
	}
	late.Summary = strings.Repeat("x", maxResultEvidenceBytes)
	oversized, err := json.Marshal(late)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.observe(ctx, runID, a.ID, local.ProcessObservation{Kind: "result_candidate", Result: oversized}); driverFailureCode(err, "") != "result_payload_limit" {
		t.Fatalf("oversize result was not rejected before schema/evidence processing: %v", err)
	}
	_, after, err := e.load(ctx, runID)
	if err != nil || before.Snapshot.EventSeq != after.Snapshot.EventSeq || !bytes.Equal(before.Snapshot.Data, after.Snapshot.Data) {
		t.Fatalf("duplicate/quota/oversize delivery mutated run state: %v", err)
	}
	if slot, owner, err := e.Store.Slot(ctx); err != nil || slot != "" || owner != "" {
		t.Fatalf("late result reacquired cancelled slot: %q %q %v", slot, owner, err)
	}
}

func TestResultEvidenceRunQuotaCountsOnlyLateIntake(t *testing.T) {
	r := Run{}
	for i := 0; i < maxLateResultsPerRun; i++ {
		r.Diagnostics = append(r.Diagnostics, Diagnostic{Phase: "result_intake", Code: "late_result", AttemptID: fmt.Sprintf("attempt:%d", i)})
	}
	if err := resultEvidenceQuota(&r, "attempt:new"); driverFailureCode(err, "") != "result_evidence_limit" {
		t.Fatalf("run evidence allowance is unbounded: %v", err)
	}
	r.Diagnostics[0].Phase = "validation"
	if err := resultEvidenceQuota(&r, "attempt:new"); err != nil {
		t.Fatalf("unrelated diagnostic exhausted intake allowance: %v", err)
	}
}
