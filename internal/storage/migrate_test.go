package storage_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
	"github.com/michmich112/congee/internal/storage/sqlite"
	"github.com/rs/zerolog"
)

func TestMigrateSQLiteToSQLite(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "a.db")
	dstPath := filepath.Join(dir, "b.db")

	src, err := sqlite.Open(ctx, srcPath, nil, zerolog.Nop())
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	dst, err := sqlite.Open(ctx, dstPath, nil, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer dst.Close()

	pk := strings.Repeat("b", 64)
	ev := &nostr.Event{
		ID:        strings.Repeat("a", 64),
		PubKey:    pk,
		CreatedAt: 5,
		Kind:      1,
		Tags:      [][]string{{"p", pk}},
		Content:   "migrated",
		Sig:       strings.Repeat("c", 128),
	}
	if err := src.SaveEvent(ctx, ev); err != nil {
		t.Fatal(err)
	}

	sum, err := storage.Migrate(ctx, src, dst, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if sum.EventsInserted != 1 || sum.EventsSkipped != 0 {
		t.Fatalf("summary: %+v", sum)
	}

	out, err := dst.QueryEvents(ctx, []nostr.Filter{{IDs: []string{ev.ID}}})
	if err != nil || len(out) != 1 || out[0].Content != "migrated" {
		t.Fatalf("dst query: %v %+v", err, out)
	}
}

func TestMigrateSkipsExistingRowsOnDestination(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "src.db")
	dstPath := filepath.Join(dir, "dst.db")

	src, err := sqlite.Open(ctx, srcPath, nil, zerolog.Nop())
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	dst, err := sqlite.Open(ctx, dstPath, nil, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer dst.Close()

	pk := strings.Repeat("b", 64)
	ev := &nostr.Event{
		ID:        strings.Repeat("a", 64),
		PubKey:    pk,
		CreatedAt: 5,
		Kind:      1,
		Tags:      [][]string{{"p", pk}, {"x", "y"}},
		Content:   "body",
		Sig:       strings.Repeat("c", 128),
	}
	if err := src.SaveEvent(ctx, ev); err != nil {
		t.Fatal(err)
	}

	if err := dst.SaveEvent(ctx, ev); err != nil {
		t.Fatal(err)
	}

	sum, err := storage.Migrate(ctx, src, dst, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if sum.EventsInserted != 0 || sum.EventsSkipped != 1 {
		t.Fatalf("summary: %+v", sum)
	}

	out, err := dst.QueryEvents(ctx, []nostr.Filter{{IDs: []string{ev.ID}}})
	if err != nil || len(out) != 1 || len(out[0].Tags) != 2 {
		t.Fatalf("dst event: %v tags=%d", err, len(out[0].Tags))
	}
}
