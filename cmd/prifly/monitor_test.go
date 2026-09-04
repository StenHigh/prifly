package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

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
