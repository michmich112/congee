package sqlitemeta

import (
	"context"
	"fmt"

	"github.com/michmich112/congee/internal/storage"
)

// AdminStorageSnapshot returns audit row count and on-disk bytes for the meta database.
func (s *Store) AdminStorageSnapshot(ctx context.Context) (storage.AdminStorageSnapshot, error) {
	var out storage.AdminStorageSnapshot
	out.MetaBytes = onDiskBytes(s.dbPath)
	row := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_log`)
	if err := row.Scan(&out.Audit); err != nil {
		return storage.AdminStorageSnapshot{}, fmt.Errorf("sqlitemeta: admin snapshot counts: %w", err)
	}
	return out, nil
}
