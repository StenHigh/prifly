package runtime

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

const (
	maxLocalRegistryEntries = 512
	maxInventoryBytes       = 64 << 20
)

// PinnedResource preserves an explicitly typed context dependency. Bytes are
// canonical JSON or exact UTF-8 text, never an executable definition. RawDigest
// additionally identifies the authoring file, including JSON/YAML formatting.
type PinnedResource struct {
	Ref          flow.Ref `json:"ref"`
	RawDigest    string   `json:"raw_digest"`
	ByteEncoding string   `json:"byte_encoding"`
	MediaType    string   `json:"media_type"`
	Bytes        []byte   `json:"bytes"`
}

// localRegistry validates the version before accepting any new representation
// fields. A shared Go struct must not make explicit null valid in old contracts.
func (e *Engine) localRegistry() (RegistryFile, error) {
	b, err := readLocal(e.Root, e.Config.Configuration.RegistryFile, MaxDefinitionBytes)
	if err != nil {
		return RegistryFile{}, err
	}
	var file RegistryFile
	if err := decode(b, &file); err != nil {
		return RegistryFile{}, err
	}
	if file.SchemaVersion != "1" && file.SchemaVersion != "2" && file.SchemaVersion != "3" || file.Entries == nil || len(file.Entries) > maxLocalRegistryEntries || len(file.Aliases) > 512 || file.SchemaVersion == "1" && file.Aliases != nil {
		return RegistryFile{}, errors.New("unsupported or oversized local registry")
	}
	var wire struct {
		Entries []map[string]json.RawMessage `json:"entries"`
	}
	if err := json.Unmarshal(b, &wire); err != nil {
		return RegistryFile{}, err
	}
	seen := make(map[string]bool, len(file.Entries))
	for i, entry := range file.Entries {
		_, encodingPresent := wire.Entries[i]["byte_encoding"]
		_, mediaPresent := wire.Entries[i]["media_type"]
		if encodingPresent || mediaPresent {
			if file.SchemaVersion != "3" {
				return RegistryFile{}, errors.New("older registry cannot contain context resource representation fields")
			}
			if !encodingPresent || !mediaPresent || entry.ByteEncoding == "" || entry.MediaType == "" || entry.Kind != "resource" {
				return RegistryFile{}, fault("invalid_context_resource", "resource requires both byte_encoding and media_type")
			}
			if e.Config.Configuration.SemanticsProfile != flow.CoreProfile {
				return RegistryFile{}, fault("unsupported", "typed context resources require core-workflow/1")
			}
		}
		refBytes, err := canonical(entry.Ref)
		if err != nil {
			return RegistryFile{}, err
		}
		if err := flow.ValidateProtocol("ImmutableRef", refBytes); err != nil {
			return RegistryFile{}, err
		}
		if strings.HasPrefix(entry.Ref.ID, "core:") {
			return RegistryFile{}, errors.New("local registry cannot replace core contracts")
		}
		switch entry.Kind {
		case "step", "schema", "workflow", "policy", "adapter", "resource":
		case "check", "tool":
			if file.SchemaVersion != "3" || e.Config.Configuration.SemanticsProfile != flow.CoreProfile {
				return RegistryFile{}, fault("unsupported", "checks and tools require Registry3 and core-workflow/1")
			}
		default:
			return RegistryFile{}, errors.New("unsupported registry kind")
		}
		if !safeRelative(entry.Path) {
			return RegistryFile{}, local.ErrUnsafePath
		}
		key := entry.Ref.ID + "@" + entry.Ref.Version
		if seen[key] {
			return RegistryFile{}, errors.New("duplicate registry identity")
		}
		seen[key] = true
	}
	return file, nil
}

// inventoryResources keeps JSON definitions and explicitly typed context data
// separate. The same entry and byte budgets cover both local collections.
func (e *Engine) inventoryResources() ([]PinnedDefinition, flow.Registry, []PinnedResource, error) {
	defs, registry, err := Builtins()
	if err != nil {
		return nil, nil, nil, err
	}
	file, err := e.localRegistry()
	if err != nil {
		return nil, nil, nil, err
	}
	resources := []PinnedResource{}
	var sourceBytes, pinnedBytes int
	entries := append(append([]Definition{}, file.Entries...), e.packageEntries()...)
	if len(entries) > maxLocalRegistryEntries {
		return nil, nil, nil, fault("dependency_limit", "local and package definitions exceed 512 entries")
	}
	identities := make(map[string]bool, len(entries))
	for _, entry := range entries {
		key := entry.Ref.ID + "@" + entry.Ref.Version
		if identities[key] {
			return nil, nil, nil, errors.New("duplicate registry identity")
		}
		identities[key] = true
	}
	for _, entry := range entries {
		raw, err := readLocal(e.Root, entry.Path, MaxDefinitionBytes)
		if err != nil {
			return nil, nil, nil, err
		}
		sourceBytes += len(raw)
		if sourceBytes > maxInventoryBytes {
			return nil, nil, nil, fault("dependency_limit", "local registry exceeds 64 MiB source budget")
		}
		format := "json"
		if strings.HasSuffix(entry.Path, ".yaml") || strings.HasSuffix(entry.Path, ".yml") {
			format = "yaml"
		}
		if entry.ByteEncoding != "" {
			data := raw
			if entry.ByteEncoding == "json" && format == "yaml" {
				data, err = flow.JSONBytes(raw, format)
				if err != nil {
					return nil, nil, nil, err
				}
			}
			resource, err := flow.CanonicalContextResource(entry.Ref, flow.ContextResource{ByteEncoding: entry.ByteEncoding, MediaType: entry.MediaType, Bytes: data})
			if err != nil {
				return nil, nil, nil, err
			}
			pinnedBytes += len(resource.Bytes)
			if pinnedBytes > maxInventoryBytes {
				return nil, nil, nil, fault("dependency_limit", "local registry exceeds 64 MiB pinned budget")
			}
			resources = append(resources, PinnedResource{Ref: entry.Ref, RawDigest: rawDigest(raw), ByteEncoding: resource.ByteEncoding, MediaType: resource.MediaType, Bytes: resource.Bytes})
			continue
		}
		if entry.Kind == "workflow" {
			if err := flow.ValidateWorkflowConditions(raw, format); err != nil {
				return nil, nil, nil, err
			}
		}
		var data []byte
		if entry.Kind == "workflow" {
			data, err = flow.WorkflowJSONBytes(raw, format)
		} else {
			data, err = flow.JSONBytes(raw, format)
		}
		if err != nil {
			return nil, nil, nil, err
		}
		data, err = flow.Canonical(data)
		if err != nil {
			return nil, nil, nil, err
		}
		if len(data) > MaxDefinitionBytes {
			return nil, nil, nil, fault("document_too_large", "canonical definition exceeds 2 MiB")
		}
		if rawDigest(data) != entry.Ref.Digest {
			return nil, nil, nil, faultf("digest_mismatch", "%s", entry.Ref.ID)
		}
		if entry.Kind == "check" {
			check, err := flow.ParseCheckDefinition(data)
			if err != nil {
				return nil, nil, nil, err
			}
			if check.ID != entry.Ref.ID || check.Version != entry.Ref.Version {
				return nil, nil, nil, fault("ref_identity_mismatch", "CheckDefinition differs from exact reference")
			}
		}
		if entry.Kind == "tool" {
			descriptor, err := flow.ParseToolDescriptor(data)
			if err != nil {
				return nil, nil, nil, err
			}
			if descriptor.ID != entry.Ref.ID || descriptor.Version != entry.Ref.Version {
				return nil, nil, nil, fault("ref_identity_mismatch", "ToolDescriptor differs from exact reference")
			}
		}
		pinnedBytes += len(data)
		if pinnedBytes > maxInventoryBytes {
			return nil, nil, nil, fault("dependency_limit", "local registry exceeds 64 MiB pinned budget")
		}
		defs = append(defs, PinnedDefinition{Ref: entry.Ref, Kind: entry.Kind, RawDigest: rawDigest(raw), Bytes: data})
		registry[entry.Ref] = data
	}
	sort.Slice(defs, func(i, j int) bool { return defs[i].Ref.String() < defs[j].Ref.String() })
	sort.Slice(resources, func(i, j int) bool { return resources[i].Ref.String() < resources[j].Ref.String() })
	return defs, registry, resources, nil
}

// resourcesFromPins checks an immutable snapshot without repairing it. The
// returned compiler inputs own their bytes and cannot mutate the stored pins.
func resourcesFromPins(pins []PinnedResource) (flow.ContextResources, error) {
	if len(pins) > maxLocalRegistryEntries {
		return nil, fault("dependency_limit", "context resource snapshot exceeds 512 entries")
	}
	resources := make(flow.ContextResources, len(pins))
	seen := make(map[string]bool, len(pins))
	total := 0
	for _, pin := range pins {
		if pin.Bytes == nil {
			return nil, errors.New("context resource snapshot requires bytes, not null")
		}
		key := pin.Ref.ID + "@" + pin.Ref.Version
		if strings.HasPrefix(pin.Ref.ID, "core:") || seen[key] {
			return nil, errors.New("invalid context resource identity")
		}
		seen[key] = true
		digest, ok := strings.CutPrefix(pin.RawDigest, "sha256:")
		if !ok || len(digest) != 2*sha256.Size {
			return nil, errors.New("invalid context resource raw digest")
		}
		decoded, err := hex.DecodeString(digest)
		if err != nil || len(decoded) != sha256.Size || strings.ToLower(digest) != digest {
			return nil, errors.New("invalid context resource raw digest")
		}
		total += len(pin.Bytes)
		if total > maxInventoryBytes {
			return nil, fault("dependency_limit", "context resource snapshot exceeds 64 MiB")
		}
		resource, err := flow.CanonicalContextResource(pin.Ref, flow.ContextResource{ByteEncoding: pin.ByteEncoding, MediaType: pin.MediaType, Bytes: pin.Bytes})
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(resource.Bytes, pin.Bytes) {
			return nil, errors.New("context resource snapshot is not canonical")
		}
		if pin.ByteEncoding == "utf8_text" && pin.RawDigest != pin.Ref.Digest {
			return nil, errors.New("UTF-8 context resource raw digest differs from its exact bytes")
		}
		resources[pin.Ref] = resource
	}
	return resources, nil
}

// pinResources uses the existing definition identity registration. Encoding and
// media cannot change under an old id/version, even for equal content digests.
func (e *Engine) pinResources(pins []PinnedResource) error {
	if e.ReadOnly {
		return local.ErrReadOnly
	}
	if _, err := resourcesFromPins(pins); err != nil {
		return err
	}
	for _, pin := range pins {
		name := fmt.Sprintf("%x.json", sha256.Sum256([]byte(pin.Ref.ID+"@"+pin.Ref.Version)))
		relative := filepath.Join(".prifly/inventory", name)
		record, err := canonical(map[string]any{"ref": pin.Ref, "raw_digest": pin.RawDigest, "kind": "resource", "byte_encoding": pin.ByteEncoding, "media_type": pin.MediaType})
		if err != nil {
			return err
		}
		previous, err := readLocal(e.Root, relative, MaxDefinitionBytes)
		if err == nil {
			if !bytes.Equal(previous, record) {
				return faultf("definition_drift", "%s@%s resource representation changed; assign a new version", pin.Ref.ID, pin.Ref.Version)
			}
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := writeExclusive(filepath.Join(e.Root, relative), record); err != nil {
			if !errors.Is(err, os.ErrExist) {
				return err
			}
			previous, err = readLocal(e.Root, relative, MaxDefinitionBytes)
			if err != nil || !bytes.Equal(previous, record) {
				return errors.New("definition registration conflict")
			}
		}
	}
	return nil
}
