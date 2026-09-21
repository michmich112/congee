package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/plugin"
	"github.com/rs/zerolog"
)

const interceptLogHTTPPassword = "intercept-log-http-secret"

func interceptLogHTTP(t *testing.T) (*plugin.Manager, http.Handler, string, func()) {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	cfg := config.DefaultConfig()
	cfg.Plugins.Directory = filepath.Join(dir, "plugins")
	cfg.Plugins.Items = []config.PluginItem{{ID: "fixture", Enabled: false}}
	if err := config.WriteConfigAtomic(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	m := plugin.NewManager(cfg, cfgPath, nil, zerolog.Nop())
	m.StartInterceptLogWorker(ctx)
	s := &Server{cfg: cfg, cfgPath: cfgPath, plugins: m, password: interceptLogHTTPPassword}
	api := http.NewServeMux()
	registerPluginRoutes(api, s)
	h := RequireAdminAuth(interceptLogHTTPPassword, http.StripPrefix("/api", api))
	return m, h, cfgPath, func() {
		m.Stop()
		cancel()
	}
}

func interceptLogReq(t *testing.T, h http.Handler, method, path, token string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != nil {
		r = httptest.NewRequest(method, path, bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	return rr
}

func TestHandlePluginInterceptLogAuthAnd404(t *testing.T) {
	_, h, _, stop := interceptLogHTTP(t)
	defer stop()

	rr := interceptLogReq(t, h, http.MethodGet, "/api/plugins/fixture/intercept-log", "", nil)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("no auth %d", rr.Code)
	}
	rr = interceptLogReq(t, h, http.MethodGet, "/api/plugins/missing/intercept-log", interceptLogHTTPPassword, nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("missing %d body=%s", rr.Code, rr.Body.String())
	}
	rr = interceptLogReq(t, h, http.MethodPut, "/api/plugins/missing/intercept-log", interceptLogHTTPPassword, []byte(`{"limit":10}`))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("put missing %d", rr.Code)
	}
	rr = interceptLogReq(t, h, http.MethodPut, "/api/plugins/fixture/intercept-log", interceptLogHTTPPassword, []byte(`{}`))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("put empty %d", rr.Code)
	}
}

func TestHandlePluginInterceptLogGetPutTrim(t *testing.T) {
	m, h, cfgPath, stop := interceptLogHTTP(t)
	defer stop()

	rr := interceptLogReq(t, h, http.MethodGet, "/api/plugins/fixture/intercept-log", interceptLogHTTPPassword, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("get %d %s", rr.Code, rr.Body.String())
	}
	var empty plugin.InterceptLogSnapshot
	if err := json.Unmarshal(rr.Body.Bytes(), &empty); err != nil {
		t.Fatal(err)
	}
	if empty.Limit != config.DefaultPluginInterceptLogSize || len(empty.Entries) != 0 {
		t.Fatalf("empty %+v", empty)
	}

	for i := 0; i < 3; i++ {
		m.RecordInterceptLog("fixture", &nostr.ReqMessage{
			SubID:   fmt.Sprintf("s%d", i),
			Filters: []nostr.Filter{{Kinds: []int{30402}}},
		}, plugin.InterceptResult{Action: plugin.InterceptRespond, EventIDs: []string{fmt.Sprintf("e%d", i)}}, "")
	}
	deadline := time.Now().Add(2 * time.Second)
	var filled plugin.InterceptLogSnapshot
	for time.Now().Before(deadline) {
		rr = interceptLogReq(t, h, http.MethodGet, "/api/plugins/fixture/intercept-log", interceptLogHTTPPassword, nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("get filled %d %s", rr.Code, rr.Body.String())
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &filled); err != nil {
			t.Fatal(err)
		}
		if len(filled.Entries) == 3 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if len(filled.Entries) != 3 || filled.Entries[0].SubID != "s2" || filled.Entries[2].SubID != "s0" {
		t.Fatalf("filled %+v", filled.Entries)
	}
	if filled.Entries[0].Action != "respond" || filled.Entries[0].EventIDs[0] != "e2" {
		t.Fatalf("newest %+v", filled.Entries[0])
	}

	rr = interceptLogReq(t, h, http.MethodPut, "/api/plugins/fixture/intercept-log", interceptLogHTTPPassword, []byte(`{"limit":2}`))
	if rr.Code != http.StatusOK {
		t.Fatalf("put %d %s", rr.Code, rr.Body.String())
	}
	var put struct {
		OK    bool `json:"ok"`
		Limit int  `json:"limit"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &put); err != nil {
		t.Fatal(err)
	}
	if !put.OK || put.Limit != 2 {
		t.Fatalf("put body %+v", put)
	}

	rr = interceptLogReq(t, h, http.MethodGet, "/api/plugins/fixture/intercept-log", interceptLogHTTPPassword, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("get trimmed %d %s", rr.Code, rr.Body.String())
	}
	var trimmed plugin.InterceptLogSnapshot
	if err := json.Unmarshal(rr.Body.Bytes(), &trimmed); err != nil {
		t.Fatal(err)
	}
	if trimmed.Limit != 2 || len(trimmed.Entries) != 2 || trimmed.Entries[0].SubID != "s2" || trimmed.Entries[1].SubID != "s1" {
		t.Fatalf("trimmed %+v", trimmed)
	}

	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	var onDisk config.Config
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatal(err)
	}
	if onDisk.Plugins.InterceptLogSize == nil || *onDisk.Plugins.InterceptLogSize != 2 {
		t.Fatalf("persist intercept_log_size %+v", onDisk.Plugins.InterceptLogSize)
	}
}

func TestHandlePluginInterceptLogNilPlugins(t *testing.T) {
	s := &Server{password: interceptLogHTTPPassword}
	api := http.NewServeMux()
	registerPluginRoutes(api, s)
	h := RequireAdminAuth(interceptLogHTTPPassword, http.StripPrefix("/api", api))
	rr := interceptLogReq(t, h, http.MethodGet, "/api/plugins/fixture/intercept-log", interceptLogHTTPPassword, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("get nil plugins %d", rr.Code)
	}
	rr = interceptLogReq(t, h, http.MethodPut, "/api/plugins/fixture/intercept-log", interceptLogHTTPPassword, []byte(`{"limit":10}`))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("put nil plugins %d", rr.Code)
	}
}
