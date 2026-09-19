package sqlitewriter

import (
	"context"
	"database/sql"
	"strings"
)

// ExecSQL executes stmt, retrying as QueryRow when libsql reports that the statement returned rows.
func ExecSQL(ctx context.Context, sqldb *sql.DB, stmt string) error {
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
