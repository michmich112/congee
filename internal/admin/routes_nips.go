package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"sync"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/nipmeta"
	"github.com/michmich112/congee/internal/nips"
	"github.com/michmich112/congee/internal/storage"
)

type nipRow struct {
	Number        int    `json:"number"`
	Title         string `json:"title"`
	GitHubURL     string `json:"github_url"`
	Mandatory     bool   `json:"mandatory"`
	Implemented   bool   `json:"implemented"`
	Enabled       bool   `json:"enabled"`
}

type patchNIPBody struct {
	NIP     int  `json:"nip"`
	Enabled bool `json:"enabled"`
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
		enabledSet := make(map[int]struct{})
		for _, n := range cfg.NIPs.Enabled {
			enabledSet[n] = struct{}{}
		}
		var nums []int
		for n := range nipmeta.KnownNIPs {
			nums = append(nums, n)
		}
		slices.Sort(nums)
		out := make([]nipRow, 0, len(nums))
		for _, n := range nums {
			m := nipmeta.KnownNIPs[n]
			_, on := enabledSet[n]
			out = append(out, nipRow{
				Number:      m.Number,
				Title:       m.Title,
				GitHubURL:   m.GitHubURL,
				Mandatory:   m.Mandatory,
				Implemented: nips.IsImplemented(n),
				Enabled:     on,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"nips": out})
	}
}

func handleNIPsPatch(cfgPath string, cfgMu *sync.Mutex, st storage.Store, scheduleRestart func()) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, `{"error":"read body"}`, http.StatusBadRequest)
			return
		}
		var req patchNIPBody
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		meta, known := nipmeta.KnownNIPs[req.NIP]
		if !known {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "unknown nip"})
			return
		}
		if meta.Mandatory && !req.Enabled {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "cannot disable mandatory nip"})
			return
		}
		if req.Enabled && !nips.IsImplemented(req.NIP) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "nip not implemented in relay yet"})
			return
		}

		cfgMu.Lock()
		defer cfgMu.Unlock()

		cfg, err := config.LoadJSON(cfgPath)
		if err != nil {
			http.Error(w, `{"error":"load config"}`, http.StatusInternalServerError)
			return
		}
		next := slices.Clone(cfg.NIPs.Enabled)
		if req.Enabled {
			if !slices.Contains(next, req.NIP) {
				next = append(next, req.NIP)
			}
		} else {
			next = slices.DeleteFunc(next, func(n int) bool { return n == req.NIP })
		}
		slices.Sort(next)
		cfg.NIPs.Enabled = slices.Compact(next)

		if err := cfg.Validate(); err != nil {
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
		summary := "PATCH /api/nips"
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
		})
	}
}
