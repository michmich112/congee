package relay

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/nostr"
)

func TestSubscriptionManagerSubIDLength(t *testing.T) {
	cfg := minimalRelayCfg()
	m := NewSubscriptionManager(cfg, zerolog.Nop())
	long := make([]byte, cfg.MaxSubscriptionIDLength+1)
	for i := range long {
		long[i] = 'a'
	}
	err := m.Add("c1", string(long), nil)
	if err == nil {
		t.Fatal("expected error for long sub id")
	}
	if err := m.Add("c1", "ok", []nostr.Filter{{Kinds: []int{1}}}); err != nil {
		t.Fatal(err)
	}
	if m.SubCount("c1") != 1 {
		t.Fatalf("sub count %d", m.SubCount("c1"))
	}
}

func TestSubscriptionManagerMaxSubs(t *testing.T) {
	cfg := minimalRelayCfg()
	cfg.ConnectionLimits.MaxSubscriptionsPerConnection = 1
	m := NewSubscriptionManager(cfg, zerolog.Nop())
	m.RegisterSender("c1", func([]byte) bool { return true })
	if err := m.Add("c1", "a", nil); err != nil {
		t.Fatal(err)
	}
	if err := m.Add("c1", "b", nil); err != ErrTooManySubscriptions {
		t.Fatalf("got %v", err)
	}
}

func TestFiltersMatch_SearchNeverMatchesLive(t *testing.T) {
	ev := &nostr.Event{ID: strings.Repeat("1", 64), PubKey: strings.Repeat("2", 64), CreatedAt: 1, Kind: 1, Content: "hello"}
	q := "hello"
	f := nostr.Filter{Kinds: []int{1}, Search: &q}
	if filtersMatch([]nostr.Filter{f}, ev) {
		t.Fatal("subscription fan-out must not treat search as a live filter")
	}
}

func TestFiltersMatchOR(t *testing.T) {
	ev := &nostr.Event{Kind: 1, PubKey: "abc", Content: "x"}
	f1 := nostr.Filter{Kinds: []int{2}}
	f2 := nostr.Filter{Kinds: []int{1}}
	if filtersMatch([]nostr.Filter{f1, f2}, ev) != true {
		t.Fatal("expected OR match")
	}
}

func TestBroadcastRespectsClose(t *testing.T) {
	cfg := minimalRelayCfg()
	m := NewSubscriptionManager(cfg, zerolog.Nop())

	var sent atomic.Int64
	m.RegisterSender("c1", func(b []byte) bool {
		sent.Add(1)
		return true
	})
	if err := m.Add("c1", "sub1", []nostr.Filter{{Kinds: []int{1}}}); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	removeDone := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-removeDone
		m.Remove("c1", "sub1")
	}()

	for i := 0; i < 100; i++ {
		ev := &nostr.Event{ID: fmt.Sprintf("%064d", i), PubKey: strings.Repeat("2", 64), Kind: 1, Content: "x"}
		m.Broadcast(ev, nil)
	}
	m.FinishSnapshot("c1", "sub1")
	close(removeDone)
	wg.Wait()

	time.Sleep(50 * time.Millisecond)

	for i := 0; i < 100; i++ {
		ev := &nostr.Event{ID: fmt.Sprintf("%064d", i+200), PubKey: strings.Repeat("2", 64), Kind: 1, Content: "x"}
		m.Broadcast(ev, nil)
	}

	if sent.Load() != 100 {
		t.Fatalf("expected exactly 100 events sent after close, got %d", sent.Load())
	}
}

func TestConcurrentBroadcastRemove(t *testing.T) {
	cfg := minimalRelayCfg()
	m := NewSubscriptionManager(cfg, zerolog.Nop())

	var sent atomic.Int64
	m.RegisterSender("c1", func(b []byte) bool {
		sent.Add(1)
		return true
	})
	if err := m.Add("c1", "s1", []nostr.Filter{{Kinds: []int{1}}}); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	start := sync.WaitGroup{}
	start.Add(1)

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			start.Wait()
			ev := &nostr.Event{ID: fmt.Sprintf("%064d", n), PubKey: strings.Repeat("2", 64), Kind: 1, Content: "x"}
			m.Broadcast(ev, nil)
		}(i)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		start.Wait()
		m.Remove("c1", "s1")
	}()

	start.Done()
	wg.Wait()

	if m.SubCount("c1") != 0 {
		t.Fatalf("sub count %d, expected 0", m.SubCount("c1"))
	}
}

func TestCloseThenBroadcastNeverSends(t *testing.T) {
	cfg := minimalRelayCfg()
	m := NewSubscriptionManager(cfg, zerolog.Nop())

	var sent atomic.Int64
	m.RegisterSender("c1", func(b []byte) bool {
		sent.Add(1)
		return true
	})
	if err := m.Add("c1", "s1", []nostr.Filter{{Kinds: []int{1}}}); err != nil {
		t.Fatal(err)
	}

	m.Remove("c1", "s1")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		for i := 0; i < 1000; i++ {
			select {
			case <-ctx.Done():
				done <- false
				return
			default:
				ev := &nostr.Event{ID: fmt.Sprintf("%064d", i), PubKey: strings.Repeat("2", 64), Kind: 1, Content: "x"}
				m.Broadcast(ev, nil)
			}
		}
		done <- true
	}()

	<-done
	if sent.Load() != 0 {
		t.Fatalf("expected 0 events sent after close, got %d", sent.Load())
	}
}

func TestBroadcastBuffersUntilFinishSnapshot(t *testing.T) {
	cfg := minimalRelayCfg()
	m := NewSubscriptionManager(cfg, zerolog.Nop())

	var sent atomic.Int64
	m.RegisterSender("c1", func(b []byte) bool {
		sent.Add(1)
		return true
	})
	if err := m.Add("c1", "s1", []nostr.Filter{{Kinds: []int{1}}}); err != nil {
		t.Fatal(err)
	}

	ev1 := &nostr.Event{ID: strings.Repeat("1", 64), PubKey: strings.Repeat("2", 64), Kind: 1, Content: "a"}
	ev2 := &nostr.Event{ID: strings.Repeat("3", 64), PubKey: strings.Repeat("2", 64), Kind: 1, Content: "b"}
	m.Broadcast(ev1, nil)
	m.Broadcast(ev2, nil)
	if sent.Load() != 0 {
		t.Fatalf("expected 0 sends before FinishSnapshot, got %d", sent.Load())
	}

	m.FinishSnapshot("c1", "s1")
	if sent.Load() != 2 {
		t.Fatalf("expected 2 flushed events, got %d", sent.Load())
	}

	ev3 := &nostr.Event{ID: strings.Repeat("5", 64), PubKey: strings.Repeat("2", 64), Kind: 1, Content: "c"}
	m.Broadcast(ev3, nil)
	if sent.Load() != 3 {
		t.Fatalf("expected immediate send after snapshot, got %d", sent.Load())
	}
}

func TestBroadcastSnapshotOverflowStopsBuffering(t *testing.T) {
	cfg := minimalRelayCfg()
	m := NewSubscriptionManager(cfg, zerolog.Nop())

	var sent atomic.Int64
	m.RegisterSender("c1", func(b []byte) bool {
		sent.Add(1)
		return true
	})
	if err := m.Add("c1", "s1", []nostr.Filter{{Kinds: []int{1}}}); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < pendingLiveCap+10; i++ {
		ev := &nostr.Event{ID: fmt.Sprintf("%064d", i), PubKey: strings.Repeat("2", 64), Kind: 1, Content: "x"}
		m.Broadcast(ev, nil)
	}

	m.FinishSnapshot("c1", "s1")
	if sent.Load() != pendingLiveCap {
		t.Fatalf("expected %d buffered events flushed, got %d", pendingLiveCap, sent.Load())
	}
}

func TestAddResetsSnapshotBuffer(t *testing.T) {
	cfg := minimalRelayCfg()
	m := NewSubscriptionManager(cfg, zerolog.Nop())
	m.RegisterSender("c1", func([]byte) bool { return true })
	if err := m.Add("c1", "s1", []nostr.Filter{{Kinds: []int{1}}}); err != nil {
		t.Fatal(err)
	}
	ev := &nostr.Event{ID: strings.Repeat("1", 64), PubKey: strings.Repeat("2", 64), Kind: 1, Content: "x"}
	m.Broadcast(ev, nil)

	if err := m.Add("c1", "s1", []nostr.Filter{{Kinds: []int{1}}}); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	e := m.subs["c1"]["s1"]
	if e.snapshotDone.Load() {
		t.Fatal("replaced sub should reset snapshotDone")
	}
	if len(e.pendingLive) != 0 {
		t.Fatalf("replaced sub should clear pendingLive, got %d", len(e.pendingLive))
	}
	m.mu.Unlock()
}

func TestIsSameSnapshot(t *testing.T) {
	cfg := minimalRelayCfg()
	m := NewSubscriptionManager(cfg, zerolog.Nop())
	m.RegisterSender("c1", func([]byte) bool { return true })
	if err := m.Add("c1", "s1", []nostr.Filter{{Kinds: []int{1}}}); err != nil {
		t.Fatal(err)
	}
	opened, ok := m.SubOpenedUnix("c1", "s1")
	if !ok {
		t.Fatal("SubOpenedUnix")
	}
	if !m.IsSameSnapshot("c1", "s1", opened) {
		t.Fatal("expected same snapshot")
	}
	if m.IsSameSnapshot("c1", "s1", opened-1) {
		t.Fatal("expected stale opened_unix to mismatch")
	}
	time.Sleep(1100 * time.Millisecond)
	if err := m.Add("c1", "s1", []nostr.Filter{{Kinds: []int{1}}}); err != nil {
		t.Fatal(err)
	}
	if m.IsSameSnapshot("c1", "s1", opened) {
		t.Fatal("expected replacement to invalidate stale opened_unix")
	}
	reopened, ok := m.SubOpenedUnix("c1", "s1")
	if !ok || !m.IsSameSnapshot("c1", "s1", reopened) {
		t.Fatal("expected new snapshot after replacement")
	}
	m.Remove("c1", "s1")
	if m.IsSameSnapshot("c1", "s1", reopened) {
		t.Fatal("expected closed sub to fail snapshot check")
	}
}

func minimalRelayCfg() *config.Config {
	return &config.Config{
		MaxSubscriptionIDLength: 64,
		ConnectionLimits: config.ConnectionLimitsSection{
			MaxSubscriptionsPerConnection: 10,
			MaxFiltersPerReq:              5,
			ConnectionsPerMinutePerIP:     100,
		},
	}
}
