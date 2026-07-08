package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/michmich112/congee/internal/storage"
	"github.com/michmich112/congee/internal/storage/sqlevent"
	"github.com/michmich112/congee/internal/storage/sqlitewriter"
	"github.com/rs/zerolog"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/driver/sqliteshim"
	_ "github.com/uptrace/bun/driver/sqliteshim"
)

// Store is a SQLite-backed event store.
type Store = sqlevent.Store

var _ storage.EventStore = (*Store)(nil)
var _ storage.MigrationSource = (*Store)(nil)

// CurrentSchemaVersion is the PRAGMA user_version / app-expected value for this binary.
func CurrentSchemaVersion() int { return sqlevent.CurrentSchemaVersion() }

// Open opens a SQLite database (WAL, Bun + sqliteshim), runs migrations, and starts the writer loop.
func Open(ctx context.Context, dsn string, notifier storage.EventNotifier, log zerolog.Logger) (*Store, error) {
	if !sqliteshim.HasDriver() {
		return nil, errors.New("sqlite: sqliteshim driver not available for this build target")
	}
	return sqlevent.Open(ctx, sqlevent.OpenConfig{
		Engine:        "sqlite",
		DSN:           dsn,
		Notifier:      notifier,
		Log:           log,
		OpenHandles:   sqlitewriter.OpenHandles,
		ResolveDBPath: sqlitewriter.ResolveMainFilePath,
	})
}

// PreflightMigrationTarget inspects a SQLite DSN without running migrations or starting the writer loop.
func PreflightMigrationTarget(ctx context.Context, dsn string, log zerolog.Logger) storage.MigrationTargetPreflight {
	cfg := sqlevent.DefaultSQLitePreflightConfig(dsn, log)
	if !sqliteshim.HasDriver() {
		cfg.HasDriver = func() bool { return false }
	} else {
		cfg.HasDriver = sqliteshim.HasDriver
		cfg.DriverName = sqliteshim.ShimName
		cfg.OpenDB = func(dsn string) (*sql.DB, error) {
			return sql.Open(sqliteshim.ShimName, dsn)
		}
	}
	return sqlevent.PreflightMigrationTarget(ctx, cfg)
}

// runMigrations applies schema DDL (used by migration loop tests).
func runMigrations(ctx context.Context, db *bun.DB, log zerolog.Logger) error {
	return sqlevent.RunMigrations(ctx, db, "sqlite", log)
}
