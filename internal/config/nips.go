package config

// Known optional plugin IDs registered in the NIP plugin map.
var KnownPluginIDs = []string{"nip-02", "nip-29", "nip-50"}

// NIPNumberToPluginID maps optional NIP numbers to plugin ids in the nips config map.
var NIPNumberToPluginID = map[int]string{
	2:  "nip-02",
	29: "nip-29",
	50: "nip-50",
}

// PluginEnabled reports whether an optional plugin is enabled in cfg.NIPs.
func PluginEnabled(cfg *Config, pluginID string) bool {
	if cfg == nil || cfg.NIPs == nil {
		return false
	}
	e, ok := cfg.NIPs[pluginID]
	return ok && e.Enabled
}

// PluginSettings returns raw settings for pluginID, or nil when absent.
func PluginSettings(cfg *Config, pluginID string) []byte {
	if cfg == nil || cfg.NIPs == nil {
		return nil
	}
	e, ok := cfg.NIPs[pluginID]
	if !ok {
		return nil
	}
	return e.Settings
}

// IsKnownPluginID reports whether id is a registered optional plugin key.
func IsKnownPluginID(id string) bool {
	for _, known := range KnownPluginIDs {
		if known == id {
			return true
		}
	}
	return false
}
