package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

type ForkCommand struct {
	SchemaVersion      string      `json:"schema_version"`
	CommandID          string      `json:"command_id"`
	RunID              string      `json:"run_id"`
	ExpectedRunVersion int64       `json:"expected_run_version"`
	Payload            ForkPayload `json:"payload"`
}

type ForkPayload struct {
	WorkflowRef flow.Ref               `json:"workflow_ref"`
	BriefRef    ArtifactRef            `json:"brief_ref"`
	Inputs      map[string]ArtifactRef `json:"inputs"`
	ReuseRefs   []ArtifactRef          `json:"reuse_refs"`
	Reason      string                 `json:"reason"`
}

// Fork creates a new Run from explicit sealed materials. It never reopens or
// alters the source Run, and its transaction checks the source version again
// after all files and artifacts have been prepared.
func (e *Engine) Fork(ctx context.Context, command ForkCommand) (local.ApplyResult, error) {
	if e.ReadOnly {
		return local.ApplyResult{}, local.ErrReadOnly
	}
	encoded, err := canonical(command)
	if err != nil {
		return local.ApplyResult{}, err
	}
	if err := flow.ValidateProtocol("ForkCommand", encoded); err != nil {
		return local.ApplyResult{}, err
	}
	if command.SchemaVersion != "1" || command.CommandID == "" || command.RunID == "" || command.ExpectedRunVersion < 1 {
		return local.ApplyResult{}, errors.New("invalid fork command")
	}
	source, sourceView, err := e.load(ctx, command.RunID)
	if err != nil {
		return local.ApplyResult{}, err
	}
	if sourceView.Snapshot.Version != command.ExpectedRunVersion {
		return local.ApplyResult{}, local.Reject("version_conflict", "expected source run version differs from current version")
	}
	if source.Profile != flow.CoreProfile {
		return local.ApplyResult{}, local.Reject("unsupported_fork", "fork requires the versioned core workflow profile")
	}
	reusePin, err := e.reuseTrustPin(ctx, source, command.Payload.ReuseRefs)
	if err != nil {
		return local.ApplyResult{}, err
	}
	prepared, err := e.prepareFork(command, source)
	if err != nil {
		return local.ApplyResult{}, err
	}
	pin, blocked, err := e.admissionGate(ctx)
	if err != nil {
		return local.ApplyResult{}, err
	}
	pins := []local.ControlPin{}
	if pin != nil {
		pins = append(pins, *pin)
	}
	if reusePin != nil {
		pins = append(pins, *reusePin)
	}
	newRunID := startRunID(e.owner, command.CommandID)
	return e.Store.CreateLinkedRun(ctx, local.LinkedRunCommand{ID: command.CommandID, Actor: e.owner, SourceRunID: command.RunID, NewRunID: newRunID, Payload: encoded, ExpectedVersion: command.ExpectedRunVersion, Pins: pins}, func(snapshot local.Snapshot) (local.Change, error) {
		if blocked != nil {
			return local.Change{}, blocked
		}
		var current Run
		if err := decodeState(snapshot.Data, &current); err != nil || !supportedRun(current) || current.ID != command.RunID || current.AuthorityID != e.Installation.ID || current.ProjectID != e.Config.ID {
			return local.Change{}, local.Reject("incompatible_run", "source run is unsupported or foreign")
		}
		return prepared.change(e, newRunID, snapshot.Version, e.clock.now())
	})
}

type preparedFork struct {
	command        ForkCommand
	plan           *flow.Plan
	definitions    []PinnedDefinition
	resources      []PinnedResource
	executors      map[string]PinnedExecutor
	effective      *EffectiveConfiguration
	configurations map[string]*EffectiveConfiguration
	inputs         map[string]ArtifactRef
	brief          ArtifactRef
	lock           flow.Ref
}

func (e *Engine) prepareFork(command ForkCommand, source Run) (preparedFork, error) {
	prepared := preparedFork{command: command}
	plan, definitions, resources, err := e.compileForkWorkflow(command.Payload.WorkflowRef)
	if err != nil {
		return prepared, err
	}
	prepared.plan, prepared.resources = plan, resources
	artifact, brief, err := e.Artifact(command.Payload.BriefRef)
	if err != nil {
		return prepared, err
	}
	if artifact.Format != "blob" || flow.ValidateProtocol("RunBrief", brief) != nil {
		return prepared, local.Reject("invalid_brief", "fork brief must be one sealed RunBrief artifact")
	}
	effective, err := e.effectiveConfiguration(plan, nil, command.Payload.Inputs, true)
	if err != nil {
		return prepared, err
	}
	for port := range command.Payload.Inputs {
		if _, exists := plan.Workflow.Inputs[port]; !exists {
			return prepared, local.Reject("binding_type_mismatch", "fork input does not name a workflow port")
		}
	}
	inputs := map[string]ArtifactRef{}
	for name, port := range plan.Workflow.Inputs {
		ref, present := command.Payload.Inputs[name]
		if !present {
			if port.Required && (effective == nil || len(effective.Inputs[name].Value) == 0) {
				return prepared, local.Reject("binding_type_mismatch", "fork is missing a required workflow input")
			}
			continue
		}
		candidate, data, err := e.Artifact(ref)
		if err != nil {
			return prepared, err
		}
		if err := e.validatePortArtifact(plan, port.Port, candidate, data); err != nil {
			return prepared, err
		}
		inputs[name] = ref
	}
	if err := validateForkReuse(source, command.Payload, inputs); err != nil {
		return prepared, err
	}
	configurations, err := e.workflowConfigurations(plan, effective)
	if err != nil {
		return prepared, err
	}
	if configurations == nil {
		configurations = map[string]*EffectiveConfiguration{plan.Digest: effective}
	}
	prepared.effective, prepared.configurations, prepared.inputs = effective, configurations, inputs
	definitions, err = e.forkDefinitions(plan, definitions)
	if err != nil {
		return prepared, err
	}
	executors, err := e.forkExecutors(plan, definitions)
	if err != nil {
		return prepared, err
	}
	configVersion := "core-run-configuration/2"
	configSchema := builtinRef(definitions, "core:schema/core-configuration")
	if requiresContextState(plan) {
		configVersion = "core-run-configuration/3"
		configSchema = builtinVersionRef(definitions, "core:schema/core-configuration", "2.0.0")
	}
	configBytes, err := canonical(map[string]any{"schema_version": configVersion, "semantics_profile": plan.Profile, "configuration_schema_ref": configSchema, "executors": executors, "effective_configuration": effective, "workflow_configurations": configurations})
	if err != nil {
		return prepared, err
	}
	configDigest, _ := flow.Digest(configBytes)
	configRef := flow.Ref{ID: "snapshot:executors/" + strings.TrimPrefix(configDigest, "sha256:"), Version: "1.0.0", Digest: configDigest}
	definitions = append(definitions, PinnedDefinition{Ref: configRef, Kind: "resource", RawDigest: rawDigest(configBytes), Bytes: configBytes})
	prepared.definitions, prepared.executors = definitions, executors
	if err := e.pinDefinitions(definitions); err != nil {
		return prepared, err
	}
	if err := e.pinResources(resources); err != nil {
		return prepared, err
	}
	if requiresContextState(plan) {
		snapshot, err := contextResourceSnapshot(resources)
		if err != nil {
			return prepared, err
		}
		definitions = append(definitions, snapshot)
		prepared.definitions = definitions
	}
	if err := prepared.lockDefinitions(); err != nil {
		return prepared, err
	}
	return prepared, nil
}

func (e *Engine) compileForkWorkflow(ref flow.Ref) (*flow.Plan, []PinnedDefinition, []PinnedResource, error) {
	definitions, registry, resources, err := e.inventoryResources()
	if err != nil {
		return nil, nil, nil, err
	}
	var found *PinnedDefinition
	for i := range definitions {
		if definitions[i].Ref == ref && definitions[i].Kind == "workflow" {
			found = &definitions[i]
			break
		}
	}
	if found == nil {
		return nil, nil, nil, local.Reject("workflow_unavailable", "fork workflow reference is not currently installed and trusted")
	}
	var plan *flow.Plan
	if e.Config.Configuration.SchemaVersion == CoreContextConfigVersion {
		contextResources, err := resourcesFromPins(resources)
		if err != nil {
			return nil, nil, nil, err
		}
		plan, err = flow.CompileCore(found.Bytes, "json", registry, contextResources)
	} else {
		plan, err = flow.CompileProfile(found.Bytes, "json", registry, flow.CoreProfile)
	}
	if err != nil {
		return nil, nil, nil, err
	}
	if planRef(plan) != ref {
		return nil, nil, nil, local.Reject("workflow_unavailable", "fork workflow bytes differ from their immutable reference")
	}
	if err := e.selectContextProfiles(plan, definitions, registry); err != nil {
		return nil, nil, nil, err
	}
	if err := e.checkCapabilities(plan); err != nil {
		return nil, nil, nil, err
	}
	selected := resources[:0]
	for _, resource := range resources {
		if _, needed := plan.Resources[resource.Ref]; needed {
			selected = append(selected, resource)
		}
	}
	return plan, definitions, selected, nil
}

func validateForkReuse(source Run, payload ForkPayload, inputs map[string]ArtifactRef) error {
	if len(payload.ReuseRefs) != 0 && !source.terminal() {
		return local.Reject("invalid_reuse", "fork may reuse outputs only after its source run is complete")
	}
	completed := map[ArtifactRef]bool{}
	for _, ref := range source.Outputs {
		completed[ref] = true
	}
	seen := map[ArtifactRef]bool{}
	for _, ref := range payload.ReuseRefs {
		if seen[ref] {
			return local.Reject("invalid_reuse", "fork reuse references must be unique")
		}
		seen[ref] = true
		if !completed[ref] {
			return local.Reject("invalid_reuse", "fork may reuse only an explicit completed output of its source run")
		}
		used := false
		for _, input := range inputs {
			used = used || input == ref
		}
		if !used {
			return local.Reject("invalid_reuse", "fork reuse reference must be an explicit input of the new run")
		}
	}
	return nil
}

func (e *Engine) forkDefinitions(plan *flow.Plan, definitions []PinnedDefinition) ([]PinnedDefinition, error) {
	transport := map[flow.Ref]bool{}
	for _, id := range []string{"core:schema/local-configuration", "core:schema/local-context", "core:schema/step-result", "core:profile/redaction", "core:resolver/local", "core:adapter/local-process", "core:policy/local"} {
		transport[builtinRef(definitions, id)] = true
	}
	if requiresParallelState(plan) || requiresMapState(plan) {
		transport[builtinRef(definitions, flow.AggregateSchemaID)] = true
	}
	if requiresArtifactClosureState(plan) {
		transport[builtinRef(definitions, "core:schema/artifact-manifest")] = true
	}
	if requiresPublicationSubscriptionState(plan) {
		for _, id := range []string{publicationHandleSchemaID, publicationCursorSchemaID, publicationDeliverySchemaID} {
			transport[builtinRef(definitions, id)] = true
		}
	}
	if requiresContextState(plan) {
		for _, id := range []string{"core:schema/local-context", "core:resolver/local", "core:adapter/local-process"} {
			transport[builtinVersionRef(definitions, id, "2.0.0")] = true
		}
		for _, id := range []string{"core:schema/context-profile", "core:schema/full-context", "core:schema/context-json", "core:schema/evidence", "core:context/local-json", "core:assembly/local-json"} {
			transport[builtinRef(definitions, id)] = true
		}
	}
	projection := builtinRef(definitions, "core:schema/json-projection")
	selected := definitions[:0]
	for _, definition := range definitions {
		if plan.Registry[definition.Ref] != nil || transport[definition.Ref] || definition.Ref == e.Config.ConfigurationSchemaRef || definition.Ref == projection {
			selected = append(selected, definition)
		}
	}
	return selected, nil
}

func (e *Engine) forkExecutors(plan *flow.Plan, definitions []PinnedDefinition) (map[string]PinnedExecutor, error) {
	executors := map[string]PinnedExecutor{}
	for ref, executor := range executorDefinitions(plan) {
		if _, exists := executors[ref.String()]; exists || isAssistedExecutor(definitions, executor) {
			continue
		}
		config := e.Config.Configuration.Executors[ref.ID]
		resolved, err := filepath.EvalSymlinks(config.Executable)
		if err != nil {
			return nil, err
		}
		config.Executable = resolved
		digest, err := local.ProcessExecutableDigest(config.Executable)
		if err != nil {
			return nil, err
		}
		pinned := PinnedExecutor{Config: config, ExecutableDigest: digest, Files: map[string]local.BlobRef{}}
		if requiresContextState(plan) && executor.AdapterRef == builtinVersionRef(definitions, "core:adapter/local-process", "2.0.0") {
			registry := flow.Registry{}
			for _, definition := range definitions {
				registry[definition.Ref] = definition.Bytes
			}
			profile, ref, err := contextProfileFor(config, definitions, registry)
			if err != nil {
				return nil, err
			}
			pinned.Config.ContextProfileRef, pinned.ContextProfile = &ref, &profile
		}
		for target, source := range config.Files {
			data, err := readLocal(e.Root, source, MaxArtifactBytes)
			if err != nil {
				return nil, err
			}
			blob, err := e.Blobs.Put(bytes.NewReader(data), MaxArtifactBytes)
			if err != nil {
				return nil, err
			}
			pinned.Files[target] = blob
		}
		executors[ref.String()] = pinned
	}
	return executors, nil
}

func (p *preparedFork) lockDefinitions() error {
	closure := make([]flow.Ref, 0, len(p.definitions)+len(p.resources))
	for _, definition := range p.definitions {
		closure = append(closure, definition.Ref)
	}
	for _, resource := range p.resources {
		closure = append(closure, resource.Ref)
	}
	sort.Slice(closure, func(i, j int) bool { return closure[i].String() < closure[j].String() })
	resolver := builtinRef(p.definitions, "core:resolver/local")
	if requiresContextState(p.plan) {
		resolver = builtinVersionRef(p.definitions, "core:resolver/local", "2.0.0")
	}
	lock := map[string]any{"schema_version": "1", "id": derivedID("lock", "fork", p.command.CommandID), "version": "1.0.0", "core_protocol": "1", "closure": closure, "resolver_ref": resolver}
	data, err := canonical(lock)
	if err != nil {
		return err
	}
	if err := flow.ValidateProtocol("PackageLock", data); err != nil {
		return err
	}
	digest, _ := flow.Digest(data)
	p.lock = flow.Ref{ID: lock["id"].(string), Version: "1.0.0", Digest: digest}
	p.definitions = append(p.definitions, PinnedDefinition{Ref: p.lock, Kind: "resource", RawDigest: rawDigest(data), Bytes: data})
	return nil
}

func (p preparedFork) change(e *Engine, runID string, sourceVersion int64, observed Observation) (local.Change, error) {
	rootID, activationID, stepID := newID("invocation"), newID("activation"), newID("step")
	if p.plan.Workflow.Definition.Stages[p.plan.Workflow.Definition.Entry].Kind == "wait" {
		activationID = waitActivationID(runID, rootID, p.plan.Workflow.Definition.Entry)
	}
	run := Run{SchemaVersion: CoreForkStateVersion, ID: runID, AuthorityID: e.Installation.ID, ProjectID: e.Config.ID, Profile: flow.CoreProfile, TrustProfile: "core-local/cooperative", InteractionMode: "with_human", ExecutionMode: "managed", CapacityProfile: "foundation:one-slot", Status: "ready", RootInvocationID: rootID, WorkflowRef: planRef(p.plan), Workflow: p.plan.Canonical, Definitions: p.definitions, ContextResources: p.resources, Executors: p.executors, EffectiveConfiguration: p.effective, WorkflowConfigurations: p.configurations, Fork: &ForkProvenance{SchemaVersion: "1", SourceRunID: p.command.RunID, SourceRunVersion: sourceVersion, CommandID: p.command.CommandID, Reason: p.command.Payload.Reason, ReuseRefs: p.command.Payload.ReuseRefs}, Brief: p.command.Payload.BriefRef, LockRef: p.lock, Inputs: p.inputs, Outputs: map[string]ArtifactRef{}, Ready: nil, Invocations: map[string]*Invocation{rootID: {ID: rootID, RunID: runID, WorkflowRef: planRef(p.plan), Status: "ready", Inputs: p.inputs, Outputs: map[string]ArtifactRef{}, Ready: []string{p.plan.Workflow.Definition.Entry}, Created: observed}}, Active: []string{}, Activations: map[string]*Activation{}, Steps: map[string]*Step{}, Attempts: map[string]*Attempt{}, CheckExecutions: map[string]*CheckExecution{}, Stops: []Stop{}, Publications: []Publication{}, Diagnostics: []Diagnostic{}, Created: observed, LastObserved: observed, CoreBuild: Version, Gaps: []TimingGap{}, Transitions: []StateChange{}}
	if err := run.beginWorkflowInputAcceptance(p.plan, rootID, observed); err != nil {
		return local.Change{}, err
	}
	if run.PendingAcceptance == nil {
		if err := activateFor(&run, p.plan, rootID, p.plan.Workflow.Definition.Entry, activationID, stepID, 0, observed); err != nil {
			return local.Change{}, err
		}
	}
	if err := invariant(run); err != nil {
		return local.Change{}, err
	}
	data, err := canonicalState(run)
	if err != nil {
		return local.Change{}, err
	}
	created, err := canonical(map[string]any{"observation": observed, "status": run.Status, "fork": run.Fork})
	if err != nil {
		return local.Change{}, err
	}
	pinned, err := canonical(map[string]any{"observation": observed, "state_version": run.SchemaVersion, "package_lock_ref": run.LockRef})
	if err != nil {
		return local.Change{}, err
	}
	return local.Change{Data: data, Events: []local.EventInput{{Type: "run.created", Version: 1, Data: created}, {Type: "run.context_pinned", Version: 1, Data: pinned}}, Result: json.RawMessage(fmt.Sprintf(`{"new_run_id":%q}`, runID)), RequireStorageBudget: true}, nil
}

func forkProvenanceInvariant(r Run) error {
	if r.Fork == nil {
		return nil
	}
	p := r.Fork
	if !isForkState(r.SchemaVersion) || p.SchemaVersion != "1" || p.SourceRunID == "" || p.SourceRunID == r.ID || p.SourceRunVersion < 1 || p.CommandID == "" || p.Reason == "" || p.ReuseRefs == nil {
		return errors.New("invalid fork provenance")
	}
	seen := map[ArtifactRef]bool{}
	for _, ref := range p.ReuseRefs {
		if seen[ref] {
			return errors.New("duplicate fork reuse reference")
		}
		seen[ref] = true
		if _, present := r.InputsPort(ref); !present {
			return errors.New("fork reuse reference is not a declared new input")
		}
	}
	return nil
}

// InputsPort says whether a sealed reference is an explicit input of this Run.
// Reuse is limited to declared inputs; no hidden historical artifact is made
// available just because the runs are related.
func (r Run) InputsPort(ref ArtifactRef) (string, bool) {
	for port, value := range r.Inputs {
		if value == ref {
			return port, true
		}
	}
	return "", false
}
