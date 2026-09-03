package flow

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"unicode/utf8"
)

const CheckDefinitionVersion = "check-definition/1"

// Executor identifies an operation, not permission to run it. Configuration
// and the executable/environment snapshot are pinned by runtime admission.
// The historical StepDefinition keeps its anonymous executor wire shape.
type Executor struct {
	AdapterRef Ref    `json:"adapter_ref"`
	Operation  string `json:"operation"`
}

// CheckDefinition declares one automatic content or result check. It is not a
// StepDefinition: a CheckExecution has no invented Stage or producer Attempt.
// The definition itself is the exact method reference used by its Evidence.
type CheckDefinition struct {
	SchemaVersion string   `json:"schema_version"`
	ID            string   `json:"id"`
	Version       string   `json:"version"`
	Title         string   `json:"title"`
	Kind          string   `json:"kind"`
	Claim         string   `json:"claim"`
	Executor      Executor `json:"executor"`
}

// ParseCheckDefinition validates a closed bootstrap contract using the fixed
// protocol primitives. It performs no dependency lookup, I/O or execution.
func ParseCheckDefinition(data []byte) (CheckDefinition, error) {
	value, err := Parse(data, "json")
	if err != nil {
		return CheckDefinition{}, err
	}
	fields := []string{"schema_version", "id", "version", "title", "kind", "claim", "executor"}
	object, err := checkDefinitionObject(value, fields, "")
	if err != nil {
		return CheckDefinition{}, err
	}
	if object["schema_version"] != CheckDefinitionVersion {
		return CheckDefinition{}, problem("invalid_check_definition", "/schema_version", "unsupported check definition version")
	}
	for _, field := range []struct{ name, schema string }{{"id", "Identifier"}, {"version", "Version"}} {
		if err := validateProtocolValue(field.schema, object[field.name], "/"+field.name); err != nil {
			return CheckDefinition{}, err
		}
	}
	title, ok := object["title"].(string)
	if !ok || utf8.RuneCountInString(title) < 1 || utf8.RuneCountInString(title) > 256 {
		return CheckDefinition{}, problem("invalid_check_definition", "/title", "title must contain 1 to 256 characters")
	}
	kind, _ := object["kind"].(string)
	claim, _ := object["claim"].(string)
	if kind != "content" && kind != "result" {
		return CheckDefinition{}, problem("invalid_check_definition", "/kind", "expected content or result")
	}
	if kind == "content" && claim != "content_valid" || kind == "result" && claim != "check_passed" && claim != "semantic_review" {
		return CheckDefinition{}, problem("invalid_check_definition", "/claim", "claim does not match the declared check kind")
	}
	executor, err := checkDefinitionObject(object["executor"], []string{"adapter_ref", "operation"}, "/executor")
	if err != nil {
		return CheckDefinition{}, err
	}
	if err := validateProtocolValue("ImmutableRef", executor["adapter_ref"], "/executor/adapter_ref"); err != nil {
		return CheckDefinition{}, err
	}
	if err := validateProtocolValue("Identifier", executor["operation"], "/executor/operation"); err != nil {
		return CheckDefinition{}, err
	}
	var definition CheckDefinition
	if err := decodeValue(value, &definition); err != nil {
		return CheckDefinition{}, problem("invalid_check_definition", "", "check definition cannot be decoded")
	}
	return definition, nil
}

func ValidateCheckDefinition(definition CheckDefinition) error {
	if !utf8.ValidString(definition.Title) {
		return problem("invalid_check_definition", "/title", "title must be valid UTF-8")
	}
	data, err := json.Marshal(definition)
	if err != nil {
		return err
	}
	_, err = ParseCheckDefinition(data)
	return err
}

// ParseSafeInteger proves mathematical integrality in the JSON safe range
// before JCS or float conversion. Equivalent decimal/exponent forms are valid;
// an almost-integer that would round to an integer is not.
func ParseSafeInteger(raw string) (int64, bool) {
	return controlInteger(raw)
}

func checkDefinitionObject(value any, fields []string, path string) (map[string]any, error) {
	object, ok := value.(map[string]any)
	if !ok {
		return nil, problem("invalid_check_definition", path, "expected a closed object")
	}
	for _, name := range keys(object) {
		if !slices.Contains(fields, name) {
			return nil, problem("invalid_check_definition", path+"/"+escapePointer(name), "undeclared check definition field")
		}
	}
	for _, name := range fields {
		if object[name] == nil {
			return nil, problem("invalid_check_definition", path+"/"+name, "required field is missing or null")
		}
	}
	return object, nil
}

func (p *Plan) loadChecks(refs []Ref, kind, path string) error {
	for i, ref := range refs {
		refPath := fmt.Sprintf("%s/%d", path, i)
		definition, exists := p.Checks[ref]
		if !exists {
			data, exists := p.Registry[ref]
			if !exists {
				return problem("missing_ref", refPath, "automatic check definition is not pinned")
			}
			var err error
			definition, err = ParseCheckDefinition(data)
			if err != nil {
				return checkDefinitionAt(err, refPath)
			}
			if definition.ID != ref.ID || definition.Version != ref.Version {
				return problem("ref_identity_mismatch", refPath, "check definition identity differs from its reference")
			}
			p.Checks[ref] = definition
		}
		if definition.Kind != kind {
			return problem("check_kind_mismatch", refPath, "check kind does not match the declaring position")
		}
		if _, exists := p.Registry[definition.Executor.AdapterRef]; !exists {
			return problem("missing_ref", refPath+"/executor/adapter_ref", "check adapter dependency is not pinned")
		}
	}
	return nil
}

func checkDefinitionAt(err error, path string) error {
	var failure *Problem
	if errors.As(err, &failure) {
		return problem(failure.Code, path+failure.Path, failure.Message)
	}
	return problem("invalid_check_definition", path, "invalid automatic check definition")
}

func inputCheckCount(inputs map[string]InputPort) int64 {
	var count int64
	for _, port := range inputs {
		count += int64(len(port.ContentCheckRefs))
	}
	return count
}

// Static bounds assume every optional port is present. They cover failed
// prefixes too: a late negative check may follow all earlier check admissions.
// CheckExecutions consume controls, never synthetic StepInstances.
func (p *Plan) stepCheckCount(stageID string) int64 {
	if p.Checks == nil {
		return 0
	}
	step := p.Steps[stageID]
	count := inputCheckCount(step.Inputs) + int64(len(step.ResultCheckRefs))
	for _, port := range step.Outputs {
		count += int64(len(port.ContentCheckRefs))
	}
	return count
}

func (p *Plan) finishCheckCount(stage Stage) int64 {
	if p.Checks == nil {
		return 0
	}
	var count int64
	for name := range stage.OutputBindings {
		count += int64(len(p.Workflow.Outputs[name].ContentCheckRefs))
	}
	return count
}
