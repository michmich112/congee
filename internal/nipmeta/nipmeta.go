package nipmeta

// Meta describes a known NIP for validation and admin UI.
type Meta struct {
	Number    int
	Title     string
	GitHubURL string
	Mandatory bool
}

// KnownNIPs maps NIP numbers to metadata.
var KnownNIPs = map[int]Meta{
	1: {
		Number:    1,
		Title:     "Basic protocol flow description",
		GitHubURL: "https://github.com/nostr-protocol/nips/blob/master/01.md",
		Mandatory: true,
	},
	2: {
		Number:    2,
		Title:     "Follow list",
		GitHubURL: "https://github.com/nostr-protocol/nips/blob/master/02.md",
		Mandatory: false,
	},
	42: {
		Number:    42,
		Title:     "Authentication of clients to relays",
		GitHubURL: "https://github.com/nostr-protocol/nips/blob/master/42.md",
		Mandatory: false,
	},
	11: {
		Number:    11,
		Title:     "Relay Information Document",
		GitHubURL: "https://github.com/nostr-protocol/nips/blob/master/11.md",
		Mandatory: true,
	},
	50: {
		Number:    50,
		Title:     "Search Capability",
		GitHubURL: "https://github.com/nostr-protocol/nips/blob/master/50.md",
		Mandatory: false,
	},
}

// IsKnown reports whether n is present in the registry.
func IsKnown(n int) bool {
	_, ok := KnownNIPs[n]
	return ok
}
