package runtime

import (
	"context"

	"github.com/stenhigh/prifly/internal/local"
)

// MonitorMaxRuns bounds one listing. A monitor reads what is there; it does not
// become a second unbounded scan of the authority.
const MonitorMaxRuns = 200

// RunSummary is the compact projection a monitor lists. Every field is copied
// from the recorded Run: nothing here is derived, estimated or filled in.
type RunSummary struct {
	SchemaVersion string  `json:"schema_version"`
	ID            string  `json:"run_id"`
	WorkflowID    string  `json:"workflow_id"`
	Profile       string  `json:"profile"`
	Status        string  `json:"status"`
	Outcome       *string `json:"outcome"`
	Created       string  `json:"created"`
	LastObserved  string  `json:"last_observed"`
	Steps         int     `json:"step_instances"`
	Attempts      int     `json:"attempts"`
	Invocations   int     `json:"invocations"`
	Active        int     `json:"active_attempts"`
	AwaitingHosts int     `json:"awaiting_hosts"`
	// Settled counts what is already behind: with one work finishing as another
	// starts, the momentary counters can repeat while the Run is moving.
	SettledAttempts int   `json:"settled_attempts"`
	Version         int64 `json:"run_version"`
	Events          int64 `json:"event_sequence"`
}

// Runs lists the runs this reader may see, newest observation first. Access is
// checked before anything is selected, exactly as it is for telemetry: a reader
// who may not read this project sees nothing rather than a filtered subset.
func (e *Engine) Runs(ctx context.Context) ([]RunSummary, error) {
	if _, err := e.readAccess(ctx); err != nil {
		return nil, err
	}
	snapshots, _, err := e.Store.ReadAll(ctx, MonitorMaxRuns)
	if err != nil {
		return nil, err
	}
	summaries := make([]RunSummary, 0, len(snapshots))
	for _, snapshot := range snapshots {
		var r Run
		if err := decodeState(snapshot.Data, &r); err != nil {
			return nil, err
		}
		if !supportedRun(r) || r.AuthorityID != e.Installation.ID || r.ProjectID != e.Config.ID {
			return nil, local.ErrIntegrity
		}
		awaiting, settled := 0, 0
		for _, a := range r.Attempts {
			if a == nil {
				continue
			}
			if a.Settled != nil {
				settled++
			}
			if a.Session != nil && a.Session.HostState == SessionAwaiting && a.Settled == nil {
				awaiting++
			}
		}
		summaries = append(summaries, RunSummary{
			SchemaVersion: r.SchemaVersion, ID: r.ID, WorkflowID: r.WorkflowRef.ID, Profile: r.Profile,
			Status: r.Status, Outcome: r.Outcome, Created: r.Created.UTC, LastObserved: r.LastObserved.UTC,
			Steps: len(r.Steps), Attempts: len(r.Attempts), Invocations: len(r.Invocations),
			Active: len(r.Active), AwaitingHosts: awaiting, SettledAttempts: settled,
			Version: snapshot.Version, Events: snapshot.EventSeq,
		})
	}
	return summaries, nil
}
