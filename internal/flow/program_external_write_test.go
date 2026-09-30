package flow

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// programSource is a program step as a project writes it. The boundary names
// are placeholders: which system, operations and target is the author's.
func programSource(extra string) string {
	digest := "sha256:" + strings.Repeat("0", 64)
	return `authoring: prifly-step/1
id: test:step/program
version: 1.0.0
refs:
  adapter: {id: core:adapter/local-process, version: 2.0.0, digest: ` + digest + `}
  result: {id: test:schema/result, version: 1.0.0, digest: ` + digest + `}
kind: worker
executor: {adapter_ref: adapter, operation: process}
result_schema_ref: result
` + extra
}

const programBoundary = "external_write: {system: example-system, operations: [stack.up, stack.down], target: example/target}\n"

// A program that changes an external system says so in contract 14. Before it
// the pilot's tests step had to declare class none, and the Run's journal said
// nothing changed while the program created and removed containers.
func TestAProgramExternalWriteLowersToV14(t *testing.T) {
	t.Parallel()
	for _, pinned := range []string{"", "schema_version: '14'\n"} {
		source := programSource(pinned + "effects: {class: external_write, retry_class: idempotent}\n" + programBoundary)
		data, err := StepJSONBytes([]byte(source), "yaml")
		if err != nil {
			t.Fatal(err)
		}
		if err := ValidateProtocol("StepDefinitionV14", data); err != nil {
			t.Fatal(err)
		}
		var step StepDefinition
		if err := json.Unmarshal(data, &step); err != nil {
			t.Fatal(err)
		}
		if step.SchemaVersion != "14" || step.ExternalWrite == nil || step.ExternalWrite.Target != "example/target" || len(step.ExternalWrite.Operations) != 2 {
			t.Fatalf("the boundary was not sealed as v14: %+v", step)
		}
	}
	// A program without the field keeps the contract it had, byte for byte.
	data, err := StepJSONBytes([]byte(programSource("effects: {class: none, retry_class: never}\n")), "yaml")
	if err != nil {
		t.Fatal(err)
	}
	var step StepDefinition
	if err := json.Unmarshal(data, &step); err != nil || step.SchemaVersion != "2" || step.ExternalWrite != nil {
		t.Fatalf("a program without a boundary changed contract: %+v %v", step, err)
	}
}

// Each refusal names what the author should write instead.
func TestAProgramExternalWriteIsRefusedWhereItCannotBeCarried(t *testing.T) {
	t.Parallel()
	effects := "effects: {class: external_write, retry_class: idempotent}\n"
	for _, test := range []struct {
		name, source, path, names string
	}{
		{"pinned below 14", programSource("schema_version: '12'\n" + effects + programBoundary), "/schema_version", "v14"},
		{"an assisted step under prifly-step/1", strings.Replace(programSource(effects+programBoundary), "operation: process", "operation: session", 1), "/external_write", StepSessionAuthoringVersion},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := StepJSONBytes([]byte(test.source), "yaml")
			p := expectProblem(t, err, "schema_invalid")
			if p.Path != test.path || !strings.Contains(p.Message, test.names) {
				t.Fatalf("the refusal does not name %s: %s %s", test.names, p.Path, p.Message)
			}
		})
	}
	// The pilot's third attempt: a program under the session marker is sent to
	// the marker that carries its external write.
	_, err := StepJSONBytes([]byte(strings.Replace(programSource(effects+programBoundary), "prifly-step/1", StepSessionAuthoringVersion, 1)), "yaml")
	p := expectProblem(t, err, "schema_invalid")
	if !strings.Contains(p.Message, "also carries a program's external_write") {
		t.Fatalf("the refusal does not say where a program declares it: %s", p.Message)
	}
}

// The editor schema is what the author sees first; it must agree with the tool.
func TestProgramExternalWriteEditorSchemaMatchesAuthoring(t *testing.T) {
	t.Parallel()
	effects := "effects: {class: external_write, retry_class: idempotent}\n"
	for _, test := range []struct {
		name, source string
		valid        bool
	}{
		{"program with a boundary", programSource(effects + programBoundary), true},
		{"program pinned to 14", programSource("schema_version: '14'\n" + effects + programBoundary), true},
		{"program pinned below 14", programSource("schema_version: '12'\n" + effects + programBoundary), false},
		{"assisted under prifly-step/1", strings.Replace(programSource(effects+programBoundary), "operation: process", "operation: session", 1), false},
		{"empty operations", strings.Replace(programSource(effects+programBoundary), "[stack.up, stack.down]", "[]", 1), false},
	} {
		value, err := Parse([]byte(test.source), "yaml")
		if err != nil {
			t.Fatal(err)
		}
		// Both editor schemas: step-v2 is the one the editor maps to step
		// files, step-v1 is published for prifly-step/1 alone.
		for _, editor := range []struct{ file, url string }{
			{"../runtime/authoring/step-v2.schema.json", "urn:prifly:yaml-authoring:step:2"},
			{"../runtime/authoring/step-v1.schema.json", "urn:prifly:yaml-authoring:step:1"},
		} {
			data, err := os.ReadFile(editor.file)
			if err != nil {
				t.Fatal(err)
			}
			schema, err := Parse(data, "json")
			if err != nil {
				t.Fatal(err)
			}
			compiler := newSchemaCompiler()
			if err := compiler.AddResource(editor.url, schema); err != nil {
				t.Fatal(err)
			}
			validator, err := compiler.Compile(editor.url)
			if err != nil {
				t.Fatal(err)
			}
			if err := validator.Validate(value); (err == nil) != test.valid {
				t.Fatalf("%s: %s says %v", test.name, editor.file, err)
			}
		}
		// The tool is lowering and then the contract it lowered to, as the
		// compiler reads it.
		data, toolErr := StepJSONBytes([]byte(test.source), "yaml")
		if toolErr == nil {
			var step StepDefinition
			if toolErr = json.Unmarshal(data, &step); toolErr == nil {
				toolErr = ValidateProtocol(StepContractFor(step.SchemaVersion), data)
			}
		}
		if (toolErr == nil) != test.valid {
			t.Fatalf("%s: the tool disagrees with the editor: %v", test.name, toolErr)
		}
	}
}

// Contract 14 is the program branch: 12 plus the boundary, no session field,
// and the boundary is the same schema the assisted line declares in 11.
func TestContract14IsTheProgramBranchWithTheBoundary(t *testing.T) {
	t.Parallel()
	read := func(name string) map[string]any {
		schema, err := ProtocolSchema(name)
		if err != nil {
			t.Fatal(err)
		}
		var document map[string]any
		if err := json.Unmarshal(schema, &document); err != nil {
			t.Fatal(err)
		}
		return document["properties"].(map[string]any)
	}
	v14, v11 := read("StepDefinitionV14"), read("StepDefinitionV11")
	for _, assisted := range []string{"session_limits", "model_profile", "repository_workspace"} {
		if _, exists := v14[assisted]; exists {
			t.Errorf("contract 14 admits the assisted field %s", assisted)
		}
	}
	operation := v14["executor"].(map[string]any)["properties"].(map[string]any)["operation"].(map[string]any)
	if operation["const"] != "process" {
		t.Fatalf("contract 14 does not pin the program executor: %v", operation)
	}
	mine, _ := json.Marshal(v14["external_write"])
	theirs, _ := json.Marshal(v11["external_write"])
	if len(mine) == 0 || string(mine) != string(theirs) {
		t.Fatalf("the two lines bound an external write differently:\n%s\n%s", mine, theirs)
	}
}

// A build older than a step's contract refused it as "the declared value is 1",
// which sends the reader to downgrade the step. It now names the contract as
// newer than the build. Builds before this one keep the old text: a step
// contract 14 read by 0.13.64 is refused, but not by this name.
func TestAStepContractNewerThanThisBuildIsNamedNewer(t *testing.T) {
	t.Parallel()
	data, err := StepJSONBytes([]byte(programSource("effects: {class: none, retry_class: never}\n")), "yaml")
	if err != nil {
		t.Fatal(err)
	}
	future := []byte(strings.Replace(string(data), `"schema_version":"2"`, `"schema_version":"99"`, 1))
	ref := Ref{ID: "test:step/program", Version: "1.0.0", Digest: "sha256:" + strings.Repeat("1", 64)}
	p := &Plan{Registry: Registry{ref: future}, Profile: CoreProfile}
	_, err = p.loadStep(ref, "/steps/program")
	problem := expectProblem(t, err, "unsupported_contract")
	if !strings.Contains(problem.Message, "newer than this build") || !strings.Contains(problem.Message, StepContracts[len(StepContracts)-1]) {
		t.Fatalf("a newer step contract is not named newer: %s", problem.Message)
	}
}
