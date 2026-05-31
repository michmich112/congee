package nip50

import (
	"context"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/plugin"
)

type nip50 struct {
	host plugin.HostAPI
}

func New() plugin.Plugin { return &nip50{} }

func (p *nip50) Manifest() plugin.Manifest {
	return plugin.Manifest{
		ID:             "nip-50",
		NIPNumber:      50,
		Title:          "Search capability",
		Description:    "Full-text search filters on REQ",
		DefaultEnabled: false,
		Priority:       50,
		Capabilities: []plugin.Capability{
			plugin.CapReadEvents,
			plugin.CapProvideReqQuery,
		},
		Routes: []plugin.Route{
			{
				CatchAll: true,
				Req:      &plugin.DirectionPolicy{},
			},
		},
	}
}

func (p *nip50) Init(host plugin.HostAPI) error {
	p.host = host
	return nil
}

func (p *nip50) QueryReq(ctx context.Context, rc *plugin.ReqContext) ([]*nostr.Event, bool, error) {
	for i := range rc.Filters {
		f := &rc.Filters[i]
		if !f.HasSearch() {
			continue
		}
		evs, err := p.host.SearchEvents(ctx, f.SearchText(), f.WithoutSearch())
		if err != nil {
			return nil, false, err
		}
		return evs, true, nil
	}
	return nil, false, nil
}
