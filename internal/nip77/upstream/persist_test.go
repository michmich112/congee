package upstream

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/db"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/plugin"
	"github.com/michmich112/congee/internal/relay"
	"github.com/michmich112/congee/internal/storage/turso"
	"github.com/rs/zerolog"
)

type recordingRuntime struct {
	mu     sync.Mutex
	stored []*nostr.Event
}

func (r *recordingRuntime) Observe(any) {}

func (r *recordingRuntime) InterceptREQ(context.Context, *nostr.ReqMessage) plugin.InterceptResult {
	return plugin.InterceptResult{Action: plugin.InterceptPassthrough}
}

func (r *recordingRuntime) EnqueueStoredEvent(ev *nostr.Event, stored bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if stored {
		r.stored = append(r.stored, ev)
	}
}

func (r *recordingRuntime) Snapshot() []plugin.InstanceSnapshot { return nil }

func (r *recordingRuntime) ids() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.stored))
	for _, ev := range r.stored {
		out = append(out, ev.ID)
	}
	return out
}

func TestPersistImportedEventNotifiesPlugins(t *testing.T) {
	if !turso.HasDriver() {
		t.Skip("libsql driver not available")
	}
	ctx := context.Background()
	st, closeFn, err := db.OpenTestStore(ctx, filepath.Join(t.TempDir(), "events.db"), zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer closeFn()

	srv, err := relay.NewServer(config.DefaultConfig(), st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	rt := &recordingRuntime{}
	srv.SetPluginRuntime(rt)

	sch := NewScheduler(nil, st, srv, nil, zerolog.Nop())
	ev := &nostr.Event{
		ID:        "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		PubKey:    "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		CreatedAt: 1,
		Kind:      30402,
		Content:   "imported",
		Sig:       "cc",
	}
	ok, err := sch.persistImportedEvent(ctx, ev)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected import")
	}
	ids := rt.ids()
	if len(ids) != 1 || ids[0] != ev.ID {
		t.Fatalf("plugin stored: %v", ids)
	}

	ok, err = sch.persistImportedEvent(ctx, ev)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("duplicate import should be skipped")
	}
	if got := rt.ids(); len(got) != 1 {
		t.Fatalf("duplicate should not re-notify plugins: %v", got)
	}
}

func TestPersistImportedEventNilServer(t *testing.T) {
	if !turso.HasDriver() {
		t.Skip("libsql driver not available")
	}
	ctx := context.Background()
	st, closeFn, err := db.OpenTestStore(ctx, filepath.Join(t.TempDir(), "events.db"), zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer closeFn()

	sch := NewScheduler(nil, st, nil, nil, zerolog.Nop())
	ev := &nostr.Event{
		ID:        "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		PubKey:    "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		CreatedAt: 1,
		Kind:      1,
		Content:   "x",
		Sig:       "cc",
	}
	ok, err := sch.persistImportedEvent(ctx, ev)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected import")
	}
	has, err := st.HasEventID(ctx, ev.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Fatal("event should be stored")
	}
}
