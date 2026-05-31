package registry

import (
	"encoding/json"
	"testing"

	"github.com/michmich112/congee/internal/config"
)

func TestValidatePluginConfigsRejectsNIP29NegativeLatePublication(t *testing.T) {
	settings, _ := json.Marshal(map[string]int{"late_publication_max_past_seconds": -1})
	cfg := &config.Config{
		ConfigVersion: config.ConfigVersionCurrent,
		NIPs: map[string]config.NipPluginEntry{
			"nip-29": {Enabled: true, Settings: settings},
		},
	}
	if err := ValidatePluginConfigs(cfg); err == nil {
		t.Fatal("expected error")
	}
}

func TestCatalogIncludesCoreAndOptionalPlugins(t *testing.T) {
	cfg := config.DefaultConfig()
	catalog := Catalog(cfg)
	if len(catalog) < 6 {
		t.Fatalf("catalog len = %d, want at least 6", len(catalog))
	}
	var sawCore, sawOptional bool
	for _, e := range catalog {
		if e.ID == "nip-01" && e.Core && e.Enabled {
			sawCore = true
		}
		if e.ID == "nip-29" && !e.Core {
			sawOptional = true
		}
	}
	if !sawCore || !sawOptional {
		t.Fatalf("core=%v optional=%v catalog=%v", sawCore, sawOptional, catalog)
	}
}
