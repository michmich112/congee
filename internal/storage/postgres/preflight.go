package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/michmich112/congee/internal/storage"
	"github.com/rs/zerolog"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

// PreflightMigrationTarget inspects a Postgres DSN without running DDL or starting LISTEN.
func PreflightMigrationTarget(ctx context.Context, dsn string, log zerolog.Logger) storage.MigrationTargetPreflight {
	dsn = strings.TrimSpace(dsn)
	exp := CurrentSchemaVersion()
	out := storage.MigrationTargetPreflight{
		ExpectedVersion: exp,
		Detail:          "",
	}
	if dsn == "" {
		out.Status = storage.MigrationPreflightUnreadable
		out.Detail = "postgres dsn is empty"
		return out
	}

	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	if err := sqldb.PingContext(ctx); err != nil {
		_ = sqldb.Close()
		log.Debug().Err(err).Msg("migration preflight: postgres ping failed")
		out.Status = storage.MigrationPreflightUnreadable
		out.Detail = fmt.Sprintf("cannot reach postgres: %v", err)
		return out
	}

	db := bun.NewDB(sqldb, pgdialect.New())
	defer func() { _ = db.Close() }()

	var hasEvents bool
	qEv := `SELECT EXISTS (
		SELECT 1 FROM information_schema.tables
		WHERE table_schema = CURRENT_SCHEMA() AND table_name = 'events'
	)`
	if err := db.QueryRowContext(ctx, qEv).Scan(&hasEvents); err != nil {
		out.Status = storage.MigrationPreflightUnreadable
		out.Detail = fmt.Sprintf("postgres: check events table: %v", err)
		return out
	}
	out.HasEventsTable = hasEvents
	if !hasEvents {
		out.Status = storage.MigrationPreflightEmpty
		out.HasVersionTable = false
		out.Detail = "no events table; opening this target will apply a fresh schema"
		return out
	}

	var hasVerTab bool
	qVer := `SELECT EXISTS (
		SELECT 1 FROM information_schema.tables
		WHERE table_schema = CURRENT_SCHEMA() AND table_name = 'congee_schema_version'
	)`
	if err := db.QueryRowContext(ctx, qVer).Scan(&hasVerTab); err != nil {
		out.Status = storage.MigrationPreflightUnreadable
		out.Detail = fmt.Sprintf("postgres: check schema version table: %v", err)
		return out
	}
	out.HasVersionTable = hasVerTab
	if !hasVerTab {
		out.Status = storage.MigrationPreflightUnreadable
		out.Detail = "events table exists but congee_schema_version is missing; automatic upgrade is not supported for this database"
		return out
	}

	var v int
	err := db.QueryRowContext(ctx, `SELECT version FROM congee_schema_version WHERE id = 1`).Scan(&v)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			out.Status = storage.MigrationPreflightUnreadable
			out.Detail = "congee_schema_version has no row for id=1"
			return out
		}
		out.Status = storage.MigrationPreflightUnreadable
		out.Detail = fmt.Sprintf("postgres: read schema version: %v", err)
		return out
	}
	out.ReportedVersion = &v
	switch {
	case v > exp:
		out.Status = storage.MigrationPreflightAhead
		out.Detail = fmt.Sprintf("database schema version %d is newer than this binary (%d); use a newer Congee release", v, exp)
	case v < exp:
		out.Status = storage.MigrationPreflightBehind
		out.Detail = fmt.Sprintf("database is at schema version %d; this binary expects %d. Continuing will run DDL migrations on the target, then copy data", v, exp)
	default:
		out.Status = storage.MigrationPreflightCurrent
		out.Detail = "schema version matches this binary"
	}
	return out
}
