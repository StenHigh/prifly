package local

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/stenhigh/prifly/internal/purity"
)

const MaxBlobBytes int64 = 100 << 20

var (
	ErrBlobLimit  = errors.New("artifact exceeds byte limit")
	ErrIntegrity  = errors.New("stored content failed integrity verification")
	ErrUnsafePath = errors.New("unsafe filesystem path")
)

// BlobRef addresses bytes, not a mutable producer file. Media type and producer
// provenance belong to the runtime's artifact record, not the content store.
type BlobRef struct {
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}

// BlobStore does not delete blobs: F1 retains orphaned uploads rather than
// racing a reference commit. The state directory must be on a local filesystem.
type BlobStore struct {
	root     *os.Root
	readOnly bool
}

// A digest is written as its algorithm and 64 hex characters, so its text has
// one length: "sha256:" plus the hash.
const digestPrefix = "sha256:"
const digestTextLength = len(digestPrefix) + sha256.Size*2

func OpenBlobStore(dir string) (*BlobStore, error) {
	r, err := privateRoot(dir)
	if err != nil {
		return nil, err
	}
	return &BlobStore{root: r}, nil
}

func OpenBlobStoreReadOnly(dir string) (*BlobStore, error) {
	r, err := existingRoot(dir)
	if err != nil {
		return nil, err
	}
	return &BlobStore{root: r, readOnly: true}, nil
}

func (b *BlobStore) Close() error { return b.root.Close() }

// Put writes and syncs bytes before publishing an immutable, no-replace name.
// Its successful return is the earliest point at which SQL may reference them.
// A failed SQL commit must not remove this blob: another run may already use it.
func (b *BlobStore) Put(src io.Reader, limit int64) (BlobRef, error) {
	purity.Guard("blob.put")
	if b.readOnly {
		return BlobRef{}, ErrReadOnly
	}
	limit, err := blobLimit(limit)
	if err != nil {
		return BlobRef{}, err
	}
	name, f, err := createStaging(b.root, ".")
	if err != nil {
		return BlobRef{}, err
	}
	defer func() { _ = f.Close(); _ = b.root.Remove(name) }()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(src, limit+1))
	if err != nil {
		return BlobRef{}, fmt.Errorf("write artifact: %w", err)
	}
	if n > limit {
		return BlobRef{}, ErrBlobLimit
	}
	ref := BlobRef{Digest: "sha256:" + hex.EncodeToString(h.Sum(nil)), Size: n}
	if err := f.Chmod(0400); err != nil {
		return BlobRef{}, err
	}
	if err := f.Sync(); err != nil {
		return BlobRef{}, fmt.Errorf("sync artifact: %w", err)
	}
	if err := f.Close(); err != nil {
		return BlobRef{}, err
	}
	sealed, _ := blobName(ref)
	// Link is atomic and, unlike Rename, never replaces an existing digest.
	if err := b.root.Link(name, sealed); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return BlobRef{}, fmt.Errorf("seal artifact: %w", err)
		}
		if err := b.Inspect(ref); err != nil {
			return BlobRef{}, err
		}
	}
	if err := syncRoot(b.root, "."); err != nil {
		return BlobRef{}, fmt.Errorf("sync artifact directory: %w", err)
	}
	return ref, nil
}

// Import rejects links, devices, traversal and over-limit files. It copies the
// opened bytes into authority storage; subsequent edits of src cannot alter it.
func (b *BlobStore) Import(rootDir, relative string, limit int64) (BlobRef, error) {
	if b.readOnly {
		return BlobRef{}, ErrReadOnly
	}
	r, err := existingRoot(rootDir)
	if err != nil {
		return BlobRef{}, err
	}
	defer r.Close()
	f, err := openRegular(r, relative)
	if err != nil {
		return BlobRef{}, err
	}
	defer f.Close()
	return b.Put(f, limit)
}

// Read verifies the exact bytes returned, including size. Callers needing only
// an integrity check should use Inspect, which does not allocate the whole blob.
func (b *BlobStore) Read(ref BlobRef) ([]byte, error) {
	purity.Guard("blob.read")
	f, err := b.open(ref)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, ref.Size+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) != ref.Size || digestBytes(data) != ref.Digest {
		return nil, ErrIntegrity
	}
	return data, nil
}

func (b *BlobStore) Inspect(ref BlobRef) error {
	purity.Guard("blob.inspect")
	f, err := b.open(ref)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, ref.Size+1))
	if err != nil {
		return err
	}
	if n != ref.Size || "sha256:"+hex.EncodeToString(h.Sum(nil)) != ref.Digest {
		return ErrIntegrity
	}
	return nil
}

// Export publishes a verified copy without replacing any existing destination.
// The caller chooses an existing destination root and parent directories.
// Exporting to a worker workspace never gives it a link to authority bytes.
func (b *BlobStore) Export(ref BlobRef, rootDir, relative string) error {
	purity.Guard("blob.export")
	if b.readOnly {
		return ErrReadOnly
	}
	src, err := b.open(ref)
	if err != nil {
		return err
	}
	defer src.Close()
	r, err := existingRoot(rootDir)
	if err != nil {
		return err
	}
	defer r.Close()
	if err := safeRelative(r, relative, true); err != nil {
		return err
	}
	if _, err := r.Lstat(relative); err == nil {
		return fmt.Errorf("export destination exists: %w", os.ErrExist)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	parent := filepath.Dir(relative)
	tmp, f, err := createStaging(r, parent)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close(); _ = r.Remove(tmp) }()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(src, ref.Size+1))
	if err != nil {
		return err
	}
	if n != ref.Size || "sha256:"+hex.EncodeToString(h.Sum(nil)) != ref.Digest {
		return ErrIntegrity
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := safeRelative(r, relative, true); err != nil {
		return err
	}
	if err := r.Link(tmp, relative); err != nil {
		return fmt.Errorf("publish export: %w", err)
	}
	return syncRoot(r, parent)
}

func (b *BlobStore) open(ref BlobRef) (*os.File, error) {
	name, err := blobName(ref)
	if err != nil {
		return nil, err
	}
	f, err := openRegular(b.root, name)
	if err != nil {
		return nil, fmt.Errorf("artifact unavailable: %w", err)
	}
	st, err := f.Stat()
	if err != nil || st.Size() != ref.Size {
		_ = f.Close()
		if err != nil {
			return nil, err
		}
		return nil, ErrIntegrity
	}
	return f, nil
}

func blobName(ref BlobRef) (string, error) {
	if ref.Size < 0 || ref.Size > MaxBlobBytes || len(ref.Digest) != digestTextLength || !strings.HasPrefix(ref.Digest, digestPrefix) {
		return "", fmt.Errorf("invalid blob reference: %w", ErrIntegrity)
	}
	name := ref.Digest[7:]
	if _, err := hex.DecodeString(name); err != nil || strings.ToLower(name) != name {
		return "", fmt.Errorf("invalid blob digest: %w", ErrIntegrity)
	}
	return name, nil
}

func blobLimit(limit int64) (int64, error) {
	if limit == 0 {
		limit = MaxBlobBytes
	}
	if limit < 0 || limit > MaxBlobBytes {
		return 0, ErrBlobLimit
	}
	return limit, nil
}

func digestBytes(data []byte) string {
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func createStaging(r *os.Root, parent string) (string, *os.File, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", nil, err
	}
	name := filepath.Join(parent, ".upload-"+hex.EncodeToString(id[:]))
	f, err := r.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	return name, f, err
}

// os.Root is the race-safe confinement boundary; the additional checks reject
// even in-root symlinks instead of treating them as supported artifact paths.
func safeRelative(r *os.Root, name string, missingLeaf bool) error {
	if name == "." || !filepath.IsLocal(name) || filepath.Clean(name) != name || strings.ContainsAny(name, "\\\x00") {
		return ErrUnsafePath
	}
	parts := strings.Split(name, string(os.PathSeparator))
	for i := range parts {
		component := filepath.Join(parts[:i+1]...)
		st, err := r.Lstat(component)
		if missingLeaf && i == len(parts)-1 && errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if st.Mode()&os.ModeSymlink != 0 || (i < len(parts)-1 && !st.IsDir()) {
			return ErrUnsafePath
		}
	}
	return nil
}

func openRegular(r *os.Root, relative string) (*os.File, error) {
	if err := safeRelative(r, relative, false); err != nil {
		return nil, err
	}
	// NONBLOCK makes malicious FIFOs fail regular-file validation without hanging.
	f, err := r.OpenFile(relative, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		_ = f.Close()
		if err != nil {
			return nil, err
		}
		return nil, ErrUnsafePath
	}
	return f, nil
}

func privateRoot(dir string) (*os.Root, error) {
	// Only create the selected directory; never silently chmod an existing one.
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	r, err := existingRoot(dir)
	if err != nil {
		return nil, err
	}
	st, err := r.Stat(".")
	if err != nil || st.Mode().Perm()&0077 != 0 {
		_ = r.Close()
		if err != nil {
			return nil, err
		}
		return nil, errors.New("state directory must have owner-only permissions (0700)")
	}
	if err := syncRoot(r, "."); err != nil {
		_ = r.Close()
		return nil, err
	}
	// The new directory's name is durable as well as its contents.
	parent, err := os.Open(filepath.Dir(filepath.Clean(dir)))
	if err != nil {
		_ = r.Close()
		return nil, err
	}
	err = parent.Sync()
	_ = parent.Close()
	if err != nil {
		_ = r.Close()
		return nil, err
	}
	return r, nil
}

func existingRoot(dir string) (*os.Root, error) {
	st, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return nil, ErrUnsafePath
	}
	r, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	opened, err := r.Stat(".")
	if err != nil || !os.SameFile(st, opened) {
		_ = r.Close()
		if err != nil {
			return nil, err
		}
		return nil, ErrUnsafePath
	}
	return r, nil
}

func syncRoot(r *os.Root, relative string) error {
	f, err := r.Open(relative)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
