package relay

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/michmich112/congee/internal/audit"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
	"github.com/michmich112/congee/internal/storage/sqlite"
	"github.com/rs/zerolog"
)

func signedEventWithTags(t *testing.T, priv *btcec.PrivateKey, kind int, content string, tags [][]string) *nostr.Event {
	t.Helper()
	pubHex := hex.EncodeToString(priv.PubKey().SerializeCompressed()[1:])
	ev := &nostr.Event{
		PubKey:    pubHex,
		CreatedAt: time.Now().Unix(),
		Kind:      kind,
		Content:   content,
		Tags:      tags,
	}
	if _, err := ev.ComputeID(); err != nil {
		t.Fatal(err)
	}
	if err := ev.Sign(priv); err != nil {
		t.Fatal(err)
	}
	return ev
}

func openDeletionTestStore(t *testing.T) *sqlite.Store {
	t.Helper()
	ctx := context.Background()
	st, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "deletion.db"), nil, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func newDeletionTestServer(t *testing.T, st storage.Store) *Server {
	t.Helper()
	srv, err := NewServer(testRelayConfig(), st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	RegisterNIP01(srv, st)
	return srv
}

func publishEvent(t *testing.T, ctx context.Context, srv *Server, st storage.Store, c *Conn, ev *nostr.Event) {
	t.Helper()
	msg := &nostr.EventMessage{Event: *ev}
	if err := handleEVENT(ctx, srv, st, c, msg); err != nil {
		t.Fatal(err)
	}
}

func queryByID(t *testing.T, ctx context.Context, st storage.Store, id string) []*nostr.Event {
	t.Helper()
	out, err := st.QueryEvents(ctx, []nostr.Filter{{IDs: []string{id}}})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestKind5DeleteOwnEventByID(t *testing.T) {
	ctx := context.Background()
	st := openDeletionTestStore(t)
	srv := newDeletionTestServer(t, st)
	c := testConn(srv)
	defer c.cancel()

	priv, _ := btcec.NewPrivateKey()
	target := signedEventWithTags(t, priv, 1, "note", nil)
	publishEvent(t, ctx, srv, st, c, target)
	if len(queryByID(t, ctx, st, target.ID)) != 1 {
		t.Fatal("target not stored")
	}

	del := signedEventWithTags(t, priv, nostr.KindDeletion, "", [][]string{{"e", target.ID}})
	publishEvent(t, ctx, srv, st, c, del)

	if len(queryByID(t, ctx, st, target.ID)) != 0 {
		t.Fatal("deleted event still queryable by id")
	}
	if len(queryByID(t, ctx, st, del.ID)) != 1 {
		t.Fatal("kind-5 tombstone not stored")
	}

	rows, err := st.QueryAuditLog(ctx, storage.AuditQuery{Action: audit.ActionEventDeleted, Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("want one deletion audit row, got %d", len(rows))
	}
	if !strings.Contains(rows[0].Detail, "deleted_event_id="+target.ID) {
		t.Fatalf("audit detail: %q", rows[0].Detail)
	}
}

func TestKind5DeleteOwnAddressableEvent(t *testing.T) {
	ctx := context.Background()
	st := openDeletionTestStore(t)
	srv := newDeletionTestServer(t, st)
	c := testConn(srv)
	defer c.cancel()

	priv, _ := btcec.NewPrivateKey()
	pubHex := hex.EncodeToString(priv.PubKey().SerializeCompressed()[1:])
	const kind = 30023
	target := signedEventWithTags(t, priv, kind, "article", [][]string{{"d", "my-doc"}})
	publishEvent(t, ctx, srv, st, c, target)

	coord := fmt.Sprintf("%d:%s:my-doc", kind, pubHex)
	del := signedEventWithTags(t, priv, nostr.KindDeletion, "", [][]string{{"a", coord}})
	publishEvent(t, ctx, srv, st, c, del)

	if len(queryByID(t, ctx, st, target.ID)) != 0 {
		t.Fatal("addressable target still stored")
	}
}

func TestKind5CannotDeleteOtherAuthorsEvent(t *testing.T) {
	ctx := context.Background()
	st := openDeletionTestStore(t)
	srv := newDeletionTestServer(t, st)
	c := testConn(srv)
	defer c.cancel()

	owner, _ := btcec.NewPrivateKey()
	attacker, _ := btcec.NewPrivateKey()

	target := signedEventWithTags(t, owner, 1, "not yours", nil)
	publishEvent(t, ctx, srv, st, c, target)

	del := signedEventWithTags(t, attacker, nostr.KindDeletion, "", [][]string{{"e", target.ID}})
	publishEvent(t, ctx, srv, st, c, del)

	if len(queryByID(t, ctx, st, target.ID)) != 1 {
		t.Fatal("other author's event must not be deleted")
	}
	rows, err := st.QueryAuditLog(ctx, storage.AuditQuery{Action: audit.ActionEventDeleted, Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("unexpected deletion audit rows: %+v", rows)
	}
}

func TestKind5SubsequentREQExcludesDeleted(t *testing.T) {
	ctx := context.Background()
	st := openDeletionTestStore(t)
	srv := newDeletionTestServer(t, st)
	c := testConn(srv)
	defer c.cancel()

	priv, _ := btcec.NewPrivateKey()
	pubHex := hex.EncodeToString(priv.PubKey().SerializeCompressed()[1:])
	target := signedEventWithTags(t, priv, 1, "visible-then-gone", nil)
	publishEvent(t, ctx, srv, st, c, target)

	req := &nostr.ReqMessage{
		SubID:   "sub1",
		Filters: []nostr.Filter{{Authors: []string{pubHex}, Kinds: []int{1}}},
	}
	if err := handleREQ(ctx, srv, c, req); err != nil {
		t.Fatal(err)
	}
	if got := drainConnEvents(t, c); len(got) != 1 || got[0].ID != target.ID {
		t.Fatalf("initial REQ: want target event, got %+v", got)
	}

	del := signedEventWithTags(t, priv, nostr.KindDeletion, "", [][]string{{"e", target.ID}})
	publishEvent(t, ctx, srv, st, c, del)

	req2 := &nostr.ReqMessage{
		SubID:   "sub2",
		Filters: []nostr.Filter{{Authors: []string{pubHex}, Kinds: []int{1}}},
	}
	if err := handleREQ(ctx, srv, c, req2); err != nil {
		t.Fatal(err)
	}
	if got := drainConnEvents(t, c); len(got) != 0 {
		t.Fatalf("subsequent REQ must exclude deleted event, got %+v", got)
	}
}

func TestKind5RepublishAfterDelete(t *testing.T) {
	ctx := context.Background()
	st := openDeletionTestStore(t)
	srv := newDeletionTestServer(t, st)
	c := testConn(srv)
	defer c.cancel()

	priv, _ := btcec.NewPrivateKey()
	target := signedEventWithTags(t, priv, 1, "ephemeral life", nil)
	publishEvent(t, ctx, srv, st, c, target)

	del := signedEventWithTags(t, priv, nostr.KindDeletion, "", [][]string{{"e", target.ID}})
	publishEvent(t, ctx, srv, st, c, del)
	if len(queryByID(t, ctx, st, target.ID)) != 0 {
		t.Fatal("target should be deleted before republish")
	}

	// Re-publish the same signed event after deletion.
	publishEvent(t, ctx, srv, st, c, target)
	if len(queryByID(t, ctx, st, target.ID)) != 1 {
		t.Fatal("republished event should be stored again")
	}

	req := &nostr.ReqMessage{
		SubID:   "sub-repub",
		Filters: []nostr.Filter{{IDs: []string{target.ID}}},
	}
	if err := handleREQ(ctx, srv, c, req); err != nil {
		t.Fatal(err)
	}
	if got := drainConnEvents(t, c); len(got) != 1 || got[0].ID != target.ID {
		t.Fatalf("REQ after republish: got %+v", got)
	}
}

func drainConnEvents(t *testing.T, c *Conn) []*nostr.Event {
	t.Helper()
	deadline := time.After(500 * time.Millisecond)
	var out []*nostr.Event
	for {
		select {
		case raw := <-c.send:
			if ev, ok := parseRelayEventFrame(raw); ok {
				out = append(out, ev)
			}
		case <-deadline:
			return out
		}
	}
}

func parseRelayEventFrame(raw []byte) (*nostr.Event, bool) {
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil || len(arr) < 3 {
		return nil, false
	}
	var typ string
	if err := json.Unmarshal(arr[0], &typ); err != nil || typ != "EVENT" {
		return nil, false
	}
	var ev nostr.Event
	if err := json.Unmarshal(arr[2], &ev); err != nil {
		return nil, false
	}
	return &ev, true
}
