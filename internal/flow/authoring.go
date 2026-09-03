package flow

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// WorkflowAuthoringVersion identifies the concise YAML source form. It is
// lowered to an ordinary WorkflowRevision before validation, pinning or run
// admission; runtime never interprets authoring shortcuts.
const WorkflowAuthoringVersion = "prifly-workflow/1"

// StepAuthoringVersion identifies the concise YAML source form for a
// StepDefinition v2. It is lowered before schema validation and sealing.
const StepAuthoringVersion = "prifly-step/1"

// WorkflowJSONBytes returns the machine WorkflowRevision represented by JSON,
// long-form YAML or the concise YAML authoring form.
func WorkflowJSONBytes(data []byte, format string) ([]byte, error) {
	value, _, err := workflowValue(data, format)
	if err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

// StepJSONBytes returns the machine StepDefinition represented by JSON,
// long-form YAML or the concise YAML authoring form.
func StepJSONBytes(data []byte, format string) ([]byte, error) {
	value, _, err := stepValue(data, format)
	if err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

func stepValue(data []byte, format string) (any, bool, error) {
	value, err := Parse(data, format)
	if err != nil {
		return nil, false, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return value, false, nil
	}
	marker, exists := object["authoring"]
	if !exists {
		return value, false, nil
	}
	if format != "yaml" && format != "yml" {
		return nil, false, problem("unsupported_authoring", "/authoring", "concise step authoring is available only in YAML")
	}
	if marker != StepAuthoringVersion {
		return nil, false, problem("unsupported_authoring", "/authoring", "step authoring version is not supported")
	}
	lowered, err := lowerStepAuthoring(object)
	return lowered, true, err
}

func workflowValue(data []byte, format string) (any, bool, error) {
	value, err := Parse(data, format)
	if err != nil {
		return nil, false, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return value, false, nil
	}
	marker, exists := object["authoring"]
	if !exists {
		return value, false, nil
	}
	if format != "yaml" && format != "yml" {
		return nil, false, problem("unsupported_authoring", "/authoring", "concise workflow authoring is available only in YAML")
	}
	if marker != WorkflowAuthoringVersion {
		return nil, false, problem("unsupported_authoring", "/authoring", "workflow authoring version is not supported")
	}
	lowered, err := lowerWorkflowAuthoring(object)
	return lowered, true, err
}

func lowerWorkflowAuthoring(source map[string]any) (map[string]any, error) {
	allowed := []string{"authoring", "schema_version", "id", "version", "title", "refs", "inputs", "outputs", "allowed_outcomes", "entry", "stages", "limits", "policy_ref"}
	for key := range source {
		if !slices.Contains(allowed, key) {
			return nil, problem("schema_invalid", "/"+escapePointer(key), "field is not part of "+WorkflowAuthoringVersion)
		}
	}
	refs, err := authorRefs(source["refs"])
	if err != nil {
		return nil, err
	}
	inputs, err := authorPorts(source["inputs"], true, refs, "/inputs")
	if err != nil {
		return nil, err
	}
	outputs, err := authorPorts(source["outputs"], false, refs, "/outputs")
	if err != nil {
		return nil, err
	}
	stages, err := authorStages(source["stages"], refs)
	if err != nil {
		return nil, err
	}
	policy, err := authorRef(source["policy_ref"], refs, "/policy_ref")
	if err != nil {
		return nil, err
	}
	limits := cloneObject(source["limits"])
	if limits == nil {
		limits = map[string]any{}
	}
	if _, exists := limits["max_parallelism"]; !exists {
		limits["max_parallelism"] = json.Number("1")
	}
	if _, exists := limits["max_child_depth"]; !exists {
		limits["max_child_depth"] = json.Number("0")
	}
	version, _ := source["schema_version"].(string)
	if version == "" {
		version = authorSchemaVersion(inputs, stages)
	}
	title, _ := source["title"].(string)
	if title == "" {
		title, _ = source["id"].(string)
	}
	outcomes := source["allowed_outcomes"]
	if outcomes == nil {
		outcomes = authorOutcomes(stages)
	}
	return map[string]any{
		"schema_version":   version,
		"id":               source["id"],
		"version":          source["version"],
		"title":            title,
		"inputs":           inputs,
		"outputs":          outputs,
		"allowed_outcomes": outcomes,
		"definition":       map[string]any{"entry": source["entry"], "stages": stages},
		"limits":           limits,
		"policy_ref":       policy,
	}, nil
}

func lowerStepAuthoring(source map[string]any) (map[string]any, error) {
	allowed := []string{"authoring", "schema_version", "id", "version", "title", "refs", "kind", "inputs", "outputs", "executor", "instructions_ref", "context_refs", "required_capabilities", "effects", "result_check_refs", "result_schema_ref", "hooks", "telemetry", "workspace_trees"}
	for key := range source {
		if !slices.Contains(allowed, key) {
			return nil, problem("schema_invalid", "/"+escapePointer(key), "field is not part of "+StepAuthoringVersion)
		}
	}
	if version, exists := source["schema_version"]; exists && version != "2" && version != "5" {
		return nil, problem("schema_invalid", "/schema_version", StepAuthoringVersion+" lowers only to StepDefinition v2 or v5")
	}
	refs, err := authorRefs(source["refs"])
	if err != nil {
		return nil, err
	}
	inputs, err := authorPorts(source["inputs"], true, refs, "/inputs")
	if err != nil {
		return nil, err
	}
	outputs, err := authorPorts(source["outputs"], false, refs, "/outputs")
	if err != nil {
		return nil, err
	}
	contextRefs, err := authorOptionalRefList(source, "context_refs", refs)
	if err != nil {
		return nil, err
	}
	resultCheckRefs, err := authorOptionalRefList(source, "result_check_refs", refs)
	if err != nil {
		return nil, err
	}
	title, _ := source["title"].(string)
	if title == "" {
		title, _ = source["id"].(string)
	}
	schemaVersion := "2"
	if _, exists := source["workspace_trees"]; exists {
		schemaVersion = "5"
	}
	if value, exists := source["schema_version"]; exists {
		schemaVersion = value.(string)
	}
	result := map[string]any{
		"schema_version":        schemaVersion,
		"id":                    source["id"],
		"version":               source["version"],
		"title":                 title,
		"kind":                  source["kind"],
		"inputs":                inputs,
		"outputs":               outputs,
		"context_refs":          contextRefs,
		"required_capabilities": authorOptionalList(source, "required_capabilities"),
		"result_check_refs":     resultCheckRefs,
	}
	if executor := cloneObject(source["executor"]); executor != nil {
		if rawRef, exists := executor["adapter_ref"]; exists {
			ref, err := authorRef(rawRef, refs, "/executor/adapter_ref")
			if err != nil {
				return nil, err
			}
			executor["adapter_ref"] = ref
		}
		result["executor"] = executor
	} else if _, exists := source["executor"]; exists {
		result["executor"] = source["executor"]
	}
	for _, field := range []string{"instructions_ref", "result_schema_ref"} {
		if rawRef, exists := source[field]; exists {
			ref, err := authorRef(rawRef, refs, "/"+field)
			if err != nil {
				return nil, err
			}
			result[field] = ref
		}
	}
	if value, exists := source["effects"]; exists {
		result["effects"] = value
	}
	if value, exists := source["hooks"]; exists {
		hooks, err := authorStepHooks(value, refs)
		if err != nil {
			return nil, err
		}
		result["hooks"] = hooks
	}
	if value, exists := source["telemetry"]; exists {
		result["telemetry"] = value
	}
	if value, exists := source["workspace_trees"]; exists {
		result["workspace_trees"] = value
	}
	return result, nil
}

func authorOptionalRefList(source map[string]any, field string, refs map[string]any) ([]any, error) {
	value, exists := source[field]
	if !exists {
		return []any{}, nil
	}
	return authorRefList(value, refs, "/"+field)
}

func authorOptionalList(source map[string]any, field string) any {
	if value, exists := source[field]; exists {
		return value
	}
	return []any{}
}

func authorStepHooks(value any, refs map[string]any) (any, error) {
	hooks := cloneObject(value)
	if hooks == nil {
		return value, nil
	}
	for name, raw := range hooks {
		hook := cloneObject(raw)
		if hook == nil {
			continue
		}
		if rawRef, exists := hook["schema_ref"]; exists {
			ref, err := authorRef(rawRef, refs, "/hooks/"+escapePointer(name)+"/schema_ref")
			if err != nil {
				return nil, err
			}
			hook["schema_ref"] = ref
		}
		hooks[name] = hook
	}
	return hooks, nil
}

func authorRefs(value any) (map[string]any, error) {
	if value == nil {
		return map[string]any{}, nil
	}
	refs, ok := value.(map[string]any)
	if !ok || len(refs) > 1024 {
		return nil, problem("schema_invalid", "/refs", "refs must be an object with at most 1024 exact references")
	}
	for name, ref := range refs {
		if err := validateProtocolValue("ImmutableRef", ref, "/refs/"+escapePointer(name)); err != nil {
			return nil, err
		}
	}
	return refs, nil
}

func authorRef(value any, refs map[string]any, path string) (any, error) {
	name, shorthand := value.(string)
	if !shorthand {
		return value, nil
	}
	ref, exists := refs[name]
	if !exists {
		return nil, problem("unknown_ref", path, "reference alias is not declared in refs")
	}
	return ref, nil
}

func authorPorts(value any, input bool, refs map[string]any, path string) (map[string]any, error) {
	if value == nil {
		return map[string]any{}, nil
	}
	ports, ok := value.(map[string]any)
	if !ok {
		return nil, problem("schema_invalid", path, "ports must be an object")
	}
	result := make(map[string]any, len(ports))
	for name, raw := range ports {
		portPath := path + "/" + escapePointer(name)
		port := map[string]any{}
		if _, shorthand := raw.(string); shorthand {
			ref, err := authorRef(raw, refs, portPath)
			if err != nil {
				return nil, err
			}
			port["format"], port["schema_ref"] = "json", ref
		} else {
			port = cloneObject(raw)
			if port == nil {
				return nil, problem("schema_invalid", portPath, "port must be a reference alias or object")
			}
			if rawRef, exists := port["schema_ref"]; exists {
				ref, err := authorRef(rawRef, refs, portPath+"/schema_ref")
				if err != nil {
					return nil, err
				}
				port["schema_ref"] = ref
			}
			if checks, exists := port["content_check_refs"]; exists {
				normalized, err := authorRefList(checks, refs, portPath+"/content_check_refs")
				if err != nil {
					return nil, err
				}
				port["content_check_refs"] = normalized
			}
			if _, exists := port["format"]; !exists {
				switch {
				case port["schema_ref"] != nil:
					port["format"] = "json"
				case port["media_types"] != nil:
					port["format"] = "blob"
				}
			}
		}
		if input {
			if _, exists := port["required"]; !exists {
				port["required"] = true
			}
		} else if _, exists := port["required_for"]; !exists {
			port["required_for"] = []any{}
		}
		result[name] = port
	}
	return result, nil
}

func authorRefList(value any, refs map[string]any, path string) ([]any, error) {
	values, ok := value.([]any)
	if !ok {
		return nil, problem("schema_invalid", path, "reference list must be an array")
	}
	for i := range values {
		ref, err := authorRef(values[i], refs, fmt.Sprintf("%s/%d", path, i))
		if err != nil {
			return nil, err
		}
		values[i] = ref
	}
	return values, nil
}

func authorStages(value any, refs map[string]any) (map[string]any, error) {
	stages, ok := value.(map[string]any)
	if !ok {
		return nil, problem("schema_invalid", "/stages", "stages must be an object")
	}
	result := make(map[string]any, len(stages))
	for id, raw := range stages {
		path := "/stages/" + escapePointer(id)
		stage := cloneObject(raw)
		if stage == nil {
			return nil, problem("schema_invalid", path, "stage must be an object")
		}
		for _, field := range []string{"step_ref", "workflow_ref", "body_workflow_ref", "source_ref", "event_schema_ref"} {
			if rawRef, exists := stage[field]; exists {
				ref, err := authorRef(rawRef, refs, path+"/"+field)
				if err != nil {
					return nil, err
				}
				stage[field] = ref
			}
		}
		for _, field := range []string{"input_bindings", "initial_bindings", "next_bindings", "output_bindings"} {
			if bindings, exists := stage[field]; exists {
				normalized, err := authorBindings(bindings, refs, path+"/"+field)
				if err != nil {
					return nil, err
				}
				stage[field] = normalized
			}
		}
		if rawCompensation, exists := stage["compensation"]; exists {
			compensation := cloneObject(rawCompensation)
			if compensation == nil {
				return nil, problem("schema_invalid", path+"/compensation", "compensation must be an object")
			}
			ref, err := authorRef(compensation["workflow_ref"], refs, path+"/compensation/workflow_ref")
			if err != nil {
				return nil, err
			}
			compensation["workflow_ref"] = ref
			if bindings, exists := compensation["input_bindings"]; exists {
				compensation["input_bindings"], err = authorBindings(bindings, refs, path+"/compensation/input_bindings")
				if err != nil {
					return nil, err
				}
			} else {
				compensation["input_bindings"] = map[string]any{}
			}
			stage["compensation"] = compensation
		}
		kind, _ := stage["kind"].(string)
		switch kind {
		case "step", "call":
			if _, exists := stage["input_bindings"]; !exists {
				stage["input_bindings"] = map[string]any{}
			}
		case "finish":
			if _, exists := stage["output_bindings"]; !exists {
				stage["output_bindings"] = map[string]any{}
			}
		case "repeat":
			for _, field := range []string{"initial_bindings", "next_bindings"} {
				if _, exists := stage[field]; !exists {
					stage[field] = map[string]any{}
				}
			}
			if until, exists := stage["until"]; exists {
				predicate, err := authorPredicate(until, path+"/until")
				if err != nil {
					return nil, err
				}
				stage["until"] = predicate
			}
		case "map":
			if _, exists := stage["input_bindings"]; !exists {
				stage["input_bindings"] = map[string]any{}
			}
		}
		for _, field := range []string{"items", "correlation_input", "cursor_input"} {
			if binding, exists := stage[field]; exists {
				normalized, err := authorBinding(binding, refs, path+"/"+field)
				if err != nil {
					return nil, err
				}
				stage[field] = normalized
			}
		}
		if branches, exists := stage["branches"].([]any); exists {
			for i, rawBranch := range branches {
				branchPath := fmt.Sprintf("%s/branches/%d", path, i)
				branch := cloneObject(rawBranch)
				if branch == nil {
					return nil, problem("schema_invalid", branchPath, "branch must be an object")
				}
				if kind == "parallel" {
					ref, err := authorRef(branch["workflow_ref"], refs, branchPath+"/workflow_ref")
					if err != nil {
						return nil, err
					}
					branch["workflow_ref"] = ref
					if bindings, exists := branch["input_bindings"]; exists {
						branch["input_bindings"], err = authorBindings(bindings, refs, branchPath+"/input_bindings")
						if err != nil {
							return nil, err
						}
					} else {
						branch["input_bindings"] = map[string]any{}
					}
				} else if predicate, exists := branch["predicate"]; exists {
					normalized, err := authorPredicate(predicate, branchPath+"/predicate")
					if err != nil {
						return nil, err
					}
					branch["predicate"] = normalized
				}
				branches[i] = branch
			}
			stage["branches"] = branches
		}
		result[id] = stage
	}
	return result, nil
}

func authorBindings(value any, refs map[string]any, path string) (map[string]any, error) {
	bindings, ok := value.(map[string]any)
	if !ok {
		return nil, problem("schema_invalid", path, "bindings must be an object")
	}
	result := make(map[string]any, len(bindings))
	for port, raw := range bindings {
		binding, err := authorBinding(raw, refs, path+"/"+escapePointer(port))
		if err != nil {
			return nil, err
		}
		result[port] = binding
	}
	return result, nil
}

func authorBinding(value any, refs map[string]any, path string) (map[string]any, error) {
	if shorthand, ok := value.(string); ok {
		return authorSource(shorthand, refs, path)
	}
	binding := cloneObject(value)
	if binding == nil {
		return nil, problem("schema_invalid", path, "binding must be a source expression or object")
	}
	if shorthand, ok := binding["from"].(string); ok && strings.HasPrefix(shorthand, "$") {
		source, err := authorSource(shorthand, refs, path+"/from")
		if err != nil {
			return nil, err
		}
		delete(binding, "from")
		for key, child := range source {
			if _, conflict := binding[key]; conflict {
				return nil, problem("schema_invalid", path+"/"+key, "source expression already defines this field")
			}
			binding[key] = child
		}
	}
	for _, field := range []string{"schema_ref", "projected_schema_ref", "source_ref"} {
		if rawRef, exists := binding[field]; exists {
			ref, err := authorRef(rawRef, refs, path+"/"+field)
			if err != nil {
				return nil, err
			}
			binding[field] = ref
		}
	}
	return binding, nil
}

func authorSource(expression string, refs map[string]any, path string) (map[string]any, error) {
	base, pointer, hasPointer := expression, "", false
	if before, after, found := strings.Cut(expression, "#"); found {
		base, pointer, hasPointer = before, after, true
		if pointer != "" && !strings.HasPrefix(pointer, "/") {
			return nil, problem("invalid_binding", path, "source fragment must be an empty JSON Pointer or begin with /")
		}
	}
	result := map[string]any{}
	switch {
	case strings.HasPrefix(base, "$inputs."):
		result["from"], result["port"] = "workflow_input", strings.TrimPrefix(base, "$inputs.")
	case strings.HasPrefix(base, "$iteration."):
		result["from"], result["port"] = "iteration_output", strings.TrimPrefix(base, "$iteration.")
	case strings.HasPrefix(base, "$stages."):
		stageAndPort := strings.TrimPrefix(base, "$stages.")
		separator := strings.LastIndex(stageAndPort, ".")
		if separator <= 0 || separator == len(stageAndPort)-1 {
			return nil, problem("invalid_binding", path, "stage source must be $stages.STAGE.PORT")
		}
		result["from"], result["stage_id"], result["port"] = "stage_output", stageAndPort[:separator], stageAndPort[separator+1:]
	case strings.HasPrefix(base, "$publication."):
		stageID := strings.TrimPrefix(base, "$publication.")
		if stageID == "" {
			return nil, problem("invalid_binding", path, "publication source must be $publication.STAGE")
		}
		result["from"], result["stage_id"] = "publication", stageID
	case strings.HasPrefix(base, "$subscription."):
		refAndPort := strings.TrimPrefix(base, "$subscription.")
		separator := strings.LastIndex(refAndPort, ".")
		if separator <= 0 || separator == len(refAndPort)-1 {
			return nil, problem("invalid_binding", path, "subscription source must be $subscription.REF.handle or .cursor")
		}
		ref, err := authorRef(refAndPort[:separator], refs, path)
		if err != nil {
			return nil, err
		}
		result["from"], result["source_ref"], result["port"] = "subscription", ref, refAndPort[separator+1:]
	case base == "$compensation":
		result["from"] = "compensation_context"
	default:
		return nil, problem("invalid_binding", path, "unknown source expression")
	}
	if hasPointer {
		result["pointer"] = pointer
	}
	return result, nil
}

func authorPredicate(value any, path string) (any, error) {
	if constant, ok := value.(bool); ok {
		return map[string]any{"op": "eq", "left": map[string]any{"kind": "literal", "value": constant}, "right": map[string]any{"kind": "literal", "value": true}}, nil
	}
	predicate := cloneObject(value)
	if predicate == nil {
		return value, nil
	}
	op, _ := predicate["op"].(string)
	switch op {
	case "eq", "ne":
		for _, side := range []string{"left", "right"} {
			if operand, exists := predicate[side]; exists {
				normalized, err := authorOperand(operand, path+"/"+side)
				if err != nil {
					return nil, err
				}
				predicate[side] = normalized
			}
		}
	case "exists":
		if ref, exists := predicate["ref"].(string); exists {
			field, err := authorFieldRef(ref, path+"/ref")
			if err != nil {
				return nil, err
			}
			predicate["ref"] = field
		}
	case "all", "any":
		if args, exists := predicate["args"].([]any); exists {
			for i := range args {
				normalized, err := authorPredicate(args[i], fmt.Sprintf("%s/args/%d", path, i))
				if err != nil {
					return nil, err
				}
				args[i] = normalized
			}
			predicate["args"] = args
		}
	}
	return predicate, nil
}

func authorOperand(value any, path string) (any, error) {
	if expression, ok := value.(string); ok && strings.HasPrefix(expression, "$") {
		ref, err := authorFieldRef(expression, path)
		if err != nil {
			return nil, err
		}
		return map[string]any{"kind": "field", "ref": ref}, nil
	}
	if operand := cloneObject(value); operand != nil {
		if operand["kind"] == "field" {
			if expression, ok := operand["ref"].(string); ok {
				ref, err := authorFieldRef(expression, path+"/ref")
				if err != nil {
					return nil, err
				}
				operand["ref"] = ref
			}
		}
		return operand, nil
	}
	return map[string]any{"kind": "literal", "value": value}, nil
}

func authorFieldRef(expression, path string) (map[string]any, error) {
	ref, err := authorSource(expression, nil, path)
	if err != nil {
		return nil, err
	}
	if ref["from"] != "workflow_input" && ref["from"] != "stage_output" && ref["from"] != "iteration_output" {
		return nil, problem("invalid_predicate", path, "predicate field must read a workflow input, stage output or iteration output")
	}
	return ref, nil
}

func authorSchemaVersion(inputs, stages map[string]any) string {
	configured := false
	for _, raw := range inputs {
		if port := cloneObject(raw); port != nil && port["configuration"] != nil {
			configured = true
		}
	}
	for _, raw := range stages {
		stage := cloneObject(raw)
		if stage == nil {
			continue
		}
		if _, exists := stage["cursor_input"]; exists || bindingsUseV3(stage["input_bindings"]) || bindingsUseV3(stage["initial_bindings"]) || bindingsUseV3(stage["next_bindings"]) || bindingsUseV3(stage["output_bindings"]) || bindingUsesV3(stage["items"]) || bindingUsesV3(stage["correlation_input"]) {
			return "3"
		}
		if compensation := cloneObject(stage["compensation"]); compensation != nil && bindingsUseV3(compensation["input_bindings"]) {
			return "3"
		}
		branches, _ := stage["branches"].([]any)
		for _, rawBranch := range branches {
			branch, _ := rawBranch.(map[string]any)
			if bindingsUseV3(branch["input_bindings"]) {
				return "3"
			}
		}
	}
	if configured {
		return "2"
	}
	return "1"
}

func bindingsUseV3(value any) bool {
	bindings, _ := value.(map[string]any)
	for _, raw := range bindings {
		binding, _ := raw.(map[string]any)
		if binding["from"] == "subscription" || binding["from"] == "publication" {
			return true
		}
	}
	return false
}

func bindingUsesV3(value any) bool {
	binding, _ := value.(map[string]any)
	return binding["from"] == "subscription" || binding["from"] == "publication"
}

func authorOutcomes(stages map[string]any) []any {
	set := map[string]bool{}
	for _, raw := range stages {
		stage, _ := raw.(map[string]any)
		if stage["kind"] == "finish" {
			if outcome, ok := stage["outcome"].(string); ok {
				set[outcome] = true
			}
		}
	}
	result := make([]any, 0, len(set))
	for outcome := range set {
		result = append(result, outcome)
	}
	slices.SortFunc(result, func(a, b any) int { return strings.Compare(a.(string), b.(string)) })
	return result
}

func cloneObject(value any) map[string]any {
	object, _ := value.(map[string]any)
	if object == nil {
		return nil
	}
	clone := make(map[string]any, len(object))
	for key, child := range object {
		clone[key] = child
	}
	return clone
}
