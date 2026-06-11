package sqlitemeta

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func TestAnalyzeStatsTablesPopulatesStat1(t *testing.T) {
	if !sqliteshim.HasDriver() {
		t.Skip("sqliteshim not available")
	}
	t.Parallel()
	ctx := context.Background()
	st, err := Open(ctx, filepath.Join(t.TempDir(), "meta-analyze.db"), zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()

	if _, err := st.db().ExecContext(ctx, `INSERT INTO audit_log (created_at, action, detail, pubkey) VALUES (1,'test','d','pk')`); err != nil {
		t.Fatal(err)
	}
	if err := st.AnalyzeStatsTables(ctx); err != nil {
		t.Fatal(err)
	}
	var statRows int
	if err := st.db().QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_stat1 WHERE tbl = 'audit_log'`).Scan(&statRows); err != nil {
		t.Fatal(err)
	}
	if statRows == 0 {
		t.Fatal("sqlite_stat1 empty after ANALYZE audit_log")
	}
}
