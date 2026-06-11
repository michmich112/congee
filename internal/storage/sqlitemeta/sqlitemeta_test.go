package sqlitemeta_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michmich112/congee/internal/storage"
	"github.com/michmich112/congee/internal/storage/sqlitemeta"
	"github.com/rs/zerolog"
)

func TestMetaAuditAndChangelog(t *testing.T) {
	ctx := context.Background()
	st, err := sqlitemeta.Open(ctx, filepath.Join(t.TempDir(), "meta.db"), zerolog.Nop())
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	pk := nostrRepeat("p", 64)
	if err := st.SaveAuditEntry(ctx, storage.AuditEntry{CreatedAt: 100, Action: "x", Detail: "d", Pubkey: pk}); err != nil {
		t.Fatal(err)
	}
	nAudit, err := st.CountAuditLog(ctx, storage.AuditQuery{})
	if err != nil || nAudit != 1 {
		t.Fatalf("CountAuditLog: want 1, got %d %v", nAudit, err)
	}
	rows, err := st.QueryAuditLog(ctx, storage.AuditQuery{Limit: 10})
	if err != nil || len(rows) != 1 {
		t.Fatalf("audit: %+v %v", rows, err)
	}
	n, err := st.PurgeAuditLog(ctx, 200)
	if err != nil || n != 1 {
		t.Fatalf("purge: %d %v", n, err)
	}
	if err := st.SaveConfigChange(ctx, storage.ConfigChange{CreatedAt: 1, Summary: "s", JSONDiff: "{}"}); err != nil {
		t.Fatal(err)
	}
	ch, err := st.QueryConfigChangelog(ctx, 5)
	if err != nil || len(ch) != 1 {
		t.Fatalf("changelog: %+v %v", ch, err)
	}
}

func TestMetaListDistinctAuditKinds(t *testing.T) {
	ctx := context.Background()
	st, err := sqlitemeta.Open(ctx, filepath.Join(t.TempDir(), "kinds.db"), zerolog.Nop())
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	id := nostrRepeat("e", 64)
	for _, k := range []int{9, 42, 42, 9} {
		detail := fmt.Sprintf("event_id=%s conn_id=ab12cd34 kind=%d", id, k)
		if err := st.SaveAuditEntry(ctx, storage.AuditEntry{CreatedAt: 1, Action: "event_stored", Detail: detail, Pubkey: nostrRepeat("p", 64)}); err != nil {
			t.Fatal(err)
		}
	}
	kinds, err := st.ListDistinctAuditKinds(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(kinds) != 2 || kinds[0] != 9 || kinds[1] != 42 {
		t.Fatalf("want [9 42], got %v", kinds)
	}
}

func nostrRepeat(c string, n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = c[0]
	}
	return string(b)
}
