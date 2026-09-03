package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

func contextContractObject(t *testing.T, value any) map[string]any {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatal(err)
	}
	return object
}

func TestContextPublicSchemasPreservePublishedBundles(t *testing.T) {
	for _, previous := range []struct {
		name, digest string
		content      []byte
	}{
		{"foundation", "sha256:4fda82e5908602a8df274a23981884682c38fb08bc465d8cc9a1dd27af3d9c42", publicContracts},
		{"core", "sha256:573e440951b857afc6b22e4b77a4c0db08a1a252bd9e8007b5bde300cff06441", corePublicContracts},
		{"choice", "sha256:7fcacd3aa4719606b3f7ec0d1395b20feabdd393f22f013e48178a13f38f9cd8", choiceContracts},
		{"invocations", "sha256:ff73ea6801148b60e077b20093b904b465ca298c3b233c106375ae2194654864", invocationPublicContracts},
		{"repeats", "sha256:50102a1139609120e763cb614ee86ea67843c0d5d1550f7193b3bae4055f0c06", repeatPublicContracts},
		{"contexts", "sha256:2597335349737522dbe334675e5f75b565e4ec15f0f95ef70d3c5ee6c8f6c9bf", contextPublicContracts},
		{"sessions", "sha256:931d1d538a3955f4ff4dc8e094b8ac7f2719093a340754f1340c5c4f59236b38", sessionPublicContracts},
		{"waivers", "sha256:20a5e050629bcc09c832801e55b83fb29e7d2628c809925f77a1d62717c3a834", waiverPublicContracts},
	} {
		if rawDigest(previous.content) != previous.digest {
			t.Fatalf("context extension changed the delivered %s bundle", previous.name)
		}
	}
	for _, published := range []struct {
		path, id string
		content  []byte
	}{
		{"../../schemas/core/contexts.schema.json", "urn:prifly:core-context-public:4", contextPublicContracts},
		{"../../schemas/core/sessions.schema.json", "urn:prifly:core-session-public:5", sessionPublicContracts},
		{"../../schemas/core/waivers.schema.json", "urn:prifly:core-waiver-public:6", waiverPublicContracts},
		{"../../schemas/core/parallel.schema.json", "urn:prifly:core-parallel-public:7", parallelPublicContracts},
	} {
		distributed, err := os.ReadFile(published.path)
		if err != nil || !bytes.Equal(distributed, published.content) {
			t.Fatalf("embedded and distributed schemas differ for %s: %v", published.id, err)
		}
		var bundle struct {
			ID string `json:"$id"`
		}
		if err := json.Unmarshal(distributed, &bundle); err != nil || bundle.ID != published.id {
			t.Fatalf("extension did not use its separate public contract identity: %s", published.id)
		}
	}
	if _, err := PublicSchema("ContextManifest"); err == nil {
		t.Fatal("the local transport replaced the baseline ContextManifest contract")
	}
	if _, err := flow.ProtocolSchema("ContextManifest"); err != nil {
		t.Fatal("baseline ContextManifest is no longer available", err)
	}
}

func TestContextPublicSchemasMatchActualStateAndReadViews(t *testing.T) {
	prior, options := emptyRuntime(t)
	definitions, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	configuration := prior.Config
	configuration.ConfigurationSchemaRef = builtinVersionRef(definitions, "core:schema/core-configuration", "2.0.0")
	configuration.Configuration.SchemaVersion = CoreContextConfigVersion
	configuration.Configuration.SemanticsProfile = flow.CoreProfile
	configuration.AdapterBindings["local_process"] = builtinVersionRef(definitions, "core:adapter/local-process", "2.0.0")
	writeRuntimeJSON(t, filepath.Join(prior.Root, "prifly.json"), configuration)
	if err := prior.Close(); err != nil {
		t.Fatal(err)
	}
	e, err := Open(prior.Root, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close() })
	preview, err := e.Preview(PreviewOptions{WorkflowFile: options.WorkflowFile, BriefFile: options.BriefFile})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	started, err := e.Start(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	view, err := e.View(ctx, started.Receipt.RunID)
	if err != nil {
		t.Fatal(err)
	}
	next, err := e.Next(ctx, started.Receipt.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Run.SchemaVersion != CoreWaiverStateVersion || view.SchemaVersion != CoreWaiverReadVersion || next.SchemaVersion != CoreWaiverNextVersion || preview.SchemaVersion != CoreWaiverPreviewVersion {
		t.Fatalf("the opted-in authority did not create the current DTOs: state=%s read=%s next=%s preview=%s", view.Run.SchemaVersion, view.SchemaVersion, next.SchemaVersion, preview.SchemaVersion)
	}
	values := map[string]any{
		"CoreRunStateV6": view.Run, "CoreRunViewV6": view, "CoreNextViewV6": next,
		"CorePreviewV6": preview, "CoreWorkflowInvocationV6": view.Run.Invocations[view.Run.RootInvocationID],
		"CoreConfigurationV2": e.Config.Configuration,
	}
	for name, value := range values {
		t.Run(name, func(t *testing.T) {
			if err := validatePublic(t, name, value); err != nil {
				t.Fatal("actual DTO rejected", err)
			}
			for _, mutation := range []string{"unknown", "wrong_version", "null_version"} {
				object := contextContractObject(t, value)
				switch mutation {
				case "unknown":
					object["unexpected_context_authority"] = true
				case "wrong_version":
					object["schema_version"] = "core-state/3"
				case "null_version":
					object["schema_version"] = nil
				}
				if validatePublic(t, name, object) == nil {
					t.Fatal("public context contract accepted", mutation)
				}
			}
		})
	}
	for _, name := range []string{"CoreRunState", "CoreRunStateV2", "CoreRunStateV3"} {
		if validatePublic(t, name, view.Run) == nil {
			t.Fatal("new context state masqueraded as a delivered contract", name)
		}
	}
	state := contextContractObject(t, view.Run)
	state["context_resources"] = []any{map[string]any{
		"ref":        flow.Ref{ID: "test:resource/empty", Version: "1.0.0", Digest: rawDigest(nil)},
		"raw_digest": rawDigest(nil), "byte_encoding": "utf8_text", "media_type": "text/plain", "bytes": "",
	}}
	if err := validatePublic(t, "CoreRunStateV6", state); err != nil {
		t.Fatal("empty exact resource bytes were not representable", err)
	}
	state["context_resources"].([]any)[0].(map[string]any)["bytes"] = nil
	if validatePublic(t, "CoreRunStateV6", state) == nil {
		t.Fatal("null resource bytes were confused with an empty source")
	}
	for _, field := range []string{"context_resources", "check_executions", "active_check_execution_id", "pending_acceptance"} {
		state := contextContractObject(t, view.Run)
		state[field] = nil
		if validatePublic(t, "CoreRunStateV6", state) == nil {
			t.Fatal("context state accepted present null", field)
		}
	}
}

func TestContextPublicTransportAndSourceContracts(t *testing.T) {
	e := artifactEngine(t)
	sourceTestFile(t, e, "source", []byte("source bytes"))
	artifact, snapshot := sourceTestImport(t, e, SourceImportOptions{Path: "source", Format: "blob"})
	definitions, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	var profile ContextProfile
	for _, definition := range definitions {
		if definition.Ref.ID == "core:context/local-json" {
			if err := decode(definition.Bytes, &profile); err != nil {
				t.Fatal(err)
			}
		}
	}
	// These are real DTO shapes with accepted artifact references. The worker
	// materialization and content contract of each slot are tested separately.
	input := LocalPort{Ref: snapshot.ContentRef, Path: ContextSourcePath(0)}
	transport := ContextManifest{
		SchemaVersion: "local-context/2", Inputs: map[string]LocalPort{"source": input}, Outputs: map[string]OutputSlot{}, Dependencies: []flow.Ref{},
		Manifest: &LocalPort{Ref: artifact.Ref(), Path: "context/manifest.json"}, Rendering: &LocalPort{Ref: artifact.Ref(), Path: "context/rendered.json"}, Sources: []LocalPort{input},
	}
	registry := RegistryFile{SchemaVersion: "3", Entries: []Definition{{
		Ref:  flow.Ref{ID: "test:resource/source", Version: "1.0.0", Digest: rawDigest([]byte("source bytes"))},
		Kind: "resource", Path: "source", ByteEncoding: "utf8_text", MediaType: "text/plain",
	}}}
	request := ContextRequest{
		SchemaVersion: ContextRequestVersion, SourceAdapterRef: snapshot.AdapterRef, Selector: "explicit/file", Format: "blob", MediaType: "application/octet-stream", MaxBytes: 1024, Reason: "Required source data.",
	}
	values := map[string]any{
		"LocalContextManifestV2": transport, "LocalRegistryV3": registry,
		"ContextProfile": profile, "SourceSnapshot": snapshot, "ContextRequest": request,
	}
	for name, value := range values {
		t.Run(name, func(t *testing.T) {
			if err := validatePublic(t, name, value); err != nil {
				t.Fatal("valid descriptor rejected", err)
			}
			for _, field := range []string{"schema_version", "unexpected_field"} {
				object := contextContractObject(t, value)
				object[field] = nil
				if validatePublic(t, name, object) == nil {
					t.Fatal("public descriptor accepted null/unknown field", field)
				}
			}
		})
	}
	for _, field := range []string{"manifest", "rendering", "sources"} {
		object := contextContractObject(t, transport)
		object[field] = nil
		if validatePublic(t, "LocalContextManifestV2", object) == nil {
			t.Fatal("transport accepted null", field)
		}
	}
	object := contextContractObject(t, transport)
	object["inputs"].(map[string]any)["source"].(map[string]any)["path"] = "inputs/source"
	if validatePublic(t, "LocalContextManifestV2", object) == nil {
		t.Fatal("transport accepted an extra unaccounted input copy")
	}
	for _, mutate := range []func(map[string]any){
		func(v map[string]any) { v["kind"] = "step" },
		func(v map[string]any) { v["byte_encoding"] = nil },
		func(v map[string]any) { delete(v, "media_type") },
		func(v map[string]any) { v["byte_encoding"] = "json" },
	} {
		object := contextContractObject(t, registry)
		mutate(object["entries"].([]any)[0].(map[string]any))
		if validatePublic(t, "LocalRegistryV3", object) == nil {
			t.Fatal("registry accepted an invalid representation declaration")
		}
	}
}

func TestContextPublicCheckContractsMatchClosedParsers(t *testing.T) {
	definition := flow.CheckDefinition{
		SchemaVersion: flow.CheckDefinitionVersion, ID: "test:check/content", Version: "1.0.0", Title: "Content check", Kind: "content", Claim: "content_valid",
		Executor: flow.Executor{AdapterRef: checkProtocolRef("core:adapter/local-process", 'a'), Operation: "check"},
	}
	if err := flow.ValidateCheckDefinition(definition); err != nil {
		t.Fatal(err)
	}
	for _, boundary := range []string{"workflow_input", "step_input", "step_output", "workflow_output", "step_result"} {
		request := checkRequestFixture(boundary)
		if err := ValidateCheckRequest(request); err != nil {
			t.Fatal(err)
		}
		if err := validatePublic(t, "CheckRequest", request); err != nil {
			t.Fatal(boundary, err)
		}
		if boundary == "workflow_input" {
			object := contextContractObject(t, request)
			object["stage_activation_id"] = "activation:invented"
			if validatePublic(t, "CheckRequest", object) == nil {
				t.Fatal("workflow input check invented a stage activation")
			}
		}
		if boundary != "step_output" && boundary != "step_result" {
			object := contextContractObject(t, request)
			object["producer_attempt_id"] = "attempt:unrelated"
			if validatePublic(t, "CheckRequest", object) == nil {
				t.Fatal("check boundary accepted an unrelated producer attempt", boundary)
			}
		}
	}
	request := checkRequestFixture("step_output")
	requestBytes := checkRequestBytes(t, request)
	report := CheckResult{SchemaVersion: CheckResultVersion, CheckID: request.CheckID, RunID: request.RunID, RequestDigest: rawDigest(requestBytes), Status: "pass", Summary: "Declared check passed.", Limitations: []string{}}
	if err := ValidateCheckResult(report, requestBytes); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]any{"CheckDefinition": definition, "CheckRequest": request, "CheckResult": report} {
		if err := validatePublic(t, name, value); err != nil {
			t.Fatal(name, err)
		}
		for _, field := range []string{"schema_version", "unexpected_field"} {
			object := contextContractObject(t, value)
			object[field] = nil
			if validatePublic(t, name, object) == nil {
				t.Fatal(name, "accepted null/unknown", field)
			}
		}
	}
	for field, value := range map[string]any{"subjects": nil, "candidate_ref": nil, "producer_attempt_id": nil, "port": nil} {
		object := contextContractObject(t, request)
		object[field] = value
		if validatePublic(t, "CheckRequest", object) == nil {
			t.Fatal("check request accepted an explicitly null field", field)
		}
	}
	object := contextContractObject(t, request)
	delete(object, "producer_attempt_id")
	if validatePublic(t, "CheckRequest", object) == nil {
		t.Fatal("output check lost its actual producer owner")
	}
	object = contextContractObject(t, definition)
	object["claim"] = "semantic_review"
	if validatePublic(t, "CheckDefinition", object) == nil {
		t.Fatal("a content check claimed semantic review")
	}
	object = contextContractObject(t, report)
	object["limitations"] = nil
	if validatePublic(t, "CheckResult", object) == nil {
		t.Fatal("null limitations were confused with an empty list")
	}
}

func TestContextPublicCheckExecutionLifecycle(t *testing.T) {
	// This is a DTO fixture, not an execution receipt or native checker run.
	request := checkRequestFixture("workflow_input")
	requestBytes := checkRequestBytes(t, request)
	observed := newClock().now()
	execution := CheckExecution{
		ID: request.CheckID, Request: request,
		RequestBytes: local.BlobRef{Digest: rawDigest(requestBytes), Size: int64(len(requestBytes))},
		Workspace:    "workspaces/check", Status: "pending", Admitted: observed, Deadline: observed, DispatchDeadline: observed,
		Context: ContextManifest{
			SchemaVersion: "local-context/2", Inputs: map[string]LocalPort{}, Outputs: map[string]OutputSlot{}, Dependencies: []flow.Ref{},
			Manifest: &LocalPort{Ref: request.ContextManifestRef, Path: "context/manifest.json"}, Rendering: &LocalPort{Ref: request.ContextManifestRef, Path: "context/rendered.json"},
		},
	}
	if err := validatePublic(t, "CheckExecution", execution); err != nil {
		t.Fatal("valid check DTO shape rejected", err)
	}
	for _, field := range []string{"dispatch", "started", "executor_end", "settled", "process", "process_outcome", "report", "report_bytes", "token_hash", "failure"} {
		object := contextContractObject(t, execution)
		object[field] = nil
		if validatePublic(t, "CheckExecution", object) == nil {
			t.Fatal("check execution accepted present null", field)
		}
	}
	for _, status := range []string{"completed", "failed", "cancelled"} {
		object := contextContractObject(t, execution)
		object["status"] = status
		if validatePublic(t, "CheckExecution", object) == nil {
			t.Fatal("terminal check execution did not require settlement", status)
		}
	}
	object := contextContractObject(t, execution)
	object["settled"] = observed
	if validatePublic(t, "CheckExecution", object) == nil {
		t.Fatal("pending check execution fabricated a settlement")
	}
	object["status"] = "completed"
	if validatePublic(t, "CheckExecution", object) == nil {
		t.Fatal("completed check execution did not require report and process evidence")
	}
	object = contextContractObject(t, execution)
	object["started"] = observed
	if validatePublic(t, "CheckExecution", object) == nil {
		t.Fatal("started check execution did not require process identity")
	}
	object = contextContractObject(t, execution)
	object["status"], object["dispatch"] = "dispatching", observed
	if err := validatePublic(t, "CheckExecution", object); err != nil {
		t.Fatal("metadata read required the redacted token hash", err)
	}
}

func TestContextPublicAcceptanceStatesFromNativeExecution(t *testing.T) {
	ctx := context.Background()
	e, options := acceptanceProject(t, []string{"workflow_input", "step_output", "step_result"}, "", "pass", false)
	preview, err := e.Preview(PreviewOptions{WorkflowFile: options.WorkflowFile, BriefFile: options.BriefFile})
	if err != nil || len(preview.CheckExecutors) != 2 {
		t.Fatal("preview omitted the two declared checker bindings", err)
	}
	if err := validatePublic(t, "CorePreviewV6", preview); err != nil {
		t.Fatal("actual checker preview rejected", err)
	}
	object := contextContractObject(t, preview)
	object["check_executors"] = nil
	if validatePublic(t, "CorePreviewV6", object) == nil {
		t.Fatal("preview accepted a present-null checker catalog")
	}
	started, err := e.Start(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	runID := started.Receipt.RunID
	// Every checkpoint is read from the real authority. Only the subsequent
	// negative wire cases mutate detached JSON copies, never a Run's history.
	checkpoint := func(label string) (Run, RunView, NextView) {
		t.Helper()
		r, _, err := e.load(ctx, runID)
		if err != nil {
			t.Fatal(label, err)
		}
		view, err := e.View(ctx, runID)
		if err != nil {
			t.Fatal(label, err)
		}
		next, err := e.Next(ctx, runID)
		if err != nil {
			t.Fatal(label, err)
		}
		for name, value := range map[string]any{"CoreRunStateV6": r, "CoreRunViewV6": view, "CoreNextViewV6": next} {
			if err := validatePublic(t, name, value); err != nil {
				t.Fatal(label, name, err)
			}
		}
		if r.PendingAcceptance != nil {
			if err := validatePublic(t, "PendingAcceptance", r.PendingAcceptance); err != nil {
				t.Fatal(label, "PendingAcceptance", err)
			}
		}
		return r, view, next
	}
	advance := func() {
		t.Helper()
		lock, err := e.driverLock(runID)
		if err != nil {
			t.Fatal(err)
		}
		defer lock.Close()
		r, view, err := e.load(ctx, runID)
		if err != nil {
			t.Fatal(err)
		}
		if err := e.driveAcceptance(ctx, r, view); err != nil {
			t.Fatal(err)
		}
	}
	initial, initialView, next := checkpoint("workflow input pending")
	if initial.PendingAcceptance == nil || initial.PendingAcceptance.Kind != "workflow_input" || len(initial.Steps) != 0 || len(initial.Attempts) != 0 || len(initial.Activations) != 0 || next.Action != "acceptance" || next.StageID != "" || next.InvocationID != initial.RootInvocationID || next.WorkID != initial.PendingAcceptance.ID {
		t.Fatal("input acceptance acquired a fabricated producer or activation")
	}
	for name, value := range map[string]any{"CoreRunStateV6": initial, "CoreRunViewV6": initialView} {
		object := contextContractObject(t, value)
		state := object
		if name == "CoreRunViewV6" {
			state = object["run"].(map[string]any)
		}
		state["pending_acceptance"] = nil
		if validatePublic(t, name, object) == nil {
			t.Fatal(name, "confused present null with an omitted pending boundary")
		}
	}
	advance()
	active, activeView, next := checkpoint("input checker admitted")
	check := active.CheckExecutions[active.ActiveCheckID]
	if check == nil || check.Status != "pending" || next.Action != "check" || next.WorkID != check.ID || next.StageID != "" || len(active.Attempts) != 0 {
		t.Fatal("checker admission did not expose its own active work identity")
	}
	for _, value := range []*CheckExecution{check, activeView.Run.CheckExecutions[check.ID]} {
		if err := validatePublic(t, "CheckExecution", value); err != nil {
			t.Fatal("admitted checker rejected", err)
		}
	}
	executeCheckExecution(t, e, runID, check.ID)
	settled, settledView, _ := checkpoint("input checker settled")
	completedCheck := settled.CheckExecutions[check.ID]
	if completedCheck.Status != "completed" || completedCheck.Report == nil || completedCheck.Report.Status != "pass" || completedCheck.Settled == nil || settled.ActiveCheckID != "" {
		t.Fatal("native checker did not produce a settled report")
	}
	for name, value := range map[string]any{"CheckExecution": completedCheck, "CheckRequest": completedCheck.Request, "CheckResult": completedCheck.Report} {
		if err := validatePublic(t, name, value); err != nil {
			t.Fatal("settled input checker", name, err)
		}
	}
	if completedCheck.TokenHash == "" || settledView.Run.CheckExecutions[check.ID].TokenHash != "" || settledView.Run.CheckExecutions[check.ID].Report.Summary != "" {
		t.Fatal("actual checker read view did not preserve redaction")
	}
	// A short native checker need not be slowed down to observe running: its
	// committed start cut contains the actual process identity and request.
	history, err := e.Store.Read(ctx, runID, 0, 1000)
	if err != nil || history.More {
		t.Fatal("checker history is incomplete", err)
	}
	foundRunning := false
	for _, event := range history.Events {
		if event.Type != "check.started" {
			continue
		}
		atStart, err := e.Store.ReadAt(ctx, runID, event.Cut, 0, 1)
		if err != nil {
			t.Fatal(err)
		}
		var running Run
		if err := decodeState(atStart.Snapshot.Data, &running); err != nil {
			t.Fatal(err)
		}
		execution := running.CheckExecutions[check.ID]
		if execution == nil || execution.Status != "running" || execution.Started == nil || execution.Process == nil || execution.Settled != nil || running.ActiveCheckID != check.ID {
			t.Fatal("start cut is not a real active checker state")
		}
		if err := validatePublic(t, "CoreRunStateV6", running); err != nil {
			t.Fatal("actual running state rejected", err)
		}
		if err := validatePublic(t, "CheckExecution", execution); err != nil {
			t.Fatal("actual running checker rejected", err)
		}
		foundRunning = true
	}
	if !foundRunning {
		t.Fatal("native execution recorded no checker start")
	}
	advance()                      // Accept the checked workflow input and expose the real Step.
	callActivateReady(t, e, runID) // The released workflow frontier has no activation yet.
	attempt := driverExecuteFirst(t, e, runID)
	producer, _, next := checkpoint("producer settled, result pending")
	pending := producer.PendingAcceptance
	if attempt.Status != "completed" || attempt.Settled == nil || attempt.Accepted != nil || pending == nil || pending.Kind != "step_result" || len(pending.PreparedArtifacts) != 1 || pending.CandidateRef == nil || len(pending.Checks) != 2 || next.Action != "acceptance" || next.StageID != "work" {
		t.Fatal("producer settlement did not retain a separately checked result")
	}
	for port, artifact := range pending.PreparedArtifacts {
		if artifact.Ref() != pending.Bindings[port] {
			t.Fatal("prepared output descriptor differs from its pending binding")
		}
		if _, _, err := e.Artifact(artifact.Ref()); !os.IsNotExist(err) {
			t.Fatal("contract fixture exposed a prepared output as accepted", err)
		}
	}
	contextAcceptanceWireNegatives(t, initial.PendingAcceptance, pending)
	acceptanceRunChecksThroughPassed(t, e, runID)
	passed, _, _ := checkpoint("result checks passed, acceptance pending")
	if passed.PendingAcceptance == nil || passed.PendingAcceptance.Status != "passed" || passed.PendingAcceptance.Checked == nil || passed.Attempts[attempt.ID].Accepted != nil {
		t.Fatal("passed check reports were confused with completed result acceptance")
	}
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatal(err)
	}
	completed, view, next := checkpoint("result accepted and workflow completed")
	if completed.Status != "completed" || completed.PendingAcceptance != nil || completed.ActiveCheckID != "" || len(completed.CheckExecutions) != 3 || completed.Attempts[attempt.ID].Accepted == nil || next.Action != "terminal" {
		t.Fatal("actual accepted result was not reflected in terminal read contracts")
	}
	for _, execution := range view.Run.CheckExecutions {
		if err := validatePublic(t, "CheckExecution", execution); err != nil {
			t.Fatal("final checker metadata rejected", err)
		}
		if execution.TokenHash != "" || execution.Report == nil || execution.Report.Summary != "" || len(execution.Report.Limitations) != 0 {
			t.Fatal("final metadata view exposed checker credentials or report text")
		}
	}
}

func contextAcceptanceWireNegatives(t *testing.T, input, result *PendingAcceptance) {
	t.Helper()
	// IDs/ref equality, port membership and exact content provenance are runtime
	// proofs. These cases exercise only the public wire's closed variants.
	for _, test := range []struct {
		name   string
		value  *PendingAcceptance
		mutate func(map[string]any)
	}{
		{"input_activation", input, func(v map[string]any) { v["stage_activation_id"] = result.ActivationID }},
		{"input_producer", input, func(v map[string]any) { v["producer_attempt_id"] = result.ProducerAttemptID }},
		{"input_candidate", input, func(v map[string]any) { v["candidate_ref"] = result.CandidateRef }},
		{"input_prepared", input, func(v map[string]any) { v["prepared_artifacts"] = result.PreparedArtifacts }},
		{"result_activation_missing", result, func(v map[string]any) { delete(v, "stage_activation_id") }},
		{"result_producer_missing", result, func(v map[string]any) { delete(v, "producer_attempt_id") }},
		{"result_candidate_missing", result, func(v map[string]any) { delete(v, "candidate_ref") }},
		{"result_prepared_null", result, func(v map[string]any) { v["prepared_artifacts"] = nil }},
		{"result_candidate_null", result, func(v map[string]any) { v["candidate_ref"] = nil }},
		{"checks_empty", input, func(v map[string]any) { v["checks"] = []any{} }},
		{"checks_null", input, func(v map[string]any) { v["checks"] = nil }},
		{"passed_without_checked", result, func(v map[string]any) { v["status"] = "passed" }},
		{"pending_with_checked", input, func(v map[string]any) { v["checked"] = result.Created }},
		{"unknown_owner_field", result, func(v map[string]any) { v["execution_envelope"] = map[string]any{} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := contextContractObject(t, test.value)
			test.mutate(value)
			if validatePublic(t, "PendingAcceptance", value) == nil {
				t.Fatal("pending acceptance admitted an invalid wire variant")
			}
		})
	}
	for _, test := range []struct {
		name   string
		value  *PendingAcceptance
		index  int
		mutate func(map[string]any)
	}{
		{"content_port_missing", input, 0, func(v map[string]any) { delete(v, "port") }},
		{"content_port_null", input, 0, func(v map[string]any) { v["port"] = nil }},
		{"content_port_invalid", input, 0, func(v map[string]any) { v["port"] = "invalid/port" }},
		{"content_subject_empty", input, 0, func(v map[string]any) { v["subjects"] = []any{} }},
		{"content_subject_null", input, 0, func(v map[string]any) { v["subjects"] = nil }},
		{"content_subject_multiple", input, 0, func(v map[string]any) {
			v["subjects"] = []ArtifactRef{input.Checks[0].Subjects[0], *result.CandidateRef}
		}},
		{"content_owner_boundary", input, 0, func(v map[string]any) { v["boundary"] = "step_input" }},
		{"check_invented_producer", input, 0, func(v map[string]any) { v["producer_attempt_id"] = result.ProducerAttemptID }},
		{"result_port_forbidden", result, 1, func(v map[string]any) { v["port"] = "report" }},
		{"result_subject_null", result, 1, func(v map[string]any) { v["subjects"] = nil }},
		{"result_subject_duplicate", result, 1, func(v map[string]any) {
			v["subjects"] = []ArtifactRef{result.Checks[1].Subjects[0], result.Checks[1].Subjects[0]}
		}},
		{"result_owner_boundary", result, 0, func(v map[string]any) { v["boundary"] = "workflow_output" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := contextContractObject(t, test.value)
			check := value["checks"].([]any)[test.index].(map[string]any)
			test.mutate(check)
			if validatePublic(t, "PendingAcceptance", value) == nil {
				t.Fatal("pending check admitted an invalid port/subject variant")
			}
		})
	}
}
