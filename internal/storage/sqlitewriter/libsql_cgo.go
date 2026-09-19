//go:build cgo

package sqlitewriter

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog"
	_ "github.com/tursodatabase/go-libsql"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
)

const libsqlDriverName = "libsql"

const libsqlOpenAttempts = 15
const libsqlOpenRetry = 50 * time.Millisecond

// HasLibsqlDriver reports whether the go-libsql driver is linked (CGO build).
func HasLibsqlDriver() bool { return true }

// NormalizeLibsqlDSN returns a file: DSN for go-libsql local databases.
func NormalizeLibsqlDSN(dsn string) string {
	return NormalizeDSN(dsn)
}

func isLibsqlLocked(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "database is locked") || strings.Contains(s, "sqlite_busy")
}

// OpenLibsqlHandles opens a local libSQL file, applies WAL pragmas, and returns sql.DB + bun.DB.
func OpenLibsqlHandles(ctx context.Context, dsn string, log zerolog.Logger) (*sql.DB, *bun.DB, error) {
	var last error
	for i := 0; i < libsqlOpenAttempts; i++ {
		sqldb, db, err := openLibsqlHandlesOnce(ctx, dsn, log)
		if err == nil {
			return sqldb, db, nil
		}
		last = err
		if !isLibsqlLocked(err) || ctx.Err() != nil {
			return nil, nil, err
		}
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-time.After(libsqlOpenRetry):
		}
	}
	return nil, nil, last
}

func openLibsqlHandlesOnce(ctx context.Context, dsn string, log zerolog.Logger) (*sql.DB, *bun.DB, error) {
	norm := NormalizeLibsqlDSN(dsn)
	sqldb, err := sql.Open(libsqlDriverName, norm)
	if err != nil {
		return nil, nil, fmt.Errorf("sql.Open libsql: %w", err)
	}
	sqldb.SetMaxOpenConns(1)
	sqldb.SetMaxIdleConns(1)

	if err := sqldb.PingContext(ctx); err != nil {
		_ = sqldb.Close()
		return nil, nil, fmt.Errorf("ping: %w", err)
	}
	for _, stmt := range []struct {
		sql string
		msg string
	}{
		{`PRAGMA busy_timeout = 5000;`, "busy_timeout"},
		{`PRAGMA foreign_keys = ON;`, "foreign_keys"},
		{`PRAGMA journal_mode = WAL;`, "journal_mode"},
	} {
		if err := ExecSQL(ctx, sqldb, stmt.sql); err != nil {
			_ = sqldb.Close()
			if log.GetLevel() <= zerolog.DebugLevel {
				log.Debug().Err(err).Str("pragma", stmt.msg).Msg("libsql reconnect pragma failed")
			}
			return nil, nil, fmt.Errorf("pragma %s: %w", stmt.msg, err)
		}
	}
	return sqldb, bun.NewDB(sqldb, sqlitedialect.New()), nil
}
