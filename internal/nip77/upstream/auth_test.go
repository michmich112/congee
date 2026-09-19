package upstream

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/michmich112/congee/internal/relayidentity"
)

func TestBuildUpstreamAuthEvent(t *testing.T) {
	priv, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "relay.secrets.json")
	if err := relayidentity.WriteTestSecrets(path, fmt.Sprintf("%x", priv.Serialize())); err != nil {
		t.Fatal(err)
	}
	id, err := relayidentity.Load(path)
	if err != nil {
		t.Fatal(err)
	}

	ev, err := buildUpstreamAuthEvent(id, "wss://Relay.Kubo.Watch/", "challenge-abc")
	if err != nil {
		t.Fatal(err)
	}
	if ev.Kind != 22242 {
		t.Fatalf("kind %d", ev.Kind)
	}
	if ev.PubKey != id.PubKeyHex() {
		t.Fatalf("pubkey %s want %s", ev.PubKey, id.PubKeyHex())
	}
	if err := ev.VerifySig(); err != nil {
		t.Fatal(err)
	}
	if got := tagFirst(ev.Tags, "relay"); got != "wss://relay.kubo.watch/" {
		t.Fatalf("relay tag %q", got)
	}
	if got := tagFirst(ev.Tags, "challenge"); got != "challenge-abc" {
		t.Fatalf("challenge tag %q", got)
	}
}

func TestBuildUpstreamAuthEventNilIdentity(t *testing.T) {
	if _, err := buildUpstreamAuthEvent(nil, "wss://example.com/", "c"); err == nil {
		t.Fatal("expected error")
	}
}

func tagFirst(tags [][]string, name string) string {
	for _, t := range tags {
		if len(t) >= 2 && t[0] == name {
			return t[1]
		}
	}
	return ""
}
