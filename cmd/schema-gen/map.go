package main

import (
	"reflect"

	"github.com/stenhigh/prifly/internal/flow"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// P2-11 map fields must be excluded before traversal of every earlier bundle,
// so the delivered contracts keep their exact published bytes.
func mapField(t reflect.Type, name string) bool {
	return t == reflect.TypeFor[prifly.ParallelProgress]() && name == "Sealed"
}

func mapConstraints(g *generator) {
	ref := func(name string) map[string]any { return map[string]any{"$ref": "#/$defs/" + name} }
	for name, version := range map[string]string{
		"runtime_Run": prifly.CoreMapStateVersion, "runtime_RunView": prifly.CoreMapReadVersion,
		"runtime_NextView": prifly.CoreMapNextVersion, "runtime_Preview": prifly.CoreMapPreviewVersion,
	} {
		g.property(name, "schema_version", map[string]any{"const": version})
	}
	// An item identity carries its own type: the number 1 and the string "1"
	// are different items, so the encoded key names which one it is.
	g.property("runtime_SealedItem", "key", map[string]any{"type": "string", "minLength": 8, "maxLength": 2048, "pattern": "^(string|integer):"})
	g.property("runtime_SealedItem", "position", map[string]any{"type": "integer", "minimum": 0, "maximum": flow.MaxMapItems - 1})
	// A map that sealed an empty collection has no branches at all, so unlike a
	// parallel stage's declared branches this list may legitimately be empty.
	g.property("runtime_ParallelProgress", "branch_ids", map[string]any{"type": "array", "minItems": 0, "maxItems": flow.MaxMapItems, "uniqueItems": true, "items": ref("Identifier")})
	g.property("runtime_ParallelProgress", "sealed", map[string]any{"type": "array", "minItems": 0, "maxItems": flow.MaxMapItems, "items": ref("runtime_SealedItem")})
}
