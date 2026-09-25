package relay

import (
	"strings"
	"testing"

	"github.com/michmich112/congee/internal/nostr"
	"github.com/rs/zerolog"
)

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

func TestEventVisibleToSubscriptionGiftWrapNIP17DisabledWithholdsStoredWraps(t *testing.T) {
	t.Parallel()
	st := &visibilityStoreStub{}
	cfg := minimalRelayCfg()
	cfg.NIPs.Enabled = []int{1, 11, 42}
	cfg.NIP42.RelayURL = "wss://relay.example/"
	srv, err := NewServer(cfg, st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []int{nip17KindGiftWrap, nip59KindEphemeralGiftWrap} {
		ev := &nostr.Event{Kind: kind, Tags: [][]string{{"p", strings.Repeat("e", 64)}}}
		if srv.EventVisibleToSubscription("missing", ev) {
			t.Fatalf("kind %d must be withheld when NIP-17 is disabled", kind)
		}
	}
}

func TestEventVisibleToSubscriptionEphemeralGiftWrapRequiresRecipientAuth(t *testing.T) {
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
	ev := &nostr.Event{Kind: nip59KindEphemeralGiftWrap, Tags: [][]string{{"p", recipient}}}
	c := registerTestConn(t, srv, "ephemeral-gw")
	if srv.EventVisibleToSubscription(c.ID, ev) {
		t.Fatal("ephemeral gift wrap was visible before AUTH")
	}
	c.nip42AddPubkey(strings.Repeat("b", 64))
	if srv.EventVisibleToSubscription(c.ID, ev) {
		t.Fatal("ephemeral gift wrap was visible to a different authenticated pubkey")
	}
	c.nip42AddPubkey(recipient)
	if !srv.EventVisibleToSubscription(c.ID, ev) {
		t.Fatal("ephemeral gift wrap was hidden from its authenticated recipient")
	}
}

func TestNIP17ValidatePublishedGiftWrapAndSeal(t *testing.T) {
	t.Parallel()
	goodWrap := &nostr.Event{Kind: nip17KindGiftWrap, Tags: [][]string{{"p", strings.Repeat("f", 64)}}}
	if err := nip17ValidatePublishedEvent(goodWrap); err != nil {
		t.Fatal(err)
	}
	ephemeralWrap := &nostr.Event{Kind: nip59KindEphemeralGiftWrap, Tags: [][]string{{"p", strings.Repeat("f", 64)}}}
	if err := nip17ValidatePublishedEvent(ephemeralWrap); err != nil {
		t.Fatal(err)
	}
	badWrap := &nostr.Event{Kind: nip17KindGiftWrap, Tags: [][]string{{"p", "bad"}}}
	if err := nip17ValidatePublishedEvent(badWrap); err == nil {
		t.Fatal("expected error for gift wrap without valid p tag")
	}
	badEphemeralWrap := &nostr.Event{Kind: nip59KindEphemeralGiftWrap, Tags: [][]string{{"p", "bad"}}}
	if err := nip17ValidatePublishedEvent(badEphemeralWrap); err == nil {
		t.Fatal("expected error for ephemeral gift wrap without valid p tag")
	}
	for _, kind := range []int{nip17KindGiftWrap, nip59KindEphemeralGiftWrap} {
		multiRecipient := &nostr.Event{Kind: kind, Tags: [][]string{{"p", strings.Repeat("a", 64)}, {"p", strings.Repeat("b", 64)}}}
		if err := nip17ValidatePublishedEvent(multiRecipient); err == nil {
			t.Fatalf("kind %d with multiple recipients accepted", kind)
		}
		upperRecipient := &nostr.Event{Kind: kind, Tags: [][]string{{"p", strings.Repeat("A", 64)}}}
		if err := nip17ValidatePublishedEvent(upperRecipient); err == nil {
			t.Fatalf("kind %d with noncanonical recipient accepted", kind)
		}
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

func TestNIP17DisabledRejectsBothGiftWrapKinds(t *testing.T) {
	t.Parallel()
	cfg := minimalRelayCfg()
	for _, kind := range []int{nip17KindGiftWrap, nip59KindEphemeralGiftWrap} {
		if err := nip17ValidateRejectGiftWrapWhenDisabled(cfg, &nostr.Event{Kind: kind}); err == nil {
			t.Fatalf("kind %d must be rejected while NIP-17 is disabled", kind)
		}
	}
}
