package runtime

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// These are new exact definitions. None of the delivered configuration,
// adapter or transport definitions is modified in place.
func contextBuiltinDefinitions(coreConfiguration, localContext map[string]any) ([]PinnedDefinition, error) {
	clone := func(value map[string]any) (map[string]any, error) {
		data, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		var result map[string]any
		err = json.Unmarshal(data, &result)
		return result, err
	}
	configuration, err := clone(coreConfiguration)
	if err != nil {
		return nil, err
	}
	refSchema := map[string]any{"type": "object", "required": []string{"id", "version", "digest"}, "properties": map[string]any{
		"id": map[string]any{"type": "string", "minLength": 1, "maxLength": 128}, "version": map[string]any{"type": "string", "minLength": 1, "maxLength": 128}, "digest": map[string]any{"type": "string", "pattern": "^sha256:[0-9a-f]{64}$"},
	}, "additionalProperties": false}
	properties := configuration["properties"].(map[string]any)
	properties["schema_version"] = map[string]any{"const": CoreContextConfigVersion}
	properties["semantics_profile"] = map[string]any{"const": flow.CoreProfile}
	properties["executors"].(map[string]any)["additionalProperties"].(map[string]any)["properties"].(map[string]any)["context_profile_ref"] = refSchema
	transport, err := clone(localContext)
	if err != nil {
		return nil, err
	}
	properties = transport["properties"].(map[string]any)
	properties["schema_version"] = map[string]any{"const": "local-context/2"}
	artifactRef := map[string]any{"type": "object", "required": []string{"artifact_id", "revision", "digest"}, "properties": map[string]any{
		"artifact_id": map[string]any{"type": "string", "pattern": "^[A-Za-z][A-Za-z0-9._:/-]{0,127}$"}, "revision": map[string]any{"const": 1}, "digest": map[string]any{"type": "string", "pattern": "^sha256:[0-9a-f]{64}$"},
	}, "additionalProperties": false}
	slot := func(path any) map[string]any {
		return map[string]any{"type": "object", "required": []string{"ref", "path"}, "properties": map[string]any{"ref": artifactRef, "path": path}, "additionalProperties": false}
	}
	properties["manifest"] = slot(map[string]any{"const": "context/manifest.json"})
	properties["rendering"] = slot(map[string]any{"const": "context/rendered.json"})
	properties["sources"] = map[string]any{"type": "array", "maxItems": MaxContextReferences, "items": slot(map[string]any{"type": "string", "pattern": "^context/sources/[0-9]{3}$"})}
	// An input is the same materialized source, not an unaccounted second copy.
	properties["inputs"].(map[string]any)["additionalProperties"] = slot(map[string]any{"type": "string", "pattern": "^context/sources/[0-9]{3}$"})
	properties["dependencies"].(map[string]any)["maxItems"] = 1024
	transport["required"] = []string{"schema_version", "inputs", "outputs", "dependencies", "manifest", "rendering"}
	profileSchema := map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object", "required": []string{"schema_version", "id", "version", "assembly_ref", "max_bytes", "max_references", "max_tokens", "isolation_required", "truncation", "include_brief"}, "properties": map[string]any{
		"schema_version": map[string]any{"const": ContextProfileVersion}, "id": map[string]any{"type": "string", "minLength": 1, "maxLength": 128}, "version": map[string]any{"type": "string", "minLength": 1, "maxLength": 128}, "assembly_ref": refSchema,
		"max_bytes": map[string]any{"type": "integer", "minimum": 0, "maximum": MaxArtifactBytes}, "max_references": map[string]any{"type": "integer", "minimum": 0, "maximum": MaxContextReferences},
		"max_tokens":         map[string]any{"anyOf": []any{map[string]any{"type": "null"}, map[string]any{"type": "integer", "minimum": 1, "maximum": 100000000}}},
		"isolation_required": map[string]any{"enum": []string{"fresh", "declared_inherited"}}, "truncation": map[string]any{"enum": []string{"reject", "explicit_transform"}}, "include_brief": map[string]any{"type": "boolean"},
	}, "additionalProperties": false}
	manifestSchema, err := flow.ProtocolSchema("ContextManifest")
	if err != nil {
		return nil, err
	}
	evidenceSchema, err := flow.ProtocolSchema("Evidence")
	if err != nil {
		return nil, err
	}
	definitions := []PinnedDefinition{}
	add := func(id, version, kind string, value any) error {
		data, err := canonical(value)
		if err != nil {
			return err
		}
		ref := flow.Ref{ID: id, Version: version, Digest: rawDigest(data)}
		definitions = append(definitions, PinnedDefinition{Ref: ref, Kind: kind, RawDigest: ref.Digest, Bytes: data})
		return nil
	}
	for _, item := range []struct {
		id, version, kind string
		value             any
	}{
		{"core:schema/core-configuration", "2.0.0", "schema", configuration},
		{"core:schema/local-context", "2.0.0", "schema", transport},
		{"core:schema/context-profile", "1.0.0", "schema", profileSchema},
		{"core:schema/full-context", "1.0.0", "schema", json.RawMessage(manifestSchema)},
		{"core:schema/evidence", "1.0.0", "schema", json.RawMessage(evidenceSchema)},
		{"core:schema/context-json", "1.0.0", "schema", true},
		{"core:resolver/local", "2.0.0", "resource", map[string]any{"id": "core:resolver/local", "version": "2.0.0", "resolution": "explicit_local_files", "typed_context_resources": true}},
		{"core:adapter/local-process", "2.0.0", "adapter", map[string]any{"schema_version": "1", "id": "core:adapter/local-process", "version": "2.0.0", "execution_modes": []string{"managed"}, "operations": []string{"process", "check"}, "context_isolation": "unknown", "process_control": "cooperative", "effect_fencing": "cooperative", "sandbox": "none", "durable_wakeup": false, "qualification_evidence": []any{}}},
		{"core:assembly/local-json", "1.0.0", "resource", map[string]any{"id": "core:assembly/local-json", "version": "1.0.0", "format": ContextRenderingVersion, "byte_accounting": "source_copies_plus_rendering", "tokenized": false, "overflow": "reject"}},
	} {
		if err := add(item.id, item.version, item.kind, item.value); err != nil {
			return nil, err
		}
	}
	profile := ContextProfile{SchemaVersion: ContextProfileVersion, ID: "core:context/local-json", Version: "1.0.0", AssemblyRef: builtinRef(definitions, "core:assembly/local-json"), MaxBytes: MaxArtifactBytes, MaxReferences: MaxContextReferences, IsolationRequired: "declared_inherited", Truncation: "reject"}
	if err := add(profile.ID, profile.Version, "resource", profile); err != nil {
		return nil, err
	}
	return definitions, nil
}

func contextProfileFor(config ExecutorConfig, definitions []PinnedDefinition, registry flow.Registry) (ContextProfile, flow.Ref, error) {
	ref := builtinRef(definitions, "core:context/local-json")
	if config.ContextProfileRef != nil {
		ref = *config.ContextProfileRef
	}
	data, exists := registry[ref]
	if !exists {
		return ContextProfile{}, flow.Ref{}, faultf("missing_context_profile", "%s", ref.String())
	}
	if rawDigest(data) != ref.Digest {
		return ContextProfile{}, flow.Ref{}, fault("context_profile_drift", "profile bytes differ from their exact reference")
	}
	if err := flow.ValidateSchema(registry, builtinRef(definitions, "core:schema/context-profile"), data); err != nil {
		return ContextProfile{}, flow.Ref{}, err
	}
	var profile ContextProfile
	if err := decode(data, &profile); err != nil {
		return ContextProfile{}, flow.Ref{}, err
	}
	if profile.ID != ref.ID || profile.Version != ref.Version {
		return ContextProfile{}, flow.Ref{}, fault("context_profile_drift", "definition identity differs from its reference")
	}
	if err := ValidateContextProfile(profile, false); err != nil {
		return ContextProfile{}, flow.Ref{}, err
	}
	if profile.AssemblyRef != builtinRef(definitions, "core:assembly/local-json") {
		return ContextProfile{}, flow.Ref{}, fault("unsupported_context_assembly", "only the exact local JSON assembly is supported")
	}
	if profile.IsolationRequired == "fresh" {
		return ContextProfile{}, flow.Ref{}, fault("unsupported_context_isolation", "local process isolation is not qualified as fresh")
	}
	if profile.MaxTokens != nil {
		return ContextProfile{}, flow.Ref{}, fault("unsupported_tokenization", "local JSON assembly does not qualify token budgets or AI dispatch")
	}
	return profile, ref, nil
}

func (e *Engine) selectContextProfiles(plan *flow.Plan, definitions []PinnedDefinition, registry flow.Registry) error {
	if !requiresContextState(plan) {
		return nil
	}
	adapter := builtinVersionRef(definitions, "core:adapter/local-process", "2.0.0")
	for ref := range plan.Checks {
		kind := ""
		for _, definition := range definitions {
			if definition.Ref == ref {
				kind = definition.Kind
			}
		}
		if kind != "check" {
			return fault("check_definition_kind", "automatic check requires a typed check registry entry")
		}
	}
	for ref, executor := range executorDefinitions(plan) {
		// An assisted step is performed by a host session, so it binds no
		// executable and needs no pinned executor configuration.
		if isAssistedExecutor(definitions, executor) {
			continue
		}
		config, exists := e.Config.Configuration.Executors[ref.ID]
		if !exists {
			return faultf("missing_executor", "%s", ref.ID)
		}
		if executor.AdapterRef != adapter {
			if config.ContextProfileRef != nil {
				return fault("unsupported_context_adapter", "context profiles require local-process@2.0.0")
			}
			continue
		}
		profile, profileRef, err := contextProfileFor(config, definitions, registry)
		if err != nil {
			return err
		}
		kind := ""
		for _, definition := range definitions {
			if definition.Ref == profileRef {
				kind = definition.Kind
				break
			}
		}
		if kind != "resource" {
			return fault("context_profile_type_mismatch", "context profile must be a JSON resource definition")
		}
		// Configuration references are selected explicitly at Start, not
		// discovered by walking arbitrary fields of resource JSON data.
		plan.Registry[profileRef] = registry[profileRef]
		plan.Registry[profile.AssemblyRef] = registry[profile.AssemblyRef]
	}
	return nil
}

func executorDefinitions(plan *flow.Plan) map[flow.Ref]flow.Executor {
	result := map[flow.Ref]flow.Executor{}
	for _, workflow := range workflowPlans(plan) {
		for id, step := range workflow.Steps {
			ref := workflow.Workflow.Definition.Stages[id].StepRef
			result[ref] = flow.Executor{AdapterRef: step.Executor.AdapterRef, Operation: step.Executor.Operation}
		}
	}
	for ref, check := range plan.Checks {
		result[ref] = check.Executor
	}
	return result
}

// The lock covers interpretation as well as bytes: raw `true` and JSON `true`
// have equal content digests but may never change encoding under an old Run.
func contextResourceSnapshot(resources []PinnedResource) (PinnedDefinition, error) {
	pins := append([]PinnedResource{}, resources...)
	sort.Slice(pins, func(i, j int) bool { return pins[i].Ref.String() < pins[j].Ref.String() })
	records := make([]map[string]any, 0, len(pins))
	for _, pin := range pins {
		records = append(records, map[string]any{"ref": pin.Ref, "kind": "resource", "raw_digest": pin.RawDigest, "byte_encoding": pin.ByteEncoding, "media_type": pin.MediaType})
	}
	data, err := canonical(map[string]any{"schema_version": "context-resources/1", "resources": records})
	if err != nil {
		return PinnedDefinition{}, err
	}
	digest := rawDigest(data)
	ref := flow.Ref{ID: "snapshot:context-resources/" + strings.TrimPrefix(digest, "sha256:"), Version: "1.0.0", Digest: digest}
	return PinnedDefinition{Ref: ref, Kind: "resource", RawDigest: digest, Bytes: data}, nil
}

func (e *Engine) prepareFullContext(r Run, step flow.StepDefinition, attemptID, commandID string, profile ContextProfile, inputs map[string]ArtifactRef) (FullContextManifest, []ContextSource, Artifact, error) {
	return e.prepareContext(r, step.InstructionsRef, step.ContextRefs, attemptID, commandID, profile, inputs, nil)
}

func (e *Engine) prepareContext(r Run, instructionsRef *flow.Ref, contextRefs []flow.Ref, executionID, commandID string, profile ContextProfile, inputs map[string]ArtifactRef, prepared map[ArtifactRef]Artifact) (FullContextManifest, []ContextSource, Artifact, error) {
	manifest := FullContextManifest{SchemaVersion: "1", ID: derivedID("context", executionID), Version: "1.0.0", Entries: []FullContextEntry{}, IsolationRequired: profile.IsolationRequired, MaxBytes: profile.MaxBytes, MaxTokens: profile.MaxTokens, Truncation: profile.Truncation, AssemblyRef: profile.AssemblyRef}
	sources, provenance := []ContextSource{}, []ArtifactRef{}
	seen := map[ArtifactRef]bool{}
	var sourceBytes int64
	add := func(id, role, trust string, artifact Artifact, data []byte) error {
		if int64(len(sources)) >= profile.MaxReferences {
			return local.Reject("context_reference_limit", "selected context exceeds the pinned reference budget")
		}
		if int64(len(data)) > profile.MaxBytes-sourceBytes {
			return local.Reject("context_byte_limit", "selected source copies exceed the pinned context budget")
		}
		sourceBytes += int64(len(data))
		manifest.Entries = append(manifest.Entries, FullContextEntry{SourceID: id, ArtifactRef: artifact.Ref(), Role: role, Trust: trust, Classification: artifact.Classification})
		sources = append(sources, ContextSource{Artifact: artifact, Bytes: data})
		// Provisional subjects are not accepted ArtifactRevisions. Their exact
		// metadata/provenance lives in PendingAcceptance; manifest entries and
		// CheckRequest bind their bytes without publishing them as accepted.
		_, provisional := prepared[artifact.Ref()]
		if !seen[artifact.Ref()] && !provisional {
			provenance = append(provenance, artifact.Ref())
			seen[artifact.Ref()] = true
		}
		return nil
	}
	resources := map[flow.Ref]PinnedResource{}
	for _, resource := range r.ContextResources {
		resources[resource.Ref] = resource
	}
	addResource := func(id, role, trust string, ref flow.Ref) error {
		resource, ok := resources[ref]
		if !ok {
			return local.Reject("context_resource_missing", "the selected context resource is not pinned in this Run")
		}
		format := "blob"
		var schema *flow.Ref
		if resource.ByteEncoding == "json" {
			format = "json"
			value := builtinRef(r.Definitions, "core:schema/context-json")
			schema = &value
		}
		identity := derivedID("import", r.AuthorityID, ref.String(), resource.ByteEncoding, resource.MediaType)
		artifact, err := e.putArtifact(resource.Bytes, format, schema, derivedID("artifact", identity), map[string]any{"kind": "import", "import_id": identity, "principal_id": e.owner, "source_ref": ref}, nil, r.registry(), resource.MediaType)
		if err != nil {
			return err
		}
		return add(id, role, trust, artifact, resource.Bytes)
	}
	if instructionsRef != nil {
		if err := addResource("instructions", "instruction", "trusted_instruction", *instructionsRef); err != nil {
			return manifest, nil, Artifact{}, err
		}
	}
	for i, ref := range contextRefs {
		if err := addResource(fmt.Sprintf("context:%d", i), "reference", "user_data", ref); err != nil {
			return manifest, nil, Artifact{}, err
		}
	}
	ports := make([]string, 0, len(inputs))
	for port := range inputs {
		ports = append(ports, port)
	}
	sort.Strings(ports)
	for _, port := range ports {
		artifact, data, err := e.contextArtifact(inputs[port], prepared)
		if err != nil {
			return manifest, nil, Artifact{}, err
		}
		trust := "user_data"
		switch artifact.Producer["kind"] {
		case "step":
			trust = "generated_data"
		case "import":
			trust = "external_data"
		}
		if err := add("input:"+port, "data", trust, artifact, data); err != nil {
			return manifest, nil, Artifact{}, err
		}
	}
	if profile.IncludeBrief {
		artifact, data, err := e.Artifact(r.Brief)
		if err != nil {
			return manifest, nil, Artifact{}, err
		}
		if err := add("brief", "data", "user_data", artifact, data); err != nil {
			return manifest, nil, Artifact{}, err
		}
	}
	data, err := validateFullContextManifest(manifest)
	if err != nil {
		return manifest, nil, Artifact{}, err
	}
	schema := builtinRef(r.Definitions, "core:schema/full-context")
	artifact, err := e.putArtifact(data, "json", &schema, derivedID("artifact", executionID, "context"), map[string]any{"kind": "authority", "authority_id": r.AuthorityID, "command_id": commandID, "port": "context"}, provenance, r.registry())
	return manifest, sources, artifact, err
}

func (e *Engine) contextArtifact(ref ArtifactRef, prepared map[ArtifactRef]Artifact) (Artifact, []byte, error) {
	if artifact, ok := prepared[ref]; ok {
		if artifact.Ref() != ref || artifact.SizeBytes < 0 || artifact.SizeBytes > MaxArtifactBytes {
			return Artifact{}, nil, local.ErrIntegrity
		}
		data, err := e.Blobs.Read(local.BlobRef{Digest: ref.Digest, Size: artifact.SizeBytes})
		return artifact, data, err
	}
	return e.Artifact(ref)
}

// verifyFullWorkspaceContext reads only the already materialized workspace.
// Inputs reuse source files; no live registry, profile or source is consulted.
func verifyFullWorkspaceContext(workspace string, transport ContextManifest, manifestRef ArtifactRef, inputs map[string]ArtifactRef, profile *ContextProfile) error {
	invalid := func() error {
		return local.Reject("context_manifest_drift", "materialized full context differs from the pinned admission")
	}
	if profile == nil || transport.SchemaVersion != "local-context/2" || transport.Manifest == nil || transport.Rendering == nil || transport.Manifest.Ref != manifestRef || transport.Manifest.Path != "context/manifest.json" || transport.Rendering.Path != "context/rendered.json" {
		return invalid()
	}
	data, err := readLocal(workspace, transport.Manifest.Path, MaxDefinitionBytes)
	if err != nil || rawDigest(data) != manifestRef.Digest || flow.ValidateProtocol("ContextManifest", data) != nil {
		return invalid()
	}
	var manifest FullContextManifest
	if decode(data, &manifest) != nil || manifest.MaxBytes != profile.MaxBytes || manifest.MaxTokens != nil || manifest.AssemblyRef != profile.AssemblyRef || manifest.IsolationRequired != profile.IsolationRequired || manifest.Truncation != profile.Truncation || int64(len(manifest.Entries)) > profile.MaxReferences || len(manifest.Entries) != len(transport.Sources) {
		return invalid()
	}
	want, err := canonical(manifest)
	if err != nil || !bytes.Equal(data, want) || len(inputs) != len(transport.Inputs) {
		return invalid()
	}
	rendering, err := readLocal(workspace, transport.Rendering.Path, MaxArtifactBytes)
	if err != nil || rawDigest(rendering) != transport.Rendering.Ref.Digest || int64(len(rendering)) > profile.MaxBytes {
		return invalid()
	}
	remaining := profile.MaxBytes - int64(len(rendering))
	seenInputs := map[string]bool{}
	for i, entry := range manifest.Entries {
		source := transport.Sources[i]
		if source.Ref != entry.ArtifactRef || source.Path != ContextSourcePath(i) {
			return invalid()
		}
		data, err := readLocal(workspace, source.Path, MaxArtifactBytes)
		if err != nil || rawDigest(data) != source.Ref.Digest || int64(len(data)) > remaining {
			return invalid()
		}
		remaining -= int64(len(data))
		if strings.HasPrefix(entry.SourceID, "input:") {
			port := strings.TrimPrefix(entry.SourceID, "input:")
			ref, exists := inputs[port]
			if !exists || seenInputs[port] || transport.Inputs[port] != source || ref != source.Ref {
				return invalid()
			}
			seenInputs[port] = true
		}
	}
	if len(seenInputs) != len(inputs) {
		return invalid()
	}
	return nil
}

func hasContextStateFields(r Run) bool {
	if r.ContextResources != nil || r.CheckExecutions != nil || r.ActiveCheckID != "" || r.PendingAcceptance != nil || r.PendingArtifactPublication != nil {
		return true
	}
	for _, executor := range r.Executors {
		if executor.ContextProfile != nil || executor.Config.ContextProfileRef != nil {
			return true
		}
	}
	for _, attempt := range r.Attempts {
		if attempt != nil && (attempt.Context.SchemaVersion == "local-context/2" || attempt.Context.Manifest != nil || attempt.Context.Rendering != nil || attempt.Context.Sources != nil) {
			return true
		}
	}
	return false
}

func contextWireFields(fields map[string]json.RawMessage, version string) error {
	check := func(record map[string]json.RawMessage, allowed bool, names ...string) error {
		for _, name := range names {
			if value, exists := record[name]; exists && (!allowed || bytes.Equal(bytes.TrimSpace(value), []byte("null"))) {
				return errors.New("context contract fields require their new version and non-null values")
			}
		}
		return nil
	}
	current := isContextState(version)
	if err := check(fields, current, "context_resources", "check_executions", "active_check_execution_id", "pending_acceptance"); err != nil {
		return err
	}
	var executors map[string]map[string]json.RawMessage
	if err := json.Unmarshal(fields["executors"], &executors); err != nil {
		return err
	}
	for _, executor := range executors {
		if err := check(executor, current, "context_profile"); err != nil {
			return err
		}
		var configuration map[string]json.RawMessage
		if err := json.Unmarshal(executor["config"], &configuration); err != nil {
			return err
		}
		if err := check(configuration, current, "context_profile_ref"); err != nil {
			return err
		}
	}
	var attempts map[string]map[string]json.RawMessage
	if err := json.Unmarshal(fields["attempts"], &attempts); err != nil {
		return err
	}
	for _, attempt := range attempts {
		var transport map[string]json.RawMessage
		if err := json.Unmarshal(attempt["context"], &transport); err != nil {
			return err
		}
		allowed := current && string(transport["schema_version"]) == `"local-context/2"`
		if err := check(transport, allowed, "manifest", "rendering", "sources"); err != nil {
			return err
		}
		// A handoff belongs to session state alone; an older Run may not carry
		// one, and a present-null is not an omitted optional field.
		if err := check(attempt, isSessionState(version), "session"); err != nil {
			return err
		}
	}
	return nil
}

// A state revision is not permission to reinterpret pinned bytes. The exact
// lock covers resource encodings and profiles as well as executable bindings.
func contextPinnedInvariant(r Run) error {
	if !isContextState(r.SchemaVersion) {
		return nil
	}
	invalid := func() error {
		return local.Reject("context_pin_drift", "context resources, executor configuration or lock differ from the pinned Run")
	}
	if _, err := resourcesFromPins(r.ContextResources); err != nil {
		return err
	}
	definitions := map[flow.Ref]PinnedDefinition{}
	closure := []flow.Ref{}
	for _, definition := range r.Definitions {
		if _, exists := definitions[definition.Ref]; exists || definition.Ref.Digest != rawDigest(definition.Bytes) {
			return invalid()
		}
		definitions[definition.Ref] = definition
		if definition.Ref != r.LockRef {
			closure = append(closure, definition.Ref)
		}
	}
	for _, resource := range r.ContextResources {
		if _, exists := definitions[resource.Ref]; exists {
			return invalid()
		}
		closure = append(closure, resource.Ref)
	}
	// A version at or above the context state does not by itself make a Run a
	// context Run: a fan-out reaches this version without declaring resources.
	// The class is not read from a field that an empty value could erase; the
	// Run must match exactly one pinned shape, and each shape is fully checked.
	snapshot, err := contextResourceSnapshot(r.ContextResources)
	if err != nil {
		return err
	}
	sort.Slice(closure, func(i, j int) bool { return closure[i].String() < closure[j].String() })
	matched := false
	for _, contextClass := range []bool{true, false} {
		resolver, configVersion, schemaRef := builtinRef(r.Definitions, "core:resolver/local"), "core-run-configuration/2", builtinRef(r.Definitions, "core:schema/core-configuration")
		if contextClass {
			resolver, configVersion, schemaRef = builtinVersionRef(r.Definitions, "core:resolver/local", "2.0.0"), "core-run-configuration/3", builtinVersionRef(r.Definitions, "core:schema/core-configuration", "2.0.0")
		}
		expectedLock, err := canonical(map[string]any{"schema_version": "1", "id": r.LockRef.ID, "version": r.LockRef.Version, "core_protocol": "1", "closure": closure, "resolver_ref": resolver})
		if err != nil {
			return err
		}
		configuration, err := canonical(map[string]any{"schema_version": configVersion, "semantics_profile": r.Profile, "configuration_schema_ref": schemaRef, "executors": r.Executors, "effective_configuration": r.EffectiveConfiguration, "workflow_configurations": r.WorkflowConfigurations})
		if err != nil {
			return err
		}
		digest := rawDigest(configuration)
		ref := flow.Ref{ID: "snapshot:executors/" + strings.TrimPrefix(digest, "sha256:"), Version: "1.0.0", Digest: digest}
		_, pinnedSnapshot := definitions[snapshot.Ref]
		if !bytes.Equal(definitions[r.LockRef].Bytes, expectedLock) || !bytes.Equal(definitions[ref].Bytes, configuration) {
			continue
		}
		if contextClass && !bytes.Equal(definitions[snapshot.Ref].Bytes, snapshot.Bytes) || !contextClass && pinnedSnapshot {
			continue
		}
		matched = true
		break
	}
	if !matched {
		return invalid()
	}
	for _, executor := range r.Executors {
		if executor.ContextProfile == nil && executor.Config.ContextProfileRef == nil {
			continue
		}
		if executor.ContextProfile == nil || executor.Config.ContextProfileRef == nil {
			return invalid()
		}
		profile := *executor.ContextProfile
		data, err := canonical(profile)
		definition := definitions[*executor.Config.ContextProfileRef]
		if err != nil || definition.Kind != "resource" || !bytes.Equal(data, definition.Bytes) || ValidateContextProfile(profile, false) != nil || profile.MaxTokens != nil || profile.IsolationRequired != "declared_inherited" || profile.AssemblyRef != builtinRef(r.Definitions, "core:assembly/local-json") {
			return invalid()
		}
	}
	return nil
}
