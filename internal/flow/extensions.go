package flow

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
)

type outputKey struct{ stage, port string }
type graphEdge struct{ target, verdict, repeatRoute, publicationStage string }

// checkCoreGraph keeps one dataflow summary per pending stage. true means a
// referenced output exists on every incoming path; false means its producer
// occurs on some path but the value may be absent. Error edges produce none.
// AggregateSchemaID names the shipped standard form of a parallel stage's
// summary. A consumer declares this exact reference to receive it; projecting
// it into a project's own shape is the supported way to use a different form.
const AggregateSchemaID = "core:schema/aggregate-manifest"

// WorkspaceTreeManifestSchemaID names the standard, sealed index of native
// workspace files. Its entries refer to ordinary raw ArtifactRevisions; it is
// not a virtual filesystem and does not name a provider-specific layout.
const WorkspaceTreeManifestSchemaID = "core:schema/workspace-tree-manifest"

// MaxQualifiedParallelism is how many attempts this build is qualified to run
// at once. It bounds what a definition may declare; the authority separately
// bounds what it will admit, and the smaller of the two governs.
const MaxQualifiedParallelism = 4

// AggregateResultsPort is the only port a parallel stage produces.
const AggregateResultsPort = "results"

// MaxParallelBranches is the qualified profile's fan-out cap per activation.
// The protocol schema permits far more, so a definition within the schema can
// still exceed what this installation is qualified to run.
const MaxParallelBranches = 32

// checkJoin validates what the schema cannot: that the branches exist, are
// distinct, within the qualified fan-out, and that the required successes are
// reachable at all. A quorum larger than the branch count would be a contract
// nobody can satisfy.
func (p *Plan) checkJoin(stage Stage, id, path string) error {
	if stage.Join == nil || len(stage.ParallelBranches) == 0 {
		return problem("invalid_join", path+"/join", "a parallel stage declares its branches and its join contract")
	}
	if len(stage.ParallelBranches) > MaxParallelBranches {
		return problem("unsupported", path+"/branches", fmt.Sprintf("the qualified profile admits at most %d branches on one activation", MaxParallelBranches))
	}
	seen := map[string]bool{}
	for i, branch := range stage.ParallelBranches {
		branchPath := fmt.Sprintf("%s/branches/%d", path, i)
		if seen[branch.ID] {
			return problem("duplicate_branch_id", branchPath+"/id", "parallel branch identities must be unique")
		}
		seen[branch.ID] = true
		if p.Branches[id][branch.ID] == nil {
			return problem("missing_ref", branchPath+"/workflow_ref", "branch workflow is not pinned")
		}
	}
	if stage.MaxParallelism < 1 {
		return problem("invalid_join", path+"/max_parallelism", "a parallel stage declares how many branches may run at once")
	}
	// A stage may not claim more simultaneity than the workflow it belongs to:
	// the declaration would otherwise be quietly ignored at run time.
	if stage.MaxParallelism > p.Workflow.Limits.MaxParallelism {
		return problem("unsupported", path+"/max_parallelism", "a stage cannot exceed the workflow's declared parallelism")
	}

	if stage.Join.Mode == "quorum" && stage.Join.RequiredSuccesses > len(stage.ParallelBranches) {
		return problem("invalid_join", path+"/join/required_successes", "the quorum cannot exceed the number of branches")
	}
	for _, outcome := range stage.Join.AcceptOutcomes {
		accepted := false
		for _, branch := range stage.ParallelBranches {
			accepted = accepted || slices.Contains(p.Branches[id][branch.ID].Workflow.AllowedOutcomes, outcome)
		}
		if !accepted {
			return problem("invalid_join", path+"/join/accept_outcomes", "no branch declares the accepted outcome "+outcome)
		}
	}
	return nil
}

// A parallel stage exposes no stage outputs yet: consuming branch results needs
// an aggregate contract that is not settled. Referencing one is refused with
// that reason rather than resolving to nothing.
// checkParallelOutputReferences allows the one port a parallel stage produces
// and refuses any other. A reference to a port the stage cannot produce must
// fail by name rather than resolve to nothing at run time.
func (p *Plan) checkParallelOutputReferences() error {
	stages := p.Workflow.Definition.Stages
	refuse := func(binding Binding, path string) error {
		kind := stages[binding.StageID].Kind
		if binding.From != "stage_output" || (kind != "parallel" && kind != "map") {
			return nil
		}
		if binding.Port != AggregateResultsPort {
			return problem("unknown_port", path+"/port", "a fan-out stage produces only "+AggregateResultsPort)
		}
		if p.aggregateSchemaRef() == nil {
			return problem("missing_ref", path+"/port", "the shipped summary form "+AggregateSchemaID+" is not available")
		}
		return nil
	}
	for _, id := range keys(stages) {
		stage := stages[id]
		path := "/definition/stages/" + escapePointer(id)
		for _, group := range []struct {
			name     string
			bindings map[string]Binding
		}{{"input_bindings", stage.InputBindings}, {"initial_bindings", stage.InitialBindings}, {"next_bindings", stage.NextBindings}, {"output_bindings", stage.OutputBindings}} {
			for _, port := range keys(group.bindings) {
				if err := refuse(group.bindings[port], path+"/"+group.name+"/"+escapePointer(port)); err != nil {
					return err
				}
			}
		}
		for i, branch := range stage.ParallelBranches {
			for _, port := range keys(branch.InputBindings) {
				if err := refuse(branch.InputBindings[port], fmt.Sprintf("%s/branches/%d/input_bindings/%s", path, i, escapePointer(port))); err != nil {
					return err
				}
			}
		}
		if stage.Items != nil {
			if err := refuse(*stage.Items, path+"/items"); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *Plan) checkCoreGraph() error {
	w := &p.Workflow
	stages := w.Definition.Stages
	if _, exists := stages[w.Definition.Entry]; !exists {
		return problem("missing_stage", "/definition/entry", "entry stage does not exist")
	}
	edges := make(map[string][]graphEdge, len(stages))
	indegree := make(map[string]int, len(stages))
	referenced := make(map[outputKey]bool)
	for _, id := range keys(stages) {
		stage := stages[id]
		path := "/definition/stages/" + escapePointer(id)
		if stage.Kind == "choice" {
			branchIDs := make(map[string]bool, len(stage.Branches))
			for i, branch := range stage.Branches {
				branchPath := fmt.Sprintf("%s/branches/%d", path, i)
				if branchIDs[branch.ID] {
					return problem("duplicate_branch_id", branchPath+"/id", "choice branch identities must be unique")
				}
				branchIDs[branch.ID] = true
				if _, exists := stages[branch.Next]; !exists {
					return problem("missing_stage", branchPath+"/next", "branch transition target does not exist")
				}
				edges[id] = append(edges[id], graphEdge{target: branch.Next, publicationStage: p.itemPredicatePublicationStage(branch.Predicate)})
				indegree[branch.Next]++
				if err := predicateFields(branch.Predicate, branchPath+"/predicate", func(ref FieldRef, _ string) error {
					if ref.From == "stage_output" {
						referenced[outputKey{ref.StageID, ref.Port}] = true
					}
					return nil
				}); err != nil {
					return err
				}
			}
			for _, route := range []struct{ name, target string }{{"default", stage.Default}, {"on_unknown", stage.OnUnknown}} {
				if route.target == "" {
					continue
				}
				if _, exists := stages[route.target]; !exists {
					return problem("missing_stage", path+"/"+route.name, "choice transition target does not exist")
				}
				edges[id] = append(edges[id], graphEdge{target: route.target})
				indegree[route.target]++
			}
		}
		if stage.Kind == "repeat" {
			body := p.Repeats[id]
			if stage.LimitConfiguration != "" {
				input, exists := w.Inputs[stage.LimitConfiguration]
				if !exists || input.Configuration == nil {
					return problem("unknown_configuration_input", path+"/limit_configuration", "repeat limit names a declared configuration input")
				}
				if input.Configuration.Scope != "project" {
					return problem("configuration_scope", path+"/limit_configuration", "repeat limit configuration must be project-scoped")
				}
				if input.Format != "json" || input.SchemaRef == nil {
					return problem("configuration_type", path+"/limit_configuration", "repeat limit configuration requires an explicitly typed JSON input")
				}
			}
			for i, outcome := range stage.ContinueOn {
				if !slices.Contains(body.Workflow.AllowedOutcomes, outcome) {
					return problem("invalid_outcome", fmt.Sprintf("%s/continue_on/%d", path, i), "continuation outcome is not allowed by the body workflow")
				}
			}
			for _, outcome := range keys(stage.OnComplete) {
				if !slices.Contains(body.Workflow.AllowedOutcomes, outcome) {
					return problem("invalid_outcome", path+"/on_complete/"+outcome, "route is not an allowed body outcome")
				}
				target := stage.OnComplete[outcome]
				if _, exists := stages[target]; !exists {
					return problem("missing_stage", path+"/on_complete/"+outcome, "repeat transition target does not exist")
				}
				edges[id] = append(edges[id], graphEdge{target: target, verdict: outcome, repeatRoute: "on_complete"})
				indegree[target]++
			}
			for _, route := range []struct{ name, target string }{{"on_limit", stage.OnLimit}, {"on_unknown", stage.OnUnknown}} {
				if route.target == "" {
					continue
				}
				if _, exists := stages[route.target]; !exists {
					return problem("missing_stage", path+"/"+route.name, "repeat transition target does not exist")
				}
				edges[id] = append(edges[id], graphEdge{target: route.target, repeatRoute: route.name})
				indegree[route.target]++
			}
			if err := predicateFields(stage.Until, path+"/until", func(ref FieldRef, _ string) error {
				if ref.From == "stage_output" {
					referenced[outputKey{ref.StageID, ref.Port}] = true
				}
				return nil
			}); err != nil {
				return err
			}
		}
		if stage.Kind == "parallel" {
			if err := p.checkJoin(stage, id, path); err != nil {
				return err
			}
		}
		if stage.Kind == "map" {
			if err := p.checkMap(stage, id, path); err != nil {
				return err
			}
		}
		if stage.Kind == "wait" {
			if err := p.checkWait(stage, path); err != nil {
				return err
			}
			for _, route := range []struct{ name, target string }{{"on_event", stage.OnEvent}, {"on_timeout", stage.OnTimeout}} {
				if route.target == "" {
					continue
				}
				if _, exists := stages[route.target]; !exists {
					return problem("missing_stage", path+"/"+route.name, "wait transition target does not exist")
				}
				edges[id] = append(edges[id], graphEdge{target: route.target, verdict: route.name})
				indegree[route.target]++
			}
		}
		if stage.Kind == "parallel" || stage.Kind == "map" {
			if err := p.checkParallelOutputReferences(); err != nil {
				return err
			}
		}
		for _, verdict := range keys(stage.On) {
			if stage.Kind == "call" && !slices.Contains(p.Calls[id].Workflow.AllowedOutcomes, verdict) {
				return problem("invalid_outcome", path+"/on/"+verdict, "route is not an allowed child outcome")
			}
			target := stage.On[verdict]
			if _, exists := stages[target]; !exists {
				return problem("missing_stage", path+"/on/"+verdict, "transition target does not exist")
			}
			edges[id] = append(edges[id], graphEdge{target: target, verdict: verdict})
			indegree[target]++
		}
		if stage.OnError != "" {
			if _, exists := stages[stage.OnError]; !exists {
				return problem("missing_stage", path+"/on_error", "error transition target does not exist")
			}
			edges[id] = append(edges[id], graphEdge{target: stage.OnError})
			indegree[stage.OnError]++
		}
		for _, bindings := range []map[string]Binding{stage.InputBindings, stage.OutputBindings, stage.InitialBindings, stage.NextBindings} {
			for _, binding := range bindings {
				if binding.From == "stage_output" {
					referenced[outputKey{binding.StageID, binding.Port}] = true
				} else if binding.From == "publication" {
					referenced[outputKey{binding.StageID, WaitPublicationPort}] = true
				}
			}
		}
		for _, branch := range stage.ParallelBranches {
			for _, binding := range branch.InputBindings {
				if binding.From == "stage_output" {
					referenced[outputKey{binding.StageID, binding.Port}] = true
				}
			}
		}
		if stage.Items != nil && stage.Items.From == "stage_output" {
			referenced[outputKey{stage.Items.StageID, stage.Items.Port}] = true
		}
		if stage.CorrelationInput != nil && stage.CorrelationInput.From == "stage_output" {
			referenced[outputKey{stage.CorrelationInput.StageID, stage.CorrelationInput.Port}] = true
		}
		if stage.CursorInput != nil && stage.CursorInput.From == "stage_output" {
			referenced[outputKey{stage.CursorInput.StageID, stage.CursorInput.Port}] = true
		}
	}
	seen := map[string]bool{w.Definition.Entry: true}
	queue := []string{w.Definition.Entry}
	for i := 0; i < len(queue); i++ {
		for _, edge := range edges[queue[i]] {
			if !seen[edge.target] {
				seen[edge.target] = true
				queue = append(queue, edge.target)
			}
		}
	}
	for _, id := range keys(stages) {
		if !seen[id] {
			return problem("unreachable_stage", "/definition/stages/"+escapePointer(id), "stage is not reachable from entry")
		}
	}
	queue = nil
	if indegree[w.Definition.Entry] == 0 {
		queue = append(queue, w.Definition.Entry)
	}
	for i := 0; i < len(queue); i++ {
		for _, edge := range edges[queue[i]] {
			indegree[edge.target]--
			if indegree[edge.target] == 0 {
				queue = append(queue, edge.target)
			}
		}
	}
	if len(queue) != len(stages) {
		return problem("cycle", "/definition/stages", "stage graph contains a cycle; repeats require an explicit supported operator")
	}
	p.Sequence = queue
	available := map[string]map[outputKey]bool{w.Definition.Entry: {}}
	costs := make(map[string]executionCost)
	if p.Checks != nil {
		entry, err := p.addCost(executionCost{}, executionCost{transitions: inputCheckCount(w.Inputs)})
		if err != nil {
			return err
		}
		costs[w.Definition.Entry] = entry
	}
	p.bounds = executionBounds{outcomes: make(map[string]executionCost)}
	work := len(stages)
	for _, outgoing := range edges {
		work += len(outgoing)
	}
	if p.compilation != nil {
		work += p.compilation.graphWork
		defer func() { p.compilation.graphWork = work }()
	}
	if work > 1_000_000 {
		return problem("graph_validation_limit", "/definition", "nested graph proof exceeds one million work units")
	}
	for _, id := range queue {
		stage, facts := stages[id], available[id]
		path := "/definition/stages/" + escapePointer(id)
		increment := executionCost{transitions: 1}
		check := func(binding Binding, target Port, required bool, path string) error {
			if binding.From == "publication" && stage.Kind != "call" {
				return problem("invalid_publication_source", path+"/from", "an assigned publication can feed only a call")
			}
			return p.checkAvailableBinding(binding, target, required, facts, path)
		}
		switch stage.Kind {
		case "step":
			if err := checkInputBindings(stage.InputBindings, p.Steps[id].Inputs, path+"/input_bindings", check); err != nil {
				return err
			}
			increment.steps = 1
			increment.transitions += p.stepCheckCount(id)
		case "parallel":
			// Every branch may run, so the bound is their sum rather than the
			// largest: a quorum that cancels the remainder spends less, never
			// more, and a conservative bound must not assume the cheaper path.
			increment.transitions = 2
			for i, branch := range stage.ParallelBranches {
				child := p.Branches[id][branch.ID]
				branchPath := fmt.Sprintf("%s/branches/%d", path, i)
				if err := checkChildInputBindings(branch.InputBindings, child.Workflow.Inputs, true, branchPath+"/input_bindings", check); err != nil {
					return err
				}
				summed, err := p.addCost(increment, child.bounds.prefix)
				if err != nil {
					return err
				}
				increment = summed
			}
		case "wait":
			// Waiting is one control transition to enter and one to resolve. It
			// admits no attempt at all: nothing executes while a wait waits,
			// which is the whole point of having the operator.
			increment.transitions = 2
			if stage.CorrelationInput != nil {
				subject := "the key a wait correlates its event by"
				if source, ok := p.PublicationSource(stage.SourceRef); ok && source.Mode == "each_publication" {
					if err := check(*stage.CorrelationInput, Port{Format: "json", SchemaRef: source.HandleSchemaRef}, true, path+"/correlation_input"); err != nil {
						return err
					}
					subject = "the subscription handle"
				} else if err := p.checkOpaqueJSON(*stage.CorrelationInput, facts, path+"/correlation_input", subject); err != nil {
					return err
				}
			}
			if stage.CursorInput != nil {
				source, ok := p.PublicationSource(stage.SourceRef)
				if !ok || source.Mode != "each_publication" {
					return problem("invalid_wait", path+"/cursor_input", "only an each_publication wait accepts a cursor")
				}
				if err := check(*stage.CursorInput, Port{Format: "json", SchemaRef: source.CursorSchemaRef}, true, path+"/cursor_input"); err != nil {
					return err
				}
			}
		case "map":
			// The cap is what the definition may spend, not what one collection
			// happens to hold: a bound that assumed the smaller actual list
			// would be no bound at all, since the list is not known until run time.
			body := p.Maps[id]
			increment.transitions = 2
			if err := checkChildInputBindings(stage.InputBindings, body.Workflow.Inputs, false, path+"/input_bindings", check); err != nil {
				return err
			}
			if stage.Items != nil {
				if err := p.checkOpaqueJSON(*stage.Items, facts, path+"/items", "the collection a map fans out over"); err != nil {
					return err
				}
			}
			for range stage.MaxItems {
				summed, err := p.addCost(increment, body.bounds.prefix)
				if err != nil {
					return err
				}
				increment = summed
			}
			p.bounds.depth = max(p.bounds.depth, body.bounds.depth+1)
			if p.bounds.depth > w.Limits.MaxChildDepth {
				return problem("limit_exceeded", "/limits/max_child_depth", "child invocations exceed the workflow's descendant depth limit")
			}
		case "call", "repeat":
			child := p.BodyPlan(id)
			if stage.Kind == "call" {
				if err := checkChildInputBindings(stage.InputBindings, child.Workflow.Inputs, true, path+"/input_bindings", check); err != nil {
					return err
				}
				// RUN-006 charges entering and returning from a call. A repeat
				// has one activation; its later decisions are charged below.
				increment.transitions = 2
			} else {
				if err := checkChildInputBindings(stage.InitialBindings, child.Workflow.Inputs, true, path+"/initial_bindings", func(binding Binding, target Port, required bool, bindingPath string) error {
					return p.checkRepeatInitialBinding(binding, target, required, child, id, facts, bindingPath)
				}); err != nil {
					return err
				}
				if err := checkChildInputBindings(stage.NextBindings, child.Workflow.Inputs, stage.MaxIterations > 1, path+"/next_bindings", func(binding Binding, target Port, required bool, bindingPath string) error {
					return p.checkRepeatBinding(binding, target, required, child, id, stage.ContinueOn, facts, bindingPath)
				}); err != nil {
					return err
				}
				if err := p.checkStreamRepeat(stage, id, child, path); err != nil {
					return err
				}
				if err := predicateFields(stage.Until, path+"/until", func(ref FieldRef, fieldPath string) error {
					return p.checkRepeatFieldRef(ref, child, facts, fieldPath)
				}); err != nil {
					return err
				}
			}
			p.bounds.depth = max(p.bounds.depth, child.bounds.depth+1)
			if p.bounds.depth > w.Limits.MaxChildDepth {
				return problem("limit_exceeded", "/limits/max_child_depth", "child invocations exceed the workflow's descendant depth limit")
			}
		case "finish":
			if err := p.checkFinishBindings(stage, path, check); err != nil {
				return err
			}
			increment.transitions += p.finishCheckCount(stage)
		case "choice":
			for i, branch := range stage.Branches {
				if err := predicateFields(branch.Predicate, fmt.Sprintf("%s/branches/%d/predicate", path, i), func(ref FieldRef, fieldPath string) error {
					return p.checkFieldRef(ref, facts, fieldPath)
				}); err != nil {
					return err
				}
			}
		}
		cost, err := p.addCost(costs[id], increment)
		if err != nil {
			return err
		}
		prefix := cost
		if stage.Kind == "call" {
			prefix, err = p.addCost(cost, p.Calls[id].bounds.prefix)
			if err != nil {
				return err
			}
		}
		if stage.Kind == "repeat" {
			bodyCost, err := p.repeatCost(stage, p.Repeats[id], "", "")
			if err != nil {
				return err
			}
			prefix, err = p.addCost(cost, bodyCost)
			if err != nil {
				return err
			}
		}
		p.bounds.prefix = maxCost(p.bounds.prefix, prefix)
		if stage.Kind == "finish" {
			p.bounds.outcomes[stage.Outcome] = maxCost(p.bounds.outcomes[stage.Outcome], cost)
		}
		produced := make(map[outputKey]OutputPort)
		for name, port := range p.StageOutputs(id) {
			key := outputKey{id, name}
			if referenced[key] {
				produced[key] = port
			}
		}
		publicationKey := outputKey{id, WaitPublicationPort}
		if stage.Kind == "wait" && referenced[publicationKey] {
			if source, ok := p.PublicationSource(stage.SourceRef); ok && source.Mode == "each_publication" {
				produced[publicationKey] = OutputPort{Port: source.ArtifactPort()}
			}
		}
		for _, edge := range edges[id] {
			pathCost := cost
			if stage.Kind == "call" {
				child := p.Calls[id]
				childCost, exists := child.bounds.outcomes[edge.verdict]
				if !exists {
					// Error returns include the largest child prefix. A declared
					// but unused outcome is kept conservative, not constant-folded.
					childCost = child.bounds.prefix
				}
				pathCost, err = p.addCost(cost, childCost)
				if err != nil {
					return err
				}
			}
			if stage.Kind == "repeat" {
				bodyCost, err := p.repeatCost(stage, p.Repeats[id], edge.repeatRoute, edge.verdict)
				if err != nil {
					return err
				}
				pathCost, err = p.addCost(cost, bodyCost)
				if err != nil {
					return err
				}
			}
			destination, prior := available[edge.target]
			if !prior {
				destination = make(map[outputKey]bool)
				available[edge.target] = destination
			}
			// ponytail: at most one million propagated facts; use liveness-based
			// pruning if real definitions exceed this explicit validation ceiling.
			work += len(destination) + len(facts) + len(produced)
			if work > 1_000_000 {
				return problem("graph_validation_limit", "/definition", "nested data availability proof exceeds one million work units")
			}
			for key := range destination {
				if _, inherited := facts[key]; !inherited {
					if _, emitted := produced[key]; !emitted {
						destination[key] = false
					}
				}
			}
			merge := func(key outputKey, guaranteed bool) {
				before, exists := destination[key]
				destination[key] = guaranteed && ((!prior) || (exists && before))
			}
			for key, guaranteed := range facts {
				merge(key, guaranteed)
			}
			for key, port := range produced {
				guaranteed := edge.verdict != "" && slices.Contains(port.RequiredFor, edge.verdict)
				if stage.Kind == "repeat" && (edge.repeatRoute == "on_limit" || edge.repeatRoute == "on_unknown") {
					guaranteed = requiredForAll(port, stage.ContinueOn)
				}
				merge(key, guaranteed)
			}
			if edge.publicationStage != "" {
				merge(outputKey{edge.publicationStage, WaitPublicationPort}, true)
			}
			costs[edge.target] = maxCost(costs[edge.target], pathCost)
		}
		delete(available, id)
	}
	return nil
}

// itemPredicatePublicationStage recognizes the one proof that can make a
// publication binding total: exact equality of a stream delivery tag to Item.
// General predicate implication is intentionally outside this compiler.
func (p *Plan) deliveryPredicateStage(predicate Predicate, kind string) string {
	if predicate.Op != "eq" || predicate.Left == nil || predicate.Right == nil {
		return ""
	}
	match := func(field, literal *Operand) string {
		if field.Kind != "field" || field.Ref == nil || literal.Kind != "literal" {
			return ""
		}
		ref := *field.Ref
		if ref.From != "stage_output" || ref.Port != WaitEventPort || ref.Pointer != "/kind" {
			return ""
		}
		var tag string
		if json.Unmarshal(literal.Value, &tag) != nil || tag != kind {
			return ""
		}
		stage, exists := p.Workflow.Definition.Stages[ref.StageID]
		if !exists || stage.Kind != "wait" {
			return ""
		}
		source, publication, err := publicationSourceAt(p.Registry[stage.SourceRef], "")
		if err != nil || !publication || source.Mode != "each_publication" {
			return ""
		}
		return ref.StageID
	}
	if stage := match(predicate.Left, predicate.Right); stage != "" {
		return stage
	}
	return match(predicate.Right, predicate.Left)
}

func (p *Plan) itemPredicatePublicationStage(predicate Predicate) string {
	return p.deliveryPredicateStage(predicate, "Item")
}

func (p *Plan) checkAvailableBinding(binding Binding, target Port, required bool, available map[outputKey]bool, path string) error {
	source, guaranteed, err := p.bindingSource(binding, available, path)
	if err != nil {
		return err
	}
	return p.checkBoundPort(binding, source, target, required, guaranteed, path)
}

// bindingSource resolves what a binding reads and whether that value is
// guaranteed on every incoming path. It says nothing about whether the
// destination accepts it: that judgement differs between a port-to-port
// binding and a collection a map fans out over.
func (p *Plan) bindingSource(binding Binding, available map[outputKey]bool, path string) (Port, bool, error) {
	var source Port
	guaranteed := true
	switch binding.From {
	case "workflow_input":
		input, exists := p.Workflow.Inputs[binding.Port]
		if !exists {
			return Port{}, false, problem("unknown_port", path+"/port", "workflow input does not exist")
		}
		source = input.Port
		guaranteed = input.Required || (input.Configuration != nil && len(input.Configuration.Default) != 0)
	case "stage_output":
		output, exists := p.StageOutputs(binding.StageID)[binding.Port]
		if !exists {
			return Port{}, false, problem("unknown_port", path+"/port", "producer output does not exist")
		}
		source = output.Port
		guaranteed, exists = available[outputKey{binding.StageID, binding.Port}]
		if !exists {
			return Port{}, false, problem("unavailable_output", path+"/stage_id", "producer has not executed on any incoming path")
		}
	case "literal":
		var err error
		if source, err = p.literalPort(binding, path); err != nil {
			return Port{}, false, err
		}
	case "publication":
		stage, exists := p.Workflow.Definition.Stages[binding.StageID]
		if !exists || stage.Kind != "wait" {
			return Port{}, false, problem("missing_stage", path+"/stage_id", "publication binding producer is not a wait")
		}
		definition, exists := p.PublicationSource(stage.SourceRef)
		if !exists || definition.Mode != "each_publication" {
			return Port{}, false, problem("invalid_publication_source", path+"/stage_id", "publication binding requires an each_publication wait")
		}
		source = definition.ArtifactPort()
		guaranteed, exists = available[outputKey{binding.StageID, WaitPublicationPort}]
		if !exists {
			return Port{}, false, problem("unavailable_output", path+"/stage_id", "publication item is not proven on this path")
		}
	default:
		return Port{}, false, problem("unsupported", path+"/from", "unsupported binding source")
	}
	return source, guaranteed, nil
}

// checkOpaqueJSON validates a binding whose value has no destination port to
// match against: a collection a map fans out over, or the business key a wait
// correlates its event by. Neither is a port-to-port hand-off, so demanding an
// equal schema would refuse every valid one. What must hold is that the value
// exists on every incoming path and is JSON at all; what the value means is
// checked where it is used - each item against the body's item schema when the
// collection is sealed, the key against the event when one arrives.
func (p *Plan) checkOpaqueJSON(binding Binding, available map[outputKey]bool, path, subject string) error {
	source, guaranteed, err := p.bindingSource(binding, available, path)
	if err != nil {
		return err
	}
	if !guaranteed {
		return problem("unavailable_output", path, subject+" is not guaranteed on this path")
	}
	if binding.Pointer != nil {
		if _, err := pointerParts(*binding.Pointer); err != nil {
			return problem("invalid_pointer", path+"/pointer", err.Error())
		}
		if source.Format != "json" {
			return problem("port_type_mismatch", path, "projection requires a JSON source")
		}
		return nil
	}
	if source.Format != "json" {
		return problem("port_type_mismatch", path, subject+" must be JSON")
	}
	return nil
}

func (p *Plan) checkFieldRef(ref FieldRef, available map[outputKey]bool, path string) error {
	var source Port
	switch ref.From {
	case "workflow_input":
		input, exists := p.Workflow.Inputs[ref.Port]
		if !exists {
			return problem("unknown_port", path+"/port", "condition workflow input does not exist")
		}
		source = input.Port
	case "stage_output":
		if _, exists := p.Workflow.Definition.Stages[ref.StageID]; !exists {
			return problem("missing_stage", path+"/stage_id", "condition producer does not exist")
		}
		output, exists := p.StageOutputs(ref.StageID)[ref.Port]
		if !exists {
			return problem("unknown_port", path+"/port", "condition producer output does not exist")
		}
		if _, exists := available[outputKey{ref.StageID, ref.Port}]; !exists {
			return problem("unavailable_output", path+"/stage_id", "condition producer has not executed on any incoming path")
		}
		source = output.Port
	default:
		return problem("unsupported", path+"/from", "condition source is not supported by "+CoreProfile)
	}
	if source.Format != "json" || source.SchemaRef == nil {
		return problem("condition_type_mismatch", path, "condition fields require a JSON port")
	}
	if _, err := pointerParts(ref.Pointer); err != nil {
		return problem("invalid_pointer", path+"/pointer", err.Error())
	}
	return nil
}

// NextError only chooses a declared route. The runtime must first establish a
// known technical failure and settle its effects; uncertain work cannot enter
// this path merely because an error handler exists.
func (p *Plan) NextError(stageID string) (string, error) {
	if p.Profile != CoreProfile {
		return "", problem("unsupported", "", "error transitions require "+CoreProfile)
	}
	stage, exists := p.Workflow.Definition.Stages[stageID]
	if !exists || stage.Kind != "step" && stage.Kind != "choice" && stage.Kind != "call" && stage.Kind != "repeat" {
		return "", problem("invalid_stage", "/definition/stages/"+escapePointer(stageID), "expected a compiled step, choice, call or repeat stage")
	}
	if stage.OnError == "" {
		return "", problem("unhandled_error", "/definition/stages/"+escapePointer(stageID)+"/on_error", "known technical failure has no declared route")
	}
	return stage.OnError, nil
}

// ProjectJSON selects data, never runs code or reads external state. The caller
// verifies the source artifact and then seals these bytes with its provenance.
// Missing is (nil,false,nil); present null is ([]byte("null"),true,nil).
func (p *Plan) ProjectJSON(binding Binding, data []byte) ([]byte, bool, error) {
	if p.Profile != CoreProfile {
		return nil, false, problem("unsupported", "/pointer", "projection requires "+CoreProfile)
	}
	if binding.Pointer == nil || binding.ProjectedSchemaRef == nil {
		return nil, false, problem("invalid_projection", "", "projection requires pointer and projected schema")
	}
	if p.schemas[*binding.ProjectedSchemaRef] == nil {
		return nil, false, problem("missing_ref", "/projected_schema_ref", "projection schema was not compiled")
	}
	if _, err := pointerParts(*binding.Pointer); err != nil {
		return nil, false, problem("invalid_pointer", "/pointer", err.Error())
	}
	value, err := Parse(data, "json")
	if err != nil {
		return nil, false, err
	}
	selected, exists := JSONPointer(value, *binding.Pointer)
	if !exists {
		return nil, false, nil
	}
	encoded, err := json.Marshal(selected)
	if err != nil {
		return nil, false, err
	}
	if err := p.ValidateJSON(*binding.ProjectedSchemaRef, encoded); err != nil {
		return nil, false, problem("projection_schema_invalid", "/projected_schema_ref", err.Error())
	}
	canonical, err := Canonical(encoded)
	return canonical, err == nil, err
}

// projectionGuaranteed proves presence, not general schema inclusion. Explicit
// object required/properties, array minItems/items/prefixItems and acyclic local
// $refs suffice; projected value types are checked against their own schema at
// runtime. A composed schema without this proof fails closed. The caller rules
// out nested resource IDs, where a local fragment would change its base schema.
func projectionGuaranteed(root any, pointer string) (bool, error) {
	parts, err := pointerParts(pointer)
	if err != nil {
		return false, err
	}
	current := root
	for i, part := range parts {
		object, err := resolveLocalSchema(root, current)
		if err != nil {
			return false, fmt.Errorf("projection presence needs an explicit typed schema with supported local references")
		}
		switch object["type"] {
		case "object":
			required, _ := object["required"].([]any)
			if !slices.Contains(required, any(part)) {
				return false, nil
			}
			properties, _ := object["properties"].(map[string]any)
			current = properties[part]
		case "array":
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || strconv.Itoa(index) != part {
				return false, nil
			}
			minimum, _ := object["minItems"].(json.Number)
			count, err := minimum.Int64()
			if err != nil || int64(index) >= count {
				return false, nil
			}
			prefix, _ := object["prefixItems"].([]any)
			if index < len(prefix) {
				current = prefix[index]
			} else {
				current = object["items"]
			}
		default:
			return false, fmt.Errorf("projection presence requires explicit object or array types on its path")
		}
		if i == len(parts)-1 {
			return true, nil
		}
	}
	return true, nil
}

// The proof uses root-relative fragments only. Reject nested $id strings rather
// than resolve a different schema resource as if it were the outer document.
// This is a deliberately conservative proof subset, not a JSON Schema limit.
func nestedResourceID(value any, root bool) bool {
	switch value := value.(type) {
	case map[string]any:
		if _, hasID := value["$id"].(string); !root && hasID {
			return true
		}
		for _, child := range value {
			if nestedResourceID(child, false) {
				return true
			}
		}
	case []any:
		for _, child := range value {
			if nestedResourceID(child, false) {
				return true
			}
		}
	}
	return false
}

// MaxMapItems is the qualified profile's cap on one map expansion. The
// published contract permits far more, so a definition within the schema can
// still declare more items than this installation is qualified to run. It is
// the same cap as a parallel fan-out because a map is one: the items are the
// branches, and nothing about deriving them from a collection makes running
// them cheaper.
const MaxMapItems = MaxParallelBranches

// checkMap validates what the schema cannot: that the body exists, that the
// item port is the body's and is not bound twice, that the declared cap stays
// within the qualified fan-out, and that the join can be satisfied by the one
// body every item runs.
func (p *Plan) checkMap(stage Stage, id, path string) error {
	body := p.Maps[id]
	if stage.Join == nil || body == nil || stage.Items == nil {
		return problem("invalid_map", path, "a map stage declares its collection, its body and its join contract")
	}
	if stage.MaxItems < 1 || stage.MaxItems > MaxMapItems {
		return problem("unsupported", path+"/max_items", fmt.Sprintf("the qualified profile admits at most %d items on one activation", MaxMapItems))
	}
	if stage.MaxParallelism < 1 {
		return problem("invalid_map", path+"/max_parallelism", "a map stage declares how many items may run at once")
	}
	if stage.MaxParallelism > p.Workflow.Limits.MaxParallelism {
		return problem("unsupported", path+"/max_parallelism", "a stage cannot exceed the workflow's declared parallelism")
	}
	item, declared := body.Workflow.Inputs[stage.ItemInput]
	if !declared {
		return problem("unknown_port", path+"/item_input", "the body does not declare the item input port")
	}
	// One port cannot receive both the item and a stage binding: the two would
	// silently contend, and which one won would depend on evaluation order.
	if _, bound := stage.InputBindings[stage.ItemInput]; bound {
		return problem("duplicate_binding", path+"/input_bindings/"+escapePointer(stage.ItemInput), "the item input port is supplied by the collection, not by a binding")
	}
	if item.Port.Format != "json" {
		return problem("invalid_map", path+"/item_input", "an item is JSON, so the body's item port must accept JSON")
	}
	if _, err := pointerParts(stage.ItemKeyPointer); err != nil {
		return problem("invalid_pointer", path+"/item_key_pointer", "the item key pointer is not a JSON Pointer")
	}
	// The root of an item is the item itself, not a field of it. A whole item
	// as its own key would make identity depend on every byte of its content,
	// so a later edit anywhere in it would rename the child.
	if stage.ItemKeyPointer == "" {
		return problem("invalid_pointer", path+"/item_key_pointer", "the item key is a field of the item, not the whole item")
	}
	if stage.Join.Mode == "quorum" && stage.Join.RequiredSuccesses > stage.MaxItems {
		return problem("invalid_join", path+"/join/required_successes", "the quorum cannot exceed the declared item cap")
	}
	for _, outcome := range stage.Join.AcceptOutcomes {
		if !slices.Contains(body.Workflow.AllowedOutcomes, outcome) {
			return problem("invalid_join", path+"/join/accept_outcomes", "the body does not declare the accepted outcome "+outcome)
		}
	}
	// A sealed collection may legitimately be empty, and an empty fan-out
	// produces no branch results at all. The definition must say what that
	// means rather than let the join decide a verdict about nothing.
	if stage.On["empty"] == "" {
		return problem("unhandled_verdict", path+"/on/empty", "a map stage declares where an empty collection leads")
	}
	return nil
}

// CollectionPort returns the declared port a map's collection binding reads.
// The compiler already proved the value exists on every incoming path; this
// answers what shape the producer promised, so the bytes can be validated as
// that before a single item is cut from them.
func (p *Plan) CollectionPort(binding Binding) (Port, bool, error) {
	if binding.Pointer != nil {
		if binding.ProjectedSchemaRef == nil {
			return Port{}, false, problem("port_type_mismatch", "/items", "a projected collection declares its projected schema")
		}
		return Port{Format: "json", SchemaRef: binding.ProjectedSchemaRef}, true, nil
	}
	switch binding.From {
	case "workflow_input":
		input, exists := p.Workflow.Inputs[binding.Port]
		if !exists {
			return Port{}, false, problem("unknown_port", "/items/port", "workflow input does not exist")
		}
		return input.Port, true, nil
	case "stage_output":
		output, exists := p.StageOutputs(binding.StageID)[binding.Port]
		if !exists {
			return Port{}, false, problem("unknown_port", "/items/port", "producer output does not exist")
		}
		return output.Port, true, nil
	case "literal":
		source, err := p.literalPort(binding, "/items")
		return source, err == nil, err
	}
	return Port{}, false, problem("unsupported", "/items/from", "unsupported binding source")
}

// WaitEventPort is the only port a wait stage produces: the accepted event's
// payload. A timeout produces nothing, because no event arrived.
const WaitEventPort = "event"

// WaitPublicationPort is an internal dataflow fact, not a stage_output port.
// It becomes true only on a choice edge proving delivery.kind == Item.
const WaitPublicationPort = "publication"

// MaxWaitSeconds is the longest finite deadline a definition may declare. The
// published contract permits ten years; this build refuses to promise a wakeup
// it cannot own for longer than a year.
const MaxWaitSeconds int64 = 366 * 24 * 60 * 60

// checkWait validates what the schema cannot: that the source and the event
// schema are pinned, that the correlation key is readable, and that a finite
// deadline stays within what this build is prepared to hold.
func (p *Plan) checkWait(stage Stage, path string) error {
	if stage.SourceRef.ID == "" || stage.EventSchemaRef.ID == "" {
		return problem("invalid_wait", path, "a wait stage names its source and the schema of the event it accepts")
	}
	if _, err := p.schema(stage.EventSchemaRef); err != nil {
		return problem("invalid_schema_ref", path+"/event_schema_ref", err.Error())
	}
	if p.Registry[stage.SourceRef] == nil {
		return problem("missing_ref", path+"/source_ref", "the source that may resolve this wait is not pinned")
	}
	source, publication, err := publicationSourceAt(p.Registry[stage.SourceRef], path+"/source_ref@"+stage.SourceRef.ID)
	if err != nil {
		return err
	}
	if publication {
		p.publicationSources[stage.SourceRef] = source
		switch source.Mode {
		case "once":
			if stage.EventType != "artifact.published" {
				return problem("invalid_publication_source", path+"/event_type", "once artifact publication wait uses event type artifact.published")
			}
			if stage.EventSchemaRef != source.HookSchemaRef {
				return problem("port_type_mismatch", path+"/event_schema_ref", "wait and publication source require the same exact schema")
			}
			if stage.CursorInput != nil {
				return problem("invalid_publication_source", path+"/cursor_input", "once publication has no stream cursor")
			}
			if stage.CorrelationInput == nil || stage.CorrelationInput.From != "literal" || stage.CorrelationInput.Pointer != nil {
				return problem("invalid_publication_source", path+"/correlation_input", "the once subscription correlates its fixed item key with a literal")
			}
			value, parseErr := Parse(stage.CorrelationInput.Value, "json")
			if parseErr != nil || value != source.ItemKey {
				return problem("invalid_publication_source", path+"/correlation_input/value", "correlation literal must equal the declared publication item key")
			}
		case "each_publication":
			if stage.EventType != "artifact.publication" {
				return problem("invalid_publication_source", path+"/event_type", "each_publication wait uses event type artifact.publication")
			}
			if source.DeliverySchemaRef == nil || stage.EventSchemaRef != *source.DeliverySchemaRef {
				return problem("port_type_mismatch", path+"/event_schema_ref", "stream wait requires the exact delivery schema")
			}
			if stage.CorrelationInput == nil || stage.CursorInput == nil || stage.CorrelationInput.Pointer != nil || stage.CursorInput.Pointer != nil {
				return problem("invalid_publication_source", path+"/correlation_input", "each_publication wait requires whole typed handle and cursor inputs")
			}
		default:
			return problem("invalid_publication_source", path+"/source_ref", "publication source mode is unsupported")
		}
		if stage.TimeoutSeconds == nil {
			return problem("invalid_publication_source", path+"/timeout_seconds", "wait_until_timeout requires a finite wait deadline")
		}
	}
	if stage.CorrelationInput == nil {
		return problem("invalid_wait", path+"/correlation_input", "a wait stage names the business key that correlates its event")
	}
	// A finite deadline is a promise about time. An indefinite wait promises
	// nothing about it, which is why it is allowed to have no timeout route.
	if stage.TimeoutSeconds != nil {
		if *stage.TimeoutSeconds < 1 || *stage.TimeoutSeconds > MaxWaitSeconds {
			return problem("unsupported", path+"/timeout_seconds", fmt.Sprintf("this build holds a deadline for at most %d seconds", MaxWaitSeconds))
		}
		if stage.OnTimeout == "" {
			return problem("unhandled_verdict", path+"/on_timeout", "a wait with a deadline declares where expiry leads")
		}
	} else if stage.OnTimeout != "" {
		return problem("invalid_wait", path+"/on_timeout", "an indefinite wait cannot expire, so it has no expiry route")
	}
	if stage.OnEvent == "" {
		return problem("unhandled_verdict", path+"/on_event", "a wait stage declares where an accepted event leads")
	}
	return nil
}
