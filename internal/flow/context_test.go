package flow

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
)

func contextRef(id string, data []byte) Ref {
	return Ref{ID: id, Version: "1.0.0", Digest: fmt.Sprintf("sha256:%x", sha256.Sum256(data))}
}

func contextWorkflow(t *testing.T, instructions *Ref, refs []Ref) (WorkflowRevision, Registry) {
	t.Helper()
	w, registry := callWorkflow(t, "test:workflow/context")
	ref := callStep(t, registry, map[string]InputPort{})
	var step StepDefinition
	if err := json.Unmarshal(registry[ref], &step); err != nil {
		t.Fatal(err)
	}
	step.ID, step.InstructionsRef = "test:step/context", instructions
	step.ContextRefs = append([]Ref{}, refs...)
	data, err := Canonical(encoded(t, step))
	if err != nil {
		t.Fatal(err)
	}
	ref = contextRef(step.ID, data)
	registry[ref] = data
	w.Definition.Entry = "work"
	w.Definition.Stages["work"] = Stage{Kind: "step", StepRef: ref, InputBindings: map[string]Binding{}, On: map[string]string{"pass": "done"}}
	return w, registry
}

func TestCoreContextExactResourcesAreDataAndDetached(t *testing.T) {
	// Ref-shaped JSON (including id/version) is instance data in this leaf.
	jsonData := []byte(" {\"version\":\"9.0.0\",\"id\":\"not:a-definition\",\"digest\":\"sha256:" + strings.Repeat("a", 64) + "\"} \n")
	canonicalJSON, err := Canonical(jsonData)
	if err != nil {
		t.Fatal(err)
	}
	textData := []byte("\ufeff# Rules\r\nНе удалять пробелы. e\u0301 / é / 🛠\r\n${HOME} $(touch nope) {{exec}}\r\n ")
	textRef, jsonRef := contextRef("test:context/instructions", textData), contextRef("test:context/data", canonicalJSON)
	w, registry := contextWorkflow(t, &textRef, []Ref{jsonRef, textRef})
	resources := ContextResources{
		textRef: {ByteEncoding: "utf8_text", MediaType: "text/markdown; charset=utf-8", Bytes: textData},
		jsonRef: {ByteEncoding: "json", MediaType: "application/json", Bytes: jsonData},
		// An unrelated source is neither loaded nor pinned.
		contextRef("test:context/unused", nil): {ByteEncoding: "unknown", Bytes: []byte{0xff}},
	}
	registry[jsonRef] = append([]byte("\n"), canonicalJSON...)
	p, err := CompileCore(encoded(t, w), "json", registry, resources)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Resources) != 2 || !bytes.Equal(p.Resources[textRef].Bytes, textData) || !bytes.Equal(p.Resources[jsonRef].Bytes, canonicalJSON) {
		t.Fatal("typed resource bytes or selected closure changed")
	}
	if p.Steps["work"].InstructionsRef == nil || *p.Steps["work"].InstructionsRef != textRef || !slices.Equal(p.Steps["work"].ContextRefs, []Ref{jsonRef, textRef}) {
		t.Fatal("instructions/reference roles or declared context order were collapsed")
	}
	for _, ref := range []Ref{textRef, jsonRef} {
		if _, exists := p.Registry[ref]; exists {
			t.Fatal("context leaf was added to executable JSON definitions")
		}
		_, err := p.schema(ref)
		expectProblem(t, err, "resource_type_mismatch")
	}
	wantText := bytes.Clone(textData)
	textData[0], jsonData[1] = 'X', 'X'
	resources[textRef] = ContextResource{}
	delete(resources, jsonRef)
	registry[jsonRef][0] = 'X'
	if !bytes.Equal(p.Resources[textRef].Bytes, wantText) || !bytes.Equal(p.Resources[jsonRef].Bytes, canonicalJSON) || p.compilation.availableResources != nil {
		t.Fatal("returned plan retained mutable input resources")
	}
}

func TestCoreContextRawUTF8Boundaries(t *testing.T) {
	for _, data := range [][]byte{nil, []byte("true"), []byte("\r\n"), []byte("\ufeff"), []byte{0}, bytes.Repeat([]byte("x"), MaxDocumentBytes)} {
		t.Run(fmt.Sprintf("bytes_%d_%x", len(data), sha256.Sum256(data))[:30], func(t *testing.T) {
			ref := contextRef("test:context/text", data)
			w, registry := contextWorkflow(t, &ref, nil)
			p, err := CompileCore(encoded(t, w), "json", registry, ContextResources{ref: {ByteEncoding: "utf8_text", MediaType: "text/plain", Bytes: data}})
			if err != nil || !bytes.Equal(p.Resources[ref].Bytes, data) {
				t.Fatalf("raw bytes did not survive: %v", err)
			}
		})
	}
}

func TestCoreContextRejectsInvalidOrUntypedResources(t *testing.T) {
	for _, tc := range []struct {
		name, encoding, media string
		data                  []byte
		code                  string
	}{
		{"unknown_encoding", "yaml", "text/plain", []byte("hello"), "invalid_context_resource"},
		{"missing_encoding", "", "text/plain", []byte("hello"), "invalid_context_resource"},
		{"invalid_utf8", "utf8_text", "text/plain", []byte{0xff}, "invalid_unicode"},
		{"too_large", "utf8_text", "text/plain", bytes.Repeat([]byte("x"), MaxDocumentBytes+1), "document_too_large"},
		{"missing_media", "utf8_text", "", []byte("hello"), "invalid_context_resource"},
		{"wildcard_media", "utf8_text", "text/*", []byte("hello"), "invalid_context_resource"},
		{"media_control", "utf8_text", "text/plain\r\nX-Secret: canary", []byte("hello"), "invalid_context_resource"},
		{"wrong_charset", "utf8_text", "text/plain; charset=iso-8859-1", []byte("hello"), "invalid_context_resource"},
		{"raw_json_media", "utf8_text", "application/json", []byte("true"), "invalid_context_resource"},
		{"json_text_media", "json", "text/plain", []byte("true"), "invalid_context_resource"},
		{"json_vendor_media", "json", "application/example+json", []byte("true"), "invalid_context_resource"},
		{"json_charset", "json", "application/json; charset=utf-8", []byte("true"), "invalid_context_resource"},
		{"oversized_media", "utf8_text", "text/" + strings.Repeat("x", 124), []byte("hello"), "invalid_context_resource"},
		{"invalid_json", "json", "application/json", []byte("CANARY-CONTENT"), "invalid_context_resource"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ref := contextRef("test:context/data", tc.data)
			w, registry := contextWorkflow(t, nil, []Ref{ref})
			_, err := CompileCore(encoded(t, w), "json", registry, ContextResources{ref: {ByteEncoding: tc.encoding, MediaType: tc.media, Bytes: tc.data}})
			p := expectProblem(t, err, tc.code)
			if strings.Contains(p.Error(), "CANARY") || strings.Contains(p.Error(), "X-Secret") || !strings.HasSuffix(p.Path, "/context_refs/0") {
				t.Fatal("resource error leaked content or lost its declaring position", p)
			}
		})
	}
	for _, position := range []string{"instructions", "context"} {
		for _, mode := range []string{"missing", "json_only", "digest", "raw_in_json", "json_conflict"} {
			t.Run(position+"/"+mode, func(t *testing.T) {
				data := []byte(`true`)
				ref := contextRef("test:context/data", data)
				var instructions *Ref
				var refs []Ref
				if position == "instructions" {
					instructions = &ref
				} else {
					refs = []Ref{ref}
				}
				w, registry := contextWorkflow(t, instructions, refs)
				resources := ContextResources{ref: {ByteEncoding: "utf8_text", MediaType: "text/plain", Bytes: data}}
				code := "resource_type_mismatch"
				switch mode {
				case "missing":
					resources = nil
					code = "missing_ref"
				case "json_only":
					resources = nil
					registry[ref] = data
				case "digest":
					resources[ref] = ContextResource{ByteEncoding: "utf8_text", MediaType: "text/plain", Bytes: []byte("changed")}
					code = "digest_mismatch"
				case "raw_in_json":
					registry[ref] = data
				case "json_conflict":
					resources[ref] = ContextResource{ByteEncoding: "json", MediaType: "application/json", Bytes: data}
					registry[ref] = []byte(`false`)
				}
				_, err := CompileCore(encoded(t, w), "json", registry, resources)
				expectProblem(t, err, code)
			})
		}
	}
}

func TestCoreContextCannotBecomeDefinition(t *testing.T) {
	for _, encoding := range []string{"utf8_text", "json"} {
		for _, position := range []string{"schema", "step", "child_workflow", "root_workflow", "policy"} {
			t.Run(encoding+"/"+position, func(t *testing.T) {
				w, registry := contextWorkflow(t, nil, nil)
				var ref Ref
				switch position {
				case "schema":
					ref = contextRef("test:schema/data", []byte(`true`))
					registry[ref] = []byte(`true`)
					w.Inputs["data"] = InputPort{Port: Port{Format: "json", SchemaRef: &ref}}
				case "step":
					ref = w.Definition.Stages["work"].StepRef
				case "child_workflow":
					child, _ := callWorkflow(t, "test:workflow/child")
					ref = registerCallWorkflow(t, registry, child)
					w.Definition.Stages["work"] = callStage(ref, "done")
				case "root_workflow":
					data, err := Canonical(encoded(t, w))
					if err != nil {
						t.Fatal(err)
					}
					ref = contextRef(w.ID, data)
					registry[ref] = data
				case "policy":
					ref = w.PolicyRef
				}
				data, err := Canonical(registry[ref])
				if err != nil {
					t.Fatal(err)
				}
				media := "application/json"
				if encoding == "utf8_text" {
					media = "text/plain"
				}
				_, err = CompileCore(encoded(t, w), "json", registry, ContextResources{ref: {ByteEncoding: encoding, MediaType: media, Bytes: data}})
				expectProblem(t, err, "resource_type_mismatch")
			})
		}
	}
}

func TestCoreContextIdentityConflictsAcrossTypedClosure(t *testing.T) {
	first := contextRef("test:context/same", []byte("one"))
	second := contextRef(first.ID, []byte("two"))
	w, registry := contextWorkflow(t, &first, []Ref{second})
	resources := ContextResources{
		first:  {ByteEncoding: "utf8_text", MediaType: "text/plain", Bytes: []byte("one")},
		second: {ByteEncoding: "utf8_text", MediaType: "text/plain", Bytes: []byte("two")},
	}
	_, err := CompileCore(encoded(t, w), "json", registry, resources)
	expectProblem(t, err, "ref_identity_conflict")
	// A JSON definition cannot take the same identity/version with other bytes.
	data := []byte(`true`)
	schema := contextRef(first.ID, data)
	w, registry = contextWorkflow(t, &first, nil)
	registry[schema] = data
	w.Inputs["data"] = InputPort{Port: Port{Format: "json", SchemaRef: &schema}}
	_, err = CompileCore(encoded(t, w), "json", registry, ContextResources{first: resources[first]})
	expectProblem(t, err, "ref_identity_conflict")
}

func TestCoreContextSharedCallRepeatClosure(t *testing.T) {
	data := []byte("# Shared bytes\r\n")
	ref := contextRef("test:context/nested", data)
	leaf, registry := contextWorkflow(t, &ref, []Ref{ref})
	body, _ := callWorkflow(t, "test:workflow/repeating-context")
	body.Definition.Entry = "repeat"
	body.Definition.Stages["repeat"] = repeatStage(registerCallWorkflow(t, registry, leaf), "done", 2)
	root, _ := callWorkflow(t, "test:workflow/context-caller")
	childRef := registerCallWorkflow(t, registry, body)
	root.Definition.Entry = "first"
	root.Definition.Stages["first"] = callStage(childRef, "second")
	root.Definition.Stages["second"] = callStage(childRef, "done")
	p, err := CompileCore(encoded(t, root), "json", registry, ContextResources{ref: {ByteEncoding: "utf8_text", MediaType: "text/markdown", Bytes: data}})
	if err != nil {
		t.Fatal(err)
	}
	if p.Calls["first"] != p.Calls["second"] || len(p.Resources) != 1 || len(p.Calls["first"].Repeats["repeat"].Resources) != 1 {
		t.Fatal("nested definitions lost their resource closure or duplicated shared plans")
	}
	wantBytes := len(data)
	for _, definition := range p.Registry {
		wantBytes += len(definition)
	}
	if p.compilation.dependencyBytes != wantBytes {
		t.Fatal("nested context bytes were double charged or reset", p.compilation.dependencyBytes, wantBytes)
	}
	data[0] = '!'
	if p.Calls["first"].Repeats["repeat"].Resources[ref].Bytes[0] != '#' {
		t.Fatal("nested plan still reads mutable context bytes")
	}
}

func TestCoreContextSchemaFirstReferenceUses(t *testing.T) {
	for _, nested := range []bool{false, true} {
		for _, supplied := range []bool{false, true} {
			t.Run(fmt.Sprintf("nested_%t/supplied_%t", nested, supplied), func(t *testing.T) {
				data := []byte("# Required instructions\r\n")
				ref := contextRef("test:context/schema-first", data)
				leaf, registry := contextWorkflow(t, &ref, []Ref{ref})
				stepRef := leaf.Definition.Stages["work"].StepRef
				// A StepDefinition is also a valid permissive JSON Schema. Its
				// first use as a schema must not suppress later Step dependencies.
				reader := callStep(t, registry, map[string]InputPort{
					"schema": {Port: Port{Format: "json", SchemaRef: &stepRef}},
				})
				root := leaf
				next := "work"
				if nested {
					root, _ = callWorkflow(t, "test:workflow/schema-first-parent")
					next = "first"
					leaf.ID = "test:workflow/schema-first-one"
					root.Definition.Stages["first"] = callStage(registerCallWorkflow(t, registry, leaf), "second")
					leaf.ID = "test:workflow/schema-first-two"
					root.Definition.Stages["second"] = callStage(registerCallWorkflow(t, registry, leaf), "done")
				}
				root.Definition.Entry = "a"
				root.Definition.Stages["a"] = Stage{Kind: "step", StepRef: reader, InputBindings: map[string]Binding{}, On: map[string]string{"pass": next}}
				resources := ContextResources{}
				if supplied {
					resources[ref] = ContextResource{ByteEncoding: "utf8_text", MediaType: "text/markdown", Bytes: data}
				}
				p, err := CompileCore(encoded(t, root), "json", registry, resources)
				if !supplied {
					expectProblem(t, err, "missing_ref")
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if len(p.Resources) != 1 || !bytes.Equal(p.Resources[ref].Bytes, data) {
					t.Fatal("schema cache suppressed the Step's required context")
				}
				if nested {
					if p.Calls["first"] == p.Calls["second"] {
						t.Fatal("distinct child definitions were merged")
					}
					for _, id := range []string{"first", "second"} {
						child := p.Calls[id]
						if child.Workflow.Definition.Stages["work"].StepRef != stepRef || len(child.Resources) != 1 || !bytes.Equal(child.Resources[ref].Bytes, data) {
							t.Fatal("shared Step lost its context in a child definition", id)
						}
					}
				}
				wantBytes := len(data)
				for _, definition := range p.Registry {
					wantBytes += len(definition)
				}
				if p.compilation.dependencyBytes != wantBytes {
					t.Fatal("each reference role charged the same bytes again", p.compilation.dependencyBytes, wantBytes)
				}
			})
		}
	}
}

func TestCoreContextSchemaFirstCheckDependency(t *testing.T) {
	for _, supplied := range []bool{false, true} {
		t.Run(fmt.Sprintf("adapter_supplied_%t", supplied), func(t *testing.T) {
			w, registry := contextWorkflow(t, nil, nil)
			adapterData := []byte(`{"operation":"check"}`)
			adapterRef := contextRef("test:adapter/schema-first", adapterData)
			check := checkDefinitionFixture("result", "check_passed")
			check.ID, check.Executor.AdapterRef = "test:check/schema-first", adapterRef
			checkRef := checkComponent(t, registry, check.ID, check.Version, check)
			changeCheckedStep(t, &w, registry, func(step *StepDefinition) {
				step.ResultCheckRefs = []Ref{checkRef}
			})
			reader := callStep(t, registry, map[string]InputPort{
				"schema": {Port: Port{Format: "json", SchemaRef: &checkRef}},
			})
			w.Definition.Entry = "a"
			w.Definition.Stages["a"] = Stage{Kind: "step", StepRef: reader, InputBindings: map[string]Binding{}, On: map[string]string{"pass": "work"}}
			if supplied {
				registry[adapterRef] = adapterData
			}
			p, err := CompileCore(encoded(t, w), "json", registry, nil)
			if !supplied {
				expectProblem(t, err, "missing_ref")
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if p.Checks[checkRef] != check || !bytes.Equal(p.Registry[adapterRef], adapterData) {
				t.Fatal("schema cache suppressed the check's exact executor dependency")
			}
		})
	}
}

func TestCoreContextLegacyEntrypointsStayJSONOnly(t *testing.T) {
	data := []byte(`{"text":"legacy instructions"}`)
	ref := contextRef("test:context/legacy", data)
	w, registry := contextWorkflow(t, &ref, nil)
	w.Limits.MaxChildDepth = 0
	registry[ref] = data
	for _, profile := range []string{Profile, CoreProfile} {
		p, err := CompileProfile(encoded(t, w), "json", registry, profile)
		if err != nil || p.Resources != nil || !bytes.Equal(p.Registry[ref], data) {
			t.Fatalf("legacy %s instruction pin changed: %v", profile, err)
		}
	}
	w, registry = contextWorkflow(t, nil, []Ref{ref})
	w.Limits.MaxChildDepth = 0
	registry[ref] = data
	for _, profile := range []string{Profile, CoreProfile} {
		_, err := CompileProfile(encoded(t, w), "json", registry, profile)
		expectProblem(t, err, "unsupported")
	}
	if _, err := CompileCore(encoded(t, w), "json", registry, ContextResources{ref: {ByteEncoding: "json", MediaType: "application/json", Bytes: data}}); err != nil {
		t.Fatal("explicit Core entrypoint did not enable typed context", err)
	}
}

func TestCoreContextResourcePositionsAreScoped(t *testing.T) {
	w, registry := contextWorkflow(t, nil, nil)
	schema := contextRef("test:schema/context-names", []byte(`true`))
	registry[schema] = []byte(`true`)
	stepRef := w.Definition.Stages["work"].StepRef
	var step StepDefinition
	if err := json.Unmarshal(registry[stepRef], &step); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"instructions_ref", "context_refs"} {
		step.Inputs[name] = InputPort{Port: Port{Format: "json", SchemaRef: &schema}}
	}
	data, err := Canonical(encoded(t, step))
	if err != nil {
		t.Fatal(err)
	}
	stepRef = contextRef(step.ID, data)
	registry[stepRef] = data
	stage := w.Definition.Stages["work"]
	stage.StepRef = stepRef
	w.Definition.Stages["work"] = stage
	p, err := CompileCore(encoded(t, w), "json", registry, nil)
	if err != nil || len(p.Resources) != 0 || !bytes.Equal(p.Registry[schema], []byte(`true`)) {
		t.Fatal("an input port name changed schema references into context leaves", err)
	}
}

func TestCoreContextCompleteClosureCountLimit(t *testing.T) {
	build := func(resourceCount int) (WorkflowRevision, Registry, ContextResources) {
		w, registry := callWorkflow(t, "test:workflow/context-count")
		stepRef := callStep(t, registry, map[string]InputPort{})
		var step StepDefinition
		if err := json.Unmarshal(registry[stepRef], &step); err != nil {
			t.Fatal(err)
		}
		resources := ContextResources{}
		for i := range 9 {
			step.ID = fmt.Sprintf("test:step/context-count-%d", i)
			step.ContextRefs = []Ref{}
			for j := i * 128; j < min((i+1)*128, resourceCount); j++ {
				data := []byte(fmt.Sprintf("context %d", j))
				ref := contextRef(fmt.Sprintf("test:context/count-%d", j), data)
				resources[ref] = ContextResource{ByteEncoding: "utf8_text", MediaType: "text/plain", Bytes: data}
				step.ContextRefs = append(step.ContextRefs, ref)
			}
			data, err := Canonical(encoded(t, step))
			if err != nil {
				t.Fatal(err)
			}
			stepRef = contextRef(step.ID, data)
			registry[stepRef] = data
			next := "done"
			if i < 8 {
				next = fmt.Sprintf("work_%d", i+1)
			}
			w.Definition.Stages[fmt.Sprintf("work_%d", i)] = Stage{Kind: "step", StepRef: stepRef, InputBindings: map[string]Binding{}, On: map[string]string{"pass": next}}
		}
		w.Definition.Entry = "work_0"
		return w, registry, resources
	}
	w, registry, resources := build(0)
	base, err := CompileCore(encoded(t, w), "json", registry, resources)
	if err != nil {
		t.Fatal(err)
	}
	for _, extra := range []int{0, 1} {
		w, registry, resources = build(1024 - len(base.Registry) + extra)
		p, err := CompileCore(encoded(t, w), "json", registry, resources)
		if extra != 0 {
			expectProblem(t, err, "dependency_limit")
		} else if err != nil || len(p.Registry)+len(p.Resources) != 1024 {
			t.Fatalf("complete JSON/resource closure rejected at exact limit: %v", err)
		}
	}
}

func TestCoreContextSharedDependencyLimits(t *testing.T) {
	// Test exact accounting boundaries without allocating a 64 MiB fixture.
	// The nested integration above verifies the same counter is shared by real
	// call/repeat compilations rather than reset for each child or use site.
	data := []byte("bytes")
	ref := contextRef("test:context/budget", data)
	for _, tc := range []struct {
		name        string
		definitions int
		usedBytes   int
		code        string
	}{
		{"count_exact", 1023, 0, ""},
		{"count_over", 1024, 0, "dependency_limit"},
		{"bytes_exact", 0, (64 << 20) - len(data), ""},
		{"bytes_over", 0, (64 << 20) - len(data) + 1, "dependency_limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			shared := newCompilation()
			shared.resources = ContextResources{}
			shared.availableResources = ContextResources{ref: {ByteEncoding: "utf8_text", MediaType: "text/plain", Bytes: data}}
			shared.dependencyBytes = tc.usedBytes
			p := &Plan{Profile: CoreProfile, Registry: shared.pinned, Resources: shared.resources, compilation: shared}
			for i := range tc.definitions {
				p.Registry[contextRef(fmt.Sprintf("test:definition/%d", i), nil)] = nil
			}
			err := p.pinContextResource(ref, nil, "/context_refs/0", map[Ref]bool{})
			if tc.code != "" {
				expectProblem(t, err, tc.code)
				if shared.dependencyBytes != tc.usedBytes || len(p.Resources) != 0 {
					t.Fatal("refused resource consumed a budget or pin")
				}
				return
			}
			if err != nil || shared.dependencyBytes != tc.usedBytes+len(data) {
				t.Fatalf("exact budget refused: %v", err)
			}
			if err := p.pinContextResource(ref, nil, "/instructions_ref", map[Ref]bool{}); err != nil || shared.dependencyBytes != tc.usedBytes+len(data) {
				t.Fatal("duplicate use charged dependency bytes again", err)
			}
		})
	}
}
