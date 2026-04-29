package sqlite

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
)

func TestSQLiteCRUD(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "t.db")
	st, err := Open(ctx, path, nil)
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
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

func TestSQLiteReplaceableKind0(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "r.db")
	st, err := Open(ctx, path, nil)
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
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

func TestSQLiteSearchKindsAndQuotedContent(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := Open(ctx, filepath.Join(dir, "search.db"), nil)
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
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

func TestSQLiteAddressable(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "a.db")
	st, err := Open(ctx, path, nil)
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
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

func TestSQLiteAuditAndChangelog(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := Open(ctx, filepath.Join(dir, "audit.db"), nil)
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.SaveAuditEntry(ctx, storage.AuditEntry{CreatedAt: 100, Action: "x", Detail: "d", Pubkey: nostrRepeat("p", 64)}); err != nil {
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

func nostrRepeat(c string, n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = c[0]
	}
	return string(b)
}
