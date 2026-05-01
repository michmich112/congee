package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureRelayInstanceIDFileWritesUUID(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	c := minimalValidConfig()
	if err := WriteConfigAtomic(p, c); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadJSON(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Unsetenv("CONGEE_INSTANCE_ID") })
	if err := EnsureRelayInstanceIDFile(loaded, p); err != nil {
		t.Fatal(err)
	}
	if loaded.Relay.InstanceID == "" {
		t.Fatal("expected instance id")
	}
	if _, err := LoadJSON(p); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), loaded.Relay.InstanceID) {
		t.Fatalf("file should contain instance id")
	}
}

func TestEnsureRelayInstanceIDFileSkippedWhenEnvSet(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	c := minimalValidConfig()
	if err := WriteConfigAtomic(p, c); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadJSON(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONGEE_INSTANCE_ID", "env-fixed-id")
	if err := EnsureRelayInstanceIDFile(loaded, p); err != nil {
		t.Fatal(err)
	}
	if loaded.Relay.InstanceID != "" {
		t.Fatal("env locked: should not mutate cfg in memory for empty field")
	}
}

func TestEnsureRelayInstanceIDFileNoOpWhenAlreadySet(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	c := minimalValidConfig()
	c.Relay.InstanceID = "already"
	if err := WriteConfigAtomic(p, c); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadJSON(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Unsetenv("CONGEE_INSTANCE_ID") })
	if err := EnsureRelayInstanceIDFile(loaded, p); err != nil {
		t.Fatal(err)
	}
	if loaded.Relay.InstanceID != "already" {
		t.Fatalf("got %q", loaded.Relay.InstanceID)
	}
}

func TestValidateRelayInstanceIDTooLong(t *testing.T) {
	c := minimalValidConfig()
	c.Relay.InstanceID = strings.Repeat("a", 257)
	if err := c.Validate(); err == nil {
		t.Fatal("expected error")
	}
}

func TestEffectiveRelayInstanceIDPrefersEnv(t *testing.T) {
	c := minimalValidConfig()
	c.Relay.InstanceID = "from-config"
	t.Setenv("CONGEE_INSTANCE_ID", "from-env")
	t.Cleanup(func() { _ = os.Unsetenv("CONGEE_INSTANCE_ID") })
	if got := EffectiveRelayInstanceID(c); got != "from-env" {
		t.Fatalf("got %q", got)
	}
}
