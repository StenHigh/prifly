package runtime

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// Removing a package closes it for new resolution; revoking it additionally
// stops old pins from admitting new work. Neither destroys bytes a Run or its
// evidence still holds, and neither erases that an external effect happened.
const (
	PackageTrusted     = "trusted"
	PackageQuarantined = "quarantined"
	PackageRevoked     = "revoked"
	PackageRemoved     = "removed"

	// A bounded scan: an installation with more runs than this refuses to claim
	// it proved nothing holds the package.
	maxPinScanRuns = 1000
)

// PackageVerification reports what the sealed bytes are now, file by file. It
// is a check of storage, not a statement about the package's quality or trust.
type PackageVerification struct {
	SchemaVersion string   `json:"schema_version"`
	Ref           flow.Ref `json:"ref"`
	Status        string   `json:"status"`
	FilesChecked  int      `json:"files_checked"`
	Mismatched    []string `json:"mismatched_paths"`
	Missing       []string `json:"missing_paths"`
	ManifestValid bool     `json:"manifest_valid"`
}

// PackageInspection is the read-only record for one exact sealed package.
// Manifest metadata is read only after its digest matches the recorded
// identity, so changed local bytes are never presented as installed metadata.
type PackageInspection struct {
	SchemaVersion string                  `json:"schema_version"`
	Ref           flow.Ref                `json:"ref"`
	Manifest      PackageManifestMetadata `json:"manifest"`
	Origin        PackageOrigin           `json:"origin"`
	Trust         PackageTrust            `json:"trust"`
	Status        string                  `json:"status"`
	StatusReason  string                  `json:"status_reason,omitempty"`
	StatusChanged *Observation            `json:"status_changed,omitempty"`
	Resolvable    bool                    `json:"resolvable"`
	Dependencies  []flow.Ref              `json:"dependencies"`
	Components    []Definition            `json:"components"`
	Files         []PackageFile           `json:"files"`
	Imported      Observation             `json:"imported"`
}

// PackageManifestMetadata contains the non-executable fields a package
// declared about itself. Requested capabilities are requests, never grants.
type PackageManifestMetadata struct {
	Description           string   `json:"description"`
	RequiresCoreProtocol  string   `json:"requires_core_protocol"`
	RequestedCapabilities []string `json:"requested_capabilities"`
	License               string   `json:"license"`
}

// InspectPackage returns the recorded trust/lifecycle record and the metadata
// from exactly the sealed manifest named by ref. It does not resolve, execute
// or repair any package bytes.
func (e *Engine) InspectPackage(ctx context.Context, ref flow.Ref) (PackageInspection, error) {
	record, _, err := e.readPackages(ctx)
	if err != nil {
		return PackageInspection{}, err
	}
	for _, pkg := range record.Packages {
		if pkg.Ref != ref {
			continue
		}
		if pkg.Root != packageDirectory(ref.ID, ref.Version) || pkg.ManifestDigest != ref.Digest {
			return PackageInspection{}, local.ErrIntegrity
		}
		manifestBytes, err := readLocal(e.Root, pkg.Root+"/"+PackageManifestFile, MaxDefinitionBytes)
		if err != nil {
			return PackageInspection{}, err
		}
		if rawDigest(manifestBytes) != pkg.ManifestDigest || flow.ValidateProtocol("PackageManifest", manifestBytes) != nil {
			return PackageInspection{}, local.ErrIntegrity
		}
		var manifest packageManifest
		if err := decode(manifestBytes, &manifest); err != nil || manifest.ID != ref.ID || manifest.Version != ref.Version {
			return PackageInspection{}, local.ErrIntegrity
		}
		status := pkg.Status
		if status == "" {
			status = PackageTrusted
		}
		return PackageInspection{
			SchemaVersion: "foundation-package-inspection/1", Ref: pkg.Ref,
			Manifest: PackageManifestMetadata{Description: manifest.Description, RequiresCoreProtocol: manifest.RequiresCoreProtocol, RequestedCapabilities: manifest.RequestedCapabilities, License: manifest.License},
			Origin:   pkg.Origin, Trust: pkg.Trust, Status: status, StatusReason: pkg.StatusReason, StatusChanged: pkg.StatusChanged,
			Resolvable: resolvablePackages(record)[pkg.Ref], Dependencies: pkg.Dependencies, Components: pkg.Components, Files: pkg.Files, Imported: pkg.Imported,
		}, nil
	}
	return PackageInspection{}, local.ErrNotFound
}

// VerifyPackage re-reads every declared byte of a sealed package. A missing or
// changed file is reported, never repaired: repairing would replace evidence
// with a guess.
func (e *Engine) VerifyPackage(ctx context.Context, id, version string) (PackageVerification, error) {
	record, _, err := e.readPackages(ctx)
	if err != nil {
		return PackageVerification{}, err
	}
	for _, pkg := range record.Packages {
		if pkg.Ref.ID != id || pkg.Ref.Version != version {
			continue
		}
		report := PackageVerification{SchemaVersion: "foundation-package-verification/1", Ref: pkg.Ref, Status: pkg.Status, Mismatched: []string{}, Missing: []string{}}
		manifest, err := readLocal(e.Root, pkg.Root+"/"+PackageManifestFile, MaxDefinitionBytes)
		report.ManifestValid = err == nil && rawDigest(manifest) == pkg.ManifestDigest
		for _, file := range pkg.Files {
			data, err := readLocal(e.Root, pkg.Root+"/"+file.Path, MaxPackageFileBytes)
			if err != nil {
				report.Missing = append(report.Missing, file.Path)
				continue
			}
			report.FilesChecked++
			if rawDigest(data) != file.Digest || int64(len(data)) != file.SizeBytes {
				report.Mismatched = append(report.Mismatched, file.Path)
			}
		}
		return report, nil
	}
	return PackageVerification{}, local.ErrNotFound
}

type PackageLifecycleRequest struct {
	CommandID string
	ID        string
	Version   string
	Status    string
	Reason    string
}

// SetPackageStatus records removal, quarantine or revocation. Removal refuses
// while a non-terminal Run still holds the package, so uninstalling never
// breaks work in flight. Revocation is deliberately allowed then: its whole
// purpose is to stop admissions that old pins would otherwise still permit.
func (e *Engine) SetPackageStatus(ctx context.Context, c PackageLifecycleRequest) (local.AuthorityApplyResult, error) {
	if e.ReadOnly {
		return local.AuthorityApplyResult{}, local.ErrReadOnly
	}
	if c.CommandID == "" || c.ID == "" || c.Version == "" || c.Reason == "" || len(c.Reason) > 4096 {
		return local.AuthorityApplyResult{}, errors.New("explicit command, package identity and reason required")
	}
	switch c.Status {
	case PackageRemoved, PackageQuarantined, PackageRevoked, PackageTrusted:
	default:
		return local.AuthorityApplyResult{}, errors.New("a package status is trusted, quarantined, revoked or removed")
	}
	control, _, err := e.ensureControl(ctx)
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	if !control.allows(e.owner, "project", e.Config.ID, ControlOperationTrust) {
		return local.AuthorityApplyResult{}, local.Reject("object_access_denied", "the session principal cannot change package trust")
	}
	holders, err := e.packageHolders(ctx, c.ID, c.Version)
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	if c.Status == PackageRemoved && len(holders) != 0 {
		return local.AuthorityApplyResult{}, local.Reject("package_in_use", "a non-terminal run still holds this package: "+holders[0])
	}
	record, _, err := e.readPackages(ctx)
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	for _, pkg := range record.Packages {
		if pkg.Ref.ID != c.ID || pkg.Ref.Version != c.Version {
			continue
		}
		// Removal is orderly and refuses to break a dependent. Revocation is
		// not: an incident reaches the dependents through the closure instead.
		if c.Status == PackageRemoved {
			if needed := dependents(record, pkg.Ref); len(needed) != 0 {
				return local.AuthorityApplyResult{}, local.Reject("package_depended_on", "another installed package still depends on this one: "+needed[0])
			}
		}
	}
	command, err := canonical(map[string]any{"operation": "package.status", "command_id": c.CommandID, "package_id": c.ID, "package_version": c.Version, "status": c.Status, "reason": c.Reason})
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	return e.Store.ApplyAuthority(ctx, local.AuthorityCommand{ID: c.CommandID, Actor: e.owner, Key: AuthorityPackagesKey, Payload: command}, func(s local.AuthoritySnapshot) (local.AuthorityChange, error) {
		var record PackageRecord
		if err := decode(s.Data, &record); err != nil {
			return local.AuthorityChange{}, err
		}
		if record.SchemaVersion != AuthorityPackagesVersion || record.AuthorityID != e.Installation.ID {
			return local.AuthorityChange{}, errors.New("unsupported or foreign package record")
		}
		found := false
		for i := range record.Packages {
			pkg := &record.Packages[i]
			if pkg.Ref.ID != c.ID || pkg.Ref.Version != c.Version {
				continue
			}
			if pkg.Status == PackageRevoked && c.Status != PackageRevoked {
				return local.AuthorityChange{}, local.Reject("package_revoked", "a revoked package is not restored by a status change")
			}
			obs := e.clock.now()
			pkg.Status, pkg.StatusReason, pkg.StatusChanged = c.Status, c.Reason, &obs
			found = true
		}
		if !found {
			return local.AuthorityChange{}, local.Reject("not_found", "no trusted package with this identity")
		}
		data, err := canonicalState(record)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		return local.AuthorityChange{Data: data, Result: json.RawMessage(`{"status":"` + c.Status + `"}`)}, nil
	})
}

// packageHolders names the non-terminal runs whose pinned closure contains a
// component of this package. It is the honest answer to "who still needs it".
func (e *Engine) packageHolders(ctx context.Context, id, version string) ([]string, error) {
	record, _, err := e.readPackages(ctx)
	if err != nil {
		return nil, err
	}
	components := map[flow.Ref]bool{}
	for _, pkg := range record.Packages {
		if pkg.Ref.ID == id && pkg.Ref.Version == version {
			for _, component := range pkg.Components {
				components[component.Ref] = true
			}
		}
	}
	if len(components) == 0 {
		return nil, nil
	}
	snapshots, _, err := e.Store.ReadAllAt(ctx, -1, maxPinScanRuns)
	if err != nil {
		return nil, err
	}
	if len(snapshots) >= maxPinScanRuns {
		return nil, errors.New("scan_limit: this installation holds more runs than one bounded scan can prove")
	}
	holders := []string{}
	for _, snapshot := range snapshots {
		var r Run
		if err := decodeState(snapshot.Data, &r); err != nil {
			continue
		}
		if r.terminal() {
			continue
		}
		for _, definition := range r.Definitions {
			if components[definition.Ref] {
				holders = append(holders, r.ID)
				break
			}
		}
		if len(holders) == 0 {
			for _, resource := range r.ContextResources {
				if components[resource.Ref] {
					holders = append(holders, r.ID)
					break
				}
			}
		}
	}
	return holders, nil
}

// withdrawn reports the package's own status. It is not the whole answer: a
// package whose dependency was withdrawn stops resolving too.
func (p PackageEntry) withdrawn() bool {
	return p.Status != "" && p.Status != PackageTrusted
}

// resolvablePackages computes the closure once. A package supplies definitions
// only if it and every package it depends on are still trusted: resolving a
// package whose dependency was withdrawn would use bytes nobody vouches for.
func resolvablePackages(record PackageRecord) map[flow.Ref]bool {
	byRef := map[flow.Ref]PackageEntry{}
	for _, pkg := range record.Packages {
		byRef[pkg.Ref] = pkg
	}
	resolvable := map[flow.Ref]bool{}
	var visit func(flow.Ref, int) bool
	visit = func(ref flow.Ref, depth int) bool {
		if decided, seen := resolvable[ref]; seen {
			return decided
		}
		pkg, installed := byRef[ref]
		if !installed || pkg.withdrawn() || depth > MaxPackageDepth {
			resolvable[ref] = false
			return false
		}
		// Mark before recursing: a declaration that somehow closed a loop must
		// not spin, and a loop is not a reason to trust anything in it.
		resolvable[ref] = false
		for _, dependency := range pkg.Dependencies {
			if !visit(dependency, depth+1) {
				return false
			}
		}
		resolvable[ref] = true
		return true
	}
	for _, pkg := range record.Packages {
		visit(pkg.Ref, 0)
	}
	return resolvable
}

// packageDepth measures the declared closure so an import states its bound
// instead of discovering it as a stack overflow.
func packageDepth(record PackageRecord, dependencies []flow.Ref, depth int) int {
	if depth > MaxPackageDepth || len(dependencies) == 0 {
		return depth
	}
	deepest := depth
	for _, dependency := range dependencies {
		for _, pkg := range record.Packages {
			if pkg.Ref != dependency {
				continue
			}
			if found := packageDepth(record, pkg.Dependencies, depth+1); found > deepest {
				deepest = found
			}
		}
	}
	return deepest
}

// dependents names the resolvable packages that declare this one. Withdrawing
// a package another still needs is refused rather than silently breaking it.
func dependents(record PackageRecord, ref flow.Ref) []string {
	holders := []string{}
	for _, pkg := range record.Packages {
		if pkg.withdrawn() {
			continue
		}
		for _, dependency := range pkg.Dependencies {
			if dependency == ref {
				holders = append(holders, pkg.Ref.ID+"@"+pkg.Ref.Version)
			}
		}
	}
	return holders
}

func runPackageRefs(r Run) []flow.Ref {
	refs := make([]flow.Ref, 0, len(r.Definitions)+len(r.ContextResources))
	for _, definition := range r.Definitions {
		refs = append(refs, definition.Ref)
	}
	for _, resource := range r.ContextResources {
		refs = append(refs, resource.Ref)
	}
	return refs
}

// packageAdmissionGate evaluates the current package authority state and
// returns a pin for the exact record it checked. New Runs require every
// package to remain resolvable; an existing Run is interrupted only by direct
// security revocation, not orderly removal or quarantine of its old pin.
func (e *Engine) packageAdmissionGate(ctx context.Context, refs []flow.Ref, requireResolvable bool) (*local.ControlPin, error, error) {
	record, version, err := e.readPackages(ctx)
	if err != nil {
		return nil, nil, err
	}
	wanted := map[flow.Ref]bool{}
	for _, ref := range refs {
		wanted[ref] = true
	}
	resolvable := resolvablePackages(record)
	tracked := false
	for _, pkg := range record.Packages {
		for _, component := range pkg.Components {
			if !wanted[component.Ref] {
				continue
			}
			tracked = true
			if requireResolvable && !resolvable[pkg.Ref] {
				return &local.ControlPin{Key: AuthorityPackagesKey, Version: version}, local.Reject("package_not_resolvable", "a package no longer trusted supplied "+component.Ref.ID+" from "+pkg.Ref.ID), nil
			}
			if !requireResolvable && pkg.Status == PackageRevoked {
				return &local.ControlPin{Key: AuthorityPackagesKey, Version: version}, local.Reject("package_revoked", "a revoked package supplied "+component.Ref.ID+" from "+pkg.Ref.ID), nil
			}
		}
	}
	if !tracked {
		return nil, nil, nil
	}
	return &local.ControlPin{Key: AuthorityPackagesKey, Version: version}, nil, nil
}

func (e *Engine) revokedPin(ctx context.Context, r Run) (*local.ControlPin, error, error) {
	return e.packageAdmissionGate(ctx, runPackageRefs(r), false)
}

// reuseTrustPin rechecks the current lifecycle of every package component that
// contributed to a source Run. A fork may copy ordinary immutable output, but
// it may not present that copy as trusted reuse after the supplying package or
// one of its dependencies is no longer trusted. The returned pin closes the gap
// between this evaluation and linked-Run creation.
func (e *Engine) reuseTrustPin(ctx context.Context, source Run, refs []ArtifactRef) (*local.ControlPin, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	record, version, err := e.readPackages(ctx)
	if err != nil {
		return nil, err
	}
	pinned := map[flow.Ref]bool{}
	for _, definition := range source.Definitions {
		pinned[definition.Ref] = true
	}
	for _, resource := range source.ContextResources {
		pinned[resource.Ref] = true
	}
	resolvable := resolvablePackages(record)
	tracked := false
	for _, pkg := range record.Packages {
		for _, component := range pkg.Components {
			if !pinned[component.Ref] {
				continue
			}
			tracked = true
			if !resolvable[pkg.Ref] {
				return nil, local.Reject("reuse_trust_invalid", "a package no longer trusted supplied "+component.Ref.ID+" to the source run")
			}
		}
	}
	if !tracked {
		return nil, nil
	}
	return &local.ControlPin{Key: AuthorityPackagesKey, Version: version}, nil
}

// PackageLifecycleView is the reference CLI projection of installed packages.
func PackageLifecycleView(record PackageRecord) map[string]any {
	resolvable := resolvablePackages(record)
	packages := []map[string]any{}
	for _, pkg := range record.Packages {
		status := pkg.Status
		if status == "" {
			status = PackageTrusted
		}
		packages = append(packages, map[string]any{"ref": pkg.Ref, "status": status, "reason": pkg.StatusReason, "resolvable": resolvable[pkg.Ref], "dependencies": pkg.Dependencies, "components": len(pkg.Components)})
	}
	return map[string]any{"schema_version": "foundation-package-lifecycle/1", "packages": packages}
}
