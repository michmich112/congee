package turso

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/michmich112/congee/internal/storage"
	"github.com/michmich112/congee/internal/storage/ftsv7detach"
	"github.com/michmich112/congee/internal/storage/sqlevent"
	"github.com/michmich112/congee/internal/storage/sqlitewriter"
	"github.com/rs/zerolog"
)

// Store is a Turso/libSQL-backed event store (local on-disk file).
type Store = sqlevent.Store

var _ storage.EventStore = (*Store)(nil)
var _ storage.MigrationSource = (*Store)(nil)

// HasDriver reports whether the local Turso driver is linked.
func HasDriver() bool { return sqlitewriter.HasTursoDriver() }

// CurrentSchemaVersion is the PRAGMA user_version / app-expected value for this binary.
func CurrentSchemaVersion() int { return sqlevent.CurrentSchemaVersion() }

// Open opens a local database file, upgrades a pre-v8 FTS5 schema when needed, runs migrations, and starts the writer loop.
func Open(ctx context.Context, dsn string, notifier storage.EventNotifier, log zerolog.Logger) (*Store, error) {
	if !HasDriver() {
		return nil, errors.New("turso: driver not available")
	}
	path, err := sqlitewriter.ResolveMainFilePath(dsn)
	if err != nil {
		return nil, err
	}
	if _, statErr := os.Stat(path); statErr == nil {
		if err := ftsv7detach.PrepareEvents(ctx, path, log); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(statErr) {
		return nil, fmt.Errorf("turso: stat database: %w", statErr)
	}
	return sqlevent.Open(ctx, sqlevent.OpenConfig{
		Engine:        "turso",
		DSN:           dsn,
		Notifier:      notifier,
		Log:           log,
		OpenHandles:   sqlitewriter.OpenTursoHandles,
		ResolveDBPath: sqlitewriter.ResolveMainFilePath,
	})
}

// PreflightMigrationTarget inspects a Turso/libSQL DSN without running migrations.
// Missing files are reported as empty without opening libSQL (which would create the file).
func PreflightMigrationTarget(ctx context.Context, dsn string, log zerolog.Logger) storage.MigrationTargetPreflight {
	exp := CurrentSchemaVersion()
	if strings.TrimSpace(dsn) == "" {
		return storage.MigrationTargetPreflight{
			Status:          storage.MigrationPreflightUnreadable,
			ExpectedVersion: exp,
			Detail:          "turso dsn is empty",
		}
	}
	path, err := sqlitewriter.ResolveMainFilePath(dsn)
	if err != nil {
		return storage.MigrationTargetPreflight{
			Status:          storage.MigrationPreflightUnreadable,
			ExpectedVersion: exp,
			Detail:          "turso: resolve path: " + err.Error(),
		}
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return storage.MigrationTargetPreflight{
				Status:          storage.MigrationPreflightEmpty,
				ExpectedVersion: exp,
				Detail:          "destination file does not exist; row-by-row migrate will create it",
			}
		}
		return storage.MigrationTargetPreflight{
			Status:          storage.MigrationPreflightUnreadable,
			ExpectedVersion: exp,
			Detail:          "turso: stat destination: " + err.Error(),
		}
	}

	hasFTS, probeErr := ftsv7detach.HasEventFTS5(ctx, path)
	cfg := sqlevent.DefaultTursoPreflightConfig(dsn, log)
	if probeErr == nil && hasFTS {
		cfg.OpenDB = func(dsn string) (*sql.DB, error) {
			sqldb, _, err := sqlitewriter.OpenLibsqlHandles(ctx, dsn, log)
			return sqldb, err
		}
		return sqlevent.PreflightMigrationTarget(ctx, cfg)
	}
	cfg.OpenDB = func(dsn string) (*sql.DB, error) {
		sqldb, _, err := sqlitewriter.OpenTursoHandles(ctx, dsn, log)
		return sqldb, err
	}
	return sqlevent.PreflightMigrationTarget(ctx, cfg)
}
