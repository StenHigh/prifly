package main

import (
	"reflect"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func decisionStateField(t reflect.Type, name string) bool {
	return t == reflect.TypeFor[prifly.Run]() && (name == "DecisionCatalog" || name == "DecisionSheet" || name == "DecisionLedger" || name == "PendingDecision") ||
		t == reflect.TypeFor[prifly.SessionHandoff]() && (name == "DecisionContext" || name == "DeliveryGeneration") ||
		t == reflect.TypeFor[prifly.SessionTask]() && (name == "RunVersion" || name == "DecisionSheet" || name == "DecisionContext" || name == "DecisionBridge") ||
		t == reflect.TypeFor[prifly.SessionSubmission]() && name == "DecisionRequest"
}

func decisionSchema() map[string]any {
	g := generator{defs: map[string]any{}}
	contracts := map[string]reflect.Type{
		"DecisionDefinition": reflect.TypeFor[prifly.DecisionDefinition](),
		"DecisionCatalog":    reflect.TypeFor[prifly.DecisionCatalog](),
		"DecisionSheet":      reflect.TypeFor[prifly.DecisionSheet](),
		"DecisionRequest":    reflect.TypeFor[prifly.DecisionRequest](),
		"DecisionAnswer":     reflect.TypeFor[prifly.DecisionAnswer](),
		"DecisionRecord":     reflect.TypeFor[prifly.DecisionRecord](),
	}
	for name, typ := range contracts {
		g.defs[name] = g.schema(typ)
	}
	for name, version := range map[string]string{
		"runtime_DecisionDefinition": prifly.DecisionDefinitionVersion,
		"runtime_DecisionCatalog":    prifly.DecisionCatalogVersion,
		"runtime_DecisionSheet":      prifly.DecisionSheetVersion,
		"runtime_DecisionRequest":    prifly.DecisionRequestVersion,
		"runtime_DecisionAnswer":     prifly.DecisionAnswerVersion,
		"runtime_DecisionRecord":     prifly.DecisionRecordVersion,
	} {
		g.property(name, "schema_version", map[string]any{"const": version})
	}
	g.property("runtime_DecisionDefinition", "phase", enum("preflight", "runtime"))
	g.property("runtime_DecisionDefinition", "sensitivity", enum("ordinary", "scope-changing", "approval-like"))
	g.property("runtime_DecisionDestination", "kind", enum("package_profile", "launch_input", "session_context"))
	g.property("runtime_DecisionRecord", "status", enum("presented", "answered", "defaulted", "rejected", "pending"))
	g.property("runtime_DecisionRecord", "source", enum("actor", "project_default", "package_default", "autonomous_policy", "unanswered"))
	return map[string]any{
		"$schema":            "https://json-schema.org/draft/2020-12/schema",
		"$id":                "urn:prifly:run-decisions:1",
		"title":              "Pri-Fly Run decision contracts",
		"description":        "Versioned data contracts for declared Run decisions. Authority, routing and answer validation remain runtime responsibilities.",
		"x-prifly-contracts": []string{"DecisionAnswer", "DecisionCatalog", "DecisionDefinition", "DecisionRecord", "DecisionRequest", "DecisionSheet"},
		"$defs":              g.defs,
	}
}

func decisionStateConstraints(g *generator) {
	for name, version := range map[string]string{
		"runtime_Run": prifly.CoreDecisionStateVersion, "runtime_RunView": prifly.CoreDecisionReadVersion,
		"runtime_NextView": prifly.CoreDecisionNextVersion, "runtime_Preview": prifly.CoreDecisionPreviewVersion,
		"runtime_StepReadView": prifly.CoreDecisionStepReadVersion,
	} {
		g.property(name, "schema_version", map[string]any{"const": version})
	}
	for _, name := range []string{"runtime_SessionHandoff", "runtime_SessionTask", "runtime_SessionSubmission"} {
		g.property(name, "schema_version", map[string]any{"const": prifly.AssistedSessionDecisionVersion})
	}
}
