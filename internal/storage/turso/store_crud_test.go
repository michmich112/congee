package turso

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
	"github.com/rs/zerolog"
)

func TestStoreCRUD(t *testing.T) {
	skipNoDriver(t)
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "t.db")
	st, err := Open(ctx, path, nil, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	ev := &nostr.Event{
		ID:        nostrRepeat("a", 64),
		PubKey:    nostrRepeat("b", 64),
		CreatedAt: 10,
		Kind:      1,
		Tags:      [][]string{{"e", nostrRepeat("c", 64)}},
		Content:   "hello",
		Sig:       nostrRepeat("d", 128),
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

func TestReplaceableRevisionOrder(t *testing.T) {
	skipNoDriver(t)
	ctx := context.Background()
	st, err := Open(ctx, filepath.Join(t.TempDir(), "order.db"), nil, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	pk := nostrRepeat("b", 64)
	for _, tc := range []struct {
		kind int
		tags [][]string
	}{
		{kind: 0}, {kind: 30402, tags: [][]string{{"d", "product"}}},
	} {
		suffix := "0"
		if tc.kind != 0 {
			suffix = "1"
		}
		makeEvent := func(id string, created int64) *nostr.Event {
			return &nostr.Event{ID: nostrRepeat(id, 63) + suffix, PubKey: pk, CreatedAt: created,
				Kind: tc.kind, Tags: tc.tags, Content: id, Sig: nostrRepeat("f", 128)}
		}
		for _, ev := range []*nostr.Event{makeEvent("c", 10), makeEvent("b", 10), makeEvent("a", 9), makeEvent("d", 10)} {
			err := st.SaveEvent(ctx, ev)
			if ev.ID[0] == 'a' || ev.ID[0] == 'd' {
				if !errors.Is(err, storage.ErrStaleReplaceable) {
					t.Fatalf("kind %d stale %s: %v", tc.kind, ev.ID, err)
				}
			} else if err != nil {
				t.Fatalf("kind %d save %s: %v", tc.kind, ev.ID, err)
			}
		}
		out, err := st.QueryEvents(ctx, []nostr.Filter{{Authors: []string{pk}, Kinds: []int{tc.kind}}})
		if err != nil || len(out) != 1 || out[0].ID != nostrRepeat("b", 63)+suffix {
			t.Fatalf("kind %d winner: %+v err=%v", tc.kind, out, err)
		}
	}
}

func TestStoreReplaceableKind0(t *testing.T) {
	skipNoDriver(t)
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "r.db")
	st, err := Open(ctx, path, nil, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	pk := nostrRepeat("b", 64)
	ev1 := &nostr.Event{
		ID:        nostrRepeat("1", 64),
		PubKey:    pk,
		CreatedAt: 1,
		Kind:      0,
		Tags:      nil,
		Content:   `{"name":"a"}`,
		Sig:       nostrRepeat("a", 128),
	}
	ev2 := &nostr.Event{
		ID:        nostrRepeat("2", 64),
		PubKey:    pk,
		CreatedAt: 2,
		Kind:      0,
		Tags:      nil,
		Content:   `{"name":"b"}`,
		Sig:       nostrRepeat("c", 128),
	}
	if err := st.SaveEvent(ctx, ev1); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveEvent(ctx, ev2); err != nil {
		t.Fatal(err)
	}
	f := nostr.Filter{Authors: []string{pk}, Kinds: []int{0}}
	out, err := st.QueryEvents(ctx, []nostr.Filter{f})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("want 1 event after replace, got %d", len(out))
	}
	if out[0].ID != ev2.ID {
		t.Fatalf("want latest id %s, got %s", ev2.ID, out[0].ID)
	}
}

func TestStoreSearchKindsAndQuotedContent(t *testing.T) {
	skipNoDriver(t)
	ctx := context.Background()
	dir := t.TempDir()
	st, err := Open(ctx, filepath.Join(dir, "search.db"), nil, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	pk := nostrRepeat("p", 64)
	sig := nostrRepeat("s", 128)
	match := &nostr.Event{
		ID: nostrRepeat("m", 64), PubKey: pk, CreatedAt: 10, Kind: 1,
		Content: `say "hello" world`, Sig: sig,
	}
	other := &nostr.Event{
		ID: nostrRepeat("n", 64), PubKey: pk, CreatedAt: 11, Kind: 1,
		Content: "no match here", Sig: sig,
	}
	wrongKind := &nostr.Event{
		ID: nostrRepeat("k", 64), PubKey: pk, CreatedAt: 12, Kind: 7,
		Content: `say "hello" world`, Sig: sig,
	}
	for _, e := range []*nostr.Event{match, other, wrongKind} {
		if err := st.SaveEvent(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	q := "hello"
	f := nostr.Filter{Kinds: []int{1}, Search: &q}
	found, err := st.SearchEvents(ctx, f.SearchText(), f.WithoutSearch())
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].ID != match.ID {
		t.Fatalf("search+kinds: %+v", found)
	}
}

func TestStoreAddressable(t *testing.T) {
	skipNoDriver(t)
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "a.db")
	st, err := Open(ctx, path, nil, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	pk := nostrRepeat("e", 64)
	ev1 := &nostr.Event{
		ID:        nostrRepeat("3", 64),
		PubKey:    pk,
		CreatedAt: 1,
		Kind:      30023,
		Tags:      [][]string{{"d", "doc1"}},
		Content:   "v1",
		Sig:       nostrRepeat("f", 128),
	}
	ev2 := &nostr.Event{
		ID:        nostrRepeat("4", 64),
		PubKey:    pk,
		CreatedAt: 2,
		Kind:      30023,
		Tags:      [][]string{{"d", "doc1"}},
		Content:   "v2",
		Sig:       nostrRepeat("g", 128),
	}
	evOther := &nostr.Event{
		ID:        nostrRepeat("5", 64),
		PubKey:    pk,
		CreatedAt: 3,
		Kind:      30023,
		Tags:      [][]string{{"d", "doc2"}},
		Content:   "other",
		Sig:       nostrRepeat("h", 128),
	}
	for _, e := range []*nostr.Event{ev1, ev2, evOther} {
		if err := st.SaveEvent(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	f := nostr.Filter{Authors: []string{pk}, Kinds: []int{30023}}
	out, err := st.QueryEvents(ctx, []nostr.Filter{f})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("want 2 addressable docs, got %d", len(out))
	}
}

func TestStoreFilterLimit_NilLimit(t *testing.T) {
	skipNoDriver(t)
	pk := nostrRepeat("b", 64)
	sig := nostrRepeat("s", 128)
	ctx := context.Background()
	dir := t.TempDir()
	st, err := Open(ctx, filepath.Join(dir, "fl.db"), nil, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// Insert 6 events
	for i := 0; i < 6; i++ {
		id := fmt.Sprintf("%064d", i)
		ev := &nostr.Event{
			ID: id, PubKey: pk, CreatedAt: int64(6 - i),
			Kind: 1, Tags: nil, Content: "test", Sig: sig,
		}
		if err := st.SaveEvent(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}

	// No limit set — should return all events
	f := nostr.Filter{Kinds: []int{1}}
	out, err := st.QueryEvents(ctx, []nostr.Filter{f})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 6 {
		t.Fatalf("want 6 with nil limit, got %d", len(out))
	}

	// Explicit positive limit
	lim := 3
	f = nostr.Filter{Kinds: []int{1}, Limit: &lim}
	out, err = st.QueryEvents(ctx, []nostr.Filter{f})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 3 {
		t.Fatalf("want 3 with limit=3, got %d", len(out))
	}

	// Explicit zero limit — should be unlimited
	zero := 0
	f = nostr.Filter{Kinds: []int{1}, Limit: &zero}
	out, err = st.QueryEvents(ctx, []nostr.Filter{f})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 6 {
		t.Fatalf("want 6 with limit=0 (unlimited), got %d", len(out))
	}

	// Negative limit — should be unlimited
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
