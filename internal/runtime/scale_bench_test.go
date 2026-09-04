package runtime

import (
	"fmt"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

// wideRun builds a Run of the shape a long workflow produces: many invocations,
// each with its own activation, step, attempt and recorded state changes.
func wideRun(count int) Run {
	r := Run{
		SchemaVersion: CoreInvocationStateVersion, ID: "run:wide", RootInvocationID: "invocation:root",
		Status: "running", Created: timingObservation(0), LastObserved: timingObservation(int64(count) * 10),
		Invocations: map[string]*Invocation{},
		Activations: map[string]*Activation{},
		Steps:       map[string]*Step{},
		Attempts:    map[string]*Attempt{},
	}
	ref := flow.Ref{ID: "test:step/wide", Version: "1.0.0", Digest: "sha256:" + fmt.Sprintf("%064d", 1)}
	for i := range count {
		at := int64(i) * 10
		invocation := fmt.Sprintf("invocation:%d", i)
		activation := fmt.Sprintf("activation:%d", i)
		step := fmt.Sprintf("step:%d", i)
		attempt := fmt.Sprintf("attempt:%d", i)
		r.Invocations[invocation] = &Invocation{ID: invocation, RunID: r.ID, Status: "completed", Created: timingObservation(at), Settled: timingPoint(at + 5)}
		r.Activations[activation] = &Activation{ID: activation, StageID: "work", InvocationID: invocation, Kind: "step", Status: "completed", StepID: step, Created: timingObservation(at), Settled: timingPoint(at + 5)}
		r.Steps[step] = &Step{ID: step, ActivationID: activation, Ref: ref, Status: "completed", Verdict: "pass", AttemptIDs: []string{attempt}, Created: timingObservation(at), Settled: timingPoint(at + 5)}
		r.Attempts[attempt] = &Attempt{ID: attempt, StepID: step, ActivationID: activation, Status: "completed", Admitted: timingObservation(at), Started: timingPoint(at + 1), ExecutorEnd: timingPoint(at + 4), Settled: timingPoint(at + 5), Accepted: &Result{Verdict: "pass"}}
		r.Transitions = append(r.Transitions,
			StateChange{Kind: "attempt", ID: attempt, To: "running", At: timingObservation(at + 1)},
			StateChange{Kind: "attempt", ID: attempt, From: "running", To: "completed", At: timingObservation(at + 5)},
			StateChange{Kind: "step", ID: step, To: "completed", At: timingObservation(at + 5)},
		)
	}
	return r
}

func BenchmarkLoad1000Invocations(b *testing.B) {
	state, err := canonicalState(wideRun(1000))
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(state)))
	b.ResetTimer()
	for b.Loop() {
		var r Run
		if err := decodeState(state, &r); err != nil {
			b.Fatal(err)
		}
		if len(r.Invocations) != 1000 {
			b.Fatal("incomplete decode")
		}
	}
}

func BenchmarkTiming1000Nodes(b *testing.B) {
	r := wideRun(1000)
	b.ResetTimer()
	for b.Loop() {
		if tree := Timing(r, r.LastObserved, false); tree.Root.ID == "" {
			b.Fatal("timing produced no tree")
		}
	}
}
