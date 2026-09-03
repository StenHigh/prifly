package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// The component fixture creates a NEW context-state Run with real compiler,
// configuration/resource snapshots and a complete lock. It does not rewrite
// an older Run or enable the still separately guarded Start/acceptance gate.
func checkExecutionFixture(t *testing.T, mode string, timeoutMS int64) (*Engine, string) {
	t.Helper()
	e := contextRegistryRuntime(t)
	definitions, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	registry := flow.Registry{}
	for _, definition := range definitions {
		registry[definition.Ref] = definition.Bytes
	}
	check := flow.CheckDefinition{SchemaVersion: flow.CheckDefinitionVersion, ID: "test:check/runner", Version: "1.0.0", Title: "Native check runner", Kind: "content", Claim: "content_valid", Executor: flow.Executor{AdapterRef: builtinVersionRef(definitions, "core:adapter/local-process", "2.0.0"), Operation: "check"}}
	checkBytes, err := canonical(check)
	if err != nil {
		t.Fatal(err)
	}
	checkRef := flow.Ref{ID: check.ID, Version: check.Version, Digest: rawDigest(checkBytes)}
	registry[checkRef] = checkBytes
	definitions = append(definitions, PinnedDefinition{Ref: checkRef, Kind: "check", RawDigest: checkRef.Digest, Bytes: checkBytes})
	workflow := flow.WorkflowRevision{SchemaVersion: "1", ID: "test:workflow/check-runner", Version: "1.0.0", Title: "Check runner fixture", Inputs: map[string]flow.InputPort{"value": {Port: flow.Port{Format: "blob", MediaTypes: []string{"text/plain"}, ContentCheckRefs: []flow.Ref{checkRef}}, Required: true}}, Outputs: map[string]flow.OutputPort{}, AllowedOutcomes: []string{"no_work"}, Limits: flow.Limits{MaxStepInstances: 1, MaxControlTransitions: 16, MaxParallelism: 1}, PolicyRef: builtinVersionRef(definitions, "core:policy/local", "2.0.0")}
	workflow.Definition.Entry = "done"
	workflow.Definition.Stages = map[string]flow.Stage{"done": {Kind: "finish", Outcome: "no_work", OutputBindings: map[string]flow.Binding{}}}
	workflowBytes, err := canonical(workflow)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := flow.CompileCore(workflowBytes, "json", registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	workflowRef := planRef(plan)
	definitions = append(definitions, PinnedDefinition{Ref: workflowRef, Kind: "workflow", RawDigest: workflowRef.Digest, Bytes: plan.Canonical})
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		t.Fatal(err)
	}
	executor := ExecutorConfig{Executable: executable, Args: []string{"-test.run=^TestCheckExecutionWorkerHelper$"}, Files: map[string]string{}, Environment: map[string]string{"CHECK_EXECUTION_HELPER": "1", "CHECK_EXECUTION_MODE": mode, "GORACE": "atexit_sleep_ms=0"}, TimeoutMS: timeoutMS, GraceMS: 30, MaxOutputBytes: MaxCheckWireBytes}
	profile, profileRef, err := contextProfileFor(executor, definitions, registry)
	if err != nil {
		t.Fatal(err)
	}
	executor.ContextProfileRef = &profileRef
	executableDigest, err := local.ProcessExecutableDigest(executable)
	if err != nil {
		t.Fatal(err)
	}
	executors := map[string]PinnedExecutor{checkRef.String(): {Config: executor, ExecutableDigest: executableDigest, Files: map[string]local.BlobRef{}, ContextProfile: &profile}}
	e.Config.Configuration.SchemaVersion, e.Config.ConfigurationSchemaRef = CoreContextConfigVersion, builtinVersionRef(definitions, "core:schema/core-configuration", "2.0.0")
	e.Config.AdapterBindings["local_process"] = check.Executor.AdapterRef
	e.Config.Configuration.Executors[check.ID] = executor
	writeRuntimeJSON(t, filepath.Join(e.Root, "prifly.json"), e.Config)
	effective := &EffectiveConfiguration{SchemaVersion: "effective-configuration/1", WorkflowRef: workflowRef, Inputs: map[string]ConfigurationValue{}}
	configurations := map[string]*EffectiveConfiguration{workflowRef.Digest: effective}
	configuration, err := canonical(map[string]any{"schema_version": "core-run-configuration/3", "semantics_profile": flow.CoreProfile, "configuration_schema_ref": e.Config.ConfigurationSchemaRef, "executors": executors, "effective_configuration": effective, "workflow_configurations": configurations})
	if err != nil {
		t.Fatal(err)
	}
	configurationDigest := rawDigest(configuration)
	configurationRef := flow.Ref{ID: "snapshot:executors/" + strings.TrimPrefix(configurationDigest, "sha256:"), Version: "1.0.0", Digest: configurationDigest}
	definitions = append(definitions, PinnedDefinition{Ref: configurationRef, Kind: "resource", RawDigest: configurationDigest, Bytes: configuration})
	resources := []PinnedResource{}
	resourceSnapshot, err := contextResourceSnapshot(resources)
	if err != nil {
		t.Fatal(err)
	}
	definitions = append(definitions, resourceSnapshot)
	closure := make([]flow.Ref, 0, len(definitions))
	for _, definition := range definitions {
		closure = append(closure, definition.Ref)
	}
	sort.Slice(closure, func(i, j int) bool { return closure[i].String() < closure[j].String() })
	lockID := newID("lock")
	lockBytes, err := canonical(map[string]any{"schema_version": "1", "id": lockID, "version": "1.0.0", "core_protocol": "1", "closure": closure, "resolver_ref": builtinVersionRef(definitions, "core:resolver/local", "2.0.0")})
	if err != nil || flow.ValidateProtocol("PackageLock", lockBytes) != nil {
		t.Fatal("invalid fixture lock", err)
	}
	lockRef := flow.Ref{ID: lockID, Version: "1.0.0", Digest: rawDigest(lockBytes)}
	definitions = append(definitions, PinnedDefinition{Ref: lockRef, Kind: "resource", RawDigest: lockRef.Digest, Bytes: lockBytes})
	if err := e.pinDefinitions(definitions); err != nil {
		t.Fatal(err)
	}
	commandID, runID, invocationID := newID("command"), newID("run"), newID("invocation")
	producer := map[string]any{"kind": "authority", "authority_id": e.Installation.ID, "command_id": commandID, "port": "value"}
	input, err := e.putArtifact([]byte("sealed check subject"), "blob", nil, newID("artifact"), producer, nil, registry, "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]ArtifactRef{"value": input.Ref()}
	zero := int64(0)
	_, err = e.apply(context.Background(), e.owner, commandID, runID, "run.created", map[string]any{"workflow_ref": workflowRef, "package_lock_ref": lockRef}, &zero, local.CommandCAS, func(r *Run, _ local.Snapshot, obs Observation) (local.Change, error) {
		*r = Run{SchemaVersion: CoreContextStateVersion, ID: runID, AuthorityID: e.Installation.ID, ProjectID: e.Config.ID, Profile: flow.CoreProfile, TrustProfile: "core-local/cooperative", InteractionMode: "with_human", ExecutionMode: "managed", CapacityProfile: "foundation:one-slot", Status: "ready", RootInvocationID: invocationID, WorkflowRef: workflowRef, Workflow: plan.Canonical, Definitions: definitions, ContextResources: resources, Executors: executors, EffectiveConfiguration: effective, WorkflowConfigurations: configurations, Brief: input.Ref(), LockRef: lockRef, Inputs: inputs, Outputs: map[string]ArtifactRef{}, Invocations: map[string]*Invocation{invocationID: {ID: invocationID, RunID: runID, WorkflowRef: workflowRef, Status: "ready", Inputs: inputs, Outputs: map[string]ArtifactRef{}, Ready: []string{"done"}, Created: obs}}, Active: []string{}, Activations: map[string]*Activation{}, Steps: map[string]*Step{}, Attempts: map[string]*Attempt{}, CheckExecutions: map[string]*CheckExecution{}, Stops: []Stop{}, Publications: []Publication{}, Diagnostics: []Diagnostic{}, Created: obs, CoreBuild: Version, Gaps: []TimingGap{}, Transitions: []StateChange{}}
		return local.Change{}, r.beginWorkflowInputAcceptance(plan, invocationID, obs)
	})
	if err != nil {
		t.Fatal(err)
	}
	return e, runID
}

func prepareCheckExecution(t *testing.T, e *Engine, runID string) CheckAdmission {
	t.Helper()
	r, view, err := e.load(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if r.PendingAcceptance == nil || r.PendingAcceptance.Kind != "workflow_input" || len(r.PendingAcceptance.Checks) != 1 {
		t.Fatal("fixture has no derived workflow-input acceptance boundary")
	}
	required := r.PendingAcceptance.Checks[0]
	checkRef := required.Ref
	executor := r.Executors[checkRef.String()]
	profile := *executor.ContextProfile
	input, sourceBytes, err := e.Artifact(r.Inputs["value"])
	if err != nil {
		t.Fatal(err)
	}
	checkID, commandID := required.ID, newID("command")
	workspace := filepath.Join(e.Root, e.Config.Configuration.WorkspaceRoot, strings.TrimPrefix(checkID, "check:"))
	for _, directory := range []string{"context/sources", "tmp"} {
		if err := os.MkdirAll(filepath.Join(workspace, directory), 0700); err != nil {
			t.Fatal(err)
		}
	}
	manifest := FullContextManifest{SchemaVersion: "1", ID: newID("context"), Version: "1.0.0", Entries: []FullContextEntry{{SourceID: "input:value", ArtifactRef: input.Ref(), Role: "data", Trust: "user_data", Classification: input.Classification}}, IsolationRequired: profile.IsolationRequired, MaxBytes: profile.MaxBytes, Truncation: profile.Truncation, AssemblyRef: profile.AssemblyRef}
	manifestBytes, err := canonical(manifest)
	if err != nil {
		t.Fatal(err)
	}
	producer := map[string]any{"kind": "authority", "authority_id": r.AuthorityID, "command_id": commandID, "port": "context"}
	manifestSchema := builtinRef(r.Definitions, "core:schema/full-context")
	manifestArtifact, err := e.putArtifact(manifestBytes, "json", &manifestSchema, newID("artifact"), producer, nil, r.registry())
	if err != nil {
		t.Fatal(err)
	}
	prepared := e.clock.now()
	now, err := time.Parse(time.RFC3339Nano, prepared.UTC)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := r.plan()
	if err != nil {
		t.Fatal(err)
	}
	request := CheckRequest{SchemaVersion: CheckRequestVersion, CheckID: checkID, RunID: runID, InvocationID: r.RootInvocationID, Boundary: "workflow_input", Port: "value", CheckRef: checkRef, WorkflowRef: r.WorkflowRef, PolicyRef: plan.Workflow.PolicyRef, AdmissionID: newID("admission"), AdmittedVersion: view.Snapshot.Version + 1, ControlEpoch: r.ControlEpoch, PackageLockDigest: r.LockRef.Digest, Subjects: []ArtifactRef{input.Ref()}, ContextManifestRef: manifestArtifact.Ref(), DispatchNotAfter: now.Add(min(time.Duration(executor.Config.TimeoutMS)*time.Millisecond, 30*time.Second)).Format(time.RFC3339Nano), CheckDeadline: now.Add(time.Duration(executor.Config.TimeoutMS) * time.Millisecond).Format(time.RFC3339Nano)}
	requestBytes, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	requestBytes = append(requestBytes, '\n')
	renderBytes, err := RenderCheckContext(manifest, requestBytes, []ContextSource{{Artifact: input, Bytes: sourceBytes}})
	if err != nil {
		t.Fatal(err)
	}
	renderSchema := builtinRef(r.Definitions, "core:schema/context-json")
	rendered, err := e.putArtifact(renderBytes, "json", &renderSchema, newID("artifact"), producer, nil, r.registry())
	if err != nil {
		t.Fatal(err)
	}
	source := LocalPort{Ref: input.Ref(), Path: ContextSourcePath(0)}
	transport := ContextManifest{SchemaVersion: "local-context/2", Inputs: map[string]LocalPort{"value": source}, Outputs: map[string]OutputSlot{}, Dependencies: []flow.Ref{}, Manifest: &LocalPort{Ref: manifestArtifact.Ref(), Path: "context/manifest.json"}, Rendering: &LocalPort{Ref: rendered.Ref(), Path: "context/rendered.json"}, Sources: []LocalPort{source}}
	transportBytes, err := canonical(transport)
	if err != nil {
		t.Fatal(err)
	}
	for path, data := range map[string][]byte{"context.json": transportBytes, "context/manifest.json": manifestBytes, "context/rendered.json": renderBytes, source.Path: sourceBytes} {
		if err := os.WriteFile(filepath.Join(workspace, path), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return CheckAdmission{Request: requestBytes, Workspace: workspace, Context: transport, Prepared: prepared}
}

func admitCheckExecution(t *testing.T, e *Engine, runID string, prepared CheckAdmission) *CheckExecution {
	t.Helper()
	r, view, err := e.load(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.admitCheck(context.Background(), r, view, newID("command"), prepared); err != nil {
		t.Fatal(err)
	}
	r = driverRun(t, e, runID)
	return r.CheckExecutions[r.ActiveCheckID]
}

func executeCheckExecution(t *testing.T, e *Engine, runID, checkID string) {
	t.Helper()
	lock, err := e.driverLock(runID)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	r, view, err := e.load(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.executePendingCheck(context.Background(), r, view, r.CheckExecutions[checkID]); err != nil {
		t.Fatal(err)
	}
}

func TestCheckExecutionNativeReportsAreNotProcessStatuses(t *testing.T) {
	for _, verdict := range []string{"pass", "fail", "inconclusive"} {
		t.Run(verdict, func(t *testing.T) {
			e, runID := checkExecutionFixture(t, verdict, 10000)
			before := driverRun(t, e, runID)
			prepared := prepareCheckExecution(t, e, runID)
			check := admitCheckExecution(t, e, runID, prepared)
			slot, owner, err := e.Store.Slot(context.Background())
			if err != nil || slot != check.ID || owner != runID {
				t.Fatal("check did not acquire the actual authority slot", slot, owner, err)
			}
			executeCheckExecution(t, e, runID, check.ID)
			after := driverRun(t, e, runID)
			settled := after.CheckExecutions[check.ID]
			if settled.Status != "completed" || settled.Report == nil || settled.Report.Status != verdict || settled.Failure != "" || settled.Settled == nil || settled.Started == nil || settled.ExecutorEnd == nil || settled.Process == nil || settled.ProcessOutcome == nil || !settled.ProcessOutcome.Started || !settled.ProcessOutcome.WaitReturned || !settled.ProcessOutcome.GroupEmpty {
				t.Fatalf("native report or process settlement missing: %+v", settled)
			}
			if len(after.Steps) != 0 || len(after.Attempts) != 0 || len(after.Activations) != len(before.Activations) || after.ControlTransitions != before.ControlTransitions+1 || after.Invocations[after.RootInvocationID].StepInstances != 0 || after.ActiveCheckID != "" {
				t.Fatal("automatic check became a Step/Attempt or lost its one control charge")
			}
			requestBytes, err := e.Blobs.Read(settled.RequestBytes)
			if err != nil || !bytes.Equal(requestBytes, prepared.Request) || settled.Report.RequestDigest != rawDigest(prepared.Request) {
				t.Fatal("exact request bytes were normalized or lost", err)
			}
			reportBytes, err := e.Blobs.Read(*settled.ReportBytes)
			if err != nil || ValidateCheckResult(*settled.Report, requestBytes) != nil || !bytes.Contains(reportBytes, []byte(verdict)) {
				t.Fatal("exact stdout report was not retained", err)
			}
			_, view, err := e.load(context.Background(), runID)
			if err != nil {
				t.Fatal(err)
			}
			executeCheckExecution(t, e, runID, check.ID)
			_, again, err := e.load(context.Background(), runID)
			launches, readErr := os.ReadFile(filepath.Join(check.Workspace, "launches"))
			if err != nil || readErr != nil || string(launches) != "launch\n" || again.Snapshot.Version != view.Snapshot.Version {
				t.Fatal("settled check was replayed", err, readErr)
			}
		})
	}
}

func TestCheckExecutionNativeRejectsInvalidReportsAndProcessFailures(t *testing.T) {
	for _, test := range []struct{ mode, failure string }{
		{"malformed", "invalid_json"},
		{"missing", "missing_check_result"},
		{"wrong_request", "check_result_identity_mismatch"},
		{"wrong_run", "check_result_identity_mismatch"},
		{"extra_field", "invalid_check_result"},
		{"nonzero", "nonzero_exit"},
		{"fd3", ""},
		{"stdout_limit", "stdout_limit"},
		{"stderr_limit", "stderr_limit"},
	} {
		t.Run(test.mode, func(t *testing.T) {
			e, runID := checkExecutionFixture(t, test.mode, 10000)
			check := admitCheckExecution(t, e, runID, prepareCheckExecution(t, e, runID))
			executeCheckExecution(t, e, runID, check.ID)
			r := driverRun(t, e, runID)
			settled := r.CheckExecutions[check.ID]
			if settled.Status != "failed" || settled.Report != nil || settled.Failure == "" || settled.Settled == nil || settled.Started == nil || settled.ProcessOutcome == nil || !settled.ProcessOutcome.WaitReturned || !settled.ProcessOutcome.GroupEmpty || settled.ProcessOutcome.Uncertain {
				t.Fatalf("invalid native check was accepted or not settled: %+v", settled)
			}
			if test.failure != "" && settled.Failure != test.failure {
				t.Fatalf("failure=%q, want %q", settled.Failure, test.failure)
			}
			if test.mode == "fd3" && settled.ProcessOutcome.ResultBytes == 0 {
				t.Fatal("test did not exercise the forbidden StepResult channel")
			}
			if test.mode == "stdout_limit" && (!settled.ProcessOutcome.Stdout.Truncated || settled.ReportBytes == nil || settled.ReportBytes.Size > MaxCheckWireBytes) {
				t.Fatal("stdout capture or retained bytes exceeded the bounded check protocol")
			}
			if test.mode == "stderr_limit" && (!settled.ProcessOutcome.Stderr.Truncated || settled.ProcessOutcome.Stderr.BytesRead <= 64<<10) {
				t.Fatal("stderr limit was not exercised")
			}
			if slot, _, err := e.Store.Slot(context.Background()); err != nil || slot != "" || r.ActiveCheckID != "" || len(r.Steps) != 0 || len(r.Attempts) != 0 {
				t.Fatal("proven check failure retained a worker slot or invented producer work", slot, err)
			}
		})
	}
}

func TestCheckExecutionMaterializedDriftNeverLaunches(t *testing.T) {
	for _, path := range []string{"context.json", "context/manifest.json", "context/rendered.json", ContextSourcePath(0)} {
		t.Run(path, func(t *testing.T) {
			e, runID := checkExecutionFixture(t, "pass", 10000)
			check := admitCheckExecution(t, e, runID, prepareCheckExecution(t, e, runID))
			if err := os.WriteFile(filepath.Join(check.Workspace, path), []byte("changed"), 0600); err != nil {
				t.Fatal(err)
			}
			executeCheckExecution(t, e, runID, check.ID)
			r := driverRun(t, e, runID)
			settled := r.CheckExecutions[check.ID]
			failure := "context_manifest_drift"
			if path == "context.json" {
				failure = "check_context_drift"
			}
			if settled.Status != "failed" || settled.Failure != failure || settled.Settled == nil || settled.Dispatch != nil || settled.Started != nil || settled.ExecutorEnd != nil || settled.Report != nil || settled.ProcessOutcome == nil || settled.ProcessOutcome.Started {
				t.Fatalf("materialized drift crossed dispatch: %+v", settled)
			}
			if _, err := os.Stat(filepath.Join(check.Workspace, "launches")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("drift-refused check launched a process", err)
			}
			if slot, _, err := e.Store.Slot(context.Background()); err != nil || slot != "" || r.ActiveCheckID != "" {
				t.Fatal("known no-spawn failure retained capacity", err)
			}
		})
	}
}

func TestCheckExecutionRejectsSelfConsistentForeignRendering(t *testing.T) {
	for _, changed := range []string{"request", "digest", "envelope"} {
		t.Run(changed, func(t *testing.T) {
			e, runID := checkExecutionFixture(t, "pass", 10000)
			prepared := prepareCheckExecution(t, e, runID)
			r, before, err := e.load(context.Background(), runID)
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(prepared.Workspace, prepared.Context.Rendering.Path))
			if err != nil || !json.Valid(data) {
				t.Fatal("invalid prepared rendering", err)
			}
			switch changed {
			case "request":
				request, err := ParseCheckRequest(prepared.Request)
				if err != nil {
					t.Fatal(err)
				}
				request.AdmissionID = newID("admission")
				other, err := json.MarshalIndent(request, "", "  ")
				if err != nil {
					t.Fatal(err)
				}
				data = bytes.Replace(data, prepared.Request, append(other, '\n'), 1)
			case "digest":
				original := []byte(`"check_request_digest":"` + rawDigest(prepared.Request) + `"`)
				other := []byte(`"check_request_digest":"` + rawDigest([]byte("another request")) + `"`)
				data = bytes.Replace(data, original, other, 1)
			case "envelope":
				data = bytes.TrimSpace(data)
				data = append(data[:len(data)-1], []byte(`,"envelope":{}}`)...)
			}
			if !json.Valid(data) {
				t.Fatal("fixture did not preserve a valid rendering JSON object")
			}
			schema := builtinRef(r.Definitions, "core:schema/context-json")
			producer := map[string]any{"kind": "authority", "authority_id": r.AuthorityID, "command_id": newID("command"), "port": "context"}
			artifact, err := e.putArtifact(data, "json", &schema, newID("artifact"), producer, nil, r.registry())
			if err != nil {
				t.Fatal(err)
			}
			prepared.Context.Rendering.Ref = artifact.Ref()
			manifest, err := canonical(prepared.Context)
			if err != nil {
				t.Fatal(err)
			}
			for path, contents := range map[string][]byte{"context.json": manifest, prepared.Context.Rendering.Path: data} {
				if err := os.WriteFile(filepath.Join(prepared.Workspace, path), contents, 0600); err != nil {
					t.Fatal(err)
				}
			}
			_, err = e.admitCheck(context.Background(), r, before, newID("command"), prepared)
			if driverFailureCode(err, "") != "check_context_drift" {
				t.Fatal("self-consistent rendering with another bootstrap was admitted", err)
			}
			_, after, err := e.load(context.Background(), runID)
			if err != nil || !bytes.Equal(before.Snapshot.Data, after.Snapshot.Data) || before.Snapshot.Version != after.Snapshot.Version {
				t.Fatal("invalid rendering mutated the Run", err)
			}
			if slot, _, err := e.Store.Slot(context.Background()); err != nil || slot != "" {
				t.Fatal("invalid rendering acquired execution capacity", slot, err)
			}
		})
	}
}

func TestCheckExecutionStopsFenceAdmissionAndDispatch(t *testing.T) {
	for _, phase := range []string{"admission", "dispatch"} {
		for _, kind := range []string{"pause", "cancel"} {
			t.Run(phase+"/"+kind, func(t *testing.T) {
				e, runID := checkExecutionFixture(t, "pass", 10000)
				prepared := prepareCheckExecution(t, e, runID)
				var check *CheckExecution
				if phase == "dispatch" {
					check = admitCheckExecution(t, e, runID, prepared)
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
				if _, err := controller.Restrict(context.Background(), RestrictCommand{SchemaVersion: "1", CommandID: newID("command"), Scope: "run", ScopeID: runID, Kind: kind, Reason: "stop before check " + phase}); err != nil {
					t.Fatal(err)
				}
				if phase == "admission" {
					_, err = e.admitCheck(context.Background(), stale, view, newID("command"), prepared)
					if driverFailureCode(err, "") != "version_conflict" {
						t.Fatal("stale check admission crossed a committed restriction", err)
					}
				} else {
					executeCheckExecution(t, e, runID, check.ID)
				}
				r := driverRun(t, e, runID)
				if phase == "admission" && (len(r.CheckExecutions) != 0 || r.ControlTransitions != stale.ControlTransitions) {
					t.Fatal("rejected check admission left a record or budget charge")
				}
				if phase == "dispatch" {
					current := r.CheckExecutions[check.ID]
					if current.Dispatch != nil || current.Started != nil || current.Report != nil {
						t.Fatal("stopped check dispatched or produced evidence")
					}
					if kind == "cancel" && (current.Status != "cancelled" || current.Settled == nil || r.ActiveCheckID != "") {
						t.Fatal("known no-spawn cancellation did not settle")
					}
					if kind == "pause" && (current.Status != "pending" || current.Settled != nil || r.ActiveCheckID != check.ID || !r.ResumeRequired) {
						t.Fatal("pause lost the pending check or its separate resume gate")
					}
				}
				if _, err := os.Stat(filepath.Join(prepared.Workspace, "launches")); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("stopped check actually launched", err)
				}
				wantSlot := ""
				if phase == "dispatch" && kind == "pause" {
					wantSlot = check.ID
				}
				if slot, _, err := e.Store.Slot(context.Background()); err != nil || slot != wantSlot {
					t.Fatal("stopped check has an incorrect authority slot", slot, err)
				}
			})
		}
	}
}

func TestCheckExecutionPendingClockIsNeverRebased(t *testing.T) {
	for _, mode := range []string{"new_session", "elapsed"} {
		t.Run(mode, func(t *testing.T) {
			e, runID := checkExecutionFixture(t, "pass", 10000)
			prepared := prepareCheckExecution(t, e, runID)
			check := admitCheckExecution(t, e, runID, prepared)
			failure := "deadline_clock_unqualified"
			if mode == "new_session" {
				e.clock = newClock()
			} else {
				// A deterministic monotonic-domain test, not hardware suspend
				// evidence. Neither saved deadline nor request bytes are rewritten.
				e.clock.start = e.clock.start.Add(-time.Hour)
				failure = "attempt_deadline_expired"
			}
			executeCheckExecution(t, e, runID, check.ID)
			r := driverRun(t, e, runID)
			current := r.CheckExecutions[check.ID]
			if current.Status != "failed" || current.Failure != failure || current.Settled == nil || current.Dispatch != nil || current.Started != nil || current.Deadline != check.Deadline || current.RequestBytes != check.RequestBytes {
				t.Fatalf("unqualified or elapsed deadline restarted: %+v", current)
			}
			if _, err := os.Stat(filepath.Join(check.Workspace, "launches")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("invalid clock launched a check", err)
			}
			if slot, _, err := e.Store.Slot(context.Background()); err != nil || slot != "" {
				t.Fatal("known no-spawn deadline refusal retained capacity", err)
			}
		})
	}
}

func TestCheckExecutionQuotaProtectsAdmissionButNotSettlement(t *testing.T) {
	for _, boundary := range []string{"admission", "settlement"} {
		t.Run(boundary, func(t *testing.T) {
			e, runID := checkExecutionFixture(t, "pass", 10000)
			prepared := prepareCheckExecution(t, e, runID)
			var check *CheckExecution
			if boundary == "settlement" {
				check = admitCheckExecution(t, e, runID, prepared)
			}
			before, view, err := e.load(context.Background(), runID)
			if err != nil {
				t.Fatal(err)
			}
			usage, err := e.Store.StorageUsage(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if err := e.Store.Close(); err != nil {
				t.Fatal(err)
			}
			e.Store, err = local.OpenStore(filepath.Join(e.Root, e.Config.Configuration.StateRoot), local.StoreOptions{EventTypes: EventTypes, SoftLimitBytes: max(64<<10, usage.AllocatedBytes)})
			if err != nil {
				t.Fatal(err)
			}
			if boundary == "admission" {
				_, err = e.admitCheck(context.Background(), before, view, newID("command"), prepared)
				if driverFailureCode(err, "") != "storage_budget_exhausted" {
					t.Fatal("new checker crossed the actual storage quota", err)
				}
				after, current, err := e.load(context.Background(), runID)
				if err != nil || !bytes.Equal(current.Snapshot.Data, view.Snapshot.Data) || current.Snapshot.Version != view.Snapshot.Version || after.ControlTransitions != before.ControlTransitions {
					t.Fatal("quota refusal mutated the Run", err)
				}
				if _, err := e.Restrict(context.Background(), RestrictCommand{SchemaVersion: "1", CommandID: newID("command"), Scope: "run", ScopeID: runID, Kind: "cancel", Reason: "control remains available above work quota"}); err != nil {
					t.Fatal(err)
				}
			} else {
				executeCheckExecution(t, e, runID, check.ID)
				after := driverRun(t, e, runID)
				settled := after.CheckExecutions[check.ID]
				if settled.Status != "completed" || settled.Report == nil || settled.Report.Status != "pass" || settled.Settled == nil || !settled.ProcessOutcome.GroupEmpty {
					t.Fatalf("quota prevented actual admitted process settlement: %+v", settled)
				}
			}
			if slot, _, err := e.Store.Slot(context.Background()); err != nil || slot != "" {
				t.Fatal("quota left an unnecessary execution slot", slot, err)
			}
			if err := e.Store.Verify(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCheckExecutionNativeCancellationAndOriginalDeadline(t *testing.T) {
	for _, boundary := range []string{"cancel", "deadline"} {
		t.Run(boundary, func(t *testing.T) {
			// The checker is a separate race-instrumented test binary. Its
			// fixture allowance covers verified startup before cancellation;
			// production checker deadlines are not changed by this test.
			mode, timeout := "report_then_wait", int64(30000)
			settleWait := 10 * time.Second
			if boundary == "deadline" {
				mode, timeout = "wait", 10000
				settleWait = 20 * time.Second
			}
			e, runID := checkExecutionFixture(t, mode, timeout)
			check := admitCheckExecution(t, e, runID, prepareCheckExecution(t, e, runID))
			lock, err := e.driverLock(runID)
			if err != nil {
				t.Fatal(err)
			}
			defer lock.Close()
			r, view, err := e.load(context.Background(), runID)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			finished, exited := make(chan error, 1), make(chan struct{})
			go func() {
				defer close(exited)
				finished <- e.executePendingCheck(ctx, r, view, check)
			}()
			defer func() {
				cancel()
				select {
				case <-exited:
				case <-time.After(10 * time.Second):
					t.Error("checker did not finish its owned process cleanup")
				}
			}()
			driverWait(t, e, runID, func(r Run) bool {
				current := r.CheckExecutions[check.ID]
				marker := "launches"
				if boundary == "cancel" {
					marker = "report-written"
				}
				_, err := os.Stat(filepath.Join(check.Workspace, marker))
				return current.Started != nil && current.Settled == nil && err == nil
			})
			if boundary == "cancel" {
				controller, err := Open(e.Root, false)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = controller.Close() })
				if _, err := controller.Restrict(context.Background(), RestrictCommand{SchemaVersion: "1", CommandID: newID("command"), Scope: "run", ScopeID: runID, Kind: "cancel", Reason: "cancel even though the checker already wrote a positive report"}); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case err := <-finished:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(settleWait):
				t.Fatal("bounded checker did not settle")
			}
			after := driverRun(t, e, runID)
			settled := after.CheckExecutions[check.ID]
			if settled.Settled == nil || settled.Report != nil || settled.ProcessOutcome == nil || !settled.ProcessOutcome.WaitReturned || !settled.ProcessOutcome.GroupEmpty || settled.ProcessOutcome.Uncertain || settled.Deadline != check.Deadline || settled.RequestBytes != check.RequestBytes {
				t.Fatalf("check stop lost proven settlement or changed its original allowance: %+v", settled)
			}
			if boundary == "cancel" {
				if settled.Status != "cancelled" || settled.Failure != "cancelled" || settled.ReportBytes == nil {
					t.Fatal("cancellation became a semantic rejection or lost captured stdout")
				}
				data, err := e.Blobs.Read(*settled.ReportBytes)
				request, readErr := e.Blobs.Read(settled.RequestBytes)
				report, parseErr := ParseCheckResult(data, request)
				if err != nil || readErr != nil || parseErr != nil || report.Status != "pass" {
					t.Fatal("fixture did not write a valid positive report before cancellation", err, readErr, parseErr)
				}
			} else {
				if settled.Status != "failed" || settled.Failure != "check_deadline_expired" && settled.Failure != "attempt_deadline_expired" && settled.Failure != "runtime_limit" && settled.Failure != "deadline_clock_rollback" {
					t.Fatalf("original deadline did not stop the actual process: %+v", settled)
				}
			}
			if slot, _, err := e.Store.Slot(context.Background()); err != nil || slot != "" || after.ActiveCheckID != "" {
				t.Fatal("settled cancelled/expired check retained its slot", slot, err)
			}
		})
	}
}

// This second real Run shares only immutable pins and input artifacts. Mutable
// ownership, pending acceptance and scoped identities are freshly constructed.
func checkExecutionPeerRun(t *testing.T, e *Engine, sourceRunID string) string {
	base := driverRun(t, e, sourceRunID)
	if len(base.CheckExecutions) != 0 || len(base.Attempts) != 0 || len(base.Activations) != 0 {
		t.Fatal("peer fixture must be made before any executable work")
	}
	plan, err := base.plan()
	if err != nil {
		t.Fatal(err)
	}
	runID, invocationID := newID("run"), newID("invocation")
	zero := int64(0)
	_, err = e.apply(context.Background(), e.owner, newID("command"), runID, "run.created", map[string]any{"workflow_ref": base.WorkflowRef, "package_lock_ref": base.LockRef}, &zero, local.CommandCAS, func(r *Run, _ local.Snapshot, obs Observation) (local.Change, error) {
		*r = base
		r.ID, r.RootInvocationID, r.Status = runID, invocationID, "ready"
		r.Created, r.LastObserved = obs, obs
		r.PendingAcceptance = nil
		r.CheckExecutions = map[string]*CheckExecution{}
		r.Transitions = []StateChange{}
		r.Invocations = map[string]*Invocation{invocationID: {ID: invocationID, RunID: runID, WorkflowRef: r.WorkflowRef, Status: "ready", Inputs: r.Inputs, Outputs: map[string]ArtifactRef{}, Ready: []string{}, Created: obs}}
		return local.Change{}, r.beginWorkflowInputAcceptance(plan, invocationID, obs)
	})
	if err != nil {
		t.Fatal(err)
	}
	return runID
}

// This includes a fresh subprocess's cold/race startup and its storage work,
// not just checker execution. The admitted production deadlines stay unchanged.
func checkExecutionCrashWait(t *testing.T, e *Engine, runID string, ready func(Run) bool) Run {
	t.Helper()
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		r := driverRun(t, e, runID)
		if message, err := os.ReadFile(filepath.Join(e.Root, "check-helper-error")); err == nil {
			t.Fatalf("owned check helper failed: %s", message)
		}
		if ready(r) {
			return r
		}
		select {
		case <-deadline.C:
			states := map[string]string{}
			for id, check := range r.CheckExecutions {
				states[id] = check.Status + ":" + check.Failure
			}
			t.Fatalf("check crash boundary not reached: Run=%s checks=%v", r.Status, states)
		case <-ticker.C:
		}
	}
}

func TestCheckExecutionCrashRecoveryNeverRepeatsDispatch(t *testing.T) {
	for _, boundary := range []string{"before_start", "running", "settled"} {
		t.Run(boundary, func(t *testing.T) {
			mode := "pass"
			if boundary == "running" {
				mode = "crash_short"
			}
			e, runID := checkExecutionFixture(t, mode, 10000)
			peerID := checkExecutionPeerRun(t, e, runID)
			if err := os.WriteFile(filepath.Join(e.Root, "check-crash-boundary"), []byte(boundary), 0600); err != nil {
				t.Fatal(err)
			}
			_, crash := choiceCrashDriver(t, "TestCheckExecutionCrashHelper", "CHECK_CRASH_HELPER", e.Root, runID)
			before := checkExecutionCrashWait(t, e, runID, func(r Run) bool {
				for _, check := range r.CheckExecutions {
					if boundary == "running" {
						_, err := os.Stat(filepath.Join(check.Workspace, "launches"))
						return check.Started != nil && check.Settled == nil && err == nil
					}
					_, err := os.Stat(filepath.Join(e.Root, "check-boundary-ready"))
					if err == nil && boundary == "settled" {
						// The marker can appear after this poll loaded an older
						// verifying snapshot. Wait for the committed record too.
						return check.Status == "completed" && check.Report != nil && check.Report.Status == "pass" && check.Settled != nil && check.ProcessOutcome != nil && check.ProcessOutcome.WaitReturned && check.ProcessOutcome.GroupEmpty
					}
					return err == nil
				}
				return false
			})
			// Take the cut after the readiness marker, not an earlier poll snapshot.
			before, cut, err := e.load(context.Background(), runID)
			if err != nil || len(before.CheckExecutions) != 1 {
				t.Fatal("crash helper did not establish one check", err)
			}
			var original *CheckExecution
			for _, check := range before.CheckExecutions {
				original = check
			}
			if boundary == "running" {
				if original.Process == nil {
					t.Fatal("actual check process identity was not observed")
				}
				if probe := local.ProbeProcess(*original.Process); probe.State != "present" || probe.GroupAlive == nil || !*probe.GroupAlive {
					t.Fatalf("checker was not actually live before owner crash: %+v", probe)
				}
			}
			if boundary == "settled" && (original.Status != "completed" || original.Report == nil || original.Settled == nil || original.ProcessOutcome == nil || !original.ProcessOutcome.GroupEmpty) {
				t.Fatal("helper has no actual completed checker settlement")
			}
			crash()
			if err := e.Close(); err != nil {
				t.Fatal(err)
			}
			e, err = Open(e.Root, false)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = e.Close() })
			lock, err := e.driverLock(runID)
			if err != nil {
				t.Fatal(err)
			}
			defer lock.Close()
			for retry := 0; retry < 2; retry++ {
				r, view, err := e.load(context.Background(), runID)
				if err != nil {
					t.Fatal(err)
				}
				err = e.executePendingCheck(context.Background(), r, view, r.CheckExecutions[original.ID])
				if boundary == "settled" {
					if err != nil {
						t.Fatal(err)
					}
				} else if err == nil || !strings.Contains(err.Error(), "recovery_required") {
					t.Fatal("recorded dispatch was blindly retried", err)
				}
			}
			after, current, err := e.load(context.Background(), runID)
			if err != nil {
				t.Fatal(err)
			}
			saved := after.CheckExecutions[original.ID]
			if saved.RequestBytes != original.RequestBytes || saved.Deadline != original.Deadline || after.ControlTransitions != before.ControlTransitions || len(after.CheckExecutions) != 1 || len(after.Attempts) != 0 || len(after.Steps) != 0 {
				t.Fatal("recovery changed request identity, budget, or execution population")
			}
			if boundary == "settled" {
				if !bytes.Equal(cut.Snapshot.Data, current.Snapshot.Data) || cut.Snapshot.Version != current.Snapshot.Version {
					t.Fatal("completed check was replayed after owner crash")
				}
			} else {
				if saved.Status != "uncertain" || saved.Settled != nil || saved.Report != nil || !after.HasUnresolvedEffects || after.Status != "uncertain" || after.ActiveCheckID != original.ID {
					t.Fatal("lost checker ownership did not hold the whole Run barrier")
				}
				peer, peerCut, err := e.load(context.Background(), peerID)
				if err != nil {
					t.Fatal(err)
				}
				prepared := prepareCheckExecution(t, e, peerID)
				_, err = e.admitCheck(context.Background(), peer, peerCut, newID("command"), prepared)
				if driverFailureCode(err, "") != "capacity_conflict" {
					t.Fatal("uncertain check released the actual authority slot to another Run", err)
				}
				if slot, owner, err := e.Store.Slot(context.Background()); err != nil || slot != original.ID || owner != runID {
					t.Fatal("uncertainty no longer owns its authority capacity", slot, owner, err)
				}
				unchanged, _, err := e.load(context.Background(), peerID)
				if err != nil || len(unchanged.CheckExecutions) != 0 || unchanged.ControlTransitions != peer.ControlTransitions {
					t.Fatal("capacity-conflicting check left a record or budget charge", err)
				}
			}
			if boundary == "running" {
				// The orphan has its own short bound. Never signal a recovered PID.
				checkExecutionCrashWait(t, e, runID, func(Run) bool {
					_, err := os.Stat(filepath.Join(original.Workspace, "check-self-finished"))
					return err == nil && local.ProbeProcess(*original.Process).State == "absent"
				})
				if current := driverRun(t, e, runID); current.CheckExecutions[original.ID].Settled != nil || !current.HasUnresolvedEffects {
					t.Fatal("later absence was treated as retroactive settlement proof")
				}
			}
			launches, readErr := os.ReadFile(filepath.Join(original.Workspace, "launches"))
			if boundary == "before_start" {
				if !errors.Is(readErr, os.ErrNotExist) {
					t.Fatal("saved pre-spawn dispatch was launched by recovery", readErr)
				}
			} else if readErr != nil || string(launches) != "launch\n" {
				t.Fatal("check process was launched more than once", readErr)
			}
			if err := e.Store.Verify(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCheckExecutionCrashHelper(t *testing.T) {
	if os.Getenv("CHECK_CRASH_HELPER") != "1" {
		return
	}
	e, err := Open(os.Args[len(os.Args)-2], false)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	runID := os.Args[len(os.Args)-1]
	lock, err := e.driverLock(runID)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	boundary, err := os.ReadFile(filepath.Join(e.Root, "check-crash-boundary"))
	if err != nil {
		t.Fatal(err)
	}
	check := admitCheckExecution(t, e, runID, prepareCheckExecution(t, e, runID))
	r, view, err := e.load(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if string(boundary) == "before_start" {
		// Commit the real dispatch boundary in an owned child, then stop this
		// test harness before RunProcess. No OS start evidence is manufactured.
		_, err = e.apply(context.Background(), e.owner, newID("command"), runID, "check.dispatching", map[string]any{"check_execution_id": check.ID}, &view.Snapshot.Version, local.CommandCAS, func(r *Run, _ local.Snapshot, obs Observation) (local.Change, error) {
			current, err := activeCheck(r, check.ID, "")
			if err != nil {
				return local.Change{}, err
			}
			if err := checkPendingBinding(*r, current.Request); err != nil {
				return local.Change{}, err
			}
			current.Dispatch, current.Status, current.TokenHash = &obs, "dispatching", rawDigest([]byte("owned-test-dispatch-token"))
			return local.Change{}, nil
		})
	} else {
		err = e.executePendingCheck(context.Background(), r, view, check)
	}
	if err != nil {
		_ = os.WriteFile(filepath.Join(e.Root, "check-helper-error"), []byte(err.Error()), 0600)
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.Root, "check-boundary-ready"), []byte(check.ID), 0600); err != nil {
		t.Fatal(err)
	}
	// A short self bound also prevents a stranded helper if its parent disappears.
	time.Sleep(10 * time.Second)
	os.Exit(80)
}

func TestCheckExecutionWorkerHelper(t *testing.T) {
	if os.Getenv("CHECK_EXECUTION_HELPER") != "1" {
		return
	}
	mode := os.Getenv("CHECK_EXECUTION_MODE")
	stop := make(chan os.Signal, 1)
	if mode == "report_then_wait" {
		signal.Notify(stop, syscall.SIGTERM)
	}
	requestBytes, err := io.ReadAll(io.LimitReader(os.Stdin, MaxCheckWireBytes+1))
	if err != nil {
		os.Exit(40)
	}
	request, err := ParseCheckRequest(requestBytes)
	if err != nil {
		os.Exit(41)
	}
	data, err := os.ReadFile(os.Getenv("PRIFLY_CONTEXT_FILE"))
	var transport ContextManifest
	if err != nil || json.Unmarshal(data, &transport) != nil {
		os.Exit(42)
	}
	input := transport.Inputs["value"]
	if request.Boundary == "artifact_publication" {
		input = transport.Inputs["subject_000"]
	}
	data, err = os.ReadFile(input.Path)
	if err != nil || request.Boundary != "artifact_publication" && string(data) != "sealed check subject" {
		os.Exit(43)
	}
	data, err = os.ReadFile(transport.Rendering.Path)
	var rendering struct {
		Request json.RawMessage `json:"check_request"`
		Digest  string          `json:"check_request_digest"`
	}
	if err != nil || json.Unmarshal(data, &rendering) != nil || rendering.Digest != rawDigest(requestBytes) || !bytes.Equal(bytes.TrimSpace(rendering.Request), bytes.TrimSpace(requestBytes)) {
		os.Exit(46)
	}
	file, err := os.OpenFile("launches", os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0600)
	if err != nil {
		os.Exit(44)
	}
	_, _ = file.WriteString("launch\n")
	_ = file.Close()
	if mode == "wait" {
		for {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if mode == "crash_short" {
		_ = os.NewFile(3, "result").Close()
		_ = os.Stdout.Close()
		_ = os.Stderr.Close()
		time.Sleep(3 * time.Second)
		_ = os.WriteFile("check-self-finished", []byte("normal self exit"), 0600)
		os.Exit(0)
	}
	result := CheckResult{SchemaVersion: CheckResultVersion, CheckID: request.CheckID, RunID: request.RunID, RequestDigest: rawDigest(requestBytes), Status: "pass", Summary: "checked the sealed subject", Limitations: []string{}}
	switch mode {
	case "fail", "inconclusive":
		result.Status = mode
	case "malformed":
		_, _ = os.Stdout.WriteString("{")
		os.Exit(0)
	case "missing":
		os.Exit(0)
	case "wrong_request":
		result.RequestDigest = rawDigest([]byte("not the admitted request"))
	case "wrong_run":
		result.RunID = newID("run")
	case "fd3":
		channel := os.NewFile(3, "forbidden-check-channel")
		_, _ = channel.WriteString("{}")
		_ = channel.Close()
	case "stdout_limit":
		_, _ = os.Stdout.Write(bytes.Repeat([]byte("x"), MaxCheckWireBytes+1))
		os.Exit(0)
	case "stderr_limit":
		_, _ = os.Stderr.Write(bytes.Repeat([]byte("x"), (64<<10)+1))
	}
	data, err = json.Marshal(result)
	if err != nil {
		os.Exit(45)
	}
	if mode == "extra_field" {
		data = append(data[:len(data)-1], []byte(`,"claim":"semantic_review"}`)...)
	}
	_, _ = os.Stdout.Write(data)
	if mode == "report_then_wait" {
		_ = os.WriteFile("report-written", nil, 0600)
		select {
		case <-stop:
		case <-time.After(10 * time.Second):
		}
	}
	if mode == "nonzero" {
		os.Exit(7)
	}
	os.Exit(0)
}
