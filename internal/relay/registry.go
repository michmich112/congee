package relay

import (
	"context"
	"fmt"
	"sync"

	"github.com/michmich112/congee/internal/nostr"
)

// MessageHandler handles a parsed NIP-01 client message for one connection.
type MessageHandler func(ctx context.Context, c *Conn, msg any) error

// Registry maps NIP-01 command names ("EVENT", "REQ", "CLOSE") to handlers.
type Registry struct {
	mu       sync.RWMutex
	handlers map[string]MessageHandler
}

// NewRegistry returns an empty handler registry.
func NewRegistry() *Registry {
	return &Registry{handlers: make(map[string]MessageHandler)}
}

// Register associates a message type with a handler (replaces any prior).
func (r *Registry) Register(typ string, h MessageHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[typ] = h
}

// Dispatch runs the handler for the parsed message.
func (r *Registry) Dispatch(ctx context.Context, c *Conn, msg any) error {
	var typ string
	switch msg.(type) {
	case *nostr.EventMessage:
		typ = "EVENT"
	case *nostr.ReqMessage:
		typ = "REQ"
	case *nostr.CloseMessage:
		typ = "CLOSE"
	default:
		return fmt.Errorf("relay: unknown message type %T", msg)
	}
	r.mu.RLock()
	h := r.handlers[typ]
	r.mu.RUnlock()
	if h == nil {
		return fmt.Errorf("relay: no handler for %q", typ)
	}
	return h(ctx, c, msg)
}
