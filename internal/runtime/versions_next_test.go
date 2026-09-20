package runtime

import (
	"strings"
	"testing"
)

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
		// 36 is a next-action answer and records nothing, so every state still
		// being created took it up, the way 31 and 32 took up 33. 33, 34 and
		// 35 are what a Run sealed before it answers under; nothing rewrites
		// those.
		{CoreStageWorkStateVersion, CoreRunFinishNextVersion},
		{CoreEnvironmentSourceStateVersion, CoreRunFinishNextVersion},
		{CoreModelProfileStateVersion, CoreRunFinishNextVersion},
		{CoreProfileTranslationStateVersion, CoreRunFinishNextVersion},
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

// The cap the published capability document sets on its own version lists was
// 32, and this build filled it. Raising it is a withdrawal of compatibility,
// not a number going up, so both sides are held: the bundle that allows more
// and the bundles that do not.
func TestTheNewBundleAllowsWhatEveryOlderOneRefuses(t *testing.T) {
	manifest := Capabilities()
	core := manifest.Profiles[1]
	if len(core.StateVersions) != len(versionContracts) || len(core.ReadVersions) != len(versionContracts) {
		t.Fatalf("the capability document lists %d state and %d read versions against a ladder of %d", len(core.StateVersions), len(core.ReadVersions), len(versionContracts))
	}
	if len(core.StateVersions) <= 32 {
		t.Fatalf("this test is about passing the cap of 32 and the document lists %d", len(core.StateVersions))
	}
	if err := validatePublic(t, "CoreCapabilitiesV35", manifest); err != nil {
		t.Fatalf("the bundle this build publishes rejects its own capability document: %v", err)
	}
	// And the withdrawal is real, not cosmetic: the bundle published before it
	// refuses the same document, for the reason it was raised.
	err := validatePublic(t, "CoreCapabilitiesV31", manifest)
	if err == nil {
		t.Fatal("an older bundle accepted a document past the cap it declares, so nothing was withdrawn and nothing needed raising")
	}
	if !strings.Contains(err.Error(), "32") && !strings.Contains(err.Error(), "maxItems") && !strings.Contains(err.Error(), "at most") {
		t.Fatalf("the older bundle refused for some other reason: %v", err)
	}
}
