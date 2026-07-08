package sqlevent

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/michmich112/congee/internal/storage"
	"github.com/michmich112/congee/internal/storage/sqlitewriter"
	"github.com/rs/zerolog"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
)

// PreflightConfig inspects a SQLite-compatible DSN without running migrations or starting the writer loop.
type PreflightConfig struct {
	Engine      string
	DSN         string
	Log         zerolog.Logger
	HasDriver   func() bool
	DriverName  string
	NormalizeDSN func(string) string
	OpenDB      func(dsn string) (*sql.DB, error)
}

// PreflightMigrationTarget inspects a target database for admin migration tooling.
func PreflightMigrationTarget(ctx context.Context, cfg PreflightConfig) storage.MigrationTargetPreflight {
	engine := strings.TrimSpace(cfg.Engine)
	if engine == "" {
		engine = "sqlite"
	}
	exp := CurrentSchemaVersion()
	out := storage.MigrationTargetPreflight{
		ExpectedVersion: exp,
		Detail:          "",
	}
	if cfg.HasDriver != nil && !cfg.HasDriver() {
		out.Status = storage.MigrationPreflightUnreadable
		out.Detail = engine + " driver not available for this build target"
		return out
	}
	dsn := strings.TrimSpace(cfg.DSN)
	if dsn == "" {
		out.Status = storage.MigrationPreflightUnreadable
		out.Detail = engine + " dsn is empty"
		return out
	}

	norm := cfg.DSN
	if cfg.NormalizeDSN != nil {
		norm = cfg.NormalizeDSN(dsn)
	}
	var sqldb *sql.DB
	var err error
	if cfg.OpenDB != nil {
		sqldb, err = cfg.OpenDB(norm)
	} else {
		sqldb, err = sql.Open(cfg.DriverName, norm)
	}
	if err != nil {
		out.Status = storage.MigrationPreflightUnreadable
		out.Detail = fmt.Sprintf("%s: open: %v", engine, err)
		return out
	}

	if err := sqldb.PingContext(ctx); err != nil {
		_ = sqldb.Close()
		cfg.Log.Debug().Err(err).Msg("migration preflight: ping failed")
		out.Status = storage.MigrationPreflightUnreadable
		out.Detail = fmt.Sprintf("cannot reach %s database: %v", engine, err)
		return out
	}

	if _, err := sqldb.ExecContext(ctx, `PRAGMA foreign_keys = ON;`); err != nil {
		_ = sqldb.Close()
		out.Status = storage.MigrationPreflightUnreadable
		out.Detail = fmt.Sprintf("%s: foreign_keys pragma: %v", engine, err)
		return out
	}

	db := bun.NewDB(sqldb, sqlitedialect.New())
	defer func() { _ = db.Close() }()

	var hasEvents bool
	if err := db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'events')`,
	).Scan(&hasEvents); err != nil {
		out.Status = storage.MigrationPreflightUnreadable
		out.Detail = fmt.Sprintf("%s: check events table: %v", engine, err)
		return out
	}
	out.HasEventsTable = hasEvents

	var userVer int
	row := db.QueryRowContext(ctx, "PRAGMA user_version")
	if err := row.Scan(&userVer); err != nil {
		out.Status = storage.MigrationPreflightUnreadable
		out.Detail = fmt.Sprintf("%s: read user_version: %v", engine, err)
		return out
	}
	out.HasVersionTable = true

	if !hasEvents && userVer == 0 {
		out.Status = storage.MigrationPreflightEmpty
		out.Detail = "no events table and user_version 0; opening this target will apply a fresh schema"
		return out
	}
	if hasEvents && userVer == 0 {
		out.Status = storage.MigrationPreflightUnreadable
		out.Detail = "events table exists but user_version is 0; schema state is ambiguous"
		return out
	}

	out.ReportedVersion = &userVer
	switch {
	case userVer > exp:
		out.Status = storage.MigrationPreflightAhead
		out.Detail = fmt.Sprintf("database user_version %d is newer than this binary (%d); use a newer Congee release", userVer, exp)
	case userVer < exp:
		out.Status = storage.MigrationPreflightBehind
		out.Detail = fmt.Sprintf("database is at user_version %d; this binary expects %d. Continuing will run schema migrations on the target, then copy data", userVer, exp)
	default:
		out.Status = storage.MigrationPreflightCurrent
		out.Detail = "schema version matches this binary"
	}
	return out
}

// MainFilePath resolves the on-disk path from a file: DSN.
func MainFilePath(rawDSN string) (string, error) {
	return sqlitewriter.ResolveMainFilePath(rawDSN)
}

// DefaultSQLitePreflightConfig returns preflight settings for modernc/sqliteshim.
func DefaultSQLitePreflightConfig(dsn string, log zerolog.Logger) PreflightConfig {
	return PreflightConfig{
		Engine:       "sqlite",
		DSN:          dsn,
		Log:          log,
		NormalizeDSN: sqlitewriter.NormalizeDSN,
	}
}

// DefaultTursoPreflightConfig returns preflight settings for go-libsql.
func DefaultTursoPreflightConfig(dsn string, log zerolog.Logger) PreflightConfig {
	return PreflightConfig{
		Engine:       "turso",
		DSN:          dsn,
		Log:          log,
		HasDriver:    sqlitewriter.HasLibsqlDriver,
		DriverName:   "libsql",
		NormalizeDSN: sqlitewriter.NormalizeLibsqlDSN,
	}
}
