package relay

import (
	"slices"
	"strings"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/nostr"
)

func readScopeForConn(cfg *config.Config, c *Conn) *nostr.ReadScope {
	scope := &nostr.ReadScope{RecipientKinds: []int{nip17KindGiftWrap, nip59KindEphemeralGiftWrap}}
	if c != nil && nip17Enabled(cfg) && relayNIP42Enabled(cfg) {
		scope.RecipientPubkeys = c.nip42AuthedPubkeys()
		for i := range scope.RecipientPubkeys {
			scope.RecipientPubkeys[i] = strings.ToLower(scope.RecipientPubkeys[i])
		}
	}
	if relayNIP42Enabled(cfg) && (c == nil || !c.nip42HasAnyAuth()) {
		scope.ExcludedKinds = slices.Clone(cfg.NIP42.RequireAuthSubscribeKinds)
	}
	return scope
}

func queryReadFilters(cfg *config.Config, c *Conn, filters []nostr.Filter) []nostr.Filter {
	result := slices.Clone(filters)
	scope := readScopeForConn(cfg, c)
	for i := range result {
		result[i].ReadScope = scope
	}
	return result
}

// A partially served request can still offer AUTH without closing its public
// subscription. The challenge is sent at most once per connection.
func readMayNeedAuth(cfg *config.Config, filters []nostr.Filter) bool {
	for _, filter := range filters {
		if len(filter.Kinds) == 0 && (nip17Enabled(cfg) || relayNIP42Enabled(cfg) && len(cfg.NIP42.RequireAuthSubscribeKinds) > 0) {
			return true
		}
		for _, kind := range filter.Kinds {
			if nip17Enabled(cfg) && isGiftWrapKind(kind) || relayNIP42Enabled(cfg) && slices.Contains(cfg.NIP42.RequireAuthSubscribeKinds, kind) {
				return true
			}
		}
	}
	return false
}

// readRequiresAuth rejects only explicit requests entirely for auth-gated
// kinds. Broad or mixed requests remain open and return the authorized subset.
func readRequiresAuth(cfg *config.Config, c *Conn, filters []nostr.Filter) bool {
	if c.nip42HasAnyAuth() || len(filters) == 0 {
		return false
	}
	for _, filter := range filters {
		if len(filter.Kinds) == 0 {
			return false
		}
		for _, kind := range filter.Kinds {
			giftWrap := nip17Enabled(cfg) && isGiftWrapKind(kind)
			configured := relayNIP42Enabled(cfg) && slices.Contains(cfg.NIP42.RequireAuthSubscribeKinds, kind)
			if !giftWrap && !configured {
				return false
			}
		}
	}
	return true
}
