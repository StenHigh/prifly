package runtime

import (
	"context"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

func TestPackageCheckImportsKeepVersionAndIdentityBoundaries(t *testing.T) {
	for _, failure := range []string{"legacy-manifest", "unknown-version", "malformed-check", "identity-mismatch", "digest-mismatch"} {
		t.Run(failure, func(t *testing.T) {
			e := contextRegistryRuntime(t)
			definitions, _, err := Builtins()
			if err != nil {
				t.Fatal(err)
			}
			check := flow.CheckDefinition{SchemaVersion: flow.CheckDefinitionVersion, ID: "test:check/package", Version: "1.0.0", Title: "Check package input", Kind: "content", Claim: "content_valid", Executor: flow.Executor{AdapterRef: builtinVersionRef(definitions, "core:adapter/local-process", "2.0.0"), Operation: "check"}}
			if failure == "malformed-check" {
				check.Claim = "undeclared_claim"
			}
			data, err := canonical(check)
			if err != nil {
				t.Fatal(err)
			}
			ref := flow.Ref{ID: check.ID, Version: check.Version, Digest: rawDigest(data)}
			if failure == "identity-mismatch" {
				ref.ID = "test:check/different"
			}
			if failure == "digest-mismatch" {
				ref.Digest = rawDigest([]byte("different bytes"))
			}
			source := packageSource(t, map[string]string{"checks/input.json": string(data)}, []map[string]any{{"kind": "check", "ref": ref, "path": "checks/input.json"}}, func(manifest map[string]any) {
				manifest["id"], manifest["schema_version"] = "test:package/checked", "2"
				if failure == "legacy-manifest" {
					manifest["schema_version"] = "1"
				}
				if failure == "unknown-version" {
					manifest["schema_version"] = "3"
				}
			})
			if _, err := e.ImportPackage(context.Background(), PackageImportRequest{CommandID: "command:check-package", Directory: source, Reason: "invalid check package must be refused"}); err == nil {
				t.Fatal("invalid check package imported")
			}
			packages, err := e.Packages(context.Background())
			if err != nil || len(packages.Packages) != 0 {
				t.Fatalf("rejected import changed trust registry: %+v %v", packages, err)
			}
		})
	}
}
