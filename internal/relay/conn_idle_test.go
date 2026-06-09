package relay

import (
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func testIdleServer() *Server {
	cfg := testRelayConfig()
	return &Server{
		cfg:  cfg,
		subs: NewSubscriptionManager(cfg, zerolog.Nop()),
	}
}

func TestConnIdleClockEventExempt(t *testing.T) {
	t.Parallel()
	srv := testIdleServer()
	c := &Conn{
		server:      srv,
		ID:          "abc",
		startedUnix: time.Now().Unix(),
	}
	c.initIdleClock()
	if !c.idleEligible() {
		t.Fatal("expected idle eligible on connect")
	}
	c.noteClientEvent()
	if c.idleEligible() {
		t.Fatal("expected exempt after client event")
	}
}

func TestConnIdleClockSubscriptionExempt(t *testing.T) {
	t.Parallel()
	srv := testIdleServer()
	c := &Conn{
		server:      srv,
		ID:          "def",
		startedUnix: time.Now().Unix(),
	}
	c.initIdleClock()
	srv.subs.Add(c.ID, "sub1", nil)
	c.noteSubscriptionCount(srv.subs.SubCount(c.ID))
	if c.idleEligible() {
		t.Fatal("expected exempt while subscription open")
	}
	srv.subs.Remove(c.ID, "sub1")
	c.noteSubscriptionCount(srv.subs.SubCount(c.ID))
	if !c.idleEligible() {
		t.Fatal("expected idle eligible after last sub closed with no events")
	}
}

func TestConnIdleClockEventPreventsRestart(t *testing.T) {
	t.Parallel()
	srv := testIdleServer()
	c := &Conn{
		server:      srv,
		ID:          "ghi",
		startedUnix: time.Now().Unix(),
	}
	c.initIdleClock()
	c.clientEventTotal.Add(1)
	c.noteClientEvent()
	srv.subs.Add(c.ID, "sub1", nil)
	c.noteSubscriptionCount(srv.subs.SubCount(c.ID))
	srv.subs.Remove(c.ID, "sub1")
	c.noteSubscriptionCount(srv.subs.SubCount(c.ID))
	if c.idleEligible() {
		t.Fatal("expected still exempt after sub closed when event was sent")
	}
}
