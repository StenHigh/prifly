package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func contextSourceFixture(t *testing.T) (string, string, string) {
	t.Helper()
	root, authority := t.TempDir(), filepath.Join(t.TempDir(), "authority")
	if code, _, stderr := runCLI(t, "project", "init", "--repository", root, "--state-root", authority); code != 0 {
		t.Fatalf("init: %d %s", code, stderr)
	}
	folder := ".prifly/workflows/notes"
	writeFixtureWorkflowFolder(t, root, folder, "notes")
	writeFixtureFile(t, root, ".prifly/project.yaml", "schema_version: prifly-project-profile/3\npackages:\n  notes: {source: "+folder+"}\nlaunches: {}\n")
	return root, authority, folder
}

func TestCLIProjectContextSourcePreservesInlineBytes(t *testing.T) {
	t.Parallel()
	root, authority, folder := contextSourceFixture(t)
	text := "# Проверка\r\n\r\nKeep exact bytes.\r\n\r\n"
	metadata := "id: test:context/notes\nversion: 1.0.0\nmedia_type: text/markdown; charset=utf-8\n"
	contextPath := folder + "/contexts/notes.yaml"
	compile := func() projectCompileResult {
		t.Helper()
		output := filepath.Join(t.TempDir(), "sealed")
		code, stdout, stderr := runCLI(t, "--project", authority, "project", "compile", "--repository", root, "--package", "notes", "--output", output)
		if code != 0 {
			t.Fatalf("compile: %d %s", code, stderr)
		}
		var result projectCompileResult
		if err := json.Unmarshal([]byte(stdout), &result); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, component := range result.Components {
			if component.Kind != "context" {
				continue
			}
			found = true
			data, err := os.ReadFile(filepath.Join(output, component.Path))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(data, []byte(text)) {
				t.Fatalf("sealed context bytes changed: %q", data)
			}
		}
		if !found {
			t.Fatal("compile inspected no context resource")
		}
		return result
	}
	writeFixtureFile(t, root, contextPath, metadata+"text: "+strconv.Quote(text)+"\n")
	inline := compile()
	source := folder + "/instructions/notes.md"
	writeFixtureFile(t, root, source, text)
	writeFixtureFile(t, root, contextPath, metadata+"source: "+source+"\n")
	fromFile := compile()
	if inline.Package != fromFile.Package || inline.BuildKey != fromFile.BuildKey || !reflect.DeepEqual(inline.Components, fromFile.Components) {
		t.Fatal("moving unchanged context bytes changed the sealed build")
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); !os.IsNotExist(err) {
		t.Fatalf("hostless compile created Git: %v", err)
	}
}

func TestCLIProjectContextSourceRefusesMissingAndEscapingFiles(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"missing", "traversal", "symlink"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root, authority, folder := contextSourceFixture(t)
			source := folder + "/instructions/missing.md"
			outside := filepath.Join(t.TempDir(), "outside.md")
			if err := os.WriteFile(outside, []byte("outside\n"), 0600); err != nil {
				t.Fatal(err)
			}
			switch name {
			case "traversal":
				relative, err := filepath.Rel(root, outside)
				if err != nil {
					t.Fatal(err)
				}
				source = filepath.ToSlash(relative)
			case "symlink":
				source = folder + "/instructions/link.md"
				path := filepath.Join(root, filepath.FromSlash(source))
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, path); err != nil {
					t.Fatal(err)
				}
			}
			writeFixtureFile(t, root, folder+"/contexts/notes.yaml", "id: test:context/notes\nversion: 1.0.0\nmedia_type: text/markdown\nsource: "+strconv.Quote(source)+"\n")
			output := filepath.Join(t.TempDir(), "sealed")
			code, _, stderr := runCLI(t, "--project", authority, "project", "compile", "--repository", root, "--package", "notes", "--output", output)
			if code == 0 || !strings.Contains(stderr, "project_package_invalid") {
				t.Fatalf("unsafe context source accepted: %d %s", code, stderr)
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatalf("refusal wrote a sealed output: %v", err)
			}
		})
	}
}
