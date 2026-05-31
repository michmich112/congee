package registry

import (
	"encoding/json"
	"fmt"

	"github.com/michmich112/congee/internal/config"
	nip01manifest "github.com/michmich112/congee/internal/nips/nip01"
	nip11manifest "github.com/michmich112/congee/internal/nips/nip11"
	nip42manifest "github.com/michmich112/congee/internal/nips/nip42"
	"github.com/michmich112/congee/internal/plugin"
)

func init() {
	config.SetPluginConfigValidator(ValidatePluginConfigs)
}

// CatalogEntry is one row returned by GET /api/nips.
type CatalogEntry struct {
	ID             string               `json:"id"`
	NIPNumber      int                  `json:"nip_number"`
	Title          string               `json:"title"`
	Description    string               `json:"description"`
	URL            string               `json:"url,omitempty"`
	Core           bool                 `json:"core"`
	Mandatory      bool                 `json:"mandatory"`
	Enabled        bool                 `json:"enabled"`
	DefaultEnabled bool                 `json:"default_enabled"`
	Capabilities   []plugin.Capability  `json:"capabilities"`
	ConfigSchema   plugin.ConfigSchema  `json:"config_schema"`
	Settings       json.RawMessage      `json:"settings,omitempty"`
}

// ValidatePluginConfigs checks optional plugin settings via each plugin's ConfigValidator.
func ValidatePluginConfigs(cfg *config.Config) error {
	if cfg == nil {
		return nil
	}
	validators := pluginValidators()
	for id, entry := range cfg.NIPs {
		v, ok := validators[id]
		if !ok {
			continue
		}
		if err := v.ValidateConfig(entry.Settings); err != nil {
			return err
		}
	}
	return nil
}

func pluginValidators() map[string]plugin.ConfigValidator {
	out := make(map[string]plugin.ConfigValidator)
	for _, factory := range RegisterBuiltinPlugins() {
		p := factory
		if v, ok := p.(plugin.ConfigValidator); ok {
			out[p.Manifest().ID] = v
		}
	}
	return out
}

// Catalog builds the admin NIP/plugin catalog from cfg and registered manifests.
func Catalog(cfg *config.Config) []CatalogEntry {
	var out []CatalogEntry

	out = append(out, coreCatalogEntry(nip01manifest.Manifest(), true, true, nil))
	out = append(out, coreCatalogEntry(nip11manifest.Manifest(), true, true, nip11Settings(cfg)))
	out = append(out, coreCatalogEntry(nip42manifest.Manifest(), cfg != nil && cfg.NIP42.Enabled, false, nip42Settings(cfg)))

	for _, factory := range RegisterBuiltinPlugins() {
		m := factory.Manifest()
		entry := CatalogEntry{
			ID:             m.ID,
			NIPNumber:      m.NIPNumber,
			Title:          m.Title,
			Description:    m.Description,
			URL:            m.URL,
			Core:           false,
			Mandatory:      false,
			DefaultEnabled: m.DefaultEnabled,
			Capabilities:   append([]plugin.Capability(nil), m.Capabilities...),
			ConfigSchema:   m.ConfigSchema,
		}
		if cfg != nil && cfg.NIPs != nil {
			if pe, ok := cfg.NIPs[m.ID]; ok {
				entry.Enabled = pe.Enabled
				entry.Settings = pe.Settings
			}
		}
		out = append(out, entry)
	}
	return out
}

func coreCatalogEntry(m plugin.Manifest, enabled, mandatory bool, settings json.RawMessage) CatalogEntry {
	return CatalogEntry{
		ID:             m.ID,
		NIPNumber:      m.NIPNumber,
		Title:          m.Title,
		Description:    m.Description,
		URL:            m.URL,
		Core:           true,
		Mandatory:      mandatory,
		Enabled:        enabled,
		DefaultEnabled: m.DefaultEnabled,
		Capabilities:   append([]plugin.Capability(nil), m.Capabilities...),
		ConfigSchema:   m.ConfigSchema,
		Settings:       settings,
	}
}

func nip11Settings(cfg *config.Config) json.RawMessage {
	if cfg == nil {
		return nil
	}
	b, err := json.Marshal(cfg.NIP11)
	if err != nil {
		return nil
	}
	return b
}

func nip42Settings(cfg *config.Config) json.RawMessage {
	if cfg == nil {
		return nil
	}
	type nip42Settings struct {
		Enabled                   bool     `json:"enabled"`
		RelayURL                  string   `json:"relay_url"`
		SendChallengeOnConnect    bool     `json:"send_challenge_on_connect"`
		CreatedAtSkewSeconds      int      `json:"created_at_skew_seconds"`
		RequireAuthSubscribeKinds []int    `json:"require_auth_subscribe_kinds"`
		RequireAuthPublishKinds   []int    `json:"require_auth_publish_kinds"`
		AllowlistedPubkeys        []string `json:"allowlisted_pubkeys"`
	}
	b, err := json.Marshal(nip42Settings{
		Enabled:                   cfg.NIP42.Enabled,
		RelayURL:                  cfg.NIP42.RelayURL,
		SendChallengeOnConnect:    cfg.NIP42.SendChallengeOnConnect,
		CreatedAtSkewSeconds:      cfg.NIP42.CreatedAtSkewSeconds,
		RequireAuthSubscribeKinds: cfg.NIP42.RequireAuthSubscribeKinds,
		RequireAuthPublishKinds:   cfg.NIP42.RequireAuthPublishKinds,
		AllowlistedPubkeys:        cfg.NIP42.AllowlistedPubkeys,
	})
	if err != nil {
		return nil
	}
	return b
}

// ApplyPluginPatch merges a plugin enable/settings update into cfg.
func ApplyPluginPatch(cfg *config.Config, pluginID string, enabled *bool, settings json.RawMessage) error {
	if cfg == nil {
		return fmt.Errorf("config: nil")
	}
	if !config.IsKnownPluginID(pluginID) {
		return fmt.Errorf("config: unknown plugin id %q", pluginID)
	}
	if cfg.NIPs == nil {
		cfg.NIPs = make(map[string]config.NipPluginEntry)
	}
	entry := cfg.NIPs[pluginID]
	if enabled != nil {
		entry.Enabled = *enabled
	}
	if settings != nil {
		entry.Settings = settings
	}
	cfg.NIPs[pluginID] = entry
	return cfg.Validate()
}
