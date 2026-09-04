package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

func TestProjectLocalExecutableAllowList(t *testing.T) {
	root, state := t.TempDir(), filepath.Join(t.TempDir(), "authority")
	if code, _, stderr := runCLI(t, "project", "init", "--repository", root, "--state-root", state); code != 0 {
		t.Fatalf("init: %d %s", code, stderr)
	}
	shared, err := os.ReadFile(filepath.Join(root, ".prifly", "project.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	authority, programs, err := projectLocalExecution(root)
	if err != nil || len(programs) != 0 {
		t.Fatalf("optional allow mapping: %q %#v %v", authority, programs, err)
	}
	marker := filepath.Join(t.TempDir(), "executed")
	program := filepath.Join(t.TempDir(), "worker")
	writeFixtureFile(t, filepath.Dir(program), filepath.Base(program), "#!/bin/sh\necho unexpected > "+strconv.Quote(marker)+"\n")
	if err := os.Chmod(program, 0755); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runCLI(t, "project", "local", "set", "--repository", root, "--allow-executable", "report-worker="+program, "--allow-executable", "validate-worker="+program); code != 0 {
		t.Fatalf("allow executables: %d %s", code, stderr)
	}
	afterAuthority, programs, err := projectLocalExecution(root)
	if err != nil || afterAuthority != authority || len(programs) != 2 || programs["report-worker"] != program || programs["validate-worker"] != program {
		t.Fatalf("registered mapping: %q %#v %v", afterAuthority, programs, err)
	}
	localPath := filepath.Join(root, ".prifly", "local.yaml")
	local, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, root, ".prifly/local.yaml", string(local)+"# keep the team's machine-only notes\nextra_setting: keep\n")
	beforeUpdate, _ := os.ReadFile(localPath)
	newLauncher := filepath.Join(t.TempDir(), "future-prifly")
	if code, _, stderr := runCLI(t, "project", "local", "set", "--repository", root, "--executable", newLauncher); code != 0 {
		t.Fatalf("legacy launcher replacement: %d %s", code, stderr)
	}
	afterUpdate, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(beforeUpdate), "\n") {
		if !strings.HasPrefix(line, "prifly_executable:") && !strings.Contains(string(afterUpdate), line) {
			t.Fatalf("legacy --executable changed unrelated line %q", line)
		}
	}
	if code, _, stderr := runCLI(t, "project", "local", "set", "--repository", root, "--allow-executable", "extra-worker="+program, "--executable", program); code != 0 {
		t.Fatalf("combined local update: %d %s", code, stderr)
	}
	_, programs, err = projectLocalExecution(root)
	if err != nil || len(programs) != 3 {
		t.Fatalf("combined update lost permissions: %#v %v", programs, err)
	}
	afterShared, err := os.ReadFile(filepath.Join(root, ".prifly", "project.yaml"))
	if err != nil || !bytes.Equal(shared, afterShared) {
		t.Fatalf("local permission changed shared profile: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("registration executed the worker: %v", err)
	}
	// Reading unrelated authority state must not depend on every previously
	// allowed program still being installed. Launch checks selected bytes.
	if err := os.Remove(program); err != nil {
		t.Fatal(err)
	}
	if _, _, err := projectLocalExecution(root); err != nil {
		t.Fatalf("uninstalled unused executable blocked local read: %v", err)
	}
}

func TestProjectLocalExecutableAllowRejectsUnsafeInputs(t *testing.T) {
	root, authority := t.TempDir(), t.TempDir()
	if code, _, stderr := runCLI(t, "project", "init", "--repository", root, "--state-root", authority); code != 0 {
		t.Fatalf("init: %d %s", code, stderr)
	}
	program, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	notExecutable := filepath.Join(t.TempDir(), "data")
	writeFixtureFile(t, filepath.Dir(notExecutable), "data", "not executable\n")
	path := filepath.Join(root, ".prifly", "local.yaml")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		args []string
	}{
		{"relative", []string{"--allow-executable", "worker=relative"}},
		{"unknown-name", []string{"--allow-executable", "../worker=" + program}},
		{"directory", []string{"--allow-executable", "worker=" + t.TempDir()}},
		{"missing", []string{"--allow-executable", "worker=" + filepath.Join(t.TempDir(), "missing")}},
		{"not-executable", []string{"--allow-executable", "worker=" + notExecutable}},
		{"duplicate", []string{"--allow-executable", "worker=" + program, "--allow-executable", "worker=" + program}},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := append([]string{"project", "local", "set", "--repository", root}, test.args...)
			if code, _, stderr := runCLI(t, args...); code == 0 || !strings.Contains(stderr, "project_local_invalid_executable") {
				t.Fatalf("unsafe executable admitted: %d %s", code, stderr)
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("invalid update mutated local configuration: %v", err)
			}
		})
	}
	for _, test := range []struct{ name, contents string }{
		{"duplicate-key", string(before) + "executables: {worker: /bin/sh, worker: /bin/echo}\n"},
		{"duplicate-authority", string(before) + "authority_root: /another/authority\n"},
		{"relative-authority", "authority_root: relative\nprifly_executable: /bin/sh\n"},
		{"relative-program", string(before) + "executables: {worker: relative}\n"},
		{"unsafe-authority", "authority_root: " + strconv.Quote(root) + "\nprifly_executable: /bin/sh\n"},
		{"oversized", string(before) + "#" + strings.Repeat("x", flow.MaxDocumentBytes)},
	} {
		t.Run(test.name, func(t *testing.T) {
			writeFixtureFile(t, root, ".prifly/local.yaml", test.contents)
			if _, _, err := projectLocalExecution(root); err == nil {
				t.Fatal("unsafe local YAML admitted")
			}
		})
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "local.yaml")
	writeFixtureFile(t, filepath.Dir(outside), "local.yaml", string(before))
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	if _, _, err := projectLocalExecution(root); err == nil {
		t.Fatal("symlink local.yaml admitted")
	}
}
