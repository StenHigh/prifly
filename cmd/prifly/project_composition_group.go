package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/stenhigh/prifly/internal/flow"
)

type projectCompositionGroup struct {
	Kind       string                     `json:"kind"`
	Members    []projectWorkflowExtension `json:"members"`
	Selection  string                     `json:"selection,omitempty"`
	Predicates []any                      `json:"predicates,omitempty"`
	Default    int                        `json:"default,omitempty"`
}

// Collapse a declared group into one contribution before another descendant
// inherits it. An inherited mandatory group cannot lose an internal member.
func projectCompositionArrangeNode(node *projectCompositionNode) error {
	bySelector := map[string]projectWorkflowExtension{}
	for _, extension := range node.options.Extensions {
		if _, exists := bySelector[extension.Selector]; exists {
			return refusal("project_composition_duplicate_contribution", extension.Selector)
		}
		bySelector[extension.Selector] = extension
	}
	used := map[string]bool{}
	grouped := []projectWorkflowExtension{}
	groupIDs := map[string]bool{}
	for _, raw := range node.groups {
		object, ok := raw.(map[string]any)
		if !ok {
			return refusal("project_composition_invalid_group", node.name+": arrange entry must be an object")
		}
		for key := range object {
			if !slices.Contains([]string{"id", "between", "sequence", "parallel", "choice", "on", "on_error"}, key) {
				return refusal("project_composition_invalid_group", node.name+": unknown group field "+key)
			}
		}
		id, ok := object["id"].(string)
		if !ok || !projectValueName.MatchString(id) {
			return refusal("project_composition_invalid_group", "group requires a valid id")
		}
		if groupIDs[id] {
			return refusal("project_extension_stage_conflict", node.name+": duplicate group id "+id)
		}
		groupIDs[id] = true
		between, ok := object["between"].(map[string]any)
		if !ok || len(between) != 2 {
			return refusal("project_composition_invalid_group", id+": between requires from/to")
		}
		from, _ := between["from"].(string)
		to, _ := between["to"].(string)
		if from == "" || to == "" {
			return refusal("project_composition_invalid_group", id+": empty attachment point")
		}
		group, err := projectCompositionReadGroup(object, bySelector, used, from, to, 0)
		if err != nil {
			return err
		}
		routes, ok := object["on"].(map[string]any)
		if !ok || len(routes) != 2 {
			return refusal("project_composition_invalid_group", id+": on requires succeeded/rejected")
		}
		succeeded, _ := routes["succeeded"].(string)
		rejected, _ := routes["rejected"].(string)
		if succeeded != to || rejected == "" || rejected == to {
			return refusal("project_composition_invalid_group", id+": success must reach between.to; rejection needs a different route")
		}
		onError, _ := object["on_error"].(string)
		if onError == "" {
			onError = rejected
		}
		grouped = append(grouped, projectWorkflowExtension{ID: id, Workflow: node.rootDocument().Name, From: from, To: to, On: map[string]string{"succeeded": succeeded, "rejected": rejected}, OnError: onError, Composed: true, Source: node.rootDocument().Source + "#arrange/" + id, Selector: node.name + "." + id, Group: group})
	}
	groups := grouped
	grouped = nil
	emitted := map[string]bool{}
	for _, extension := range node.options.Extensions {
		if !used[extension.Selector] {
			grouped = append(grouped, extension)
			continue
		}
		for _, group := range groups {
			if !emitted[group.Selector] && projectCompositionContainsSelector(group.Group.Members, extension.Selector) {
				grouped = append(grouped, group)
				emitted[group.Selector] = true
			}
		}
	}
	if len(emitted) != len(groups) {
		return refusal("project_composition_invalid_group", node.name+": arrange did not preserve every group")
	}
	if err := projectCompositionValidateMembers(grouped, 0, map[string]string{}); err != nil {
		return err
	}
	edges := map[string]string{}
	ids := map[string]string{}
	for _, extension := range grouped {
		edge := extension.From + " -> " + extension.To
		if previous := edges[edge]; previous != "" {
			return refusal("project_composition_arrange_required", previous+" and "+extension.Selector+" share "+edge+"; declare sequence, parallel or choice")
		}
		if previous := ids[extension.ID]; previous != "" {
			return refusal("project_extension_stage_conflict", previous+" and "+extension.Selector+" use stage "+extension.ID)
		}
		edges[edge] = extension.Selector
		ids[extension.ID] = extension.Selector
	}
	node.options.Extensions = grouped
	return nil
}

func projectCompositionReadGroup(object map[string]any, contributions map[string]projectWorkflowExtension, used map[string]bool, from, to string, depth int) (*projectCompositionGroup, error) {
	if depth >= 8 {
		return nil, refusal("project_composition_limit", "group depth exceeds 8")
	}
	kind := ""
	for _, candidate := range []string{"sequence", "parallel", "choice"} {
		if _, exists := object[candidate]; exists {
			if kind != "" {
				return nil, refusal("project_composition_invalid_group", "declare exactly one group operator")
			}
			kind = candidate
		}
	}
	if kind == "" {
		return nil, refusal("project_composition_invalid_group", "missing sequence, parallel or choice")
	}
	group := &projectCompositionGroup{Kind: kind}
	readMember := func(raw any) (projectWorkflowExtension, error) {
		if selector, ok := raw.(string); ok {
			extension, exists := contributions[selector]
			if !exists {
				return extension, refusal("project_composition_unknown_contribution", selector)
			}
			if used[selector] {
				return extension, refusal("project_composition_duplicate_contribution", selector)
			}
			if extension.From != from || extension.To != to {
				return extension, refusal("project_composition_attachment_conflict", selector+": contribution belongs to a different edge")
			}
			used[selector] = true
			return extension, nil
		}
		nested, ok := raw.(map[string]any)
		if !ok {
			return projectWorkflowExtension{}, refusal("project_composition_invalid_group", "member must name a contribution or group")
		}
		for key := range nested {
			if key != "sequence" && key != "parallel" && key != "choice" {
				return projectWorkflowExtension{}, refusal("project_composition_invalid_group", "unknown nested group field "+key)
			}
		}
		child, err := projectCompositionReadGroup(nested, contributions, used, from, to, depth+1)
		return projectWorkflowExtension{Group: child, Composed: true}, err
	}
	if kind != "choice" {
		members, ok := object[kind].([]any)
		if !ok || len(members) == 0 || len(members) > 32 {
			return nil, refusal("project_composition_invalid_group", kind+" needs 1..32 members")
		}
		for _, raw := range members {
			member, err := readMember(raw)
			if err != nil {
				return nil, err
			}
			group.Members = append(group.Members, member)
		}
		return group, nil
	}
	choice, ok := object[kind].(map[string]any)
	if !ok {
		return nil, refusal("project_composition_invalid_group", "choice must be an object")
	}
	for key := range choice {
		if !slices.Contains([]string{"selection", "branches", "default", "on_unknown"}, key) {
			return nil, refusal("project_composition_invalid_group", "unknown choice field "+key)
		}
	}
	group.Selection, _ = choice["selection"].(string)
	if group.Selection != "first_match" && group.Selection != "exclusive" {
		return nil, refusal("project_composition_invalid_group", "choice selection must be first_match or exclusive")
	}
	if choice["on_unknown"] != "rejected" {
		return nil, refusal("project_composition_invalid_group", "choice on_unknown must explicitly reject")
	}
	branches, ok := choice["branches"].([]any)
	if !ok || len(branches) == 0 || len(branches) > 32 {
		return nil, refusal("project_composition_invalid_group", "choice requires 1..32 branches")
	}
	for _, raw := range branches {
		branch, ok := raw.(map[string]any)
		if !ok || len(branch) != 2 || branch["predicate"] == nil || branch["use"] == nil {
			return nil, refusal("project_composition_invalid_group", "choice branch requires predicate/use")
		}
		member, err := readMember(branch["use"])
		if err != nil {
			return nil, err
		}
		group.Members = append(group.Members, member)
		group.Predicates = append(group.Predicates, branch["predicate"])
	}
	member, err := readMember(choice["default"])
	if err != nil {
		return nil, err
	}
	group.Default = len(group.Members)
	group.Members = append(group.Members, member)
	return group, nil
}

func projectCompositionLeaves(extensions []projectWorkflowExtension) []projectWorkflowExtension {
	result := []projectWorkflowExtension{}
	for _, extension := range extensions {
		if extension.Group == nil {
			result = append(result, extension)
		} else {
			result = append(result, projectCompositionLeaves(extension.Group.Members)...)
		}
	}
	return result
}

// All generated definitions use the root's explicit limits and policy. Native
// compilation costs the entire closure, including wrappers, before admission.
func projectApplyComposedExtensions(component *projectCompileComponent, components *[]projectCompileComponent, extensions []projectWorkflowExtension) error {
	var root flow.WorkflowRevision
	if err := json.Unmarshal(component.Bytes, &root); err != nil {
		return err
	}
	for _, extension := range extensions {
		if extension.Group == nil {
			if err := projectApplyExtensions(component, *components, []projectWorkflowExtension{extension}); err != nil {
				return projectCompositionSourceError(extension.Source, err)
			}
			if err := json.Unmarshal(component.Bytes, &root); err != nil {
				return err
			}
			continue
		}
		lowerer := projectCompositionLowerer{root: root, components: components, external: map[string]flow.Binding{}, ports: map[string]flow.InputPort{}}
		ref, err := lowerer.lower(extension)
		if err != nil {
			return projectCompositionSourceError(extension.Source, err)
		}
		extension.Group = nil
		extension.Call = projectExtensionWorkflowName(ref.ID)
		extension.InputBindings = map[string]string{}
		extension.Bindings = map[string]any{}
		for port, binding := range lowerer.external {
			data, _ := json.Marshal(binding)
			var value any
			_ = json.Unmarshal(data, &value)
			extension.Bindings[port] = value
		}
		if err := projectApplyExtensions(component, *components, []projectWorkflowExtension{extension}); err != nil {
			return err
		}
		if err := json.Unmarshal(component.Bytes, &root); err != nil {
			return err
		}
	}
	return nil
}

type projectCompositionLowerer struct {
	root       flow.WorkflowRevision
	components *[]projectCompileComponent
	external   map[string]flow.Binding
	ports      map[string]flow.InputPort
}

func (l *projectCompositionLowerer) lift(binding flow.Binding) (flow.Binding, error) {
	if binding.From == "literal" {
		return binding, nil
	}
	key := binding.Port
	var port flow.Port
	if binding.From == "workflow_input" {
		input, ok := l.root.Inputs[binding.Port]
		if !ok {
			return binding, refusal("project_composition_binding_scope", "unknown root input "+binding.Port)
		}
		port = input.Port
	} else if binding.From == "stage_output" {
		stage, ok := l.root.Definition.Stages[binding.StageID]
		if !ok {
			return binding, refusal("project_composition_binding_scope", "group member cannot read sibling output "+binding.StageID+"; declare a typed child workflow")
		}
		key = "stage_" + binding.StageID + "_" + binding.Port
		found := false
		for _, component := range *l.components {
			if stage.Kind == "step" && component.Ref == stage.StepRef {
				var step flow.StepDefinition
				if err := json.Unmarshal(component.Bytes, &step); err != nil {
					return binding, err
				}
				output, ok := step.Outputs[binding.Port]
				if ok {
					port = output.Port
					found = true
				}
			}
			if stage.Kind == "call" && component.Ref == stage.WorkflowRef {
				var child flow.WorkflowRevision
				if err := json.Unmarshal(component.Bytes, &child); err != nil {
					return binding, err
				}
				output, ok := child.Outputs[binding.Port]
				if ok {
					port = output.Port
					found = true
				}
			}
		}
		if !found {
			return binding, refusal("project_composition_binding_scope", "no typed output for "+binding.StageID+"."+binding.Port)
		}
	} else {
		return binding, refusal("project_composition_binding_scope", "unsupported cross-group binding")
	}
	if binding.Pointer != nil {
		if binding.ProjectedSchemaRef == nil {
			return binding, refusal("project_composition_binding_scope", "projection requires projected_schema_ref")
		}
		port.SchemaRef = binding.ProjectedSchemaRef
		data, _ := projectCanonicalJSON(binding)
		key += fmt.Sprintf("_%x", sha256.Sum256(data))[:17]
	}
	if old, exists := l.external[key]; exists && !projectCompositionEqual(old, binding) {
		return binding, refusal("project_composition_binding_scope", "lifted input collision "+key)
	}
	l.external[key] = binding
	l.ports[key] = flow.InputPort{Port: port, Required: true}
	return flow.Binding{From: "workflow_input", Port: key}, nil
}

// Generated wrappers leave technical failures unhandled so the outer Call can
// apply the authored group error route. Business rejection remains an outcome.
func (l *projectCompositionLowerer) lower(extension projectWorkflowExtension) (flow.Ref, error) {
	workflow := flow.WorkflowRevision{SchemaVersion: l.root.SchemaVersion, Version: l.root.Version, Title: "Composed contribution", Inputs: map[string]flow.InputPort{}, Outputs: map[string]flow.OutputPort{}, AllowedOutcomes: []string{"succeeded", "rejected"}, Limits: l.root.Limits, PolicyRef: l.root.PolicyRef}
	workflow.Definition.Entry = "run"
	workflow.Definition.Stages = map[string]flow.Stage{"done": {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]flow.Binding{}}, "rejected": {Kind: "finish", Outcome: "rejected", OutputBindings: map[string]flow.Binding{}}}
	if extension.Group == nil {
		stage := flow.Stage{InputBindings: map[string]flow.Binding{}, On: map[string]string{}, ImpossibleVerdicts: extension.ImpossibleVerdicts}
		for port, raw := range extension.Bindings {
			value, err := flow.CompileAuthoringBinding(raw, nil)
			if err != nil {
				return flow.Ref{}, err
			}
			data, _ := json.Marshal(value)
			var binding flow.Binding
			if err := json.Unmarshal(data, &binding); err != nil {
				return flow.Ref{}, err
			}
			lifted, err := l.lift(binding)
			if err != nil {
				return flow.Ref{}, err
			}
			stage.InputBindings[port] = lifted
		}
		for port, expression := range extension.InputBindings {
			binding := flow.Binding{From: "workflow_input", Port: strings.TrimPrefix(expression, "$inputs.")}
			if strings.HasPrefix(expression, "$stages.") {
				parts := strings.Split(strings.TrimPrefix(expression, "$stages."), ".")
				if len(parts) != 2 {
					return flow.Ref{}, refusal("project_composition_binding_scope", "invalid source")
				}
				binding = flow.Binding{From: "stage_output", StageID: parts[0], Port: parts[1]}
			}
			lifted, err := l.lift(binding)
			if err != nil {
				return flow.Ref{}, err
			}
			stage.InputBindings[port] = lifted
		}
		if extension.Input != "" {
			binding, err := l.lift(flow.Binding{From: "stage_output", StageID: extension.InputStage, Port: extension.InputPort})
			if err != nil {
				return flow.Ref{}, err
			}
			stage.InputBindings[extension.Input] = binding
		}
		target := extension.Call
		kind := "workflow"
		if target == "" {
			target = extension.Step
			kind = "step"
		}
		found := false
		for _, component := range *l.components {
			if component.Kind == kind && projectCompositionComponentNamed(component, target) {
				if found {
					return flow.Ref{}, refusal("project_composition_ambiguous_component", target)
				}
				found = true
				if kind == "workflow" {
					stage.Kind = "call"
					stage.WorkflowRef = component.Ref
					var child flow.WorkflowRevision
					if err := json.Unmarshal(component.Bytes, &child); err != nil {
						return flow.Ref{}, err
					}
					for _, outcome := range child.AllowedOutcomes {
						route, ok := extension.On[outcome]
						if !ok {
							return flow.Ref{}, refusal("project_extension_call_outcome_unmapped", extension.Selector+": "+outcome)
						}
						if outcome == "succeeded" && route == extension.To {
							stage.On[outcome] = "done"
						} else {
							stage.On[outcome] = "rejected"
						}
					}
				} else {
					stage.Kind = "step"
					stage.StepRef = component.Ref
					for verdict, route := range extension.On {
						if verdict == "pass" && route == extension.To {
							stage.On[verdict] = "done"
						} else {
							stage.On[verdict] = "rejected"
						}
					}
				}
			}
		}
		if !found {
			return flow.Ref{}, refusal("project_composition_unknown_component", target)
		}
		workflow.Definition.Stages["run"] = stage
	} else {
		group := extension.Group
		refs := make([]flow.Ref, len(group.Members))
		for i, member := range group.Members {
			ref, err := l.lower(member)
			if err != nil {
				return flow.Ref{}, err
			}
			refs[i] = ref
		}
		bindings := map[string]flow.Binding{}
		for key := range l.ports {
			bindings[key] = flow.Binding{From: "workflow_input", Port: key}
		}
		switch group.Kind {
		case "sequence":
			workflow.Definition.Entry = "member-0"
			for i, ref := range refs {
				next := "done"
				if i+1 < len(refs) {
					next = fmt.Sprintf("member-%d", i+1)
				}
				workflow.Definition.Stages[fmt.Sprintf("member-%d", i)] = flow.Stage{Kind: "call", WorkflowRef: ref, InputBindings: l.childBindings(ref, bindings), On: map[string]string{"succeeded": next, "rejected": "rejected"}}
			}
		case "parallel":
			stage := flow.Stage{Kind: "parallel", Join: &flow.Join{Mode: "all", AcceptOutcomes: []string{"succeeded"}, Selection: "all", Remainder: "wait"}, MaxParallelism: len(refs), On: map[string]string{"satisfied": "done", "unsatisfied": "rejected"}}
			for i, ref := range refs {
				stage.ParallelBranches = append(stage.ParallelBranches, flow.ParallelBranch{ID: fmt.Sprintf("member-%d", i), WorkflowRef: ref, InputBindings: l.childBindings(ref, bindings)})
			}
			workflow.Definition.Stages["run"] = stage
		case "choice":
			stage := flow.Stage{Kind: "choice", Selection: group.Selection, Default: fmt.Sprintf("member-%d", group.Default), OnUnknown: "rejected"}
			for i, raw := range group.Predicates {
				predicate, err := l.predicate(raw)
				if err != nil {
					return flow.Ref{}, err
				}
				stage.Branches = append(stage.Branches, flow.ChoiceBranch{ID: fmt.Sprintf("alternative-%d", i), Predicate: predicate, Next: fmt.Sprintf("member-%d", i)})
			}
			for i, ref := range refs {
				workflow.Definition.Stages[fmt.Sprintf("member-%d", i)] = flow.Stage{Kind: "call", WorkflowRef: ref, InputBindings: l.childBindings(ref, bindings), On: map[string]string{"succeeded": "done", "rejected": "rejected"}}
			}
			workflow.Definition.Stages["run"] = stage
		}
	}
	workflow.Inputs = maps.Clone(l.ports)
	identity, _ := projectCanonicalJSON(map[string]any{"root": l.root.ID, "contribution": extension})
	namespace, _, _ := strings.Cut(l.root.ID, ":")
	prefix := namespace + ":workflow/"
	digestName := fmt.Sprintf("%x", sha256.Sum256(identity))
	available := 128 - len(prefix)
	if available < 32 {
		return flow.Ref{}, refusal("project_composition_limit", "root namespace leaves no room for a generated workflow identity")
	}
	if available < len(digestName) {
		digestName = digestName[:available]
	}
	workflow.ID = prefix + digestName
	data, err := projectCanonicalJSON(workflow)
	if err != nil {
		return flow.Ref{}, err
	}
	digest, err := flow.Digest(data)
	if err != nil {
		return flow.Ref{}, err
	}
	ref := flow.Ref{ID: workflow.ID, Version: workflow.Version, Digest: digest}
	for _, existing := range *l.components {
		if existing.Ref == ref {
			return ref, nil
		}
	}
	component := projectCompileComponent{Kind: "workflow", Ref: ref, Bytes: data, Path: fmt.Sprintf("workflows/%03d.json", len(*l.components))}
	*l.components = append(*l.components, component)
	return ref, nil
}

func (l *projectCompositionLowerer) childBindings(ref flow.Ref, available map[string]flow.Binding) map[string]flow.Binding {
	result := map[string]flow.Binding{}
	for _, component := range *l.components {
		if component.Ref == ref {
			var child flow.WorkflowRevision
			_ = json.Unmarshal(component.Bytes, &child)
			for port := range child.Inputs {
				if binding, ok := available[port]; ok {
					result[port] = binding
				}
			}
		}
	}
	return result
}

func (l *projectCompositionLowerer) predicate(raw any) (flow.Predicate, error) {
	// Reuse the authoring predicate compiler through a minimal source graph.
	author := map[string]any{"authoring": "prifly-workflow/1", "id": l.root.ID, "version": "1.0.0", "title": "Predicate", "inputs": map[string]any{}, "outputs": map[string]any{}, "entry": "choose", "policy_ref": projectRefValue(l.root.PolicyRef), "limits": map[string]any{"max_step_instances": 1, "max_control_transitions": 4}, "stages": map[string]any{"choose": map[string]any{"kind": "choice", "selection": "first_match", "branches": []any{map[string]any{"id": "selected", "predicate": raw, "next": "done"}}, "default": "done", "on_unknown": "done"}, "done": map[string]any{"kind": "finish", "outcome": "succeeded"}}}
	data, _ := json.Marshal(author)
	data, err := flow.WorkflowJSONBytes(data, "yaml")
	if err != nil {
		return flow.Predicate{}, err
	}
	var workflow flow.WorkflowRevision
	if err := json.Unmarshal(data, &workflow); err != nil {
		return flow.Predicate{}, err
	}
	predicate := workflow.Definition.Stages["choose"].Branches[0].Predicate
	encoded, _ := json.Marshal(predicate)
	var value any
	_ = json.Unmarshal(encoded, &value)
	var lift func(any) error
	lift = func(value any) error {
		switch v := value.(type) {
		case map[string]any:
			if from, ok := v["from"].(string); ok && (from == "workflow_input" || from == "stage_output") {
				data, _ := json.Marshal(v)
				var binding flow.Binding
				_ = json.Unmarshal(data, &binding)
				binding.Pointer = nil // A predicate reads a field of the lifted whole value.
				lifted, err := l.lift(binding)
				if err != nil {
					return err
				}
				delete(v, "stage_id")
				v["from"] = lifted.From
				v["port"] = lifted.Port
			}
			for _, child := range v {
				if err := lift(child); err != nil {
					return err
				}
			}
		case []any:
			for _, child := range v {
				if err := lift(child); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := lift(value); err != nil {
		return predicate, err
	}
	encoded, _ = json.Marshal(value)
	// Unmarshal into a fresh value: lifting a stage field removes stage_id.
	// Reusing the old nested pointer would retain that deleted field.
	predicate = flow.Predicate{}
	err = json.Unmarshal(encoded, &predicate)
	return predicate, err
}

// Match contribution containers before descending into unnamed wrappers.
// Flattening here loses the identity of an inherited mandatory group.
func projectCompositionContainsSelector(members []projectWorkflowExtension, selector string) bool {
	for _, member := range members {
		if member.Selector == selector {
			return true
		}
		if member.Group != nil && projectCompositionContainsSelector(member.Group.Members, selector) {
			return true
		}
	}
	return false
}

// Inherited groups stay intact. Overlapping origins cannot be deduplicated
// by removing a member, because that would change an already declared group.
func projectCompositionValidateMembers(members []projectWorkflowExtension, depth int, origins map[string]string) error {
	for _, member := range members {
		if member.Group != nil {
			if depth >= 8 {
				return refusal("project_composition_limit", "effective group depth exceeds 8")
			}
			if err := projectCompositionValidateMembers(member.Group.Members, depth+1, origins); err != nil {
				return err
			}
		} else if member.Source != "" {
			if previous, exists := origins[member.Source]; exists {
				return refusal("project_composition_duplicate_contribution", member.Source+": overlaps "+previous+" and "+member.Selector)
			}
			origins[member.Source] = member.Selector
		}
	}
	return nil
}
