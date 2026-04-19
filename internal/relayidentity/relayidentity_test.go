package relayidentity

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/michmich112/congee/internal/config"
)

func TestResolvePathDefault(t *testing.T) {
	t.Setenv("RELAY_SECRETS_PATH", "")
	got := ResolvePath(filepath.Join("foo", "bar", "config.json"))
	want := filepath.Join("foo", "bar", "relay.secrets.json")
	if got != want {
		t.Fatalf("ResolvePath nested: got %q want %q", got, want)
	}
	if ResolvePath("config.json") != "relay.secrets.json" {
		t.Fatalf("ResolvePath cwd-style: got %q", ResolvePath("config.json"))
	}
}

func TestResolvePathEnvOverride(t *testing.T) {
	t.Setenv("RELAY_SECRETS_PATH", "/custom/secret.json")
	if ResolvePath("/irrelevant/config.json") != "/custom/secret.json" {
		t.Fatalf("RELAY_SECRETS_PATH override not respected")
	}
}

func TestLoadGeneratesStableRoundTrip(t *testing.T) {
	t.Setenv("RELAY_SECRETS_PATH", "")
	dir := t.TempDir()
	secPath := filepath.Join(dir, "relay.secrets.json")
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := ResolvePath(cfgPath)
	if path != secPath {
		t.Fatalf("unexpected resolved path %q want %q", path, secPath)
	}
	id1, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if id1.PubKeyHex() == "" || id1.NPub() == "" || !strings.HasPrefix(id1.NPub(), "npub1") {
		t.Fatalf("unexpected identity %+v %+v", id1.PubKeyHex(), id1.NPub())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var sf secretsFile
	if err := json.Unmarshal(data, &sf); err != nil {
		t.Fatal(err)
	}
	if len(sf.SecretKeyHex) != 64 {
		t.Fatalf("secret hex length: got %d", len(sf.SecretKeyHex))
	}
	if runtime.GOOS != "windows" {
		st, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if perm := st.Mode().Perm(); perm != 0o600 {
			t.Fatalf("want 0600 perms on Unix, got %o", perm)
		}
	}
	id2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if id1.PubKeyHex() != id2.PubKeyHex() || id1.NPub() != id2.NPub() {
		t.Fatalf("reload changed pubkey: %s vs %s", id1.PubKeyHex(), id2.PubKeyHex())
	}
}

func TestLoadKnownSecretDerivation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "relay.secrets.json")
	priv, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	wantHex := hex.EncodeToString(schnorr.SerializePubKey(priv.PubKey()))
	secretHex := hex.EncodeToString(priv.Serialize())
	if err := WriteTestSecrets(path, secretHex); err != nil {
		t.Fatal(err)
	}
	id, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if id.PubKeyHex() != wantHex {
		t.Fatalf("pubkey: got %s want %s", id.PubKeyHex(), wantHex)
	}
	npub, err := EncodeNpub(schnorr.SerializePubKey(priv.PubKey()))
	if err != nil {
		t.Fatal(err)
	}
	if id.NPub() != npub {
		t.Fatalf("npub: got %s want %s", id.NPub(), npub)
	}
}

func TestReconcileNIP11PubKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "relay.secrets.json")
	priv, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteTestSecrets(path, hex.EncodeToString(priv.Serialize())); err != nil {
		t.Fatal(err)
	}
	id, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{NIP11: config.NIP11Section{PubKey: "deadbeef"}}
	if err := ReconcileNIP11PubKey(cfg, id); err == nil {
		t.Fatal("expected mismatch error")
	}
	cfg2 := &config.Config{NIP11: config.NIP11Section{PubKey: ""}}
	if err := ReconcileNIP11PubKey(cfg2, id); err != nil {
		t.Fatal(err)
	}
	if cfg2.NIP11.PubKey != id.PubKeyHex() {
		t.Fatalf("empty config pubkey not filled: %q", cfg2.NIP11.PubKey)
	}
	cfg3 := &config.Config{NIP11: config.NIP11Section{PubKey: strings.ToUpper(id.PubKeyHex())}}
	if err := ReconcileNIP11PubKey(cfg3, id); err != nil {
		t.Fatal(err)
	}
	if cfg3.NIP11.PubKey != id.PubKeyHex() {
		t.Fatalf("case-insensitive match should canonicalize to lowercase derived hex")
	}
}

func TestEncodeNpubLength(t *testing.T) {
	if _, err := EncodeNpub([]byte{1, 2, 3}); err == nil {
		t.Fatal("expected error for short pubkey")
	}
}

func TestReconcileNilConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "relay.secrets.json")
	priv, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteTestSecrets(path, hex.EncodeToString(priv.Serialize())); err != nil {
		t.Fatal(err)
	}
	id, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := ReconcileNIP11PubKey(nil, id); err == nil {
		t.Fatal("expected error for nil config")
	}
}
