// Package approxrows reads fast approximate table row counts from SQLite and PostgreSQL statistics.
package approxrows

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
)

// Querier runs a single-row query (satisfied by *sql.DB, *bun.DB, etc.).
type Querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// SQLiteTable returns the sqlite_stat1 row estimate for table (requires ANALYZE).
// Returns 0 when no statistics row exists yet.
func SQLiteTable(ctx context.Context, q Querier, table string) (int64, error) {
	var stat sql.NullString
	// Table-wide stats use idx IS NULL; TEXT PRIMARY KEY tables only have per-index rows (often sqlite_autoindex_*).
	err := q.QueryRowContext(ctx, `
SELECT stat FROM sqlite_stat1
WHERE tbl = ?
ORDER BY
  CASE
    WHEN idx IS NULL OR idx = '' THEN 0
    WHEN idx = ? THEN 1
    WHEN idx LIKE 'sqlite_autoindex_%' THEN 2
    ELSE 3
  END
LIMIT 1
`, table, table).Scan(&stat)
	if err == sql.ErrNoRows || !stat.Valid {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("sqlite stat1 %q: %w", table, err)
	}
	return parseSQLiteStatFirstInt(stat.String)
}

func parseSQLiteStatFirstInt(stat string) (int64, error) {
	stat = strings.TrimSpace(stat)
	if stat == "" {
		return 0, nil
	}
	end := strings.IndexByte(stat, ' ')
	if end < 0 {
		n, err := strconv.ParseInt(stat, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("sqlite stat1 parse %q: %w", stat, err)
		}
		return n, nil
	}
	n, err := strconv.ParseInt(stat[:end], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("sqlite stat1 parse %q: %w", stat, err)
	}
	return n, nil
}

// PostgresTable returns n_live_tup when available, otherwise reltuples from pg_class.
// Estimates are refreshed by autovacuum / ANALYZE.
func PostgresTable(ctx context.Context, q Querier, table string) (int64, error) {
	var n sql.NullInt64
	err := q.QueryRowContext(ctx, `
SELECT COALESCE(s.n_live_tup, c.reltuples, 0)::bigint
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
LEFT JOIN pg_stat_user_tables s ON s.relid = c.oid
WHERE n.nspname = current_schema() AND c.relname = $1 AND c.relkind = 'r'
`, table).Scan(&n)
	if err == sql.ErrNoRows || !n.Valid {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("postgres table stats %q: %w", table, err)
	}
	return n.Int64, nil
}
