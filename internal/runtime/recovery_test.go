package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// The source fixture has accepted quality gates and a later technical failure.
// A passing process is deliberately not an accepted StepResult.
func recoverFailedTestsFixture() Run {
	observed := Observation{UTC: "2026-09-22T20:18:38Z"}
	code := 0
	return Run{
		ID: "run:source", Status: "failed", Profile: flow.CoreProfile,
		Settled: &observed, Active: []string{}, Stops: []Stop{},
		RootInvocationID: "invocation:root",
		Invocations: map[string]*Invocation{
			"invocation:root":   {ID: "invocation:root", Status: "failed"},
			"invocation:verify": {ID: "invocation:verify", ParentInvocationID: "invocation:root", CallerActivationID: "activation:verify_call", Status: "completed", Outputs: map[string]ArtifactRef{"gate": {ArtifactID: "artifact:verify", Revision: 1, Digest: "sha256:verify"}}},
			"invocation:review": {ID: "invocation:review", ParentInvocationID: "invocation:root", CallerActivationID: "activation:review_call", Status: "completed", Outputs: map[string]ArtifactRef{"gate": {ArtifactID: "artifact:review", Revision: 1, Digest: "sha256:review"}}},
		},
		Activations: map[string]*Activation{
			"activation:verify_call": {ID: "activation:verify_call", InvocationID: "invocation:root", StageID: "verify_call", Kind: "call", Status: "completed", Settled: &observed},
			"activation:review_call": {ID: "activation:review_call", InvocationID: "invocation:root", StageID: "review_call", Kind: "call", Status: "completed", Settled: &observed},
			"activation:verify":      {ID: "activation:verify", InvocationID: "invocation:verify", StageID: "verify", Kind: "step", Status: "completed", StepID: "step:verify", Settled: &observed},
			"activation:review":      {ID: "activation:review", InvocationID: "invocation:review", StageID: "review", Kind: "step", Status: "completed", StepID: "step:review", Settled: &observed},
			"activation:tests":       {ID: "activation:tests", InvocationID: "invocation:root", StageID: "tests", Kind: "step", Status: "failed", StepID: "step:tests", Settled: &observed},
		},
		Steps: map[string]*Step{
			"step:verify": {ID: "step:verify", ActivationID: "activation:verify", Status: "completed", Verdict: "pass", AttemptIDs: []string{"attempt:verify"}, Outputs: map[string]ArtifactRef{"gate": {ArtifactID: "artifact:verify", Revision: 1, Digest: "sha256:verify"}}},
			"step:review": {ID: "step:review", ActivationID: "activation:review", Status: "completed", Verdict: "pass", AttemptIDs: []string{"attempt:review"}, Outputs: map[string]ArtifactRef{"gate": {ArtifactID: "artifact:review", Revision: 1, Digest: "sha256:review"}}},
			"step:tests":  {ID: "step:tests", ActivationID: "activation:tests", Status: "failed", AttemptIDs: []string{"attempt:tests"}, Outputs: map[string]ArtifactRef{}},
		},
		Attempts: map[string]*Attempt{
			"attempt:verify": {ID: "attempt:verify", StepID: "step:verify", ActivationID: "activation:verify", Status: "completed", Settled: &observed, Session: &SessionHandoff{}, Accepted: &Result{Verdict: "pass", Outputs: map[string]ArtifactRef{"gate": {ArtifactID: "artifact:verify", Revision: 1, Digest: "sha256:verify"}}}},
			"attempt:review": {ID: "attempt:review", StepID: "step:review", ActivationID: "activation:review", Status: "completed", Settled: &observed, Session: &SessionHandoff{}, Accepted: &Result{Verdict: "pass", Outputs: map[string]ArtifactRef{"gate": {ArtifactID: "artifact:review", Revision: 1, Digest: "sha256:review"}}}},
			"attempt:tests":  {ID: "attempt:tests", StepID: "step:tests", ActivationID: "activation:tests", Status: "failed", Settled: &observed, ProcessOutcome: &local.ProcessOutcome{Started: true, WaitReturned: true, GroupEmpty: true, ExitCode: &code}},
		},
		Diagnostics: []Diagnostic{{AttemptID: "attempt:tests", Code: "invalid_output"}},
	}
}

func TestRecoverSourceFindsTechnicalFailureWithoutPromotingProcessExit(t *testing.T) {
	source := recoverFailedTestsFixture()
	frontier, attempt, err := recoveryFrontier(source)
	if err != nil || frontier.StageID != "tests" || attempt.ID != "attempt:tests" {
		t.Fatalf("frontier: %+v %+v %v", frontier, attempt, err)
	}
	if source.Steps["step:tests"].Verdict != "" || source.Attempts["attempt:tests"].Accepted != nil {
		t.Fatal("a settled exit 0 was promoted to an accepted StepResult")
	}
	candidate := ArtifactRef{ArtifactID: "artifact:candidate", Revision: 1, Digest: "sha256:candidate"}
	events := []local.Event{{EventInput: local.EventInput{Type: "attempt.result_candidate", Version: 1, Data: json.RawMessage(`{"attempt_id":"attempt:tests","candidate_digest":"sha256:candidate","disposition":"candidate","evidence_ref":{"artifact_id":"artifact:candidate","revision":1,"digest":"sha256:candidate"}}`)}}}
	selected, err := recoveryCandidateRef(events, attempt.ID)
	if err != nil || selected != candidate {
		t.Fatalf("candidate evidence: %+v %v", selected, err)
	}
	if err := validateForkReuse(source, ForkPayload{ReuseRefs: []ArtifactRef{source.Steps["step:review"].Outputs["gate"]}}, nil); err == nil {
		t.Fatal("ordinary fork reused an output from a technically failed Run")
	}
}

func TestRecoverChoicesCannotChange(t *testing.T) {
	base := &DecisionSheet{Records: []DecisionRecord{{DefinitionID: "security", DefinitionDigest: "sha256:same", Status: "answered", Value: json.RawMessage(`true`)}}}
	same := &DecisionSheet{Records: []DecisionRecord{{DefinitionID: "security", DefinitionDigest: "sha256:same", Status: "answered", Value: json.RawMessage(`true`)}}}
	if !recoverySameChoices(base, same) {
		t.Fatal("unchanged choices rejected")
	}
	same.Records[0].Value = json.RawMessage(`false`)
	if recoverySameChoices(base, same) {
		t.Fatal("changed choice accepted")
	}
}

func TestRecoverRevalidatedProvenanceNeedsCandidate(t *testing.T) {
	r := Run{SchemaVersion: CoreRecoveryStateVersion, Fork: &ForkProvenance{SourceRunID: "run:source", SourceRunVersion: 2}, Recovery: &RecoveryProvenance{SchemaVersion: "recovery/1", SourceRunID: "run:source", SourceRunVersion: 2, ReviewDigest: "sha256:review", SubjectCommit: strings.Repeat("a", 40), FrontierStageID: "tests", FrontierAction: "revalidate", Reused: []RecoveryReuse{{StageID: "verify"}}, RootOutputs: map[string]map[string]ArtifactRef{}}}
	if recoveryInvariant(r) == nil {
		t.Fatal("revalidation without immutable candidate was accepted")
	}
	ref := ArtifactRef{ArtifactID: "artifact:candidate", Revision: 1, Digest: "sha256:candidate"}
	r.Recovery.CandidateRef = &ref
	if err := recoveryInvariant(r); err != nil {
		t.Fatalf("sealed candidate provenance rejected: %v", err)
	}
}

func TestRecoverSealedCandidateRevalidatesWithoutProcess(t *testing.T) {
	e, options := contextDriverProject(t, nil)
	plan, _, _, _, _, err := e.compileFileWithBindings(options.WorkflowFile, nil)
	if err != nil {
		t.Fatal(err)
	}
	step := plan.Steps["work"]
	step.Effects.Class = "none"
	plan.Steps["work"] = step
	port := plan.Steps["work"].Outputs["report"]
	data := []byte("accepted output\n")
	artifact, err := e.putArtifact(data, port.Format, port.SchemaRef, derivedID("artifact", "recover-candidate"), artifactProducer(e), nil, plan.Registry, portMedia(port.Port))
	if err != nil {
		t.Fatal(err)
	}
	result := Result{SchemaVersion: "1", RunID: "run:source", StepInstanceID: "step:source", AttemptID: "attempt:source", EnvelopeDigest: "sha256:" + strings.Repeat("a", 64), Verdict: "pass", Outputs: map[string]ArtifactRef{"report": artifact.Ref()}, EvidenceRefs: []any{}, EffectReceiptRefs: []any{}, Summary: "sealed"}
	encoded, _ := json.Marshal(result)
	if err := e.recoveryValidateCandidate(plan, "work", result, encoded); err != nil {
		t.Fatalf("sealed candidate: %v", err)
	}
	step.Effects.Class = "workspace_write"
	plan.Steps["work"] = step
	if err := e.recoveryValidateCandidate(plan, "work", result, encoded); err == nil {
		t.Fatal("workspace effect was silently skipped")
	}
	step.Effects.Class = "none"
	plan.Steps["work"] = step
	result.Outputs["report"] = ArtifactRef{ArtifactID: "artifact:missing", Revision: 1, Digest: artifact.Digest}
	if err := e.recoveryValidateCandidate(plan, "work", result, encoded); err == nil {
		t.Fatal("missing sealed bytes were accepted")
	}
}

func TestRecoverCandidateRejectsUnsealedOrConflictingEvidence(t *testing.T) {
	event := func(digest string) local.Event {
		return local.Event{EventInput: local.EventInput{Type: "attempt.result_candidate", Version: 1, Data: json.RawMessage(`{"attempt_id":"attempt:tests","candidate_digest":"sha256:expected","disposition":"candidate","evidence_ref":{"artifact_id":"artifact:candidate","revision":1,"digest":"` + digest + `"}}`)}}
	}
	for name, events := range map[string][]local.Event{
		"unsealed":  {event("sha256:wrong")},
		"conflict":  {event("sha256:expected"), {EventInput: local.EventInput{Type: "attempt.result_candidate", Version: 1, Data: json.RawMessage(`{"attempt_id":"attempt:tests","candidate_digest":"sha256:other","disposition":"candidate","evidence_ref":{"artifact_id":"artifact:other","revision":1,"digest":"sha256:other"}}`)}}},
		"malformed": {{EventInput: local.EventInput{Type: "attempt.result_candidate", Version: 1, Data: json.RawMessage(`{`)}}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := recoveryCandidateRef(events, "attempt:tests"); err == nil {
				t.Fatal("invalid candidate evidence was accepted")
			}
		})
	}
}

func TestRecoverEffectiveDefinitionIgnoresCompiledRefOnlyChange(t *testing.T) {
	build := func(version, schemaType string) (flow.Ref, []PinnedDefinition) {
		schemaBytes := []byte(fmt.Sprintf(`{"id":"test:schema/gate","version":%q,"type":%q}`, version, schemaType))
		schemaDigest, _ := flow.Digest(schemaBytes)
		schemaRef := flow.Ref{ID: "test:schema/gate", Version: version, Digest: schemaDigest}
		stepBytes := []byte(fmt.Sprintf(`{"id":"test:step/verify","version":%q,"result_schema_ref":{"id":%q,"version":%q,"digest":%q}}`, version, schemaRef.ID, schemaRef.Version, schemaRef.Digest))
		stepDigest, _ := flow.Digest(stepBytes)
		stepRef := flow.Ref{ID: "test:step/verify", Version: version, Digest: stepDigest}
		return stepRef, []PinnedDefinition{{Ref: schemaRef, Kind: "schema", Bytes: schemaBytes}, {Ref: stepRef, Kind: "step", Bytes: stepBytes}}
	}
	oldRef, oldDefinitions := build("0.0.0-b1.old", "object")
	newRef, newDefinitions := build("0.0.0-b1.new", "object")
	oldBytes, err := recoveryEffectiveBytes(oldRef, oldDefinitions, nil)
	if err != nil {
		t.Fatal(err)
	}
	newBytes, err := recoveryEffectiveBytes(newRef, newDefinitions, nil)
	if err != nil || !bytes.Equal(oldBytes, newBytes) {
		t.Fatalf("compiled-only change invalidated the gate: %v", err)
	}
	changedRef, changedDefinitions := build("0.0.0-b1.changed", "array")
	changedBytes, err := recoveryEffectiveBytes(changedRef, changedDefinitions, nil)
	if err != nil || bytes.Equal(oldBytes, changedBytes) {
		t.Fatalf("changed result check was treated as equivalent: %v", err)
	}
	if _, err := recoveryEffectiveBytes(oldRef, oldDefinitions[1:], nil); err == nil {
		t.Fatal("missing pinned schema was treated as equivalent")
	}
	oldStage := flow.Stage{Kind: "step", StepRef: oldRef, InputBindings: map[string]flow.Binding{"implementation": {From: "stage_output", StageID: "review", Port: "implementation"}}, On: map[string]string{"pass": "tests"}}
	newStage := flow.Stage{Kind: "step", StepRef: newRef, InputBindings: oldStage.InputBindings, On: oldStage.On}
	oldShape, err := recoveryEffectiveStage(oldStage, oldDefinitions, nil)
	if err != nil {
		t.Fatal(err)
	}
	newShape, err := recoveryEffectiveStage(newStage, newDefinitions, nil)
	if err != nil || !bytes.Equal(oldShape, newShape) {
		t.Fatalf("compiled-only stage change invalidated the route: %v", err)
	}
	newStage.On = map[string]string{"pass": "other"}
	changedRoute, err := recoveryEffectiveStage(newStage, newDefinitions, nil)
	if err != nil || bytes.Equal(oldShape, changedRoute) {
		t.Fatalf("changed route was reused: %v", err)
	}
	newStage.On = oldStage.On
	newStage.InputBindings = map[string]flow.Binding{"implementation": {From: "input", Port: "implementation"}}
	changedInput, err := recoveryEffectiveStage(newStage, newDefinitions, nil)
	if err != nil || bytes.Equal(oldShape, changedInput) {
		t.Fatalf("changed binding was reused: %v", err)
	}
}

func TestRecoverSourceRejectsOpenOrAnsweredRuns(t *testing.T) {
	cases := map[string]func(*Run){
		"outcome":          func(r *Run) { r.Status, r.Outcome = "completed", stringPointer("rejected") },
		"active attempt":   func(r *Run) { r.Active = []string{"attempt:tests"} },
		"uncertain effect": func(r *Run) { r.HasUnresolvedEffects = true },
		"cancel":           func(r *Run) { r.CancelRequested = true },
		"stop":             func(r *Run) { r.Stops = []Stop{{Kind: "pause", Status: "active"}} },
		"no frontier":      func(r *Run) { r.Activations["activation:tests"].Status = "completed" },
		"no settlement":    func(r *Run) { r.Attempts["attempt:tests"].Settled = nil },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			source := recoverFailedTestsFixture()
			change(&source)
			if _, _, err := recoveryFrontier(source); err == nil || !strings.Contains(err.Error(), "recover_") {
				t.Fatalf("ineligible source accepted: %v", err)
			}
		})
	}
}

func TestRecoverTraceReusesNestedQualityGatesAndRejectsChangedPrefix(t *testing.T) {
	definition := func(id, version string) (flow.Ref, PinnedDefinition) {
		data := []byte(fmt.Sprintf(`{"id":%q,"version":%q,"kind":"step"}`, id, version))
		digest, _ := flow.Digest(data)
		ref := flow.Ref{ID: id, Version: version, Digest: digest}
		return ref, PinnedDefinition{Ref: ref, Kind: "step", Bytes: data}
	}
	workflow := func(id, version string) (flow.Ref, PinnedDefinition) {
		data := []byte(fmt.Sprintf(`{"id":%q,"version":%q,"kind":"workflow"}`, id, version))
		digest, _ := flow.Digest(data)
		ref := flow.Ref{ID: id, Version: version, Digest: digest}
		return ref, PinnedDefinition{Ref: ref, Kind: "workflow", Bytes: data}
	}
	makePlan := func(version string) (*flow.Plan, []PinnedDefinition, flow.Ref, flow.Ref) {
		verifyStep, verifyDef := definition("test:step/verify", version)
		reviewStep, reviewDef := definition("test:step/review", version)
		testsStep, testsDef := definition("test:step/tests", version)
		verifyRef, verifyWorkflow := workflow("test:workflow/verify", version)
		reviewRef, reviewWorkflow := workflow("test:workflow/review", version)
		verify := &flow.Plan{Workflow: flow.WorkflowRevision{ID: verifyRef.ID}, Calls: map[string]*flow.Plan{}}
		verify.Workflow.Definition.Entry = "verify"
		verify.Workflow.Definition.Stages = map[string]flow.Stage{"verify": {Kind: "step", StepRef: verifyStep, On: map[string]string{"pass": "done"}}}
		review := &flow.Plan{Workflow: flow.WorkflowRevision{ID: reviewRef.ID}, Calls: map[string]*flow.Plan{}}
		review.Workflow.Definition.Entry = "review"
		review.Workflow.Definition.Stages = map[string]flow.Stage{"review": {Kind: "step", StepRef: reviewStep, On: map[string]string{"pass": "done"}}}
		root := &flow.Plan{Workflow: flow.WorkflowRevision{ID: "test:workflow/quality-tail"}, Calls: map[string]*flow.Plan{"verify_call": verify, "review_call": review}}
		root.Workflow.Definition.Entry = "verify_call"
		root.Workflow.Definition.Stages = map[string]flow.Stage{
			"verify_call": {Kind: "call", WorkflowRef: verifyRef, On: map[string]string{"succeeded": "review_call"}},
			"review_call": {Kind: "call", WorkflowRef: reviewRef, On: map[string]string{"succeeded": "tests"}},
			"tests":       {Kind: "step", StepRef: testsStep, InputBindings: map[string]flow.Binding{"gate": {From: "stage_output", StageID: "review_call", Port: "gate"}}, On: map[string]string{"pass": "done"}},
		}
		return root, []PinnedDefinition{verifyDef, reviewDef, testsDef, verifyWorkflow, reviewWorkflow}, verifyStep, reviewStep
	}
	oldPlan, oldDefs, oldVerify, oldReview := makePlan("0.0.0-b1.old")
	newPlan, newDefs, _, _ := makePlan("0.0.0-b1.new")
	source := recoverFailedTestsFixture()
	source.Definitions = oldDefs
	source.Steps["step:verify"].Ref = oldVerify
	source.Steps["step:review"].Ref = oldReview
	ids := []string{"activation:verify_call", "activation:verify", "activation:review_call", "activation:review", "activation:tests"}
	events := make([]local.Event, 0, len(ids))
	for i, id := range ids {
		data, _ := json.Marshal(map[string]string{"stage_activation_id": id})
		events = append(events, local.Event{Seq: int64(i + 1), EventInput: local.EventInput{Type: "stage.activated", Data: data}})
	}
	reused, outputs, err := recoveryTrace(source, oldPlan, newPlan, newDefs, nil, events)
	if err != nil || len(reused) != 4 || outputs["review_call"]["gate"] != source.Steps["step:review"].Outputs["gate"] {
		t.Fatalf("nested gate reuse: %v; reused=%+v outputs=%+v", err, reused, outputs)
	}
	source.Attempts["attempt:verify"].Session = nil
	if _, _, err := recoveryTrace(source, oldPlan, newPlan, newDefs, nil, events); err == nil || !strings.Contains(err.Error(), "recover_topology_unsupported") {
		t.Fatalf("program gate was reused without executor pin: %v", err)
	}
	source.Attempts["attempt:verify"].Session = &SessionHandoff{}
	changed := *newPlan
	changed.Workflow = newPlan.Workflow
	changed.Workflow.Definition.Stages = map[string]flow.Stage{}
	for id, stage := range newPlan.Workflow.Definition.Stages {
		changed.Workflow.Definition.Stages[id] = stage
	}
	stage := changed.Workflow.Definition.Stages["review_call"]
	stage.On = map[string]string{"succeeded": "other"}
	changed.Workflow.Definition.Stages["review_call"] = stage
	if _, _, err := recoveryTrace(source, oldPlan, &changed, newDefs, nil, events); err == nil || !strings.Contains(err.Error(), "recover_prefix_changed") {
		t.Fatalf("changed route was reused: %v", err)
	}
}
