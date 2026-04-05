package sqlite

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
	tags, err := s.db.NewSelect().Model((*storage.EventTagRow)(nil)).Count(ctx)
	if err != nil {
		return storage.MigrationCounts{}, err
	}
	audit, err := s.db.NewSelect().Model((*storage.AuditLogRow)(nil)).Count(ctx)
	if err != nil {
		return storage.MigrationCounts{}, err
	}
	ch, err := s.db.NewSelect().Model((*storage.ConfigChangelogRow)(nil)).Count(ctx)
	if err != nil {
		return storage.MigrationCounts{}, err
	}
	return storage.MigrationCounts{
		Events:    int64(ev),
		Tags:      int64(tags),
		Audit:     int64(audit),
		Changelog: int64(ch),
	}, nil
}

// ScanEventsForMigration iterates all events in stable order.
func (s *Store) ScanEventsForMigration(ctx context.Context, fn func(ev *nostr.Event) error) error {
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

// ScanAuditForMigration iterates audit rows in primary key order.
func (s *Store) ScanAuditForMigration(ctx context.Context, fn func(storage.AuditEntry) error) error {
	const page = 500
	var lastID int64
	for {
		var rows []storage.AuditLogRow
		q := s.db.NewSelect().Model(&rows).Order("id ASC").Limit(page)
		if lastID > 0 {
			q = q.Where("id > ?", lastID)
		}
		if err := q.Scan(ctx); err != nil {
			return err
		}
		if len(rows) == 0 {
			break
		}
		for i := range rows {
			if err := fn(storage.AuditEntry{
				CreatedAt: rows[i].CreatedAt,
				Action:    rows[i].Action,
				Detail:    rows[i].Detail,
				Pubkey:    rows[i].Pubkey,
			}); err != nil {
				return err
			}
		}
		lastID = rows[len(rows)-1].ID
	}
	return nil
}

// ScanChangelogForMigration iterates config changelog rows in primary key order.
func (s *Store) ScanChangelogForMigration(ctx context.Context, fn func(storage.ConfigChange) error) error {
	const page = 500
	var lastID int64
	for {
		var rows []storage.ConfigChangelogRow
		q := s.db.NewSelect().Model(&rows).Order("id ASC").Limit(page)
		if lastID > 0 {
			q = q.Where("id > ?", lastID)
		}
		if err := q.Scan(ctx); err != nil {
			return err
		}
		if len(rows) == 0 {
			break
		}
		for i := range rows {
			if err := fn(storage.ConfigChange{
				CreatedAt: rows[i].CreatedAt,
				Summary:   rows[i].Summary,
				JSONDiff:  rows[i].JSONDiff,
			}); err != nil {
				return err
			}
		}
		lastID = rows[len(rows)-1].ID
	}
	return nil
}
