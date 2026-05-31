package plugin

import (
	"context"

	"github.com/michmich112/congee/internal/nostr"
)

// EventValidator runs before an EVENT is stored.
type EventValidator interface {
	ValidateEvent(ctx context.Context, ec *EventContext) error
}

// EventStoredHook runs after an EVENT is stored (derive relay-signed events, side effects).
type EventStoredHook interface {
	OnEventStored(ctx context.Context, ec *EventContext) error
}

// ReqTransformer narrows REQ filters before query execution (intersect-only).
type ReqTransformer interface {
	TransformReq(ctx context.Context, rc *ReqContext) error
}

// ReqVisibility gates whether an event may be delivered for a subscription filter.
type ReqVisibility interface {
	EventVisible(ctx context.Context, rc *ReqContext, ev *nostr.Event) (bool, error)
}

// ReqQueryProvider owns query execution for matched REQ filters (e.g. NIP-50 search).
// When handled is true, the relay uses events and skips the default store query.
type ReqQueryProvider interface {
	QueryReq(ctx context.Context, rc *ReqContext) (events []*nostr.Event, handled bool, err error)
}
