package plugin

import (
	"errors"

	"github.com/michmich112/congee/internal/nostr"
)

// ConnInfo exposes read-only connection state for plugin phases.
type ConnInfo interface {
	ID() string
	PeerIP() string
	HasAuth() bool
	AuthedPubkeys() []string
}

// EventContext is passed through EVENT pipeline phases.
// Event is the immutable signed client event; Values is per-request scratch space.
type EventContext struct {
	Conn   ConnInfo
	Event  *nostr.Event
	Stored bool
	Values map[string]any
}

// ReqContext is passed through REQ pipeline phases.
// Filters may be narrowed by TransformReq; Values is immutable after transform.
type ReqContext struct {
	Conn    ConnInfo
	SubID   string
	Filters []nostr.Filter
	Values  map[string]any
}

// ErrReject is the sentinel for errors.Is when a plugin rejects an operation.
var ErrReject = errors.New("plugin: reject")

// Reject signals the relay should reject with Reason as the client message.
type Reject struct {
	Reason string
}

func (r Reject) Error() string {
	if r.Reason == "" {
		return ErrReject.Error()
	}
	return r.Reason
}

func (Reject) Is(target error) bool {
	return target == ErrReject
}

func (r Reject) As(target any) bool {
	p, ok := target.(*Reject)
	if !ok {
		return false
	}
	*p = r
	return true
}

// ErrAuthRequired is the sentinel for errors.Is when a plugin requires authentication.
var ErrAuthRequired = errors.New("plugin: auth required")

// AuthRequired signals the relay should challenge AUTH and reject with Reason.
type AuthRequired struct {
	Reason string
}

func (a AuthRequired) Error() string {
	if a.Reason == "" {
		return ErrAuthRequired.Error()
	}
	return a.Reason
}

func (AuthRequired) Is(target error) bool {
	return target == ErrAuthRequired
}

func (a AuthRequired) As(target any) bool {
	p, ok := target.(*AuthRequired)
	if !ok {
		return false
	}
	*p = a
	return true
}
