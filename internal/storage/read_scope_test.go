package storage_test

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
	"github.com/michmich112/congee/internal/storage/postgres"
	"github.com/michmich112/congee/internal/storage/turso"
	"github.com/rs/zerolog"
)

func TestReadScopeAcrossQueryCountSearchAndSync(t *testing.T) {
	for _, backend := range []string{"turso", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			var st storage.EventStore
			if backend == "turso" {
				if !turso.HasDriver() {
					t.Skip("libsql driver not available")
				}
				store, err := turso.Open(t.Context(), filepath.Join(t.TempDir(), "events.db"), nil, zerolog.Nop())
				if err != nil {
					t.Fatal(err)
				}
				defer store.Close()
				st = store
			} else {
				dsn := os.Getenv("TEST_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("TEST_POSTGRES_DSN not set")
				}
				store, err := postgres.Open(t.Context(), dsn, "read-scope-test", zerolog.Nop())
				if err != nil {
					t.Fatal(err)
				}
				defer store.Close()
				st = store
			}
			author, alice, bob := strings.Repeat("d", 64), strings.Repeat("a", 64), strings.Repeat("b", 64)
			var ids []string
			for i, tags := range [][][]string{nil, {{"p", alice}}, {{"p", bob}}, {{"p", alice}, {"p"}}} {
				id := fmt.Sprintf("%064x", 900+i)
				ids = append(ids, id)
				kind := 1059
				if i == 0 {
					kind = 1
				}
				ev := &nostr.Event{ID: id, PubKey: author, Sig: strings.Repeat("f", 128), Kind: kind, CreatedAt: int64(i + 1), Content: "scopedcatalog", Tags: tags}
				if err := st.SaveEvent(t.Context(), ev); err != nil {
					t.Fatal(err)
				}
				defer st.DeleteEvent(t.Context(), id)
			}
			for _, authenticated := range []bool{false, true} {
				scope := &nostr.ReadScope{RecipientKinds: []int{1059, 21059}}
				want := ids[:1]
				if authenticated {
					scope.RecipientPubkeys = []string{alice}
					want = ids[:2]
				}
				filter := nostr.Filter{IDs: ids, ReadScope: scope}
				count, err := st.CountEvents(t.Context(), []nostr.Filter{filter})
				if err != nil || count != len(want) {
					t.Fatalf("auth=%v count=%d err=%v", authenticated, count, err)
				}
				refs, err := st.QueryEventSyncItems(t.Context(), filter)
				if err != nil {
					t.Fatal(err)
				}
				var refIDs []string
				for _, ref := range refs {
					refIDs = append(refIDs, ref.ID)
				}
				sort.Strings(refIDs)
				if !reflect.DeepEqual(refIDs, want) {
					t.Fatalf("sync metadata mismatch: %v want %v", refIDs, want)
				}
				limit := 1
				filter.Limit = &limit
				for _, search := range []bool{false, true} {
					var events []*nostr.Event
					if search {
						events, err = st.SearchEvents(t.Context(), "scopedcatalog", filter)
					} else {
						events, err = st.QueryEvents(t.Context(), []nostr.Filter{filter})
					}
					if err != nil {
						t.Fatal(err)
					}
					if len(events) != 1 || !scope.Allows(events[0]) {
						t.Fatalf("auth=%v search=%v protected row consumed limit: %v", authenticated, search, events)
					}
					if !search && events[0].ID != want[len(want)-1] {
						t.Fatal("query did not return the newest authorized event")
					}
				}
			}
		})
	}
}
