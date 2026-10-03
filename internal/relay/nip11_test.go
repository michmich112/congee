package relay

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/relayidentity"
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
	request.Header.Set("X-Forwarded-Proto", "https://evil.example")
	if doc := read(); doc["icon"] != "http://example.com/assets/icon" {
		t.Fatalf("non-http forwarded proto should be ignored: %v", doc["icon"])
	}
	request.Header.Set("X-Forwarded-Proto", "https")
	doc := read()
	if doc["icon"] != "https://example.com/assets/icon" || doc["banner"] != "https://example.com/assets/banner" {
		t.Fatalf("default images should point at hosted routes: icon=%v banner=%v", doc["icon"], doc["banner"])
	}
	cfg.NIP11.IconSource = config.NIP11ImageSourceURL
	cfg.NIP11.BannerSource = config.NIP11ImageSourceURL
	cfg.NIP11.Icon = "https://images.example/icon.png"
	cfg.NIP11.Banner = "https://images.example/banner.jpg"
	cfg.NIP11.Contact = "   "
	doc = read()
	if doc["icon"] != cfg.NIP11.Icon || doc["banner"] != cfg.NIP11.Banner {
		t.Fatalf("external images missing from NIP-11: %v", doc)
	}
	if _, ok := doc["contact"]; ok {
		t.Fatal("whitespace contact should be omitted")
	}
}

func TestNIP11IdentityAndEnforcedLimitations(t *testing.T) {
	id, err := relayidentity.Load(filepath.Join(t.TempDir(), "relay.secrets.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig()
	cfg.NIP11.AdminPubKey = strings.Repeat("a", 64)
	cfg.WebSocket.MaxMessageBytes = 2048
	cfg.ConnectionLimits.MaxSubscriptionsPerConnection = 7
	cfg.MaxSubscriptionIDLength = 64
	cfg.ConnectionLimits.MaxFiltersPerReq = 3
	cfg.NIPs.Enabled = []int{1, 11, 42}
	cfg.NIP42.RequireAuthSubscribeKinds = []int{1059}
	cfg.NIP42.RequireAuthPublishKinds = []int{21059}
	h := &NIP11Handler{Cfg: cfg, RelayID: id}
	read := func() map[string]any {
		t.Helper()
		response := httptest.NewRecorder()
		h.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
		var doc map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &doc); err != nil {
			t.Fatal(err)
		}
		return doc
	}
	doc := read()
	if doc["self"] != id.PubKeyHex() || doc["pubkey"] != cfg.NIP11.AdminPubKey {
		t.Fatalf("relay and administrator identities confused: self=%v pubkey=%v", doc["self"], doc["pubkey"])
	}
	limits, ok := doc["limitation"].(map[string]any)
	if !ok || limits["max_message_length"] != float64(2048) ||
		limits["max_subscriptions"] != float64(7) || limits["max_subid_length"] != float64(64) ||
		limits["max_filters"] != float64(3) ||
		limits["default_limit"] != float64(500) || limits["auth_required"] != false {
		t.Fatalf("incorrect advertised limits: %v", doc["limitation"])
	}
	if _, ok := limits["max_limit"]; ok {
		t.Fatal("unimplemented explicit limit cap advertised")
	}
	if _, ok := limits["max_event_tags"]; ok {
		t.Fatal("unimplemented tag cap advertised")
	}
	cfg.NIP11.AdminPubKey = ""
	noDefault := 0
	cfg.ConnectionLimits.DefaultQueryLimit = &noDefault
	doc = read()
	if _, ok := doc["pubkey"]; ok {
		t.Fatal("unset administrator contact should be omitted")
	}
	limits = doc["limitation"].(map[string]any)
	if _, ok := limits["default_limit"]; ok {
		t.Fatal("disabled default cap should be omitted")
	}
	cfg.NIP42.RequireAuth = config.NIP42RequireAuthConnect
	doc = read()
	limits = doc["limitation"].(map[string]any)
	if limits["auth_required"] != true {
		t.Fatalf("connect mode should advertise auth_required: %v", limits)
	}
	cfg.NIPs.Enabled = []int{1, 11}
	doc = read()
	limits = doc["limitation"].(map[string]any)
	if limits["auth_required"] != false {
		t.Fatal("connect mode must not lock the relay when NIP-42 is disabled")
	}
}
