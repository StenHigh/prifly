package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func compositionProgramConfig(marker string) map[string]any {
	return map[string]any{"executable": "shell", "args": []any{marker}, "timeout_ms": 30000, "grace_ms": 100, "max_output_bytes": 65536}
}

func compositionProcessStep(t *testing.T, root, folder, name string) {
	t.Helper()
	compositionEdit(t, root, ".prifly/workflows/"+folder+"/workflow.yaml", func(v map[string]any) {
		v["package"].(map[string]any)["references"].(map[string]any)["process"] = "core:adapter/local-process@2.0.0"
	})
	compositionEdit(t, root, ".prifly/workflows/"+folder+"/steps/"+name+".yaml", func(v map[string]any) {
		v["executor"] = map[string]any{"adapter_ref": "{{process}}", "operation": "process"}
	})
}

func compositionSourceProgram(t *testing.T, root, folder, group, id string, config map[string]any) {
	t.Helper()
	compositionEdit(t, root, ".prifly/workflows/"+folder+"/workflow.yaml", func(v map[string]any) {
		v["execution_bindings"] = map[string]any{group: map[string]any{id: config}}
	})
}

func TestProjectCompositionProgramsRequireSourceOwnership(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"provider-to-consumer-step", "consumer-to-provider-step", "provider-to-consumer-check", "wrong-kind"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			root, authority := writeCompositionFixture(t)
			owner, group, id := "review-library", "steps", "test:step/build"
			switch kind {
			case "provider-to-consumer-step":
				compositionProcessStep(t, root, "cycle", "build")
			case "consumer-to-provider-step":
				compositionProcessStep(t, root, "review-library", "tests")
				owner, id = "cycle", "test:step/tests"
			case "provider-to-consumer-check":
				compositionEdit(t, root, ".prifly/workflows/cycle/workflow.yaml", func(v map[string]any) {
					v["package"].(map[string]any)["references"].(map[string]any)["process"] = "core:adapter/local-process@2.0.0"
				})
				compositionEdit(t, root, ".prifly/workflows/cycle/steps/build.yaml", func(v map[string]any) { v["result_check_refs"] = []any{"{{check_validation}}"} })
				writeFixtureFile(t, root, ".prifly/workflows/cycle/checks/validation.yaml", "schema_version: check-definition/1\nid: test:check/validation\nversion: 1.0.0\ntitle: Synthetic check\nkind: result\nclaim: semantic_review\nexecutor: {adapter_ref: \"{{process}}\", operation: check}\n")
				group, id = "checks", "test:check/validation"
			case "wrong-kind":
				group, id = "checks", "test:step/tests"
			}
			compositionSourceProgram(t, root, owner, group, id, compositionProgramConfig("FOREIGN_PROGRAM"))
			_, output, problem := compileCallExtensionFixture(t, root, authority)
			if problem == nil || problem.Code != "project_execution_invalid" {
				t.Fatalf("foreign %s binding accepted: %+v", kind, problem)
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatalf("refusal left package: %v", err)
			}
		})
	}
}

func TestProjectCompositionUnselectedProgramCannotBindForeignIdentity(t *testing.T) {
	t.Parallel()
	for _, differentVersion := range []bool{false, true} {
		t.Run(map[bool]string{false: "different-bytes", true: "different-version"}[differentVersion], func(t *testing.T) {
			t.Parallel()
			root, authority := writeCompositionFixture(t)
			compositionProcessStep(t, root, "cycle", "build")
			data, err := os.ReadFile(filepath.Join(root, ".prifly/workflows/cycle/steps/build.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			writeFixtureFile(t, root, ".prifly/workflows/review-library/steps/build.yaml", string(data))
			compositionEdit(t, root, ".prifly/workflows/review-library/steps/build.yaml", func(v map[string]any) {
				v["title"] = "Private unselected definition"
				if differentVersion {
					v["version"] = "2.0.0"
				}
			})
			private := compositionProgramConfig("PRIVATE_PROGRAM")
			private["files"] = map[string]any{"worker.sh": "missing-private-worker.sh"}
			compositionSourceProgram(t, root, "review-library", "steps", "test:step/build", private)
			compositionSourceProgram(t, root, "cycle", "steps", "test:step/build", compositionProgramConfig("CONSUMER_PROGRAM"))
			compiled, output, problem := compileCallExtensionFixture(t, root, authority)
			if problem != nil {
				t.Fatal(problem)
			}
			data, err = os.ReadFile(filepath.Join(output, projectExecutionFile))
			if err != nil {
				t.Fatal(err)
			}
			payload, err := projectDecodeExecution(data)
			if err != nil {
				t.Fatal(err)
			}
			if len(payload.Bindings) != 1 || payload.Bindings[0].DefinitionRef.ID != "test:step/build" || len(payload.Bindings[0].Config.Args) != 1 || payload.Bindings[0].Config.Args[0] != "CONSUMER_PROGRAM" {
				t.Fatalf("private program escaped closure: %+v", payload)
			}
			for _, path := range compiled.Sources {
				if strings.Contains(path, "missing-private-worker.sh") {
					t.Fatalf("unselected source entered closure: %s", path)
				}
			}
		})
	}
}

func TestProjectCompositionOwnedProgramRetainsExactBinding(t *testing.T) {
	t.Parallel()
	root, authority := writeCompositionFixture(t)
	compositionProcessStep(t, root, "review-library", "tests")
	config := compositionProgramConfig("worker.sh")
	config["files"] = map[string]any{"worker.sh": "worker.sh"}
	writeFixtureFile(t, root, ".prifly/workflows/review-library/worker.sh", "exit 0 # provider\n")
	compositionSourceProgram(t, root, "review-library", "steps", "test:step/tests", config)
	compiled, output, problem := compileCallExtensionFixture(t, root, authority)
	if problem != nil {
		t.Fatal(problem)
	}
	data, err := os.ReadFile(filepath.Join(output, projectExecutionFile))
	if err != nil {
		t.Fatal(err)
	}
	payload, err := projectDecodeExecution(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload.Bindings) != 1 || string(payload.Bindings[0].Files["worker.sh"]) != "exit 0 # provider\n" {
		t.Fatalf("wrong owned program: %+v", payload)
	}
	for _, component := range compiled.Components {
		if component.Ref.ID == "test:step/tests" {
			if component.Ref != payload.Bindings[0].DefinitionRef {
				t.Fatal("binding lost exact compiled ref")
			}
			return
		}
	}
	t.Fatal("owned step missing")
}

func TestProjectCompositionInheritedProgramRetainsOwnership(t *testing.T) {
	t.Parallel()
	root, authority := writeCompositionFixture(t)
	compositionProcessStep(t, root, "cycle", "build")
	compositionSourceProgram(t, root, "cycle", "steps", "test:step/build", compositionProgramConfig("INHERITED_PROGRAM"))
	registerCompositionDerived(t, root, "delivery", "cycle", "")
	output := filepath.Join(t.TempDir(), "compiled")
	code, _, stderr := runCLI(t, "--project", authority, "project", "compile", "--repository", root, "--package", "delivery", "--output", output)
	if code != 0 {
		t.Fatal(stderr)
	}
	data, err := os.ReadFile(filepath.Join(output, projectExecutionFile))
	if err != nil {
		t.Fatal(err)
	}
	payload, err := projectDecodeExecution(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload.Bindings) != 1 || payload.Bindings[0].Config.Args[0] != "INHERITED_PROGRAM" {
		t.Fatalf("inherited program lost: %+v", payload)
	}
}

func TestProjectCompositionOwnedCheckRetainsExactBinding(t *testing.T) {
	t.Parallel()
	root, authority := writeCompositionFixture(t)
	compositionEdit(t, root, ".prifly/workflows/review-library/workflow.yaml", func(v map[string]any) {
		v["package"].(map[string]any)["references"].(map[string]any)["process"] = "core:adapter/local-process@2.0.0"
	})
	compositionEdit(t, root, ".prifly/workflows/review-library/steps/tests.yaml", func(v map[string]any) { v["result_check_refs"] = []any{"{{check_validation}}"} })
	writeFixtureFile(t, root, ".prifly/workflows/review-library/checks/validation.yaml", "schema_version: check-definition/1\nid: test:check/validation\nversion: 1.0.0\ntitle: Synthetic check\nkind: result\nclaim: semantic_review\nexecutor: {adapter_ref: \"{{process}}\", operation: check}\n")
	compositionSourceProgram(t, root, "review-library", "checks", "test:check/validation", compositionProgramConfig("PROVIDER_CHECK"))
	compiled, output, problem := compileCallExtensionFixture(t, root, authority)
	if problem != nil {
		t.Fatal(problem)
	}
	data, err := os.ReadFile(filepath.Join(output, projectExecutionFile))
	if err != nil {
		t.Fatal(err)
	}
	payload, err := projectDecodeExecution(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload.Bindings) != 1 || payload.Bindings[0].Config.Args[0] != "PROVIDER_CHECK" {
		t.Fatalf("owned check missing: %+v", payload)
	}
	for _, component := range compiled.Components {
		if component.Kind == "check" && component.Ref == payload.Bindings[0].DefinitionRef {
			return
		}
	}
	t.Fatal("check binding lost exact compiled ref")
}

func TestProjectCompositionSharedProgramUsesEveryOwnedAlias(t *testing.T) {
	t.Parallel()
	for _, conflict := range []bool{false, true} {
		t.Run(map[bool]string{false: "identical", true: "conflict"}[conflict], func(t *testing.T) {
			t.Parallel()
			root, authority := writeCompositionFixture(t)
			compositionProcessStep(t, root, "review-library", "tests")
			config := compositionProgramConfig("worker.sh")
			config["files"] = map[string]any{"worker.sh": "worker.sh"}
			writeFixtureFile(t, root, ".prifly/workflows/review-library/worker.sh", "exit 0 # shared\n")
			compositionSourceProgram(t, root, "review-library", "steps", "test:step/tests", config)
			from := filepath.Join(root, ".prifly/workflows/review-library")
			err := filepath.WalkDir(from, func(path string, entry os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.IsDir() {
					return nil
				}
				relative, err := filepath.Rel(from, path)
				if err != nil {
					return err
				}
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				writeFixtureFile(t, root, filepath.Join(".prifly/workflows/second-library", relative), string(data))
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			profile, err := os.ReadFile(filepath.Join(root, ".prifly/project.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			writeFixtureFile(t, root, ".prifly/project.yaml", strings.Replace(string(profile), "launches:", "  second-library: {source: .prifly/workflows/second-library}\nlaunches:", 1))
			if conflict {
				compositionSourceProgram(t, root, "second-library", "steps", "test:step/tests", compositionProgramConfig("DIFFERENT_PROGRAM"))
			}
			extension := strings.Replace(compositionImportExtension, "imports: {ui: review-library}", "imports: {ui: review-library, second: second-library}", 1)
			second := strings.SplitN(compositionImportExtension, "extensions:\n", 2)[1]
			second = strings.ReplaceAll(strings.Replace(second, "interface-review", "interface-second", 1), "ui.review-for-change", "second.review-for-change")
			extension += second + "arrange:\n  - id: both\n    between: {from: build, to: commit}\n    sequence: [cycle.interface-review, cycle.interface-second]\n    on: {succeeded: commit, rejected: rejected}\n"
			writeFixtureFile(t, root, ".prifly/workflows/cycle/extend.yaml", extension)
			compiled, output, problem := compileCallExtensionFixture(t, root, authority)
			if conflict {
				if problem == nil || problem.Code != "project_composition_program_conflict" {
					t.Fatalf("shared program conflict lost: %+v", problem)
				}
				return
			}
			if problem != nil {
				t.Fatal(problem)
			}
			data, err := os.ReadFile(filepath.Join(output, projectExecutionFile))
			if err != nil {
				t.Fatal(err)
			}
			payload, err := projectDecodeExecution(data)
			if err != nil {
				t.Fatal(err)
			}
			if len(payload.Bindings) != 1 || payload.Bindings[0].Config.Args[0] != "worker.sh" {
				t.Fatalf("shared exact program lost or duplicated: %+v", payload)
			}
			for _, folder := range []string{"review-library", "second-library"} {
				if compiled.Composition.Files[".prifly/workflows/"+folder+"/worker.sh"] == "" {
					t.Fatalf("shared supporting path missing from provenance: %s", folder)
				}
			}
			count := 0
			for _, component := range compiled.Components {
				if component.Ref.ID == "test:step/tests" {
					count++
					if component.Ref != payload.Bindings[0].DefinitionRef {
						t.Fatal("shared binding lost exact ref")
					}
				}
			}
			if count != 1 {
				t.Fatalf("shared step emitted %d times", count)
			}
		})
	}
}
