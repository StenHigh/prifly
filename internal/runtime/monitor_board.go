package runtime

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/stenhigh/prifly/internal/local"
)

// MonitorBoard is an optional local presentation, never a saved Run state.
// All fields share Cut with the snapshot used by MonitorSummary.
type MonitorBoard struct {
	SchemaVersion string          `json:"schema_version"`
	Cut           int64           `json:"cut"`
	Category      string          `json:"category"`
	NextAction    string          `json:"next_action"`
	Reasons       []MonitorReason `json:"reasons"`
	Stages        []MonitorStage  `json:"stages"`
}
type MonitorReason struct {
	Code         string `json:"code"`
	Text         string `json:"text"`
	Action       string `json:"action"`
	ActivationID string `json:"stage_activation_id,omitempty"`
	AttemptID    string `json:"attempt_id,omitempty"`
}
type MonitorStage struct {
	StageID      string           `json:"stage_id"`
	InvocationID string           `json:"workflow_invocation_id"`
	ActivationID string           `json:"stage_activation_id,omitempty"`
	Branches     []string         `json:"branches"`
	Kind         string           `json:"kind,omitempty"`
	Status       string           `json:"status"`
	Attempts     []MonitorAttempt `json:"attempts"`
}
type MonitorAttempt struct {
	ID        string `json:"attempt_id"`
	Status    string `json:"status"`
	HostState string `json:"host_state,omitempty"`
}

func monitorBranches(r Run, invocation string) []string {
	branches := []string{}
	seen := map[string]bool{}
	for inv := r.Invocations[invocation]; inv != nil && !seen[inv.ID]; inv = r.Invocations[inv.ParentInvocationID] {
		seen[inv.ID] = true
		if inv.BranchID != "" {
			branches = append(branches, inv.BranchID)
		}
	}
	slices.Reverse(branches)
	return branches
}

func monitorBoard(r Run, view local.MonitorRead) *MonitorBoard {
	b := &MonitorBoard{SchemaVersion: "local-run-board/1", Cut: view.Cut, Category: "unknown", Reasons: []MonitorReason{}, Stages: []MonitorStage{}}
	b.NextAction, _ = nextKind(r)
	rank := map[string]int{"terminal": 0, "stopped": 1, "decision": 2, "working": 3, "resource": 4, "waiting": 5, "ready": 6, "unknown": 7}
	add := func(category, code, text, action, activation, attempt string) {
		if rank[category] < rank[b.Category] {
			b.Category = category
		}
		b.Reasons = append(b.Reasons, MonitorReason{Code: code, Text: text, Action: action, ActivationID: activation, AttemptID: attempt})
	}
	if r.terminal() {
		text := r.Status
		if r.Outcome != nil {
			text += " / " + *r.Outcome
		}
		add("terminal", "terminal", text, "facts", "", "")
		if f := runFailure(r); f != nil {
			add("terminal", f.Code, "Диагностика: "+f.Code, "journal", "", f.AttemptID)
		}
		return b
	}
	if r.HasUnresolvedEffects || r.Status == "uncertain" {
		add("stopped", "unresolved", "Исход обязательств не установлен; требуется разрешение неопределённости", "facts", "", "")
	}
	if r.CancelRequested || r.Status == "stopping" {
		add("stopped", "cancelling", "Запрошена отмена; завершение ещё не записано", "journal", "", "")
	}
	if b.NextAction == "restricted" || b.NextAction == "guarded" {
		add("stopped", b.NextAction, "Допуск ограничен сохранёнными правилами; требуется проверка управления", "facts", "", "")
	}
	if r.ResumeRequired {
		add("stopped", "resume_required", "Требуется явное возобновление", "facts", "", "")
	}
	for _, stop := range r.Stops {
		if stop.Status == "active" {
			add("stopped", "stop", stop.Kind+": "+stop.Reason, "facts", "", "")
		}
	}
	if d := r.PendingDecision; d != nil {
		add("decision", "decision", "Ожидается ответ: "+d.DecisionID, "facts", "", d.AttemptID)
	}
	ids := make([]string, 0, len(r.Activations))
	for id, a := range r.Activations {
		if a != nil && a.Settled == nil {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	covered := map[string]bool{}
	for _, id := range ids {
		a := r.Activations[id]
		stage := MonitorStage{StageID: a.StageID, InvocationID: a.InvocationID, ActivationID: a.ID, Branches: monitorBranches(r, a.InvocationID), Kind: a.Kind, Status: a.Status, Attempts: []MonitorAttempt{}}
		covered[a.InvocationID+"\x00"+a.StageID] = true
		attemptIDs := []string{}
		for id, attempt := range r.Attempts {
			if attempt != nil && attempt.ActivationID == a.ID && attempt.Settled == nil {
				attemptIDs = append(attemptIDs, id)
			}
		}
		sort.Strings(attemptIDs)
		for _, id := range attemptIDs {
			attempt := r.Attempts[id]
			item := MonitorAttempt{ID: id, Status: attempt.Status}
			switch {
			case attempt.Session != nil:
				session := attempt.Session
				item.HostState = session.HostState
				switch session.HostState {
				case sessionWaitingDecision:
					add("decision", "decision", "Сессия ожидает ответ владельца", "facts", a.ID, id)
				case SessionWaitingAdmission:
					add("resource", "admission", "Сессия ожидает повторный допуск", "facts", a.ID, id)
				case SessionDisconnected:
					add("stopped", "disconnected", "Связь с хостом потеряна; исход требует проверки", "facts", a.ID, id)
				case SessionAwaiting:
					if session.Taken != nil {
						add("working", "host_taken", "Хост записал принятие задания; внешняя активность не измеряется", "workflow", a.ID, id)
					} else {
						add("waiting", "host", "Задание выдано; хост ещё не записал принятие", "workflow", a.ID, id)
					}
				default:
					add("ready", "session_reported", "Результат сессии ожидает обработки движком", "facts", a.ID, id)
				}
			case attempt.Started != nil && attempt.ExecutorEnd == nil && attempt.CandidateAt == nil:
				add("working", "started", "Записан старт исполнения; завершение ещё не записано", "workflow", a.ID, id)
			case attempt.ExecutorEnd != nil || attempt.CandidateAt != nil:
				add("ready", "candidate", "Результат ожидает обработки движком", "facts", a.ID, id)
			default:
				add("waiting", "pending", "Попытка допущена; начало исполнения не записано", "workflow", a.ID, id)
			}
			stage.Attempts = append(stage.Attempts, item)
		}
		if a.Kind == "wait" && a.Wait != nil && a.Wait.Resolution == "" && r.Waits[a.Wait.RegistrationID] != nil && r.Waits[a.Wait.RegistrationID].Status == "active" {
			add("waiting", "signal", "Стадия ожидает сигнал", "facts", a.ID, "")
		}
		if a.Status == "ready" {
			add("ready", "ready", "Стадия готова к обработке движком: "+a.StageID, "workflow", a.ID, "")
		}
		// Only a NEW ready Step admission acquires resources. Checks and timed
		// session resumes acquire slots without resource claims.
		if view.Resources != nil && monitorResourceCandidate(r, a) {
			for _, claim := range r.resourcesFor(a.ID) {
				if claim.Limit > 0 && claim.Limit <= MaxResourceLimit && view.Resources[claim.Name] >= claim.Limit {
					add("resource", "resource_busy", fmt.Sprintf("Ресурс %s занят %d/%d для готовой стадии %s", claim.Name, view.Resources[claim.Name], claim.Limit, a.StageID), "facts", a.ID, "")
				}
			}
		}
		b.Stages = append(b.Stages, stage)
	}
	invocationIDs := []string{}
	for id, inv := range r.Invocations {
		if inv != nil && inv.Settled == nil {
			invocationIDs = append(invocationIDs, id)
		}
	}
	if len(r.Invocations) == 0 {
		invocationIDs = append(invocationIDs, r.RootInvocationID)
	}
	sort.Strings(invocationIDs)
	for _, id := range invocationIDs {
		if inv := r.Invocations[id]; inv != nil {
			if inv.CancelRequested {
				add("stopped", "branch_cancel", "Запрошена отмена invocation "+id, "journal", "", "")
			}
			if inv.ResumeRequired {
				add("stopped", "branch_resume", "Invocation требует возобновления: "+id, "facts", "", "")
			}
		}
		for _, stage := range r.readyFor(id) {
			if covered[id+"\x00"+stage] {
				continue
			}
			b.Stages = append(b.Stages, MonitorStage{StageID: stage, InvocationID: id, Branches: monitorBranches(r, id), Status: "ready", Attempts: []MonitorAttempt{}})
			add("ready", "ready", "Стадия готова к обработке движком: "+stage, "workflow", "", "")
		}
	}
	if view.Waiting {
		add("resource", "admission", "Authority зарегистрировала ожидание admission; точная причина не записана", "facts", "", "")
	}
	if c := r.CheckExecutions[r.ActiveCheckID]; c != nil {
		if c.Started != nil && c.ExecutorEnd == nil {
			add("working", "check_started", "Записан старт проверки", "facts", "", "")
		} else {
			add("ready", "check", "Проверка ожидает обработки движком", "facts", "", "")
		}
	}
	if r.PendingAcceptance != nil {
		add("ready", "acceptance", "Ожидается проверка или приёмка результата", "facts", r.PendingAcceptance.ActivationID, "")
	}
	if r.PendingArtifactPublication != nil {
		add("ready", "publication", "Публикация ожидает проверок", "facts", r.PendingArtifactPublication.ActivationID, "")
	}
	sort.Slice(b.Stages, func(i, j int) bool {
		a, c := b.Stages[i], b.Stages[j]
		return strings.Join([]string{a.InvocationID, a.StageID, a.ActivationID}, "\x00") < strings.Join([]string{c.InvocationID, c.StageID, c.ActivationID}, "\x00")
	})
	if len(b.Reasons) == 0 {
		add("unknown", "unknown", "Причина текущего состояния не записана", "journal", "", "")
	}
	return b
}

func monitorResourceCandidate(r Run, a *Activation) bool {
	step := r.Steps[a.StepID]
	if a.Kind != "step" || a.Status != "ready" || step == nil || step.Status != "ready" || !slices.Contains(r.readyFor(a.InvocationID), a.StageID) || r.activeIn(a.InvocationID) != "" {
		return false
	}
	if r.HasUnresolvedEffects || r.Status == "uncertain" || r.admissionsBlockedFor(a.InvocationID) || r.cancelRequestedFor(a.InvocationID) || r.ActiveCheckID != "" || r.PendingArtifactPublication != nil {
		return false
	}
	if r.PendingAcceptance != nil && !pendingPassed(r, "step_input", a.ID) {
		return false
	}
	if guard, _ := r.guardBlock(a.InvocationID, a.StageID); guard != "" {
		return false
	}
	limit, err := r.declaredParallelism()
	return err == nil && r.executingAttempts() < limit
}
