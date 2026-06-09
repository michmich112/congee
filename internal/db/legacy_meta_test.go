package db_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/db"
	"github.com/michmich112/congee/internal/storage"
	"github.com/rs/zerolog"
	"github.com/uptrace/bun/driver/sqliteshim"
)

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
		`INSERT INTO audit_log (created_at, action, detail, pubkey) VALUES (10, 'event_stored', 'kind=1', 'pub')`,
		`INSERT INTO config_changelog (created_at, summary, json_diff) VALUES (11, 's', '{}')`,
		`INSERT INTO relay_metric_buckets (bucket_start_unix, events_stored) VALUES (60, 3)`,
		`PRAGMA user_version = 6`,
	}
	for _, q := range legacyDDL {
		if _, err := sqldb.ExecContext(ctx, q); err != nil {
			t.Fatalf("legacy ddl: %v", err)
		}
	}
	_ = sqldb.Close()

	sec := config.DatabaseSection{Type: "sqlite", DSN: eventsPath}
	h, err := db.Open(ctx, sec, "", zerolog.Nop())
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
	_ = metaPath
}
