package flow

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/cyberphone/json-canonicalization/go/src/webpki.org/jsoncanonicalizer"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
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
// ProtocolSchemaNames lists the baseline contracts ProtocolSchema answers for.
// A caller that must name an exact contract has to be able to read the set of
// names from the tool instead of guessing them.
func ProtocolSchemaNames() ([]string, error) {
	baseline, err := Parse(protocolSchema, "json")
	if err != nil {
		return nil, err
	}
	defs, ok := baseline.(map[string]any)["$defs"].(map[string]any)
	if !ok {
		return nil, problem("unsupported_contract", "", "baseline protocol schema has no definitions")
	}
	names := []string{
		"RunStartV2",
		"PackageManifestV2",
		"PublicationSourceDefinition", "PublicationSourceDefinitionV2", "PublicationSourceDefinitionV3",
		"PublicationSourceDefinitionV4", "PublicationSourceDefinitionV5", "PublicationSourceDefinitionV6",
		"PublicationSourceDefinitionV7", "PublicationSourceDefinitionV8",
		"StepDefinitionV2", "StepDefinitionV3", "StepDefinitionV4", "StepDefinitionV5", "StepDefinitionV6", "StepDefinitionV7", "StepDefinitionV8", "StepDefinitionV9", "StepDefinitionV10", "StepDefinitionV11",
		"WorkflowRevisionV2", "WorkflowRevisionV3", "WorkflowRevisionV4", "WorkflowRevisionV5", "WorkflowRevisionV6", "WorkflowRevisionV7",
	}
	for name := range defs {
		names = append(names, name)
	}
	slices.Sort(names)
	return slices.Compact(names), nil
}

// protocolSchemaCache keeps the derived contract documents. Each one is built
// from the same embedded baseline by walking its reference closure, and the
// result is a pure function of its name; rebuilding it per call was pure waste.
var protocolSchemaCache = struct {
	sync.Mutex
	entries map[string][]byte
}{entries: map[string][]byte{}}

func ProtocolSchema(name string) ([]byte, error) {
	protocolSchemaCache.Lock()
	cached, found := protocolSchemaCache.entries[name]
	protocolSchemaCache.Unlock()
	if found {
		return bytes.Clone(cached), nil
	}
	schema, err := buildProtocolSchema(name)
	if err != nil {
		return nil, err
	}
	protocolSchemaCache.Lock()
	protocolSchemaCache.entries[name] = schema
	protocolSchemaCache.Unlock()
	return bytes.Clone(schema), nil
}

func buildProtocolSchema(name string) ([]byte, error) {
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
	if name == "PackageManifestV2" {
		manifest := defs["PackageManifest"].(map[string]any)
		properties := manifest["properties"].(map[string]any)
		properties["schema_version"] = map[string]any{"const": "2"}
		kind := properties["components"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)["kind"].(map[string]any)
		kind["enum"] = append(kind["enum"].([]any), "check")
		defs[name] = manifest
	}
	if name == "RunStartV2" {
		start := defs["RunStart"].(map[string]any)
		start["properties"].(map[string]any)["schema_version"] = map[string]any{"const": "2"}
		start["required"] = slices.DeleteFunc(start["required"].([]any), func(value any) bool { return value == "brief_ref" })
		defs[name] = start
	}
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
	case "StepDefinitionV6":
		extension = stepDefinitionV2Schema
	case "StepDefinitionV7":
		extension = stepDefinitionV2Schema
	case "StepDefinitionV8":
		extension = stepDefinitionV2Schema
	case "StepDefinitionV9":
		extension = stepDefinitionV2Schema
	case "StepDefinitionV10":
		extension = stepDefinitionV2Schema
	case "StepDefinitionV11":
		extension = stepDefinitionV2Schema
	case "WorkflowRevisionV2":
		extension = workflowRevisionV2Schema
	case "WorkflowRevisionV3":
		extension = workflowRevisionV2Schema
	case "WorkflowRevisionV4":
		extension = workflowRevisionV2Schema
	case "WorkflowRevisionV5":
		extension = workflowRevisionV2Schema
	case "WorkflowRevisionV6":
		extension = workflowRevisionV2Schema
	case "WorkflowRevisionV7":
		extension = workflowRevisionV2Schema
	}
	if extension != nil {
		value, err := Parse(extension, "json")
		if err != nil {
			return nil, err
		}
		root = value.(map[string]any)
		selected = root["$defs"].(map[string]any)
		// Each step contract is the previous one plus its own change, so the
		// requested name is reached by running the mutators in order up to it.
		// A name outside this list is a base contract and runs none of them.
		stepContracts := []string{"StepDefinitionV3", "StepDefinitionV4", "StepDefinitionV5", "StepDefinitionV6", "StepDefinitionV7", "StepDefinitionV8", "StepDefinitionV9", "StepDefinitionV10", "StepDefinitionV11"}
		stepMutators := []func(){
			func() { stepDefinitionV3(root) }, func() { stepDefinitionV4(root) }, func() { stepDefinitionV5(root) },
			func() { stepDefinitionV6(root) }, func() { stepDefinitionV7(root) }, func() { stepDefinitionV8(root) },
			func() { stepDefinitionV9(root) }, func() { stepDefinitionV10(root, defs) },
			func() { stepDefinitionV11(root) },
		}
		for i := 0; i <= slices.Index(stepContracts, name); i++ {
			stepMutators[i]()
		}
		revisions := []string{"WorkflowRevisionV3", "WorkflowRevisionV4", "WorkflowRevisionV5", "WorkflowRevisionV6", "WorkflowRevisionV7"}
		revisionMutators := []func(){
			func() { workflowRevisionV3(root, defs) },
			func() { workflowRevisionV4(root, defs) },
			func() { workflowRevisionV5(root) },
			func() { workflowRevisionV6(root) },
			func() { workflowRevisionV7(root) },
		}
		for i := 0; i <= slices.Index(revisions, name); i++ {
			revisionMutators[i]()
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

// WorkflowRevision v4 adds only impossible_verdicts, and it is the version that
// carries the completeness requirement: a v4 step stage answers for every
// verdict its step can return. Older revisions cannot say "cannot happen here",
// so they keep the rule they were sealed under and are still routed at run time.
// Running the derivation on top of v3 keeps the delivered v2 and v3 bytes intact.
func workflowRevisionV4(root map[string]any, baseline map[string]any) {
	root["$id"] = "urn:prifly:workflow-revision:4"
	defs := root["$defs"].(map[string]any)
	workflow := defs["WorkflowRevisionV3"].(map[string]any)
	delete(defs, "WorkflowRevisionV3")
	defs["WorkflowRevisionV4"] = workflow
	root["$ref"] = "#/$defs/WorkflowRevisionV4"
	workflow["properties"].(map[string]any)["schema_version"].(map[string]any)["const"] = "4"

	data, _ := json.Marshal(baseline["StepStage"])
	var step map[string]any
	_ = json.Unmarshal(data, &step)
	// The published set, not the growing one. This enum was generated from
	// StepVerdicts and so moved by itself when a verdict was added: the frozen
	// contracts of revisions 4 and 5 would have started naming a verdict their
	// own documents cannot carry, and their maxItems would have loosened by
	// one without anyone deciding to.
	step["properties"].(map[string]any)["impossible_verdicts"] = impossibleVerdictsSchema(VerdictsRequiredBy(WorkflowRevisionVerdictVersion))
	defs["StepStage"] = step
}

// WorkflowRevision v5 adds one number to a step stage: how many more attempts
// it may take when one ends in a technical failure. Everything else is v4.
// impossibleVerdictsSchema is the declaration a revision offers for the set it
// answers for. A step stage must keep at least one route, so declaring the
// whole set impossible is a stage that can never be left: at most one fewer.
func impossibleVerdictsSchema(verdicts []string) map[string]any {
	values := make([]any, len(verdicts))
	for i, verdict := range verdicts {
		values[i] = verdict
	}
	return map[string]any{
		"type": "array", "items": map[string]any{"enum": values},
		"minItems": json.Number("1"), "maxItems": json.Number(strconv.Itoa(len(verdicts) - 1)), "uniqueItems": true,
	}
}

// workflowRevisionV6 widens the declaration to the verdict a step returns when
// it could not judge the work. Revisions 4 and 5 keep the set they published.
func workflowRevisionV6(root map[string]any) {
	root["$id"] = "urn:prifly:workflow-revision:6"
	defs := root["$defs"].(map[string]any)
	workflow := defs["WorkflowRevisionV5"].(map[string]any)
	delete(defs, "WorkflowRevisionV5")
	defs["WorkflowRevisionV6"] = workflow
	root["$ref"] = "#/$defs/WorkflowRevisionV6"
	workflow["properties"].(map[string]any)["schema_version"].(map[string]any)["const"] = WorkflowRevisionBlockedVersion
	step := defs["StepStage"].(map[string]any)
	properties := step["properties"].(map[string]any)
	properties["impossible_verdicts"] = impossibleVerdictsSchema(StepVerdicts)
	// A route for it as well as a declaration about it: the route map names
	// every verdict it accepts, so widening only the declaration would let an
	// author say the verdict is impossible and never say where it leads.
	routes := properties["on"].(map[string]any)["properties"].(map[string]any)
	for _, verdict := range StepVerdicts {
		if _, exists := routes[verdict]; !exists {
			routes[verdict] = map[string]any{"$ref": "#/$defs/Identifier"}
		}
	}
}

// workflowRevisionV7 lets a workflow say what it keeps and what it continues
// from: the schema of its checkpoint, the step stages reporting it, and where
// each input of a continuation comes from in the Run it continues. Everything
// else is v6, completeness included.
func workflowRevisionV7(root map[string]any) {
	root["$id"] = "urn:prifly:workflow-revision:7"
	defs := root["$defs"].(map[string]any)
	workflow := defs["WorkflowRevisionV6"].(map[string]any)
	delete(defs, "WorkflowRevisionV6")
	defs["WorkflowRevisionV7"] = workflow
	root["$ref"] = "#/$defs/WorkflowRevisionV7"
	properties := workflow["properties"].(map[string]any)
	properties["schema_version"].(map[string]any)["const"] = WorkflowRevisionContinuationVersion
	properties["checkpoint"] = map[string]any{
		"type": "object", "properties": map[string]any{"schema_ref": map[string]any{"$ref": "#/$defs/ImmutableRef"}},
		"required": []any{"schema_ref"}, "additionalProperties": false,
	}
	stageName := map[string]any{"$ref": "#/$defs/Identifier"}
	port := map[string]any{"$ref": "#/$defs/PortName"}
	verdicts := make([]any, len(StepVerdicts))
	for i, verdict := range StepVerdicts {
		verdicts[i] = verdict
	}
	source := func(required []any, fields map[string]any) map[string]any {
		return map[string]any{"type": "object", "properties": fields, "required": required, "additionalProperties": false}
	}
	properties["continuation"] = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"from_workflows": map[string]any{"type": "array", "items": stageName, "minItems": json.Number("1"), "maxItems": json.Number("64"), "uniqueItems": true},
			"from_outcomes":  map[string]any{"type": "array", "items": map[string]any{"$ref": "#/$defs/Outcome"}, "minItems": json.Number("1"), "maxItems": json.Number("5"), "uniqueItems": true},
			// A cancelled Run has no outcome, so continuing one is its own
			// statement rather than a sixth outcome that does not exist.
			"from_cancelled": map[string]any{"const": true},
			"inputs": map[string]any{
				"type": "object", "propertyNames": port, "minProperties": json.Number("1"), "maxProperties": json.Number("256"),
				"additionalProperties": map[string]any{"oneOf": []any{
					source([]any{"source_input"}, map[string]any{"source_input": port}),
					source([]any{"stage", "output", "verdict"}, map[string]any{"stage": stageName, "output": port, "verdict": map[string]any{"enum": verdicts}}),
					source([]any{"stage", "output", "outcome"}, map[string]any{"stage": stageName, "output": port, "outcome": map[string]any{"$ref": "#/$defs/Outcome"}}),
					source([]any{"checkpoint"}, map[string]any{"checkpoint": map[string]any{"const": true}}),
				}},
			},
		},
		"required": []any{"from_workflows", "inputs"}, "additionalProperties": false,
		"anyOf": []any{
			map[string]any{"required": []any{"from_outcomes"}},
			map[string]any{"required": []any{"from_cancelled"}},
		},
	}
	defs["StepStage"].(map[string]any)["properties"].(map[string]any)["checkpoint"] = port
}

func workflowRevisionV5(root map[string]any) {
	root["$id"] = "urn:prifly:workflow-revision:5"
	defs := root["$defs"].(map[string]any)
	workflow := defs["WorkflowRevisionV4"].(map[string]any)
	delete(defs, "WorkflowRevisionV4")
	defs["WorkflowRevisionV5"] = workflow
	root["$ref"] = "#/$defs/WorkflowRevisionV5"
	workflow["properties"].(map[string]any)["schema_version"].(map[string]any)["const"] = WorkflowRevisionRetryVersion
	step := defs["StepStage"].(map[string]any)
	// A bound, because an unbounded repeat of a failing program is a loop the
	// declaration cannot stop, and the Run's own step budget is spent by it.
	step["properties"].(map[string]any)["technical_retries"] = map[string]any{
		"type": "integer", "minimum": json.Number("1"), "maximum": json.Number("8"),
	}
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

// StepDefinition v6 pins separate active and decision-wait allowances. Workspace
// trees remain available, but a timed assisted step need not touch a repository.
func stepDefinitionV6(root map[string]any) {
	root["$id"] = "urn:prifly:step-definition:6"
	root["title"] = "Pri-Fly StepDefinition v6: assisted work and decision wait limits"
	properties := root["properties"].(map[string]any)
	properties["schema_version"].(map[string]any)["const"] = "6"
	properties["session_limits"] = map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []any{"active_timeout_ms", "decision_wait_timeout_ms"},
		"properties": map[string]any{
			"active_timeout_ms":        map[string]any{"type": "integer", "minimum": 1, "maximum": MaxSessionTimeoutMS},
			"decision_wait_timeout_ms": map[string]any{"type": []any{"integer", "null"}, "minimum": 1, "maximum": MaxSessionTimeoutMS},
		},
	}
	required := slices.DeleteFunc(root["required"].([]any), func(value any) bool { return value == "workspace_trees" })
	root["required"] = append(required, "session_limits")
	executor := properties["executor"].(map[string]any)["properties"].(map[string]any)
	executor["operation"] = map[string]any{"const": "session"}
	executor["adapter_ref"].(map[string]any)["properties"] = map[string]any{"id": map[string]any{"const": "core:adapter/assisted-session"}}
}

// StepDefinition v7 lets the active allowance be null, the way the decision
// wait beside it always could. v6 can only name a number, so an author who
// wanted no work deadline had to invent one large enough to stand in for "off";
// a dependent package spelled thirty days in thirteen step files. Both fields
// now say the same thing the same way, and v6 keeps the bytes it was sealed
// under: a definition that names a number is still a v6 definition.
func stepDefinitionV7(root map[string]any) {
	root["$id"] = "urn:prifly:step-definition:7"
	root["title"] = "Pri-Fly StepDefinition v7: assisted work limit may be absent"
	properties := root["properties"].(map[string]any)
	properties["schema_version"].(map[string]any)["const"] = "7"
	limits := properties["session_limits"].(map[string]any)["properties"].(map[string]any)
	limits["active_timeout_ms"] = map[string]any{"type": []any{"integer", "null"}, "minimum": 1, "maximum": MaxSessionTimeoutMS}
}

// StepDefinition v8 lets a read-only assisted step read a captured tree: a
// binding with an input port and a capture policy but no output port is
// materialized into the claimed workspace before handoff and never captured.
// The tree lives in the same workspace_trees list, since the absent output
// port already says everything a second list would; v7 and below keep
// requiring the output port, so their sealed bytes and meaning do not move.
// Session limits become optional again: a v8 step is not necessarily timed,
// and v6 had made them required only because it was the first timed contract.
func stepDefinitionV8(root map[string]any) {
	root["$id"] = "urn:prifly:step-definition:8"
	root["title"] = "Pri-Fly StepDefinition v8: materialize-only workspace tree on a read-only step"
	properties := root["properties"].(map[string]any)
	properties["schema_version"].(map[string]any)["const"] = "8"
	binding := root["$defs"].(map[string]any)["WorkspaceTreeBinding"].(map[string]any)
	binding["required"] = []any{"capture"}
	binding["anyOf"] = []any{
		map[string]any{"required": []any{"output_port"}},
		map[string]any{"required": []any{"input_port"}},
	}
	root["required"] = slices.DeleteFunc(root["required"].([]any), func(value any) bool { return value == "session_limits" })
}

// stepDefinitionV9 admits the profile of model a step wants. It is a property
// of the step, so it is sealed in the plan; what the host did with it is a
// fact of one execution and is reported, not declared here.
func stepDefinitionV9(root map[string]any) {
	root["$id"] = "urn:prifly:step-definition:9"
	root["title"] = "Pri-Fly StepDefinition v9: a step declares the model profile it wants"
	properties := root["properties"].(map[string]any)
	properties["schema_version"].(map[string]any)["const"] = "9"
	// Both fields are required together: a request without a reason reads as a
	// preference, and the next author cannot tell whether it may be dropped.
	properties["model_profile"] = map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []any{"requested", "reason"},
		"properties": map[string]any{
			"requested": map[string]any{"type": "string", "pattern": "^[a-z][a-z0-9-]{1,63}$"},
			"reason":    map[string]any{"type": "string", "minLength": 1, "maxLength": 512},
		},
	}
}

// stepDefinitionV10 lets an output be declared required for the verdict a step
// returns when it could not judge the work. Without it a gate could return that
// verdict and not hand over what it found: the binding on that edge is refused
// as unguaranteed, correctly, because the step never promised the value there.
// The reason an operator needs most would have reached only the journal.
func stepDefinitionV10(root map[string]any, baseline map[string]any) {
	root["$id"] = "urn:prifly:step-definition:10"
	root["title"] = "Pri-Fly StepDefinition v10: an output may be promised for the blocked verdict"
	properties := root["properties"].(map[string]any)
	properties["schema_version"].(map[string]any)["const"] = "10"
	// The port lives in the baseline defs at this point, the way the stage does
	// for a workflow revision: copied, widened, and placed in this contract's
	// own defs so every earlier one keeps the port it published.
	data, _ := json.Marshal(baseline["StepOutputPort"])
	var port map[string]any
	_ = json.Unmarshal(data, &port)
	required := port["properties"].(map[string]any)["required_for"].(map[string]any)
	values := required["items"].(map[string]any)["enum"].([]any)
	required["items"].(map[string]any)["enum"] = append(values, "blocked")
	required["maxItems"] = json.Number(strconv.Itoa(len(values) + 1))
	defs, ok := root["$defs"].(map[string]any)
	if !ok {
		defs = map[string]any{}
		root["$defs"] = defs
	}
	defs["StepOutputPort"] = port
}

// StepContracts are the versioned StepDefinition contracts, oldest first. The
// list the mutators run from is the same one callers ask, so a contract cannot
// be added to one and missing from the other.
var StepContracts = []string{"2", "3", "4", "5", "6", "7", "8", "9", "10"}

// StepContractFor names the protocol contract a step of this schema_version is
// validated against, or "" for a version this build does not know. Every caller
// asks here: the mapping was written out a second time in the project compile
// path, that copy stopped at 9, and a step lowered to 10 fell through to the
// base contract -- whose output port refuses the verdict the tenth exists for.
func StepContractFor(version string) string {
	if !slices.Contains(StepContracts, version) {
		return ""
	}
	return "StepDefinitionV" + version
}

// stepDefinitionV11 lets an assisted step declare that it changes an external
// system, and bound what it may change. The class was always in the published
// EffectClass enum and refused by the profile's own gates; what was missing is
// the boundary, without which the permission would be unbounded.
func stepDefinitionV11(root map[string]any) {
	root["$id"] = "urn:prifly:step-definition:11"
	root["title"] = "Pri-Fly StepDefinition v11: an assisted step declares a bounded external write"
	properties := root["properties"].(map[string]any)
	properties["schema_version"].(map[string]any)["const"] = "11"
	properties["external_write"] = map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []any{"system", "operations", "target"},
		"properties": map[string]any{
			"system": map[string]any{"type": "string", "minLength": 1, "maxLength": 128},
			// Changes only. Reading the same system is not a change, needs no
			// declaration, and its absence from this list forbids nothing.
			"operations": map[string]any{
				"type": "array", "minItems": json.Number("1"), "maxItems": json.Number("32"), "uniqueItems": true,
				"items": map[string]any{"type": "string", "minLength": 1, "maxLength": 128},
			},
			"target": map[string]any{"type": "string", "minLength": 1, "maxLength": 1024},
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

// ValidatePackageManifest selects the explicit wire version; admitting checks
// in /2 must not widen the published /1 component-kind contract.
func ValidatePackageManifest(data []byte) error {
	value, err := Parse(data, "json")
	if err != nil {
		return err
	}
	name := "PackageManifest"
	if object, ok := value.(map[string]any); ok && object["schema_version"] == "2" {
		name = "PackageManifestV2"
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
	return validationProblem(schema.Validate(value), path, name)
}

func validationProblem(err error, path, contract string) error {
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
	return problem("schema_invalid", location(leaf), "value does not satisfy the declared contract"+declaredExpectation(leaf, contract))
}

// declaredExpectation names what the contract asks for at the failing pointer.
// Only the schema's own side is rendered — never the value that was supplied,
// which is the caller's input and stays out of diagnostics. Saying that a value
// does not satisfy a contract without saying what the contract wants sends the
// reader to prifly schema and back for every field: an enum of four names cost
// the pilot three attempts to discover.
func declaredExpectation(e *jsonschema.ValidationError, contract string) string {
	render := func(value any) string {
		data, err := json.Marshal(value)
		if err != nil || len(data) > 120 {
			return ""
		}
		return string(data)
	}
	switch want := e.ErrorKind.(type) {
	case *kind.Enum:
		names := []string{}
		for _, value := range want.Want {
			text := render(value)
			if text == "" {
				return ""
			}
			names = append(names, text)
		}
		if len(names) == 0 {
			return ""
		}
		return "; the declared values are " + strings.Join(names, ", ")
	case *kind.Const:
		if text := render(want.Want); text != "" {
			return "; the declared value is " + text
		}
	case *kind.Type:
		if len(want.Want) != 0 {
			text := "; the declared type is " + strings.Join(want.Want, " or ")
			// A scalar type is the whole answer. A shape is not: naming "object"
			// tells a reader nothing about which fields it holds, and the one
			// place that answer is written is the contract itself. An author
			// who wrote prose where an object belongs had nowhere to look.
			if contract != "" && (slices.Contains(want.Want, "object") || slices.Contains(want.Want, "array")) {
				text += ", whose shape is printed by prifly schema " + contract
			}
			return text
		}
	case *kind.Required:
		if len(want.Missing) != 0 {
			return "; the contract requires " + strings.Join(want.Missing, ", ")
		}
	// A bound is the contract's side too, and the value's length or count is
	// not the value: it answers the one question left, how much to shrink or
	// add. A gate composition of 4277 characters against maxLength 4000 was
	// diagnosed by reading the package schema, not this message.
	case *kind.MaxLength:
		return fmt.Sprintf("; the contract allows at most %d characters, this value has %d", want.Want, want.Got)
	case *kind.MinLength:
		return fmt.Sprintf("; the contract requires at least %d characters, this value has %d", want.Want, want.Got)
	case *kind.MaxItems:
		return fmt.Sprintf("; the contract allows at most %d items, this value has %d", want.Want, want.Got)
	case *kind.MinItems:
		return fmt.Sprintf("; the contract requires at least %d items, this value has %d", want.Want, want.Got)
	case *kind.MaxProperties:
		return fmt.Sprintf("; the contract allows at most %d fields, this value has %d", want.Want, want.Got)
	case *kind.MinProperties:
		return fmt.Sprintf("; the contract requires at least %d fields, this value has %d", want.Want, want.Got)
	// A numeric bound names only the bound: the number that failed it is the
	// value itself, and a pattern's match is the value too.
	case *kind.Maximum:
		return "; the contract allows at most " + renderBound(want.Want)
	case *kind.Minimum:
		return "; the contract requires at least " + renderBound(want.Want)
	case *kind.ExclusiveMaximum:
		return "; the contract requires a value below " + renderBound(want.Want)
	case *kind.ExclusiveMinimum:
		return "; the contract requires a value above " + renderBound(want.Want)
	case *kind.Pattern:
		return "; the contract requires a value matching " + want.Want
	}
	return ""
}

func renderBound(bound *big.Rat) string {
	if bound == nil {
		return ""
	}
	if bound.IsInt() {
		return bound.Num().String()
	}
	value, _ := bound.Float64()
	return strconv.FormatFloat(value, 'g', -1, 64)
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
