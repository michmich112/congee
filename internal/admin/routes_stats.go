package admin

import (
	"encoding/json"
	"net/http"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/relay"
)

func handleStats(cfg *config.Config, relaySrv *relay.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var oc int64
		if relaySrv != nil {
			oc = relaySrv.OpenConnections()
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"open_connections": oc,
			"relay_port":       cfg.Relay.Port,
			"admin_port":       cfg.Admin.Port,
		})
	}
}
