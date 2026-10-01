package upstream

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/db"
	"github.com/michmich112/congee/internal/nip77"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
	"github.com/michmich112/congee/internal/storage/turso"
	"github.com/rs/zerolog"
)

// protocolPeer exercises real JSON frames and Negentropy reconciliation without
// sockets. Only the advertised remote gift wrap can be fetched.
type protocolPeer struct {
	reconcile       func(string) (string, error)
	expectedInitial string
	recipient       string
	fresh           *nostr.Event
	frames          [][]byte
	fetched         int
}

func (p *protocolPeer) Close() error                    { return nil }
func (p *protocolPeer) SetReadDeadline(time.Time) error { return nil }
func (p *protocolPeer) ReadMessage() (int, []byte, error) {
	if len(p.frames) == 0 {
		return 0, nil, io.EOF
	}
	frame := p.frames[0]
	p.frames = p.frames[1:]
	return 1, frame, nil
}
func (p *protocolPeer) respond(frame []any) error {
	data, err := json.Marshal(frame)
	if err == nil {
		p.frames = append(p.frames, data)
	}
	return err
}
func (p *protocolPeer) WriteJSON(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var frame []json.RawMessage
	if err := json.Unmarshal(data, &frame); err != nil {
		return err
	}
	typ, subID := jsonStringAt(frame, 0), jsonStringAt(frame, 1)
	switch typ {
	case "NEG-OPEN":
		initial := jsonStringAt(frame, 3)
		if initial != p.expectedInitial {
			return fmt.Errorf("local inbox metadata entered vector: %s", initial)
		}
		var filter nostr.Filter
		if err := json.Unmarshal(frame[2], &filter); err != nil {
			return err
		}
		if len(filter.Tag["#p"]) != 1 || filter.Tag["#p"][0] != p.recipient {
			return fmt.Errorf("recipient filter lost on wire")
		}
		out, err := p.reconcile(initial)
		if err != nil {
			return err
		}
		return p.respond([]any{"NEG-MSG", subID, out})
	case "NEG-MSG":
		out, err := p.reconcile(jsonStringAt(frame, 2))
		if err != nil {
			return err
		}
		return p.respond([]any{"NEG-MSG", subID, out})
	case "REQ":
		var filter nostr.Filter
		if err := json.Unmarshal(frame[2], &filter); err != nil {
			return err
		}
		if len(filter.IDs) != 1 || filter.IDs[0] != p.fresh.ID {
			return fmt.Errorf("already stored wrapper fetched again: %v", filter.IDs)
		}
		p.fetched++
		return p.respond([]any{"EVENT", subID, p.fresh})
	case "CLOSE", "NEG-CLOSE":
		return nil
	default:
		return fmt.Errorf("unexpected frame: %s", typ)
	}
}

func TestUpstreamImportsGiftWrapsWithoutAdvertisingLocalInbox(t *testing.T) {
	if !turso.HasDriver() {
		t.Skip("libsql driver not available")
	}
	ctx := t.Context()
	st, closeFn, err := db.OpenTestStore(ctx, filepath.Join(t.TempDir(), "events.db"), zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer closeFn()
	alice := strings.Repeat("a", 64)
	public := &nostr.Event{ID: strings.Repeat("1", 64), PubKey: alice, Kind: 1, CreatedAt: 1, Tags: [][]string{{"p", alice}}}
	known := &nostr.Event{ID: strings.Repeat("2", 64), PubKey: alice, Kind: 1059, CreatedAt: 2, Tags: [][]string{{"p", alice}}}
	for _, ev := range []*nostr.Event{public, known} {
		if err := st.SaveEvent(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	priv, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	fresh := &nostr.Event{PubKey: hex.EncodeToString(priv.PubKey().SerializeCompressed()[1:]), Kind: 1059, CreatedAt: 3, Tags: [][]string{{"p", alice}}, Content: "cipher"}
	if err := fresh.Sign(priv); err != nil {
		t.Fatal(err)
	}
	publicVector := []storage.SyncItem{{ID: public.ID, CreatedAt: public.CreatedAt}}
	expectedInitial := nip77.NewClientNegentropy(nip77.BuildVector(publicVector), 0).Start()
	remoteVector := append(publicVector, storage.SyncItem{ID: known.ID, CreatedAt: known.CreatedAt}, storage.SyncItem{ID: fresh.ID, CreatedAt: fresh.CreatedAt})
	// More than the Negentropy channel capacity: every known private wrapper
	// is rediscovered remotely, but must neither stall nor be fetched again.
	for i := 0; i < 80; i++ {
		ev := &nostr.Event{ID: fmt.Sprintf("%064x", i+100), PubKey: alice, Kind: 1059, CreatedAt: int64(i + 4), Tags: [][]string{{"p", alice}}}
		if err := st.SaveEvent(ctx, ev); err != nil {
			t.Fatal(err)
		}
		remoteVector = append(remoteVector, storage.SyncItem{ID: ev.ID, CreatedAt: ev.CreatedAt})
	}
	remote := nip77.NewServerNegentropy(nip77.BuildVector(remoteVector), 0)
	peer := &protocolPeer{reconcile: remote.Reconcile, expectedInitial: expectedInitial, recipient: alice, fresh: fresh}
	c := &wsClient{conn: peer}
	sch := NewScheduler(config.DefaultConfig(), st, nil, nil, zerolog.Nop())
	filter := nostr.Filter{Kinds: []int{1, 1059}, Tag: map[string][]string{"#p": {alice}}}
	_, imported, err := syncFilter(ctx, sch, zerolog.Nop(), c, "wss://upstream.example/", filter, 0)
	if err != nil {
		t.Fatal(err)
	}
	if imported != 1 || peer.fetched != 1 {
		t.Fatalf("import unavailable or known wrap fetched: imported=%d fetched=%d", imported, peer.fetched)
	}
	if has, err := st.HasEventID(ctx, fresh.ID); err != nil || !has {
		t.Fatalf("new wrapper not stored: %v", err)
	}
}
