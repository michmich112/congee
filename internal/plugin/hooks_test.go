package plugin

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRunManifestHookWritesMarker(t *testing.T) {
	pkg := t.TempDir()
	if err := os.MkdirAll(filepath.Join(pkg, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\necho \"$1\" > \"$CONGEE_PLUGIN_DATA_DIR/hook\"\n"
	if err := os.WriteFile(filepath.Join(pkg, "bin", "p"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	man := &Manifest{
		ID:   "fixture",
		Exec: map[string]string{runtime.GOOS + "_" + runtime.GOARCH: "bin/p"},
		Hooks: PluginHooks{
			Install: []string{"--hook=install"},
		},
	}
	data := filepath.Join(pkg, "data")
	if err := runManifestHook(context.Background(), man, pkg, data, "fixture", "{}", man.Hooks.Install, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(data, "hook"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "--hook=install") {
		t.Fatalf("got %q", got)
	}
}

func TestLoadManifestUpdateHook(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(`{"id":"fixture","hooks":{"update":["--hook=update"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	man, err := loadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(man.Hooks.Update) != 1 || man.Hooks.Update[0] != "--hook=update" {
		t.Fatalf("update hook: %#v", man.Hooks.Update)
	}
}

func TestRunManifestHookMissingIsNoop(t *testing.T) {
	if err := runManifestHook(context.Background(), &Manifest{ID: "x"}, "", "", "x", "", nil, time.Second); err != nil {
		t.Fatal(err)
	}
}
