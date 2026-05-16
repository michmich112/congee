package relay

import (
	"context"
	"fmt"
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

type namedPostHook struct {
	name string
	fn   PostStoreHook
}

// HookChain runs registered hooks in registration order.
// Hook names appear in errors from Run as "post_hook <name>: ..." for observability.
type HookChain struct {
	mu    sync.RWMutex
	hooks []namedPostHook
}

// Append registers a named hook (stable grep-friendly name, e.g. nip01_audit_event).
func (c *HookChain) Append(name string, h PostStoreHook) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hooks = append(c.hooks, namedPostHook{name: name, fn: h})
}

// Prepend registers hooks to run before those added with Append.
func (c *HookChain) Prepend(name string, h PostStoreHook) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hooks = append([]namedPostHook{{name: name, fn: h}}, c.hooks...)
}

// Run executes all hooks; the first error is returned wrapped with the hook name.
func (c *HookChain) Run(ctx context.Context, env HookEnv) error {
	c.mu.RLock()
	list := c.hooks
	c.mu.RUnlock()
	for _, nh := range list {
		if err := nh.fn(ctx, env); err != nil {
			return fmt.Errorf("post_hook %s: %w", nh.name, err)
		}
	}
	return nil
}
