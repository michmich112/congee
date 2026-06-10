package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/michmich112/congee/internal/storage"
	"github.com/michmich112/congee/internal/storage/sqlitemeta"
	"github.com/uptrace/bun/driver/pgdriver"
	"github.com/uptrace/bun/driver/sqliteshim"
)

// migrateLegacyMetaSQLite copies operational metadata from a pre-v7 events SQLite file
// into the meta store before the event store runs schema v7 (which drops meta tables).
func migrateLegacyMetaSQLite(ctx context.Context, eventsDSN string, meta *sqlitemeta.Store) error {
	if !sqliteshim.HasDriver() {
		return errors.New("legacy meta: sqliteshim driver not available")
	}
	sqldb, err := sql.Open(sqliteshim.ShimName, normalizeLegacyDSN(eventsDSN))
	if err != nil {
		return fmt.Errorf("legacy meta: open events db: %w", err)
	}
	defer func() { _ = sqldb.Close() }()

	if err := sqldb.PingContext(ctx); err != nil {
		return fmt.Errorf("legacy meta: ping events db: %w", err)
	}

	var userVer int
	if err := sqldb.QueryRowContext(ctx, "PRAGMA user_version").Scan(&userVer); err != nil {
		return fmt.Errorf("legacy meta: read user_version: %w", err)
	}
	if userVer >= 7 {
		return nil
	}

	return copyAllLegacyMeta(ctx, sqldb, meta, false)
}

func normalizeLegacyDSN(dsn string) string {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return "file:congee.db?cache=shared"
	}
	if strings.HasPrefix(dsn, "file:") {
		return dsn
	}
	return "file:" + dsn + "?cache=shared"
}

func legacyTableExists(ctx context.Context, legacy *sql.DB, table string, postgres bool) (bool, error) {
	if postgres {
		var exists bool
		err := legacy.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1 FROM information_schema.tables
  WHERE table_schema = CURRENT_SCHEMA() AND table_name = $1
)`, table).Scan(&exists)
		return exists, err
	}
	var exists bool
	err := legacy.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?)`,
		table,
	).Scan(&exists)
	return exists, err
}

func copyAllLegacyMeta(ctx context.Context, legacy *sql.DB, meta *sqlitemeta.Store, postgres bool) error {
	if err := copyLegacyAudit(ctx, legacy, meta, postgres); err != nil {
		return err
	}
	if err := copyLegacyChangelog(ctx, legacy, meta, postgres); err != nil {
		return err
	}
	if err := copyLegacyMetricBuckets(ctx, legacy, meta, postgres); err != nil {
		return err
	}
	return copyLegacyWSSessions(ctx, legacy, meta, postgres)
}

func copyLegacyAudit(ctx context.Context, legacy *sql.DB, meta *sqlitemeta.Store, postgres bool) error {
	exists, err := legacyTableExists(ctx, legacy, "audit_log", postgres)
	if err != nil {
		return fmt.Errorf("legacy meta: check audit_log: %w", err)
	}
	if !exists {
		return nil
	}
	rows, err := legacy.QueryContext(ctx, `SELECT created_at, action, detail, pubkey FROM audit_log ORDER BY id ASC`)
	if err != nil {
		return fmt.Errorf("legacy meta: scan audit_log: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var e storage.AuditEntry
		if err := rows.Scan(&e.CreatedAt, &e.Action, &e.Detail, &e.Pubkey); err != nil {
			return err
		}
		dup, err := meta.HasAuditDuplicate(ctx, e)
		if err != nil {
			return fmt.Errorf("legacy meta: audit duplicate check: %w", err)
		}
		if dup {
			continue
		}
		if err := meta.SaveAuditEntry(ctx, e); err != nil {
			return fmt.Errorf("legacy meta: save audit: %w", err)
		}
	}
	return rows.Err()
}

func legacyChangelogExists(ctx context.Context, meta *sqlitemeta.Store, c storage.ConfigChange) (bool, error) {
	rows, err := meta.QueryConfigChangelog(ctx, 0)
	if err != nil {
		return false, err
	}
	for _, r := range rows {
		if r.CreatedAt == c.CreatedAt && r.Summary == c.Summary && r.JSONDiff == c.JSONDiff {
			return true, nil
		}
	}
	return false, nil
}

func copyLegacyChangelog(ctx context.Context, legacy *sql.DB, meta *sqlitemeta.Store, postgres bool) error {
	exists, err := legacyTableExists(ctx, legacy, "config_changelog", postgres)
	if err != nil {
		return fmt.Errorf("legacy meta: check config_changelog: %w", err)
	}
	if !exists {
		return nil
	}
	rows, err := legacy.QueryContext(ctx, `SELECT created_at, summary, json_diff FROM config_changelog ORDER BY id ASC`)
	if err != nil {
		return fmt.Errorf("legacy meta: scan config_changelog: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var c storage.ConfigChange
		if err := rows.Scan(&c.CreatedAt, &c.Summary, &c.JSONDiff); err != nil {
			return err
		}
		exists, err := legacyChangelogExists(ctx, meta, c)
		if err != nil {
			return fmt.Errorf("legacy meta: changelog duplicate check: %w", err)
		}
		if exists {
			continue
		}
		if err := meta.SaveConfigChange(ctx, c); err != nil {
			return fmt.Errorf("legacy meta: save changelog: %w", err)
		}
	}
	return rows.Err()
}

func copyLegacyMetricBuckets(ctx context.Context, legacy *sql.DB, meta *sqlitemeta.Store, postgres bool) error {
	exists, err := legacyTableExists(ctx, legacy, "relay_metric_buckets", postgres)
	if err != nil {
		return fmt.Errorf("legacy meta: check relay_metric_buckets: %w", err)
	}
	if !exists {
		return nil
	}
	rows, err := legacy.QueryContext(ctx, `
SELECT bucket_start_unix, events_stored, events_rejected, req_count, close_count, query_ms_sum, query_ms_count, subscriptions_open
FROM relay_metric_buckets ORDER BY bucket_start_unix ASC`)
	if err != nil {
		return fmt.Errorf("legacy meta: scan relay_metric_buckets: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var b storage.RelayMetricBucket
		if err := rows.Scan(&b.BucketStartUnix, &b.EventsStored, &b.EventsRejected, &b.ReqCount, &b.CloseCount, &b.QueryMsSum, &b.QueryMsCount, &b.SubscriptionsOpen); err != nil {
			return err
		}
		if err := meta.UpsertRelayMetricBucket(ctx, b); err != nil {
			return fmt.Errorf("legacy meta: save metric bucket: %w", err)
		}
	}
	return rows.Err()
}

func copyLegacyWSSessions(ctx context.Context, legacy *sql.DB, meta *sqlitemeta.Store, postgres bool) error {
	exists, err := legacyTableExists(ctx, legacy, "ws_connection_sessions", postgres)
	if err != nil {
		return fmt.Errorf("legacy meta: check ws_connection_sessions: %w", err)
	}
	if !exists {
		return nil
	}
	rows, err := legacy.QueryContext(ctx, `
SELECT conn_id, peer_ip, remote_addr, started_unix, ended_unix, total_req, total_client_event, series_json, subs_json
FROM ws_connection_sessions ORDER BY id ASC`)
	if err != nil {
		return fmt.Errorf("legacy meta: scan ws_connection_sessions: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var s storage.WSConnectionSession
		var series, subs string
		if err := rows.Scan(&s.ConnID, &s.PeerIP, &s.RemoteAddr, &s.StartedUnix, &s.EndedUnix, &s.TotalReq, &s.TotalClientEvent, &series, &subs); err != nil {
			return err
		}
		s.SeriesJSON = []byte(series)
		s.SubsJSON = []byte(subs)
		dup, err := meta.HasWSConnectionSession(ctx, s.ConnID, s.StartedUnix)
		if err != nil {
			return fmt.Errorf("legacy meta: ws session duplicate check: %w", err)
		}
		if dup {
			continue
		}
		if _, err := meta.SaveWSConnectionSession(ctx, s); err != nil {
			return fmt.Errorf("legacy meta: save ws session: %w", err)
		}
	}
	return rows.Err()
}

// migrateLegacyMetaPostgres copies operational metadata from a pre-v7 PostgreSQL events
// database into the SQLite meta sidecar before the event store runs schema v7.
func migrateLegacyMetaPostgres(ctx context.Context, eventsDSN string, meta *sqlitemeta.Store) error {
	dsn := strings.TrimSpace(eventsDSN)
	if dsn == "" {
		return nil
	}
	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	defer func() { _ = sqldb.Close() }()
	if err := sqldb.PingContext(ctx); err != nil {
		return fmt.Errorf("legacy meta: ping postgres: %w", err)
	}

	var version int
	err := sqldb.QueryRowContext(ctx, `SELECT version FROM congee_schema_version WHERE id = 1`).Scan(&version)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("legacy meta: read postgres schema version: %w", err)
	}
	if version >= 7 {
		return nil
	}

	return copyAllLegacyMeta(ctx, sqldb, meta, true)
}
