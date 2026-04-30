package postgres

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
)

func testPostgresDSN(t *testing.T) string {
	t.Helper()
	d := strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
	if d == "" {
		t.Skip("set TEST_POSTGRES_DSN to run postgres tests (e.g. postgres://user:pass@127.0.0.1:5432/congee?sslmode=disable)")
	}
	return d
}

func nostrRepeat(c byte, n int) string {
	return strings.Repeat(string(c), n)
}

func TestPostgresCRUD(t *testing.T) {
	ctx := context.Background()
	dsn := testPostgresDSN(t)
	t.Setenv("CONGEE_INSTANCE_ID", "test-crud")
	st, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	ev := &nostr.Event{
		ID:        nostrRepeat('a', 64),
		PubKey:    nostrRepeat('b', 64),
		CreatedAt: 10,
		Kind:      1,
		Tags:      [][]string{{"e", nostrRepeat('c', 64)}},
		Content:   "hello",
		Sig:       nostrRepeat('d', 128),
	}
	if err := st.SaveEvent(ctx, ev); err != nil {
		t.Fatal(err)
	}
	f := nostr.Filter{IDs: []string{ev.ID}}
	out, err := st.QueryEvents(ctx, []nostr.Filter{f})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Content != "hello" {
		t.Fatalf("query: %+v", out)
	}
	n, err := st.CountEvents(ctx, []nostr.Filter{f})
	if err != nil || n != 1 {
		t.Fatalf("count: %d %v", n, err)
	}
	found, err := st.SearchEvents(ctx, "hello", nostr.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].ID != ev.ID {
		t.Fatalf("search: %+v", found)
	}
	if empty, err := st.SearchEvents(ctx, "", nostr.Filter{}); err != nil || len(empty) != 0 {
		t.Fatalf("empty search: %v %d", err, len(empty))
	}
	if err := st.DeleteEvent(ctx, ev.ID); err != nil {
		t.Fatal(err)
	}
	out2, _ := st.QueryEvents(ctx, []nostr.Filter{f})
	if len(out2) != 0 {
		t.Fatalf("after delete: %d", len(out2))
	}
}

func TestPostgresSearchWithKindFilter(t *testing.T) {
	ctx := context.Background()
	dsn := testPostgresDSN(t)
	t.Setenv("CONGEE_INSTANCE_ID", "test-search")
	st, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	pk := nostrRepeat('f', 64)
	sig := nostrRepeat('g', 128)
	ev := &nostr.Event{
		ID:        nostrRepeat('h', 64),
		PubKey:    pk,
		CreatedAt: 200,
		Kind:      1,
		Tags:      nil,
		Content:   "unique postgres search token xyz",
		Sig:       sig,
	}
	if err := st.SaveEvent(ctx, ev); err != nil {
		t.Fatal(err)
	}
	q := "xyz"
	f := nostr.Filter{Kinds: []int{1}, Search: &q}
	out, err := st.SearchEvents(ctx, f.SearchText(), f.WithoutSearch())
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].ID != ev.ID {
		t.Fatalf("postgres search: %+v", out)
	}
}

func TestPostgresNotifierForeignOrigin(t *testing.T) {
	ctx := context.Background()
	dsn := testPostgresDSN(t)

	t.Setenv("CONGEE_INSTANCE_ID", "relay-a")
	sa, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer sa.Close()

	t.Setenv("CONGEE_INSTANCE_ID", "relay-b")
	sb, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer sb.Close()

	ch := sb.Notifier().Listen()
	time.Sleep(300 * time.Millisecond)

	ev := &nostr.Event{
		ID:        nostrRepeat('e', 64),
		PubKey:    nostrRepeat('f', 64),
		CreatedAt: 99,
		Kind:      1,
		Tags:      nil,
		Content:   "cross",
		Sig:       nostrRepeat('0', 128),
	}
	if err := sa.SaveEvent(ctx, ev); err != nil {
		t.Fatal(err)
	}

	select {
	case id := <-ch:
		if id != ev.ID {
			t.Fatalf("got id %q want %q", id, ev.ID)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for NOTIFY")
	}
}

func TestPostgresAuditLogCountAndPagination(t *testing.T) {
	ctx := context.Background()
	dsn := testPostgresDSN(t)
	t.Setenv("CONGEE_INSTANCE_ID", "test-audit-page")
	st, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// Unique action so concurrent test runs do not count unrelated rows.
	action := "pg_audit_pagination_test"
	if _, err := st.db.NewDelete().Model((*storage.AuditLogRow)(nil)).Where("action = ?", action).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 15; i++ {
		if err := st.SaveAuditEntry(ctx, storage.AuditEntry{
			CreatedAt: int64(8000 - i),
			Action:    action,
			Detail:    fmt.Sprintf("event_id=%s conn_id=c stored=true kind=%d", nostrRepeat('e', 64), i%3),
			Pubkey:    nostrRepeat('p', 64),
		}); err != nil {
			t.Fatal(err)
		}
	}
	qBase := storage.AuditQuery{Action: action}
	total, err := st.CountAuditLog(ctx, qBase)
	if err != nil || total != 15 {
		t.Fatalf("CountAuditLog: want 15, got %d %v", total, err)
	}
	page0, err := st.QueryAuditLog(ctx, storage.AuditQuery{Action: action, Limit: 6, Offset: 0})
	if err != nil || len(page0) != 6 {
		t.Fatalf("page0: len=%d %v", len(page0), err)
	}
	if page0[0].CreatedAt != 8000 || page0[5].CreatedAt != 7995 {
		t.Fatalf("page0 window: first=%d last=%d", page0[0].CreatedAt, page0[5].CreatedAt)
	}
	page1, err := st.QueryAuditLog(ctx, storage.AuditQuery{Action: action, Limit: 6, Offset: 6})
	if err != nil || len(page1) != 6 {
		t.Fatalf("page1: len=%d %v", len(page1), err)
	}
	if page1[0].CreatedAt != 7994 {
		t.Fatalf("page1 first: want 7994, got %d", page1[0].CreatedAt)
	}
	total2, err := st.CountAuditLog(ctx, storage.AuditQuery{Action: action, Limit: 6, Offset: 6})
	if err != nil || total2 != 15 {
		t.Fatalf("CountAuditLog ignores limit/offset: want 15, got %d %v", total2, err)
	}
	k0 := 0
	nKind0, err := st.CountAuditLog(ctx, storage.AuditQuery{Action: action, Kind: &k0})
	if err != nil || nKind0 != 5 {
		t.Fatalf("CountAuditLog kind=0: want 5, got %d %v", nKind0, err)
	}
	if _, err := st.db.NewDelete().Model((*storage.AuditLogRow)(nil)).Where("action = ?", action).Exec(ctx); err != nil {
		t.Fatal(err)
	}
}
