package postgres

import (
	"context"
	"fmt"

	"github.com/michmich112/congee/internal/storage"
	"github.com/michmich112/congee/internal/storage/approxrows"
)

// AdminStorageSnapshot implements storage.EventStore snapshot (events and tags only).
func (s *Store) AdminStorageSnapshot(ctx context.Context) (storage.AdminStorageSnapshot, error) {
	var out storage.AdminStorageSnapshot
	var err error
	out.Events, err = approxrows.PostgresTable(ctx, s.db, "events")
	if err != nil {
		return storage.AdminStorageSnapshot{}, fmt.Errorf("postgres: admin snapshot events count: %w", err)
	}
	out.Tags, err = approxrows.PostgresTable(ctx, s.db, "event_tags")
	if err != nil {
		return storage.AdminStorageSnapshot{}, fmt.Errorf("postgres: admin snapshot tags count: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT pg_database_size(current_database())`).Scan(&out.Bytes); err != nil {
		return storage.AdminStorageSnapshot{}, fmt.Errorf("postgres: pg_database_size: %w", err)
	}
	return out, nil
}
