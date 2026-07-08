package turso

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
	"github.com/michmich112/congee/internal/storage/sqlite"
	"github.com/rs/zerolog"
)

func TestMigrateSQLiteToTursoNative(t *testing.T) {
	if !HasDriver() {
		t.Skip("libsql driver not available")
	}
	ctx := context.Background()
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "src.db")
	dstPath := filepath.Join(dir, "dst.db")

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
		Content:   "migrate me",
		Sig:       strings.Repeat("c", 128),
	}
	if err := src.SaveEvent(ctx, ev); err != nil {
		t.Fatal(err)
	}
	if err := src.Close(); err != nil {
		t.Fatal(err)
	}

	sum, err := storage.MigrateSQLiteToTursoNative(ctx, srcPath, dstPath)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Source.Events != 1 || sum.DestinationFinal.Events != 1 {
		t.Fatalf("counts: %+v", sum)
	}

	dst, err := Open(ctx, dstPath, nil, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer dst.Close()
	out, err := dst.QueryEvents(ctx, []nostr.Filter{{IDs: []string{ev.ID}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Content != "migrate me" {
		t.Fatalf("query dst: %+v", out)
	}
}

func TestMigrateSQLiteToTursoNativeRemovesEmptyPreflightShell(t *testing.T) {
	if !HasDriver() {
		t.Skip("libsql driver not available")
	}
	ctx := context.Background()
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "src.db")
	dstPath := filepath.Join(dir, "dst.db")

	src, err := sqlite.Open(ctx, srcPath, nil, zerolog.Nop())
	if err != nil {
		if strings.Contains(err.Error(), "not available") {
			t.Skip(err)
		}
		t.Fatal(err)
	}
	ev := &nostr.Event{
		ID:        strings.Repeat("d", 64),
		PubKey:    strings.Repeat("e", 64),
		CreatedAt: 2,
		Kind:      1,
		Content:   "after empty shell",
		Sig:       strings.Repeat("f", 128),
	}
	if err := src.SaveEvent(ctx, ev); err != nil {
		t.Fatal(err)
	}
	if err := src.Close(); err != nil {
		t.Fatal(err)
	}

	// Simulate a previous preflight that opened libSQL and created an empty file.
	pf := PreflightMigrationTarget(ctx, dstPath, zerolog.Nop())
	if pf.Status != "empty" {
		t.Fatalf("preflight status=%q want empty", pf.Status)
	}
	if err := os.WriteFile(dstPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	sum, err := storage.MigrateSQLiteToTursoNative(ctx, srcPath, dstPath)
	if err != nil {
		t.Fatal(err)
	}
	if sum.DestinationFinal.Events != 1 {
		t.Fatalf("counts: %+v", sum)
	}
}
