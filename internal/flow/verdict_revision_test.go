package flow

import (
	"slices"
	"strings"
	"testing"
)

// A plan is recompiled from sealed bytes on every read of its Run. So growing
// the list of legal verdicts is not a forward-compatible act: the completeness
// rule of revisions 4 and 5 would start demanding an answer for a verdict those
// documents cannot even name, and a Run that read yesterday would stop reading.
//
// This test is written before the list grows and is the reason the set
// completeness answers for is a property of the revision rather than the global
// list. It fails the moment a verdict is added without that gating.
func TestSealedRevisionsAnswerOnlyForTheirOwnVerdicts(t *testing.T) {
	for _, version := range []string{WorkflowRevisionVerdictVersion, WorkflowRevisionRetryVersion} {
		t.Run("revision "+version, func(t *testing.T) {
			workflow, registry := verdictFixture(t)
			workflow["schema_version"] = version
			if _, err := CompileProfile(encoded(t, workflow), "json", registry, CoreProfile); err != nil {
				t.Fatalf("a graph complete for its own revision was refused: %v", err)
			}
			// And the revision could not be made complete for a wider set even
			// by an author who wanted to: its published schema names the
			// verdicts it knows, so gating the rule is the only way out rather
			// than one of two. Declared alone, so the refusal is the enum and
			// not the count -- the first writing of this test passed on maxItems and
			// proved nothing about the enum.
			for _, id := range []string{"check_first", "check_second"} {
				stages(workflow)[id].(map[string]any)["impossible_verdicts"] = []any{"blocked"}
			}
			_, err := CompileProfile(encoded(t, workflow), "json", registry, CoreProfile)
			if err == nil {
				t.Fatal("a sealed revision accepted a verdict its own contract does not name")
			}
			if p := expectProblem(t, err, "schema_invalid"); !strings.Contains(p.Message, "contract") {
				t.Fatalf("refused for some other reason: %v", p)
			}
			// The same declaration is accepted by the revision that names it,
			// which is what makes the refusal above about the contract rather
			// than about the word.
			widened, widenedRegistry := verdictFixture(t)
			widened["schema_version"] = WorkflowRevisionBlockedVersion
			for _, id := range []string{"check_first", "check_second"} {
				stages(widened)[id].(map[string]any)["impossible_verdicts"] = []any{"needs_revision", "no_work", "blocked"}
			}
			if _, err := CompileProfile(encoded(t, widened), "json", widenedRegistry, CoreProfile); err != nil {
				t.Fatalf("the revision that names the verdict refused to declare it: %v", err)
			}
		})
	}
}

// The set a revision answers for is what the rule reads. Revisions 4 and 5 keep
// exactly the four they were published with; anything added belongs to a
// revision that can name it.
func TestEachRevisionNamesTheVerdictsItAnswersFor(t *testing.T) {
	sealed := []string{"pass", "fail", "needs_revision", "no_work"}
	for _, version := range []string{WorkflowRevisionVerdictVersion, WorkflowRevisionRetryVersion} {
		if required := VerdictsRequiredBy(version); !slices.Equal(required, sealed) {
			t.Errorf("revision %s answers for %v, it was published answering for %v", version, required, sealed)
		}
	}
	// Every verdict a step may legally return is answered for by the newest
	// revision: a verdict nobody has to route is one an author can forget.
	newest := VerdictsRequiredBy(WorkflowRevisionBlockedVersion)
	for _, verdict := range StepVerdicts {
		if !slices.Contains(newest, verdict) {
			t.Errorf("%s is a legal verdict that the newest revision does not require an answer for", verdict)
		}
	}
	if len(newest) <= len(sealed) {
		t.Fatalf("the newest revision answers for %d verdicts and the sealed ones for %d", len(newest), len(sealed))
	}
}
