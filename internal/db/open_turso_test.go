package db

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
	"github.com/michmich112/congee/internal/storage/sqlite"
	"github.com/michmich112/congee/internal/storage/sqlitewriter"
	"github.com/michmich112/congee/internal/storage/turso"
	"github.com/rs/zerolog"
)

func TestOpenTurso(t *testing.T) {
	if !turso.HasDriver() {
		t.Skip("libsql driver not available")
	}
	ctx := context.Background()
	dir := t.TempDir()
	sec := config.DatabaseSection{
		Type:    "turso",
		DSN:     filepath.Join(dir, "events.db"),
		MetaDSN: filepath.Join(dir, "meta.db"),
	}
	h, err := Open(ctx, sec, "", zerolog.Nop())
	if err != nil {
		if strings.Contains(err.Error(), "not available") {
			t.Skip(err)
		}
		t.Fatal(err)
	}
	defer func() { _ = h.Close() }()
	if h.Store == nil {
		t.Fatal("expected store")
	}
	_ = sqlitewriter.HasLibsqlDriver()
}

func TestOpenTursoAfterSQLiteMigration(t *testing.T) {
	if !turso.HasDriver() {
		t.Skip("libsql driver not available")
	}
	ctx := context.Background()
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "events.db")
	tursoPath := filepath.Join(dir, "events-turso.db")
	metaPath := filepath.Join(dir, "meta.db")

	src, err := sqlite.Open(ctx, srcPath, nil, zerolog.Nop())
	if err != nil {
		if strings.Contains(err.Error(), "not available") {
			t.Skip(err)
		}
		t.Fatal(err)
	}
	ev := &nostr.Event{
		ID:        strings.Repeat("a", 64),
		PubKey:    strings.Repeat("b", 64),
		CreatedAt: 1,
		Kind:      1,
		Content:   "post-migration open",
		Sig:       strings.Repeat("c", 128),
	}
	if err := src.SaveEvent(ctx, ev); err != nil {
		t.Fatal(err)
	}
	if err := src.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := storage.MigrateSQLiteToTursoNative(ctx, srcPath, tursoPath); err != nil {
		t.Fatal(err)
	}

	sec := config.DatabaseSection{
		Type:    "turso",
		DSN:     tursoPath,
		MetaDSN: metaPath,
	}
	h, err := Open(ctx, sec, "", zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = h.Close() }()
}
