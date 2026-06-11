package approxrows

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/uptrace/bun/driver/sqliteshim"
)

func TestParseSQLiteStatFirstInt(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want int64
	}{
		{"", 0},
		{"   ", 0},
		{"42", 42},
		{"1000 50 200", 1000},
		{"  7 1 2  ", 7},
	}
	for _, tc := range cases {
		got, err := parseSQLiteStatFirstInt(tc.in)
		if err != nil {
			t.Fatalf("parseSQLiteStatFirstInt(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("parseSQLiteStatFirstInt(%q) = %d want %d", tc.in, got, tc.want)
		}
	}
}

func TestSQLiteTableRequiresAnalyze(t *testing.T) {
	if !sqliteshim.HasDriver() {
		t.Skip("sqliteshim not available")
	}
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "approx.db")
	sqldb, err := sql.Open(sqliteshim.ShimName, "file:"+path+"?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sqldb.Close() }()

	if _, err := sqldb.ExecContext(ctx, `CREATE TABLE items (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if _, err := sqldb.ExecContext(ctx, `INSERT INTO items (id) VALUES (1), (2), (3)`); err != nil {
		t.Fatal(err)
	}

	n, err := SQLiteTable(ctx, sqldb, "items")
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("before ANALYZE: got %d want 0", n)
	}

	if _, err := sqldb.ExecContext(ctx, `ANALYZE items`); err != nil {
		t.Fatal(err)
	}
	n, err = SQLiteTable(ctx, sqldb, "items")
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("after ANALYZE: got %d want 3", n)
	}
}
