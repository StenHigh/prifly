package runtime

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

func publicationSubscriptionRuntime(t *testing.T) (*Engine, string) {
	return publicationSubscriptionRuntimeInitial(t, "retained")
}

func publicationSubscriptionRuntimeInitial(t *testing.T, initial string) (*Engine, string) {
	return publicationSubscriptionRuntimeSource(t, initial, "wait_until_timeout")
}

func publicationSubscriptionRuntimeSource(t *testing.T, initial, producerFailure string) (*Engine, string) {
	return publicationSubscriptionRuntimeSourceFormat(t, initial, producerFailure, "json")
}

func publicationSubscriptionRuntimeSourceFormat(t *testing.T, initial, producerFailure, format string) (*Engine, string) {
	t.Helper()
	e := contextRegistryRuntime(t)
	definitions, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	documentSchema := map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object", "required": []string{"value"}, "properties": map[string]any{"value": map[string]any{"type": "integer"}}, "additionalProperties": false}
	if format == "blob" {
		documentSchema = map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object", "required": []string{"media_type", "size_bytes", "digest"}, "properties": map[string]any{"media_type": map[string]any{"const": "text/plain"}, "size_bytes": map[string]any{"type": "integer", "minimum": 0}, "digest": map[string]any{"type": "string"}}, "additionalProperties": false}
	}
	documentBytes := writeRegistryDocument(t, e, "schemas/document.json", documentSchema)
	documentRef := flow.Ref{ID: "test:schema/document", Version: "1.0.0", Digest: rawDigest(documentBytes)}
	artifactPort := flow.Port{Format: "json", SchemaRef: &documentRef}
	artifactMediaTypes := []string(nil)
	if format == "blob" {
		artifactPort, artifactMediaTypes = flow.Port{Format: "blob", MediaTypes: []string{"text/plain"}}, []string{"text/plain"}
	}
	keySchema := map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "string", "const": "item"}
	keyBytes := writeRegistryDocument(t, e, "schemas/item-key.json", keySchema)
	keyRef := flow.Ref{ID: "test:schema/item-key", Version: "1.0.0", Digest: rawDigest(keyBytes)}

	producerSkill := []byte("---\nname: publish-document\n---\n\nProduce one document.\n")
	consumerSkill := []byte("---\nname: consume-document\n---\n\nConsume one sealed document.\n")
	if err := os.WriteFile(filepath.Join(e.Root, "resources/producer.md"), producerSkill, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.Root, "resources/consumer.md"), consumerSkill, 0600); err != nil {
		t.Fatal(err)
	}
	producerSkillRef := flow.Ref{ID: "test:context/producer", Version: "1.0.0", Digest: rawDigest(producerSkill)}
	consumerSkillRef := flow.Ref{ID: "test:context/consumer", Version: "1.0.0", Digest: rawDigest(consumerSkill)}

	producerStep := flow.StepDefinition{
		SchemaVersion: "4", ID: "test:step/producer", Version: "1.0.0", Title: "Publish one document", Kind: "worker",
		Inputs: map[string]flow.InputPort{}, Outputs: map[string]flow.OutputPort{
			"document": {Port: flow.Port{Format: "json", SchemaRef: &documentRef}, RequiredFor: []string{"pass"}},
		},
		InstructionsRef: &producerSkillRef, ContextRefs: []flow.Ref{}, RequiredCapabilities: []string{},
		ResultCheckRefs: []flow.Ref{}, ResultSchemaRef: builtinRef(definitions, "core:schema/step-result"),
		Hooks: map[string]flow.Hook{"document_created": {
			Kind: "artifact", SchemaRef: documentRef, Description: "One sealed document", Classification: "internal", ReadPolicy: "declared_subscribers",
			MaxPayloadBytes: 4096, MaxCount: 3, MaxPerMinute: 3,
			Artifact: &flow.ArtifactHook{Format: format, MediaTypes: artifactMediaTypes, Cardinality: "keyed_many", ContentCheckRefs: []flow.Ref{}, EarlyConsumption: true},
		}},
	}
	producerStep.Executor.AdapterRef, producerStep.Executor.Operation = builtinRef(definitions, "core:adapter/assisted-session"), "session"
	producerStep.Effects.Class, producerStep.Effects.RetryClass = "none", "never"
	producerStepBytes := writeRegistryDocument(t, e, "steps/producer.json", producerStep)
	producerStepRef := flow.Ref{ID: producerStep.ID, Version: producerStep.Version, Digest: rawDigest(producerStepBytes)}

	consumerStep := flow.StepDefinition{
		SchemaVersion: "2", ID: "test:step/consumer", Version: "1.0.0", Title: "Consume one document", Kind: "worker",
		Inputs: map[string]flow.InputPort{"document": {Port: artifactPort, Required: true}}, Outputs: map[string]flow.OutputPort{},
		InstructionsRef: &consumerSkillRef, ContextRefs: []flow.Ref{}, RequiredCapabilities: []string{},
		ResultCheckRefs: []flow.Ref{}, ResultSchemaRef: builtinRef(definitions, "core:schema/step-result"),
	}
	consumerStep.Executor.AdapterRef, consumerStep.Executor.Operation = builtinRef(definitions, "core:adapter/assisted-session"), "session"
	consumerStep.Effects.Class, consumerStep.Effects.RetryClass = "none", "never"
	consumerStepBytes := writeRegistryDocument(t, e, "steps/consumer.json", consumerStep)
	consumerStepRef := flow.Ref{ID: consumerStep.ID, Version: consumerStep.Version, Digest: rawDigest(consumerStepBytes)}

	producer := flow.WorkflowRevision{
		SchemaVersion: "1", ID: "test:workflow/producer", Version: "1.0.0", Title: "Producer branch",
		Inputs: map[string]flow.InputPort{}, Outputs: map[string]flow.OutputPort{}, AllowedOutcomes: []string{"succeeded"},
		Limits:    flow.Limits{MaxStepInstances: 2, MaxControlTransitions: 16, MaxParallelism: 1},
		PolicyRef: builtinVersionRef(definitions, "core:policy/local", "2.0.0"),
	}
	producer.Definition.Entry = "produce"
	producer.Definition.Stages = map[string]flow.Stage{
		"produce": {Kind: "step", StepRef: producerStepRef, InputBindings: map[string]flow.Binding{}, On: map[string]string{"pass": "done"}},
		"done":    {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]flow.Binding{}},
	}
	producerBytes := writeRegistryDocument(t, e, "workflows/producer.json", producer)
	producerRef := flow.Ref{ID: producer.ID, Version: producer.Version, Digest: rawDigest(producerBytes)}

	sourceVersion := flow.PublicationSourceVersion
	if format == "blob" {
		sourceVersion = flow.PublicationBlobSourceVersion
	} else if producerFailure == "interrupt_on_terminal_failure" {
		sourceVersion = flow.PublicationFailureSourceVersion
	} else if initial == "new_only" {
		sourceVersion = flow.PublicationNewOnlySourceVersion
	}
	source := flow.PublicationSourceDefinition{
		SchemaVersion: sourceVersion, ID: "test:source/document", Version: "1.0.0", Mode: "once",
		ProducerBranchID: "producer", ProducerStageID: "produce", Hook: "document_created", HookSchemaRef: documentRef,
		ItemKey: "item", Initial: initial, ProducerFailure: producerFailure,
	}
	if format == "blob" {
		source.Format, source.MediaTypes = format, artifactMediaTypes
	}
	sourceBytes := writeRegistryDocument(t, e, "sources/document.json", source)
	sourceRef := flow.Ref{ID: source.ID, Version: source.Version, Digest: rawDigest(sourceBytes)}
	timeout := int64(60)
	consumer := flow.WorkflowRevision{
		SchemaVersion: "1", ID: "test:workflow/consumer", Version: "1.0.0", Title: "Consumer branch",
		Inputs: map[string]flow.InputPort{}, Outputs: map[string]flow.OutputPort{}, AllowedOutcomes: []string{"succeeded", "rejected"},
		Limits:    flow.Limits{MaxStepInstances: 2, MaxControlTransitions: 24, MaxParallelism: 1},
		PolicyRef: builtinVersionRef(definitions, "core:policy/local", "2.0.0"),
	}
	consumer.Definition.Entry = "await_document"
	consumer.Definition.Stages = map[string]flow.Stage{
		"await_document": {
			Kind: "wait", SourceRef: sourceRef, EventType: "artifact.published", EventSchemaRef: documentRef,
			CorrelationInput: &flow.Binding{From: "literal", SchemaRef: &keyRef, Value: []byte(`"item"`)},
			TimeoutSeconds:   &timeout, OnEvent: "consume", OnTimeout: "expired",
		},
		"consume": {
			Kind: "step", StepRef: consumerStepRef,
			InputBindings: map[string]flow.Binding{"document": {From: "stage_output", StageID: "await_document", Port: flow.WaitEventPort}},
			On:            map[string]string{"pass": "done"},
		},
		"done":    {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]flow.Binding{}},
		"expired": {Kind: "finish", Outcome: "rejected", OutputBindings: map[string]flow.Binding{}},
	}
	consumerBytes := writeRegistryDocument(t, e, "workflows/consumer.json", consumer)
	consumerRef := flow.Ref{ID: consumer.ID, Version: consumer.Version, Digest: rawDigest(consumerBytes)}

	parent := flow.WorkflowRevision{
		SchemaVersion: "1", ID: "test:workflow/publication-overlap", Version: "1.0.0", Title: "Publish and consume before producer settlement",
		Inputs: map[string]flow.InputPort{}, Outputs: map[string]flow.OutputPort{}, AllowedOutcomes: []string{"succeeded", "rejected"},
		Limits:    flow.Limits{MaxStepInstances: 8, MaxControlTransitions: 96, MaxParallelism: 2, MaxChildDepth: 2},
		PolicyRef: builtinVersionRef(definitions, "core:policy/local", "3.0.0"),
	}
	parent.Definition.Entry = "fan"
	parent.Definition.Stages = map[string]flow.Stage{
		"fan": {
			Kind: "parallel", MaxParallelism: 2,
			ParallelBranches: []flow.ParallelBranch{
				{ID: "producer", WorkflowRef: producerRef, InputBindings: map[string]flow.Binding{}},
				{ID: "consumer", WorkflowRef: consumerRef, InputBindings: map[string]flow.Binding{}},
			},
			Join: &flow.Join{Mode: "all", AcceptOutcomes: []string{"succeeded"}, Selection: "all", Remainder: "wait"},
			On:   map[string]string{"satisfied": "done", "unsatisfied": "rejected"},
		},
		"done":     {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]flow.Binding{}},
		"rejected": {Kind: "finish", Outcome: "rejected", OutputBindings: map[string]flow.Binding{}},
	}
	writeRuntimeJSON(t, filepath.Join(e.Root, "workflows/overlap.json"), parent)

	registry := RegistryFile{SchemaVersion: "3", Entries: []Definition{
		{Ref: documentRef, Kind: "schema", Path: "schemas/document.json"},
		{Ref: keyRef, Kind: "schema", Path: "schemas/item-key.json"},
		{Ref: producerSkillRef, Kind: "resource", Path: "resources/producer.md", ByteEncoding: "utf8_text", MediaType: "text/markdown; charset=utf-8"},
		{Ref: consumerSkillRef, Kind: "resource", Path: "resources/consumer.md", ByteEncoding: "utf8_text", MediaType: "text/markdown; charset=utf-8"},
		{Ref: producerStepRef, Kind: "step", Path: "steps/producer.json"},
		{Ref: consumerStepRef, Kind: "step", Path: "steps/consumer.json"},
		{Ref: producerRef, Kind: "workflow", Path: "workflows/producer.json"},
		{Ref: consumerRef, Kind: "workflow", Path: "workflows/consumer.json"},
		{Ref: sourceRef, Kind: "resource", Path: "sources/document.json"},
	}}
	writeRuntimeJSON(t, filepath.Join(e.Root, e.Config.Configuration.RegistryFile), registry)
	e.Config.Configuration.SchemaVersion = CoreContextConfigVersion
	e.Config.ConfigurationSchemaRef = builtinVersionRef(definitions, "core:schema/core-configuration", "2.0.0")
	e.Config.AdapterBindings["local_process"] = builtinVersionRef(definitions, "core:adapter/local-process", "2.0.0")
	e.Config.DefaultPolicyRef = builtinVersionRef(definitions, "core:policy/local", "2.0.0")
	writeRuntimeJSON(t, filepath.Join(e.Root, "prifly.json"), e.Config)
	writeRuntimeJSON(t, filepath.Join(e.Root, "brief.json"), Brief{"1", "test:brief/publication-overlap", "Consume an early document", "Consumer starts before producer settles", []string{"One sealed document"}, []string{"External writes"}, []string{"Producer and consumer overlap"}, []ArtifactRef{}, []string{}, "explicit"})
	if _, err := e.SetAdmissionCapacity(context.Background(), CapacityRequest{CommandID: newID("command"), Capacity: 2, Reason: "qualify producer and consumer overlap"}); err != nil {
		t.Fatal(err)
	}
	result, err := e.Start(context.Background(), StartOptions{CommandID: newID("command"), WorkflowFile: "workflows/overlap.json", BriefFile: "brief.json", Inputs: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	return e, result.Receipt.RunID
}

func sessionPass(t *testing.T, task SessionTask, outputs map[string]ArtifactRef) SessionSubmission {
	t.Helper()
	result, err := canonical(map[string]any{
		"schema_version": "1", "run_id": task.RunID, "step_instance_id": task.StepInstanceID,
		"attempt_id": task.AttemptID, "envelope_digest": task.EnvelopeDigest, "verdict": "pass",
		"outputs": outputs, "evidence_refs": []any{}, "effect_receipt_refs": []any{}, "summary": "done",
	})
	if err != nil {
		t.Fatal(err)
	}
	return SessionSubmission{SchemaVersion: task.SchemaVersion, RunID: task.RunID, AttemptID: task.AttemptID, EnvelopeDigest: task.EnvelopeDigest, Result: result}
}

func sessionFail(t *testing.T, task SessionTask) SessionSubmission {
	t.Helper()
	result, err := canonical(map[string]any{
		"schema_version": "1", "run_id": task.RunID, "step_instance_id": task.StepInstanceID,
		"attempt_id": task.AttemptID, "envelope_digest": task.EnvelopeDigest, "verdict": "fail",
		"outputs": map[string]ArtifactRef{}, "evidence_refs": []any{}, "effect_receipt_refs": []any{}, "summary": "failed",
	})
	if err != nil {
		t.Fatal(err)
	}
	return SessionSubmission{SchemaVersion: task.SchemaVersion, RunID: task.RunID, AttemptID: task.AttemptID, EnvelopeDigest: task.EnvelopeDigest, Result: result}
}

func TestOncePublicationInterruptsOnTerminalProducerFailure(t *testing.T) {
	e, runID := publicationSubscriptionRuntimeSource(t, "retained", "interrupt_on_terminal_failure")
	ctx := context.Background()
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatal(err)
	}
	before := driverRun(t, e, runID)
	if before.SchemaVersion != CoreActionDeliveryStateVersion || len(before.Active) != 1 {
		t.Fatalf("terminal-failure source did not establish current-session producer overlap: state=%s active=%v", before.SchemaVersion, before.Active)
	}
	producer, err := e.SessionTask(ctx, runID, before.Active[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.SubmitSession(ctx, sessionFail(t, producer)); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatal(err)
	}
	after := driverRun(t, e, runID)
	var wait *Activation
	for _, candidate := range after.Activations {
		if candidate.Kind == "wait" && after.Invocations[candidate.InvocationID].BranchID == "consumer" {
			wait = candidate
		}
	}
	if wait == nil || wait.Wait == nil || wait.Wait.Resolution != "producer_failed" || wait.Wait.EventRef != nil {
		t.Fatalf("producer failure did not resolve the once wait explicitly: %+v", wait)
	}
	registration := after.Waits[wait.Wait.RegistrationID]
	if registration == nil || registration.Status != "interrupted" {
		t.Fatalf("once producer failure retained an active or expired registration: %+v", registration)
	}
	if err := validatePublic(t, "CoreRunStateV21", after); err != nil {
		t.Fatalf("current session state rejects the terminal-failure interruption: %v", err)
	}
}

// PUB-004 once qualification: the producer is still held by its host when the
// consumer receives a distinct admission whose input is the sealed publication.
func TestOncePublicationAdmitsConsumerBeforeProducerSettlement(t *testing.T) {
	e, runID := publicationSubscriptionRuntime(t)
	ctx := context.Background()
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	if len(r.Active) != 1 {
		t.Fatalf("producer should be the only admitted attempt before publication: %v", r.Active)
	}
	producerTask, err := e.SessionTask(ctx, runID, r.Active[0])
	if err != nil {
		t.Fatal(err)
	}
	producerAttempt := r.Attempts[producerTask.AttemptID]
	producerInvocation := r.Invocations[r.Activations[producerAttempt.ActivationID].InvocationID]
	if producerInvocation.BranchID != "producer" || producerAttempt.Session.HostState != SessionAwaiting {
		t.Fatalf("wrong producer handoff: branch=%s attempt=%+v", producerInvocation.BranchID, producerAttempt)
	}
	var registration *WaitRegistration
	for _, candidate := range r.Waits {
		if r.Invocations[candidate.InvocationID].BranchID == "consumer" {
			registration = candidate
		}
	}
	if registration == nil || registration.Status != "active" {
		t.Fatalf("consumer did not establish its durable wait before producer effect: %+v", registration)
	}
	beforePublication, err := e.View(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.DeliverEvent(ctx, DeliverEventRequest{RunID: runID, RegistrationID: registration.ID, EventID: "event:spoof", EventType: "artifact.published", Nonce: registration.Nonce, Generation: registration.Generation, Payload: []byte(`{"value":0}`)})
	rejectionCode(t, err, "managed_source")

	data := []byte(`{"value":1}`)
	slot := producerTask.Context.Outputs["document"]
	if err := os.WriteFile(filepath.Join(producerTask.Workspace, slot.Path), data, 0600); err != nil {
		t.Fatal(err)
	}
	size := int64(len(data))
	command := PublishCommand{
		SchemaVersion: "2", CommandID: "command:session-artifact", RunID: runID,
		StepID: producerTask.StepInstanceID, AttemptID: producerTask.AttemptID, EnvelopeDigest: producerTask.EnvelopeDigest,
		Hook: "document_created", Kind: "artifact", ItemKey: "item", CandidatePath: slot.Path, ExpectedDigest: rawDigest(data), ExpectedSizeBytes: &size,
	}
	first, err := e.PublishSessionArtifact(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	r = driverRun(t, e, runID)
	if len(r.ArtifactPublications) != 1 || r.Attempts[producerTask.AttemptID].Session.HostState != SessionAwaiting || r.Attempts[producerTask.AttemptID].Settled != nil {
		t.Fatalf("publication settled or lost its still-running producer: %+v", r)
	}
	publication := r.ArtifactPublications[0]
	wait := r.Activations[registration.ActivationID]
	if registration = r.Waits[registration.ID]; registration.Status != "consumed" || wait.Wait.EventRef == nil || *wait.Wait.EventRef != publication.Artifact {
		t.Fatalf("wait did not receive the exact publication: registration=%+v wait=%+v publication=%+v", registration, wait, publication)
	}
	afterPublication, err := e.View(ctx, runID)
	if err != nil || afterPublication.RunVersion != beforePublication.RunVersion+1 {
		t.Fatalf("delivery did not invalidate stale workflow CAS: before=%d after=%d err=%v", beforePublication.RunVersion, afterPublication.RunVersion, err)
	}
	if err := os.WriteFile(filepath.Join(producerTask.Workspace, slot.Path), []byte(`{"value":2}`), 0600); err != nil {
		t.Fatal(err)
	}
	retry, err := e.PublishSessionArtifact(ctx, command)
	if err != nil || !bytes.Equal(first.Receipt.Result, retry.Receipt.Result) {
		t.Fatalf("exact session retry reread mutable producer bytes: %v", err)
	}
	if err := os.WriteFile(filepath.Join(producerTask.Workspace, slot.Path), data, 0600); err != nil {
		t.Fatal(err)
	}

	if err := e.Drive(ctx, runID); err != nil {
		t.Fatal(err)
	}
	tasks, err := e.SessionTasks(ctx, runID)
	if err != nil || len(tasks) != 2 {
		t.Fatalf("producer and consumer do not hold tasks at once: %d %v", len(tasks), err)
	}
	var consumerTask SessionTask
	snapshot := driverRun(t, e, runID)
	for _, task := range tasks {
		attempt := snapshot.Attempts[task.AttemptID]
		invocation := snapshot.Invocations[snapshot.Activations[attempt.ActivationID].InvocationID]
		if invocation.BranchID == "consumer" {
			consumerTask = task
		}
	}
	if consumerTask.AttemptID == "" || consumerTask.Context.Inputs["document"].Ref != publication.Artifact {
		t.Fatalf("consumer admission did not pin the published revision: %+v", consumerTask)
	}
	consumerInput, err := os.ReadFile(filepath.Join(consumerTask.Workspace, consumerTask.Context.Inputs["document"].Path))
	if err != nil || !bytes.Equal(consumerInput, data) {
		t.Fatalf("consumer read mutable or wrong bytes: %q %v", consumerInput, err)
	}

	if _, err := e.SubmitSession(ctx, sessionPass(t, consumerTask, map[string]ArtifactRef{})); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatal(err)
	}
	producerOutput := ArtifactRef{ArtifactID: slot.ArtifactID, Revision: slot.Revision, Digest: rawDigest(data)}
	if _, err := e.SubmitSession(ctx, sessionPass(t, producerTask, map[string]ArtifactRef{"document": producerOutput})); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatal(err)
	}
	settled := driverRun(t, e, runID)
	if settled.Status != "completed" || settled.Outcome == nil || *settled.Outcome != "succeeded" || len(settled.ArtifactPublications) != 1 {
		t.Fatalf("overlap workflow did not settle normally: status=%s outcome=%v", settled.Status, settled.Outcome)
	}
}

func TestOnceBlobPublicationDeliversSealedBytesBeforeProducerSettlement(t *testing.T) {
	e, runID := publicationSubscriptionRuntimeSourceFormat(t, "retained", "wait_until_timeout", "blob")
	ctx := context.Background()
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	producer, err := e.SessionTask(ctx, runID, r.Active[0])
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("sealed blob\n")
	slot := producer.Context.Outputs["document"]
	if err := os.WriteFile(filepath.Join(producer.Workspace, slot.Path), data, 0600); err != nil {
		t.Fatal(err)
	}
	size := int64(len(data))
	if _, err := e.PublishSessionArtifact(ctx, PublishCommand{SchemaVersion: "2", CommandID: "command:blob-once", RunID: runID, StepID: producer.StepInstanceID, AttemptID: producer.AttemptID, EnvelopeDigest: producer.EnvelopeDigest, Hook: "document_created", Kind: "artifact", ItemKey: "item", CandidatePath: slot.Path, ExpectedDigest: rawDigest(data), ExpectedSizeBytes: &size}); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatal(err)
	}
	r = driverRun(t, e, runID)
	if len(r.ArtifactPublications) != 1 || r.ArtifactPublications[0].Format != "blob" || r.ArtifactPublications[0].MediaType != "text/plain" || r.Attempts[producer.AttemptID].Settled != nil {
		t.Fatalf("blob was not published while producer remained live: %+v", r.ArtifactPublications)
	}
	if artifact, actual, err := e.Artifact(r.ArtifactPublications[0].Artifact); err != nil || artifact.Format != "blob" || !bytes.Equal(actual, data) {
		t.Fatalf("published blob is not independently readable: artifact=%+v data=%q err=%v", artifact, actual, err)
	}
	tasks, err := e.SessionTasks(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range tasks {
		if task.AttemptID == producer.AttemptID {
			continue
		}
		input := task.Context.Inputs["document"]
		artifact, actual, err := e.Artifact(input.Ref)
		if err != nil || artifact.Format != "blob" || artifact.MediaType != "text/plain" || !bytes.Equal(actual, data) {
			t.Fatalf("consumer did not receive the sealed blob: artifact=%+v data=%q err=%v", artifact, actual, err)
		}
		return
	}
	t.Fatalf("blob consumer was not admitted: active=%v diagnostics=%+v", r.Active, r.Diagnostics)
}

func TestOncePublicationConsumesAReservedDeliveryAtWaitEntry(t *testing.T) {
	e, runID := publicationSubscriptionRuntime(t)
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	root, err := r.plan()
	if err != nil {
		t.Fatal(err)
	}
	var registration *WaitRegistration
	for _, candidate := range r.Waits {
		if r.Invocations[candidate.InvocationID].BranchID == "consumer" {
			registration = candidate
		}
	}
	if registration == nil {
		t.Fatal("consumer wait registration is absent")
	}
	wait := r.Activations[registration.ActivationID]
	producerID := r.Active[0]
	producer := r.Attempts[producerID]
	producerPlan, err := r.planFor(r.Activations[producer.ActivationID].InvocationID)
	if err != nil {
		t.Fatal(err)
	}
	hook := producerPlan.Steps["produce"].Hooks["document_created"]
	publication := ArtifactPublication{
		SchemaVersion: ArtifactPublicationVersion, ID: "publication:reserved", AttemptID: producerID,
		StepID: producer.StepID, Hook: "document_created", ItemKey: "item",
		Artifact: ArtifactRef{ArtifactID: "artifact:reserved", Revision: 1, Digest: rawDigest([]byte(`{"value":1}`))},
		Format:   "json", SchemaRef: hook.SchemaRef, Consumption: "early",
	}
	registration.Status = "reserved"
	wait.Status, wait.Settled = "ready", nil
	wait.Wait.Resolution, wait.Wait.EventRef = "", nil
	r.Invocations[wait.InvocationID].Status = "ready"
	firstEvents, assigned, err := assignPublicationToWait(&r, root, registration, publication, e.clock.now())
	if err != nil || !assigned || len(firstEvents) != 1 || r.Inbox[0].Disposition != "held" {
		t.Fatalf("reserved publication was not held once: events=%d assigned=%v inbox=%+v err=%v", len(firstEvents), assigned, r.Inbox, err)
	}
	registration.Status, wait.Status = "active", "waiting"
	r.Invocations[wait.InvocationID].Status = "waiting"
	secondEvents, assigned, err := assignPublicationToWait(&r, root, registration, publication, e.clock.now())
	if err != nil || !assigned || len(secondEvents) != 1 || secondEvents[0].Type != "stage.wait_resolved" || len(r.Inbox) != 1 || r.Inbox[0].Disposition != "consumed" || registration.Status != "consumed" || wait.Status != "completed" {
		t.Fatalf("held publication was not consumed atomically at entry: events=%+v assigned=%v registration=%+v wait=%+v inbox=%+v err=%v", secondEvents, assigned, registration, wait, r.Inbox, err)
	}
}

func TestNewOnlyOncePublicationRejectsItemsBeforeItsAuthorityCut(t *testing.T) {
	e, runID := publicationSubscriptionRuntimeInitial(t, "new_only")
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	if r.SchemaVersion != CoreActionDeliveryStateVersion {
		t.Fatalf("new-only source did not select the current session state: %s", r.SchemaVersion)
	}
	root, err := r.plan()
	if err != nil {
		t.Fatal(err)
	}
	var registration *WaitRegistration
	for _, candidate := range r.Waits {
		if r.Invocations[candidate.InvocationID].BranchID == "consumer" {
			registration = candidate
		}
	}
	if registration == nil || registration.PublicationStartSequence == nil {
		t.Fatalf("new-only once source did not persist its authority cut: %+v", registration)
	}
	producer := r.Attempts[r.Active[0]]
	producerPlan, err := r.planFor(r.Activations[producer.ActivationID].InvocationID)
	if err != nil {
		t.Fatal(err)
	}
	hook := producerPlan.Steps["produce"].Hooks["document_created"]
	publication := ArtifactPublication{StepID: producer.StepID, Hook: "document_created", ItemKey: "item", Format: "json", SchemaRef: hook.SchemaRef, Consumption: "early"}
	publication.AcceptedSequence = *registration.PublicationStartSequence
	if _, _, matched, err := publicationWait(&r, root, registration, publication); err != nil || matched {
		t.Fatalf("new-only once source accepted a publication at or before its cut: matched=%v err=%v", matched, err)
	}
	publication.AcceptedSequence++
	if _, _, matched, err := publicationWait(&r, root, registration, publication); err != nil || !matched {
		t.Fatalf("new-only once source refused its first later publication: matched=%v err=%v", matched, err)
	}
	if err := validatePublic(t, "CoreRunStateV21", r); err != nil {
		t.Fatalf("current session state rejects the persisted once cut: %v", err)
	}
}
