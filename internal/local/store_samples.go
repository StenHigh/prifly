package local

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"
)

type SampleInput struct {
	ID    string          `json:"id"`
	RunID string          `json:"run_id"`
	Data  json.RawMessage `json:"data"`
}

type Sample struct {
	SampleInput
	Seq int64 `json:"seq"`
	Cut int64 `json:"cut"`
}

type SamplePage struct {
	Cut     int64    `json:"cut"`
	Records []Sample `json:"records"`
	More    bool     `json:"more"`
}

// AppendSamples persists a bounded diagnostic batch, never a run transition.
// Root-scoped records use an empty RunID. Stable IDs prevent repeat accounting;
// a conflicting repeat rejects the entire batch. Mandatory facts stay in events.
// F1 retains history: reaching the finite cap reports a gap to the caller rather
// than deleting data needed by old cuts or competing forever with control writes.
func (s *Store) AppendSamples(ctx context.Context, batch []SampleInput) (int64, error) {
	if s.info.ReadOnly {
		return 0, ErrReadOnly
	}
	if err := validSampleBatch(batch); err != nil {
		return 0, err
	}
	conn, err := s.begin(ctx, true)
	if err != nil {
		return 0, err
	}
	defer rollbackClose(conn)
	var cut int64
	if err := conn.QueryRowContext(ctx, "SELECT cut FROM authority WHERE singleton=1").Scan(&cut); err != nil {
		return 0, err
	}
	written, err := insertSamples(ctx, conn, s.softLimitBytes, cut+1, batch)
	if err != nil {
		return 0, err
	}
	if written == 0 {
		return cut, nil
	}
	cut++
	if _, err := conn.ExecContext(ctx, "UPDATE authority SET cut=? WHERE singleton=1", cut); err != nil {
		return 0, err
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return 0, err
	}
	return cut, nil
}

func validSampleBatch(batch []SampleInput) error {
	if len(batch) == 0 || len(batch) > 100 {
		return errors.New("diagnostic batch must contain 1..100 records")
	}
	seen := make(map[string]bool, len(batch))
	for _, sample := range batch {
		if !validIdentity(sample.ID) || seen[sample.ID] || sample.RunID != "" && !validIdentity(sample.RunID) || len(sample.Data) > 64<<10 || !json.Valid(sample.Data) {
			return errors.New("invalid diagnostic record")
		}
		seen[sample.ID] = true
	}
	return nil
}

// insertSamples writes a validated batch at one cut inside an open write
// transaction. It is shared with command application, which records its own
// telemetry without opening a second transaction. It returns how many records
// were actually written: an exact repeat is already recorded, and a batch that
// no longer fits the diagnostic allowance is refused with ErrSampleLimit.
func insertSamples(ctx context.Context, conn *sql.Conn, softLimitBytes, cut int64, batch []SampleInput) (int, error) {
	// samples.seq is a never-reused AUTOINCREMENT key and no sample is ever
	// deleted, so the highest key is the record count without a table scan.
	var highest sql.NullInt64
	if err := conn.QueryRowContext(ctx, "SELECT MAX(seq) FROM samples").Scan(&highest); err != nil {
		return 0, err
	}
	recorded, err := recordedSamples(ctx, conn, batch)
	if err != nil {
		return 0, err
	}
	newSamples := make([]SampleInput, 0, len(batch))
	runs := make([]string, 0, len(batch))
	for _, sample := range batch {
		if known, exists := recorded[sample.ID]; exists {
			if known.runID != sample.RunID || known.digest != digestBytes(sample.Data) {
				return 0, ErrCommandConflict
			}
			continue
		}
		if sample.RunID != "" && !slices.Contains(runs, sample.RunID) {
			runs = append(runs, sample.RunID)
		}
		newSamples = append(newSamples, sample)
	}
	if len(newSamples) == 0 {
		return 0, nil
	}
	if err := requireRuns(ctx, conn, runs); err != nil {
		return 0, err
	}
	if highest.Int64+int64(len(newSamples)) > MaxSampleRecords {
		return 0, ErrSampleLimit
	}
	usage, err := storageUsage(ctx, conn, softLimitBytes)
	if err != nil {
		return 0, err
	}
	if usage.AllocatedBytes >= softLimitBytes {
		return 0, ErrSampleLimit
	}
	insert, err := conn.PrepareContext(ctx, "INSERT INTO samples(id,run_id,cut,data,digest) VALUES(?,?,?,?,?)")
	if err != nil {
		return 0, err
	}
	defer insert.Close()
	for _, sample := range newSamples {
		if _, err := insert.ExecContext(ctx, sample.ID, sample.RunID, cut, []byte(sample.Data), digestBytes(sample.Data)); err != nil {
			return 0, err
		}
	}
	usage, err = storageUsage(ctx, conn, softLimitBytes)
	if err != nil {
		return 0, err
	}
	if usage.AllocatedBytes > softLimitBytes {
		return 0, ErrSampleLimit
	}
	return len(newSamples), nil
}

type recordedSample struct{ runID, digest string }

// recordedSamples reads every already recorded id of this batch in one query.
func recordedSamples(ctx context.Context, conn *sql.Conn, batch []SampleInput) (map[string]recordedSample, error) {
	arguments := make([]any, 0, len(batch))
	for _, sample := range batch {
		arguments = append(arguments, sample.ID)
	}
	rows, err := conn.QueryContext(ctx, "SELECT id,run_id,digest FROM samples WHERE id IN ("+placeholders(len(arguments))+")", arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	recorded := make(map[string]recordedSample, len(batch))
	for rows.Next() {
		var id, runID, digest string
		if err := rows.Scan(&id, &runID, &digest); err != nil {
			return nil, err
		}
		recorded[id] = recordedSample{runID, digest}
	}
	return recorded, rows.Err()
}

// requireRuns refuses a batch that names a Run this authority does not hold.
func requireRuns(ctx context.Context, conn *sql.Conn, runs []string) error {
	if len(runs) == 0 {
		return nil
	}
	arguments := make([]any, 0, len(runs))
	for _, id := range runs {
		arguments = append(arguments, id)
	}
	rows, err := conn.QueryContext(ctx, "SELECT run_id FROM runs WHERE run_id IN ("+placeholders(len(arguments))+")", arguments...)
	if err != nil {
		return err
	}
	defer rows.Close()
	found := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		found++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if found != len(runs) {
		return ErrNotFound
	}
	return nil
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func (s *Store) ReadSamples(ctx context.Context, cut, after int64, limit int) (SamplePage, error) {
	page := SamplePage{Records: []Sample{}}
	if after < 0 {
		return page, errors.New("invalid diagnostic cursor")
	}
	limit, err := readLimit(limit)
	if err != nil {
		return page, err
	}
	conn, err := s.begin(ctx, false)
	if err != nil {
		return page, err
	}
	defer rollbackClose(conn)
	page.Cut, err = readCut(ctx, conn, cut)
	if err != nil {
		return page, err
	}
	rows, err := conn.QueryContext(ctx, "SELECT seq,id,run_id,cut,data,digest FROM samples WHERE cut<=? AND seq>? ORDER BY seq LIMIT ?", page.Cut, after, limit+1)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var sample Sample
		var digest string
		if err := rows.Scan(&sample.Seq, &sample.ID, &sample.RunID, &sample.Cut, scanJSON{&sample.Data}, &digest); err != nil {
			return page, err
		}
		if digestBytes(sample.Data) != digest {
			return page, ErrIntegrity
		}
		if len(page.Records) == limit {
			page.More = true
			break
		}
		page.Records = append(page.Records, sample)
	}
	return page, rows.Err()
}
