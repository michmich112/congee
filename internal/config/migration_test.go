package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const legacyConfigJSON = `{
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
  "nip29": {
    "late_publication_max_past_seconds": 86400,
    "strict_previous_same_h": true
  },
  "nips": { "enabled": [1, 11, 29, 50] }
}`

func TestLegacyConfigMigration(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	if err := os.WriteFile(p, []byte(legacyConfigJSON), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadJSON(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ConfigVersion != ConfigVersionCurrent {
		t.Fatalf("config_version = %d, want %d", cfg.ConfigVersion, ConfigVersionCurrent)
	}
	if !PluginEnabled(cfg, "nip-29") {
		t.Fatal("expected nip-29 enabled")
	}
	if !PluginEnabled(cfg, "nip-50") {
		t.Fatal("expected nip-50 enabled")
	}
	if PluginEnabled(cfg, "nip-02") {
		t.Fatal("nip-02 should remain disabled")
	}

	var settings map[string]any
	if err := json.Unmarshal(cfg.NIPs["nip-29"].Settings, &settings); err != nil {
		t.Fatal(err)
	}
	if settings["late_publication_max_past_seconds"] != float64(86400) {
		t.Fatalf("late_publication_max_past_seconds = %v", settings["late_publication_max_past_seconds"])
	}
	if settings["strict_previous_same_h"] != true {
		t.Fatalf("strict_previous_same_h = %v", settings["strict_previous_same_h"])
	}

	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if needsLegacyMigration(data) {
		t.Fatal("migrated file should not need legacy migration")
	}

	cfg2, err := LoadJSON(p)
	if err != nil {
		t.Fatal(err)
	}
	if !PluginEnabled(cfg2, "nip-29") || !PluginEnabled(cfg2, "nip-50") {
		t.Fatal("reloaded config lost enabled plugins")
	}
	if cfg2.ConfigVersion != ConfigVersionCurrent {
		t.Fatalf("config_version = %d", cfg2.ConfigVersion)
	}
}

func TestMigrationRoundTripViaParseConfigJSON(t *testing.T) {
	migrated, _, err := migrateLegacyJSON([]byte(legacyConfigJSON))
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(migrated, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')

	cfg, err := ParseConfigJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	if !PluginEnabled(cfg, "nip-29") {
		t.Fatal("expected nip-29 enabled after round-trip")
	}
}

func TestNeedsLegacyMigrationDetectsEnabledArray(t *testing.T) {
	if !needsLegacyMigration([]byte(`{"nips":{"enabled":[1,11]}}`)) {
		t.Fatal("expected legacy enabled array to trigger migration")
	}
}

func TestNeedsLegacyMigrationDetectsNIP29Section(t *testing.T) {
	if !needsLegacyMigration([]byte(`{"config_version":1,"nips":{},"nip29":{"strict_previous_same_h":true}}`)) {
		t.Fatal("expected top-level nip29 section to trigger migration")
	}
}
