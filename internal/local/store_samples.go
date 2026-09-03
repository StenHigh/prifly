package local

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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
	if len(batch) == 0 || len(batch) > 100 {
		return 0, errors.New("diagnostic batch must contain 1..100 records")
	}
	seen := make(map[string]bool, len(batch))
	for _, sample := range batch {
		if !validIdentity(sample.ID) || seen[sample.ID] || sample.RunID != "" && !validIdentity(sample.RunID) || len(sample.Data) > 64<<10 || !json.Valid(sample.Data) {
			return 0, errors.New("invalid diagnostic record")
		}
		seen[sample.ID] = true
	}
	conn, err := s.begin(ctx, true)
	if err != nil {
		return 0, err
	}
	defer rollbackClose(conn)
	var cut int64
	var count int
	if err := conn.QueryRowContext(ctx, "SELECT cut FROM authority WHERE singleton=1").Scan(&cut); err != nil {
		return 0, err
	}
	if err := conn.QueryRowContext(ctx, "SELECT count(*) FROM samples").Scan(&count); err != nil {
		return 0, err
	}
	newSamples := make([]SampleInput, 0, len(batch))
	for _, sample := range batch {
		var runID, digest string
		err := conn.QueryRowContext(ctx, "SELECT run_id,digest FROM samples WHERE id=?", sample.ID).Scan(&runID, &digest)
		if err == nil {
			if runID != sample.RunID || digest != digestBytes(sample.Data) {
				return 0, ErrCommandConflict
			}
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
		if sample.RunID != "" {
			var exists int
			if err := conn.QueryRowContext(ctx, "SELECT 1 FROM runs WHERE run_id=?", sample.RunID).Scan(&exists); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return 0, ErrNotFound
				}
				return 0, err
			}
		}
		newSamples = append(newSamples, sample)
	}
	if len(newSamples) == 0 {
		return cut, nil
	}
	if count+len(newSamples) > MaxSampleRecords {
		return 0, ErrSampleLimit
	}
	usage, err := storageUsage(ctx, conn, s.softLimitBytes)
	if err != nil {
		return 0, err
	}
	if usage.AllocatedBytes >= s.softLimitBytes {
		return 0, ErrSampleLimit
	}
	cut++
	for _, sample := range newSamples {
		if _, err := conn.ExecContext(ctx, "INSERT INTO samples(id,run_id,cut,data,digest) VALUES(?,?,?,?,?)", sample.ID, sample.RunID, cut, []byte(sample.Data), digestBytes(sample.Data)); err != nil {
			return 0, err
		}
	}
	usage, err = storageUsage(ctx, conn, s.softLimitBytes)
	if err != nil {
		return 0, err
	}
	if usage.AllocatedBytes > s.softLimitBytes {
		return 0, ErrSampleLimit
	}
	if _, err := conn.ExecContext(ctx, "UPDATE authority SET cut=? WHERE singleton=1", cut); err != nil {
		return 0, err
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return 0, err
	}
	return cut, nil
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
