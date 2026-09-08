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
	"path/filepath"
	"strconv"
	"strings"
	"time"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

//go:embed monitor.html
var monitorPage []byte

//go:embed monitor.css
var monitorCSS []byte

//go:embed monitor.js
var monitorJS []byte

// The monitor is a window, not a second control plane. It serves what the read
// model already returns and offers no command: a page cannot start, stop or
// approve anything, so nothing here needs to answer "who did that".
//
// It listens on the loopback interface only. Any address reachable from another
// machine would publish sealed plans, skill bytes and results to that machine,
// and this build has no way to say who is asking.
func (c *cli) monitor(ctx context.Context, root string, args []string) error {
	f := flags("monitor")
	scanRoot := f.String("scan-root", "", "search roots separated by the OS path-list separator; default: all accessible local directories")
	addr := f.String("addr", "127.0.0.1:7777", "loopback address to listen on")
	if err := parse(f, args); err != nil {
		return err
	}
	if !loopbackOnly(*addr) {
		return usageError("monitor listens on a loopback address only: what it serves is not for another machine")
	}
	registry, err := monitorDirectory()
	if err != nil {
		return err
	}
	roots := []string{}
	if *scanRoot != "" {
		roots = append(roots, strings.Split(*scanRoot, string(os.PathListSeparator))...)
	} else {
		home, _ := os.UserHomeDir()
		roots = []string{filepath.Join(filepath.Dir(registry), "projects"), home, os.TempDir(), "/"}
	}
	catalog := newMonitorCatalog(registry, roots)
	catalog.add(root)
	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	defer listener.Close()
	catalog.start(ctx)
	server := &http.Server{Handler: monitorHost(listener.Addr().String())(monitorMux(catalog)), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	fmt.Fprintf(c.errout, "monitor: http://%s (local monitor)\n", listener.Addr())
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func monitorMux(catalog *monitorCatalog) http.Handler {
	mux := http.NewServeMux()
	monitorMaintenance(mux, catalog)
	write := func(w http.ResponseWriter, value any, err error) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if err != nil {
			// The monitor answers in the same envelope every other reader gets,
			// so a refusal here reads as the refusal it is rather than as a
			// bare sentence assembled for this page alone.
			problem, _ := prifly.ProblemFor(err)
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(problem)
			return
		}
		_ = json.NewEncoder(w).Encode(value)
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = w.Write(monitorPage)
	})
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		write(w, map[string]any{"service": "prifly-local-monitor/1", "uid": os.Geteuid()}, nil)
	})
	mux.HandleFunc("/monitor.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		_, _ = w.Write(monitorCSS)
	})
	mux.HandleFunc("/monitor.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = w.Write(monitorJS)
	})
	mux.HandleFunc("/api/runs", catalog.listHTTP)
	mux.HandleFunc("/api/storage", catalog.storageHTTP)
	scoped := func(path string, handler func(http.ResponseWriter, *http.Request, *prifly.Engine)) {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			engine, err := catalog.open(r.URL.Query().Get("source"))
			if err != nil {
				write(w, nil, err)
				return
			}
			defer engine.Close()
			if err := engine.MonitorAccess(r.Context()); err != nil {
				write(w, nil, err)
				return
			}
			handler(w, r, engine)
		})
	}
	scoped("/api/capacity", func(w http.ResponseWriter, r *http.Request, engine *prifly.Engine) {
		capacity, held, err := engine.AdmissionCapacity(r.Context())
		if err != nil {
			write(w, nil, err)
			return
		}
		queue, err := engine.AdmissionQueue(r.Context())
		write(w, map[string]any{"capacity": capacity, "held": held, "waiting": queue}, err)
	})
	scoped("/api/run", func(w http.ResponseWriter, r *http.Request, engine *prifly.Engine) {
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}
		view, err := engine.MonitorView(r.Context(), id)
		write(w, view, err)
	})
	scoped("/api/events", func(w http.ResponseWriter, r *http.Request, engine *prifly.Engine) {
		id := r.URL.Query().Get("id")
		after := int64(0)
		if value := r.URL.Query().Get("after"); value != "" {
			var err error
			after, err = strconv.ParseInt(value, 10, 64)
			if err != nil || after < 0 {
				http.Error(w, "invalid event cursor", 400)
				return
			}
		}
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
	scoped("/api/timing", func(w http.ResponseWriter, r *http.Request, engine *prifly.Engine) {
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}
		view, err := engine.MonitorView(r.Context(), id)
		if err != nil {
			write(w, nil, err)
			return
		}
		write(w, map[string]any{"timing": view.Timing, "read_version": view.SchemaVersion}, nil)
	})
	// Debugging needs the recorded object itself, not a summary of it. This
	// returns the Run as it is stored, so a node can be expanded to exactly
	// what the authority holds about it.
	scoped("/api/state", func(w http.ResponseWriter, r *http.Request, engine *prifly.Engine) {
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}
		view, err := engine.MonitorView(r.Context(), id)
		write(w, view, err)
	})
	// Debugging a step means reading what it was actually given and what it
	// actually returned, not a summary of either. The bytes are already sealed
	// and addressed, so this hands back exactly the recorded artifact.
	scoped("/api/artifact", func(w http.ResponseWriter, r *http.Request, engine *prifly.Engine) {
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
		body, truncated := string(data), false
		if len(data) > monitorArtifactLimit && r.URL.Query().Get("full") != "1" {
			body, truncated = string(data[:monitorArtifactLimit]), true
		}
		write(w, map[string]any{"artifact": artifact, "bytes": len(data), "truncated": truncated, "content": body}, nil)
	})
	scoped("/api/tasks", func(w http.ResponseWriter, r *http.Request, engine *prifly.Engine) {
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}
		tasks, err := engine.SessionTasks(r.Context(), id)
		if err != nil {
			// A failed read must remain visible, even when no handoff is expected.
			write(w, nil, err)
			return
		}
		write(w, map[string]any{"tasks": tasks}, nil)
	})
	scoped("/api/definitions", func(w http.ResponseWriter, r *http.Request, engine *prifly.Engine) {
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

	return mux
}

// monitorArtifactLimit bounds the preview; full text is available on explicit request.
const monitorArtifactLimit = 1 << 20

// The monitor serves a local page over loopback, and a browser will send this
// server whatever Host a page asks it to. Answering only for the address this
// process is listening on keeps another page on the machine, or a name that
// resolves to loopback, from reading an authority through the browser.
func monitorHost(listen string) func(http.Handler) http.Handler {
	host, port, err := net.SplitHostPort(strings.TrimSpace(listen))
	allowed := map[string]bool{}
	if err == nil {
		allowed[net.JoinHostPort(host, port)] = true
		for _, name := range []string{"127.0.0.1", "[::1]", "localhost"} {
			allowed[name+":"+port] = true
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !allowed[r.Host] {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Content-Type-Options", "nosniff")
				w.WriteHeader(http.StatusMisdirectedRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "this monitor answers only for the address it listens on"})
				return
			}
			if r.Method != http.MethodGet && r.Method != http.MethodHead && !(r.Method == http.MethodPost && r.URL.Path == "/api/maintenance") {
				w.Header().Set("Allow", "GET, HEAD")
				http.Error(w, "read-only monitor", http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self'; frame-ancestors 'none'; base-uri 'none'")
			next.ServeHTTP(w, r)
		})
	}
}

func loopbackOnly(addr string) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
