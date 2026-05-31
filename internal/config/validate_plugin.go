package config

// PluginConfigValidator validates optional plugin settings in cfg.NIPs.
type PluginConfigValidator func(c *Config) error

var pluginConfigValidator PluginConfigValidator

// SetPluginConfigValidator registers a validator invoked from Config.Validate().
// The registry sets this at init to validate per-plugin settings via manifests.
func SetPluginConfigValidator(fn PluginConfigValidator) {
	pluginConfigValidator = fn
}
