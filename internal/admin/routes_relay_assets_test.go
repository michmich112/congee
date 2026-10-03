package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/db"
	"github.com/rs/zerolog"
)

func TestRelayAssetUploadRequiresAuthAndStoresFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := config.WriteConfigAtomic(cfgPath, config.DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	st, closeStore, err := db.OpenTestStore(context.Background(), filepath.Join(dir, "meta.db"), zerolog.Nop())
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()

	var mu sync.Mutex
	post := handlePostRelayAsset(cfgPath, &mu, st, zerolog.Nop(), nil, config.NIP11AssetIcon)
	png := onePixelPNG()
	wrapped := RequireAdminAuth("secret", post)
	denied := postRelayAsset(t, wrapped, png, "icon.png", "")
	if denied.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status %d", denied.Code)
	}

	ok := postRelayAsset(t, wrapped, png, "icon.png", "secret")
	if ok.Code != http.StatusOK {
		t.Fatalf("upload status %d body %s", ok.Code, ok.Body.String())
	}
	loaded, err := config.LoadJSON(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.NIP11.IconSource != config.NIP11ImageSourceUpload || loaded.NIP11.Icon != "" {
		t.Fatalf("config after upload: %+v", loaded.NIP11)
	}
	stored, err := os.ReadFile(filepath.Join(config.NIP11AssetDir(cfgPath), config.NIP11AssetIcon))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, png) {
		t.Fatal("stored bytes differ")
	}

	rejected := postRelayAsset(t, wrapped, []byte("<svg xmlns='http://www.w3.org/2000/svg'></svg>"), "icon.svg", "secret")
	if rejected.Code != http.StatusBadRequest {
		t.Fatalf("svg status %d body %s", rejected.Code, rejected.Body.String())
	}
	again, err := os.ReadFile(filepath.Join(config.NIP11AssetDir(cfgPath), config.NIP11AssetIcon))
	if err != nil || !bytes.Equal(again, png) {
		t.Fatal("rejected upload changed the stored file")
	}

	loaded.NIP11.IconSource = config.NIP11ImageSourceDefault
	body, err := json.Marshal(loaded)
	if err != nil {
		t.Fatal(err)
	}
	putReq := httptest.NewRequest(http.MethodPut, "/config", bytes.NewReader(body))
	putRec := httptest.NewRecorder()
	handlePutConfig(cfgPath, &mu, st, zerolog.Nop(), nil).ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("put status %d body %s", putRec.Code, putRec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(config.NIP11AssetDir(cfgPath), config.NIP11AssetIcon)); !os.IsNotExist(err) {
		t.Fatalf("default source should delete the upload, stat err=%v", err)
	}
}

func postRelayAsset(t *testing.T, h http.Handler, data []byte, filename, token string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/relay-assets/icon", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func onePixelPNG() []byte {
	return []byte{
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
}
