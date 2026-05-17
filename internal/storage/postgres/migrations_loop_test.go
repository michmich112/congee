package postgres

import (
	"context"
	"database/sql"
	"testing"

	"github.com/rs/zerolog"
	"github.com/uptrace/bun/driver/pgdriver"
)

// TestRunMigrationsLoopsFakeV3ToV6OnV6Schema leaves the physical schema at v6 but sets congee_schema_version
// to 3. One Open must apply v3→v4, v4→v5, and v5→v6 in sequence and end at version 6.
func TestRunMigrationsLoopsFakeV3ToV6OnV6Schema(t *testing.T) {
	ctx := context.Background()
	dsn := testPostgresDSN(t)
	const instanceID = "test-pg-multistep-migrate"

	st, err := Open(ctx, dsn, instanceID, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	if err := sqldb.PingContext(ctx); err != nil {
		_ = sqldb.Close()
		t.Fatal(err)
	}
	if _, err := sqldb.ExecContext(ctx, `UPDATE congee_schema_version SET version = 3 WHERE id = 1`); err != nil {
		_ = sqldb.Close()
		t.Fatal(err)
	}
	if err := sqldb.Close(); err != nil {
		t.Fatal(err)
	}

	st2, err := Open(ctx, dsn, instanceID, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st2.Close() }()

	var v int
	if err := st2.db.QueryRowContext(ctx, `SELECT version FROM congee_schema_version WHERE id = 1`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != schemaVersion {
		t.Fatalf("schema version after multi-step migrate: got %d want %d", v, schemaVersion)
	}
}
