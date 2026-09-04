package runtime

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"sort"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// CapabilityManifest describes implemented contracts, not grants or release
// qualification. A profile's meaning is stable while its supported subset grows.
type CapabilityManifest struct {
	SchemaVersion    string                `json:"schema_version"`
	CoreBuild        string                `json:"core_build"`
	ProtocolVersions []string              `json:"protocol_versions"`
	StorageVersion   int                   `json:"storage_version"`
	EventVersion     int                   `json:"event_version"`
	Profiles         []ProfileCapabilities `json:"profiles"`
	Unsupported      []string              `json:"unsupported"`
}

type ProfileCapabilities struct {
	Profile          string   `json:"semantics_profile"`
	WorkflowVersions []string `json:"workflow_definition_versions"`
	StepVersions     []string `json:"step_definition_versions"`
	StateVersion     string   `json:"state_version"`
	ReadVersion      string   `json:"read_version"`
	Capabilities     []string `json:"capabilities"`
	StateVersions    []string `json:"state_versions,omitempty"`
	ReadVersions     []string `json:"read_versions,omitempty"`
}

func Capabilities() CapabilityManifest {
	base := []string{"step", "finish", "local_process", "state_hook", "event_hook", "telemetry.catalog", "telemetry.records", "telemetry.aggregate"}
	core := append(append([]string{}, base...), "on_error", "json_projection", "input_configuration", "choice", "call", "repeat", "partial", "local_workflow_aliases", "context_resources", "full_context", "source_import", "context_request", "automatic_checks", "assisted_session", "quality_waivers", "parallel", "map", "wait", "schedule", "live_guards", "reported_cost", "artifact_publication", "artifact_publication_checks", "artifact_close", "publication_subscription_once", "publication_subscription_each", "publication_subscription_new_only", "publication_subscription_terminal_failure", "publication_subscription_blob", "action_intent_proposal", "action_admission", "action_grant_admission", "action_delivery_prepared", "run_fork", "workspace_modes", "workspace_tree_artifacts", "decision_catalog")
	manifest := CapabilityManifest{
		SchemaVersion: "capabilities/2", CoreBuild: Version, ProtocolVersions: []string{"1"}, StorageVersion: local.StorageVersion, EventVersion: local.EventVersion,
		Profiles: []ProfileCapabilities{
			{Profile: flow.Profile, WorkflowVersions: []string{"1"}, StepVersions: []string{"1", "2"}, StateVersion: StateVersion, ReadVersion: ReadVersion, Capabilities: base, StateVersions: []string{StateVersion}, ReadVersions: []string{ReadVersion}},
			{Profile: flow.CoreProfile, WorkflowVersions: []string{"1", "2", "3"}, StepVersions: []string{"1", "2", "3", "4", "5"}, StateVersion: CoreDecisionStateVersion, ReadVersion: CoreDecisionReadVersion, Capabilities: core, StateVersions: []string{CoreStateVersion, CoreInvocationStateVersion, CoreRepeatStateVersion, CoreContextStateVersion, CoreSessionStateVersion, CoreWaiverStateVersion, CoreParallelStateVersion, CoreMapStateVersion, CoreWaitStateVersion, CoreGuardStateVersion, CoreReportedCostStateVersion, CoreArtifactPublicationStateVersion, CoreArtifactClosureStateVersion, CorePublicationSubscriptionStateVersion, CorePublicationChecksStateVersion, CorePublicationNewOnlyStateVersion, CorePublicationFailureStateVersion, CoreActionIntentStateVersion, CoreActionAdmissionStateVersion, CoreActionGrantAdmissionStateVersion, CoreActionDeliveryStateVersion, CoreForkStateVersion, CoreWorkspaceStateVersion, CoreWorkspaceTreeStateVersion, CoreDecisionStateVersion}, ReadVersions: []string{CoreReadVersion, CoreInvocationReadVersion, CoreRepeatReadVersion, CoreContextReadVersion, CoreSessionReadVersion, CoreWaiverReadVersion, CoreParallelReadVersion, CoreMapReadVersion, CoreWaitReadVersion, CoreGuardReadVersion, CoreReportedCostReadVersion, CoreArtifactPublicationReadVersion, CoreArtifactClosureReadVersion, CorePublicationSubscriptionReadVersion, CorePublicationChecksReadVersion, CorePublicationNewOnlyReadVersion, CorePublicationFailureReadVersion, CoreActionIntentReadVersion, CoreActionAdmissionReadVersion, CoreActionGrantAdmissionReadVersion, CoreActionDeliveryReadVersion, CoreForkReadVersion, CoreWorkspaceReadVersion, CoreWorkspaceTreeReadVersion, CoreDecisionReadVersion}},
		},
		// live_guards is implemented over the facts a Run already holds. It is
		// no longer unsupported, and saying otherwise while the operator works
		// would be the manifest lying about its own build.
		Unsupported: []string{"artifact_checks", "subscriptions", "unattended", "external_write", "destructive", "managed_isolation", "automatic_retry", "telemetry.compare", "provider_usage", "profiling"},
	}
	profile := &manifest.Profiles[1]
	profile.StateVersion, profile.ReadVersion = CoreNeutralStateVersion, CoreNeutralReadVersion
	profile.StateVersions = append(profile.StateVersions, CoreNeutralStateVersion)
	profile.ReadVersions = append(profile.ReadVersions, CoreNeutralReadVersion)
	profile.Capabilities = append(profile.Capabilities, "neutral_start", "execution_bindings")
	return manifest
}

func supportedRun(r Run) bool {
	if decisionInvariant(r) != nil {
		return false
	}
	if forkProvenanceInvariant(r) != nil {
		return false
	}
	if actionIntentInvariant(r) != nil {
		return false
	}
	if artifactPublicationInvariant(r) != nil {
		return false
	}
	if artifactClosureInvariant(r) != nil {
		return false
	}
	if publicationSubscriptionInvariant(r) != nil {
		return false
	}
	if r.guardInvariant() != nil {
		return false
	}
	if !isContextState(r.SchemaVersion) && hasContextStateFields(r) {
		return false
	}
	if !isSessionState(r.SchemaVersion) && hasSessionStateFields(r) {
		return false
	}
	if !isReportedCostState(r.SchemaVersion) && hasReportedCostStateFields(r) {
		return false
	}
	if !isWorkspaceState(r.SchemaVersion) && hasWorkspaceStateFields(r) {
		return false
	}
	for _, attempt := range r.Attempts {
		if attempt == nil {
			continue
		}
		if validateReportedCosts(attempt.ReportedCosts) != nil {
			return false
		}
		if attempt.Session != nil {
			want := AssistedSessionVersion
			if isReportedCostState(r.SchemaVersion) {
				want = AssistedSessionCostVersion
			}
			if isDecisionState(r.SchemaVersion) {
				want = AssistedSessionDecisionVersion
			} else if isWorkspaceTreeState(r.SchemaVersion) {
				want = AssistedSessionTreeVersion
			} else if isWorkspaceState(r.SchemaVersion) {
				want = AssistedSessionWorkspaceVersion
			}
			if attempt.Session.SchemaVersion != want {
				return false
			}
			if (attempt.Session.SchemaVersion == AssistedSessionWorkspaceVersion || attempt.Session.SchemaVersion == AssistedSessionTreeVersion) && attempt.Session.ClaimID != "" && (attempt.Session.WorkspaceMode != "worktree" && attempt.Session.WorkspaceMode != "checkout") {
				return false
			}
		}
	}
	if !isWaiverState(r.SchemaVersion) && (len(r.Waivers) != 0 || r.WaiverApplied) {
		return false
	}
	if !isRepeatState(r.SchemaVersion) {
		for _, a := range r.Activations {
			if a != nil && a.Repeat != nil {
				return false
			}
		}
		for _, inv := range r.Invocations {
			if inv != nil && inv.Iteration != nil {
				return false
			}
		}
	}
	if !isInvocationState(r.SchemaVersion) {
		for _, stop := range r.Stops {
			if stop.Scope != "" || stop.ScopeID != "" {
				return false
			}
		}
	}
	switch r.Profile {
	case flow.Profile:
		return r.SchemaVersion == StateVersion && r.EffectiveConfiguration == nil && r.Invocations == nil && r.WorkflowConfigurations == nil
	case flow.CoreProfile:
		c := r.EffectiveConfiguration
		if c == nil || c.SchemaVersion != "effective-configuration/1" || c.WorkflowRef != r.WorkflowRef || c.Inputs == nil {
			return false
		}
		if r.SchemaVersion == CoreStateVersion {
			return r.Invocations == nil && r.WorkflowConfigurations == nil
		}
		return isInvocationState(r.SchemaVersion) && r.Invocations != nil && r.Invocations[r.RootInvocationID] != nil && r.WorkflowConfigurations != nil && r.Ready == nil
	default:
		return false
	}
}

// A closure is visited once per exact definition, while execution budgets count
// every call site. Definition reuse must not expand an exponential plan tree.
func workflowPlans(root *flow.Plan) []*flow.Plan {
	result := []*flow.Plan{}
	seen := map[string]bool{}
	var visit func(*flow.Plan)
	visit = func(p *flow.Plan) {
		if seen[p.Digest] {
			return
		}
		seen[p.Digest] = true
		result = append(result, p)
		ids := make([]string, 0, len(p.Calls)+len(p.Repeats)+len(p.Maps))
		for id := range p.Calls {
			ids = append(ids, id)
		}
		for id := range p.Repeats {
			ids = append(ids, id)
		}
		// A map's body is one child definition serving every item. Leaving it
		// out would hide its resources, budgets and required state version.
		for id := range p.Maps {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			if body := p.BodyPlan(id); body != nil {
				visit(body)
			} else {
				visit(p.Maps[id])
			}
		}
		// A branch is an ordinary child definition. Leaving it out of the
		// closure would hide its resources, budgets and required state version.
		stages := make([]string, 0, len(p.Branches))
		for stage := range p.Branches {
			stages = append(stages, stage)
		}
		sort.Strings(stages)
		for _, stage := range stages {
			branches := make([]string, 0, len(p.Branches[stage]))
			for branch := range p.Branches[stage] {
				branches = append(branches, branch)
			}
			sort.Strings(branches)
			for _, branch := range branches {
				visit(p.Branches[stage][branch])
			}
		}
	}
	visit(root)
	return result
}

func planRef(p *flow.Plan) flow.Ref {
	return flow.Ref{ID: p.Workflow.ID, Version: p.Workflow.Version, Digest: p.Digest}
}

func requiresInvocationState(p *flow.Plan) bool {
	// A wait stops its own invocation rather than the Run, so it needs the
	// scoped frontier even when it is the only operator in the workflow.
	return requiresContextState(p) || requiresArtifactPublicationState(p) || len(p.Calls) != 0 || len(p.Repeats) != 0 || len(p.Branches) != 0 || len(p.Maps) != 0 || requiresWaitState(p) || slices.Contains(p.Workflow.AllowedOutcomes, "partial")
}

// CompileCore is the explicit context/check semantics opt-in. The preceding
// CompileProfile contract always leaves this selected resource map nil.
func requiresContextState(p *flow.Plan) bool { return p.Resources != nil || len(p.Checks) != 0 }

// A parallel stage anywhere in the closure needs the branch fan-out state, not
// only when the root declares it: a branch may itself contain one.
func requiresParallelState(p *flow.Plan) bool {
	for _, child := range workflowPlans(p) {
		if len(child.Branches) != 0 {
			return true
		}
	}
	return false
}

// A wait anywhere in the closure needs the registration state: an early
// callback has to have somewhere durable to land, wherever the wait sits.
func requiresWaitState(p *flow.Plan) bool {
	for _, child := range workflowPlans(p) {
		for _, stage := range child.Workflow.Definition.Stages {
			if stage.Kind == "wait" {
				return true
			}
		}
	}
	return false
}

// A map anywhere in the closure needs the sealed-collection state, for the same
// reason: the body of one may itself contain another.
func requiresMapState(p *flow.Plan) bool {
	for _, child := range workflowPlans(p) {
		if len(child.Maps) != 0 {
			return true
		}
	}
	return false
}

func requiresRepeatState(p *flow.Plan) bool {
	for _, child := range workflowPlans(p) {
		if len(child.Repeats) != 0 {
			return true
		}
	}
	return false
}

// Artifact hooks live in StepDefinition v3 anywhere in the pinned closure.
// Their publications need the latest scoped state even in a flat workflow.
func requiresArtifactPublicationState(p *flow.Plan) bool {
	for _, child := range workflowPlans(p) {
		for _, step := range child.Steps {
			for _, hook := range step.Hooks {
				if hook.Kind == "artifact" {
					return true
				}
			}
		}
	}
	return false
}

func requiresArtifactClosureState(p *flow.Plan) bool {
	for _, child := range workflowPlans(p) {
		for _, step := range child.Steps {
			for _, hook := range step.Hooks {
				if hook.Kind == "artifact" && hook.Artifact != nil && hook.Artifact.Cardinality == "keyed_many" {
					return true
				}
			}
		}
	}
	return false
}

func requiresPublicationSubscriptionState(p *flow.Plan) bool {
	for _, child := range workflowPlans(p) {
		for _, stage := range child.Workflow.Definition.Stages {
			if source, ok := child.PublicationSource(stage.SourceRef); ok && source.Mode == "each_publication" {
				return true
			}
		}
	}
	return false
}

func requiresPublicationChecksState(p *flow.Plan) bool {
	for _, child := range workflowPlans(p) {
		for _, step := range child.Steps {
			for _, hook := range step.Hooks {
				if hook.Kind == "artifact" && hook.Artifact != nil && len(hook.Artifact.ContentCheckRefs) != 0 {
					return true
				}
			}
		}
	}
	return false
}

func requiresPublicationNewOnlyState(p *flow.Plan) bool {
	for _, child := range workflowPlans(p) {
		for _, stage := range child.Workflow.Definition.Stages {
			if source, ok := child.PublicationSource(stage.SourceRef); ok && source.Initial == "new_only" {
				return true
			}
		}
	}
	return false
}

func requiresPublicationFailureState(p *flow.Plan) bool {
	for _, child := range workflowPlans(p) {
		for _, stage := range child.Workflow.Definition.Stages {
			if source, ok := child.PublicationSource(stage.SourceRef); ok && source.ProducerFailure == "interrupt_on_terminal_failure" {
				return true
			}
		}
	}
	return false
}

func (e *Engine) workflowConfigurations(p *flow.Plan, root *EffectiveConfiguration) (map[string]*EffectiveConfiguration, error) {
	if !requiresInvocationState(p) {
		return nil, nil
	}
	values := map[string]*EffectiveConfiguration{p.Digest: root}
	for _, child := range workflowPlans(p)[1:] {
		c, err := e.effectiveConfiguration(child, nil, nil, false)
		if err != nil {
			return nil, err
		}
		values[child.Digest] = c
	}
	for _, parent := range workflowPlans(p) {
		configuration := values[parent.Digest]
		for _, id := range parent.Sequence {
			if stage := parent.Workflow.Definition.Stages[id]; stage.Kind == "repeat" {
				if _, err := repeatLimit(parent, configuration, id); err != nil {
					return nil, err
				}
			}
			child := parent.BodyPlan(id)
			if child == nil {
				continue
			}
			stage := parent.Workflow.Definition.Stages[id]
			bindings := []map[string]flow.Binding{stage.InputBindings}
			if stage.Kind == "repeat" {
				bindings[0] = stage.InitialBindings
				if stage.MaxIterations > 1 {
					bindings = append(bindings, stage.NextBindings)
				}
			}
			for _, inputs := range bindings {
				for name, port := range child.Workflow.Inputs {
					_, bound := inputs[name]
					if port.Required && port.Configuration != nil && !bound && len(values[child.Digest].Inputs[name].Value) == 0 {
						return nil, faultf("missing_required_configuration", "%s %s %s input %s", parent.Workflow.ID, stage.Kind, id, name)
					}
				}
			}
		}
	}
	return values, nil
}

// repeatLimit reads only a value already pinned with the WorkflowRevision's
// configuration. It deliberately accepts no run override: that would make a
// reviewed project limit change at dispatch time instead of at Start.
func repeatLimit(p *flow.Plan, configuration *EffectiveConfiguration, stageID string) (int64, error) {
	stage, exists := p.Workflow.Definition.Stages[stageID]
	if !exists || stage.Kind != "repeat" || configuration == nil || configuration.WorkflowRef != planRef(p) {
		return 0, local.ErrIntegrity
	}
	if stage.LimitConfiguration == "" {
		return stage.MaxIterations, nil
	}
	value, exists := configuration.Inputs[stage.LimitConfiguration]
	if !exists || len(value.Value) == 0 {
		return 0, faultf("missing_repeat_limit_configuration", "%s", stage.LimitConfiguration)
	}
	decoder := json.NewDecoder(bytes.NewReader(value.Value))
	decoder.UseNumber()
	var number json.Number
	if err := decoder.Decode(&number); err != nil || decoder.More() {
		return 0, faultf("invalid_repeat_limit_configuration", "%s must be an integer", stage.LimitConfiguration)
	}
	limit, err := number.Int64()
	if err != nil || limit < 1 || limit > stage.MaxIterations {
		return 0, faultf("invalid_repeat_limit_configuration", "%s must be between 1 and %d", stage.LimitConfiguration, stage.MaxIterations)
	}
	return limit, nil
}

func executorKey(r Run, ref flow.Ref, definitionID string) string {
	if isInvocationState(r.SchemaVersion) {
		return ref.String()
	}
	return definitionID
}

type ConfigurationValue struct {
	Source string          `json:"source"`
	Value  json.RawMessage `json:"value,omitempty"`
}

// EffectiveConfiguration is pinned with the Run's lock. Values are ordinary
// declared JSON inputs, never executor arguments, credentials or permissions.
type EffectiveConfiguration struct {
	SchemaVersion string                        `json:"schema_version"`
	WorkflowRef   flow.Ref                      `json:"workflow_ref"`
	Inputs        map[string]ConfigurationValue `json:"inputs"`
}

// ProjectionManifest records the exact transformation beside the source bytes.
// It is an ordinary immutable JSON artifact, not executable converter code.
type ProjectionManifest struct {
	SchemaVersion      string      `json:"schema_version"`
	Source             ArtifactRef `json:"source_ref"`
	Pointer            string      `json:"pointer"`
	ProjectedSchemaRef flow.Ref    `json:"projected_schema_ref"`
	WorkflowRef        flow.Ref    `json:"workflow_ref"`
}

func (e *Engine) effectiveConfiguration(p *flow.Plan, files map[string]string, refs map[string]ArtifactRef, requireInputs bool) (*EffectiveConfiguration, error) {
	return e.effectiveConfigurationWithValues(p, files, refs, nil, requireInputs)
}

func (e *Engine) effectiveConfigurationWithValues(p *flow.Plan, files map[string]string, refs map[string]ArtifactRef, values map[string]json.RawMessage, requireInputs bool) (*EffectiveConfiguration, error) {
	if p.Profile == flow.Profile {
		if len(e.Config.Configuration.InputValues) != 0 {
			return nil, fault("unsupported_configuration", "input values require core-workflow/1")
		}
		return nil, nil
	}
	c := &EffectiveConfiguration{SchemaVersion: "effective-configuration/1", WorkflowRef: flow.Ref{ID: p.Workflow.ID, Version: p.Workflow.Version, Digest: p.Digest}, Inputs: map[string]ConfigurationValue{}}
	project := e.Config.Configuration.InputValues[p.Workflow.ID]
	for name := range project {
		port, ok := p.Workflow.Inputs[name]
		if !ok || port.Configuration == nil {
			return nil, faultf("unknown_configuration_input", "%s", name)
		}
	}
	for name, port := range p.Workflow.Inputs {
		declaration := port.Configuration
		if declaration == nil {
			continue
		}
		value := ConfigurationValue{Source: "absent"}
		if len(declaration.Default) != 0 {
			value = ConfigurationValue{Source: "package_default", Value: declaration.Default}
		}
		if configured, ok := project[name]; ok {
			value = ConfigurationValue{Source: "project", Value: configured}
		}
		file, hasFile := files[name]
		ref, hasRef := refs[name]
		inline, hasValue := values[name]
		if hasFile || hasRef || hasValue {
			if declaration.Scope != "run" {
				return nil, faultf("configuration_scope", "run override is forbidden for %s", name)
			}
			if hasFile && hasRef {
				return nil, errors.New("input file and reference cannot both bind the same port")
			}
			if hasValue && (hasFile || hasRef) {
				return nil, errors.New("input value, file and reference cannot bind the same port")
			}
			var err error
			value.Source = "run"
			if hasValue {
				value.Value = inline
				if len(inline) == 0 {
					return nil, fault("invalid_json", "configuration input requires a JSON value")
				}
			} else if hasFile {
				value.Value, err = e.inputBytes(file)
			} else {
				var artifact Artifact
				artifact, value.Value, err = e.Artifact(ref)
				if err == nil {
					err = e.validatePortArtifact(p, port.Port, artifact, value.Value)
				}
			}
			if err != nil {
				return nil, err
			}
		}
		if len(value.Value) == 0 {
			if requireInputs && port.Required {
				return nil, faultf("missing_required_configuration", "%s", name)
			}
		} else {
			if port.Format != "json" || port.SchemaRef == nil {
				return nil, errors.New("configuration requires an explicitly typed JSON input")
			}
			if port.SchemaRef.ID == sourceSnapshotSchemaID && !hasRef {
				return nil, local.Reject("source_snapshot_invalid", "source configuration requires an acquired snapshot reference, not a literal or file descriptor")
			}
			if err := p.ValidateJSON(*port.SchemaRef, value.Value); err != nil {
				return nil, err
			}
			data, err := flow.Canonical(value.Value)
			if err != nil {
				return nil, err
			}
			value.Value = data
		}
		c.Inputs[name] = value
	}
	return c, nil
}
