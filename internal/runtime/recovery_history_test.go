package runtime

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

func TestResumeChainInheritedTraceAndFromStage(t *testing.T) {
	t.Parallel()
	origin, p, sequences := resumeFixture()
	origin.Steps["s:prepare"].AttemptIDs = []string{"attempt:prepare"}
	ref := ArtifactRef{ArtifactID: "artifact:original", Revision: 1, Digest: "sha256:bytes"}
	origin.Steps["s:prepare"].Outputs = map[string]ArtifactRef{"handoff": ref}
	origin.Attempts = map[string]*Attempt{"attempt:prepare": {ID: "attempt:prepare", Settled: origin.Settled, Accepted: &Result{Verdict: "pass", Outputs: origin.Steps["s:prepare"].Outputs}}}
	entries, _, err := recoveryAcceptedTrace(origin, p, p, nil, nil, sequences, 7)
	if err != nil || len(entries) != 1 {
		t.Fatalf("origin: %v %+v", err, entries)
	}
	history := []recoveryHistoryEntry{{Source: origin, Plan: p, Sequences: sequences, Entry: entries[0], Root: true}}
	data, _ := json.Marshal(origin)
	var child Run
	if err := json.Unmarshal(data, &child); err != nil {
		t.Fatal(err)
	}
	child.ID = "run:child"
	child.RootInvocationID = "child-root"
	child.Invocations = map[string]*Invocation{"child-root": {ID: "child-root", Status: "completed", Outcome: child.Outcome}}
	delete(child.Activations, "a:prepare")
	delete(child.Steps, "s:prepare")
	for _, a := range child.Activations {
		a.InvocationID = child.RootInvocationID
	}
	copyRef := ref
	copyRef.ArtifactID = "artifact:copy"
	child.Recovery = &RecoveryProvenance{RootOutputs: map[string]map[string]ArtifactRef{"prepare": {"handoff": copyRef}}}
	target := declaring(p, &flow.Resumable{FromOutcomes: []string{"partial"}})
	point, cut, err := recoveryPointWithHistory(child, p, target, sequences, "", history)
	if err != nil {
		t.Fatal(err)
	}
	reused, outputs, err := recoveryTraceWithHistory(child, p, target, nil, nil, sequences, point, history, cut)
	if err != nil || len(reused) != 1 || outputs["prepare"]["handoff"] != copyRef || reused[0].AttemptID != "attempt:prepare" {
		t.Fatalf("composed trace: %v %+v %+v", err, reused, outputs)
	}
	chosen, cut, err := recoveryPointWithHistory(child, p, target, sequences, "prepare", history)
	if err != nil || cut != 0 || chosen.Cutoff != 0 {
		t.Fatalf("inherited root point: %v %+v %d", err, chosen, cut)
	}
	if _, _, err := recoveryTraceWithHistory(child, p, target, nil, nil, sequences, chosen, history, cut); refusalCode(err) != "resume_prefix_empty" {
		t.Fatalf("entry prefix: %v", err)
	}
	history[0].Root = false
	if _, _, err := recoveryPointWithHistory(child, p, target, sequences, "prepare", history); refusalCode(err) != "resume_from_stage_invalid" {
		t.Fatalf("nested stage chosen as root: %v", err)
	}
	history[0].Root = true
	history[0].Entry.Sequence = math.MaxInt64 - 1
	if _, _, err := recoveryTraceWithHistory(child, p, target, nil, nil, sequences, point, history, 1); refusalCode(err) != "recover_trace_invalid" {
		t.Fatalf("foreign event sequence trusted: %v", err)
	}
	history[0].Entry = entries[0]
	changed := *target
	changed.Workflow = target.Workflow
	changed.Workflow.Definition.Stages = map[string]flow.Stage{}
	for id, stage := range target.Workflow.Definition.Stages {
		changed.Workflow.Definition.Stages[id] = stage
	}
	stage := changed.Workflow.Definition.Stages["prepare"]
	stage.On = map[string]string{"pass": "stopped"}
	changed.Workflow.Definition.Stages["prepare"] = stage
	if _, _, err := recoveryTraceWithHistory(child, p, &changed, nil, nil, sequences, point, history, 1); refusalCode(err) != "recover_prefix_changed" {
		t.Fatalf("changed inherited contract trusted: %v", err)
	}
}

func TestResumeChainRevalidatedOutputsRemainBindingsWithoutExecutions(t *testing.T) {
	t.Parallel()
	r, p, sequences := resumeFixture()
	r.Steps["s:prepare"].AttemptIDs = []string{"attempt:prepare"}
	r.Attempts = map[string]*Attempt{"attempt:prepare": {ID: "attempt:prepare", Settled: r.Settled, Accepted: &Result{Verdict: "pass"}}}
	entries, _, err := recoveryAcceptedTrace(r, p, p, nil, nil, sequences, 7)
	if err != nil {
		t.Fatal(err)
	}
	history := []recoveryHistoryEntry{{Source: r, Plan: p, Sequences: sequences, Entry: entries[0], Root: true}}
	p.Workflow.Definition.Stages["tail"] = flow.Stage{Kind: "step", On: map[string]string{"pass": "done"}}
	child := Run{ID: "run:child", RootInvocationID: "child-root", Activations: map[string]*Activation{}}
	ref := ArtifactRef{ArtifactID: "artifact:current-copy", Revision: 1, Digest: "sha256:candidate-bytes"}
	child.Recovery = &RecoveryProvenance{RootOutputs: map[string]map[string]ArtifactRef{"verify": {"report": ref}}}
	history = append(history, recoveryHistoryEntry{Source: r, Plan: p, Entry: RecoveryReuse{StageID: "verify", Kind: "step"}, Root: true, Revalidated: true})
	point := recoveryPoint{StageID: "tail", Cutoff: math.MaxInt64}
	for i := 0; i < 3; i++ {
		reused, outputs, err := recoveryTraceWithHistory(child, p, p, nil, nil, nil, point, history, len(history))
		if err != nil || len(reused) != 1 || outputs["verify"]["report"] != ref {
			t.Fatalf("revalidated bindings lost or fake execution: %v %+v %+v", err, reused, outputs)
		}
		// Another transfer changes the accessible reference, never the bytes.
		ref.ArtifactID += "-copy"
		child.Recovery.RootOutputs["verify"]["report"] = ref
	}
	reused, outputs, err := recoveryTraceWithHistory(child, p, p, nil, nil, nil, recoveryPoint{StageID: "verify", Cutoff: 0}, history, 1)
	if err != nil || len(reused) != 1 || len(outputs["verify"]) != 0 {
		t.Fatalf("chosen revalidated stage was retained: %v %+v", err, outputs)
	}
}

func TestResumeChainMissingAndCyclicEvidence(t *testing.T) {
	t.Parallel()
	e := artifactEngine(t)
	for _, test := range []struct{ name, parent, code string }{
		{"missing", "run:missing", "recover_evidence_unavailable"},
		{"cycle", "run:source", "recover_trace_invalid"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			source := Run{ID: "run:source", Recovery: &RecoveryProvenance{SourceRunID: test.parent, SourceRunVersion: 1}}
			if _, err := e.recoveryHistory(context.Background(), source, map[string]bool{source.ID: true}); refusalCode(err) != test.code {
				t.Fatalf("history: %v", err)
			}
		})
	}
}

func TestResumeChainUnsupportedAncestorIsEvidenceRefusal(t *testing.T) {
	t.Parallel()
	e := artifactEngine(t)
	zero := int64(0)
	_, err := e.Store.Apply(context.Background(), local.Command{ID: "command:unsupported-ancestor", Actor: e.owner, RunID: "run:unsupported", Payload: json.RawMessage(`{"test":"unsupported historical edition"}`), ExpectedVersion: &zero, Mode: local.CommandCAS}, func(local.Snapshot) (local.Change, error) {
		return local.Change{Data: json.RawMessage(`{"schema_version":"unsupported-test"}`), Events: []local.EventInput{{Type: "run.created", Version: 1, Data: json.RawMessage(`{}`)}}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	source := Run{ID: "run:child", Recovery: &RecoveryProvenance{SourceRunID: "run:unsupported", SourceRunVersion: 1}}
	if _, err := e.recoveryHistory(context.Background(), source, map[string]bool{source.ID: true}); refusalCode(err) != "recover_evidence_unavailable" {
		t.Fatalf("unsupported ancestor: %v", err)
	}
}
