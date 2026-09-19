package approxrows

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/michmich112/congee/internal/storage/sqlitewriter"
	"github.com/rs/zerolog"
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
	if !sqlitewriter.HasLibsqlDriver() {
		t.Skip("libsql driver not available")
	}
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "approx.db")
	sqldb, _, err := sqlitewriter.OpenLibsqlHandles(ctx, path, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sqldb.Close() }()

	if err := sqlitewriter.ExecSQL(ctx, sqldb, `CREATE TABLE items (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if err := sqlitewriter.ExecSQL(ctx, sqldb, `INSERT INTO items (id) VALUES (1), (2), (3)`); err != nil {
		t.Fatal(err)
	}

	n, err := SQLiteTable(ctx, sqldb, "items")
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("before ANALYZE: got %d want 0", n)
	}

	if err := sqlitewriter.ExecSQL(ctx, sqldb, `ANALYZE items`); err != nil {
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
