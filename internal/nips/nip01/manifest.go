package nip01

import (
	"github.com/michmich112/congee/internal/plugin"
)

// Manifest is the core NIP-01 entry (validation handled by relay core pipeline).
func Manifest() plugin.Manifest {
	return plugin.Manifest{
		ID:             "nip-01",
		NIPNumber:      1,
		Title:          "Basic protocol flow",
		Description:    "Core EVENT/REQ/CLOSE handling, signature validation, and broadcast",
		DefaultEnabled: true,
		Priority:       0,
	}
}
