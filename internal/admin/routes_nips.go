package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"sync"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/nips/registry"
	"github.com/michmich112/congee/internal/storage"
)

type patchPluginBody struct {
	Enabled  *bool           `json:"enabled"`
	Settings json.RawMessage `json:"settings"`
}

func handleNIPsGet(cfgPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		cfg, err := config.LoadJSON(cfgPath)
		if err != nil {
			http.Error(w, `{"error":"load config"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"config_version": cfg.ConfigVersion,
			"plugins":        registry.Catalog(cfg),
		})
	}
}

func handleNIPPluginPatch(cfgPath string, cfgMu *sync.Mutex, st storage.Store, scheduleRestart func()) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		pluginID := r.PathValue("id")
		if pluginID == "" {
			http.Error(w, `{"error":"missing plugin id"}`, http.StatusBadRequest)
			return
		}
		if !config.IsKnownPluginID(pluginID) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "unknown plugin id"})
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, `{"error":"read body"}`, http.StatusBadRequest)
			return
		}
		var req patchPluginBody
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if req.Enabled == nil && req.Settings == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "enabled or settings required"})
			return
		}

		cfgMu.Lock()
		defer cfgMu.Unlock()

		cfg, err := config.LoadJSON(cfgPath)
		if err != nil {
			http.Error(w, `{"error":"load config"}`, http.StatusInternalServerError)
			return
		}
		if err := registry.ApplyPluginPatch(cfg, pluginID, req.Enabled, req.Settings); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		if err := config.WriteConfigAtomic(cfgPath, cfg); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		summary := "PATCH /api/nips/" + pluginID
		if err := config.SaveConfigChange(r.Context(), st, summary, string(body)); err != nil {
			http.Error(w, `{"error":"changelog write failed"}`, http.StatusInternalServerError)
			return
		}
		if scheduleRestart != nil {
			go scheduleRestartSoon(scheduleRestart)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":               true,
			"restart_required": true,
			"restarting":       scheduleRestart != nil,
			"plugin":           registry.Catalog(cfg),
		})
	}
}
