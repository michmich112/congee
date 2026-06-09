package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ApplyBootstrapEnvOverrides mutates c from process environment after JSON load.
// It applies CONGEE_RELAY_PORT, CONGEE_ADMIN_PORT, and CONGEE_DATA_DIR (SQLite events + meta paths),
// then re-validates c.
func ApplyBootstrapEnvOverrides(c *Config) error {
	if c == nil {
		return errors.New("config: nil")
	}
	if p, set, err := parseEnvPort("CONGEE_RELAY_PORT"); err != nil {
		return err
	} else if set {
		c.Relay.Port = p
	}
	if p, set, err := parseEnvPort("CONGEE_ADMIN_PORT"); err != nil {
		return err
	} else if set {
		c.Admin.Port = p
	}
	if dir := strings.TrimSpace(os.Getenv("CONGEE_DATA_DIR")); dir != "" {
		switch strings.TrimSpace(c.Database.Type) {
		case "", "sqlite":
			c.Database.Type = strings.TrimSpace(c.Database.Type)
			if c.Database.Type == "" {
				c.Database.Type = "sqlite"
			}
			clean := filepath.Clean(dir)
			c.Database.DSN = filepath.Join(clean, "congee.db")
			c.Database.MetaDSN = filepath.Join(clean, "congee-meta.db")
		case "postgres":
			// Documented: ignored when using PostgreSQL.
		default:
			// leave as-is; Validate will catch unsupported type
		}
	}
	return c.Validate()
}

// parseEnvPort reads an optional decimal port from the environment.
// If the variable is unset or whitespace-only, ok is false and p is 0.
func parseEnvPort(name string) (p int, ok bool, err error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return 0, false, nil
	}
	p, err = strconv.Atoi(raw)
	if err != nil {
		return 0, false, fmt.Errorf("%s: invalid integer %q", name, raw)
	}
	if err := validatePort(name, p); err != nil {
		return 0, false, err
	}
	return p, true, nil
}
