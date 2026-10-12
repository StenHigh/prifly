package runtime

import (
	"encoding/json"
	"slices"

	"github.com/stenhigh/prifly/internal/local"
)

// Recovery follows the recorded cause to its root caller. Reopen deliberately
// keeps its narrower brokenStage policy: these are different operations.
func recoveryFailureRoot(source Run, events []local.Event) (*Activation, error) {
	unsupported := func() (*Activation, error) {
		return nil, local.Reject("recover_frontier_unsupported", "source Run has no single proven root failure chain")
	}
	handled := map[string]bool{}
	for _, event := range events {
		if event.Type != "stage.error_handled" {
			continue
		}
		var fact struct {
			ActivationID string `json:"stage_activation_id"`
			StageID      string `json:"stage_id"`
			Next         string `json:"next_stage_id"`
		}
		if json.Unmarshal(event.Data, &fact) != nil || fact.Next == "" {
			return unsupported()
		}
		a := source.Activations[fact.ActivationID]
		if a == nil || a.StageID != fact.StageID || a.Status != "failed" || a.Settled == nil {
			return unsupported()
		}
		handled[a.ID] = true
	}
	failed := map[string]*Activation{}
	for id, a := range source.Activations {
		if a == nil || a.Status != "failed" {
			continue
		}
		if a.ID != id || a.Settled == nil {
			return unsupported()
		}
		ignored := handled[id]
		if isInvocationState(source.SchemaVersion) {
			lineage, err := source.invocationLineage(a.InvocationID)
			if err != nil {
				return unsupported()
			}
			for _, inv := range lineage {
				if inv.Status == "completed" && inv.Outcome != nil && inv.Settled != nil {
					ignored = true
				}
				if inv.ParentInvocationID != "" {
					caller := source.Activations[inv.CallerActivationID]
					if !source.childMatchesCaller(inv, caller) {
						return unsupported()
					}
					if handled[caller.ID] {
						ignored = true
					}
				}
			}
		}
		if !ignored {
			failed[id] = a
		}
	}
	var root *Activation
	for _, a := range failed {
		if a.InvocationID == source.RootInvocationID {
			if root != nil {
				return unsupported()
			}
			root = a
		}
	}
	if root == nil {
		return unsupported()
	}
	seen := map[string]bool{}
	for a := root; ; {
		if seen[a.ID] {
			return unsupported()
		}
		seen[a.ID] = true
		if isInvocationState(source.SchemaVersion) {
			inv := source.Invocations[a.InvocationID]
			if inv == nil || inv.Status != "failed" || inv.Outcome != nil || inv.Settled == nil || inv.CancelRequested || len(inv.Ready) != 0 {
				return unsupported()
			}
		}
		child := source.currentBody(a)
		if child == nil {
			// A malformed caller link must not masquerade as preparation.
			for _, inv := range source.Invocations {
				if inv != nil && inv.CallerActivationID == a.ID {
					return unsupported()
				}
			}
			if a.Kind == "step" {
				if _, err := recoveryFailedAttempt(source, a, events); err != nil {
					return nil, err
				}
			} else if (a.Kind != "call" && a.Kind != "repeat") || !recoveryPreparationRecorded(source, a, events) {
				return unsupported()
			}
			break
		}
		if !source.childMatchesCaller(child, a) || child.Status != "failed" || child.Outcome != nil || child.Settled == nil || !recoveryChildFailureRecorded(source, a, child) {
			return unsupported()
		}
		var next *Activation
		for _, candidate := range failed {
			if candidate.InvocationID == child.ID {
				if next != nil {
					return unsupported()
				}
				next = candidate
			}
		}
		if next == nil {
			return unsupported()
		}
		a = next
	}
	if len(seen) != len(failed) {
		return unsupported()
	}
	return root, nil
}

func recoveryChildFailureRecorded(source Run, caller *Activation, child *Invocation) bool {
	phase, causes := "call", []string{child.ID}
	if caller.Kind == "repeat" {
		if source.repeatProgressInvariant(caller) != nil {
			return false
		}
		d := caller.Repeat.LastDecision
		if d == nil || d.BodyInvocationID != child.ID || d.Route != "failed" || d.Failure != "child_failed" || d.UntilResult != "not_evaluated" {
			return false
		}
		phase, causes = "repeat", []string{d.ID, child.ID}
	} else if caller.Kind != "call" {
		return false
	}
	for _, diagnostic := range source.Diagnostics {
		if diagnostic.ActivationID == caller.ID && diagnostic.RunID == source.ID && diagnostic.Origin == "core" && diagnostic.Severity == "error" && diagnostic.Code == "child_failed" && diagnostic.Phase == phase && diagnostic.AttemptID == "" && slices.Equal(diagnostic.CauseRefs, causes) {
			return true
		}
	}
	return false
}

func recoveryPreparationRecorded(source Run, a *Activation, events []local.Event) bool {
	for _, diagnostic := range source.Diagnostics {
		if diagnostic.ActivationID != a.ID || diagnostic.RunID != source.ID || diagnostic.Origin != "core" || diagnostic.Severity != "error" || diagnostic.Phase != "preparation" || diagnostic.AttemptID != "" || diagnostic.Code == "" {
			continue
		}
		for _, event := range events {
			if event.Type != "stage.failed" {
				continue
			}
			var fact struct {
				ActivationID string `json:"stage_activation_id"`
				Failure      string `json:"failure"`
				Execution    string `json:"execution"`
			}
			if json.Unmarshal(event.Data, &fact) == nil && fact.ActivationID == a.ID && fact.Failure == diagnostic.Code && fact.Execution == "not_admitted" {
				return true
			}
		}
	}
	return false
}

func recoveryFailedAttempt(source Run, a *Activation, events []local.Event) (*Attempt, error) {
	step := source.Steps[a.StepID]
	if step == nil || step.ID != a.StepID || step.ActivationID != a.ID || step.Status != "failed" {
		return nil, local.Reject("recover_frontier_unsupported", "failed step has no recorded instance")
	}
	if len(step.AttemptIDs) == 0 {
		for _, attempt := range source.Attempts {
			if attempt != nil && (attempt.StepID == step.ID || attempt.ActivationID == a.ID) {
				return nil, local.Reject("recover_frontier_unsupported", "preparation failure has an admitted Attempt")
			}
		}
		if step.Settled == nil || !recoveryPreparationRecorded(source, a, events) {
			return nil, local.Reject("recover_frontier_unsupported", "failed step lacks recorded pre-admission evidence")
		}
		return nil, nil
	}
	attempt := source.Attempts[step.AttemptIDs[len(step.AttemptIDs)-1]]
	if attempt == nil || attempt.StepID != step.ID || attempt.ActivationID != a.ID || attempt.Settled == nil || attempt.ProcessOutcome != nil && attempt.ProcessOutcome.Uncertain {
		return nil, local.Reject("recover_source_unsettled", "failed Attempt is missing settlement or has an uncertain process")
	}
	return attempt, nil
}
