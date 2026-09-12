package main

import (
	"bytes"
	"context"
	"github.com/stenhigh/prifly/assets"
	prifly "github.com/stenhigh/prifly/internal/runtime"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMonitorServesDesignAssets(t *testing.T) {
	handler := monitorHost("127.0.0.1:7777")(monitorMux(newMonitorCatalog(t.TempDir(), nil)))
	for _, asset := range []struct {
		path, contentType string
		body              []byte
	}{
		{"/monitor-theme.js", "text/javascript; charset=utf-8", monitorThemeJS},
		{"/monitor-logo.jpg", "image/jpeg", assets.MonitorLogo},
		{"/monitor-hero.jpg", "image/jpeg", assets.MonitorHero},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", "http://127.0.0.1:7777"+asset.path, nil))
		if response.Code != http.StatusOK || response.Header().Get("Content-Type") != asset.contentType || len(asset.body) == 0 || !bytes.Equal(response.Body.Bytes(), asset.body) {
			t.Fatalf("design asset unavailable or changed: %s (%d)", asset.path, response.Code)
		}
	}
	if theme, css := bytes.Index(monitorPage, []byte("/monitor-theme.js")), bytes.Index(monitorPage, []byte("/monitor.css")); theme < 0 || css < theme {
		t.Fatal("theme must load before the stylesheet")
	}
	for _, required := range []string{`id="theme"`, `src="/monitor-logo.jpg"`, `<h1 id="runs-title">Pri-Fly`, `url('/monitor-hero.jpg')`} {
		if !bytes.Contains(monitorPage, []byte(required)) && !bytes.Contains(monitorCSS, []byte(required)) {
			t.Fatalf("design binding missing: %s", required)
		}
	}
}

// The monitor serves sealed plans, skill bytes and results. This build cannot
// say who is asking from another machine, so an address reachable from one is
// refused rather than served with a warning.
func TestMonitorListensOnLoopbackOnly(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:7777", "[::1]:7777", "127.0.0.5:1"} {
		if !loopbackOnly(addr) {
			t.Fatalf("a loopback address was refused: %s", addr)
		}
	}
	for _, addr := range []string{"0.0.0.0:7777", "192.168.1.10:7777", "[::]:7777", "example.test:7777", "7777", ""} {
		if loopbackOnly(addr) {
			t.Fatalf("an address reachable from another machine was accepted: %q", addr)
		}
	}
}

// A browser sends whatever Host the page it loaded asks for. Another page on
// the same machine, or a name that happens to resolve to loopback, must not be
// able to read an authority through this server.
func TestMonitorAnswersOnlyForItsOwnAddress(t *testing.T) {
	served := monitorHost("127.0.0.1:7777")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for _, host := range []string{"127.0.0.1:7777", "localhost:7777", "[::1]:7777"} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/api/runs", nil)
		request.Host = host
		served.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s was refused: %d", host, recorder.Code)
		}
	}
	for _, host := range []string{"prifly.test:7777", "127.0.0.1:9999", "attacker.example", ""} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/api/runs", nil)
		request.Host = host
		served.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusMisdirectedRequest {
			t.Fatalf("%q was answered with %d", host, recorder.Code)
		}
		if recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("%q was refused without the content-type guard", host)
		}
	}
}

func TestMonitorAnswersAlternateLoopbackAddress(t *testing.T) {
	handler := monitorHost("127.0.0.5:7777")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	response := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://127.0.0.5:7777/", nil)
	handler.ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatal(response.Code)
	}
}

// Every start spawns a monitor for its authority, detached, when none is
// listening. One spawned for a test fixture under /tmp outlived the fixture by
// three days, reading every other authority on the machine at 90% CPU, because
// nothing knew it existed. A monitor started for one authority stops when that
// authority is gone; one given scan roots was asked to browse and stays.
func TestMonitorForOneAuthorityStopsWhenThatAuthorityIsGone(t *testing.T) {
	previous := monitorAuthorityCheckInterval
	monitorAuthorityCheckInterval = 50 * time.Millisecond
	t.Cleanup(func() { monitorAuthorityCheckInterval = previous })
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), ".config"))

	run := func(t *testing.T, scanRoot bool) (string, <-chan error) {
		t.Helper()
		root := filepath.Join(t.TempDir(), "authority")
		if err := prifly.Init(root); err != nil {
			t.Fatal(err)
		}
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		addr := listener.Addr().String()
		_ = listener.Close()
		args := []string{"--addr", addr}
		if scanRoot {
			args = append(args, "--scan-root", filepath.Dir(root))
		}
		done := make(chan error, 1)
		var errout bytes.Buffer
		c := &cli{errout: &errout}
		go func() { done <- c.monitor(context.Background(), root, args) }()
		time.Sleep(200 * time.Millisecond)
		if err := os.RemoveAll(root); err != nil {
			t.Fatal(err)
		}
		return root, done
	}

	_, bound := run(t, false)
	select {
	case err := <-bound:
		if err != nil {
			t.Fatalf("the project-bound monitor stopped with an error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("a monitor whose authority was deleted kept running")
	}

	_, browsing := run(t, true)
	select {
	case err := <-browsing:
		t.Fatalf("a monitor given scan roots stopped when one authority vanished: %v", err)
	case <-time.After(500 * time.Millisecond):
	}
}
