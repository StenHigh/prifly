package main

import (
	"crypto/sha256"
	_ "embed"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/stenhigh/prifly/internal/flow"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

const projectVariantProfileVersion = "prifly-project-profile/3"
const projectBuildFile = "build-provenance.json"
const projectBuildVersion = "prifly-build-provenance/1"

//go:embed project_build.schema.json
var projectBuildSchema []byte

type projectBuildIdentity struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

type projectBuildMapping struct {
	Kind        string   `json:"kind"`
	AuthorRef   flow.Ref `json:"author_ref"`
	CompiledRef flow.Ref `json:"compiled_ref"`
	Path        string   `json:"path"`
}

type projectBuildProvenance struct {
	SchemaVersion  string                    `json:"schema_version"`
	Algorithm      string                    `json:"algorithm"`
	AuthorPackage  projectBuildIdentity      `json:"author_package"`
	BuildKey       string                    `json:"build_key"`
	PackageProfile string                    `json:"package_profile"`
	Values         map[string]any            `json:"values"`
	Settings       map[string]map[string]any `json:"settings"`
	Exclude        []string                  `json:"exclude"`
	RootRef        flow.Ref                  `json:"root_ref"`
	Components     []projectBuildMapping     `json:"components"`
}

func projectCanonicalJSON(value any) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return flow.Canonical(data)
}

func projectBuildVersionFor(digest [32]byte) string {
	return "0.0.0-b1." + strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(digest[:]))
}

func projectBuildPackageVersion(buildKey string) (string, error) {
	encoded, ok := strings.CutPrefix(buildKey, "sha256:")
	key, err := hex.DecodeString(encoded)
	if !ok || err != nil || len(key) != sha256.Size {
		return "", usageError("project_compile_invalid_provenance: invalid build key")
	}
	return projectBuildVersionFor([32]byte(key)), nil
}

func projectBuildComponentVersion(buildKey, kind string, author flow.Ref) (string, error) {
	identity, err := projectCanonicalJSON([]any{buildKey, kind, author.ID, author.Version})
	if err != nil {
		return "", err
	}
	return projectBuildVersionFor(sha256.Sum256(identity)), nil
}

func projectResolvedDependencies(source projectPackageSource, packages prifly.PackageRecord) ([]flow.Ref, error) {
	if source.ResolvedDependencies != nil {
		return source.ResolvedDependencies, nil
	}
	refs := make([]flow.Ref, 0, len(source.Dependencies))
	for _, logical := range source.Dependencies {
		ref, err := projectLogicalPackageRef(packages, logical)
		if err != nil {
			return nil, usageError("project_compile_dependency: " + err.Error())
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func writeProjectComponent(output string, component projectCompileComponent) error {
	path := filepath.Join(output, filepath.FromSlash(component.Path))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, component.Bytes, 0644)
}

// Render once: both public entry points seal the very bytes used for the build
// identity. A second compilation could read changed skills or extend settings.
func compileAndSealProjectPackage(root, skillsRoot, output, profileVersion, selectedProfile string, source projectPackageSource, registry flow.Registry, packages prifly.PackageRecord, values map[string]any, options projectWorkflowOptions) (projectCompileResult, error) {
	variant := profileVersion == projectVariantProfileVersion
	intermediateOutput := output
	if variant {
		intermediateOutput = ""
	}
	parameters := maps.Clone(values)
	components, err := compileProjectPackage(root, skillsRoot, intermediateOutput, source, values, options)
	if err != nil {
		return projectCompileResult{}, err
	}
	if variant {
		source.ResolvedDependencies, err = projectResolvedDependencies(source, packages)
		if err != nil {
			return projectCompileResult{}, err
		}
		if selectedProfile == "" {
			selectedProfile = source.DefaultProfile
		}
		components, source.Build, err = projectBuildVariant(source, components, selectedProfile, parameters, options)
		if err != nil {
			return projectCompileResult{}, err
		}
		source.Version, err = projectBuildPackageVersion(source.Build.BuildKey)
		if err != nil {
			return projectCompileResult{}, err
		}
	}
	if err := projectValidatePackageWorkflows(components, registry); err != nil {
		return projectCompileResult{}, err
	}
	if variant {
		for _, component := range components {
			if err := writeProjectComponent(output, component); err != nil {
				return projectCompileResult{}, err
			}
		}
	}
	ref, err := writeProjectPackageManifest(output, source, components, packages)
	if err != nil {
		return projectCompileResult{}, err
	}
	result := projectCompileResult{SchemaVersion: "project-compile/1", Repository: root, Package: ref, Output: output, Components: components}
	if source.Build != nil {
		result.SchemaVersion = "project-compile/2"
		result.AuthorPackage = &source.Build.AuthorPackage
		result.BuildKey = source.Build.BuildKey
	}
	return result, nil
}

// The same instance-data boundary as flow.pinValueRefs: literal values and
// input configuration defaults are data, even when they resemble exact refs.
// JSON Schema and raw context payloads never enter this walker.
func projectMapDefinitionRefs(value any, configuration bool, visit func(flow.Ref) (any, error)) (any, error) {
	switch object := value.(type) {
	case map[string]any:
		id, idOK := object["id"].(string)
		version, versionOK := object["version"].(string)
		digest, digestOK := object["digest"].(string)
		if len(object) == 3 && idOK && versionOK && digestOK {
			return visit(flow.Ref{ID: id, Version: version, Digest: digest})
		}
		result := make(map[string]any, len(object))
		for key, child := range object {
			if key == "value" && object["from"] == "literal" || key == "default" && configuration {
				result[key] = child
				continue
			}
			_, input := object["required"].(bool)
			isConfiguration := key == "configuration" && input && (object["format"] == "json" || object["format"] == "blob")
			mapped, err := projectMapDefinitionRefs(child, isConfiguration, visit)
			if err != nil {
				return nil, err
			}
			result[key] = mapped
		}
		return result, nil
	case []any:
		result := make([]any, len(object))
		for i, child := range object {
			mapped, err := projectMapDefinitionRefs(child, false, visit)
			if err != nil {
				return nil, err
			}
			result[i] = mapped
		}
		return result, nil
	default:
		return value, nil
	}
}

func projectBuildVariant(source projectPackageSource, components []projectCompileComponent, profile string, values map[string]any, options projectWorkflowOptions) ([]projectCompileComponent, *projectBuildProvenance, error) {
	components = slices.Clone(components)
	slices.SortFunc(components, func(a, b projectCompileComponent) int {
		return strings.Compare(a.Kind+":"+a.Ref.String(), b.Kind+":"+b.Ref.String())
	})
	owned := make(map[flow.Ref]int, len(components))
	for index, component := range components {
		owned[component.Ref] = index
	}
	parsed := make([]any, len(components))
	inputDocuments := make([]any, len(components))
	for index, component := range components {
		var document any
		if component.Kind == "context" {
			document = map[string]any{"digest": component.Ref.Digest, "media_type": component.Resource.MediaType, "byte_encoding": component.Resource.ByteEncoding}
		} else {
			var err error
			parsed[index], err = flow.Parse(component.Bytes, "json")
			if err != nil {
				return nil, nil, err
			}
			document = parsed[index]
			if component.Kind != "schema" {
				document, err = projectMapDefinitionRefs(document, false, func(ref flow.Ref) (any, error) {
					if _, internal := owned[ref]; internal {
						return projectBuildIdentity{ID: ref.ID, Version: ref.Version}, nil
					}
					return projectRefValue(ref), nil
				})
				if err != nil {
					return nil, nil, err
				}
			}
		}
		inputDocuments[index] = map[string]any{"kind": component.Kind, "id": component.Ref.ID, "version": component.Ref.Version, "root": component.Root, "document": document}
	}
	provenance := &projectBuildProvenance{SchemaVersion: projectBuildVersion, Algorithm: "b1", AuthorPackage: projectBuildIdentity{ID: source.ID, Version: source.Version}, PackageProfile: profile, Values: values, Settings: options.Settings, Exclude: options.Exclude, Components: []projectBuildMapping{}}
	input := map[string]any{
		"algorithm": "b1", "author_package": provenance.AuthorPackage, "description": source.Description,
		"license": source.License, "requires_core_protocol": source.RequiresCoreProtocol,
		"requested_capabilities": source.RequestedCapabilities, "dependencies": source.ResolvedDependencies,
		"decision_catalog": source.DecisionCatalog, "package_profile": profile, "values": values,
		"settings": options.Settings, "exclude": options.Exclude, "extensions": options.Extensions, "documents": inputDocuments,
	}
	data, err := projectCanonicalJSON(input)
	if err != nil {
		return nil, nil, err
	}
	key := sha256.Sum256(data)
	provenance.BuildKey = fmt.Sprintf("sha256:%x", key)
	result := make([]projectCompileComponent, len(components))
	active, done := make(map[int]bool), make(map[int]bool)
	var rebuild func(int) (flow.Ref, error)
	rebuild = func(index int) (flow.Ref, error) {
		if done[index] {
			return result[index].Ref, nil
		}
		if active[index] || len(active) >= flow.MaxDepth {
			return flow.Ref{}, usageError("project_compile_dependency_cycle: owned definitions cannot be sealed recursively")
		}
		active[index] = true
		component := components[index]
		component.AuthorRef = component.Ref
		version, err := projectBuildComponentVersion(provenance.BuildKey, component.Kind, component.AuthorRef)
		if err != nil {
			return flow.Ref{}, err
		}
		component.Ref.Version = version
		if component.Kind != "context" {
			value := parsed[index]
			if component.Kind != "schema" {
				value, err = projectMapDefinitionRefs(value, false, func(ref flow.Ref) (any, error) {
					child, internal := owned[ref]
					if !internal {
						return projectRefValue(ref), nil
					}
					resolved, err := rebuild(child)
					return projectRefValue(resolved), err
				})
				if err != nil {
					return flow.Ref{}, err
				}
			}
			object := maps.Clone(value.(map[string]any))
			object["version"] = component.Ref.Version
			component.Bytes, err = projectCanonicalJSON(object)
			if err != nil {
				return flow.Ref{}, err
			}
			component.Ref.Digest, err = flow.Digest(component.Bytes)
			if err != nil {
				return flow.Ref{}, err
			}
		}
		directory, suffix := component.Kind+"s", ".json"
		if component.Kind == "context" {
			directory, suffix = "contexts", ".txt"
		}
		component.Path = fmt.Sprintf("%s/%03d%s", directory, index, suffix)
		result[index], done[index] = component, true
		delete(active, index)
		return component.Ref, nil
	}
	for index, original := range components {
		ref, err := rebuild(index)
		if err != nil {
			return nil, nil, err
		}
		provenance.Components = append(provenance.Components, projectBuildMapping{Kind: original.Kind, AuthorRef: original.Ref, CompiledRef: ref, Path: result[index].Path})
		if original.Root {
			if provenance.RootRef.ID != "" {
				return nil, nil, usageError("project_compile_invalid_root: package has multiple roots")
			}
			provenance.RootRef = ref
		}
	}
	return result, provenance, nil
}

func validateProjectBuild(data []byte, components []projectCompileComponent) error {
	schema, err := flow.Canonical(projectBuildSchema)
	if err != nil {
		return err
	}
	digest, err := flow.Digest(schema)
	if err != nil {
		return err
	}
	ref := flow.Ref{ID: "compiler:schema/build-provenance", Version: "1.0.0", Digest: digest}
	if err := flow.ValidateSchema(flow.Registry{ref: schema}, ref, data); err != nil {
		return usageError("project_compile_invalid_provenance: " + err.Error())
	}
	var build projectBuildProvenance
	if err := json.Unmarshal(data, &build); err != nil {
		return err
	}
	if len(build.Components) != len(components) {
		return usageError("project_compile_invalid_provenance: component inventory differs")
	}
	seen, roots := map[flow.Ref]bool{}, 0
	for _, mapping := range build.Components {
		if seen[mapping.CompiledRef] || mapping.AuthorRef.ID != mapping.CompiledRef.ID {
			return usageError("project_compile_invalid_provenance: duplicate or mismatched identity")
		}
		version, err := projectBuildComponentVersion(build.BuildKey, mapping.Kind, mapping.AuthorRef)
		if err != nil {
			return err
		}
		if mapping.CompiledRef.Version != version {
			return usageError("project_compile_invalid_provenance: component version differs from build identity")
		}
		seen[mapping.CompiledRef] = true
		matched := false
		// ponytail: at most 1024 exports; index by ref if this bound grows.
		for _, component := range components {
			if component.Ref == mapping.CompiledRef && component.AuthorRef == mapping.AuthorRef && component.Kind == mapping.Kind && component.Path == mapping.Path {
				matched = true
				if component.Root && component.Ref == build.RootRef && component.Kind == "workflow" {
					roots++
				}
				break
			}
		}
		if !matched {
			return usageError("project_compile_invalid_provenance: mapping is not an exported component")
		}
	}
	if roots != 1 {
		return usageError("project_compile_invalid_provenance: root is not the compiled folder root")
	}
	return nil
}

func projectCompiledLaunchPath(root string, launch projectLaunch, compiled projectCompileResult) (string, error) {
	if compiled.SchemaVersion != "project-compile/2" {
		return projectCompiledLaunchWorkflow(root, launch, compiled.Components)
	}
	data, err := readFile(filepath.Join(compiled.Output, projectBuildFile), flow.MaxDocumentBytes)
	if err != nil {
		return "", err
	}
	if err := validateProjectBuild(data, compiled.Components); err != nil {
		return "", err
	}
	var build projectBuildProvenance
	if err := json.Unmarshal(data, &build); err != nil {
		return "", err
	}
	if compiled.AuthorPackage == nil || build.AuthorPackage != *compiled.AuthorPackage || build.BuildKey != compiled.BuildKey {
		return "", usageError("project_compile_invalid_provenance: build identity differs")
	}
	version, err := projectBuildPackageVersion(build.BuildKey)
	if err != nil {
		return "", err
	}
	if compiled.Package.ID != build.AuthorPackage.ID || compiled.Package.Version != version {
		return "", usageError("project_compile_invalid_provenance: package version differs from build identity")
	}
	for _, component := range compiled.Components {
		if component.Ref == build.RootRef {
			return component.Path, nil
		}
	}
	return "", usageError("project_compile_invalid_provenance: root is absent")
}
