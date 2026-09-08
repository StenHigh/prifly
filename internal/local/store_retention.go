package local

import (
	"context"
	"encoding/json"
)

// VisitRetentionRoots streams the records that survive Run pruning. Receipts
// remain roots: forgetting a retry's result could repeat an external effect.
func (s *Store) VisitRetentionRoots(ctx context.Context, omitted map[string]int64, visit func([]byte) error) error {
	conn, err := s.begin(ctx, false)
	if err != nil {
		return err
	}
	defer rollbackClose(conn)
	queries := []string{
		"SELECT run_id,snapshot,snapshot_packed,snapshot_digest FROM runs",
		"SELECT run_id,state_after,state_packed,state_digest FROM events WHERE state_after IS NOT NULL",
		"SELECT run_id,data,0,digest FROM events",
		"SELECT '',receipt,0,receipt_digest FROM commands",
		"SELECT '',data,0,digest FROM authority_states",
		"SELECT '',receipt,0,receipt_digest FROM authority_commands",
		"SELECT run_id,data,0,digest FROM samples",
	}
	for _, query := range queries {
		rows, err := conn.QueryContext(ctx, query)
		if err != nil {
			return err
		}
		for rows.Next() {
			var run, digest string
			var data json.RawMessage
			var packed bool
			if err = rows.Scan(&run, scanJSON{&data}, &packed, &digest); err != nil {
				rows.Close()
				return err
			}
			if _, skip := omitted[run]; skip {
				continue
			}
			data, err = restore(ctx, conn, data, packed, digest)
			if err == nil {
				err = visit(data)
			}
			if err != nil {
				rows.Close()
				return err
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// Compact runs after history commit. Failure cannot roll back that deletion;
// callers must report it and may retry the compaction independently.
func (s *Store) Compact(ctx context.Context) error {
	if s.info.ReadOnly {
		return ErrReadOnly
	}
	// Packed fragments are immutable and never nest another packed marker.
	_, err := s.db.ExecContext(ctx, `DELETE FROM pinned_bytes WHERE digest NOT IN (
 SELECT j.value FROM runs,json_tree(runs.snapshot) j WHERE runs.snapshot_packed=1 AND j.key='$pinned'
 UNION SELECT j.value FROM events,json_tree(events.state_after) j WHERE events.state_packed=1 AND j.key='$pinned')`)
	if err != nil {
		return err
	}
	if _, err = s.db.ExecContext(ctx, "VACUUM"); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
	return err
}

// Only authority-owned audit rows may authorize recovery of a workspace whose
// Run history was already pruned. Arbitrary event or artifact JSON may not.
func (s *Store) VisitRetentionAudit(ctx context.Context, visit func([]byte) error) error {
	rows, err := s.db.QueryContext(ctx, "SELECT data,digest FROM authority_states WHERE state_key LIKE 'retention:%'")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var data []byte
		var digest string
		if err = rows.Scan(&data, &digest); err != nil {
			return err
		}
		if digestBytes(data) != digest {
			return ErrIntegrity
		}
		if err = visit(data); err != nil {
			return err
		}
	}
	return rows.Err()
}
