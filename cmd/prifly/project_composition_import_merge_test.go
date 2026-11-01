package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectCompositionMergesIdenticalImportDeclarations(t *testing.T) {
	t.Parallel()
	for _, target := range []string{"review-library", "cycle"} {
		t.Run(target, func(t *testing.T) {
			t.Parallel()
			root, authority := writeCompositionFixture(t)
			path := filepath.Join(root, ".prifly/workflows/cycle/workflow.yaml")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			writeFixtureFile(t, root, ".prifly/workflows/cycle/workflow.yaml", strings.Replace(string(data), "prifly-project-workflow/1", "prifly-project-workflow/2", 1)+"\nimports: {ui: review-library}\n")
			writeFixtureFile(t, root, ".prifly/workflows/cycle/extend.yaml", compositionImportExtension+"layers: [../../../shared/imports.yaml]\n")
			writeFixtureFile(t, root, "shared/imports.yaml", "authoring: prifly-extension/3\nimports: {ui: "+target+"}\n")
			compiled, _, problem := compileCallExtensionFixture(t, root, authority)
			if target != "review-library" {
				if problem == nil || problem.Code != "project_composition_invalid_import" {
					t.Fatalf("conflicting alias accepted: %v", problem)
				}
				return
			}
			if problem != nil {
				t.Fatal(problem)
			}
			refs := map[string]bool{}
			workflows := 0
			for _, component := range compiled.Components {
				if component.Kind == "workflow" {
					workflows++
				}
				key := component.Ref.ID + "@" + component.Ref.Version
				if refs[key] {
					t.Fatalf("provider component included twice: %s", key)
				}
				refs[key] = true
			}
			if workflows != 2 {
				t.Fatalf("expected root and one imported workflow, got %d", workflows)
			}
		})
	}
}

func TestProjectCompositionRetainsDeclaredImportTargetThroughCache(t *testing.T) {
	t.Parallel()
	for _, target := range []string{"review-library", "a-library"} {
		t.Run(target, func(t *testing.T) {
			t.Parallel()
			root, authority := writeCompositionFixture(t)
			profilePath := filepath.Join(root, ".prifly/project.yaml")
			profile, err := os.ReadFile(profilePath)
			if err != nil {
				t.Fatal(err)
			}
			writeFixtureFile(t, root, ".prifly/project.yaml", strings.Replace(string(profile), "launches:", "  a-library: {source: .prifly/workflows/review-library}\nlaunches:", 1))
			path := filepath.Join(root, ".prifly/workflows/cycle/workflow.yaml")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			writeFixtureFile(t, root, ".prifly/workflows/cycle/workflow.yaml", strings.Replace(string(data), "prifly-project-workflow/1", "prifly-project-workflow/2", 1)+"\nimports: {first: a-library, ui: review-library}\n")
			writeFixtureFile(t, root, ".prifly/workflows/cycle/extend.yaml", strings.Replace(compositionImportExtension, "imports: {ui: review-library}", "imports: {ui: "+target+"}", 1))
			compiled, _, problem := compileCallExtensionFixture(t, root, authority)
			if target != "review-library" {
				if problem == nil || problem.Code != "project_composition_invalid_import" {
					t.Fatalf("different declared target must refuse despite shared source: %v", problem)
				}
				return
			}
			if problem != nil {
				t.Fatal(problem)
			}
			if compiled.Composition == nil || compiled.Composition.Imports["first"] != "a-library" || compiled.Composition.Imports["ui"] != "review-library" {
				t.Fatalf("overview lost declared import targets: %+v", compiled.Composition)
			}
		})
	}
}
