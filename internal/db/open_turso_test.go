package db

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/storage/sqlitewriter"
	"github.com/michmich112/congee/internal/storage/turso"
	"github.com/rs/zerolog"
)

func TestOpenTurso(t *testing.T) {
	if !turso.HasDriver() {
		t.Skip("libsql driver not available")
	}
	ctx := context.Background()
	dir := t.TempDir()
	sec := config.DatabaseSection{
		Type:    "turso",
		DSN:     filepath.Join(dir, "events.db"),
		MetaDSN: filepath.Join(dir, "meta.db"),
	}
	h, err := Open(ctx, sec, "", zerolog.Nop())
	if err != nil {
		if strings.Contains(err.Error(), "not available") {
			t.Skip(err)
		}
		t.Fatal(err)
	}
	defer func() { _ = h.Close() }()
	if h.Store == nil {
		t.Fatal("expected store")
	}
	_ = sqlitewriter.HasLibsqlDriver()
}
