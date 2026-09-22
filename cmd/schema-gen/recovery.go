package main

import prifly "github.com/stenhigh/prifly/internal/runtime"

func recoveryConstraints(g *generator) {
	g.property("runtime_Run", "schema_version", map[string]any{"const": prifly.CoreRecoveryStateVersion})
	g.property("runtime_RunView", "schema_version", map[string]any{"const": prifly.CoreRecoveryReadVersion})
	g.describe("runtime_Run", "recovery", "Exact source Run, accepted evidence, frontier and copied output references. Reused source Attempts are provenance, not Attempts executed by this Run.")
}
