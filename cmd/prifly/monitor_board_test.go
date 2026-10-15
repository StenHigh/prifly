package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"testing"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func TestMonitorBoardPagesAllRunsAndKeepsSourceScopedProvenance(t *testing.T) {
	t.Parallel()
	m := newMonitorCatalog("", nil)
	m.runs["a"] = map[string]monitorRun{}
	for i := 0; i < 251; i++ {
		id := fmt.Sprintf("run:%03d", i)
		m.runs["a"][id] = monitorRun{Source: "a", RunSummary: prifly.RunSummary{ID: id, Subject: fmt.Sprintf("task %03d", i), Status: "completed"}}
	}
	child := m.runs["a"]["run:250"]
	child.ForkSourceRunID = "run:200"
	m.runs["a"][child.ID] = child
	m.runs["b"] = map[string]monitorRun{"run:250": {Source: "b", RunSummary: prifly.RunSummary{ID: "run:250", Subject: "same title", ForkSourceRunID: "run:200"}}}
	for _, query := range []string{"view=board&page=5", "view=board&q=task+200"} {
		rec := httptest.NewRecorder()
		m.listHTTP(rec, httptest.NewRequest("GET", "/api/runs?"+query, nil))
		var page struct {
			Runs     []monitorBoardRun `json:"runs"`
			Total    int               `json:"total"`
			Filtered int               `json:"filtered"`
		}
		if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &page) != nil || page.Total != 252 {
			t.Fatalf("board catalog: %d %s", rec.Code, rec.Body.String())
		}
		found := false
		for _, run := range page.Runs {
			if run.Source == "a" && run.ID == "run:200" {
				found = true
				if len(run.RelatedRuns) != 1 || run.RelatedRuns[0].ID != "run:250" {
					t.Fatalf("page-local or cross-source links: %+v", run)
				}
			}
		}
		if !found {
			t.Fatalf("Run beyond metadata limit missing: %s", rec.Body.String())
		}
	}
}

func TestMonitorBoardRefreshesAuthorityFactsAtTheSameRunRevision(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	id := monitorFixture(t, root)
	catalog := newMonitorCatalog(filepath.Join(t.TempDir(), "registry"), nil)
	catalog.add(root)
	catalog.refresh(context.Background())
	source := monitorSourceID(root)
	cached := catalog.runs[source][id]
	version, sequence := cached.Version, cached.Events
	cached.Board = &prifly.MonitorBoard{Category: "wrong_cached_category"}
	catalog.runs[source][id] = cached
	writer, err := prifly.Open(root, false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = writer.SetAdmissionCapacity(context.Background(), prifly.CapacityRequest{CommandID: "command:board-capacity", Capacity: 2, Reason: "authority-only change"})
	_ = writer.Close()
	if err != nil {
		t.Fatal(err)
	}
	catalog.refresh(context.Background())
	fresh := catalog.runs[source][id]
	if fresh.Version != version || fresh.Events != sequence || fresh.Board == nil || fresh.Board.Category == "wrong_cached_category" {
		t.Fatalf("authority facts kept stale: %+v", fresh)
	}
}
