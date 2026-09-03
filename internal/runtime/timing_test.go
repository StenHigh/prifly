package runtime

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stenhigh/prifly/internal/flow"
)

func timingObservation(ms int64) Observation {
	return Observation{UTC: time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC).Add(time.Duration(ms) * time.Millisecond).Format(time.RFC3339Nano), Session: "clock-fixture", MonotonicMS: ms, Source: "qualified-test-clock", SuspendBasis: "includes_suspend", UTCTrust: "trusted"}
}

func timingPoint(ms int64) *Observation { p := timingObservation(ms); return &p }

func timingFixture() Run {
	outcome := "succeeded"
	r := Run{
		ID: "run", RootInvocationID: "invocation", Status: "completed", Outcome: &outcome,
		Created: timingObservation(0), Settled: timingPoint(15000), LastObserved: timingObservation(15000),
		Activations: map[string]*Activation{
			"activation": {ID: "activation", StageID: "check", InvocationID: "invocation", Kind: "step", Status: "completed", StepID: "step", Created: timingObservation(0), Settled: timingPoint(15000)},
			"finish":     {ID: "finish", StageID: "done", InvocationID: "invocation", Kind: "finish", Status: "completed", Created: timingObservation(15000), Settled: timingPoint(15000)},
		},
		Steps: map[string]*Step{
			"step": {ID: "step", ActivationID: "activation", Ref: flow.Ref{ID: "shared-definition", Version: "1.0.0", Digest: "sha256:" + strings.Repeat("a", 64)}, Status: "completed", Verdict: "pass", AttemptIDs: []string{"attempt"}, Created: timingObservation(0), Settled: timingPoint(15000)},
		},
		Attempts: map[string]*Attempt{
			"attempt": {ID: "attempt", StepID: "step", ActivationID: "activation", Status: "completed", Admitted: timingObservation(5000), Started: timingPoint(5000), CandidateAt: timingPoint(12000), ExecutorEnd: timingPoint(13000), Settled: timingPoint(15000), Accepted: &Result{Verdict: "pass"}},
		},
		Transitions: []StateChange{
			{Kind: "run", ID: "run", From: "", To: "running", At: timingObservation(0)},
			{Kind: "activation", ID: "activation", From: "", To: "running", At: timingObservation(0)},
			{Kind: "step", ID: "step", From: "", To: "ready", At: timingObservation(0)},
			{Kind: "attempt", ID: "attempt", From: "", To: "admitted", At: timingObservation(5000)},
			{Kind: "step", ID: "step", From: "ready", To: "running", At: timingObservation(5000)},
			{Kind: "attempt", ID: "attempt", From: "admitted", To: "running", At: timingObservation(5000)},
			{Kind: "attempt", ID: "attempt", From: "running", To: "verifying", At: timingObservation(13000)},
			{Kind: "step", ID: "step", From: "running", To: "verifying", At: timingObservation(13000)},
			{Kind: "attempt", ID: "attempt", From: "verifying", To: "completed", At: timingObservation(15000)},
			{Kind: "step", ID: "step", From: "verifying", To: "completed", At: timingObservation(15000)},
			{Kind: "activation", ID: "activation", From: "running", To: "completed", At: timingObservation(15000)},
			{Kind: "run", ID: "run", From: "running", To: "completed", At: timingObservation(15000)},
			{Kind: "activation", ID: "finish", From: "", To: "completed", At: timingObservation(15000)},
		},
	}
	return r
}

func timingFind(t *testing.T, root TimingNode, id string) TimingNode {
	t.Helper()
	var found *TimingNode
	var walk func(TimingNode)
	walk = func(n TimingNode) {
		if n.ID == id {
			copy := n
			found = &copy
		}
		for _, child := range n.Children {
			walk(child)
		}
	}
	walk(root)
	if found == nil {
		t.Fatalf("timing node %q absent", id)
	}
	return *found
}

func timingMeasured(t *testing.T, got Duration, value int64, open bool) {
	t.Helper()
	if got.Quality != "measured" || got.ValueMS == nil || *got.ValueMS != value || got.KnownMS == nil || *got.KnownMS != value || got.EstimateMS != nil || got.IsOpen != open {
		t.Fatalf("want measured %d ms/open=%v, got %+v", value, open, got)
	}
}

func timingOpenFixture() Run {
	r := timingFixture()
	r.Status, r.Outcome, r.Settled = "running", nil, nil
	r.LastObserved = timingObservation(7000)
	r.Activations["activation"].Status, r.Activations["activation"].Settled = "running", nil
	delete(r.Activations, "finish")
	r.Steps["step"].Status, r.Steps["step"].Settled = "running", nil
	a := r.Attempts["attempt"]
	a.Status, a.Settled, a.ExecutorEnd, a.Accepted, a.CandidateAt = "running", nil, nil, nil, timingPoint(7000)
	r.Transitions = r.Transitions[:6]
	return r
}

func TestTimingIdentityAndNoControlExecutor(t *testing.T) { // OBS-AC-01
	r := timingFixture()
	second := *r.Steps["step"]
	second.ID, second.ActivationID, second.AttemptIDs = "second-step", "second-activation", []string{"second-attempt"}
	second.Created, second.Settled = timingObservation(15000), timingPoint(18000)
	r.Steps[second.ID] = &second
	r.Activations["second-activation"] = &Activation{ID: "second-activation", InvocationID: r.RootInvocationID, StageID: "check-again", Kind: "step", StepID: second.ID, Created: second.Created, Settled: second.Settled, Status: "completed"}
	r.Attempts["second-attempt"] = &Attempt{ID: "second-attempt", StepID: second.ID, ActivationID: second.ActivationID, Status: "completed", Admitted: timingObservation(15000), Started: timingPoint(15000), ExecutorEnd: timingPoint(18000), CandidateAt: timingPoint(18000), Settled: timingPoint(18000)}
	r.Activations["finish"].Created, r.Activations["finish"].Settled = timingObservation(18000), timingPoint(18000)
	r.Settled = timingPoint(18000)
	r.Transitions = nil // identity projection does not invent absent history
	report := Timing(r, timingObservation(18000), false)
	if timingFind(t, report.Root, "step").ID == timingFind(t, report.Root, "second-step").ID || report.Root.AttemptCount != 2 {
		t.Fatal("two executions of one definition were merged")
	}
	finish := timingFind(t, report.Root, "finish")
	if len(finish.Children) != 0 || finish.AttemptCount != 0 || finish.Metrics["executor_time"].Quality != "not_applicable" {
		t.Fatalf("control stage has a fictional executor: %+v", finish)
	}
	timingMeasured(t, finish.Metrics["elapsed"], 0, false)
	timingMeasured(t, report.Root.Metrics["executor_sum"], 11000, false)
}

func TestTimingQueueExecutionSettlementAndStateTimeline(t *testing.T) { // OBS-AC-02
	r := timingFixture()
	before, _ := json.Marshal(r)
	report := Timing(r, timingObservation(20000), false)
	step := timingFind(t, report.Root, "step")
	attempt := timingFind(t, report.Root, "attempt")
	timingMeasured(t, report.Root.Metrics["elapsed"], 15000, false)
	timingMeasured(t, step.Metrics["ready_queue"], 5000, false)
	timingMeasured(t, attempt.Metrics["dispatch_latency"], 0, false)
	timingMeasured(t, attempt.Metrics["executor_time"], 8000, false)
	timingMeasured(t, attempt.Metrics["post_execution_settlement"], 2000, false)
	timingMeasured(t, attempt.Metrics["result_to_acceptance"], 3000, false)
	timingMeasured(t, report.Root.Metrics["executor_sum"], 8000, false)
	timingMeasured(t, report.Root.Metrics["executor_active_union"], 8000, false)
	timingMeasured(t, step.StateTime["ready"], 5000, false)
	timingMeasured(t, step.StateTime["running"], 8000, false)
	timingMeasured(t, step.StateTime["verifying"], 2000, false)
	for _, interval := range step.Intervals {
		if interval.Metric == "state_time" && interval.State == "ready" && (interval.From == nil || interval.From.MonotonicMS != 0 || interval.To.MonotonicMS != 5000) {
			t.Fatal("state boundary pointers were rewritten by later transitions")
		}
	}
	after, _ := json.Marshal(r)
	if !bytes.Equal(before, after) {
		t.Fatal("read-only timing changed authoritative state")
	}
	first, _ := json.Marshal(report)
	second, _ := json.Marshal(Timing(r, timingObservation(20000), false))
	if !bytes.Equal(first, second) {
		t.Fatal("fixed-cut timing projection is not deterministic")
	}
}

func TestTimingEarlyResultAndMissingDriver(t *testing.T) { // OBS-AC-03, OBS-AC-11
	r := timingOpenFixture()
	live := timingFind(t, Timing(r, timingObservation(10000), true).Root, "attempt")
	timingMeasured(t, live.Metrics["executor_time"], 5000, true)
	timingMeasured(t, live.Metrics["result_to_acceptance"], 3000, true)
	if got := live.Metrics["post_execution_settlement"]; got.Quality != "unavailable" || got.IsOpen {
		t.Fatalf("early result started post-execution settlement: %+v", got)
	}
	stale := timingFind(t, Timing(r, timingObservation(10000), false).Root, "attempt").Metrics["executor_time"]
	if stale.Quality != "partial" || stale.ValueMS != nil || stale.KnownMS == nil || *stale.KnownMS != 2000 || !stale.IsOpen {
		t.Fatalf("persisted running state extended measured execution: %+v", stale)
	}
	r.Attempts["attempt"].CandidateAt = nil
	unknown := timingFind(t, Timing(r, timingObservation(10000), false).Root, "attempt").Metrics["executor_time"]
	if unknown.Quality != "unavailable" || unknown.ValueMS != nil || unknown.KnownMS != nil {
		t.Fatalf("missing process observation became zero: %+v", unknown)
	}
}

func TestTimingUTCRollbackUsesMonotonicOrder(t *testing.T) { // OBS-AC-04
	r := timingFixture()
	back := timingObservation(15000)
	back.UTC = timingObservation(-2000).UTC
	r.Settled = &back
	r.Steps["step"].Settled, r.Activations["activation"].Settled = &back, &back
	r.Attempts["attempt"].Settled = &back
	end := *r.Attempts["attempt"].ExecutorEnd
	end.UTC = timingObservation(1000).UTC
	r.Attempts["attempt"].ExecutorEnd = &end
	report := Timing(r, back, false)
	d := report.Root.Metrics["elapsed"]
	if d.Quality != "partial" || d.ValueMS != nil || d.KnownMS == nil || *d.KnownMS != 15000 || d.EstimateMS != nil || !strings.Contains(strings.Join(d.Reasons, ","), "utc_rollback") {
		t.Fatalf("calendar rollback hidden: %+v", d)
	}
	timingMeasured(t, timingFind(t, report.Root, "attempt").Metrics["executor_time"], 8000, false)
	if got := timingFind(t, report.Root, "step").StateTime["ready"]; got.ValueMS == nil || *got.ValueMS != 5000 {
		t.Fatal("state event order changed with wall clock")
	}
}

func TestTimingRecoveryGapKeepsKnownSegments(t *testing.T) { // OBS-AC-05
	r := timingOpenFixture()
	recovered := timingObservation(20000)
	recovered.Session, recovered.MonotonicMS, recovered.UTCTrust = "new-driver", 0, "local_wall_unqualified"
	asOf := recovered
	asOf.UTC, asOf.MonotonicMS = timingObservation(22000).UTC, 2000
	r.Gaps = []TimingGap{{From: timingObservation(8000), To: recovered, Reason: "driver_lost"}}
	r.LastObserved = recovered
	report := Timing(r, asOf, false)
	d := timingFind(t, report.Root, "attempt").Metrics["executor_time"]
	if d.Quality != "partial" || d.ValueMS != nil || d.KnownMS == nil || *d.KnownMS != 3000 || d.EstimateMS != nil {
		t.Fatalf("old execution clock continued through crash: %+v", d)
	}
	elapsed := report.Root.Metrics["elapsed"]
	if elapsed.Quality != "partial" || elapsed.KnownMS == nil || *elapsed.KnownMS != 10000 || elapsed.EstimateMS != nil {
		t.Fatalf("recoverable calendar pieces lost or gap invented: %+v", elapsed)
	}
	r.Attempts["attempt"].ExecutorEnd = &asOf
	r.Attempts["attempt"].Settled = &asOf
	r.Attempts["attempt"].Status = "completed"
	closed := timingFind(t, Timing(r, asOf, false).Root, "attempt").Metrics["executor_time"]
	if closed.Quality != "partial" || closed.ValueMS != nil || closed.KnownMS == nil || *closed.KnownMS != 3000 || closed.IsOpen {
		t.Fatalf("late end erased the recovery gap: %+v", closed)
	}
}

func TestTimingSuspendAndSerializedWallTimestamp(t *testing.T) { // OBS-AC-06
	r := timingFixture()
	data, _ := json.Marshal(r)
	data = bytes.ReplaceAll(data, []byte(`"includes_suspend"`), []byte(`"excludes_suspend_on_darwin"`))
	data = bytes.ReplaceAll(data, []byte(`"trusted"`), []byte(`"local_wall_unqualified"`))
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatal(err)
	}
	asOf := timingObservation(15000)
	asOf.SuspendBasis, asOf.UTCTrust = "excludes_suspend_on_darwin", "local_wall_unqualified"
	report := Timing(r, asOf, false)
	if got := report.Root.Metrics["elapsed"]; got.Quality != "partial" || got.ValueMS != nil || got.KnownMS == nil || *got.KnownMS != 15000 || got.EstimateMS != nil {
		t.Fatalf("suspend-excluding clock reported calendar elapsed: %+v", got)
	}
	attempt := timingFind(t, report.Root, "attempt")
	timingMeasured(t, attempt.Metrics["executor_time"], 8000, false)
	for _, interval := range attempt.Intervals {
		if interval.Metric == "executor_time" && (interval.SuspendBasis != "excludes_suspend_on_darwin" || interval.Method != "driver_observed_executor_occupancy") {
			t.Fatalf("executor method/suspend basis hidden: %+v", interval)
		}
	}
	from, to := Observation{UTC: timingObservation(0).UTC}, Observation{UTC: timingObservation(1000).UTC}
	c := timingCalculator{}
	d, _ := c.measure(timingSpan{from: &from, to: &to}, true, false)
	if d.Quality != "unavailable" || d.ValueMS != nil || d.KnownMS != nil || d.EstimateMS != nil {
		t.Fatalf("serialized time.Time gained invented monotonic data: %+v", d)
	}
}

func TestTimingRestrictionsAreUnionNotExtraElapsed(t *testing.T) { // OBS-AC-15
	r := timingFixture()
	a := r.Attempts["attempt"]
	a.Admitted, a.Started, a.ExecutorEnd = timingObservation(2000), timingPoint(2000), timingPoint(12000)
	r.Stops = []Stop{
		{ID: "pause-first", Kind: "pause", Status: "released", Created: timingObservation(4000), Released: timingPoint(8000)},
		{ID: "pause-second", Kind: "pause", Status: "released", Created: timingObservation(6000), Released: timingPoint(9000)},
	}
	report := Timing(r, timingObservation(15000), false)
	timingMeasured(t, report.Root.Metrics["elapsed"], 15000, false)
	timingMeasured(t, report.Root.Metrics["restricted_time"], 5000, false)
	timingMeasured(t, timingFind(t, report.Root, "attempt").Metrics["executor_time"], 10000, false)
	if d := report.Root.Metrics["cancel_to_settlement"]; d.Quality != "not_applicable" {
		t.Fatal("pause was treated as cancel")
	}
	r.Stops = append(r.Stops, Stop{ID: "cancel", Kind: "cancel", Status: "active", Created: timingObservation(10000)})
	report = Timing(r, timingObservation(30000), false)
	timingMeasured(t, report.Root.Metrics["cancel_to_settlement"], 5000, false)
	if got := timingFind(t, report.Root, "finish").Metrics["cancel_to_settlement"]; got.Quality != "not_applicable" {
		t.Fatalf("cancel interval invented for already instantaneous control stage: %+v", got)
	}
}

func TestTimingLeafDedupAndParallelArithmetic(t *testing.T) {
	r := timingFixture()
	r.Steps["step"].AttemptIDs = append(r.Steps["step"].AttemptIDs, "attempt")
	report := Timing(r, timingObservation(15000), false)
	if report.Root.AttemptCount != 1 {
		t.Fatal("attempt counted through multiple references")
	}
	timingMeasured(t, report.Root.Metrics["executor_sum"], 8000, false)
	parts := []Duration{measuredDuration(10000, false), measuredDuration(10000, false)}
	segments := []clockSegment{{timingObservation(0), timingObservation(10000)}, {timingObservation(0), timingObservation(10000)}}
	timingMeasured(t, sumTiming(parts), 20000, false)
	timingMeasured(t, unionTiming(parts, segments), 10000, false)
	segments[1].from.Session, segments[1].to.Session = "remote-clock", "remote-clock"
	if got := unionTiming(parts, segments); got.Quality != "unavailable" || got.ValueMS != nil {
		t.Fatalf("cross-domain parallel union invented: %+v", got)
	}
}

func TestTimingQualityOverflowAndTrustedEstimate(t *testing.T) {
	c := timingCalculator{driverLive: true}
	from, to := timingObservation(0), timingObservation(1000)
	to.Session = "new-clock"
	d, _ := c.measure(timingSpan{from: &from, to: &to}, true, false)
	if d.Quality != "estimated" || d.ValueMS != nil || d.KnownMS != nil || d.EstimateMS == nil || *d.EstimateMS != 1000 {
		t.Fatalf("explicit trusted UTC estimate missing: %+v", d)
	}
	to.UTCTrust = "local_wall_unqualified"
	d, _ = c.measure(timingSpan{from: &from, to: &to}, true, false)
	if d.Quality != "unavailable" || d.ValueMS != nil || d.KnownMS != nil || d.EstimateMS != nil {
		t.Fatalf("unqualified UTC was trusted: %+v", d)
	}
	to = from
	d, _ = c.measure(timingSpan{from: &from, to: &to}, false, false)
	timingMeasured(t, d, 0, false)
	for _, invalid := range []int64{-1, maxDurationMS + 1} {
		to.MonotonicMS = invalid
		d, _ = c.measure(timingSpan{from: &from, to: &to}, false, false)
		if d.Quality != "unavailable" || d.ValueMS != nil || d.KnownMS != nil {
			t.Fatalf("negative/overflow duration accepted: %+v", d)
		}
	}
	if got := sumTiming([]Duration{measuredDuration(maxDurationMS, false), measuredDuration(1, false)}); got.Quality != "unavailable" || got.ValueMS != nil {
		t.Fatalf("sum overflow wrapped: %+v", got)
	}
	// Fractional timestamps round the elapsed difference down, not the endpoints.
	from.UTC, to.UTC, from.UTCTrust, to.UTCTrust = "2026-08-28T00:00:00.000999999Z", "2026-08-28T00:00:00.001000001Z", "trusted", "trusted"
	if estimate := utcEstimate(from, to); estimate == nil || *estimate != 0 {
		t.Fatalf("sub-millisecond gap rounded up: %v", estimate)
	}
}

func TestTimingRetainsPiecesAcrossCommandClockSessions(t *testing.T) {
	r := timingFixture()
	r.Created.Session = "start-command"
	r.Created.UTCTrust = "local_wall_unqualified"
	report := Timing(r, timingObservation(15000), false)
	d := report.Root.Metrics["elapsed"]
	if d.Quality != "partial" || d.ValueMS != nil || d.KnownMS == nil || *d.KnownMS != 15000 || d.EstimateMS != nil {
		t.Fatalf("known foreground segments lost across command domains: %+v", d)
	}
	if report.Root.StateTime["running"].Quality == "measured" {
		t.Fatal("cross-domain state elapsed presented as a complete calendar interval")
	}
}
