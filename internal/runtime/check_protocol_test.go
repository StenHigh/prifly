package runtime

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

func checkProtocolRef(id string, fill byte) flow.Ref {
	return flow.Ref{ID: id, Version: "1.0.0", Digest: "sha256:" + strings.Repeat(string(fill), 64)}
}

func checkArtifactRef(id string, fill byte) ArtifactRef {
	return ArtifactRef{ArtifactID: id, Revision: 1, Digest: "sha256:" + strings.Repeat(string(fill), 64)}
}

func checkRequestFixture(boundary string) CheckRequest {
	request := CheckRequest{
		SchemaVersion:      CheckRequestVersion,
		CheckID:            "check:example",
		RunID:              "run:example",
		InvocationID:       "invocation:root",
		ActivationID:       "activation:work",
		Boundary:           boundary,
		Port:               "document",
		CheckRef:           checkProtocolRef("example:check/pdf", 'a'),
		WorkflowRef:        checkProtocolRef("example:workflow", 'b'),
		PolicyRef:          checkProtocolRef("core:policy/local", 'c'),
		AdmissionID:        "admission:check",
		AdmittedVersion:    3,
		ControlEpoch:       0,
		PackageLockDigest:  "sha256:" + strings.Repeat("d", 64),
		Subjects:           []ArtifactRef{checkArtifactRef("artifact:subject", 'e')},
		ContextManifestRef: checkArtifactRef("artifact:context", 'f'),
		DispatchNotAfter:   "2026-08-28T09:00:00Z",
		CheckDeadline:      "2026-08-28T09:01:00Z",
	}
	if boundary == "workflow_input" {
		request.ActivationID = ""
	}
	if boundary == "step_output" {
		request.ProducerAttemptID = "attempt:producer"
	}
	if boundary == "step_result" {
		request.ProducerAttemptID = "attempt:producer"
		request.Port = ""
		request.Subjects = []ArtifactRef{}
		candidate := checkArtifactRef("artifact:candidate", '1')
		request.CandidateRef = &candidate
	}
	return request
}

func checkRequestBytes(t *testing.T, request any) []byte {
	t.Helper()
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func checkProtocolProblem(t *testing.T, err error, code string) *flow.Problem {
	t.Helper()
	var problem *flow.Problem
	if !errors.As(err, &problem) || problem.Code != code {
		t.Fatalf("expected %s problem, got %v", code, err)
	}
	return problem
}

func TestCheckRequestBoundariesAndExactSubjects(t *testing.T) {
	for _, boundary := range []string{"workflow_input", "step_input", "step_output", "workflow_output", "step_result"} {
		t.Run(boundary, func(t *testing.T) {
			request := checkRequestFixture(boundary)
			data := checkRequestBytes(t, request)
			parsed, err := ParseCheckRequest(data)
			if err != nil || parsed.CheckID != request.CheckID || ValidateCheckRequest(request) != nil {
				t.Fatalf("valid %s request was rejected: %+v %v", boundary, parsed, err)
			}
		})
	}

	request := checkRequestFixture("step_result")
	request.Subjects = make([]ArtifactRef, MaxCheckSubjects)
	for i := range request.Subjects {
		request.Subjects[i] = checkArtifactRef("artifact:subject:"+strconv.Itoa(i), '2')
	}
	if err := ValidateCheckRequest(request); err != nil {
		t.Fatal("256 result subjects should be valid", err)
	}
	request.Subjects = append(request.Subjects, checkArtifactRef("artifact:overflow", '3'))
	checkProtocolProblem(t, ValidateCheckRequest(request), "invalid_check_request")

	content := checkRequestFixture("step_output")
	content.Subjects = append(content.Subjects, checkArtifactRef("artifact:second", '4'))
	checkProtocolProblem(t, ValidateCheckRequest(content), "invalid_check_request")
	conflict := checkRequestFixture("step_result")
	conflict.Subjects = []ArtifactRef{checkArtifactRef("artifact:same", '5'), checkArtifactRef("artifact:same", '6')}
	checkProtocolProblem(t, ValidateCheckRequest(conflict), "invalid_check_request")
	conflict.Subjects[1] = conflict.Subjects[0]
	checkProtocolProblem(t, ValidateCheckRequest(conflict), "invalid_check_request")
}

func TestCheckRequestStrictRejections(t *testing.T) {
	request := checkRequestFixture("step_output")
	base := checkRequestBytes(t, request)
	mutations := map[string]func(map[string]any){
		"unknown_field":  func(v map[string]any) { v["next_stage_id"] = "stage:escape" },
		"missing_field":  func(v map[string]any) { delete(v, "check_ref") },
		"null_field":     func(v map[string]any) { v["subjects"] = nil },
		"wrong_version":  func(v map[string]any) { v["schema_version"] = "check-request/2" },
		"wrong_boundary": func(v map[string]any) { v["boundary"] = "live_file" },
		"case_folded_field": func(v map[string]any) {
			v["RUN_ID"] = v["run_id"]
			delete(v, "run_id")
		},
		"missing_port":         func(v map[string]any) { delete(v, "port") },
		"unexpected_candidate": func(v map[string]any) { v["candidate_ref"] = v["context_manifest_ref"] },
		"missing_activation":   func(v map[string]any) { delete(v, "stage_activation_id") },
		"missing_producer":     func(v map[string]any) { delete(v, "producer_attempt_id") },
		"future_epoch":         func(v map[string]any) { v["control_epoch"] = float64(9007199254740992) },
		"fractional_version":   func(v map[string]any) { v["admitted_run_version"] = 3.5 },
		"invalid_ref": func(v map[string]any) {
			v["subjects"].([]any)[0].(map[string]any)["revision"] = 0
		},
		"late_dispatch": func(v map[string]any) { v["dispatch_not_after"] = "2026-08-28T09:02:00Z" },
		"non_utc":       func(v map[string]any) { v["check_deadline"] = "2026-08-28T12:01:00+03:00" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			var value map[string]any
			if err := json.Unmarshal(base, &value); err != nil {
				t.Fatal(err)
			}
			mutate(value)
			data, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ParseCheckRequest(data); err == nil {
				t.Fatal("invalid request was accepted")
			}
		})
	}
	if _, err := ParseCheckRequest([]byte(`{"schema_version":"check-request/1","schema_version":"check-request/1"}`)); err == nil {
		t.Fatal("duplicate key was accepted")
	}
	oversized := append(append([]byte{}, base...), make([]byte, MaxCheckWireBytes+1)...)
	checkProtocolProblem(t, func() error { _, err := ParseCheckRequest(oversized); return err }(), "invalid_check_request")
}

func TestCheckRequestDoesNotInventBoundaryOwners(t *testing.T) {
	for _, boundary := range []string{"workflow_input", "step_input", "workflow_output"} {
		t.Run(boundary, func(t *testing.T) {
			request := checkRequestFixture(boundary)
			request.ProducerAttemptID = "attempt:unrelated"
			failure := checkProtocolProblem(t, ValidateCheckRequest(request), "invalid_check_request")
			if failure.Path != "/producer_attempt_id" {
				t.Fatal("unexpected owner error", failure)
			}
		})
	}
	request := checkRequestFixture("workflow_input")
	request.ActivationID = "activation:unrelated"
	failure := checkProtocolProblem(t, ValidateCheckRequest(request), "invalid_check_request")
	if failure.Path != "/stage_activation_id" {
		t.Fatal("workflow input accepted a surrogate stage owner", failure)
	}
}

func TestCheckRequestNumbersAreExactBeforeCanonicalization(t *testing.T) {
	base := string(checkRequestBytes(t, checkRequestFixture("step_result")))
	for _, raw := range []string{"3", "3.0", "3e0", "0.003e3"} {
		data := []byte(strings.Replace(base, `"admitted_run_version":3`, `"admitted_run_version":`+raw, 1))
		request, err := ParseCheckRequest(data)
		if err != nil || request.AdmittedVersion != 3 {
			t.Fatalf("equivalent integer %s failed: %+v %v", raw, request, err)
		}
	}
	for _, raw := range []string{"3.0000000000000000001", "9007199254740990.9", "9007199254740992", "3.5", "null", `"3"`} {
		data := []byte(strings.Replace(base, `"admitted_run_version":3`, `"admitted_run_version":`+raw, 1))
		if _, err := ParseCheckRequest(data); err == nil {
			t.Fatalf("invalid or rounded integer %s was accepted", raw)
		}
	}
	for _, raw := range []string{"1.0", "1e0"} {
		data := []byte(strings.ReplaceAll(base, `"revision":1`, `"revision":`+raw))
		if _, err := ParseCheckRequest(data); err != nil {
			t.Fatalf("equivalent artifact revision %s was rejected: %v", raw, err)
		}
	}
	data := []byte(strings.ReplaceAll(base, `"revision":1`, `"revision":1.0000000000000000001`))
	if _, err := ParseCheckRequest(data); err == nil {
		t.Fatal("artifact revision was rounded before exact preflight")
	}
}

func TestCheckResultBindsExactRequestBytes(t *testing.T) {
	request := checkRequestFixture("step_result")
	compact := checkRequestBytes(t, request)
	var indented []byte
	var object any
	if err := json.Unmarshal(compact, &object); err != nil {
		t.Fatal(err)
	}
	indented, _ = json.MarshalIndent(object, "", "  ")
	if rawDigest(compact) == rawDigest(indented) {
		t.Fatal("fixture does not exercise exact request bytes")
	}
	for _, status := range []string{"pass", "fail", "inconclusive"} {
		result := CheckResult{SchemaVersion: CheckResultVersion, CheckID: request.CheckID, RunID: request.RunID, RequestDigest: rawDigest(compact), Status: status, Summary: "checked exact subjects", Limitations: []string{}}
		data := checkRequestBytes(t, result)
		parsed, err := ParseCheckResult(data, compact)
		if err != nil || parsed.Status != status || ValidateCheckResult(result, compact) != nil {
			t.Fatalf("valid %s result was rejected: %+v %v", status, parsed, err)
		}
		checkProtocolProblem(t, func() error { _, err := ParseCheckResult(data, indented); return err }(), "check_result_identity_mismatch")
	}
}

func TestCheckResultStrictIdentityAndReportLimits(t *testing.T) {
	request := checkRequestFixture("step_result")
	requestBytes := checkRequestBytes(t, request)
	base := CheckResult{SchemaVersion: CheckResultVersion, CheckID: request.CheckID, RunID: request.RunID, RequestDigest: rawDigest(requestBytes), Status: "pass", Summary: "", Limitations: []string{}}
	baseBytes := checkRequestBytes(t, base)
	mutations := map[string]func(map[string]any){
		"unknown_field":    func(v map[string]any) { v["claim"] = "semantic_review" },
		"missing_field":    func(v map[string]any) { delete(v, "limitations") },
		"null_field":       func(v map[string]any) { v["limitations"] = nil },
		"wrong_version":    func(v map[string]any) { v["schema_version"] = "check-result/2" },
		"wrong_status":     func(v map[string]any) { v["status"] = "approved" },
		"wrong_check":      func(v map[string]any) { v["check_execution_id"] = "check:other" },
		"wrong_run":        func(v map[string]any) { v["run_id"] = "run:other" },
		"wrong_digest":     func(v map[string]any) { v["request_digest"] = "sha256:" + strings.Repeat("0", 64) },
		"long_summary":     func(v map[string]any) { v["summary"] = strings.Repeat("x", 16001) },
		"empty_limitation": func(v map[string]any) { v["limitations"] = []any{""} },
		"long_limitation":  func(v map[string]any) { v["limitations"] = []any{strings.Repeat("x", 4097)} },
		"too_many_limitations": func(v map[string]any) {
			values := make([]string, 129)
			for i := range values {
				values[i] = "limited coverage"
			}
			v["limitations"] = values
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			var value map[string]any
			if err := json.Unmarshal(baseBytes, &value); err != nil {
				t.Fatal(err)
			}
			mutate(value)
			data, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ParseCheckResult(data, requestBytes); err == nil {
				t.Fatal("invalid result was accepted")
			}
		})
	}
	validUnicode := base
	validUnicode.Summary = strings.Repeat("э", 16000)
	validUnicode.Limitations = []string{strings.Repeat("界", 4096)}
	if err := ValidateCheckResult(validUnicode, requestBytes); err != nil {
		t.Fatal("character limits were counted as bytes", err)
	}
	validUnicode.Summary = string([]byte{0xff})
	checkProtocolProblem(t, ValidateCheckResult(validUnicode, requestBytes), "invalid_check_result")
	validUnicode.Summary = ""
	validUnicode.Limitations = []string{string([]byte{0xff})}
	checkProtocolProblem(t, ValidateCheckResult(validUnicode, requestBytes), "invalid_check_result")
	oversized := append(append([]byte{}, baseBytes...), []byte(strings.Repeat(" ", MaxCheckWireBytes))...)
	_, err := ParseCheckResult(oversized, requestBytes)
	checkProtocolProblem(t, err, "invalid_check_result")
}
