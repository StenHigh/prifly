package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/stenhigh/prifly/internal/flow"
	prifly "github.com/stenhigh/prifly/internal/runtime"
	"go.yaml.in/yaml/v3"
)

const projectPackageSourceVersion = "prifly-package-source/1"
const projectWorkflowFolderVersion = "prifly-project-workflow/1"
const projectDecisionCatalogFile = "decisions.json"

var projectValueName = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
var projectValueReference = regexp.MustCompile(`^\{\{([a-z][a-z0-9_-]*)\}\}$`)

type projectPackageSource struct {
	ID                    string
	Version               string
	Description           string
	License               string
	RequiresCoreProtocol  string
	RequestedCapabilities []string
	Dependencies          []string
	References            map[string]string
	DefaultProfile        string
	Profiles              map[string]map[string]any
	DecisionCatalog       []prifly.DecisionDefinition
	Documents             []projectPackageDocument
	RootValue             any
	ResolvedDependencies  []flow.Ref
	Build                 *projectBuildProvenance
}

type projectPackageDocument struct {
	Kind         string
	Source       string
	AliasPrefix  string
	MediaType    string
	ID           string
	Version      string
	Extensions   string
	FolderRoot   bool
	FolderMember bool
}

type projectCompileComponent struct {
	Kind      string                `json:"kind"`
	Ref       flow.Ref              `json:"ref"`
	Path      string                `json:"path"`
	Bytes     []byte                `json:"-"`
	Resource  *flow.ContextResource `json:"-"`
	Root      bool                  `json:"-"`
	AuthorRef flow.Ref              `json:"-"`
}

type projectPendingDocument struct {
	document projectPackageDocument
	value    any
	index    int
}

type projectWorkflowFeature struct {
	Workflow string
	Input    string
}

type projectCompileResult struct {
	SchemaVersion string                    `json:"schema_version"`
	Repository    string                    `json:"repository"`
	Package       flow.Ref                  `json:"package"`
	Output        string                    `json:"output"`
	Components    []projectCompileComponent `json:"components"`
	AuthorPackage *projectBuildIdentity     `json:"author_package,omitempty"`
	BuildKey      string                    `json:"build_key,omitempty"`
}

func (c *cli) projectCompile(ctx context.Context, args []string) error {
	f := flags("project compile")
	repository := f.String("repository", ".", "Git repository that owns the shared Pri-Fly profile")
	name := f.String("package", "", "named package from project.yaml")
	output := f.String("output", "", "new directory outside the repository and local authority")
	host := f.String("host", "", "host entry point that selects project skills")
	packageProfile := f.String("package-profile", "", "per-compilation package profile")
	values := stringsFlag{}
	f.Var(&values, "value", "explicit YAML value NAME=JSON")
	if err := parse(f, args); err != nil {
		return err
	}
	if *name == "" || *output == "" || *host == "" {
		return usageError("project compile requires --package, --host and --output")
	}
	root, err := projectRepositoryRoot(ctx, *repository)
	if err != nil {
		return err
	}
	profile, err := readProjectProfile(root)
	if err != nil {
		return err
	}
	pkg, exists := profile.Packages[*name]
	if !exists {
		return usageError("project_compile_unknown_package: " + *name)
	}
	skillsRoot, err := profile.skillsRoot(*host)
	if err != nil {
		return err
	}
	skillsRoot, err = canonicalProjectPath(filepath.Join(root, filepath.FromSlash(skillsRoot)))
	if err != nil {
		return err
	}
	if relative, err := filepath.Rel(root, skillsRoot); err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return usageError("project_compile_invalid_host_root: selected host skills root must stay inside the repository")
	}
	outputRoot, err := canonicalProjectPath(*output)
	if err != nil {
		return err
	}
	if projectPathsOverlap(root, outputRoot) || projectPathsOverlap(c.project, outputRoot) {
		return usageError("project_compile_unsafe_output: output must stay outside the repository and local authority")
	}
	if _, err := os.Lstat(outputRoot); err == nil {
		return usageError("project_compile_output_exists: output directory was not overwritten")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	compiledValues, err := parseProjectCompileValues(values)
	if err != nil {
		return err
	}
	engine, err := prifly.Open(c.project, true)
	if err != nil {
		return err
	}
	defer engine.Close()
	_, registry, err := engine.Inventory()
	if err != nil {
		return err
	}
	packages, err := engine.Packages(ctx)
	if err != nil {
		return err
	}
	sourcePath, err := projectPackageSourceLocation(root, pkg.Source)
	if err != nil {
		return err
	}
	source, err := readProjectWorkflowFolder(root, sourcePath)
	if err != nil {
		return err
	}
	options, err := projectReadWorkflowOptions(root, source, compiledValues)
	if err != nil {
		return err
	}
	for alias, logical := range source.References {
		ref, err := projectLogicalRef(registry, logical)
		if err != nil {
			return usageError("project_compile_reference " + alias + ": " + err.Error())
		}
		if _, exists := compiledValues[alias]; exists {
			return usageError("project_compile_duplicate_value: " + alias)
		}
		compiledValues[alias] = projectRefValue(ref)
	}
	selectedProfile := options.Profile
	if *packageProfile != "" {
		selectedProfile = *packageProfile
	}
	if err := projectApplyPackageProfile(source, selectedProfile, compiledValues); err != nil {
		return err
	}
	if err := os.Mkdir(outputRoot, 0755); err != nil {
		return err
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(outputRoot)
		}
	}()
	result, err := compileAndSealProjectPackage(root, skillsRoot, outputRoot, profile.SchemaVersion, selectedProfile, source, registry, packages, compiledValues, options)
	if err != nil {
		return err
	}
	complete = true
	return c.emit(result)
}

func parseProjectCompileValues(values []string) (map[string]any, error) {
	result := make(map[string]any, len(values))
	for _, raw := range values {
		name, value, ok := strings.Cut(raw, "=")
		if !ok || !projectValueName.MatchString(name) || value == "" {
			return nil, usageError("project_compile_invalid_value: expected unique NAME=JSON")
		}
		if _, exists := result[name]; exists {
			return nil, usageError("project_compile_invalid_value: expected unique NAME=JSON")
		}
		parsed, err := flow.Parse([]byte(value), "json")
		if err != nil {
			return nil, usageError("project_compile_invalid_value: " + err.Error())
		}
		result[name] = parsed
	}
	return result, nil
}

func parseProjectPackageSource(value any) (projectPackageSource, error) {
	object, ok := value.(map[string]any)
	if !ok {
		return projectPackageSource{}, usageError("project_package_invalid: package source must be an object")
	}
	allowed := map[string]bool{"schema_version": true, "id": true, "version": true, "description": true, "license": true, "requires_core_protocol": true, "requested_capabilities": true, "dependencies": true, "references": true, "profiles": true, "documents": true}
	for key := range object {
		if !allowed[key] {
			return projectPackageSource{}, usageError("project_package_invalid: unknown field " + key)
		}
	}
	text := func(name string, required bool) (string, error) {
		value, exists := object[name]
		if !exists && !required {
			return "", nil
		}
		result, ok := value.(string)
		if !ok || result == "" {
			return "", usageError("project_package_invalid: " + name + " must be a non-empty string")
		}
		return result, nil
	}
	result := projectPackageSource{References: map[string]string{}, Profiles: map[string]map[string]any{}, RequestedCapabilities: []string{}, Dependencies: []string{}}
	for _, field := range []struct {
		name     string
		target   *string
		required bool
	}{{"id", &result.ID, true}, {"version", &result.Version, true}, {"description", &result.Description, true}, {"license", &result.License, false}, {"requires_core_protocol", &result.RequiresCoreProtocol, true}} {
		value, err := text(field.name, field.required)
		if err != nil {
			return projectPackageSource{}, err
		}
		*field.target = value
	}
	if result.License == "" {
		result.License = "MIT"
	}
	if version, _ := object["schema_version"].(string); version != projectPackageSourceVersion {
		return projectPackageSource{}, usageError("project_package_invalid: schema_version must be " + projectPackageSourceVersion)
	}
	if _, err := projectIdentity(result.ID, result.Version); err != nil {
		return projectPackageSource{}, usageError("project_package_invalid: " + err.Error())
	}
	for _, field := range []string{"requested_capabilities", "dependencies"} {
		if raw, exists := object[field]; exists {
			items, ok := raw.([]any)
			if !ok {
				return projectPackageSource{}, usageError("project_package_invalid: " + field + " must be a list")
			}
			values := make([]string, 0, len(items))
			for _, item := range items {
				value, ok := item.(string)
				if !ok || value == "" {
					return projectPackageSource{}, usageError("project_package_invalid: " + field + " values must be non-empty strings")
				}
				values = append(values, value)
			}
			if field == "dependencies" {
				result.Dependencies = values
			} else {
				result.RequestedCapabilities = values
			}
		}
	}
	if raw, exists := object["references"]; exists {
		references, ok := raw.(map[string]any)
		if !ok {
			return projectPackageSource{}, usageError("project_package_invalid: references must be an object")
		}
		for name, raw := range references {
			logical, ok := raw.(string)
			if !projectValueName.MatchString(name) || !ok || logical == "" {
				return projectPackageSource{}, usageError("project_package_invalid: references require valid names and logical refs")
			}
			result.References[name] = logical
		}
	}
	if raw, exists := object["profiles"]; exists {
		profiles, ok := raw.(map[string]any)
		if !ok || len(profiles) != 2 {
			return projectPackageSource{}, usageError("project_package_invalid: profiles requires default and values")
		}
		defaultName, ok := profiles["default"].(string)
		if !ok || !projectValueName.MatchString(defaultName) {
			return projectPackageSource{}, usageError("project_package_invalid: profiles default must be a valid name")
		}
		values, ok := profiles["values"].(map[string]any)
		if !ok || len(values) == 0 {
			return projectPackageSource{}, usageError("project_package_invalid: profiles values must be a non-empty object")
		}
		for name, rawValues := range values {
			fields, ok := rawValues.(map[string]any)
			if !ok || len(fields) == 0 || !projectValueName.MatchString(name) {
				return projectPackageSource{}, usageError("project_package_invalid: each profile requires a valid name and non-empty values")
			}
			for field := range fields {
				if !projectValueName.MatchString(field) {
					return projectPackageSource{}, usageError("project_package_invalid: profile values require valid names")
				}
			}
			result.Profiles[name] = fields
		}
		if _, exists := result.Profiles[defaultName]; !exists {
			return projectPackageSource{}, usageError("project_package_invalid: profiles default must name a declared profile")
		}
		result.DefaultProfile = defaultName
	}
	rawDocuments, exists := object["documents"]
	if !exists {
		return projectPackageSource{}, usageError("project_package_invalid: documents is required")
	}
	documents, ok := rawDocuments.([]any)
	if !ok || len(documents) == 0 {
		return projectPackageSource{}, usageError("project_package_invalid: documents must be a non-empty list")
	}
	for index, raw := range documents {
		object, ok := raw.(map[string]any)
		if !ok {
			return projectPackageSource{}, usageError(fmt.Sprintf("project_package_invalid: documents/%d must be an object", index))
		}
		allowed := map[string]bool{"kind": true, "source": true, "alias_prefix": true, "media_type": true, "id": true, "version": true, "extensions": true}
		for key := range object {
			if !allowed[key] {
				return projectPackageSource{}, usageError(fmt.Sprintf("project_package_invalid: documents/%d has unknown field %s", index, key))
			}
		}
		document := projectPackageDocument{}
		for key, target := range map[string]*string{"kind": &document.Kind, "source": &document.Source, "alias_prefix": &document.AliasPrefix, "media_type": &document.MediaType, "id": &document.ID, "version": &document.Version, "extensions": &document.Extensions} {
			if raw, exists := object[key]; exists {
				value, ok := raw.(string)
				if !ok || value == "" {
					return projectPackageSource{}, usageError(fmt.Sprintf("project_package_invalid: documents/%d %s must be a non-empty string", index, key))
				}
				*target = value
			}
		}
		if document.Kind != "schema" && document.Kind != "step" && document.Kind != "workflow" && document.Kind != "context" {
			return projectPackageSource{}, usageError(fmt.Sprintf("project_package_invalid: documents/%d has unsupported kind", index))
		}
		if document.Source == "" || document.AliasPrefix == "" || !projectValueName.MatchString(document.AliasPrefix) {
			return projectPackageSource{}, usageError(fmt.Sprintf("project_package_invalid: documents/%d requires source and valid alias_prefix", index))
		}
		if document.Kind != "context" && (document.MediaType != "" || document.ID != "" || document.Version != "") {
			return projectPackageSource{}, usageError(fmt.Sprintf("project_package_invalid: documents/%d non-context fields are not allowed", index))
		}
		if document.Kind != "workflow" && document.Extensions != "" {
			return projectPackageSource{}, usageError(fmt.Sprintf("project_package_invalid: documents/%d extensions are workflow-only", index))
		}
		result.Documents = append(result.Documents, document)
	}
	return result, nil
}

func projectIdentity(id, version string) (flow.Ref, error) {
	ref := flow.Ref{ID: id, Version: version, Digest: "sha256:" + strings.Repeat("0", 64)}
	data, _ := json.Marshal(ref)
	if err := flow.ValidateProtocol("ImmutableRef", data); err != nil {
		return flow.Ref{}, err
	}
	return ref, nil
}

func projectLogicalRef(registry flow.Registry, logical string) (flow.Ref, error) {
	id, version, found := strings.Cut(logical, "@")
	if !found || id == "" || version == "" || strings.Contains(version, "@") {
		return flow.Ref{}, errors.New("expected exact ID@VERSION")
	}
	var selected flow.Ref
	for ref := range registry {
		if ref.ID == id && ref.Version == version {
			if selected.ID != "" && selected != ref {
				return flow.Ref{}, errors.New("logical reference is ambiguous")
			}
			selected = ref
		}
	}
	if selected.ID == "" {
		return flow.Ref{}, errors.New("logical reference is not installed")
	}
	return selected, nil
}

func projectLogicalPackageRef(packages prifly.PackageRecord, logical string) (flow.Ref, error) {
	id, version, found := strings.Cut(logical, "@")
	if !found || id == "" || version == "" || strings.Contains(version, "@") {
		return flow.Ref{}, errors.New("expected exact ID@VERSION")
	}
	for _, pkg := range packages.Packages {
		if pkg.Ref.ID != id || pkg.Ref.Version != version {
			continue
		}
		if pkg.Status != "" && pkg.Status != prifly.PackageTrusted {
			return flow.Ref{}, errors.New("package is not trusted")
		}
		return pkg.Ref, nil
	}
	return flow.Ref{}, errors.New("package is not installed")
}

func projectRefValue(ref flow.Ref) map[string]any {
	return map[string]any{"id": ref.ID, "version": ref.Version, "digest": ref.Digest}
}

func projectPackageSourcePath(root, source string) (string, error) {
	if source == "" || filepath.IsAbs(source) {
		return "", usageError("project_package_invalid: document source must be a relative .prifly path")
	}
	profileRoot, err := canonicalProjectPath(filepath.Join(root, ".prifly"))
	if err != nil {
		return "", err
	}
	path, err := canonicalProjectPath(filepath.Join(root, source))
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(profileRoot, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", usageError("project_package_invalid: document source must stay inside .prifly")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", usageError("project_package_invalid: document source does not exist: " + source)
	}
	if !info.Mode().IsRegular() {
		return "", usageError("project_package_invalid: document source must be a regular file: " + source)
	}
	return path, nil
}

func projectContextSourcePath(root, skillsRoot, source string) (string, error) {
	if source == "" || filepath.IsAbs(source) {
		return "", usageError("project_package_invalid: context source must be a relative project path")
	}
	path, err := canonicalProjectPath(filepath.Join(root, source))
	if err != nil {
		return "", err
	}
	allowed := false
	for _, directory := range []string{filepath.Join(root, ".prifly"), skillsRoot} {
		base, err := canonicalProjectPath(directory)
		if err != nil {
			return "", err
		}
		relative, err := filepath.Rel(base, path)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			allowed = true
			break
		}
	}
	if !allowed {
		return "", usageError("project_package_invalid: context source must stay inside .prifly or the selected host skills root")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", usageError("project_package_invalid: context source does not exist: " + source)
	}
	if !info.Mode().IsRegular() {
		return "", usageError("project_package_invalid: context source must be a regular file: " + source)
	}
	return path, nil
}

func projectHostContextSourcePath(skillsRoot, source string) (string, error) {
	if source == "" || filepath.IsAbs(source) {
		return "", usageError("project_package_invalid: host context source must be a relative skills path")
	}
	path, err := canonicalProjectPath(filepath.Join(skillsRoot, source))
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(skillsRoot, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", usageError("project_package_invalid: host context source must stay inside the selected host skills root")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", usageError("project_package_invalid: host context source does not exist: " + source)
	}
	if !info.Mode().IsRegular() {
		return "", usageError("project_package_invalid: host context source must be a regular file: " + source)
	}
	return path, nil
}

func readProjectWorkflowFolder(root, folder string) (projectPackageSource, error) {
	workflowPath := filepath.Join(folder, "workflow.yaml")
	workflowValue, err := projectYAMLDocument(workflowPath)
	if err != nil {
		return projectPackageSource{}, usageError("project_workflow_folder_invalid: " + err.Error())
	}
	if _, err := projectFolderWorkflowDefinition(workflowValue); err != nil {
		return projectPackageSource{}, usageError("project_workflow_folder_invalid: " + err.Error())
	}
	packageValue, ok := workflowValue.(map[string]any)["package"]
	if !ok {
		return projectPackageSource{}, usageError("project_workflow_folder_invalid: workflow.yaml requires package")
	}
	documents := []projectPackageDocument{}
	for _, declaration := range []struct {
		directory, kind, prefix string
	}{
		{"schemas", "schema", "schema"},
		{"contexts", "context", "context"},
		{"steps", "step", "step"},
		{"workflows", "workflow", "workflow"},
	} {
		items, err := projectWorkflowFolderDocuments(root, folder, declaration.directory, declaration.kind, declaration.prefix)
		if err != nil {
			return projectPackageSource{}, err
		}
		documents = append(documents, items...)
	}
	workflowSource, err := projectRelativePath(root, workflowPath)
	if err != nil {
		return projectPackageSource{}, err
	}
	rootDocument := projectPackageDocument{Kind: "workflow", Source: workflowSource, AliasPrefix: "workflow", FolderRoot: true}
	extensionsPath := filepath.Join(folder, "extend.yaml")
	if info, err := os.Stat(extensionsPath); err == nil {
		if !info.Mode().IsRegular() {
			return projectPackageSource{}, usageError("project_workflow_folder_invalid: extend.yaml must be a regular file")
		}
		rootDocument.Extensions, err = projectRelativePath(root, extensionsPath)
		if err != nil {
			return projectPackageSource{}, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return projectPackageSource{}, err
	}
	documents = append(documents, rootDocument)
	source, err := projectWorkflowFolderPackage(packageValue, documents)
	if err != nil {
		return projectPackageSource{}, err
	}
	source.DecisionCatalog, err = projectWorkflowFolderDecisionCatalog(root, folder, workflowValue, source)
	if err != nil {
		return projectPackageSource{}, err
	}
	source.RootValue = workflowValue
	return source, nil
}

func projectWorkflowFolderDecisionCatalog(root, folder string, workflowValue any, source projectPackageSource) ([]prifly.DecisionDefinition, error) {
	workflow, ok := workflowValue.(map[string]any)
	if !ok {
		return nil, errors.New("workflow.yaml must be an object")
	}
	rawCatalog, exists := workflow["decision_catalog"]
	if !exists {
		return []prifly.DecisionDefinition{}, nil
	}
	paths, ok := rawCatalog.([]any)
	if !ok {
		return nil, usageError("project_workflow_folder_invalid: decision_catalog must be a list")
	}
	canonicalFolder, err := canonicalProjectPath(folder)
	if err != nil {
		return nil, err
	}
	result := make([]prifly.DecisionDefinition, 0, len(paths))
	seenPaths := map[string]bool{}
	seenIDs := map[string]bool{}
	for index, rawPath := range paths {
		sourcePath, ok := rawPath.(string)
		if !ok || sourcePath == "" {
			return nil, usageError(fmt.Sprintf("project_workflow_folder_invalid: decision_catalog/%d must be a non-empty source path", index))
		}
		path, err := projectPackageSourcePath(root, sourcePath)
		if err != nil {
			return nil, err
		}
		relative, err := filepath.Rel(canonicalFolder, path)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return nil, usageError("project_workflow_folder_invalid: decision catalog source must stay inside its workflow folder")
		}
		if filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml" {
			return nil, usageError("project_workflow_folder_invalid: decision catalog source must be YAML")
		}
		if seenPaths[path] {
			return nil, usageError("project_workflow_folder_invalid: decision_catalog must not repeat an exact source")
		}
		seenPaths[path] = true
		documents, err := projectYAMLDocuments(path)
		if err != nil {
			return nil, usageError("project_workflow_folder_invalid: " + err.Error())
		}
		if len(documents) != 1 {
			return nil, usageError("project_workflow_folder_invalid: decision catalog source requires exactly one YAML document")
		}
		definition, err := projectDecisionDefinition(documents[0])
		if err != nil {
			return nil, usageError("project_workflow_folder_invalid: " + err.Error())
		}
		if seenIDs[definition.ID] {
			return nil, usageError("project_workflow_folder_invalid: duplicate decision ID " + definition.ID)
		}
		if err := projectValidateDecisionDefinition(definition, workflow, source, result); err != nil {
			return nil, usageError("project_workflow_folder_invalid: " + err.Error())
		}
		seenIDs[definition.ID] = true
		result = append(result, definition)
	}
	return result, nil
}

func projectDecisionDefinition(value any) (prifly.DecisionDefinition, error) {
	object, ok := value.(map[string]any)
	if !ok {
		return prifly.DecisionDefinition{}, errors.New("decision definition must be an object")
	}
	allowed := map[string]bool{"authoring": true, "id": true, "title": true, "description": true, "phase": true, "required": true, "choices": true, "value_schema": true, "recommendation": true, "automatic": true, "sensitivity": true, "destination": true, "when": true}
	for key := range object {
		if !allowed[key] {
			return prifly.DecisionDefinition{}, errors.New("decision definition has unknown field " + key)
		}
	}
	if object["authoring"] != "prifly-run-decision/1" {
		return prifly.DecisionDefinition{}, errors.New("decision definition authoring must be prifly-run-decision/1")
	}
	copy := make(map[string]any, len(object)+3)
	for key, field := range object {
		if key != "authoring" {
			copy[key] = field
		}
	}
	if _, exists := copy["required"]; !exists {
		// The default exists so a declared preflight question is answered
		// before a Run starts. A runtime question is answered when its executor
		// raises a request, and nothing can demand that, so defaulting it to
		// true would mark every such decision with a promise it cannot keep.
		copy["required"] = copy["phase"] == "preflight"
	}
	if _, exists := copy["automatic"]; !exists {
		copy["automatic"] = false
	}
	if _, exists := copy["sensitivity"]; !exists {
		copy["sensitivity"] = "ordinary"
	}
	copy["schema_version"] = prifly.DecisionDefinitionVersion
	data, err := json.Marshal(copy)
	if err != nil {
		return prifly.DecisionDefinition{}, err
	}
	var definition prifly.DecisionDefinition
	if err := json.Unmarshal(data, &definition); err != nil {
		return prifly.DecisionDefinition{}, err
	}
	if err := prifly.ValidateDecisionDefinition(definition); err != nil {
		return prifly.DecisionDefinition{}, err
	}
	return definition, nil
}

func projectValidateDecisionDefinition(definition prifly.DecisionDefinition, workflow map[string]any, source projectPackageSource, prior []prifly.DecisionDefinition) error {
	if definition.When != nil {
		for _, profile := range definition.When.Profiles {
			if _, exists := source.Profiles[profile]; !exists {
				return errors.New("decision condition names unknown profile " + profile)
			}
		}
		priorDefinitions := make(map[string]prifly.DecisionDefinition, len(prior))
		for _, priorDefinition := range prior {
			priorDefinitions[priorDefinition.ID] = priorDefinition
		}
		for id, value := range definition.When.Answers {
			predecessor, exists := priorDefinitions[id]
			if !exists || predecessor.Phase != "preflight" {
				return errors.New("decision condition names unknown or forward preflight predecessor " + id)
			}
			if err := prifly.ValidateDecisionValue(predecessor, value); err != nil {
				return errors.New("decision condition answer is invalid for predecessor " + id + ": " + err.Error())
			}
		}
	}
	switch definition.Destination.Kind {
	case "package_profile":
		if definition.Phase != "preflight" || len(source.Profiles) == 0 {
			return errors.New("package profile decision requires declared preflight profiles")
		}
		for _, choice := range definition.Choices {
			var profile string
			if json.Unmarshal(choice.Value, &profile) != nil {
				return errors.New("package profile choice value must be a profile name")
			}
			if _, exists := source.Profiles[profile]; !exists {
				return errors.New("package profile choice names unknown profile " + profile)
			}
		}
	case "launch_input":
		inputs, ok := workflow["inputs"].(map[string]any)
		if !ok || inputs[definition.Destination.Name] == nil {
			return errors.New("decision destination names unknown launch input " + definition.Destination.Name)
		}
	}
	return nil
}

func projectWorkflowFolderPackage(raw any, documents []projectPackageDocument) (projectPackageSource, error) {
	packageFields, ok := raw.(map[string]any)
	if !ok {
		return projectPackageSource{}, usageError("project_workflow_folder_invalid: package must be an object")
	}
	value := make(map[string]any, len(packageFields)+2)
	for key, field := range packageFields {
		if key == "schema_version" || key == "documents" {
			return projectPackageSource{}, usageError("project_workflow_folder_invalid: package does not declare " + key)
		}
		value[key] = field
	}
	value["schema_version"] = projectPackageSourceVersion
	declared := make([]any, 0, len(documents))
	for _, document := range documents {
		item := map[string]any{"kind": document.Kind, "source": document.Source, "alias_prefix": document.AliasPrefix}
		if document.Extensions != "" {
			item["extensions"] = document.Extensions
		}
		declared = append(declared, item)
	}
	value["documents"] = declared
	source, err := parseProjectPackageSource(value)
	if err != nil {
		return projectPackageSource{}, err
	}
	for index := range source.Documents {
		source.Documents[index].FolderRoot = documents[index].FolderRoot
		source.Documents[index].FolderMember = documents[index].FolderMember
	}
	return source, nil
}

func projectWorkflowFolderDocuments(root, folder, directory, kind, prefix string) ([]projectPackageDocument, error) {
	path := filepath.Join(folder, directory)
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, usageError("project_workflow_folder_invalid: " + directory + " must be a directory")
	}
	result := []projectPackageDocument{}
	err = filepath.Walk(path, func(current string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return usageError("project_workflow_folder_invalid: symlinks are not allowed")
		}
		if info.IsDir() {
			return nil
		}
		extension := strings.ToLower(filepath.Ext(current))
		if extension != ".yaml" && extension != ".yml" {
			return nil
		}
		source, err := projectRelativePath(root, current)
		if err != nil {
			return err
		}
		result = append(result, projectPackageDocument{Kind: kind, Source: source, AliasPrefix: prefix, FolderMember: true})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Source < result[right].Source })
	return result, nil
}

func projectRelativePath(root, path string) (string, error) {
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", usageError("project_workflow_folder_invalid: source must stay inside the repository")
	}
	return filepath.ToSlash(relative), nil
}

func projectYAMLDocument(path string) (any, error) {
	documents, err := projectYAMLDocuments(path)
	if err != nil {
		return nil, err
	}
	if len(documents) != 1 {
		return nil, errors.New("workflow.yaml requires exactly one YAML document")
	}
	return documents[0], nil
}

func projectFolderWorkflowDefinition(value any) (map[string]any, error) {
	workflow, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("workflow.yaml must be an object")
	}
	if authoring, _ := workflow["authoring"].(string); authoring != projectWorkflowFolderVersion {
		return nil, errors.New("workflow.yaml authoring must be " + projectWorkflowFolderVersion)
	}
	if _, ok := workflow["package"].(map[string]any); !ok {
		return nil, errors.New("workflow.yaml package must be an object")
	}
	result := make(map[string]any, len(workflow)-1)
	for key, field := range workflow {
		if key != "package" && key != "decision_catalog" {
			result[key] = field
		}
	}
	result["authoring"] = "prifly-workflow/1"
	return result, nil
}

func compileProjectPackage(root, skillsRoot, output string, source projectPackageSource, values map[string]any, options projectWorkflowOptions) ([]projectCompileComponent, error) {
	pending := []projectPendingDocument{}
	components := []projectCompileComponent{}
	for _, declaration := range source.Documents {
		sourcePath, err := projectSourceValue(declaration.Source, values)
		if err != nil {
			return nil, err
		}
		var path string
		if declaration.Kind == "context" {
			path, err = projectContextSourcePath(root, skillsRoot, sourcePath)
		} else {
			path, err = projectPackageSourcePath(root, sourcePath)
		}
		if err != nil {
			return nil, err
		}
		if declaration.Kind == "context" && filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml" {
			if declaration.ID == "" || declaration.Version == "" || declaration.MediaType == "" {
				return nil, usageError("project_package_invalid: raw context requires id, version and media_type")
			}
			data, err := readFile(path, flow.MaxDocumentBytes)
			if err != nil {
				return nil, err
			}
			component, err := projectRawContextComponent(declaration, data)
			if err != nil {
				return nil, err
			}
			if err := projectAddComponent(output, declaration, component, values, &components); err != nil {
				return nil, err
			}
			continue
		}
		if declaration.Kind == "context" && (declaration.ID != "" || declaration.Version != "" || declaration.MediaType != "") {
			return nil, usageError("project_package_invalid: YAML context declares id, version and media_type in its own document")
		}
		if filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml" {
			return nil, usageError("project_package_invalid: document sources must be .yaml")
		}
		var documents []any
		if declaration.FolderRoot && source.RootValue != nil {
			documents = []any{source.RootValue}
		} else {
			documents, err = projectYAMLDocuments(path)
		}
		if err != nil {
			return nil, err
		}
		if declaration.FolderRoot || declaration.FolderMember {
			if len(documents) != 1 {
				name := declaration.Source
				if declaration.FolderRoot {
					name = "workflow.yaml"
				}
				return nil, usageError("project_workflow_folder_invalid: " + name + " requires exactly one YAML document")
			}
		}
		if declaration.FolderRoot {
			documents[0], err = projectFolderWorkflowDefinition(documents[0])
			if err != nil {
				return nil, usageError("project_workflow_folder_invalid: " + err.Error())
			}
		}
		for index, value := range documents {
			pending = append(pending, projectPendingDocument{document: declaration, value: value, index: index})
		}
	}
	if err := projectApplyWorkflowOptions(pending, options); err != nil {
		return nil, err
	}
	for len(pending) > 0 {
		next := make([]projectPendingDocument, 0, len(pending))
		progress := false
		for _, item := range pending {
			rendered, missing, err := projectSubstituteValue(item.value, values)
			if err != nil {
				return nil, usageError(fmt.Sprintf("project_compile_invalid_source: %s/%d: %s", item.document.Source, item.index, err))
			}
			if len(missing) != 0 {
				next = append(next, item)
				continue
			}
			component, err := compileProjectComponent(root, skillsRoot, item.document, rendered)
			if err != nil {
				return nil, usageError(fmt.Sprintf("project_compile_invalid_source: %s/%d: %s", item.document.Source, item.index, err))
			}
			if item.document.Extensions != "" {
				if err := projectApplyExtensions(&component, components, options.Extensions); err != nil {
					return nil, err
				}
			}
			if err := projectAddComponent(output, item.document, component, values, &components); err != nil {
				return nil, err
			}
			progress = true
		}
		if !progress {
			missing := map[string]bool{}
			for _, item := range next {
				_, names, _ := projectSubstituteValue(item.value, values)
				for _, name := range names {
					missing[name] = true
				}
			}
			names := make([]string, 0, len(missing))
			for name := range missing {
				names = append(names, name)
			}
			sort.Strings(names)
			return nil, usageError("project_compile_unresolved_values: " + strings.Join(names, ", "))
		}
		pending = next
	}
	return components, nil
}

func projectSourceValue(source string, values map[string]any) (string, error) {
	if name := projectValueReference.FindStringSubmatch(source); name != nil {
		value, exists := values[name[1]]
		path, stringValue := value.(string)
		if !exists || !stringValue || path == "" {
			return "", usageError("project_compile_unresolved_values: " + name[1])
		}
		return path, nil
	}
	if strings.Contains(source, "{{") || strings.Contains(source, "}}") {
		return "", usageError("project_package_invalid: document source substitution must occupy the full string")
	}
	return source, nil
}

func projectYAMLDocuments(path string) ([]any, error) {
	data, err := readFile(path, flow.MaxDocumentBytes)
	if err != nil {
		return nil, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	result := []any{}
	for {
		var document yaml.Node
		err := decoder.Decode(&document)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		raw, err := yaml.Marshal(&document)
		if err != nil {
			return nil, err
		}
		value, err := flow.Parse(raw, "yaml")
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	if len(result) == 0 {
		return nil, errors.New("source has no YAML documents")
	}
	return result, nil
}

func projectSubstituteValue(value any, values map[string]any) (any, []string, error) {
	missing := map[string]bool{}
	var visit func(any) (any, error)
	visit = func(value any) (any, error) {
		switch value := value.(type) {
		case string:
			if name := projectValueReference.FindStringSubmatch(value); name != nil {
				resolved, exists := values[name[1]]
				if !exists {
					missing[name[1]] = true
					return value, nil
				}
				return resolved, nil
			}
			if strings.Contains(value, "{{") || strings.Contains(value, "}}") {
				return nil, errors.New("substitution must occupy one complete YAML string")
			}
			return value, nil
		case map[string]any:
			result := make(map[string]any, len(value))
			for key, child := range value {
				resolved, err := visit(child)
				if err != nil {
					return nil, err
				}
				result[key] = resolved
			}
			return result, nil
		case []any:
			result := make([]any, len(value))
			for index, child := range value {
				resolved, err := visit(child)
				if err != nil {
					return nil, err
				}
				result[index] = resolved
			}
			return result, nil
		default:
			return value, nil
		}
	}
	result, err := visit(value)
	names := make([]string, 0, len(missing))
	for name := range missing {
		names = append(names, name)
	}
	sort.Strings(names)
	return result, names, err
}

func compileProjectComponent(root, skillsRoot string, document projectPackageDocument, value any) (projectCompileComponent, error) {
	if document.Kind == "context" {
		object, ok := value.(map[string]any)
		if !ok || len(object) != 4 {
			return projectCompileComponent{}, errors.New("YAML context requires id, version, media_type and text or source only")
		}
		id, idOK := object["id"].(string)
		version, versionOK := object["version"].(string)
		media, mediaOK := object["media_type"].(string)
		if !idOK || !versionOK || !mediaOK {
			return projectCompileComponent{}, errors.New("YAML context requires string id, version and media_type")
		}
		if text, ok := object["text"].(string); ok {
			return projectContextComponent(document, id, version, media, []byte(text))
		}
		path, err := projectContextSourcePathValue(root, skillsRoot, object["source"])
		if err != nil {
			return projectCompileComponent{}, err
		}
		data, err := readFile(path, flow.MaxDocumentBytes)
		if err != nil {
			return projectCompileComponent{}, err
		}
		return projectContextComponent(document, id, version, media, data)
	}
	data, err := json.Marshal(value)
	if err != nil {
		return projectCompileComponent{}, err
	}
	if document.Kind == "workflow" {
		data, err = flow.WorkflowJSONBytes(data, "yaml")
	} else if document.Kind == "step" {
		data, err = flow.StepJSONBytes(data, "yaml")
	} else {
		data, err = flow.JSONBytes(data, "json")
	}
	if err != nil {
		return projectCompileComponent{}, err
	}
	canonical, err := flow.Canonical(data)
	if err != nil {
		return projectCompileComponent{}, err
	}
	parsed, err := flow.Parse(canonical, "json")
	if err != nil {
		return projectCompileComponent{}, err
	}
	object, ok := parsed.(map[string]any)
	if !ok {
		return projectCompileComponent{}, errors.New("component must be an object with id and version")
	}
	id, idOK := object["id"].(string)
	version, versionOK := object["version"].(string)
	if !idOK || !versionOK {
		return projectCompileComponent{}, errors.New("component requires string id and version")
	}
	if document.Kind == "step" {
		protocol := "StepDefinition"
		switch object["schema_version"] {
		case "2":
			protocol = "StepDefinitionV2"
		case "3":
			protocol = "StepDefinitionV3"
		case "4":
			protocol = "StepDefinitionV4"
		case "5":
			protocol = "StepDefinitionV5"
		}
		if err := flow.ValidateProtocol(protocol, canonical); err != nil {
			return projectCompileComponent{}, err
		}
	}
	ref, err := projectIdentity(id, version)
	if err != nil {
		return projectCompileComponent{}, err
	}
	ref.Digest, err = flow.Digest(canonical)
	if err != nil {
		return projectCompileComponent{}, err
	}
	return projectCompileComponent{Kind: document.Kind, Ref: ref, Bytes: canonical}, nil
}

func projectContextSourcePathValue(root, skillsRoot string, value any) (string, error) {
	if source, ok := value.(string); ok {
		return projectContextSourcePath(root, skillsRoot, source)
	}
	object, ok := value.(map[string]any)
	if !ok || len(object) != 2 || object["root"] != "host_skills" {
		return "", errors.New("YAML context source must be a string or {root: host_skills, path: PATH}")
	}
	path, ok := object["path"].(string)
	if !ok {
		return "", errors.New("YAML context source path must be a string")
	}
	return projectHostContextSourcePath(skillsRoot, path)
}

func projectRawContextComponent(document projectPackageDocument, data []byte) (projectCompileComponent, error) {
	return projectContextComponent(document, document.ID, document.Version, document.MediaType, data)
}

func projectContextComponent(document projectPackageDocument, id, version, media string, data []byte) (projectCompileComponent, error) {
	ref, err := projectIdentity(id, version)
	if err != nil {
		return projectCompileComponent{}, err
	}
	ref.Digest = fmt.Sprintf("sha256:%x", sha256.Sum256(data))
	resource := flow.ContextResource{ByteEncoding: "utf8_text", MediaType: media, Bytes: data}
	resource, err = flow.CanonicalContextResource(ref, resource)
	if err != nil {
		return projectCompileComponent{}, err
	}
	return projectCompileComponent{Kind: "context", Ref: ref, Bytes: resource.Bytes, Resource: &resource}, nil
}

func projectAddComponent(output string, document projectPackageDocument, component projectCompileComponent, values map[string]any, components *[]projectCompileComponent) error {
	name := component.Ref.ID[strings.LastIndex(component.Ref.ID, "/")+1:]
	for _, existing := range *components {
		if existing.Ref.ID == component.Ref.ID && existing.Ref.Version == component.Ref.Version {
			return usageError("project_compile_duplicate_component: " + component.Ref.ID + "@" + component.Ref.Version)
		}
	}
	alias := document.AliasPrefix + "_" + name
	if _, exists := values[alias]; exists {
		return usageError("project_compile_duplicate_value: " + alias)
	}
	index := len(*components)
	directory, suffix := component.Kind+"s", ".json"
	if component.Kind == "context" {
		directory, suffix = "contexts", ".txt"
	}
	component.Path = fmt.Sprintf("%s/%03d%s", directory, index, suffix)
	component.Root = document.FolderRoot
	if output != "" {
		if err := writeProjectComponent(output, component); err != nil {
			return err
		}
	}
	*components = append(*components, component)
	values[alias] = projectRefValue(component.Ref)
	return nil
}

func projectReadWorkflowOptions(root string, source projectPackageSource, values map[string]any) (projectWorkflowOptions, error) {
	result := projectWorkflowOptions{Extensions: []projectWorkflowExtension{}, Settings: map[string]map[string]any{}, Exclude: []string{}}
	for _, document := range source.Documents {
		if document.Extensions == "" {
			continue
		}
		resolved, err := projectSourceValue(document.Extensions, values)
		if err != nil {
			return projectWorkflowOptions{}, err
		}
		path, err := projectPackageSourcePath(root, resolved)
		if err != nil {
			return projectWorkflowOptions{}, err
		}
		data, err := readFile(path, flow.MaxDocumentBytes)
		if err != nil {
			return projectWorkflowOptions{}, err
		}
		options, err := parseProjectWorkflowOptions(data)
		if err != nil {
			return projectWorkflowOptions{}, err
		}
		if len(result.Extensions) != 0 || len(result.Settings) != 0 || len(result.Exclude) != 0 {
			return projectWorkflowOptions{}, usageError("project_extension_invalid: only one extend.yaml is allowed per workflow folder")
		}
		result = options
	}
	return result, nil
}

func projectApplyPackageProfile(source projectPackageSource, requested string, values map[string]any) error {
	if requested != "" && len(source.Profiles) == 0 {
		return usageError("project_compile_unknown_profile: package declares no profiles")
	}
	if len(source.Profiles) == 0 {
		return nil
	}
	selected := source.DefaultProfile
	if requested != "" {
		selected = requested
	}
	profile, exists := source.Profiles[selected]
	if !exists {
		return usageError("project_compile_unknown_profile: " + selected)
	}
	for name, value := range profile {
		if _, exists := values[name]; exists {
			return usageError("project_compile_profile_value_conflict: " + name)
		}
		values[name] = value
	}
	return nil
}

func projectApplyWorkflowOptions(pending []projectPendingDocument, options projectWorkflowOptions) error {
	authors := map[string]map[string]any{}
	features := map[string]projectWorkflowFeature{}
	for index := range pending {
		item := &pending[index]
		if item.document.Kind != "workflow" {
			continue
		}
		source, ok := item.value.(map[string]any)
		if !ok {
			continue
		}
		id, ok := source["id"].(string)
		if !ok || id == "" {
			if len(options.Settings) != 0 || len(options.Exclude) != 0 || source["features"] != nil {
				return usageError("project_option_invalid_workflow: workflow id must be a non-empty string")
			}
			continue
		}
		workflow := projectExtensionWorkflowName(id)
		if _, exists := authors[workflow]; exists {
			return usageError("project_option_duplicate_workflow: " + workflow)
		}
		authors[workflow] = source
		raw, exists := source["features"]
		if !exists {
			continue
		}
		delete(source, "features")
		declared, ok := raw.(map[string]any)
		if !ok {
			return usageError("project_feature_invalid: features must be an object")
		}
		for name, rawFeature := range declared {
			if !projectValueName.MatchString(name) {
				return usageError("project_feature_invalid: feature must be a valid name")
			}
			feature, ok := rawFeature.(map[string]any)
			if !ok || len(feature) != 1 {
				return usageError("project_feature_invalid: " + name + " requires input only")
			}
			input, ok := feature["input"].(string)
			if !ok || input == "" {
				return usageError("project_feature_invalid: " + name + " input must be non-empty")
			}
			if !projectFeatureHasChoiceBypass(source, input) {
				return usageError("project_feature_invalid: " + name + " requires a Choice bypass for " + input)
			}
			if _, exists := features[name]; exists {
				return usageError("project_feature_duplicate: " + name)
			}
			features[name] = projectWorkflowFeature{Workflow: workflow, Input: input}
		}
	}
	assigned := map[string]any{}
	for workflow, settings := range options.Settings {
		source, exists := authors[workflow]
		if !exists {
			return usageError("project_option_unknown_workflow: " + workflow)
		}
		for input, value := range settings {
			if err := projectSetWorkflowSetting(source, workflow, input, value); err != nil {
				return err
			}
			assigned[workflow+"\x00"+input] = value
		}
	}
	for _, name := range options.Exclude {
		feature, exists := features[name]
		if !exists {
			return usageError("project_option_unknown_feature: " + name)
		}
		key := feature.Workflow + "\x00" + feature.Input
		if value, exists := assigned[key]; exists {
			if enabled, ok := value.(bool); !ok || enabled {
				return usageError("project_option_conflict: exclude " + name + " conflicts with setting " + feature.Workflow + "." + feature.Input)
			}
		}
		if err := projectSetWorkflowSetting(authors[feature.Workflow], feature.Workflow, feature.Input, false); err != nil {
			return usageError("project_feature_invalid: " + name + ": " + err.Error())
		}
	}
	return nil
}

func projectFeatureHasChoiceBypass(source map[string]any, input string) bool {
	stages, ok := source["stages"].(map[string]any)
	if !ok {
		return false
	}
	needle := "$inputs." + input
	for _, raw := range stages {
		stage, ok := raw.(map[string]any)
		if !ok || stage["kind"] != "choice" {
			continue
		}
		if _, ok := stage["default"].(string); !ok {
			continue
		}
		branches, ok := stage["branches"].([]any)
		if !ok || len(branches) == 0 {
			continue
		}
		for _, rawBranch := range branches {
			branch, ok := rawBranch.(map[string]any)
			if ok && projectValueContains(branch["predicate"], needle) {
				return true
			}
		}
	}
	return false
}

func projectValueContains(value any, needle string) bool {
	switch value := value.(type) {
	case string:
		return value == needle || strings.HasPrefix(value, needle+"#")
	case map[string]any:
		for _, child := range value {
			if projectValueContains(child, needle) {
				return true
			}
		}
	case []any:
		for _, child := range value {
			if projectValueContains(child, needle) {
				return true
			}
		}
	}
	return false
}

func projectSetWorkflowSetting(source map[string]any, workflow, input string, value any) error {
	inputs, ok := source["inputs"].(map[string]any)
	if !ok {
		return usageError("project_option_unknown_input: " + workflow + "." + input)
	}
	raw, exists := inputs[input]
	if !exists {
		return usageError("project_option_unknown_input: " + workflow + "." + input)
	}
	port, ok := raw.(map[string]any)
	if !ok {
		return usageError("project_option_not_project_scoped: " + workflow + "." + input)
	}
	configuration, ok := port["configuration"].(map[string]any)
	if !ok || configuration["scope"] != "project" {
		return usageError("project_option_not_project_scoped: " + workflow + "." + input)
	}
	configuration["default"] = value
	return nil
}

func projectApplyExtensions(component *projectCompileComponent, components []projectCompileComponent, extensions []projectWorkflowExtension) error {
	if len(extensions) == 0 {
		return nil
	}
	var workflow map[string]any
	if err := json.Unmarshal(component.Bytes, &workflow); err != nil {
		return err
	}
	steps := map[string]projectCompileComponent{}
	for _, candidate := range components {
		if candidate.Kind == "step" {
			steps[candidate.Ref.ID[strings.LastIndex(candidate.Ref.ID, "/")+1:]] = candidate
		}
	}
	// An extension names components by their short folder name, while every
	// component file carries a full id inside it. Taking the id is the natural
	// guess and it is wrong, so the refusal lists the names that would work.
	known := make([]string, 0, len(steps))
	for name := range steps {
		known = append(known, name)
	}
	slices.Sort(known)
	for _, extension := range extensions {
		if workflowName := projectExtensionWorkflowName(component.Ref.ID); extension.Workflow != workflowName {
			return usageError("project_extension_unknown_workflow: " + extension.Workflow + " (known: " + workflowName + ")")
		}
		step, exists := steps[extension.Step]
		if !exists {
			return usageError("project_extension_unknown_step: " + extension.Step + " (known: " + strings.Join(known, ", ") + ")")
		}
		var definition flow.StepDefinition
		if err := json.Unmarshal(step.Bytes, &definition); err != nil || len(definition.Inputs) != 0 {
			return usageError("project_extension_requires_full_yaml: an extension inserts a step without inputs; a step that needs inputs belongs in a workflow graph you write yourself, not in extend.yaml")
		}
		if err := applyProjectExtension(workflow, extension, projectRefValue(step.Ref)); err != nil {
			return err
		}
	}
	data, err := json.Marshal(workflow)
	if err != nil {
		return err
	}
	component.Bytes, err = flow.Canonical(data)
	if err != nil {
		return err
	}
	component.Ref.Digest, err = flow.Digest(component.Bytes)
	return err
}

func projectValidatePackageWorkflows(components []projectCompileComponent, base flow.Registry) error {
	registry := make(flow.Registry, len(base)+len(components))
	for ref, data := range base {
		registry[ref] = data
	}
	resources := flow.ContextResources{}
	for _, component := range components {
		if component.Kind == "context" {
			resources[component.Ref] = *component.Resource
		} else {
			registry[component.Ref] = component.Bytes
		}
	}
	for _, component := range components {
		if component.Kind == "workflow" {
			if _, err := flow.CompileCore(component.Bytes, "json", registry, resources); err != nil {
				return usageError("project_compile_invalid_workflow: " + err.Error())
			}
		}
	}
	return nil
}

func writeProjectPackageManifest(output string, source projectPackageSource, components []projectCompileComponent, packages prifly.PackageRecord) (flow.Ref, error) {
	dependencies, err := projectResolvedDependencies(source, packages)
	if err != nil {
		return flow.Ref{}, err
	}
	files := make([]map[string]any, 0, len(components))
	manifestComponents := make([]map[string]any, 0, len(components))
	for _, component := range components {
		media := "application/json"
		if component.Kind == "context" {
			media = component.Resource.MediaType
		}
		files = append(files, map[string]any{"path": component.Path, "digest": fmt.Sprintf("sha256:%x", sha256.Sum256(component.Bytes)), "size_bytes": len(component.Bytes), "media_type": media, "role": "data"})
		manifestComponents = append(manifestComponents, map[string]any{"kind": component.Kind, "ref": component.Ref, "path": component.Path})
	}
	if len(source.DecisionCatalog) != 0 {
		encoded, err := json.Marshal(prifly.DecisionCatalog{SchemaVersion: prifly.DecisionCatalogVersion, Decisions: source.DecisionCatalog})
		if err != nil {
			return flow.Ref{}, err
		}
		catalog, err := flow.Canonical(encoded)
		if err != nil {
			return flow.Ref{}, err
		}
		data := append(catalog, '\n')
		if err := os.WriteFile(filepath.Join(output, projectDecisionCatalogFile), data, 0644); err != nil {
			return flow.Ref{}, err
		}
		files = append(files, map[string]any{"path": projectDecisionCatalogFile, "digest": fmt.Sprintf("sha256:%x", sha256.Sum256(data)), "size_bytes": len(data), "media_type": "application/json", "role": "data"})
	}
	if source.Build != nil {
		data, err := projectCanonicalJSON(source.Build)
		if err != nil {
			return flow.Ref{}, err
		}
		if err := validateProjectBuild(data, components); err != nil {
			return flow.Ref{}, err
		}
		if err := os.WriteFile(filepath.Join(output, projectBuildFile), data, 0644); err != nil {
			return flow.Ref{}, err
		}
		files = append(files, map[string]any{"path": projectBuildFile, "digest": fmt.Sprintf("sha256:%x", sha256.Sum256(data)), "size_bytes": len(data), "media_type": "application/json", "role": "data"})
	}
	sort.Slice(files, func(i, j int) bool { return files[i]["path"].(string) < files[j]["path"].(string) })
	manifest := map[string]any{"schema_version": "1", "id": source.ID, "version": source.Version, "description": source.Description, "requires_core_protocol": source.RequiresCoreProtocol, "dependencies": dependencies, "components": manifestComponents, "files": files, "requested_capabilities": source.RequestedCapabilities, "license": source.License}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return flow.Ref{}, err
	}
	data, err := flow.Canonical(encoded)
	if err != nil {
		return flow.Ref{}, err
	}
	if err := flow.ValidateProtocol("PackageManifest", data); err != nil {
		return flow.Ref{}, err
	}
	if err := os.WriteFile(filepath.Join(output, prifly.PackageManifestFile), append(data, '\n'), 0644); err != nil {
		return flow.Ref{}, err
	}
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256(append(data, '\n')))
	return flow.Ref{ID: source.ID, Version: source.Version, Digest: digest}, nil
}
