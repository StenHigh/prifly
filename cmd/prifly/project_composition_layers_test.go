package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func TestProjectCompositionRepeatedExplicitLayersRefuse(t *testing.T) {
	t.Parallel()
	for _, structural := range []bool{false, true} {
		for _, shape := range []string{"siblings-forward", "siblings-reverse", "ancestor", "normalized-same-owner"} {
			t.Run(map[bool]string{false: "settings", true: "contribution"}[structural]+"/"+shape, func(t *testing.T) {
				t.Parallel()
				root, authority := writeCompositionFixture(t)
				if err := os.Remove(filepath.Join(root, ".prifly/workflows/cycle/extend.yaml")); err != nil {
					t.Fatal(err)
				}
				layer := "authoring: prifly-extension/3\nresources: {cpu: {limit: 2, stages: [build]}}\n"
				if structural {
					layer = compositionImportExtension
				}
				writeFixtureFile(t, root, "shared/repeat.yaml", layer)
				include := "authoring: prifly-extension/3\nlayers: [../../../shared/repeat.yaml]\n"
				switch shape {
				case "siblings-forward", "siblings-reverse":
					registerCompositionDerived(t, root, "left", "cycle", include)
					registerCompositionDerived(t, root, "right", "cycle", strings.Replace(include, "shared/repeat.yaml", "shared/../shared/repeat.yaml", 1))
					parents := "[left, right]"
					if shape == "siblings-reverse" {
						parents = "[right, left]"
					}
					registerCompositionDerived(t, root, "delivery", parents, "")
				case "ancestor":
					writeFixtureFile(t, root, ".prifly/workflows/cycle/extend.yaml", include)
					registerCompositionDerived(t, root, "delivery", "cycle", include)
				case "normalized-same-owner":
					registerCompositionDerived(t, root, "delivery", "cycle", strings.Replace(include, "repeat.yaml]", "repeat.yaml, ../../../shared/../shared/repeat.yaml]", 1))
				}
				output := filepath.Join(t.TempDir(), "compiled")
				code, _, stderr := runCLI(t, "--project", authority, "project", "compile", "--repository", root, "--package", "delivery", "--output", output)
				var problem prifly.Problem
				if err := json.Unmarshal([]byte(stderr), &problem); err != nil || code != 2 || problem.Code != "project_composition_duplicate_layer" {
					t.Fatalf("repeated declaration accepted or wrong refusal: exit=%d problem=%s parse=%v", code, stderr, err)
				}
				if _, err := os.Stat(output); !os.IsNotExist(err) {
					t.Fatalf("duplicate layer left partial output: %v", err)
				}
			})
		}
	}
}

func TestProjectCompositionCommonAncestorLayerAppliesOnce(t *testing.T) {
	t.Parallel()
	for _, structural := range []bool{false, true} {
		t.Run(map[bool]string{false: "settings", true: "contribution"}[structural], func(t *testing.T) {
			t.Parallel()
			root, authority := writeCompositionFixture(t)
			layer := "authoring: prifly-extension/3\nresources: {cpu: {limit: 2, stages: [build]}}\n"
			if structural {
				layer = compositionImportExtension
			}
			writeFixtureFile(t, root, "shared/repeat.yaml", layer)
			writeFixtureFile(t, root, ".prifly/workflows/cycle/extend.yaml", "authoring: prifly-extension/3\nlayers: [../../../shared/repeat.yaml]\n")
			registerCompositionDerived(t, root, "left", "cycle", "")
			registerCompositionDerived(t, root, "right", "cycle", "")
			registerCompositionDerived(t, root, "delivery", "[left, right]", "")
			compiled := compileCompositionName(t, root, authority, "delivery")
			if compiled.Composition == nil || len(compiled.Composition.Layers) != 1 || compiled.Composition.Layers[0] != "shared/repeat.yaml" {
				t.Fatalf("common layer not retained once: %+v", compiled.Composition)
			}
			if structural {
				if len(compiled.Composition.Contributions) != 1 {
					t.Fatalf("common contribution duplicated: %+v", compiled.Composition.Contributions)
				}
			} else {
				source, err := readProjectWorkflowFolder(root, filepath.Join(root, ".prifly/workflows/delivery"))
				if err != nil {
					t.Fatal(err)
				}
				if source.Composition.Options.Resources["cpu"].Limit != 2 {
					t.Fatalf("common layer resource lost: %+v", source.Composition.Options.Resources)
				}
			}
		})
	}
}

func TestProjectCompositionLibraryAndEffectiveLayersRemainSeparate(t *testing.T) {
	t.Parallel()
	for _, importFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "effective-first", true: "library-first"}[importFirst], func(t *testing.T) {
			t.Parallel()
			root, authority := writeCompositionFixture(t)
			compositionEdit(t, root, ".prifly/workflows/cycle/workflow.yaml", func(v map[string]any) {
				v["authoring"] = "prifly-project-workflow/2"
				v["exports"] = map[string]any{"steps": map[string]any{"build": "{{step_build}}"}}
			})
			writeFixtureFile(t, root, "shared/repeat.yaml", "authoring: prifly-extension/3\nresources: {cpu: {limit: 2, stages: [build]}}\n")
			writeFixtureFile(t, root, ".prifly/workflows/cycle/extend.yaml", "authoring: prifly-extension/3\nlayers: [../../../shared/repeat.yaml]\n")
			registerCompositionDerived(t, root, "delivery", "cycle", "")
			if importFirst {
				compositionEdit(t, root, ".prifly/workflows/delivery/workflow.yaml", func(v map[string]any) { v["imports"] = map[string]any{"base": "cycle"} })
			} else {
				writeFixtureFile(t, root, ".prifly/workflows/delivery/extend.yaml", "authoring: prifly-extension/3\nimports: {base: cycle}\n")
			}
			compiled := compileCompositionName(t, root, authority, "delivery")
			if len(compiled.Composition.Layers) != 1 {
				t.Fatalf("library repeated effective layers: %+v", compiled.Composition)
			}
		})
	}
}
