package runtime

import (
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

// resumeFixture is a Run of prepare -> verify -> finish that ended partial
// because verify returned blocked, and the plan it ran under. The names are
// the test's own: the point a Run stopped at is read from its routing, never
// from a stage name.
func resumeFixture() (Run, *flow.Plan, map[string]int64) {
	observed := Observation{UTC: "2026-09-28T10:00:00Z"}
	outcome := "partial"
	r := Run{
		ID: "run:source", Status: "completed", Outcome: &outcome, Profile: flow.CoreProfile,
		Settled: &observed, Active: []string{}, Stops: []Stop{}, RootInvocationID: "root",
		Invocations: map[string]*Invocation{"root": {ID: "root", Status: "completed", Outcome: &outcome}},
		Activations: map[string]*Activation{
			"a:prepare": {ID: "a:prepare", InvocationID: "root", StageID: "prepare", Kind: "step", Status: "completed", StepID: "s:prepare", Settled: &observed},
			"a:verify":  {ID: "a:verify", InvocationID: "root", StageID: "verify", Kind: "step", Status: "completed", StepID: "s:verify", Settled: &observed},
			"a:stopped": {ID: "a:stopped", InvocationID: "root", StageID: "stopped", Kind: "finish", Status: "completed", Settled: &observed},
		},
		Steps: map[string]*Step{
			"s:prepare": {ID: "s:prepare", ActivationID: "a:prepare", Status: "completed", Verdict: "pass"},
			"s:verify":  {ID: "s:verify", ActivationID: "a:verify", Status: "completed", Verdict: "blocked"},
		},
	}
	p := &flow.Plan{}
	p.Workflow.ID = "test:workflow/work"
	p.Workflow.Definition.Entry = "prepare"
	p.Workflow.Definition.Stages = map[string]flow.Stage{
		"prepare": {Kind: "step", On: map[string]string{"pass": "verify"}},
		"verify":  {Kind: "step", On: map[string]string{"pass": "done", "blocked": "stopped"}},
		"done":    {Kind: "finish", Outcome: "succeeded"},
		"stopped": {Kind: "finish", Outcome: "partial"},
	}
	sequences := map[string]int64{"a:prepare": 3, "a:verify": 7, "a:stopped": 11}
	return r, p, sequences
}

func declaring(p *flow.Plan, resumable *flow.Resumable) *flow.Plan {
	target := *p
	target.Workflow = p.Workflow
	target.Workflow.Resumable = resumable
	return &target
}

func TestAStoppedRunResumesWhereItStoppedOnlyWhenItsWorkflowSaysSo(t *testing.T) {
	t.Parallel()
	r, p, sequences := resumeFixture()
	if _, err := recoveryPointOf(r, p, p, sequences, ""); refusalCode(err) != "resume_undeclared" {
		t.Fatalf("a Run of a workflow that declared nothing was resumed: %v", err)
	}
	if _, err := recoveryPointOf(r, p, declaring(p, &flow.Resumable{FromOutcomes: []string{"rejected"}}), sequences, ""); refusalCode(err) != "resume_undeclared" {
		t.Fatalf("an outcome the workflow does not resume from was resumed: %v", err)
	}
	target := declaring(p, &flow.Resumable{FromOutcomes: []string{"partial"}})
	point, err := recoveryPointOf(r, p, target, sequences, "")
	if err != nil || point.StageID != "verify" || point.Cutoff != 7 || point.Attempt != nil {
		t.Fatalf("the Run did not resume at the stage that led to finish: %v %+v", err, point)
	}

	// An earlier accepted stage may be named; the stopping stage itself, a
	// stage after it or one never accepted may not.
	if _, err := recoveryPointOf(r, p, target, sequences, "prepare"); err != nil {
		t.Fatalf("an earlier accepted stage was refused: %v", err)
	}
	for _, stage := range []string{"verify", "stopped", "missing"} {
		if _, err := recoveryPointOf(r, p, target, sequences, stage); refusalCode(err) != "resume_from_stage_invalid" {
			t.Errorf("--from-stage %s: %v", stage, err)
		}
	}

	// Resuming from the entry carries nothing: that is a new start.
	early, _ := recoveryPointOf(r, p, target, sequences, "prepare")
	if _, _, err := recoveryTrace(r, p, target, nil, nil, sequences, early); refusalCode(err) != "resume_prefix_empty" {
		t.Fatalf("an empty prefix was accepted: %v", err)
	}

	// Unresolved work is never resumed over.
	r.HasUnresolvedEffects = true
	if _, err := recoveryPointOf(r, p, target, sequences, ""); refusalCode(err) != "recover_source_unsettled" {
		t.Fatalf("a source with unresolved effects was resumed: %v", err)
	}
}

func TestACancelledRunResumesAtTheStageItWasStoppedAt(t *testing.T) {
	t.Parallel()
	r, p, sequences := resumeFixture()
	r.Status, r.Outcome, r.CancelRequested = "cancelled", nil, true
	delete(r.Activations, "a:stopped")
	r.Activations["a:verify"].Status = "cancelled"
	r.Steps["s:verify"].Status, r.Steps["s:verify"].Verdict = "cancelled", ""
	if _, err := recoveryPointOf(r, p, declaring(p, &flow.Resumable{FromOutcomes: []string{"partial"}}), sequences, ""); refusalCode(err) != "resume_undeclared" {
		t.Fatalf("a cancelled Run was resumed without from_cancelled: %v", err)
	}
	target := declaring(p, &flow.Resumable{FromCancelled: true})
	point, err := recoveryPointOf(r, p, target, sequences, "")
	if err != nil || point.StageID != "verify" || point.Cutoff != 7 {
		t.Fatalf("the cancelled stage is not the point: %v %+v", err, point)
	}

	// Cancelled between stages: verify was never activated, and prepare's
	// accepted result routed there.
	delete(r.Activations, "a:verify")
	delete(r.Steps, "s:verify")
	point, err = recoveryPointOf(r, p, target, sequences, "")
	if err != nil || point.StageID != "verify" || point.Activation != nil {
		t.Fatalf("the stage the Run was cancelled before is not the point: %v %+v", err, point)
	}
}
