package runtime

import (
	"sort"
	"time"
)

// TimingCalculatorRevisionSessions adds the phases of an assisted session and
// the Run's time with no attempt active to revision 3. Every metric revision 3
// published keeps its number; only new names are added.
const TimingCalculatorRevisionSessions = "core-timing/4"

// sessionPhases splits the time an assisted attempt spent with its host. The
// executor's own start is never observed for such an attempt, so executor_time
// stays unavailable and says why; these name what is known instead.
//
// host_pickup runs from the handoff to the host taking the task (session take),
// host_session from the handoff to the host's report, and host_work is the
// host's own share: from taking it to the report, less the waits for answers.
// A host that never ran session take leaves the pickup unknown, and its wait
// for a host stays inside host_work -- said by the reason, not hidden.
func (c timingCalculator) sessionPhases(n *TimingNode, a *Attempt) {
	end, endRef, open := a.Session.Reported, timingRef("attempt", a.ID, "session.reported"), false
	if end == nil {
		end, endRef = c.scopeEnd("attempt", a.ID, a.Settled)
		open = a.Settled == nil
	}
	admitted := timingRef("attempt", a.ID, "admitted")
	c.interval(n, "host_session", timingSpan{&a.Admitted, end, admitted, endRef, open}, true, false)
	wait := c.decisionWait(a)
	n.Metrics["decision_wait"] = wait
	start, startRef := &a.Admitted, admitted
	if taken := a.Session.Taken; taken != nil {
		c.interval(n, "host_pickup", timingSpan{&a.Admitted, taken, admitted, timingRef("attempt", a.ID, "session.taken"), false}, true, false)
		start, startRef = taken, timingRef("attempt", a.ID, "session.taken")
	} else {
		n.Metrics["host_pickup"] = noDuration("unavailable", "host_take_not_recorded", false)
	}
	held, _ := c.measure(timingSpan{start, end, startRef, endRef, open}, true, false)
	work := timingDifference(held, wait)
	if a.Session.Taken == nil {
		addReason(&work, "host_take_not_recorded")
	}
	n.Metrics["host_work"] = work
}

// decisionWait sums the waits for answers to the runtime decisions this attempt
// requested. A record without its request time -- every Run sealed before 41 --
// makes the sum unavailable rather than shorter.
func (c timingCalculator) decisionWait(a *Attempt) Duration {
	var parts []Duration
	for _, record := range c.r.DecisionLedger {
		if record.AttemptID != a.ID {
			continue
		}
		pending := record.Status == "pending"
		if record.Requested == nil {
			parts = append(parts, noDuration("unavailable", "decision_request_time_not_recorded", pending))
			continue
		}
		end, endRef := record.Observed, timingRef("decision", record.DefinitionID, "observed")
		if pending || end == nil {
			end, endRef = &c.asOf, timingRef("report", c.r.ID, "as_of")
		}
		d, _ := c.measure(timingSpan{record.Requested, end, timingRef("decision", record.DefinitionID, "requested"), endRef, pending}, true, false)
		parts = append(parts, d)
	}
	if len(parts) == 0 {
		return measuredDuration(0, false)
	}
	return sumTiming(parts)
}

// timingDifference is the host's own time: the session less the waits for
// answers inside it. Either side unknown leaves it unknown; either side an
// estimate makes it one.
func timingDifference(whole, part Duration) Duration {
	value := func(d Duration) *int64 {
		if d.ValueMS != nil {
			return d.ValueMS
		}
		return d.EstimateMS
	}
	a, b := value(whole), value(part)
	if a == nil || b == nil {
		d := noDuration("unavailable", "host_work_needs_session_and_wait", whole.IsOpen || part.IsOpen)
		return d
	}
	difference := max(*a-*b, 0)
	if whole.Quality == "measured" && part.Quality == "measured" {
		return measuredDuration(difference, whole.IsOpen || part.IsOpen)
	}
	d := Duration{Quality: "estimated", EstimateMS: &difference, IsOpen: whole.IsOpen || part.IsOpen, Reasons: []string{}}
	for _, reason := range append(append([]string{}, whole.Reasons...), part.Reasons...) {
		addReason(&d, reason)
	}
	addReason(&d, "difference_of_intervals")
	return d
}

var sessionPhaseMetrics = []string{"host_pickup", "host_session", "host_work", "decision_wait"}

// rollupSessions sums each session phase over the attempts a node covers.
// These are sums, not unions: attempts of parallel branches overlap.
func (c timingCalculator) rollupSessions(node *TimingNode) {
	metrics := map[string][]Duration{}
	for _, id := range node.attempts {
		a := c.r.Attempts[id]
		if a == nil || a.Session == nil {
			continue
		}
		for _, name := range sessionPhaseMetrics {
			if d, ok := c.attemptMetric(node, id, name); ok {
				metrics[name] = append(metrics[name], d)
			}
		}
	}
	for _, name := range sessionPhaseMetrics {
		if len(metrics[name]) != 0 {
			node.Metrics[name+"_sum"] = sumTiming(metrics[name])
		}
	}
}

// attemptMetric finds the metric an attempt leaf below this node published.
func (c timingCalculator) attemptMetric(node *TimingNode, id, name string) (Duration, bool) {
	if node.Kind == "attempt" && node.ID == id {
		d, ok := node.Metrics[name]
		return d, ok
	}
	for index := range node.Children {
		if d, ok := c.attemptMetric(&node.Children[index], id, name); ok {
			return d, true
		}
	}
	return Duration{}, false
}

// idleTime is the part of the Run's span when no attempt or check was active: before
// the first, between a stop and a continuation, after the last. It reads the
// authority's wall clock across processes, so it is an estimate and says so.
func (c timingCalculator) idleTime(root *TimingNode) Duration {
	parse := func(o *Observation) (time.Time, bool) {
		if o == nil {
			return time.Time{}, false
		}
		at, err := time.Parse(time.RFC3339Nano, o.UTC)
		return at, err == nil
	}
	start, ok := parse(&c.r.Created)
	if !ok {
		return noDuration("unavailable", "run_start_not_observed", false)
	}
	endObservation := c.r.Settled
	open := endObservation == nil
	if open {
		endObservation = &c.asOf
	}
	end, ok := parse(endObservation)
	if !ok || end.Before(start) {
		return noDuration("unavailable", "run_end_not_observed", open)
	}
	type span struct{ from, to time.Time }
	spans := []span{}
	for _, a := range c.r.Attempts {
		if a == nil {
			continue
		}
		from, ok := parse(&a.Admitted)
		if !ok {
			return noDuration("unavailable", "attempt_start_not_observed", open)
		}
		to := end
		if a.Settled != nil {
			if to, ok = parse(a.Settled); !ok {
				return noDuration("unavailable", "attempt_end_not_observed", open)
			}
		}
		spans = append(spans, span{from, to})
	}
	for _, check := range c.r.CheckExecutions {
		if check == nil {
			continue
		}
		from, ok := parse(&check.Admitted)
		if !ok {
			return noDuration("unavailable", "check_start_not_observed", open)
		}
		to := end
		if check.Settled != nil {
			if to, ok = parse(check.Settled); !ok {
				return noDuration("unavailable", "check_end_not_observed", open)
			}
		}
		spans = append(spans, span{from, to})
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].from.Before(spans[j].from) })
	idle, cursor := time.Duration(0), start
	for _, s := range spans {
		if s.from.After(cursor) {
			idle += s.from.Sub(cursor)
		}
		if s.to.After(cursor) {
			cursor = s.to
		}
	}
	if end.After(cursor) {
		idle += end.Sub(cursor)
	}
	millis := idle.Milliseconds()
	d := Duration{Quality: "estimated", EstimateMS: &millis, IsOpen: open, Reasons: []string{"authority_wall_estimate"}}
	if c.r.Created.SuspendBasis != "includes_suspend" {
		addReason(&d, "calendar_suspend_coverage_unqualified")
	}
	return d
}
