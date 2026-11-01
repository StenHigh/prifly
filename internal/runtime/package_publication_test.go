package runtime

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPackageDirectoryPublicationCollision(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"identical", "manifest-conflict", "payload-drift", "undeclared-file", "missing-staging"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			source, _, _ := skillPackage(t, "# Plan\n")
			raw, manifest, err := readPackageManifest(source)
			if err != nil {
				t.Fatal(err)
			}
			payload, err := sealPackagePayload(source, manifest)
			if err != nil {
				t.Fatal(err)
			}
			e := &Engine{Root: t.TempDir()}
			root := packageDirectory(manifest.ID, manifest.Version)
			if err := e.materializePackage(root, manifest, payload, raw); err != nil {
				t.Fatal(err)
			}
			final := filepath.Join(e.Root, filepath.FromSlash(root))
			staging := final + ".pending-loser"
			if err := os.CopyFS(staging, os.DirFS(source)); err != nil {
				t.Fatal(err)
			}
			// Both importers saw no final directory; the winner published before
			// the loser's rename. Start at that boundary to fix the ordering.
			wantCode := ""
			write := func(path string, data []byte) {
				t.Helper()
				if err := os.Chmod(path, 0600); err != nil && !errors.Is(err, os.ErrNotExist) {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			switch name {
			case "manifest-conflict":
				write(filepath.Join(final, PackageManifestFile), append(append([]byte(nil), raw...), '\n'))
				wantCode = "package_identity_conflict"
			case "payload-drift":
				write(filepath.Join(final, "skills/aif-plan/SKILL.md"), []byte("changed\n"))
				wantCode = "digest_mismatch"
			case "undeclared-file":
				write(filepath.Join(final, "unexpected.txt"), []byte("unexpected\n"))
				wantCode = "undeclared_file"
			case "missing-staging":
				if err := os.RemoveAll(staging); err != nil {
					t.Fatal(err)
				}
			}
			before := map[string][]byte{}
			if err := filepath.WalkDir(final, func(path string, entry os.DirEntry, err error) error {
				if err != nil || entry.IsDir() {
					return err
				}
				before[path], err = os.ReadFile(path)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			err = e.publishPackageDirectory(root, staging, manifest, raw)
			switch {
			case name == "missing-staging":
				if !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("missing source was hidden by identical destination: %v", err)
				}
			case wantCode != "":
				var refusal *Fault
				if !errors.As(err, &refusal) || refusal.Code != wantCode {
					t.Fatalf("want %s, got %v", wantCode, err)
				}
			case err != nil:
				t.Fatalf("identical concurrent publication refused: %v", err)
			}
			for path, want := range before {
				got, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(got, want) {
					t.Fatalf("winner changed at %s: %q %v", path, got, err)
				}
			}
		})
	}
}
