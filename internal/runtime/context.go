package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime"
	"strings"
	"unicode/utf8"

	"github.com/stenhigh/prifly/internal/flow"
)

const (
	ContextProfileVersion   = "context-profile/1"
	ContextRenderingVersion = "context-rendering/1"
	MaxContextReferences    = 512
)

// FullContextManifest is the baseline ContextManifest v1. ContextManifest is
// retained as the historical local transport type; neither is an admission.
type FullContextManifest struct {
	SchemaVersion     string             `json:"schema_version"`
	ID                string             `json:"id"`
	Version           string             `json:"version"`
	Entries           []FullContextEntry `json:"entries"`
	IsolationRequired string             `json:"isolation_required"`
	MaxBytes          int64              `json:"max_bytes"`
	MaxTokens         *int64             `json:"max_tokens"`
	Truncation        string             `json:"truncation"`
	AssemblyRef       flow.Ref           `json:"assembly_ref"`
}

type FullContextEntry struct {
	SourceID       string      `json:"source_id"`
	ArtifactRef    ArtifactRef `json:"artifact_ref"`
	Role           string      `json:"role"`
	Trust          string      `json:"trust"`
	Classification string      `json:"classification"`
	Selector       *string     `json:"selector,omitempty"`
}

// ContextProfile is pinned with the executor configuration. IncludeBrief is
// an assembly choice, never permission for the renderer to find a live brief.
type ContextProfile struct {
	SchemaVersion     string   `json:"schema_version"`
	ID                string   `json:"id"`
	Version           string   `json:"version"`
	AssemblyRef       flow.Ref `json:"assembly_ref"`
	MaxBytes          int64    `json:"max_bytes"`
	MaxReferences     int64    `json:"max_references"`
	MaxTokens         *int64   `json:"max_tokens"`
	IsolationRequired string   `json:"isolation_required"`
	Truncation        string   `json:"truncation"`
	IncludeBrief      bool     `json:"include_brief"`
}

// ContextSource contains already sealed content in manifest order. Bytes are
// supplied by the caller, not read by the renderer from metadata or a path.
type ContextSource struct {
	Artifact Artifact
	Bytes    []byte
}

// ValidateContextProfile validates the bounded profile and token applicability.
// It does not qualify an adapter, tokenizer, isolation, trust or permission.
func ValidateContextProfile(profile ContextProfile, tokenized bool) error {
	if profile.SchemaVersion != ContextProfileVersion {
		return contextProblem("context_profile_invalid", "/schema_version", "Unsupported context profile version.")
	}
	if profile.MaxReferences < 0 || profile.MaxReferences > MaxContextReferences {
		return contextProblem("context_reference_limit", "/max_references", "Context reference limit must be between 0 and 512.")
	}
	manifest := FullContextManifest{
		SchemaVersion: "1", ID: profile.ID, Version: profile.Version,
		Entries: []FullContextEntry{}, IsolationRequired: profile.IsolationRequired,
		MaxBytes: profile.MaxBytes, MaxTokens: profile.MaxTokens,
		Truncation: profile.Truncation, AssemblyRef: profile.AssemblyRef,
	}
	if _, err := validateFullContextManifest(manifest); err != nil {
		return err
	}
	if tokenized && profile.MaxTokens == nil {
		return contextProblem("context_token_budget_required", "/max_tokens", "Tokenized execution requires a finite positive token budget.")
	}
	return nil
}

func validateFullContextManifest(manifest FullContextManifest) ([]byte, error) {
	data, err := canonical(manifest)
	if err != nil {
		return nil, err
	}
	if err := flow.ValidateProtocol("ContextManifest", data); err != nil {
		return nil, err
	}
	if manifest.MaxBytes > MaxArtifactBytes {
		return nil, contextProblem("context_byte_limit", "/max_bytes", "Context byte limit exceeds the local artifact bound.")
	}
	if manifest.Truncation != "reject" {
		return nil, contextProblem("unsupported_context_truncation", "/truncation", "Context transformation requires an explicit workflow step.")
	}
	return data, nil
}

// ContextSourcePath is the exact workspace locator for a manifest position.
// The caller validates 0 <= index < MaxContextReferences before materializing;
// source IDs, filenames and source bytes are never interpolated into this path.
func ContextSourcePath(index int) string { return fmt.Sprintf("context/sources/%03d", index) }

type renderedContextSource struct {
	SourceID       string      `json:"source_id"`
	ArtifactRef    ArtifactRef `json:"artifact_ref"`
	Role           string      `json:"role"`
	Trust          string      `json:"trust"`
	Classification string      `json:"classification"`
	MediaType      string      `json:"media_type"`
	SizeBytes      int64       `json:"size_bytes"`
	Representation string      `json:"representation"`
	Path           string      `json:"path,omitempty"`
}

// RenderContext produces the first mechanical JSON rendering, without I/O or
// ambient context. The caller establishes provenance, allowed sources and the
// supported assembly revision before calling it. Actual source copies plus the
// rendered JSON must fit MaxBytes; repeated entries count repeatedly. A valid
// profile does not imply a supported tokenizer or qualified fresh isolation.
func RenderContext(manifest FullContextManifest, envelope json.RawMessage, sources []ContextSource) ([]byte, error) {
	manifestBytes, err := validateFullContextManifest(manifest)
	if err != nil {
		return nil, err
	}
	if manifest.MaxTokens != nil {
		return nil, contextProblem("unsupported_tokenization", "/max_tokens", "The mechanical JSON renderer does not measure or qualify token budgets.")
	}
	if err := flow.ValidateProtocol("ExecutionEnvelope", envelope); err != nil {
		return nil, err
	}
	envelopeBytes, err := flow.Canonical(envelope)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(envelope, envelopeBytes) {
		return nil, contextProblem("context_envelope_invalid", "/envelope", "The renderer requires the exact canonical execution envelope.")
	}
	var binding struct {
		Context ArtifactRef `json:"context_manifest_ref"`
	}
	if err := json.Unmarshal(envelope, &binding); err != nil {
		return nil, err
	}
	if binding.Context.Digest != rawDigest(manifestBytes) {
		return nil, contextProblem("context_manifest_mismatch", "/context_manifest_ref", "The envelope does not identify the supplied context manifest bytes.")
	}
	return renderContext(manifest, sources, func(w *contextJSONBuffer) {
		w.literal(`"envelope":`)
		w.value(json.RawMessage(envelopeBytes))
	})
}

// RenderCheckContext renders a check's own bootstrap contract, not a Step or
// ExecutionEnvelope. Exact request bytes are preserved without compaction or
// number normalization, and the digest identifies those same stdin bytes even
// for consumers that parse and reserialize the embedded JSON object.
func RenderCheckContext(manifest FullContextManifest, request json.RawMessage, sources []ContextSource) ([]byte, error) {
	manifestBytes, err := validateFullContextManifest(manifest)
	if err != nil {
		return nil, err
	}
	if manifest.MaxTokens != nil {
		return nil, contextProblem("unsupported_tokenization", "/max_tokens", "The mechanical JSON renderer does not measure or qualify token budgets.")
	}
	parsed, err := ParseCheckRequest(request)
	if err != nil {
		return nil, err
	}
	if parsed.ContextManifestRef.Digest != rawDigest(manifestBytes) {
		return nil, contextProblem("context_manifest_mismatch", "/context_manifest_ref", "The check request does not identify the supplied context manifest bytes.")
	}
	return renderContext(manifest, sources, func(w *contextJSONBuffer) {
		w.literal(`"check_request":`)
		w.write(request)
		w.literal(`,"check_request_digest":`)
		w.value(rawDigest(request))
	})
}

// Only the two validated bootstrap variants above choose the fixed header.
// Source validation, representations and all byte accounting stay shared.
func renderContext(manifest FullContextManifest, sources []ContextSource, bootstrap func(*contextJSONBuffer)) ([]byte, error) {
	if len(sources) != len(manifest.Entries) {
		return nil, contextProblem("context_source_invalid", "/entries", "Each manifest entry requires exactly one ordered source.")
	}
	remaining := manifest.MaxBytes
	for i, source := range sources {
		if int64(len(source.Bytes)) != source.Artifact.SizeBytes {
			return nil, contextProblem("context_source_invalid", fmt.Sprintf("/entries/%d/artifact_ref", i), "Context source size does not match its immutable metadata.")
		}
		if int64(len(source.Bytes)) > remaining {
			return nil, contextProblem("context_byte_limit", "/max_bytes", "Provided context copies exceed the context byte budget.")
		}
		remaining -= int64(len(source.Bytes))
	}

	// Each JSON document remains subject to flow's 2 MiB/depth/node bounds.
	// Text is escaped incrementally below so an oversized escaped string does
	// not first allocate a complete multiple of the remaining context budget.
	values := make([]json.RawMessage, len(sources))
	records := make([]renderedContextSource, len(sources))
	for i, source := range sources {
		entry := manifest.Entries[i]
		path := fmt.Sprintf("/entries/%d", i)
		if entry.Selector != nil {
			return nil, contextProblem("unsupported_context_selector", path+"/selector", "Use an explicitly projected artifact instead of a context selector.")
		}
		metadata, err := canonical(source.Artifact)
		if err != nil {
			return nil, err
		}
		if err := flow.ValidateProtocol("ArtifactRevision", metadata); err != nil {
			return nil, contextProblem("context_source_invalid", path+"/artifact_ref", "Context source metadata violates the artifact contract.")
		}
		if entry.ArtifactRef != source.Artifact.Ref() || source.Artifact.Digest != rawDigest(source.Bytes) {
			return nil, contextProblem("context_source_invalid", path+"/artifact_ref", "Context source identity or bytes differ from the manifest reference.")
		}
		if entry.Classification != source.Artifact.Classification {
			return nil, contextProblem("context_source_invalid", path+"/classification", "Context classification differs from its immutable source metadata.")
		}
		media, params, err := mime.ParseMediaType(source.Artifact.MediaType)
		if err != nil {
			return nil, contextProblem("context_source_invalid", path+"/artifact_ref", "Context source media type is invalid.")
		}
		record := renderedContextSource{
			SourceID: entry.SourceID, ArtifactRef: entry.ArtifactRef,
			Role: entry.Role, Trust: entry.Trust, Classification: entry.Classification,
			MediaType: source.Artifact.MediaType, SizeBytes: source.Artifact.SizeBytes,
		}
		switch {
		case source.Artifact.Format == "json":
			values[i], err = flow.Canonical(source.Bytes)
			if err != nil {
				return nil, err
			}
			record.Representation = "json"
		case strings.HasPrefix(media, "text/"):
			if charset := params["charset"]; charset != "" && !strings.EqualFold(charset, "utf-8") {
				return nil, contextProblem("unsupported_context_encoding", path+"/artifact_ref", "Text context requires UTF-8 encoding.")
			}
			if !utf8.Valid(source.Bytes) {
				return nil, contextProblem("context_source_invalid", path+"/artifact_ref", "Text context must contain valid UTF-8 bytes.")
			}
			record.Representation = "utf8_text"
		default:
			record.Representation, record.Path = "file", ContextSourcePath(i)
		}
		records[i] = record
	}

	w := contextJSONBuffer{limit: remaining}
	w.literal(`{"schema_version":"` + ContextRenderingVersion + `",`)
	bootstrap(&w)
	w.literal(`,"manifest":`)
	w.value(manifest)
	w.literal(`,"sources":[`)
	for i, record := range records {
		if w.err != nil {
			break
		}
		if i > 0 {
			w.literal(",")
		}
		if record.Representation == "file" {
			w.value(record)
			continue
		}
		header, err := json.Marshal(record)
		if err != nil {
			return nil, err
		}
		w.write(header[:len(header)-1])
		w.literal(`,"value":`)
		if record.Representation == "json" {
			w.value(values[i])
		} else {
			w.text(sources[i].Bytes)
		}
		w.literal("}")
	}
	w.literal("]}")
	if w.err != nil {
		return nil, w.err
	}
	return w.buffer.Bytes(), nil
}

// This is a bounded buffer for the fixed rendering format, not a general JSON
// encoder. The standard library escapes supplied values; only the validated
// CheckRequest bootstrap is copied verbatim. Text chunks end at UTF-8 boundaries,
// preserving exactly json.Marshal's string representation.
type contextJSONBuffer struct {
	buffer bytes.Buffer
	limit  int64
	err    error
}

func (w *contextJSONBuffer) write(data []byte) {
	if w.err != nil {
		return
	}
	if int64(len(data)) > w.limit-int64(w.buffer.Len()) {
		w.err = contextProblem("context_byte_limit", "/max_bytes", "Provided context copies and rendering exceed the context byte budget.")
		return
	}
	w.buffer.Write(data)
}

func (w *contextJSONBuffer) literal(value string) { w.write([]byte(value)) }

func (w *contextJSONBuffer) value(value any) {
	if w.err != nil {
		return
	}
	data, err := json.Marshal(value)
	if err != nil {
		w.err = err
		return
	}
	w.write(data)
}

func (w *contextJSONBuffer) text(value []byte) {
	w.literal(`"`)
	for len(value) > 0 && w.err == nil {
		n := min(4096, len(value))
		for n < len(value) && !utf8.RuneStart(value[n]) {
			n--
		}
		encoded, err := json.Marshal(string(value[:n]))
		if err != nil {
			w.err = err
			return
		}
		w.write(encoded[1 : len(encoded)-1])
		value = value[n:]
	}
	w.literal(`"`)
}

func contextProblem(code, path, message string) error {
	return &flow.Problem{Code: code, Path: path, Message: message}
}
