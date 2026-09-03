package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

//go:embed monitor.html
var monitorPage []byte

// The monitor is a window, not a second control plane. It serves what the read
// model already returns and offers no command: a page cannot start, stop or
// approve anything, so nothing here needs to answer "who did that".
//
// It listens on the loopback interface only. Any address reachable from another
// machine would publish sealed plans, skill bytes and results to that machine,
// and this build has no way to say who is asking.
func (c *cli) monitor(ctx context.Context, root string, args []string) error {
	f := flags("monitor")
	addr := f.String("addr", "127.0.0.1:7777", "loopback address to listen on")
	if err := parse(f, args); err != nil {
		return err
	}
	if !loopbackOnly(*addr) {
		return usageError("monitor listens on a loopback address only: what it serves is not for another machine")
	}
	engine, err := prifly.Open(root, true)
	if err != nil {
		return err
	}
	defer engine.Close()

	mux := http.NewServeMux()
	write := func(w http.ResponseWriter, value any, err error) {
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(value)
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(monitorPage)
	})
	mux.HandleFunc("/api/runs", func(w http.ResponseWriter, r *http.Request) {
		runs, err := engine.Runs(r.Context())
		write(w, map[string]any{"runs": runs}, err)
	})
	mux.HandleFunc("/api/capacity", func(w http.ResponseWriter, r *http.Request) {
		capacity, held, err := engine.AdmissionCapacity(r.Context())
		if err != nil {
			write(w, nil, err)
			return
		}
		queue, err := engine.AdmissionQueue(r.Context())
		write(w, map[string]any{"capacity": capacity, "held": held, "waiting": queue}, err)
	})
	mux.HandleFunc("/api/run", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}
		view, err := engine.View(r.Context(), id)
		write(w, view, err)
	})
	mux.HandleFunc("/api/events", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
		if id == "" {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}
		view, err := engine.Events(r.Context(), id, after, 200)
		if err != nil {
			write(w, nil, err)
			return
		}
		events := make([]map[string]any, 0, len(view.Events))
		for _, event := range view.Events {
			events = append(events, map[string]any{
				"seq": event.Seq, "type": event.Type, "actor": event.Actor,
				"run_version": event.RunVersion, "data": json.RawMessage(event.Data),
			})
		}
		write(w, map[string]any{"events": events, "more": view.More}, nil)
	})
	// The timing tree is the structural spine: it already carries every stage,
	// step and attempt with its durations and, importantly, the quality of each
	// duration. A monitor that showed 0 where the answer is "unknown" would be
	// telling a story the journal does not.
	mux.HandleFunc("/api/timing", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}
		view, err := engine.View(r.Context(), id)
		if err != nil {
			write(w, nil, err)
			return
		}
		write(w, map[string]any{"timing": view.Timing, "read_version": view.SchemaVersion}, nil)
	})
	// Debugging needs the recorded object itself, not a summary of it. This
	// returns the Run as it is stored, so a node can be expanded to exactly
	// what the authority holds about it.
	mux.HandleFunc("/api/state", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}
		view, err := engine.View(r.Context(), id)
		write(w, view, err)
	})
	// Debugging a step means reading what it was actually given and what it
	// actually returned, not a summary of either. The bytes are already sealed
	// and addressed, so this hands back exactly the recorded artifact.
	mux.HandleFunc("/api/artifact", func(w http.ResponseWriter, r *http.Request) {
		revision, _ := strconv.ParseInt(r.URL.Query().Get("revision"), 10, 64)
		ref := prifly.ArtifactRef{ArtifactID: r.URL.Query().Get("artifact_id"), Revision: revision, Digest: r.URL.Query().Get("digest")}
		if ref.ArtifactID == "" || ref.Digest == "" {
			http.Error(w, "artifact_id, revision and digest required", http.StatusBadRequest)
			return
		}
		artifact, data, err := engine.Artifact(ref)
		if err != nil {
			write(w, nil, err)
			return
		}
		// A monitor page is not the place to move a large blob through; the
		// truncation is reported rather than silently applied.
		const limit = 1 << 20
		body, truncated := string(data), false
		if len(data) > limit {
			body, truncated = string(data[:limit]), true
		}
		write(w, map[string]any{"artifact": artifact, "bytes": len(data), "truncated": truncated, "content": body}, nil)
	})
	mux.HandleFunc("/api/tasks", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}
		tasks, err := engine.SessionTasks(r.Context(), id)
		if err != nil {
			// A run holding no handoff is the ordinary case, not a failure.
			write(w, map[string]any{"tasks": []any{}}, nil)
			return
		}
		write(w, map[string]any{"tasks": tasks}, nil)
	})
	mux.HandleFunc("/api/definitions", func(w http.ResponseWriter, r *http.Request) {
		defs, _, err := engine.Inventory()
		if err != nil {
			write(w, nil, err)
			return
		}
		listing := make([]map[string]any, 0, len(defs))
		for _, d := range defs {
			listing = append(listing, map[string]any{"id": d.Ref.ID, "version": d.Ref.Version, "digest": d.Ref.Digest, "kind": d.Kind})
		}
		write(w, map[string]any{"definitions": listing}, nil)
	})

	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	fmt.Fprintf(os.Stderr, "monitor: read-only view on http://%s (ctrl-c to stop)\n", listener.Addr())
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func loopbackOnly(addr string) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
