package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"slices"
	"strings"
	"sync"

	"github.com/stenhigh/prifly/examples"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

const runnerManualReceipt = "references/manual-version.json"

type runnerManualVersion struct {
	SchemaVersion string            `json:"schema_version"`
	BinaryVersion string            `json:"binary_version"`
	SourceRef     string            `json:"source_ref"`
	Development   bool              `json:"development"`
	Files         map[string]string `json:"files"`
}

var runnerMarkdownLink = regexp.MustCompile(`\]\(([^\s)]+)\)`)
var runnerEditorSchema = regexp.MustCompile(`(?m)(\$schema=)([^\s]+)`)
var runnerStableVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

func runnerSourceRef() (string, bool) {
	if runnerStableVersion.MatchString(prifly.Version) {
		return "v" + prifly.Version, false
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" && setting.Value != "" {
				return setting.Value, true
			}
		}
	}
	// Builds with -buildvcs=false have no evidence for a source commit. Do not
	// manufacture a tag or silently link their handbook to moving main.
	return "unavailable", true
}

func runnerAssetPath(name string) string {
	if strings.HasPrefix(name, "operations/") {
		return "references/manual/" + strings.TrimPrefix(name, "operations/")
	}
	return "references/examples/" + name
}

func runnerDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// These bytes depend only on this executable's embedded files and build info.
// Cache decoding/link rewriting; callers get their own maps and byte slices.
var loadRunnerAssets = sync.OnceValues(buildProjectRunnerAssets)

func projectRunnerAssets() (map[string][]byte, error) {
	assets, err := loadRunnerAssets()
	if err != nil {
		return nil, err
	}
	result := make(map[string][]byte, len(assets))
	for name, data := range assets {
		result[name] = bytes.Clone(data)
	}
	return result, nil
}

func buildProjectRunnerAssets() (map[string][]byte, error) {
	raw := map[string][]byte{}
	err := fs.WalkDir(examples.Handbook, ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		data, err := examples.Handbook.ReadFile(name)
		if err != nil {
			return err
		}
		raw[name] = data
		return nil
	})
	if err != nil {
		return nil, err
	}
	ref, development := runnerSourceRef()
	result := map[string][]byte{}
	for name, data := range raw {
		destination := runnerAssetPath(name)
		resolve := func(target string) string {
			if strings.HasPrefix(target, "#") || strings.Contains(target, ":") || strings.HasPrefix(target, "//") {
				return target
			}
			file, fragment, _ := strings.Cut(target, "#")
			canonical := path.Clean(path.Join("examples", path.Dir(name), file))
			relative := func(to string) string {
				p, e := filepath.Rel(filepath.FromSlash(path.Dir(destination)), filepath.FromSlash(to))
				if e != nil {
					return target
				}
				return filepath.ToSlash(p)
			}
			if strings.HasPrefix(canonical, "examples/") {
				other := strings.TrimPrefix(canonical, "examples/")
				if _, ok := raw[other]; ok {
					value := relative(runnerAssetPath(other))
					if fragment != "" {
						value += "#" + fragment
					}
					return value
				}
			}
			if ref == "unavailable" {
				return relative("references/manual/source-unavailable.md")
			}
			value := "https://github.com/StenHigh/prifly/blob/" + ref + "/" + canonical
			if fragment != "" {
				value += "#" + fragment
			}
			return value
		}
		if strings.HasSuffix(name, ".md") {
			data = runnerMarkdownLink.ReplaceAllFunc(data, func(match []byte) []byte {
				text := string(match)
				return []byte("](" + resolve(text[2:len(text)-1]) + ")")
			})
		}
		if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
			data = runnerEditorSchema.ReplaceAllFunc(data, func(match []byte) []byte {
				parts := strings.SplitN(string(match), "=", 2)
				if ref == "unavailable" {
					return []byte("schema ref unavailable in this development build; use prifly schema for the installed contract")
				}
				return []byte(parts[0] + "=" + resolve(parts[1]))
			})
		}
		result[destination] = data
	}
	version := runnerManualVersion{SchemaVersion: "prifly-runner-manual/1", BinaryVersion: prifly.Version, SourceRef: ref, Development: development, Files: map[string]string{}}
	for name, data := range result {
		version.Files[name] = runnerDigest(data)
	}
	receipt, err := json.MarshalIndent(version, "", "  ")
	if err != nil {
		return nil, err
	}
	result[runnerManualReceipt] = append(receipt, '\n')
	return result, nil
}

func runnerManagedPath(name string) bool {
	return name == path.Clean(name) && !strings.HasPrefix(name, "/") && !strings.Contains(name, "\\") &&
		(strings.HasPrefix(name, "references/manual/") || strings.HasPrefix(name, "references/examples/"))
}

// Read only regular files through real parent directories. A symlink in a
// managed prefix is a conflict, not permission to write outside the skill.
func runnerReadFile(root, name string) ([]byte, error) {
	info, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("runner root is not a real directory: %s", root)
	}
	current := root
	for _, part := range strings.Split(name, "/") {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("managed runner path is a symlink: %s", name)
		}
	}
	info, err = os.Stat(current)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("managed runner path is not a regular file: %s", name)
	}
	return os.ReadFile(current)
}

func inspectRunnerAssets(root string, expected map[string][]byte) (bool, *runnerManualVersion, error) {
	var previous *runnerManualVersion
	data, err := runnerReadFile(root, runnerManualReceipt)
	if err == nil {
		var value runnerManualVersion
		if len(data) > 2<<20 || json.Unmarshal(data, &value) != nil || value.SchemaVersion != "prifly-runner-manual/1" || len(value.Files) > 2048 {
			return false, nil, refusal("project_runner_conflict", "invalid "+runnerManualReceipt)
		}
		for name, digest := range value.Files {
			if !runnerManagedPath(name) || len(digest) != 71 || !strings.HasPrefix(digest, "sha256:") {
				return false, nil, refusal("project_runner_conflict", "invalid manual ownership entry "+name)
			}
			old, readErr := runnerReadFile(root, name)
			if errors.Is(readErr, fs.ErrNotExist) {
				continue
			}
			if readErr != nil {
				return false, nil, refusal("project_runner_conflict", readErr.Error())
			}
			if runnerDigest(old) != digest {
				return false, nil, refusal("project_runner_conflict", "locally modified managed reference "+name+"; preserve the edit before updating")
			}
		}
		previous = &value
	} else if !errors.Is(err, fs.ErrNotExist) {
		return false, nil, refusal("project_runner_conflict", err.Error())
	}
	changed := previous == nil
	for name, want := range expected {
		got, readErr := runnerReadFile(root, name)
		if errors.Is(readErr, fs.ErrNotExist) {
			changed = true
			continue
		}
		if readErr != nil {
			return false, nil, refusal("project_runner_conflict", readErr.Error())
		}
		if string(got) == string(want) {
			continue
		}
		if name != runnerManualReceipt {
			if previous == nil {
				return false, nil, refusal("project_runner_conflict", "unmanaged file occupies supplied reference "+name)
			}
			if _, owned := previous.Files[name]; !owned {
				return false, nil, refusal("project_runner_conflict", "unmanaged file occupies supplied reference "+name)
			}
		}
		changed = true
	}
	if previous != nil {
		for name := range previous.Files {
			if _, ok := expected[name]; !ok {
				changed = true
			}
		}
	}
	return changed, previous, nil
}

func lockProjectRunners(root string) (func(), error) {
	lock := filepath.Join(root, ".prifly-runner-update.lock")
	if err := os.Mkdir(lock, 0700); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return nil, refusal("project_runner_conflict", "another runner update or an interrupted update holds "+lock+"; inspect it before retrying")
		}
		return nil, refusal("project_runner_conflict", "cannot create runner update lock: "+err.Error())
	}
	return func() { _ = os.Remove(lock) }, nil
}

func copyRunnerTree(from, to string) error {
	return filepath.WalkDir(from, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(from, name)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		destination := filepath.Join(to, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(name)
			if err != nil {
				return err
			}
			return os.Symlink(target, destination)
		}
		if entry.IsDir() {
			return os.Mkdir(destination, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return refusal("project_runner_conflict", "unsupported custom runner file "+relative)
		}
		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, data, info.Mode().Perm())
	})
}

// Stage a complete skill tree, preserving author additions, before replacing
// the visible root. A failed rename restores the previous tree.
func installRunnerBundle(root string, host projectHost, assets map[string][]byte, previous *runnerManualVersion) error {
	parent := filepath.Dir(root)
	stage, err := os.MkdirTemp(parent, ".prifly-runner-stage-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := copyRunnerTree(root, stage); err != nil {
		return err
	}
	if previous != nil {
		for name := range previous.Files {
			if _, keep := assets[name]; !keep {
				if err := os.Remove(filepath.Join(stage, filepath.FromSlash(name))); err != nil && !errors.Is(err, fs.ErrNotExist) {
					return err
				}
			}
		}
	}
	names := make([]string, 0, len(assets))
	for name := range assets {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		target := filepath.Join(stage, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(target, assets[name], 0644); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(stage, "SKILL.md"), []byte(projectRunnerSkill(host)), 0644); err != nil {
		return err
	}
	backup, err := os.MkdirTemp(parent, ".prifly-runner-backup-")
	if err != nil {
		return err
	}
	if err := os.Remove(backup); err != nil {
		return err
	}
	if err := os.Rename(root, backup); err != nil {
		return err
	}
	if err := os.Rename(stage, root); err != nil {
		if restoreErr := os.Rename(backup, root); restoreErr != nil {
			return fmt.Errorf("runner replacement failed: %w; previous tree remains at %s; restore failed: %v", err, backup, restoreErr)
		}
		return err
	}
	return os.RemoveAll(backup)
}
