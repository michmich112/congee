package relay

import (
	"context"
	"sync"

	"github.com/michmich112/congee/internal/nostr"
)

// HookEnv is passed to each post-accept hook after an event is validated.
type HookEnv struct {
	Conn   *Conn
	Event  *nostr.Event
	Stored bool
}

// PostStoreHook runs after a valid EVENT is accepted (stored or ephemeral fan-out).
type PostStoreHook func(ctx context.Context, env HookEnv) error

// HookChain runs registered hooks in registration order.
type HookChain struct {
	mu    sync.RWMutex
	hooks []PostStoreHook
}

// Append registers hooks.
func (c *HookChain) Append(h ...PostStoreHook) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hooks = append(c.hooks, h...)
}

// Prepend registers hooks to run before those added with Append (same registration order among prepended hooks).
func (c *HookChain) Prepend(h ...PostStoreHook) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hooks = append(append([]PostStoreHook(nil), h...), c.hooks...)
}

// Run executes all hooks; the first error is returned.
func (c *HookChain) Run(ctx context.Context, env HookEnv) error {
	c.mu.RLock()
	list := c.hooks
	c.mu.RUnlock()
	for _, h := range list {
		if err := h(ctx, env); err != nil {
			return err
		}
	}
	return nil
}
