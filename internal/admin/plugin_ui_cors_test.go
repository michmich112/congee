package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWritePluginUIAccessHeadersNullOrigin(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/plugin-ui/conduit/assets/app.js", nil)
	req.Header.Set("Origin", "null")
	writePluginUIAccessHeaders(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "null" {
		t.Fatalf("ACAO=%q", got)
	}
	if rec.Header().Get("Cross-Origin-Resource-Policy") != "cross-origin" {
		t.Fatal("missing CORP")
	}
}

func TestWritePluginUIAccessHeadersMissingOrigin(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/plugin-ui/conduit/", nil)
	writePluginUIAccessHeaders(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("ACAO=%q", got)
	}
}
