package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func TestCLIProjectCompilesAndImportsCheckPackageWithoutGit(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	root, authority := t.TempDir(), filepath.Join(t.TempDir(), "authority")
	if code, _, stderr := runCLI(t, "project", "init", "--repository", root, "--state-root", authority); code != 0 {
		t.Fatalf("neutral init: %d %s", code, stderr)
	}
	writeFixtureFile(t, root, ".prifly/project.yaml", "schema_version: prifly-project-profile/3\npackages:\n  report: {source: .prifly/workflows/report}\nlaunches: {}\n")
	writeFixtureFile(t, root, ".prifly/workflows/report/checks/content.yaml", `schema_version: check-definition/1
id: test:check/content
version: 1.0.0
title: Verify input content
kind: content
claim: content_valid
executor: {adapter_ref: "{{check_adapter}}", operation: check}
`)
	writeFixtureFile(t, root, ".prifly/workflows/report/workflow.yaml", `authoring: prifly-project-workflow/1
package:
  id: test:package/checked-report
  version: 1.0.0
  description: Input content validation
  requires_core_protocol: "1"
  references:
    local_policy: core:policy/local@3.0.0
    check_adapter: core:adapter/local-process@2.0.0
id: test:workflow/checked-report
version: 1.0.0
inputs:
  value: {format: blob, media_types: [text/plain], content_check_refs: ["{{check_content}}"]}
outputs: {}
entry: done
limits: {max_step_instances: 1, max_control_transitions: 4}
policy_ref: "{{local_policy}}"
stages:
  done: {kind: finish, outcome: no_work}
`)
	shared, err := os.ReadFile(filepath.Join(root, ".prifly", "project.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "compiled")
	code, out, stderr := runCLI(t, "--project", authority, "project", "compile", "--repository", root, "--package", "report", "--output", output)
	if code != 0 {
		t.Fatalf("compile check package: %d %s", code, stderr)
	}
	var compiled projectCompileResult
	if err := json.Unmarshal([]byte(out), &compiled); err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile(filepath.Join(output, prifly.PackageManifestFile))
	if err != nil {
		t.Fatal(err)
	}
	if err := flow.ValidateProtocol("PackageManifestV2", manifest); err != nil {
		t.Fatalf("new manifest contract: %v", err)
	}
	published, err := os.ReadFile(filepath.Join("..", "..", "schemas", "core", "package-manifest-v2.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	current, err := flow.ProtocolSchema("PackageManifestV2")
	if err != nil || !bytes.Equal(published, append(current, '\n')) {
		t.Fatalf("published manifest v2 schema differs from validator: %v", err)
	}
	if err := flow.ValidateProtocol("PackageManifest", manifest); err == nil {
		t.Fatal("legacy manifest reader accepted version 2")
	}
	var legacy map[string]any
	if err := json.Unmarshal(manifest, &legacy); err != nil {
		t.Fatal(err)
	}
	legacy["schema_version"] = "1"
	legacyBytes, _ := json.Marshal(legacy)
	if err := flow.ValidatePackageManifest(legacyBytes); err == nil {
		t.Fatal("version 1 admitted new check component kind")
	}
	engine, err := prifly.Open(authority, true)
	if err != nil {
		t.Fatal(err)
	}
	packages, err := engine.Packages(context.Background())
	if err != nil || len(packages.Packages) != 0 {
		t.Fatalf("compile unexpectedly imported package: %+v %v", packages, err)
	}
	_ = engine.Close()
	if code, _, stderr := runCLI(t, "--project", authority, "package", "import", "--dir", output, "--reason", "check package version 2 fixture"); code != 0 {
		t.Fatalf("import check package: %d %s", code, stderr)
	}
	engine, err = prifly.Open(authority, true)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	_, registry, err := engine.Inventory()
	if err != nil {
		t.Fatal(err)
	}
	var checkRef, workflowRef flow.Ref
	for _, component := range compiled.Components {
		switch component.Kind {
		case "check":
			checkRef = component.Ref
			if _, err := flow.ParseCheckDefinition(registry[checkRef]); err != nil {
				t.Fatalf("imported check bytes unavailable: %v", err)
			}
		case "workflow":
			workflowRef = component.Ref
		}
	}
	if checkRef.ID == "" || workflowRef.ID == "" {
		t.Fatalf("compiled package omitted check or workflow: %+v", compiled.Components)
	}
	plan, err := flow.CompileCore(registry[workflowRef], "json", registry, nil)
	if err != nil || len(plan.Checks) != 1 || plan.Checks[checkRef].ID != checkRef.ID {
		t.Fatalf("workflow did not bind its imported exact check: %+v %v", plan, err)
	}
	if _, err := engine.InspectPackage(context.Background(), compiled.Package); err != nil {
		t.Fatalf("package inspection rejected version 2 after reopen: %v", err)
	}
	after, err := os.ReadFile(filepath.Join(root, ".prifly", "project.yaml"))
	if err != nil || !bytes.Equal(shared, after) {
		t.Fatalf("compile or import changed shared YAML: %v", err)
	}
	writeFixtureFile(t, root, ".prifly/workflows/report/checks/content.yaml", "schema_version: check-definition/1\nid: test:check/content\nversion: 1.0.0\n")
	if code, _, stderr := runCLI(t, "--project", authority, "project", "compile", "--repository", root, "--package", "report", "--output", filepath.Join(t.TempDir(), "rejected")); code == 0 || !strings.Contains(stderr, "invalid_check_definition") {
		t.Fatalf("incomplete check accepted: %d %s", code, stderr)
	}
}

func TestProjectLegacyProfileRejectsCheckDocumentsBeforeOutput(t *testing.T) {
	output := filepath.Join(t.TempDir(), "must-not-exist")
	source := projectPackageSource{Documents: []projectPackageDocument{{Kind: "check"}}}
	_, err := compileAndSealProjectPackage(t.TempDir(), "", output, "prifly-project-profile/2", "", source, nil, prifly.PackageRecord{}, nil, projectWorkflowOptions{})
	if err == nil || !strings.Contains(err.Error(), "project_compile_profile_required: checks require profile /3") {
		t.Fatalf("legacy compilation accepted or silently dropped checks: %v", err)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("legacy refusal wrote output: %v", err)
	}
}
