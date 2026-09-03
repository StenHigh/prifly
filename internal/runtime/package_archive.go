package runtime

import (
	"archive/tar"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/stenhigh/prifly/internal/local"
)

// An archive is read as data, never as instructions about where to write. Every
// entry is checked before anything is created, and the manifest and its
// detached signature stay outside the payload inventory: the manifest is hashed
// on its own, the signature covers that hash, and the archive hash is a third
// separate quantity.
const (
	PackageSignatureFile = "prifly.package.sig"

	MaxPackageArchiveBytes   = 64 << 20
	MaxPackageArchiveEntries = 20000
	MaxTrustRoots            = 32
)

// TrustRoot is configured by the authority, never carried by a package. A key
// inside a package cannot appoint itself trusted, so verification looks only at
// roots recorded here.
type TrustRoot struct {
	ID        string      `json:"id"`
	PublicKey string      `json:"public_key"`
	Algorithm string      `json:"algorithm"`
	Note      string      `json:"note"`
	Added     Observation `json:"added"`
}

// PackageSignature is a detached statement about one manifest digest. It proves
// who sealed those bytes; it says nothing about whether the package is any good
// or whether this installation may run it.
type PackageSignature struct {
	SchemaVersion  string `json:"schema_version"`
	ManifestDigest string `json:"manifest_digest"`
	TrustRootID    string `json:"trust_root_id"`
	Algorithm      string `json:"algorithm"`
	Signature      string `json:"signature"`
}

type TrustRootRequest struct {
	CommandID string
	ID        string
	PublicKey string
	Note      string
	Remove    bool
	Reason    string
}

// SetTrustRoot records or withdraws a signing key this installation accepts.
// It is a control decision, so the set of acceptable signers is authority
// state rather than an editable file beside the packages it authorises.
func (e *Engine) SetTrustRoot(ctx context.Context, c TrustRootRequest) (local.AuthorityApplyResult, error) {
	if c.CommandID == "" || c.ID == "" || c.Reason == "" || len(c.Reason) > 4096 {
		return local.AuthorityApplyResult{}, errors.New("explicit command, root identity and reason required")
	}
	var key ed25519.PublicKey
	if !c.Remove {
		raw, err := hex.DecodeString(c.PublicKey)
		if err != nil || len(raw) != ed25519.PublicKeySize {
			return local.AuthorityApplyResult{}, errors.New("a trust root is an explicit hex ed25519 public key")
		}
		key = raw
	}
	control, _, err := e.ensureControl(ctx)
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	if !control.allows(e.owner, "project", e.Config.ID, ControlOperationTrust) {
		return local.AuthorityApplyResult{}, local.Reject("object_access_denied", "the session principal cannot change trust roots")
	}
	command, err := canonical(map[string]any{"operation": "trust_root.set", "command_id": c.CommandID, "root_id": c.ID, "public_key": c.PublicKey, "remove": c.Remove, "reason": c.Reason})
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	return e.applyControl(ctx, c.CommandID, command, func(control *AuthorityControl, obs Observation) (json.RawMessage, error) {
		kept := []TrustRoot{}
		for _, root := range control.TrustRoots {
			if root.ID != c.ID {
				kept = append(kept, root)
			}
		}
		if !c.Remove {
			if len(kept) >= MaxTrustRoots {
				return nil, local.Reject("trust_root_capacity", "withdraw a recorded trust root before adding another")
			}
			kept = append(kept, TrustRoot{ID: c.ID, PublicKey: hex.EncodeToString(key), Algorithm: "ed25519", Note: c.Note, Added: obs})
		} else if len(kept) == len(control.TrustRoots) {
			return nil, local.Reject("not_found", "no trust root with this identity")
		}
		control.TrustRoots = kept
		control.ControlEpoch++
		return canonical(map[string]any{"trust_roots": len(kept)})
	})
}

// verifySignature checks a detached signature against the recorded roots. A key
// travelling with the package is never consulted: that would let a package
// appoint itself trusted.
func (c AuthorityControl) verifySignature(signature PackageSignature, manifestDigest string) error {
	if signature.SchemaVersion != "1" || signature.Algorithm != "ed25519" {
		return local.Reject("unsupported_signature", "only an explicit ed25519 detached signature is verified")
	}
	if signature.ManifestDigest != manifestDigest {
		return local.Reject("signature_subject_conflict", "the signature covers a different manifest digest")
	}
	raw, err := hex.DecodeString(signature.Signature)
	if err != nil || len(raw) != ed25519.SignatureSize {
		return local.Reject("invalid_signature", "the detached signature is not a well-formed ed25519 signature")
	}
	for _, root := range c.TrustRoots {
		if root.ID != signature.TrustRootID {
			continue
		}
		key, err := hex.DecodeString(root.PublicKey)
		if err != nil || len(key) != ed25519.PublicKeySize {
			return local.ErrIntegrity
		}
		if !ed25519.Verify(key, []byte(manifestDigest), raw) {
			return local.Reject("invalid_signature", "the signature does not verify against its named trust root")
		}
		return nil
	}
	return local.Reject("unknown_trust_root", "no recorded trust root has this identity")
}

// extractPackageArchive reads a tar archive into a fresh directory. Every entry
// is validated before creation: a path that escapes, a link, a device or a name
// that collides after normalisation rejects the whole archive rather than
// producing a partly written tree.
func extractPackageArchive(path, into string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() > MaxPackageArchiveBytes {
		return "", local.ErrUnsafePath
	}
	data, err := io.ReadAll(io.LimitReader(file, MaxPackageArchiveBytes+1))
	if err != nil {
		return "", err
	}
	if int64(len(data)) > MaxPackageArchiveBytes {
		return "", local.ErrBlobLimit
	}
	digest := rawDigest(data)
	if err := os.MkdirAll(into, 0700); err != nil {
		return "", err
	}
	reader := tar.NewReader(strings.NewReader(string(data)))
	seen := map[string]bool{}
	entries, total := 0, int64(0)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("unsafe_archive: %w", err)
		}
		entries++
		if entries > MaxPackageArchiveEntries {
			return "", errors.New("unsafe_archive: the archive declares more entries than this installation reads")
		}
		name := filepath.ToSlash(filepath.Clean(header.Name))
		switch header.Typeflag {
		case tar.TypeDir:
			if !safeRelative(name) {
				return "", local.ErrUnsafePath
			}
			if err := os.MkdirAll(filepath.Join(into, filepath.FromSlash(name)), 0700); err != nil {
				return "", err
			}
			continue
		case tar.TypeReg:
		default:
			// Links, devices and anything else are refused: an archive says
			// what bytes it carries, never where the filesystem should point.
			return "", fmt.Errorf("unsafe_archive: %s is not a regular file or directory", header.Name)
		}
		if !safeRelative(name) || name != filepath.ToSlash(header.Name) && filepath.ToSlash(header.Name) != "./"+name {
			return "", local.ErrUnsafePath
		}
		if seen[strings.ToLower(name)] {
			return "", fmt.Errorf("unsafe_archive: %s collides with another entry after normalisation", header.Name)
		}
		seen[strings.ToLower(name)] = true
		total += header.Size
		if header.Size < 0 || total > MaxPackageArchiveBytes {
			return "", local.ErrBlobLimit
		}
		body, err := io.ReadAll(io.LimitReader(reader, header.Size+1))
		if err != nil {
			return "", err
		}
		if int64(len(body)) != header.Size {
			return "", errors.New("unsafe_archive: an entry does not carry its declared size")
		}
		target := filepath.Join(into, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return "", err
		}
		if err := os.WriteFile(target, body, 0600); err != nil {
			return "", err
		}
	}
	return digest, nil
}

// ImportPackageArchive seals a package delivered as one archive. The archive
// digest, the manifest digest and the signature subject stay three separate
// quantities: an archive that hashes correctly still proves nothing about who
// sealed the manifest.
func (e *Engine) ImportPackageArchive(ctx context.Context, request PackageImportRequest, archive, signature string) (local.AuthorityApplyResult, error) {
	if archive == "" {
		return local.AuthorityApplyResult{}, errors.New("an archive import names its archive")
	}
	staging, err := os.MkdirTemp("", "prifly-package-")
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	defer os.RemoveAll(staging)
	source := filepath.Join(staging, "payload")
	archiveDigest, err := extractPackageArchive(archive, source)
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	request.Directory = source
	request.Origin = PackageOrigin{Kind: "local_archive", Location: archive, ArchiveDigest: archiveDigest}
	if signature != "" {
		raw, err := os.ReadFile(signature)
		if err != nil {
			return local.AuthorityApplyResult{}, err
		}
		var detached PackageSignature
		if err := decode(raw, &detached); err != nil {
			return local.AuthorityApplyResult{}, err
		}
		request.Signature = &detached
	}
	return e.ImportPackage(ctx, request)
}
