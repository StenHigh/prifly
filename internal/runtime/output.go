package runtime

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/stenhigh/prifly/internal/local"
)

// ProgramOutputReadVersion is the read of what a program opened to readers:
// the newest bytes of its stdout and stderr, kept beside the Run.
const ProgramOutputReadVersion = "program-output-read/1"

func isLiveOutputState(version string) bool { return atLeast(version, CoreLiveOutputStateVersion) }

// requiresLiveOutputState says whether any sealed executor opens its output
// or names an output limit: only such a Run is sealed at 43.
func requiresLiveOutputState(executors map[string]PinnedExecutor) bool {
	for _, executor := range executors {
		if executor.Config.LiveOutput || executor.Config.OutputLimitBytes != 0 {
			return true
		}
	}
	return false
}

// OutputStreamView is what is kept of one stream. Text starts at StartOffset
// bytes into the stream; a reader who last saw an earlier end missed what lies
// between, and Gap says so for the chunks it holds. The text is the program's
// own, untrusted, with invalid UTF-8 replaced.
type OutputStreamView struct {
	Stream      string     `json:"stream"`
	StartOffset int64      `json:"start_offset"`
	EndOffset   int64      `json:"end_offset"`
	Gap         bool       `json:"gap"`
	Text        string     `json:"text"`
	Observed    *time.Time `json:"observed,omitempty"`
}

type ProgramOutputView struct {
	SchemaVersion string `json:"schema_version"`
	RunID         string `json:"run_id"`
	AttemptID     string `json:"attempt_id"`
	StageID       string `json:"stage_id"`
	// Disclosed is false when the program's executor did not open its output:
	// then nothing of it is here, whatever the driver saw.
	Disclosed     bool   `json:"disclosed"`
	State         string `json:"state"`
	AttemptStatus string `json:"attempt_status"`
	Settled       bool   `json:"settled"`
	// OutputLimitBytes is how much the program may print to each stream
	// before it is stopped for it.
	OutputLimitBytes int64              `json:"output_limit_bytes"`
	Streams          []OutputStreamView `json:"streams"`
}

// ProgramOutput reads the kept output of one program attempt. It starts
// nothing and writes nothing, and it returns no bytes of a program whose
// executor did not open them.
func (e *Engine) ProgramOutput(ctx context.Context, runID, attemptID string) (ProgramOutputView, error) {
	if _, err := e.readAccess(ctx); err != nil {
		return ProgramOutputView{}, err
	}
	r, _, err := e.load(ctx, runID)
	if err != nil {
		return ProgramOutputView{}, err
	}
	a := r.Attempts[attemptID]
	if a == nil || a.Session != nil {
		return ProgramOutputView{}, fault("attempt_not_found", "no program attempt "+attemptID+" in "+runID)
	}
	view := ProgramOutputView{SchemaVersion: ProgramOutputReadVersion, RunID: runID, AttemptID: attemptID, State: ProgressNotReported, AttemptStatus: a.Status, Settled: a.Settled != nil, OutputLimitBytes: DefaultProgramOutputBytes, Streams: []OutputStreamView{}}
	activation := r.Activations[a.ActivationID]
	if activation != nil {
		view.StageID = activation.StageID
	}
	config, found := attemptExecutorConfig(r, a)
	if found {
		view.OutputLimitBytes = programOutputLimit(config)
	}
	if !found || !config.LiveOutput || !isLiveOutputState(r.SchemaVersion) {
		return view, nil
	}
	view.Disclosed = true
	chunks, err := e.Store.ReadAttemptOutput(ctx, attemptID)
	if errors.Is(err, local.ErrOutputUnavailable) {
		view.State = ProgressUnavailable
		return view, nil
	}
	if err != nil {
		return ProgramOutputView{}, err
	}
	view.Streams = outputStreams(chunks)
	if len(view.Streams) != 0 {
		view.State = progressFreshness(a, chunks[len(chunks)-1].LaunchID, e.driverLiveFor(runID))
	}
	return view, nil
}

// progressFreshness is the same judgement progress gets: a record is now only
// for a running attempt of this launch under a live driver, or the last word
// of a settled one; otherwise it is stale.
func progressFreshness(a *Attempt, launchID string, live bool) string {
	switch {
	case a.Process == nil || a.Process.LaunchID != launchID:
		return ProgressStale
	case a.Settled != nil, live:
		return ProgressReported
	default:
		return ProgressStale
	}
}

// outputStreams joins the kept chunks of each stream. Chunks that do not
// meet leave a gap, which the view names instead of closing.
func outputStreams(chunks []local.AttemptOutput) []OutputStreamView {
	byStream := map[string][]local.AttemptOutput{}
	for _, chunk := range chunks {
		byStream[chunk.Stream] = append(byStream[chunk.Stream], chunk)
	}
	streams := []OutputStreamView{}
	for name, list := range byStream {
		sort.Slice(list, func(i, j int) bool { return list[i].Offset < list[j].Offset })
		view := OutputStreamView{Stream: name, StartOffset: list[0].Offset}
		var text strings.Builder
		end := list[0].Offset
		for _, chunk := range list {
			if chunk.Offset > end {
				view.Gap = true
			}
			data := chunk.Data
			if chunk.Offset < end {
				data = data[min(int64(len(data)), end-chunk.Offset):]
			}
			text.Write(data)
			end = max(end, chunk.Offset+int64(len(chunk.Data)))
			observed := chunk.Observed
			view.Observed = &observed
		}
		view.EndOffset, view.Text = end, strings.ToValidUTF8(text.String(), "�")
		streams = append(streams, view)
	}
	sort.Slice(streams, func(i, j int) bool { return streams[i].Stream < streams[j].Stream })
	return streams
}

// attemptExecutorConfig is the sealed executor config of a program attempt.
func attemptExecutorConfig(r Run, a *Attempt) (ExecutorConfig, bool) {
	activation := r.Activations[a.ActivationID]
	if activation == nil {
		return ExecutorConfig{}, false
	}
	p, err := r.planFor(activation.InvocationID)
	if err != nil {
		return ExecutorConfig{}, false
	}
	step, found := p.Steps[activation.StageID]
	if !found {
		return ExecutorConfig{}, false
	}
	executor, bound := r.Executors[executorKey(r, p.Workflow.Definition.Stages[activation.StageID].StepRef, step.ID)]
	return executor.Config, bound
}
