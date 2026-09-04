package flow

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
)

const (
	PublicationSourceVersion              = "publication-source/1"
	PublicationStreamSourceVersion        = "publication-source/2"
	PublicationNewOnlySourceVersion       = "publication-source/3"
	PublicationNewOnlyStreamSourceVersion = "publication-source/4"
	PublicationFailureSourceVersion       = "publication-source/5"
	PublicationFailureStreamSourceVersion = "publication-source/6"
	PublicationBlobSourceVersion          = "publication-source/7"
	PublicationBlobStreamSourceVersion    = "publication-source/8"
)

// PublicationSourceDefinition is the decoded form shared by the closed once
// and each_publication source contracts. Its exact Ref names the subscription.
type PublicationSourceDefinition struct {
	SchemaVersion     string   `json:"schema_version"`
	ID                string   `json:"id"`
	Version           string   `json:"version"`
	Mode              string   `json:"mode"`
	ProducerBranchID  string   `json:"producer_branch_id"`
	ProducerStageID   string   `json:"producer_stage_id"`
	Hook              string   `json:"hook"`
	HookSchemaRef     Ref      `json:"hook_schema_ref"`
	Format            string   `json:"format,omitempty"`
	MediaTypes        []string `json:"media_types,omitempty"`
	ItemKey           string   `json:"item_key,omitempty"`
	HandleSchemaRef   *Ref     `json:"handle_schema_ref,omitempty"`
	CursorSchemaRef   *Ref     `json:"cursor_schema_ref,omitempty"`
	DeliverySchemaRef *Ref     `json:"delivery_schema_ref,omitempty"`
	Initial           string   `json:"initial"`
	ProducerFailure   string   `json:"producer_failure"`
}

// ArtifactPort is the exact value a publication source can deliver. Frozen
// source versions through /6 are JSON-only; /7 and /8 state their blob media
// type explicitly so a consumer cannot widen it by merely knowing the hook.
func (s PublicationSourceDefinition) ArtifactPort() Port {
	if s.SchemaVersion == PublicationBlobSourceVersion || s.SchemaVersion == PublicationBlobStreamSourceVersion {
		if s.Format == "blob" {
			return Port{Format: "blob", MediaTypes: slices.Clone(s.MediaTypes)}
		}
	}
	return Port{Format: "json", SchemaRef: &s.HookSchemaRef}
}

// PublicationSourceSchema is a standalone author contract. It is not added to
// an already published Run bundle because once delivery uses existing wait and
// inbox state without widening either DTO.
// sourceSchemaCache keeps the eight pinned source contracts. Each is built by
// walking the embedded baseline and rewriting a few properties; the bytes are
// fixed by the version they name, so they are built once per process.
var sourceSchemaCache = struct {
	sync.Mutex
	entries map[string][]byte
}{entries: map[string][]byte{}}

func cachedSourceSchema(id string, build func() ([]byte, error)) ([]byte, error) {
	sourceSchemaCache.Lock()
	cached, found := sourceSchemaCache.entries[id]
	sourceSchemaCache.Unlock()
	if found {
		return bytes.Clone(cached), nil
	}
	schema, err := build()
	if err != nil {
		return nil, err
	}
	sourceSchemaCache.Lock()
	sourceSchemaCache.entries[id] = schema
	sourceSchemaCache.Unlock()
	return bytes.Clone(schema), nil
}

func PublicationSourceSchema() ([]byte, error) {
	return cachedSourceSchema("1", buildPublicationSourceSchema)
}

func buildPublicationSourceSchema() ([]byte, error) {
	baseline, err := Parse(protocolSchema, "json")
	if err != nil {
		return nil, err
	}
	available := baseline.(map[string]any)["$defs"].(map[string]any)
	defs := map[string]any{}
	for _, name := range []string{"Identifier", "PortName", "Version", "ImmutableRef", "Digest"} {
		definition, exists := available[name]
		if !exists {
			return nil, problem("unsupported_contract", "", "missing embedded definition: "+name)
		}
		defs[name] = definition
	}
	ref := func(name string) map[string]any { return map[string]any{"$ref": "#/$defs/" + name} }
	document := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$id":     "urn:prifly:publication-source:1",
		"title":   "Pri-Fly once artifact publication source",
		"$defs":   defs,
		"type":    "object",
		"properties": map[string]any{
			"schema_version":     map[string]any{"const": PublicationSourceVersion},
			"id":                 ref("Identifier"),
			"version":            ref("Version"),
			"mode":               map[string]any{"const": "once"},
			"producer_branch_id": ref("Identifier"),
			"producer_stage_id":  ref("Identifier"),
			"hook":               ref("PortName"),
			"hook_schema_ref":    ref("ImmutableRef"),
			"item_key":           ref("Identifier"),
			"initial":            map[string]any{"const": "retained"},
			"producer_failure":   map[string]any{"const": "wait_until_timeout"},
		},
		"required":             []string{"schema_version", "id", "version", "mode", "producer_branch_id", "producer_stage_id", "hook", "hook_schema_ref", "item_key", "initial", "producer_failure"},
		"additionalProperties": false,
	}
	data, err := json.Marshal(document)
	if err != nil {
		return nil, err
	}
	return Canonical(data)
}

// PublicationSourceSchemaV2 describes the finite each_publication lowering.
// It is a new contract rather than a widening of the delivered once source.
func PublicationSourceSchemaV2() ([]byte, error) {
	return cachedSourceSchema("2", buildPublicationSourceSchemaV2)
}

func buildPublicationSourceSchemaV2() ([]byte, error) {
	baseline, err := Parse(protocolSchema, "json")
	if err != nil {
		return nil, err
	}
	available := baseline.(map[string]any)["$defs"].(map[string]any)
	defs := map[string]any{}
	for _, name := range []string{"Identifier", "PortName", "Version", "ImmutableRef", "Digest"} {
		definition, exists := available[name]
		if !exists {
			return nil, problem("unsupported_contract", "", "missing embedded definition: "+name)
		}
		defs[name] = definition
	}
	ref := func(name string) map[string]any { return map[string]any{"$ref": "#/$defs/" + name} }
	document := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$id":     "urn:prifly:publication-source:2",
		"title":   "Pri-Fly each-publication artifact source",
		"$defs":   defs,
		"type":    "object",
		"properties": map[string]any{
			"schema_version":      map[string]any{"const": PublicationStreamSourceVersion},
			"id":                  ref("Identifier"),
			"version":             ref("Version"),
			"mode":                map[string]any{"const": "each_publication"},
			"producer_branch_id":  ref("Identifier"),
			"producer_stage_id":   ref("Identifier"),
			"hook":                ref("PortName"),
			"hook_schema_ref":     ref("ImmutableRef"),
			"handle_schema_ref":   ref("ImmutableRef"),
			"cursor_schema_ref":   ref("ImmutableRef"),
			"delivery_schema_ref": ref("ImmutableRef"),
			"initial":             map[string]any{"const": "retained"},
			"producer_failure":    map[string]any{"const": "wait_until_timeout"},
		},
		"required":             []string{"schema_version", "id", "version", "mode", "producer_branch_id", "producer_stage_id", "hook", "hook_schema_ref", "handle_schema_ref", "cursor_schema_ref", "delivery_schema_ref", "initial", "producer_failure"},
		"additionalProperties": false,
	}
	data, err := json.Marshal(document)
	if err != nil {
		return nil, err
	}
	return Canonical(data)
}

// PublicationSourceSchemaV3 adds the explicit new_only initial position for a
// once source. The delivered retained source remains a separate frozen shape.
func PublicationSourceSchemaV3() ([]byte, error) {
	return cachedSourceSchema("3", buildPublicationSourceSchemaV3)
}

func buildPublicationSourceSchemaV3() ([]byte, error) {
	baseline, err := Parse(protocolSchema, "json")
	if err != nil {
		return nil, err
	}
	available := baseline.(map[string]any)["$defs"].(map[string]any)
	defs := map[string]any{}
	for _, name := range []string{"Identifier", "PortName", "Version", "ImmutableRef", "Digest"} {
		definition, exists := available[name]
		if !exists {
			return nil, problem("unsupported_contract", "", "missing embedded definition: "+name)
		}
		defs[name] = definition
	}
	ref := func(name string) map[string]any { return map[string]any{"$ref": "#/$defs/" + name} }
	document := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$id":     "urn:prifly:publication-source:3",
		"title":   "Pri-Fly once new-only artifact publication source",
		"$defs":   defs,
		"type":    "object",
		"properties": map[string]any{
			"schema_version":     map[string]any{"const": PublicationNewOnlySourceVersion},
			"id":                 ref("Identifier"),
			"version":            ref("Version"),
			"mode":               map[string]any{"const": "once"},
			"producer_branch_id": ref("Identifier"),
			"producer_stage_id":  ref("Identifier"),
			"hook":               ref("PortName"),
			"hook_schema_ref":    ref("ImmutableRef"),
			"item_key":           ref("Identifier"),
			"initial":            map[string]any{"const": "new_only"},
			"producer_failure":   map[string]any{"const": "wait_until_timeout"},
		},
		"required":             []string{"schema_version", "id", "version", "mode", "producer_branch_id", "producer_stage_id", "hook", "hook_schema_ref", "item_key", "initial", "producer_failure"},
		"additionalProperties": false,
	}
	data, err := json.Marshal(document)
	if err != nil {
		return nil, err
	}
	return Canonical(data)
}

// PublicationSourceSchemaV4 is the stream counterpart of v3. A separate
// version keeps retained stream history from silently becoming new-only.
func PublicationSourceSchemaV4() ([]byte, error) {
	return cachedSourceSchema("4", buildPublicationSourceSchemaV4)
}

func buildPublicationSourceSchemaV4() ([]byte, error) {
	baseline, err := Parse(protocolSchema, "json")
	if err != nil {
		return nil, err
	}
	available := baseline.(map[string]any)["$defs"].(map[string]any)
	defs := map[string]any{}
	for _, name := range []string{"Identifier", "PortName", "Version", "ImmutableRef", "Digest"} {
		definition, exists := available[name]
		if !exists {
			return nil, problem("unsupported_contract", "", "missing embedded definition: "+name)
		}
		defs[name] = definition
	}
	ref := func(name string) map[string]any { return map[string]any{"$ref": "#/$defs/" + name} }
	document := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$id":     "urn:prifly:publication-source:4",
		"title":   "Pri-Fly each-publication new-only artifact source",
		"$defs":   defs,
		"type":    "object",
		"properties": map[string]any{
			"schema_version":      map[string]any{"const": PublicationNewOnlyStreamSourceVersion},
			"id":                  ref("Identifier"),
			"version":             ref("Version"),
			"mode":                map[string]any{"const": "each_publication"},
			"producer_branch_id":  ref("Identifier"),
			"producer_stage_id":   ref("Identifier"),
			"hook":                ref("PortName"),
			"hook_schema_ref":     ref("ImmutableRef"),
			"handle_schema_ref":   ref("ImmutableRef"),
			"cursor_schema_ref":   ref("ImmutableRef"),
			"delivery_schema_ref": ref("ImmutableRef"),
			"initial":             map[string]any{"const": "new_only"},
			"producer_failure":    map[string]any{"const": "wait_until_timeout"},
		},
		"required":             []string{"schema_version", "id", "version", "mode", "producer_branch_id", "producer_stage_id", "hook", "hook_schema_ref", "handle_schema_ref", "cursor_schema_ref", "delivery_schema_ref", "initial", "producer_failure"},
		"additionalProperties": false,
	}
	data, err := json.Marshal(document)
	if err != nil {
		return nil, err
	}
	return Canonical(data)
}

// PublicationSourceSchemaV5 makes terminal producer failure an explicit once
// policy. It starts from v3 so the frozen source shapes cannot change.
func PublicationSourceSchemaV5() ([]byte, error) {
	return cachedSourceSchema("5", func() ([]byte, error) {
		return publicationFailureSourceSchema(PublicationSourceSchemaV3, PublicationFailureSourceVersion, "urn:prifly:publication-source:5", "Pri-Fly once artifact publication source with terminal-failure interruption")
	})
}

// PublicationSourceSchemaV6 is the stream counterpart of v5.
func PublicationSourceSchemaV6() ([]byte, error) {
	return cachedSourceSchema("6", func() ([]byte, error) {
		return publicationFailureSourceSchema(PublicationSourceSchemaV4, PublicationFailureStreamSourceVersion, "urn:prifly:publication-source:6", "Pri-Fly each-publication source with terminal-failure interruption")
	})
}

// PublicationSourceSchemaV7 declares the complete artifact value type. The
// older source shapes remain JSON-only and cannot silently acquire blob data.
func PublicationSourceSchemaV7() ([]byte, error) {
	return cachedSourceSchema("7", func() ([]byte, error) {
		return publicationBlobSourceSchema(PublicationSourceSchemaV5, PublicationBlobSourceVersion, "urn:prifly:publication-source:7", "Pri-Fly once JSON or blob artifact publication source")
	})
}

// PublicationSourceSchemaV8 is the bounded stream counterpart of v7.
func PublicationSourceSchemaV8() ([]byte, error) {
	return cachedSourceSchema("8", func() ([]byte, error) {
		return publicationBlobSourceSchema(PublicationSourceSchemaV6, PublicationBlobStreamSourceVersion, "urn:prifly:publication-source:8", "Pri-Fly each-publication JSON or blob artifact source")
	})
}

func publicationFailureSourceSchema(base func() ([]byte, error), version, id, title string) ([]byte, error) {
	data, err := base()
	if err != nil {
		return nil, err
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, err
	}
	properties, ok := document["properties"].(map[string]any)
	if !ok {
		return nil, problem("unsupported_contract", "", "publication source baseline has no properties")
	}
	document["$id"], document["title"] = id, title
	properties["schema_version"] = map[string]any{"const": version}
	properties["initial"] = map[string]any{"enum": []string{"retained", "new_only"}}
	properties["producer_failure"] = map[string]any{"const": "interrupt_on_terminal_failure"}
	encoded, err := json.Marshal(document)
	if err != nil {
		return nil, err
	}
	return Canonical(encoded)
}

func publicationBlobSourceSchema(base func() ([]byte, error), version, id, title string) ([]byte, error) {
	data, err := base()
	if err != nil {
		return nil, err
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, err
	}
	properties, ok := document["properties"].(map[string]any)
	if !ok {
		return nil, problem("unsupported_contract", "", "publication source baseline has no properties")
	}
	document["$id"], document["title"] = id, title
	properties["schema_version"] = map[string]any{"const": version}
	properties["producer_failure"] = map[string]any{"enum": []string{"wait_until_timeout", "interrupt_on_terminal_failure"}}
	properties["format"] = map[string]any{"enum": []string{"json", "blob"}}
	properties["media_types"] = map[string]any{"type": "array", "items": map[string]any{"type": "string", "minLength": 1, "maxLength": 128}, "minItems": 1, "maxItems": 1, "uniqueItems": true}
	document["required"] = append(document["required"].([]any), "format")
	document["allOf"] = []any{
		map[string]any{"if": map[string]any{"properties": map[string]any{"format": map[string]any{"const": "json"}}}, "then": map[string]any{"not": map[string]any{"required": []any{"media_types"}}}},
		map[string]any{"if": map[string]any{"properties": map[string]any{"format": map[string]any{"const": "blob"}}}, "then": map[string]any{"required": []any{"media_types"}}},
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return nil, err
	}
	return Canonical(encoded)
}

func publicationSourceAt(data []byte, path string) (PublicationSourceDefinition, bool, error) {
	value, err := Parse(data, "json")
	if err != nil {
		return PublicationSourceDefinition{}, false, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return PublicationSourceDefinition{}, false, nil
	}
	version, _ := object["schema_version"].(string)
	if !strings.HasPrefix(version, "publication-source/") {
		return PublicationSourceDefinition{}, false, nil
	}
	var schema []byte
	switch version {
	case PublicationSourceVersion:
		schema, err = PublicationSourceSchema()
	case PublicationStreamSourceVersion:
		schema, err = PublicationSourceSchemaV2()
	case PublicationNewOnlySourceVersion:
		schema, err = PublicationSourceSchemaV3()
	case PublicationNewOnlyStreamSourceVersion:
		schema, err = PublicationSourceSchemaV4()
	case PublicationFailureSourceVersion:
		schema, err = PublicationSourceSchemaV5()
	case PublicationFailureStreamSourceVersion:
		schema, err = PublicationSourceSchemaV6()
	case PublicationBlobSourceVersion:
		schema, err = PublicationSourceSchemaV7()
	case PublicationBlobStreamSourceVersion:
		schema, err = PublicationSourceSchemaV8()
	default:
		err = problem("unsupported_contract", "/schema_version", "publication source version is unsupported")
	}
	if err == nil {
		err = checkSchema(schema, data)
	}
	if err != nil {
		var p *Problem
		if errors.As(err, &p) {
			return PublicationSourceDefinition{}, true, problem(p.Code, path+p.Path, p.Message)
		}
		return PublicationSourceDefinition{}, true, err
	}
	var source PublicationSourceDefinition
	if err := decodeValue(value, &source); err != nil {
		return source, true, problem("invalid_publication_source", path, err.Error())
	}
	return source, true, nil
}

// PublicationSource returns a source already validated while its wait stage
// was compiled. Unknown refs are ordinary external wait sources.
func (p *Plan) PublicationSource(ref Ref) (PublicationSourceDefinition, bool) {
	source, exists := p.publicationSources[ref]
	return source, exists
}

type publicationUse struct {
	parent           *Plan
	parallelStageID  string
	consumerBranchID string
	repeatDepth      int
	path             string
}

// checkPublicationCompositions validates source coordinates only at the root,
// where a child workflow's actual use as a parallel branch is known.
func (p *Plan) checkPublicationCompositions() error {
	work := 0
	var visit func(*Plan, *publicationUse, string) error
	visit = func(current *Plan, use *publicationUse, prefix string) error {
		work += len(current.Workflow.Definition.Stages)
		if work > 1_000_000 {
			return problem("graph_validation_limit", "/definition", "publication composition proof exceeds one million work units")
		}
		for _, stageID := range keys(current.Workflow.Definition.Stages) {
			stage := current.Workflow.Definition.Stages[stageID]
			stagePath := prefix + "/definition/stages/" + escapePointer(stageID)
			if stage.Kind == "wait" {
				source, publication := current.PublicationSource(stage.SourceRef)
				if publication {
					if use == nil || source.Mode == "once" && use.repeatDepth != 0 || source.Mode == "each_publication" && use.repeatDepth != 1 {
						placement := "a direct consumer branch of parallel"
						if source.Mode == "each_publication" {
							placement = "a repeat body in a consumer branch of parallel"
						}
						return problem("unsupported_publication_source", stagePath+"/source_ref", source.Mode+" publication source requires "+placement)
					}
					parentStage := use.parent.Workflow.Definition.Stages[use.parallelStageID]
					sourcePath := stagePath + "/source_ref@" + source.ID
					if parentStage.MaxParallelism < 2 || use.parent.Workflow.Limits.MaxParallelism < 2 {
						return problem("unsupported_publication_source", use.path+"/max_parallelism", "early producer and consumer require declared parallelism of at least two")
					}
					if source.ProducerBranchID == use.consumerBranchID {
						return problem("invalid_publication_source", sourcePath+"/producer_branch_id", "producer and consumer must be distinct sibling branches")
					}
					producer := use.parent.Branches[use.parallelStageID][source.ProducerBranchID]
					if producer == nil {
						return problem("invalid_publication_source", sourcePath+"/producer_branch_id", "producer branch is not declared by this parallel stage")
					}
					producerStage, exists := producer.Workflow.Definition.Stages[source.ProducerStageID]
					if !exists || producerStage.Kind != "step" {
						return problem("invalid_publication_source", sourcePath+"/producer_stage_id", "producer stage must be a direct step in its sibling branch")
					}
					hook, exists := producer.Steps[source.ProducerStageID].Hooks[source.Hook]
					if !exists || hook.Kind != "artifact" || hook.Artifact == nil {
						return problem("invalid_publication_source", sourcePath+"/hook", "producer hook is not a declared artifact hook")
					}
					if hook.SchemaRef != source.HookSchemaRef {
						return problem("port_type_mismatch", sourcePath+"/hook_schema_ref", "source and hook require the same exact schema")
					}
					port := source.ArtifactPort()
					if port.Format != hook.Artifact.Format || !slices.Equal(port.MediaTypes, hook.Artifact.MediaTypes) {
						return problem("port_type_mismatch", sourcePath+"/format", "source and hook require the same exact artifact type")
					}
					if !hook.Artifact.EarlyConsumption {
						return problem("unsupported_publication_source", sourcePath+"/hook", "producer hook does not permit consumption before terminal success")
					}
					if hook.ReadPolicy != "declared_subscribers" {
						return problem("publication_read_forbidden", sourcePath+"/hook", "producer hook does not permit declared subscribers")
					}
					if source.Mode == "once" && hook.Artifact.Cardinality == "one" && source.ItemKey != "item" {
						return problem("invalid_publication_source", sourcePath+"/item_key", "one artifact hook uses the fixed item key item")
					}
					if source.Mode == "each_publication" {
						if hook.Artifact.Cardinality != "keyed_many" {
							return problem("invalid_publication_source", sourcePath+"/hook", "each_publication requires a keyed_many artifact hook")
						}
						for field, ref := range map[string]*Ref{"handle_schema_ref": source.HandleSchemaRef, "cursor_schema_ref": source.CursorSchemaRef, "delivery_schema_ref": source.DeliverySchemaRef} {
							if ref == nil || current.Registry[*ref] == nil {
								return problem("missing_ref", sourcePath+"/"+field, "stream transport schema is not pinned")
							}
						}
					}
				}
			}

			switch stage.Kind {
			case "call":
				if err := visit(current.Calls[stageID], nil, stagePath+"/workflow_ref@"+current.Calls[stageID].Workflow.ID); err != nil {
					return err
				}
			case "repeat":
				var bodyUse *publicationUse
				if use != nil {
					copy := *use
					copy.repeatDepth++
					bodyUse = &copy
				}
				if err := visit(current.Repeats[stageID], bodyUse, stagePath+"/body_workflow_ref@"+current.Repeats[stageID].Workflow.ID); err != nil {
					return err
				}
			case "map":
				if err := visit(current.Maps[stageID], nil, stagePath+"/body_workflow_ref@"+current.Maps[stageID].Workflow.ID); err != nil {
					return err
				}
			case "parallel":
				for i, branch := range stage.ParallelBranches {
					branchPlan := current.Branches[stageID][branch.ID]
					branchPath := fmt.Sprintf("%s/branches/%d", stagePath, i)
					if err := visit(branchPlan, &publicationUse{parent: current, parallelStageID: stageID, consumerBranchID: branch.ID, path: stagePath}, branchPath+"/workflow_ref@"+branchPlan.Workflow.ID); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	return visit(p, nil, "")
}
