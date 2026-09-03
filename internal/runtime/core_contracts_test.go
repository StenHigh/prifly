package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

func TestFoundationPublicSchemaIsImmutable(t *testing.T) {
	// The released F1 schema is a compatibility fixture, not generated evidence
	// to update when a new field is added to a shared Go type.
	const released = "sha256:4fda82e5908602a8df274a23981884682c38fb08bc465d8cc9a1dd27af3d9c42"
	if got := rawDigest(publicContracts); got != released {
		t.Fatalf("released foundation-public:1 changed: %s", got)
	}
}

func TestCorePublicSchemaIsImmutable(t *testing.T) {
	const released = "sha256:573e440951b857afc6b22e4b77a4c0db08a1a252bd9e8007b5bde300cff06441"
	if got := rawDigest(corePublicContracts); got != released {
		t.Fatalf("released core-public:1 changed: %s", got)
	}
}

func TestChoiceDecisionPublicContract(t *testing.T) {
	distributed, err := os.ReadFile("../../schemas/core/choice-decision.schema.json")
	if err != nil || !bytes.Equal(distributed, choiceContracts) {
		t.Fatalf("choice embedded/distributed schemas differ: %v", err)
	}
	e, workflow, options := choiceFixture(t, `{"flag":true}`, "")
	runID := choiceStart(t, e, workflow, options)
	if err := e.Drive(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	_, events := choiceHistory(t, e, runID)
	if len(events) != 1 {
		t.Fatal("expected one committed decision")
	}
	var d ChoiceDecision
	if err := json.Unmarshal(events[0].Data, &d); err != nil {
		t.Fatal(err)
	}
	if err := validatePublic(t, "ChoiceDecision", d); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(map[string]any){
		"unknown_field":   func(v map[string]any) { v["override"] = true },
		"wrong_version":   func(v map[string]any) { v["schema_version"] = "choice-decision/2" },
		"wrong_route":     func(v map[string]any) { v["route"] = "guess" },
		"null_inputs":     func(v map[string]any) { v["inputs"] = nil },
		"empty_trace":     func(v map[string]any) { v["evaluations"] = []any{} },
		"missing_branch":  func(v map[string]any) { delete(v, "branch_id") },
		"missing_target":  func(v map[string]any) { delete(v, "next_stage_id") },
		"branch_failure":  func(v map[string]any) { v["failure"] = "condition_unknown" },
		"invalid_time":    func(v map[string]any) { v["observation"].(map[string]any)["utc"] = "yesterday" },
		"trace_extension": func(v map[string]any) { v["evaluations"].([]any)[0].(map[string]any)["why"] = "guessed" },
		"coerced_truth":   func(v map[string]any) { v["evaluations"].([]any)[0].(map[string]any)["result"] = true },
		"unqualified_ref": func(v map[string]any) { delete(v["inputs"].([]any)[0].(map[string]any), "source_ref") },
		"zero_revision": func(v map[string]any) {
			v["inputs"].([]any)[0].(map[string]any)["source_ref"].(map[string]any)["revision"] = 0
		},
		"unsupported_source": func(v map[string]any) {
			v["inputs"].([]any)[0].(map[string]any)["field_ref"].(map[string]any)["from"] = "iteration_output"
		},
		"invalid_pointer": func(v map[string]any) {
			v["inputs"].([]any)[0].(map[string]any)["field_ref"].(map[string]any)["pointer"] = "/bad~2"
		},
		"unknown_is_not_error_handler": func(v map[string]any) {
			v["route"], v["failure"] = "on_error", "condition_unknown"
			delete(v, "branch_id")
		},
		"no_transition_is_not_error_handler": func(v map[string]any) {
			v["route"], v["failure"] = "on_error", "no_transition"
			delete(v, "branch_id")
		},
	} {
		t.Run(name, func(t *testing.T) {
			var object map[string]any
			if err := json.Unmarshal(events[0].Data, &object); err != nil {
				t.Fatal(err)
			}
			mutate(object)
			if validatePublic(t, "ChoiceDecision", object) == nil {
				t.Fatal("invalid decision accepted")
			}
		})
	}
}

func TestChoiceSourceBudgetAndExactReferenceCache(t *testing.T) {
	const data = `{"flag":true,"label":"value"}`
	e, workflow, options := choiceFixture(t, data, "")
	inputs := workflow["inputs"].(map[string]any)
	inputs["alias"] = inputs["control"]
	schemaRef := inputs["control"].(map[string]any)["schema_ref"].(flow.Ref)
	artifact, err := e.ImportArtifact("control.json", "json", &schemaRef)
	if err != nil {
		t.Fatal(err)
	}
	options.Inputs = map[string]string{}
	options.InputRefs = map[string]ArtifactRef{"control": artifact.Ref(), "alias": artifact.Ref()}
	runID := choiceStart(t, e, workflow, options)
	r := driverRun(t, e, runID)
	p, err := r.plan()
	if err != nil {
		t.Fatal(err)
	}
	ref := flow.FieldRef{From: "workflow_input", Port: "control", Pointer: "/flag"}
	// Exercise the shared reader at its exact remaining-byte boundary without
	// manufacturing large artifacts or presenting this unit check as a load test.
	readBytes := int64(MaxArtifactBytes - len(data))
	cache := map[ArtifactRef]choiceSourceValue{}
	input, value, present, err := e.choiceSource(r, p, r.RootInvocationID, ref, &readBytes, cache)
	if err != nil || !present || input.SourceRef == nil || *input.SourceRef != artifact.Ref() || readBytes != MaxArtifactBytes || len(cache) != 1 {
		t.Fatalf("exact source budget boundary: %+v %v %d %v", input, present, readBytes, err)
	}
	if selected, ok := flow.JSONPointer(value, ref.Pointer); !ok || selected != true {
		t.Fatal("source reader changed the pinned data")
	}
	ref.Port, ref.Pointer = "alias", "/label"
	_, value, present, err = e.choiceSource(r, p, r.RootInvocationID, ref, &readBytes, cache)
	if err != nil || !present || readBytes != MaxArtifactBytes || len(cache) != 1 {
		t.Fatal("the same revision through another port/pointer was read or charged again", err)
	}
	if selected, ok := flow.JSONPointer(value, ref.Pointer); !ok || selected != "value" {
		t.Fatal("cache confused fields of the same source")
	}
	readBytes = int64(MaxArtifactBytes - len(data) + 1)
	input, _, present, err = e.choiceSource(r, p, r.RootInvocationID, ref, &readBytes, map[ArtifactRef]choiceSourceValue{})
	var problem *flow.Problem
	if !errors.As(err, &problem) || problem.Code != "condition_input_limit" || present || input.SourceRef == nil || *input.SourceRef != artifact.Ref() {
		t.Fatalf("over-budget source became unknown or lost its reference: %+v %v", input, err)
	}
}

func TestCorePublicContracts(t *testing.T) {
	distributed, err := os.ReadFile("../../schemas/core/public.schema.json")
	if err != nil || !bytes.Equal(distributed, corePublicContracts) {
		t.Fatalf("core embedded/distributed schemas differ: %v", err)
	}
	// Reuse the ordinary empty workflow fixture, but reopen it with the exact
	// new configuration contract before producing actual core read views.
	prior, options := emptyRuntime(t)
	definitions, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	configuration := prior.Config
	configuration.ConfigurationSchemaRef = builtinRef(definitions, "core:schema/core-configuration")
	configuration.Configuration.SchemaVersion = CoreConfigVersion
	configuration.Configuration.SemanticsProfile = flow.CoreProfile
	writeRuntimeJSON(t, filepath.Join(prior.Root, "prifly.json"), configuration)
	if err := prior.Close(); err != nil {
		t.Fatal(err)
	}
	e, err := Open(prior.Root, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close() })
	preview, err := e.Preview(PreviewOptions{WorkflowFile: options.WorkflowFile, BriefFile: options.BriefFile})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	started, err := e.Start(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	view, err := e.View(ctx, started.Receipt.RunID)
	if err != nil {
		t.Fatal(err)
	}
	object := func(value any) map[string]any {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var result map[string]any
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	// The published v1 capability contract stays readable; current discovery
	// uses a separate v2 contract to enumerate both Core state/read versions.
	legacyCapabilities := Capabilities()
	legacyCapabilities.SchemaVersion = "capabilities/1"
	for i := range legacyCapabilities.Profiles {
		profile := &legacyCapabilities.Profiles[i]
		profile.StateVersion, profile.ReadVersion = profile.StateVersions[0], profile.ReadVersions[0]
		profile.StateVersions, profile.ReadVersions = nil, nil
	}
	values := map[string]any{
		"CoreRunView": view, "CoreRunState": view.Run, "CorePreview": preview,
		"CapabilityManifest": legacyCapabilities, "CoreConfiguration": e.Config.Configuration,
		"EffectiveConfiguration": view.Run.EffectiveConfiguration,
		// Projection shape fixture; execution/provenance intake is tested by
		// projection runtime tests, not proved by validating this descriptor.
		"JSONProjection": ProjectionManifest{
			SchemaVersion:      "json-projection/1",
			Source:             ArtifactRef{ArtifactID: "artifact:source", Revision: 1, Digest: rawDigest([]byte(`null`))},
			Pointer:            "",
			ProjectedSchemaRef: flow.Ref{ID: "test:schema/null", Version: "1.0.0", Digest: rawDigest([]byte(`{"type":"null"}`))},
			WorkflowRef:        view.Run.WorkflowRef,
		},
	}
	for name, value := range values {
		t.Run(name, func(t *testing.T) {
			schema, err := PublicSchema(name)
			if err != nil || object(json.RawMessage(schema))["$id"] != "urn:prifly:core-public:1" {
				t.Fatalf("wrong public bundle for %s: %v", name, err)
			}
			if err := validatePublic(t, name, value); err != nil {
				t.Fatal(err)
			}
			changed := object(value)
			changed["unannounced_extension"] = true
			if err := validatePublic(t, name, changed); err == nil {
				t.Fatal("unknown field accepted")
			}
			delete(changed, "unannounced_extension")
			changed["schema_version"] = "unsupported/99"
			if err := validatePublic(t, name, changed); err == nil {
				t.Fatal("unsupported version accepted")
			}
		})
	}
	projection := object(values["JSONProjection"])
	delete(projection, "pointer")
	if validatePublic(t, "JSONProjection", projection) == nil {
		t.Fatal("projection accepted a missing pointer instead of an explicit whole-value pointer")
	}
	projection["pointer"] = "/bad~2"
	if validatePublic(t, "JSONProjection", projection) == nil {
		t.Fatal("projection accepted an invalid JSON Pointer escape")
	}
	projection = object(values["JSONProjection"])
	projection["source_ref"].(map[string]any)["revision"] = 0
	if validatePublic(t, "JSONProjection", projection) == nil {
		t.Fatal("projection accepted an invalid source revision")
	}
	if validatePublic(t, "FoundationRunView", view) == nil || validatePublic(t, "FoundationPreview", preview) == nil {
		t.Fatal("core view accepted as an unchanged F1 contract")
	}
	for _, name := range []string{"CoreRunState", "CorePreview"} {
		changed := object(values[name])
		delete(changed, "effective_configuration")
		if validatePublic(t, name, changed) == nil {
			t.Fatalf("%s accepts missing effective configuration", name)
		}
		changed["effective_configuration"] = nil
		if validatePublic(t, name, changed) == nil {
			t.Fatalf("%s accepts null effective configuration", name)
		}
		changed = object(values[name])
		changed["semantics_profile"] = flow.Profile
		if validatePublic(t, name, changed) == nil {
			t.Fatalf("%s accepts F1 semantics", name)
		}
	}
	for _, source := range []string{"package_default", "project", "run", "absent"} {
		c := *view.Run.EffectiveConfiguration
		value := ConfigurationValue{Source: source}
		if source != "absent" {
			value.Value = json.RawMessage(`null`)
		}
		c.Inputs = map[string]ConfigurationValue{"nullable_value": value}
		if err := validatePublic(t, "EffectiveConfiguration", c); err != nil {
			t.Fatalf("source %s rejected a valid value/absence: %v", source, err)
		}
		if source == "absent" {
			value.Value = json.RawMessage(`null`)
		} else {
			value.Value = nil
		}
		c.Inputs["nullable_value"] = value
		if validatePublic(t, "EffectiveConfiguration", c) == nil {
			t.Fatalf("source %s did not distinguish missing value from null", source)
		}
	}
	c := *view.Run.EffectiveConfiguration
	c.Inputs = map[string]ConfigurationValue{"bad": {Source: "environment", Value: json.RawMessage(`true`)}}
	if validatePublic(t, "EffectiveConfiguration", c) == nil {
		t.Fatal("undeclared configuration source accepted")
	}
	c.Inputs = nil
	if validatePublic(t, "EffectiveConfiguration", c) == nil {
		t.Fatal("null configuration inputs accepted")
	}
	changed := object(e.Config.Configuration)
	changed["semantics_profile"] = "future/99"
	if validatePublic(t, "CoreConfiguration", changed) == nil {
		t.Fatal("configuration accepts unknown profile")
	}
	changed["semantics_profile"] = flow.Profile
	if err := validatePublic(t, "CoreConfiguration", changed); err != nil {
		t.Fatal("new configuration contract cannot select F1 for new runs", err)
	}
	changed["input_values"] = nil
	if validatePublic(t, "CoreConfiguration", changed) == nil {
		t.Fatal("configuration does not preserve the builtin input_values constraint")
	}
	for _, name := range []string{"runtime_Run", "CoreNextView", "CoreTelemetryQuery", "Unknown"} {
		if _, err := PublicSchema(name); err == nil {
			t.Fatalf("unpublished contract %q is selectable", name)
		}
	}
}

// A bundle that is generated and drift-checked but not embedded is a contract
// the tool publishes and then cannot hand out. Three of them had accumulated
// that way, because nothing compared the files on disk against what PublicSchema
// can actually reach.
func TestEveryGeneratedBundleIsReachable(t *testing.T) {
	files, err := filepath.Glob("*.schema.json")
	if err != nil || len(files) == 0 {
		t.Fatalf("no generated bundles found: %v", err)
	}
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var bundle map[string]json.RawMessage
		if err := json.Unmarshal(content, &bundle); err != nil {
			t.Fatalf("%s is not a bundle: %v", file, err)
		}
		var names []string
		if err := json.Unmarshal(bundle["x-prifly-contracts"], &names); err != nil || len(names) == 0 {
			t.Fatalf("%s names no contracts: %v", file, err)
		}
		// One name per bundle is enough: reaching it proves the bundle is
		// embedded and searched at all.
		if _, err := PublicSchema(names[0]); err != nil {
			t.Fatalf("%s declares %s but PublicSchema cannot serve it: %v", file, names[0], err)
		}
	}
}
