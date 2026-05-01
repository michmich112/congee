package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
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
//   - kind= filter (repeat for OR; suffix " kind=<n>" as written by the relay post-hook)
//   - GET /api/audit/kinds returns distinct kinds from recent rows
//   - action= exact filter
//   - pubkey= exact filter
//   - since= and until= unix bounds on created_at (inclusive)
//   - combined filters narrow the result set consistently
//   - X-Admin-Token accepted

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
			Detail:    fmt.Sprintf("event_id=%s conn_id=c stored=true kind=%d", id, i%4),
			Pubkey:    strings.Repeat("9", 64),
		}); err != nil {
			t.Fatal(err)
		}
	}
}

// seedAuditFilterRows writes 10 rows: created_at 2000..2009, kinds (i%3)+1 in relay-shaped detail,
// actions event_accepted for i<8 and special_only for i>=8, pubkeys alternate a.. / b..
func seedAuditFilterRows(ctx context.Context, t *testing.T, st *sqlite.Store) {
	t.Helper()
	pkA := strings.Repeat("a", 64)
	pkB := strings.Repeat("b", 64)
	for i := 0; i < 10; i++ {
		action := "event_accepted"
		if i >= 8 {
			action = "special_only"
		}
		pub := pkA
		if i%2 == 1 {
			pub = pkB
		}
		kind := (i % 3) + 1
		id := fmt.Sprintf("%064x", i+1)
		detail := fmt.Sprintf("event_id=%s conn_id=c%d stored=true kind=%d", id, i, kind)
		if err := st.SaveAuditEntry(ctx, storage.AuditEntry{
			CreatedAt: int64(2000 + i),
			Action:    action,
			Detail:    detail,
			Pubkey:    pub,
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

func TestIntegrationAdminAuditAPI_QueryFilters(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "audit_filters.db")
	st, err := sqlite.Open(ctx, dbPath, nil)
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	seedAuditFilterRows(ctx, t, st)

	api := http.NewServeMux()
	api.HandleFunc("GET /audit", admin.HandleAudit(st).ServeHTTP)
	h := admin.RequireAdminAuth(integrationAuditPassword, http.StripPrefix("/api", api))
	srv := httptest.NewServer(h)
	defer srv.Close()

	client := &http.Client{}
	do := func(query string) (integrationAuditResponse, int) {
		t.Helper()
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

	pkB := strings.Repeat("b", 64)

	t.Run("kind_multi_or", func(t *testing.T) {
		// kind 1: i=0,3,6,9; kind 2: i=1,4,7 => 7 rows (disjoint)
		body, code := do("kind=1&kind=2&limit=50&offset=0")
		if code != http.StatusOK {
			t.Fatalf("status %d", code)
		}
		if body.Total != 7 || len(body.Entries) != 7 {
			t.Fatalf("kind=1&kind=2: want 7, got total=%d len=%d", body.Total, len(body.Entries))
		}
	})

	t.Run("kind_filter", func(t *testing.T) {
		// kind=2 at i=1,4,7 (all event_accepted)
		body, code := do("kind=2&limit=50&offset=0")
		if code != http.StatusOK {
			t.Fatalf("status %d", code)
		}
		if body.Total != 3 || len(body.Entries) != 3 {
			t.Fatalf("kind=2: want total 3, got %d len %d", body.Total, len(body.Entries))
		}
		for _, e := range body.Entries {
			if !strings.HasSuffix(e.Detail, " kind=2") {
				t.Fatalf("expected suffix \" kind=2\": %q", e.Detail)
			}
		}
	})

	t.Run("action_filter", func(t *testing.T) {
		body, code := do("action=special_only&limit=50&offset=0")
		if code != http.StatusOK {
			t.Fatalf("status %d", code)
		}
		if body.Total != 2 || len(body.Entries) != 2 {
			t.Fatalf("special_only: want 2, got %d", body.Total)
		}
	})

	t.Run("pubkey_filter", func(t *testing.T) {
		body, code := do("pubkey=" + pkB + "&limit=50&offset=0")
		if code != http.StatusOK {
			t.Fatalf("status %d", code)
		}
		if body.Total != 5 || len(body.Entries) != 5 {
			t.Fatalf("pubkey b: want 5, got %d", body.Total)
		}
	})

	t.Run("since_until_filter", func(t *testing.T) {
		// created_at 2003..2006 inclusive => 4 rows
		body, code := do("since=2003&until=2006&limit=50&offset=0")
		if code != http.StatusOK {
			t.Fatalf("status %d", code)
		}
		if body.Total != 4 || len(body.Entries) != 4 {
			t.Fatalf("time range: want 4, got total=%d len=%d", body.Total, len(body.Entries))
		}
		for _, e := range body.Entries {
			if e.CreatedAt < 2003 || e.CreatedAt > 2006 {
				t.Fatalf("out of range: %d", e.CreatedAt)
			}
		}
	})

	t.Run("combined_kind_and_action", func(t *testing.T) {
		// kind=1 at i=0,3,6; i=6 only event_accepted with kind 1 — i=0,3,6 all event_accepted => 3 rows
		body, code := do("kind=1&action=event_accepted&limit=50&offset=0")
		if code != http.StatusOK {
			t.Fatalf("status %d", code)
		}
		if body.Total != 3 || len(body.Entries) != 3 {
			t.Fatalf("combined: want 3, got %d", body.Total)
		}
	})

	t.Run("combined_kind_pubkey_since", func(t *testing.T) {
		// pkB at odd i. kind=3 when (i%3)+1=3 => i=2,5,8. Odd => i=5,9. action event_accepted => i=5 only (i=9 is special_only).
		body, code := do("kind=3&pubkey=" + pkB + "&action=event_accepted&since=2000&until=2008&limit=50&offset=0")
		if code != http.StatusOK {
			t.Fatalf("status %d", code)
		}
		if body.Total != 1 || len(body.Entries) != 1 {
			t.Fatalf("combined narrow: want 1 row (i=5), got total=%d", body.Total)
		}
		if body.Entries[0].CreatedAt != 2005 {
			t.Fatalf("want row i=5 created_at 2005, got %d", body.Entries[0].CreatedAt)
		}
	})
}

func TestIntegrationAdminAuditAPI_KindFilterMatchesRelayDetailSuffix(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "audit_kind_suffix.db")
	st, err := sqlite.Open(ctx, dbPath, nil)
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	id := strings.Repeat("c", 64)
	// Exact shape from internal/relay/nip01.go post-hook (note space before kind=).
	detail := fmt.Sprintf("event_id=%s conn_id=conn1 stored=true kind=7", id)
	if err := st.SaveAuditEntry(ctx, storage.AuditEntry{
		CreatedAt: 1,
		Action:    "event_accepted",
		Detail:    detail,
		Pubkey:    strings.Repeat("d", 64),
	}); err != nil {
		t.Fatal(err)
	}

	api := http.NewServeMux()
	api.HandleFunc("GET /audit", admin.HandleAudit(st).ServeHTTP)
	h := admin.RequireAdminAuth(integrationAuditPassword, http.StripPrefix("/api", api))
	srv := httptest.NewServer(h)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/audit?kind="+strconv.Itoa(7)+"&limit=10&offset=0", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+integrationAuditPassword)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var body integrationAuditResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Total != 1 || len(body.Entries) != 1 {
		t.Fatalf("want 1 row for relay-shaped detail, got total=%d len=%d", body.Total, len(body.Entries))
	}
}

func TestIntegrationAdminAuditAPI_AuditKindsEndpoint(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "audit_kinds_list.db")
	st, err := sqlite.Open(ctx, dbPath, nil)
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	seedAuditFilterRows(ctx, t, st)

	api := http.NewServeMux()
	api.HandleFunc("GET /audit/kinds", admin.HandleAuditKinds(st).ServeHTTP)
	h := admin.RequireAdminAuth(integrationAuditPassword, http.StripPrefix("/api", api))
	srv := httptest.NewServer(h)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/audit/kinds", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+integrationAuditPassword)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var body struct {
		Kinds []int `json:"kinds"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Kinds) != 3 || body.Kinds[0] != 1 || body.Kinds[1] != 2 || body.Kinds[2] != 3 {
		t.Fatalf("want kinds [1,2,3] from seedAuditFilterRows, got %v", body.Kinds)
	}
}
