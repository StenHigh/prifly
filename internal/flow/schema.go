package flow

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"

	"github.com/cyberphone/json-canonicalization/go/src/webpki.org/jsoncanonicalizer"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed protocol.schema.json
var protocolSchema []byte

//go:embed step-definition-v2.schema.json
var stepDefinitionV2Schema []byte

//go:embed workflow-revision-v2.schema.json
var workflowRevisionV2Schema []byte

// Canonical returns RFC 8785 bytes after the stricter Pri-Fly input checks.
// JSON data uses finite doubles; exact large integers belong in typed strings.
func Canonical(data []byte) ([]byte, error) {
	if _, err := Parse(data, "json"); err != nil {
		return nil, err
	}
	// This JCS implementation accepts only an object/array document root.
	// A one-element array preserves canonicalization of every JSON value,
	// including scalar hook values and boolean JSON Schemas, without inventing
	// a second numeric/string serializer.
	wrapped := make([]byte, 0, len(data)+2)
	wrapped = append(wrapped, '[')
	wrapped = append(wrapped, data...)
	wrapped = append(wrapped, ']')
	canonical, err := jsoncanonicalizer.Transform(wrapped)
	if err != nil {
		return nil, problem("invalid_json", "", "JSON canonicalization failed")
	}
	return canonical[1 : len(canonical)-1], nil
}

// Digest hashes canonical JSON definition bytes. Artifact blobs use raw bytes
// and must be hashed separately by the artifact store.
func Digest(data []byte) (string, error) {
	canonical, err := Canonical(data)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

type noExternalSchema struct{}

func (noExternalSchema) Load(string) (any, error) {
	return nil, errors.New("external schema loading is disabled")
}

func newSchemaCompiler() *jsonschema.Compiler {
	c := jsonschema.NewCompiler()
	c.UseLoader(noExternalSchema{})
	c.DefaultDraft(jsonschema.Draft2020)
	c.AssertFormat()
	return c
}

var protocolCache struct {
	sync.Mutex
	compiled map[string]*jsonschema.Schema
}

func protocolValidator(name string) (*jsonschema.Schema, error) {
	protocolCache.Lock()
	defer protocolCache.Unlock()
	if protocolCache.compiled == nil {
		protocolCache.compiled = make(map[string]*jsonschema.Schema)
	}
	if schema := protocolCache.compiled[name]; schema != nil {
		return schema, nil
	}
	data, err := ProtocolSchema(name)
	if err != nil {
		return nil, err
	}
	value, err := Parse(data, "json")
	if err != nil {
		return nil, err
	}
	compiler := newSchemaCompiler()
	url := "urn:prifly:selected:" + name
	if err := compiler.AddResource(url, value); err != nil {
		return nil, problem("unsupported_contract", "", "embedded protocol schema is unavailable")
	}
	schema, err := compiler.Compile(url)
	if err == nil {
		protocolCache.compiled[name] = schema
	} else {
		err = problem("unsupported_contract", "", "embedded protocol schema cannot be compiled")
	}
	return schema, err
}

// ProtocolSchema returns canonical self-contained JSON Schema bytes. A baseline
// component uses the same local-$ref closure shape as the historical fixtures;
// unrelated future contracts cannot add dependencies to a running F1 contract.
func ProtocolSchema(name string) ([]byte, error) {
	if name == "PublicationSourceDefinition" {
		return PublicationSourceSchema()
	}
	if name == "PublicationSourceDefinitionV2" {
		return PublicationSourceSchemaV2()
	}
	if name == "PublicationSourceDefinitionV3" {
		return PublicationSourceSchemaV3()
	}
	if name == "PublicationSourceDefinitionV4" {
		return PublicationSourceSchemaV4()
	}
	if name == "PublicationSourceDefinitionV5" {
		return PublicationSourceSchemaV5()
	}
	if name == "PublicationSourceDefinitionV6" {
		return PublicationSourceSchemaV6()
	}
	if name == "PublicationSourceDefinitionV7" {
		return PublicationSourceSchemaV7()
	}
	if name == "PublicationSourceDefinitionV8" {
		return PublicationSourceSchemaV8()
	}
	baseline, err := Parse(protocolSchema, "json")
	if err != nil {
		return nil, err
	}
	defs := baseline.(map[string]any)["$defs"].(map[string]any)
	selected := make(map[string]any)
	root := map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "$ref": "#/$defs/" + name, "$defs": selected}
	var extension []byte
	switch name {
	case "StepDefinitionV2":
		extension = stepDefinitionV2Schema
	case "StepDefinitionV3":
		extension = stepDefinitionV2Schema
	case "StepDefinitionV4":
		extension = stepDefinitionV2Schema
	case "StepDefinitionV5":
		extension = stepDefinitionV2Schema
	case "WorkflowRevisionV2":
		extension = workflowRevisionV2Schema
	case "WorkflowRevisionV3":
		extension = workflowRevisionV2Schema
	}
	if extension != nil {
		value, err := Parse(extension, "json")
		if err != nil {
			return nil, err
		}
		root = value.(map[string]any)
		selected = root["$defs"].(map[string]any)
		if name == "StepDefinitionV3" || name == "StepDefinitionV4" || name == "StepDefinitionV5" {
			stepDefinitionV3(root)
		}
		if name == "StepDefinitionV4" || name == "StepDefinitionV5" {
			stepDefinitionV4(root)
		}
		if name == "StepDefinitionV5" {
			stepDefinitionV5(root)
		}
		if name == "WorkflowRevisionV3" {
			workflowRevisionV3(root, defs)
		}
	} else if _, exists := defs[name]; !exists {
		return nil, problem("unsupported_contract", "", "unknown protocol contract")
	}
	seen := make(map[string]bool)
	var visit func(any) error
	visit = func(value any) error {
		switch value := value.(type) {
		case map[string]any:
			if ref, ok := value["$ref"].(string); ok {
				ref = strings.TrimPrefix(ref, "urn:prifly:protocol:1")
				value["$ref"] = ref
				if !strings.HasPrefix(ref, "#/$defs/") {
					return problem("unsupported_contract", "", "protocol reference is outside the embedded closure")
				}
				definition := strings.TrimPrefix(ref, "#/$defs/")
				if !seen[definition] {
					seen[definition] = true
					if _, exists := selected[definition]; !exists {
						dependency, exists := defs[definition]
						if !exists {
							return problem("unsupported_contract", "", "missing embedded definition: "+definition)
						}
						selected[definition] = dependency
					}
					if err := visit(selected[definition]); err != nil {
						return err
					}
				}
			}
			for key, child := range value {
				if key != "$defs" {
					if err := visit(child); err != nil {
						return err
					}
				}
			}
		case []any:
			for _, child := range value {
				if err := visit(child); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := visit(root); err != nil {
		return nil, err
	}
	data, err := json.Marshal(root)
	if err != nil {
		return nil, err
	}
	return Canonical(data)
}

// WorkflowRevision v3 adds only the two publication bindings and the cursor
// input of a stream wait. Deriving it keeps the delivered v2 bytes immutable.
func workflowRevisionV3(root map[string]any, baseline map[string]any) {
	root["$id"] = "urn:prifly:workflow-revision:3"
	defs := root["$defs"].(map[string]any)
	workflow := defs["WorkflowRevisionV2"].(map[string]any)
	delete(defs, "WorkflowRevisionV2")
	defs["WorkflowRevisionV3"] = workflow
	root["$ref"] = "#/$defs/WorkflowRevisionV3"
	workflow["properties"].(map[string]any)["schema_version"].(map[string]any)["const"] = "3"

	clone := func(name string) map[string]any {
		data, _ := json.Marshal(baseline[name])
		var value map[string]any
		_ = json.Unmarshal(data, &value)
		return value
	}
	bindings := clone("InputBinding")
	variants := bindings["oneOf"].([]any)
	variants = append(variants,
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"from": map[string]any{"const": "subscription"}, "source_ref": map[string]any{"$ref": "#/$defs/ImmutableRef"},
				"port": map[string]any{"enum": []any{"handle", "cursor"}},
			},
			"required": []any{"from", "source_ref", "port"}, "additionalProperties": false,
		},
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"from": map[string]any{"const": "publication"}, "stage_id": map[string]any{"$ref": "#/$defs/Identifier"},
			},
			"required": []any{"from", "stage_id"}, "additionalProperties": false,
		},
	)
	bindings["oneOf"] = variants
	defs["InputBinding"] = bindings
	wait := clone("WaitStage")
	wait["properties"].(map[string]any)["cursor_input"] = map[string]any{"$ref": "#/$defs/InputBinding"}
	defs["WaitStage"] = wait
}

// StepDefinition v3 changes only the hook variant. Deriving it from the
// immutable embedded v2 schema keeps the already published contract byte-for-
// byte intact without maintaining a second 400-line copy.
func stepDefinitionV3(root map[string]any) {
	root["$id"] = "urn:prifly:step-definition:3"
	root["title"] = "Pri-Fly StepDefinition v3: state, event and early artifact hooks"
	properties := root["properties"].(map[string]any)
	properties["schema_version"].(map[string]any)["const"] = "3"
	defs := root["$defs"].(map[string]any)
	hook := defs["Hook"].(map[string]any)
	hookProperties := hook["properties"].(map[string]any)
	hookProperties["kind"] = map[string]any{"enum": []any{"state", "event", "artifact"}}
	hookProperties["max_payload_bytes"] = map[string]any{"type": "integer", "minimum": json.Number("1"), "maximum": json.Number("16777216")}
	hookProperties["artifact"] = map[string]any{"$ref": "#/$defs/ArtifactHook"}
	delete(hook, "allOf")
	hook["oneOf"] = []any{
		map[string]any{
			"properties": map[string]any{"kind": map[string]any{"const": "state"}},
			"required":   []any{"freshness_ms"},
			"not":        map[string]any{"required": []any{"artifact"}},
		},
		map[string]any{
			"properties": map[string]any{"kind": map[string]any{"const": "event"}},
			"not": map[string]any{"anyOf": []any{
				map[string]any{"required": []any{"freshness_ms"}},
				map[string]any{"required": []any{"artifact"}},
			}},
		},
		map[string]any{
			"properties": map[string]any{
				"kind":              map[string]any{"const": "artifact"},
				"allow_during_stop": map[string]any{"const": false},
			},
			"required": []any{"artifact"},
			"not":      map[string]any{"required": []any{"freshness_ms"}},
		},
	}
	defs["ArtifactHook"] = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"format": map[string]any{"enum": []any{"json", "blob"}},
			"media_types": map[string]any{
				"type": "array", "items": map[string]any{"type": "string", "minLength": json.Number("1"), "maxLength": json.Number("128")},
				"minItems": json.Number("1"), "maxItems": json.Number("1"), "uniqueItems": true,
			},
			"cardinality": map[string]any{"enum": []any{"one", "keyed_many"}},
			"content_check_refs": map[string]any{
				"type": "array", "items": map[string]any{"$ref": "urn:prifly:protocol:1#/$defs/ImmutableRef"},
				"minItems": json.Number("0"), "maxItems": json.Number("32"), "uniqueItems": true,
			},
			"early_consumption": map[string]any{"type": "boolean"},
		},
		"required":             []any{"format", "cardinality", "content_check_refs", "early_consumption"},
		"additionalProperties": false,
		"allOf": []any{
			map[string]any{
				"if":   map[string]any{"properties": map[string]any{"format": map[string]any{"const": "json"}}},
				"then": map[string]any{"not": map[string]any{"required": []any{"media_types"}}},
			},
			map[string]any{
				"if":   map[string]any{"properties": map[string]any{"format": map[string]any{"const": "blob"}}},
				"then": map[string]any{"required": []any{"media_types"}},
			},
		},
	}
}

// StepDefinition v4 lets an artifact-hook author opt into exact workflow
// subscriptions. Earlier contracts remain owner-only and cannot be widened by
// a workflow that merely knows the hook ref.
func stepDefinitionV4(root map[string]any) {
	root["$id"] = "urn:prifly:step-definition:4"
	root["title"] = "Pri-Fly StepDefinition v4: declared artifact subscribers"
	properties := root["properties"].(map[string]any)
	properties["schema_version"].(map[string]any)["const"] = "4"
	hook := root["$defs"].(map[string]any)["Hook"].(map[string]any)
	hookProperties := hook["properties"].(map[string]any)
	hookProperties["read_policy"] = map[string]any{"enum": []any{"owner", "declared_subscribers"}}
	variants := hook["oneOf"].([]any)
	for _, variant := range variants[:2] {
		variant.(map[string]any)["properties"].(map[string]any)["read_policy"] = map[string]any{"const": "owner"}
	}
}

// StepDefinition v5 declares a finite manifest-backed portion of a claimed
// Workspace. Prior definitions have no workspace-tree semantics.
func stepDefinitionV5(root map[string]any) {
	root["$id"] = "urn:prifly:step-definition:5"
	root["title"] = "Pri-Fly StepDefinition v5: declared workspace trees"
	properties := root["properties"].(map[string]any)
	properties["schema_version"].(map[string]any)["const"] = "5"
	properties["workspace_trees"] = map[string]any{
		"type": "array", "minItems": json.Number("1"), "maxItems": json.Number("32"),
		"items": map[string]any{"$ref": "#/$defs/WorkspaceTreeBinding"},
	}
	required := root["required"].([]any)
	root["required"] = append(required, "workspace_trees")
	defs := root["$defs"].(map[string]any)
	defs["WorkspaceTreeBinding"] = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"input_port":  map[string]any{"$ref": "urn:prifly:protocol:1#/$defs/PortName"},
			"output_port": map[string]any{"$ref": "urn:prifly:protocol:1#/$defs/PortName"},
			"capture":     map[string]any{"$ref": "#/$defs/WorkspaceTreeCapturePolicy"},
		},
		"required": []any{"output_port", "capture"}, "additionalProperties": false,
	}
	defs["WorkspaceTreeCapturePolicy"] = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"kind":       map[string]any{"enum": []any{"exact_file", "direct_child_file", "direct_child_tree"}},
			"path":       map[string]any{"$ref": "urn:prifly:protocol:1#/$defs/SafeRelativePath"},
			"entrypoint": map[string]any{"$ref": "urn:prifly:protocol:1#/$defs/SafeRelativePath"},
		},
		"required": []any{"kind", "path"}, "additionalProperties": false,
		"allOf": []any{
			map[string]any{
				"if":   map[string]any{"properties": map[string]any{"kind": map[string]any{"const": "direct_child_tree"}}},
				"then": map[string]any{"properties": map[string]any{"entrypoint": map[string]any{"const": "index.md"}}, "required": []any{"entrypoint"}},
			},
			map[string]any{
				"if":   map[string]any{"properties": map[string]any{"kind": map[string]any{"enum": []any{"exact_file", "direct_child_file"}}}},
				"then": map[string]any{"not": map[string]any{"required": []any{"entrypoint"}}},
			},
		},
	}
}

// ValidateSchema checks data before a Run exists, using the same pinned schema
// machinery used for compiled workflow ports. No file/URL resolution is allowed.
func ValidateSchema(registry Registry, ref Ref, data []byte) error {
	p := &Plan{Registry: make(Registry), schemas: make(map[Ref][]byte), schemaValues: make(map[Ref]any)}
	if err := p.pinValueRefs(map[string]any{"id": ref.ID, "version": ref.Version, "digest": ref.Digest}, registry, "/schema_ref", make(map[Ref]bool), schemaReference); err != nil {
		return err
	}
	if _, err := p.schema(ref); err != nil {
		return err
	}
	return p.ValidateJSON(ref, data)
}

// ValidateProtocol validates a named baseline DTO or an explicit extension.
// This validates shape, never execution capability, trust or authorization.
func ValidateProtocol(name string, data []byte) error {
	value, err := Parse(data, "json")
	if err != nil {
		return err
	}
	return validateProtocolValue(name, value, "")
}

func validateProtocolValue(name string, value any, path string) error {
	if err := preflightConditions(name, value, path); err != nil {
		return err
	}
	schema, err := protocolValidator(name)
	if err != nil {
		return err
	}
	return validationProblem(schema.Validate(value), path)
}

func validationProblem(err error, path string) error {
	if err == nil {
		return nil
	}
	validation, ok := err.(*jsonschema.ValidationError)
	if !ok {
		return problem("schema_invalid", path, "value does not satisfy the declared contract")
	}
	// Return one deterministic leaf; exhaustive schema errors can be very large
	// for malformed union DTOs. Values are deliberately omitted from diagnostics.
	leaves := []*jsonschema.ValidationError{}
	var collect func(*jsonschema.ValidationError)
	collect = func(e *jsonschema.ValidationError) {
		if len(e.Causes) == 0 {
			leaves = append(leaves, e)
			return
		}
		for _, child := range e.Causes {
			collect(child)
		}
	}
	collect(validation)
	location := func(e *jsonschema.ValidationError) string {
		pointer := path
		for _, part := range e.InstanceLocation {
			pointer += "/" + escapePointer(part)
		}
		return pointer
	}
	slices.SortFunc(leaves, func(a, b *jsonschema.ValidationError) int {
		return strings.Compare(location(a)+a.SchemaURL, location(b)+b.SchemaURL)
	})
	leaf := leaves[0]
	return problem("schema_invalid", location(leaf), "value does not satisfy the declared contract")
}

func decodeValue(value any, target any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	// JSON Schema accepts 1.0 as an integer. Normalize through JCS before
	// decoding integer Go fields so equivalent JSON/YAML numbers behave alike.
	data, err = Canonical(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}
