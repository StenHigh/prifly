package flow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stenhigh/prifly/internal/purity"
)

const schemaWorkerArgument = "--prifly-schema-worker-v1"
const schemaCheckTimeout = 2 * time.Second
const schemaRequestLimit = 2*MaxDocumentBytes + 1024
const schemaReplyLimit = 8 << 10

type schemaRequest struct {
	Schema json.RawMessage `json:"schema"`
	Value  json.RawMessage `json:"value,omitempty"`
}

type schemaReply struct {
	OK      bool     `json:"ok"`
	Problem *Problem `json:"problem,omitempty"`
}

// SchemaWorker handles the private, bounded schema-computation mode. CLI main
// and test TestMain must call it before parsing their ordinary flags. This
// branch reads only the supplied bytes: no authority, files, AI or URL loader.
// The parent owns the hard deadline and always waits for this child to exit.
func SchemaWorker(args []string, input io.Reader, output io.Writer) (bool, int) {
	if len(args) != 1 || args[0] != schemaWorkerArgument {
		return false, 0
	}
	request, err := readSchemaRequest(input)
	if err == nil {
		err = computeSchema(request)
	}
	reply := schemaReply{OK: err == nil}
	if err != nil {
		if !errors.As(err, &reply.Problem) {
			reply.Problem = &Problem{Code: "invalid_schema", Message: "schema computation failed"}
		}
	}
	if err := json.NewEncoder(output).Encode(reply); err != nil {
		return true, 1
	}
	return true, 0
}

func readSchemaRequest(input io.Reader) (schemaRequest, error) {
	var request schemaRequest
	data, err := io.ReadAll(io.LimitReader(input, schemaRequestLimit+1))
	if err != nil || len(data) > schemaRequestLimit {
		return request, problem("invalid_schema_request", "", "schema request exceeds its byte allowance")
	}
	// The enclosing transport may hold two 2 MiB JSON documents. Decode its
	// two closed fields here; each document still uses the normal strict parser.
	d := json.NewDecoder(bytes.NewReader(data))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return request, problem("invalid_schema_request", "", "expected a schema request object")
	}
	seen := map[string]bool{}
	for d.More() {
		token, err := d.Token()
		key, ok := token.(string)
		if err != nil || !ok || seen[key] || (key != "schema" && key != "value") {
			return request, problem("invalid_schema_request", "", "schema request has an unknown or repeated field")
		}
		seen[key] = true
		target := &request.Schema
		if key == "value" {
			target = &request.Value
		}
		if err := d.Decode(target); err != nil {
			return request, problem("invalid_schema_request", "", "invalid schema request JSON")
		}
	}
	if token, err := d.Token(); err != nil || token != json.Delim('}') || len(request.Schema) == 0 {
		return request, problem("invalid_schema_request", "", "schema document is required")
	}
	if _, err := d.Token(); err != io.EOF {
		return request, problem("invalid_schema_request", "", "expected one schema request")
	}
	return request, nil
}

func computeSchema(request schemaRequest) error {
	value, err := Parse(request.Schema, "json")
	if err != nil {
		return err
	}
	c := newSchemaCompiler()
	const resource = "urn:prifly:schema-worker"
	if err := c.AddResource(resource, value); err != nil {
		return problem("invalid_schema", "", "schema cannot be loaded from the pinned definition")
	}
	schema, err := c.Compile(resource)
	if err != nil {
		return problem("invalid_schema", "", "schema cannot be compiled without external resources")
	}
	if len(request.Value) == 0 {
		return nil
	}
	value, err = Parse(request.Value, "json")
	if err != nil {
		return err
	}
	return validationProblem(schema.Validate(value), "")
}

// Only successful compilation is cached, by the complete exact schema bytes.
// Actual instance checks always execute; a previously valid value grants no
// permission to accept another one. Eviction is deliberately a simple reset.
var schemaCompileCache = struct {
	sync.Mutex
	entries map[string]bool
	bytes   int
}{entries: map[string]bool{}}

var schemaWorkers = make(chan struct{}, 4)
var stepResultSchema struct {
	sync.Once
	bytes []byte
}

// The schema helper process existed because a schema could hang the validator.
// Two dangers were folded into that one answer. Patterns are no longer one of
// them: this build compiles with Go's regexp, which is linear. The other is
// real and stays: a shared reference under allOf/anyOf/oneOf is evaluated once
// per occurrence, so a small valid schema can expand exponentially while
// validating a single value, and nothing in this process could stop it.
//
// So a value is validated here when its schema's evaluation is bounded, and in
// the helper - which owns a hard deadline and can be killed - when it is not.
// PRIFLY_SCHEMA_WORKER=1 sends everything to the helper for one release.
const schemaWorkerVariable = "PRIFLY_SCHEMA_WORKER"

// maxInProcessEvaluations is the evaluation budget a schema must fit to be
// checked in this process. A pinned authored schema is far below it; the
// pathological expansions are astronomically above it, so the boundary needs no
// precision.
const maxInProcessEvaluations = 1 << 20

// schemaEvaluationBound counts how many subschema evaluations one value can
// cost, memoized per referenced definition so that computing the bound is
// itself linear. It saturates: the answer only has to say "over the budget".
func schemaEvaluationBound(document any) int64 {
	visiting := map[string]bool{}
	memo := map[string]int64{}
	var walk func(node any, path string) int64
	resolve := func(ref string) (any, bool) {
		if ref == "#" {
			return document, true
		}
		pointer, found := strings.CutPrefix(ref, "#")
		if !found {
			return nil, false
		}
		return JSONPointer(document, pointer)
	}
	walk = func(node any, path string) int64 {
		switch value := node.(type) {
		case map[string]any:
			if ref, ok := value["$ref"].(string); ok {
				if visiting[ref] {
					// A reference back to an enclosing definition is recursion:
					// its cost is paid per node of the value, which is already
					// bounded, not per copy of the schema. What is charged here
					// is duplication - the same definition reached along several
					// paths - because that is what multiplies.
					return 1
				}
				if cost, known := memo[ref]; known {
					return cost
				}
				target, found := resolve(ref)
				if !found {
					return 1
				}
				visiting[ref] = true
				cost := walk(target, ref)
				delete(visiting, ref)
				memo[ref] = cost
				return cost
			}
			total := int64(1)
			for key, child := range value {
				if key == "$defs" || key == "definitions" {
					continue // Reached through the references that use them.
				}
				total = saturatingAdd(total, walk(child, path+"/"+key))
			}
			return total
		case []any:
			total := int64(1)
			for _, child := range value {
				total = saturatingAdd(total, walk(child, path))
			}
			return total
		}
		return 1
	}
	return walk(document, "")
}

func saturatingAdd(a, b int64) int64 {
	if a >= maxInProcessEvaluations || b >= maxInProcessEvaluations || a+b >= maxInProcessEvaluations {
		return maxInProcessEvaluations
	}
	return a + b
}

// compiledSchemaCache keeps compiled schemas by their exact bytes. Only
// compilation is cached: every instance check still runs, because a value that
// once validated grants no permission to accept another. Eviction is a reset.
var compiledSchemaCache = struct {
	sync.Mutex
	entries map[string]compiledSchemaEntry
	bytes   int
}{entries: map[string]compiledSchemaEntry{}}

// bounded says the schema's evaluation fits the in-process budget, so a value
// may be checked here rather than in the helper.
type compiledSchemaEntry struct {
	schema  *jsonschema.Schema
	bounded bool
}

const maxCompiledSchemas = 64
const maxCompiledSchemaBytes = 8 << 20

func compiledSchema(schema []byte) (compiledSchemaEntry, error) {
	key := string(schema)
	compiledSchemaCache.Lock()
	cached, found := compiledSchemaCache.entries[key]
	compiledSchemaCache.Unlock()
	if found {
		return cached, nil
	}
	value, err := Parse(schema, "json")
	if err != nil {
		return compiledSchemaEntry{}, err
	}
	c := newSchemaCompiler()
	const resource = "urn:prifly:schema"
	if err := c.AddResource(resource, value); err != nil {
		return compiledSchemaEntry{}, problem("invalid_schema", "", "schema cannot be loaded from the pinned definition")
	}
	compiled, err := c.Compile(resource)
	if err != nil {
		return compiledSchemaEntry{}, problem("invalid_schema", "", "schema cannot be compiled without external resources")
	}
	entry := compiledSchemaEntry{schema: compiled, bounded: schemaEvaluationBound(value) < maxInProcessEvaluations}
	compiledSchemaCache.Lock()
	if len(compiledSchemaCache.entries) >= maxCompiledSchemas || compiledSchemaCache.bytes+len(schema) > maxCompiledSchemaBytes {
		compiledSchemaCache.entries, compiledSchemaCache.bytes = map[string]compiledSchemaEntry{}, 0
	}
	compiledSchemaCache.entries[key] = entry
	compiledSchemaCache.bytes += len(schema)
	compiledSchemaCache.Unlock()
	return entry, nil
}

// checkSchemaInProcess reports whether it answered. It declines a value whose
// schema can expand beyond the in-process budget, leaving that check to the
// helper process that can be stopped.
func checkSchemaInProcess(schema, value []byte) (error, bool) {
	if len(schema) == 0 || len(schema) > MaxDocumentBytes || len(value) > MaxDocumentBytes {
		return problem("document_too_large", "", "schema/value exceeds the document byte allowance"), true
	}
	entry, err := compiledSchema(schema)
	if err != nil {
		return err, true
	}
	if value == nil {
		return nil, true
	}
	if !entry.bounded {
		return nil, false
	}
	parsed, err := Parse(value, "json")
	if err != nil {
		return err, true
	}
	return validationProblem(entry.schema.Validate(parsed), ""), true
}

func checkSchema(schema, value []byte) error {
	stepResultSchema.Do(func() { stepResultSchema.bytes, _ = ProtocolSchema("StepResult") })
	if len(stepResultSchema.bytes) != 0 && bytes.Equal(schema, stepResultSchema.bytes) {
		// Trust only byte-exact embedded content, never an author-chosen core ID.
		if value == nil {
			return nil
		}
		return ValidateProtocol("StepResult", value)
	}
	if os.Getenv(schemaWorkerVariable) != "1" {
		if err, answered := checkSchemaInProcess(schema, value); answered {
			return err
		}
	}
	if value == nil {
		schemaCompileCache.Lock()
		cached := schemaCompileCache.entries[string(schema)]
		schemaCompileCache.Unlock()
		if cached {
			return nil
		}
	}
	err := runSchemaWorker(schema, value)
	if err == nil && value == nil {
		schemaCompileCache.Lock()
		if !schemaCompileCache.entries[string(schema)] {
			if len(schemaCompileCache.entries) >= 64 || schemaCompileCache.bytes+len(schema) > 8<<20 {
				schemaCompileCache.entries, schemaCompileCache.bytes = map[string]bool{}, 0
			}
			schemaCompileCache.entries[string(schema)] = true
			schemaCompileCache.bytes += len(schema)
		}
		schemaCompileCache.Unlock()
	}
	return err
}

type schemaOutput struct{ buffer bytes.Buffer }

func (w *schemaOutput) Write(p []byte) (int, error) {
	if w.buffer.Len()+len(p) > schemaReplyLimit {
		return 0, errors.New("schema helper output exceeds its allowance")
	}
	return w.buffer.Write(p)
}

// schemaWorkerRuns counts helper launches. A validation that reaches the helper
// costs a process; the count is what a test reads to prove it did not.
var schemaWorkerRuns atomic.Int64

func runSchemaWorker(schema, value []byte) error {
	purity.Guard("schema.worker")
	schemaWorkerRuns.Add(1)
	if len(schema) == 0 || len(schema) > MaxDocumentBytes || len(value) > MaxDocumentBytes {
		return problem("document_too_large", "", "schema/value exceeds the document byte allowance")
	}
	// RawMessage avoids base64 growth. Disabling HTML escaping preserves the
	// transport byte bound for otherwise valid strings containing '<' or '&'.
	var input bytes.Buffer
	encoder := json.NewEncoder(&input)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(schemaRequest{Schema: schema, Value: value}); err != nil || input.Len() > schemaRequestLimit {
		return problem("invalid_schema_request", "", "schema request cannot be encoded within its allowance")
	}
	ctx, cancel := context.WithTimeout(context.Background(), schemaCheckTimeout)
	defer cancel()
	select {
	case schemaWorkers <- struct{}{}:
		defer func() { <-schemaWorkers }()
	case <-ctx.Done():
		return problem("schema_timeout", "", "schema computation exceeded its hard deadline")
	}
	executable, err := os.Executable()
	if err != nil {
		return problem("schema_check_failed", "", "schema computation process is unavailable")
	}
	command := exec.CommandContext(ctx, executable, schemaWorkerArgument)
	command.Env = []string{"GORACE=atexit_sleep_ms=0", "GOMAXPROCS=1"}
	command.Stdin = &input
	var output schemaOutput
	command.Stdout, command.Stderr = &output, io.Discard
	err = command.Run() // Run always waits/reaps, including deadline termination.
	if ctx.Err() != nil {
		return problem("schema_timeout", "", "schema computation exceeded its hard deadline")
	}
	if err != nil {
		return problem("schema_check_failed", "", "schema computation process did not return a proven result")
	}
	var reply schemaReply
	d := json.NewDecoder(&output.buffer)
	d.DisallowUnknownFields()
	if err := d.Decode(&reply); err != nil || reply.OK == (reply.Problem != nil) {
		return problem("schema_check_failed", "", "schema computation returned an invalid response")
	}
	if _, err := d.Token(); err != io.EOF {
		return problem("schema_check_failed", "", "schema computation returned trailing data")
	}
	if reply.Problem != nil {
		return reply.Problem
	}
	return nil
}
