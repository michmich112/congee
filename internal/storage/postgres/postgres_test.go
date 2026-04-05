package postgres

import (
	"context"
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
	if _, err := st.SearchEvents(ctx, "foo", 10); err != storage.ErrSearchNotImplemented {
		t.Fatalf("search: %v", err)
	}
	if err := st.DeleteEvent(ctx, ev.ID); err != nil {
		t.Fatal(err)
	}
	out2, _ := st.QueryEvents(ctx, []nostr.Filter{f})
	if len(out2) != 0 {
		t.Fatalf("after delete: %d", len(out2))
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
