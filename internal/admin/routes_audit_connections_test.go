package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/storage"
	"github.com/michmich112/congee/internal/storage/sqlite"
	"github.com/rs/zerolog"
)

func TestAuditConnectionsList_HTTP(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := sqlite.Open(ctx, filepath.Join(dir, "conn-audit.db"), nil, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()

	if _, err := st.SaveWSConnectionSession(ctx, storage.WSConnectionSession{
		ConnID:           "deadbeef",
		PeerIP:           "10.0.0.1",
		RemoteAddr:       "10.0.0.1:1234",
		StartedUnix:      1000,
		EndedUnix:        2000,
		TotalReq:         3,
		TotalClientEvent: 1,
		SeriesJSON:       []byte(`[{"t":1000,"req":0,"ev":0},{"t":2000,"req":3,"ev":1}]`),
		SubsJSON:         []byte(`[]`),
	}); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	cfg.Audit.RetentionDays = 14

	api := http.NewServeMux()
	api.HandleFunc("GET /audit/connections", HandleAuditConnectionsList(cfg, nil, st))
	h := RequireAdminAuth(auditHTTPTestPassword, http.StripPrefix("/api", api))

	req := httptest.NewRequest(http.MethodGet, "/api/audit/connections", nil)
	req.Header.Set("Authorization", "Bearer "+auditHTTPTestPassword)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		RetentionDays int             `json:"retention_days"`
		Live          json.RawMessage `json:"live"`
		Closed        []struct {
			Ref    string `json:"ref"`
			ConnID string `json:"conn_id"`
		} `json:"closed"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.RetentionDays != 14 {
		t.Fatalf("retention_days: got %d", body.RetentionDays)
	}
	if len(body.Closed) != 1 || body.Closed[0].ConnID != "deadbeef" {
		t.Fatalf("closed: %+v", body.Closed)
	}
}

func TestAuditConnectionsDetail_HTTP(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := sqlite.Open(ctx, filepath.Join(dir, "conn-audit2.db"), nil, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()

	id, err := st.SaveWSConnectionSession(ctx, storage.WSConnectionSession{
		ConnID:           "abc12345",
		PeerIP:           "192.168.1.2",
		RemoteAddr:       "192.168.1.2:99",
		StartedUnix:      500,
		EndedUnix:        600,
		TotalReq:         1,
		TotalClientEvent: 2,
		SeriesJSON:       []byte(`[]`),
		SubsJSON:         []byte(`[{"sub_id":"sub1","opened_unix":500,"filter_count":1,"kinds":[1],"initial_events_sent":0,"initial_events_dropped":0,"broadcast_events_enqueued":0,"broadcast_events_dropped":0,"eose_sent":1}]`),
	})
	if err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	api := http.NewServeMux()
	api.HandleFunc("GET /audit/connections/{ref}", HandleAuditConnectionsDetail(cfg, nil, st))
	h := RequireAdminAuth(auditHTTPTestPassword, http.StripPrefix("/api", api))

	path := "/api/audit/connections/session:" + strconv.FormatInt(id, 10)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+auditHTTPTestPassword)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
	var detail struct {
		Kind                string          `json:"kind"`
		ConnID              string          `json:"conn_id"`
		Subscriptions       int             `json:"subscriptions"`
		SubscriptionDetails json.RawMessage `json:"subscription_details"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Kind != "session" || detail.ConnID != "abc12345" || detail.Subscriptions != 1 {
		t.Fatalf("detail: %+v", detail)
	}
}
