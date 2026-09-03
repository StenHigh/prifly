package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/stenhigh/prifly/internal/flow"
)

const (
	CheckRequestVersion = "check-request/1"
	CheckResultVersion  = "check-result/1"
	MaxCheckWireBytes   = 1 << 20
	MaxCheckSubjects    = 256
)

// CheckRequest describes one admitted check. Activation and producer Attempt
// are absent where the boundary has no such owner; no surrogate IDs are used.
// Subject bytes, current permissions, the slot and expiry are checked by the
// runtime, not by this structural bootstrap protocol.
type CheckRequest struct {
	SchemaVersion      string        `json:"schema_version"`
	CheckID            string        `json:"check_execution_id"`
	RunID              string        `json:"run_id"`
	InvocationID       string        `json:"workflow_invocation_id"`
	ActivationID       string        `json:"stage_activation_id,omitempty"`
	ProducerAttemptID  string        `json:"producer_attempt_id,omitempty"`
	Boundary           string        `json:"boundary"`
	Port               string        `json:"port,omitempty"`
	CheckRef           flow.Ref      `json:"check_ref"`
	WorkflowRef        flow.Ref      `json:"workflow_ref"`
	PolicyRef          flow.Ref      `json:"policy_ref"`
	AdmissionID        string        `json:"admission_id"`
	AdmittedVersion    int64         `json:"admitted_run_version"`
	ControlEpoch       int64         `json:"control_epoch"`
	PackageLockDigest  string        `json:"package_lock_digest"`
	Subjects           []ArtifactRef `json:"subjects"`
	CandidateRef       *ArtifactRef  `json:"candidate_ref,omitempty"`
	ContextManifestRef ArtifactRef   `json:"context_manifest_ref"`
	DispatchNotAfter   string        `json:"dispatch_not_after"`
	CheckDeadline      string        `json:"check_deadline"`
}

// CheckResult is the checker report, not an OS settlement or permission to
// accept a producer's result. Only pass can satisfy the required check; fail
// and inconclusive remain valid reports. The claim comes from CheckDefinition.
type CheckResult struct {
	SchemaVersion string   `json:"schema_version"`
	CheckID       string   `json:"check_execution_id"`
	RunID         string   `json:"run_id"`
	RequestDigest string   `json:"request_digest"`
	Status        string   `json:"status"`
	Summary       string   `json:"summary"`
	Limitations   []string `json:"limitations"`
}

func ParseCheckRequest(data []byte) (CheckRequest, error) {
	const code = "invalid_check_request"
	required := []string{"schema_version", "check_execution_id", "run_id", "workflow_invocation_id", "boundary", "check_ref", "workflow_ref", "policy_ref", "admission_id", "admitted_run_version", "control_epoch", "package_lock_digest", "subjects", "context_manifest_ref", "dispatch_not_after", "check_deadline"}
	optional := []string{"stage_activation_id", "producer_attempt_id", "port", "candidate_ref"}
	object, err := checkProtocolObject(data, code, required, optional)
	if err != nil {
		return CheckRequest{}, err
	}
	if object["schema_version"] != CheckRequestVersion {
		return CheckRequest{}, checkProtocolError(code, "/schema_version", "unsupported check request version")
	}
	for _, name := range []string{"check_execution_id", "run_id", "workflow_invocation_id", "admission_id", "stage_activation_id", "producer_attempt_id"} {
		if value, exists := object[name]; exists {
			if err := checkProtocolPrimitive("Identifier", value, code, "/"+name); err != nil {
				return CheckRequest{}, err
			}
		}
	}
	for _, name := range []string{"check_ref", "workflow_ref", "policy_ref"} {
		if err := checkProtocolPrimitive("ImmutableRef", object[name], code, "/"+name); err != nil {
			return CheckRequest{}, err
		}
	}
	if err := checkProtocolPrimitive("Digest", object["package_lock_digest"], code, "/package_lock_digest"); err != nil {
		return CheckRequest{}, err
	}
	for _, field := range []struct {
		name string
		min  int64
	}{{"admitted_run_version", 1}, {"control_epoch", 0}} {
		if err := checkProtocolInteger(object[field.name], field.min, code, "/"+field.name); err != nil {
			return CheckRequest{}, err
		}
	}
	if err := checkProtocolArtifactRef(object["context_manifest_ref"], code, "/context_manifest_ref"); err != nil {
		return CheckRequest{}, err
	}
	subjects, ok := object["subjects"].([]any)
	if !ok || len(subjects) > MaxCheckSubjects {
		return CheckRequest{}, checkProtocolError(code, "/subjects", "expected at most 256 exact subject references")
	}
	for i, subject := range subjects {
		if err := checkProtocolArtifactRef(subject, code, fmt.Sprintf("/subjects/%d", i)); err != nil {
			return CheckRequest{}, err
		}
	}
	if value, exists := object["candidate_ref"]; exists {
		if err := checkProtocolArtifactRef(value, code, "/candidate_ref"); err != nil {
			return CheckRequest{}, err
		}
	}
	if value, exists := object["port"]; exists {
		if err := checkProtocolPrimitive("PortName", value, code, "/port"); err != nil {
			return CheckRequest{}, err
		}
	}
	boundary, _ := object["boundary"].(string)
	if !slices.Contains([]string{"workflow_input", "step_input", "step_output", "workflow_output", "step_result", "artifact_publication"}, boundary) {
		return CheckRequest{}, checkProtocolError(code, "/boundary", "unsupported check boundary")
	}
	if boundary == "step_result" {
		if object["candidate_ref"] == nil || object["port"] != nil {
			return CheckRequest{}, checkProtocolError(code, "/candidate_ref", "result check requires a candidate reference and no port")
		}
	} else if len(subjects) != 1 || object["port"] == nil || object["candidate_ref"] != nil {
		return CheckRequest{}, checkProtocolError(code, "/subjects", "content check requires one subject and a port, without a candidate reference")
	}
	if boundary != "workflow_input" && object["stage_activation_id"] == nil {
		return CheckRequest{}, checkProtocolError(code, "/stage_activation_id", "this boundary requires its owning stage activation")
	}
	if boundary == "workflow_input" && object["stage_activation_id"] != nil {
		return CheckRequest{}, checkProtocolError(code, "/stage_activation_id", "workflow input check has no owning stage activation")
	}
	if boundary != "step_output" && boundary != "step_result" && boundary != "artifact_publication" && object["producer_attempt_id"] != nil {
		return CheckRequest{}, checkProtocolError(code, "/producer_attempt_id", "this boundary has no producer attempt")
	}
	if (boundary == "step_output" || boundary == "step_result" || boundary == "artifact_publication") && object["producer_attempt_id"] == nil {
		return CheckRequest{}, checkProtocolError(code, "/producer_attempt_id", "producer result boundary requires its owning attempt")
	}
	var timestamps [2]time.Time
	for i, name := range []string{"dispatch_not_after", "check_deadline"} {
		if err := checkProtocolPrimitive("Timestamp", object[name], code, "/"+name); err != nil {
			return CheckRequest{}, err
		}
		timestamps[i], err = time.Parse(time.RFC3339Nano, object[name].(string))
		if err != nil {
			return CheckRequest{}, checkProtocolError(code, "/"+name, "invalid UTC deadline")
		}
	}
	if timestamps[0].After(timestamps[1]) {
		return CheckRequest{}, checkProtocolError(code, "/check_deadline", "check deadline precedes its dispatch deadline")
	}
	var request CheckRequest
	// Every counter/revision was proved exactly integral above. Only then may
	// JCS normalize equivalent forms such as 3.0 and 3e0 for Go integer fields.
	canonical, err := flow.Canonical(data)
	if err != nil {
		return CheckRequest{}, err
	}
	if err := json.Unmarshal(canonical, &request); err != nil {
		return CheckRequest{}, checkProtocolError(code, "", "check request cannot be decoded")
	}
	seen := make(map[ArtifactRef]bool, len(request.Subjects))
	for i, ref := range request.Subjects {
		identity := ref
		identity.Digest = ""
		if seen[identity] {
			return CheckRequest{}, checkProtocolError(code, fmt.Sprintf("/subjects/%d", i), "subject revision identity is duplicated or conflicting")
		}
		seen[identity] = true
	}
	return request, nil
}

func ValidateCheckRequest(request CheckRequest) error {
	data, err := json.Marshal(request)
	if err != nil {
		return err
	}
	_, err = ParseCheckRequest(data)
	return err
}

// ParseCheckResult binds a report to the exact persisted request bytes. Even
// a semantically equal request with different whitespace has another digest.
func ParseCheckResult(data, requestBytes []byte) (CheckResult, error) {
	const code = "invalid_check_result"
	request, err := ParseCheckRequest(requestBytes)
	if err != nil {
		return CheckResult{}, err
	}
	object, err := checkProtocolObject(data, code, []string{"schema_version", "check_execution_id", "run_id", "request_digest", "status", "summary", "limitations"}, nil)
	if err != nil {
		return CheckResult{}, err
	}
	if object["schema_version"] != CheckResultVersion {
		return CheckResult{}, checkProtocolError(code, "/schema_version", "unsupported check result version")
	}
	for _, name := range []string{"check_execution_id", "run_id"} {
		if err := checkProtocolPrimitive("Identifier", object[name], code, "/"+name); err != nil {
			return CheckResult{}, err
		}
	}
	if err := checkProtocolPrimitive("Digest", object["request_digest"], code, "/request_digest"); err != nil {
		return CheckResult{}, err
	}
	status, _ := object["status"].(string)
	if !slices.Contains([]string{"pass", "fail", "inconclusive"}, status) {
		return CheckResult{}, checkProtocolError(code, "/status", "expected pass, fail or inconclusive")
	}
	summary, ok := object["summary"].(string)
	if !ok || utf8.RuneCountInString(summary) > 16000 {
		return CheckResult{}, checkProtocolError(code, "/summary", "summary exceeds 16000 characters or is not a string")
	}
	limitations, ok := object["limitations"].([]any)
	if !ok || len(limitations) > 128 {
		return CheckResult{}, checkProtocolError(code, "/limitations", "expected at most 128 limitations")
	}
	for i, value := range limitations {
		limitation, ok := value.(string)
		if !ok || utf8.RuneCountInString(limitation) < 1 || utf8.RuneCountInString(limitation) > 4096 {
			return CheckResult{}, checkProtocolError(code, fmt.Sprintf("/limitations/%d", i), "limitation must contain 1 to 4096 characters")
		}
	}
	if object["check_execution_id"] != request.CheckID || object["run_id"] != request.RunID || object["request_digest"] != rawDigest(requestBytes) {
		return CheckResult{}, checkProtocolError("check_result_identity_mismatch", "", "report does not belong to the exact admitted check request")
	}
	var result CheckResult
	if err := json.Unmarshal(data, &result); err != nil {
		return CheckResult{}, checkProtocolError(code, "", "check result cannot be decoded")
	}
	return result, nil
}

func ValidateCheckResult(result CheckResult, requestBytes []byte) error {
	if !utf8.ValidString(result.Summary) {
		return checkProtocolError("invalid_check_result", "/summary", "summary must be valid UTF-8")
	}
	for i, limitation := range result.Limitations {
		if !utf8.ValidString(limitation) {
			return checkProtocolError("invalid_check_result", fmt.Sprintf("/limitations/%d", i), "limitation must be valid UTF-8")
		}
	}
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = ParseCheckResult(data, requestBytes)
	return err
}

func checkProtocolObject(data []byte, code string, required, optional []string) (map[string]any, error) {
	if len(data) > MaxCheckWireBytes {
		return nil, checkProtocolError(code, "", "check message exceeds 1 MiB")
	}
	value, err := flow.Parse(data, "json")
	if err != nil {
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, checkProtocolError(code, "", "expected a closed check object")
	}
	names := make([]string, 0, len(object))
	for name := range object {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if !slices.Contains(required, name) && !slices.Contains(optional, name) {
			return nil, checkProtocolError(code, "", "undeclared check message field")
		}
		if object[name] == nil {
			return nil, checkProtocolError(code, "/"+name, "check message field cannot be null")
		}
	}
	for _, name := range required {
		if _, exists := object[name]; !exists {
			return nil, checkProtocolError(code, "/"+name, "required check message field is missing")
		}
	}
	return object, nil
}

func checkProtocolPrimitive(schema string, value any, code, path string) error {
	data, err := json.Marshal(value)
	if err == nil {
		err = flow.ValidateProtocol(schema, data)
	}
	if err == nil {
		return nil
	}
	var failure *flow.Problem
	if errors.As(err, &failure) {
		return checkProtocolError(code, path+failure.Path, failure.Message)
	}
	return checkProtocolError(code, path, "invalid protocol value")
}

func checkProtocolArtifactRef(value any, code, path string) error {
	if err := checkProtocolPrimitive("ArtifactRef", value, code, path); err != nil {
		return err
	}
	return checkProtocolInteger(value.(map[string]any)["revision"], 1, code, path+"/revision")
}

func checkProtocolInteger(value any, minimum int64, code, path string) error {
	number, ok := value.(json.Number)
	if !ok {
		return checkProtocolError(code, path, "expected a safe JSON integer")
	}
	integer, exact := flow.ParseSafeInteger(string(number))
	if !exact || integer < minimum {
		return checkProtocolError(code, path, "expected an exact safe integer in the declared range")
	}
	return nil
}

func checkProtocolError(code, path, message string) error {
	if len(path) > 1024 {
		path = strings.TrimSuffix(path[:strings.LastIndex(path[:1024], "/")+1], "/")
	}
	return &flow.Problem{Code: code, Path: path, Message: message}
}
