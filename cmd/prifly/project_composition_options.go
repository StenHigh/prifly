package main

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
)

// Each entry is one replaceable authoring value. Scope separates settings
// targets, answer phases and model hosts without splitting opaque objects.
type projectCompositionOptionKey struct{ kind, scope, name string }
type projectCompositionOptionDeclaration struct {
	owner *projectCompositionNode
	value projectWorkflowOptions
}

func (k projectCompositionOptionKey) label() string {
	parts := []string{k.kind}
	if k.scope != "" {
		parts = append(parts, k.scope)
	}
	if k.name != "" {
		parts = append(parts, k.name)
	}
	return strings.Join(parts, ".")
}

func projectCompositionOptionEntries(owner *projectCompositionNode, root string) map[projectCompositionOptionKey]projectWorkflowOptions {
	o := owner.ownOptions
	entries := map[projectCompositionOptionKey]projectWorkflowOptions{}
	for target, values := range o.Settings {
		if target == owner.rootDocument().Name {
			target = root
		}
		for name, value := range values {
			entries[projectCompositionOptionKey{"settings", target, name}] = projectWorkflowOptions{Settings: map[string]map[string]any{target: {name: value}}}
		}
	}
	for name, value := range o.Resources {
		entries[projectCompositionOptionKey{"resources", "", name}] = projectWorkflowOptions{Resources: map[string]projectResource{name: value}}
	}
	for name, value := range o.References {
		entries[projectCompositionOptionKey{"references", "", name}] = projectWorkflowOptions{References: map[string]string{name: value}}
	}
	for host, profiles := range o.ModelProfiles {
		for name, value := range profiles {
			entries[projectCompositionOptionKey{"model_profiles", host, name}] = projectWorkflowOptions{ModelProfiles: map[string]map[string]map[string]string{host: {name: value}}}
		}
	}
	for name, value := range o.Answers.Preflight {
		entries[projectCompositionOptionKey{"answers", "preflight", name}] = projectWorkflowOptions{Answers: projectWorkflowAnswers{Preflight: map[string]json.RawMessage{name: value}}}
	}
	for name, value := range o.Answers.Runtime {
		entries[projectCompositionOptionKey{"answers", "runtime", name}] = projectWorkflowOptions{Answers: projectWorkflowAnswers{Runtime: map[string]json.RawMessage{name: value}}}
	}
	if o.Answers.DecisionPolicy != "" {
		entries[projectCompositionOptionKey{"answers", "", "decision_policy"}] = projectWorkflowOptions{Answers: projectWorkflowAnswers{DecisionPolicy: o.Answers.DecisionPolicy}}
	}
	if o.Profile != "" {
		entries[projectCompositionOptionKey{kind: "profile"}] = projectWorkflowOptions{Profile: o.Profile}
	}
	if o.Exclude != nil {
		entries[projectCompositionOptionKey{kind: "exclude"}] = projectWorkflowOptions{Exclude: slices.Clone(o.Exclude)}
	}
	return entries
}

func projectCompositionResolveOptions(node *projectCompositionNode) (projectWorkflowOptions, error) {
	declarations := map[projectCompositionOptionKey][]projectCompositionOptionDeclaration{}
	for _, path := range slices.Sorted(maps.Keys(node.ancestors)) {
		owner := node.ancestors[path]
		for key, value := range projectCompositionOptionEntries(owner, node.rootDocument().Name) {
			declarations[key] = append(declarations[key], projectCompositionOptionDeclaration{owner, value})
		}
	}
	keys := slices.Collect(maps.Keys(declarations))
	slices.SortFunc(keys, func(a, b projectCompositionOptionKey) int {
		if c := strings.Compare(a.kind, b.kind); c != 0 {
			return c
		}
		if c := strings.Compare(a.scope, b.scope); c != 0 {
			return c
		}
		return strings.Compare(a.name, b.name)
	})
	result := projectWorkflowOptions{}
	for _, key := range keys {
		declared := declarations[key]
		// References retain their existing uniqueness rule. Consumer declarations
		// cannot resolve a reference conflict or repeat an inherited alias.
		if key.kind == "references" {
			var inherited *projectCompositionOptionDeclaration
			own := false
			for i := range declared {
				current := &declared[i]
				if current.owner == node {
					own = true
					continue
				}
				if inherited != nil && !projectCompositionEqual(inherited.value, current.value) {
					return result, refusal("project_composition_setting_conflict", inherited.owner.name+" and "+current.owner.name+" disagree on "+key.label())
				}
				inherited = current
			}
			if own && inherited != nil {
				return result, refusal("project_composition_reference_conflict", node.name+": parent and consumer "+key.label()+" overlap")
			}
		}
		maximal := []projectCompositionOptionDeclaration{}
		for _, current := range declared {
			shadowed := false
			for _, other := range declared {
				if current.owner.key != other.owner.key && other.owner.ancestors[current.owner.key] != nil {
					shadowed = true
					break
				}
			}
			if !shadowed {
				maximal = append(maximal, current)
			}
		}
		if len(maximal) == 0 {
			return result, refusal("project_composition_cycle", "option declarations have no maximal source")
		}
		winner := maximal[0]
		for _, other := range maximal[1:] {
			if !projectCompositionEqual(winner.value, other.value) {
				return result, refusal("project_composition_setting_conflict", winner.owner.name+" and "+other.owner.name+" disagree on "+key.label()+"; declare an explicit consumer value")
			}
		}
		projectCompositionSetOption(&result, key, winner.value)
	}
	// Programs keep their separate ownership/provenance validation. This legacy
	// option view does not select or suppress the declarations in Programs.
	for _, parent := range node.parents {
		result.ExecutionBindings = projectCompositionMapMerge(result.ExecutionBindings, parent.options.ExecutionBindings)
	}
	result.ExecutionBindings = projectCompositionMapMerge(result.ExecutionBindings, node.ownOptions.ExecutionBindings)
	return result, nil
}

// The resolver owns these maps. Apply one selected entry without cloning every
// previously selected key; the immutable declaration values remain shared.
func projectCompositionSetOption(dst *projectWorkflowOptions, key projectCompositionOptionKey, value projectWorkflowOptions) {
	switch key.kind {
	case "settings":
		if dst.Settings == nil {
			dst.Settings = map[string]map[string]any{}
		}
		if dst.Settings[key.scope] == nil {
			dst.Settings[key.scope] = map[string]any{}
		}
		dst.Settings[key.scope][key.name] = value.Settings[key.scope][key.name]
	case "resources":
		if dst.Resources == nil {
			dst.Resources = map[string]projectResource{}
		}
		dst.Resources[key.name] = value.Resources[key.name]
	case "references":
		if dst.References == nil {
			dst.References = map[string]string{}
		}
		dst.References[key.name] = value.References[key.name]
	case "model_profiles":
		if dst.ModelProfiles == nil {
			dst.ModelProfiles = map[string]map[string]map[string]string{}
		}
		if dst.ModelProfiles[key.scope] == nil {
			dst.ModelProfiles[key.scope] = map[string]map[string]string{}
		}
		dst.ModelProfiles[key.scope][key.name] = value.ModelProfiles[key.scope][key.name]
	case "answers":
		switch key.scope {
		case "preflight":
			if dst.Answers.Preflight == nil {
				dst.Answers.Preflight = map[string]json.RawMessage{}
			}
			dst.Answers.Preflight[key.name] = value.Answers.Preflight[key.name]
		case "runtime":
			if dst.Answers.Runtime == nil {
				dst.Answers.Runtime = map[string]json.RawMessage{}
			}
			dst.Answers.Runtime[key.name] = value.Answers.Runtime[key.name]
		default:
			dst.Answers.DecisionPolicy = value.Answers.DecisionPolicy
		}
	case "profile":
		dst.Profile = value.Profile
	case "exclude":
		dst.Exclude = slices.Clone(value.Exclude)
	}
}
