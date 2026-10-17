package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

type lightSourceSnapshot struct {
	Info      prifly.LightSourceInfo
	View      prifly.LightSourceView
	StateRoot string
}
type lightGeneration struct {
	ID      string
	Created time.Time
	Sources map[string]lightSourceSnapshot
}
type lightService struct {
	catalog     *monitorCatalog
	options     lightOptions
	mu          sync.RWMutex
	refreshMu   sync.Mutex
	caches      map[string]*prifly.LightCache
	latest      map[string]lightSourceSnapshot
	generations []*lightGeneration
}

func newLightService(catalog *monitorCatalog, options lightOptions) *lightService {
	return &lightService{catalog: catalog, options: options, caches: map[string]*prifly.LightCache{}, latest: map[string]lightSourceSnapshot{}}
}
func (s *lightService) start(ctx context.Context) {
	s.catalog.registered()
	if len(s.catalog.discovery.Roots) > 0 {
		go s.catalog.scan(ctx)
	}
	go func() {
		for {
			s.refresh(ctx)
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
		}
	}()
}
func (s *lightService) refresh(ctx context.Context) {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	s.catalog.registered()
	for _, source := range s.catalog.sourceList() {
		if ctx.Err() != nil {
			return
		}
		s.mu.RLock()
		old, exists := s.latest[source.ID]
		s.mu.RUnlock()
		info := old.Info
		info.Source = source.ID
		info.AuthorityID = source.Authority
		stateRoot := old.StateRoot
		mark := lightStorageMark(source.Root, stateRoot)
		// A cheap access check still happens for HTTP reads. On refresh, unchanged
		// storage needs no snapshot or artifact decode, but errors are retried.
		if exists && mark != "" && mark == source.mark && old.Info.Freshness == "fresh" {
			continue
		}
		readCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		cache := s.cache(source.ID)
		engine, err := prifly.OpenLight(source.Root, cache)
		var view prifly.LightSourceView
		if err == nil {
			if engine.Installation.ID != source.Authority {
				err = fmt.Errorf("source authority identity changed")
			}
			if err == nil {

				view, err = engine.Light(readCtx, cache, s.options.Projection)
				info.ProjectID = engine.Config.ID
				stateRoot = engine.Config.Configuration.StateRoot
				mark = lightStorageMark(source.Root, stateRoot)
				for i := range view.Cards {
					if view.Cards[i].ProjectTitle == view.Cards[i].ProjectID {
						view.Cards[i].ProjectTitle = "Локальное имя: " + filepath.Base(source.Root)
					}
				}
			}
			_ = engine.Close()
		}
		cancel()
		now := time.Now().UTC().Format(time.RFC3339Nano)
		info.ObservedAt = now
		if err != nil {
			msg := err.Error()
			info.Error = &msg
			info.Freshness = "unavailable"
			info.Coverage = "unsupported"
			if exists {
				info.Freshness = "stale"
				view = old.View
				info.Coverage = old.Info.Coverage
			}
		} else {
			info.Error = nil
			info.LastSuccessAt = now
			info.Freshness = "fresh"
			info.Coverage = "complete"
			info.Cut = view.Cut
			source.mark = mark
		}
		s.mu.Lock()
		s.latest[source.ID] = lightSourceSnapshot{Info: info, View: view, StateRoot: stateRoot}
		s.mu.Unlock()
		s.catalog.mu.Lock()
		s.catalog.sources[source.ID] = source
		s.catalog.mu.Unlock()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.latest) == 0 {
		s.publishLocked()
		return
	}
	// Copies of authorities must not merge cards under a shared identity.
	owners := map[string]string{}
	for id, src := range s.latest {
		if other, ok := owners[src.Info.AuthorityID]; ok {
			for _, key := range []string{id, other} {
				x := s.latest[key]
				msg := "conflicting authority identity at multiple sources"
				x.Info.Error = &msg
				x.Info.Freshness = "unavailable"
				s.latest[key] = x
			}
		} else {
			owners[src.Info.AuthorityID] = id
		}
	}
	s.publishLocked()
}
func (s *lightService) publishLocked() {
	sources := map[string]lightSourceSnapshot{}
	keys := []string{}
	for id := range s.latest {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	signature := ""
	if s.options.Projection != nil {
		signature = s.options.Projection.Digest
	}
	for _, id := range keys {
		source := s.latest[id]
		sources[id] = source
		signature += fmt.Sprintf("|%s:%d:%s:%s", id, source.Info.Cut, source.Info.Freshness, source.Info.Coverage)
		if source.Info.Error != nil {
			signature += *source.Info.Error
		}
	}
	sum := sha256.Sum256([]byte(signature))
	id := fmt.Sprintf("%x", sum)
	if len(s.generations) > 0 && strings.HasPrefix(s.generations[len(s.generations)-1].ID, id+"-") && time.Since(s.generations[len(s.generations)-1].Created) < 4*time.Minute {
		return
	}
	id = fmt.Sprintf("%s-%x", id, time.Now().Unix()/240)
	s.generations = append(s.generations, &lightGeneration{id, time.Now(), sources})
	if len(s.generations) > 8 {
		s.generations = s.generations[len(s.generations)-8:]
	}
}
func (s *lightService) current() *lightGeneration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.generations) == 0 {
		sources := map[string]lightSourceSnapshot{}
		for _, src := range s.catalog.sourceList() {
			sources[src.ID] = lightSourceSnapshot{Info: prifly.LightSourceInfo{Source: src.ID, AuthorityID: src.Authority, Coverage: "indexing", Freshness: "updating"}}
		}
		return &lightGeneration{ID: "indexing", Created: time.Now(), Sources: sources}
	}
	return s.generations[len(s.generations)-1]
}

// Every HTTP read checks current owner/access, even for a retained generation.
func (s *lightService) sourceAccess(ctx context.Context, source prifly.LightSourceInfo) error {
	s.catalog.mu.RLock()
	record, ok := s.catalog.sources[source.Source]
	s.catalog.mu.RUnlock()
	if !ok {
		return fmt.Errorf("unknown registered source")
	}
	e, err := prifly.OpenLight(record.Root, s.cache(source.Source))
	if err != nil {
		return err
	}
	defer e.Close()
	if e.Installation.ID != source.AuthorityID {
		return fmt.Errorf("source identity changed")
	}
	return e.MonitorAccess(ctx)
}
func lightQueryKey(q url.Values) string {
	copy := url.Values{}
	for k, v := range q {
		if k != "cursor" {
			copy[k] = v
		}
	}
	return copy.Encode()
}
func (s *lightService) page(r *http.Request, limit int) (*lightGeneration, int, error) {
	cursor := r.URL.Query().Get("cursor")
	if cursor == "" {
		return s.current(), 0, nil
	}
	pieces := strings.Split(cursor, ".")
	if len(pieces) != 3 {
		return nil, 0, fmt.Errorf("invalid page cursor")
	}
	n, err := strconv.Atoi(pieces[1])
	if err != nil || n < 0 || n%limit != 0 {
		return nil, 0, fmt.Errorf("invalid page offset")
	}
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(lightQueryKey(r.URL.Query()))))
	if pieces[2] != digest {
		return nil, 0, fmt.Errorf("cursor query changed")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, g := range s.generations {
		if g.ID == pieces[0] && time.Since(g.Created) <= 5*time.Minute {
			return g, n, nil
		}
	}
	return nil, 0, fmt.Errorf("page generation expired; refresh with the same filters")
}
func lightPageMeta(g *lightGeneration, q url.Values, start, limit, total int, complete bool) prifly.LightPage {
	page := prifly.LightPage{Limit: limit, Complete: complete}
	if complete {
		page.Total = &total
	}
	if start+limit < total {
		next := fmt.Sprintf("%s.%d.%x", g.ID, start+limit, sha256.Sum256([]byte(lightQueryKey(q))))
		page.NextCursor = &next
	}
	return page
}
func (s *lightService) board(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, err := lightLimit(q, 25, 50)
	if err != nil {
		lightProblem(w, 400, "light_query_invalid", err.Error())
		return
	}
	if lane := q.Get("lane"); lane != "" && !slices.Contains([]string{"attention", "queue", "plan", "implementation", "review", "work", "result", "done"}, lane) {
		lightProblem(w, 400, "light_query_invalid", "unknown lane")
		return
	}
	g, start, err := s.page(r, limit)
	if err != nil {
		lightProblem(w, 409, "light_page_expired", err.Error())
		return
	}
	page := prifly.LightBoardPage{SchemaVersion: "prifly-light-board/1", Generation: g.ID, Sources: []prifly.LightSourceInfo{}, Cards: []prifly.LightBoardCard{}, Counters: map[string]int{}, FullMonitorURL: s.options.FullURL}
	if s.options.Projection != nil {
		page.MappingDigest = &s.options.Projection.Digest
	}
	rows := []prifly.LightBoardCard{}
	complete := true
	keys := []string{}
	for key := range g.Sources {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, id := range keys {
		src := g.Sources[id]
		if q.Get("source") != "" && q.Get("source") != id {
			continue
		}
		if q.Get("project") != "" && q.Get("project") != src.Info.ProjectID {
			continue
		}
		info := src.Info
		if err := s.sourceAccess(r.Context(), info); err != nil {
			message := err.Error()
			info.Error = &message
			info.Freshness = "unavailable"
			complete = false
			page.Sources = append(page.Sources, info)
			continue
		}
		page.Sources = append(page.Sources, info)
		complete = complete && info.Coverage == "complete" && info.Freshness == "fresh"
		for _, card := range src.View.Cards {
			text := strings.ToLower(strings.Join([]string{card.Title, card.ProjectID, card.ProjectTitle, card.Key}, " "))
			for _, run := range src.View.Runs[card.Key] {
				text += " " + strings.ToLower(run.ID)
			}
			if !strings.Contains(text, strings.ToLower(q.Get("q"))) || q.Get("lane") != "" && q.Get("lane") != card.Lane {
				continue
			}
			rows = append(rows, prifly.LightBoardCard{Source: id, LightCard: card})
			page.Counters[card.Lane]++
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		rank := func(c prifly.LightBoardCard) int {
			if c.Lane == "attention" {
				return 0
			}
			if c.Lane == "queue" {
				return 1
			}
			return 2
		}
		if rank(a) != rank(b) {
			return rank(a) < rank(b)
		}
		if a.Lane == "queue" && b.Lane == "queue" {
			if a.Priority != b.Priority {
				return a.Priority > b.Priority
			}
			if a.Ordinal != b.Ordinal {
				return a.Ordinal < b.Ordinal
			}
		}
		if a.LastObserved != b.LastObserved {
			return a.LastObserved > b.LastObserved
		}
		return a.Source+a.Key < b.Source+b.Key
	})
	total := len(rows)
	start = min(start, total)
	for _, row := range rows[start:min(start+limit, total)] {
		row.LightCard = lightCompact(row.LightCard)
		page.Cards = append(page.Cards, row)
	}
	page.Page = lightPageMeta(g, q, start, limit, total, complete)
	lightWrite(w, r, page, true)
}
func lightCompact(card prifly.LightCard) prifly.LightCard {
	card.Title = lightText(card.Title, 100)
	card.ProjectTitle = lightText(card.ProjectTitle, 80)
	card.CurrentRuns = slices.Clone(card.CurrentRuns[:min(2, len(card.CurrentRuns))])
	for i := range card.CurrentRuns {
		card.CurrentRuns[i].Board = nil
		card.CurrentRuns[i].Reviews = []prifly.LightReview{}
		card.CurrentRuns[i].Phases = card.CurrentRuns[i].Phases[:min(2, len(card.CurrentRuns[i].Phases))]
	}
	card.ActiveBranches = slices.Clone(card.ActiveBranches[:min(3, len(card.ActiveBranches))])
	for i := range card.ActiveBranches {
		card.ActiveBranches[i].Attempts = card.ActiveBranches[i].Attempts[:min(2, len(card.ActiveBranches[i].Attempts))]
	}
	card.Reasons = slices.Clone(card.Reasons[:min(2, len(card.Reasons))])
	for i := range card.Reasons {
		card.Reasons[i].Text = lightText(card.Reasons[i].Text, 160)
	}
	card.Reviews = card.Reviews[:min(4, len(card.Reviews))]
	card.Truncated = card.CurrentCount > len(card.CurrentRuns) || card.BranchCount > len(card.ActiveBranches) || card.ReviewCount > len(card.Reviews)
	// Previews yield to the hard card budget. Exact identities remain reachable
	// through the task endpoint; counts tell the user what was not included.
	data, _ := json.Marshal(card)
	if len(data) > 4096 {
		card.Truncated = true
		card.CurrentRuns = []prifly.LightRun{}
		card.ActiveBranches = []prifly.MonitorStage{}
		card.Reviews = []prifly.LightReviewChip{}
		card.Reasons = []prifly.MonitorReason{}
	}
	return card
}
func lightText(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

func (s *lightService) cache(source string) *prifly.LightCache {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.caches[source]
	if c == nil {
		c = prifly.NewLightCache()
		s.caches[source] = c
	}
	return c
}

func lightStorageMark(root, stateRoot string) string {
	if stateRoot == "" {
		stateRoot = filepath.Join(".prifly", "state")
	}
	mark := monitorStorageMark(root)
	for _, name := range []string{"prifly.json", filepath.Join(".prifly", "installation.json"), filepath.Join(stateRoot, "state.sqlite3"), filepath.Join(stateRoot, "state.sqlite3-wal")} {
		info, err := os.Lstat(filepath.Join(root, name))
		if err != nil {
			mark += "|missing:" + name
			continue
		}
		if strings.HasSuffix(name, "-wal") && info.Size() == 0 {
			continue
		}
		mark += fmt.Sprintf("|%s:%d:%d:%v:%v", name, info.Size(), info.ModTime().UnixNano(), info.Mode(), monitorPhysical(info))
	}
	return mark
}
