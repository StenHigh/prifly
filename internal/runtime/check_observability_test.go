package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// These deterministic observations qualify projection arithmetic, not an OS
// suspend profile or the separate native checker/acceptance integration gate.
func checkTimingFixture() Run {
	r := invocationTimingFixture()
	r.SchemaVersion = CoreContextStateVersion
	r.Attempts["attempt"].Settled = timingPoint(13000)
	r.CheckExecutions = map[string]*CheckExecution{
		"check:output": {
			ID: "check:output", Request: CheckRequest{CheckID: "check:output", RunID: r.ID, InvocationID: "child", ActivationID: "activation", ProducerAttemptID: "attempt", Boundary: "step_output"},
			Status: "completed", Admitted: timingObservation(13000), Dispatch: timingPoint(13100), Started: timingPoint(13200), ExecutorEnd: timingPoint(14200), Settled: timingPoint(14500),
		},
		"check:input": {
			ID: "check:input", Request: CheckRequest{CheckID: "check:input", RunID: r.ID, InvocationID: r.RootInvocationID, Boundary: "workflow_input"},
			Status: "completed", Admitted: timingObservation(100), Dispatch: timingPoint(150), Started: timingPoint(200), ExecutorEnd: timingPoint(500), Settled: timingPoint(700),
		},
	}
	for i := range r.Transitions {
		if r.Transitions[i].Kind == "attempt" && r.Transitions[i].To == "completed" {
			r.Transitions[i].At = timingObservation(13000)
		}
	}
	for id, check := range r.CheckExecutions {
		previous := ""
		for _, boundary := range []struct {
			status   string
			observed Observation
		}{{"pending", check.Admitted}, {"dispatching", *check.Dispatch}, {"running", *check.Started}, {"verifying", *check.ExecutorEnd}, {"completed", *check.Settled}} {
			r.Transitions = append(r.Transitions, StateChange{Kind: "check_execution", ID: id, From: previous, To: boundary.status, At: boundary.observed})
			previous = boundary.status
		}
	}
	sort.SliceStable(r.Transitions, func(i, j int) bool { return r.Transitions[i].At.MonotonicMS < r.Transitions[j].At.MonotonicMS })
	return r
}

func TestCheckTimingScopesAndNoProducerDoubleCounting(t *testing.T) {
	r := checkTimingFixture()
	r.Stops = []Stop{{ID: "stop:child", Scope: "invocation", ScopeID: "child", Kind: "pause", Status: "released", Created: timingObservation(13500), Released: timingPoint(14000)}}
	before, _ := json.Marshal(r)
	report := Timing(r, r.LastObserved, false)
	if report.CalculatorRevision != TimingCalculatorRevisionContext || report.Root.AttemptCount != 1 {
		t.Fatal("checks changed the producer Attempt population or used the old calculator")
	}
	check := timingFind(t, report.Root, "check:output")
	if check.Kind != "check_execution" || check.AttemptCount != 0 || len(check.Children) != 0 || check.Verdict != "" {
		t.Fatal("check was represented as a Step/Attempt or its result became a Step verdict")
	}
	for metric, want := range map[string]int64{"elapsed": 1500, "admission_to_dispatch": 100, "dispatch_to_start": 100, "dispatch_latency": 200, "executor_time": 1000, "post_execution_settlement": 300, "restricted_time": 500} {
		timingMeasured(t, check.Metrics[metric], want, false)
	}
	timingMeasured(t, check.StateTime["running"], 1000, false)
	if check.Intervals[0].FromRef != (TimingBoundaryRef{Kind: "check_execution", ID: check.ID, Field: "admitted"}) {
		t.Fatal("check admission has a fabricated creation field")
	}
	activation := timingFind(t, report.Root, "activation")
	if len(activation.Children) != 2 || activation.Children[0].Kind != "step_instance" || activation.Children[1].ID != check.ID {
		t.Fatal("output check is not a separate child of its owning activation")
	}
	rootInvocation := timingFind(t, report.Root, r.RootInvocationID)
	if rootInvocation.Children[len(rootInvocation.Children)-1].ID != "check:input" {
		t.Fatal("workflow-input check acquired a fabricated activation")
	}
	timingMeasured(t, report.Root.Metrics["executor_sum"], 8000, false)
	timingMeasured(t, report.Root.Metrics["executor_active_union"], 8000, false)
	timingMeasured(t, report.Root.Metrics["check_executor_sum"], 1300, false)
	timingMeasured(t, report.Root.Metrics["check_executor_active_union"], 1300, false)
	timingMeasured(t, timingFind(t, report.Root, "call").Metrics["check_executor_sum"], 1000, false)
	if timingFind(t, report.Root, "sibling").Metrics["check_executor_sum"].Quality != "not_applicable" {
		t.Fatal("a child check leaked into its sibling's timing")
	}
	timingMeasured(t, timingFind(t, report.Root, "check:input").Metrics["restricted_time"], 0, false)
	after, _ := json.Marshal(r)
	if !bytes.Equal(before, after) {
		t.Fatal("timing projection changed authoritative facts")
	}
}

func TestCheckTimingAcceptanceIsSeparateFromProcessSettlement(t *testing.T) {
	for _, state := range []string{"accepted", "pending", "rejected"} {
		t.Run(state, func(t *testing.T) {
			r := checkTimingFixture()
			attempt, step := r.Attempts["attempt"], r.Steps["step"]
			if state != "accepted" {
				attempt.Accepted = nil
			}
			if state == "pending" {
				step.Settled, step.Status = nil, "verifying"
			} else if state == "rejected" {
				step.Status = "failed"
			}
			report := Timing(r, timingObservation(17000), false)
			node := timingFind(t, report.Root, attempt.ID)
			timingMeasured(t, node.Metrics["elapsed"], 8000, false)
			timingMeasured(t, node.Metrics["post_execution_settlement"], 0, false)
			duration := node.Metrics["result_to_acceptance"]
			switch state {
			case "accepted":
				timingMeasured(t, duration, 3000, false)
			case "pending":
				if duration.Quality != "partial" || duration.ValueMS != nil || duration.KnownMS == nil || *duration.KnownMS != 5000 || !duration.IsOpen || !slices.Contains(duration.Reasons, "acceptance_pending") {
					t.Fatalf("pending acceptance became a completed sample: %+v", duration)
				}
			case "rejected":
				if duration.Quality != "not_applicable" || duration.ValueMS != nil || !slices.Contains(duration.Reasons, "candidate_not_accepted") {
					t.Fatalf("failed check was reported as result acceptance: %+v", duration)
				}
			}
		})
	}
}

func TestCheckTimingUnknownAndKnownNoSpawnAreDifferent(t *testing.T) {
	t.Run("unknown_after_start", func(t *testing.T) {
		r := checkTimingFixture()
		check := r.CheckExecutions["check:output"]
		check.Status, check.ExecutorEnd, check.Settled = "uncertain", nil, nil
		r.Transitions = slices.DeleteFunc(r.Transitions, func(change StateChange) bool {
			return change.Kind == "check_execution" && change.ID == check.ID && change.At.MonotonicMS > check.Started.MonotonicMS
		})
		recovered := timingObservation(20000)
		recovered.Session, recovered.UTCTrust = "after-restart", "local_wall_unqualified"
		r.Gaps = []TimingGap{{From: timingObservation(14000), To: recovered, Reason: "check_ownership_lost"}}
		r.Transitions = append(r.Transitions, StateChange{Kind: "check_execution", ID: check.ID, From: "running", To: "uncertain", At: recovered})
		for _, live := range []bool{false, true} {
			node := timingFind(t, Timing(r, recovered, live).Root, check.ID)
			duration := node.Metrics["executor_time"]
			if duration.Quality != "partial" || duration.ValueMS != nil || duration.KnownMS == nil || *duration.KnownMS != 800 || !duration.IsOpen || !slices.Contains(duration.Reasons, "executor_continuity_not_confirmed") {
				t.Fatalf("recovery invented continued checker execution: %+v", duration)
			}
		}
	})
	t.Run("status_is_not_liveness", func(t *testing.T) {
		r := checkTimingFixture()
		check := r.CheckExecutions["check:output"]
		check.Status, check.ExecutorEnd, check.Settled = "uncertain", nil, nil
		r.Transitions = []StateChange{{Kind: "check_execution", ID: check.ID, From: "running", To: "uncertain", At: timingObservation(17000)}}
		duration := timingFind(t, Timing(r, timingObservation(20000), true).Root, check.ID).Metrics["executor_time"]
		if duration.ValueMS != nil || duration.KnownMS != nil || duration.Quality != "unavailable" {
			t.Fatalf("an uncertainty transition extended checker liveness: %+v", duration)
		}
	})
	t.Run("known_not_started", func(t *testing.T) {
		r := checkTimingFixture()
		check := r.CheckExecutions["check:output"]
		check.Status, check.Dispatch, check.Started, check.ExecutorEnd = "failed", nil, nil, nil
		check.ProcessOutcome = &local.ProcessOutcome{Started: false}
		node := timingFind(t, Timing(r, r.LastObserved, false).Root, check.ID)
		if duration := node.Metrics["executor_time"]; duration.Quality != "not_applicable" || duration.ValueMS != nil || duration.IsOpen {
			t.Fatalf("no-spawn proof became a zero-time Attempt: %+v", duration)
		}
		timingMeasured(t, Timing(r, r.LastObserved, false).Root.Metrics["check_executor_sum"], 300, false)
	})
}

func TestCheckTimingClockQualityAndBounds(t *testing.T) {
	for _, reason := range []string{"invalid_monotonic_order", "duration_overflow", "incomparable_clock_domains"} {
		t.Run(reason, func(t *testing.T) {
			r := checkTimingFixture()
			check := r.CheckExecutions["check:output"]
			switch reason {
			case "invalid_monotonic_order":
				check.ExecutorEnd.MonotonicMS = check.Started.MonotonicMS - 1
			case "duration_overflow":
				check.ExecutorEnd.MonotonicMS = check.Started.MonotonicMS + maxDurationMS + 1
			case "incomparable_clock_domains":
				check.Started.SuspendBasis, check.ExecutorEnd.SuspendBasis = "unknown", "unknown"
			}
			duration := timingFind(t, Timing(r, r.LastObserved, false).Root, check.ID).Metrics["executor_time"]
			if duration.Quality != "unavailable" || duration.ValueMS != nil || duration.KnownMS != nil || !slices.Contains(duration.Reasons, reason) {
				t.Fatalf("invalid clock became a numeric measurement: %+v", duration)
			}
		})
	}
	t.Run("settlement_pending", func(t *testing.T) {
		r := checkTimingFixture()
		check := r.CheckExecutions["check:output"]
		check.Status, check.Settled = "verifying", nil
		duration := timingFind(t, Timing(r, timingObservation(15000), false).Root, check.ID).Metrics["post_execution_settlement"]
		if duration.Quality != "partial" || duration.ValueMS != nil || duration.KnownMS == nil || *duration.KnownMS != 800 || !duration.IsOpen || !slices.Contains(duration.Reasons, "check_settlement_pending") {
			t.Fatalf("pending settlement became a completed latency sample: %+v", duration)
		}
	})
}

func TestCheckTimingLegacyVersionsIgnoreNewFields(t *testing.T) {
	for _, version := range []string{StateVersion, CoreStateVersion, CoreInvocationStateVersion, CoreRepeatStateVersion} {
		t.Run(version, func(t *testing.T) {
			r := invocationTimingFixture()
			r.SchemaVersion = version
			before, _ := json.Marshal(Timing(r, r.LastObserved, false))
			r.CheckExecutions = checkTimingFixture().CheckExecutions
			after, _ := json.Marshal(Timing(r, r.LastObserved, false))
			if !bytes.Equal(before, after) {
				t.Fatal("new check fields changed a legacy timing projection")
			}
		})
	}
}

func checkTelemetryFixture(t *testing.T) (*Engine, Run, int64) {
	t.Helper()
	e, runID := checkExecutionFixture(t, "pass", 5000)
	r, view, err := e.load(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	var checkRef flow.Ref
	for _, definition := range r.Definitions {
		if definition.Kind == "check" {
			checkRef = definition.Ref
		}
	}
	r.Created, r.LastObserved = timingObservation(0), timingObservation(10000)
	r.Invocations[r.RootInvocationID].Created = r.Created
	for _, activation := range r.Activations {
		activation.Created = r.Created
	}
	r.Transitions = nil
	r.CheckExecutions = map[string]*CheckExecution{}
	plan, err := r.plan()
	if err != nil {
		t.Fatal(err)
	}
	for index, result := range []string{"pass", "fail", "inconclusive", "failed", "uncertain", "cancelled"} {
		id := "check:" + result
		requestDigest := rawDigest([]byte(id))
		check := &CheckExecution{ID: id, Request: CheckRequest{CheckID: id, RunID: r.ID, InvocationID: r.RootInvocationID, WorkflowRef: r.WorkflowRef, PolicyRef: plan.Workflow.PolicyRef, PackageLockDigest: r.LockRef.Digest, CheckRef: checkRef, AdmissionID: "admission:" + result, Boundary: "workflow_input", Port: "value"}, RequestBytes: local.BlobRef{Digest: requestDigest, Size: int64(len(id))}, Status: "completed", Admitted: timingObservation(int64(index * 1000)), Dispatch: timingPoint(int64(index*1000 + 10)), Started: timingPoint(int64(index*1000 + 20)), ExecutorEnd: timingPoint(int64(index*1000 + 120)), Settled: timingPoint(int64(index*1000 + 150))}
		check.Report = &CheckResult{SchemaVersion: CheckResultVersion, CheckID: id, RunID: r.ID, RequestDigest: requestDigest, Status: result, Summary: "PRIVATE-CHECK-SUMMARY", Limitations: []string{"PRIVATE-LIMITATION"}}
		if result == "failed" || result == "uncertain" || result == "cancelled" {
			check.Report, check.Status, check.Failure = nil, result, "check_failure_fixture"
		}
		if result == "uncertain" {
			check.ExecutorEnd, check.Settled = nil, nil
		}
		r.CheckExecutions[id] = check
		r.Transitions = append(r.Transitions, StateChange{Kind: "check_execution", ID: id, To: "pending", At: check.Admitted})
		if check.Settled != nil {
			r.Transitions = append(r.Transitions, StateChange{Kind: "check_execution", ID: id, From: "pending", To: check.Status, At: *check.Settled})
		} else {
			r.Transitions = append(r.Transitions, StateChange{Kind: "check_execution", ID: id, From: "pending", To: "uncertain", At: timingObservation(4500)})
		}
	}
	return e, r, view.Snapshot.Version
}

func TestCheckTelemetryCountsOutcomesAndFixedCut(t *testing.T) {
	e, r, version := checkTelemetryFixture(t)
	cut := telemetrySaveRun(t, e, r, version)
	query := telemetryQuery("records", "check.admitted", "check.settled", "check.failed", "check.cancelled", "check.uncertain", "check.reports", "timing.executor_time")
	query.RunIDs, query.Cut, query.Limit, query.Filters.Scope = []string{r.ID}, &cut, 1000, []string{"check_execution"}
	report := telemetryReport(t, e, query)
	if report.CalculatorRevision != TelemetryCalculatorRevisionContext || report.TimingRevision != TimingCalculatorRevisionContext || report.Population.Attempts != 0 || report.Population.Steps != 0 {
		t.Fatal("checker facts became producer populations or legacy timing")
	}
	counts, reports := map[string]int{}, map[string]int{}
	for _, record := range report.Records {
		if record.Subject.Kind != "check_execution" || record.Subject.StepInstanceID != "" || record.Subject.AttemptID != "" || record.Dimensions["step_id"] != "" || record.Dimensions["verdict"] != "unknown" {
			t.Fatal("checker facts acquired producer or verdict identity")
		}
		counts[record.Metric]++
		if record.Metric == "check.reports" {
			reports[record.Dimensions["check_result"]]++
		}
		if record.Metric == "timing.executor_time" && record.Subject.ID == "check:uncertain" && (record.Quality == "measured" || record.Value != nil) {
			t.Fatal("uncertain execution became a complete duration")
		}
	}
	if counts["check.admitted"] != 6 || counts["check.settled"] != 5 || counts["check.failed"] != 1 || counts["check.cancelled"] != 1 || counts["check.uncertain"] != 1 || counts["check.reports"] != 3 || reports["pass"] != 1 || reports["fail"] != 1 || reports["inconclusive"] != 1 {
		t.Fatalf("check outcomes were collapsed or lost: counts=%v reports=%v", counts, reports)
	}
	before, _ := json.Marshal(report)
	if bytes.Contains(before, []byte("PRIVATE-CHECK")) || bytes.Contains(before, []byte("PRIVATE-LIMITATION")) {
		t.Fatal("telemetry leaked report text")
	}
	// A later observation cannot extend the old cut's checker timing or change
	// which outcomes were known. This fixture is saved journal data, not a
	// claim that the native runner performed these deterministic clock events.
	r.LastObserved = timingObservation(12000)
	telemetrySaveRun(t, e, r, version+1)
	after, _ := json.Marshal(telemetryReport(t, e, query))
	if !bytes.Equal(before, after) {
		t.Fatal("later facts changed an earlier checker report")
	}
	query.Mode, query.GroupBy, query.Metrics = "aggregate", []string{"check_result"}, []string{"check.reports"}
	groups := telemetryReport(t, e, query).Aggregates
	if len(groups) != 3 {
		t.Fatal("independent report outcomes cannot be grouped")
	}
	for _, group := range groups {
		if group.Total == nil || *group.Total != 1 || group.Coverage.Observed != 1 || group.Coverage.Expected == nil || *group.Coverage.Expected != 1 {
			t.Fatal("check occurrence counts lost exact coverage")
		}
	}
}

func TestCheckTelemetryRejectsForeignOwners(t *testing.T) {
	_, r, _ := checkTelemetryFixture(t)
	plan, err := r.plan()
	if err != nil {
		t.Fatal(err)
	}
	plans, err := telemetryPlanIndex(r, plan)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []string{"invocation", "activation", "producer", "definition", "report"} {
		t.Run(change, func(t *testing.T) {
			check := r.CheckExecutions["check:pass"]
			copy := *check
			report := *check.Report
			check.Report = &report
			defer func() { *check = copy }()
			switch change {
			case "invocation":
				check.Request.InvocationID = "invocation:foreign"
			case "activation":
				check.Request.ActivationID = "activation:foreign"
			case "producer":
				check.Request.ProducerAttemptID = "attempt:foreign"
			case "definition":
				check.Request.CheckRef.Digest = "sha256:" + strings.Repeat("0", 64)
			case "report":
				check.Report.CheckID = "check:foreign"
			}
			if _, err := telemetryCheckLabels(r, plans, check); !errors.Is(err, local.ErrIntegrity) {
				t.Fatal("foreign check ownership was silently projected", err)
			}
		})
	}
}

func TestCheckTelemetryCohortVersionAndLegacyCut(t *testing.T) {
	e, options := contextDriverProject(t, nil)
	snapshots, oldCut, err := e.Store.ReadAllAt(context.Background(), -1, 10)
	if err != nil || len(snapshots) != 1 {
		t.Fatal("missing preserved legacy Run", err)
	}
	legacyID := snapshots[0].RunID
	query := telemetryQuery("catalog")
	query.Cut = &oldCut
	old := telemetryReport(t, e, query)
	if old.CalculatorRevision != TelemetryCalculatorRevision || old.TimingRevision != "foundation-timing/1" {
		t.Fatal("legacy cohort used new calculator revisions")
	}
	before, _ := json.Marshal(old)
	started, err := e.Start(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(telemetryReport(t, e, query))
	if !bytes.Equal(before, after) {
		t.Fatal("starting a context Run changed an earlier catalog cut")
	}
	query.Cut, query.RunIDs = nil, []string{legacyID}
	if report := telemetryReport(t, e, query); report.CalculatorRevision != TelemetryCalculatorRevision || report.TimingRevision != "foundation-timing/1" {
		t.Fatal("an unselected context Run changed legacy cohort versions")
	}
	query.Mode, query.GroupBy, query.Metrics = "aggregate", []string{"check_result"}, []string{"timing.elapsed"}
	var problem *flow.Problem
	if _, err := e.Telemetry(context.Background(), query); !errors.As(err, &problem) || problem.Code != "unknown_dimension" {
		t.Fatal("a legacy-only cohort advertised a check-only grouping dimension", err)
	}
	query = telemetryQuery("aggregate", "timing.elapsed")
	query.Filters.Scope, query.RunIDs = []string{"run"}, []string{legacyID, started.Receipt.RunID}
	mixed := telemetryReport(t, e, query)
	if mixed.CalculatorRevision != TelemetryCalculatorRevisionContext || mixed.TimingRevision != TimingCalculatorRevisionContext || len(mixed.Aggregates) != 2 {
		t.Fatal("mixed cohort merged distinct calculator methods or lost new versions")
	}
	revisions := map[string]bool{}
	for _, group := range mixed.Aggregates {
		revisions[group.DescriptorID] = true
	}
	if !revisions["core:timing.elapsed/1"] || !revisions["core:timing.elapsed/3"] {
		t.Fatal("legacy and context timing descriptors were merged", revisions)
	}
}

func TestCheckTelemetryValidatesEveryRunBeforePlanCache(t *testing.T) {
	e, options := contextDriverProject(t, nil)
	ids := []string{}
	for i := 0; i < 2; i++ {
		executor := e.Config.Configuration.Executors["test:step/context"]
		executor.TimeoutMS += int64(i * 1000)
		e.Config.Configuration.Executors["test:step/context"] = executor
		options.CommandID = newID("command")
		started, err := e.Start(context.Background(), options)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, started.Receipt.RunID)
	}
	sort.Strings(ids) // ReadAllAt returns Runs in this order; poison the cached-plan case.
	first, _, err := e.load(context.Background(), ids[0])
	if err != nil {
		t.Fatal(err)
	}
	second, view, err := e.load(context.Background(), ids[1])
	if err != nil || first.WorkflowRef != second.WorkflowRef {
		t.Fatal("fixture did not share an exact compiled workflow", err)
	}
	firstConfig, _ := json.Marshal(first.Executors)
	secondConfig, _ := json.Marshal(second.Executors)
	if bytes.Equal(firstConfig, secondConfig) {
		t.Fatal("fixture did not pin independent executor configurations")
	}
	query := telemetryQuery("catalog", "timing.elapsed")
	query.RunIDs = ids
	telemetryReport(t, e, query)
	for key, executor := range second.Executors {
		executor.Config.TimeoutMS++
		second.Executors[key] = executor
		break
	}
	telemetrySaveRun(t, e, second, view.Snapshot.Version)
	if _, err := e.Telemetry(context.Background(), query); publicationErrorCode(err) != "context_pin_drift" {
		t.Fatal("cached plan bypassed another Run's pinned configuration", err)
	}
	query.RunIDs = ids[:1]
	telemetryReport(t, e, query)
}

func TestCheckTelemetryPendingAcceptanceDoesNotFailFirstAttempt(t *testing.T) {
	e, options := contextDriverProject(t, nil)
	started, err := e.Start(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	r, view, err := e.load(context.Background(), started.Receipt.RunID)
	if err != nil || len(r.Steps) != 1 {
		t.Fatal("missing real step activation", err)
	}
	var step *Step
	for _, created := range r.Steps {
		step = created
	}
	r.Created, r.LastObserved, r.Transitions = timingObservation(0), timingObservation(100), nil
	r.Invocations[r.RootInvocationID].Created = r.Created
	activation := r.Activations[step.ActivationID]
	activation.Created, step.Created = r.Created, r.Created
	attempt := &Attempt{ID: "attempt:acceptance-observation", StepID: step.ID, ActivationID: step.ActivationID, Status: "completed", Admitted: timingObservation(10), Dispatch: timingPoint(10), Started: timingPoint(20), CandidateAt: timingPoint(40), ExecutorEnd: timingPoint(50), Settled: timingPoint(60), ProcessOutcome: &local.ProcessOutcome{Started: true, WaitReturned: true, GroupEmpty: true, ExitCode: telemetryPtr(0)}}
	r.Attempts[attempt.ID], step.AttemptIDs = attempt, []string{attempt.ID}
	query := telemetryQuery("aggregate", "core.first_attempt_pass_fraction")
	query.RunIDs = []string{r.ID}
	version := view.Snapshot.Version
	// Seed only the observed acceptance boundaries. Native worker/checker tests
	// establish how these facts are produced; this test qualifies their query.
	for _, state := range []string{"pending", "rejected", "accepted"} {
		t.Run(state, func(t *testing.T) {
			step.Status, activation.Status = "verifying", "verifying"
			step.Settled, activation.Settled, attempt.Accepted = nil, nil, nil
			if state != "pending" {
				step.Status, activation.Status = "failed", "failed"
				step.Settled, activation.Settled = timingPoint(90), timingPoint(90)
			}
			if state == "accepted" {
				step.Status, activation.Status, step.Verdict = "completed", "completed", "pass"
				attempt.Accepted = &Result{RunID: r.ID, StepInstanceID: step.ID, AttemptID: attempt.ID, Verdict: "pass"}
			}
			cut := telemetrySaveRun(t, e, r, version)
			version++
			query.Cut = &cut
			report := telemetryReport(t, e, query)
			ratio := report.Population.Ratios["core.first_attempt_pass_fraction"]
			wantNumerator, wantDenominator := int64(0), int64(1)
			if state == "pending" {
				wantDenominator = 0
			} else if state == "accepted" {
				wantNumerator = 1
			}
			if report.Population.SettledAttempts != 1 || ratio.Numerator != wantNumerator || ratio.Denominator != wantDenominator || (state == "pending") != (ratio.Value == nil) {
				t.Fatalf("process settlement was confused with result acceptance: %+v", report.Population)
			}
			latencies := telemetryQuery("records", "timing.result_to_acceptance")
			latencies.RunIDs, latencies.Cut, latencies.Filters.Scope = []string{r.ID}, &cut, []string{"attempt"}
			if state == "accepted" {
				latencies.EventFrom, latencies.EventBefore = timingObservation(85).UTC, timingObservation(95).UTC
			}
			observed := telemetryReport(t, e, latencies).Records
			if len(observed) != 1 {
				t.Fatal("acceptance metric was timestamped at earlier process settlement")
			}
			wantObserved := step.Settled
			if wantObserved == nil {
				wantObserved = &r.LastObserved
			}
			if observed[0].Observed == nil || *observed[0].Observed != *wantObserved {
				t.Fatal("acceptance observation does not use its own boundary")
			}
		})
	}
}
