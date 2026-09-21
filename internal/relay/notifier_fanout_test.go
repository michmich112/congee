package relay

import (
	"context"
	"testing"
	"time"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/rs/zerolog"
)

type eventByIDStore struct {
	visibilityStoreStub
	ev *nostr.Event
}

func (s *eventByIDStore) QueryEvents(ctx context.Context, filters []nostr.Filter) ([]*nostr.Event, error) {
	_, _ = ctx, filters
	if s.ev == nil {
		return nil, nil
	}
	for _, f := range filters {
		for _, id := range f.IDs {
			if id == s.ev.ID {
				cp := *s.ev
				return []*nostr.Event{&cp}, nil
			}
		}
	}
	return nil, nil
}

type chanNotifier struct {
	ch chan string
}

func (n *chanNotifier) Notify(id string) {
	select {
	case n.ch <- id:
	default:
	}
}

func (n *chanNotifier) Listen() <-chan string { return n.ch }

func (n *chanNotifier) Close() error { return nil }

func TestRunImportedEventFanoutNotifiesPlugins(t *testing.T) {
	ev := &nostr.Event{ID: "imported-event-id", Kind: 30402, PubKey: "aa"}
	st := &eventByIDStore{ev: ev}
	rt := &recordingPluginRuntime{}
	srv := &Server{plugins: rt, subs: NewSubscriptionManager(minimalRelayCfg(), zerolog.Nop())}

	n := &chanNotifier{ch: make(chan string, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		RunImportedEventFanout(ctx, srv, st, n, zerolog.Nop())
	}()

	n.ch <- ev.ID

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if rt.storedCount() == 1 && rt.observeCount() == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if rt.storedCount() != 1 {
		t.Fatalf("plugin stored notifies: %d", rt.storedCount())
	}
	got := rt.lastStored()
	if got.ev.ID != ev.ID || got.ev.Kind != 30402 || !got.stored {
		t.Fatalf("stored notify: %+v", got)
	}
	msg, ok := rt.lastObserve().(*nostr.EventMessage)
	if !ok || msg.Event.ID != ev.ID {
		t.Fatalf("observe: %#v", rt.observe)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("fanout did not exit")
	}
}
