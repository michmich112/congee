package relay

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/michmich112/congee/internal/db"
	"github.com/michmich112/congee/internal/nip77"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
	"github.com/michmich112/congee/internal/storage/turso"
	"github.com/rs/zerolog"
)

func TestValidateNegFilterPreservesBroadAndMixedRequests(t *testing.T) {
	cfg := nip17SecurityTestCfg()
	cfg.NIP42.RequireAuthSubscribeKinds = []int{1059}
	for _, filter := range []nostr.Filter{{}, {IDs: []string{strings.Repeat("a", 64)}}, {Kinds: []int{30402, 1059}}, {Kinds: []int{0, 21059}}} {
		if err := validateNegFilter(cfg, &Conn{}, &filter); err != nil {
			t.Fatalf("public portion rejected: %v", err)
		}
	}
	private := nostr.Filter{Kinds: []int{1059}}
	if err := validateNegFilter(cfg, &Conn{}, &private); err == nil || !strings.HasPrefix(err.Error(), "auth-required:") {
		t.Fatalf("want private-only AUTH requirement, got %v", err)
	}
	c := &Conn{}
	c.nip42AddPubkey(strings.Repeat("a", 64))
	if err := validateNegFilter(cfg, c, &private); err != nil {
		t.Fatalf("recipient recovery blocked: %v", err)
	}
}

func TestNEGOpenPrivateOnlyIssuesChallenge(t *testing.T) {
	cfg := nip17SecurityTestCfg()
	cfg.NIPs.Enabled = append(cfg.NIPs.Enabled, 77)
	srv, err := NewServer(cfg, &visibilityStoreStub{}, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	c := registerTestConnLargeSend(t, srv, "neg-auth")
	if err := handleNEGOpen(t.Context(), srv, c, &nostr.NegOpenMessage{SubID: "s", Filter: nostr.Filter{Kinds: []int{1059}}, InitialHex: "61"}); err != nil {
		t.Fatal(err)
	}
	var challenge, rejected bool
	for len(c.send) > 0 {
		var frame []any
		if err := json.Unmarshal(<-c.send, &frame); err != nil {
			t.Fatal(err)
		}
		if frame[0] == "AUTH" {
			challenge = true
		}
		if frame[0] == "NEG-ERR" && strings.HasPrefix(frame[2].(string), "auth-required:") {
			rejected = true
		}
	}
	if !challenge || !rejected {
		t.Fatal("missing AUTH challenge or auth-required NEG-ERR")
	}
}

func reconcileServerIDs(t *testing.T, srv *Server, c *Conn, filter nostr.Filter) []string {
	t.Helper()
	client := nip77.NewClientNegentropy(nip77.BuildVector(nil), 0)
	msg := &nostr.NegOpenMessage{SubID: "s", Filter: queryReadFilters(srv.cfg, c, []nostr.Filter{filter})[0], InitialHex: client.Start()}
	c.negSessions = newNegSessionMap()
	srv.metricsCtx = t.Context()
	srv.negLoadSlots = make(chan struct{}, 1)
	srv.negLoadSlots <- struct{}{}
	srv.negActiveSessions.Add(1)
	srv.runNegOpenJob(&negOpenJob{ctx: t.Context(), c: c, msg: msg})
	t.Cleanup(c.negSessions.closeAll)
	for round := 0; round < 10; round++ {
		var frame []any
		select {
		case data := <-c.send:
			if err := json.Unmarshal(data, &frame); err != nil {
				t.Fatal(err)
			}
		default:
			t.Fatal("missing NEG-MSG")
		}
		if frame[0] != "NEG-MSG" {
			t.Fatalf("reconciliation rejected: %v", frame)
		}
		out, err := client.Reconcile(frame[2].(string))
		if err != nil {
			t.Fatal(err)
		}
		if out == "" {
			var ids []string
			for id := range client.HaveNots {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			if err := handleNEGClose(t.Context(), srv, c, &nostr.NegCloseMessage{SubID: "s"}); err != nil {
				t.Fatal(err)
			}
			return ids
		}
		if err := handleNEGMsg(t.Context(), srv, c, &nostr.NegMsgMessage{SubID: "s", MessageHex: out}); err != nil {
			t.Fatal(err)
		}
	}
	t.Fatal("reconciliation did not finish")
	return nil
}

func TestNegentropyPublicAndRecipientScopedRecovery(t *testing.T) {
	if !turso.HasDriver() {
		t.Skip("libsql driver not available")
	}
	st, closeFn, err := db.OpenTestStore(t.Context(), filepath.Join(t.TempDir(), "events.db"), zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer closeFn()
	alice, bob := strings.Repeat("a", 64), strings.Repeat("b", 64)
	public := &nostr.Event{ID: strings.Repeat("1", 64), PubKey: alice, Kind: 30402, CreatedAt: 1}
	wrap := &nostr.Event{ID: strings.Repeat("2", 64), PubKey: alice, Kind: 1059, CreatedAt: 2, Tags: [][]string{{"p", alice}}}
	other := &nostr.Event{ID: strings.Repeat("3", 64), PubKey: alice, Kind: 1059, CreatedAt: 3, Tags: [][]string{{"p", bob}}}
	malformed := &nostr.Event{ID: strings.Repeat("4", 64), PubKey: alice, Kind: 1059, CreatedAt: 4, Tags: [][]string{{"p", alice}, {"p"}}}
	for _, ev := range []*nostr.Event{public, wrap, other, malformed} {
		if err := st.SaveEvent(t.Context(), ev); err != nil {
			t.Fatal(err)
		}
	}
	for _, mode := range []string{"anonymous", "alice", "bob", "disabled"} {
		for _, filter := range []nostr.Filter{{}, {Kinds: []int{30402, 1059}}, {IDs: []string{public.ID, wrap.ID, other.ID, malformed.ID}}} {
			cfg := nip17SecurityTestCfg()
			cfg.NIPs.Enabled = append(cfg.NIPs.Enabled, 77)
			maxRecords := 2
			cfg.NIP77.MaxRecordsPerQuery = maxRecords
			if mode == "disabled" {
				cfg.NIPs.Enabled = []int{1, 11, 42, 77}
			}
			srv, err := NewServer(cfg, st, zerolog.Nop(), nil)
			if err != nil {
				t.Fatal(err)
			}
			c := registerTestConnLargeSend(t, srv, "recover")
			want := []string{public.ID}
			switch mode {
			case "alice":
				c.nip42AddPubkey(alice)
				want = append(want, wrap.ID)
			case "bob":
				c.nip42AddPubkey(bob)
				want = append(want, other.ID)
			}
			if got := reconcileServerIDs(t, srv, c, filter); !reflect.DeepEqual(got, want) {
				t.Fatalf("mode=%s got=%v want=%v", mode, got, want)
			}
			if mode == "alice" || mode == "bob" {
				if got := reconcileServerIDs(t, srv, c, nostr.Filter{Kinds: []int{1059}}); !reflect.DeepEqual(got, want[1:]) {
					t.Fatalf("private-only recovery unavailable: mode=%s got=%v", mode, got)
				}
			}
		}
	}
}

type negOverReturnStore struct{ nip17OverReturnStore }

func (s *negOverReturnStore) QueryEventSyncItems(context.Context, nostr.Filter) ([]storage.SyncItem, error) {
	var items []storage.SyncItem
	for _, ev := range s.events {
		items = append(items, storage.SyncItem{ID: ev.ID, CreatedAt: ev.CreatedAt})
	}
	return items, nil
}

func TestNegentropyOverReturnedPrivateReferencesAreWithheld(t *testing.T) {
	alice := strings.Repeat("a", 64)
	public := &nostr.Event{ID: strings.Repeat("1", 64), Kind: 30402, CreatedAt: 1}
	wrap := &nostr.Event{ID: strings.Repeat("2", 64), Kind: 1059, CreatedAt: 2, Tags: [][]string{{"p", alice}}}
	st := &negOverReturnStore{nip17OverReturnStore{events: []*nostr.Event{public, wrap}}}
	cfg := nip17SecurityTestCfg()
	cfg.NIPs.Enabled = append(cfg.NIPs.Enabled, 77)
	srv, err := NewServer(cfg, st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	c := registerTestConnLargeSend(t, srv, "over-return")
	if got := reconcileServerIDs(t, srv, c, nostr.Filter{}); !reflect.DeepEqual(got, []string{public.ID}) {
		t.Fatalf("private metadata leaked: %v", got)
	}
}
