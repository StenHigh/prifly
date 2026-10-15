package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"unicode/utf8"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

func queueBindingInvariant(r Run) error {
	if r.QueueTask == nil {
		if len(r.QueueRequestBytes) > 0 {
			return local.ErrIntegrity
		}
		if atLeast(r.SchemaVersion, CoreQueueStateVersion) {
			return local.ErrIntegrity
		}
		return nil
	}
	if !atLeast(r.SchemaVersion, CoreQueueStateVersion) {
		return local.ErrIntegrity
	}
	if rawDigest(r.QueueRequestBytes) != r.QueueTask.Selection.RequestDigest {
		return local.ErrIntegrity
	}
	if _, err := queueRequest(r.QueueRequestBytes); err != nil {
		return local.ErrIntegrity
	}
	data, err := canonical(r.QueueTask)
	if err != nil {
		return err
	}
	if flow.ValidateProtocol("QueueTaskBinding", data) != nil {
		return local.ErrIntegrity
	}
	return nil
}

func queueSelectionOwned(task QueueTask, selection QueueTaskSelection) error {
	if task.ID != selection.TaskID {
		return local.Reject("queue_task_missing", "task is not recorded in this authority")
	}
	if err := queueTaskOwned(task, selection.TaskRevision, selection.Owner, selection.ClaimGeneration); err != nil {
		return err
	}
	if task.Revisions[len(task.Revisions)-1].RequestDigest != selection.RequestDigest {
		return local.Reject("queue_task_revision_conflict", "task request differs from its reviewed digest")
	}
	return nil
}

// QueueSelection verifies a complete selection without claiming, launching or writing.
func (e *Engine) QueueSelection(ctx context.Context, taskID, owner string, revision, generation int64) (QueueTaskSelection, []byte, error) {
	queue, _, err := e.readTaskQueue(ctx)
	if err != nil {
		return QueueTaskSelection{}, nil, err
	}
	task, ok := queue.Tasks[taskID]
	if !ok {
		return QueueTaskSelection{}, nil, fault("queue_task_missing", "task is not recorded in this authority")
	}
	if err := queueTaskOwned(task, revision, owner, generation); err != nil {
		return QueueTaskSelection{}, nil, err
	}
	rev := task.Revisions[len(task.Revisions)-1]
	input, err := queueRequest(rev.RequestBytes)
	if err != nil {
		return QueueTaskSelection{}, nil, err
	}
	for _, ref := range input.SourceRefs {
		if _, err := e.SourceSnapshot(ref); err != nil {
			return QueueTaskSelection{}, nil, err
		}
	}
	return QueueTaskSelection{TaskID: task.ID, TaskRevision: task.Revision, RequestDigest: rev.RequestDigest, Owner: owner, ClaimGeneration: generation}, append([]byte(nil), rev.RequestBytes...), nil
}

func (e *Engine) queueAdmissionGuard(binding QueueTaskBinding, runID string) local.AuthorityGuard {
	return local.AuthorityGuard{Key: TaskQueueKey, Check: func(snapshot local.AuthoritySnapshot) error {
		queue, err := e.queueSnapshot(snapshot)
		if err != nil {
			return err
		}
		task := queue.Tasks[binding.Selection.TaskID]
		if err := queueSelectionOwned(task, binding.Selection); err != nil {
			return err
		}
		intent, ok := task.Intents[binding.IntentID]
		if !ok || intent.State != "dispatching" || intent.RunID != runID || intent.Selection != binding.Selection {
			return local.Reject("queue_intent_conflict", "launch intent no longer authorizes this admission")
		}
		return nil
	}}
}

func queueRunIDs(queue TaskQueue) []string {
	ids := map[string]bool{}
	for _, task := range queue.Tasks {
		for _, intent := range task.Intents {
			ids[intent.RunID] = true
		}
		for _, association := range task.Associations {
			ids[association.RunID] = true
		}
	}
	out := make([]string, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func (e *Engine) queueFacts(ctx context.Context) (TaskQueue, local.AuthoritySnapshot, map[string]Run, []local.RunPin, error) {
	if _, err := e.readAccess(ctx); err != nil {
		return TaskQueue{}, local.AuthoritySnapshot{}, nil, nil, err
	}
	snapshot, snapshots, cut, err := e.Store.ReadAuthorityRuns(ctx, TaskQueueKey, func(snapshot local.AuthoritySnapshot) ([]string, error) {
		queue, err := e.queueSnapshot(snapshot)
		if err != nil {
			return nil, err
		}
		return queueRunIDs(queue), nil
	})
	if err != nil {
		return TaskQueue{}, snapshot, nil, nil, err
	}
	queue, err := e.queueSnapshot(snapshot)
	if err != nil {
		return queue, snapshot, nil, nil, err
	}
	snapshot.Cut = cut
	runs := map[string]Run{}
	pins := make([]local.RunPin, 0, len(snapshots))
	for id, s := range snapshots {
		pins = append(pins, local.RunPin{ID: id, Version: s.Version})
		if s.Version == 0 {
			continue
		}
		run, err := e.decodeSnapshot(s)
		if err != nil {
			return queue, snapshot, nil, nil, err
		}
		if !supportedRun(run) || run.ID != id || run.AuthorityID != e.Installation.ID || run.ProjectID != e.Config.ID || run.QueueTask == nil {
			return queue, snapshot, nil, nil, local.ErrIntegrity
		}
		runs[id] = run
	}
	for id, task := range queue.Tasks {
		seen := map[string]QueueTaskBinding{}
		for _, association := range task.Associations {
			run, ok := runs[association.RunID]
			if !ok || *run.QueueTask != association.Binding || association.Binding.Selection.TaskID != id {
				return queue, snapshot, nil, nil, local.ErrIntegrity
			}
			if _, dup := seen[association.RunID]; dup {
				return queue, snapshot, nil, nil, local.ErrIntegrity
			}
			seen[association.RunID] = association.Binding
		}
		intentIDs := make([]string, 0, len(task.Intents))
		for id := range task.Intents {
			intentIDs = append(intentIDs, id)
		}
		sort.Strings(intentIDs)
		for _, intentID := range intentIDs {
			intent := task.Intents[intentID]
			run, ok := runs[intent.RunID]
			if !ok {
				if intent.State == "admitted" {
					return queue, snapshot, nil, nil, local.ErrIntegrity
				}
				continue
			}
			binding := QueueTaskBinding{Selection: intent.Selection, IntentID: intent.ID}
			if *run.QueueTask != binding || intent.State == "reserved" || intent.State == "resolved" {
				return queue, snapshot, nil, nil, local.ErrIntegrity
			}
			if _, ok := seen[intent.RunID]; !ok {
				task.Associations = append(task.Associations, QueueRunAssociation{RunID: intent.RunID, Binding: binding})
				seen[intent.RunID] = binding
			}
		}
		queue.Tasks[id] = task
	}
	return queue, snapshot, runs, pins, nil
}

// IntendedRunID is the engine-owned command-to-Run mapping, used before dispatch.
func (e *Engine) IntendedRunID(commandID string) string { return startRunID(e.owner, commandID) }

type QueueReserveRequest struct {
	CommandID     string             `json:"command_id"`
	Selection     QueueTaskSelection `json:"selection"`
	CommandDigest string             `json:"command_digest"`
	ReviewDigest  string             `json:"review_digest"`
	SummaryBytes  []byte             `json:"summary_bytes"`
}

func queueIntentID(commandID string) string { return derivedID("queue-intent", commandID) }
func queueIntentCommand(commandID, phase string) string {
	return derivedID("queue-command", commandID, phase)
}

func (e *Engine) QueueReserve(ctx context.Context, request QueueReserveRequest) (local.AuthorityApplyResult, error) {
	if result, found, err := e.queueReplay(ctx, queueIntentCommand(request.CommandID, "reserve"), "queue.reserve", request); found || err != nil {
		return result, err
	}
	// Dispatcher evidence is a current observation, excluded from retry identity.
	dispatcher, err := local.CurrentDispatcher()
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	if request.CommandID == "" || len(request.SummaryBytes) == 0 || (!utf8.Valid(request.SummaryBytes) || !json.Valid(request.SummaryBytes)) {
		return local.AuthorityApplyResult{}, fault("queue_intent_invalid", "intent requires an exact reviewed summary and command identity")
	}
	id := queueIntentID(request.CommandID)
	intent := QueueLaunchIntent{ID: id, CommandID: request.CommandID, RunID: e.IntendedRunID(request.CommandID), Selection: request.Selection, CommandDigest: request.CommandDigest, ReviewDigest: request.ReviewDigest, SummaryBytes: append([]byte(nil), request.SummaryBytes...), State: "reserved", Dispatcher: QueueDispatcherIdentity{PID: dispatcher.PID, UID: dispatcher.UID, Hostname: dispatcher.Hostname, BootID: dispatcher.BootID, StartID: dispatcher.StartID}}
	return e.queueApply(ctx, queueIntentCommand(request.CommandID, "reserve"), "queue.reserve", request, func(queue *TaskQueue) (any, error) {
		task := queue.Tasks[request.Selection.TaskID]
		if err := queueSelectionOwned(task, request.Selection); err != nil {
			return nil, err
		}
		for _, previous := range queue.Tasks {
			for _, existing := range previous.Intents {
				if existing.CommandID == request.CommandID {
					return nil, local.Reject("queue_intent_conflict", "launch command already has an intent")
				}
			}
		}
		for _, existing := range task.Intents {
			if existing.State == "reserved" || existing.State == "dispatching" {
				return nil, local.Reject("queue_task_busy", "another launch intent requires inspection or reconcile")
			}
		}
		task.Intents[id] = intent
		queue.Tasks[task.ID] = task
		return intent, nil
	}, local.RunPin{ID: intent.RunID, Version: 0})
}

func (e *Engine) QueueDispatch(ctx context.Context, binding QueueTaskBinding) (local.AuthorityApplyResult, error) {
	return e.queueApply(ctx, derivedID("queue-command", binding.IntentID, "dispatch"), "queue.dispatch", binding, func(queue *TaskQueue) (any, error) {
		task := queue.Tasks[binding.Selection.TaskID]
		if err := queueSelectionOwned(task, binding.Selection); err != nil {
			return nil, err
		}
		intent, ok := task.Intents[binding.IntentID]
		if !ok || intent.Selection != binding.Selection || intent.State != "reserved" {
			return nil, local.Reject("queue_intent_conflict", "launch intent cannot dispatch")
		}
		intent.State = "dispatching"
		task.Intents[intent.ID] = intent
		queue.Tasks[task.ID] = task
		return intent, nil
	})
}

func (e *Engine) QueueFinalize(ctx context.Context, binding QueueTaskBinding, result []byte) (local.AuthorityApplyResult, error) {
	queue, _, runs, _, err := e.queueFacts(ctx)
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	task := queue.Tasks[binding.Selection.TaskID]
	intent, ok := task.Intents[binding.IntentID]
	run, admitted := runs[intent.RunID]
	if !ok || !admitted || run.QueueTask == nil || *run.QueueTask != binding {
		return local.AuthorityApplyResult{}, fault("queue_admission_missing", "no matching admitted Run is known")
	}
	if len(result) > 0 && (!utf8.Valid(result) || !json.Valid(result)) {
		return local.AuthorityApplyResult{}, fault("queue_result_invalid", "start result must be valid JSON")
	}
	// Mutable Run versions do not govern finalization: binding is immutable.
	return e.queueApply(ctx, derivedID("queue-command", binding.IntentID, "finalize", rawDigest(result)), "queue.finalize", struct {
		Binding QueueTaskBinding `json:"binding"`
		Result  []byte           `json:"result"`
	}{binding, result}, func(queue *TaskQueue) (any, error) {
		return recordQueueAdmission(queue, binding, result)
	})
}

func recordQueueAdmission(queue *TaskQueue, binding QueueTaskBinding, result []byte) (any, error) {
	task := queue.Tasks[binding.Selection.TaskID]
	intent, ok := task.Intents[binding.IntentID]
	if !ok || intent.Selection != binding.Selection || intent.State != "dispatching" && intent.State != "admitted" {
		return nil, local.Reject("queue_intent_conflict", "intent was resolved or changed before finalization")
	}
	if len(result) > 0 && len(intent.ResultBytes) > 0 && !bytes.Equal(intent.ResultBytes, result) {
		return nil, local.Reject("queue_result_conflict", "an exact start result is already recorded")
	}
	intent.State = "admitted"
	if len(result) > 0 {
		intent.ResultBytes = append([]byte(nil), result...)
	}
	task.Intents[intent.ID] = intent
	exists := false
	for _, a := range task.Associations {
		if a.RunID == intent.RunID {
			if a.Binding != binding {
				return nil, local.ErrIntegrity
			}
			exists = true
		}
	}
	if !exists {
		task.Associations = append(task.Associations, QueueRunAssociation{RunID: intent.RunID, Binding: binding})
	}
	queue.Tasks[task.ID] = task
	return intent, nil
}

type QueueReconcileRequest struct {
	CommandID  string             `json:"command_id"`
	Selection  QueueTaskSelection `json:"selection"`
	IntentID   string             `json:"intent_id"`
	Resolution string             `json:"resolution"`
	Reason     string             `json:"reason"`
}

func (e *Engine) QueueReconcile(ctx context.Context, request QueueReconcileRequest) (local.AuthorityApplyResult, error) {
	if result, found, err := e.queueReplay(ctx, request.CommandID, "queue.reconcile", request); found || err != nil {
		return result, err
	}
	queue, _, runs, pins, err := e.queueFacts(ctx)
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	task := queue.Tasks[request.Selection.TaskID]
	intent, ok := task.Intents[request.IntentID]
	if !ok {
		return local.AuthorityApplyResult{}, fault("queue_intent_missing", "launch intent is not recorded")
	}
	binding := QueueTaskBinding{Selection: intent.Selection, IntentID: intent.ID}
	_, admitted := runs[intent.RunID]
	if admitted && request.Resolution != "" {
		return local.AuthorityApplyResult{}, fault("queue_admission_known", "the intended Run was admitted; inspect that Run")
	}
	if request.Resolution == "" {
		return e.queueApply(ctx, request.CommandID, "queue.reconcile", request, func(queue *TaskQueue) (any, error) {
			task := queue.Tasks[request.Selection.TaskID]
			if err := queueSelectionOwned(task, request.Selection); err != nil {
				return nil, err
			}
			current, ok := task.Intents[request.IntentID]
			if !ok || !reflect.DeepEqual(current, intent) {
				return nil, local.Reject("queue_intent_conflict", "intent changed after inspection")
			}
			if admitted {
				return recordQueueAdmission(queue, binding, nil)
			}
			return current, nil
		})
	}

	gone := local.DispatcherGone(local.DispatcherIdentity{PID: intent.Dispatcher.PID, UID: intent.Dispatcher.UID, Hostname: intent.Dispatcher.Hostname, BootID: intent.Dispatcher.BootID, StartID: intent.Dispatcher.StartID})
	return e.queueApply(ctx, request.CommandID, "queue.reconcile", request, func(queue *TaskQueue) (any, error) {
		task := queue.Tasks[request.Selection.TaskID]
		if err := queueSelectionOwned(task, request.Selection); err != nil {
			return nil, err
		}
		current, ok := task.Intents[request.IntentID]
		if !ok || !reflect.DeepEqual(current, intent) {
			return nil, local.Reject("queue_intent_conflict", "intent changed after inspection")
		}
		if !gone {
			return nil, local.Reject("queue_dispatcher_active", "dispatcher is live or its termination is not established")
		}
		if request.Reason == "" || intent.State == "reserved" && request.Resolution != "not_started" || intent.State == "dispatching" && request.Resolution != "effects_resolved" || intent.State != "reserved" && intent.State != "dispatching" {
			return nil, local.Reject("queue_resolution_invalid", "resolution must establish the possible effects for the recorded dispatch phase")
		}
		current.State = "resolved"
		current.Resolution = request.Resolution
		current.Reason = request.Reason
		task.Intents[current.ID] = current
		queue.Tasks[task.ID] = task
		return current, nil
	}, pins...)
}

type QueueFinishRequest struct {
	CommandID string             `json:"command_id"`
	Selection QueueTaskSelection `json:"selection"`
	Reason    string             `json:"reason"`
}

func queueTaskHistoryFence(task QueueTask) string {
	ids := map[string]bool{}
	for _, intent := range task.Intents {
		ids[intent.RunID] = true
	}
	for _, a := range task.Associations {
		ids[a.RunID] = true
	}
	data, _ := canonical(struct {
		Intents map[string]QueueLaunchIntent
		Runs    map[string]bool
	}{task.Intents, ids})
	return rawDigest(data)
}

func (e *Engine) queueFinish(ctx context.Context, request QueueFinishRequest, complete bool) (local.AuthorityApplyResult, error) {
	operation := "queue.release"
	if complete {
		operation = "queue.complete"
	}
	if result, found, err := e.queueReplay(ctx, request.CommandID, operation, request); found || err != nil {
		return result, err
	}
	queue, _, runs, pins, err := e.queueFacts(ctx)
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	task := queue.Tasks[request.Selection.TaskID]
	fence := queueTaskHistoryFence(task)
	busy := ""
	selectedIDs := map[string]bool{}
	for _, intent := range task.Intents {
		selectedIDs[intent.RunID] = true
		if intent.State == "reserved" || intent.State == "dispatching" {
			busy = "unfinished launch intent requires reconciliation"
		}
	}
	for _, association := range task.Associations {
		selectedIDs[association.RunID] = true
	}
	for id := range selectedIDs {
		if run, ok := runs[id]; ok {
			if reason := cleanupRefusal(run); reason != "" {
				busy = "associated Run still holds an active, uncertain or decision obligation"
			}
		} else {
			// Only explicitly resolved, unadmitted intents may have no Run.
			resolved := false
			for _, intent := range task.Intents {
				if intent.RunID == id && intent.State == "resolved" {
					resolved = true
				}
			}
			if !resolved {
				busy = "an associated Run is missing; its settlement is not established"
			}
		}
	}
	selectedPins := []local.RunPin{}
	for _, pin := range pins {
		if selectedIDs[pin.ID] {
			selectedPins = append(selectedPins, pin)
		}
	}
	return e.queueApply(ctx, request.CommandID, operation, request, func(queue *TaskQueue) (any, error) {
		task := queue.Tasks[request.Selection.TaskID]
		if err := queueSelectionOwned(task, request.Selection); err != nil {
			return nil, err
		}
		if queueTaskHistoryFence(task) != fence {
			return nil, local.Reject("queue_history_changed", "launch history changed after settlement inspection")
		}
		if busy != "" {
			return nil, local.Reject("queue_task_busy", busy)
		}
		if !queueOwnerValid(request.Reason) {
			return nil, local.Reject("queue_reason_required", "record the owner's bounded reason for this local decision")
		}
		task.Claim = nil
		task.State = "ready"
		if complete {
			task.State = "done"
			task.Completion = &QueueTaskCompletion{CommandID: request.CommandID, Owner: request.Selection.Owner, Reason: request.Reason}
		}
		queue.Tasks[task.ID] = task
		return queueTaskSummary(task), nil
	}, selectedPins...)
}
func (e *Engine) QueueRelease(ctx context.Context, request QueueFinishRequest) (local.AuthorityApplyResult, error) {
	return e.queueFinish(ctx, request, false)
}
func (e *Engine) QueueComplete(ctx context.Context, request QueueFinishRequest) (local.AuthorityApplyResult, error) {
	return e.queueFinish(ctx, request, true)
}

func (e *Engine) queueReopenGuard(binding QueueTaskBinding) local.AuthorityGuard {
	return local.AuthorityGuard{Key: TaskQueueKey, Check: func(snapshot local.AuthoritySnapshot) error {
		queue, err := e.queueSnapshot(snapshot)
		if err != nil {
			return err
		}
		return queueSelectionOwned(queue.Tasks[binding.Selection.TaskID], binding.Selection)
	}}
}

// QueueLaunchCommand reads a previously reserved call before CLI source reads,
// process checks or preflight. The request digest covers every launch argument.
func (e *Engine) QueueLaunchCommand(ctx context.Context, commandID, commandDigest, reviewDigest string) (*QueueLaunchIntent, error) {
	queue, _, err := e.queueLedger(ctx)
	if err != nil {
		return nil, err
	}
	for _, task := range queue.Tasks {
		for _, intent := range task.Intents {
			if intent.CommandID != commandID {
				continue
			}
			if intent.CommandDigest != commandDigest || intent.ReviewDigest != reviewDigest {
				return nil, local.ErrCommandConflict
			}
			return &intent, nil
		}
	}
	return nil, nil
}

func (e *Engine) queueAccessGuard(operation string) local.AuthorityGuard {
	return local.AuthorityGuard{Key: AuthorityControlKey, Check: func(snapshot local.AuthoritySnapshot) error {
		if snapshot.Version == 0 {
			return nil
		} // unchanged local-owner bootstrap boundary
		control, err := decodeControl(snapshot.Data)
		if err != nil {
			return err
		}
		if err := e.controlCompatible(control); err != nil {
			return err
		}
		required := ControlOperationResolve
		admitting := operation == "queue.claim-next" || operation == "queue.reserve" || operation == "queue.dispatch"
		if admitting {
			required = ControlOperationAdmit
		}
		if !control.allows(e.owner, "project", e.Config.ID, required) {
			return local.Reject("object_access_denied", "the session principal cannot mutate this project's queue")
		}
		if admitting && control.blockingStop(e.Installation.ID, e.Config.ID) != nil {
			return local.Reject("control_stop_active", "an authority stop forbids new queue ownership or dispatch")
		}
		return nil
	}}
}

// A reserved queue command identity cannot be repurposed through a legacy
// entry point. ErrCommandConflict rolls back without consuming that identity.
func queueUnboundRunAllowed(queue TaskQueue, runID string) error {
	for _, task := range queue.Tasks {
		for _, intent := range task.Intents {
			if intent.RunID == runID {
				return local.ErrCommandConflict
			}
		}
	}
	return nil
}
func (e *Engine) checkUnboundQueueRun(ctx context.Context, runID string) error {
	queue, _, err := e.queueLedger(ctx)
	if err != nil {
		return err
	}
	return queueUnboundRunAllowed(queue, runID)
}
func (e *Engine) queueUnboundRunGuard(runID string) local.AuthorityGuard {
	return local.AuthorityGuard{Key: TaskQueueKey, Check: func(snapshot local.AuthoritySnapshot) error {
		queue, err := e.queueSnapshot(snapshot)
		if err != nil {
			return err
		}
		return queueUnboundRunAllowed(queue, runID)
	}}
}
