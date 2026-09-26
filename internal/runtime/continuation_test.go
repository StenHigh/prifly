package runtime

import (
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

// The names below belong to no package: a continuation is whatever the
// continuing workflow declares, and the authority finds exactly that.
func TestContinuationTakesExactlyWhatTheWorkflowDeclares(t *testing.T) {
	ref := func(id string) ArtifactRef { return ArtifactRef{ArtifactID: id, Revision: 1, Digest: "sha256:" + id} }
	early, late := &Observation{UTC: "2026-09-22T12:00:00Z"}, &Observation{UTC: "2026-09-22T13:00:00Z"}
	outcome := "partial"
	r := Run{ID: "run:source", Status: "completed", Outcome: &outcome, WorkflowRef: flow.Ref{ID: "example:workflow/source"}, RootInvocationID: "root",
		Inputs: map[string]ArtifactRef{"brief": ref("brief")},
		Activations: map[string]*Activation{
			"a1": {ID: "a1", StageID: "prepare", InvocationID: "root", Status: "completed", StepID: "s1"},
			"a2": {ID: "a2", StageID: "draft", InvocationID: "root", Status: "completed", StepID: "s2"},
			"a3": {ID: "a3", StageID: "draft", InvocationID: "root", Status: "completed", StepID: "s3"},
			"a4": {ID: "a4", StageID: "gate", InvocationID: "root", Status: "completed", Kind: "call"},
		},
		Steps: map[string]*Step{
			"s1": {Status: "completed", Verdict: "pass", ActivationID: "a1", Settled: early, Outputs: map[string]ArtifactRef{"notes": ref("notes")}},
			"s2": {Status: "completed", Verdict: "pass", ActivationID: "a2", Settled: early, Outputs: map[string]ArtifactRef{"text": ref("old-text")}},
			"s3": {Status: "completed", Verdict: "pass", ActivationID: "a3", Settled: late, Outputs: map[string]ArtifactRef{"text": ref("new-text")}},
		},
		Invocations: map[string]*Invocation{
			"child": {ID: "child", CallerActivationID: "a4", Status: "completed", Outcome: &outcome, Settled: late, Outputs: map[string]ArtifactRef{"finding": ref("finding")}},
		},
	}
	target := &flow.Plan{}
	target.Workflow.ID = "example:workflow/continue"
	target.Workflow.Continuation = &flow.Continuation{
		FromWorkflows: []string{"example:workflow/source"},
		FromOutcomes:  []string{"partial", "rejected"},
		Inputs: map[string]flow.ContinuationSource{
			"brief":   {SourceInput: "brief"},
			"notes":   {Stage: "prepare", Output: "notes", Verdict: "pass"},
			"text":    {Stage: "draft", Output: "text", Verdict: "pass"},
			"finding": {Stage: "gate", Output: "finding", Outcome: "partial"},
		},
	}
	selected, err := continuationRefs(r, 7, target)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]ArtifactRef{"brief": ref("brief"), "notes": ref("notes"), "text": ref("new-text"), "finding": ref("finding")} {
		if got := selected.Inputs[name]; got.Ref != want || got.Source != target.Workflow.Continuation.Inputs[name] {
			t.Errorf("%s took %+v, want %v", name, got, want)
		}
	}
	if selected.RunVersion != 7 || selected.WorkflowID != "example:workflow/source" || selected.Outcome != "partial" {
		t.Fatalf("source identity not kept: %+v", selected)
	}
	refuse := func(code, why string, edit func()) {
		t.Helper()
		edit()
		if _, err := continuationRefs(r, 7, target); refusalCode(err) != code {
			t.Fatalf("%s: got %v, want %s", why, err, code)
		}
	}
	refuse("continuation_source_ineligible", "an outcome the workflow does not continue", func() { succeeded := "succeeded"; r.Outcome = &succeeded })
	refuse("continuation_source_ineligible", "a Run that has not ended", func() { r.Outcome, r.Status = &outcome, "running" })
	refuse("continuation_source_ineligible", "a workflow it does not continue", func() { r.Status, r.WorkflowRef.ID = "completed", "example:workflow/other" })
	refuse("continuation_source_incomplete", "a verdict that was not accepted", func() {
		r.WorkflowRef.ID = "example:workflow/source"
		r.Steps["s1"].Verdict = "fail"
	})
	refuse("continuation_source_incomplete", "an output the stage did not report", func() {
		r.Steps["s1"].Verdict = "pass"
		delete(r.Steps["s1"].Outputs, "notes")
	})
	refuse("continuation_source_ambiguous", "two settlements that cannot be ordered", func() {
		r.Steps["s1"].Outputs["notes"] = ref("notes")
		r.Steps["s2"].Settled = late
	})
	r.Steps["s2"].Settled = early
	// A cancelled Run has no outcome and is continued only where the workflow
	// says so; one still holding an execution nobody resolved is not.
	r.Status, r.Outcome = "cancelled", nil
	refuse("continuation_source_ineligible", "a cancelled Run the workflow does not continue", func() {})
	target.Workflow.Continuation.FromCancelled = true
	if cancelled, err := continuationRefs(r, 7, target); err != nil || cancelled.Outcome != "cancelled" || cancelled.Inputs["text"].Ref != ref("new-text") {
		t.Fatalf("a declared cancelled source was not continued: %+v %v", cancelled, err)
	}
	refuse("continuation_source_unsettled", "a cancellation with an unresolved effect", func() { r.HasUnresolvedEffects = true })
	target.Workflow.Continuation = nil
	if _, err := continuationRefs(r, 7, target); refusalCode(err) != "project_continue_undeclared" {
		t.Fatalf("an undeclared continuation was served: %v", err)
	}
}
func stringPointer(value string) *string { return &value }
