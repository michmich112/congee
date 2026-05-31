package nostr

import (
	"strconv"
	"strings"
)

// KindDeletion is NIP-09 event deletion request kind.
const KindDeletion = 5

// ParseAddressableCoordinate parses an NIP-33 addressable coordinate from a kind-5 "a" tag value.
// Format: "<kind>:<pubkey>:<d-tag>".
func ParseAddressableCoordinate(s string) (kind int, pubkey, dTag string, ok bool) {
	parts := strings.SplitN(s, ":", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return 0, "", "", false
	}
	k, err := strconv.Atoi(parts[0])
	if err != nil || k < 0 {
		return 0, "", "", false
	}
	return k, parts[1], parts[2], true
}
