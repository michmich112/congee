package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadExampleValidates(t *testing.T) {
	_, err := LoadJSON(filepath.Join("..", "..", "config.example.json"))
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsBadPort(t *testing.T) {
	c := minimalValidConfig()
	c.Relay.Port = 0
	if err := c.Validate(); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateRequiresMandatoryNIPs(t *testing.T) {
	c := minimalValidConfig()
	c.NIPs.Enabled = []int{2}
	if err := c.Validate(); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateUnknownNIP(t *testing.T) {
	c := minimalValidConfig()
	c.NIPs.Enabled = []int{1, 99999}
	if err := c.Validate(); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateRejectsNIP29NegativeLatePublication(t *testing.T) {
	c := minimalValidConfig()
	c.NIPs.Enabled = []int{1, 11, 29}
	c.NIP29.LatePublicationMaxPastSeconds = -1
	if err := c.Validate(); err == nil {
		t.Fatal("expected error")
	}
}

func minimalValidConfig() *Config {
	return &Config{
		Relay:                   RelaySection{Port: 3334},
		Admin:                   AdminSection{Port: 3335},
		Database:                DatabaseSection{Type: "sqlite", DSN: "./x.db"},
		Logging:                 LoggingSection{Level: "info", Format: "json"},
		Audit:                   AuditSection{RetentionDays: 30},
		RateLimits: RateLimitsSection{
			EventsPerMinutePerConnection: 1,
			BytesPerSecondPerConnection:  1,
			ReqsPerMinutePerConnection:   1,
			MessagesPerMinutePerIP:       1,
		},
		ConnectionLimits: ConnectionLimitsSection{
			MaxOpen:                       1,
			MaxSubscriptionsPerConnection: 1,
			MaxFiltersPerReq:              1,
			ConnectionsPerMinutePerIP:     1,
			ReadDeadlineSeconds:           1,
			WriteDeadlineSeconds:          1,
		},
		WebSocket:               WebSocketSection{CompressionEnabled: true, MaxMessageBytes: 1},
		MaxSubscriptionIDLength: 128,
		NIP11:                   NIP11Section{Name: "t"},
		NIPs:                    NIPsSection{Enabled: []int{1, 11}},
		NIP42:                   NIP42Section{},
		NIP29:                   NIP29Section{},
	}
}

func TestLoadMissingFile(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadJSON(filepath.Join(dir, "nope.json"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(p, []byte(`{`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadJSON(p)
	if err == nil {
		t.Fatal("expected error")
	}
}
