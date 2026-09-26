package main

import (
	"reflect"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// nextHandoffField names what only next 41 carries: how a Run got to its
// action, its checkpoint, the bounded repeats around it and what continues
// it. Bundles published before it describe the answer without them.
func nextHandoffField(t reflect.Type, name string) bool {
	return t == reflect.TypeFor[prifly.NextView]() && (name == "ArrivedFrom" || name == "Checkpoint" || name == "Repeats" || name == "Continuations")
}

func nextHandoffConstraints(g *generator) {
	g.property("runtime_NextView", "schema_version", map[string]any{"const": prifly.CoreHandoffNextVersion})
	g.describe("runtime_NextView", "arrived_from", "The accepted result that led to the current action, or to the finish of a Run that reached an outcome: the stage in the same invocation, what it routed by -- a step's accepted verdict, a call's child outcome, or a repeat's decision route such as on_limit -- and the outputs it left (for a repeat, those of the body it decided on), as references. After a step returned blocked, this is where the reason it handed over is found. Absent when the sealed plan's own routing names more than one settled stage into this one; absent means not named, never that there was none.")
	g.describe("runtime_NextView", "checkpoint", "The Run's last accepted checkpoint, where its workflow declares one: the output a stage declared as the workflow's checkpoint, from the accepted result settled last. Its content is the author's and is not read here.")
	g.describe("runtime_NextView", "repeats", "The bounded repeats the current action runs inside, outermost first: the iteration it is on, the limit this Run applies (a project may narrow the declared ceiling) and the stage the workflow goes to when the limit is reached. Absent when the action is inside no repeat.")
	g.describe("runtime_NextView", "continuations", "For a Run that completed with an outcome or was cancelled: the workflows of installed, trusted packages whose declared continuation takes this Run. project.continue is then among safe_next_actions. Empty means no installed package declares one -- not that continuing is impossible. project continue --prepare checks everything again.")
}
