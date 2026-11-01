package main

import (
	"encoding/json"
	"maps"
	"path/filepath"
	"slices"
	"strings"
)

func (n *projectCompositionNode) readUnifiedPoints() error {
	pending := []projectPendingDocument{}
	owner := filepath.ToSlash(filepath.Dir(n.rootDocument().Source))
	for i := range n.source.Documents {
		d := &n.source.Documents[i]
		object, _ := d.Value.(map[string]any)
		id, _ := object["id"].(string)
		version, _ := object["version"].(string)
		d.Unified = &projectUnifiedDefinition{Owner: owner, Source: d.Source, AuthorID: id, AuthorVersion: version, Alias: d.Alias}
		pending = append(pending, projectPendingDocument{document: *d, value: d.Value, index: i})
	}
	_, derived := n.source.RootValue.(map[string]any)["extends"]
	if _, err := projectPreparePointExtensions(n.source, pending, nil); err != nil && !derived {
		return projectCompositionSourceError(n.rootDocument().Source, err)
	}
	if n.source.ExtensionPoints == nil {
		return nil
	}
	for _, name := range slices.Sorted(maps.Keys(n.source.ExtensionPoints.Points)) {
		contract := n.source.ExtensionPoints.Points[name]
		target := ""
		for _, d := range n.source.Documents {
			if d.Kind == "workflow" && d.Unified.AuthorID == contract.Workflow && !strings.HasPrefix(d.Source, owner+"/project/") {
				target = d.Alias
			}
		}
		if target == "" {
			return refusal("project_extension_point_invalid", name+": target must belong to declaring package")
		}
		p := projectUnifiedPoint{Identity: owner + "#" + name, Owner: owner, Name: name, Target: target, Contract: contract}
		n.points[name] = []projectUnifiedPoint{p}
	}
	return nil
}
func (n *projectCompositionNode) readUnifiedPointExports() error {
	object, _ := n.source.RootValue.(map[string]any)
	exports, _ := object["exports"].(map[string]any)
	raw, exists := exports["points"]
	if !exists {
		return nil
	}
	if object["authoring"] != projectUnifiedFolderVersion {
		return refusal("project_composition_marker_required", "point exports require folder /3")
	}
	entries, ok := raw.(map[string]any)
	if !ok || len(entries) == 0 {
		return refusal("project_composition_invalid_export", "points must be nonempty")
	}
	for _, name := range slices.Sorted(maps.Keys(entries)) {
		fields, ok := entries[name].(map[string]any)
		if !ok || len(fields) != 2 || !projectValueName.MatchString(name) {
			return refusal("project_composition_invalid_export", name)
		}
		workflow, wok := fields["workflow"].(string)
		point, pok := fields["point"].(string)
		ps := n.points[point]
		target := n.exports[workflow]
		if !wok || !pok || len(ps) != 1 || target == "" || n.exports[name] != "" || !n.unifiedReachable(target, ps[0].Target, map[string]bool{}) {
			return refusal("project_composition_invalid_export", name+": point must be owned and reachable from its workflow export")
		}
		ownedWorkflow := false
		for _, d := range n.source.Documents {
			if d.Alias == target && d.Kind == "workflow" && !d.FolderRoot {
				ownedWorkflow = true
			}
		}
		if !ownedWorkflow {
			return refusal("project_composition_invalid_export", name+": workflow export required")
		}
		n.pointExports[name] = projectUnifiedPointExport{Workflow: workflow, Point: ps[0]}
	}
	return nil
}
func (n *projectCompositionNode) unifiedReachable(from, target string, seen map[string]bool) bool {
	if from == target {
		return true
	}
	if seen[from] {
		return false
	}
	seen[from] = true
	for _, d := range n.source.Documents {
		if d.Alias == from {
			for _, a := range projectCompositionPlaceholders(d.Value) {
				if resolved := n.locals[a]; resolved != "" {
					a = resolved
				}
				if n.unifiedReachable(a, target, seen) {
					return true
				}
			}
		}
	}
	return false
}
func (n *projectCompositionNode) inheritUnifiedPoints() {
	for _, parent := range n.parents {
		for name, points := range parent.points {
			for _, p := range points {
				if p.Target == parent.rootDocument().Name {
					p.Target = n.rootDocument().Name
				}
				if !slices.ContainsFunc(n.points[name], func(old projectUnifiedPoint) bool { return old.Identity == p.Identity && old.Target == p.Target }) {
					n.points[name] = append(n.points[name], p)
				}
			}
		}
	}
}
func (n *projectCompositionNode) unifiedImportedTarget(name, kind string) (string, error) {
	parts := strings.Split(name, ".")
	p := n.imports[parts[0]]
	if p == nil {
		return "", refusal("project_composition_unknown_import", name)
	}
	target := p.exports[parts[1]]
	if target == "" {
		return "", refusal("project_composition_unknown_export", name)
	}
	if kind != "" {
		found := false
		for _, d := range p.source.Documents {
			if d.Alias == target && d.Kind == kind {
				found = true
			}
		}
		if !found {
			return "", refusal("project_composition_wrong_kind", name)
		}
	}
	alias := projectUnifiedAlias([]string{n.rootDocument().Source, name})
	n.importUses[alias] = target
	n.importUseLabels[alias] = n.name + "." + name
	return alias, nil
}
func (n *projectCompositionNode) unifiedSelectPoint(raw any) (projectUnifiedPoint, error) {
	if name, ok := raw.(string); ok {
		ps := n.points[name]
		if len(ps) != 1 {
			return projectUnifiedPoint{}, refusal("project_extension_point_ambiguous", name+": select one visible point")
		}
		return ps[0], nil
	}
	object, ok := raw.(map[string]any)
	if !ok || len(object) != 2 {
		return projectUnifiedPoint{}, refusal("project_extension_point_invalid", "closed point selector required")
	}
	if parent, ok := object["parent"].(string); ok {
		name, ok := object["name"].(string)
		if !ok {
			return projectUnifiedPoint{}, refusal("project_extension_point_invalid", "parent/name required")
		}
		for i, p := range n.parents {
			if n.parentNames[i] == parent {
				ps := p.points[name]
				if len(ps) != 1 {
					return projectUnifiedPoint{}, refusal("project_extension_point_ambiguous", parent+"."+name)
				}
				point := ps[0]
				if point.Target == p.rootDocument().Name {
					point.Target = n.rootDocument().Name
				}
				return point, nil
			}
		}
		return projectUnifiedPoint{}, refusal("project_composition_unknown_parent", parent)
	}
	alias, aok := object["import"].(string)
	export, eok := object["export"].(string)
	if !aok || !eok {
		return projectUnifiedPoint{}, refusal("project_extension_point_invalid", "import/export required")
	}
	provider := n.imports[alias]
	if provider == nil {
		return projectUnifiedPoint{}, refusal("project_composition_unknown_import", alias)
	}
	published, ok := provider.pointExports[export]
	if !ok {
		return projectUnifiedPoint{}, refusal("project_composition_point_not_exported", alias+"."+export)
	}
	point := published.Point
	point.Export = export
	scope, err := n.unifiedImportedTarget(alias+"."+published.Workflow, "workflow")
	point.Scope = scope
	return point, err
}

func (r *projectCompositionReader) unifiedOptionsDocument(n *projectCompositionNode, object map[string]any, source string, depth int) (projectWorkflowOptions, error) {
	marker, _ := object["authoring"].(string)
	if marker == projectUnifiedExtensionVersion {
		if err := projectValidatePointShape("extension-v5", object); err != nil {
			return projectWorkflowOptions{}, err
		}
	}
	if marker == projectPointExtensionVersion {
		if err := projectValidatePointShape("extension-v4", object); err != nil {
			return projectWorkflowOptions{}, err
		}
	}
	copy := projectCompositionClone(object).(map[string]any)
	entries, _ := copy["extensions"].([]any)
	roots := []any{}
	points := []any{}
	for _, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok {
			return projectWorkflowOptions{}, refusal("project_extension_invalid", source)
		}
		if _, exists := entry["point"]; exists {
			points = append(points, entry)
		} else {
			roots = append(roots, entry)
		}
	}
	copy["extensions"] = roots
	if marker == projectUnifiedExtensionVersion {
		copy["authoring"] = projectCompositionExtensionVersion
	}
	programStart := len(n.programs)
	options, err := r.legacyOptionsDocument(n, copy, source, depth)
	for i := programStart; i < len(n.programs); i++ {
		n.programs[i].Owner = filepath.ToSlash(filepath.Dir(n.rootDocument().Source))
	}
	if err != nil {
		return options, err
	}
	for _, raw := range points {
		entry := raw.(map[string]any)
		if marker != projectUnifiedExtensionVersion && marker != projectPointExtensionVersion {
			return options, refusal("project_extension_unsupported_authoring", "point requires extension /4 or /5")
		}
		point, err := n.unifiedSelectPoint(entry["point"])
		if err != nil {
			return options, projectCompositionSourceError(source, err)
		}
		normalized := projectCompositionClone(entry).(map[string]any)
		normalized["point"] = point.Name
		for _, key := range []string{"step", "call"} {
			if _, exists := normalized[key]; exists {
				normalized[key] = "implementation"
			}
		}
		data, _ := json.Marshal(map[string]any{"authoring": projectPointExtensionVersion, "extensions": []any{normalized}})
		parsed, err := parseProjectWorkflowOptions(data)
		if err != nil {
			return options, err
		}
		ext := parsed.Extensions[0]
		ext.Source = source + "#" + ext.ID
		ext.Selector = n.name + "." + ext.ID
		ext.Composed = true
		ext.UnifiedPoint = &point
		ext.Workflow = point.Target
		ext.ResolvedPoint = &ext.UnifiedPoint.Contract
		if len(ext.OutputBindings) != len(point.Contract.Consumers) {
			return options, refusal("project_extension_point_binding_not_permitted", ext.Source+": bind every consumer")
		}
		for name := range ext.OutputBindings {
			if _, ok := point.Contract.Consumers[name]; !ok {
				return options, refusal("project_extension_point_binding_not_permitted", name)
			}
		}
		if projectExtensionEnabled(ext) {
			kind, key := "step", "step"
			if _, exists := entry["call"]; exists {
				kind, key = "workflow", "call"
			}
			target, ok := entry[key].(string)
			if !ok {
				return options, refusal("project_extension_point_invalid", ext.Source)
			}
			if marker == projectPointExtensionVersion && !projectValueName.MatchString(target) {
				return options, refusal("project_extension_point_invalid", ext.Source)
			}
			resolved, err := n.target(target, kind)
			if err != nil {
				return options, err
			}
			if !projectCompositionQualified(target) {
				owned := false
				prefix := filepath.ToSlash(filepath.Join(filepath.Dir(n.rootDocument().Source), projectOwnedFolder)) + "/"
				for _, d := range n.source.Documents {
					if d.Alias == resolved && strings.HasPrefix(d.Source, prefix) && d.Unified.Owner == filepath.ToSlash(filepath.Dir(n.rootDocument().Source)) {
						owned = true
					}
				}
				if !owned {
					return options, refusal("project_extension_point_component_not_permitted", target+": implementation must belong to declaring project/")
				}
			}
			if kind == "step" {
				ext.Step = resolved
				ext.Call = ""
			} else {
				ext.Call = resolved
				ext.Step = ""
			}
			routes := point.Contract.Routes.Step
			if kind == "workflow" {
				routes = point.Contract.Routes.Call
			}
			if routes == nil {
				return options, refusal("project_extension_point_component_not_permitted", ext.Source)
			}
		} else {
			ext.Step = ""
			ext.Call = ""
		}
		options.Extensions = append(options.Extensions, ext)
	}
	seen := map[string]bool{}
	for _, e := range options.Extensions {
		if seen[e.ID] {
			return options, refusal("project_extension_stage_conflict", e.ID)
		}
		seen[e.ID] = true
	}
	return options, nil
}

// Root and point groups use the same native lowerer but separate authority.
func (n *projectCompositionNode) validateUnifiedOwnPoints() error {
	owner := filepath.ToSlash(filepath.Dir(n.rootDocument().Source))
	pending := []projectPendingDocument{}
	source := n.source
	source.Documents = nil
	for _, d := range n.source.Documents {
		if d.Unified != nil && d.Unified.Owner == owner {
			pending = append(pending, projectPendingDocument{document: d, value: d.Value})
			source.Documents = append(source.Documents, d)
		}
	}
	_, err := projectPreparePointExtensions(source, pending, nil)
	return err
}
