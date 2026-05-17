package relay

import (
	"strings"
	"testing"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/rs/zerolog"
)

func TestNIP17SubscribeAuthRequiresAuth(t *testing.T) {
	t.Parallel()
	cfg17 := &config.Config{
		NIPs: config.NIPsSection{Enabled: []int{1, 11, 17, 42}},
	}
	if nip17SubscribeAuthRequiresAuth(cfg17, []nostr.Filter{{Kinds: []int{1}}}) {
		t.Fatal("kind 1 only should not require auth for NIP-17")
	}
	if !nip17SubscribeAuthRequiresAuth(cfg17, []nostr.Filter{{Kinds: []int{nip17KindGiftWrap}}}) {
		t.Fatal("kind 1059 filter should require auth")
	}
	if !nip17SubscribeAuthRequiresAuth(cfg17, []nostr.Filter{{}}) {
		t.Fatal("wildcard kinds should require auth when NIP-17 enabled")
	}
	cfgNo17 := &config.Config{
		NIPs: config.NIPsSection{Enabled: []int{1, 11, 42}},
	}
	if nip17SubscribeAuthRequiresAuth(cfgNo17, []nostr.Filter{{Kinds: []int{nip17KindGiftWrap}}}) {
		t.Fatal("1059 filter without NIP-17 should not trigger NIP-17 auth rule")
	}
}

func TestEventVisibleToSubscriptionGiftWrapRecipientMatch(t *testing.T) {
	t.Parallel()
	st := &visibilityStoreStub{}
	cfg := minimalRelayCfg()
	cfg.NIPs.Enabled = []int{1, 11, 17, 42}
	cfg.NIP42.RelayURL = "wss://relay.example/"
	srv, err := NewServer(cfg, st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	recipient := strings.Repeat("a", 64)
	ev := &nostr.Event{
		Kind: nip17KindGiftWrap,
		Tags: [][]string{{"p", recipient}},
	}
	c := registerTestConn(t, srv, "gw-1")
	c.nip42AddPubkey(recipient)
	if !srv.EventVisibleToSubscription("gw-1", ev) {
		t.Fatal("expected visible when AUTH pubkey matches gift wrap p tag")
	}
}

func TestEventVisibleToSubscriptionGiftWrapWrongAuthHidden(t *testing.T) {
	t.Parallel()
	st := &visibilityStoreStub{}
	cfg := minimalRelayCfg()
	cfg.NIPs.Enabled = []int{1, 11, 17, 42}
	cfg.NIP42.RelayURL = "wss://relay.example/"
	srv, err := NewServer(cfg, st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	ev := &nostr.Event{
		Kind: nip17KindGiftWrap,
		Tags: [][]string{{"p", strings.Repeat("b", 64)}},
	}
	c := registerTestConn(t, srv, "gw-2")
	c.nip42AddPubkey(strings.Repeat("c", 64))
	if srv.EventVisibleToSubscription("gw-2", ev) {
		t.Fatal("expected hidden when AUTH pubkey does not match p tag")
	}
}

func TestEventVisibleToSubscriptionGiftWrapNoValidPHidden(t *testing.T) {
	t.Parallel()
	st := &visibilityStoreStub{}
	cfg := minimalRelayCfg()
	cfg.NIPs.Enabled = []int{1, 11, 17, 42}
	cfg.NIP42.RelayURL = "wss://relay.example/"
	srv, err := NewServer(cfg, st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	ev := &nostr.Event{
		Kind: nip17KindGiftWrap,
		Tags: [][]string{{"p", "short"}},
	}
	c := registerTestConn(t, srv, "gw-3")
	c.nip42AddPubkey(strings.Repeat("d", 64))
	if srv.EventVisibleToSubscription("gw-3", ev) {
		t.Fatal("expected hidden without valid recipient p tag")
	}
}

func TestEventVisibleToSubscriptionGiftWrapNIP17DisabledPassesThrough(t *testing.T) {
	t.Parallel()
	st := &visibilityStoreStub{}
	cfg := minimalRelayCfg()
	cfg.NIPs.Enabled = []int{1, 11, 42}
	cfg.NIP42.RelayURL = "wss://relay.example/"
	srv, err := NewServer(cfg, st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	ev := &nostr.Event{
		Kind: nip17KindGiftWrap,
		Tags: [][]string{{"p", strings.Repeat("e", 64)}},
	}
	if !srv.EventVisibleToSubscription("missing", ev) {
		t.Fatal("without NIP-17, gift wrap should not be gated by recipient")
	}
}

func TestNIP17ValidatePublishedGiftWrapAndSeal(t *testing.T) {
	t.Parallel()
	goodWrap := &nostr.Event{Kind: nip17KindGiftWrap, Tags: [][]string{{"p", strings.Repeat("f", 64)}}}
	if err := nip17ValidatePublishedEvent(goodWrap); err != nil {
		t.Fatal(err)
	}
	badWrap := &nostr.Event{Kind: nip17KindGiftWrap, Tags: [][]string{{"p", "bad"}}}
	if err := nip17ValidatePublishedEvent(badWrap); err == nil {
		t.Fatal("expected error for gift wrap without valid p tag")
	}
	sealBad := &nostr.Event{Kind: nip17KindSeal, Tags: [][]string{{"e", "x"}}}
	if err := nip17ValidatePublishedEvent(sealBad); err == nil {
		t.Fatal("expected error for seal with non-empty tags")
	}
	sealOK := &nostr.Event{Kind: nip17KindSeal, Tags: [][]string{}}
	if err := nip17ValidatePublishedEvent(sealOK); err != nil {
		t.Fatal(err)
	}
}
