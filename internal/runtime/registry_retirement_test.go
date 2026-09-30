package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

// retirementEngine is a core authority holding one Run, with its local
// registry filled so that fill entries plus the trusted editions' components
// is the whole budget.
func retirementEngine(t *testing.T, local int) *Engine {
	t.Helper()
	e, _ := driverProject(t, "pass", 10000, programExternalWrite("idempotent"))
	var registry RegistryFile
	readRuntimeJSON(t, filepath.Join(e.Root, "definitions.json"), &registry)
	filler := []byte(`{"type":"object"}`)
	if err := os.MkdirAll(filepath.Join(e.Root, "schemas"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.Root, "schemas/filler.json"), filler, 0600); err != nil {
		t.Fatal(err)
	}
	for index := len(registry.Entries); index < local; index++ {
		registry.Entries = append(registry.Entries, Definition{Ref: flow.Ref{ID: fmt.Sprintf("example:schema/filler-%d", index), Version: "1.0.0", Digest: rawDigest(filler)}, Kind: "schema", Path: "schemas/filler.json"})
	}
	writeRuntimeJSON(t, filepath.Join(e.Root, "definitions.json"), registry)
	return e
}

// editionSource is one build of a package: its components carry the
// edition's version, as a compiled edition's do.
func editionSource(t *testing.T, version string, components int) string {
	t.Helper()
	files, declared := map[string]string{}, []map[string]any{}
	for index := range components {
		path := fmt.Sprintf("contexts/c%d.md", index)
		body := fmt.Sprintf("# Context %d of %s\n", index, version)
		files[path] = body
		declared = append(declared, map[string]any{"kind": "context", "ref": map[string]any{"id": fmt.Sprintf("example:context/c%d", index), "version": version, "digest": rawDigest([]byte(body))}, "path": path})
	}
	return packageSource(t, files, declared, func(manifest map[string]any) {
		manifest["id"], manifest["version"] = "example:package/editions", version
	})
}

func importEdition(t *testing.T, e *Engine, version string, retire *RegistryRetirement) PackageEntry {
	t.Helper()
	result, err := e.ImportPackage(context.Background(), PackageImportRequest{CommandID: "command:import-" + version, Directory: editionSource(t, version, 3), Reason: "edition " + version, Retire: retire})
	if err != nil {
		t.Fatal(err)
	}
	if result.Receipt.Rejection != nil {
		t.Fatalf("edition %s was refused: %+v", version, result.Receipt.Rejection)
	}
	return packageNamed(t, e, version)
}

func packageNamed(t *testing.T, e *Engine, version string) PackageEntry {
	t.Helper()
	record, err := e.Packages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range record.Packages {
		if pkg.Ref.ID == "example:package/editions" && pkg.Ref.Version == version {
			return pkg
		}
	}
	t.Fatalf("edition %s is not recorded", version)
	return PackageEntry{}
}

func planFor(t *testing.T, e *Engine, version string) (*RegistryRetirement, flow.Ref) {
	t.Helper()
	source := editionSource(t, version, 3)
	data, err := os.ReadFile(filepath.Join(source, PackageManifestFile))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		ID      string `json:"id"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	ref := flow.Ref{ID: manifest.ID, Version: manifest.Version, Digest: rawDigest(data)}
	plan, err := e.PlanRegistryRetirement(context.Background(), ref, 3)
	if err != nil {
		t.Fatal(err)
	}
	return plan, ref
}

// Three builds of one package are trusted; a fourth does not fit. The oldest
// by import -- not by version string, 1.10.0 was imported before 1.9.0 -- is
// withdrawn, the newest earlier edition is kept for going back, and nothing
// is written until the launch applies it with its own edition.
func TestTheOldestEditionIsWithdrawnSoTheNewOneFits(t *testing.T) {
	t.Parallel()
	e := retirementEngine(t, MaxLocalRegistryEntries-9)
	ctx := context.Background()
	for _, version := range []string{"1.10.0", "1.9.0", "1.11.0"} {
		importEdition(t, e, version, nil)
	}
	plan, _ := planFor(t, e, "1.12.0")
	if plan == nil || len(plan.Retire) != 1 || plan.Retire[0].Ref.Version != "1.10.0" || plan.Retire[0].Entries != 3 || plan.Retire[0].Undo != "package restore --id example:package/editions --version 1.10.0 --reason TEXT" {
		t.Fatalf("the oldest imported edition is not the one withdrawn: %+v", plan)
	}
	if plan.Budget.Entries != MaxLocalRegistryEntries+3 || plan.After.Entries != MaxLocalRegistryEntries || plan.After.WouldRefuse != "" {
		t.Fatalf("the budget before and after is misreported: %+v %+v", plan.Budget, plan.After)
	}
	kept := false
	for _, protected := range plan.Protected {
		if protected.Ref.Version == "1.11.0" && strings.Contains(protected.Reason, "newest earlier edition") {
			kept = true
		}
	}
	if !kept {
		t.Fatalf("the newest earlier edition is not named as kept: %+v", plan.Protected)
	}
	if status := packageNamed(t, e, "1.10.0").Status; status != "" && status != PackageTrusted {
		t.Fatalf("planning withdrew an edition: %s", status)
	}
	importEdition(t, e, "1.12.0", plan)
	retired := packageNamed(t, e, "1.10.0")
	if retired.Status != PackageRemoved || !strings.Contains(retired.StatusReason, "package restore --id example:package/editions --version 1.10.0") {
		t.Fatalf("the withdrawn edition does not say what withdrew it and how to undo it: %+v", retired)
	}
	for _, version := range []string{"1.9.0", "1.11.0", "1.12.0"} {
		if status := packageNamed(t, e, version).Status; status != "" && status != PackageTrusted {
			t.Fatalf("edition %s was withdrawn with the oldest: %s", version, status)
		}
	}
	// The sealed bytes stay: withdrawal is not deletion.
	if _, err := os.Stat(filepath.Join(e.Root, filepath.FromSlash(retired.Components[0].Path))); err != nil {
		t.Fatalf("withdrawal destroyed the sealed bytes: %v", err)
	}
	if _, _, _, err := e.inventoryResources(); err != nil {
		t.Fatalf("the registry still overflows after the plan: %v", err)
	}
	// The same command again changes nothing more.
	again, err := e.ImportPackage(ctx, PackageImportRequest{CommandID: "command:import-1.12.0", Directory: editionSource(t, "1.12.0", 3), Reason: "edition 1.12.0", Retire: plan})
	if err != nil || !again.Duplicate {
		t.Fatalf("a repeated command was applied again: %+v %v", again, err)
	}
}

// A Run started after the plan counted the Runs may be one on an edition the
// plan withdraws. Applying such a plan is refused, and nothing changes: not
// the withdrawal, not the new edition's trust.
func TestAPlanIsRefusedWhenARunAppearedAfterIt(t *testing.T) {
	t.Parallel()
	e := retirementEngine(t, MaxLocalRegistryEntries-9)
	for _, version := range []string{"1.0.0", "2.0.0", "3.0.0"} {
		importEdition(t, e, version, nil)
	}
	plan, _ := planFor(t, e, "4.0.0")
	if plan == nil || len(plan.Retire) != 1 {
		t.Fatalf("no withdrawal was planned: %+v", plan)
	}
	driverStart(t, e)
	result, err := e.ImportPackage(context.Background(), PackageImportRequest{CommandID: "command:import-stale", Directory: editionSource(t, "4.0.0", 3), Reason: "edition 4.0.0", Retire: plan})
	if err != nil {
		t.Fatal(err)
	}
	if result.Receipt.Rejection == nil || result.Receipt.Rejection.Code != "runs_changed" {
		t.Fatalf("a plan older than a Run was applied: %+v", result.Receipt)
	}
	record, err := e.Packages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range record.Packages {
		if pkg.Ref.Version == "4.0.0" || pkg.withdrawn() {
			t.Fatalf("a refused plan changed the packages: %+v", pkg)
		}
	}
}

// When withdrawing every edition that may go is not enough, the refusal says
// how many entries are needed and which editions stay, and why.
func TestAPlanThatCannotMakeRoomNamesWhatStays(t *testing.T) {
	t.Parallel()
	e := retirementEngine(t, MaxLocalRegistryEntries-6)
	for _, version := range []string{"1.0.0", "2.0.0"} {
		importEdition(t, e, version, nil)
	}
	_, err := e.PlanRegistryRetirement(context.Background(), flow.Ref{ID: "example:package/editions", Version: "3.0.0", Digest: "sha256:" + strings.Repeat("3", 64)}, 9)
	if refusalCode(err) != "dependency_limit" {
		t.Fatalf("an edition that cannot fit was planned: %v", err)
	}
	for _, part := range []string{"2.0.0 (the newest earlier edition is kept", "package restore --id ID --version VERSION --reason TEXT"} {
		if !strings.Contains(err.Error(), part) {
			t.Fatalf("the refusal does not say %q: %v", part, err)
		}
	}
}
