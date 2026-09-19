package runtime

import "testing"

// The contract a next-action answer is written under was a second copy of the
// state ladder, living inside Next: sixteen sequential ifs that each overwrote
// the last, then a fourteen-branch else-if chain running the other way. Both
// halves computed the same thing — the highest row the state reaches — in two
// shapes, which is exactly how the older copies of this ladder drifted apart.
//
// This table is that ladder written out by hand, one row per state version this
// build knows. It is deliberately not derived from versionContracts: a table
// generated from the thing it checks agrees with it by construction and would
// have agreed with the broken copies too.
func TestEveryStateNamesItsNextContract(t *testing.T) {
	expected := []struct{ state, next string }{
		{CoreStateVersion, "foundation-next/1"},
		{CoreInvocationStateVersion, CoreInvocationNextVersion},
		{CoreRepeatStateVersion, CoreRepeatNextVersion},
		{CoreContextStateVersion, CoreContextNextVersion},
		{CoreSessionStateVersion, CoreSessionNextVersion},
		{CoreWaiverStateVersion, CoreWaiverNextVersion},
		{CoreParallelStateVersion, CoreParallelNextVersion},
		{CoreMapStateVersion, CoreMapNextVersion},
		{CoreWaitStateVersion, CoreWaitNextVersion},
		{CoreGuardStateVersion, CoreGuardNextVersion},
		{CoreReportedCostStateVersion, CoreReportedCostNextVersion},
		{CoreArtifactPublicationStateVersion, CoreArtifactPublicationNextVersion},
		{CoreArtifactClosureStateVersion, CoreArtifactClosureNextVersion},
		{CorePublicationSubscriptionStateVersion, CorePublicationSubscriptionNextVersion},
		{CorePublicationChecksStateVersion, CorePublicationChecksNextVersion},
		{CorePublicationNewOnlyStateVersion, CorePublicationNewOnlyNextVersion},
		{CorePublicationFailureStateVersion, CorePublicationFailureNextVersion},
		{CoreActionIntentStateVersion, CoreActionIntentNextVersion},
		{CoreActionAdmissionStateVersion, CoreActionAdmissionNextVersion},
		{CoreActionGrantAdmissionStateVersion, CoreActionGrantAdmissionNextVersion},
		{CoreActionDeliveryStateVersion, CoreActionDeliveryNextVersion},
		{CoreForkStateVersion, CoreForkNextVersion},
		{CoreWorkspaceStateVersion, CoreWorkspaceNextVersion},
		{CoreWorkspaceTreeStateVersion, CoreWorkspaceTreeNextVersion},
		{CoreDecisionStateVersion, CoreDecisionNextVersion},
		{CoreNeutralStateVersion, CoreNeutralNextVersion},
		{CoreTimingStateVersion, CoreTimingNextVersion},
		{CoreRoutedStateVersion, CoreRoutedNextVersion},
		{CoreEffectsStateVersion, CoreEffectsNextVersion},
		{CoreMaterializedStateVersion, CoreMaterializedNextVersion},
		{CoreStageWorkStateVersion, CoreProgramEnvironmentNextVersion},
		// 32 adds a field to the sealed executor config and nothing to the
		// answer, so it answers under the contract 31 already publishes.
		{CoreEnvironmentSourceStateVersion, CoreProgramEnvironmentNextVersion},
	}
	if len(expected) != len(versionContracts) {
		t.Fatalf("the ladder has %d rows and this table names %d; a new state version needs its next contract here", len(versionContracts), len(expected))
	}
	for index, row := range expected {
		if versionContracts[index].State != row.state {
			t.Fatalf("row %d of the ladder is %s, this table expects %s", index, versionContracts[index].State, row.state)
		}
		if answered := nextVersionFor(row.state); answered != row.next {
			t.Errorf("a Run at %s answers under %s, the ladder says %s", row.state, answered, row.next)
		}
	}
	// A state this build has never heard of is not "at least" anything, so it
	// gets the oldest answer rather than the newest.
	if answered := nextVersionFor("core-state/999"); answered != "foundation-next/1" {
		t.Errorf("an unknown state answered under %s", answered)
	}
}

// The exit table calls 2 "the form, the input, an object that does not exist"
// and 3 "the version, epoch, claim, slot, admission or access this command
// assumed is not the one held". A project refusal is a declaration that does
// not hold together, which is the first, but exitForCode sorts by substring
// and eight project codes carry "conflict" in their names. Until the project
// surface stopped hiding behind invalid_usage the question never arose,
// because invalid_usage carries neither word.
func TestAProjectRefusalIsNotAnAuthorityStateConflict(t *testing.T) {
	for _, code := range []string{
		"project_profile_conflict",
		"project_runner_conflict",
		"project_option_conflict",
		"project_local_conflict",
		"project_compile_profile_value_conflict",
		"project_extension_stage_conflict",
		"project_start_package_identity_conflict",
		"project_workflow_package_conflict",
		"project_root_invalid",
	} {
		if exit := exitForCode(code); exit != 2 {
			t.Errorf("%s exits %d; a declaration that does not hold together is the form class", code, exit)
		}
	}
	// The substring rules still sort the authority's own codes.
	for code, want := range map[string]int{
		"version_conflict":            3,
		"capacity_exhausted":          5,
		"unsupported_storage_version": 5,
		"recovery_required":           6,
	} {
		if exit := exitForCode(code); exit != want {
			t.Errorf("%s exits %d, expected %d", code, exit, want)
		}
	}
}
