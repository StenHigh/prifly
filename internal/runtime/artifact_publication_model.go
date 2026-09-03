package runtime

import (
	"fmt"

	"github.com/stenhigh/prifly/internal/flow"
)

func isArtifactPublicationState(version string) bool {
	return version == CoreArtifactPublicationStateVersion || isArtifactClosureState(version)
}

func artifactConsumption(hook flow.Hook) string {
	if hook.Artifact != nil && hook.Artifact.EarlyConsumption {
		return "early"
	}
	return "after_producer_success"
}

// artifactPublicationInvariant checks the durable logical records. Artifact
// bytes are checked by Engine.Artifact at use time; a state-only invariant must
// not turn every read into filesystem I/O.
func artifactPublicationInvariant(r Run) error {
	if !isArtifactPublicationState(r.SchemaVersion) {
		if len(r.ArtifactPublications) != 0 || r.PendingArtifactPublication != nil {
			return fmt.Errorf("artifact publication invariant: older state contains records")
		}
		return nil
	}
	if !isPublicationChecksState(r.SchemaVersion) && r.PendingArtifactPublication != nil {
		return fmt.Errorf("artifact publication invariant: older state contains a pending check")
	}
	if len(r.Publications)+len(r.ArtifactPublications) > MaxRunPublications {
		return fmt.Errorf("artifact publication invariant: global publication limit exceeded")
	}
	logical := map[string]bool{}
	lastSequence := int64(0)
	for _, publication := range r.ArtifactPublications {
		attempt := r.Attempts[publication.AttemptID]
		step := r.Steps[publication.StepID]
		if attempt == nil || step == nil || attempt.StepID != step.ID || attempt.ActivationID == "" || publication.Actor != "publisher:"+attempt.ID || publication.SchemaVersion != ArtifactPublicationVersion || publication.AcceptedSequence <= lastSequence {
			return fmt.Errorf("artifact publication invariant: invalid producer or sequence")
		}
		lastSequence = publication.AcceptedSequence
		activation := r.Activations[attempt.ActivationID]
		if activation == nil || activation.StepID != step.ID || activation.StageID == "" {
			return fmt.Errorf("artifact publication invariant: invalid producer activation")
		}
		key := publication.StepID + "\x00" + publication.Hook + "\x00" + publication.ItemKey
		if logical[key] || publication.ID != derivedID("artifact-publication", r.ID, publication.StepID, publication.Hook, publication.ItemKey) {
			return fmt.Errorf("artifact publication invariant: duplicate or invalid logical key")
		}
		logical[key] = true
		refBytes, err := canonical(publication.Artifact)
		if err != nil || flow.ValidateProtocol("ArtifactRef", refBytes) != nil || publication.Artifact.Revision != 1 {
			return fmt.Errorf("artifact publication invariant: invalid artifact reference")
		}
		itemBytes, _ := canonical(publication.ItemKey)
		if flow.ValidateProtocol("Identifier", itemBytes) != nil {
			return fmt.Errorf("artifact publication invariant: invalid item key")
		}
		for _, evidence := range publication.ContentCheckEvidence {
			matched := false
			for _, check := range r.CheckExecutions {
				if check != nil && evidence.ID == derivedID("evidence", check.ID) && evidence.Digest != "" && check.Status == "completed" && check.Report != nil && check.Report.Status == "pass" && check.Request.Boundary == "artifact_publication" && len(check.Request.Subjects) == 1 && check.Request.Subjects[0] == publication.Artifact {
					matched = true
					break
				}
			}
			if !matched {
				return fmt.Errorf("artifact publication invariant: content-check evidence lacks its passing check")
			}
		}
		schemaBytes, _ := canonical(publication.SchemaRef)
		media, mediaErr := artifactMediaType(publication.Format, []string{publication.MediaType})
		if flow.ValidateProtocol("ImmutableRef", schemaBytes) != nil || mediaErr != nil || publication.MediaType != media || publication.SizeBytes < 0 || publication.SizeBytes > MaxArtifactBytes || publication.Artifact.Digest == "" || (publication.Classification != "public" && publication.Classification != "internal" && publication.Classification != "confidential") || (publication.Consumption != "early" && publication.Consumption != "after_producer_success") {
			return fmt.Errorf("artifact publication invariant: type, policy or evidence differs from the pinned hook")
		}
	}
	if pending := r.PendingArtifactPublication; pending != nil {
		if pending.ID != derivedID("pending-artifact-publication", r.ID, pending.StepID, pending.Hook, pending.ItemKey, pending.CommandID) || pending.AttemptID == "" || pending.ActivationID == "" || pending.CommandID == "" || pending.Artifact.Digest == "" || pending.SizeBytes < 0 || len(pending.CheckIDs) == 0 || len(pending.CheckIDs) > 128 {
			return fmt.Errorf("artifact publication invariant: invalid pending check")
		}
	}
	return nil
}
