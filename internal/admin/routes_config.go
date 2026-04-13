package admin

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/relayidentity"
	"github.com/michmich112/congee/internal/storage"
)

func handleGetConfig(cfgPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		data, err := os.ReadFile(cfgPath)
		if err != nil {
			http.Error(w, `{"error":"read config failed"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	}
}

func handlePutConfig(cfgPath string, cfgMu *sync.Mutex, st storage.Store, scheduleRestart func(), relayID *relayidentity.Identity) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, `{"error":"read body"}`, http.StatusBadRequest)
			return
		}
		newCfg, err := config.ParseConfigJSON(body)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		// NIP-11 pubkey is always derived from relay identity at runtime (ReconcileNIP11PubKey);
		// do not persist a separate operator-controlled value in the JSON file.
		if relayID != nil {
			newCfg.NIP11.PubKey = ""
		}

		cfgMu.Lock()

		prev, _ := os.ReadFile(cfgPath)
		needRestart := configRestartNeeded(prev, newCfg)

		if err := config.WriteConfigAtomic(cfgPath, newCfg); err != nil {
			cfgMu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		diff := string(body)
		if len(prev) > 0 {
			diff = "previous_bytes=" + strconv.Itoa(len(prev)) + "\n" + string(body)
		}
		if err := config.SaveConfigChange(r.Context(), st, "PUT /api/config", diff); err != nil {
			cfgMu.Unlock()
			http.Error(w, `{"error":"changelog write failed"}`, http.StatusInternalServerError)
			return
		}
		cfgMu.Unlock()

		if needRestart && scheduleRestart != nil {
			go scheduleRestartSoon(scheduleRestart)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":               true,
			"restart_required": needRestart,
			"restarting":       needRestart && scheduleRestart != nil,
		})
	}
}

func configRestartNeeded(prevFile []byte, newCfg *config.Config) bool {
	if len(prevFile) == 0 {
		return true
	}
	prevCfg, err := config.ParseConfigJSON(prevFile)
	if err != nil {
		return true
	}
	return !config.Equal(prevCfg, newCfg)
}

func scheduleRestartSoon(scheduleRestart func()) {
	time.Sleep(150 * time.Millisecond)
	scheduleRestart()
}

func handleConfigChangelog(st storage.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		limit := 50
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
				limit = n
			}
		}
		qctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		rows, err := st.QueryConfigChangelog(qctx, limit)
		if err != nil {
			http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"changelog": rows})
	}
}
