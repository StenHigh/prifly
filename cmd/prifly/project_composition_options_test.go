package main

import (
	"path/filepath"
	"testing"
)

const compositionBaseOptions = `authoring: prifly-extension/3
resources: {machine: {limit: 1, stages: [build]}}
settings: {root: {browser: {width: 800, height: 600}}}
answers:
  decision_policy: attended
  preflight: {approve: {old: 1}}
  runtime: {confirm: 1}
model_profiles: {codex-app: {quick: {model: base, effort: high}}}
profile: base
exclude: [optional]
`
const compositionLeftOptions = `authoring: prifly-extension/3
resources: {machine: {limit: 2, stages: [build]}}
settings: {root: {browser: {width: 1280}}}
answers:
  decision_policy: autonomous
  preflight: {approve: {new: 2}}
  runtime: {confirm: 2}
model_profiles: {codex-app: {quick: {model: left}}}
profile: left
exclude: []
`

func TestProjectCompositionMergesAncestryOptionDeclarations(t *testing.T) {
	t.Parallel()
	for _, diamond := range []bool{false, true} {
		for _, reverse := range []bool{false, true} {
			t.Run(map[bool]string{false: "ancestor", true: "diamond"}[diamond]+map[bool]string{false: "/forward", true: "/reverse"}[reverse], func(t *testing.T) {
				t.Parallel()
				root, _ := writeCompositionFixture(t)
				writeFixtureFile(t, root, ".prifly/workflows/cycle/extend.yaml", compositionBaseOptions)
				registerCompositionDerived(t, root, "left", "cycle", compositionLeftOptions)
				other := "cycle"
				if diamond {
					registerCompositionDerived(t, root, "right", "cycle", "")
					other = "right"
				}
				parents := "[left, " + other + "]"
				if reverse {
					parents = "[" + other + ", left]"
				}
				registerCompositionDerived(t, root, "consumer", parents, "")
				source, err := readProjectWorkflowFolder(root, filepath.Join(root, ".prifly/workflows/consumer"))
				if err != nil {
					t.Fatal(err)
				}
				options := source.Composition.Options
				if options.Resources["machine"].Limit != 2 {
					t.Fatalf("resource override lost: %+v", options.Resources)
				}
				target := ""
				for _, doc := range source.Documents {
					if doc.FolderRoot {
						target = doc.Name
					}
				}
				if len(options.Settings) != 1 || !projectCompositionEqual(options.Settings[target]["browser"], map[string]any{"width": 1280}) {
					t.Fatalf("root settings were not replaced/remapped: %+v", options.Settings)
				}
				if string(options.Answers.Preflight["approve"]) != "{\"new\":2}" || string(options.Answers.Runtime["confirm"]) != "2" || options.Answers.DecisionPolicy != "autonomous" {
					t.Fatalf("answers did not preserve branch declaration: %+v", options.Answers)
				}
				if !projectCompositionEqual(options.ModelProfiles["codex-app"]["quick"], map[string]string{"model": "left"}) {
					t.Fatalf("opaque model profile merged fields: %+v", options.ModelProfiles)
				}
				if options.Profile != "left" {
					t.Fatalf("wrong selected profile: %s", options.Profile)
				}
				if options.Exclude == nil || len(options.Exclude) != 0 {
					t.Fatalf("explicit empty exclude lost: %#v", options.Exclude)
				}
			})
		}
	}
}

func TestProjectCompositionIndependentDeclarationsRequireResolution(t *testing.T) {
	t.Parallel()
	for _, rightLimit := range []string{"1", "2", "3"} {
		for _, reverse := range []bool{false, true} {
			t.Run(rightLimit+map[bool]string{false: "/forward", true: "/reverse"}[reverse], func(t *testing.T) {
				t.Parallel()
				root, _ := writeCompositionFixture(t)
				writeFixtureFile(t, root, ".prifly/workflows/cycle/extend.yaml", "authoring: prifly-extension/3\nresources: {machine: {limit: 1, stages: [build]}}\n")
				registerCompositionDerived(t, root, "left", "cycle", "authoring: prifly-extension/3\nresources: {machine: {limit: 2, stages: [build]}}\n")
				registerCompositionDerived(t, root, "right", "cycle", "authoring: prifly-extension/3\nresources: {machine: {limit: "+rightLimit+", stages: [build]}}\n")
				parents := "[left, right]"
				if reverse {
					parents = "[right, left]"
				}
				registerCompositionDerived(t, root, "consumer", parents, "")
				folder := filepath.Join(root, ".prifly/workflows/consumer")
				source, err := readProjectWorkflowFolder(root, folder)
				if rightLimit == "2" {
					if err != nil {
						t.Fatal(err)
					}
					if source.Composition.Options.Resources["machine"].Limit != 2 {
						t.Fatal("equal declarations changed value")
					}
					return
				}
				if compositionFaultCode(err) != "project_composition_setting_conflict" {
					t.Fatalf("independent declaration was hidden: %v", err)
				}
				writeFixtureFile(t, root, ".prifly/workflows/consumer/extend.yaml", "authoring: prifly-extension/3\nresources: {machine: {limit: 4, stages: [build]}}\n")
				source, err = readProjectWorkflowFolder(root, folder)
				if err != nil {
					t.Fatal(err)
				}
				if source.Composition.Options.Resources["machine"].Limit != 4 {
					t.Fatal("consumer resolution lost")
				}
			})
		}
	}
}

func TestProjectCompositionPackageDefaultHasOneDeclaration(t *testing.T) {
	t.Parallel()
	root, _ := writeCompositionFixture(t)
	writeFixtureFile(t, root, ".prifly/workflows/cycle/extend.yaml", "authoring: prifly-extension/3\n")
	compositionEdit(t, root, ".prifly/workflows/cycle/workflow.yaml", func(value map[string]any) {
		value["package"].(map[string]any)["profiles"] = map[string]any{"default": "base", "values": map[string]any{"base": map[string]any{"label": "base"}, "left": map[string]any{"label": "left"}}}
	})
	registerCompositionDerived(t, root, "left", "cycle", "authoring: prifly-extension/3\nprofile: left\n")
	registerCompositionDerived(t, root, "right", "cycle", "")
	registerCompositionDerived(t, root, "consumer", "[right, left]", "")
	source, err := readProjectWorkflowFolder(root, filepath.Join(root, ".prifly/workflows/consumer"))
	if err != nil {
		t.Fatal(err)
	}
	if source.Composition.Options.Profile != "left" {
		t.Fatal("inherited package default replaced descendant profile")
	}
}

func TestProjectCompositionReferenceDeclarationsRemainUnique(t *testing.T) {
	t.Parallel()
	for _, common := range []bool{false, true} {
		t.Run(map[bool]string{false: "independent", true: "common"}[common], func(t *testing.T) {
			t.Parallel()
			root, _ := writeCompositionFixture(t)
			base := "authoring: prifly-extension/3\n"
			left, right := "", ""
			const reference = "references: {extra: core:adapter/assisted-session@1.0.0}\n"
			if common {
				base += reference
			} else {
				left = "authoring: prifly-extension/3\n" + reference
				right = "authoring: prifly-extension/3\nreferences: {extra: core:adapter/local-process@2.0.0}\n"
			}
			writeFixtureFile(t, root, ".prifly/workflows/cycle/extend.yaml", base)
			registerCompositionDerived(t, root, "left", "cycle", left)
			registerCompositionDerived(t, root, "right", "cycle", right)
			registerCompositionDerived(t, root, "consumer", "[left, right]", "")
			folder := filepath.Join(root, ".prifly/workflows/consumer")
			source, err := readProjectWorkflowFolder(root, folder)
			if common {
				if err != nil {
					t.Fatal(err)
				}
				if source.Composition.Options.References["extra"] != "core:adapter/assisted-session@1.0.0" {
					t.Fatal("shared reference lost")
				}
			} else if compositionFaultCode(err) != "project_composition_setting_conflict" {
				t.Fatalf("independent references must refuse: %v", err)
			}
			writeFixtureFile(t, root, ".prifly/workflows/consumer/extend.yaml", "authoring: prifly-extension/3\n"+reference)
			_, err = readProjectWorkflowFolder(root, folder)
			expected := "project_composition_setting_conflict"
			if common {
				expected = "project_composition_reference_conflict"
			}
			if compositionFaultCode(err) != expected {
				t.Fatalf("consumer cannot override reference uniqueness: %v", err)
			}
		})
	}
}

func TestProjectCompositionRetainsIndependentSettingOrigins(t *testing.T) {
	t.Parallel()
	root, _ := writeCompositionFixture(t)
	writeFixtureFile(t, root, ".prifly/workflows/cycle/extend.yaml", "authoring: prifly-extension/3\nsettings: {root: {browser: {width: 800}}}\n")
	for _, name := range []string{"left", "right"} {
		registerCompositionDerived(t, root, name, "cycle", "authoring: prifly-extension/3\nsettings: {root: {browser: {width: 1280}}}\n")
	}
	registerCompositionDerived(t, root, "consumer", "[right, left]", "")
	source, err := readProjectWorkflowFolder(root, filepath.Join(root, ".prifly/workflows/consumer"))
	if err != nil {
		t.Fatal(err)
	}
	if len(source.Composition.Settings) != 3 {
		t.Fatalf("equal independent origins were removed: %+v", source.Composition.Settings)
	}
	writeFixtureFile(t, root, ".prifly/workflows/right/extend.yaml", "authoring: prifly-extension/3\nsettings: {root: {browser: {width: 800}}}\n")
	_, err = readProjectWorkflowFolder(root, filepath.Join(root, ".prifly/workflows/consumer"))
	if compositionFaultCode(err) != "project_composition_setting_conflict" {
		t.Fatalf("explicit reset is an independent declaration: %v", err)
	}
	writeFixtureFile(t, root, ".prifly/workflows/consumer/extend.yaml", "authoring: prifly-extension/3\nsettings: {root: {browser: {height: 900}}}\n")
	source, err = readProjectWorkflowFolder(root, filepath.Join(root, ".prifly/workflows/consumer"))
	if err != nil {
		t.Fatal(err)
	}
	for _, values := range source.Composition.Options.Settings {
		if !projectCompositionEqual(values["browser"], map[string]any{"height": 900}) {
			t.Fatal("consumer object resolution retained inherited fields")
		}
	}
	if len(source.Composition.Settings) != 4 {
		t.Fatal("resolved conflict lost source history")
	}
}
