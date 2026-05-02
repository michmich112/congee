package sqlite

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/michmich112/congee/internal/storage"
	"github.com/uptrace/bun"
)

func sqliteMainFilePath(rawDSN string) (string, error) {
	s := normalizeDSN(rawDSN)
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("sqlite: parse dsn: %w", err)
	}
	if u.Scheme != "file" {
		return "", fmt.Errorf("sqlite: expected file: scheme in dsn")
	}
	if p := strings.TrimSpace(u.Path); p != "" && p != "/" {
		return filepath.Clean(p), nil
	}
	name := strings.TrimSpace(u.Opaque)
	if name == "" {
		return "", fmt.Errorf("sqlite: empty path in file dsn")
	}
	name = strings.TrimPrefix(name, "./")
	if filepath.IsAbs(name) {
		return filepath.Clean(name), nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Clean(filepath.Join(cwd, name)), nil
}

func sqliteOnDiskBytes(mainPath string) int64 {
	var sum int64
	for _, p := range []string{mainPath, mainPath + "-wal", mainPath + "-shm"} {
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		if !st.IsDir() {
			sum += st.Size()
		}
	}
	return sum
}

// AdminStorageSnapshot implements storage.Store.
func (s *Store) AdminStorageSnapshot(ctx context.Context) (storage.AdminStorageSnapshot, error) {
	var out storage.AdminStorageSnapshot
	out.Bytes = sqliteOnDiskBytes(s.dbPath)
	row := s.db.QueryRowContext(ctx, `
SELECT
  (SELECT COUNT(*) FROM events) AS ev,
  (SELECT COUNT(*) FROM event_tags) AS tg,
  (SELECT COUNT(*) FROM audit_log) AS au
`)
	if err := row.Scan(&out.Events, &out.Tags, &out.Audit); err != nil {
		return storage.AdminStorageSnapshot{}, fmt.Errorf("sqlite: admin snapshot counts: %w", err)
	}
	return out, nil
}

// UpsertRelayMetricBucket implements storage.Store (single-writer queue).
func (s *Store) UpsertRelayMetricBucket(ctx context.Context, b storage.RelayMetricBucket) error {
	return s.runWrite(ctx, func(ctx context.Context, db bun.IDB) error {
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
		return nil, fmt.Errorf("sqlite: query relay_metric_buckets: %w", err)
	}
	out := make([]storage.RelayMetricBucket, 0, len(rows))
	for i := range rows {
		r := rows[i]
		out = append(out, storage.RelayMetricBucket{
			BucketStartUnix:     r.BucketStartUnix,
			EventsStored:        r.EventsStored,
			EventsRejected:      r.EventsRejected,
			ReqCount:            r.ReqCount,
			CloseCount:          r.CloseCount,
			QueryMsSum:          r.QueryMsSum,
			QueryMsCount:        r.QueryMsCount,
			SubscriptionsOpen:   r.SubscriptionsOpen,
		})
	}
	return out, nil
}

// PurgeRelayMetricBucketsBefore implements storage.Store.
func (s *Store) PurgeRelayMetricBucketsBefore(ctx context.Context, cutoffStartUnixExclusive int64) (int64, error) {
	var n int64
	err := s.runWrite(ctx, func(ctx context.Context, db bun.IDB) error {
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
