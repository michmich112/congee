package postgres

import (
	"context"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
)

var _ storage.MigrationSource = (*Store)(nil)

// MigrationRowCounts returns table row totals for migration verification.
func (s *Store) MigrationRowCounts(ctx context.Context) (storage.MigrationCounts, error) {
	ev, err := s.db.NewSelect().Model((*storage.EventRow)(nil)).Count(ctx)
	if err != nil {
		return storage.MigrationCounts{}, err
	}
	tags, err := s.db.NewSelect().Model((*storage.EventTagRow)(nil)).
		Where("event_id IN (SELECT id FROM events)").
		Count(ctx)
	if err != nil {
		return storage.MigrationCounts{}, err
	}
	return storage.MigrationCounts{
		Events: int64(ev),
		Tags:   int64(tags),
	}, nil
}

// ScanEventsForMigration iterates all events in stable order.
func (s *Store) ScanEventsForMigration(ctx context.Context, fn func(*nostr.Event) error) error {
	const page = 500
	var lastCA int64
	var lastID string
	var started bool
	for {
		var rows []storage.EventRow
		q := s.db.NewSelect().Model(&rows).Order("created_at ASC", "id ASC").Limit(page)
		if started {
			q = q.Where("(created_at > ?) OR (created_at = ? AND id > ?)", lastCA, lastCA, lastID)
		}
		if err := q.Scan(ctx); err != nil {
			return err
		}
		if len(rows) == 0 {
			break
		}
		for i := range rows {
			ev, err := s.rowToEvent(ctx, &rows[i])
			if err != nil {
				return err
			}
			if err := fn(ev); err != nil {
				return err
			}
		}
		last := rows[len(rows)-1]
		lastCA, lastID = last.CreatedAt, last.ID
		started = true
	}
	return nil
}
