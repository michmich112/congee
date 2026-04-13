package relayidentity

import (
	"fmt"

	"github.com/btcsuite/btcd/btcutil/bech32"
)

const npubHRP = "npub"

// EncodeNpub encodes a 32-byte x-only secp256k1 pubkey as a NIP-19 npub (bech32).
func EncodeNpub(pubKeyXOnly32 []byte) (string, error) {
	if len(pubKeyXOnly32) != 32 {
		return "", fmt.Errorf("relayidentity: npub expects 32-byte pubkey, got %d", len(pubKeyXOnly32))
	}
	return bech32.EncodeFromBase256(npubHRP, pubKeyXOnly32)
}
