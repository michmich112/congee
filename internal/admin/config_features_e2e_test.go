package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/db"
	"github.com/rs/zerolog"
)

// End-to-end admin API: the production mux accepts relay alias edits, rejects
// invalid URLs, and hides storage size until database.analyze is loaded on restart.
func TestE2EAdminRelayAliasesAndDatabaseAnalysis(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "events.db")
	cfgPath := filepath.Join(dir, "config.json")

	cfg := config.DefaultConfig()
	cfg.Database.DSN = dbPath
	cfg.Database.MetaDSN = filepath.Join(dir, "congee-meta.db")
	cfg.Database.Analyze = false
	if err := config.WriteConfigAtomic(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}

	st, closeStore, err := db.OpenTestStore(ctx, dbPath, zerolog.Nop())
	if err != nil && strings.Contains(err.Error(), "not available") {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()

	const password = "e2e-admin"
	running := config.DefaultConfig()
	*running = *cfg
	srv := NewServer(running, cfgPath, st, nil, zerolog.Nop(), password, dir, nil, nil, config.RelayInstanceResolution{}, nil)
	ts := httptest.NewServer(srv.http.Handler)
	defer ts.Close()

	client := ts.Client()
	do := func(method, path string, body []byte, auth bool) (int, []byte) {
		t.Helper()
		var rdr io.Reader
		if body != nil {
			rdr = bytes.NewReader(body)
		}
		req, err := http.NewRequest(method, ts.URL+path, rdr)
		if err != nil {
			t.Fatal(err)
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if auth {
			req.Header.Set("Authorization", "Bearer "+password)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, raw
	}

	code, raw := do(http.MethodGet, "/api/stats", nil, false)
	if code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated stats: %d %s", code, raw)
	}

	code, raw = do(http.MethodGet, "/api/stats", nil, true)
	if code != http.StatusOK {
		t.Fatalf("stats: %d %s", code, raw)
	}
	var stats map[string]any
	if err := json.Unmarshal(raw, &stats); err != nil {
		t.Fatal(err)
	}
	storage, _ := stats["storage"].(map[string]any)
	if storage["analysis_enabled"] != false {
		t.Fatalf("expected analysis off, got %#v", storage)
	}
	if _, ok := storage["bytes"]; ok {
		t.Fatalf("size leaked while analysis is off: %#v", storage)
	}

	file, err := osReadConfig(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	dbSec := file["database"].(map[string]any)
	dbSec["analyze"] = true
	nip42 := file["nip42"].(map[string]any)
	nip42["relay_url"] = "wss://Relay.EXAMPLE/"
	nip42["relay_aliases"] = []any{"wss://Alias.EXAMPLE/path/"}
	body, err := json.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	code, raw = do(http.MethodPut, "/api/config", body, true)
	if code != http.StatusOK {
		t.Fatalf("put config: %d %s", code, raw)
	}

	saved, err := osReadConfig(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if saved["database"].(map[string]any)["analyze"] != true {
		t.Fatalf("analyze not saved: %#v", saved["database"])
	}
	savedNIP := saved["nip42"].(map[string]any)
	if savedNIP["relay_url"] != "wss://relay.example/" {
		t.Fatalf("canonical URL: %#v", savedNIP["relay_url"])
	}
	aliases, _ := savedNIP["relay_aliases"].([]any)
	if len(aliases) != 1 || aliases[0] != "wss://alias.example/path" {
		t.Fatalf("aliases: %#v", savedNIP["relay_aliases"])
	}

	savedNIP["relay_aliases"] = []any{"https://not-websocket.example/"}
	body, err = json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	code, raw = do(http.MethodPut, "/api/config", body, true)
	if code != http.StatusBadRequest {
		t.Fatalf("invalid alias status %d body %s", code, raw)
	}
	if !strings.Contains(string(raw), "relay_aliases") {
		t.Fatalf("expected alias validation error, got %s", raw)
	}

	code, raw = do(http.MethodGet, "/api/stats", nil, true)
	if code != http.StatusOK {
		t.Fatalf("stats after save: %d %s", code, raw)
	}
	if err := json.Unmarshal(raw, &stats); err != nil {
		t.Fatal(err)
	}
	storage, _ = stats["storage"].(map[string]any)
	if storage["analysis_enabled"] != false {
		t.Fatalf("running process should keep analysis off until restart: %#v", storage)
	}

	restarted, err := config.LoadJSON(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !restarted.Database.Analyze {
		t.Fatal("reloaded config did not enable analysis")
	}
	restartSrv := NewServer(restarted, cfgPath, st, nil, zerolog.Nop(), password, dir, nil, nil, config.RelayInstanceResolution{}, nil)
	restartTS := httptest.NewServer(restartSrv.http.Handler)
	defer restartTS.Close()
	req, err := http.NewRequest(http.MethodGet, restartTS.URL+"/api/stats", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+password)
	resp, err := restartTS.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err = io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("restarted stats: %d %s", resp.StatusCode, raw)
	}
	if err := json.Unmarshal(raw, &stats); err != nil {
		t.Fatal(err)
	}
	storage, _ = stats["storage"].(map[string]any)
	if storage["analysis_enabled"] != true {
		t.Fatalf("analysis after restart: %#v", storage)
	}
	for _, k := range []string{"bytes", "meta_bytes", "events", "tags", "audit"} {
		if _, ok := storage[k]; !ok {
			t.Errorf("storage missing %q", k)
		}
	}
}

func osReadConfig(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	return doc, nil
}
