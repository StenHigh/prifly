package runtime

import (
	"fmt"
	"slices"

	"github.com/stenhigh/prifly/internal/flow"
)

func isArtifactClosureState(version string) bool {
	return atLeast(version, CoreArtifactClosureStateVersion)
}

// artifactClosureInvariant validates the exact state cut without reading the
// sealed manifest bytes. Engine.Artifact validates those bytes when consumed.
func artifactClosureInvariant(r Run) error {
	if !isArtifactClosureState(r.SchemaVersion) {
		if len(r.ArtifactClosures) != 0 {
			return fmt.Errorf("artifact closure invariant: older state contains closures")
		}
		return nil
	}
	if len(r.Publications)+len(r.ArtifactPublications)+len(r.ArtifactClosures) > MaxRunPublications {
		return fmt.Errorf("artifact closure invariant: global publication limit exceeded")
	}
	seen := map[string]bool{}
	lastSequence := int64(0)
	for _, closure := range r.ArtifactClosures {
		attempt := r.Attempts[closure.AttemptID]
		step := r.Steps[closure.StepID]
		if attempt == nil || step == nil || attempt.StepID != step.ID || attempt.ActivationID == "" || closure.Actor != "publisher:"+attempt.ID || closure.SchemaVersion != ArtifactClosureVersion || closure.AcceptedSequence <= lastSequence {
			return fmt.Errorf("artifact closure invariant: invalid producer or sequence")
		}
		lastSequence = closure.AcceptedSequence
		activation := r.Activations[attempt.ActivationID]
		if activation == nil || activation.StepID != step.ID || activation.StageID == "" {
			return fmt.Errorf("artifact closure invariant: invalid producer activation")
		}
		key := closure.StepID + "\x00" + closure.Hook
		if seen[key] || closure.ID != derivedID("artifact-closure", r.ID, closure.StepID, closure.Hook) {
			return fmt.Errorf("artifact closure invariant: duplicate or invalid logical key")
		}
		seen[key] = true
		manifestBytes, err := canonical(closure.Manifest)
		if err != nil || flow.ValidateProtocol("ArtifactRef", manifestBytes) != nil || closure.Manifest.Revision != 1 {
			return fmt.Errorf("artifact closure invariant: invalid manifest reference")
		}
		items := make([]string, 0, len(closure.ItemKeys))
		cut := int64(0)
		for _, publication := range r.ArtifactPublications {
			if publication.StepID == closure.StepID && publication.Hook == closure.Hook {
				items = append(items, publication.ItemKey)
				cut = publication.AcceptedSequence
			}
		}
		if closure.ItemCount != int64(len(closure.ItemKeys)) || !slices.Equal(items, closure.ItemKeys) || closure.CutSequence != cut || closure.AcceptedSequence <= cut {
			return fmt.Errorf("artifact closure invariant: manifest cut differs from accepted items")
		}
		for _, item := range closure.ItemKeys {
			itemBytes, _ := canonical(item)
			if flow.ValidateProtocol("Identifier", itemBytes) != nil {
				return fmt.Errorf("artifact closure invariant: invalid item key")
			}
		}
		if closure.Accepted.UTC == "" || closure.Accepted.Session == "" || closure.Accepted.MonotonicMS < 0 {
			return fmt.Errorf("artifact closure invariant: missing acceptance observation")
		}
	}
	return nil
}
