package main

import (
	"maps"
	"reflect"

	"github.com/stenhigh/prifly/internal/flow"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func timedSessionField(t reflect.Type, name string) bool {
	return t == reflect.TypeFor[prifly.SessionHandoff]() && name == "Timing" ||
		t == reflect.TypeFor[prifly.SessionTask]() && name == "Delivery" ||
		t == reflect.TypeFor[prifly.DecisionRequest]() && name == "YieldExecution" ||
		t == reflect.TypeFor[prifly.DecisionRecord]() && name == "ClosureReason" ||
		t == reflect.TypeFor[prifly.Preview]() && name == "SessionLimits" ||
		t == reflect.TypeFor[prifly.NextView]() && name == "ReasonCode"
}

func timedSessionConstraints(g *generator) {
	ref := func(name string) map[string]any { return map[string]any{"$ref": "#/$defs/" + name} }
	// State 27 may contain both legacy and timed steps. Detached shapes retain
	// the old closed fields; a Run version alone never changes a step's clock.
	legacy := func(name, target string, omitted ...string) {
		definition := maps.Clone(g.defs[name].(map[string]any))
		properties := maps.Clone(definition["properties"].(map[string]any))
		for _, field := range omitted {
			delete(properties, field)
		}
		definition["properties"] = properties
		g.defs[target] = definition
	}
	legacy("runtime_SessionHandoff", "runtime_LegacySessionHandoff", "timing")
	legacy("runtime_DecisionRequest", "runtime_LegacyDecisionRequest", "yield_execution")
	legacy("runtime_DecisionRecord", "runtime_LegacyDecisionRecord", "closure_reason")
	// The old bridge already emitted waiting_decision, although its delivered
	// bundle omitted that value. Describe it here without rewriting that bundle.
	g.property("runtime_LegacySessionHandoff", "host_state", enum(prifly.SessionAwaiting, prifly.SessionReported, prifly.SessionDisconnected, "waiting_decision"))
	g.property("runtime_LegacyDecisionRequest", "schema_version", map[string]any{"const": prifly.DecisionRequestVersion})
	g.property("runtime_LegacyDecisionRecord", "schema_version", map[string]any{"const": prifly.DecisionRecordVersion})
	g.property("runtime_LegacyDecisionRecord", "status", enum("presented", "answered", "defaulted", "rejected", "pending"))

	for name, version := range map[string]string{
		"runtime_Run": prifly.CoreTimingStateVersion, "runtime_RunView": prifly.CoreTimingReadVersion,
		"runtime_NextView": prifly.CoreTimingNextVersion, "runtime_Preview": prifly.CoreTimingPreviewVersion,
		"runtime_StepReadView": prifly.CoreTimingStepReadVersion,
	} {
		g.property(name, "schema_version", map[string]any{"const": version})
	}
	nextActions := g.defs["runtime_NextView"].(map[string]any)["properties"].(map[string]any)["action"].(map[string]any)["enum"].([]string)
	g.property("runtime_NextView", "action", enum(append(nextActions, "waiting_decision", "session_resume", "session_expired")...))
	g.property("runtime_NextView", "reason_code", enum("attempt_deadline_expired", "decision_wait_expired", "deadline_clock_unqualified"))
	for _, name := range []string{"runtime_SessionHandoff", "runtime_SessionTask", "runtime_SessionSubmission"} {
		g.property(name, "schema_version", map[string]any{"const": prifly.AssistedSessionTimingVersion})
	}
	g.property("runtime_SessionHandoff", "host_state", enum(prifly.SessionAwaiting, prifly.SessionReported, prifly.SessionDisconnected, "waiting_decision", prifly.SessionWaitingAdmission))
	g.property("runtime_SessionHandoff", "timing", ref("runtime_SessionTiming"))
	g.defs["runtime_SessionHandoff"].(map[string]any)["required"] = append(g.defs["runtime_SessionHandoff"].(map[string]any)["required"].([]string), "timing")
	g.property("runtime_SessionTask", "delivery", ref("runtime_SessionDelivery"))
	g.defs["runtime_SessionTask"].(map[string]any)["required"] = append(g.defs["runtime_SessionTask"].(map[string]any)["required"].([]string), "delivery")
	g.property("runtime_Attempt", "session", nullable(map[string]any{"oneOf": []any{ref("runtime_LegacySessionHandoff"), ref("runtime_SessionHandoff")}}))
	g.property("runtime_SessionTiming", "remaining_ms", map[string]any{"type": "integer", "minimum": 0, "maximum": flow.MaxSessionTimeoutMS})
	g.property("flow_SessionLimits", "active_timeout_ms", map[string]any{"type": "integer", "minimum": 1, "maximum": flow.MaxSessionTimeoutMS})
	g.property("flow_SessionLimits", "decision_wait_timeout_ms", nullable(map[string]any{"type": "integer", "minimum": 1, "maximum": flow.MaxSessionTimeoutMS}))
	g.property("runtime_SessionDelivery", "base_envelope_digest", ref("Digest"))
	g.property("runtime_SessionDelivery", "generation", map[string]any{"type": "integer", "minimum": 1, "maximum": int64(9007199254740991)})
	g.property("runtime_SessionLimitPreview", "limits", ref("flow_SessionLimits"))
	g.property("runtime_SessionLimitPreview", "legacy_absolute_timeout_ms", map[string]any{"const": flow.DefaultSessionActiveTimeoutMS})
	g.defs["runtime_SessionLimitPreview"].(map[string]any)["oneOf"] = []any{
		map[string]any{"required": []string{"limits"}},
		map[string]any{"required": []string{"legacy_absolute_timeout_ms"}},
	}

	g.property("runtime_DecisionRequest", "schema_version", map[string]any{"const": prifly.DecisionRequestTimingVersion})
	g.property("runtime_DecisionRequest", "yield_execution", map[string]any{"const": true})
	g.defs["runtime_DecisionRequest"].(map[string]any)["required"] = append(g.defs["runtime_DecisionRequest"].(map[string]any)["required"].([]string), "yield_execution")
	g.property("runtime_Run", "pending_decision", nullable(map[string]any{"oneOf": []any{ref("runtime_LegacyDecisionRequest"), ref("runtime_DecisionRequest")}}))
	g.property("runtime_DecisionRecord", "schema_version", map[string]any{"const": prifly.DecisionRecordTimingVersion})
	g.property("runtime_DecisionRecord", "status", enum("presented", "answered", "defaulted", "rejected", "pending", "cancelled", "expired"))
	g.property("runtime_DecisionRecord", "closure_reason", enum("run_cancelled", "decision_wait_expired", "attempt_deadline_expired"))
	g.defs["runtime_DecisionRecord"].(map[string]any)["allOf"] = []any{
		map[string]any{"if": map[string]any{"properties": map[string]any{"status": map[string]any{"const": "cancelled"}}}, "then": map[string]any{"required": []string{"closure_reason"}, "properties": map[string]any{"closure_reason": map[string]any{"const": "run_cancelled"}}}},
		map[string]any{"if": map[string]any{"properties": map[string]any{"status": map[string]any{"const": "expired"}}}, "then": map[string]any{"required": []string{"closure_reason"}, "properties": map[string]any{"closure_reason": enum("decision_wait_expired", "attempt_deadline_expired")}}},
	}
	g.property("runtime_DecisionSheet", "records", nullable(map[string]any{"type": "array", "items": ref("runtime_LegacyDecisionRecord")}))
	g.property("runtime_Run", "decision_ledger", nullable(map[string]any{"type": "array", "items": map[string]any{"oneOf": []any{ref("runtime_LegacyDecisionRecord"), ref("runtime_DecisionRecord")}}}))
}
