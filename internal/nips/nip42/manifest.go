package nip42

import (
	"github.com/michmich112/congee/internal/plugin"
)

// Manifest advertises NIP-42 when enabled; AUTH handling remains in relay until P2 migration.
func Manifest() plugin.Manifest {
	return plugin.Manifest{
		ID:             "nip-42",
		NIPNumber:      42,
		Title:          "Authentication of clients to relays",
		Description:    "AUTH challenges and publish/subscribe authentication policy",
		DefaultEnabled: false,
		Priority:       0,
	}
}
