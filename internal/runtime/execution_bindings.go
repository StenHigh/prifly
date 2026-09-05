package runtime

import (
	"context"
	_ "embed"
	"encoding/json"
	"strings"

	"github.com/stenhigh/prifly/internal/flow"
)

const ExecutionBindingsVersion = "execution-bindings/1"
const MaxExecutionBindingsBytes = 2 * MaxArtifactBytes

//go:embed execution-bindings.schema.json
var executionBindingPublicContracts []byte

// ExecutionBindings is a per-Run local-owner request, not package permission
// or ordinary workflow input configuration. Files carry already confined
// source bytes; no source path in this request is read from the authority.
type ExecutionBindings struct {
	SchemaVersion string             `json:"schema_version"`
	Bindings      []ExecutionBinding `json:"bindings"`
}

type ExecutionBinding struct {
	DefinitionRef flow.Ref          `json:"definition_ref"`
	Config        ExecutorConfig    `json:"config"`
	Files         map[string][]byte `json:"files"`
}

// ValidateExecutionBindingsPayload checks the closed public payload before a
// JSON caller decodes it. Closure membership is checked with the selected plan.
func ValidateExecutionBindingsPayload(data []byte) error {
	if len(data) > MaxExecutionBindingsBytes {
		return fault("execution_bindings_invalid", "execution bindings exceed the byte limit")
	}
	schema, err := flow.Canonical(executionBindingPublicContracts)
	if err != nil {
		return err
	}
	ref := flow.Ref{ID: "core:schema/execution-bindings", Version: "1.0.0", Digest: rawDigest(schema)}
	if err := flow.ValidateSchema(flow.Registry{ref: schema}, ref, data); err != nil {
		return faultf("execution_bindings_invalid", "%v", err)
	}
	var payload ExecutionBindings
	if err := decode(data, &payload); err != nil {
		return faultf("execution_bindings_invalid", "%v", err)
	}
	seen := map[flow.Ref]bool{}
	total := 0
	for _, binding := range payload.Bindings {
		if seen[binding.DefinitionRef] {
			return fault("execution_bindings_invalid", "duplicate definition reference")
		}
		seen[binding.DefinitionRef] = true
		if err := validateExecutorConfig(binding.Config, true); err != nil {
			return faultf("execution_bindings_invalid", "%v", err)
		}
		if strings.ContainsRune(binding.Config.Executable, 0) {
			return fault("execution_bindings_invalid", "executable contains a NUL byte")
		}
		for _, argument := range binding.Config.Args {
			if strings.ContainsRune(argument, 0) {
				return fault("execution_bindings_invalid", "argument contains a NUL byte")
			}
		}
		sources := map[string]bool{}
		for _, source := range binding.Config.Files {
			sources[source] = true
		}
		if len(sources) != len(binding.Files) {
			return fault("execution_bindings_invalid", "supporting bytes must match exactly the configured sources")
		}
		for source, data := range binding.Files {
			if !sources[source] || !safeRelative(source) || data == nil {
				return fault("execution_bindings_invalid", "undeclared or unsafe supporting source")
			}
			total += len(data)
			if total > MaxArtifactBytes {
				return fault("execution_bindings_invalid", "combined supporting bytes exceed the artifact limit")
			}
		}
	}
	return nil
}

func (e *Engine) resolveExecutionBindings(plan *flow.Plan, definitions []PinnedDefinition, payload *ExecutionBindings) (map[flow.Ref]ExecutionBinding, error) {
	result := map[flow.Ref]ExecutionBinding{}
	executors := executorDefinitions(plan)
	if payload == nil {
		for ref := range executors {
			if config, exists := e.Config.Configuration.Executors[ref.ID]; exists {
				result[ref] = ExecutionBinding{DefinitionRef: ref, Config: config}
			}
		}
		return result, nil
	}
	if payload != nil {
		data, err := canonical(payload)
		if err != nil {
			return nil, err
		}
		if err := ValidateExecutionBindingsPayload(data); err != nil {
			return nil, err
		}
		// Own the validated data for both context selection and pinning. Caller
		// maps must not become a second mutable configuration source.
		var selected ExecutionBindings
		if err := json.Unmarshal(data, &selected); err != nil {
			return nil, err
		}
		for _, binding := range selected.Bindings {
			executor, exists := executors[binding.DefinitionRef]
			if !exists {
				return nil, faultf("execution_binding_outside_closure", "%s", binding.DefinitionRef.String())
			}
			if isAssistedExecutor(definitions, executor) {
				return nil, fault("execution_binding_unsupported", "an assisted session does not bind a local executable")
			}
			result[binding.DefinitionRef] = binding
		}
	}
	for ref, executor := range executors {
		if isAssistedExecutor(definitions, executor) {
			continue
		}
		if _, exists := result[ref]; !exists {
			return nil, faultf("missing_executor", "%s", ref.String())
		}
	}
	return result, nil
}

func (e *Engine) executionConfiguration(plan *flow.Plan, definitions []PinnedDefinition, registry flow.Registry, payload *ExecutionBindings) (map[flow.Ref]ExecutionBinding, error) {
	bindings, err := e.resolveExecutionBindings(plan, definitions, payload)
	if err != nil {
		return nil, err
	}
	if err := e.selectContextProfilesWithBindings(plan, definitions, registry, bindings); err != nil {
		return nil, err
	}
	if err := e.checkCapabilitiesWithBindings(plan, bindings); err != nil {
		return nil, err
	}
	return bindings, nil
}

// ValidateExecutionBindings is read-only with respect to the authority. The
// supplied compiled plan gains the same explicitly selected context refs that
// Start locks, allowing Project to validate before package registration.
func (e *Engine) ValidateExecutionBindings(plan *flow.Plan, definitions []PinnedDefinition, registry flow.Registry, bindings *ExecutionBindings) error {
	if plan == nil {
		return fault("execution_bindings_invalid", "a compiled workflow is required")
	}
	_, err := e.executionConfiguration(plan, definitions, registry, bindings)
	return err
}

// CheckPinnedExecutables compares a caller's reviewed exact executors without
// exposing the private configuration stripped from ordinary Run views. It is
// read-only and grants no right to dispatch or bypass normal admission.
func (e *Engine) CheckPinnedExecutables(ctx context.Context, runID string, expected map[string]string) error {
	run, _, err := e.load(ctx, runID)
	if err != nil {
		return err
	}
	if len(run.Executors) != len(expected) {
		return fault("execution_review_mismatch", "pinned executable set differs from the reviewed set")
	}
	for ref, digest := range expected {
		if executor, exists := run.Executors[ref]; !exists || executor.ExecutableDigest != digest {
			return fault("execution_review_mismatch", "pinned executable differs from the reviewed bytes")
		}
	}
	return nil
}

func executorBindingVersion(version string, bindings *ExecutionBindings) error {
	if bindings != nil && version != "2" {
		return fault("unsupported_execution_bindings", "explicit execution bindings require Start version 2")
	}
	return nil
}
