package turso

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
	"github.com/michmich112/congee/internal/storage/sqlitewriter"
	"github.com/rs/zerolog"
)

func TestUpgradeV7FTS5KeepsEvents(t *testing.T) {
	skipNoDriver(t)
	if !sqlitewriter.HasLibsqlDriver() {
		t.Skip("libsql driver not available")
	}
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "events.db")
	writeV7Events(t, ctx, path)

	st, err := Open(ctx, path, nil, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	var uv int
	if err := st.DB().QueryRowContext(ctx, `PRAGMA user_version`).Scan(&uv); err != nil {
		t.Fatal(err)
	}
	if uv != 8 {
		t.Fatalf("user_version %d", uv)
	}
	var mode string
	if err := st.DB().QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Fatalf("journal_mode %s", mode)
	}
	var events, tags int
	if err := st.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM events`).Scan(&events); err != nil || events != 2 {
		t.Fatalf("events %d %v", events, err)
	}
	if err := st.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM event_tags`).Scan(&tags); err != nil || tags != 0 {
		t.Fatalf("tags %d %v", tags, err)
	}
	hello, err := st.SearchEvents(ctx, "hello", nostr.Filter{})
	if err != nil || len(hello) != 1 || hello[0].ID != "id-hello" {
		t.Fatalf("hello search: %+v %v", hello, err)
	}
	running, err := st.SearchEvents(ctx, "running", nostr.Filter{})
	if err != nil || len(running) != 1 || running[0].ID != "id-run" {
		t.Fatalf("running search: %+v %v", running, err)
	}
	stem, err := st.SearchEvents(ctx, "run", nostr.Filter{})
	if err != nil || len(stem) != 0 {
		t.Fatalf("porter stem should not match: %+v %v", stem, err)
	}
	bak := path + ".pre-v8.bak"
	raw, err := os.ReadFile(bak)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st2, err := Open(ctx, path, nil, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	if err := st2.DB().QueryRowContext(ctx, `PRAGMA user_version`).Scan(&uv); err != nil || uv != 8 {
		t.Fatalf("second open version %d %v", uv, err)
	}
	raw2, err := os.ReadFile(bak)
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(raw2) != sum {
		t.Fatal("backup changed on second open")
	}

	pk := nostrRepeat("b", 64)
	newer := &nostr.Event{ID: nostrRepeat("c", 64), PubKey: pk, CreatedAt: 10, Kind: 0, Content: "winner note", Sig: nostrRepeat("s", 128)}
	older := &nostr.Event{ID: nostrRepeat("a", 64), PubKey: pk, CreatedAt: 9, Kind: 0, Content: "loser note", Sig: nostrRepeat("s", 128)}
	if err := st2.SaveEvent(ctx, newer); err != nil {
		t.Fatal(err)
	}
	if err := st2.SaveEvent(ctx, older); !errors.Is(err, storage.ErrStaleReplaceable) {
		t.Fatalf("stale replaceable: %v", err)
	}
	found, err := st2.SearchEvents(ctx, "winner", nostr.Filter{})
	if err != nil || len(found) != 1 || found[0].ID != newer.ID {
		t.Fatalf("winner search: %+v %v", found, err)
	}
	lost, err := st2.SearchEvents(ctx, "loser", nostr.Filter{})
	if err != nil || len(lost) != 0 {
		t.Fatalf("loser search: %+v %v", lost, err)
	}
}

func TestUpgradeV7FTSUnicodeProbe(t *testing.T) {
	skipNoDriver(t)
	if !sqlitewriter.HasLibsqlDriver() {
		t.Skip("libsql driver not available")
	}
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "events.db")
	writeV7Rows(t, ctx, path, []v7Row{{id: "id-cafe", content: "café"}})

	st, err := Open(ctx, path, nil, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var uv int
	if err := st.DB().QueryRowContext(ctx, `PRAGMA user_version`).Scan(&uv); err != nil {
		t.Fatal(err)
	}
	if uv != 8 {
		t.Fatalf("user_version %d", uv)
	}
	found, err := st.SearchEvents(ctx, "café", nostr.Filter{})
	if err != nil || len(found) != 1 || found[0].ID != "id-cafe" {
		t.Fatalf("café search: %+v %v", found, err)
	}
}

func TestSearchNoPorterStem(t *testing.T) {
	skipNoDriver(t)
	ctx := context.Background()
	st, err := Open(ctx, filepath.Join(t.TempDir(), "stem.db"), nil, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ev := &nostr.Event{
		ID: nostrRepeat("a", 64), PubKey: nostrRepeat("b", 64), CreatedAt: 1, Kind: 1,
		Content: "running late", Sig: nostrRepeat("c", 128),
	}
	if err := st.SaveEvent(ctx, ev); err != nil {
		t.Fatal(err)
	}
	got, err := st.SearchEvents(ctx, "run", nostr.Filter{})
	if err != nil || len(got) != 0 {
		t.Fatalf("stem match: %+v %v", got, err)
	}
}

func TestConcurrentReadsDuringWrite(t *testing.T) {
	skipNoDriver(t)
	ctx := context.Background()
	st, err := Open(ctx, filepath.Join(t.TempDir(), "conc.db"), nil, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	pk := nostrRepeat("b", 64)
	sig := nostrRepeat("s", 128)
	if err := st.SaveEvent(ctx, &nostr.Event{
		ID: nostrRepeat("a", 64), PubKey: pk, CreatedAt: 1, Kind: 1, Content: "seed", Sig: sig,
	}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errCh := make(chan error, 9)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := st.QueryEvents(ctx, []nostr.Filter{{Kinds: []int{1}}}); err != nil {
				errCh <- err
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		ev := &nostr.Event{ID: nostrRepeat("c", 64), PubKey: pk, CreatedAt: 2, Kind: 1, Content: "more", Sig: sig}
		if err := st.SaveEvent(ctx, ev); err != nil {
			errCh <- err
		}
	}()
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}
}

func writeV7Events(t *testing.T, ctx context.Context, path string) {
	t.Helper()
	writeV7Rows(t, ctx, path, []v7Row{
		{id: "id-hello", content: "say hello world"},
		{id: "id-run", content: "running late"},
	})
}

type v7Row struct {
	id      string
	content string
}

func writeV7Rows(t *testing.T, ctx context.Context, path string, rows []v7Row) {
	t.Helper()
	db, _, err := sqlitewriter.OpenLibsqlHandles(ctx, path, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	stmts := []string{
		`CREATE TABLE events (
			id TEXT NOT NULL PRIMARY KEY,
			pubkey TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			kind INTEGER NOT NULL,
			content TEXT NOT NULL,
			sig TEXT NOT NULL,
			d_tag TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE event_tags (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			event_id TEXT NOT NULL,
			pos INTEGER NOT NULL,
			name TEXT NOT NULL,
			value TEXT NOT NULL DEFAULT '',
			full_json TEXT NOT NULL
		)`,
		`CREATE VIRTUAL TABLE event_fts USING fts5(event_id UNINDEXED, content)`,
		`CREATE TRIGGER events_ai_fts AFTER INSERT ON events BEGIN
			INSERT INTO event_fts(event_id, content) VALUES (new.id, new.content);
		END`,
	}
	for _, q := range stmts {
		if err := sqlitewriter.ExecSQL(ctx, db, q); err != nil {
			t.Fatal(err)
		}
	}
	for i, row := range rows {
		q := fmt.Sprintf(
			`INSERT INTO events (id, pubkey, created_at, kind, content, sig) VALUES ('%s', 'pk', %d, 1, '%s', 'sig')`,
			strings.ReplaceAll(row.id, "'", "''"),
			i+1,
			strings.ReplaceAll(row.content, "'", "''"),
		)
		if err := sqlitewriter.ExecSQL(ctx, db, q); err != nil {
			t.Fatal(err)
		}
	}
	if err := sqlitewriter.ExecSQL(ctx, db, `PRAGMA user_version = 7`); err != nil {
		t.Fatal(err)
	}
}
