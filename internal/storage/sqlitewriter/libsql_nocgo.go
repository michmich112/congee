//go:build !cgo

package sqlitewriter

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/rs/zerolog"
	"github.com/uptrace/bun"
)

// HasLibsqlDriver reports whether the go-libsql driver is linked.
func HasLibsqlDriver() bool { return false }

// NormalizeLibsqlDSN returns a file: DSN for go-libsql local databases.
func NormalizeLibsqlDSN(dsn string) string {
	return NormalizeDSN(dsn)
}

// OpenLibsqlHandles is unavailable without CGO.
func OpenLibsqlHandles(ctx context.Context, dsn string, log zerolog.Logger) (*sql.DB, *bun.DB, error) {
	_ = ctx
	_ = dsn
	_ = log
	return nil, nil, fmt.Errorf("libsql: driver not available (build with CGO_ENABLED=1)")
}
