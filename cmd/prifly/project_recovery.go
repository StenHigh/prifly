package main

import (
	"context"
	"encoding/json"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// projectRecoverySource carries only sealed source bytes. The review output
// names the code commit that a new worktree must actually claim.
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
	var reviewID string
	for _, activation := range run.Activations {
		if activation != nil && activation.InvocationID == run.RootInvocationID && activation.StageID == "review" && activation.Kind == "call" && activation.Status == "completed" {
			reviewID = activation.ID
		}
	}
	if reviewID == "" {
		return request, nil, refusal("recover_subject_unproven", "source quality-tail has no completed review")
	}
	var implementation prifly.ArtifactRef
	for _, invocation := range run.Invocations {
		if invocation != nil && invocation.CallerActivationID == reviewID && invocation.Status == "completed" {
			implementation = invocation.Outputs["implementation"]
		}
	}
	if implementation == (prifly.ArtifactRef{}) {
		return request, nil, refusal("recover_subject_unproven", "review did not export a sealed implementation")
	}
	_, data, err := engine.Artifact(implementation)
	if err != nil {
		return request, nil, err
	}
	var subject projectImplementation
	if json.Unmarshal(data, &subject) != nil || !projectCommitID.MatchString(subject.HeadCommit) {
		return request, nil, refusal("recover_subject_unproven", "review implementation has no valid Git head commit")
	}
	request = prifly.RecoveryRequest{SourceRunID: run.ID, SourceRunVersion: view.RunVersion, SubjectCommit: subject.HeadCommit}
	return request, values, nil
}
