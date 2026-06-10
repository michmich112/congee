package db

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/storage"
	"github.com/michmich112/congee/internal/storage/sqlitemeta"
	"github.com/rs/zerolog"
	"github.com/uptrace/bun/driver/pgdriver"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func testPostgresDSN(t *testing.T) string {
	t.Helper()
	d := strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
	if d == "" {
		t.Skip("set TEST_POSTGRES_DSN to run postgres legacy meta tests")
	}
	return d
}

func TestLegacyMetaMigrationFromV6EventsDB(t *testing.T) {
	if !sqliteshim.HasDriver() {
		t.Skip("sqliteshim not available")
	}
	ctx := context.Background()
	dir := t.TempDir()
	eventsPath := filepath.Join(dir, "legacy-events.db")

	sqldb, err := sql.Open(sqliteshim.ShimName, "file:"+eventsPath+"?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	legacyDDL := []string{
		`CREATE TABLE audit_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at INTEGER NOT NULL,
			action TEXT NOT NULL,
			detail TEXT,
			pubkey TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE config_changelog (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at INTEGER NOT NULL,
			summary TEXT NOT NULL,
			json_diff TEXT NOT NULL
		)`,
		`CREATE TABLE relay_metric_buckets (
			bucket_start_unix INTEGER NOT NULL PRIMARY KEY,
			events_stored INTEGER NOT NULL DEFAULT 0,
			events_rejected INTEGER NOT NULL DEFAULT 0,
			req_count INTEGER NOT NULL DEFAULT 0,
			close_count INTEGER NOT NULL DEFAULT 0,
			query_ms_sum INTEGER NOT NULL DEFAULT 0,
			query_ms_count INTEGER NOT NULL DEFAULT 0,
			subscriptions_open INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE ws_connection_sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			conn_id TEXT NOT NULL,
			peer_ip TEXT NOT NULL,
			remote_addr TEXT NOT NULL,
			started_unix INTEGER NOT NULL,
			ended_unix INTEGER NOT NULL,
			total_req INTEGER NOT NULL DEFAULT 0,
			total_client_event INTEGER NOT NULL DEFAULT 0,
			series_json TEXT NOT NULL DEFAULT '[]',
			subs_json TEXT NOT NULL DEFAULT '[]'
		)`,
		`INSERT INTO audit_log (created_at, action, detail, pubkey) VALUES (10, 'event_stored', 'kind=1', 'pub')`,
		`INSERT INTO config_changelog (created_at, summary, json_diff) VALUES (11, 's', '{}')`,
		`INSERT INTO relay_metric_buckets (bucket_start_unix, events_stored) VALUES (60, 3)`,
		`INSERT INTO ws_connection_sessions (conn_id, peer_ip, remote_addr, started_unix, ended_unix, total_req, total_client_event, series_json, subs_json)
		 VALUES ('conn-1', '127.0.0.1', '127.0.0.1:1', 100, 200, 2, 1, '[]', '[]')`,
		`PRAGMA user_version = 6`,
	}
	for _, q := range legacyDDL {
		if _, err := sqldb.ExecContext(ctx, q); err != nil {
			t.Fatalf("legacy ddl: %v", err)
		}
	}
	_ = sqldb.Close()

	sec := config.DatabaseSection{Type: "sqlite", DSN: eventsPath}
	h, err := Open(ctx, sec, "", zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	n, err := h.CountAuditLog(ctx, storage.AuditQuery{})
	if err != nil || n != 1 {
		t.Fatalf("audit after legacy migrate: %d %v", n, err)
	}
	ch, err := h.QueryConfigChangelog(ctx, 5)
	if err != nil || len(ch) != 1 {
		t.Fatalf("changelog: %+v %v", ch, err)
	}
	buckets, err := h.QueryRelayMetricBuckets(ctx, storage.RelayMetricBucketQuery{MinBucketStartUnix: 0, Limit: 10})
	if err != nil || len(buckets) != 1 || buckets[0].EventsStored != 3 {
		t.Fatalf("buckets: %+v %v", buckets, err)
	}
	wsN, err := h.CountWSConnectionSessions(ctx)
	if err != nil || wsN != 1 {
		t.Fatalf("ws sessions after legacy migrate: %d %v", wsN, err)
	}
	sessions, err := h.QueryWSConnectionSessions(ctx, storage.WSConnectionSessionQuery{Limit: 10})
	if err != nil || len(sessions) != 1 || sessions[0].ConnID != "conn-1" || sessions[0].StartedUnix != 100 {
		t.Fatalf("ws session rows: %+v %v", sessions, err)
	}

	var userVer int
	checkDB, err := sql.Open(sqliteshim.ShimName, "file:"+eventsPath+"?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer checkDB.Close()
	if err := checkDB.QueryRowContext(ctx, "PRAGMA user_version").Scan(&userVer); err != nil {
		t.Fatal(err)
	}
	if userVer != 7 {
		t.Fatalf("events db user_version=%d want 7", userVer)
	}
	var hasAudit bool
	if err := checkDB.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type='table' AND name='audit_log')`,
	).Scan(&hasAudit); err != nil {
		t.Fatal(err)
	}
	if hasAudit {
		t.Fatal("audit_log should be dropped from events db after v7")
	}

	metaPath := filepath.Join(dir, "congee-meta.db")
	var metaAudit int
	metaDB, err := sql.Open(sqliteshim.ShimName, "file:"+metaPath+"?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer metaDB.Close()
	if err := metaDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_log`).Scan(&metaAudit); err != nil {
		t.Fatalf("meta db: %v", err)
	}
	if metaAudit != 1 {
		t.Fatalf("meta audit rows=%d", metaAudit)
	}
	var metaWS int
	if err := metaDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM ws_connection_sessions`).Scan(&metaWS); err != nil {
		t.Fatalf("meta ws sessions: %v", err)
	}
	if metaWS != 1 {
		t.Fatalf("meta ws session rows=%d", metaWS)
	}
}

func TestLegacyMetaMigrationIdempotentReopen(t *testing.T) {
	if !sqliteshim.HasDriver() {
		t.Skip("sqliteshim not available")
	}
	ctx := context.Background()
	dir := t.TempDir()
	eventsPath := filepath.Join(dir, "legacy-events.db")
	metaPath := filepath.Join(dir, "congee-meta.db")

	sqldb, err := sql.Open(sqliteshim.ShimName, "file:"+eventsPath+"?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	legacyDDL := []string{
		`CREATE TABLE audit_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at INTEGER NOT NULL,
			action TEXT NOT NULL,
			detail TEXT,
			pubkey TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE config_changelog (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at INTEGER NOT NULL,
			summary TEXT NOT NULL,
			json_diff TEXT NOT NULL
		)`,
		`INSERT INTO audit_log (created_at, action, detail, pubkey) VALUES (10, 'event_stored', 'kind=1', 'pub')`,
		`INSERT INTO config_changelog (created_at, summary, json_diff) VALUES (11, 's', '{}')`,
		`PRAGMA user_version = 6`,
	}
	for _, q := range legacyDDL {
		if _, err := sqldb.ExecContext(ctx, q); err != nil {
			t.Fatalf("legacy ddl: %v", err)
		}
	}
	_ = sqldb.Close()

	sec := config.DatabaseSection{Type: "sqlite", DSN: eventsPath}
	h, err := Open(ctx, sec, "", zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	auditBefore, err := h.CountAuditLog(ctx, storage.AuditQuery{})
	if err != nil || auditBefore != 1 {
		t.Fatalf("audit after first open: %d %v", auditBefore, err)
	}
	chBefore, err := h.QueryConfigChangelog(ctx, 5)
	if err != nil || len(chBefore) != 1 {
		t.Fatalf("changelog after first open: %+v %v", chBefore, err)
	}

	meta, err := sqlitemeta.Open(ctx, metaPath, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer meta.Close()

	if err := migrateLegacyMetaSQLite(ctx, eventsPath, meta); err != nil {
		t.Fatal(err)
	}

	auditAfter, err := h.CountAuditLog(ctx, storage.AuditQuery{})
	if err != nil || auditAfter != 1 {
		t.Fatalf("audit after re-migrate: %d want 1 %v", auditAfter, err)
	}
	chAfter, err := h.QueryConfigChangelog(ctx, 5)
	if err != nil || len(chAfter) != 1 {
		t.Fatalf("changelog after re-migrate: %+v want 1 %v", chAfter, err)
	}
}

func TestLegacyMetaMigrationFromV6PostgresDB(t *testing.T) {
	dsn := testPostgresDSN(t)
	ctx := context.Background()
	dir := t.TempDir()
	metaPath := filepath.Join(dir, "congee-meta.db")

	pg := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	defer pg.Close()

	legacyDDL := []string{
		`CREATE TABLE IF NOT EXISTS congee_schema_version (
			id SMALLINT PRIMARY KEY CHECK (id = 1),
			version INT NOT NULL
		)`,
		`INSERT INTO congee_schema_version (id, version) VALUES (1, 7) ON CONFLICT (id) DO UPDATE SET version = EXCLUDED.version`,
		`CREATE TABLE IF NOT EXISTS audit_log (
			id BIGSERIAL PRIMARY KEY,
			created_at BIGINT NOT NULL,
			action TEXT NOT NULL,
			detail TEXT,
			pubkey TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS config_changelog (
			id BIGSERIAL PRIMARY KEY,
			created_at BIGINT NOT NULL,
			summary TEXT NOT NULL,
			json_diff TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS relay_metric_buckets (
			bucket_start_unix BIGINT NOT NULL PRIMARY KEY,
			events_stored BIGINT NOT NULL DEFAULT 0,
			events_rejected BIGINT NOT NULL DEFAULT 0,
			req_count BIGINT NOT NULL DEFAULT 0,
			close_count BIGINT NOT NULL DEFAULT 0,
			query_ms_sum BIGINT NOT NULL DEFAULT 0,
			query_ms_count BIGINT NOT NULL DEFAULT 0,
			subscriptions_open BIGINT NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS ws_connection_sessions (
			id BIGSERIAL PRIMARY KEY,
			conn_id TEXT NOT NULL,
			peer_ip TEXT NOT NULL,
			remote_addr TEXT NOT NULL,
			started_unix BIGINT NOT NULL,
			ended_unix BIGINT NOT NULL,
			total_req BIGINT NOT NULL DEFAULT 0,
			total_client_event BIGINT NOT NULL DEFAULT 0,
			series_json TEXT NOT NULL DEFAULT '[]',
			subs_json TEXT NOT NULL DEFAULT '[]'
		)`,
	}
	for _, q := range legacyDDL {
		if _, err := pg.ExecContext(ctx, q); err != nil {
			t.Fatalf("legacy postgres ddl: %v", err)
		}
	}

	cleanup := func() {
		_, _ = pg.ExecContext(ctx, `DELETE FROM ws_connection_sessions WHERE conn_id = 'legacy-pg-conn'`)
		_, _ = pg.ExecContext(ctx, `DELETE FROM relay_metric_buckets WHERE bucket_start_unix = 4242`)
		_, _ = pg.ExecContext(ctx, `DELETE FROM config_changelog WHERE summary = 'legacy-pg-test'`)
		_, _ = pg.ExecContext(ctx, `DELETE FROM audit_log WHERE action = 'legacy_pg_test'`)
		_, _ = pg.ExecContext(ctx, `UPDATE congee_schema_version SET version = 7 WHERE id = 1`)
	}
	defer cleanup()

	if _, err := pg.ExecContext(ctx, `INSERT INTO audit_log (created_at, action, detail, pubkey) VALUES (42, 'legacy_pg_test', 'd', 'pub')`); err != nil {
		t.Fatal(err)
	}
	if _, err := pg.ExecContext(ctx, `INSERT INTO config_changelog (created_at, summary, json_diff) VALUES (43, 'legacy-pg-test', '{}')`); err != nil {
		t.Fatal(err)
	}
	if _, err := pg.ExecContext(ctx, `INSERT INTO relay_metric_buckets (bucket_start_unix, events_stored) VALUES (4242, 9)`); err != nil {
		t.Fatal(err)
	}
	if _, err := pg.ExecContext(ctx, `INSERT INTO ws_connection_sessions (conn_id, peer_ip, remote_addr, started_unix, ended_unix, total_req, total_client_event, series_json, subs_json)
		VALUES ('legacy-pg-conn', '10.0.0.1', '10.0.0.1:9', 500, 600, 1, 0, '[]', '[]')`); err != nil {
		t.Fatal(err)
	}
	if _, err := pg.ExecContext(ctx, `UPDATE congee_schema_version SET version = 6 WHERE id = 1`); err != nil {
		t.Fatal(err)
	}

	meta, err := sqlitemeta.Open(ctx, metaPath, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer meta.Close()

	if err := migrateLegacyMetaPostgres(ctx, dsn, meta); err != nil {
		t.Fatal(err)
	}

	nAudit, err := meta.CountAuditLog(ctx, storage.AuditQuery{Action: "legacy_pg_test"})
	if err != nil || nAudit != 1 {
		t.Fatalf("postgres legacy audit: %d %v", nAudit, err)
	}
	ch, err := meta.QueryConfigChangelog(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	var foundChangelog bool
	for _, c := range ch {
		if c.Summary == "legacy-pg-test" {
			foundChangelog = true
			break
		}
	}
	if !foundChangelog {
		t.Fatalf("postgres legacy changelog missing: %+v", ch)
	}
	buckets, err := meta.QueryRelayMetricBuckets(ctx, storage.RelayMetricBucketQuery{MinBucketStartUnix: 4242, Limit: 1})
	if err != nil || len(buckets) != 1 || buckets[0].EventsStored != 9 {
		t.Fatalf("postgres legacy buckets: %+v %v", buckets, err)
	}
	wsN, err := meta.CountWSConnectionSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := meta.QueryWSConnectionSessions(ctx, storage.WSConnectionSessionQuery{Limit: 500})
	if err != nil {
		t.Fatal(err)
	}
	var foundWS bool
	for _, s := range sessions {
		if s.ConnID == "legacy-pg-conn" && s.StartedUnix == 500 {
			foundWS = true
			break
		}
	}
	if !foundWS {
		t.Fatalf("postgres legacy ws session missing (total=%d): %+v", wsN, sessions)
	}
}
