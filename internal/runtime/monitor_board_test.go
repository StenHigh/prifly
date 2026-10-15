package runtime

import (
	"context"
	"testing"

	"github.com/stenhigh/prifly/internal/local"
)

func TestMonitorBoardDistinguishesRecordedWorkAndWaiting(t *testing.T) {
	t.Parallel()
	observation := &Observation{}
	for _, c := range []struct {
		name, status, host string
		started, taken     bool
		want               string
	}{
		{name: "pending", status: "pending", want: "waiting"},
		{name: "native started", status: "running", started: true, want: "working"},
		{name: "host awaiting", status: "pending", host: SessionAwaiting, want: "waiting"},
		{name: "host taken", status: "pending", host: SessionAwaiting, taken: true, want: "working"},
		{name: "host decision", status: "pending", host: sessionWaitingDecision, taken: true, want: "decision"},
		{name: "session admission", status: "pending", host: SessionWaitingAdmission, taken: true, want: "resource"},
		{name: "disconnected", status: "pending", host: SessionDisconnected, taken: true, want: "stopped"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			a := &Attempt{ID: "a", ActivationID: "act", Status: c.status}
			if c.started {
				a.Started = observation
			}
			if c.host != "" {
				a.Session = &SessionHandoff{HostState: c.host}
				if c.taken {
					a.Session.Taken = observation
				}
			}
			r := Run{Status: "running", Activations: map[string]*Activation{"act": {ID: "act", StageID: "generic", Kind: "step", Status: "running"}}, Attempts: map[string]*Attempt{"a": a}}
			b := monitorBoard(r, local.MonitorRead{Cut: 17})
			if b.Category != c.want || b.Cut != 17 || len(b.Stages) != 1 || len(b.Stages[0].Attempts) != 1 {
				t.Fatalf("board %+v, want %s", b, c.want)
			}
		})
	}
}

func TestMonitorBoardTerminalAndUnknownAreExplicit(t *testing.T) {
	t.Parallel()
	partial := "partial"
	for _, c := range []struct {
		status   string
		outcome  *string
		category string
	}{
		{"completed", &partial, "terminal"}, {"failed", nil, "terminal"}, {"cancelled", nil, "terminal"}, {"uncertain", nil, "stopped"}, {"running", nil, "unknown"},
	} {
		b := monitorBoard(Run{Status: c.status, Outcome: c.outcome}, local.MonitorRead{})
		if b.Category != c.category || len(b.Reasons) == 0 {
			t.Fatalf("%s: %+v", c.status, b)
		}
	}
}

func TestMonitorBoardShowsEveryParallelBranchWhileDecisionWaits(t *testing.T) {
	t.Parallel()
	e, id := reviewFanOut(t, 2)
	if err := e.Drive(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, e, id)
	r.PendingDecision = &DecisionRequest{DecisionID: "review"}
	b := monitorBoard(r, local.MonitorRead{})
	branches := map[string]bool{}
	for _, stage := range b.Stages {
		for _, branch := range stage.Branches {
			branches[branch] = true
		}
	}
	if b.Category != "decision" || len(branches) != 2 {
		t.Fatalf("decision hid parallel branches: %+v", b)
	}
}

func TestMonitorBoardDoesNotInventAnActiveSignalFromAReadyWait(t *testing.T) {
	t.Parallel()
	r := Run{Status: "running", Activations: map[string]*Activation{"act": {ID: "act", StageID: "receive", Kind: "wait", Status: "ready"}}}
	b := monitorBoard(r, local.MonitorRead{})
	if b.Category != "ready" {
		t.Fatalf("unentered wait was labelled as signal waiting: %+v", b)
	}
	r.Activations["act"].Wait = &WaitProgress{RegistrationID: "reg"}
	r.Waits = map[string]*WaitRegistration{"reg": {ID: "reg", Status: "active"}}
	b = monitorBoard(r, local.MonitorRead{})
	if b.Category != "waiting" {
		t.Fatalf("active signal missing: %+v", b)
	}
}
