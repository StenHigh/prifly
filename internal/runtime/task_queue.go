package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

const (
	TaskQueueKey               = "task_queue"
	TaskQueueVersion           = "project-task-queue/1"
	TaskQueueReadVersion       = "project-task-queue-read/1"
	QueueTaskReadVersion       = "project-queue-task/1"
	queueMaxInteger      int64 = 1<<53 - 1
)

// TaskQueue belongs to one local authority. Reading an absent queue never
// enrolls it; the first explicit add records it alongside its command receipt.
type TaskQueue struct {
	SchemaVersion string               `json:"schema_version"`
	AuthorityID   string               `json:"authority_id"`
	ProjectID     string               `json:"project_id"`
	NextOrdinal   int64                `json:"next_ordinal"`
	Tasks         map[string]QueueTask `json:"tasks"`
}

type QueueTask struct {
	ID              string                       `json:"id"`
	Revision        int64                        `json:"revision"`
	Title           string                       `json:"title"`
	Priority        int64                        `json:"priority"`
	Ordinal         int64                        `json:"ordinal"`
	State           string                       `json:"state"`
	Revisions       []QueueTaskRevision          `json:"revisions"`
	ClaimGeneration int64                        `json:"claim_generation"`
	Claim           *QueueTaskClaim              `json:"claim,omitempty"`
	Intents         map[string]QueueLaunchIntent `json:"intents"`
	Associations    []QueueRunAssociation        `json:"associations"`
	Completion      *QueueTaskCompletion         `json:"completion,omitempty"`
}

// RequestBytes is intentionally byte-encoded, not RawMessage: re-encoding an
// embedded JSON object would change the owner's exact TaskInput evidence.
type QueueTaskRevision struct {
	Revision      int64  `json:"revision"`
	RequestBytes  []byte `json:"request_bytes"`
	RequestDigest string `json:"request_digest"`
	Priority      int64  `json:"priority"`
}

type QueueTaskClaim struct {
	ID         string `json:"id"`
	Owner      string `json:"owner"`
	Generation int64  `json:"generation"`
}

// Selection is the task part of a review; an intent is attached only after
// that review is accepted. Content revision and ownership generation differ.
type QueueTaskSelection struct {
	TaskID          string `json:"task_id"`
	TaskRevision    int64  `json:"task_revision"`
	RequestDigest   string `json:"request_digest"`
	Owner           string `json:"owner"`
	ClaimGeneration int64  `json:"claim_generation"`
}

type QueueTaskBinding struct {
	Selection QueueTaskSelection `json:"selection"`
	IntentID  string             `json:"intent_id"`
}

type QueueDispatcherIdentity struct {
	PID      int    `json:"pid"`
	UID      int    `json:"uid"`
	Hostname string `json:"hostname"`
	BootID   string `json:"boot_id"`
	StartID  string `json:"start_id"`
}

type QueueLaunchIntent struct {
	ID            string                  `json:"id"`
	CommandID     string                  `json:"command_id"`
	RunID         string                  `json:"run_id"`
	Selection     QueueTaskSelection      `json:"selection"`
	CommandDigest string                  `json:"command_digest"`
	ReviewDigest  string                  `json:"review_digest"`
	SummaryBytes  []byte                  `json:"summary_bytes"`
	State         string                  `json:"state"`
	Dispatcher    QueueDispatcherIdentity `json:"dispatcher"`
	ResultBytes   []byte                  `json:"result_bytes,omitempty"`
	Resolution    string                  `json:"resolution,omitempty"`
	Reason        string                  `json:"reason,omitempty"`
}

type QueueRunAssociation struct {
	RunID   string           `json:"run_id"`
	Binding QueueTaskBinding `json:"binding"`
}

type QueueTaskCompletion struct {
	CommandID string `json:"command_id"`
	Owner     string `json:"owner"`
	Reason    string `json:"reason"`
}

type QueueTaskSummary struct {
	ID              string          `json:"id"`
	Revision        int64           `json:"revision"`
	Title           string          `json:"title"`
	Priority        int64           `json:"priority"`
	Ordinal         int64           `json:"ordinal"`
	State           string          `json:"state"`
	ClaimGeneration int64           `json:"claim_generation"`
	Claim           *QueueTaskClaim `json:"claim,omitempty"`
	RunCount        int             `json:"run_count"`
}

type TaskQueueView struct {
	SchemaVersion string             `json:"schema_version"`
	AuthorityID   string             `json:"authority_id"`
	ProjectID     string             `json:"project_id"`
	Version       int64              `json:"version"`
	Cut           int64              `json:"cut"`
	Initialized   bool               `json:"initialized"`
	Tasks         []QueueTaskSummary `json:"tasks"`
}

type QueueTaskView struct {
	SchemaVersion string    `json:"schema_version"`
	AuthorityID   string    `json:"authority_id"`
	ProjectID     string    `json:"project_id"`
	Version       int64     `json:"version"`
	Cut           int64     `json:"cut"`
	Task          QueueTask `json:"task"`
}

type QueueAddRequest struct {
	CommandID    string `json:"command_id"`
	RequestBytes []byte `json:"request_bytes"`
	Priority     int64  `json:"priority"`
}

type QueueUpdateRequest struct {
	CommandID        string `json:"command_id"`
	TaskID           string `json:"task_id"`
	ExpectedRevision int64  `json:"expected_revision"`
	Owner            string `json:"owner"`
	ClaimGeneration  int64  `json:"claim_generation"`
	RequestBytes     []byte `json:"request_bytes"`
	Priority         int64  `json:"priority"`
}

type QueueClaimRequest struct {
	CommandID string `json:"command_id"`
	Owner     string `json:"owner"`
}

func queuePriorityValid(priority int64) bool {
	return priority >= -queueMaxInteger && priority <= queueMaxInteger
}
func queueOwnerValid(owner string) bool {
	return utf8.ValidString(owner) && strings.TrimSpace(owner) != "" && len(owner) <= 1024
}

func queueRequest(data []byte) (TaskInput, error) {
	if !utf8.Valid(data) || len(data) > local.MaxCommandBytes {
		return TaskInput{}, fault("queue_task_invalid_request", "TaskInput must be bounded UTF-8 JSON")
	}
	return ParseTaskInput(data)
}

func (e *Engine) queueSnapshot(snapshot local.AuthoritySnapshot) (TaskQueue, error) {
	queue := TaskQueue{SchemaVersion: TaskQueueVersion, AuthorityID: e.Installation.ID, ProjectID: e.Config.ID, NextOrdinal: 1, Tasks: map[string]QueueTask{}}
	if snapshot.Version == 0 {
		return queue, nil
	}
	if err := flow.ValidateProtocol("ProjectTaskQueue", snapshot.Data); err != nil {
		return TaskQueue{}, fault("queue_incompatible", "stored queue does not match its closed contract")
	}
	if err := decode(snapshot.Data, &queue); err != nil {
		return TaskQueue{}, err
	}
	if queue.SchemaVersion != TaskQueueVersion || queue.AuthorityID != e.Installation.ID || queue.ProjectID != e.Config.ID {
		return TaskQueue{}, fault("queue_incompatible", "queue belongs to another authority/project or contract version")
	}
	if queue.Tasks == nil || queue.NextOrdinal < 1 || queue.NextOrdinal > queueMaxInteger {
		return TaskQueue{}, local.ErrIntegrity
	}
	ordinals := map[int64]bool{}
	commands := map[string]bool{}
	runIDs := map[string]bool{}
	for id, task := range queue.Tasks {
		if task.ID != id || task.Revision < 1 || task.Revision != int64(len(task.Revisions)) || task.Ordinal < 1 || task.Ordinal >= queue.NextOrdinal || task.ClaimGeneration < 0 || task.ClaimGeneration > queueMaxInteger || task.Intents == nil || task.Associations == nil {
			return TaskQueue{}, local.ErrIntegrity
		}
		if ordinals[task.Ordinal] {
			return TaskQueue{}, local.ErrIntegrity
		}
		ordinals[task.Ordinal] = true
		if (task.State == "done") != (task.Completion != nil) {
			return TaskQueue{}, local.ErrIntegrity
		}
		if task.State != "ready" && task.State != "claimed" && task.State != "done" {
			return TaskQueue{}, local.ErrIntegrity
		}
		if (task.State == "claimed") != (task.Claim != nil) {
			return TaskQueue{}, local.ErrIntegrity
		}
		if task.Claim != nil && (task.Claim.Generation != task.ClaimGeneration || !queueOwnerValid(task.Claim.Owner) || task.Claim.ID != derivedID("task-claim", id, fmt.Sprint(task.ClaimGeneration))) {
			return TaskQueue{}, local.ErrIntegrity
		}
		for index, rev := range task.Revisions {
			if rev.Revision != int64(index+1) || !queuePriorityValid(rev.Priority) || rev.RequestDigest != rawDigest(rev.RequestBytes) {
				return TaskQueue{}, local.ErrIntegrity
			}
			input, err := queueRequest(rev.RequestBytes)
			if err != nil {
				return TaskQueue{}, local.ErrIntegrity
			}
			if index == len(task.Revisions)-1 && (input.Title != task.Title || rev.Priority != task.Priority) {
				return TaskQueue{}, local.ErrIntegrity
			}
		}
		for intentID, intent := range task.Intents {
			selection := intent.Selection
			if intent.ID != intentID || intent.ID != queueIntentID(intent.CommandID) || commands[intent.CommandID] || runIDs[intent.RunID] || selection.TaskID != id || selection.TaskRevision < 1 || selection.TaskRevision > task.Revision || selection.ClaimGeneration < 1 || selection.ClaimGeneration > task.ClaimGeneration || !queueOwnerValid(selection.Owner) || selection.RequestDigest != task.Revisions[selection.TaskRevision-1].RequestDigest || (!utf8.Valid(intent.SummaryBytes) || !json.Valid(intent.SummaryBytes)) {
				return TaskQueue{}, local.ErrIntegrity
			}
			if len(intent.ResultBytes) > 0 && (intent.State != "admitted" || (!utf8.Valid(intent.ResultBytes) || !json.Valid(intent.ResultBytes))) {
				return TaskQueue{}, local.ErrIntegrity
			}
			if (intent.State == "reserved" || intent.State == "dispatching") && queueSelectionOwned(task, selection) != nil {
				return TaskQueue{}, local.ErrIntegrity
			}
			commands[intent.CommandID] = true
			runIDs[intent.RunID] = true
		}
		associated := map[string]bool{}
		for _, association := range task.Associations {
			intent, ok := task.Intents[association.Binding.IntentID]
			if !ok || associated[association.RunID] || intent.State != "admitted" || intent.RunID != association.RunID || intent.Selection != association.Binding.Selection {
				return TaskQueue{}, local.ErrIntegrity
			}
			associated[association.RunID] = true
		}

	}
	return queue, nil
}

// queueLedger reads only immutable command/task evidence. Receipt lookup must
// not depend on the later mutable state of any associated Run.
func (e *Engine) queueLedger(ctx context.Context) (TaskQueue, local.AuthoritySnapshot, error) {
	if _, err := e.readAccess(ctx); err != nil {
		return TaskQueue{}, local.AuthoritySnapshot{}, err
	}
	snapshot, err := e.Store.ReadAuthority(ctx, TaskQueueKey)
	if errors.Is(err, local.ErrNotFound) {
		snapshot = local.AuthoritySnapshot{Key: TaskQueueKey}
		err = nil
	}
	if err != nil {
		return TaskQueue{}, snapshot, err
	}
	queue, err := e.queueSnapshot(snapshot)
	return queue, snapshot, err
}

// QueueRecordedSelection reconstructs a command envelope from a retained task
// revision. It proves no current ownership; mutations spend their own CAS only
// after exact receipt lookup.
func (e *Engine) QueueRecordedSelection(ctx context.Context, id, owner string, revision, generation int64) (QueueTaskSelection, error) {
	queue, _, err := e.queueLedger(ctx)
	if err != nil {
		return QueueTaskSelection{}, err
	}
	task, ok := queue.Tasks[id]
	if !ok {
		return QueueTaskSelection{}, fault("queue_task_missing", "task is not recorded in this authority")
	}
	if revision < 1 || revision > int64(len(task.Revisions)) {
		return QueueTaskSelection{}, fault("queue_task_revision_conflict", "task revision is not recorded")
	}
	if !queueOwnerValid(owner) || generation < 1 || generation > queueMaxInteger {
		return QueueTaskSelection{}, fault("queue_task_claim_conflict", "a recorded owner and claim generation are required")
	}
	return QueueTaskSelection{TaskID: id, TaskRevision: revision, RequestDigest: task.Revisions[revision-1].RequestDigest, Owner: owner, ClaimGeneration: generation}, nil
}

func (e *Engine) readTaskQueue(ctx context.Context) (TaskQueue, local.AuthoritySnapshot, error) {
	queue, snapshot, _, _, err := e.queueFacts(ctx)
	return queue, snapshot, err
}

func queueTaskSummary(task QueueTask) QueueTaskSummary {
	return QueueTaskSummary{ID: task.ID, Revision: task.Revision, Title: task.Title, Priority: task.Priority, Ordinal: task.Ordinal, State: task.State, ClaimGeneration: task.ClaimGeneration, Claim: task.Claim, RunCount: len(task.Associations)}
}
func queueTaskEarlier(a, b QueueTaskSummary) bool {
	if a.Priority != b.Priority {
		return a.Priority > b.Priority
	}
	if a.Ordinal != b.Ordinal {
		return a.Ordinal < b.Ordinal
	}
	return a.ID < b.ID
}

func (e *Engine) Queue(ctx context.Context) (TaskQueueView, error) {
	queue, snapshot, err := e.readTaskQueue(ctx)
	if err != nil {
		return TaskQueueView{}, err
	}
	view := TaskQueueView{SchemaVersion: TaskQueueReadVersion, AuthorityID: queue.AuthorityID, ProjectID: queue.ProjectID, Version: snapshot.Version, Cut: snapshot.Cut, Initialized: snapshot.Version != 0, Tasks: []QueueTaskSummary{}}
	for _, task := range queue.Tasks {
		view.Tasks = append(view.Tasks, queueTaskSummary(task))
	}
	sort.Slice(view.Tasks, func(i, j int) bool { return queueTaskEarlier(view.Tasks[i], view.Tasks[j]) })
	return view, nil
}

func (e *Engine) QueueTask(ctx context.Context, id string) (QueueTaskView, error) {
	queue, snapshot, err := e.readTaskQueue(ctx)
	if err != nil {
		return QueueTaskView{}, err
	}
	task, exists := queue.Tasks[id]
	if !exists {
		return QueueTaskView{}, fault("queue_task_missing", "task is not recorded in this project queue")
	}
	return QueueTaskView{SchemaVersion: QueueTaskReadVersion, AuthorityID: queue.AuthorityID, ProjectID: queue.ProjectID, Version: snapshot.Version, Cut: snapshot.Cut, Task: task}, nil
}

// Validate text and exact integer fields before JSON encoding can replace
// malformed UTF-8 or canonicalize an out-of-range integer into another request.
func queueEnvelopeValid(request any) error {
	var owner, reason string
	var selection *QueueTaskSelection
	switch r := request.(type) {
	case QueueClaimRequest:
		if !queueOwnerValid(r.Owner) {
			return fault("queue_owner_invalid", "claim owner must be bounded nonempty UTF-8")
		}
		owner = r.Owner
	case QueueUpdateRequest:
		owner = r.Owner
		if r.ExpectedRevision < 1 || r.ExpectedRevision > queueMaxInteger || r.ClaimGeneration < 0 || r.ClaimGeneration > queueMaxInteger {
			return fault("queue_selection_invalid", "revision and generation must be exact bounded integers")
		}
	case QueueReserveRequest:
		encodedID, _ := json.Marshal(r.CommandID)
		if flow.ValidateProtocol("Identifier", encodedID) != nil {
			return fault("queue_command_invalid", "original Run command identity is outside the supported shape")
		}
		selection = &r.Selection
	case QueueTaskBinding:
		selection = &r.Selection
	case QueueFinishRequest:
		selection = &r.Selection
		reason = r.Reason
	case QueueReconcileRequest:
		selection = &r.Selection
		reason = r.Reason
	}
	if owner != "" && !queueOwnerValid(owner) {
		return fault("queue_owner_invalid", "owner must be bounded nonempty UTF-8")
	}
	if !utf8.ValidString(reason) || len(reason) > 4096 {
		return fault("queue_command_invalid", "reason must be bounded UTF-8")
	}
	if selection != nil {
		if !queueOwnerValid(selection.Owner) {
			return fault("queue_owner_invalid", "selection owner must be bounded nonempty UTF-8")
		}
		data, err := json.Marshal(selection)
		if err != nil {
			return err
		}
		if flow.ValidateProtocol("QueueTaskSelection", data) != nil {
			return fault("queue_selection_invalid", "selection fields must match the closed queue contract")
		}
	}
	return nil
}

func (e *Engine) queueCommand(ctx context.Context, id, operation string, request any) (local.AuthorityCommand, error) {
	if e.ReadOnly {
		return local.AuthorityCommand{}, local.ErrReadOnly
	}
	if _, err := e.readAccess(ctx); err != nil {
		return local.AuthorityCommand{}, err
	}
	if id == "" {
		return local.AuthorityCommand{}, fault("queue_command_required", "queue mutations require a stable command identity")
	}
	if err := queueEnvelopeValid(request); err != nil {
		return local.AuthorityCommand{}, err
	}

	payload, err := canonical(map[string]any{"operation": operation, "authority_id": e.Installation.ID, "project_id": e.Config.ID, "request": request})
	if err != nil {
		return local.AuthorityCommand{}, err
	}
	encodedID, _ := json.Marshal(id)
	if err := flow.ValidateProtocol("Identifier", encodedID); err != nil {
		return local.AuthorityCommand{}, fault("queue_command_invalid", "command identity is outside the supported shape")
	}
	if len(payload) > local.MaxCommandBytes {
		return local.AuthorityCommand{}, fault("queue_capacity", "encoded command exceeds the authority document limit")
	}
	return local.AuthorityCommand{ID: id, Actor: e.owner, Key: TaskQueueKey, Payload: payload}, nil
}

func (e *Engine) queueReplay(ctx context.Context, id, operation string, request any) (local.AuthorityApplyResult, bool, error) {
	command, err := e.queueCommand(ctx, id, operation, request)
	if err != nil {
		return local.AuthorityApplyResult{}, false, err
	}
	receipt, err := e.Store.LookupAuthorityReceipt(ctx, command)
	if errors.Is(err, local.ErrNotFound) {
		return local.AuthorityApplyResult{}, false, nil
	}
	return local.AuthorityApplyResult{Receipt: receipt, Duplicate: err == nil}, err == nil, err
}

func (e *Engine) queueApply(ctx context.Context, id, operation string, request any, transform func(*TaskQueue) (any, error), pins ...local.RunPin) (local.AuthorityApplyResult, error) {
	command, err := e.queueCommand(ctx, id, operation, request)
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	command.RunPins = pins
	if reservation, ok := request.(QueueReserveRequest); ok && operation == "queue.reserve" {
		command.AbsentCommandID = reservation.CommandID
	}
	command.AuthorityGuards = []local.AuthorityGuard{e.queueAccessGuard(operation)}
	return e.Store.ApplyAuthority(ctx, command, func(snapshot local.AuthoritySnapshot) (local.AuthorityChange, error) {
		queue, err := e.queueSnapshot(snapshot)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		result, err := transform(&queue)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		data, err := canonical(queue)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		if len(data) > local.MaxSnapshotBytes {
			return local.AuthorityChange{}, local.Reject("queue_capacity", "queue history exceeds the authority document limit; nothing was truncated")
		}
		if _, err := e.queueSnapshot(local.AuthoritySnapshot{Version: 1, Data: data}); err != nil {
			return local.AuthorityChange{}, err
		}
		body, err := canonical(result)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		return local.AuthorityChange{Data: data, Result: body}, nil
	})
}

func (e *Engine) QueueAdd(ctx context.Context, request QueueAddRequest) (local.AuthorityApplyResult, error) {
	if result, found, err := e.queueReplay(ctx, request.CommandID, "queue.add", request); found || err != nil {
		return result, err
	}
	input, err := queueRequest(request.RequestBytes)
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	for _, ref := range input.SourceRefs {
		if _, err := e.SourceSnapshot(ref); err != nil {
			return local.AuthorityApplyResult{}, err
		}
	}
	if !queuePriorityValid(request.Priority) {
		return local.AuthorityApplyResult{}, fault("queue_priority_invalid", "priority must be an exact JSON integer")
	}
	return e.queueApply(ctx, request.CommandID, "queue.add", request, func(queue *TaskQueue) (any, error) {
		if queue.NextOrdinal == queueMaxInteger {
			return nil, local.Reject("queue_capacity", "insertion order has reached its exact integer limit")
		}
		id := derivedID("queue-task", e.Installation.ID, request.CommandID)
		task := QueueTask{ID: id, Revision: 1, Title: input.Title, Priority: request.Priority, Ordinal: queue.NextOrdinal, State: "ready", Revisions: []QueueTaskRevision{{Revision: 1, RequestBytes: append([]byte(nil), request.RequestBytes...), RequestDigest: rawDigest(request.RequestBytes), Priority: request.Priority}}, Intents: map[string]QueueLaunchIntent{}, Associations: []QueueRunAssociation{}}
		if _, exists := queue.Tasks[id]; exists {
			return nil, local.Reject("queue_task_identity_conflict", "task identity is already recorded; inspect its original command")
		}
		queue.Tasks[id] = task
		queue.NextOrdinal++
		return queueTaskSummary(task), nil
	})
}

func queueTaskOwned(task QueueTask, revision int64, owner string, generation int64) error {
	if task.Revision != revision {
		return local.Reject("queue_task_revision_conflict", "task content changed; read and review its current revision")
	}
	if task.State != "claimed" || task.Claim == nil || task.Claim.Owner != owner || task.Claim.Generation != generation {
		return local.Reject("queue_task_claim_conflict", "owner or claim generation is no longer current")
	}
	return nil
}

func (e *Engine) QueueUpdate(ctx context.Context, request QueueUpdateRequest) (local.AuthorityApplyResult, error) {
	if result, found, err := e.queueReplay(ctx, request.CommandID, "queue.update", request); found || err != nil {
		return result, err
	}
	input, err := queueRequest(request.RequestBytes)
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	for _, ref := range input.SourceRefs {
		if _, err := e.SourceSnapshot(ref); err != nil {
			return local.AuthorityApplyResult{}, err
		}
	}
	if !queuePriorityValid(request.Priority) {
		return local.AuthorityApplyResult{}, fault("queue_priority_invalid", "priority must be an exact JSON integer")
	}
	return e.queueApply(ctx, request.CommandID, "queue.update", request, func(queue *TaskQueue) (any, error) {
		task, exists := queue.Tasks[request.TaskID]
		if !exists {
			return nil, local.Reject("queue_task_missing", "task is not recorded in this project queue")
		}
		if task.Revision != request.ExpectedRevision {
			return nil, local.Reject("queue_task_revision_conflict", "task content changed; read its current revision")
		}
		if task.State == "done" {
			return nil, local.Reject("queue_task_done", "a completed local task cannot be edited")
		}
		if task.State == "claimed" {
			if err := queueTaskOwned(task, request.ExpectedRevision, request.Owner, request.ClaimGeneration); err != nil {
				return nil, err
			}
		} else if request.Owner != "" || request.ClaimGeneration != 0 {
			return nil, local.Reject("queue_task_claim_conflict", "ready task has no owner or claim generation to spend")
		}
		for _, intent := range task.Intents {
			if intent.State == "reserved" || intent.State == "dispatching" {
				return nil, local.Reject("queue_task_busy", "an unfinished launch intent pins this task revision; inspect or reconcile it")
			}
		}
		if task.Revision == queueMaxInteger {
			return nil, local.Reject("queue_capacity", "task revision has reached its exact integer limit")
		}
		task.Revision++
		task.Title = input.Title
		task.Priority = request.Priority
		task.Revisions = append(task.Revisions, QueueTaskRevision{Revision: task.Revision, RequestBytes: append([]byte(nil), request.RequestBytes...), RequestDigest: rawDigest(request.RequestBytes), Priority: request.Priority})
		queue.Tasks[task.ID] = task
		return queueTaskSummary(task), nil
	})
}

func (e *Engine) QueueClaimNext(ctx context.Context, request QueueClaimRequest) (local.AuthorityApplyResult, error) {
	if result, found, err := e.queueReplay(ctx, request.CommandID, "queue.claim-next", request); found || err != nil {
		return result, err
	}
	if !queueOwnerValid(request.Owner) {
		return local.AuthorityApplyResult{}, fault("queue_owner_invalid", "claim owner must be a bounded nonempty session identity")
	}
	return e.queueApply(ctx, request.CommandID, "queue.claim-next", request, func(queue *TaskQueue) (any, error) {
		var selected *QueueTask
		for _, task := range queue.Tasks {
			if task.State != "ready" {
				continue
			}
			if selected == nil || queueTaskEarlier(queueTaskSummary(task), queueTaskSummary(*selected)) {
				copy := task
				selected = &copy
			}
		}
		if selected == nil {
			return nil, local.Reject("queue_empty", "no ready task is recorded in this project queue")
		}
		if selected.ClaimGeneration == queueMaxInteger {
			return nil, local.Reject("queue_capacity", "claim generation has reached its exact integer limit")
		}
		selected.ClaimGeneration++
		selected.Claim = &QueueTaskClaim{ID: derivedID("task-claim", selected.ID, fmt.Sprint(selected.ClaimGeneration)), Owner: request.Owner, Generation: selected.ClaimGeneration}
		selected.State = "claimed"
		queue.Tasks[selected.ID] = *selected
		return queueTaskSummary(*selected), nil
	})
}
