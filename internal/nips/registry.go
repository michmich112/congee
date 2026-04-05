package nips

import "github.com/michmich112/congee/internal/nipmeta"

// KnownNIPs re-exports NIP metadata for callers that import nips.
var KnownNIPs = nipmeta.KnownNIPs

// IsKnown reports whether n is present in the registry.
func IsKnown(n int) bool {
	return nipmeta.IsKnown(n)
}
