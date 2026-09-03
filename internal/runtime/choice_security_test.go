package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

func assertChoiceSecurityUnchanged(t *testing.T, e *Engine, before TelemetryResponse, canary string) {
	t.Helper()
	after := telemetryReport(t, e, telemetryQuery("catalog"))
	if after.Cut != before.Cut || !reflect.DeepEqual(after.Population, before.Population) {
		t.Fatal("refused condition changed the authority catalog")
	}
	runs, cut, err := e.Store.ReadAll(context.Background(), 100)
	if err != nil || cut != before.Cut || len(runs) != 0 {
		t.Fatalf("refused condition admitted a Run or Attempt: runs=%d cut=%d error=%v", len(runs), cut, err)
	}
	if files, err := os.ReadDir(filepath.Join(e.Root, e.Config.Configuration.WorkspaceRoot)); err != nil || len(files) != 0 {
		t.Fatalf("refused condition allocated a worker workspace: %v %v", files, err)
	}
	if canary != "" {
		if _, err := os.Lstat(canary); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("condition executed its canary or concealed its state: %v", err)
		}
	}
}

func TestChoiceSecurityRefusesExecutablePredicatesBeforeAdmission(t *testing.T) {
	e, workflow, options := choiceFixture(t, `{"flag":true}`, "")
	canary := filepath.Join(t.TempDir(), "condition-executed")
	command := "printf touched > '" + strings.ReplaceAll(canary, "'", "'\\''") + "'"
	depth := any(map[string]any{"op": "eq", "left": map[string]any{"kind": "literal", "value": true}, "right": map[string]any{"kind": "literal", "value": true}})
	for i := 1; i < 17; i++ {
		depth = map[string]any{"op": "all", "args": []any{depth}}
	}
	args := make([]any, 256)
	for i := range args {
		args[i] = map[string]any{"op": "exists", "ref": map[string]any{"from": "workflow_input", "port": "control", "pointer": "/flag"}}
	}
	cases := []struct {
		name, code string
		predicate  any
	}{
		{"shell_expression", "schema_invalid", map[string]any{"op": "shell", "command": command}},
		{"sql_expression", "schema_invalid", map[string]any{"op": "sql", "query": "SELECT writefile('" + strings.ReplaceAll(canary, "'", "''") + "', 'touched')"}},
		{"template_expression", "schema_invalid", map[string]any{"op": "template", "expression": "{{ exec " + command + " }}"}},
		{"raw_string_predicate", "schema_invalid", "$(" + command + ")"},
		{"unknown_operator", "schema_invalid", map[string]any{"op": "eval"}},
		{"depth17", "predicate_limit", depth},
		{"operators257", "predicate_limit", map[string]any{"op": "all", "args": args}},
	}
	for _, source := range []string{"filesystem", "environment", "network", "llm", "live_state"} {
		cases = append(cases, struct {
			name, code string
			predicate  any
		}{source + "_field", "schema_invalid", map[string]any{"op": "exists", "ref": map[string]any{"from": source, "port": "control", "pointer": "/flag"}}})
	}
	writeRuntimeJSON(t, filepath.Join(e.Root, "prifly.json"), e.Config)
	before := telemetryReport(t, e, telemetryQuery("catalog"))
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			choiceStages(workflow)["pick"] = map[string]any{"kind": "choice", "selection": "exclusive", "branches": []any{map[string]any{"id": "invalid", "predicate": tc.predicate, "next": "done"}}}
			writeRuntimeJSON(t, filepath.Join(e.Root, options.WorkflowFile), workflow)
			for _, operation := range []string{"preview", "start"} {
				var err error
				if operation == "preview" {
					_, err = e.Preview(PreviewOptions{WorkflowFile: options.WorkflowFile, BriefFile: options.BriefFile})
				} else {
					_, err = e.Start(context.Background(), options)
				}
				var problem *flow.Problem
				if !errors.As(err, &problem) || problem.Code != tc.code {
					t.Fatalf("%s expected %s before admission, got %v", operation, tc.code, err)
				}
				assertChoiceSecurityUnchanged(t, e, before, canary)
			}
		})
	}
}

func TestChoiceSecurityLiteralStringsRemainData(t *testing.T) {
	canary := filepath.Join(t.TempDir(), "literal-executed")
	command := "printf touched > '" + strings.ReplaceAll(canary, "'", "'\\''") + "'"
	t.Setenv("CHOICE_SECURITY_VALUE", "expanded-control-value")
	for name, value := range map[string]string{
		"shell":       "$(" + command + ")",
		"sql":         "SELECT writefile('" + strings.ReplaceAll(canary, "'", "''") + "', 'touched')",
		"template":    "{{ exec " + command + " }}",
		"environment": "${CHOICE_SECURITY_VALUE}",
	} {
		t.Run(name, func(t *testing.T) {
			e, workflow, options := choiceFixture(t, `{}`, "")
			equal := func(other string) map[string]any {
				return map[string]any{"op": "eq", "left": map[string]any{"kind": "literal", "value": value}, "right": map[string]any{"kind": "literal", "value": other}}
			}
			stages := choiceStages(workflow)
			stages["pick"] = choiceStage("exclusive", choiceBranch("empty", equal(""), "rejected"), choiceBranch("expanded", equal("expanded-control-value"), "rejected"), choiceBranch("unchanged", equal(value), "done"))
			stages["rejected"] = choiceFinish("rejected")
			writeRuntimeJSON(t, filepath.Join(e.Root, options.WorkflowFile), workflow)
			writeRuntimeJSON(t, filepath.Join(e.Root, "prifly.json"), e.Config)
			plan, _, _, _, err := e.compileFile(options.WorkflowFile)
			if err != nil {
				t.Fatal("literal text was mistaken for executable AST", err)
			}
			calls := 0
			decision, err := plan.SelectChoice("pick", func(flow.FieldRef) (any, bool, error) {
				calls++
				return nil, false, errors.New("literal must not resolve a source")
			})
			if err != nil || calls != 0 || decision.Route != "branch" || decision.BranchID != "unchanged" {
				t.Fatalf("literal was expanded, executed or resolved: %+v calls=%d error=%v", decision, calls, err)
			}
			before := telemetryReport(t, e, telemetryQuery("catalog"))
			if _, err := e.Preview(PreviewOptions{WorkflowFile: options.WorkflowFile, BriefFile: options.BriefFile}); err != nil {
				t.Fatal(err)
			}
			assertChoiceSecurityUnchanged(t, e, before, canary)
			runID := choiceStart(t, e, workflow, options)
			if err := e.Drive(context.Background(), runID); err != nil {
				t.Fatal(err)
			}
			r := driverRun(t, e, runID)
			if r.Status != "completed" || r.Outcome == nil || *r.Outcome != "succeeded" || len(r.Steps) != 0 || len(r.Attempts) != 0 {
				t.Fatal("literal condition did not finish without a worker")
			}
			if _, err := os.Lstat(canary); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("literal command executed: %v", err)
			}
		})
	}
}

func TestChoiceSecurityInputSchemaFailurePrecedesRouting(t *testing.T) {
	// Accepted artifacts cannot carry this value under the declared object
	// schema. Exercise the real input boundary instead of forging stored state.
	e, workflow, options := choiceFixture(t, `[]`, "")
	stages := choiceStages(workflow)
	stage := stages["pick"].(map[string]any)
	stage["default"], stage["on_unknown"], stage["on_error"] = "rejected", "rejected", "rejected"
	stages["rejected"] = choiceFinish("rejected")
	writeRuntimeJSON(t, filepath.Join(e.Root, options.WorkflowFile), workflow)
	writeRuntimeJSON(t, filepath.Join(e.Root, "prifly.json"), e.Config)
	before := telemetryReport(t, e, telemetryQuery("catalog"))
	_, err := e.Start(context.Background(), options)
	var problem *flow.Problem
	if !errors.As(err, &problem) || problem.Code != "schema_invalid" {
		t.Fatalf("invalid source schema entered condition routing: %v", err)
	}
	assertChoiceSecurityUnchanged(t, e, before, "")
}
