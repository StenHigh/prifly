package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func monitorFixture(t *testing.T, root string) string {
	t.Helper()
	if err := prifly.Init(root); err != nil {
		t.Fatal(err)
	}
	brief := prifly.Brief{SchemaVersion: "1", ID: "test:brief/monitor", Subject: "Проверить монитор <img src=x onerror=alert(1)>", DesiredOutcome: "Read stored facts", InScope: []string{"Read local facts"}, OutOfScope: []string{}, CompletionCriteria: []string{"Read"}, SourceRefs: []prifly.ArtifactRef{}, Assumptions: []string{}, Confirmation: "explicit"}
	for path, value := range map[string]any{"workflows/monitor.json": emptyCLIWorkflow(t), "brief.json": brief} {
		data, _ := json.Marshal(value)
		if err := os.WriteFile(filepath.Join(root, path), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	e, err := prifly.Open(root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	result, err := e.Start(context.Background(), prifly.StartOptions{CommandID: "command:monitor", WorkflowFile: "workflows/monitor.json", BriefFile: "brief.json", Inputs: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	return result.Receipt.RunID
}
func TestMonitorCatalogAndScopedReads(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(root, "first")
	id := monitorFixture(t, first)
	second := filepath.Join(root, "second")
	monitorFixture(t, second)
	link := filepath.Join(root, "alias")
	if err := os.Symlink(first, link); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(root, "foreign")
	if err := prifly.Init(foreign); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(foreign, ".prifly/installation.json"))
	if err != nil {
		t.Fatal(err)
	}
	var installation prifly.Installation
	_ = json.Unmarshal(data, &installation)
	installation.OwnerUID++
	data, _ = json.Marshal(installation)
	if err = os.WriteFile(filepath.Join(foreign, ".prifly/installation.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	catalog := newMonitorCatalog(filepath.Join(root, "registry"), []string{root, filepath.Join(root, "unreadable")})
	catalog.add(first)
	catalog.add(link)
	catalog.scan(context.Background())
	catalog.refresh(context.Background())
	if len(catalog.sources) != 2 || catalog.discovery.Unreadable == 0 {
		t.Fatalf("incorrect discovery: %+v %+v", catalog.sources, catalog.discovery)
	}
	handler := monitorHost("127.0.0.1:7777")(monitorMux(catalog))
	request := func(method, path string) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, nil)
		req.Host = "127.0.0.1:7777"
		handler.ServeHTTP(rec, req)
		return rec
	}
	list := request("GET", "/api/runs")
	if list.Code != 200 || !bytes.Contains(list.Body.Bytes(), []byte(`"total":2`)) {
		t.Fatal(list.Code, list.Body.String())
	}
	src := monitorSourceID(first)
	for _, path := range []string{"/api/run?source=" + src + "&id=" + id, "/api/events?source=" + src + "&id=" + id, "/api/tasks?source=" + src + "&id=" + id} {
		if rec := request("GET", path); rec.Code != 200 {
			t.Fatal(path, rec.Code, rec.Body.String())
		}
	}
	for _, path := range []string{"/api/run?source=/etc&id=" + id, "/api/tasks?source=" + src + "&id=run:missing", "/api/events?source=" + src + "&id=" + id + "&after=oops"} {
		if rec := request("GET", path); rec.Code == 200 {
			t.Fatal("invalid scope answered", path)
		}
	}
	if rec := request("POST", "/api/runs"); rec.Code != http.StatusMethodNotAllowed {
		t.Fatal(rec.Code)
	}
	// Preview truncation must never be mistaken for the complete stored text.
	writer, err := prifly.Open(first, false)
	if err != nil {
		t.Fatal(err)
	}
	body := bytes.Repeat([]byte("x"), monitorArtifactLimit+7)
	blobPath := filepath.Join(first, "preview.txt")
	if err = os.WriteFile(blobPath, body, 0600); err != nil {
		t.Fatal(err)
	}
	artifact, err := writer.ImportArtifact(blobPath, "blob", nil, "text/plain")
	_ = writer.Close()
	if err != nil {
		t.Fatal(err)
	}
	q := url.Values{"source": {src}, "artifact_id": {artifact.ID}, "revision": {fmt.Sprint(artifact.Revision)}, "digest": {artifact.Digest}}
	for _, full := range []bool{false, true} {
		if full {
			q.Set("full", "1")
		}
		rec := request("GET", "/api/artifact?"+q.Encode())
		var preview struct {
			Content   string `json:"content"`
			Truncated bool   `json:"truncated"`
			Bytes     int    `json:"bytes"`
		}
		if err = json.Unmarshal(rec.Body.Bytes(), &preview); err != nil || rec.Code != 200 {
			t.Fatal(rec.Code, err)
		}
		want := monitorArtifactLimit
		if full {
			want = len(body)
		}
		if preview.Bytes != len(body) || len(preview.Content) != want || preview.Truncated == full {
			t.Fatal("incorrect artifact preview", full, len(preview.Content), preview.Truncated)
		}
	}
	// One broken source cannot erase the other source's history.
	if err = os.WriteFile(filepath.Join(second, "prifly.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	catalog.refresh(context.Background())
	list = request("GET", "/api/runs")
	if !strings.Contains(list.Body.String(), `"total":1`) || !strings.Contains(list.Body.String(), `"error":`) {
		t.Fatal(list.Body.String())
	}
	if err = os.RemoveAll(first); err != nil {
		t.Fatal(err)
	}
	catalog.refresh(context.Background())
	if _, ok := catalog.sources[src]; ok {
		t.Fatal("deleted source retained")
	}
}
func TestMonitorSearchBeforePagination(t *testing.T) {
	m := newMonitorCatalog("", nil)
	m.runs["test"] = map[string]monitorRun{}
	for i := 0; i < 251; i++ {
		id := fmt.Sprintf("run:%03d", i)
		m.runs["test"][id] = monitorRun{RunSummary: prifly.RunSummary{ID: id, Subject: fmt.Sprintf("task %03d", i), Status: "completed"}}
	}
	for _, tc := range []struct{ query, want string }{{"?page=5", `"run_id":"run:200"`}, {"?q=task+250", `"filtered":1`}, {"?q=task+250", `"run_id":"run:250"`}} {
		rec := httptest.NewRecorder()
		m.listHTTP(rec, httptest.NewRequest("GET", "/api/runs"+tc.query, nil))
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), tc.want) {
			t.Fatal(rec.Code, rec.Body.String())
		}
	}
	for _, q := range []string{"?page=-1", "?from=never", "?from=2026-09-10&to=2026-09-01", "?sort=unknown"} {
		rec := httptest.NewRecorder()
		m.listHTTP(rec, httptest.NewRequest("GET", "/api/runs"+q, nil))
		if rec.Code != 400 {
			t.Fatal(q, rec.Code)
		}
	}
}
func TestMonitorStartupWarningLeavesRunCreated(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	monitorFixture(t, root)
	prior := ensureRunMonitor
	t.Cleanup(func() { ensureRunMonitor = prior })
	calls := 0
	ensureRunMonitor = func(got string) error {
		calls++
		if got != root {
			t.Fatal(got)
		}
		return errors.New("port unavailable")
	}
	var out, errout bytes.Buffer
	code := execute(context.Background(), []string{"--project", root, "run", "start", "--workflow", "workflows/monitor.json", "--brief", "brief.json", "--command-id", "command:monitor-warning", "--json"}, &out, &errout)
	// The line says the monitor is optional and the Run is unaffected: a project
	// that never wanted one saw the old wording on every start and read it as a
	// fault.
	if code != 0 || calls != 1 || !strings.Contains(errout.String(), "the Run is unaffected") || !json.Valid(out.Bytes()) {
		t.Fatalf("code %d calls %d stdout %s stderr %s", code, calls, out.String(), errout.String())
	}
}

func TestMonitorRegistrationAndConcurrentStart(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	authority := filepath.Join(root, "authority")
	monitorFixture(t, authority)
	previous := monitorUserConfigDir
	t.Cleanup(func() { monitorUserConfigDir = previous })
	monitorUserConfigDir = func() (string, error) { return root, nil }
	alias := filepath.Join(root, "alias")
	if err = os.Symlink(authority, alias); err != nil {
		t.Fatal(err)
	}
	if err = registerMonitorRoot(authority); err != nil {
		t.Fatal(err)
	}
	if err = registerMonitorRoot(alias); err != nil {
		t.Fatal(err)
	}
	directory, err := monitorDirectory()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(directory, "sources"))
	if err != nil || len(entries) != 1 {
		t.Fatal(entries, err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	t.Setenv("PRIFLY_MONITOR_TEST_CONFIG", root)
	type launched struct {
		process *os.Process
		err     error
	}
	done := make(chan launched, 2)
	for i := 0; i < 2; i++ {
		go func() {
			process, err := spawnMonitor(os.Args[0], authority, address, root)
			done <- launched{process, err}
		}()
	}
	for i := 0; i < 2; i++ {
		result := <-done
		if result.process != nil {
			t.Cleanup(func() { _ = result.process.Signal(os.Interrupt); _ = result.process.Kill() })
		}
		if result.err != nil {
			t.Fatal(result.err)
		}
	}
	if !monitorReady(address) {
		t.Fatal("no monitor survived competing startup")
	}
	// An unrelated process occupying the port is not mistaken for our service.
	occupied := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"service":"other"}`) }))
	defer occupied.Close()
	process, err := spawnMonitor(os.Args[0], authority, strings.TrimPrefix(occupied.URL, "http://"), root)
	if process != nil {
		defer process.Kill()
	}
	if err == nil {
		t.Fatal("unrelated service accepted as monitor")
	}
}

// A maintenance request takes the authority exclusively, so the read-only
// refresh running beside it fails to open with storage_busy. Blanking the
// source's rows on that made every maintenance request -- a refused one
// included -- report zero Runs for up to half a second, which on a screen whose
// whole job is visibility reads as "everything was deleted". Measured: 188 dips
// in 200 refused requests before this.
func TestMonitorCatalogKeepsRunsWhileTheAuthorityIsBusy(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "busy")
	monitorFixture(t, source)
	catalog := newMonitorCatalog(filepath.Join(root, "registry"), []string{})
	catalog.add(source)
	catalog.refresh(context.Background())
	id := monitorSourceID(source)
	if len(catalog.runs[id]) != 1 {
		t.Fatalf("the fixture Run was not indexed: %+v", catalog.runs[id])
	}
	// The same opener a cleanup preview uses: exclusive, so the read-only
	// refresh beside it cannot take its shared lock.
	holder, err := prifly.OpenMonitorMaintenance(source, true)
	if err != nil {
		t.Fatal(err)
	}
	// Make the refresh read rather than skip: storage that has not moved is
	// not re-opened, and the property here is what a read that meets the
	// lock does with the rows it already holds.
	forced := catalog.sources[id]
	forced.mark = ""
	catalog.sources[id] = forced
	catalog.refresh(context.Background())
	held := len(catalog.runs[id])
	busy := catalog.sources[id].Error
	_ = holder.Close()
	if held != 1 {
		t.Fatalf("a busy authority erased %d indexed Runs; the source said %q", 1-held, busy)
	}
	if !strings.Contains(busy, "storage_busy") || catalog.sources[id].Indexed {
		t.Fatalf("the source did not report itself unread while busy: %q indexed=%v", busy, catalog.sources[id].Indexed)
	}
	// And a source that is genuinely unreadable still contributes nothing.
	if err = os.WriteFile(filepath.Join(source, "prifly.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	catalog.refresh(context.Background())
	if len(catalog.runs[id]) != 0 {
		t.Fatalf("a broken source kept %d Runs", len(catalog.runs[id]))
	}
}

// Every refresh used to be a full prifly.Open of every discovered authority,
// twice a second: with two hundred sources an idle monitor burned a whole core
// for days, and an orphaned one on a deleted authority kept re-reading two
// hundred and fifty-nine other people's authorities for three. A source whose
// state files have not moved is skipped; one that moved, or one whose last read
// failed, is read again.
func TestMonitorCatalogSkipsSourcesWhoseStorageHasNotMoved(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "quiet")
	monitorFixture(t, source)
	catalog := newMonitorCatalog(filepath.Join(root, "registry"), []string{})
	catalog.add(source)
	catalog.refresh(context.Background())
	id := monitorSourceID(source)
	first := catalog.sources[id]
	if first.mark == "" || !first.Indexed || len(catalog.runs[id]) != 1 {
		t.Fatalf("the first read did not index the source or record its mark: %+v", first)
	}
	// Nothing moved: the source keeps its rows, its status and its read time.
	catalog.sources[id] = monitorSource{ID: first.ID, Root: first.Root, Project: first.Project, Authority: first.Authority, Indexed: true, Updated: "1970-01-01T00:00:00Z", mark: first.mark, physical: first.physical}
	catalog.refresh(context.Background())
	if catalog.sources[id].Updated != "1970-01-01T00:00:00Z" {
		t.Fatalf("an unchanged source was read again: %+v", catalog.sources[id])
	}
	if len(catalog.runs[id]) != 1 {
		t.Fatalf("skipping the read dropped the rows: %+v", catalog.runs[id])
	}
	// A change to the authority moves the state files, and the next refresh
	// reads it and sees the new Run.
	writer, err := prifly.Open(source, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Start(context.Background(), prifly.StartOptions{CommandID: "command:monitor-second", WorkflowFile: "workflows/monitor.json", BriefFile: "brief.json", Inputs: map[string]string{}}); err != nil {
		t.Fatal(err)
	}
	_ = writer.Close()
	catalog.refresh(context.Background())
	if catalog.sources[id].Updated == "1970-01-01T00:00:00Z" || len(catalog.runs[id]) != 2 {
		t.Fatalf("a changed source was not read: %+v rows=%d", catalog.sources[id], len(catalog.runs[id]))
	}
	// A read that failed leaves no mark, so it is retried rather than assumed
	// unchanged: the busy authority is read again once its holder lets go.
	holder, err := prifly.OpenMonitorMaintenance(source, true)
	if err != nil {
		t.Fatal(err)
	}
	stale := monitorSource{ID: first.ID, Root: first.Root, Project: first.Project, Authority: first.Authority, Indexed: true, mark: "", physical: first.physical}
	catalog.sources[id] = stale
	catalog.refresh(context.Background())
	busy := catalog.sources[id]
	_ = holder.Close()
	if busy.Indexed || !strings.Contains(busy.Error, "storage_busy") || busy.mark != "" {
		t.Fatalf("a busy read recorded a mark or reported success: %+v", busy)
	}
	catalog.refresh(context.Background())
	if !catalog.sources[id].Indexed || catalog.sources[id].mark == "" {
		t.Fatalf("the source was not read again after its holder let go: %+v", catalog.sources[id])
	}
}

// Every start registers the authority it touches, so test fixtures under /tmp
// piled up by the hundreds: 428 entries, 218 of them already deleted, re-read
// twice a second and re-added by one step only for the next to drop them. A
// registry entry whose authority is gone is removed; one whose authority is
// merely unreadable stays visible, as before.
func TestMonitorCatalogPrunesRegistryEntriesWhoseAuthorityIsGone(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	kept := filepath.Join(root, "kept")
	gone := filepath.Join(root, "gone")
	monitorFixture(t, kept)
	monitorFixture(t, gone)
	registry := filepath.Join(root, "registry")
	for _, source := range []string{kept, gone} {
		if err := os.MkdirAll(filepath.Join(registry, "sources"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(registry, "sources", monitorSourceID(source)), []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	catalog := newMonitorCatalog(registry, []string{})
	catalog.refresh(context.Background())
	if len(catalog.sources) != 2 {
		t.Fatalf("both registered authorities were not indexed: %d", len(catalog.sources))
	}
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}
	// The registry directory did not change, so the entries are not re-read;
	// the refresh itself notices the missing authority and drops it.
	catalog.refresh(context.Background())
	if _, still := catalog.sources[monitorSourceID(gone)]; still {
		t.Fatal("a deleted authority stayed indexed")
	}
	// A registry read after the directory changed prunes the dangling entry
	// instead of re-adding it, and keeps the one whose authority exists.
	if err := os.WriteFile(filepath.Join(registry, "sources", "newcomer"), []byte(filepath.Join(root, "absent")), 0600); err != nil {
		t.Fatal(err)
	}
	catalog.refresh(context.Background())
	entries, err := os.ReadDir(filepath.Join(registry, "sources"))
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	if len(names) != 1 || names[0] != monitorSourceID(kept) {
		t.Fatalf("dangling registry entries survived the read: %v", names)
	}
	if _, indexed := catalog.sources[monitorSourceID(kept)]; !indexed {
		t.Fatal("the surviving authority was dropped along with the dangling ones")
	}
}
