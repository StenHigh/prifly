package runtime

import (
	"context"
	"errors"
	"slices"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

func branchInvocationID(runID, activationID, branchID string) string {
	return derivedID("invocation", runID, activationID, branchID)
}

func joinDecisionID(runID, activationID, branchID string) string {
	return derivedID("decision", runID, activationID, branchID)
}

// checkFanOutActivation is the shared guard for both fan-out kinds. What
// differs is only where the branch identities come from: a parallel stage
// declares them in the pinned plan, and a map derives them from a collection it
// sealed. Everything else - ownership of the ready scope, the pinned parent,
// the join contract - is identical, and is checked once here.
func checkFanOutActivation(r Run, p *flow.Plan, a *Activation, status string) error {
	kind, stageID := nextKind(r)
	parentID, _ := r.readyScope()
	if a == nil || !isParallelState(r.SchemaVersion) || r.Profile != flow.CoreProfile || p.Profile != r.Profile || kind != "stage" || a.ID == "" || a.InvocationID != parentID || a.StageID != stageID || !fanOut(a.Kind) || a.Status != status || a.Settled != nil || a.StepID != "" || a.Parallel == nil {
		return local.Reject("parallel_blocked", "the fan-out no longer owns its ready scope")
	}
	parent, stage := r.Invocations[parentID], p.Workflow.Definition.Stages[stageID]
	if parent == nil || parent.WorkflowRef != planRef(p) || stage.Kind != a.Kind || stage.Join == nil {
		return local.Reject("stage_conflict", "fan-out activation or pinned definition differs")
	}
	if stage.Kind == "map" {
		return checkSealedCollection(r, stage, a)
	}
	if len(p.Branches[stageID]) != len(stage.ParallelBranches) {
		return local.Reject("stage_conflict", "parallel activation or pinned branches differ")
	}
	// The sealed order is the activation's own; a redefined stage does not
	// renumber branches this activation already owns.
	if len(a.Parallel.BranchIDs) != len(stage.ParallelBranches) {
		return local.Reject("stage_conflict", "sealed branch order differs from the pinned stage")
	}
	for i, branch := range stage.ParallelBranches {
		if a.Parallel.BranchIDs[i] != branch.ID || p.Branches[stageID][branch.ID] == nil {
			return local.Reject("stage_conflict", "sealed branch order differs from the pinned stage")
		}
	}
	return nil
}

// checkSealedCollection holds the map's own invariant: the branch identities
// are the sealed ones, in the sealed order, and nothing outside the seal can
// add to them. A changed source collection reaches no further than this.
func checkSealedCollection(r Run, stage flow.Stage, a *Activation) error {
	sealed := a.Parallel.Sealed
	if !isMapState(r.SchemaVersion) {
		return local.Reject("incompatible_run", "a sealed collection requires "+CoreMapStateVersion)
	}
	// An unentered map has sealed nothing yet; entry seals and enters at once.
	if len(sealed) == 0 {
		if a.Parallel.EnteredCount != 0 || len(a.Parallel.BranchIDs) != 0 {
			return local.Reject("stage_conflict", "a map entered branches it never sealed")
		}
		return nil
	}
	if len(sealed) > stage.MaxItems || len(sealed) > flow.MaxMapItems || len(a.Parallel.BranchIDs) != len(sealed) {
		return local.Reject("stage_conflict", "the sealed collection differs from what the stage admits")
	}
	seen := make(map[string]bool, len(sealed))
	for i, item := range sealed {
		if item.Key == "" || item.Position != int64(i) || a.Parallel.BranchIDs[i] != item.Key || seen[item.Key] {
			return local.Reject("stage_conflict", "the sealed item identities are not the branch identities")
		}
		seen[item.Key] = true
	}
	return nil
}

// createParallelBranch is part of entry or of a decision that continues to the
// next branch. Each pays its own control transition; creation never charges a
// second time for the same activation.
func (r *Run) createParallelBranch(a *Activation, branchID string, branch *flow.Plan, inputs map[string]ArtifactRef, obs Observation) (local.EventInput, error) {
	position := a.Parallel.EnteredCount
	id := branchInvocationID(r.ID, a.ID, branchID)
	if inputs == nil || r.Invocations[id] != nil || position >= int64(len(a.Parallel.BranchIDs)) || a.Parallel.BranchIDs[position] != branchID {
		return local.EventInput{}, local.ErrIntegrity
	}
	if err := r.setReadyFor(a.InvocationID, []string{}); err != nil {
		return local.EventInput{}, err
	}
	if err := r.setInvocationStatus(a.InvocationID, "waiting", nil); err != nil {
		return local.EventInput{}, err
	}
	a.Status = "waiting"
	r.Invocations[id] = &Invocation{ID: id, RunID: r.ID, ParentInvocationID: a.InvocationID, CallerActivationID: a.ID, BranchID: branchID, WorkflowRef: planRef(branch), Status: "ready", Inputs: inputs, Outputs: map[string]ArtifactRef{}, Ready: []string{branch.Workflow.Definition.Entry}, Created: obs}
	a.Parallel.EnteredCount, a.Parallel.CurrentBranchInvocationID = position+1, id
	if err := r.beginWorkflowInputAcceptance(branch, id, obs); err != nil {
		return local.EventInput{}, err
	}
	data, err := canonical(map[string]any{"workflow_invocation_id": id, "parent_invocation_id": a.InvocationID, "caller_stage_activation_id": a.ID, "workflow_ref": planRef(branch), "input_refs": inputs, "branch_id": branchID, "position": position + 1, "observation": obs})
	return local.EventInput{Type: "invocation.created", Version: 1, Data: data}, err
}

func (e *Engine) enterParallel(ctx context.Context, loaded Run, view local.ReadView, p *flow.Plan, activation *Activation) error {
	if err := checkFanOutActivation(loaded, p, activation, "ready"); err != nil {
		return err
	}
	stage := p.Workflow.Definition.Stages[activation.StageID]
	// A stage that declares simultaneity enters that many branches at once.
	// Entering one and waiting for it would make the declaration decorative.
	opening := min(stage.MaxParallelism, len(stage.ParallelBranches))
	commandID := newID("command")
	inputs := make([]map[string]ArtifactRef, 0, opening)
	for _, declared := range stage.ParallelBranches[:opening] {
		branch := p.Branches[activation.StageID][declared.ID]
		refs, err := e.prepareBodyInputs(loaded, activation.InvocationID, "", branch, declared.InputBindings, commandID, declared.ID)
		if err != nil {
			return e.failPreparation(ctx, loaded, view, p, activation, err, "branch_input_binding_failed")
		}
		inputs = append(inputs, refs)
	}
	opened := make([]string, 0, opening)
	for _, declared := range stage.ParallelBranches[:opening] {
		opened = append(opened, declared.ID)
	}
	payload := map[string]any{"stage_activation_id": activation.ID, "branch_ids": opened, "input_refs": inputs}
	_, err := e.apply(ctx, e.owner, commandID, loaded.ID, "stage.parallel_entered", payload, &view.Snapshot.Version, local.CommandCAS, func(r *Run, s local.Snapshot, obs Observation) (local.Change, error) {
		a := r.Activations[activation.ID]
		if err := checkFanOutActivation(*r, p, a, "ready"); err != nil {
			return local.Change{}, err
		}
		if a.Parallel.EnteredCount != 0 || a.Parallel.CurrentBranchInvocationID != "" || a.Parallel.LastDecision != nil {
			return local.Change{}, local.Reject("stage_conflict", "parallel already entered its first branch")
		}
		events := make([]local.EventInput, 0, opening+1)
		for i, declared := range stage.ParallelBranches[:opening] {
			created, err := r.createParallelBranch(a, declared.ID, p.Branches[a.StageID][declared.ID], inputs[i], obs)
			if err != nil {
				return local.Change{}, err
			}
			events = append(events, created)
		}
		data, err := canonical(map[string]any{"run_id": r.ID, "stage_activation_id": a.ID, "stage_id": a.StageID, "workflow_invocation_id": a.InvocationID, "branch_ids": a.Parallel.BranchIDs, "entered_branch_ids": opened, "join": stage.Join, "max_parallelism": stage.MaxParallelism, "observation": obs})
		if err != nil {
			return local.Change{}, err
		}
		return local.Change{RequireStorageBudget: true, Events: append([]local.EventInput{{Type: "stage.parallel_entered", Version: 1, Data: data}}, events...)}, nil
	})
	return err
}

func joinFailure(d *JoinDecision, p *flow.Plan, cause error, fallback string) {
	d.Route, d.NextStageID, d.NextBranchID, d.NextBranchInvocationID, d.Failure = "failed", "", "", "", driverFailureCode(cause, fallback)
	var problem *flow.Problem
	if errors.As(cause, &problem) {
		d.Failure, d.FailurePath = problem.Code, problem.Path
	}
	// A verdict the definition does not route, or a join that never decided, is
	// a gap in the contract. on_error handles technical failures; it is not an
	// implicit catch-all for a route the author never declared.
	if d.Failure != "unhandled_verdict" && d.Failure != "no_transition" && d.Failure != "join_undecided" {
		if next, err := p.NextError(d.StageID); err == nil {
			d.Route, d.NextStageID = "on_error", next
		}
	}
}

// evaluateJoin derives the decision for the settled current branch. Counts come
// from the branches that actually settled, so the verdict is a function of
// recorded results rather than of anything carried forward by hand.
func (e *Engine) evaluateJoin(r Run, p *flow.Plan, a *Activation) (JoinDecision, error) {
	if err := checkFanOutActivation(r, p, a, "waiting"); err != nil {
		return JoinDecision{}, err
	}
	entered, err := r.parallelBranches(a.ID)
	if err != nil {
		return JoinDecision{}, err
	}
	stage := p.Workflow.Definition.Stages[a.StageID]
	// The total is the sealed order's own, not the stage's declaration: a map
	// declares a cap and seals the branches it actually got.
	join, total := *stage.Join, int64(len(a.Parallel.BranchIDs))
	current, position := settledUndecidedBranch(a, entered)
	if current == nil {
		return JoinDecision{}, local.Reject("parallel_blocked", "no settled branch is waiting for its decision")
	}
	// Counts follow the branches folded in, including this one. Several may be
	// live, so a position says nothing about how many have been observed.
	decided := map[string]bool{}
	for _, id := range a.Parallel.DecidedBranchIDs {
		decided[id] = true
	}
	accepted, observed := int64(0), int64(len(a.Parallel.DecidedBranchIDs))+1
	for _, inv := range entered {
		if !decided[inv.BranchID] && inv != current {
			continue
		}
		if inv.Status == "completed" && inv.Outcome != nil && slices.Contains(join.AcceptOutcomes, *inv.Outcome) {
			accepted++
		}
	}
	d := JoinDecision{SchemaVersion: JoinDecisionVersion, ID: joinDecisionID(r.ID, a.ID, current.BranchID), RunID: r.ID, InvocationID: a.InvocationID, ActivationID: a.ID, StageID: a.StageID, WorkflowRef: planRef(p), Mode: join.Mode, Selection: join.Selection, Remainder: join.Remainder, BranchID: current.BranchID, BranchInvocationID: current.ID, Position: position, BranchCount: total, BranchStatus: current.Status, BranchOutcome: current.Outcome, AcceptedCount: accepted, ObservedCount: observed}
	d.Accepted = current.Status == "completed" && current.Outcome != nil && slices.Contains(join.AcceptOutcomes, *current.Outcome)

	// A branch stopped because the join no longer needed it is neither an
	// unsatisfied outcome nor a failure: it is a branch the contract released.
	if current.Status == "cancelled" {
		d.Verdict = joinVerdict(join, accepted, observed, total)
		live, unentered := int64(0), total-int64(len(entered))
		for _, inv := range entered {
			if inv.Settled == nil {
				live++
			}
		}
		if d.Verdict != "undecided" && live == 0 {
			d.Route = d.Verdict
			if observed < total || unentered > 0 {
				d.RemainderDisposition = "cancelled"
			}
			next, err := p.NextJoin(a.StageID, d.Verdict)
			if err != nil {
				joinFailure(&d, p, err, "no_transition")
				return d, nil
			}
			d.NextStageID = next
			return d, nil
		}
		d.Route, d.RemainderDisposition = "cancelling", "cancel_requested"
		return d, nil
	}
	// A branch that failed produced no outcome at all. That is a technical
	// failure of a child, not an unsatisfied join, and it is routed as one.
	if current.Status == "failed" {
		d.Verdict = "undecided"
		joinFailure(&d, p, nil, "branch_failed")
		return d, nil
	}
	branch := p.BranchPlan(a.StageID, current.BranchID)
	if branch == nil || current.Outcome == nil || !slices.Contains(branch.Workflow.AllowedOutcomes, *current.Outcome) {
		return JoinDecision{}, local.ErrIntegrity
	}
	d.Verdict = joinVerdict(join, accepted, observed, total)

	// A branch still running is work with an owner, so the join may not settle
	// over it. remainder=wait keeps entering the declared branches even once
	// the verdict is known; remainder=cancel stops entering them, and a branch
	// never entered has no work to abandon.
	live := int64(0)
	for _, inv := range entered {
		if inv.Settled == nil {
			live++
		}
	}
	unentered := total - int64(len(entered))
	settled := d.Verdict != "undecided" && live == 0 && (observed == total || join.Remainder == "cancel")
	// A verdict reached over branches that are still running does not settle the
	// join: the remainder is asked to stop and the join waits for that to be
	// confirmed. Requesting a stop is not the same as having stopped, so the
	// transition and the summary are held until every branch is terminal.
	if !settled && d.Verdict != "undecided" && join.Remainder == "cancel" && live > 0 {
		d.Route, d.RemainderDisposition = "cancelling", "cancel_requested"
		return d, nil
	}
	if !settled {
		if unentered > 0 && (d.Verdict == "undecided" || join.Remainder == "wait") && live < int64(stage.MaxParallelism) {
			d.Route = "next_branch"
			d.NextBranchID = a.Parallel.BranchIDs[len(entered)]
			d.NextBranchInvocationID = branchInvocationID(r.ID, a.ID, d.NextBranchID)
			return d, nil
		}
		if observed == total && d.Verdict == "undecided" {
			// Every branch was folded in without deciding the contract. The
			// join declares no third answer, so this is a contract failure.
			joinFailure(&d, p, nil, "join_undecided")
			return d, nil
		}
		if observed < total {
			// The result is recorded; the join waits for the rest. What matters
			// is that branches remain to be folded in, not whether they are
			// still running: with several entered at once they can all settle
			// before the first decision is taken, and the join must still wait
			// for their decisions rather than route an undecided verdict.
			d.Route = "recorded"
			return d, nil
		}
	}
	d.Route = d.Verdict
	if observed < total {
		d.RemainderDisposition = "not_entered"
	}
	next, err := p.NextJoin(a.StageID, d.Verdict)
	if err != nil {
		joinFailure(&d, p, err, "no_transition")
		return d, nil
	}
	d.NextStageID = next
	return d, nil
}

// aggregateManifest describes what the branches produced. It reports only the
// branches that were actually entered: one that was never entered has no
// invocation and no result, so inventing a row for it would be a fact nobody
// observed. selection_seq is the position in the sealed order at which the
// verdict was fixed, which is the durable sequence this profile observes in.
func (e *Engine) aggregateManifest(r Run, p *flow.Plan, a *Activation, d JoinDecision) ([]byte, error) {
	entered, err := r.parallelBranches(a.ID)
	if err != nil {
		return nil, err
	}
	join := *p.Workflow.Definition.Stages[a.StageID].Join
	rows := make([]any, 0, len(entered))
	selected := []any{}
	accepted := int64(0)
	for _, inv := range entered {
		row := map[string]any{
			"id": inv.BranchID, "run_id": r.ID, "workflow_invocation_id": inv.ID,
			"status": inv.Status, "outcome": inv.Outcome,
			"has_waivers": r.waiverAppliedWithin(inv.ID),
			"output_refs": inv.Outputs, "effect_receipt_refs": []any{},
		}
		rows = append(rows, row)
		if d.Verdict != "satisfied" || inv.Status != "completed" || inv.Outcome == nil || !slices.Contains(join.AcceptOutcomes, *inv.Outcome) {
			continue
		}
		// A quorum counts the first qualifying branches in the sealed order;
		// mode all counts every accepted branch.
		if join.Mode == "quorum" && accepted >= int64(join.RequiredSuccesses) {
			continue
		}
		accepted++
		selected = append(selected, inv.BranchID)
	}
	manifest := map[string]any{
		"schema_version": "1", "stage_activation_id": a.ID, "selection_seq": d.Position,
		"selected_branch_ids": selected, "join_result": d.Verdict, "branches": rows,
	}
	encoded, err := canonical(manifest)
	if err != nil {
		return nil, err
	}
	return encoded, flow.ValidateProtocol("AggregateManifest", encoded)
}

// settledUndecidedBranch is the branch whose result is waiting to be folded in:
// the earliest settled one in the sealed order that has no decision yet. Order
// is the sealed one, so which branch is decided next never depends on timing.
func settledUndecidedBranch(a *Activation, entered []*Invocation) (*Invocation, int64) {
	decided := map[string]bool{}
	for _, id := range a.Parallel.DecidedBranchIDs {
		decided[id] = true
	}
	for i, inv := range entered {
		if decided[inv.BranchID] || inv.Settled == nil {
			continue
		}
		// A cancelled branch settled too. It contributed no outcome, so it is
		// folded in as what it is rather than left out of the account.
		if inv.Status != "completed" && inv.Status != "failed" && inv.Status != "cancelled" {
			continue
		}
		return inv, int64(i + 1)
	}
	return nil, 0
}

func (e *Engine) decideJoin(ctx context.Context, r Run, view local.ReadView, p *flow.Plan, a *Activation) error {
	d, err := e.evaluateJoin(r, p, a)
	if err != nil {
		return err
	}
	commandID := newID("command")
	var results *ArtifactRef
	if d.Route == "satisfied" || d.Route == "unsatisfied" {
		encoded, err := e.aggregateManifest(r, p, a, d)
		if err != nil {
			return err
		}
		schema := builtinRef(r.Definitions, flow.AggregateSchemaID)
		if schema.Digest == "" {
			return local.Reject("missing_ref", "this Run pinned no summary form for a parallel stage")
		}
		producer := map[string]any{"kind": "authority", "authority_id": r.AuthorityID, "command_id": commandID, "port": flow.AggregateResultsPort}
		artifact, err := e.putArtifact(encoded, "json", &schema, derivedID("artifact", r.ID, a.ID, "results"), producer, []ArtifactRef{}, r.registry())
		if err != nil {
			return err
		}
		ref := artifact.Ref()
		results = &ref
	}
	var inputs map[string]ArtifactRef
	if d.Route == "next_branch" {
		stage := p.Workflow.Definition.Stages[a.StageID]
		bindings := stage.InputBindings
		if stage.Kind == "parallel" {
			index := slices.IndexFunc(stage.ParallelBranches, func(branch flow.ParallelBranch) bool { return branch.ID == d.NextBranchID })
			if index < 0 {
				return local.ErrIntegrity
			}
			bindings = stage.ParallelBranches[index].InputBindings
		}
		var supplied []string
		if stage.Kind == "map" {
			supplied = []string{stage.ItemInput}
		}
		inputs, err = e.prepareBodyInputs(r, a.InvocationID, "", p.BranchPlan(a.StageID, d.NextBranchID), bindings, commandID, d.NextBranchID, supplied...)
		if err == nil && stage.Kind == "map" {
			// The item comes from the seal, not from a binding: the collection
			// is not read again, so a source that changed since cannot reach
			// the branch this decision is about to open.
			item, err := a.Parallel.sealedItem(d.NextBranchID)
			if err != nil {
				return err
			}
			inputs[stage.ItemInput] = item.Ref
		}
		if err != nil {
			joinFailure(&d, p, err, "branch_input_binding_failed")
			inputs = nil
		}
	}
	_, err = e.commitJoin(ctx, r, view, p, a, commandID, d, inputs, results)
	return err
}

func (e *Engine) commitJoin(ctx context.Context, loaded Run, view local.ReadView, p *flow.Plan, activation *Activation, commandID string, decision JoinDecision, nextInputs map[string]ArtifactRef, results *ArtifactRef) (local.ApplyResult, error) {
	payload := map[string]any{"decision": decision, "next_input_refs": nextInputs, "results_ref": results}
	return e.apply(ctx, e.owner, commandID, loaded.ID, "stage.join_decided", payload, &view.Snapshot.Version, local.CommandCAS, func(r *Run, s local.Snapshot, obs Observation) (local.Change, error) {
		if activation == nil {
			return local.Change{}, local.ErrIntegrity
		}
		a := r.Activations[activation.ID]
		if err := checkFanOutActivation(*r, p, a, "waiting"); err != nil {
			return local.Change{}, err
		}
		entered, err := r.parallelBranches(a.ID)
		if err != nil {
			return local.Change{}, err
		}
		stage := p.Workflow.Definition.Stages[a.StageID]
		current, position := settledUndecidedBranch(a, entered)
		if current == nil || current.WorkflowRef != planRef(p.BranchPlan(a.StageID, current.BranchID)) {
			return local.Change{}, local.Reject("parallel_blocked", "no settled branch of this pinned stage is waiting for its decision")
		}
		if decision.SchemaVersion != JoinDecisionVersion || decision.ID != joinDecisionID(r.ID, a.ID, current.BranchID) || decision.RunID != r.ID || decision.InvocationID != a.InvocationID || decision.ActivationID != a.ID || decision.StageID != a.StageID || decision.WorkflowRef != planRef(p) || decision.BranchInvocationID != current.ID || decision.BranchID != current.BranchID || decision.Position != position || decision.BranchStatus != current.Status || (decision.BranchOutcome == nil) != (current.Outcome == nil) || decision.BranchOutcome != nil && *decision.BranchOutcome != *current.Outcome || decision.Observed != (Observation{}) {
			return local.Change{}, local.Reject("stage_conflict", "join decision does not belong to this branch")
		}
		if err := checkJoinRoute(stage, *a.Parallel, entered, decision); err != nil {
			return local.Change{}, err
		}
		if decision.Route == "next_branch" {
			if decision.NextBranchInvocationID != branchInvocationID(r.ID, a.ID, decision.NextBranchID) || nextInputs == nil {
				return local.Change{}, local.Reject("stage_conflict", "join continuation has no exact next branch and inputs")
			}
		} else if decision.NextBranchInvocationID != "" || nextInputs != nil {
			return local.Change{}, local.Reject("stage_conflict", "only a continuation creates a branch")
		}
		if err := r.chargeInvocation(a.InvocationID, 1, 0); err != nil {
			return local.Change{}, err
		}
		decision.Observed = obs
		a.Parallel.DecidedBranchIDs = append(a.Parallel.DecidedBranchIDs, decision.BranchID)
		var created, errorEvent local.EventInput
		if decision.Route == "cancelling" {
			// The verdict is fixed, so the branches it no longer needs are asked
			// to stop. Their own settlement, not this request, releases the join.
			for _, inv := range entered {
				if inv.Settled == nil {
					inv.CancelRequested = true
				}
			}
			if err := r.setReadyFor(a.InvocationID, []string{}); err != nil {
				return local.Change{}, err
			}
			if err := r.setInvocationStatus(a.InvocationID, "waiting", nil); err != nil {
				return local.Change{}, err
			}
		} else if decision.Route == "recorded" {
			// The branch is folded in and the join keeps waiting. It holds no
			// frontier meanwhile: a scope that is ready with nothing to do
			// would be handed to the driver again and again.
			if err := r.setReadyFor(a.InvocationID, []string{}); err != nil {
				return local.Change{}, err
			}
			if err := r.setInvocationStatus(a.InvocationID, "waiting", nil); err != nil {
				return local.Change{}, err
			}
		} else if decision.Route == "next_branch" {
			created, err = r.createParallelBranch(a, decision.NextBranchID, p.BranchPlan(a.StageID, decision.NextBranchID), nextInputs, obs)
			if err != nil {
				return local.Change{}, err
			}
		} else {
			settled := decision.Route == "satisfied" || decision.Route == "unsatisfied"
			if settled != (results != nil) {
				return local.Change{}, local.Reject("stage_conflict", "a settled join reports exactly one summary")
			}
			a.Parallel.ResultsRef = results
			a.Status, a.Settled = "completed", &obs
			if decision.Failure != "" {
				a.Status = "failed"
				if err := r.failInvocation(a.InvocationID, obs); err != nil {
					return local.Change{}, err
				}
				if err := recordDiagnostic(r, Diagnostic{ID: derivedID("diagnostic", decision.ID, decision.Failure), RunID: r.ID, ActivationID: a.ID, Origin: "core", Severity: "error", Code: decision.Failure, Category: "workflow", Phase: "parallel", Message: "Parallel failed; its decision identifies the settled branch and the counts it observed", Observed: obs, CauseRefs: []string{decision.ID, current.ID}}); err != nil {
					return local.Change{}, err
				}
				if decision.Route == "on_error" {
					var handled bool
					errorEvent, handled, err = routeKnownError(r, p, a.ID, "", decision.Failure, obs)
					if err != nil {
						return local.Change{}, err
					}
					ready := r.readyFor(a.InvocationID)
					if !handled || len(ready) != 1 || ready[0] != decision.NextStageID {
						return local.Change{}, local.Reject("stage_conflict", "join error route differs from the pinned plan")
					}
				}
			} else if err := r.advanceInvocation(a.InvocationID, decision.NextStageID); err != nil {
				return local.Change{}, err
			}
		}
		a.Parallel.LastDecision = &decision
		data, err := canonical(decision)
		change := local.Change{RequireStorageBudget: decision.Route == "next_branch" || decision.NextStageID != "", Events: []local.EventInput{{Type: "stage.join_decided", Version: 1, Data: data}}}
		if created.Type != "" {
			change.Events = append(change.Events, created)
		}
		if errorEvent.Type != "" {
			change.Events = append(change.Events, errorEvent)
		}
		return change, err
	})
}

// checkJoinRoute re-derives the verdict from the decision's own counts and
// compares the route against the pinned stage. A decision that claims a route
// its counts do not produce is refused rather than trusted.
func checkJoinRoute(stage flow.Stage, progress ParallelProgress, entered []*Invocation, d JoinDecision) error {
	join := stage.Join
	total := int64(len(progress.BranchIDs))
	live, unentered := int64(0), total-int64(len(entered))
	for _, inv := range entered {
		if inv.Settled == nil {
			live++
		}
	}
	valid := join != nil && d.BranchCount == total && d.Position >= 1 && d.Position <= total &&
		d.ObservedCount == int64(len(progress.DecidedBranchIDs))+1 && d.AcceptedCount >= 0 && d.AcceptedCount <= d.ObservedCount
	valid = valid && join != nil && d.Mode == join.Mode && d.Selection == join.Selection && d.Remainder == join.Remainder
	if d.BranchStatus == "cancelled" {
		valid = valid && d.BranchOutcome == nil && !d.Accepted
	} else if d.BranchStatus == "failed" {
		valid = valid && d.BranchOutcome == nil && !d.Accepted && d.Verdict == "undecided" && d.Failure == "branch_failed" && (d.Route == "failed" || d.Route == "on_error")
	} else {
		valid = valid && d.BranchStatus == "completed" && d.BranchOutcome != nil && d.Accepted == slices.Contains(join.AcceptOutcomes, *d.BranchOutcome)
		if valid && d.Failure == "" {
			valid = d.Verdict == joinVerdict(*join, d.AcceptedCount, d.ObservedCount, total)
		}
	}
	switch d.Route {
	case "next_branch":
		valid = valid && unentered > 0 && d.Failure == "" && d.NextStageID == "" && d.RemainderDisposition == "" &&
			(d.Verdict == "undecided" || join.Remainder == "wait") && live < int64(stage.MaxParallelism) &&
			d.NextBranchID == progress.BranchIDs[len(entered)]
	case "recorded":
		// Folding a branch in without settling is admissible only while the
		// join still has decisions to receive. Branches that already settled
		// but have not been decided count: they still owe the join a decision.
		valid = valid && d.Failure == "" && d.NextStageID == "" && d.NextBranchID == "" && d.RemainderDisposition == "" &&
			d.ObservedCount < total && !(d.ObservedCount == total && d.Verdict != "undecided")
	case "cancelling":
		// A fixed verdict over branches still running: the remainder is asked to
		// stop and nothing settles until it has.
		valid = valid && d.Verdict != "undecided" && join.Remainder == "cancel" && live > 0 &&
			d.Failure == "" && d.NextStageID == "" && d.NextBranchID == "" && d.RemainderDisposition == "cancel_requested"
	case "satisfied", "unsatisfied":
		// Every branch is terminal. What was left out of the account is named:
		// never entered, or entered and stopped once the verdict was fixed.
		valid = valid && d.Verdict == d.Route && d.Failure == "" && d.NextStageID != "" && d.NextStageID == stage.On[d.Route] &&
			live == 0 && (d.ObservedCount == total || join.Remainder == "cancel" &&
			(d.RemainderDisposition == "not_entered" || d.RemainderDisposition == "cancelled")) &&
			(d.ObservedCount == total) == (d.RemainderDisposition == "")
	case "on_error":
		valid = valid && d.Failure != "" && d.NextStageID != "" && d.NextStageID == stage.OnError &&
			d.Failure != "unhandled_verdict" && d.Failure != "no_transition" && d.Failure != "join_undecided"
	case "failed":
		valid = valid && d.Failure != "" && d.NextStageID == ""
	default:
		valid = false
	}
	if !valid || d.Failure == "" && d.FailurePath != "" {
		return local.Reject("stage_conflict", "join decision route differs from the pinned plan")
	}
	return nil
}
