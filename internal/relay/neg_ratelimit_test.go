package relay

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/db"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/rs/zerolog"
)

// testRelayConfigNIP77 returns the base relay config with NIP-77 enabled so
// handleNEGOpen reaches its handler-side rate limit.
func testRelayConfigNIP77() *config.Config {
	cfg := testRelayConfig()
	cfg.NIPs.Enabled = []int{1, 11, 77}
	return cfg
}

// newTestNegConn builds a Conn wired to srv for driving dispatchPayload with
// negentropy messages. negOpenMax/negMsgMax override the per-message limiter.
func newTestNegConn(t *testing.T, srv *Server, negOpenMax, negMsgMax int) *Conn {
	t.Helper()
	cfg := testRelayConfigNIP77()
	lim := newConnLimiter(cfg)
	lim.negOpen = newSlidingEvents(time.Minute, negOpenMax)
	lim.negMsg = newSlidingEvents(time.Minute, negMsgMax)

	ctxConn, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	return &Conn{
		ID:           "neg-test",
		server:       srv,
		send:         make(chan []byte, 64),
		ctx:          ctxConn,
		cancel:       cancel,
		limiter:      lim,
		negSessions:  newNegSessionMap(),
		negOpenTotal: atomic.Uint64{},
		negMsgTotal:  atomic.Uint64{},
	}
}

// TestNegOpenRateLimitConsumesOneTokenPerMessage guards against double
// consumption: dispatchPayload rate-limits NEG-OPEN and handleNEGOpen
// rate-limits it again, so a single legitimate message burned two tokens and
// halved the effective per-minute cap.
func TestNegOpenRateLimitConsumesOneTokenPerMessage(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, closeStore, err := db.OpenTestStore(ctx, filepath.Join(dir, "neg.db"), zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()

	srv, err := NewServer(testRelayConfigNIP77(), st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	srv.negQueue = newNegQueue(srv)
	srv.registry.Register("NEG-OPEN", func(ctx context.Context, c *Conn, msg any) error {
		return handleNEGOpen(ctx, srv, c, msg.(*nostr.NegOpenMessage))
	})

	c := newTestNegConn(t, srv, 100, 100)
	c.dispatchPayload([]byte(`["NEG-OPEN","s1",{"kinds":[1]},"6161"]`))

	if got := len(c.limiter.negOpen.stamps); got != 1 {
		t.Fatalf("one NEG-OPEN consumed %d rate-limit tokens, want 1 (effective cap is halved)", got)
	}
}

// TestNegMsgRateLimitConsumesOneTokenPerMessage guards against double
// consumption on NEG-MSG: dispatchPayload rate-limits the message and
// handleNEGMsg rate-limits it again.
func TestNegMsgRateLimitConsumesOneTokenPerMessage(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, closeStore, err := db.OpenTestStore(ctx, filepath.Join(dir, "neg.db"), zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()

	srv, err := NewServer(testRelayConfigNIP77(), st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	srv.negQueue = newNegQueue(srv)
	srv.registry.Register("NEG-MSG", func(ctx context.Context, c *Conn, msg any) error {
		return handleNEGMsg(ctx, srv, c, msg.(*nostr.NegMsgMessage))
	})

	c := newTestNegConn(t, srv, 100, 100)
	// No session for subID: handleNEGMsg still consumes its rate-limit token
	// before rejecting, so the stamps count reflects dispatch + handler.
	c.dispatchPayload([]byte(`["NEG-MSG","s1","6161"]`))

	if got := len(c.limiter.negMsg.stamps); got != 1 {
		t.Fatalf("one NEG-MSG consumed %d rate-limit tokens, want 1 (effective cap is halved)", got)
	}
}
