package relay

import (
	"sync"
	"time"

	"github.com/michmich112/congee/internal/config"
)

// slidingEvents counts events in a rolling time window (minute or second).
type slidingEvents struct {
	mu     sync.Mutex
	window time.Duration
	max    int
	stamps []time.Time
}

func newSlidingEvents(window time.Duration, max int) *slidingEvents {
	return &slidingEvents{window: window, max: max}
}

func (s *slidingEvents) allow() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-s.window)
	i := 0
	for i < len(s.stamps) && s.stamps[i].Before(cutoff) {
		i++
	}
	s.stamps = append(s.stamps[:0], s.stamps[i:]...)
	if len(s.stamps) >= s.max {
		return false
	}
	s.stamps = append(s.stamps, now)
	return true
}

// byteWindow enforces max bytes summed over the last `window` (e.g. 1 second).
type byteWindow struct {
	mu       sync.Mutex
	window   time.Duration
	maxBytes int
	entries  []struct {
		t time.Time
		n int
	}
}

func newByteWindow(window time.Duration, maxBytes int) *byteWindow {
	return &byteWindow{window: window, maxBytes: maxBytes}
}

func (b *byteWindow) allow(n int) bool {
	if n <= 0 {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-b.window)
	i := 0
	sum := 0
	for i < len(b.entries) && b.entries[i].t.Before(cutoff) {
		i++
	}
	b.entries = append(b.entries[:0], b.entries[i:]...)
	for _, e := range b.entries {
		sum += e.n
	}
	if sum+n > b.maxBytes {
		return false
	}
	b.entries = append(b.entries, struct {
		t time.Time
		n int
	}{t: now, n: n})
	return true
}

// ipWindows holds per-IP sliding windows shared across connections.
type ipWindows struct {
	mu         sync.Mutex
	messages   map[string]*slidingEvents
	connections map[string]*slidingEvents
}

func newIPWindows() *ipWindows {
	return &ipWindows{
		messages:    make(map[string]*slidingEvents),
		connections: make(map[string]*slidingEvents),
	}
}

func (w *ipWindows) messageWindow(ip string, maxPerMinute int) *slidingEvents {
	w.mu.Lock()
	defer w.mu.Unlock()
	if x, ok := w.messages[ip]; ok {
		return x
	}
	x := newSlidingEvents(time.Minute, maxPerMinute)
	w.messages[ip] = x
	return x
}

func (w *ipWindows) connectionWindow(ip string, maxPerMinute int) *slidingEvents {
	w.mu.Lock()
	defer w.mu.Unlock()
	if x, ok := w.connections[ip]; ok {
		return x
	}
	x := newSlidingEvents(time.Minute, maxPerMinute)
	w.connections[ip] = x
	return x
}

// ConnLimiter is per-WebSocket-connection rate state.
type ConnLimiter struct {
	events  *slidingEvents
	reqs    *slidingEvents
	negOpen *slidingEvents
	negMsg  *slidingEvents
	bytes   *byteWindow
}

func newConnLimiter(cfg *config.Config) *ConnLimiter {
	negOpenMax := config.EffectiveNIP77NegOpenPerMinute(cfg)
	negMsgMax := config.EffectiveNIP77NegMsgPerMinute(cfg)
	if negOpenMax <= 0 {
		negOpenMax = 1 << 30
	}
	if negMsgMax <= 0 {
		negMsgMax = 1 << 30
	}
	return &ConnLimiter{
		events:  newSlidingEvents(time.Minute, cfg.RateLimits.EventsPerMinutePerConnection),
		reqs:    newSlidingEvents(time.Minute, cfg.RateLimits.ReqsPerMinutePerConnection),
		negOpen: newSlidingEvents(time.Minute, negOpenMax),
		negMsg:  newSlidingEvents(time.Minute, negMsgMax),
		bytes:   newByteWindow(time.Second, cfg.RateLimits.BytesPerSecondPerConnection),
	}
}

// Hub coordinates IP-level and constructs per-connection limiters.
type Hub struct {
	cfg *config.Config
	ip  *ipWindows
}

// NewLimiterHub builds rate limit state from config.
func NewLimiterHub(cfg *config.Config) *Hub {
	return &Hub{cfg: cfg, ip: newIPWindows()}
}

// AllowNewConnection reports whether a new TCP/WebSocket from ip is allowed.
func (h *Hub) AllowNewConnection(ip string) bool {
	return h.ip.connectionWindow(ip, h.cfg.ConnectionLimits.ConnectionsPerMinutePerIP).allow()
}

// AllowMessage counts one inbound message toward the IP cap (all WS messages).
func (h *Hub) AllowMessage(ip string) bool {
	return h.ip.messageWindow(ip, h.cfg.RateLimits.MessagesPerMinutePerIP).allow()
}

// NewConnLimiter returns a new per-connection limiter.
func (h *Hub) NewConnLimiter() *ConnLimiter {
	return newConnLimiter(h.cfg)
}

// AllowInboundBytes counts raw message bytes against the per-second cap.
func (cl *ConnLimiter) AllowInboundBytes(n int) bool {
	return cl.bytes.allow(n)
}

// AllowEvent counts one EVENT against the per-minute cap (after AllowInboundBytes).
func (cl *ConnLimiter) AllowEvent() bool {
	return cl.events.allow()
}

// AllowReq counts one REQ against the per-minute cap.
func (cl *ConnLimiter) AllowReq() bool {
	return cl.reqs.allow()
}

// AllowNegOpen counts one NEG-OPEN against the NIP-77 per-minute cap.
func (cl *ConnLimiter) AllowNegOpen() bool {
	return cl.negOpen.allow()
}

// AllowNegMsg counts one NEG-MSG against the NIP-77 per-minute cap.
func (cl *ConnLimiter) AllowNegMsg() bool {
	return cl.negMsg.allow()
}
