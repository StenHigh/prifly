package runtime

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

func TestArtifactPreparationPrivateAcrossReopen(t *testing.T) {
	e := artifactEngine(t)
	schema, reg := artifactSchema(t, e, `{"type":"object","required":["value"],"properties":{"value":{"type":"integer"}},"additionalProperties":false}`)
	source, err := e.putArtifact([]byte("accepted source"), "blob", nil, "artifact:source", artifactProducer(e), nil, reg)
	if err != nil {
		t.Fatal(err)
	}
	_, before, err := e.Store.ReadAll(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("{ \"value\": 7 }\r\n")
	wantData, wantSchema := bytes.Clone(data), schema
	producer, provenance := artifactProducer(e), []ArtifactRef{source.Ref()}
	prepared, err := e.prepareArtifact(data, "json", &schema, "artifact:prepared", producer, provenance, reg)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.Artifact(prepared.Ref()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("prepared identity became readable before acceptance: %v", err)
	}
	if sealed, err := e.Blobs.Read(local.BlobRef{Digest: prepared.Digest, Size: prepared.SizeBytes}); err != nil || !bytes.Equal(sealed, wantData) {
		t.Fatal("preparation did not seal the exact bytes", err)
	}
	producer["port"], provenance[0].Digest, schema.ID, data[0] = "changed", rawDigest([]byte("changed")), "test:changed", '!'
	if prepared.Producer["port"] != "output" || prepared.SchemaRef == nil || *prepared.SchemaRef != wantSchema || prepared.Provenance[0] != source.Ref() {
		t.Fatal("prepared metadata aliases mutable caller input")
	}
	encoded, err := canonical(prepared)
	if err != nil {
		t.Fatal(err)
	}
	if err := flow.ValidateProtocol("ArtifactRevision", encoded); err != nil {
		t.Fatal("preparation returned a different artifact wire shape", err)
	}
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	reader, err := Open(e.Root, true)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if _, _, err := reader.Artifact(prepared.Ref()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("reopen promoted a sealed candidate: %v", err)
	}
	if _, err := reader.prepareArtifact(wantData, "json", &wantSchema, prepared.ID, prepared.Producer, prepared.Provenance, reg); !errors.Is(err, local.ErrReadOnly) {
		t.Fatal("read-only prepare did not refuse", err)
	}
	if _, err := reader.publishPreparedArtifact(prepared, reg); !errors.Is(err, local.ErrReadOnly) {
		t.Fatal("read-only publication did not refuse", err)
	}
	writer, err := Open(e.Root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	var recovered Artifact
	if err := decode(encoded, &recovered); err != nil {
		t.Fatal(err)
	}
	// This is an EvidenceRef shape fixture, not a claim that a checker ran.
	// Binding refs to successful durable CheckExecutions belongs to acceptance.
	recovered.ContentCheckEvidence = []any{map[string]any{"id": "evidence:storage-fixture", "digest": rawDigest([]byte("structural evidence ref"))}}
	accepted, err := writer.publishPreparedArtifact(recovered, reg)
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Ref() != prepared.Ref() || accepted.CreatedAt != prepared.CreatedAt || len(accepted.ContentCheckEvidence) != 1 {
		t.Fatal("publication changed prepared identity/time or dropped declared EvidenceRefs")
	}
	stored, retained, err := reader.Artifact(accepted.Ref())
	if err != nil || !bytes.Equal(retained, wantData) || stored.Provenance[0] != source.Ref() {
		t.Fatal("publication did not expose the exact prepared bytes/provenance", err)
	}
	metadata, err := os.ReadFile(filepath.Join(e.Root, artifactMetadataPath(accepted.ID)))
	if err != nil {
		t.Fatal(err)
	}
	retry := recovered
	retry.CreatedAt = "2000-01-01T00:00:00Z"
	again, err := writer.publishPreparedArtifact(retry, reg)
	if err != nil {
		t.Fatal(err)
	}
	againBytes, err := canonical(again)
	if err != nil || !bytes.Equal(metadata, againBytes) {
		t.Fatal("exact publication retry changed the first accepted metadata", err)
	}
	preparedBytes, err := canonical(recovered)
	if err != nil || !bytes.Equal(metadata, preparedBytes) {
		t.Fatal("publication normalized or invented metadata", err)
	}
	changedEvidence := recovered
	changedEvidence.ContentCheckEvidence = []any{}
	if _, err := writer.publishPreparedArtifact(changedEvidence, reg); !errors.Is(err, ErrArtifactIdentity) {
		t.Fatal("publication retry rewrote already-accepted evidence metadata", err)
	}
	runs, after, err := reader.Store.ReadAll(context.Background(), 10)
	if err != nil || len(runs) != 0 || after != before {
		t.Fatal("artifact preparation/publication created a Run or acceptance journal", err)
	}
	if slot, _, err := reader.Store.Slot(context.Background()); err != nil || slot != "" {
		t.Fatal("artifact storage helpers admitted a checker or worker", err)
	}
}

func TestArtifactPreparationRejectsBeforeSealing(t *testing.T) {
	e := artifactEngine(t)
	schema, reg := artifactSchema(t, e, `{"type":"integer"}`)
	for _, name := range []string{"json", "missing_schema", "producer", "identity", "provenance", "media", "size"} {
		t.Run(name, func(t *testing.T) {
			data, id := []byte("7"), "artifact:invalid-"+name
			selected, producer := &schema, artifactProducer(e)
			provenance, media := []ArtifactRef{}, "application/json"
			switch name {
			case "json":
				data = []byte(`"not an integer"`)
			case "missing_schema":
				selected = nil
			case "producer":
				producer["kind"] = "invalid"
			case "identity":
				id = "not a valid identity"
			case "provenance":
				provenance = []ArtifactRef{{ArtifactID: "artifact:absent", Revision: 1, Digest: rawDigest([]byte("absent"))}}
			case "media":
				media = "text/plain"
			case "size":
				data = bytes.Repeat([]byte("x"), MaxArtifactBytes+1)
			}
			if _, err := e.prepareArtifact(data, "json", selected, id, producer, provenance, reg, media); err == nil {
				t.Fatal("invalid artifact was prepared")
			}
			blob := filepath.Join(e.Root, e.Config.Configuration.ArtifactRoot, strings.TrimPrefix(rawDigest(data), "sha256:"))
			if _, err := os.Stat(blob); !os.IsNotExist(err) {
				t.Fatal("validation failure sealed unvalidated bytes", err)
			}
			if entries, err := os.ReadDir(filepath.Join(e.Root, ".prifly", "artifact-refs")); err != nil || len(entries) != 0 {
				t.Fatal("validation failure published metadata", err)
			}
		})
	}
}

func TestArtifactPreparedPublicationRevalidates(t *testing.T) {
	for _, name := range []string{"missing_blob", "corrupt_blob", "size", "oversized", "revision", "producer", "null_provenance", "null_evidence", "invalid_evidence", "media", "missing_schema", "json_schema", "blob_schema", "lost_provenance"} {
		t.Run(name, func(t *testing.T) {
			e := artifactEngine(t)
			schema, reg := artifactSchema(t, e, `true`)
			source, err := e.putArtifact([]byte("source"), "blob", nil, "artifact:source", artifactProducer(e), nil, reg)
			if err != nil {
				t.Fatal(err)
			}
			format := "json"
			if name == "blob_schema" || name == "media" {
				format = "blob"
			}
			prepared, err := e.prepareArtifact([]byte(`{"value":7}`), format, &schema, "artifact:pending", artifactProducer(e), []ArtifactRef{source.Ref()}, reg)
			if err != nil {
				t.Fatal(err)
			}
			ref := prepared.Ref()
			blob := filepath.Join(e.Root, e.Config.Configuration.ArtifactRoot, strings.TrimPrefix(prepared.Digest, "sha256:"))
			switch name {
			case "missing_blob":
				err = os.Remove(blob)
			case "corrupt_blob":
				if err = os.Chmod(blob, 0600); err == nil {
					err = os.WriteFile(blob, []byte(`{"value":8}`), 0600)
				}
			case "size":
				prepared.SizeBytes++
			case "oversized":
				prepared.SizeBytes = MaxArtifactBytes + 1
			case "revision":
				prepared.Revision = 2
			case "producer":
				prepared.Producer["kind"] = "not-a-producer"
			case "null_provenance":
				prepared.Provenance = nil
			case "null_evidence":
				prepared.ContentCheckEvidence = nil
			case "invalid_evidence":
				prepared.ContentCheckEvidence = []any{map[string]any{"id": "evidence:invalid", "digest": rawDigest(nil), "claim": "content_valid"}}
			case "media":
				prepared.MediaType = "Application/Octet-Stream"
			case "missing_schema":
				delete(reg, schema)
			case "json_schema", "blob_schema":
				ref := flow.Ref{ID: "test:schema/reject", Version: "1.0.0", Digest: rawDigest([]byte("false"))}
				reg[ref], prepared.SchemaRef = []byte("false"), &ref
			case "lost_provenance":
				err = os.Remove(filepath.Join(e.Root, e.Config.Configuration.ArtifactRoot, strings.TrimPrefix(source.Digest, "sha256:")))
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := e.publishPreparedArtifact(prepared, reg); err == nil {
				t.Fatal("publication trusted stale prepared validation")
			}
			if _, _, err := e.Artifact(ref); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("failed publication exposed accepted metadata: %v", err)
			}
			if entries, err := os.ReadDir(filepath.Join(e.Root, ".prifly", "artifact-refs")); err != nil || len(entries) != 1 {
				t.Fatal("failed publication changed the metadata set", err)
			}
			if name == "missing_blob" {
				if _, err := os.Stat(blob); !os.IsNotExist(err) {
					t.Fatal("publication reconstructed a missing blob", err)
				}
			}
		})
	}
}

func TestArtifactPreparationAcceptedIdentityNeverRepairs(t *testing.T) {
	for _, failure := range []string{"missing", "corrupt"} {
		t.Run(failure, func(t *testing.T) {
			e := artifactEngine(t)
			_, reg, err := e.Inventory()
			if err != nil {
				t.Fatal(err)
			}
			data, producer := []byte("data"), artifactProducer(e)
			accepted, err := e.putArtifact(data, "blob", nil, "artifact:accepted", producer, nil, reg)
			if err != nil {
				t.Fatal(err)
			}
			metadataPath := filepath.Join(e.Root, artifactMetadataPath(accepted.ID))
			before, err := os.ReadFile(metadataPath)
			if err != nil {
				t.Fatal(err)
			}
			blob := filepath.Join(e.Root, e.Config.Configuration.ArtifactRoot, strings.TrimPrefix(accepted.Digest, "sha256:"))
			if failure == "missing" {
				err = os.Remove(blob)
			} else if err = os.Chmod(blob, 0600); err == nil {
				err = os.WriteFile(blob, []byte("evil"), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := e.prepareArtifact(data, "blob", nil, accepted.ID, producer, nil, reg); !errors.Is(err, local.ErrIntegrity) {
				t.Fatal("prepare repaired or misclassified accepted evidence loss", err)
			}
			if _, err := e.publishPreparedArtifact(accepted, reg); !errors.Is(err, local.ErrIntegrity) {
				t.Fatal("publication repaired or misclassified accepted evidence loss", err)
			}
			after, err := os.ReadFile(metadataPath)
			if err != nil || !bytes.Equal(after, before) {
				t.Fatal("failed retry changed accepted metadata", err)
			}
			contents, err := os.ReadFile(blob)
			if failure == "missing" && !os.IsNotExist(err) || failure == "corrupt" && (err != nil || !bytes.Equal(contents, []byte("evil"))) {
				t.Fatal("a retry replaced the missing/corrupt evidence", err)
			}
		})
	}
}

func TestArtifactPreparedConcurrentIdentityPublication(t *testing.T) {
	e := artifactEngine(t)
	_, reg, err := e.Inventory()
	if err != nil {
		t.Fatal(err)
	}
	candidates := make([]Artifact, 2)
	for i, data := range [][]byte{[]byte("first"), []byte("second")} {
		candidates[i], err = e.prepareArtifact(data, "blob", nil, "artifact:competing", artifactProducer(e), nil, reg)
		if err != nil {
			t.Fatal(err)
		}
	}
	// Preparation does not reserve identity in another index. The existing
	// no-replace metadata publication chooses one complete accepted record.
	const writers = 8
	type result struct {
		artifact Artifact
		err      error
	}
	start, results := make(chan struct{}), make(chan result, writers)
	var wg sync.WaitGroup
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			artifact, err := e.publishPreparedArtifact(candidates[i%2], reg)
			results <- result{artifact, err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	var first []byte
	successes, conflicts := 0, 0
	for result := range results {
		if result.err != nil {
			if !errors.Is(result.err, ErrArtifactIdentity) {
				t.Fatal(result.err)
			}
			conflicts++
			continue
		}
		successes++
		encoded, err := canonical(result.artifact)
		if err != nil {
			t.Fatal(err)
		}
		if first == nil {
			first = encoded
		} else if !bytes.Equal(first, encoded) {
			t.Fatal("successful concurrent publishers returned different accepted facts")
		}
	}
	if successes != writers/2 || conflicts != writers/2 {
		t.Fatalf("publication winners=%d conflicts=%d", successes, conflicts)
	}
	metadata, err := os.ReadFile(filepath.Join(e.Root, artifactMetadataPath(candidates[0].ID)))
	if err != nil || !bytes.Equal(metadata, first) {
		t.Fatal("metadata publication was partial or replaced the winner", err)
	}
	if entries, err := os.ReadDir(filepath.Join(e.Root, ".prifly", "artifact-refs")); err != nil || len(entries) != 1 {
		t.Fatal("concurrent publishers left staging metadata or a second record", err)
	}
}
