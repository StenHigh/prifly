package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/stenhigh/prifly/internal/flow"
)

func publicationStreamRuntime(t *testing.T, timeout int64) (*Engine, StartOptions) {
	return publicationStreamRuntimeInitial(t, timeout, "retained")
}

func publicationStreamRuntimeInitial(t *testing.T, timeout int64, initial string) (*Engine, StartOptions) {
	return publicationStreamRuntimeSource(t, timeout, initial, "wait_until_timeout")
}

func publicationStreamRuntimeSource(t *testing.T, timeout int64, initial, producerFailure string) (*Engine, StartOptions) {
	return publicationStreamRuntimeSourceFormat(t, timeout, initial, producerFailure, "json")
}

func publicationStreamRuntimeSourceFormat(t *testing.T, timeout int64, initial, producerFailure, format string) (*Engine, StartOptions) {
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
	handleRef := builtinRef(definitions, publicationHandleSchemaID)
	cursorRef := builtinRef(definitions, publicationCursorSchemaID)
	deliveryRef := builtinRef(definitions, publicationDeliverySchemaID)

	producerSkill := []byte("---\nname: publish-documents\n---\n\nPublish a finite document stream.\n")
	consumerSkill := []byte("---\nname: consume-document\n---\n\nConsume one sealed document.\n")
	if err := os.WriteFile(filepath.Join(e.Root, "resources/producer.md"), producerSkill, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.Root, "resources/consumer.md"), consumerSkill, 0600); err != nil {
		t.Fatal(err)
	}
	producerSkillRef := flow.Ref{ID: "test:context/stream-producer", Version: "1.0.0", Digest: rawDigest(producerSkill)}
	consumerSkillRef := flow.Ref{ID: "test:context/stream-consumer", Version: "1.0.0", Digest: rawDigest(consumerSkill)}

	producerStep := flow.StepDefinition{
		SchemaVersion: "4", ID: "test:step/stream-producer", Version: "1.0.0", Title: "Publish documents", Kind: "worker",
		Inputs: map[string]flow.InputPort{}, Outputs: map[string]flow.OutputPort{
			"document": {Port: flow.Port{Format: "json", SchemaRef: &documentRef}, RequiredFor: []string{"pass"}},
		},
		InstructionsRef: &producerSkillRef, ContextRefs: []flow.Ref{}, RequiredCapabilities: []string{},
		ResultCheckRefs: []flow.Ref{}, ResultSchemaRef: builtinRef(definitions, "core:schema/step-result"),
		Hooks: map[string]flow.Hook{"document_created": {
			Kind: "artifact", SchemaRef: documentRef, Description: "Sealed documents", Classification: "internal", ReadPolicy: "declared_subscribers",
			MaxPayloadBytes: 4096, MaxCount: 3, MaxPerMinute: 3,
			Artifact: &flow.ArtifactHook{Format: format, MediaTypes: artifactMediaTypes, Cardinality: "keyed_many", ContentCheckRefs: []flow.Ref{}, EarlyConsumption: true},
		}},
	}
	producerStep.Executor.AdapterRef, producerStep.Executor.Operation = builtinRef(definitions, "core:adapter/assisted-session"), "session"
	producerStep.Effects.Class, producerStep.Effects.RetryClass = "none", "never"
	producerStepBytes := writeRegistryDocument(t, e, "steps/stream-producer.json", producerStep)
	producerStepRef := flow.Ref{ID: producerStep.ID, Version: producerStep.Version, Digest: rawDigest(producerStepBytes)}

	consumerStep := flow.StepDefinition{
		SchemaVersion: "2", ID: "test:step/stream-consumer", Version: "1.0.0", Title: "Consume document", Kind: "worker",
		Inputs: map[string]flow.InputPort{"document": {Port: artifactPort, Required: true}}, Outputs: map[string]flow.OutputPort{},
		InstructionsRef: &consumerSkillRef, ContextRefs: []flow.Ref{}, RequiredCapabilities: []string{},
		ResultCheckRefs: []flow.Ref{}, ResultSchemaRef: builtinRef(definitions, "core:schema/step-result"),
	}
	consumerStep.Executor.AdapterRef, consumerStep.Executor.Operation = builtinRef(definitions, "core:adapter/assisted-session"), "session"
	consumerStep.Effects.Class, consumerStep.Effects.RetryClass = "none", "never"
	consumerStepBytes := writeRegistryDocument(t, e, "steps/stream-consumer.json", consumerStep)
	consumerStepRef := flow.Ref{ID: consumerStep.ID, Version: consumerStep.Version, Digest: rawDigest(consumerStepBytes)}

	producer := flow.WorkflowRevision{
		SchemaVersion: "1", ID: "test:workflow/stream-producer", Version: "1.0.0", Title: "Stream producer",
		Inputs: map[string]flow.InputPort{}, Outputs: map[string]flow.OutputPort{}, AllowedOutcomes: []string{"succeeded"},
		Limits: flow.Limits{MaxStepInstances: 2, MaxControlTransitions: 16, MaxParallelism: 1}, PolicyRef: builtinVersionRef(definitions, "core:policy/local", "2.0.0"),
	}
	producer.Definition.Entry = "produce"
	producer.Definition.Stages = map[string]flow.Stage{
		"produce": {Kind: "step", StepRef: producerStepRef, InputBindings: map[string]flow.Binding{}, On: map[string]string{"pass": "done"}},
		"done":    {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]flow.Binding{}},
	}
	producerBytes := writeRegistryDocument(t, e, "workflows/stream-producer.json", producer)
	producerRef := flow.Ref{ID: producer.ID, Version: producer.Version, Digest: rawDigest(producerBytes)}

	worker := flow.WorkflowRevision{
		SchemaVersion: "1", ID: "test:workflow/stream-worker", Version: "1.0.0", Title: "One document consumer",
		Inputs: map[string]flow.InputPort{"document": {Port: artifactPort, Required: true}}, Outputs: map[string]flow.OutputPort{}, AllowedOutcomes: []string{"succeeded"},
		Limits: flow.Limits{MaxStepInstances: 1, MaxControlTransitions: 12, MaxParallelism: 1}, PolicyRef: builtinVersionRef(definitions, "core:policy/local", "2.0.0"),
	}
	worker.Definition.Entry = "consume"
	worker.Definition.Stages = map[string]flow.Stage{
		"consume": {Kind: "step", StepRef: consumerStepRef, InputBindings: map[string]flow.Binding{"document": {From: "workflow_input", Port: "document"}}, On: map[string]string{"pass": "done"}},
		"done":    {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]flow.Binding{}},
	}
	workerBytes := writeRegistryDocument(t, e, "workflows/stream-worker.json", worker)
	workerRef := flow.Ref{ID: worker.ID, Version: worker.Version, Digest: rawDigest(workerBytes)}

	sourceVersion := flow.PublicationStreamSourceVersion
	if format == "blob" {
		sourceVersion = flow.PublicationBlobStreamSourceVersion
	} else if producerFailure == "interrupt_on_terminal_failure" {
		sourceVersion = flow.PublicationFailureStreamSourceVersion
	} else if initial == "new_only" {
		sourceVersion = flow.PublicationNewOnlyStreamSourceVersion
	}
	source := flow.PublicationSourceDefinition{
		SchemaVersion: sourceVersion, ID: "test:source/documents", Version: "1.0.0", Mode: "each_publication",
		ProducerBranchID: "producer", ProducerStageID: "produce", Hook: "document_created", HookSchemaRef: documentRef,
		HandleSchemaRef: &handleRef, CursorSchemaRef: &cursorRef, DeliverySchemaRef: &deliveryRef,
		Initial: initial, ProducerFailure: producerFailure,
	}
	if format == "blob" {
		source.Format, source.MediaTypes = format, artifactMediaTypes
	}
	sourceBytes := writeRegistryDocument(t, e, "sources/documents.json", source)
	sourceRef := flow.Ref{ID: source.ID, Version: source.Version, Digest: rawDigest(sourceBytes)}
	nextCursor := "/next_cursor"
	deliveryKind := func(kind string) flow.Predicate {
		return flow.Predicate{Op: "eq",
			Left:  &flow.Operand{Kind: "field", Ref: &flow.FieldRef{From: "stage_output", StageID: "await_document", Port: flow.WaitEventPort, Pointer: "/kind"}},
			Right: &flow.Operand{Kind: "literal", Value: json.RawMessage(`"` + kind + `"`)},
		}
	}
	body := flow.WorkflowRevision{
		SchemaVersion: "3", ID: "test:workflow/publication-body", Version: "1.0.0", Title: "One publication delivery",
		Inputs: map[string]flow.InputPort{
			"subscription": {Port: flow.Port{Format: "json", SchemaRef: &handleRef}, Required: true},
			"cursor":       {Port: flow.Port{Format: "json", SchemaRef: &cursorRef}, Required: true},
		},
		Outputs: map[string]flow.OutputPort{
			"next_cursor": {Port: flow.Port{Format: "json", SchemaRef: &cursorRef}, RequiredFor: []string{"succeeded"}},
		},
		AllowedOutcomes: []string{"succeeded", "no_work", "rejected"},
		Limits:          flow.Limits{MaxStepInstances: 1, MaxControlTransitions: 32, MaxParallelism: 1, MaxChildDepth: 1},
		PolicyRef:       builtinVersionRef(definitions, "core:policy/local", "2.0.0"),
	}
	body.Definition.Entry = "await_document"
	body.Definition.Stages = map[string]flow.Stage{
		"await_document": {
			Kind: "wait", SourceRef: sourceRef, EventType: "artifact.publication", EventSchemaRef: deliveryRef,
			CorrelationInput: &flow.Binding{From: "workflow_input", Port: "subscription"}, CursorInput: &flow.Binding{From: "workflow_input", Port: "cursor"},
			TimeoutSeconds: &timeout, OnEvent: "delivery", OnTimeout: "delivery",
		},
		"delivery": {
			Kind: "choice", Selection: "first_match",
			Branches: []flow.ChoiceBranch{
				{ID: "item", Predicate: deliveryKind("Item"), Next: "consume"},
				{ID: "closed", Predicate: deliveryKind("Closed"), Next: "closed"},
			},
			Default: "interrupted",
		},
		"consume": {
			Kind: "call", WorkflowRef: workerRef,
			InputBindings: map[string]flow.Binding{"document": {From: "publication", StageID: "await_document"}},
			On:            map[string]string{"succeeded": "item_done"}, OnError: "interrupted",
		},
		"item_done": {
			Kind: "finish", Outcome: "succeeded",
			OutputBindings: map[string]flow.Binding{"next_cursor": {
				From: "stage_output", StageID: "await_document", Port: flow.WaitEventPort,
				Pointer: &nextCursor, ProjectedSchemaRef: &cursorRef,
			}},
		},
		"closed":      {Kind: "finish", Outcome: "no_work", OutputBindings: map[string]flow.Binding{}},
		"interrupted": {Kind: "finish", Outcome: "rejected", OutputBindings: map[string]flow.Binding{}},
	}
	bodyBytes := writeRegistryDocument(t, e, "workflows/publication-body.json", body)
	bodyRef := flow.Ref{ID: body.ID, Version: body.Version, Digest: rawDigest(bodyBytes)}

	never := flow.Predicate{Op: "eq", Left: &flow.Operand{Kind: "literal", Value: json.RawMessage("false")}, Right: &flow.Operand{Kind: "literal", Value: json.RawMessage("true")}}
	subscriber := flow.WorkflowRevision{
		SchemaVersion: "3", ID: "test:workflow/publication-subscriber", Version: "1.0.0", Title: "Finite publication subscriber",
		Inputs: map[string]flow.InputPort{}, Outputs: map[string]flow.OutputPort{}, AllowedOutcomes: []string{"succeeded", "rejected"},
		Limits: flow.Limits{MaxStepInstances: 4, MaxControlTransitions: 160, MaxParallelism: 1, MaxChildDepth: 2}, PolicyRef: builtinVersionRef(definitions, "core:policy/local", "2.0.0"),
	}
	subscriber.Definition.Entry = "publications"
	subscriber.Definition.Stages = map[string]flow.Stage{
		"publications": {
			Kind: "repeat", BodyWorkflowRef: bodyRef,
			InitialBindings: map[string]flow.Binding{
				"subscription": {From: "subscription", SourceRef: &sourceRef, Port: "handle"},
				"cursor":       {From: "subscription", SourceRef: &sourceRef, Port: "cursor"},
			},
			NextBindings: map[string]flow.Binding{
				"subscription": {From: "subscription", SourceRef: &sourceRef, Port: "handle"},
				"cursor":       {From: "iteration_output", Port: "next_cursor"},
			},
			ContinueOn: []string{"succeeded"}, Until: never, MaxIterations: 4,
			OnComplete: map[string]string{"succeeded": "done", "no_work": "done", "rejected": "failed"},
			OnLimit:    "failed", OnUnknown: "failed", OnError: "failed",
		},
		"done":   {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]flow.Binding{}},
		"failed": {Kind: "finish", Outcome: "rejected", OutputBindings: map[string]flow.Binding{}},
	}
	subscriberBytes := writeRegistryDocument(t, e, "workflows/publication-subscriber.json", subscriber)
	subscriberRef := flow.Ref{ID: subscriber.ID, Version: subscriber.Version, Digest: rawDigest(subscriberBytes)}

	parent := flow.WorkflowRevision{
		SchemaVersion: "3", ID: "test:workflow/publication-stream", Version: "1.0.0", Title: "Producer with two independent subscribers",
		Inputs: map[string]flow.InputPort{}, Outputs: map[string]flow.OutputPort{}, AllowedOutcomes: []string{"succeeded", "rejected"},
		Limits: flow.Limits{MaxStepInstances: 12, MaxControlTransitions: 512, MaxParallelism: 3, MaxChildDepth: 3}, PolicyRef: builtinVersionRef(definitions, "core:policy/local", "3.0.0"),
	}
	parent.Definition.Entry = "fan"
	parent.Definition.Stages = map[string]flow.Stage{
		"fan": {
			Kind: "parallel", MaxParallelism: 3,
			ParallelBranches: []flow.ParallelBranch{
				{ID: "producer", WorkflowRef: producerRef, InputBindings: map[string]flow.Binding{}},
				{ID: "subscriber_b", WorkflowRef: subscriberRef, InputBindings: map[string]flow.Binding{}},
				{ID: "subscriber_c", WorkflowRef: subscriberRef, InputBindings: map[string]flow.Binding{}},
			},
			Join: &flow.Join{Mode: "all", AcceptOutcomes: []string{"succeeded"}, Selection: "all", Remainder: "wait"},
			On:   map[string]string{"satisfied": "done", "unsatisfied": "failed"},
		},
		"done":   {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]flow.Binding{}},
		"failed": {Kind: "finish", Outcome: "rejected", OutputBindings: map[string]flow.Binding{}},
	}
	writeRuntimeJSON(t, filepath.Join(e.Root, "workflows/publication-stream.json"), parent)

	registry := RegistryFile{SchemaVersion: "3", Entries: []Definition{
		{Ref: documentRef, Kind: "schema", Path: "schemas/document.json"},
		{Ref: producerSkillRef, Kind: "resource", Path: "resources/producer.md", ByteEncoding: "utf8_text", MediaType: "text/markdown; charset=utf-8"},
		{Ref: consumerSkillRef, Kind: "resource", Path: "resources/consumer.md", ByteEncoding: "utf8_text", MediaType: "text/markdown; charset=utf-8"},
		{Ref: producerStepRef, Kind: "step", Path: "steps/stream-producer.json"},
		{Ref: consumerStepRef, Kind: "step", Path: "steps/stream-consumer.json"},
		{Ref: producerRef, Kind: "workflow", Path: "workflows/stream-producer.json"},
		{Ref: workerRef, Kind: "workflow", Path: "workflows/stream-worker.json"},
		{Ref: bodyRef, Kind: "workflow", Path: "workflows/publication-body.json"},
		{Ref: subscriberRef, Kind: "workflow", Path: "workflows/publication-subscriber.json"},
		{Ref: sourceRef, Kind: "resource", Path: "sources/documents.json"},
	}}
	writeRuntimeJSON(t, filepath.Join(e.Root, e.Config.Configuration.RegistryFile), registry)
	e.Config.Configuration.SchemaVersion = CoreContextConfigVersion
	e.Config.ConfigurationSchemaRef = builtinVersionRef(definitions, "core:schema/core-configuration", "2.0.0")
	e.Config.AdapterBindings["local_process"] = builtinVersionRef(definitions, "core:adapter/local-process", "2.0.0")
	e.Config.DefaultPolicyRef = builtinVersionRef(definitions, "core:policy/local", "2.0.0")
	writeRuntimeJSON(t, filepath.Join(e.Root, "prifly.json"), e.Config)
	writeRuntimeJSON(t, filepath.Join(e.Root, "brief.json"), Brief{"1", "test:brief/publication-stream", "Consume every early document", "Two subscribers finish only after close", []string{"Every sealed document"}, []string{"External writes"}, []string{"Independent finite delivery"}, []ArtifactRef{}, []string{}, "explicit"})
	if _, err := e.SetAdmissionCapacity(context.Background(), CapacityRequest{CommandID: newID("command"), Capacity: 3, Reason: "qualify producer and two subscribers"}); err != nil {
		t.Fatal(err)
	}
	return e, StartOptions{CommandID: newID("command"), WorkflowFile: "workflows/publication-stream.json", BriefFile: "brief.json", Inputs: map[string]string{}}
}

func sessionTaskBranch(r Run, task SessionTask) string {
	attempt := r.Attempts[task.AttemptID]
	if attempt == nil {
		return ""
	}
	invocation := r.Invocations[r.Activations[attempt.ActivationID].InvocationID]
	for invocation != nil {
		if invocation.BranchID != "" {
			return invocation.BranchID
		}
		invocation = r.Invocations[invocation.ParentInvocationID]
	}
	return ""
}

func streamTasks(t *testing.T, e *Engine, runID string) map[string]SessionTask {
	t.Helper()
	tasks, err := e.SessionTasks(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	result := map[string]SessionTask{}
	for _, task := range tasks {
		result[sessionTaskBranch(r, task)] = task
	}
	return result
}

func TestEachPublicationKeepsIndependentCursorsAndPendingAssignments(t *testing.T) {
	e, options := publicationStreamRuntime(t, 60)
	ctx := context.Background()
	started, err := e.Start(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	runID := started.Receipt.RunID
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatal(err)
	}
	tasks := streamTasks(t, e, runID)
	producer := tasks["producer"]
	if len(tasks) != 1 || producer.AttemptID == "" {
		t.Fatalf("stream did not start with one held producer: %+v", tasks)
	}
	r := driverRun(t, e, runID)
	if r.SchemaVersion != CoreActionDeliveryStateVersion || len(r.PublicationSubscriptions) != 2 {
		t.Fatalf("two durable subscribers were not created: state=%s subscriptions=%+v", r.SchemaVersion, r.PublicationSubscriptions)
	}
	for _, subscription := range r.PublicationSubscriptions {
		if subscription.Cursor != 0 || subscription.PendingAssignmentID != "" || subscription.Status != "open" {
			t.Fatalf("subscriber did not begin at retained cursor zero: %+v", subscription)
		}
	}

	slot := producer.Context.Outputs["document"]
	publications := []ArtifactPublication{}
	for index, data := range [][]byte{[]byte(`{"value":1}`), []byte(`{"value":2}`)} {
		ordinal := strconv.Itoa(index + 1)
		if err := os.WriteFile(filepath.Join(producer.Workspace, slot.Path), data, 0600); err != nil {
			t.Fatal(err)
		}
		size := int64(len(data))
		command := PublishCommand{
			SchemaVersion: "3", CommandID: "command:stream-item-" + ordinal, RunID: runID,
			StepID: producer.StepInstanceID, AttemptID: producer.AttemptID, EnvelopeDigest: producer.EnvelopeDigest,
			Hook: "document_created", Kind: "artifact", ItemKey: "document-" + ordinal, CandidatePath: slot.Path,
			ExpectedDigest: rawDigest(data), ExpectedSizeBytes: &size,
		}
		first, err := e.PublishSessionPublication(ctx, command)
		if err != nil {
			t.Fatal(err)
		}
		r = driverRun(t, e, runID)
		publications = append(publications, r.ArtifactPublications[index])
		if index == 1 {
			closeCommand := PublishCommand{
				SchemaVersion: "3", CommandID: "command:stream-close", RunID: runID,
				StepID: producer.StepInstanceID, AttemptID: producer.AttemptID, EnvelopeDigest: producer.EnvelopeDigest,
				Hook: "document_created", Kind: "close", ItemKeys: []string{"document-1", "document-2"},
			}
			closed, err := e.PublishSessionPublication(ctx, closeCommand)
			if err != nil {
				t.Fatal(err)
			}
			r = driverRun(t, e, runID)
			for _, subscription := range r.PublicationSubscriptions {
				if subscription.Cursor != 1 || subscription.PendingAssignmentID == "" {
					t.Fatalf("close replaced or advanced a pending second item: %+v", subscription)
				}
			}
			if err := os.WriteFile(filepath.Join(producer.Workspace, slot.Path), []byte(`{"value":999}`), 0600); err != nil {
				t.Fatal(err)
			}
			retry, err := e.PublishSessionPublication(ctx, command)
			if err != nil || !bytes.Equal(first.Receipt.Result, retry.Receipt.Result) {
				t.Fatalf("item retry reread mutable bytes or changed receipt: %v", err)
			}
			reclosed, err := e.PublishSessionPublication(ctx, closeCommand)
			if err != nil || !bytes.Equal(closed.Receipt.Result, reclosed.Receipt.Result) {
				t.Fatalf("close retry changed its exact result: %v", err)
			}
			if err := os.WriteFile(filepath.Join(producer.Workspace, slot.Path), data, 0600); err != nil {
				t.Fatal(err)
			}
		}
		if err := e.Drive(ctx, runID); err != nil {
			t.Fatal(err)
		}
		tasks = streamTasks(t, e, runID)
		if len(tasks) != 3 || tasks["subscriber_b"].AttemptID == "" || tasks["subscriber_c"].AttemptID == "" {
			t.Fatalf("both subscribers were not admitted beside the live producer: %+v", tasks)
		}
		for _, branch := range []string{"subscriber_b", "subscriber_c"} {
			task := tasks[branch]
			if task.Context.Inputs["document"].Ref != publications[index].Artifact {
				t.Fatalf("%s received a wrapper or the wrong item: %+v", branch, task.Context.Inputs["document"])
			}
			if _, err := e.SubmitSession(ctx, sessionPass(t, task, map[string]ArtifactRef{})); err != nil {
				t.Fatal(err)
			}
		}
		if err := e.Drive(ctx, runID); err != nil {
			t.Fatal(err)
		}
	}

	r = driverRun(t, e, runID)
	if len(r.PublicationAssignments) != 6 {
		t.Fatalf("two subscribers did not receive two items and one closure each: %+v", r.PublicationAssignments)
	}
	perSubscription := map[string][]PublicationAssignment{}
	for _, assignment := range r.PublicationAssignments {
		perSubscription[assignment.SubscriptionID] = append(perSubscription[assignment.SubscriptionID], assignment)
	}
	for id, assignments := range perSubscription {
		sort.Slice(assignments, func(i, j int) bool { return assignments[i].Cursor < assignments[j].Cursor })
		if len(assignments) != 3 || assignments[0].Kind != "Item" || assignments[1].Kind != "Item" || assignments[2].Kind != "Closed" {
			t.Fatalf("subscriber %s received a wrong delivery sequence: %+v", id, assignments)
		}
		for cursor, assignment := range assignments {
			if assignment.Cursor != int64(cursor) || assignment.NextCursor != int64(cursor+1) || assignment.Status != "processed" || assignment.Processed == nil {
				t.Fatalf("subscriber %s lost a durable cursor transition: %+v", id, assignment)
			}
		}
		subscription := r.PublicationSubscriptions[id]
		if subscription.Cursor != 3 || subscription.Status != "closed" || subscription.PendingAssignmentID != "" {
			t.Fatalf("subscriber %s did not settle independently at closure: %+v", id, subscription)
		}
	}
	tasks = streamTasks(t, e, runID)
	if len(tasks) != 1 || tasks["producer"].AttemptID != producer.AttemptID {
		t.Fatalf("closure invoked a consumer or settled the producer: %+v", tasks)
	}
	view, err := e.View(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	next, err := e.Next(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := e.Preview(PreviewOptions{WorkflowFile: options.WorkflowFile, BriefFile: options.BriefFile})
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]any{
		"CoreRunStateV21": r, "CoreRunViewV21": view, "CoreNextViewV21": next, "CorePreviewV21": preview,
		"CoreWorkflowInvocationV21": r.Invocations[r.RootInvocationID], "CoreCapabilitiesV21": Capabilities(),
	} {
		if err := validatePublic(t, name, value); err != nil {
			t.Fatalf("%s rejects the live stream value: %v", name, err)
		}
	}
	for _, subscription := range r.PublicationSubscriptions {
		if err := validatePublic(t, "PublicationSubscription", subscription); err != nil {
			t.Fatalf("public contract rejects a live subscription: %v", err)
		}
	}
	for _, assignment := range r.PublicationAssignments {
		if err := validatePublic(t, "PublicationAssignment", assignment); err != nil {
			t.Fatalf("public contract rejects a live assignment: %v", err)
		}
		_, data, err := e.Artifact(assignment.Delivery)
		if err != nil {
			t.Fatal(err)
		}
		var delivery PublicationDelivery
		if err := json.Unmarshal(data, &delivery); err != nil {
			t.Fatal(err)
		}
		if err := validatePublic(t, "PublicationDelivery", delivery); err != nil {
			t.Fatalf("public contract rejects a live delivery: %v", err)
		}
	}
	transportSeen := map[string]bool{}
	for _, invocation := range r.Invocations {
		if invocation.Iteration == nil {
			continue
		}
		for _, artifactRef := range invocation.Inputs {
			artifact, data, err := e.Artifact(artifactRef)
			if err != nil {
				t.Fatal(err)
			}
			if artifact.SchemaRef == nil {
				continue
			}
			switch artifact.SchemaRef.ID {
			case publicationHandleSchemaID:
				var handle PublicationSubscriptionHandle
				if json.Unmarshal(data, &handle) != nil || validatePublic(t, "PublicationSubscriptionHandle", handle) != nil {
					t.Fatal("public contract rejects a live subscription handle")
				}
				transportSeen["handle"] = true
			case publicationCursorSchemaID:
				var cursor PublicationCursor
				if json.Unmarshal(data, &cursor) != nil || validatePublic(t, "PublicationCursor", cursor) != nil {
					t.Fatal("public contract rejects a live publication cursor")
				}
				transportSeen["cursor"] = true
			}
		}
	}
	if !transportSeen["handle"] || !transportSeen["cursor"] {
		t.Fatalf("live repeat inputs omitted a transport artifact: %+v", transportSeen)
	}
	finalData := []byte(`{"value":2}`)
	producerOutput := ArtifactRef{ArtifactID: slot.ArtifactID, Revision: slot.Revision, Digest: rawDigest(finalData)}
	if _, err := e.SubmitSession(ctx, sessionPass(t, producer, map[string]ArtifactRef{"document": producerOutput})); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatal(err)
	}
	settled := driverRun(t, e, runID)
	if settled.Status != "completed" || settled.Outcome == nil || *settled.Outcome != "succeeded" {
		t.Fatalf("closed stream did not let the parent settle: status=%s outcome=%v", settled.Status, settled.Outcome)
	}
}

func TestNewOnlyStreamStartsAfterItsAuthorityCut(t *testing.T) {
	e, options := publicationStreamRuntimeInitial(t, 60, "new_only")
	ctx := context.Background()
	started, err := e.Start(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(ctx, started.Receipt.RunID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, started.Receipt.RunID)
	if r.SchemaVersion != CoreActionDeliveryStateVersion || len(r.PublicationSubscriptions) != 2 {
		t.Fatalf("new-only stream did not select the current session state with durable subscriptions: state=%s subscriptions=%+v", r.SchemaVersion, r.PublicationSubscriptions)
	}
	var subscription *PublicationSubscription
	for _, candidate := range r.PublicationSubscriptions {
		if candidate.SchemaVersion != PublicationNewOnlySubscriptionVersion || candidate.PublicationStartSequence == nil {
			t.Fatalf("new-only stream stored the retained subscription shape: %+v", candidate)
		}
		subscription = candidate
	}
	if subscription == nil {
		t.Fatal("new-only stream has no subscription")
	}
	if err := validatePublic(t, "CoreRunStateV21", r); err != nil {
		t.Fatalf("current session state rejects the persisted stream cut: %v", err)
	}
	state, err := canonicalState(r)
	if err != nil {
		t.Fatal(err)
	}
	var legacy map[string]any
	if err := json.Unmarshal(state, &legacy); err != nil {
		t.Fatal(err)
	}
	legacy["schema_version"] = CorePublicationChecksStateVersion
	legacyState, err := canonical(legacy)
	if err != nil {
		t.Fatal(err)
	}
	var rejected Run
	if err := decodeState(legacyState, &rejected); err == nil {
		t.Fatal("older state accepted a new-only authority cut")
	}
	repeat := r.Activations[subscription.RepeatActivationID]
	if repeat == nil || repeat.Repeat == nil || repeat.Repeat.CurrentBodyInvocationID == "" {
		t.Fatalf("new-only stream repeat did not create its first body: %+v", repeat)
	}
	body := r.Invocations[repeat.Repeat.CurrentBodyInvocationID]
	bodyPlan, err := r.planFor(body.ID)
	if err != nil {
		t.Fatal(err)
	}
	stage := bodyPlan.Workflow.Definition.Stages["await_document"]
	source, ok := bodyPlan.PublicationSource(stage.SourceRef)
	if !ok {
		t.Fatal("stream source was not compiled")
	}
	producer, err := streamProducerInvocation(r, subscription, source)
	if err != nil {
		t.Fatal(err)
	}
	producerActivation := r.activationForInvocation(producer.ID, source.ProducerStageID)
	if producerActivation == nil || producerActivation.StepID == "" {
		t.Fatal("live producer identity is absent")
	}
	old := ArtifactPublication{ID: "publication:old", StepID: producerActivation.StepID, Hook: source.Hook, ItemKey: "old", Format: "json", SchemaRef: source.HookSchemaRef, Consumption: "early", AcceptedSequence: *subscription.PublicationStartSequence}
	r.ArtifactPublications = []ArtifactPublication{old}
	if _, available, err := nextStreamDelivery(r, subscription, source); err != nil || available {
		t.Fatalf("new-only stream exposed a publication at its cut: available=%v err=%v", available, err)
	}
	newer := old
	newer.ID, newer.ItemKey, newer.AcceptedSequence = "publication:new", "new", old.AcceptedSequence+1
	r.ArtifactPublications = append(r.ArtifactPublications, newer)
	delivery, available, err := nextStreamDelivery(r, subscription, source)
	if err != nil || !available || delivery.Delivery.Kind != "Item" || delivery.PublicationID != newer.ID {
		t.Fatalf("new-only stream did not select exactly the first later publication: %+v available=%v err=%v", delivery, available, err)
	}
}

func TestEachBlobPublicationDeliversSealedBytesToEverySubscriber(t *testing.T) {
	e, options := publicationStreamRuntimeSourceFormat(t, 60, "retained", "wait_until_timeout", "blob")
	ctx := context.Background()
	started, err := e.Start(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(ctx, started.Receipt.RunID); err != nil {
		t.Fatal(err)
	}
	producer := streamTasks(t, e, started.Receipt.RunID)["producer"]
	data := []byte("streamed sealed blob\n")
	slot := producer.Context.Outputs["document"]
	if err := os.WriteFile(filepath.Join(producer.Workspace, slot.Path), data, 0600); err != nil {
		t.Fatal(err)
	}
	size := int64(len(data))
	if _, err := e.PublishSessionPublication(ctx, PublishCommand{SchemaVersion: "3", CommandID: "command:blob-stream", RunID: started.Receipt.RunID, StepID: producer.StepInstanceID, AttemptID: producer.AttemptID, EnvelopeDigest: producer.EnvelopeDigest, Hook: "document_created", Kind: "artifact", ItemKey: "document-1", CandidatePath: slot.Path, ExpectedDigest: rawDigest(data), ExpectedSizeBytes: &size}); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(ctx, started.Receipt.RunID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, started.Receipt.RunID)
	if len(r.ArtifactPublications) != 1 || r.ArtifactPublications[0].Format != "blob" || r.ArtifactPublications[0].MediaType != "text/plain" {
		t.Fatalf("stream did not publish a typed blob: %+v", r.ArtifactPublications)
	}
	tasks, err := e.SessionTasks(ctx, started.Receipt.RunID)
	if err != nil {
		t.Fatal(err)
	}
	consumers := 0
	for _, task := range tasks {
		if task.AttemptID == producer.AttemptID {
			continue
		}
		consumers++
		input := task.Context.Inputs["document"]
		artifact, actual, err := e.Artifact(input.Ref)
		if err != nil || artifact.Format != "blob" || artifact.MediaType != "text/plain" || !bytes.Equal(actual, data) {
			t.Fatalf("subscriber did not receive the sealed blob: input=%+v artifact=%+v data=%q err=%v active=%v assignments=%+v", input, artifact, actual, err, r.Active, r.PublicationAssignments)
		}
	}
	if consumers != 2 {
		t.Fatalf("expected both stream subscribers, got %d tasks: %+v", consumers, tasks)
	}
}

func TestEachPublicationEmptyCloseSkipsConsumer(t *testing.T) {
	e, options := publicationStreamRuntime(t, 60)
	ctx := context.Background()
	started, err := e.Start(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(ctx, started.Receipt.RunID); err != nil {
		t.Fatal(err)
	}
	producer := streamTasks(t, e, started.Receipt.RunID)["producer"]
	if _, err := e.PublishSessionPublication(ctx, PublishCommand{
		SchemaVersion: "3", CommandID: "command:empty-stream-close", RunID: started.Receipt.RunID,
		StepID: producer.StepInstanceID, AttemptID: producer.AttemptID, EnvelopeDigest: producer.EnvelopeDigest,
		Hook: "document_created", Kind: "close", ItemKeys: []string{},
	}); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(ctx, started.Receipt.RunID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, started.Receipt.RunID)
	if len(r.PublicationAssignments) != 2 || len(streamTasks(t, e, r.ID)) != 1 {
		t.Fatalf("empty stream invoked an item consumer: assignments=%+v", r.PublicationAssignments)
	}
	for _, assignment := range r.PublicationAssignments {
		if assignment.Kind != "Closed" || assignment.Cursor != 0 || assignment.Status != "processed" {
			t.Fatalf("empty stream did not use one explicit closure iteration: %+v", assignment)
		}
	}
}

func TestEachPublicationTimeoutIsInterruptedNotEOF(t *testing.T) {
	e, options := publicationStreamRuntime(t, 1)
	ctx := context.Background()
	started, err := e.Start(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(ctx, started.Receipt.RunID); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond)
	if err := e.Drive(ctx, started.Receipt.RunID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, started.Receipt.RunID)
	if len(r.PublicationAssignments) != 2 || len(streamTasks(t, e, r.ID)) != 1 {
		t.Fatalf("timeout invoked an item consumer: assignments=%+v", r.PublicationAssignments)
	}
	for _, assignment := range r.PublicationAssignments {
		if assignment.Kind != "Interrupted" || assignment.Status != "processed" {
			t.Fatalf("timeout was mistaken for successful EOF: %+v", assignment)
		}
		if r.PublicationSubscriptions[assignment.SubscriptionID].Status != "interrupted" {
			t.Fatalf("interrupted delivery left an open subscription: %+v", r.PublicationSubscriptions[assignment.SubscriptionID])
		}
	}
}

func TestEachPublicationInterruptsOnTerminalProducerFailure(t *testing.T) {
	e, options := publicationStreamRuntimeSource(t, 60, "retained", "interrupt_on_terminal_failure")
	ctx := context.Background()
	started, err := e.Start(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(ctx, started.Receipt.RunID); err != nil {
		t.Fatal(err)
	}
	producer := streamTasks(t, e, started.Receipt.RunID)["producer"]
	if producer.AttemptID == "" {
		t.Fatal("stream producer was not admitted")
	}
	if _, err := e.SubmitSession(ctx, sessionFail(t, producer)); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(ctx, started.Receipt.RunID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, started.Receipt.RunID)
	if r.SchemaVersion != CoreActionDeliveryStateVersion || len(r.PublicationAssignments) != 2 {
		t.Fatalf("producer failure did not create interrupted deliveries in the current session state: state=%s assignments=%+v", r.SchemaVersion, r.PublicationAssignments)
	}
	for _, assignment := range r.PublicationAssignments {
		if assignment.Kind != "Interrupted" || r.PublicationSubscriptions[assignment.SubscriptionID].Status != "interrupted" {
			t.Fatalf("stream producer failure was not a durable interruption: %+v", assignment)
		}
		_, data, err := e.Artifact(assignment.Delivery)
		if err != nil {
			t.Fatal(err)
		}
		var delivery PublicationDelivery
		if err := json.Unmarshal(data, &delivery); err != nil || delivery.Reason != "producer_terminal_failed" {
			t.Fatalf("stream interruption lost its terminal failure reason: delivery=%+v err=%v", delivery, err)
		}
	}
	if err := validatePublic(t, "CoreRunStateV21", r); err != nil {
		t.Fatalf("current session state rejects terminal producer failure: %v", err)
	}
}
