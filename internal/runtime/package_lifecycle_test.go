package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

func importedPilotPackage(t *testing.T) (*Engine, context.Context, PackageEntry, string) {
	t.Helper()
	e := contextRegistryRuntime(t)
	ctx := context.Background()
	source, _, _ := skillPackage(t, "---\nname: aif-plan\n---\n\n# Plan\n")
	if _, err := e.ImportPackage(ctx, PackageImportRequest{CommandID: "command:import", Directory: source, Reason: "pilot skills"}); err != nil {
		t.Fatal(err)
	}
	record, err := e.Packages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return e, ctx, record.Packages[0], source
}

func TestPackageVerifyReportsDriftWithoutRepairingIt(t *testing.T) {
	e, ctx, pkg, _ := importedPilotPackage(t)
	report, err := e.VerifyPackage(ctx, pkg.Ref.ID, pkg.Ref.Version)
	if err != nil {
		t.Fatal(err)
	}
	if !report.ManifestValid || report.FilesChecked != len(pkg.Files) || len(report.Mismatched) != 0 || len(report.Missing) != 0 {
		t.Fatalf("a freshly sealed package did not verify: %+v", report)
	}
	sealed := filepath.Join(e.Root, filepath.FromSlash(pkg.Components[0].Path))
	if err := os.Chmod(sealed, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sealed, []byte("replaced"), 0600); err != nil {
		t.Fatal(err)
	}
	drifted, err := e.VerifyPackage(ctx, pkg.Ref.ID, pkg.Ref.Version)
	if err != nil {
		t.Fatal(err)
	}
	if len(drifted.Mismatched) != 1 {
		t.Fatalf("drift was not reported: %+v", drifted)
	}
	// Verification reports; it never rewrites the bytes it found.
	data, err := os.ReadFile(sealed)
	if err != nil || string(data) != "replaced" {
		t.Fatalf("verification repaired the file instead of reporting it: %v", err)
	}
}

func TestInspectPackageRequiresTheExactSealedManifest(t *testing.T) {
	e, ctx, pkg, _ := importedPilotPackage(t)
	inspection, err := e.InspectPackage(ctx, pkg.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Ref != pkg.Ref || inspection.Manifest.Description == "" || inspection.Manifest.License == "" || inspection.Resolvable != true {
		t.Fatalf("inspection lost sealed package metadata: %+v", inspection)
	}
	other := pkg.Ref
	other.Digest = "sha256:other"
	if _, err := e.InspectPackage(ctx, other); !errors.Is(err, local.ErrNotFound) {
		t.Fatalf("different manifest digest was accepted: %v", err)
	}
	manifest := filepath.Join(e.Root, filepath.FromSlash(pkg.Root), PackageManifestFile)
	if err := os.Chmod(manifest, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, []byte(`{"not":"the sealed manifest"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.InspectPackage(ctx, pkg.Ref); !errors.Is(err, local.ErrIntegrity) {
		t.Fatalf("changed manifest was presented as metadata: %v", err)
	}
}

// PKG-010: uninstall closes a package for new resolution and must not break a
// Run that still holds it.
func TestRemoveRefusesWhileARunHoldsThePackageAndStopsResolutionAfter(t *testing.T) {
	e, ctx, pkg, _ := importedPilotPackage(t)
	opened, err := Open(e.Root, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, resources, err := opened.inventoryResources(); err != nil || len(resources) != 1 {
		t.Fatalf("the imported component did not resolve: %v", err)
	}
	opened.Close()
	removed, err := e.SetPackageStatus(ctx, PackageLifecycleRequest{CommandID: "command:remove", ID: pkg.Ref.ID, Version: pkg.Ref.Version, Status: PackageRemoved, Reason: "no longer needed"})
	if err != nil {
		t.Fatal(err)
	}
	if removed.Receipt.Rejection != nil {
		t.Fatalf("removal of an unheld package was refused: %+v", removed.Receipt.Rejection)
	}
	reopened, err := Open(e.Root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, _, resources, err := reopened.inventoryResources(); err != nil || len(resources) != 0 {
		t.Fatalf("a removed package still supplied definitions: %v %+v", err, resources)
	}
	// The bytes stay: removal is not deletion of evidence.
	if _, err := os.Stat(filepath.Join(e.Root, filepath.FromSlash(pkg.Components[0].Path))); err != nil {
		t.Fatalf("removal destroyed the sealed bytes: %v", err)
	}
	report, err := reopened.VerifyPackage(ctx, pkg.Ref.ID, pkg.Ref.Version)
	if err != nil || report.Status != PackageRemoved || !report.ManifestValid {
		t.Fatalf("a removed package is no longer inspectable: %v %+v", err, report)
	}
}

// An identity the authority pinned could never be released: definition_drift
// refused the next package that named the same id and version, and no command
// cleared the pin — the pilot tried remove, revoke and quarantine in turn and
// had to abandon the authority with its whole run history. Removal is the
// answer to "this identity must be free again", and it already refuses while a
// Run still holds the package, so it is the one place the release belongs.
func TestRemovingAPackageReleasesTheIdentitiesItPinned(t *testing.T) {
	e, ctx, pkg, _ := importedPilotPackage(t)
	ref := pkg.Components[0].Ref
	original := []byte(`{"first":true}`)
	changed := []byte(`{"second":true}`)
	pin := func(bytes []byte) error {
		return e.pinDefinitions([]PinnedDefinition{{Ref: ref, Kind: pkg.Components[0].Kind, RawDigest: rawDigest(bytes), Bytes: bytes}})
	}
	if err := pin(original); err != nil {
		t.Fatal(err)
	}
	if err := pin(changed); err == nil || !strings.Contains(err.Error(), "definition_drift") {
		t.Fatalf("the identity accepted different bytes while the package held it: %v", err)
	}
	removed, err := e.SetPackageStatus(ctx, PackageLifecycleRequest{CommandID: "command:remove", ID: pkg.Ref.ID, Version: pkg.Ref.Version, Status: PackageRemoved, Reason: "release the identity"})
	if err != nil || removed.Receipt.Rejection != nil {
		t.Fatalf("removal was refused: %v %+v", err, removed.Receipt.Rejection)
	}
	if err := pin(changed); err != nil {
		t.Fatalf("removal did not release the identity: %v", err)
	}
}

// Replacing a package is remove then import, and the gate refused on the record
// that was just removed: it demanded every package that ever declared a
// component still be resolvable, instead of one that is. The documented way to
// replace a package could not be completed.
func TestAComponentNeedsOneResolvableSupplierNotEveryPast(t *testing.T) {
	e, ctx, pkg, _ := importedPilotPackage(t)
	refs := []flow.Ref{pkg.Components[0].Ref}
	if _, refusal, err := e.packageAdmissionGate(ctx, refs, true); err != nil || refusal != nil {
		t.Fatalf("a trusted package was refused: %v %v", err, refusal)
	}
	removed, err := e.SetPackageStatus(ctx, PackageLifecycleRequest{CommandID: "command:remove", ID: pkg.Ref.ID, Version: pkg.Ref.Version, Status: PackageRemoved, Reason: "replaced by a newer build"})
	if err != nil || removed.Receipt.Rejection != nil {
		t.Fatalf("removal was refused: %v %+v", err, removed.Receipt.Rejection)
	}
	// Nothing supplies it now, so the refusal is correct and says so plainly.
	_, refusal, err := e.packageAdmissionGate(ctx, refs, true)
	if err != nil {
		t.Fatal(err)
	}
	rejection, ok := refusal.(*local.Rejection)
	if !ok || rejection.Code != "package_not_resolvable" {
		t.Fatalf("a component with no supplier was admitted: %v", refusal)
	}
	if !strings.Contains(rejection.Message, "no trusted package supplies") {
		t.Fatalf("the refusal does not say what is missing: %s", rejection.Message)
	}
}

// Two upgrades leave three trusted imports of one package in an authority, all
// declaring the same component ids. Answering with whichever came first handed a
// reader the wrong edition of their own step's instructions, and they found out
// only by comparing digests against their envelope.
func TestPackageComponentRefusesWhenTwoPackagesDeclareIt(t *testing.T) {
	e, ctx, pkg, _ := importedPilotPackage(t)
	component := pkg.Components[0].Ref
	// One trusted package: the component reads back as itself.
	definition, _, err := e.PackageComponent(ctx, component.ID, "")
	if err != nil || definition.Ref != component {
		t.Fatalf("a single declaring package did not answer: %v %+v", err, definition)
	}
	// A second edition: same component id under a later component and package
	// version, which is what an upgrade produces and what the identity guard
	// deliberately admits.
	body := "---\nname: aif-plan\n---\n\n# Plan, second edition\n"
	files := map[string]string{"skills/aif-plan/SKILL.md": body, "skills/aif-plan/references/TASK-FORMAT.md": "# Task format\n"}
	components := []map[string]any{{"kind": "context", "ref": map[string]any{"id": component.ID, "version": "1.1.0", "digest": rawDigest([]byte(body))}, "path": "skills/aif-plan/SKILL.md"}}
	upgraded := packageSource(t, files, components, func(manifest map[string]any) { manifest["version"] = "1.1.0" })
	if _, err := e.ImportPackage(ctx, PackageImportRequest{CommandID: "command:upgrade", Directory: upgraded, Reason: "second edition"}); err != nil {
		t.Fatal(err)
	}
	second := pkg.Ref.ID + "@1.1.0"
	if _, _, err := e.PackageComponent(ctx, component.ID, ""); err == nil || !strings.Contains(err.Error(), "package_component_ambiguous") {
		t.Fatalf("an ambiguous component was answered anyway: %v", err)
	}
	// Naming the edition answers again, and names which one it read.
	for _, from := range []string{pkg.Ref.ID + "@" + pkg.Ref.Version, second} {
		if _, _, err := e.PackageComponent(ctx, component.ID, from); err != nil {
			t.Fatalf("%s did not answer for its own component: %v", from, err)
		}
	}
}

func TestRevocationIsTerminalAndNotUndoneByReimport(t *testing.T) {
	e, ctx, pkg, source := importedPilotPackage(t)
	if _, err := e.SetPackageStatus(ctx, PackageLifecycleRequest{CommandID: "command:revoke", ID: pkg.Ref.ID, Version: pkg.Ref.Version, Status: PackageRevoked, Reason: "withdrawn after an incident"}); err != nil {
		t.Fatal(err)
	}
	restore, err := e.SetPackageStatus(ctx, PackageLifecycleRequest{CommandID: "command:restore", ID: pkg.Ref.ID, Version: pkg.Ref.Version, Status: PackageTrusted, Reason: "put it back"})
	if err != nil {
		t.Fatal(err)
	}
	if restore.Receipt.Rejection == nil || restore.Receipt.Rejection.Code != "package_revoked" {
		t.Fatalf("a revoked package was restored by a status change: %+v", restore.Receipt)
	}
	again, err := e.ImportPackage(ctx, PackageImportRequest{CommandID: "command:reimport", Directory: source, Reason: "import it again"})
	if err != nil {
		t.Fatal(err)
	}
	if again.Receipt.Rejection == nil || again.Receipt.Rejection.Code != "package_revoked" {
		t.Fatalf("a revoked package was re-trusted by importing it again: %+v", again.Receipt)
	}
}

func TestQuarantineStopsResolutionAndIsReversible(t *testing.T) {
	e, ctx, pkg, _ := importedPilotPackage(t)
	if _, err := e.SetPackageStatus(ctx, PackageLifecycleRequest{CommandID: "command:quarantine", ID: pkg.Ref.ID, Version: pkg.Ref.Version, Status: PackageQuarantined, Reason: "under review"}); err != nil {
		t.Fatal(err)
	}
	held, err := Open(e.Root, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, resources, err := held.inventoryResources(); err != nil || len(resources) != 0 {
		t.Fatalf("a quarantined package still supplied definitions: %v", err)
	}
	held.Close()
	if _, err := e.SetPackageStatus(ctx, PackageLifecycleRequest{CommandID: "command:restore", ID: pkg.Ref.ID, Version: pkg.Ref.Version, Status: PackageTrusted, Reason: "review cleared it"}); err != nil {
		t.Fatal(err)
	}
	restored, err := Open(e.Root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if _, _, resources, err := restored.inventoryResources(); err != nil || len(resources) != 1 {
		t.Fatalf("a cleared package did not resolve again: %v", err)
	}
}

func TestForkReuseTrustRefusesAWithdrawnSourcePackage(t *testing.T) {
	e, ctx, pkg, _ := importedPilotPackage(t)
	source := Run{Definitions: []PinnedDefinition{{Ref: pkg.Components[0].Ref}}}
	if pin, err := e.reuseTrustPin(ctx, source, []ArtifactRef{{ArtifactID: "artifact:result", Revision: 1, Digest: "sha256:result"}}); err != nil || pin == nil || pin.Key != AuthorityPackagesKey {
		t.Fatalf("trusted source was not pinned for reuse: %+v %v", pin, err)
	}
	if _, err := e.SetPackageStatus(ctx, PackageLifecycleRequest{CommandID: "command:quarantine-reuse", ID: pkg.Ref.ID, Version: pkg.Ref.Version, Status: PackageQuarantined, Reason: "review the source skill"}); err != nil {
		t.Fatal(err)
	}
	_, err := e.reuseTrustPin(ctx, source, []ArtifactRef{{ArtifactID: "artifact:result", Revision: 1, Digest: "sha256:result"}})
	rejectionCode(t, err, "reuse_trust_invalid")
}

func TestRevokedPackageBlocksAdmissionWithoutReopeningEngine(t *testing.T) {
	e, ctx, pkg, _ := importedPilotPackage(t)
	run := Run{Definitions: []PinnedDefinition{{Ref: pkg.Components[0].Ref}}}
	pin, blocked, err := e.revokedPin(ctx, run)
	if err != nil || blocked != nil || pin == nil || pin.Key != AuthorityPackagesKey {
		t.Fatalf("trusted package blocked or was not pinned: %+v %v", blocked, err)
	}
	if _, err := e.SetPackageStatus(ctx, PackageLifecycleRequest{CommandID: "command:revoke-admission", ID: pkg.Ref.ID, Version: pkg.Ref.Version, Status: PackageRevoked, Reason: "security incident"}); err != nil {
		t.Fatal(err)
	}
	_, blocked, err = e.revokedPin(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	rejectionCode(t, blocked, "package_revoked")
}

func TestNewRunRequiresCurrentPackageResolution(t *testing.T) {
	e, ctx, pkg, _ := importedPilotPackage(t)
	refs := []flow.Ref{pkg.Components[0].Ref}
	pin, blocked, err := e.packageAdmissionGate(ctx, refs, true)
	if err != nil || blocked != nil || pin == nil {
		t.Fatalf("trusted package blocked a new run: %+v %v", blocked, err)
	}
	if _, err := e.SetPackageStatus(ctx, PackageLifecycleRequest{CommandID: "command:quarantine-new-run", ID: pkg.Ref.ID, Version: pkg.Ref.Version, Status: PackageQuarantined, Reason: "review the package"}); err != nil {
		t.Fatal(err)
	}
	_, blocked, err = e.packageAdmissionGate(ctx, refs, true)
	if err != nil {
		t.Fatal(err)
	}
	rejectionCode(t, blocked, "package_not_resolvable")
}

func TestSideBySideVersionsAndComponentCollision(t *testing.T) {
	e, ctx, pkg, _ := importedPilotPackage(t)
	// The same component identity from a second resolvable package is refused.
	collide := packageSource(t, map[string]string{"skills/other/SKILL.md": "# Other\n"}, []map[string]any{
		{"kind": "context", "ref": map[string]any{"id": "aif:context/plan-skill", "version": "1.0.0", "digest": rawDigest([]byte("# Other\n"))}, "path": "skills/other/SKILL.md"},
	}, func(manifest map[string]any) { manifest["version"] = "2.0.0" })
	result, err := e.ImportPackage(ctx, PackageImportRequest{CommandID: "command:collide", Directory: collide, Reason: "same component identity"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Receipt.Rejection == nil || result.Receipt.Rejection.Code != "component_identity_conflict" {
		t.Fatalf("two resolvable packages exported one component identity: %+v", result.Receipt)
	}
	// Withdrawing the first frees the identity, so a replacement can be sealed.
	if _, err := e.SetPackageStatus(ctx, PackageLifecycleRequest{CommandID: "command:remove", ID: pkg.Ref.ID, Version: pkg.Ref.Version, Status: PackageRemoved, Reason: "replaced"}); err != nil {
		t.Fatal(err)
	}
	replacement, err := e.ImportPackage(ctx, PackageImportRequest{CommandID: "command:replace", Directory: collide, Reason: "install the replacement"})
	if err != nil {
		t.Fatal(err)
	}
	if replacement.Receipt.Rejection != nil {
		t.Fatalf("a replacement was refused after the previous package was withdrawn: %+v", replacement.Receipt.Rejection)
	}
	record, err := e.Packages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Packages) != 2 {
		t.Fatalf("the withdrawn package was erased instead of kept: %+v", record.Packages)
	}
}

// A dependency must already be installed and still resolvable. Nothing is
// fetched, so a missing link is an explainable refusal naming the chain.
func TestPackageDependencyMustBeInstalledAndResolvable(t *testing.T) {
	e, ctx, base, _ := importedPilotPackage(t)
	dependent := packageSource(t, map[string]string{"skills/extra/SKILL.md": "# Extra\n"}, []map[string]any{
		{"kind": "context", "ref": map[string]any{"id": "aif:context/extra-skill", "version": "1.0.0", "digest": rawDigest([]byte("# Extra\n"))}, "path": "skills/extra/SKILL.md"},
	}, func(manifest map[string]any) {
		manifest["id"] = "aif:package/extra"
		manifest["dependencies"] = []map[string]any{{"id": base.Ref.ID, "version": base.Ref.Version, "digest": base.Ref.Digest}}
	})
	accepted, err := e.ImportPackage(ctx, PackageImportRequest{CommandID: "command:dependent", Directory: dependent, Reason: "depends on the pilot package"})
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Receipt.Rejection != nil {
		t.Fatalf("a satisfiable dependency was refused: %+v", accepted.Receipt.Rejection)
	}

	missing := packageSource(t, map[string]string{"skills/orphan/SKILL.md": "# Orphan\n"}, []map[string]any{
		{"kind": "context", "ref": map[string]any{"id": "aif:context/orphan-skill", "version": "1.0.0", "digest": rawDigest([]byte("# Orphan\n"))}, "path": "skills/orphan/SKILL.md"},
	}, func(manifest map[string]any) {
		manifest["id"] = "aif:package/orphan"
		manifest["dependencies"] = []map[string]any{{"id": "aif:package/absent", "version": "1.0.0", "digest": rawDigest([]byte("absent"))}}
	})
	refused, err := e.ImportPackage(ctx, PackageImportRequest{CommandID: "command:orphan", Directory: missing, Reason: "depends on nothing installed"})
	if err != nil {
		t.Fatal(err)
	}
	if refused.Receipt.Rejection == nil || refused.Receipt.Rejection.Code != "dependency_missing" {
		t.Fatalf("a package with an absent dependency was trusted: %+v", refused.Receipt)
	}
}

// PKG-010: withdrawing a package another one needs is refused, and revoking it
// reaches the dependents through the closure instead of leaving them resolving.
func TestWithdrawalRespectsTheDependencyClosure(t *testing.T) {
	e, ctx, base, _ := importedPilotPackage(t)
	dependent := packageSource(t, map[string]string{"skills/extra/SKILL.md": "# Extra\n"}, []map[string]any{
		{"kind": "context", "ref": map[string]any{"id": "aif:context/extra-skill", "version": "1.0.0", "digest": rawDigest([]byte("# Extra\n"))}, "path": "skills/extra/SKILL.md"},
	}, func(manifest map[string]any) {
		manifest["id"] = "aif:package/extra"
		manifest["dependencies"] = []map[string]any{{"id": base.Ref.ID, "version": base.Ref.Version, "digest": base.Ref.Digest}}
	})
	if _, err := e.ImportPackage(ctx, PackageImportRequest{CommandID: "command:dependent", Directory: dependent, Reason: "depends on the pilot package"}); err != nil {
		t.Fatal(err)
	}
	_, err := e.SetPackageStatus(ctx, PackageLifecycleRequest{CommandID: "command:remove", ID: base.Ref.ID, Version: base.Ref.Version, Status: PackageRemoved, Reason: "orderly removal"})
	rejectionCode(t, err, "package_depended_on")

	// Revocation is an incident, not an orderly removal: it proceeds and the
	// dependent stops resolving with it.
	if _, err := e.SetPackageStatus(ctx, PackageLifecycleRequest{CommandID: "command:revoke", ID: base.Ref.ID, Version: base.Ref.Version, Status: PackageRevoked, Reason: "incident"}); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(e.Root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, _, resources, err := reopened.inventoryResources(); err != nil || len(resources) != 0 {
		t.Fatalf("a package whose dependency was revoked still resolved: %v %+v", err, resources)
	}
	record, err := reopened.Packages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	view := PackageLifecycleView(record)["packages"].([]map[string]any)
	for _, entry := range view {
		if entry["resolvable"].(bool) {
			t.Fatalf("the closure still reported something resolvable: %+v", entry)
		}
	}
}

// Stage acceptance: a failed installation leaves the working directory as it
// was. Nothing half-sealed survives, and the previously resolvable set stands.
func TestFailedInstallationLeavesTheWorkingDirectoryIntact(t *testing.T) {
	e, ctx, pkg, _ := importedPilotPackage(t)
	before, err := os.ReadDir(filepath.Join(e.Root, filepath.FromSlash(PackageRoot)))
	if err != nil {
		t.Fatal(err)
	}
	broken := packageSource(t, map[string]string{"skills/broken/SKILL.md": "# Broken\n"}, []map[string]any{
		{"kind": "context", "ref": map[string]any{"id": "aif:context/broken", "version": "1.0.0", "digest": rawDigest([]byte("# Different\n"))}, "path": "skills/broken/SKILL.md"},
	}, func(manifest map[string]any) { manifest["id"] = "aif:package/broken" })
	if _, err := e.ImportPackage(ctx, PackageImportRequest{CommandID: "command:broken", Directory: broken, Reason: "a component naming other bytes"}); err == nil {
		t.Fatal("a package whose component names other bytes was installed")
	}
	after, err := os.ReadDir(filepath.Join(e.Root, filepath.FromSlash(PackageRoot)))
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("a failed installation left something behind: before=%d after=%d", len(before), len(after))
	}
	for _, entry := range after {
		if strings.Contains(entry.Name(), "pending") {
			t.Fatalf("a staging tree survived a failed installation: %s", entry.Name())
		}
	}
	// The package that was already installed still resolves.
	reopened, err := Open(e.Root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, _, resources, err := reopened.inventoryResources(); err != nil || len(resources) != 1 {
		t.Fatalf("a failed installation disturbed the resolvable set: %v", err)
	}
	if report, err := reopened.VerifyPackage(ctx, pkg.Ref.ID, pkg.Ref.Version); err != nil || len(report.Mismatched) != 0 || len(report.Missing) != 0 {
		t.Fatalf("a failed installation disturbed sealed bytes: %v %+v", err, report)
	}
}

// The shape of a declared output slot lives in the package that declared it.
// Reachable only as a file inside the authority, it is storage, not a contract.
func TestPackageComponentIsReadableByItsDeclaredID(t *testing.T) {
	e, ctx, entry, _ := importedPilotPackage(t)
	manifest, err := os.ReadFile(filepath.Join(e.Root, filepath.FromSlash(entry.Root), PackageManifestFile))
	if err != nil {
		t.Fatal(err)
	}
	var declared packageManifest
	if err := decode(manifest, &declared); err != nil || len(declared.Components) == 0 {
		t.Fatalf("the installed package declares no component: %v", err)
	}
	component := declared.Components[0]
	definition, body, err := e.PackageComponent(ctx, component.Ref.ID, "")
	if err != nil {
		t.Fatalf("a declared component was not readable by its ID: %v", err)
	}
	if definition.Ref != component.Ref || rawDigest(body) != component.Ref.Digest {
		t.Fatalf("the component read back differs: %+v", definition)
	}
	if _, _, err := e.PackageComponent(ctx, "test:schema/absent", ""); refusalCode(err) != "package_component_not_found" {
		t.Fatalf("an unknown component was not refused by name: %v", err)
	}
}

// A package profile that derives component versions from the package build key
// gives every rebuild a new version for a file it never touched. Comparing the
// whole reference then reads "two versions" as "two answers" and demands a
// choice where the bytes are identical — thirteen components of one pilot
// package became unreadable without naming an edition, and every edition held
// the same content. Identity is the digest.
func TestIdenticalComponentBytesUnderTwoVersionsStayReadable(t *testing.T) {
	e, ctx, pkg, _ := importedPilotPackage(t)
	component := pkg.Components[0].Ref
	body := "---\nname: aif-plan\n---\n\n# Plan\n"
	if rawDigest([]byte(body)) != component.Digest {
		t.Fatalf("the fixture body is not the sealed component: %s", component.Digest)
	}
	// A rebuild that changed nothing in this file: same bytes, own version.
	files := map[string]string{"skills/aif-plan/SKILL.md": body, "skills/aif-plan/references/TASK-FORMAT.md": "# Task format\n"}
	components := []map[string]any{{"kind": "context", "ref": map[string]any{"id": component.ID, "version": "1.1.0", "digest": component.Digest}, "path": "skills/aif-plan/SKILL.md"}}
	rebuilt := packageSource(t, files, components, func(manifest map[string]any) { manifest["version"] = "1.1.0" })
	if _, err := e.ImportPackage(ctx, PackageImportRequest{CommandID: "command:rebuild", Directory: rebuilt, Reason: "same file, new build"}); err != nil {
		t.Fatal(err)
	}
	definition, data, err := e.PackageComponent(ctx, component.ID, "")
	if err != nil {
		t.Fatalf("byte-identical editions were refused as ambiguous: %v", err)
	}
	if definition.Ref.Digest != component.Digest || string(data) != body {
		t.Fatalf("the answer is not the sealed content: %+v %s", definition, data)
	}
}

// "more than one, including A and B" reads as "exactly two" when seven trusted
// packages declare the id. The count is what says the list is cut short.
func TestAmbiguityRefusalNamesHowManyDeclareIt(t *testing.T) {
	e, ctx, pkg, _ := importedPilotPackage(t)
	component := pkg.Components[0].Ref
	for index, edition := range []string{"1.1.0", "1.2.0"} {
		body := fmt.Sprintf("---\nname: aif-plan\n---\n\n# Plan, edition %d\n", index)
		files := map[string]string{"skills/aif-plan/SKILL.md": body, "skills/aif-plan/references/TASK-FORMAT.md": "# Task format\n"}
		components := []map[string]any{{"kind": "context", "ref": map[string]any{"id": component.ID, "version": edition, "digest": rawDigest([]byte(body))}, "path": "skills/aif-plan/SKILL.md"}}
		source := packageSource(t, files, components, func(manifest map[string]any) { manifest["version"] = edition })
		if _, err := e.ImportPackage(ctx, PackageImportRequest{CommandID: "command:edition" + edition, Directory: source, Reason: "another edition"}); err != nil {
			t.Fatal(err)
		}
	}
	_, _, err := e.PackageComponent(ctx, component.ID, "")
	if err == nil || !strings.Contains(err.Error(), "package_component_ambiguous") {
		t.Fatalf("three differing editions were answered anyway: %v", err)
	}
	if !strings.Contains(err.Error(), "by 3 trusted packages") {
		t.Fatalf("the refusal does not say how many declare it: %v", err)
	}
}

// Under a profile that derives versions from the build key, the version an
// author wrote in the manifest addresses no installed package. Answering that
// with "no package declares this component" sent the reader to look for the
// component, which is installed twice over.
func TestNamingAnUninstalledEditionSaysSo(t *testing.T) {
	e, ctx, pkg, _ := importedPilotPackage(t)
	component := pkg.Components[0].Ref
	_, _, err := e.PackageComponent(ctx, component.ID, pkg.Ref.ID+"@9.9.9")
	if err == nil || !strings.Contains(err.Error(), "package_not_installed") {
		t.Fatalf("an uninstalled edition was reported as a missing component: %v", err)
	}
	if !strings.Contains(err.Error(), "build key") || !strings.Contains(err.Error(), "package list") {
		t.Fatalf("the refusal does not say why the version missed or where to look: %v", err)
	}
	// The component itself is still readable without naming an edition.
	if _, _, err := e.PackageComponent(ctx, component.ID, ""); err != nil {
		t.Fatalf("the component stopped being readable: %v", err)
	}
}

// A project that follows every package release fills the 512-entry budget by
// being diligent: each import contributes its components and the editions it
// replaced keep theirs. The pilot reached it in one evening over seven package
// releases, and the refusal named the limit without naming the release.
func TestDependencyLimitNamesTheEditionsThatFillIt(t *testing.T) {
	packages := []PackageEntry{}
	for _, version := range []string{"1.0.0", "1.1.0", "1.2.0"} {
		packages = append(packages, PackageEntry{
			Ref:        flow.Ref{ID: "aif:package/classic", Version: version, Digest: rawDigest([]byte(version))},
			Status:     PackageTrusted,
			Components: []Definition{{Ref: flow.Ref{ID: "aif:step/tests", Version: version}}, {Ref: flow.Ref{ID: "aif:step/plan", Version: version}}},
		})
	}
	packages = append(packages, PackageEntry{
		Ref:        flow.Ref{ID: "other:package/tool", Version: "1.0.0", Digest: rawDigest([]byte("tool"))},
		Status:     PackageTrusted,
		Components: []Definition{{Ref: flow.Ref{ID: "other:step/one", Version: "1.0.0"}}},
	})
	note := heaviestPackage(packages)
	for _, expected := range []string{"aif:package/classic", "3 editions", "6 of them"} {
		if !strings.Contains(note, expected) {
			t.Fatalf("the note does not name %q: %q", expected, note)
		}
	}
	// A withdrawn edition supplies no definitions, so it must not be counted as
	// occupying the budget the operator is being asked to free.
	packages[0].Status = PackageRemoved
	if note := heaviestPackage(packages); !strings.Contains(note, "2 editions") {
		t.Fatalf("a removed edition still counts against the budget: %q", note)
	}
	// One edition of one package is not the diagnosis, so nothing is claimed.
	single := []PackageEntry{{Ref: flow.Ref{ID: "aif:package/classic", Version: "1.0.0"}, Status: PackageTrusted}}
	if note := heaviestPackage(single); note != "" {
		t.Fatalf("a single edition was blamed for the budget: %q", note)
	}
}
