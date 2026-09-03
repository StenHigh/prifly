package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

func invocationTimingFixture() Run {
	r := timingFixture()
	r.SchemaVersion, r.Profile = CoreInvocationStateVersion, flow.CoreProfile
	r.Settled, r.LastObserved = timingPoint(20000), timingObservation(20000)
	r.Invocations = map[string]*Invocation{
		r.RootInvocationID: {ID: r.RootInvocationID, RunID: r.ID, Status: "completed", Outcome: r.Outcome, Created: r.Created, Settled: r.Settled},
		"child":            {ID: "child", RunID: r.ID, ParentInvocationID: r.RootInvocationID, CallerActivationID: "call", Status: "completed", Outcome: r.Outcome, Created: timingObservation(1000), Settled: timingPoint(15000)},
		"sibling":          {ID: "sibling", RunID: r.ID, ParentInvocationID: r.RootInvocationID, CallerActivationID: "call-again", Status: "completed", Outcome: r.Outcome, Created: timingObservation(16000), Settled: timingPoint(17000)},
	}
	r.Activations["activation"].InvocationID = "child"
	r.Activations["activation"].Created = timingObservation(1000)
	r.Steps["step"].Created = timingObservation(1000)
	r.Activations["finish"].Created, r.Activations["finish"].Settled = timingObservation(20000), r.Settled
	r.Activations["call"] = &Activation{ID: "call", StageID: "check", InvocationID: r.RootInvocationID, Kind: "call", Status: "completed", Created: timingObservation(1000), Settled: timingPoint(15000)}
	r.Activations["call-again"] = &Activation{ID: "call-again", StageID: "check-again", InvocationID: r.RootInvocationID, Kind: "call", Status: "completed", Created: timingObservation(16000), Settled: timingPoint(17000)}
	r.Activations["child-finish"] = &Activation{ID: "child-finish", StageID: "done", InvocationID: "child", Kind: "finish", Status: "completed", Created: timingObservation(15000), Settled: timingPoint(15000)}
	r.Activations["sibling-finish"] = &Activation{ID: "sibling-finish", StageID: "done", InvocationID: "sibling", Kind: "finish", Status: "completed", Created: timingObservation(17000), Settled: timingPoint(17000)}
	for i := range r.Transitions {
		change := &r.Transitions[i]
		if change.Kind == "run" && change.To == "completed" || change.ID == "finish" {
			change.At = timingObservation(20000)
		} else if change.At.MonotonicMS == 0 && change.Kind != "run" {
			change.At = timingObservation(1000)
		}
	}
	r.Transitions = append(r.Transitions,
		StateChange{Kind: "invocation", ID: r.RootInvocationID, To: "running", At: r.Created},
		StateChange{Kind: "invocation", ID: r.RootInvocationID, From: "running", To: "completed", At: *r.Settled},
		StateChange{Kind: "invocation", ID: "child", To: "running", At: timingObservation(1000)},
		StateChange{Kind: "invocation", ID: "child", From: "running", To: "waiting", At: timingObservation(6000)},
		StateChange{Kind: "invocation", ID: "child", From: "waiting", To: "running", At: timingObservation(8000)},
		StateChange{Kind: "invocation", ID: "child", From: "running", To: "completed", At: timingObservation(15000)},
	)
	sort.SliceStable(r.Transitions, func(i, j int) bool { return r.Transitions[i].At.MonotonicMS < r.Transitions[j].At.MonotonicMS })
	return r
}

func TestInvocationTimingTreeAndNoDoubleCounting(t *testing.T) {
	r := invocationTimingFixture()
	before, _ := json.Marshal(r)
	report := Timing(r, timingObservation(22000), false)
	if report.CalculatorRevision != TimingCalculatorRevisionCore {
		t.Fatal("nested timing used the legacy calculator")
	}
	call := timingFind(t, report.Root, "call")
	child := timingFind(t, report.Root, "child")
	if len(call.Children) != 1 || call.Children[0].ID != child.ID || child.Kind != "workflow_invocation" || call.Metrics["executor_time"].Quality != "not_applicable" {
		t.Fatalf("call did not own the child invocation: %+v", call)
	}
	if report.Root.AttemptCount != 1 || call.AttemptCount != 1 || child.AttemptCount != 1 {
		t.Fatal("one leaf Attempt was counted more than once")
	}
	timingMeasured(t, report.Root.Metrics["elapsed"], 20000, false)
	timingMeasured(t, child.Metrics["elapsed"], 14000, false)
	timingMeasured(t, child.StateTime["waiting"], 2000, false)
	timingMeasured(t, report.Root.Metrics["executor_sum"], 8000, false)
	timingMeasured(t, report.Root.Metrics["executor_active_union"], 8000, false)
	timingMeasured(t, timingFind(t, report.Root, "sibling").Metrics["elapsed"], 1000, false)
	if got := child.Intervals[0]; got.FromRef.Kind != "workflow_invocation" || got.FromRef.ID != child.ID || got.ToRef.ID != child.ID {
		t.Fatalf("child boundaries were attributed to its Run: %+v", got)
	}
	after, _ := json.Marshal(r)
	if !bytes.Equal(before, after) {
		t.Fatal("timing changed authoritative invocation state")
	}
	a, _ := json.Marshal(report)
	b, _ := json.Marshal(Timing(r, timingObservation(22000), false))
	if !bytes.Equal(a, b) {
		t.Fatal("fixed-cut nested timing is nondeterministic")
	}
}

func TestInvocationTimingRestrictionsStayInScope(t *testing.T) {
	for _, scope := range []string{"child", "invocation"} {
		t.Run(scope, func(t *testing.T) {
			r := invocationTimingFixture()
			r.Stops = []Stop{{ID: "stop", Scope: "invocation", ScopeID: scope, Kind: "pause", Status: "released", Created: timingObservation(6000), Released: timingPoint(8000)}}
			report := Timing(r, *r.Settled, false)
			for _, id := range []string{"child", "activation", "step", "attempt"} {
				timingMeasured(t, timingFind(t, report.Root, id).Metrics["restricted_time"], 2000, false)
			}
			timingMeasured(t, report.Root.Metrics["restricted_time"], 0, false)
			timingMeasured(t, timingFind(t, report.Root, "sibling").Metrics["restricted_time"], 0, false)
			rootRestriction := int64(0)
			if scope == r.RootInvocationID {
				rootRestriction = 2000
			}
			timingMeasured(t, timingFind(t, report.Root, r.RootInvocationID).Metrics["restricted_time"], rootRestriction, false)
		})
	}
}

func TestInvocationTimingCancelledChildDoesNotKeepElapsedOpen(t *testing.T) {
	r := invocationTimingFixture()
	r.Status, r.Outcome, r.Settled = "waiting", nil, nil
	r.Invocations[r.RootInvocationID].Status, r.Invocations[r.RootInvocationID].Outcome, r.Invocations[r.RootInvocationID].Settled = "waiting", nil, nil
	r.Invocations["child"].Status, r.Invocations["child"].Outcome = "cancelled", nil
	r.Activations["call"].Status, r.Activations["call"].Settled = "waiting", nil
	r.Transitions = nil
	delete(r.Invocations, "sibling")
	for _, id := range []string{"finish", "call-again", "sibling-finish"} {
		delete(r.Activations, id)
	}
	r.Stops = []Stop{{ID: "cancel-child", Scope: "invocation", ScopeID: "child", Kind: "cancel", Status: "active", Created: timingObservation(6000)}}
	report := Timing(r, timingObservation(20000), false)
	child := timingFind(t, report.Root, "child")
	timingMeasured(t, child.Metrics["elapsed"], 14000, false)
	timingMeasured(t, child.Metrics["cancel_to_settlement"], 9000, false)
	if !timingFind(t, report.Root, "call").Metrics["elapsed"].IsOpen || report.Root.Metrics["cancel_to_settlement"].Quality != "not_applicable" {
		t.Fatal("child cancellation settled or cancelled the caller's scope")
	}
}

func TestInvocationTimingLegacyProjectionIsUnchanged(t *testing.T) {
	r := timingFixture()
	before, _ := json.Marshal(Timing(r, timingObservation(20000), false))
	// New in-memory fields cannot reinterpret the history of a legacy state.
	r.Invocations = invocationTimingFixture().Invocations
	after, _ := json.Marshal(Timing(r, timingObservation(20000), false))
	if !bytes.Equal(before, after) {
		t.Fatal("legacy state was reinterpreted as an invocation tree")
	}
}

// Reuse the existing authenticated publication fixture, replacing its root
// definition with a caller. These are seeded facts for observation tests, not
// evidence that a real child process ran; driver integration tests cover that.
func invocationPublicationFixture(t *testing.T) (*Engine, string, PublishCommand, Run, int64) {
	t.Helper()
	e, token, command := publicationFixture(t, nil)
	r, view := publicationRun(t, e, command)
	childRef := r.WorkflowRef
	r.Definitions = append(r.Definitions, PinnedDefinition{Ref: childRef, Kind: "workflow", RawDigest: rawDigest(r.Workflow), Bytes: r.Workflow})
	definitions, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	policyRef := builtinVersionRef(definitions, "core:policy/local", "2.0.0")
	for _, definition := range definitions {
		if definition.Ref == policyRef {
			r.Definitions = append(r.Definitions, definition)
		}
	}
	w := flow.WorkflowRevision{SchemaVersion: "1", ID: "test:workflow/observed-call", Version: "1.0.0", Title: "Observe a child", Inputs: map[string]flow.InputPort{}, Outputs: map[string]flow.OutputPort{}, AllowedOutcomes: []string{"succeeded"}, PolicyRef: policyRef, Limits: flow.Limits{MaxStepInstances: 1, MaxControlTransitions: 8, MaxParallelism: 1, MaxChildDepth: 1}}
	w.Definition.Entry = "report"
	w.Definition.Stages = map[string]flow.Stage{
		"report": {Kind: "call", WorkflowRef: childRef, InputBindings: map[string]flow.Binding{}, On: map[string]string{"succeeded": "done"}},
		"done":   {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]flow.Binding{}},
	}
	r.Workflow, err = canonical(w)
	if err != nil {
		t.Fatal(err)
	}
	r.ID, r.RootInvocationID = "run:child-observability", "invocation:root-observability"
	r.SchemaVersion, r.Profile = CoreInvocationStateVersion, flow.CoreProfile
	r.WorkflowRef = flow.Ref{ID: w.ID, Version: w.Version, Digest: rawDigest(r.Workflow)}
	r.EffectiveConfiguration = &EffectiveConfiguration{SchemaVersion: "effective-configuration/1", WorkflowRef: r.WorkflowRef, Inputs: map[string]ConfigurationValue{}}
	r.WorkflowConfigurations = map[string]*EffectiveConfiguration{r.WorkflowRef.Digest: r.EffectiveConfiguration, childRef.Digest: {SchemaVersion: "effective-configuration/1", WorkflowRef: childRef, Inputs: map[string]ConfigurationValue{}}}
	r.Ready, r.Transitions = nil, nil
	r.Inputs, r.Outputs = map[string]ArtifactRef{}, map[string]ArtifactRef{}
	childID, callerID := "invocation:child-observability", "activation:caller"
	r.Activations[callerID] = &Activation{ID: callerID, StageID: "report", InvocationID: r.RootInvocationID, Kind: "call", Status: "waiting", Created: r.Created}
	r.Activations["activation:one"].InvocationID = childID
	r.Invocations = map[string]*Invocation{
		r.RootInvocationID: {ID: r.RootInvocationID, RunID: r.ID, WorkflowRef: r.WorkflowRef, Status: "running", Inputs: r.Inputs, Outputs: r.Outputs, Ready: []string{}, Created: r.Created, ControlTransitions: 2, StepInstances: 1},
		childID:            {ID: childID, RunID: r.ID, ParentInvocationID: r.RootInvocationID, CallerActivationID: callerID, WorkflowRef: childRef, Status: "running", Inputs: map[string]ArtifactRef{}, Outputs: map[string]ArtifactRef{}, Ready: []string{}, Created: r.Created, ControlTransitions: 1, StepInstances: 1},
	}
	r.ControlTransitions = 2
	if _, err := r.plan(); err != nil {
		t.Fatal(err)
	}
	command.RunID = r.ID
	return e, token, command, r, view.Cut
}

func TestInvocationPublicationUsesChildHookAndStopScope(t *testing.T) {
	e, token, command, r, _ := invocationPublicationFixture(t)
	telemetrySaveRun(t, e, r, 0)
	ctx := context.Background()
	status, err := e.publisherStatus(ctx, token, command)
	if err != nil || len(status.Hooks) != 3 || status.Restricted {
		t.Fatalf("child hooks were resolved from the caller: %+v %v", status, err)
	}
	if _, err := e.Publish(ctx, token, command); err != nil {
		t.Fatal(err)
	}
	changePublicationRun(t, e, command, func(r *Run, obs Observation) {
		r.Stops = append(r.Stops, Stop{ID: "stop:child", Kind: "pause", Status: "active", Scope: "invocation", ScopeID: "invocation:child-observability", Created: obs})
	})
	status, err = e.publisherStatus(ctx, token, command)
	if err != nil || !status.Restricted || status.Hooks["progress_changed"].Count != 1 {
		t.Fatalf("child stop/publication missing from own status: %+v %v", status, err)
	}
	warning := command
	warning.CommandID, warning.Hook, warning.Kind, warning.EventKey, warning.ExpectedStateVersion = "command:child-warning", "warning_raised", "event", "warning:one", nil
	if _, err := e.Publish(ctx, token, warning); publicationErrorCode(err) != "publication_restricted" {
		t.Fatalf("child stop did not fence its declared hook: %v", err)
	}
	command.CommandID, command.ExpectedStateVersion = "command:child-stopped-progress", telemetryPtr(int64(1))
	if _, err := e.Publish(ctx, token, command); err != nil {
		t.Fatalf("allowed own progress stopped working in a child: %v", err)
	}
	command.CommandID, command.StepID = "command:wrong-child", "step:someone-else"
	if _, err := e.Publish(ctx, token, command); publicationErrorCode(err) != "publisher_forbidden" {
		t.Fatalf("child token acquired sibling authority: %v", err)
	}
}

func completeInvocationObservationFixture(r *Run) {
	r.Status, r.Outcome = "completed", telemetryPtr("succeeded")
	r.Created, r.LastObserved, r.Settled = timingObservation(0), timingObservation(20000), timingPoint(20000)
	r.Active = nil
	root := r.Invocations[r.RootInvocationID]
	root.Status, root.Outcome, root.Created, root.Settled = r.Status, r.Outcome, r.Created, r.Settled
	child := r.Invocations["invocation:child-observability"]
	child.Status, child.Outcome, child.Created, child.Settled = r.Status, r.Outcome, timingObservation(5000), timingPoint(15000)
	caller := r.Activations[child.CallerActivationID]
	caller.Status, caller.Created, caller.Settled = "completed", timingObservation(3000), child.Settled
	activation, step, attempt := r.Activations["activation:one"], r.Steps["step:one"], r.Attempts["attempt:one"]
	activation.Status, activation.Created, activation.Settled = "completed", child.Created, timingPoint(14000)
	step.Status, step.Verdict, step.Created, step.Settled = "completed", "pass", child.Created, activation.Settled
	attempt.Status, attempt.Admitted, attempt.Dispatch, attempt.Started = "completed", timingObservation(6000), timingPoint(6000), timingPoint(7000)
	attempt.ExecutorEnd, attempt.CandidateAt, attempt.Settled = timingPoint(12000), timingPoint(13000), step.Settled
	attempt.Accepted = &Result{RunID: r.ID, StepInstanceID: step.ID, AttemptID: attempt.ID, Verdict: "pass"}
	attempt.ProcessOutcome = &local.ProcessOutcome{Started: true, WaitReturned: true, GroupEmpty: true, ExitCode: telemetryPtr(0)}
	r.Activations["activation:child-finish"] = &Activation{ID: "activation:child-finish", StageID: "done", InvocationID: child.ID, Kind: "finish", Status: "completed", Created: *child.Settled, Settled: child.Settled}
	r.Activations["activation:root-finish"] = &Activation{ID: "activation:root-finish", StageID: "done", InvocationID: root.ID, Kind: "finish", Status: "completed", Created: *r.Settled, Settled: r.Settled}
	r.Publications = []Publication{{ID: "publication:child-warning", AttemptID: attempt.ID, StepID: step.ID, Hook: "warning_raised", Kind: "event", EventKey: "warning:child", Value: json.RawMessage(`{"phase":"working"}`), Received: timingObservation(10000), Actor: "publisher"}}
}

func TestInvocationTelemetryChildPopulationAndHistoricalCut(t *testing.T) {
	e, _, command, r, oldCut := invocationPublicationFixture(t)
	oldQuery := telemetryQuery("records", "core.entities_created", "timing.elapsed", "step.quality_warnings")
	oldQuery.Cut, oldQuery.Limit = &oldCut, 1000
	oldReport := telemetryReport(t, e, oldQuery)
	if oldReport.CalculatorRevision != TelemetryCalculatorRevision || oldReport.TimingRevision != "foundation-timing/1" {
		t.Fatal("legacy cut did not use the legacy calculator")
	}
	before, _ := json.Marshal(oldReport)
	completeInvocationObservationFixture(&r)
	cut := telemetrySaveRun(t, e, r, 0)
	q := oldQuery
	q.Cut, q.RunIDs = &cut, []string{r.ID}
	report := telemetryReport(t, e, q)
	if report.CalculatorRevision != TelemetryCalculatorRevisionInvocation || report.TimingRevision != TimingCalculatorRevisionCore {
		t.Fatalf("wrong nested calculator revisions: %s %s", report.CalculatorRevision, report.TimingRevision)
	}
	if p := report.Population; p.Matched != 1 || p.Invocations != 2 || p.Activations != 4 || p.Steps != 1 || p.Attempts != 1 || p.FullWarningCoverage != 1 || p.WarnedIncomplete != 0 {
		t.Fatalf("actual child entities or declared warning coverage were lost: %+v", p)
	}
	find := func(metric, subject string) TelemetryRecord {
		t.Helper()
		var found *TelemetryRecord
		for _, record := range report.Records {
			if record.Metric == metric && record.Subject.ID == subject {
				if found != nil {
					t.Fatalf("duplicate %s record for %s", metric, subject)
				}
				copy := record
				found = &copy
			}
		}
		if found == nil {
			t.Fatalf("missing %s for %s", metric, subject)
		}
		return *found
	}
	child := r.Invocations["invocation:child-observability"]
	created := find("core.entities_created", child.ID)
	if created.Subject.Kind != "workflow_invocation" || created.Observed == nil || *created.Observed != child.Created {
		t.Fatalf("child creation used the Run's timestamp: %+v", created)
	}
	elapsed := find("timing.elapsed", child.ID)
	if elapsed.Value == nil || *elapsed.Value != 10000 || elapsed.DescriptorID != "core:timing.elapsed/2" || !strings.HasPrefix(elapsed.Method, TimingCalculatorRevisionCore+"/") {
		t.Fatalf("wrong child timing: %+v", elapsed)
	}
	warning := find("step.quality_warnings", command.AttemptID)
	if warning.Integer == nil || *warning.Integer != 1 || warning.Dimensions["step_id"] != r.Steps[command.StepID].Ref.ID || warning.Dimensions["stage_id"] != "report" || warning.Dimensions["workflow_id"] != r.WorkflowRef.ID {
		t.Fatalf("warning was not scoped to the child Step/root Run cohort: %+v", warning)
	}
	descriptorFound := false
	for _, descriptor := range report.Descriptors {
		if descriptor.ID == warning.DescriptorID {
			descriptorFound = descriptor.DefinitionRef != nil && *descriptor.DefinitionRef == r.Steps[command.StepID].Ref
		}
	}
	if !descriptorFound {
		t.Fatal("child warning mapping lost its exact definition reference")
	}
	after, _ := json.Marshal(telemetryReport(t, e, oldQuery))
	if !bytes.Equal(before, after) {
		t.Fatal("a later invocation Run changed a legacy fixed-cut report")
	}
}

func TestInvocationTelemetryMixedCalculatorGroupsStaySeparate(t *testing.T) {
	e, _, command, r, _ := invocationPublicationFixture(t)
	completeInvocationObservationFixture(&r)
	telemetrySaveRun(t, e, r, 0)
	command.RunID = "run:publications"
	base, _ := publicationRun(t, e, command)
	legacy := telemetryHistoryRun(t, base, "zzz-legacy-core", "completed", 10000)
	legacy.SchemaVersion, legacy.Profile = CoreStateVersion, flow.CoreProfile
	legacy.EffectiveConfiguration = &EffectiveConfiguration{SchemaVersion: "effective-configuration/1", WorkflowRef: legacy.WorkflowRef, Inputs: map[string]ConfigurationValue{}}
	telemetrySaveRun(t, e, legacy, 0)
	q := telemetryQuery("aggregate", "timing.elapsed")
	q.RunIDs, q.Filters.Scope = []string{r.ID, legacy.ID}, []string{"run"}
	report := telemetryReport(t, e, q)
	if report.CalculatorRevision != TelemetryCalculatorRevisionInvocation || report.TimingRevision != TimingCalculatorRevisionCore || report.Population.Invocations != 3 || len(report.Aggregates) != 2 {
		t.Fatalf("mixed versions were downgraded or merged: %+v", report)
	}
	values := map[string]float64{}
	for _, group := range report.Aggregates {
		if group.N != 1 || group.Sum == nil {
			t.Fatalf("mixed timing descriptor populations: %+v", group)
		}
		values[group.DescriptorID] = *group.Sum
	}
	if values["core:timing.elapsed/1"] != 10000 || values["core:timing.elapsed/2"] != 20000 {
		t.Fatalf("calculator revisions were not kept separate: %+v", values)
	}
	q.RunIDs = []string{legacy.ID}
	if got := telemetryReport(t, e, q); got.CalculatorRevision != TelemetryCalculatorRevisionCore || got.TimingRevision != "foundation-timing/1" {
		t.Fatal("an unrelated invocation Run changed the selected legacy cohort's revision")
	}
}

func TestInvocationTelemetryRejectsInvalidChildOwnership(t *testing.T) {
	e, _, _, r, _ := invocationPublicationFixture(t)
	r.Activations["activation:caller"].InvocationID = "invocation:child-observability"
	telemetrySaveRun(t, e, r, 0)
	q := telemetryQuery("records", "timing.elapsed")
	q.RunIDs = []string{r.ID}
	if _, err := e.Telemetry(context.Background(), q); !errors.Is(err, local.ErrIntegrity) {
		t.Fatalf("invalid caller identity produced a root-plan fallback: %v", err)
	}
}
