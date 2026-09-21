package relay

import (
	"context"
	"sync"
	"testing"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/plugin"
)

type recordingPluginRuntime struct {
	mu      sync.Mutex
	stored  []storedNotify
	observe []any
}

type storedNotify struct {
	ev     *nostr.Event
	stored bool
}

func (r *recordingPluginRuntime) Observe(msg any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.observe = append(r.observe, msg)
}

func (r *recordingPluginRuntime) InterceptREQ(context.Context, *nostr.ReqMessage) plugin.InterceptResult {
	return plugin.InterceptResult{Action: plugin.InterceptPassthrough}
}

func (r *recordingPluginRuntime) EnqueueStoredEvent(ev *nostr.Event, stored bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stored = append(r.stored, storedNotify{ev: ev, stored: stored})
}

func (r *recordingPluginRuntime) Snapshot() []plugin.InstanceSnapshot { return nil }

func (r *recordingPluginRuntime) storedCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.stored)
}

func (r *recordingPluginRuntime) observeCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.observe)
}

func (r *recordingPluginRuntime) lastStored() storedNotify {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.stored) == 0 {
		return storedNotify{}
	}
	return r.stored[len(r.stored)-1]
}

func (r *recordingPluginRuntime) lastObserve() any {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.observe) == 0 {
		return nil
	}
	return r.observe[len(r.observe)-1]
}

func TestNotifyPluginStoredEventEnqueuesAndObserves(t *testing.T) {
	rt := &recordingPluginRuntime{}
	s := &Server{plugins: rt}
	ev := &nostr.Event{ID: "aa", Kind: 30402, PubKey: "bb"}
	s.NotifyPluginStoredEvent(ev, true)
	if rt.storedCount() != 1 {
		t.Fatalf("stored notifies: %d", rt.storedCount())
	}
	got := rt.lastStored()
	if !got.stored || got.ev.ID != "aa" || got.ev.Kind != 30402 {
		t.Fatalf("stored notify: %+v", got)
	}
	if rt.observeCount() != 1 {
		t.Fatalf("observe: %d", rt.observeCount())
	}
	msg, ok := rt.lastObserve().(*nostr.EventMessage)
	if !ok {
		t.Fatalf("observe type %T", rt.lastObserve())
	}
	if msg.Event.Kind != 30402 || msg.Event.ID != "aa" {
		t.Fatalf("observe event: %+v", msg.Event)
	}
}

func TestNotifyPluginStoredEventNilSafe(t *testing.T) {
	var s *Server
	s.NotifyPluginStoredEvent(&nostr.Event{ID: "aa"}, true)
	s = &Server{}
	s.NotifyPluginStoredEvent(&nostr.Event{ID: "aa"}, true)
	s.NotifyPluginStoredEvent(nil, true)
}

func TestPluginOnStoredHookDoesNotObserve(t *testing.T) {
	rt := &recordingPluginRuntime{}
	s := &Server{plugins: rt}
	s.notifyPluginStoredEvent(&nostr.Event{ID: "aa", Kind: 1}, true, false)
	if rt.storedCount() != 1 {
		t.Fatalf("stored notifies: %d", rt.storedCount())
	}
	if rt.observeCount() != 0 {
		t.Fatalf("live EVENT path already observed inbound; got %d extra observes", rt.observeCount())
	}
}
