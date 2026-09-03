package runtime

import (
	"slices"
	"sort"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// Keep this separate from telemetryDimensions: changing the legacy slice would
// rewrite every previously published descriptor, including at an earlier cut.
var telemetryCheckDimensions = []string{"check_id", "check_revision", "check_result", "check_boundary"}

func contextTelemetryDescriptor(name, kind, unit, scope, method string) TelemetryDescriptor {
	d := telemetryDescriptor(name, kind, unit, scope, "core", method, "observation", "supported")
	d.ID, d.Revision = "core:"+name+"/3", "3"
	d.Dimensions = append(d.Dimensions, telemetryCheckDimensions...)
	if kind == "occurrence" {
		d.Temporality = "delta"
	}
	return d
}

// A historical query must validate ownership without requiring the invocation
// to remain live, reading a mutable registry, or acquiring a dispatch slot.
func telemetryCheckLabels(r Run, plans map[flow.Ref]*flow.Plan, check *CheckExecution) (map[string]string, error) {
	if !isContextState(r.SchemaVersion) || check == nil || check.ID == "" || r.CheckExecutions[check.ID] != check || check.Request.CheckID != check.ID || check.Request.RunID != r.ID {
		return nil, local.ErrIntegrity
	}
	request := check.Request
	invocation := r.Invocations[request.InvocationID]
	if invocation == nil || invocation.ID != request.InvocationID || invocation.RunID != r.ID || invocation.WorkflowRef != request.WorkflowRef {
		return nil, local.ErrIntegrity
	}
	plan := plans[invocation.WorkflowRef]
	if plan == nil || request.PolicyRef != plan.Workflow.PolicyRef || request.PackageLockDigest != r.LockRef.Digest {
		return nil, local.ErrIntegrity
	}
	definition, exists := plan.Checks[request.CheckRef]
	if !exists {
		return nil, local.ErrIntegrity
	}
	labels := telemetryRunLabels(r)
	labels["status"], labels["verdict"] = check.Status, "unknown"
	labels["check_id"], labels["check_revision"] = definition.ID, request.CheckRef.String()
	labels["check_result"], labels["check_boundary"] = "unknown", request.Boundary
	labels["executor_id"] = definition.Executor.AdapterRef.String()
	var refs []flow.Ref
	if request.Boundary == "workflow_input" {
		port, exists := plan.Workflow.Inputs[request.Port]
		if !exists || request.ActivationID != "" || request.ProducerAttemptID != "" {
			return nil, local.ErrIntegrity
		}
		refs = port.ContentCheckRefs
	} else {
		activation := r.Activations[request.ActivationID]
		if activation == nil || activation.ID != request.ActivationID || activation.InvocationID != invocation.ID {
			return nil, local.ErrIntegrity
		}
		labels["stage_id"] = activation.StageID
		if request.Boundary == "workflow_output" {
			port, exists := plan.Workflow.Outputs[request.Port]
			if !exists || activation.Kind != "finish" || request.ProducerAttemptID != "" {
				return nil, local.ErrIntegrity
			}
			refs = port.ContentCheckRefs
		} else {
			step, exists := plan.Steps[activation.StageID]
			instance := r.Steps[activation.StepID]
			if !exists || activation.Kind != "step" || instance == nil || instance.ActivationID != activation.ID {
				return nil, local.ErrIntegrity
			}
			switch request.Boundary {
			case "step_input":
				port, exists := step.Inputs[request.Port]
				if !exists || request.ProducerAttemptID != "" {
					return nil, local.ErrIntegrity
				}
				refs = port.ContentCheckRefs
			case "step_output", "step_result":
				producer := r.Attempts[request.ProducerAttemptID]
				if producer == nil || producer.ActivationID != activation.ID || producer.StepID != instance.ID {
					return nil, local.ErrIntegrity
				}
				refs = step.ResultCheckRefs
				if request.Boundary == "step_output" {
					port, exists := step.Outputs[request.Port]
					if !exists {
						return nil, local.ErrIntegrity
					}
					refs = port.ContentCheckRefs
				}
			default:
				return nil, local.ErrIntegrity
			}
		}
	}
	if !slices.Contains(refs, request.CheckRef) || (request.Boundary == "step_result") != (definition.Kind == "result") {
		return nil, local.ErrIntegrity
	}
	if !slices.Contains([]string{"pending", "dispatching", "running", "stopping", "verifying", "completed", "failed", "cancelled", "uncertain"}, check.Status) {
		return nil, local.ErrIntegrity
	}
	terminal := slices.Contains([]string{"completed", "failed", "cancelled"}, check.Status)
	if terminal != (check.Settled != nil) || (check.Status == "completed") != (check.Report != nil) {
		return nil, local.ErrIntegrity
	}
	if report := check.Report; report != nil {
		if !slices.Contains([]string{"pass", "fail", "inconclusive"}, report.Status) || report.CheckID != check.ID || report.RunID != r.ID || report.RequestDigest != check.RequestBytes.Digest {
			return nil, local.ErrIntegrity
		}
		labels["check_result"] = report.Status
	}
	if check.Failure != "" {
		labels["code"] = check.Failure
	}
	return labels, nil
}

func (c *telemetryCollector) collectChecks(r Run, plans map[flow.Ref]*flow.Plan) error {
	for _, name := range ContextTimingMetricNames() {
		d := contextTelemetryDescriptor("timing."+name, "distribution", "ms", "entity", TimingCalculatorRevisionContext)
		c.descriptors[d.ID] = d
	}
	for _, name := range []string{"admitted", "dispatched", "started", "settled", "failed", "cancelled", "uncertain", "reports"} {
		d := contextTelemetryDescriptor("check."+name, "occurrence", "count", "check_execution", "durable_journal")
		c.descriptors[d.ID] = d
	}
	ids := make([]string, 0, len(r.CheckExecutions))
	for id := range r.CheckExecutions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		check := r.CheckExecutions[id]
		if check == nil || check.ID != id {
			return local.ErrIntegrity
		}
		labels, err := telemetryCheckLabels(r, plans, check)
		if err != nil {
			return err
		}
		subject := TelemetrySubject{Kind: "check_execution", ID: check.ID, RunID: r.ID}
		add := func(name string, observed *Observation) error {
			descriptor := c.descriptors["core:check."+name+"/3"]
			record := telemetryBase(descriptor, derivedID("telemetry-check", r.ID, check.ID, name), subject, observed, labels)
			record.Integer = telemetryPtr(int64(1))
			record.Generation = check.ID
			record.Evidence = []string{check.ID, check.Request.AdmissionID, check.Request.CheckRef.String(), check.RequestBytes.Digest}
			if name == "reports" && check.ReportBytes != nil {
				record.Evidence = append(record.Evidence, check.ReportBytes.Digest)
			}
			return c.add(record)
		}
		if err := add("admitted", &check.Admitted); err != nil {
			return err
		}
		for _, fact := range []struct {
			name     string
			observed *Observation
		}{{"dispatched", check.Dispatch}, {"started", check.Started}, {"settled", check.Settled}} {
			if fact.observed != nil {
				if err := add(fact.name, fact.observed); err != nil {
					return err
				}
			}
		}
		if check.Status == "failed" || check.Status == "cancelled" {
			if err := add(check.Status, check.Settled); err != nil {
				return err
			}
		}
		if check.Status == "uncertain" {
			var observed *Observation
			for _, change := range r.Transitions {
				if change.Kind == "check_execution" && change.ID == id && change.To == "uncertain" {
					point := change.At
					observed = &point
				}
			}
			if err := add("uncertain", observed); err != nil {
				return err
			}
		}
		if check.Report != nil {
			if err := add("reports", check.Settled); err != nil {
				return err
			}
		}
	}
	return nil
}
