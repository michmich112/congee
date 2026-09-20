package plugin

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/michmich112/congee/internal/config"
	"github.com/rs/zerolog"
)

func writeHookPkg(t *testing.T, root, id string, withUninstall bool) string {
	t.Helper()
	pkg := filepath.Join(root, id)
	if err := os.MkdirAll(filepath.Join(pkg, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(pkg, "data", "models"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "data", "models", "minilm.onnx"), []byte("blob"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "data", "keep.txt"), []byte("index"), 0o644); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n" +
		"case \"$1\" in\n" +
		"--hook=uninstall) rm -rf \"$CONGEE_PLUGIN_DATA_DIR/models\" ;;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(pkg, "bin", "p"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	hooks := `"install":["--hook=install"],"launch":["--hook=launch"]`
	if withUninstall {
		hooks += `,"uninstall":["--hook=uninstall"]`
	}
	man := `{"id":"` + id + `","name":"Fixture","version":"0.0.1","api_version":1,"exec":{"` +
		runtime.GOOS + `_` + runtime.GOARCH + `":"bin/p"},"hooks":{` + hooks + `}}`
	if err := os.WriteFile(filepath.Join(pkg, "plugin.json"), []byte(man), 0o644); err != nil {
		t.Fatal(err)
	}
	return pkg
}

func TestUninstallWipeRemovesPackageAndConfig(t *testing.T) {
	root := t.TempDir()
	writeHookPkg(t, root, "fixture", true)
	cfg := config.DefaultConfig()
	cfg.Plugins.Directory = root
	cfg.Plugins.Items = []config.PluginItem{{ID: "fixture", Enabled: true}}
	m := NewManager(cfg, filepath.Join(root, "config.json"), nil, zerolog.Nop())
	if err := m.Uninstall("fixture", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "fixture")); !os.IsNotExist(err) {
		t.Fatalf("package still present: %v", err)
	}
	if _, idx := config.PluginItemByID(cfg, "fixture"); idx >= 0 {
		t.Fatal("config item still present after wipe")
	}
}

func TestUninstallKeepsDataAfterHookRemovesModels(t *testing.T) {
	root := t.TempDir()
	pkg := writeHookPkg(t, root, "fixture", true)
	cfg := config.DefaultConfig()
	cfg.Plugins.Directory = root
	cfg.Plugins.Items = []config.PluginItem{{ID: "fixture", Enabled: true}}
	m := NewManager(cfg, filepath.Join(root, "config.json"), nil, zerolog.Nop())
	if err := m.Uninstall("fixture", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(pkg, "bin", "p")); !os.IsNotExist(err) {
		t.Fatal("bin should be removed")
	}
	got, err := os.ReadFile(filepath.Join(pkg, "data", "keep.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "index" {
		t.Fatalf("kept data: %s", got)
	}
	if _, err := os.Stat(filepath.Join(pkg, "data", "models", "minilm.onnx")); !os.IsNotExist(err) {
		t.Fatal("models should be removed by uninstall hook")
	}
	if _, idx := config.PluginItemByID(cfg, "fixture"); idx >= 0 {
		t.Fatal("config item still present")
	}
}

func TestUninstallHookMissingStillClearsConfig(t *testing.T) {
	root := t.TempDir()
	writeHookPkg(t, root, "fixture", false)
	cfg := config.DefaultConfig()
	cfg.Plugins.Directory = root
	cfg.Plugins.Items = []config.PluginItem{{ID: "fixture"}}
	m := NewManager(cfg, filepath.Join(root, "config.json"), nil, zerolog.Nop())
	if err := m.Uninstall("fixture", true); err != nil {
		t.Fatal(err)
	}
	if _, idx := config.PluginItemByID(cfg, "fixture"); idx >= 0 {
		t.Fatal("config item still present")
	}
}

func TestDirUsesDataDirPlugins(t *testing.T) {
	t.Setenv("CONGEE_DATA_DIR", "/data")
	got := Dir(&config.Config{}, "")
	if got != "/data/plugins" {
		t.Fatalf("Dir=%q want /data/plugins", got)
	}
}
