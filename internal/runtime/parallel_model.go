package runtime

import (
	"fmt"

	"github.com/stenhigh/prifly/internal/flow"
)

const JoinDecisionVersion = "join-decision/1"

// ParallelProgress is the materialized frontier of one parallel activation.
// Branches are entered in declared order, one at a time: the qualified local
// profile advances a single frontier, so the fan-out is real while its
// simultaneity stays bounded. Only the latest decision is retained here; every
// branch and every earlier decision remain in the invocation tree and journal.
type ParallelProgress struct {
	// BranchIDs is the declared order, sealed when the stage is entered. The
	// pinned plan decides it once; a later definition may not renumber the
	// branches this activation already owns.
	BranchIDs    []string `json:"branch_ids"`
	EnteredCount int64    `json:"entered_count"`
	// DecidedBranchIDs are the branches already folded into the join, in the
	// order their decisions were committed. With several branches live at once
	// there is no "current" branch for that order to follow from.
	DecidedBranchIDs          []string      `json:"decided_branch_ids"`
	CurrentBranchInvocationID string        `json:"current_branch_workflow_invocation_id,omitempty"`
	LastDecision              *JoinDecision `json:"last_decision,omitempty"`
	// ResultsRef is the sealed summary of the branches, produced when the join
	// settles. It exists only for a settled verdict: a failed stage produced no
	// join result, so it has no summary to report.
	ResultsRef *ArtifactRef `json:"results_ref,omitempty"`
	// Sealed is the map's collection as it was when the stage was entered: one
	// entry per item, in collection order, each naming its derived artifact.
	// A parallel stage declares its branches in the plan and seals nothing, so
	// this is absent there and a run without a map is unchanged by its
	// existence.
	Sealed []SealedItem `json:"sealed,omitempty"`
}

// JoinDecision records what one exact settled branch contributed to the join
// and what follows from it. It is evidence about that branch, not an
// instruction to run anything again. Counts describe the branches observed so
// far, so the verdict never depends on a value carried forward by hand.
type JoinDecision struct {
	SchemaVersion          string   `json:"schema_version"`
	ID                     string   `json:"id"`
	RunID                  string   `json:"run_id"`
	InvocationID           string   `json:"workflow_invocation_id"`
	ActivationID           string   `json:"stage_activation_id"`
	StageID                string   `json:"stage_id"`
	WorkflowRef            flow.Ref `json:"workflow_ref"`
	Mode                   string   `json:"mode"`
	Selection              string   `json:"selection"`
	Remainder              string   `json:"remainder"`
	BranchID               string   `json:"branch_id"`
	BranchInvocationID     string   `json:"branch_workflow_invocation_id"`
	Position               int64    `json:"position"`
	BranchCount            int64    `json:"branch_count"`
	BranchStatus           string   `json:"branch_status"`
	BranchOutcome          *string  `json:"branch_outcome"`
	Accepted               bool     `json:"accepted"`
	AcceptedCount          int64    `json:"accepted_count"`
	ObservedCount          int64    `json:"observed_count"`
	Verdict                string   `json:"verdict"`
	Route                  string   `json:"route"`
	NextBranchID           string   `json:"next_branch_id,omitempty"`
	NextBranchInvocationID string   `json:"next_branch_workflow_invocation_id,omitempty"`
	NextStageID            string   `json:"next_stage_id,omitempty"`
	// RemainderDisposition names what happened to the branches this decision
	// left out. Sequential entry never leaves work running without an owner, so
	// the honest word is that they were never entered, not that they were cancelled.
	RemainderDisposition string      `json:"remainder_disposition,omitempty"`
	Failure              string      `json:"failure,omitempty"`
	FailurePath          string      `json:"failure_path,omitempty"`
	Observed             Observation `json:"observation"`
}

// isParallelState covers every version that can own branch invocations. A map
// is a fan-out, so its own version admits parallel stages too.
func isParallelState(version string) bool {
	return version == CoreParallelStateVersion || isMapState(version)
}

// isMapState is narrower: from this version on, a Run can hold a sealed
// collection. Later versions carry everything the earlier ones carried.
func isMapState(version string) bool { return version == CoreMapStateVersion || isWaitState(version) }

// fanOut names the two stage kinds that own branch invocations and settle on a
// join. Everything after entry is the same for both.
func fanOut(kind string) bool { return kind == "parallel" || kind == "map" }

// parallelBranches returns the entered branch invocations in declared order.
// It validates saved identities without compiling definitions or reading
// artifacts, and never repairs a counter or invents a missing member.
func (r Run) parallelBranches(activationID string) ([]*Invocation, error) {
	a := r.Activations[activationID]
	if !isParallelState(r.SchemaVersion) || a == nil || a.ID != activationID || !fanOut(a.Kind) || a.StepID != "" || a.Parallel == nil {
		return nil, fmt.Errorf("parallel invariant: missing parallel activation")
	}
	progress := a.Parallel
	// A map has no branches until it seals its collection, and a collection
	// that turned out to be empty never gets any. Neither is a broken
	// invariant: what would be broken is a branch without a declared identity.
	if len(progress.BranchIDs) == 0 {
		if progress.EnteredCount != 0 || progress.CurrentBranchInvocationID != "" || progress.LastDecision != nil {
			return nil, fmt.Errorf("parallel invariant: entered a branch that was never sealed")
		}
		if a.Kind == "parallel" {
			return nil, fmt.Errorf("parallel invariant: a parallel stage declares its branches")
		}
		return []*Invocation{}, nil
	}
	if len(progress.BranchIDs) > flow.MaxParallelBranches || progress.EnteredCount < 0 || progress.EnteredCount > int64(len(progress.BranchIDs)) {
		return nil, fmt.Errorf("parallel invariant: entered count is outside the sealed branches")
	}
	order := make(map[string]int, len(progress.BranchIDs))
	for i, id := range progress.BranchIDs {
		if _, duplicate := order[id]; duplicate || id == "" {
			return nil, fmt.Errorf("parallel invariant: sealed branch order is not a set of names")
		}
		order[id] = i
	}
	entered := make([]*Invocation, int(progress.EnteredCount))
	for id, inv := range r.Invocations {
		if inv == nil || inv.CallerActivationID != a.ID {
			continue
		}
		index, declaredBranch := order[inv.BranchID]
		if id != inv.ID || inv.RunID != r.ID || inv.ParentInvocationID != a.InvocationID || inv.Iteration != nil || inv.BranchID == "" || !declaredBranch || index >= len(entered) {
			return nil, fmt.Errorf("parallel invariant: branch owner or identity changed")
		}
		if entered[index] != nil {
			return nil, fmt.Errorf("parallel invariant: duplicate branch identity")
		}
		entered[index] = inv
	}
	for i, inv := range entered {
		if inv == nil {
			return nil, fmt.Errorf("parallel invariant: branch history is not contiguous")
		}
		// Several branches may be live at once, so an earlier one being
		// unsettled says nothing: only entry order is contiguous.
		_ = i
	}
	if len(entered) == 0 {
		if progress.CurrentBranchInvocationID != "" || progress.LastDecision != nil {
			return nil, fmt.Errorf("parallel invariant: unentered parallel has a branch or decision")
		}
	} else if progress.CurrentBranchInvocationID != entered[len(entered)-1].ID {
		return nil, fmt.Errorf("parallel invariant: current branch is not the last entered one")
	}
	return entered, nil
}

func (r Run) currentBranchFor(activationID string) *Invocation {
	entered, err := r.parallelBranches(activationID)
	if err != nil || len(entered) == 0 {
		return nil
	}
	return entered[len(entered)-1]
}

// joinVerdict derives the verdict from counts alone, so a verdict reached early
// under remainder=wait stays the same as later branches settle. It never
// depends on a value carried by hand from an earlier decision.
func joinVerdict(join flow.Join, accepted, observed, total int64) string {
	if join.Mode == "quorum" {
		required := int64(join.RequiredSuccesses)
		switch {
		case accepted >= required:
			return "satisfied"
		case accepted+total-observed < required:
			return "unsatisfied"
		}
		return "undecided"
	}
	switch {
	case accepted < observed:
		return "unsatisfied"
	case observed == total:
		return "satisfied"
	}
	return "undecided"
}

// parallelProgressInvariant checks the bounded projection against the branches
// that exist. It never repairs a counter, guesses a missing branch result or
// recovers a decision by evaluating the join again.
func (r Run) parallelProgressInvariant(a *Activation) error {
	if a == nil {
		return fmt.Errorf("parallel invariant: missing activation")
	}
	if !fanOut(a.Kind) {
		if a.Parallel != nil {
			return fmt.Errorf("parallel invariant: progress on another stage kind")
		}
		return nil
	}
	entered, err := r.parallelBranches(a.ID)
	if err != nil {
		return err
	}
	decided := map[string]bool{}
	for _, id := range a.Parallel.DecidedBranchIDs {
		if decided[id] {
			return fmt.Errorf("parallel invariant: a branch was decided twice")
		}
		decided[id] = true
	}
	for _, id := range a.Parallel.DecidedBranchIDs {
		found := false
		for _, inv := range entered {
			if inv.BranchID != id {
				continue
			}
			found = true
			if inv.Settled == nil {
				return fmt.Errorf("parallel invariant: a live branch was decided")
			}
		}
		if !found {
			return fmt.Errorf("parallel invariant: a decision names a branch that was never entered")
		}
	}
	d := a.Parallel.LastDecision
	if d == nil {
		if len(a.Parallel.DecidedBranchIDs) != 0 {
			return fmt.Errorf("parallel invariant: decisions were recorded without the latest one")
		}
		return nil
	}
	if len(a.Parallel.DecidedBranchIDs) == 0 || a.Parallel.DecidedBranchIDs[len(a.Parallel.DecidedBranchIDs)-1] != d.BranchID {
		return fmt.Errorf("parallel invariant: the latest decision is not the last one recorded")
	}
	parent := r.Invocations[a.InvocationID]
	if parent == nil || d.SchemaVersion != JoinDecisionVersion || d.ID == "" || d.RunID != r.ID || d.InvocationID != a.InvocationID || d.ActivationID != a.ID || d.StageID != a.StageID || d.WorkflowRef != parent.WorkflowRef {
		return fmt.Errorf("parallel invariant: decision identity changed")
	}
	if d.Position < 1 || d.Position > int64(len(entered)) || d.BranchCount != int64(len(a.Parallel.BranchIDs)) {
		return fmt.Errorf("parallel invariant: decision position is outside the entered branches")
	}
	branch := entered[d.Position-1]
	if d.BranchID != branch.BranchID || d.BranchInvocationID != branch.ID || d.BranchStatus != branch.Status || (d.BranchOutcome == nil) != (branch.Outcome == nil) || d.BranchOutcome != nil && *d.BranchOutcome != *branch.Outcome {
		return fmt.Errorf("parallel invariant: decision branch differs from the retained result")
	}
	if branch.Settled == nil || d.BranchStatus != "completed" && d.BranchStatus != "failed" && d.BranchStatus != "cancelled" || (d.BranchStatus == "completed") != (d.BranchOutcome != nil) {
		return fmt.Errorf("parallel invariant: decision requires a settled branch result")
	}
	if d.ObservedCount != int64(len(a.Parallel.DecidedBranchIDs)) || d.AcceptedCount < 0 || d.AcceptedCount > d.ObservedCount || d.Accepted && d.BranchStatus != "completed" {
		return fmt.Errorf("parallel invariant: decision counts differ from the branches folded in")
	}
	if d.Observed.UTC == "" || d.Observed.Session == "" || d.Observed.MonotonicMS < 0 {
		return fmt.Errorf("parallel invariant: decision lacks a committed observation")
	}
	switch d.Verdict {
	case "satisfied", "unsatisfied", "undecided":
	default:
		return fmt.Errorf("parallel invariant: unknown join verdict")
	}
	if a.Parallel.ResultsRef != nil && (d.Route == "next_branch" || a.Settled == nil) {
		return fmt.Errorf("parallel invariant: an unsettled join reported a summary")
	}
	if d.Route == "next_branch" {
		if d.NextBranchInvocationID != entered[len(entered)-1].ID || d.NextBranchID != entered[len(entered)-1].BranchID || d.NextStageID != "" || d.Failure != "" || d.RemainderDisposition != "" {
			return fmt.Errorf("parallel invariant: continuation does not identify the next branch")
		}
		return nil
	}
	// A cancelling decision fixed the verdict and asked the remainder to stop.
	// It settles nothing: a stop requested is not a stop confirmed.
	if d.Route == "cancelling" {
		if d.NextBranchID != "" || d.NextBranchInvocationID != "" || d.NextStageID != "" || d.Failure != "" || a.Settled != nil || a.Parallel.ResultsRef != nil {
			return fmt.Errorf("parallel invariant: a cancelling decision created or settled something")
		}
		if d.Verdict == "undecided" || d.RemainderDisposition != "cancel_requested" {
			return fmt.Errorf("parallel invariant: a cancelling decision names no fixed verdict or request")
		}
		return nil
	}
	// A recorded decision folds one branch in and leaves the join running: it
	// creates nothing and settles nothing, which is the honest shape when the
	// remaining branches are already live.
	if d.Route == "recorded" {
		if d.NextBranchID != "" || d.NextBranchInvocationID != "" || d.NextStageID != "" || d.Failure != "" || d.RemainderDisposition != "" || a.Settled != nil || a.Parallel.ResultsRef != nil {
			return fmt.Errorf("parallel invariant: a recorded decision created or settled something")
		}
		if d.Verdict == "satisfied" || d.Verdict == "unsatisfied" {
			if d.ObservedCount == d.BranchCount {
				return fmt.Errorf("parallel invariant: a decided join was left unsettled")
			}
		}
		return nil
	}
	if d.NextBranchInvocationID != "" || d.NextBranchID != "" {
		return fmt.Errorf("parallel invariant: a terminal decision creates no branch")
	}
	switch d.Route {
	case "satisfied", "unsatisfied":
		if d.NextStageID == "" || d.Verdict != d.Route || d.Failure != "" || d.FailurePath != "" || a.Status != "completed" || a.Settled == nil {
			return fmt.Errorf("parallel invariant: settled join has no route")
		}
		if a.Parallel.ResultsRef == nil {
			return fmt.Errorf("parallel invariant: a settled join reports no summary")
		}
		if d.ObservedCount < d.BranchCount && d.RemainderDisposition != "not_entered" && d.RemainderDisposition != "cancelled" {
			return fmt.Errorf("parallel invariant: unobserved branches lack a recorded disposition")
		}
	case "on_error", "failed":
		if d.Failure == "" || a.Status != "failed" || a.Settled == nil || (d.Route == "on_error") != (d.NextStageID != "") {
			return fmt.Errorf("parallel invariant: failed join lacks failure or settlement")
		}
		if a.Parallel.ResultsRef != nil {
			return fmt.Errorf("parallel invariant: a failed join reported a summary")
		}
	default:
		return fmt.Errorf("parallel invariant: unknown decision route")
	}
	return nil
}
