package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// A pilot skill is a markdown file plus its own reference material, so the
// fixture keeps that exact shape: core never learns what a skill is, it only
// pins declared bytes.
func packageSource(t *testing.T, files map[string]string, components []map[string]any, mutate func(map[string]any)) string {
	t.Helper()
	source := t.TempDir()
	declared := []map[string]any{}
	for path, content := range files {
		full := filepath.Join(source, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		media := "text/markdown; charset=utf-8"
		if filepath.Ext(path) == ".json" {
			media = "application/json"
		}
		declared = append(declared, map[string]any{"path": path, "digest": rawDigest([]byte(content)), "size_bytes": len(content), "media_type": media, "role": "data"})
	}
	manifest := map[string]any{
		"schema_version": "1", "id": "aif:package/pilot", "version": "1.0.0",
		"description": "Externally authored skills sealed for this installation", "requires_core_protocol": "1",
		"dependencies": []any{}, "components": components, "files": declared,
		"requested_capabilities": []any{}, "license": "MIT",
	}
	if mutate != nil {
		mutate(manifest)
	}
	b, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, PackageManifestFile), b, 0600); err != nil {
		t.Fatal(err)
	}
	return source
}

func skillPackage(t *testing.T, body string) (string, map[string]string, []map[string]any) {
	t.Helper()
	files := map[string]string{
		"skills/aif-plan/SKILL.md":                  body,
		"skills/aif-plan/references/TASK-FORMAT.md": "# Task format\n",
	}
	components := []map[string]any{
		{"kind": "context", "ref": map[string]any{"id": "aif:context/plan-skill", "version": "1.0.0", "digest": rawDigest([]byte(body))}, "path": "skills/aif-plan/SKILL.md"},
	}
	return packageSource(t, files, components, nil), files, components
}

func toolPackage(t *testing.T, adapter flow.Ref, operation, effectClass, retryClass string) (string, flow.Ref) {
	t.Helper()
	definitions, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	descriptor := flow.ToolDescriptor{
		SchemaVersion:        flow.ToolDescriptorVersion,
		ID:                   "aif:tool/announce",
		Version:              "1.0.0",
		AdapterRef:           adapter,
		Operation:            operation,
		ArgumentsSchemaRef:   builtinRef(definitions, "core:schema/context-json"),
		ResultSchemaRef:      builtinRef(definitions, "core:schema/context-json"),
		EffectClass:          effectClass,
		RetryClass:           retryClass,
		RequiredCapabilities: []string{"network"},
	}
	data, err := canonical(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	ref := flow.Ref{ID: descriptor.ID, Version: descriptor.Version, Digest: rawDigest(data)}
	source := packageSource(t, map[string]string{"tools/announce.json": string(data)}, []map[string]any{
		{"kind": "tool", "ref": ref, "path": "tools/announce.json"},
	}, nil)
	return source, ref
}

func TestPackageImportSealsDeclaredBytesAndRecordsTrust(t *testing.T) {
	e := contextRegistryRuntime(t)
	ctx := context.Background()
	body := "---\nname: aif-plan\n---\n\n# Plan\n"
	source, _, _ := skillPackage(t, body)
	result, err := e.ImportPackage(ctx, PackageImportRequest{CommandID: "command:import", Directory: source, Reason: "external pilot skills reviewed by the owner"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Receipt.Rejection != nil {
		t.Fatalf("import rejected: %+v", result.Receipt.Rejection)
	}
	record, err := e.Packages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Packages) != 1 {
		t.Fatalf("expected exactly one trusted package: %+v", record.Packages)
	}
	pkg := record.Packages[0]
	if pkg.Ref.ID != "aif:package/pilot" || pkg.Trust.Decision != "local_owner_accepted" || pkg.Trust.ActorID != e.owner || pkg.Trust.ControlAdmissionID == "" {
		t.Fatalf("trust decision was not recorded against the session principal: %+v", pkg.Trust)
	}
	resolved, err := filepath.EvalSymlinks(source)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Origin.Location != resolved || pkg.Origin.Kind != "local_directory" {
		t.Fatalf("origin was not recorded: %+v", pkg.Origin)
	}
	// The sealed payload is a copy: deleting the source cannot unpin the bytes.
	if err := os.RemoveAll(source); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(e.Root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	_, registry, resources, err := reopened.inventoryResources()
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 1 || resources[0].Ref.ID != "aif:context/plan-skill" || string(resources[0].Bytes) != body {
		t.Fatalf("package component did not resolve to its pinned bytes: %+v", resources)
	}
	if resources[0].MediaType != "text/markdown; charset=utf-8" || resources[0].ByteEncoding != "utf8_text" {
		t.Fatalf("declared representation was not preserved: %+v", resources[0])
	}
	if len(registry) == 0 {
		t.Fatal("registry is empty")
	}
}

func TestPackageImportSealsToolDescriptorWithoutExecutingIt(t *testing.T) {
	e := contextRegistryRuntime(t)
	ctx := context.Background()
	definitions, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	source, ref := toolPackage(t, builtinVersionRef(definitions, "core:adapter/local-process", "2.0.0"), "announce", "external_write", "deduplicated")
	if _, err := e.ImportPackage(ctx, PackageImportRequest{CommandID: "command:tool", Directory: source, Reason: "seal tool contract for later admission"}); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(e.Root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	defs, registry, resources, err := reopened.inventoryResources()
	if err != nil || len(resources) != 0 {
		t.Fatalf("tool descriptor inventory failed: %v", err)
	}
	if _, err := flow.ParseToolDescriptor(registry[ref]); err != nil {
		t.Fatalf("sealed tool descriptor was not retained: %v", err)
	}
	for _, definition := range defs {
		if definition.Ref == ref && definition.Kind == "tool" {
			return
		}
	}
	t.Fatalf("tool descriptor is absent from the pinned definition inventory: %+v", defs)
}

// Pilot acceptance 1: changed skill bytes cannot substitute for the pinned ones.
func TestChangedPackageBytesDoNotResolve(t *testing.T) {
	e := contextRegistryRuntime(t)
	ctx := context.Background()
	body := "---\nname: aif-plan\n---\n\n# Plan\n"
	source, _, _ := skillPackage(t, body)
	if _, err := e.ImportPackage(ctx, PackageImportRequest{CommandID: "command:import", Directory: source, Reason: "sealed"}); err != nil {
		t.Fatal(err)
	}
	record, err := e.Packages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sealed := filepath.Join(e.Root, filepath.FromSlash(record.Packages[0].Components[0].Path))
	if err := os.Chmod(sealed, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sealed, []byte("---\nname: aif-plan\n---\n\n# Replaced\n"), 0600); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(e.Root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, _, _, err := reopened.inventoryResources(); err == nil {
		t.Fatal("replaced skill bytes resolved under the pinned reference")
	}
}

func TestPackageImportRejectsUndeclaredAndMismatchedContent(t *testing.T) {
	e := contextRegistryRuntime(t)
	ctx := context.Background()
	body := "# Plan\n"
	source, _, _ := skillPackage(t, body)
	if err := os.WriteFile(filepath.Join(source, "skills/aif-plan/EXTRA.md"), []byte("smuggled"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.ImportPackage(ctx, PackageImportRequest{CommandID: "command:undeclared", Directory: source, Reason: "undeclared file"}); err == nil {
		t.Fatal("an undeclared payload file was imported")
	}

	mismatched := packageSource(t, map[string]string{"skills/a/SKILL.md": "# A\n"}, []map[string]any{
		{"kind": "context", "ref": map[string]any{"id": "aif:context/a", "version": "1.0.0", "digest": rawDigest([]byte("# other\n"))}, "path": "skills/a/SKILL.md"},
	}, nil)
	if _, err := e.ImportPackage(ctx, PackageImportRequest{CommandID: "command:mismatch", Directory: mismatched, Reason: "wrong component digest"}); err == nil {
		t.Fatal("a component reference naming other bytes was imported")
	}

	tampered := packageSource(t, map[string]string{"skills/a/SKILL.md": "# A\n"}, []map[string]any{
		{"kind": "context", "ref": map[string]any{"id": "aif:context/a", "version": "1.0.0", "digest": rawDigest([]byte("# A\n"))}, "path": "skills/a/SKILL.md"},
	}, func(manifest map[string]any) {
		manifest["files"].([]map[string]any)[0]["digest"] = rawDigest([]byte("# tampered\n"))
	})
	if _, err := e.ImportPackage(ctx, PackageImportRequest{CommandID: "command:tampered", Directory: tampered, Reason: "manifest disagrees with bytes"}); err == nil {
		t.Fatal("a file disagreeing with its declared digest was imported")
	}

	unsupported := packageSource(t, map[string]string{"policy.json": "{}"}, []map[string]any{
		{"kind": "policy", "ref": map[string]any{"id": "aif:policy/a", "version": "1.0.0", "digest": rawDigest([]byte("{}"))}, "path": "policy.json"},
	}, nil)
	if _, err := e.ImportPackage(ctx, PackageImportRequest{CommandID: "command:policy", Directory: unsupported, Reason: "policy component"}); err == nil {
		t.Fatal("a package supplied its own policy")
	}
	malformed := packageSource(t, map[string]string{"tools/bad.json": "{}"}, []map[string]any{
		{"kind": "tool", "ref": map[string]any{"id": "aif:tool/bad", "version": "1.0.0", "digest": rawDigest([]byte("{}"))}, "path": "tools/bad.json"},
	}, nil)
	if _, err := e.ImportPackage(ctx, PackageImportRequest{CommandID: "command:bad-tool", Directory: malformed, Reason: "malformed tool contract"}); err == nil {
		t.Fatal("a malformed tool descriptor was imported")
	}
	if record, err := e.Packages(ctx); err != nil || len(record.Packages) != 0 {
		t.Fatalf("a rejected import registered a package: %v %+v", err, record.Packages)
	}
}

func TestPackageIdentityConflictAndRepeatedImport(t *testing.T) {
	e := contextRegistryRuntime(t)
	ctx := context.Background()
	source, _, _ := skillPackage(t, "# Plan\n")
	if _, err := e.ImportPackage(ctx, PackageImportRequest{CommandID: "command:first", Directory: source, Reason: "first"}); err != nil {
		t.Fatal(err)
	}
	repeat, err := e.ImportPackage(ctx, PackageImportRequest{CommandID: "command:second", Directory: source, Reason: "second"})
	if err != nil {
		t.Fatal(err)
	}
	if repeat.Receipt.Rejection == nil || repeat.Receipt.Rejection.Code != "package_present" {
		t.Fatalf("the same sealed revision was trusted twice: %+v", repeat.Receipt)
	}
	// Same id and version, different bytes: a conflict, never a silent update.
	other, _, _ := skillPackage(t, "# Plan changed\n")
	if _, err := e.ImportPackage(ctx, PackageImportRequest{CommandID: "command:conflict", Directory: other, Reason: "other bytes"}); err == nil {
		t.Fatal("the same package id and version was re-trusted with other bytes")
	}
}

// A control plane enrolled before package trust existed must upgrade, not brick
// the installation: the core operation set belongs to the enrolled owner.
func TestControlPlaneEnrolledWithoutTrustIsReconciled(t *testing.T) {
	e := contextRegistryRuntime(t)
	ctx := context.Background()
	if _, _, err := e.ensureControl(ctx); err != nil {
		t.Fatal(err)
	}
	control, version, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for i := range control.Access {
		control.Access[i].Operations = []string{ControlOperationAdmit}
	}
	payload, err := canonical(map[string]any{"operation": "test.downgrade"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Store.ApplyAuthority(ctx, local.AuthorityCommand{ID: "command:downgrade", Actor: e.owner, Key: AuthorityControlKey, Payload: payload, ExpectedVersion: &version}, func(local.AuthoritySnapshot) (local.AuthorityChange, error) {
		data, err := canonicalState(control)
		return local.AuthorityChange{Data: data}, err
	}); err != nil {
		t.Fatal(err)
	}
	if before, _, err := e.Control(ctx); err != nil || before.allows(e.owner, "project", e.Config.ID, ControlOperationTrust) {
		t.Fatalf("fixture did not remove the trust operation: %v", err)
	}
	source, _, _ := skillPackage(t, "# Plan\n")
	if _, err := e.ImportPackage(ctx, PackageImportRequest{CommandID: "command:after-upgrade", Directory: source, Reason: "upgraded control plane"}); err != nil {
		t.Fatal(err)
	}
	after, _, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !after.allows(e.owner, "project", e.Config.ID, ControlOperationTrust) || !after.allows(e.owner, "installation", e.Installation.ID, ControlOperationTrust) {
		t.Fatalf("the owner operation set was not reconciled: %+v", after.Access)
	}
}

func TestPackageComponentCannotShadowCoreOrLocalDefinitions(t *testing.T) {
	e := contextRegistryRuntime(t)
	ctx := context.Background()
	core := packageSource(t, map[string]string{"a.md": "# A\n"}, []map[string]any{
		{"kind": "context", "ref": map[string]any{"id": "core:context/a", "version": "1.0.0", "digest": rawDigest([]byte("# A\n"))}, "path": "a.md"},
	}, nil)
	if _, err := e.ImportPackage(ctx, PackageImportRequest{CommandID: "command:core", Directory: core, Reason: "core shadow"}); err == nil {
		t.Fatal("a package replaced a core contract")
	}
	if e.Config.Configuration.SemanticsProfile != flow.CoreProfile {
		t.Fatal("fixture is not a core installation")
	}
}
