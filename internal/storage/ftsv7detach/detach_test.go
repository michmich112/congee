package ftsv7detach

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/michmich112/congee/internal/storage/sqlitewriter"
	"github.com/rs/zerolog"
)

func TestDetachRollsBackWhenAggregatesMismatch(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "events.db")
	buildV7Fixture(t, ctx, path)

	prev := verifyEventsAggregates
	t.Cleanup(func() { verifyEventsAggregates = prev })
	verifyEventsAggregates = func(before, after eventAggregates) error {
		return errors.New("forced mismatch")
	}
	err := PrepareEvents(ctx, path, zerolog.Nop())
	if err == nil {
		t.Fatal("expected aggregate mismatch")
	}
	has, herr := HasEventFTS5(ctx, path)
	if herr != nil || !has {
		t.Fatalf("fts5 should remain after rollback: has=%v err=%v", has, herr)
	}
	var uv int
	db, oerr := openLibsqlRW(ctx, path)
	if oerr != nil {
		t.Fatal(oerr)
	}
	defer db.Close()
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&uv); err != nil {
		t.Fatal(err)
	}
	if uv != 7 {
		t.Fatalf("user_version %d", uv)
	}
	if _, err := os.Stat(path + backupSuffix); err != nil {
		t.Fatal(err)
	}
}

func TestDetachIsIdempotentAndKeepsBackup(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "events.db")
	buildV7Fixture(t, ctx, path)
	if err := PrepareEvents(ctx, path, zerolog.Nop()); err != nil {
		t.Fatal(err)
	}
	bak := path + backupSuffix
	raw, err := os.ReadFile(bak)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	has, err := HasEventFTS5(ctx, path)
	if err != nil || has {
		t.Fatalf("fts5 still present: %v %v", has, err)
	}
	if err := PrepareEvents(ctx, path, zerolog.Nop()); err != nil {
		t.Fatal(err)
	}
	raw2, err := os.ReadFile(bak)
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(raw2) != sum {
		t.Fatal("backup bytes changed on second detach")
	}
}

func buildV7Fixture(t *testing.T, ctx context.Context, path string) {
	t.Helper()
	if !sqlitewriter.HasLibsqlDriver() {
		t.Skip("libsql driver not available")
	}
	db, err := openLibsqlRW(ctx, path)
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
		`INSERT INTO events (id, pubkey, created_at, kind, content, sig) VALUES ('id-hello', 'pk', 1, 1, 'say hello world', 'sig')`,
		`INSERT INTO events (id, pubkey, created_at, kind, content, sig) VALUES ('id-run', 'pk', 2, 1, 'running late', 'sig')`,
		`PRAGMA user_version = 7`,
	}
	for _, q := range stmts {
		if err := sqlitewriter.ExecSQL(ctx, db, q); err != nil {
			t.Fatal(err)
		}
	}
}
