package local

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func testBlobs(t *testing.T) *BlobStore {
	t.Helper()
	b, err := OpenBlobStore(filepath.Join(t.TempDir(), "blobs"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	return b
}

func TestBlobSealImportExport(t *testing.T) {
	b := testBlobs(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "input.txt"), []byte("original bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	ref, err := b.Import(root, "input.txt", 32)
	if err != nil {
		t.Fatal(err)
	}
	if ref.Digest != digestBytes([]byte("original bytes")) || ref.Size != 14 {
		t.Fatalf("bad ref: %+v", ref)
	}
	if err := os.WriteFile(filepath.Join(root, "input.txt"), []byte("changed source"), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := b.Read(ref)
	if err != nil || string(data) != "original bytes" {
		t.Fatalf("sealed input changed: %q, %v", data, err)
	}
	again, err := b.Put(strings.NewReader("original bytes"), 32)
	if err != nil || again != ref {
		t.Fatalf("dedup: %+v %v", again, err)
	}
	if err := b.Inspect(ref); err != nil {
		t.Fatal(err)
	}
	if err := b.Export(ref, root, "output.txt"); err != nil {
		t.Fatal(err)
	}
	if err := b.Export(ref, root, "output.txt"); !errors.Is(err, os.ErrExist) {
		t.Fatalf("export overwrote destination: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "output.txt"), []byte("mutable export"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := b.Inspect(ref); err != nil {
		t.Fatalf("export shared authority inode: %v", err)
	}
	entries, err := b.root.Open(".")
	if err != nil {
		t.Fatal(err)
	}
	defer entries.Close()
	names, err := entries.Readdirnames(-1)
	if err != nil || len(names) != 1 {
		t.Fatalf("staging files not cleaned: %v, %v", names, err)
	}
}

func TestBlobRejectsPathsAndNonregularFiles(t *testing.T) {
	b := testBlobs(t)
	root, outside := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "directory")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "real"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(root, "fifo"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"../secret", filepath.Join(outside, "secret"), "link", "directory/secret", "real/../link", "real", "fifo", "", ".", "a\\b"} {
		t.Run(name, func(t *testing.T) {
			if _, err := b.Import(root, name, 32); err == nil {
				t.Fatalf("accepted unsafe path %q", name)
			}
		})
	}
	ref, err := b.Put(strings.NewReader("ok"), 32)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"../escape", "directory/escape", "link"} {
		if err := b.Export(ref, root, name); err == nil {
			t.Fatalf("export accepted unsafe path %q", name)
		}
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenBlobStore(alias); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("symlink root accepted: %v", err)
	}
}

func TestBlobFailuresAndCorruption(t *testing.T) {
	b := testBlobs(t)
	if _, err := b.Put(strings.NewReader("too many"), 3); !errors.Is(err, ErrBlobLimit) {
		t.Fatalf("byte limit: %v", err)
	}
	if _, err := b.Put(brokenReader{}, 32); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("read failure: %v", err)
	}
	ref, err := b.Put(strings.NewReader("good"), 32)
	if err != nil {
		t.Fatal(err)
	}
	name, _ := blobName(ref)
	if err := b.root.Chmod(name, 0600); err != nil {
		t.Fatal(err)
	}
	if err := b.root.WriteFile(name, []byte("evil"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := b.Inspect(ref); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("accepted corrupt blob: %v", err)
	}
	if _, err := b.Read(ref); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("returned corrupt bytes: %v", err)
	}
	if _, err := b.Put(strings.NewReader("good"), 32); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("silently repaired existing digest: %v", err)
	}
	for _, bad := range []BlobRef{{Digest: "sha256:../escape", Size: 4}, {Digest: ref.Digest, Size: 5}, {Digest: ref.Digest, Size: -1}, {Digest: ref.Digest, Size: MaxBlobBytes + 1}} {
		if _, err := b.Read(bad); err == nil {
			t.Fatalf("accepted malformed ref: %+v", bad)
		}
	}
	if err := b.root.Remove(name); err != nil {
		t.Fatal(err)
	}
	if err := b.Inspect(ref); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing blob: %v", err)
	}
}

func TestBlobReadOnlyDoesNotCreateOrWrite(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "missing")
	if _, err := OpenBlobStoreReadOnly(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing readonly root: %v", err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("read created directory")
	}
	w, err := OpenBlobStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := w.Put(strings.NewReader("bytes"), 32)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	r, err := OpenBlobStoreReadOnly(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if _, err := r.Read(ref); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Put(strings.NewReader("new"), 32); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("readonly write: %v", err)
	}
	if err := r.Export(ref, parent, "export"); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("readonly export: %v", err)
	}
}

type brokenReader struct{}

func (brokenReader) Read(p []byte) (int, error) { copy(p, "partial"); return 7, io.ErrUnexpectedEOF }
