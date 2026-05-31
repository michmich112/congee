package nip11

import (
	"github.com/michmich112/congee/internal/plugin"
)

// Manifest is the core NIP-11 advertisement entry (no pipeline phases).
func Manifest() plugin.Manifest {
	return plugin.Manifest{
		ID:             "nip-11",
		NIPNumber:      11,
		Title:          "Relay Information Document",
		Description:    "JSON metadata served on GET / with Accept: application/nostr+json",
		DefaultEnabled: true,
		Priority:       0,
	}
}
