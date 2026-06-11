package sqlitemeta

import (
	"context"
	"database/sql"
	"errors"
	"sort"

	"github.com/michmich112/congee/internal/storage"
	"github.com/uptrace/bun"
)

func (s *Store) SaveAuditEntry(ctx context.Context, e storage.AuditEntry) error {
	return s.runWrite(ctx, "SaveAuditEntry", func(ctx context.Context, db bun.IDB) error {
		row := storage.AuditLogRow{
			CreatedAt: e.CreatedAt,
			Action:    e.Action,
			Detail:    e.Detail,
			Pubkey:    e.Pubkey,
		}
		_, err := db.NewInsert().Model(&row).Exec(ctx)
		return err
	})
}

func (s *Store) HasAuditDuplicate(ctx context.Context, e storage.AuditEntry) (bool, error) {
	n, err := s.db().NewSelect().Model((*storage.AuditLogRow)(nil)).
		Where("created_at = ? AND action = ? AND detail = ? AND pubkey = ?",
			e.CreatedAt, e.Action, e.Detail, e.Pubkey).
		Limit(1).
		Count(ctx)
	if err != nil {
		return false, err
	}
	if n > 0 {
		return true, nil
	}
	evID := storage.ExtractAuditDetailEventID(e.Detail)
	if evID == "" {
		return false, nil
	}
	pat := "%event_id=" + evID + "%"
	n2, err := s.db().NewSelect().Model((*storage.AuditLogRow)(nil)).
		Where("pubkey = ? AND LOWER(detail) LIKE ?", e.Pubkey, pat).
		Limit(1).
		Count(ctx)
	return n2 > 0, err
}

func applyAuditLogFilters(q *bun.SelectQuery, query storage.AuditQuery) *bun.SelectQuery {
	if query.Since > 0 {
		q = q.Where("created_at >= ?", query.Since)
	}
	if query.Until > 0 {
		q = q.Where("created_at <= ?", query.Until)
	}
	if query.Action != "" {
		q = q.Where("action = ?", query.Action)
	}
	if query.Pubkey != "" {
		q = q.Where("pubkey = ?", query.Pubkey)
	}
	if query.ConnID != "" {
		if pat, ok := storage.AuditDetailConnIDLikePattern(query.ConnID); ok {
			q = q.Where("detail LIKE ?", pat)
		}
	}
	if sql, args := storage.AuditDetailKindSuffixMatchOr(true, query.Kinds); sql != "" {
		q = q.Where(sql, args...)
	}
	return q
}

func (s *Store) QueryAuditLog(ctx context.Context, query storage.AuditQuery) ([]storage.AuditEntry, error) {
	var rows []storage.AuditLogRow
	q := applyAuditLogFilters(s.db().NewSelect().Model(&rows), query).Order("created_at DESC")
	if query.Limit > 0 {
		q = q.Limit(query.Limit)
	}
	if query.Offset > 0 {
		q = q.Offset(query.Offset)
	}
	if err := q.Scan(ctx); err != nil {
		return nil, err
	}
	out := make([]storage.AuditEntry, len(rows))
	for i, r := range rows {
		out[i] = storage.AuditEntry{
			CreatedAt: r.CreatedAt,
			Action:    r.Action,
			Detail:    r.Detail,
			Pubkey:    r.Pubkey,
		}
	}
	return out, nil
}

func (s *Store) CountAuditLog(ctx context.Context, query storage.AuditQuery) (int64, error) {
	q := applyAuditLogFilters(s.db().NewSelect().Model((*storage.AuditLogRow)(nil)), query)
	n, err := q.Count(ctx)
	return int64(n), err
}

func (s *Store) ListDistinctAuditKinds(ctx context.Context, scanLimit int) ([]int, error) {
	if scanLimit <= 0 {
		scanLimit = storage.DefaultAuditKindsScanLimit
	}
	if scanLimit > storage.MaxAuditKindsScanLimit {
		scanLimit = storage.MaxAuditKindsScanLimit
	}
	var rows []struct {
		Detail string `bun:"detail"`
	}
	err := s.db().NewSelect().
		Model((*storage.AuditLogRow)(nil)).
		Column("detail").
		Order("created_at DESC").
		Limit(scanLimit).
		Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	seen := make(map[int]struct{})
	for _, r := range rows {
		if k, ok := storage.ParseAuditDetailTrailingKind(r.Detail); ok {
			seen[k] = struct{}{}
		}
	}
	out := make([]int, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Ints(out)
	return out, nil
}

func (s *Store) PurgeAuditLog(ctx context.Context, olderThanUnix int64) (int64, error) {
	var n int64
	err := s.runWrite(ctx, "PurgeAuditLog", func(ctx context.Context, db bun.IDB) error {
		res, err := db.NewDelete().Model((*storage.AuditLogRow)(nil)).
			Where("created_at < ?", olderThanUnix).
			Exec(ctx)
		if err != nil {
			return err
		}
		v, err := res.RowsAffected()
		n = v
		return err
	})
	return n, err
}

func (s *Store) SaveWSConnectionSession(ctx context.Context, e storage.WSConnectionSession) (int64, error) {
	var insertedID int64
	err := s.runWrite(ctx, "SaveWSConnectionSession", func(ctx context.Context, db bun.IDB) error {
		row := storage.WSConnectionSessionToRow(e)
		_, err := db.NewInsert().Model(&row).Returning("id").Exec(ctx)
		if err != nil {
			return err
		}
		insertedID = row.ID
		return nil
	})
	return insertedID, err
}

func (s *Store) QueryWSConnectionSessions(ctx context.Context, q storage.WSConnectionSessionQuery) ([]storage.WSConnectionSession, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	offset := q.Offset
	if offset < 0 {
		offset = 0
	}
	var rows []storage.WSConnectionSessionRow
	err := s.db().NewSelect().Model(&rows).
		Order("ended_unix DESC", "id DESC").
		Limit(limit).
		Offset(offset).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]storage.WSConnectionSession, len(rows))
	for i := range rows {
		out[i] = storage.WSConnectionSessionFromRow(rows[i])
	}
	return out, nil
}

func (s *Store) CountWSConnectionSessions(ctx context.Context) (int64, error) {
	n, err := s.db().NewSelect().Model((*storage.WSConnectionSessionRow)(nil)).Count(ctx)
	return int64(n), err
}

func (s *Store) HasWSConnectionSession(ctx context.Context, connID string, startedUnix int64) (bool, error) {
	n, err := s.db().NewSelect().Model((*storage.WSConnectionSessionRow)(nil)).
		Where("conn_id = ? AND started_unix = ?", connID, startedUnix).
		Limit(1).
		Count(ctx)
	return n > 0, err
}

func (s *Store) GetWSConnectionSessionByID(ctx context.Context, id int64) (*storage.WSConnectionSession, error) {
	var row storage.WSConnectionSessionRow
	err := s.db().NewSelect().Model(&row).Where("id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	e := storage.WSConnectionSessionFromRow(row)
	return &e, nil
}

func (s *Store) PurgeWSConnectionSessionsBefore(ctx context.Context, olderThanUnix int64) (int64, error) {
	var n int64
	err := s.runWrite(ctx, "PurgeWSConnectionSessionsBefore", func(ctx context.Context, db bun.IDB) error {
		res, err := db.NewDelete().Model((*storage.WSConnectionSessionRow)(nil)).
			Where("ended_unix < ?", olderThanUnix).
			Exec(ctx)
		if err != nil {
			return err
		}
		v, err := res.RowsAffected()
		n = v
		return err
	})
	return n, err
}

func (s *Store) SaveConfigChange(ctx context.Context, c storage.ConfigChange) error {
	return s.runWrite(ctx, "SaveConfigChange", func(ctx context.Context, db bun.IDB) error {
		row := storage.ConfigChangelogRow{
			CreatedAt: c.CreatedAt,
			Summary:   c.Summary,
			JSONDiff:  c.JSONDiff,
		}
		_, err := db.NewInsert().Model(&row).Exec(ctx)
		return err
	})
}

func (s *Store) QueryConfigChangelog(ctx context.Context, limit int) ([]storage.ConfigChange, error) {
	var rows []storage.ConfigChangelogRow
	q := s.db().NewSelect().Model(&rows).Order("created_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Scan(ctx); err != nil {
		return nil, err
	}
	out := make([]storage.ConfigChange, len(rows))
	for i, r := range rows {
		out[i] = storage.ConfigChange{
			CreatedAt: r.CreatedAt,
			Summary:   r.Summary,
			JSONDiff:  r.JSONDiff,
		}
	}
	return out, nil
}

func (s *Store) UpsertRelayMetricBucket(ctx context.Context, b storage.RelayMetricBucket) error {
	return s.runWrite(ctx, "UpsertRelayMetricBucket", func(ctx context.Context, db bun.IDB) error {
		_, err := db.ExecContext(ctx, `
INSERT INTO relay_metric_buckets (
  bucket_start_unix, events_stored, events_rejected, req_count, close_count, query_ms_sum, query_ms_count, subscriptions_open
) VALUES (?,?,?,?,?,?,?,?)
ON CONFLICT(bucket_start_unix) DO UPDATE SET
  events_stored = excluded.events_stored,
  events_rejected = excluded.events_rejected,
  req_count = excluded.req_count,
  close_count = excluded.close_count,
  query_ms_sum = excluded.query_ms_sum,
  query_ms_count = excluded.query_ms_count,
  subscriptions_open = excluded.subscriptions_open
`, b.BucketStartUnix, b.EventsStored, b.EventsRejected, b.ReqCount, b.CloseCount, b.QueryMsSum, b.QueryMsCount, b.SubscriptionsOpen)
		return err
	})
}

func (s *Store) QueryRelayMetricBuckets(ctx context.Context, q storage.RelayMetricBucketQuery) ([]storage.RelayMetricBucket, error) {
	lim := q.Limit
	if lim <= 0 {
		lim = 1440
	}
	if lim > 100000 {
		lim = 100000
	}
	var rows []storage.RelayMetricBucketRow
	err := s.db().NewSelect().Model(&rows).
		Where("bucket_start_unix >= ?", q.MinBucketStartUnix).
		Order("bucket_start_unix ASC").
		Limit(lim).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]storage.RelayMetricBucket, 0, len(rows))
	for i := range rows {
		r := rows[i]
		out = append(out, storage.RelayMetricBucket{
			BucketStartUnix:   r.BucketStartUnix,
			EventsStored:      r.EventsStored,
			EventsRejected:    r.EventsRejected,
			ReqCount:          r.ReqCount,
			CloseCount:        r.CloseCount,
			QueryMsSum:        r.QueryMsSum,
			QueryMsCount:      r.QueryMsCount,
			SubscriptionsOpen: r.SubscriptionsOpen,
		})
	}
	return out, nil
}

func (s *Store) PurgeRelayMetricBucketsBefore(ctx context.Context, cutoffStartUnixExclusive int64) (int64, error) {
	var n int64
	err := s.runWrite(ctx, "PurgeRelayMetricBucketsBefore", func(ctx context.Context, db bun.IDB) error {
		res, err := db.ExecContext(ctx, `DELETE FROM relay_metric_buckets WHERE bucket_start_unix < ?`, cutoffStartUnixExclusive)
		if err != nil {
			return err
		}
		v, err := res.RowsAffected()
		n = v
		return err
	})
	return n, err
}
