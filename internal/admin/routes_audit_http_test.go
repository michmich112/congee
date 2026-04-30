package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michmich112/congee/internal/storage"
	"github.com/michmich112/congee/internal/storage/sqlite"
)

// Scenarios covered by TestHandleAudit_HTTP and helpers:
//   - default limit (no limit param) returns up to 50 rows and correct total
//   - explicit limit and offset slice pages; newest-first order preserved across pages
//   - total is identical for different pages with the same filters
//   - last page is short when (offset + limit) > total
//   - large limit (no server cap) passes through to storage
//   - invalid limit (0, negative) falls back to default 50
//   - negative offset is ignored (treated as 0)
//   - action= exact filter on total and entries
//   - since / until unix filters on total and entries
//   - pubkey= exact filter
//   - kind= matches rows whose detail ends with NIP-01-style "... kind=<n>"
//   - POST returns 405; missing auth returns 401
//   - JSON body includes entries and total

const auditHTTPTestPassword = "audit-http-test-secret"

type auditAPIResponse struct {
	Entries []storage.AuditEntry `json:"entries"`
	Total   int64                `json:"total"`
}

func auditHTTPHandler(st storage.Store) http.Handler {
	api := http.NewServeMux()
	api.HandleFunc("GET /audit", HandleAudit(st).ServeHTTP)
	return RequireAdminAuth(auditHTTPTestPassword, http.StripPrefix("/api", api))
}

func getAuditJSON(t *testing.T, h http.Handler, query string) (auditAPIResponse, int) {
	t.Helper()
	path := "/api/audit"
	if query != "" {
		path += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+auditHTTPTestPassword)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	var body auditAPIResponse
	if rr.Code == http.StatusOK {
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v body=%s", err, rr.Body.String())
		}
	}
	return body, rr.Code
}

// seedAuditHTTPTestData writes 32 rows: created_at 3000..2969 (newest first by id order),
// rows i in [0,26] action event_accepted; i in [27,31] action onlyme (5 rows).
// Rows i in [0,2] use pubkey pkRare; others use pkCommon.
// Detail matches NIP-01 audit post-hook shape and ends with kind=(i mod 5).
func seedAuditHTTPTestData(ctx context.Context, t *testing.T, st *sqlite.Store) {
	t.Helper()
	pkCommon := strings.Repeat("a", 64)
	pkRare := strings.Repeat("f", 64)
	for i := 0; i < 32; i++ {
		id := fmt.Sprintf("%064x", i+1)
		action := "event_accepted"
		if i >= 27 {
			action = "onlyme"
		}
		pub := pkCommon
		if i < 3 {
			pub = pkRare
		}
		kindN := i % 5
		detail := fmt.Sprintf("event_id=%s conn_id=c%d stored=true kind=%d", id, i, kindN)
		if err := st.SaveAuditEntry(ctx, storage.AuditEntry{
			CreatedAt: int64(3000 - i),
			Action:    action,
			Detail:    detail,
			Pubkey:    pub,
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func assertDescCreatedAt(t *testing.T, entries []storage.AuditEntry) {
	t.Helper()
	for i := 1; i < len(entries); i++ {
		if entries[i].CreatedAt > entries[i-1].CreatedAt {
			t.Fatalf("not descending at %d: %d > %d", i, entries[i].CreatedAt, entries[i-1].CreatedAt)
		}
	}
}

func TestHandleAudit_HTTP(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "audit_http.db")
	st, err := sqlite.Open(ctx, path, nil)
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	seedAuditHTTPTestData(ctx, t, st)
	h := auditHTTPHandler(st)

	pkRare := strings.Repeat("f", 64)

	t.Run("unauthorized_without_credentials", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/audit", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("want 401, got %d", rr.Code)
		}
	})

	t.Run("method_not_allowed_post", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/audit", nil)
		req.Header.Set("Authorization", "Bearer "+auditHTTPTestPassword)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusMethodNotAllowed {
			t.Fatalf("want 405, got %d", rr.Code)
		}
	})

	t.Run("default_limit_and_total", func(t *testing.T) {
		body, code := getAuditJSON(t, h, "")
		if code != http.StatusOK {
			t.Fatalf("status %d", code)
		}
		if body.Total != 32 {
			t.Fatalf("total: want 32, got %d", body.Total)
		}
		if len(body.Entries) != 32 {
			t.Fatalf("entries len: want 32, got %d", len(body.Entries))
		}
		if body.Entries[0].CreatedAt != 3000 {
			t.Fatalf("newest created_at: want 3000, got %d", body.Entries[0].CreatedAt)
		}
		assertDescCreatedAt(t, body.Entries)
	})

	t.Run("limit_10_offset_0_first_page", func(t *testing.T) {
		body, code := getAuditJSON(t, h, "limit=10&offset=0")
		if code != http.StatusOK {
			t.Fatalf("status %d", code)
		}
		if body.Total != 32 {
			t.Fatalf("total: want 32, got %d", body.Total)
		}
		if len(body.Entries) != 10 {
			t.Fatalf("entries: want 10, got %d", len(body.Entries))
		}
		if body.Entries[0].CreatedAt != 3000 || body.Entries[9].CreatedAt != 2991 {
			t.Fatalf("range: got first=%d last=%d", body.Entries[0].CreatedAt, body.Entries[9].CreatedAt)
		}
		assertDescCreatedAt(t, body.Entries)
	})

	t.Run("limit_10_offset_10_second_page", func(t *testing.T) {
		body, code := getAuditJSON(t, h, "limit=10&offset=10")
		if code != http.StatusOK {
			t.Fatalf("status %d", code)
		}
		if body.Total != 32 {
			t.Fatalf("total: want 32, got %d", body.Total)
		}
		if len(body.Entries) != 10 {
			t.Fatalf("entries: want 10, got %d", len(body.Entries))
		}
		if body.Entries[0].CreatedAt != 2990 || body.Entries[9].CreatedAt != 2981 {
			t.Fatalf("range: got first=%d last=%d", body.Entries[0].CreatedAt, body.Entries[9].CreatedAt)
		}
		assertDescCreatedAt(t, body.Entries)
	})

	t.Run("limit_10_offset_30_last_partial_page", func(t *testing.T) {
		body, code := getAuditJSON(t, h, "limit=10&offset=30")
		if code != http.StatusOK {
			t.Fatalf("status %d", code)
		}
		if body.Total != 32 {
			t.Fatalf("total: want 32, got %d", body.Total)
		}
		if len(body.Entries) != 2 {
			t.Fatalf("entries: want 2, got %d", len(body.Entries))
		}
		if body.Entries[0].CreatedAt != 2970 || body.Entries[1].CreatedAt != 2969 {
			t.Fatalf("last page: got %+v", body.Entries)
		}
	})

	t.Run("large_limit_no_server_cap", func(t *testing.T) {
		body, code := getAuditJSON(t, h, "limit=2000&offset=0")
		if code != http.StatusOK {
			t.Fatalf("status %d", code)
		}
		if body.Total != 32 || len(body.Entries) != 32 {
			t.Fatalf("want 32/32, got total=%d len=%d", body.Total, len(body.Entries))
		}
	})

	t.Run("invalid_limit_zero_uses_default", func(t *testing.T) {
		body, code := getAuditJSON(t, h, "limit=0")
		if code != http.StatusOK {
			t.Fatalf("status %d", code)
		}
		if body.Total != 32 || len(body.Entries) != 32 {
			t.Fatalf("default limit 50 should return all 32 rows: total=%d len=%d", body.Total, len(body.Entries))
		}
	})

	t.Run("negative_offset_ignored_same_as_first_page", func(t *testing.T) {
		a, _ := getAuditJSON(t, h, "limit=5&offset=0")
		b, code := getAuditJSON(t, h, "limit=5&offset=-3")
		if code != http.StatusOK {
			t.Fatalf("status %d", code)
		}
		if b.Total != 32 || len(b.Entries) != 5 {
			t.Fatalf("want total 32 len 5, got %d %d", b.Total, len(b.Entries))
		}
		for i := range a.Entries {
			if i >= len(b.Entries) {
				break
			}
			if a.Entries[i].Detail != b.Entries[i].Detail {
				t.Fatalf("row %d mismatch negative offset handling", i)
			}
		}
	})

	t.Run("action_filter_total_and_rows", func(t *testing.T) {
		body, code := getAuditJSON(t, h, "action=onlyme&limit=100&offset=0")
		if code != http.StatusOK {
			t.Fatalf("status %d", code)
		}
		if body.Total != 5 || len(body.Entries) != 5 {
			t.Fatalf("onlyme: want total 5 len 5, got %d %d", body.Total, len(body.Entries))
		}
		for _, e := range body.Entries {
			if e.Action != "onlyme" {
				t.Fatalf("action leak: %q", e.Action)
			}
		}
	})

	t.Run("since_until_filter", func(t *testing.T) {
		// created_at in [2980, 2990] inclusive -> i from 10..20 => 11 rows
		body, code := getAuditJSON(t, h, "since=2980&until=2990&limit=50&offset=0")
		if code != http.StatusOK {
			t.Fatalf("status %d", code)
		}
		if body.Total != 11 || len(body.Entries) != 11 {
			t.Fatalf("time filter: want 11/11, got total=%d len=%d", body.Total, len(body.Entries))
		}
		for _, e := range body.Entries {
			if e.CreatedAt < 2980 || e.CreatedAt > 2990 {
				t.Fatalf("row outside range: %+v", e)
			}
		}
	})

	t.Run("pubkey_filter", func(t *testing.T) {
		body, code := getAuditJSON(t, h, "pubkey="+pkRare+"&limit=50&offset=0")
		if code != http.StatusOK {
			t.Fatalf("status %d", code)
		}
		if body.Total != 3 || len(body.Entries) != 3 {
			t.Fatalf("pubkey: want 3/3, got total=%d len=%d", body.Total, len(body.Entries))
		}
		for _, e := range body.Entries {
			if e.Pubkey != pkRare {
				t.Fatalf("pubkey leak")
			}
		}
	})

	t.Run("kind_filter_suffix_match", func(t *testing.T) {
		// kind=0 for indices i where i%5==0: 0,5,10,15,20,25,30 => 7 rows
		body, code := getAuditJSON(t, h, "kind=0&limit=100&offset=0")
		if code != http.StatusOK {
			t.Fatalf("status %d", code)
		}
		if body.Total != 7 || len(body.Entries) != 7 {
			t.Fatalf("kind=0: want total 7 len 7, got %d %d", body.Total, len(body.Entries))
		}
		for _, e := range body.Entries {
			if !strings.HasSuffix(e.Detail, "kind=0") {
				t.Fatalf("detail should end with kind=0: %q", e.Detail)
			}
		}
	})

	t.Run("offset_beyond_total_returns_empty_entries_same_total", func(t *testing.T) {
		body, code := getAuditJSON(t, h, "limit=10&offset=100")
		if code != http.StatusOK {
			t.Fatalf("status %d", code)
		}
		if body.Total != 32 {
			t.Fatalf("total: want 32, got %d", body.Total)
		}
		if len(body.Entries) != 0 {
			t.Fatalf("want empty page, got %d rows", len(body.Entries))
		}
	})
}
