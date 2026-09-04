package runtime

import (
	"encoding/json"
	"mime"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

const (
	SourceSnapshotVersion  = "source-snapshot/1"
	ContextRequestVersion  = "context-request/1"
	sourceSnapshotSchemaID = "core:schema/source-snapshot"
	contextRequestSchemaID = "core:schema/context-request"
	localSourceAdapterID   = "core:adapter/local-source"
)

// SourceScope records the actual root and relative file selected by the owner.
// It is acquisition provenance, not a permission to open that path again.
type SourceScope struct {
	Root string `json:"root"`
	Path string `json:"path"`
}

// External fields are declared, unverified metadata. Core does not interpret
// tracker IDs, source versions or external scopes as authority or instructions.
type SourceSnapshot struct {
	SchemaVersion    string      `json:"schema_version"`
	AdapterRef       flow.Ref    `json:"adapter_ref"`
	ContentRef       ArtifactRef `json:"content_ref"`
	Scope            SourceScope `json:"scope"`
	Observed         Observation `json:"observed"`
	ExternalIdentity string      `json:"external_identity,omitempty"`
	ExternalVersion  string      `json:"external_version,omitempty"`
	ExternalScope    string      `json:"external_scope,omitempty"`
}

type SourceImportOptions struct {
	Path             string
	Format           string
	MediaType        string
	SchemaRef        *flow.Ref
	ExternalIdentity string
	ExternalVersion  string
	ExternalScope    string
}

// ContextRequest is a declared output value, normally paired with the existing
// needs_revision verdict. No parser or import operation dispatches its selector.
type ContextRequest struct {
	SchemaVersion    string    `json:"schema_version"`
	SourceAdapterRef flow.Ref  `json:"source_adapter_ref"`
	Selector         string    `json:"selector"`
	Format           string    `json:"format"`
	SchemaRef        *flow.Ref `json:"schema_ref,omitempty"`
	MediaType        string    `json:"media_type"`
	MaxBytes         int64     `json:"max_bytes"`
	Reason           string    `json:"reason"`
}

func SourceSnapshotSchema() ([]byte, error) {
	ref := func(name string) any { return map[string]any{"$ref": "#/$defs/" + name} }
	bounded := func(maximum int) any {
		return map[string]any{"type": "string", "minLength": 1, "maxLength": maximum}
	}
	document, err := sourceContractSchema("urn:prifly:source-snapshot:1", map[string]any{
		"schema_version": map[string]any{"const": SourceSnapshotVersion},
		"adapter_ref":    ref("ImmutableRef"), "content_ref": ref("ArtifactRef"),
		"scope": map[string]any{"type": "object", "properties": map[string]any{
			"root": bounded(4096), "path": bounded(4096),
		}, "required": []string{"root", "path"}, "additionalProperties": false},
		"observed": map[string]any{"type": "object", "properties": map[string]any{
			"utc": ref("Timestamp"), "session": ref("Identifier"),
			"monotonic_ms": map[string]any{"type": "integer", "minimum": 0, "maximum": int64(9007199254740991)},
			"source":       bounded(128), "suspend_basis": bounded(128), "utc_trust": bounded(128),
		}, "required": []string{"utc", "session", "monotonic_ms", "source", "suspend_basis", "utc_trust"}, "additionalProperties": false},
		"external_identity": bounded(1024), "external_version": bounded(256), "external_scope": bounded(1024),
	}, []string{"schema_version", "adapter_ref", "content_ref", "scope", "observed"})
	if err != nil {
		return nil, err
	}
	return canonical(document)
}

func ContextRequestSchema() ([]byte, error) {
	ref := func(name string) any { return map[string]any{"$ref": "#/$defs/" + name} }
	bounded := func(maximum int) any {
		return map[string]any{"type": "string", "minLength": 1, "maxLength": maximum}
	}
	// The v1 request contract has a fixed 16 MiB ceiling. Increasing a future
	// storage limit must not silently alter this schema's immutable identity.
	document, err := sourceContractSchema("urn:prifly:context-request:1", map[string]any{
		"schema_version":     map[string]any{"const": ContextRequestVersion},
		"source_adapter_ref": ref("ImmutableRef"), "selector": bounded(4096),
		"format": map[string]any{"enum": []string{"json", "blob"}}, "schema_ref": ref("ImmutableRef"),
		"media_type": bounded(128), "max_bytes": map[string]any{"type": "integer", "minimum": 1, "maximum": 16 << 20},
		"reason": bounded(4096),
	}, []string{"schema_version", "source_adapter_ref", "selector", "format", "media_type", "max_bytes", "reason"})
	if err != nil {
		return nil, err
	}
	document["if"] = map[string]any{"properties": map[string]any{"format": map[string]any{"const": "json"}}}
	document["then"] = map[string]any{"required": []string{"schema_ref"}, "properties": map[string]any{"media_type": map[string]any{"const": "application/json"}}}
	return canonical(document)
}

func sourceContractSchema(id string, properties map[string]any, required []string) (map[string]any, error) {
	defs := map[string]any{}
	for _, name := range []string{"ImmutableRef", "ArtifactRef", "Timestamp"} {
		data, err := flow.ProtocolSchema(name)
		if err != nil {
			return nil, err
		}
		var protocol struct {
			Defs map[string]any `json:"$defs"`
		}
		if err := json.Unmarshal(data, &protocol); err != nil {
			return nil, err
		}
		for name, value := range protocol.Defs {
			defs[name] = value
		}
	}
	return map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema", "$id": id, "$defs": defs,
		"type": "object", "properties": properties, "required": required, "additionalProperties": false,
	}, nil
}

// sourceBuiltinDefinitions has no dependency on Builtins or the installation
// registry: callers can include these records in Builtins and historical reads
// can check the exact implemented contract without consulting mutable files.
// The definitions are fixed by this build, so they are derived once. Every
// caller receives the same pinned records rather than rebuilding and rehashing
// three documents on each project read.
var builtinSourceDefinitions struct {
	sync.Once
	definitions []PinnedDefinition
	err         error
}

func sourceBuiltinDefinitions() ([]PinnedDefinition, error) {
	builtinSourceDefinitions.Do(func() {
		builtinSourceDefinitions.definitions, builtinSourceDefinitions.err = buildSourceBuiltinDefinitions()
	})
	return builtinSourceDefinitions.definitions, builtinSourceDefinitions.err
}

func buildSourceBuiltinDefinitions() ([]PinnedDefinition, error) {
	snapshot, err := SourceSnapshotSchema()
	if err != nil {
		return nil, err
	}
	request, err := ContextRequestSchema()
	if err != nil {
		return nil, err
	}
	adapter, err := canonical(map[string]any{
		"schema_version": "1", "id": localSourceAdapterID, "version": "1.0.0",
		"execution_modes": []string{"managed"}, "operations": []string{"import_file"},
		"context_isolation": "unknown", "process_control": "none", "effect_fencing": "none",
		"sandbox": "none", "durable_wakeup": false, "qualification_evidence": []any{},
	})
	if err != nil {
		return nil, err
	}
	definitions := []PinnedDefinition{}
	for _, item := range []struct {
		id, kind string
		data     []byte
	}{{sourceSnapshotSchemaID, "schema", snapshot}, {contextRequestSchemaID, "schema", request}, {localSourceAdapterID, "adapter", adapter}} {
		digest := rawDigest(item.data)
		definitions = append(definitions, PinnedDefinition{
			Ref: flow.Ref{ID: item.id, Version: "1.0.0", Digest: digest}, Kind: item.kind, RawDigest: digest, Bytes: item.data,
		})
	}
	return definitions, nil
}

func parseSourceContract(data, schema []byte, id string, target any) error {
	ref := flow.Ref{ID: id, Version: "1.0.0", Digest: rawDigest(schema)}
	if err := flow.ValidateSchema(flow.Registry{ref: schema}, ref, data); err != nil {
		return err
	}
	return decode(data, target)
}

func ParseSourceSnapshot(data []byte) (SourceSnapshot, error) {
	schema, err := SourceSnapshotSchema()
	if err != nil {
		return SourceSnapshot{}, err
	}
	var snapshot SourceSnapshot
	if err := parseSourceContract(data, schema, sourceSnapshotSchemaID, &snapshot); err != nil {
		return SourceSnapshot{}, err
	}
	if err := validateSourceScope(snapshot.Scope); err != nil {
		return SourceSnapshot{}, err
	}
	return snapshot, nil
}

func ParseContextRequest(data []byte) (ContextRequest, error) {
	schema, err := ContextRequestSchema()
	if err != nil {
		return ContextRequest{}, err
	}
	var request ContextRequest
	if err := parseSourceContract(data, schema, contextRequestSchemaID, &request); err != nil {
		return ContextRequest{}, err
	}
	if request.Format == "json" && request.SchemaRef == nil {
		return ContextRequest{}, contextProblem("context_request_invalid", "/schema_ref", "A JSON context request requires an exact schema reference.")
	}
	media, err := artifactMediaType(request.Format, []string{request.MediaType})
	if err != nil || media != request.MediaType {
		return ContextRequest{}, contextProblem("context_request_invalid", "/media_type", "A context request requires an explicit canonical media type.")
	}
	return request, nil
}

func validateSourceScope(scope SourceScope) error {
	if len(scope.Root) > 4096 || !utf8.ValidString(scope.Root) || !filepath.IsAbs(scope.Root) || filepath.Clean(scope.Root) != scope.Root || strings.ContainsAny(scope.Root, "\\\x00") {
		return contextProblem("source_scope_invalid", "/scope/root", "Source root must be an explicit canonical absolute directory locator.")
	}
	if len(scope.Path) > 4096 || !utf8.ValidString(scope.Path) || !safeRelative(scope.Path) {
		return contextProblem("source_scope_invalid", "/scope/path", "Source path must identify one explicit file confined to its recorded root.")
	}
	return nil
}

func validateSourceMetadata(options SourceImportOptions) error {
	for _, field := range []struct {
		name, value string
		limit       int
	}{{"external_identity", options.ExternalIdentity, 1024}, {"external_version", options.ExternalVersion, 256}, {"external_scope", options.ExternalScope, 1024}} {
		if !utf8.ValidString(field.value) || utf8.RuneCountInString(field.value) > field.limit {
			return contextProblem("source_metadata_invalid", "/"+field.name, "Declared source metadata exceeds its closed UTF-8 field contract.")
		}
	}
	return nil
}

func validateSourceText(format, media string, data []byte) error {
	if format != "blob" {
		return nil
	}
	base, params, err := mime.ParseMediaType(media)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(base, "text/") {
		return nil
	}
	if charset := params["charset"]; charset != "" && !strings.EqualFold(charset, "utf-8") {
		return contextProblem("unsupported_source_encoding", "/media_type", "Local text source import supports UTF-8 only.")
	}
	if !utf8.Valid(data) {
		return contextProblem("source_content_invalid", "", "Declared text source bytes are not valid UTF-8.")
	}
	return nil
}

// ImportSource is an explicit owner acquisition of one regular file. Each call
// records a new import identity and observation; it is not a command retry.
// Readers never revisit Scope. A separately authorized new import uses the
// existing BlobStore.Put semantics, including writing identical bytes again;
// this API does not qualify or audit general storage incident repair.
func (e *Engine) ImportSource(options SourceImportOptions) (Artifact, error) {
	if e.ReadOnly {
		return Artifact{}, local.ErrReadOnly
	}
	if err := validateSourceMetadata(options); err != nil {
		return Artifact{}, err
	}
	media, err := artifactMediaType(options.Format, []string{options.MediaType})
	if err != nil {
		return Artifact{}, err
	}
	if options.Format == "json" && options.SchemaRef == nil {
		return Artifact{}, fault("source_content_invalid", "JSON source import requires an exact schema reference")
	}
	_, registry, err := e.Inventory()
	if err != nil {
		return Artifact{}, err
	}
	definitions, err := sourceBuiltinDefinitions()
	if err != nil {
		return Artifact{}, err
	}
	adapterRef, schemaRef := builtinRef(definitions, localSourceAdapterID), builtinRef(definitions, sourceSnapshotSchemaID)
	if _, present := registry[schemaRef]; !present {
		return Artifact{}, fault("unsupported_source_contract", "source snapshot schema is not installed")
	}
	if _, present := registry[adapterRef]; !present {
		return Artifact{}, fault("unsupported_source_contract", "local source adapter is not installed")
	}
	root, relative, err := e.artifactOwnerPath(options.Path)
	if err != nil {
		return Artifact{}, err
	}
	scope := SourceScope{Root: root, Path: relative}
	if err := validateSourceScope(scope); err != nil {
		return Artifact{}, err
	}
	data, err := readLocal(root, relative, MaxArtifactBytes)
	if err != nil {
		return Artifact{}, err
	}
	observed := e.clock.now()
	if err := validateSourceText(options.Format, media, data); err != nil {
		return Artifact{}, err
	}
	importID := newID("import")
	producer := map[string]any{"kind": "import", "import_id": importID, "principal_id": e.owner, "source_ref": adapterRef}
	content, err := e.putArtifact(data, options.Format, options.SchemaRef, derivedID("artifact", e.Installation.ID, importID, "content"), producer, nil, registry, media)
	if err != nil {
		return Artifact{}, err
	}
	snapshot := SourceSnapshot{
		SchemaVersion: SourceSnapshotVersion, AdapterRef: adapterRef, ContentRef: content.Ref(), Scope: scope, Observed: observed,
		ExternalIdentity: options.ExternalIdentity, ExternalVersion: options.ExternalVersion, ExternalScope: options.ExternalScope,
	}
	descriptor, err := canonical(snapshot)
	if err != nil {
		return Artifact{}, err
	}
	if _, err := ParseSourceSnapshot(descriptor); err != nil {
		return Artifact{}, err
	}
	return e.putArtifact(descriptor, "json", &schemaRef, derivedID("artifact", e.Installation.ID, importID, "snapshot"), producer, []ArtifactRef{content.Ref()}, registry)
}

// SourceSnapshot verifies the implemented local source contract and both
// immutable artifacts. No mutable registry, source directory or acquisition
// clock is consulted. A missing/corrupt accepted blob is never repaired here.
func (e *Engine) SourceSnapshot(ref ArtifactRef) (SourceSnapshot, error) {
	artifact, descriptor, err := e.Artifact(ref)
	if err != nil {
		return SourceSnapshot{}, err
	}
	definitions, err := sourceBuiltinDefinitions()
	if err != nil {
		return SourceSnapshot{}, err
	}
	adapterRef, schemaRef := builtinRef(definitions, localSourceAdapterID), builtinRef(definitions, sourceSnapshotSchemaID)
	if artifact.Format != "json" || artifact.MediaType != "application/json" || artifact.SchemaRef == nil || *artifact.SchemaRef != schemaRef || artifact.Classification != "restricted" {
		return SourceSnapshot{}, contextProblem("source_snapshot_invalid", "", "The artifact does not have the exact local source snapshot contract.")
	}
	snapshot, err := ParseSourceSnapshot(descriptor)
	if err != nil {
		return SourceSnapshot{}, err
	}
	if snapshot.AdapterRef != adapterRef {
		return SourceSnapshot{}, contextProblem("unsupported_source_adapter", "/adapter_ref", "The source adapter is not the implemented exact local import adapter.")
	}
	importID, err := e.sourceImportProducer(artifact, adapterRef)
	if err != nil {
		return SourceSnapshot{}, err
	}
	if artifact.ID != derivedID("artifact", e.Installation.ID, importID, "snapshot") || len(artifact.Provenance) != 1 || artifact.Provenance[0] != snapshot.ContentRef || snapshot.ContentRef.ArtifactID != derivedID("artifact", e.Installation.ID, importID, "content") {
		return SourceSnapshot{}, contextProblem("source_snapshot_invalid", "/content_ref", "Source snapshot identity or content provenance does not match its import.")
	}
	content, data, err := e.Artifact(snapshot.ContentRef)
	if err != nil {
		return SourceSnapshot{}, err
	}
	contentImport, err := e.sourceImportProducer(content, adapterRef)
	if err != nil {
		return SourceSnapshot{}, err
	}
	if contentImport != importID || len(content.Provenance) != 0 || content.Classification != "restricted" {
		return SourceSnapshot{}, contextProblem("source_snapshot_invalid", "/content_ref", "Source content belongs to a different import or provenance chain.")
	}
	if _, err := artifactMediaType(content.Format, []string{content.MediaType}); err != nil {
		return SourceSnapshot{}, err
	}
	if err := validateSourceText(content.Format, content.MediaType, data); err != nil {
		return SourceSnapshot{}, err
	}
	return snapshot, nil
}

func (e *Engine) sourceImportProducer(artifact Artifact, adapter flow.Ref) (string, error) {
	importID, _ := artifact.Producer["import_id"].(string)
	var source flow.Ref
	encoded, err := json.Marshal(artifact.Producer["source_ref"])
	if err == nil {
		err = json.Unmarshal(encoded, &source)
	}
	if err != nil || artifact.Producer["kind"] != "import" || artifact.Producer["principal_id"] != e.owner || importID == "" || source != adapter {
		return "", contextProblem("source_snapshot_invalid", "/producer", "The artifact is not an owner import by the exact local source adapter.")
	}
	return importID, nil
}
