package config

import (
	"path/filepath"
	"testing"
)

func TestLoadJSONBackfillsUnsetConnectionLimits(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	// Omit max_open_per_ip and idle_no_event_no_sub_seconds (pre-PR shape).
	body := `{
  "relay": { "port": 3334 },
  "admin": { "port": 3335 },
  "database": { "type": "sqlite", "dsn": "./congee.db" },
  "logging": { "level": "info", "format": "json" },
  "audit": { "retention_days": 30 },
  "rate_limits": {
    "events_per_minute_per_connection": 120,
    "bytes_per_second_per_connection": 1048576,
    "reqs_per_minute_per_connection": 60,
    "messages_per_minute_per_ip": 6000
  },
  "connection_limits": {
    "max_open": 100,
    "max_subscriptions_per_connection": 20,
    "max_filters_per_req": 10,
    "connections_per_minute_per_ip": 60,
    "read_deadline_seconds": 120,
    "write_deadline_seconds": 30
  },
  "websocket": { "compression_enabled": true, "max_message_bytes": 1048576 },
  "max_subscription_id_length": 128,
  "nip11": { "name": "t", "description": "", "pubkey": "", "contact": "", "software": "" },
  "nips": { "enabled": [1, 11] }
}`
	if err := writeTestConfig(p, body); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadJSON(p)
	if err != nil {
		t.Fatal(err)
	}
	def := DefaultConfig().ConnectionLimits
	if cfg.ConnectionLimits.MaxOpenPerIP != def.MaxOpenPerIP {
		t.Fatalf("max_open_per_ip = %d, want default %d", cfg.ConnectionLimits.MaxOpenPerIP, def.MaxOpenPerIP)
	}
	if cfg.ConnectionLimits.IdleNoEventNoSubSeconds != def.IdleNoEventNoSubSeconds {
		t.Fatalf("idle_no_event_no_sub_seconds = %d, want default %d",
			cfg.ConnectionLimits.IdleNoEventNoSubSeconds, def.IdleNoEventNoSubSeconds)
	}
}

func TestParseConfigJSONPreservesExplicitZeroConnectionLimits(t *testing.T) {
	body := []byte(`{
  "relay": { "port": 3334 },
  "admin": { "port": 3335 },
  "database": { "type": "sqlite", "dsn": "./x.db" },
  "logging": { "level": "info", "format": "json" },
  "audit": { "retention_days": 30 },
  "rate_limits": {
    "events_per_minute_per_connection": 1,
    "bytes_per_second_per_connection": 1,
    "reqs_per_minute_per_connection": 1,
    "messages_per_minute_per_ip": 1
  },
  "connection_limits": {
    "max_open": 1,
    "max_open_per_ip": 0,
    "max_subscriptions_per_connection": 1,
    "max_filters_per_req": 1,
    "connections_per_minute_per_ip": 1,
    "idle_no_event_no_sub_seconds": 0,
    "read_deadline_seconds": 1,
    "write_deadline_seconds": 1
  },
  "websocket": { "compression_enabled": true, "max_message_bytes": 1 },
  "max_subscription_id_length": 128,
  "nip11": { "name": "t" },
  "nips": { "enabled": [1, 11] }
}`)
	cfg, err := ParseConfigJSON(body)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ConnectionLimits.MaxOpenPerIP != 0 {
		t.Fatalf("max_open_per_ip = %d, want explicit 0", cfg.ConnectionLimits.MaxOpenPerIP)
	}
	if cfg.ConnectionLimits.IdleNoEventNoSubSeconds != 0 {
		t.Fatalf("idle_no_event_no_sub_seconds = %d, want explicit 0", cfg.ConnectionLimits.IdleNoEventNoSubSeconds)
	}
}

func writeTestConfig(path, body string) error {
	return WriteFileAtomic(path, []byte(body))
}
