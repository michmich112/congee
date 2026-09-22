package nip77

import (
	"sync"

	"github.com/nbd-wtf/go-nostr/nip77/negentropy"
	"github.com/nbd-wtf/go-nostr/nip77/negentropy/storage/vector"
)

// SyncClient is the client-role negentropy session. It drains Haves and HaveNots
// while Reconcile runs. go-nostr sends those IDs on buffered channels (capacity 64);
// a Reconcile that discovers more IDs blocks forever unless a consumer is running.
//
// Each drainer keeps reading after a panic in the per-ID hook or append path so a
// consumer crash cannot fill the buffer and stall Reconcile.
type SyncClient struct {
	neg *negentropy.Negentropy

	haves    []string
	haveNots []string

	havesDone    chan struct{}
	haveNotsDone chan struct{}
	stop         sync.Once
}

// SyncClientOption configures a SyncClient.
type SyncClientOption func(*syncClientHooks)

type syncClientHooks struct {
	onHaveNot func(id string)
}

// WithHaveNotObserver runs fn for each non-empty HaveNot after it is recorded.
// A panic inside fn is recovered by the drainer, which keeps reading.
func WithHaveNotObserver(fn func(id string)) SyncClientOption {
	return func(h *syncClientHooks) {
		h.onHaveNot = fn
	}
}

// NewSyncClient seals client-side reconciliation and starts ID drainers immediately.
func NewSyncClient(vec *vector.Vector, frameSizeLimit int, opts ...SyncClientOption) *SyncClient {
	var hooks syncClientHooks
	for _, opt := range opts {
		if opt != nil {
			opt(&hooks)
		}
	}
	neg := negentropy.New(vec, frameSizeLimit)
	c := &SyncClient{
		neg:          neg,
		havesDone:    make(chan struct{}),
		haveNotsDone: make(chan struct{}),
	}
	go c.drain(neg.Haves, &c.haves, c.havesDone, nil)
	go c.drain(neg.HaveNots, &c.haveNots, c.haveNotsDone, hooks.onHaveNot)
	return c
}

func (c *SyncClient) drain(ch <-chan string, dst *[]string, done chan struct{}, hook func(string)) {
	defer close(done)
	seen := make(map[string]struct{})
	for {
		id, ok := <-ch
		if !ok {
			return
		}
		func() {
			defer func() { _ = recover() }()
			if id == "" {
				return
			}
			if _, dup := seen[id]; dup {
				return
			}
			seen[id] = struct{}{}
			*dst = append(*dst, id)
			if hook != nil {
				hook(id)
			}
		}()
	}
}

// Start builds the initial NEG-OPEN hex message.
func (c *SyncClient) Start() string {
	return c.neg.Start()
}

// Reconcile continues the client session. An empty result means the peer set is
// fully reconciled and the ID channels have been closed by go-nostr.
func (c *SyncClient) Reconcile(msg string) (string, error) {
	return c.neg.Reconcile(msg)
}

// NeedIDs returns IDs the upstream has and this client does not.
// It waits until the HaveNots channel is closed (session finished or Stop).
func (c *SyncClient) NeedIDs() []string {
	<-c.haveNotsDone
	out := make([]string, len(c.haveNots))
	copy(out, c.haveNots)
	return out
}

// HaveCount returns how many IDs this client has that the upstream does not.
// It waits until the Haves channel is closed (session finished or Stop).
func (c *SyncClient) HaveCount() int {
	<-c.havesDone
	return len(c.haves)
}

// Stop closes the ID channels when the session is abandoned before go-nostr
// closes them (timeout or reconcile error) and waits for the drainers to exit.
// Safe to call after a finished Reconcile, and safe to call more than once.
// Do not call Stop concurrently with Reconcile.
func (c *SyncClient) Stop() {
	if c == nil || c.neg == nil {
		return
	}
	c.stop.Do(func() {
		closeChan(c.neg.Haves)
		closeChan(c.neg.HaveNots)
		<-c.havesDone
		<-c.haveNotsDone
	})
}

func closeChan(ch chan string) {
	defer func() { _ = recover() }()
	close(ch)
}
