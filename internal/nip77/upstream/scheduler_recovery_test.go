package upstream

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/gorilla/websocket"
	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/db"
	"github.com/michmich112/congee/internal/nip77"
	"github.com/michmich112/congee/internal/nostr"
	"github.com/michmich112/congee/internal/storage"
	"github.com/michmich112/congee/internal/storage/turso"
	"github.com/rs/zerolog"
)

func TestUpstreamPullFailureThenRecovery(t *testing.T) {
	if !turso.HasDriver() {
		t.Skip("libsql driver not available")
	}
	ctx := context.Background()
	st, closeFn, err := db.OpenTestStore(ctx, filepath.Join(t.TempDir(), "events.db"), zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer closeFn()
	key, _ := btcec.PrivKeyFromBytes([]byte{11})
	ev := &nostr.Event{PubKey: hex.EncodeToString(key.PubKey().SerializeCompressed()[1:]), CreatedAt: 100, Kind: 1, Content: "recover"}
	if err := ev.Sign(key); err != nil {
		t.Fatal(err)
	}
	var fetches atomic.Int32
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		neg := nip77.NewServerNegentropy(nip77.BuildVector([]storage.SyncItem{{ID: ev.ID, CreatedAt: ev.CreatedAt}}), 1<<20)
		for {
			var raw []json.RawMessage
			if err := conn.ReadJSON(&raw); err != nil {
				return
			}
			if len(raw) < 2 {
				continue
			}
			typ, sub := jsonStringAt(raw, 0), jsonStringAt(raw, 1)
			switch typ {
			case "NEG-OPEN":
				if len(raw) < 4 {
					return
				}
				out, err := neg.Reconcile(jsonStringAt(raw, 3))
				if err != nil {
					return
				}
				if err := conn.WriteJSON([]any{"NEG-MSG", sub, out}); err != nil {
					return
				}
			case "NEG-MSG":
				if len(raw) < 3 {
					return
				}
				out, err := neg.Reconcile(jsonStringAt(raw, 2))
				if err != nil {
					return
				}
				if err := conn.WriteJSON([]any{"NEG-MSG", sub, out}); err != nil {
					return
				}
			case "REQ":
				if fetches.Add(1) <= maxFetchAttempts {
					if err := conn.WriteJSON([]any{"EOSE", sub}); err != nil {
						return
					}
				} else {
					if err := conn.WriteJSON([]any{"EVENT", sub, ev}); err != nil {
						return
					}
					if err := conn.WriteJSON([]any{"EOSE", sub}); err != nil {
						return
					}
				}
			}
		}
	}))
	defer upstream.Close()
	cfg := config.DefaultConfig()
	cfg.NIPs.Enabled = []int{1, 11, 77}
	cfg.NIP77.UpstreamEnabled = true
	cfg.NIP77.UpstreamAuthWaitSeconds = 0
	cfg.NIP77.UpstreamMessageTimeoutSeconds = 1
	u := config.NIP77Upstream{Name: "flaky", URL: "ws" + strings.TrimPrefix(upstream.URL, "http"), Enabled: true,
		Filters: []json.RawMessage{json.RawMessage(`{"kinds":[1]}`)}}
	sch := NewScheduler(cfg, st, nil, nil, zerolog.Nop())
	sch.runOnce(ctx, u)
	first := sch.Status()
	if len(first) != 1 || first[0].LastFailed != 1 || first[0].LastImported != 0 || first[0].LastError == "" {
		t.Fatalf("first run should report failure: %+v", first)
	}
	if got := fetches.Load(); got != maxFetchAttempts {
		t.Fatalf("first run should make %d bounded fetch attempts, got %d", maxFetchAttempts, got)
	}
	sch.runOnce(ctx, u)
	second := sch.Status()
	if len(second) != 1 || second[0].LastFailed != 0 || second[0].LastImported != 1 || second[0].LastError != "" {
		t.Fatalf("recovery should report success: %+v", second)
	}
	if got := fetches.Load(); got != maxFetchAttempts+1 {
		t.Fatalf("recovery should fetch once more, got %d attempts", got)
	}
	if has, err := st.HasEventID(ctx, ev.ID); err != nil || !has {
		t.Fatalf("recovered event missing: %v %v", has, err)
	}
}
