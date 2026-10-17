package main

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

//go:embed light.html
var lightHTML []byte

//go:embed light.css
var lightCSS []byte

//go:embed light.js
var lightJS []byte

type lightOptions struct {
	FullURL    string
	BasePath   string
	Projection *prifly.LightProjection
}

func parseLightOptions(full, base, mapping string) (lightOptions, error) {
	result := lightOptions{FullURL: full, BasePath: base}
	u, err := url.Parse(full)
	if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return result, usageError("full-monitor-url requires an absolute HTTP(S) URL without credentials or fragment")
	}
	if base == "" || !strings.HasPrefix(base, "/") || strings.ContainsAny(base, "?#%\\\x00\r\n") || strings.HasPrefix(base, "//") || path.Clean(base) != strings.TrimSuffix(base, "/") && base != "/" {
		return result, usageError("base-path requires a clean absolute URL path")
	}
	result.BasePath = strings.TrimSuffix(base, "/") + "/"
	if mapping != "" {
		f, err := os.Open(mapping)
		if err != nil {
			return result, err
		}
		defer f.Close()
		body, err := io.ReadAll(io.LimitReader(f, 256<<10+1))
		if err != nil {
			return result, err
		}
		result.Projection, err = prifly.ParseLightProjection(body)
		if err != nil {
			return result, err
		}
	}
	return result, nil
}
func (c *cli) light(ctx context.Context, root, addr, scan string, options lightOptions) error {
	registry, err := monitorDirectory()
	if err != nil {
		return err
	}
	roots := []string{}
	if scan != "" {
		roots = strings.Split(scan, string(os.PathListSeparator))
	}
	catalog := newMonitorCatalog(registry, roots)
	catalog.add(root)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	board := newLightService(catalog, options)
	board.start(ctx)
	server := &http.Server{Handler: monitorHost(listener.Addr().String())(board.handler()), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		closeCtx, stop := context.WithTimeout(context.Background(), 2*time.Second)
		defer stop()
		_ = server.Shutdown(closeCtx)
	}()
	fmt.Fprintf(c.errout, "monitor: http://%s%s (Pri-Fly Light, read-only)\n", listener.Addr(), options.BasePath)
	err = server.Serve(listener)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}
func lightProblem(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	problem, _ := prifly.ProblemFor(&prifly.Fault{Code: code, Message: message})
	_ = json.NewEncoder(w).Encode(problem)
}
func lightWrite(w http.ResponseWriter, r *http.Request, value any, conditional bool) {
	data, err := json.Marshal(value)
	if err != nil {
		lightProblem(w, 500, "light_encoding_failed", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if conditional {
		sum := sha256.Sum256(data)
		tag := fmt.Sprintf("\"%x\"", sum)
		w.Header().Set("ETag", tag)
		if r.Header.Get("If-None-Match") == tag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	if r.Method != http.MethodHead {
		_, _ = w.Write(data)
	}
}
func lightLimit(q url.Values, defaultLimit, max int) (int, error) {
	if q.Get("limit") == "" {
		return defaultLimit, nil
	}
	n, err := strconv.Atoi(q.Get("limit"))
	if err != nil || n < 1 || n > max {
		return 0, fmt.Errorf("limit must be between 1 and %d", max)
	}
	return n, nil
}
