package relay

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/michmich112/congee/internal/db"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/plugin"
	"github.com/michmich112/congee/internal/storage/turso"
	"github.com/rs/zerolog"
)

func snapshotIDs(t *testing.T, c *Conn) []string {
	t.Helper()
	var ids []string
	var eose bool
	for len(c.send) > 0 {
		var frame []json.RawMessage
		if err := json.Unmarshal(<-c.send, &frame); err != nil {
			t.Fatal(err)
		}
		var typ string
		_ = json.Unmarshal(frame[0], &typ)
		switch typ {
		case "EVENT":
			var ev nostr.Event
			if err := json.Unmarshal(frame[2], &ev); err != nil {
				t.Fatal(err)
			}
			ids = append(ids, ev.ID)
		case "EOSE":
			eose = true
		case "AUTH": // A challenge may accompany a partially served request.
		default:
			t.Fatalf("unexpected response: %s", frame)
		}
	}
	if !eose {
		t.Fatal("missing EOSE")
	}
	sort.Strings(ids)
	return ids
}

func TestREQMixedBroadAndIDQueriesPreservePublicResults(t *testing.T) {
	alice, bob := strings.Repeat("a", 64), strings.Repeat("b", 64)
	public := &nostr.Event{ID: strings.Repeat("1", 64), Kind: 30402, CreatedAt: 1}
	wrap := &nostr.Event{ID: strings.Repeat("2", 64), Kind: 1059, Tags: [][]string{{"p", alice}}, CreatedAt: 2}
	other := &nostr.Event{ID: strings.Repeat("3", 64), Kind: 1059, Tags: [][]string{{"p", bob}}, CreatedAt: 3}
	for _, authed := range []bool{false, true} {
		for _, filters := range [][]nostr.Filter{
			{{}},
			{{Kinds: []int{30402, 1059, 21059}}},
			{{Kinds: []int{30402}}, {Kinds: []int{1059}, Tag: map[string][]string{"#p": {alice, bob}}}},
			{{IDs: []string{public.ID, wrap.ID, other.ID}}},
		} {
			st := &nip17FilterStore{pool: []*nostr.Event{public, wrap, other}}
			cfg := nip17SecurityTestCfg()
			cfg.NIP42.RequireAuthSubscribeKinds = []int{1059, 21059}
			srv, err := NewServer(cfg, st, zerolog.Nop(), nil)
			if err != nil {
				t.Fatal(err)
			}
			c := registerTestConnLargeSend(t, srv, "mixed")
			if authed {
				c.nip42AddPubkey(alice)
			}
			if err := handleREQ(t.Context(), srv, c, &nostr.ReqMessage{SubID: "s", Filters: filters}, false); err != nil {
				t.Fatal(err)
			}
			want := []string{public.ID}
			if authed {
				want = append(want, wrap.ID)
			}
			if got := snapshotIDs(t, c); !reflect.DeepEqual(got, want) {
				t.Fatalf("auth=%v filters=%+v got=%v want=%v", authed, filters, got, want)
			}
		}
	}
}

func TestREQProtectedRowsDoNotConsumeLimitOrPagination(t *testing.T) {
	if !turso.HasDriver() {
		t.Skip("libsql driver not available")
	}
	st, closeFn, err := db.OpenTestStore(t.Context(), filepath.Join(t.TempDir(), "events.db"), zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer closeFn()
	alice := strings.Repeat("a", 64)
	for i, ev := range []*nostr.Event{
		{Kind: 1, CreatedAt: 1, Content: "catalog"},
		{Kind: 1, CreatedAt: 2, Content: "catalog"},
		{Kind: 1059, CreatedAt: 3, Content: "catalog", Tags: [][]string{{"p", alice}}},
	} {
		ev.ID, ev.PubKey, ev.Sig = strings.Repeat(string(rune('1'+i)), 64), alice, strings.Repeat("f", 128)
		if err := st.SaveEvent(t.Context(), ev); err != nil {
			t.Fatal(err)
		}
	}
	for _, search := range []bool{false, true} {
		cfg := nip17SecurityTestCfg()
		pageSize, defaultLimit := 1, 0
		cfg.ConnectionLimits.QueryPageSize = &pageSize
		cfg.ConnectionLimits.DefaultQueryLimit = &defaultLimit
		srv, err := NewServer(cfg, st, zerolog.Nop(), nil)
		if err != nil {
			t.Fatal(err)
		}
		srv.readQueue = nil // Exercise synchronous draining of all pages.
		c := registerTestConnLargeSend(t, srv, "limited")
		limit := 2
		filter := nostr.Filter{Limit: &limit}
		if search {
			query := "catalog"
			filter.Search = &query
		}
		if err := handleREQ(t.Context(), srv, c, &nostr.ReqMessage{SubID: "s", Filters: []nostr.Filter{filter}}, search); err != nil {
			t.Fatal(err)
		}
		want := []string{strings.Repeat("1", 64), strings.Repeat("2", 64)}
		if got := snapshotIDs(t, c); !reflect.DeepEqual(got, want) {
			t.Fatalf("search=%v got=%v want=%v", search, got, want)
		}
	}
}

type privacyPluginRuntime struct {
	recordingPluginRuntime
	result plugin.InterceptResult
}

func (r *privacyPluginRuntime) InterceptREQ(context.Context, *nostr.ReqMessage) plugin.InterceptResult {
	return r.result
}

func TestREQPluginResponseAndReshapeCannotExposeGiftWrap(t *testing.T) {
	alice := strings.Repeat("a", 64)
	public := &nostr.Event{ID: strings.Repeat("1", 64), Kind: 30402, CreatedAt: 1}
	wrap := &nostr.Event{ID: strings.Repeat("2", 64), Kind: 1059, Tags: [][]string{{"p", alice}}, CreatedAt: 2}
	for _, action := range []plugin.InterceptAction{plugin.InterceptRespond, plugin.InterceptReshapeREQ} {
		st := &nip17OverReturnStore{events: []*nostr.Event{public, wrap}}
		srv, err := NewServer(nip17SecurityTestCfg(), st, zerolog.Nop(), nil)
		if err != nil {
			t.Fatal(err)
		}
		srv.SetPluginRuntime(&privacyPluginRuntime{result: plugin.InterceptResult{Action: action, EventIDs: []string{public.ID, wrap.ID}, Filters: []nostr.Filter{{}}, SubscriptionFilters: []nostr.Filter{{}}}})
		c := registerTestConnLargeSend(t, srv, "plugin")
		if err := handleREQ(t.Context(), srv, c, &nostr.ReqMessage{SubID: "s", Filters: []nostr.Filter{{}}}, false); err != nil {
			t.Fatal(err)
		}
		if got := snapshotIDs(t, c); !reflect.DeepEqual(got, []string{public.ID}) {
			t.Fatalf("action=%v leaked or dropped public: %v", action, got)
		}
	}
}

func TestBroadLiveSubscriptionUsesCurrentAuthPermissions(t *testing.T) {
	srv, err := NewServer(nip17SecurityTestCfg(), &nip17FilterStore{}, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	c := registerTestConnLargeSend(t, srv, "live-broad")
	srv.subs.RegisterSender(c.ID, func(b []byte) bool { return c.enqueue(b) == nil })
	if err := handleREQ(t.Context(), srv, c, &nostr.ReqMessage{SubID: "s", Filters: []nostr.Filter{{}}}, false); err != nil {
		t.Fatal(err)
	}
	snapshotIDs(t, c)
	alice := strings.Repeat("a", 64)
	wrap := &nostr.Event{ID: strings.Repeat("2", 64), Kind: 21059, Tags: [][]string{{"p", alice}}}
	srv.broadcastEvent(wrap)
	if len(c.send) != 0 {
		t.Fatal("anonymous live gift wrap leaked")
	}
	c.nip42AddPubkey(alice)
	srv.broadcastEvent(wrap)
	if len(c.send) != 1 {
		t.Fatal("broad subscription failed to serve newly authenticated recipient")
	}
}

func TestGiftWrapPublishWithoutAuthStoresValidSignedWrapper(t *testing.T) {
	if !turso.HasDriver() {
		t.Skip("libsql driver not available")
	}
	st, closeFn, err := db.OpenTestStore(t.Context(), filepath.Join(t.TempDir(), "events.db"), zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer closeFn()
	srv, err := NewServer(nip17SecurityTestCfg(), st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	registerNIP01NIP42NIP17(srv, st)
	c := registerTestConnLargeSend(t, srv, "write-without-auth")
	priv, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	ev := signedGiftWrapEvent(t, priv, strings.Repeat("a", 64))
	if err := handleEVENT(t.Context(), srv, st, c, &nostr.EventMessage{Event: *ev}); err != nil {
		t.Fatal(err)
	}
	var frame []any
	if err := json.Unmarshal(<-c.send, &frame); err != nil {
		t.Fatal(err)
	}
	if frame[0] != "OK" || frame[2] != true {
		t.Fatalf("publish rejected: %v", frame)
	}
	if c.nip42HasAnyAuth() {
		t.Fatal("test unexpectedly authenticated")
	}
	if has, err := st.HasEventID(t.Context(), ev.ID); err != nil || !has {
		t.Fatalf("wrap not stored: %v", err)
	}
}

func TestConfiguredAuthKindsDoNotCloseMixedRequests(t *testing.T) {
	public := &nostr.Event{ID: strings.Repeat("1", 64), Kind: 1}
	protected := &nostr.Event{ID: strings.Repeat("2", 64), Kind: 4}
	cfg := nip17SecurityTestCfg()
	cfg.NIP42.RequireAuthSubscribeKinds = []int{4}
	st := &nip17OverReturnStore{events: []*nostr.Event{public, protected}}
	srv, err := NewServer(cfg, st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	c := registerTestConnLargeSend(t, srv, "configured")
	for _, filters := range [][]nostr.Filter{{{}}, {{Kinds: []int{1, 4}}}, {{Kinds: []int{1}}, {Kinds: []int{4}}}} {
		if err := handleREQ(t.Context(), srv, c, &nostr.ReqMessage{SubID: "s", Filters: filters}, false); err != nil {
			t.Fatal(err)
		}
		if got := snapshotIDs(t, c); !reflect.DeepEqual(got, []string{public.ID}) {
			t.Fatalf("configured kind leaked or public kind dropped: %v", got)
		}
	}
	srv.subs.RegisterSender(c.ID, func(b []byte) bool { return c.enqueue(b) == nil })
	srv.broadcastEvent(protected)
	if len(c.send) != 0 {
		t.Fatal("live configured kind bypassed AUTH")
	}
	c.nip42AddPubkey(strings.Repeat("a", 64))
	srv.broadcastEvent(protected)
	if len(c.send) != 1 {
		t.Fatal("authenticated configured kind was withheld")
	}
}
