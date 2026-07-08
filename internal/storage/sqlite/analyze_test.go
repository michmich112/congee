package sqlite

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
	st, err := Open(ctx, filepath.Join(t.TempDir(), "analyze.db"), nil, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()

	if _, err := st.DB().ExecContext(ctx, `INSERT INTO events (id, pubkey, created_at, kind, content, sig) VALUES ('a','b',1,1,'c','d')`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB().ExecContext(ctx, `INSERT INTO event_tags (event_id, pos, name, value, full_json) VALUES ('a',0,'p','b','[]')`); err != nil {
		t.Fatal(err)
	}

	if err := st.AnalyzeStatsTables(ctx); err != nil {
		t.Fatal(err)
	}
	var statRows int
	if err := st.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_stat1 WHERE tbl IN ('events', 'event_tags')`).Scan(&statRows); err != nil {
		t.Fatal(err)
	}
	if statRows == 0 {
		t.Fatal("sqlite_stat1 empty after ANALYZE")
	}
}
