package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func compositionEdit(t *testing.T, root, path string, edit func(map[string]any)) {
	t.Helper()
	value, err := projectYAMLDocument(filepath.Join(root, path))
	if err != nil {
		t.Fatal(err)
	}
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatal("fixture document is not an object")
	}
	edit(object)
	data, err := json.MarshalIndent(object, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, root, path, string(data))
}

func compositionFaultCode(err error) string {
	var fault *prifly.Fault
	if errors.As(err, &fault) {
		return fault.Code
	}
	return ""
}

func TestProjectCompositionRefusesSourceAndLayerHazards(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, code string
		change     func(*testing.T, string)
	}{
		{"unknown package", "project_composition_unknown_package", func(t *testing.T, root string) {
			writeFixtureFile(t, root, ".prifly/workflows/cycle/extend.yaml", strings.Replace(compositionImportExtension, "review-library", "not-registered", 1))
		}},
		{"wrong kind", "project_composition_export_kind", func(t *testing.T, root string) {
			compositionEdit(t, root, ".prifly/workflows/review-library/workflow.yaml", func(value map[string]any) {
				value["exports"] = map[string]any{"steps": map[string]any{"review-for-change": "{{step_tests}}"}}
			})
		}},
		{"origin drift", "project_composition_source_drift", func(t *testing.T, root string) {
			compositionEdit(t, root, ".prifly/project.yaml", func(value map[string]any) {
				value["packages"].(map[string]any)["review-library"].(map[string]any)["origin"] = map[string]any{"repository": "https://github.com/example/workflows.git", "path": "review-library", "ref": "v1.0.0", "commit": strings.Repeat("a", 40), "digest": "sha256:" + strings.Repeat("b", 64)}
			})
		}},
		{"import cycle", "project_composition_cycle", func(t *testing.T, root string) {
			compositionEdit(t, root, ".prifly/workflows/review-library/workflow.yaml", func(value map[string]any) { value["imports"] = map[string]any{"back": "cycle"} })
		}},
		{"repeated layer", "project_composition_duplicate_layer", func(t *testing.T, root string) {
			writeFixtureFile(t, root, "shared/layer.yaml", "authoring: prifly-extension/3\n")
			writeFixtureFile(t, root, ".prifly/workflows/cycle/extend.yaml", "authoring: prifly-extension/3\nlayers: [../../../shared/layer.yaml, ../../../shared/./layer.yaml]\n")
		}},
		{"nested layer", "project_composition_invalid_layer", func(t *testing.T, root string) {
			writeFixtureFile(t, root, "shared/layer.yaml", "authoring: prifly-extension/3\nlayers: [other.yaml]\n")
			writeFixtureFile(t, root, ".prifly/workflows/cycle/extend.yaml", "authoring: prifly-extension/3\nlayers: [../../../shared/layer.yaml]\n")
		}},
		{"symlink layer", "project_composition_source_invalid", func(t *testing.T, root string) {
			outside := filepath.Join(t.TempDir(), "layer.yaml")
			writeFixtureFile(t, filepath.Dir(outside), "layer.yaml", "authoring: prifly-extension/3\n")
			if err := os.Symlink(outside, filepath.Join(root, "layer.yaml")); err != nil {
				t.Fatal(err)
			}
			writeFixtureFile(t, root, ".prifly/workflows/cycle/extend.yaml", "authoring: prifly-extension/3\nlayers: [../../../layer.yaml]\n")
		}},
		{"layer escape", "project_workflow_folder_invalid", func(t *testing.T, root string) {
			writeFixtureFile(t, root, ".prifly/workflows/cycle/extend.yaml", "authoring: prifly-extension/3\nlayers: [../../../../outside.yaml]\n")
		}},
		{"unrelated parents", "project_composition_incompatible_bases", func(t *testing.T, root string) {
			if err := os.Remove(filepath.Join(root, ".prifly/workflows/review-library/extend.yaml")); err != nil {
				t.Fatal(err)
			}
			registerCompositionDerived(t, root, "unrelated", "[cycle, review-library]", "")
		}},
		{"structural override", "project_composition_structural_override", func(t *testing.T, root string) {
			registerCompositionDerived(t, root, "unrelated", "cycle", "")
			compositionEdit(t, root, ".prifly/workflows/unrelated/workflow.yaml", func(value map[string]any) {
				value["stages"] = map[string]any{"done": map[string]any{"kind": "finish", "outcome": "succeeded"}}
			})
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root, _ := writeCompositionFixture(t)
			test.change(t, root)
			name := "cycle"
			if test.name == "unrelated parents" || test.name == "structural override" {
				name = "unrelated"
			}
			_, err := readProjectWorkflowFolder(root, filepath.Join(root, ".prifly/workflows", name))
			if compositionFaultCode(err) != test.code {
				t.Fatalf("want %s; got %v", test.code, err)
			}
		})
	}
}

func TestProjectCompositionLayersReplaceSettingsAtomicallyAndExplain(t *testing.T) {
	t.Parallel()
	root, authority := writeCompositionFixture(t)
	compositionEdit(t, root, ".prifly/workflows/review-library/project/workflows/z-review.yaml", func(value map[string]any) {
		value["inputs"].(map[string]any)["browser"] = map[string]any{"schema_ref": "handoff", "required": false, "configuration": map[string]any{"scope": "project", "default": map[string]any{"width": 800, "height": 600}}}
	})
	writeFixtureFile(t, root, "shared/delivery.yaml", "authoring: prifly-extension/3\nimports: {ui: review-library}\nsettings:\n  ui.review-for-change:\n    browser: {width: 1024, height: 768, flags: [layer]}\n")
	options := strings.Replace(compositionImportExtension, "imports: {ui: review-library}\n", "layers: [../../../shared/delivery.yaml]\nsettings:\n  ui.review-for-change:\n    browser: {width: 1280}\n", 1)
	writeFixtureFile(t, root, ".prifly/workflows/cycle/extend.yaml", options)
	compiled := compileCompositionName(t, root, authority, "cycle")
	for _, component := range compiled.Components {
		if component.Ref.ID == "test:workflow/review" {
			data, err := os.ReadFile(filepath.Join(compiled.Output, component.Path))
			if err != nil {
				t.Fatal(err)
			}
			var workflow flow.WorkflowRevision
			if err := json.Unmarshal(data, &workflow); err != nil {
				t.Fatal(err)
			}
			if string(workflow.Inputs["browser"].Configuration.Default) != "{\"width\":1280}" {
				t.Fatalf("object was recursively merged: %s", workflow.Inputs["browser"].Configuration.Default)
			}
		}
	}
	code, out, stderr := runCLI(t, "--project", authority, "project", "compile", "--repository", root, "--package", "cycle", "--explain")
	if code != 0 {
		t.Fatal(stderr)
	}
	if !strings.Contains(out, "shared/delivery.yaml/settings/ui.review-for-change/browser") || !strings.Contains(out, "extend.yaml/settings/ui.review-for-change/browser") {
		t.Fatalf("missing setting provenance: %s", out)
	}
}

func TestProjectCompositionParentDisagreementRequiresConsumerValue(t *testing.T) {
	t.Parallel()
	for _, reverse := range []bool{false, true} {
		t.Run(map[bool]string{false: "original", true: "reversed"}[reverse], func(t *testing.T) {
			t.Parallel()
			root, _ := writeCompositionFixture(t)
			registerCompositionDerived(t, root, "left", "cycle", "authoring: prifly-extension/3\nresources: {machine: {limit: 1, stages: [build]}}\n")
			registerCompositionDerived(t, root, "right", "cycle", "authoring: prifly-extension/3\nresources: {machine: {limit: 2, stages: [build]}}\n")
			parents := "[left, right]"
			if reverse {
				parents = "[right, left]"
			}
			registerCompositionDerived(t, root, "delivery", parents, "")
			folder := filepath.Join(root, ".prifly/workflows/delivery")
			_, err := readProjectWorkflowFolder(root, folder)
			if compositionFaultCode(err) != "project_composition_setting_conflict" {
				t.Fatalf("implicit parent order: %v", err)
			}
			writeFixtureFile(t, root, ".prifly/workflows/delivery/extend.yaml", "authoring: prifly-extension/3\nresources: {machine: {limit: 3, stages: [build]}}\n")
			if _, err := readProjectWorkflowFolder(root, folder); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestProjectCompositionEmittedContracts(t *testing.T) {
	t.Parallel()
	root, authority := writeCompositionFixture(t)
	compiled := compileCompositionName(t, root, authority, "cycle")
	data, err := json.Marshal(compiled)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := prifly.PublicSchema("ProjectCompileV3")
	if err != nil {
		t.Fatal(err)
	}
	digest, err := flow.Digest(schema)
	if err != nil {
		t.Fatal(err)
	}
	ref := flow.Ref{ID: "test:schema/composition-compile", Version: "1.0.0", Digest: digest}
	if err := flow.ValidateSchema(flow.Registry{ref: schema}, ref, data); err != nil {
		t.Fatal(err)
	}
	for _, marker := range []struct{ name, source string }{{"project-workflow-folder-v2", strings.Replace(extensionInputWorkflow, "prifly-project-workflow/1", "prifly-project-workflow/2", 1)}, {"extension-v3", "authoring: prifly-extension/3\nimports: {ui: review-library}\n"}} {
		value, err := flow.Parse([]byte(marker.source), "yaml")
		if err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(value)
		if err := validateAuthoringForTest(t, marker.name, encoded); err != nil {
			t.Fatal(err)
		}
		object := value.(map[string]any)
		object["typo"] = true
		encoded, _ = json.Marshal(object)
		if err := validateAuthoringForTest(t, marker.name, encoded); err == nil {
			t.Fatal("closed authoring schema accepted unknown field")
		}
	}
}

func TestProjectCompositionNestedLibraryIncludesCheckAndContext(t *testing.T) {
	t.Parallel()
	root, authority := writeCompositionFixture(t)
	profile, err := os.ReadFile(filepath.Join(root, ".prifly/project.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, root, ".prifly/project.yaml", strings.Replace(string(profile), "launches:", "  audit-library: {source: .prifly/workflows/audit-library}\nlaunches:", 1))
	provider := strings.ReplaceAll(extensionInputWorkflow, "prifly-project-workflow/1", "prifly-project-workflow/2")
	provider = strings.ReplaceAll(provider, "test:package/cycle", "audit:package/library")
	provider = strings.ReplaceAll(provider, "test:workflow/cycle", "audit:workflow/root")
	provider += "exports:\n  steps: {checked: \"{{step_tests}}\"}\n"
	writeFixtureFile(t, root, ".prifly/workflows/audit-library/workflow.yaml", provider)
	for _, path := range []string{"schemas/handoff.yaml", "steps/tests.yaml"} {
		data, err := os.ReadFile(filepath.Join(root, ".prifly/workflows/review-library", path))
		if err != nil {
			t.Fatal(err)
		}
		writeFixtureFile(t, root, ".prifly/workflows/audit-library/"+path, string(data))
	}
	compositionEdit(t, root, ".prifly/workflows/audit-library/workflow.yaml", func(value map[string]any) {
		value["package"].(map[string]any)["references"].(map[string]any)["process"] = "core:adapter/local-process@2.0.0"
	})
	compositionEdit(t, root, ".prifly/workflows/audit-library/steps/tests.yaml", func(value map[string]any) {
		value["result_check_refs"] = []any{"{{check_validation}}"}
		value["instructions_ref"] = "{{context_notes}}"
	})
	writeFixtureFile(t, root, ".prifly/workflows/audit-library/checks/validation.yaml", "schema_version: check-definition/1\nid: audit:check/validation\nversion: 1.0.0\ntitle: Synthetic result check\nkind: result\nclaim: semantic_review\nexecutor: {adapter_ref: \"{{process}}\", operation: check}\n")
	writeFixtureFile(t, root, ".prifly/workflows/audit-library/contexts/notes.yaml", "id: audit:context/notes\nversion: 1.0.0\nmedia_type: text/plain\ntext: Synthetic imported instructions.\n")
	compositionEdit(t, root, ".prifly/workflows/review-library/workflow.yaml", func(value map[string]any) { value["imports"] = map[string]any{"audit": "audit-library"} })
	compositionEdit(t, root, ".prifly/workflows/review-library/project/workflows/z-review.yaml", func(value map[string]any) { value["refs"].(map[string]any)["tests"] = "audit.checked" })
	compiled := compileCompositionName(t, root, authority, "cycle")
	counts := map[string]int{}
	for _, component := range compiled.Components {
		counts[component.Ref.ID]++
	}
	if counts["audit:check/validation"] != 1 || counts["audit:context/notes"] != 1 || counts["test:step/tests"] != 1 || counts["audit:workflow/root"] != 0 {
		t.Fatalf("wrong nested closure: %v", counts)
	}
}

func TestProjectCompositionIdentityConflictAndNamespacedShortNames(t *testing.T) {
	t.Parallel()
	t.Run("identity conflict", func(t *testing.T) {
		t.Parallel()
		root, authority := writeCompositionFixture(t)
		compositionEdit(t, root, ".prifly/workflows/review-library/schemas/handoff.yaml", func(value map[string]any) { value["description"] = "Different bytes under the same author identity" })
		_, _, problem := compileCallExtensionFixture(t, root, authority)
		if problem == nil || problem.Code != "project_compile_duplicate_component" {
			t.Fatalf("identity conflict hidden: %+v", problem)
		}
	})
	t.Run("same short name", func(t *testing.T) {
		t.Parallel()
		root, authority := writeCompositionFixture(t)
		compositionEdit(t, root, ".prifly/workflows/cycle/steps/build.yaml", func(value map[string]any) { value["id"] = "consumer:step/tests" })
		compositionEdit(t, root, ".prifly/workflows/cycle/workflow.yaml", func(value map[string]any) { value["refs"].(map[string]any)["step_build"] = "{{step_tests}}" })
		compiled := compileCompositionName(t, root, authority, "cycle")
		counts := map[string]int{}
		for _, component := range compiled.Components {
			counts[component.Ref.ID]++
		}
		if counts["consumer:step/tests"] != 1 || counts["test:step/tests"] != 1 {
			t.Fatalf("namespaces mixed: %v", counts)
		}
	})
}

func TestProjectCompositionStagingIsInertWithoutAProjectProfile(t *testing.T) {
	t.Parallel()
	root, _ := writeCompositionFixture(t)
	target := t.TempDir()
	staging, err := stageProjectWorkflowFolder(target, filepath.Join(root, ".prifly/workflows/review-library"), "library")
	if err != nil {
		t.Fatal(err)
	}
	defer staging.discard()
	inspection, err := inspectProjectWorkflowFolder(staging.root, staging.folder)
	if err != nil {
		t.Fatalf("folder /2 cannot be installed in an inert staging mirror: %v", err)
	}
	if inspection.source.ID != "test:package/review-library" {
		t.Fatalf("wrong source identity: %s", inspection.source.ID)
	}
	if _, err := os.Stat(filepath.Join(staging.root, ".prifly/project.yaml")); !os.IsNotExist(err) {
		t.Fatal("inspection created a project profile")
	}
}

func TestProjectCompositionPrepareAndStartMatchPublishedViews(t *testing.T) {
	t.Parallel()
	root, authority := writeCompositionFixture(t)
	for folder, files := range map[string][]string{"cycle": {"steps/build.yaml", "steps/commit.yaml"}, "review-library": {"steps/tests.yaml"}} {
		prefix := ".prifly/workflows/" + folder + "/"
		writeFixtureFile(t, root, prefix+"contexts/notes.yaml", "id: test:context/notes\nversion: 1.0.0\nmedia_type: text/plain\ntext: Synthetic schema-view test.\n")
		for _, file := range files {
			compositionEdit(t, root, prefix+file, func(value map[string]any) { value["instructions_ref"] = "{{context_notes}}" })
		}
	}
	compositionEdit(t, root, ".prifly/project.yaml", func(value map[string]any) {
		value["launches"] = map[string]any{"cycle": map[string]any{"title": "Composition view fixture", "description": "Synthetic prepare/start contracts.", "kind": "workflow", "workflow": ".prifly/workflows/cycle/workflow.yaml"}}
	})
	if code, _, stderr := runCLI(t, "project", "runners", "add", "--repository", root, "--host", "codex-app"); code != 0 {
		t.Fatal(stderr)
	}
	command := func(args ...string) []byte {
		t.Helper()
		code, out, stderr := runCLI(t, append([]string{"--project", authority}, args...)...)
		if code != 0 {
			t.Fatal(stderr)
		}
		return []byte(out)
	}
	validate := func(name string, data []byte) {
		t.Helper()
		schema, err := prifly.PublicSchema(name)
		if err != nil {
			t.Fatal(err)
		}
		digest, err := flow.Digest(schema)
		if err != nil {
			t.Fatal(err)
		}
		ref := flow.Ref{ID: "test:schema/composition-view", Version: "1.0.0", Digest: digest}
		if err := flow.ValidateSchema(flow.Registry{ref: schema}, ref, data); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	args := []string{"--repository", root, "--launch", "cycle", "--host", "codex-app"}
	prepared := command(append([]string{"project", "questionnaire", "--prepare"}, args...)...)
	validate("ProjectLaunchSummaryV6", prepared)
	var summary projectLaunchSummary
	if err := json.Unmarshal(prepared, &summary); err != nil {
		t.Fatal(err)
	}
	started := command(append(append([]string{"project", "start"}, args...), "--expected-launch-digest", summary.ReviewDigest)...)
	validate("ProjectStartV6", started)
}

func TestProjectCompositionPackageProfileConflictHasNoParentOrder(t *testing.T) {
	t.Parallel()
	root, _ := writeCompositionFixture(t)
	for _, name := range []string{"left", "right"} {
		registerCompositionDerived(t, root, name, "cycle", "")
		compositionEdit(t, root, ".prifly/workflows/"+name+"/workflow.yaml", func(value map[string]any) {
			value["package"].(map[string]any)["profiles"] = map[string]any{"default": "fast", "values": map[string]any{"fast": map[string]any{"gate_mode": name}}}
		})
	}
	registerCompositionDerived(t, root, "delivery", "[left, right]", "")
	folder := filepath.Join(root, ".prifly/workflows/delivery")
	if _, err := readProjectWorkflowFolder(root, folder); compositionFaultCode(err) != "project_composition_setting_conflict" {
		t.Fatalf("profile declaration chose the first parent: %v", err)
	}
	compositionEdit(t, root, ".prifly/workflows/delivery/workflow.yaml", func(value map[string]any) {
		value["package"].(map[string]any)["profiles"] = map[string]any{"default": "fast", "values": map[string]any{"fast": map[string]any{"gate_mode": "explicit"}}}
	})
	if _, err := readProjectWorkflowFolder(root, folder); err != nil {
		t.Fatal(err)
	}
}

func TestProjectCompositionDoesNotResolveQualifiedRefsInsideData(t *testing.T) {
	t.Parallel()
	root, authority := writeCompositionFixture(t)
	literal := map[string]any{"refs": map[string]any{"url": "example.com"}, "record": map[string]any{"id": "literal:workflow/record", "version": "1.0.0", "digest": "sha256:" + strings.Repeat("a", 64)}}
	compositionEdit(t, root, ".prifly/workflows/review-library/schemas/handoff.yaml", func(value map[string]any) { value["default"] = literal })
	compositionEdit(t, root, ".prifly/workflows/review-library/project/workflows/z-review.yaml", func(value map[string]any) {
		value["inputs"].(map[string]any)["data"] = map[string]any{"schema_ref": "handoff", "required": false, "configuration": map[string]any{"scope": "project", "default": literal}}
	})
	// Both providers use the same schema identity, so keep their source bytes equal.
	compositionEdit(t, root, ".prifly/workflows/cycle/schemas/handoff.yaml", func(value map[string]any) { value["default"] = literal })
	compiled := compileCompositionName(t, root, authority, "cycle")
	for _, component := range compiled.Components {
		if component.Ref.ID != "test:workflow/review" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(compiled.Output, component.Path))
		if err != nil {
			t.Fatal(err)
		}
		var workflow flow.WorkflowRevision
		if err := json.Unmarshal(data, &workflow); err != nil {
			t.Fatal(err)
		}
		expected, _ := projectCanonicalJSON(literal)
		if string(workflow.Inputs["data"].Configuration.Default) != string(expected) {
			t.Fatalf("instance data was rewritten: %s", workflow.Inputs["data"].Configuration.Default)
		}
	}
}
