package runtime

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stenhigh/prifly/internal/local"
)

// This covers the first PUB-003 boundary: the worker publishes while still
// running, mutates and deletes its candidate, then retries the exact command.
// The accepted bytes and receipt remain the first sealed copy across exact and
// logical-key retries; a prior mismatch created no record and different bytes
// under the accepted key conflict. Concurrent mutation and crash/cancel around
// the commit remain parts of PUB-AC-05/06 that this test does not claim.
func TestArtifactPublicationSealsBeforeProducerSettlementAndDoesNotReread(t *testing.T) {
	e, runID := driverProject(t, "artifact-publish", 20000)
	_, finished := driverAsync(t, e, runID)
	select {
	case err := <-finished:
		r := driverRun(t, e, runID)
		var outcome any
		var helperError []byte
		for _, attempt := range r.Attempts {
			outcome = attempt.ProcessOutcome
			helperError, _ = os.ReadFile(filepath.Join(attempt.Workspace, "artifact-helper-error"))
		}
		t.Fatalf("driver stopped before artifact publication: %v status=%s outcome=%+v helper=%s diagnostics=%+v", err, r.Status, outcome, helperError, r.Diagnostics)
	case <-time.After(250 * time.Millisecond):
	}
	r := driverWait(t, e, runID, func(r Run) bool {
		if len(r.Active) != 1 || len(r.ArtifactPublications) != 1 {
			return false
		}
		attempt := r.Attempts[r.Active[0]]
		if attempt == nil || attempt.Workspace == "" {
			return false
		}
		_, err := os.Stat(filepath.Join(attempt.Workspace, "artifact-published"))
		return err == nil
	})
	if r.SchemaVersion != CoreArtifactPublicationStateVersion || r.Status != "running" || len(r.Outputs) != 0 {
		t.Fatalf("early publication changed producer lifecycle or final outputs: %+v", r)
	}
	attempt := r.Attempts[r.Active[0]]
	if attempt.Status != "running" || attempt.Settled != nil {
		t.Fatal("publication was not observed before producer settlement")
	}
	view, err := e.View(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	next, err := e.Next(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]any{
		"CoreRunStateV12": r, "CoreRunViewV12": view,
		"CoreNextViewV12": next, "CoreCapabilitiesV12": Capabilities(),
	} {
		if err := validatePublic(t, name, value); err != nil {
			t.Fatalf("%s rejects the live artifact-publication value: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(attempt.Workspace, "early.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("worker did not prove a retry after candidate removal: %v", err)
	}
	publication := r.ArtifactPublications[0]
	if publication.ItemKey != "item" || publication.Consumption != "early" || publication.AttemptID != attempt.ID || publication.ContentCheckEvidence == nil || len(publication.ContentCheckEvidence) != 0 {
		t.Fatalf("accepted publication lost its logical key, policy or provenance: %+v", publication)
	}
	if err := validatePublic(t, "ArtifactPublication", publication); err != nil {
		t.Fatal("artifact publication violates its public contract", err)
	}
	artifact, data, err := e.Artifact(publication.Artifact)
	if err != nil || !bytes.Equal(data, []byte(`{"value":1}`)) || artifact.Digest != publication.Artifact.Digest || artifact.SizeBytes != publication.SizeBytes || artifact.Classification != "internal" {
		t.Fatalf("accepted artifact does not resolve to the original sealed bytes: %+v %q %v", artifact, data, err)
	}
	if artifact.Producer["attempt_id"] != attempt.ID || artifact.Producer["port"] != "document_created" {
		t.Fatalf("artifact producer provenance is incomplete: %+v", artifact.Producer)
	}
	size := int64(len(data))
	command := PublishCommand{
		SchemaVersion: "2", CommandID: "command:artifact-publication-contract",
		RunID: runID, StepID: attempt.StepID, AttemptID: attempt.ID,
		EnvelopeDigest: attempt.EnvelopeDigest, Hook: "document_created", Kind: "artifact",
		CandidatePath: "early.json", ExpectedDigest: rawDigest(data), ExpectedSizeBytes: &size,
	}
	if err := validatePublic(t, "PublishStepPublicationCommandV2", command); err != nil {
		t.Fatal("the published command contract rejects the command accepted by runtime", err)
	}
	tooLong := command
	tooLong.CandidatePath = strings.Repeat("a", 4097)
	if validatePublishCommand(tooLong) == nil || validatePublic(t, "PublishStepPublicationCommandV2", tooLong) == nil {
		t.Fatal("runtime or public schema accepted an overlong artifact candidate path")
	}
	history, err := e.Store.Read(context.Background(), runID, 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	publicationEvents := []local.Event{}
	for _, event := range history.Events {
		if event.Type == "step.publication" && event.Version == local.EventVersion {
			publicationEvents = append(publicationEvents, event)
		}
	}
	if len(publicationEvents) != 1 || publication.AcceptedSequence != publicationEvents[0].Seq {
		t.Fatalf("mismatch/retries created extra publication events: record=%+v events=%+v", publication, publicationEvents)
	}

	if err := os.WriteFile(filepath.Join(attempt.Workspace, "finish"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	driverDone(t, finished, false)
	settled := driverRun(t, e, runID)
	if settled.Status != "completed" || settled.Outcome == nil || *settled.Outcome != "succeeded" || len(settled.ArtifactPublications) != 1 || !reflect.DeepEqual(settled.ArtifactPublications[0], publication) {
		t.Fatalf("producer settlement rewrote or discarded its early publication: %+v", settled)
	}
	_, settledData, err := e.Artifact(publication.Artifact)
	if err != nil || !bytes.Equal(settledData, data) {
		t.Fatalf("settlement changed sealed publication bytes: %q %v", settledData, err)
	}
}
