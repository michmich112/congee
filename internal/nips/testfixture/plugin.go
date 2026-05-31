package testfixture

import (
	"context"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/plugin"
)

// TestKind is a dedicated kind for registry/fixture tests (not a real NIP).
const TestKind = 990001

// Plugin is a test-only fixture implementing ReqTransformer and ReqVisibility.
// Register via registry.SetExtraPluginFactoriesForTest; not loaded in production.
type Plugin struct {
	host plugin.HostAPI
}

func New() plugin.Plugin { return &Plugin{} }

func (p *Plugin) Manifest() plugin.Manifest {
	return plugin.Manifest{
		ID:             "test-fixture",
		Title:          "Test fixture (transform + gate)",
		Description:    "Test-only plugin for REQ transform and visibility tests",
		DefaultEnabled: true,
		Priority:       9999,
		Capabilities: []plugin.Capability{
			plugin.CapTransformReq,
			plugin.CapGateReqEvents,
		},
		Routes: []plugin.Route{
			{
				Kinds: []int{TestKind},
				Req:   &plugin.DirectionPolicy{},
			},
		},
	}
}

func (p *Plugin) Init(host plugin.HostAPI) error {
	p.host = host
	return nil
}

// TransformReq narrows multi-author filters to the first author and marks the subscription gated.
func (p *Plugin) TransformReq(ctx context.Context, rc *plugin.ReqContext) error {
	_ = ctx
	if len(rc.Filters) == 0 {
		return nil
	}
	f := &rc.Filters[0]
	if len(f.Authors) > 1 {
		f.Authors = []string{f.Authors[0]}
	}
	if rc.Values == nil {
		rc.Values = make(map[string]any)
	}
	rc.Values["gated"] = true
	return nil
}

// EventVisible requires AUTH when the subscription was gated by TransformReq.
func (p *Plugin) EventVisible(ctx context.Context, rc *plugin.ReqContext, ev *nostr.Event) (bool, error) {
	_ = ctx
	_ = ev
	if rc.Values == nil {
		return true, nil
	}
	gated, _ := rc.Values["gated"].(bool)
	if !gated {
		return true, nil
	}
	if rc.Conn == nil {
		return false, nil
	}
	return rc.Conn.HasAuth(), nil
}
