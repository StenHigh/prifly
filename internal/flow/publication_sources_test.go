package flow

import "testing"

func publicationComposition(readPolicy string) *Plan {
	schema := Ref{ID: "test:schema/document", Version: "1.0.0", Digest: "sha256:0000000000000000000000000000000000000000000000000000000000000000"}
	sourceRef := Ref{ID: "test:source/document", Version: "1.0.0", Digest: "sha256:1111111111111111111111111111111111111111111111111111111111111111"}
	source := PublicationSourceDefinition{
		SchemaVersion: PublicationSourceVersion, ID: sourceRef.ID, Version: sourceRef.Version,
		Mode: "once", ProducerBranchID: "producer", ProducerStageID: "produce", Hook: "document_created",
		HookSchemaRef: schema, ItemKey: "item", Initial: "retained", ProducerFailure: "wait_until_timeout",
	}
	producer := &Plan{Steps: map[string]StepDefinition{}, publicationSources: map[Ref]PublicationSourceDefinition{}}
	producer.Workflow.Definition.Stages = map[string]Stage{"produce": {Kind: "step"}}
	producer.Steps["produce"] = StepDefinition{Hooks: map[string]Hook{"document_created": {
		Kind: "artifact", SchemaRef: schema, ReadPolicy: readPolicy,
		Artifact: &ArtifactHook{Format: "json", Cardinality: "one", EarlyConsumption: true},
	}}}
	consumer := &Plan{publicationSources: map[Ref]PublicationSourceDefinition{sourceRef: source}}
	consumer.Workflow.Definition.Stages = map[string]Stage{"await_document": {Kind: "wait", SourceRef: sourceRef, EventSchemaRef: schema}}
	root := &Plan{Branches: map[string]map[string]*Plan{"fan": {"producer": producer, "consumer": consumer}}}
	root.Workflow.Limits.MaxParallelism = 2
	root.Workflow.Definition.Stages = map[string]Stage{"fan": {
		Kind: "parallel", MaxParallelism: 2,
		ParallelBranches: []ParallelBranch{{ID: "producer"}, {ID: "consumer"}},
	}}
	return root
}

func TestOncePublicationRequiresDeclaredSubscriberAccess(t *testing.T) {
	err := publicationComposition("owner").checkPublicationCompositions()
	p := expectProblem(t, err, "publication_read_forbidden")
	if p.Path != "/definition/stages/fan/branches/1/workflow_ref@/definition/stages/await_document/source_ref@test:source/document/hook" {
		t.Fatalf("access refusal points elsewhere: %s", p.Path)
	}
	if err := publicationComposition("declared_subscribers").checkPublicationCompositions(); err != nil {
		t.Fatalf("an explicitly readable sibling publication was refused: %v", err)
	}
}

func TestEachPublicationRequiresOneDirectRepeat(t *testing.T) {
	root := publicationComposition("declared_subscribers")
	producer := root.Branches["fan"]["producer"]
	hook := producer.Steps["produce"].Hooks["document_created"]
	hook.Artifact.Cardinality = "keyed_many"
	producer.Steps["produce"] = StepDefinition{Hooks: map[string]Hook{"document_created": hook}}

	consumer := root.Branches["fan"]["consumer"]
	sourceRef := consumer.Workflow.Definition.Stages["await_document"].SourceRef
	source := consumer.publicationSources[sourceRef]
	source.SchemaVersion, source.Mode, source.ItemKey = PublicationStreamSourceVersion, "each_publication", ""
	source.HandleSchemaRef, source.CursorSchemaRef, source.DeliverySchemaRef = &source.HookSchemaRef, &source.HookSchemaRef, &source.HookSchemaRef
	streamBody := &Plan{Registry: Registry{source.HookSchemaRef: []byte(`{}`)}, publicationSources: map[Ref]PublicationSourceDefinition{sourceRef: source}}
	streamBody.Workflow.Definition.Stages = consumer.Workflow.Definition.Stages

	outerBody := &Plan{Repeats: map[string]*Plan{"inner": streamBody}}
	outerBody.Workflow.Definition.Stages = map[string]Stage{"inner": {Kind: "repeat"}}
	consumer.Repeats = map[string]*Plan{"outer": outerBody}
	consumer.Workflow.Definition.Stages = map[string]Stage{"outer": {Kind: "repeat"}}

	p := expectProblem(t, root.checkPublicationCompositions(), "unsupported_publication_source")
	if p.Path != "/definition/stages/fan/branches/1/workflow_ref@/definition/stages/outer/body_workflow_ref@/definition/stages/inner/body_workflow_ref@/definition/stages/await_document/source_ref" {
		t.Fatalf("nested stream refusal points elsewhere: %s", p.Path)
	}
}

func TestPublicationSourceSchemaIsClosed(t *testing.T) {
	source := PublicationSourceDefinition{
		SchemaVersion: PublicationSourceVersion, ID: "test:source/document", Version: "1.0.0", Mode: "once",
		ProducerBranchID: "producer", ProducerStageID: "produce", Hook: "document_created",
		HookSchemaRef: Ref{ID: "test:schema/document", Version: "1.0.0", Digest: "sha256:0000000000000000000000000000000000000000000000000000000000000000"},
		ItemKey:       "item", Initial: "retained", ProducerFailure: "wait_until_timeout",
	}
	if err := ValidateProtocol("PublicationSourceDefinition", encoded(t, source)); err != nil {
		t.Fatal(err)
	}
	value := map[string]any{}
	if err := decodeValue(source, &value); err != nil {
		t.Fatal(err)
	}
	value["cursor"] = "future"
	if err := ValidateProtocol("PublicationSourceDefinition", encoded(t, value)); err == nil {
		t.Fatal("once source silently accepted a future streaming cursor")
	}
}

func TestPublicationStreamSourceSchemaIsClosed(t *testing.T) {
	ref := Ref{ID: "core:schema/transport", Version: "1.0.0", Digest: "sha256:0000000000000000000000000000000000000000000000000000000000000000"}
	source := PublicationSourceDefinition{
		SchemaVersion: PublicationStreamSourceVersion, ID: "test:source/documents", Version: "1.0.0", Mode: "each_publication",
		ProducerBranchID: "producer", ProducerStageID: "produce", Hook: "document_created", HookSchemaRef: ref,
		HandleSchemaRef: &ref, CursorSchemaRef: &ref, DeliverySchemaRef: &ref,
		Initial: "retained", ProducerFailure: "wait_until_timeout",
	}
	if err := ValidateProtocol("PublicationSourceDefinitionV2", encoded(t, source)); err != nil {
		t.Fatal(err)
	}
	value := map[string]any{}
	if err := decodeValue(source, &value); err != nil {
		t.Fatal(err)
	}
	value["item_key"] = "first"
	if err := ValidateProtocol("PublicationSourceDefinitionV2", encoded(t, value)); err == nil {
		t.Fatal("each-publication source silently accepted one fixed item key")
	}
	if err := ValidateProtocol("PublicationSourceDefinition", encoded(t, source)); err == nil {
		t.Fatal("the delivered once source silently acquired stream semantics")
	}
}

func TestNewOnlyPublicationSourcesAreSeparateClosedContracts(t *testing.T) {
	ref := Ref{ID: "core:schema/transport", Version: "1.0.0", Digest: "sha256:0000000000000000000000000000000000000000000000000000000000000000"}
	once := PublicationSourceDefinition{
		SchemaVersion: PublicationNewOnlySourceVersion, ID: "test:source/new-document", Version: "1.0.0", Mode: "once",
		ProducerBranchID: "producer", ProducerStageID: "produce", Hook: "document_created", HookSchemaRef: ref,
		ItemKey: "item", Initial: "new_only", ProducerFailure: "wait_until_timeout",
	}
	if err := ValidateProtocol("PublicationSourceDefinitionV3", encoded(t, once)); err != nil {
		t.Fatal(err)
	}
	if err := ValidateProtocol("PublicationSourceDefinition", encoded(t, once)); err == nil {
		t.Fatal("retained once source silently accepted new_only semantics")
	}
	stream := PublicationSourceDefinition{
		SchemaVersion: PublicationNewOnlyStreamSourceVersion, ID: "test:source/new-documents", Version: "1.0.0", Mode: "each_publication",
		ProducerBranchID: "producer", ProducerStageID: "produce", Hook: "document_created", HookSchemaRef: ref,
		HandleSchemaRef: &ref, CursorSchemaRef: &ref, DeliverySchemaRef: &ref,
		Initial: "new_only", ProducerFailure: "wait_until_timeout",
	}
	if err := ValidateProtocol("PublicationSourceDefinitionV4", encoded(t, stream)); err != nil {
		t.Fatal(err)
	}
	if err := ValidateProtocol("PublicationSourceDefinitionV2", encoded(t, stream)); err == nil {
		t.Fatal("retained stream source silently accepted new_only semantics")
	}
}

func TestTerminalFailurePublicationSourcesAreSeparateClosedContracts(t *testing.T) {
	ref := Ref{ID: "core:schema/transport", Version: "1.0.0", Digest: "sha256:0000000000000000000000000000000000000000000000000000000000000000"}
	once := PublicationSourceDefinition{SchemaVersion: PublicationFailureSourceVersion, ID: "test:source/failing-document", Version: "1.0.0", Mode: "once", ProducerBranchID: "producer", ProducerStageID: "produce", Hook: "document_created", HookSchemaRef: ref, ItemKey: "item", Initial: "retained", ProducerFailure: "interrupt_on_terminal_failure"}
	if err := ValidateProtocol("PublicationSourceDefinitionV5", encoded(t, once)); err != nil {
		t.Fatal(err)
	}
	if err := ValidateProtocol("PublicationSourceDefinitionV3", encoded(t, once)); err == nil {
		t.Fatal("new-only source silently accepted terminal-failure semantics")
	}
	stream := PublicationSourceDefinition{SchemaVersion: PublicationFailureStreamSourceVersion, ID: "test:source/failing-documents", Version: "1.0.0", Mode: "each_publication", ProducerBranchID: "producer", ProducerStageID: "produce", Hook: "document_created", HookSchemaRef: ref, HandleSchemaRef: &ref, CursorSchemaRef: &ref, DeliverySchemaRef: &ref, Initial: "new_only", ProducerFailure: "interrupt_on_terminal_failure"}
	if err := ValidateProtocol("PublicationSourceDefinitionV6", encoded(t, stream)); err != nil {
		t.Fatal(err)
	}
	if err := ValidateProtocol("PublicationSourceDefinitionV4", encoded(t, stream)); err == nil {
		t.Fatal("new-only stream source silently accepted terminal-failure semantics")
	}
}

func TestBlobPublicationSourcesAreSeparateClosedContracts(t *testing.T) {
	ref := Ref{ID: "core:schema/blob-descriptor", Version: "1.0.0", Digest: "sha256:0000000000000000000000000000000000000000000000000000000000000000"}
	once := PublicationSourceDefinition{SchemaVersion: PublicationBlobSourceVersion, ID: "test:source/blob", Version: "1.0.0", Mode: "once", ProducerBranchID: "producer", ProducerStageID: "produce", Hook: "document_created", HookSchemaRef: ref, ItemKey: "item", Initial: "retained", ProducerFailure: "wait_until_timeout", Format: "blob", MediaTypes: []string{"text/plain"}}
	if err := ValidateProtocol("PublicationSourceDefinitionV7", encoded(t, once)); err != nil {
		t.Fatal(err)
	}
	if err := ValidateProtocol("PublicationSourceDefinitionV5", encoded(t, once)); err == nil {
		t.Fatal("terminal-failure source silently acquired blob delivery semantics")
	}
	broken := once
	broken.MediaTypes = nil
	if err := ValidateProtocol("PublicationSourceDefinitionV7", encoded(t, broken)); err == nil {
		t.Fatal("blob source accepted an unspecified media type")
	}
	stream := PublicationSourceDefinition{SchemaVersion: PublicationBlobStreamSourceVersion, ID: "test:source/blobs", Version: "1.0.0", Mode: "each_publication", ProducerBranchID: "producer", ProducerStageID: "produce", Hook: "document_created", HookSchemaRef: ref, HandleSchemaRef: &ref, CursorSchemaRef: &ref, DeliverySchemaRef: &ref, Initial: "new_only", ProducerFailure: "interrupt_on_terminal_failure", Format: "blob", MediaTypes: []string{"text/plain"}}
	if err := ValidateProtocol("PublicationSourceDefinitionV8", encoded(t, stream)); err != nil {
		t.Fatal(err)
	}
	if err := ValidateProtocol("PublicationSourceDefinitionV6", encoded(t, stream)); err == nil {
		t.Fatal("terminal-failure stream source silently acquired blob delivery semantics")
	}
}
