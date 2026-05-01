package postgres

import (
	"context"
	"fmt"

	"github.com/michmich112/congee/internal/storage"
)

// AdminStorageSnapshot implements storage.Store.
func (s *Store) AdminStorageSnapshot(ctx context.Context) (storage.AdminStorageSnapshot, error) {
	var out storage.AdminStorageSnapshot
	row := s.db.QueryRowContext(ctx, `
SELECT
  (SELECT COUNT(*)::bigint FROM events) AS ev,
  (SELECT COUNT(*)::bigint FROM event_tags) AS tg,
  (SELECT COUNT(*)::bigint FROM audit_log) AS au
`)
	if err := row.Scan(&out.Events, &out.Tags, &out.Audit); err != nil {
		return storage.AdminStorageSnapshot{}, fmt.Errorf("postgres: admin snapshot counts: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT pg_database_size(current_database())`).Scan(&out.Bytes); err != nil {
		return storage.AdminStorageSnapshot{}, fmt.Errorf("postgres: pg_database_size: %w", err)
	}
	return out, nil
}

// UpsertRelayMetricBucket implements storage.Store.
func (s *Store) UpsertRelayMetricBucket(ctx context.Context, b storage.RelayMetricBucket) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO relay_metric_buckets (
  bucket_start_unix, events_stored, events_rejected, req_count, close_count, query_ms_sum, query_ms_count
) VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (bucket_start_unix) DO UPDATE SET
  events_stored = EXCLUDED.events_stored,
  events_rejected = EXCLUDED.events_rejected,
  req_count = EXCLUDED.req_count,
  close_count = EXCLUDED.close_count,
  query_ms_sum = EXCLUDED.query_ms_sum,
  query_ms_count = EXCLUDED.query_ms_count
`, b.BucketStartUnix, b.EventsStored, b.EventsRejected, b.ReqCount, b.CloseCount, b.QueryMsSum, b.QueryMsCount)
	if err != nil {
		return fmt.Errorf("postgres: upsert relay_metric_buckets: %w", err)
	}
	return nil
}

// QueryRelayMetricBuckets implements storage.Store.
func (s *Store) QueryRelayMetricBuckets(ctx context.Context, q storage.RelayMetricBucketQuery) ([]storage.RelayMetricBucket, error) {
	lim := q.Limit
	if lim <= 0 {
		lim = 1440
	}
	if lim > 100000 {
		lim = 100000
	}
	var rows []storage.RelayMetricBucketRow
	err := s.db.NewSelect().Model(&rows).
		Where("bucket_start_unix >= ?", q.MinBucketStartUnix).
		Order("bucket_start_unix ASC").
		Limit(lim).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("postgres: query relay_metric_buckets: %w", err)
	}
	out := make([]storage.RelayMetricBucket, 0, len(rows))
	for i := range rows {
		r := rows[i]
		out = append(out, storage.RelayMetricBucket{
			BucketStartUnix: r.BucketStartUnix,
			EventsStored:    r.EventsStored,
			EventsRejected:  r.EventsRejected,
			ReqCount:        r.ReqCount,
			CloseCount:      r.CloseCount,
			QueryMsSum:      r.QueryMsSum,
			QueryMsCount:    r.QueryMsCount,
		})
	}
	return out, nil
}

// PurgeRelayMetricBucketsBefore implements storage.Store.
func (s *Store) PurgeRelayMetricBucketsBefore(ctx context.Context, cutoffStartUnixExclusive int64) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM relay_metric_buckets WHERE bucket_start_unix < $1`, cutoffStartUnixExclusive)
	if err != nil {
		return 0, fmt.Errorf("postgres: purge relay_metric_buckets: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return n, nil
}
