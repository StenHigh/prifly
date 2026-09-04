package runtime

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
	"github.com/stenhigh/prifly/internal/purity"
)

var ErrArtifactIdentity = errors.New("artifact identity already names different content or metadata")

// ImportArtifact is an explicit owner operation, independent of a Run. Relative
// paths are confined to the project; an absolute path explicitly selects an
// external source. Workers instead use their preallocated output slots.
func (e *Engine) ImportArtifact(path, format string, schema *flow.Ref, mediaType ...string) (Artifact, error) {
	if e.ReadOnly {
		return Artifact{}, local.ErrReadOnly
	}
	media, err := artifactMediaType(format, mediaType)
	if err != nil {
		return Artifact{}, err
	}
	_, reg, err := e.Inventory()
	if err != nil {
		return Artifact{}, err
	}
	root, relative, err := e.artifactOwnerPath(path)
	if err != nil {
		return Artifact{}, err
	}
	data, err := readLocal(root, relative, MaxArtifactBytes)
	if err != nil {
		return Artifact{}, err
	}
	seed, err := canonical(map[string]any{"digest": rawDigest(data), "format": format, "schema": schema, "principal": e.owner, "media_type": media})
	if err != nil {
		return Artifact{}, err
	}
	identity := strings.TrimPrefix(rawDigest(seed), "sha256:")
	producer := map[string]any{
		"kind": "import", "import_id": "import:" + identity, "principal_id": e.owner,
		"source_ref": flow.Ref{ID: "blob:" + strings.TrimPrefix(rawDigest(data), "sha256:"), Version: "1.0.0", Digest: rawDigest(data)},
	}
	return e.putArtifact(data, format, schema, "artifact:import/"+identity, producer, nil, reg, media)
}

// putArtifact validates everything before sealing, then publishes metadata only
// after the blob is durable. A subsequent SQL failure may leave an orphan, never
// a successful reference to absent bytes. Existing accepted content is not
// silently repaired when a referenced blob is missing or corrupt.
func (e *Engine) putArtifact(data []byte, format string, schema *flow.Ref, id string, producer map[string]any, provenance []ArtifactRef, reg flow.Registry, mediaType ...string) (Artifact, error) {
	artifact, err := e.prepareArtifact(data, format, schema, id, producer, provenance, reg, mediaType...)
	if err != nil {
		return Artifact{}, err
	}
	return e.publishPreparedArtifact(artifact, reg)
}

// artifactMetadata reads and verifies one accepted artifact's descriptor. It is
// deliberately not cached: the descriptor records who produced the artifact and
// what it came from, and a reader that trusted an earlier read would not notice
// that the recorded descriptor had been altered underneath it.
func (e *Engine) artifactMetadata(ref ArtifactRef) (Artifact, error) {
	encoded, err := canonical(ref)
	if err != nil {
		return Artifact{}, err
	}
	if err := flow.ValidateProtocol("ArtifactRef", encoded); err != nil {
		return Artifact{}, err
	}
	metadata, err := readLocal(e.Root, artifactMetadataPath(ref.ArtifactID), MaxDefinitionBytes)
	if err != nil {
		return Artifact{}, err
	}
	if err := flow.ValidateProtocol("ArtifactRevision", metadata); err != nil {
		return Artifact{}, err
	}
	var artifact Artifact
	if err := decode(metadata, &artifact); err != nil {
		return Artifact{}, err
	}
	if artifact.Ref() != ref || artifact.Revision != 1 || artifact.SizeBytes < 0 || artifact.SizeBytes > MaxArtifactBytes {
		return Artifact{}, local.ErrIntegrity
	}
	return artifact, nil
}

// prepareArtifact validates and seals bytes without publishing a new accepted
// identity. The returned metadata is detached from caller-owned maps/slices.
// A pending acceptance may retain it, but Artifact cannot resolve its reference
// until publication. An exact already-accepted identity remains an inert retry.
func (e *Engine) prepareArtifact(data []byte, format string, schema *flow.Ref, id string, producer map[string]any, provenance []ArtifactRef, reg flow.Registry, mediaType ...string) (Artifact, error) {
	return e.prepareArtifactWithClassification(data, format, schema, id, producer, provenance, reg, "restricted", mediaType...)
}

// prepareArtifactWithClassification is used when a pinned declaration, rather
// than the generic artifact boundary, already supplies the exact label.
func (e *Engine) prepareArtifactWithClassification(data []byte, format string, schema *flow.Ref, id string, producer map[string]any, provenance []ArtifactRef, reg flow.Registry, classification string, mediaType ...string) (Artifact, error) {
	if e.ReadOnly {
		return Artifact{}, local.ErrReadOnly
	}
	if len(data) > MaxArtifactBytes {
		return Artifact{}, local.ErrBlobLimit
	}
	media, err := artifactMediaType(format, mediaType)
	if err != nil {
		return Artifact{}, err
	}
	if provenance == nil {
		provenance = []ArtifactRef{}
	}
	artifact := Artifact{
		SchemaVersion: "1", ID: id, Revision: 1, Digest: rawDigest(data), Producer: producer,
		Format: format, SchemaRef: schema, MediaType: media, SizeBytes: int64(len(data)),
		Classification: classification, ContentCheckEvidence: []any{}, Provenance: provenance,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	artifact, _, err = e.validatePreparedArtifact(data, artifact, reg)
	if err != nil {
		return Artifact{}, err
	}
	// Check accepted identity before Put: an absent accepted blob is evidence
	// loss, not permission to reconstruct it from the new caller's bytes.
	if old, err := e.existingArtifact(artifact); err == nil {
		return old, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return Artifact{}, err
	}
	sealed, err := e.Blobs.Put(bytes.NewReader(data), MaxArtifactBytes)
	if err != nil {
		return Artifact{}, err
	}
	if sealed.Digest != artifact.Digest || sealed.Size != artifact.SizeBytes {
		return Artifact{}, local.ErrIntegrity
	}
	return artifact, nil
}

// publishPreparedArtifact accepts only the exact sealed bytes and validated
// metadata supplied by its caller. The acceptance transition owns any required
// check decisions/EvidenceRefs; this helper does not infer successful checks.
// It never seals or repairs bytes, including on an exact publication retry.
func (e *Engine) publishPreparedArtifact(artifact Artifact, reg flow.Registry) (Artifact, error) {
	if e.ReadOnly {
		return Artifact{}, local.ErrReadOnly
	}
	if artifact.Revision != 1 || artifact.SizeBytes < 0 || artifact.SizeBytes > MaxArtifactBytes {
		return Artifact{}, local.ErrIntegrity
	}
	data, err := e.Blobs.Read(local.BlobRef{Digest: artifact.Digest, Size: artifact.SizeBytes})
	if err != nil {
		return Artifact{}, fmt.Errorf("prepared artifact content unavailable: %w: %v", local.ErrIntegrity, err)
	}
	artifact, metadata, err := e.validatePreparedArtifact(data, artifact, reg)
	if err != nil {
		return Artifact{}, err
	}
	if old, err := e.existingArtifact(artifact); err == nil {
		return old, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return Artifact{}, err
	}
	if err := e.publishArtifactMetadata(artifact.ID, metadata); err != nil {
		if errors.Is(err, os.ErrExist) {
			return e.existingArtifact(artifact)
		}
		return Artifact{}, err
	}
	return artifact, nil
}

// Both sides of a deferred publication validate the same content/descriptor
// schema and accepted provenance. In particular, no metadata field is repaired
// or defaulted when a prepared record is read back from pending acceptance.
func (e *Engine) validatePreparedArtifact(data []byte, artifact Artifact, reg flow.Registry) (Artifact, []byte, error) {
	if artifact.Revision != 1 || artifact.SizeBytes != int64(len(data)) || artifact.Digest != rawDigest(data) {
		return Artifact{}, nil, local.ErrIntegrity
	}
	media, err := artifactMediaType(artifact.Format, []string{artifact.MediaType})
	if err != nil {
		return Artifact{}, nil, err
	}
	if media != artifact.MediaType {
		return Artifact{}, nil, errors.New("prepared artifact media type must be explicit and canonical")
	}
	if artifact.Format == "json" {
		if artifact.SchemaRef == nil {
			return Artifact{}, nil, errors.New("JSON artifacts require an exact schema reference")
		}
		if err := flow.ValidateSchema(reg, *artifact.SchemaRef, data); err != nil {
			return Artifact{}, nil, err
		}
	}
	metadata, err := canonical(artifact)
	if err != nil {
		return Artifact{}, nil, err
	}
	if err := flow.ValidateProtocol("ArtifactRevision", metadata); err != nil {
		return Artifact{}, nil, err
	}
	var detached Artifact
	if err := decode(metadata, &detached); err != nil {
		return Artifact{}, nil, err
	}
	if detached.Format == "blob" && detached.SchemaRef != nil {
		// This checks the descriptor, not the contents of a PDF, image or program.
		descriptor, err := canonical(map[string]any{"media_type": detached.MediaType, "size_bytes": detached.SizeBytes, "digest": detached.Digest})
		if err != nil {
			return Artifact{}, nil, err
		}
		if err := flow.ValidateSchema(reg, *detached.SchemaRef, descriptor); err != nil {
			return Artifact{}, nil, err
		}
	}
	for _, ref := range detached.Provenance {
		if _, _, err := e.Artifact(ref); err != nil {
			return Artifact{}, nil, fmt.Errorf("artifact provenance is unavailable: %w", err)
		}
	}
	return detached, metadata, nil
}

// Artifact is a read of accepted immutable content. It does not consult the
// mutable installation registry. Admission and reuse validate against their own
// pinned plan; a changed installed schema must not rewrite historical meaning.
func (e *Engine) Artifact(ref ArtifactRef) (Artifact, []byte, error) {
	artifact, err := e.artifactMetadata(ref)
	if err != nil {
		return Artifact{}, nil, err
	}
	data, err := e.Blobs.Read(local.BlobRef{Digest: artifact.Digest, Size: artifact.SizeBytes})
	if err != nil {
		// This identity already names accepted bytes. Their disappearance is
		// evidence loss, not an ordinary unknown input/reference. Preserve that
		// classification even when the underlying filesystem reports ENOENT.
		return Artifact{}, nil, fmt.Errorf("accepted artifact content unavailable: %w: %v", local.ErrIntegrity, err)
	}
	return artifact, data, nil
}

func (e *Engine) ExportArtifact(ref ArtifactRef, destination string) error {
	if e.ReadOnly {
		return local.ErrReadOnly
	}
	artifact, _, err := e.Artifact(ref)
	if err != nil {
		return err
	}
	root, relative, err := e.artifactOwnerPath(destination)
	if err != nil {
		return err
	}
	return e.Blobs.Export(local.BlobRef{Digest: artifact.Digest, Size: artifact.SizeBytes}, root, relative)
}

func artifactMetadataPath(id string) string {
	digest := sha256.Sum256([]byte(id))
	return filepath.Join(".prifly", "artifact-refs", fmt.Sprintf("%x.json", digest))
}

// A media type is an explicit label, not inferred evidence about file contents.
func artifactMediaType(format string, selection []string) (string, error) {
	if format != "json" && format != "blob" {
		return "", errors.New("artifact format must be json or blob")
	}
	if len(selection) > 1 {
		return "", errors.New("choose exactly one artifact media type")
	}
	media := "application/octet-stream"
	if format == "json" {
		media = "application/json"
	}
	if len(selection) == 1 && selection[0] != "" {
		media = selection[0]
	}
	if len(media) > 128 || strings.ContainsAny(media, "\r\n\x00") {
		return "", errors.New("invalid artifact media type")
	}
	base, params, err := mime.ParseMediaType(media)
	if err != nil || !strings.Contains(base, "/") || strings.Contains(base, "*") {
		return "", errors.New("invalid artifact media type")
	}
	media = mime.FormatMediaType(base, params)
	if media == "" || len(media) > 128 {
		return "", errors.New("invalid artifact media type")
	}
	if format == "json" && media != "application/json" {
		return "", errors.New("JSON artifact media type must be application/json")
	}
	return media, nil
}

func (e *Engine) existingArtifact(candidate Artifact) (Artifact, error) {
	metadata, err := readLocal(e.Root, artifactMetadataPath(candidate.ID), MaxDefinitionBytes)
	if err != nil {
		return Artifact{}, err
	}
	if err := flow.ValidateProtocol("ArtifactRevision", metadata); err != nil {
		return Artifact{}, err
	}
	var previous Artifact
	if err := decode(metadata, &previous); err != nil {
		return Artifact{}, err
	}
	// Time belongs to the first durable publication, not to a retrying caller.
	candidate.CreatedAt = previous.CreatedAt
	want, err := canonical(candidate)
	if err != nil {
		return Artifact{}, err
	}
	have, err := canonical(previous)
	if err != nil {
		return Artifact{}, err
	}
	if !bytes.Equal(want, have) {
		return Artifact{}, ErrArtifactIdentity
	}
	if err := e.Blobs.Inspect(local.BlobRef{Digest: previous.Digest, Size: previous.SizeBytes}); err != nil {
		// An accepted metadata file existed, so a missing blob is an integrity
		// incident. Do not treat it as permission to create the identity anew.
		return Artifact{}, fmt.Errorf("accepted artifact content unavailable: %w: %v", local.ErrIntegrity, err)
	}
	// A concurrent publisher may have linked the complete file but not synced
	// its directory yet. Our duplicate acknowledgment must establish durability.
	r, err := e.artifactMetadataRoot()
	if err != nil {
		return Artifact{}, err
	}
	defer r.Close()
	dir, err := r.Open(".")
	if err != nil {
		return Artifact{}, err
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return Artifact{}, err
	}
	return previous, nil
}

func (e *Engine) publishArtifactMetadata(id string, metadata []byte) error {
	root, err := e.artifactMetadataRoot()
	if err != nil {
		return err
	}
	defer root.Close()
	staging := ".pending-" + newID("metadata")
	f, err := root.OpenFile(staging, os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close(); _ = root.Remove(staging) }()
	if _, err := f.Write(metadata); err != nil {
		return err
	}
	if err := f.Chmod(0400); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := root.Link(staging, filepath.Base(artifactMetadataPath(id))); err != nil {
		return err
	}
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func (e *Engine) artifactMetadataRoot() (*os.Root, error) {
	purity.Guard("os.open")
	project, err := os.OpenRoot(e.Root)
	if err != nil {
		return nil, err
	}
	defer project.Close()
	for _, path := range []string{".prifly", ".prifly/artifact-refs"} {
		st, err := project.Lstat(path)
		if err != nil {
			return nil, err
		}
		if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return nil, local.ErrUnsafePath
		}
	}
	return project.OpenRoot(".prifly/artifact-refs")
}

func (e *Engine) artifactOwnerPath(path string) (root, relative string, err error) {
	if !filepath.IsAbs(path) {
		if !safeRelative(path) || path == "." {
			return "", "", local.ErrUnsafePath
		}
		return e.Root, path, nil
	}
	if filepath.Clean(path) != path || strings.ContainsAny(path, "\\\x00") {
		return "", "", local.ErrUnsafePath
	}
	if rel, err := filepath.Rel(e.Root, path); err == nil && safeRelative(rel) && rel != "." {
		return e.Root, rel, nil
	}
	// The owner selected this external parent explicitly; reject a symlink root
	// and leaf, and keep all relative operations confined to that opened root.
	parent := filepath.Dir(path)
	st, err := os.Lstat(parent)
	if err != nil {
		return "", "", err
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return "", "", local.ErrUnsafePath
	}
	return parent, filepath.Base(path), nil
}
