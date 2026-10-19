package runtime

import "github.com/stenhigh/prifly/internal/local"

// Old resolution events contain status, not the target or outcome. Their
// command identity binds the diagnostic to the owner's recorded resolution;
// matching a code or parsing its message alone would invent that proof.
func recoveryResolutionSequence(source Run, attempt *Attempt, events []local.Event) int64 {
	if attempt == nil || attempt.Status != "failed" || attempt.Settled == nil || attempt.Accepted != nil {
		return 0
	}
	for _, event := range events {
		if event.Type != "attempt.resolved" || event.Version != 1 || event.RunID != source.ID || event.CommandID == "" || event.Actor == "" || event.Seq <= 0 {
			continue
		}
		for _, d := range source.Diagnostics {
			if d.RunID == source.ID && d.AttemptID == attempt.ID && d.ActivationID == attempt.ActivationID && d.Origin == "core" && d.Severity == "error" && d.Phase == "resolution" && (d.Code == "resolved_applied" || d.Code == "resolved_not_applied") && d.ID == derivedID("diagnostic", source.ID, event.CommandID, d.Phase, d.Code) {
				return event.Seq
			}
		}
	}
	return 0
}

// This grants a new reviewed Run, never admission in the cancelled source.
// Each cancel must cover an explicitly resolved failed Attempt, and a later
// cancel needs a later attestation. Other Stops and live descendants still bar
// recovery; the ordinary failure-chain proof is required independently.
func recoveryCancellationResolved(source Run, events []local.Event) bool {
	if recoverySourceUnsettled(source) {
		return false
	}
	for _, inv := range source.Invocations {
		if inv == nil || !invocationTerminal(inv.Status) || inv.Settled == nil || len(inv.Ready) != 0 {
			return false
		}
	}
	covers := func(stop Stop, invID string) bool {
		switch stop.Scope {
		case "", "run":
			return stop.ScopeID == "" || stop.ScopeID == source.ID
		case "invocation":
			return source.Invocations[stop.ScopeID] != nil && source.withinInvocation(invID, stop.ScopeID)
		}
		return false
	}
	runCovered, found := false, false
	for _, stop := range source.Stops {
		if stop.Status != "active" {
			continue
		}
		if stop.Kind != "cancel" {
			return false
		}
		var cancelledAt int64
		for _, event := range events {
			if event.Type == "run.restricted" && event.Version == 1 && event.RunID == source.ID && event.Actor == stop.Actor && event.CommandID != "" && stop.ID == derivedID("stop", event.Actor, event.CommandID) {
				cancelledAt = event.Seq
			}
		}
		if cancelledAt <= 0 {
			return false
		}
		resolved := false
		for _, attempt := range source.Attempts {
			if attempt == nil {
				continue
			}
			activation := source.Activations[attempt.ActivationID]
			if activation != nil && activation.Status == "failed" && covers(stop, activation.InvocationID) && recoveryResolutionSequence(source, attempt, events) > cancelledAt {
				resolved = true
			}
		}
		if !resolved {
			return false
		}
		found = true
		runCovered = runCovered || (stop.Scope == "" || stop.Scope == "run")
	}
	if !found || source.CancelRequested && !runCovered {
		return false
	}
	for _, inv := range source.Invocations {
		if !inv.CancelRequested {
			continue
		}
		covered := false
		for _, stop := range source.Stops {
			if stop.Status == "active" && stop.Kind == "cancel" && covers(stop, inv.ID) {
				covered = true
			}
		}
		if !covered {
			return false
		}
	}
	return true
}
