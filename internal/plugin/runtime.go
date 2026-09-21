package plugin

import (
	"context"

	"github.com/michmich112/congee/internal/nostr"
)

// InterceptAction mirrors the SDK intercept decision.
type InterceptAction int

const (
	InterceptPassthrough InterceptAction = iota
	InterceptReshapeREQ
	InterceptRespond
)

// InterceptResult is returned to the relay REQ handler.
type InterceptResult struct {
	Action              InterceptAction
	Filters             []nostr.Filter
	EventIDs            []string
	SubscriptionFilters []nostr.Filter
}

// Runtime is the relay-facing plugin surface. All methods are nil-safe on a nil Runtime.
type Runtime interface {
	// Observe enqueues a listen-only copy. Never blocks the caller.
	Observe(msg any)
	// InterceptREQ is the only synchronous plugin call on the REQ path.
	InterceptREQ(ctx context.Context, req *nostr.ReqMessage) InterceptResult
	// EnqueueStoredEvent enqueues an accepted EVENT for index plugins. Never blocks.
	// Called for WebSocket EVENT post-hooks and for NIP-77 imported events.
	EnqueueStoredEvent(ev *nostr.Event, stored bool)
	// Snapshot returns admin-facing plugin rows.
	Snapshot() []InstanceSnapshot
}
