package nostr

import "slices"

// ReadScope is a server-owned constraint, never decoded from or sent over Nostr.
// Stores apply it before ordering, limits, counts, and reconciliation.
type ReadScope struct {
	ExcludedKinds    []int
	RecipientKinds   []int
	RecipientPubkeys []string
}

func (s *ReadScope) Allows(e *Event) bool {
	if s == nil {
		return true
	}
	if e == nil || slices.Contains(s.ExcludedKinds, e.Kind) {
		return false
	}
	if !slices.Contains(s.RecipientKinds, e.Kind) {
		return true
	}
	var recipient string
	for _, tag := range e.Tags {
		if len(tag) > 0 && tag[0] == "p" {
			if recipient != "" || len(tag) < 2 || !slices.Contains(s.RecipientPubkeys, tag[1]) {
				return false
			}
			recipient = tag[1]
		}
	}
	return recipient != ""
}
