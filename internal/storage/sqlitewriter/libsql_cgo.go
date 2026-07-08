//go:build cgo

package sqlitewriter

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/rs/zerolog"
	_ "github.com/tursodatabase/go-libsql"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
)

const libsqlDriverName = "libsql"

// HasLibsqlDriver reports whether the go-libsql driver is linked (CGO build).
func HasLibsqlDriver() bool { return true }

// NormalizeLibsqlDSN returns a file: DSN for go-libsql local databases.
func NormalizeLibsqlDSN(dsn string) string {
	return NormalizeDSN(dsn)
}

// OpenLibsqlHandles opens a local libSQL file, applies WAL pragmas, and returns sql.DB + bun.DB.
func OpenLibsqlHandles(ctx context.Context, dsn string, log zerolog.Logger) (*sql.DB, *bun.DB, error) {
	norm := NormalizeLibsqlDSN(dsn)
	sqldb, err := sql.Open(libsqlDriverName, norm)
	if err != nil {
		return nil, nil, fmt.Errorf("sql.Open libsql: %w", err)
	}
	sqldb.SetMaxOpenConns(8)
	sqldb.SetMaxIdleConns(8)

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
		if err := execLibsqlPragma(ctx, sqldb, stmt.sql); err != nil {
			_ = sqldb.Close()
			if log.GetLevel() <= zerolog.DebugLevel {
				log.Debug().Err(err).Str("pragma", stmt.msg).Msg("libsql reconnect pragma failed")
			}
			return nil, nil, fmt.Errorf("pragma %s: %w", stmt.msg, err)
		}
	}
	return sqldb, bun.NewDB(sqldb, sqlitedialect.New()), nil
}

func execLibsqlPragma(ctx context.Context, sqldb *sql.DB, stmt string) error {
	if _, err := sqldb.ExecContext(ctx, stmt); err == nil {
		return nil
	} else if !strings.Contains(err.Error(), "Execute returned rows") {
		return err
	}
	var ignored string
	if err := sqldb.QueryRowContext(ctx, stmt).Scan(&ignored); err != nil && err != sql.ErrNoRows {
		return err
	}
	return nil
}
