package relay

import "sync"

// ipConnTracker counts open WebSocket connections per resolved peer IP.
type ipConnTracker struct {
	mu     sync.Mutex
	counts map[string]int
}

func newIPConnTracker() *ipConnTracker {
	return &ipConnTracker{counts: make(map[string]int)}
}

// tryAcquire reserves one open slot for ip when below max. Returns the new count
// for ip and whether the reservation succeeded.
func (t *ipConnTracker) tryAcquire(ip string, max int) (openForIP int, ok bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	n := t.counts[ip]
	if max > 0 && n >= max {
		return n, false
	}
	n++
	t.counts[ip] = n
	return n, true
}

// release drops one open connection for ip (no-op when already zero).
func (t *ipConnTracker) release(ip string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	n := t.counts[ip]
	if n <= 1 {
		delete(t.counts, ip)
		return
	}
	t.counts[ip] = n - 1
}

// openCount returns how many connections are currently open for ip.
func (t *ipConnTracker) openCount(ip string) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.counts[ip]
}
