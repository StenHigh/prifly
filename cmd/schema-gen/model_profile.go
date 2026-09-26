package main

import (
	"reflect"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// modelProfileField names what only the model-profile bundle carries: what a
// step's author asked of the model that executes it. Bundles published before
// it describe the task without it, byte for byte.
func modelProfileField(t reflect.Type, name string) bool {
	if t == reflect.TypeFor[prifly.SessionTask]() && name == "ModelProfile" {
		return true
	}
	// The host's answer is a new fact of one attempt, so unlike the
	// declaration it is recorded -- and every bundle published before this one
	// describes an attempt without it.
	if t == reflect.TypeFor[prifly.Attempt]() && name == "ModelProfileReport" {
		return true
	}
	// And the field the host sends it in. Without this the statement's
	// definition leaks into every bundle that carries a submission, which is
	// every bundle: a contract nobody published announcing a field nobody can
	// send it.
	return t == reflect.TypeFor[prifly.SessionSubmission]() && name == "ModelProfile"
}

func modelProfileConstraints(g *generator) {
	for name, version := range map[string]string{
		"runtime_Run": prifly.CoreModelProfileStateVersion, "runtime_RunView": prifly.CoreModelProfileReadVersion,
		"runtime_NextView": prifly.CoreModelProfileNextVersion, "runtime_Preview": prifly.CoreModelProfilePreviewVersion,
		"runtime_StepReadView": prifly.CoreModelProfileStepReadVersion,
	} {
		g.property(name, "schema_version", map[string]any{"const": version})
	}
	// The cap this document sets on its own version lists was 32, and the
	// build filled it. The owner withdrew the compatibility it protected on
	// 2026-09-19 -- this engine has one user -- so the new bundle allows more
	// and every earlier bundle keeps saying 32, byte for byte.
	for _, field := range []string{"state_versions", "read_versions"} {
		g.property("runtime_ProfileCapabilities", field, map[string]any{"type": "array", "items": map[string]any{"type": "string", "minLength": 1}, "minItems": 1, "maxItems": 64, "uniqueItems": true})
	}
	g.describe("runtime_Attempt", "model_profile_report", "What the host said it did with the model profile this step declared: honoured and which model, unavailable because its platform does not choose, or declined and why. The host's statement about itself, recorded as given. This authority holds no channel to a session that existed before the Run and confirms nothing here.")
	// The task carries the declaration on the edition it was already written
	// under: the profile is sealed in the plan, so no handoff records it and no
	// stored session moves. Pinning a later edition here described a task this
	// engine never emits, and every bundle from this one on repeated it.
	g.property("runtime_SessionTask", "schema_version", map[string]any{"const": prifly.AssistedSessionRoutedVersion})
	g.describe("runtime_SessionTask", "model_profile", "What this step's author asked of the model that executes it, read from the sealed plan. A profile of work, not a provider or product. Absent means the step asked nothing, never that the host may decide silently. This authority selects no model: an assisted session exists before the Run and it holds no channel to it.")
}
