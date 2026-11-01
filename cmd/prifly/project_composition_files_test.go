package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestProjectCompositionLayerSupportingFilesUseDeclaringDirectory(t *testing.T) {
	t.Parallel()
	for _, decoy := range []bool{false, true} {
		t.Run(fmt.Sprintf("decoy=%t", decoy), func(t *testing.T) {
			t.Parallel()
			extend := strings.Replace(callExtensionSource, "prifly-extension/2", "prifly-extension/3", 1) + "layers: [../../../shared/delivery.yaml]\n"
			root, authority := writeCallExtensionFixture(t, extend)
			const layer = "authoring: prifly-extension/3\nreferences:\n  process: core:adapter/local-process@2.0.0\nexecution_bindings:\n  steps:\n    tests: {executable: shell, args: [worker.sh], files: {worker.sh: worker.sh}, timeout_ms: 30000, grace_ms: 100, max_output_bytes: 65536}\n"
			writeFixtureFile(t, root, "shared/delivery.yaml", layer)
			path := filepath.Join(root, ".prifly/workflows/cycle/steps/tests.yaml")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			writeFixtureFile(t, root, ".prifly/workflows/cycle/steps/tests.yaml", strings.Replace(string(data), `executor: {adapter_ref: "{{assisted}}", operation: session}`, `executor: {adapter_ref: "{{process}}", operation: process}`, 1))
			const worker = "exit 0 # layer\n"
			writeFixtureFile(t, root, "shared/worker.sh", worker)
			const decoyPath = ".prifly/workflows/cycle/worker.sh"
			if decoy {
				writeFixtureFile(t, root, decoyPath, "exit 1 # workflow decoy\n")
			}
			before, output, problem := compileCallExtensionFixture(t, root, authority)
			if problem != nil {
				t.Fatal(problem)
			}
			data, err = os.ReadFile(filepath.Join(output, projectExecutionFile))
			if err != nil {
				t.Fatal(err)
			}
			binding, err := projectDecodeExecution(data)
			if err != nil {
				t.Fatal(err)
			}
			if binding == nil || len(binding.Bindings) != 1 || string(binding.Bindings[0].Files["worker.sh"]) != worker {
				t.Fatalf("wrong sealed layer bytes: %+v", binding)
			}
			if before.Composition == nil || before.Composition.Files["shared/worker.sh"] != fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(worker))) {
				t.Fatal("missing actual layer file provenance")
			}
			if slices.Contains(before.Sources, decoyPath) || before.Composition.Files[decoyPath] != "" {
				t.Fatal("decoy entered the reviewed closure")
			}
			if !slices.Contains(before.Sources, "shared/worker.sh") {
				t.Fatal("layer file missing from sources")
			}
			writeFixtureFile(t, root, decoyPath, "exit 2 # changed decoy\n")
			decoyChanged, _, problem := compileCallExtensionFixture(t, root, authority)
			if problem != nil {
				t.Fatal(problem)
			}
			if before.BuildKey != decoyChanged.BuildKey {
				t.Fatal("unread decoy changed build identity")
			}
			writeFixtureFile(t, root, "shared/worker.sh", "exit 0 # changed layer\n")
			after, _, problem := compileCallExtensionFixture(t, root, authority)
			if problem != nil {
				t.Fatal(problem)
			}
			if before.BuildKey == after.BuildKey {
				t.Fatal("changed layer bytes kept old build identity")
			}
			if string(binding.Bindings[0].Files["worker.sh"]) != worker {
				t.Fatal("old sealed bytes changed")
			}
			if err := os.Remove(filepath.Join(root, "shared/worker.sh")); err != nil {
				t.Fatal(err)
			}
			outside := filepath.Join(t.TempDir(), "worker.sh")
			if err := os.WriteFile(outside, []byte(worker), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(root, "shared/worker.sh")); err != nil {
				t.Fatal(err)
			}
			_, _, problem = compileCallExtensionFixture(t, root, authority)
			if problem == nil {
				t.Fatal("symlink was accepted despite an available workflow decoy")
			}
		})
	}
}
