package turso

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage/sqlite"
	"github.com/michmich112/congee/internal/storage/sqlitewriter"
	"github.com/rs/zerolog"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func TestTursoCRUD(t *testing.T) {
	if !HasDriver() {
		t.Skip("libsql driver not available")
	}
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "t.db")
	st, err := Open(ctx, path, nil, zerolog.Nop())
	if err != nil {
		if strings.Contains(err.Error(), "not available") {
			t.Skip(err)
		}
		t.Fatal(err)
	}
	defer st.Close()

	ev := &nostr.Event{
		ID:        nostrRepeat("a", 64),
		PubKey:    nostrRepeat("b", 64),
		CreatedAt: 10,
		Kind:      1,
		Tags:      [][]string{{"e", nostrRepeat("c", 64)}},
		Content:   "hello turso",
		Sig:       nostrRepeat("d", 128),
	}
	if err := st.SaveEvent(ctx, ev); err != nil {
		t.Fatal(err)
	}
	out, err := st.QueryEvents(ctx, []nostr.Filter{{IDs: []string{ev.ID}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Content != "hello turso" {
		t.Fatalf("query: got %+v", out)
	}
}

func nostrRepeat(c string, n int) string {
	return strings.Repeat(c, n)
}

func TestPreflightEmptyTurso(t *testing.T) {
	if !HasDriver() {
		t.Skip("libsql driver not available")
	}
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "new.db")
	out := PreflightMigrationTarget(ctx, path, zerolog.Nop())
	if out.Status != "empty" {
		t.Fatalf("status: got %q want empty", out.Status)
	}
}

func TestPreflightCurrentTurso(t *testing.T) {
	if !HasDriver() {
		t.Skip("libsql driver not available")
	}
	ctx := context.Background()
	log := zerolog.Nop()
	path := filepath.Join(t.TempDir(), "current.db")
	// Seed via modernc SQLite then preflight with libSQL (same on-disk schema).
	src, err := sqlite.Open(ctx, path, nil, log)
	if err != nil {
		if strings.Contains(err.Error(), "not available") {
			t.Skip(err)
		}
		t.Fatal(err)
	}
	if err := src.Close(); err != nil {
		t.Fatal(err)
	}

	out := PreflightMigrationTarget(ctx, path, log)
	if out.Status != "current" {
		t.Fatalf("status: got %q detail=%q", out.Status, out.Detail)
	}
	if out.ExpectedVersion != CurrentSchemaVersion() {
		t.Fatalf("expected_version: got %d", out.ExpectedVersion)
	}
	if out.ReportedVersion == nil || *out.ReportedVersion != CurrentSchemaVersion() {
		t.Fatalf("reported_version: %+v", out.ReportedVersion)
	}
}

func TestPreflightBehindTurso(t *testing.T) {
	if !HasDriver() {
		t.Skip("libsql driver not available")
	}
	ctx := context.Background()
	log := zerolog.Nop()
	path := filepath.Join(t.TempDir(), "behind.db")
	src, err := sqlite.Open(ctx, path, nil, log)
	if err != nil {
		if strings.Contains(err.Error(), "not available") {
			t.Skip(err)
		}
		t.Fatal(err)
	}
	if err := src.Close(); err != nil {
		t.Fatal(err)
	}

	sqldb, err := sql.Open(sqliteshim.ShimName, sqlitewriter.NormalizeDSN(path))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sqldb.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, 6)); err != nil {
		_ = sqldb.Close()
		t.Fatal(err)
	}
	if err := sqldb.Close(); err != nil {
		t.Fatal(err)
	}

	out := PreflightMigrationTarget(ctx, path, log)
	if out.Status != "behind" {
		t.Fatalf("status: got %q detail=%q", out.Status, out.Detail)
	}
	if out.ReportedVersion == nil || *out.ReportedVersion != 6 {
		t.Fatalf("reported_version: %+v", out.ReportedVersion)
	}
}
