package db_test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/michmich112/congee/internal/db"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
	"github.com/rs/zerolog"
	"github.com/uptrace/bun/driver/sqliteshim"
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

	metaPath := filepath.Join(dir, "congee-meta.db")
	for _, p := range []string{eventsPath, metaPath} {
		if err := analyzeSQLiteFile(ctx, p); err != nil {
			t.Fatal(err)
		}
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

func TestCompositeConcurrentEventAndAuditWrites(t *testing.T) {
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

	const workers = 16
	const perWorker = 8
	var wg sync.WaitGroup
	errCh := make(chan error, workers*2)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < perWorker; j++ {
				id := fmt.Sprintf("%064x", i*perWorker+j)
				ev := &nostr.Event{
					ID:        id,
					PubKey:    strings.Repeat("b", 64),
					CreatedAt: int64(1000 + i*perWorker + j),
					Kind:      1,
					Tags:      [][]string{{"p", strings.Repeat("b", 64)}},
					Content:   "x",
					Sig:       strings.Repeat("c", 128),
				}
				if err := st.SaveEvent(ctx, ev); err != nil {
					errCh <- err
					return
				}
			}
		}(i)
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < perWorker; j++ {
				e := storage.AuditEntry{
					CreatedAt: int64(2000 + i*perWorker + j),
					Action:    fmt.Sprintf("concurrent_test_%d", i),
					Detail:    fmt.Sprintf("j=%d", j),
					Pubkey:    strings.Repeat("d", 64),
				}
				if err := st.SaveAuditEntry(ctx, e); err != nil {
					errCh <- err
					return
				}
			}
		}(i)
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			if strings.Contains(err.Error(), "SQLITE_BUSY") {
				t.Fatalf("unexpected SQLITE_BUSY: %v", err)
			}
			t.Fatal(err)
		}
	}

	nEvents, err := st.CountEvents(ctx, []nostr.Filter{{Kinds: []int{1}}})
	if err != nil {
		t.Fatal(err)
	}
	if nEvents != workers*perWorker {
		t.Fatalf("events saved: got %d want %d", nEvents, workers*perWorker)
	}
	nAudit, err := st.CountAuditLog(ctx, storage.AuditQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if nAudit != int64(workers*perWorker) {
		t.Fatalf("audit rows: got %d want %d", nAudit, workers*perWorker)
	}
}

func analyzeSQLiteFile(ctx context.Context, path string) error {
	if !sqliteshim.HasDriver() {
		return nil
	}
	sqldb, err := sql.Open(sqliteshim.ShimName, "file:"+path+"?cache=shared")
	if err != nil {
		return err
	}
	defer func() { _ = sqldb.Close() }()
	_, err = sqldb.ExecContext(ctx, `ANALYZE`)
	return err
}
