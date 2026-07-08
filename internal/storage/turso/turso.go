package turso

import (
	"context"
	"database/sql"
	"errors"
	"os"

	"github.com/michmich112/congee/internal/storage"
	"github.com/michmich112/congee/internal/storage/sqlevent"
	"github.com/michmich112/congee/internal/storage/sqlitewriter"
	"github.com/rs/zerolog"
)

// Store is a Turso/libSQL-backed event store (local on-disk file).
type Store = sqlevent.Store

var _ storage.EventStore = (*Store)(nil)
var _ storage.MigrationSource = (*Store)(nil)

// HasDriver reports whether go-libsql is linked (CGO build).
func HasDriver() bool { return sqlitewriter.HasLibsqlDriver() }

// CurrentSchemaVersion is the PRAGMA user_version / app-expected value for this binary.
func CurrentSchemaVersion() int { return sqlevent.CurrentSchemaVersion() }

// Open opens a local libSQL database file, runs migrations, and starts the writer loop.
func Open(ctx context.Context, dsn string, notifier storage.EventNotifier, log zerolog.Logger) (*Store, error) {
	if !HasDriver() {
		return nil, errors.New("turso: libsql driver not available (build with CGO_ENABLED=1)")
	}
	return sqlevent.Open(ctx, sqlevent.OpenConfig{
		Engine:        "turso",
		DSN:           dsn,
		Notifier:      notifier,
		Log:           log,
		OpenHandles:   sqlitewriter.OpenLibsqlHandles,
		ResolveDBPath: sqlitewriter.ResolveMainFilePath,
	})
}

// PreflightMigrationTarget inspects a Turso/libSQL DSN without running migrations.
// Missing files are reported as empty without opening libSQL (which would create the file
// and break a later VACUUM INTO into that path).
func PreflightMigrationTarget(ctx context.Context, dsn string, log zerolog.Logger) storage.MigrationTargetPreflight {
	exp := CurrentSchemaVersion()
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
				Detail:          "destination file does not exist; native sqlite→turso copy will create it",
			}
		}
		return storage.MigrationTargetPreflight{
			Status:          storage.MigrationPreflightUnreadable,
			ExpectedVersion: exp,
			Detail:          "turso: stat destination: " + err.Error(),
		}
	}

	cfg := sqlevent.DefaultTursoPreflightConfig(dsn, log)
	cfg.OpenDB = func(dsn string) (*sql.DB, error) {
		return sql.Open("libsql", dsn)
	}
	return sqlevent.PreflightMigrationTarget(ctx, cfg)
}
