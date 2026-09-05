package runtime

import (
	"encoding/json"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

func TestTimingWireFieldsRespectExactEditions(t *testing.T) {
	for _, test := range []struct {
		name, fields string
		valid        bool
	}{
		{"legacy unchanged", `"schema_version":"core-state/26","attempts":{"attempt:one":{"session":{"schema_version":"assisted-session/5"}}}`, true},
		{"legacy timing null", `"schema_version":"core-state/26","attempts":{"attempt:one":{"session":{"schema_version":"assisted-session/5","timing":null}}}`, false},
		{"mixed legacy timing null", `"schema_version":"core-state/27","attempts":{"attempt:one":{"session":{"schema_version":"assisted-session/5","timing":null}}}`, false},
		{"legacy yield false", `"schema_version":"core-state/26","pending_decision":{"schema_version":"decision-request/1","yield_execution":false}`, false},
		{"legacy request v2", `"schema_version":"core-state/26","pending_decision":{"schema_version":"decision-request/2","yield_execution":true}`, false},
		{"timed request", `"schema_version":"core-state/27","pending_decision":{"schema_version":"decision-request/2","yield_execution":true}`, true},
		{"legacy closure empty", `"schema_version":"core-state/26","decision_ledger":[{"schema_version":"decision-record/1","closure_reason":""}]`, false},
		{"legacy record v2", `"schema_version":"core-state/26","decision_ledger":[{"schema_version":"decision-record/2"}]`, false},
		{"mixed ledger", `"schema_version":"core-state/27","decision_ledger":[{"schema_version":"decision-record/1"},{"schema_version":"decision-record/2","closure_reason":"run_cancelled"}]`, true},
		{"preflight runtime closure", `"schema_version":"core-state/27","decision_sheet":{"records":[{"schema_version":"decision-record/1","closure_reason":null}]}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var document map[string]any
			if err := json.Unmarshal([]byte("{"+test.fields+"}"), &document); err != nil {
				t.Fatal(err)
			}
			document["executors"] = map[string]any{}
			if attempts, ok := document["attempts"].(map[string]any); ok {
				for _, attempt := range attempts {
					attempt.(map[string]any)["context"] = map[string]any{}
				}
			} else {
				document["attempts"] = map[string]any{}
			}
			data, _ := json.Marshal(document)
			var run Run
			if err := decodeState(data, &run); (err == nil) != test.valid {
				t.Fatalf("valid=%t, decode error=%v", test.valid, err)
			}
		})
	}
}

func TestTimedPublicContractsPreserveLegacyShapes(t *testing.T) {
	request := DecisionRequest{SchemaVersion: DecisionRequestTimingVersion, RunID: "run:one", AttemptID: "attempt:one", EnvelopeDigest: rawDigest([]byte("delivery")), DecisionID: "answer", DefinitionDigest: rawDigest([]byte("decision")), ExpectedRunVersion: 3, YieldExecution: true}
	if err := validatePublic(t, "DecisionRequestV2", request); err != nil {
		t.Fatal(err)
	}
	if err := validatePublic(t, "DecisionRequest", request); err == nil {
		t.Fatal("legacy request accepted a timed handover")
	}
	request.YieldExecution = false
	if err := validatePublic(t, "DecisionRequestV2", request); err == nil {
		t.Fatal("timed request accepted missing handover")
	}
	request.SchemaVersion = DecisionRequestVersion
	if err := validatePublic(t, "DecisionRequest", request); err != nil {
		t.Fatal(err)
	}
	observed := Observation{UTC: "2026-09-06T12:00:00Z", Session: "clock:test", Source: "test", UTCTrust: "local_wall_unqualified", SuspendBasis: "test"}
	timing := SessionTiming{Limits: flow.SessionLimits{ActiveTimeoutMS: 3600000}, RemainingMS: 3000000, Observed: observed}
	handoff := SessionHandoff{SchemaVersion: AssistedSessionTimingVersion, PrincipalID: "principal:test", SkillRefs: []flow.Ref{}, Timing: &timing, HostState: SessionWaitingAdmission, DeadlineTrust: "local_wall_unqualified", Handed: observed}
	if err := validatePublic(t, "SessionHandoffV6", handoff); err != nil {
		t.Fatal(err)
	}
	if err := validatePublic(t, "SessionHandoffV5", handoff); err == nil {
		t.Fatal("legacy handoff accepted timing")
	}
	handoff.Timing = nil
	if err := validatePublic(t, "SessionHandoffV6", handoff); err == nil {
		t.Fatal("timed handoff accepted missing timing")
	}
	// Validate the actual mixed-state union, not only the new standalone DTO.
	schema, err := PublicSchema("CoreRunStateV27")
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(schema, &document); err != nil {
		t.Fatal(err)
	}
	document["$ref"] = "#/$defs/runtime_Attempt/properties/session"
	schema, _ = json.Marshal(document)
	digest, _ := flow.Digest(schema)
	ref := flow.Ref{ID: "test:schema/mixed-session", Version: "1.0.0", Digest: digest}
	for _, phase := range []string{SessionAwaiting, "waiting_decision", SessionWaitingAdmission} {
		handoff.SchemaVersion, handoff.HostState = AssistedSessionDecisionVersion, phase
		data, _ := json.Marshal(handoff)
		if err := flow.ValidateSchema(flow.Registry{ref: schema}, ref, data); (err == nil) != (phase != SessionWaitingAdmission) {
			t.Fatalf("legacy mixed phase %s: %v", phase, err)
		}
	}
	for _, status := range []string{"cancelled", "expired"} {
		t.Run(status, func(t *testing.T) {
			record := DecisionRecord{SchemaVersion: DecisionRecordTimingVersion, DefinitionID: "answer", DefinitionDigest: request.DefinitionDigest, AttemptID: request.AttemptID, Status: status, Source: "unanswered", Observed: &observed, ClosureReason: "run_cancelled"}
			if status == "expired" {
				record.ClosureReason = "decision_wait_expired"
			}
			if err := validatePublic(t, "DecisionRecordV2", record); err != nil {
				t.Fatal(err)
			}
			if err := validatePublic(t, "DecisionRecord", record); err == nil {
				t.Fatal("legacy decision record accepted a closure")
			}
			record.ClosureReason = ""
			if err := validatePublic(t, "DecisionRecordV2", record); err == nil {
				t.Fatal("closed decision lost its closure reason")
			}
		})
	}
}

func TestTimingStateLadderAndWireRoundTrip(t *testing.T) {
	if !isTimingState(CoreTimingStateVersion) || isTimingState(CoreNeutralStateVersion) || readVersionFor(CoreTimingStateVersion, flow.CoreProfile) != CoreTimingReadVersion || stepReadVersionFor(CoreTimingStateVersion) != CoreTimingStepReadVersion {
		t.Fatal("timing state/read boundary drifted")
	}
	for _, version := range []string{AssistedSessionDecisionVersion, AssistedSessionTimingVersion} {
		t.Run(version, func(t *testing.T) {
			session := &SessionHandoff{SchemaVersion: version}
			if version == AssistedSessionTimingVersion {
				session.Timing = &SessionTiming{Limits: flow.SessionLimits{ActiveTimeoutMS: 3600000}, RemainingMS: 3000000}
			}
			run := Run{SchemaVersion: CoreTimingStateVersion, Attempts: map[string]*Attempt{"attempt:one": {Session: session}}}
			data, err := canonicalState(run)
			if err != nil {
				t.Fatal(err)
			}
			var reopened Run
			if err := decodeState(data, &reopened); err != nil {
				t.Fatal(err)
			}
			before, _ := json.Marshal(session)
			after, _ := json.Marshal(reopened.Attempts["attempt:one"].Session)
			if string(before) != string(after) {
				t.Fatalf("session changed: %s != %s", before, after)
			}
		})
	}
}
