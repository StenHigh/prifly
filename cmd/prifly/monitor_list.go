package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type monitorRelatedRun struct {
	monitorRun
	Children         int               `json:"children"`
	Descendants      int               `json:"descendants"`
	Matches          int               `json:"matches"`
	Context          bool              `json:"context"`
	MissingParent    bool              `json:"missing_parent"`
	LatestDescendant *monitorBranchTip `json:"latest_descendant,omitempty"`
}

type monitorBranchTip struct {
	ID           string  `json:"run_id"`
	Status       string  `json:"status"`
	Outcome      *string `json:"outcome"`
	LastObserved string  `json:"last_observed"`
}

func monitorRunKey(run monitorRun) string { return run.Source + "\x00" + run.ID }

// relatedRuns works from the complete catalog, then exposes one bounded level.
// A matching descendant keeps its ancestor path even when the parent fails a filter.
func relatedRuns(all map[string]monitorRun, matched []monitorRun, parent string, source string, newestFirst bool) ([]monitorRelatedRun, int, bool) {
	parents := map[string]string{}
	children := map[string][]string{}
	for key, run := range all {
		if run.ForkSourceRunID == "" {
			continue
		}
		candidate := run.Source + "\x00" + run.ForkSourceRunID
		if _, ok := all[candidate]; !ok || candidate == key {
			continue
		}
		// Malformed provenance must not turn catalog traversal into a cycle.
		seen := map[string]bool{key: true}
		for at := candidate; at != ""; at = parents[at] {
			if seen[at] {
				candidate = ""
				break
			}
			seen[at] = true
		}
		if candidate != "" {
			parents[key] = candidate
		}
	}
	for key, parentKey := range parents {
		children[parentKey] = append(children[parentKey], key)
	}
	match := map[string]bool{}
	include := map[string]bool{}
	counts := map[string]int{}
	for _, run := range matched {
		key := monitorRunKey(run)
		match[key] = true
		for at := key; at != "" && !include[at]; at = parents[at] {
			include[at] = true
		}
		for at := key; at != ""; at = parents[at] {
			counts[at]++
		}
	}
	latestDescendant := map[string]string{}
	for key, run := range all {
		for at := parents[key]; at != ""; at = parents[at] {
			previous := latestDescendant[at]
			if previous == "" || run.LastObserved > all[previous].LastObserved || run.LastObserved == all[previous].LastObserved && key > previous {
				latestDescendant[at] = key
			}
		}
	}
	descendants := map[string]int{}
	for key := range include {
		for at := parents[key]; at != ""; at = parents[at] {
			descendants[at]++
		}
	}
	root := ""
	if parent != "" {
		root = source + "\x00" + parent
		if _, ok := all[root]; !ok {
			return nil, len(matched), false
		}
	}
	keys := []string{}
	if root != "" {
		for _, key := range children[root] {
			if include[key] {
				keys = append(keys, key)
			}
		}
	} else {
		for key := range include {
			if parents[key] == "" {
				keys = append(keys, key)
			}
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := all[keys[i]], all[keys[j]]
		av, bv := a.Created, b.Created
		if av == bv {
			return keys[i] < keys[j]
		}
		if newestFirst {
			return av > bv
		}
		return av < bv
	})
	result := make([]monitorRelatedRun, 0, len(keys))
	for _, key := range keys {
		run := all[key]
		visibleChildren := 0
		for _, child := range children[key] {
			if include[child] {
				visibleChildren++
			}
		}
		row := monitorRelatedRun{monitorRun: run, Children: visibleChildren, Descendants: descendants[key], Matches: counts[key], Context: !match[key], MissingParent: run.ForkSourceRunID != "" && parents[key] == ""}
		if descendant := latestDescendant[key]; descendant != "" {
			last := all[descendant]
			row.LatestDescendant = &monitorBranchTip{ID: last.ID, Status: last.Status, Outcome: last.Outcome, LastObserved: last.LastObserved}
		}
		result = append(result, row)
	}
	return result, len(matched), true
}

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
	view := q.Get("view")
	if view != "" && view != "related" && view != "related-newest" && view != "flat" {
		http.Error(w, "invalid view", 400)
		return
	}
	isRelated := view == "related" || view == "related-newest"
	if q.Get("parent") != "" && (!isRelated || q.Get("source") == "") {
		http.Error(w, "invalid parent", 400)
		return
	}
	search := strings.ToLower(q.Get("q"))
	rows := []monitorRun{}
	all := map[string]monitorRun{}
	statuses, projects, executors := map[string]bool{}, map[string]bool{}, map[string]bool{}
	m.mu.RLock()
	discovery := m.discovery
	discovery.Errors = append([]string{}, discovery.Errors...)
	total, active := 0, 0
	for _, runs := range m.runs {
		for _, run := range runs {
			all[monitorRunKey(run)] = run
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
	filtered := len(rows)
	var related []monitorRelatedRun
	if isRelated {
		var ok bool
		related, filtered, ok = relatedRuns(all, rows, q.Get("parent"), q.Get("source"), view == "related-newest")
		if !ok {
			if os.Getenv("LOG_LEVEL") == "debug" {
				log.Printf("[FIX:run-lineage] unknown parent source=%q run=%q", q.Get("source"), q.Get("parent"))
			}
			http.Error(w, "unknown parent", 404)
			return
		}
		if os.Getenv("LOG_LEVEL") == "debug" {
			log.Printf("[FIX:run-lineage] source=%q parent=%q matches=%d visible=%d page=%d", q.Get("source"), q.Get("parent"), filtered, len(related), page)
		}
	}
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
	pageLength := len(rows)
	if isRelated {
		pageLength = len(related)
	}
	pages := (pageLength + 49) / 50
	if pages < 1 {
		pages = 1
	}
	if page > pages {
		page = pages
	}
	start := (page - 1) * 50
	end := min(start+50, pageLength)
	if isRelated {
		related = related[start:end]
	} else {
		rows = rows[start:end]
	}
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
	var result any = rows
	if isRelated {
		result = related
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"runs": result, "page": page, "pages": pages, "filtered": filtered, "total": total, "active": active, "sources": m.sourceList(), "discovery": discovery, "filters": map[string]any{"statuses": keys(statuses), "projects": keys(projects), "executors": keys(executors)}})
}
