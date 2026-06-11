package sqlitemeta

import (
	"context"
	"fmt"

	"github.com/michmich112/congee/internal/storage"
	"github.com/michmich112/congee/internal/storage/approxrows"
)

// AdminStorageSnapshot returns audit row count and on-disk bytes for the meta database.
func (s *Store) AdminStorageSnapshot(ctx context.Context) (storage.AdminStorageSnapshot, error) {
	var out storage.AdminStorageSnapshot
	out.MetaBytes = onDiskBytes(s.dbPath)
	var err error
	out.Audit, err = approxrows.SQLiteTable(ctx, s.db(), "audit_log")
	if err != nil {
		return storage.AdminStorageSnapshot{}, fmt.Errorf("sqlitemeta: admin snapshot audit count: %w", err)
	}
	return out, nil
}
