package relay

import (
	"context"
	"sync"

	"github.com/michmich112/congee/internal/nostr"
)

// EventValidator runs one validation step on an event before storage.
type EventValidator interface {
	Validate(ctx context.Context, conn *Conn, ev *nostr.Event) error
}

// EventValidatorFunc adapts a function to EventValidator.
type EventValidatorFunc func(ctx context.Context, conn *Conn, ev *nostr.Event) error

// Validate implements EventValidator.
func (f EventValidatorFunc) Validate(ctx context.Context, conn *Conn, ev *nostr.Event) error {
	return f(ctx, conn, ev)
}

// ValidatorChain is an ordered list of validators (e.g. NIP-01 ID+signature).
type ValidatorChain struct {
	mu         sync.RWMutex
	validators []EventValidator
}

// Append adds validators to the chain in order.
func (c *ValidatorChain) Append(v ...EventValidator) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.validators = append(c.validators, v...)
}

// Validate runs all validators; the first error stops the chain.
func (c *ValidatorChain) Validate(ctx context.Context, conn *Conn, ev *nostr.Event) error {
	c.mu.RLock()
	list := c.validators
	c.mu.RUnlock()
	for _, v := range list {
		if err := v.Validate(ctx, conn, ev); err != nil {
			return err
		}
	}
	return nil
}
