package postgres

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/rs/zerolog"
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
	st, err := Open(ctx, dsn, "test-crud", zerolog.Nop())
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

// TestPostgresManyTagsJSONBRoundTrip guards against Bun double-encoding full_json when a
// Go string is combined with type:jsonb (JSON array must round-trip for kind-5-style tags).
func TestPostgresManyTagsJSONBRoundTrip(t *testing.T) {
	ctx := context.Background()
	dsn := testPostgresDSN(t)
	t.Setenv("CONGEE_INSTANCE_ID", "test-tag-jsonb")
	st, err := Open(ctx, dsn, "test-tag-jsonb", zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	const nTags = 48
	tags := make([][]string, 0, nTags)
	for i := range nTags {
		tags = append(tags, []string{"e", fmt.Sprintf("%064x", i+1)})
	}
	ev := &nostr.Event{
		ID:        nostrRepeat('3', 64),
		PubKey:    nostrRepeat('4', 64),
		CreatedAt: 100,
		Kind:      5,
		Tags:      tags,
		Content:   "delete",
		Sig:       nostrRepeat('5', 128),
	}
	if err := st.SaveEvent(ctx, ev); err != nil {
		t.Fatal(err)
	}
	out, err := st.QueryEvents(ctx, []nostr.Filter{{IDs: []string{ev.ID}}})
	if err != nil {
		t.Fatalf("QueryEvents: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("want 1 event, got %d", len(out))
	}
	if !reflect.DeepEqual(out[0].Tags, tags) {
		t.Fatalf("tags round-trip: want %d tags first=%#v; got %d tags first=%#v",
			len(tags), tags[0], len(out[0].Tags), out[0].Tags[0])
	}
	if err := st.DeleteEvent(ctx, ev.ID); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresSearchWithKindFilter(t *testing.T) {
	ctx := context.Background()
	dsn := testPostgresDSN(t)
	t.Setenv("CONGEE_INSTANCE_ID", "test-search")
	st, err := Open(ctx, dsn, "test-search", zerolog.Nop())
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
	sa, err := Open(ctx, dsn, "relay-a", zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer sa.Close()

	t.Setenv("CONGEE_INSTANCE_ID", "relay-b")
	sb, err := Open(ctx, dsn, "relay-b", zerolog.Nop())
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

func TestPostgresFilterLimit(t *testing.T) {
	ctx := context.Background()
	dsn := testPostgresDSN(t)
	t.Setenv("CONGEE_INSTANCE_ID", "test-filter-limit")
	st, err := Open(ctx, dsn, "test-filter-limit", zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	sig := nostrRepeat('s', 128)
	for i := 0; i < 6; i++ {
		id := fmt.Sprintf("%064d", i)
		ev := &nostr.Event{
			ID: id, PubKey: nostrRepeat('b', 64), CreatedAt: int64(6 - i),
			Kind: 1, Tags: nil, Content: "test", Sig: sig,
		}
		if err := st.SaveEvent(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}

	f := nostr.Filter{Kinds: []int{1}}
	out, err := st.QueryEvents(ctx, []nostr.Filter{f})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 6 {
		t.Fatalf("want 6 with nil limit, got %d", len(out))
	}

	lim := 3
	f = nostr.Filter{Kinds: []int{1}, Limit: &lim}
	out, err = st.QueryEvents(ctx, []nostr.Filter{f})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 3 {
		t.Fatalf("want 3 with limit=3, got %d", len(out))
	}

	zero := 0
	f = nostr.Filter{Kinds: []int{1}, Limit: &zero}
	out, err = st.QueryEvents(ctx, []nostr.Filter{f})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 6 {
		t.Fatalf("want 6 with limit=0 (unlimited), got %d", len(out))
	}

	neg := -1
	f = nostr.Filter{Kinds: []int{1}, Limit: &neg}
	out, err = st.QueryEvents(ctx, []nostr.Filter{f})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 6 {
		t.Fatalf("want 6 with limit=-1 (unlimited), got %d", len(out))
	}
}
