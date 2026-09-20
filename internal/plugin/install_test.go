package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michmich112/congee/internal/nostr"
)

func writePluginPkg(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	man := `{"id":"fixture","name":"Fixture","version":"0.0.1","api_version":1,"exec":{"darwin_arm64":"bin/p"}}`
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(man), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bin", "p"), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestInstallLocalKeepsData(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(t.TempDir(), "pkg")
	writePluginPkg(t, src)
	if _, _, err := installFromDir(root, src); err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(root, "fixture", "data")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(data, "keep.txt")
	if err := os.WriteFile(marker, []byte("yes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "plugin.json"), []byte(`{"id":"fixture","name":"Fixture","version":"0.0.2","api_version":1,"exec":{"darwin_arm64":"bin/p"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := installFromDir(root, src); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "yes" {
		t.Fatalf("data wiped: %s", got)
	}
}

func TestInstallURLChecksumMismatch(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "p.bin")
	payload := []byte("not-an-archive-but-hashed")
	if err := os.WriteFile(archive, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	t.Cleanup(srv.Close)
	_, _, err := installFromURL(t.TempDir(), srv.URL, strings.Repeat("ab", 32))
	if err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("want mismatch, got %v", err)
	}
	_, _, err = installFromURL(t.TempDir(), srv.URL, "")
	if err == nil || !strings.Contains(err.Error(), "sha256 required") {
		t.Fatalf("want sha required, got %v", err)
	}
	_, _, err = installFromURL(t.TempDir(), srv.URL, hex.EncodeToString(sum[:]))
	if err == nil {
		t.Fatal("expected extract error after matching checksum")
	}
}

func TestListenEnqueueDoesNotBlock(t *testing.T) {
	in := &instance{listenQ: make(chan listenJob, 1), mgr: &Manager{}}
	in.listenQ <- listenJob{}
	in.enqueueStored(&nostr.Event{ID: "aa", PubKey: "bb", Kind: 1}, true)
	if in.mgr.listenDropped.Load() != 1 {
		t.Fatalf("dropped=%d want 1", in.mgr.listenDropped.Load())
	}
}
