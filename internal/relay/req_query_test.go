package relay

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage/sqlite"
	"github.com/rs/zerolog"
)

func TestQueryInitialREQEvents_SearchORWithKinds(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := sqlite.Open(ctx, filepath.Join(dir, "q.db"), nil, zerolog.Nop())
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	pk := strings.Repeat("b", 64)
	sig := strings.Repeat("s", 128)
	ev1 := &nostr.Event{
		ID: strings.Repeat("1", 64), PubKey: pk, CreatedAt: 2, Kind: 1,
		Tags: nil, Content: "alpha beta gamma", Sig: sig,
	}
	ev2 := &nostr.Event{
		ID: strings.Repeat("2", 64), PubKey: pk, CreatedAt: 1, Kind: 7,
		Tags: nil, Content: "alpha ignored", Sig: sig,
	}
	for _, e := range []*nostr.Event{ev1, ev2} {
		if err := st.SaveEvent(ctx, e); err != nil {
			t.Fatal(err)
		}
	}

	q := "alpha"
	fSearch := nostr.Filter{Search: &q, Kinds: []int{1}}
	fKind := nostr.Filter{Kinds: []int{7}}
	out, err := queryInitialREQEvents(ctx, st, []nostr.Filter{fSearch, fKind}, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("want 2 events (search+kind OR), got %d", len(out))
	}
}

func TestQueryInitialREQEvents_SearchDisabledSkipsSearchBranch(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := sqlite.Open(ctx, filepath.Join(dir, "q2.db"), nil, zerolog.Nop())
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	q := "nope"
	f := nostr.Filter{Search: &q}
	out, err := queryInitialREQEvents(ctx, st, []nostr.Filter{f}, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatalf("expected empty when search branch skipped, got %d", len(out))
	}
}

func createTestEvents(t *testing.T, st *sqlite.Store, ctx context.Context, n int) {
	t.Helper()
	pk := strings.Repeat("b", 64)
	sig := strings.Repeat("s", 128)
	for i := 0; i < n; i++ {
		id := strings.Repeat(string(rune('a'+(i%26))), 64)
		if len(id) < 64 {
			id = id + strings.Repeat("x", 64-len(id))
		}
		ev := &nostr.Event{
			ID:        id,
			PubKey:    pk,
			CreatedAt: int64(n - i),
			Kind:      1,
			Tags:      nil,
			Content:   "test event",
			Sig:       sig,
		}
		if err := st.SaveEvent(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
}

func TestQueryInitialREQEvents_DefaultLimitApplies(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := sqlite.Open(ctx, filepath.Join(dir, "q3.db"), nil, zerolog.Nop())
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	createTestEvents(t, st, ctx, 6)

	f := nostr.Filter{Kinds: []int{1}}
	out, err := queryInitialREQEvents(ctx, st, []nostr.Filter{f}, false, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 3 {
		t.Fatalf("want 3 events (config default limit 3), got %d", len(out))
	}
}

func TestQueryInitialREQEvents_ExplicitLimitOverridesDefault(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := sqlite.Open(ctx, filepath.Join(dir, "q4.db"), nil, zerolog.Nop())
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	createTestEvents(t, st, ctx, 6)

	lim := 2
	f := nostr.Filter{Kinds: []int{1}, Limit: &lim}
	out, err := queryInitialREQEvents(ctx, st, []nostr.Filter{f}, false, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("want 2 events (explicit filter limit overrides config default), got %d", len(out))
	}
}

func TestQueryInitialREQEvents_ZeroDefaultIsUnlimited(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := sqlite.Open(ctx, filepath.Join(dir, "q5.db"), nil, zerolog.Nop())
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	createTestEvents(t, st, ctx, 6)

	f := nostr.Filter{Kinds: []int{1}}
	out, err := queryInitialREQEvents(ctx, st, []nostr.Filter{f}, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 6 {
		t.Fatalf("want 6 events (zero default = unlimited), got %d", len(out))
	}
}
