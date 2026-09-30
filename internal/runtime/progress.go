package runtime

import (
	"context"
	_ "embed"
	"errors"
	"sort"
	"time"

	"github.com/stenhigh/prifly/internal/local"
)

// ProgramProgressReadVersion is the read of what the programs of a Run said
// about their own progress over fd 4. It is not Run state and not part of any
// Run read: the Run keeps its version while its programs report.
const ProgramProgressReadVersion = "program-progress-read/1"

//go:embed program-progress.schema.json
var programProgressContracts []byte

// Progress states. Each says how far the record can be trusted as "now", not
// how far the work is.
const (
	// ProgressReported: the latest report of a program whose attempt is still
	// running under a live driver, or the last one of a settled attempt.
	ProgressReported = "reported"
	// ProgressNotReported: the program said nothing valid about its progress.
	ProgressNotReported = "not_reported"
	// ProgressStale: a report exists, but its attempt runs with no live
	// driver, or it came from another launch; it is the last word, not now.
	ProgressStale = "stale"
	// ProgressUnavailable: this authority's storage keeps no progress.
	ProgressUnavailable = "unavailable"
)

// AttemptProgressView is one program attempt's progress. Current and Total are
// absent when the program did not name them, never zero.
type AttemptProgressView struct {
	AttemptID      string `json:"attempt_id"`
	StepInstanceID string `json:"step_instance_id"`
	StageID        string `json:"stage_id"`
	AttemptStatus  string `json:"attempt_status"`
	// Settled says the attempt is over: a report is then the last thing the
	// program said, and proves nothing about how the attempt ended.
	Settled  bool       `json:"settled"`
	State    string     `json:"state"`
	Phase    string     `json:"phase,omitempty"`
	Message  string     `json:"message,omitempty"`
	Current  *int64     `json:"current,omitempty"`
	Total    *int64     `json:"total,omitempty"`
	Observed *time.Time `json:"observed,omitempty"`
	// Accepted and Rejected count the program's reports; a rejected one was
	// malformed and is not shown. Truncated says the program wrote more than
	// the bound and the rest was discarded unread.
	Accepted  int64 `json:"accepted"`
	Rejected  int64 `json:"rejected"`
	Truncated bool  `json:"truncated"`
}

type ProgramProgressView struct {
	SchemaVersion string                `json:"schema_version"`
	RunID         string                `json:"run_id"`
	RunVersion    int64                 `json:"run_version"`
	DriverLive    bool                  `json:"driver_live"`
	AsOf          Observation           `json:"as_of"`
	Attempts      []AttemptProgressView `json:"attempts"`
}

// ProgramProgress reads the progress of every program attempt of a Run. It
// starts nothing, extends no lease and writes nothing.
func (e *Engine) ProgramProgress(ctx context.Context, id string) (ProgramProgressView, error) {
	if _, err := e.readAccess(ctx); err != nil {
		return ProgramProgressView{}, err
	}
	r, read, err := e.load(ctx, id)
	if err != nil {
		return ProgramProgressView{}, err
	}
	records, err := e.Store.RunProgress(ctx, id)
	available := true
	if errors.Is(err, local.ErrProgressUnavailable) {
		available, err = false, nil
	}
	if err != nil {
		return ProgramProgressView{}, err
	}
	live := e.driverLiveFor(id)
	return programProgressView(r, read.Snapshot.Version, records, available, live, e.clock.now()), nil
}

func programProgressView(r Run, version int64, records map[string]local.AttemptProgress, available, live bool, asOf Observation) ProgramProgressView {
	view := ProgramProgressView{SchemaVersion: ProgramProgressReadVersion, RunID: r.ID, RunVersion: version, DriverLive: live, AsOf: asOf, Attempts: []AttemptProgressView{}}
	for _, a := range r.Attempts {
		// A host's attempt has no fd 4; its progress is its session's.
		if a == nil || a.Session != nil {
			continue
		}
		row := AttemptProgressView{AttemptID: a.ID, StepInstanceID: a.StepID, AttemptStatus: a.Status, Settled: a.Settled != nil, State: ProgressNotReported}
		if activation := r.Activations[a.ActivationID]; activation != nil {
			row.StageID = activation.StageID
		}
		record, found := records[a.ID]
		switch {
		case !available:
			row.State = ProgressUnavailable
		case !found:
		default:
			row.Accepted, row.Rejected, row.Truncated = record.Accepted, record.Rejected, record.Truncated
			if record.Progress.Phase == "" {
				break
			}
			observed := record.Observed
			row.Phase, row.Message, row.Current, row.Total, row.Observed = record.Progress.Phase, record.Progress.Message, record.Progress.Current, record.Progress.Total, &observed
			switch {
			case a.Process == nil || a.Process.LaunchID != record.LaunchID:
				row.State = ProgressStale
			case row.Settled:
				row.State = ProgressReported
			case live:
				row.State = ProgressReported
			default:
				row.State = ProgressStale
			}
		}
		view.Attempts = append(view.Attempts, row)
	}
	sort.Slice(view.Attempts, func(i, j int) bool { return view.Attempts[i].AttemptID < view.Attempts[j].AttemptID })
	return view
}
