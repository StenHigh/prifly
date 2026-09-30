package runtime

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stenhigh/prifly/internal/local"
)

// A program whose executor opens its output is sealed at 43, its output is
// read beside the Run, and printing a verdict changes nothing; a program that
// did not open it keeps nothing readable.
func TestDriverLiveOutputIsReadOnlyWhereTheExecutorOpensIt(t *testing.T) {
	t.Parallel()
	for _, opened := range []bool{true, false} {
		t.Run(map[bool]string{true: "opened", false: "closed"}[opened], func(t *testing.T) {
			t.Parallel()
			e, options := contextDriverProject(t, nil)
			config := e.Config.Configuration.Executors["test:step/context"]
			config.Environment = maps.Clone(config.Environment)
			config.Environment["DRIVER_TEST_PRINT"] = "1"
			config.LiveOutput = opened
			config.OutputLimitBytes = 128 << 10
			e.Config.Configuration.Executors["test:step/context"] = config
			started, err := e.Start(context.Background(), options)
			if err != nil {
				t.Fatal(err)
			}
			runID := started.Receipt.RunID
			if err := e.Drive(context.Background(), runID); err != nil {
				t.Fatal(err)
			}
			r := driverRun(t, e, runID)
			if r.Status != "completed" || r.SchemaVersion != CoreLiveOutputStateVersion {
				t.Fatalf("an output declaration did not run under 43: %s %s %+v", r.Status, r.SchemaVersion, r.Diagnostics)
			}
			var attempt *Attempt
			for _, a := range r.Attempts {
				attempt = a
			}
			if attempt.Accepted == nil || attempt.Accepted.Verdict != "pass" {
				t.Fatalf("printed text changed the verdict: %+v", attempt.Accepted)
			}
			view, err := e.ProgramOutput(context.Background(), runID, attempt.ID)
			if err != nil {
				t.Fatal(err)
			}
			if view.OutputLimitBytes != 128<<10 {
				t.Fatalf("the sealed limit was not read back: %d", view.OutputLimitBytes)
			}
			chunks, err := e.Store.ReadAttemptOutput(context.Background(), attempt.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !opened {
				if view.Disclosed || len(view.Streams) != 0 || len(chunks) != 0 {
					t.Fatalf("output of a program that did not open it was kept or read: %+v %d", view, len(chunks))
				}
				return
			}
			if !view.Disclosed || view.State != ProgressReported || !view.Settled || len(view.Streams) != 2 {
				t.Fatalf("opened output: %+v", view)
			}
			byName := map[string]OutputStreamView{}
			for _, stream := range view.Streams {
				byName[stream.Stream] = stream
			}
			if !strings.Contains(byName["stdout"].Text, "verdict: fail") || byName["stdout"].StartOffset != 0 || byName["stdout"].Gap || !strings.Contains(byName["stderr"].Text, "\x1b[2J") {
				t.Fatalf("streams: %+v", byName)
			}
		})
	}
}

func TestOutputStreamsNameWhatWasSkipped(t *testing.T) {
	t.Parallel()
	at := time.Now()
	streams := outputStreams([]local.AttemptOutput{
		{Stream: "stdout", Offset: 10, Data: []byte("abc"), Observed: at},
		{Stream: "stdout", Offset: 13, Data: []byte("def"), Observed: at},
		{Stream: "stdout", Offset: 20, Data: []byte("xyz"), Observed: at},
		{Stream: "stdout", Offset: 21, Data: []byte("yz!"), Observed: at},
		{Stream: "stderr", Offset: 0, Data: []byte{'o', 'k', 0xff}, Observed: at},
	})
	if len(streams) != 2 || streams[0].Stream != "stderr" || streams[0].Text != "ok�" || streams[0].Gap {
		t.Fatalf("stderr: %+v", streams)
	}
	out := streams[1]
	if out.StartOffset != 10 || out.EndOffset != 24 || !out.Gap || out.Text != "abcdefxyz!" {
		t.Fatalf("stdout: %+v", out)
	}
}

func TestLiveOutputStateIsRequiredOnlyByADeclaration(t *testing.T) {
	t.Parallel()
	for _, config := range []ExecutorConfig{{LiveOutput: true}, {OutputLimitBytes: 1}} {
		if requiresLiveOutputState(map[string]PinnedExecutor{"x": {Config: config}}) != true {
			t.Fatalf("%+v does not require 43", config)
		}
	}
	if requiresLiveOutputState(map[string]PinnedExecutor{"x": {Config: ExecutorConfig{MaxOutputBytes: 1}}}) {
		t.Fatal("a plain executor was sealed at 43")
	}
}

// While the program works, a second reader sees what it printed so far and
// that it is now; after it ends, the same output is its last word.
func TestDriverLiveOutputIsReadWhileTheProgramWorks(t *testing.T) {
	t.Parallel()
	e, options := contextDriverProject(t, nil)
	config := e.Config.Configuration.Executors["test:step/context"]
	config.Environment = maps.Clone(config.Environment)
	config.Environment["DRIVER_TEST_PRINT"] = "1"
	config.Args = append(append([]string{}, config.Args[:len(config.Args)-1]...), "progress-pass")
	config.LiveOutput = true
	e.Config.Configuration.Executors["test:step/context"] = config
	started, err := e.Start(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	runID := started.Receipt.RunID
	_, finished := driverAsync(t, e, runID)
	r := driverWait(t, e, runID, func(r Run) bool {
		for _, a := range r.Attempts {
			if a.Started != nil {
				return true
			}
		}
		return false
	})
	a := r.Attempts[r.Active[0]]
	var live ProgramOutputView
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if live, err = e.ProgramOutput(context.Background(), runID, a.ID); err != nil {
			t.Fatal(err)
		}
		if len(live.Streams) == 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(live.Streams) != 2 || live.State != ProgressReported || live.Settled || !strings.Contains(live.Streams[1].Text, "running product suite") {
		t.Fatalf("a second reader did not see the working program's output: %+v", live)
	}
	if err := os.WriteFile(filepath.Join(a.Workspace, "finish"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	driverDone(t, finished, false)
	final, err := e.ProgramOutput(context.Background(), runID, a.ID)
	if err != nil || !final.Settled || final.State != ProgressReported {
		t.Fatalf("after the program ended: %+v %v", final, err)
	}
}
