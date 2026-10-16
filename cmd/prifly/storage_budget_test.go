package main

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestStorageBudgetShowAndReadOnlyPrepare(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "authority")
	if code, _, stderr := runCLI(t, "init", root); code != 0 {
		t.Fatal(stderr)
	}
	for _, args := range [][]string{{"storage", "budget", "show"}, {"doctor", "--storage"}, {"storage", "budget", "set", "--bytes", "1073741824", "--reason", "keep accepted evidence", "--prepare"}} {
		code, out, stderr := runCLI(t, append([]string{"--project", root}, args...)...)
		if code != 0 {
			t.Fatalf("%v: exit %d: %s", args, code, stderr)
		}
		var view map[string]any
		if err := json.Unmarshal([]byte(out), &view); err != nil {
			t.Fatal(err)
		}
		if view["schema_version"] == nil {
			t.Fatal("missing versioned budget view")
		}
	}
}
