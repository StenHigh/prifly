package flow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"go.yaml.in/yaml/v3"
)

func aliasAuthor(t *testing.T, w WorkflowRevision, selectors map[string]string) []byte {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(encoded(t, w), &value); err != nil {
		t.Fatal(err)
	}
	for stageID, name := range selectors {
		stage := stages(value)[stageID].(map[string]any)
		field := "workflow_ref"
		if stage["kind"] == "repeat" {
			field = "body_workflow_ref"
		}
		stage[field] = map[string]any{"alias": name}
	}
	return encoded(t, value)
}

func TestWorkflowAliasesResolveNestedCalls(t *testing.T) {
	root, registry := callWorkflow(t, "test:workflow/alias-root")
	middle, _ := callWorkflow(t, "test:workflow/alias-middle")
	leaf, _ := callWorkflow(t, "test:workflow/alias-leaf")
	root.Definition.Entry, root.Definition.Stages["call"] = "call", callStage(Ref{}, "again")
	root.Definition.Stages["again"] = callStage(Ref{}, "done")
	middle.Definition.Entry, middle.Definition.Stages["call"] = "call", callStage(Ref{}, "done")
	aliases := map[string][]byte{"middle": aliasAuthor(t, middle, map[string]string{"call": "leaf"}), "leaf": encoded(t, leaf)}
	before := bytes.Clone(aliases["middle"])
	raw := aliasAuthor(t, root, map[string]string{"call": "middle", "again": "middle"})
	resolved, pinned, err := ResolveWorkflowAliases(raw, "json", registry, aliases)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(resolved, []byte(`"alias"`)) || !bytes.Equal(before, aliases["middle"]) || len(pinned) != len(registry)+2 {
		t.Fatal("resolver changed authoring inventory or retained mutable call selectors")
	}
	p, err := CompileProfile(resolved, "json", pinned, CoreProfile)
	if err != nil {
		t.Fatal("resolved aliases do not form an exact executable closure", err)
	}
	if p.Calls["call"] != p.Calls["again"] || p.Calls["call"].Calls["call"].Workflow.ID != leaf.ID || p.bounds.outcomes["no_work"].transitions != 13 {
		t.Fatal("resolved aliases lost nesting, definition reuse, or shared call costs")
	}
	for stageID, child := range p.Calls {
		ref := p.Workflow.Definition.Stages[stageID].WorkflowRef
		digest, err := Digest(pinned[ref])
		if err != nil || digest != ref.Digest || child.Digest != ref.Digest {
			t.Fatal("call was not pinned after resolving its children", err)
		}
	}
	_, err = Compile(resolved, "json", pinned)
	expectProblem(t, err, "unsupported")
}

func TestWorkflowAliasCyclesAndConflicts(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		edit       func(*WorkflowRevision, *WorkflowRevision, *WorkflowRevision, Registry, map[string][]byte) []byte
	}{
		{"direct cycle", "alias_cycle", func(r, a, _ *WorkflowRevision, _ Registry, aliases map[string][]byte) []byte {
			a.Definition.Entry, a.Definition.Stages["call"] = "call", callStage(Ref{}, "done")
			aliases["a"] = aliasAuthor(t, *a, map[string]string{"call": "a"})
			return aliasAuthor(t, *r, map[string]string{"call": "a"})
		}},
		{"transitive cycle", "alias_cycle", func(r, a, b *WorkflowRevision, _ Registry, aliases map[string][]byte) []byte {
			a.Definition.Entry, a.Definition.Stages["call"] = "call", callStage(Ref{}, "done")
			b.Definition.Entry, b.Definition.Stages["call"] = "call", callStage(Ref{}, "done")
			aliases["a"], aliases["b"] = aliasAuthor(t, *a, map[string]string{"call": "b"}), aliasAuthor(t, *b, map[string]string{"call": "a"})
			return aliasAuthor(t, *r, map[string]string{"call": "a"})
		}},
		{"different alias same active identity", "alias_cycle", func(r, a, b *WorkflowRevision, _ Registry, aliases map[string][]byte) []byte {
			a.Definition.Entry, a.Definition.Stages["call"] = "call", callStage(Ref{}, "done")
			b.ID = a.ID
			aliases["a"], aliases["b"] = aliasAuthor(t, *a, map[string]string{"call": "b"}), encoded(t, *b)
			return aliasAuthor(t, *r, map[string]string{"call": "a"})
		}},
		{"root identity recursion", "alias_cycle", func(r, a, _ *WorkflowRevision, _ Registry, aliases map[string][]byte) []byte {
			a.ID = r.ID
			aliases["a"] = encoded(t, *a)
			return aliasAuthor(t, *r, map[string]string{"call": "a"})
		}},
		{"unknown alias", "unknown_alias", func(r, _, _ *WorkflowRevision, _ Registry, _ map[string][]byte) []byte {
			return aliasAuthor(t, *r, map[string]string{"call": "missing"})
		}},
		{"invalid local name", "invalid_alias", func(r, _, _ *WorkflowRevision, _ Registry, _ map[string][]byte) []byte {
			return aliasAuthor(t, *r, map[string]string{"call": ""})
		}},
		{"mixed mutable and immutable selector", "invalid_alias", func(r, _, _ *WorkflowRevision, _ Registry, _ map[string][]byte) []byte {
			var value map[string]any
			_ = json.Unmarshal(aliasAuthor(t, *r, map[string]string{"call": "a"}), &value)
			stages(value)["call"].(map[string]any)["workflow_ref"].(map[string]any)["id"] = "test:workflow/other"
			return encoded(t, value)
		}},
		{"alias is not a step reference", "schema_invalid", func(r, _, _ *WorkflowRevision, _ Registry, _ map[string][]byte) []byte {
			var value map[string]any
			_ = json.Unmarshal(encoded(t, *r), &value)
			stages(value)["call"] = map[string]any{"kind": "step", "step_ref": map[string]any{"alias": "a"}, "input_bindings": map[string]any{}, "on": map[string]any{"pass": "done"}}
			return encoded(t, value)
		}},
		{"existing exact identity conflict", "ref_identity_conflict", func(r, a, _ *WorkflowRevision, registry Registry, _ map[string][]byte) []byte {
			old := *a
			old.Title = "Existing different immutable content"
			registerCallWorkflow(t, registry, old)
			return aliasAuthor(t, *r, map[string]string{"call": "a"})
		}},
		{"child control literal before JCS", "condition_type_mismatch", func(r, a, _ *WorkflowRevision, _ Registry, aliases map[string][]byte) []byte {
			a.Definition.Entry = "choose"
			a.Definition.Stages["choose"] = Stage{Kind: "choice", Selection: "first_match", Branches: []ChoiceBranch{{ID: "one", Predicate: Predicate{Op: "eq", Left: conditionLiteral("1.0000000000000000001"), Right: conditionLiteral("1")}, Next: "done"}}}
			aliases["a"] = encoded(t, *a)
			return aliasAuthor(t, *r, map[string]string{"call": "a"})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, registry := callWorkflow(t, "test:workflow/alias-root")
			a, _ := callWorkflow(t, "test:workflow/alias-a")
			b, _ := callWorkflow(t, "test:workflow/alias-b")
			root.Definition.Entry, root.Definition.Stages["call"] = "call", callStage(Ref{}, "done")
			aliases := map[string][]byte{"a": encoded(t, a)}
			raw := tc.edit(&root, &a, &b, registry, aliases)
			_, _, err := ResolveWorkflowAliases(raw, "json", registry, aliases)
			expectProblem(t, err, tc.code)
		})
	}
}

func TestWorkflowAliasesDoNotRewriteDataOrUnchangedRoots(t *testing.T) {
	root, registry := callWorkflow(t, "test:workflow/data-alias")
	root.Limits.MaxChildDepth = 0
	schema := []byte(`true`)
	digest, _ := Digest(schema)
	ref := Ref{ID: "test:schema/data-alias", Version: "1.0.0", Digest: digest}
	registry[ref] = schema
	data := json.RawMessage(`{"alias":"missing","definition":{"stages":{"call":{"kind":"call","workflow_ref":{"alias":"missing"}}}}}`)
	root.Outputs = map[string]OutputPort{"data": {Port: Port{Format: "json", SchemaRef: &ref}, RequiredFor: []string{"no_work"}}}
	root.Definition.Stages["done"] = Stage{Kind: "finish", Outcome: "no_work", OutputBindings: map[string]Binding{"data": {From: "literal", SchemaRef: &ref, Value: data}}}
	raw := append([]byte(" \n"), encoded(t, root)...)
	var object any
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatal(err)
	}
	yamlBytes, err := yaml.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	for format, raw := range map[string][]byte{"json": raw, "yaml": yamlBytes} {
		t.Run(format, func(t *testing.T) {
			resolved, pinned, err := ResolveWorkflowAliases(raw, format, registry, map[string][]byte{"unused": []byte(`not a workflow`)})
			if err != nil || !bytes.Equal(resolved, raw) || len(pinned) != len(registry) {
				t.Fatalf("data/unchanged bytes rewritten: %v", err)
			}
			if _, err := Compile(resolved, format, pinned); err != nil {
				t.Fatal("alias-looking data no longer works in F1", err)
			}
		})
	}
}

func TestWorkflowAliasInventoryAndDepthBounds(t *testing.T) {
	root, registry := callWorkflow(t, "test:workflow/alias-bounds")
	aliases := make(map[string][]byte)
	for i := 0; i < 1025; i++ {
		aliases[fmt.Sprintf("alias%d", i)] = nil
	}
	_, _, err := ResolveWorkflowAliases(encoded(t, root), "json", registry, aliases)
	expectProblem(t, err, "dependency_limit")
	aliases = make(map[string][]byte)
	for i := 0; i < 65; i++ {
		child := root
		child.ID = fmt.Sprintf("test:workflow/depth%d", i)
		child.Definition.Stages = map[string]Stage{"done": {Kind: "finish", Outcome: "no_work", OutputBindings: map[string]Binding{}}, "call": callStage(Ref{}, "done")}
		child.Definition.Entry = "call"
		aliases[fmt.Sprintf("alias%d", i)] = aliasAuthor(t, child, map[string]string{"call": fmt.Sprintf("alias%d", i+1)})
	}
	root.Definition.Entry, root.Definition.Stages["call"] = "call", callStage(Ref{}, "done")
	_, _, err = ResolveWorkflowAliases(aliasAuthor(t, root, map[string]string{"call": "alias0"}), "json", registry, aliases)
	expectProblem(t, err, "dependency_limit")
}
