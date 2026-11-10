package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/stenhigh/prifly/internal/flow"
)

const projectCompileParametersFile = "compile-parameters.yaml"
const projectCompileValuesFile = "project/compile-values.yaml"

// These source inputs belong to the compiler, never to mutable Run state.
// A separate marker lets existing folder/extension contracts keep their bytes.
type projectCompileParameter struct {
	Type     string                           `json:"type"`
	Minimum  json.Number                      `json:"minimum"`
	Maximum  json.Number                      `json:"maximum"`
	Default  json.Number                      `json:"default"`
	Bindings []projectCompileParameterBinding `json:"bindings"`
}
type projectCompileParameterBinding struct {
	Schema  string `json:"schema"`
	Pointer string `json:"pointer"`
}

func projectCompileSidecar(root, folder, name, marker, field string, snapshots map[string][]byte) (map[string]any, string, error) {
	path := filepath.Join(folder, filepath.FromSlash(name))
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return nil, "", nil
	} else if err != nil {
		return nil, "", err
	}
	relative, err := projectRelativePath(root, path)
	if err != nil {
		return nil, "", err
	}
	data, err := projectExecutionSource(root, root, relative)
	if err != nil {
		return nil, "", refusal("project_compile_parameter_invalid", relative+": expected a confined regular source")
	}
	snapshots[relative] = data
	value, err := flow.Parse(data, "yaml")
	if err != nil {
		return nil, "", refusal("project_compile_parameter_invalid", relative+": "+err.Error())
	}
	object, ok := value.(map[string]any)
	if !ok || len(object) != 2 || object["authoring"] != marker {
		return nil, "", refusal("project_compile_parameter_invalid", relative+": requires authoring "+marker+" and "+field)
	}
	entries, ok := object[field].(map[string]any)
	if !ok || len(entries) > 256 {
		return nil, "", refusal("project_compile_parameter_invalid", relative+": "+field+" requires an object with at most 256 entries")
	}
	return entries, relative, nil
}

func projectCompileParameterInteger(value any) (int64, bool) {
	number, ok := value.(json.Number)
	if !ok {
		return 0, false
	}
	return flow.ParseSafeInteger(number.String())
}

func projectReadCompileParameters(root string, source *projectPackageSource) error {
	snapshots := map[string][]byte{}
	declared, path, err := projectCompileSidecar(root, source.Folder, projectCompileParametersFile, "prifly-compile-parameters/1", "parameters", snapshots)
	if err != nil {
		return err
	}
	owned, ownerPath, err := projectCompileSidecar(root, source.Folder, projectCompileValuesFile, "prifly-compile-values/1", "values", snapshots)
	if err != nil {
		return err
	}
	if path == "" && ownerPath == "" {
		return nil
	}
	source.CompileSources = snapshots
	profile, err := readProjectProfile(root)
	if err != nil {
		return err
	}
	if !projectNeutralProfile(profile.SchemaVersion) {
		return refusal("project_compile_profile_required", "compile parameters require Project profile /3 or newer")
	}
	for _, name := range slices.Sorted(maps.Keys(owned)) {
		if _, ok := declared[name]; !ok {
			return refusal("project_compile_parameter_unknown", ownerPath+": "+name+" is not declared by this package")
		}
	}
	if path == "" || len(declared) == 0 {
		return refusal("project_compile_parameter_invalid", "compile-parameters.yaml requires 1..256 parameters")
	}
	effective := map[string]any{}
	parameters := map[string]projectCompileParameter{}
	for _, name := range slices.Sorted(maps.Keys(declared)) {
		raw, ok := declared[name].(map[string]any)
		if !ok || !projectValueName.MatchString(name) || len(raw) != 5 || raw["type"] != "integer" {
			return refusal("project_compile_parameter_invalid", path+": "+name+" requires type integer, minimum, maximum, default and bindings")
		}
		minimum, minOK := projectCompileParameterInteger(raw["minimum"])
		maximum, maxOK := projectCompileParameterInteger(raw["maximum"])
		fallback, defaultOK := projectCompileParameterInteger(raw["default"])
		if !minOK || !maxOK || !defaultOK || minimum < 1 || maximum < minimum || fallback < minimum || fallback > maximum {
			return refusal("project_compile_parameter_invalid", path+": "+name+" requires a positive integer range and default within that range")
		}
		value := raw["default"]
		if selected, ok := owned[name]; ok {
			value = selected
		}
		number, valid := projectCompileParameterInteger(value)
		if !valid || number < minimum || number > maximum {
			return refusal("project_compile_parameter_invalid", fmt.Sprintf("%s: %s requires integer in [%d, %d]", projectCompileValuesFile, name, minimum, maximum))
		}
		for _, profileValues := range source.Profiles {
			if _, ok := profileValues[name]; ok {
				return refusal("project_compile_parameter_invalid", name+": package profile conflicts with declared compile parameter")
			}
		}
		if _, ok := source.References[name]; ok {
			return refusal("project_compile_parameter_invalid", name+": reference conflicts with declared compile parameter")
		}
		bindings, ok := raw["bindings"].([]any)
		if !ok || len(bindings) == 0 || len(bindings) > 256 {
			return refusal("project_compile_parameter_binding", name+": requires 1..256 schema bindings")
		}
		parameter := projectCompileParameter{Type: "integer", Minimum: raw["minimum"].(json.Number), Maximum: raw["maximum"].(json.Number), Default: raw["default"].(json.Number)}
		for _, binding := range bindings {
			object, ok := binding.(map[string]any)
			if !ok || len(object) != 2 {
				return refusal("project_compile_parameter_binding", name+": binding requires schema and pointer")
			}
			schema, schemaOK := object["schema"].(string)
			pointer, pointerOK := object["pointer"].(string)
			if !schemaOK || !pointerOK || !projectValueName.MatchString(schema) {
				return refusal("project_compile_parameter_binding", name+": binding requires a local schema alias and JSON Pointer")
			}
			parameter.Bindings = append(parameter.Bindings, projectCompileParameterBinding{Schema: schema, Pointer: pointer})
		}
		parameters[name] = parameter
		effective[name] = value
	}
	// Freeze source documents once. Composition records their original bytes,
	// then consumes these rendered values before it rewrites local aliases.
	documents := map[string]int{}
	for i := range source.Documents {
		doc := &source.Documents[i]
		if doc.Kind == "context" && filepath.Ext(doc.Source) != ".yaml" && filepath.Ext(doc.Source) != ".yml" {
			continue
		}
		data, err := projectExecutionSource(root, root, doc.Source)
		if err != nil {
			return err
		}
		snapshots[doc.Source] = data
		value, err := flow.Parse(data, "yaml")
		if err != nil {
			return err
		}
		doc.Value = value
		if doc.Kind != "schema" {
			continue
		}
		object, ok := value.(map[string]any)
		if !ok {
			return refusal("project_compile_parameter_binding", doc.Source+": schema must be an object")
		}
		id, _ := object["id"].(string)
		alias := doc.AliasPrefix + "_" + projectExtensionWorkflowName(id)
		if _, ok := documents[alias]; ok {
			return refusal("project_compile_parameter_binding", alias+": ambiguous local schema")
		}
		documents[alias] = i
	}
	seen := map[string]bool{}
	for _, name := range slices.Sorted(maps.Keys(parameters)) {
		for _, binding := range parameters[name].Bindings {
			index, ok := documents[binding.Schema]
			label := name + ": " + binding.Schema + binding.Pointer
			if !ok {
				return refusal("project_compile_parameter_binding", label+": target must be an owned schema")
			}
			key := binding.Schema + "#" + binding.Pointer
			if seen[key] {
				return refusal("project_compile_parameter_binding", label+": repeated target")
			}
			seen[key] = true
			parent, keyword, err := projectCompileParameterTarget(source.Documents[index].Value, binding.Pointer)
			if err != nil {
				return refusal("project_compile_parameter_binding", label+": "+err.Error())
			}
			if parent["type"] != "string" || keyword != "maxLength" || parent[keyword] != "{{"+name+"}}" {
				return refusal("project_compile_parameter_binding", label+": expected maxLength placeholder of a string schema")
			}
			rendered, _, err := projectSubstituteValue(parent[keyword], map[string]any{name: effective[name]})
			if err != nil {
				return err
			}
			parent[keyword] = rendered
		}
	}
	// A declared parameter cannot also act as an unrestricted template value.
	for _, doc := range source.Documents {
		_, missing, err := projectSubstituteValue(doc.Value, map[string]any{})
		if err != nil {
			return err
		}
		for _, name := range missing {
			if _, ok := parameters[name]; ok {
				return refusal("project_compile_parameter_binding", doc.Source+": "+name+" occurs outside its declared bindings")
			}
		}
	}
	source.CompileParameters = map[string]any{path: map[string]any{"parameters": declared, "values": effective}}
	for name := range parameters {
		if source.CompileParameterNames == nil {
			source.CompileParameterNames = map[string]bool{}
		}
		source.CompileParameterNames[name] = true
	}
	source.ReadPaths = append(source.ReadPaths, path)
	if ownerPath != "" {
		source.ReadPaths = append(source.ReadPaths, ownerPath)
	}
	slices.Sort(source.ReadPaths)
	return nil
}

// Traverse schema-bearing keywords only. A similar object inside default,
// examples or enum is instance data, not permission to replace a schema.
func projectCompileParameterTarget(value any, pointer string) (map[string]any, string, error) {
	if !strings.HasPrefix(pointer, "/") {
		return nil, "", fmt.Errorf("expected a nonempty JSON Pointer")
	}
	parts := strings.Split(pointer[1:], "/")
	for i, encoded := range parts {
		for j := 0; j < len(encoded); j++ {
			if encoded[j] == '~' {
				if j+1 >= len(encoded) || encoded[j+1] != '0' && encoded[j+1] != '1' {
					return nil, "", fmt.Errorf("invalid JSON Pointer escape")
				}
				j++
			}
		}
		parts[i] = strings.ReplaceAll(strings.ReplaceAll(encoded, "~1", "/"), "~0", "~")
	}
	current := value
	for i := 0; i < len(parts); i++ {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, "", fmt.Errorf("pointer must traverse schema objects")
		}
		part := parts[i]
		next, ok := object[part]
		if !ok {
			return nil, "", fmt.Errorf("pointer does not exist")
		}
		if i == len(parts)-1 {
			return object, part, nil
		}
		switch part {
		case "properties", "patternProperties", "$defs", "definitions", "dependentSchemas":
			entries, ok := next.(map[string]any)
			if !ok || i+2 >= len(parts) {
				return nil, "", fmt.Errorf("pointer must select a subschema")
			}
			i++
			current, ok = entries[parts[i]]
			if !ok {
				return nil, "", fmt.Errorf("pointer does not exist")
			}
		case "allOf", "anyOf", "oneOf", "prefixItems":
			entries, ok := next.([]any)
			if !ok || i+2 >= len(parts) {
				return nil, "", fmt.Errorf("pointer must select a subschema")
			}
			i++
			index, err := strconv.Atoi(parts[i])
			if err != nil || index < 0 || index >= len(entries) || strconv.Itoa(index) != parts[i] {
				return nil, "", fmt.Errorf("pointer has invalid subschema index")
			}
			current = entries[index]
		case "additionalProperties", "unevaluatedProperties", "items", "unevaluatedItems", "contains", "propertyNames", "not", "if", "then", "else":
			current = next
		default:
			return nil, "", fmt.Errorf("pointer must traverse schema keywords, not instance data")
		}
	}
	return nil, "", fmt.Errorf("pointer does not select a keyword")
}
