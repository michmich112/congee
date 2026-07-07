package relay

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func testConnWithPipe(t *testing.T) (*Conn, net.Conn, func()) {
	t.Helper()
	client, server := net.Pipe()
	cfg := testRelayConfig()
	srv := &Server{
		cfg:  cfg,
		subs: NewSubscriptionManager(cfg, zerolog.Nop()),
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &Conn{
		ID:          "test-conn",
		server:      srv,
		nc:          server,
		send:        make(chan []byte, 8),
		writerDone:  make(chan struct{}),
		ctx:         ctx,
		cancel:      cancel,
		log:         zerolog.Nop(),
		startedUnix: time.Now().Unix(),
	}
	c.initIdleClock()
	cleanup := func() {
		c.initiateShutdown()
		_ = client.Close()
		cancel()
	}
	return c, client, cleanup
}

func TestInitiateShutdownUnblocksReadLoop(t *testing.T) {
	t.Parallel()
	c, _, cleanup := testConnWithPipe(t)
	defer cleanup()

	go c.writeLoop()
	done := make(chan struct{})
	go func() {
		c.readLoopPlain()
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	c.initiateShutdown()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("read loop did not exit after initiateShutdown")
	}
}

func TestInitiateShutdownUnblocksWriteLoop(t *testing.T) {
	t.Parallel()
	c, client, cleanup := testConnWithPipe(t)
	defer cleanup()

	go c.writeLoop()
	if err := c.enqueue([]byte("hello")); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	// Block client reads so the write loop stalls in WriteServerMessage.
	block := make(chan struct{})
	go func() {
		buf := make([]byte, 4096)
		for {
			select {
			case <-block:
				return
			default:
				_, _ = client.Read(buf)
			}
		}
	}()

	time.Sleep(20 * time.Millisecond)
	c.initiateShutdown()
	close(block)

	if !c.waitWriterDone(2 * time.Second) {
		t.Fatal("write loop did not exit after initiateShutdown")
	}
}

func TestInitiateShutdownIdempotent(t *testing.T) {
	t.Parallel()
	c, _, cleanup := testConnWithPipe(t)
	defer cleanup()

	c.initiateShutdown()
	c.initiateShutdown()

	c.sendMu.Lock()
	closed := c.outboundClosed
	c.sendMu.Unlock()
	if !closed {
		t.Fatal("expected outbound channel to be closed")
	}
}

func TestSweepIdleConnectionsInitiatesShutdown(t *testing.T) {
	t.Parallel()
	c, _, cleanup := testConnWithPipe(t)
	defer cleanup()

	cfg := testRelayConfig()
	cfg.ConnectionLimits.IdleNoEventNoSubSeconds = 1
	srv := testIdleServer()
	srv.cfg = cfg

	c.server = srv
	atomic.StoreInt64(&c.idleSinceUnix, time.Now().Unix()-10)
	srv.conns.Store(c.ID, c)

	go c.writeLoop()
	srv.sweepIdleConnections(1)

	c.sendMu.Lock()
	closed := c.outboundClosed
	c.sendMu.Unlock()
	if !closed {
		t.Fatal("expected idle sweep to initiate shutdown")
	}
}
