package sqlitewriter

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/rs/zerolog"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"

	"github.com/uptrace/bun/driver/sqliteshim"
)

// NormalizeDSN returns a sqliteshim file DSN with shared cache.
func NormalizeDSN(dsn string) string {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return "file:congee.db?cache=shared"
	}
	if strings.HasPrefix(dsn, "file:") {
		return dsn
	}
	return "file:" + dsn + "?cache=shared"
}

// OpenHandles opens SQLite, applies WAL pragmas, and returns sql.DB + bun.DB.
func OpenHandles(ctx context.Context, dsn string, log zerolog.Logger) (*sql.DB, *bun.DB, error) {
	return openHandles(ctx, dsn, log)
}

func openHandles(ctx context.Context, dsn string, log zerolog.Logger) (*sql.DB, *bun.DB, error) {
	if !sqliteshim.HasDriver() {
		return nil, nil, fmt.Errorf("sqliteshim driver not available for this build target")
	}
	norm := NormalizeDSN(dsn)
	sqldb, err := sql.Open(sqliteshim.ShimName, norm)
	if err != nil {
		return nil, nil, fmt.Errorf("sql.Open: %w", err)
	}
	sqldb.SetMaxOpenConns(64)
	sqldb.SetMaxIdleConns(64)

	if err := sqldb.PingContext(ctx); err != nil {
		_ = sqldb.Close()
		return nil, nil, fmt.Errorf("ping: %w", err)
	}
	for _, stmt := range []struct {
		sql string
		msg string
	}{
		{`PRAGMA foreign_keys = ON;`, "foreign_keys"},
		{`PRAGMA journal_mode = WAL;`, "journal_mode"},
		{`PRAGMA busy_timeout = 5000;`, "busy_timeout"},
	} {
		if _, err := sqldb.ExecContext(ctx, stmt.sql); err != nil {
			_ = sqldb.Close()
			if log.GetLevel() <= zerolog.DebugLevel {
				log.Debug().Err(err).Str("pragma", stmt.msg).Msg("reconnect pragma failed")
			}
			return nil, nil, fmt.Errorf("pragma %s: %w", stmt.msg, err)
		}
	}
	return sqldb, bun.NewDB(sqldb, sqlitedialect.New()), nil
}
