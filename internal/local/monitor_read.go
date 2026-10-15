package local

import "context"

// MonitorRead holds one authority cut, including admission facts which can
// change without a new Run revision. Nil Resources means unavailable on this
// storage version, rather than an observation of zero holders.
type MonitorRead struct {
	Snapshot  Snapshot
	Cut       int64
	Waiting   bool
	Resources map[string]int64
}

func (s *Store) ReadMonitor(ctx context.Context, runID string) (MonitorRead, error) {
	var view MonitorRead
	if !validIdentity(runID) {
		return view, ErrNotFound
	}
	conn, err := s.begin(ctx, false)
	if err != nil {
		return view, err
	}
	defer rollbackClose(conn)
	view.Cut, err = readCut(ctx, conn, -1)
	if err != nil {
		return view, err
	}
	view.Snapshot, err = snapshotAt(ctx, conn, runID, view.Cut, s.packedColumn("state_packed"))
	if err != nil {
		return view, err
	}
	if s.info.StorageVersion >= 4 {
		var count int
		if err = conn.QueryRowContext(ctx, "SELECT count(*) FROM slot_waiters WHERE run_id=?", runID).Scan(&count); err != nil {
			return view, err
		}
		view.Waiting = count > 0
	}
	if s.info.StorageVersion >= 7 {
		view.Resources = map[string]int64{}
		rows, err := conn.QueryContext(ctx, "SELECT resource,count(*) FROM slot_resources r JOIN slots s ON s.slot_id=r.slot_id GROUP BY resource")
		if err != nil {
			return view, err
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			var held int64
			if err = rows.Scan(&name, &held); err != nil {
				return view, err
			}
			view.Resources[name] = held
		}
		if err = rows.Err(); err != nil {
			return view, err
		}
	}
	return view, nil
}
