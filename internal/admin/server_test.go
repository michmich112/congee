package admin

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestServeAdminStatic_GET_root(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!DOCTYPE html><html></html>\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	spa := spaFileSystem{dir: http.Dir(dir)}
	h := serveAdminStatic(dir, http.FileServer(spa))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /: want status %d, got %d body=%q", http.StatusOK, rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if body != "<!DOCTYPE html><html></html>\n" {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestServeAdminStatic_HEAD(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!DOCTYPE html>\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	spa := spaFileSystem{dir: http.Dir(dir)}
	h := serveAdminStatic(dir, http.FileServer(spa))

	req := httptest.NewRequest(http.MethodHead, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("HEAD /: want status %d, got %d", http.StatusOK, rr.Code)
	}
}

func TestServeAdminStatic_clientRouteUsesSPAIndex(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!DOCTYPE html><title>spa</title>\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	spa := spaFileSystem{dir: http.Dir(dir)}
	h := serveAdminStatic(dir, http.FileServer(spa))

	req := httptest.NewRequest(http.MethodGet, "/settings/deep", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /settings/deep: want status %d, got %d body=%q", http.StatusOK, rr.Code, rr.Body.String())
	}
	if got := rr.Body.String(); got != "<!DOCTYPE html><title>spa</title>\n" {
		t.Fatalf("unexpected body: %q", got)
	}
}
