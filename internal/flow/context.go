package flow

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"mime"
	"strings"
	"unicode/utf8"
)

// ContextResource is explicitly typed context data, never an executable
// definition. JSON bytes are canonicalized; utf8_text bytes are preserved.
// MediaType describes the content, not its trust or instruction authority.
type ContextResource struct {
	ByteEncoding string
	MediaType    string
	Bytes        []byte
}

// ContextResources is an explicit dependency inventory for instruction and
// context references. CompileCore copies only the selected resource bytes.
type ContextResources map[Ref]ContextResource

// CanonicalContextResource validates the exact identity, encoding and media
// type, returning detached canonical JSON or unchanged UTF-8 text bytes.
// It does not register a resource or grant instruction authority.
func CanonicalContextResource(ref Ref, resource ContextResource) (ContextResource, error) {
	data, err := json.Marshal(ref)
	if err != nil {
		return ContextResource{}, err
	}
	if err := ValidateProtocol("ImmutableRef", data); err != nil {
		return ContextResource{}, err
	}
	return canonicalContextResource(ref, resource, "")
}

// CompileCore enables typed context dependencies for new Core executions.
// Compile and CompileProfile retain their delivered JSON-only contract.
func CompileCore(data []byte, format string, registry Registry, resources ContextResources) (*Plan, error) {
	shared := newCompilation()
	shared.availableResources = resources
	shared.resources = make(ContextResources)
	shared.expandedUses = make(map[referenceUse]bool)
	// A returned plan owns only its selected copies, not the caller's complete
	// mutable inventory (which may contain unrelated or oversized resources).
	defer func() { shared.availableResources = nil }()
	return compileWorkflow(data, format, registry, CoreProfile, shared)
}

func (p *Plan) isContextResource(ref Ref) bool {
	if p.Resources == nil {
		return false
	}
	if _, exists := p.Resources[ref]; exists {
		return true
	}
	if p.compilation != nil {
		_, exists := p.compilation.availableResources[ref]
		return exists
	}
	return false
}

func (p *Plan) pinContextResource(ref Ref, registry Registry, path string, active map[Ref]bool) error {
	if _, exists := p.Resources[ref]; exists {
		return nil
	}
	resource, exists := p.compilation.availableResources[ref]
	if !exists {
		if _, exists := registry[ref]; exists {
			return problem("resource_type_mismatch", path, "context reference requires an explicitly typed resource")
		}
		return problem("missing_ref", path, "exact context resource is not supplied: "+ref.String())
	}
	if len(p.Registry)+len(p.Resources) >= 1024 || len(active) >= MaxDepth {
		return problem("dependency_limit", path, "dependency closure exceeds 1024 documents or depth 64")
	}
	resource, err := canonicalContextResource(ref, resource, path)
	if err != nil {
		return err
	}
	if other, exists := registry[ref]; exists {
		// An explicitly typed JSON resource may also be supplied by a JSON
		// inventory, but never acquires definition semantics from that copy.
		if resource.ByteEncoding != "json" {
			return problem("resource_type_mismatch", path, "raw context resource also appears in the JSON definition inventory")
		}
		canonical, err := Canonical(other)
		if err != nil || !bytes.Equal(canonical, resource.Bytes) {
			return problem("resource_type_mismatch", path, "context resource conflicts with its JSON inventory copy")
		}
	}
	if p.compilation.dependencyBytes+len(resource.Bytes) > 64<<20 {
		return problem("dependency_limit", path, "dependency closure exceeds 64 MiB")
	}
	p.compilation.dependencyBytes += len(resource.Bytes)
	p.Resources[ref] = resource
	return nil
}

func canonicalContextResource(ref Ref, resource ContextResource, path string) (ContextResource, error) {
	if len(resource.Bytes) > MaxDocumentBytes {
		return ContextResource{}, problem("document_too_large", path, "context resource exceeds 2 MiB")
	}
	if !utf8.Valid(resource.Bytes) {
		return ContextResource{}, problem("invalid_unicode", path, "context resource must be valid UTF-8")
	}
	if resource.MediaType == "" || len(resource.MediaType) > 128 || !utf8.ValidString(resource.MediaType) || strings.ContainsAny(resource.MediaType, "\r\n\x00") {
		return ContextResource{}, problem("invalid_context_resource", path, "context resource requires an explicit valid media type")
	}
	media, parameters, err := mime.ParseMediaType(resource.MediaType)
	if err != nil || !strings.Contains(media, "/") || strings.Contains(media, "*") {
		return ContextResource{}, problem("invalid_context_resource", path, "context resource requires an explicit valid media type")
	}
	if charset, present := parameters["charset"]; present && !strings.EqualFold(charset, "utf-8") {
		return ContextResource{}, problem("invalid_context_resource", path, "context resource charset must be UTF-8")
	}
	data := resource.Bytes
	switch resource.ByteEncoding {
	case "json":
		if resource.MediaType != "application/json" {
			return ContextResource{}, problem("invalid_context_resource", path, "JSON context resource requires application/json")
		}
		data, err = Canonical(data)
		if err != nil {
			return ContextResource{}, problem("invalid_context_resource", path, "context resource is not valid bounded JSON")
		}
	case "utf8_text":
		if !strings.HasPrefix(media, "text/") {
			return ContextResource{}, problem("invalid_context_resource", path, "UTF-8 text context resource requires a text media type")
		}
	default:
		return ContextResource{}, problem("invalid_context_resource", path, "context resource encoding must be json or utf8_text")
	}
	if len(data) > MaxDocumentBytes {
		return ContextResource{}, problem("document_too_large", path, "canonical context resource exceeds 2 MiB")
	}
	if fmt.Sprintf("sha256:%x", sha256.Sum256(data)) != ref.Digest {
		return ContextResource{}, problem("digest_mismatch", path, "context resource bytes do not match exact reference")
	}
	resource.Bytes = make([]byte, len(data))
	copy(resource.Bytes, data)
	return resource, nil
}
