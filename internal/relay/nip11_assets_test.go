package relay

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/michmich112/congee/internal/config"
	"github.com/rs/zerolog"
)

func TestNIP11HostedAssets(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	cfg := config.DefaultConfig()
	cfg.NIP11.CORSAllowAnyOrigin = true
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	s := &Server{cfg: cfg, log: zerolog.Nop(), configPath: cfgPath}
	mux := s.routes()

	get := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec
	}

	icon := get("/assets/icon")
	if icon.Code != http.StatusOK || !bytes.Equal(icon.Body.Bytes(), defaultIconSVG) {
		t.Fatalf("default icon status %d len %d", icon.Code, icon.Body.Len())
	}
	if icon.Header().Get("Content-Type") != "image/svg+xml" || icon.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("default icon headers: %v", icon.Header())
	}
	if icon.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("hosted image missing NIP-11 CORS header")
	}
	banner := get("/assets/banner")
	if banner.Code != http.StatusOK || !bytes.Equal(banner.Body.Bytes(), defaultBannerSVG) {
		t.Fatalf("default banner status %d", banner.Code)
	}

	h := &NIP11Handler{Cfg: cfg}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://relay.example/", nil))
	var doc map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc["icon"] != "http://relay.example/assets/icon" || doc["banner"] != "http://relay.example/assets/banner" {
		t.Fatalf("default document URLs: icon=%v banner=%v", doc["icon"], doc["banner"])
	}

	png := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xde, 0x00, 0x00, 0x00, 0x0c, 0x49, 0x44, 0x41,
		0x54, 0x08, 0xd7, 0x63, 0xf8, 0xcf, 0xc0, 0x00,
		0x00, 0x00, 0x03, 0x00, 0x01, 0x00, 0x05, 0xfe,
		0x02, 0xfe, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45,
		0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
	}
	if err := config.WriteNIP11Asset(cfgPath, config.NIP11AssetIcon, png); err != nil {
		t.Fatal(err)
	}
	cfg.NIP11.IconSource = config.NIP11ImageSourceUpload
	uploaded := get("/assets/icon")
	if uploaded.Code != http.StatusOK || !bytes.Equal(uploaded.Body.Bytes(), png) {
		t.Fatalf("uploaded icon status %d type %s", uploaded.Code, uploaded.Header().Get("Content-Type"))
	}
	if uploaded.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("uploaded content type %s", uploaded.Header().Get("Content-Type"))
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://relay.example/", nil))
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc["icon"] != "http://relay.example/assets/icon" {
		t.Fatalf("upload document icon: %v", doc["icon"])
	}

	if err := os.Remove(filepath.Join(config.NIP11AssetDir(cfgPath), config.NIP11AssetIcon)); err != nil {
		t.Fatal(err)
	}
	missing := get("/assets/icon")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing upload status %d", missing.Code)
	}

	over := bytes.Repeat([]byte{0x89}, config.NIP11IconMaxBytes+1)
	if err := os.WriteFile(filepath.Join(config.NIP11AssetDir(cfgPath), config.NIP11AssetIcon), over, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg.NIP11.IconSource = config.NIP11ImageSourceUpload
	if tooBig := get("/assets/icon"); tooBig.Code != http.StatusNotFound {
		t.Fatalf("oversized upload status %d", tooBig.Code)
	}
	if err := os.Remove(filepath.Join(config.NIP11AssetDir(cfgPath), config.NIP11AssetIcon)); err != nil {
		t.Fatal(err)
	}

	cfg.NIP11.IconSource = config.NIP11ImageSourceURL
	cfg.NIP11.Icon = "https://images.example/icon.png"
	cfg.NIP11.BannerSource = config.NIP11ImageSourceURL
	cfg.NIP11.Banner = "https://images.example/banner.jpg"
	if urlMode := get("/assets/icon"); urlMode.Code != http.StatusNotFound {
		t.Fatalf("url mode icon status %d", urlMode.Code)
	}
	if urlMode := get("/assets/banner"); urlMode.Code != http.StatusNotFound {
		t.Fatalf("url mode banner status %d", urlMode.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://relay.example/", nil))
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc["icon"] != "https://images.example/icon.png" || doc["banner"] != "https://images.example/banner.jpg" {
		t.Fatalf("external document URLs: %v %v", doc["icon"], doc["banner"])
	}
}
