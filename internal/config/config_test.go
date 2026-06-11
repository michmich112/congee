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

func TestValidateNIP17RequiresNIP42(t *testing.T) {
	c := minimalValidConfig()
	c.NIPs.Enabled = []int{1, 11, 17}
	if err := c.Validate(); err == nil {
		t.Fatal("expected error when NIP 17 is enabled without NIP 42")
	}
	c2 := minimalValidConfig()
	c2.NIPs.Enabled = []int{1, 11, 17, 42}
	c2.NIP42.RelayURL = "wss://relay.example/"
	if err := c2.Validate(); err != nil {
		t.Fatal(err)
	}
}

func minimalValidConfig() *Config {
	return &Config{
		Relay:    RelaySection{Port: 3334},
		Admin:    AdminSection{Port: 3335},
		Database: DatabaseSection{Type: "sqlite", DSN: "./x.db"},
		Logging:  LoggingSection{Level: "info", Format: "json"},
		Audit:    AuditSection{RetentionDays: 30},
		RateLimits: RateLimitsSection{
			EventsPerMinutePerConnection: 1,
			BytesPerSecondPerConnection:  1,
			ReqsPerMinutePerConnection:   1,
			MessagesPerMinutePerIP:       1,
		},
		ConnectionLimits: ConnectionLimitsSection{
			MaxOpen:                       1,
			MaxOpenPerIP:                  1,
			MaxSubscriptionsPerConnection: 1,
			MaxFiltersPerReq:              1,
			ConnectionsPerMinutePerIP:     1,
			IdleNoEventNoSubSeconds:       0,
			ReadDeadlineSeconds:           1,
			WriteDeadlineSeconds:          1,
		},
		WebSocket:               WebSocketSection{CompressionEnabled: true, MaxMessageBytes: 1},
		MaxSubscriptionIDLength: 128,
		NIP11:                   NIP11Section{Name: "t"},
		NIPs:                    NIPsSection{Enabled: []int{1, 11}},
		NIP42:                   NIP42Section{},
		NIP29:                   NIP29Section{},
		NIP17:                   NIP17Section{},
	}
}

func TestLoadMissingFile(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadJSON(filepath.Join(dir, "nope.json"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDefaultConfigValidates(t *testing.T) {
	if err := DefaultConfig().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureConfigFileCreatesDefault(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	if err := EnsureConfigFile(p); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadJSON(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Relay.Port != 3334 {
		t.Fatalf("relay.port: got %d", cfg.Relay.Port)
	}
	if cfg.Database.DSN != "./congee.db" {
		t.Fatalf("database.dsn: got %q", cfg.Database.DSN)
	}
	if err := EnsureConfigFile(p); err != nil {
		t.Fatal(err)
	}
	cfg2, err := LoadJSON(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.Relay.Port != cfg.Relay.Port {
		t.Fatal("second EnsureConfigFile should not change file")
	}
}

func TestEnsureConfigFileCreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "nested", "deep", "config.json")
	if err := EnsureConfigFile(p); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "nested", "deep")); err != nil {
		t.Fatalf("expected parent dirs: %v", err)
	}
	cfg, err := LoadJSON(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Relay.Port != 3334 {
		t.Fatalf("relay.port: got %d", cfg.Relay.Port)
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

func TestValidateAcceptsZeroMaxOpenPerIP(t *testing.T) {
	c := minimalValidConfig()
	c.ConnectionLimits.MaxOpenPerIP = 0
	if err := c.Validate(); err != nil {
		t.Fatalf("expected zero max_open_per_ip to be valid (unlimited), got: %v", err)
	}
}

func TestValidateAcceptsNegativeDefaultQueryLimit(t *testing.T) {
	c := minimalValidConfig()
	neg := -1
	c.ConnectionLimits.DefaultQueryLimit = &neg
	if err := c.Validate(); err != nil {
		t.Fatalf("expected negative default_query_limit to be valid (unlimited), got: %v", err)
	}
}

func TestValidateAcceptsNilDefaultQueryLimit(t *testing.T) {
	c := minimalValidConfig()
	c.ConnectionLimits.DefaultQueryLimit = nil
	if err := c.Validate(); err != nil {
		t.Fatalf("expected nil default_query_limit to be valid, got: %v", err)
	}
}

func TestValidateAcceptsZeroDefaultQueryLimit(t *testing.T) {
	c := minimalValidConfig()
	zero := 0
	c.ConnectionLimits.DefaultQueryLimit = &zero
	if err := c.Validate(); err != nil {
		t.Fatalf("expected zero default_query_limit to be valid, got: %v", err)
	}
}

func TestValidateAcceptsPositiveDefaultQueryLimit(t *testing.T) {
	c := minimalValidConfig()
	v := 500
	c.ConnectionLimits.DefaultQueryLimit = &v
	if err := c.Validate(); err != nil {
		t.Fatalf("expected positive default_query_limit to be valid, got: %v", err)
	}
}

func TestEffectiveREQDefaultQueryLimit(t *testing.T) {
	if g, w := EffectiveREQDefaultQueryLimit(nil), DefaultQueryLimitIfUnset; g != w {
		t.Fatalf("nil: got %d want %d", g, w)
	}
	z := 0
	if g := EffectiveREQDefaultQueryLimit(&z); g != 0 {
		t.Fatalf("zero: got %d want 0", g)
	}
	n := -3
	if g := EffectiveREQDefaultQueryLimit(&n); g != 0 {
		t.Fatalf("negative: got %d want 0", g)
	}
	p := 42
	if g := EffectiveREQDefaultQueryLimit(&p); g != 42 {
		t.Fatalf("positive: got %d want 42", g)
	}
}

func TestValidateAcceptsNegativeQueryPageSize(t *testing.T) {
	c := minimalValidConfig()
	neg := -1
	c.ConnectionLimits.QueryPageSize = &neg
	if err := c.Validate(); err != nil {
		t.Fatalf("expected negative query_page_size to be valid (paging disabled), got: %v", err)
	}
}

func TestValidateAcceptsNilQueryPageSize(t *testing.T) {
	c := minimalValidConfig()
	c.ConnectionLimits.QueryPageSize = nil
	if err := c.Validate(); err != nil {
		t.Fatalf("expected nil query_page_size to be valid, got: %v", err)
	}
}

func TestValidateAcceptsZeroQueryPageSize(t *testing.T) {
	c := minimalValidConfig()
	zero := 0
	c.ConnectionLimits.QueryPageSize = &zero
	if err := c.Validate(); err != nil {
		t.Fatalf("expected zero query_page_size to be valid, got: %v", err)
	}
}

func TestValidateAcceptsPositiveQueryPageSize(t *testing.T) {
	c := minimalValidConfig()
	v := 100
	c.ConnectionLimits.QueryPageSize = &v
	if err := c.Validate(); err != nil {
		t.Fatalf("expected positive query_page_size to be valid, got: %v", err)
	}
}

func TestValidateRejectsPostgresMetaDSN(t *testing.T) {
	c := minimalValidConfig()
	c.Database.MetaDSN = "postgres://localhost/meta"
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for postgres meta_dsn")
	}
}

func TestValidateAcceptsSQLiteMetaDSN(t *testing.T) {
	c := minimalValidConfig()
	c.Database.MetaDSN = "./congee-meta.db"
	if err := c.Validate(); err != nil {
		t.Fatalf("expected sqlite meta_dsn to be valid: %v", err)
	}
}

func TestEffectiveQueryPageSize(t *testing.T) {
	if g, w := EffectiveQueryPageSize(nil), DefaultQueryPageSizeIfUnset; g != w {
		t.Fatalf("nil: got %d want %d", g, w)
	}
	z := 0
	if g := EffectiveQueryPageSize(&z); g != 0 {
		t.Fatalf("zero: got %d want 0", g)
	}
	n := -3
	if g := EffectiveQueryPageSize(&n); g != -3 {
		t.Fatalf("negative: got %d want -3", g)
	}
	p := 42
	if g := EffectiveQueryPageSize(&p); g != 42 {
		t.Fatalf("positive: got %d want 42", g)
	}
}
