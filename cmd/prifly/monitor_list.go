package main

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (m *monitorCatalog) listHTTP(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page := 1
	if v := q.Get("page"); v != "" {
		var err error
		page, err = strconv.Atoi(v)
		if err != nil || page < 1 || page > 100000000 {
			http.Error(w, "invalid page", 400)
			return
		}
	}
	from, to := q.Get("from"), q.Get("to")
	for _, v := range []string{from, to} {
		if v != "" {
			if _, err := time.Parse("2006-01-02", v); err != nil {
				http.Error(w, "invalid date", 400)
				return
			}
		}
	}
	if from != "" && to != "" && from > to {
		http.Error(w, "invalid date range", 400)
		return
	}
	sortBy := q.Get("sort")
	if sortBy != "" && sortBy != "created" && sortBy != "oldest" && sortBy != "status" {
		http.Error(w, "invalid sort", 400)
		return
	}
	search := strings.ToLower(q.Get("q"))
	rows := []monitorRun{}
	statuses, projects, executors := map[string]bool{}, map[string]bool{}, map[string]bool{}
	m.mu.RLock()
	discovery := m.discovery
	discovery.Errors = append([]string{}, discovery.Errors...)
	total, active := 0, 0
	for _, runs := range m.runs {
		for _, run := range runs {
			total++
			if run.Active > 0 {
				active++
			}
			statuses[run.Status] = true
			projects[run.Project] = true
			for _, v := range run.Executors {
				executors[v] = true
			}
			if q.Get("project") != "" && q.Get("project") != run.Project {
				continue
			}
			if q.Get("status") != "" && q.Get("status") != run.Status {
				continue
			}
			if executor := q.Get("executor"); executor != "" {
				found := false
				for _, v := range run.Executors {
					if v == executor {
						found = true
					}
				}
				if !found {
					continue
				}
			}
			day := run.Created
			if len(day) > 10 {
				day = day[:10]
			}
			if from != "" && day < from || to != "" && day > to {
				continue
			}
			if !strings.Contains(strings.ToLower(strings.Join([]string{run.ID, run.Subject, run.WorkflowID, run.Project, run.Root, strings.Join(run.Executors, " ")}, " ")), search) {
				continue
			}
			rows = append(rows, run)
		}
	}
	m.mu.RUnlock()
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		av, bv := a.LastObserved, b.LastObserved
		if sortBy == "created" || sortBy == "oldest" {
			av, bv = a.Created, b.Created
		}
		if sortBy == "status" {
			av, bv = a.Status, b.Status
		}
		if sortBy == "status" {
			if av == bv {
				return a.Source+a.ID < b.Source+b.ID
			}
			return av < bv
		}
		at, _ := time.Parse(time.RFC3339Nano, av)
		bt, _ := time.Parse(time.RFC3339Nano, bv)
		if at.Equal(bt) {
			return a.Source+a.ID < b.Source+b.ID
		}
		if sortBy == "oldest" {
			return at.Before(bt)
		}
		return at.After(bt)
	})
	filtered := len(rows)
	pages := (filtered + 49) / 50
	if pages < 1 {
		pages = 1
	}
	if page > pages {
		page = pages
	}
	start := (page - 1) * 50
	end := min(start+50, len(rows))
	rows = rows[start:end]
	keys := func(values map[string]bool) []string {
		result := []string{}
		for v := range values {
			if v != "" {
				result = append(result, v)
			}
		}
		sort.Strings(result)
		return result
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"runs": rows, "page": page, "pages": pages, "filtered": filtered, "total": total, "active": active, "sources": m.sourceList(), "discovery": discovery, "filters": map[string]any{"statuses": keys(statuses), "projects": keys(projects), "executors": keys(executors)}})
}
