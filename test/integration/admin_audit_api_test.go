package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michmich112/congee/internal/admin"
	"github.com/michmich112/congee/internal/storage"
	"github.com/michmich112/congee/internal/storage/sqlite"
)

// Integration-style black-box tests for GET /api/audit (same routing as production:
// RequireAdminAuth + StripPrefix("/api") + HandleAudit). Uses real SQLite.
//
// Scenarios:
//   - 401 without Authorization
//   - limit/offset pagination across two pages with identical total
//   - third request verifies offset math (page 2 vs page 0 differ, same total)

const integrationAuditPassword = "integration-audit-pass"

type integrationAuditResponse struct {
	Entries []storage.AuditEntry `json:"entries"`
	Total   int64                `json:"total"`
}

func seedIntegrationAuditRows(ctx context.Context, t *testing.T, st *sqlite.Store, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("%064x", i+100)
		if err := st.SaveAuditEntry(ctx, storage.AuditEntry{
			CreatedAt: int64(5000 - i),
			Action:    "event_accepted",
			Detail:    fmt.Sprintf("event_id=%s kind=1 n=%d", id, i),
			Pubkey:    strings.Repeat("9", 64),
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestIntegrationAdminAuditAPI_Pagination(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "integration_audit.db")
	st, err := sqlite.Open(ctx, dbPath, nil)
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	const rowCount = 25
	seedIntegrationAuditRows(ctx, t, st, rowCount)

	api := http.NewServeMux()
	api.HandleFunc("GET /audit", admin.HandleAudit(st).ServeHTTP)
	h := admin.RequireAdminAuth(integrationAuditPassword, http.StripPrefix("/api", api))
	srv := httptest.NewServer(h)
	defer srv.Close()

	t.Run("unauthorized", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/api/audit?limit=5&offset=0")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("want 401, got %d", resp.StatusCode)
		}
	})

	client := &http.Client{}
	do := func(query string) (integrationAuditResponse, int) {
		req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/audit?"+query, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+integrationAuditPassword)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var body integrationAuditResponse
		if resp.StatusCode == http.StatusOK {
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("decode: %v", err)
			}
		}
		return body, resp.StatusCode
	}

	t.Run("page0_page1_same_total_distinct_rows", func(t *testing.T) {
		p0, code0 := do("limit=7&offset=0")
		if code0 != http.StatusOK {
			t.Fatalf("page0 status %d", code0)
		}
		if p0.Total != rowCount || len(p0.Entries) != 7 {
			t.Fatalf("page0: total=%d len=%d", p0.Total, len(p0.Entries))
		}
		if p0.Entries[0].CreatedAt != 5000 || p0.Entries[6].CreatedAt != 4994 {
			t.Fatalf("page0 window: first=%d last=%d", p0.Entries[0].CreatedAt, p0.Entries[6].CreatedAt)
		}

		p1, code1 := do("limit=7&offset=7")
		if code1 != http.StatusOK {
			t.Fatalf("page1 status %d", code1)
		}
		if p1.Total != rowCount || len(p1.Entries) != 7 {
			t.Fatalf("page1: total=%d len=%d", p1.Total, len(p1.Entries))
		}
		if p1.Total != p0.Total {
			t.Fatalf("total mismatch: %d vs %d", p1.Total, p0.Total)
		}
		if p1.Entries[0].CreatedAt != 4993 {
			t.Fatalf("page1 first created_at: want 4993, got %d", p1.Entries[0].CreatedAt)
		}
		if p0.Entries[0].Detail == p1.Entries[0].Detail {
			t.Fatal("expected different first row per page")
		}
	})

	t.Run("x_admin_token_header", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/audit?limit=1&offset=0", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-Admin-Token", integrationAuditPassword)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status %d", resp.StatusCode)
		}
	})
}
