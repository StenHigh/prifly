package main

import (
	"context"
	"encoding/json"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// projectRecoverySource carries only sealed source bytes. The tree the failed
// stage runs in again is the source Run's own, which the plan hands over.
func projectRecoverySource(ctx context.Context, engine *prifly.Engine, runID string) (prifly.RecoveryRequest, map[string]json.RawMessage, error) {
	var request prifly.RecoveryRequest
	view, err := engine.View(ctx, runID)
	if err != nil {
		return request, nil, err
	}
	run := view.Run
	if run.Status != "failed" || run.Outcome != nil {
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
	request = prifly.RecoveryRequest{SourceRunID: run.ID, SourceRunVersion: view.RunVersion}
	return request, values, nil
}
