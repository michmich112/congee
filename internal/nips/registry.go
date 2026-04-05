package nips

// Meta describes a known NIP for validation and admin UI.
type Meta struct {
	Number    int
	Title     string
	GitHubURL string
	Mandatory bool
}

// KnownNIPs maps NIP numbers to metadata. Phase 3 includes NIP-01 only; more NIPs register here in later phases.
var KnownNIPs = map[int]Meta{
	1: {
		Number:    1,
		Title:     "Basic protocol flow description",
		GitHubURL: "https://github.com/nostr-protocol/nips/blob/master/01.md",
		Mandatory: true,
	},
}

// IsKnown reports whether n is present in the registry.
func IsKnown(n int) bool {
	_, ok := KnownNIPs[n]
	return ok
}
