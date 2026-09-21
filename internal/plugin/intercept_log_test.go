package plugin

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/rs/zerolog"
)

func waitInterceptLog(t *testing.T, m *Manager, id string, n int) InterceptLogSnapshot {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snap := m.InterceptLogSnapshot(id)
		if len(snap.Entries) == n {
			return snap
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("intercept log len want %d got %d", n, len(m.InterceptLogSnapshot(id).Entries))
	return InterceptLogSnapshot{}
}

func TestInterceptLogNotReadyPassthrough(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := NewManager(nil, t.TempDir(), nil, zerolog.Nop())
	m.startInterceptLogWorker(ctx)
	in := &instance{id: "fixture", mgr: m}
	req := &nostr.ReqMessage{SubID: "sub-1", Filters: []nostr.Filter{{Kinds: []int{30402}}}}
	res := in.intercept(ctx, req)
	if res.Action != InterceptPassthrough {
		t.Fatalf("action %v", res.Action)
	}
	snap := waitInterceptLog(t, m, "fixture", 1)
	got := snap.Entries[0]
	if got.Action != "passthrough" || got.Error != "not_ready" || got.SubID != "sub-1" {
		t.Fatalf("entry %+v", got)
	}
	if len(got.Filters) != 1 || len(got.Filters[0].Kinds) != 1 || got.Filters[0].Kinds[0] != 30402 {
		t.Fatalf("filters %+v", got.Filters)
	}
}

func TestInterceptLogRollingWindow(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := NewManager(nil, t.TempDir(), nil, zerolog.Nop())
	m.startInterceptLogWorker(ctx)
	if got := m.SetInterceptLogLimit(3); got != 3 {
		t.Fatalf("limit %d", got)
	}
	for i := 0; i < 5; i++ {
		m.enqueueInterceptLog("p", &nostr.ReqMessage{SubID: fmt.Sprintf("s%d", i)}, InterceptResult{
			Action:   InterceptRespond,
			EventIDs: []string{fmt.Sprintf("e%d", i)},
		}, time.Millisecond, "")
	}
	deadline := time.Now().Add(2 * time.Second)
	var snap InterceptLogSnapshot
	for time.Now().Before(deadline) {
		snap = m.InterceptLogSnapshot("p")
		if len(snap.Entries) == 3 && snap.Entries[0].SubID == "s4" {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if len(snap.Entries) != 3 || snap.Entries[0].SubID != "s4" || snap.Entries[2].SubID != "s2" {
		t.Fatalf("order %+v", snap.Entries)
	}
	if snap.Entries[0].Action != "respond" || snap.Entries[0].EventIDs[0] != "e4" {
		t.Fatalf("respond %+v", snap.Entries[0])
	}
}

func TestEnqueueInterceptLogDoesNotBlock(t *testing.T) {
	m := NewManager(nil, t.TempDir(), nil, zerolog.Nop())
	m.interceptLogCh = make(chan interceptLogJob)
	done := make(chan struct{})
	go func() {
		m.enqueueInterceptLog("p", &nostr.ReqMessage{SubID: "x"}, InterceptResult{}, 0, "")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("enqueue blocked")
	}
	if m.interceptLogDropped.Load() != 1 {
		t.Fatalf("dropped %d", m.interceptLogDropped.Load())
	}
	if len(m.InterceptLogSnapshot("p").Entries) != 0 {
		t.Fatal("dropped job should not appear in snapshot")
	}
}

func TestInterceptLogDisableStopsRecording(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := NewManager(nil, t.TempDir(), nil, zerolog.Nop())
	m.startInterceptLogWorker(ctx)
	m.enqueueInterceptLog("p", &nostr.ReqMessage{SubID: "keep"}, InterceptResult{Action: InterceptPassthrough}, 0, "")
	waitInterceptLog(t, m, "p", 1)
	if got := m.SetInterceptLogLimit(0); got != 0 {
		t.Fatalf("limit %d", got)
	}
	if len(m.InterceptLogSnapshot("p").Entries) != 0 {
		t.Fatal("limit 0 should clear log")
	}
	m.enqueueInterceptLog("p", &nostr.ReqMessage{SubID: "new"}, InterceptResult{Action: InterceptRespond}, 0, "")
	time.Sleep(30 * time.Millisecond)
	if len(m.InterceptLogSnapshot("p").Entries) != 0 {
		t.Fatal("limit 0 should not record")
	}
}

func TestInterceptLogShrinkTrimsExisting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := NewManager(nil, t.TempDir(), nil, zerolog.Nop())
	m.startInterceptLogWorker(ctx)
	for i := 0; i < 4; i++ {
		m.enqueueInterceptLog("p", &nostr.ReqMessage{SubID: fmt.Sprintf("s%d", i)}, InterceptResult{}, 0, "")
	}
	waitInterceptLog(t, m, "p", 4)
	if got := m.SetInterceptLogLimit(2); got != 2 {
		t.Fatalf("limit %d", got)
	}
	snap := m.InterceptLogSnapshot("p")
	if len(snap.Entries) != 2 || snap.Entries[0].SubID != "s3" || snap.Entries[1].SubID != "s2" {
		t.Fatalf("trimmed %+v", snap.Entries)
	}
}

func TestInterceptLogIsolatesPlugins(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := NewManager(nil, t.TempDir(), nil, zerolog.Nop())
	m.startInterceptLogWorker(ctx)
	m.enqueueInterceptLog("a", &nostr.ReqMessage{SubID: "sa"}, InterceptResult{Action: InterceptRespond, EventIDs: []string{"ea"}}, 0, "")
	m.enqueueInterceptLog("b", &nostr.ReqMessage{SubID: "sb"}, InterceptResult{Action: InterceptPassthrough}, 0, "not_ready")
	waitInterceptLog(t, m, "a", 1)
	waitInterceptLog(t, m, "b", 1)
	if m.InterceptLogSnapshot("a").Entries[0].SubID != "sa" {
		t.Fatal("plugin a")
	}
	if m.InterceptLogSnapshot("b").Entries[0].SubID != "sb" {
		t.Fatal("plugin b")
	}
}

func TestInterceptLogClonesFilters(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := NewManager(nil, t.TempDir(), nil, zerolog.Nop())
	m.startInterceptLogWorker(ctx)
	req := &nostr.ReqMessage{SubID: "c", Filters: []nostr.Filter{{Kinds: []int{30402}}}}
	m.enqueueInterceptLog("p", req, InterceptResult{Action: InterceptPassthrough}, 0, "")
	req.Filters[0].Kinds[0] = 1
	snap := waitInterceptLog(t, m, "p", 1)
	if snap.Entries[0].Filters[0].Kinds[0] != 30402 {
		t.Fatalf("cloned kinds %v", snap.Entries[0].Filters[0].Kinds)
	}
}

func TestInterceptReturnsBeforeLogPersists(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := NewManager(nil, t.TempDir(), nil, zerolog.Nop())
	ch := make(chan interceptLogJob, 4)
	m.interceptLogCh = ch
	in := &instance{id: "fixture", mgr: m}
	req := &nostr.ReqMessage{SubID: "fast", Filters: []nostr.Filter{{Kinds: []int{1}}}}
	res := in.intercept(ctx, req)
	if res.Action != InterceptPassthrough {
		t.Fatalf("action %v", res.Action)
	}
	if len(m.InterceptLogSnapshot("fixture").Entries) != 0 {
		t.Fatal("log should not be visible until worker runs")
	}
	go m.runInterceptLogWorker(ctx, ch)
	snap := waitInterceptLog(t, m, "fixture", 1)
	if snap.Entries[0].SubID != "fast" || snap.Entries[0].Error != "not_ready" {
		t.Fatalf("entry %+v", snap.Entries[0])
	}
}

func TestSetInterceptLogLimitClamps(t *testing.T) {
	m := NewManager(nil, t.TempDir(), nil, zerolog.Nop())
	if got := m.SetInterceptLogLimit(-3); got != 0 {
		t.Fatalf("neg %d", got)
	}
	if got := m.SetInterceptLogLimit(config.MaxPluginInterceptLogSize + 5); got != config.MaxPluginInterceptLogSize {
		t.Fatalf("max %d", got)
	}
}
