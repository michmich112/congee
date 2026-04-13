package relayidentity

import (
	"errors"
	"fmt"
	"strings"

	"github.com/michmich112/congee/internal/config"
)

// ReconcileNIP11PubKey sets cfg.NIP11.PubKey to the derived hex pubkey.
// If cfg already has a non-empty nip11.pubkey, it must equal the derived key (case-insensitive) or an error is returned.
func ReconcileNIP11PubKey(cfg *config.Config, id *Identity) error {
	if cfg == nil {
		return errors.New("relayidentity: nil config")
	}
	if id == nil {
		return fmt.Errorf("relayidentity: nil identity")
	}
	derived := id.PubKeyHex()
	if p := strings.TrimSpace(cfg.NIP11.PubKey); p != "" && !strings.EqualFold(p, derived) {
		return fmt.Errorf("nip11.pubkey %q does not match relay identity pubkey %q (from relay secrets); fix config or secrets file", p, derived)
	}
	cfg.NIP11.PubKey = derived
	return nil
}
