package config

import (
	"errors"
	"os"
	"strings"

	"github.com/google/uuid"
)

// RelayInstanceIDLockedByEnv reports whether CONGEE_INSTANCE_ID is set (non-whitespace).
func RelayInstanceIDLockedByEnv() bool {
	return strings.TrimSpace(os.Getenv("CONGEE_INSTANCE_ID")) != ""
}

// EffectiveRelayInstanceID returns the LISTEN/NOTIFY origin id: env when set, else relay.instance_id from cfg.
func EffectiveRelayInstanceID(cfg *Config) string {
	if cfg == nil {
		return ""
	}
	if s := strings.TrimSpace(os.Getenv("CONGEE_INSTANCE_ID")); s != "" {
		return s
	}
	return strings.TrimSpace(cfg.Relay.InstanceID)
}

// EnsureRelayInstanceIDFile assigns a random relay.instance_id and persists cfg when env is unset and the field is empty.
func EnsureRelayInstanceIDFile(cfg *Config, path string) error {
	if cfg == nil {
		return errors.New("config: nil")
	}
	if RelayInstanceIDLockedByEnv() {
		return nil
	}
	if strings.TrimSpace(cfg.Relay.InstanceID) != "" {
		return nil
	}
	cfg.Relay.InstanceID = uuid.New().String()
	if err := cfg.Validate(); err != nil {
		return err
	}
	return WriteConfigAtomic(path, cfg)
}
