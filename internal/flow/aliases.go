package flow

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
)

// ResolveWorkflowAliases is a local pre-lock authoring boundary. A call's
// workflow_ref or repeat's body_workflow_ref may contain the closed
// {"alias":"name"} selector. Alias values
// are unsealed JSON workflow bytes; callers convert a YAML file with JSONBytes,
// not Canonical, so exact condition literals survive until their preflight.
// Immutable registry documents are never rewritten. No file, network or host
// lookup occurs here. CompileProfile must still validate the resolved closure.
// An unchanged root keeps its original representation/bytes; a changed root is
// JSON (also valid under the YAML facade). The input inventory is never mutated.
func ResolveWorkflowAliases(data []byte, format string, registry Registry, aliases map[string][]byte) ([]byte, Registry, error) {
	if len(registry) > 1024 || len(aliases) > 1024 {
		return nil, nil, problem("dependency_limit", "", "local alias inventory exceeds 1024 entries")
	}
	r := aliasResolver{registry: make(Registry), aliases: aliases, resolved: make(map[string]Ref), active: make(map[string]bool), activeIdentities: make(map[definitionIdentity]bool), identities: make(map[definitionIdentity]Ref)}
	for _, ref := range registryRefs(registry) {
		value := registry[ref]
		if err := r.charge(len(value)); err != nil {
			return nil, nil, err
		}
		key := definitionIdentity{ref.ID, ref.Version}
		if before, exists := r.identities[key]; exists && before != ref {
			return nil, nil, problem("ref_identity_conflict", "", "local inventory has conflicting identity/version content")
		}
		r.identities[key], r.registry[ref] = ref, bytes.Clone(value)
	}
	root, _, changed, err := r.workflow(data, format)
	if err != nil {
		return nil, nil, err
	}
	if !changed {
		root = bytes.Clone(data)
	}
	return root, r.registry, nil
}

type aliasResolver struct {
	registry         Registry
	aliases          map[string][]byte
	resolved         map[string]Ref
	active           map[string]bool
	activeIdentities map[definitionIdentity]bool
	identities       map[definitionIdentity]Ref
	bytes, documents int
}

func (r *aliasResolver) charge(size int) error {
	r.bytes += size
	r.documents++
	if size > MaxDocumentBytes || r.bytes > 64<<20 || r.documents > 1024 {
		return problem("dependency_limit", "", "alias closure exceeds bounded document count or bytes")
	}
	return nil
}

func (r *aliasResolver) alias(name string, path string) (Ref, error) {
	if r.active[name] {
		return Ref{}, problem("alias_cycle", path, "local alias graph re-enters an active workflow")
	}
	if ref, exists := r.resolved[name]; exists {
		return ref, nil
	}
	data, exists := r.aliases[name]
	if !exists {
		return Ref{}, problem("unknown_alias", path, "local workflow alias is not supplied")
	}
	r.active[name] = true
	defer delete(r.active, name)
	resolved, ref, _, err := r.workflow(data, "json")
	if err != nil {
		return Ref{}, err
	}
	r.registry[ref], r.resolved[name] = resolved, ref
	return ref, nil
}

func (r *aliasResolver) workflow(data []byte, format string) ([]byte, Ref, bool, error) {
	if err := r.charge(len(data)); err != nil {
		return nil, Ref{}, false, err
	}
	if len(r.activeIdentities) > MaxDepth {
		return nil, Ref{}, false, problem("dependency_limit", "", "alias workflow nesting exceeds depth 64")
	}
	value, authoring, err := workflowValue(data, format)
	if err != nil {
		return nil, Ref{}, false, err
	}
	if err := preflightConditions("WorkflowRevision", value, ""); err != nil {
		return nil, Ref{}, false, err
	}
	object, _ := value.(map[string]any)
	id, _ := object["id"].(string)
	version, _ := object["version"].(string)
	ref := Ref{ID: id, Version: version, Digest: "sha256:" + strings.Repeat("0", 64)}
	encodedRef, _ := json.Marshal(ref)
	if err := ValidateProtocol("ImmutableRef", encodedRef); err != nil {
		return nil, Ref{}, false, err
	}
	key := definitionIdentity{id, version}
	if r.activeIdentities[key] {
		return nil, Ref{}, false, problem("alias_cycle", "/definition", "alias resolves to an active workflow identity/version")
	}
	r.activeIdentities[key] = true
	defer delete(r.activeIdentities, key)
	definition, _ := object["definition"].(map[string]any)
	stages, _ := definition["stages"].(map[string]any)
	changed := authoring
	for _, stageID := range keys(stages) {
		stage, _ := stages[stageID].(map[string]any)
		field := "workflow_ref"
		if stage["kind"] == "repeat" {
			field = "body_workflow_ref"
		} else if stage["kind"] != "call" {
			continue
		}
		path := "/definition/stages/" + escapePointer(stageID) + "/" + field
		target, _ := stage[field].(map[string]any)
		if selector, exists := target["alias"]; exists {
			name, valid := selector.(string)
			nameBytes, _ := json.Marshal(name)
			if len(target) != 1 || !valid || ValidateProtocol("Identifier", nameBytes) != nil {
				return nil, Ref{}, false, problem("invalid_alias", path, "workflow alias must be one bounded local identifier")
			}
			child, err := r.alias(name, path)
			if err != nil {
				return nil, Ref{}, false, err
			}
			stage[field] = map[string]any{"id": child.ID, "version": child.Version, "digest": child.Digest}
			changed = true
		} else {
			childID, _ := target["id"].(string)
			childVersion, _ := target["version"].(string)
			if r.activeIdentities[definitionIdentity{childID, childVersion}] {
				return nil, Ref{}, false, problem("call_cycle", path, "call re-enters an active workflow identity/version")
			}
		}
	}
	contract := "WorkflowRevision"
	switch object["schema_version"] {
	case "2":
		contract = "WorkflowRevisionV2"
	case "3":
		contract = "WorkflowRevisionV3"
	}
	if err := validateProtocolValue(contract, value, ""); err != nil {
		return nil, Ref{}, false, err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, Ref{}, false, err
	}
	canonical, err := Canonical(encoded)
	if err != nil {
		return nil, Ref{}, false, err
	}
	ref.Digest, err = Digest(canonical)
	if err != nil {
		return nil, Ref{}, false, err
	}
	if before, exists := r.identities[key]; exists && before != ref {
		return nil, Ref{}, false, problem("ref_identity_conflict", "", "resolved workflow conflicts with existing identity/version content")
	}
	r.identities[key] = ref
	return canonical, ref, changed, nil
}

func registryRefs(registry Registry) []Ref {
	refs := make([]Ref, 0, len(registry))
	for ref := range registry {
		refs = append(refs, ref)
	}
	slices.SortFunc(refs, func(a, b Ref) int { return strings.Compare(a.String(), b.String()) })
	return refs
}
