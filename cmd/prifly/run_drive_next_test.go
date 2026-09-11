package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// run drive answers with the shared read document, the one status and timing
// also return, so it says what the Run is and never what to do now. A dependent
// session called another command after every successful step to learn that --
// nine extra round trips in one Run. Widening that read contract for three
// commands to save a call in one is the wrong trade, so drive can be asked for
// the other document by name instead.
func TestRunDriveCanAnswerWithTheNextActionInsteadOfTheRunView(t *testing.T) {
	project := t.TempDir()
	if err := prifly.Init(project); err != nil {
		t.Fatal(err)
	}
	brief := prifly.Brief{SchemaVersion: "1", ID: "test:brief/drive-next", Subject: "Drive reports where it got to", DesiredOutcome: "Finish with no_work", InScope: []string{"Local state"}, OutOfScope: []string{"Network"}, CompletionCriteria: []string{"no_work"}, SourceRefs: []prifly.ArtifactRef{}, Assumptions: []string{}, Confirmation: "explicit"}
	for path, value := range map[string]any{"workflows/drive.json": emptyCLIWorkflow(t), "brief.json": brief} {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(project, path), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	run := func(arguments ...string) (int, string) {
		t.Helper()
		var out, errout bytes.Buffer
		code := execute(context.Background(), append([]string{"--project", project}, arguments...), &out, &errout)
		if code != 0 {
			t.Fatalf("%v: exit=%d %s", arguments, code, errout.String())
		}
		return code, out.String()
	}
	_, started := run("--json", "run", "start", "--workflow", "workflows/drive.json", "--brief", "brief.json", "--command-id", "command:drive-next")
	var receipt struct {
		Receipt struct {
			RunID string `json:"run_id"`
		} `json:"receipt"`
	}
	if err := json.Unmarshal([]byte(started), &receipt); err != nil || receipt.Receipt.RunID == "" {
		t.Fatalf("no run id in the start result: %v %s", err, started)
	}

	_, asked := run("--json", "run", "drive", receipt.Receipt.RunID, "--next")
	var next prifly.NextView
	if err := json.Unmarshal([]byte(asked), &next); err != nil {
		t.Fatal(err)
	}
	if next.SchemaVersion != "foundation-next/1" || next.Action == "" {
		t.Fatalf("drive --next did not answer with the next action: %s", asked)
	}
	if next.RunID != receipt.Receipt.RunID || len(next.SafeNextActions) == 0 {
		t.Fatalf("the next document does not describe this Run: %s", asked)
	}

	// Without the flag the answer is unchanged, because status and timing read
	// the same document and a widened contract would reach them too.
	_, plain := run("--json", "run", "drive", receipt.Receipt.RunID)
	var view prifly.RunView
	if err := json.Unmarshal([]byte(plain), &view); err != nil {
		t.Fatal(err)
	}
	if view.SchemaVersion != prifly.ReadVersion || view.Run.ID != receipt.Receipt.RunID {
		t.Fatalf("plain drive stopped answering with the run view: %s", plain)
	}
	var stray map[string]any
	if err := json.Unmarshal([]byte(plain), &stray); err != nil {
		t.Fatal(err)
	}
	if _, widened := stray["action"]; widened {
		t.Fatalf("the read contract gained an action field, which status and timing return too: %s", plain)
	}
}
