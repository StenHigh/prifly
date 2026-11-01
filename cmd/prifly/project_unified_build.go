package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/projectwire"
)

func projectUnifiedComponentVersion(buildKey, kind string, author flow.Ref, key string) (string, error) {
	data, err := projectCanonicalJSON([]any{buildKey, kind, author.ID, author.Version, key})
	if err != nil {
		return "", err
	}
	return strings.Replace(projectBuildVersionFor(sha256.Sum256(data)), "-b1.", "-b2.", 1), nil
}
func projectUnifiedPackageVersion(key string) (string, error) {
	v, e := projectBuildPackageVersion(key)
	return strings.Replace(v, "-b1.", "-b2.", 1), e
}

func projectBuildUnifiedVariant(source projectPackageSource, components []projectCompileComponent, profile string, values map[string]any, options projectWorkflowOptions) ([]projectCompileComponent, *projectBuildProvenance, error) {
	owned := map[flow.Ref]int{}
	for i, c := range components {
		owned[c.Ref] = i
	}
	keys := make([]string, len(components))
	authors := make([]flow.Ref, len(components))
	records := make([]map[string]any, len(components))
	metadata := make([]projectUnifiedDefinition, len(components))
	parsed := make([]any, len(components))
	active := map[int]bool{}
	count := 0
	for i, c := range components {
		if c.Kind == "workflow" {
			count++
		}
		if count > 256 {
			return nil, nil, refusal("project_composition_variant_limit", "generated wrappers exceed 256 Workflow variants")
		}
		if c.Unified != nil {
			metadata[i] = *c.Unified
		} else {
			metadata[i] = projectUnifiedDefinition{Owner: filepathOwner(source), Source: "generated", SourceDigest: projectUnifiedHash(map[string]any{"kind": "native-wrapper"}), AuthorID: c.Ref.ID, AuthorVersion: "1.0.0"}
		}
		if metadata[i].Settings == nil {
			metadata[i].Settings = map[string]any{}
		}
		if c.Kind != "context" {
			v, e := flow.Parse(c.Bytes, "json")
			if e != nil {
				return nil, nil, e
			}
			parsed[i] = v
		}
	}
	var prepare func(int) error
	prepare = func(i int) error {
		if keys[i] != "" {
			return nil
		}
		if active[i] {
			return refusal("project_compile_dependency_cycle", "variant dependency cycle")
		}
		active[i] = true
		defer delete(active, i)
		c, m := components[i], metadata[i]
		var document any
		if c.Kind == "context" {
			document = map[string]any{"bytes": c.Bytes, "media_type": c.Resource.MediaType, "byte_encoding": c.Resource.ByteEncoding}
		} else {
			object := maps.Clone(parsed[i].(map[string]any))
			object["id"] = m.AuthorID
			object["version"] = m.AuthorVersion
			document = object
			if c.Kind != "schema" {
				var err error
				document, err = projectMapDefinitionRefs(document, false, func(ref flow.Ref) (any, error) {
					child, ok := owned[ref]
					if !ok {
						return projectRefValue(ref), nil
					}
					if err := prepare(child); err != nil {
						return nil, err
					}
					return map[string]any{"owner": metadata[child].Owner, "kind": components[child].Kind, "author_id": metadata[child].AuthorID, "author_version": metadata[child].AuthorVersion, "variant_key": keys[child]}, nil
				})
				if err != nil {
					return err
				}
			}
		}
		// Native bodies can coincide for disabled and absent selections. Keep the
		// canonical source policy key so those reviewed choices seal separately.
		record := map[string]any{"owner": m.Owner, "source": m.Source, "source_digest": m.SourceDigest, "source_variant_key": m.VariantKey, "kind": c.Kind, "author_id": m.AuthorID, "author_version": m.AuthorVersion, "root": c.Root, "settings": m.Settings, "document": document}
		keys[i] = projectUnifiedHash(record)
		records[i] = record
		// An author ref is the exact prepared native definition before build versions.
		author := flow.Ref{ID: m.AuthorID, Version: m.AuthorVersion}
		if c.Kind == "context" {
			author.Digest = c.Ref.Digest
		} else {
			object := maps.Clone(parsed[i].(map[string]any))
			object["id"] = m.AuthorID
			object["version"] = m.AuthorVersion
			var value any = object
			if c.Kind != "schema" {
				var err error
				value, err = projectMapDefinitionRefs(value, false, func(ref flow.Ref) (any, error) {
					child, ok := owned[ref]
					if !ok {
						return projectRefValue(ref), nil
					}
					if err := prepare(child); err != nil {
						return nil, err
					}
					return projectRefValue(authors[child]), nil
				})
				if err != nil {
					return err
				}
			}
			data, err := projectCanonicalJSON(value)
			if err != nil {
				return err
			}
			author.Digest, err = flow.Digest(data)
			if err != nil {
				return err
			}
		}
		authors[i] = author
		return nil
	}
	for i := range components {
		if err := prepare(i); err != nil {
			return nil, nil, err
		}
	}
	inventory := slices.Clone(keys)
	slices.Sort(inventory)
	input := map[string]any{"algorithm": "b2", "author_package": projectBuildIdentity{ID: source.ID, Version: source.Version}, "description": source.Description, "license": source.License, "requires_core_protocol": source.RequiresCoreProtocol, "requested_capabilities": source.RequestedCapabilities, "dependencies": source.ResolvedDependencies, "decision_catalog": source.DecisionCatalog, "package_profile": profile, "values": values, "exclude": options.Exclude, "resources": options.Resources, "source_files": source.Composition.Files, "variants": inventory}
	if source.ExecutionBindings != nil {
		raw, _ := json.Marshal(source.ExecutionBindings)
		v, _ := flow.Parse(raw, "json")
		v, err := projectMapDefinitionRefs(v, false, func(ref flow.Ref) (any, error) {
			if i, ok := owned[ref]; ok {
				return map[string]any{"variant_key": keys[i]}, nil
			}
			return projectRefValue(ref), nil
		})
		if err != nil {
			return nil, nil, err
		}
		input["execution_bindings"] = v
	}
	provenance := &projectBuildProvenance{SchemaVersion: "prifly-build-provenance/3", Algorithm: "b2", AuthorPackage: projectBuildIdentity{ID: source.ID, Version: source.Version}, BuildKey: projectUnifiedHash(input), PackageProfile: profile, Values: values, Settings: options.Settings, Exclude: options.Exclude, Composition: &source.Composition.CompositionReview, Inputs: input, Components: []projectBuildMapping{}}
	result := make([]projectCompileComponent, len(components))
	done := map[int]bool{}
	active = map[int]bool{}
	var seal func(int) (flow.Ref, error)
	seal = func(i int) (flow.Ref, error) {
		if done[i] {
			return result[i].Ref, nil
		}
		if active[i] {
			return flow.Ref{}, refusal("project_compile_dependency_cycle", "variant sealing cycle")
		}
		active[i] = true
		defer delete(active, i)
		c := components[i]
		c.CompileRef = c.Ref
		c.AuthorRef = authors[i]
		c.Ref.ID = authors[i].ID
		version, err := projectUnifiedComponentVersion(provenance.BuildKey, c.Kind, authors[i], keys[i])
		if err != nil {
			return flow.Ref{}, err
		}
		c.Ref.Version = version
		if c.Kind != "context" {
			object := maps.Clone(parsed[i].(map[string]any))
			object["id"] = c.Ref.ID
			object["version"] = version
			var v any = object
			if c.Kind != "schema" {
				v, err = projectMapDefinitionRefs(v, false, func(ref flow.Ref) (any, error) {
					child, ok := owned[ref]
					if !ok {
						return projectRefValue(ref), nil
					}
					r, e := seal(child)
					return projectRefValue(r), e
				})
				if err != nil {
					return flow.Ref{}, err
				}
			}
			c.Bytes, err = projectCanonicalJSON(v)
			if err != nil {
				return flow.Ref{}, err
			}
			c.Ref.Digest, err = flow.Digest(c.Bytes)
			if err != nil {
				return flow.Ref{}, err
			}
		}
		m := metadata[i]
		m.VariantKey = keys[i]
		c.Unified = &m
		directory, suffix := c.Kind+"s", ".json"
		if c.Kind == "context" {
			directory, suffix = "contexts", ".txt"
		}
		c.Path = fmt.Sprintf("%s/%03d%s", directory, i, suffix)
		result[i] = c
		done[i] = true
		return c.Ref, nil
	}
	for i, c := range components {
		ref, err := seal(i)
		if err != nil {
			return nil, nil, err
		}
		m := metadata[i]
		provenance.Components = append(provenance.Components, projectBuildMapping{Kind: c.Kind, AuthorRef: authors[i], CompiledRef: ref, Path: result[i].Path, SourceOwner: m.Owner, Source: m.Source, SourceDigest: m.SourceDigest, VariantKey: keys[i], VariantInputs: records[i]})
		if c.Root {
			provenance.RootRef = ref
		}
	}

	details := &projectwire.CompositionPointsDetails{Owners: []projectwire.CompositionSourceOwner{}, Points: []projectwire.CompositionPointReview{}, Variants: []projectwire.CompositionVariantReview{}}
	owners := map[string]bool{}
	byName := map[string]flow.Ref{}
	for i, c := range result {
		m := metadata[i]
		if !owners[m.Owner] {
			files := map[string]string{}
			for path, digest := range source.Composition.Files {
				if strings.HasPrefix(path, m.Owner+"/") {
					files[path] = digest
				}
			}
			details.Owners = append(details.Owners, projectwire.CompositionSourceOwner{Source: m.Owner, Digest: projectUnifiedHash(files)})
			owners[m.Owner] = true
		}
		details.Variants = append(details.Variants, projectwire.CompositionVariantReview{Owner: m.Owner, Source: m.Source, AuthorRef: authors[i], VariantKey: keys[i], CompiledRef: c.Ref, Scopes: append([]string{}, source.Unified.Scopes[c.Name]...), Settings: m.Settings})
		byName[c.Name] = c.Ref
	}
	for _, target := range slices.Sorted(maps.Keys(source.Unified.Points)) {
		for _, e := range source.Unified.Points[target] {
			p := e.UnifiedPoint
			implementations := []flow.Ref{}
			for _, leaf := range projectCompositionLeaves([]projectWorkflowExtension{e}) {
				name := leaf.Call
				if name == "" {
					name = leaf.Step
				}
				if ref := byName[name]; ref.ID != "" {
					implementations = append(implementations, ref)
				}
			}
			details.Points = append(details.Points, projectwire.CompositionPointReview{Owner: p.Owner, Name: p.Name, Export: p.Export, Source: e.Source, Scope: source.Unified.ScopeLabels[p.Scope], Enabled: projectExtensionEnabled(e), Target: byName[target], Implementations: implementations, Consumers: e.OutputBindings})
		}
	}
	slices.SortFunc(details.Owners, func(a, b projectwire.CompositionSourceOwner) int { return strings.Compare(a.Source, b.Source) })
	source.Composition.Unified = details
	return result, provenance, nil
}
func filepathOwner(source projectPackageSource) string {
	for _, d := range source.Documents {
		if d.FolderRoot && d.Unified != nil {
			return d.Unified.Owner
		}
	}
	return "generated"
}

func projectValidateUnifiedBuild(build projectBuildProvenance, components []projectCompileComponent) error {
	invalid := func(message string) error { return refusal("project_compile_invalid_provenance", message) }
	if build.Algorithm != "b2" || projectUnifiedHash(build.Inputs) != build.BuildKey || build.Composition == nil || build.Composition.SchemaVersion != "workflow-composition/2" {
		return invalid("unified build identity differs")
	}
	if len(build.Components) != len(components) {
		return invalid("component inventory differs")
	}
	mappings := map[flow.Ref]projectBuildMapping{}
	byRef := map[flow.Ref]projectCompileComponent{}
	keys := []string{}
	roots := 0
	for _, c := range components {
		byRef[c.Ref] = c
	}
	for _, m := range build.Components {
		c, ok := byRef[m.CompiledRef]
		if !ok || c.Path != m.Path || c.Kind != m.Kind || m.AuthorRef.ID != m.CompiledRef.ID || mappings[m.CompiledRef].VariantKey != "" {
			return invalid("duplicate or swapped mapping")
		}
		if projectUnifiedHash(m.VariantInputs) != m.VariantKey || m.SourceOwner != m.VariantInputs["owner"] || m.Source != m.VariantInputs["source"] || m.SourceDigest != m.VariantInputs["source_digest"] || m.AuthorRef.ID != m.VariantInputs["author_id"] || m.AuthorRef.Version != m.VariantInputs["author_version"] || m.Kind != m.VariantInputs["kind"] {
			return invalid("source/author/variant proof differs")
		}
		version, e := projectUnifiedComponentVersion(build.BuildKey, m.Kind, m.AuthorRef, m.VariantKey)
		if e != nil {
			return e
		}
		if version != m.CompiledRef.Version {
			return invalid("component version differs from b2 tuple")
		}
		if c.Ref == build.RootRef && c.Kind == "workflow" && c.Root {
			roots++
		}
		mappings[m.CompiledRef] = m
		keys = append(keys, m.VariantKey)
	}
	slices.Sort(keys)
	if roots != 1 || !projectCompositionEqual(keys, build.Inputs["variants"]) {
		return invalid("root or variant inventory differs")
	}
	for _, m := range build.Components {
		c := byRef[m.CompiledRef]
		var document any
		var authorBytes []byte
		if c.Kind == "context" {
			if c.Resource == nil {
				return invalid("missing context metadata")
			}
			document = map[string]any{"bytes": c.Bytes, "media_type": c.Resource.MediaType, "byte_encoding": c.Resource.ByteEncoding}
			authorBytes = c.Bytes
		} else {
			value, err := flow.Parse(c.Bytes, "json")
			if err != nil {
				return err
			}
			object := value.(map[string]any)
			object["id"] = m.AuthorRef.ID
			object["version"] = m.AuthorRef.Version
			document = object
			if c.Kind != "schema" {
				document, err = projectMapDefinitionRefs(object, false, func(ref flow.Ref) (any, error) {
					child, ok := mappings[ref]
					if !ok {
						return projectRefValue(ref), nil
					}
					return map[string]any{"owner": child.SourceOwner, "kind": child.Kind, "author_id": child.AuthorRef.ID, "author_version": child.AuthorRef.Version, "variant_key": child.VariantKey}, nil
				})
				if err != nil {
					return err
				}
			}
			authored := any(object)
			if c.Kind != "schema" {
				authored, err = projectMapDefinitionRefs(object, false, func(ref flow.Ref) (any, error) {
					if child, ok := mappings[ref]; ok {
						return projectRefValue(child.AuthorRef), nil
					}
					return projectRefValue(ref), nil
				})
				if err != nil {
					return err
				}
			}
			authorBytes, err = projectCanonicalJSON(authored)
			if err != nil {
				return err
			}
		}
		digest := fmt.Sprintf("sha256:%x", sha256.Sum256(authorBytes))
		if c.Kind != "context" {
			var err error
			digest, err = flow.Digest(authorBytes)
			if err != nil {
				return err
			}
		}
		if digest != m.AuthorRef.Digest || !projectCompositionEqual(document, m.VariantInputs["document"]) {
			return invalid("variant does not describe its native component")
		}
	}
	if details := build.Composition.Unified; details == nil || len(details.Variants) != len(components) {
		return invalid("review variant inventory differs")
	} else {
		seen := map[flow.Ref]bool{}
		for _, v := range details.Variants {
			m, ok := mappings[v.CompiledRef]
			if !ok || seen[v.CompiledRef] || v.Owner != m.SourceOwner || v.Source != m.Source || v.AuthorRef != m.AuthorRef || v.VariantKey != m.VariantKey {
				return invalid("review swaps source/variant")
			}
			seen[v.CompiledRef] = true
		}
	}
	return nil
}
