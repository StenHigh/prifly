package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"

	"github.com/stenhigh/prifly/internal/local"
)

type CleanupPlan struct {
	Root       string            `json:"root"`
	Authority  string            `json:"authority"`
	Digest     string            `json:"digest"`
	Runs       map[string]int64  `json:"runs"`
	Protected  map[string]string `json:"protected"`
	Files      int               `json:"files"`
	Bytes      int64             `json:"bytes"`
	files      []cleanupFile
	audit      json.RawMessage
	workspaces map[string]cleanupFile
}
type cleanupFile struct {
	Directory bool
	Link      string
	Device    uint64
	Inode     uint64
	Path      string
	Size      int64
	Digest    string
}
type CleanupResult struct {
	DeletedRuns  int      `json:"deleted_runs"`
	DeletedFiles int      `json:"deleted_files"`
	DeletedBytes int64    `json:"deleted_bytes"`
	Warnings     []string `json:"warnings"`
}

func OpenMonitorMaintenance(root string, readOnly bool) (*Engine, error) {
	return openEngine(root, readOnly, true)
}

func cleanupRefusal(r Run) string {
	if !r.terminal() || r.Settled == nil || r.HasUnresolvedEffects || r.ResumeRequired {
		return "Run не завершён окончательно"
	}
	if len(r.Active) > 0 || r.ActiveCheckID != "" || r.PendingAcceptance != nil || r.PendingArtifactPublication != nil || r.PendingDecision != nil || len(r.ActionDeliveries) > 0 {
		return "Есть незавершённые операции"
	}
	for _, a := range r.Attempts {
		if a != nil && a.Settled == nil {
			return "Есть незавершённая попытка"
		}
	}
	for _, c := range r.CheckExecutions {
		if c != nil && c.Settled == nil {
			return "Есть незавершённая проверка"
		}
	}
	return ""
}

var retentionDigest = regexp.MustCompile(`sha256:[0-9a-f]{64}`)
var retentionStaging = regexp.MustCompile(`^(\.upload-[0-9a-f]{32}|\.pending-metadata:[0-9a-f]{32})$`)

// Mark exact digest strings, including structured values embedded as JSON or
// base64 JSON in a pinned document. Arbitrary text can only retain extra bytes.
func markRetention(data []byte, marked map[string]bool) {
	for _, digest := range retentionDigest.FindAll(data, -1) {
		marked[string(digest)] = true
	}
	var value any
	if json.Unmarshal(data, &value) != nil {
		return
	}
	var walk func(any, int)
	walk = func(v any, depth int) {
		if depth > 32 {
			marked["*"] = true
			return
		}
		switch x := v.(type) {
		case string:
			for _, d := range retentionDigest.FindAllString(x, -1) {
				marked[d] = true
			}
			var nested any
			if json.Unmarshal([]byte(x), &nested) == nil {
				walk(nested, depth+1)
			} else if decoded, err := base64.StdEncoding.DecodeString(x); err == nil && json.Unmarshal(decoded, &nested) == nil {
				walk(nested, depth+1)
			}
		case []any:
			for _, v := range x {
				walk(v, depth+1)
			}
		case map[string]any:
			for _, v := range x {
				walk(v, depth+1)
			}
		}
	}
	walk(value, 0)
}

func (e *Engine) PreviewCleanup(ctx context.Context, selected string) (CleanupPlan, error) {
	plan := CleanupPlan{Root: e.Root, Authority: e.Installation.ID, Runs: map[string]int64{}, Protected: map[string]string{}, files: []cleanupFile{}}
	if !e.maintenance {
		return plan, fault("maintenance_required", "exclusive authority access required")
	}
	if _, err := e.readAccess(ctx); err != nil {
		return plan, err
	}
	control, version, err := e.Control(ctx)
	if err != nil {
		return plan, err
	}
	if version > 0 && !control.allows(e.owner, "project", e.Config.ID, ControlOperationResolve) {
		return plan, fault("object_access_denied", "owner has no project resolution access")
	}
	if e.driverLive() {
		return plan, fault("storage_busy", "driver is still active")
	}
	runs := map[string]Run{}
	after := ""
	for {
		revisions, next, err := e.MonitorRevisions(ctx, after)
		if err != nil {
			return plan, err
		}
		for _, s := range revisions {
			r, _, err := e.load(ctx, s.RunID)
			if err != nil {
				return plan, err
			}
			runs[r.ID] = r
			if selected != "" && selected != r.ID {
				continue
			}
			if reason := cleanupRefusal(r); reason != "" {
				plan.Protected[r.ID] = reason
			} else {
				plan.Runs[r.ID] = s.Version
			}
		}
		if next == "" {
			break
		}
		after = next
	}
	if selected != "" {
		if _, ok := runs[selected]; !ok {
			return plan, local.ErrNotFound
		}
	}
	claims, _, err := e.readClaims(ctx)
	if err != nil {
		return plan, err
	}
	for _, c := range claims.Claims {
		if c.Released == nil {
			if _, ok := plan.Runs[c.RunID]; ok {
				delete(plan.Runs, c.RunID)
				plan.Protected[c.RunID] = "Сохраняется claim рабочего дерева"
			}
		}
	}
	// A surviving fork still names an exact source cut. Keep that source's
	// journal; bulk deletion may remove a closed fork and its source together.
	for changed := true; changed; {
		changed = false
		for id, r := range runs {
			if _, deleting := plan.Runs[id]; deleting {
				continue
			}
			if r.Fork != nil {
				if _, ok := plan.Runs[r.Fork.SourceRunID]; ok {
					delete(plan.Runs, r.Fork.SourceRunID)
					plan.Protected[r.Fork.SourceRunID] = "Источник другого сохранённого Run"
					changed = true
				}
			}
		}
	}
	if selected != "" && len(plan.Runs) == 0 {
		return plan, fault("run_not_deletable", plan.Protected[selected])
	}
	audits := map[string]any{}
	for id := range plan.Runs {
		r := runs[id]
		audits[id] = map[string]any{"decisions": r.DecisionLedger, "waivers": r.Waivers, "action_intents": r.ActionIntents, "action_admissions": r.ActionAdmissions, "action_deliveries": r.ActionDeliveries}
	}
	plan.audit, err = canonical(audits)
	if err != nil {
		return plan, err
	}
	marked := map[string]bool{}
	markRetention(plan.audit, marked)
	if err = e.Store.VisitRetentionRoots(ctx, plan.Runs, func(data []byte) error { markRetention(data, marked); return ctx.Err() }); err != nil {
		return plan, err
	}
	defs, _, err := e.Inventory()
	if err != nil {
		return plan, err
	}
	for _, d := range defs {
		markRetention(d.Bytes, marked)
		marked[d.RawDigest] = true
	}
	config, _ := canonical(e.Config)
	markRetention(config, marked)
	root, err := os.OpenRoot(e.Root)
	if err != nil {
		return plan, err
	}
	defer root.Close()
	// Non-artifact authority metadata (including packages, task intake and
	// inventory) remains a root. Never walk external project/worktree files.
	cfg := e.Config.Configuration
	err = fs.WalkDir(root.FS(), ".prifly", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if path == filepath.ToSlash(cfg.StateRoot) || path == filepath.ToSlash(cfg.ArtifactRoot) || path == filepath.ToSlash(cfg.WorkspaceRoot) || path == ".prifly/artifact-refs" {
			return fs.SkipDir
		}
		if d.Type()&os.ModeSymlink != 0 {
			return faultf("unsafe_path", "Подготовка очистки остановлена: путь метаданных %q — символическая ссылка. Уберите ссылку из метаданных authority; тестовые файлы храните за пределами .prifly. Run ещё не удалены.", path)
		}
		if !d.IsDir() && strings.HasSuffix(path, ".json") {
			data, err := readLocal(e.Root, path, MaxArtifactBytes)
			if err != nil {
				return err
			}
			markRetention(data, marked)
		}
		return nil
	})
	if err != nil {
		return plan, err
	}
	entries, err := fs.ReadDir(root.FS(), ".prifly/artifact-refs")
	if errors.Is(err, fs.ErrNotExist) {
		entries = nil
	} else if err != nil {
		return plan, err
	}
	staging := []string{}
	artifacts := map[string]Artifact{}
	metadata := map[string][]byte{}
	for _, entry := range entries {
		if retentionStaging.MatchString(entry.Name()) {
			staging = append(staging, filepath.Join(".prifly/artifact-refs", entry.Name()))
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(".prifly/artifact-refs", entry.Name())
		data, err := readLocal(e.Root, path, MaxDefinitionBytes)
		if err != nil {
			return plan, err
		}
		var a Artifact
		if err = decode(data, &a); err != nil {
			return plan, err
		}
		if artifactMetadataPath(a.ID) != path {
			return plan, local.ErrIntegrity
		}
		artifacts[path] = a
		metadata[path] = data
		if a.Producer["kind"] == "import" {
			marked[a.Digest] = true
		}
	}
	// Follow provenance and manifests transitively. An imported artifact is an
	// independent owner object, so its content stays even without a Run.
	visited := map[string]bool{}
	for changed := true; changed; {
		changed = false
		for path, a := range artifacts {
			if !marked[a.Digest] || visited[path] {
				continue
			}
			visited[path] = true
			changed = true
			markRetention(metadata[path], marked)
			_, data, err := e.Artifact(a.Ref())
			if err != nil {
				return plan, err
			}
			markRetention(data, marked)
		}
	}
	add := func(path string, limit int64) error {
		data, err := readLocal(e.Root, path, limit)
		if err != nil {
			return err
		}
		plan.files = append(plan.files, cleanupFile{Path: path, Size: int64(len(data)), Digest: rawDigest(data)})
		plan.Bytes += int64(len(data))
		return nil
	}
	if marked["*"] {
		return plan, fault("retention_reference_depth", "reference nesting is too deep for safe cleanup")
	}
	for path, a := range artifacts {
		if !marked[a.Digest] {
			if err = add(path, MaxDefinitionBytes); err != nil {
				return plan, err
			}
		}
	}
	entries, err = fs.ReadDir(root.FS(), filepath.ToSlash(cfg.ArtifactRoot))
	if err != nil {
		return plan, err
	}
	for _, entry := range entries {
		if retentionStaging.MatchString(entry.Name()) {
			staging = append(staging, filepath.Join(cfg.ArtifactRoot, entry.Name()))
			continue
		}
		if len(entry.Name()) != 64 {
			continue
		}
		if _, err := hex.DecodeString(entry.Name()); err != nil {
			continue
		}
		if marked["sha256:"+entry.Name()] {
			continue
		}
		if err = add(filepath.Join(cfg.ArtifactRoot, entry.Name()), local.MaxBlobBytes); err != nil {
			return plan, err
		}
	}
	for _, path := range staging {
		file, err := inspectCleanupFile(root, path)
		if err != nil {
			return plan, err
		}
		if file.Directory {
			return plan, local.ErrUnsafePath
		}
		plan.files = append(plan.files, file)
		plan.Bytes += file.Size
	}
	workspaces := map[string]bool{}
	plan.workspaces = map[string]cleanupFile{}
	if err = e.Store.VisitRetentionAudit(ctx, func(data []byte) error {
		var record struct {
			SchemaVersion string                 `json:"schema_version"`
			Workspaces    map[string]cleanupFile `json:"workspace_roots"`
		}
		if err := json.Unmarshal(data, &record); err != nil {
			return err
		}
		if record.SchemaVersion != "local-retention/1" {
			return local.ErrIncompatible
		}
		for path, expected := range record.Workspaces {
			if filepath.Dir(path) != cfg.WorkspaceRoot || !safeRelative(path) {
				return local.ErrUnsafePath
			}
			current, err := inspectCleanupFile(root, path)
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				return err
			}
			if current != expected {
				return local.ErrIntegrity
			}
			workspaces[filepath.Join(e.Root, path)] = true
		}
		return nil
	}); err != nil {
		return plan, err
	}
	for id := range plan.Runs {
		r := runs[id]
		for _, a := range r.Attempts {
			if a != nil && a.Workspace != "" {
				expected := filepath.Join(e.Root, cfg.WorkspaceRoot, strings.TrimPrefix(a.ID, "attempt:"))
				if a.Workspace != expected {
					return plan, local.ErrUnsafePath
				}
				workspaces[a.Workspace] = true
			}
		}
		for _, c := range r.CheckExecutions {
			if c != nil && c.Workspace != "" {
				expected := filepath.Join(e.Root, cfg.WorkspaceRoot, strings.TrimPrefix(c.ID, "check:"))
				if c.Workspace != expected {
					return plan, local.ErrUnsafePath
				}
				workspaces[c.Workspace] = true
			}
		}
	}
	for id, r := range runs {
		if _, deleting := plan.Runs[id]; deleting {
			continue
		}
		for _, a := range r.Attempts {
			if a != nil {
				delete(workspaces, a.Workspace)
			}
		}
		for _, c := range r.CheckExecutions {
			if c != nil {
				delete(workspaces, c.Workspace)
			}
		}
	}
	for path := range workspaces {
		relative, err := filepath.Rel(e.Root, path)
		if err != nil {
			return plan, err
		}
		if _, err = root.Lstat(relative); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return plan, err
		}
		record, err := inspectCleanupFile(root, relative)
		if err != nil {
			return plan, err
		}
		if !record.Directory {
			return plan, local.ErrUnsafePath
		}
		plan.workspaces[relative] = record
		err = fs.WalkDir(root.FS(), filepath.ToSlash(relative), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			file, err := inspectCleanupFile(root, path)
			if err != nil {
				return err
			}
			plan.files = append(plan.files, file)
			plan.Bytes += file.Size
			return nil
		})
		if err != nil {
			return plan, err
		}
	}
	sort.Slice(plan.files, func(i, j int) bool {
		a, b := plan.files[i], plan.files[j]
		if a.Directory != b.Directory {
			return !a.Directory
		}
		if a.Directory && len(a.Path) != len(b.Path) {
			return len(a.Path) > len(b.Path)
		}
		return a.Path < b.Path
	})
	plan.Files = len(plan.files)
	rootInfo, err := root.Stat(".")
	if err != nil {
		return plan, err
	}
	identity, ok := rootInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return plan, local.ErrUnsafePath
	}
	fingerprint, err := canonical(map[string]any{"authority": plan.Authority, "root_device": uint64(identity.Dev), "root_inode": identity.Ino, "runs": plan.Runs, "files": plan.files})
	if err != nil {
		return plan, err
	}
	plan.Digest = rawDigest(fingerprint)
	return plan, nil
}

func (e *Engine) Cleanup(ctx context.Context, selected, digest string) (CleanupResult, error) {
	result := CleanupResult{Warnings: []string{}}
	if e.ReadOnly {
		return result, local.ErrReadOnly
	}
	plan, err := e.PreviewCleanup(ctx, selected)
	if err != nil {
		return result, err
	}
	if digest == "" || digest != plan.Digest {
		return result, fault("cleanup_plan_changed", "storage changed; preview and confirm again")
	}
	// A durable audit entry and history deletion commit together. Receipts,
	// control state and tombstones are retained; this is retention, not erasure.
	audit := map[string]any{"schema_version": "local-retention/1", "runs": plan.Runs, "files": plan.Files, "bytes": plan.Bytes, "actor": e.owner, "retained_audit": plan.audit, "workspace_roots": plan.workspaces}
	payload, err := canonical(audit)
	if err != nil {
		return result, err
	}
	id := newID("retention")
	applied, err := e.Store.ApplyAuthority(ctx, local.AuthorityCommand{ID: id, Actor: e.owner, Key: "retention:r" + strings.TrimPrefix(id, "retention:")[:30], Payload: payload}, func(local.AuthoritySnapshot) (local.AuthorityChange, error) {
		return local.AuthorityChange{Data: payload, Result: payload, PruneRuns: plan.Runs}, nil
	})
	if err != nil {
		return result, err
	}
	if applied.Receipt.Rejection != nil {
		return result, applied.Receipt.Rejection
	}
	result.DeletedRuns = len(plan.Runs)
	root, err := os.OpenRoot(e.Root)
	if err != nil {
		result.Warnings = append(result.Warnings, err.Error())
		return result, nil
	}
	defer root.Close()
	// Deletion is restartable: a failure only leaves unreachable files behind.
	// Recheck exact immutable bytes before removing each confined name.
	for _, file := range plan.files {
		var err error
		if file.Directory || file.Link != "" || file.Inode != 0 {
			var current cleanupFile
			current, err = inspectCleanupFile(root, file.Path)
			if err == nil && current != file {
				err = local.ErrIntegrity
			}
		} else {
			var data []byte
			data, err = readLocal(e.Root, file.Path, local.MaxBlobBytes)
			if err == nil && rawDigest(data) != file.Digest {
				err = local.ErrIntegrity
			}
		}
		if err == nil {
			err = root.Remove(file.Path)
		}
		if err != nil {
			result.Warnings = append(result.Warnings, file.Path+": "+err.Error())
			break
		}
		result.DeletedFiles++
		result.DeletedBytes += file.Size
	}
	audit["result"] = result
	completed, encodeErr := canonical(audit)
	if encodeErr == nil {
		_, encodeErr = e.Store.ApplyAuthority(ctx, local.AuthorityCommand{ID: newID("retention"), Actor: e.owner, Key: "retention:r" + strings.TrimPrefix(id, "retention:")[:30], Payload: completed}, func(local.AuthoritySnapshot) (local.AuthorityChange, error) {
			return local.AuthorityChange{Data: completed, Result: completed}, nil
		})
	}
	if encodeErr != nil {
		result.Warnings = append(result.Warnings, "GC audit result: "+encodeErr.Error())
	}
	if err = e.Store.Compact(ctx); err != nil {
		result.Warnings = append(result.Warnings, "SQLite: "+err.Error())
	}
	return result, nil
}

// Workspaces may contain large process logs or symlinks. Hash regular files
// without buffering them and unlink a symlink itself, never its target.
func inspectCleanupFile(root *os.Root, path string) (cleanupFile, error) {
	file := cleanupFile{Path: path}
	info, err := root.Lstat(path)
	if err != nil {
		return file, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return file, local.ErrUnsafePath
	}
	file.Device = uint64(st.Dev)
	file.Inode = uint64(st.Ino)
	if info.IsDir() {
		file.Directory = true
		return file, nil
	}
	if info.Mode()&os.ModeSymlink != 0 {
		file.Link, err = root.Readlink(path)
		return file, err
	}
	if !info.Mode().IsRegular() {
		return file, local.ErrUnsafePath
	}
	opened, err := root.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return file, err
	}
	defer opened.Close()
	actual, err := opened.Stat()
	if err != nil {
		return file, err
	}
	if !os.SameFile(info, actual) {
		return file, local.ErrUnsafePath
	}
	hash := sha256.New()
	file.Size, err = io.Copy(hash, opened)
	file.Digest = fmt.Sprintf("sha256:%x", hash.Sum(nil))
	return file, err
}
