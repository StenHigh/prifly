package local

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// ProgramProgressVersion is the one line a program may write to fd 4 to say
// where it is. It is a diagnostic statement about its own attempt, never a
// result: fd 3 stays the only channel a StepResult arrives on.
const ProgramProgressVersion = "program-progress/1"

const (
	// MaxProgressLineBytes bounds one report; a longer line is refused whole.
	MaxProgressLineBytes = 1024
	// MaxProgressStreamBytes bounds everything read from fd 4 in one attempt.
	// Past it the channel is drained and discarded, so a chatty program is
	// never blocked on its own diagnostics.
	MaxProgressStreamBytes = 1 << 20
	// MaxProgressPhaseBytes and MaxProgressMessageRunes bound the text a report
	// carries into the authority and onto a monitor page.
	MaxProgressPhaseBytes   = 64
	MaxProgressMessageRunes = 200
	// ProgressDeliveryInterval is how often the latest report is handed on.
	// Reports in between are superseded, not lost: only the latest is kept.
	ProgressDeliveryInterval = time.Second
)

var progressPhase = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ProgramProgress is one validated report. Current and Total are absent when
// the program did not name them, never an invented zero.
type ProgramProgress struct {
	SchemaVersion string `json:"schema_version"`
	Phase         string `json:"phase"`
	Message       string `json:"message,omitempty"`
	Current       *int64 `json:"current,omitempty"`
	Total         *int64 `json:"total,omitempty"`
}

// ProgressDelivery is what the driver hands on: the latest valid report, when
// it arrived, and how many reports were accepted and refused so far. Refused
// reports are counted so a reader can tell the record is incomplete.
type ProgressDelivery struct {
	// LaunchID names the process that wrote the reports.
	LaunchID string
	Latest   *ProgramProgress
	At       time.Time
	Accepted int64
	Rejected int64
	// Truncated says fd 4 went past MaxProgressStreamBytes and the rest of it
	// was discarded unread.
	Truncated bool
}

// ParseProgramProgress validates one fd 4 line against program-progress/1. It
// refuses unknown fields, a phase that is not a short name, control text and a
// counter that contradicts itself; it never repairs a report.
func ParseProgramProgress(line []byte) (ProgramProgress, error) {
	var p ProgramProgress
	if len(line) > MaxProgressLineBytes {
		return p, errors.New("progress report longer than 1024 bytes")
	}
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&p); err != nil {
		return p, err
	}
	if decoder.More() {
		return p, errors.New("progress report carries more than one JSON value")
	}
	switch {
	case p.SchemaVersion != ProgramProgressVersion:
		return p, errors.New("progress report schema_version must be " + ProgramProgressVersion)
	case len(p.Phase) > MaxProgressPhaseBytes || !progressPhase.MatchString(p.Phase):
		return p, errors.New("progress phase must be a name of letters, digits, '.', '_' or '-', at most 64 bytes")
	case !utf8.ValidString(p.Message) || utf8.RuneCountInString(p.Message) > MaxProgressMessageRunes:
		return p, errors.New("progress message must be UTF-8 text of at most 200 characters")
	case p.Current != nil && *p.Current < 0:
		return p, errors.New("progress current must not be negative")
	case p.Total != nil && (p.Current == nil || *p.Total <= 0 || *p.Current > *p.Total):
		return p, errors.New("progress total needs a current between zero and itself")
	}
	for _, r := range p.Message {
		if unicode.IsControl(r) {
			return p, errors.New("progress message must not carry control characters")
		}
	}
	return p, nil
}

// AttemptProgress is the one record the authority keeps per program attempt:
// the latest report and its counters. It is not Run state: writing it moves no
// Run version, event or deadline.
type AttemptProgress struct {
	RunID     string          `json:"run_id"`
	AttemptID string          `json:"attempt_id"`
	LaunchID  string          `json:"launch_id"`
	Progress  ProgramProgress `json:"progress"`
	Observed  time.Time       `json:"observed"`
	Accepted  int64           `json:"accepted"`
	Rejected  int64           `json:"rejected"`
	Truncated bool            `json:"truncated"`
}

// ErrProgressUnavailable says this authority's storage predates progress.
var ErrProgressUnavailable = errors.New("storage predates program progress")

// PutAttemptProgress replaces the record of one attempt. A record written by a
// different launch of the same attempt id is replaced too: only the process the
// driver started last speaks for the attempt.
func (s *Store) PutAttemptProgress(ctx context.Context, p AttemptProgress) error {
	if s.info.StorageVersion < 8 {
		return ErrProgressUnavailable
	}
	if !validIdentity(p.RunID) || !validIdentity(p.AttemptID) || p.LaunchID == "" {
		return errors.New("progress record needs a run, an attempt and a launch")
	}
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, "INSERT INTO attempt_progress(attempt_id,run_id,data) VALUES(?,?,?) ON CONFLICT(attempt_id) DO UPDATE SET run_id=excluded.run_id,data=excluded.data", p.AttemptID, p.RunID, data)
	return err
}

// RunProgress reads every progress record of one Run, keyed by attempt.
func (s *Store) RunProgress(ctx context.Context, runID string) (map[string]AttemptProgress, error) {
	if s.info.StorageVersion < 8 {
		return nil, ErrProgressUnavailable
	}
	rows, err := s.db.QueryContext(ctx, "SELECT data FROM attempt_progress WHERE run_id=?", runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]AttemptProgress{}
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var p AttemptProgress
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, err
		}
		out[p.AttemptID] = p
	}
	return out, rows.Err()
}

// migrateAttemptProgress adds the progress records. No program could report
// before, so an upgraded installation starts with none.
func (s *Store) migrateAttemptProgress(ctx context.Context, conn *sql.Conn) error {
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	defer conn.ExecContext(context.Background(), "ROLLBACK")
	var current int
	if err := conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&current); err != nil {
		return err
	}
	if current >= 8 {
		return nil
	}
	if current != 7 {
		return ErrIncompatible
	}
	if _, err := conn.ExecContext(ctx, attemptProgressTable); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA user_version=8"); err != nil {
		return err
	}
	_, err := conn.ExecContext(ctx, "COMMIT")
	return err
}

const attemptProgressTable = "CREATE TABLE attempt_progress(attempt_id TEXT PRIMARY KEY,run_id TEXT NOT NULL,data BLOB NOT NULL)"

// progressCollector reads fd 4 on its own goroutine and keeps only the latest
// valid report, so a program that reports faster than the driver hands them on
// is never blocked and never grows memory.
type progressCollector struct {
	mu        sync.Mutex
	latest    *ProgramProgress
	at        time.Time
	accepted  int64
	rejected  int64
	truncated bool
	changed   bool
}

func (c *progressCollector) read(r io.Reader, done chan<- struct{}) {
	defer func() { done <- struct{}{} }()
	limited := &io.LimitedReader{R: r, N: MaxProgressStreamBytes}
	lines := bufio.NewReaderSize(limited, MaxProgressLineBytes+1)
	for {
		line, err := lines.ReadSlice('\n')
		tooLong := errors.Is(err, bufio.ErrBufferFull)
		for errors.Is(err, bufio.ErrBufferFull) {
			_, err = lines.ReadSlice('\n')
		}
		if trimmed := bytes.TrimSpace(line); tooLong || len(trimmed) != 0 {
			c.accept(trimmed, tooLong)
		}
		if err != nil {
			break
		}
	}
	if limited.N == 0 {
		c.mu.Lock()
		c.truncated, c.changed = true, true
		c.mu.Unlock()
		_, _ = io.Copy(io.Discard, r)
	}
}

func (c *progressCollector) accept(line []byte, tooLong bool) {
	var report ProgramProgress
	err := errors.New("progress report longer than 1024 bytes")
	if !tooLong {
		report, err = ParseProgramProgress(line)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.changed = true
	if err != nil {
		c.rejected++
		return
	}
	c.accepted++
	c.latest, c.at = &report, time.Now()
}

// retry marks the collector changed again after a delivery that failed, so
// the next tick hands on the latest state instead of waiting for a new report.
func (c *progressCollector) retry() {
	c.mu.Lock()
	c.changed = true
	c.mu.Unlock()
}

// take returns what changed since the last take, or false when nothing did.
func (c *progressCollector) take() (ProgressDelivery, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.changed {
		return ProgressDelivery{}, false
	}
	c.changed = false
	return ProgressDelivery{Latest: c.latest, At: c.at, Accepted: c.accepted, Rejected: c.rejected, Truncated: c.truncated}, true
}
