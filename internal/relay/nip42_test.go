package relay

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/nostr"
)

func TestSubscribeAuthRequired(t *testing.T) {
	cfg := &config.Config{
		NIPs: make(map[string]config.NipPluginEntry),
		NIP42: config.NIP42Section{
			Enabled:                   true,
			RelayURL:                  "wss://r.example/nostr",
			RequireAuthSubscribeKinds: []int{4, 40},
		},
	}
	if subscribeAuthRequired(cfg, []nostr.Filter{{Kinds: []int{1}}}) {
		t.Fatal("kind 1 alone should not require auth")
	}
	if !subscribeAuthRequired(cfg, []nostr.Filter{{Kinds: []int{4}}}) {
		t.Fatal("kind 4 should require auth")
	}
	if !subscribeAuthRequired(cfg, []nostr.Filter{{Kinds: []int{1, 4}}}) {
		t.Fatal("mixed filter with 4 should require auth")
	}
	if !subscribeAuthRequired(cfg, []nostr.Filter{{}}) {
		t.Fatal("empty kinds should require auth when policy lists kinds")
	}
}

func TestVerifyNIP42AuthEvent(t *testing.T) {
	priv, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	pub := priv.PubKey()
	pubHex := hex.EncodeToString(pub.SerializeCompressed()[1:])

	cfg := &config.Config{
		NIPs: make(map[string]config.NipPluginEntry),
		NIP42: config.NIP42Section{
			Enabled:              true,
			RelayURL:             "wss://relay.example.com/",
			CreatedAtSkewSeconds: 600,
		},
	}
	challenge := "test-challenge-abc"
	now := time.Unix(1700000000, 0)
	ev := nostr.Event{
		PubKey:    pubHex,
		CreatedAt: now.Unix(),
		Kind:      nip42AuthEventKind,
		Tags: [][]string{
			{"relay", "wss://relay.example.com"},
			{"challenge", challenge},
		},
		Content: "",
	}
	if _, err := ev.ComputeID(); err != nil {
		t.Fatal(err)
	}
	if err := ev.Sign(priv); err != nil {
		t.Fatal(err)
	}
	if err := verifyNIP42AuthEvent(cfg, &ev, challenge, now); err != nil {
		t.Fatal(err)
	}
	if err := verifyNIP42AuthEvent(cfg, &ev, "wrong", now); err == nil {
		t.Fatal("expected challenge mismatch")
	}
	ev2 := ev
	ev2.CreatedAt = now.Unix() - 10000
	if _, err := ev2.ComputeID(); err != nil {
		t.Fatal(err)
	}
	if err := ev2.Sign(priv); err != nil {
		t.Fatal(err)
	}
	if err := verifyNIP42AuthEvent(cfg, &ev2, challenge, now); err == nil {
		t.Fatal("expected skew failure")
	}
}

func TestValidateNIP42PublishPolicy(t *testing.T) {
	cfg := &config.Config{
		NIPs: make(map[string]config.NipPluginEntry),
		NIP42: config.NIP42Section{
			Enabled:                 true,
			RelayURL:                "wss://r/",
			RequireAuthPublishKinds: []int{1},
			AllowlistedPubkeys:      []string{"abc"},
		},
	}
	c := &Conn{}
	ev := &nostr.Event{Kind: 1, PubKey: "abc"}
	if validateNIP42PublishPolicy(cfg, c, ev) == nil {
		t.Fatal("expected auth-required")
	}
	c.nip42AddPubkey("abc")
	if err := validateNIP42PublishPolicy(cfg, c, ev); err != nil {
		t.Fatal(err)
	}
	ev2 := &nostr.Event{Kind: 1, PubKey: "def"}
	c2 := &Conn{}
	c2.nip42AddPubkey("def")
	if err := validateNIP42PublishPolicy(cfg, c2, ev2); err == nil {
		t.Fatal("expected restricted")
	}
}
