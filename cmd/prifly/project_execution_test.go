package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

func TestProjectExecutionSourceAndBuild(t *testing.T) {
	folder := t.TempDir()
	file := filepath.Join(folder, "worker.js")
	if err := os.WriteFile(file, []byte("first worker bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"id":"test:step/convert","version":"1.0.0"}`)
	digest, _ := flow.Digest(data)
	ref := flow.Ref{ID: "test:step/convert", Version: "1.0.0", Digest: digest}
	components := []projectCompileComponent{{Kind: "step", Ref: ref, Bytes: data}}
	config := map[string]any{"executable": "node", "args": []any{"worker.js"}, "files": map[string]any{"worker.js": "worker.js"}, "timeout_ms": 1000, "grace_ms": 100, "max_output_bytes": 4096}
	source := projectPackageSource{Folder: folder, ID: "test:package/convert", Version: "1.0.0", Description: "Convert", RequiresCoreProtocol: "1", RootValue: map[string]any{"execution_bindings": map[string]any{"steps": map[string]any{ref.ID: config}}}}
	build := func() (projectPackageSource, []projectCompileComponent, string) {
		t.Helper()
		payload, err := projectReadExecution(folder, source, components, map[string]any{})
		if err != nil {
			t.Fatal(err)
		}
		selected := source
		selected.ExecutionBindings = payload
		compiled, provenance, err := projectBuildVariant(selected, components, "", map[string]any{}, projectWorkflowOptions{})
		if err != nil {
			t.Fatal(err)
		}
		return selected, compiled, provenance.BuildKey
	}
	a, compiledA, keyA := build()
	_, _, same := build()
	if same != keyA {
		t.Fatal("same binding bytes changed b1")
	}
	if err := os.WriteFile(file, []byte("second worker bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	b, _, keyB := build()
	if keyA == keyB {
		t.Fatal("supporting bytes did not change b1")
	}
	config["args"] = []any{"worker.js", "changed"}
	_, _, keyC := build()
	if keyC == keyB {
		t.Fatal("argv did not change b1")
	}
	if string(a.ExecutionBindings.Bindings[0].Files["worker.js"]) != "first worker bytes" {
		t.Fatal("source edit changed captured binding")
	}
	if err := projectSealExecution(a.ExecutionBindings, compiledA); err != nil {
		t.Fatal(err)
	}
	if a.ExecutionBindings.Bindings[0].DefinitionRef != compiledA[0].Ref {
		t.Fatal("binding did not select exact compiled ref")
	}
	encoded, err := projectCanonicalJSON(a.ExecutionBindings)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(folder)) {
		t.Fatal("machine path leaked into sealed package")
	}
	if _, err := projectDecodeExecution(encoded); err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(encoded, &object); err != nil {
		t.Fatal(err)
	}
	object["unknown"] = true
	bad, _ := json.Marshal(object)
	if _, err := projectDecodeExecution(bad); err == nil {
		t.Fatal("unknown sealed metadata accepted")
	}
	if bytes.Equal(a.ExecutionBindings.Bindings[0].Files["worker.js"], b.ExecutionBindings.Bindings[0].Files["worker.js"]) {
		t.Fatal("independent snapshots unexpectedly equal")
	}
	for name, mutate := range map[string]func(){
		"unknown field":       func() { config["shell"] = true },
		"null optional":       func() { config["args"] = nil },
		"absolute executable": func() { config["executable"] = "/bin/sh" },
		"missing limit":       func() { delete(config, "timeout_ms") },
		"unsafe target":       func() { config["files"] = map[string]any{"../worker.js": "worker.js"} },
		"unsafe source":       func() { config["files"] = map[string]any{"worker.js": "../outside.js"} },
		"symlink source": func() {
			if err := os.Symlink(file, filepath.Join(folder, "link.js")); err != nil {
				t.Fatal(err)
			}
			config["files"] = map[string]any{"worker.js": "link.js"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			saved, _ := json.Marshal(config)
			mutate()
			if _, err := projectReadExecution(folder, source, components, map[string]any{}); err == nil {
				t.Fatal("unsafe execution binding accepted")
			}
			for key := range config {
				delete(config, key)
			}
			if err := json.Unmarshal(saved, &config); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestProjectNeutralInputsDeferRequiredness(t *testing.T) {
	launch := projectLaunchDetail{Inputs: []projectLaunchInput{{Name: "configured", Required: true}}}
	if err := projectStartInputs(launch, nil, nil, false); err != nil {
		t.Fatal("neutral listing overrode compiled configuration defaults", err)
	}
	if err := projectStartInputs(launch, nil, nil, true); err == nil {
		t.Fatal("legacy explicit input requirement changed")
	}
	if err := projectStartInputs(launch, bindings{"unknown": "file"}, nil, false); err == nil {
		t.Fatal("unknown neutral input accepted")
	}
	if err := projectStartInputs(launch, bindings{"configured": "file"}, bindings{"configured": "ref"}, false); err == nil {
		t.Fatal("duplicate neutral input accepted")
	}
}
