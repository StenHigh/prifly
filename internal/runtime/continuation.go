package runtime

import (
	"context"
	"time"

	"github.com/stenhigh/prifly/internal/local"
)

const ContinuationReason = "project continuation"

// ContinuationSource names only artifacts produced on the declared classic
// route. The implementation is read for Git provenance, then replaced by a
// fresh input describing the current committed tree.
type ContinuationSource struct {
	RunID          string      `json:"source_run_id"`
	RunVersion     int64       `json:"source_run_version"`
	Task           ArtifactRef `json:"task"`
	Handoff        ArtifactRef `json:"handoff"`
	Plan           ArtifactRef `json:"plan"`
	Implementation ArtifactRef `json:"implementation"`
}

func continuationRefs(r Run, version int64) (ContinuationSource, error) {
	if r.Status != "completed" || r.Outcome == nil || (*r.Outcome != "partial" && *r.Outcome != "rejected") {
		return ContinuationSource{}, local.Reject("continuation_source_ineligible", "source Run must be completed with partial or rejected outcome")
	}
	if r.WorkflowRef.ID != "aif:workflow/classic" && r.WorkflowRef.ID != "aif-profiled:workflow/classic" {
		return ContinuationSource{}, local.Reject("continuation_source_ineligible", "source Run is not a declared AI Factory classic route")
	}
	result := ContinuationSource{RunID: r.ID, RunVersion: version, Task: r.Inputs["task"]}
	var lastImplementation time.Time
	for _, step := range r.Steps {
		if step == nil || step.Status != "completed" || step.Verdict != "pass" {
			continue
		}
		activation := r.Activations[step.ActivationID]
		if activation == nil || activation.InvocationID != r.RootInvocationID {
			continue
		}
		switch activation.StageID {
		case "warmup":
			result.Handoff = step.Outputs["handoff"]
		case "implement":
			if step.Settled == nil {
				return ContinuationSource{}, local.Reject("continuation_source_incomplete", "accepted implementation has no settled observation")
			}
			settled, err := time.Parse(time.RFC3339Nano, step.Settled.UTC)
			if err != nil || !lastImplementation.IsZero() && settled.Equal(lastImplementation) {
				return ContinuationSource{}, local.Reject("continuation_source_ambiguous", "accepted implementation order is unavailable")
			}
			if !lastImplementation.IsZero() && settled.Before(lastImplementation) {
				continue
			}
			lastImplementation = settled
			result.Implementation, result.Plan = step.Outputs["implementation"], step.Outputs["plan"]
		}
	}
	if result.Task == (ArtifactRef{}) || result.Handoff == (ArtifactRef{}) || result.Plan == (ArtifactRef{}) || result.Implementation == (ArtifactRef{}) {
		return ContinuationSource{}, local.Reject("continuation_source_incomplete", "source Run lacks task, warmup handoff, accepted plan or implementation")
	}
	return result, nil
}

func (e *Engine) ContinuationSource(ctx context.Context, runID string) (ContinuationSource, error) {
	r, read, err := e.load(ctx, runID)
	if err != nil {
		return ContinuationSource{}, err
	}
	refs, err := continuationRefs(r, read.Snapshot.Version)
	if err != nil {
		return refs, err
	}
	for _, ref := range []ArtifactRef{refs.Task, refs.Handoff, refs.Plan, refs.Implementation} {
		if _, _, err := e.Artifact(ref); err != nil {
			return ContinuationSource{}, err
		}
	}
	return refs, nil
}
