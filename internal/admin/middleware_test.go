package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireAdminAuth_Bearer(t *testing.T) {
	h := RequireAdminAuth("secret", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("got %d", rr.Code)
	}
}

func TestRequireAdminAuth_XAdminToken(t *testing.T) {
	h := RequireAdminAuth("tok", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Admin-Token", "tok")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("got %d", rr.Code)
	}
}

func TestRequireAdminAuth_RejectWrong(t *testing.T) {
	h := RequireAdminAuth("secret", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("next called")
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("got %d", rr.Code)
	}
}

func TestRequireAdminAuth_EmptyPassword(t *testing.T) {
	h := RequireAdminAuth("", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("next called")
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer x")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("got %d", rr.Code)
	}
}

func TestRequireAdminAuth_MissingCredentials(t *testing.T) {
	h := RequireAdminAuth("secret", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("next called")
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("got %d want 401", rr.Code)
	}
}
