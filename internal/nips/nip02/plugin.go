package nip02

import "github.com/michmich112/congee/internal/plugin"

type nip02 struct{}

func New() plugin.Plugin { return nip02{} }

func (nip02) Manifest() plugin.Manifest {
	return plugin.Manifest{
		ID:             "nip-02",
		NIPNumber:      2,
		Title:          "Follow list",
		Description:    "Kind 3 replaceable follow lists (handled by NIP-01 storage semantics)",
		DefaultEnabled: false,
		Priority:       100,
	}
}

func (nip02) Init(_ plugin.HostAPI) error { return nil }
