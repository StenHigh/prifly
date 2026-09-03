package local

import (
	"context"
	"database/sql"
	"math"
)

// StorageUsage describes allocated logical SQLite pages, including the current
// committed WAL view. It is NOT physical WAL bytes, filesystem usage or free
// disk space. No checkpoint, vacuum, retention or filesystem probe is performed.
type StorageUsage struct {
	PageCount      int64 `json:"page_count"`
	PageSize       int64 `json:"page_size"`
	FreePages      int64 `json:"free_pages"`
	AllocatedBytes int64 `json:"allocated_bytes"`
	SoftLimitBytes int64 `json:"soft_limit_bytes"`
}

func (s *Store) StorageUsage(ctx context.Context) (StorageUsage, error) {
	conn, err := s.begin(ctx, false)
	if err != nil {
		return StorageUsage{}, err
	}
	defer rollbackClose(conn)
	return storageUsage(ctx, conn, s.softLimitBytes)
}
func storageUsage(ctx context.Context, conn *sql.Conn, limit int64) (StorageUsage, error) {
	usage := StorageUsage{SoftLimitBytes: limit}
	if err := conn.QueryRowContext(ctx, "PRAGMA page_count").Scan(&usage.PageCount); err != nil {
		return usage, err
	}
	if err := conn.QueryRowContext(ctx, "PRAGMA page_size").Scan(&usage.PageSize); err != nil {
		return usage, err
	}
	if err := conn.QueryRowContext(ctx, "PRAGMA freelist_count").Scan(&usage.FreePages); err != nil {
		return usage, err
	}
	if usage.PageCount < 0 || usage.PageSize <= 0 || usage.PageCount > math.MaxInt64/usage.PageSize || usage.FreePages < 0 || usage.FreePages > usage.PageCount {
		return usage, ErrIntegrity
	}
	usage.AllocatedBytes = usage.PageCount * usage.PageSize
	return usage, nil
}
func storageBudgetRejection() *Rejection {
	return &Rejection{Code: "storage_budget_exhausted", Message: "optional/new work exceeds the SQLite logical-page budget; control and settlement keep priority"}
}
