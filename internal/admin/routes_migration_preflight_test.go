package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michmich112/congee/internal/storage"
	"github.com/michmich112/congee/internal/storage/sqlite"
	"github.com/rs/zerolog"
)

const migrationPreflightHTTPTestPassword = "migration-preflight-test-secret"

func TestHandleMigrationTargetPreflightSQLiteCurrent(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "pf-admin.db")

	st, err := sqlite.Open(ctx, path, nil, zerolog.Nop())
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	api := http.NewServeMux()
	api.HandleFunc("POST /migration/target-preflight", handleMigrationTargetPreflight(zerolog.Nop()))
	h := RequireAdminAuth(migrationPreflightHTTPTestPassword, http.StripPrefix("/api", api))

	payload := map[string]any{
		"target": map[string]string{"type": "sqlite", "dsn": path},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/migration/target-preflight", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+migrationPreflightHTTPTestPassword)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	var out storage.MigrationTargetPreflight
	if err := json.NewDecoder(rr.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Status != storage.MigrationPreflightCurrent {
		t.Fatalf("status=%q detail=%q", out.Status, out.Detail)
	}
	if out.ExpectedVersion != sqlite.CurrentSchemaVersion() {
		t.Fatalf("expected_version=%d", out.ExpectedVersion)
	}
}

func TestHandleMigrationTargetPreflightBadJSON(t *testing.T) {
	api := http.NewServeMux()
	api.HandleFunc("POST /migration/target-preflight", handleMigrationTargetPreflight(zerolog.Nop()))
	h := RequireAdminAuth(migrationPreflightHTTPTestPassword, http.StripPrefix("/api", api))

	req := httptest.NewRequest(http.MethodPost, "/api/migration/target-preflight", bytes.NewReader([]byte(`{`)))
	req.Header.Set("Authorization", "Bearer "+migrationPreflightHTTPTestPassword)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", rr.Code)
	}
}

func TestHandleMigrationTargetPreflightMissingType(t *testing.T) {
	api := http.NewServeMux()
	api.HandleFunc("POST /migration/target-preflight", handleMigrationTargetPreflight(zerolog.Nop()))
	h := RequireAdminAuth(migrationPreflightHTTPTestPassword, http.StripPrefix("/api", api))

	raw, _ := json.Marshal(map[string]any{"target": map[string]string{"type": "", "dsn": "x"}})
	req := httptest.NewRequest(http.MethodPost, "/api/migration/target-preflight", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+migrationPreflightHTTPTestPassword)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", rr.Code)
	}
}
