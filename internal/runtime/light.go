package runtime

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/stenhigh/prifly/internal/local"
)

type lightCachedRun struct {
	Version   int64
	Run       Run
	Title     string
	Origin    string
	Summary   LightRun
	Resources []lightResourceCandidate
}
type lightResourceCandidate struct {
	Activation string
	Stage      string
	Name       string
	Limit      int64
}
type lightArtifact struct {
	Meta  Artifact
	Value any
}

// LightCache belongs to one source reader. Callers serialize refreshes; published
// views contain copies, so an older generation cannot change under a cursor.
type LightCache struct {
	verificationMu sync.Mutex
	verification   *local.ReadVerification
	Runs           map[string]lightCachedRun
	Artifacts      map[ArtifactRef]lightArtifact
	Queue          TaskQueue
	QueueVersion   int64
	Reads          int
	artifactBytes  int64
	budget         int64
}

func NewLightCache() *LightCache {
	return &LightCache{Runs: map[string]lightCachedRun{}, Artifacts: map[ArtifactRef]lightArtifact{}, QueueVersion: -1}
}

func (e *Engine) Light(ctx context.Context, c *LightCache, p *LightProjection) (LightSourceView, error) {
	v := LightSourceView{Cards: []LightCard{}, Runs: map[string][]LightRun{}, Reviews: map[string][]LightReview{}}
	if err := e.MonitorAccess(ctx); err != nil {
		return v, err
	}
	versions := map[string]int64{}
	for id, entry := range c.Runs {
		versions[id] = entry.Version
	}
	c.Reads = 0
	c.budget = 16 << 20
	read, err := e.Store.StreamLight(ctx, TaskQueueKey, versions, func(snap local.Snapshot, cut int64) error {
		r, err := e.decodeSnapshot(snap)
		if err != nil {
			return err
		}
		v.Decodes++
		c.Runs[snap.RunID] = e.lightCacheRun(r, snap.Version, snap.EventSeq, cut, c, p)
		return nil
	})
	if err != nil {
		return v, err
	}
	v.Cut = read.Cut
	if read.Queue.Version != c.QueueVersion {
		q, err := e.queueSnapshot(read.Queue)
		if err != nil {
			return v, err
		}
		c.Queue = q
		c.QueueVersion = read.Queue.Version
		v.QueueDecodes++
	}
	present := map[string]bool{}
	for _, rev := range read.Revisions {
		present[rev.RunID] = true
	}
	for id := range c.Runs {
		if !present[id] {
			delete(c.Runs, id)
		}
	}
	cards := map[string]*LightCard{}
	for id, t := range c.Queue.Tasks {
		state, tid, revision := t.State, t.ID, t.Revision
		card := lightEmptyCard(e, "task:"+id, "queue_task", t.Title, "queue")
		card.TaskID = &tid
		card.TaskRevision = &revision
		card.QueueState = &state
		card.Status = state
		card.Priority = t.Priority
		card.Ordinal = t.Ordinal
		card.Lane = "queue"
		card.Phase = LightPhase{Key: "queue", Label: "Очередь"}
		card.CompletionScope = "Задача ещё не запускалась"
		if state == "claimed" {
			card.Lane = "work"
			card.Phase = LightPhase{Key: "work", Label: "Ожидание запуска"}
			card.CompletionScope = "Задача захвачена; запуск не подтверждён"
		}
		for _, intent := range t.Intents {
			admitted, known := c.Runs[intent.RunID]
			bound := known && admitted.Run.QueueTask != nil && admitted.Run.QueueTask.IntentID == intent.ID && admitted.Run.QueueTask.Selection == intent.Selection
			if intent.State != "admitted" && intent.State != "resolved" && intent.State != "revoked" && !bound {
				card.Lane = "attention"
				card.Reasons = append(card.Reasons, MonitorReason{Code: "launch_intent", Text: "Незавершённое намерение запуска: " + intent.State, Action: "facts"})
			}
		}
		cards[card.Key] = &card
	}
	ids := make([]string, 0, len(c.Runs))
	for id := range c.Runs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		entry := c.Runs[id]
		r := entry.Run
		key := "run:" + id
		if r.QueueTask != nil {
			key = "task:" + r.QueueTask.Selection.TaskID
		}
		card := cards[key]
		if card == nil {
			created := lightEmptyCard(e, key, "run", entry.Title, entry.Origin)
			if r.QueueTask != nil {
				created.Kind = "queue_task"
				tid := r.QueueTask.Selection.TaskID
				created.TaskID = &tid
				created.Reasons = append(created.Reasons, MonitorReason{Code: "queue_record_missing", Text: "Запись задачи отсутствует; связь сохранена в Run", Action: "facts"})
			}
			cards[key] = &created
			card = &created
		}
		if r.ProjectTitle != "" {
			card.ProjectTitle = r.ProjectTitle
		}
		summary := entry.Summary
		summary.Board = lightAuthorityBoard(entry, read)
		summary.ReasonCount = len(summary.Board.Reasons)
		summary.Reviews = slices.Clone(entry.Summary.Reviews)
		for i := range summary.Reviews {
			summary.Reviews[i].Cut = read.Cut
		}

		v.Runs[key] = append(v.Runs[key], summary)
		v.Reviews[id] = summary.Reviews
		if summary.LastObserved > card.LastObserved {
			card.LastObserved = summary.LastObserved
		}
	}
	// The immutable Run binding wins over an index that may lag admission. A
	// disagreement remains visible and cannot be repaired by an observer.
	for _, t := range c.Queue.Tasks {
		card := cards["task:"+t.ID]
		for _, a := range t.Associations {
			entry, ok := c.Runs[a.RunID]
			if !ok || entry.Run.QueueTask == nil || entry.Run.QueueTask.Selection.TaskID != t.ID || *entry.Run.QueueTask != a.Binding {
				card.Reasons = append(card.Reasons, MonitorReason{Code: "association_integrity", Text: "Связь Run отсутствует или противоречит binding: " + a.RunID, Action: "facts"})
			}
		}
	}
	for key, card := range cards {
		lightSummarize(card, v.Runs[key])
		v.Cards = append(v.Cards, *card)
	}
	sort.Slice(v.Cards, func(i, j int) bool { return v.Cards[i].Key < v.Cards[j].Key })
	v.ArtifactReads = c.Reads
	return v, nil
}
func lightEmptyCard(e *Engine, key, kind, title, origin string) LightCard {
	if strings.TrimSpace(title) == "" {
		title = "Название не записано"
		origin = "fallback"
	}
	return LightCard{Key: key, Kind: kind, AuthorityID: e.Installation.ID, ProjectID: e.Config.ID, ProjectTitle: e.Config.ID, Title: title, TitleOrigin: origin, Reasons: []MonitorReason{}, CurrentRuns: []LightRun{}, ActiveBranches: []MonitorStage{}, Reviews: []LightReviewChip{}, Phase: LightPhase{Key: "work", Label: "В работе / этап не сопоставлен"}}
}
func (e *Engine) lightTitle(r Run, c *LightCache) (string, string) {
	if r.Brief != (ArtifactRef{}) {
		if item, err := e.lightArtifact(r, c, r.Brief, nil); err == nil {
			if value, ok := lightPointer(item.Value, "/subject"); ok {
				if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
					return s, "brief"
				}
			}
		}
	}
	if ref, ok := r.Inputs["task"]; ok {
		if item, err := e.lightArtifact(r, c, ref, nil); err == nil {
			if value, ok := lightPointer(item.Value, "/title"); ok {
				if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
					return s, "task_input"
				}
			}
		}
	}
	return "Название задачи не записано", "fallback"
}
func lightPhases(r Run, p *LightProjection, b *MonitorBoard) []LightPhase {
	phases := []LightPhase{}
	if p == nil {
		return phases
	}
	for _, stage := range b.Stages {
		ref := r.WorkflowRef
		if inv := r.Invocations[stage.InvocationID]; inv != nil {
			ref = inv.WorkflowRef
		}
		for _, w := range p.Workflows {
			if w.WorkflowRef != ref {
				continue
			}
			for _, rule := range w.Phases {
				if stage.StageID == rule.Stage {
					phase := LightPhase{rule.Key, rule.Label, true, p.Digest}
					if !slices.Contains(phases, phase) {
						phases = append(phases, phase)
					}
				}
			}
		}
	}
	sort.Slice(phases, func(i, j int) bool { return phases[i].Key < phases[j].Key })
	return phases
}
func lightSummarize(card *LightCard, all []LightRun) {
	card.HistoryCount = len(all)
	parents := map[string]bool{}
	for _, r := range all {
		if r.Parent != r.ID {
			parents[r.Parent] = true
		}
	}
	frontier := []LightRun{}
	for _, r := range all {
		if !parents[r.ID] {
			frontier = append(frontier, r)
		}
	}
	if len(frontier) == 0 && len(all) > 0 {
		frontier = all
		card.Reasons = append(card.Reasons, MonitorReason{Code: "lineage_integrity", Text: "Невозможно установить текущие Runs", Action: "facts"})
	}
	card.CurrentRuns = frontier
	card.CurrentCount = len(frontier)
	if len(frontier) > 0 {
		card.Status = "mixed"
		card.Lane = "work"
		card.CompletionScope = "Исполнение Run; внешняя поставка не подтверждена"
	}
	if len(frontier) == 1 {
		card.Status = frontier[0].Status
		card.Outcome = frontier[0].Outcome
	}
	terminal, success := len(frontier) > 0, len(frontier) > 0
	for _, r := range frontier {
		isTerminal := slices.Contains([]string{"completed", "failed", "cancelled"}, r.Status)
		terminal = terminal && isTerminal
		success = success && r.Status == "completed" && r.Outcome != nil && *r.Outcome == "succeeded"
		if r.Board != nil {
			card.Reasons = append(card.Reasons, r.Board.Reasons...)
			card.ActiveBranches = append(card.ActiveBranches, r.Board.Stages...)
			if slices.Contains([]string{"stopped", "decision"}, r.Board.Category) || isTerminal && (r.Status != "completed" || r.Outcome == nil || *r.Outcome != "succeeded") {
				card.Lane = "attention"
			}
		}
		for _, phase := range r.Phases {
			if !card.Phase.Mapped || phase.Key < card.Phase.Key {
				card.Phase = phase
			}
		}
		for _, review := range r.Reviews {
			if review.Freshness == "stale" {
				continue
			}
			if review.State == "defect" || review.State == "unavailable" || review.Blocking || review.OwnerOnly {
				card.Lane = "attention"
				card.Reasons = append(card.Reasons, MonitorReason{Code: "review_gate_blocked", Text: review.Lens + ": " + review.Reason, Action: "facts"})
			}
			card.Reviews = append(card.Reviews, LightReviewChip{ID: review.ID, RunID: review.RunID, Lens: review.Lens, State: review.State, CheckStatus: review.CheckStatus, GateStatus: review.GateStatus, Freshness: review.Freshness, GateScope: review.GateScope, Reason: lightPreview(review.Reason, 160)})
		}
	}
	if success && card.Lane != "attention" {
		if card.Kind == "run" {
			card.Lane = "done"
			card.CompletionScope = "Run завершён"
		} else {
			card.Lane = "result"
			card.CompletionScope = "Run успешен; завершение задачи владельцем не подтверждено"
		}
	}
	if card.QueueState != nil && *card.QueueState == "done" {
		if len(frontier) == 0 || terminal {
			card.Lane = "done"
			card.CompletionScope = "Локальная обработка задачи завершена владельцем"
		} else {
			card.Lane = "attention"
			card.Reasons = append(card.Reasons, MonitorReason{Code: "queue_done_conflict", Text: "Завершённая задача связана с незавершённым Run", Action: "facts"})
		}
	}
	for _, reason := range card.Reasons {
		if slices.Contains([]string{"association_integrity", "queue_record_missing", "launch_intent", "lineage_integrity"}, reason.Code) {
			card.Lane = "attention"
		}
	}
	if card.Lane == "work" && card.Phase.Mapped {
		card.Lane = card.Phase.Key
	}
	if len(frontier) == 1 {
		var basis *LightReview
		consistent := true
		for i := range frontier[0].Reviews {
			r := &frontier[0].Reviews[i]
			if r.Freshness != "fresh" || r.Round == nil || r.CandidateDigest == nil {
				continue
			}
			if basis != nil && (*r.Round != *basis.Round || *r.CandidateDigest != *basis.CandidateDigest) {
				consistent = false
			}
			basis = r
		}
		if basis != nil && consistent {
			card.Round = basis.Round
			card.CandidateDigest = basis.CandidateDigest
		}
	}
	card.BranchCount = len(card.ActiveBranches)
	card.ReviewCount = len(card.Reviews)
}
func lightPreview(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
func lightIntegrity(message string) error {
	return fault("light_source_integrity", message)
}

// OpenLight reuses only a verified immutable journal prefix. It still opens a
// short-lived engine and rechecks owner, current control and storage identity;
// no installation lock or database connection is retained between requests.
func OpenLight(root string, c *LightCache) (*Engine, error) {
	c.verificationMu.Lock()
	defer c.verificationMu.Unlock()
	e, err := openEngineVerified(root, true, false, c.verification)
	if err == nil {
		c.verification = e.Store.ReadVerification()
	}
	return e, err
}

func (e *Engine) lightCacheRun(r Run, version, events, cut int64, c *LightCache, p *LightProjection) lightCachedRun {
	title, origin := e.lightTitle(r, c)
	base := monitorBoard(r, local.MonitorRead{Cut: cut})
	summary := LightRun{ID: r.ID, Version: version, Events: events, Workflow: r.WorkflowRef, Status: r.Status, Outcome: r.Outcome, LastObserved: r.LastObserved.UTC, Board: base, BranchCount: len(base.Stages), ReasonCount: len(base.Reasons), Phases: lightPhases(r, p, base), Reviews: e.lightReviews(r, c, p, cut)}
	if r.QueueTask != nil {
		revision := r.QueueTask.Selection.TaskRevision
		summary.TaskRevision = &revision
	}
	if r.Fork != nil {
		summary.Parent = r.Fork.SourceRunID
	}
	if r.Recovery != nil {
		summary.Parent = r.Recovery.SourceRunID
	}
	resources := []lightResourceCandidate{}
	if len(r.StageResources) > 0 {
		for _, activation := range r.Activations {
			if activation != nil && monitorResourceCandidate(r, activation) {
				for _, claim := range r.resourcesFor(activation.ID) {
					resources = append(resources, lightResourceCandidate{activation.ID, activation.StageID, claim.Name, claim.Limit})
				}
			}
		}
	}
	// Retain only grouping metadata. Graphs, definitions, envelopes, prior
	// Attempts and artifact bodies are not the process index.
	compact := Run{ID: r.ID, ProjectTitle: r.ProjectTitle, QueueTask: r.QueueTask}
	return lightCachedRun{Version: version, Run: compact, Title: title, Origin: origin, Summary: summary, Resources: resources}
}
func lightAuthorityBoard(entry lightCachedRun, read local.LightRead) *MonitorBoard {
	board := *entry.Summary.Board
	board.Cut = read.Cut
	board.Reasons = slices.Clone(board.Reasons)
	if board.Category == "terminal" {
		return &board
	}
	add := func(reason MonitorReason) {
		rank := map[string]int{"stopped": 1, "decision": 2, "working": 3, "resource": 4, "waiting": 5, "ready": 6, "unknown": 7}
		if rank[board.Category] > rank["resource"] {
			board.Category = "resource"
		}
		if len(board.Reasons) == 1 && board.Reasons[0].Code == "unknown" {
			board.Reasons = []MonitorReason{}
		}
		board.Reasons = append(board.Reasons, reason)
	}
	if read.Waiting[entry.Summary.ID] {
		add(MonitorReason{Code: "admission", Text: "Authority зарегистрировала ожидание admission; точная причина не записана", Action: "facts"})
	}
	for _, candidate := range entry.Resources {
		if candidate.Limit > 0 && candidate.Limit <= MaxResourceLimit && read.Resources != nil && read.Resources[candidate.Name] >= candidate.Limit {
			add(MonitorReason{Code: "resource_busy", Text: fmt.Sprintf("Ресурс %s занят %d/%d для готовой стадии %s", candidate.Name, read.Resources[candidate.Name], candidate.Limit, candidate.Stage), Action: "facts", ActivationID: candidate.Activation})
		}
	}
	return &board
}
