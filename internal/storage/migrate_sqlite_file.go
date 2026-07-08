package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/michmich112/congee/internal/storage/sqlitewriter"
	"github.com/uptrace/bun/driver/sqliteshim"
)

// MigrateSQLiteFileViaVacuumInto copies a live SQLite database to dstPath using VACUUM INTO.
// The destination file must not exist; parent directories are created as needed.
func MigrateSQLiteFileViaVacuumInto(ctx context.Context, srcDSN, dstPath string) error {
	if !sqliteshim.HasDriver() {
		return fmt.Errorf("migration: sqlite driver not available")
	}
	dstPath, err := sqlitewriter.ResolveMainFilePath(dstPath)
	if err != nil {
		return fmt.Errorf("migration: resolve destination path: %w", err)
	}
	if _, err := os.Stat(dstPath); err == nil {
		return fmt.Errorf("migration: destination file already exists: %s", dstPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("migration: stat destination: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return fmt.Errorf("migration: create destination directory: %w", err)
	}

	srcDSN = strings.TrimSpace(srcDSN)
	if srcDSN == "" {
		return fmt.Errorf("migration: source dsn is empty")
	}
	normSrc := sqlitewriter.NormalizeDSN(srcDSN)
	sqldb, err := sql.Open(sqliteshim.ShimName, normSrc)
	if err != nil {
		return fmt.Errorf("migration: open source: %w", err)
	}
	defer func() { _ = sqldb.Close() }()
	if err := sqldb.PingContext(ctx); err != nil {
		return fmt.Errorf("migration: ping source: %w", err)
	}

	// VACUUM INTO requires a string literal path in SQLite; use absolute path with escaped quotes.
	absDst, err := filepath.Abs(dstPath)
	if err != nil {
		return fmt.Errorf("migration: absolute destination path: %w", err)
	}
	escaped := strings.ReplaceAll(absDst, `'`, `''`)
	query := fmt.Sprintf("VACUUM INTO '%s'", escaped)
	if _, err := sqldb.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("migration: vacuum into: %w", err)
	}
	return nil
}

// MigrateSQLiteToTursoNative copies events from a SQLite source to a new Turso/libSQL file via VACUUM INTO.
func MigrateSQLiteToTursoNative(ctx context.Context, srcDSN, dstDSN string) (MigrationSummary, error) {
	src, err := openSQLiteMigrationCounts(ctx, srcDSN)
	if err != nil {
		return MigrationSummary{}, err
	}
	if err := MigrateSQLiteFileViaVacuumInto(ctx, srcDSN, dstDSN); err != nil {
		return MigrationSummary{}, err
	}
	dst, err := openTursoMigrationCounts(ctx, dstDSN)
	if err != nil {
		return MigrationSummary{}, err
	}
	if src.Events != dst.Events || src.Tags != dst.Tags {
		return MigrationSummary{}, fmt.Errorf("migration: verification failed: source events=%d tags=%d, destination events=%d tags=%d",
			src.Events, src.Tags, dst.Events, dst.Tags)
	}
	return MigrationSummary{
		Source:           src,
		DestinationFinal: dst,
		EventsInserted:   dst.Events,
		TagsAdded:        dst.Tags,
	}, nil
}

func openSQLiteMigrationCounts(ctx context.Context, dsn string) (MigrationCounts, error) {
	if !sqliteshim.HasDriver() {
		return MigrationCounts{}, fmt.Errorf("migration: sqlite driver not available")
	}
	norm := sqlitewriter.NormalizeDSN(dsn)
	sqldb, err := sql.Open(sqliteshim.ShimName, norm)
	if err != nil {
		return MigrationCounts{}, err
	}
	defer func() { _ = sqldb.Close() }()
	if err := sqldb.PingContext(ctx); err != nil {
		return MigrationCounts{}, err
	}
	return sqliteFileCounts(ctx, sqldb)
}

func openTursoMigrationCounts(ctx context.Context, dsn string) (MigrationCounts, error) {
	if !sqlitewriter.HasLibsqlDriver() {
		return MigrationCounts{}, fmt.Errorf("migration: turso driver not available")
	}
	norm := sqlitewriter.NormalizeLibsqlDSN(dsn)
	sqldb, err := sql.Open("libsql", norm)
	if err != nil {
		return MigrationCounts{}, err
	}
	defer func() { _ = sqldb.Close() }()
	if err := sqldb.PingContext(ctx); err != nil {
		return MigrationCounts{}, err
	}
	return sqliteFileCounts(ctx, sqldb)
}

func sqliteFileCounts(ctx context.Context, sqldb *sql.DB) (MigrationCounts, error) {
	var events, tags int64
	if err := sqldb.QueryRowContext(ctx, `SELECT COUNT(*) FROM events`).Scan(&events); err != nil {
		return MigrationCounts{}, fmt.Errorf("migration: count events: %w", err)
	}
	if err := sqldb.QueryRowContext(ctx, `SELECT COUNT(*) FROM event_tags WHERE event_id IN (SELECT id FROM events)`).Scan(&tags); err != nil {
		return MigrationCounts{}, fmt.Errorf("migration: count tags: %w", err)
	}
	return MigrationCounts{Events: events, Tags: tags}, nil
}
