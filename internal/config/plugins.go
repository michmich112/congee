package config

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

func validatePlugins(c *Config) error {
	if c.Plugins.InterceptTimeoutMs < 0 {
		return fmt.Errorf("config: plugins.intercept_timeout_ms must be >= 0")
	}
	if c.Plugins.InterceptLogSize != nil {
		n := *c.Plugins.InterceptLogSize
		if n < 0 || n > MaxPluginInterceptLogSize {
			return fmt.Errorf("config: plugins.intercept_log_size must be between 0 and %d", MaxPluginInterceptLogSize)
		}
	}
	seen := make(map[string]struct{})
	for i, it := range c.Plugins.Items {
		id := strings.TrimSpace(it.ID)
		if id == "" {
			return fmt.Errorf("config: plugins.items[%d].id is required", i)
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("config: duplicate plugins.items id %q", id)
		}
		seen[id] = struct{}{}
		if it.SHA256 != "" {
			b, err := hex.DecodeString(strings.TrimSpace(it.SHA256))
			if err != nil || len(b) != 32 {
				return fmt.Errorf("config: plugins.items[%d].sha256 must be 64 hex chars", i)
			}
		}
	}
	return nil
}

// EffectivePluginInterceptLogSize is the in-memory intercept log window (0 disables).
func EffectivePluginInterceptLogSize(c *Config) int {
	if c != nil && c.Plugins.InterceptLogSize != nil {
		n := *c.Plugins.InterceptLogSize
		if n < 0 {
			return 0
		}
		if n > MaxPluginInterceptLogSize {
			return MaxPluginInterceptLogSize
		}
		return n
	}
	return DefaultPluginInterceptLogSize
}

// EffectivePluginInterceptTimeout is the host intercept deadline ceiling.
func EffectivePluginInterceptTimeout(c *Config) time.Duration {
	ms := DefaultPluginInterceptTimeoutMs
	if c != nil && c.Plugins.InterceptTimeoutMs > 0 {
		ms = c.Plugins.InterceptTimeoutMs
	}
	return time.Duration(ms) * time.Millisecond
}

// PluginItemByID returns a copy of the named plugin item and its index, or -1 if missing.
func PluginItemByID(c *Config, id string) (PluginItem, int) {
	if c == nil {
		return PluginItem{}, -1
	}
	for i, it := range c.Plugins.Items {
		if it.ID == id {
			return it, i
		}
	}
	return PluginItem{}, -1
}
