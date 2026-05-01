package admin

import (
	"encoding/json"
	"net/http"

	"github.com/michmich112/congee/internal/relayidentity"
)

func handleRelayIdentity(id *relayidentity.Identity, relayRuntimeInstanceID string, relayInstanceIDEnvLocked bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if id == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "relay identity not available"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"pubkey_hex":                   id.PubKeyHex(),
			"npub":                         id.NPub(),
			"relay_instance_id":            relayRuntimeInstanceID,
			"relay_instance_id_env_locked": relayInstanceIDEnvLocked,
		})
	})
}
