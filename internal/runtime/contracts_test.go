package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

func validatePublic(t *testing.T, name string, value any) error {
	t.Helper()
	schema, err := PublicSchema(name)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := flow.Digest(schema)
	if err != nil {
		t.Fatal(err)
	}
	ref := flow.Ref{ID: "test:schema/" + name, Version: "1.0.0", Digest: digest}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return flow.ValidateSchema(flow.Registry{ref: schema}, ref, data)
}
func TestPublicSchemasMatchActualReadViewsAndRejectExtensions(t *testing.T) {
	copy, err := os.ReadFile("../../schemas/foundation/public.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(copy, publicContracts) {
		t.Fatal("published and embedded schemas differ")
	}
	e, options := emptyRuntime(t)
	ctx := context.Background()
	result, err := e.Start(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	view, err := e.View(ctx, result.Receipt.RunID)
	if err != nil {
		t.Fatal(err)
	}
	next, err := e.Next(ctx, result.Receipt.RunID)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := e.Preview(PreviewOptions{WorkflowFile: options.WorkflowFile, BriefFile: options.BriefFile})
	if err != nil {
		t.Fatal(err)
	}
	report, err := e.Telemetry(ctx, TelemetryQuery{SchemaVersion: TelemetryQueryVersion, Mode: "catalog"})
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]any{"FoundationRunView": view, "FoundationNextView": next, "FoundationPreview": preview, "TimingTree": view.Timing, "TelemetryReport": report, "CommandReceipt": result.Receipt} {
		if err := validatePublic(t, name, value); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var object map[string]any
		if err := json.Unmarshal(data, &object); err != nil {
			t.Fatal(err)
		}
		object["surprise_extension"] = true
		if err := validatePublic(t, name, object); err == nil {
			t.Fatalf("%s accepted unknown top-level field", name)
		}
	}
	data, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if flow.ValidateProtocol("RunSnapshot", data) == nil {
		t.Fatal("new view silently masqueraded as baseline RunSnapshot")
	}
	legacy := map[string]any{"schema_version": "1", "id": view.Run.ID, "run_version": view.RunVersion, "status": view.Run.Status, "root_workflow_invocation_id": view.Run.RootInvocationID, "outcome": view.Run.Outcome, "workflow_ref": view.Run.WorkflowRef, "brief_ref": view.Run.Brief, "package_lock_ref": view.Run.LockRef, "control_epoch": view.Run.ControlEpoch, "active_attempt_ids": []string{}, "active_stop_ids": []string{}, "waiting_reasons": []string{}, "has_waivers": false, "has_unresolved_effects": false, "has_pending_effects": false, "output_artifacts": map[string]ArtifactRef{}}
	data, err = json.Marshal(legacy)
	if err != nil || flow.ValidateProtocol("RunSnapshot", data) != nil {
		t.Fatal("baseline RunSnapshot fixture is not independently valid", err)
	}
	legacy["telemetry"] = view.Timing
	data, err = json.Marshal(legacy)
	if err != nil || flow.ValidateProtocol("RunSnapshot", data) == nil {
		t.Fatal("telemetry silently extended the original closed RunSnapshot", err)
	}
}
func TestPublicationSchemaClosesVariants(t *testing.T) {
	zero := int64(0)
	command := PublishCommand{SchemaVersion: "1", CommandID: "command:publish", RunID: "run:one", StepID: "step:one", AttemptID: "attempt:one", EnvelopeDigest: rawDigest([]byte("envelope")), Hook: "progress", Kind: "state", ExpectedStateVersion: &zero, Value: json.RawMessage(`false`)}
	if err := validatePublic(t, "PublishStepPublicationCommand", command); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(command)
	var object map[string]any
	if err := json.Unmarshal(b, &object); err != nil {
		t.Fatal(err)
	}
	object["event_key"] = ""
	if err := validatePublic(t, "PublishStepPublicationCommand", object); err == nil {
		t.Fatal("state accepted a forbidden event field")
	}
	delete(object, "event_key")
	object["kind"] = "event"
	object["event_key"] = "occurrence:one"
	object["expected_state_version"] = nil
	if err := validatePublic(t, "PublishStepPublicationCommand", object); err == nil {
		t.Fatal("event accepted a null state CAS field")
	}
	delete(object, "expected_state_version")
	if err := validatePublic(t, "PublishStepPublicationCommand", object); err != nil {
		t.Fatal(err)
	}
}

// This checks only explicitly listed source bindings. Meaning, industry usage
// and completeness of the vocabulary still need review, not an AST assertion.
func checkGlossaryBindings(document string, sources *os.Root) error {
	const start, end = "<!-- glossary-bindings:start -->", "<!-- glossary-bindings:end -->"
	if strings.Count(document, start) != 1 || strings.Count(document, end) != 1 {
		return fmt.Errorf("glossary needs exactly one bindings marker pair")
	}
	from, to := strings.Index(document, start)+len(start), strings.Index(document, end)
	if from >= to {
		return fmt.Errorf("glossary bindings markers are out of order")
	}
	table := strings.Split(strings.TrimSpace(document[from:to]), "\n")
	if len(table) < 3 {
		return fmt.Errorf("glossary bindings table is empty")
	}
	unquote := func(cell string) (string, bool) {
		if len(cell) < 3 || cell[0] != '`' || cell[len(cell)-1] != '`' || strings.Contains(cell[1:len(cell)-1], "`") {
			return "", false
		}
		return cell[1 : len(cell)-1], true
	}
	files, seen := map[string]*ast.File{}, map[string]bool{}
	for line, text := range table {
		cells := strings.Split(strings.TrimSpace(text), "|")
		if len(cells) != 6 || strings.TrimSpace(cells[0]) != "" || strings.TrimSpace(cells[5]) != "" {
			return fmt.Errorf("glossary table line %d must have four columns", line+1)
		}
		cells = cells[1:5]
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		if line == 0 {
			if strings.Join(cells, "|") != "Термин|Исходник от корня|Go|JSON" {
				return fmt.Errorf("glossary bindings table has an unexpected header")
			}
			continue
		}
		if line == 1 {
			for _, cell := range cells {
				if strings.Trim(cell, "-:") != "" || strings.Count(cell, "-") < 3 {
					return fmt.Errorf("glossary bindings table has no Markdown separator")
				}
			}
			continue
		}
		link := cells[0]
		anchorAt := strings.Index(link, "](#")
		if !strings.HasPrefix(link, "[") || !strings.HasSuffix(link, ")") || anchorAt <= 1 || anchorAt+3 >= len(link)-1 {
			return fmt.Errorf("glossary table line %d needs a term anchor link", line+1)
		}
		anchor := link[anchorAt+3 : len(link)-1]
		if !strings.Contains(document, `<a id="`+anchor+`"></a>`) {
			return fmt.Errorf("glossary anchor %q does not exist", anchor)
		}
		file, fileOK := unquote(cells[1])
		goName, goOK := unquote(cells[2])
		if !fileOK || !goOK || path.Clean(file) != file || !strings.HasPrefix(file, "internal/") || !strings.HasSuffix(file, ".go") || strings.Contains(file, `\`) {
			return fmt.Errorf("glossary source must be an internal Go file with a quoted Go binding")
		}
		key := file + "\x00" + goName
		if seen[key] {
			return fmt.Errorf("duplicate glossary binding %s in %s", goName, file)
		}
		seen[key] = true
		parsed := files[file]
		if parsed == nil {
			// Root also rejects a symlink escaping internal; cleaning strings alone
			// is not permission to inspect some other source tree.
			data, err := sources.ReadFile(strings.TrimPrefix(file, "internal/"))
			if err != nil {
				return fmt.Errorf("read glossary source %s: %w", file, err)
			}
			parsed, err = parser.ParseFile(token.NewFileSet(), file, data, 0)
			if err != nil {
				return fmt.Errorf("parse glossary source %s: %w", file, err)
			}
			files[file] = parsed
		}
		parts := strings.Split(goName, ".")
		if (len(parts) != 2 && len(parts) != 3) || parts[0] != parsed.Name.Name {
			return fmt.Errorf("binding %s must name the source package and a type or direct field", goName)
		}
		var declared *ast.TypeSpec
		for _, declaration := range parsed.Decls {
			group, ok := declaration.(*ast.GenDecl)
			if !ok || group.Tok != token.TYPE {
				continue
			}
			for _, spec := range group.Specs {
				if name := spec.(*ast.TypeSpec); name.Name.Name == parts[1] {
					declared = name
				}
			}
		}
		if declared == nil {
			return fmt.Errorf("glossary type %s is absent from %s", goName, file)
		}
		if len(parts) == 2 {
			if cells[3] != "—" {
				return fmt.Errorf("type binding %s cannot declare a field JSON tag", goName)
			}
			continue
		}
		var field *ast.Field
		if structure, ok := declared.Type.(*ast.StructType); ok {
			for _, candidate := range structure.Fields.List {
				for _, name := range candidate.Names {
					if name.Name == parts[2] {
						field = candidate
					}
				}
			}
		}
		if field == nil || field.Tag == nil {
			return fmt.Errorf("glossary field %s has no direct declaration with a JSON tag", goName)
		}
		tag, err := strconv.Unquote(field.Tag.Value)
		if err != nil {
			return fmt.Errorf("invalid Go struct tag on %s: %w", goName, err)
		}
		jsonTag, exists := reflect.StructTag(tag).Lookup("json")
		jsonName := strings.Split(jsonTag, ",")[0]
		if jsonName == "" {
			jsonName = parts[2] // encoding/json's name when only options are set.
		}
		want, quoted := unquote(cells[3])
		if !exists || !quoted || want != jsonName {
			return fmt.Errorf("glossary JSON binding for %s disagrees with its struct tag", goName)
		}
	}
	return nil
}

func TestGlossaryBindings(t *testing.T) {
	data, err := os.ReadFile("../../openspec/specs/specification-governance/terms.md")
	if err != nil {
		t.Fatal(err)
	}
	sources, err := os.OpenRoot("..")
	if err != nil {
		t.Fatal(err)
	}
	defer sources.Close()
	document := string(data)
	if err := checkGlossaryBindings(document, sources); err != nil {
		t.Fatal(err)
	}
	const start, end = "<!-- glossary-bindings:start -->", "<!-- glossary-bindings:end -->"
	from, to := strings.Index(document, start)+len(start), strings.Index(document, end)
	table := document[from:to]
	var typeRow, fieldRow string
	for _, line := range strings.Split(table, "\n") {
		cells := strings.Split(line, "|")
		if len(cells) != 6 {
			continue
		}
		goName := strings.Trim(strings.TrimSpace(cells[3]), "`")
		if strings.Count(goName, ".") == 1 && typeRow == "" {
			typeRow = line
		}
		if strings.Count(goName, ".") == 2 && fieldRow == "" {
			fieldRow = line
		}
	}
	if typeRow == "" || fieldRow == "" {
		t.Fatal("glossary needs type and direct-field examples to exercise its binding checks")
	}
	typeName := strings.Trim(strings.TrimSpace(strings.Split(typeRow, "|")[3]), "`")
	fieldName := strings.Trim(strings.TrimSpace(strings.Split(fieldRow, "|")[3]), "`")
	for _, test := range []struct {
		name, row  string
		column     int
		value, why string
	}{
		{"unknown type", typeRow, 3, "`" + strings.Split(typeName, ".")[0] + ".GlossaryMissingType`", "glossary type"},
		{"unknown field", fieldRow, 3, "`" + fieldName[:strings.LastIndex(fieldName, ".")] + ".GlossaryMissingField`", "glossary field"},
		{"wrong JSON tag", fieldRow, 4, "`glossary_wrong_json_tag`", "JSON binding"},
		{"missing anchor", typeRow, 1, "[Missing](#glossary-missing-anchor)", "anchor"},
		{"path escape", typeRow, 2, "`internal/../cmd/prifly/main.go`", "source"},
	} {
		t.Run(test.name, func(t *testing.T) {
			cells := strings.Split(test.row, "|")
			cells[test.column] = " " + test.value + " "
			changed := strings.Replace(table, test.row, strings.Join(cells, "|"), 1)
			if err := checkGlossaryBindings(document[:from]+changed+document[to:], sources); err == nil || !strings.Contains(err.Error(), test.why) {
				t.Fatalf("expected %s rejection, got %v", test.why, err)
			}
		})
	}
	for name, invalid := range map[string]string{
		"missing markers": strings.Replace(document, start, "", 1),
		"empty table":     document[:from] + "\n" + document[to:],
		"duplicate row":   document[:to] + typeRow + "\n" + document[to:],
	} {
		t.Run(name, func(t *testing.T) {
			if err := checkGlossaryBindings(invalid, sources); err == nil {
				t.Fatal("malformed glossary table was accepted")
			}
		})
	}
}
