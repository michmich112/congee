package db_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michmich112/congee/internal/db"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
	"github.com/rs/zerolog"
)

func TestCompositeAdminStorageSnapshotMerge(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	eventsPath := filepath.Join(dir, "events.db")
	st, closeStore, err := db.OpenTestStore(ctx, eventsPath, zerolog.Nop())
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()

	ev := &nostr.Event{
		ID:        strings.Repeat("a", 64),
		PubKey:    strings.Repeat("b", 64),
		CreatedAt: 1,
		Kind:      1,
		Tags:      [][]string{{"p", strings.Repeat("b", 64)}},
		Content:   "x",
		Sig:       strings.Repeat("c", 128),
	}
	if err := st.SaveEvent(ctx, ev); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveAuditEntry(ctx, storage.AuditEntry{CreatedAt: 2, Action: "x", Detail: "d", Pubkey: ev.PubKey}); err != nil {
		t.Fatal(err)
	}

	snap, err := st.AdminStorageSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Events != 1 || snap.Tags != 1 {
		t.Fatalf("events snapshot: events=%d tags=%d", snap.Events, snap.Tags)
	}
	if snap.Audit != 1 {
		t.Fatalf("meta snapshot audit=%d", snap.Audit)
	}
	if snap.Bytes <= 0 || snap.MetaBytes <= 0 {
		t.Fatalf("bytes: events=%d meta=%d", snap.Bytes, snap.MetaBytes)
	}
}
