package local

import (
	"context"
	"errors"
)

// LightRead is one SQLite snapshot, including authority facts that can move
// without a new Run version. The callback receives bounded metadata batches;
// only changed snapshots are restored. No transaction escapes this method.
type LightRead struct {
	Cut       int64
	Queue     AuthoritySnapshot
	Revisions []Snapshot
	Changed   map[string]Snapshot
	Waiting   map[string]bool
	Resources map[string]int64
}

func (s *Store) ReadLight(ctx context.Context, queueKey string, versions map[string]int64) (LightRead, error) {
	return s.readLight(ctx, queueKey, versions, nil)
}

// StreamLight restores one changed snapshot at a time. The observer callback
// cannot retain a SQL connection and runs only in a read transaction.
func (s *Store) StreamLight(ctx context.Context, queueKey string, versions map[string]int64, observe func(Snapshot, int64) error) (LightRead, error) {
	return s.readLight(ctx, queueKey, versions, observe)
}

func (s *Store) readLight(ctx context.Context, queueKey string, versions map[string]int64, observe func(Snapshot, int64) error) (LightRead, error) {
	v := LightRead{Changed: map[string]Snapshot{}, Waiting: map[string]bool{}}
	conn, err := s.begin(ctx, false)
	if err != nil {
		return v, err
	}
	defer rollbackClose(conn)
	v.Cut, err = readCut(ctx, conn, -1)
	if err != nil {
		return v, err
	}
	if s.info.StorageVersion >= 4 {
		v.Queue, err = loadAuthoritySnapshot(ctx, conn, queueKey)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return v, err
		}
		rows, err := conn.QueryContext(ctx, "SELECT run_id FROM slot_waiters")
		if err != nil {
			return v, err
		}
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return v, err
			}
			v.Waiting[id] = true
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return v, err
		}
	}
	if s.info.StorageVersion >= 7 {
		v.Resources = map[string]int64{}
		rows, err := conn.QueryContext(ctx, "SELECT resource,count(*) FROM slot_resources r JOIN slots s ON s.slot_id=r.slot_id GROUP BY resource")
		if err != nil {
			return v, err
		}
		for rows.Next() {
			var name string
			var n int64
			if err = rows.Scan(&name, &n); err != nil {
				rows.Close()
				return v, err
			}
			v.Resources[name] = n
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return v, err
		}
	}
	after := ""
	for {
		rows, err := conn.QueryContext(ctx, "SELECT run_id,version,event_seq FROM runs WHERE run_id>? ORDER BY run_id LIMIT 200", after)
		if err != nil {
			return v, err
		}
		page := []Snapshot{}
		for rows.Next() {
			var r Snapshot
			if err = rows.Scan(&r.RunID, &r.Version, &r.EventSeq); err != nil {
				rows.Close()
				return v, err
			}
			page = append(page, r)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return v, err
		}
		for _, r := range page {
			v.Revisions = append(v.Revisions, r)
			if versions[r.RunID] != r.Version {
				// Older stores have no packed snapshot column. snapshotAt uses the
				// version-aware event column and keeps all reads on this transaction.
				snapshot, err := snapshotAt(ctx, conn, r.RunID, v.Cut, s.packedColumn("state_packed"))
				if err != nil {
					return v, err
				}
				if observe != nil {
					if err := observe(snapshot, v.Cut); err != nil {
						return v, err
					}
				} else {
					v.Changed[r.RunID] = snapshot
				}
			}
		}
		if len(page) < 200 {
			break
		}
		after = page[len(page)-1].RunID
	}
	return v, nil
}
