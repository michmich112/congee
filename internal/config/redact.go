package config

import (
	"bytes"
	"encoding/json"
	"strings"
)

// RedactSecretsForLog returns JSON suitable for changelog / logs with plugin DB passwords removed.
func RedactSecretsForLog(raw []byte) []byte {
	if len(bytes.TrimSpace(raw)) == 0 {
		return raw
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return raw
	}
	redactPluginSettingsMap(m)
	out, err := json.Marshal(m)
	if err != nil {
		return raw
	}
	return out
}

func redactPluginSettingsMap(m map[string]any) {
	plugins, ok := m["plugins"].(map[string]any)
	if !ok {
		return
	}
	items, ok := plugins["items"].([]any)
	if !ok {
		return
	}
	for _, it := range items {
		item, ok := it.(map[string]any)
		if !ok {
			continue
		}
		settings, ok := item["settings"].(map[string]any)
		if !ok {
			continue
		}
		for k := range settings {
			lk := strings.ToLower(k)
			if strings.Contains(lk, "password") || strings.Contains(lk, "secret") {
				settings[k] = ""
			}
		}
	}
}
