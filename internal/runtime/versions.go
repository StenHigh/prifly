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
type versionContract struct{ State, Read, StepRead, Next string }

var versionContracts = []versionContract{
	{CoreStateVersion, CoreReadVersion, "", "foundation-next/1"},
	{CoreInvocationStateVersion, CoreInvocationReadVersion, "", CoreInvocationNextVersion},
	{CoreRepeatStateVersion, CoreRepeatReadVersion, "", CoreRepeatNextVersion},
	{CoreContextStateVersion, CoreContextReadVersion, "", CoreContextNextVersion},
	{CoreSessionStateVersion, CoreSessionReadVersion, "", CoreSessionNextVersion},
	{CoreWaiverStateVersion, CoreWaiverReadVersion, "", CoreWaiverNextVersion},
	{CoreParallelStateVersion, CoreParallelReadVersion, "", CoreParallelNextVersion},
	{CoreMapStateVersion, CoreMapReadVersion, "", CoreMapNextVersion},
	{CoreWaitStateVersion, CoreWaitReadVersion, "", CoreWaitNextVersion},
	{CoreGuardStateVersion, CoreGuardReadVersion, "", CoreGuardNextVersion},
	{CoreReportedCostStateVersion, CoreReportedCostReadVersion, "", CoreReportedCostNextVersion},
	{CoreArtifactPublicationStateVersion, CoreArtifactPublicationReadVersion, CoreArtifactPublicationStepReadVersion, CoreArtifactPublicationNextVersion},
	{CoreArtifactClosureStateVersion, CoreArtifactClosureReadVersion, CoreArtifactClosureStepReadVersion, CoreArtifactClosureNextVersion},
	{CorePublicationSubscriptionStateVersion, CorePublicationSubscriptionReadVersion, CorePublicationSubscriptionStepReadVersion, CorePublicationSubscriptionNextVersion},
	{CorePublicationChecksStateVersion, CorePublicationChecksReadVersion, CorePublicationChecksStepReadVersion, CorePublicationChecksNextVersion},
	{CorePublicationNewOnlyStateVersion, CorePublicationNewOnlyReadVersion, CorePublicationNewOnlyStepReadVersion, CorePublicationNewOnlyNextVersion},
	{CorePublicationFailureStateVersion, CorePublicationFailureReadVersion, CorePublicationFailureStepReadVersion, CorePublicationFailureNextVersion},
	{CoreActionIntentStateVersion, CoreActionIntentReadVersion, CoreActionIntentStepReadVersion, CoreActionIntentNextVersion},
	{CoreActionAdmissionStateVersion, CoreActionAdmissionReadVersion, CoreActionAdmissionStepReadVersion, CoreActionAdmissionNextVersion},
	{CoreActionGrantAdmissionStateVersion, CoreActionGrantAdmissionReadVersion, CoreActionGrantAdmissionStepReadVersion, CoreActionGrantAdmissionNextVersion},
	{CoreActionDeliveryStateVersion, CoreActionDeliveryReadVersion, CoreActionDeliveryStepReadVersion, CoreActionDeliveryNextVersion},
	{CoreForkStateVersion, CoreForkReadVersion, CoreForkStepReadVersion, CoreForkNextVersion},
	{CoreWorkspaceStateVersion, CoreWorkspaceReadVersion, CoreWorkspaceStepReadVersion, CoreWorkspaceNextVersion},
	{CoreWorkspaceTreeStateVersion, CoreWorkspaceTreeReadVersion, CoreWorkspaceTreeStepReadVersion, CoreWorkspaceTreeNextVersion},
	{CoreDecisionStateVersion, CoreDecisionReadVersion, CoreDecisionStepReadVersion, CoreDecisionNextVersion},
	{CoreNeutralStateVersion, CoreNeutralReadVersion, CoreNeutralStepReadVersion, CoreNeutralNextVersion},
	{CoreTimingStateVersion, CoreTimingReadVersion, CoreTimingStepReadVersion, CoreTimingNextVersion},
	{CoreRoutedStateVersion, CoreRoutedReadVersion, CoreRoutedStepReadVersion, CoreRoutedNextVersion},
	{CoreEffectsStateVersion, CoreEffectsReadVersion, CoreEffectsStepReadVersion, CoreEffectsNextVersion},
	{CoreMaterializedStateVersion, CoreMaterializedReadVersion, CoreMaterializedStepReadVersion, CoreMaterializedNextVersion},
	{CoreStageWorkStateVersion, CoreStageWorkReadVersion, CoreStageWorkStepReadVersion, CoreRunFinishNextVersion},
	// 32 adds a field to the sealed executor config alone. The step read is
	// left empty on purpose: nothing about it changed, so a Run at 32 answers
	// step reads under the contract 31 already published.
	{CoreEnvironmentSourceStateVersion, CoreEnvironmentSourceReadVersion, "", CoreRunFinishNextVersion},
	// 34 records what a host said it did with a declared model profile. It is
	// the first row past the cap the published capability document set on
	// these lists; the owner withdrew that compatibility on 2026-09-19, and
	// the new bundle allows more while every earlier one keeps saying 32.
	{CoreModelProfileStateVersion, CoreModelProfileReadVersion, CoreModelProfileStepReadVersion, CoreRunFinishNextVersion},
	// 35 seals what the project says a declared profile name means for its
	// host, so the Run answers with what it started with rather than with
	// whatever the machine says now.
	{CoreProfileTranslationStateVersion, CoreProfileTranslationReadVersion, CoreProfileTranslationStepReadVersion, CoreRunFinishNextVersion},
	{CoreProjectTitleStateVersion, CoreProjectTitleReadVersion, "", CoreRunFinishNextVersion},
	{CoreRecoveryStateVersion, CoreRecoveryReadVersion, "", CoreRunFinishNextVersion},
	// 39 records the declared boundary of an external write on the handoff.
	{CoreExternalWriteStateVersion, CoreExternalWriteReadVersion, "", CoreRunFinishNextVersion},
	// 40 records recovery/2, which takes the source's tree and chooses no
	// commit, and reads the last accepted checkpoint.
	{CoreContinuationStateVersion, CoreContinuationReadVersion, "", CoreRunFinishNextVersion},
	// 36 mints no state row of its own: it is a next-action answer, so every
	// state that can describe a finished Run and is still being created takes
	// it up, the way 31 and 32 took up 33. That includes 31 and 32 themselves,
	// which is where every Run this engine has finished so far actually sits.
	// Every earlier state keeps the next contract it was published with.
}

// nextVersionFor is the contract a next-action answer is written under for a
// Run at this state. Next used to compute it from a second copy of the ladder
// above: sixteen sequential ifs that each overwrote the last, then a fourteen-
// branch else-if chain running the other way. Both halves said the same thing
// -- the highest row this state reaches -- in two shapes that could drift, and
// the earlier copies of this ladder did exactly that.
func nextVersionFor(version string) string {
	rank := stateRank(version)
	if rank < 0 || versionContracts[rank].Next == "" {
		return "foundation-next/1"
	}
	return versionContracts[rank].Next
}

func isNeutralState(version string) bool { return atLeast(version, CoreNeutralStateVersion) }
func isTimingState(version string) bool  { return atLeast(version, CoreTimingStateVersion) }
func isRoutedState(version string) bool  { return atLeast(version, CoreRoutedStateVersion) }
func isEffectsState(version string) bool { return atLeast(version, CoreEffectsStateVersion) }
func isMaterializedState(version string) bool {
	return atLeast(version, CoreMaterializedStateVersion)
}

func isStageWorkState(version string) bool {
	return atLeast(version, CoreStageWorkStateVersion)
}
func isProjectTitleState(version string) bool {
	return atLeast(version, CoreProjectTitleStateVersion)
}

func isRecoveryState(version string) bool { return atLeast(version, CoreRecoveryStateVersion) }

func isContinuationState(version string) bool {
	return atLeast(version, CoreContinuationStateVersion)
}

// higherState is whichever of two known states is later in the ladder. A Run
// needing several features is sealed at the highest of them, which carries
// every lower one; the last feature checked is not necessarily the highest,
// and taking it instead sealed an external write under a state with no place
// for it.
func higherState(current, candidate string) string {
	if stateRank(candidate) > stateRank(current) {
		return candidate
	}
	return current
}

// requiresContinuationState reports whether any workflow of this closure
// declares its checkpoint or what it continues from.
func requiresContinuationState(p *flow.Plan) bool {
	for _, workflow := range workflowPlans(p) {
		if workflow.Workflow.Checkpoint != nil || workflow.Workflow.Continuation != nil {
			return true
		}
	}
	return false
}

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
