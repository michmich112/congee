package admin

import (
	"testing"

	"github.com/michmich112/congee/internal/config"
)

func TestMigrationCanonicalDBType(t *testing.T) {
	if g, w := migrationCanonicalDBType(""), "sqlite"; g != w {
		t.Fatalf("empty: got %q want %q", g, w)
	}
	if g, w := migrationCanonicalDBType("  SQLITE  "), "sqlite"; g != w {
		t.Fatalf("sqlite: got %q want %q", g, w)
	}
	if g, w := migrationCanonicalDBType("postgres"), "postgres"; g != w {
		t.Fatalf("postgres: got %q want %q", g, w)
	}
	if g, w := migrationCanonicalDBType("turso"), "turso"; g != w {
		t.Fatalf("turso: got %q want %q", g, w)
	}
}

func TestMigrationSourceMatchesConfig(t *testing.T) {
	cfg := &config.Config{
		Database: config.DatabaseSection{Type: "", DSN: "./congee.db"},
	}
	if !migrationSourceMatchesConfig(cfg, migrationEndpoint{Type: "sqlite", DSN: "./congee.db"}) {
		t.Fatal("expected match for empty type as sqlite")
	}
	if !migrationSourceMatchesConfig(cfg, migrationEndpoint{Type: "sqlite", DSN: "./congee.db"}) {
		t.Fatal("expected match explicit sqlite")
	}
	if migrationSourceMatchesConfig(cfg, migrationEndpoint{Type: "postgres", DSN: "./congee.db"}) {
		t.Fatal("expected mismatch for wrong type")
	}
	if migrationSourceMatchesConfig(cfg, migrationEndpoint{Type: "sqlite", DSN: "./other.db"}) {
		t.Fatal("expected mismatch for wrong dsn")
	}
}
