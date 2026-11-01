package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

func compositionParentGroup(t *testing.T, root, name, parents, first, second string) {
	t.Helper()
	ext := strings.Replace(compositionImportExtension, "interface-review", first, 1)
	if second != "" {
		ext += strings.TrimPrefix(strings.Replace(compositionImportExtension, "interface-review", second, 1), "authoring: prifly-extension/3\nimports: {ui: review-library}\nextensions:\n")
	}
	selectors := name + "." + first
	if second != "" {
		selectors += ", " + name + "." + second
	}
	ext += "arrange:\n  - id: checks\n    between: {from: build, to: commit}\n    sequence: [" + selectors + "]\n    on: {succeeded: commit, rejected: rejected}\n"
	registerCompositionDerived(t, root, name, parents, ext)
}

// Count reachable calls by invocation path. A global visited set would hide
// repeated execution of the same workflow revision in independent branches.
func compositionReachableReviewCalls(t *testing.T, c projectCompileResult, rootName string) int {
	t.Helper()
	workflows := map[flow.Ref]flow.WorkflowRevision{}
	root := flow.Ref{}
	for _, component := range c.Components {
		if component.Kind != "workflow" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(c.Output, component.Path))
		if err != nil {
			t.Fatal(err)
		}
		var w flow.WorkflowRevision
		if err := json.Unmarshal(data, &w); err != nil {
			t.Fatal(err)
		}
		workflows[component.Ref] = w
		if strings.HasSuffix(component.Ref.ID, "/"+rootName) {
			root = component.Ref
		}
	}
	if root.ID == "" {
		t.Fatal("missing compiled root")
	}
	var count func(flow.Ref, int) int
	count = func(ref flow.Ref, depth int) int {
		if depth > 32 {
			t.Fatal("unexpected recursive compiled graph")
		}
		w, ok := workflows[ref]
		if !ok {
			t.Fatalf("missing workflow %v", ref)
		}
		total := 0
		for _, stage := range w.Definition.Stages {
			if stage.Kind == "call" {
				if stage.WorkflowRef.ID == "test:workflow/review" {
					total++
				}
				total += count(stage.WorkflowRef, depth+1)
			}
			for _, branch := range stage.ParallelBranches {
				total += count(branch.WorkflowRef, depth+1)
			}
		}
		return total
	}
	return count(root, 0)
}

func TestProjectCompositionRegroupsInheritedGroupsWithoutDroppingChecks(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, operator string
		multiple       bool
	}{
		{"single", "sequence: [parent.checks]", false},
		{"multiple", "parallel: [parent.checks, other.checks]", true},
		{"nested wrappers", "sequence: [{parallel: [{sequence: [parent.checks]}]}]", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root, authority := writeCompositionFixture(t)
			if err := os.Remove(filepath.Join(root, ".prifly/workflows/cycle/extend.yaml")); err != nil {
				t.Fatal(err)
			}
			compositionParentGroup(t, root, "parent", "cycle", "first", "second")
			parents := "parent"
			want := 2
			if test.multiple {
				compositionParentGroup(t, root, "other", "cycle", "third", "fourth")
				parents = "[parent, other]"
				want = 4
			}
			ext := "authoring: prifly-extension/3\narrange:\n  - id: outer-checks\n    between: {from: build, to: commit}\n    " + test.operator + "\n    on: {succeeded: commit, rejected: rejected}\n"
			registerCompositionDerived(t, root, "consumer", parents, ext)
			c := compileCompositionName(t, root, authority, "consumer")
			if got := compositionReachableReviewCalls(t, c, "consumer"); got != want {
				t.Fatalf("mandatory checks disappeared: got %d reachable calls, want %d", got, want)
			}
		})
	}
}

func TestProjectCompositionRefusesDuplicateArrangeGroupIDs(t *testing.T) {
	t.Parallel()
	root, _ := writeCompositionFixture(t)
	if err := os.Remove(filepath.Join(root, ".prifly/workflows/cycle/extend.yaml")); err != nil {
		t.Fatal(err)
	}
	first := strings.Replace(compositionImportExtension, "interface-review", "first", 1)
	second := strings.TrimPrefix(strings.Replace(strings.Replace(compositionImportExtension, "interface-review", "second", 1), "between: {from: build, to: commit}", "between: {from: commit, to: done}", 1), "authoring: prifly-extension/3\nimports: {ui: review-library}\nextensions:\n")
	ext := first + second + "arrange:\n"
	for _, part := range []struct{ from, to, selector string }{{"build", "commit", "first"}, {"commit", "done", "second"}} {
		ext += fmt.Sprintf("  - id: duplicate\n    between: {from: %s, to: %s}\n    sequence: [parent.%s]\n    on: {succeeded: %s, rejected: rejected}\n", part.from, part.to, part.selector, part.to)
	}
	registerCompositionDerived(t, root, "parent", "cycle", ext)
	_, err := readProjectWorkflowFolder(root, filepath.Join(root, ".prifly/workflows/parent"))
	if compositionFaultCode(err) != "project_extension_stage_conflict" {
		t.Fatalf("duplicate group id did not refuse: %v", err)
	}
}

func TestProjectCompositionRefusesOverlappingGroupedOrigins(t *testing.T) {
	t.Parallel()
	for _, groupBoth := range []bool{false, true} {
		t.Run(fmt.Sprintf("both-grouped=%t", groupBoth), func(t *testing.T) {
			t.Parallel()
			root, _ := writeCompositionFixture(t)
			group := func() string {
				return "authoring: prifly-extension/3\narrange:\n  - id: checks\n    between: {from: build, to: commit}\n    sequence: [cycle.interface-review]\n    on: {succeeded: commit, rejected: rejected}\n"
			}
			registerCompositionDerived(t, root, "left", "cycle", group())
			other := ""
			selector := "cycle.interface-review"
			if groupBoth {
				other = group()
				selector = "right.checks"
			}
			registerCompositionDerived(t, root, "right", "cycle", other)
			ext := "authoring: prifly-extension/3\narrange:\n  - id: combined\n    between: {from: build, to: commit}\n    sequence: [left.checks, " + selector + "]\n    on: {succeeded: commit, rejected: rejected}\n"
			registerCompositionDerived(t, root, "consumer", "[left, right]", ext)
			_, err := readProjectWorkflowFolder(root, filepath.Join(root, ".prifly/workflows/consumer"))
			if compositionFaultCode(err) != "project_composition_duplicate_contribution" {
				t.Fatalf("overlapping origin accepted: %v", err)
			}
		})
	}
}

func TestProjectCompositionBoundsEffectiveInheritedGroupDepth(t *testing.T) {
	t.Parallel()
	root, _ := writeCompositionFixture(t)
	member := "cycle.interface-review"
	for range 7 {
		member = "{sequence: [" + member + "]}"
	}
	ext := "authoring: prifly-extension/3\narrange:\n  - id: checks\n    between: {from: build, to: commit}\n    sequence: [" + member + "]\n    on: {succeeded: commit, rejected: rejected}\n"
	registerCompositionDerived(t, root, "parent", "cycle", ext)
	registerCompositionDerived(t, root, "consumer", "parent", "authoring: prifly-extension/3\narrange:\n  - id: outer\n    between: {from: build, to: commit}\n    sequence: [parent.checks]\n    on: {succeeded: commit, rejected: rejected}\n")
	_, err := readProjectWorkflowFolder(root, filepath.Join(root, ".prifly/workflows/consumer"))
	if compositionFaultCode(err) != "project_composition_limit" {
		t.Fatalf("effective depth exceeded without refusal: %v", err)
	}
}

func TestProjectCompositionDeduplicatesAnUnchangedInheritedGroup(t *testing.T) {
	t.Parallel()
	root, authority := writeCompositionFixture(t)
	group := "authoring: prifly-extension/3\narrange:\n  - id: checks\n    between: {from: build, to: commit}\n    sequence: [cycle.interface-review]\n    on: {succeeded: commit, rejected: rejected}\n"
	registerCompositionDerived(t, root, "common", "cycle", group)
	registerCompositionDerived(t, root, "left", "common", "")
	registerCompositionDerived(t, root, "right", "common", "")
	registerCompositionDerived(t, root, "consumer", "[left, right]", "")
	c := compileCompositionName(t, root, authority, "consumer")
	if got := compositionReachableReviewCalls(t, c, "consumer"); got != 1 {
		t.Fatalf("shared group executes %d times, want once", got)
	}
}
