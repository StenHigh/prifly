package runtime

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/stenhigh/prifly/internal/local"
)

// MonitorMaxRuns bounds one metadata page, not the complete Run population.
const MonitorMaxRuns = 200

// RunSummary is the compact projection a monitor lists. Every field is copied
// from the recorded Run: nothing here is derived, estimated or filled in.
type RunSummary struct {
	Subject       string   `json:"subject"`
	Executors     []string `json:"executors"`
	SchemaVersion string   `json:"schema_version"`
	ID            string   `json:"run_id"`
	WorkflowID    string   `json:"workflow_id"`
	Profile       string   `json:"profile"`
	Status        string   `json:"status"`
	Outcome       *string  `json:"outcome"`
	Created       string   `json:"created"`
	LastObserved  string   `json:"last_observed"`
	Steps         int      `json:"step_instances"`
	Attempts      int      `json:"attempts"`
	Invocations   int      `json:"invocations"`
	Active        int      `json:"active_attempts"`
	AwaitingHosts int      `json:"awaiting_hosts"`
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
	summaries := []RunSummary{}
	after := ""
	for {
		revisions, next, err := e.MonitorRevisions(ctx, after)
		if err != nil {
			return nil, err
		}
		for _, revision := range revisions {
			summary, err := e.MonitorSummary(ctx, revision.RunID)
			if err != nil {
				return nil, err
			}
			summaries = append(summaries, summary)
		}
		if next == "" {
			break
		}
		after = next
	}
	sort.Slice(summaries, func(i, j int) bool {
		a, _ := time.Parse(time.RFC3339Nano, summaries[i].LastObserved)
		b, _ := time.Parse(time.RFC3339Nano, summaries[j].LastObserved)
		if a.Equal(b) {
			return summaries[i].ID < summaries[j].ID
		}
		return a.After(b)
	})
	return summaries, nil
}

func (e *Engine) MonitorRevisions(ctx context.Context, after string) ([]local.Snapshot, string, error) {
	if _, err := e.readAccess(ctx); err != nil {
		return nil, "", err
	}
	return e.Store.RevisionPage(ctx, after, MonitorMaxRuns)
}

func (e *Engine) MonitorSummary(ctx context.Context, id string) (RunSummary, error) {
	if _, err := e.readAccess(ctx); err != nil {
		return RunSummary{}, err
	}
	r, view, err := e.load(ctx, id)
	if err != nil {
		return RunSummary{}, err
	}
	snapshot := view.Snapshot
	var brief Brief
	if r.Brief != (ArtifactRef{}) {
		_, body, err := e.Artifact(r.Brief)
		if err != nil {
			return RunSummary{}, err
		}
		if err := json.Unmarshal(body, &brief); err != nil {
			return RunSummary{}, err
		}
	}
	executorNames := map[string]bool{}
	for name := range r.Executors {
		executorNames[name] = true
	}
	for _, attempt := range r.Attempts {
		if attempt != nil && attempt.Session != nil && attempt.Session.PrincipalID != "" {
			executorNames[attempt.Session.PrincipalID] = true
		}
	}
	executors := []string{}
	for name := range executorNames {
		executors = append(executors, name)
	}
	sort.Strings(executors)
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
	return RunSummary{Subject: brief.Subject, Executors: executors,
		SchemaVersion: r.SchemaVersion, ID: r.ID, WorkflowID: r.WorkflowRef.ID, Profile: r.Profile,
		Status: r.Status, Outcome: r.Outcome, Created: r.Created.UTC, LastObserved: r.LastObserved.UTC,
		Steps: len(r.Steps), Attempts: len(r.Attempts), Invocations: len(r.Invocations),
		Active: len(r.Active), AwaitingHosts: awaiting, SettledAttempts: settled,
		Version: snapshot.Version, Events: snapshot.EventSeq,
	}, nil
}

// MonitorView is the owner's local inspection surface. It includes pinned plans
// and the actual task payload; credentials and executable environments stay out.
// The ordinary public View keeps its existing redaction and wire behavior.
type MonitorRunView struct {
	RunView
	SchemaVersion string           `json:"schema_version"`
	ReadVersion   string           `json:"read_version"`
	Choices       []ChoiceDecision `json:"choices"`
}

func (e *Engine) MonitorView(ctx context.Context, id string) (MonitorRunView, error) {
	if _, err := e.readAccess(ctx); err != nil {
		return MonitorRunView{}, err
	}
	r, read, err := e.load(ctx, id)
	if err != nil {
		return MonitorRunView{}, err
	}
	if err = e.hydrateTransitions(ctx, &r); err != nil {
		return MonitorRunView{}, err
	}
	asOf, live := e.clock.now(), e.driverLiveFor(id)
	timing := Timing(r, asOf, live)
	r.Executors = nil
	for _, a := range r.Attempts {
		a.TokenHash = ""
	}
	for _, check := range r.CheckExecutions {
		check.TokenHash = ""
	}
	choices := []ChoiceDecision{}
	after := int64(0)
	for {
		events, more, err := e.Store.ReadEventsOfType(ctx, id, "stage.choice_decided", after, 200)
		if err != nil {
			return MonitorRunView{}, err
		}
		for _, event := range events {
			after = event.Seq
			if event.Seq > read.Snapshot.EventSeq {
				continue
			}
			var decision ChoiceDecision
			if err = json.Unmarshal(event.Data, &decision); err != nil {
				return MonitorRunView{}, err
			}
			choices = append(choices, decision)
		}
		if !more || after >= read.Snapshot.EventSeq {
			break
		}
	}
	return MonitorRunView{RunView: RunView{readVersionFor(r.SchemaVersion, r.Profile), read.Snapshot.Version, read.Snapshot.EventSeq, read.Cut, asOf, live, r, timing}, Choices: choices, SchemaVersion: "local-run-monitor/1", ReadVersion: readVersionFor(r.SchemaVersion, r.Profile)}, nil
}

func (e *Engine) MonitorAccess(ctx context.Context) error {
	_, err := e.readAccess(ctx)
	return err
}
