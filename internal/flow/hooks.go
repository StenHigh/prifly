package flow

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
)

func (p *Plan) checkHooks(step StepDefinition, path string) error {
	for _, name := range keys(step.Hooks) {
		hook := step.Hooks[name]
		hookPath := path + "/hooks/" + escapePointer(name)
		if reservedName(name) {
			return problem("reserved_namespace", hookPath, "hook names cannot impersonate core or provider namespaces")
		}
		if _, err := p.schema(hook.SchemaRef); err != nil {
			return problem("invalid_schema_ref", hookPath+"/schema_ref", err.Error())
		}
		if hook.Kind == "artifact" {
			if hook.Artifact == nil {
				return problem("invalid_artifact_hook", hookPath+"/artifact", "artifact hook contract is missing")
			}
			if hook.Artifact.Cardinality == "one" && hook.MaxCount != 1 {
				return problem("invalid_artifact_hook", hookPath+"/max_count", "one artifact hook must accept exactly one logical item")
			}
			if len(hook.Artifact.ContentCheckRefs) != 0 && p.Checks == nil {
				return problem("unsupported", hookPath+"/artifact/content_check_refs", "content checks require the explicit Core compilation contract")
			}
			if err := p.loadChecks(hook.Artifact.ContentCheckRefs, "content", hookPath+"/artifact/content_check_refs"); err != nil {
				return err
			}
		}
	}
	seen := make(map[string]bool)
	for i, mapping := range step.Telemetry {
		mappingPath := fmt.Sprintf("%s/telemetry/%d", path, i)
		if reservedName(mapping.Name) || reservedName(mapping.Code) {
			return problem("reserved_namespace", mappingPath+"/name", "worker telemetry cannot impersonate core/OS/provider records")
		}
		if seen[mapping.Name] {
			return problem("ambiguous_mapping", mappingPath+"/name", "descriptor name occurs more than once")
		}
		seen[mapping.Name] = true
		hook, exists := step.Hooks[mapping.Hook]
		if !exists {
			return problem("unknown_hook", mappingPath+"/hook", "mapping references an undeclared hook")
		}
		expectedHook, expectedReset := "state", "none"
		allowedAggregations := []string{"last", "min", "max"}
		switch mapping.Kind {
		case "counter":
			expectedReset = "attempt"
			allowedAggregations = []string{"delta"}
		case "distribution":
			expectedHook = "event"
			allowedAggregations = []string{"observations"}
		case "diagnostic":
			expectedHook = "event"
			allowedAggregations = []string{"occurrences"}
		}
		if hook.Kind != expectedHook || mapping.Reset != expectedReset || !slices.Contains(allowedAggregations, mapping.Aggregation) {
			return problem("invalid_mapping", mappingPath, "instrument, hook, aggregation and reset are incompatible")
		}
		if mapping.Minimum != nil && mapping.Maximum != nil && *mapping.Minimum > *mapping.Maximum {
			return problem("invalid_mapping", mappingPath, "minimum exceeds maximum")
		}
		if mapping.Kind != "diagnostic" {
			field, err := schemaField(p.schemaValues[hook.SchemaRef], mapping.Field)
			if err != nil {
				return problem("invalid_mapping", mappingPath+"/field", err.Error())
			}
			typeName, ok := field["type"].(string)
			if !ok || (typeName != "integer" && typeName != "number") || (mapping.Kind == "counter" && typeName != "integer") {
				return problem("invalid_mapping", mappingPath+"/field", "metric field must have one numeric type; counters require integer")
			}
			if mapping.Kind == "counter" && (mapping.Minimum == nil || *mapping.Minimum < 0) {
				return problem("invalid_mapping", mappingPath+"/minimum", "cumulative counters require a nonnegative declared minimum")
			}
		}
		for _, dimension := range keys(mapping.Dimensions) {
			field, err := schemaField(p.schemaValues[hook.SchemaRef], mapping.Dimensions[dimension])
			if err != nil || !boundedDimension(field) {
				return problem("invalid_mapping", mappingPath+"/dimensions/"+escapePointer(dimension), "dimension requires a boolean or a finite enum of at most 32 short scalar values")
			}
		}
	}
	return nil
}

func reservedName(name string) bool {
	for _, prefix := range []string{"core", "prifly", "os", "provider"} {
		if name == prefix || strings.HasPrefix(name, prefix+"_") || strings.HasPrefix(name, prefix+".") {
			return true
		}
	}
	return false
}

// schemaField deliberately accepts the inspectable object-properties subset
// for purpose mappings. The artifact validator supports full JSON Schema, but
// a dynamic/conditional field schema cannot silently become a numeric meter.
func schemaField(root any, pointer string) (map[string]any, error) {
	parts, err := pointerParts(pointer)
	if err != nil {
		return nil, err
	}
	current := root
	for i := 0; ; i++ {
		object, err := resolveLocalSchema(root, current)
		if err != nil {
			return nil, err
		}
		for _, keyword := range []string{"oneOf", "anyOf", "allOf", "if", "$dynamicRef"} {
			if _, exists := object[keyword]; exists {
				return nil, fmt.Errorf("mapping field schema cannot use %s in F1", keyword)
			}
		}
		if i == len(parts) {
			return object, nil
		}
		if object["type"] != "object" {
			return nil, fmt.Errorf("field traversal requires explicitly typed object properties")
		}
		properties, _ := object["properties"].(map[string]any)
		var exists bool
		current, exists = properties[parts[i]]
		if !exists {
			return nil, fmt.Errorf("field is not declared in properties")
		}
	}
}

func resolveLocalSchema(root, current any) (map[string]any, error) {
	seen := make(map[string]bool)
	for depth := 0; depth <= MaxDepth; depth++ {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("mapping needs an explicit typed schema")
		}
		ref, exists := object["$ref"]
		if !exists {
			return object, nil
		}
		text, ok := ref.(string)
		if !ok || !strings.HasPrefix(text, "#/") || seen[text] {
			return nil, fmt.Errorf("mapping requires an acyclic local JSON Pointer schema reference")
		}
		// Sibling constraints can narrow/change the declared type. Reject such
		// mappings rather than guessing at their composed schema semantics.
		for key := range object {
			if !slices.Contains([]string{"$ref", "title", "description", "$comment"}, key) {
				return nil, fmt.Errorf("mapping schema reference has unsupported sibling constraints")
			}
		}
		seen[text] = true
		value, ok := JSONPointer(root, strings.TrimPrefix(text, "#"))
		if !ok {
			return nil, fmt.Errorf("local mapping schema reference is missing")
		}
		current = value
	}
	return nil, fmt.Errorf("mapping schema exceeds reference depth")
}

func boundedDimension(field map[string]any) bool {
	if field["type"] == "boolean" {
		return true
	}
	values, ok := field["enum"].([]any)
	if !ok || len(values) == 0 || len(values) > 32 {
		return false
	}
	for _, value := range values {
		switch value := value.(type) {
		case string:
			if len(value) > 64 || strings.ContainsAny(value, "\r\n\t\x1b") {
				return false
			}
		case bool, json.Number:
		default:
			return false
		}
	}
	return true
}

func pointerParts(pointer string) ([]string, error) {
	if pointer == "" {
		return nil, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, fmt.Errorf("expected an RFC 6901 JSON Pointer")
	}
	parts := strings.Split(pointer[1:], "/")
	for i, part := range parts {
		for j := 0; j < len(part); j++ {
			if part[j] == '~' {
				if j+1 == len(part) || (part[j+1] != '0' && part[j+1] != '1') {
					return nil, fmt.Errorf("invalid JSON Pointer escape")
				}
				j++
			}
		}
		parts[i] = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
	}
	return parts, nil
}

// JSONPointer distinguishes an absent field (false) from a present JSON null
// (nil,true). This helper reads data only; it is not a workflow projection.
func JSONPointer(value any, pointer string) (any, bool) {
	parts, err := pointerParts(pointer)
	if err != nil {
		return nil, false
	}
	for _, part := range parts {
		switch current := value.(type) {
		case map[string]any:
			var exists bool
			value, exists = current[part]
			if !exists {
				return nil, false
			}
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(current) || strconv.Itoa(index) != part {
				return nil, false
			}
			value = current[index]
		default:
			return nil, false
		}
	}
	return value, true
}

// ValidatePublication checks the declared hook and per-payload contract. Caller
// identity, CAS, event-key dedup, rate/count and terminal fencing are runtime's
// transactional responsibilities.
func (p *Plan) ValidatePublication(stageID, hookName, kind string, payload []byte) (Hook, error) {
	step, exists := p.Steps[stageID]
	if !exists {
		return Hook{}, problem("invalid_stage", "", "publisher step is not part of this plan")
	}
	hook, exists := step.Hooks[hookName]
	if !exists {
		return Hook{}, problem("unknown_hook", "/hook", "step has not declared this hook")
	}
	if hook.Kind != kind {
		return Hook{}, problem("hook_kind_mismatch", "/kind", "publication kind differs from hook declaration")
	}
	if int64(len(payload)) > hook.MaxPayloadBytes {
		return Hook{}, problem("payload_too_large", "/payload", "publication exceeds declared payload bound")
	}
	if err := p.ValidateJSON(hook.SchemaRef, payload); err != nil {
		return Hook{}, err
	}
	value, err := Parse(payload, "json")
	if err != nil {
		return Hook{}, err
	}
	for _, mapping := range step.Telemetry {
		if mapping.Hook != hookName || mapping.Kind == "diagnostic" {
			continue
		}
		measurement, exists := JSONPointer(value, mapping.Field)
		if !exists {
			continue // Full replacement can remove an optional meter; not zero.
		}
		number, ok := measurement.(json.Number)
		if !ok {
			return Hook{}, problem("invalid_measurement", "/payload"+mapping.Field, "measurement must be numeric")
		}
		n, err := number.Float64()
		if err != nil || math.IsInf(n, 0) || math.IsNaN(n) || (mapping.Kind == "counter" && math.Trunc(n) != n) {
			return Hook{}, problem("invalid_measurement", "/payload"+mapping.Field, "invalid numeric measurement")
		}
		if (mapping.Minimum != nil && n < *mapping.Minimum) || (mapping.Maximum != nil && n > *mapping.Maximum) {
			return Hook{}, problem("measurement_out_of_bounds", "/payload"+mapping.Field, "measurement exceeds descriptor bounds")
		}
	}
	return hook, nil
}

// ValidateArtifactPublication selects a declared artifact channel. Byte type,
// identity and publisher authority are checked by runtime against its sealed
// candidate; state/event payload validation stays in ValidatePublication.
func (p *Plan) ValidateArtifactPublication(stageID, hookName string) (Hook, error) {
	step, exists := p.Steps[stageID]
	if !exists {
		return Hook{}, problem("invalid_stage", "", "publisher step is not part of this plan")
	}
	hook, exists := step.Hooks[hookName]
	if !exists {
		return Hook{}, problem("unknown_hook", "/hook", "step has not declared this hook")
	}
	if hook.Kind != "artifact" || hook.Artifact == nil {
		return Hook{}, problem("hook_kind_mismatch", "/kind", "publication kind differs from hook declaration")
	}
	return hook, nil
}
