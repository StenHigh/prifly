package local

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

func (s *Store) Read(ctx context.Context, runID string, after int64, limit int) (ReadView, error) {
	return s.ReadAt(ctx, runID, -1, after, limit)
}

// ReadAt uses one SQLite read transaction. cut=-1 selects the current cut;
// cut=0 is the actual empty authority, not an alias for the latest state.
// Reading history never refreshes meters, runs a checkpoint or changes state.
func (s *Store) ReadAt(ctx context.Context, runID string, cut, after int64, limit int) (ReadView, error) {
	view := ReadView{Events: []Event{}}
	if !validIdentity(runID) || after < 0 {
		return view, errors.New("invalid read scope or cursor")
	}
	limit, err := readLimit(limit)
	if err != nil {
		return view, err
	}
	conn, err := s.begin(ctx, false)
	if err != nil {
		return view, err
	}
	defer rollbackClose(conn)
	view.Cut, err = readCut(ctx, conn, cut)
	if err != nil {
		return view, err
	}
	view.Snapshot, err = snapshotAt(ctx, conn, runID, view.Cut)
	if err != nil {
		return view, err
	}
	rows, err := conn.QueryContext(ctx, `SELECT seq,run_version,cut,type,schema_version,actor,command_id,data,digest,state_after FROM events WHERE run_id=? AND seq>? AND seq<=? ORDER BY seq LIMIT ?`, runID, after, view.Snapshot.EventSeq, limit+1)
	if err != nil {
		return view, err
	}
	defer rows.Close()
	bytes := len(view.Snapshot.Data)
	for rows.Next() {
		var e Event
		e.RunID = runID
		if err := rows.Scan(&e.Seq, &e.RunVersion, &e.Cut, &e.Type, &e.Version, &e.Actor, &e.CommandID, scanJSON{&e.Data}, &e.Digest, scanJSON{&e.StateAfter}); err != nil {
			return view, err
		}
		if e.Version != EventVersion || !s.eventTypes[e.Type] {
			return view, ErrIncompatible
		}
		if digestBytes(e.Data) != e.Digest {
			return view, ErrIntegrity
		}
		bytes += len(e.Data) + len(e.StateAfter)
		if len(view.Events) == limit || bytes > 64<<20 {
			view.More = true
			break
		}
		view.Events = append(view.Events, e)
	}
	return view, rows.Err()
}

func (s *Store) ReadAll(ctx context.Context, limit int) ([]Snapshot, int64, error) {
	return s.ReadAllAt(ctx, -1, limit)
}

// ReadAllAt fails instead of silently truncating a population used in telemetry.
// F1 keeps complete history; the cut is also valid for a later ReadSamples call.
func (s *Store) ReadAllAt(ctx context.Context, cut int64, limit int) ([]Snapshot, int64, error) {
	limit, err := readLimit(limit)
	if err != nil {
		return nil, 0, err
	}
	conn, err := s.begin(ctx, false)
	if err != nil {
		return nil, 0, err
	}
	defer rollbackClose(conn)
	cut, err = readCut(ctx, conn, cut)
	if err != nil {
		return nil, 0, err
	}
	rows, err := conn.QueryContext(ctx, `SELECT e.run_id,e.run_version,e.seq,e.state_after,e.state_digest
FROM events e WHERE e.state_after IS NOT NULL AND e.cut<=? AND NOT EXISTS
(SELECT 1 FROM events n WHERE n.run_id=e.run_id AND n.state_after IS NOT NULL AND n.seq>e.seq AND n.cut<=?)
ORDER BY e.run_id LIMIT ?`, cut, cut, limit+1)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := []Snapshot{}
	bytes := 0
	for rows.Next() {
		var st Snapshot
		var digest string
		if err := rows.Scan(&st.RunID, &st.Version, &st.EventSeq, scanJSON{&st.Data}, &digest); err != nil {
			return nil, 0, err
		}
		if digestBytes(st.Data) != digest {
			return nil, 0, ErrIntegrity
		}
		bytes += len(st.Data)
		if len(result) == limit || bytes > 64<<20 {
			return nil, 0, errors.New("authority population exceeds the bounded F1 scan (1000 records or 64 MiB)")
		}
		result = append(result, st)
	}
	return result, cut, rows.Err()
}

// LookupReceipt supports recovery when COMMIT succeeded but its response was
// lost. The caller still performs current read authorization for the target.
func (s *Store) LookupReceipt(ctx context.Context, actor, id string) (Receipt, error) {
	var receipt Receipt
	if !validIdentity(actor) || !validIdentity(id) {
		return receipt, errors.New("invalid receipt scope")
	}
	var data []byte
	var digest string
	err := s.db.QueryRowContext(ctx, "SELECT receipt,receipt_digest FROM commands WHERE actor=? AND command_id=?", actor, id).Scan(&data, &digest)
	if errors.Is(err, sql.ErrNoRows) {
		return receipt, ErrNotFound
	}
	if err != nil {
		return receipt, err
	}
	if digestBytes(data) != digest || json.Unmarshal(data, &receipt) != nil {
		return receipt, ErrIntegrity
	}
	return receipt, nil
}

// ReadReceiptsAt provides the complete bounded command population, including
// persisted rejections. Transport duplicates have no additional receipt row.
// Runtime checks current owner access before exposing any receipt contents.
func (s *Store) ReadReceiptsAt(ctx context.Context, cut int64, limit int) ([]Receipt, int64, error) {
	if limit == 0 {
		limit = 1000
	}
	if limit < 1 || limit > MaxReceiptRecords {
		return nil, 0, fmt.Errorf("receipt scan limit must be within 1..%d", MaxReceiptRecords)
	}
	conn, err := s.begin(ctx, false)
	if err != nil {
		return nil, 0, err
	}
	defer rollbackClose(conn)
	cut, err = readCut(ctx, conn, cut)
	if err != nil {
		return nil, 0, err
	}
	rows, err := conn.QueryContext(ctx, "SELECT receipt,receipt_digest FROM commands WHERE cut<=? ORDER BY cut LIMIT ?", cut, limit+1)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	receipts := []Receipt{}
	bytes := 0
	for rows.Next() {
		var data []byte
		var digest string
		var receipt Receipt
		if err := rows.Scan(&data, &digest); err != nil {
			return nil, 0, err
		}
		if digestBytes(data) != digest || json.Unmarshal(data, &receipt) != nil {
			return nil, 0, ErrIntegrity
		}
		bytes += len(data)
		if len(receipts) == limit || bytes > 64<<20 {
			return nil, 0, errors.New("receipt population exceeds requested complete scan limit or 64 MiB (maximum 100000 records)")
		}
		receipts = append(receipts, receipt)
	}
	return receipts, cut, rows.Err()
}

// Slot exposes ownership as a read, not as permission to dispatch. Acquisition
// and release always take place atomically inside Apply. It reports the single
// held slot when exactly one is held, so a one-slot installation reads exactly
// as before; Slots reports the whole set.
func (s *Store) Slot(ctx context.Context) (id, runID string, err error) {
	held, err := s.Slots(ctx)
	if err != nil || len(held) != 1 {
		return "", "", err
	}
	for slot, run := range held {
		return slot, run, nil
	}
	return "", "", nil
}

// Slots reports every held admission slot with its owning run.
func (s *Store) Slots(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT slot_id,run_id FROM slots")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	held := map[string]string{}
	for rows.Next() {
		var slot, run string
		if err := rows.Scan(&slot, &run); err != nil {
			return nil, err
		}
		held[slot] = run
	}
	return held, rows.Err()
}

// SlotWaiters reports the admission queue: which runs are waiting and since
// which cut. The order this map implies is the declared policy's order.
func (s *Store) SlotWaiters(ctx context.Context) (map[string]int64, error) {
	if s.info.StorageVersion < 4 {
		return map[string]int64{}, nil
	}
	rows, err := s.db.QueryContext(ctx, "SELECT run_id,since_seq FROM slot_waiters ORDER BY since_seq,run_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	waiting := map[string]int64{}
	for rows.Next() {
		var runID string
		var since int64
		if err := rows.Scan(&runID, &since); err != nil {
			return nil, err
		}
		waiting[runID] = since
	}
	return waiting, rows.Err()
}

// SlotCapacity reports how many attempts this authority admits at once.
func (s *Store) SlotCapacity(ctx context.Context) (int64, error) {
	var capacity int64
	err := s.db.QueryRowContext(ctx, "SELECT slot_capacity FROM authority WHERE singleton=1").Scan(&capacity)
	return capacity, err
}

func readLimit(limit int) (int, error) {
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > MaxReadRecords {
		return 0, fmt.Errorf("read limit must be between 1 and %d", MaxReadRecords)
	}
	return limit, nil
}

func readCut(ctx context.Context, conn *sql.Conn, requested int64) (int64, error) {
	var current int64
	if err := conn.QueryRowContext(ctx, "SELECT cut FROM authority WHERE singleton=1").Scan(&current); err != nil {
		return 0, err
	}
	if requested < -1 || requested > current {
		return 0, errors.New("cut lies outside stored history")
	}
	if requested == -1 {
		requested = current
	}
	return requested, nil
}

func snapshotAt(ctx context.Context, conn *sql.Conn, runID string, cut int64) (Snapshot, error) {
	state := Snapshot{RunID: runID}
	var digest string
	err := conn.QueryRowContext(ctx, "SELECT run_version,seq,state_after,state_digest FROM events WHERE run_id=? AND cut<=? AND state_after IS NOT NULL ORDER BY seq DESC LIMIT 1", runID, cut).Scan(&state.Version, &state.EventSeq, scanJSON{&state.Data}, &digest)
	if errors.Is(err, sql.ErrNoRows) {
		return state, ErrNotFound
	}
	if err != nil {
		return state, err
	}
	if digestBytes(state.Data) != digest {
		return state, ErrIntegrity
	}
	return state, nil
}

// Verify checks the materialized projection against the journal without replaying
// a process or consulting mutable workflow files. Unknown events fail closed.
func (s *Store) Verify(ctx context.Context) error {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	return s.verify(ctx, conn)
}

func (s *Store) verify(ctx context.Context, conn *sql.Conn) error {
	if _, err := conn.ExecContext(ctx, "BEGIN"); err != nil {
		return err
	}
	defer conn.ExecContext(context.Background(), "ROLLBACK")
	if err := storageHeader(ctx, conn); err != nil {
		return err
	}
	var check string
	if err := conn.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&check); err != nil {
		return err
	}
	if check != "ok" {
		return ErrIntegrity
	}
	fkRows, err := conn.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	failed := fkRows.Next()
	err = fkRows.Err()
	_ = fkRows.Close()
	if err != nil {
		return err
	}
	if failed {
		return ErrIntegrity
	}
	last := make(map[string]Snapshot)
	lastCut := make(map[string]int64)
	lastSeq := make(map[string]int64)
	rows, err := conn.QueryContext(ctx, "SELECT run_id,seq,run_version,cut,type,schema_version,data,digest,state_after,state_digest FROM events ORDER BY run_id,seq")
	if err != nil {
		return err
	}
	for rows.Next() {
		var e Event
		var stateDigest sql.NullString
		if err := rows.Scan(&e.RunID, &e.Seq, &e.RunVersion, &e.Cut, &e.Type, &e.Version, scanJSON{&e.Data}, &e.Digest, scanJSON{&e.StateAfter}, &stateDigest); err != nil {
			_ = rows.Close()
			return err
		}
		if !s.eventTypes[e.Type] || e.Version != EventVersion {
			_ = rows.Close()
			return fmt.Errorf("%w: %s/%d", ErrIncompatible, e.Type, e.Version)
		}
		if e.Seq != lastSeq[e.RunID]+1 || e.Cut < lastCut[e.RunID] || !json.Valid(e.Data) || digestBytes(e.Data) != e.Digest {
			_ = rows.Close()
			return ErrIntegrity
		}
		if e.Cut > lastCut[e.RunID] && lastSeq[e.RunID] != last[e.RunID].EventSeq {
			_ = rows.Close()
			return ErrIntegrity
		}
		if e.StateAfter != nil {
			if !stateDigest.Valid || !json.Valid(e.StateAfter) || digestBytes(e.StateAfter) != stateDigest.String {
				_ = rows.Close()
				return ErrIntegrity
			}
			previous := last[e.RunID]
			if e.RunVersion < previous.Version || e.RunVersion > previous.Version+1 || (previous.Version == 0 && e.RunVersion != 1) {
				_ = rows.Close()
				return ErrIntegrity
			}
			last[e.RunID] = Snapshot{RunID: e.RunID, Version: e.RunVersion, EventSeq: e.Seq, Data: e.StateAfter}
		}
		lastSeq[e.RunID], lastCut[e.RunID] = e.Seq, e.Cut
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	for runID, seq := range lastSeq {
		if last[runID].EventSeq != seq {
			return ErrIntegrity
		}
	}
	rows, err = conn.QueryContext(ctx, "SELECT run_id,version,event_seq,snapshot,snapshot_digest FROM runs")
	if err != nil {
		return err
	}
	for rows.Next() {
		var st Snapshot
		var digest string
		if err := rows.Scan(&st.RunID, &st.Version, &st.EventSeq, scanJSON{&st.Data}, &digest); err != nil {
			_ = rows.Close()
			return err
		}
		e, ok := last[st.RunID]
		if !ok || st.Version != e.Version || st.EventSeq != e.EventSeq || digestBytes(st.Data) != digest || string(st.Data) != string(e.Data) {
			_ = rows.Close()
			return ErrIntegrity
		}
		delete(last, st.RunID)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	if len(last) != 0 {
		return ErrIntegrity
	}
	rows, err = conn.QueryContext(ctx, "SELECT actor,command_id,run_id,digest,cut,receipt,receipt_digest FROM commands")
	if err != nil {
		return err
	}
	for rows.Next() {
		var actor, id, runID, digest, bodyDigest string
		var cut int64
		var data []byte
		var receipt Receipt
		if err := rows.Scan(&actor, &id, &runID, &digest, &cut, &data, &bodyDigest); err != nil {
			_ = rows.Close()
			return err
		}
		if digestBytes(data) != bodyDigest || json.Unmarshal(data, &receipt) != nil || receipt.ID != id || receipt.Actor != actor || receipt.RunID != runID || receipt.Cut != cut || receipt.Digest != digest {
			_ = rows.Close()
			return ErrIntegrity
		}
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	if s.info.StorageVersion >= 2 {
		rows, err = conn.QueryContext(ctx, "SELECT state_key,version,cut,data,digest FROM authority_states")
		if err != nil {
			return err
		}
		for rows.Next() {
			var key, digest string
			var version, cut int64
			var data []byte
			if err := rows.Scan(&key, &version, &cut, &data, &digest); err != nil {
				_ = rows.Close()
				return err
			}
			if !validIdentity(key) || version < 1 || cut < 1 || !json.Valid(data) || digestBytes(data) != digest {
				_ = rows.Close()
				return ErrIntegrity
			}
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return err
		}
		rows, err = conn.QueryContext(ctx, "SELECT actor,command_id,state_key,digest,cut,receipt,receipt_digest FROM authority_commands")
		if err != nil {
			return err
		}
		for rows.Next() {
			var actor, id, key, digest, bodyDigest string
			var cut int64
			var data []byte
			var receipt AuthorityReceipt
			if err := rows.Scan(&actor, &id, &key, &digest, &cut, &data, &bodyDigest); err != nil {
				_ = rows.Close()
				return err
			}
			if digestBytes(data) != bodyDigest || json.Unmarshal(data, &receipt) != nil || receipt.ID != id || receipt.Actor != actor || receipt.Key != key || receipt.Cut != cut || receipt.Digest != digest {
				_ = rows.Close()
				return ErrIntegrity
			}
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return err
		}
	}
	rows, err = conn.QueryContext(ctx, "SELECT data,digest FROM samples")
	if err != nil {
		return err
	}
	for rows.Next() {
		var data []byte
		var digest string
		if err := rows.Scan(&data, &digest); err != nil {
			_ = rows.Close()
			return err
		}
		if !json.Valid(data) || digestBytes(data) != digest {
			_ = rows.Close()
			return ErrIntegrity
		}
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	var cut, maxCut, capacity int64
	if err := conn.QueryRowContext(ctx, "SELECT cut,slot_capacity FROM authority WHERE singleton=1").Scan(&cut, &capacity); err != nil {
		return err
	}
	maxCutSQL := "SELECT COALESCE(MAX(cut),0) FROM (SELECT cut FROM commands UNION ALL SELECT cut FROM samples)"
	if s.info.StorageVersion >= 2 {
		maxCutSQL = "SELECT COALESCE(MAX(cut),0) FROM (SELECT cut FROM commands UNION ALL SELECT cut FROM samples UNION ALL SELECT cut FROM authority_commands)"
	}
	if err := conn.QueryRowContext(ctx, maxCutSQL).Scan(&maxCut); err != nil {
		return err
	}
	if cut != maxCut {
		return ErrIntegrity
	}
	// Every held slot belongs to a run that exists, and the set never exceeds
	// the recorded capacity: an over-limit admission would otherwise be a
	// storage fact nobody noticed.
	slotRows, err := conn.QueryContext(ctx, "SELECT slot_id,run_id FROM slots")
	if err != nil {
		return err
	}
	held := int64(0)
	owners := []string{}
	for slotRows.Next() {
		var slot, owner string
		if err := slotRows.Scan(&slot, &owner); err != nil {
			_ = slotRows.Close()
			return err
		}
		if !validIdentity(slot) || !validIdentity(owner) {
			_ = slotRows.Close()
			return ErrIntegrity
		}
		held++
		owners = append(owners, owner)
	}
	err = slotRows.Err()
	_ = slotRows.Close()
	if err != nil {
		return err
	}
	if capacity < 1 || held > capacity {
		return ErrIntegrity
	}
	// The admission queue is bounded too: an unbounded queue would be a second
	// unbounded resource hiding behind a bounded one.
	if s.info.StorageVersion >= 4 {
		var waiting int64
		if err := conn.QueryRowContext(ctx, "SELECT count(*) FROM slot_waiters").Scan(&waiting); err != nil {
			return err
		}
		if waiting > MaxSlotWaiters {
			return ErrIntegrity
		}
	}
	for _, owner := range owners {
		if _, err := loadSnapshot(ctx, conn, owner); err != nil {
			return ErrIntegrity
		}
	}
	return nil
}
