package config

import "strings"

// PromoteLegacySQLite rewrites empty or sqlite database.type to turso in memory.
// DSN and meta_dsn are unchanged. Returns true if the type was changed.
func PromoteLegacySQLite(c *Config) bool {
	if c == nil {
		return false
	}
	t := strings.TrimSpace(c.Database.Type)
	if t != "" && t != "sqlite" {
		return false
	}
	c.Database.Type = "turso"
	return true
}
