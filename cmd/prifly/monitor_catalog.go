package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

type monitorSource struct {
	physical  [2]uint64
	ID        string `json:"id"`
	Root      string `json:"root"`
	Project   string `json:"project"`
	Authority string `json:"authority"`
	Error     string `json:"error,omitempty"`
	Conflict  bool   `json:"conflict"`
	Indexed   bool   `json:"indexed"`
	Updated   string `json:"updated,omitempty"`
}
type monitorRun struct {
	prifly.RunSummary
	Source  string `json:"source"`
	Project string `json:"project"`
	Root    string `json:"root"`
	Error   string `json:"error,omitempty"`
}
type monitorDiscovery struct {
	Scanning    bool     `json:"scanning"`
	Roots       []string `json:"roots"`
	Directories int      `json:"directories"`
	Unreadable  int      `json:"unreadable"`
	Excluded    int      `json:"excluded"`
	Errors      []string `json:"errors"`
	Completed   string   `json:"completed,omitempty"`
}
type monitorCatalog struct {
	storage   monitorStorage
	mu        sync.RWMutex
	sources   map[string]monitorSource
	runs      map[string]map[string]monitorRun
	discovery monitorDiscovery
	registry  string
}

var monitorUserConfigDir = os.UserConfigDir

func monitorDirectory() (string, error) {
	base, err := monitorUserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "Pri-Fly", "monitor"), nil
}
func monitorSourceID(root string) string {
	sum := sha256.Sum256([]byte(root))
	return hex.EncodeToString(sum[:])
}
func registerMonitorRoot(root string) error {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return err
	}
	dir, err := monitorDirectory()
	if err != nil {
		return err
	}
	dir = filepath.Join(dir, "sources")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".register-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.WriteString(root); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(dir, monitorSourceID(root)))
}
func newMonitorCatalog(registry string, roots []string) *monitorCatalog {
	return &monitorCatalog{sources: map[string]monitorSource{}, runs: map[string]map[string]monitorRun{}, registry: registry,
		discovery: monitorDiscovery{Roots: roots, Errors: []string{}}}
}
func (m *monitorCatalog) add(root string) {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return
	}
	id := monitorSourceID(root)
	m.mu.RLock()
	_, exists := m.sources[id]
	m.mu.RUnlock()
	if exists {
		return
	}
	path := filepath.Join(root, ".prifly", "installation.json")
	info, err := os.Lstat(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			m.scanError(path, err)
		}
		return
	}
	if !info.Mode().IsRegular() {
		m.scanError(path, errors.New("installation is not a regular file"))
		return
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(owner.Uid) != os.Geteuid() {
		return
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		m.scanError(path, err)
		return
	}
	defer f.Close()
	actual, err := f.Stat()
	if err != nil || !actual.Mode().IsRegular() || !os.SameFile(info, actual) {
		return
	}
	var installation prifly.Installation
	if err = json.NewDecoder(io.LimitReader(f, 65536)).Decode(&installation); err != nil {
		m.scanError(path, err)
		return
	}
	if installation.OwnerUID != os.Geteuid() {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var physical [2]uint64
	if info, err := os.Stat(root); err == nil {
		if st, ok := info.Sys().(*syscall.Stat_t); ok {
			physical = [2]uint64{uint64(st.Dev), uint64(st.Ino)}
		}
	}
	for _, source := range m.sources {
		if physical != ([2]uint64{}) && source.physical == physical {
			return
		}
	}
	m.sources[id] = monitorSource{ID: id, Root: root, Authority: installation.ID, physical: physical}
	m.runs[id] = map[string]monitorRun{}
}
func (m *monitorCatalog) registered() {
	entries, err := os.ReadDir(filepath.Join(m.registry, "sources"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		m.scanError(m.registry, err)
	}
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		path := filepath.Join(m.registry, "sources", entry.Name())
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() > 32768 {
			continue
		}
		data, err := os.ReadFile(path)
		if err == nil {
			m.add(string(data))
			// A previously registered root remains visible when access is lost.
			root := string(data)
			if _, statErr := os.Stat(filepath.Join(root, ".prifly", "installation.json")); statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
				id := monitorSourceID(root)
				m.mu.Lock()
				if _, exists := m.sources[id]; !exists {
					m.sources[id] = monitorSource{ID: id, Root: root, Error: statErr.Error()}
					m.runs[id] = map[string]monitorRun{}
				}
				m.mu.Unlock()
			}
		}
	}
}
func (m *monitorCatalog) scanError(path string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.discovery.Unreadable++
	// ponytail: keep 30 example paths, counters retain the full coverage deficit.
	if len(m.discovery.Errors) < 30 {
		m.discovery.Errors = append(m.discovery.Errors, fmt.Sprintf("%s: %v", path, err))
	}
}
func (m *monitorCatalog) scan(ctx context.Context) {
	m.mu.Lock()
	roots := m.discovery.Roots
	m.discovery = monitorDiscovery{Scanning: true, Roots: roots, Errors: []string{}}
	m.mu.Unlock()
	seen := map[[2]uint64]bool{}
	localDevices := map[uint64]bool{}
	for _, root := range roots {
		root, err := filepath.EvalSymlinks(root)
		if err != nil {
			m.scanError(root, err)
			continue
		}
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err != nil {
				m.scanError(path, err)
				return nil
			}
			if !d.IsDir() {
				return nil
			}
			// Kernel and network mounts are outside the local Run population.
			if path == "/proc" || path == "/sys" || path == "/dev" || path == "/net" {
				m.mu.Lock()
				m.discovery.Excluded++
				m.mu.Unlock()
				return filepath.SkipDir
			}
			info, err := d.Info()
			if err != nil {
				m.scanError(path, err)
				return filepath.SkipDir
			}
			if st, ok := info.Sys().(*syscall.Stat_t); ok {
				device := uint64(st.Dev)
				isLocal, known := localDevices[device]
				if !known {
					var err error
					isLocal, err = monitorLocalFilesystem(path)
					if err != nil {
						m.scanError(path, err)
						return filepath.SkipDir
					}
					localDevices[device] = isLocal
				}
				if !isLocal {
					m.mu.Lock()
					m.discovery.Excluded++
					m.mu.Unlock()
					return filepath.SkipDir
				}
				key := [2]uint64{uint64(st.Dev), uint64(st.Ino)}
				if seen[key] {
					return filepath.SkipDir
				}
				seen[key] = true
			}
			m.mu.Lock()
			m.discovery.Directories++
			m.mu.Unlock()
			if d.Name() == ".prifly" {
				m.add(filepath.Dir(path))
			}
			return nil
		})
		if ctx.Err() != nil {
			break
		}
	}
	m.mu.Lock()
	m.discovery.Scanning = false
	m.discovery.Completed = time.Now().UTC().Format(time.RFC3339)
	m.mu.Unlock()
}
func (m *monitorCatalog) start(ctx context.Context) {
	m.registered()
	go func() {
		for {
			m.measureStorage(ctx)
			select {
			case <-ctx.Done():
				return
			case <-time.After(30 * time.Second):
			}
		}
	}()
	go func() {
		for {
			m.scan(ctx)
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Minute):
			}
		}
	}()
	go func() {
		for {
			m.refresh(ctx)
			select {
			case <-ctx.Done():
				return
			case <-time.After(500 * time.Millisecond):
			}
		}
	}()
}
func (m *monitorCatalog) sourceList() []monitorSource {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := make([]monitorSource, 0, len(m.sources))
	counts := map[string]int{}
	for _, s := range m.sources {
		if s.Authority != "" {
			counts[s.Authority]++
		}
	}
	for _, s := range m.sources {
		s.Conflict = s.Authority != "" && counts[s.Authority] > 1
		list = append(list, s)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Root < list[j].Root })
	return list
}
func (m *monitorCatalog) refresh(ctx context.Context) {
	m.registered()
	for _, s := range m.sourceList() {
		if ctx.Err() != nil {
			return
		}
		// A missing source stops contributing immediately; registration may find it again.
		if _, err := os.Stat(filepath.Join(s.Root, ".prifly", "installation.json")); errors.Is(err, fs.ErrNotExist) {
			m.mu.Lock()
			delete(m.sources, s.ID)
			delete(m.runs, s.ID)
			m.mu.Unlock()
			continue
		}
		next := map[string]monitorRun{}
		engine, err := prifly.Open(s.Root, true)
		if err == nil {
			s.Project = engine.Config.ID
			if info, statErr := os.Stat(engine.Root); statErr == nil {
				s.physical = monitorPhysical(info)
			}
			if s.Authority != engine.Installation.ID {
				m.mu.Lock()
				m.runs[s.ID] = map[string]monitorRun{}
				m.mu.Unlock()
			}
			s.Authority = engine.Installation.ID
			readCtx := ctx
			after := ""
			for {
				revisions, cursor, readErr := engine.MonitorRevisions(readCtx, after)
				if readErr != nil {
					err = readErr
					break
				}
				for _, revision := range revisions {
					m.mu.RLock()
					cached := m.runs[s.ID][revision.RunID]
					m.mu.RUnlock()
					if cached.Version == revision.Version && cached.Events == revision.EventSeq && cached.Error == "" {
						next[revision.RunID] = cached
						continue
					}
					summary, readErr := engine.MonitorSummary(readCtx, revision.RunID)
					row := monitorRun{RunSummary: summary, Source: s.ID, Root: s.Root, Project: s.Project}
					if readErr != nil {
						row.ID = revision.RunID
						row.Error = readErr.Error()
					}
					next[revision.RunID] = row
				}
				if cursor == "" {
					break
				}
				after = cursor
			}
			_ = engine.Close()
		}
		s.Error = ""
		s.Indexed = err == nil
		if err != nil {
			s.Error = err.Error()
		} else {
			s.Updated = time.Now().UTC().Format(time.RFC3339)
		}
		m.mu.Lock()
		m.sources[s.ID] = s
		if err == nil {
			m.runs[s.ID] = next
		} else {
			m.runs[s.ID] = map[string]monitorRun{}
		}
		m.mu.Unlock()
	}
}
func (m *monitorCatalog) open(id string) (*prifly.Engine, error) {
	m.mu.RLock()
	source, ok := m.sources[id]
	m.mu.RUnlock()
	if !ok {
		return nil, errors.New("unknown monitor source")
	}
	return prifly.Open(source.Root, true)
}
