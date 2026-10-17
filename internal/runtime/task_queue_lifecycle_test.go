package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

func queueClaimFixture(t *testing.T, e *Engine) QueueTaskSelection {
	t.Helper()
	ctx := context.Background()
	result, err := e.QueueAdd(ctx, QueueAddRequest{CommandID: "command:queue-add", RequestBytes: queueTaskInput(t, "request")})
	queueSummaryResult(t, result, err)
	result, err = e.QueueClaimNext(ctx, QueueClaimRequest{CommandID: "command:queue-claim", Owner: "host:queue"})
	task := queueSummaryResult(t, result, err)
	selection, _, err := e.QueueSelection(ctx, task.ID, task.Claim.Owner, task.Revision, task.Claim.Generation)
	if err != nil {
		t.Fatal(err)
	}
	return selection
}
func queueReserveFixture(t *testing.T, e *Engine, selection QueueTaskSelection, command string) QueueTaskBinding {
	t.Helper()
	request := QueueReserveRequest{CommandID: command, Selection: selection, CommandDigest: rawDigest([]byte(command)), ReviewDigest: rawDigest([]byte("review")), SummaryBytes: []byte(`{"reviewed":true}`)}
	result, err := e.QueueReserve(context.Background(), request)
	if err != nil || result.Receipt.Rejection != nil {
		t.Fatalf("reserve: %+v %v", result, err)
	}
	var intent QueueLaunchIntent
	if err := json.Unmarshal(result.Receipt.Result, &intent); err != nil {
		t.Fatal(err)
	}
	return QueueTaskBinding{Selection: selection, IntentID: intent.ID}
}
func queueDispatchFixture(t *testing.T, e *Engine, binding QueueTaskBinding) {
	t.Helper()
	result, err := e.QueueDispatch(context.Background(), binding)
	if err != nil || result.Receipt.Rejection != nil || result.Duplicate {
		t.Fatalf("dispatch: %+v %v", result, err)
	}
}

func TestTaskQueueLostIndexStillNamesTheAdmittedRun(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e, legacy, claim := assistedWorkspaceFixture(t, "checkout")
	if _, err := e.Restrict(ctx, RestrictCommand{SchemaVersion: "1", CommandID: "command:settle-legacy", Scope: "run", ScopeID: legacy, Kind: "cancel", Reason: "isolate queue admission fixture"}); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(ctx, legacy); err != nil {
		t.Fatal(err)
	}
	selection := queueClaimFixture(t, e)
	binding := queueReserveFixture(t, e, selection, "command:queue-start")
	options := StartOptions{CommandID: "command:queue-start", WorkflowFile: "workflows/pilot.json", BriefFile: "brief.json", Inputs: map[string]string{}, WorkspaceMode: "checkout", QueueTask: &binding, WorkspaceClaim: &claim}
	before, _ := e.Store.ReadAuthority(ctx, TaskQueueKey)
	claimBefore, _ := e.Store.ReadAuthority(ctx, AuthorityClaimsKey)
	refused, err := e.Start(ctx, options)
	if refusalCode(err) != "queue_intent_conflict" || refused.Receipt.Rejection == nil || refused.Receipt.Rejection.Code != "queue_intent_conflict" {
		t.Fatalf("unmarked dispatch admitted: %+v %v", refused, err)
	}
	if _, err := e.Store.Read(ctx, e.IntendedRunID(options.CommandID), 0, 1); !errors.Is(err, local.ErrNotFound) {
		t.Fatalf("rejection left a Run: %v", err)
	}
	claimAfter, _ := e.Store.ReadAuthority(ctx, AuthorityClaimsKey)
	if claimAfter.Version != claimBefore.Version || !bytes.Equal(claimAfter.Data, claimBefore.Data) {
		t.Fatal("queue refusal spent workspace binding")
	}
	// A durable refusal uses a fresh caller command for a new admission attempt.
	binding = queueReserveFixtureAfterResolved(t, e, selection, binding)
	options.CommandID = "command:queue-start-admitted"
	options.QueueTask = &binding
	queueDispatchFixture(t, e, binding)
	other, err := e.QueueAdd(ctx, QueueAddRequest{CommandID: "command:unrelated-task", RequestBytes: queueTaskInput(t, "unrelated")})
	queueSummaryResult(t, other, err)
	prepared, _ := e.Store.ReadAuthority(ctx, TaskQueueKey)
	admitted, err := e.Start(ctx, options)
	if err != nil || admitted.Receipt.Rejection != nil {
		t.Fatalf("admission: %+v %v", admitted, err)
	}
	after, _ := e.Store.ReadAuthority(ctx, TaskQueueKey)
	if !bytes.Equal(prepared.Data, after.Data) || before.Version >= after.Version {
		t.Fatal("admission unexpectedly rewrote the queue index")
	}
	stored, err := e.queueSnapshot(after)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.Tasks[selection.TaskID].Associations) != 0 {
		t.Fatal("fixture did not exercise the lost-index crash window")
	}
	light, lightErr := e.Light(ctx, NewLightCache(), nil)
	if lightErr != nil {
		t.Fatal(lightErr)
	}
	found := false
	for _, card := range light.Cards {
		if card.TaskID != nil && *card.TaskID == selection.TaskID {
			found = true
			if card.HistoryCount != 1 || card.CurrentRuns[0].ID != admitted.Receipt.RunID {
				t.Fatalf("Light lost canonical admission: %+v", card)
			}
			for _, reason := range card.Reasons {
				if reason.Code == "launch_intent" {
					t.Fatal("admitted Run shown as unresolved dispatch")
				}
			}
		}
	}
	if !found {
		t.Fatal("Light hid task after index lag")
	}

	visible, err := e.QueueTask(ctx, selection.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if len(visible.Task.Associations) != 1 || visible.Task.Associations[0].RunID != admitted.Receipt.RunID {
		t.Fatalf("read hid admitted Run: %+v", visible)
	}
	unchanged, _ := e.Store.ReadAuthority(ctx, TaskQueueKey)
	if !bytes.Equal(unchanged.Data, after.Data) {
		t.Fatal("inspection wrote index")
	}
	run := driverRun(t, e, admitted.Receipt.RunID)
	view, err := e.View(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if run.SchemaVersion != CoreQueueStateVersion || view.SchemaVersion != CoreQueueReadVersion || run.QueueTask == nil || *run.QueueTask != binding {
		t.Fatalf("binding was not sealed: %+v", run.QueueTask)
	}
	for name, value := range map[string]any{"CoreRunStateV44": run, "CoreRunViewV44": view} {
		if err := validateInBundle(t, taskQueuePublicContracts, name, value); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if err := validateInBundle(t, liveOutputPublicContracts, "CoreRunStateV43", run); err == nil {
		t.Fatal("old closed bundle accepted queue binding")
	}
	final, err := e.QueueFinalize(ctx, binding, []byte(`{"schema_version":"fixture-result/1","run_id":"known"}`))
	if err != nil || final.Receipt.Rejection != nil {
		t.Fatalf("finalize: %+v %v", final, err)
	}
	if result, err := e.QueueFinalize(ctx, binding, nil); err != nil || result.Receipt.Rejection != nil {
		t.Fatalf("known admission reconcile: %+v %v", result, err)
	}
	// Project retries read the durable intent before reevaluating mutable
	// Workspace/control pins. The public CLI test proves exact result replay.
	known, err := e.QueueLaunchCommand(ctx, options.CommandID, rawDigest([]byte(options.CommandID)), rawDigest([]byte("review")))
	if err != nil || known == nil || known.RunID != run.ID {
		t.Fatalf("known admission lookup: %+v %v", known, err)
	}

	forked, err := e.Fork(ctx, ForkCommand{SchemaVersion: "1", CommandID: "command:raw-queue-fork", RunID: run.ID, ExpectedRunVersion: view.RunVersion, Payload: ForkPayload{WorkflowRef: run.WorkflowRef, BriefRef: run.Brief, Inputs: map[string]ArtifactRef{}, ReuseRefs: []ArtifactRef{}, Reason: "preserve task identity"}})
	if refusalCode(err) != "queue_fork_requires_project_launch" || forked.Receipt.RunID != "" {
		t.Fatalf("raw Fork discarded queue identity: %+v %v", forked, err)
	}
	finish := QueueFinishRequest{CommandID: "command:queue-release-live", Selection: selection, Reason: "owner decision"}
	result, err := e.QueueRelease(ctx, finish)
	if err != nil || result.Receipt.Rejection == nil || result.Receipt.Rejection.Code != "queue_task_busy" {
		t.Fatalf("live Run released: %+v %v", result, err)
	}
}

// This test adapter fixes only dispatcher evidence, never a production crash
// backdoor. It establishes the settled reserved intent before a fresh command.
func queueReserveFixtureAfterResolved(t *testing.T, e *Engine, selection QueueTaskSelection, previous QueueTaskBinding) QueueTaskBinding {
	t.Helper()
	result, err := e.queueApply(context.Background(), "command:fixture-resolve", "fixture.resolve", previous, func(queue *TaskQueue) (any, error) {
		task := queue.Tasks[selection.TaskID]
		intent := task.Intents[previous.IntentID]
		intent.State = "resolved"
		intent.Resolution = "not_started"
		intent.Reason = "test adapter establishes no effects"
		task.Intents[intent.ID] = intent
		queue.Tasks[task.ID] = task
		return intent, nil
	})
	if err != nil || result.Receipt.Rejection != nil {
		t.Fatalf("fixture resolve: %+v %v", result, err)
	}
	return queueReserveFixture(t, e, selection, "command:queue-start-admitted")
}

func TestTaskQueueLiveDispatcherCannotBeResolvedOrDispatchedTwice(t *testing.T) {
	t.Parallel()
	e, _ := emptyRuntime(t)
	ctx := context.Background()
	selection := queueClaimFixture(t, e)
	binding := queueReserveFixture(t, e, selection, "command:live-intent")
	result, err := e.QueueReconcile(ctx, QueueReconcileRequest{CommandID: "command:resolve-live", Selection: selection, IntentID: binding.IntentID, Resolution: "not_started", Reason: "cannot prove process gone"})
	if err != nil || result.Receipt.Rejection == nil || result.Receipt.Rejection.Code != "queue_dispatcher_active" {
		t.Fatalf("live dispatcher resolved: %+v %v", result, err)
	}
	queueDispatchFixture(t, e, binding)
	duplicate, err := e.QueueDispatch(ctx, binding)
	if err != nil || !duplicate.Duplicate {
		t.Fatalf("dispatch replay did not identify previous dispatch: %+v %v", duplicate, err)
	}
	result, err = e.QueueRelease(ctx, QueueFinishRequest{CommandID: "command:release-unknown", Selection: selection, Reason: "no timeout requeue"})
	if err != nil || result.Receipt.Rejection == nil {
		t.Fatalf("unknown dispatch released: %+v %v", result, err)
	}
}

func TestTaskQueueReleaseReplayAfterReclaimAndExplicitCompletion(t *testing.T) {
	t.Parallel()
	e, _ := emptyRuntime(t)
	ctx := context.Background()
	selection := queueClaimFixture(t, e)
	request := QueueFinishRequest{CommandID: "command:release-task", Selection: selection, Reason: "choose another host"}
	first, err := e.QueueRelease(ctx, request)
	queueSummaryResult(t, first, err)
	result, err := e.QueueClaimNext(ctx, QueueClaimRequest{CommandID: "command:reclaim-task", Owner: "host:second"})
	task := queueSummaryResult(t, result, err)
	if task.Claim.Generation != 2 {
		t.Fatalf("generation not advanced: %+v", task)
	}
	replay, err := e.QueueRelease(ctx, request)
	if err != nil || !replay.Duplicate || !bytes.Equal(replay.Receipt.Result, first.Receipt.Result) {
		t.Fatalf("late replay changed release: %+v %v", replay, err)
	}
	current, err := e.QueueTask(ctx, task.ID)
	if err != nil || current.Task.Claim.Owner != "host:second" {
		t.Fatalf("replay spent new owner: %+v %v", current, err)
	}
	request.CommandID = "command:late-owner"
	result, err = e.QueueRelease(ctx, request)
	if err != nil || result.Receipt.Rejection == nil {
		t.Fatalf("stale owner released: %+v %v", result, err)
	}
	next, _, err := e.QueueSelection(ctx, task.ID, "host:second", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	result, err = e.QueueComplete(ctx, QueueFinishRequest{CommandID: "command:complete-task", Selection: next, Reason: "local processing finished"})
	done := queueSummaryResult(t, result, err)
	if done.State != "done" || done.Claim != nil {
		t.Fatalf("completion: %+v", done)
	}
	closed, _ := e.QueueTask(ctx, task.ID)
	if closed.Task.Completion == nil || closed.Task.Completion.Owner != "host:second" {
		t.Fatal("missing explicit completion decision")
	}
}

func TestTaskQueueOldStateRejectsBindingEvenNull(t *testing.T) {
	t.Parallel()
	e, options := emptyRuntime(t)
	started, err := e.Start(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	run := driverRun(t, e, started.Receipt.RunID)
	raw, _ := json.Marshal(run)
	var fields map[string]json.RawMessage
	json.Unmarshal(raw, &fields)
	fields["queue_task"] = json.RawMessage("null")
	raw, _ = json.Marshal(fields)
	var decoded Run
	if err := decodeState(raw, &decoded); err == nil {
		t.Fatal("old state accepted explicit queue null")
	}
	run.SchemaVersion = CoreQueueStateVersion
	if queueBindingInvariant(run) == nil {
		t.Fatal("44 accepted a missing binding")
	}
	before := driverRun(t, e, started.Receipt.RunID)
	if !reflect.DeepEqual(run.QueueTask, before.QueueTask) {
		t.Fatal("inspection changed old Run")
	}
}

func TestQueueDispatcherIdentityHelper(t *testing.T) {
	t.Parallel()
	if os.Getenv("PRIFLY_QUEUE_DISPATCHER_HELPER") != "1" {
		return
	}
	identity, err := local.CurrentDispatcher()
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(identity); err != nil {
		t.Fatal(err)
	}
}

func TestTaskQueueExplicitReconcileFencesLateAdmission(t *testing.T) {
	t.Parallel()
	for _, dispatching := range []bool{false, true} {
		t.Run(map[bool]string{false: "reserved", true: "dispatching"}[dispatching], func(t *testing.T) {
			t.Parallel()
			e, _, _ := assistedWorkspaceFixture(t, "checkout")
			ctx := context.Background()
			selection := queueClaimFixture(t, e)
			binding := queueReserveFixture(t, e, selection, "command:late-admission")
			if dispatching {
				queueDispatchFixture(t, e, binding)
			}
			helper := exec.Command(os.Args[0], "-test.run=^TestQueueDispatcherIdentityHelper$")
			helper.Env = append(os.Environ(), "PRIFLY_QUEUE_DISPATCHER_HELPER=1")
			output, err := helper.Output()
			if err != nil {
				t.Fatal(err)
			}
			var identity local.DispatcherIdentity
			if err := json.NewDecoder(bytes.NewReader(output)).Decode(&identity); err != nil {
				t.Fatalf("helper identity: %s %v", output, err)
			}
			if !local.DispatcherGone(identity) {
				t.Fatal("exited helper's termination was not established")
			}
			// Test-only adapter records the actual exited dispatcher's native identity.
			result, err := e.queueApply(ctx, "command:fixture-dispatcher", "fixture.dispatcher", identity, func(queue *TaskQueue) (any, error) {
				task := queue.Tasks[selection.TaskID]
				intent := task.Intents[binding.IntentID]
				intent.Dispatcher = QueueDispatcherIdentity{PID: identity.PID, UID: identity.UID, Hostname: identity.Hostname, BootID: identity.BootID, StartID: identity.StartID}
				task.Intents[intent.ID] = intent
				queue.Tasks[task.ID] = task
				return intent, nil
			})
			if err != nil || result.Receipt.Rejection != nil {
				t.Fatalf("record helper identity: %+v %v", result, err)
			}
			resolution := "not_started"
			if dispatching {
				resolution = "effects_resolved"
				result, err = e.QueueReconcile(ctx, QueueReconcileRequest{CommandID: "command:bad-resolution", Selection: selection, IntentID: binding.IntentID, Resolution: "not_started", Reason: "PID death is not effect resolution"})
				if err != nil || result.Receipt.Rejection == nil || result.Receipt.Rejection.Code != "queue_resolution_invalid" {
					t.Fatalf("dispatch effects guessed: %+v %v", result, err)
				}
			}
			request := QueueReconcileRequest{CommandID: "command:resolve-terminated", Selection: selection, IntentID: binding.IntentID, Resolution: resolution, Reason: "owner established the effects for this fixture"}
			resolved, err := e.QueueReconcile(ctx, request)
			if err != nil || resolved.Receipt.Rejection != nil {
				t.Fatalf("explicit resolution: %+v %v", resolved, err)
			}
			late, err := e.Start(ctx, StartOptions{CommandID: "command:late-admission", WorkflowFile: "workflows/pilot.json", BriefFile: "brief.json", Inputs: map[string]string{}, WorkspaceMode: "checkout", QueueTask: &binding})
			if refusalCode(err) != "queue_intent_conflict" || late.Receipt.Rejection == nil {
				t.Fatalf("resolved intent admitted late: %+v %v", late, err)
			}
			if _, err := e.Store.Read(ctx, e.IntendedRunID("command:late-admission"), 0, 1); !errors.Is(err, local.ErrNotFound) {
				t.Fatal("late admission left a Run")
			}
			released, err := e.QueueRelease(ctx, QueueFinishRequest{CommandID: "command:release-resolved", Selection: selection, Reason: "safe explicit return"})
			queueSummaryResult(t, released, err)
			replay, err := e.QueueReconcile(ctx, request)
			if err != nil || !replay.Duplicate || !bytes.Equal(replay.Receipt.Result, resolved.Receipt.Result) {
				t.Fatalf("late resolution replay: %+v %v", replay, err)
			}
		})
	}
}

func TestTaskQueueReopenIsFencedAfterOwnerRelease(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	broken := environmentSourceFile(t, "env", "OTHER=ignored\n")
	e, _, _ := programAfterWriteFixture(t, "workspace-read", "none", map[string]EnvironmentSource{"DRIVER_TEST_SOURCED": {DotEnv: broken, Key: "PASSWORD"}})
	selection := queueClaimFixture(t, e)
	binding := queueReserveFixture(t, e, selection, "command:queue-broken-run")
	queueDispatchFixture(t, e, binding)
	started, err := e.Start(ctx, StartOptions{CommandID: "command:queue-broken-run", WorkflowFile: "workflows/pilot-program.json", BriefFile: "brief.json", Inputs: map[string]string{}, WorkspaceMode: "checkout", QueueTask: &binding})
	if err != nil {
		t.Fatal(err)
	}
	result, err := e.QueueFinalize(ctx, binding, nil)
	if err != nil || result.Receipt.Rejection != nil {
		t.Fatal(err)
	}
	task := handOver(t, e, started.Receipt.RunID)
	if _, err := e.SubmitSession(ctx, hostResult(t, e, task, "planned")); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(ctx, started.Receipt.RunID); err == nil {
		t.Fatal("missing environment did not produce a technical failure")
	}
	run, read, err := e.load(ctx, started.Receipt.RunID)
	if err != nil || !run.reopenable() {
		t.Fatalf("fixture is not reopenable: %s %v", run.Status, err)
	}
	// The current owner can reopen, and a subsequent technical failure settles.
	if _, err := e.Reopen(ctx, run.ID, "command:queue-reopen-current", "inspect repaired source", read.Snapshot.Version); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(ctx, run.ID); err == nil {
		t.Fatal("unrepaired environment unexpectedly worked")
	}
	before, read, err := e.load(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	released, err := e.QueueRelease(ctx, QueueFinishRequest{CommandID: "command:release-broken", Selection: selection, Reason: "return settled technical failure"})
	queueSummaryResult(t, released, err)
	// The new claim may launch a reviewed continuation, never revive the old one.
	reclaimed, err := e.QueueClaimNext(ctx, QueueClaimRequest{CommandID: "command:queue-reclaim-broken", Owner: "host:new-owner"})
	queueSummaryResult(t, reclaimed, err)
	if _, err := e.Reopen(ctx, run.ID, "command:queue-reopen-stale", "old owner tries again", read.Snapshot.Version); refusalCode(err) != "queue_task_claim_conflict" {
		t.Fatalf("old Run reopened after ownership ended: %v", err)
	}
	after, afterRead, err := e.load(ctx, run.ID)
	if err != nil || afterRead.Snapshot.Version != read.Snapshot.Version || !reflect.DeepEqual(before, after) {
		t.Fatal("refused reopen changed historical Run")
	}
}

func TestTaskQueueSourceRefsValidateBeforeMutationButNotReceiptReplay(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := artifactEngine(t)
	sourceTestFile(t, e, "source", []byte("original source"))
	source, snapshot := sourceTestImport(t, e, SourceImportOptions{Path: "source", Format: "blob"})
	var input TaskInput
	if err := json.Unmarshal(queueTaskInput(t, "with source"), &input); err != nil {
		t.Fatal(err)
	}
	input.SourceRefs = []ArtifactRef{source.Ref()}
	data, _ := json.Marshal(input)
	request := QueueAddRequest{CommandID: "command:queue-source", RequestBytes: data}
	first, err := e.QueueAdd(ctx, request)
	queueSummaryResult(t, first, err)
	blob := filepath.Join(e.Root, e.Config.Configuration.ArtifactRoot, strings.TrimPrefix(snapshot.ContentRef.Digest, "sha256:"))
	if err := os.Remove(blob); err != nil {
		t.Fatal(err)
	}
	replay, err := e.QueueAdd(ctx, request)
	if err != nil || !replay.Duplicate || !bytes.Equal(replay.Receipt.Result, first.Receipt.Result) {
		t.Fatalf("exact receipt depended on current source blob: %+v %v", replay, err)
	}
	request.CommandID = "command:new-source-request"
	before, _ := e.Queue(ctx)
	if _, err := e.QueueAdd(ctx, request); err == nil {
		t.Fatal("new task accepted missing source blob")
	}
	after, _ := e.Queue(ctx)
	if after.Version != before.Version || len(after.Tasks) != 1 {
		t.Fatal("invalid source mutated queue")
	}
}

func TestTaskQueueReleaseAndCompleteRefuseUncertaintyAndDecisions(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"uncertain", "decision"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			definition := DecisionDefinition{SchemaVersion: DecisionDefinitionVersion, ID: "continue", Title: "Continue", Phase: "runtime", Choices: []DecisionChoice{{ID: "yes", Title: "Yes", Value: json.RawMessage(`true`)}}, Sensitivity: "ordinary", Destination: DecisionDestination{Kind: "session_context", Name: "continue"}}
			catalog := DecisionCatalog{SchemaVersion: DecisionCatalogVersion, Decisions: []DecisionDefinition{definition}}
			digest, err := DecisionCatalogDigest(catalog)
			if err != nil {
				t.Fatal(err)
			}
			sheet := DecisionSheet{SchemaVersion: DecisionSheetVersion, CatalogDigest: digest, ProfileSource: "none", DecisionPolicy: "attended", Records: []DecisionRecord{}}
			e, _, _ := assistedWorkspaceFixtureWithDecisions(t, "checkout", &catalog, &sheet)
			selection := queueClaimFixture(t, e)
			binding := queueReserveFixture(t, e, selection, "command:queue-obligation")
			queueDispatchFixture(t, e, binding)
			created, err := e.Start(ctx, StartOptions{CommandID: "command:queue-obligation", WorkflowFile: "workflows/pilot.json", BriefFile: "brief.json", Inputs: map[string]string{}, WorkspaceMode: "checkout", QueueTask: &binding, DecisionCatalog: &catalog, DecisionSheet: &sheet})
			if err != nil {
				t.Fatal(err)
			}
			final, err := e.QueueFinalize(ctx, binding, nil)
			if err != nil || final.Receipt.Rejection != nil {
				t.Fatal(err)
			}
			task := handOver(t, e, created.Receipt.RunID)
			if kind == "decision" {
				definitionDigest, _ := DecisionDefinitionDigest(definition)
				request := DecisionRequest{SchemaVersion: DecisionRequestVersion, RunID: task.RunID, AttemptID: task.AttemptID, EnvelopeDigest: task.EnvelopeDigest, DecisionID: definition.ID, DefinitionDigest: definitionDigest, ExpectedRunVersion: task.RunVersion}
				if _, err := e.SubmitSession(ctx, SessionSubmission{SchemaVersion: task.SchemaVersion, RunID: task.RunID, AttemptID: task.AttemptID, EnvelopeDigest: task.EnvelopeDigest, DecisionRequest: &request}); err != nil {
					t.Fatal(err)
				}
				run := driverRun(t, e, task.RunID)
				if run.PendingDecision == nil {
					t.Fatal("fixture did not wait for an actual runtime decision")
				}
			} else {
				_, read, err := e.load(ctx, task.RunID)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := e.apply(ctx, e.owner, "command:fixture-deadline", task.RunID, "diagnostic.recorded", map[string]any{"expire": true}, &read.Snapshot.Version, local.CommandCAS, func(run *Run, _ local.Snapshot, _ Observation) (local.Change, error) {
					attempt := run.Attempts[task.AttemptID]
					attempt.Deadline = attempt.Admitted
					return local.Change{}, nil
				}); err != nil {
					t.Fatal(err)
				}
				if _, err := e.MarkSessionDisconnected(ctx, task.RunID, task.AttemptID); err != nil {
					t.Fatal(err)
				}
				run := driverRun(t, e, task.RunID)
				if run.Status != "uncertain" || !run.HasUnresolvedEffects {
					t.Fatal("fixture did not retain uncertain effects")
				}
			}
			before, _ := e.QueueTask(ctx, selection.TaskID)
			for _, complete := range []bool{false, true} {
				request := QueueFinishRequest{CommandID: map[bool]string{false: "command:refuse-release", true: "command:refuse-complete"}[complete], Selection: selection, Reason: "unsettled owner decision"}
				var result local.AuthorityApplyResult
				var err error
				if complete {
					result, err = e.QueueComplete(ctx, request)
				} else {
					result, err = e.QueueRelease(ctx, request)
				}
				if err != nil || result.Receipt.Rejection == nil || result.Receipt.Rejection.Code != "queue_task_busy" {
					t.Fatalf("%s obligation was cleared: %+v %v", kind, result, err)
				}
			}
			after, _ := e.QueueTask(ctx, selection.TaskID)
			if before.Version != after.Version || !reflect.DeepEqual(before.Task, after.Task) {
				t.Fatal("refusal changed ownership/history")
			}
		})
	}
}

func TestTaskQueueDoesNotTrustSemanticallyCorruptRunSnapshots(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e, _, _ := assistedWorkspaceFixture(t, "checkout")
	selection := queueClaimFixture(t, e)
	binding := queueReserveFixture(t, e, selection, "command:queue-corrupt-read")
	queueDispatchFixture(t, e, binding)
	created, err := e.Start(ctx, StartOptions{CommandID: "command:queue-corrupt-read", WorkflowFile: "workflows/pilot.json", BriefFile: "brief.json", Inputs: map[string]string{}, WorkspaceMode: "checkout", QueueTask: &binding})
	if err != nil {
		t.Fatal(err)
	}
	knownResult := []byte(`{"fixture":"known original result"}`)
	finalized, err := e.QueueFinalize(ctx, binding, knownResult)
	if err != nil || finalized.Receipt.Rejection != nil {
		t.Fatal(err)
	}
	run, read, err := e.load(ctx, created.Receipt.RunID)
	if err != nil {
		t.Fatal(err)
	}
	// A test-only storage adapter keeps byte integrity valid while contradicting
	// a semantic invariant. Queue readers must use the normal Run validator.
	run.Invocations[run.RootInvocationID].ControlTransitions++
	data, err := canonicalState(run)
	if err != nil {
		t.Fatal(err)
	}
	injected, err := e.Store.Apply(ctx, local.Command{ID: "command:fixture-semantic-corruption", Actor: e.owner, RunID: run.ID, Mode: local.CommandCAS, ExpectedVersion: &read.Snapshot.Version, Payload: json.RawMessage(`{"fixture":"inconsistent root budget"}`)}, func(local.Snapshot) (local.Change, error) {
		return local.Change{Data: data, Events: []local.EventInput{{Type: "diagnostic.recorded", Data: json.RawMessage(`{"fixture":true}`)}}}, nil
	})
	if err != nil || injected.Receipt.Rejection != nil {
		t.Fatalf("fixture corruption: %+v %v", injected, err)
	}
	if _, err := e.View(ctx, run.ID); err == nil {
		t.Fatal("fixture did not violate the normal Run reader")
	}
	known, err := e.QueueLaunchCommand(ctx, "command:queue-corrupt-read", rawDigest([]byte("command:queue-corrupt-read")), rawDigest([]byte("review")))
	if err != nil || known == nil || !bytes.Equal(known.ResultBytes, knownResult) {
		t.Fatalf("receipt lookup depended on the later corrupt Run: %+v %v", known, err)
	}
	before, err := e.Store.ReadAuthority(ctx, TaskQueueKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.QueueTask(ctx, selection.TaskID); err == nil {
		t.Fatal("queue inspection accepted a semantically corrupt associated Run")
	}
	if _, err := e.QueueRelease(ctx, QueueFinishRequest{CommandID: "command:release-corrupt", Selection: selection, Reason: "cannot establish corrupt Run settlement"}); err == nil {
		t.Fatal("queue release accepted an unvalidated Run")
	}
	after, err := e.Store.ReadAuthority(ctx, TaskQueueKey)
	if err != nil || before.Version != after.Version || !bytes.Equal(before.Data, after.Data) {
		t.Fatal("corrupt Run inspection changed ownership")
	}
}

func TestTaskQueueReservedIdentityRefusesRawStartWithoutConsumingIt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e, _, _ := assistedWorkspaceFixture(t, "checkout")
	selection := queueClaimFixture(t, e)
	commandID := "command:reserved-raw-start"
	binding := queueReserveFixture(t, e, selection, commandID)
	options := StartOptions{CommandID: commandID, WorkflowFile: "workflows/pilot.json", BriefFile: "brief.json", Inputs: map[string]string{}, WorkspaceMode: "checkout"}
	if result, err := e.Start(ctx, options); !errors.Is(err, local.ErrCommandConflict) {
		t.Fatalf("raw Start spent a queue-reserved identity: %+v %v", result, err)
	}
	if _, err := e.Store.LookupReceipt(ctx, e.owner, commandID); !errors.Is(err, local.ErrNotFound) {
		t.Fatalf("wrong request consumed the original command identity: %v", err)
	}
	queueDispatchFixture(t, e, binding)
	options.QueueTask = &binding
	admitted, err := e.Start(ctx, options)
	if err != nil || admitted.Receipt.Rejection != nil {
		t.Fatalf("reserved reviewed Start could not use its identity: %+v %v", admitted, err)
	}
}

func TestTaskQueueReserveCannotAliasAnExistingRunCommand(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e, _, _ := assistedWorkspaceFixture(t, "checkout")
	commandID := "command:existing-raw-start"
	created, err := e.Start(ctx, StartOptions{CommandID: commandID, WorkflowFile: "workflows/pilot.json", BriefFile: "brief.json", Inputs: map[string]string{}, WorkspaceMode: "checkout"})
	if err != nil {
		t.Fatal(err)
	}
	selection := queueClaimFixture(t, e)
	reserved, err := e.QueueReserve(ctx, QueueReserveRequest{CommandID: commandID, Selection: selection, CommandDigest: rawDigest([]byte(commandID)), ReviewDigest: rawDigest([]byte("review")), SummaryBytes: []byte(`{"reviewed":true}`)})
	if err == nil && reserved.Receipt.Rejection == nil {
		t.Fatalf("reserve aliased existing unbound Run %s: %+v", created.Receipt.RunID, reserved)
	}
	queue, _, err := e.queueLedger(ctx)
	if err != nil || len(queue.Tasks[selection.TaskID].Intents) != 0 {
		t.Fatalf("failed reserve left an orphan intent: %+v %v", queue, err)
	}
}

func TestTaskQueueReservedIdentityRefusesLegacyFork(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e, options := emptyRuntime(t)
	definitions, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	e.Config.Configuration.SemanticsProfile = flow.CoreProfile
	e.Config.Configuration.SchemaVersion = CoreConfigVersion
	e.Config.ConfigurationSchemaRef = builtinRef(definitions, "core:schema/core-configuration")
	workflow, err := flow.WorkflowJSONBytes(mustReadRuntime(t, filepath.Join(e.Root, options.WorkflowFile)), "json")
	if err != nil {
		t.Fatal(err)
	}
	digest, err := flow.Digest(workflow)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := e.localRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registry.Entries = append(registry.Entries, Definition{Ref: flow.Ref{ID: "test:workflow/empty", Version: "1.0.0", Digest: digest}, Kind: "workflow", Path: options.WorkflowFile})
	writeRuntimeJSON(t, filepath.Join(e.Root, e.Config.Configuration.RegistryFile), registry)
	writeRuntimeJSON(t, filepath.Join(e.Root, "prifly.json"), e.Config)
	created, err := e.Start(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(ctx, created.Receipt.RunID); err != nil {
		t.Fatal(err)
	}
	source, read, err := e.load(ctx, created.Receipt.RunID)
	if err != nil {
		t.Fatal(err)
	}
	selection := queueClaimFixture(t, e)
	commandID := "command:reserved-legacy-fork"
	queueReserveFixture(t, e, selection, commandID)
	command := ForkCommand{SchemaVersion: "1", CommandID: commandID, RunID: source.ID, ExpectedRunVersion: read.Snapshot.Version, Payload: ForkPayload{WorkflowRef: source.WorkflowRef, BriefRef: source.Brief, Inputs: map[string]ArtifactRef{}, ReuseRefs: []ArtifactRef{}, Reason: "different legacy request"}}
	if forked, err := e.Fork(ctx, command); !errors.Is(err, local.ErrCommandConflict) {
		t.Fatalf("legacy Fork spent a queue-reserved identity: %+v %v", forked, err)
	}
	if _, err := e.Store.Read(ctx, e.IntendedRunID(commandID), 0, 1); !errors.Is(err, local.ErrNotFound) {
		t.Fatalf("legacy Fork left an unbound Run: %v", err)
	}
}
