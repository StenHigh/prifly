package flow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

func TestSchemaWorkerClosedTransport(t *testing.T) {
	for _, test := range []struct {
		name, request, code string
	}{
		{"compile boolean", `{"schema":false}`, ""},
		{"validate null", `{"schema":{"type":"integer"},"value":null}`, "schema_invalid"},
		{"valid scalar", `{"schema":{"type":"boolean"},"value":true}`, ""},
		{"missing schema", `{}`, "invalid_schema_request"},
		{"unknown field", `{"schema":true,"network":true}`, "invalid_schema_request"},
		{"duplicate field", `{"schema":true,"schema":false}`, "invalid_schema_request"},
		{"trailing document", `{"schema":true} {}`, "invalid_schema_request"},
		{"invalid syntax", `{"schema":`, "invalid_schema_request"},
		{"nonobject transport", `[]`, "invalid_schema_request"},
		{"duplicate value key", `{"schema":true,"value":{"x":1,"x":2}}`, "duplicate_key"},
		{"depth", `{"schema":true,"value":` + strings.Repeat("[", MaxDepth+1) + `0` + strings.Repeat("]", MaxDepth+1) + `}`, "document_limit"},
		{"outside loader", `{"schema":{"$ref":"file:///SECRET/schema.json"}}`, "invalid_schema"},
		{"transport allowance", strings.Repeat(" ", schemaRequestLimit+1), "invalid_schema_request"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out bytes.Buffer
			handled, exit := SchemaWorker([]string{schemaWorkerArgument}, strings.NewReader(test.request), &out)
			var reply schemaReply
			if !handled || exit != 0 || json.Unmarshal(out.Bytes(), &reply) != nil || reply.OK != (test.code == "") {
				t.Fatalf("worker did not return a bounded structured result: %q/%v/%d", out.String(), handled, exit)
			}
			if test.code != "" && (reply.Problem == nil || reply.Problem.Code != test.code) {
				t.Fatalf("expected %s, got %s", test.code, out.String())
			}
			if out.Len() > schemaReplyLimit || bytes.Contains(out.Bytes(), []byte("SECRET")) {
				t.Fatal("worker emitted unbounded or sensitive diagnostics")
			}
		})
	}
	for _, args := range [][]string{nil, {"run"}, {schemaWorkerArgument, "extra"}} {
		var output bytes.Buffer
		if handled, _ := SchemaWorker(args, strings.NewReader(`{"schema":true}`), &output); handled || output.Len() != 0 {
			t.Fatalf("worker consumed ordinary CLI arguments: %v", args)
		}
	}
}

func TestSchemaWorkerTransportFitsTwoBoundedRawDocuments(t *testing.T) {
	schema := []byte(`{"type":"string","title":"` + strings.Repeat("<", 1<<20) + `"}`)
	value := []byte(`"` + strings.Repeat("&", 1<<20) + `"`)
	// encoding/json's default escaping would make each field much larger;
	// transport uses raw canonical documents with HTML escaping disabled.
	var err error
	schema, err = Canonical(schema)
	if err != nil {
		t.Fatal(err)
	}
	value, err = Canonical(value)
	if err != nil {
		t.Fatal(err)
	}
	var input bytes.Buffer
	encoder := json.NewEncoder(&input)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(schemaRequest{Schema: schema, Value: value}); err != nil {
		t.Fatal(err)
	}
	if input.Len() <= MaxDocumentBytes || input.Len() > schemaRequestLimit {
		t.Fatalf("unexpected transport size %d", input.Len())
	}
	decoded, err := readSchemaRequest(&input)
	if err != nil || !bytes.Equal(decoded.Schema, schema) || !bytes.Equal(decoded.Value, value) {
		t.Fatalf("transport changed or rejected its two bounded documents: %v", err)
	}
}

func TestSchemaWorkerOutputAllowanceCannotBeBypassedByCopy(t *testing.T) {
	var output schemaOutput
	n, err := io.Copy(&output, strings.NewReader(strings.Repeat("x", schemaReplyLimit+1)))
	if err == nil || n > schemaReplyLimit || output.buffer.Len() > schemaReplyLimit {
		t.Fatal("writer fast path bypassed helper response allowance")
	}
}

func TestAuthorSchemaHardDeadlineKillsRealComputation(t *testing.T) {
	// This is a valid, small schema. Its shared-reference allOf graph expands
	// exponentially during successful validation; it is not a sleeping fake.
	defs := map[string]any{"n0": map[string]any{"type": "integer"}}
	for i := 1; i <= 36; i++ {
		ref := map[string]any{"$ref": fmt.Sprintf("#/$defs/n%d", i-1)}
		defs[fmt.Sprintf("n%d", i)] = map[string]any{"allOf": []any{ref, ref}}
	}
	schema, err := Canonical(encoded(t, map[string]any{"$defs": defs, "$ref": "#/$defs/n36"}))
	if err != nil {
		t.Fatal(err)
	}
	if err := checkSchema(schema, nil); err != nil {
		t.Fatalf("pathological validation fixture must first compile: %v", err)
	}
	started := time.Now()
	err = checkSchema(schema, []byte(`7`))
	elapsed := time.Since(started)
	expectProblem(t, err, "schema_timeout")
	if elapsed < schemaCheckTimeout-100*time.Millisecond || elapsed > schemaCheckTimeout+3*time.Second {
		t.Fatalf("real computation did not obey the owned process deadline: %s", elapsed)
	}
	// Run() has reaped its owned child before returning, releasing the permit;
	// a fresh real schema computation must remain available after the timeout.
	if len(schemaWorkers) != 0 {
		t.Fatal("timed out worker retained its concurrency permit")
	}
	if err := checkSchema([]byte(`{"type":"integer"}`), []byte(`8`)); err != nil {
		t.Fatalf("schema computation unavailable after timeout: %v", err)
	}
}

func TestSchemaCompileCacheDoesNotCacheValidationResults(t *testing.T) {
	schema := []byte(`{"type":"integer","maximum":1}`)
	if err := checkSchema(schema, nil); err != nil {
		t.Fatal(err)
	}
	if err := checkSchema(schema, []byte(`1`)); err != nil {
		t.Fatal(err)
	}
	expectProblem(t, checkSchema(schema, []byte(`2`)), "schema_invalid")
	// A byte change invalidates the compile identity. It cannot reuse the
	// previous successful schema merely because an external Ref ID is equal.
	expectProblem(t, checkSchema([]byte(`{"type":"not_a_type"}`), nil), "invalid_schema")
}
