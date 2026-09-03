package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

func TestPendingArtifactPublicationPinsEveryDeclaredCheck(t *testing.T) {
	check := flow.Ref{ID: "test:check/content", Version: "1.0.0", Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	hook := flow.Hook{SchemaRef: flow.Ref{ID: "test:schema/document", Version: "1.0.0", Digest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}, Artifact: &flow.ArtifactHook{Format: "json", ContentCheckRefs: []flow.Ref{check}}}
	attempt := &Attempt{ID: "attempt:producer", StepID: "step:producer"}
	activation := &Activation{ID: "activation:producer"}
	artifact := Artifact{ID: "artifact:document", Revision: 1, Digest: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", Format: "json", MediaType: "application/json", SizeBytes: 11}
	pending, err := newPendingArtifactPublication("run:one", "command:publish", attempt, activation, "document_created", "item", hook, artifact, Observation{Session: "clock:test", UTC: "2026-08-30T00:00:00Z"})
	if err != nil || pending.ID == "" || len(pending.CheckIDs) != 1 || pending.Artifact != artifact.Ref() {
		t.Fatalf("pending publication did not pin its exact check and sealed subject: %+v %v", pending, err)
	}
}

func TestArtifactPublicationChecksAcceptSealedItemBeforeProducerSettles(t *testing.T) {
	e, runID := driverProject(t, "artifact-publish-check", 20000)
	if _, err := e.SetAdmissionCapacity(context.Background(), CapacityRequest{CommandID: newID("command"), Capacity: 2, Reason: "qualify producer and its publication check"}); err != nil {
		t.Fatal(err)
	}
	attempt := driverAdmit(t, e, runID)
	data := []byte(`{"value":1}`)
	if err := os.WriteFile(filepath.Join(attempt.Workspace, "early.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	size := int64(len(data))
	command := PublishCommand{SchemaVersion: "2", CommandID: "command:checked-artifact-publication", RunID: runID, StepID: attempt.StepID, AttemptID: attempt.ID, EnvelopeDigest: attempt.EnvelopeDigest, Hook: "document_created", Kind: "artifact", CandidatePath: "early.json", ExpectedDigest: rawDigest(data), ExpectedSizeBytes: &size}
	r := driverRun(t, e, runID)
	activation := r.Activations[attempt.ActivationID]
	current := func(r *Run, _ Observation) (*Attempt, *Activation, error) {
		return r.Attempts[attempt.ID], r.Activations[attempt.ActivationID], nil
	}
	_, err := e.publishArtifactAs(context.Background(), command, r, attempt, activation, current, func(result local.ApplyResult, err error) (local.ApplyResult, error) { return result, err })
	if driverFailureCode(err, "") != "publication_check_pending" {
		t.Fatalf("publication with a declared check was not held: %v", err)
	}
	r, view, err := e.load(context.Background(), runID)
	if err != nil || r.SchemaVersion != CorePublicationChecksStateVersion || r.PendingArtifactPublication == nil || len(r.ArtifactPublications) != 0 || r.Attempts[attempt.ID].Settled != nil {
		t.Fatalf("candidate was not sealed-but-hidden while its producer remained live: state=%s pending=%+v publications=%+v attempt=%+v err=%v", r.SchemaVersion, r.PendingArtifactPublication, r.ArtifactPublications, r.Attempts[attempt.ID], err)
	}
	pendingNext, err := e.Next(context.Background(), runID)
	if err != nil || pendingNext.Action != "publication_checks" || pendingNext.InvocationID == "" {
		t.Fatalf("pending publication has no qualified next action: %+v %v", pendingNext, err)
	}
	for name, value := range map[string]any{"CoreRunStateV15": r, "CoreNextViewV15": pendingNext, "PendingArtifactPublication": r.PendingArtifactPublication} {
		if err := validatePublic(t, name, value); err != nil {
			t.Fatalf("%s rejects the pending checked-publication value: %v", name, err)
		}
	}
	if err := os.Remove(filepath.Join(attempt.Workspace, "early.json")); err != nil {
		t.Fatal(err)
	}
	_, err = e.publishArtifactAs(context.Background(), command, r, r.Attempts[attempt.ID], r.Activations[attempt.ActivationID], current, func(result local.ApplyResult, err error) (local.ApplyResult, error) { return result, err })
	if driverFailureCode(err, "") != "publication_check_pending" {
		t.Fatalf("retry reread a sealed pending candidate instead of retaining its boundary: %v", err)
	}
	if err := e.driveArtifactPublicationChecks(context.Background(), r, view); err != nil {
		t.Fatal(err)
	}
	r, view, err = e.load(context.Background(), runID)
	if err != nil || r.ActiveCheckID == "" {
		t.Fatalf("declared publication check was not admitted alongside the live producer: active_check=%q err=%v", r.ActiveCheckID, err)
	}
	if err := e.executePendingCheck(context.Background(), r, view, r.CheckExecutions[r.ActiveCheckID]); err != nil {
		t.Fatal(err)
	}
	r, view, err = e.load(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.driveArtifactPublicationChecks(context.Background(), r, view); err != nil {
		t.Fatal(err)
	}
	r = driverRun(t, e, runID)
	if r.PendingArtifactPublication != nil || len(r.ArtifactPublications) != 1 || len(r.ArtifactPublications[0].ContentCheckEvidence) != 1 || r.Attempts[attempt.ID].Settled != nil {
		t.Fatalf("passing check did not publish exactly the sealed item before producer settlement: pending=%+v publications=%+v attempt=%+v", r.PendingArtifactPublication, r.ArtifactPublications, r.Attempts[attempt.ID])
	}
	runView, err := e.View(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	next, err := e.Next(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]any{"CoreRunStateV15": r, "CoreRunViewV15": runView, "CoreNextViewV15": next, "ArtifactPublication": r.ArtifactPublications[0], "CoreCapabilitiesV15": Capabilities()} {
		if err := validatePublic(t, name, value); err != nil {
			t.Fatalf("%s rejects the live checked-publication value: %v", name, err)
		}
	}
}

func TestArtifactPublicationCheckFailureReleasesCandidateWithoutPublishing(t *testing.T) {
	e, runID := driverProject(t, "artifact-publish-check-fail", 20000)
	if _, err := e.SetAdmissionCapacity(context.Background(), CapacityRequest{CommandID: newID("command"), Capacity: 2, Reason: "qualify producer and its publication check"}); err != nil {
		t.Fatal(err)
	}
	attempt := driverAdmit(t, e, runID)
	data := []byte(`{"value":1}`)
	if err := os.WriteFile(filepath.Join(attempt.Workspace, "early.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	size := int64(len(data))
	command := PublishCommand{SchemaVersion: "2", CommandID: "command:failed-checked-artifact-publication", RunID: runID, StepID: attempt.StepID, AttemptID: attempt.ID, EnvelopeDigest: attempt.EnvelopeDigest, Hook: "document_created", Kind: "artifact", CandidatePath: "early.json", ExpectedDigest: rawDigest(data), ExpectedSizeBytes: &size}
	r := driverRun(t, e, runID)
	current := func(r *Run, _ Observation) (*Attempt, *Activation, error) {
		return r.Attempts[attempt.ID], r.Activations[attempt.ActivationID], nil
	}
	_, err := e.publishArtifactAs(context.Background(), command, r, attempt, r.Activations[attempt.ActivationID], current, func(result local.ApplyResult, err error) (local.ApplyResult, error) { return result, err })
	if driverFailureCode(err, "") != "publication_check_pending" {
		t.Fatalf("publication with a declared check was not held: %v", err)
	}
	r, view, err := e.load(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	firstCheckID := r.PendingArtifactPublication.CheckIDs[0]
	if err := e.driveArtifactPublicationChecks(context.Background(), r, view); err != nil {
		t.Fatal(err)
	}
	r, view, err = e.load(context.Background(), runID)
	if err != nil || r.ActiveCheckID == "" {
		t.Fatalf("failed publication check was not admitted: active=%q err=%v", r.ActiveCheckID, err)
	}
	if err := e.executePendingCheck(context.Background(), r, view, r.CheckExecutions[r.ActiveCheckID]); err != nil {
		t.Fatal(err)
	}
	r, view, err = e.load(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.driveArtifactPublicationChecks(context.Background(), r, view); err != nil {
		t.Fatal(err)
	}
	r = driverRun(t, e, runID)
	next, _ := nextKind(r)
	if r.PendingArtifactPublication != nil || len(r.ArtifactPublications) != 0 || r.Attempts[attempt.ID].Settled != nil || next != "active" {
		t.Fatalf("failed check left a visible item or blocked the live producer: pending=%+v publications=%+v attempt=%+v next=%q", r.PendingArtifactPublication, r.ArtifactPublications, r.Attempts[attempt.ID], next)
	}
	data = []byte(`{"value":2}`)
	if err := os.WriteFile(filepath.Join(attempt.Workspace, "early.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	size = int64(len(data))
	command.CommandID, command.ExpectedDigest, command.ExpectedSizeBytes = "command:replacement-checked-artifact-publication", rawDigest(data), &size
	_, err = e.publishArtifactAs(context.Background(), command, r, r.Attempts[attempt.ID], r.Activations[attempt.ActivationID], current, func(result local.ApplyResult, err error) (local.ApplyResult, error) { return result, err })
	if driverFailureCode(err, "") != "publication_check_pending" {
		t.Fatalf("an explicit replacement candidate was not held for a new check: %v", err)
	}
	r = driverRun(t, e, runID)
	if r.PendingArtifactPublication == nil || r.PendingArtifactPublication.CheckIDs[0] == firstCheckID {
		t.Fatalf("replacement candidate reused the failed check identity: %+v", r.PendingArtifactPublication)
	}
}
