package runtime

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

func telemetryQuery(mode string, metrics ...string) TelemetryQuery {
	return TelemetryQuery{SchemaVersion: TelemetryQueryVersion, Mode: mode, Metrics: metrics}
}
func telemetryReport(t *testing.T, e *Engine, q TelemetryQuery) TelemetryResponse {
	t.Helper()
	r, err := e.Telemetry(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func telemetryGroup(t *testing.T, r TelemetryResponse, name string) TelemetryAggregate {
	t.Helper()
	var found *TelemetryAggregate
	for _, group := range r.Aggregates {
		if group.Metric == name {
			if found != nil {
				t.Fatalf("more than one %s group", name)
			}
			v := group
			found = &v
		}
	}
	if found == nil {
		t.Fatalf("missing %s: %+v", name, r.Aggregates)
	}
	return *found
}

// Seed authoritative historical facts through the real Store transaction.
// No fake telemetry report or precomputed aggregate is supplied to the query.
func telemetrySaveRun(t *testing.T, e *Engine, r Run, version int64) int64 {
	t.Helper()
	data, err := canonicalState(r)
	if err != nil {
		t.Fatal(err)
	}
	event, err := canonical(map[string]any{"observation": r.LastObserved, "status": r.Status})
	if err != nil {
		t.Fatal(err)
	}
	result, err := e.Store.Apply(context.Background(), local.Command{ID: newID("command"), Actor: e.owner, RunID: r.ID, Mode: local.CommandCAS, ExpectedVersion: &version, Payload: json.RawMessage(`{"seed":"historical-facts"}`)}, func(local.Snapshot) (local.Change, error) {
		return local.Change{Data: data, Events: []local.EventInput{{Type: "run.created", Data: event}}}, nil
	})
	if err != nil || result.Receipt.Rejection != nil {
		t.Fatalf("save facts: %+v %v", result, err)
	}
	return result.Receipt.Cut
}
func telemetryHistoryRun(t *testing.T, base Run, id, status string, duration int64) Run {
	t.Helper()
	b, err := canonicalState(base)
	if err != nil {
		t.Fatal(err)
	}
	var r Run
	if err := decodeState(b, &r); err != nil {
		t.Fatal(err)
	}
	r.ID = "run:" + id
	r.RootInvocationID = "invocation:" + id
	r.Status = status
	r.Outcome = nil
	r.Created = timingObservation(0)
	r.LastObserved = timingObservation(duration)
	r.Settled = nil
	r.Publications = nil
	r.Diagnostics = nil
	r.Transitions = nil
	r.Gaps = nil
	stepRef := r.Steps["step:one"].Ref
	activationID, stepID, attemptID := "activation:"+id, "step:"+id, "attempt:"+id
	r.Activations = map[string]*Activation{activationID: {ID: activationID, StageID: "report", InvocationID: r.RootInvocationID, Kind: "step", Status: status, StepID: stepID, Created: r.Created}}
	r.Steps = map[string]*Step{stepID: {ID: stepID, ActivationID: activationID, Ref: stepRef, Status: status, AttemptIDs: []string{attemptID}, Outputs: map[string]ArtifactRef{}, Created: r.Created}}
	r.Attempts = map[string]*Attempt{attemptID: {ID: attemptID, StepID: stepID, ActivationID: activationID, Status: status, Admitted: r.Created, Dispatch: timingPoint(0), Started: timingPoint(0)}}
	r.Active = []string{attemptID}
	r.HasUnresolvedEffects = false
	if r.terminal() {
		end := timingPoint(duration)
		r.Active = nil
		r.Settled = end
		r.Activations[activationID].Settled = end
		r.Steps[stepID].Settled = end
		a := r.Attempts[attemptID]
		a.Settled = end
		a.ExecutorEnd = end
		a.CandidateAt = end
		if status == "completed" {
			r.Outcome = telemetryPtr("succeeded")
			r.Steps[stepID].Verdict = "pass"
			a.Accepted = &Result{RunID: r.ID, StepInstanceID: stepID, AttemptID: attemptID, Verdict: "pass", Summary: "SECRET-RESULT-BODY"}
			a.ProcessOutcome = &local.ProcessOutcome{WaitReturned: true, GroupEmpty: true, ExitCode: telemetryPtr(0)}
		}
	}
	return r
}

func TestTelemetryLifecyclePopulationAndIndependentVerdict(t *testing.T) {
	e, _, command := publicationFixture(t, nil)
	base, _ := publicationRun(t, e, command)
	ids := []string{}
	for i, status := range []string{"completed", "completed", "failed", "cancelled", "running"} {
		r := telemetryHistoryRun(t, base, fmt.Sprintf("population-%d", i), status, 10)
		if i == 1 {
			for _, step := range r.Steps {
				step.Verdict = "fail"
			}
			for _, attempt := range r.Attempts {
				attempt.Accepted.Verdict = "fail"
			}
		}
		telemetrySaveRun(t, e, r, 0)
		ids = append(ids, r.ID)
	}
	q := telemetryQuery("aggregate", "core.failed_run_fraction", "core.succeeded_run_fraction", "core.first_attempt_pass_fraction")
	q.RunIDs = ids
	r := telemetryReport(t, e, q)
	p := r.Population
	if p.Matched != 5 || p.Terminal != 4 || p.Open != 1 || p.Cancelled != 1 || p.Attempts != 5 || p.StartedAttempts != 5 || p.SettledAttempts != 4 || p.FirstAttemptsOpen != 1 {
		t.Fatalf("population axes mixed: %+v", p)
	}
	if ratio := p.Ratios["core.failed_run_fraction"]; ratio.Numerator != 1 || ratio.Denominator != 4 || ratio.Value == nil || *ratio.Value != .25 {
		t.Fatalf("failed fraction: %+v", ratio)
	}
	if ratio := p.Ratios["core.succeeded_run_fraction"]; ratio.Numerator != 2 || ratio.Denominator != 4 {
		t.Fatalf("domain fail created a technical failure: %+v", ratio)
	}
	if ratio := p.Ratios["core.first_attempt_pass_fraction"]; ratio.Numerator != 1 || ratio.Denominator != 4 {
		t.Fatalf("first attempt denominator ignored failed/cancelled settlement: %+v", ratio)
	}
	if p.StepVerdict["fail"] != 1 || p.RunStatus["failed"] != 1 {
		t.Fatal("verdict and lifecycle collapsed")
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"SECRET-RESULT-BODY", "token_hash", "telemetry_cursor_key", "executable", "envelope"} {
		if bytes.Contains(b, []byte(forbidden)) {
			t.Fatalf("private payload leaked: %s", forbidden)
		}
	}
}

func TestTelemetryWarningCoverageUsesDeclaredClosedChannel(t *testing.T) {
	e, _, command := publicationFixture(t, nil)
	base, _ := publicationRun(t, e, command)
	ids := []string{}
	for i := 0; i < 4; i++ {
		r := telemetryHistoryRun(t, base, fmt.Sprintf("warning-%d", i), "completed", 10)
		a := r.Attempts["attempt:"+fmt.Sprintf("warning-%d", i)]
		if i > 0 {
			r.Publications = []Publication{{ID: "publication:" + fmt.Sprint(i), AttemptID: a.ID, StepID: a.StepID, Hook: "warning_raised", Kind: "event", EventKey: "warning:event", Value: json.RawMessage(`{"phase":"working"}`), Digest: rawDigest([]byte(`{"phase":"working"}`)), Received: timingObservation(5), Actor: "publisher"}}
		}
		if i == 2 {
			a.ProcessOutcome = nil
		}
		if i == 3 {
			r.Gaps = []TimingGap{{From: timingObservation(2), To: timingObservation(3), Reason: "crash_gap"}}
		}
		telemetrySaveRun(t, e, r, 0)
		ids = append(ids, r.ID)
	}
	q := telemetryQuery("aggregate", "core.warning_run_fraction", "step.quality_warnings")
	q.RunIDs = ids
	r := telemetryReport(t, e, q)
	ratio := r.Population.Ratios["core.warning_run_fraction"]
	if ratio.Numerator != 1 || ratio.Denominator != 2 || ratio.Value == nil || *ratio.Value != .5 || r.Population.WarnedIncomplete != 2 || r.Population.FullWarningCoverage != 2 {
		t.Fatalf("warning coverage denominator inflated: %+v", r.Population)
	}
	if r.Population.RunStatus["completed"] != 4 {
		t.Fatal("warning changed lifecycle")
	}
}

func TestTelemetryExactDistributionAndFixedKnowledgeCut(t *testing.T) {
	e, _, command := publicationFixture(t, nil)
	base, _ := publicationRun(t, e, command)
	ids := []string{}
	for i, value := range []int64{1, 2, 3, 4, 100} {
		r := telemetryHistoryRun(t, base, fmt.Sprintf("duration-%d", i), "completed", value)
		r.CoreBuild = "build-a"
		if i >= 2 {
			r.CoreBuild = "build-b"
		}
		telemetrySaveRun(t, e, r, 0)
		ids = append(ids, r.ID)
	}
	q := telemetryQuery("aggregate", "timing.elapsed")
	q.RunIDs = ids
	q.Filters.Scope = []string{"run"}
	q.GroupBy = []string{"core_build"}
	r := telemetryReport(t, e, q)
	if len(r.Aggregates) != 2 || r.Aggregates[0].N+r.Aggregates[1].N != 5 {
		t.Fatalf("weighted groups: %+v", r.Aggregates)
	}
	q.GroupBy = nil
	all := telemetryReport(t, e, q)
	g := telemetryGroup(t, all, "timing.elapsed")
	if g.N != 5 || g.Sum == nil || *g.Sum != 110 || g.Mean == nil || *g.Mean != 22 || g.P50 == nil || *g.P50 != 3 || g.P95 == nil || *g.P95 != 100 || g.QuantileMethod != "exact_nearest_rank" {
		t.Fatalf("raw observations were not merged exactly: %+v", g)
	}
	q.Cut = &all.Cut
	before, _ := json.Marshal(telemetryReport(t, e, q))
	future := telemetryHistoryRun(t, base, "duration-future", "completed", 900)
	telemetrySaveRun(t, e, future, 0)
	after, _ := json.Marshal(telemetryReport(t, e, q))
	if !bytes.Equal(before, after) {
		t.Fatal("future facts changed a fixed-cut report")
	}
}

func TestTelemetryUTCRollbackSurvivesJournalReplay(t *testing.T) {
	e, _, command := publicationFixture(t, nil)
	base, _ := publicationRun(t, e, command)
	r := telemetryHistoryRun(t, base, "rollback", "completed", 15000)
	a, step := r.Attempts["attempt:rollback"], r.Steps["step:rollback"]
	a.Admitted, a.Started = timingObservation(5000), timingPoint(5000)
	a.ExecutorEnd, a.CandidateAt = timingPoint(13000), timingPoint(12000)
	r.Transitions = []StateChange{
		{Kind: "step", ID: step.ID, To: "ready", At: timingObservation(0)},
		{Kind: "step", ID: step.ID, From: "ready", To: "running", At: timingObservation(5000)},
		{Kind: "step", ID: step.ID, From: "running", To: "completed", At: timingObservation(15000)},
	}
	telemetrySaveRun(t, e, r, 0)
	// These are explicit clock observations, not a change to the host clock.
	// A later journal revision has earlier UTC but monotonically ordered ticks.
	back := timingObservation(15000)
	back.UTC = timingObservation(-2000).UTC
	r.LastObserved, r.Settled = back, &back
	r.Activations["activation:rollback"].Settled, step.Settled, a.Settled = &back, &back, &back
	r.Transitions[2].At = back
	a.ExecutorEnd.UTC = timingObservation(1000).UTC
	cut := telemetrySaveRun(t, e, r, 1)
	q := telemetryQuery("records", "timing.elapsed", "timing.executor_time")
	q.RunIDs, q.Cut = []string{r.ID}, &cut
	before, err := json.Marshal(telemetryReport(t, e, q))
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	recovered, err := Open(e.Root, true)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	ctx := context.Background()
	if err := recovered.Store.Verify(ctx); err != nil {
		t.Fatal(err)
	}
	view, err := recovered.Store.ReadAt(ctx, r.ID, cut, 0, 100)
	if err != nil || len(view.Events) != 2 || view.Events[0].Seq != 1 || view.Events[1].Seq != 2 || !bytes.Equal(view.Snapshot.Data, view.Events[1].StateAfter) {
		t.Fatal("UTC reordered journal history or replay changed the committed projection", err)
	}
	var replayed Run
	if err := decodeState(view.Events[1].StateAfter, &replayed); err != nil {
		t.Fatal(err)
	}
	timing := Timing(replayed, replayed.LastObserved, false)
	duration := timing.Root.Metrics["elapsed"]
	if duration.Quality != "partial" || duration.ValueMS != nil || duration.KnownMS == nil || *duration.KnownMS != 15000 || !strings.Contains(strings.Join(duration.Reasons, ","), "utc_rollback") {
		t.Fatalf("replayed rollback became a negative or measured calendar duration: %+v", duration)
	}
	timingMeasured(t, timingFind(t, timing.Root, a.ID).Metrics["executor_time"], 8000, false)
	timingMeasured(t, timingFind(t, timing.Root, step.ID).StateTime["ready"], 5000, false)
	after, err := json.Marshal(telemetryReport(t, recovered, q))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("replay changed the fixed-cut timing report", err)
	}
}

func TestTelemetryCountersGaugesEventsAndMissingReplacement(t *testing.T) {
	e, token, command := publicationFixture(t, func(step *flow.StepDefinition) {
		step.Hooks["gauge_changed"] = step.Hooks["progress_changed"]
		step.Telemetry = append(step.Telemetry, flow.Mapping{Name: "queue_depth", Revision: "1.0.0", Description: "Current queue", Hook: "gauge_changed", Kind: "gauge", Field: "/completed", Unit: "1", Aggregation: "last", Reset: "none", Dimensions: map[string]string{}}, flow.Mapping{Name: "latency", Revision: "1.0.0", Description: "Latency observations", Hook: "warning_raised", Kind: "distribution", Field: "/completed", Unit: "ms", Aggregation: "observations", Reset: "none", Dimensions: map[string]string{}})
	})
	for i, value := range []string{"NaN", "1e309", "9007199254740992", "-1", "1.5", `"not-a-number"`} {
		c := command
		c.CommandID = fmt.Sprintf("command:invalid-raw-meter-%d", i)
		c.Value = json.RawMessage(`{"phase":"working","completed":` + value + `}`)
		if _, err := e.Publish(context.Background(), token, c); err == nil {
			t.Fatalf("raw nonfinite/overflow/invalid counter crossed publication intake: %s", value)
		}
	}
	// These ordinary values must still start at state version zero: a rejected
	// number cannot consume a publication version or become a hidden zero.
	for i, value := range []int{10, 13, 13} {
		c := command
		version := int64(i)
		c.CommandID = fmt.Sprintf("command:counter-%d", i)
		c.ExpectedStateVersion = &version
		c.Value = json.RawMessage(fmt.Sprintf(`{"phase":"working","completed":%d}`, value))
		if _, err := e.Publish(context.Background(), token, c); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		c := command
		version := int64(i)
		c.Hook = "gauge_changed"
		c.CommandID = fmt.Sprintf("command:gauge-%d", i)
		c.ExpectedStateVersion = &version
		c.Value = json.RawMessage(`{"phase":"working","completed":42}`)
		if _, err := e.Publish(context.Background(), token, c); err != nil {
			t.Fatal(err)
		}
	}
	for i, key := range []string{"event:x", "event:x", "event:y"} {
		c := command
		c.Hook, c.Kind, c.EventKey, c.ExpectedStateVersion = "warning_raised", "event", key, nil
		c.CommandID = fmt.Sprintf("command:event-%d", i)
		c.Value = json.RawMessage(`{"phase":"working","completed":3}`)
		if _, err := e.Publish(context.Background(), token, c); err != nil {
			t.Fatal(err)
		}
	}
	q := telemetryQuery("aggregate", "step.processed_total", "step.queue_depth", "step.quality_warnings", "step.latency")
	q.RunIDs = []string{command.RunID}
	r := telemetryReport(t, e, q)
	counter := telemetryGroup(t, r, "step.processed_total")
	gauge := telemetryGroup(t, r, "step.queue_depth")
	events := telemetryGroup(t, r, "step.quality_warnings")
	latency := telemetryGroup(t, r, "step.latency")
	if counter.Total == nil || *counter.Total != 13 || counter.Delta == nil || *counter.Delta != 3 {
		t.Fatalf("cumulative samples added: %+v", counter)
	}
	if gauge.Last == nil || *gauge.Last != 42 || gauge.Sum != nil || gauge.N != 2 {
		t.Fatalf("gauge summed/repeated: %+v", gauge)
	}
	if events.Total == nil || *events.Total != 2 || latency.N != 2 || latency.Sum == nil || *latency.Sum != 6 {
		t.Fatalf("event occurrence dedup: %+v %+v", events, latency)
	}
	version := int64(3)
	command.CommandID = "command:omit"
	command.ExpectedStateVersion = &version
	command.Value = json.RawMessage(`{"phase":"working"}`)
	if _, err := e.Publish(context.Background(), token, command); err != nil {
		t.Fatal(err)
	}
	missing := telemetryGroup(t, telemetryReport(t, e, q), "step.processed_total")
	if missing.Total != nil || missing.Delta != nil || missing.Coverage.Unavailable != 1 {
		t.Fatalf("absent field became zero or previous current value: %+v", missing)
	}
	q.Aggregations = []string{"sum"}
	if _, err := e.Telemetry(context.Background(), q); err == nil {
		t.Fatal("incompatible gauge/counter aggregation accepted")
	}
}

func TestTelemetryCohortEventAndCompletionWindows(t *testing.T) {
	e, _, command := publicationFixture(t, nil)
	base, _ := publicationRun(t, e, command)
	r := telemetryHistoryRun(t, base, "window", "running", 5)
	earlyCut := telemetrySaveRun(t, e, r, 0)
	r.Status = "failed"
	r.Active = nil
	r.Settled = timingPoint(15)
	r.LastObserved = timingObservation(15)
	for _, a := range r.Attempts {
		a.Status = "failed"
		a.Settled = r.Settled
		a.ExecutorEnd = r.Settled
	}
	for _, s := range r.Steps {
		s.Status = "failed"
		s.Settled = r.Settled
	}
	for _, a := range r.Activations {
		a.Status = "failed"
		a.Settled = r.Settled
	}
	r.Diagnostics = []Diagnostic{{ID: "diagnostic:late", RunID: r.ID, Origin: "core", Severity: "error", Code: "late_error", Observed: timingObservation(15)}}
	lateCut := telemetrySaveRun(t, e, r, 1)
	q := telemetryQuery("aggregate", "core.diagnostics")
	q.RunIDs = []string{r.ID}
	q.CreatedBefore = timingObservation(10).UTC
	q.Cut = &lateCut
	cohort := telemetryReport(t, e, q)
	if g := telemetryGroup(t, cohort, "core.diagnostics"); g.Total == nil || *g.Total != 1 || cohort.Population.Terminal != 1 {
		t.Fatalf("late outcome outside creation window was dropped: %+v", cohort)
	}
	q.EventBefore = timingObservation(10).UTC
	window := telemetryReport(t, e, q)
	if window.RecordCount != 0 || window.Population.Terminal != 1 {
		t.Fatal("event window changed cohort or included late occurrence")
	}
	q.EventBefore = ""
	q.CreatedBefore = ""
	q.CompletedFrom = timingObservation(10).UTC
	q.CompletedBefore = timingObservation(20).UTC
	completed := telemetryReport(t, e, q)
	if completed.Population.Matched != 1 || completed.Population.Basis != "completed_within_cohort" {
		t.Fatal("completed-within population failed")
	}
	q.CompletedFrom = ""
	q.CompletedBefore = ""
	q.Cut = &earlyCut
	early := telemetryReport(t, e, q)
	if early.Population.Open != 1 || early.Population.Terminal != 0 || early.RecordCount != 0 {
		t.Fatal("future settlement leaked before knowledge cut")
	}
}

func TestCoreTelemetryPreparationFailureAttribution(t *testing.T) {
	e, workflow := coreDriverFixture(t, "pass")
	before := telemetryReport(t, e, telemetryQuery("records", "core.diagnostics"))
	if before.CalculatorRevision != TelemetryCalculatorRevision || len(before.AsOf) != 1 {
		t.Fatalf("F1-only history used the current project profile: %+v", before)
	}
	legacyID := before.AsOf[0].RunID
	runID := coreDriverStart(t, e, workflow)
	r := driverRun(t, e, runID)
	artifact, contents, err := e.Artifact(r.Inputs["source"])
	if err != nil || string(contents) != "pinned input" {
		t.Fatalf("input was not sealed before the fault: %q %v", contents, err)
	}
	blob := filepath.Join(e.Root, e.Config.Configuration.ArtifactRoot, strings.TrimPrefix(artifact.Digest, "sha256:"))
	if err := os.Chmod(blob, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blob, []byte("corrupt sealed input"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	r = driverRun(t, e, runID)
	activation := activationFor(&r, "work")
	if r.Status != "completed" || r.Outcome == nil || *r.Outcome != "rejected" || len(r.Attempts) != 0 || activation == nil || len(r.Diagnostics) != 1 {
		t.Fatalf("preparation failure did not recover before admission: %+v", r)
	}
	diagnostic := r.Diagnostics[0]
	if diagnostic.ActivationID != activation.ID || diagnostic.AttemptID != "" {
		t.Fatalf("preparation diagnostic lost its actual subject: %+v", diagnostic)
	}
	step := r.Steps[activation.StepID]
	query := telemetryQuery("records", "core.diagnostics")
	query.RunIDs = []string{runID}
	query.Filters.StepID = []string{step.Ref.ID}
	report := telemetryReport(t, e, query)
	if report.CalculatorRevision != TelemetryCalculatorRevisionCore || report.RecordCount != 1 || len(report.Records) != 1 {
		t.Fatalf("step filter dropped a recoverable preparation failure: %+v", report)
	}
	record := report.Records[0]
	wantSubject := TelemetrySubject{Kind: "step_instance", ID: step.ID, RunID: runID, StepInstanceID: step.ID}
	if record.Subject != wantSubject || record.Dimensions["step_id"] != step.Ref.ID || record.Dimensions["step_revision"] != step.Ref.String() || record.Dimensions["stage_id"] != "work" || record.Integer == nil || *record.Integer != 1 || len(record.Evidence) != 1 || record.Evidence[0] != diagnostic.ID {
		t.Fatalf("incorrect preparation diagnostic attribution: %+v", record)
	}
	view, err := e.Store.Read(context.Background(), runID, 0, 100)
	if err != nil || view.More {
		t.Fatalf("read complete diagnostic history: %v", err)
	}
	recorded := 0
	for _, event := range view.Events {
		if event.Type != "diagnostic.recorded" {
			continue
		}
		recorded++
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(event.Data, &payload); err != nil {
			t.Fatal(err)
		}
		if event.Version != 1 || payload["stage_activation_id"] != nil {
			t.Fatal("diagnostic event v1 was extended without a new contract")
		}
	}
	if recorded != 1 {
		t.Fatalf("expected one durable diagnostic event, got %d", recorded)
	}
	query.RunIDs = []string{legacyID}
	if got := telemetryReport(t, e, query).CalculatorRevision; got != TelemetryCalculatorRevision {
		t.Fatalf("F1-only cohort changed calculator: %s", got)
	}
	query.RunIDs = nil
	query.Cut = &before.Cut
	if got := telemetryReport(t, e, query).CalculatorRevision; got != TelemetryCalculatorRevision {
		t.Fatalf("a Core Run leaked into an earlier cut: %s", got)
	}
	query.Cut = nil
	if got := telemetryReport(t, e, query); got.CalculatorRevision != TelemetryCalculatorRevisionCore || got.Population.Matched != 2 {
		t.Fatalf("mixed cohort did not select the Core calculator: %+v", got)
	}
}

func TestCoreTelemetryControlStageDiagnostic(t *testing.T) {
	e, options := emptyRuntime(t)
	definitions, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	e.Config.ConfigurationSchemaRef = builtinRef(definitions, "core:schema/core-configuration")
	e.Config.Configuration.SchemaVersion = CoreConfigVersion
	e.Config.Configuration.SemanticsProfile = flow.CoreProfile
	writeRuntimeJSON(t, filepath.Join(e.Root, "prifly.json"), e.Config)
	ctx := context.Background()
	started, err := e.Start(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	r, view, err := e.load(ctx, started.Receipt.RunID)
	if err != nil {
		t.Fatal(err)
	}
	activation := activationFor(&r, "done")
	if activation == nil || activation.Kind != "finish" || len(r.Steps) != 0 || len(r.Attempts) != 0 {
		t.Fatal("fixture has executable entities instead of a control stage")
	}
	diagnostic := Diagnostic{ID: "diagnostic:control-error", RunID: r.ID, ActivationID: activation.ID, Origin: "core", Severity: "error", Code: "projection_schema_invalid", Category: "executor", Phase: "preparation", Message: "Control output projection failed", CauseRefs: []string{}}
	// Store real diagnostic evidence for the telemetry projection. The actual
	// finish preparation failure path is covered by the driver regression.
	_, err = e.apply(ctx, e.owner, "command:control-error", r.ID, "stage.failed", map[string]any{"stage_activation_id": activation.ID}, &view.Snapshot.Version, local.CommandCAS, func(saved *Run, _ local.Snapshot, obs Observation) (local.Change, error) {
		a := saved.Activations[activation.ID]
		a.Status, a.Settled = "failed", &obs
		saved.Status, saved.Settled, saved.Ready = "failed", &obs, []string{}
		diagnostic.Observed = obs
		return local.Change{}, recordDiagnostic(saved, diagnostic)
	})
	if err != nil {
		t.Fatal(err)
	}
	query := telemetryQuery("records", "core.diagnostics")
	query.RunIDs = []string{r.ID}
	query.Filters.Scope = []string{"stage_activation"}
	report := telemetryReport(t, e, query)
	if report.CalculatorRevision != TelemetryCalculatorRevisionCore || len(report.Records) != 1 {
		t.Fatalf("control diagnostic did not select its stage scope: %+v", report)
	}
	record := report.Records[0]
	if record.Subject != (TelemetrySubject{Kind: "stage_activation", ID: activation.ID, RunID: r.ID}) || record.Dimensions["stage_id"] != "done" || record.Integer == nil || *record.Integer != 1 {
		t.Fatalf("incorrect control diagnostic attribution: %+v", record)
	}
	for _, label := range []string{"step_id", "step_revision"} {
		if _, exists := record.Dimensions[label]; exists {
			t.Fatalf("control stage acquired an invented %s", label)
		}
	}
	if err := validatePublic(t, "TelemetryReport", report); err != nil {
		t.Fatal("existing telemetry contract rejected the control subject", err)
	}
	for _, linkage := range []string{"step", "attempt"} {
		t.Run("reject_control_"+linkage, func(t *testing.T) {
			bad, a, d := r, *activation, diagnostic
			bad.Activations = map[string]*Activation{a.ID: &a}
			if linkage == "step" {
				a.StepID = "step:invented"
			} else {
				d.AttemptID = "attempt:invented"
				bad.Attempts = map[string]*Attempt{d.AttemptID: {ID: d.AttemptID, ActivationID: a.ID}}
			}
			rejectionCode(t, recordDiagnostic(&bad, d), "diagnostic_identity_mismatch")
		})
	}
}

func telemetrySamples(t *testing.T, e *Engine, values []float64) (int64, error) {
	t.Helper()
	batch := []local.SampleInput{}
	for i, value := range values {
		data, err := json.Marshal(TelemetrySampleData{SchemaVersion: "telemetry-sample/1", Metric: "core.command_duration", Value: &value, Unit: "ms", Method: "runtime_observation", Quality: "measured", Coverage: "sample", Observed: timingObservation(int64(i + 1)), CommandID: fmt.Sprintf("command:sample-%d", i)})
		if err != nil {
			return 0, err
		}
		batch = append(batch, local.SampleInput{ID: newID("sample"), Data: data})
	}
	return e.Store.AppendSamples(context.Background(), batch)
}
func TestTelemetrySavedSamplesMissingMetersAndCrashGap(t *testing.T) {
	e := artifactEngine(t)
	cut, err := telemetrySamples(t, e, []float64{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := telemetrySamples(t, e, []float64{100}); err != nil {
		t.Fatal(err)
	}
	q := telemetryQuery("aggregate", "core.command_duration")
	q.Cut = &cut
	r := telemetryReport(t, e, q)
	g := telemetryGroup(t, r, "core.command_duration")
	if g.N != 2 || g.Sum == nil || *g.Sum != 3 || !g.Coverage.ExpectedUnknown || !g.Coverage.LossUnknown || g.Quality != "partial" {
		t.Fatalf("sample coverage or fixed cut invented: %+v", g)
	}
	q = telemetryQuery("aggregate", "os.rss")
	missing := telemetryGroup(t, telemetryReport(t, e, q), "os.rss")
	if missing.Quality != "unavailable" || missing.Last != nil || missing.Total != nil || len(missing.Reasons) != 1 || missing.Reasons[0] != "unsupported_meter" {
		t.Fatalf("RSS reconstructed: %+v", missing)
	}
	if _, err := telemetrySamples(t, e, []float64{math.NaN()}); err == nil {
		t.Fatal("NaN accepted")
	}
	data := json.RawMessage(`{"schema_version":"telemetry-sample/1","metric":"core.command_duration","value":1,"unit":"bytes","method":"runtime_observation","quality":"measured","coverage":"sample","observed":{"utc":"2026-08-28T00:00:00Z","session":"fixture","monotonic_ms":0,"source":"fixture","suspend_basis":"includes_suspend","utc_trust":"trusted"}}`)
	if _, err := e.Store.AppendSamples(context.Background(), []local.SampleInput{{ID: "sample:wrong-unit", Data: data}}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Telemetry(context.Background(), telemetryQuery("aggregate", "core.command_duration")); err == nil {
		t.Fatal("wrong unit silently aggregated")
	}
	t.Run("unsupported CPU meter profile", func(t *testing.T) {
		// Deliberately controlled accounting fixture, not a second native OS
		// qualification. Real positive CPU is exercised by the driver hash-loop
		// test; this profile preserves exit/time facts but supplies no CPU meter.
		engine, _, command := publicationFixture(t, nil)
		base, _ := publicationRun(t, engine, command)
		run := telemetryHistoryRun(t, base, "without-cpu-meter", "completed", 1000)
		var attempt *Attempt
		for _, a := range run.Attempts {
			attempt = a
		}
		attempt.ProcessOutcome.Started = true
		attempt.ProcessOutcome.CPU = nil
		telemetrySaveRun(t, engine, run, 0)
		query := telemetryQuery("records", "os.cpu_user", "os.cpu_system", "os.cpu_total", "timing.executor_time")
		query.RunIDs = []string{run.ID}
		query.Filters.Scope = []string{"attempt"}
		report := telemetryReport(t, engine, query)
		seen := map[string]bool{}
		for _, record := range report.Records {
			if record.Subject.ID != attempt.ID {
				t.Fatal("resource profile lost the exact attempt scope")
			}
			seen[record.Metric] = true
			if record.Metric == "timing.executor_time" {
				if record.Value == nil || *record.Value != 1000 || record.Unit != "ms" || record.Quality != "measured" {
					t.Fatalf("missing CPU erased known fixture executor time: %+v", record)
				}
			} else if record.Value != nil || record.Integer != nil || record.Unit != "s" || record.Quality != "unavailable" || len(record.Reasons) != 1 || record.Reasons[0] != "os_accounting_unavailable" {
				t.Fatalf("unsupported CPU meter became zero/elapsed/a fake measurement: %+v", record)
			}
		}
		if len(seen) != 4 {
			t.Fatalf("unsupported fields were omitted instead of labelled unavailable: %v", seen)
		}
	})
}

func TestTelemetryPaginationBoundToOwnerQueryAndCut(t *testing.T) {
	e, _, command := publicationFixture(t, nil)
	base, _ := publicationRun(t, e, command)
	ids := []string{}
	for i := 0; i < 3; i++ {
		r := telemetryHistoryRun(t, base, fmt.Sprintf("page-%d", i), "completed", 1)
		telemetrySaveRun(t, e, r, 0)
		ids = append(ids, r.ID)
	}
	q := telemetryQuery("records", "core.entities_created")
	q.RunIDs = ids
	q.Filters.Scope = []string{"run"}
	q.Limit = 1
	first := telemetryReport(t, e, q)
	if first.NextCursor == "" || len(first.Records) != 1 || first.RecordCount != 3 {
		t.Fatalf("first page: %+v", first)
	}
	future := telemetryHistoryRun(t, base, "future-page", "completed", 2)
	telemetrySaveRun(t, e, future, 0)
	q.Cursor = first.NextCursor
	second := telemetryReport(t, e, q)
	if second.Cut != first.Cut || len(second.Records) != 1 || second.Records[0].ID == first.Records[0].ID {
		t.Fatal("page membership or cut changed")
	}
	bad := q
	bad.Metrics = []string{"core.entities_settled"}
	if _, err := e.Telemetry(context.Background(), bad); err == nil {
		t.Fatal("cursor reused with changed filters")
	}
	bad = q
	parts := strings.Split(bad.Cursor, ".")
	parts[1] = "A" + parts[1][1:]
	if parts[1] == strings.Split(bad.Cursor, ".")[1] {
		parts[1] = "B" + parts[1][1:]
	}
	bad.Cursor = strings.Join(parts, ".")
	if _, err := e.Telemetry(context.Background(), bad); err == nil {
		t.Fatal("forged cursor accepted")
	}
	installationPath := filepath.Join(e.Root, ".prifly/installation.json")
	saved, err := os.ReadFile(installationPath)
	if err != nil {
		t.Fatal(err)
	}
	var installation Installation
	if err := json.Unmarshal(saved, &installation); err != nil {
		t.Fatal(err)
	}
	installation.OwnerUID++
	changed, _ := json.Marshal(installation)
	if err := os.WriteFile(installationPath, changed, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Telemetry(context.Background(), q); err == nil {
		t.Fatal("cursor bypassed current access")
	}
	if err := os.WriteFile(installationPath, saved, 0600); err != nil {
		t.Fatal(err)
	}
	readOnly, err := Open(e.Root, true)
	if err != nil {
		t.Fatal(err)
	}
	defer readOnly.Close()
	_, before, err := e.Store.ReadAll(context.Background(), 1000)
	if err != nil {
		t.Fatal(err)
	}
	telemetryReport(t, readOnly, telemetryQuery("aggregate", "core.commands"))
	_, after, err := e.Store.ReadAll(context.Background(), 1000)
	if err != nil || before != after {
		t.Fatal("read-only query advanced state or collected samples")
	}
}

func TestTelemetryClosedQueriesLimitsAndCancellation(t *testing.T) {
	e := artifactEngine(t)
	for _, input := range []string{`{"schema_version":"telemetry-query/1","mode":"catalog","refresh":true}`, `{"schema_version":"telemetry-query/1","mode":"catalog","filters":{"nonsense":[]}}`, `{"schema_version":"telemetry-query/1","mode":"catalog","mode":"records"}`, `{"schema_version":"telemetry-query/1","mode":"records","filters":null}`, `{"schema_version":"telemetry-query/1","mode":"records","filters":{"status":null}}`, `{"schema_version":"telemetry-query/1","mode":"records","limit":0}`} {
		var q TelemetryQuery
		if err := json.Unmarshal([]byte(input), &q); err == nil {
			t.Fatalf("unknown/duplicate field accepted: %s", input)
		}
	}
	for _, q := range []TelemetryQuery{{SchemaVersion: TelemetryQueryVersion, Mode: "compare"}, {SchemaVersion: "1", Mode: "catalog"}, {SchemaVersion: TelemetryQueryVersion, Mode: "aggregate", Metrics: []string{"not.a.metric"}}, {SchemaVersion: TelemetryQueryVersion, Mode: "aggregate", Metrics: []string{"core.commands"}, Aggregations: []string{"mean"}}, {SchemaVersion: TelemetryQueryVersion, Mode: "records", Limit: 1001}} {
		if _, err := e.Telemetry(context.Background(), q); err == nil {
			t.Fatalf("invalid query accepted: %+v", q)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.Telemetry(ctx, telemetryQuery("catalog")); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled query continued: %v", err)
	}
	catalog := telemetryReport(t, e, telemetryQuery("catalog"))
	if len(catalog.Descriptors) == 0 || catalog.Cut != 0 || catalog.Population.Ratios["core.failed_run_fraction"].Value != nil {
		t.Fatal("empty authority claimed measurements or omitted catalog")
	}
}

func TestTelemetryCounterLabelsCannotCreateNewAdditiveSource(t *testing.T) {
	e, token, command := publicationFixture(t, nil)
	for i, payload := range []string{`{"phase":"working","completed":10}`, `{"phase":"finished","completed":13}`} {
		c := command
		v := int64(i)
		c.CommandID = fmt.Sprintf("command:label-%d", i)
		c.ExpectedStateVersion = &v
		c.Value = json.RawMessage(payload)
		if _, err := e.Publish(context.Background(), token, c); err != nil {
			t.Fatal(err)
		}
	}
	q := telemetryQuery("aggregate", "step.processed_total")
	q.RunIDs = []string{command.RunID}
	q.GroupBy = []string{"custom.phase"}
	report := telemetryReport(t, e, q)
	if len(report.Aggregates) != 1 {
		t.Fatal("label change created overlapping additive counter sources")
	}
	g := report.Aggregates[0]
	if g.Total == nil || *g.Total != 13 || g.Delta == nil || *g.Delta != 3 || g.Dimensions["custom.phase"] != `"finished"` {
		t.Fatalf("counter grouped as two reset generations: %+v", g)
	}
	q.Filters.Dimensions = map[string][]json.RawMessage{"phase": {json.RawMessage(`"finished"`)}}
	filtered := telemetryGroup(t, telemetryReport(t, e, q), "step.processed_total")
	if filtered.Total == nil || *filtered.Total != 13 || filtered.Delta != nil {
		t.Fatalf("one selected reading invented a window increment: %+v", filtered)
	}
}

func TestTelemetryAggregateLimitAndDescriptorIdentity(t *testing.T) {
	e, _, command := publicationFixture(t, nil)
	base, _ := publicationRun(t, e, command)
	other, _, otherCommand := publicationFixture(t, func(step *flow.StepDefinition) {
		step.ID = "test:step/other-progress"
		step.Telemetry[0].Revision = "2.0.0"
	})
	otherBase, _ := publicationRun(t, other, otherCommand)
	r1 := telemetryHistoryRun(t, base, "revision-one", "completed", 10)
	r2 := telemetryHistoryRun(t, otherBase, "revision-two", "completed", 10)
	r2.AuthorityID = e.Installation.ID
	r2.ProjectID = e.Config.ID
	telemetrySaveRun(t, e, r1, 0)
	telemetrySaveRun(t, e, r2, 0)
	q := telemetryQuery("aggregate", "step.processed_total")
	q.RunIDs = []string{r1.ID, r2.ID}
	report := telemetryReport(t, e, q)
	if len(report.Aggregates) != 2 || report.Aggregates[0].DescriptorID == report.Aggregates[1].DescriptorID {
		t.Fatal("different pinned descriptor revisions merged")
	}
	q.Limit = 1
	if partial, err := e.Telemetry(context.Background(), q); err == nil || len(partial.Aggregates) != 0 {
		t.Fatalf("group limit returned a successful partial aggregate: %+v %v", partial, err)
	}
}

// Exercise real command handlers and the actual best-effort collector. A SQL
// trigger fails one receipt write; no sample/report fixture supplies the answer.
func TestTelemetryActualCommandRequestsRejectionsAndPersistenceFailure(t *testing.T) {
	e, options := emptyRuntime(t)
	ctx := context.Background()
	started, err := e.Start(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	runID := started.Receipt.RunID
	firstSession := e.clock.session
	pause := RestrictCommand{SchemaVersion: "1", CommandID: "command:telemetry-pause", Scope: "run", ScopeID: runID, Kind: "pause", Reason: "Exercise measured control path"}
	stopped, err := e.Restrict(ctx, pause)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := e.Restrict(ctx, pause)
	if err != nil || !repeated.Duplicate || repeated.Receipt.Version != stopped.Receipt.Version {
		t.Fatalf("retry was not the original committed command: %+v %v", repeated, err)
	}
	staleID := "command:telemetry-stale"
	_, err = e.Resume(ctx, runID, staleID, "Stale UI command", started.Receipt.Version)
	rejectionCode(t, err, "version_conflict")
	beforeFailure, viewBefore, err := e.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	// The connection is only a test-owned fault injector against this temp DB.
	// Samples use a different table and can report the aborted command write.
	databaseURL := url.URL{Scheme: "file", Path: filepath.Join(e.Root, e.Config.Configuration.StateRoot, "state.sqlite3"), RawQuery: "mode=rw&_foreign_keys=on&_busy_timeout=500"}
	fault, err := sql.Open("sqlite3", databaseURL.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fault.Close() })
	if _, err := fault.ExecContext(ctx, `CREATE TRIGGER telemetry_fail_receipt BEFORE INSERT ON commands WHEN NEW.command_id='command:telemetry-persistence-failure' BEGIN SELECT RAISE(ABORT,'injected receipt persistence failure'); END`); err != nil {
		t.Fatal(err)
	}
	fail := pause
	fail.CommandID = "command:telemetry-persistence-failure"
	if _, err := e.Restrict(ctx, fail); err == nil || !persistenceFailure(err) {
		t.Fatalf("controlled persistence failure was acknowledged/misclassified: %v", err)
	}
	if _, err := fault.ExecContext(ctx, "DROP TRIGGER telemetry_fail_receipt"); err != nil {
		t.Fatal(err)
	}
	if err := fault.Close(); err != nil {
		t.Fatal(err)
	}
	afterFailure, viewAfter, err := e.load(ctx, runID)
	if err != nil || viewAfter.Snapshot.Version != viewBefore.Snapshot.Version || viewAfter.Snapshot.EventSeq != viewBefore.Snapshot.EventSeq || len(afterFailure.Stops) != len(beforeFailure.Stops) {
		t.Fatal("receipt failure left a partial restriction or transition", err)
	}
	if _, err := e.Receipt(ctx, fail.CommandID); !errors.Is(err, local.ErrNotFound) {
		t.Fatalf("failed write acquired a durable receipt: %v", err)
	}
	metrics := []string{"core.commands", "core.command_rejections", "core.command_requests", "core.command_duplicates", "core.persistence_failures", "core.command_duration", "core.lock_wait", "core.transaction_duration"}
	query := telemetryQuery("aggregate", metrics...)
	query.RunIDs = []string{runID}
	before := telemetryReport(t, e, query)
	query.Cut = &before.Cut
	saved, err := json.Marshal(telemetryReport(t, e, query))
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(e.Root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if reopened.clock.session == firstSession || reopened.Installation.ID != e.Installation.ID {
		t.Fatal("clock generation and authority identity were conflated")
	}
	last, err := reopened.Restrict(ctx, pause)
	if err != nil || !last.Duplicate || last.Receipt.EventSeq != stopped.Receipt.EventSeq {
		t.Fatalf("retry after reopen became another transition: %+v %v", last, err)
	}
	historical, err := json.Marshal(telemetryReport(t, reopened, query))
	if err != nil || !bytes.Equal(saved, historical) {
		t.Fatal("later request generation changed the fixed-cut history", err)
	}
	query.Cut = nil
	report := telemetryReport(t, reopened, query)
	// Six received requests: start, pause, pause duplicate, stale resume,
	// failed write, and the same pause duplicate in the new Engine clock session.
	for metric, want := range map[string]int64{"core.commands": 3, "core.command_rejections": 1, "core.command_requests": 6, "core.command_duplicates": 2, "core.persistence_failures": 1} {
		group := telemetryGroup(t, report, metric)
		if group.Total == nil || *group.Total != want {
			t.Fatalf("%s: wanted %d real observations, got %+v", metric, want, group)
		}
		if metric != "core.commands" && metric != "core.command_rejections" && (!group.Coverage.ExpectedUnknown || !group.Coverage.LossUnknown || group.Quality != "partial") {
			t.Fatalf("best-effort samples claimed complete request coverage: %+v", group)
		}
	}
	for _, metric := range []string{"core.command_duration", "core.lock_wait", "core.transaction_duration"} {
		group := telemetryGroup(t, report, metric)
		if group.N != 6 || group.Unit != "ms" || group.Method != "runtime_observation" || group.Sum == nil || *group.Sum < 0 || group.Mean == nil || group.Quality != "partial" {
			t.Fatalf("missing or dishonest command timing: %+v", group)
		}
	}
	query.Mode = "records"
	records := telemetryReport(t, reopened, query)
	requests := map[string]int{}
	for _, record := range records.Records {
		switch record.Metric {
		case "core.command_requests":
			if record.Observed == nil || record.Generation != record.Observed.Session || record.Generation != firstSession && record.Generation != reopened.clock.session || record.Integer == nil || *record.Integer != 1 {
				t.Fatalf("request has incorrect measured source generation: %+v", record)
			}
			requests[record.Subject.ID]++
			if record.Generation == reopened.clock.session && record.Subject.ID != pause.CommandID {
				t.Fatal("old observations were reassigned to the new clock generation")
			}
		case "core.commands", "core.command_rejections":
			if record.Generation != reopened.Installation.ID || record.Method != "durable_receipt" || record.Subject.ID == fail.CommandID {
				t.Fatalf("receipt fact invented or attributed to process generation: %+v", record)
			}
		case "core.persistence_failures":
			if record.Subject.ID != fail.CommandID || record.Generation != firstSession {
				t.Fatalf("semantic rejection was counted as persistence failure: %+v", record)
			}
		}
	}
	if len(requests) != 4 || requests[options.CommandID] != 1 || requests[pause.CommandID] != 3 || requests[staleID] != 1 || requests[fail.CommandID] != 1 {
		t.Fatalf("request/unique command identity lost: %v", requests)
	}
	_, cut, err := reopened.Store.ReadAll(ctx, 1000)
	if err != nil || cut != report.Cut {
		t.Fatal("telemetry queries recorded their own requests or changed knowledge cut", err)
	}
	if err := reopened.Store.Verify(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeAdmissionStorageBudgetRefusesBeforeSlotAndAllowsStop(t *testing.T) {
	e, runID := driverProject(t, "pass", 5000)
	ctx := context.Background()
	usage, err := e.Store.StorageUsage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Store.Close(); err != nil {
		t.Fatal(err)
	}
	// A small logical allocation limit exercises the real quota path. No host
	// disk is filled and no success/failure is injected into runtime admission.
	e.Store, err = local.OpenStore(filepath.Join(e.Root, e.Config.Configuration.StateRoot), local.StoreOptions{EventTypes: EventTypes, SoftLimitBytes: max(64<<10, usage.AllocatedBytes)})
	if err != nil {
		t.Fatal(err)
	}
	r, before, err := e.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := r.plan()
	if err != nil {
		t.Fatal(err)
	}
	err = e.admit(ctx, r, before, plan, activationFor(&r, "work"))
	rejectionCode(t, err, "storage_budget_exhausted")
	r, after, err := e.load(ctx, runID)
	if err != nil || before.Snapshot.Version != after.Snapshot.Version || before.Snapshot.EventSeq != after.Snapshot.EventSeq || !bytes.Equal(before.Snapshot.Data, after.Snapshot.Data) || len(r.Attempts) != 0 || len(r.Active) != 0 {
		t.Fatal("quota-refused admission left partial state or a pending attempt", err)
	}
	if slot, owner, err := e.Store.Slot(ctx); err != nil || slot != "" || owner != "" {
		t.Fatalf("refused admission acquired authority capacity: %q %q %v", slot, owner, err)
	}
	receipts, _, err := e.Store.ReadReceiptsAt(ctx, after.Cut, 100)
	if err != nil || len(receipts) != 2 || receipts[1].Rejection == nil || receipts[1].Rejection.Code != "storage_budget_exhausted" {
		t.Fatalf("admission rejection was not durable: %+v %v", receipts, err)
	}
	stopped, err := e.Restrict(ctx, RestrictCommand{SchemaVersion: "1", CommandID: "command:stop-above-quota", Scope: "run", ScopeID: runID, Kind: "pause", Reason: "Storage admission refused; preserve operator control"})
	if err != nil || stopped.Receipt.Version != before.Snapshot.Version+1 {
		t.Fatalf("optional-work quota denied mandatory control: %+v %v", stopped, err)
	}
	r, _, err = e.load(ctx, runID)
	if err != nil || len(r.Stops) != 1 || !r.restricted() || len(r.Attempts) != 0 {
		t.Fatal("successful stop was not committed after quota rejection", err)
	}
	if err := e.Store.Verify(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestTelemetryReadOnlyFixedCutWhileControlWriterCommits(t *testing.T) {
	e, runID := driverProject(t, "pass", 5000)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, start, err := e.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	// Reads must use pinned history even when a mutable execution source has
	// disappeared. This removes only this fixture's scratch file, not a worker
	// executable or an immutable blob.
	if err := os.Remove(filepath.Join(e.Root, "worker.data")); err != nil {
		t.Fatal(err)
	}
	reader, err := Open(e.Root, true)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if !reader.ReadOnly || !reader.Store.Info().ReadOnly {
		t.Fatal("query Engine did not open the authority read-only")
	}
	query := telemetryQuery("aggregate")
	query.RunIDs = []string{runID}
	initial, err := reader.Telemetry(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	readState := func() {
		t.Helper()
		next, err := reader.Next(ctx, runID) // CLI next and explain share this path.
		if err != nil || next.Admission || !next.ReadOnly {
			t.Fatal("read-only next/explain created an admission", err)
		}
		view, err := reader.View(ctx, runID)
		if err != nil || len(view.Run.Steps) != 1 || len(view.Run.Attempts) != 0 || view.DriverLive || view.Timing.Root.Metrics["executor_time"].ValueMS != nil {
			t.Fatal("read-only status/timing invented execution or a missing measurement", err)
		}
		events, err := reader.Events(ctx, runID, 0, 100)
		if err != nil || len(events.Events) == 0 {
			t.Fatal("read-only events failed without a mutable worker source", err)
		}
	}
	for i := 0; i < 3; i++ {
		readState()
	}
	_, unchangedCut, err := e.Store.ReadAll(ctx, 100)
	if err != nil || unchangedCut != initial.Cut {
		t.Fatal("ready-instance reads changed authority state before the explicit command", err)
	}
	query.Cut = &initial.Cut
	before, err := json.Marshal(telemetryReport(t, reader, query))
	if err != nil {
		t.Fatal(err)
	}
	requests, completed := make(chan int), make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case i := <-requests:
				_, err := e.Restrict(ctx, RestrictCommand{SchemaVersion: "1", CommandID: fmt.Sprintf("command:concurrent-control-%d", i), Scope: "run", ScopeID: runID, Kind: "pause", Reason: "Owner control while historical query is read"})
				completed <- err
			case <-ctx.Done():
				return
			}
		}
	}()
	defer func() { cancel(); <-done }()
	for i := 0; i < 4; i++ {
		select {
		case requests <- i:
		case <-ctx.Done():
			t.Fatal("control writer did not receive its command")
		}
		for j := 0; j < 2; j++ {
			readState()
			r, err := reader.Telemetry(ctx, query)
			if err != nil {
				t.Fatal(err)
			}
			after, err := json.Marshal(r)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("fixed-cut query observed an uncommitted or future control/sample", err)
			}
		}
		select {
		case err := <-completed:
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal("historical queries prevented control commit")
		}
	}
	r, view, err := e.load(ctx, runID)
	if err != nil || view.Snapshot.Version != start.Snapshot.Version+4 || len(r.Stops) != 4 || len(r.Attempts) != 0 {
		t.Fatal("reader created or lost a control transition", err)
	}
	// Each of those commands also records the state changes it made, so the
	// journal grows by more than one entry per command.
	if view.Snapshot.EventSeq <= start.Snapshot.EventSeq+4 {
		t.Fatalf("control commands recorded no state changes: %d to %d", start.Snapshot.EventSeq, view.Snapshot.EventSeq)
	}
	receipts, cut, err := e.Store.ReadReceiptsAt(ctx, -1, 100)
	if err != nil || len(receipts) != 5 {
		t.Fatalf("queries created command receipts: %+v %v", receipts, err)
	}
	query.Cut = nil
	latest, err := reader.Telemetry(ctx, query)
	if err != nil || latest.Cut != cut || latest.Population.RunStatus["waiting"] != 1 {
		t.Fatal("latest read does not reflect committed owner controls", err)
	}
	_, afterCut, err := e.Store.ReadAll(ctx, 100)
	if err != nil || afterCut != cut {
		t.Fatal("query collected samples or advanced knowledge cut", err)
	}
}

type telemetryCrashMarker struct {
	RunID        string          `json:"run_id"`
	Session      string          `json:"session"`
	EarlyCut     int64           `json:"early_cut"`
	FinalCut     int64           `json:"final_cut"`
	EarlyReport  json.RawMessage `json:"early_report"`
	FinalReport  json.RawMessage `json:"final_report"`
	QueuedSample string          `json:"queued_sample_id"`
}

// Only this harness instruments this extra validation call. Production's
// automatic collector is not claimed to collect validation_duration yet.
func telemetryMeasuredValidation(t *testing.T, e *Engine, runID, id string) local.SampleInput {
	t.Helper()
	data, err := readLocal(e.Root, "brief.json", MaxDefinitionBytes)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if err := flow.ValidateProtocol("RunBrief", data); err != nil {
		t.Fatal(err)
	}
	duration := float64(time.Since(started)) / float64(time.Millisecond)
	sample, err := canonical(TelemetrySampleData{SchemaVersion: "telemetry-sample/1", Metric: "core.validation_duration", Value: &duration, Unit: "ms", Method: "runtime_observation", Quality: "measured", Coverage: "sample", Observed: e.clock.now()})
	if err != nil {
		t.Fatal(err)
	}
	return local.SampleInput{ID: id, RunID: runID, Data: sample}
}

func telemetryCrashQuery(cut *int64) TelemetryQuery {
	q := telemetryQuery("aggregate", "core.commands", "core.validation_duration", "os.rss")
	q.Cut = cut
	return q
}

func TestTelemetryCrashPreservesFactsAndCutsButNotQueuedSamples(t *testing.T) {
	e, _ := emptyRuntime(t)
	root := e.Root
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(executable, "-test.run=^TestTelemetryCrashHelper$")
	child.Env = []string{"PRIFLY_TELEMETRY_CRASH_HELPER=" + root, "GORACE=atexit_sleep_ms=0"}
	var output bytes.Buffer
	child.Stdout, child.Stderr = &output, &output
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		if !waited {
			_ = child.Process.Kill()
			_ = child.Wait()
		}
	}()
	markerPath := filepath.Join(root, "telemetry-crash-ready.json")
	var marker telemetryCrashMarker
	deadline := time.Now().Add(10 * time.Second)
	for {
		data, err := os.ReadFile(markerPath)
		if err == nil {
			if err := json.Unmarshal(data, &marker); err != nil {
				t.Fatal(err)
			}
			break
		}
		if !errors.Is(err, os.ErrNotExist) || time.Now().After(deadline) {
			t.Fatalf("crash helper did not reach its prepared batch: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Kill only our live, unreaped child. This is a real unclean process exit,
	// not a reopen() call and not a state fixture with a pretend crash gap.
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	err = child.Wait()
	waited = true
	var exited *exec.ExitError
	if !errors.As(err, &exited) {
		t.Fatalf("helper did not crash: %v %s", err, output.String())
	}
	status, ok := exited.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() || status.Signal() != syscall.SIGKILL {
		t.Fatalf("helper did not reach the injected SIGKILL boundary: %v %s", err, output.String())
	}
	recovered, err := Open(root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	if recovered.clock.session == marker.Session {
		t.Fatal("recovery reused the lost sample source generation")
	}
	ctx := context.Background()
	r, _, err := recovered.load(ctx, marker.RunID)
	if err != nil || r.Status != "waiting" || len(r.Stops) != 1 || len(r.Attempts) != 0 {
		t.Fatal("committed lifecycle facts did not recover", err)
	}
	for _, point := range []struct {
		cut    int64
		report json.RawMessage
	}{
		{marker.EarlyCut, marker.EarlyReport},
		{marker.FinalCut, marker.FinalReport},
	} {
		got, err := json.Marshal(telemetryReport(t, recovered, telemetryCrashQuery(&point.cut)))
		if err != nil || !bytes.Equal(got, point.report) {
			t.Fatal("crash changed an already observed historical report", err)
		}
	}
	page, err := recovered.Store.ReadSamples(ctx, marker.FinalCut, 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	known := false
	for _, sample := range page.Records {
		if sample.ID == marker.QueuedSample {
			t.Fatal("a memory-only measurement appeared in the recovered store")
		}
		known = known || sample.ID == "sample:before-crash"
	}
	if !known {
		t.Fatal("committed sample batch disappeared on restart")
	}
	report := telemetryReport(t, recovered, telemetryCrashQuery(&marker.FinalCut))
	commands := telemetryGroup(t, report, "core.commands")
	measurements := telemetryGroup(t, report, "core.validation_duration")
	rss := telemetryGroup(t, report, "os.rss")
	if commands.Total == nil || *commands.Total != 2 || measurements.N != 1 || !measurements.Coverage.ExpectedUnknown || !measurements.Coverage.LossUnknown || measurements.Quality != "partial" {
		t.Fatal("fact counts or sample-loss coverage were reconstructed incorrectly")
	}
	if rss.Quality != "unavailable" || rss.Last != nil || rss.Sum != nil {
		t.Fatal("missing RSS was reconstructed from process timestamps")
	}
	future := telemetryMeasuredValidation(t, recovered, marker.RunID, "sample:after-recovery")
	if _, err := recovered.Store.AppendSamples(ctx, []local.SampleInput{future}); err != nil {
		t.Fatal(err)
	}
	old, err := json.Marshal(telemetryReport(t, recovered, telemetryCrashQuery(&marker.FinalCut)))
	if err != nil || !bytes.Equal(old, marker.FinalReport) {
		t.Fatal("a future generation leaked into the saved cut", err)
	}
	latest := telemetryReport(t, recovered, telemetryCrashQuery(nil))
	if telemetryGroup(t, latest, "core.validation_duration").N != 2 {
		t.Fatal("new generation was lost or the crashed batch was invented")
	}
	if err := recovered.Store.Verify(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestTelemetryCrashHelper(t *testing.T) {
	root := os.Getenv("PRIFLY_TELEMETRY_CRASH_HELPER")
	if root == "" {
		return
	}
	e, err := Open(root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	ctx := context.Background()
	start, err := e.Start(ctx, StartOptions{CommandID: "command:crash-start", WorkflowFile: "workflows/empty.json", BriefFile: "brief.json", Inputs: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	runID := start.Receipt.RunID
	saved := telemetryMeasuredValidation(t, e, runID, "sample:before-crash")
	if _, err := e.Store.AppendSamples(ctx, []local.SampleInput{saved}); err != nil {
		t.Fatal(err)
	}
	early := telemetryReport(t, e, telemetryCrashQuery(nil))
	earlyJSON, err := json.Marshal(early)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Restrict(ctx, RestrictCommand{SchemaVersion: "1", CommandID: "command:crash-pause", Scope: "run", ScopeID: runID, Kind: "pause", Reason: "Lifecycle fact must outlive diagnostic batch loss"}); err != nil {
		t.Fatal(err)
	}
	queued := []local.SampleInput{telemetryMeasuredValidation(t, e, runID, "sample:queued-but-not-appended")}
	final := telemetryReport(t, e, telemetryCrashQuery(nil))
	finalJSON, err := json.Marshal(final)
	if err != nil {
		t.Fatal(err)
	}
	marker, err := json.Marshal(telemetryCrashMarker{RunID: runID, Session: e.clock.session, EarlyCut: early.Cut, FinalCut: final.Cut, EarlyReport: earlyJSON, FinalReport: finalJSON, QueuedSample: queued[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := writeExclusive(filepath.Join(root, "telemetry-crash-ready.json"), marker); err != nil {
		t.Fatal(err)
	}
	// Test-only batching barrier. Parent SIGKILLs here. A bounded fallthrough
	// intentionally exits differently so a missed crash can never pass.
	time.Sleep(10 * time.Second)
	_, _ = e.Store.AppendSamples(ctx, queued)
	os.Exit(84)
}

// Measuring a command used to cost a second write transaction after it had
// already committed. The samples now ride the command's own transaction, so a
// command is measured exactly once, at the cut it committed.
func TestTelemetrySamplesRecordedWithCommand(t *testing.T) {
	e, options := emptyRuntime(t)
	ctx := context.Background()
	options.CommandID = newID("command")
	result, err := e.Start(ctx, options)
	if err != nil || result.Receipt.Version == 0 {
		t.Fatalf("the fixture is not a committed command: %+v %v", result, err)
	}
	if !result.SamplesRecorded {
		t.Fatal("the command did not record its own telemetry")
	}
	page, err := e.Store.ReadSamples(ctx, -1, 0, 100)
	if err != nil || page.More {
		t.Fatalf("bounded sample read: %v", err)
	}
	metrics := map[string]bool{}
	for _, row := range page.Records {
		if row.Cut != result.Receipt.Cut {
			t.Fatalf("a sample landed at another cut than its command: %d vs %d", row.Cut, result.Receipt.Cut)
		}
		var value TelemetrySampleData
		if err := decode(row.Data, &value); err != nil {
			t.Fatal(err)
		}
		metrics[value.Metric] = true
	}
	for _, metric := range []string{"core.command_requests", "core.command_duration", "core.lock_wait", "core.transaction_duration", "core.storage_bytes"} {
		if !metrics[metric] {
			t.Fatalf("the command's own batch lost %s: %+v", metric, metrics)
		}
	}
	// An exact repeat never reaches a transaction, so it is still measured by
	// the separate collector, and it says so.
	repeat, err := e.Start(ctx, options)
	if err != nil || !repeat.Duplicate || repeat.SamplesRecorded {
		t.Fatalf("a repeat was applied as a command or claimed a transaction: %+v %v", repeat, err)
	}
}

// A report reads every Run in the population. This is the regression that keeps
// that read linear: the population this build qualifies for must be answered
// inside the bound a person waits at a terminal, not eventually.
func TestTelemetryRegressionScansItsWholePopulation(t *testing.T) {
	e, _, command := publicationFixture(t, nil)
	base, _ := publicationRun(t, e, command)
	// The scan is bounded by records and by bytes, and for Runs of a realistic
	// size the byte budget is what stops first: a thousand of these would be
	// refused before any deadline could matter. This is the largest population
	// that fits, which is what the report has to answer for quickly.
	const population = 120
	for i := range population {
		telemetrySaveRun(t, e, telemetryHistoryRun(t, base, fmt.Sprintf("scale-%d", i), "completed", 10), 0)
	}
	// A selector names at most 64 Runs, so the population is the whole recorded
	// history rather than a list: that is the read this regression is about.
	query := telemetryQuery("aggregate", "core.command_duration")
	query.Limit = 1000
	started := time.Now()
	report := telemetryReport(t, e, query)
	elapsed := time.Since(started)
	if report.Population.Matched < population {
		t.Fatalf("the report did not read the whole population: %+v", report.Population)
	}
	if elapsed > 20*time.Second {
		t.Fatalf("reading %d Runs took %s", population, elapsed)
	}
	t.Logf("telemetry over %d runs: %s", population, elapsed)
}
