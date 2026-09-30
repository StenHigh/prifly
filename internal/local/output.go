package local

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"
)

const (
	// OutputTailBytes bounds what is kept of one stream for a reader: the
	// newest bytes, never the whole output. Older bytes leave a gap that the
	// offsets show; they are not a transcript.
	OutputTailBytes = 64 << 10
	// OutputDeliveryInterval is how often new output is handed on.
	OutputDeliveryInterval = time.Second
)

// OutputDelivery is new output of one stream since the previous delivery:
// its bytes and the stream offset of the first one. A gap between two
// deliveries is output that did not fit the tail, not output that never was.
type OutputDelivery struct {
	LaunchID string
	Stream   string
	Offset   int64
	Data     []byte
	At       time.Time
}

// outputTail keeps the newest undelivered bytes of one stream. The reader
// goroutine writes into it; the driver loop takes from it. The program is
// never slowed by a reader of its output.
type outputTail struct {
	mu      sync.Mutex
	pending []byte
	start   int64
	end     int64
}

func (t *outputTail) write(offset int64, p []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.pending) == 0 {
		t.start = offset
	}
	t.pending = append(t.pending, p...)
	if over := len(t.pending) - OutputTailBytes; over > 0 {
		t.pending = append([]byte(nil), t.pending[over:]...)
		t.start += int64(over)
	}
	t.end = offset + int64(len(p))
}

func (t *outputTail) take() (offset int64, data []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.pending) == 0 {
		return 0, nil
	}
	offset, data = t.start, t.pending
	t.pending, t.start = nil, t.end
	return offset, data
}

// AttemptOutput is one stored chunk of a program's output.
type AttemptOutput struct {
	Stream   string    `json:"stream"`
	Offset   int64     `json:"offset"`
	Data     []byte    `json:"data"`
	Observed time.Time `json:"observed"`
	LaunchID string    `json:"launch_id"`
}

// PutAttemptOutput stores one chunk and forgets whatever of that stream lies
// wholly before the newest OutputTailBytes. It is not Run state.
func (s *Store) PutAttemptOutput(ctx context.Context, runID, attemptID string, chunk AttemptOutput) error {
	if s.info.StorageVersion < 9 {
		return ErrOutputUnavailable
	}
	if !validIdentity(runID) || !validIdentity(attemptID) || chunk.LaunchID == "" || chunk.Stream != "stdout" && chunk.Stream != "stderr" || chunk.Offset < 0 || len(chunk.Data) == 0 {
		return errors.New("output chunk needs a run, an attempt, a launch, a stream and bytes")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "INSERT OR REPLACE INTO attempt_output(attempt_id,stream,start_offset,run_id,launch_id,observed,data) VALUES(?,?,?,?,?,?,?)", attemptID, chunk.Stream, chunk.Offset, runID, chunk.LaunchID, chunk.Observed.UTC().Format(time.RFC3339Nano), chunk.Data); err != nil {
		return err
	}
	cutoff := chunk.Offset + int64(len(chunk.Data)) - OutputTailBytes
	if _, err := tx.ExecContext(ctx, "DELETE FROM attempt_output WHERE attempt_id=? AND stream=? AND start_offset+length(data)<=?", attemptID, chunk.Stream, cutoff); err != nil {
		return err
	}
	return tx.Commit()
}

// ReadAttemptOutput returns the stored chunks of one attempt, oldest first.
func (s *Store) ReadAttemptOutput(ctx context.Context, attemptID string) ([]AttemptOutput, error) {
	if s.info.StorageVersion < 9 {
		return nil, ErrOutputUnavailable
	}
	rows, err := s.db.QueryContext(ctx, "SELECT stream,start_offset,launch_id,observed,data FROM attempt_output WHERE attempt_id=? ORDER BY stream,start_offset", attemptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AttemptOutput{}
	for rows.Next() {
		var chunk AttemptOutput
		var observed string
		if err := rows.Scan(&chunk.Stream, &chunk.Offset, &chunk.LaunchID, &observed, &chunk.Data); err != nil {
			return nil, err
		}
		if chunk.Observed, err = time.Parse(time.RFC3339Nano, observed); err != nil {
			return nil, err
		}
		out = append(out, chunk)
	}
	return out, rows.Err()
}

// ErrOutputUnavailable says this authority's storage predates kept output.
var ErrOutputUnavailable = errors.New("storage predates program output")

const attemptOutputTable = "CREATE TABLE attempt_output(attempt_id TEXT NOT NULL,stream TEXT NOT NULL,start_offset INTEGER NOT NULL,run_id TEXT NOT NULL,launch_id TEXT NOT NULL,observed TEXT NOT NULL,data BLOB NOT NULL,PRIMARY KEY(attempt_id,stream,start_offset))"

// migrateAttemptOutput adds the kept output. No program's output was kept
// before, so an upgraded installation starts with none.
func (s *Store) migrateAttemptOutput(ctx context.Context, conn *sql.Conn) error {
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	defer conn.ExecContext(context.Background(), "ROLLBACK")
	var current int
	if err := conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&current); err != nil {
		return err
	}
	if current >= 9 {
		return nil
	}
	if current != 8 {
		return ErrIncompatible
	}
	if _, err := conn.ExecContext(ctx, attemptOutputTable); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA user_version=9"); err != nil {
		return err
	}
	_, err := conn.ExecContext(ctx, "COMMIT")
	return err
}
