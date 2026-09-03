package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stenhigh/prifly/internal/flow"
)

func TestRepeatObservationMixedTreeAndHistoricalCut(t *testing.T) {
	e, inner, _, options := repeatFixture(t, "commit-pass", "succeeded", 3)
	inner["id"] = "test:workflow/observed-inner-repeat"
	innerRef := callRegister(t, e, inner, "workflows/observed-inner-repeat.json")
	wrapper := callClone(t, inner)
	wrapper["id"] = "test:workflow/observed-call-wrapper"
	choiceStages(wrapper)["work"] = callStage(innerRef, wrapper["inputs"].(map[string]any), "succeeded", "done")
	wrapperRef := callRegister(t, e, wrapper, "workflows/observed-call-wrapper.json")
	outer := callClone(t, inner)
	outer["id"] = "test:workflow/observed-outer-repeat"
	stage := choiceStages(outer)["work"].(map[string]any)
	stage["body_workflow_ref"], stage["max_iterations"] = wrapperRef, 2
	runID := choiceStart(t, e, outer, options)
	ctx := context.Background()
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	if r.Status != "completed" || len(r.Invocations) != 11 || len(r.Attempts) != 6 || r.ControlTransitions != 32 || driverObservedStarts(t, e) != 6 {
		t.Fatalf("mixed repeat/call tree: status=%s invocations=%d attempts=%d controls=%d", r.Status, len(r.Invocations), len(r.Attempts), r.ControlTransitions)
	}
	view, err := e.View(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePublic(t, "CoreRunViewV3", view); err != nil {
		t.Fatal(err)
	}
	// The tree contains every actual entity once, including historical bodies.
	seen := map[string]TimingNode{}
	var walk func(TimingNode)
	walk = func(node TimingNode) {
		if _, duplicate := seen[node.ID]; duplicate {
			t.Fatalf("timing duplicated historical entity %s", node.ID)
		}
		seen[node.ID] = node
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(view.Timing.Root)
	if len(seen) != 1+len(r.Invocations)+len(r.Activations)+len(r.Steps)+len(r.Attempts) || view.Timing.Root.AttemptCount != 6 {
		t.Fatal("timing omitted or double-counted a mixed-tree entity")
	}
	for _, invocation := range r.Invocations {
		if invocation.ID == r.RootInvocationID {
			continue
		}
		caller := seen[invocation.CallerActivationID]
		found := false
		for _, child := range caller.Children {
			found = found || child.ID == invocation.ID
		}
		if !found || caller.StageID != "work" || caller.Metrics["executor_time"].Quality != "not_applicable" {
			t.Fatal("child observation was attached to another same-named caller", invocation.ID)
		}
	}
	var executorMS int64
	for _, attempt := range r.Attempts {
		if attempt.Started == nil || attempt.ExecutorEnd == nil {
			t.Fatal("native worker lacks settled timing boundaries")
		}
		elapsed, reason := clockDelta(*attempt.Started, *attempt.ExecutorEnd)
		if reason != "" {
			t.Fatal("native worker changed clock domain", reason)
		}
		timingMeasured(t, seen[attempt.ID].Metrics["executor_time"], elapsed, false)
		executorMS += elapsed
	}
	timingMeasured(t, view.Timing.Root.Metrics["executor_sum"], executorMS, false)
	timingMeasured(t, view.Timing.Root.Metrics["executor_active_union"], executorMS, false)

	query := telemetryQuery("aggregate", "timing.executor_time")
	query.RunIDs, query.Filters.Scope = []string{runID}, []string{"attempt"}
	query.GroupBy = []string{"workflow_id", "step_id", "stage_id"}
	latest := telemetryReport(t, e, query)
	observedGroup := func(report TelemetryResponse) TelemetryAggregate {
		t.Helper()
		var found *TelemetryAggregate
		for _, group := range report.Aggregates {
			// The report may also describe the legacy instrument with no
			// matching observations; its revision must not enter this group.
			if group.N == 0 {
				continue
			}
			if found != nil || group.DescriptorID != "core:timing.executor_time/2" {
				t.Fatalf("mixed invocation timing groups: %+v", report.Aggregates)
			}
			copy := group
			found = &copy
		}
		if found == nil {
			t.Fatal("missing observed executor group")
		}
		return *found
	}
	group := observedGroup(latest)
	if group.N != 6 || group.Sum == nil || *group.Sum != float64(executorMS) || group.Dimensions["workflow_id"] != outer["id"] || group.Dimensions["stage_id"] != "work" || group.Dimensions["step_id"] != "test:step/driver" {
		t.Fatalf("same-definition workers were merged before aggregation: %+v", group)
	}
	read, _ := repeatHistory(t, e, runID)
	outerActivation := r.activationForInvocation(r.RootInvocationID, "work")
	var cut int64
	for _, event := range read.Events {
		if event.Type != "stage.repeat_decided" {
			continue
		}
		var decision RepeatDecision
		if err := json.Unmarshal(event.Data, &decision); err != nil {
			t.Fatal(err)
		}
		if decision.ActivationID == outerActivation.ID && decision.Iteration == 1 {
			cut = event.Cut
		}
	}
	if cut == 0 || cut >= latest.Cut {
		t.Fatal("fixture did not retain an intermediate outer-iteration cut")
	}
	query.Cut = &cut
	historical := telemetryReport(t, e, query)
	if p := historical.Population; p.Open != 1 || p.Terminal != 0 || p.Invocations != 7 || p.Attempts != 3 || p.SettledAttempts != 3 {
		t.Fatalf("later iterations leaked into the fixed-cut population: %+v", p)
	}
	if got := observedGroup(historical); got.N != 3 {
		t.Fatalf("historical aggregate included future workers: %+v", got)
	}
	before, err := json.Marshal(historical)
	if err != nil {
		t.Fatal(err)
	}
	// A separate reader has a new clock session. It must still use the exact
	// saved observations, even though the current Run completed more work.
	reopened, err := Open(e.Root, true)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	after, err := json.Marshal(telemetryReport(t, reopened, query))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("reopening changed the historical repeat report", err)
	}
	oldView, err := e.Store.ReadAt(ctx, runID, cut, 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	var old Run
	if err := decodeState(oldView.Snapshot.Data, &old); err != nil {
		t.Fatal(err)
	}
	tree := Timing(old, old.LastObserved, false)
	controller := timingFind(t, tree.Root, outerActivation.ID)
	if tree.Root.AttemptCount != 3 || len(controller.Children) != 2 || controller.AttemptCount != 3 {
		t.Fatal("historical timing read the current repeat frontier")
	}
}

// This is a pure calculator regression with explicit clock observations; the
// preceding test supplies actual driver evidence for the mixed invocation tree.
func TestRepeatObservationExactTimingAndHistoricalRestriction(t *testing.T) {
	r := invocationTimingFixture()
	r.SchemaVersion = CoreRepeatStateVersion
	controller := r.Activations["call"]
	controller.Kind, controller.Settled = "repeat", timingPoint(17000)
	controller.Repeat = &RepeatProgress{IterationCount: 2, CurrentBodyInvocationID: "sibling"}
	delete(r.Activations, "call-again")
	r.Invocations["child"].Iteration = telemetryPtr(int64(1))
	r.Invocations["sibling"].Iteration = telemetryPtr(int64(2))
	r.Invocations["sibling"].CallerActivationID = controller.ID
	controller.Repeat.LastDecision = &RepeatDecision{SchemaVersion: RepeatDecisionVersion, ID: "decision:sibling", RunID: r.ID, InvocationID: r.RootInvocationID, ActivationID: controller.ID, StageID: controller.StageID, BodyInvocationID: "sibling", Iteration: 2, BodyStatus: "completed", BodyOutcome: r.Outcome, UntilResult: "false", Inputs: []ChoiceInput{}, Route: "on_limit", NextStageID: "done", Observed: *controller.Settled}
	activation := *r.Activations["activation"]
	activation.ID, activation.InvocationID, activation.StepID = "second-activation", "sibling", "second-step"
	activation.Created, activation.Settled = timingObservation(16000), timingPoint(17000)
	r.Activations[activation.ID] = &activation
	step := *r.Steps["step"]
	step.ID, step.ActivationID, step.AttemptIDs = activation.StepID, activation.ID, []string{"second-attempt"}
	step.Created, step.Settled = activation.Created, activation.Settled
	r.Steps[step.ID] = &step
	attempt := *r.Attempts["attempt"]
	attempt.ID, attempt.StepID, attempt.ActivationID = "second-attempt", step.ID, activation.ID
	attempt.Admitted, attempt.Started = activation.Created, timingPoint(16000)
	attempt.ExecutorEnd, attempt.CandidateAt, attempt.Settled = timingPoint(16500), timingPoint(16600), activation.Settled
	r.Attempts[attempt.ID] = &attempt
	r.Stops = []Stop{{ID: "first-body-pause", Scope: "invocation", ScopeID: "child", Kind: "pause", Status: "released", Created: timingObservation(6000), Released: timingPoint(8000)}}
	before, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	tree := Timing(r, timingObservation(22000), false)
	repeat := timingFind(t, tree.Root, controller.ID)
	if len(repeat.Children) != 2 || repeat.AttemptCount != 2 || tree.Root.AttemptCount != 2 || repeat.Metrics["executor_time"].Quality != "not_applicable" {
		t.Fatal("repeat calculator retained only the current body or invented an executor")
	}
	timingMeasured(t, tree.Root.Metrics["executor_sum"], 8500, false)
	timingMeasured(t, repeat.Metrics["executor_active_union"], 8500, false)
	timingMeasured(t, repeat.Metrics["elapsed"], 16000, false)
	timingMeasured(t, timingFind(t, tree.Root, "child").Metrics["restricted_time"], 2000, false)
	timingMeasured(t, timingFind(t, tree.Root, "sibling").Metrics["restricted_time"], 0, false)
	timingMeasured(t, timingFind(t, tree.Root, "second-attempt").Metrics["executor_time"], 500, false)
	after, err := json.Marshal(r)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("timing mutated authoritative repeat observations", err)
	}
}

func TestRepeatObservationNativeHookNamespaces(t *testing.T) {
	e, workflow, body, options := repeatFixture(t, "commit-pass", "succeeded", 3)
	data, err := os.ReadFile(filepath.Join(e.Root, "steps/driver.json"))
	if err != nil {
		t.Fatal(err)
	}
	var step flow.StepDefinition
	if err := json.Unmarshal(data, &step); err != nil {
		t.Fatal(err)
	}
	hookSchema := []byte(`{"type":"object","properties":{"completed":{"type":"integer","minimum":0},"warning":{"type":"boolean"}},"additionalProperties":false}`)
	digest, err := flow.Digest(hookSchema)
	if err != nil {
		t.Fatal(err)
	}
	hookRef := flow.Ref{ID: "test:schema/repeat-observation", Version: "1.0.0", Digest: digest}
	if err := os.WriteFile(filepath.Join(e.Root, "schemas/repeat-observation.json"), hookSchema, 0600); err != nil {
		t.Fatal(err)
	}
	step.ID, step.SchemaVersion = "test:step/repeat-observation", "2"
	freshness := int64(30000)
	minimum := 0.0
	step.Hooks = map[string]flow.Hook{
		"progress_changed": {Kind: "state", SchemaRef: hookRef, Description: "Own iteration progress", Classification: "internal", ReadPolicy: "owner", MaxPayloadBytes: 1024, MaxCount: 4, MaxPerMinute: 4, FreshnessMS: &freshness},
		"warning_raised":   {Kind: "event", SchemaRef: hookRef, Description: "Own iteration warning", Classification: "internal", ReadPolicy: "owner", MaxPayloadBytes: 1024, MaxCount: 4, MaxPerMinute: 4},
	}
	step.Telemetry = []flow.Mapping{
		{Name: "processed_total", Revision: "1.0.0", Description: "Items per Attempt", Hook: "progress_changed", Kind: "counter", Field: "/completed", Unit: "1", Aggregation: "delta", Reset: "attempt", Minimum: &minimum, Dimensions: map[string]string{}},
		{Name: "quality_warnings", Revision: "1.0.0", Description: "Warnings per Attempt", Hook: "warning_raised", Kind: "diagnostic", Aggregation: "occurrences", Reset: "none", Severity: "warn", Code: "quality_warning", Message: "Repeat fixture warning", Dimensions: map[string]string{}},
	}
	data, err = canonical(step)
	if err != nil {
		t.Fatal(err)
	}
	stepRef := flow.Ref{ID: step.ID, Version: step.Version, Digest: rawDigest(data)}
	if err := os.WriteFile(filepath.Join(e.Root, "steps/repeat-observation.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	registryData, err := os.ReadFile(filepath.Join(e.Root, "definitions.json"))
	var registry RegistryFile
	if err != nil || json.Unmarshal(registryData, &registry) != nil {
		t.Fatal("read registry", err)
	}
	registry.Entries = append(registry.Entries, Definition{Ref: stepRef, Kind: "step", Path: "steps/repeat-observation.json"}, Definition{Ref: hookRef, Kind: "schema", Path: "schemas/repeat-observation.json"})
	writeRuntimeJSON(t, filepath.Join(e.Root, "definitions.json"), registry)
	worker := e.Config.Configuration.Executors["test:step/driver"]
	worker.Args = []string{"-test.run=^TestRepeatObservationWorkerHelper$"}
	worker.Environment = map[string]string{"REPEAT_OBSERVATION_HELPER": "1", "GORACE": "atexit_sleep_ms=0"}
	e.Config.Configuration.Executors[step.ID] = worker
	body["id"] = "test:workflow/repeat-observed-hooks"
	choiceStages(body)["work"].(map[string]any)["step_ref"] = stepRef
	bodyRef := callRegister(t, e, body, "workflows/repeat-observed-hooks.json")
	choiceStages(workflow)["work"].(map[string]any)["body_workflow_ref"] = bodyRef
	runID := choiceStart(t, e, workflow, options)
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, runID)
	if r.Status != "completed" || len(r.Attempts) != 3 || len(r.Publications) != 9 {
		t.Fatalf("actual hook workers did not complete: status=%s attempts=%d publications=%d diagnostics=%+v", r.Status, len(r.Attempts), len(r.Publications), r.Diagnostics)
	}
	for _, attempt := range r.Attempts {
		var status StepReadView
		data, err := os.ReadFile(filepath.Join(attempt.Workspace, "observed-status.json"))
		if err != nil || json.Unmarshal(data, &status) != nil {
			t.Fatal("worker did not record its actual scoped status", err)
		}
		progress, warning := status.Hooks["progress_changed"], status.Hooks["warning_raised"]
		if status.RunID != r.ID || status.AttemptID != attempt.ID || status.StepID != attempt.StepID || status.Frozen || progress.Count != 2 || progress.LatestState == nil || progress.LatestState.Version != 2 || progress.LatestState.AttemptID != attempt.ID || warning.Count != 1 {
			t.Fatalf("same-stage iterations shared publication state: %+v", status)
		}
		if attempt.Accepted == nil || attempt.Started == nil || attempt.ProcessOutcome == nil || !attempt.ProcessOutcome.GroupEmpty {
			t.Fatal("publication evidence did not come from an accepted, settled native worker")
		}
	}
	query := telemetryQuery("aggregate", "step.processed_total", "step.quality_warnings")
	query.RunIDs, query.Filters.StepID, query.GroupBy = []string{runID}, []string{step.ID}, []string{"step_id", "stage_id"}
	report := telemetryReport(t, e, query)
	counter := telemetryGroup(t, report, "step.processed_total")
	warnings := telemetryGroup(t, report, "step.quality_warnings")
	if counter.Total == nil || *counter.Total != 9 || counter.Delta == nil || *counter.Delta != 6 || counter.N != 3 || counter.Coverage.Observed != 6 || warnings.Total == nil || *warnings.Total != 3 {
		t.Fatalf("per-Attempt counters/events merged across iterations: counter=%+v warnings=%+v", counter, warnings)
	}
	if report.Population.FullWarningCoverage != 1 || report.Population.WarnedIncomplete != 0 || counter.Dimensions["stage_id"] != "work" || warnings.Dimensions["step_id"] != step.ID {
		t.Fatal("historical body definitions were not used for warning coverage and dimensions")
	}
	query.Mode, query.GroupBy, query.Limit = "records", nil, 100
	records := telemetryReport(t, e, query)
	seenWarnings := map[string]bool{}
	for _, record := range records.Records {
		if record.Metric != "step.quality_warnings" {
			continue
		}
		attempt := r.Attempts[record.Subject.AttemptID]
		if attempt == nil || record.Subject.ID != attempt.ID || record.Subject.StepInstanceID != attempt.StepID || seenWarnings[attempt.ID] {
			t.Fatal("warning lost exact historical Attempt provenance", record.Subject)
		}
		seenWarnings[attempt.ID] = true
	}
	if len(seenWarnings) != 3 {
		t.Fatal("warning event keys were deduplicated across independent iterations")
	}
}

// The fixture runs through the real process runner and the authenticated Unix
// API. It stores no token: only the public status response becomes test evidence.
func TestRepeatObservationWorkerHelper(t *testing.T) {
	if os.Getenv("REPEAT_OBSERVATION_HELPER") != "1" {
		return
	}
	var envelope struct {
		RunID     string `json:"run_id"`
		StepID    string `json:"step_instance_id"`
		AttemptID string `json:"attempt_id"`
	}
	if err := json.NewDecoder(os.Stdin).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	command := PublishCommand{SchemaVersion: "1", RunID: envelope.RunID, StepID: envelope.StepID, AttemptID: envelope.AttemptID, EnvelopeDigest: os.Getenv("PRIFLY_ENVELOPE_DIGEST"), Hook: "progress_changed", Kind: "state"}
	transport := &http.Transport{DisableKeepAlives: true, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", os.Getenv("PRIFLY_SOCKET"))
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	request := func(method, path string, body []byte) []byte {
		t.Helper()
		req, err := http.NewRequest(method, "http://local"+path, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+os.Getenv("PRIFLY_TOKEN"))
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		if err != nil || response.StatusCode != http.StatusOK {
			t.Fatalf("publication request status=%d body=%s err=%v", response.StatusCode, data, err)
		}
		return data
	}
	for index, count := range []int{1, 3} {
		version := int64(index)
		command.CommandID, command.ExpectedStateVersion = fmt.Sprintf("command:state-%d", index), &version
		command.Value = json.RawMessage(fmt.Sprintf(`{"completed":%d}`, count))
		data, err := json.Marshal(command)
		if err != nil {
			t.Fatal(err)
		}
		request(http.MethodPost, "/publish", data)
	}
	// Same logical key in each independent Attempt, plus a retry within each.
	command.Hook, command.Kind, command.EventKey, command.ExpectedStateVersion = "warning_raised", "event", "warning:shared-key", nil
	command.Value = json.RawMessage(`{"warning":true}`)
	for index := 0; index < 2; index++ {
		command.CommandID = fmt.Sprintf("command:warning-%d", index)
		data, err := json.Marshal(command)
		if err != nil {
			t.Fatal(err)
		}
		request(http.MethodPost, "/publish", data)
	}
	query := url.Values{"run_id": {command.RunID}, "step_instance_id": {command.StepID}, "attempt_id": {command.AttemptID}, "envelope_digest": {command.EnvelopeDigest}}
	status := request(http.MethodGet, "/status?"+query.Encode(), nil)
	if err := os.WriteFile("observed-status.json", status, 0600); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(os.Getenv("PRIFLY_CONTEXT_FILE"))
	var manifest ContextManifest
	if err != nil || json.Unmarshal(data, &manifest) != nil {
		t.Fatal("read context", err)
	}
	output := []byte("accepted hook observations\n")
	slot := manifest.Outputs["report"]
	if err := os.WriteFile(slot.Path, output, 0600); err != nil {
		t.Fatal(err)
	}
	result := Result{SchemaVersion: "1", RunID: command.RunID, StepInstanceID: command.StepID, AttemptID: command.AttemptID, EnvelopeDigest: command.EnvelopeDigest, Verdict: "pass", Outputs: map[string]ArtifactRef{"report": {ArtifactID: slot.ArtifactID, Revision: slot.Revision, Digest: rawDigest(output)}}, EvidenceRefs: []any{}, EffectReceiptRefs: []any{}, Summary: "repeat publication fixture"}
	fd := os.NewFile(3, "result")
	if err := json.NewEncoder(fd).Encode(result); err != nil {
		t.Fatal(err)
	}
	if err := fd.Close(); err != nil {
		t.Fatal(err)
	}
}
