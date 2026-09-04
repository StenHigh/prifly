package runtime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

type StartOptions struct {
	// Empty and "1" retain the mandatory RunBrief contract. Version 2 admits
	// declared workflow inputs without manufacturing a separate task document.
	SchemaVersion string
	CommandID     string
	WorkflowFile  string
	BriefFile     string
	Inputs        map[string]string
	InputRefs     map[string]ArtifactRef
	// Brief and InputValues carry bytes a caller already holds instead of a
	// path to read them from. A schedule is the reason they exist: its brief
	// and inputs were pinned by digest when it was created, so re-reading a
	// file at fire time would run something other than what was approved.
	Brief             json.RawMessage
	InputValues       map[string]json.RawMessage
	ExecutionBindings *ExecutionBindings
	DecisionCatalog   *DecisionCatalog
	DecisionSheet     *DecisionSheet
	// Guards are the live start/stop rules this Run is registered with. They
	// are declared here rather than installed later because a registration has
	// to exist before the first admission it protects; one installed afterwards
	// would already have let something through.
	Guards []GuardDeclaration
	// WorkspaceMode is set only by project start. An empty value retains the
	// historical raw run-start contract.
	WorkspaceMode string
}
type PreviewOptions struct {
	SchemaVersion     string
	WorkflowFile      string
	BriefFile         string
	InputRefs         map[string]ArtifactRef
	ExecutionBindings *ExecutionBindings
}
type Preview struct {
	SchemaVersion          string                          `json:"schema_version"`
	WorkflowRef            flow.Ref                        `json:"workflow_ref"`
	Profile                string                          `json:"semantics_profile"`
	TrustProfile           string                          `json:"trust_profile"`
	Sequence               []string                        `json:"sequence"`
	Hooks                  map[string]map[string]flow.Hook `json:"hooks"`
	Limits                 flow.Limits                     `json:"limits"`
	Admission              bool                            `json:"admission"`
	Warnings               []string                        `json:"warnings"`
	Brief                  *Brief                          `json:"brief,omitempty"`
	Inputs                 map[string]ArtifactRef          `json:"inputs,omitempty"`
	Executors              map[string]ExecutorPreview      `json:"executors"`
	CheckExecutors         map[string]ExecutorPreview      `json:"check_executors,omitempty"`
	Validation             ValidationSummary               `json:"validation"`
	EffectiveConfiguration *EffectiveConfiguration         `json:"effective_configuration,omitempty"`
	Workflows              map[string]WorkflowPreview      `json:"workflows,omitempty"`
}
type WorkflowPreview struct {
	WorkflowRef flow.Ref                        `json:"workflow_ref"`
	Sequence    []string                        `json:"sequence"`
	Hooks       map[string]map[string]flow.Hook `json:"hooks"`
	Executors   map[string]ExecutorPreview      `json:"executors"`
}
type ExecutorPreview struct {
	Executable          string   `json:"executable"`
	ArgumentCount       int      `json:"argument_count"`
	ConfigurationDigest string   `json:"configuration_digest"`
	SourceFiles         []string `json:"source_files"`
	EffectClass         string   `json:"effect_class"`
}
type ValidationSummary struct {
	ShapeValid         bool   `json:"shape_valid"`
	ReferencesResolved bool   `json:"references_resolved"`
	GraphValid         bool   `json:"graph_valid"`
	ProfileSupported   bool   `json:"profile_supported"`
	Inputs             string `json:"inputs"`
	AuthorizedNow      string `json:"authorized_now"`
	ExecutableNow      string `json:"executable_now"`
}

func (e *Engine) compileFile(path string) (*flow.Plan, []PinnedDefinition, []PinnedResource, []byte, error) {
	plan, defs, resources, raw, _, err := e.compileFileWithBindings(path, nil)
	return plan, defs, resources, raw, err
}

func (e *Engine) compileFileWithBindings(path string, payload *ExecutionBindings) (*flow.Plan, []PinnedDefinition, []PinnedResource, []byte, map[flow.Ref]ExecutionBinding, error) {
	defs, reg, resources, err := e.inventoryResources()
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	if filepath.IsAbs(path) {
		relative, err := filepath.Rel(e.Root, path)
		if err != nil {
			return nil, nil, nil, nil, nil, err
		}
		path = relative
	}
	raw, err := readLocal(e.Root, path, MaxDefinitionBytes)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	format := "json"
	if strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") {
		format = "yaml"
	}
	aliases, err := e.workflowAliases()
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	if len(aliases) != 0 {
		resolved, registry, err := flow.ResolveWorkflowAliases(raw, format, reg, aliases)
		if err != nil {
			return nil, nil, nil, nil, nil, err
		}
		for ref, data := range registry {
			if _, present := reg[ref]; !present {
				defs = append(defs, PinnedDefinition{ref, "workflow", rawDigest(data), data})
			}
		}
		reg, raw = registry, resolved
		sort.Slice(defs, func(i, j int) bool { return defs[i].Ref.String() < defs[j].Ref.String() })
	}
	var plan *flow.Plan
	if e.Config.Configuration.SchemaVersion == CoreContextConfigVersion {
		contextResources, resourceErr := resourcesFromPins(resources)
		if resourceErr != nil {
			return nil, nil, nil, nil, nil, resourceErr
		}
		plan, err = flow.CompileCore(raw, format, reg, contextResources)
	} else {
		plan, err = flow.CompileProfile(raw, format, reg, e.Config.Configuration.SemanticsProfile)
	}
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	bindings, err := e.executionConfiguration(plan, defs, reg, payload)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	// The root is not its own dependency. Check its registered identity before
	// Start prunes the inventory to the actual dependency closure.
	rootRef := flow.Ref{ID: plan.Workflow.ID, Version: plan.Workflow.Version, Digest: plan.Digest}
	for _, d := range defs {
		if d.Ref.ID == rootRef.ID && d.Ref.Version == rootRef.Version && (d.Ref != rootRef || d.RawDigest != rawDigest(raw)) {
			return nil, nil, nil, nil, nil, fault("workflow_registry_conflict", "selected root bytes differ from the registered version")
		}
	}
	selectedResources := resources[:0]
	for _, resource := range resources {
		if _, needed := plan.Resources[resource.Ref]; needed {
			selectedResources = append(selectedResources, resource)
		}
	}
	return plan, defs, selectedResources, raw, bindings, nil
}

func (e *Engine) checkCapabilities(plan *flow.Plan) error {
	return e.checkCapabilitiesWithBindings(plan, nil)
}

func (e *Engine) checkCapabilitiesWithBindings(plan *flow.Plan, bindings map[flow.Ref]ExecutionBinding) error {
	for _, p := range workflowPlans(plan) {
		if err := e.checkWorkflowCapabilitiesWithBindings(p, bindings); err != nil {
			return fmt.Errorf("%w (workflow %s)", err, p.Workflow.ID)
		}
	}
	return nil
}

func (e *Engine) checkWorkflowCapabilities(plan *flow.Plan) error {
	return e.checkWorkflowCapabilitiesWithBindings(plan, nil)
}

func (e *Engine) checkWorkflowCapabilitiesWithBindings(plan *flow.Plan, bindings map[flow.Ref]ExecutionBinding) error {
	defs, registry, err := Builtins()
	if err != nil {
		return err
	}
	supportedPolicy := plan.Workflow.PolicyRef == builtinRef(defs, "core:policy/local") ||
		plan.Profile == flow.CoreProfile && (plan.Workflow.PolicyRef == builtinVersionRef(defs, "core:policy/local", "2.0.0") ||
			plan.Workflow.PolicyRef == builtinVersionRef(defs, "core:policy/local", "3.0.0"))
	if !supportedPolicy {
		return fault("unsupported_policy", "expected an exact local policy supported by the selected profile")
	}
	var policy struct {
		Limits flow.Limits `json:"limits"`
	}
	if err := json.Unmarshal(registry[plan.Workflow.PolicyRef], &policy); err != nil {
		return err
	}
	limits := plan.Workflow.Limits
	if limits.MaxStepInstances > policy.Limits.MaxStepInstances || limits.MaxControlTransitions > policy.Limits.MaxControlTransitions || limits.MaxParallelism > policy.Limits.MaxParallelism || limits.MaxChildDepth > policy.Limits.MaxChildDepth {
		return faultf("resource_limit", "workflow limits exceed pinned policy %s", plan.Workflow.PolicyRef.String())
	}
	for _, stage := range plan.Workflow.Definition.Stages {
		if stage.Kind == "repeat" && stage.MaxIterations > 100 {
			return fault("resource_limit", "the qualified local profile supports at most 100 repeat iterations")
		}
	}
	for id, step := range plan.Steps {
		if len(step.WorkspaceTrees) != 0 {
			manifest := builtinRef(defs, flow.WorkspaceTreeManifestSchemaID)
			if !isAssistedExecutor(defs, flow.Executor{AdapterRef: step.Executor.AdapterRef, Operation: step.Executor.Operation}) {
				return fault("unsupported_workspace_tree_executor", "workspace trees require an assisted session")
			}
			for _, binding := range step.WorkspaceTrees {
				output := step.Outputs[binding.OutputPort]
				if output.SchemaRef == nil || *output.SchemaRef != manifest {
					return fault("workspace_tree_manifest_contract_mismatch", "")
				}
				if binding.InputPort != "" {
					input := step.Inputs[binding.InputPort]
					if input.SchemaRef == nil || *input.SchemaRef != manifest {
						return fault("workspace_tree_manifest_contract_mismatch", "")
					}
				}
			}
		}
		if isAssistedExecutor(defs, flow.Executor{AdapterRef: step.Executor.AdapterRef, Operation: step.Executor.Operation}) {
			if err := validateAssistedStep(plan, step); err != nil {
				return err
			}
			continue
		}
		fullContext := requiresContextState(plan) && step.Executor.AdapterRef == builtinVersionRef(defs, "core:adapter/local-process", "2.0.0")
		if !fullContext && step.Executor.AdapterRef != builtinRef(defs, "core:adapter/local-process") || step.Executor.Operation != "process" {
			return fault("unsupported_executor", "expected pinned core local process adapter")
		}
		if step.Effects.Class != "none" && step.Effects.Class != "workspace_write" {
			return fault("unsupported_effect", "F1 does not qualify external_write or destructive; an assisted session step is narrowed further to workspace_write or none")
		}
		for _, output := range step.Outputs {
			if output.Format == "blob" && len(output.MediaTypes) > 1 {
				return fault("unsupported_output_media", "a fixed local output slot requires one declared media type")
			}
		}
		if len(step.RequiredCapabilities) > 0 {
			return fault("unsupported_capability", "F1 supplies the fixed local process contract only")
		}
		if !fullContext && (step.InstructionsRef != nil || len(step.ContextRefs) > 0) {
			return fault("unsupported_context", "use explicit sealed inputs and selected executor files in F1")
		}
		config, ok := e.Config.Configuration.Executors[step.ID]
		if bindings != nil {
			binding, exists := bindings[plan.Workflow.Definition.Stages[id].StepRef]
			config, ok = binding.Config, exists
		}
		if !ok {
			return faultf("missing_executor", "%s", step.ID)
		}
		if !fullContext && config.ContextProfileRef != nil {
			return fault("unsupported_context_adapter", "context profiles require local-process@2.0.0")
		}
		if err := validateExecutorConfig(config, fullContext); err != nil {
			return err
		}
	}
	for ref, check := range plan.Checks {
		if !requiresContextState(plan) || check.Executor.AdapterRef != builtinVersionRef(defs, "core:adapter/local-process", "2.0.0") || check.Executor.Operation != "check" {
			return fault("unsupported_check_executor", "automatic checks require the exact local check operation")
		}
		config, exists := e.Config.Configuration.Executors[check.ID]
		if bindings != nil {
			binding, found := bindings[ref]
			config, exists = binding.Config, found
		}
		if !exists {
			return faultf("missing_executor", "%s", check.ID)
		}
		if err := validateExecutorConfig(config, true); err != nil {
			return err
		}
	}
	return nil
}

func validateExecutorConfig(config ExecutorConfig, fullContext bool) error {
	if !filepath.IsAbs(config.Executable) {
		return errors.New("executor executable must be an explicit absolute path")
	}
	if config.TimeoutMS < 1 || config.TimeoutMS > 3600000 || config.GraceMS < 1 || config.GraceMS > 5000 || config.MaxOutputBytes < 1 || config.MaxOutputBytes > MaxArtifactBytes {
		return errors.New("executor limits are out of range")
	}
	for target, source := range config.Files {
		if !safeRelative(target) || !safeRelative(source) || strings.HasPrefix(target, "inputs/") || strings.HasPrefix(target, "outputs/") || target == "context.json" || target == "inputs" || target == "outputs" || target == "tmp" {
			return errors.New("unsafe executor file binding")
		}
		if fullContext && (target == "context" || strings.HasPrefix(target, "context/")) {
			return errors.New("unsafe executor file binding: context directory is reserved")
		}
	}
	for name, value := range config.Environment {
		if name == "" || strings.ContainsAny(name, "=\x00") || strings.HasPrefix(name, "PRIFLY_") || strings.ContainsRune(value, 0) {
			return errors.New("invalid or reserved environment binding")
		}
	}
	return nil
}

func (e *Engine) Preview(options PreviewOptions) (Preview, error) {
	if err := executorBindingVersion(options.SchemaVersion, options.ExecutionBindings); err != nil {
		return Preview{}, err
	}
	if options.SchemaVersion != "" && options.SchemaVersion != "1" && options.SchemaVersion != "2" {
		return Preview{}, fault("unsupported_start_version", "unsupported Start contract version")
	}
	p, _, _, _, bindings, err := e.compileFileWithBindings(options.WorkflowFile, options.ExecutionBindings)
	if err != nil {
		return Preview{}, err
	}
	if options.SchemaVersion == "2" && p.Profile != flow.CoreProfile {
		return Preview{}, fault("unsupported_start_version", "Start version 2 requires core-workflow/1")
	}
	effective, err := e.effectiveConfiguration(p, nil, options.InputRefs, false)
	if err != nil {
		return Preview{}, err
	}
	hooks := map[string]map[string]flow.Hook{}
	executors := map[string]ExecutorPreview{}
	for id, step := range p.Steps {
		hooks[id] = step.Hooks
		cfg := bindings[p.Workflow.Definition.Stages[id].StepRef].Config
		data, err := canonical(cfg)
		if err != nil {
			return Preview{}, err
		}
		sources := []string{}
		for _, source := range cfg.Files {
			sources = append(sources, source)
		}
		sort.Strings(sources)
		executors[id] = ExecutorPreview{cfg.Executable, len(cfg.Args), rawDigest(data), sources, step.Effects.Class}
	}
	var brief *Brief
	if options.BriefFile != "" {
		data, err := e.inputBytes(options.BriefFile)
		if err != nil {
			return Preview{}, err
		}
		if err := flow.ValidateProtocol("RunBrief", data); err != nil {
			return Preview{}, err
		}
		brief = &Brief{}
		if err := decode(data, brief); err != nil {
			return Preview{}, err
		}
	}
	inputStatus := "not_checked"
	inputs := map[string]ArtifactRef{}
	if options.InputRefs != nil {
		for name := range options.InputRefs {
			if _, ok := p.Workflow.Inputs[name]; !ok {
				return Preview{}, errors.New("unknown preview input port")
			}
		}
		for name, port := range p.Workflow.Inputs {
			ref, ok := options.InputRefs[name]
			if !ok {
				configured := effective != nil && len(effective.Inputs[name].Value) > 0
				if port.Required && !configured {
					return Preview{}, errors.New("missing required preview input")
				}
				continue
			}
			a, data, err := e.Artifact(ref)
			if err != nil {
				return Preview{}, err
			}
			if err := e.validatePortArtifact(p, port.Port, a, data); err != nil {
				return Preview{}, err
			}
			inputs[name] = ref
		}
		inputStatus = "sealed_refs_verified"
		if effective != nil && len(effective.Inputs) > 0 {
			inputStatus = "provided_refs_and_configuration_verified"
		}
	}
	version := "foundation-preview/1"
	warnings := []string{"trusted local executable; no sandbox or per-tool effect enforcement", "preview grants no admission; pass the same input references at start to keep reviewed bytes", "arguments and environment are omitted; review prifly.json against configuration_digest"}
	if options.ExecutionBindings != nil {
		warnings[2] = "arguments and environment are omitted; review the explicit execution bindings against configuration_digest"
	}
	if p.Profile == flow.CoreProfile {
		version = "core-preview/1"
		warnings = append(warnings, "sequence is a topological inventory, not a prediction of the executed path", "configuration values are ordinary inputs; do not use them for credentials")
	}
	var checkExecutors map[string]ExecutorPreview
	if len(p.Checks) != 0 {
		checkExecutors = map[string]ExecutorPreview{}
		for ref := range p.Checks {
			config := bindings[ref].Config
			data, err := canonical(config)
			if err != nil {
				return Preview{}, err
			}
			sources := []string{}
			for _, source := range config.Files {
				sources = append(sources, source)
			}
			sort.Strings(sources)
			checkExecutors[ref.String()] = ExecutorPreview{config.Executable, len(config.Args), rawDigest(data), sources, "workspace_write"}
		}
		warnings = append(warnings, "required content/result checks have not run; preview cannot satisfy acceptance", "check executables are trusted local code with cooperative workspace effects, not sandboxed independent reviewers")
	}
	var workflows map[string]WorkflowPreview
	if requiresInvocationState(p) || options.SchemaVersion == "2" {
		version = CoreInvocationPreviewVersion
		if requiresRepeatState(p) {
			version = CoreRepeatPreviewVersion
		}
		if builtins, _, err := Builtins(); err == nil && (requiresContextState(p) || requiresSessionState(builtins, p)) {
			version = CoreWaiverPreviewVersion
		}
		if requiresParallelState(p) {
			version = CoreParallelPreviewVersion
		}
		if requiresArtifactPublicationState(p) {
			version = CoreArtifactPublicationPreviewVersion
		}
		if requiresArtifactClosureState(p) {
			version = CoreArtifactClosurePreviewVersion
		}
		if requiresPublicationSubscriptionState(p) {
			version = CorePublicationSubscriptionPreviewVersion
		}
		if requiresPublicationChecksState(p) {
			version = CorePublicationChecksPreviewVersion
		}
		if requiresPublicationNewOnlyState(p) {
			version = CorePublicationNewOnlyPreviewVersion
		}
		if requiresPublicationFailureState(p) {
			version = CorePublicationFailurePreviewVersion
		}
		if builtins, _, err := Builtins(); err == nil && requiresSessionState(builtins, p) {
			version = CoreActionDeliveryPreviewVersion
		}
		if requiresWorkspaceTreeState(p) {
			version = CoreWorkspaceTreePreviewVersion
		}
		workflows = map[string]WorkflowPreview{}
		if _, err := e.workflowConfigurations(p, effective); err != nil {
			return Preview{}, err
		}
		for _, child := range workflowPlans(p) {
			item := WorkflowPreview{WorkflowRef: planRef(child), Sequence: child.Sequence, Hooks: map[string]map[string]flow.Hook{}, Executors: map[string]ExecutorPreview{}}
			for id, step := range child.Steps {
				item.Hooks[id] = step.Hooks
				cfg := bindings[child.Workflow.Definition.Stages[id].StepRef].Config
				data, err := canonical(cfg)
				if err != nil {
					return Preview{}, err
				}
				sources := []string{}
				for _, path := range cfg.Files {
					sources = append(sources, path)
				}
				sort.Strings(sources)
				item.Executors[id] = ExecutorPreview{cfg.Executable, len(cfg.Args), rawDigest(data), sources, step.Effects.Class}
			}
			workflows[child.Digest] = item
		}
	}
	if options.SchemaVersion == "2" {
		version = CoreNeutralPreviewVersion
	}
	return Preview{SchemaVersion: version, WorkflowRef: planRef(p), Profile: p.Profile, TrustProfile: "core-local/cooperative", Sequence: p.Sequence, Hooks: hooks, Limits: p.Workflow.Limits, Admission: false, Warnings: warnings, Brief: brief, Inputs: inputs, Executors: executors, CheckExecutors: checkExecutors, Validation: ValidationSummary{true, true, true, true, inputStatus, "not_admitted", "not_checked"}, EffectiveConfiguration: effective, Workflows: workflows}, nil
}

func (e *Engine) inputBytes(path string) ([]byte, error) {
	if !filepath.IsAbs(path) {
		return readLocal(e.Root, path, MaxArtifactBytes)
	}
	if rel, err := filepath.Rel(e.Root, path); err == nil && safeRelative(rel) {
		return readLocal(e.Root, rel, MaxArtifactBytes)
	}
	return readLocal(filepath.Dir(path), filepath.Base(path), MaxArtifactBytes)
}

func (e *Engine) pinDefinitions(defs []PinnedDefinition) error {
	for _, d := range defs {
		if strings.HasPrefix(d.Ref.ID, "core:") {
			continue
		}
		name := fmt.Sprintf("%x.json", sha256.Sum256([]byte(d.Ref.ID+"@"+d.Ref.Version)))
		path := filepath.Join(e.Root, ".prifly/inventory", name)
		record, _ := canonical(map[string]any{"ref": d.Ref, "raw_digest": d.RawDigest})
		if d.Kind == "check" {
			record, _ = canonical(map[string]any{"ref": d.Ref, "raw_digest": d.RawDigest, "kind": d.Kind})
		}
		previous, err := readLocal(e.Root, filepath.Join(".prifly/inventory", name), MaxDefinitionBytes)
		if err == nil {
			if !bytes.Equal(previous, record) {
				return faultf("definition_drift", "%s@%s bytes changed; assign a new version", d.Ref.ID, d.Ref.Version)
			}
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := writeExclusive(path, record); err != nil {
			if !errors.Is(err, os.ErrExist) {
				return err
			}
			previous, err = readLocal(e.Root, filepath.Join(".prifly/inventory", name), MaxDefinitionBytes)
			if err != nil || !bytes.Equal(previous, record) {
				return errors.New("definition registration conflict")
			}
		}
	}
	return nil
}

func (e *Engine) Start(ctx context.Context, options StartOptions) (local.ApplyResult, error) {
	if e.ReadOnly {
		return local.ApplyResult{}, local.ErrReadOnly
	}
	if err := executorBindingVersion(options.SchemaVersion, options.ExecutionBindings); err != nil {
		return local.ApplyResult{}, err
	}
	if options.SchemaVersion != "" && options.SchemaVersion != "1" && options.SchemaVersion != "2" {
		return local.ApplyResult{}, fault("unsupported_start_version", "unsupported Start contract version")
	}
	neutral := options.SchemaVersion == "2"
	if options.CommandID == "" || !neutral && options.BriefFile == "" && len(options.Brief) == 0 {
		return local.ApplyResult{}, errors.New("explicit command_id and RunBrief are required")
	}
	if options.WorkspaceMode != "" && options.WorkspaceMode != "worktree" && options.WorkspaceMode != "checkout" {
		return local.ApplyResult{}, errors.New("workspace mode must be worktree or checkout")
	}
	if options.DecisionCatalog != nil || options.DecisionSheet != nil {
		if options.DecisionCatalog == nil || options.DecisionSheet == nil || ValidateDecisionSheet(*options.DecisionCatalog, *options.DecisionSheet) != nil {
			return local.ApplyResult{}, errors.New("decision catalog and sheet must be a matching valid pair")
		}
	}
	plan, defs, resources, workflowRaw, bindings, err := e.compileFileWithBindings(options.WorkflowFile, options.ExecutionBindings)
	if err != nil {
		return local.ApplyResult{}, err
	}
	if neutral && plan.Profile != flow.CoreProfile {
		return local.ApplyResult{}, fault("unsupported_start_version", "Start version 2 requires core-workflow/1")
	}
	var configurationValues map[string]json.RawMessage
	if neutral {
		configurationValues = options.InputValues
	}
	effective, err := e.effectiveConfigurationWithValues(plan, options.Inputs, options.InputRefs, configurationValues, true)
	if err != nil {
		return local.ApplyResult{}, err
	}
	configurations, err := e.workflowConfigurations(plan, effective)
	if err != nil {
		return local.ApplyResult{}, err
	}
	// A guard is registered against an invocation, so a Run that declares one
	// holds scoped invocations even when its graph alone would not need them.
	// A single-plan closure has no children, so this is the same map that
	// function would have built, not a second way of building it.
	if (neutral || len(options.Guards) != 0 || requiresArtifactPublicationState(plan) || requiresContextState(plan)) && configurations == nil && plan.Profile == flow.CoreProfile {
		configurations = map[string]*EffectiveConfiguration{plan.Digest: effective}
	}
	// Pin only the actual compiler closure and core transport contracts. An
	// unrelated installed definition does not become part of this run's history.
	// Exact legacy refs preserve the historical command digest when new builtin
	// versions are installed. The compiler adds any explicitly selected version.
	transport := map[flow.Ref]bool{}
	for _, id := range []string{"core:schema/local-configuration", "core:schema/local-context", "core:schema/step-result", "core:profile/redaction", "core:resolver/local", "core:adapter/local-process", "core:policy/local"} {
		transport[builtinRef(defs, id)] = true
	}
	if requiresParallelState(plan) || requiresMapState(plan) {
		// A settled join reports its branches whether or not anything consumes
		// the summary, so the form that describes it belongs to this Run. A map
		// settles on the same join, so it needs the same form.
		transport[builtinRef(defs, flow.AggregateSchemaID)] = true
	}
	if requiresArtifactClosureState(plan) {
		transport[builtinRef(defs, "core:schema/artifact-manifest")] = true
	}
	if requiresPublicationSubscriptionState(plan) {
		for _, id := range []string{publicationHandleSchemaID, publicationCursorSchemaID, publicationDeliverySchemaID} {
			transport[builtinRef(defs, id)] = true
		}
	}
	if requiresContextState(plan) {
		for _, id := range []string{"core:schema/local-context", "core:resolver/local", "core:adapter/local-process"} {
			transport[builtinVersionRef(defs, id, "2.0.0")] = true
		}
		for _, id := range []string{"core:schema/context-profile", "core:schema/full-context", "core:schema/context-json", "core:schema/evidence", "core:context/local-json", "core:assembly/local-json"} {
			transport[builtinRef(defs, id)] = true
		}
	}
	projectionSchema := builtinRef(defs, "core:schema/json-projection")
	selected := defs[:0]
	for _, d := range defs {
		_, needed := plan.Registry[d.Ref]
		if needed || transport[d.Ref] || plan.Profile == flow.CoreProfile && (d.Ref == e.Config.ConfigurationSchemaRef || d.Ref == projectionSchema) {
			selected = append(selected, d)
		}
	}
	defs = selected
	briefBytes := []byte(options.Brief)
	if len(briefBytes) == 0 && options.BriefFile != "" {
		briefBytes, err = e.inputBytes(options.BriefFile)
		if err != nil {
			return local.ApplyResult{}, err
		}
	}
	if !neutral || options.BriefFile != "" || len(options.Brief) != 0 {
		if err := flow.ValidateProtocol("RunBrief", briefBytes); err != nil {
			return local.ApplyResult{}, err
		}
		var brief Brief
		if err := decode(briefBytes, &brief); err != nil {
			return local.ApplyResult{}, err
		}
		if brief.Confirmation != "explicit" {
			return local.ApplyResult{}, errors.New("brief requires explicit owner confirmation")
		}
	}
	workflowRef := flow.Ref{ID: plan.Workflow.ID, Version: plan.Workflow.Version, Digest: plan.Digest}
	foundWorkflow := false
	for _, d := range defs {
		if d.Ref.ID == workflowRef.ID && d.Ref.Version == workflowRef.Version {
			if d.Ref != workflowRef {
				return local.ApplyResult{}, errors.New("workflow registry mismatch")
			}
			foundWorkflow = true
		}
	}
	if !foundWorkflow {
		defs = append(defs, PinnedDefinition{workflowRef, "workflow", rawDigest(workflowRaw), plan.Canonical})
	}
	resolvedInputs, err := e.resolveStartInputs(plan, effective, options)
	if err != nil {
		return local.ApplyResult{}, err
	}
	inputs := map[string]ArtifactRef{}
	for name, input := range resolvedInputs {
		port, ref := plan.Workflow.Inputs[name], input.Ref
		if ref == (ArtifactRef{}) {
			identity := fmt.Sprintf("artifact:%x", sha256.Sum256([]byte(options.CommandID+"/input/"+name)))
			a, err := e.putArtifact(input.Data, port.Format, port.SchemaRef, identity, map[string]any{"kind": "authority", "authority_id": e.Installation.ID, "command_id": options.CommandID, "port": name}, nil, plan.Registry, portMedia(port.Port))
			if err != nil {
				return local.ApplyResult{}, err
			}
			if err := e.validatePortArtifact(plan, port.Port, a, input.Data); err != nil {
				return local.ApplyResult{}, err
			}
			ref = a.Ref()
		}
		inputs[name] = ref
	}
	executors := map[string]PinnedExecutor{}
	for ref, executor := range executorDefinitions(plan) {
		key := ref.ID
		if requiresInvocationState(plan) || neutral {
			key = ref.String()
		}
		if _, ok := executors[key]; ok {
			continue
		}
		// An assisted step binds no executable, so there is nothing to resolve,
		// hash or materialise for it. Its boundary is the claim, not a binary.
		if isAssistedExecutor(defs, executor) {
			continue
		}
		cfg := bindings[ref].Config
		resolved, err := filepath.EvalSymlinks(cfg.Executable)
		if err != nil {
			return local.ApplyResult{}, err
		}
		cfg.Executable = resolved
		digest, err := local.ProcessExecutableDigest(cfg.Executable)
		if err != nil {
			return local.ApplyResult{}, err
		}
		pinned := PinnedExecutor{Config: cfg, ExecutableDigest: digest, Files: map[string]local.BlobRef{}}
		if requiresContextState(plan) && executor.AdapterRef == builtinVersionRef(defs, "core:adapter/local-process", "2.0.0") {
			registry := flow.Registry{}
			for _, definition := range defs {
				registry[definition.Ref] = definition.Bytes
			}
			profile, ref, err := contextProfileFor(cfg, defs, registry)
			if err != nil {
				return local.ApplyResult{}, err
			}
			pinned.Config.ContextProfileRef, pinned.ContextProfile = &ref, &profile
		}
		for target, source := range cfg.Files {
			data := bindings[ref].Files[source]
			if options.ExecutionBindings == nil {
				data, err = readLocal(e.Root, source, MaxArtifactBytes)
				if err != nil {
					return local.ApplyResult{}, err
				}
			}
			ref, err := e.Blobs.Put(bytes.NewReader(data), MaxArtifactBytes)
			if err != nil {
				return local.ApplyResult{}, err
			}
			pinned.Files[target] = ref
		}
		executors[key] = pinned
	}
	var pinnedConfig any = executors
	if effective != nil {
		pinnedConfig = map[string]any{"schema_version": "core-run-configuration/1", "semantics_profile": plan.Profile, "configuration_schema_ref": e.Config.ConfigurationSchemaRef, "executors": executors, "effective_configuration": effective}
	}
	if configurations != nil {
		pinnedConfig = map[string]any{"schema_version": "core-run-configuration/2", "semantics_profile": plan.Profile, "configuration_schema_ref": e.Config.ConfigurationSchemaRef, "executors": executors, "effective_configuration": effective, "workflow_configurations": configurations}
	}
	if requiresContextState(plan) {
		pinnedConfig.(map[string]any)["schema_version"] = "core-run-configuration/3"
	}
	configBytes, err := canonical(pinnedConfig)
	if err != nil {
		return local.ApplyResult{}, err
	}
	configDigest, _ := flow.Digest(configBytes)
	configRef := flow.Ref{ID: "snapshot:executors/" + strings.TrimPrefix(configDigest, "sha256:"), Version: "1.0.0", Digest: configDigest}
	defs = append(defs, PinnedDefinition{configRef, "resource", rawDigest(configBytes), configBytes})
	runID := startRunID(e.owner, options.CommandID)
	if plan.Profile == flow.Profile {
		// Earlier F1 readers could pin ref-shaped schema annotations. Retain
		// those historical pins on a retry rather than silently changing its
		// PackageLock. Current definitions/inputs/executors are still checked;
		// a changed request cannot become an exact retry by reusing this ID.
		previous, _, readErr := e.load(ctx, runID)
		if readErr != nil && !errors.Is(readErr, local.ErrNotFound) {
			return local.ApplyResult{}, readErr
		}
		if readErr == nil && previous.Profile == flow.Profile {
			seen := map[flow.Ref]bool{}
			for _, d := range defs {
				seen[d.Ref] = true
			}
			for _, d := range previous.Definitions {
				if d.Ref != previous.LockRef && !seen[d.Ref] {
					defs = append(defs, d)
					seen[d.Ref] = true
				}
			}
		}
	}
	if err := e.pinDefinitions(defs); err != nil {
		return local.ApplyResult{}, err
	}
	if err := e.pinResources(resources); err != nil {
		return local.ApplyResult{}, err
	}
	if requiresContextState(plan) {
		snapshot, err := contextResourceSnapshot(resources)
		if err != nil {
			return local.ApplyResult{}, err
		}
		defs = append(defs, snapshot)
	}
	closure := make([]flow.Ref, 0, len(defs))
	for _, d := range defs {
		closure = append(closure, d.Ref)
	}
	for _, resource := range resources {
		closure = append(closure, resource.Ref)
	}
	sort.Slice(closure, func(i, j int) bool { return closure[i].String() < closure[j].String() })
	resolver := builtinRef(defs, "core:resolver/local")
	if requiresContextState(plan) {
		resolver = builtinVersionRef(defs, "core:resolver/local", "2.0.0")
	}
	lock := map[string]any{"schema_version": "1", "id": derivedID("lock", e.owner, options.CommandID), "version": "1.0.0", "core_protocol": "1", "closure": closure, "resolver_ref": resolver}
	lockBytes, err := canonical(lock)
	if err != nil {
		return local.ApplyResult{}, err
	}
	if err := flow.ValidateProtocol("PackageLock", lockBytes); err != nil {
		return local.ApplyResult{}, err
	}
	lockDigest, _ := flow.Digest(lockBytes)
	lockRef := flow.Ref{ID: lock["id"].(string), Version: "1.0.0", Digest: lockDigest}
	defs = append(defs, PinnedDefinition{lockRef, "resource", rawDigest(lockBytes), lockBytes})
	var briefRef ArtifactRef
	if len(briefBytes) != 0 {
		briefArtifact, err := e.putArtifact(briefBytes, "blob", nil, derivedID("artifact", e.owner, options.CommandID, "brief"), map[string]any{"kind": "authority", "authority_id": e.Installation.ID, "command_id": options.CommandID, "port": "brief"}, nil, plan.Registry)
		if err != nil {
			return local.ApplyResult{}, err
		}
		briefRef = briefArtifact.Ref()
	}
	startCommand := map[string]any{"schema_version": "1", "command_id": options.CommandID, "project_id": e.Config.ID, "workflow_ref": workflowRef, "package_lock_ref": lockRef, "inputs": inputs, "interaction_mode": "with_human", "execution_mode": "managed", "capacity_profile": "foundation:one-slot", "grant_refs": []any{}}
	contract := "RunStart"
	if neutral {
		startCommand["schema_version"], contract = "2", "RunStartV2"
	}
	if briefRef != (ArtifactRef{}) {
		startCommand["brief_ref"] = briefRef
	}
	cb, _ := canonical(startCommand)
	if err := flow.ValidateProtocol(contract, cb); err != nil {
		return local.ApplyResult{}, err
	}
	// A declared guard is checked against the pinned plan before the Run
	// exists. What is refusable now is refused now: a registration that could
	// only fail later would fail while something already waits on it.
	for _, declaration := range options.Guards {
		if err := validateGuardDeclaration(declaration, plan); err != nil {
			return local.ApplyResult{}, err
		}
	}
	rootID := newID("invocation")
	activationID := newID("activation")
	if plan.Workflow.Definition.Stages[plan.Workflow.Definition.Entry].Kind == "wait" {
		activationID = waitActivationID(runID, rootID, plan.Workflow.Definition.Entry)
	}
	stepID := newID("step")
	zero := int64(0)
	pin, blocked, err := e.admissionGate(ctx)
	if err != nil {
		return local.ApplyResult{}, err
	}
	packageRefs := make([]flow.Ref, 0, len(defs)+len(resources))
	for _, definition := range defs {
		packageRefs = append(packageRefs, definition.Ref)
	}
	for _, resource := range resources {
		packageRefs = append(packageRefs, resource.Ref)
	}
	packagePin, packageBlocked, err := e.packageAdmissionGate(ctx, packageRefs, true)
	if err != nil {
		return local.ApplyResult{}, err
	}
	if blocked == nil {
		blocked = packageBlocked
	}
	pins := []local.ControlPin{}
	if packagePin != nil {
		pins = append(pins, *packagePin)
	}
	return e.applyControlledWithPins(ctx, pin, pins, e.owner, options.CommandID, runID, "run.created", startCommand, &zero, local.CommandCAS, func(r *Run, s local.Snapshot, obs Observation) (local.Change, error) {
		if blocked != nil {
			return local.Change{}, blocked
		}
		if r.ID != "" {
			return local.Change{}, local.Reject("run_exists", "run already exists")
		}
		stateVersion := StateVersion
		if plan.Profile == flow.CoreProfile {
			stateVersion = CoreStateVersion
		}
		if configurations != nil {
			stateVersion = CoreInvocationStateVersion
			if requiresRepeatState(plan) {
				stateVersion = CoreRepeatStateVersion
			}
			if requiresContextState(plan) || requiresSessionState(defs, plan) {
				stateVersion = CoreWaiverStateVersion
			}
			if requiresParallelState(plan) {
				stateVersion = CoreParallelStateVersion
			}
			if requiresMapState(plan) {
				stateVersion = CoreMapStateVersion
			}
			if requiresWaitState(plan) {
				stateVersion = CoreWaitStateVersion
			}
			if len(options.Guards) != 0 {
				stateVersion = CoreGuardStateVersion
			}
			if requiresSessionState(defs, plan) {
				stateVersion = CoreReportedCostStateVersion
			}
			if requiresArtifactPublicationState(plan) {
				stateVersion = CoreArtifactPublicationStateVersion
			}
			if requiresArtifactClosureState(plan) {
				stateVersion = CoreArtifactClosureStateVersion
			}
			if requiresPublicationSubscriptionState(plan) {
				stateVersion = CorePublicationSubscriptionStateVersion
			}
			if requiresPublicationChecksState(plan) {
				stateVersion = CorePublicationChecksStateVersion
			}
			if requiresPublicationNewOnlyState(plan) {
				stateVersion = CorePublicationNewOnlyStateVersion
			}
			if requiresPublicationFailureState(plan) {
				stateVersion = CorePublicationFailureStateVersion
			}
			if requiresSessionState(defs, plan) {
				stateVersion = CoreActionDeliveryStateVersion
				if options.WorkspaceMode != "" {
					stateVersion = CoreWorkspaceStateVersion
				}
			}
			if requiresWorkspaceTreeState(plan) {
				stateVersion = CoreWorkspaceTreeStateVersion
			}
		} else if len(options.Guards) != 0 {
			// Guards live on scoped invocations. A flat Run has no scope to
			// register one against, so the declaration is refused rather than
			// accepted and quietly ignored.
			return local.Change{}, local.Reject("unsupported_guards", "live guards require the scoped invocation state")
		}
		if options.DecisionCatalog != nil {
			if plan.Profile != flow.CoreProfile || configurations == nil {
				return local.Change{}, local.Reject("unsupported_decisions", "decision sheets require scoped core state")
			}
			stateVersion = CoreDecisionStateVersion
		}
		if neutral {
			stateVersion = CoreNeutralStateVersion
		}
		ledger := decisionInitialLedger(options.DecisionSheet, obs)
		*r = Run{SchemaVersion: stateVersion, ID: runID, AuthorityID: e.Installation.ID, ProjectID: e.Config.ID, Profile: plan.Profile, TrustProfile: "core-local/cooperative", InteractionMode: "with_human", ExecutionMode: "managed", CapacityProfile: "foundation:one-slot", Status: "ready", RootInvocationID: rootID, WorkflowRef: workflowRef, Workflow: plan.Canonical, Definitions: defs, Executors: executors, EffectiveConfiguration: effective, Brief: briefRef, LockRef: lockRef, Inputs: inputs, Outputs: map[string]ArtifactRef{}, DecisionCatalog: options.DecisionCatalog, DecisionSheet: options.DecisionSheet, DecisionLedger: ledger, Ready: []string{plan.Workflow.Definition.Entry}, Active: []string{}, Activations: map[string]*Activation{}, Steps: map[string]*Step{}, Attempts: map[string]*Attempt{}, Stops: []Stop{}, Publications: []Publication{}, Diagnostics: []Diagnostic{}, Created: obs, CoreBuild: Version, Gaps: []TimingGap{}, Transitions: []StateChange{}}
		if configurations != nil {
			r.Ready = nil
			r.WorkflowConfigurations = configurations
			r.Invocations = map[string]*Invocation{rootID: {ID: rootID, RunID: runID, WorkflowRef: workflowRef, Status: "ready", Inputs: inputs, Outputs: map[string]ArtifactRef{}, Ready: []string{plan.Workflow.Definition.Entry}, Created: obs}}
		}
		if requiresContextState(plan) {
			r.ContextResources = resources
			r.CheckExecutions = map[string]*CheckExecution{}
		}
		if err := r.beginWorkflowInputAcceptance(plan, rootID, obs); err != nil {
			return local.Change{}, err
		}
		if err := installGuards(r, options.Guards, e.owner, obs); err != nil {
			return local.Change{}, err
		}
		if r.PendingAcceptance == nil {
			// A guarded entry stage is not opened here. Nothing has been
			// observed yet, and opening the activation first and asking the
			// guard afterwards would be a start that no guard could prevent.
			guarded, _ := r.guardBlock(rootID, plan.Workflow.Definition.Entry)
			if guarded == "" {
				if err := activateFor(r, plan, rootID, plan.Workflow.Definition.Entry, activationID, stepID, s.EventSeq, obs); err != nil {
					return local.Change{}, err
				}
			}
		}
		if requiresContextState(plan) {
			created, err := canonical(map[string]any{"observation": obs, "status": r.Status})
			if err != nil {
				return local.Change{}, err
			}
			pinned, err := canonical(map[string]any{"observation": obs, "state_version": r.SchemaVersion, "package_lock_ref": r.LockRef})
			return local.Change{Events: []local.EventInput{{Type: "run.created", Version: 1, Data: created}, {Type: "run.context_pinned", Version: 1, Data: pinned}}}, err
		}
		return local.Change{}, nil
	})
}

// startRunID derives a Run's identity from who asked and which command asked.
// A caller that has to name the Run before it exists - a schedule reserving a
// slot - derives the same identity here rather than repeating the formula.
func startRunID(owner, commandID string) string {
	return fmt.Sprintf("run:%x", sha256.Sum256([]byte(owner+"/"+commandID)))
}

func sameRef(a, b *flow.Ref) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
func portMedia(p flow.Port) string {
	if p.Format == "json" {
		return "application/json"
	}
	if len(p.MediaTypes) == 1 {
		return p.MediaTypes[0]
	}
	return "application/octet-stream"
}
func (e *Engine) validatePortArtifact(p *flow.Plan, port flow.Port, a Artifact, data []byte) error {
	if a.Format != port.Format || port.Format == "json" && !sameRef(a.SchemaRef, port.SchemaRef) {
		return fault("binding_type_mismatch", "artifact format/schema differs from port")
	}
	if port.SchemaRef != nil && port.SchemaRef.ID == sourceSnapshotSchemaID {
		// Schema shape alone cannot assert that a source adapter acquired the
		// bytes. Every binding/preview/condition uses the same provenance guard.
		if _, err := e.SourceSnapshot(a.Ref()); err != nil {
			return err
		}
	}
	if port.Format == "json" {
		return p.ValidateJSON(*port.SchemaRef, data)
	}
	if len(port.MediaTypes) > 0 {
		accepted := false
		for _, label := range port.MediaTypes {
			media, err := artifactMediaType("blob", []string{label})
			if err != nil {
				return err
			}
			accepted = accepted || media == a.MediaType
		}
		if !accepted {
			return fault("binding_media_mismatch", "import an artifact with an accepted media type and bind its exact reference")
		}
	}
	if port.SchemaRef != nil {
		descriptor, err := canonical(map[string]any{"media_type": a.MediaType, "size_bytes": a.SizeBytes, "digest": a.Digest})
		if err != nil {
			return err
		}
		return p.ValidateJSON(*port.SchemaRef, descriptor)
	}
	return nil
}

func activateFor(r *Run, p *flow.Plan, invocationID, stageID, activationID, stepID string, eventSeq int64, obs Observation) error {
	stage := p.Workflow.Definition.Stages[stageID]
	steps := int64(0)
	if stage.Kind == "step" {
		steps = 1
	}
	if err := r.chargeInvocation(invocationID, 1, steps); err != nil {
		return err
	}
	a := &Activation{ID: activationID, StageID: stageID, InvocationID: invocationID, Kind: stage.Kind, Status: "ready", Created: obs}
	if stage.Kind == "repeat" {
		if !isRepeatState(r.SchemaVersion) {
			return local.ErrIntegrity
		}
		a.Repeat = &RepeatProgress{}
		body := p.Repeats[stageID]
		seen := map[flow.Ref]bool{}
		for _, binding := range stage.InitialBindings {
			if binding.From != "subscription" || binding.SourceRef == nil || seen[*binding.SourceRef] {
				continue
			}
			if !isPublicationSubscriptionState(r.SchemaVersion) || body == nil {
				return local.ErrIntegrity
			}
			source, ok := body.PublicationSource(*binding.SourceRef)
			if !ok || source.Mode != "each_publication" || len(r.PublicationSubscriptions) >= MaxPublicationSubscriptions {
				return local.ErrIntegrity
			}
			version := PublicationSubscriptionVersion
			var start *int64
			if source.Initial == "new_only" {
				if !isPublicationNewOnlyState(r.SchemaVersion) {
					return local.ErrIntegrity
				}
				version, start = PublicationNewOnlySubscriptionVersion, &eventSeq
			}
			id := publicationSubscriptionID(r.ID, activationID, *binding.SourceRef)
			if r.PublicationSubscriptions == nil {
				r.PublicationSubscriptions = map[string]*PublicationSubscription{}
			}
			if r.PublicationSubscriptions[id] != nil {
				return local.ErrIntegrity
			}
			r.PublicationSubscriptions[id] = &PublicationSubscription{
				SchemaVersion: version, ID: id, RunID: r.ID,
				InvocationID: invocationID, RepeatActivationID: activationID, SourceRef: *binding.SourceRef,
				Generation: 1, Cursor: 0, PublicationStartSequence: start, Status: "open", Created: obs,
			}
			seen[*binding.SourceRef] = true
		}
	}
	if stage.Kind == "parallel" {
		if !isParallelState(r.SchemaVersion) {
			return local.ErrIntegrity
		}
		// The declared order is sealed here, from the pinned plan, so the
		// branches this activation owns cannot be renumbered afterwards.
		ids := make([]string, 0, len(stage.ParallelBranches))
		for _, branch := range stage.ParallelBranches {
			ids = append(ids, branch.ID)
		}
		a.Parallel = &ParallelProgress{BranchIDs: ids}
	}
	if stage.Kind == "wait" {
		if !isWaitState(r.SchemaVersion) {
			return local.ErrIntegrity
		}
		// A promise may already exist: something started an external job and
		// reserved this wait so a fast answer would have somewhere to land.
		// Activation adopts that promise rather than making a second one, which
		// is what keeps the identity the sender was given valid.
		const firstGeneration = 1
		id := waitRegistrationID(r.ID, activationID, firstGeneration)
		registration := r.Waits[id]
		if registration == nil {
			if len(r.Waits) >= MaxWaitRegistrations {
				return local.Reject("wait_registrations_exhausted", "this run already holds the most registrations it may")
			}
			registration = &WaitRegistration{SchemaVersion: WaitRegistrationVersion, ID: id,
				RunID: r.ID, InvocationID: invocationID, TargetStageID: stageID, ActivationID: activationID,
				SourceRef: stage.SourceRef, EventSchemaRef: stage.EventSchemaRef, Generation: firstGeneration,
				Nonce: waitNonce(r.ID, activationID, firstGeneration), Status: "reserved"}
			if source, publication := p.PublicationSource(stage.SourceRef); publication && source.Initial == "new_only" {
				if !isPublicationNewOnlyState(r.SchemaVersion) {
					return local.ErrIntegrity
				}
				registration.PublicationStartSequence = &eventSeq
			}
			if r.Waits == nil {
				r.Waits = map[string]*WaitRegistration{}
			}
			r.Waits[id] = registration
		} else if registration.Status != "reserved" || registration.InvocationID != invocationID || registration.TargetStageID != stageID {
			return local.Reject("wait_reservation_conflict", "the promise made about this wait is not one it can adopt")
		}
		a.Wait = &WaitProgress{RegistrationID: registration.ID, Generation: firstGeneration}
	}
	if stage.Kind == "map" {
		if !isMapState(r.SchemaVersion) {
			return local.ErrIntegrity
		}
		// A map's branches are not known here: the collection is read and
		// sealed when the stage is entered, so this activation starts with no
		// identities rather than guessed ones.
		a.Parallel = &ParallelProgress{BranchIDs: []string{}}
	}
	if stage.Kind == "step" {
		a.StepID = stepID
		r.Steps[stepID] = &Step{ID: stepID, ActivationID: activationID, Ref: stage.StepRef, Status: "ready", AttemptIDs: []string{}, Outputs: map[string]ArtifactRef{}, Created: obs}
	}
	r.Activations[activationID] = a
	return nil
}

func activationFor(r *Run, stageID string) *Activation {
	return r.activationForInvocation(r.RootInvocationID, stageID)
}
func bindingRefs(r Run, invocationID string, bindings map[string]flow.Binding) (map[string]ArtifactRef, error) {
	return bindingRefsForBody(r, invocationID, "", bindings)
}

func bindingRefsForBody(r Run, invocationID, bodyID string, bindings map[string]flow.Binding) (map[string]ArtifactRef, error) {
	refs := map[string]ArtifactRef{}
	for name, b := range bindings {
		switch b.From {
		case "workflow_input":
			if ref, ok := r.inputsFor(invocationID)[b.Port]; ok {
				refs[name] = ref
			}
		case "stage_output":
			a := r.activationForInvocation(invocationID, b.StageID)
			if a != nil && a.Kind == "wait" {
				if a.Status == "completed" && a.Wait != nil && (a.Wait.Resolution == "event" || a.Wait.Resolution == "interrupted") && a.Wait.EventRef != nil {
					if b.Port != flow.WaitEventPort {
						return nil, errors.New("binding wait producer has no such output")
					}
					refs[name] = *a.Wait.EventRef
					continue
				}
				if a.Status == "failed" || a.Status == "completed" && a.Wait != nil && a.Wait.Resolution == "timeout" {
					continue
				}
				return nil, errors.New("binding wait producer has not received an event")
			}
			if a != nil && fanOut(a.Kind) {
				// A settled join reports its branches; a failed one produced no
				// join result at all, so it exports nothing rather than an
				// empty summary that would read as "no branches went wrong".
				if a.Status == "completed" && a.Parallel != nil && a.Parallel.ResultsRef != nil {
					if b.Port == flow.AggregateResultsPort {
						refs[name] = *a.Parallel.ResultsRef
					}
					continue
				}
				if a.Status == "failed" {
					continue
				}
				return nil, errors.New("binding join producer has not settled")
			}
			if a != nil && (a.Kind == "call" || a.Kind == "repeat") {
				child := r.currentBody(a)
				if a.Status == "completed" && child != nil && child.Status == "completed" {
					if ref, ok := child.Outputs[b.Port]; ok {
						refs[name] = ref
					}
					continue
				}
				if a.Status == "failed" {
					continue
				}
				return nil, errors.New("binding body producer has no accepted exports")
			}
			if a == nil || a.StepID == "" {
				if r.Profile == flow.CoreProfile && a == nil {
					continue // An optional producer may be absent on this path.
				}
				return nil, errors.New("binding producer was not executed")
			}
			step := r.Steps[a.StepID]
			if step == nil {
				return nil, errors.New("binding producer identity is missing")
			}
			if step.Status != "completed" {
				if r.Profile == flow.CoreProfile && step.Status == "failed" {
					continue // on_error never publishes unaccepted outputs.
				}
				return nil, errors.New("binding producer has no accepted result")
			}
			if ref, ok := step.Outputs[b.Port]; ok {
				refs[name] = ref
			}
		case "iteration_output":
			body, err := r.iterationBody(invocationID, bodyID)
			if err != nil || b.StageID != "" {
				return nil, errors.New("binding iteration output has no accepted current body")
			}
			if ref, ok := body.Outputs[b.Port]; ok {
				refs[name] = ref
			}
		case "publication":
			a := r.activationForInvocation(invocationID, b.StageID)
			if a == nil || a.Kind != "wait" || a.Status != "completed" || a.Wait == nil || a.Wait.PublicationAssignmentID == "" {
				return nil, errors.New("binding publication producer has no assigned item")
			}
			for i := range r.PublicationAssignments {
				assignment := r.PublicationAssignments[i]
				if assignment.ID == a.Wait.PublicationAssignmentID {
					if assignment.Kind != "Item" || assignment.Item == nil || assignment.WaitActivationID != a.ID {
						return nil, errors.New("binding publication producer did not assign an item")
					}
					refs[name] = *assignment.Item
					break
				}
			}
			if _, ok := refs[name]; !ok {
				return nil, errors.New("binding publication assignment is missing")
			}
		case "subscription": // Materialized by prepareRepeatBodyInputs.
		case "literal": // Materialized with a typed schema outside the transaction by prepareBindings.
		default:
			return nil, errors.New("unsupported binding")
		}
	}
	return refs, nil
}
func (e *Engine) prepareBindings(r Run, invocationID string, bindings map[string]flow.Binding, ports map[string]flow.InputPort, commandID string) (map[string]ArtifactRef, error) {
	return e.prepareBindingsForBody(r, invocationID, "", bindings, ports, commandID, "")
}

// scope separates literals prepared by one command for different destinations.
// One command opens every branch of a fan-out, and two branches may bind the
// same port to different values: without the scope both would claim one
// artifact identity and the second would collide with the first.
func (e *Engine) prepareBindingsForBody(r Run, invocationID, bodyID string, bindings map[string]flow.Binding, ports map[string]flow.InputPort, commandID, scope string) (map[string]ArtifactRef, error) {
	refs, err := bindingRefsForBody(r, invocationID, bodyID, bindings)
	if err != nil {
		return nil, err
	}
	p, err := r.planFor(invocationID)
	if err != nil {
		return nil, err
	}
	for name, b := range bindings {
		if b.From == "literal" {
			a, err := e.putArtifact(b.Value, "json", b.SchemaRef, fmt.Sprintf("artifact:%x", sha256.Sum256([]byte(commandID+"/literal/"+scope+"/"+name))), map[string]any{"kind": "authority", "authority_id": e.Installation.ID, "command_id": commandID, "port": name}, nil, p.Registry)
			if err != nil {
				return nil, err
			}
			refs[name] = a.Ref()
		}
		if source, ok := refs[name]; ok && b.Pointer != nil {
			a, data, err := e.Artifact(source)
			if err != nil {
				return nil, err
			}
			var sourcePort flow.Port
			switch b.From {
			case "workflow_input":
				sourcePort = p.Workflow.Inputs[b.Port].Port
			case "stage_output":
				sourcePort = p.StageOutputs(b.StageID)[b.Port].Port
			case "iteration_output":
				body, err := r.iterationBody(invocationID, bodyID)
				if err != nil {
					return nil, err
				}
				bodyPlan := p.BodyPlan(r.Activations[body.CallerActivationID].StageID)
				if bodyPlan == nil || planRef(bodyPlan) != body.WorkflowRef {
					return nil, local.ErrIntegrity
				}
				sourcePort = bodyPlan.Workflow.Outputs[b.Port].Port
			default:
				return nil, errors.New("unsupported projection source")
			}
			if err := e.validatePortArtifact(p, sourcePort, a, data); err != nil {
				return nil, err
			}
			projected, present, err := p.ProjectJSON(b, data)
			if err != nil {
				return nil, err
			}
			if !present {
				delete(refs, name)
				continue
			}
			manifest := ProjectionManifest{SchemaVersion: "json-projection/1", Source: source, Pointer: *b.Pointer, ProjectedSchemaRef: *b.ProjectedSchemaRef, WorkflowRef: planRef(p)}
			manifestBytes, err := canonical(manifest)
			if err != nil {
				return nil, err
			}
			manifestSchema := builtinRef(r.Definitions, "core:schema/json-projection")
			producer := map[string]any{"kind": "authority", "authority_id": r.AuthorityID, "command_id": commandID, "port": name}
			manifestArtifact, err := e.putArtifact(manifestBytes, "json", &manifestSchema, derivedID("artifact", commandID, "projection_manifest", name), producer, []ArtifactRef{source}, r.registry())
			if err != nil {
				return nil, err
			}
			derived, err := e.putArtifact(projected, "json", b.ProjectedSchemaRef, derivedID("artifact", commandID, "projection", name), producer, []ArtifactRef{source, manifestArtifact.Ref()}, p.Registry)
			if err != nil {
				return nil, err
			}
			refs[name] = derived.Ref()
		}
	}
	for name, port := range ports {
		ref, ok := refs[name]
		if !ok {
			if port.Required {
				return nil, errors.New("required binding is absent")
			}
			continue
		}
		a, data, err := e.Artifact(ref)
		if err != nil {
			return nil, err
		}
		if err := e.validatePortArtifact(p, port.Port, a, data); err != nil {
			return nil, err
		}
	}
	return refs, nil
}

// Defaults are pinned once at Start. Explicit bindings own absence as well as
// values; a later body never inherits an earlier body's inputs implicitly.
// supplied names ports whose artifact the caller already holds and will set
// itself. A map's item port is the only one: the item comes from the sealed
// collection rather than from a binding, so demanding a binding for it would
// refuse every valid map.
func (e *Engine) prepareBodyInputs(r Run, parentID, bodyID string, body *flow.Plan, bindings map[string]flow.Binding, commandID, scope string, supplied ...string) (map[string]ArtifactRef, error) {
	config := r.WorkflowConfigurations[body.Digest]
	if config == nil || config.WorkflowRef != planRef(body) {
		return nil, local.ErrIntegrity
	}
	ports := maps.Clone(body.Workflow.Inputs)
	for _, name := range supplied {
		if _, declared := ports[name]; !declared {
			return nil, local.ErrIntegrity
		}
		delete(ports, name)
	}
	for name, port := range ports {
		if _, declared := bindings[name]; !declared && len(config.Inputs[name].Value) > 0 {
			port.Required = false
			ports[name] = port
		}
	}
	inputs, err := e.prepareBindingsForBody(r, parentID, bodyID, bindings, ports, commandID, scope)
	if err != nil {
		return nil, err
	}
	for name, port := range body.Workflow.Inputs {
		if _, declared := bindings[name]; declared {
			continue
		}
		if _, own := ports[name]; !own {
			continue
		}
		value := config.Inputs[name].Value
		if len(value) == 0 {
			if port.Required {
				return nil, errors.New("missing required body input")
			}
			continue
		}
		artifact, err := e.putArtifact(value, port.Format, port.SchemaRef, derivedID("artifact", commandID, name), map[string]any{"kind": "authority", "authority_id": r.AuthorityID, "command_id": commandID, "port": name}, nil, body.Registry, portMedia(port.Port))
		if err != nil {
			return nil, err
		}
		inputs[name] = artifact.Ref()
	}
	return inputs, nil
}
