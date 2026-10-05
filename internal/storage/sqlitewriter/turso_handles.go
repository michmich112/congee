package sqlitewriter

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/rs/zerolog"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	_ "turso.tech/database/tursogo"
)

const (
	tursoDriverName = "turso"
	// tursoReadPool is the connection cap for concurrent readers. Writes stay on
	// the single writer queue. Each pooled connection is used by one goroutine.
	tursoReadPool = 8
)

// HasTursoDriver reports whether the tursogo database/sql driver is linked.
func HasTursoDriver() bool { return true }

// TursoDSN returns a tursogo DSN for a filesystem path. index_method enables the
// Turso full-text index used by NIP-50 search.
func TursoDSN(path string) string {
	return path + "?experimental=index_method"
}

// OpenTursoHandles opens a local database with tursogo, WAL, and a read pool.
func OpenTursoHandles(ctx context.Context, dsn string, log zerolog.Logger) (*sql.DB, *bun.DB, error) {
	path, err := ResolveMainFilePath(dsn)
	if err != nil {
		return nil, nil, err
	}
	sqldb, err := sql.Open(tursoDriverName, TursoDSN(path))
	if err != nil {
		return nil, nil, fmt.Errorf("sql.Open turso: %w", err)
	}
	sqldb.SetMaxOpenConns(tursoReadPool)
	sqldb.SetMaxIdleConns(tursoReadPool)
	if err := sqldb.PingContext(ctx); err != nil {
		_ = sqldb.Close()
		return nil, nil, fmt.Errorf("ping: %w", err)
	}
	var mode string
	if err := sqldb.QueryRowContext(ctx, `PRAGMA journal_mode = WAL`).Scan(&mode); err != nil {
		_ = sqldb.Close()
		return nil, nil, fmt.Errorf("pragma journal_mode: %w", err)
	}
	if !strings.EqualFold(mode, "wal") {
		_ = sqldb.Close()
		return nil, nil, fmt.Errorf("journal_mode %q", mode)
	}
	for _, stmt := range []struct {
		sql string
		msg string
	}{
		{`PRAGMA busy_timeout = 5000;`, "busy_timeout"},
		{`PRAGMA foreign_keys = ON;`, "foreign_keys"},
		{`PRAGMA analysis_limit = 1000;`, "analysis_limit"},
	} {
		if err := ExecSQL(ctx, sqldb, stmt.sql); err != nil {
			_ = sqldb.Close()
			if log.GetLevel() <= zerolog.DebugLevel {
				log.Debug().Err(err).Str("pragma", stmt.msg).Msg("turso pragma failed")
			}
			return nil, nil, fmt.Errorf("pragma %s: %w", stmt.msg, err)
		}
	}
	return sqldb, bun.NewDB(sqldb, sqlitedialect.New()), nil
}
