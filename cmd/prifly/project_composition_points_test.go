package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectCompositionRejectsPointFormsBeforeFlattening(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		change func(*testing.T, string)
	}{
		{"root sidecar", func(t *testing.T, root string) { compositionPointSidecar(t, root, "cycle") }},
		{"provider sidecar", func(t *testing.T, root string) { compositionPointSidecar(t, root, "review-library") }},
		{"parent sidecar", func(t *testing.T, root string) {
			registerCompositionDerived(t, root, "derived", "cycle", "")
			compositionPointSidecar(t, root, "cycle")
		}},
		{"effective /4", func(t *testing.T, root string) {
			compositionEdit(t, root, ".prifly/workflows/cycle/workflow.yaml", func(v map[string]any) { v["authoring"] = projectWorkflowCompositionVersion })
			writeFixtureFile(t, root, ".prifly/workflows/cycle/extend.yaml", "authoring: prifly-extension/4\n")
		}},
		{"parent /4", func(t *testing.T, root string) {
			registerCompositionDerived(t, root, "derived", "cycle", "")
			writeFixtureFile(t, root, ".prifly/workflows/cycle/extend.yaml", "authoring: prifly-extension/4\n")
		}},
		{"layer /4", func(t *testing.T, root string) {
			writeFixtureFile(t, root, "shared/points.yaml", "authoring: prifly-extension/4\n")
			writeFixtureFile(t, root, ".prifly/workflows/cycle/extend.yaml", "authoring: prifly-extension/3\nlayers: [../../../shared/points.yaml]\n")
		}},
		{"point in /3", func(t *testing.T, root string) {
			writeFixtureFile(t, root, ".prifly/workflows/cycle/extend.yaml", "authoring: prifly-extension/3\nextensions: [{id: foreign, point: provider, step: tests, output_bindings: {}}]\n")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root, authority := writeCompositionFixture(t)
			test.change(t, root)
			name := "cycle"
			if strings.HasPrefix(test.name, "parent ") {
				name = "derived"
			}
			_, err := readProjectWorkflowFolder(root, filepath.Join(root, ".prifly/workflows", name))
			if code := compositionFaultCode(err); code != "project_composition_extension_points_unsupported" {
				t.Fatalf("unexpected code %s: %v", code, err)
			}
			output := filepath.Join(t.TempDir(), "compiled")
			code, _, stderr := runCLI(t, "--project", authority, "project", "compile", "--repository", root, "--package", name, "--output", output)
			if code == 0 || !strings.Contains(stderr, `"code":"project_composition_extension_points_unsupported"`) && !strings.Contains(stderr, `"code": "project_composition_extension_points_unsupported"`) {
				t.Fatalf("CLI did not preserve typed refusal: %d %s", code, stderr)
			}
			if _, err := os.Stat(filepath.Join(output, "prifly.package.json")); !os.IsNotExist(err) {
				t.Fatalf("refusal published a package: %v", err)
			}
		})
	}
}

func compositionPointSidecar(t *testing.T, root, name string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "examples", "nested-extension-points", "extension-points.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, root, ".prifly/workflows/"+name+"/extension-points.yaml", string(data))
}

func TestProjectCompositionImportIgnoresStandalonePointExtensions(t *testing.T) {
	t.Parallel()
	root, authority := writeCompositionFixture(t)
	before := compileCompositionName(t, root, authority, "cycle")
	// A source import must ignore the whole standalone document, including its
	// point selection and an unavailable implementation. It grants no consent.
	writeFixtureFile(t, root, ".prifly/workflows/review-library/extend.yaml", "authoring: prifly-extension/4\nextensions: [{id: absent, point: absent, call: private, output_bindings: {}}]\nexecution_bindings: {private: {program: forbidden}}\n")
	after := compileCompositionName(t, root, authority, "cycle")
	if before.Package != after.Package || before.BuildKey != after.BuildKey {
		t.Fatal("standalone provider /4 changed imported source identity")
	}
}
