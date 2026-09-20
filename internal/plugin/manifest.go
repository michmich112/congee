package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Manifest is plugin.json in an installed package.
type Manifest struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Version    string            `json:"version"`
	APIVersion int               `json:"api_version"`
	Exec       map[string]string `json:"exec"`
	Hooks      PluginHooks       `json:"hooks"`
}

// PluginHooks are extra argv passed to the plugin exec after install, before Serve, and before package delete.
// Example: "install": ["--hook=install"], "launch": ["--hook=launch"], "uninstall": ["--hook=uninstall"].
type PluginHooks struct {
	Install   []string `json:"install,omitempty"`
	Launch    []string `json:"launch,omitempty"`
	Uninstall []string `json:"uninstall,omitempty"`
}

func loadManifest(dir string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, "plugin.json"))
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("plugin.json: %w", err)
	}
	if m.ID == "" {
		return nil, fmt.Errorf("plugin.json: id is required")
	}
	if m.APIVersion != 0 && m.APIVersion != 1 {
		return nil, fmt.Errorf("plugin.json: unsupported api_version %d", m.APIVersion)
	}
	return &m, nil
}

func (m *Manifest) execPath(pkgDir string) (string, error) {
	key := runtime.GOOS + "_" + runtime.GOARCH
	rel, ok := m.Exec[key]
	if !ok || rel == "" {
		return "", fmt.Errorf("plugin %s: no exec for %s", m.ID, key)
	}
	p := filepath.Join(pkgDir, rel)
	st, err := os.Stat(p)
	if err != nil {
		return "", err
	}
	if st.IsDir() {
		return "", fmt.Errorf("plugin exec is a directory: %s", p)
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return abs, nil
}
