package flow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// Plan is a validated definition snapshot, not an execution admission.
// Sequence is the F1 pass chain or the CoreProfile's deterministic topological
// order. A topological order is not a prediction of the path a Run will take.
type Plan struct {
	Profile  string
	Workflow WorkflowRevision
	Steps    map[string]StepDefinition
	// Calls is keyed by the caller's local stage ID. Cached definitions are
	// shared, while each runtime call creates a distinct WorkflowInvocation.
	Calls   map[string]*Plan
	Repeats map[string]*Plan
	// Maps is keyed by the map stage ID. Every item runs the same pinned body,
	// so one definition serves the whole fan-out however many items it seals.
	Maps map[string]*Plan
	// Branches is keyed by the parallel stage ID and then by branch ID. A
	// branch is an ordinary child workflow, so it shares the same cycle,
	// budget and pinning rules as a call.
	Branches  map[string]map[string]*Plan
	Sequence  []string
	Canonical []byte
	Digest    string
	Registry  Registry
	// Resources contains the selected typed context closure for CompileCore.
	// Legacy entrypoints leave it nil. Bytes never alias the supplied inventory.
	Resources ContextResources
	// Checks is the selected exact automatic-check closure. It shares parsed
	// definitions across calls/repeats, not CheckExecutions or admission costs.
	// Like Resources, only CompileCore makes this map non-nil.
	Checks             map[Ref]CheckDefinition
	schemas            map[Ref][]byte
	schemaValues       map[Ref]any
	projectionSources  map[Ref]bool
	dependencyBytes    int
	compilation        *compilation
	conditionChecked   map[Ref]bool
	publicationSources map[Ref]PublicationSourceDefinition
	bounds             executionBounds
}

// Compile validates every declared stage, including unreachable ones. Policy
// qualification, actual input bytes, executable trust and authority are checked
// by runtime before admission; successful compilation grants no permission.
func Compile(data []byte, format string, registry Registry) (*Plan, error) {
	return CompileProfile(data, format, registry, Profile)
}

// CompileProfile never infers execution semantics from newer definition bytes.
// The caller explicitly selects a supported profile and pins it in the Run.
func CompileProfile(data []byte, format string, registry Registry, profile string) (*Plan, error) {
	if profile != Profile && profile != CoreProfile {
		return nil, problem("unsupported_profile", "", "execution semantics profile is not supported")
	}
	var shared *compilation
	if profile == CoreProfile {
		shared = newCompilation()
	}
	return compileWorkflow(data, format, registry, profile, shared)
}

func compileWorkflow(data []byte, format string, registry Registry, profile string, shared *compilation) (*Plan, error) {
	value, _, err := workflowValue(data, format)
	if err != nil {
		return nil, err
	}
	contract := "WorkflowRevision"
	if object, ok := value.(map[string]any); ok && profile == CoreProfile {
		switch object["schema_version"] {
		case "2":
			contract = "WorkflowRevisionV2"
		case "3":
			contract = "WorkflowRevisionV3"
		}
	}
	if err := validateProtocolValue(contract, value, ""); err != nil {
		return nil, err
	}
	if err := supportedWorkflowProfile(value.(map[string]any), profile); err != nil {
		return nil, err
	}
	plan := &Plan{Profile: profile, Steps: make(map[string]StepDefinition), Registry: make(Registry), schemas: make(map[Ref][]byte), schemaValues: make(map[Ref]any), conditionChecked: make(map[Ref]bool), publicationSources: make(map[Ref]PublicationSourceDefinition), compilation: shared}
	if shared != nil {
		plan.Branches = make(map[string]map[string]*Plan)
		plan.Calls = make(map[string]*Plan)
		plan.Repeats = make(map[string]*Plan)
		plan.Maps = make(map[string]*Plan)
		plan.Registry, plan.schemas, plan.schemaValues = shared.pinned, shared.schemas, shared.schemaValues
		plan.Resources = shared.resources
		if plan.Resources != nil {
			if shared.checks == nil {
				shared.checks = make(map[Ref]CheckDefinition)
			}
			plan.Checks = shared.checks
		}
		plan.conditionChecked = shared.conditionChecked
	}
	if err := decodeValue(value, &plan.Workflow); err != nil {
		return nil, err
	}
	jsonBytes, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	plan.Canonical, err = Canonical(jsonBytes)
	if err != nil {
		return nil, err
	}
	plan.Digest, err = Digest(plan.Canonical)
	if err != nil {
		return nil, err
	}
	if shared != nil {
		ref := Ref{ID: plan.Workflow.ID, Version: plan.Workflow.Version, Digest: plan.Digest}
		if plan.isContextResource(ref) {
			return nil, problem("resource_type_mismatch", "", "context resource cannot be a workflow definition")
		}
		if err := shared.enter(ref); err != nil {
			return nil, err
		}
		defer delete(shared.active, ref)
		defer delete(shared.activeIdentities, definitionIdentity{ref.ID, ref.Version})
		shared.plans[ref] = plan
	}
	if err := plan.pinRefs(value, registry, "", make(map[Ref]bool)); err != nil {
		return nil, err
	}
	for _, name := range keys(plan.Workflow.Inputs) {
		input := plan.Workflow.Inputs[name]
		path := "/inputs/" + escapePointer(name)
		if err := plan.checkPort(input.Port, path); err != nil {
			return nil, err
		}
		if input.Configuration != nil {
			if input.Format != "json" || input.SchemaRef == nil {
				return nil, problem("unsupported", path+"/configuration", "configuration values require a JSON input port")
			}
			if len(input.Configuration.Default) != 0 {
				if err := plan.ValidateJSON(*input.SchemaRef, input.Configuration.Default); err != nil {
					return nil, problem("invalid_default", path+"/configuration/default", err.Error())
				}
			}
		}
	}
	for _, name := range keys(plan.Workflow.Outputs) {
		output := plan.Workflow.Outputs[name]
		path := "/outputs/" + escapePointer(name)
		if err := plan.checkPort(output.Port, path); err != nil {
			return nil, err
		}
		for _, outcome := range output.RequiredFor {
			if !slices.Contains(plan.Workflow.AllowedOutcomes, outcome) {
				return nil, problem("invalid_outcome", path+"/required_for", "output requires an outcome not allowed by workflow")
			}
		}
	}
	stepCache := make(map[Ref]StepDefinition)
	if shared != nil {
		stepCache = shared.steps
	}
	for _, id := range keys(plan.Workflow.Definition.Stages) {
		stage := plan.Workflow.Definition.Stages[id]
		if stage.Kind == "parallel" {
			branches := map[string]*Plan{}
			for i, branch := range stage.ParallelBranches {
				child, err := shared.child(plan, branch.WorkflowRef, fmt.Sprintf("/definition/stages/%s/branches/%d/workflow_ref", escapePointer(id), i))
				if err != nil {
					return nil, err
				}
				branches[branch.ID] = child
			}
			plan.Branches[id] = branches
			continue
		}
		if stage.Kind == "call" || stage.Kind == "repeat" || stage.Kind == "map" {
			ref, field := stage.WorkflowRef, "workflow_ref"
			if stage.Kind == "repeat" || stage.Kind == "map" {
				ref, field = stage.BodyWorkflowRef, "body_workflow_ref"
			}
			child, err := shared.child(plan, ref, "/definition/stages/"+escapePointer(id)+"/"+field)
			if err != nil {
				return nil, err
			}
			switch stage.Kind {
			case "call":
				plan.Calls[id] = child
			case "repeat":
				plan.Repeats[id] = child
			default:
				plan.Maps[id] = child
			}
			continue
		}
		if stage.Kind != "step" {
			continue
		}
		path := "/definition/stages/" + escapePointer(id) + "/step_ref"
		if step, exists := stepCache[stage.StepRef]; exists {
			plan.Steps[id] = step
			continue
		}
		step, err := plan.loadStep(stage.StepRef, path)
		if err != nil {
			return nil, err
		}
		plan.Steps[id] = step
		stepCache[stage.StepRef] = step
	}
	checkGraph := plan.checkGraph
	if profile == CoreProfile {
		checkGraph = plan.checkCoreGraph
	}
	if err := checkGraph(); err != nil {
		return nil, err
	}
	if shared != nil && len(shared.active) == 1 {
		if err := plan.checkPublicationCompositions(); err != nil {
			return nil, err
		}
	}
	return plan, nil
}

func supportedWorkflowProfile(workflow map[string]any, profile string) error {
	stages := workflow["definition"].(map[string]any)["stages"].(map[string]any)
	workflowVersion, _ := workflow["schema_version"].(string)
	for _, id := range keys(stages) {
		stage := stages[id].(map[string]any)
		path := "/definition/stages/" + escapePointer(id)
		if stage["kind"] != "step" && stage["kind"] != "finish" && !(profile == CoreProfile && (stage["kind"] == "choice" || stage["kind"] == "call" || stage["kind"] == "repeat" || stage["kind"] == "parallel" || stage["kind"] == "map" || stage["kind"] == "wait")) {
			return problem("unsupported", path+"/kind", fmt.Sprintf("%s does not support %v", profile, stage["kind"]))
		}
		for _, field := range []string{"compensation", "on_error"} {
			if field == "on_error" && profile == CoreProfile {
				continue
			}
			if _, exists := stage[field]; exists {
				return problem("unsupported", path+"/"+field, profile+" does not support "+field)
			}
		}
		if on, ok := stage["on"].(map[string]any); ok {
			for _, verdict := range keys(on) {
				if stage["kind"] == "parallel" {
					if !slices.Contains([]string{"satisfied", "unsatisfied"}, verdict) {
						return problem("unsupported", path+"/on/"+verdict, "a parallel stage routes on its join result, not on an outcome")
					}
					continue
				}
				if stage["kind"] == "map" {
					// A collection that turned out to be empty is not a join
					// result, so it gets a route of its own beside the two.
					if !slices.Contains([]string{"satisfied", "unsatisfied", "empty"}, verdict) {
						return problem("unsupported", path+"/on/"+verdict, "a map stage routes on its join result or on an empty collection")
					}
					continue
				}
				if stage["kind"] == "call" {
					if !slices.Contains([]string{"succeeded", "rejected", "no_work", "partial", "completed_with_waivers"}, verdict) {
						return problem("unsupported", path+"/on/"+verdict, "child outcome is not supported by "+profile)
					}
					continue
				}
				if !slices.Contains([]string{"pass", "fail", "needs_revision", "no_work"}, verdict) {
					return problem("unsupported", path+"/on/"+verdict, "verdict route is not supported by "+profile)
				}
			}
		}
		for _, field := range []string{"input_bindings", "output_bindings", "initial_bindings", "next_bindings"} {
			bindings, _ := stage[field].(map[string]any)
			for _, port := range keys(bindings) {
				binding := bindings[port].(map[string]any)
				bindingPath := path + "/" + field + "/" + escapePointer(port)
				streamBinding := profile == CoreProfile && workflowVersion == "3" && (binding["from"] == "publication" && stage["kind"] == "call" && field == "input_bindings" || binding["from"] == "subscription" && stage["kind"] == "repeat" && (field == "initial_bindings" || field == "next_bindings"))
				if !slices.Contains([]any{"workflow_input", "stage_output", "literal"}, binding["from"]) && !(profile == CoreProfile && stage["kind"] == "repeat" && field == "next_bindings" && binding["from"] == "iteration_output") && !streamBinding {
					return problem("unsupported", bindingPath+"/from", "binding source is not supported by "+profile)
				}
				for _, projection := range []string{"pointer", "projected_schema_ref"} {
					if _, exists := binding[projection]; exists && profile == Profile {
						return problem("unsupported", bindingPath+"/"+projection, "only whole-port bindings are supported")
					}
				}
			}
		}
	}
	limits := workflow["limits"].(map[string]any)
	if n, _ := limits["max_parallelism"].(json.Number).Float64(); n != 1 {
		// The foundation profile advances one stage at a time by definition.
		// Core admits concurrent attempts up to what this build is qualified
		// for; beyond that a definition would claim simultaneity nobody tested.
		if profile != CoreProfile {
			return problem("unsupported", "/limits/max_parallelism", profile+" permits one active Attempt")
		}
		if n < 1 || n > MaxQualifiedParallelism {
			return problem("unsupported", "/limits/max_parallelism", fmt.Sprintf("%s is qualified for 1 to %d active Attempts", profile, MaxQualifiedParallelism))
		}
	}
	if n, _ := limits["max_child_depth"].(json.Number).Float64(); n != 0 && profile == Profile {
		return problem("unsupported", "/limits/max_child_depth", "child invocations are not supported")
	}
	for _, outcome := range workflow["allowed_outcomes"].([]any) {
		// completed_with_waivers is not a second kind of success: it is the
		// declared success reported with its quality reduction still visible.
		// The published state already carries it on any invocation, so a
		// workflow that may rest on a waived check must be able to declare it.
		core := profile == CoreProfile && (outcome == "partial" || outcome == "completed_with_waivers")
		if !slices.Contains([]any{"succeeded", "rejected", "no_work"}, outcome) && !core {
			return problem("unsupported", "/allowed_outcomes", "outcome is not supported by "+profile)
		}
	}
	return nil
}

// pinRefs follows exact JSON dependency references, not arbitrary schema URLs.
// Repeated refs are cached and cycles terminate. Nothing reads ambient files.
func (p *Plan) pinRefs(value any, registry Registry, path string, active map[Ref]bool) error {
	return p.pinValueRefs(value, registry, path, active, definitionReferences)
}

type referenceContext uint8

const (
	definitionReferences referenceContext = iota
	schemaReference
	inputConfiguration
	workflowReference
	stepReference
	stepDefinitionReferences
	contextResourceReference
	checkReference
)

type referenceUse struct {
	ref     Ref
	context referenceContext
}

func (p *Plan) pinValueRefs(value any, registry Registry, path string, active map[Ref]bool, context referenceContext) error {
	switch value := value.(type) {
	case map[string]any:
		_, isID := value["id"].(string)
		_, isVersion := value["version"].(string)
		_, isDigest := value["digest"].(string)
		if len(value) == 3 && isID && isVersion && isDigest {
			if err := validateProtocolValue("ImmutableRef", value, path); err != nil {
				return err
			}
			var ref Ref
			if err := decodeValue(value, &ref); err != nil {
				return err
			}
			if p.compilation != nil {
				if err := p.compilation.identify(ref, path); err != nil {
					return err
				}
			}
			if p.Resources != nil {
				if context == contextResourceReference {
					return p.pinContextResource(ref, registry, path, active)
				}
				if p.isContextResource(ref) {
					return problem("resource_type_mismatch", path, "context resource cannot be used as a definition")
				}
			}
			if context == workflowReference && p.Profile == CoreProfile && !p.conditionChecked[ref] {
				if data, exists := registry[ref]; exists {
					if err := ValidateWorkflowConditions(data, "json"); err != nil {
						return err
					}
					p.conditionChecked[ref] = true
				}
			}
			canonical, pinned := p.Registry[ref]
			var expandedUses map[referenceUse]bool
			if p.compilation != nil {
				expandedUses = p.compilation.expandedUses
			}
			use := referenceUse{ref, context}
			// CompileCore pins bytes once but expands each actual reference
			// role: a schema use cannot suppress later Step/check dependencies.
			// A nil role cache preserves the delivered legacy entrypoints.
			if active[ref] || pinned && (expandedUses == nil || expandedUses[use]) {
				return nil
			}
			if !pinned && len(p.Registry)+len(p.Resources) >= 1024 || len(active) >= MaxDepth {
				return problem("dependency_limit", path, "dependency closure exceeds 1024 documents or depth 64")
			}
			if !pinned {
				data, exists := registry[ref]
				if !exists {
					return problem("missing_ref", path, "exact dependency is not supplied: "+ref.String())
				}
				var err error
				canonical, err = Canonical(data)
				if err != nil {
					return problem("invalid_dependency", path, err.Error())
				}
				digest, err := Digest(canonical)
				if err != nil || digest != ref.Digest {
					return problem("digest_mismatch", path, "dependency bytes do not match exact reference")
				}
			}
			parsed, err := Parse(canonical, "json")
			if err != nil {
				return err
			}
			if object, ok := parsed.(map[string]any); ok {
				if id, exists := object["id"]; exists && id != ref.ID {
					return problem("ref_identity_mismatch", path, "dependency id differs from exact reference")
				}
				if version, exists := object["version"]; exists && version != ref.Version {
					return problem("ref_identity_mismatch", path, "dependency version differs from exact reference")
				}
			}
			if context == checkReference && p.Checks != nil {
				// Reject recursive checks or other undeclared fields before
				// following any purported dependencies in their payload.
				definition, err := ParseCheckDefinition(canonical)
				if err != nil {
					return checkDefinitionAt(err, path)
				}
				p.Checks[ref] = definition
			}
			active[ref] = true
			if !pinned {
				dependencyBytes := &p.dependencyBytes
				if p.compilation != nil {
					dependencyBytes = &p.compilation.dependencyBytes
				}
				if *dependencyBytes+len(canonical) > 64<<20 {
					return problem("dependency_limit", path, "dependency closure exceeds 64 MiB")
				}
				*dependencyBytes += len(canonical)
				p.Registry[ref] = canonical
			}
			// JSON Schema is its own namespace: properties, const/default/enum
			// values and examples are not Pri-Fly dependency references. Schema
			// $refs are validated separately with external loading disabled.
			if context != schemaReference {
				childContext := definitionReferences
				if p.Resources != nil && context == stepReference {
					childContext = stepDefinitionReferences
				}
				if err := p.pinValueRefs(parsed, registry, path+"@"+ref.ID, active, childContext); err != nil {
					return err
				}
			}
			delete(active, ref)
			if expandedUses != nil {
				expandedUses[use] = true
			}
			return nil
		}
		for _, key := range keys(value) {
			// Only the actual literal/configuration value is instance data.
			// A stage or port named value/default is still a definition node.
			if key == "value" && value["from"] == "literal" || key == "default" && context == inputConfiguration {
				continue
			}
			childContext := definitionReferences
			if p.Checks != nil && (key == "content_check_refs" && (value["format"] == "json" || value["format"] == "blob") || key == "result_check_refs" && context == stepDefinitionReferences) {
				childContext = checkReference
			} else if p.Resources != nil && context == stepDefinitionReferences && (key == "instructions_ref" || key == "context_refs") {
				childContext = contextResourceReference
			} else if p.Resources != nil && key == "step_ref" && value["kind"] == "step" {
				childContext = stepReference
			} else if key == "schema_ref" || strings.HasSuffix(key, "_schema_ref") {
				childContext = schemaReference
			} else if key == "workflow_ref" && value["kind"] == "call" || key == "body_workflow_ref" && value["kind"] == "repeat" {
				childContext = workflowReference
			} else if key == "configuration" && (value["format"] == "json" || value["format"] == "blob") {
				if _, isInput := value["required"].(bool); isInput {
					childContext = inputConfiguration
				}
			}
			if err := p.pinValueRefs(value[key], registry, path+"/"+escapePointer(key), active, childContext); err != nil {
				return err
			}
		}
	case []any:
		for i, child := range value {
			childContext := definitionReferences
			if context == contextResourceReference || context == checkReference {
				childContext = context
			}
			if err := p.pinValueRefs(child, registry, fmt.Sprintf("%s/%d", path, i), active, childContext); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *Plan) loadStep(ref Ref, path string) (StepDefinition, error) {
	var step StepDefinition
	data := p.Registry[ref]
	value, err := Parse(data, "json")
	if err != nil {
		return step, err
	}
	name := "StepDefinition"
	if object, ok := value.(map[string]any); ok {
		switch object["schema_version"] {
		case "2":
			name = "StepDefinitionV2"
		case "3":
			if p.Profile != CoreProfile {
				return step, problem("unsupported", path+"/schema_version", "artifact hooks require core-workflow/1")
			}
			name = "StepDefinitionV3"
		case "4":
			if p.Profile != CoreProfile {
				return step, problem("unsupported", path+"/schema_version", "artifact subscriptions require core-workflow/1")
			}
			name = "StepDefinitionV4"
		case "5":
			if p.Profile != CoreProfile {
				return step, problem("unsupported", path+"/schema_version", "workspace trees require core-workflow/1")
			}
			name = "StepDefinitionV5"
		}
	}
	if err := validateProtocolValue(name, value, path); err != nil {
		return step, err
	}
	if err := decodeValue(value, &step); err != nil {
		return step, err
	}
	if len(step.ResultCheckRefs) != 0 {
		if p.Checks == nil {
			return step, problem("unsupported", path+"/result_check_refs", "implement domain checks as ordinary steps")
		}
		if err := p.loadChecks(step.ResultCheckRefs, "result", path+"/result_check_refs"); err != nil {
			return step, err
		}
	}
	if len(step.ContextRefs) != 0 && p.Resources == nil {
		return step, problem("unsupported", path+"/context_refs", "F1 accepts explicit inputs; context composition belongs to F2")
	}
	if step.Effects.Class != "none" && step.Effects.Class != "workspace_write" {
		return step, problem("unsupported", path+"/effects/class", "external writes and destructive steps are outside F1 qualification")
	}
	for _, name := range keys(step.Inputs) {
		if err := p.checkPort(step.Inputs[name].Port, path+"/inputs/"+escapePointer(name)); err != nil {
			return step, err
		}
	}
	for _, name := range keys(step.Outputs) {
		output := step.Outputs[name]
		if err := p.checkPort(output.Port, path+"/outputs/"+escapePointer(name)); err != nil {
			return step, err
		}
		for _, verdict := range output.RequiredFor {
			if verdict == "skipped" || verdict == "waived" {
				return step, problem("unsupported", path+"/outputs/"+escapePointer(name)+"/required_for", "skip and waiver are not supported")
			}
		}
	}
	if _, err := p.schema(step.ResultSchemaRef); err != nil {
		return step, err
	}
	if err := p.checkHooks(step, path); err != nil {
		return step, err
	}
	if err := p.checkWorkspaceTrees(step, path); err != nil {
		return step, err
	}
	return step, nil
}

func (p *Plan) checkWorkspaceTrees(step StepDefinition, path string) error {
	if len(step.WorkspaceTrees) == 0 {
		return nil
	}
	if step.SchemaVersion != "5" || step.Effects.Class != "workspace_write" {
		return problem("invalid_workspace_tree", path+"/workspace_trees", "workspace trees require StepDefinition v5 and workspace_write")
	}
	seenPaths, seenOutputs := map[string]bool{}, map[string]bool{}
	for index, binding := range step.WorkspaceTrees {
		bindingPath := fmt.Sprintf("%s/workspace_trees/%d", path, index)
		if seenPaths[binding.Capture.Path] || seenOutputs[binding.OutputPort] {
			return problem("invalid_workspace_tree", bindingPath, "workspace tree paths and output ports must be unique")
		}
		seenPaths[binding.Capture.Path], seenOutputs[binding.OutputPort] = true, true
		output, exists := step.Outputs[binding.OutputPort]
		if !exists || output.Format != "json" || output.SchemaRef == nil || output.SchemaRef.ID != WorkspaceTreeManifestSchemaID {
			return problem("invalid_workspace_tree", bindingPath+"/output_port", "workspace tree output names one declared JSON manifest port")
		}
		if len(output.ContentCheckRefs) != 0 || len(step.ResultCheckRefs) != 0 {
			return problem("unsupported_workspace_tree_check", bindingPath, "workspace-tree results cannot use deferred acceptance checks")
		}
		if binding.InputPort == "" {
			continue
		}
		input, exists := step.Inputs[binding.InputPort]
		if !exists || input.Format != "json" || input.SchemaRef == nil || input.SchemaRef.ID != WorkspaceTreeManifestSchemaID || *input.SchemaRef != *output.SchemaRef {
			return problem("invalid_workspace_tree", bindingPath+"/input_port", "workspace tree input names a compatible declared JSON manifest port")
		}
	}
	return nil
}

func (p *Plan) checkPort(port Port, path string) error {
	if len(port.ContentCheckRefs) > 0 {
		if p.Checks == nil {
			return problem("unsupported", path+"/content_check_refs", "content check plugins are not supported; use an ordinary check step")
		}
		if err := p.loadChecks(port.ContentCheckRefs, "content", path+"/content_check_refs"); err != nil {
			return err
		}
	}
	if port.SchemaRef != nil {
		if _, err := p.schema(*port.SchemaRef); err != nil {
			return problem("invalid_schema_ref", path+"/schema_ref", err.Error())
		}
	}
	return nil
}

func (p *Plan) schema(ref Ref) ([]byte, error) {
	if p.isContextResource(ref) {
		return nil, problem("resource_type_mismatch", "", "context resource cannot be used as a schema")
	}
	if schema := p.schemas[ref]; schema != nil {
		return schema, nil
	}
	data, exists := p.Registry[ref]
	if !exists {
		return nil, problem("missing_ref", "", "schema is not pinned: "+ref.String())
	}
	value, err := Parse(data, "json")
	if err != nil {
		return nil, err
	}
	if err := checkSchema(data, nil); err != nil {
		return nil, err
	}
	p.schemas[ref] = bytes.Clone(data)
	p.schemaValues[ref] = value
	return p.schemas[ref], nil
}

// ValidateJSON validates actual input/output/publication bytes against a schema
// pinned during compilation. It does not accept a mutable external schema URL.
func (p *Plan) ValidateJSON(ref Ref, data []byte) error {
	schema := p.schemas[ref]
	if schema == nil {
		return problem("missing_ref", "", "schema was not compiled: "+ref.String())
	}
	_, err := Parse(data, "json")
	if err != nil {
		return err
	}
	return checkSchema(schema, data)
}

func (p *Plan) checkGraph() error {
	w := &p.Workflow
	completed := make(map[string]string)
	reached := make(map[string]bool)
	passSeen := make(map[string]bool)
	current := w.Definition.Entry
	for {
		stage, exists := w.Definition.Stages[current]
		if !exists {
			return problem("missing_stage", "/definition", "transition target does not exist: "+current)
		}
		path := "/definition/stages/" + escapePointer(current)
		if passSeen[current] {
			return problem("cycle", path, "pass chain re-enters a stage")
		}
		reached[current] = true
		passSeen[current] = true
		p.Sequence = append(p.Sequence, current)
		if stage.Kind == "finish" {
			if err := p.checkFinish(stage, path, completed); err != nil {
				return err
			}
			break
		}
		step := p.Steps[current]
		if err := p.checkInputs(stage.InputBindings, step.Inputs, completed, path+"/input_bindings"); err != nil {
			return err
		}
		if _, exists := stage.On["pass"]; !exists {
			return problem("missing_handler", path+"/on/pass", "each F1 step requires a pass route")
		}
		for _, verdict := range []string{"fail", "needs_revision", "no_work"} {
			target, exists := stage.On[verdict]
			if !exists {
				continue // Unhandled results are accepted, then runtime reports routing error.
			}
			finish, exists := w.Definition.Stages[target]
			if !exists {
				return problem("missing_stage", path+"/on/"+verdict, "transition target does not exist")
			}
			if finish.Kind != "finish" {
				return problem("unsupported", path+"/on/"+verdict, "negative routes must lead directly to finish")
			}
			expected := "rejected"
			if verdict == "no_work" {
				expected = "no_work"
			}
			if finish.Outcome != expected {
				return problem("invalid_outcome", path+"/on/"+verdict, "verdict must terminate with "+expected)
			}
			completed[current] = verdict
			if err := p.checkFinish(finish, "/definition/stages/"+escapePointer(target), completed); err != nil {
				return err
			}
			reached[target] = true
		}
		completed[current] = "pass"
		current = stage.On["pass"]
	}
	for _, id := range keys(w.Definition.Stages) {
		if !reached[id] {
			return problem("unreachable_stage", "/definition/stages/"+escapePointer(id), "stage is outside the pass chain and its terminal routes")
		}
	}
	if int64(len(p.Sequence)-1) > w.Limits.MaxStepInstances {
		return problem("limit_exceeded", "/limits/max_step_instances", "limit cannot cover the pass chain")
	}
	if int64(len(p.Sequence)) > w.Limits.MaxControlTransitions {
		return problem("limit_exceeded", "/limits/max_control_transitions", "limit cannot cover stage activations on the pass chain")
	}
	return nil
}

func (p *Plan) checkInputs(bindings map[string]Binding, inputs map[string]InputPort, completed map[string]string, path string) error {
	return checkInputBindings(bindings, inputs, path, func(binding Binding, target Port, required bool, path string) error {
		return p.checkBinding(binding, target, required, completed, path)
	})
}

func checkInputBindings(bindings map[string]Binding, inputs map[string]InputPort, path string, check func(Binding, Port, bool, string) error) error {
	for _, name := range keys(inputs) {
		if _, exists := bindings[name]; inputs[name].Required && !exists {
			return problem("missing_binding", path+"/"+escapePointer(name), "required input has no binding")
		}
	}
	for _, name := range keys(bindings) {
		target, exists := inputs[name]
		if !exists {
			return problem("unknown_port", path+"/"+escapePointer(name), "input port is not declared")
		}
		if err := check(bindings[name], target.Port, target.Required, path+"/"+escapePointer(name)); err != nil {
			return err
		}
	}
	return nil
}

func (p *Plan) checkFinish(stage Stage, path string, completed map[string]string) error {
	return p.checkFinishBindings(stage, path, func(binding Binding, target Port, required bool, path string) error {
		return p.checkBinding(binding, target, required, completed, path)
	})
}

func (p *Plan) checkFinishBindings(stage Stage, path string, check func(Binding, Port, bool, string) error) error {
	if !slices.Contains(p.Workflow.AllowedOutcomes, stage.Outcome) {
		return problem("invalid_outcome", path+"/outcome", "finish outcome is not allowed by workflow")
	}
	for _, name := range keys(p.Workflow.Outputs) {
		port := p.Workflow.Outputs[name]
		if _, exists := stage.OutputBindings[name]; slices.Contains(port.RequiredFor, stage.Outcome) && !exists {
			return problem("missing_binding", path+"/output_bindings/"+escapePointer(name), "required workflow output has no export")
		}
	}
	for _, name := range keys(stage.OutputBindings) {
		target, exists := p.Workflow.Outputs[name]
		bindingPath := path + "/output_bindings/" + escapePointer(name)
		if !exists {
			return problem("unknown_port", bindingPath, "workflow output is not declared")
		}
		if err := check(stage.OutputBindings[name], target.Port, slices.Contains(target.RequiredFor, stage.Outcome), bindingPath); err != nil {
			return err
		}
	}
	return nil
}

func (p *Plan) checkBinding(binding Binding, target Port, required bool, completed map[string]string, path string) error {
	var source Port
	guaranteed := true
	switch binding.From {
	case "workflow_input":
		input, exists := p.Workflow.Inputs[binding.Port]
		if !exists {
			return problem("unknown_port", path+"/port", "workflow input does not exist")
		}
		source = input.Port
		guaranteed = input.Required
	case "stage_output":
		verdict, exists := completed[binding.StageID]
		if !exists {
			return problem("unavailable_output", path+"/stage_id", "producer has not executed on this path")
		}
		output, exists := p.Steps[binding.StageID].Outputs[binding.Port]
		if !exists {
			return problem("unknown_port", path+"/port", "producer output does not exist")
		}
		source = output.Port
		guaranteed = slices.Contains(output.RequiredFor, verdict)
	case "literal":
		var err error
		if source, err = p.literalPort(binding, path); err != nil {
			return err
		}
	default:
		return problem("unsupported", path+"/from", "unsupported binding source")
	}
	return p.checkBoundPort(binding, source, target, required, guaranteed, path)
}

func (p *Plan) literalPort(binding Binding, path string) (Port, error) {
	source := Port{Format: "json", SchemaRef: binding.SchemaRef}
	if binding.SchemaRef == nil {
		return source, problem("missing_ref", path+"/schema_ref", "literal requires a schema")
	}
	if _, err := p.schema(*binding.SchemaRef); err != nil {
		return source, err
	}
	if err := p.ValidateJSON(*binding.SchemaRef, binding.Value); err != nil {
		return source, problem("invalid_literal", path+"/value", err.Error())
	}
	return source, nil
}

func (p *Plan) checkBoundPort(binding Binding, source, target Port, required, guaranteed bool, path string) error {
	if required && !guaranteed {
		return problem("unavailable_output", path, "required value is not guaranteed on this path")
	}
	if binding.Pointer != nil {
		if p.Profile != CoreProfile {
			return problem("unsupported", path+"/pointer", "projection requires "+CoreProfile)
		}
		if source.Format != "json" || source.SchemaRef == nil || binding.ProjectedSchemaRef == nil {
			return problem("port_type_mismatch", path, "projection requires JSON source and explicit projected schema")
		}
		if _, err := pointerParts(*binding.Pointer); err != nil {
			return problem("invalid_pointer", path+"/pointer", err.Error())
		}
		if _, err := p.schema(*binding.ProjectedSchemaRef); err != nil {
			return problem("invalid_schema_ref", path+"/projected_schema_ref", err.Error())
		}
		if required {
			if p.projectionSources == nil {
				p.projectionSources = make(map[Ref]bool)
			}
			if !p.projectionSources[*source.SchemaRef] {
				if nestedResourceID(p.schemaValues[*source.SchemaRef], true) {
					return problem("unsupported_projection", path+"/pointer", "projection presence proof does not support nested schema resource identities")
				}
				p.projectionSources[*source.SchemaRef] = true
			}
			present, err := projectionGuaranteed(p.schemaValues[*source.SchemaRef], *binding.Pointer)
			if err != nil {
				return problem("unsupported_projection", path+"/pointer", err.Error())
			}
			if !present {
				return problem("unavailable_output", path+"/pointer", "required projection is not guaranteed by the source schema")
			}
		}
		source = Port{Format: "json", SchemaRef: binding.ProjectedSchemaRef}
	}
	if source.Format != target.Format {
		return problem("port_type_mismatch", path, "source and destination formats differ")
	}
	if source.Format == "json" {
		if source.SchemaRef == nil || target.SchemaRef == nil || *source.SchemaRef != *target.SchemaRef {
			return problem("port_type_mismatch", path, "JSON ports require the same exact schema reference")
		}
	} else {
		for _, mediaType := range source.MediaTypes {
			if !slices.Contains(target.MediaTypes, mediaType) {
				return problem("port_type_mismatch", path, "destination does not accept every producer media type")
			}
		}
	}
	return nil
}

// Next chooses a route from an accepted result. It never accepts that result,
// creates a worker, changes a verdict or treats an absent handler as pass.
func (p *Plan) Next(stageID, verdict string) (string, error) {
	stage, exists := p.Workflow.Definition.Stages[stageID]
	if !exists || stage.Kind != "step" {
		return "", problem("invalid_stage", "/definition/stages/"+escapePointer(stageID), "expected a compiled step stage")
	}
	if !slices.Contains([]string{"pass", "fail", "needs_revision", "no_work"}, verdict) {
		return "", problem("invalid_verdict", "", "not a StepResult verdict")
	}
	next, exists := stage.On[verdict]
	if !exists {
		return "", problem("unhandled_verdict", "/definition/stages/"+escapePointer(stageID)+"/on", "accepted result has no declared route: "+verdict)
	}
	return next, nil
}

func keys[V any](values map[string]V) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	slices.Sort(result)
	return result
}
