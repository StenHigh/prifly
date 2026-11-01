package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectCompositionBoundsCachedDependencySubtrees(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"parents", "imports"} {
		for _, length := range []int{6, 7} {
			for _, reverse := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/chain=%d/reverse=%t", mode, length, reverse), func(t *testing.T) {
					t.Parallel()
					root, authority := writeCompositionFixture(t)
					if err := os.Remove(filepath.Join(root, ".prifly/workflows/cycle/extend.yaml")); err != nil {
						t.Fatal(err)
					}
					registerCompositionDerived(t, root, "ancestor", "cycle", "")
					parent := "ancestor"
					for i := length; i >= 1; i-- {
						name := fmt.Sprintf("chain-%d", i)
						registerCompositionDerived(t, root, name, parent, "")
						parent = name
					}
					parents := "[ancestor, chain-1]"
					if reverse {
						parents = "[chain-1, ancestor]"
					}
					if mode == "imports" {
						parents = "cycle"
					}
					registerCompositionDerived(t, root, "consumer", parents, "")
					if mode == "imports" {
						compositionEdit(t, root, ".prifly/workflows/consumer/workflow.yaml", func(value map[string]any) {
							first, last := "ancestor", "chain-1"
							if reverse {
								first, last = last, first
							}
							value["imports"] = map[string]any{"a": first, "z": last}
						})
					}
					_, err := readProjectWorkflowFolder(root, filepath.Join(root, ".prifly/workflows/consumer"))
					if length == 7 {
						if compositionFaultCode(err) != "project_composition_limit" {
							t.Fatalf("depth 9 must refuse regardless of cache order: %v", err)
						}
						return
					}
					if err != nil {
						t.Fatalf("depth 8 must remain valid: %v", err)
					}
					compileCompositionName(t, root, authority, "consumer")
				})
			}
		}
	}
}

func TestProjectCompositionOverviewRetainsDeclaredParentNames(t *testing.T) {
	t.Parallel()
	root, authority := writeCompositionFixture(t)
	if err := os.Remove(filepath.Join(root, ".prifly/workflows/cycle/extend.yaml")); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".prifly/project.yaml")
	profile, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, root, ".prifly/project.yaml", strings.Replace(string(profile), "launches:", "  a-cycle: {source: .prifly/workflows/cycle}\nlaunches:", 1))
	registerCompositionDerived(t, root, "left", "a-cycle", "")
	registerCompositionDerived(t, root, "consumer", "[left, cycle]", "")
	compiled := compileCompositionName(t, root, authority, "consumer")
	if compiled.Composition == nil || strings.Join(compiled.Composition.Parents, ",") != "left,cycle" {
		t.Fatalf("overview replaced declared parent with cached alias: %+v", compiled.Composition)
	}
}
