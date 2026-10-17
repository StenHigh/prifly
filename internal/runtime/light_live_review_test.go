package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

// A real two-step, assisted Run accepts typed consumer reports through the
// session protocol. The host is deterministic fixture code, never an LLM.
func TestLightNativeTypedReviewRun(t *testing.T) {
	t.Parallel()
	f := newLightReviewFixture(t, "blocked", "passed", "executed", false, nil)
	e := f.Engine
	ctx := context.Background()
	defs, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	entries := []Definition{}
	for i, d := range f.Run.Definitions {
		name := filepath.Join("schemas", string(rune('a'+i))+".json")
		writeRuntimeJSON(t, filepath.Join(e.Root, name), json.RawMessage(d.Bytes))
		entries = append(entries, Definition{Ref: d.Ref, Kind: "schema", Path: name})
	}
	resource := []byte("# Typed fixture host\nReturn the provided immutable report.\n")
	resourceRef := flow.Ref{ID: "test:context/light-review", Version: "1.0.0", Digest: rawDigest(resource)}
	if err := os.MkdirAll(filepath.Join(e.Root, "resources"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.Root, "resources/review.md"), resource, 0600); err != nil {
		t.Fatal(err)
	}
	entries = append(entries, Definition{Ref: resourceRef, Kind: "resource", Path: "resources/review.md", ByteEncoding: "utf8_text", MediaType: "text/markdown; charset=utf-8"})
	requestRef, candidateRef := f.Reader.Context.Inputs["request"].Ref, f.Reader.Context.Inputs["candidate"].Ref
	requestMeta, _, err := e.Artifact(requestRef)
	if err != nil {
		t.Fatal(err)
	}
	candidateMeta, _, err := e.Artifact(candidateRef)
	if err != nil {
		t.Fatal(err)
	}
	steps := map[string]flow.Ref{}
	for _, stage := range []string{"review", "evaluate"} {
		inputs := map[string]flow.InputPort{"request": {Port: flow.Port{Format: "json", SchemaRef: requestMeta.SchemaRef}, Required: true}, "candidate": {Port: flow.Port{Format: "json", SchemaRef: candidateMeta.SchemaRef}, Required: true}}
		output, schema := "report", f.Mapping.ReportSchema
		if stage == "evaluate" {
			output = "gate"
			schema = f.Mapping.GateSchema
			inputs["report"] = flow.InputPort{Port: flow.Port{Format: "json", SchemaRef: &f.Mapping.ReportSchema}, Required: true}
		}
		step := flow.StepDefinition{SchemaVersion: "2", ID: "test:step/light-" + stage, Version: "1.0.0", Title: stage, Kind: "worker", Inputs: inputs, Outputs: map[string]flow.OutputPort{output: {Port: flow.Port{Format: "json", SchemaRef: &schema}, RequiredFor: []string{"pass"}}}, InstructionsRef: &resourceRef, ContextRefs: []flow.Ref{}, RequiredCapabilities: []string{}, ResultCheckRefs: []flow.Ref{}, ResultSchemaRef: builtinRef(defs, "core:schema/step-result")}
		step.Executor.AdapterRef = builtinRef(defs, "core:adapter/assisted-session")
		step.Executor.Operation = "session"
		step.Effects.Class = "none"
		step.Effects.RetryClass = "never"
		name := "steps/light-" + stage + ".json"
		body := writeRegistryDocument(t, e, name, step)
		ref := flow.Ref{ID: step.ID, Version: step.Version, Digest: rawDigest(body)}
		steps[stage] = ref
		entries = append(entries, Definition{Ref: ref, Kind: "step", Path: name})
	}
	workflow := flow.WorkflowRevision{SchemaVersion: "1", ID: "test:workflow/light-review", Version: "1.0.0", Title: "Typed review fixture", Inputs: map[string]flow.InputPort{"request": {Port: flow.Port{Format: "json", SchemaRef: requestMeta.SchemaRef}, Required: true}, "candidate": {Port: flow.Port{Format: "json", SchemaRef: candidateMeta.SchemaRef}, Required: true}}, Outputs: map[string]flow.OutputPort{"gate": {Port: flow.Port{Format: "json", SchemaRef: &f.Mapping.GateSchema}, RequiredFor: []string{"succeeded"}}}, AllowedOutcomes: []string{"succeeded"}, Limits: flow.Limits{MaxStepInstances: 4, MaxControlTransitions: 24, MaxParallelism: 1, MaxChildDepth: 1}, PolicyRef: builtinVersionRef(defs, "core:policy/local", "2.0.0")}
	workflow.Definition.Entry = "review"
	workflow.Definition.Stages = map[string]flow.Stage{"review": {Kind: "step", StepRef: steps["review"], InputBindings: map[string]flow.Binding{"request": {From: "workflow_input", Port: "request"}, "candidate": {From: "workflow_input", Port: "candidate"}}, On: map[string]string{"pass": "evaluate"}}, "evaluate": {Kind: "step", StepRef: steps["evaluate"], InputBindings: map[string]flow.Binding{"request": {From: "workflow_input", Port: "request"}, "candidate": {From: "workflow_input", Port: "candidate"}, "report": {From: "stage_output", StageID: "review", Port: "report"}}, On: map[string]string{"pass": "done"}}, "done": {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]flow.Binding{"gate": {From: "stage_output", StageID: "evaluate", Port: "gate"}}}}
	writeRuntimeJSON(t, filepath.Join(e.Root, "workflows/light-review.json"), workflow)
	writeRuntimeJSON(t, filepath.Join(e.Root, e.Config.Configuration.RegistryFile), RegistryFile{SchemaVersion: "3", Entries: entries})
	e.Config.Configuration.SemanticsProfile = flow.CoreProfile
	e.Config.Configuration.SchemaVersion = CoreContextConfigVersion
	e.Config.ConfigurationSchemaRef = builtinVersionRef(defs, "core:schema/core-configuration", "2.0.0")
	e.Config.DefaultPolicyRef = builtinVersionRef(defs, "core:policy/local", "2.0.0")
	e.Config.AdapterBindings["local_process"] = builtinVersionRef(defs, "core:adapter/local-process", "2.0.0")
	writeRuntimeJSON(t, filepath.Join(e.Root, "prifly.json"), e.Config)
	started, err := e.Start(ctx, StartOptions{CommandID: "command:light-native-review", WorkflowFile: "workflows/light-review.json", BriefFile: "brief.json", Inputs: map[string]string{}, InputRefs: map[string]ArtifactRef{"request": requestRef, "candidate": candidateRef}})
	if err != nil {
		t.Fatal(err)
	}
	for _, port := range []string{"report", "gate"} {
		if err := e.Drive(ctx, started.Receipt.RunID); err != nil {
			t.Fatal(err)
		}
		tasks, err := e.SessionTasks(ctx, started.Receipt.RunID)
		if err != nil || len(tasks) != 1 {
			t.Fatalf("one actual task: %v %v", tasks, err)
		}
		task, err := e.SessionTask(ctx, started.Receipt.RunID, tasks[0].AttemptID)
		if err != nil {
			t.Fatal(err)
		}
		artifact := f.Reader.Accepted.Outputs["report"]
		if port == "gate" {
			artifact = f.Run.Attempts["attempt:gate"].Accepted.Outputs["gate"]
		}
		_, body, err := e.Artifact(artifact)
		if err != nil {
			t.Fatal(err)
		}
		slot := task.Context.Outputs[port]
		if err := os.WriteFile(filepath.Join(task.Workspace, slot.Path), body, 0600); err != nil {
			t.Fatal(err)
		}
		result := Result{SchemaVersion: "1", RunID: started.Receipt.RunID, StepInstanceID: task.StepInstanceID, AttemptID: task.AttemptID, EnvelopeDigest: task.EnvelopeDigest, Verdict: "pass", Outputs: map[string]ArtifactRef{port: {ArtifactID: slot.ArtifactID, Revision: slot.Revision, Digest: rawDigest(body)}}, EvidenceRefs: []any{}, EffectReceiptRefs: []any{}, Summary: "Typed fixture report"}
		raw, _ := json.Marshal(result)
		if _, err := e.SubmitSession(ctx, SessionSubmission{SchemaVersion: task.SchemaVersion, RunID: task.RunID, AttemptID: task.AttemptID, EnvelopeDigest: task.EnvelopeDigest, Result: raw, AnsweredQuestions: noQuestions(task)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.Drive(ctx, started.Receipt.RunID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, started.Receipt.RunID)
	m := f.Mapping
	m.CandidateStage = ""
	m.CandidateOutput = ""
	projection := LightProjection{SchemaVersion: LightProjectionVersion, Workflows: []LightWorkflowMapping{{WorkflowRef: r.WorkflowRef, Phases: []LightPhaseMapping{{Stage: "review", Key: "review", Label: "Ревью"}}, Reviews: []LightReviewMapping{m}}}}
	raw, _ := json.Marshal(projection)
	p, err := ParseLightProjection(raw)
	if err != nil {
		t.Fatal(err)
	}
	view, err := e.Light(ctx, NewLightCache(), p)
	if err != nil {
		t.Fatal(err)
	}
	reviews := view.Reviews[r.ID]
	if len(reviews) != 1 || reviews[0].State != "unavailable" || reviews[0].GateStatus != "blocked" || view.Cards[0].Lane != "attention" {
		t.Fatalf("native usable report became PASS: %+v", view)
	}
	if target := os.Getenv("PRIFLY_LIGHT_PERF_FIXTURE_DIR"); target != "" {
		if err := e.Close(); err != nil {
			t.Fatal(err)
		}
		if err := monitorCopyAuthority(filepath.Join(target, "typed-review"), e.Root); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(target, "typed-review-mapping.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
