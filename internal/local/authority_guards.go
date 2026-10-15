package local

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
)

func orderedAuthorityGuards(guards []AuthorityGuard) ([]AuthorityGuard, error) {
	ordered := append([]AuthorityGuard(nil), guards...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Key < ordered[j].Key })
	for i, g := range ordered {
		if !validAuthorityKey(g.Key) || g.Check == nil || i > 0 && ordered[i-1].Key == g.Key {
			return nil, errors.New("invalid command authority guards")
		}
	}
	return ordered, nil
}

func checkAuthorityGuards(ctx context.Context, conn *sql.Conn, guards []AuthorityGuard) error {
	ordered, err := orderedAuthorityGuards(guards)
	if err != nil {
		return err
	}
	for _, g := range ordered {
		snapshot, err := loadAuthoritySnapshot(ctx, conn, g.Key)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		if err := applyAuthorityGuard(g.Check, snapshot); err != nil {
			return err
		}
	}
	return nil
}

// LookupAuthorityReceipt checks the original request before runtime reads
// current task ownership or executable files. Runtime checks read access first.
func (s *Store) LookupAuthorityReceipt(ctx context.Context, cmd AuthorityCommand) (AuthorityReceipt, error) {
	digest, err := authorityCommandDigest(cmd)
	if err != nil {
		return AuthorityReceipt{}, err
	}
	var data []byte
	var savedDigest, receiptDigest string
	err = s.db.QueryRowContext(ctx, "SELECT digest,receipt,receipt_digest FROM authority_commands WHERE actor=? AND command_id=?", cmd.Actor, cmd.ID).Scan(&savedDigest, &data, &receiptDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return AuthorityReceipt{}, ErrNotFound
	}
	if err != nil {
		return AuthorityReceipt{}, err
	}
	if savedDigest != digest {
		return AuthorityReceipt{}, ErrCommandConflict
	}
	var receipt AuthorityReceipt
	if digestBytes(data) != receiptDigest || json.Unmarshal(data, &receipt) != nil || receipt.ID != cmd.ID || receipt.Actor != cmd.Actor || receipt.Key != cmd.Key || receipt.Digest != digest {
		return AuthorityReceipt{}, ErrIntegrity
	}
	return receipt, nil
}

// ReadAuthorityRuns reads a ledger and its selected Runs at one SQLite cut.
// The selector is pure; missing Runs are represented by a zero version.
func (s *Store) ReadAuthorityRuns(ctx context.Context, key string, selectIDs func(AuthoritySnapshot) ([]string, error)) (AuthoritySnapshot, map[string]Snapshot, int64, error) {
	if !validAuthorityKey(key) || selectIDs == nil {
		return AuthoritySnapshot{}, nil, 0, errors.New("invalid authority read selector")
	}
	conn, err := s.begin(ctx, false)
	if err != nil {
		return AuthoritySnapshot{}, nil, 0, err
	}
	defer rollbackClose(conn)
	var cut int64
	if err := conn.QueryRowContext(ctx, "SELECT cut FROM authority WHERE singleton=1").Scan(&cut); err != nil {
		return AuthoritySnapshot{}, nil, 0, err
	}
	state, err := loadAuthoritySnapshot(ctx, conn, key)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return state, nil, cut, err
	}
	ids, err := authorityRunIDs(selectIDs, state)
	if err != nil {
		return state, nil, cut, err
	}
	runs := map[string]Snapshot{}
	for _, id := range ids {
		if !validIdentity(id) {
			return state, nil, cut, errors.New("invalid selected Run identity")
		}
		if _, ok := runs[id]; ok {
			continue
		}
		run, err := loadSnapshot(ctx, conn, id)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return state, nil, cut, err
		}
		runs[id] = run
	}
	return state, runs, cut, nil
}
