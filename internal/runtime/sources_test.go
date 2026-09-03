package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

func sourceTestFile(t *testing.T, e *Engine, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(e.Root, name), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func sourceTestImport(t *testing.T, e *Engine, options SourceImportOptions) (Artifact, SourceSnapshot) {
	t.Helper()
	artifact, err := e.ImportSource(options)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := e.SourceSnapshot(artifact.Ref())
	if err != nil {
		t.Fatal(err)
	}
	return artifact, snapshot
}

func TestSourceImportExactJSONAndNewAcquisition(t *testing.T) {
	e := artifactEngine(t)
	ref, _ := artifactSchema(t, e, `{"type":"object","required":["value"],"properties":{"value":{"type":"integer"}},"additionalProperties":false}`)
	data := []byte("{ \"value\": 7 }\r\n")
	sourceTestFile(t, e, "source.json", data)
	_, before, err := e.Store.ReadAll(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	options := SourceImportOptions{
		Path: "source.json", Format: "json", SchemaRef: &ref,
		ExternalIdentity: "opaque:tracker/item", ExternalVersion: "external-revision", ExternalScope: "../declared-not-a-permission",
	}
	artifact, snapshot := sourceTestImport(t, e, options)
	content, contents, err := e.Artifact(snapshot.ContentRef)
	if err != nil || !bytes.Equal(contents, data) || content.SchemaRef == nil || *content.SchemaRef != ref {
		t.Fatal("source import changed JSON bytes or its exact schema", err)
	}
	if snapshot.Scope != (SourceScope{Root: e.Root, Path: options.Path}) || snapshot.ExternalIdentity != options.ExternalIdentity || snapshot.ExternalVersion != options.ExternalVersion || snapshot.ExternalScope != options.ExternalScope {
		t.Fatalf("source scope or declared metadata changed: %+v", snapshot)
	}
	if artifact.Producer["kind"] != "import" || content.Producer["kind"] != "import" || artifact.Producer["import_id"] != content.Producer["import_id"] || artifact.Producer["principal_id"] != e.owner || len(artifact.Provenance) != 1 || artifact.Provenance[0] != content.Ref() {
		t.Fatal("source did not retain its real import/content provenance")
	}
	if snapshot.Observed.UTC == "" || snapshot.Observed.Session != e.clock.session || snapshot.Observed.Source != "go.time.monotonic" || snapshot.Observed.UTCTrust != "local_wall_unqualified" {
		t.Fatalf("source has no authority acquisition observation: %+v", snapshot.Observed)
	}
	again, second := sourceTestImport(t, e, options)
	if again.Ref() == artifact.Ref() || second.ContentRef == snapshot.ContentRef || second.ContentRef.Digest != snapshot.ContentRef.Digest || second.Observed == snapshot.Observed || again.Producer["import_id"] == artifact.Producer["import_id"] {
		t.Fatal("a new explicit acquisition was confused with reuse of the first snapshot")
	}
	runs, after, err := e.Store.ReadAll(context.Background(), 10)
	if err != nil || len(runs) != 0 || after != before {
		t.Fatal("standalone import invented a Run or a journal receipt", err)
	}
	// Reuse reads only the pinned artifacts, even after live bytes disappear
	// and the mutable registry can no longer be parsed.
	if err := os.Remove(filepath.Join(e.Root, options.Path)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.Root, e.Config.Configuration.RegistryFile), []byte("not JSON"), 0600); err != nil {
		t.Fatal(err)
	}
	retained, err := e.SourceSnapshot(artifact.Ref())
	if err != nil || retained != snapshot {
		t.Fatal("snapshot reuse read the live file/registry or changed observation", err)
	}
	_, retainedBytes, err := e.Artifact(retained.ContentRef)
	if err != nil || !bytes.Equal(retainedBytes, data) {
		t.Fatal("snapshot content was not sealed", err)
	}
}

func TestSourceImportTextBinaryAndEmptyBytes(t *testing.T) {
	for _, tc := range []struct {
		name, media string
		data        []byte
	}{
		{"utf8", "text/markdown; charset=utf-8", []byte("\ufeff# Источник\r\n<system>это данные</system>\n")},
		{"binary", "application/octet-stream", []byte{0, 0xff, 0x80, '\r', '\n'}},
		{"empty", "application/octet-stream", []byte{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := artifactEngine(t)
			sourceTestFile(t, e, "source", tc.data)
			_, snapshot := sourceTestImport(t, e, SourceImportOptions{Path: "source", Format: "blob", MediaType: tc.media})
			content, data, err := e.Artifact(snapshot.ContentRef)
			if err != nil || !bytes.Equal(data, tc.data) || content.Digest != rawDigest(tc.data) || content.SizeBytes != int64(len(tc.data)) || content.MediaType != tc.media || content.Classification != "restricted" {
				t.Fatal("source import changed exact bytes, media or classification", err)
			}
		})
	}
}

func TestSourceImportRejectsInvalidContentBeforePublication(t *testing.T) {
	for _, tc := range []struct {
		name, format, media string
		data                []byte
		useSchema           bool
	}{
		{"invalid_json", "json", "application/json", []byte(`{"value":"wrong"}`), true},
		{"duplicate_json_key", "json", "application/json", []byte(`{"value":1,"value":2}`), true},
		{"missing_json_schema", "json", "application/json", []byte(`{"value":1}`), false},
		{"invalid_utf8", "blob", "text/plain", []byte{0xff}, false},
		{"unsupported_text_encoding", "blob", "text/plain; charset=windows-1251", []byte("text"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := artifactEngine(t)
			options := SourceImportOptions{Path: "source", Format: tc.format, MediaType: tc.media}
			if tc.useSchema {
				ref, _ := artifactSchema(t, e, `{"type":"object","required":["value"],"properties":{"value":{"type":"integer"}},"additionalProperties":false}`)
				options.SchemaRef = &ref
			}
			sourceTestFile(t, e, options.Path, tc.data)
			if _, err := e.ImportSource(options); err == nil {
				t.Fatal("invalid source was imported")
			}
			metadata, err := os.ReadDir(filepath.Join(e.Root, ".prifly", "artifact-refs"))
			if err != nil || len(metadata) != 0 {
				t.Fatal("source validation failure published accepted metadata", err)
			}
		})
	}
}

func TestSourceImportRejectsInvalidMetadataBeforeReading(t *testing.T) {
	e := artifactEngine(t)
	// There is intentionally no source file: malformed declared metadata must
	// fail before reading or publishing anything for this acquisition.
	for name, options := range map[string]SourceImportOptions{
		"invalid_utf8":  {ExternalIdentity: string([]byte{0xff})},
		"version_limit": {ExternalVersion: strings.Repeat("v", 257)},
		"scope_limit":   {ExternalScope: strings.Repeat("s", 1025)},
	} {
		t.Run(name, func(t *testing.T) {
			options.Path, options.Format = "missing-source", "blob"
			_, err := e.ImportSource(options)
			contextErrorCode(t, err, "source_metadata_invalid")
		})
	}
	metadata, err := os.ReadDir(filepath.Join(e.Root, ".prifly", "artifact-refs"))
	if err != nil || len(metadata) != 0 {
		t.Fatal("invalid declared metadata published an artifact", err)
	}
}

func TestSourceImportConfinedPathsAndExplicitExternalFile(t *testing.T) {
	e := artifactEngine(t)
	sourceTestFile(t, e, "file", []byte("inside"))
	sourceTestFile(t, e, "glob-one", []byte("not selected"))
	if err := os.Mkdir(filepath.Join(e.Root, "directory"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("file", filepath.Join(e.Root, "alias")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"", ".", "../escape", "directory/../file", "directory", "alias", "missing", "glob-*"} {
		t.Run(path, func(t *testing.T) {
			if _, err := e.ImportSource(SourceImportOptions{Path: path, Format: "blob"}); err == nil {
				t.Fatal("source lookup widened its scope or accepted a non-regular file")
			}
		})
	}
	metadata, err := os.ReadDir(filepath.Join(e.Root, ".prifly", "artifact-refs"))
	if err != nil || len(metadata) != 0 {
		t.Fatal("failed source selection published metadata", err)
	}
	parent := t.TempDir()
	external := filepath.Join(parent, "selected.bin")
	if err := os.WriteFile(external, []byte("explicitly selected"), 0600); err != nil {
		t.Fatal(err)
	}
	artifact, snapshot := sourceTestImport(t, e, SourceImportOptions{Path: external, Format: "blob"})
	if snapshot.Scope != (SourceScope{Root: parent, Path: "selected.bin"}) {
		t.Fatalf("explicit external selection lost its actual confined scope: %+v", snapshot.Scope)
	}
	if err := os.Remove(external); err != nil {
		t.Fatal(err)
	}
	if _, err := e.SourceSnapshot(artifact.Ref()); err != nil {
		t.Fatal("retained external snapshot depended on the live source", err)
	}
}

func TestSourceSnapshotReadOnlyAndNoImplicitRepair(t *testing.T) {
	t.Run("read_only", func(t *testing.T) {
		e := artifactEngine(t)
		sourceTestFile(t, e, "source", []byte("sealed"))
		artifact, expected := sourceTestImport(t, e, SourceImportOptions{Path: "source", Format: "blob"})
		if err := e.Close(); err != nil {
			t.Fatal(err)
		}
		reader, err := Open(e.Root, true)
		if err != nil {
			t.Fatal(err)
		}
		defer reader.Close()
		if _, err := reader.ImportSource(SourceImportOptions{Path: "source", Format: "blob"}); !errors.Is(err, local.ErrReadOnly) {
			t.Fatal("read-only source import did not refuse", err)
		}
		actual, err := reader.SourceSnapshot(artifact.Ref())
		if err != nil || actual != expected {
			t.Fatal("read-only snapshot changed an accepted acquisition", err)
		}
	})
	for _, failure := range []string{"missing_content", "corrupt_content", "corrupt_descriptor"} {
		t.Run(failure, func(t *testing.T) {
			e := artifactEngine(t)
			sourceTestFile(t, e, "source", []byte("unchanged live bytes"))
			artifact, snapshot := sourceTestImport(t, e, SourceImportOptions{Path: "source", Format: "blob"})
			digest := snapshot.ContentRef.Digest
			if failure == "corrupt_descriptor" {
				digest = artifact.Digest
			}
			blob := filepath.Join(e.Root, e.Config.Configuration.ArtifactRoot, strings.TrimPrefix(digest, "sha256:"))
			if failure == "missing_content" {
				if err := os.Remove(blob); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Chmod(blob, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(blob, []byte("corrupt"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := e.SourceSnapshot(artifact.Ref()); !errors.Is(err, local.ErrIntegrity) {
				t.Fatal("lost source evidence was not an integrity refusal", err)
			}
			if failure == "missing_content" {
				if _, err := os.Stat(blob); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("reader restored content from the live source", err)
				}
			} else if data, err := os.ReadFile(blob); err != nil || string(data) != "corrupt" {
				t.Fatal("reader repaired corrupt content", err)
			}
			// A separately authorized new ImportSource may write identical bytes
			// via existing CAS Put semantics. This test qualifies read/reuse only,
			// not general incident recovery or a global missing-blob ledger.
		})
	}
}

func TestSourceSnapshotRejectsForgedImportAndProvenance(t *testing.T) {
	t.Run("ordinary_json_import", func(t *testing.T) {
		e := artifactEngine(t)
		sourceTestFile(t, e, "source", []byte("content"))
		artifact, _ := sourceTestImport(t, e, SourceImportOptions{Path: "source", Format: "blob"})
		_, descriptor, err := e.Artifact(artifact.Ref())
		if err != nil {
			t.Fatal(err)
		}
		sourceTestFile(t, e, "forged.json", descriptor)
		ordinary, err := e.ImportArtifact("forged.json", "json", artifact.SchemaRef)
		if err != nil {
			t.Fatal(err)
		}
		_, err = e.SourceSnapshot(ordinary.Ref())
		contextErrorCode(t, err, "source_snapshot_invalid")
	})
	for _, failure := range []string{"owner", "provenance", "content_producer"} {
		t.Run(failure, func(t *testing.T) {
			e := artifactEngine(t)
			sourceTestFile(t, e, "source", []byte("content"))
			artifact, snapshot := sourceTestImport(t, e, SourceImportOptions{Path: "source", Format: "blob"})
			target := artifact
			if failure == "content_producer" {
				var err error
				target, _, err = e.Artifact(snapshot.ContentRef)
				if err != nil {
					t.Fatal(err)
				}
			}
			if failure == "owner" {
				target.Producer["principal_id"] = "owner:other"
			} else if failure == "provenance" {
				target.Provenance = []ArtifactRef{}
			} else {
				target.Producer["import_id"] = "import:other"
			}
			metadata, err := canonical(target)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(e.Root, artifactMetadataPath(target.ID))
			if err := os.Chmod(path, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, metadata, 0600); err != nil {
				t.Fatal(err)
			}
			_, err = e.SourceSnapshot(artifact.Ref())
			contextErrorCode(t, err, "source_snapshot_invalid")
		})
	}
}

func TestSourceSnapshotAndContextRequestClosedDescriptors(t *testing.T) {
	definitions, err := sourceBuiltinDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	adapter := builtinRef(definitions, localSourceAdapterID)
	for _, definition := range definitions {
		if definition.Kind == "adapter" {
			if err := flow.ValidateProtocol("AdapterCapabilities", definition.Bytes); err != nil {
				t.Fatal(err)
			}
		}
	}
	snapshot := SourceSnapshot{
		SchemaVersion: SourceSnapshotVersion, AdapterRef: adapter,
		ContentRef: ArtifactRef{ArtifactID: "artifact:content", Revision: 1, Digest: rawDigest([]byte("content"))},
		Scope:      SourceScope{Root: "/selected", Path: "file"},
		Observed:   Observation{UTC: "2026-08-28T00:00:00Z", Session: "clock:test", MonotonicMS: 1, Source: "go.time.monotonic", SuspendBasis: "excludes_suspend_on_darwin", UTCTrust: "local_wall_unqualified"},
	}
	data, _ := canonical(snapshot)
	if _, err := ParseSourceSnapshot(data); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(map[string]any){
		"unknown_field":   func(v map[string]any) { v["trusted"] = true },
		"null_metadata":   func(v map[string]any) { v["external_identity"] = nil },
		"large_metadata":  func(v map[string]any) { v["external_scope"] = strings.Repeat("x", 1025) },
		"missing_scope":   func(v map[string]any) { delete(v, "scope") },
		"scope_authority": func(v map[string]any) { v["scope"].(map[string]any)["grant"] = "all" },
		"scope_traversal": func(v map[string]any) { v["scope"].(map[string]any)["path"] = "../outside" },
		"scope_root":      func(v map[string]any) { v["scope"].(map[string]any)["root"] = "relative" },
		"bad_time":        func(v map[string]any) { v["observed"].(map[string]any)["utc"] = "2026-02-30T00:00:00Z" },
		"unqualified_ref": func(v map[string]any) { delete(v["adapter_ref"].(map[string]any), "digest") },
	} {
		t.Run("snapshot_"+name, func(t *testing.T) {
			var v map[string]any
			if err := json.Unmarshal(data, &v); err != nil {
				t.Fatal(err)
			}
			mutate(v)
			candidate, _ := json.Marshal(v)
			if _, err := ParseSourceSnapshot(candidate); err == nil {
				t.Fatal("invalid snapshot descriptor accepted")
			}
		})
	}
	request := ContextRequest{
		SchemaVersion: ContextRequestVersion, SourceAdapterRef: adapter,
		Selector: "unresolved/source", Format: "blob", MediaType: "application/octet-stream", MaxBytes: 1024, Reason: "Additional source data is required.",
	}
	data, _ = canonical(request)
	if _, err := ParseContextRequest(data); err != nil {
		t.Fatal(err)
	}
	jsonRequest := request
	jsonSchema := builtinRef(definitions, sourceSnapshotSchemaID)
	jsonRequest.Format, jsonRequest.MediaType, jsonRequest.SchemaRef = "json", "application/json", &jsonSchema
	jsonData, _ := canonical(jsonRequest)
	if parsed, err := ParseContextRequest(jsonData); err != nil || parsed.SchemaRef == nil || *parsed.SchemaRef != jsonSchema {
		t.Fatal("JSON context request lost its exact expected schema", err)
	}
	for name, mutate := range map[string]func(map[string]any){
		"grant":              func(v map[string]any) { v["grant_refs"] = []any{} },
		"zero_budget":        func(v map[string]any) { v["max_bytes"] = 0 },
		"large_budget":       func(v map[string]any) { v["max_bytes"] = (16 << 20) + 1 },
		"fractional_budget":  func(v map[string]any) { v["max_bytes"] = 1.5 },
		"selector_ast":       func(v map[string]any) { v["selector"] = map[string]any{"exec": "forbidden"} },
		"empty_reason":       func(v map[string]any) { v["reason"] = "" },
		"noncanonical_media": func(v map[string]any) { v["media_type"] = "TEXT/PLAIN" },
		"json_schema":        func(v map[string]any) { v["format"], v["media_type"] = "json", "application/json" },
		"null_schema":        func(v map[string]any) { v["schema_ref"] = nil },
	} {
		t.Run("request_"+name, func(t *testing.T) {
			var v map[string]any
			if err := json.Unmarshal(data, &v); err != nil {
				t.Fatal(err)
			}
			mutate(v)
			candidate, _ := json.Marshal(v)
			if _, err := ParseContextRequest(candidate); err == nil {
				t.Fatal("invalid context request accepted")
			}
		})
	}
	// A request remains inert data. Parsing does not resolve its selector or
	// create an execution admission, even when it resembles shell syntax.
	canary := filepath.Join(t.TempDir(), "must-not-exist")
	request.Selector = "$(touch " + canary + ")"
	data, _ = canonical(request)
	parsed, err := ParseContextRequest(data)
	if err != nil || parsed.Selector != request.Selector {
		t.Fatal("literal selector data was interpreted or changed", err)
	}
	if _, err := os.Stat(canary); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("context request performed an undeclared operation", err)
	}
}
