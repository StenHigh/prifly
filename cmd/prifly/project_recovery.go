package main

import (
	"context"
	"encoding/json"

	"github.com/stenhigh/prifly/internal/flow"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// projectRecoverySource carries only sealed source bytes. The tree the stage
// runs in again is the source Run's own, which the plan hands over. Recovery
// takes a technical failure; resuming takes whatever the workflow declares,
// which the plan checks.
func projectRecoverySource(ctx context.Context, engine *prifly.Engine, runID, fromStage string, resuming bool) (prifly.RecoveryRequest, map[string]json.RawMessage, error) {
	var request prifly.RecoveryRequest
	view, err := engine.View(ctx, runID)
	if err != nil {
		return request, nil, err
	}
	run := view.Run
	if !resuming && (run.Status != "failed" || run.Outcome != nil) {
		return request, nil, refusal("recover_source_ineligible", "source Run must have a technical failure without an outcome")
	}
	values := map[string]json.RawMessage{}
	for name, ref := range run.Inputs {
		_, data, err := engine.Artifact(ref)
		if err != nil {
			return request, nil, err
		}
		values[name] = data
	}
	request = prifly.RecoveryRequest{SourceRunID: run.ID, SourceRunVersion: view.RunVersion, FromStage: fromStage}
	return request, values, nil
}

// projectResumes answers whether a continuation launch runs the source Run's
// own workflow, which makes it a resume rather than a continuation.
func projectResumes(ctx context.Context, engine *prifly.Engine, runID string, target *flow.Plan) (bool, error) {
	view, err := engine.View(ctx, runID)
	if err != nil {
		return false, err
	}
	return view.Run.WorkflowRef.ID == target.Workflow.ID, nil
}

// projectCarrySourceInputs adds the source Run's inputs, except those a
// decision answers: the answers must be the source's, which the plan checks.
func projectCarrySourceInputs(preflight projectPreflight, details projectLaunchDetail, carried, inputValues map[string]json.RawMessage) {
	decisionPorts := map[string]bool{}
	for _, decision := range preflight.Catalog.Decisions {
		if decision.Destination.Kind == "launch_input" {
			decisionPorts[decision.Destination.Name] = true
		}
	}
	for _, input := range details.Inputs {
		if input.Configured {
			decisionPorts[input.Name] = true
		}
	}
	for name, value := range carried {
		if !decisionPorts[name] {
			inputValues[name] = value
		}
	}
}
