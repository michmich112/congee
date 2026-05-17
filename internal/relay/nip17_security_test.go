package relay

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
	"github.com/rs/zerolog"
)

// nip17FilterStore implements QueryEvents by matching the in-memory pool with nostr.Filter.Matches
// (same AND semantics clients rely on). Other Store methods come from visibilityStoreStub.
type nip17FilterStore struct {
	visibilityStoreStub
	pool []*nostr.Event
}

func (s *nip17FilterStore) QueryEvents(ctx context.Context, filters []nostr.Filter) ([]*nostr.Event, error) {
	_ = ctx
	var out []*nostr.Event
	seen := make(map[string]struct{})
	for _, f := range filters {
		for _, ev := range s.pool {
			if f.Matches(ev) {
				if _, ok := seen[ev.ID]; !ok {
					seen[ev.ID] = struct{}{}
					out = append(out, ev)
				}
			}
		}
	}
	return out, nil
}

func nip17SecurityTestCfg() *config.Config {
	cfg := testRelayConfig()
	cfg.NIPs.Enabled = []int{1, 11, 17, 42}
	cfg.NIP42.RelayURL = "wss://relay.example/"
	return cfg
}

func signedGiftWrapEvent(t *testing.T, wrapPriv *btcec.PrivateKey, recipientPubHex string) *nostr.Event {
	t.Helper()
	pub := wrapPriv.PubKey()
	pubHex := hex.EncodeToString(pub.SerializeCompressed()[1:])
	ev := &nostr.Event{
		PubKey:    pubHex,
		CreatedAt: 1700000000,
		Kind:      nip17KindGiftWrap,
		Content:   "cipher",
		Tags:      [][]string{{"p", recipientPubHex}},
	}
	if _, err := ev.ComputeID(); err != nil {
		t.Fatal(err)
	}
	if err := ev.Sign(wrapPriv); err != nil {
		t.Fatal(err)
	}
	return ev
}

func registerTestConnLargeSend(t *testing.T, srv *Server, id string) *Conn {
	t.Helper()
	c := registerTestConn(t, srv, id)
	c.send = make(chan []byte, 64)
	return c
}

func drainAuthThenReadClosed(t *testing.T, c *Conn) (closedMsg string) {
	t.Helper()
	for {
		var arr []any
		select {
		case b := <-c.send:
			if err := json.Unmarshal(b, &arr); err != nil {
				t.Fatalf("outbound json: %v", err)
			}
			if len(arr) == 0 {
				t.Fatal("empty outbound frame")
			}
			typ, _ := arr[0].(string)
			switch typ {
			case "AUTH":
				continue
			case "CLOSED":
				if len(arr) > 2 {
					closedMsg, _ = arr[2].(string)
				}
				return closedMsg
			default:
				t.Fatalf("unexpected frame type %q, want AUTH or CLOSED", typ)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("timeout waiting for CLOSED")
		}
	}
}

func drainOutboundChan(t *testing.T, c *Conn, max int) []string {
	t.Helper()
	var types []string
	for i := 0; i < max; i++ {
		select {
		case b := <-c.send:
			var arr []any
			if err := json.Unmarshal(b, &arr); err != nil {
				t.Fatalf("outbound json: %v", err)
			}
			if len(arr) == 0 {
				t.Fatal("empty outbound frame")
			}
			typ, _ := arr[0].(string)
			types = append(types, typ)
			if typ == "EOSE" {
				return types
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("timeout after %d frames: got %#v", i, types)
		}
	}
	t.Fatalf("no EOSE after %d frames: %#v", max, types)
	return types
}

func registerNIP01NIP42NIP17(srv *Server, st storage.Store) {
	RegisterNIP01(srv, st)
	RegisterNIP42(srv, st)
	RegisterNIP17(srv, st)
}

func TestHandleREQ_NIP17_IDsOnlyGiftWrapWithoutAuthClosed(t *testing.T) {
	t.Parallel()
	alice := strings.Repeat("a", 64)
	priv, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	wrap := signedGiftWrapEvent(t, priv, alice)
	st := &nip17FilterStore{pool: []*nostr.Event{wrap}}
	srv, err := NewServer(nip17SecurityTestCfg(), st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	registerNIP01NIP42NIP17(srv, st)
	c := registerTestConnLargeSend(t, srv, "req-ids-noauth")
	ctx := context.Background()
	req := &nostr.ReqMessage{
		SubID:   "sub1",
		Filters: []nostr.Filter{{IDs: []string{wrap.ID}}},
	}
	if err := handleREQ(ctx, srv, c, req, false); err != nil {
		t.Fatal(err)
	}
	msg := drainAuthThenReadClosed(t, c)
	if !strings.Contains(msg, "auth-required") {
		t.Fatalf("CLOSED message should mention auth-required, got %q", msg)
	}
}

func TestHandleREQ_NIP17_Kinds1059WithoutAuthClosed(t *testing.T) {
	t.Parallel()
	alice := strings.Repeat("b", 64)
	priv, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	wrap := signedGiftWrapEvent(t, priv, alice)
	st := &nip17FilterStore{pool: []*nostr.Event{wrap}}
	srv, err := NewServer(nip17SecurityTestCfg(), st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	registerNIP01NIP42NIP17(srv, st)
	c := registerTestConnLargeSend(t, srv, "req-k1059-noauth")
	req := &nostr.ReqMessage{
		SubID:   "sub1",
		Filters: []nostr.Filter{{Kinds: []int{nip17KindGiftWrap}}},
	}
	if err := handleREQ(context.Background(), srv, c, req, false); err != nil {
		t.Fatal(err)
	}
	msg := drainAuthThenReadClosed(t, c)
	if !strings.Contains(msg, "auth-required") {
		t.Fatalf("CLOSED message should mention auth-required, got %q", msg)
	}
}

func TestHandleREQ_NIP17_IDsOnlyWithAuthRecipientGetsEventThenEOSE(t *testing.T) {
	t.Parallel()
	alice := strings.Repeat("c", 64)
	priv, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	wrap := signedGiftWrapEvent(t, priv, alice)
	st := &nip17FilterStore{pool: []*nostr.Event{wrap}}
	srv, err := NewServer(nip17SecurityTestCfg(), st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	registerNIP01NIP42NIP17(srv, st)
	c := registerTestConnLargeSend(t, srv, "req-ids-auth-ok")
	c.nip42AddPubkey(alice)
	req := &nostr.ReqMessage{
		SubID:   "sub1",
		Filters: []nostr.Filter{{IDs: []string{wrap.ID}}},
	}
	if err := handleREQ(context.Background(), srv, c, req, false); err != nil {
		t.Fatal(err)
	}
	types := drainOutboundChan(t, c, 4)
	if len(types) < 2 || types[0] != "EVENT" || types[len(types)-1] != "EOSE" {
		t.Fatalf("want EVENT then EOSE, got %#v", types)
	}
}

func TestHandleREQ_NIP17_IDsOnlyWithAuthWrongRecipientNoEvent(t *testing.T) {
	t.Parallel()
	alice := strings.Repeat("d", 64)
	priv, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	wrap := signedGiftWrapEvent(t, priv, alice)
	st := &nip17FilterStore{pool: []*nostr.Event{wrap}}
	srv, err := NewServer(nip17SecurityTestCfg(), st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	registerNIP01NIP42NIP17(srv, st)
	c := registerTestConnLargeSend(t, srv, "req-ids-auth-wrong")
	c.nip42AddPubkey(strings.Repeat("e", 64))
	req := &nostr.ReqMessage{
		SubID:   "sub1",
		Filters: []nostr.Filter{{IDs: []string{wrap.ID}}},
	}
	if err := handleREQ(context.Background(), srv, c, req, false); err != nil {
		t.Fatal(err)
	}
	types := drainOutboundChan(t, c, 4)
	for _, typ := range types {
		if typ == "EVENT" {
			t.Fatal("non-recipient must not receive gift wrap EVENT")
		}
	}
	if len(types) == 0 || types[len(types)-1] != "EOSE" {
		t.Fatalf("want EOSE only, got %#v", types)
	}
}

func TestHandleREQ_NIP17_Kinds1AndIDGiftWrap_NoAuth_NoLeak(t *testing.T) {
	t.Parallel()
	alice := strings.Repeat("f", 64)
	priv, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	wrap := signedGiftWrapEvent(t, priv, alice)
	st := &nip17FilterStore{pool: []*nostr.Event{wrap}}
	srv, err := NewServer(nip17SecurityTestCfg(), st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	registerNIP01NIP42NIP17(srv, st)
	c := registerTestConnLargeSend(t, srv, "req-narrow-noauth")
	// Contradictory filter: kind 1 AND this id — cannot match a 1059; must not require NIP-17 AUTH
	// but also must never surface the wrap.
	req := &nostr.ReqMessage{
		SubID: "sub1",
		Filters: []nostr.Filter{{
			Kinds: []int{1},
			IDs:   []string{wrap.ID},
		}},
	}
	if err := handleREQ(context.Background(), srv, c, req, false); err != nil {
		t.Fatal(err)
	}
	types := drainOutboundChan(t, c, 4)
	for _, typ := range types {
		if typ == "EVENT" {
			t.Fatal("narrow kinds=[1] must not leak kind 1059")
		}
	}
	if types[len(types)-1] != "EOSE" {
		t.Fatalf("want EOSE, got %#v", types)
	}
}

func TestHandleREQ_NIP17_MultiFilterORSecondIDsOnlyRequiresAuth(t *testing.T) {
	t.Parallel()
	alice := strings.Repeat("1", 64)
	priv, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	wrap := signedGiftWrapEvent(t, priv, alice)
	st := &nip17FilterStore{pool: []*nostr.Event{wrap}}
	srv, err := NewServer(nip17SecurityTestCfg(), st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	registerNIP01NIP42NIP17(srv, st)
	c := registerTestConnLargeSend(t, srv, "req-or-noauth")
	req := &nostr.ReqMessage{
		SubID: "sub1",
		Filters: []nostr.Filter{
			{Kinds: []int{1}},
			{IDs: []string{wrap.ID}},
		},
	}
	if err := handleREQ(context.Background(), srv, c, req, false); err != nil {
		t.Fatal(err)
	}
	msg := drainAuthThenReadClosed(t, c)
	if !strings.Contains(msg, "auth-required") {
		t.Fatalf("CLOSED message should mention auth-required, got %q", msg)
	}
}

func TestNIP17SubscribeAuthRequiresAuth_MultiFilterAnyWildcardKinds(t *testing.T) {
	t.Parallel()
	cfg := nip17SecurityTestCfg()
	if !nip17SubscribeAuthRequiresAuth(cfg, []nostr.Filter{
		{Kinds: []int{1}},
		{IDs: []string{strings.Repeat("a", 64)}},
	}) {
		t.Fatal("expected auth when one filter has wildcard kinds (ids-only)")
	}
	if nip17SubscribeAuthRequiresAuth(cfg, []nostr.Filter{
		{Kinds: []int{1}},
		{Kinds: []int{2}},
	}) {
		t.Fatal("kinds [1] and [2] only should not require NIP-17 auth")
	}
}

func TestBroadcast_NIP17GiftWrapNotSentToWrongAuthedSubscriber(t *testing.T) {
	t.Parallel()
	alice := strings.Repeat("2", 64)
	priv, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	wrap := signedGiftWrapEvent(t, priv, alice)
	st := &visibilityStoreStub{}
	srv, err := NewServer(nip17SecurityTestCfg(), st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	registerNIP01NIP42NIP17(srv, st)
	var got int
	srv.subs.RegisterSender("bc1", func([]byte) bool {
		got++
		return true
	})
	c := registerTestConn(t, srv, "bc1")
	c.nip42AddPubkey(strings.Repeat("3", 64))
	if err := srv.subs.Add("bc1", "sub1", []nostr.Filter{{Kinds: []int{nip17KindGiftWrap}}}); err != nil {
		t.Fatal(err)
	}
	srv.broadcastEvent(wrap)
	if got != 0 {
		t.Fatalf("expected 0 live EVENT for wrong recipient, got %d", got)
	}
	c.nip42AddPubkey(alice)
	srv.broadcastEvent(wrap)
	if got != 1 {
		t.Fatalf("expected 1 EVENT after recipient AUTH added, got %d", got)
	}
}

func TestHandleEVENT_NIP17_GiftWrapInvalidSigRejected(t *testing.T) {
	t.Parallel()
	alice := strings.Repeat("4", 64)
	st := &visibilityStoreStub{}
	srv, err := NewServer(nip17SecurityTestCfg(), st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	registerNIP01NIP42NIP17(srv, st)
	c := registerTestConnLargeSend(t, srv, "ev-badsig")
	ev := &nostr.Event{
		ID:        strings.Repeat("a", 64),
		PubKey:    strings.Repeat("b", 64),
		CreatedAt: 1,
		Kind:      nip17KindGiftWrap,
		Tags:      [][]string{{"p", alice}},
		Content:   "x",
		Sig:       strings.Repeat("c", 128),
	}
	msg := &nostr.EventMessage{Event: *ev}
	if err := handleEVENT(context.Background(), srv, st, c, msg); err != nil {
		t.Fatal(err)
	}
	var okArr []any
	if err := json.Unmarshal(<-c.send, &okArr); err != nil {
		t.Fatal(err)
	}
	if len(okArr) < 3 {
		t.Fatalf("OK frame: %#v", okArr)
	}
	if typ, _ := okArr[0].(string); typ != "OK" {
		t.Fatalf("want OK, got %q", typ)
	}
	ok, _ := okArr[2].(bool)
	if ok {
		t.Fatal("invalid signature must not be accepted for kind 1059")
	}
}

func TestHandleEVENT_NIP17_GiftWrapValidSigNoRecipientTagRejected(t *testing.T) {
	t.Parallel()
	priv, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	pub := priv.PubKey()
	pubHex := hex.EncodeToString(pub.SerializeCompressed()[1:])
	ev := &nostr.Event{
		PubKey:    pubHex,
		CreatedAt: 1700000100,
		Kind:      nip17KindGiftWrap,
		Tags:      [][]string{{"e", strings.Repeat("f", 64)}},
		Content:   "x",
	}
	if _, err := ev.ComputeID(); err != nil {
		t.Fatal(err)
	}
	if err := ev.Sign(priv); err != nil {
		t.Fatal(err)
	}
	st := &visibilityStoreStub{}
	srv, err := NewServer(nip17SecurityTestCfg(), st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	registerNIP01NIP42NIP17(srv, st)
	c := registerTestConnLargeSend(t, srv, "ev-nop")
	msg := &nostr.EventMessage{Event: *ev}
	if err := handleEVENT(context.Background(), srv, st, c, msg); err != nil {
		t.Fatal(err)
	}
	var okArr []any
	if err := json.Unmarshal(<-c.send, &okArr); err != nil {
		t.Fatal(err)
	}
	ok, _ := okArr[2].(bool)
	if ok {
		t.Fatal("gift wrap without valid p tag must be rejected")
	}
}

func TestHandleEVENT_NIP17_GiftWrapNonHexPubkeyInPTagRejected(t *testing.T) {
	t.Parallel()
	priv, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	pub := priv.PubKey()
	pubHex := hex.EncodeToString(pub.SerializeCompressed()[1:])
	badP := strings.Repeat("g", 64) // 64 chars but not valid hex
	ev := &nostr.Event{
		PubKey:    pubHex,
		CreatedAt: 1700000200,
		Kind:      nip17KindGiftWrap,
		Tags:      [][]string{{"p", badP}},
		Content:   "x",
	}
	if _, err := ev.ComputeID(); err != nil {
		t.Fatal(err)
	}
	if err := ev.Sign(priv); err != nil {
		t.Fatal(err)
	}
	st := &visibilityStoreStub{}
	srv, err := NewServer(nip17SecurityTestCfg(), st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	registerNIP01NIP42NIP17(srv, st)
	c := registerTestConnLargeSend(t, srv, "ev-badp")
	msg := &nostr.EventMessage{Event: *ev}
	if err := handleEVENT(context.Background(), srv, st, c, msg); err != nil {
		t.Fatal(err)
	}
	var okArr []any
	if err := json.Unmarshal(<-c.send, &okArr); err != nil {
		t.Fatal(err)
	}
	ok, _ := okArr[2].(bool)
	if ok {
		t.Fatal("invalid hex in p tag must be rejected")
	}
}

func TestHandleEVENT_NIP17_SealNonEmptyTagsRejected(t *testing.T) {
	t.Parallel()
	priv, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	pub := priv.PubKey()
	pubHex := hex.EncodeToString(pub.SerializeCompressed()[1:])
	ev := &nostr.Event{
		PubKey:    pubHex,
		CreatedAt: 1700000300,
		Kind:      nip17KindSeal,
		Tags:      [][]string{{"e", strings.Repeat("a", 64)}},
		Content:   "sealed",
	}
	if _, err := ev.ComputeID(); err != nil {
		t.Fatal(err)
	}
	if err := ev.Sign(priv); err != nil {
		t.Fatal(err)
	}
	st := &visibilityStoreStub{}
	srv, err := NewServer(nip17SecurityTestCfg(), st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	registerNIP01NIP42NIP17(srv, st)
	c := registerTestConnLargeSend(t, srv, "ev-seal")
	msg := &nostr.EventMessage{Event: *ev}
	if err := handleEVENT(context.Background(), srv, st, c, msg); err != nil {
		t.Fatal(err)
	}
	var okArr []any
	if err := json.Unmarshal(<-c.send, &okArr); err != nil {
		t.Fatal(err)
	}
	ok, _ := okArr[2].(bool)
	if ok {
		t.Fatal("kind 13 with non-empty tags must be rejected when NIP-17 is enabled")
	}
}

func TestEventVisibleToSubscriptionGiftWrapMultiplePMatchesAny(t *testing.T) {
	t.Parallel()
	alice := strings.Repeat("5", 64)
	bob := strings.Repeat("6", 64)
	st := &visibilityStoreStub{}
	cfg := nip17SecurityTestCfg()
	srv, err := NewServer(cfg, st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	ev := &nostr.Event{
		ID:        strings.Repeat("7", 64),
		Kind:      nip17KindGiftWrap,
		Tags:      [][]string{{"p", alice}, {"p", bob}},
		CreatedAt: 1,
	}
	c := registerTestConn(t, srv, "multi-p")
	c.nip42AddPubkey(strings.ToUpper(bob[:8]) + bob[8:])
	if !srv.EventVisibleToSubscription("multi-p", ev) {
		t.Fatal("expected visible when second p tag matches (case-insensitive)")
	}
}
