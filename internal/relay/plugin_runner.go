package relay

import (
	"context"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/plugin"
)

// PluginRunner executes registered plugin pipeline phases (implemented by nips/registry).
type PluginRunner interface {
	ValidateEvent(ctx context.Context, ec *plugin.EventContext) error
	OnEventStored(ctx context.Context, ec *plugin.EventContext) error
	PrepareReq(ctx context.Context, conn plugin.ConnInfo, subID string, filters []nostr.Filter) ([]SubFilterGate, error)
	QueryInitialFilter(ctx context.Context, gate SubFilterGate) ([]*nostr.Event, error)
	EventVisible(ctx context.Context, gate SubFilterGate, ev *nostr.Event) (bool, error)
	EventRequiresAuth(ctx context.Context, ec *plugin.EventContext) bool
	ReqRequiresAuth(ctx context.Context, conn plugin.ConnInfo, filters []nostr.Filter) bool
	SupportedNIPs() []int
}

// SubFilterGate is one REQ filter after transform, with per-filter visibility plugin snapshots.
type SubFilterGate struct {
	Filter nostr.Filter
	Gates  []VisibilityGate
}

// VisibilityGate snapshots one gating plugin for live broadcast re-evaluation.
type VisibilityGate struct {
	PluginID   string
	ReqContext plugin.ReqContext
	Visible    plugin.ReqVisibility
}
