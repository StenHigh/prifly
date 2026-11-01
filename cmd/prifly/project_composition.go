package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/projectwire"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

const projectWorkflowCompositionVersion = "prifly-project-workflow/2"
const projectCompositionExtensionVersion = "prifly-extension/3"

// Composition is source preparation. It never opens an authority or executes
// a program. Documents retain their real paths; Value is an in-memory view.
type projectComposition struct {
	projectwire.CompositionReview
	Options       projectWorkflowOptions      `json:"-"`
	Programs      []projectCompositionProgram `json:"-"`
	Settings      []projectCompositionSetting `json:"-"`
	Labels        map[string]string           `json:"-"`
	ExportTargets map[string]string           `json:"-"`
}

type projectCompositionSetting struct {
	Target, Input, Source string
	Value                 any
}

type projectCompositionProgram struct {
	Folder    string
	Value     any
	Extension bool
	Targets   map[string][]string // kind and author ID to owned document aliases
}

type projectCompositionNode struct {
	source           projectPackageSource
	name             string
	key              string
	prefix           string
	locals           map[string]string
	exports          map[string]string
	imports          map[string]*projectCompositionNode
	importTargets    map[string]string
	parentNames      []string
	height           int
	ancestors        map[string]*projectCompositionNode
	parents          []*projectCompositionNode
	options          projectWorkflowOptions
	ownOptions       projectWorkflowOptions
	groups           []any
	programs         []projectCompositionProgram
	settings         []projectCompositionSetting
	referenceOrigins map[string]string
}

type projectCompositionReader struct {
	root       string
	profile    projectProfile
	nodes      map[string]*projectCompositionNode
	active     map[string]bool
	files      map[string]string
	layers     map[string]bool
	folders    map[string]bool
	layerOrder []string
	bytes      int
	sizes      map[string]int
}

func projectCompositionClone(value any) any {
	data, _ := json.Marshal(value)
	var result any
	_ = json.Unmarshal(data, &result)
	return result
}

func projectCompositionQualified(name string) bool {
	parts := strings.Split(name, ".")
	return len(parts) == 2 && projectValueName.MatchString(parts[0]) && projectValueName.MatchString(parts[1])
}

func projectCompositionBindingExpression(expression string) bool {
	if before, pointer, found := strings.Cut(expression, "#"); found {
		if pointer != "" && !strings.HasPrefix(pointer, "/") {
			return false
		}
		expression = before
	}
	if strings.HasPrefix(expression, "$inputs.") {
		return projectValueName.MatchString(strings.TrimPrefix(expression, "$inputs."))
	}
	return projectCallBindingExpression(expression)
}

func projectReadComposedFolder(root, folder string) (projectPackageSource, error) {
	source, err := readProjectWorkflowFolderRaw(root, folder)
	if err != nil {
		return source, err
	}
	object := source.RootValue.(map[string]any)
	marked := object["authoring"] == projectWorkflowCompositionVersion
	if data, err := readFile(filepath.Join(folder, "extend.yaml"), flow.MaxDocumentBytes); err == nil {
		if value, err := flow.Parse(data, "yaml"); err == nil {
			if extension, ok := value.(map[string]any); ok {
				marked = marked || extension["authoring"] == projectCompositionExtensionVersion
			}
		}
	}
	if !marked {
		return source, nil
	}
	profile, err := readProjectProfile(root)
	if err != nil {
		return source, err
	}
	r := &projectCompositionReader{root: root, profile: profile, nodes: map[string]*projectCompositionNode{}, active: map[string]bool{}, files: map[string]string{}, layers: map[string]bool{}, folders: map[string]bool{}}
	name := "root"
	for _, key := range slices.Sorted(maps.Keys(profile.Packages)) {
		pkg := profile.Packages[key]
		path, err := projectPackageSourceLocation(root, pkg.Source)
		if err == nil && filepath.Clean(path) == filepath.Clean(folder) {
			if _, err := projectExecutionSource(root, root, filepath.ToSlash(filepath.Join(pkg.Source, "workflow.yaml"))); err != nil {
				return source, projectCompositionSourceError(pkg.Source, err)
			}
			name = key
			break
		}
	}
	node, err := r.load(name, folder, true, 0)
	if err != nil {
		return source, err
	}
	return r.finish(node)
}

func (r *projectCompositionReader) record(path string) error {
	_, err := r.read(path)
	return err
}

func (r *projectCompositionReader) read(path string) ([]byte, error) {
	if _, known := r.files[path]; !known && len(r.files) >= prifly.MaxPackagePayloadFiles {
		return nil, refusal("project_composition_limit", "source file count exceeds package payload limit")
	}
	data, err := projectExecutionSource(r.root, r.root, path)
	if err != nil {
		return nil, &prifly.Fault{Code: "project_composition_source_invalid", Message: path + ": cannot read a confined regular source", Cause: err}
	}
	if len(data) > flow.MaxDocumentBytes {
		return nil, refusal("project_composition_limit", path+": source exceeds document byte limit")
	}
	if r.sizes == nil {
		r.sizes = map[string]int{}
	}
	r.bytes += len(data) - r.sizes[path]
	if r.bytes > prifly.MaxPackageArchiveBytes {
		return nil, refusal("project_composition_limit", "source bytes exceed package archive limit")
	}
	r.sizes[path] = len(data)
	r.files[path] = fmt.Sprintf("sha256:%x", sha256.Sum256(data))
	return data, nil
}

func (r *projectCompositionReader) document(path string) (any, error) {
	data, err := r.read(path)
	if err != nil {
		return nil, err
	}
	value, err := flow.Parse(data, "yaml")
	if err != nil {
		return nil, projectCompositionSourceError(path, err)
	}
	return value, nil
}

func (r *projectCompositionReader) dependency(name string, effective bool, depth int) (*projectCompositionNode, error) {
	pkg, ok := r.profile.Packages[name]
	if !ok {
		return nil, refusal("project_composition_unknown_package", name+" is not registered in project.yaml")
	}
	if _, err := projectExecutionSource(r.root, r.root, filepath.ToSlash(filepath.Join(pkg.Source, "workflow.yaml"))); err != nil {
		return nil, projectCompositionSourceError(pkg.Source, err)
	}
	folder, err := projectPackageSourceLocation(r.root, pkg.Source)
	if err != nil {
		return nil, err
	}
	if pkg.Origin != nil {
		digest, err := projectWorkflowTreeDigest(folder)
		if err != nil {
			return nil, err
		}
		if digest != pkg.Origin.Digest {
			return nil, refusal("project_composition_source_drift", name+": source differs from its pinned origin")
		}
	}
	return r.load(name, folder, effective, depth)
}

func (r *projectCompositionReader) load(name, folder string, effective bool, depth int) (*projectCompositionNode, error) {
	if depth > 8 {
		return nil, refusal("project_composition_limit", "parent/import depth exceeds 8 at "+name)
	}
	key := filepath.Clean(folder)
	modeKey := fmt.Sprintf("%s:%t", key, effective)
	if r.active[key] {
		return nil, refusal("project_composition_cycle", "source cycle at "+name)
	}
	if node := r.nodes[modeKey]; node != nil {
		if depth+node.height > 8 {
			return nil, refusal("project_composition_limit", "parent/import depth exceeds 8 at "+name)
		}
		return node, nil
	}
	if !r.folders[key] && len(r.folders) >= 32 {
		return nil, refusal("project_composition_limit", "source folder count exceeds 32")
	}
	r.folders[key] = true
	r.active[key] = true
	defer delete(r.active, key)
	// A flattened inventory cannot prove package ownership for point targets.
	// Refuse even an unused declaration before collecting any provider documents.
	if _, err := os.Lstat(filepath.Join(folder, projectExtensionPointsFile)); err == nil {
		return nil, refusal("project_composition_extension_points_unsupported", name+": extension-points.yaml cannot participate in composition")
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	source, err := readProjectWorkflowFolderSource(r.root, folder, effective)
	if err != nil {
		return nil, err
	}
	object := source.RootValue.(map[string]any)
	relativeFolder, err := projectRelativePath(r.root, folder)
	if err != nil {
		return nil, err
	}
	prefix := fmt.Sprintf("n%x", sha256.Sum256([]byte(relativeFolder)))[:17]
	n := &projectCompositionNode{source: source, name: name, key: key, prefix: prefix, locals: map[string]string{}, exports: map[string]string{}, imports: map[string]*projectCompositionNode{}, ancestors: map[string]*projectCompositionNode{}}
	r.nodes[modeKey] = n
	n.ancestors[key] = n
	for key, logical := range maps.Clone(n.source.References) {
		alias := prefix + "_ref_" + key
		n.locals[key] = alias
		delete(n.source.References, key)
		n.source.References[alias] = logical
	}
	if raw, exists := object["imports"]; exists {
		if object["authoring"] != projectWorkflowCompositionVersion {
			return nil, refusal("project_composition_marker_required", name+": imports require folder /2")
		}
		if err := r.readImports(n, raw, depth); err != nil {
			return nil, err
		}
	}
	for i := range n.source.Documents {
		doc := &n.source.Documents[i]
		value, err := r.document(doc.Source)
		if err != nil {
			return nil, err
		}
		component, ok := value.(map[string]any)
		if !ok {
			return nil, refusal("project_composition_invalid_component", doc.Source+": expected one object")
		}
		id, _ := component["id"].(string)
		short := projectExtensionWorkflowName(id)
		alias := doc.AliasPrefix + "_" + short
		if _, exists := n.locals[alias]; exists {
			return nil, refusal("project_composition_ambiguous_component", name+": "+alias)
		}
		doc.Alias = prefix + "_" + alias
		doc.Name = doc.Alias
		doc.Value = value
		n.locals[alias] = doc.Alias
	}
	if raw, exists := object["exports"]; exists {
		if object["authoring"] != projectWorkflowCompositionVersion {
			return nil, refusal("project_composition_marker_required", name+": exports require folder /2")
		}
		groups, ok := raw.(map[string]any)
		if !ok || len(groups) == 0 {
			return nil, refusal("project_composition_invalid_export", name+": exports must be a nonempty object")
		}
		for group, rawExports := range groups {
			kind := strings.TrimSuffix(group, "s")
			if group != "steps" && group != "workflows" {
				return nil, refusal("project_composition_invalid_export", name+": unknown export kind "+group)
			}
			exports, ok := rawExports.(map[string]any)
			if !ok || len(exports) == 0 {
				return nil, refusal("project_composition_invalid_export", name+": empty exports "+group)
			}
			for export, rawTarget := range exports {
				target, ok := rawTarget.(string)
				match := projectValueReference.FindStringSubmatch(target)
				if !ok || !projectValueName.MatchString(export) || match == nil || !strings.HasPrefix(match[1], kind+"_") || n.locals[match[1]] == "" {
					return nil, refusal("project_composition_invalid_export", name+"."+export+": export must name an owned "+kind)
				}
				if n.exports[export] != "" {
					return nil, refusal("project_composition_invalid_export", name+": duplicate export "+export)
				}
				owned := false
				for _, doc := range n.source.Documents {
					if doc.FolderRoot && doc.Alias == n.locals[match[1]] {
						return nil, refusal("project_composition_invalid_export", name+": export must name a component, not the launch root")
					}
					if !doc.FolderRoot && doc.Kind == kind && doc.Alias == n.locals[match[1]] {
						owned = true
					}
				}
				if !owned {
					return nil, refusal("project_composition_invalid_export", name+"."+export+": export must name an owned component")
				}
				n.exports[export] = n.locals[match[1]]
			}
		}
	}
	{
		if raw, exists := object["extends"]; exists {
			if object["authoring"] != projectWorkflowCompositionVersion {
				return nil, refusal("project_composition_marker_required", name+": extends requires folder /2")
			}
			parents := []any{raw}
			if list, ok := raw.([]any); ok {
				parents = list
			}
			if len(parents) == 0 || len(parents) > 32 {
				return nil, refusal("project_composition_invalid_parent", name+": extends requires one or more parents")
			}
			seen := map[string]bool{}
			for _, rawParent := range parents {
				parentName, ok := rawParent.(string)
				if !ok || !projectValueName.MatchString(parentName) || seen[parentName] {
					return nil, refusal("project_composition_invalid_parent", name+": invalid or repeated parent")
				}
				seen[parentName] = true
				parent, err := r.dependency(parentName, effective, depth+1)
				if err != nil {
					return nil, err
				}
				n.parents = append(n.parents, parent)
				n.parentNames = append(n.parentNames, parentName)
				maps.Copy(n.ancestors, parent.ancestors)
			}
			if effective {
				if err := n.inherit(); err != nil {
					return nil, err
				}
			} else {
				n.inheritDocuments()
			}
		}
	}
	for i := range n.source.Documents {
		doc := &n.source.Documents[i]
		if doc.Kind == "workflow" || doc.Kind == "step" {
			value := projectCompositionClone(doc.Value).(map[string]any)
			if refs, ok := value["refs"].(map[string]any); ok {
				for ref, raw := range refs {
					if name, ok := raw.(string); ok && projectCompositionQualified(name) {
						alias, err := n.target(name, "")
						if err != nil {
							return nil, projectCompositionSourceError(doc.Source, err)
						}
						refs[ref] = "{{" + alias + "}}"
					}
				}
			}
			doc.Value = value
		}
		doc.Value, err = n.rewrite(doc.Value)
		if err != nil {
			return nil, err
		}
	}
	if effective {
		if _, derived := object["extends"]; derived {
			if _, own := object["decision_catalog"]; own {
				var paths []string
				n.source.DecisionCatalog, paths, err = projectWorkflowFolderDecisionCatalog(r.root, folder, n.rootDocument().Value, n.source)
				if err != nil {
					return nil, err
				}
				for _, path := range paths {
					if err := r.record(path); err != nil {
						return nil, err
					}
				}
			}
		}
		declared := map[string]bool{}
		for _, document := range source.Documents {
			declared[document.Source] = true
			declared[document.Extensions] = true
		}
		for _, path := range source.ReadPaths {
			if !declared[path] {
				if err := r.record(path); err != nil {
					return nil, err
				}
			}
		}
		if err := r.readOptions(n, depth); err != nil {
			return nil, err
		}
		if err := projectCompositionArrangeNode(n); err != nil {
			return nil, err
		}
	}
	// A cached node keeps its longest remaining path, including effective
	// imports from layers and extend. Shared branches do not add their depths.
	for _, parent := range n.parents {
		n.height = max(n.height, 1+parent.height)
	}
	for _, provider := range n.imports {
		n.height = max(n.height, 1+provider.height)
	}
	return n, nil
}

func (r *projectCompositionReader) readImports(n *projectCompositionNode, raw any, depth int) error {
	imports, ok := raw.(map[string]any)
	if !ok || len(imports) == 0 {
		return refusal("project_composition_invalid_import", n.name+": imports must be a nonempty alias map")
	}
	if n.importTargets == nil {
		n.importTargets = map[string]string{}
	}
	for _, alias := range slices.Sorted(maps.Keys(imports)) {
		name, ok := imports[alias].(string)
		if !ok || !projectValueName.MatchString(alias) || !projectValueName.MatchString(name) {
			return refusal("project_composition_invalid_import", n.name+": duplicate or invalid import "+alias)
		}
		if previous := n.imports[alias]; previous != nil {
			if n.importTargets[alias] != name {
				return refusal("project_composition_invalid_import", n.name+": conflicting import "+alias)
			}
			continue
		}
		provider, err := r.dependency(name, false, depth+1)
		if err != nil {
			return err
		}
		n.imports[alias] = provider
		n.importTargets[alias] = name
	}
	return nil
}

func (n *projectCompositionNode) target(name, kind string) (string, error) {
	if projectCompositionQualified(name) {
		parts := strings.Split(name, ".")
		provider := n.imports[parts[0]]
		if provider == nil {
			return "", refusal("project_composition_unknown_import", n.name+": "+name)
		}
		alias := provider.exports[parts[1]]
		if alias == "" {
			return "", refusal("project_composition_private_export", n.name+": "+name+" is not exported")
		}
		if kind != "" && !strings.Contains(alias, "_"+kind+"_") {
			return "", refusal("project_composition_export_kind", n.name+": "+name+" is not a "+kind)
		}
		return alias, nil
	}
	if alias := n.locals[kind+"_"+name]; alias != "" {
		return alias, nil
	}
	return "", refusal("project_composition_unknown_component", n.name+": "+name)
}

func (n *projectCompositionNode) rewrite(value any) (any, error) {
	switch v := value.(type) {
	case string:
		if match := projectValueReference.FindStringSubmatch(v); match != nil {
			if alias := n.locals[match[1]]; alias != "" {
				return "{{" + alias + "}}", nil
			}
		}
		return value, nil
	case []any:
		result := make([]any, len(v))
		for i, child := range v {
			var err error
			result[i], err = n.rewrite(child)
			if err != nil {
				return nil, err
			}
		}
		return result, nil
	case map[string]any:
		result := map[string]any{}
		for key, child := range v {
			var err error
			result[key], err = n.rewrite(child)
			if err != nil {
				return nil, err
			}
		}
		return result, nil
	}
	return value, nil
}

func (n *projectCompositionNode) rootDocument() *projectPackageDocument {
	for i := range n.source.Documents {
		if n.source.Documents[i].FolderRoot {
			return &n.source.Documents[i]
		}
	}
	return nil
}

func (n *projectCompositionNode) inherit() error {
	common := map[string]*projectCompositionNode{}
	maps.Copy(common, n.parents[0].ancestors)
	for _, parent := range n.parents[1:] {
		for key := range common {
			if parent.ancestors[key] == nil {
				delete(common, key)
			}
		}
	}
	nearest := []*projectCompositionNode{}
	for key, candidate := range common {
		shadowed := false
		for otherKey, other := range common {
			if otherKey != key && other.ancestors[key] != nil {
				shadowed = true
			}
		}
		if !shadowed {
			nearest = append(nearest, candidate)
		}
	}
	if len(nearest) != 1 {
		return refusal("project_composition_incompatible_bases", n.name+": parents need one nearest exact common source ancestor")
	}
	base := nearest[0]
	rootDoc := n.rootDocument()
	owned := rootDoc.Value.(map[string]any)
	root := projectCompositionClone(base.rootDocument().Value).(map[string]any)
	if _, explicit := owned["limits"]; !explicit {
		limits := n.parents[0].rootDocument().Value.(map[string]any)["limits"]
		for _, parent := range n.parents[1:] {
			if !projectCompositionEqual(limits, parent.rootDocument().Value.(map[string]any)["limits"]) {
				return refusal("project_composition_setting_conflict", n.name+": parents disagree on limits; declare limits explicitly")
			}
		}
		root["limits"] = limits
	}
	for key, value := range owned {
		switch key {
		case "authoring", "package", "id", "version", "title", "extends", "imports", "exports", "limits", "execution_bindings", "decision_catalog":
			root[key] = value
		default:
			return refusal("project_composition_structural_override", n.name+": derived root cannot override "+key)
		}
	}
	rootDoc.Value = root
	for _, parent := range n.parents {
		for _, ext := range parent.options.Extensions {
			ext.Workflow = rootDoc.Name
			found := false
			for _, existing := range n.options.Extensions {
				if existing.Source == ext.Source {
					left, right := existing, ext
					left.Selector = ""
					right.Selector = ""
					if !projectCompositionEqual(left, right) {
						return refusal("project_composition_contribution_conflict", ext.Source)
					}
					found = true
				}
			}
			if !found {
				ext.Workflow = rootDoc.Name
				n.options.Extensions = append(n.options.Extensions, ext)
			}
		}
	}
	n.inheritDocuments()
	if len(n.source.Profiles) == 0 {
		var inherited map[string]map[string]any
		for _, parent := range n.parents {
			if len(parent.source.Profiles) == 0 {
				continue
			}
			if inherited != nil && !projectCompositionEqual(inherited, parent.source.Profiles) {
				return refusal("project_composition_setting_conflict", n.name+": parent package profiles disagree; declare package.profiles explicitly")
			}
			inherited = parent.source.Profiles
		}
	}
	// Root-only semantic fields belong to the common graph. Supporting
	// references and profiles remain available from every effective ancestor.
	n.source.DecisionCatalog = slices.Clone(base.source.DecisionCatalog)
	for _, parent := range n.parents {
		if n.source.References == nil {
			n.source.References = map[string]string{}
		}
		for key, value := range parent.source.References {
			if old, ok := n.source.References[key]; ok && old != value {
				return refusal("project_composition_reference_conflict", n.name+": "+key)
			}
			n.source.References[key] = value
		}
		if len(n.source.Profiles) == 0 {
			n.source.Profiles = parent.source.Profiles
			n.source.DefaultProfile = parent.source.DefaultProfile
		}
	}
	return nil
}

func (n *projectCompositionNode) inheritDocuments() {
	owned := n.locals
	locals := map[string]string{}
	seen := map[string]bool{}
	documents := []projectPackageDocument{}
	for _, parent := range n.parents {
		for key, alias := range parent.locals {
			if alias == parent.rootDocument().Alias {
				continue
			}
			if prior, exists := locals[key]; exists && prior != alias {
				locals[key] = ""
			} else if !exists {
				locals[key] = alias
			}
		}
		for _, doc := range parent.source.Documents {
			if !doc.FolderRoot && !seen[doc.Alias] {
				seen[doc.Alias] = true
				documents = append(documents, doc)
			}
		}
	}
	maps.Copy(locals, owned)
	n.locals = locals
	n.source.Documents = append(documents, n.source.Documents...)
}

func projectCompositionEqual(a, b any) bool {
	left, _ := projectCanonicalJSON(a)
	right, _ := projectCanonicalJSON(b)
	return string(left) == string(right)
}

func (r *projectCompositionReader) readOptions(n *projectCompositionNode, depth int) error {
	localOptions := projectWorkflowOptions{}
	path := filepath.Join(n.source.Folder, "extend.yaml")
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return n.mergeParentOptions(projectWorkflowOptions{})
	} else if err != nil {
		return err
	}
	relative, err := projectRelativePath(r.root, path)
	if err != nil {
		return err
	}
	value, err := r.document(relative)
	if err != nil {
		return err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return refusal("project_composition_invalid_layer", relative+": expected object")
	}
	if err := projectCompositionCheckPointBoundary(object, relative); err != nil {
		return err
	}
	marked := object["authoring"] == projectCompositionExtensionVersion
	if raw, exists := object["layers"]; exists {
		if !marked {
			return refusal("project_composition_marker_required", relative+": layers require /3")
		}
		layers, ok := raw.([]any)
		if !ok || len(layers) == 0 {
			return refusal("project_composition_invalid_layer", relative+": layers must be a nonempty list")
		}
		for _, rawPath := range layers {
			name, ok := rawPath.(string)
			if !ok || name == "" || filepath.IsAbs(name) {
				return refusal("project_composition_invalid_layer", relative+": invalid layer path")
			}
			layerPath := filepath.Join(filepath.Dir(path), filepath.FromSlash(name))
			layerRelative, err := projectRelativePath(r.root, layerPath)
			if err != nil {
				return err
			}
			// Common ancestors return from the node cache before readOptions.
			// Reaching an existing layer here is a new explicit declaration.
			if r.layers[layerRelative] {
				return refusal("project_composition_duplicate_layer", layerRelative)
			}
			if len(r.layers) >= 32 {
				return refusal("project_composition_limit", "layer count exceeds 32")
			}
			r.layerOrder = append(r.layerOrder, layerRelative)
			r.layers[layerRelative] = true
			layer, err := r.document(layerRelative)
			if err != nil {
				return err
			}
			layerObject, ok := layer.(map[string]any)
			if err := projectCompositionCheckPointBoundary(layerObject, layerRelative); err != nil {
				return err
			}
			if !ok || layerObject["authoring"] != projectCompositionExtensionVersion {
				return refusal("project_composition_invalid_layer", layerRelative+": layer requires /3")
			}
			for _, key := range []string{"layers", "extends", "arrange"} {
				if _, exists := layerObject[key]; exists {
					return refusal("project_composition_invalid_layer", layerRelative+": nested "+key+" is forbidden")
				}
			}
			options, err := r.optionsDocument(n, layerObject, layerRelative, depth)
			if err != nil {
				return err
			}
			localOptions = projectCompositionMergeOptions(localOptions, options)
		}
	}
	options, err := r.optionsDocument(n, object, relative, depth)
	if err != nil {
		return err
	}
	n.groups, _ = object["arrange"].([]any)
	if _, exists := object["arrange"]; exists && (!marked || n.groups == nil || len(n.groups) == 0) {
		return refusal("project_composition_invalid_group", relative+": arrange must be a nonempty /3 list")
	}
	return n.mergeParentOptions(projectCompositionMergeOptions(localOptions, options))
}

func (r *projectCompositionReader) optionsDocument(n *projectCompositionNode, object map[string]any, source string, depth int) (projectWorkflowOptions, error) {
	if err := projectCompositionCheckPointBoundary(object, source); err != nil {
		return projectWorkflowOptions{}, err
	}
	marked := object["authoring"] == projectCompositionExtensionVersion
	if raw, exists := object["imports"]; exists {
		if !marked {
			return projectWorkflowOptions{}, refusal("project_composition_marker_required", source+": imports require /3")
		}
		if err := r.readImports(n, raw, depth); err != nil {
			return projectWorkflowOptions{}, err
		}
	}
	copy := projectCompositionClone(object).(map[string]any)
	delete(copy, "imports")
	delete(copy, "layers")
	delete(copy, "arrange")
	data, _ := json.Marshal(copy)
	options, err := parseProjectWorkflowOptions(data)
	if err != nil {
		return options, refusal("project_composition_invalid_layer", source+": "+err.Error())
	}
	if _, exists := copy["exclude"]; !exists {
		options.Exclude = nil
	}
	for i := range options.Extensions {
		ext := &options.Extensions[i]
		if ext.Bindings != nil {
			rewritten, err := n.rewrite(ext.Bindings)
			if err != nil {
				return options, err
			}
			ext.Bindings = rewritten.(map[string]any)
		}
		ext.Source = source + "#" + ext.ID
		ext.Selector = n.name + "." + ext.ID
		ext.Composed = marked
		ext.StageID = ext.Step
		if marked || ext.Call != "" {
			ext.StageID = ext.ID
		}
		if ext.Workflow == "root" && marked {
			ext.Workflow = n.rootDocument().Name
		} else {
			if ext.Workflow != projectExtensionWorkflowName(n.rootDocument().Value.(map[string]any)["id"].(string)) {
				return options, refusal("project_extension_unknown_workflow", source+": "+ext.Workflow)
			}
			ext.Workflow = n.rootDocument().Name
		}
		if ext.Call != "" {
			ext.Call, err = n.target(ext.Call, "workflow")
		} else {
			ext.Step, err = n.target(ext.Step, "step")
		}
		if err != nil {
			return options, err
		}
	}
	settings := map[string]map[string]any{}
	for name, inputs := range options.Settings {
		target := n.rootDocument().Name
		if name != "root" || !marked {
			target, err = n.target(name, "workflow")
			if err != nil {
				return options, err
			}
		}
		settings[target] = inputs
		for input, value := range inputs {
			n.settings = append(n.settings, projectCompositionSetting{Target: target, Input: input, Source: source + "/settings/" + name + "/" + input, Value: value})
		}
	}
	options.Settings = settings
	if n.referenceOrigins == nil {
		n.referenceOrigins = map[string]string{}
	}
	for alias := range options.References {
		if declared := n.locals[alias]; declared != "" {
			return options, refusal("project_composition_reference_conflict", source+"/references/"+alias+" collides with the package declaration")
		}
		if previous := n.referenceOrigins[alias]; previous != "" {
			return options, refusal("project_composition_reference_conflict", previous+" and "+source+"/references/"+alias+" repeat one reference alias")
		}
		n.referenceOrigins[alias] = source + "/references/" + alias
	}
	bindings := map[string]any{}
	programs := map[string]any{}
	programTargets := map[string][]string{}
	for name, config := range options.ExecutionBindings {
		if projectCompositionQualified(name) {
			return options, refusal("project_composition_program_override", source+": imported programs belong to their provider")
		}
		target, err := n.target(name, "step")
		if err != nil {
			return options, err
		}
		bindings[target] = config
		for _, doc := range n.source.Documents {
			if doc.Name == target {
				id, _ := doc.Value.(map[string]any)["id"].(string)
				programs[id] = config
				programTargets["step "+id] = append(programTargets["step "+id], doc.Name)
			}
		}
	}
	options.ExecutionBindings = bindings
	if len(programs) != 0 {
		n.programs = append(n.programs, projectCompositionProgram{Folder: filepath.Join(r.root, filepath.Dir(filepath.FromSlash(source))), Value: map[string]any{"steps": programs}, Extension: true, Targets: programTargets})
	}
	return options, nil
}

func projectCompositionMergeOptions(base, overlay projectWorkflowOptions) projectWorkflowOptions {
	base.Authoring = projectCompositionExtensionVersion
	base.Extensions = append(slices.Clone(base.Extensions), overlay.Extensions...)
	settings := map[string]map[string]any{}
	for key, values := range base.Settings {
		settings[key] = maps.Clone(values)
	}
	for key, values := range overlay.Settings {
		if settings[key] == nil {
			settings[key] = map[string]any{}
		}
		maps.Copy(settings[key], values)
	}
	base.Settings = settings
	if overlay.Exclude != nil {
		base.Exclude = slices.Clone(overlay.Exclude)
	}
	if overlay.Profile != "" {
		base.Profile = overlay.Profile
	}
	if overlay.Answers.DecisionPolicy != "" {
		base.Answers.DecisionPolicy = overlay.Answers.DecisionPolicy
	}
	base.Answers.Preflight = projectCompositionMapMerge(base.Answers.Preflight, overlay.Answers.Preflight)
	base.Answers.Runtime = projectCompositionMapMerge(base.Answers.Runtime, overlay.Answers.Runtime)
	base.References = projectCompositionMapMerge(base.References, overlay.References)
	base.Resources = projectCompositionMapMerge(base.Resources, overlay.Resources)
	base.ExecutionBindings = projectCompositionMapMerge(base.ExecutionBindings, overlay.ExecutionBindings)
	profiles := map[string]map[string]map[string]string{}
	for host, records := range base.ModelProfiles {
		profiles[host] = maps.Clone(records)
	}
	for host, records := range overlay.ModelProfiles {
		if profiles[host] == nil {
			profiles[host] = map[string]map[string]string{}
		}
		maps.Copy(profiles[host], records)
	}
	base.ModelProfiles = profiles
	return base
}

func projectCompositionMapMerge[T any](base, overlay map[string]T) map[string]T {
	result := map[string]T{}
	maps.Copy(result, base)
	maps.Copy(result, overlay)
	return result
}

func (n *projectCompositionNode) mergeParentOptions(own projectWorkflowOptions) error {
	if packageValue, ok := n.source.RootValue.(map[string]any)["package"].(map[string]any); ok {
		if _, declared := packageValue["profiles"]; declared && own.Profile == "" {
			own.Profile = n.source.DefaultProfile
		}
	}
	data, err := json.Marshal(own)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &n.ownOptions); err != nil {
		return err
	}
	merged, err := projectCompositionResolveOptions(n)
	if err != nil {
		return err
	}
	localSettings := n.settings
	n.settings = nil
	seenSettings := map[string]bool{}
	for _, parent := range n.parents {
		for _, setting := range parent.settings {
			if setting.Target == parent.rootDocument().Name {
				setting.Target = n.rootDocument().Name
			}
			key := setting.Source + "\x00" + setting.Target
			if !seenSettings[key] {
				n.settings = append(n.settings, setting)
				seenSettings[key] = true
			}
		}
	}
	n.settings = append(n.settings, localSettings...)
	// Contributions have already been deduplicated by source identity.
	extensions := append(slices.Clone(n.options.Extensions), own.Extensions...)
	n.options = projectCompositionMergeOptions(merged, projectWorkflowOptions{Extensions: extensions})
	return nil
}

func (r *projectCompositionReader) finish(n *projectCompositionNode) (projectPackageSource, error) {
	if n.options.Exclude == nil {
		n.options.Exclude = []string{}
	}
	composition := &projectComposition{Options: n.options, CompositionReview: projectwire.CompositionReview{SchemaVersion: "workflow-composition/1", Files: r.files, Parents: []string{}, Sources: []string{}, Imports: map[string]string{}, Layers: append([]string{}, slices.Sorted(maps.Keys(r.layers))...), Contributions: []string{}, Groups: []projectwire.CompositionGroupReview{}}}
	for _, node := range r.nodes {
		composition.Programs = append(composition.Programs, node.programs...)
		relative, err := projectRelativePath(r.root, node.source.Folder)
		if err != nil {
			return n.source, err
		}
		composition.Sources = append(composition.Sources, relative)
	}
	slices.Sort(composition.Sources)
	composition.Sources = slices.Compact(composition.Sources)
	composition.Layers = append([]string{}, r.layerOrder...)
	composition.Settings = n.settings
	composition.Labels = map[string]string{}
	composition.ExportTargets = map[string]string{}
	for _, node := range r.nodes {
		for _, doc := range node.source.Documents {
			if strings.HasPrefix(doc.Alias, node.prefix+"_") {
				id, _ := doc.Value.(map[string]any)["id"].(string)
				composition.Labels[doc.Name] = node.name + "." + projectExtensionWorkflowName(id)
			}
		}
		for alias, provider := range node.imports {
			key := node.name + "." + alias
			if node == n {
				key = alias
			}
			composition.Imports[key] = node.importTargets[alias]
			for export, target := range provider.exports {
				composition.ExportTargets[key+"."+export] = target
				composition.Labels[target] = key + "." + export
			}
		}
	}
	composition.Labels[n.rootDocument().Name] = "root"
	composition.Parents = append(composition.Parents, n.parentNames...)
	for alias := range n.imports {
		composition.Imports[alias] = n.importTargets[alias]
	}
	for _, ext := range n.options.Extensions {
		composition.Contributions = append(composition.Contributions, ext.Source)
		if ext.Group != nil {
			members := []string{}
			for _, leaf := range projectCompositionLeaves(ext.Group.Members) {
				members = append(members, leaf.Selector)
			}
			composition.Groups = append(composition.Groups, projectwire.CompositionGroupReview{ID: ext.ID, Mode: ext.Group.Kind, Members: members})
		}
	}
	available := map[string]projectPackageDocument{}
	for _, node := range r.nodes {
		for _, doc := range node.source.Documents {
			available[doc.Alias] = doc
		}
	}
	for _, doc := range n.source.Documents {
		available[doc.Alias] = doc
	}
	rootDoc := *n.rootDocument()
	rootDoc.Extensions = rootDoc.Source // compiler insertion marker; no file is read here
	available[rootDoc.Alias] = rootDoc
	selected := map[string]bool{}
	active := map[string]bool{}
	ordered := []projectPackageDocument{}
	var add func(string) error
	add = func(alias string) error {
		if active[alias] {
			return refusal("project_composition_cycle", alias)
		}
		if selected[alias] {
			return nil
		}
		doc, exists := available[alias]
		if !exists {
			return refusal("project_composition_unknown_component", alias)
		}
		active[alias] = true
		for _, dependency := range projectCompositionPlaceholders(doc.Value) {
			if _, exists := available[dependency]; exists {
				if err := add(dependency); err != nil {
					return err
				}
			}
		}
		delete(active, alias)
		selected[alias] = true
		doc.Extensions = ""
		if alias == rootDoc.Alias {
			doc.FolderRoot = true
			doc.Extensions = rootDoc.Extensions
		} else {
			if doc.FolderRoot {
				value, err := projectFolderWorkflowDefinition(doc.Value)
				if err != nil {
					return err
				}
				doc.Value = value
			}
			doc.FolderRoot = false
		}
		ordered = append(ordered, doc)
		return nil
	}
	if err := add(rootDoc.Alias); err != nil {
		return n.source, err
	}
	for _, ext := range projectCompositionLeaves(n.options.Extensions) {
		target := ext.Call
		if target == "" {
			target = ext.Step
		}
		if err := add(target); err != nil {
			return n.source, err
		}
		for _, dependency := range projectCompositionPlaceholders(ext.Bindings) {
			if _, exists := available[dependency]; exists {
				if err := add(dependency); err != nil {
					return n.source, err
				}
			}
		}
	}
	for _, node := range r.nodes {
		root := node.source.RootValue.(map[string]any)
		if programs, exists := root["execution_bindings"]; exists {
			rewritten, err := node.rewrite(programs)
			if err != nil {
				return n.source, err
			}
			composition.Programs = append(composition.Programs, projectCompositionProgram{Folder: node.source.Folder, Value: rewritten, Targets: projectCompositionProgramTargets(node.source.Documents)})
		}
		for key, logical := range node.source.References {
			if previous, ok := n.source.References[key]; ok && previous != logical {
				return n.source, refusal("project_composition_reference_conflict", key)
			}
			n.source.References[key] = logical
		}
		n.source.RequestedCapabilities = append(n.source.RequestedCapabilities, node.source.RequestedCapabilities...)
		n.source.Dependencies = append(n.source.Dependencies, node.source.Dependencies...)
	}
	slices.Sort(n.source.RequestedCapabilities)
	n.source.RequestedCapabilities = slices.Compact(n.source.RequestedCapabilities)
	slices.Sort(n.source.Dependencies)
	n.source.Dependencies = slices.Compact(n.source.Dependencies)
	n.source.Documents = ordered
	slices.SortStableFunc(n.source.Documents, func(a, b projectPackageDocument) int {
		if a.FolderRoot == b.FolderRoot {
			return 0
		}
		if a.FolderRoot {
			return 1
		}
		return -1
	})
	n.source.RootValue = rootDoc.Value
	n.source.ReadPaths = slices.Sorted(maps.Keys(r.files))
	n.source.Composition = composition
	return n.source, nil
}

func projectCompositionPlaceholders(value any) []string {
	result := []string{}
	var walk func(any)
	walk = func(value any) {
		switch v := value.(type) {
		case string:
			if match := projectValueReference.FindStringSubmatch(v); match != nil {
				result = append(result, match[1])
			}
		case []any:
			for _, child := range v {
				walk(child)
			}
		case map[string]any:
			for _, child := range v {
				walk(child)
			}
		}
	}
	walk(value)
	slices.Sort(result)
	return slices.Compact(result)
}

func projectCompositionComponentNamed(component projectCompileComponent, name string) bool {
	if component.Name == "" {
		return projectExtensionWorkflowName(component.Ref.ID) == name
	}
	return component.Name == name || slices.Contains(component.Names, name)
}

func projectCompositionRenderBindings(extensions []projectWorkflowExtension, values map[string]any) ([]projectWorkflowExtension, error) {
	result := slices.Clone(extensions)
	for index := range result {
		extension := &result[index]
		if extension.Bindings != nil {
			rendered, missing, err := projectSubstituteValue(extension.Bindings, values)
			if err != nil {
				return nil, err
			}
			if len(missing) != 0 {
				return nil, refusal("project_compile_unresolved_values", strings.Join(missing, ", "))
			}
			extension.Bindings = rendered.(map[string]any)
		}
		if extension.Group != nil {
			group := *extension.Group
			members, err := projectCompositionRenderBindings(group.Members, values)
			if err != nil {
				return nil, err
			}
			group.Members = members
			extension.Group = &group
		}
	}
	return result, nil
}

func projectCompositionExplain(source projectPackageSource, options projectWorkflowOptions, components []projectCompileComponent) []string {
	composition := source.Composition
	result := []string{"Root: " + source.ID + "@" + source.Version, "Parents: " + strings.Join(composition.Parents, ", "), "Sources (each once): " + strings.Join(composition.Sources, ", ")}
	for _, alias := range slices.Sorted(maps.Keys(composition.Imports)) {
		result = append(result, "Import "+alias+": "+composition.Imports[alias])
	}
	for _, layer := range composition.Layers {
		result = append(result, "Layer: "+layer)
	}
	for _, group := range composition.Groups {
		result = append(result, "Group "+group.ID+" ("+group.Mode+"): "+strings.Join(group.Members, ", "))
	}
	for _, label := range slices.Sorted(maps.Keys(composition.ExportTargets)) {
		target := composition.ExportTargets[label]
		for _, component := range components {
			if projectCompositionComponentNamed(component, target) {
				result = append(result, "Export "+label+" = "+component.Ref.String())
			}
		}
	}
	for _, extension := range options.Extensions {
		result = append(result, "Contribution "+extension.Selector+" from "+extension.Source+": "+extension.From+" -> "+extension.ID+" -> "+extension.To)
		routes, _ := json.Marshal(extension.On)
		result = append(result, "  routes "+string(routes)+"; on_error="+extension.OnError)
	}
	for _, target := range slices.Sorted(maps.Keys(options.Settings)) {
		label := composition.Labels[target]
		if label == "" {
			label = target
		}
		for _, key := range slices.Sorted(maps.Keys(options.Settings[target])) {
			data, _ := json.Marshal(options.Settings[target][key])
			result = append(result, "Setting "+label+"."+key+" = "+string(data))
			for _, setting := range composition.Settings {
				if setting.Target == target && setting.Input == key {
					value, _ := json.Marshal(setting.Value)
					result = append(result, "  "+setting.Source+" = "+string(value))
				}
			}
		}
	}
	result = append(result, "Every contribution is invoked. Its workflow decides applicability; no skip is assumed before execution.")
	return result
}

func projectCompositionSourceError(source string, err error) error {
	var fault *prifly.Fault
	if errors.As(err, &fault) {
		copy := *fault
		copy.Message = source + ": " + copy.Message
		return &copy
	}
	return &prifly.Fault{Code: "project_composition_invalid", Message: source + ": composition failed", Cause: err}
}

// Ownership is the declaring source inventory, before imported documents are
// flattened into the final closure. A private document can remain unselected.
func projectCompositionProgramTargets(documents []projectPackageDocument) map[string][]string {
	targets := map[string][]string{}
	for _, doc := range documents {
		if doc.Kind != "step" && doc.Kind != "check" {
			continue
		}
		id, _ := doc.Value.(map[string]any)["id"].(string)
		key := doc.Kind + " " + id
		targets[key] = append(targets[key], doc.Name)
	}
	return targets
}

func projectReadCompositionExecution(root string, source projectPackageSource, components []projectCompileComponent, values map[string]any, projectBindings map[string]any) (*projectPackageExecution, error) {
	result := &projectPackageExecution{SchemaVersion: projectExecutionVersion, Bindings: []prifly.ExecutionBinding{}}
	seen := map[flow.Ref]prifly.ExecutionBinding{}
	total := 0
	programs := slices.Clone(source.Composition.Programs)
	slices.SortFunc(programs, func(a, b projectCompositionProgram) int {
		if a.Extension != b.Extension {
			if a.Extension {
				return 1
			}
			return -1
		}
		return strings.Compare(a.Folder, b.Folder)
	})
	for _, program := range programs {
		groups, ok := program.Value.(map[string]any)
		if !ok {
			return nil, refusal("project_execution_invalid", program.Folder+": expected binding groups")
		}
		filtered := map[string]any{}
		ownedComponents := []projectCompileComponent{}
		selected := map[flow.Ref]bool{}
		for group, raw := range groups {
			if group != "steps" && group != "checks" {
				return nil, refusal("project_execution_invalid", "unknown binding group "+group)
			}
			bindings, ok := raw.(map[string]any)
			if !ok {
				return nil, refusal("project_execution_invalid", "binding group must be an object")
			}
			entries := map[string]any{}
			for id, config := range bindings {
				kind := strings.TrimSuffix(group, "s")
				aliases := program.Targets[kind+" "+id]
				if len(aliases) == 0 {
					return nil, refusal("project_execution_invalid", program.Folder+": unknown owned "+kind+" "+id)
				}
				for _, component := range components {
					if component.Ref.ID != id || component.Kind != kind {
						continue
					}
					owned := slices.ContainsFunc(aliases, func(alias string) bool { return projectCompositionComponentNamed(component, alias) })
					if owned {
						entries[id] = config
						if !selected[component.Ref] {
							ownedComponents = append(ownedComponents, component)
							selected[component.Ref] = true
						}
					}
				}
			}
			if len(entries) != 0 {
				filtered[group] = entries
			}
		}
		if len(filtered) == 0 {
			continue
		}
		part := source
		part.Composition = nil
		part.Folder = program.Folder
		part.RootValue = map[string]any{"execution_bindings": filtered}
		payload, err := projectReadExecution(root, part, ownedComponents, values, nil)
		if err != nil {
			return nil, err
		}
		if payload == nil {
			continue
		}
		for _, binding := range payload.Bindings {
			if old, exists := seen[binding.DefinitionRef]; exists {
				if program.Extension || !projectCompositionEqual(old, binding) {
					return nil, refusal("project_composition_program_conflict", binding.DefinitionRef.ID)
				}
			} else {
				seen[binding.DefinitionRef] = binding
				for _, data := range binding.Files {
					total += len(data)
				}
				if total > prifly.MaxArtifactBytes {
					return nil, refusal("project_execution_invalid", "composition supporting files exceed byte limit")
				}
				result.Bindings = append(result.Bindings, binding)
			}
			// Exact shared programs emit once, but every read source path
			// belongs in the review and freshness provenance.
			for name := range binding.Files {
				path, err := projectRelativePath(root, filepath.Join(program.Folder, filepath.FromSlash(name)))
				if err != nil {
					return nil, err
				}
				source.Composition.Files[path] = fmt.Sprintf("sha256:%x", sha256.Sum256(binding.Files[name]))
			}
		}
	}
	if len(result.Bindings) == 0 {
		return nil, nil
	}
	slices.SortFunc(result.Bindings, func(a, b prifly.ExecutionBinding) int {
		return strings.Compare(a.DefinitionRef.String(), b.DefinitionRef.String())
	})
	if err := projectValidateExecution(result); err != nil {
		return nil, err
	}
	return result, nil
}

func projectCompositionCheckPointBoundary(object map[string]any, source string) error {
	unsupported := object["authoring"] == projectPointExtensionVersion
	if entries, ok := object["extensions"].([]any); ok {
		for _, raw := range entries {
			if entry, ok := raw.(map[string]any); ok {
				if _, present := entry["point"]; present {
					unsupported = true
				}
			}
		}
	}
	if unsupported {
		return refusal("project_composition_extension_points_unsupported", source+": effective /4 and point entries cannot participate in composition")
	}
	return nil
}
