package runtime

import "testing"

func phasesFixture() Run {
	r := checkTimingFixture()
	r.SchemaVersion = CoreQuestionStateVersion
	a := r.Attempts["attempt"]
	// Admitted at 5000 and settled at 13000: taken a second after the
	// handoff, a question asked at 8000 and answered at 11000, reported at
	// 12000.
	a.Started, a.ExecutorEnd = nil, nil
	a.Session = &SessionHandoff{HostState: SessionReported, Handed: timingObservation(5000), Taken: timingPoint(6000), Reported: timingPoint(12000)}
	r.DecisionLedger = []DecisionRecord{{DefinitionID: "improve_apply", AttemptID: "attempt", Status: "answered", Source: "actor", Requested: timingPoint(8000), Observed: timingPoint(11000)}}
	return r
}

func phaseValue(t *testing.T, node TimingNode, metric string) int64 {
	t.Helper()
	d, ok := node.Metrics[metric]
	if !ok || d.ValueMS == nil {
		t.Fatalf("%s of %s is not measured: %+v", metric, node.ID, d)
	}
	return *d.ValueMS
}

// Seven seconds with the host were one number the timing tree could not give:
// the attempt's executor start is never observed. Its phases are known from what
// the Run holds -- when it was handed, taken, asked, answered and reported.
func TestAnAssistedAttemptNamesWhereItsTimeWent(t *testing.T) {
	report := Timing(phasesFixture(), timingObservation(22000), false)
	if report.CalculatorRevision != TimingCalculatorRevisionSessions {
		t.Fatalf("calculator %s", report.CalculatorRevision)
	}
	attempt := timingFind(t, report.Root, "attempt")
	for metric, want := range map[string]int64{"host_pickup": 1000, "host_session": 7000, "decision_wait": 3000, "host_work": 3000} {
		if got := phaseValue(t, attempt, metric); got != want {
			t.Fatalf("%s = %d, want %d", metric, got, want)
		}
	}
	if attempt.Metrics["executor_time"].Quality != "unavailable" {
		t.Fatalf("a published metric changed meaning: %+v", attempt.Metrics["executor_time"])
	}
	for metric, want := range map[string]int64{"host_work_sum": 3000, "decision_wait_sum": 3000, "host_pickup_sum": 1000} {
		if got := phaseValue(t, report.Root, metric); got != want {
			t.Fatalf("root %s = %d, want %d", metric, got, want)
		}
	}
	idle := report.Root.Metrics["idle"]
	// The Run spans 0..20000; its attempt runs 5000..13000 and its checks
	// 100..700 and 13000..14500, so 9900 of it had nothing running.
	if idle.Quality != "estimated" || idle.EstimateMS == nil || *idle.EstimateMS != 9900 {
		t.Fatalf("idle = %+v, want an estimate of 9900", idle)
	}
}

// A host that never ran session take, and a Run sealed before the request time
// was kept, leave what they did not record unknown -- never zero, never a
// shorter number that reads as progress.
func TestUnrecordedPhasesStayUnknown(t *testing.T) {
	r := phasesFixture()
	r.Attempts["attempt"].Session.Taken = nil
	attempt := timingFind(t, Timing(r, timingObservation(22000), false).Root, "attempt")
	if pickup := attempt.Metrics["host_pickup"]; pickup.Quality != "unavailable" || pickup.Reasons[0] != "host_take_not_recorded" {
		t.Fatalf("an unrecorded take was given a value: %+v", pickup)
	}
	work := attempt.Metrics["host_work"]
	if work.ValueMS == nil || *work.ValueMS != 4000 || !containsReason(work, "host_take_not_recorded") {
		t.Fatalf("the host's time does not say it includes the wait for a host: %+v", work)
	}

	r = phasesFixture()
	r.DecisionLedger[0].Requested = nil
	attempt = timingFind(t, Timing(r, timingObservation(22000), false).Root, "attempt")
	if wait := attempt.Metrics["decision_wait"]; wait.Quality != "unavailable" || !containsReason(wait, "decision_request_time_not_recorded") {
		t.Fatalf("an unrecorded request time was given a value: %+v", wait)
	}
	if work := attempt.Metrics["host_work"]; work.Quality != "unavailable" {
		t.Fatalf("the host's time was taken as the whole session: %+v", work)
	}
}

func containsReason(d Duration, reason string) bool {
	for _, r := range d.Reasons {
		if r == reason {
			return true
		}
	}
	return false
}
