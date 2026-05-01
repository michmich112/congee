package storage_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
	"github.com/michmich112/congee/internal/storage/sqlite"
)

func TestMigrateSQLiteToSQLite(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "a.db")
	dstPath := filepath.Join(dir, "b.db")

	src, err := sqlite.Open(ctx, srcPath, nil)
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	dst, err := sqlite.Open(ctx, dstPath, nil)
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
	if err := src.SaveAuditEntry(ctx, storage.AuditEntry{CreatedAt: 1, Action: "x", Detail: "d", Pubkey: pk}); err != nil {
		t.Fatal(err)
	}
	if err := src.SaveConfigChange(ctx, storage.ConfigChange{CreatedAt: 2, Summary: "s", JSONDiff: "{}"}); err != nil {
		t.Fatal(err)
	}

	if err := storage.Migrate(ctx, src, dst, nil, nil); err != nil {
		t.Fatal(err)
	}

	out, err := dst.QueryEvents(ctx, []nostr.Filter{{IDs: []string{ev.ID}}})
	if err != nil || len(out) != 1 || out[0].Content != "migrated" {
		t.Fatalf("dst query: %v %+v", err, out)
	}
}
