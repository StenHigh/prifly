package local

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

const MaxScanBytes int64 = 64 << 20

// ScanLimitError reports the observed prefix, never an invented population total.
type ScanLimitError struct {
	Reason        string
	Records       int64
	RestoredBytes int64
	RecordLimit   int64
	ByteLimit     int64
}

func (e *ScanLimitError) Error() string {
	return fmt.Sprintf("bounded snapshot scan (%s) observed at least %d records / %d restored bytes; limits %d records / %d bytes", e.Reason, e.Records, e.RestoredBytes, e.RecordLimit, e.ByteLimit)
}

// SnapshotScan describes one SQLite read snapshot. Complete is true only at EOF.
type SnapshotScan struct {
	Cut           int64
	Records       int64
	RestoredBytes int64
	Complete      bool
}

// ScanSnapshots visits the complete current population with one snapshot in
// memory. The callback must not retain snapshots or mutate this authority.
func (s *Store) ScanSnapshots(ctx context.Context, visit func(Snapshot) error) (scan SnapshotScan, err error) {
	conn, err := s.begin(ctx, false)
	if err != nil {
		return scan, err
	}
	defer rollbackClose(conn)
	scan.Cut, err = currentCut(ctx, conn)
	if err != nil {
		return scan, err
	}
	rows, err := conn.QueryContext(ctx, `SELECT run_id,version,event_seq,CASE WHEN length(snapshot)<=? THEN snapshot ELSE NULL END,snapshot_digest,`+s.packedColumn("snapshot_packed")+` FROM runs ORDER BY run_id`, MaxSnapshotBytes)
	if err != nil {
		return scan, err
	}
	defer rows.Close()
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return scan, err
		}
		var snapshot Snapshot
		var digest string
		var packed bool
		if err := rows.Scan(&snapshot.RunID, &snapshot.Version, &snapshot.EventSeq, scanJSON{&snapshot.Data}, &digest, &packed); err != nil {
			return scan, err
		}
		if len(snapshot.Data) == 0 {
			return scan, ErrIntegrity
		}
		if packed {
			snapshot.Data, err = unpackPinnedBytesBounded(ctx, conn, snapshot.Data, MaxSnapshotBytes)
			if err != nil {
				return scan, err
			}
		}
		if len(snapshot.Data) > MaxSnapshotBytes || !json.Valid(snapshot.Data) || digestBytes(snapshot.Data) != digest {
			return scan, ErrIntegrity
		}
		scan.Records++
		scan.RestoredBytes += int64(len(snapshot.Data))
		if visit != nil {
			if err := visit(snapshot); err != nil {
				return scan, err
			}
		}
	}
	if err := rows.Err(); err != nil {
		return scan, err
	}
	if err := ctx.Err(); err != nil {
		return scan, err
	}
	scan.Complete = true
	return scan, nil
}

func (s *Store) checkExpectedCut(ctx context.Context, conn *sql.Conn, expected *int64) (*Rejection, error) {
	if expected == nil {
		return nil, nil
	}
	if *expected < 0 || *expected > (1<<53)-1 {
		return nil, fmt.Errorf("invalid expected read cut")
	}
	cut, err := currentCut(ctx, conn)
	if err != nil {
		return nil, err
	}
	if cut != *expected {
		return &Rejection{Code: "claim_ownership_conflict", Message: "authority evidence changed after ownership was checked; inspect and prepare the claim operation again"}, nil
	}
	return nil, nil
}
