package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"

	"github.com/stenhigh/prifly/internal/flow"
)

// NextArrival names the accepted result that led to the current action: the
// stage it came from, the verdict or child outcome it took, and the outputs it
// left. A fresh executor reads why it is here from this, not from a
// conversation it never saw.
type NextArrival struct {
	InvocationID string `json:"workflow_invocation_id"`
	StageID      string `json:"stage_id"`
	// Verdict is what the stage routes by: a step's accepted verdict, a call's
	// child outcome, or a repeat's decision route (on_limit, on_complete,
	// on_unknown).
	Verdict string                 `json:"verdict"`
	Outputs map[string]ArtifactRef `json:"outputs"`
}

// RepeatPosition is one bounded repeat the current action runs inside: the
// iteration it is on, the limit that applies to this Run and where the
// workflow goes when it is reached.
type RepeatPosition struct {
	InvocationID string `json:"workflow_invocation_id"`
	StageID      string `json:"stage_id"`
	Iteration    int64  `json:"iteration"`
	Limit        int64  `json:"limit"`
	OnLimit      string `json:"on_limit"`
}

// arrivalAt names the one settled stage of an invocation whose accepted result
// the sealed plan routes to target. The rule is the driver's own routing, read
// again: a step by its verdict, a call by its child's outcome. Two stages
// declaring a route here with nothing in the state saying which was taken name
// none -- absent means not named, never that there was none.
func arrivalAt(r Run, p *flow.Plan, invocationID, target string) *NextArrival {
	var found *NextArrival
	for _, a := range r.Activations {
		if a.InvocationID != invocationID || a.StageID == target {
			continue
		}
		taken, next := "", ""
		var outputs map[string]ArtifactRef
		switch {
		case a.StepID != "":
			step := r.Steps[a.StepID]
			if step == nil || step.Verdict == "" {
				continue
			}
			routed, err := p.Next(a.StageID, step.Verdict)
			if err != nil {
				continue
			}
			taken, next, outputs = step.Verdict, routed, step.Outputs
		case a.Kind == "repeat":
			// A repeat routes by its last decision: on_limit, on_complete or
			// on_unknown. Its outputs are those of the body it decided on.
			if a.Repeat == nil || a.Repeat.LastDecision == nil {
				continue
			}
			decision := a.Repeat.LastDecision
			taken, next = decision.Route, decision.NextStageID
			if body := r.Invocations[decision.BodyInvocationID]; body != nil {
				outputs = body.Outputs
			}
		case a.Kind == "call":
			child := r.childForCall(a.ID)
			if child == nil || child.Outcome == nil {
				continue
			}
			routed, err := p.NextOutcome(a.StageID, *child.Outcome)
			if err != nil {
				continue
			}
			taken, next, outputs = *child.Outcome, routed, child.Outputs
		default:
			continue
		}
		if next != target {
			continue
		}
		if found != nil && (found.StageID != a.StageID || found.Verdict != taken) {
			return nil
		}
		if outputs == nil {
			outputs = map[string]ArtifactRef{}
		}
		found = &NextArrival{InvocationID: invocationID, StageID: a.StageID, Verdict: taken, Outputs: outputs}
	}
	return found
}

// repeatsAround lists the bounded repeats an invocation runs inside, outermost
// first, with the limit this Run applies -- a project may narrow the declared
// ceiling, and the ceiling alone would promise iterations that will not run.
func repeatsAround(r Run, invocationID string) ([]RepeatPosition, error) {
	lineage, plans, err := r.invocationPlans(invocationID)
	if err != nil {
		return nil, err
	}
	positions := []RepeatPosition{}
	for i := len(lineage) - 2; i >= 0; i-- {
		inv := lineage[i]
		caller := r.Activations[inv.CallerActivationID]
		if caller == nil || caller.Kind != "repeat" || inv.Iteration == nil {
			continue
		}
		parent := plans[i+1]
		limit, err := repeatLimit(parent, r.WorkflowConfigurations[parent.Digest], caller.StageID)
		if err != nil {
			return nil, err
		}
		positions = append(positions, RepeatPosition{InvocationID: caller.InvocationID, StageID: caller.StageID, Iteration: *inv.Iteration, Limit: limit, OnLimit: parent.Workflow.Definition.Stages[caller.StageID].OnLimit})
	}
	return positions, nil
}

// continuationsOf lists the workflows of installed, resolvable packages whose
// declared continuation takes this Run: its workflow is named, and it ended
// with a named outcome or was cancelled where cancellation is admitted. It is
// an index, not an admission -- project continue checks everything again --
// so only the two declared fields are read from bytes verified at install.
func (e *Engine) continuationsOf(ctx context.Context, r Run) ([]flow.Ref, error) {
	record, _, err := e.readPackages(ctx)
	if err != nil {
		return nil, err
	}
	resolvable := resolvablePackages(record)
	found := []flow.Ref{}
	for _, pkg := range record.Packages {
		if !resolvable[pkg.Ref] {
			continue
		}
		for _, component := range pkg.Components {
			if component.Kind != "workflow" {
				continue
			}
			data, err := os.ReadFile(filepath.Join(e.Root, component.Path))
			if err != nil {
				return nil, err
			}
			var declared struct {
				Continuation *flow.Continuation `json:"continuation"`
			}
			if err := json.Unmarshal(data, &declared); err != nil {
				return nil, err
			}
			c := declared.Continuation
			if c == nil || !slices.Contains(c.FromWorkflows, r.WorkflowRef.ID) {
				continue
			}
			ended := r.Status == "completed" && r.Outcome != nil && slices.Contains(c.FromOutcomes, *r.Outcome)
			if ended || c.FromCancelled && r.Status == "cancelled" {
				found = append(found, component.Ref)
			}
		}
	}
	slices.SortFunc(found, func(a, b flow.Ref) int { return compareRefs(a, b) })
	return slices.CompactFunc(found, func(a, b flow.Ref) bool { return a == b }), nil
}

func compareRefs(a, b flow.Ref) int {
	for _, pair := range [][2]string{{a.ID, b.ID}, {a.Version, b.Version}, {a.Digest, b.Digest}} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	return 0
}
