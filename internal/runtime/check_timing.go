package runtime

import "sort"

// These names are added only by the context-state calculator. The published
// foundation catalog remains byte-for-byte unchanged for earlier histories.
var contextTimingMetrics = []string{"admission_to_dispatch", "dispatch_to_start", "check_executor_sum", "check_executor_active_union"}

func ContextTimingMetricNames() []string {
	return append(TimingMetricNames(), contextTimingMetrics...)
}

// A pending acceptance is a known prefix, not a completed latency sample. A
// rejected candidate never becomes an accepted result merely because its Step
// has settled; the producer's own settlement remains its process boundary.
func (c timingCalculator) contextAcceptance(node *TimingNode, attempt *Attempt) {
	step := c.r.Steps[attempt.StepID]
	if step == nil || step.ActivationID != attempt.ActivationID {
		node.Metrics["result_to_acceptance"] = noDuration("unavailable", "acceptance_owner_missing", false)
		return
	}
	if attempt.Accepted == nil && step.Settled != nil {
		node.Metrics["result_to_acceptance"] = noDuration("not_applicable", "candidate_not_accepted", false)
		return
	}
	if attempt.Accepted != nil && step.Settled == nil {
		node.Metrics["result_to_acceptance"] = noDuration("unavailable", "acceptance_boundary_not_observed", false)
		return
	}
	end, ref := c.scopeEnd("step_instance", step.ID, step.Settled)
	d := c.interval(node, "result_to_acceptance", timingSpan{attempt.CandidateAt, end, timingRef("attempt", attempt.ID, "candidate_at"), ref, step.Settled == nil}, true, false)
	if attempt.Accepted == nil {
		rightCensored(&d, "acceptance_pending")
		node.Metrics["result_to_acceptance"] = d
		node.Intervals[len(node.Intervals)-1].Duration = d
	}
}

func rightCensored(d *Duration, reason string) {
	d.ValueMS, d.EstimateMS, d.IsOpen = nil, nil, true
	d.Quality = "unavailable"
	if d.KnownMS != nil {
		d.Quality = "partial"
	}
	addReason(d, reason)
}

func (c timingCalculator) checkSpan(check *CheckExecution) timingSpan {
	end, ref := check.ExecutorEnd, timingRef("check_execution", check.ID, "executor_end")
	if end == nil {
		end, ref = &c.asOf, timingRef("report", c.r.ID, "as_of")
	}
	return timingSpan{check.Started, end, timingRef("check_execution", check.ID, "started"), ref, check.ExecutorEnd == nil}
}

func (c timingCalculator) checkPhase(node *TimingNode, check *CheckExecution, metric string, from, to *Observation, fromField, toField string) {
	if from == nil {
		node.Metrics[metric] = noDuration("unavailable", "start_boundary_not_observed", check.Settled == nil)
		return
	}
	if to == nil && check.Settled != nil {
		node.Metrics[metric] = noDuration("not_applicable", "check_boundary_not_reached", false)
		return
	}
	end, ref, open := to, timingRef("check_execution", check.ID, toField), to == nil
	if open {
		end, ref = &c.asOf, timingRef("report", c.r.ID, "as_of")
	}
	d := c.interval(node, metric, timingSpan{from, end, timingRef("check_execution", check.ID, fromField), ref, open}, false, false)
	if open {
		rightCensored(&d, "check_boundary_pending")
		node.Metrics[metric] = d
		node.Intervals[len(node.Intervals)-1].Duration = d
	}
}

func (c timingCalculator) check(check *CheckExecution) TimingNode {
	node := c.newNode("check_execution", check.ID, check.Status, check.Admitted, check.Settled)
	node.checks = []string{check.ID}
	if activation := c.r.Activations[check.Request.ActivationID]; activation != nil {
		node.StageID = activation.StageID
	}
	c.checkPhase(&node, check, "admission_to_dispatch", &check.Admitted, check.Dispatch, "admitted", "dispatch")
	c.checkPhase(&node, check, "dispatch_to_start", check.Dispatch, check.Started, "dispatch", "started")
	c.checkPhase(&node, check, "dispatch_latency", &check.Admitted, check.Started, "admitted", "started")
	if check.Started == nil {
		node.Metrics["executor_time"] = noDuration("unavailable", "executor_start_not_observed", check.Settled == nil)
		if check.Settled != nil && check.ProcessOutcome != nil && !check.ProcessOutcome.Started {
			node.Metrics["executor_time"] = noDuration("not_applicable", "executor_did_not_start", false)
		}
	} else {
		// A live driver does not establish continuity of an uncertain checker.
		// Lifecycle transitions (including failure) are not worker liveness
		// observations, so they must not extend its known execution prefix.
		execution := c
		execution.driverLive = c.driverLive && (check.Status == "running" || check.Status == "stopping")
		execution.interval(&node, "executor_time", c.checkSpan(check), false, true)
	}
	if check.ExecutorEnd == nil {
		node.Metrics["post_execution_settlement"] = noDuration("unavailable", "executor_end_not_observed", check.Settled == nil)
	} else {
		end, ref := c.scopeEnd("check_execution", check.ID, check.Settled)
		d := c.interval(&node, "post_execution_settlement", timingSpan{check.ExecutorEnd, end, timingRef("check_execution", check.ID, "executor_end"), ref, check.Settled == nil}, true, false)
		if check.Settled == nil {
			rightCensored(&d, "check_settlement_pending")
			node.Metrics["post_execution_settlement"] = d
			node.Intervals[len(node.Intervals)-1].Duration = d
		}
	}
	// CheckResult is not StepResult. There is no separate persisted receipt
	// timestamp from which to invent a check-report acceptance interval.
	node.Metrics["result_to_acceptance"] = noDuration("not_applicable", "check_has_no_step_result", false)
	return node
}

func (c timingCalculator) appendChecks(node *TimingNode, invocationID, activationID string) {
	ids := []string{}
	for id, check := range c.r.CheckExecutions {
		if check != nil && check.ID == id && check.Request.CheckID == id && check.Request.RunID == c.r.ID && check.Request.InvocationID == invocationID && check.Request.ActivationID == activationID {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		node.Children = append(node.Children, c.check(c.r.CheckExecutions[id]))
	}
}

func (c timingCalculator) rollupChecks(node *TimingNode) {
	node.checks = nil
	seen := map[string]bool{}
	for _, child := range node.Children {
		for _, id := range child.checks {
			seen[id] = true
		}
	}
	for id := range seen {
		node.checks = append(node.checks, id)
	}
	sort.Strings(node.checks)
	var durations []Duration
	var segments []clockSegment
	basis := ""
	for _, id := range node.checks {
		check := c.r.CheckExecutions[id]
		if check == nil {
			durations = append(durations, noDuration("unavailable", "check_record_missing", false))
			continue
		}
		if check.Started == nil {
			quality, reason := "unavailable", "executor_start_not_observed"
			if check.Settled != nil && check.ProcessOutcome != nil && !check.ProcessOutcome.Started {
				quality, reason = "not_applicable", "executor_did_not_start"
			}
			durations = append(durations, noDuration(quality, reason, check.Settled == nil))
			continue
		}
		currentBasis := check.Started.Source + "\x00" + check.Started.SuspendBasis
		if basis != "" && basis != currentBasis {
			node.Metrics["check_executor_sum"] = noDuration("unavailable", "incompatible_measurement_basis", false)
			node.Metrics["check_executor_active_union"] = noDuration("unavailable", "incompatible_measurement_basis", false)
			return
		}
		basis = currentBasis
		execution := c
		execution.driverLive = c.driverLive && (check.Status == "running" || check.Status == "stopping")
		duration, known := execution.measure(c.checkSpan(check), false, true)
		durations = append(durations, duration)
		segments = append(segments, known...)
	}
	node.Metrics["check_executor_sum"] = sumTiming(durations)
	node.Metrics["check_executor_active_union"] = unionTiming(durations, segments)
}
