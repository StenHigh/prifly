package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func lightHTTPFixture(t *testing.T) (*lightService, string) {
	t.Helper()
	root := t.TempDir()
	monitorFixture(t, root)
	e, err := prifly.Open(root, false)
	if err != nil {
		t.Fatal(err)
	}
	request := queueCLIRequest(t, root, "Очередь <script>bad()</script>")
	body, err := os.ReadFile(request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.QueueAdd(context.Background(), prifly.QueueAddRequest{CommandID: "command:light-queued", RequestBytes: body}); err != nil {
		t.Fatal(err)
	}
	_ = e.Close()
	catalog := newMonitorCatalog(filepath.Join(t.TempDir(), "registry"), nil)
	catalog.add(root)
	service := newLightService(catalog, lightOptions{FullURL: "https://full.example/prefix/?view=related", BasePath: "/light/"})
	service.refresh(context.Background())
	return service, monitorSourceID(root)
}
func lightGet(t *testing.T, s *lightService, path string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	s.handler().ServeHTTP(response, httptest.NewRequest("GET", path, nil))
	return response
}
func lightValidate(t *testing.T, name string, body []byte) {
	t.Helper()
	schema, err := prifly.PublicSchema(name)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := flow.Digest(schema)
	if err != nil {
		t.Fatal(err)
	}
	ref := flow.Ref{ID: "test:schema/" + name, Version: "1.0.0", Digest: digest}
	if err := flow.ValidateSchema(flow.Registry{ref: schema}, ref, body); err != nil {
		t.Fatalf("%s: %v\n%s", name, err, body)
	}
}
func TestLightSeparateReadOnlyUIAndPrefix(t *testing.T) {
	t.Parallel()
	s, source := lightHTTPFixture(t)
	for _, asset := range []string{"/light/", "/light/light.css", "/light/light.js", "/light/monitor-theme.js", "/light/api/health"} {
		response := lightGet(t, s, asset)
		if response.Code != 200 || response.Body.Len() == 0 {
			t.Fatalf("%s: %d %s", asset, response.Code, response.Body.String())
		}
	}
	html := lightGet(t, s, "/light/").Body.String()
	if html == string(monitorPage) || !strings.Contains(html, "board-screen") || strings.Contains(html, "/monitor.js") {
		t.Fatal("Light is the full monitor")
	}
	for _, path := range []string{"/", "/api/runs", "/light/api/runs", "/light/api/maintenance/preview", "/light/api/maintenance/delete"} {
		if response := lightGet(t, s, path); response.Code != 404 {
			t.Fatalf("full route leaked: %s %d", path, response.Code)
		}
	}
	for _, method := range []string{"POST", "DELETE", "PUT", "PATCH", "OPTIONS"} {
		rec := httptest.NewRecorder()
		s.handler().ServeHTTP(rec, httptest.NewRequest(method, "/light/api/light/board", nil))
		if rec.Code != 405 {
			t.Fatalf("write method: %s %d", method, rec.Code)
		}
	}
	response := lightGet(t, s, "/light/api/light/board")
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	lightValidate(t, "LightBoard", response.Body.Bytes())
	var page prifly.LightBoardPage
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Cards) != 2 || page.Page.Total == nil || *page.Page.Total != 2 {
		t.Fatalf("queued and standalone missing: %+v", page)
	}
	request := httptest.NewRequest("GET", "/light/api/light/board", nil)
	request.Header.Set("If-None-Match", response.Header().Get("ETag"))
	rec := httptest.NewRecorder()
	s.handler().ServeHTTP(rec, request)
	if rec.Code != 304 || rec.Body.Len() != 0 {
		t.Fatalf("conditional poll: %d", rec.Code)
	}
	for _, card := range page.Cards {
		q := url.Values{"source": {source}, "key": {card.Key}}
		task := lightGet(t, s, "/light/api/light/task?"+q.Encode())
		if task.Code != 200 {
			t.Fatal(task.Body.String())
		}
		lightValidate(t, "LightTask", task.Body.Bytes())
	}
	rec = httptest.NewRecorder()
	request = httptest.NewRequest("GET", "http://attacker.example/light/", nil)
	monitorHost("127.0.0.1:7777")(s.handler()).ServeHTTP(rec, request)
	if rec.Code != http.StatusMisdirectedRequest {
		t.Fatal("foreign Host accepted")
	}
	// The original listener keeps its exact frontend and health identity.
	rec = httptest.NewRecorder()
	monitorMux(s.catalog).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Body.String() != string(monitorPage) {
		t.Fatal("full frontend changed")
	}
}
func TestLightPagesKeepGenerationAndQueryAcrossThousandCards(t *testing.T) {
	t.Parallel()
	s, id := lightHTTPFixture(t)
	s.mu.Lock()
	source := s.latest[id]
	source.View.Cards = []prifly.LightCard{}
	for i := 0; i < 1001; i++ {
		source.View.Cards = append(source.View.Cards, prifly.LightCard{Key: fmt.Sprintf("task:%04d", i), Kind: "queue_task", Title: fmt.Sprintf("task %04d", i), Lane: "queue", ProjectID: source.Info.ProjectID, AuthorityID: source.Info.AuthorityID, Priority: int64(i % 3), Ordinal: int64(i), CurrentRuns: []prifly.LightRun{}, ActiveBranches: []prifly.MonitorStage{}, Reviews: []prifly.LightReviewChip{}, Reasons: []prifly.MonitorReason{}})
	}
	source.Info.Cut++
	s.latest[id] = source
	s.publishLocked()
	s.mu.Unlock()
	seen := map[string]bool{}
	cursor := ""
	var next string
	for {
		q := url.Values{"limit": {"50"}}
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		rec := lightGet(t, s, "/light/api/light/board?"+q.Encode())
		if rec.Code != 200 {
			t.Fatal(rec.Body.String())
		}
		var page prifly.LightBoardPage
		json.Unmarshal(rec.Body.Bytes(), &page)
		if page.Page.Total == nil || *page.Page.Total != 1001 {
			t.Fatal("silent truncation")
		}
		for _, c := range page.Cards {
			if seen[c.Key] {
				t.Fatal("duplicate card")
			}
			seen[c.Key] = true
		}
		if page.Page.NextCursor == nil {
			break
		}
		cursor = *page.Page.NextCursor
		next = cursor
	}
	if len(seen) != 1001 {
		t.Fatal("lost cards")
	}
	if rec := lightGet(t, s, "/light/api/light/board?limit=50&q=changed&cursor="+url.QueryEscape(next)); rec.Code != 409 {
		t.Fatal("cursor accepted changed query")
	}
	rec := lightGet(t, s, "/light/api/light/board?q=task+1000")
	var page prifly.LightBoardPage
	json.Unmarshal(rec.Body.Bytes(), &page)
	if len(page.Cards) != 1 {
		t.Fatal("search beyond 200 failed")
	}
}
func TestLightOptionsRejectInvalidConfiguration(t *testing.T) {
	t.Parallel()
	for _, test := range [][3]string{{"javascript:alert(1)", "/", ""}, {"https://u:p@example.com/", "/", ""}, {"https://example.com/#run", "/", ""}, {"https://example.com/", "/../light/", ""}, {"https://example.com/", "//light/", ""}, {"https://example.com/", "/light%2f/", ""}} {
		if _, err := parseLightOptions(test[0], test[1], test[2]); err == nil {
			t.Fatalf("accepted invalid config: %v", test)
		}
	}
	if _, err := parseLightOptions("https://example.com/full/?q=1", "/light/", ""); err != nil {
		t.Fatal(err)
	}
}

func TestLightHealthCannotSatisfyFullMonitorProbe(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lightWrite(w, r, map[string]any{"service": "prifly-light/1", "mode": "light"}, false)
	}))
	defer server.Close()
	if monitorReady(strings.TrimPrefix(server.URL, "http://")) {
		t.Fatal("Light health satisfied full monitor probe")
	}
}

func TestLightStorageMarkTracksConfiguredState(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	state := filepath.Join("custom", "authority")
	if err := os.MkdirAll(filepath.Join(root, state), 0700); err != nil {
		t.Fatal(err)
	}
	before := lightStorageMark(root, state)
	if err := os.WriteFile(filepath.Join(root, state, "state.sqlite3-wal"), []byte("new authority cut"), 0600); err != nil {
		t.Fatal(err)
	}
	if lightStorageMark(root, state) == before {
		t.Fatal("custom state change missed")
	}
}
