package runtime

import (
	"sort"
	"strings"
	"time"
)

const maxDurationMS int64 = 1<<53 - 1

const TimingCalculatorRevisionCore = "core-timing/1"
const TimingCalculatorRevisionContext = "core-timing/2"

// Duration follows OBS-002. Nil is unknown/inapplicable, never a measured zero.
// The caller must publish TimingTree with the same RunView journal cut used to
// read Run. Timing is pure: it does not refresh a worker or write clock ticks.
type Duration struct {
	Quality    string   `json:"quality"`
	ValueMS    *int64   `json:"value_ms"`
	KnownMS    *int64   `json:"known_ms"`
	EstimateMS *int64   `json:"estimate_ms"`
	IsOpen     bool     `json:"is_open"`
	Reasons    []string `json:"reasons"`
}

type TimingBoundaryRef struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Field string `json:"field"`
}

type TimingInterval struct {
	Metric       string            `json:"metric"`
	State        string            `json:"state,omitempty"`
	From         *Observation      `json:"from"`
	To           *Observation      `json:"to"`
	FromRef      TimingBoundaryRef `json:"from_ref"`
	ToRef        TimingBoundaryRef `json:"to_ref"`
	Method       string            `json:"method"`
	SuspendBasis string            `json:"suspend_basis"`
	Duration     Duration          `json:"duration"`
}

type TimingNode struct {
	Kind         string              `json:"kind"`
	ID           string              `json:"id"`
	Status       string              `json:"status"`
	Verdict      string              `json:"verdict,omitempty"`
	Outcome      *string             `json:"outcome,omitempty"`
	StageID      string              `json:"stage_id,omitempty"`
	StageKind    string              `json:"stage_kind,omitempty"`
	AttemptCount int                 `json:"attempt_count"`
	Metrics      map[string]Duration `json:"metrics"`
	StateTime    map[string]Duration `json:"state_time"`
	Intervals    []TimingInterval    `json:"intervals"`
	Children     []TimingNode        `json:"children"`
	Reasons      []string            `json:"reasons"`
	// Leaf identities, never parent rollups, feed aggregate executor metrics.
	attempts []string
	checks   []string
}

type TimingTree struct {
	SchemaVersion      string      `json:"schema_version"`
	CalculatorRevision string      `json:"calculator_revision"`
	RunID              string      `json:"run_id"`
	AsOf               Observation `json:"as_of"`
	DriverLive         bool        `json:"driver_live"`
	Root               TimingNode  `json:"root"`
}

type timingSpan struct {
	from, to       *Observation
	fromRef, toRef TimingBoundaryRef
	open           bool
}

type clockSegment struct{ from, to Observation }

type timingCalculator struct {
	r          Run
	asOf       Observation
	driverLive bool
}

func measuredDuration(value int64, open bool) Duration {
	return Duration{Quality: "measured", ValueMS: &value, KnownMS: &value, IsOpen: open, Reasons: []string{}}
}

func noDuration(quality, reason string, open bool) Duration {
	return Duration{Quality: quality, IsOpen: open, Reasons: []string{reason}}
}

func addReason(d *Duration, reason string) {
	for _, existing := range d.Reasons {
		if existing == reason {
			return
		}
	}
	d.Reasons = append(d.Reasons, reason)
}

func clockComparable(a, b Observation) bool {
	knownBasis := a.SuspendBasis == "includes_suspend" || a.SuspendBasis == "excludes_suspend" || a.SuspendBasis == "excludes_suspend_on_darwin"
	return a.Session != "" && a.Session == b.Session && a.Source != "" && a.Source == b.Source && knownBasis && a.SuspendBasis == b.SuspendBasis
}

func clockDelta(a, b Observation) (int64, string) {
	if !clockComparable(a, b) {
		return 0, "incomparable_clock_domains"
	}
	if a.MonotonicMS < 0 || b.MonotonicMS < a.MonotonicMS {
		return 0, "invalid_monotonic_order"
	}
	delta := b.MonotonicMS - a.MonotonicMS
	if delta > maxDurationMS {
		return 0, "duration_overflow"
	}
	return delta, ""
}

func utcEstimate(a, b Observation) *int64 {
	// A locally observed wall timestamp is not a claim that UTC stayed correct
	// across a crash or suspend. Qualification is explicit in the observation.
	if a.UTCTrust != "trusted" || b.UTCTrust != "trusted" {
		return nil
	}
	start, err1 := time.Parse(time.RFC3339Nano, a.UTC)
	end, err2 := time.Parse(time.RFC3339Nano, b.UTC)
	if err1 != nil || err2 != nil || end.Before(start) {
		return nil
	}
	// Avoid time.Duration's approximately 292-year saturation on timestamp gaps.
	seconds := end.Unix() - start.Unix()
	nanoseconds := end.Nanosecond() - start.Nanosecond()
	if nanoseconds < 0 {
		seconds--
		nanoseconds += 1e9
	}
	if seconds > maxDurationMS/1000 {
		return nil
	}
	millis := seconds*1000 + int64(nanoseconds/1e6)
	if millis < 0 || millis > maxDurationMS {
		return nil
	}
	return &millis
}

func utcBackwards(a, b Observation) bool {
	start, err1 := time.Parse(time.RFC3339Nano, a.UTC)
	end, err2 := time.Parse(time.RFC3339Nano, b.UTC)
	return err1 == nil && err2 == nil && end.Before(start)
}

// measure returns measured clock segments separately from their interpretation.
// Calendar elapsed cannot be fully measured by a suspend-excluding clock. The
// executor's declared occupancy method can use that clock, but never claims CPU
// time or physical wall-clock lifetime. Intervals crossing sessions retain gaps.
func (c timingCalculator) measure(span timingSpan, calendar, executor bool, anchors ...Observation) (Duration, []clockSegment) {
	if span.from == nil {
		return noDuration("unavailable", "start_boundary_not_observed", false), nil
	}
	if span.to == nil {
		return noDuration("unavailable", "end_boundary_not_observed", span.open), nil
	}
	start, end := *span.from, *span.to
	var segments []clockSegment
	d := measuredDuration(0, span.open)
	known := int64(0)
	appendSegment := func(a, b Observation) bool {
		value, reason := clockDelta(a, b)
		if reason != "" {
			addReason(&d, reason)
			return false
		}
		if value > maxDurationMS-known {
			addReason(&d, "duration_overflow")
			return false
		}
		known += value
		segments = append(segments, clockSegment{a, b})
		return true
	}
	if executor && span.open && !c.driverLive {
		// Only recorded evidence extends the known prefix. Persisted 'running'
		// and a later query clock do not establish that a worker remained alive.
		last := start
		found := false
		for _, a := range anchors {
			if clockComparable(start, a) && a.MonotonicMS >= last.MonotonicMS {
				last, found = a, true
			}
		}
		for _, gap := range c.r.Gaps {
			if clockComparable(start, gap.From) && gap.From.MonotonicMS >= last.MonotonicMS {
				last, found = gap.From, true
			}
		}
		if found {
			appendSegment(start, last)
		}
		addReason(&d, "executor_continuity_not_confirmed")
	} else {
		cursor := start
		crossedGap := false
		for _, gap := range c.r.Gaps {
			if !clockComparable(cursor, gap.From) || gap.From.MonotonicMS < cursor.MonotonicMS {
				continue
			}
			if clockComparable(end, gap.From) && gap.From.MonotonicMS >= end.MonotonicMS {
				continue
			}
			appendSegment(cursor, gap.From)
			crossedGap = true
			addReason(&d, "recovery_gap")
			cursor = gap.To
			if executor {
				// Reattachment evidence is not present in the foundation model.
				// A recovered end timestamp does not prove execution throughout the gap.
				break
			}
		}
		if !executor || !crossedGap {
			if !appendSegment(cursor, end) {
				if !executor && !crossedGap {
					// Journal-ordered consecutive observations in one domain form
					// disjoint known pieces. Never sum per-domain min/max ranges:
					// concurrent control clients could make those ranges overlap.
					previous := start
					for _, point := range append(append([]Observation{}, anchors...), end) {
						if clockComparable(previous, point) {
							appendSegment(previous, point)
						}
						previous = point
					}
				} else {
					last := cursor
					found := false
					for _, a := range anchors {
						if clockComparable(cursor, a) && a.MonotonicMS >= last.MonotonicMS {
							last, found = a, true
						}
					}
					if found {
						appendSegment(cursor, last)
					}
				}
			}
		}
	}
	if calendar && start.SuspendBasis != "includes_suspend" {
		addReason(&d, "calendar_suspend_coverage_unqualified")
	}
	if calendar && utcBackwards(start, end) {
		addReason(&d, "utc_rollback")
	}
	for _, segment := range segments {
		if segment.from.Source != start.Source || segment.from.SuspendBasis != start.SuspendBasis {
			return noDuration("unavailable", "incompatible_measurement_basis", span.open), nil
		}
		if calendar && utcBackwards(segment.from, segment.to) {
			addReason(&d, "utc_rollback")
		}
	}
	for _, reason := range d.Reasons {
		if reason == "duration_overflow" || reason == "invalid_monotonic_order" {
			return noDuration("unavailable", reason, span.open), nil
		}
	}
	if len(d.Reasons) == 0 {
		return measuredDuration(known, span.open), segments
	}
	d.ValueMS, d.KnownMS = nil, nil
	if len(segments) > 0 {
		d.Quality, d.KnownMS = "partial", &known
	} else {
		d.Quality = "unavailable"
	}
	if calendar && !executor {
		d.EstimateMS = utcEstimate(start, end)
		if d.EstimateMS != nil && len(segments) == 0 {
			d.Quality = "estimated"
		}
	}
	return d, segments
}

func sumTiming(durations []Duration) Duration {
	known, fullEstimate := int64(0), int64(0)
	knownCount, applicable := 0, 0
	complete, hasEstimate := true, true
	out := measuredDuration(0, false)
	for _, d := range durations {
		if d.Quality == "not_applicable" {
			continue
		}
		applicable++
		out.IsOpen = out.IsOpen || d.IsOpen
		complete = complete && d.Quality == "measured"
		for _, reason := range d.Reasons {
			addReason(&out, reason)
		}
		if d.KnownMS != nil {
			if *d.KnownMS > maxDurationMS-known {
				return noDuration("unavailable", "duration_overflow", out.IsOpen)
			}
			known += *d.KnownMS
			knownCount++
		}
		value := d.ValueMS
		if value == nil {
			value = d.EstimateMS
		}
		if value == nil {
			hasEstimate = false
		} else if *value > maxDurationMS-fullEstimate {
			return noDuration("unavailable", "duration_overflow", out.IsOpen)
		} else {
			fullEstimate += *value
		}
	}
	if applicable == 0 {
		return noDuration("not_applicable", "no_applicable_intervals", false)
	}
	if complete {
		return measuredDuration(known, out.IsOpen)
	}
	out.ValueMS, out.KnownMS = nil, nil
	out.Quality = "unavailable"
	if knownCount > 0 {
		out.Quality, out.KnownMS = "partial", &known
	}
	if hasEstimate {
		out.EstimateMS = &fullEstimate
		if knownCount == 0 {
			out.Quality = "estimated"
		}
	}
	if len(out.Reasons) == 0 {
		addReason(&out, "required_interval_unavailable")
	}
	return out
}

func unionTiming(durations []Duration, segments []clockSegment) Duration {
	combined := sumTiming(durations)
	if len(segments) == 0 {
		return combined
	}
	for _, segment := range segments[1:] {
		if !clockComparable(segments[0].from, segment.from) {
			return noDuration("unavailable", "union_requires_comparable_clock_domains", combined.IsOpen)
		}
	}
	sort.Slice(segments, func(i, j int) bool { return segments[i].from.MonotonicMS < segments[j].from.MonotonicMS })
	start, end := segments[0].from.MonotonicMS, segments[0].to.MonotonicMS
	value := int64(0)
	for _, segment := range segments[1:] {
		if segment.from.MonotonicMS <= end {
			end = max(end, segment.to.MonotonicMS)
		} else {
			if end-start > maxDurationMS-value {
				return noDuration("unavailable", "duration_overflow", combined.IsOpen)
			}
			value += end - start
			start, end = segment.from.MonotonicMS, segment.to.MonotonicMS
		}
	}
	if end-start > maxDurationMS-value {
		return noDuration("unavailable", "duration_overflow", combined.IsOpen)
	}
	value += end - start
	if combined.Quality == "measured" {
		return measuredDuration(value, combined.IsOpen)
	}
	combined.Quality, combined.ValueMS, combined.KnownMS, combined.EstimateMS = "partial", nil, &value, nil
	return combined
}

func timingRef(kind, id, field string) TimingBoundaryRef {
	return TimingBoundaryRef{kind, id, field}
}

func (c timingCalculator) interval(node *TimingNode, metric string, span timingSpan, calendar, executor bool, anchors ...Observation) Duration {
	d, _ := c.measure(span, calendar, executor, anchors...)
	basis := "unknown"
	if span.from != nil {
		basis = span.from.SuspendBasis
	}
	method := "authority_observation_segments"
	if metric == "executor_time" {
		method = "driver_observed_executor_occupancy"
	}
	node.Intervals = append(node.Intervals, TimingInterval{Metric: metric, From: span.from, To: span.to, FromRef: span.fromRef, ToRef: span.toRef, Method: method, SuspendBasis: basis, Duration: d})
	node.Metrics[metric] = d
	return d
}

func (c timingCalculator) scopeEnd(kind, id string, settled *Observation) (*Observation, TimingBoundaryRef) {
	if settled != nil {
		if kind == "workflow_invocation" && !isInvocationState(c.r.SchemaVersion) {
			return settled, timingRef("run", c.r.ID, "settled")
		}
		return settled, timingRef(kind, id, "settled")
	}
	return &c.asOf, timingRef("report", c.r.ID, "as_of")
}

func (c timingCalculator) newNode(kind, id, status string, created Observation, settled *Observation) TimingNode {
	n := TimingNode{Kind: kind, ID: id, Status: status, Metrics: map[string]Duration{}, StateTime: map[string]Duration{}, Intervals: []TimingInterval{}, Children: []TimingNode{}, Reasons: []string{}}
	end, endRef := c.scopeEnd(kind, id, settled)
	createdRef := timingRef(kind, id, "created")
	if kind == "attempt" || kind == "check_execution" {
		createdRef.Field = "admitted"
	} else if kind == "workflow_invocation" && !isInvocationState(c.r.SchemaVersion) {
		createdRef = timingRef("run", c.r.ID, "created")
	}
	c.interval(&n, "elapsed", timingSpan{&created, end, createdRef, endRef, settled == nil}, true, false, c.scopeAnchors(kind, id)...)
	for _, metric := range []string{"ready_queue", "dispatch_latency", "executor_time", "post_execution_settlement", "result_to_acceptance", "executor_sum", "executor_active_union"} {
		n.Metrics[metric] = noDuration("not_applicable", "metric_belongs_to_execution_leaf", false)
	}
	if isContextState(c.r.SchemaVersion) {
		for _, metric := range contextTimingMetrics {
			n.Metrics[metric] = noDuration("not_applicable", "metric_belongs_to_check_execution", false)
		}
	}
	c.stateIntervals(&n, created, settled)
	c.controlIntervals(&n, created, settled)
	return n
}

func (c timingCalculator) scopeAnchors(kind, id string) []Observation {
	var anchors []Observation
	for _, change := range c.r.Transitions {
		owned := kind == "run" || kind == "workflow_invocation"
		switch kind {
		case "workflow_invocation":
			if isInvocationState(c.r.SchemaVersion) {
				invocationID := ""
				switch change.Kind {
				case "invocation":
					invocationID = change.ID
				case "activation":
					invocationID = c.activationInvocation(change.ID)
				case "step":
					if step := c.r.Steps[change.ID]; step != nil {
						invocationID = c.activationInvocation(step.ActivationID)
					}
				case "attempt":
					if attempt := c.r.Attempts[change.ID]; attempt != nil {
						invocationID = c.activationInvocation(attempt.ActivationID)
					}
				case "check_execution":
					if check := c.r.CheckExecutions[change.ID]; isContextState(c.r.SchemaVersion) && check != nil {
						invocationID = check.Request.InvocationID
					}
				}
				owned = invocationID != "" && c.r.withinInvocation(invocationID, id)
			}
		case "stage_activation":
			owned = change.Kind == "activation" && change.ID == id
			if step := c.r.Steps[change.ID]; change.Kind == "step" && step != nil {
				owned = step.ActivationID == id
			}
			if attempt := c.r.Attempts[change.ID]; change.Kind == "attempt" && attempt != nil {
				owned = attempt.ActivationID == id
			}
			if check := c.r.CheckExecutions[change.ID]; isContextState(c.r.SchemaVersion) && change.Kind == "check_execution" && check != nil {
				owned = check.Request.ActivationID == id
			}
		case "step_instance":
			owned = change.Kind == "step" && change.ID == id
			if attempt := c.r.Attempts[change.ID]; change.Kind == "attempt" && attempt != nil {
				owned = attempt.StepID == id
			}
			if check := c.r.CheckExecutions[change.ID]; isContextState(c.r.SchemaVersion) && change.Kind == "check_execution" && check != nil {
				step := c.r.Steps[id]
				owned = step != nil && check.Request.ActivationID == step.ActivationID
			}
		case "attempt":
			owned = change.Kind == "attempt" && change.ID == id
		case "check_execution":
			owned = change.Kind == "check_execution" && change.ID == id
		}
		if owned {
			anchors = append(anchors, change.At)
		}
	}
	return anchors
}

func (c timingCalculator) stateIntervals(node *TimingNode, created Observation, settled *Observation) {
	kind, id := node.Kind, node.ID
	// The foundation's one root invocation has the same lifecycle as its Run.
	if kind == "workflow_invocation" {
		if isInvocationState(c.r.SchemaVersion) {
			kind = "invocation"
		} else {
			kind, id = "run", c.r.ID
		}
	} else if kind == "stage_activation" {
		kind = "activation"
	} else if kind == "step_instance" {
		kind = "step"
	}
	var changes []StateChange
	for _, change := range c.r.Transitions {
		if change.Kind == kind && change.ID == id {
			changes = append(changes, change)
		}
	}
	if len(changes) == 0 {
		node.Reasons = append(node.Reasons, "state_history_not_recorded")
		return
	}
	cursor, cursorRef := created, timingRef(kind, id, "created")
	if kind == "attempt" || kind == "check_execution" {
		cursorRef.Field = "admitted"
	}
	state := changes[0].From
	initialKnown := state != "" || (clockComparable(created, changes[0].At) && created.MonotonicMS == changes[0].At.MonotonicMS)
	if !initialKnown {
		node.Reasons = append(node.Reasons, "initial_state_boundary_not_observed")
	}
	perState := map[string][]Duration{}
	appendState := func(to Observation, toRef TimingBoundaryRef, open bool) {
		if state == "" {
			return
		}
		from := cursor
		span := timingSpan{&from, &to, cursorRef, toRef, open}
		d, _ := c.measure(span, true, false)
		node.Intervals = append(node.Intervals, TimingInterval{Metric: "state_time", State: state, From: &from, To: &to, FromRef: cursorRef, ToRef: toRef, Method: "ordered_state_transitions", SuspendBasis: from.SuspendBasis, Duration: d})
		perState[state] = append(perState[state], d)
	}
	for i, change := range changes {
		ref := timingRef(kind, id, "transition:"+change.To)
		if i > 0 && change.From != state {
			node.Reasons = append(node.Reasons, "state_history_discontinuity")
			node.StateTime = map[string]Duration{}
			return
		}
		appendState(change.At, ref, false)
		cursor, cursorRef, state = change.At, ref, change.To
	}
	end, endRef := c.scopeEnd(kind, id, settled)
	appendState(*end, endRef, settled == nil)
	for state, parts := range perState {
		d := sumTiming(parts)
		if !initialKnown {
			d.ValueMS, d.EstimateMS = nil, nil
			if d.KnownMS != nil {
				d.Quality = "partial"
			}
			addReason(&d, "initial_state_boundary_not_observed")
		}
		node.StateTime[state] = d
	}
}

// clip uses a qualified common monotonic domain, never wall-time ordering.
func clipTiming(span timingSpan, created Observation, end Observation, startRef, endRef TimingBoundaryRef) (timingSpan, bool, bool) {
	if span.from == nil || span.to == nil || !clockComparable(*span.from, created) || !clockComparable(*span.to, end) || !clockComparable(created, end) {
		return span, false, false
	}
	if span.to.MonotonicMS <= created.MonotonicMS || span.from.MonotonicMS >= end.MonotonicMS {
		return span, false, true
	}
	if span.from.MonotonicMS < created.MonotonicMS {
		span.from, span.fromRef = &created, startRef
	}
	if span.to.MonotonicMS > end.MonotonicMS {
		span.to, span.toRef = &end, endRef
	}
	return span, true, true
}

func (c timingCalculator) controlIntervals(node *TimingNode, created Observation, settled *Observation) {
	end, endRef := c.scopeEnd(node.Kind, node.ID, settled)
	createdRef := timingRef(node.Kind, node.ID, "created")
	if node.Kind == "check_execution" {
		createdRef.Field = "admitted"
	}
	var durations []Duration
	var segments []clockSegment
	var cancel *Stop
	for i := range c.r.Stops {
		stop := &c.r.Stops[i]
		if !c.stopApplies(*node, *stop) {
			continue
		}
		if stop.Kind == "cancel" && cancel == nil {
			cancel = stop // journal/receipt order, not UTC order
		}
		stopEnd, stopRef := stop.Released, timingRef("stop", stop.ID, "released")
		if stopEnd == nil {
			stopEnd, stopRef = end, endRef
		}
		span := timingSpan{&stop.Created, stopEnd, timingRef("stop", stop.ID, "created"), stopRef, stop.Released == nil && settled == nil}
		clipped, relevant, comparable := clipTiming(span, created, *end, createdRef, endRef)
		if !comparable {
			durations = append(durations, noDuration("unavailable", "restriction_scope_clock_incomparable", span.open))
			continue
		}
		if !relevant {
			continue
		}
		d := c.interval(node, "restricted_time", clipped, true, false)
		durations = append(durations, d)
		_, known := c.measure(clipped, true, false)
		segments = append(segments, known...)
	}
	if len(durations) == 0 {
		node.Metrics["restricted_time"] = measuredDuration(0, false)
	} else {
		node.Metrics["restricted_time"] = unionTiming(durations, segments)
	}
	if cancel == nil {
		node.Metrics["cancel_to_settlement"] = noDuration("not_applicable", "cancel_not_requested", false)
	} else {
		span := timingSpan{&cancel.Created, end, timingRef("stop", cancel.ID, "created"), endRef, settled == nil}
		clipped, relevant, comparable := clipTiming(span, created, *end, createdRef, endRef)
		if !comparable {
			node.Metrics["cancel_to_settlement"] = noDuration("unavailable", "cancel_scope_clock_incomparable", span.open)
		} else if !relevant {
			node.Metrics["cancel_to_settlement"] = noDuration("not_applicable", "cancel_outside_scope", false)
		} else {
			c.interval(node, "cancel_to_settlement", clipped, true, false)
		}
	}
}

func (c timingCalculator) activationInvocation(id string) string {
	if a := c.r.Activations[id]; a != nil {
		return a.InvocationID
	}
	return ""
}

func (c timingCalculator) stopApplies(node TimingNode, stop Stop) bool {
	if !isInvocationState(c.r.SchemaVersion) || stop.Scope == "" || stop.Scope == "run" {
		return true
	}
	if stop.Scope != "invocation" {
		return false
	}
	invocationID := ""
	switch node.Kind {
	case "workflow_invocation":
		invocationID = node.ID
	case "stage_activation":
		invocationID = c.activationInvocation(node.ID)
	case "step_instance":
		if step := c.r.Steps[node.ID]; step != nil {
			invocationID = c.activationInvocation(step.ActivationID)
		}
	case "attempt":
		if attempt := c.r.Attempts[node.ID]; attempt != nil {
			invocationID = c.activationInvocation(attempt.ActivationID)
		}
	case "check_execution":
		if check := c.r.CheckExecutions[node.ID]; isContextState(c.r.SchemaVersion) && check != nil {
			invocationID = check.Request.InvocationID
		}
	}
	return invocationID != "" && c.r.withinInvocation(invocationID, stop.ScopeID)
}

func (c timingCalculator) attemptSpan(a *Attempt) timingSpan {
	end, ref := a.ExecutorEnd, timingRef("attempt", a.ID, "executor_end")
	if end == nil {
		end, ref = &c.asOf, timingRef("report", c.r.ID, "as_of")
	}
	return timingSpan{a.Started, end, timingRef("attempt", a.ID, "started"), ref, a.ExecutorEnd == nil}
}

func (c timingCalculator) attemptAnchors(a *Attempt) []Observation {
	var anchors []Observation
	if a.CandidateAt != nil {
		anchors = append(anchors, *a.CandidateAt)
	}
	return anchors
}

func (c timingCalculator) attempt(a *Attempt) TimingNode {
	n := c.newNode("attempt", a.ID, a.Status, a.Admitted, a.Settled)
	n.AttemptCount, n.attempts = 1, []string{a.ID}
	if a.Accepted != nil {
		n.Verdict = a.Accepted.Verdict
	}
	end, endRef := c.scopeEnd("attempt", a.ID, a.Settled)
	started, startRef := a.Started, timingRef("attempt", a.ID, "started")
	if started == nil {
		started, startRef = &c.asOf, timingRef("report", c.r.ID, "as_of")
	}
	c.interval(&n, "dispatch_latency", timingSpan{&a.Admitted, started, timingRef("attempt", a.ID, "admitted"), startRef, a.Started == nil}, false, a.Started == nil)
	if a.Started == nil {
		n.Metrics["executor_time"] = noDuration("unavailable", "executor_start_not_observed", a.Settled == nil)
		if a.Settled != nil && a.ProcessOutcome != nil && !a.ProcessOutcome.Started {
			n.Metrics["executor_time"] = noDuration("not_applicable", "executor_did_not_start", false)
		}
	} else {
		c.interval(&n, "executor_time", c.attemptSpan(a), false, true, c.attemptAnchors(a)...)
	}
	if a.CandidateAt == nil {
		n.Metrics["result_to_acceptance"] = noDuration("unavailable", "result_not_received", false)
	} else if isContextState(c.r.SchemaVersion) {
		c.contextAcceptance(&n, a)
	} else {
		c.interval(&n, "result_to_acceptance", timingSpan{a.CandidateAt, end, timingRef("attempt", a.ID, "candidate_at"), endRef, a.Settled == nil}, true, false)
	}
	if a.ExecutorEnd == nil {
		n.Metrics["post_execution_settlement"] = noDuration("unavailable", "executor_end_not_observed", false)
	} else {
		c.interval(&n, "post_execution_settlement", timingSpan{a.ExecutorEnd, end, timingRef("attempt", a.ID, "executor_end"), endRef, a.Settled == nil}, true, false)
	}
	return n
}

func (c timingCalculator) rollup(node *TimingNode) {
	if isContextState(c.r.SchemaVersion) {
		c.rollupChecks(node)
	}
	node.attempts = nil
	seen := map[string]bool{}
	for _, child := range node.Children {
		for _, id := range child.attempts {
			seen[id] = true
		}
	}
	for id := range seen {
		node.attempts = append(node.attempts, id)
	}
	sort.Strings(node.attempts)
	node.AttemptCount = len(node.attempts)
	var parts []Duration
	var segments []clockSegment
	basis := ""
	for _, id := range node.attempts {
		a := c.r.Attempts[id]
		if a == nil {
			parts = append(parts, noDuration("unavailable", "attempt_record_missing", false))
			continue
		}
		if a.Started == nil {
			parts = append(parts, noDuration("unavailable", "executor_start_not_observed", a.Settled == nil))
			continue
		}
		currentBasis := a.Started.Source + "\x00" + a.Started.SuspendBasis
		if basis != "" && basis != currentBasis {
			node.Metrics["executor_sum"] = noDuration("unavailable", "incompatible_measurement_basis", false)
			node.Metrics["executor_active_union"] = noDuration("unavailable", "incompatible_measurement_basis", false)
			return
		}
		basis = currentBasis
		d, known := c.measure(c.attemptSpan(a), false, true, c.attemptAnchors(a)...)
		parts = append(parts, d)
		segments = append(segments, known...)
	}
	node.Metrics["executor_sum"] = sumTiming(parts)
	node.Metrics["executor_active_union"] = unionTiming(parts, segments)
}

func (c timingCalculator) activation(a *Activation) TimingNode {
	node := c.newNode("stage_activation", a.ID, a.Status, a.Created, a.Settled)
	node.StageID, node.StageKind = a.StageID, a.Kind
	if a.Kind != "step" {
		node.Metrics["executor_time"] = noDuration("not_applicable", "control_stage_has_no_executor", false)
	}
	stepIDs := make([]string, 0)
	for id, step := range c.r.Steps {
		if step != nil && step.ActivationID == a.ID {
			stepIDs = append(stepIDs, id)
		}
	}
	sort.Strings(stepIDs)
	for _, stepID := range stepIDs {
		step := c.r.Steps[stepID]
		child := c.newNode("step_instance", step.ID, step.Status, step.Created, step.Settled)
		child.Verdict = step.Verdict
		seen := map[string]bool{}
		var first *Attempt
		for _, attemptID := range step.AttemptIDs {
			attempt := c.r.Attempts[attemptID]
			if seen[attemptID] {
				continue
			}
			seen[attemptID] = true
			if attempt == nil || attempt.StepID != step.ID {
				child.Reasons = append(child.Reasons, "attempt_reference_invalid")
				continue
			}
			if first == nil {
				first = attempt
			}
			child.Children = append(child.Children, c.attempt(attempt))
		}
		readyEnd, readyRef := c.scopeEnd("step_instance", step.ID, step.Settled)
		if first != nil {
			readyEnd, readyRef = &first.Admitted, timingRef("attempt", first.ID, "admitted")
		}
		c.interval(&child, "ready_queue", timingSpan{&step.Created, readyEnd, timingRef("step_instance", step.ID, "created"), readyRef, first == nil && step.Settled == nil}, true, false)
		c.rollup(&child)
		node.Children = append(node.Children, child)
	}
	if isContextState(c.r.SchemaVersion) {
		c.appendChecks(&node, a.InvocationID, a.ID)
	}
	c.rollup(&node)
	return node
}

func (c timingCalculator) invocation(id string, seen map[string]bool) (TimingNode, bool) {
	i := c.r.Invocations[id]
	if i == nil || i.ID != id || seen[id] {
		return TimingNode{}, false
	}
	seen[id] = true
	node := c.newNode("workflow_invocation", i.ID, i.Status, i.Created, i.Settled)
	node.Outcome = i.Outcome
	activationIDs := []string{}
	for id, a := range c.r.Activations {
		if a != nil && a.InvocationID == i.ID {
			activationIDs = append(activationIDs, id)
		}
	}
	sort.Strings(activationIDs)
	for _, id := range activationIDs {
		a := c.r.Activations[id]
		activation := c.activation(a)
		childIDs := []string{}
		for id, child := range c.r.Invocations {
			if child != nil && child.ParentInvocationID == i.ID && child.CallerActivationID == a.ID {
				childIDs = append(childIDs, id)
			}
		}
		sort.Strings(childIDs)
		for _, id := range childIDs {
			if child, ok := c.invocation(id, seen); ok {
				activation.Children = append(activation.Children, child)
			} else {
				activation.Reasons = append(activation.Reasons, "invocation_reference_invalid")
			}
		}
		c.rollup(&activation)
		node.Children = append(node.Children, activation)
	}
	if isContextState(c.r.SchemaVersion) {
		c.appendChecks(&node, i.ID, "")
	}
	c.rollup(&node)
	return node, true
}

// Timing projects only actual entities. Children have deterministic identity
// order, not invented UTC event order. No control stage receives a fake Attempt.
func Timing(r Run, asOf Observation, driverLive bool) TimingTree {
	c := timingCalculator{r, asOf, driverLive}
	root := c.newNode("run", r.ID, r.Status, r.Created, r.Settled)
	root.Outcome = r.Outcome
	revision := "foundation-timing/1"
	if isInvocationState(r.SchemaVersion) {
		revision = TimingCalculatorRevisionCore
		if isContextState(r.SchemaVersion) {
			revision = TimingCalculatorRevisionContext
		}
		seen := map[string]bool{}
		if invocation, ok := c.invocation(r.RootInvocationID, seen); ok {
			root.Children = append(root.Children, invocation)
		}
		if len(seen) != len(r.Invocations) || len(root.Children) == 0 {
			root.Reasons = append(root.Reasons, "invocation_tree_incomplete")
		}
	} else {
		invocation := c.newNode("workflow_invocation", r.RootInvocationID, r.Status, r.Created, r.Settled)
		invocation.Outcome = r.Outcome
		activationIDs := make([]string, 0, len(r.Activations))
		for id := range r.Activations {
			activationIDs = append(activationIDs, id)
		}
		sort.Strings(activationIDs)
		for _, id := range activationIDs {
			if a := r.Activations[id]; a != nil {
				invocation.Children = append(invocation.Children, c.activation(a))
			}
		}
		c.rollup(&invocation)
		root.Children = append(root.Children, invocation)
	}
	c.rollup(&root)
	if isContextState(r.SchemaVersion) && len(root.checks) != len(r.CheckExecutions) {
		root.Reasons = append(root.Reasons, "check_tree_incomplete")
	}
	return TimingTree{SchemaVersion: "1", CalculatorRevision: revision, RunID: r.ID, AsOf: asOf, DriverLive: driverLive, Root: root}
}

// TimingMetricNames is the closed foundation metric catalog used by query and
// DTO validation; state labels live in StateTime rather than dynamic metric keys.
func TimingMetricNames() []string {
	return strings.Fields("elapsed ready_queue dispatch_latency executor_time post_execution_settlement result_to_acceptance restricted_time cancel_to_settlement executor_sum executor_active_union")
}
