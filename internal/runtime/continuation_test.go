package runtime

import (
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

func TestContinuationSelectsAcceptedSourceEvidence(t *testing.T) {
	ref := func(id string) ArtifactRef { return ArtifactRef{ArtifactID: id, Revision: 1, Digest: "sha256:" + id} }
	stamp := &Observation{UTC: "2026-09-22T12:00:00Z"}
	r := Run{ID: "run:source", Status: "completed", Outcome: stringPointer("partial"), WorkflowRef: flow.Ref{ID: "aif:workflow/classic"}, RootInvocationID: "root", Inputs: map[string]ArtifactRef{"task": ref("task")}, Activations: map[string]*Activation{"warmup": {StageID: "warmup", InvocationID: "root"}, "implement": {StageID: "implement", InvocationID: "root"}}, Steps: map[string]*Step{"w": {Status: "completed", Verdict: "pass", ActivationID: "warmup", Outputs: map[string]ArtifactRef{"handoff": ref("handoff")}}, "i": {Status: "completed", Verdict: "pass", ActivationID: "implement", Settled: stamp, Outputs: map[string]ArtifactRef{"plan": ref("plan"), "implementation": ref("implementation")}}}}
	selected, err := continuationRefs(r, 7)
	if err != nil || selected.RunVersion != 7 || selected.Task != ref("task") || selected.Handoff != ref("handoff") || selected.Plan != ref("plan") || selected.Implementation != ref("implementation") {
		t.Fatalf("selected %+v: %v", selected, err)
	}
	r.Outcome = stringPointer("rejected")
	if _, err := continuationRefs(r, 7); err != nil {
		t.Fatalf("rejected source refused: %v", err)
	}
	r.Status = "running"
	if _, err := continuationRefs(r, 7); err == nil {
		t.Fatal("non-terminal source accepted")
	}
	r.Status = "completed"
	r.WorkflowRef.ID = "other:workflow"
	if _, err := continuationRefs(r, 7); err == nil {
		t.Fatal("foreign workflow accepted")
	}
	r.WorkflowRef.ID = "aif:workflow/classic"
	r.Outcome = stringPointer("succeeded")
	if _, err := continuationRefs(r, 7); err == nil {
		t.Fatal("successful source accepted")
	}
	r.Outcome = stringPointer("partial")
	delete(r.Steps["i"].Outputs, "plan")
	if _, err := continuationRefs(r, 7); err == nil {
		t.Fatal("missing plan accepted")
	}
}

func stringPointer(value string) *string { return &value }
