package main

import (
	"reflect"

	"github.com/stenhigh/prifly/internal/flow"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// P2-10 branch fan-out fields must be excluded before traversal of every
// earlier bundle, so the delivered contracts keep their exact published bytes.
func parallelField(t reflect.Type, name string) bool {
	return t == reflect.TypeFor[prifly.Activation]() && name == "Parallel" ||
		t == reflect.TypeFor[prifly.Invocation]() && name == "BranchID"
}

func parallelConstraints(g *generator) {
	ref := func(name string) map[string]any { return map[string]any{"$ref": "#/$defs/" + name} }
	for name, version := range map[string]string{
		"runtime_Run": prifly.CoreParallelStateVersion, "runtime_RunView": prifly.CoreParallelReadVersion,
		"runtime_NextView": prifly.CoreParallelNextVersion, "runtime_Preview": prifly.CoreParallelPreviewVersion,
	} {
		g.property(name, "schema_version", map[string]any{"const": version})
	}
	g.property("runtime_JoinDecision", "schema_version", map[string]any{"const": prifly.JoinDecisionVersion})
	g.property("runtime_Invocation", "branch_id", ref("Identifier"))
	g.property("runtime_JoinDecision", "branch_id", ref("Identifier"))
	g.property("runtime_JoinDecision", "next_branch_id", ref("Identifier"))
	g.property("runtime_JoinDecision", "stage_id", ref("Identifier"))
	g.property("runtime_JoinDecision", "next_stage_id", ref("Identifier"))
	g.property("runtime_JoinDecision", "mode", enum("all", "quorum"))
	g.property("runtime_JoinDecision", "selection", enum("all", "first_observed"))
	g.property("runtime_JoinDecision", "remainder", enum("wait", "cancel"))
	g.property("runtime_JoinDecision", "branch_status", enum("completed", "failed"))
	g.property("runtime_JoinDecision", "verdict", enum("satisfied", "unsatisfied", "undecided"))
	// Every route a decision can carry, including the two that settle nothing:
	// a branch folded in while the join still owes itself decisions, and a
	// verdict waiting for the branches it asked to stop.
	g.property("runtime_JoinDecision", "route", enum("next_branch", "recorded", "cancelling", "satisfied", "unsatisfied", "on_error", "failed"))
	// What happened to the branches a decision left out. Never entered and
	// stopped are different facts and are recorded as different words.
	g.property("runtime_JoinDecision", "remainder_disposition", enum("not_entered", "cancel_requested", "cancelled"))
	counted := map[string]any{"type": "integer", "minimum": 0, "maximum": flow.MaxParallelBranches}
	for _, name := range []string{"position", "branch_count", "accepted_count", "observed_count"} {
		g.property("runtime_JoinDecision", name, counted)
	}
	g.property("runtime_ParallelProgress", "entered_count", counted)
	g.property("runtime_ParallelProgress", "branch_ids", map[string]any{"type": "array", "minItems": 1, "maxItems": flow.MaxParallelBranches, "uniqueItems": true, "items": ref("Identifier")})
}
