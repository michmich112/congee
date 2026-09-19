package config

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestPromoteLegacySQLite(t *testing.T) {
	c := minimalValidConfig()
	c.Database.Type = "sqlite"
	c.Database.DSN = "./keep.db"
	if !PromoteLegacySQLite(c) {
		t.Fatal("expected promote")
	}
	if c.Database.Type != "turso" {
		t.Fatalf("type: %q", c.Database.Type)
	}
	if c.Database.DSN != "./keep.db" {
		t.Fatalf("dsn rewritten: %q", c.Database.DSN)
	}
	if PromoteLegacySQLite(c) {
		t.Fatal("second promote should be a no-op")
	}

	empty := minimalValidConfig()
	empty.Database.Type = ""
	if !PromoteLegacySQLite(empty) {
		t.Fatal("expected empty type to promote")
	}
	if empty.Database.Type != "turso" {
		t.Fatalf("empty type: %q", empty.Database.Type)
	}

	pg := minimalValidConfig()
	pg.Database.Type = "postgres"
	pg.Database.DSN = "postgres://localhost/test?sslmode=disable"
	if PromoteLegacySQLite(pg) {
		t.Fatal("postgres must not promote")
	}
}

func TestParseConfigJSONPromotesSQLite(t *testing.T) {
	src := DefaultConfig()
	src.Database.Type = "sqlite"
	raw, err := json.Marshal(src)
	if err != nil {
		t.Fatal(err)
	}
	c, err := ParseConfigJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if c.Database.Type != "turso" {
		t.Fatalf("type: %q", c.Database.Type)
	}
	if c.Database.DSN != src.Database.DSN {
		t.Fatalf("dsn: %q", c.Database.DSN)
	}
}

func TestPromoteLegacySQLitePersists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	c := DefaultConfig()
	c.Database.Type = "sqlite"
	if err := WriteConfigAtomic(path, c); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Database.Type != "turso" {
		t.Fatalf("write/load should persist turso, got %q", loaded.Database.Type)
	}
	if loaded.Database.DSN != c.Database.DSN {
		t.Fatalf("dsn rewritten: %q", loaded.Database.DSN)
	}
}
