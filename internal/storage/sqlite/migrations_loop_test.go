package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/uptrace/bun/driver/sqliteshim"
	_ "github.com/uptrace/bun/driver/sqliteshim"
)

// TestRunMigrationsLoopsV6ToV7 builds a v7 file, re-adds ws_connection_sessions with user_version 6, and checks Open drops meta tables.
func TestRunMigrationsLoopsV6ToV7(t *testing.T) {
	ctx := context.Background()
	log := zerolog.Nop()
	dir := t.TempDir()
	path := filepath.Join(dir, "loop.db")

	s, err := Open(ctx, path, nil, log)
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	sqldb, err := sql.Open(sqliteshim.ShimName, normalizeDSN(path))
	if err != nil {
		t.Fatal(err)
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS ws_connection_sessions (
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
		`CREATE TABLE IF NOT EXISTS audit_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at INTEGER NOT NULL,
			action TEXT NOT NULL,
			detail TEXT,
			pubkey TEXT NOT NULL DEFAULT ''
		)`,
		`PRAGMA user_version = 6`,
	}
	for _, q := range stmts {
		if _, err := sqldb.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if err := sqldb.Close(); err != nil {
		t.Fatal(err)
	}

	s2, err := Open(ctx, path, nil, log)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s2.Close() }()

	db := s2.db
	var uv int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&uv); err != nil {
		t.Fatal(err)
	}
	if uv != schemaVersion {
		t.Fatalf("user_version: got %d want %d", uv, schemaVersion)
	}
	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='ws_connection_sessions'`,
	).Scan(&n); err != nil || n != 0 {
		t.Fatalf("ws_connection_sessions should be dropped: err=%v n=%d", err, n)
	}
}

// TestRunMigrationsLoopsFakeV5ToV7 keeps a v7 events schema but sets user_version to 5 with legacy meta tables present.
func TestRunMigrationsLoopsFakeV5ToV7(t *testing.T) {
	ctx := context.Background()
	log := zerolog.Nop()
	dir := t.TempDir()
	path := filepath.Join(dir, "multistep.db")

	s, err := Open(ctx, path, nil, log)
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	sqldb, err := sql.Open(sqliteshim.ShimName, normalizeDSN(path))
	if err != nil {
		t.Fatal(err)
	}
	legacy := []string{
		`CREATE TABLE IF NOT EXISTS audit_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at INTEGER NOT NULL,
			action TEXT NOT NULL,
			detail TEXT,
			pubkey TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS config_changelog (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at INTEGER NOT NULL,
			summary TEXT NOT NULL,
			json_diff TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS relay_metric_buckets (
			bucket_start_unix INTEGER NOT NULL PRIMARY KEY,
			events_stored INTEGER NOT NULL DEFAULT 0,
			events_rejected INTEGER NOT NULL DEFAULT 0,
			req_count INTEGER NOT NULL DEFAULT 0,
			close_count INTEGER NOT NULL DEFAULT 0,
			query_ms_sum INTEGER NOT NULL DEFAULT 0,
			query_ms_count INTEGER NOT NULL DEFAULT 0,
			subscriptions_open INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS ws_connection_sessions (
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
		`PRAGMA user_version = 5`,
	}
	for _, q := range legacy {
		if _, err := sqldb.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if err := sqldb.Close(); err != nil {
		t.Fatal(err)
	}

	s2, err := Open(ctx, path, nil, log)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s2.Close() }()

	var uv int
	if err := s2.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&uv); err != nil {
		t.Fatal(err)
	}
	if uv != schemaVersion {
		t.Fatalf("user_version after multi-step migrate: got %d want %d", uv, schemaVersion)
	}
	var metaTables int
	if err := s2.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('audit_log','config_changelog','relay_metric_buckets','ws_connection_sessions')`,
	).Scan(&metaTables); err != nil || metaTables != 0 {
		t.Fatalf("meta tables should be dropped: err=%v n=%d", err, metaTables)
	}
}

// TestPreflightMigrationTargetCurrent checks preflight on a fresh Open database.
func TestPreflightMigrationTargetCurrent(t *testing.T) {
	ctx := context.Background()
	log := zerolog.Nop()
	dir := t.TempDir()
	path := filepath.Join(dir, "pf.db")

	s, err := Open(ctx, path, nil, log)
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()

	out := PreflightMigrationTarget(ctx, path, log)
	if out.Status != "current" {
		t.Fatalf("status: got %q detail=%q", out.Status, out.Detail)
	}
	if out.ExpectedVersion != CurrentSchemaVersion() {
		t.Fatalf("expected_version: got %d", out.ExpectedVersion)
	}
	if out.ReportedVersion == nil || *out.ReportedVersion != CurrentSchemaVersion() {
		t.Fatalf("reported_version: %+v", out.ReportedVersion)
	}
}

func TestPreflightMigrationTargetEmptyPath(t *testing.T) {
	ctx := context.Background()
	out := PreflightMigrationTarget(ctx, "", zerolog.Nop())
	if out.Status != "unreadable" {
		t.Fatalf("got %q", out.Status)
	}
}
