package relayidentity

import (
	"encoding/hex"
	"strings"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
)

// Identity holds the relay's long-lived secp256k1 key and derived public forms.
// The private key is never exposed via HTTP APIs.
type Identity struct {
	priv   *btcec.PrivateKey
	pubHex string
	npub   string
}

// PubKeyHex returns the lowercase hex x-only (BIP-340) public key used in Nostr.
func (id *Identity) PubKeyHex() string { return id.pubHex }

// NPub returns the NIP-19 bech32 npub for the relay public key.
func (id *Identity) NPub() string { return id.npub }

func newIdentityFromPriv(priv *btcec.PrivateKey) (*Identity, error) {
	pubBytes := schnorr.SerializePubKey(priv.PubKey())
	pubHex := hex.EncodeToString(pubBytes)
	npub, err := EncodeNpub(pubBytes)
	if err != nil {
		return nil, err
	}
	return &Identity{priv: priv, pubHex: pubHex, npub: npub}, nil
}

func parseSecretKeyHex(s string) (*btcec.PrivateKey, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, errEmptySecretHex
	}
	raw, err := hex.DecodeString(s)
	if err != nil {
		return nil, errInvalidSecretHex
	}
	if len(raw) != btcec.PrivKeyBytesLen {
		return nil, errWrongSecretLen
	}
	priv, _ := btcec.PrivKeyFromBytes(raw)
	return priv, nil
}
