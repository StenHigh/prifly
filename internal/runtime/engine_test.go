package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
	"github.com/stenhigh/prifly/internal/purity"
)

// knownImpureTransforms is the baseline this build still carries. Both are
// deliberate and named here so that every other impure transform fails the
// run rather than joining an unexamined pile.
var knownImpureTransforms = []string{
	// A plan compiled for the first time inside the command that pinned it:
	// run.created and context pinning have no earlier point at which its key
	// exists. Every later command reads the compiled plan from the cache.
	"runtime.Run.plan(",
	// A stream delivery is sealed while its assignment is being decided. That
	// belongs to publication redesign, not to this change.
	"runtime.(*Engine).createStreamAssignment(",
}

func TestMain(m *testing.M) {
	if handled, code := flow.SchemaWorker(os.Args[1:], os.Stdin, os.Stdout); handled {
		os.Exit(code)
	}
	// A transform decides from its snapshot and its command. Any file read,
	// artifact seal or subprocess reached from inside one is a fact the caller
	// should have computed first. Violations are collected rather than
	// panicked so one run reports every one of them with its stack.
	var impure sync.Map
	purity.Impure = func(operation string) {
		var buffer [8 << 10]byte
		frames := string(buffer[:runtime.Stack(buffer[:], false)])
		for _, known := range knownImpureTransforms {
			if strings.Contains(frames, known) {
				return
			}
		}
		impure.Store(operation+"\n"+frames, true)
	}
	code := m.Run()
	impure.Range(func(key, _ any) bool {
		code = 1
		fmt.Fprintf(os.Stderr, "impure transform: %s\n", key)
		return true
	})
	os.Exit(code)
}

// This fixture is a real empty installation and an independently authored
// control-only workflow. It never imports the original project's packages.
func emptyRuntime(t *testing.T) (*Engine, StartOptions) {
	t.Helper()
	root := t.TempDir()
	if err := Init(root); err != nil {
		t.Fatal(err)
	}
	e, err := Open(root, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close() })
	defs, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	w := flow.WorkflowRevision{SchemaVersion: "1", ID: "test:workflow/empty", Version: "1.0.0", Title: "Explicit empty local workflow", Inputs: map[string]flow.InputPort{}, Outputs: map[string]flow.OutputPort{}, AllowedOutcomes: []string{"no_work"}, Limits: flow.Limits{MaxStepInstances: 1, MaxControlTransitions: 4, MaxParallelism: 1, MaxChildDepth: 0}, PolicyRef: builtinRef(defs, "core:policy/local")}
	w.Definition.Entry = "done"
	w.Definition.Stages = map[string]flow.Stage{"done": {Kind: "finish", Outcome: "no_work", OutputBindings: map[string]flow.Binding{}}}
	writeRuntimeJSON(t, filepath.Join(root, "workflows/empty.json"), w)
	brief := Brief{"1", "test:brief/empty", "An explicit empty control path", "Return no_work without a worker", []string{"Local state only"}, []string{"Network, AI and external effects"}, []string{"Finish with no_work"}, []ArtifactRef{}, []string{}, "explicit"}
	writeRuntimeJSON(t, filepath.Join(root, "brief.json"), brief)
	return e, StartOptions{CommandID: "command:empty", WorkflowFile: "workflows/empty.json", BriefFile: "brief.json", Inputs: map[string]string{}}
}
func writeRuntimeJSON(t *testing.T, path string, value any) {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
}
func rejectionCode(t *testing.T, err error, code string) {
	t.Helper()
	var rejected *local.Rejection
	if !errors.As(err, &rejected) || rejected.Code != code {
		t.Fatalf("wanted %s, got %v", code, err)
	}
}

func TestEmptyInstallationAndReadOnlyPlanning(t *testing.T) {
	e, options := emptyRuntime(t)
	ctx := context.Background()
	info, err := e.Check(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info["local_definitions"] != 0 {
		t.Fatal("init installed workflow definitions")
	}
	if _, ok := info["local_worker_socket"].(bool); !ok {
		t.Fatalf("doctor did not report local worker socket availability: %+v", info)
	}
	if _, err := e.Preview(PreviewOptions{WorkflowFile: "workflows/missing.json"}); err == nil {
		t.Fatal("unknown workflow accepted")
	}
	before, cut, err := e.Store.ReadAll(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := e.Preview(PreviewOptions{WorkflowFile: options.WorkflowFile})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Admission || len(preview.Sequence) != 1 {
		t.Fatal("preview is not a read-only empty plan")
	}
	after, afterCut, err := e.Store.ReadAll(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 0 || len(after) != 0 || cut != afterCut {
		t.Fatal("preview created state")
	}
	if err := Init(e.Root); err == nil {
		t.Fatal("init overwrote a project")
	}
	reader, err := Open(e.Root, true)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if _, err := reader.Start(ctx, options); !errors.Is(err, local.ErrReadOnly) {
		t.Fatalf("read-only start: %v", err)
	}
	if _, err := reader.Check(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestStartExactRetrySnapshotAndSourceDrift(t *testing.T) {
	e, options := emptyRuntime(t)
	ctx := context.Background()
	first, err := e.Start(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	second, err := e.Start(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Duplicate || first.Receipt.Version != second.Receipt.Version || first.Receipt.EventSeq != second.Receipt.EventSeq || !bytes.Equal(first.Receipt.Result, second.Receipt.Result) {
		t.Fatal("exact retry changed the accepted command")
	}
	r, read, err := e.load(ctx, first.Receipt.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Activations) != 1 || len(r.Steps) != 0 || len(r.Attempts) != 0 || r.RootInvocationID == "" || read.Snapshot.Version != 1 {
		t.Fatal("control stage acquired fake execution identities")
	}
	view, err := e.View(ctx, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Timing.AsOf != view.AsOf || view.Timing.RunID != view.Run.ID || len(view.Run.Definitions) != 0 || view.Run.Workflow != nil || view.Run.Executors != nil {
		t.Fatal("read view detached timing or leaked internal definition/config bytes")
	}
	path := filepath.Join(e.Root, options.WorkflowFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	options.CommandID = "command:changed-bytes"
	if _, err := e.Start(ctx, options); err == nil || !strings.Contains(err.Error(), "definition_drift") {
		t.Fatalf("raw definition replacement accepted: %v", err)
	}
	if _, err := r.plan(); err != nil {
		t.Fatalf("mutable install changed historical plan: %v", err)
	}
	if err := e.Store.Verify(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestMonotonicStopExactReleaseAndSeparateResume(t *testing.T) {
	e, options := emptyRuntime(t)
	ctx := context.Background()
	started, err := e.Start(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	runID := started.Receipt.RunID
	stale := int64(0)
	pause := RestrictCommand{"1", "command:pause", "run", runID, "pause", "Owner review", &stale}
	stopped, err := e.Restrict(ctx, pause)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Resume(ctx, runID, "command:unsafe-resume", "Resume", stopped.Receipt.Version); err == nil {
		t.Fatal("resume released a pause")
	}
	r, _, err := e.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	ref := StopGeneration{r.Stops[0].ID, r.Stops[0].Generation}
	request := ReleaseRequest{"command:stale-release", runID, 0, []StopGeneration{ref}, "Reviewed exact stop"}
	_, err = e.Release(ctx, request)
	rejectionCode(t, err, "control_epoch_conflict")
	request.CommandID = "command:release"
	request.ExpectedControlEpoch = r.ControlEpoch
	released, err := e.Release(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := e.Release(ctx, request)
	if err != nil || !duplicate.Duplicate || duplicate.Receipt.Version != released.Receipt.Version {
		t.Fatalf("release retry not stable: %v", err)
	}
	r, _, err = e.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if kind, _ := nextKind(r); kind != "resume_required" {
		t.Fatalf("release itself resumed work: %s", kind)
	}
	// A new stop in the gap invalidates the following resume even with fresh CAS.
	pause.CommandID = "command:new-pause"
	if _, err := e.Restrict(ctx, pause); err != nil {
		t.Fatal(err)
	}
	r, read, err := e.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.Resume(ctx, runID, "command:blocked-resume", "Resume", read.Snapshot.Version)
	rejectionCode(t, err, "active_stop")
	request.CommandID = "command:release-second"
	request.ExpectedControlEpoch = r.ControlEpoch
	request.Stops = []StopGeneration{{r.Stops[1].ID, r.Stops[1].Generation}}
	released, err = e.Release(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.Resume(ctx, runID, "command:stale-resume", "Resume", 0)
	rejectionCode(t, err, "version_conflict")
	if _, err := e.Resume(ctx, runID, "command:resume", "Resume", released.Receipt.Version); err != nil {
		t.Fatal(err)
	}
	r, _, err = e.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if kind, _ := nextKind(r); kind != "stage" {
		t.Fatalf("resumed plan: %s", kind)
	}
}

func TestRootWorkflowRegistrationCannotBeShadowed(t *testing.T) {
	e, options := emptyRuntime(t)
	ctx := context.Background()
	preview, err := e.Preview(PreviewOptions{WorkflowFile: options.WorkflowFile})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(e.Root, options.WorkflowFile))
	if err != nil {
		t.Fatal(err)
	}
	registeredPath := "workflows/registered.json"
	if err := os.WriteFile(filepath.Join(e.Root, registeredPath), raw, 0600); err != nil {
		t.Fatal(err)
	}
	writeRuntimeJSON(t, filepath.Join(e.Root, "definitions.json"), RegistryFile{SchemaVersion: "1", Entries: []Definition{{Ref: preview.WorkflowRef, Kind: "workflow", Path: registeredPath}}})
	if _, err := e.Preview(PreviewOptions{WorkflowFile: options.WorkflowFile}); err != nil {
		t.Fatalf("same registered bytes refused: %v", err)
	}
	// A canonical-equivalent source still cannot silently replace registered
	// raw bytes. Preview and admission must agree, including before any Run.
	if err := os.WriteFile(filepath.Join(e.Root, options.WorkflowFile), append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Preview(PreviewOptions{WorkflowFile: options.WorkflowFile}); err == nil || !strings.Contains(err.Error(), "workflow_registry_conflict") {
		t.Fatalf("preview accepted shadowed registered root: %v", err)
	}
	if _, err := e.Start(ctx, options); err == nil || !strings.Contains(err.Error(), "workflow_registry_conflict") {
		t.Fatalf("start accepted shadowed registered root: %v", err)
	}
	runs, _, err := e.Store.ReadAll(ctx, 100)
	if err != nil || len(runs) != 0 {
		t.Fatalf("refused root created a Run: %v", err)
	}
}

func TestPreviewSealedInputsSurviveSourceReplacement(t *testing.T) {
	e, _ := driverProject(t, "duplicate", 30000)
	ctx := context.Background()
	input, err := e.ImportArtifact("source.txt", "blob", nil, "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	refs := map[string]ArtifactRef{"source": input.Ref()}
	_, before, err := e.Store.ReadAll(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := e.Preview(PreviewOptions{WorkflowFile: "workflows/driver.json", BriefFile: "brief.json", InputRefs: refs})
	if err != nil || preview.Validation.Inputs != "sealed_refs_verified" || preview.Inputs["source"] != input.Ref() || preview.Admission {
		t.Fatalf("sealed preview: %+v %v", preview, err)
	}
	_, after, err := e.Store.ReadAll(ctx, 100)
	if err != nil || before != after {
		t.Fatalf("preview wrote authority state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(e.Root, "replacement.txt"), []byte("different live bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(e.Root, "source.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("replacement.txt", filepath.Join(e.Root, "source.txt")); err != nil {
		t.Fatal(err)
	}
	options := StartOptions{CommandID: "command:reviewed-input", WorkflowFile: "workflows/driver.json", BriefFile: "brief.json", InputRefs: refs}
	started, err := e.Start(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(ctx, started.Receipt.RunID); err != nil {
		t.Fatal(err)
	}
	r, _, err := e.load(ctx, started.Receipt.RunID)
	if err != nil || r.Status != "completed" || r.Inputs["source"] != input.Ref() {
		t.Fatalf("reviewed input did not complete unchanged: %+v %v", r, err)
	}
	for _, attempt := range r.Attempts {
		slot := attempt.Context.Inputs["source"]
		data, err := os.ReadFile(filepath.Join(attempt.Workspace, slot.Path))
		if err != nil || string(data) != "pinned input" || slot.Ref != input.Ref() {
			t.Fatalf("replacement bytes reached the executor workspace: %q %v", data, err)
		}
	}
	options.CommandID, options.InputRefs, options.Inputs = "command:unsafe-live-input", nil, map[string]string{"source": "source.txt"}
	if _, err := e.Start(ctx, options); err == nil {
		t.Fatal("live symlink bypassed regular-file checks")
	}
}

func TestRejectedBriefNeedsExplicitCorrectionAndConfirmation(t *testing.T) {
	e, options := emptyRuntime(t)
	ctx := context.Background()
	var brief Brief
	data, err := os.ReadFile(filepath.Join(e.Root, options.BriefFile))
	if err != nil || json.Unmarshal(data, &brief) != nil {
		t.Fatal("read fixture brief", err)
	}
	brief.Subject, brief.Confirmation = "Pricing engine", "unconfirmed"
	brief.Assumptions = []string{"Owner rejected this draft: Pri-Fly orchestration was requested"}
	writeRuntimeJSON(t, filepath.Join(e.Root, options.BriefFile), brief)
	preview, err := e.Preview(PreviewOptions{WorkflowFile: options.WorkflowFile, BriefFile: options.BriefFile})
	if err != nil || preview.Brief.Subject != "Pricing engine" || preview.Brief.Confirmation != "unconfirmed" {
		t.Fatalf("preview hid the wrong subject or refusal: %+v %v", preview, err)
	}
	if _, err := e.Start(ctx, options); err == nil {
		t.Fatal("unconfirmed wrong subject was admitted")
	}
	brief.Subject = "Pri-Fly universal step orchestration"
	// Changing the subject is not confirmation. A client cannot invent an
	// exemption that the installed local policy does not provide either.
	for _, state := range []string{"unconfirmed", "not_required_by_policy", "standing_grant"} {
		brief.Confirmation = state
		writeRuntimeJSON(t, filepath.Join(e.Root, options.BriefFile), brief)
		if _, err := e.Start(ctx, options); err == nil {
			t.Fatalf("unconfirmed correction admitted via %s", state)
		}
	}
	runs, _, err := e.Store.ReadAll(ctx, 100)
	if err != nil || len(runs) != 0 {
		t.Fatal("rejected briefs created work", err)
	}
	brief.Confirmation = "explicit"
	writeRuntimeJSON(t, filepath.Join(e.Root, options.BriefFile), brief)
	if _, err := e.Start(ctx, options); err != nil {
		t.Fatalf("owner's corrected and confirmed brief refused: %v", err)
	}
}

func TestCoreDiagnosticOccurrenceIsSharedByEventsProblemsAndReports(t *testing.T) {
	e, first := driverProject(t, "nonzero", 30000)
	ctx := context.Background()
	second := driverStart(t, e)
	ids := map[string]bool{}
	for _, runID := range []string{first, second} {
		if err := e.Drive(ctx, runID); err != nil {
			t.Fatal(err)
		}
		r := driverRun(t, e, runID)
		if r.Status != "failed" || len(r.Diagnostics) != 1 || r.Diagnostics[0].Code != "nonzero_exit" {
			t.Fatalf("actual failure diagnostic: %+v", r.Diagnostics)
		}
		d := r.Diagnostics[0]
		ids[d.ID] = true
		if d.Origin != "core" || d.AttemptID == "" || len(d.CauseRefs) != 0 {
			t.Fatal("unknown cause invented or actual scope lost")
		}
		for range 2 {
			if err := recordDiagnostic(&r, d); err != nil || len(r.Diagnostics) != 1 {
				t.Fatalf("duplicate occurrence was counted: %v", err)
			}
			problem, _ := ProblemFor(&DiagnosticError{ID: d.ID, Err: errors.New("nonzero_exit: traceback\nframe 1\nframe 2")})
			if problem.CorrelationID != d.ID || strings.Contains(problem.Message, "traceback") {
				t.Fatal("Problem lost the original occurrence or leaked raw error text")
			}
		}
		changed := d
		changed.Message = "another occurrence cannot overwrite this one"
		rejectionCode(t, recordDiagnostic(&r, changed), "diagnostic_conflict")
		events, err := e.Events(ctx, runID, 0, 1000)
		if err != nil {
			t.Fatal(err)
		}
		linked := 0
		for _, event := range events.Events {
			if event.Type == "diagnostic.recorded" {
				var data struct {
					ID string `json:"diagnostic_id"`
				}
				if json.Unmarshal(event.Data, &data) != nil || data.ID != d.ID {
					t.Fatal("journal presentation points at a different occurrence")
				}
				linked++
			}
		}
		if linked != 1 {
			t.Fatalf("diagnostic links: %d", linked)
		}
		if err := e.Drive(ctx, runID); err != nil {
			t.Fatal(err)
		}
	}
	report, err := e.Telemetry(ctx, TelemetryQuery{SchemaVersion: TelemetryQueryVersion, Mode: "records", Metrics: []string{"core.diagnostics"}})
	if err != nil || len(report.Records) != 2 || len(ids) != 2 {
		t.Fatalf("two failures with one code were not two occurrences: %+v %v", report.Records, err)
	}
	for _, record := range report.Records {
		if len(record.Evidence) != 1 || !ids[record.Evidence[0]] {
			t.Fatal("report lost occurrence drilldown")
		}
	}
}

func TestCancelCannotBeReleasedOrTerminalReopened(t *testing.T) {
	e, options := emptyRuntime(t)
	ctx := context.Background()
	started, err := e.Start(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	runID := started.Receipt.RunID
	if _, err := e.Restrict(ctx, RestrictCommand{SchemaVersion: "1", CommandID: "command:cancel", Scope: "run", ScopeID: runID, Kind: "cancel", Reason: "Owner stop"}); err != nil {
		t.Fatal(err)
	}
	r, read, err := e.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.Release(ctx, ReleaseRequest{"command:no-cancel-release", runID, r.ControlEpoch, []StopGeneration{{r.Stops[0].ID, r.Stops[0].Generation}}, "Attempt release"})
	rejectionCode(t, err, "cancel_not_reversible")
	_, err = e.Resume(ctx, runID, "command:no-cancel-resume", "Attempt resume", read.Snapshot.Version)
	rejectionCode(t, err, "terminal_run")
}

func TestRootsAndUnknownProfileFailClosed(t *testing.T) {
	e, _ := emptyRuntime(t)
	for _, paths := range [][3]string{{".prifly/state", ".prifly/state/blobs", ".prifly/work"}, {".", ".prifly/artifacts", ".prifly/work"}, {".prifly/state", ".prifly/artifacts", ".prifly"}} {
		c := e.Config
		c.Configuration.StateRoot, c.Configuration.ArtifactRoot, c.Configuration.WorkspaceRoot = paths[0], paths[1], paths[2]
		writeRuntimeJSON(t, filepath.Join(e.Root, "prifly.json"), c)
		if opened, err := Open(e.Root, true); err == nil {
			opened.Close()
			t.Fatal("overlapping/unsafe roots accepted")
		}
	}
	c := e.Config
	c.Configuration.SemanticsProfile = "future/2"
	writeRuntimeJSON(t, filepath.Join(e.Root, "prifly.json"), c)
	if opened, err := Open(e.Root, false); err == nil {
		opened.Close()
		t.Fatal("unknown semantics profile opened for writing")
	}
	for _, mutate := range []func(*ProjectConfig){
		func(c *ProjectConfig) { c.DefaultPolicyRef.ID = "test:policy/other" },
		func(c *ProjectConfig) { c.AdapterBindings = map[string]flow.Ref{} },
		func(c *ProjectConfig) { c.AdapterBindings = map[string]flow.Ref{"local_process": c.DefaultPolicyRef} },
	} {
		c = e.Config
		mutate(&c)
		writeRuntimeJSON(t, filepath.Join(e.Root, "prifly.json"), c)
		if opened, err := Open(e.Root, true); err == nil {
			opened.Close()
			t.Fatal("unsupported policy or adapter silently ignored")
		}
	}
	writeRuntimeJSON(t, filepath.Join(e.Root, "prifly.json"), e.Config)
	link := filepath.Join(e.Root, "alias")
	if err := os.Symlink(filepath.Join(e.Root, ".prifly"), link); err != nil {
		t.Fatal(err)
	}
	c = e.Config
	c.Configuration.WorkspaceRoot = "alias/work"
	writeRuntimeJSON(t, filepath.Join(e.Root, "prifly.json"), c)
	if opened, err := Open(e.Root, false); err == nil {
		opened.Close()
		t.Fatal("symlink parent accepted")
	}
}

func TestAtomicWriteAndDriverOwnershipMetadata(t *testing.T) {
	e, _ := emptyRuntime(t)
	path := filepath.Join(e.Root, "exclusive")
	if err := writeExclusive(path, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := writeExclusive(path, []byte("second")); !errors.Is(err, os.ErrExist) {
		t.Fatalf("replacement: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "first" {
		t.Fatal("existing bytes changed")
	}
	lock, err := e.driverLock("run:first")
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if !e.driverLiveFor("run:first") || e.driverLiveFor("run:second") {
		t.Fatal("run confused with another authority driver")
	}
	if other, err := e.driverLock("run:second"); err == nil {
		other.Close()
		t.Fatal("second driver acquired ownership")
	}
}

func TestMaximumCommandIDDoesNotOverflowDerivedContracts(t *testing.T) {
	e, options := emptyRuntime(t)
	options.CommandID = "x" + strings.Repeat("a", 127)
	first, err := e.Start(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	c := RestrictCommand{SchemaVersion: "1", CommandID: "y" + strings.Repeat("b", 127), Scope: "run", ScopeID: first.Receipt.RunID, Kind: "pause", Reason: "Review"}
	if _, err := e.Restrict(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	r, _, err := e.load(context.Background(), first.Receipt.RunID)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range r.Stops {
		data, err := canonical(s.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err := flow.ValidateProtocol("Identifier", data); err != nil {
			t.Fatal(err)
		}
	}
}
