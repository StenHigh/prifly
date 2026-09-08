package main

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

type monitorStorageSource struct {
	Measured  bool     `json:"measured"`
	ID        string   `json:"id"`
	Root      string   `json:"root"`
	Bytes     int64    `json:"bytes"`
	Artifacts int64    `json:"artifacts"`
	Available *uint64  `json:"available"`
	Errors    []string `json:"errors"`
}
type monitorStorage struct {
	Sources  []monitorStorageSource `json:"sources"`
	Bytes    int64                  `json:"bytes"`
	Measured string                 `json:"measured"`
	Scanning bool                   `json:"scanning"`
}

// Allocated blocks approximate disk pressure. APFS clones and snapshots can
// share blocks across different inodes, so this is not an erasure estimate.
func monitorMeasure(ctx context.Context, source monitorSource, seen map[[2]uint64]bool) (monitorStorageSource, int64) {
	out := monitorStorageSource{ID: source.ID, Root: source.Root, Errors: []string{}}
	fail := func(err error) {
		if len(out.Errors) < 10 {
			out.Errors = append(out.Errors, err.Error())
		}
	}
	e, err := prifly.Open(source.Root, true)
	if err != nil {
		fail(err)
		return out, 0
	}
	defer e.Close()
	if err = e.MonitorAccess(ctx); err != nil {
		fail(err)
		return out, 0
	}
	root, err := os.OpenRoot(e.Root)
	if err != nil {
		fail(err)
		return out, 0
	}
	defer root.Close()
	var disk syscall.Statfs_t
	if err = syscall.Statfs(e.Root, &disk); err == nil {
		n := uint64(disk.Bavail) * uint64(disk.Bsize)
		out.Available = &n
	} else {
		fail(err)
	}
	config := e.Config.Configuration
	paths := []string{".prifly", "prifly.json", config.StateRoot, config.ArtifactRoot, config.WorkspaceRoot, config.RegistryFile}
	out.Measured = true
	localSeen := map[[2]uint64]bool{}
	var total int64
	for _, path := range paths {
		err = fs.WalkDir(root.FS(), filepath.ToSlash(path), func(name string, d fs.DirEntry, err error) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err != nil {
				fail(err)
				return nil
			}
			if d.Type()&os.ModeSymlink != 0 {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				fail(err)
				return nil
			}
			if !info.IsDir() && !info.Mode().IsRegular() {
				return nil
			}
			st, ok := info.Sys().(*syscall.Stat_t)
			if !ok {
				return nil
			}
			key := [2]uint64{uint64(st.Dev), uint64(st.Ino)}
			if localSeen[key] {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			localSeen[key] = true
			size := st.Blocks * 512
			out.Bytes += size
			artifactPath := filepath.ToSlash(config.ArtifactRoot)
			if name == artifactPath || strings.HasPrefix(name, artifactPath+"/") || name == ".prifly/artifact-refs" || strings.HasPrefix(name, ".prifly/artifact-refs/") {
				out.Artifacts += size
			}
			if !seen[key] {
				seen[key] = true
				total += size
			}
			return nil
		})
		if err != nil {
			fail(err)
			break
		}
	}
	return out, total
}
func (m *monitorCatalog) measureStorage(ctx context.Context) {
	m.mu.Lock()
	m.storage.Scanning = true
	m.mu.Unlock()
	next := monitorStorage{Sources: []monitorStorageSource{}}
	seen := map[[2]uint64]bool{}
	for _, source := range m.sourceList() {
		if ctx.Err() != nil {
			return
		}
		measured, size := monitorMeasure(ctx, source, seen)
		next.Sources = append(next.Sources, measured)
		next.Bytes += size
	}
	next.Measured = time.Now().UTC().Format(time.RFC3339)
	m.mu.Lock()
	m.storage = next
	m.mu.Unlock()
}
func (m *monitorCatalog) storageHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.RLock()
	value := m.storage
	m.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
