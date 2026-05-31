package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Load reads and validates JSON config from path (e.g. from CONFIG_PATH).
func Load(path string) (*Config, error) {
	return LoadJSON(path)
}

// DefaultConfig returns the stock configuration matching config.example.json.
func DefaultConfig() *Config {
	return &Config{
		ConfigVersion: ConfigVersionCurrent,
		Relay:         RelaySection{Port: 3334},
		Admin:         AdminSection{Port: 3335},
		Database:      DatabaseSection{Type: "sqlite", DSN: "./congee.db"},
		Logging:       LoggingSection{Level: "info", Format: "json"},
		Audit:         AuditSection{RetentionDays: 30},
		Metrics:       MetricsSection{RelayBucketRetentionDays: 30},
		RateLimits: RateLimitsSection{
			EventsPerMinutePerConnection: 120,
			BytesPerSecondPerConnection:  1048576,
			ReqsPerMinutePerConnection:   60,
			MessagesPerMinutePerIP:       6000,
		},
		ConnectionLimits: ConnectionLimitsSection{
			MaxOpen:                       10000,
			MaxSubscriptionsPerConnection: 20,
			MaxFiltersPerReq:              10,
			ConnectionsPerMinutePerIP:     60,
			ReadDeadlineSeconds:           120,
			WriteDeadlineSeconds:          30,
			DefaultQueryLimit:             ptrInt(DefaultQueryLimitIfUnset),
		},
		WebSocket: WebSocketSection{
			CompressionEnabled: true,
			MaxMessageBytes:    1048576,
		},
		MaxSubscriptionIDLength: 128,
		NIP11: NIP11Section{
			Name:               "Congee",
			Description:        "Nostr relay (example metadata)",
			PubKey:             "",
			Contact:            "",
			Software:           "https://github.com/michmich112/congee",
			CORSAllowAnyOrigin: false,
		},
		NIPs: make(map[string]NipPluginEntry),
	}
}

// EnsureConfigFile writes DefaultConfig to path when no file exists there yet.
// Existing files are never modified.
func EnsureConfigFile(path string) error {
	_, err := os.Stat(path)
	if err == nil {
		return nil
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	c := DefaultConfig()
	if err := c.Validate(); err != nil {
		return fmt.Errorf("default config invalid: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	if err := WriteConfigAtomic(path, c); err != nil {
		return fmt.Errorf("write default %s: %w", path, err)
	}
	return nil
}

// LoadJSON reads JSON from path, migrates legacy configs when needed, and runs semantic validation.
func LoadJSON(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	if needsLegacyMigration(data) {
		migrated, mig, err := migrateLegacyJSON(data)
		if err != nil {
			return nil, err
		}
		if err := WriteConfigAtomic(path, migrated); err != nil {
			return nil, fmt.Errorf("config: migrate write %s: %w", path, err)
		}
		PendingMigration = mig
		return migrated, nil
	}
	c, err := ParseConfigBytes(data)
	if err != nil {
		return nil, err
	}
	if needsConfigVersionBump(data) {
		if err := WriteConfigAtomic(path, c); err != nil {
			return nil, fmt.Errorf("config: version bump write %s: %w", path, err)
		}
	}
	return c, nil
}

// ParseConfigBytes unmarshals JSON bytes and validates.
func ParseConfigBytes(data []byte) (*Config, error) {
	if needsLegacyMigration(data) {
		migrated, _, err := migrateLegacyJSON(data)
		return migrated, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("config: json: %w", err)
	}
	normalizeConfigVersion(&c)
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

// Validate checks ports, retention, plugin map keys, and core NIP sections.
func (c *Config) Validate() error {
	if c == nil {
		return errors.New("config: nil")
	}
	if err := validatePort("relay.port", c.Relay.Port); err != nil {
		return err
	}
	if err := validateRelayInstanceIDField(c.Relay.InstanceID); err != nil {
		return err
	}
	if err := validatePort("admin.port", c.Admin.Port); err != nil {
		return err
	}
	switch c.Database.Type {
	case "", "sqlite", "postgres":
		if c.Database.DSN == "" {
			return errors.New("config: database.dsn is required")
		}
	default:
		return fmt.Errorf("config: unsupported database.type %q", c.Database.Type)
	}
	switch c.Logging.Level {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("config: unsupported logging.level %q", c.Logging.Level)
	}
	switch c.Logging.Format {
	case "json", "console":
	default:
		return fmt.Errorf("config: unsupported logging.format %q", c.Logging.Format)
	}
	if c.Audit.RetentionDays <= 0 {
		return errors.New("config: audit.retention_days must be > 0")
	}
	if c.Metrics.RelayBucketRetentionDays <= 0 {
		c.Metrics.RelayBucketRetentionDays = 30
	}
	if c.Metrics.RelayBucketRetentionDays > 3650 {
		return errors.New("config: metrics.relay_bucket_retention_days must be <= 3650")
	}
	if c.RateLimits.EventsPerMinutePerConnection <= 0 {
		return errors.New("config: rate_limits.events_per_minute_per_connection must be > 0")
	}
	if c.RateLimits.BytesPerSecondPerConnection <= 0 {
		return errors.New("config: rate_limits.bytes_per_second_per_connection must be > 0")
	}
	if c.RateLimits.ReqsPerMinutePerConnection <= 0 {
		return errors.New("config: rate_limits.reqs_per_minute_per_connection must be > 0")
	}
	if c.RateLimits.MessagesPerMinutePerIP <= 0 {
		return errors.New("config: rate_limits.messages_per_minute_per_ip must be > 0")
	}
	if c.ConnectionLimits.MaxOpen <= 0 {
		return errors.New("config: connection_limits.max_open must be > 0")
	}
	if c.ConnectionLimits.MaxSubscriptionsPerConnection <= 0 {
		return errors.New("config: connection_limits.max_subscriptions_per_connection must be > 0")
	}
	if c.ConnectionLimits.MaxFiltersPerReq <= 0 {
		return errors.New("config: connection_limits.max_filters_per_req must be > 0")
	}
	if c.ConnectionLimits.ConnectionsPerMinutePerIP <= 0 {
		return errors.New("config: connection_limits.connections_per_minute_per_ip must be > 0")
	}
	if c.ConnectionLimits.ReadDeadlineSeconds <= 0 {
		return errors.New("config: connection_limits.read_deadline_seconds must be > 0")
	}
	if c.ConnectionLimits.WriteDeadlineSeconds <= 0 {
		return errors.New("config: connection_limits.write_deadline_seconds must be > 0")
	}
	if c.WebSocket.MaxMessageBytes <= 0 {
		return errors.New("config: websocket.max_message_bytes must be > 0")
	}
	if c.MaxSubscriptionIDLength <= 0 {
		return errors.New("config: max_subscription_id_length must be > 0")
	}
	if c.NIP11.Name == "" {
		return errors.New("config: nip11.name is required")
	}
	if c.NIPs == nil {
		c.NIPs = make(map[string]NipPluginEntry)
	}
	for id := range c.NIPs {
		if !IsKnownPluginID(id) {
			return fmt.Errorf("config: unknown plugin %q in nips (expected one of %v)", id, KnownPluginIDs)
		}
	}
	if c.NIP42.Enabled {
		if strings.TrimSpace(c.NIP42.RelayURL) == "" {
			return errors.New("config: nip42.relay_url is required when NIP 42 is enabled")
		}
		if _, err := NormalizeNIP42RelayURL(c.NIP42.RelayURL); err != nil {
			return fmt.Errorf("config: nip42.relay_url: %w", err)
		}
	}
	if c.NIP42.CreatedAtSkewSeconds < 0 {
		return errors.New("config: nip42.created_at_skew_seconds must be >= 0")
	}
	if pluginConfigValidator != nil {
		if err := pluginConfigValidator(c); err != nil {
			return err
		}
	}
	return nil
}

func validatePort(field string, p int) error {
	if p < 1 || p > 65535 {
		return fmt.Errorf("config: %s must be between 1 and 65535", field)
	}
	return nil
}

func validateRelayInstanceIDField(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if len(s) > 256 {
		return errors.New("config: relay.instance_id must be at most 256 characters")
	}
	if strings.ContainsAny(s, "\r\n\x00") {
		return errors.New("config: relay.instance_id must not contain newline or null characters")
	}
	return nil
}

func ptrInt(v int) *int {
	return &v
}
