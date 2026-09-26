package flow

import (
	"strings"
	"testing"
)

// continuationFixture is the two-check graph at revision 7 with the report of
// both checks declared as its checkpoint, and a continuation filling both of
// its inputs from a source Run.
func continuationFixture(t *testing.T) (map[string]any, Registry) {
	t.Helper()
	workflow, registry := verdictFixture(t)
	workflow["schema_version"] = WorkflowRevisionContinuationVersion
	report := workflow["outputs"].(map[string]any)["report_first"].(map[string]any)["schema_ref"]
	workflow["checkpoint"] = map[string]any{"schema_ref": report}
	for _, id := range []string{"check_first", "check_second"} {
		stage := stages(workflow)[id].(map[string]any)
		stage["checkpoint"] = "report"
		stage["impossible_verdicts"] = []any{"needs_revision", "no_work", "blocked"}
	}
	workflow["continuation"] = map[string]any{
		"from_workflows": []any{"example:workflow/source"},
		"from_outcomes":  []any{"partial", "rejected"},
		"inputs": map[string]any{
			"first":  map[string]any{"source_input": "first"},
			"second": map[string]any{"stage": "prepare", "output": "document", "verdict": "pass"},
		},
	}
	return workflow, registry
}

func TestRevisionSevenSealsCheckpointAndContinuation(t *testing.T) {
	workflow, registry := continuationFixture(t)
	plan, err := CompileProfile(encoded(t, workflow), "json", registry, CoreProfile)
	if err != nil {
		t.Fatalf("a complete declaration was refused: %v", err)
	}
	if plan.Workflow.Checkpoint == nil || plan.Workflow.Definition.Stages["check_first"].Checkpoint != "report" {
		t.Fatalf("the checkpoint declaration did not reach the plan: %+v", plan.Workflow.Checkpoint)
	}
	continuation := plan.Workflow.Continuation
	if continuation == nil || continuation.Inputs["second"].Stage != "prepare" || continuation.Inputs["first"].SourceInput != "first" {
		t.Fatalf("the continuation did not reach the plan: %+v", continuation)
	}
	if !strings.Contains(string(plan.Canonical), `"continuation"`) {
		t.Fatal("the continuation is not part of the sealed bytes, so the digest does not cover it")
	}
	// The checkpoint alone is also a whole declaration: a workflow may keep one
	// without continuing anything.
	delete(workflow, "continuation")
	if _, err := CompileProfile(encoded(t, workflow), "json", registry, CoreProfile); err != nil {
		t.Fatalf("a checkpoint without a continuation was refused: %v", err)
	}
}

func TestRevisionSevenRefusesWhatItCannotKeep(t *testing.T) {
	for _, test := range []struct {
		name, code, pointer string
		edit                func(map[string]any)
	}{
		{"stage reports an undeclared checkpoint", "invalid_checkpoint", "/definition/stages/check_first/checkpoint", func(w map[string]any) {
			delete(w, "checkpoint")
		}},
		{"stage names an output the step lacks", "invalid_checkpoint", "/definition/stages/check_first/checkpoint", func(w map[string]any) {
			stages(w)["check_first"].(map[string]any)["checkpoint"] = "missing"
		}},
		{"output carries another schema", "invalid_checkpoint", "/definition/stages/check_first/checkpoint", func(w map[string]any) {
			w["checkpoint"] = map[string]any{"schema_ref": w["inputs"].(map[string]any)["first"].(map[string]any)["schema_ref"]}
		}},
		{"continuation fills an undeclared input", "invalid_continuation", "/continuation/inputs/third", func(w map[string]any) {
			w["continuation"].(map[string]any)["inputs"].(map[string]any)["third"] = map[string]any{"checkpoint": true}
		}},
		{"empty source list", "schema_invalid", "", func(w map[string]any) {
			w["continuation"].(map[string]any)["from_workflows"] = []any{}
		}},
		{"empty outcome list", "schema_invalid", "", func(w map[string]any) {
			w["continuation"].(map[string]any)["from_outcomes"] = []any{}
		}},
		{"a technical failure is not an outcome", "schema_invalid", "", func(w map[string]any) {
			w["continuation"].(map[string]any)["from_outcomes"] = []any{"failed"}
		}},
		{"unknown verdict", "schema_invalid", "", func(w map[string]any) {
			w["continuation"].(map[string]any)["inputs"].(map[string]any)["second"] = map[string]any{"stage": "prepare", "output": "document", "verdict": "maybe"}
		}},
		{"two sources for one input", "schema_invalid", "", func(w map[string]any) {
			w["continuation"].(map[string]any)["inputs"].(map[string]any)["second"] = map[string]any{"source_input": "second", "checkpoint": true}
		}},
		{"stage with both a verdict and an outcome", "schema_invalid", "", func(w map[string]any) {
			w["continuation"].(map[string]any)["inputs"].(map[string]any)["second"] = map[string]any{"stage": "prepare", "output": "document", "verdict": "pass", "outcome": "succeeded"}
		}},
		{"revision 6 cannot name a continuation", "schema_invalid", "", func(w map[string]any) {
			w["schema_version"] = WorkflowRevisionBlockedVersion
			delete(w, "checkpoint")
			for _, id := range []string{"check_first", "check_second"} {
				delete(stages(w)[id].(map[string]any), "checkpoint")
			}
		}},
		{"revision 6 cannot name a checkpoint", "schema_invalid", "", func(w map[string]any) {
			w["schema_version"] = WorkflowRevisionBlockedVersion
			delete(w, "continuation")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			workflow, registry := continuationFixture(t)
			test.edit(workflow)
			_, err := CompileProfile(encoded(t, workflow), "json", registry, CoreProfile)
			p := expectProblem(t, err, test.code)
			if test.pointer != "" && p.Path != test.pointer {
				t.Fatalf("refused at %s, want %s: %v", p.Path, test.pointer, p)
			}
		})
	}
}

// Revision 7 answers for every verdict, as revision 6 does: a declaration of
// what a workflow keeps is no reason for its routes to answer for less.
func TestRevisionSevenAnswersForEveryVerdict(t *testing.T) {
	workflow, registry := continuationFixture(t)
	stages(workflow)["check_first"].(map[string]any)["impossible_verdicts"] = []any{"needs_revision", "no_work"}
	_, err := CompileProfile(encoded(t, workflow), "json", registry, CoreProfile)
	if p := expectProblem(t, err, "missing_handler"); !strings.Contains(p.Message, "blocked") {
		t.Fatalf("refused for another reason: %v", p)
	}
}

// The authoring ladder reaches revision 7 on the fields alone, the way it
// reaches every other revision: an author writes what they mean and the
// lowest contract that carries it is chosen.
func TestAuthoringRaisesToRevisionSevenOnItsFields(t *testing.T) {
	for _, source := range []map[string]any{
		{"checkpoint": map[string]any{"schema_ref": "report"}},
		{"continuation": map[string]any{"from_workflows": []any{"a:workflow/b"}, "from_outcomes": []any{"partial"}, "inputs": map[string]any{}}},
	} {
		if version := authorSchemaVersion(source, map[string]any{}, map[string]any{}); version != WorkflowRevisionContinuationVersion {
			t.Errorf("%v derived revision %s", source, version)
		}
	}
	stage := map[string]any{"s": map[string]any{"kind": "step", "checkpoint": "state"}}
	if version := authorSchemaVersion(map[string]any{}, map[string]any{}, stage); version != WorkflowRevisionContinuationVersion {
		t.Errorf("a stage checkpoint derived revision %s", version)
	}
}

// A called workflow reports its checkpoint into the same Run as its caller, so
// it may keep only the caller's shape; one the caller does not declare at all
// would leave the Run's last checkpoint without a shape to check against.
func TestCalledWorkflowKeepsTheCallersCheckpointShape(t *testing.T) {
	root, child, registry := callBindingFixture(t)
	ref := *root.Inputs["value"].SchemaRef
	other := []byte(`{"type":"object"}`)
	digest, _ := Digest(other)
	otherRef := Ref{ID: "test:schema/other", Version: "1.0.0", Digest: digest}
	registry[otherRef] = other
	root.SchemaVersion, child.SchemaVersion = WorkflowRevisionContinuationVersion, WorkflowRevisionContinuationVersion
	child.Checkpoint = &CheckpointDeclaration{SchemaRef: ref}
	if _, err := compileCallFixture(t, root, child, registry); expectProblem(t, err, "invalid_checkpoint").Path != "/definition/stages/call" {
		t.Fatalf("a caller without a checkpoint accepted a child that reports one: %v", err)
	}
	root.Checkpoint = &CheckpointDeclaration{SchemaRef: otherRef}
	if _, err := compileCallFixture(t, root, child, registry); expectProblem(t, err, "invalid_checkpoint").Path != "/definition/stages/call" {
		t.Fatalf("a child of another checkpoint shape was accepted: %v", err)
	}
	root.Checkpoint = &CheckpointDeclaration{SchemaRef: ref}
	if _, err := compileCallFixture(t, root, child, registry); err != nil {
		t.Fatalf("the same checkpoint shape was refused: %v", err)
	}
}
