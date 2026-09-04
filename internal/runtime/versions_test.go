package runtime

import "testing"

// The ladder is one list, and every question about a state version is answered
// from it. This pins the two properties the copies used to get wrong: a later
// state answers yes to every earlier question, and the read contract a state is
// reported under is the one that belongs to it, not an earlier one.
func TestVersionLadderAnswersEveryQuestionConsistently(t *testing.T) {
	for rank, row := range versionContracts {
		if got := readVersionFor(row.State, "core-workflow/1"); got != row.Read {
			t.Fatalf("%s is reported as %s, not %s", row.State, got, row.Read)
		}
		for earlier := range rank + 1 {
			if !atLeast(row.State, versionContracts[earlier].State) {
				t.Fatalf("%s does not carry %s", row.State, versionContracts[earlier].State)
			}
		}
		for later := rank + 1; later < len(versionContracts); later++ {
			if atLeast(row.State, versionContracts[later].State) {
				t.Fatalf("%s claims to carry the later %s", row.State, versionContracts[later].State)
			}
		}
		if step := stepReadVersionFor(row.State); step == "" {
			t.Fatalf("%s has no step read contract", row.State)
		}
	}
	// An unknown version carries nothing and is reported under the baseline.
	if atLeast("core-state/999", CoreStateVersion) {
		t.Fatal("an unknown state version claimed to carry the baseline")
	}
	if got := readVersionFor("foundation-state/1", "foundation-workflow/1"); got != ReadVersion {
		t.Fatalf("a foundation state is reported as %s", got)
	}
}

// Every predicate reads the same ladder, so each one is true exactly from its
// own version onwards.
func TestStatePredicatesFollowTheLadder(t *testing.T) {
	for _, c := range []struct {
		name    string
		version string
		holds   func(string) bool
	}{
		{"invocations", CoreInvocationStateVersion, isInvocationState},
		{"contexts", CoreContextStateVersion, isContextState},
		{"sessions", CoreSessionStateVersion, isSessionState},
		{"waivers", CoreWaiverStateVersion, isWaiverState},
		{"parallel", CoreParallelStateVersion, isParallelState},
		{"map", CoreMapStateVersion, isMapState},
		{"wait", CoreWaitStateVersion, isWaitState},
		{"guards", CoreGuardStateVersion, isGuardState},
		{"reported cost", CoreReportedCostStateVersion, isReportedCostState},
		{"artifact publication", CoreArtifactPublicationStateVersion, isArtifactPublicationState},
		{"artifact closure", CoreArtifactClosureStateVersion, isArtifactClosureState},
		{"subscriptions", CorePublicationSubscriptionStateVersion, isPublicationSubscriptionState},
		{"publication checks", CorePublicationChecksStateVersion, isPublicationChecksState},
		{"new-only sources", CorePublicationNewOnlyStateVersion, isPublicationNewOnlyState},
		{"failure sources", CorePublicationFailureStateVersion, isPublicationFailureState},
		{"action intents", CoreActionIntentStateVersion, isActionIntentState},
		{"action admissions", CoreActionAdmissionStateVersion, isActionAdmissionState},
		{"grant admissions", CoreActionGrantAdmissionStateVersion, isActionGrantAdmissionState},
		{"action deliveries", CoreActionDeliveryStateVersion, isActionDeliveryState},
		{"forks", CoreForkStateVersion, isForkState},
		{"workspaces", CoreWorkspaceStateVersion, isWorkspaceState},
		{"workspace trees", CoreWorkspaceTreeStateVersion, isWorkspaceTreeState},
		{"decisions", CoreDecisionStateVersion, isDecisionState},
	} {
		t.Run(c.name, func(t *testing.T) {
			rank := stateRank(c.version)
			for i, row := range versionContracts {
				if holds := c.holds(row.State); holds != (i >= rank) {
					t.Fatalf("%s reports %t for %s", c.version, holds, row.State)
				}
			}
		})
	}
}
