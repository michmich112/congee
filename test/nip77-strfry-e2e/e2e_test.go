//go:build e2e

package nip77strfry_test

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/gorilla/websocket"
	"github.com/michmich112/congee/internal/nostr"
)

const (
	strfryImage = "ghcr.io/hoytech/strfry:latest"
	seedCount   = 1100
	adminPass   = "e2e-admin"
)

func TestCongeeSyncsFromStrfry(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not installed")
	}
	bin := filepath.Join(moduleRoot(t), "bin", "congee")
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("missing %s (run make build)", bin)
	}

	ensureStrfryImage(t)

	tmp, err := os.MkdirTemp("/tmp", "c77-")
	if err != nil {
		t.Fatal(err)
	}
	strfryName := fmt.Sprintf("congee-nip77-e2e-%d", time.Now().UnixNano())
	strfryPort := freePort(t)
	relayPort := freePort(t)
	adminPort := freePort(t)

	var congee *exec.Cmd
	t.Cleanup(func() {
		if congee != nil && congee.Process != nil {
			_ = congee.Process.Kill()
			_, _ = congee.Process.Wait()
		}
		out, _ := exec.Command("docker", "rm", "-f", strfryName).CombinedOutput()
		if t.Failed() && len(out) > 0 {
			t.Logf("docker rm: %s", out)
		}
		_ = os.RemoveAll(tmp)
	})

	confPath := filepath.Join(tmp, "strfry.conf")
	if err := os.WriteFile(confPath, []byte(strfryConf), 0o644); err != nil {
		t.Fatal(err)
	}
	run := exec.Command("docker", "run", "-d", "--name", strfryName,
		"-p", fmt.Sprintf("127.0.0.1:%d:7777", strfryPort),
		"-v", confPath+":/app/strfry.conf:ro",
		strfryImage,
	)
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("docker run: %v\n%s", err, out)
	}
	strfryURL := fmt.Sprintf("ws://127.0.0.1:%d/", strfryPort)
	waitWebSocket(t, strfryURL, 60*time.Second)

	priv, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	jsonl, wantIDs := seedJSONL(t, priv, seedCount)
	importCmd := exec.Command("docker", "exec", "-i", "-w", "/app", strfryName, "./strfry", "import")
	importCmd.Stdin = bytes.NewReader(jsonl)
	if out, err := importCmd.CombinedOutput(); err != nil {
		t.Fatalf("strfry import: %v\n%s", err, out)
	}

	cfgPath := writeCongeeConfig(t, tmp, relayPort, adminPort, strfryURL)
	var logBuf safeBuf
	congee = exec.Command(bin)
	congee.Dir = tmp
	congee.Env = congeeEnv(cfgPath, tmp)
	congee.Stdout = &logBuf
	congee.Stderr = &logBuf
	if err := congee.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if t.Failed() {
			logs, _ := exec.Command("docker", "logs", strfryName).CombinedOutput()
			t.Logf("congee log:\n%s", tail(logBuf.String(), 8000))
			t.Logf("strfry log:\n%s", tail(string(logs), 4000))
		}
	})

	waitHTTP(t, fmt.Sprintf("http://127.0.0.1:%d/health", relayPort), 30*time.Second)
	waitSyncComplete(t, &logBuf, 8*time.Minute)

	imported, failures := upstreamStats(t, adminPort)
	if failures != 0 {
		t.Fatalf("neg_upstream_failures_total=%d", failures)
	}
	if imported < seedCount {
		t.Fatalf("neg_upstream_imported_total=%d want >= %d", imported, seedCount)
	}

	got := reqKindIDs(t, fmt.Sprintf("ws://127.0.0.1:%d/", relayPort), 1)
	if len(got) < seedCount {
		t.Fatalf("REQ returned %d events, want >= %d", len(got), seedCount)
	}
	for id := range wantIDs {
		if _, ok := got[id]; !ok {
			t.Fatalf("missing imported id %s", id)
		}
	}
}

func ensureStrfryImage(t *testing.T) {
	t.Helper()
	if err := exec.Command("docker", "image", "inspect", strfryImage).Run(); err == nil {
		return
	}
	pull := exec.Command("docker", "pull", strfryImage)
	pull.Stdout = os.Stdout
	pull.Stderr = os.Stderr
	if err := pull.Run(); err != nil {
		t.Fatalf("docker pull %s: %v", strfryImage, err)
	}
}

func seedJSONL(t *testing.T, priv *btcec.PrivateKey, n int) ([]byte, map[string]struct{}) {
	t.Helper()
	pub := hex.EncodeToString(priv.PubKey().SerializeCompressed()[1:])
	base := time.Now().Unix() - int64(n) - 10
	var buf bytes.Buffer
	ids := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		ev := nostr.Event{
			PubKey:    pub,
			CreatedAt: base + int64(i),
			Kind:      1,
			Tags:      [][]string{},
			Content:   fmt.Sprintf("nip77-e2e-%d", i),
		}
		if err := ev.Sign(priv); err != nil {
			t.Fatal(err)
		}
		line, err := json.Marshal(ev)
		if err != nil {
			t.Fatal(err)
		}
		buf.Write(line)
		buf.WriteByte('\n')
		ids[ev.ID] = struct{}{}
	}
	return buf.Bytes(), ids
}

func writeCongeeConfig(t *testing.T, dir string, relayPort, adminPort int, upstreamURL string) string {
	t.Helper()
	dbPath := filepath.Join(dir, "events.db")
	body := fmt.Sprintf(`{
  "relay": { "port": %d },
  "admin": { "port": %d },
  "database": { "type": "turso", "dsn": %q },
  "logging": { "level": "info", "format": "json" },
  "audit": { "retention_days": 7 },
  "rate_limits": {
    "events_per_minute_per_connection": 6000,
    "bytes_per_second_per_connection": 10485760,
    "reqs_per_minute_per_connection": 6000,
    "messages_per_minute_per_ip": 60000
  },
  "connection_limits": {
    "max_open": 50,
    "max_open_per_ip": 20,
    "max_subscriptions_per_connection": 20,
    "max_filters_per_req": 10,
    "connections_per_minute_per_ip": 120,
    "idle_no_event_no_sub_seconds": 300,
    "read_deadline_seconds": 120,
    "write_deadline_seconds": 60,
    "default_query_limit": 0,
    "query_page_size": 500
  },
  "websocket": { "compression_enabled": false, "max_message_bytes": 1048576 },
  "max_subscription_id_length": 128,
  "nip11": { "name": "e2e", "description": "nip77 strfry", "pubkey": "", "contact": "", "software": "congee" },
  "nip77": {
    "max_records_per_query": 100000,
    "session_idle_timeout_seconds": 60,
    "frame_size_limit_bytes": 1048576,
    "max_concurrent_sessions": 4,
    "max_concurrent_loads": 2,
    "neg_open_per_minute_per_connection": 30,
    "neg_msg_per_minute_per_connection": 600,
    "backpressure_req_queue_depth": 0,
    "upstream_enabled": true,
    "upstream_pause_when_busy": false,
    "upstream_message_timeout_seconds": 180,
    "upstream_auth_wait_seconds": 0,
    "upstreams": [{
      "name": "strfry",
      "url": %q,
      "filters": [{"kinds":[1]}],
      "interval_seconds": 86400,
      "enabled": true
    }]
  },
  "nips": { "enabled": [1, 11, 77] }
}`, relayPort, adminPort, dbPath, upstreamURL)
	p := filepath.Join(dir, "config.json")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func congeeEnv(cfgPath, dataDir string) []string {
	env := os.Environ()
	drop := map[string]struct{}{
		"CONGEE_RELAY_PORT": {},
		"CONGEE_ADMIN_PORT": {},
		"CONFIG_PATH":       {},
		"CONGEE_DATA_DIR":   {},
		"ENABLE_ADMIN_UI":   {},
		"ADMIN_PASSWORD":    {},
		"CONGEE_ENV":        {},
	}
	out := make([]string, 0, len(env)+6)
	for _, e := range env {
		k, _, _ := strings.Cut(e, "=")
		if _, skip := drop[k]; skip {
			continue
		}
		out = append(out, e)
	}
	out = append(out,
		"CONFIG_PATH="+cfgPath,
		"CONGEE_DATA_DIR="+dataDir,
		"CONGEE_ENV=production",
		"ENABLE_ADMIN_UI=true",
		"ADMIN_PASSWORD="+adminPass,
	)
	return out
}

func waitSyncComplete(t *testing.T, logBuf *safeBuf, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		log := logBuf.String()
		if strings.Contains(log, "upstream sync failed") {
			t.Fatalf("upstream sync failed:\n%s", tail(log, 4000))
		}
		if strings.Contains(log, "upstream sync complete") {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for upstream sync complete:\n%s", tail(logBuf.String(), 4000))
}

func upstreamStats(t *testing.T, adminPort int) (imported, failures int) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/api/stats", adminPort), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+adminPass)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stats status %d", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	rc, _ := body["relay_counters"].(map[string]any)
	return jsonInt(rc["neg_upstream_imported_total"]), jsonInt(rc["neg_upstream_failures_total"])
}

func jsonInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}

func reqKindIDs(t *testing.T, url string, kind int) map[string]struct{} {
	t.Helper()
	d := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	c, _, err := d.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	payload, err := json.Marshal([]any{"REQ", "verify", map[string]any{"kinds": []int{kind}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.WriteMessage(websocket.TextMessage, payload); err != nil {
		t.Fatal(err)
	}
	_ = c.SetReadDeadline(time.Now().Add(60 * time.Second))
	got := map[string]struct{}{}
	for {
		_, data, err := c.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		var raw []json.RawMessage
		if err := json.Unmarshal(data, &raw); err != nil || len(raw) == 0 {
			continue
		}
		var typ string
		if err := json.Unmarshal(raw[0], &typ); err != nil {
			continue
		}
		switch typ {
		case "EVENT":
			if len(raw) < 3 {
				continue
			}
			var ev nostr.Event
			if err := json.Unmarshal(raw[2], &ev); err != nil {
				continue
			}
			got[ev.ID] = struct{}{}
		case "EOSE":
			return got
		case "NOTICE", "CLOSED":
			t.Fatalf("relay %s: %s", typ, data)
		}
	}
}

func waitWebSocket(t *testing.T, url string, timeout time.Duration) {
	t.Helper()
	d := websocket.Dialer{HandshakeTimeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		c, _, err := d.Dial(url, nil)
		if err == nil {
			_ = c.Close()
			return
		}
		last = err
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("websocket %s not ready: %v", url, last)
}

func waitHTTP(t *testing.T, url string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
			err = fmt.Errorf("status %d", resp.StatusCode)
		}
		last = err
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("http %s not ready: %v", url, last)
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

type safeBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *safeBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *safeBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}
