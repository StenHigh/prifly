package main

import (
	"crypto/sha256"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/stenhigh/prifly/internal/projectwire"
)

func (r *projectCompositionReader) finishUnified(n *projectCompositionNode) (projectPackageSource, error) {
	if n.options.Exclude == nil {
		n.options.Exclude = []string{}
	}
	source := n.source
	available := map[string]projectPackageDocument{}
	uses := map[string]string{}
	policies := map[string]map[string][]projectWorkflowExtension{}
	composition := &projectComposition{Options: n.options, CompositionReview: projectwire.CompositionReview{SchemaVersion: "workflow-composition/2", Files: r.files, Parents: append([]string{}, n.parentNames...), Sources: []string{}, Imports: map[string]string{}, Layers: append([]string{}, r.layerOrder...), Contributions: []string{}, Groups: []projectwire.CompositionGroupReview{}}, Labels: map[string]string{}, ExportTargets: map[string]string{}}
	for _, key := range slices.Sorted(maps.Keys(r.nodes)) {
		node := r.nodes[key]
		relative, err := projectRelativePath(r.root, node.source.Folder)
		if err != nil {
			return source, err
		}
		composition.Sources = append(composition.Sources, relative)
		for _, d := range node.source.Documents {
			available[d.Alias] = d
		}
		maps.Copy(uses, node.importUses)
		composition.Programs = append(composition.Programs, node.programs...)
		for alias, provider := range node.imports {
			label := node.name + "." + alias
			if node == n {
				label = alias
			}
			composition.Imports[label] = node.importTargets[alias]
			for export, target := range provider.exports {
				composition.ExportTargets[label+"."+export] = target
			}
		}
		if raw, ok := node.source.RootValue.(map[string]any)["execution_bindings"]; ok {
			value, err := node.rewrite(raw)
			if err != nil {
				return source, err
			}
			composition.Programs = append(composition.Programs, projectCompositionProgram{Folder: node.source.Folder, Value: value, Owner: relative, Targets: projectUnifiedProgramTargets(node.source.Documents, relative)})
		}
		for key, value := range node.source.References {
			if old, ok := source.References[key]; ok && old != value {
				return source, refusal("project_composition_reference_conflict", key)
			}
			source.References[key] = value
		}
		source.Dependencies = append(source.Dependencies, node.source.Dependencies...)
		source.RequestedCapabilities = append(source.RequestedCapabilities, node.source.RequestedCapabilities...)
		if node.source.ExtensionPoints != nil {
			if err := r.record(filepath.ToSlash(filepath.Join(relative, projectExtensionPointsFile))); err != nil {
				return source, err
			}
		}
	}
	// The effective inherited root is the authority for its root correspondence.
	for _, d := range n.source.Documents {
		available[d.Alias] = d
	}
	rootAlias := n.rootDocument().Alias
	roots := projectPointRootExtensions(n.options.Extensions)
	for _, e := range n.options.Extensions {
		composition.Contributions = append(composition.Contributions, e.Source)
		if e.Point != "" {
			p := e.UnifiedPoint
			if policies[p.Scope] == nil {
				policies[p.Scope] = map[string][]projectWorkflowExtension{}
			}
			policies[p.Scope][p.Target] = append(policies[p.Scope][p.Target], e)
		}
		if e.Group != nil {
			members := []string{}
			for _, m := range projectCompositionLeaves(e.Group.Members) {
				members = append(members, m.Selector)
			}
			composition.Groups = append(composition.Groups, projectwire.CompositionGroupReview{ID: e.ID, Mode: e.Group.Kind, Members: members})
		}
	}
	scopeLabels := map[string]string{}
	for _, node := range r.nodes {
		maps.Copy(scopeLabels, node.importUseLabels)
	}
	unified := &projectUnifiedBuild{Scopes: map[string][]string{}, ScopeLabels: scopeLabels, Points: map[string][]projectWorkflowExtension{}, Definitions: map[string]*projectUnifiedDefinition{}}
	built := map[string]projectPackageDocument{}
	memo := map[string]string{}
	active := map[string]bool{}
	ordered := []projectPackageDocument{}
	workflowCount := 0
	var build func(string, string) (string, error)
	var buildExtension func(projectWorkflowExtension, string) (projectWorkflowExtension, error)
	buildExtension = func(e projectWorkflowExtension, scope string) (projectWorkflowExtension, error) {
		if !projectExtensionEnabled(e) {
			return e, nil
		}
		var err error
		if e.Group != nil {
			g := *e.Group
			g.Members = slices.Clone(g.Members)
			for i, m := range g.Members {
				g.Members[i], err = buildExtension(m, scope)
				if err != nil {
					return e, err
				}
			}
			e.Group = &g
		} else if e.Call != "" {
			e.Call, err = build(e.Call, "")
		} else if e.Step != "" {
			e.Step, err = build(e.Step, "")
		}
		if err != nil {
			return e, err
		}
		// Owned binding refs use the selected closure. External references and
		// late-bound values stay for the ordinary reference/value resolver.
		e.Bindings, err = projectUnifiedRewriteBindings(e.Bindings, func(alias string) (string, error) {
			if _, ok := available[alias]; !ok {
				if _, ok := uses[alias]; !ok {
					return alias, nil
				}
			}
			return build(alias, scope)
		})
		return e, err
	}
	build = func(alias, scope string) (string, error) {
		if target, ok := uses[alias]; ok {
			return build(target, alias)
		}
		memoKey := alias + "\x00" + scope
		if old := memo[memoKey]; old != "" {
			return old, nil
		}
		if active[memoKey] {
			return "", refusal("project_compile_dependency_cycle", alias)
		}
		active[memoKey] = true
		defer delete(active, memoKey)
		d, ok := available[alias]
		if !ok {
			return "", refusal("project_composition_unknown_component", alias)
		}
		value := projectCompositionClone(d.Value)
		dependencies := map[string]string{}
		rewrite := func(a string) (string, error) {
			if _, ok := available[a]; !ok {
				if _, ok := uses[a]; !ok {
					return a, nil
				}
			}
			v, err := build(a, scope)
			if err == nil {
				dependencies[v] = built[v].Unified.VariantKey
			}
			return v, err
		}
		var err error
		value, err = projectUnifiedRewriteValue(value, rewrite)
		if err != nil {
			return "", err
		}
		selected := []projectWorkflowExtension{}
		for _, e := range policies[scope][alias] {
			e, err = buildExtension(e, scope)
			if err != nil {
				return "", err
			}
			selected = append(selected, e)
		}
		localRoots := []projectWorkflowExtension{}
		if alias == rootAlias && scope == "" {
			for _, e := range roots {
				e, err = buildExtension(e, scope)
				if err != nil {
					return "", err
				}
				localRoots = append(localRoots, e)
			}
		}
		var settings map[string]any
		if scope == "" {
			settings = n.options.Settings[alias]
		} else if uses[scope] == alias {
			settings = n.options.Settings[scope]
		}
		object, _ := value.(map[string]any)
		if len(settings) > 0 {
			for input, v := range settings {
				if err := projectSetWorkflowSetting(object, projectExtensionWorkflowName(d.Unified.AuthorID), input, v); err != nil {
					return "", err
				}
			}
		}
		// Normalize dependencies before hashing; internal aliases are never identity.
		normalize := func(a string) (string, error) {
			if doc, ok := built[a]; ok {
				return doc.Unified.VariantKey, nil
			}
			return a, nil
		}
		keyValue, err := projectUnifiedRewriteValue(value, normalize)
		if err != nil {
			return "", err
		}
		policy := []any{}
		var policyRecord func(projectWorkflowExtension) any
		policyRecord = func(e projectWorkflowExtension) any {
			record := map[string]any{"id": e.ID, "enabled": projectExtensionEnabled(e), "source": e.Source, "outputs": e.OutputBindings, "bindings": e.Bindings, "inputs": e.InputBindings, "on": e.On, "on_error": e.OnError, "impossible_verdicts": e.ImpossibleVerdicts}
			if e.UnifiedPoint != nil {
				record["point"] = e.UnifiedPoint.Identity
				record["contract"] = e.UnifiedPoint.Contract
			}
			if e.Group != nil {
				members := []any{}
				for _, m := range e.Group.Members {
					members = append(members, policyRecord(m))
				}
				record["group"] = map[string]any{"kind": e.Group.Kind, "members": members, "selection": e.Group.Selection, "predicates": e.Group.Predicates, "default": e.Group.Default}
			} else {
				target := e.Call
				if target == "" {
					target = e.Step
				}
				if doc, ok := built[target]; ok {
					record["implementation"] = doc.Unified.VariantKey
					dependencies[target] = doc.Unified.VariantKey
				}
			}
			return record
		}
		for _, e := range append(slices.Clone(selected), localRoots...) {
			policy = append(policy, policyRecord(e))
		}
		scopeKind := "component"
		if alias == rootAlias && scope == "" {
			scopeKind = "root"
		}
		key := projectUnifiedHash(map[string]any{"owner": d.Unified.Owner, "source": d.Source, "source_digest": r.files[d.Source], "author_id": d.Unified.AuthorID, "author_version": d.Unified.AuthorVersion, "scope": scopeKind, "document": keyValue, "policy": policy, "settings": settings})
		variantAlias := "uv" + strings.TrimPrefix(key, "sha256:")
		memo[memoKey] = variantAlias
		label := scopeLabels[scope]
		if label == "" {
			label = "root"
		}
		if !slices.Contains(unified.Scopes[variantAlias], label) {
			unified.Scopes[variantAlias] = append(unified.Scopes[variantAlias], label)
			slices.Sort(unified.Scopes[variantAlias])
		}
		if _, exists := built[variantAlias]; exists {
			return variantAlias, nil
		}
		if d.Kind == "workflow" {
			workflowCount++
			if workflowCount > 256 {
				return "", refusal("project_composition_variant_limit", "at most 256 Workflow variants")
			}
		}
		meta := *d.Unified
		meta.SourceDigest = r.files[d.Source]
		meta.VariantKey = key
		meta.Settings = map[string]any{}
		maps.Copy(meta.Settings, settings)
		object["version"] = strings.Replace(projectBuildVersionFor(sha256.Sum256([]byte(key))), "-b1.", "-uv.", 1)
		d.Value = object
		d.Name = variantAlias
		d.Alias = variantAlias
		d.Extensions = ""
		d.Unified = &meta
		d.FolderRoot = alias == rootAlias && scope == ""
		if !d.FolderRoot {
			if _, ok := object["package"]; ok {
				d.Value, err = projectFolderWorkflowDefinition(object)
				if err != nil {
					return "", err
				}
			}
		} else {
			d.Extensions = d.Source
		}
		if d.FolderRoot {
			for i := range localRoots {
				localRoots[i].Workflow = variantAlias
			}
			composition.Options.Extensions = localRoots
		}
		// Point insertion and direct root insertion cannot occupy the same edge.
		occupied := map[string]bool{}
		for i := range selected {
			e := &selected[i]
			p := *e.UnifiedPoint
			p.Target = variantAlias
			e.UnifiedPoint = &p
			e.ResolvedPoint = &p.Contract
			e.Workflow = variantAlias
			if !projectExtensionEnabled(*e) {
				continue
			}
			anchor := "entry"
			if p.Contract.Between != nil {
				anchor = p.Contract.Between.From + "/" + p.Contract.Between.To
			}
			if occupied[anchor] {
				return "", refusal("project_extension_point_conflict", p.Name)
			}
			occupied[anchor] = true
			for _, consumer := range p.Contract.Consumers {
				key := "consumer/" + consumer.Stage + "/" + consumer.Input + "/" + consumer.Output
				if occupied[key] {
					return "", refusal("project_extension_point_conflict", p.Name)
				}
				occupied[key] = true
			}
		}
		for _, e := range localRoots {
			if occupied[e.From+"/"+e.To] {
				return "", refusal("project_extension_point_conflict", e.Source)
			}
		}
		unified.Points[variantAlias] = selected
		unified.Definitions[variantAlias] = &meta
		built[variantAlias] = d
		ordered = append(ordered, d)
		return variantAlias, nil
	}
	rootVariant, err := build(rootAlias, "")
	if err != nil {
		return source, err
	}
	source.Documents = ordered
	source.RootValue = built[rootVariant].Value
	source.ExtensionPoints = nil
	source.Unified = unified
	source.Composition = composition
	source.ReadPaths = slices.Sorted(maps.Keys(r.files))
	source.Dependencies = slices.Compact(slices.Sorted(slices.Values(source.Dependencies)))
	source.RequestedCapabilities = append([]string{}, slices.Compact(slices.Sorted(slices.Values(source.RequestedCapabilities)))...)
	composition.Sources = slices.Compact(slices.Sorted(slices.Values(composition.Sources)))
	composition.Options.Settings = map[string]map[string]any{} // Applied to the isolated source variant before native compilation.
	for alias, d := range built {
		composition.Labels[alias] = d.Unified.Owner + ":" + d.Unified.AuthorID
	}
	composition.Labels[rootVariant] = "root"
	// Keep each declared program's aliases; a compiled variant retains its origin alias.
	return source, nil
}
func projectUnifiedRewriteValue(value any, resolve func(string) (string, error)) (any, error) {
	switch v := value.(type) {
	case string:
		if m := projectValueReference.FindStringSubmatch(v); m != nil {
			alias, err := resolve(m[1])
			return "{{" + alias + "}}", err
		}
		return v, nil
	case map[string]any:
		result := map[string]any{}
		for _, k := range slices.Sorted(maps.Keys(v)) {
			x, err := projectUnifiedRewriteValue(v[k], resolve)
			if err != nil {
				return nil, err
			}
			result[k] = x
		}
		return result, nil
	case []any:
		result := make([]any, len(v))
		for i, x := range v {
			var err error
			result[i], err = projectUnifiedRewriteValue(x, resolve)
			if err != nil {
				return nil, err
			}
		}
		return result, nil
	default:
		return value, nil
	}
}
func projectUnifiedRewriteBindings(bindings map[string]any, resolve func(string) (string, error)) (map[string]any, error) {
	if bindings == nil {
		return nil, nil
	}
	v, e := projectUnifiedRewriteValue(bindings, resolve)
	if e != nil {
		return nil, e
	}
	return v.(map[string]any), nil
}
