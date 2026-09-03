package flow

import "slices"

type definitionIdentity struct{ id, version string }

// compilation owns caches and validation budgets for one complete closure.
// It never expands one cached definition into all of its runtime invocations.
type compilation struct {
	pinned Registry
	// Only CompileCore enables these typed context inventories. All nested
	// plans share the selected closure and its dependency budgets.
	availableResources ContextResources
	resources          ContextResources
	expandedUses       map[referenceUse]bool
	schemas            map[Ref][]byte
	schemaValues       map[Ref]any
	plans              map[Ref]*Plan
	steps              map[Ref]StepDefinition
	checks             map[Ref]CheckDefinition
	active             map[Ref]bool
	activeIdentities   map[definitionIdentity]bool
	identities         map[definitionIdentity]Ref
	conditionChecked   map[Ref]bool
	dependencyBytes    int
	graphWork          int
}

func newCompilation() *compilation {
	return &compilation{
		pinned: make(Registry), schemas: make(map[Ref][]byte), schemaValues: make(map[Ref]any),
		plans: make(map[Ref]*Plan), steps: make(map[Ref]StepDefinition), active: make(map[Ref]bool), activeIdentities: make(map[definitionIdentity]bool),
		identities: make(map[definitionIdentity]Ref), conditionChecked: make(map[Ref]bool),
	}
}

func (c *compilation) identify(ref Ref, path string) error {
	key := definitionIdentity{ref.ID, ref.Version}
	if before, exists := c.identities[key]; exists && before != ref {
		return problem("ref_identity_conflict", path, "one definition identity/version resolves to different content")
	}
	c.identities[key] = ref
	return nil
}

func (c *compilation) enter(ref Ref) error {
	key := definitionIdentity{ref.ID, ref.Version}
	if c.active[ref] || c.activeIdentities[key] {
		return problem("call_cycle", "/definition", "workflow call graph re-enters an active definition")
	}
	if len(c.active) > MaxDepth || len(c.plans) >= 1024 {
		return problem("dependency_limit", "/definition", "workflow closure exceeds bounded depth or definition count")
	}
	if err := c.identify(ref, ""); err != nil {
		return err
	}
	c.active[ref], c.activeIdentities[key] = true, true
	return nil
}

func (c *compilation) child(parent *Plan, ref Ref, path string) (*Plan, error) {
	if c.active[ref] || c.activeIdentities[definitionIdentity{ref.ID, ref.Version}] {
		return nil, problem("call_cycle", path, "workflow call graph re-enters an active definition")
	}
	if child := c.plans[ref]; child != nil {
		return child, nil
	}
	data, exists := parent.Registry[ref]
	if !exists {
		return nil, problem("missing_ref", path, "child workflow is not pinned")
	}
	// pinRefs already checked the original child condition literals before
	// canonicalization. Compile the detached pinned bytes, not mutable inventory.
	return compileWorkflow(data, "json", parent.Registry, CoreProfile, c)
}

// StageOutputs exposes only the local producer's declared public ports. A
// caller cannot address its child's private stages through a deep stage path.
func (p *Plan) StageOutputs(stageID string) map[string]OutputPort {
	stage := p.Workflow.Definition.Stages[stageID]
	switch stage.Kind {
	case "step":
		return p.Steps[stageID].Outputs
	case "call", "repeat":
		if child := p.BodyPlan(stageID); child != nil {
			return child.Workflow.Outputs
		}
	case "wait":
		// An accepted event is exported as an artifact of its own declared
		// schema. Expiry produces nothing: no event arrived, so there is no
		// payload to hand on, and the two routes differ in exactly that.
		ref := stage.EventSchemaRef
		required := []string{"on_event"}
		if source, ok := p.PublicationSource(stage.SourceRef); ok {
			if source.Mode == "once" {
				return map[string]OutputPort{WaitEventPort: {Port: source.ArtifactPort(), RequiredFor: required}}
			}
			if source.Mode == "each_publication" {
				required = append(required, "on_timeout")
			}
		}
		return map[string]OutputPort{WaitEventPort: {Port: Port{Format: "json", SchemaRef: &ref}, RequiredFor: required}}
	case "parallel", "map":
		ref := p.aggregateSchemaRef()
		if ref == nil {
			return nil
		}
		// A settled join produces its summary on both of its verdicts. An empty
		// map reaches neither: it has no branch results to summarise, so the
		// value is absent on that route rather than an empty success.
		return map[string]OutputPort{AggregateResultsPort: {Port: Port{Format: "json", SchemaRef: ref}, RequiredFor: []string{"satisfied", "unsatisfied"}}}
	}
	return nil
}

// aggregateSchemaRef finds the shipped summary form among the pinned
// definitions. A build without it cannot describe what a parallel stage
// produces, so the reference is looked up rather than assumed.
func (p *Plan) aggregateSchemaRef() *Ref {
	for ref := range p.Registry {
		if ref.ID == AggregateSchemaID {
			found := ref
			return &found
		}
	}
	return nil
}

// BodyPlan returns the exact nested definition for a local control stage. A
// cached definition does not identify a particular invocation or iteration.
// A parallel stage has one definition per branch, so it is addressed by branch
// identity through BranchPlan rather than by the stage alone.
func (p *Plan) BodyPlan(stageID string) *Plan {
	switch p.Workflow.Definition.Stages[stageID].Kind {
	case "call":
		return p.Calls[stageID]
	case "repeat":
		return p.Repeats[stageID]
	}
	return nil
}

// BranchPlan returns the exact pinned definition one branch runs. A map's
// items all run the same sealed body, so the key selects the item rather than
// a definition; a parallel stage has one definition per declared branch.
func (p *Plan) BranchPlan(stageID, branchID string) *Plan {
	switch p.Workflow.Definition.Stages[stageID].Kind {
	case "parallel":
		return p.Branches[stageID][branchID]
	case "map":
		return p.Maps[stageID]
	}
	return nil
}

// NextOutcome handles a completed child outcome, never a StepResult verdict or
// technical failure. A missing mapping is not an implicit successful return.
func (p *Plan) NextOutcome(stageID, outcome string) (string, error) {
	if p.Profile != CoreProfile {
		return "", problem("unsupported", "", "child outcomes require "+CoreProfile)
	}
	stage, exists := p.Workflow.Definition.Stages[stageID]
	child := p.Calls[stageID]
	path := "/definition/stages/" + escapePointer(stageID)
	if !exists || stage.Kind != "call" || child == nil {
		return "", problem("invalid_stage", path, "expected a compiled call stage")
	}
	if !slices.Contains(child.Workflow.AllowedOutcomes, outcome) {
		return "", problem("invalid_outcome", path+"/on", "outcome is not declared by the child workflow")
	}
	next, exists := stage.On[outcome]
	if !exists {
		return "", problem("unhandled_outcome", path+"/on", "completed child has no declared outcome route")
	}
	return next, nil
}

// NextJoin handles a join verdict, never a branch outcome or a StepResult
// verdict. A join that reached a verdict the definition does not route is a
// contract gap, not an implicit route to the other verdict.
func (p *Plan) NextJoin(stageID, verdict string) (string, error) {
	if p.Profile != CoreProfile {
		return "", problem("unsupported", "", "join verdicts require "+CoreProfile)
	}
	stage, exists := p.Workflow.Definition.Stages[stageID]
	path := "/definition/stages/" + escapePointer(stageID)
	fanOut := stage.Kind == "parallel" && len(p.Branches[stageID]) != 0 || stage.Kind == "map" && p.Maps[stageID] != nil
	if !exists || stage.Join == nil || !fanOut {
		return "", problem("invalid_stage", path, "expected a compiled fan-out stage")
	}
	if verdict != "satisfied" && verdict != "unsatisfied" {
		return "", problem("invalid_verdict", path+"/on", "not a join verdict")
	}
	next, exists := stage.On[verdict]
	if !exists {
		return "", problem("unhandled_verdict", path+"/on", "join verdict has no declared route: "+verdict)
	}
	return next, nil
}

func checkChildInputBindings(bindings map[string]Binding, inputs map[string]InputPort, requireInputs bool, path string, check func(Binding, Port, bool, string) error) error {
	for _, name := range keys(inputs) {
		input := inputs[name]
		if _, exists := bindings[name]; requireInputs && input.Required && input.Configuration == nil && !exists {
			return problem("missing_binding", path+"/"+escapePointer(name), "required child input has no binding")
		}
	}
	for _, name := range keys(bindings) {
		input, exists := inputs[name]
		bindingPath := path + "/" + escapePointer(name)
		if !exists {
			return problem("unknown_port", bindingPath, "child input port is not declared")
		}
		if input.Configuration != nil && input.Configuration.Scope == "project" {
			return problem("configuration_scope", bindingPath, "child binding cannot override project-scoped configuration")
		}
		if err := check(bindings[name], input.Port, input.Required, bindingPath); err != nil {
			return err
		}
	}
	// An omitted configuration input is resolved and sealed by root Start.
	// This authoring check does not assert that a project value actually exists.
	return nil
}

type executionCost struct{ steps, transitions int64 }
type executionBounds struct {
	outcomes map[string]executionCost
	prefix   executionCost
	depth    int
}

func maxCost(a, b executionCost) executionCost {
	return executionCost{max(a.steps, b.steps), max(a.transitions, b.transitions)}
}

func (p *Plan) addCost(a, b executionCost) (executionCost, error) {
	limits := p.Workflow.Limits
	// Subtract before adding: even a deeply reused DAG cannot wrap a counter
	// into an apparently affordable path.
	if a.steps > limits.MaxStepInstances || b.steps > limits.MaxStepInstances-a.steps || a.transitions > limits.MaxControlTransitions || b.transitions > limits.MaxControlTransitions-a.transitions {
		return executionCost{}, problem("limit_exceeded", "/limits", "limits cannot cover every permitted nested path")
	}
	return executionCost{a.steps + b.steps, a.transitions + b.transitions}, nil
}
