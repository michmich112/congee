package relay

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/michmich112/congee/internal/config"
)

func TestNIP11OptionalImages(t *testing.T) {
	cfg := config.DefaultConfig()
	h := &NIP11Handler{Cfg: cfg}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Accept", "application/nostr+json")
	read := func() map[string]any {
		t.Helper()
		response := httptest.NewRecorder()
		h.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status %d", response.Code)
		}
		var doc map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &doc); err != nil {
			t.Fatal(err)
		}
		return doc
	}
	if doc := read(); doc["icon"] != nil || doc["banner"] != nil {
		t.Fatalf("unset image fields should be omitted: %v", doc)
	}
	cfg.NIP11.Icon = "https://images.example/icon.png"
	cfg.NIP11.Banner = "https://images.example/banner.jpg"
	if doc := read(); doc["icon"] != cfg.NIP11.Icon || doc["banner"] != cfg.NIP11.Banner {
		t.Fatalf("configured images missing from NIP-11: %v", doc)
	}
}
