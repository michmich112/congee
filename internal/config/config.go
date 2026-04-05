package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"

	"github.com/michmich112/congee/internal/nips"
)

// Load reads and validates JSON config from path (e.g. from CONFIG_PATH).
func Load(path string) (*Config, error) {
	return LoadJSON(path)
}

// LoadJSON reads JSON from path and runs semantic validation.
func LoadJSON(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("config: json: %w", err)
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

// Validate checks ports, retention, NIP lists, and registry membership.
func (c *Config) Validate() error {
	if c == nil {
		return errors.New("config: nil")
	}
	if err := validatePort("relay.port", c.Relay.Port); err != nil {
		return err
	}
	if err := validatePort("admin.port", c.Admin.Port); err != nil {
		return err
	}
	switch c.Database.Type {
	case "", "sqlite":
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
	if c.RateLimits.EventsPerMinutePerConnection <= 0 {
		return errors.New("config: rate_limits.events_per_minute_per_connection must be > 0")
	}
	if c.RateLimits.BytesPerSecondPerConnection <= 0 {
		return errors.New("config: rate_limits.bytes_per_second_per_connection must be > 0")
	}
	if c.ConnectionLimits.MaxOpen <= 0 {
		return errors.New("config: connection_limits.max_open must be > 0")
	}
	if c.ConnectionLimits.MaxSubscriptionsPerConnection <= 0 {
		return errors.New("config: connection_limits.max_subscriptions_per_connection must be > 0")
	}
	if c.ConnectionLimits.ReadDeadlineSeconds <= 0 {
		return errors.New("config: connection_limits.read_deadline_seconds must be > 0")
	}
	if c.ConnectionLimits.WriteDeadlineSeconds <= 0 {
		return errors.New("config: connection_limits.write_deadline_seconds must be > 0")
	}
	if c.MaxSubscriptionIDLength <= 0 {
		return errors.New("config: max_subscription_id_length must be > 0")
	}
	if c.NIP11.Name == "" {
		return errors.New("config: nip11.name is required")
	}
	if len(c.NIPs.Enabled) == 0 {
		return errors.New("config: nips.enabled must be non-empty")
	}
	for _, n := range c.NIPs.Enabled {
		if !nips.IsKnown(n) {
			return fmt.Errorf("config: unknown nip %d in nips.enabled (not in registry)", n)
		}
	}
	if !slices.Contains(c.NIPs.Enabled, 1) {
		return errors.New("config: nips.enabled must include mandatory nip 1")
	}
	for _, n := range c.NIP11.SupportedNIPs {
		if !nips.IsKnown(n) {
			return fmt.Errorf("config: unknown nip %d in nip11.supported_nips", n)
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
