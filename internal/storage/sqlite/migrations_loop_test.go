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

// TestRunMigrationsLoopsV5ToV6 builds a v6 file, downgrades to v5 metadata, and checks one Open converges to v6.
func TestRunMigrationsLoopsV5ToV6(t *testing.T) {
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
	if _, err := sqldb.ExecContext(ctx, `DROP TABLE IF EXISTS ws_connection_sessions`); err != nil {
		t.Fatal(err)
	}
	if _, err := sqldb.ExecContext(ctx, `PRAGMA user_version = 5`); err != nil {
		t.Fatal(err)
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
	).Scan(&n); err != nil || n != 1 {
		t.Fatalf("ws_connection_sessions missing: err=%v n=%d", err, n)
	}
}

// TestRunMigrationsLoopsFakeV3ToV6OnV6Schema keeps a fully migrated database file but sets user_version
// back to 3. One Open must run v3→v4, v4→v5, and v5→v6 (multiple loop iterations) and land at v6.
func TestRunMigrationsLoopsFakeV3ToV6OnV6Schema(t *testing.T) {
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
	if _, err := sqldb.ExecContext(ctx, `PRAGMA user_version = 3`); err != nil {
		t.Fatal(err)
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
