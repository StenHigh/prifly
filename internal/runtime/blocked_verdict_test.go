package runtime

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// A mandatory gate returned broken because PostgreSQL had gone away, with
// new_failures_total=0: nothing was wrong with the work, and the vocabulary had
// no way to say so. The graph took the route declared for a judgment about the
// work, the fix attempt found nothing to fix, and the Run ended partial.
//
// A step that declares the result contract naming the verdict can now say it
// could not judge, and a graph at the revision that answers for it routes that
// wherever its author decided -- here, back to the same stage.
func TestAStepCanSayItCouldNotJudgeAndTheGraphRoutesIt(t *testing.T) {
	e, workflow := coreDriverFixture(t, "pass")
	definitions, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	blocked := builtinVersionRef(definitions, "core:schema/step-result", "2.0.0")
	if blocked == (flow.Ref{}) {
		t.Fatal("this build publishes no result contract that names the verdict")
	}
	if v1 := builtinRef(definitions, "core:schema/step-result"); v1.Digest == blocked.Digest {
		t.Fatal("the two published result contracts are the same bytes, so one of them says nothing new")
	}

	work := workflow.Definition.Stages["work"]
	work.On = map[string]string{"pass": "done", "blocked": "recovered"}
	work.ImpossibleVerdicts = []string{"fail", "needs_revision", "no_work"}
	workflow.Definition.Stages["work"] = work
	workflow.SchemaVersion = flow.WorkflowRevisionBlockedVersion
	runID := coreDriverStart(t, e, workflow)

	// The host is told what its node routes, and the added verdict is in it:
	// a verdict the graph routes and the task does not name is one the worker
	// cannot know it may return.
	r := driverRun(t, e, runID)
	p, err := r.planFor(r.RootInvocationID)
	if err != nil {
		t.Fatal(err)
	}
	if routed := routedVerdicts(p, "work"); !slices.Contains(routed, "blocked") {
		t.Fatalf("the routed verdicts handed to a host are %v", routed)
	}
}

// The contract a step declared is what narrows it. Intake accepts the widest
// shape because it runs before the step is resolved; a step that declared the
// first result contract still may not return the later verdict, and the
// refusal names the contract rather than the word.
func TestAStepThatDeclaredTheOlderContractMayNotReturnTheNewVerdict(t *testing.T) {
	for _, test := range []struct {
		name, version string
		accepted      bool
	}{
		{"declared the contract that names it", "2.0.0", true},
		{"declared the contract that does not", "1.0.0", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			definitions, registry, err := Builtins()
			if err != nil {
				t.Fatal(err)
			}
			ref := builtinVersionRef(definitions, "core:schema/step-result", test.version)
			if ref == (flow.Ref{}) {
				t.Fatalf("no result contract published at %s", test.version)
			}
			result := []byte(`{"schema_version":"1","run_id":"run:x","step_instance_id":"step:x","attempt_id":"attempt:x","envelope_digest":"sha256:` + strings.Repeat("0", 64) + `","verdict":"blocked","outputs":{},"evidence_refs":[],"effect_receipt_refs":[],"summary":"the database never answered"}`)
			err = flow.ValidateSchema(registry, ref, result)
			if test.accepted && err != nil {
				t.Fatalf("the contract that names the verdict refused it: %v", err)
			}
			if !test.accepted && err == nil {
				t.Fatal("the contract published before the verdict accepted it")
			}
		})
	}
}

// And intake, which runs before the step is resolved, must not be the place the
// verdict dies. This goes through observeResult rather than calling a validator
// a second time: the first writing of this test asserted the contract directly
// and stayed green when intake was narrowed back, which is the whole failure it
// existed to catch.
func TestIntakeDoesNotRefuseTheVerdictBeforeTheStepIsKnown(t *testing.T) {
	e, workflow := coreDriverFixture(t, "pass")
	runID := coreDriverStart(t, e, workflow)
	attempt := driverAdmit(t, e, runID)
	result := []byte(`{"schema_version":"1","run_id":"` + runID + `","step_instance_id":"` + attempt.StepID + `","attempt_id":"` + attempt.ID + `","envelope_digest":"` + attempt.EnvelopeDigest + `","verdict":"blocked","outputs":{},"evidence_refs":[],"effect_receipt_refs":[],"summary":"the database never answered"}`)
	err := e.observeResult(context.Background(), runID, attempt.ID, local.ProcessObservation{Result: result})
	if err != nil && strings.Contains(err.Error(), "schema_invalid") {
		t.Fatalf("intake refused the verdict before any step could declare it: %v", err)
	}
}

// A sealed graph that predates the verdict keeps every accepted result if one
// arrives anyway. This is the behaviour an unrouted verdict already had; the
// test is here because the whole change rests on it being true.
func TestASealedGraphKeepsItsWorkWhenTheNewVerdictHasNoRoute(t *testing.T) {
	e, workflow := coreDriverFixture(t, "pass")
	runID := coreDriverStart(t, e, workflow)
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	if r.SchemaVersion == "" {
		t.Fatal("the fixture sealed no state version")
	}
	for _, a := range r.Attempts {
		if a.Accepted == nil {
			t.Fatalf("a graph sealed before the verdict lost its accepted result: %+v", a)
		}
	}
}
