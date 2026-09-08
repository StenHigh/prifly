package flow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

func sessionLimitSource(marker, limits string) []byte {
	digest := "sha256:" + strings.Repeat("0", 64)
	return []byte(fmt.Sprintf(`authoring: %s
id: test:step/timed
version: 1.0.0
refs:
  adapter: {id: core:adapter/assisted-session, version: 1.0.0, digest: %s}
  result: {id: test:schema/result, version: 1.0.0, digest: %s}
kind: worker
executor: {adapter_ref: adapter, operation: session}
effects: {class: none, retry_class: never}
result_schema_ref: result
%s`, marker, digest, digest, limits))
}

func TestSessionLimitsAuthoringDefaultsAndValues(t *testing.T) {
	// A zero expectation means the field is absent: nothing declares a deadline
	// of zero, and both fields spell "no deadline" the same way.
	for _, test := range []struct {
		name, limits, version string
		active, wait          int64
	}{
		{"omitted", "", "6", DefaultSessionActiveTimeoutMS, 0},
		{"empty", "session_limits: {}", "6", DefaultSessionActiveTimeoutMS, 0},
		{"active only", "session_limits: {active_timeout_ms: 7200000}", "6", 7200000, 0},
		{"wait only", "session_limits: {decision_wait_timeout_ms: 1209600000}", "6", DefaultSessionActiveTimeoutMS, 1209600000},
		{"explicit null wait", "session_limits: {active_timeout_ms: 1, decision_wait_timeout_ms: null}", "6", 1, 0},
		{"representation maximum", "session_limits: {active_timeout_ms: 9223372036854, decision_wait_timeout_ms: 9223372036854}", "6", MaxSessionTimeoutMS, MaxSessionTimeoutMS},
		{"explicit null active", "session_limits: {active_timeout_ms: null}", "7", 0, 0},
		{"neither deadline", "session_limits: {active_timeout_ms: null, decision_wait_timeout_ms: null}", "7", 0, 0},
		{"no work deadline with a declared wait", "session_limits: {active_timeout_ms: null, decision_wait_timeout_ms: 86400000}", "7", 0, 86400000},
		{"pinned v7 may still name a number", "schema_version: '7'\nsession_limits: {active_timeout_ms: 7200000}", "7", 7200000, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			data, err := StepJSONBytes(sessionLimitSource(StepSessionAuthoringVersion, test.limits), "yaml")
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateProtocol("StepDefinitionV"+test.version, data); err != nil {
				t.Fatal(err)
			}
			var step StepDefinition
			if err := json.Unmarshal(data, &step); err != nil {
				t.Fatal(err)
			}
			if step.SchemaVersion != test.version || step.SessionLimits == nil || len(step.WorkspaceTrees) != 0 {
				t.Fatalf("limits were not pinned independently of workspace trees: %+v", step)
			}
			active := step.SessionLimits.ActiveTimeoutMS
			if (test.active == 0 && active != nil) || (test.active != 0 && (active == nil || *active != test.active)) {
				t.Fatalf("wrong active work limit: %+v", step.SessionLimits)
			}
			// A declared absence must never reach the runtime as a window that
			// has already elapsed: that would be worse than the hour it removes.
			if allowance := step.SessionLimits.ActiveAllowanceMS(); allowance < 1 || allowance > MaxSessionTimeoutMS {
				t.Fatalf("unusable working window for %+v: %d", step.SessionLimits, allowance)
			}
			if test.active == 0 && step.SessionLimits.ActiveAllowanceMS() != MaxSessionTimeoutMS {
				t.Fatalf("no declared deadline did not reach the longest representable window: %+v", step.SessionLimits)
			}
			wait := step.SessionLimits.DecisionWaitTimeoutMS
			if (test.wait == 0 && wait != nil) || (test.wait != 0 && (wait == nil || *wait != test.wait)) {
				t.Fatalf("wrong decision wait limit: %+v", step.SessionLimits)
			}
			if bytes.Contains(data, []byte("authoring")) || !bytes.Contains(data, []byte(`"decision_wait_timeout_ms":`)) {
				t.Fatal("sealed definition kept authoring or omitted its explicit wait policy")
			}
		})
	}
	implicit, _ := StepJSONBytes(sessionLimitSource(StepSessionAuthoringVersion, ""), "yaml")
	explicit, _ := StepJSONBytes(sessionLimitSource(StepSessionAuthoringVersion, "session_limits: {active_timeout_ms: 3600000, decision_wait_timeout_ms: null}"), "yaml")
	if !bytes.Equal(implicit, explicit) {
		t.Fatal("spelling the defaults changed sealed definition bytes")
	}
	// v7 exists only to carry a declared absence. A step that names a number is
	// still sealed as v6, so publishing the new contract moved no known digest.
	pinned, _ := StepJSONBytes(sessionLimitSource(StepSessionAuthoringVersion, "schema_version: '6'\nsession_limits: {active_timeout_ms: 3600000, decision_wait_timeout_ms: null}"), "yaml")
	if !bytes.Equal(implicit, pinned) || !bytes.Contains(implicit, []byte(`"schema_version":"6"`)) {
		t.Fatal("a step that declares a work deadline stopped sealing v6 bytes")
	}
}

func TestSessionLimitsAuthoringRefusesInvalidContracts(t *testing.T) {
	for _, limits := range []string{
		"null", "[]", "{active_timeout_ms: 0}",
		"{active_timeout_ms: -1}", "{active_timeout_ms: 1.5}", "{active_timeout_ms: '1000'}",
		"{active_timeout_ms: 9223372036855}", "{active_timeout_ms: 9223372036854775808}",
		"{decision_wait_timeout_ms: 0}", "{decision_wait_timeout_ms: -1}",
		"{decision_wait_timeout_ms: 1.5}", "{decision_wait_timeout_ms: 9223372036855}",
		"{unexpected: 1000}",
	} {
		t.Run(limits, func(t *testing.T) {
			_, err := StepJSONBytes(sessionLimitSource(StepSessionAuthoringVersion, "session_limits: "+limits), "yaml")
			if err == nil {
				t.Fatal("invalid session limit was accepted")
			}
		})
	}
	for _, test := range []struct{ name, before, after, code string }{
		{"managed adapter", "core:adapter/assisted-session", "core:adapter/local-process", "schema_invalid"},
		{"other operation", "operation: session", "operation: execute", "schema_invalid"},
		{"unknown marker", "prifly-step/2", "prifly-step/3", "unsupported_authoring"},
		{"wrong machine edition", "kind: worker", "schema_version: '5'\nkind: worker", "schema_invalid"},
		{"unknown machine edition", "kind: worker", "schema_version: '8'\nkind: worker", "schema_invalid"},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := strings.Replace(string(sessionLimitSource(StepSessionAuthoringVersion, "")), test.before, test.after, 1)
			_, err := StepJSONBytes([]byte(source), "yaml")
			expectProblem(t, err, test.code)
		})
	}
	// Pinning v6 while declaring no work deadline is answered by v6 itself: the
	// author is refused, never quietly moved to the contract that would accept it.
	_, err := StepJSONBytes(sessionLimitSource(StepSessionAuthoringVersion, "schema_version: '6'\nsession_limits: {active_timeout_ms: null}"), "yaml")
	expectProblem(t, err, "schema_invalid")
	_, err = StepJSONBytes(sessionLimitSource(StepAuthoringVersion, "session_limits: {}"), "yaml")
	expectProblem(t, err, "schema_invalid")
	_, err = StepJSONBytes(sessionLimitSource(StepAuthoringVersion, "schema_version: '6'"), "yaml")
	expectProblem(t, err, "schema_invalid")
	_, err = StepJSONBytes([]byte(`{"authoring":"prifly-step/2"}`), "json")
	expectProblem(t, err, "unsupported_authoring")
}

func TestSessionLimitsWireAndLegacyIsolation(t *testing.T) {
	legacy, err := StepJSONBytes(sessionLimitSource(StepAuthoringVersion, ""), "yaml")
	if err != nil {
		t.Fatal(err)
	}
	var step StepDefinition
	if err := json.Unmarshal(legacy, &step); err != nil || step.SchemaVersion != "2" || step.SessionLimits != nil {
		t.Fatalf("legacy authoring acquired timed semantics: %+v %v", step, err)
	}
	if !bytes.Equal(legacy, encoded(t, step)) {
		// Canonical order differs between structs and maps, but their sealed bytes must not.
		left, _ := Canonical(legacy)
		right, _ := Canonical(encoded(t, step))
		if !bytes.Equal(left, right) {
			t.Fatal("new optional Go field altered the legacy wire shape")
		}
	}
	hour := DefaultSessionActiveTimeoutMS
	step.SessionLimits = &SessionLimits{ActiveTimeoutMS: &hour}
	if err := ValidateProtocol("StepDefinitionV2", encoded(t, step)); err == nil {
		t.Fatal("legacy machine contract accepted session limits")
	}
	step.SchemaVersion = "6"
	if err := ValidateProtocol("StepDefinitionV6", encoded(t, step)); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"active_timeout_ms", "decision_wait_timeout_ms"} {
		var value map[string]any
		if err := json.Unmarshal(encoded(t, step), &value); err != nil {
			t.Fatal(err)
		}
		delete(value["session_limits"].(map[string]any), field)
		for _, name := range []string{"StepDefinitionV6", "StepDefinitionV7"} {
			if err := ValidateProtocol(name, encoded(t, value)); err == nil {
				t.Fatalf("%s did not require materialized %s", name, field)
			}
		}
	}
	// The published v6 bytes still refuse the absence v7 was published to carry,
	// and v7 changes nothing about the neighbour it copied.
	step.SessionLimits.ActiveTimeoutMS = nil
	if err := ValidateProtocol("StepDefinitionV6", encoded(t, step)); err == nil {
		t.Fatal("frozen v6 contract accepted an absent work deadline")
	}
	step.SchemaVersion = "7"
	if err := ValidateProtocol("StepDefinitionV7", encoded(t, step)); err != nil {
		t.Fatal(err)
	}
	for _, wait := range []int64{0, -1, MaxSessionTimeoutMS + 1} {
		step.SessionLimits.DecisionWaitTimeoutMS = &wait
		if err := ValidateProtocol("StepDefinitionV7", encoded(t, step)); err == nil {
			t.Fatalf("v7 widened the decision wait limit it inherited: %d", wait)
		}
	}
	step.SessionLimits.DecisionWaitTimeoutMS = nil
	for _, active := range []int64{0, -1, MaxSessionTimeoutMS + 1} {
		step.SessionLimits.ActiveTimeoutMS = &active
		if err := ValidateProtocol("StepDefinitionV7", encoded(t, step)); err == nil {
			t.Fatalf("v7 accepted a work limit outside the representable range: %d", active)
		}
	}
}

func TestSessionLimitsCompileAndWorkspaceTrees(t *testing.T) {
	w, registry := contextWorkflow(t, nil, nil)
	changeCheckedStep(t, &w, registry, func(step *StepDefinition) {
		var adapter map[string]any
		if err := json.Unmarshal(registry[step.Executor.AdapterRef], &adapter); err != nil {
			t.Fatal(err)
		}
		adapter["id"] = "core:adapter/assisted-session"
		step.Executor.AdapterRef = checkComponent(t, registry, "core:adapter/assisted-session", step.Executor.AdapterRef.Version, adapter)
		step.Executor.Operation, step.SchemaVersion = "session", "6"
		active := int64(7200000)
		step.SessionLimits = &SessionLimits{ActiveTimeoutMS: &active}
	})
	plan, err := CompileCore(encoded(t, w), "json", registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	if limits := plan.Steps["work"].SessionLimits; limits == nil || limits.ActiveAllowanceMS() != 7200000 {
		t.Fatalf("compiler lost step-specific limits: %+v", limits)
	}
	source, err := os.ReadFile("../../examples/authoring/step-authoring-reference.yaml")
	if err != nil {
		t.Fatal(err)
	}
	data, err := StepJSONBytes(source, "yaml")
	if err != nil {
		t.Fatal(err)
	}
	var step StepDefinition
	if err := json.Unmarshal(data, &step); err != nil {
		t.Fatal(err)
	}
	if err := (&Plan{}).checkWorkspaceTrees(step, "/step"); err != nil {
		t.Fatalf("timed step lost workspace-tree support: %v", err)
	}
}

func TestSessionLimitsEditorSchemaMatchesAuthoring(t *testing.T) {
	data, err := os.ReadFile("../runtime/authoring/step-v2.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	schema, err := Parse(data, "json")
	if err != nil {
		t.Fatal(err)
	}
	compiler := newSchemaCompiler()
	const url = "urn:prifly:yaml-authoring:step:2"
	if err := compiler.AddResource(url, schema); err != nil {
		t.Fatal(err)
	}
	validator, err := compiler.Compile(url)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		marker, limits string
		valid          bool
	}{
		{StepAuthoringVersion, "", true},
		{StepSessionAuthoringVersion, "", true},
		{StepSessionAuthoringVersion, "session_limits: {active_timeout_ms: 7200000, decision_wait_timeout_ms: null}", true},
		{StepSessionAuthoringVersion, "session_limits: {decision_wait_timeout_ms: 1209600000}", true},
		{StepSessionAuthoringVersion, "session_limits: {active_timeout_ms: null}", true},
		{StepSessionAuthoringVersion, "schema_version: '7'\nsession_limits: {active_timeout_ms: null}", true},
		{StepSessionAuthoringVersion, "schema_version: '8'", false},
		{StepAuthoringVersion, "session_limits: {}", false},
		{StepSessionAuthoringVersion, "session_limits: {active_timeout_ms: 0}", false},
		{StepSessionAuthoringVersion, "session_limits: {decision_wait_timeout_ms: -1}", false},
		{StepSessionAuthoringVersion, "session_limits: {active_timeout_ms: 9223372036855}", false},
		{StepSessionAuthoringVersion, "session_limits: {extra: 1}", false},
	} {
		value, err := Parse(sessionLimitSource(test.marker, test.limits), "yaml")
		if err != nil {
			t.Fatal(err)
		}
		if err := validator.Validate(value); (err == nil) != test.valid {
			t.Fatalf("editor disagrees with %s %q: %v", test.marker, test.limits, err)
		}
	}
}

// A step written against prifly-step/1 that declares session limits is not a
// typo: it is a step written against the wrong contract, and the author meant
// the limits. Answering "field is not part of prifly-step/1" sends them to
// delete the line they meant to keep, and an inserted step that quietly loses
// its limits inherits the default hour without anyone choosing it.
func TestLimitsUnderTheOlderAuthoringNameTheContractThatCarriesThem(t *testing.T) {
	source := map[string]any{
		"authoring": StepAuthoringVersion,
		"id":        "test:step/tests",
		"version":   "1.0.0",
		"kind":      "worker",
		"inputs":    map[string]any{},
		"outputs":   map[string]any{},
		// The value is irrelevant: the field itself is the thing this contract
		// does not carry.
		"session_limits": map[string]any{"active_timeout_ms": nil},
	}
	_, err := lowerStepAuthoring(source)
	if err == nil {
		t.Fatal("the older authoring accepted a field it does not declare")
	}
	message := err.Error()
	for _, expected := range []string{"session_limits", StepSessionAuthoringVersion} {
		if !strings.Contains(message, expected) {
			t.Fatalf("the refusal does not name %q, so the author cannot tell a wrong contract from a wrong line: %s", expected, message)
		}
	}
	// A genuinely unknown field keeps the plain refusal: naming a contract that
	// does not carry it either would be a second wrong answer.
	source["session_limits"] = nil
	delete(source, "session_limits")
	source["not_a_field"] = true
	if _, err := lowerStepAuthoring(source); err == nil || strings.Contains(err.Error(), StepSessionAuthoringVersion) {
		t.Fatalf("an unknown field was blamed on the session contract: %v", err)
	}
}
