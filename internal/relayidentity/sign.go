package relayidentity

import (
	"errors"

	"github.com/michmich112/congee/internal/nostr"
)

// SignEvent sets pubkey to the relay identity, computes the id, and signs (NIP-01 Schnorr).
func (id *Identity) SignEvent(e *nostr.Event) error {
	if id == nil || id.priv == nil {
		return errors.New("relayidentity: cannot sign with nil identity")
	}
	if e == nil {
		return errors.New("relayidentity: nil event")
	}
	e.PubKey = id.pubHex
	return e.Sign(id.priv)
}
