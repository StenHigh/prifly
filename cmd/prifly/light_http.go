package main

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func (s *lightService) handler() http.Handler {
	mux := http.NewServeMux()
	base := s.options.BasePath
	mux.HandleFunc(base, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != base {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.Method != http.MethodHead {
			_, _ = w.Write(lightHTML)
		}
	})
	for name, asset := range map[string]struct {
		Data []byte
		Type string
	}{"light.css": {lightCSS, "text/css; charset=utf-8"}, "light.js": {lightJS, "text/javascript; charset=utf-8"}, "monitor-theme.js": {monitorThemeJS, "text/javascript; charset=utf-8"}} {
		mux.HandleFunc(base+name, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", asset.Type)
			if r.Method != http.MethodHead {
				_, _ = w.Write(asset.Data)
			}
		})
	}
	mux.HandleFunc(base+"api/health", func(w http.ResponseWriter, r *http.Request) {
		g := s.current()
		ready := g.ID != "indexing"
		lightWrite(w, r, map[string]any{"service": "prifly-light/1", "mode": "light", "ready": ready, "base_path": base, "sources": len(g.Sources)}, false)
	})
	mux.HandleFunc(base+"api/light/board", s.board)
	mux.HandleFunc(base+"api/light/task", s.task)
	mux.HandleFunc(base+"api/light/reviews", s.reviews)
	mux.HandleFunc(base+"api/light/report", s.report)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "private, no-cache")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'")
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			lightProblem(w, 405, "light_read_only", "Light accepts only GET and HEAD")
			return
		}
		if r.URL.Path == strings.TrimSuffix(base, "/") && base != "/" {
			http.Redirect(w, r, base, http.StatusTemporaryRedirect)
			return
		}
		mux.ServeHTTP(w, r)
	})
}
func (s *lightService) scoped(w http.ResponseWriter, r *http.Request, limit int) (*lightGeneration, lightSourceSnapshot, int, bool) {
	g, start, err := s.page(r, limit)
	if err != nil {
		lightProblem(w, 409, "light_page_expired", err.Error())
		return nil, lightSourceSnapshot{}, 0, false
	}
	source, ok := g.Sources[r.URL.Query().Get("source")]
	if !ok {
		lightProblem(w, 404, "light_source_missing", "unknown registered source")
		return nil, source, 0, false
	}
	if err := s.sourceAccess(r.Context(), source.Info); err != nil {
		lightProblem(w, 403, "light_access_denied", err.Error())
		return nil, source, 0, false
	}
	return g, source, start, true
}
func (s *lightService) task(w http.ResponseWriter, r *http.Request) {
	limit, err := lightLimit(r.URL.Query(), 25, 50)
	if err != nil {
		lightProblem(w, 400, "light_query_invalid", err.Error())
		return
	}
	g, source, start, ok := s.scoped(w, r, limit)
	if !ok {
		return
	}
	key := r.URL.Query().Get("key")
	for _, card := range source.View.Cards {
		if card.Key != key {
			continue
		}
		all := source.View.Runs[key]
		total := len(all)
		start = min(start, total)
		runs := slices.Clone(all[start:min(start+limit, total)])
		if runs == nil {
			runs = []prifly.LightRun{}
		}
		// Reviews have their own pagination. A task read never expands report bodies.
		for i := range runs {
			runs[i].Reviews = []prifly.LightReview{}
			if runs[i].Board != nil {
				board := *runs[i].Board
				board.Stages = slices.Clone(board.Stages[:min(16, len(board.Stages))])
				board.Reasons = board.Reasons[:min(16, len(board.Reasons))]
				for j := range board.Stages {
					board.Stages[j].Attempts = board.Stages[j].Attempts[:min(4, len(board.Stages[j].Attempts))]
				}
				runs[i].Board = &board
				runs[i].Truncated = runs[i].BranchCount > len(board.Stages) || runs[i].ReasonCount > len(board.Reasons)
			}
		}
		page := prifly.LightTaskPage{SchemaVersion: "prifly-light-task/1", FullMonitorURL: s.options.FullURL, Source: source.Info, Card: lightCompact(card), Runs: runs, Page: lightPageMeta(g, r.URL.Query(), start, limit, total, true)}
		lightWrite(w, r, page, false)
		return
	}
	lightProblem(w, 404, "light_task_missing", "task not found")
}
func (s *lightService) reviews(w http.ResponseWriter, r *http.Request) {
	limit, err := lightLimit(r.URL.Query(), 10, 20)
	if err != nil {
		lightProblem(w, 400, "light_query_invalid", err.Error())
		return
	}
	g, source, start, ok := s.scoped(w, r, limit)
	if !ok {
		return
	}
	q := r.URL.Query()
	all, found := source.View.Reviews[q.Get("run")]
	if !found {
		lightProblem(w, 404, "light_run_missing", "Run not found")
		return
	}
	rows := []prifly.LightReview{}
	for _, review := range all {
		if q.Get("invocation") != "" && q.Get("invocation") != review.InvocationID || q.Get("candidate") != "" && (review.CandidateDigest == nil || q.Get("candidate") != *review.CandidateDigest) || q.Get("round") != "" && (review.Round == nil || q.Get("round") != fmt.Sprint(*review.Round)) {
			continue
		}
		rows = append(rows, review)
	}
	total := len(rows)
	start = min(start, total)
	page := prifly.LightReviewsPage{SchemaVersion: "prifly-light-reviews/1", Source: source.Info, Reviews: rows[start:min(start+limit, total)], Page: lightPageMeta(g, q, start, limit, total, true)}
	lightWrite(w, r, page, false)
}
func (s *lightService) report(w http.ResponseWriter, r *http.Request) {
	_, source, _, ok := s.scoped(w, r, 10)
	if !ok {
		return
	}
	q := r.URL.Query()
	for _, row := range source.View.Reviews[q.Get("run")] {
		if row.ID != q.Get("review") {
			continue
		}
		var ref *prifly.ArtifactRef
		switch q.Get("kind") {
		case "report":
			ref = row.Report
		case "gate":
			ref = row.Gate
		case "candidate":
			ref = row.Candidate
		case "request":
			ref = row.Request
		default:
			lightProblem(w, 400, "light_query_invalid", "unknown report kind")
			return
		}
		if ref == nil {
			lightProblem(w, 404, "light_report_missing", "recorded artifact unavailable")
			return
		}
		e, err := s.catalog.open(source.Info.Source)
		if err != nil {
			lightProblem(w, 502, "light_report_unavailable", err.Error())
			return
		}
		defer e.Close()
		if e.Installation.ID != source.Info.AuthorityID {
			lightProblem(w, 403, "light_access_denied", "source identity changed")
			return
		}
		if err := e.MonitorAccess(r.Context()); err != nil {
			lightProblem(w, 403, "light_access_denied", err.Error())
			return
		}
		meta, body, err := e.Artifact(*ref)
		if err != nil {
			lightProblem(w, 502, "light_report_unavailable", err.Error())
			return
		}
		view := prifly.LightReportView{SchemaVersion: "prifly-light-report/1", Source: source.Info.Source, RunID: row.RunID, ReviewID: row.ID, Kind: q.Get("kind"), Ref: *ref, TotalBytes: meta.SizeBytes, Truncated: len(body) > 65536, Text: string(body[:min(len(body), 65536)])}
		lightWrite(w, r, view, false)
		return
	}
	lightProblem(w, 404, "light_review_missing", "review not found")
}
