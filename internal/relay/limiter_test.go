package relay

import (
	"testing"
	"time"

	"github.com/michmich112/congee/internal/config"
)

func TestSlidingEvents_allow(t *testing.T) {
	s := newSlidingEvents(time.Minute, 2)
	if !s.allow() || !s.allow() {
		t.Fatal("expected first two allows")
	}
	if s.allow() {
		t.Fatal("expected third deny within window")
	}
}

func TestByteWindow_allow(t *testing.T) {
	b := newByteWindow(time.Second, 100)
	if !b.allow(50) || !b.allow(50) {
		t.Fatal("expected 50+50")
	}
	if b.allow(1) {
		t.Fatal("expected over cap")
	}
}

func TestHubAllowNewConnection(t *testing.T) {
	cfg := &config.Config{
		ConnectionLimits: config.ConnectionLimitsSection{ConnectionsPerMinutePerIP: 2},
		RateLimits: config.RateLimitsSection{
			EventsPerMinutePerConnection: 10,
			BytesPerSecondPerConnection:  10000,
			ReqsPerMinutePerConnection:   10,
			MessagesPerMinutePerIP:         100,
		},
	}
	h := NewLimiterHub(cfg)
	ip := "127.0.0.1"
	if !h.AllowNewConnection(ip) || !h.AllowNewConnection(ip) {
		t.Fatal("expected two connection opens")
	}
	if h.AllowNewConnection(ip) {
		t.Fatal("expected third connection denied")
	}
}

func TestConnLimiterInboundAndEvent(t *testing.T) {
	cfg := &config.Config{
		RateLimits: config.RateLimitsSection{
			EventsPerMinutePerConnection: 1,
			BytesPerSecondPerConnection:  1000,
			ReqsPerMinutePerConnection:   2,
			MessagesPerMinutePerIP:       10,
		},
		ConnectionLimits: config.ConnectionLimitsSection{ConnectionsPerMinutePerIP: 10},
	}
	cl := newConnLimiter(cfg)
	if !cl.AllowInboundBytes(10) || !cl.AllowEvent() {
		t.Fatal("first event")
	}
	if cl.AllowEvent() {
		t.Fatal("second event should fail")
	}
	if !cl.AllowReq() || !cl.AllowReq() {
		t.Fatal("reqs")
	}
	if cl.AllowReq() {
		t.Fatal("third req should fail")
	}
}
