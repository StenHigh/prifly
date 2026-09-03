package runtime

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

func publicationFixture(t *testing.T, change func(*flow.StepDefinition)) (*Engine, string, PublishCommand) {
	t.Helper()
	e := artifactEngine(t)
	defs, reg, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	pin := func(kind, id string, value any) flow.Ref {
		b, err := canonical(value)
		if err != nil {
			t.Fatal(err)
		}
		ref := flow.Ref{ID: id, Version: "1.0.0", Digest: rawDigest(b)}
		defs = append(defs, PinnedDefinition{Ref: ref, Kind: kind, RawDigest: rawDigest(b), Bytes: b})
		reg[ref] = b
		return ref
	}
	progress := pin("schema", "test:schema/progress", json.RawMessage(`{"type":"object","properties":{"phase":{"type":"string","enum":["working","finished"]},"completed":{"type":"integer","minimum":0},"note":{"type":"string"},"status":{"enum":["completed","approved"]}},"required":["phase"],"additionalProperties":false}`))
	ready := pin("schema", "test:schema/ready", json.RawMessage(`{"type":"boolean"}`))
	step := flow.StepDefinition{SchemaVersion: "2", ID: "test:step/progress", Version: "1.0.0", Title: "Report progress", Kind: "command", Inputs: map[string]flow.InputPort{}, Outputs: map[string]flow.OutputPort{}, ContextRefs: []flow.Ref{}, RequiredCapabilities: []string{}, ResultCheckRefs: []flow.Ref{}, ResultSchemaRef: builtinRef(defs, "core:schema/step-result")}
	step.Executor.AdapterRef, step.Executor.Operation = builtinRef(defs, "core:adapter/local-process"), "process"
	step.Effects.Class, step.Effects.RetryClass = "none", "pure"
	freshness := int64(30000)
	step.Hooks = map[string]flow.Hook{
		"progress_changed": {Kind: "state", SchemaRef: progress, Description: "Current progress", Classification: "internal", ReadPolicy: "owner", MaxPayloadBytes: 65536, MaxCount: 100, MaxPerMinute: 60, AllowDuringStop: true, FreshnessMS: &freshness},
		"warning_raised":   {Kind: "event", SchemaRef: progress, Description: "Quality warning", Classification: "internal", ReadPolicy: "owner", MaxPayloadBytes: 65536, MaxCount: 100, MaxPerMinute: 60},
		"ready_changed":    {Kind: "state", SchemaRef: ready, Description: "Own readiness observation", Classification: "internal", ReadPolicy: "owner", MaxPayloadBytes: 16, MaxCount: 10, MaxPerMinute: 10, FreshnessMS: &freshness},
	}
	minimum, maximum := 0.0, 1000.0
	step.Telemetry = []flow.Mapping{
		{Name: "processed_total", Revision: "1.0.0", Description: "Cumulative items", Hook: "progress_changed", Kind: "counter", Field: "/completed", Unit: "1", Aggregation: "delta", Reset: "attempt", Minimum: &minimum, Maximum: &maximum, Dimensions: map[string]string{"phase": "/phase"}},
		{Name: "quality_warnings", Revision: "1.0.0", Description: "Quality warnings", Hook: "warning_raised", Kind: "diagnostic", Aggregation: "occurrences", Reset: "none", Severity: "warn", Code: "quality_warning", Message: "Declared quality warning", Dimensions: map[string]string{}},
	}
	if change != nil {
		change(&step)
	}
	stepRef := pin("step", step.ID, step)
	workflow := flow.WorkflowRevision{SchemaVersion: "1", ID: "test:workflow/publications", Version: "1.0.0", Title: "Publish own observations", Inputs: map[string]flow.InputPort{}, Outputs: map[string]flow.OutputPort{}, AllowedOutcomes: []string{"succeeded"}, Limits: flow.Limits{MaxStepInstances: 1, MaxControlTransitions: 2, MaxParallelism: 1}, PolicyRef: builtinRef(defs, "core:policy/local")}
	workflow.Definition.Entry = "report"
	workflow.Definition.Stages = map[string]flow.Stage{
		"report": {Kind: "step", StepRef: stepRef, InputBindings: map[string]flow.Binding{}, On: map[string]string{"pass": "done"}},
		"done":   {Kind: "finish", Outcome: "succeeded", OutputBindings: map[string]flow.Binding{}},
	}
	wb, err := canonical(workflow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := flow.Compile(wb, "json", reg); err != nil {
		t.Fatal(err)
	}
	token, now := strings.Repeat("a", 64), e.clock.now()
	r := Run{SchemaVersion: StateVersion, ID: "run:publications", AuthorityID: e.Installation.ID, ProjectID: e.Config.ID, Profile: flow.Profile, TrustProfile: "core-local/cooperative", Status: "running", RootInvocationID: "invocation:one", WorkflowRef: flow.Ref{ID: workflow.ID, Version: workflow.Version, Digest: rawDigest(wb)}, Workflow: wb, Definitions: defs, Ready: []string{}, Active: []string{"attempt:one"}, Created: now, LastObserved: now, CoreBuild: Version}
	r.Activations = map[string]*Activation{"activation:one": {ID: "activation:one", StageID: "report", InvocationID: r.RootInvocationID, Kind: "step", Status: "running", StepID: "step:one", Created: now}}
	r.Steps = map[string]*Step{"step:one": {ID: "step:one", ActivationID: "activation:one", Ref: stepRef, Status: "running", AttemptIDs: []string{"attempt:one"}, Outputs: map[string]ArtifactRef{}, Created: now}}
	r.Attempts = map[string]*Attempt{"attempt:one": {ID: "attempt:one", StepID: "step:one", ActivationID: "activation:one", Status: "running", EnvelopeDigest: rawDigest([]byte("envelope")), TokenHash: rawDigest([]byte(token)), Admitted: now, Dispatch: &now, Started: &now}}
	zero := int64(0)
	_, err = e.apply(context.Background(), e.owner, "command:seed", r.ID, "run.created", map[string]string{"fixture": "publication"}, &zero, local.CommandCAS, func(target *Run, _ local.Snapshot, _ Observation) (local.Change, error) {
		*target = r
		return local.Change{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return e, token, PublishCommand{SchemaVersion: "1", CommandID: "command:initial", RunID: r.ID, StepID: "step:one", AttemptID: "attempt:one", EnvelopeDigest: r.Attempts["attempt:one"].EnvelopeDigest, Hook: "progress_changed", Kind: "state", ExpectedStateVersion: &zero, Value: json.RawMessage(`{"phase":"working","completed":4,"note":"remove me"}`)}
}

func publicationRun(t *testing.T, e *Engine, c PublishCommand) (Run, local.ReadView) {
	t.Helper()
	r, view, err := e.load(context.Background(), c.RunID)
	if err != nil {
		t.Fatal(err)
	}
	return r, view
}

func publicationErrorCode(err error) string {
	var rejection *local.Rejection
	if errors.As(err, &rejection) {
		return rejection.Code
	}
	return ""
}

func changePublicationRun(t *testing.T, e *Engine, c PublishCommand, fn func(*Run, Observation)) {
	t.Helper()
	_, err := e.apply(context.Background(), e.owner, newID("command"), c.RunID, "run.restricted", map[string]string{"fixture": "control"}, nil, local.CommandGuarded, func(r *Run, _ local.Snapshot, obs Observation) (local.Change, error) {
		fn(r, obs)
		return local.Change{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// PUB-AC-02: one CAS winner; no merge, duplicate revision or refreshed receipt.
func TestPublicationStateCASReplacementAndDurableRetry(t *testing.T) {
	e, token, c := publicationFixture(t, nil)
	ctx := context.Background()
	if _, err := e.Publish(ctx, token, c); err != nil {
		t.Fatal(err)
	}
	one := int64(1)
	commands := []PublishCommand{c, c}
	for i := range commands {
		commands[i].CommandID = fmt.Sprintf("command:race-%d", i)
		commands[i].ExpectedStateVersion = &one
		commands[i].Value = json.RawMessage(fmt.Sprintf(`{"phase":"working","completed":%d}`, i+5))
	}
	var wg sync.WaitGroup
	errorsSeen := make([]error, 2)
	results := make([]local.ApplyResult, 2)
	start := make(chan struct{})
	for i := range commands {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i], errorsSeen[i] = e.Publish(ctx, token, commands[i])
		}(i)
	}
	close(start)
	wg.Wait()
	winner := -1
	for i, err := range errorsSeen {
		if err == nil {
			if winner != -1 {
				t.Fatal("both state writers won the same version")
			}
			winner = i
		} else if publicationErrorCode(err) != "state_version_conflict" {
			t.Fatalf("unexpected concurrent failure: %v", err)
		}
	}
	if winner < 0 {
		t.Fatalf("neither writer committed: %v", errorsSeen)
	}
	r, read := publicationRun(t, e, c)
	if len(r.Publications) != 2 || r.Publications[1].Version != 2 || strings.Contains(string(r.Publications[1].Value), "note") || read.Snapshot.Version != 1 || read.Snapshot.EventSeq != 3 {
		t.Fatalf("CAS, replacement or RunVersion contract changed: %+v", r.Publications)
	}
	before, _ := canonicalState(r)
	// Reopen a second authority handle: exact receipts survive the connection
	// and clock session, without authorizing a new write from the old process.
	reopened, err := Open(e.Root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	again, err := reopened.Publish(ctx, token, commands[winner])
	if err != nil || !again.Duplicate || !bytes.Equal(again.Receipt.Result, results[winner].Receipt.Result) {
		t.Fatalf("lost durable receipt: %+v %v", again, err)
	}
	after, nextRead := publicationRun(t, e, c)
	afterBytes, _ := canonicalState(after)
	if !bytes.Equal(before, afterBytes) || read.Snapshot.EventSeq != nextRead.Snapshot.EventSeq {
		t.Fatal("exact retry refreshed state or receipt time")
	}
	changed := commands[winner]
	changed.Value = json.RawMessage(`{"phase":"finished","completed":99}`)
	if _, err := e.Publish(ctx, token, changed); !errors.Is(err, local.ErrCommandConflict) {
		t.Fatalf("command identity reused with other data: %v", err)
	}
}

// PUB-AC-01/03: names are declarative, scope is authenticated, application
// status values never mutate the engine's status or become trusted evidence.
func TestPublicationDeclarationAndOwnerScope(t *testing.T) {
	e, token, c := publicationFixture(t, nil)
	for _, test := range []struct {
		name, code string
		change     func(*PublishCommand)
	}{
		{"undeclared", "unknown_hook", func(c *PublishCommand) { c.Hook = "not_declared" }},
		{"wrong-kind", "hook_kind_mismatch", func(c *PublishCommand) { c.Kind, c.EventKey, c.ExpectedStateVersion = "event", "event:one", nil }},
		{"invalid-schema", "schema_invalid", func(c *PublishCommand) { c.Value = json.RawMessage(`{"phase":"working","completed":"four"}`) }},
		{"bounds", "measurement_out_of_bounds", func(c *PublishCommand) { c.Value = json.RawMessage(`{"phase":"working","completed":1001}`) }},
		{"sibling", "publisher_forbidden", func(c *PublishCommand) { c.StepID = "step:sibling" }},
		{"foreign-attempt", "publisher_forbidden", func(c *PublishCommand) { c.AttemptID = "attempt:foreign" }},
		{"wrong-envelope", "publisher_forbidden", func(c *PublishCommand) { c.EnvelopeDigest = rawDigest([]byte("other")) }},
		{"artifact", "unsupported_publication", func(c *PublishCommand) { c.Kind = "artifact" }},
		{"state-no-version", "invalid_publication", func(c *PublishCommand) { c.ExpectedStateVersion = nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := c
			command.CommandID = "command:" + test.name
			test.change(&command)
			_, err := e.Publish(context.Background(), token, command)
			if publicationErrorCode(err) != test.code {
				t.Fatalf("expected %s, got %v", test.code, err)
			}
		})
	}
	if _, err := e.Publish(context.Background(), strings.Repeat("b", 64), c); publicationErrorCode(err) != "publisher_forbidden" {
		t.Fatalf("knowledge of ids authenticated a publisher: %v", err)
	}
	c.Value = json.RawMessage(`{"phase":"finished","status":"completed"}`)
	if _, err := e.Publish(context.Background(), token, c); err != nil {
		t.Fatal(err)
	}
	r, read := publicationRun(t, e, c)
	if r.Status != "running" || r.Steps[c.StepID].Status != "running" || r.Attempts[c.AttemptID].Accepted != nil || r.Outcome != nil || read.Snapshot.Version != 1 || len(r.Publications) != 1 {
		t.Fatal("own data changed lifecycle or rejected payloads were applied")
	}
}

func TestPublicationLogicalEventDedupAndWarnings(t *testing.T) {
	e, token, c := publicationFixture(t, nil)
	c.Hook, c.Kind, c.EventKey, c.ExpectedStateVersion = "warning_raised", "event", "event:one", nil
	first, err := e.Publish(context.Background(), token, c)
	if err != nil {
		t.Fatal(err)
	}
	r, read := publicationRun(t, e, c)
	before, _ := canonicalState(r)
	c.CommandID = "command:logical-retry"
	c.Value = json.RawMessage(`{"note":"remove me", "completed":4, "phase":"working"}`)
	duplicate, err := e.Publish(context.Background(), token, c)
	if err != nil || duplicate.Duplicate || !bytes.Equal(first.Receipt.Result, duplicate.Receipt.Result) || duplicate.Receipt.EventSeq != first.Receipt.EventSeq {
		t.Fatalf("logical duplicate must get a new receipt for the old occurrence: %+v %v", duplicate, err)
	}
	r, afterRead := publicationRun(t, e, c)
	after, _ := canonicalState(r)
	if !bytes.Equal(before, after) || afterRead.Snapshot.EventSeq != read.Snapshot.EventSeq || afterRead.Cut <= read.Cut || len(r.Diagnostics) != 1 {
		t.Fatal("logical duplicate emitted a second event, refreshed time or lost its receipt")
	}
	d := r.Diagnostics[0]
	if d.Origin != "worker-reported" || d.Severity != "warn" || d.Code != "quality_warning" || d.Message != "Declared quality warning" || d.PublicationID != r.Publications[0].ID || r.Status != "running" {
		t.Fatalf("warning mapping lost provenance or became a failure: %+v", d)
	}
	c.CommandID, c.EventKey = "command:second-occurrence", "event:two"
	if _, err := e.Publish(context.Background(), token, c); err != nil {
		t.Fatal(err)
	}
	c.CommandID, c.EventKey, c.Value = "command:key-conflict", "event:one", json.RawMessage(`{"phase":"finished"}`)
	if _, err := e.Publish(context.Background(), token, c); publicationErrorCode(err) != "event_key_conflict" {
		t.Fatalf("logical event key accepted conflicting bytes: %v", err)
	}
	r, _ = publicationRun(t, e, c)
	if len(r.Publications) != 2 || len(r.Diagnostics) != 2 {
		t.Fatal("different keys should remain distinct occurrences; conflicts are not publications")
	}
}

func TestPublicationCounterOmissionCannotReset(t *testing.T) {
	e, token, c := publicationFixture(t, nil)
	for i, value := range []string{`{"phase":"working","completed":10}`, `{"phase":"finished"}`} {
		version := int64(i)
		c.CommandID, c.ExpectedStateVersion, c.Value = fmt.Sprintf("command:counter-%d", i), &version, json.RawMessage(value)
		if _, err := e.Publish(context.Background(), token, c); err != nil {
			t.Fatal(err)
		}
	}
	two := int64(2)
	c.CommandID, c.ExpectedStateVersion, c.Value = "command:reset", &two, json.RawMessage(`{"phase":"finished","completed":1}`)
	if _, err := e.Publish(context.Background(), token, c); publicationErrorCode(err) != "counter_decreased" {
		t.Fatalf("omission or changed labels reset the cumulative counter: %v", err)
	}
	c.CommandID, c.Value = "command:continue", json.RawMessage(`{"phase":"finished","completed":11}`)
	if _, err := e.Publish(context.Background(), token, c); err != nil {
		t.Fatal(err)
	}
}

func TestPublicationScalarHookValues(t *testing.T) {
	e, token, c := publicationFixture(t, nil)
	c.Hook = "ready_changed"
	for i, value := range []string{"false", "true"} {
		version := int64(i)
		c.CommandID, c.ExpectedStateVersion, c.Value = fmt.Sprintf("command:scalar-%d", i), &version, json.RawMessage(value)
		if _, err := e.Publish(context.Background(), token, c); err != nil {
			t.Fatalf("declared scalar state rejected: %s %v", value, err)
		}
	}
	r, _ := publicationRun(t, e, c)
	if r.Status != "running" || len(r.Publications) != 2 || string(r.Publications[0].Value) != "false" || string(r.Publications[1].Value) != "true" {
		t.Fatal("scalar values were wrapped, discarded or converted into lifecycle")
	}
}

// PUB-AC-04: pause/cancel permits only declared bounded shutdown publications;
// frozen namespaces stay frozen, and receipt reads still require current access.
func TestPublicationStopFreezeAndCurrentReceiptAccess(t *testing.T) {
	e, token, c := publicationFixture(t, nil)
	if _, err := e.Publish(context.Background(), token, c); err != nil {
		t.Fatal(err)
	}
	original := c
	changePublicationRun(t, e, c, func(r *Run, obs Observation) {
		r.Stops = append(r.Stops, Stop{ID: "stop:one", Generation: 1, Epoch: 1, Kind: "pause", Reason: "test", Actor: e.owner, Status: "active", Created: obs})
		r.ControlEpoch++
	})
	one := int64(1)
	c.CommandID, c.ExpectedStateVersion, c.Value = "command:shutdown", &one, json.RawMessage(`{"phase":"finished","completed":5}`)
	if _, err := e.Publish(context.Background(), token, c); err != nil {
		t.Fatalf("declared stop diagnostic rejected: %v", err)
	}
	event := c
	event.CommandID, event.Hook, event.Kind, event.EventKey, event.ExpectedStateVersion = "command:stop-event", "warning_raised", "event", "event:stop", nil
	if _, err := e.Publish(context.Background(), token, event); publicationErrorCode(err) != "publication_restricted" {
		t.Fatalf("undeclared stop publication accepted: %v", err)
	}
	changePublicationRun(t, e, c, func(r *Run, obs Observation) {
		r.Status, r.Active, r.Settled = "cancelled", nil, &obs
		r.Attempts[c.AttemptID].Status, r.Attempts[c.AttemptID].Settled = "cancelled", &obs
		r.Steps[c.StepID].Status = "cancelled"
	})
	if duplicate, err := e.Publish(context.Background(), token, original); err != nil || !duplicate.Duplicate {
		t.Fatalf("settlement invalidated an existing receipt read: %+v %v", duplicate, err)
	}
	c.CommandID = "command:late"
	if _, err := e.Publish(context.Background(), token, c); publicationErrorCode(err) != "publisher_frozen" {
		t.Fatalf("terminal namespace changed: %v", err)
	}
	changePublicationRun(t, e, c, func(r *Run, _ Observation) { r.Attempts[c.AttemptID].TokenHash = "" })
	if _, err := e.Publish(context.Background(), token, original); publicationErrorCode(err) != "publisher_forbidden" {
		t.Fatalf("durable receipt bypassed current read access: %v", err)
	}
}

func TestPublicationCountRateAndControlReserve(t *testing.T) {
	for _, limit := range []string{"count", "rate"} {
		t.Run(limit, func(t *testing.T) {
			e, token, c := publicationFixture(t, func(step *flow.StepDefinition) {
				hook := step.Hooks["progress_changed"]
				if limit == "count" {
					hook.MaxCount = 1
				} else {
					hook.MaxPerMinute = 1
				}
				step.Hooks["progress_changed"] = hook
			})
			if _, err := e.Publish(context.Background(), token, c); err != nil {
				t.Fatal(err)
			}
			original := c
			one := int64(1)
			c.CommandID, c.ExpectedStateVersion = "command:over-limit", &one
			if _, err := e.Publish(context.Background(), token, c); publicationErrorCode(err) != "publication_"+limit+"_exhausted" {
				t.Fatalf("%s bound ignored: %v", limit, err)
			}
			if again, err := e.Publish(context.Background(), token, original); err != nil || !again.Duplicate {
				t.Fatal("rate/count limit blocked an exact receipt read")
			}
		})
	}
	e, token, c := publicationFixture(t, nil)
	large, _ := json.Marshal(map[string]string{"phase": "working", "note": strings.Repeat("x", 60000)})
	c.Value = large
	accepted := 0
	for i := 0; i < 32; i++ {
		version := int64(i)
		c.CommandID, c.ExpectedStateVersion = fmt.Sprintf("command:large-%d", i), &version
		_, err := e.Publish(context.Background(), token, c)
		if publicationErrorCode(err) == "publication_budget_exhausted" {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		accepted++
	}
	if accepted == 0 || accepted == 32 {
		t.Fatalf("publication history did not enforce its byte budget: %d", accepted)
	}
	changePublicationRun(t, e, c, func(r *Run, _ Observation) { r.CancelRequested = true; r.Status = "stopping" })
	r, _ := publicationRun(t, e, c)
	if !r.CancelRequested || len(r.Publications) != accepted {
		t.Fatal("optional flood consumed control reserve or applied rejected data")
	}
}

func TestPublisherStatusFreshnessIsReadOnly(t *testing.T) {
	e, token, c := publicationFixture(t, nil)
	initial, err := e.publisherStatus(context.Background(), token, c)
	if err != nil || initial.Hooks[c.Hook].Availability != "unpublished" || initial.Hooks[c.Hook].LatestState != nil {
		t.Fatalf("unknown state was invented: %+v %v", initial, err)
	}
	if _, err := e.Publish(context.Background(), token, c); err != nil {
		t.Fatal(err)
	}
	r, before := publicationRun(t, e, c)
	stateBefore, _ := canonicalState(r)
	fresh, err := e.publisherStatus(context.Background(), token, c)
	if err != nil || fresh.Hooks[c.Hook].Freshness != "fresh" || fresh.Frozen {
		t.Fatalf("fresh own state missing: %+v %v", fresh, err)
	}
	e.clock.start = e.clock.start.Add(-time.Minute)
	stale, err := e.publisherStatus(context.Background(), token, c)
	if err != nil || stale.Hooks[c.Hook].Freshness != "stale" {
		t.Fatalf("expiry depended on a new publication: %+v %v", stale, err)
	}
	e.clock = newClock()
	unknown, err := e.publisherStatus(context.Background(), token, c)
	if err != nil || unknown.Hooks[c.Hook].Freshness != "unknown" || !unknown.Frozen {
		t.Fatalf("new clock revived old publisher freshness: %+v %v", unknown, err)
	}
	one := int64(1)
	c.CommandID, c.ExpectedStateVersion = "command:stale-generation", &one
	if _, err := e.Publish(context.Background(), token, c); publicationErrorCode(err) != "publisher_frozen" {
		t.Fatalf("new core session accepted an old publisher: %v", err)
	}
	r, after := publicationRun(t, e, c)
	stateAfter, _ := canonicalState(r)
	if !bytes.Equal(stateBefore, stateAfter) || before.Snapshot.EventSeq != after.Snapshot.EventSeq {
		t.Fatal("read or rejected stale publication mutated the projection")
	}
}

func TestPublicationHTTPClosedContractAndScopedStatus(t *testing.T) {
	e, token, c := publicationFixture(t, nil)
	socket, stop, err := e.serveSteps()
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	for path, mode := range map[string]os.FileMode{filepath.Dir(socket): 0700, socket: 0600} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != mode {
			t.Fatalf("publisher socket permissions: %s %v %v", path, info, err)
		}
	}
	transport := &http.Transport{DisableKeepAlives: true, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	request := func(method, path, bearer string, body []byte) (int, []byte) {
		t.Helper()
		req, err := http.NewRequest(method, "http://local"+path, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		return response.StatusCode, data
	}
	data, _ := canonical(c)
	if status, body := request("POST", "/publish", "", data); status != http.StatusForbidden || flow.ValidateProtocol("Problem", body) != nil {
		t.Fatalf("HTTP authentication must return the public Problem contract: %d %s", status, body)
	}
	for _, body := range [][]byte{
		append(append([]byte{}, data[:len(data)-1]...), []byte(`,"actor_id":"core"}`)...),
		append(append([]byte{}, data[:len(data)-1]...), []byte(`,"command_id":"command:duplicate"}`)...),
		append(append([]byte{}, data[:len(data)-1]...), []byte(`,"event_key":""}`)...),
		append(append([]byte{}, data...), []byte(` {}`)...),
		[]byte(strings.Repeat(" ", maxPublishBodyBytes+1)),
	} {
		if status, response := request("POST", "/publish", token, body); status != http.StatusBadRequest || flow.ValidateProtocol("Problem", response) != nil {
			t.Fatalf("invalid/oversized HTTP envelope needs a public Problem: %d %s", status, response)
		}
	}
	event := c
	event.CommandID, event.Kind, event.Hook, event.EventKey, event.ExpectedStateVersion = "command:http-event", "event", "warning_raised", "event:http", nil
	eventBody, _ := canonical(event)
	eventBody = append(eventBody[:len(eventBody)-1], []byte(`,"expected_state_version":null}`)...)
	if status, _ := request("POST", "/publish", token, eventBody); status != http.StatusBadRequest {
		t.Fatal("JSON null bypassed the closed event variant")
	}
	if status, body := request("POST", "/publish", token, data); status != http.StatusOK || !bytes.Contains(body, []byte(`"publication":`)) {
		t.Fatalf("valid publication failed: %d %s", status, body)
	}
	query := url.Values{"run_id": {c.RunID}, "step_instance_id": {c.StepID}, "attempt_id": {c.AttemptID}, "envelope_digest": {c.EnvelopeDigest}}
	if status, body := request("GET", "/status?"+query.Encode(), token, nil); status != http.StatusOK || !bytes.Contains(body, []byte(stepReadVersion)) || bytes.Contains(body, []byte("token_hash")) || bytes.Contains(body, []byte("context")) || bytes.Contains(body, []byte("definitions")) {
		t.Fatalf("own status failed or disclosed private engine context: %d %s", status, body)
	}
	query.Set("step_instance_id", "step:other")
	if status, _ := request("GET", "/status?"+query.Encode(), token, nil); status != http.StatusForbidden {
		t.Fatal("publisher could read another namespace")
	}
	// A controlled admitted-namespace fixture, a real owner cancel transaction
	// and real HTTP requests qualify the cancel-pending gate without claiming
	// that this fixture itself launched an OS worker.
	if _, err := e.Restrict(context.Background(), RestrictCommand{SchemaVersion: "1", CommandID: "command:http-cancel", Scope: "run", ScopeID: c.RunID, Kind: "cancel", Reason: "Stop this admitted namespace"}); err != nil {
		t.Fatal(err)
	}
	before, beforeRead := publicationRun(t, e, c)
	if !before.CancelRequested || before.Status != "stopping" || before.Attempts[c.AttemptID].Settled != nil {
		t.Fatal("cancel-pending fixture is not an unsettled admission")
	}
	shutdown := c
	one := int64(1)
	shutdown.CommandID, shutdown.ExpectedStateVersion, shutdown.Value = "command:http-shutdown", &one, json.RawMessage(`{"phase":"finished","completed":5}`)
	shutdownBody, _ := canonical(shutdown)
	if status, body := request("POST", "/publish", token, shutdownBody); status != http.StatusOK {
		t.Fatalf("declared bounded shutdown state refused: %d %s", status, body)
	}
	eventBody, _ = canonical(event)
	if status, body := request("POST", "/publish", token, eventBody); status == http.StatusOK || !bytes.Contains(body, []byte(`"code":"publication_restricted"`)) {
		t.Fatalf("undeclared cancel-time publication accepted: %d %s", status, body)
	}
	after, afterRead := publicationRun(t, e, c)
	if after.Status != "stopping" || !after.CancelRequested || after.ControlEpoch != before.ControlEpoch || afterRead.Snapshot.Version != beforeRead.Snapshot.Version || len(after.Active) != 1 {
		t.Fatal("publication changed cancellation, business CAS or execution obligations")
	}
	stop()
	stop()
	if _, err := os.Stat(filepath.Dir(socket)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("ephemeral socket directory survived shutdown")
	}
}

func TestSlowPublisherClientsCannotHoldControlWriter(t *testing.T) {
	e, token, c := publicationFixture(t, nil)
	socket, stop, err := e.serveSteps()
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	for i := 0; i < 4; i++ {
		conn, err := net.Dial("unix", socket)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(time.Second))
		_, err = fmt.Fprintf(conn, "POST /publish HTTP/1.1\r\nHost: local\r\nAuthorization: Bearer %s\r\nContent-Length: 10000\r\nExpect: 100-continue\r\n\r\n", token)
		if err != nil {
			t.Fatal(err)
		}
		line, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil || !strings.Contains(line, "100 Continue") {
			t.Fatalf("slow body did not enter a bounded handler: %q %v", line, err)
		}
	}
	started := time.Now()
	changePublicationRun(t, e, c, func(r *Run, _ Observation) { r.CancelRequested = true; r.Status = "stopping" })
	if time.Since(started) >= time.Second {
		t.Fatal("slow optional clients held the control writer")
	}
	conn, err := net.Dial("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(time.Second))
	_, _ = fmt.Fprintf(conn, "GET /status HTTP/1.1\r\nHost: local\r\nAuthorization: Bearer %s\r\n\r\n", token)
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil || !strings.Contains(line, "429") {
		t.Fatalf("publisher concurrency was not bounded: %q %v", line, err)
	}
}
