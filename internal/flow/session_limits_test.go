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
	for _, test := range []struct {
		name, limits string
		active, wait int64
	}{
		{"omitted", "", DefaultSessionActiveTimeoutMS, 0},
		{"empty", "session_limits: {}", DefaultSessionActiveTimeoutMS, 0},
		{"active only", "session_limits: {active_timeout_ms: 7200000}", 7200000, 0},
		{"wait only", "session_limits: {decision_wait_timeout_ms: 1209600000}", DefaultSessionActiveTimeoutMS, 1209600000},
		{"explicit null wait", "session_limits: {active_timeout_ms: 1, decision_wait_timeout_ms: null}", 1, 0},
		{"representation maximum", "session_limits: {active_timeout_ms: 9223372036854, decision_wait_timeout_ms: 9223372036854}", MaxSessionTimeoutMS, MaxSessionTimeoutMS},
	} {
		t.Run(test.name, func(t *testing.T) {
			data, err := StepJSONBytes(sessionLimitSource(StepSessionAuthoringVersion, test.limits), "yaml")
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateProtocol("StepDefinitionV6", data); err != nil {
				t.Fatal(err)
			}
			var step StepDefinition
			if err := json.Unmarshal(data, &step); err != nil {
				t.Fatal(err)
			}
			if step.SchemaVersion != "6" || step.SessionLimits == nil || step.SessionLimits.ActiveTimeoutMS != test.active || len(step.WorkspaceTrees) != 0 {
				t.Fatalf("limits were not pinned independently of workspace trees: %+v", step)
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
}

func TestSessionLimitsAuthoringRefusesInvalidContracts(t *testing.T) {
	for _, limits := range []string{
		"null", "[]", "{active_timeout_ms: null}", "{active_timeout_ms: 0}",
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
	} {
		t.Run(test.name, func(t *testing.T) {
			source := strings.Replace(string(sessionLimitSource(StepSessionAuthoringVersion, "")), test.before, test.after, 1)
			_, err := StepJSONBytes([]byte(source), "yaml")
			expectProblem(t, err, test.code)
		})
	}
	_, err := StepJSONBytes(sessionLimitSource(StepAuthoringVersion, "session_limits: {}"), "yaml")
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
	step.SessionLimits = &SessionLimits{ActiveTimeoutMS: DefaultSessionActiveTimeoutMS}
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
		if err := ValidateProtocol("StepDefinitionV6", encoded(t, value)); err == nil {
			t.Fatalf("machine definition did not require materialized %s", field)
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
		step.SessionLimits = &SessionLimits{ActiveTimeoutMS: 7200000}
	})
	plan, err := CompileCore(encoded(t, w), "json", registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	if limits := plan.Steps["work"].SessionLimits; limits == nil || limits.ActiveTimeoutMS != 7200000 {
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
