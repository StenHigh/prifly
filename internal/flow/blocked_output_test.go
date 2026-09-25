package flow

import (
	"encoding/json"
	"slices"
	"testing"
)

// A gate that returns the verdict meaning "I could not judge" has something to
// hand over: what was unreachable and which checks never ran. Until the output
// port could be promised for that verdict, it could not: the binding on that
// edge was refused as unguaranteed, correctly, because the step had promised
// nothing there. The reason an operator needs most reached only the journal.
func TestAnOutputMayBePromisedForTheVerdictThatJudgedNothing(t *testing.T) {
	for _, test := range []struct {
		contract string
		accepted bool
	}{
		{"StepDefinitionV9", false},
		{"StepDefinitionV10", true},
	} {
		t.Run(test.contract, func(t *testing.T) {
			schema, err := ProtocolSchema(test.contract)
			if err != nil {
				t.Fatal(err)
			}
			var document map[string]any
			if err := json.Unmarshal(schema, &document); err != nil {
				t.Fatal(err)
			}
			port := document["$defs"].(map[string]any)["StepOutputPort"].(map[string]any)
			required := port["properties"].(map[string]any)["required_for"].(map[string]any)
			values := []string{}
			for _, value := range required["items"].(map[string]any)["enum"].([]any) {
				values = append(values, value.(string))
			}
			if slices.Contains(values, "blocked") != test.accepted {
				t.Fatalf("%s names %v", test.contract, values)
			}
			// The bound moves with the set, or the contract accepts a list it
			// cannot fill: six values and a maximum of seven is a promise
			// nobody can keep.
			want := float64(len(values))
			if got, ok := required["maxItems"].(float64); !ok || got != want {
				t.Fatalf("%s allows %v of %v values", test.contract, required["maxItems"], want)
			}
		})
	}
}

// An insertion that declares impossible verdicts raises the revision it is
// spliced into. It used to assign v4 outright, which lowered a graph authored
// later -- a project's insertion silently rewriting the package author's
// contract, and every route the later revision added refused as unsupported.
func TestAnInsertionRaisesTheRevisionAndNeverLowersIt(t *testing.T) {
	for _, test := range []struct {
		sealed, want string
	}{
		{"1", WorkflowRevisionVerdictVersion},
		{"3", WorkflowRevisionVerdictVersion},
		{WorkflowRevisionVerdictVersion, WorkflowRevisionVerdictVersion},
		{WorkflowRevisionRetryVersion, WorkflowRevisionRetryVersion},
		{WorkflowRevisionBlockedVersion, WorkflowRevisionBlockedVersion},
	} {
		raised := test.sealed
		if WorkflowRevisionAtLeast(WorkflowRevisionVerdictVersion, raised) {
			raised = WorkflowRevisionVerdictVersion
		}
		if raised != test.want {
			t.Errorf("an insertion into a revision %s graph left it at %s, expected %s", test.sealed, raised, test.want)
		}
	}
	// A revision this build does not know is older than everything it does, so
	// a caller raising to a known revision never lowers an unknown one it
	// cannot reason about.
	if WorkflowRevisionAtLeast("1", "99") {
		t.Fatal("an unknown revision compared as older than a known one")
	}
}
