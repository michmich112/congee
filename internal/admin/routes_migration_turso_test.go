package admin

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
	"github.com/michmich112/congee/internal/storage/sqlite"
	"github.com/michmich112/congee/internal/storage/sqlitemeta"
	"github.com/michmich112/congee/internal/storage/turso"
	"github.com/rs/zerolog"
)

const migrationTursoHTTPTestPassword = "migration-turso-test-secret"

func TestHandleMigrationStartSQLiteToTursoNative(t *testing.T) {
	if !turso.HasDriver() {
		t.Skip("libsql driver not available")
	}
	ctx := context.Background()
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "src.db")
	dstPath := filepath.Join(dir, "dst.db")
	cfgPath := filepath.Join(dir, "config.json")
	metaPath := filepath.Join(dir, "meta.db")

	src, err := sqlite.Open(ctx, srcPath, nil, zerolog.Nop())
	if err != nil {
		if strings.Contains(err.Error(), "not available") {
			t.Skip(err)
		}
		t.Fatal(err)
	}
	ev := &nostr.Event{
		ID:        strings.Repeat("a", 64),
		PubKey:    strings.Repeat("b", 64),
		CreatedAt: 1,
		Kind:      1,
		Content:   "via admin native",
		Sig:       strings.Repeat("c", 128),
	}
	if err := src.SaveEvent(ctx, ev); err != nil {
		t.Fatal(err)
	}
	if err := src.Close(); err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultConfig()
	cfg.Database.Type = "sqlite"
	cfg.Database.DSN = srcPath
	cfg.Database.MetaDSN = metaPath
	if err := config.WriteConfigAtomic(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}

	meta, err := sqlitemeta.Open(ctx, metaPath, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = meta.Close() }()

	var cfgMu sync.Mutex
	api := http.NewServeMux()
	api.HandleFunc("POST /migration/start", handleMigrationStart(zerolog.Nop(), cfgPath, &cfgMu, meta, nil, nil))
	h := RequireAdminAuth(migrationTursoHTTPTestPassword, http.StripPrefix("/api", api))

	payload := map[string]any{
		"source":               map[string]string{"type": "sqlite", "dsn": srcPath},
		"target":               map[string]string{"type": "turso", "dsn": dstPath},
		"make_target_primary":  false,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/migration/start", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+migrationTursoHTTPTestPassword)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}

	var done map[string]any
	sc := bufio.NewScanner(bytes.NewReader(rr.Body.Bytes()))
	var curEvent string
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "event: ") {
			curEvent = strings.TrimPrefix(line, "event: ")
			continue
		}
		if strings.HasPrefix(line, "data: ") && curEvent == "done" {
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &done); err != nil {
				t.Fatal(err)
			}
		}
		if strings.HasPrefix(line, "data: ") && curEvent == "error" {
			t.Fatalf("migration error SSE: %s", line)
		}
	}
	if done == nil {
		t.Fatalf("no done event in SSE body:\n%s", rr.Body.String())
	}
	if status, _ := done["status"].(string); status != "ok" {
		t.Fatalf("done status=%v body=%v", done["status"], done)
	}
	sumRaw, ok := done["summary"].(map[string]any)
	if !ok {
		t.Fatalf("summary missing: %#v", done)
	}
	if events, _ := sumRaw["events_inserted"].(float64); events != 1 {
		t.Fatalf("events_inserted=%v want 1", sumRaw["events_inserted"])
	}

	if _, err := os.Stat(dstPath); err != nil {
		t.Fatalf("expected destination file: %v", err)
	}
	dst, err := turso.Open(ctx, dstPath, nil, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dst.Close() }()
	out, err := dst.QueryEvents(ctx, []nostr.Filter{{IDs: []string{ev.ID}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Content != "via admin native" {
		t.Fatalf("query dst: %+v", out)
	}
}

func TestHandleMigrationTargetPreflightTursoEmpty(t *testing.T) {
	if !turso.HasDriver() {
		t.Skip("libsql driver not available")
	}
	api := http.NewServeMux()
	api.HandleFunc("POST /migration/target-preflight", handleMigrationTargetPreflight(zerolog.Nop()))
	h := RequireAdminAuth(migrationTursoHTTPTestPassword, http.StripPrefix("/api", api))

	path := filepath.Join(t.TempDir(), "empty-turso.db")
	payload := map[string]any{
		"target": map[string]string{"type": "turso", "dsn": path},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/migration/target-preflight", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+migrationTursoHTTPTestPassword)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	var out storage.MigrationTargetPreflight
	if err := json.NewDecoder(rr.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Status != storage.MigrationPreflightEmpty {
		t.Fatalf("status=%q detail=%q", out.Status, out.Detail)
	}
}
