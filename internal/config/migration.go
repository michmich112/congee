package config

import (
	"encoding/json"
	"fmt"
)

// MigrationResult describes a config file rewritten during load-time migration.
type MigrationResult struct {
	Summary string
	Diff    string
}

// legacyNIP29Section is the pre-migration top-level nip29 block folded into nips["nip-29"].settings.
type legacyNIP29Section struct {
	LatePublicationMaxPastSeconds int  `json:"late_publication_max_past_seconds"`
	StrictPreviousSameH           bool `json:"strict_previous_same_h"`
}

type legacyNIPsSection struct {
	Enabled []int `json:"enabled"`
}

func needsLegacyMigration(data []byte) bool {
	var probe struct {
		NIPs  json.RawMessage `json:"nips"`
		NIP29 json.RawMessage `json:"nip29"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return false
	}
	if len(probe.NIP29) > 0 && string(probe.NIP29) != "null" {
		return true
	}
	if len(probe.NIPs) == 0 {
		return false
	}
	var legacy legacyNIPsSection
	if json.Unmarshal(probe.NIPs, &legacy) == nil && legacy.Enabled != nil {
		return true
	}
	return false
}

func needsConfigVersionBump(data []byte) bool {
	var probe struct {
		ConfigVersion int `json:"config_version"`
	}
	_ = json.Unmarshal(data, &probe)
	return probe.ConfigVersion < ConfigVersionCurrent
}

func migrateLegacyJSON(data []byte) (*Config, *MigrationResult, error) {
	envelope := make(map[string]json.RawMessage)
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, nil, fmt.Errorf("config: json: %w", err)
	}

	var legacyNIPs legacyNIPsSection
	if raw, ok := envelope["nips"]; ok {
		_ = json.Unmarshal(raw, &legacyNIPs)
	}
	var nip29 legacyNIP29Section
	if raw, ok := envelope["nip29"]; ok {
		_ = json.Unmarshal(raw, &nip29)
	}
	delete(envelope, "nips")
	delete(envelope, "nip29")

	stripped, err := json.Marshal(envelope)
	if err != nil {
		return nil, nil, err
	}
	var cfg Config
	if err := json.Unmarshal(stripped, &cfg); err != nil {
		return nil, nil, fmt.Errorf("config: json: %w", err)
	}

	cfg.ConfigVersion = ConfigVersionCurrent
	if cfg.NIPs == nil {
		cfg.NIPs = make(map[string]NipPluginEntry)
	}

	for _, n := range legacyNIPs.Enabled {
		switch n {
		case 42:
			cfg.NIP42.Enabled = true
		default:
			if id, ok := NIPNumberToPluginID[n]; ok {
				entry := cfg.NIPs[id]
				entry.Enabled = true
				cfg.NIPs[id] = entry
			}
		}
	}

	nip29Settings, err := json.Marshal(nip29)
	if err != nil {
		return nil, nil, fmt.Errorf("config: migrate nip29 settings: %w", err)
	}
	var settingsProbe map[string]json.RawMessage
	_ = json.Unmarshal(nip29Settings, &settingsProbe)
	if len(settingsProbe) > 0 {
		entry := cfg.NIPs["nip-29"]
		entry.Settings = nip29Settings
		cfg.NIPs["nip-29"] = entry
	}

	if err := cfg.Validate(); err != nil {
		return nil, nil, err
	}

	migratedData, err := json.MarshalIndent(&cfg, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	migratedData = append(migratedData, '\n')

	return &cfg, &MigrationResult{
		Summary: "config migration to config_version 1 (nips map)",
		Diff:    "previous:\n" + string(data) + "\nmigrated:\n" + string(migratedData),
	}, nil
}

// normalizeConfigVersion sets config_version on configs already in the new shape.
func normalizeConfigVersion(cfg *Config) {
	if cfg != nil && cfg.ConfigVersion < ConfigVersionCurrent {
		cfg.ConfigVersion = ConfigVersionCurrent
	}
}

// PendingMigration is set when LoadJSON rewrites a legacy file; main may persist a changelog row after the store opens.
var PendingMigration *MigrationResult
