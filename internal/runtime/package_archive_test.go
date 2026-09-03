package runtime

import (
	"archive/tar"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// archiveOf packs a prepared package directory. Tests then bend one entry at a
// time, so each refusal is attributable to exactly one property.
func archiveOf(t *testing.T, source string, bend func(*tar.Header) bool) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "package.tar")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	writer := tar.NewWriter(file)
	err = filepath.Walk(source, func(name string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		relative, err := filepath.Rel(source, name)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		header := &tar.Header{Name: filepath.ToSlash(relative), Mode: 0600, Size: int64(len(data)), Typeflag: tar.TypeReg}
		if bend != nil && bend(header) {
			if err := writer.WriteHeader(header); err != nil {
				return err
			}
			if header.Typeflag == tar.TypeReg {
				_, err = writer.Write(data)
			}
			return err
		}
		if err := writer.WriteHeader(header); err != nil {
			return err
		}
		_, err = writer.Write(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestArchiveImportSealsTheSameBytesAsADirectory(t *testing.T) {
	e := contextRegistryRuntime(t)
	ctx := context.Background()
	source, _, _ := skillPackage(t, "---\nname: aif-plan\n---\n\n# Plan\n")
	archive := archiveOf(t, source, nil)
	result, err := e.ImportPackageArchive(ctx, PackageImportRequest{CommandID: "command:archive", Reason: "sealed archive"}, archive, "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Receipt.Rejection != nil {
		t.Fatalf("a well-formed archive was refused: %+v", result.Receipt.Rejection)
	}
	record, err := e.Packages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pkg := record.Packages[0]
	if pkg.Origin.Kind != "local_archive" || pkg.Origin.ArchiveDigest == "" {
		t.Fatalf("the archive origin was not recorded: %+v", pkg.Origin)
	}
	// The archive hash is its own quantity, never the manifest's.
	if pkg.Origin.ArchiveDigest == pkg.ManifestDigest {
		t.Fatal("the archive digest was confused with the manifest digest")
	}
	if pkg.Trust.SignedBy != "" {
		t.Fatalf("an unsigned package claimed a signer: %+v", pkg.Trust)
	}
}

func TestArchiveRefusesEscapingLinkingAndCollidingEntries(t *testing.T) {
	e := contextRegistryRuntime(t)
	ctx := context.Background()
	source, _, _ := skillPackage(t, "# Plan\n")
	for _, bend := range []struct {
		name string
		fn   func(*tar.Header) bool
	}{
		{"traversal", func(h *tar.Header) bool {
			if h.Name == PackageManifestFile {
				h.Name = "../escaped.json"
				return true
			}
			return false
		}},
		{"absolute", func(h *tar.Header) bool {
			if h.Name == PackageManifestFile {
				h.Name = "/etc/escaped.json"
				return true
			}
			return false
		}},
		{"symlink", func(h *tar.Header) bool {
			if h.Name == PackageManifestFile {
				h.Typeflag, h.Linkname, h.Size = tar.TypeSymlink, "/etc/passwd", 0
				return true
			}
			return false
		}},
	} {
		archive := archiveOf(t, source, bend.fn)
		if _, err := e.ImportPackageArchive(ctx, PackageImportRequest{CommandID: "command:" + bend.name, Reason: bend.name}, archive, ""); err == nil {
			t.Fatalf("an archive with a %s entry was accepted", bend.name)
		}
	}
	if record, err := e.Packages(ctx); err != nil || len(record.Packages) != 0 {
		t.Fatalf("a refused archive registered a package: %v", err)
	}
	// Nothing escaped the extraction directory.
	if _, err := os.Stat("/etc/escaped.json"); !os.IsNotExist(err) {
		t.Fatalf("an archive wrote outside its extraction directory: %v", err)
	}
}

// PKG-007: a signature proves who sealed the bytes. A key inside the package
// never appoints itself, so verification consults only recorded trust roots.
func TestSignatureIsVerifiedAgainstRecordedTrustRootsOnly(t *testing.T) {
	e := contextRegistryRuntime(t)
	ctx := context.Background()
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.SetTrustRoot(ctx, TrustRootRequest{CommandID: "command:root", ID: "root:release", PublicKey: hex.EncodeToString(public), Note: "release signer", Reason: "accept the release signer"}); err != nil {
		t.Fatal(err)
	}
	source, _, _ := skillPackage(t, "# Plan\n")
	archive := archiveOf(t, source, nil)

	// With roots recorded, an unsigned package is refused rather than trusted.
	unsigned, err := e.ImportPackageArchive(ctx, PackageImportRequest{CommandID: "command:unsigned", Reason: "no signature"}, archive, "")
	if err == nil {
		t.Fatalf("an unsigned package was trusted where roots are recorded: %+v", unsigned)
	}
	rejectionCode(t, err, "signature_required")

	manifest, err := os.ReadFile(filepath.Join(source, PackageManifestFile))
	if err != nil {
		t.Fatal(err)
	}
	digest := rawDigest(manifest)
	write := func(name string, value PackageSignature) string {
		path := filepath.Join(t.TempDir(), name)
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}

	// A signature by an unrecorded key is refused even though it verifies.
	_, foreignKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	foreign := write("foreign.sig", PackageSignature{SchemaVersion: "1", ManifestDigest: digest, TrustRootID: "root:unknown", Algorithm: "ed25519", Signature: hex.EncodeToString(ed25519.Sign(foreignKey, []byte(digest)))})
	_, err = e.ImportPackageArchive(ctx, PackageImportRequest{CommandID: "command:foreign", Reason: "foreign signer"}, archive, foreign)
	rejectionCode(t, err, "unknown_trust_root")

	// A signature over other bytes is refused.
	wrong := write("wrong.sig", PackageSignature{SchemaVersion: "1", ManifestDigest: rawDigest([]byte("other")), TrustRootID: "root:release", Algorithm: "ed25519", Signature: hex.EncodeToString(ed25519.Sign(private, []byte(rawDigest([]byte("other")))))})
	_, err = e.ImportPackageArchive(ctx, PackageImportRequest{CommandID: "command:wrong", Reason: "wrong subject"}, archive, wrong)
	rejectionCode(t, err, "signature_subject_conflict")

	valid := write("valid.sig", PackageSignature{SchemaVersion: "1", ManifestDigest: digest, TrustRootID: "root:release", Algorithm: "ed25519", Signature: hex.EncodeToString(ed25519.Sign(private, []byte(digest)))})
	result, err := e.ImportPackageArchive(ctx, PackageImportRequest{CommandID: "command:signed", Reason: "signed release"}, archive, valid)
	if err != nil {
		t.Fatal(err)
	}
	if result.Receipt.Rejection != nil {
		t.Fatalf("a correctly signed package was refused: %+v", result.Receipt.Rejection)
	}
	record, err := e.Packages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if record.Packages[0].Trust.SignedBy != "root:release" {
		t.Fatalf("the verified signer was not recorded: %+v", record.Packages[0].Trust)
	}
	// Withdrawing the root does not rewrite what was already admitted.
	if _, err := e.SetTrustRoot(ctx, TrustRootRequest{CommandID: "command:withdraw", ID: "root:release", Remove: true, Reason: "signer rotated"}); err != nil {
		t.Fatal(err)
	}
	after, err := e.Packages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.Packages[0].Trust.SignedBy != "root:release" {
		t.Fatal("withdrawing a trust root rewrote a recorded decision")
	}
}
