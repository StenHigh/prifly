package runtime

import (
	"slices"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

// 0.13.49 shipped able to compile workflow revision 6 and declaring that it
// could not: the capability document listed 1 through 5. That document is what
// a dependent package reads before adopting a revision, so the build was
// telling every reader the opposite of what it does.
//
// The check reads both sides — what the compiler accepts and what the document
// claims — rather than either alone, because either alone is what shipped.
func TestTheDocumentDeclaresEveryRevisionThisBuildCompiles(t *testing.T) {
	declared := Capabilities().Profiles[1].WorkflowVersions
	for _, version := range flow.WorkflowRevisions {
		if !slices.Contains(declared, version) {
			t.Errorf("this build compiles workflow revision %s and its capability document does not declare it", version)
		}
	}
	for _, version := range declared {
		if !slices.Contains(flow.WorkflowRevisions, version) {
			t.Errorf("the capability document declares workflow revision %s, which this build does not compile", version)
		}
	}
}

// The verdict needs two declarations and a package needs both: the revision
// whose graphs may route it, and the result contract a step references to be
// allowed to return it. Declaring one without the other is an adoption that
// fails halfway.
func TestTheVerdictIsDeclaredOnBothSidesItNeeds(t *testing.T) {
	profile := Capabilities().Profiles[1]
	if !slices.Contains(profile.Capabilities, "blocked_verdict") {
		t.Fatal("the verdict is not named in the capability list")
	}
	if !slices.Contains(profile.WorkflowVersions, flow.WorkflowRevisionBlockedVersion) {
		t.Fatalf("the revision that routes it is not declared: %v", profile.WorkflowVersions)
	}
	definitions, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	if ref := builtinVersionRef(definitions, "core:schema/step-result", "2.0.0"); ref == (flow.Ref{}) {
		t.Fatal("the result contract a step must reference is not published")
	}
	// And the step contract that lets the gate hand over what it found on that
	// verdict. Without it the verdict arrives and its reason does not.
	if !slices.Contains(profile.StepVersions, "10") {
		t.Fatalf("the step contract that promises an output for the verdict is not declared: %v", profile.StepVersions)
	}
	// And the contract published before it is still published, unchanged: a
	// step that referenced it keeps referencing it.
	if ref := builtinVersionRef(definitions, "core:schema/step-result", "1.0.0"); ref == (flow.Ref{}) {
		t.Fatal("the first result contract stopped being published")
	}
}

// Four published bundles named assisted-session/8 as the edition a task carries
// and the engine never emitted it: a real handoff could not satisfy the contract
// that described it. The version existed as a label for a field, and a field is
// described by the bundle that adds it, not by an edition nothing reaches.
//
// The property held here is the one that was broken: every assisted edition this
// build declares is one some handoff can carry. A constant nobody emits is how
// the wrong const reached six bundles.
func TestEveryAssistedEditionDeclaredIsOneAHandoffCanCarry(t *testing.T) {
	declared := []string{
		AssistedSessionVersion, AssistedSessionCostVersion, AssistedSessionWorkspaceVersion,
		AssistedSessionTreeVersion, AssistedSessionDecisionVersion, AssistedSessionTimingVersion,
		AssistedSessionRoutedVersion,
	}
	// What the driver can assign, written out separately on purpose: a list
	// derived from the constants agrees with them by construction, which is
	// exactly how an edition nothing assigns stayed declared.
	assignable := []string{
		"assisted-session/1", "assisted-session/2", "assisted-session/3",
		"assisted-session/4", "assisted-session/5", "assisted-session/6",
		"assisted-session/7",
	}
	if len(declared) != len(assignable) {
		t.Fatalf("this build declares %d assisted editions and can assign %d", len(declared), len(assignable))
	}
	for i, edition := range declared {
		if edition != assignable[i] {
			t.Errorf("declared edition %s is not the one the driver assigns at that rank (%s)", edition, assignable[i])
		}
	}
	// And the newest is what a handoff actually carries, measured rather than
	// assumed: a constant and an emission agreeing on paper is what the six
	// bundles already did.
	e, runID, _ := assistedWorkspaceFixture(t, "checkout")
	if task := handOver(t, e, runID); task.SchemaVersion != AssistedSessionRoutedVersion {
		t.Fatalf("a handoff carries %q and the newest declared edition is %q", task.SchemaVersion, AssistedSessionRoutedVersion)
	}
}
