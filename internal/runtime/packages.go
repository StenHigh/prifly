package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// Package import separates fetching, checking and registration: nothing in the
// imported bytes is executed, and no lifecycle script or README instruction is
// honoured. Registration is one authority transaction, so a failed import
// leaves the previous resolvable set intact.
const (
	AuthorityPackagesKey     = "packages"
	AuthorityPackagesVersion = "authority-packages/1"

	PackageManifestFile = "prifly.package.json"
	PackageRoot         = ".prifly/packages"

	MaxPackagePayloadFiles = 10000
	MaxPackageFileBytes    = MaxArtifactBytes
	MaxPackageDependencies = 32
	// A closure deeper than this is refused rather than walked further: the
	// bound is stated instead of discovered as a stack overflow.
	MaxPackageDepth = 16
)

type PackageRecord struct {
	SchemaVersion string         `json:"schema_version"`
	AuthorityID   string         `json:"authority_id"`
	Packages      []PackageEntry `json:"packages"`
}

type PackageEntry struct {
	Ref            flow.Ref      `json:"ref"`
	ManifestDigest string        `json:"manifest_digest"`
	Root           string        `json:"root"`
	Origin         PackageOrigin `json:"origin"`
	Trust          PackageTrust  `json:"trust"`
	Components     []Definition  `json:"components"`
	Files          []PackageFile `json:"files"`
	Dependencies   []flow.Ref    `json:"dependencies,omitempty"`
	Imported       Observation   `json:"imported"`
	// An entry written before the lifecycle existed carries no status, which
	// honestly means it was trusted and never withdrawn.
	Status        string       `json:"status,omitempty"`
	StatusReason  string       `json:"status_reason,omitempty"`
	StatusChanged *Observation `json:"status_changed,omitempty"`
}

// Location is what the importer stated, not a verified provenance claim. The
// digest is the only identity that survives a moved or renamed source.
type PackageOrigin struct {
	Kind     string `json:"kind"`
	Location string `json:"location"`
	// The archive hash is its own quantity: an archive that hashes correctly
	// still says nothing about who sealed the manifest inside it.
	ArchiveDigest string `json:"archive_digest,omitempty"`
}

type PackageTrust struct {
	Decision string `json:"decision"`
	// SignedBy names the verified trust root, when one signed the manifest.
	// An empty value means nobody outside this installation vouched for it.
	SignedBy           string      `json:"signed_by,omitempty"`
	ActorID            string      `json:"actor_id"`
	Reason             string      `json:"reason"`
	ControlAdmissionID string      `json:"control_admission_id"`
	IntentDigest       string      `json:"intent_digest"`
	Decided            Observation `json:"decided"`
}

type PackageFile struct {
	Path      string `json:"path"`
	Digest    string `json:"digest"`
	SizeBytes int64  `json:"size_bytes"`
	MediaType string `json:"media_type"`
	Role      string `json:"role"`
}

type packageManifest struct {
	SchemaVersion         string              `json:"schema_version"`
	ID                    string              `json:"id"`
	Version               string              `json:"version"`
	Description           string              `json:"description"`
	RequiresCoreProtocol  string              `json:"requires_core_protocol"`
	Dependencies          []flow.Ref          `json:"dependencies"`
	Components            []manifestComponent `json:"components"`
	Files                 []PackageFile       `json:"files"`
	RequestedCapabilities []string            `json:"requested_capabilities"`
	License               string              `json:"license"`
}

type manifestComponent struct {
	Kind string   `json:"kind"`
	Ref  flow.Ref `json:"ref"`
	Path string   `json:"path"`
}

// The manifest names a component kind; the registry names how the bytes are
// interpreted. A ToolDescriptor is sealed and checked here but remains inert:
// action admission and delivery belong to their own slice.
func registryKind(kind string) (string, error) {
	switch kind {
	case "step", "workflow", "schema", "tool":
		return kind, nil
	case "context":
		return "resource", nil
	}
	return "", fmt.Errorf("unsupported_component: package component kind %q is not admitted by this installation", kind)
}

// A context resource declares its representation through the declared media
// type of its own payload file, never through a guess about the extension.
func resourceEncoding(mediaType string) (string, error) {
	base := strings.TrimSpace(strings.Split(mediaType, ";")[0])
	if base == "application/json" {
		return "json", nil
	}
	if strings.HasPrefix(base, "text/") {
		return "utf8_text", nil
	}
	return "", fmt.Errorf("unsupported_component: context resource media type %q is neither JSON nor text", mediaType)
}

func packageDirectory(id, version string) string {
	sum := sha256.Sum256([]byte(id + "\x00" + version))
	return PackageRoot + "/" + hex.EncodeToString(sum[:])
}

func (e *Engine) readPackages(ctx context.Context) (PackageRecord, int64, error) {
	snapshot, err := e.Store.ReadAuthority(ctx, AuthorityPackagesKey)
	if errors.Is(err, local.ErrNotFound) {
		return PackageRecord{SchemaVersion: AuthorityPackagesVersion, AuthorityID: e.Installation.ID, Packages: []PackageEntry{}}, 0, nil
	}
	if err != nil {
		return PackageRecord{}, 0, err
	}
	var record PackageRecord
	if err := decode(snapshot.Data, &record); err != nil {
		return PackageRecord{}, 0, err
	}
	if record.SchemaVersion != AuthorityPackagesVersion || record.AuthorityID != e.Installation.ID {
		return PackageRecord{}, 0, errors.New("unsupported or foreign package record")
	}
	return record, snapshot.Version, nil
}

// Packages lists the trusted packages of this installation.
func (e *Engine) Packages(ctx context.Context) (PackageRecord, error) {
	record, _, err := e.readPackages(ctx)
	return record, err
}

type PackageImportRequest struct {
	CommandID string
	Directory string
	Reason    string
	Origin    PackageOrigin
	Signature *PackageSignature
}

// ImportPackage seals a local package directory, verifies every declared byte
// against the manifest and registers its components under one explicit trust
// decision. It never executes the package and never registers a file the
// manifest did not declare.
func (e *Engine) ImportPackage(ctx context.Context, request PackageImportRequest) (local.AuthorityApplyResult, error) {
	if e.ReadOnly {
		return local.AuthorityApplyResult{}, local.ErrReadOnly
	}
	if request.CommandID == "" || request.Reason == "" || len(request.Reason) > 4096 || request.Directory == "" {
		return local.AuthorityApplyResult{}, errors.New("explicit command, source directory and trust reason required")
	}
	if e.Config.Configuration.SemanticsProfile != flow.CoreProfile {
		return local.AuthorityApplyResult{}, errors.New("unsupported: package import requires core-workflow/1")
	}
	source, err := filepath.Abs(request.Directory)
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	source, err = filepath.EvalSymlinks(source)
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	if overlaps(filepath.ToSlash(source), filepath.ToSlash(e.Root)) {
		return local.AuthorityApplyResult{}, errors.New("unsafe_source: a package source cannot overlap the authority project root")
	}
	manifestBytes, manifest, err := readPackageManifest(source)
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	payload, err := sealPackagePayload(source, manifest)
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	root := packageDirectory(manifest.ID, manifest.Version)
	components, err := packageComponents(manifest, payload, root)
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	manifestDigest := rawDigest(manifestBytes)
	ref := flow.Ref{ID: manifest.ID, Version: manifest.Version, Digest: manifestDigest}
	origin := request.Origin
	if origin.Kind == "" {
		origin = PackageOrigin{Kind: "local_directory", Location: source}
	}
	if err := e.materializePackage(root, manifest, payload, manifestBytes); err != nil {
		return local.AuthorityApplyResult{}, err
	}
	// Trust is an authority control decision over the exact manifest digest, so
	// the same id/version with other bytes cannot inherit an earlier decision.
	intentPayload, err := canonical(map[string]any{"package_ref": ref, "manifest_digest": manifestDigest, "component_count": len(components), "reason": request.Reason})
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	_, reg, err := e.Inventory()
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	intent, intentBytes, expiry, err := e.controlIntentFor("package.trust", "project", e.Config.ID, e.Config.DefaultPolicyRef, reg, request.CommandID, intentPayload)
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	command, err := canonical(map[string]any{"operation": "package.trust", "command_id": request.CommandID, "package_ref": ref, "control_intent_ref": intent.Ref(), "approval_refs": []any{}, "reason": request.Reason})
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	control, _, err := e.ensureControl(ctx)
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	// Trust widens what may later be admitted, so it needs its own object
	// access. A control stop does not block it: a stop forbids new admissions,
	// and control decisions are exactly what stays available under one.
	if !control.allows(e.owner, "project", e.Config.ID, ControlOperationTrust) {
		return local.AuthorityApplyResult{}, local.Reject("object_access_denied", "the session principal cannot record a package trust decision")
	}
	// Signature verification is separate from the local trust decision: it
	// proves who sealed the bytes, not that this installation may run them.
	// A key travelling with the package is never consulted.
	signer := ""
	if request.Signature != nil {
		if err := control.verifySignature(*request.Signature, manifestDigest); err != nil {
			return local.AuthorityApplyResult{}, err
		}
		signer = request.Signature.TrustRootID
	} else if len(control.TrustRoots) != 0 {
		return local.AuthorityApplyResult{}, local.Reject("signature_required", "this installation records trust roots, so a package arrives with a detached signature")
	}
	result, err := e.Store.ApplyAuthority(ctx, local.AuthorityCommand{ID: request.CommandID, Actor: e.owner, Key: AuthorityPackagesKey, Payload: command}, func(s local.AuthoritySnapshot) (local.AuthorityChange, error) {
		record := PackageRecord{SchemaVersion: AuthorityPackagesVersion, AuthorityID: e.Installation.ID, Packages: []PackageEntry{}}
		if s.Version > 0 {
			if err := decode(s.Data, &record); err != nil {
				return local.AuthorityChange{}, err
			}
			if record.SchemaVersion != AuthorityPackagesVersion || record.AuthorityID != e.Installation.ID {
				return local.AuthorityChange{}, errors.New("unsupported or foreign package record")
			}
		}
		obs := e.clock.now()
		if err := controlIntentCurrent(intent, expiry, obs); err != nil {
			return local.AuthorityChange{}, err
		}
		for _, existing := range record.Packages {
			if existing.Ref.ID == ref.ID && existing.Ref.Version == ref.Version {
				if existing.Status == PackageRevoked {
					return local.AuthorityChange{}, local.Reject("package_revoked", "this package revision was revoked and is not re-trusted by importing it again")
				}
				if existing.ManifestDigest == manifestDigest {
					return local.AuthorityChange{}, local.Reject("package_present", "this exact package revision is already trusted")
				}
				return local.AuthorityChange{}, local.Reject("package_identity_conflict", "the same package id and version is already trusted with other bytes")
			}
		}
		// Side-by-side versions are admitted; two resolvable packages exporting
		// the same component identity are not.
		// A dependency must already be installed, resolvable and byte-identical
		// to what this package names. Resolution never fetches: the chain is
		// stated, so a missing link is an explainable refusal.
		resolvable := resolvablePackages(record)
		for _, dependency := range manifest.Dependencies {
			installed := false
			for _, existing := range record.Packages {
				if existing.Ref == dependency {
					installed = true
					if !resolvable[dependency] {
						return local.AuthorityChange{}, local.Reject("dependency_not_resolvable", "dependency "+dependency.ID+"@"+dependency.Version+" is withdrawn or depends on a withdrawn package")
					}
				}
			}
			if !installed {
				return local.AuthorityChange{}, local.Reject("dependency_missing", "dependency "+dependency.ID+"@"+dependency.Version+" with this exact digest is not installed")
			}
		}
		if depth := packageDepth(record, manifest.Dependencies, 1); depth > MaxPackageDepth {
			return local.AuthorityChange{}, local.Reject("dependency_limit", "the dependency closure is deeper than this installation resolves")
		}
		taken := map[string]bool{}
		for _, existing := range record.Packages {
			if !resolvable[existing.Ref] {
				continue
			}
			for _, component := range existing.Components {
				taken[component.Ref.ID+"@"+component.Ref.Version] = true
			}
		}
		for _, component := range components {
			if taken[component.Ref.ID+"@"+component.Ref.Version] {
				return local.AuthorityChange{}, local.Reject("component_identity_conflict", "another trusted package already exports "+component.Ref.ID)
			}
		}
		admissionID := derivedID("control-admission", e.owner, request.CommandID)
		admission := map[string]any{"schema_version": "1", "id": admissionID, "scope": "project", "scope_id": e.Config.ID, "command_id": request.CommandID, "intent_digest": rawDigest(intentBytes), "approval_refs": []any{}, "control_epoch": 0, "admitted_at": obs.UTC}
		ab, err := canonical(admission)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		if err := flow.ValidateProtocol("ControlAdmission", ab); err != nil {
			return local.AuthorityChange{}, err
		}
		record.Packages = append(record.Packages, PackageEntry{
			Ref: ref, ManifestDigest: manifestDigest, Root: root,
			Origin:       origin,
			Dependencies: append([]flow.Ref{}, manifest.Dependencies...),
			Trust:        PackageTrust{Decision: "local_owner_accepted", ActorID: e.owner, Reason: request.Reason, ControlAdmissionID: admissionID, IntentDigest: rawDigest(intentBytes), SignedBy: signer, Decided: obs},
			Components:   components, Files: manifest.Files, Imported: obs,
		})
		sort.Slice(record.Packages, func(i, j int) bool { return record.Packages[i].Ref.String() < record.Packages[j].Ref.String() })
		data, err := canonicalState(record)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		return local.AuthorityChange{Data: data, Result: ab}, nil
	})
	if err == nil && result.Receipt.Rejection == nil {
		// The imported package is available to this engine immediately. Callers
		// used to close and reopen the authority to see it, which re-verified
		// the store and re-read every package to learn one new name.
		if err := e.loadPackages(); err != nil {
			return result, err
		}
	}
	return result, err
}

func readPackageManifest(source string) ([]byte, packageManifest, error) {
	raw, err := readLocal(source, PackageManifestFile, MaxDefinitionBytes)
	if err != nil {
		return nil, packageManifest{}, fmt.Errorf("read %s: %w", PackageManifestFile, err)
	}
	if err := flow.ValidateProtocol("PackageManifest", raw); err != nil {
		return nil, packageManifest{}, err
	}
	var manifest packageManifest
	if err := decode(raw, &manifest); err != nil {
		return nil, packageManifest{}, err
	}
	if len(manifest.Dependencies) > MaxPackageDependencies {
		return nil, packageManifest{}, errors.New("a package declares at most 32 direct dependencies")
	}
	if len(manifest.Components) == 0 || len(manifest.Components) > maxLocalRegistryEntries {
		return nil, packageManifest{}, errors.New("a package must export 1..512 components")
	}
	if len(manifest.Files) > MaxPackagePayloadFiles {
		return nil, packageManifest{}, errors.New("package declares too many payload files")
	}
	return raw, manifest, nil
}

// sealPackagePayload reads exactly the declared files and proves the directory
// holds nothing else. An undeclared file is a rejected import, not an ignored
// extra: it would otherwise travel with the trusted bytes unchecked.
func sealPackagePayload(source string, manifest packageManifest) (map[string][]byte, error) {
	declared := map[string]PackageFile{}
	for _, file := range manifest.Files {
		if !safeRelative(file.Path) || file.Path == PackageManifestFile {
			return nil, local.ErrUnsafePath
		}
		key := strings.ToLower(file.Path)
		if _, duplicate := declared[key]; duplicate {
			return nil, errors.New("package declares colliding file paths")
		}
		declared[key] = file
	}
	present := map[string]bool{}
	var total int64
	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		relative = filepath.ToSlash(relative)
		if entry.IsDir() {
			if !safeRelative(relative) {
				return local.ErrUnsafePath
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("unsafe_package: %s is not a regular file", relative)
		}
		if relative == PackageManifestFile {
			return nil
		}
		if _, ok := declared[strings.ToLower(relative)]; !ok {
			return fmt.Errorf("undeclared_file: %s is not listed in the package manifest", relative)
		}
		present[strings.ToLower(relative)] = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	payload := make(map[string][]byte, len(declared))
	for key, file := range declared {
		if !present[key] {
			return nil, fmt.Errorf("missing_file: %s is declared but absent", file.Path)
		}
		data, err := readLocal(source, file.Path, MaxPackageFileBytes)
		if err != nil {
			return nil, err
		}
		if int64(len(data)) != file.SizeBytes || rawDigest(data) != file.Digest {
			return nil, fmt.Errorf("digest_mismatch: %s does not match its declared bytes", file.Path)
		}
		total += file.SizeBytes
		if total > maxInventoryBytes {
			return nil, errors.New("dependency_limit: package payload exceeds the 64 MiB budget")
		}
		payload[file.Path] = data
	}
	return payload, nil
}

// packageComponents proves each exported reference against the exact declared
// bytes before anything is registered, so a component ref cannot name content
// the package does not actually carry.
func packageComponents(manifest packageManifest, payload map[string][]byte, root string) ([]Definition, error) {
	components := make([]Definition, 0, len(manifest.Components))
	media := map[string]string{}
	for _, file := range manifest.Files {
		media[file.Path] = file.MediaType
	}
	seen := map[string]bool{}
	for _, component := range manifest.Components {
		kind, err := registryKind(component.Kind)
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(component.Ref.ID, "core:") {
			return nil, errors.New("a package cannot replace core contracts")
		}
		key := component.Ref.ID + "@" + component.Ref.Version
		if seen[key] {
			return nil, errors.New("package exports duplicate component identity")
		}
		seen[key] = true
		raw, present := payload[component.Path]
		if !present {
			return nil, fmt.Errorf("missing_component: %s has no declared payload file", component.Path)
		}
		definition := Definition{Ref: component.Ref, Kind: kind, Path: root + "/" + component.Path}
		if kind == "resource" {
			encoding, err := resourceEncoding(media[component.Path])
			if err != nil {
				return nil, err
			}
			definition.ByteEncoding, definition.MediaType = encoding, media[component.Path]
			data := raw
			if encoding == "json" {
				if data, err = flow.Canonical(raw); err != nil {
					return nil, err
				}
			}
			if _, err := flow.CanonicalContextResource(component.Ref, flow.ContextResource{ByteEncoding: encoding, MediaType: definition.MediaType, Bytes: data}); err != nil {
				return nil, err
			}
			components = append(components, definition)
			continue
		}
		format := "json"
		if strings.HasSuffix(component.Path, ".yaml") || strings.HasSuffix(component.Path, ".yml") {
			format = "yaml"
		}
		var data []byte
		if kind == "workflow" {
			data, err = flow.WorkflowJSONBytes(raw, format)
		} else {
			data, err = flow.JSONBytes(raw, format)
		}
		if err != nil {
			return nil, err
		}
		if data, err = flow.Canonical(data); err != nil {
			return nil, err
		}
		if len(data) > MaxDefinitionBytes {
			return nil, errors.New("document_too_large: canonical package component exceeds 2 MiB")
		}
		if rawDigest(data) != component.Ref.Digest {
			return nil, fmt.Errorf("digest_mismatch: %s does not match its exact reference", component.Ref.ID)
		}
		if kind == "tool" {
			descriptor, err := flow.ParseToolDescriptor(data)
			if err != nil {
				return nil, err
			}
			if descriptor.ID != component.Ref.ID || descriptor.Version != component.Ref.Version {
				return nil, errors.New("ref_identity_mismatch: ToolDescriptor differs from exact reference")
			}
		}
		components = append(components, definition)
	}
	return components, nil
}

// materializePackage publishes the checked bytes under an immutable directory
// named by the package identity. An interrupted copy leaves a staging tree that
// never becomes resolvable, and an existing identical revision is inert.
func (e *Engine) materializePackage(root string, manifest packageManifest, payload map[string][]byte, manifestBytes []byte) error {
	final := filepath.Join(e.Root, filepath.FromSlash(root))
	if existing, err := readLocal(e.Root, root+"/"+PackageManifestFile, MaxDefinitionBytes); err == nil {
		if rawDigest(existing) != rawDigest(manifestBytes) {
			return errors.New("package_identity_conflict: this package directory holds different sealed bytes")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Join(e.Root, filepath.FromSlash(PackageRoot)), 0700); err != nil {
		return err
	}
	staging := final + ".pending-" + newID("package")
	defer func() { _ = os.RemoveAll(staging) }()
	if err := os.MkdirAll(staging, 0700); err != nil {
		return err
	}
	files := make([]string, 0, len(payload)+1)
	for path := range payload {
		files = append(files, path)
	}
	sort.Strings(files)
	for _, path := range append(files, PackageManifestFile) {
		data := payload[path]
		if path == PackageManifestFile {
			data = manifestBytes
		}
		target := filepath.Join(staging, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		if err := os.WriteFile(target, data, 0400); err != nil {
			return err
		}
	}
	if err := os.Rename(staging, final); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Join(e.Root, filepath.FromSlash(PackageRoot)))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

// packageEntries returns the registry entries the trusted packages contribute.
// The snapshot is taken when the engine is opened: a package trusted by another
// process becomes visible to the next command, never mid-run.
func (e *Engine) packageEntries() []Definition {
	entries := []Definition{}
	resolvable := resolvablePackages(PackageRecord{Packages: e.packages})
	for _, pkg := range e.packages {
		// A withdrawn package, or one whose dependency was withdrawn, stops
		// supplying definitions. Its bytes stay for the runs that pinned them
		// and for evidence.
		if !resolvable[pkg.Ref] {
			continue
		}
		entries = append(entries, pkg.Components...)
	}
	return entries
}

func (e *Engine) loadPackages() error {
	record, _, err := e.readPackages(context.Background())
	if err != nil {
		return err
	}
	for _, pkg := range record.Packages {
		if pkg.Root != packageDirectory(pkg.Ref.ID, pkg.Ref.Version) {
			return errors.New("trusted package record names a foreign payload directory")
		}
		for _, component := range pkg.Components {
			if !safeRelative(component.Path) || !strings.HasPrefix(component.Path, pkg.Root+"/") {
				return local.ErrUnsafePath
			}
		}
	}
	e.packages = record.Packages
	return nil
}

// PackageView is the reference CLI projection of the trusted package set.
func PackageView(record PackageRecord) map[string]any {
	resolvable := resolvablePackages(record)
	packages := []map[string]any{}
	for _, pkg := range record.Packages {
		components := []flow.Ref{}
		for _, component := range pkg.Components {
			components = append(components, component.Ref)
		}
		status := pkg.Status
		if status == "" {
			status = PackageTrusted
		}
		packages = append(packages, map[string]any{"ref": pkg.Ref, "root": pkg.Root, "origin": pkg.Origin, "trust": pkg.Trust, "status": status, "resolvable": resolvable[pkg.Ref], "dependencies": pkg.Dependencies, "component_refs": components, "file_count": len(pkg.Files), "imported": pkg.Imported})
	}
	return map[string]any{"schema_version": "foundation-packages/1", "packages": packages}
}
