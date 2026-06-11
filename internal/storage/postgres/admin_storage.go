package postgres

import (
	"context"
	"fmt"

	"github.com/michmich112/congee/internal/storage"
)

// AdminStorageSnapshot implements storage.EventStore snapshot (events and tags only).
func (s *Store) AdminStorageSnapshot(ctx context.Context) (storage.AdminStorageSnapshot, error) {
	var out storage.AdminStorageSnapshot
	row := s.db.QueryRowContext(ctx, `
SELECT
  (SELECT COUNT(*)::bigint FROM events) AS ev,
  (SELECT COUNT(*)::bigint FROM event_tags) AS tg
`)
	if err := row.Scan(&out.Events, &out.Tags); err != nil {
		return storage.AdminStorageSnapshot{}, fmt.Errorf("postgres: admin snapshot counts: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT pg_database_size(current_database())`).Scan(&out.Bytes); err != nil {
		return storage.AdminStorageSnapshot{}, fmt.Errorf("postgres: pg_database_size: %w", err)
	}
	return out, nil
}
