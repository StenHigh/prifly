package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestArtifactClosureSealsExactManifestBeforeProducerSettlement(t *testing.T) {
	e, runID := publicationSubscriptionRuntime(t)
	ctx := context.Background()
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	producerTask, err := e.SessionTask(ctx, runID, r.Active[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		key  string
		data []byte
	}{{"document-1", []byte(`{"value":1}`)}, {"document-2", []byte(`{"value":2}`)}} {
		slot := producerTask.Context.Outputs["document"]
		if err := os.WriteFile(filepath.Join(producerTask.Workspace, slot.Path), item.data, 0600); err != nil {
			t.Fatal(err)
		}
		size := int64(len(item.data))
		if _, err := e.PublishSessionPublication(ctx, PublishCommand{
			SchemaVersion: "3", CommandID: "command:artifact-" + item.key, RunID: runID,
			StepID: producerTask.StepInstanceID, AttemptID: producerTask.AttemptID, EnvelopeDigest: producerTask.EnvelopeDigest,
			Hook: "document_created", Kind: "artifact", ItemKey: item.key, CandidatePath: slot.Path,
			ExpectedDigest: rawDigest(item.data), ExpectedSizeBytes: &size,
		}); err != nil {
			t.Fatal(err)
		}
	}
	before, err := e.View(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	command := PublishCommand{
		SchemaVersion: "3", CommandID: "command:close-documents", RunID: runID,
		StepID: producerTask.StepInstanceID, AttemptID: producerTask.AttemptID, EnvelopeDigest: producerTask.EnvelopeDigest,
		Hook: "document_created", Kind: "close", ItemKeys: []string{"document-1", "document-2"},
	}
	incomplete := command
	incomplete.CommandID = "command:close-incomplete"
	incomplete.ItemKeys = []string{"document-1"}
	_, err = e.PublishSessionPublication(ctx, incomplete)
	rejectionCode(t, err, "artifact_manifest_conflict")
	first, err := e.PublishSessionPublication(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	r = driverRun(t, e, runID)
	after, err := e.View(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	next, err := e.Next(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := e.Preview(PreviewOptions{WorkflowFile: "workflows/overlap.json", BriefFile: "brief.json"})
	if err != nil {
		t.Fatal(err)
	}
	if r.SchemaVersion != CoreActionDeliveryStateVersion || len(r.ArtifactClosures) != 1 || len(r.ArtifactPublications) != 2 || after.RunVersion != before.RunVersion {
		t.Fatalf("close changed lifecycle or failed to record one exact cut: state=%s publications=%d closures=%d versions=%d/%d", r.SchemaVersion, len(r.ArtifactPublications), len(r.ArtifactClosures), before.RunVersion, after.RunVersion)
	}
	producer := r.Attempts[producerTask.AttemptID]
	if producer.Settled != nil || producer.Session == nil || producer.Session.HostState != SessionAwaiting {
		t.Fatalf("close settled its producer: %+v", producer)
	}
	closure := r.ArtifactClosures[0]
	manifestArtifact, manifestBytes, err := e.Artifact(closure.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	var manifest ArtifactManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.ItemCount != 2 || manifest.CutSequence != r.ArtifactPublications[1].AcceptedSequence || !reflect.DeepEqual(manifest.Items, r.ArtifactPublications) || !reflect.DeepEqual(closure.ItemKeys, command.ItemKeys) || closure.Manifest.Digest != rawDigest(manifestBytes) {
		t.Fatalf("sealed manifest differs from the accepted publication cut: closure=%+v manifest=%+v publications=%+v", closure, manifest, r.ArtifactPublications)
	}
	if len(manifestArtifact.Provenance) != 0 {
		t.Fatalf("manifest duplicated its unbounded item list into bounded ArtifactRevision provenance: %+v", manifestArtifact.Provenance)
	}
	for name, value := range map[string]any{
		"CoreRunStateV21": r, "CoreRunViewV21": after, "CoreNextViewV21": next, "CorePreviewV21": preview,
		"CoreWorkflowInvocationV21": r.Invocations[r.RootInvocationID], "ArtifactManifest": manifest,
		"ArtifactClosure": closure, "PublishStepPublicationCommandV3": command, "CoreCapabilitiesV21": Capabilities(),
	} {
		if err := validatePublic(t, name, value); err != nil {
			t.Fatalf("%s rejects the live closure value: %v", name, err)
		}
	}
	exact, err := e.PublishSessionPublication(ctx, command)
	if err != nil || !bytes.Equal(first.Receipt.Result, exact.Receipt.Result) {
		t.Fatalf("exact close retry changed its result: %v", err)
	}
	logical := command
	logical.CommandID = "command:close-documents-again"
	again, err := e.PublishSessionPublication(ctx, logical)
	if err != nil || !bytes.Equal(first.Receipt.Result, again.Receipt.Result) {
		t.Fatalf("logical close retry changed its result: %v", err)
	}
	conflict := logical
	conflict.CommandID = "command:close-documents-conflict"
	conflict.ItemKeys = []string{"document-2", "document-1"}
	_, err = e.PublishSessionPublication(ctx, conflict)
	rejectionCode(t, err, "artifact_close_conflict")

	missingSize := int64(2)
	_, err = e.PublishSessionPublication(ctx, PublishCommand{
		SchemaVersion: "3", CommandID: "command:late-artifact", RunID: runID,
		StepID: producerTask.StepInstanceID, AttemptID: producerTask.AttemptID, EnvelopeDigest: producerTask.EnvelopeDigest,
		Hook: "document_created", Kind: "artifact", ItemKey: "document-3", CandidatePath: "missing.json",
		ExpectedDigest: rawDigest([]byte(`{}`)), ExpectedSizeBytes: &missingSize,
	})
	rejectionCode(t, err, "artifact_hook_closed")
}

func TestArtifactClosureAcceptsAnExplicitEmptyManifest(t *testing.T) {
	e, runID := publicationSubscriptionRuntime(t)
	ctx := context.Background()
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	task, err := e.SessionTask(ctx, runID, r.Active[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.PublishSessionPublication(ctx, PublishCommand{
		SchemaVersion: "3", CommandID: "command:close-empty", RunID: runID,
		StepID: task.StepInstanceID, AttemptID: task.AttemptID, EnvelopeDigest: task.EnvelopeDigest,
		Hook: "document_created", Kind: "close", ItemKeys: []string{},
	}); err != nil {
		t.Fatal(err)
	}
	r = driverRun(t, e, runID)
	if len(r.ArtifactPublications) != 0 || len(r.ArtifactClosures) != 1 || r.ArtifactClosures[0].ItemCount != 0 || r.ArtifactClosures[0].CutSequence != 0 {
		t.Fatalf("empty close invented an item: %+v", r.ArtifactClosures)
	}
	_, data, err := e.Artifact(r.ArtifactClosures[0].Manifest)
	if err != nil {
		t.Fatal(err)
	}
	var manifest ArtifactManifest
	if err := json.Unmarshal(data, &manifest); err != nil || manifest.Items == nil || len(manifest.Items) != 0 {
		t.Fatalf("empty manifest is not an explicit empty array: %s %v", data, err)
	}
	for _, registration := range r.Waits {
		if r.Invocations[registration.InvocationID].BranchID == "consumer" && registration.Status != "active" {
			t.Fatalf("empty close woke the once subscriber: %+v", registration)
		}
	}
}

func TestArtifactCloseWireKeepsExplicitEmptyMembership(t *testing.T) {
	valid := []byte(`{"schema_version":"3","command_id":"command:close-empty","run_id":"run:wire","step_instance_id":"step:wire","attempt_id":"attempt:wire","envelope_digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","hook":"documents","kind":"close","item_keys":[]}`)
	command, err := ParsePublishCommand(valid)
	if err != nil || command.ItemKeys == nil || len(command.ItemKeys) != 0 || validatePublishCommand(command) != nil {
		t.Fatalf("explicit empty membership was lost: %+v %v", command, err)
	}
	for _, invalid := range [][]byte{
		bytes.Replace(valid, []byte(`"item_keys":[]`), []byte(`"item_keys":null`), 1),
		bytes.Replace(valid, []byte(`,"item_keys":[]`), nil, 1),
		bytes.Replace(valid, []byte(`"item_keys":[]`), []byte(`"item_keys":[],"candidate_path":null`), 1),
	} {
		parsed, parseErr := ParsePublishCommand(invalid)
		if parseErr == nil && validatePublishCommand(parsed) == nil {
			t.Fatalf("closed wire accepted absent, null or foreign fields: %s", invalid)
		}
	}
}
