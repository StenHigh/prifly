package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

func TestProjectBuildProvenanceRejectsIdentityTampering(t *testing.T) {
	data := []byte(`{"id":"test:workflow/root","version":"1.0.0"}`)
	digest, err := flow.Digest(data)
	if err != nil {
		t.Fatal(err)
	}
	components, build, err := projectBuildVariant(
		projectPackageSource{ID: "test:package/build", Version: "1.0.0"},
		[]projectCompileComponent{{Kind: "workflow", Ref: flow.Ref{ID: "test:workflow/root", Version: "1.0.0", Digest: digest}, Bytes: data, Root: true}},
		"", map[string]any{}, projectWorkflowOptions{Settings: map[string]map[string]any{}, Exclude: []string{}},
	)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := projectCanonicalJSON(build)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateProjectBuild(baseline, components); err != nil {
		t.Fatalf("valid provenance: %v", err)
	}
	t.Run("unknown field", func(t *testing.T) {
		var changed map[string]any
		if err := json.Unmarshal(baseline, &changed); err != nil {
			t.Fatal(err)
		}
		changed["future_permission"] = true
		encoded, err := projectCanonicalJSON(changed)
		if err != nil {
			t.Fatal(err)
		}
		if err := validateProjectBuild(encoded, components); err == nil {
			t.Fatal("the provenance schema accepted an unknown field")
		}
	})
	for _, test := range []struct {
		name   string
		change func(*projectBuildProvenance)
	}{
		{"author version", func(build *projectBuildProvenance) { build.Components[0].AuthorRef.Version = "2.0.0" }},
		{"author digest", func(build *projectBuildProvenance) {
			build.Components[0].AuthorRef.Digest = "sha256:" + strings.Repeat("f", 64)
		}},
		{"build key", func(build *projectBuildProvenance) { build.BuildKey = "sha256:" + strings.Repeat("f", 64) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			var changed projectBuildProvenance
			if err := json.Unmarshal(baseline, &changed); err != nil {
				t.Fatal(err)
			}
			test.change(&changed)
			encoded, err := projectCanonicalJSON(changed)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateProjectBuild(encoded, components); err == nil || !strings.Contains(err.Error(), "project_compile_invalid_provenance") {
				t.Fatalf("changed provenance must fail: %v", err)
			}
		})
	}
	version, err := projectBuildPackageVersion(build.BuildKey)
	if err != nil {
		t.Fatal(err)
	}
	output := t.TempDir()
	writeFixtureFile(t, output, projectBuildFile, string(baseline))
	compiled := projectCompileResult{SchemaVersion: "project-compile/2", Package: flow.Ref{ID: build.AuthorPackage.ID, Version: version}, Output: output, Components: components, AuthorPackage: &build.AuthorPackage, BuildKey: build.BuildKey}
	if _, err := projectCompiledLaunchPath("", projectLaunch{}, compiled); err != nil {
		t.Fatalf("valid root: %v", err)
	}
	compiled.Package.Version = "1.0.0"
	if _, err := projectCompiledLaunchPath("", projectLaunch{}, compiled); err == nil || !strings.Contains(err.Error(), "project_compile_invalid_provenance") {
		t.Fatalf("changed package version must fail: %v", err)
	}
}
