package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyBootstrapEnvOverrides_relayAndAdminPorts(t *testing.T) {
	c := minimalValidConfig()
	t.Setenv("CONGEE_RELAY_PORT", "8080")
	t.Setenv("CONGEE_ADMIN_PORT", "8081")
	if err := ApplyBootstrapEnvOverrides(c); err != nil {
		t.Fatal(err)
	}
	if c.Relay.Port != 8080 || c.Admin.Port != 8081 {
		t.Fatalf("ports: relay=%d admin=%d", c.Relay.Port, c.Admin.Port)
	}
}

func TestApplyBootstrapEnvOverrides_sqliteDataDir(t *testing.T) {
	c := minimalValidConfig()
	dir := t.TempDir()
	t.Setenv("CONGEE_DATA_DIR", dir)
	if err := ApplyBootstrapEnvOverrides(c); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "congee.db")
	wantMeta := filepath.Join(dir, "congee-meta.db")
	if c.Database.Type != "sqlite" {
		t.Fatalf("type: %q", c.Database.Type)
	}
	if c.Database.DSN != want {
		t.Fatalf("dsn: got %q want %q", c.Database.DSN, want)
	}
	if c.Database.MetaDSN != wantMeta {
		t.Fatalf("meta_dsn: got %q want %q", c.Database.MetaDSN, wantMeta)
	}
}

func TestApplyBootstrapEnvOverrides_dataDirPostgresIgnored(t *testing.T) {
	c := minimalValidConfig()
	c.Database = DatabaseSection{Type: "postgres", DSN: "postgres://localhost/test?sslmode=disable"}
	t.Setenv("CONGEE_DATA_DIR", "/data")
	if err := ApplyBootstrapEnvOverrides(c); err != nil {
		t.Fatal(err)
	}
	if c.Database.DSN != "postgres://localhost/test?sslmode=disable" {
		t.Fatalf("postgres dsn should be unchanged, got %q", c.Database.DSN)
	}
}

func TestApplyBootstrapEnvOverrides_emptyTypeDefaultsToTursoWithDataDir(t *testing.T) {
	c := minimalValidConfig()
	c.Database.Type = ""
	dir := filepath.Join(t.TempDir(), "nested")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONGEE_DATA_DIR", dir)
	if err := ApplyBootstrapEnvOverrides(c); err != nil {
		t.Fatal(err)
	}
	if c.Database.Type != DefaultDatabaseType {
		t.Fatalf("type: %q", c.Database.Type)
	}
	want := filepath.Join(dir, "congee.db")
	if c.Database.DSN != want {
		t.Fatalf("dsn: got %q want %q", c.Database.DSN, want)
	}
}

func TestApplyBootstrapEnvOverrides_tursoDataDir(t *testing.T) {
	c := minimalValidConfig()
	c.Database.Type = "turso"
	dir := t.TempDir()
	t.Setenv("CONGEE_DATA_DIR", dir)
	if err := ApplyBootstrapEnvOverrides(c); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "congee.db")
	if c.Database.Type != "turso" {
		t.Fatalf("type: %q", c.Database.Type)
	}
	if c.Database.DSN != want {
		t.Fatalf("dsn: got %q want %q", c.Database.DSN, want)
	}
}

func TestApplyBootstrapEnvOverrides_invalidRelayPort(t *testing.T) {
	c := minimalValidConfig()
	t.Setenv("CONGEE_RELAY_PORT", "99999")
	if err := ApplyBootstrapEnvOverrides(c); err == nil {
		t.Fatal("expected error")
	}
}

func TestApplyBootstrapEnvOverrides_invalidRelayPortNonNumeric(t *testing.T) {
	c := minimalValidConfig()
	t.Setenv("CONGEE_RELAY_PORT", "abc")
	if err := ApplyBootstrapEnvOverrides(c); err == nil {
		t.Fatal("expected error")
	}
}
