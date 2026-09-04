package runtime

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

func artifactEngine(t testing.TB) *Engine {
	t.Helper()
	root := t.TempDir()
	if err := Init(root); err != nil {
		t.Fatal(err)
	}
	e, err := Open(root, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close() })
	return e
}

func artifactSchema(t *testing.T, e *Engine, value string) (flow.Ref, flow.Registry) {
	t.Helper()
	data, err := flow.Canonical([]byte(value))
	if err != nil {
		t.Fatal(err)
	}
	digest, err := flow.Digest(data)
	if err != nil {
		t.Fatal(err)
	}
	ref := flow.Ref{ID: "test:schema/artifact", Version: "1.0.0", Digest: digest}
	if err := os.WriteFile(filepath.Join(e.Root, "schemas", "artifact.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	registry, err := json.Marshal(RegistryFile{SchemaVersion: "1", Entries: []Definition{{Ref: ref, Kind: "schema", Path: "schemas/artifact.json"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.Root, e.Config.Configuration.RegistryFile), registry, 0600); err != nil {
		t.Fatal(err)
	}
	_, reg, err := e.Inventory()
	if err != nil {
		t.Fatal(err)
	}
	return ref, reg
}

func artifactProducer(e *Engine) map[string]any {
	return map[string]any{"kind": "authority", "authority_id": e.Installation.ID, "command_id": "command:test", "port": "output"}
}

func TestArtifactImportExportAndStableIdentity(t *testing.T) {
	e := artifactEngine(t)
	data := []byte("opaque\x00bytes\n")
	if err := os.WriteFile(filepath.Join(e.Root, "input.bin"), data, 0600); err != nil {
		t.Fatal(err)
	}
	a, err := e.ImportArtifact("input.bin", "blob", nil)
	if err != nil {
		t.Fatal(err)
	}
	if a.Digest != rawDigest(data) || a.SizeBytes != int64(len(data)) || a.Format != "blob" || a.Classification != "restricted" {
		t.Fatalf("incorrect metadata: %+v", a)
	}
	again, err := e.ImportArtifact("input.bin", "blob", nil)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, _ := canonical(a)
	againJSON, _ := canonical(again)
	if !bytes.Equal(firstJSON, againJSON) {
		t.Fatal("repeat changed identity, provenance or creation time")
	}
	var invalid map[string]any
	if err := json.Unmarshal(firstJSON, &invalid); err != nil {
		t.Fatal(err)
	}
	invalid["producer"].(map[string]any)["source_ref"].(map[string]any)["version"] = "not-a-version"
	invalidJSON, err := json.Marshal(invalid)
	if err != nil {
		t.Fatal(err)
	}
	if err := flow.ValidateProtocol("ArtifactRevision", invalidJSON); err == nil {
		t.Fatal("invalid source reference version passed the protocol schema")
	}
	if err := os.WriteFile(filepath.Join(e.Root, "input.bin"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	stored, contents, err := e.Artifact(a.Ref())
	if err != nil || stored.Ref() != a.Ref() || !bytes.Equal(contents, data) {
		t.Fatalf("source edit changed artifact: %+v %q %v", stored, contents, err)
	}
	if err := e.ExportArtifact(a.Ref(), "copy.bin"); err != nil {
		t.Fatal(err)
	}
	if err := e.ExportArtifact(a.Ref(), "copy.bin"); !errors.Is(err, os.ErrExist) {
		t.Fatalf("export overwrote a file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(e.Root, "copy.bin"), []byte("mutable"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, contents, err := e.Artifact(a.Ref()); err != nil || !bytes.Equal(contents, data) {
		t.Fatal("export shared authority bytes")
	}
	entries, err := os.ReadDir(filepath.Join(e.Root, ".prifly", "artifact-refs"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("metadata or staging leak: %d %v", len(entries), err)
	}
}

func TestArtifactJSONValidationPreservesRawBytes(t *testing.T) {
	e := artifactEngine(t)
	ref, _ := artifactSchema(t, e, `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","required":["value"],"properties":{"value":{"type":"integer","minimum":0}},"additionalProperties":false}`)
	for _, invalid := range []string{`{"value":"one"}`, `{"value":1,"value":2}`, `{"other":1}`} {
		if err := os.WriteFile(filepath.Join(e.Root, "value.json"), []byte(invalid), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := e.ImportArtifact("value.json", "json", &ref); err == nil {
			t.Fatalf("accepted invalid JSON/schema: %s", invalid)
		}
	}
	if entries, err := os.ReadDir(filepath.Join(e.Root, ".prifly", "artifact-refs")); err != nil || len(entries) != 0 {
		t.Fatal("validation failure published metadata")
	}
	data := []byte("{ \"value\": 7 }\n")
	if err := os.WriteFile(filepath.Join(e.Root, "value.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	a, err := e.ImportArtifact("value.json", "json", &ref)
	if err != nil {
		t.Fatal(err)
	}
	canonicalDigest, _ := flow.Digest(data)
	if a.Digest != rawDigest(data) || a.Digest == canonicalDigest {
		t.Fatal("artifact bytes were canonicalized before sealing")
	}
	// History is independent of later installed definition edits.
	if err := os.WriteFile(filepath.Join(e.Root, "schemas", "artifact.json"), []byte(`{"type":"string"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, contents, err := e.Artifact(a.Ref()); err != nil || !bytes.Equal(contents, data) {
		t.Fatalf("historical artifact consulted mutable registry: %q %v", contents, err)
	}
}

func TestArtifactDescriptorSchemaIsNotContentValidation(t *testing.T) {
	e := artifactEngine(t)
	ref, reg := artifactSchema(t, e, `{"type":"object","required":["media_type","size_bytes","digest"],"properties":{"media_type":{"const":"application/octet-stream"},"size_bytes":{"const":4},"digest":{"type":"string"}},"additionalProperties":false}`)
	a, err := e.putArtifact([]byte("data"), "blob", &ref, "artifact:descriptor", artifactProducer(e), nil, reg)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.ContentCheckEvidence) != 0 {
		t.Fatal("descriptor validation invented content evidence")
	}
	if _, err := e.putArtifact([]byte("longer"), "blob", &ref, "artifact:too-long", artifactProducer(e), nil, reg); err == nil {
		t.Fatal("descriptor size ignored")
	}
}

func TestArtifactExplicitMediaType(t *testing.T) {
	e := artifactEngine(t)
	if err := os.WriteFile(filepath.Join(e.Root, "input.txt"), []byte("text"), 0600); err != nil {
		t.Fatal(err)
	}
	a, err := e.ImportArtifact("input.txt", "blob", nil, "text/plain")
	if err != nil || a.MediaType != "text/plain" {
		t.Fatalf("declared media type: %+v %v", a, err)
	}
	b, err := e.ImportArtifact("input.txt", "blob", nil)
	if err != nil || b.ID == a.ID || b.Digest != a.Digest {
		t.Fatalf("media declaration missing from identity: %+v %v", b, err)
	}
	for _, bad := range []string{"text", "*/*", "text/plain\r\nX-Secret: leaked", "text/plain; broken"} {
		if _, err := e.ImportArtifact("input.txt", "blob", nil, bad); err == nil {
			t.Fatalf("bad media type accepted: %q", bad)
		}
	}
	if _, err := artifactMediaType("json", []string{"text/plain"}); err == nil {
		t.Fatal("JSON media type overridden")
	}
	if _, err := artifactMediaType("blob", []string{"text/plain", "application/json"}); err == nil {
		t.Fatal("multiple media types selected")
	}
}

func TestArtifactIdentityConflictAndProvenance(t *testing.T) {
	e := artifactEngine(t)
	_, reg, err := e.Inventory()
	if err != nil {
		t.Fatal(err)
	}
	a, err := e.putArtifact([]byte("first"), "blob", nil, "artifact:output", artifactProducer(e), nil, reg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.putArtifact([]byte("other"), "blob", nil, a.ID, artifactProducer(e), nil, reg); !errors.Is(err, ErrArtifactIdentity) {
		t.Fatalf("identity overwritten: %v", err)
	}
	producer := artifactProducer(e)
	producer["port"] = "different"
	if _, err := e.putArtifact([]byte("first"), "blob", nil, a.ID, producer, nil, reg); !errors.Is(err, ErrArtifactIdentity) {
		t.Fatalf("producer overwritten: %v", err)
	}
	if _, err := e.putArtifact([]byte("derived"), "blob", nil, "artifact:derived", artifactProducer(e), []ArtifactRef{a.Ref()}, reg); err != nil {
		t.Fatal(err)
	}
	missing := ArtifactRef{ArtifactID: "artifact:missing", Revision: 1, Digest: rawDigest([]byte("missing"))}
	if _, _, err := e.Artifact(missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ordinary absent identity was not reported as missing: %v", err)
	} else if problem, exit := ProblemFor(err); problem.Code != "not_found" || exit != 2 {
		t.Fatalf("ordinary absent identity was presented as evidence corruption: %+v exit=%d", problem, exit)
	}
	if _, err := e.putArtifact([]byte("derived"), "blob", nil, "artifact:false-provenance", artifactProducer(e), []ArtifactRef{missing}, reg); err == nil {
		t.Fatal("accepted missing provenance")
	}
	bad := a.Ref()
	bad.Revision = 2
	if _, _, err := e.Artifact(bad); !errors.Is(err, local.ErrIntegrity) {
		t.Fatalf("accepted different revision: %v", err)
	}
}

func TestArtifactCorruptionNeverHealsImplicitly(t *testing.T) {
	for _, failure := range []string{"missing", "corrupt"} {
		t.Run(failure, func(t *testing.T) {
			e := artifactEngine(t)
			if err := os.WriteFile(filepath.Join(e.Root, "input"), []byte("data"), 0600); err != nil {
				t.Fatal(err)
			}
			a, err := e.ImportArtifact("input", "blob", nil)
			if err != nil {
				t.Fatal(err)
			}
			blob := filepath.Join(e.Root, e.Config.Configuration.ArtifactRoot, strings.TrimPrefix(a.Digest, "sha256:"))
			if failure == "missing" {
				err = os.Remove(blob)
			} else {
				if err := os.Chmod(blob, 0600); err != nil {
					t.Fatal(err)
				}
				err = os.WriteFile(blob, []byte("evil"), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := e.Artifact(a.Ref()); !errors.Is(err, local.ErrIntegrity) {
				t.Fatalf("accepted artifact loss was not an integrity incident: %v", err)
			} else if problem, exit := ProblemFor(err); problem.Code != "integrity_failure" || exit != 6 {
				t.Fatalf("accepted artifact loss was presented as ordinary input absence: %+v exit=%d", problem, exit)
			}
			if _, err := e.ImportArtifact("input", "blob", nil); !errors.Is(err, local.ErrIntegrity) {
				t.Fatalf("implicit repair hid integrity incident: %v", err)
			}
		})
	}
}

func TestArtifactReadOnlyPathsAndConcurrentPublication(t *testing.T) {
	e := artifactEngine(t)
	if err := os.WriteFile(filepath.Join(e.Root, "input"), []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("input", filepath.Join(e.Root, "alias")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"../escape", "alias"} {
		if _, err := e.ImportArtifact(path, "blob", nil); err == nil {
			t.Fatalf("unsafe path accepted: %s", path)
		}
	}
	const writers = 8
	var wg sync.WaitGroup
	results := make(chan Artifact, writers)
	failures := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, err := e.ImportArtifact("input", "blob", nil)
			results <- a
			failures <- err
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	var first Artifact
	for a := range results {
		if first.ID == "" {
			first = a
		}
		x, _ := canonical(first)
		y, _ := canonical(a)
		if !bytes.Equal(x, y) {
			t.Fatal("concurrent metadata writers returned different facts")
		}
	}
	ro, err := Open(e.Root, true)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	if _, _, err := ro.Artifact(first.Ref()); err != nil {
		t.Fatal(err)
	}
	if _, err := ro.ImportArtifact("input", "blob", nil); !errors.Is(err, local.ErrReadOnly) {
		t.Fatalf("readonly import: %v", err)
	}
	if err := ro.ExportArtifact(first.Ref(), "copy"); !errors.Is(err, local.ErrReadOnly) {
		t.Fatalf("readonly export: %v", err)
	}
}
