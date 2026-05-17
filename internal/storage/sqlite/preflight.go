package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/michmich112/congee/internal/storage"
	"github.com/rs/zerolog"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"

	"github.com/uptrace/bun/driver/sqliteshim"
)

// PreflightMigrationTarget inspects a SQLite DSN without running migrations or starting the writer loop.
func PreflightMigrationTarget(ctx context.Context, dsn string, log zerolog.Logger) storage.MigrationTargetPreflight {
	exp := CurrentSchemaVersion()
	out := storage.MigrationTargetPreflight{
		ExpectedVersion: exp,
		Detail:          "",
	}
	if !sqliteshim.HasDriver() {
		out.Status = storage.MigrationPreflightUnreadable
		out.Detail = "sqlite driver not available for this build target"
		return out
	}
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		out.Status = storage.MigrationPreflightUnreadable
		out.Detail = "sqlite dsn is empty"
		return out
	}

	sqldb, err := sql.Open(sqliteshim.ShimName, normalizeDSN(dsn))
	if err != nil {
		out.Status = storage.MigrationPreflightUnreadable
		out.Detail = fmt.Sprintf("sqlite: open: %v", err)
		return out
	}

	if err := sqldb.PingContext(ctx); err != nil {
		_ = sqldb.Close()
		log.Debug().Err(err).Msg("migration preflight: sqlite ping failed")
		out.Status = storage.MigrationPreflightUnreadable
		out.Detail = fmt.Sprintf("cannot reach sqlite database: %v", err)
		return out
	}

	if _, err := sqldb.ExecContext(ctx, `PRAGMA foreign_keys = ON;`); err != nil {
		_ = sqldb.Close()
		out.Status = storage.MigrationPreflightUnreadable
		out.Detail = fmt.Sprintf("sqlite: foreign_keys pragma: %v", err)
		return out
	}

	db := bun.NewDB(sqldb, sqlitedialect.New())
	defer func() { _ = db.Close() }()

	var hasEvents bool
	if err := db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'events')`,
	).Scan(&hasEvents); err != nil {
		out.Status = storage.MigrationPreflightUnreadable
		out.Detail = fmt.Sprintf("sqlite: check events table: %v", err)
		return out
	}
	out.HasEventsTable = hasEvents

	var userVer int
	row := db.QueryRowContext(ctx, "PRAGMA user_version")
	if err := row.Scan(&userVer); err != nil {
		out.Status = storage.MigrationPreflightUnreadable
		out.Detail = fmt.Sprintf("sqlite: read user_version: %v", err)
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
