package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stenhigh/prifly/internal/local"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func TestMonitorMaintenanceRefusesAndReclaims(t *testing.T) {
	ctx := context.Background()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	id := monitorFixture(t, root)
	e, err := prifly.Open(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if other, err := prifly.OpenMonitorMaintenance(root, false); err == nil {
		other.Close()
		t.Fatal("maintenance raced an open engine")
	}
	if err = e.Drive(ctx, id); err != nil {
		t.Fatal(err)
	}
	second, err := e.Start(ctx, prifly.StartOptions{CommandID: "command:second", WorkflowFile: "workflows/monitor.json", BriefFile: "brief.json", Inputs: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	// An independent import stays; an unreferenced upload is collected.
	if err = os.WriteFile(filepath.Join(root, "keep.txt"), []byte("keep imported content"), 0600); err != nil {
		t.Fatal(err)
	}
	imported, err := e.ImportArtifact("keep.txt", "blob", nil)
	if err != nil {
		t.Fatal(err)
	}
	orphan, err := e.Blobs.Put(bytes.NewReader(bytes.Repeat([]byte("unused"), 1000)), local.MaxBlobBytes)
	if err != nil {
		t.Fatal(err)
	}
	staging := filepath.Join(root, e.Config.Configuration.ArtifactRoot, ".upload-00000000000000000000000000000000")
	if err = os.WriteFile(staging, []byte("interrupted upload"), 0600); err != nil {
		t.Fatal(err)
	}
	e.Close()
	m, err := prifly.OpenMonitorMaintenance(root, false)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := m.PreviewCleanup(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Runs) != 1 || plan.Files == 0 || plan.Bytes < orphan.Size {
		t.Fatalf("bad plan: %+v", plan)
	}
	if _, err = m.Cleanup(ctx, id, "wrong digest"); err == nil {
		t.Fatal("stale preview accepted")
	}
	if _, err = m.MonitorView(ctx, id); err != nil {
		t.Fatal("refusal deleted Run", err)
	}
	result, err := m.Cleanup(ctx, id, plan.Digest)
	if err != nil || len(result.Warnings) > 0 || result.DeletedRuns != 1 {
		t.Fatal(result, err)
	}
	if _, err = m.MonitorView(ctx, id); err == nil {
		t.Fatal("Run survived deletion")
	}
	if _, err = m.MonitorSummary(ctx, second.Receipt.RunID); err != nil {
		t.Fatal("shared artifact lost", err)
	}
	if err = m.Store.Verify(ctx); err != nil {
		t.Fatal("history integrity after cleanup", err)
	}
	if _, _, err = m.Artifact(imported.Ref()); err != nil {
		t.Fatal("independent import lost", err)
	}
	if _, err = os.Stat(staging); !os.IsNotExist(err) {
		t.Fatal("interrupted upload survived", err)
	}
	if _, err = m.Blobs.Read(orphan); err == nil {
		t.Fatal("orphan blob survived")
	}
	m.Close()
	e, err = prifly.Open(root, false)
	if err != nil {
		t.Fatal("reopen after retention", err)
	}
	defer e.Close()
	found := false
	if err = e.Store.VisitRetentionRoots(ctx, nil, func(data []byte) error {
		if bytes.Contains(data, []byte("command:monitor")) {
			found = true
		}
		return nil
	}); err != nil || !found {
		t.Fatal("dedup receipt lost", err)
	}
}

func TestMonitorMaintenanceHTTPAndActiveRun(t *testing.T) {
	ctx := context.Background()
	root, _ := filepath.EvalSymlinks(t.TempDir())
	id := monitorFixture(t, root)
	catalog := newMonitorCatalog(t.TempDir(), []string{root})
	catalog.add(root)
	catalog.refresh(ctx)
	handler := monitorHost("127.0.0.1:7781")(monitorMux(catalog))
	request := func(method, path, origin, token, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Host = "127.0.0.1:7781"
		req.Header.Set("Origin", origin)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-PriFly-Maintenance", token)
		handler.ServeHTTP(rec, req)
		return rec
	}
	response := request("GET", "/api/maintenance-token", "", "", "")
	var credential struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &credential); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"source": monitorSourceID(root), "run": id, "action": "preview"})
	for _, origin := range []string{"", "http://evil.example"} {
		if response = request("POST", "/api/maintenance", origin, credential.Token, string(body)); response.Code != 403 {
			t.Fatal(response.Code)
		}
	}
	if response = request("POST", "/api/maintenance", "http://127.0.0.1:7781", "wrong", string(body)); response.Code != 403 {
		t.Fatal(response.Code)
	}
	response = request("POST", "/api/maintenance", "http://127.0.0.1:7781", credential.Token, string(body))
	if response.Code != 409 {
		t.Fatal("active Run accepted", response.Code, response.Body.String())
	}
	if response = request("GET", "/api/maintenance", "", "", string(body)); response.Code != 405 {
		t.Fatal("GET may mutate")
	}
	e, err := prifly.Open(root, true)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if _, err = e.MonitorView(ctx, id); err != nil {
		t.Fatal("active Run lost", err)
	}
	e.Close()
	writer, err := prifly.Open(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = writer.Drive(ctx, id); err != nil {
		t.Fatal(err)
	}
	writer.Close()
	response = request("POST", "/api/maintenance", "http://127.0.0.1:7781", credential.Token, string(body))
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	var plan prifly.CleanupPlan
	if err = json.Unmarshal(response.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	command, _ := json.Marshal(map[string]string{"source": monitorSourceID(root), "run": id, "action": "delete", "digest": plan.Digest})
	response = request("POST", "/api/maintenance", "http://127.0.0.1:7781", credential.Token, string(command))
	if response.Code != 200 || !bytes.Contains(response.Body.Bytes(), []byte(`"deleted_runs":1`)) {
		t.Fatal(response.Code, response.Body.String())
	}
	response = request("POST", "/api/maintenance", "http://127.0.0.1:7781", credential.Token, string(command))
	if response.Code == 200 {
		t.Fatal("repeat deleted Run again")
	}
}

func TestMonitorStorageCountsHardlinksAndSkipsSymlinks(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	monitorFixture(t, root)
	catalog := newMonitorCatalog(t.TempDir(), nil)
	catalog.add(root)
	source := catalog.sourceList()[0]
	before, _ := monitorMeasure(context.Background(), source, map[[2]uint64]bool{})
	path := filepath.Join(root, ".prifly", "disk-test")
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), 65536), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(path, path+"-hardlink"); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, bytes.Repeat([]byte("z"), 1<<20), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path+"-symlink"); err != nil {
		t.Fatal(err)
	}
	seen := map[[2]uint64]bool{}
	after, total := monitorMeasure(context.Background(), source, seen)
	if after.Bytes-before.Bytes < 65536 || after.Bytes-before.Bytes > 131072 || total != after.Bytes || after.Available == nil || len(after.Errors) > 0 {
		t.Fatalf("bad measurement: before=%+v after=%+v total=%d", before, after, total)
	}
	_, duplicate := monitorMeasure(context.Background(), source, seen)
	if duplicate != 0 {
		t.Fatal("duplicate inode counted", duplicate)
	}
}
