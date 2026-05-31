package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/storage/sqlite"
	"github.com/rs/zerolog"
)

func TestNIPsGetReturnsCatalog(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := config.WriteConfigAtomic(cfgPath, config.DefaultConfig()); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/nips", nil)
	rr := httptest.NewRecorder()
	handleNIPsGet(cfgPath).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var out struct {
		ConfigVersion int `json:"config_version"`
		Plugins       []struct {
			ID      string `json:"id"`
			Enabled bool   `json:"enabled"`
			Core    bool   `json:"core"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.ConfigVersion != config.ConfigVersionCurrent {
		t.Fatalf("config_version = %d", out.ConfigVersion)
	}
	if len(out.Plugins) < 4 {
		t.Fatalf("expected core + optional plugins, got %d", len(out.Plugins))
	}
}

func TestNIPPluginPatchUpdatesSettings(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	cfg := config.DefaultConfig()
	cfg.NIPs["nip-29"] = config.NipPluginEntry{Enabled: true}
	if err := config.WriteConfigAtomic(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}

	st, err := sqlite.Open(ctx, filepath.Join(dir, "admin.db"), nil, zerolog.Nop())
	if err != nil && err.Error() == "sqlite driver not available (CGO)" {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	var cfgMu sync.Mutex
	body, _ := json.Marshal(map[string]any{
		"settings": map[string]any{"late_publication_max_past_seconds": 3600},
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/nips/nip-29", bytes.NewReader(body))
	req.SetPathValue("id", "nip-29")
	rr := httptest.NewRecorder()
	handleNIPPluginPatch(cfgPath, &cfgMu, st, nil).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}

	updated, err := config.LoadJSON(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(updated.NIPs["nip-29"].Settings, &settings); err != nil {
		t.Fatal(err)
	}
	if settings["late_publication_max_past_seconds"] != float64(3600) {
		t.Fatalf("settings = %v", settings)
	}
}

func TestConfigMigrationRoundTripViaAdminPut(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	legacy := []byte(`{
  "relay": { "port": 3334 },
  "admin": { "port": 3335 },
  "database": { "type": "sqlite", "dsn": "./congee.db" },
  "logging": { "level": "info", "format": "json" },
  "audit": { "retention_days": 30 },
  "metrics": { "relay_bucket_retention_days": 30 },
  "rate_limits": {
    "events_per_minute_per_connection": 120,
    "bytes_per_second_per_connection": 1048576,
    "reqs_per_minute_per_connection": 60,
    "messages_per_minute_per_ip": 6000
  },
  "connection_limits": {
    "max_open": 10000,
    "max_subscriptions_per_connection": 20,
    "max_filters_per_req": 10,
    "connections_per_minute_per_ip": 60,
    "read_deadline_seconds": 120,
    "write_deadline_seconds": 30
  },
  "websocket": { "compression_enabled": true, "max_message_bytes": 1048576 },
  "max_subscription_id_length": 128,
  "nip11": { "name": "Congee", "software": "https://github.com/michmich112/congee" },
  "nip42": { "relay_url": "", "send_challenge_on_connect": false, "created_at_skew_seconds": 600 },
  "nip29": { "late_publication_max_past_seconds": 86400 },
  "nips": { "enabled": [1, 11, 29] }
}`)
	if err := os.WriteFile(cfgPath, legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.LoadJSON(cfgPath); err != nil {
		t.Fatal(err)
	}

	st, err := sqlite.Open(ctx, filepath.Join(dir, "admin.db"), nil, zerolog.Nop())
	if err != nil && err.Error() == "sqlite driver not available (CGO)" {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	var cfgMu sync.Mutex
	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(data))
	rr := httptest.NewRecorder()
	handlePutConfig(cfgPath, &cfgMu, st, nil, nil).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT status = %d body=%s", rr.Code, rr.Body.String())
	}

	reloaded, err := config.LoadJSON(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !config.PluginEnabled(reloaded, "nip-29") {
		t.Fatal("nip-29 should stay enabled")
	}
	var settings map[string]any
	if err := json.Unmarshal(reloaded.NIPs["nip-29"].Settings, &settings); err != nil {
		t.Fatal(err)
	}
	if settings["late_publication_max_past_seconds"] != float64(86400) {
		t.Fatalf("settings = %v", settings)
	}
}
