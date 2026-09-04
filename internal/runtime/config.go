package runtime

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cyberphone/json-canonicalization/go/src/webpki.org/jsoncanonicalizer"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

var EventTypes = []string{"run.created", "stage.activated", "attempt.admitted", "attempt.dispatching", "attempt.started", "attempt.result_candidate", "attempt.cost_reported", "attempt.observed", "attempt.settled", "attempt.resolved", "check.resolved", "run.finished", "run.restricted", "stop.released", "run.resumed", "run.recovered", "step.publication", "artifact.publication_prepared", "artifact.publication_checked", "artifact.publication_checks_failed", "action.intent_proposed", "action.admitted", "diagnostic.recorded", "stage.failed", "stage.error_handled", "stage.choice_decided", "invocation.created", "invocation.finished", "stage.call_returned", "stage.repeat_entered", "stage.repeat_decided", "stage.parallel_entered", "stage.join_decided", "stage.map_entered", "stage.map_empty", "stage.wait_entered", "stage.wait_resolved", "wait.event_received", "wait.reserved", "guard.observed", "guard.processed", "run.context_pinned", "check.admitted", "check.dispatching", "check.started", "check.observed", "check.settled", "check.recovered", "acceptance.prepared", "acceptance.passed", "acceptance.failed", "attempt.accepted", "decision.requested", "decision.defaulted", "decision.answered"}

type Engine struct {
	Root         string
	Config       ProjectConfig
	Installation Installation
	Store        *local.Store
	Blobs        *local.BlobStore
	ReadOnly     bool
	clock        clock
	owner        string
	// Trusted packages are read once when the engine opens. A package trusted
	// by another process becomes resolvable to the next command, never inside a
	// command that already computed its dependency closure.
	packages []PackageEntry
}

func canonical(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return flow.Canonical(b)
}
func decode(data []byte, value any) error {
	b, err := flow.Canonical(data)
	if err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return fmt.Errorf("invalid closed contract: %w", err)
	}
	return nil
}

// Internal snapshots are produced by typed core transitions, not accepted as a
// worker wire payload. They have their own bounded size; wire limits stay strict.
func canonicalState(value any) ([]byte, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if len(b) > local.MaxSnapshotBytes {
		return nil, errors.New("state size exceeds storage budget")
	}
	return jsoncanonicalizer.Transform(b)
}
func decodeState(data []byte, value any) error {
	if len(data) > local.MaxSnapshotBytes {
		return errors.New("state size exceeds storage budget")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("invalid state framing")
	}
	if r, ok := value.(*Run); ok {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(data, &fields); err != nil {
			return err
		}
		if err := contextWireFields(fields, r.SchemaVersion); err != nil {
			return err
		}
		// A guard field in an older wire contract is refused, including an
		// explicit null: a state that names a rule its version cannot describe
		// would be read by an older core as a Run with no rule at all.
		if _, exists := fields["guards"]; exists && !isGuardState(r.SchemaVersion) {
			return errors.New("older state cannot contain live guard registrations")
		}
		if _, exists := fields["artifact_closures"]; exists && !isArtifactClosureState(r.SchemaVersion) {
			return errors.New("older state cannot contain artifact closures")
		}
		if _, exists := fields["pending_artifact_publication"]; exists && !isPublicationChecksState(r.SchemaVersion) {
			return errors.New("older state cannot contain pending artifact publication checks")
		}
		if _, exists := fields["action_intents"]; exists && !isActionIntentState(r.SchemaVersion) {
			return errors.New("older state cannot contain action proposals")
		}
		if _, exists := fields["action_admissions"]; exists && !isActionAdmissionState(r.SchemaVersion) {
			return errors.New("older state cannot contain action admissions")
		}
		if _, exists := fields["action_deliveries"]; exists && !isActionDeliveryState(r.SchemaVersion) {
			return errors.New("older state cannot contain action deliveries")
		}
		if _, exists := fields["fork"]; exists && !isForkState(r.SchemaVersion) {
			return errors.New("older state cannot contain fork provenance")
		}
		if !isDecisionState(r.SchemaVersion) {
			for _, field := range []string{"decision_catalog", "decision_sheet", "decision_ledger", "pending_decision"} {
				if _, exists := fields[field]; exists {
					return errors.New("older state cannot contain decision contract fields")
				}
			}
		}
		if !isPublicationNewOnlyState(r.SchemaVersion) {
			for _, collection := range []string{"wait_registrations", "publication_subscriptions"} {
				var records map[string]map[string]json.RawMessage
				if raw := fields[collection]; len(raw) != 0 {
					if err := json.Unmarshal(raw, &records); err != nil {
						return err
					}
				}
				for _, record := range records {
					if _, exists := record["publication_start_sequence"]; exists {
						return errors.New("older state cannot contain new-only publication start sequence")
					}
				}
			}
		}
		if _, exists := fields["ready_stages"]; exists && isInvocationState(r.SchemaVersion) {
			return errors.New("invocation state stores ready stages only in workflow invocations")
		}
		if !isRepeatState(r.SchemaVersion) {
			// Shared Go structs understand the new fields, but their presence
			// is forbidden in an older wire contract, including explicit null.
			for collection, field := range map[string]string{"activations": "repeat", "invocations": "iteration"} {
				var records map[string]map[string]json.RawMessage
				if raw := fields[collection]; len(raw) != 0 {
					if err := json.Unmarshal(raw, &records); err != nil {
						return err
					}
				}
				for _, record := range records {
					if _, exists := record[field]; exists {
						return errors.New("older state cannot contain repeat contract fields")
					}
				}
			}
		}
		if r.SchemaVersion == StateVersion || r.SchemaVersion == CoreStateVersion {
			for _, name := range []string{"invocations", "workflow_configurations"} {
				if _, exists := fields[name]; exists {
					return errors.New("legacy state cannot contain invocation contract fields")
				}
			}
			var stops []map[string]json.RawMessage
			if raw := fields["stops"]; len(raw) != 0 {
				if err := json.Unmarshal(raw, &stops); err != nil {
					return err
				}
			}
			for _, stop := range stops {
				if _, exists := stop["scope"]; exists {
					return errors.New("legacy state cannot contain scoped stops")
				}
				if _, exists := stop["scope_id"]; exists {
					return errors.New("legacy state cannot contain scoped stops")
				}
			}
		}
	}
	return nil
}
func safeRelative(path string) bool {
	return path != "" && path != "." && filepath.IsLocal(path) && filepath.Clean(path) == path && !strings.Contains(path, "\\") && !strings.ContainsRune(path, 0)
}
func readLocal(rootDir, path string, limit int64) ([]byte, error) {
	if !safeRelative(path) {
		return nil, local.ErrUnsafePath
	}
	r, err := os.OpenRoot(rootDir)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	// Reject symlinks even when their current target happens to be under root.
	parts := strings.Split(filepath.ToSlash(path), "/")
	current := ""
	for _, p := range parts {
		current = filepath.Join(current, p)
		st, err := r.Lstat(current)
		if err != nil {
			return nil, err
		}
		if st.Mode()&os.ModeSymlink != 0 || (current == path && !st.Mode().IsRegular()) {
			return nil, local.ErrUnsafePath
		}
	}
	f, err := r.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() || st.Size() > limit {
		return nil, local.ErrUnsafePath
	}
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if int64(len(b)) > limit {
		return nil, local.ErrBlobLimit
	}
	return b, err
}
func writeExclusive(path string, data []byte) error {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer root.Close()
	staging := ".pending-" + newID("file")
	f, err := root.OpenFile(staging, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close(); _ = root.Remove(staging) }()
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := root.Link(staging, filepath.Base(path)); err != nil {
		return err
	}
	d, err := root.Open(".")
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// Definition paths are data, never permission to relocate authority files into
// a workspace or follow a directory symlink. os.Root also fences path races.
func checkDirectory(root, path string) error {
	r, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer r.Close()
	current := ""
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		current = filepath.Join(current, part)
		st, err := r.Lstat(current)
		if err != nil {
			return err
		}
		if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return local.ErrUnsafePath
		}
	}
	return nil
}
func overlaps(a, b string) bool {
	return a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

var builtinDefinitions struct {
	sync.Once
	defs     []PinnedDefinition
	registry flow.Registry
	err      error
}

// Builtins are core contracts, not installed workflow packages. They contain no
// executable code and init never installs or runs a default workflow.
func Builtins() ([]PinnedDefinition, flow.Registry, error) {
	builtinDefinitions.Do(func() {
		builtinDefinitions.defs, builtinDefinitions.registry, builtinDefinitions.err = buildBuiltins()
	})
	if builtinDefinitions.err != nil {
		return nil, nil, builtinDefinitions.err
	}
	copyDefs := make([]PinnedDefinition, len(builtinDefinitions.defs))
	copyRegistry := make(flow.Registry, len(builtinDefinitions.registry))
	for index, definition := range builtinDefinitions.defs {
		copyDefs[index] = definition
		copyDefs[index].Bytes = bytes.Clone(definition.Bytes)
	}
	for ref, data := range builtinDefinitions.registry {
		copyRegistry[ref] = bytes.Clone(data)
	}
	return copyDefs, copyRegistry, nil
}

func buildBuiltins() ([]PinnedDefinition, flow.Registry, error) {
	resultSchema, err := flow.ProtocolSchema("StepResult")
	if err != nil {
		return nil, nil, err
	}
	// The summary a parallel stage produces is a shipped standard form. A
	// project is free to project it into a shape of its own; it cannot ask this
	// build to report facts it does not have.
	aggregateSchema, err := flow.ProtocolSchema("AggregateManifest")
	if err != nil {
		return nil, nil, err
	}
	artifactManifestSchema, err := PublicSchema("ArtifactManifest")
	if err != nil {
		return nil, nil, err
	}
	publicationHandleSchema, publicationCursorSchema, publicationDeliverySchema, err := publicationTransportSchemas()
	if err != nil {
		return nil, nil, err
	}
	type item struct {
		id, kind string
		value    any
	}
	refSchema := map[string]any{"type": "object", "required": []string{"id", "version", "digest"}, "properties": map[string]any{"id": map[string]any{"type": "string"}, "version": map[string]any{"type": "string"}, "digest": map[string]any{"type": "string"}}, "additionalProperties": false}
	configurationSchema := map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object", "required": []string{"schema_version", "semantics_profile", "trust_profile", "state_root", "artifact_root", "workspace_root", "registry_file", "executors"}, "properties": map[string]any{
		"schema_version": map[string]any{"const": "1"}, "semantics_profile": map[string]any{"const": flow.Profile}, "trust_profile": map[string]any{"const": "core-local/cooperative"},
		"state_root": map[string]any{"type": "string"}, "artifact_root": map[string]any{"type": "string"}, "workspace_root": map[string]any{"type": "string"}, "registry_file": map[string]any{"type": "string"},
		"executors": map[string]any{"type": "object", "maxProperties": 256, "additionalProperties": map[string]any{"type": "object", "required": []string{"executable", "args", "files", "environment", "timeout_ms", "grace_ms", "max_output_bytes"}, "properties": map[string]any{
			"executable": map[string]any{"type": "string", "minLength": 1}, "args": map[string]any{"type": "array", "maxItems": 256, "items": map[string]any{"type": "string", "maxLength": 4096}},
			"files": map[string]any{"type": "object", "maxProperties": 128, "additionalProperties": map[string]any{"type": "string"}}, "environment": map[string]any{"type": "object", "maxProperties": 64, "additionalProperties": map[string]any{"type": "string", "maxLength": 4096}},
			"timeout_ms": map[string]any{"type": "integer", "minimum": 1, "maximum": 3600000}, "grace_ms": map[string]any{"type": "integer", "minimum": 1, "maximum": 5000}, "max_output_bytes": map[string]any{"type": "integer", "minimum": 1, "maximum": MaxArtifactBytes}}, "additionalProperties": false}},
	}, "additionalProperties": false}
	// A new exact schema ref selects new configuration semantics. Keep the old
	// schema bytes (and their digest) unchanged for saved F1 definitions.
	configurationBytes, err := json.Marshal(configurationSchema)
	if err != nil {
		return nil, nil, err
	}
	var coreConfigurationSchema map[string]any
	if err := json.Unmarshal(configurationBytes, &coreConfigurationSchema); err != nil {
		return nil, nil, err
	}
	coreProperties := coreConfigurationSchema["properties"].(map[string]any)
	coreProperties["schema_version"] = map[string]any{"const": CoreConfigVersion}
	coreProperties["semantics_profile"] = map[string]any{"enum": []string{flow.Profile, flow.CoreProfile}}
	coreProperties["input_values"] = map[string]any{"type": "object", "maxProperties": 256, "additionalProperties": map[string]any{"type": "object", "maxProperties": 256, "additionalProperties": true}}
	identifier := map[string]any{"type": "string", "pattern": "^[A-Za-z][A-Za-z0-9._:/-]{0,127}$"}
	artifactRefSchema := map[string]any{"type": "object", "required": []string{"artifact_id", "revision", "digest"}, "properties": map[string]any{"artifact_id": identifier, "revision": map[string]any{"const": 1}, "digest": map[string]any{"type": "string", "pattern": "^sha256:[0-9a-f]{64}$"}}, "additionalProperties": false}
	inputSlot := map[string]any{"type": "object", "required": []string{"ref", "path"}, "properties": map[string]any{"ref": artifactRefSchema, "path": map[string]any{"type": "string", "pattern": "^inputs/[a-z][a-z0-9_]{0,63}$"}}, "additionalProperties": false}
	outputSlot := map[string]any{"type": "object", "required": []string{"artifact_id", "revision", "path"}, "properties": map[string]any{"artifact_id": identifier, "revision": map[string]any{"const": 1}, "path": map[string]any{"type": "string", "pattern": "^outputs/[a-z][a-z0-9_]{0,63}$"}}, "additionalProperties": false}
	contextSchema := map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object", "required": []string{"schema_version", "inputs", "outputs", "dependencies"}, "properties": map[string]any{
		"schema_version": map[string]any{"const": "local-context/1"}, "inputs": map[string]any{"type": "object", "maxProperties": 256, "additionalProperties": inputSlot}, "outputs": map[string]any{"type": "object", "maxProperties": 256, "additionalProperties": outputSlot}, "dependencies": map[string]any{"type": "array", "maxItems": 512, "uniqueItems": true, "items": refSchema}}, "additionalProperties": false}
	projectionSchema := map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object", "required": []string{"schema_version", "source_ref", "pointer", "projected_schema_ref", "workflow_ref"}, "properties": map[string]any{
		"schema_version": map[string]any{"const": "json-projection/1"}, "source_ref": artifactRefSchema, "pointer": map[string]any{"type": "string", "maxLength": 2048, "pattern": "^(/([^~/]|~[01])*)*$"}, "projected_schema_ref": refSchema, "workflow_ref": refSchema}, "additionalProperties": false}
	items := []item{
		{"core:schema/local-configuration", "schema", configurationSchema},
		{"core:schema/core-configuration", "schema", coreConfigurationSchema},
		{"core:schema/json-projection", "schema", projectionSchema},
		{"core:schema/local-context", "schema", contextSchema},
		{"core:schema/step-result", "schema", json.RawMessage(resultSchema)},
		{flow.AggregateSchemaID, "schema", json.RawMessage(aggregateSchema)},
		{flow.WorkspaceTreeManifestSchemaID, "schema", workspaceTreeManifestSchema()},
		{"core:schema/artifact-manifest", "schema", json.RawMessage(artifactManifestSchema)},
		{publicationHandleSchemaID, "schema", json.RawMessage(publicationHandleSchema)},
		{publicationCursorSchemaID, "schema", json.RawMessage(publicationCursorSchema)},
		{publicationDeliverySchemaID, "schema", json.RawMessage(publicationDeliverySchema)},
		{"core:profile/redaction", "resource", map[string]any{"id": "core:profile/redaction", "version": "1.0.0", "capture": "metadata_only"}},
		{"core:resolver/local", "resource", map[string]any{"id": "core:resolver/local", "version": "1.0.0", "resolution": "explicit_local_files"}},
		// A control object has no adapter that owns it. Naming the control
		// plane as its own provider keeps a control ResourceIdentity honest
		// instead of borrowing an adapter that owns something else.
		{"core:resource/authority-control", "resource", map[string]any{"id": "core:resource/authority-control", "version": "1.0.0", "provides": "authority_control_objects"}},
		{"core:adapter/local-process", "adapter", map[string]any{"schema_version": "1", "id": "core:adapter/local-process", "version": "1.0.0", "execution_modes": []string{"managed"}, "operations": []string{"process"}, "context_isolation": "fresh", "process_control": "cooperative", "effect_fencing": "cooperative", "sandbox": "none", "durable_wakeup": false, "qualification_evidence": []any{}}},
		// The assisted host is not a process this authority starts, so it
		// declares no process control and no fresh-context guarantee: the
		// session already exists and the core cannot prove its isolation.
		{"core:adapter/assisted-session", "adapter", map[string]any{"schema_version": "1", "id": "core:adapter/assisted-session", "version": "1.0.0", "execution_modes": []string{"assisted"}, "operations": []string{"session"}, "context_isolation": "declared", "process_control": "host", "effect_fencing": "cooperative", "sandbox": "none", "durable_wakeup": false, "qualification_evidence": []any{}}},
	}
	defs := []PinnedDefinition{}
	reg := flow.Registry{}
	for _, i := range items {
		b, err := canonical(i.value)
		if err != nil {
			return nil, nil, err
		}
		d, err := flow.Digest(b)
		if err != nil {
			return nil, nil, err
		}
		ref := flow.Ref{ID: i.id, Version: "1.0.0", Digest: d}
		defs = append(defs, PinnedDefinition{ref, i.kind, rawDigest(b), b})
		reg[ref] = b
	}
	redaction := builtinRef(defs, "core:profile/redaction")
	policy := map[string]any{"schema_version": "1", "id": "core:policy/local", "version": "1.0.0", "allow_unattended": false, "allowed_execution_modes": []string{"managed"}, "allowed_effect_classes": []string{"none", "workspace_write"}, "approval_required_for": []any{}, "required_check_refs": []any{}, "waivable_check_refs": []any{}, "minimum_sandbox": "none", "allow_early_quorum": false, "limits": flow.Limits{MaxStepInstances: 256, MaxControlTransitions: 1024, MaxParallelism: 1, MaxChildDepth: 0}, "retention": map[string]any{"artifacts_days": 365, "audit_days": 36500, "redaction_profile_ref": redaction}}
	// Keep the old exact policy immutable. Calls require an explicitly selected
	// new version; inventory order never upgrades a saved policy reference.
	for _, version := range []struct {
		version     string
		depth       int
		parallelism int
	}{{"1.0.0", 0, 1}, {"2.0.0", 8, 1}, {"3.0.0", 8, flow.MaxQualifiedParallelism}} {
		policy["version"] = version.version
		policy["limits"] = flow.Limits{MaxStepInstances: 256, MaxControlTransitions: 1024, MaxParallelism: version.parallelism, MaxChildDepth: version.depth}
		b, err := canonical(policy)
		if err != nil {
			return nil, nil, err
		}
		d, err := flow.Digest(b)
		if err != nil {
			return nil, nil, err
		}
		ref := flow.Ref{ID: "core:policy/local", Version: version.version, Digest: d}
		defs = append(defs, PinnedDefinition{ref, "policy", rawDigest(b), b})
		reg[ref] = b
	}
	contextDefinitions, err := contextBuiltinDefinitions(coreConfigurationSchema, contextSchema)
	if err != nil {
		return nil, nil, err
	}
	sourceDefinitions, err := sourceBuiltinDefinitions()
	if err != nil {
		return nil, nil, err
	}
	for _, definition := range append(contextDefinitions, sourceDefinitions...) {
		defs = append(defs, definition)
		reg[definition.Ref] = definition.Bytes
	}
	return defs, reg, nil
}
func builtinRef(defs []PinnedDefinition, id string) flow.Ref {
	return builtinVersionRef(defs, id, "1.0.0")
}
func builtinVersionRef(defs []PinnedDefinition, id, version string) flow.Ref {
	for _, d := range defs {
		if d.Ref.ID == id && d.Ref.Version == version {
			return d.Ref
		}
	}
	return flow.Ref{}
}

func Init(root string) error { return InitProfile(root, flow.Profile) }

// InitProjectProfile creates the Core authority required by a tracked Project
// profile. Project packages pin sealed context resources, so this is deliberately
// distinct from the historical generic Core initialization contract.
func InitProjectProfile(root string) error { return initProfile(root, flow.CoreProfile, true) }

func InitProfile(root, profile string) error { return initProfile(root, profile, false) }

func initProfile(root, profile string, projectContext bool) error {
	if profile != flow.Profile && profile != flow.CoreProfile {
		return errors.New("unsupported_profile: unknown semantics profile")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(absolute, 0700); err != nil {
		return err
	}
	root, err = filepath.EvalSymlinks(absolute)
	if err != nil {
		return err
	}
	for _, path := range []string{"prifly.json", "definitions.json", ".prifly/installation.json", ".prifly/state/state.sqlite3"} {
		if _, err := os.Lstat(filepath.Join(root, path)); err == nil {
			return errors.New("initialization_conflict: existing configuration or partial installation was not overwritten")
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	defs, _, err := Builtins()
	if err != nil {
		return err
	}
	for _, p := range []string{"steps", "workflows", "schemas", "packages", ".prifly", ".prifly/state", ".prifly/artifacts", ".prifly/artifact-refs", ".prifly/work", ".prifly/inventory", PackageRoot} {
		path := filepath.Join(root, p)
		if st, err := os.Lstat(path); err == nil && (!st.IsDir() || st.Mode()&os.ModeSymlink != 0) {
			return local.ErrUnsafePath
		}
		if err := os.MkdirAll(path, 0700); err != nil {
			return err
		}
	}
	store, err := local.OpenStore(filepath.Join(root, ".prifly/state"), local.StoreOptions{EventTypes: EventTypes})
	if err != nil {
		return err
	}
	defer store.Close()
	config := ProjectConfig{"1", newID("project"), []flow.Ref{}, builtinRef(defs, "core:policy/local"), map[string]flow.Ref{"local_process": builtinRef(defs, "core:adapter/local-process")}, builtinRef(defs, "core:schema/local-configuration"), Configuration{SchemaVersion: "1", SemanticsProfile: profile, TrustProfile: "core-local/cooperative", StateRoot: ".prifly/state", ArtifactRoot: ".prifly/artifacts", WorkspaceRoot: ".prifly/work", RegistryFile: "definitions.json", Executors: map[string]ExecutorConfig{}}, map[string]string{}}
	if profile == flow.CoreProfile {
		config.DefaultPolicyRef = builtinVersionRef(defs, "core:policy/local", "2.0.0")
		config.ConfigurationSchemaRef = builtinRef(defs, "core:schema/core-configuration")
		config.Configuration.SchemaVersion = CoreConfigVersion
		if projectContext {
			config.DefaultPolicyRef = builtinVersionRef(defs, "core:policy/local", "2.0.0")
			config.ConfigurationSchemaRef = builtinVersionRef(defs, "core:schema/core-configuration", "2.0.0")
			config.AdapterBindings["local_process"] = builtinVersionRef(defs, "core:adapter/local-process", "2.0.0")
			config.Configuration.SchemaVersion = CoreContextConfigVersion
		}
	}
	var cursorKey [32]byte
	if _, err := rand.Read(cursorKey[:]); err != nil {
		return err
	}
	installation := Installation{"1", store.Info().AuthorityID, root, os.Geteuid(), time.Now().UTC().Format(time.RFC3339Nano), hex.EncodeToString(cursorKey[:])}
	// The public configuration is the last published file. An interrupted init
	// remains visibly incomplete and cannot authorize an execution.
	for _, file := range []struct {
		path  string
		value any
	}{{".prifly/installation.json", installation}, {"definitions.json", RegistryFile{SchemaVersion: "1", Entries: []Definition{}}}, {"prifly.json", config}} {
		b, err := json.MarshalIndent(file.value, "", "  ")
		if err != nil {
			return err
		}
		if err := writeExclusive(filepath.Join(root, file.path), append(b, '\n')); err != nil {
			return err
		}
	}
	return nil
}

// authorityNotFound separates a path that holds no authority from an object
// missing inside one. Reported as the same not_found, a mistyped --project
// sends the reader looking for a Run that was never the problem.
func authorityNotFound(path string) error {
	return &flow.Problem{Code: "authority_not_found", Message: "no Pri-Fly authority at " + path + "; select one with --project DIR or create it with prifly init"}
}

func Open(root string, readOnly bool) (*Engine, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	root, err = filepath.EvalSymlinks(absolute)
	if errors.Is(err, os.ErrNotExist) {
		return nil, authorityNotFound(absolute)
	}
	if err != nil {
		return nil, err
	}
	b, err := readLocal(root, "prifly.json", MaxDefinitionBytes)
	if errors.Is(err, os.ErrNotExist) {
		return nil, authorityNotFound(root)
	}
	if err != nil {
		return nil, fmt.Errorf("read project configuration (run prifly init first): %w", err)
	}
	if err := flow.ValidateProtocol("ProjectConfig", b); err != nil {
		return nil, err
	}
	var config ProjectConfig
	if err := decode(b, &config); err != nil {
		return nil, err
	}
	defs, reg, err := Builtins()
	if err != nil {
		return nil, err
	}
	contextConfiguration := config.ConfigurationSchemaRef == builtinVersionRef(defs, "core:schema/core-configuration", "2.0.0")
	if !contextConfiguration && config.ConfigurationSchemaRef != builtinRef(defs, "core:schema/local-configuration") && config.ConfigurationSchemaRef != builtinRef(defs, "core:schema/core-configuration") {
		return nil, errors.New("unsupported configuration schema")
	}
	supportedPolicy := config.DefaultPolicyRef == builtinRef(defs, "core:policy/local") || config.Configuration.SemanticsProfile == flow.CoreProfile && config.DefaultPolicyRef == builtinVersionRef(defs, "core:policy/local", "2.0.0")
	adapter := builtinRef(defs, "core:adapter/local-process")
	if contextConfiguration {
		adapter = builtinVersionRef(defs, "core:adapter/local-process", "2.0.0")
	}
	if !supportedPolicy || len(config.AdapterBindings) != 1 || config.AdapterBindings["local_process"] != adapter {
		return nil, errors.New("unsupported_configuration: expected an exact local policy supported by the selected profile and the local_process adapter binding")
	}
	var rawConfig struct {
		Configuration json.RawMessage `json:"configuration"`
	}
	if err := json.Unmarshal(b, &rawConfig); err != nil {
		return nil, err
	}
	if err := flow.ValidateSchema(reg, config.ConfigurationSchemaRef, rawConfig.Configuration); err != nil {
		return nil, err
	}
	if config.Configuration.SemanticsProfile == flow.Profile && len(config.Configuration.InputValues) != 0 {
		return nil, errors.New("unsupported_configuration: input values require core-workflow/1")
	}
	if len(config.InstalledPackages) != 0 || len(config.SecretRefs) != 0 {
		return nil, errors.New("packages and secret providers are unsupported in foundation-sequence/1")
	}
	paths := []string{config.Configuration.StateRoot, config.Configuration.ArtifactRoot, config.Configuration.WorkspaceRoot, config.Configuration.RegistryFile}
	for _, p := range paths {
		if !safeRelative(p) {
			return nil, local.ErrUnsafePath
		}
	}
	roots := []string{config.Configuration.StateRoot, config.Configuration.ArtifactRoot, config.Configuration.WorkspaceRoot, ".prifly/artifact-refs", ".prifly/inventory"}
	for i, p := range roots {
		for _, q := range roots[:i] {
			if overlaps(p, q) {
				return nil, errors.New("state, artifacts, inventory and workspaces must be disjoint roots")
			}
		}
		if overlaps(p, PackageRoot) {
			return nil, errors.New("sealed package payloads must be disjoint from the configured data roots")
		}
		if overlaps(p, config.Configuration.RegistryFile) {
			return nil, errors.New("registry must be outside protected data roots")
		}
		if err := checkDirectory(root, p); err != nil {
			return nil, err
		}
	}
	b, err = readLocal(root, ".prifly/installation.json", MaxDefinitionBytes)
	if err != nil {
		return nil, err
	}
	var installation Installation
	if err := decode(b, &installation); err != nil {
		return nil, err
	}
	if installation.SchemaVersion != "1" || installation.OwnerUID != os.Geteuid() {
		return nil, errors.New("unsupported installation or wrong OS owner")
	}
	if key, err := hex.DecodeString(installation.TelemetryCursorKey); err != nil || len(key) != 32 {
		return nil, errors.New("incompatible installation cursor key")
	}
	if installation.ProjectRoot != root && !readOnly {
		return nil, local.ErrRecoveryRequired
	}
	store, err := local.OpenStore(filepath.Join(root, config.Configuration.StateRoot), local.StoreOptions{EventTypes: EventTypes, ReadOnly: readOnly})
	if err != nil {
		return nil, err
	}
	if store.Info().AuthorityID != installation.ID {
		store.Close()
		return nil, errors.New("installation and database authority identities differ")
	}
	var blobs *local.BlobStore
	if readOnly {
		blobs, err = local.OpenBlobStoreReadOnly(filepath.Join(root, config.Configuration.ArtifactRoot))
	} else {
		blobs, err = local.OpenBlobStore(filepath.Join(root, config.Configuration.ArtifactRoot))
	}
	if err != nil {
		store.Close()
		return nil, err
	}
	engine := &Engine{Root: root, Config: config, Installation: installation, Store: store, Blobs: blobs, ReadOnly: readOnly, clock: newClock(), owner: fmt.Sprintf("local:uid:%d", os.Geteuid())}
	if err := engine.loadPackages(); err != nil {
		_ = engine.Close()
		return nil, err
	}
	return engine, nil
}
func (e *Engine) Close() error { a := e.Blobs.Close(); b := e.Store.Close(); return errors.Join(a, b) }

func (e *Engine) Inventory() ([]PinnedDefinition, flow.Registry, error) {
	defs, reg, _, err := e.inventoryResources()
	return defs, reg, err
}

// Local aliases are authoring inputs, never mutable references in a Run.
// The resolver seals only reachable definitions before CompileProfile or lock.
func (e *Engine) workflowAliases() (map[string][]byte, error) {
	file, err := e.localRegistry()
	if err != nil {
		return nil, err
	}
	if len(file.Aliases) > 0 && e.Config.Configuration.SemanticsProfile != flow.CoreProfile {
		return nil, errors.New("unsupported_alias: local workflow aliases require core-workflow/1")
	}
	aliases := map[string][]byte{}
	var total int
	for name, path := range file.Aliases {
		if name == "" || len(name) > 128 || strings.ContainsAny(name, "/\\\x00\r\n\t ") {
			return nil, errors.New("invalid local workflow alias")
		}
		raw, err := readLocal(e.Root, path, MaxDefinitionBytes)
		if err != nil {
			return nil, err
		}
		total += len(raw)
		if total > 16<<20 {
			return nil, errors.New("local workflow aliases exceed source budget")
		}
		format := "json"
		if strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") {
			format = "yaml"
		}
		if err := flow.ValidateWorkflowConditions(raw, format); err != nil {
			return nil, err
		}
		aliases[name], err = flow.WorkflowJSONBytes(raw, format)
		if err != nil {
			return nil, err
		}
	}
	return aliases, nil
}

func (e *Engine) Check(ctx context.Context) (map[string]any, error) {
	if err := e.Store.Verify(ctx); err != nil {
		return nil, err
	}
	defs, _, resources, err := e.inventoryResources()
	if err != nil {
		return nil, err
	}
	localCount := len(resources)
	for _, d := range defs {
		if !strings.HasPrefix(d.Ref.ID, "core:") {
			localCount++
		}
	}
	manifest := Capabilities()
	profile := manifest.Profiles[0]
	version := "foundation-doctor/1"
	if e.Config.Configuration.SemanticsProfile == flow.CoreProfile {
		profile = manifest.Profiles[1]
		version = "core-doctor/1"
	}
	return map[string]any{"schema_version": version, "version": Version, "profile": profile.Profile, "trust_profile": "core-local/cooperative", "isolation": false, "sqlite": e.Store.Info(), "local_definitions": localCount, "network_probe": false, "local_worker_socket": localWorkerSocketAvailable(), "supported": profile.Capabilities, "unsupported": manifest.Unsupported}, nil
}
