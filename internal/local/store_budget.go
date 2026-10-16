package local

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

const StorageBudgetKey = "storage_budget"
const MinStorageBudgetBytes int64 = 64 << 10
const MaxStorageBudgetBytes int64 = 64 << 30

// AuthorityStorageBudget records the authority's finite logical-page ceiling.
// Generation zero is bootstrap provenance, never an invented owner decision.
type AuthorityStorageBudget struct {
	SchemaVersion           string `json:"schema_version"`
	AuthorityID             string `json:"authority_id"`
	Epoch                   int64  `json:"epoch"`
	Generation              int64  `json:"generation"`
	LimitBytes              int64  `json:"limit_bytes"`
	PreviousLimitBytes      int64  `json:"previous_limit_bytes"`
	Source                  string `json:"source"`
	Actor                   string `json:"actor"`
	Reason                  string `json:"reason"`
	CommandID               string `json:"command_id"`
	ObservedAllocatedBytes  int64  `json:"observed_allocated_bytes"`
	BootstrapStorageVersion int    `json:"bootstrap_storage_version"`
}

type StorageBudgetObservation struct {
	Control        AuthorityStorageBudget
	Usage          StorageUsage
	StorageVersion int
	Cut            int64
}

func validStorageLimit(n int64) bool { return n >= MinStorageBudgetBytes && n <= MaxStorageBudgetBytes }

func (s *Store) bootstrapStorageBudget(ctx context.Context, conn *sql.Conn, from int) error {
	var id string
	var epoch int64
	if err := conn.QueryRowContext(ctx, "SELECT id,epoch FROM authority WHERE singleton=1").Scan(&id, &epoch); err != nil {
		return err
	}
	source := "default"
	if from < StorageVersion {
		source = "legacy_default"
	}
	if s.explicitSoftLimit {
		source = "bootstrap"
	}
	c := AuthorityStorageBudget{SchemaVersion: "authority-storage-budget/1", AuthorityID: id, Epoch: epoch, LimitBytes: s.softLimitBytes, PreviousLimitBytes: s.softLimitBytes, Source: source, BootstrapStorageVersion: from}
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, "UPDATE authority SET storage_limit_bytes=?,storage_budget_data=?,storage_budget_digest=? WHERE singleton=1", c.LimitBytes, data, digestBytes(data))
	return err
}

func (s *Store) migrateStorageBudget(ctx context.Context, conn *sql.Conn) error {
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	defer conn.ExecContext(context.Background(), "ROLLBACK")
	var version int
	if err := conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version == StorageVersion {
		return nil
	}
	if version != 9 {
		return ErrIncompatible
	}
	if _, err := conn.ExecContext(ctx, `ALTER TABLE authority ADD COLUMN storage_limit_bytes INTEGER NOT NULL DEFAULT 536870912;
ALTER TABLE authority ADD COLUMN storage_budget_data BLOB;
ALTER TABLE authority ADD COLUMN storage_budget_digest TEXT;`); err != nil {
		return err
	}
	if err := s.bootstrapStorageBudget(ctx, conn, s.bootstrapVersion); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA user_version=10"); err != nil {
		return err
	}
	_, err := conn.ExecContext(ctx, "COMMIT")
	return err
}

func readStorageBudget(ctx context.Context, conn *sql.Conn, legacyLimit int64) (c AuthorityStorageBudget, version int, err error) {
	if err = conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return
	}
	var id string
	var epoch int64
	if err = conn.QueryRowContext(ctx, "SELECT id,epoch FROM authority WHERE singleton=1").Scan(&id, &epoch); err != nil {
		return
	}
	if version < 10 {
		source := "legacy_default"
		if legacyLimit != DefaultSoftLimitBytes {
			source = "bootstrap"
		}
		c = AuthorityStorageBudget{SchemaVersion: "authority-storage-budget/1", AuthorityID: id, Epoch: epoch, LimitBytes: legacyLimit, PreviousLimitBytes: legacyLimit, Source: source, BootstrapStorageVersion: version}
		return
	}
	var limit int64
	var data []byte
	var digest string
	if err = conn.QueryRowContext(ctx, "SELECT storage_limit_bytes,storage_budget_data,storage_budget_digest FROM authority WHERE singleton=1").Scan(&limit, &data, &digest); err != nil {
		return
	}
	if len(data) > 4096 || digestBytes(data) != digest {
		err = ErrIntegrity
		return
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&c) != nil || c.SchemaVersion != "authority-storage-budget/1" || c.AuthorityID != id || c.Epoch != epoch || c.Generation < 0 || c.LimitBytes != limit || !validStorageLimit(limit) || !validStorageLimit(c.PreviousLimitBytes) || c.BootstrapStorageVersion < 1 || c.BootstrapStorageVersion > StorageVersion {
		err = ErrIntegrity
		return
	}
	if c.Generation == 0 {
		if (c.Source != "default" && c.Source != "legacy_default" && c.Source != "bootstrap") || c.Actor != "" || c.CommandID != "" || c.Reason != "" || c.PreviousLimitBytes != limit {
			err = ErrIntegrity
			return
		}
		var count int
		if err = conn.QueryRowContext(ctx, "SELECT count(*) FROM authority_states WHERE state_key=?", StorageBudgetKey).Scan(&count); err == nil && count != 0 {
			err = ErrIntegrity
		}
		return
	}
	if c.Source != "owner" || c.Actor == "" || c.CommandID == "" || !ValidStorageBudgetReason(c.Reason) || c.PreviousLimitBytes >= limit || c.ObservedAllocatedBytes < 0 {
		err = ErrIntegrity
		return
	}
	var stateData []byte
	var stateVersion int64
	err = conn.QueryRowContext(ctx, "SELECT version,data FROM authority_states WHERE state_key=?", StorageBudgetKey).Scan(&stateVersion, &stateData)
	if errors.Is(err, sql.ErrNoRows) || err == nil && (stateVersion != c.Generation || !bytes.Equal(stateData, data)) {
		err = ErrIntegrity
	}
	return
}

func ValidStorageBudgetReason(reason string) bool {
	return utf8.ValidString(reason) && strings.TrimSpace(reason) != "" && len(reason) <= 1024
}

// StorageBudget reads configuration and allocation at the same SQLite read cut.
func (s *Store) StorageBudget(ctx context.Context) (out StorageBudgetObservation, err error) {
	conn, err := s.begin(ctx, false)
	if err != nil {
		return out, err
	}
	defer rollbackClose(conn)
	return s.storageBudgetObservation(ctx, conn)
}
func (s *Store) storageBudgetObservation(ctx context.Context, conn *sql.Conn) (out StorageBudgetObservation, err error) {
	out.Control, out.StorageVersion, err = readStorageBudget(ctx, conn, s.softLimitBytes)
	if err != nil {
		return
	}
	out.Usage, err = storageUsage(ctx, conn, out.Control.LimitBytes)
	if err != nil {
		return
	}
	err = conn.QueryRowContext(ctx, "SELECT cut FROM authority WHERE singleton=1").Scan(&out.Cut)
	return
}

func validateStorageBudgetChange(state AuthoritySnapshot, cmd AuthorityCommand, change AuthorityChange) error {
	if change.SetStorageBudget == nil {
		if cmd.Key == StorageBudgetKey {
			return errors.New("budget control must change its scalar and provenance atomically")
		}
		return nil
	}
	if cmd.Key != StorageBudgetKey || state.Budget == nil || change.SetCapacity != nil || len(change.PruneRuns) > 0 {
		return errors.New("budget setting requires its own authority control")
	}
	before := state.Budget.Control
	var after AuthorityStorageBudget
	if json.Unmarshal(change.Data, &after) != nil || after.SchemaVersion != "authority-storage-budget/1" || after.AuthorityID != before.AuthorityID || after.Epoch != before.Epoch || after.Generation != before.Generation+1 || after.PreviousLimitBytes != before.LimitBytes || after.LimitBytes != *change.SetStorageBudget || after.Source != "owner" || after.Actor != cmd.Actor || after.CommandID != cmd.ID || after.BootstrapStorageVersion != before.BootstrapStorageVersion || after.ObservedAllocatedBytes != state.Budget.Usage.AllocatedBytes {
		return ErrIntegrity
	}
	if !validStorageLimit(after.LimitBytes) || after.LimitBytes <= before.LimitBytes || !ValidStorageBudgetReason(after.Reason) {
		return Reject("storage_budget_invalid_limit", "choose a finite increase within 64 KiB..64 GiB and a nonempty bounded UTF-8 reason")
	}
	if after.LimitBytes <= state.Budget.Usage.AllocatedBytes {
		return Reject("storage_budget_target_exhausted", "the selected target leaves no headroom above current allocated logical pages; inspect storage budget show and prepare a sufficient finite target")
	}
	return nil
}
