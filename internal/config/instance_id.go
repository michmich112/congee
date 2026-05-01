package config

import (
	"errors"
	"os"
	"strings"

	"github.com/google/uuid"
)

// RelayInstanceResolution is the effective PostgreSQL LISTEN/NOTIFY origin for a process,
// derived from CONGEE_INSTANCE_ID (when set) or relay.instance_id in cfg.
type RelayInstanceResolution struct {
	EffectiveID string
	EnvLocked   bool
}

// ResolveRelayInstance reads CONGEE_INSTANCE_ID and cfg and returns the LISTEN/NOTIFY origin.
// It is not cached: each call re-reads the environment. Process startup should call it once
// and pass RelayInstanceResolution to the database layer and admin API so the value stays
// fixed until restart (admin config edits do not change the running notifier without restart).
//
// cfg may be nil (EnvLocked and EffectiveID from env only; config-derived id is empty when unset).
func ResolveRelayInstance(cfg *Config) RelayInstanceResolution {
	if s := strings.TrimSpace(os.Getenv("CONGEE_INSTANCE_ID")); s != "" {
		return RelayInstanceResolution{EffectiveID: s, EnvLocked: true}
	}
	var fromCfg string
	if cfg != nil {
		fromCfg = strings.TrimSpace(cfg.Relay.InstanceID)
	}
	return RelayInstanceResolution{EffectiveID: fromCfg, EnvLocked: false}
}

// RelayInstanceIDLockedByEnv reports whether CONGEE_INSTANCE_ID is set (non-whitespace).
func RelayInstanceIDLockedByEnv() bool {
	return ResolveRelayInstance(nil).EnvLocked
}

// EffectiveRelayInstanceID returns ResolveRelayInstance(cfg).EffectiveID.
func EffectiveRelayInstanceID(cfg *Config) string {
	return ResolveRelayInstance(cfg).EffectiveID
}

// EnsureRelayInstanceIDFile assigns a random relay.instance_id and persists cfg when env is unset and the field is empty.
func EnsureRelayInstanceIDFile(cfg *Config, path string) error {
	if cfg == nil {
		return errors.New("config: nil")
	}
	if ResolveRelayInstance(cfg).EnvLocked {
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
