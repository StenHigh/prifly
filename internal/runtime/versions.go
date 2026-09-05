package runtime

import (
	"slices"

	"github.com/stenhigh/prifly/internal/flow"
)

// versionContracts is the one place the state ladder is written down. Each row
// is a core state version and the read contract that describes it, in the order
// they were introduced: a later state answers yes to every earlier question,
// because each version only adds to the one before it.
//
// Everything that had to decide "is this state at least X" or "which read
// contract describes it" reads this table. Those questions used to be answered
// by two dozen mutually recursive predicates and by the same ladder written out
// in seven places, which is how the copies drifted apart from each other.
type versionContract struct{ State, Read, StepRead string }

var versionContracts = []versionContract{
	{CoreStateVersion, CoreReadVersion, ""},
	{CoreInvocationStateVersion, CoreInvocationReadVersion, ""},
	{CoreRepeatStateVersion, CoreRepeatReadVersion, ""},
	{CoreContextStateVersion, CoreContextReadVersion, ""},
	{CoreSessionStateVersion, CoreSessionReadVersion, ""},
	{CoreWaiverStateVersion, CoreWaiverReadVersion, ""},
	{CoreParallelStateVersion, CoreParallelReadVersion, ""},
	{CoreMapStateVersion, CoreMapReadVersion, ""},
	{CoreWaitStateVersion, CoreWaitReadVersion, ""},
	{CoreGuardStateVersion, CoreGuardReadVersion, ""},
	{CoreReportedCostStateVersion, CoreReportedCostReadVersion, ""},
	{CoreArtifactPublicationStateVersion, CoreArtifactPublicationReadVersion, CoreArtifactPublicationStepReadVersion},
	{CoreArtifactClosureStateVersion, CoreArtifactClosureReadVersion, CoreArtifactClosureStepReadVersion},
	{CorePublicationSubscriptionStateVersion, CorePublicationSubscriptionReadVersion, CorePublicationSubscriptionStepReadVersion},
	{CorePublicationChecksStateVersion, CorePublicationChecksReadVersion, CorePublicationChecksStepReadVersion},
	{CorePublicationNewOnlyStateVersion, CorePublicationNewOnlyReadVersion, CorePublicationNewOnlyStepReadVersion},
	{CorePublicationFailureStateVersion, CorePublicationFailureReadVersion, CorePublicationFailureStepReadVersion},
	{CoreActionIntentStateVersion, CoreActionIntentReadVersion, CoreActionIntentStepReadVersion},
	{CoreActionAdmissionStateVersion, CoreActionAdmissionReadVersion, CoreActionAdmissionStepReadVersion},
	{CoreActionGrantAdmissionStateVersion, CoreActionGrantAdmissionReadVersion, CoreActionGrantAdmissionStepReadVersion},
	{CoreActionDeliveryStateVersion, CoreActionDeliveryReadVersion, CoreActionDeliveryStepReadVersion},
	{CoreForkStateVersion, CoreForkReadVersion, CoreForkStepReadVersion},
	{CoreWorkspaceStateVersion, CoreWorkspaceReadVersion, CoreWorkspaceStepReadVersion},
	{CoreWorkspaceTreeStateVersion, CoreWorkspaceTreeReadVersion, CoreWorkspaceTreeStepReadVersion},
	{CoreDecisionStateVersion, CoreDecisionReadVersion, CoreDecisionStepReadVersion},
	{CoreNeutralStateVersion, CoreNeutralReadVersion, CoreNeutralStepReadVersion},
	{CoreTimingStateVersion, CoreTimingReadVersion, CoreTimingStepReadVersion},
}

func isNeutralState(version string) bool { return atLeast(version, CoreNeutralStateVersion) }
func isTimingState(version string) bool  { return atLeast(version, CoreTimingStateVersion) }

// stateRank is a state version's place in that order, or -1 for a version this
// build does not know. An unknown version is never "at least" anything.
func stateRank(version string) int {
	return slices.IndexFunc(versionContracts, func(row versionContract) bool { return row.State == version })
}

// atLeast reports whether a state carries everything a minimum version defines.
func atLeast(version, minimum string) bool {
	rank := stateRank(version)
	return rank >= 0 && rank >= stateRank(minimum)
}

// readVersionFor is the read contract that describes a state. A state this
// build does not know is reported under the baseline its profile belongs to,
// exactly as before: a reader is never told a contract that does not exist.
func readVersionFor(state, profile string) string {
	if rank := stateRank(state); rank >= 0 {
		return versionContracts[rank].Read
	}
	if profile == flow.CoreProfile {
		return CoreReadVersion
	}
	return ReadVersion
}

// stepReadVersionFor is the step read contract a publisher is answered under.
// The earliest states have no step contract of their own, so they read under
// the foundation one.
func stepReadVersionFor(state string) string {
	for rank := stateRank(state); rank >= 0; rank-- {
		if versionContracts[rank].StepRead != "" {
			return versionContracts[rank].StepRead
		}
	}
	return stepReadVersion
}
