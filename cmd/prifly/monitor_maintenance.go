package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"syscall"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func monitorMaintenance(mux *http.ServeMux, catalog *monitorCatalog) {
	token := rand.Text()
	mux.HandleFunc("/api/maintenance-token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"token": token})
	})
	mux.HandleFunc("/api/maintenance", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", 405)
			return
		}
		if r.Header.Get("Origin") != "http://"+r.Host || r.Header.Get("Content-Type") != "application/json" || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-PriFly-Maintenance")), []byte(token)) != 1 {
			http.Error(w, "same-origin confirmation required", 403)
			return
		}
		var input struct {
			Source string `json:"source"`
			Run    string `json:"run"`
			Action string `json:"action"`
			Digest string `json:"digest"`
		}
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
		d.DisallowUnknownFields()
		if err := d.Decode(&input); err != nil {
			http.Error(w, "invalid request", 400)
			return
		}
		var extra any
		if d.Decode(&extra) != io.EOF {
			http.Error(w, "invalid request", 400)
			return
		}
		if input.Action != "preview" && input.Action != "delete" {
			http.Error(w, "invalid action", 400)
			return
		}
		catalog.mu.RLock()
		source, ok := catalog.sources[input.Source]
		catalog.mu.RUnlock()
		if !ok {
			http.Error(w, "unknown source", 404)
			return
		}
		// A replaced root is a different deletion target, even if its path matches.
		info, err := os.Stat(source.Root)
		if err == nil && source.physical != monitorPhysical(info) {
			http.Error(w, "source changed; refresh catalog", 409)
			return
		}
		var e *prifly.Engine
		if err == nil {
			e, err = prifly.OpenMonitorMaintenance(source.Root, input.Action == "preview")
		}
		w.Header().Set("Content-Type", "application/json")
		fail := func(err error) {
			problem, _ := prifly.ProblemFor(err)
			w.WriteHeader(409)
			_ = json.NewEncoder(w).Encode(problem)
		}
		if err != nil {
			fail(err)
			return
		}
		defer e.Close()
		if e.Installation.ID != source.Authority {
			http.Error(w, "authority identity changed", 409)
			return
		}
		if input.Action == "preview" {
			plan, err := e.PreviewCleanup(r.Context(), input.Run)
			if err != nil {
				fail(err)
				return
			}
			_ = json.NewEncoder(w).Encode(plan)
			return
		}
		result, err := e.Cleanup(r.Context(), input.Run, input.Digest)
		if err != nil {
			fail(err)
			return
		}
		_ = json.NewEncoder(w).Encode(result)
	})
}

func monitorPhysical(info os.FileInfo) [2]uint64 {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return [2]uint64{uint64(st.Dev), uint64(st.Ino)}
	}
	return [2]uint64{}
}
