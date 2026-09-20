// Package plugin is the Congee plugin ABI: gRPC/protobuf types, a Handler interface, and Serve.
package plugin

import (
	"context"
	"encoding/json"
)

// APIVersion is the host/plugin handshake ABI. Handshake must send and receive 1.
const APIVersion = 1

// Env vars set by the Congee host when spawning a plugin process.
const (
	EnvPluginSocket     = "CONGEE_PLUGIN_SOCKET"
	EnvPluginHostSocket = "CONGEE_PLUGIN_HOST_SOCKET"
	EnvPluginDataDir    = "CONGEE_PLUGIN_DATA_DIR"
	EnvPluginSettings   = "CONGEE_PLUGIN_SETTINGS"
	EnvPluginID         = "CONGEE_PLUGIN_ID"
)

// Capabilities declared at handshake. Host enforces them.
const (
	CapObserve     = "messages.observe"
	CapIntercept   = "req.intercept"
	CapEventsRead  = "events.read"
	CapIndexOwn    = "index.own"
	CapAdminUI     = "admin.ui"
)

// Event is a NIP-01 event on the plugin ABI (not Congee internal/nostr).
type Event struct {
	ID        string     `json:"id"`
	PubKey    string     `json:"pubkey"`
	CreatedAt int64      `json:"created_at"`
	Kind      int        `json:"kind"`
	Tags      [][]string `json:"tags"`
	Content   string     `json:"content"`
	Sig       string     `json:"sig"`
}

// Filter is a NIP-01 filter. Tag keys are single letters without '#'.
type Filter struct {
	IDs     []string
	Authors []string
	Kinds   []int
	Since   *int64
	Until   *int64
	Limit   *int
	Search  string
	Tags    map[string][]string
}

// Req is a client REQ (subscription id + filters).
type Req struct {
	SubID   string
	Filters []Filter
}

// ObserveMessage is a listen-only copy of an inbound client message.
type ObserveMessage struct {
	Type    string // EVENT, REQ, CLOSE, AUTH
	Event   *Event
	SubID   string
	Filters []Filter
}

// TrafficSubscription is a host-side match rule. Empty kinds/types match nothing.
type TrafficSubscription struct {
	MessageTypes   []string
	Kinds          []int
	ReqHasSearch   bool
	ReqTagNames    []string
	InterceptREQ   bool
	Observe        bool
	OnStoredEvent  bool
}

// HandshakeResult is returned from Handler.Handshake.
type HandshakeResult struct {
	PluginID             string
	Name                 string
	Version              string
	Capabilities         []string
	Subscriptions        []TrafficSubscription
	InterceptDeadlineMs  int
}

// InterceptAction is the plugin decision for a REQ.
type InterceptAction int

const (
	InterceptPassthrough InterceptAction = iota
	InterceptReshapeREQ
	InterceptRespond
)

// InterceptResult is returned from Handler.InterceptREQ.
type InterceptResult struct {
	Action               InterceptAction
	ReshapeFilters       []Filter
	EventIDs             []string
	SubscriptionFilters  []Filter
}

// Status is plugin health and UI JSON.
type Status struct {
	Ready bool
	JSON  json.RawMessage
}

// Host is the Congee-side RPC the plugin may call (events.read).
type Host interface {
	QueryEvents(ctx context.Context, filters []Filter) ([]Event, error)
	GetEventsByIDs(ctx context.Context, ids []string) ([]Event, error)
	Log(ctx context.Context, level, message string, fields map[string]string) error
}

// Handler is implemented by a plugin process.
type Handler interface {
	Handshake(ctx context.Context, settings json.RawMessage) (*HandshakeResult, error)
	Health(ctx context.Context) (ready bool, message string, err error)
	Observe(ctx context.Context, msg ObserveMessage) error
	InterceptREQ(ctx context.Context, req Req) (*InterceptResult, error)
	OnStoredEvent(ctx context.Context, ev Event, stored bool) error
	ApplySettings(ctx context.Context, settings json.RawMessage) ([]TrafficSubscription, error)
	AdminAction(ctx context.Context, name string, payload json.RawMessage) (json.RawMessage, error)
	Status(ctx context.Context) (*Status, error)
}

// HandlerHost is an optional Handler that receives the Host client after Serve dials.
type HandlerHost interface {
	SetHost(h Host)
}
