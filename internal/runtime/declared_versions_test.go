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
	// And the contract published before it is still published, unchanged: a
	// step that referenced it keeps referencing it.
	if ref := builtinVersionRef(definitions, "core:schema/step-result", "1.0.0"); ref == (flow.Ref{}) {
		t.Fatal("the first result contract stopped being published")
	}
}
