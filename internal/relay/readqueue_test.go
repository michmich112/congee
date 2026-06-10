package relay

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/michmich112/congee/internal/db"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
	"github.com/rs/zerolog"
)

func TestReaderQueueEnqueueBackpressure(t *testing.T) {
	ctx := context.Background()
	st, closeStore, err := db.OpenTestStore(ctx, filepath.Join(t.TempDir(), "rq.db"), zerolog.Nop())
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()

	srv, err := NewServer(testRelayConfig(), st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	q := &ReaderQueue{
		srv:    srv,
		jobs:   make(chan *reqPageJob, 1),
		ctx:    srv.metricsCtx,
		cancel: func() {},
	}
	q.jobs <- &reqPageJob{connID: "blocker"}
	if q.Enqueue(&reqPageJob{connID: "next"}) {
		t.Fatal("expected false when jobs channel is full")
	}
}

func TestReaderQueueDrainsPagesThenEOSE(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, closeStore, err := db.OpenTestStore(ctx, filepath.Join(dir, "rq2.db"), zerolog.Nop())
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()

	createTestEvents(t, st, ctx, 5)

	cfg := testRelayConfig()
	pageSize := 2
	cfg.ConnectionLimits.QueryPageSize = &pageSize

	srv, err := NewServer(cfg, st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	srv.readQueue.start()
	defer srv.readQueue.stop()

	connID := "rq-conn"
	recv := make(chan []byte, 64)
	ctxConn, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := &Conn{
		ID:     connID,
		server: srv,
		send:   recv,
		ctx:    ctxConn,
		cancel: cancel,
		log:    zerolog.Nop(),
	}
	srv.conns.Store(connID, c)
	srv.subs.RegisterSender(connID, func(b []byte) bool {
		select {
		case recv <- b:
			return true
		default:
			return false
		}
	})
	if err := srv.subs.Add(connID, "sub1", []nostr.Filter{{Kinds: []int{1}}}); err != nil {
		t.Fatal(err)
	}
	openedUnix, ok := srv.subs.SubOpenedUnix(connID, "sub1")
	if !ok {
		t.Fatal("SubOpenedUnix")
	}

	state := newREQQueryState([]nostr.Filter{{Kinds: []int{1}}}, 0, false)
	page1, hasMore, err := fetchREQPage(ctx, st, state, pageSize)
	if err != nil || !hasMore {
		t.Fatalf("page1: hasMore=%v err=%v len=%d", hasMore, err, len(page1))
	}

	job := &reqPageJob{
		connID:     connID,
		subID:      "sub1",
		openedUnix: openedUnix,
		state:      state,
		pageSize:   pageSize,
	}
	if !srv.readQueue.Enqueue(job) {
		t.Fatal("enqueue failed")
	}

	deadline := time.After(5 * time.Second)
	eventCount := 0
	gotEOSE := false
	for !gotEOSE {
		select {
		case <-deadline:
			t.Fatalf("timeout: events=%d eose=%v", eventCount, gotEOSE)
		case b := <-recv:
			var msg []json.RawMessage
			if err := json.Unmarshal(b, &msg); err != nil {
				t.Fatal(err)
			}
			if len(msg) == 0 {
				continue
			}
			var typ string
			if err := json.Unmarshal(msg[0], &typ); err != nil {
				t.Fatal(err)
			}
			switch typ {
			case "EVENT":
				eventCount++
			case "EOSE":
				gotEOSE = true
			}
		}
	}
	if eventCount != 3 {
		t.Fatalf("want 3 events from queue pages, got %d", eventCount)
	}
}

func TestReaderQueueDropsJobWhenSubClosed(t *testing.T) {
	ctx := context.Background()
	st, closeStore, err := db.OpenTestStore(ctx, filepath.Join(t.TempDir(), "rq3.db"), zerolog.Nop())
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()

	createTestEvents(t, st, ctx, 6)

	cfg := testRelayConfig()
	pageSize := 2
	cfg.ConnectionLimits.QueryPageSize = &pageSize

	srv, err := NewServer(cfg, st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	srv.readQueue.start()
	defer srv.readQueue.stop()

	connID := "rq-close"
	recv := make(chan []byte, 16)
	ctxConn, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := &Conn{
		ID:     connID,
		server: srv,
		send:   recv,
		ctx:    ctxConn,
		cancel: cancel,
		log:    zerolog.Nop(),
	}
	srv.conns.Store(connID, c)
	srv.subs.RegisterSender(connID, func(b []byte) bool {
		select {
		case recv <- b:
			return true
		default:
			return false
		}
	})
	if err := srv.subs.Add(connID, "sub1", []nostr.Filter{{Kinds: []int{1}}}); err != nil {
		t.Fatal(err)
	}
	openedUnix, ok := srv.subs.SubOpenedUnix(connID, "sub1")
	if !ok {
		t.Fatal("SubOpenedUnix")
	}

	state := newREQQueryState([]nostr.Filter{{Kinds: []int{1}}}, 0, false)
	_, hasMore, err := fetchREQPage(ctx, st, state, pageSize)
	if err != nil || !hasMore {
		t.Fatalf("setup page1: hasMore=%v err=%v", hasMore, err)
	}

	srv.subs.Remove(connID, "sub1")

	var panicked bool
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()
		srv.readQueue.runJob(&reqPageJob{
			connID:     connID,
			subID:      "sub1",
			openedUnix: openedUnix,
			state:      state,
			pageSize:   pageSize,
		})
	}()
	wg.Wait()

	if panicked {
		t.Fatal("runJob panicked after CLOSE")
	}
	select {
	case <-recv:
		t.Fatal("expected no delivery after CLOSE")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestDrainRemainingPagesSyncFallback(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, closeStore, err := db.OpenTestStore(ctx, filepath.Join(dir, "rq4.db"), zerolog.Nop())
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()

	createTestEvents(t, st, ctx, 5)

	srv, err := NewServer(testRelayConfig(), st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}

	connID := "sync-drain"
	recv := make(chan []byte, 64)
	ctxConn, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := &Conn{
		ID:     connID,
		server: srv,
		send:   recv,
		ctx:    ctxConn,
		cancel: cancel,
		log:    zerolog.Nop(),
	}
	srv.conns.Store(connID, c)
	srv.subs.RegisterSender(connID, func(b []byte) bool {
		return c.enqueue(b) == nil
	})
	if err := srv.subs.Add(connID, "sub1", []nostr.Filter{{Kinds: []int{1}}}); err != nil {
		t.Fatal(err)
	}

	state := newREQQueryState([]nostr.Filter{{Kinds: []int{1}}}, 0, false)
	_, hasMore, err := fetchREQPage(ctx, st, state, 2)
	if err != nil || !hasMore {
		t.Fatalf("setup: hasMore=%v err=%v", hasMore, err)
	}

	drainRemainingPages(ctx, srv, c, "sub1", state, 2)

	eventCount := 0
	for {
		select {
		case b := <-recv:
			var msg []json.RawMessage
			if err := json.Unmarshal(b, &msg); err != nil {
				t.Fatal(err)
			}
			var typ string
			_ = json.Unmarshal(msg[0], &typ)
			if typ == "EVENT" {
				eventCount++
			}
		default:
			if eventCount != 3 {
				t.Fatalf("want 3 events from sync drain, got %d", eventCount)
			}
			return
		}
	}
}

type queryFaultStore struct {
	storage.Store
	queryErr error
}

func (f *queryFaultStore) QueryEvents(ctx context.Context, filters []nostr.Filter) ([]*nostr.Event, error) {
	if f.queryErr != nil {
		return nil, f.queryErr
	}
	return f.Store.QueryEvents(ctx, filters)
}

func TestReaderQueueDropsStaleJobOnSubReplacement(t *testing.T) {
	ctx := context.Background()
	st, closeStore, err := db.OpenTestStore(ctx, filepath.Join(t.TempDir(), "rq-replace.db"), zerolog.Nop())
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()

	createTestEvents(t, st, ctx, 6)

	cfg := testRelayConfig()
	pageSize := 2
	cfg.ConnectionLimits.QueryPageSize = &pageSize

	srv, err := NewServer(cfg, st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}

	connID := "rq-replace"
	recv := make(chan []byte, 16)
	ctxConn, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := &Conn{
		ID:     connID,
		server: srv,
		send:   recv,
		ctx:    ctxConn,
		cancel: cancel,
		log:    zerolog.Nop(),
	}
	srv.conns.Store(connID, c)
	srv.subs.RegisterSender(connID, func(b []byte) bool {
		return c.enqueue(b) == nil
	})
	filters := []nostr.Filter{{Kinds: []int{1}}}
	if err := srv.subs.Add(connID, "sub1", filters); err != nil {
		t.Fatal(err)
	}
	staleOpened, ok := srv.subs.SubOpenedUnix(connID, "sub1")
	if !ok {
		t.Fatal("SubOpenedUnix")
	}

	state := newREQQueryState(filters, 0, false)
	_, hasMore, err := fetchREQPage(ctx, st, state, pageSize)
	if err != nil || !hasMore {
		t.Fatalf("setup page1: hasMore=%v err=%v", hasMore, err)
	}

	time.Sleep(1100 * time.Millisecond)
	if err := srv.subs.Add(connID, "sub1", filters); err != nil {
		t.Fatal(err)
	}
	if srv.subs.IsSameSnapshot(connID, "sub1", staleOpened) {
		t.Fatal("expected replacement to change opened_unix")
	}

	srv.readQueue.runJob(&reqPageJob{
		connID:     connID,
		subID:      "sub1",
		openedUnix: staleOpened,
		state:      state,
		pageSize:   pageSize,
	})

	select {
	case <-recv:
		t.Fatal("expected no delivery for stale snapshot job")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestReaderQueueQueryErrorSendsClosed(t *testing.T) {
	ctx := context.Background()
	st, closeStore, err := db.OpenTestStore(ctx, filepath.Join(t.TempDir(), "rq-err.db"), zerolog.Nop())
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()

	createTestEvents(t, st, ctx, 6)

	cfg := testRelayConfig()
	pageSize := 2
	cfg.ConnectionLimits.QueryPageSize = &pageSize

	fault := &queryFaultStore{Store: st}
	srv, err := NewServer(cfg, fault, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}

	connID := "rq-err"
	recv := make(chan []byte, 16)
	ctxConn, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := &Conn{
		ID:     connID,
		server: srv,
		send:   recv,
		ctx:    ctxConn,
		cancel: cancel,
		log:    zerolog.Nop(),
	}
	srv.conns.Store(connID, c)
	srv.subs.RegisterSender(connID, func(b []byte) bool {
		return c.enqueue(b) == nil
	})
	filters := []nostr.Filter{{Kinds: []int{1}}}
	if err := srv.subs.Add(connID, "sub1", filters); err != nil {
		t.Fatal(err)
	}
	openedUnix, ok := srv.subs.SubOpenedUnix(connID, "sub1")
	if !ok {
		t.Fatal("SubOpenedUnix")
	}

	state := newREQQueryState(filters, 0, false)
	_, hasMore, err := fetchREQPage(ctx, st, state, pageSize)
	if err != nil || !hasMore {
		t.Fatalf("setup page1: hasMore=%v err=%v", hasMore, err)
	}

	fault.queryErr = errors.New("query failed")
	srv.readQueue.runJob(&reqPageJob{
		connID:     connID,
		subID:      "sub1",
		openedUnix: openedUnix,
		state:      state,
		pageSize:   pageSize,
	})

	deadline := time.After(2 * time.Second)
	gotClosed := false
	for !gotClosed {
		select {
		case <-deadline:
			t.Fatal("timeout waiting for CLOSED")
		case b := <-recv:
			var msg []json.RawMessage
			if err := json.Unmarshal(b, &msg); err != nil {
				t.Fatal(err)
			}
			var typ string
			if err := json.Unmarshal(msg[0], &typ); err != nil {
				t.Fatal(err)
			}
			if typ != "CLOSED" {
				continue
			}
			gotClosed = true
			if len(msg) < 3 {
				t.Fatalf("CLOSED message: %s", string(b))
			}
			var reason string
			if err := json.Unmarshal(msg[2], &reason); err != nil {
				t.Fatal(err)
			}
			if reason != "internal error" {
				t.Fatalf("CLOSED reason: got %q want %q", reason, "internal error")
			}
		}
	}
	if srv.subs.IsOpen(connID, "sub1") {
		t.Fatal("subscription should be removed after query error")
	}
}

func TestReaderQueueDropsJobOnDisconnect(t *testing.T) {
	ctx := context.Background()
	st, closeStore, err := db.OpenTestStore(ctx, filepath.Join(t.TempDir(), "rq-disc.db"), zerolog.Nop())
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()

	createTestEvents(t, st, ctx, 6)

	cfg := testRelayConfig()
	pageSize := 2
	cfg.ConnectionLimits.QueryPageSize = &pageSize

	srv, err := NewServer(cfg, st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}

	connID := "rq-disc"
	recv := make(chan []byte, 16)
	ctxConn, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := &Conn{
		ID:     connID,
		server: srv,
		send:   recv,
		ctx:    ctxConn,
		cancel: cancel,
		log:    zerolog.Nop(),
	}
	srv.conns.Store(connID, c)
	srv.subs.RegisterSender(connID, func(b []byte) bool {
		return c.enqueue(b) == nil
	})
	filters := []nostr.Filter{{Kinds: []int{1}}}
	if err := srv.subs.Add(connID, "sub1", filters); err != nil {
		t.Fatal(err)
	}
	openedUnix, ok := srv.subs.SubOpenedUnix(connID, "sub1")
	if !ok {
		t.Fatal("SubOpenedUnix")
	}

	state := newREQQueryState(filters, 0, false)
	_, hasMore, err := fetchREQPage(ctx, st, state, pageSize)
	if err != nil || !hasMore {
		t.Fatalf("setup page1: hasMore=%v err=%v", hasMore, err)
	}

	srv.conns.Delete(connID)

	var panicked bool
	func() {
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()
		srv.readQueue.runJob(&reqPageJob{
			connID:     connID,
			subID:      "sub1",
			openedUnix: openedUnix,
			state:      state,
			pageSize:   pageSize,
		})
	}()
	if panicked {
		t.Fatal("runJob panicked after disconnect")
	}
	select {
	case <-recv:
		t.Fatal("expected no delivery after disconnect")
	case <-time.After(100 * time.Millisecond):
	}
}
